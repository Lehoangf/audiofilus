package engine

import (
	"math"
	"testing"
)

// TestKaraokeCancellation comprueba que las señales vocales mono se atenúen
// drásticamente mientras que los graves, los agudos y los instrumentos laterales se preservan.
func TestKaraokeCancellation(t *testing.T) {
	var k karaoke
	k.set("full", 48000)

	// 1. Voz en el centro (tono de 1 kHz presente igual en L y R)
	nSamples := 4800 // 100 ms
	buf := make([]float32, nSamples*2)
	for i := 0; i < nSamples; i++ {
		sample := float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
		buf[i*2] = sample   // L
		buf[i*2+1] = sample // R
	}

	k.process(buf)

	// Calcular energía residual (RMS)
	var sum float64
	for _, s := range buf[1000*2:] { // descartar transitorio inicial
		sum += float64(s * s)
	}
	rms := math.Sqrt(sum / float64(len(buf)-1000*2))
	if rms > 0.08 {
		t.Errorf("center vocal 1kHz RMS = %f, want < 0.08 (strong vocal cut)", rms)
	}

	// 2. Graves en el centro (tono de 60 Hz en L y R debe conservarse)
	k.set("full", 48000)
	bufBass := make([]float32, nSamples*2)
	for i := 0; i < nSamples; i++ {
		sample := float32(math.Sin(2 * math.Pi * 60 * float64(i) / 48000))
		bufBass[i*2] = sample
		bufBass[i*2+1] = sample
	}

	k.process(bufBass)

	var sumBass float64
	for _, s := range bufBass[1000*2:] {
		sumBass += float64(s * s)
	}
	rmsBass := math.Sqrt(sumBass / float64(len(bufBass)-1000*2))
	if rmsBass < 0.5 { // el original sin procesar tiene RMS ~0.707
		t.Errorf("center bass 60Hz RMS = %f, want >= 0.50 (bass preserved)", rmsBass)
	}

	// 3. Agudos/brillo en el centro (tono de 10 kHz en L y R para platillos/charles)
	k.set("full", 48000)
	bufTreble := make([]float32, nSamples*2)
	for i := 0; i < nSamples; i++ {
		sample := float32(math.Sin(2 * math.Pi * 10000 * float64(i) / 48000))
		bufTreble[i*2] = sample
		bufTreble[i*2+1] = sample
	}

	k.process(bufTreble)

	var sumTreble float64
	for _, s := range bufTreble[1000*2:] {
		sumTreble += float64(s * s)
	}
	rmsTreble := math.Sqrt(sumTreble / float64(len(bufTreble)-1000*2))
	if rmsTreble < 0.5 {
		t.Errorf("center treble 10kHz RMS = %f, want >= 0.50 (cymbals preserved)", rmsTreble)
	}

	// 4. Instrumento lateral (L = -R, guitarra en estéreo extremo)
	k.set("full", 48000)
	bufSide := make([]float32, nSamples*2)
	for i := 0; i < nSamples; i++ {
		sample := float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
		bufSide[i*2] = sample
		bufSide[i*2+1] = -sample
	}

	k.process(bufSide)

	var sumSide float64
	for _, s := range bufSide[1000*2:] {
		sumSide += float64(s * s)
	}
	rmsSide := math.Sqrt(sumSide / float64(len(bufSide)-1000*2))
	if rmsSide < 0.65 {
		t.Errorf("side instrument RMS = %f, want >= 0.65 (retained with make-up)", rmsSide)
	}
}

// TestKaraokeLightLevel comprueba que el modo "light" reduzca la voz manteniendo buen nivel.
func TestKaraokeLightLevel(t *testing.T) {
	var k karaoke
	k.set("light", 48000)

	nSamples := 4800
	buf := make([]float32, nSamples*2)
	for i := 0; i < nSamples; i++ {
		sample := float32(math.Sin(2 * math.Pi * 1000 * float64(i) / 48000))
		buf[i*2] = sample
		buf[i*2+1] = sample
	}

	k.process(buf)

	var sum float64
	for _, s := range buf[1000*2:] {
		sum += float64(s * s)
	}
	rms := math.Sqrt(sum / float64(len(buf)-1000*2))
	if rms < 0.20 || rms > 0.40 {
		t.Errorf("light vocal 1kHz RMS = %f, want ~0.30", rms)
	}
}

// BenchmarkKaraoke verifica que el procesamiento no reserve memoria en el heap.
func BenchmarkKaraoke(b *testing.B) {
	var k karaoke
	k.set("full", 48000)
	buf := make([]float32, 1024)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		k.process(buf)
	}
}
