package spotify

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juanvillacortac/audiofilus/internal/decode"
)

// IsSpotify checks whether a URI is a Spotify track URI (spotify:track:... or spotify://track/...).
func IsSpotify(uri string) bool {
	return strings.HasPrefix(uri, "spotify:track:") || strings.HasPrefix(uri, "spotify://track/")
}

// IsSmartTrack checks whether a Spotify URI has been injected by Smart Shuffle.
func IsSmartTrack(uri string) bool {
	return strings.Contains(uri, "smart=1")
}

// ExtractTrackID extracts the raw Spotify track ID from a URI.
func ExtractTrackID(uri string) string {
	if strings.HasPrefix(uri, "spotify://track/") {
		uri = strings.TrimPrefix(uri, "spotify://track/")
	} else if strings.HasPrefix(uri, "spotify:track:") {
		uri = strings.TrimPrefix(uri, "spotify:track:")
	}
	if idx := strings.IndexAny(uri, "?#"); idx != -1 {
		uri = uri[:idx]
	}
	return uri
}

// Manager coordinates authentication, catalog sync, and streaming resolution.
type Manager struct {
	basePath           string
	dataPath           string
	Auth               *AuthManager
	Catalog            *Catalog
	Resolver           *StreamResolver
	client             *http.Client
	mu                 sync.Mutex
	tracks             map[string]Track
	localPaths         map[string]string
	coverInflight      map[string]bool
	coverFailedAt      map[string]time.Time
	OnMetadataResolved func(Track)
}

// NewManager initializes a new Spotify manager for the daemon service.
func NewManager(basePath, dataPath string, clientID, clientSecret string) *Manager {
	if basePath == "" && dataPath != "" {
		basePath = filepath.Dir(dataPath)
	}
	auth := NewAuthManager(dataPath, clientID, clientSecret)
	transport := NewTransport(3.0, 5, http.DefaultTransport)
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	cacheDir := filepath.Join(dataPath, "cache", "spotify")

	cat := NewCatalog(cacheDir, auth, client)
	res := NewStreamResolver(basePath, cacheDir, auth, client)
	res.SetDataDir(dataPath)

	mgr := &Manager{
		basePath:      basePath,
		dataPath:      dataPath,
		Auth:          auth,
		Catalog:       cat,
		Resolver:      res,
		client:        client,
		tracks:        make(map[string]Track),
		localPaths:    make(map[string]string),
		coverInflight: make(map[string]bool),
		coverFailedAt: make(map[string]time.Time),
	}
	res.OnTrackResolved = func(t Track) {
		mgr.RegisterTrack(t)
	}
	return mgr
}

// Source resolves local audio for a Spotify track and opens it as decode.Source.
func (m *Manager) Source(uri string) (decode.Source, bool, error) {
	trackID := ExtractTrackID(uri)
	tr, ok := m.Track(trackID)
	if !ok || tr.ID == "" {
		tr = Track{ID: trackID, Title: "Spotify Track", Artist: "Spotify"}
	}

	localPath, ready, err := m.Resolver.Resolve(tr)
	if err != nil || !ready {
		return nil, false, err
	}

	m.mu.Lock()
	m.localPaths[trackID] = localPath
	m.mu.Unlock()

	src, err := decode.Open(localPath)
	if err != nil {
		m.mu.Lock()
		delete(m.localPaths, trackID)
		m.mu.Unlock()
		if m.Resolver.isCacheFile(localPath) {
			_ = os.Remove(localPath)
		}
		return nil, false, err
	}
	return src, true, nil
}

// RegisterTrack associates metadata with a track ID for later lookup.
func (m *Manager) RegisterTrack(t Track) {
	if t.ID == "" || t.Title == "" || t.Title == "Spotify Track" {
		return
	}
	m.mu.Lock()
	m.tracks[t.ID] = t
	if m.localPaths[t.ID] == "-" {
		delete(m.localPaths, t.ID)
	}
	cb := m.OnMetadataResolved
	m.mu.Unlock()

	if m.Catalog != nil {
		m.Catalog.SaveTrack(t)
	}
	if cb != nil {
		cb(t)
	}
}

// CacheTracksInMemory stores a batch of tracks in memory for immediate lookup without disk writes.
func (m *Manager) CacheTracksInMemory(tracks []Track) {
	if len(tracks) == 0 {
		return
	}
	m.mu.Lock()
	for _, t := range tracks {
		if t.ID != "" && t.Title != "" && t.Title != "Spotify Track" {
			m.tracks[t.ID] = t
			if m.localPaths[t.ID] == "-" {
				delete(m.localPaths, t.ID)
			}
		}
	}
	m.mu.Unlock()
}

// Track returns cached metadata for a Spotify URI.
func (m *Manager) Track(uri string) (Track, bool) {
	trackID := ExtractTrackID(uri)
	m.mu.Lock()
	t, ok := m.tracks[trackID]
	if ok && t.Title != "" && t.Title != "Spotify Track" {
		m.mu.Unlock()
		return t, true
	}
	m.mu.Unlock()

	// Check persistent on-disk catalog (tracks, liked songs, playlists, albums) thread-safely.
	if m.Catalog != nil {
		if tr, found := m.Catalog.findTrack(trackID); found {
			m.mu.Lock()
			m.tracks[trackID] = tr
			m.mu.Unlock()
			return tr, true
		}
	}
	return t, ok
}

// FetchTrack retrieves metadata for a track ID from memory, catalog cache, or web API.
func (m *Manager) FetchTrack(uri string) (Track, error) {
	trackID := ExtractTrackID(uri)
	if t, ok := m.Track(trackID); ok && t.Title != "" && t.Title != "Spotify Track" {
		return t, nil
	}
	if m.Resolver != nil && m.Auth != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		t, err := m.Resolver.fetchSpotifyMeta(ctx, trackID)
		if err == nil && t.Title != "" {
			m.RegisterTrack(t)
			return t, nil
		}
	}
	return Track{ID: trackID, Title: "Spotify Track", Artist: "Spotify"}, nil
}

// LastError returns the most recent error message from the stream resolver.
func (m *Manager) LastError() string {
	if m == nil || m.Resolver == nil {
		return ""
	}
	return m.Resolver.LastError()
}

// Cover returns local cover art image path (or embedded-art local audio path) for a Spotify URI.
func (m *Manager) Cover(uri string) string {
	if m == nil || m.Resolver == nil {
		return ""
	}
	id := ExtractTrackID(uri)
	coverPath := filepath.Join(m.Resolver.songsDir(), id+".jpg")
	if fi, err := os.Stat(coverPath); err == nil && fi.Size() > 0 {
		return coverPath
	}

	m.mu.Lock()
	localPath, checkedLocal := m.localPaths[id]
	m.mu.Unlock()

	if !checkedLocal {
		localPath = m.TrackPath(id)
		m.mu.Lock()
		if localPath != "" {
			m.localPaths[id] = localPath
		} else {
			m.localPaths[id] = "-"
		}
		m.mu.Unlock()
	}

	if localPath != "" && localPath != "-" {
		dir := filepath.Dir(localPath)
		stem := strings.TrimSuffix(filepath.Base(localPath), filepath.Ext(localPath))
		for _, name := range []string{stem + ".jpg", "cover.jpg", "folder.jpg", "album.jpg", "cover.png", "folder.png", "album.png"} {
			cand := filepath.Join(dir, name)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Size() > 0 {
				return cand
			}
		}
	}

	tr, ok := m.Track(id)
	if ok && tr.CoverURL != "" {
		m.mu.Lock()
		inflight := m.coverInflight[id]
		failedAt := m.coverFailedAt[id]
		shouldStart := !inflight && (failedAt.IsZero() || time.Since(failedAt) > time.Minute)
		if shouldStart {
			m.coverInflight[id] = true
		}
		m.mu.Unlock()
		if shouldStart {
			go func(trackID, urlStr, dest string) {
				_ = os.MkdirAll(filepath.Dir(dest), 0o755)
				err := m.Resolver.downloadToFile(urlStr, dest)
				m.mu.Lock()
				delete(m.coverInflight, trackID)
				if err != nil {
					m.coverFailedAt[trackID] = time.Now()
				} else {
					delete(m.coverFailedAt, trackID)
				}
				m.mu.Unlock()
			}(id, tr.CoverURL, coverPath)
		}
	}

	if localPath != "" && localPath != "-" {
		if fi, err := os.Stat(localPath); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return localPath
		}
	}
	return ""
}

// SetMusicDirs configures the local music directories checked for existing tracks.
func (m *Manager) SetMusicDirs(dirs []string) {
	if m != nil && m.Resolver != nil {
		m.Resolver.SetMusicDirs(dirs)
	}
}

// SetMusicDirsFunc registers a dynamic callback providing local music directories.
func (m *Manager) SetMusicDirsFunc(fn func() []string) {
	if m != nil && m.Resolver != nil {
		m.Resolver.SetMusicDirsFunc(fn)
	}
}

// IsCached reports whether the track has already been downloaded to local cache or music library.
func (m *Manager) IsCached(uri string) bool {
	return m.TrackPath(uri) != ""
}

// TrackPath returns path to the audio file for a track ID (local library or cache) if available.
func (m *Manager) TrackPath(uri string) string {
	if m == nil || m.Resolver == nil {
		return ""
	}
	id := ExtractTrackID(uri)
	if tr, ok := m.Track(id); ok {
		if local := m.Resolver.findLocalTrack(tr); local != "" {
			return local
		}
	} else if local := m.Resolver.findLocalByID(id); local != "" {
		return local
	}
	p := m.Resolver.cacheDestPath(id)
	if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
		return p
	}
	return ""
}

// Download starts background download of the Spotify track for offline use if not already present.
func (m *Manager) Download(uri string) error {
	if m == nil || m.Resolver == nil {
		return nil
	}
	id := ExtractTrackID(uri)
	if m.IsCached(id) {
		return nil
	}
	tr, err := m.FetchTrack(id)
	if err != nil {
		tr = Track{ID: id, Title: "Spotify Track", Artist: "Spotify"}
	}
	if local := m.Resolver.findLocalTrack(tr); local != "" {
		return nil
	}
	go func() {
		p, ready, _ := m.Resolver.Resolve(tr)
		m.mu.Lock()
		if ready && p != "" {
			m.localPaths[id] = p
		} else if m.localPaths[id] == "-" {
			delete(m.localPaths, id)
		}
		m.mu.Unlock()
	}()
	return nil
}
