package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Track represents a Spotify music track with essential metadata.
type Track struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Artist   string        `json:"artist"`
	Album    string        `json:"album"`
	ISRC     string        `json:"isrc,omitempty"`
	Duration time.Duration `json:"duration"`
	CoverURL string        `json:"cover_url,omitempty"`
}

// Playlist represents a playlist or album with snapshot/pagination state for local cache.
type Playlist struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	SnapshotID       string   `json:"snapshot_id"`
	TrackCount       int      `json:"track_count,omitempty"`
	Tracks           []Track  `json:"tracks"`
	TrackIDs         []string `json:"track_ids,omitempty"`
	FetchedOffset    int      `json:"fetched_offset,omitempty"`
	IDsFetchedOffset int      `json:"ids_fetched_offset,omitempty"`
	TracksComplete   bool     `json:"tracks_complete,omitempty"`
	IDsComplete      bool     `json:"ids_complete,omitempty"`
}

// PageResult represents a single paginated track slice along with total and completion state.
type PageResult struct {
	Tracks     []Track `json:"tracks"`
	Total      int     `json:"total"`
	NextOffset int     `json:"next_offset"`
	Complete   bool    `json:"complete"`
}

// catalogData is the serialized schema for on-disk cache.
type catalogData struct {
	Playlists           []Playlist       `json:"playlists"`
	LikedTracks         []Track          `json:"liked_tracks"`
	LikedTotal          int              `json:"liked_total,omitempty"`
	LikedTrackIDs       []string         `json:"liked_track_ids,omitempty"`
	LikedFetchedOffset  int              `json:"liked_fetched_offset,omitempty"`
	LikedTracksComplete bool             `json:"liked_tracks_complete,omitempty"`
	LikedIDsComplete    bool             `json:"liked_ids_complete,omitempty"`
	Albums              []Playlist       `json:"albums,omitempty"`
	Tracks              map[string]Track `json:"tracks,omitempty"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

// Catalog manages differential sync and local caching of Spotify playlists and tracks.
type Catalog struct {
	cacheDir  string
	auth      *AuthManager
	client    *http.Client
	apiBase   string
	mu        sync.Mutex
	cachedMem *catalogData
}

// NewCatalog creates a new local catalog manager.
func NewCatalog(cacheDir string, auth *AuthManager, client *http.Client) *Catalog {
	if client == nil {
		client = http.DefaultClient
	}
	return &Catalog{
		cacheDir: cacheDir,
		auth:     auth,
		client:   client,
		apiBase:  "https://api.spotify.com/v1",
	}
}

// SetAPIBaseForTest overrides the API base URL in tests.
func (c *Catalog) SetAPIBaseForTest(base string) {
	c.apiBase = base
}

func (c *Catalog) cacheFile() string {
	return filepath.Join(c.cacheDir, "catalog.json")
}

func (c *Catalog) loadCached() catalogData {
	if c.cachedMem != nil {
		return *c.cachedMem
	}
	b, err := os.ReadFile(c.cacheFile())
	if err != nil {
		cd := catalogData{}
		c.cachedMem = &cd
		return cd
	}
	var cd catalogData
	_ = json.Unmarshal(b, &cd)
	c.cachedMem = &cd
	return cd
}

func (c *Catalog) saveCached(cd catalogData) error {
	c.cachedMem = &cd
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	cd.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cd, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.cacheFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.cacheFile())
}

// SaveTrack caches metadata for an individual track.
func (c *Catalog) SaveTrack(t Track) {
	if t.ID == "" || t.Title == "" || t.Title == "Spotify Track" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()
	if cd.Tracks == nil {
		cd.Tracks = make(map[string]Track)
	}
	if existing, ok := cd.Tracks[t.ID]; ok && existing.Title == t.Title && existing.Artist == t.Artist {
		return
	}
	cd.Tracks[t.ID] = t
	_ = c.saveCached(cd)
}

func (c *Catalog) newAuthorizedRequest(method, urlStr string) (*http.Request, error) {
	return c.newAuthorizedRequestWithContext(context.Background(), method, urlStr)
}

func (c *Catalog) newAuthorizedRequestWithContext(ctx context.Context, method, urlStr string) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		return nil, err
	}
	if c.auth != nil {
		token, err := c.auth.ProactiveToken()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// CachedPlaylists returns playlists from local cache without network access.
func (c *Catalog) CachedPlaylists() []Playlist {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.loadCached().Playlists)
}

// CachedLiked returns liked tracks from local cache without network access.
func (c *Catalog) CachedLiked() []Track {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.loadCached().LikedTracks)
}

// CachedLikedState returns cached liked tracks alongside total count and completion state.
func (c *Catalog) CachedLikedState() (tracks []Track, total int, nextOffset int, complete bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()
	total = cd.LikedTotal
	if total < len(cd.LikedTracks) {
		total = len(cd.LikedTracks)
	}
	nextOffset = cd.LikedFetchedOffset
	if nextOffset < len(cd.LikedTracks) {
		nextOffset = len(cd.LikedTracks)
	}
	return slices.Clone(cd.LikedTracks), total, nextOffset, cd.LikedTracksComplete
}

// CachedAlbums returns albums from local cache without network access.
func (c *Catalog) CachedAlbums() []Playlist {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.loadCached().Albums)
}

// findTrack returns cached track metadata from tracks, liked songs, playlists, or albums.
func (c *Catalog) findTrack(trackID string) (Track, bool) {
	if trackID == "" {
		return Track{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return findTrackInCatalog(c.loadCached(), trackID)
}

func findTrackInCatalog(cd catalogData, trackID string) (Track, bool) {
	if trackID == "" {
		return Track{}, false
	}
	if tr, ok := cd.Tracks[trackID]; ok && tr.Title != "" && tr.Title != "Spotify Track" {
		return tr, true
	}
	for _, tr := range cd.LikedTracks {
		if tr.ID == trackID && tr.Title != "" && tr.Title != "Spotify Track" {
			return tr, true
		}
	}
	for _, pl := range cd.Playlists {
		for _, tr := range pl.Tracks {
			if tr.ID == trackID && tr.Title != "" && tr.Title != "Spotify Track" {
				return tr, true
			}
		}
	}
	for _, al := range cd.Albums {
		for _, tr := range al.Tracks {
			if tr.ID == trackID && tr.Title != "" && tr.Title != "Spotify Track" {
				return tr, true
			}
		}
	}
	return Track{}, false
}

// SyncPlaylists synchronizes playlists using snapshot_id delta sync without pagination caps.
func (c *Catalog) SyncPlaylists() ([]Playlist, error) {
	c.mu.Lock()
	cached := c.loadCached()
	cachedPlaylists := slices.Clone(cached.Playlists)
	cachedMap := make(map[string]Playlist, len(cachedPlaylists))
	for _, pl := range cachedPlaylists {
		cachedMap[pl.ID] = pl
	}
	c.mu.Unlock()

	var updated []Playlist
	nextURL := c.apiBase + "/me/playlists?limit=50"

	for nextURL != "" {
		req, err := c.newAuthorizedRequest(http.MethodGet, nextURL)
		if err != nil {
			if len(updated) > 0 {
				break
			}
			return cachedPlaylists, err
		}

		resp, err := c.client.Do(req)
		if err != nil {
			if len(updated) > 0 {
				break
			}
			if len(cachedPlaylists) > 0 {
				return cachedPlaylists, nil
			}
			return cachedPlaylists, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if len(updated) > 0 {
				break
			}
			if len(cachedPlaylists) > 0 {
				return cachedPlaylists, nil
			}
			return cachedPlaylists, fmt.Errorf("fetch playlists status: %d", resp.StatusCode)
		}

		var res struct {
			Next  string `json:"next"`
			Total int    `json:"total"`
			Items []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				SnapshotID string `json:"snapshot_id"`
				Tracks     struct {
					Total int `json:"total"`
				} `json:"tracks"`
			} `json:"items"`
		}
		err = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if err != nil {
			if len(updated) > 0 {
				break
			}
			return cachedPlaylists, err
		}

		if len(updated) == 0 && res.Next == "" && len(res.Items) == len(cachedPlaylists) && (res.Total == 0 || res.Total == len(cachedPlaylists)) {
			allMatch := true
			for i, item := range res.Items {
				cPl := cachedPlaylists[i]
				if cPl.ID != item.ID || cPl.Name != item.Name || cPl.SnapshotID != item.SnapshotID {
					allMatch = false
					break
				}
				if item.Tracks.Total > 0 && cPl.TrackCount != item.Tracks.Total {
					allMatch = false
					break
				}
			}
			if allMatch {
				return cachedPlaylists, nil
			}
		}

		for _, item := range res.Items {
			pl := Playlist{
				ID:         item.ID,
				Name:       item.Name,
				SnapshotID: item.SnapshotID,
				TrackCount: item.Tracks.Total,
			}
			cPl, exists := cachedMap[item.ID]
			// Delta Sync: reuse cached tracks and pagination progress only if snapshot_id matches.
			if exists && cPl.SnapshotID == item.SnapshotID {
				pl.Tracks = slices.Clone(cPl.Tracks)
				pl.TrackIDs = slices.Clone(cPl.TrackIDs)
				pl.FetchedOffset = cPl.FetchedOffset
				pl.IDsFetchedOffset = cPl.IDsFetchedOffset
				pl.TracksComplete = cPl.TracksComplete
				pl.IDsComplete = cPl.IDsComplete
				if pl.TrackCount == 0 {
					if cPl.TrackCount > 0 {
						pl.TrackCount = cPl.TrackCount
					} else if len(cPl.Tracks) > 0 {
						pl.TrackCount = len(cPl.Tracks)
					}
				}
				if pl.TrackCount > 0 {
					if effectiveFetchedOffset(pl) < pl.TrackCount && len(pl.Tracks) < pl.TrackCount {
						pl.TracksComplete = false
					}
					if pl.IDsFetchedOffset < pl.TrackCount && len(pl.TrackIDs) < pl.TrackCount {
						pl.IDsComplete = false
					}
				}
			}
			updated = append(updated, pl)
		}
		nextURL = res.Next
	}

	if len(updated) > 0 {
		c.mu.Lock()
		cd := c.loadCached()
		cd.Playlists = updated
		_ = c.saveCached(cd)
		c.mu.Unlock()
		return slices.Clone(updated), nil
	}
	return cachedPlaylists, nil
}

func effectiveFetchedOffset(pl Playlist) int {
	if pl.FetchedOffset > 0 {
		return pl.FetchedOffset
	}
	return len(pl.Tracks)
}

func trackIDsOf(tracks []Track) []string {
	if len(tracks) == 0 {
		return nil
	}
	ids := make([]string, 0, len(tracks))
	for _, t := range tracks {
		if t.ID != "" {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

type apiPlaylistTrackPage struct {
	Next  string `json:"next"`
	Total int    `json:"total"`
	Items []struct {
		Track struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DurationMS  int    `json:"duration_ms"`
			ExternalIDs struct {
				ISRC string `json:"isrc"`
			} `json:"external_ids"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name   string `json:"name"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
		} `json:"track"`
	} `json:"items"`
}

func parsePlaylistTrackPageItems(res apiPlaylistTrackPage) []Track {
	tracks := make([]Track, 0, len(res.Items))
	for _, it := range res.Items {
		t := it.Track
		if t.ID == "" {
			continue
		}
		var artists []string
		for _, a := range t.Artists {
			artists = append(artists, a.Name)
		}
		var cover string
		if len(t.Album.Images) > 0 {
			cover = t.Album.Images[0].URL
		}
		tracks = append(tracks, Track{
			ID:       t.ID,
			Title:    t.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    t.Album.Name,
			ISRC:     t.ExternalIDs.ISRC,
			Duration: time.Duration(t.DurationMS) * time.Millisecond,
			CoverURL: cover,
		})
	}
	return tracks
}

// PlaylistPage returns a paginated track slice for a playlist, serving cached slices when present
// and appending newly fetched pages into the local cache.
func (c *Catalog) PlaylistPage(playlistID string, offset, limit int) (PageResult, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	c.mu.Lock()
	cached := c.loadCached()
	var cachedPl *Playlist
	for i := range cached.Playlists {
		if cached.Playlists[i].ID == playlistID {
			plCopy := cached.Playlists[i]
			cachedPl = &plCopy
			break
		}
	}
	c.mu.Unlock()

	if cachedPl != nil {
		if offset < len(cachedPl.Tracks) {
			end := min(len(cachedPl.Tracks), offset+limit)
			nextOff := end
			if end == len(cachedPl.Tracks) && !cachedPl.TracksComplete {
				nextOff = effectiveFetchedOffset(*cachedPl)
			}
			total := cachedPl.TrackCount
			if total < len(cachedPl.Tracks) {
				total = len(cachedPl.Tracks)
			}
			return PageResult{
				Tracks:     slices.Clone(cachedPl.Tracks[offset:end]),
				Total:      total,
				NextOffset: nextOff,
				Complete:   cachedPl.TracksComplete && end == len(cachedPl.Tracks),
			}, nil
		}
		if cachedPl.TracksComplete {
			total := cachedPl.TrackCount
			if total < len(cachedPl.Tracks) {
				total = len(cachedPl.Tracks)
			}
			return PageResult{
				Total:      total,
				NextOffset: effectiveFetchedOffset(*cachedPl),
				Complete:   true,
			}, nil
		}
		if fOff := effectiveFetchedOffset(*cachedPl); fOff > offset {
			offset = fOff
		}
	}

	pageURL := fmt.Sprintf("%s/playlists/%s/tracks?offset=%d&limit=%d", c.apiBase, url.PathEscape(playlistID), offset, limit)
	req, err := c.newAuthorizedRequest(http.MethodGet, pageURL)
	if err != nil {
		return PageResult{}, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return PageResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return PageResult{}, fmt.Errorf("fetch playlist tracks status: %d", resp.StatusCode)
	}
	var res apiPlaylistTrackPage
	err = json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()
	if err != nil {
		return PageResult{}, err
	}

	pageTracks := parsePlaylistTrackPageItems(res)
	nextOff := offset + len(res.Items)
	complete := res.Next == "" || len(res.Items) == 0
	total := res.Total

	c.mu.Lock()
	cd := c.loadCached()
	cd.Playlists = slices.Clone(cd.Playlists)
	for i := range cd.Playlists {
		if cd.Playlists[i].ID == playlistID {
			pl := &cd.Playlists[i]
			if offset == 0 && len(pl.Tracks) == 0 {
				pl.Tracks = slices.Clone(pageTracks)
			} else if offset >= effectiveFetchedOffset(*pl) || len(pl.Tracks) == 0 {
				pl.Tracks = append(slices.Clone(pl.Tracks), pageTracks...)
			}
			pl.FetchedOffset = max(pl.FetchedOffset, nextOff)
			pl.TracksComplete = complete
			if total > 0 {
				pl.TrackCount = total
			} else if pl.TracksComplete {
				pl.TrackCount = len(pl.Tracks)
			}
			if pl.TracksComplete && !pl.IDsComplete {
				pl.TrackIDs = trackIDsOf(pl.Tracks)
				pl.IDsFetchedOffset = pl.FetchedOffset
				pl.IDsComplete = true
			}
			if total == 0 {
				total = pl.TrackCount
			}
			_ = c.saveCached(cd)
			break
		}
	}
	c.mu.Unlock()

	if total == 0 && complete {
		total = len(pageTracks)
	}
	return PageResult{
		Tracks:     pageTracks,
		Total:      total,
		NextOffset: nextOff,
		Complete:   complete,
	}, nil
}

// PlaylistTracks returns all tracks for a playlist, resuming from any partially cached prefix.
func (c *Catalog) PlaylistTracks(playlistID string) ([]Track, error) {
	c.mu.Lock()
	cached := c.loadCached()
	var acc []Track
	startOff := 0
	for _, pl := range cached.Playlists {
		if pl.ID == playlistID {
			if pl.TracksComplete {
				c.mu.Unlock()
				return slices.Clone(pl.Tracks), nil
			}
			acc = slices.Clone(pl.Tracks)
			startOff = effectiveFetchedOffset(pl)
			break
		}
	}
	c.mu.Unlock()

	off := startOff
	for {
		p, err := c.PlaylistPage(playlistID, off, 100)
		if err != nil {
			if len(acc) > 0 {
				return acc, nil
			}
			return nil, err
		}
		acc = append(acc, p.Tracks...)
		if p.Complete || p.NextOffset <= off {
			break
		}
		off = p.NextOffset
	}
	return acc, nil
}

// PlaylistTrackIDs returns all playable track IDs for a playlist using lightweight Spotify fields queries,
// streaming newly fetched ID batches to onBatch. If onBatch returns false or ctx cancels, hydration stops early.
func (c *Catalog) PlaylistTrackIDs(ctx context.Context, playlistID string, onBatch func([]string) bool) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	cached := c.loadCached()
	var ids []string
	offset := 0
	found := false
	for i := range cached.Playlists {
		if cached.Playlists[i].ID == playlistID {
			found = true
			pl := cached.Playlists[i]
			if pl.IDsComplete {
				out := slices.Clone(pl.TrackIDs)
				c.mu.Unlock()
				return out, nil
			}
			if pl.TracksComplete {
				out := trackIDsOf(pl.Tracks)
				cd := c.loadCached()
				cd.Playlists[i].TrackIDs = slices.Clone(out)
				cd.Playlists[i].IDsFetchedOffset = effectiveFetchedOffset(pl)
				cd.Playlists[i].IDsComplete = true
				_ = c.saveCached(cd)
				c.mu.Unlock()
				return out, nil
			}
			if len(pl.TrackIDs) > 0 {
				ids = slices.Clone(pl.TrackIDs)
				offset = pl.IDsFetchedOffset
				if offset == 0 {
					offset = len(pl.TrackIDs)
				}
			} else if len(pl.Tracks) > 0 {
				ids = trackIDsOf(pl.Tracks)
				offset = effectiveFetchedOffset(pl)
			}
			break
		}
	}
	c.mu.Unlock()

	_ = found
	const fields = "next,total,items(track(id))"
	pages := 0
	for {
		if err := ctx.Err(); err != nil {
			c.savePlaylistIDs(playlistID, ids, offset, 0, false)
			return ids, err
		}
		pageURL := fmt.Sprintf("%s/playlists/%s/tracks?offset=%d&limit=100&fields=%s",
			c.apiBase, url.PathEscape(playlistID), offset, url.QueryEscape(fields))
		req, err := c.newAuthorizedRequestWithContext(ctx, http.MethodGet, pageURL)
		if err != nil {
			c.savePlaylistIDs(playlistID, ids, offset, 0, false)
			if len(ids) > 0 {
				return ids, err
			}
			return nil, err
		}
		resp, err := c.client.Do(req)
		if err != nil {
			c.savePlaylistIDs(playlistID, ids, offset, 0, false)
			return ids, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			c.savePlaylistIDs(playlistID, ids, offset, 0, false)
			return ids, fmt.Errorf("fetch playlist track ids status: %d", resp.StatusCode)
		}
		var res struct {
			Next  string `json:"next"`
			Total int    `json:"total"`
			Items []struct {
				Track struct {
					ID string `json:"id"`
				} `json:"track"`
			} `json:"items"`
		}
		err = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if err != nil {
			c.savePlaylistIDs(playlistID, ids, offset, 0, false)
			return ids, err
		}

		var batch []string
		for _, it := range res.Items {
			if it.Track.ID != "" {
				batch = append(batch, it.Track.ID)
			}
		}
		ids = append(ids, batch...)
		offset += len(res.Items)
		pages++
		done := res.Next == "" || len(res.Items) == 0

		if done || pages%5 == 0 {
			c.savePlaylistIDs(playlistID, ids, offset, res.Total, done)
		}
		if len(batch) > 0 && onBatch != nil {
			if !onBatch(batch) {
				c.savePlaylistIDs(playlistID, ids, offset, res.Total, done)
				return ids, nil
			}
		}
		if done {
			break
		}
	}
	return ids, nil
}

func (c *Catalog) savePlaylistIDs(playlistID string, ids []string, offset, total int, complete bool) {
	if len(ids) == 0 && !complete {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()
	cd.Playlists = slices.Clone(cd.Playlists)
	for i := range cd.Playlists {
		if cd.Playlists[i].ID == playlistID {
			cd.Playlists[i].TrackIDs = slices.Clone(ids)
			cd.Playlists[i].IDsFetchedOffset = offset
			if total > 0 {
				cd.Playlists[i].TrackCount = total
			}
			if complete {
				cd.Playlists[i].IDsComplete = true
			}
			_ = c.saveCached(cd)
			return
		}
	}
}

// LikedPage returns a paginated slice of the user's Saved Tracks ("Liked Songs").
// When offset == 0 and syncHead is true, it checks Spotify for newly liked or removed tracks.
func (c *Catalog) LikedPage(offset, limit int, syncHead bool) (PageResult, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	if offset == 0 && syncHead {
		if err := c.syncLikedHead(); err != nil {
			c.mu.Lock()
			cd := c.loadCached()
			if len(cd.LikedTracks) > 0 {
				end := min(limit, len(cd.LikedTracks))
				total := max(cd.LikedTotal, len(cd.LikedTracks))
				nextOff := end
				if end == len(cd.LikedTracks) && !cd.LikedTracksComplete {
					nextOff = max(cd.LikedFetchedOffset, len(cd.LikedTracks))
				}
				res := PageResult{
					Tracks:     slices.Clone(cd.LikedTracks[:end]),
					Total:      total,
					NextOffset: nextOff,
					Complete:   cd.LikedTracksComplete && end == len(cd.LikedTracks),
				}
				c.mu.Unlock()
				return res, nil
			}
			c.mu.Unlock()
			return PageResult{}, err
		}
	}

	c.mu.Lock()
	cd := c.loadCached()
	if offset < len(cd.LikedTracks) {
		end := min(len(cd.LikedTracks), offset+limit)
		nextOff := end
		if end == len(cd.LikedTracks) && !cd.LikedTracksComplete {
			nextOff = max(cd.LikedFetchedOffset, len(cd.LikedTracks))
		}
		total := max(cd.LikedTotal, len(cd.LikedTracks))
		res := PageResult{
			Tracks:     slices.Clone(cd.LikedTracks[offset:end]),
			Total:      total,
			NextOffset: nextOff,
			Complete:   cd.LikedTracksComplete && end == len(cd.LikedTracks),
		}
		c.mu.Unlock()
		return res, nil
	}
	if cd.LikedTracksComplete {
		total := max(cd.LikedTotal, len(cd.LikedTracks))
		nextOff := max(cd.LikedFetchedOffset, len(cd.LikedTracks))
		c.mu.Unlock()
		return PageResult{Total: total, NextOffset: nextOff, Complete: true}, nil
	}
	fetchOff := max(offset, cd.LikedFetchedOffset)
	c.mu.Unlock()

	pageURL := fmt.Sprintf("%s/me/tracks?offset=%d&limit=%d", c.apiBase, fetchOff, limit)
	res, pageTracks, err := c.fetchLikedPage(context.Background(), pageURL)
	if err != nil {
		return PageResult{}, err
	}
	nextOff := fetchOff + len(res.Items)
	complete := res.Next == "" || len(res.Items) == 0

	c.mu.Lock()
	cd = c.loadCached()
	if fetchOff >= cd.LikedFetchedOffset || len(cd.LikedTracks) == 0 {
		cd.LikedTracks = append(slices.Clone(cd.LikedTracks), pageTracks...)
		cd.LikedFetchedOffset = nextOff
		if res.Total > 0 {
			cd.LikedTotal = res.Total
		} else if cd.LikedTotal < len(cd.LikedTracks) {
			cd.LikedTotal = len(cd.LikedTracks)
		}
		cd.LikedTracksComplete = complete
		if complete {
			cd.LikedTrackIDs = trackIDsOf(cd.LikedTracks)
			cd.LikedIDsComplete = true
		}
		_ = c.saveCached(cd)
	}
	total := max(cd.LikedTotal, len(cd.LikedTracks))
	c.mu.Unlock()

	return PageResult{
		Tracks:     pageTracks,
		Total:      total,
		NextOffset: nextOff,
		Complete:   complete,
	}, nil
}

func (c *Catalog) fetchLikedPage(ctx context.Context, pageURL string) (apiLikedTrackPage, []Track, error) {
	req, err := c.newAuthorizedRequestWithContext(ctx, http.MethodGet, pageURL)
	if err != nil {
		return apiLikedTrackPage{}, nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return apiLikedTrackPage{}, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return apiLikedTrackPage{}, nil, fmt.Errorf("fetch liked tracks status: %d", resp.StatusCode)
	}
	var res apiLikedTrackPage
	err = json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()
	if err != nil {
		return apiLikedTrackPage{}, nil, err
	}
	tracks := make([]Track, 0, len(res.Items))
	for _, it := range res.Items {
		t := it.Track
		if t.ID == "" {
			continue
		}
		var artists []string
		for _, a := range t.Artists {
			artists = append(artists, a.Name)
		}
		var cover string
		if len(t.Album.Images) > 0 {
			cover = t.Album.Images[0].URL
		}
		tracks = append(tracks, Track{
			ID:       t.ID,
			Title:    t.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    t.Album.Name,
			ISRC:     t.ExternalIDs.ISRC,
			Duration: time.Duration(t.DurationMS) * time.Millisecond,
			CoverURL: cover,
		})
	}
	return res, tracks, nil
}

type apiLikedTrackPage struct {
	Next  string `json:"next"`
	Total int    `json:"total"`
	Items []struct {
		Track struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DurationMS  int    `json:"duration_ms"`
			ExternalIDs struct {
				ISRC string `json:"isrc"`
			} `json:"external_ids"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name   string `json:"name"`
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			} `json:"album"`
		} `json:"track"`
	} `json:"items"`
}

func (c *Catalog) syncLikedHead() error {
	res, page0, err := c.fetchLikedPage(context.Background(), c.apiBase+"/me/tracks?offset=0&limit=50")
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()

	firstAPIID := ""
	if len(page0) > 0 {
		firstAPIID = page0[0].ID
	}
	firstCachedID := ""
	if len(cd.LikedTracks) > 0 {
		firstCachedID = cd.LikedTracks[0].ID
	}

	// Legacy migration: old catalog.json stored up to 500 tracks with LikedTotal == 0.
	if cd.LikedTotal == 0 && len(cd.LikedTracks) > 0 {
		if firstAPIID != "" && firstAPIID == firstCachedID && (res.Total == 0 || res.Total >= len(cd.LikedTracks)) {
			cd.LikedTotal = res.Total
			if cd.LikedTotal == 0 {
				cd.LikedTotal = len(cd.LikedTracks)
			}
			cd.LikedFetchedOffset = len(cd.LikedTracks)
			cd.LikedTracksComplete = res.Total <= len(cd.LikedTracks)
			if cd.LikedTracksComplete {
				cd.LikedTrackIDs = trackIDsOf(cd.LikedTracks)
				cd.LikedIDsComplete = true
			}
			_ = c.saveCached(cd)
			return nil
		}
	}

	// Unchanged head and identical total: keep cached liked collection untouched.
	if len(cd.LikedTracks) > 0 && cd.LikedTotal > 0 && res.Total == cd.LikedTotal && firstAPIID != "" && firstAPIID == firstCachedID {
		return nil
	}

	// Delta addition: only new tracks were prepended at the top within page 0.
	if len(cd.LikedTracks) > 0 && cd.LikedTotal > 0 && res.Total > cd.LikedTotal {
		delta := -1
		for i, tr := range page0 {
			if tr.ID == firstCachedID {
				delta = i
				break
			}
		}
		if delta > 0 && cd.LikedTotal+delta == res.Total {
			prepended := append(slices.Clone(page0[:delta]), cd.LikedTracks...)
			cd.LikedTracks = prepended
			cd.LikedTotal = res.Total
			cd.LikedFetchedOffset += delta
			if len(cd.LikedTrackIDs) > 0 {
				cd.LikedTrackIDs = append(trackIDsOf(page0[:delta]), cd.LikedTrackIDs...)
			}
			_ = c.saveCached(cd)
			return nil
		}
	}

	// Initial population, unlike detected, or >50 new tracks: reset cleanly from page 0.
	cd.LikedTracks = page0
	cd.LikedTotal = res.Total
	if cd.LikedTotal == 0 {
		cd.LikedTotal = len(page0)
	}
	cd.LikedFetchedOffset = len(res.Items)
	cd.LikedTracksComplete = res.Next == "" || len(res.Items) == 0
	cd.LikedTrackIDs = trackIDsOf(page0)
	cd.LikedIDsComplete = cd.LikedTracksComplete
	_ = c.saveCached(cd)
	return nil
}

// LikedSongs fetches all of the user's saved tracks ("Liked Songs") without any artificial cap.
func (c *Catalog) LikedSongs() ([]Track, error) {
	p0, err := c.LikedPage(0, 50, true)
	if err != nil {
		return nil, err
	}
	if p0.Complete {
		c.mu.Lock()
		all := slices.Clone(c.loadCached().LikedTracks)
		c.mu.Unlock()
		return all, nil
	}

	c.mu.Lock()
	cd := c.loadCached()
	off := max(cd.LikedFetchedOffset, len(cd.LikedTracks))
	complete := cd.LikedTracksComplete
	c.mu.Unlock()

	for !complete {
		p, err := c.LikedPage(off, 50, false)
		if err != nil {
			break
		}
		if p.Complete || p.NextOffset <= off {
			break
		}
		off = p.NextOffset
	}
	c.mu.Lock()
	all := slices.Clone(c.loadCached().LikedTracks)
	c.mu.Unlock()
	return all, nil
}

// LikedTrackIDs streams remaining liked song IDs (after any already cached prefix) to onBatch.
func (c *Catalog) LikedTrackIDs(ctx context.Context, onBatch func([]string) bool) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	cd := c.loadCached()
	if cd.LikedTracksComplete {
		ids := trackIDsOf(cd.LikedTracks)
		c.mu.Unlock()
		return ids, nil
	}
	ids := trackIDsOf(cd.LikedTracks)
	offset := max(cd.LikedFetchedOffset, len(cd.LikedTracks))
	c.mu.Unlock()

	pages := 0
	for {
		if err := ctx.Err(); err != nil {
			return ids, err
		}
		pageURL := fmt.Sprintf("%s/me/tracks?offset=%d&limit=50", c.apiBase, offset)
		res, pageTracks, err := c.fetchLikedPage(ctx, pageURL)
		if err != nil {
			return ids, err
		}
		batch := trackIDsOf(pageTracks)
		ids = append(ids, batch...)
		offset += len(res.Items)
		pages++
		done := res.Next == "" || len(res.Items) == 0

		c.mu.Lock()
		cur := c.loadCached()
		if offset > cur.LikedFetchedOffset {
			cur.LikedTracks = append(slices.Clone(cur.LikedTracks), pageTracks...)
			cur.LikedFetchedOffset = offset
			if res.Total > 0 {
				cur.LikedTotal = res.Total
			}
			cur.LikedTracksComplete = done
			if done {
				cur.LikedTrackIDs = trackIDsOf(cur.LikedTracks)
				cur.LikedIDsComplete = true
			}
			if done || pages%5 == 0 {
				_ = c.saveCached(cur)
			} else {
				c.cachedMem = &cur
			}
		}
		c.mu.Unlock()

		if len(batch) > 0 && onBatch != nil {
			if !onBatch(batch) {
				c.mu.Lock()
				_ = c.saveCached(c.loadCached())
				c.mu.Unlock()
				return ids, nil
			}
		}
		if done {
			break
		}
	}
	return ids, nil
}

// Search searches Spotify catalog for tracks.
func (c *Catalog) Search(query string) ([]Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	req, err := c.newAuthorizedRequest(http.MethodGet, fmt.Sprintf("%s/search?q=%s&type=track&limit=50", c.apiBase, url.QueryEscape(query)))
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spotify search status: %d", resp.StatusCode)
	}

	var res struct {
		Tracks struct {
			Items []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DurationMS  int    `json:"duration_ms"`
				ExternalIDs struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var tracks []Track
	for _, it := range res.Tracks.Items {
		if it.ID == "" {
			continue
		}
		var artists []string
		for _, a := range it.Artists {
			artists = append(artists, a.Name)
		}
		var cover string
		if len(it.Album.Images) > 0 {
			cover = it.Album.Images[0].URL
		}
		tracks = append(tracks, Track{
			ID:       it.ID,
			Title:    it.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    it.Album.Name,
			ISRC:     it.ExternalIDs.ISRC,
			Duration: time.Duration(it.DurationMS) * time.Millisecond,
			CoverURL: cover,
		})
	}
	return tracks, nil
}

// SavedAlbums fetches all saved albums from the user's library without pagination caps.
func (c *Catalog) SavedAlbums() ([]Playlist, error) {
	c.mu.Lock()
	cached := c.loadCached()
	cachedAlbums := slices.Clone(cached.Albums)
	c.mu.Unlock()

	cachedFirstID := ""
	if len(cachedAlbums) > 0 {
		cachedFirstID = cachedAlbums[0].ID
	}

	var albums []Playlist
	nextURL := c.apiBase + "/me/albums?limit=50"

	for nextURL != "" {
		req, err := c.newAuthorizedRequest(http.MethodGet, nextURL)
		if err != nil {
			if len(albums) > 0 {
				break
			}
			if len(cachedAlbums) > 0 {
				return cachedAlbums, nil
			}
			return nil, err
		}

		resp, err := c.client.Do(req)
		if err != nil {
			if len(albums) > 0 {
				break
			}
			if len(cachedAlbums) > 0 {
				return cachedAlbums, nil
			}
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if len(albums) > 0 {
				break
			}
			if len(cachedAlbums) > 0 {
				return cachedAlbums, nil
			}
			return nil, fmt.Errorf("fetch albums status: %d", resp.StatusCode)
		}

		var res struct {
			Next  string `json:"next"`
			Total int    `json:"total"`
			Items []struct {
				Album struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Tracks struct {
						Total int `json:"total"`
						Items []struct {
							ID         string `json:"id"`
							Name       string `json:"name"`
							DurationMS int    `json:"duration_ms"`
							Artists    []struct {
								Name string `json:"name"`
							} `json:"artists"`
						} `json:"items"`
					} `json:"tracks"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"items"`
		}
		err = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if err != nil {
			if len(albums) > 0 {
				break
			}
			return nil, err
		}

		if len(albums) == 0 && len(res.Items) > 0 && res.Total == len(cachedAlbums) && res.Items[0].Album.ID == cachedFirstID {
			return cachedAlbums, nil
		}

		hitCached := false
		for _, item := range res.Items {
			al := item.Album
			if al.ID == "" {
				continue
			}
			if cachedFirstID != "" && al.ID == cachedFirstID {
				albums = append(albums, cachedAlbums...)
				hitCached = true
				break
			}
			var cover string
			if len(al.Images) > 0 {
				cover = al.Images[0].URL
			}
			var tracks []Track
			for _, it := range al.Tracks.Items {
				var artists []string
				for _, a := range it.Artists {
					artists = append(artists, a.Name)
				}
				tracks = append(tracks, Track{
					ID:       it.ID,
					Title:    it.Name,
					Artist:   strings.Join(artists, ", "),
					Album:    al.Name,
					Duration: time.Duration(it.DurationMS) * time.Millisecond,
					CoverURL: cover,
				})
			}
			complete := al.Tracks.Total <= len(al.Tracks.Items)
			pl := Playlist{
				ID:             al.ID,
				Name:           al.Name,
				TrackCount:     al.Tracks.Total,
				Tracks:         tracks,
				FetchedOffset:  len(al.Tracks.Items),
				TracksComplete: complete,
			}
			if complete {
				pl.TrackIDs = trackIDsOf(tracks)
				pl.IDsFetchedOffset = len(al.Tracks.Items)
				pl.IDsComplete = true
			}
			albums = append(albums, pl)
		}
		if hitCached {
			break
		}
		nextURL = res.Next
	}
	if len(albums) > 0 {
		c.mu.Lock()
		cd := c.loadCached()
		cd.Albums = albums
		_ = c.saveCached(cd)
		c.mu.Unlock()
	}
	return albums, nil
}

type apiAlbumMeta struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
	Tracks struct {
		Total int `json:"total"`
	} `json:"tracks"`
}

type apiAlbumTrackPage struct {
	Next  string `json:"next"`
	Total int    `json:"total"`
	Items []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		DurationMS int    `json:"duration_ms"`
		Artists    []struct {
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"items"`
}

func (c *Catalog) fetchAlbumHeader(ctx context.Context, albumID string) (name, cover string, total int, err error) {
	req, err := c.newAuthorizedRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/albums/%s", c.apiBase, url.PathEscape(albumID)))
	if err != nil {
		return "", "", 0, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("fetch album status: %d", resp.StatusCode)
	}
	var res apiAlbumMeta
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", 0, err
	}
	if len(res.Images) > 0 {
		cover = res.Images[0].URL
	}
	return res.Name, cover, res.Tracks.Total, nil
}

// AlbumPage returns a paginated track slice for an album via /albums/{id}/tracks,
// serving cached inline/previously fetched tracks first and updating the album cache.
func (c *Catalog) AlbumPage(albumID string, offset, limit int) (PageResult, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	c.mu.Lock()
	cd := c.loadCached()
	var cachedAl *Playlist
	for i := range cd.Albums {
		if cd.Albums[i].ID == albumID {
			alCopy := cd.Albums[i]
			cachedAl = &alCopy
			break
		}
	}
	c.mu.Unlock()

	albumName := ""
	coverURL := ""
	if cachedAl != nil {
		albumName = cachedAl.Name
		if len(cachedAl.Tracks) > 0 {
			coverURL = cachedAl.Tracks[0].CoverURL
		}
		isComplete := cachedAl.TracksComplete || (cachedAl.TrackCount > 0 && len(cachedAl.Tracks) >= cachedAl.TrackCount)
		if offset < len(cachedAl.Tracks) {
			end := min(len(cachedAl.Tracks), offset+limit)
			nextOff := end
			if end == len(cachedAl.Tracks) && !isComplete {
				nextOff = effectiveFetchedOffset(*cachedAl)
			}
			total := max(cachedAl.TrackCount, len(cachedAl.Tracks))
			return PageResult{
				Tracks:     slices.Clone(cachedAl.Tracks[offset:end]),
				Total:      total,
				NextOffset: nextOff,
				Complete:   isComplete && end == len(cachedAl.Tracks),
			}, nil
		}
		if isComplete {
			total := max(cachedAl.TrackCount, len(cachedAl.Tracks))
			return PageResult{
				Total:      total,
				NextOffset: effectiveFetchedOffset(*cachedAl),
				Complete:   true,
			}, nil
		}
		if fOff := effectiveFetchedOffset(*cachedAl); fOff > offset {
			offset = fOff
		}
	}

	if albumName == "" {
		if name, cov, _, err := c.fetchAlbumHeader(context.Background(), albumID); err == nil {
			albumName = name
			coverURL = cov
		}
	}

	pageURL := fmt.Sprintf("%s/albums/%s/tracks?offset=%d&limit=%d", c.apiBase, url.PathEscape(albumID), offset, limit)
	req, err := c.newAuthorizedRequest(http.MethodGet, pageURL)
	if err != nil {
		return PageResult{}, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return PageResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return PageResult{}, fmt.Errorf("fetch album tracks status: %d", resp.StatusCode)
	}
	var res apiAlbumTrackPage
	err = json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()
	if err != nil {
		return PageResult{}, err
	}

	pageTracks := make([]Track, 0, len(res.Items))
	for _, it := range res.Items {
		if it.ID == "" {
			continue
		}
		var artists []string
		for _, a := range it.Artists {
			artists = append(artists, a.Name)
		}
		pageTracks = append(pageTracks, Track{
			ID:       it.ID,
			Title:    it.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    albumName,
			Duration: time.Duration(it.DurationMS) * time.Millisecond,
			CoverURL: coverURL,
		})
	}

	nextOff := offset + len(res.Items)
	complete := res.Next == "" || len(res.Items) == 0
	total := res.Total

	c.mu.Lock()
	cur := c.loadCached()
	cur.Albums = slices.Clone(cur.Albums)
	for i := range cur.Albums {
		if cur.Albums[i].ID == albumID {
			al := &cur.Albums[i]
			if offset == 0 && len(al.Tracks) == 0 {
				al.Tracks = slices.Clone(pageTracks)
			} else if offset >= effectiveFetchedOffset(*al) || len(al.Tracks) == 0 {
				al.Tracks = append(slices.Clone(al.Tracks), pageTracks...)
			}
			al.FetchedOffset = max(al.FetchedOffset, nextOff)
			al.TracksComplete = complete
			if total > 0 {
				al.TrackCount = total
			} else if complete {
				al.TrackCount = len(al.Tracks)
			}
			if complete {
				al.TrackIDs = trackIDsOf(al.Tracks)
				al.IDsFetchedOffset = al.FetchedOffset
				al.IDsComplete = true
			}
			if total == 0 {
				total = al.TrackCount
			}
			_ = c.saveCached(cur)
			break
		}
	}
	c.mu.Unlock()

	if total == 0 && complete {
		total = len(pageTracks)
	}
	return PageResult{
		Tracks:     pageTracks,
		Total:      total,
		NextOffset: nextOff,
		Complete:   complete,
	}, nil
}

// AlbumTracks fetches all tracks for an album, resuming from any partially cached prefix.
func (c *Catalog) AlbumTracks(albumID string) ([]Track, error) {
	c.mu.Lock()
	cd := c.loadCached()
	var acc []Track
	startOff := 0
	for _, al := range cd.Albums {
		if al.ID == albumID {
			if al.TracksComplete || (al.TrackCount > 0 && len(al.Tracks) >= al.TrackCount) {
				c.mu.Unlock()
				return slices.Clone(al.Tracks), nil
			}
			acc = slices.Clone(al.Tracks)
			startOff = effectiveFetchedOffset(al)
			break
		}
	}
	c.mu.Unlock()

	off := startOff
	for {
		p, err := c.AlbumPage(albumID, off, 50)
		if err != nil {
			if len(acc) > 0 {
				return acc, nil
			}
			return nil, err
		}
		acc = append(acc, p.Tracks...)
		if p.Complete || p.NextOffset <= off {
			break
		}
		off = p.NextOffset
	}
	return acc, nil
}

// AlbumTrackIDs streams remaining track IDs for an album (after cached tracks) to onBatch.
func (c *Catalog) AlbumTrackIDs(ctx context.Context, albumID string, onBatch func([]string) bool) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	cd := c.loadCached()
	var ids []string
	off := 0
	for _, al := range cd.Albums {
		if al.ID == albumID {
			if al.IDsComplete {
				out := slices.Clone(al.TrackIDs)
				c.mu.Unlock()
				return out, nil
			}
			if al.TracksComplete || (al.TrackCount > 0 && len(al.Tracks) >= al.TrackCount) {
				out := trackIDsOf(al.Tracks)
				c.mu.Unlock()
				return out, nil
			}
			ids = trackIDsOf(al.Tracks)
			off = effectiveFetchedOffset(al)
			break
		}
	}
	c.mu.Unlock()

	for {
		if err := ctx.Err(); err != nil {
			return ids, err
		}
		p, err := c.AlbumPage(albumID, off, 50)
		if err != nil {
			return ids, err
		}
		batch := trackIDsOf(p.Tracks)
		ids = append(ids, batch...)
		if len(batch) > 0 && onBatch != nil {
			if !onBatch(batch) {
				return ids, nil
			}
		}
		if p.Complete || p.NextOffset <= off {
			break
		}
		off = p.NextOffset
	}
	return ids, nil
}

// TracksByIDs resolves metadata for multiple track IDs from cache first and batches missing
// IDs via Spotify's /tracks?ids= endpoint (up to 50 IDs per call).
func (c *Catalog) TracksByIDs(ids []string) ([]Track, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byID := make(map[string]Track, len(ids))
	var missing []string
	seenMissing := make(map[string]bool)

	for _, raw := range ids {
		id := ExtractTrackID(raw)
		if id == "" {
			continue
		}
		if tr, ok := c.findTrack(id); ok {
			byID[id] = tr
			continue
		}
		if !seenMissing[id] {
			seenMissing[id] = true
			missing = append(missing, id)
		}
	}

	var fetchedAll []Track
	for i := 0; i < len(missing); i += 50 {
		chunk := missing[i:min(len(missing), i+50)]
		apiURL := fmt.Sprintf("%s/tracks?ids=%s", c.apiBase, url.QueryEscape(strings.Join(chunk, ",")))
		req, err := c.newAuthorizedRequest(http.MethodGet, apiURL)
		if err != nil {
			if len(byID) > 0 {
				break
			}
			return nil, err
		}
		resp, err := c.client.Do(req)
		if err != nil {
			if len(byID) > 0 {
				break
			}
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if len(byID) > 0 {
				break
			}
			return nil, fmt.Errorf("fetch tracks status: %d", resp.StatusCode)
		}
		var res struct {
			Tracks []*struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DurationMS  int    `json:"duration_ms"`
				ExternalIDs struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"tracks"`
		}
		err = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		if err != nil {
			break
		}
		for _, t := range res.Tracks {
			if t == nil || t.ID == "" {
				continue
			}
			var artists []string
			for _, a := range t.Artists {
				artists = append(artists, a.Name)
			}
			var cover string
			if len(t.Album.Images) > 0 {
				cover = t.Album.Images[0].URL
			}
			tr := Track{
				ID:       t.ID,
				Title:    t.Name,
				Artist:   strings.Join(artists, ", "),
				Album:    t.Album.Name,
				ISRC:     t.ExternalIDs.ISRC,
				Duration: time.Duration(t.DurationMS) * time.Millisecond,
				CoverURL: cover,
			}
			byID[tr.ID] = tr
			fetchedAll = append(fetchedAll, tr)
		}
	}

	if len(fetchedAll) > 0 {
		c.mu.Lock()
		cd := c.loadCached()
		if cd.Tracks == nil {
			cd.Tracks = make(map[string]Track)
		} else {
			cd.Tracks = maps.Clone(cd.Tracks)
		}
		for _, tr := range fetchedAll {
			cd.Tracks[tr.ID] = tr
		}
		_ = c.saveCached(cd)
		c.mu.Unlock()
	}

	out := make([]Track, 0, len(ids))
	for _, raw := range ids {
		id := ExtractTrackID(raw)
		if tr, ok := byID[id]; ok {
			out = append(out, tr)
		}
	}
	return out, nil
}

// RecentlyPlayed returns recently played tracks from the user's Spotify account.
func (c *Catalog) RecentlyPlayed() ([]Track, error) {
	req, err := c.newAuthorizedRequest(http.MethodGet, c.apiBase+"/me/player/recently-played?limit=30")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recently played api status: %d", resp.StatusCode)
	}

	var res struct {
		Items []struct {
			Track struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DurationMS  int    `json:"duration_ms"`
				ExternalIDs struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"track"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var tracks []Track
	for _, it := range res.Items {
		t := it.Track
		if t.ID == "" || seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		var artists []string
		for _, a := range t.Artists {
			artists = append(artists, a.Name)
		}
		var cover string
		if len(t.Album.Images) > 0 {
			cover = t.Album.Images[0].URL
		}
		tracks = append(tracks, Track{
			ID:       t.ID,
			Title:    t.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    t.Album.Name,
			ISRC:     t.ExternalIDs.ISRC,
			Duration: time.Duration(t.DurationMS) * time.Millisecond,
			CoverURL: cover,
		})
	}
	return tracks, nil
}

// IsTrackLiked checks if the track is in the user's liked collection.
func (c *Catalog) IsTrackLiked(trackID string) bool {
	id := ExtractTrackID(trackID)
	if id == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()
	if slices.Contains(cd.LikedTrackIDs, id) {
		return true
	}
	for _, tr := range cd.LikedTracks {
		if tr.ID == id {
			return true
		}
	}
	return false
}

// LikeTrack adds a track to the user's saved tracks on Spotify ("Liked Songs") and updates local cache.
func (c *Catalog) LikeTrack(ctx context.Context, trackID string) error {
	id := ExtractTrackID(trackID)
	if id == "" {
		return fmt.Errorf("invalid track id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	apiURL := fmt.Sprintf("%s/me/tracks?ids=%s", c.apiBase, url.QueryEscape(id))
	req, err := c.newAuthorizedRequestWithContext(ctx, http.MethodPut, apiURL)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("like track status: %d", resp.StatusCode)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()
	if !slices.Contains(cd.LikedTrackIDs, id) {
		cd.LikedTrackIDs = append([]string{id}, cd.LikedTrackIDs...)
	}
	found := false
	for _, tr := range cd.LikedTracks {
		if tr.ID == id {
			found = true
			break
		}
	}
	if !found {
		tr, ok := findTrackInCatalog(cd, id)
		if !ok {
			tr = Track{ID: id}
		}
		cd.LikedTracks = append([]Track{tr}, cd.LikedTracks...)
		cd.LikedTotal++
		cd.LikedFetchedOffset++
	}
	_ = c.saveCached(cd)
	return nil
}

// UnlikeTrack removes a track from the user's saved tracks on Spotify and updates local cache.
func (c *Catalog) UnlikeTrack(ctx context.Context, trackID string) error {
	id := ExtractTrackID(trackID)
	if id == "" {
		return fmt.Errorf("invalid track id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	apiURL := fmt.Sprintf("%s/me/tracks?ids=%s", c.apiBase, url.QueryEscape(id))
	req, err := c.newAuthorizedRequestWithContext(ctx, http.MethodDelete, apiURL)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unlike track status: %d", resp.StatusCode)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()
	if idx := slices.Index(cd.LikedTrackIDs, id); idx >= 0 {
		cd.LikedTrackIDs = slices.Delete(cd.LikedTrackIDs, idx, idx+1)
	}
	for i, tr := range cd.LikedTracks {
		if tr.ID == id {
			cd.LikedTracks = slices.Delete(cd.LikedTracks, i, i+1)
			if cd.LikedTotal > 0 {
				cd.LikedTotal--
			}
			if cd.LikedFetchedOffset > 0 {
				cd.LikedFetchedOffset--
			}
			break
		}
	}
	_ = c.saveCached(cd)
	return nil
}

// AddTrackToPlaylist adds a track to a specific Spotify playlist and updates local cache.
func (c *Catalog) AddTrackToPlaylist(ctx context.Context, playlistID, trackID string) error {
	pid := strings.TrimSpace(playlistID)
	tid := ExtractTrackID(trackID)
	if pid == "" || tid == "" {
		return fmt.Errorf("invalid playlist or track id")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	apiURL := fmt.Sprintf("%s/playlists/%s/tracks", c.apiBase, url.PathEscape(pid))
	bodyData, err := json.Marshal(map[string]any{
		"uris": []string{"spotify:track:" + tid},
	})
	if err != nil {
		return err
	}
	req, err := c.newAuthorizedRequestWithContext(ctx, http.MethodPost, apiURL)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(bytes.NewReader(bodyData))
	req.ContentLength = int64(len(bodyData))

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("add track to playlist status: %d", resp.StatusCode)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	cd := c.loadCached()
	cd.Playlists = slices.Clone(cd.Playlists)
	for i := range cd.Playlists {
		if cd.Playlists[i].ID == pid {
			pl := &cd.Playlists[i]
			pl.TrackIDs = append(pl.TrackIDs, tid)
			pl.TrackCount++
			if tr, ok := findTrackInCatalog(cd, tid); ok {
				pl.Tracks = append(pl.Tracks, tr)
			}
			_ = c.saveCached(cd)
			break
		}
	}
	return nil
}

// Recommendations fetches recommended tracks from Spotify based on seed tracks,
// or falls back to diverse cached tracks when offline or if the API is unavailable.
func (c *Catalog) Recommendations(ctx context.Context, seedTracks []string, limit int) ([]Track, error) {
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	var validSeeds []string
	seenSeeds := make(map[string]bool)
	for _, raw := range seedTracks {
		id := ExtractTrackID(raw)
		if id != "" && !seenSeeds[id] {
			seenSeeds[id] = true
			validSeeds = append(validSeeds, id)
			if len(validSeeds) >= 5 {
				break
			}
		}
	}

	var apiErr error
	if len(validSeeds) > 0 {
		apiURL := fmt.Sprintf("%s/recommendations?seed_tracks=%s&limit=%d",
			c.apiBase,
			url.QueryEscape(strings.Join(validSeeds, ",")),
			limit,
		)
		req, err := c.newAuthorizedRequestWithContext(ctx, http.MethodGet, apiURL)
		if err == nil {
			resp, doErr := c.client.Do(req)
			if doErr == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var res struct {
						Tracks []*struct {
							ID          string `json:"id"`
							Name        string `json:"name"`
							DurationMS  int    `json:"duration_ms"`
							ExternalIDs struct {
								ISRC string `json:"isrc"`
							} `json:"external_ids"`
							Artists []struct {
								Name string `json:"name"`
							} `json:"artists"`
							Album struct {
								Name   string `json:"name"`
								Images []struct {
									URL string `json:"url"`
								} `json:"images"`
							} `json:"album"`
						} `json:"tracks"`
					}
					if decErr := json.NewDecoder(resp.Body).Decode(&res); decErr == nil {
						var out []Track
						for _, t := range res.Tracks {
							if t == nil || t.ID == "" {
								continue
							}
							var artists []string
							for _, a := range t.Artists {
								artists = append(artists, a.Name)
							}
							var cover string
							if len(t.Album.Images) > 0 {
								cover = t.Album.Images[0].URL
							}
							tr := Track{
								ID:       t.ID,
								Title:    t.Name,
								Artist:   strings.Join(artists, ", "),
								Album:    t.Album.Name,
								ISRC:     t.ExternalIDs.ISRC,
								Duration: time.Duration(t.DurationMS) * time.Millisecond,
								CoverURL: cover,
							}
							out = append(out, tr)
						}
						if len(out) > 0 {
							c.mu.Lock()
							cd := c.loadCached()
							if cd.Tracks == nil {
								cd.Tracks = make(map[string]Track)
							} else {
								cd.Tracks = maps.Clone(cd.Tracks)
							}
							for _, tr := range out {
								cd.Tracks[tr.ID] = tr
							}
							_ = c.saveCached(cd)
							c.mu.Unlock()
							return out, nil
						}
					} else {
						apiErr = decErr
					}
				} else {
					apiErr = fmt.Errorf("recommendations status: %d", resp.StatusCode)
				}
			} else {
				apiErr = doErr
			}
		} else {
			apiErr = err
		}
	}

	// Fallback to local cached tracks (excluding seeds)
	c.mu.Lock()
	cd := c.loadCached()
	c.mu.Unlock()

	var fallback []Track
	fallbackSeen := make(map[string]bool)
	for s := range seenSeeds {
		fallbackSeen[s] = true
	}

	// 1. Collect from liked tracks
	for _, tr := range cd.LikedTracks {
		if tr.ID != "" && !fallbackSeen[tr.ID] {
			fallbackSeen[tr.ID] = true
			fallback = append(fallback, tr)
			if len(fallback) >= limit {
				return fallback, nil
			}
		}
	}

	// 2. Collect from cached playlists
	for _, pl := range cd.Playlists {
		for _, tr := range pl.Tracks {
			if tr.ID != "" && !fallbackSeen[tr.ID] {
				fallbackSeen[tr.ID] = true
				fallback = append(fallback, tr)
				if len(fallback) >= limit {
					return fallback, nil
				}
			}
		}
	}

	// 3. Collect from general catalog tracks
	for _, tr := range cd.Tracks {
		if tr.ID != "" && !fallbackSeen[tr.ID] {
			fallbackSeen[tr.ID] = true
			fallback = append(fallback, tr)
			if len(fallback) >= limit {
				return fallback, nil
			}
		}
	}

	if len(fallback) > 0 {
		return fallback, nil
	}
	if apiErr != nil {
		return nil, apiErr
	}
	return nil, errors.New("no recommendations available")
}
