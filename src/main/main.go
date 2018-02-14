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
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	spttb_gui "gui"
	spttb_logger "logger"
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

	tracks          spttb_track.Tracks
	tracks_failed   spttb_track.Tracks
	playlist_info   *api.FullPlaylist
	spotify_client  *spttb_spotify.Spotify = spttb_spotify.NewClient()
	wait_group      sync.WaitGroup
	wait_group_pool chan bool = make(chan bool, spttb_system.CONCURRENCY_LIMIT)

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

	if !(spttb_system.Dir(*arg_folder)) {
		fmt.Println(fmt.Sprintf("Chosen music folder does not exist: %s", *arg_folder))
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

	gui = spttb_gui.Build(*arg_debug)
	if user, err := user.Current(); err == nil {
		*arg_folder = strings.Replace(*arg_folder, user.HomeDir, "~", -1)
	}
	gui.Append(fmt.Sprintf("%s %s", spttb_gui.MessageStyle("Folder:", spttb_gui.FontStyleBold), *arg_folder), spttb_gui.PanelLeftTop)
	if *arg_log {
		gui.Append(fmt.Sprintf("%s %s", spttb_gui.MessageStyle("Log:", spttb_gui.FontStyleBold), spttb_system.DEFAULT_LOG_PATH), spttb_gui.PanelLeftTop)
	}
	gui.Append(fmt.Sprintf("%s %d", spttb_gui.MessageStyle("Version:", spttb_gui.FontStyleBold), spttb_system.VERSION), spttb_gui.PanelLeftBottom)
	gui.Append(fmt.Sprintf("%s %s", spttb_gui.MessageStyle("Date:", spttb_gui.FontStyleBold), time.Now().Format("2006-01-02 15:04:05")), spttb_gui.PanelLeftBottom)
	gui.Append(fmt.Sprintf("%s %s", spttb_gui.MessageStyle("URL:", spttb_gui.FontStyleBold), spttb_system.VERSION_REPOSITORY), spttb_gui.PanelLeftBottom)
	gui.Append(fmt.Sprintf("%s GPLv2", spttb_gui.MessageStyle("License:", spttb_gui.FontStyleBold)), spttb_gui.PanelLeftBottom)

	for _, command_name := range []string{"youtube-dl", "ffmpeg"} {
		_, err := exec.LookPath(command_name)
		if err != nil {
			gui.Prompt(fmt.Sprintf("Are you sure %s is asctually installed?", command_name), spttb_gui.PromptDismissableWithExit)
		} else {
			var (
				command_out           bytes.Buffer
				command_version_value string = "?"
				command_version_regex string = "\\d+\\.\\d+\\.\\d+"
			)
			if version_regex, version_regex_err := regexp.Compile(command_version_regex); version_regex_err != nil {
				command_version_value = "Regex compile failure"
			} else {
				command_obj := exec.Command(command_name, []string{"--version"}...)
				fmt.Println(command_out.String())
				command_obj.Stdout = &command_out
				command_obj.Stderr = &command_out
				_ = command_obj.Run()
				if command_version_regvalue := version_regex.FindString(command_out.String()); len(command_version_regvalue) > 0 {
					command_version_value = command_version_regvalue
				}
			}
			gui.Append(fmt.Sprintf("%s %s", spttb_gui.MessageStyle(fmt.Sprintf("Version %s:", command_name), spttb_gui.FontStyleBold), command_version_value), spttb_gui.PanelLeftTop)
		}
	}

	_, err := net.Dial("tcp", spttb_system.DEFAULT_TCP_CHECK)
	if err != nil {
		gui.Prompt("Are you sure you're connected to the internet?", spttb_gui.PromptDismissableWithExit)
	}

	if *arg_log {
		err := gui.LinkLogger(spttb_logger.Build(spttb_system.DEFAULT_LOG_PATH))
		if err != nil {
			gui.Prompt(fmt.Sprintf("Something went wrong while linking logger to %s", spttb_system.DEFAULT_LOG_PATH), spttb_gui.PromptDismissableWithExit)
		}
	}

	if !*arg_disable_update_check {
		type OnlineVersion struct {
			Name string `json:"name"`
		}
		version_client := http.Client{
			Timeout: time.Second * spttb_system.DEFAULT_HTTP_TIMEOUT,
		}
		version_request, version_error := http.NewRequest(http.MethodGet, spttb_system.VERSION_ORIGIN, nil)
		if version_error != nil {
			gui.WarnAppend(fmt.Sprintf("Unable to compile version request: %s", version_error.Error()), spttb_gui.PanelRight)
		} else {
			version_response, version_error := version_client.Do(version_request)
			if version_error != nil {
				gui.WarnAppend(fmt.Sprintf("Unable to read response from version request: %s", version_error.Error()), spttb_gui.PanelRight)
			} else {
				version_response_body, version_error := ioutil.ReadAll(version_response.Body)
				if version_error != nil {
					gui.WarnAppend(fmt.Sprintf("Unable to get response body: %s", version_error.Error()), spttb_gui.PanelRight)
				} else {
					version_data := OnlineVersion{}
					version_error = json.Unmarshal(version_response_body, &version_data)
					if version_error != nil {
						gui.WarnAppend(fmt.Sprintf("Unable to parse json from response body: %s", version_error.Error()), spttb_gui.PanelRight)
					} else {
						version_value := 0
						version_regex, version_error := regexp.Compile("[^0-9]+")
						if version_error != nil {
							gui.WarnAppend(fmt.Sprintf("Unable to compile regex needed to parse version: %s", version_error.Error()), spttb_gui.PanelRight)
						} else {
							version_value, version_error = strconv.Atoi(version_regex.ReplaceAllString(version_data.Name, ""))
							if version_error != nil {
								gui.WarnAppend(fmt.Sprintf("Unable to fetch latest version value: %s", version_error.Error()), spttb_gui.PanelRight)
							} else if version_value != spttb_system.VERSION {
								gui.WarnAppend(fmt.Sprintf("You're not aligned to the latest available version.\n"+
									"Although you're not forced to update, new updates mean more solid and better performing software.\n"+
									"You can find the updated version at: %s", spttb_system.VERSION_URL), spttb_gui.PanelRight)
								gui.Prompt("Press enter to continue or CTRL+C to exit.", spttb_gui.PromptDismissable)
							}
							gui.DebugAppend(fmt.Sprintf("Actual version %d, online version %d.", spttb_system.VERSION, version_value), spttb_gui.PanelRight)
						}
					}
				}
			}
		}
	}

	go func() {
		<-gui.Closing
		fmt.Println("Signal captured: cleaning up temporary files...")
		junks := CleanJunks()
		fmt.Println(fmt.Sprintf("Cleaned up %d files. Exiting.", junks))
		Exit(1 * time.Second)
	}()

	spotify_auth_url := spttb_spotify.AuthUrl()
	gui.Append(fmt.Sprintf("Authentication URL: %s", spotify_auth_url), spttb_gui.PanelRight|spttb_gui.ParagraphStyleAutoReturn)
	gui.DebugAppend("Waiting for automatic login process. If wait is too long, manually open that URL.", spttb_gui.PanelRight)
	if !spotify_client.Auth(spotify_auth_url) {
		gui.Prompt("Unable to authenticate to spotify.", spttb_gui.PromptDismissableWithExit)
	}
	gui.Append("Authentication completed.", spttb_gui.PanelRight)

	var (
		tracks_online            []api.FullTrack
		tracks_online_albums     []api.FullAlbum
		tracks_online_albums_ids []api.ID
		tracks_err               error
	)
	if *arg_playlist == "none" {
		gui.Append("Fetching music library...", spttb_gui.PanelRight)
		if tracks_online, tracks_err = spotify_client.LibraryTracks(); tracks_err != nil {
			gui.Prompt(fmt.Sprintf("Something went wrong while fetching playlist: %s.", tracks_err.Error()), spttb_gui.PromptDismissableWithExit)
		}
	} else {
		gui.Append("Fetching playlist...", spttb_gui.PanelRight)
		var playlist_err error
		playlist_info, playlist_err = spotify_client.Playlist(*arg_playlist)
		if playlist_err != nil {
			gui.Prompt("Something went wrong while fetching playlist info.", spttb_gui.PromptDismissableWithExit)
		} else {
			gui.Append(fmt.Sprintf("%s %s", spttb_gui.MessageStyle("Playlist name:", spttb_gui.FontStyleBold), playlist_info.Name), spttb_gui.PanelLeftTop)
			gui.Append(fmt.Sprintf("%s %s", spttb_gui.MessageStyle("Playlist owner:", spttb_gui.FontStyleBold), playlist_info.Owner.DisplayName), spttb_gui.PanelLeftTop)
			gui.Append(fmt.Sprintf("Getting songs from \"%s\" playlist, by \"%s\"...", playlist_info.Name, playlist_info.Owner.DisplayName), spttb_gui.PanelRight)
			if tracks_online, tracks_err = spotify_client.PlaylistTracks(*arg_playlist); tracks_err != nil {
				gui.Prompt(fmt.Sprintf("Something went wrong while fetching playlist: %s.", tracks_err.Error()), spttb_gui.PromptDismissableWithExit)
			}
		}
	}
	for _, track := range tracks_online {
		tracks_online_albums_ids = append(tracks_online_albums_ids, track.Album.ID)
	}
	if tracks_online_albums, tracks_err = spotify_client.Albums(tracks_online_albums_ids); tracks_err != nil {
		gui.Prompt(fmt.Sprintf("Something went wrong while fetching album info: %s.", tracks_err.Error()), spttb_gui.PromptDismissableWithExit)
	}

	gui.Append("Checking which songs need to be downloaded...", spttb_gui.PanelRight)
	var tracks_duplicates = 0
	for track_index := len(tracks_online) - 1; track_index >= 0; track_index-- {
		track := spttb_track.ParseSpotifyTrack(tracks_online[track_index], tracks_online_albums[track_index])
		if !tracks.Has(track) {
			tracks = append(tracks, track)
		} else {
			gui.WarnAppend(fmt.Sprintf("Ignored song duplicate \"%s\".", track.Filename), spttb_gui.PanelRight)
			tracks_duplicates++
		}
	}

	gui.Append(fmt.Sprintf("%s %d", spttb_gui.MessageStyle("Songs online:", spttb_gui.FontStyleBold), len(tracks)), spttb_gui.PanelLeftTop)
	gui.Append(fmt.Sprintf("%s %d", spttb_gui.MessageStyle("Songs offline:", spttb_gui.FontStyleBold), tracks.CountOffline()), spttb_gui.PanelLeftTop)
	gui.Append(fmt.Sprintf("%s %d", spttb_gui.MessageStyle("Songs missing:", spttb_gui.FontStyleBold), tracks.CountOnline()), spttb_gui.PanelLeftTop)
	gui.Append(fmt.Sprintf("%s %d", spttb_gui.MessageStyle("Songs duplicates:", spttb_gui.FontStyleBold), tracks_duplicates), spttb_gui.PanelLeftTop)

	if len(tracks) > 0 {
		for range [spttb_system.CONCURRENCY_LIMIT]int{} {
			wait_group_pool <- true
		}

		if *arg_replace_local {
			gui.Append(fmt.Sprintf("%d missing songs.", tracks.CountOnline()), spttb_gui.PanelRight)
		} else if *arg_flush_metadata {
			gui.Append(fmt.Sprintf("%d missing songs, %d will gets metadata flushed.", tracks.CountOnline(), tracks.CountOffline()), spttb_gui.PanelRight)
		} else {
			gui.Append(fmt.Sprintf("%d missing songs, %d ignored.", tracks.CountOnline(), tracks.CountOffline()), spttb_gui.PanelRight)
		}
		for track_index, track := range tracks {
			gui.Append(fmt.Sprintf("%d/%d: \"%s\"", track_index+1, len(tracks), track.Filename), spttb_gui.PanelRight)
			if !track.Local || *arg_replace_local || *arg_simulate {
				youtube_tracks, err := spttb_youtube.QueryTracks(&track)
				if err != nil {
					gui.WarnAppend(fmt.Sprintf("Something went wrong while searching for \"%s\" track:\n%s.", track.Filename, err.Error()), spttb_gui.PanelRight)
					tracks_failed = append(tracks_failed, track)
					continue
				}

				var youtube_track *spttb_youtube.YouTubeTrack
				var track_picked bool = false
				for youtube_tracks.HasNext() {
					var (
						ans_input     bool = false
						ans_automated bool = false
						ans_err       error
					)
					if youtube_track, err = youtube_tracks.Next(); err != nil {
						gui.DebugAppend(fmt.Sprintf("Faulty result: %s.", err.Error()), spttb_gui.PanelRight)
						continue
					}

					gui.DebugAppend(fmt.Sprintf("Result met: ID: %s,\nTitle: %s,\nUser: %s,\nDuration: %d.",
						youtube_track.ID, youtube_track.Title, youtube_track.User, youtube_track.Duration), spttb_gui.PanelRight)

					ans_err = youtube_track.Match(track)
					ans_automated = bool(ans_err == nil)
					if !*arg_interactive && ans_err != nil {
						gui.DebugAppend(fmt.Sprintf("\"%s\" seems not the one we're looking for: %s", youtube_track.Title, ans_err.Error()), spttb_gui.PanelRight)
					} else if *arg_interactive {
						var ans_automated_msg string
						if ans_automated {
							ans_automated_msg = "I would download it."
						} else {
							ans_automated_msg = "I wouldn't download it."
						}
						ans_input = gui.PromptInput(fmt.Sprintf("Do you want to download the following video for \"%s\"?\n"+
							"ID: %s\nTitle: %s\nUser: %s\nDuration: %d\nURL: %s\n\n%s",
							track.Filename, youtube_track.ID, youtube_track.Title, youtube_track.User,
							youtube_track.Duration, youtube_track.URL, ans_automated_msg), spttb_gui.OptionNil)
					}
					if (*arg_interactive && ans_input) || (!*arg_interactive && ans_automated) {
						gui.Append(fmt.Sprintf("Video \"%s\" is good to go for \"%s\".", youtube_track.Title, track.Filename), spttb_gui.PanelRight)
						track_picked = true
						break
					}
				}

				if *arg_interactive && !track_picked {
					input_url := gui.PromptInputMessage("Video not found. Please, enter URL manually", spttb_gui.PromptDismissable)
					if len(input_url) > 0 {
						if err := spttb_youtube.ValidateURL(input_url); err != nil {
							gui.Prompt(fmt.Sprintf("Something went wrong: %s", err.Error()), spttb_gui.PromptDismissable)
						} else {
							track_picked = true
							youtube_track = &spttb_youtube.YouTubeTrack{Track: &track, Title: "input video", URL: input_url}
						}
					}
				}

				if track_picked {
					track.URL = youtube_track.URL
				} else {
					gui.ErrAppend(fmt.Sprintf("Video for \"%s\" not found.", track.Filename), spttb_gui.PanelRight)
					tracks_failed = append(tracks_failed, track)
					continue
				}

				if *arg_simulate {
					gui.Append(fmt.Sprintf("I would like to download \"%s\" for \"%s\" track, but I'm just simulating.", youtube_track.URL, track.Filename), spttb_gui.PanelRight)
					continue
				} else if *arg_replace_local {
					if track.URL == youtube_track.URL {
						gui.Append(fmt.Sprintf("Track \"%s\" is still the best result I can find.", track.Filename), spttb_gui.PanelRight)
						continue
					} else {
						track.URL = ""
						track.Local = false
						os.Remove(track.FilenameFinal())
					}
				}

				gui.Append(fmt.Sprintf("Going to download \"%s\" from %s...", youtube_track.Title, youtube_track.URL), spttb_gui.PanelRight)
				err = youtube_track.Download()
				if err != nil {
					gui.WarnAppend(fmt.Sprintf("Something went wrong downloading \"%s\": %s.", track.Filename, err.Error()), spttb_gui.PanelRight)
					tracks_failed = append(tracks_failed, track)
					continue
				} else {
					track.URL = youtube_track.URL
				}
			}

			if track.Local && !*arg_flush_metadata && !*arg_replace_local {
				continue
			}

			if (track.Local && *arg_flush_metadata) || !track.Local {
				if !*arg_disable_lyrics && (!*arg_flush_missing || (*arg_flush_missing && !track.HasID3Frame(spttb_track.ID3FrameLyrics))) {
					gui.DebugAppend(fmt.Sprintf("Fetching song \"%s\" lyrics...", track.Filename), spttb_gui.PanelRight)
					err := (&track).SearchLyrics()
					if err != nil {
						gui.WarnAppend(fmt.Sprintf("Something went wrong while searching for song \"%s\" lyrics: %s", track.Filename, err.Error()), spttb_gui.PanelRight)
					}
				}

				if !spttb_system.FileExists(track.FilenameArtwork()) && (!*arg_flush_missing || (*arg_flush_missing && !track.HasID3Frame(spttb_track.ID3FrameArtwork))) {
					gui.DebugAppend(fmt.Sprintf("Downloading song \"%s\" artwork at %s...", track.Filename, track.Image), spttb_gui.PanelRight)
					var command_out bytes.Buffer
					command_cmd := "ffmpeg"
					command_args := []string{"-i", track.Image, "-q:v", "1", "-n", track.FilenameArtwork()}
					command_obj := exec.Command(command_cmd, command_args...)
					command_obj.Stderr = &command_out
					if err := command_obj.Run(); err != nil {
						gui.WarnAppend(fmt.Sprintf("Unable to download artwork file \"%s\": %s", track.Image, command_out.String()), spttb_gui.PanelRight)
					}
				}
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
					gui.WarnAppend(fmt.Sprintf("Unable to flush timestamp on %s", track.FilenameFinal()), spttb_gui.PanelRight)
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
						"./" + track.FilenameFinal() + "\n"
				}
			}
			playlist_m3u_file, playlist_err := os.Create(playlist_info.Name + ".m3u")
			if playlist_err != nil {
				gui.WarnAppend(fmt.Sprintf("Unable to create M3U file: %s", playlist_err.Error()), spttb_gui.PanelRight)
			} else {
				defer playlist_m3u_file.Close()
				_, playlist_err := playlist_m3u_file.WriteString(playlist_m3u)
				playlist_m3u_file.Sync()
				if playlist_err != nil {
					gui.WarnAppend(fmt.Sprintf("Unable to write M3U file: %s", playlist_err.Error()), spttb_gui.PanelRight)
				}
			}
		}

		junks := CleanJunks()
		gui.Append(fmt.Sprintf("Removed %d junk files.", junks), spttb_gui.PanelRight)

		if len(tracks_failed) > 0 {
			gui.Append(fmt.Sprintf("Synchronization partially completed, %d tracks failed to synchronize:", len(tracks_failed)), spttb_gui.PanelRight)
			for _, track := range tracks_failed {
				gui.Append(fmt.Sprintf(" - \"%s\"", track.Filename), spttb_gui.PanelRight)
			}
		}
		close(wait_group_pool)
		wait_group.Wait()
		gui.Prompt("Synchronization completed.", spttb_gui.PromptDismissableWithExit)
	} else {
		gui.Prompt("No song needs to be downloaded.", spttb_gui.PromptDismissableWithExit)
	}

	CleanJunks()
	Exit()
}

func ParallelSongProcess(track spttb_track.Track, wg *sync.WaitGroup) {
	defer wg.Done()
	<-wait_group_pool

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
		gui.DebugAppend(fmt.Sprintf("Getting max_volume value: \"%s %s\"...", command_cmd, strings.Join(command_args, " ")), spttb_gui.PanelRight)
		command_obj := exec.Command(command_cmd, command_args...)
		command_obj.Stderr = &command_out
		command_err = command_obj.Run()
		if command_err != nil {
			gui.WarnAppend(fmt.Sprintf("Unable to use ffmpeg to pull max_volume song value: %s.", command_out.String()), spttb_gui.PanelRight)
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
			gui.WarnAppend(fmt.Sprintf("Unable to pull max_volume delta to be applied along with song volume normalization: %s.", normalization_delta), spttb_gui.PanelRight)
			normalization_delta = "0.0"
		}
		command_args = []string{"-i", track.FilenameTemporary(), "-af", "volume=+" + normalization_delta + "dB", "-b:a", "320k", "-y", normalization_file}
		gui.DebugAppend(fmt.Sprintf("Compensating volume by %sdB...", normalization_delta), spttb_gui.PanelRight)
		gui.Append(fmt.Sprintf("Increasing audio quality for: %s...", track.Filename), spttb_gui.PanelRight)
		gui.DebugAppend(fmt.Sprintf("Firing command: \"%s %s\"...", command_cmd, strings.Join(command_args, " ")), spttb_gui.PanelRight)
		if _, command_err = exec.Command(command_cmd, command_args...).Output(); command_err != nil {
			gui.WarnAppend(fmt.Sprintf("Something went wrong while normalizing song \"%s\" volume: %s", track.Filename, command_err.Error()), spttb_gui.PanelRight)
		}
		os.Remove(track.FilenameTemporary())
		os.Rename(normalization_file, track.FilenameTemporary())
	}

	if !spttb_system.FileExists(track.FilenameTemporary()) && spttb_system.FileExists(track.FilenameFinal()) {
		err := spttb_system.FileCopy(track.FilenameFinal(),
			track.FilenameTemporary())
		if err != nil {
			gui.WarnAppend(fmt.Sprintf("Unable to prepare song for getting its metadata flushed: %s", err.Error()), spttb_gui.PanelRight)
			return
		}
	}

	if (track.Local && *arg_flush_metadata) || !track.Local {
		var (
			track_artwork_reader []byte
			track_artwork_err    error
		)
		if spttb_system.FileExists(track.FilenameArtwork()) && (!*arg_flush_missing ||
			(*arg_flush_missing && !track.HasID3Frame(spttb_track.ID3FrameArtwork))) {
			track_artwork_reader, track_artwork_err = ioutil.ReadFile(track.FilenameArtwork())
			if track_artwork_err != nil {
				gui.WarnAppend(fmt.Sprintf("Unable to read artwork file: %s", track_artwork_err.Error()), spttb_gui.PanelRight)
			}
		}

		track_mp3, err := id3.Open(track.FilenameTemporary(), id3.Options{Parse: true})
		if track_mp3 == nil || err != nil {
			gui.WarnAppend(fmt.Sprintf("Something bad happened while opening: %s", err.Error()), spttb_gui.PanelRight)
		} else {
			gui.Append(fmt.Sprintf("Fixing metadata for \"%s\"...", track.Filename), spttb_gui.PanelRight)
			if !*arg_flush_missing {
				track_mp3.DeleteAllFrames()
			}
			if len(track.Title) > 0 && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameTitle))) {
				gui.DebugAppend("Inflating title metadata...", spttb_gui.PanelRight)
				track_mp3.SetTitle(track.Title)
			}
			if len(track.Artist) > 0 && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameArtist))) {
				gui.DebugAppend("Inflating artist metadata...", spttb_gui.PanelRight)
				track_mp3.SetArtist(track.Artist)
			}
			if len(track.Album) > 0 && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameAlbum))) {
				gui.DebugAppend("Inflating album metadata...", spttb_gui.PanelRight)
				track_mp3.SetAlbum(track.Album)
			}
			if len(track.Genre) > 0 && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameGenre))) {
				gui.DebugAppend("Inflating genre metadata...", spttb_gui.PanelRight)
				track_mp3.SetGenre(track.Genre)
			}
			if len(track.Year) > 0 && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameYear))) {
				gui.DebugAppend("Inflating year metadata...", spttb_gui.PanelRight)
				track_mp3.SetYear(track.Year)
			}
			if track.TrackNumber > 0 && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameTrackNumber))) {
				gui.DebugAppend("Inflating track number metadata...", spttb_gui.PanelRight)
				track_mp3.AddFrame(track_mp3.CommonID("Track number/Position in set"),
					id3.TextFrame{
						Encoding: id3.EncodingUTF8,
						Text:     strconv.Itoa(track.TrackNumber),
					})
			}
			if track_artwork_err == nil && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameArtwork))) {
				gui.DebugAppend("Inflating artwork metadata...", spttb_gui.PanelRight)
				track_mp3.AddAttachedPicture(id3.PictureFrame{
					Encoding:    id3.EncodingUTF8,
					MimeType:    "image/jpeg",
					PictureType: id3.PTFrontCover,
					Description: "Front cover",
					Picture:     track_artwork_reader,
				})
			}
			if len(track.URL) > 0 && (!*arg_flush_missing ||
				(*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameYouTubeURL))) {
				gui.DebugAppend("Inflating youtube origin url metadata...", spttb_gui.PanelRight)
				track_mp3.AddCommentFrame(id3.CommentFrame{
					Encoding:    id3.EncodingUTF8,
					Language:    "eng",
					Description: "youtube",
					Text:        track.URL,
				})
			}
			if len(track.Lyrics) > 0 && !*arg_disable_lyrics &&
				(!*arg_flush_missing || (*arg_flush_missing && !spttb_track.TagHasFrame(track_mp3, spttb_track.ID3FrameLyrics))) {
				gui.DebugAppend("Inflating lyrics metadata...", spttb_gui.PanelRight)
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

	os.Remove(track.FilenameFinal())
	err := os.Rename(track.FilenameTemporary(), track.FilenameFinal())
	if err != nil {
		gui.WarnAppend(fmt.Sprintf("Unable to move song to its final path: %s", err.Error()), spttb_gui.PanelRight)
	}

	wait_group_pool <- true
}

func CleanJunks() int {
	var removed_junks int
	for _, junk_type := range spttb_track.JunkWildcards {
		junk_paths, err := filepath.Glob(junk_type)
		if err != nil {
			continue
		}
		for _, junk_path := range junk_paths {
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
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
	os.Exit(0)
}
