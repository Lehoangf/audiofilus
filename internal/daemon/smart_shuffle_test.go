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

func TestSmartShuffleToggleAndStatus(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	_ = os.MkdirAll(dataDir, 0o755)

	d := &Daemon{
		paths: config.Paths{Base: dir, Data: dataDir},
		cfg:   config.Config{},
		eng:   engine.NewHeadless(48000),
	}
	d.initSpotifyManager()

	if d.isSpotifySmartShuffle() {
		t.Fatalf("expected smart shuffle to be false initially")
	}

	// 1. Toggle ON via handleSpotify
	res, err := d.handleSpotify(control.Request{Args: []string{"smart_shuffle", "on"}})
	if err != nil {
		t.Fatalf("smart_shuffle on error: %v", err)
	}
	m := res.(map[string]any)
	if m["enabled"] != true {
		t.Fatalf("expected enabled: true, got %v", m["enabled"])
	}
	if !d.isSpotifySmartShuffle() {
		t.Fatalf("expected isSpotifySmartShuffle() == true")
	}
	if !d.eng.Status().Shuffle {
		t.Fatalf("expected engine shuffle to be enabled when smart shuffle is turned on")
	}

	// 2. Query status
	res, err = d.handleSpotify(control.Request{Args: []string{"smart_shuffle"}})
	if err != nil {
		t.Fatalf("smart_shuffle query error: %v", err)
	}
	m = res.(map[string]any)
	if m["enabled"] != true {
		t.Fatalf("expected query enabled: true, got %v", m["enabled"])
	}

	// 3. Toggle OFF
	res, err = d.handleSpotify(control.Request{Args: []string{"smart_shuffle", "off"}})
	if err != nil {
		t.Fatalf("smart_shuffle off error: %v", err)
	}
	m = res.(map[string]any)
	if m["enabled"] != false {
		t.Fatalf("expected enabled: false, got %v", m["enabled"])
	}
	if d.isSpotifySmartShuffle() {
		t.Fatalf("expected isSpotifySmartShuffle() == false")
	}
}

func TestSmartShuffleInjectionEveryThreeTracks(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	_ = os.MkdirAll(dataDir, 0o755)

	d := &Daemon{
		paths: config.Paths{Base: dir, Data: dataDir},
		cfg:   config.Config{},
		eng:   engine.NewHeadless(48000),
	}
	d.eng.OpenRemote = func(path string) (decode.Source, bool, error) {
		return &fakeSilentSource{}, true, nil
	}
	d.initSpotifyManager()

	initialTracks := []string{
		"spotify://track/song1",
		"spotify://track/song2",
		"spotify://track/song3",
		"spotify://track/song4",
	}
	d.eng.Load(initialTracks, 0, 0, false)

	// Pre-fill buffer with candidate
	d.spotSmartMu.Lock()
	d.spotSmartOn = true
	d.spotSmartBuffer = []spotify.Track{
		{ID: "rec_sparkle", Title: "Sparkle", Artist: "RADWIMPS"},
	}
	d.spotSmartMu.Unlock()

	// Track 1
	d.onTrackChangedSmartShuffle("spotify://track/song1")
	if len(d.eng.Queue()) != 4 {
		t.Fatalf("queue length should still be 4 after track 1, got %d", len(d.eng.Queue()))
	}

	// Track 2
	d.onTrackChangedSmartShuffle("spotify://track/song2")
	if len(d.eng.Queue()) != 4 {
		t.Fatalf("queue length should still be 4 after track 2, got %d", len(d.eng.Queue()))
	}

	// Track 3 -> triggers injection!
	d.onTrackChangedSmartShuffle("spotify://track/song3")
	queue := d.eng.Queue()
	if len(queue) != 5 {
		t.Fatalf("expected queue length 5 after 3 tracks played, got %d (queue: %v)", len(queue), queue)
	}

	expectedInjected := "spotify://track/rec_sparkle?smart=1"
	if !slices.Contains(queue, expectedInjected) {
		t.Fatalf("expected queue to contain %q, got %v", expectedInjected, queue)
	}

	d.spotSmartMu.Lock()
	played := d.spotSmartPlayed
	injected := d.spotSmartInjected["rec_sparkle"]
	d.spotSmartMu.Unlock()

	if played != 0 {
		t.Fatalf("expected spotSmartPlayed reset to 0, got %d", played)
	}
	if !injected {
		t.Fatalf("expected rec_sparkle marked as injected")
	}
}

func TestSmartShuffleRefillFromRecommendationsAPI(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/recommendations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tracks": []map[string]any{
				{
					"id":          "auto_rec1",
					"name":        "Auto Rec 1",
					"duration_ms": 190000,
					"artists":     []map[string]any{{"name": "Artist X"}},
					"album":       map[string]any{"name": "Album X"},
				},
			},
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	_ = os.MkdirAll(dataDir, 0o755)

	d := &Daemon{
		paths: config.Paths{Base: dir, Data: dataDir},
		cfg:   config.Config{},
		eng:   engine.NewHeadless(48000),
	}
	d.initSpotifyManager()
	_ = d.spotifyMgr.Auth.Save(spotify.Credentials{
		AccessToken: "test_token",
		ExpiresAt:   time.Now().Add(time.Hour),
		Product:     "premium",
	})
	d.spotifyMgr.Catalog.SetAPIBaseForTest(ts.URL + "/v1")

	d.eng.Load([]string{"spotify://track/seed_song"}, 0, 0, false)
	d.spotSmartMu.Lock()
	d.spotSmartOn = true
	d.spotSmartMu.Unlock()

	d.refillSmartShuffleBuffer()

	d.spotSmartMu.Lock()
	buf := append([]spotify.Track(nil), d.spotSmartBuffer...)
	d.spotSmartMu.Unlock()

	if len(buf) == 0 || buf[0].ID != "auto_rec1" {
		t.Fatalf("expected auto_rec1 in smart shuffle buffer, got %+v", buf)
	}
}
