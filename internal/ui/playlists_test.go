package ui

import (
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juanvillacortac/audiofilus/internal/config"
	"github.com/juanvillacortac/audiofilus/internal/library"
)

// TestSavePlaylistRoundTrip comprueba que una lista guardada se vuelve a leer
// igual, con rutas relativas para lo que está en su carpeta.
func TestSavePlaylistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "música", "a.mp3")
	outside := filepath.Join(t.TempDir(), "otra", "b.flac") // absoluta en cualquier sistema
	pl := &Playlist{Name: "Gym", Path: filepath.Join(dir, "Gym.m3u8"), Songs: []Song{{Path: inside, Title: "A"}, {Path: outside, Title: "B"}}}
	a := &App{paths: config.Paths{Data: dir}}
	if err := a.savePlaylist(pl); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(pl.Path)
	if !strings.Contains(string(b), "\nmúsica/a.mp3\n") || !strings.Contains(string(b), "\n"+outside+"\n") {
		t.Errorf("unexpected paths in playlist:\n%s", b)
	}
	got, err := library.Expand([]string{pl.Path})
	if err != nil || len(got) != 2 || got[0] != inside || got[1] != outside {
		t.Errorf("read back %v (%v), want [%s %s]", got, err, inside, outside)
	}
}

// TestPlaylistFile comprueba que el nombre de archivo vale en exFAT y no pisa
// una lista que ya existe.
func TestPlaylistFile(t *testing.T) {
	dir := t.TempDir()
	p := playlistFile(dir, `AC/DC: "Lo mejor"?`, ".m3u8")
	if filepath.Base(p) != "AC_DC_ _Lo mejor__.m3u8" {
		t.Errorf("got %q", filepath.Base(p))
	}
	os.WriteFile(p, nil, 0o644)
	if q := playlistFile(dir, `AC/DC: "Lo mejor"?`, ".m3u8"); filepath.Base(q) != "AC_DC_ _Lo mejor__ 2.m3u8" {
		t.Errorf("second got %q", filepath.Base(q))
	}
}

// TestTitleCase comprueba el paso de lo escrito en el teclado en pantalla.
func TestTitleCase(t *testing.T) {
	if got := titleCase("MI LISTA  DEL GYM"); got != "Mi Lista Del Gym" {
		t.Errorf("got %q", got)
	}
}

// TestSongKey comprueba que una canción de YouTube Music casa con la de la
// biblioteca aunque cambien el feat., la versión o las tildes.
func TestSongKey(t *testing.T) {
	same := [][4]string{
		{"Chevelle", "The Red", "Chevelle", "The Red (Remastered 2021)"},
		{"Bad Bunny, Jhay Cortez", "Dákiti", "Bad Bunny", "Dakiti - Live"},
		{"Linkin Park", "Numb [Official Audio]", "Linkin Park", "Numb"},
	}
	for _, c := range same {
		if songKey(c[0], c[1]) != songKey(c[2], c[3]) {
			t.Errorf("%q/%q should match %q/%q", c[0], c[1], c[2], c[3])
		}
	}
	if songKey("Chevelle", "The Red") == songKey("Chevelle", "The Clincher") {
		t.Error("different songs match")
	}
}

// TestDecodeCoverImageFile comprueba que una portada en un archivo de imagen
// suelto (la miniatura de YouTube Music) se lee, con sus colores.
func TestDecodeCoverImageFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.m4a.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, _ := os.Create(path)
	jpeg.Encode(f, img, nil)
	f.Close()
	al := &Album{coverPath: path, stray: true}
	if al.decodeCover() == nil {
		t.Fatal("cover not decoded")
	}
	if _, _, ok := al.Accent(); !ok {
		t.Error("no accent colors")
	}
}

// TestCoverOfStrayAlbum comprueba que la portada de un álbum de paso (lo que
// suena de YouTube Music) se carga sin romper: antes, su mapa de portadas
// estaba sin crear y la interfaz se caía al llegar la portada.
func TestCoverOfStrayAlbum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.m4a.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	f, _ := os.Create(path)
	jpeg.Encode(f, img, nil)
	f.Close()
	c := newCovers(1)
	al := &Album{coverPath: path, stray: true}
	c.request(al, 16)
	for i := 0; i < 200 && al.scaled[16] == nil; i++ {
		c.collect(float64(i))
		time.Sleep(5 * time.Millisecond)
	}
	if al.scaled[16] == nil {
		t.Fatal("cover never arrived")
	}
}

// TestPlaylistIconAndSpotifyURIs comprueba que el icono personalizado y las rutas virtuales de Spotify
// se guardan en el archivo .m3u8 y se cargan correctamente en la biblioteca.
func TestPlaylistIconAndSpotifyURIs(t *testing.T) {
	dir := t.TempDir()
	plPath := filepath.Join(dir, "TestSpot.m3u8")
	pl := &Playlist{
		Name: "TestSpot",
		Path: plPath,
		icon: "spotify",
		Songs: []Song{
			{Path: "spotify://track/123", Title: "Song 1", Artist: "Artist 1"},
		},
	}
	a := &App{}
	if err := a.savePlaylist(pl); err != nil {
		t.Fatalf("savePlaylist failed: %v", err)
	}
	b, err := os.ReadFile(plPath)
	if err != nil {
		t.Fatalf("read playlist file failed: %v", err)
	}
	if !strings.Contains(string(b), "#EXT-AUDIOFILUS-ICON:spotify") {
		t.Fatalf("expected #EXT-AUDIOFILUS-ICON:spotify in file, got:\n%s", string(b))
	}
	if !strings.Contains(string(b), "spotify://track/123") {
		t.Fatalf("expected spotify://track/123 in file, got:\n%s", string(b))
	}
	icon := readPlaylistIcon(plPath)
	if icon != "spotify" {
		t.Fatalf("expected icon 'spotify', got %q", icon)
	}

	lib := &Library{song: map[string]Song{}}
	lib.addPlaylists([]string{plPath})
	if len(lib.Playlists) != 1 {
		t.Fatalf("expected 1 playlist in library, got %d", len(lib.Playlists))
	}
	if lib.Playlists[0].icon != "spotify" {
		t.Fatalf("expected lib playlist icon 'spotify', got %q", lib.Playlists[0].icon)
	}
	if len(lib.Playlists[0].Songs) != 1 || lib.Playlists[0].Songs[0].Path != "spotify://track/123" {
		t.Fatalf("expected 1 spotify song in lib playlist, got %+v", lib.Playlists[0].Songs)
	}
}
