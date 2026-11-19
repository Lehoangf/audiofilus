package engine

import "math"

// El modo karaoke atenúa la voz principal situada en el centro del panorama
// estéreo mediante un procesador de tres bandas sobre el canal central (Mid):
// preserva los graves (< 160 Hz) con un filtro paso bajo Butterworth de 2º orden,
// preserva los agudos (> 4.5 kHz) con un filtro paso alto Butterworth de 2º orden,
// y atenúa únicamente la banda vocal intermedia (160 Hz – 4.5 kHz), aplicando
// ganancia de compensación (make-up gain) y realce estéreo lateral para evitar
// caídas perceptibles de volumen.

// karaokeLevels define la intensidad de atenuación de la voz (0 a 1).
var karaokeLevels = map[string]float32{
	"light": 0.60, // atenuación moderada para acompañamiento
	"full":  1.00, // cancelación máxima de la voz central
}

// KaraokeLevels son los niveles en el orden en que se ofrecen ("" apagado).
var KaraokeLevels = []string{"", "light", "full"}

// Frecuencias de corte para aislar la banda vocal sin tocar graves ni agudos.
const (
	karaokeBassCutoffHz   = 160.0  // paso bajo para bombo y bajo
	karaokeTrebleCutoffHz = 4500.0 // paso alto para platillos, charles y brillo
)

// karaoke es el filtro en marcha.
type karaoke struct {
	on        bool
	level     string
	alpha     float32 // factor de atenuación vocal
	rateAt    int
	gain      float32 // ganancia de compensación de volumen (make-up gain)
	sideBoost float32 // realce de amplitud para el canal lateral (Side)

	// Filtro paso bajo Butterworth (2º orden) para graves en Mid
	bb0, bb1, bb2, ba1, ba2 float32
	bx1, bx2, by1, by2      float32

	// Filtro paso alto Butterworth (2º orden) para agudos/brillo en Mid
	tb0, tb1, tb2, ta1, ta2 float32
	tx1, tx2, ty1, ty2      float32
}

// set ajusta el filtro al nivel y la frecuencia de muestreo de salida.
func (k *karaoke) set(level string, rate int) {
	alpha, ok := karaokeLevels[level]
	k.on, k.level, k.rateAt = ok, level, rate
	k.bx1, k.bx2, k.by1, k.by2 = 0, 0, 0, 0
	k.tx1, k.tx2, k.ty1, k.ty2 = 0, 0, 0, 0
	if !ok {
		k.alpha = 0
		k.gain = 1
		k.sideBoost = 1
		return
	}
	k.alpha = alpha
	// Compensación de volumen para que la canción no suene apagada al quitar la voz
	k.gain = 1.0 + 0.30*alpha
	k.sideBoost = 1.0 + 0.10*alpha
	if rate <= 0 {
		rate = 48000
	}

	q := math.Sqrt2 / 2.0

	// 1. Filtro paso bajo Butterworth 2º orden (Graves Mid)
	w0b := 2 * math.Pi * karaokeBassCutoffHz / float64(rate)
	cwb, swb := math.Cos(w0b), math.Sin(w0b)
	alphab := swb / (2 * q)
	bb0 := (1 - cwb) / 2
	bb1 := 1 - cwb
	bb2 := (1 - cwb) / 2
	ba0 := 1 + alphab
	ba1 := -2 * cwb
	ba2 := 1 - alphab

	k.bb0 = float32(bb0 / ba0)
	k.bb1 = float32(bb1 / ba0)
	k.bb2 = float32(bb2 / ba0)
	k.ba1 = float32(ba1 / ba0)
	k.ba2 = float32(ba2 / ba0)

	// 2. Filtro paso alto Butterworth 2º orden (Agudos Mid)
	w0t := 2 * math.Pi * karaokeTrebleCutoffHz / float64(rate)
	cwt, swt := math.Cos(w0t), math.Sin(w0t)
	alphat := swt / (2 * q)
	tb0 := (1 + cwt) / 2
	tb1 := -(1 + cwt)
	tb2 := (1 + cwt) / 2
	ta0 := 1 + alphat
	ta1 := -2 * cwt
	ta2 := 1 - alphat

	k.tb0 = float32(tb0 / ta0)
	k.tb1 = float32(tb1 / ta0)
	k.tb2 = float32(tb2 / ta0)
	k.ta1 = float32(ta1 / ta0)
	k.ta2 = float32(ta2 / ta0)
}

// process procesa el búfer estéreo intercalado (L, R) en el sitio.
func (k *karaoke) process(buf []float32) {
	if !k.on || k.alpha == 0 {
		return
	}
	alpha := k.alpha
	invAlpha := 1.0 - alpha
	gain := k.gain
	sideBoost := k.sideBoost

	bb0, bb1, bb2, ba1, ba2 := k.bb0, k.bb1, k.bb2, k.ba1, k.ba2
	bx1, bx2, by1, by2 := k.bx1, k.bx2, k.by1, k.by2

	tb0, tb1, tb2, ta1, ta2 := k.tb0, k.tb1, k.tb2, k.ta1, k.ta2
	tx1, tx2, ty1, ty2 := k.tx1, k.tx2, k.ty1, k.ty2

	for i := 0; i+1 < len(buf); i += 2 {
		l, r := buf[i], buf[i+1]

		// Descomposición Mid/Side
		mid := 0.5 * (l + r)
		side := 0.5 * (l - r)

		// 1. Aislar graves en canal central (< 160 Hz)
		bassMid := bb0*mid + bb1*bx1 + bb2*bx2 - ba1*by1 - ba2*by2
		bx2, bx1 = bx1, mid
		by2, by1 = by1, bassMid

		// 2. Aislar agudos y brillo en canal central (> 4.5 kHz)
		trebleMid := tb0*mid + tb1*tx1 + tb2*tx2 - ta1*ty1 - ta2*ty2
		tx2, tx1 = tx1, mid
		ty2, ty1 = ty1, trebleMid

		// 3. Banda vocal intermedia (160 Hz - 4.5 kHz)
		vocalMid := mid - bassMid - trebleMid

		// Reconstrucción con atenuación vocal exclusiva en la banda media
		filteredMid := bassMid + trebleMid + vocalMid*invAlpha

		// Realce estéreo lateral y ganancia de compensación general
		sideFinal := side * sideBoost
		buf[i] = (filteredMid + sideFinal) * gain
		buf[i+1] = (filteredMid - sideFinal) * gain
	}

	k.bx1, k.bx2, k.by1, k.by2 = bx1, bx2, by1, by2
	k.tx1, k.tx2, k.ty1, k.ty2 = tx1, tx2, ty1, ty2
}
