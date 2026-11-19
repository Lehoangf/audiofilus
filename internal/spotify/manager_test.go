package spotify

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerIsSpotifyAndExtract(t *testing.T) {
	if !IsSpotify("spotify://track/abc1234") {
		t.Errorf("expected true for spotify://track/abc1234")
	}
	if !IsSpotify("spotify:track:xyz789") {
		t.Errorf("expected true for spotify:track:xyz789")
	}
	if IsSpotify("https://open.spotify.com/track/123") {
		t.Errorf("expected false for web URL")
	}

	if id := ExtractTrackID("spotify://track/abc1234"); id != "abc1234" {
		t.Errorf("got %s, want abc1234", id)
	}
	if id := ExtractTrackID("spotify:track:xyz789"); id != "xyz789" {
		t.Errorf("got %s, want xyz789", id)
	}
	if id := ExtractTrackID("spotify://track/abc1234?smart=1"); id != "abc1234" {
		t.Errorf("got %s, want abc1234 for smart track", id)
	}
	if !IsSmartTrack("spotify://track/abc1234?smart=1") {
		t.Errorf("expected IsSmartTrack to be true for uri with smart=1")
	}
	if IsSmartTrack("spotify://track/abc1234") {
		t.Errorf("expected IsSmartTrack to be false for normal uri")
	}
}

func TestManagerTrackRegistration(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir, tempDir, "client_id", "client_secret")

	tr := Track{
		ID:       "track123",
		Title:    "Test Song",
		Artist:   "Test Artist",
		Album:    "Test Album",
		Duration: 3 * time.Minute,
	}

	mgr.RegisterTrack(tr)
	found, ok := mgr.Track("spotify://track/track123")
	if !ok {
		t.Fatalf("expected track to be found")
	}
	if found.Title != tr.Title || found.Artist != tr.Artist {
		t.Errorf("got %+v, want %+v", found, tr)
	}
}

func TestManagerSourceCached(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir, tempDir, "client_id", "client_secret")

	tr := Track{
		ID:       "cachedtrack",
		Title:    "Cached Song",
		Artist:   "Artist",
		Duration: 2 * time.Minute,
	}
	mgr.RegisterTrack(tr)

	// Pre-create cache song
	songsDir := filepath.Join(tempDir, "cache", "spotify", "songs")
	_ = os.MkdirAll(songsDir, 0o755)
	testAudioFile := filepath.Join(songsDir, tr.ID+".audio")
	_ = os.WriteFile(testAudioFile, []byte("RIFF....WAVEfmt "), 0o644)

	// Source should attempt to decode. Since it's invalid wave, it should fail or clean up.
	_, ready, err := mgr.Source("spotify://track/" + tr.ID)
	if err == nil {
		t.Errorf("expected error decoding fake wave file")
	}
	if ready {
		t.Errorf("expected ready=false on decode error")
	}
}

func TestManagerLastError(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir, tempDir, "client_id", "client_secret")
	if mgr.LastError() != "" {
		t.Errorf("expected empty initial error, got %s", mgr.LastError())
	}
}

func TestManagerOnMetadataResolved(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir, tempDir, "client_id", "client_secret")

	var notified Track
	mgr.OnMetadataResolved = func(tr Track) {
		notified = tr
	}

	tr := Track{
		ID:     "abc999",
		Title:  "Resolved Title",
		Artist: "Resolved Artist",
	}
	mgr.RegisterTrack(tr)

	if notified.ID != tr.ID || notified.Title != tr.Title {
		t.Errorf("expected callback with track %+v, got %+v", tr, notified)
	}

	// Verify track is returned by Track(id)
	found, ok := mgr.Track("spotify://track/" + tr.ID)
	if !ok || found.Title != tr.Title {
		t.Errorf("expected track to be retrieved from manager")
	}
}

func TestManagerTrackFromDiskCacheDoesNotTriggerCallback(t *testing.T) {
	tempDir := t.TempDir()
	mgr := NewManager(tempDir, tempDir, "client_id", "client_secret")
	tr := Track{
		ID:     "disk123",
		Title:  "Disk Song",
		Artist: "Disk Artist",
	}
	mgr.RegisterTrack(tr)

	// Create a new manager instance pointing to the same disk cache
	mgr2 := NewManager(tempDir, tempDir, "client_id", "client_secret")
	called := false
	mgr2.OnMetadataResolved = func(tr Track) {
		called = true
	}

	found, ok := mgr2.Track("spotify://track/" + tr.ID)
	if !ok || found.Title != tr.Title {
		t.Fatalf("expected track from disk cache, got %+v", found)
	}
	if called {
		t.Errorf("OnMetadataResolved should not be triggered during read-only Track() lookup")
	}
}

func TestManagerSourcePreservesLocalLibraryFileOnDecodeError(t *testing.T) {
	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "data")
	musicDir := t.TempDir()

	mgr := NewManager(baseDir, dataDir, "cid", "csec")
	mgr.SetMusicDirs([]string{musicDir})
	mgr.RegisterTrack(Track{ID: "localbad1", Title: "Bad Local", Artist: "Artist"})

	localFile := filepath.Join(musicDir, "Artist", "Bad Local.flac")
	_ = os.MkdirAll(filepath.Dir(localFile), 0o755)
	_ = os.WriteFile(localFile, []byte("not_really_flac_bytes"), 0o644)

	_, _, err := mgr.Source("spotify://track/localbad1")
	if err == nil {
		t.Fatal("expected decode error for invalid local audio")
	}
	if _, statErr := os.Stat(localFile); statErr != nil {
		t.Fatalf("local library file outside songs cache was deleted: %v", statErr)
	}
}

func TestManagerCoverFallsBackToSiblingOrLocalAudio(t *testing.T) {
	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "data")
	musicDir := t.TempDir()

	mgr := NewManager(baseDir, dataDir, "cid", "csec")
	mgr.SetMusicDirs([]string{musicDir})

	// 1. Sibling cover.jpg next to local track.
	mgr.RegisterTrack(Track{ID: "cov1", Title: "Cov Song", Artist: "Cov Artist", Album: "Cov Album"})
	song1 := filepath.Join(musicDir, "Cov Artist", "Cov Album", "Cov Song.m4a")
	cov1 := filepath.Join(filepath.Dir(song1), "cover.jpg")
	_ = os.MkdirAll(filepath.Dir(song1), 0o755)
	_ = os.WriteFile(song1, []byte("m4a"), 0o644)
	_ = os.WriteFile(cov1, []byte("jpg"), 0o644)

	if got := mgr.Cover("spotify://track/cov1"); got != cov1 {
		t.Errorf("Cover with sibling cover.jpg = %q, want %q", got, cov1)
	}

	// 2. Local audio file directly when no standalone image exists (for embedded artwork decoding).
	mgr.RegisterTrack(Track{ID: "cov2", Title: "Embedded Song", Artist: "Cov Artist 2"})
	song2 := filepath.Join(musicDir, "Cov Artist 2", "Embedded Song.m4a")
	_ = os.MkdirAll(filepath.Dir(song2), 0o755)
	_ = os.WriteFile(song2, []byte("m4a_embedded"), 0o644)

	if got := mgr.Cover("spotify://track/cov2"); got != song2 {
		t.Errorf("Cover without image = %q, want local audio %q", got, song2)
	}
}

func TestManagerTrackSearchesCatalogPlaylists(t *testing.T) {
	baseDir := t.TempDir()
	dataDir := filepath.Join(baseDir, "data")
	mgr := NewManager(baseDir, dataDir, "cid", "csec")

	_ = mgr.Catalog.saveCached(catalogData{
		Playlists: []Playlist{
			{
				ID:   "pl1",
				Name: "Chill",
				Tracks: []Track{
					{ID: "catTr99", Title: "Playlist Track", Artist: "Pl Artist"},
				},
			},
		},
	})

	got, ok := mgr.Track("spotify://track/catTr99")
	if !ok || got.Title != "Playlist Track" {
		t.Fatalf("Track from Catalog.Playlists = (%+v, %v), want Playlist Track", got, ok)
	}
}
