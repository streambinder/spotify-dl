package spotify

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	spttb_system "system"

	api "github.com/zmb3/spotify"
)

var (
	client_channel       = make(chan *api.Client)
	client_state         = spttb_system.RandString(20)
	client_authenticator = api.NewAuthenticator(
		spttb_system.SPOTIFY_REDIRECT_URI,
		api.ScopeUserLibraryRead,
		api.ScopePlaylistReadPrivate,
		api.ScopePlaylistReadCollaborative)
)

type Spotify struct {
	Client *api.Client
}

func NewClient() *Spotify {
	return &Spotify{}
}

func AuthUrl() string {
	client_authenticator.SetAuthInfo(spttb_system.SPOTIFY_CLIENT_ID, spttb_system.SPOTIFY_CLIENT_SECRET)
	return client_authenticator.AuthURL(client_state)
}

func (spotify *Spotify) Auth(url string) bool {
	http.HandleFunc("/favicon.ico", HttpFaviconHandler)
	http.HandleFunc("/callback", HttpCompleteAuthHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Got request for:", r.URL.String())
	})
	go http.ListenAndServe(":8080", nil)

	command_cmd := "xdg-open"
	command_args := []string{url}
	_, err := exec.Command(command_cmd, command_args...).Output()
	if err != nil {
		return false
	}

	spotify.Client = <-client_channel

	return true
}

func (spotify *Spotify) LibraryTracks() ([]api.FullTrack, error) {
	var tracks []api.FullTrack
	var iterations int = 0
	var options api.Options = spotify.DefaultOptions()
	for true {
		*options.Offset = *options.Limit * iterations
		chunk, err := spotify.Client.CurrentUsersTracksOpt(&options)
		if err != nil {
			return []api.FullTrack{}, errors.New(fmt.Sprintf("Something gone wrong while reading %dth chunk of tracks: %s.", iterations, err.Error()))
		}
		for _, track := range chunk.Tracks {
			tracks = append(tracks, track.FullTrack)
		}
		if len(chunk.Tracks) < 50 {
			break
		}
		iterations++
	}
	return tracks, nil
}

func (spotify *Spotify) Playlist(playlist_uri string) (*api.FullPlaylist, error) {
	playlist_owner, playlist_id := spotify.ParsePlaylistUri(playlist_uri)
	return spotify.Client.GetPlaylist(playlist_owner, playlist_id)
}

func (spotify *Spotify) PlaylistTracks(playlist_uri string) ([]api.FullTrack, error) {
	playlist_owner, playlist_id := spotify.ParsePlaylistUri(playlist_uri)
	var tracks []api.FullTrack
	var iterations int = 0
	var options api.Options = spotify.DefaultOptions()
	for true {
		*options.Offset = *options.Limit * iterations
		chunk, err := spotify.Client.GetPlaylistTracksOpt(playlist_owner, playlist_id, &options, "")
		if err != nil {
			return []api.FullTrack{}, errors.New(fmt.Sprintf("Something gone wrong while reading %dth chunk of tracks: %s.", iterations, err.Error()))
		}
		for _, track := range chunk.Tracks {
			tracks = append(tracks, track.Track)
		}
		if len(chunk.Tracks) < 50 {
			break
		}
		iterations++
	}
	return tracks, nil
}

func (spotify *Spotify) Albums(ids []api.ID) ([]api.FullAlbum, error) {
	var albums []api.FullAlbum
	var iterations int = 0
	var upperbound, lowerbound int
	for true {
		lowerbound = iterations * 20
		if upperbound = lowerbound + 20; upperbound >= len(ids) {
			upperbound = lowerbound + (len(ids) - lowerbound)
		}
		chunk, err := spotify.Client.GetAlbums(ids[lowerbound:upperbound]...)
		if err != nil {
			return []api.FullAlbum{}, errors.New(fmt.Sprintf("Something gone wrong in %dth chunk of albums: %s.", iterations, err.Error()))
		}
		for _, album := range chunk {
			albums = append(albums, *album)
		}
		if len(chunk) < 20 {
			break
		}
		iterations++
	}
	return albums, nil
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
	http.Redirect(w, r, spttb_system.SPOTIFY_FAVICON_URL, 301)
}

func HttpCompleteAuthHandler(w http.ResponseWriter, r *http.Request) {
	tok, err := client_authenticator.Token(client_state, r)
	if err != nil {
		http.Error(w, HttpMessage("Couldn't get token", "none"), http.StatusForbidden)
		// logger.Fatal("Couldn't get token.")
	}
	if st := r.FormValue("state"); st != client_state {
		http.NotFound(w, r)
		// logger.Fatal("\"state\" value not found.")
	}
	client := client_authenticator.NewClient(tok)
	fmt.Fprintf(w, HttpMessage("Login completed", "Come back to the shell and enjoy the magic!"))
	// logger.Log("Login process completed.")
	client_channel <- &client
}

func HttpMessage(content_title string, content_subtitle string) string {
	return fmt.Sprintf(spttb_system.SPOTIFY_HTML_TEMPLATE, content_title, content_subtitle)
}
