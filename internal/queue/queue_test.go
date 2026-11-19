package queue

import (
	"slices"
	"testing"
)

var songs = []string{"a", "b", "c", "d"}

// TestLinearAndRepeat comprueba el avance y los tres modos de repetición.
func TestLinearAndRepeat(t *testing.T) {
	q := New()
	q.Set(songs, 0)
	for _, want := range []string{"b", "c", "d"} {
		if !q.Next(true) || q.Current() != want {
			t.Fatalf("Next -> %q, want %q", q.Current(), want)
		}
	}
	if q.Next(true) {
		t.Fatal("Next at the end with repeat off should stop")
	}
	q.Repeat = RepeatAll
	if !q.Next(true) || q.Current() != "a" {
		t.Fatalf("repeat all should wrap to a, got %q", q.Current())
	}
	q.Repeat = RepeatOne
	if !q.Next(true) || q.Current() != "a" {
		t.Fatal("repeat one should stay on the song when it ends")
	}
	if !q.Next(false) || q.Current() != "b" {
		t.Fatal("a manual skip should move on even with repeat one")
	}
}

// TestPrev comprueba ir atrás, con y sin repetición.
func TestPrev(t *testing.T) {
	q := New()
	q.Set(songs, 2)
	if !q.Prev() || q.Current() != "b" {
		t.Fatal("Prev from c should go to b")
	}
	q.Set(songs, 0)
	if q.Prev() {
		t.Fatal("Prev at the start with repeat off should fail")
	}
	q.Repeat = RepeatAll
	if !q.Prev() || q.Current() != "d" {
		t.Fatal("Prev at the start with repeat all should wrap to d")
	}
}

// TestShuffleKeepsCurrentAndAllSongs comprueba que el aleatorio no pierda
// canciones ni cambie la que suena, y que al quitarlo vuelva el orden original.
func TestShuffleKeepsCurrentAndAllSongs(t *testing.T) {
	q := New()
	q.Set(songs, 2)
	q.SetShuffle(true)
	if q.Current() != "c" || q.Index() != 0 {
		t.Fatalf("shuffle should keep c playing first, got %q at %d", q.Current(), q.Index())
	}
	got := q.Tracks()
	slices.Sort(got)
	if !slices.Equal(got, songs) {
		t.Fatalf("shuffled queue lost songs: %v", got)
	}
	q.SetShuffle(false)
	if q.Current() != "c" || !slices.Equal(q.Tracks(), songs) {
		t.Fatalf("unshuffle should restore order and keep c, got %v at %q", q.Tracks(), q.Current())
	}
}

// TestPlayNextSurvivesUnshuffle comprueba que una canción encolada con PlayNext
// siga en su sitio tras activar y quitar el aleatorio.
func TestPlayNextSurvivesUnshuffle(t *testing.T) {
	q := New()
	q.Set(songs, 0)
	q.PlayNext("x")
	if !slices.Equal(q.Tracks(), []string{"a", "x", "b", "c", "d"}) {
		t.Fatalf("PlayNext order: %v", q.Tracks())
	}
	q.SetShuffle(true)
	q.SetShuffle(false)
	if !slices.Equal(q.Tracks(), []string{"a", "x", "b", "c", "d"}) {
		t.Fatalf("after shuffle on/off: %v", q.Tracks())
	}
}

// Quitar y mover canciones mantiene la actual y los ids (los de MPD): quitar
// una de antes corre la posición, quitar la actual pasa a la siguiente, y
// mover deja la actual sonando aunque cambie de sitio.
func TestRemoveMove(t *testing.T) {
	q := New()
	q.Set([]string{"a", "b", "c", "d"}, 2) // suena c
	ids := q.IDs()
	if q.Remove(0) {
		t.Error("removing a reported the current one")
	}
	if q.Current() != "c" || q.Index() != 1 {
		t.Fatalf("current %q at %d, want c at 1", q.Current(), q.Index())
	}
	if p, ok := q.Track(ids[2]); !ok || p != "c" {
		t.Errorf("id of c now %q %v", p, ok)
	}
	q.Move(1, 2) // c al final: b d c
	if q.Current() != "c" || q.Index() != 2 || q.Tracks()[1] != "d" {
		t.Fatalf("after move: %v, current %q at %d", q.Tracks(), q.Current(), q.Index())
	}
	if !q.Remove(2) || q.Current() != "d" {
		t.Errorf("removing the last (current) one: current %q", q.Current())
	}
	q.Remove(0)
	q.Remove(0)
	if q.Current() != "" || q.Index() != -1 {
		t.Errorf("empty queue: %q %d", q.Current(), q.Index())
	}
}

// TestPeekLinearAndShuffle verifica que Peek prediga exactamente la siguiente pista
// tanto en orden lineal como en modo aleatorio (shuffle).
func TestPeekLinearAndShuffle(t *testing.T) {
	q := New()
	q.Set(songs, 0) // songs: "a", "b", "c", "d"

	// 1. Lineal: la siguiente a "a" debe ser "b"
	if peek := q.Peek(); peek != "b" {
		t.Errorf("linear peek = %q, want 'b'", peek)
	}

	// 2. Aleatorio: Peek debe devolver la pista que está en la posición cur+1 del orden desordenado
	q.SetShuffle(true)
	nextSong := q.Tracks()[1]
	if peek := q.Peek(); peek != nextSong {
		t.Errorf("shuffled peek = %q, want %q", peek, nextSong)
	}
	// Al avanzar, la canción actual debe coincidir con la que predijo Peek
	q.Next(false)
	if q.Current() != nextSong {
		t.Errorf("after Next in shuffle: current = %q, want %q", q.Current(), nextSong)
	}
}

// TestAppendWhileShuffledPreservesCurrentAndShufflesTail verifica que añadir pistas
// en modo aleatorio mantenga intacta la pista actual y lo ya reproducido, mezcle
// las nuevas con la cola restante y conserve el orden lineal al quitar shuffle.
func TestAppendWhileShuffledPreservesCurrentAndShufflesTail(t *testing.T) {
	q := New()
	initial := []string{"a", "b", "c", "d", "e"}
	q.Set(initial, 2)
	q.SetShuffle(true)

	// Avanzamos una canción para tener un prefijo ya reproducido [0..cur].
	if !q.Next(false) {
		t.Fatal("expected Next to succeed")
	}
	cur := q.Current()
	played := slices.Clone(q.Tracks()[:q.Index()+1])

	var extra []string
	for i := 0; i < 40; i++ {
		extra = append(extra, string(rune('f'+i)))
	}
	q.Append(extra)

	if q.Current() != cur {
		t.Fatalf("Append changed current track: got %q, want %q", q.Current(), cur)
	}
	if gotPrefix := q.Tracks()[:q.Index()+1]; !slices.Equal(gotPrefix, played) {
		t.Fatalf("Append altered played prefix: got %v, want %v", gotPrefix, played)
	}

	// El tramo restante tras cur no debe dejar todas las pistas nuevas al final en orden lineal.
	tail := q.Tracks()[q.Index()+1:]
	allNewAtEnd := len(tail) >= len(extra) && slices.Equal(tail[len(tail)-len(extra):], extra)
	if allNewAtEnd {
		t.Fatalf("appended tracks stayed linearly at the tail while shuffle is on: %v", tail[len(tail)-len(extra):])
	}

	// Al quitar shuffle, debe restaurarse el orden lineal completo [a..e, extra...].
	q.SetShuffle(false)
	wantLinear := append(append([]string(nil), initial...), extra...)
	if !slices.Equal(q.Tracks(), wantLinear) {
		t.Fatalf("unshuffle after Append = %v, want %v", q.Tracks(), wantLinear)
	}
}
