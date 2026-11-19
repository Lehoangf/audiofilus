package spotify

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDecryptSpotifyOgg(t *testing.T) {
	key := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	expectedData := []byte("OggS_valid_vorbis_stream_content")
	// Spotify container: 167 bytes header + AES-128-CTR encrypted payload
	header := make([]byte, 167)
	block, _ := aes.NewCipher(key[:])
	iv := make([]byte, 16) // Counter starts at 0
	stream := cipher.NewCTR(block, iv)
	encryptedPayload := make([]byte, len(expectedData))
	stream.XORKeyStream(encryptedPayload, expectedData)

	fullData := append(header, encryptedPayload...)
	decrypted, err := DecryptSpotifyOgg(bytes.NewReader(fullData), key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	out, _ := io.ReadAll(decrypted)
	if !bytes.Equal(out, expectedData) {
		t.Errorf("got %q, want %q", out, expectedData)
	}
}

func TestOfflineCacheRetrieval(t *testing.T) {
	tempDir := t.TempDir()
	sr := NewStreamResolver("", tempDir, nil, nil)

	tr := Track{
		ID:       "offline123",
		Title:    "Offline Song",
		Artist:   "Cached Artist",
		Duration: 3 * time.Minute,
	}

	// Simulate track already downloaded in cache
	expectedPath := filepath.Join(tempDir, "songs", tr.ID+".audio")
	_ = os.MkdirAll(filepath.Dir(expectedPath), 0o755)
	_ = os.WriteFile(expectedPath, []byte("fake_audio_stream"), 0o644)

	path, ready, err := sr.Resolve(tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready || path != expectedPath {
		t.Errorf("expected cached path %s and ready=true, got ready=%v, path=%s", expectedPath, ready, path)
	}
}

func TestFallbackMatchFailure(t *testing.T) {
	tempDir := t.TempDir()
	sr := NewStreamResolver("", tempDir, nil, nil)
	// Mock fallback solver that finds no match
	sr.ytmFinder = func(query string) (string, error) {
		return "", os.ErrNotExist
	}

	tr := Track{
		ID:       "notfound404",
		Title:    "Obscure Track",
		Artist:   "Unknown",
		Duration: 2 * time.Minute,
	}

	_, ready, err := sr.Resolve(tr)
	if err == nil {
		t.Errorf("expected error when fallback matching fails, got nil")
	}
	if ready {
		t.Errorf("expected ready=false on fallback match failure")
	}
}

func TestDownloadSerialization(t *testing.T) {
	tmpDir := t.TempDir()
	sr := NewStreamResolver(tmpDir, tmpDir, nil, nil)

	var active int64
	var maxActive int64

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&active, 1)
		for {
			old := atomic.LoadInt64(&maxActive)
			if cur <= old || atomic.CompareAndSwapInt64(&maxActive, old, cur) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt64(&active, -1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake audio content"))
	}))
	defer ts.Close()

	sr.ytmFinder = func(query string) (string, error) {
		return ts.URL, nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tr := Track{ID: fmt.Sprintf("track%d", idx), Title: "Song", Artist: "Artist"}
			_, _, _ = sr.ResolveSync(tr)
		}(i)
	}
	wg.Wait()

	if maxActive != 1 {
		t.Errorf("expected maxActive == 1 (serialized downloads), got %d", maxActive)
	}
}

func TestYTMapCachePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	sr := NewStreamResolver(tmpDir, tmpDir, nil, nil)

	// Save mapping and verify persistence to disk
	sr.saveMappedYT("spotify123", "youtubeABC")
	if got := sr.getMappedYT("spotify123"); got != "youtubeABC" {
		t.Fatalf("getMappedYT = %q, want youtubeABC", got)
	}

	// Create new instance on the same directory to test loading from disk
	sr2 := NewStreamResolver(tmpDir, tmpDir, nil, nil)
	if got := sr2.getMappedYT("spotify123"); got != "youtubeABC" {
		t.Fatalf("persisted getMappedYT = %q, want youtubeABC", got)
	}

	// Test deletion and invalidation
	sr2.deleteMappedYT("spotify123")
	if got := sr2.getMappedYT("spotify123"); got != "" {
		t.Fatalf("deleted getMappedYT = %q, want empty", got)
	}
}

func TestPrefetchCancellationOnForegroundDemand(t *testing.T) {
	tmpDir := t.TempDir()
	sr := NewStreamResolver(tmpDir, tmpDir, nil, nil)

	trNext := Track{ID: "prefetchNext", Title: "Next Track", Artist: "Artist"}
	sr.Prefetch(trNext)

	sr.mu.Lock()
	if sr.prefetchID != "prefetchNext" {
		sr.mu.Unlock()
		t.Fatalf("prefetchID = %q, want prefetchNext", sr.prefetchID)
	}
	sr.mu.Unlock()

	// When user requests playback for another track in foreground, ongoing prefetch must cancel immediately.
	trActive := Track{ID: "foregroundActive", Title: "Active Track", Artist: "Artist"}
	_, _, _ = sr.Resolve(trActive)

	sr.mu.Lock()
	if sr.prefetchID == "prefetchNext" {
		t.Errorf("prefetchID was not cancelled upon foreground demand, got %q", sr.prefetchID)
	}
	sr.mu.Unlock()
}

func TestLocalMusicDirsResolution(t *testing.T) {
	musicDir := t.TempDir()
	cacheDir := t.TempDir()

	sr := NewStreamResolver("", cacheDir, nil, nil)
	sr.SetMusicDirs([]string{musicDir})
	sr.ytmFinder = func(query string) (string, error) {
		t.Fatalf("ytmFinder should not be called for locally available tracks (query=%s)", query)
		return "", nil
	}

	// 1. Test resolution via .library.json mapping
	libTrackFile := filepath.Join(musicDir, "RockBand", "Album1", "Song1.m4a")
	_ = os.MkdirAll(filepath.Dir(libTrackFile), 0o755)
	_ = os.WriteFile(libTrackFile, []byte("fake_m4a_audio_data"), 0o644)

	rel, _ := filepath.Rel(musicDir, libTrackFile)
	libMap := map[string]string{
		"spotify:trackID123": rel,
	}
	libJSON, _ := json.Marshal(libMap)
	_ = os.WriteFile(filepath.Join(musicDir, ".library.json"), libJSON, 0o644)

	tr1 := Track{ID: "trackID123", Title: "Different Title", Artist: "Different Artist"}
	p, ready, err := sr.Resolve(tr1)
	if err != nil {
		t.Fatalf("resolve tr1: %v", err)
	}
	if !ready || p != libTrackFile {
		t.Errorf("expected local path %q and ready=true, got ready=%v, path=%q", libTrackFile, ready, p)
	}

	// 2. Test resolution via conventional file path (Artist/Singles/Title.m4a)
	convTrackFile := filepath.Join(musicDir, "The Beatles", "Singles", "Yesterday.m4a")
	_ = os.MkdirAll(filepath.Dir(convTrackFile), 0o755)
	_ = os.WriteFile(convTrackFile, []byte("fake_beatles_m4a"), 0o644)

	tr2 := Track{ID: "yesterday999", Title: "Yesterday", Artist: "The Beatles"}
	p2, ready2, err := sr.Resolve(tr2)
	if err != nil {
		t.Fatalf("resolve tr2: %v", err)
	}
	if !ready2 || p2 != convTrackFile {
		t.Errorf("expected local path %q and ready=true, got ready=%v, path=%q", convTrackFile, ready2, p2)
	}
}

func TestMappedYTDotLibAndScanCacheMatch(t *testing.T) {
	musicDir := t.TempDir()
	dataDir := t.TempDir()
	cacheDir := filepath.Join(dataDir, "cache", "spotify")

	// 1. Track registered in .library.json under its mapped YouTube ID only.
	ytFile := filepath.Join(musicDir, "ArtistX", "Singles", "TrackYT.m4a")
	_ = os.MkdirAll(filepath.Dir(ytFile), 0o755)
	_ = os.WriteFile(ytFile, []byte("yt_m4a"), 0o644)

	rel, _ := filepath.Rel(musicDir, ytFile)
	dotLibJSON, _ := json.Marshal(map[string]string{"ytVid99": rel})
	_ = os.WriteFile(filepath.Join(musicDir, ".library.json"), dotLibJSON, 0o644)

	sr := NewStreamResolver("", cacheDir, nil, nil)
	sr.SetDataDir(dataDir)
	sr.SetMusicDirs([]string{musicDir})
	sr.saveMappedYT("spMapped1", "ytVid99")
	sr.ytmFinder = func(query string) (string, error) {
		t.Fatalf("ytmFinder should not be called when local file matches (%s)", query)
		return "", nil
	}

	p, ready, err := sr.Resolve(Track{ID: "spMapped1", Title: "Unrelated", Artist: "Other"})
	if err != nil || !ready || p != ytFile {
		t.Fatalf("mapped YT dotlib resolve = (%q, %v, %v), want %q", p, ready, err, ytFile)
	}

	// 2. Track matched via data/library.json tags under unrelated filename/folder structure.
	taggedFile := filepath.Join(musicDir, "RandomDump", "01 - Track_Tag.flac")
	_ = os.MkdirAll(filepath.Dir(taggedFile), 0o755)
	_ = os.WriteFile(taggedFile, []byte("flac_data"), 0o644)

	// Remix file must NOT be confused with the studio track.
	remixFile := filepath.Join(musicDir, "RandomDump", "02 - Track_Remix.flac")
	_ = os.WriteFile(remixFile, []byte("flac_remix"), 0o644)

	scanJSON, _ := json.Marshal(map[string]any{
		"v": 2,
		"entries": []map[string]any{
			{"path": remixFile, "title": "Có Hẹn Với Thanh Xuân (Remix)", "artist": "MONSTAR"},
			{"path": taggedFile, "title": "Có Hẹn Với Thanh Xuân (feat. GREY D)", "artist": "MONSTAR"},
		},
	})
	_ = os.WriteFile(filepath.Join(dataDir, "library.json"), scanJSON, 0o644)

	p2, ready2, err2 := sr.Resolve(Track{ID: "spScan1", Title: "Có Hẹn Với Thanh Xuân", Artist: "MONSTAR, GREY D"})
	if err2 != nil || !ready2 || p2 != taggedFile {
		t.Fatalf("scan cache resolve = (%q, %v, %v), want %q", p2, ready2, err2, taggedFile)
	}
}

func TestDynamicMusicDirsFunc(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	cacheDir := t.TempDir()

	curDirs := []string{dir1}
	sr := NewStreamResolver("", cacheDir, nil, nil)
	sr.SetMusicDirsFunc(func() []string { return curDirs })
	sr.ytmFinder = func(query string) (string, error) {
		return "", os.ErrNotExist
	}

	tr := Track{ID: "dyn1", Title: "Dyn Song", Artist: "Dyn Artist"}
	if _, ready, _ := sr.Resolve(tr); ready {
		t.Fatal("expected ready=false before adding dir2")
	}

	f := filepath.Join(dir2, "Dyn Artist", "Dyn Song.flac")
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	_ = os.WriteFile(f, []byte("flac"), 0o644)
	curDirs = []string{dir1, dir2}

	p, ready, err := sr.Resolve(tr)
	if err != nil || !ready || p != f {
		t.Fatalf("dynamic MusicDirsFunc resolve = (%q, %v, %v), want %q", p, ready, err, f)
	}
}
