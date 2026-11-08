package spotify

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDeltaSyncPlaylist(t *testing.T) {
	var trackFetches atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me/playlists" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[{"id":"pl1","name":"Hits","snapshot_id":"snap1"}]}`))
			return
		}
		if r.URL.Path == "/v1/playlists/pl1/tracks" {
			trackFetches.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[{"track":{"id":"t1","name":"Song 1","artists":[{"name":"Artist 1"}],"album":{"name":"Album 1"},"duration_ms":180000,"external_ids":{"isrc":"USRC123"}}}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cat := NewCatalog(t.TempDir(), nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"
	// First sync: must fetch tracks
	playlists, err := cat.SyncPlaylists()
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(playlists) != 1 || len(playlists[0].Tracks) != 1 {
		t.Fatalf("expected 1 playlist with 1 track, got %+v", playlists)
	}
	if trackFetches.Load() != 1 {
		t.Fatalf("expected 1 fetch, got %d", trackFetches.Load())
	}
	// Second sync with identical snapshot_id: must skip track fetch
	cached, err := cat.SyncPlaylists()
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(cached) != 1 || len(cached[0].Tracks) != 1 {
		t.Fatalf("expected 1 playlist with 1 track from cache, got %+v", cached)
	}
	if trackFetches.Load() != 1 {
		t.Fatalf("expected tracks to be cached without refetching, got %d fetches", trackFetches.Load())
	}
}

func TestLikedSongs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me/tracks" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"items":[{"track":{"id":"liked1","name":"Favorite","artists":[{"name":"Star"}],"album":{"name":"Best"},"duration_ms":210000}}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cat := NewCatalog(t.TempDir(), nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	tracks, err := cat.LikedSongs()
	if err != nil {
		t.Fatalf("LikedSongs: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Title != "Favorite" {
		t.Fatalf("unexpected tracks: %+v", tracks)
	}
}
