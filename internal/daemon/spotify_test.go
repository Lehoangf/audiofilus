package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/juanvillacortac/audiofilus/internal/config"
	"github.com/juanvillacortac/audiofilus/internal/control"
	"github.com/juanvillacortac/audiofilus/internal/decode"
	"github.com/juanvillacortac/audiofilus/internal/engine"
	"github.com/juanvillacortac/audiofilus/internal/spotify"
)

func TestSpotifyTrackURLRecognition(t *testing.T) {
	uri := "spotify://track/4cOdK2wGLETKBW3PvgPWqT"
	if !isSpotifyURI(uri) {
		t.Errorf("expected %s to be recognized as spotify URI", uri)
	}
	invalid := "https://example.com/song.mp3"
	if isSpotifyURI(invalid) {
		t.Errorf("expected %s NOT to be recognized as spotify URI", invalid)
	}
}

type fakeSilentSource struct{}

func (s *fakeSilentSource) Read(dst []float32) (int, error) {
	for i := range dst {
		dst[i] = 0
	}
	return len(dst), nil
}
func (s *fakeSilentSource) SampleRate() int       { return 48000 }
func (s *fakeSilentSource) Length() int64         { return 0 }
func (s *fakeSilentSource) Position() int64       { return 0 }
func (s *fakeSilentSource) SeekFrame(int64) error { return nil }
func (s *fakeSilentSource) Close() error          { return nil }

var _ decode.Source = (*fakeSilentSource)(nil)

func TestHandleSpotifyPaginationSubcommands(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cacheDir := filepath.Join(dataDir, "cache", "spotify")
	_ = os.MkdirAll(cacheDir, 0o755)

	catData := map[string]any{
		"playlists": []map[string]any{
			{
				"id":              "pl1",
				"name":            "Playlist 1",
				"snapshot_id":     "s1",
				"track_count":     2,
				"tracks_complete": true,
				"tracks": []map[string]any{
					{"id": "t1", "title": "Song 1", "artist": "Artist 1"},
					{"id": "t2", "title": "Song 2", "artist": "Artist 2"},
				},
			},
		},
		"liked_tracks": []map[string]any{
			{"id": "lk1", "title": "Liked 1", "artist": "Artist 1"},
		},
		"liked_total":           1,
		"liked_tracks_complete": true,
		"albums": []map[string]any{
			{
				"id":              "al1",
				"name":            "Album 1",
				"track_count":     2,
				"tracks_complete": true,
				"tracks": []map[string]any{
					{"id": "alt1", "title": "Album Song 1", "artist": "Album Artist"},
					{"id": "alt2", "title": "Album Song 2", "artist": "Album Artist"},
				},
			},
		},
	}
	raw, _ := json.Marshal(catData)
	_ = os.WriteFile(filepath.Join(cacheDir, "catalog.json"), raw, 0o644)

	d := &Daemon{
		paths: config.Paths{Base: dir, Data: dataDir},
		cfg:   config.Config{},
	}
	d.initSpotifyManager()

	// 1. playlist_page
	resPl, err := d.handleSpotify(control.Request{Args: []string{"playlist_page", "pl1", "0", "50"}})
	if err != nil {
		t.Fatalf("playlist_page error: %v", err)
	}
	pagePl, ok := resPl.(spotify.PageResult)
	if !ok || len(pagePl.Tracks) != 2 || pagePl.Total != 2 {
		t.Fatalf("playlist_page got %+v, want PageResult with 2 tracks", resPl)
	}

	// 2. liked_page
	resLk, err := d.handleSpotify(control.Request{Args: []string{"liked_page", "0", "50", "false"}})
	if err != nil {
		t.Fatalf("liked_page error: %v", err)
	}
	pageLk, ok := resLk.(spotify.PageResult)
	if !ok || len(pageLk.Tracks) != 1 || pageLk.Total != 1 {
		t.Fatalf("liked_page got %+v, want PageResult with 1 track", resLk)
	}

	// 3. album_page
	resAl, err := d.handleSpotify(control.Request{Args: []string{"album_page", "al1", "0", "50"}})
	if err != nil {
		t.Fatalf("album_page error: %v", err)
	}
	pageAl, ok := resAl.(spotify.PageResult)
	if !ok || len(pageAl.Tracks) != 2 || pageAl.Total != 2 {
		t.Fatalf("album_page got %+v, want PageResult with 2 tracks", resAl)
	}

	// 4. album_tracks
	resAlTracks, err := d.handleSpotify(control.Request{Args: []string{"album_tracks", "al1"}})
	if err != nil {
		t.Fatalf("album_tracks error: %v", err)
	}
	alTracks, ok := resAlTracks.([]spotify.Track)
	if !ok || len(alTracks) != 2 {
		t.Fatalf("album_tracks got %+v, want 2 tracks", resAlTracks)
	}

	// 5. tracks metadata resolution
	resTracks, err := d.handleSpotify(control.Request{Args: []string{"tracks", "t1", "alt2"}})
	if err != nil {
		t.Fatalf("tracks error: %v", err)
	}
	trMap, ok := resTracks.(map[string]spotify.Track)
	if !ok || len(trMap) != 2 || trMap["t1"].Title != "Song 1" || trMap["alt2"].Title != "Album Song 2" {
		t.Fatalf("tracks map got %+v, want t1 and alt2 resolved", resTracks)
	}
}

func TestHandleSpotifyPlayCollectionImmediateAndHydrates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/playlists/pl_hyd/tracks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"next":  "",
			"total": 5,
			"items": []map[string]any{
				{"track": map[string]string{"id": "h0"}},
				{"track": map[string]string{"id": "h1"}},
				{"track": map[string]string{"id": "h2"}},
				{"track": map[string]string{"id": "h3"}},
				{"track": map[string]string{"id": "h4"}},
			},
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cacheDir := filepath.Join(dataDir, "cache", "spotify")
	_ = os.MkdirAll(cacheDir, 0o755)

	d := &Daemon{
		paths: config.Paths{Base: dir, Data: dataDir},
		cfg:   config.Config{},
	}
	d.eng = engine.NewHeadless(48000)
	d.eng.OpenRemote = func(path string) (decode.Source, bool, error) {
		return &fakeSilentSource{}, true, nil
	}

	d.initSpotifyManager()
	// Point catalog API to mock test server.
	rawCat, _ := json.Marshal(map[string]any{
		"playlists": []map[string]any{
			{
				"id":          "pl_hyd",
				"name":        "Hydration Mix",
				"snapshot_id": "snap_hyd",
				"track_count": 5,
				"tracks": []map[string]any{
					{"id": "h0", "title": "Track 0"},
					{"id": "h1", "title": "Track 1"},
				},
				"fetched_offset": 2,
			},
		},
	})
	_ = os.WriteFile(filepath.Join(cacheDir, "catalog.json"), rawCat, 0o644)

	// Replace catalog's apiBase and client for the test server.
	fakeCat := spotify.NewCatalog(cacheDir, nil, ts.Client())
	fakeCat.SetAPIBaseForTest(ts.URL + "/v1")
	d.spotifyMgr.Catalog = fakeCat

	// Immediately plays h0, h1 without blocking, hydrates h2, h3, h4 in background.
	res, err := d.handleSpotify(control.Request{
		Args: []string{"play_collection", "playlist", "pl_hyd", "0", "h0", "h1"},
	})
	if err != nil {
		t.Fatalf("play_collection error: %v", err)
	}
	statusMap, ok := res.(map[string]any)
	if !ok || statusMap["status"] != "playing" {
		t.Fatalf("expected status playing, got %+v", res)
	}

	// Immediate state: queue must have at least h0 and h1 immediately.
	st := d.eng.Status()
	if st.Count < 2 {
		t.Fatalf("expected immediate queue count >= 2, got %d", st.Count)
	}

	// Poll briefly for background hydration to complete queue to 5 tracks.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && d.eng.Status().Count < 5 {
		time.Sleep(20 * time.Millisecond)
	}

	finalCount := d.eng.Status().Count
	if finalCount != 5 {
		t.Fatalf("queue after hydration count = %d, want 5 (queue=%v)", finalCount, d.eng.Queue())
	}
	wantQueue := []string{
		"spotify://track/h0",
		"spotify://track/h1",
		"spotify://track/h2",
		"spotify://track/h3",
		"spotify://track/h4",
	}
	if !slices.Equal(d.eng.Queue(), wantQueue) {
		t.Fatalf("queue = %v, want %v", d.eng.Queue(), wantQueue)
	}

	// Starting a new collection cancels previous background hydration.
	d.cancelSpotifyHydration()
}

func TestHandleSpotifyTrackActions(t *testing.T) {
	var likedMethods []string
	var addedPlaylistID string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/me/tracks":
			likedMethods = append(likedMethods, r.Method)
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/playlists/pl1/tracks":
			addedPlaylistID = "pl1"
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cacheDir := filepath.Join(dataDir, "cache", "spotify")
	_ = os.MkdirAll(cacheDir, 0o755)

	catData := map[string]any{
		"playlists": []map[string]any{
			{
				"id":          "pl1",
				"name":        "My Playlist",
				"track_count": 0,
			},
		},
	}
	raw, _ := json.Marshal(catData)
	_ = os.WriteFile(filepath.Join(cacheDir, "catalog.json"), raw, 0o644)

	d := &Daemon{
		paths: config.Paths{Base: dir, Data: dataDir},
		cfg:   config.Config{},
	}
	d.initSpotifyManager()
	_ = d.spotifyMgr.Auth.Save(spotify.Credentials{
		AccessToken: "test_token",
		ExpiresAt:   time.Now().Add(time.Hour),
		Product:     "premium",
	})
	d.spotifyMgr.Catalog.SetAPIBaseForTest(ts.URL + "/v1")

	// 1. Initially is_liked should be false
	res, err := d.handleSpotify(control.Request{Args: []string{"is_liked", "t1"}})
	if err != nil {
		t.Fatalf("is_liked error: %v", err)
	}
	m := res.(map[string]any)
	if m["liked"] != false {
		t.Fatalf("expected t1 to not be liked initially, got %v", m["liked"])
	}

	// 2. like t1
	res, err = d.handleSpotify(control.Request{Args: []string{"like", "t1"}})
	if err != nil {
		t.Fatalf("like error: %v", err)
	}
	m = res.(map[string]any)
	if m["liked"] != true {
		t.Fatalf("expected like to return liked: true, got %v", m["liked"])
	}

	// 3. is_liked should now be true
	res, err = d.handleSpotify(control.Request{Args: []string{"is_liked", "t1"}})
	if err != nil {
		t.Fatalf("is_liked error: %v", err)
	}
	m = res.(map[string]any)
	if m["liked"] != true {
		t.Fatalf("expected t1 to be liked after like, got %v", m["liked"])
	}

	// 4. unlike t1
	res, err = d.handleSpotify(control.Request{Args: []string{"unlike", "t1"}})
	if err != nil {
		t.Fatalf("unlike error: %v", err)
	}
	m = res.(map[string]any)
	if m["liked"] != false {
		t.Fatalf("expected unlike to return liked: false, got %v", m["liked"])
	}

	// 5. is_liked should now be false
	res, err = d.handleSpotify(control.Request{Args: []string{"is_liked", "t1"}})
	if err != nil {
		t.Fatalf("is_liked error: %v", err)
	}
	m = res.(map[string]any)
	if m["liked"] != false {
		t.Fatalf("expected t1 to not be liked after unlike, got %v", m["liked"])
	}

	// 6. add_to_playlist pl1 t1
	res, err = d.handleSpotify(control.Request{Args: []string{"add_to_playlist", "pl1", "t1"}})
	if err != nil {
		t.Fatalf("add_to_playlist error: %v", err)
	}
	m = res.(map[string]any)
	if m["status"] != "ok" || m["playlist_id"] != "pl1" || m["track_id"] != "t1" {
		t.Fatalf("unexpected add_to_playlist response: %v", m)
	}
	if addedPlaylistID != "pl1" {
		t.Fatalf("expected track added to pl1, got %v", addedPlaylistID)
	}
}

func TestHandleSpotifyRecommendations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/recommendations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tracks": []map[string]any{
				{
					"id":          "rec_track_1",
					"name":        "Smart Song 1",
					"duration_ms": 200000,
					"artists":     []map[string]any{{"name": "Smart Artist"}},
					"album":       map[string]any{"name": "Smart Album"},
				},
			},
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	cacheDir := filepath.Join(dataDir, "cache", "spotify")
	_ = os.MkdirAll(cacheDir, 0o755)

	d := &Daemon{
		paths: config.Paths{Base: dir, Data: dataDir},
		cfg:   config.Config{},
	}
	d.initSpotifyManager()
	_ = d.spotifyMgr.Auth.Save(spotify.Credentials{
		AccessToken: "test_token",
		ExpiresAt:   time.Now().Add(time.Hour),
		Product:     "premium",
	})
	d.spotifyMgr.Catalog.SetAPIBaseForTest(ts.URL + "/v1")

	res, err := d.handleSpotify(control.Request{Args: []string{"recommendations", "5", "s1", "s2"}})
	if err != nil {
		t.Fatalf("recommendations error: %v", err)
	}
	tracks, ok := res.([]spotify.Track)
	if !ok || len(tracks) != 1 || tracks[0].ID != "rec_track_1" {
		t.Fatalf("expected []spotify.Track with rec_track_1, got %+v", res)
	}

	// Verify in-memory cache
	if tr, found := d.spotifyMgr.Track("rec_track_1"); !found || tr.Title != "Smart Song 1" {
		t.Fatalf("expected rec_track_1 cached in memory, got %+v (found=%v)", tr, found)
	}
}
