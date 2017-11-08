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

	id3 "github.com/bogem/id3v2"
	api "github.com/zmb3/spotify"
	. "spotitube"
)

var (
	arg_folder                  *string
	arg_playlist                *string
	arg_replace_local           *bool
	arg_flush_metadata          *bool
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

	tracks           Tracks
	tracks_failed    Tracks
	playlist_info    *api.FullPlaylist
	youtube_client   *YouTube = NewYouTubeClient()
	spotify_client   *Spotify = NewSpotifyClient()
	logger           *Logger  = NewLogger()
	wait_group       sync.WaitGroup
	wait_group_limit syscall.Rlimit
)

func main() {
	arg_folder = flag.String("folder", ".", "Folder to sync with music.")
	arg_playlist = flag.String("playlist", "none", "Playlist URI to synchronize.")
	arg_replace_local = flag.Bool("replace-local", false, "Replace local library songs if better results get encountered")
	arg_flush_metadata = flag.Bool("flush-metadata", false, "Flush metadata informations to already synchronized songs")
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
		fmt.Println(fmt.Sprintf("SpotiTube, version %d.", VERSION))
		os.Exit(0)
	}

	for _, command_name := range []string{"youtube-dl", "ffmpeg"} {
		_, err := exec.LookPath(command_name)
		if err != nil {
			logger.Fatal("Are you sure " + command_name + " is actually installed?")
		}
	}

	_, err := net.Dial("tcp", DEFAULT_TCP_CHECK)
	if err != nil {
		logger.Fatal("Are you sure you're connected to the internet?")
	}

	if *arg_log {
		logger.SetFile(DEFAULT_LOG_PATH)
	}

	if *arg_debug {
		logger.EnableDebug()
	}

	if !*arg_disable_update_check {
		type OnlineVersion struct {
			Name string `json:"name"`
		}
		version_client := http.Client{
			Timeout: time.Second * 2,
		}
		version_request, version_error := http.NewRequest(http.MethodGet, VERSION_ORIGIN, nil)
		if version_error != nil {
			logger.Warn("Unable to compile version request: " + version_error.Error())
		}
		version_response, version_error := version_client.Do(version_request)
		if version_error != nil {
			logger.Warn("Unable to read response from version request: " + version_error.Error())
		}
		version_response_body, version_error := ioutil.ReadAll(version_response.Body)
		if version_error != nil {
			logger.Warn("Unable to get response body: " + version_error.Error())
		}
		version_data := OnlineVersion{}
		version_error = json.Unmarshal(version_response_body, &version_data)
		if version_error != nil {
			logger.Warn("Unable to parse json from response body: " + version_error.Error())
		}
		version_value := 0
		version_regex, version_error := regexp.Compile("[^0-9]+")
		if version_error != nil {
			logger.Warn("Unable to compile regex needed to parse version: " + version_error.Error())
		}
		version_value, version_error = strconv.Atoi(version_regex.ReplaceAllString(version_data.Name, ""))
		if version_error != nil {
			logger.Warn("Unable to fetch latest version value: " + version_error.Error())
		} else if version_value != VERSION {
			logger.Warn("You're not aligned to the latest available version.")
		}
		logger.Debug(fmt.Sprintf("Actual version %d, online version %d.", VERSION, version_value))
	}

	if !(IsDir(*arg_folder)) {
		logger.Fatal("Chosen music folder does not exist: " + *arg_folder)
	} else {
		os.Chdir(*arg_folder)
		logger.Log("Synchronization folder: " + *arg_folder)
	}

	if *arg_clean_junks {
		CleanJunks()
		return
	}

	if !spotify_client.Auth() {
		logger.Fatal("Unable to authenticate to spotify.")
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
			logger.Fatal("Something went wrong while fetching playlist info.")
		} else {
			logger.Log("Getting songs from \"" + playlist_info.Name + "\" playlist, by \"" + playlist_info.Owner.DisplayName + "\"")
			tracks_online = spotify_client.PlaylistTracks(*arg_playlist)
		}
	}
	for _, track := range tracks_online {
		tracks_online_albums_ids = append(tracks_online_albums_ids, track.Album.ID)
	}
	tracks_online_albums = spotify_client.Albums(tracks_online_albums_ids)

	logger.Log("Checking which songs need to be downloaded.")
	for track_index := len(tracks_online) - 1; track_index >= 0; track_index-- {
		track := ParseSpotifyTrack(tracks_online[track_index], tracks_online_albums[track_index])
		if !tracks.Has(track) {
			tracks = append(tracks, track)
		} else {
			logger.Warn("Ignored song duplicate \"" + track.Filename + "\".")
		}
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		logger.Log("SIGINT captured: cleaning up temporary files.")
		for _, track := range tracks {
			for _, track_filename := range track.TempFiles() {
				os.Remove(track_filename)
			}
		}
		logger.Fatal("Explicit closure request by the user. Exiting.")
	}()

	if len(tracks) > 0 {
		youtube_client.SetInteractive(*arg_interactive)
		if *arg_replace_local {
			logger.Log(strconv.Itoa(len(tracks)) + " missing songs.")
		} else if *arg_flush_metadata {
			logger.Log(strconv.Itoa(tracks.CountOnline()) + " missing songs, " +
				"flushing metadata for " + strconv.Itoa(tracks.CountOffline()) + " local ones.")
		} else {
			logger.Log(strconv.Itoa(tracks.CountOnline()) + " missing songs, " +
				strconv.Itoa(tracks.CountOffline()) + " ignored.")
		}
		for track_index, track := range tracks {
			logger.Log(strconv.Itoa(track_index+1) + "/" + strconv.Itoa(len(tracks)) + ": \"" + track.Filename + "\"")
			if !track.Local || *arg_replace_local || *arg_simulate {
				youtube_track, err := youtube_client.FindTrack(track)
				if err != nil {
					logger.Warn("Something went wrong while searching for \"" + track.Filename + "\" track: " + err.Error() + ".")
					tracks_failed = append(tracks_failed, track)
					continue
				} else if *arg_simulate {
					logger.Log("I would like to download \"" + youtube_track.URL + "\" for \"" + track.Filename + "\" track, but I'm just simulating.")
					continue
				} else if *arg_replace_local {
					if track.URL == youtube_track.URL {
						logger.Log("Track \"" + track.Filename + "\" is still the best result I can find.")
						continue
					} else {
						track.URL = ""
						track.Local = false
						os.Remove(track.FilenameFinal())
					}
				}

				err = youtube_track.Download()
				if err != nil {
					logger.Warn("Something went wrong downloading \"" + track.Filename + "\": " + err.Error() + ".")
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
				err := SyscallLimit(&wait_group_limit)
				if err == nil && wait_group_limit.Cur < (wait_group_limit.Max-50) {
					break
				}
				logger.Warn(fmt.Sprintf("%d < %d-10", wait_group_limit.Cur, wait_group_limit.Max))
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
			for _, track := range tracks {
				if !FileExists(track.FilenameFinal()) {
					continue
				}
				now := time.Now().Local()
				if err := os.Chtimes(track.FilenameFinal(), now, now); err != nil {
					logger.Warn("Unable to flush timestamp on " + track.FilenameFinal())
				}
			}
		}

		if !*arg_simulate && !*arg_disable_m3u && *arg_playlist != "none" {
			if FileExists(playlist_info.Name + ".m3u") {
				os.Remove(playlist_info.Name + ".m3u")
			}
			playlist_m3u := "#EXTM3U\n"
			for track_index := len(tracks) - 1; track_index >= 0; track_index-- {
				track := tracks[track_index]
				if FileExists(track.FilenameFinal()) {
					playlist_m3u = playlist_m3u + "#EXTINF:" + strconv.Itoa(track.Duration) + "," + track.Filename + "\n" +
						track.FilenameFinal() + "\n"
				}
			}
			playlist_m3u_file, playlist_err := os.Create(playlist_info.Name + ".m3u")
			if playlist_err != nil {
				logger.Warn("Unable to create M3U file: " + playlist_err.Error())
			} else {
				defer playlist_m3u_file.Close()
				_, playlist_err := playlist_m3u_file.WriteString(playlist_m3u)
				playlist_m3u_file.Sync()
				if playlist_err != nil {
					logger.Warn("Unable to write M3U file: " + playlist_err.Error())
				}
			}
		}

		CleanJunks()

		if len(tracks_failed) > 0 {
			logger.Log("Synchronization partially completed, " + strconv.Itoa(len(tracks_failed)) + " tracks failed to synchronize:")
			for _, track := range tracks_failed {
				logger.Log(" - \"" + track.Filename + "\"")
			}
		} else {
			logger.Log("Synchronization completed.")
		}
	} else {
		logger.Log("No song needs to be downloaded.")
	}
	wait_group.Wait()
}

func ParallelSongProcess(track Track, wg *sync.WaitGroup) {
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
		logger.Debug("Getting max_volume value: \"" + command_cmd + " " + strings.Join(command_args, " ") + "\".")
		command_obj := exec.Command(command_cmd, command_args...)
		command_obj.Stderr = &command_out
		command_err = command_obj.Run()
		if command_err != nil {
			logger.Warn("Unable to use ffmpeg to pull max_volume song value: " + command_out.String() + ".")
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
			logger.Warn("Unable to pull max_volume delta to be applied along with song volume normalization: " + normalization_delta + ".")
			normalization_delta = "0.0"
		}
		command_args = []string{"-i", track.FilenameTemporary(), "-af", "volume=+" + normalization_delta + "dB", "-b:a", "320k", "-y", normalization_file}
		logger.Debug("Going to compensate volume by " + normalization_delta + "dB")
		logger.Log("Increasing audio quality for: " + track.Filename + ".")
		logger.Debug("Using command: \"" + command_cmd + " " + strings.Join(command_args, " ") + "\"")
		if _, command_err = exec.Command(command_cmd, command_args...).Output(); command_err != nil {
			logger.Warn("Something went wrong while normalizing song \"" + track.Filename + "\" volume: " + command_err.Error())
		}
		os.Remove(track.FilenameTemporary())
		os.Rename(normalization_file, track.FilenameTemporary())
	}

	if !FileExists(track.FilenameTemporary()) && FileExists(track.FilenameFinal()) {
		err := os.Rename(track.FilenameFinal(),
			track.FilenameTemporary())
		if err != nil {
			logger.Warn("Unable to prepare song for getting its metadata flushed: " + err.Error())
			return
		}
	}

	if (track.Local && *arg_flush_metadata) || !track.Local {
		var (
			command_cmd          string   = "ffmpeg"
			command_args         []string = []string{"-i", track.Image, "-q:v", "1", track.FilenameArtwork()}
			track_artwork_err    error
			track_artwork_reader []byte
		)
		if !FileExists(track.FilenameArtwork()) {
			_, track_artwork_err = exec.Command(command_cmd, command_args...).Output()
			if track_artwork_err != nil {
				logger.Warn("Unable to download artwork file \"" + track.Image + "\": " + track_artwork_err.Error())
			}
		} else {
			track_artwork_err = nil
			logger.Debug("Reusing already download album \"" + track.Album + "\" artwork")
		}
		if track_artwork_err == nil {
			track_artwork_reader, track_artwork_err = ioutil.ReadFile(track.FilenameArtwork())
			if track_artwork_err != nil {
				logger.Warn("Unable to read artwork file: " + track_artwork_err.Error())
			}
		}

		if !*arg_disable_lyrics {
			err := (&track).SearchLyrics()
			if err != nil {
				logger.Warn("Something went wrong while searching for song \"" + track.Filename + "\" lyrics: " + err.Error())
			}
		}

		track_mp3, err := id3.Open(track.FilenameTemporary(), id3.Options{Parse: true})
		if track_mp3 == nil || err != nil {
			logger.Fatal("Something bad happened while opening: " + err.Error())
		} else {
			logger.Log("Fixing metadata for: " + track.Filename + ".")
			track_mp3.DeleteAllFrames()
			track_mp3.SetTitle(track.Title)
			track_mp3.SetArtist(track.Artist)
			track_mp3.SetAlbum(track.Album)
			track_mp3.SetGenre(track.Genre)
			track_mp3.AddFrame(track_mp3.CommonID("Track number/Position in set"),
				id3.TextFrame{Encoding: id3.EncodingUTF8, Text: strconv.Itoa(track.TrackNumber)})
			track_mp3.SetYear(track.Year)
			if track_artwork_err == nil {
				logger.Debug("Inflating artwork metadata...")
				track_mp3.AddAttachedPicture(id3.PictureFrame{
					Encoding:    id3.EncodingUTF8,
					MimeType:    "image/jpeg",
					PictureType: id3.PTFrontCover,
					Description: "Front cover",
					Picture:     track_artwork_reader,
				})
			}
			if len(track.URL) > 0 {
				logger.Debug("Inflating youtube origin url metadata...")
				track_mp3.AddCommentFrame(id3.CommentFrame{
					Encoding:    id3.EncodingUTF8,
					Language:    "eng",
					Description: "youtube",
					Text:        track.URL,
				})
			}
			if len(track.Lyrics) > 0 {
				logger.Debug("Inflating lyrics metadata...")
				track_mp3.AddUnsynchronisedLyricsFrame(id3.UnsynchronisedLyricsFrame{
					Encoding:          id3.EncodingUTF8,
					Language:          "eng",
					ContentDescriptor: track.Title,
					Lyrics:            track.Lyrics,
				})
			}
			track_mp3.Save()
		}
		track_mp3.Close()
	}

	err := os.Rename(track.FilenameTemporary(), track.FilenameFinal())
	if err != nil {
		logger.Warn("Unable to move song to its final path: " + err.Error())
	}
}

func CleanJunks() {
	logger.Log("Cleaning up junks")
	var removed_junks int
	for _, junk_type := range JunkWildcards {
		junk_paths, err := filepath.Glob(junk_type)
		if err != nil {
			logger.Warn("Something wrong while searching for \"" + junk_type + "\" junk files: " + err.Error())
			continue
		}
		for _, junk_path := range junk_paths {
			logger.Debug("Removing " + junk_path + "...")
			os.Remove(junk_path)
			removed_junks++
		}
	}
	logger.Log("Removed " + strconv.Itoa(removed_junks) + " files.")
}
