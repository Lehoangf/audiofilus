package lyrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHasLyrics verifica la detección de letras existentes (.lrc, .txt, etiquetas).
func TestHasLyrics(t *testing.T) {
	dir := t.TempDir()
	audioNoLrc := filepath.Join(dir, "song1.mp3")
	if err := os.WriteFile(audioNoLrc, []byte("fakeaudio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if HasLyrics(audioNoLrc) {
		t.Fatalf("expected HasLyrics(%q) == false", audioNoLrc)
	}

	// Con archivo .lrc al lado.
	audioWithLrc := filepath.Join(dir, "song2.mp3")
	lrcFile := filepath.Join(dir, "song2.lrc")
	_ = os.WriteFile(audioWithLrc, []byte("fakeaudio"), 0o644)
	_ = os.WriteFile(lrcFile, []byte("[00:01.00]Hello world\n"), 0o644)
	if !HasLyrics(audioWithLrc) {
		t.Fatalf("expected HasLyrics(%q) == true when .lrc exists", audioWithLrc)
	}

	// Con archivo .txt en subcarpeta lyrics/.
	subDir := filepath.Join(dir, "lyrics")
	_ = os.MkdirAll(subDir, 0o755)
	audioWithTxt := filepath.Join(dir, "song3.flac")
	txtFile := filepath.Join(subDir, "song3.txt")
	_ = os.WriteFile(audioWithTxt, []byte("fakeaudio"), 0o644)
	_ = os.WriteFile(txtFile, []byte("Plain text lyrics\n"), 0o644)
	if !HasLyrics(audioWithTxt) {
		t.Fatalf("expected HasLyrics(%q) == true when lyrics/song3.txt exists", audioWithTxt)
	}

	// Con etiquetas embebidas en testdata/tags.flac.
	absTagsFlac, _ := filepath.Abs("testdata/tags.flac")
	if !HasLyrics(absTagsFlac) {
		t.Fatalf("expected HasLyrics(%q) == true for embedded tags", absTagsFlac)
	}
}

// TestSaveLRC verifica la escritura atómica del archivo .lrc junto al audio.
func TestSaveLRC(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "track.mp3")
	_ = os.WriteFile(audio, []byte("audio"), 0o644)

	wantLrc := filepath.Join(dir, "track.lrc")
	content := "[00:05.00]Line 1\n[00:10.00]Line 2\n"

	if err := SaveLRC(audio, content); err != nil {
		t.Fatalf("SaveLRC failed: %v", err)
	}
	got, err := os.ReadFile(wantLrc)
	if err != nil {
		t.Fatalf("failed reading saved lrc: %v", err)
	}
	if string(got) != content {
		t.Fatalf("got %q, want %q", string(got), content)
	}
}

// TestFetchAndSaveLRC verifica la búsqueda en LRCLIB y guardado automático en disco.
func TestFetchAndSaveLRC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			http.NotFound(w, r)
			return
		}
		track := r.URL.Query().Get("track_name")
		if track == "FoundSong" {
			_ = json.NewEncoder(w).Encode([]Found{
				{
					ID:       123,
					Track:    "FoundSong",
					Artist:   "Artist",
					Synced:   "[00:02.00]Synced lyric\n",
					Plain:    "Plain lyric\n",
					Duration: 180,
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]Found{})
	}))
	defer srv.Close()

	web := &LRCLIB{HTTP: srv.Client(), Base: srv.URL}
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "lyrics_store"))

	audio := filepath.Join(dir, "FoundSong.mp3")
	_ = os.WriteFile(audio, []byte("audio"), 0o644)

	q := Query{Title: "FoundSong", Artist: "Artist", Duration: 180}
	ok, err := FetchAndSaveLRC(context.Background(), web, store, q, audio)
	if err != nil {
		t.Fatalf("FetchAndSaveLRC failed: %v", err)
	}
	if !ok {
		t.Fatal("expected FetchAndSaveLRC to return true")
	}

	lrcPath := filepath.Join(dir, "FoundSong.lrc")
	data, err := os.ReadFile(lrcPath)
	if err != nil {
		t.Fatalf("expected .lrc file to exist: %v", err)
	}
	if string(data) != "[00:02.00]Synced lyric\n" {
		t.Fatalf("unexpected lrc content: %q", string(data))
	}
	if !HasLyrics(audio) {
		t.Fatal("expected HasLyrics to be true after save")
	}

	// Caso de canción no encontrada.
	audioMissing := filepath.Join(dir, "OtherSong.mp3")
	_ = os.WriteFile(audioMissing, []byte("audio"), 0o644)
	qMissing := Query{Title: "OtherSong", Artist: "Artist"}
	okMiss, errMiss := FetchAndSaveLRC(context.Background(), web, store, qMissing, audioMissing)
	if errMiss != nil {
		t.Fatalf("expected nil err on miss, got: %v", errMiss)
	}
	if okMiss {
		t.Fatal("expected ok == false on miss")
	}
}
