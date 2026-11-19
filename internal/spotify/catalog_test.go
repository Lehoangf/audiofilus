package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	// First sync: syncs playlist metadata fast without blocking on track fetching
	playlists, err := cat.SyncPlaylists()
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(playlists) != 1 {
		t.Fatalf("expected 1 playlist, got %+v", playlists)
	}
	// Fetch tracks on demand
	tracks, err := cat.PlaylistTracks("pl1")
	if err != nil {
		t.Fatalf("fetch playlist tracks: %v", err)
	}
	if len(tracks) != 1 || tracks[0].ID != "t1" {
		t.Fatalf("expected 1 track, got %+v", tracks)
	}
	if trackFetches.Load() != 1 {
		t.Fatalf("expected 1 fetch, got %d", trackFetches.Load())
	}
	// Second sync with identical snapshot_id: must preserve cached tracks
	cached, err := cat.SyncPlaylists()
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(cached) != 1 || len(cached[0].Tracks) != 1 {
		t.Fatalf("expected 1 playlist with 1 track from cache, got %+v", cached)
	}
	// Calling PlaylistTracks again should return cached tracks without incrementing trackFetches
	tracks2, err := cat.PlaylistTracks("pl1")
	if err != nil {
		t.Fatalf("cached playlist tracks: %v", err)
	}
	if len(tracks2) != 1 {
		t.Fatalf("expected 1 track, got %+v", tracks2)
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

func writeLegacyCatalog(t *testing.T, dir string, cd catalogData) {
	t.Helper()
	b, err := json.Marshal(cd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "catalog.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncPlaylistsPaginationAndTotalRepair(t *testing.T) {
	const totalPlaylists = 210
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/playlists" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		offset := 0
		fmt.Sscanf(q.Get("offset"), "%d", &offset)
		limit := 50
		fmt.Sscanf(q.Get("limit"), "%d", &limit)
		if limit <= 0 {
			limit = 50
		}

		type plItem struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			SnapshotID string `json:"snapshot_id"`
			Tracks     struct {
				Total int `json:"total"`
			} `json:"tracks"`
		}
		var items []plItem
		for i := offset; i < totalPlaylists && i < offset+limit; i++ {
			id := fmt.Sprintf("pl%03d", i)
			name := fmt.Sprintf("Mix %03d", i)
			snap := "snap_" + id
			total := 20
			if i == 0 {
				id, name, snap, total = "edm", "EDM", "snapEDM", 423
			}
			var it plItem
			it.ID, it.Name, it.SnapshotID = id, name, snap
			it.Tracks.Total = total
			items = append(items, it)
		}
		next := ""
		if offset+len(items) < totalPlaylists {
			next = fmt.Sprintf("http://%s/v1/me/playlists?offset=%d&limit=%d", r.Host, offset+len(items), limit)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"next":  next,
			"total": totalPlaylists,
			"items": items,
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	var legacyTracks []Track
	for i := 0; i < 100; i++ {
		legacyTracks = append(legacyTracks, Track{ID: fmt.Sprintf("edm_%03d", i), Title: fmt.Sprintf("Track %d", i)})
	}
	writeLegacyCatalog(t, dir, catalogData{
		Playlists: []Playlist{
			{ID: "edm", Name: "EDM", SnapshotID: "snapEDM", TrackCount: 100, Tracks: legacyTracks},
		},
	})

	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	pls, err := cat.SyncPlaylists()
	if err != nil {
		t.Fatalf("SyncPlaylists: %v", err)
	}
	if len(pls) != totalPlaylists {
		t.Fatalf("SyncPlaylists len = %d, want %d (must not cap at 200)", len(pls), totalPlaylists)
	}
	if pls[0].TrackCount != 423 {
		t.Fatalf("EDM TrackCount = %d, want 423 even when snapshot_id matched", pls[0].TrackCount)
	}
	if len(pls[0].Tracks) != 100 {
		t.Fatalf("EDM cached tracks lost during total repair: len=%d, want 100", len(pls[0].Tracks))
	}
}

func TestSyncPlaylistsFastPathDetectsStaleTrackCount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/playlists" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":1,"items":[{"id":"edm","name":"EDM","snapshot_id":"snapEDM","tracks":{"total":423}}]}`))
	}))
	defer ts.Close()

	dir := t.TempDir()
	legacyTracks := make([]Track, 100)
	for i := range legacyTracks {
		legacyTracks[i] = Track{ID: fmt.Sprintf("t%d", i), Title: fmt.Sprintf("Song %d", i)}
	}
	writeLegacyCatalog(t, dir, catalogData{
		Playlists: []Playlist{
			{ID: "edm", Name: "EDM", SnapshotID: "snapEDM", TrackCount: 100, Tracks: legacyTracks},
		},
	})

	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	pls, err := cat.SyncPlaylists()
	if err != nil {
		t.Fatalf("SyncPlaylists: %v", err)
	}
	if len(pls) != 1 || pls[0].TrackCount != 423 {
		t.Fatalf("fast-path failed to repair TrackCount: got %+v", pls)
	}
	if len(pls[0].Tracks) != 100 || pls[0].TracksComplete {
		t.Fatalf("expected 100 partial tracks preserved with TracksComplete=false, got len=%d complete=%v", len(pls[0].Tracks), pls[0].TracksComplete)
	}
}

func TestPlaylistPaginationAndSelfHealingLegacyCache(t *testing.T) {
	const apiTotal = 423
	var fetchedOffsets []int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/playlists/edm/tracks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		offset, limit := 0, 100
		fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
		fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
		if limit <= 0 {
			limit = 100
		}
		fetchedOffsets = append(fetchedOffsets, offset)

		type trItem struct {
			Track *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"track"`
		}
		var items []trItem
		for i := offset; i < apiTotal && i < offset+limit; i++ {
			if i == 150 || i == 151 {
				// Simulate 2 deleted/local unavailable tracks with null track payload.
				items = append(items, trItem{Track: nil})
				continue
			}
			items = append(items, trItem{Track: &struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{
				ID:   fmt.Sprintf("edm_%03d", i),
				Name: fmt.Sprintf("Song %03d", i),
			}})
		}
		next := ""
		if offset+len(items) < apiTotal {
			next = fmt.Sprintf("http://%s/v1/playlists/edm/tracks?offset=%d&limit=%d", r.Host, offset+len(items), limit)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"next":  next,
			"total": apiTotal,
			"items": items,
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	var legacyTracks []Track
	for i := 0; i < 100; i++ {
		legacyTracks = append(legacyTracks, Track{ID: fmt.Sprintf("edm_%03d", i), Title: fmt.Sprintf("Song %03d", i)})
	}
	writeLegacyCatalog(t, dir, catalogData{
		Playlists: []Playlist{
			{ID: "edm", Name: "EDM", SnapshotID: "snapEDM", TrackCount: 100, Tracks: legacyTracks},
		},
	})

	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	// Page 0 comes strictly from cache without hitting the API.
	p0, err := cat.PlaylistPage("edm", 0, 100)
	if err != nil {
		t.Fatalf("PlaylistPage(0): %v", err)
	}
	if len(p0.Tracks) != 100 || len(fetchedOffsets) != 0 {
		t.Fatalf("page 0: got %d tracks with fetches=%v, want 100 cached tracks and 0 fetches", len(p0.Tracks), fetchedOffsets)
	}

	// Page 1 fetches offset 100..199 (containing 2 null slots -> 98 valid tracks, NextOffset=200).
	p1, err := cat.PlaylistPage("edm", 100, 100)
	if err != nil {
		t.Fatalf("PlaylistPage(100): %v", err)
	}
	if len(p1.Tracks) != 98 || p1.Total != apiTotal || p1.NextOffset != 200 || p1.Complete {
		t.Fatalf("page 1 = (len=%d total=%d next=%d complete=%v), want (98, %d, 200, false)",
			len(p1.Tracks), p1.Total, p1.NextOffset, p1.Complete, apiTotal)
	}

	// PlaylistTracks fetches ONLY the remaining pages starting at offset 200, never refetching 0..199.
	all, err := cat.PlaylistTracks("edm")
	if err != nil {
		t.Fatalf("PlaylistTracks: %v", err)
	}
	if len(all) != apiTotal-2 {
		t.Fatalf("PlaylistTracks len = %d, want %d", len(all), apiTotal-2)
	}
	wantOffsets := []int{100, 200, 300, 400}
	if fmt.Sprint(fetchedOffsets) != fmt.Sprint(wantOffsets) {
		t.Fatalf("fetched offsets = %v, want %v", fetchedOffsets, wantOffsets)
	}

	// Authoritative TrackCount remains 423 (total) while cached tracks = 421 playable tracks.
	pls := cat.CachedPlaylists()
	if pls[0].TrackCount != apiTotal || !pls[0].TracksComplete {
		t.Fatalf("after completion: TrackCount=%d TracksComplete=%v, want %d/true", pls[0].TrackCount, pls[0].TracksComplete, apiTotal)
	}

	// Repeat PlaylistTracks hits 0 API calls.
	before := len(fetchedOffsets)
	all2, err := cat.PlaylistTracks("edm")
	if err != nil || len(all2) != apiTotal-2 || len(fetchedOffsets) != before {
		t.Fatalf("expected 0 extra fetches on completed playlist, got len=%d fetches=%v", len(all2), fetchedOffsets)
	}
}

func TestPlaylistTrackIDsLightweightAndCancellable(t *testing.T) {
	const apiTotal = 573
	var gotFields []string
	var offsets []int
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/playlists/viet/tracks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		gotFields = append(gotFields, r.URL.Query().Get("fields"))
		offset, limit := 0, 100
		fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
		fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
		if limit <= 0 {
			limit = 100
		}
		offsets = append(offsets, offset)

		var items []map[string]any
		for i := offset; i < apiTotal && i < offset+limit; i++ {
			items = append(items, map[string]any{
				"track": map[string]string{"id": fmt.Sprintf("v_%03d", i)},
			})
		}
		next := ""
		if offset+len(items) < apiTotal {
			next = fmt.Sprintf("http://%s/v1/playlists/viet/tracks?offset=%d&limit=%d&fields=%s",
				r.Host, offset+len(items), limit, r.URL.Query().Get("fields"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"next":  next,
			"total": apiTotal,
			"items": items,
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	var cached100 []Track
	for i := 0; i < 100; i++ {
		cached100 = append(cached100, Track{ID: fmt.Sprintf("v_%03d", i), Title: fmt.Sprintf("Viet %d", i)})
	}
	writeLegacyCatalog(t, dir, catalogData{
		Playlists: []Playlist{
			{ID: "viet", Name: "Viet Song", SnapshotID: "snapV", TrackCount: 573, Tracks: cached100, FetchedOffset: 100},
		},
	})

	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	var streamed []string
	allIDs, err := cat.PlaylistTrackIDs(ctx, "viet", func(batch []string) bool {
		streamed = append(streamed, batch...)
		return true
	})
	if err != nil {
		t.Fatalf("PlaylistTrackIDs: %v", err)
	}
	if len(allIDs) != apiTotal {
		t.Fatalf("PlaylistTrackIDs len = %d, want %d", len(allIDs), apiTotal)
	}
	if len(streamed) != apiTotal-100 {
		t.Fatalf("streamed remaining IDs = %d, want %d", len(streamed), apiTotal-100)
	}
	if fmt.Sprint(offsets) != "[100 200 300 400 500]" {
		t.Fatalf("ID hydration offsets = %v, want [100 200 300 400 500]", offsets)
	}
	for _, f := range gotFields {
		if !strings.Contains(f, "track(id)") {
			t.Fatalf("expected lightweight fields query containing track(id), got %q", f)
		}
	}

	// Second call must return cached TrackIDs with 0 API calls.
	before := len(offsets)
	cachedIDs, err := cat.PlaylistTrackIDs(context.Background(), "viet", nil)
	if err != nil || len(cachedIDs) != apiTotal || len(offsets) != before {
		t.Fatalf("expected cached 573 IDs with 0 requests, got len=%d reqs=%d err=%v", len(cachedIDs), len(offsets)-before, err)
	}

	// Cancelling context stops ID hydration cleanly without erroring out prior progress.
	cancelCtx, cancelNow := context.WithCancel(context.Background())
	dirCancel := t.TempDir()
	writeLegacyCatalog(t, dirCancel, catalogData{
		Playlists: []Playlist{{ID: "viet", Name: "Viet Song", SnapshotID: "snapV", TrackCount: 573}},
	})
	catCancel := NewCatalog(dirCancel, nil, ts.Client())
	catCancel.apiBase = ts.URL + "/v1"
	var partial []string
	_, err = catCancel.PlaylistTrackIDs(cancelCtx, "viet", func(batch []string) bool {
		partial = append(partial, batch...)
		cancelNow()
		return true
	})
	if err == nil || len(partial) != 100 {
		t.Fatalf("expected cancellation after 1 page (100 IDs) with context error, got len=%d err=%v", len(partial), err)
	}
}

func TestLikedSongsOver500AndPaged(t *testing.T) {
	const totalLiked = 620
	var extraNew int
	var reqCount atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/tracks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		reqCount.Add(1)
		offset, limit := 0, 50
		fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
		fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
		if limit <= 0 {
			limit = 50
		}

		total := totalLiked + extraNew
		type item struct {
			Track struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"track"`
		}
		var items []item
		for i := offset; i < total && i < offset+limit; i++ {
			id := fmt.Sprintf("lk_%03d", i-extraNew)
			if i < extraNew {
				id = fmt.Sprintf("new_%d", extraNew-i)
			}
			var it item
			it.Track.ID = id
			it.Track.Name = "Liked " + id
			items = append(items, it)
		}
		next := ""
		if offset+len(items) < total {
			next = fmt.Sprintf("http://%s/v1/me/tracks?offset=%d&limit=%d", r.Host, offset+len(items), limit)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"next":  next,
			"total": total,
			"items": items,
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	// Pre-seed legacy 500-capped LikedTracks (LikedTotal == 0).
	legacy500 := make([]Track, 500)
	for i := range legacy500 {
		legacy500[i] = Track{ID: fmt.Sprintf("lk_%03d", i), Title: fmt.Sprintf("Liked lk_%03d", i)}
	}
	writeLegacyCatalog(t, dir, catalogData{LikedTracks: legacy500})

	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	// Sync page 0 detects Total=620 while preserving the 500-item prefix.
	p0, err := cat.LikedPage(0, 50, true)
	if err != nil {
		t.Fatalf("LikedPage(0, sync=true): %v", err)
	}
	if p0.Total != totalLiked || p0.Complete || len(p0.Tracks) != 50 {
		t.Fatalf("LikedPage(0) = (len=%d total=%d complete=%v), want (50, %d, false)", len(p0.Tracks), p0.Total, p0.Complete, totalLiked)
	}
	if got := reqCount.Load(); got != 1 {
		t.Fatalf("sync page 0 made %d requests, want 1", got)
	}

	// Reading offset 200 comes from migrated cache with 0 extra API calls.
	p200, err := cat.LikedPage(200, 50, false)
	if err != nil || len(p200.Tracks) != 50 || p200.Tracks[0].ID != "lk_200" || reqCount.Load() != 1 {
		t.Fatalf("expected cached page at 200, got len=%d id0=%q reqs=%d err=%v", len(p200.Tracks), p200.Tracks[0].ID, reqCount.Load(), err)
	}

	// LikedSongs completes from offset 500..619 without the 500 cap.
	all, err := cat.LikedSongs()
	if err != nil {
		t.Fatalf("LikedSongs: %v", err)
	}
	if len(all) != totalLiked {
		t.Fatalf("LikedSongs len = %d, want %d", len(all), totalLiked)
	}
	// Requests: 1 (sync page 0) + 1 (LikedSongs head check) + 3 pages (500, 550, 600) = 5 total (NOT 13!).
	if reqCount.Load() > 6 {
		t.Fatalf("expected resume from 500 rather than refetching 0..499, got %d requests", reqCount.Load())
	}

	// Adding 2 new liked songs prepends delta in a single API call and preserves completeness.
	extraNew = 2
	before := reqCount.Load()
	p0New, err := cat.LikedPage(0, 50, true)
	if err != nil {
		t.Fatalf("LikedPage after adding 2 songs: %v", err)
	}
	if reqCount.Load()-before != 1 {
		t.Fatalf("delta liked sync used %d requests, want 1", reqCount.Load()-before)
	}
	if p0New.Total != totalLiked+2 || p0New.NextOffset != 50 || p0New.Tracks[0].ID != "new_2" {
		t.Fatalf("delta liked page = (total=%d next=%d first=%q), want (%d, 50, new_2)",
			p0New.Total, p0New.NextOffset, p0New.Tracks[0].ID, totalLiked+2)
	}
	if got := len(cat.CachedLiked()); got != totalLiked+2 {
		t.Fatalf("CachedLiked len = %d, want %d", got, totalLiked+2)
	}
	// Full LikedSongs after delta prepending needs at most 1 head-check request (0 trailing page fetches).
	beforeFull := reqCount.Load()
	allAfterDelta, err := cat.LikedSongs()
	if err != nil || len(allAfterDelta) != totalLiked+2 || reqCount.Load()-beforeFull > 1 {
		t.Fatalf("LikedSongs after delta = (len=%d extraReqs=%d err=%v), want (%d, <=1, nil)",
			len(allAfterDelta), reqCount.Load()-beforeFull, err, totalLiked+2)
	}
}

func TestSavedAlbumsOver200AndAlbumPageOver50Tracks(t *testing.T) {
	const totalAlbums = 205
	const bigAlbumTracks = 75
	var albumTrackCalls atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/albums":
			offset, limit := 0, 50
			fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
			fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
			if limit <= 0 {
				limit = 50
			}
			var items []map[string]any
			for i := offset; i < totalAlbums && i < offset+limit; i++ {
				id := fmt.Sprintf("al_%03d", i)
				name := fmt.Sprintf("Album %03d", i)
				totalTracks := 10
				embeddedCount := 10
				if i == 0 {
					id, name, totalTracks, embeddedCount = "big_al", "Big Compilation", bigAlbumTracks, 50
				}
				var trs []map[string]any
				for j := 0; j < embeddedCount; j++ {
					trs = append(trs, map[string]any{
						"id":          fmt.Sprintf("%s_t%02d", id, j),
						"name":        fmt.Sprintf("Track %02d", j),
						"duration_ms": 180000,
						"artists":     []map[string]string{{"name": "Various"}},
					})
				}
				items = append(items, map[string]any{
					"album": map[string]any{
						"id":     id,
						"name":   name,
						"images": []map[string]string{{"url": "https://img/cover.jpg"}},
						"tracks": map[string]any{
							"total": totalTracks,
							"items": trs,
						},
					},
				})
			}
			next := ""
			if offset+len(items) < totalAlbums {
				next = fmt.Sprintf("http://%s/v1/me/albums?offset=%d&limit=%d", r.Host, offset+len(items), limit)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"next": next, "total": totalAlbums, "items": items})

		case "/v1/albums/big_al/tracks":
			albumTrackCalls.Add(1)
			offset, limit := 0, 50
			fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &offset)
			fmt.Sscanf(r.URL.Query().Get("limit"), "%d", &limit)
			if limit <= 0 {
				limit = 50
			}
			var items []map[string]any
			for j := offset; j < bigAlbumTracks && j < offset+limit; j++ {
				items = append(items, map[string]any{
					"id":          fmt.Sprintf("big_al_t%02d", j),
					"name":        fmt.Sprintf("Track %02d", j),
					"duration_ms": 180000,
					"artists":     []map[string]string{{"name": "Various"}},
				})
			}
			next := ""
			if offset+len(items) < bigAlbumTracks {
				next = fmt.Sprintf("http://%s/v1/albums/big_al/tracks?offset=%d&limit=%d", r.Host, offset+len(items), limit)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"next": next, "total": bigAlbumTracks, "items": items})

		case "/v1/albums/uncached_al":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"uncached_al","name":"Cold Album","images":[{"url":"https://img/cold.jpg"}],"tracks":{"total":2,"items":[]}}`))

		case "/v1/albums/uncached_al/tracks":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"total":2,"items":[{"id":"u1","name":"Cold 1","artists":[{"name":"Cold Artist"}]},{"id":"u2","name":"Cold 2","artists":[{"name":"Cold Artist"}]}]}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cat := NewCatalog(t.TempDir(), nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	albums, err := cat.SavedAlbums()
	if err != nil {
		t.Fatalf("SavedAlbums: %v", err)
	}
	if len(albums) != totalAlbums {
		t.Fatalf("SavedAlbums len = %d, want %d (must not cap at 200)", len(albums), totalAlbums)
	}

	// Page 0 of big_al uses the 50 inline tracks without hitting /albums/big_al/tracks.
	p0, err := cat.AlbumPage("big_al", 0, 50)
	if err != nil {
		t.Fatalf("AlbumPage(big_al, 0): %v", err)
	}
	if len(p0.Tracks) != 50 || p0.Complete || p0.Total != bigAlbumTracks || albumTrackCalls.Load() != 0 {
		t.Fatalf("big_al page 0 = (len=%d total=%d complete=%v calls=%d), want (50, 75, false, 0)",
			len(p0.Tracks), p0.Total, p0.Complete, albumTrackCalls.Load())
	}

	// Page 1 fetches tracks 50..74 and preserves album name & cover from SavedAlbums.
	p1, err := cat.AlbumPage("big_al", 50, 50)
	if err != nil {
		t.Fatalf("AlbumPage(big_al, 50): %v", err)
	}
	if len(p1.Tracks) != 25 || !p1.Complete || p1.Tracks[0].Album != "Big Compilation" {
		t.Fatalf("big_al page 1 = (len=%d complete=%v album=%q), want (25, true, Big Compilation)",
			len(p1.Tracks), p1.Complete, p1.Tracks[0].Album)
	}

	// Uncached album still opens cleanly via /albums/{id}/tracks.
	cold, err := cat.AlbumPage("uncached_al", 0, 50)
	if err != nil || len(cold.Tracks) != 2 || cold.Tracks[0].Album != "Cold Album" {
		t.Fatalf("uncached album = (len=%d err=%v), want 2 tracks of Cold Album", len(cold.Tracks), err)
	}
}

func TestCatalogTracksByIDs(t *testing.T) {
	var requestedIDs string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tracks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requestedIDs = r.URL.Query().Get("ids")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tracks":[
			{"id":"t2","name":"Song 2","artists":[{"name":"Artist 2"}],"album":{"name":"Album 2"},"duration_ms":150000},
			{"id":"t3","name":"Song 3","artists":[{"name":"Artist 3"}],"album":{"name":"Album 3"},"duration_ms":160000}
		]}`))
	}))
	defer ts.Close()

	dir := t.TempDir()
	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"
	cat.SaveTrack(Track{ID: "t1", Title: "Cached One", Artist: "Artist 1"})

	got, err := cat.TracksByIDs([]string{"t1", "t2", "t3"})
	if err != nil {
		t.Fatalf("TracksByIDs: %v", err)
	}
	if requestedIDs != "t2,t3" {
		t.Fatalf("requested ids = %q, want t2,t3 (skip cached t1)", requestedIDs)
	}
	if len(got) != 3 || got[0].Title != "Cached One" || got[1].Title != "Song 2" || got[2].Title != "Song 3" {
		t.Fatalf("TracksByIDs = %+v", got)
	}
	// t2 should now be cached locally too.
	if tr, ok := cat.findTrack("t2"); !ok || tr.Title != "Song 2" {
		t.Fatalf("expected t2 persisted in catalog cache, got (%+v, %v)", tr, ok)
	}
}

func TestCatalogLikeAndUnlikeTrack(t *testing.T) {
	var likedMethods []string
	var queryIDs []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/tracks" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		likedMethods = append(likedMethods, r.Method)
		queryIDs = append(queryIDs, r.URL.Query().Get("ids"))
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	dir := t.TempDir()
	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	// Initially track is not liked
	if cat.IsTrackLiked("track_123") {
		t.Fatalf("expected track_123 to not be liked initially")
	}

	// Like track
	ctx := context.Background()
	if err := cat.LikeTrack(ctx, "track_123"); err != nil {
		t.Fatalf("LikeTrack error: %v", err)
	}
	if !cat.IsTrackLiked("track_123") {
		t.Fatalf("expected track_123 to be liked after LikeTrack")
	}

	// Unlike track
	if err := cat.UnlikeTrack(ctx, "track_123"); err != nil {
		t.Fatalf("UnlikeTrack error: %v", err)
	}
	if cat.IsTrackLiked("track_123") {
		t.Fatalf("expected track_123 to not be liked after UnlikeTrack")
	}

	if fmt.Sprint(likedMethods) != "[PUT DELETE]" {
		t.Fatalf("methods = %v, want [PUT DELETE]", likedMethods)
	}
	if fmt.Sprint(queryIDs) != "[track_123 track_123]" {
		t.Fatalf("ids = %v, want [track_123 track_123]", queryIDs)
	}
}

func TestCatalogAddTrackToPlaylist(t *testing.T) {
	var receivedBody string
	var targetURL string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetURL = r.URL.Path
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/playlists/") && strings.HasSuffix(r.URL.Path, "/tracks") {
			var buf strings.Builder
			_, _ = io.Copy(&buf, r.Body)
			receivedBody = buf.String()
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"snapshot_id":"snap_new"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	dir := t.TempDir()
	writeLegacyCatalog(t, dir, catalogData{
		Playlists: []Playlist{
			{ID: "pl_rock", Name: "Rock Hits", Tracks: []Track{{ID: "song_1"}}, TrackIDs: []string{"song_1"}, TrackCount: 1},
		},
	})

	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	if err := cat.AddTrackToPlaylist(context.Background(), "pl_rock", "song_2"); err != nil {
		t.Fatalf("AddTrackToPlaylist error: %v", err)
	}

	if targetURL != "/v1/playlists/pl_rock/tracks" {
		t.Fatalf("targetURL = %q, want /v1/playlists/pl_rock/tracks", targetURL)
	}
	if !strings.Contains(receivedBody, "spotify:track:song_2") {
		t.Fatalf("expected body to contain spotify:track:song_2, got %q", receivedBody)
	}

	// Verify cached playlist updated
	pls := cat.CachedPlaylists()
	if len(pls) != 1 || pls[0].TrackCount != 2 || len(pls[0].TrackIDs) != 2 {
		t.Fatalf("cached playlist after add: %+v", pls[0])
	}
}

func TestCatalogRecommendations(t *testing.T) {
	var requestedPath string
	var requestedQuery string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedQuery = r.URL.RawQuery
		if r.URL.Path != "/v1/recommendations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tracks": []map[string]any{
				{
					"id":          "rec1",
					"name":        "Recommended 1",
					"duration_ms": 180000,
					"artists": []map[string]any{
						{"name": "Artist A"},
					},
					"album": map[string]any{
						"name": "Album A",
						"images": []map[string]any{
							{"url": "https://img/rec1.jpg"},
						},
					},
				},
				{
					"id":          "rec2",
					"name":        "Recommended 2",
					"duration_ms": 210000,
					"artists": []map[string]any{
						{"name": "Artist B"},
					},
					"album": map[string]any{
						"name": "Album B",
						"images": []map[string]any{
							{"url": "https://img/rec2.jpg"},
						},
					},
				},
			},
		})
	}))
	defer ts.Close()

	dir := t.TempDir()
	cat := NewCatalog(dir, nil, ts.Client())
	cat.apiBase = ts.URL + "/v1"

	// 1. Pass 6 seeds: must truncate to 5 seeds
	seeds := []string{"s1", "s2", "s3", "s4", "s5", "s6"}
	tracks, err := cat.Recommendations(context.Background(), seeds, 20)
	if err != nil {
		t.Fatalf("Recommendations error: %v", err)
	}
	if requestedPath != "/v1/recommendations" {
		t.Fatalf("requestedPath = %q, want /v1/recommendations", requestedPath)
	}
	if !strings.Contains(requestedQuery, "seed_tracks=s1%2Cs2%2Cs3%2Cs4%2Cs5") || strings.Contains(requestedQuery, "s6") {
		t.Fatalf("requestedQuery = %q, expected truncated 5 seeds (s1..s5 without s6)", requestedQuery)
	}
	if len(tracks) != 2 || tracks[0].ID != "rec1" || tracks[1].ID != "rec2" {
		t.Fatalf("got tracks %+v, want rec1 and rec2", tracks)
	}

	// Verify cached tracks
	cd := cat.loadCached()
	if cd.Tracks == nil || cd.Tracks["rec1"].Title != "Recommended 1" {
		t.Fatalf("expected rec1 in cached tracks, got %+v", cd.Tracks)
	}

	// 2. Fallback on server error
	tsErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsErr.Close()

	catFallback := NewCatalog(dir, nil, tsErr.Client())
	catFallback.apiBase = tsErr.URL + "/v1"
	// Seed cache with a liked track
	writeLegacyCatalog(t, dir, catalogData{
		LikedTracks: []Track{
			{ID: "liked1", Title: "Liked 1", Artist: "Artist L"},
			{ID: "s1", Title: "Seed 1", Artist: "Artist S"},
		},
		LikedTrackIDs: []string{"liked1", "s1"},
	})

	fallbackTracks, err := catFallback.Recommendations(context.Background(), []string{"s1"}, 10)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if len(fallbackTracks) == 0 || fallbackTracks[0].ID != "liked1" {
		t.Fatalf("expected fallback to return liked1 (excluding seed s1), got %+v", fallbackTracks)
	}
}
