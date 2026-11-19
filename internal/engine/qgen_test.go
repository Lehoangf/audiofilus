package engine

import (
	"slices"
	"testing"

	"github.com/juanvillacortac/audiofilus/internal/decode"
)

// TestQueueGenSurvivesShuffleAndStopsOnNewLoad verifica que AppendIfGen añada
// pistas cuando cambia qver por shuffle o PlayNext, y aborte tras LoadGen o Clear.
func TestQueueGenSurvivesShuffleAndStopsOnNewLoad(t *testing.T) {
	e := testEngine(48000, DSP{})
	e.player = &otoLike{src: e}
	e.OpenRemote = func(path string) (decode.Source, bool, error) {
		return &liveSource{}, true, nil
	}

	var prefetched []string
	e.Prefetch = func(path string) {
		prefetched = append(prefetched, path)
	}

	g1 := e.LoadGen([]string{"spotify://track/t1"}, 0, 0, false)
	if g1 == 0 || e.QueueGen() != g1 {
		t.Fatalf("LoadGen = %d (QueueGen=%d), want non-zero equal generation", g1, e.QueueGen())
	}

	// Cambiar shuffle o insertar PlayNext sube qver, pero no cambia qgen.
	e.SetShuffle(true)
	e.PlayNext("spotify://track/next1")
	e.drainEventsForTest()
	prefetched = nil

	if !e.AppendIfGen(g1, []string{"spotify://track/t2", "spotify://track/t3"}) {
		t.Fatal("AppendIfGen should succeed even after SetShuffle and PlayNext")
	}

	// Si antes no había pista siguiente (e.g. solo 1 pista) o cambió Peek, debe prefetchar.
	e2 := testEngine(48000, DSP{})
	e2.player = &otoLike{src: e2}
	e2.OpenRemote = func(path string) (decode.Source, bool, error) {
		return &liveSource{}, true, nil
	}
	var gotPrefetch []string
	e2.Prefetch = func(path string) { gotPrefetch = append(gotPrefetch, path) }
	gSingle := e2.LoadGen([]string{"spotify://track/only"}, 0, 0, false)
	e2.drainEventsForTest()
	gotPrefetch = nil
	if !e2.AppendIfGen(gSingle, []string{"spotify://track/second"}) {
		t.Fatal("AppendIfGen on single-track queue should succeed")
	}
	e2.drainEventsForTest()
	if !slices.Equal(gotPrefetch, []string{"spotify://track/second"}) {
		t.Fatalf("AppendIfGen prefetch = %v, want [spotify://track/second]", gotPrefetch)
	}

	// Un nuevo LoadGen debe invalidar g1.
	g2 := e.LoadGen([]string{"spotify://track/other"}, 0, 0, false)
	if g2 == g1 {
		t.Fatalf("new LoadGen reused generation %d", g2)
	}
	if e.AppendIfGen(g1, []string{"spotify://track/stale"}) {
		t.Fatal("AppendIfGen must abort when generation changed via LoadGen")
	}
	if st := e.Status(); st.Count != 1 {
		t.Fatalf("Count after stale AppendIfGen = %d, want 1", st.Count)
	}

	// Clear también debe invalidar g2.
	e.Clear()
	if e.AppendIfGen(g2, []string{"spotify://track/stale2"}) {
		t.Fatal("AppendIfGen must abort after Clear")
	}
}

func (e *Engine) drainEventsForTest() {
	for {
		select {
		case fn := <-e.events:
			fn()
		default:
			return
		}
	}
}
