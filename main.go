package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gosimple/slug"
	"github.com/hako/durafmt"
	"github.com/streambinder/spotitube/config"
	"github.com/streambinder/spotitube/cui"
	"github.com/streambinder/spotitube/lyrics"
	"github.com/streambinder/spotitube/provider"
	"github.com/streambinder/spotitube/shell"
	"github.com/streambinder/spotitube/spotify"
	"github.com/streambinder/spotitube/system"
	"github.com/streambinder/spotitube/track"
	"github.com/streambinder/spotitube/upstream"
)

const (
	version          = 31
	cacheDuration    = 30 * time.Minute
	concurrencyLimit = 100
)

var (
	// flags for separate flows
	argCleanJunks bool
	argVersion    bool
	// flags for media sources
	argLibrary   bool
	argAlbums    system.StringsFlag
	argPlaylists system.StringsFlag
	argTracksFix system.StringsFlag
	// flags for options
	argFolder                string
	argFlushCache            bool
	argFlushLocal            bool
	argFlushMetadata         bool
	argDisableNormalization  bool
	argDisablePlaylistFile   bool
	argPlsFile               bool
	argDisableLyrics         bool
	argDisableUpdateCheck    bool
	argDisableBrowserOpening bool
	argDisableIndexing       bool
	argAuthenticateOutside   bool
	argInteractive           bool
	argInput                 bool
	// flags for troubleshooting
	argLog        bool
	argDebug      bool
	argSimulate   bool
	argDisableGui bool

	// spotify
	c            *spotify.Client
	cUser        string
	cUserID      string
	tracks       = make(map[*track.Track]*track.SyncOptions)
	tracksIndex  = make(map[string]uint64)
	artworks     = make(map[string]*[]byte)
	playlists    []*track.Playlist
	tracksFailed []*track.Track
	index        *track.TracksIndex

	// routines
	waitGroup     sync.WaitGroup
	waitGroupPool = make(chan bool, concurrencyLimit)

	// cli
	ui *cui.CUI

	// config
	cfg *config.Config

	// user paths
	usrGob       = config.RelativeTo("%s_%s.gob", config.CachePath)
	usrIndex     = config.RelativeTo("index.gob", config.CachePath)
	regUsrBinary = regexp.MustCompile(`spotitube\.[0-9]+`)
)

func main() {
	mainSetup()
	mainFork()
	mainEnvCheck()
	mainFlagsParse()
	mainInit()
	mainUI()
	mainAuthenticate()
	mainFetch()
	mainSearch()
	mainExit()
}

func mainSetup() {
	system.Mkdir(config.CacheDir())
}

func mainFork() {
	var binVersion int
	filepath.Walk(config.CacheDir(), func(path string, fileInfo os.FileInfo, _ error) error {
		if fileInfo == nil || fileInfo.IsDir() ||
			!regUsrBinary.MatchString(fileInfo.Name()) {
			return nil
		}

		if subVersion, err := strconv.Atoi(filepath.Ext(fileInfo.Name())[1:]); err == nil && subVersion > binVersion {
			binVersion = subVersion
		}
		return nil
	})

	if binVersion > version && os.Getenv("SPOTITUBE_FORK") != "1" {
		syscall.Exec(config.CacheBin(binVersion), os.Args, append(os.Environ(), []string{"SPOTITUBE_FORK=1"}...))
		mainExit()
	}
}

func mainEnvCheck() {
	if err := spotify.Ready(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if !shell.YoutubeDL().Exists() {
		fmt.Println(fmt.Sprintf("%s command is not installed.", shell.YoutubeDL().Name()))
		os.Exit(1)
	}

	if !shell.FFmpeg().Exists() {
		fmt.Println(fmt.Sprintf("%s command is not installed.", shell.FFmpeg().Name()))
		os.Exit(1)
	}

	if !system.IsOnline() {
		fmt.Println("You're not connected to the internet.")
		os.Exit(1)
	}

	if upstreamVersion, err := upstream.Version(); err == nil &&
		!argDisableUpdateCheck && upstreamVersion > version {
		fmt.Println(fmt.Sprintf("You're not running latest version (%d). Going to update.", upstreamVersion))
		if err := upstream.Download(config.CacheBin(upstreamVersion)); err != nil {
			fmt.Println(fmt.Sprintf("Unable to update: %s", err.Error()))
		} else {
			mainFork()
		}
	}
}

func mainFlagsParse() {
	// separate flows
	flag.BoolVar(&argCleanJunks, "clean-junks", false, "Scan for and clean junk files")
	flag.BoolVar(&argVersion, "version", false, "Print version")

	// media sources
	flag.BoolVar(&argLibrary, "library", false, "Synchronize user library")
	flag.Var(&argAlbums, "album", "Album URI to synchronize")
	flag.Var(&argPlaylists, "playlist", "Playlist URI to synchronize")
	flag.Var(&argTracksFix, "fix", "Offline song filename(s) which straighten the shot to")

	// options
	flag.StringVar(&argFolder, "folder", ".", "Folder to sync your tracks collection into")
	flag.BoolVar(&argFlushCache, "flush-cache", false, "Force Spotify tracks collection cache flush")
	flag.BoolVar(&argFlushLocal, "flush-local", false, "Flush already downloaded tracks if better results get encountered")
	flag.BoolVar(&argFlushMetadata, "flush-metadata", false, "Flush metadata tags to already synchronized songs")
	flag.BoolVar(&argDisableNormalization, "disable-normalization", false, "Disable songs volume normalization")
	flag.BoolVar(&argDisablePlaylistFile, "disable-playlist-file", false, "Disable automatic creation of playlists file")
	flag.BoolVar(&argPlsFile, "pls-file", false, "Generate playlist file with .pls instead of .m3u")
	flag.BoolVar(&argDisableLyrics, "disable-lyrics", false, "Disable download of songs lyrics and their application into mp3")
	flag.BoolVar(&argDisableUpdateCheck, "disable-update-check", false, "Disable automatic update check at startup")
	flag.BoolVar(&argDisableBrowserOpening, "disable-browser-opening", false, "Disable automatic browser opening for authentication")
	flag.BoolVar(&argDisableIndexing, "disable-indexing", false, "Disable automatic library indexing (used to keep track of tracks names modifications)")
	flag.BoolVar(&argAuthenticateOutside, "authenticate-outside", false, "Enable authentication flow to be handled outside this machine")
	flag.BoolVar(&argInteractive, "interactive", false, "Enable interactive mode")
	flag.BoolVar(&argInput, "input", false, "Always manually insert URL used for songs download")

	// troubleshooting
	flag.BoolVar(&argLog, "log", false, "Enable logging into file ./spotitube.log")
	flag.BoolVar(&argDebug, "debug", false, "Enable debug messages")
	flag.BoolVar(&argSimulate, "simulate", false, "Simulate process flow, without really altering filesystem")
	flag.BoolVar(&argDisableGui, "disable-gui", false, "Disable GUI to reduce noise and increase readability of program flow")
	flag.Parse()

	if !(argAlbums.IsSet() || argPlaylists.IsSet() || argTracksFix.IsSet()) {
		argLibrary = true
	}

	if argTracksFix.IsSet() {
		for index, track := range argTracksFix.Entries {
			trackAbs, err := filepath.Abs(track)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			argTracksFix.Entries[index] = trackAbs
		}
	}

	if argAuthenticateOutside {
		argDisableBrowserOpening = true
	}
}

func mainInit() {
	system.TrapSignal(os.Interrupt, func() {
		mainExit()
	})

	if argVersion {
		fmt.Println(fmt.Sprintf("SpotiTube, version %d.", version))
		os.Exit(0)
	}

	if argCleanJunks {
		fmt.Println(fmt.Sprintf("Removed %d junks.", system.FileWildcardDelete(argFolder, track.JunkWildcards()...)))
		os.Exit(0)
	}

	// create configuration instance
	var err error
	cfg, err = config.Parse()
	if err != nil {
		fmt.Println(fmt.Sprintf("Unable to read config file: %s", err.Error()))
		os.Exit(1)
	}

	if argFolder == "." && cfg.Folder != "" {
		argFolder = cfg.Folder
	}
	if !(system.Dir(argFolder)) {
		fmt.Println(fmt.Sprintf("Chosen music folder does not exist: %s", argFolder))
		os.Exit(1)
	}
	os.Chdir(argFolder)

	if !argDisableIndexing {
		if system.FileExists(usrIndex) {
			system.FetchGob(usrIndex, index)
		}
		index = track.Index(argFolder)
	}

	for range [concurrencyLimit]int{} {
		waitGroupPool <- true
	}
}

func mainUI() {
	var (
		uiOpts uint64
		uiErr  error
	)

	if argDebug {
		uiOpts |= cui.GuiDebugMode
	}

	if argDisableGui {
		uiOpts |= cui.GuiBareMode
	}

	if argLog {
		uiOpts |= cui.LogEnable
	}

	if ui, uiErr = cui.Startup(uiOpts); uiErr != nil {
		fmt.Println(fmt.Sprintf("Unable to build user interface: %s", uiErr.Error()))
		os.Exit(1)
	}

	ui.OnShutdown(func() {
		mainExit()
	})

	ui.Append(fmt.Sprintf("%s %s", cui.Font("Folder:", cui.StyleBold), system.PrettyPath(argFolder)), cui.PanelLeftTop)
	ui.Append(fmt.Sprintf("%s %s", cui.Font(fmt.Sprintf("%s version:", shell.YoutubeDL().Name()), cui.StyleBold), shell.YoutubeDL().Version()), cui.PanelLeftTop)
	ui.Append(fmt.Sprintf("%s %s", cui.Font(fmt.Sprintf("%s version:", shell.FFmpeg().Name()), cui.StyleBold), shell.FFmpeg().Version()), cui.PanelLeftTop)
	ui.Append(fmt.Sprintf("%s %d", cui.Font("Version:", cui.StyleBold), version), cui.PanelLeftBottom)
	ui.Append(fmt.Sprintf("%s %s", cui.Font("Date:", cui.StyleBold), time.Now().Format("2006-01-02 15:04:05")), cui.PanelLeftBottom)
	ui.Append(fmt.Sprintf("%s GPL-3.0", cui.Font("License:", cui.StyleBold)), cui.PanelLeftBottom)
}

func mainAuthenticate() {
	host := "localhost"
	if argAuthenticateOutside {
		host = "spotitube.local"
		ui.Prompt("Outside authentication enabled: assure \"spotitube.local\" points to this machine.")
	}

	uri := spotify.BuildAuthURL(host)
	ui.Append(fmt.Sprintf("Authentication URL: %s", uri.Short), cui.ParagraphAutoReturn)
	if !argDisableBrowserOpening {
		ui.Append("Waiting for automatic login process. If wait is too long, manually open that URL.", cui.DebugAppend)
	}

	var err error
	if c, err = spotify.Auth(uri.Full, host, !argDisableBrowserOpening); err != nil {
		ui.Prompt(fmt.Sprintf("Authentication failed: %s", err.Error()), cui.PromptExit)
	}

	cUser, cUserID = c.User()
	ui.Append("Authentication completed.")
	ui.Append(fmt.Sprintf("%s %s", cui.Font("Session user:", cui.StyleBold), cUser), cui.PanelLeftTop)
}

func mainFetch() {
	mainFetchLibrary()
	mainFetchAlbums()
	mainFetchPlaylists()
	mainFetchTracksToFix()

	if argFlushLocal || argFlushMetadata {
		for _, opts := range tracks {
			opts.Metadata = true
			if argFlushLocal {
				opts.Source = true
			}
		}
	}

	ui.Append(fmt.Sprintf("%s %d", cui.Font("Songs online:", cui.StyleBold), len(tracks)), cui.PanelLeftTop)
	ui.Append(fmt.Sprintf("%s %d", cui.Font("Songs offline:", cui.StyleBold), track.CountOffline(tracks)), cui.PanelLeftTop)
	ui.Append(fmt.Sprintf("%s %d", cui.Font("Songs missing:", cui.StyleBold), track.CountOnline(tracks)), cui.PanelLeftTop)

	track.IndexWait()

	if len(tracks) == 0 {
		ui.Prompt("No song needs to be downloaded.", cui.PromptExit)
		mainExit()
	}
}

func mainFetchLibrary() {
	if !argLibrary {
		return
	}

	gob := fmt.Sprintf(usrGob, cUserID, "library")
	if argFlushCache {
		os.Remove(gob)
	}

	dump, dumpErr := fetchDump(gob)
	if dumpErr == nil && time.Since(dump.Time) < cacheDuration {
		ui.Append(
			fmt.Sprintf("%s %s / %s",
				cui.Font("Library cache:", cui.StyleBold),
				durafmt.ParseShort(time.Since(dump.Time)).String(),
				durafmt.ParseShort(cacheDuration).String()),
			cui.PanelLeftTop)
		for _, t := range dump.Tracks {
			tracksInflate(t)
		}
		return
	}

	ui.Append("Fetching music library...")
	library, err := c.LibraryTracks()
	if err != nil {
		ui.Prompt(fmt.Sprintf("Unable to fetch library: %s", err.Error()), cui.PromptExit)
	}

	if err := system.DumpGob(gob, track.TracksDump{Tracks: library, Time: time.Now()}); err != nil {
		ui.Append(fmt.Sprintf("Unable to cache tracks: %s", err.Error()), cui.WarningAppend)
	}

	for _, t := range library {
		tracksInflate(t)
	}
}

func mainFetchAlbums() {
	if !argAlbums.IsSet() {
		return
	}

	for _, uri := range argAlbums.Entries {
		alias := cfg.URI(uri)
		if len(alias) > 0 {
			uri = alias
		}

		gob := fmt.Sprintf(usrGob, cUserID, fmt.Sprintf("album_%s", spotify.IDFromURI(uri)))
		if argFlushCache {
			os.Remove(gob)
		}

		dump, dumpErr := fetchDump(gob)
		if dumpErr == nil && time.Since(dump.Time) < cacheDuration {
			ui.Append(
				fmt.Sprintf("%s %s / %s",
					cui.Font(fmt.Sprintf("%s cache:",
						spotify.IDFromURI(uri)), cui.StyleBold),
					durafmt.ParseShort(time.Since(dump.Time)).String(),
					durafmt.ParseShort(cacheDuration).String()),
				cui.PanelLeftTop)
			for _, t := range dump.Tracks {
				tracksInflate(t)
			}
			continue
		}

		ui.Append(fmt.Sprintf("Fetching album %s...", spotify.IDFromURI(uri)))
		album, err := c.AlbumTracks(uri)
		if err != nil {
			ui.Prompt(fmt.Sprintf("Unable to fetch album: %s", err.Error()), cui.PromptExit)
		}

		if err := system.DumpGob(gob, track.TracksDump{Tracks: album, Time: time.Now()}); err != nil {
			ui.Append(fmt.Sprintf("Unable to cache tracks: %s", err.Error()), cui.WarningAppend)
		}

		for _, t := range album {
			tracksInflate(t)
		}
	}
}

func mainFetchPlaylists() {
	if !argPlaylists.IsSet() {
		return
	}

	for _, uri := range argPlaylists.Entries {
		alias := cfg.URI(uri)
		if len(alias) > 0 {
			uri = alias
		}

		playlist := &track.Playlist{}
		if p, err := c.Playlist(uri); err == nil {
			playlist.Name = p.Name
			playlist.Owner = p.Owner.DisplayName
		}

		gob := fmt.Sprintf(usrGob, cUserID, fmt.Sprintf("playlist_%s", spotify.IDFromURI(uri)))
		if argFlushCache {
			os.Remove(gob)
		}

		dump, dumpErr := fetchDump(gob)
		if dumpErr == nil && time.Since(dump.Time) < cacheDuration {
			ui.Append(
				fmt.Sprintf("%s %s / %s",
					cui.Font(fmt.Sprintf("%s cache:", playlist.Name), cui.StyleBold),
					durafmt.ParseShort(time.Since(dump.Time)).String(),
					durafmt.ParseShort(cacheDuration).String()),
				cui.PanelLeftTop)
			for _, t := range dump.Tracks {
				tracksInflate(t)
			}
			playlist.Tracks = dump.Tracks
			playlists = append(playlists, playlist)
			continue
		}

		ui.Append(fmt.Sprintf("Fetching playlist %s...", spotify.IDFromURI(uri)))
		p, err := c.PlaylistTracks(uri)
		if err != nil {
			ui.Prompt(fmt.Sprintf("Unable to fetch playlist: %s", err.Error()), cui.PromptExit)
		}

		if err := system.DumpGob(gob, track.TracksDump{Tracks: p, Time: time.Now()}); err != nil {
			ui.Append(fmt.Sprintf("Unable to cache tracks: %s", err.Error()), cui.WarningAppend)
		}

		for _, t := range p {
			tracksInflate(t)
		}

		playlist.Tracks = p
		playlists = append(playlists, playlist)
	}
}

func mainFetchTracksToFix() {
	if !argTracksFix.IsSet() {
		return
	}

	ui.Append(fmt.Sprintf("%s %d", cui.Font("Fix song(s):", cui.StyleBold), len(argTracksFix.Entries)), cui.PanelLeftTop)
	for _, tFix := range argTracksFix.Entries {
		if t, err := track.OpenLocalTrack(tFix); err != nil {
			ui.Prompt(fmt.Sprintf("Something went wrong: %s", err.Error()))
		} else {
			tracksInflateWithOption(t, track.SyncOptionsFlush())
		}
	}
}

func mainSearch() {
	ctr := 0
	songsFetch, songsFlush, songsIgnore := countSongs()

	ui.ProgressMax = len(tracks)
	ui.Append(fmt.Sprintf("%d will be downloaded, %d flushed and %d ignored", songsFetch, songsFlush, songsIgnore))

	for track, trackOpts := range tracks {
		ctr++
		ui.ProgressIncrease()
		ui.Append(fmt.Sprintf("%d/%d: \"%s\"", ctr, len(tracks), track.Basename()), cui.StyleBold)

		// rename local file if Spotify has renamed it
		if path, match, err := index.Match(track.SpotifyID, track.Filename()); err == nil && !match {
			ui.Append(fmt.Sprintf("Track %s has been renamed: moving to %s", track.Filename(), path))
			if err := os.Rename(path, track.Filename()); err != nil {
				ui.Append(fmt.Sprintf("Unable to rename: %s", err.Error()), cui.ErrorAppend)
			} else {
				index.Rename(track.SpotifyID, track.Filename())
				if !argFlushLocal {
					trackOpts.Source = false
				}
			}
		}

		if !track.Local() || trackOpts.Source || argSimulate {
			entry := new(provider.Entry)

			for _, p := range provider.All() {
				ui.Append(fmt.Sprintf("Searching entries on %s provider", p.Name()), cui.DebugAppend)

				var (
					provEntries = []*provider.Entry{}
					provErr     error
					entryPick   bool
				)

				if !argInput {
					provEntries, provErr = p.Query(track)
					if provErr != nil {
						ui.Append(
							fmt.Sprintf("Unable to search %s on %s provider: %s.", track.Basename(), p.Name(), provErr.Error()),
							cui.WarningAppend)
						continue
					}

					for _, provEntry := range provEntries {
						ui.Append(
							fmt.Sprintf("Result met: ID: %s,\nTitle: %s,\nUser: %s,\nDuration: %d.",
								provEntry.ID, provEntry.Title, provEntry.User, provEntry.Duration),
							cui.DebugAppend)

						entryPick = bool(p.Match(provEntry, track) == nil)
						if argInteractive {
							entryPick = ui.Prompt(
								fmt.Sprintf(
									"Track: %s\n\nID: %s\nTitle: %s\nUser: %s\nDuration: %d\nURL: %s\nResult is matching: %s",
									track.Basename(), provEntry.ID, provEntry.Title, provEntry.User,
									provEntry.Duration, provEntry.URL, strconv.FormatBool(entryPick)),
								cui.PromptBinary)
						}

						if entryPick {
							ui.Append(fmt.Sprintf("Video \"%s\" is good to go for \"%s\".", provEntry.Title, track.Basename()))
							entry = provEntry
							break
						}
					}
				}

				if argInput && entry.Empty() {
					if url := ui.PromptInputMessage(fmt.Sprintf("Enter URL for \"%s\"", track.Basename()), cui.PromptInput); len(url) > 0 {
						if err := p.Support(url); err == nil {
							entry.URL = url
						} else {
							ui.Prompt(fmt.Sprintf("Something went wrong: %s", err.Error()))
						}
					}
				}

				if entry.Empty() {
					ui.Append("No entry to download has been found.", cui.ErrorAppend)
					tracksFailed = append(tracksFailed, track)
					continue
				}

				if argSimulate {
					ui.Append(fmt.Sprintf("I would like to download \"%s\" for \"%s\" track, but I'm just simulating.", entry.Repr(), track.Basename()))
					continue
				}

				if trackOpts.Source && track.URL == entry.URL {
					ui.Append("Downloaded track is still the best result I can find.")
					ui.Append(fmt.Sprintf("Local track origin URL %s is the same as the chosen one %s.", track.URL, entry.URL), cui.DebugAppend)
					continue
				}
			}

			if entry.Empty() || track.URL == entry.URL {
				continue
			}

			ui.Append(fmt.Sprintf("Going to download %s...", entry.URL))
			p, err := provider.For(entry.URL)
			if err != nil {
				ui.Append(fmt.Sprintf("Unable to reconstruct provider for \"%s\"", entry.URL), cui.ErrorAppend)
				continue
			}

			if err := p.Download(entry, track.FilenameTemporary()); err != nil {
				ui.Append(fmt.Sprintf("Something went wrong downloading \"%s\": %s.", track.Basename(), err.Error()), cui.WarningAppend)
				tracksFailed = append(tracksFailed, track)
				continue
			}

			track.URL = entry.URL
		}

		if track.Local() && !trackOpts.Metadata {
			continue
		}

		if err := songFetchLyrics(track); err != nil {
			ui.Append(err.Error(), cui.WarningAppend)
		}

		if err := songFetchArtwork(track); err != nil {
			ui.Append(err.Error(), cui.WarningAppend)
		}

		ui.Append(fmt.Sprintf("Launching track processing jobs..."))
		waitGroup.Add(1)
		go songProcess(track, trackOpts, &waitGroup)
		if argDebug {
			waitGroup.Wait()
		}
	}

	waitGroup.Wait()

	flushPlaylists()

	index.Sync(usrIndex)

	close(waitGroupPool)
	waitGroup.Wait()
	ui.ProgressFill()

	if len(tracksFailed) > 0 {
		ui.Append(fmt.Sprintf("%d tracks failed to synchronize.", len(tracksFailed)))
		for _, t := range tracksFailed {
			ui.Append(" - " + t.Basename())
		}
		system.Notify("SpotiTube", "emblem-downloads", "SpotiTube", fmt.Sprintf("%d track(s) synced, %d failed.", len(tracks)-len(tracksFailed), len(tracksFailed)))
	} else {
		system.Notify("SpotiTube", "emblem-downloads", "SpotiTube", fmt.Sprintf("%d track(s) synced", len(tracks)))
	}

	ui.Prompt("Synchronization completed.", cui.PromptExit)
}

func mainExit(delay ...time.Duration) {
	system.FileWildcardDelete(argFolder, track.JunkWildcards()...)

	if len(delay) > 0 {
		time.Sleep(delay[0])
	}

	os.Exit(0)
}

func tracksInflate(t *track.Track) {
	var opts *track.SyncOptions = track.SyncOptionsFlush()
	if t.Local() {
		opts = track.SyncOptionsDefault()
	}
	tracksInflateWithOption(t, opts)
}

func tracksInflateWithOption(t *track.Track, opts *track.SyncOptions) {
	var indexKey = t.FilenameTemporary()
	if _, isDup := tracksIndex[indexKey]; isDup {
		return
	}

	tracks[t] = opts
	tracksIndex[indexKey] = 1
}

func songFetchLyrics(t *track.Track) error {
	if argDisableLyrics {
		return nil
	}

	for _, p := range lyrics.All() {
		ui.Append(fmt.Sprintf("Searching lyrics using %s provider...", p.Name()), cui.DebugAppend)

		text, err := p.Query(t.Song, t.Artist)
		if err != nil {
			ui.Append(fmt.Sprintf("Provider encountered an error: %s", err.Error()))
			continue
		}

		t.Lyrics = text
		return nil
	}

	return fmt.Errorf("Lyrics not found")
}

func songFetchArtwork(t *track.Track) error {
	if len(t.ArtworkURL) == 0 {
		return nil
	}

	if val, ok := artworks[t.ArtworkURL]; ok {
		t.Artwork = val
		return nil
	}

	resp, err := http.Get(t.ArtworkURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	artworks[t.ArtworkURL] = &body
	t.Artwork = &body
	return nil
}

func songProcess(track *track.Track, opts *track.SyncOptions, wg *sync.WaitGroup) {
	defer wg.Done()
	<-waitGroupPool
	defer func() {
		waitGroupPool <- true
	}()

	// moving to temporary song
	if !system.FileExists(track.FilenameTemporary()) {
		if err := system.FileCopy(track.Filename(), track.FilenameTemporary()); err != nil {
			ui.Append(err.Error(), cui.ErrorAppend)
			return
		}
	}

	// volume normalization
	if err := songNormalize(track, opts); err != nil {
		ui.Append(err.Error(), cui.ErrorAppend)
	}

	// metadata flush
	if err := songMetadataFlush(track, opts); err != nil {
		ui.Append(err.Error(), cui.ErrorAppend)
	}

	// track rename
	os.Remove(track.Filename())
	if err := os.Rename(track.FilenameTemporary(), track.Filename()); err != nil {
		ui.Append(fmt.Sprintf("Unable to move song to its final path: %s", err.Error()), cui.WarningAppend)
	}
}

func songNormalize(track *track.Track, opts *track.SyncOptions) error {
	if !opts.Normalization {
		return nil
	}

	volume, err := shell.FFmpeg().VolumeDetect(track.FilenameTemporary())
	if err != nil {
		return err
	}

	if volume > 0 {
		return nil
	}

	return shell.FFmpeg().VolumeIncrease(math.Abs(volume), track.FilenameTemporary())
}

func songMetadataFlush(track *track.Track, opts *track.SyncOptions) error {
	if !opts.Metadata {
		return nil
	}

	return track.Flush()
}

func fetchDump(path string) (dump track.TracksDump, err error) {
	if err := system.FetchGob(path, &dump); err != nil {
		return dump, err
	}

	if time.Since(dump.Time) > cacheDuration {
		return dump, fmt.Errorf("Tracks cache is obsolete")
	}

	return dump, nil
}

func countSongs() (int, int, int) {
	var (
		fetch  int
		flush  int
		ignore int
	)

	if argFlushLocal {
		fetch = len(tracks)
		flush = fetch
	} else if argFlushMetadata {
		fetch = track.CountOnline(tracks)
		flush = len(tracks)
	} else {
		fetch = track.CountOnline(tracks)
		flush = fetch
		ignore = track.CountOffline(tracks)
	}

	return fetch, flush, ignore
}

func flushPlaylists() {
	if argSimulate || argDisablePlaylistFile || len(argPlaylists.Entries) == 0 {
		return
	}

	for _, p := range playlists {
		var (
			pFname   = slug.Make(p.Name)
			pContent string
		)

		if argPlsFile {
			pFname = pFname + ".pls"
		} else {
			pFname = pFname + ".m3u"
		}

		os.Remove(pFname)
		ui.Append(fmt.Sprintf("Creating playlist file at %s...", pFname))
		if system.FileExists(pFname) {
			os.Remove(pFname)
		}

		if argPlsFile {
			pContent = p.PLS(".")
		} else {
			pContent = p.M3U(".")
		}

		pFile, err := os.Create(pFname)
		if err != nil {
			ui.Append(fmt.Sprintf("Unable to create playlist file: %s", err.Error()), cui.WarningAppend)
			continue
		}
		defer pFile.Close()
		defer pFile.Sync()

		if _, err := pFile.WriteString(pContent); err != nil {
			ui.Append(fmt.Sprintf("Unable to write playlist file: %s", err.Error()), cui.WarningAppend)
		}
	}
}
