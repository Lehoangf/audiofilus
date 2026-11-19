package engine

import (
	"io"
	"sync"
	"time"

	"github.com/juanvillacortac/audiofilus/internal/queue"
)

// dummyPlayer implementa output sin tocar hardware de audio para pruebas y modo desatendido.
type dummyPlayer struct {
	mu  sync.Mutex
	src io.Seeker
}

func (p *dummyPlayer) Play()             {}
func (p *dummyPlayer) Pause()            {}
func (p *dummyPlayer) Volume() float64   { return 1 }
func (p *dummyPlayer) SetVolume(float64) {}
func (p *dummyPlayer) BufferedSize() int { return 0 }
func (p *dummyPlayer) Seek(o int64, w int) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.src != nil {
		return p.src.Seek(o, w)
	}
	return 0, nil
}

// NewHeadless crea un Engine sin hardware de audio para pruebas y entornos headless.
func NewHeadless(sampleRate int) *Engine {
	e := &Engine{
		rate:   sampleRate,
		q:      queue.New(),
		events: make(chan func(), 64),
		boost:  1,
		vol:    1,
		gain:   1,
		limit:  newLimiter(sampleRate),
		meter:  newMeter(sampleRate, 100*time.Millisecond),
	}
	e.dsp = DSP{}.Normalize()
	e.eq.set(e.dsp.EQ, sampleRate)
	p := &dummyPlayer{src: e}
	e.player = p
	go e.dispatch()
	return e
}
