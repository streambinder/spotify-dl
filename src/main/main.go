package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	spttb_gui "gui"
	spttb_spotify "spotify"
	spttb_system "system"
	spttb_track "track"
	spttb_youtube "youtube"

	id3 "github.com/bogem/id3v2"
	api "github.com/zmb3/spotify"
)

var (
	arg_folder                  *string
	arg_playlist                *string
	arg_replace_local           *bool
	arg_flush_metadata          *bool
	arg_flush_missing           *bool
	arg_disable_normalization   *bool
	arg_disable_m3u             *bool
	arg_disable_lyrics          *bool
	arg_disable_timestamp_flush *bool
	arg_disable_update_check    *bool
	arg_interactive             *bool
	arg_clean_junks             *bool
	arg_log                     *bool
	arg_debug                   *bool
	arg_simulate                *bool
	arg_version                 *bool

	tracks           spttb_track.Tracks
	tracks_failed    spttb_track.Tracks
	playlist_info    *api.FullPlaylist
	youtube_client   *spttb_youtube.YouTube = spttb_youtube.NewClient()
	spotify_client   *spttb_spotify.Spotify = spttb_spotify.NewClient()
	wait_group       sync.WaitGroup
	wait_group_limit syscall.Rlimit

	gui *spttb_gui.Gui
)

func main() {
	arg_folder = flag.String("folder", ".", "Folder to sync with music.")
	arg_playlist = flag.String("playlist", "none", "Playlist URI to synchronize.")
	arg_replace_local = flag.Bool("replace-local", false, "Replace local library songs if better results get encountered")
	arg_flush_metadata = flag.Bool("flush-metadata", false, "Flush metadata informations to already synchronized songs")
	arg_flush_missing = flag.Bool("flush-missing", false, "If -flush-metadata toggled, it will just populate empty id3 frames, instead of flushing any of those")
	arg_disable_normalization = flag.Bool("disable-normalization", false, "Disable songs volume normalization")
	arg_disable_m3u = flag.Bool("disable-m3u", false, "Disable automatic creation of playlists .m3u file")
	arg_disable_lyrics = flag.Bool("disable-lyrics", false, "Disable download of songs lyrics and their application into mp3.")
	arg_disable_timestamp_flush = flag.Bool("disable-timestamp-flush", false, "Disable automatic songs files timestamps flush")
	arg_disable_update_check = flag.Bool("disable-update-check", false, "Disable automatic update check at startup")
	arg_interactive = flag.Bool("interactive", false, "Enable interactive mode")
	arg_clean_junks = flag.Bool("clean-junks", false, "Scan for junks file and clean them")
	arg_log = flag.Bool("log", false, "Enable logging into file ./spotitube.log")
	arg_debug = flag.Bool("debug", false, "Enable debug messages")
	arg_simulate = flag.Bool("simulate", false, "Simulate process flow, without really altering filesystem")
	arg_version = flag.Bool("version", false, "Print version")
	flag.Parse()

	if *arg_version {
		fmt.Println(fmt.Sprintf("SpotiTube, version %d.", spttb_system.VERSION))
		os.Exit(0)
	}

	if !(spttb_system.IsDir(*arg_folder)) {
		fmt.Println("Chosen music folder does not exist: " + *arg_folder)
		os.Exit(1)
	} else {
		*arg_folder, _ = filepath.Abs(*arg_folder)
		os.Chdir(*arg_folder)
	}

	if *arg_clean_junks {
		junks := CleanJunks()
		fmt.Println(fmt.Sprintf("Removed %d junk files.", junks))
		os.Exit(0)
	}

	gui = spttb_gui.Build()
	gui.Append("SPOTITUBE", spttb_gui.PanelLeftTop, spttb_gui.OrientationCenter)
	gui.Append(fmt.Sprintf("Version: %d", spttb_system.VERSION), spttb_gui.PanelLeftTop, spttb_gui.OrientationCenter)
	gui.Append(fmt.Sprintf("Folder: %s", *arg_folder), spttb_gui.PanelLeftTop, spttb_gui.OrientationCenter)
	if *arg_log {
		gui.Append(fmt.Sprintf("Log filename: %s", spttb_system.DEFAULT_LOG_PATH), spttb_gui.PanelLeftTop, spttb_gui.OrientationCenter)

	}
	gui.Append(fmt.Sprintf("Date: %s", time.Now().Format("2006-01-02 15:04:05")), spttb_gui.PanelLeftBottom, spttb_gui.OrientationCenter)
	gui.Append(fmt.Sprintf("URL: %s", spttb_system.VERSION_REPOSITORY), spttb_gui.PanelLeftBottom, spttb_gui.OrientationCenter)
	gui.Append(fmt.Sprintf("License: GPLv2"), spttb_gui.PanelLeftBottom, spttb_gui.OrientationCenter)

	for _, command_name := range []string{"youtube-dl", "ffmpeg"} {
		_, err := exec.LookPath(command_name)
		if err != nil {
			gui.Prompt("Are you sure "+command_name+" is actually installed?", spttb_gui.PromptDismissableWithExit)
		}
	}

	_, err := net.Dial("tcp", spttb_system.DEFAULT_TCP_CHECK)
	if err != nil {
		gui.Prompt("Are you sure you're connected to the internet?", spttb_gui.PromptDismissableWithExit)
	}

	if *arg_log {
		// TODO: logger.SetFile(spttb_system.DEFAULT_LOG_PATH)
	}

	// TODO: pass debug to logger
	// if *arg_debug {
	// 	// logger.EnableDebug()
	// }

	if !*arg_disable_update_check {
		type OnlineVersion struct {
			Name string `json:"name"`
		}
		version_client := http.Client{
			Timeout: time.Second * spttb_system.DEFAULT_HTTP_TIMEOUT,
		}
		version_request, version_error := http.NewRequest(http.MethodGet, spttb_system.VERSION_ORIGIN, nil)
		if version_error != nil {
			// TODO: warning
			gui.Append("Unable to compile version request: "+version_error.Error(), spttb_gui.PanelRight)
		} else {
			version_response, version_error := version_client.Do(version_request)
			if version_error != nil {
				// TODO: warning
				gui.Append("Unable to read response from version request: "+version_error.Error(), spttb_gui.PanelRight)
			} else {
				version_response_body, version_error := ioutil.ReadAll(version_response.Body)
				if version_error != nil {
					// TODO: warning
					gui.Append("Unable to get response body: "+version_error.Error(), spttb_gui.PanelRight)
				} else {
					version_data := OnlineVersion{}
					version_error = json.Unmarshal(version_response_body, &version_data)
					if version_error != nil {
						// TODO: warning
						gui.Append("Unable to parse json from response body: "+version_error.Error(), spttb_gui.PanelRight)
					} else {
						version_value := 0
						version_regex, version_error := regexp.Compile("[^0-9]+")
						if version_error != nil {
							// TODO: warning
							gui.Append("Unable to compile regex needed to parse version: "+version_error.Error(), spttb_gui.PanelRight)
						} else {
							version_value, version_error = strconv.Atoi(version_regex.ReplaceAllString(version_data.Name, ""))
							if version_error != nil {
								// TODO: warning
								gui.Append("Unable to fetch latest version value: "+version_error.Error(), spttb_gui.PanelRight)
							} else if version_value != spttb_system.VERSION {
								// TODO: warning
								gui.Append("You're not aligned to the latest available version.\n"+
									"Although you're not forced to update, new updates mean more solid and better performing software.\n"+
									"You can find the updated version at: "+spttb_system.VERSION_URL, spttb_gui.PanelRight)
								gui.Prompt("Press enter to continue or CTRL+C to exit.", spttb_gui.PromptDismissable)
							}
							// TODO: debug
							gui.Append(fmt.Sprintf("Actual version %d, online version %d.", spttb_system.VERSION, version_value), spttb_gui.PanelRight)
						}
					}
				}
			}
		}
	}

	Exit(1 * time.Second)

	if !spotify_client.Auth() {
		gui.Prompt("Unable to authenticate to spotify.", spttb_gui.PromptDismissableWithExit)
	}

	var (
		tracks_online            []api.FullTrack
		tracks_online_albums     []api.FullAlbum
		tracks_online_albums_ids []api.ID
	)
	if *arg_playlist == "none" {
		tracks_online = spotify_client.LibraryTracks()
	} else {
		var playlist_err error
		playlist_info, playlist_err = spotify_client.Playlist(*arg_playlist)
		if playlist_err != nil {
			gui.Prompt("Something went wrong while fetching playlist info.", spttb_gui.PromptDismissableWithExit)
		} else {
			gui.Append(fmt.Sprintf("Getting songs from \"%s\" playlist, by \"%s\"...", playlist_info.Name, playlist_info.Owner.DisplayName), spttb_gui.PanelRight)
			tracks_online = spotify_client.PlaylistTracks(*arg_playlist)
		}
	}
	for _, track := range tracks_online {
		tracks_online_albums_ids = append(tracks_online_albums_ids, track.Album.ID)
	}
	tracks_online_albums = spotify_client.Albums(tracks_online_albums_ids)

	gui.Append("Checking which songs need to be downloaded.", spttb_gui.PanelRight)
	for track_index := len(tracks_online) - 1; track_index >= 0; track_index-- {
		track := spttb_track.ParseSpotifyTrack(tracks_online[track_index], tracks_online_albums[track_index])
		if !tracks.Has(track) {
			tracks = append(tracks, track)
		} else {
			// TODO: warning
			gui.Append(fmt.Sprintf("Ignored song duplicate \"%s\".", track.Filename), spttb_gui.PanelRight)
		}
	}

	signal_channel := make(chan os.Signal, 1)
	signal.Notify(signal_channel, os.Interrupt)
	go func() {
		<-signal_channel
		gui.Append("SIGINT captured: cleaning up temporary files.", spttb_gui.PanelRight)
		for _, track := range tracks {
			for _, track_filename := range track.TempFiles() {
				os.Remove(track_filename)
			}
		}
		gui.Prompt("Explicit closure request by the user. Enter to exit.", spttb_gui.PromptDismissableWithExit)
	}()

	if len(tracks) > 0 {
		youtube_client.SetInteractive(*arg_interactive)
		if *arg_replace_local {
			// logger.Log(strconv.Itoa(len(tracks)) + " missing songs.")
		} else if *arg_flush_metadata {
			// logger.Log(strconv.Itoa(tracks.CountOnline()) + " missing songs, " +
			// "flushing metadata for " + strconv.Itoa(tracks.CountOffline()) + " local ones.")
		} else {
			// logger.Log(strconv.Itoa(tracks.CountOnline()) + " missing songs, " +
			// strconv.Itoa(tracks.CountOffline()) + " ignored.")
		}
		for _, track := range tracks {
			// logger.Log(strconv.Itoa(track_index+1) + "/" + strconv.Itoa(len(tracks)) + ": \"" + track.Filename + "\"")
			if !track.Local || *arg_replace_local || *arg_simulate {
				youtube_track, err := youtube_client.FindTrack(track)
				if err != nil {
					// logger.Warn("Something went wrong while searching for \"" + track.Filename + "\" track: " + err.Error() + ".")
					tracks_failed = append(tracks_failed, track)
					continue
				} else if *arg_simulate {
					// logger.Log("I would like to download \"" + youtube_track.URL + "\" for \"" + track.Filename + "\" track, but I'm just simulating.")
					continue
				} else if *arg_replace_local {
					if track.URL == youtube_track.URL {
						// logger.Log("Track \"" + track.Filename + "\" is still the best result I can find.")
						continue
					} else {
						track.URL = ""
						track.Local = false
						os.Remove(track.FilenameFinal())
					}
				}

				err = youtube_track.Download()
				if err != nil {
					// logger.Warn("Something went wrong downloading \"" + track.Filename + "\": " + err.Error() + ".")
					tracks_failed = append(tracks_failed, track)
					continue
				} else {
					track.URL = youtube_track.URL
				}
			}

			if track.Local && !*arg_flush_metadata && !*arg_replace_local {
				continue
			}

			for true {
				err := spttb_system.SyscallLimit(&wait_group_limit)
				if err == nil && wait_group_limit.Cur < (wait_group_limit.Max-50) {
					break
				}
				// logger.Warn(fmt.Sprintf("%d < %d-10", wait_group_limit.Cur, wait_group_limit.Max))
				time.Sleep(100 * time.Millisecond)
			}

			wait_group.Add(1)
			go ParallelSongProcess(track, &wait_group)
			if *arg_debug {
				wait_group.Wait()
			}
		}
		wait_group.Wait()

		if !*arg_disable_timestamp_flush {
			now := time.Now().Local().Add(time.Duration(-1*len(tracks)) * time.Minute)
			for _, track := range tracks {
				if !spttb_system.FileExists(track.FilenameFinal()) {
					continue
				}
				if err := os.Chtimes(track.FilenameFinal(), now, now); err != nil {
					// logger.Warn("Unable to flush timestamp on " + track.FilenameFinal())
				}
				now = now.Add(1 * time.Minute)
			}
		}

		if !*arg_simulate && !*arg_disable_m3u && *arg_playlist != "none" {
			if spttb_system.FileExists(playlist_info.Name + ".m3u") {
				os.Remove(playlist_info.Name + ".m3u")
			}
			playlist_m3u := "#EXTM3U\n"
			for track_index := len(tracks) - 1; track_index >= 0; track_index-- {
				track := tracks[track_index]
				if spttb_system.FileExists(track.FilenameFinal()) {
					playlist_m3u = playlist_m3u + "#EXTINF:" + strconv.Itoa(track.Duration) + "," + track.Filename + "\n" +
						track.FilenameFinal() + "\n"
				}
			}
			playlist_m3u_file, playlist_err := os.Create(playlist_info.Name + ".m3u")
			if playlist_err != nil {
				// logger.Warn("Unable to create M3U file: " + playlist_err.Error())
			} else {
				defer playlist_m3u_file.Close()
				_, playlist_err := playlist_m3u_file.WriteString(playlist_m3u)
				playlist_m3u_file.Sync()
				if playlist_err != nil {
					// logger.Warn("Unable to write M3U file: " + playlist_err.Error())
				}
			}
		}

		CleanJunks()

		if len(tracks_failed) > 0 {
			// logger.Log("Synchronization partially completed, " + strconv.Itoa(len(tracks_failed)) + " tracks failed to synchronize:")
			// for _, track := range tracks_failed {
			// logger.Log(" - \"" + track.Filename + "\"")
			// }
		} else {
			// logger.Log("Synchronization completed.")
		}
	} else {
		// logger.Log("No song needs to be downloaded.")
	}
	wait_group.Wait()
}

func ParallelSongProcess(track spttb_track.Track, wg *sync.WaitGroup) {
	defer wg.Done()

	if !track.Local && !*arg_disable_normalization {
		var (
			command_cmd         string = "ffmpeg"
			command_args        []string
			command_out         bytes.Buffer
			command_err         error
			normalization_delta string
			normalization_file  string = strings.Replace(track.FilenameTemporary(),
				track.FilenameExt, ".norm"+track.FilenameExt, -1)
		)

		command_args = []string{"-i", track.FilenameTemporary(), "-af", "volumedetect", "-f", "null", "-y", "null"}
		// logger.Debug("Getting max_volume value: \"" + command_cmd + " " + strings.Join(command_args, " ") + "\".")
		command_obj := exec.Command(command_cmd, command_args...)
		command_obj.Stderr = &command_out
		command_err = command_obj.Run()
		if command_err != nil {
			// logger.Warn("Unable to use ffmpeg to pull max_volume song value: " + command_out.String() + ".")
			normalization_delta = "0.0"
		} else {
			command_scanner := bufio.NewScanner(strings.NewReader(command_out.String()))
			for command_scanner.Scan() {
				if strings.Contains(command_scanner.Text(), "max_volume:") {
					normalization_delta = strings.Split(strings.Split(command_scanner.Text(), "max_volume:")[1], " ")[1]
					normalization_delta = strings.Replace(normalization_delta, "-", "", -1)
				}
			}
		}

		if _, command_err = strconv.ParseFloat(normalization_delta, 64); command_err != nil {
			// logger.Warn("Unable to pull max_volume delta to be applied along with song volume normalization: " + normalization_delta + ".")
			normalization_delta = "0.0"
		}
		command_args = []string{"-i", track.FilenameTemporary(), "-af", "volume=+" + normalization_delta + "dB", "-b:a", "320k", "-y", normalization_file}
		// logger.Debug("Going to compensate volume by " + normalization_delta + "dB")
		// logger.Log("Increasing audio quality for: " + track.Filename + ".")
		// logger.Debug("Using command: \"" + command_cmd + " " + strings.Join(command_args, " ") + "\"")
		if _, command_err = exec.Command(command_cmd, command_args...).Output(); command_err != nil {
			// logger.Warn("Something went wrong while normalizing song \"" + track.Filename + "\" volume: " + command_err.Error())
		}
		os.Remove(track.FilenameTemporary())
		os.Rename(normalization_file, track.FilenameTemporary())
	}

	if !spttb_system.FileExists(track.FilenameTemporary()) && spttb_system.FileExists(track.FilenameFinal()) {
		err := spttb_system.FileCopy(track.FilenameFinal(),
			track.FilenameTemporary())
		if err != nil {
			// logger.Warn("Unable to prepare song for getting its metadata flushed: " + err.Error())
			return
		}
	}

	if (track.Local && *arg_flush_metadata) || !track.Local {
		var (
			command_cmd          string   = "ffmpeg"
			command_args         []string = []string{"-i", track.Image, "-q:v", "1", track.FilenameArtworkTemporary()}
			track_artwork_err    error
			track_artwork_reader []byte
		)
		if !spttb_system.FileExists(track.FilenameArtwork()) {
			_, track_artwork_err = exec.Command(command_cmd, command_args...).Output()
			if track_artwork_err != nil {
				// logger.Warn("Unable to download artwork file \"" + track.Image + "\": " + track_artwork_err.Error())
			} else {
				os.Rename(track.FilenameArtworkTemporary(), track.FilenameArtwork())
			}
		} else {
			track_artwork_err = nil
			// logger.Debug("Reusing already download album \"" + track.Album + "\" artwork")
		}
		if track_artwork_err == nil {
			track_artwork_reader, track_artwork_err = ioutil.ReadFile(track.FilenameArtwork())
			if track_artwork_err != nil {
				// logger.Warn("Unable to read artwork file: " + track_artwork_err.Error())
			}
		}

		if !*arg_disable_lyrics {
			err := (&track).SearchLyrics()
			if err != nil {
				// logger.Warn("Something went wrong while searching for song \"" + track.Filename + "\" lyrics: " + err.Error())
			}
		}

		track_mp3, err := id3.Open(track.FilenameTemporary(), id3.Options{Parse: true})
		if track_mp3 == nil || err != nil {
			// logger.Fatal("Something bad happened while opening: " + err.Error())
		} else {
			// logger.Log("Fixing metadata for: " + track.Filename + ".")
			if !*arg_flush_missing {
				track_mp3.DeleteAllFrames()
			}
			if !*arg_flush_missing || track_mp3.Title() == "" {
				track_mp3.SetTitle(track.Title)
			}
			if !*arg_flush_missing || track_mp3.Artist() == "" {
				track_mp3.SetArtist(track.Artist)
			}
			if !*arg_flush_missing || track_mp3.Album() == "" {
				track_mp3.SetAlbum(track.Album)
			}
			if !*arg_flush_missing || track_mp3.Genre() == "" {
				track_mp3.SetGenre(track.Genre)
			}
			if !*arg_flush_missing || track_mp3.Year() == "" {
				track_mp3.SetYear(track.Year)
			}
			if !*arg_flush_missing ||
				len(track_mp3.GetFrames(track_mp3.CommonID("Track number/Position in set"))) == 0 {
				track_mp3.AddFrame(track_mp3.CommonID("Track number/Position in set"),
					id3.TextFrame{
						Encoding: id3.EncodingUTF8,
						Text:     strconv.Itoa(track.TrackNumber),
					})
			}
			if track_artwork_err == nil {
				if !*arg_flush_missing ||
					len(track_mp3.GetFrames(track_mp3.CommonID("Attached picture"))) == 0 {
					// logger.Debug("Inflating artwork metadata...")
					track_mp3.AddAttachedPicture(id3.PictureFrame{
						Encoding:    id3.EncodingUTF8,
						MimeType:    "image/jpeg",
						PictureType: id3.PTFrontCover,
						Description: "Front cover",
						Picture:     track_artwork_reader,
					})
				}
			}
			if len(track.URL) > 0 {
				if !*arg_flush_missing ||
					len(track_mp3.GetFrames(track_mp3.CommonID("Comments"))) == 0 {
					// logger.Debug("Inflating youtube origin url metadata...")
					track_mp3.AddCommentFrame(id3.CommentFrame{
						Encoding:    id3.EncodingUTF8,
						Language:    "eng",
						Description: "youtube",
						Text:        track.URL,
					})
				}
			}
			if len(track.Lyrics) > 0 {
				if !*arg_flush_missing ||
					len(track_mp3.GetFrames(track_mp3.CommonID("Unsynchronised lyrics/text transcription"))) == 0 {
					// logger.Debug("Inflating lyrics metadata...")
					track_mp3.AddUnsynchronisedLyricsFrame(id3.UnsynchronisedLyricsFrame{
						Encoding:          id3.EncodingUTF8,
						Language:          "eng",
						ContentDescriptor: track.Title,
						Lyrics:            track.Lyrics,
					})
				}
			}
			track_mp3.Save()
		}
		track_mp3.Close()
	}

	os.Remove(track.FilenameFinal())
	err := os.Rename(track.FilenameTemporary(), track.FilenameFinal())
	if err != nil {
		// logger.Warn("Unable to move song to its final path: " + err.Error())
	}
}

func CleanJunks() int {
	// logger.Log("Cleaning up junks")
	var removed_junks int
	for _, junk_type := range spttb_track.JunkWildcards {
		junk_paths, err := filepath.Glob(junk_type)
		if err != nil {
			continue
		}
		for _, junk_path := range junk_paths {
			// TODO: debug
			// logger.Debug("Removing " + junk_path + "...")
			os.Remove(junk_path)
			removed_junks++
		}
	}
	return removed_junks
}

func Exit(delay ...time.Duration) {
	if len(delay) > 0 {
		time.Sleep(delay[0])
	}
	CleanJunks()
	gui.Close()
	os.Exit(0)
}
