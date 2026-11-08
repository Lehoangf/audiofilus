package daemon

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/juanvillacortac/audiofilus/internal/control"
	"github.com/juanvillacortac/audiofilus/internal/spotify"
)

// isSpotifyURI comprueba si la URI pertenece a una pista de Spotify.
func isSpotifyURI(uri string) bool {
	return spotify.IsSpotify(uri)
}

// handleSpotify atiende los comandos del socket de control relacionados con Spotify.
func (d *Daemon) handleSpotify(req control.Request) (any, error) {
	if len(req.Args) == 0 {
		return nil, errors.New("missing spotify subcommand (login, status, playlists, liked)")
	}

	sub := req.Args[0]
	if d.spotifyMgr == nil {
		return nil, errors.New("spotify manager not initialized")
	}

	switch sub {
	case "login":
		urlStr, err := d.spotifyMgr.Auth.StartLoginFlow()
		if err != nil {
			return nil, fmt.Errorf("start login flow: %w", err)
		}
		return map[string]string{"login_url": urlStr}, nil

	case "status":
		creds, err := d.spotifyMgr.Auth.Credentials()
		if err != nil {
			return map[string]any{"logged_in": false}, nil
		}
		return map[string]any{
			"logged_in": true,
			"product":   creds.Product,
		}, nil

	case "playlists":
		pls, err := d.spotifyMgr.Catalog.SyncPlaylists()
		if err != nil {
			return nil, err
		}
		for _, pl := range pls {
			for _, tr := range pl.Tracks {
				d.spotifyMgr.RegisterTrack(tr)
			}
		}
		return pls, nil

	case "liked":
		tracks, err := d.spotifyMgr.Catalog.LikedSongs()
		if err != nil {
			return nil, err
		}
		for _, tr := range tracks {
			d.spotifyMgr.RegisterTrack(tr)
		}
		return tracks, nil

	default:
		return nil, fmt.Errorf("unknown spotify subcommand: %s", sub)
	}
}

// initSpotifyManager inicializa el gestor de Spotify para el daemon.
func (d *Daemon) initSpotifyManager() {
	d.spotifyMgr = spotify.NewManager(d.paths.Data)
}

var _ = json.Marshal
