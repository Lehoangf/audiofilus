package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/juanvillacortac/audiofilus/internal/config"
	"github.com/juanvillacortac/audiofilus/internal/control"
	"github.com/juanvillacortac/audiofilus/internal/spotify"
)

// isSpotifyURI checks whether a URI is a Spotify track URI.
func isSpotifyURI(uri string) bool {
	return spotify.IsSpotify(uri)
}

// handleSpotify handles Spotify-related control socket requests.
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
			d.spotifyMgr.CacheTracksInMemory(pl.Tracks)
		}
		return pls, nil

	case "cached_playlists":
		pls := d.spotifyMgr.Catalog.CachedPlaylists()
		for _, pl := range pls {
			d.spotifyMgr.CacheTracksInMemory(pl.Tracks)
		}
		return pls, nil

	case "playlist_tracks":
		if len(req.Args) < 2 {
			return nil, errors.New("missing playlist id")
		}
		plID := req.Args[1]
		tracks, err := d.spotifyMgr.Catalog.PlaylistTracks(plID)
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(tracks)
		return tracks, nil

	case "playlist_page":
		if len(req.Args) < 2 {
			return nil, errors.New("missing playlist id")
		}
		plID := req.Args[1]
		offset := 0
		limit := 50
		if len(req.Args) >= 3 {
			if off, err := strconv.Atoi(req.Args[2]); err == nil && off >= 0 {
				offset = off
			}
		}
		if len(req.Args) >= 4 {
			if lim, err := strconv.Atoi(req.Args[3]); err == nil && lim > 0 {
				limit = lim
			}
		}
		res, err := d.spotifyMgr.Catalog.PlaylistPage(plID, offset, limit)
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(res.Tracks)
		return res, nil

	case "liked":
		tracks, err := d.spotifyMgr.Catalog.LikedSongs()
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(tracks)
		return tracks, nil

	case "liked_page":
		offset := 0
		limit := 50
		forceSync := false
		if len(req.Args) >= 2 {
			if off, err := strconv.Atoi(req.Args[1]); err == nil && off >= 0 {
				offset = off
			}
		}
		if len(req.Args) >= 3 {
			if lim, err := strconv.Atoi(req.Args[2]); err == nil && lim > 0 {
				limit = lim
			}
		}
		if len(req.Args) >= 4 {
			if fs, err := strconv.ParseBool(req.Args[3]); err == nil {
				forceSync = fs
			}
		}
		res, err := d.spotifyMgr.Catalog.LikedPage(offset, limit, forceSync)
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(res.Tracks)
		return res, nil

	case "cached_liked":
		tracks := d.spotifyMgr.Catalog.CachedLiked()
		d.spotifyMgr.CacheTracksInMemory(tracks)
		return tracks, nil

	case "search":
		if len(req.Args) < 2 {
			return nil, errors.New("missing search query")
		}
		query := req.Args[1]
		tracks, err := d.spotifyMgr.Catalog.Search(query)
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(tracks)
		return tracks, nil

	case "albums":
		albums, err := d.spotifyMgr.Catalog.SavedAlbums()
		if err != nil {
			return nil, err
		}
		for _, al := range albums {
			d.spotifyMgr.CacheTracksInMemory(al.Tracks)
		}
		return albums, nil

	case "album_page":
		if len(req.Args) < 2 {
			return nil, errors.New("missing album id")
		}
		alID := req.Args[1]
		offset := 0
		limit := 50
		if len(req.Args) >= 3 {
			if off, err := strconv.Atoi(req.Args[2]); err == nil && off >= 0 {
				offset = off
			}
		}
		if len(req.Args) >= 4 {
			if lim, err := strconv.Atoi(req.Args[3]); err == nil && lim > 0 {
				limit = lim
			}
		}
		res, err := d.spotifyMgr.Catalog.AlbumPage(alID, offset, limit)
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(res.Tracks)
		return res, nil

	case "album_tracks":
		if len(req.Args) < 2 {
			return nil, errors.New("missing album id")
		}
		alID := req.Args[1]
		tracks, err := d.spotifyMgr.Catalog.AlbumTracks(alID)
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(tracks)
		return tracks, nil

	case "cached_albums":
		albums := d.spotifyMgr.Catalog.CachedAlbums()
		for _, al := range albums {
			d.spotifyMgr.CacheTracksInMemory(al.Tracks)
		}
		return albums, nil

	case "track":
		if len(req.Args) < 2 {
			return nil, errors.New("missing track id")
		}
		tr, err := d.spotifyMgr.FetchTrack(req.Args[1])
		if err != nil {
			return nil, err
		}
		return tr, nil

	case "tracks":
		if len(req.Args) < 2 {
			return nil, errors.New("missing track ids")
		}
		out := make(map[string]spotify.Track)
		var missing []string
		for _, arg := range req.Args[1:] {
			id := spotify.ExtractTrackID(arg)
			if id == "" {
				continue
			}
			if tr, ok := d.spotifyMgr.Track(id); ok && tr.Title != "" && tr.Title != "Spotify Track" {
				out[id] = tr
			} else {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			if fetched, err := d.spotifyMgr.Catalog.TracksByIDs(missing); err == nil {
				d.spotifyMgr.CacheTracksInMemory(fetched)
				for _, tr := range fetched {
					if tr.Title != "" && tr.Title != "Spotify Track" {
						out[tr.ID] = tr
					}
				}
			}
		}
		return out, nil

	case "play_collection":
		if len(req.Args) < 4 {
			return nil, errors.New("usage: play_collection <playlist|liked|album> <id> <start> [initial_tracks...]")
		}
		if d.eng == nil {
			return nil, errors.New("engine not initialized")
		}
		collType := req.Args[1]
		collID := req.Args[2]
		start, err := strconv.Atoi(req.Args[3])
		if err != nil {
			return nil, fmt.Errorf("invalid start index: %w", err)
		}
		var initialURIs []string
		loaded := make(map[string]bool)
		for _, arg := range req.Args[4:] {
			id := spotify.ExtractTrackID(arg)
			if id == "" {
				continue
			}
			loaded[id] = true
			initialURIs = append(initialURIs, "spotify://track/"+id)
		}
		if len(initialURIs) == 0 {
			return nil, errors.New("no tracks to play")
		}
		if start < 0 || start >= len(initialURIs) {
			start = 0
		}

		d.cancelSpotifyHydration()
		gen := d.eng.LoadGen(initialURIs, start, 0, true)

		ctx, cancel := context.WithCancel(context.Background())
		d.spotHydrateMu.Lock()
		d.spotHydrateCancel = cancel
		d.spotHydrateMu.Unlock()

		go func(ctx context.Context, gen uint64, collType, collID string) {
			defer cancel()
			onBatch := func(batchIDs []string) bool {
				select {
				case <-ctx.Done():
					return false
				default:
				}
				var newURIs []string
				for _, raw := range batchIDs {
					id := spotify.ExtractTrackID(raw)
					if id != "" && !loaded[id] {
						loaded[id] = true
						newURIs = append(newURIs, "spotify://track/"+id)
					}
				}
				if len(newURIs) == 0 {
					return true
				}
				return d.eng.AppendIfGen(gen, newURIs)
			}

			switch collType {
			case "playlist":
				ids, _ := d.spotifyMgr.Catalog.PlaylistTrackIDs(ctx, collID, onBatch)
				onBatch(ids)
			case "liked":
				ids, _ := d.spotifyMgr.Catalog.LikedTrackIDs(ctx, onBatch)
				onBatch(ids)
			case "album":
				ids, _ := d.spotifyMgr.Catalog.AlbumTrackIDs(ctx, collID, onBatch)
				onBatch(ids)
			}
		}(ctx, gen, collType, collID)

		return map[string]any{
			"status": "playing",
			"count":  len(initialURIs),
			"gen":    gen,
		}, nil

	case "recent":
		tracks, err := d.spotifyMgr.Catalog.RecentlyPlayed()
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(tracks)
		return tracks, nil

	case "download":
		if len(req.Args) < 2 {
			return nil, errors.New("missing track id")
		}
		for _, arg := range req.Args[1:] {
			_ = d.spotifyMgr.Download(arg)
		}
		return map[string]string{"status": "queued"}, nil

	case "cached":
		out := make(map[string]bool)
		for _, arg := range req.Args[1:] {
			id := spotify.ExtractTrackID(arg)
			out[id] = d.spotifyMgr.IsCached(id)
		}
		return out, nil

	case "cache_path":
		if len(req.Args) < 2 {
			return nil, errors.New("missing track id")
		}
		id := spotify.ExtractTrackID(req.Args[1])
		p := d.spotifyMgr.TrackPath(id)
		return map[string]any{
			"id":     id,
			"cached": p != "",
			"path":   p,
		}, nil

	case "is_liked":
		if len(req.Args) < 2 {
			return nil, errors.New("missing track id")
		}
		id := spotify.ExtractTrackID(req.Args[1])
		liked := d.spotifyMgr.Catalog.IsTrackLiked(id)
		return map[string]any{"id": id, "liked": liked}, nil

	case "like":
		if len(req.Args) < 2 {
			return nil, errors.New("missing track id")
		}
		id := spotify.ExtractTrackID(req.Args[1])
		if err := d.spotifyMgr.Catalog.LikeTrack(context.Background(), id); err != nil {
			return nil, err
		}
		return map[string]any{"status": "ok", "id": id, "liked": true}, nil

	case "unlike":
		if len(req.Args) < 2 {
			return nil, errors.New("missing track id")
		}
		id := spotify.ExtractTrackID(req.Args[1])
		if err := d.spotifyMgr.Catalog.UnlikeTrack(context.Background(), id); err != nil {
			return nil, err
		}
		return map[string]any{"status": "ok", "id": id, "liked": false}, nil

	case "add_to_playlist":
		if len(req.Args) < 3 {
			return nil, errors.New("usage: add_to_playlist <playlist_id> <track_id>")
		}
		plID := req.Args[1]
		trackID := spotify.ExtractTrackID(req.Args[2])
		if err := d.spotifyMgr.Catalog.AddTrackToPlaylist(context.Background(), plID, trackID); err != nil {
			return nil, err
		}
		return map[string]any{"status": "ok", "playlist_id": plID, "track_id": trackID}, nil

	case "recommendations":
		limit := 20
		var seeds []string
		for _, arg := range req.Args[1:] {
			if lim, err := strconv.Atoi(arg); err == nil && len(seeds) == 0 && lim > 0 {
				limit = lim
				continue
			}
			id := spotify.ExtractTrackID(arg)
			if id != "" {
				seeds = append(seeds, id)
			}
		}
		tracks, err := d.spotifyMgr.Catalog.Recommendations(context.Background(), seeds, limit)
		if err != nil {
			return nil, err
		}
		d.spotifyMgr.CacheTracksInMemory(tracks)
		return tracks, nil

	case "smart_shuffle":
		if len(req.Args) == 1 {
			return map[string]any{"enabled": d.isSpotifySmartShuffle()}, nil
		}
		on, err := onOff(req.Args[1:], !d.isSpotifySmartShuffle())
		if err != nil {
			return nil, err
		}
		d.setSpotifySmartShuffle(on)
		return map[string]any{"status": "ok", "enabled": on}, nil

	default:
		return nil, fmt.Errorf("unknown spotify subcommand: %s", sub)
	}
}

func (d *Daemon) spotifyMusicDirs() []string {
	d.spotifyDirsMu.Lock()
	defer d.spotifyDirsMu.Unlock()

	if d.paths.Config != "" {
		if fi, err := os.Stat(d.paths.Config); err == nil && !fi.IsDir() {
			if d.spotifyDirs != nil && fi.ModTime().Equal(d.spotifyCfgMod) && fi.Size() == d.spotifyCfgSz {
				return append([]string(nil), d.spotifyDirs...)
			}
			if c, loadErr := config.Load(d.paths); loadErr == nil {
				d.spotifyDirs = append([]string(nil), c.MusicDirs...)
				d.spotifyCfgMod = fi.ModTime()
				d.spotifyCfgSz = fi.Size()
				return append([]string(nil), d.spotifyDirs...)
			}
		}
	}
	return append([]string(nil), d.cfg.MusicDirs...)
}

// initSpotifyManager initializes the Spotify manager for the daemon.
func (d *Daemon) initSpotifyManager() {
	d.spotifyMgr = spotify.NewManager(d.paths.Base, d.paths.Data, d.cfg.Spotify.ClientID, d.cfg.Spotify.ClientSecret)
	d.spotifyMgr.SetMusicDirs(d.cfg.MusicDirs)
	d.spotifyMgr.SetMusicDirsFunc(d.spotifyMusicDirs)
	d.spotifyMgr.OnMetadataResolved = func(tr spotify.Track) {
		if d.eng != nil {
			go d.eng.UpdateTrackMetadata("spotify://track/"+tr.ID, tr.Title, tr.Artist, tr.Album)
		}
	}
}

var _ = json.Marshal
