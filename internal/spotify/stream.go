package spotify

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juanvillacortac/audiofilus/internal/ytm"
)

// spotifyHeaderSize is the size in bytes of the proprietary Spotify header prepended to encrypted Ogg Vorbis.
const spotifyHeaderSize = 167

// DecryptSpotifyOgg strips the 167-byte header and decrypts the Ogg Vorbis stream using AES-128-CTR.
func DecryptSpotifyOgg(in io.Reader, key [16]byte) (io.Reader, error) {
	// Discard 167-byte proprietary header.
	header := make([]byte, spotifyHeaderSize)
	if _, err := io.ReadFull(in, header); err != nil {
		return nil, fmt.Errorf("read spotify header: %w", err)
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("init aes cipher: %w", err)
	}

	// The CTR counter in Spotify begins with an all-zero 16-byte IV.
	iv := make([]byte, 16)
	stream := cipher.NewCTR(block, iv)

	return &cipher.StreamReader{S: stream, R: in}, nil
}

// cachedDotLib stores a parsed .library.json snapshot keyed by file mtime and size.
type cachedDotLib struct {
	modTime time.Time
	size    int64
	entries map[string]string
}

// scanLibEntry stores minimal tag metadata from data/library.json for deduplication.
type scanLibEntry struct {
	path        string
	title       string
	artist      string
	albumArtist string
}

// StreamResolver resolves and caches Spotify audio tracks locally using CDN or YouTube Music fallback.
type StreamResolver struct {
	baseDir         string
	cacheDir        string
	dataDir         string
	auth            *AuthManager
	client          *http.Client
	ytmFinder       func(query string) (string, error)
	maxCacheBytes   int64
	maxCacheTracks  int
	mu              sync.Mutex
	downloadSem     chan struct{} // Serializes heavy yt-dlp invocations with cancellation support
	inflight        map[string]bool
	failed          map[string]error
	lastErr         string
	OnTrackResolved func(Track)

	prefetchCancel context.CancelFunc
	prefetchID     string

	ytMap     map[string]string
	ytMapPath string
	ytMapMu   sync.RWMutex

	musicDirs     []string
	musicDirsFunc func() []string
	musicDirsMu   sync.RWMutex

	libCacheMu      sync.Mutex
	dotLibCache     map[string]cachedDotLib
	scanCacheMod    time.Time
	scanCacheSize   int64
	scanCacheByBase map[string][]scanLibEntry
}

// SetDataDir sets the directory containing data/library.json for scanned local library matching.
func (s *StreamResolver) SetDataDir(dir string) {
	s.libCacheMu.Lock()
	defer s.libCacheMu.Unlock()
	if s.dataDir != dir {
		s.dataDir = dir
		s.scanCacheMod = time.Time{}
		s.scanCacheSize = 0
		s.scanCacheByBase = nil
	}
}

// SetMusicDirs updates the local music directories checked for existing tracks before downloading.
func (s *StreamResolver) SetMusicDirs(dirs []string) {
	s.musicDirsMu.Lock()
	defer s.musicDirsMu.Unlock()
	s.musicDirs = make([]string, len(dirs))
	copy(s.musicDirs, dirs)
}

// SetMusicDirsFunc registers a dynamic callback returning local music directories.
func (s *StreamResolver) SetMusicDirsFunc(fn func() []string) {
	s.musicDirsMu.Lock()
	defer s.musicDirsMu.Unlock()
	s.musicDirsFunc = fn
}

func (s *StreamResolver) currentMusicDirs() []string {
	s.musicDirsMu.RLock()
	fn := s.musicDirsFunc
	dirs := append([]string(nil), s.musicDirs...)
	s.musicDirsMu.RUnlock()
	if fn != nil {
		if dynamic := fn(); len(dynamic) > 0 {
			return dynamic
		}
	}
	return dirs
}

func (s *StreamResolver) loadDotLib(dir string) map[string]string {
	libPath := filepath.Join(dir, ".library.json")
	fi, err := os.Stat(libPath)
	if err != nil || fi.IsDir() {
		s.libCacheMu.Lock()
		if s.dotLibCache != nil {
			delete(s.dotLibCache, dir)
		}
		s.libCacheMu.Unlock()
		return nil
	}

	s.libCacheMu.Lock()
	if c, ok := s.dotLibCache[dir]; ok && c.modTime.Equal(fi.ModTime()) && c.size == fi.Size() {
		s.libCacheMu.Unlock()
		return c.entries
	}
	s.libCacheMu.Unlock()

	entries := ytm.LoadLibrary(dir)
	s.libCacheMu.Lock()
	if s.dotLibCache == nil {
		s.dotLibCache = make(map[string]cachedDotLib)
	}
	s.dotLibCache[dir] = cachedDotLib{
		modTime: fi.ModTime(),
		size:    fi.Size(),
		entries: entries,
	}
	s.libCacheMu.Unlock()
	return entries
}

// findLocalByID searches musicDirs for a track registered in .library.json by its Spotify or mapped YouTube ID.
func (s *StreamResolver) findLocalByID(id string) string {
	if id == "" {
		return ""
	}
	dirs := s.currentMusicDirs()
	ytID := s.getMappedYT(id)
	keys := []string{"spotify:" + id, id}
	if ytID != "" {
		keys = append(keys, ytID)
	}

	for _, dir := range dirs {
		lib := s.loadDotLib(dir)
		if len(lib) == 0 {
			continue
		}
		for _, key := range keys {
			if rel, ok := lib[key]; ok && rel != "" {
				p := filepath.Join(dir, rel)
				if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
					return p
				}
			}
		}
	}
	return ""
}

func (s *StreamResolver) findLocalInScanCache(track Track) string {
	s.libCacheMu.Lock()
	dataDir := s.dataDir
	s.libCacheMu.Unlock()
	if dataDir == "" || track.Title == "" || track.Title == "Spotify Track" {
		return ""
	}
	wantBase, _ := ytm.BaseTitleAndVersion(track.Title)
	if wantBase == "" {
		return ""
	}

	cachePath := filepath.Join(dataDir, "library.json")
	fi, err := os.Stat(cachePath)
	if err != nil || fi.IsDir() || fi.Size() == 0 {
		return ""
	}

	s.libCacheMu.Lock()
	needReload := s.scanCacheByBase == nil || !s.scanCacheMod.Equal(fi.ModTime()) || s.scanCacheSize != fi.Size()
	modTime := fi.ModTime()
	size := fi.Size()
	s.libCacheMu.Unlock()

	if needReload {
		data, readErr := os.ReadFile(cachePath)
		if readErr != nil {
			return ""
		}
		var parsed struct {
			Entries []struct {
				Path        string `json:"path"`
				Title       string `json:"title"`
				Artist      string `json:"artist"`
				AlbumArtist string `json:"album_artist"`
			} `json:"entries"`
		}
		if json.Unmarshal(data, &parsed) != nil {
			return ""
		}
		idx := make(map[string][]scanLibEntry, len(parsed.Entries))
		for _, e := range parsed.Entries {
			if e.Path == "" || strings.Contains(e.Path, "://") {
				continue
			}
			title := e.Title
			if title == "" {
				stem := strings.TrimSuffix(filepath.Base(e.Path), filepath.Ext(e.Path))
				title = ytm.StripTrackPrefix(stem)
			}
			if title == "" {
				continue
			}
			base, _ := ytm.BaseTitleAndVersion(title)
			if base == "" {
				continue
			}
			idx[base] = append(idx[base], scanLibEntry{
				path:        e.Path,
				title:       title,
				artist:      e.Artist,
				albumArtist: e.AlbumArtist,
			})
		}
		s.libCacheMu.Lock()
		s.scanCacheMod = modTime
		s.scanCacheSize = size
		s.scanCacheByBase = idx
		s.libCacheMu.Unlock()
	}

	s.libCacheMu.Lock()
	cands := append([]scanLibEntry(nil), s.scanCacheByBase[wantBase]...)
	s.libCacheMu.Unlock()

	for _, c := range cands {
		if !ytm.ArtistMatches(track.Artist, c.artist, c.albumArtist) {
			continue
		}
		if !ytm.TitleMatches(c.title, track.Title, false) {
			continue
		}
		if fi, err := os.Stat(c.path); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return c.path
		}
	}
	return ""
}

// findLocalTrack checks if the track exists in local music directories via .library.json, filesystem paths, or library.json tags.
func (s *StreamResolver) findLocalTrack(track Track) string {
	if p := s.findLocalByID(track.ID); p != "" {
		return p
	}
	if track.Title == "" || track.Title == "Spotify Track" {
		return ""
	}
	dirs := s.currentMusicDirs()
	if p := ytm.FindLocalAudio(dirs, track.Artist, track.Album, track.Title); p != "" {
		return p
	}
	if p := s.findLocalInScanCache(track); p != "" {
		return p
	}
	return ""
}

// LastError returns the most recent error message recorded.
func (s *StreamResolver) LastError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *StreamResolver) loadYTMap() {
	s.ytMapMu.Lock()
	defer s.ytMapMu.Unlock()
	s.ytMap = make(map[string]string)
	if s.ytMapPath == "" {
		return
	}
	data, err := os.ReadFile(s.ytMapPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.ytMap)
}

func (s *StreamResolver) getMappedYT(spotifyID string) string {
	s.ytMapMu.RLock()
	defer s.ytMapMu.RUnlock()
	if s.ytMap == nil {
		return ""
	}
	return s.ytMap[spotifyID]
}

func (s *StreamResolver) saveMappedYT(spotifyID, ytID string) {
	if spotifyID == "" || ytID == "" {
		return
	}
	s.ytMapMu.Lock()
	defer s.ytMapMu.Unlock()
	if s.ytMap == nil {
		s.ytMap = make(map[string]string)
	}
	if s.ytMap[spotifyID] == ytID {
		return
	}
	s.ytMap[spotifyID] = ytID
	if s.ytMapPath == "" {
		return
	}
	data, err := json.MarshalIndent(s.ytMap, "", "  ")
	if err == nil {
		_ = os.WriteFile(s.ytMapPath, data, 0o644)
	}
}

func (s *StreamResolver) deleteMappedYT(spotifyID string) {
	if spotifyID == "" {
		return
	}
	s.ytMapMu.Lock()
	defer s.ytMapMu.Unlock()
	if s.ytMap == nil {
		return
	}
	if _, ok := s.ytMap[spotifyID]; !ok {
		return
	}
	delete(s.ytMap, spotifyID)
	if s.ytMapPath == "" {
		return
	}
	data, err := json.MarshalIndent(s.ytMap, "", "  ")
	if err == nil {
		_ = os.WriteFile(s.ytMapPath, data, 0o644)
	}
}

// NewStreamResolver creates a new audio stream resolver with local caching.
func NewStreamResolver(baseDir, cacheDir string, auth *AuthManager, client *http.Client) *StreamResolver {
	if client == nil {
		client = http.DefaultClient
	}
	if baseDir == "" && cacheDir != "" {
		baseDir = filepath.Dir(cacheDir)
	}
	res := &StreamResolver{
		baseDir:        baseDir,
		cacheDir:       cacheDir,
		auth:           auth,
		client:         client,
		maxCacheBytes:  500 * 1024 * 1024, // 500 MB default (optimal for SD cards on handhelds)
		maxCacheTracks: 50,                // 50 tracks max cache
		downloadSem:    make(chan struct{}, 1),
		inflight:       make(map[string]bool),
		failed:         make(map[string]error),
		ytMap:          make(map[string]string),
		ytMapPath:      filepath.Join(cacheDir, "spotify_yt_map.json"),
	}
	res.loadYTMap()
	res.CleanOrphanDownloads()
	return res
}

// CleanOrphanDownloads removes partial or incomplete files (.part*, .tmp, .ytdl)
// left behind by aborted downloads or unexpected exits.
func (s *StreamResolver) CleanOrphanDownloads() {
	entries, err := os.ReadDir(s.songsDir())
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.Contains(name, ".part") || strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".ytdl") || strings.HasSuffix(name, ".tags.json") {
			_ = os.Remove(filepath.Join(s.songsDir(), name))
		}
	}
}

func (s *StreamResolver) songsDir() string {
	return filepath.Join(s.cacheDir, "songs")
}

func (s *StreamResolver) cacheDestPath(id string) string {
	m4a := filepath.Join(s.songsDir(), id+".m4a")
	if fi, err := os.Stat(m4a); err == nil && fi.Size() > 0 {
		return m4a
	}
	audio := filepath.Join(s.songsDir(), id+".audio")
	if fi, err := os.Stat(audio); err == nil && fi.Size() > 0 {
		return audio
	}
	return m4a
}

func (s *StreamResolver) isCacheFile(p string) bool {
	if p == "" {
		return false
	}
	dir := filepath.Clean(s.songsDir()) + string(filepath.Separator)
	return strings.HasPrefix(filepath.Clean(p), dir)
}

func (s *StreamResolver) trackPath(id string) string {
	if local := s.findLocalByID(id); local != "" {
		return local
	}
	return s.cacheDestPath(id)
}

// CoverPath returns album art image path for a track ID if cached on disk.
func (s *StreamResolver) CoverPath(id string) string {
	jpg := filepath.Join(s.songsDir(), id+".jpg")
	if fi, err := os.Stat(jpg); err == nil && fi.Size() > 0 {
		return jpg
	}
	return ""
}

// Resolve returns the local audio file path for a track. If not yet cached,
// it triggers a background download and returns ("", false, nil) so playback
// emits silence without freezing the audio engine thread.
func (s *StreamResolver) Resolve(track Track) (string, bool, error) {
	if local := s.findLocalTrack(track); local != "" {
		return local, true, nil
	}
	destPath := s.cacheDestPath(track.ID)

	// 1. If file already exists with positive size in cache, reuse immediately.
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		now := time.Now()
		_ = os.Chtimes(destPath, now, now)
		return destPath, true, nil
	}

	// Synchronous mock resolver for unit tests.
	if s.ytmFinder != nil {
		return s.ResolveSync(track)
	}

	s.mu.Lock()

	// If prefetch is downloading another track, cancel it to give priority to current user playback.
	if s.prefetchID != "" && s.prefetchID != track.ID {
		if s.prefetchCancel != nil {
			s.prefetchCancel()
			s.prefetchCancel = nil
			s.prefetchID = ""
		}
	}

	// Double-check after acquiring lock.
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		s.mu.Unlock()
		return destPath, true, nil
	}

	if err, ok := s.failed[track.ID]; ok {
		delete(s.failed, track.ID)
		s.mu.Unlock()
		return "", false, err
	}

	if s.inflight[track.ID] {
		s.mu.Unlock()
		return "", false, nil
	}

	if err := os.MkdirAll(s.songsDir(), 0o755); err != nil {
		s.mu.Unlock()
		return "", false, err
	}

	s.inflight[track.ID] = true
	s.mu.Unlock()

	go func() {
		_, _, err := s.ResolveSync(track)
		s.mu.Lock()
		delete(s.inflight, track.ID)
		if err != nil {
			log.Printf("spotify: resolve %s (%s - %s) error: %v", track.ID, track.Title, track.Artist, err)
			s.lastErr = err.Error()
			s.failed[track.ID] = err
		}
		s.mu.Unlock()
	}()

	return "", false, nil
}

// ResolveSync synchronously downloads and resolves an audio track.
func (s *StreamResolver) ResolveSync(track Track) (string, bool, error) {
	if local := s.findLocalTrack(track); local != "" {
		return local, true, nil
	}
	destPath := s.cacheDestPath(track.ID)

	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		now := time.Now()
		_ = os.Chtimes(destPath, now, now)
		coverDest := filepath.Join(s.songsDir(), track.ID+".jpg")
		if _, err := os.Stat(coverDest); err != nil && track.CoverURL != "" {
			go func(url, dest string) {
				_ = s.downloadToFile(url, dest)
			}(track.CoverURL, coverDest)
		}
		return destPath, true, nil
	}

	// Serialize heavy yt-dlp downloads to prevent CPU/memory saturation on handheld devices.
	s.downloadSem <- struct{}{}
	defer func() { <-s.downloadSem }()

	// Re-check after acquiring semaphore in case another routine finished while waiting.
	if local := s.findLocalTrack(track); local != "" {
		return local, true, nil
	}
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		now := time.Now()
		_ = os.Chtimes(destPath, now, now)
		coverDest := filepath.Join(s.songsDir(), track.ID+".jpg")
		if _, err := os.Stat(coverDest); err != nil && track.CoverURL != "" {
			go func(url, dest string) {
				_ = s.downloadToFile(url, dest)
			}(track.CoverURL, coverDest)
		}
		return destPath, true, nil
	}

	if err := os.MkdirAll(s.songsDir(), 0o755); err != nil {
		return "", false, err
	}

	// 1. If custom finder is provided (tests or custom config), invoke it first.
	if s.ytmFinder != nil {
		audioURL, err := s.ytmFinder(track.Title + " " + track.Artist)
		if err != nil {
			return "", false, fmt.Errorf("fallback resolver: %w", err)
		}
		if audioURL != "" {
			if err := s.downloadToFile(audioURL, destPath); err != nil {
				return "", false, err
			}
			s.pruneLRU()
			return destPath, true, nil
		}
	}

	// 2. Fetch track metadata from Spotify API if missing or generic.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if (track.Title == "" || track.Title == "Spotify Track") && s.auth != nil {
		if meta, err := s.fetchSpotifyMeta(ctx, track.ID); err == nil && meta.Title != "" {
			track = meta
			if s.OnTrackResolved != nil {
				s.OnTrackResolved(meta)
			}
			if local := s.findLocalTrack(track); local != "" {
				return local, true, nil
			}
		}
	}

	// 3. Fallback: Search YouTube Music with ID cache priority.
	matchedID := s.getMappedYT(track.ID)
	fromCache := matchedID != ""
	if fromCache {
		log.Printf("spotify: using cached YouTube ID %s for track %s (%s)", matchedID, track.ID, track.Title)
	} else {
		query := track.Title + " " + track.Artist
		log.Printf("spotify: searching fallback track: %q", query)
		results, err := ytm.Search(ctx, query)
		if err != nil {
			return "", false, fmt.Errorf("search fallback provider: %w", err)
		}

		targetSecs := int(track.Duration.Seconds())
		for _, res := range results {
			resSecs := parseDurationSecs(res.Duration)
			if targetSecs > 0 && resSecs > 0 && math.Abs(float64(targetSecs-resSecs)) <= 3 {
				matchedID = res.ID
				break
			}
		}

		if matchedID == "" && len(results) > 0 {
			matchedID = results[0].ID
		}

		if matchedID != "" {
			s.saveMappedYT(track.ID, matchedID)
		}
	}

	if matchedID == "" {
		return "", false, fmt.Errorf("track %s (%s - %s) unavailable on fallback provider", track.ID, track.Title, track.Artist)
	}

	// 4. Locate yt-dlp binary.
	bin := ytm.Find(s.baseDir)
	if bin == "" {
		return "", false, errors.New("yt-dlp binary not found: install yt-dlp next to audiofilus")
	}

	// 5. Download audio track via native yt-dlp AAC (format 140 m4a).
	yTrack := ytm.Track{
		ID:      matchedID,
		Title:   track.Title,
		Artists: []string{track.Artist},
		Album:   track.Album,
	}
	log.Printf("spotify: downloading %s as YouTube %s with %s", track.ID, matchedID, bin)
	downloadedPath, err := ytm.Fetch(ctx, bin, yTrack, s.songsDir(), 100, nil)
	if err != nil && fromCache {
		log.Printf("spotify: cached YouTube ID %s failed (%v), falling back to fresh search", matchedID, err)
		s.deleteMappedYT(track.ID)
		query := track.Title + " " + track.Artist
		if results, searchErr := ytm.Search(ctx, query); searchErr == nil && len(results) > 0 {
			matchedID = results[0].ID
			targetSecs := int(track.Duration.Seconds())
			for _, res := range results {
				resSecs := parseDurationSecs(res.Duration)
				if targetSecs > 0 && resSecs > 0 && math.Abs(float64(targetSecs-resSecs)) <= 3 {
					matchedID = res.ID
					break
				}
			}
			yTrack.ID = matchedID
			s.saveMappedYT(track.ID, matchedID)
			downloadedPath, err = ytm.Fetch(ctx, bin, yTrack, s.songsDir(), 100, nil)
		}
	}
	if err != nil {
		return "", false, fmt.Errorf("download fallback audio: %w", err)
	}
	// Save cover art: prefer official Spotify CoverURL if available, else YouTube thumbnail.
	coverDest := filepath.Join(s.songsDir(), track.ID+".jpg")
	if fi, err := os.Stat(coverDest); err != nil || fi.Size() == 0 {
		if track.CoverURL != "" {
			_ = s.downloadToFile(track.CoverURL, coverDest)
		}
		if fi2, err2 := os.Stat(coverDest); err2 != nil || fi2.Size() == 0 {
			ytThumb := downloadedPath + ".jpg"
			if fi3, err3 := os.Stat(ytThumb); err3 == nil && fi3.Size() > 0 {
				_ = copyFile(ytThumb, coverDest)
			}
		}
	}
	_ = os.Remove(downloadedPath + ".tags.json")
	_ = os.Remove(downloadedPath + ".jpg")

	if downloadedPath != destPath {
		_ = os.Remove(destPath)
		if err := os.Rename(downloadedPath, destPath); err != nil {
			if err := copyFile(downloadedPath, destPath); err != nil {
				return "", false, err
			}
			_ = os.Remove(downloadedPath)
		}
	}

	s.pruneLRU()
	return destPath, true, nil
}

// fetchSpotifyMeta fetches official track metadata from Spotify Web API.
func (s *StreamResolver) fetchSpotifyMeta(ctx context.Context, id string) (Track, error) {
	if s.auth == nil {
		return Track{}, errors.New("no auth manager")
	}
	token, err := s.auth.ProactiveToken()
	if err != nil {
		return Track{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.spotify.com/v1/tracks/"+url.PathEscape(id), nil)
	if err != nil {
		return Track{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		return Track{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Track{}, fmt.Errorf("spotify track api: status %d", resp.StatusCode)
	}
	var res struct {
		Name       string `json:"name"`
		DurationMS int    `json:"duration_ms"`
		Artists    []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			Name   string `json:"name"`
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"album"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return Track{}, err
	}
	var artists []string
	for _, a := range res.Artists {
		artists = append(artists, a.Name)
	}
	var cover string
	if len(res.Album.Images) > 0 {
		cover = res.Album.Images[0].URL
	}
	return Track{
		ID:       id,
		Title:    res.Name,
		Artist:   strings.Join(artists, ", "),
		Album:    res.Album.Name,
		Duration: time.Duration(res.DurationMS) * time.Millisecond,
		CoverURL: cover,
	}, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// Prefetch initiates background pre-downloading of the next track
// with cancellation support if the user switches tracks.
func (s *StreamResolver) Prefetch(track Track) {
	if track.ID == "" {
		return
	}
	if local := s.findLocalTrack(track); local != "" {
		return
	}
	destPath := s.cacheDestPath(track.ID)
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		return
	}

	s.mu.Lock()
	if s.prefetchID == track.ID {
		s.mu.Unlock()
		return
	}
	if s.prefetchCancel != nil {
		s.prefetchCancel()
		s.prefetchCancel = nil
		s.prefetchID = ""
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.prefetchCancel = cancel
	s.prefetchID = track.ID
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			if s.prefetchID == track.ID {
				s.prefetchCancel = nil
				s.prefetchID = ""
			}
			s.mu.Unlock()
		}()
		_ = s.resolvePrefetch(ctx, track)
	}()
}

func (s *StreamResolver) resolvePrefetch(ctx context.Context, track Track) error {
	if local := s.findLocalTrack(track); local != "" {
		return nil
	}
	destPath := s.cacheDestPath(track.ID)
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		return nil
	}

	select {
	case s.downloadSem <- struct{}{}:
		defer func() { <-s.downloadSem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if local := s.findLocalTrack(track); local != "" {
		return nil
	}
	if fi, err := os.Stat(destPath); err == nil && fi.Size() > 0 {
		return nil
	}

	if (track.Title == "" || track.Title == "Spotify Track") && s.auth != nil {
		if meta, err := s.fetchSpotifyMeta(ctx, track.ID); err == nil && meta.Title != "" {
			track = meta
			if s.OnTrackResolved != nil {
				s.OnTrackResolved(meta)
			}
			if local := s.findLocalTrack(track); local != "" {
				return nil
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	matchedID := s.getMappedYT(track.ID)
	fromCache := matchedID != ""
	if fromCache {
		log.Printf("spotify: prefetch using cached YouTube ID %s for track %s (%s)", matchedID, track.ID, track.Title)
	} else {
		query := track.Title + " " + track.Artist
		results, err := ytm.Search(ctx, query)
		if err != nil {
			return err
		}
		targetSecs := int(track.Duration.Seconds())
		for _, res := range results {
			resSecs := parseDurationSecs(res.Duration)
			if targetSecs > 0 && resSecs > 0 && math.Abs(float64(targetSecs-resSecs)) <= 3 {
				matchedID = res.ID
				break
			}
		}
		if matchedID == "" && len(results) > 0 {
			matchedID = results[0].ID
		}
		if matchedID != "" {
			s.saveMappedYT(track.ID, matchedID)
		}
	}

	if matchedID == "" {
		return fmt.Errorf("track %s unavailable", track.ID)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	bin := ytm.Find(s.baseDir)
	if bin == "" {
		return errors.New("yt-dlp binary not found")
	}

	yTrack := ytm.Track{
		ID:      matchedID,
		Title:   track.Title,
		Artists: []string{track.Artist},
		Album:   track.Album,
	}

	log.Printf("spotify: prefetching %s (%s - %s) as YouTube %s", track.ID, track.Title, track.Artist, matchedID)
	downloadedPath, err := ytm.Fetch(ctx, bin, yTrack, s.songsDir(), 100, nil)
	if err != nil && fromCache {
		s.deleteMappedYT(track.ID)
		query := track.Title + " " + track.Artist
		if results, searchErr := ytm.Search(ctx, query); searchErr == nil && len(results) > 0 {
			matchedID = results[0].ID
			targetSecs := int(track.Duration.Seconds())
			for _, res := range results {
				resSecs := parseDurationSecs(res.Duration)
				if targetSecs > 0 && resSecs > 0 && math.Abs(float64(targetSecs-resSecs)) <= 3 {
					matchedID = res.ID
					break
				}
			}
			yTrack.ID = matchedID
			s.saveMappedYT(track.ID, matchedID)
			downloadedPath, err = ytm.Fetch(ctx, bin, yTrack, s.songsDir(), 100, nil)
		}
	}
	if err != nil {
		return err
	}

	coverDest := filepath.Join(s.songsDir(), track.ID+".jpg")
	if fi, err := os.Stat(coverDest); err != nil || fi.Size() == 0 {
		if track.CoverURL != "" {
			_ = s.downloadToFile(track.CoverURL, coverDest)
		}
		if fi2, err2 := os.Stat(coverDest); err2 != nil || fi2.Size() == 0 {
			ytThumb := downloadedPath + ".jpg"
			if fi3, err3 := os.Stat(ytThumb); err3 == nil && fi3.Size() > 0 {
				_ = copyFile(ytThumb, coverDest)
			}
		}
	}
	_ = os.Remove(downloadedPath + ".tags.json")
	_ = os.Remove(downloadedPath + ".jpg")

	if downloadedPath != destPath {
		_ = os.Remove(destPath)
		if err := os.Rename(downloadedPath, destPath); err != nil {
			if err := copyFile(downloadedPath, destPath); err != nil {
				return err
			}
			_ = os.Remove(downloadedPath)
		}
	}

	s.pruneLRU()
	log.Printf("spotify: prefetch completed for %s (%s)", track.ID, track.Title)
	return nil
}

func (s *StreamResolver) downloadToFile(urlStr, dest string) error {
	resp, err := s.client.Get(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download audio status: %d", resp.StatusCode)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(dest)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func parseDurationSecs(d string) int {
	parts := strings.Split(d, ":")
	if len(parts) == 2 {
		m, _ := strconv.Atoi(parts[0])
		s, _ := strconv.Atoi(parts[1])
		return m*60 + s
	} else if len(parts) == 3 {
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		s, _ := strconv.Atoi(parts[2])
		return h*3600 + m*60 + s
	}
	return 0
}

func (s *StreamResolver) pruneLRU() {
	entries, err := os.ReadDir(s.songsDir())
	if err != nil {
		return
	}

	type fileItem struct {
		path    string
		size    int64
		modTime time.Time
	}

	var files []fileItem
	var totalSize int64

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		p := filepath.Join(s.songsDir(), entry.Name())
		// Clean up orphan temporary files from incomplete downloads.
		if strings.Contains(entry.Name(), ".part") || strings.HasSuffix(entry.Name(), ".tmp") || strings.HasSuffix(entry.Name(), ".ytdl") || strings.HasSuffix(entry.Name(), ".tags.json") {
			_ = os.Remove(p)
			continue
		}
		// Retain album art while the corresponding audio file exists.
		if strings.HasSuffix(entry.Name(), ".jpg") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileItem{
			path:    p,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		totalSize += info.Size()
	}

	maxTracks := s.maxCacheTracks
	if maxTracks <= 0 {
		maxTracks = 50
	}
	maxBytes := s.maxCacheBytes
	if maxBytes <= 0 {
		maxBytes = 500 * 1024 * 1024
	}

	if totalSize <= maxBytes && len(files) <= maxTracks {
		return
	}

	// Sort by modification time ascending to evict least recently used files first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	for _, fi := range files {
		if totalSize <= maxBytes && len(files) <= maxTracks {
			break
		}
		_ = os.Remove(fi.path)
		// Also remove associated album cover art if present.
		ext := filepath.Ext(fi.path)
		_ = os.Remove(strings.TrimSuffix(fi.path, ext) + ".jpg")
		totalSize -= fi.size
		files = files[1:]
	}
}
