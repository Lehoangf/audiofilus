package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanvillacortac/audiofilus/internal/config"
	"github.com/juanvillacortac/audiofilus/internal/lyrics"
	"github.com/juanvillacortac/audiofilus/internal/ytm"
)

// TestBatchLyricsFilter verifica que filterMissingLyrics descarte canciones remotas
// y canciones que ya cuenten con letra (.lrc, .txt o etiquetas).
func TestBatchLyricsFilter(t *testing.T) {
	dir := t.TempDir()

	// Canción 1: sin letra.
	song1 := filepath.Join(dir, "song1.mp3")
	_ = os.WriteFile(song1, []byte("audio"), 0o644)

	// Canción 2: con archivo .lrc al lado.
	song2 := filepath.Join(dir, "song2.mp3")
	_ = os.WriteFile(song2, []byte("audio"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "song2.lrc"), []byte("[00:01.00]Lyric"), 0o644)

	songs := []Song{
		{Path: song1, Title: "Song 1", Artist: "Artist"},
		{Path: song2, Title: "Song 2", Artist: "Artist"},
		{Path: "radio://uuid", Title: "Radio", Artist: "Radio"},
		{Path: "spotify:track:123", Title: "Spot", Artist: "Spot"},
	}

	missing := filterMissingLyrics(songs)
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing song, got %d", len(missing))
	}
	if missing[0].Path != song1 {
		t.Fatalf("expected %q, got %q", song1, missing[0].Path)
	}
}

// TestNowMenuBatchLyricsItem verifica que el menú de Now Playing incluya la opción
// de descargar letras para una canción local.
func TestNowMenuBatchLyricsItem(t *testing.T) {
	SetLanguage("en")
	dir := t.TempDir()
	songPath := filepath.Join(dir, "song.mp3")
	_ = os.WriteFile(songPath, []byte("audio"), 0o644)

	a := &App{
		offline:     true,
		playingSong: Song{Path: songPath, Title: "Test Song", Artist: "Test Artist"},
	}
	a.openNowMenu()
	if a.menu == nil {
		t.Fatal("expected now menu to open")
	}

	var found bool
	for _, it := range a.menu.items {
		if it.label == T("lyrics_download_current") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing %q in openNowMenu items", T("lyrics_download_current"))
	}
}

// TestLibraryBatchLyricsMenu verifica que el menú de álbum incluya la opción
// de descargar letras faltantes.
func TestLibraryBatchLyricsMenu(t *testing.T) {
	SetLanguage("en")
	dir := t.TempDir()
	s1 := filepath.Join(dir, "1.mp3")
	_ = os.WriteFile(s1, []byte("audio"), 0o644)

	al := &Album{
		Title:  "Album",
		Artist: "Artist",
		Songs:  []Song{{Path: s1, Title: "Track 1"}},
	}
	a := &App{
		offline: true,
		source:  0,
		menu:    &menu{},
	}
	v := &view{
		kind:   vAlbums,
		albums: []*Album{al},
		sel:    0,
	}
	a.addMetaItems(v)

	var found bool
	for _, it := range a.menu.items {
		if it.label == T("lyrics_download_missing") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing %q in addMetaItems for album", T("lyrics_download_missing"))
	}
}

// TestSettingsBatchLyricsTile verifica que Ajustes contenga la baldosa de descarga
// de letras de biblioteca.
func TestSettingsBatchLyricsTile(t *testing.T) {
	SetLanguage("en")
	var found bool
	for _, sec := range settingSections {
		for _, tile := range sec.tiles {
			if tile.id == setFetchLyrics {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected setFetchLyrics tile in settingSections")
	}

	label := settingNames()[setFetchLyrics]
	if label == "" {
		t.Fatal("expected non-empty label for setFetchLyrics")
	}
}

// TestAutoFetchLyricsHook verifica la descarga y guardado automático de .lrc
// cuando termina una descarga de pista.
func TestAutoFetchLyricsHook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]lyrics.Found{
			{
				ID:       555,
				Track:    "AutoSong",
				Artist:   "AutoArtist",
				Synced:   "[00:03.00]Auto synced\n",
				Duration: 120,
			},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "AutoSong.m4a")
	_ = os.WriteFile(audioPath, []byte("audio"), 0o644)

	a := &App{
		offline: false,
		paths:   config.Paths{Data: filepath.Join(dir, "data")},
	}
	a.lyrFinder = &lyrics.Finder{
		Store: lyrics.NewStore(filepath.Join(dir, "data", "lyrics")),
		Web:   &lyrics.LRCLIB{HTTP: srv.Client(), Base: srv.URL},
	}

	tr := ytm.Track{
		ID:      "spot-123",
		Title:   "AutoSong",
		Artists: []string{"AutoArtist"},
		Album:   "AutoAlbum",
	}

	a.autoFetchLyricsSync(audioPath, tr)

	lrcPath := filepath.Join(dir, "AutoSong.lrc")
	b, err := os.ReadFile(lrcPath)
	if err != nil {
		t.Fatalf("expected auto-saved .lrc file: %v", err)
	}
	if string(b) != "[00:03.00]Auto synced\n" {
		t.Fatalf("unexpected content: %q", string(b))
	}
}
