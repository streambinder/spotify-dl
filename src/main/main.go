package main

import (
	"bufio"
	"bytes"
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	arg_folder                *string
	arg_playlist              *string
	arg_replace_local         *bool
	arg_flush_metadata        *bool
	arg_disable_normalization *bool
	arg_interactive           *bool
	arg_clean_junks           *bool
	arg_log                   *bool
	arg_debug                 *bool
	arg_simulate              *bool

	tracks           Tracks
	tracks_failed    Tracks
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
	arg_interactive = flag.Bool("interactive", false, "Enable interactive mode")
	arg_clean_junks = flag.Bool("clean-junks", false, "Scan for junks file and clean them")
	arg_log = flag.Bool("log", false, "Enable logging into file ./spotitube.log")
	arg_debug = flag.Bool("debug", false, "Enable debug messages")
	arg_simulate = flag.Bool("simulate", false, "Simulate process flow, without really altering filesystem")
	flag.Parse()

	if *arg_log {
		logger.SetFile(DEFAULT_LOG_PATH)
	}

	if *arg_debug {
		logger.EnableDebug()
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
		tracks_online = spotify_client.Library()
	} else {
		tracks_online = spotify_client.Playlist(*arg_playlist)
	}
	for _, track := range tracks_online {
		tracks_online_albums_ids = append(tracks_online_albums_ids, track.Album.ID)
	}
	tracks_online_albums = spotify_client.Albums(tracks_online_albums_ids)

	logger.Log("Checking which songs need to be downloaded.")
	for track_index := len(tracks_online) - 1; track_index >= 0; track_index-- {
		tracks = append(tracks, ParseSpotifyTrack(tracks_online[track_index], tracks_online_albums[track_index]))
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
		} else {
			logger.Log(strconv.Itoa(tracks.CountOnline()) + " missing songs, " + strconv.Itoa(tracks.CountOffline()) + " ignored.")
		}
		for track_index, track := range tracks {
			logger.Log(strconv.Itoa(track_index+1) + "/" + strconv.Itoa(len(tracks)) + ": \"" + track.Filename + "\"")
			if !track.Local || *arg_replace_local {
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
			} else if track.Local && *arg_flush_metadata {
				os.Rename(track.FilenameFinal(),
					track.FilenameTemporary())
			}

			for true {
				err := SyscallLimit(&wait_group_limit)
				if err == nil && wait_group_limit.Cur < (wait_group_limit.Max-10) {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			wait_group.Add(1)
			go ParallelSongProcess(track, &wait_group)
			if *arg_debug {
				wait_group.Wait()
			}
		}
		wait_group.Wait()

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

	if (track.Local && *arg_flush_metadata) || !track.Local {
		if FileExists(track.FilenameArtwork()) {
			os.Remove(track.FilenameArtwork())
		}
		command_cmd := "ffmpeg"
		command_args := []string{"-i", track.Image, "-q:v", "1", track.FilenameArtwork()}
		_, track_artwork_err := exec.Command(command_cmd, command_args...).Output()
		if track_artwork_err != nil {
			logger.Warn("Unable to download artwork file:" + track_artwork_err.Error())
		}
		track_artwork_reader, track_artwork_err := ioutil.ReadFile(track.FilenameArtwork())
		if track_artwork_err != nil {
			logger.Warn("Unable to read artwork file: " + track_artwork_err.Error())
		}
		defer os.Remove(track.FilenameArtwork())

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
				track_mp3.AddAttachedPicture(id3.PictureFrame{
					Encoding:    id3.EncodingUTF8,
					MimeType:    "image/jpeg",
					PictureType: id3.PTFrontCover,
					Description: "Front cover",
					Picture:     track_artwork_reader,
				})
			}
			if len(track.URL) > 0 {
				track_mp3.AddCommentFrame(id3.CommentFrame{
					Encoding:    id3.EncodingUTF8,
					Language:    "eng",
					Description: "youtube",
					Text:        track.URL,
				})
			}
			track_mp3.Save()
		}
		track_mp3.Close()
	}

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
			normalization_delta = "0"
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
			normalization_delta = "0"
		}
		command_args = []string{"-i", track.FilenameTemporary(), "-af", "volume=+" + normalization_delta + "dB", "-b:a", "320k", "-y", normalization_file}
		logger.Log("Normalizing volume by " + normalization_delta + "dB for: " + track.Filename + ".")
		logger.Debug("Using command: \"" + command_cmd + " " + strings.Join(command_args, " ") + "\"")
		if _, command_err = exec.Command(command_cmd, command_args...).Output(); command_err != nil {
			logger.Warn("Something went wrong while normalizing song \"" + track.Filename + "\" volume: " + command_err.Error())
		}
		os.Remove(track.FilenameTemporary())
		os.Rename(normalization_file, track.FilenameTemporary())
	}
	os.Rename(track.FilenameTemporary(), track.FilenameFinal())
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
	return
}
