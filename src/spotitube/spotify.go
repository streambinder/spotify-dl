package spotitube

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	api "github.com/zmb3/spotify"
)

var (
	client_channel       = make(chan *api.Client)
	client_state         = RandString(20)
	client_authenticator = api.NewAuthenticator(
		SPOTIFY_REDIRECT_URI,
		api.ScopeUserLibraryRead,
		api.ScopePlaylistReadPrivate,
		api.ScopePlaylistReadCollaborative)
)

type Spotify struct {
	Client *api.Client
}

func NewSpotifyClient() *Spotify {
	return &Spotify{}
}

func (spotify *Spotify) Auth() bool {
	http.HandleFunc("/favicon.ico", HttpFaviconHandler)
	http.HandleFunc("/callback", HttpCompleteAuthHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Got request for:", r.URL.String())
	})
	go http.ListenAndServe(":8080", nil)

	client_authenticator.SetAuthInfo(SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET)
	url := client_authenticator.AuthURL(client_state)
	command_cmd := "xdg-open"
	command_args := []string{url}
	_, err := exec.Command(command_cmd, command_args...).Output()
	if err != nil {
		logger.Log("Something went wrong while executing trying to make the default browser open the authorization URL.")
		logger.Log("Please, in order to authorize me to read your library, go to:\n" + url)
	}

	logger.Log("Waiting for authentication process to complete.")
	client := <-client_channel
	spotify.Client = client

	return true
}

func (spotify *Spotify) Library() []api.FullTrack {
	logger.Log("Reading user library.")
	var tracks []api.FullTrack
	var iterations int = 0
	var options api.Options = spotify.DefaultOptions()
	for true {
		*options.Offset = *options.Limit * iterations
		chunk, err := spotify.Client.CurrentUsersTracksOpt(&options)
		if err != nil {
			logger.Fatal("Something gone wrong while reading " + strconv.Itoa(iterations) + "th chunk of tracks: " + err.Error() + ".")
		}
		for _, track := range chunk.Tracks {
			tracks = append(tracks, track.FullTrack)
		}
		if len(chunk.Tracks) < 50 {
			break
		}
		iterations++
	}
	return tracks
}

func (spotify *Spotify) Playlist(playlist_uri string) []api.FullTrack {
	playlist_owner, playlist_id := spotify.ParsePlaylistUri(playlist_uri)
	logger.Log("Reading playlist with ID \"" + string(playlist_id) + "\" by \"" + playlist_owner + "\".")
	var tracks []api.FullTrack
	var iterations int = 0
	var options api.Options = spotify.DefaultOptions()
	for true {
		*options.Offset = *options.Limit * iterations
		chunk, err := spotify.Client.GetPlaylistTracksOpt(playlist_owner, playlist_id, &options, "")
		if err != nil {
			logger.Fatal("Something gone wrong while reading " + strconv.Itoa(iterations) + "th chunk of tracks: " + err.Error() + ".")
		}
		for _, track := range chunk.Tracks {
			tracks = append(tracks, track.Track)
		}
		if len(chunk.Tracks) < 50 {
			break
		}
		iterations++
	}
	return tracks
}

func (spotify *Spotify) Album(id api.ID) (*api.FullAlbum, error) {
	return spotify.Client.GetAlbum(id)
}

func (spotify *Spotify) DefaultOptions() api.Options {
	var opt_limit int = 50
	var opt_offset int = 0
	return api.Options{
		Limit:  &opt_limit,
		Offset: &opt_offset,
	}
}

func (spotify *Spotify) ParsePlaylistUri(playlist_uri string) (string, api.ID) {
	return strings.Split(playlist_uri, ":")[2], api.ID(strings.Split(playlist_uri, ":")[4])
}

func HttpFaviconHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, SPOTIFY_FAVICON_URL, 301)
}

func HttpCompleteAuthHandler(w http.ResponseWriter, r *http.Request) {
	tok, err := client_authenticator.Token(client_state, r)
	if err != nil {
		http.Error(w, HttpMessage("Couldn't get token", "none"), http.StatusForbidden)
		logger.Fatal("Couldn't get token.")
	}
	if st := r.FormValue("state"); st != client_state {
		http.NotFound(w, r)
		logger.Fatal("\"state\" value not found.")
	}
	client := client_authenticator.NewClient(tok)
	fmt.Fprintf(w, HttpMessage("Login completed", "Come back to the shell and enjoy the magic!"))
	logger.Log("Login process completed.")
	client_channel <- &client
}

func HttpMessage(content_title string, content_subtitle string) string {
	return fmt.Sprintf(SPOTIFY_HTML_TEMPLATE, content_title, content_subtitle)
}
