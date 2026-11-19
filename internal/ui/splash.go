package ui

import (
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/juanvillacortac/audiofilus/internal/sfx"
)

// El splash: el logo (logo.go) entra —el CD aparece con un rebote y las
// letras cromadas caen una a una—, luego las letras ondulan, un brillo las
// cruza, un ecualizador de LEDs late debajo y aparece la firma (el logo de
// GitHub y juanvillacortac). Al final se funde con el fondo por escalones.
// Mientras tanto la biblioteca se carga en segundo plano. Cualquier tecla o
// botón lo salta.

const (
	splashLen   = 2.8  // segundos que dura
	splashOut   = 0.32 // lo que dura el fundido final
	splashSkip  = 0.35 // a partir de cuándo se puede saltar
	splashWord  = "audiofilus"
	splashScale = 2 // el logo va al doble: es lo único con píxeles gordos
	splashBy    = "juanvillacortac"
)

func (a *App) splashWord() string {
	if a.cfg.UI.SplashWord != "" {
		return a.cfg.UI.SplashWord
	}
	return splashWord
}

func (a *App) splashBy() string {
	if a.cfg.UI.SplashBy != "" {
		return a.cfg.UI.SplashBy
	}
	return splashBy
}

func (a *App) splashTheme() Theme {
	th := DefaultTheme
	switch a.cfg.UI.SplashColor {
	case 1: // Now Playing tone
		if a.playing != nil {
			if c1, c2, ok := a.playing.Accent(); ok {
				th.Accent, th.Accent2 = c1, c2
				return th
			}
		}
		if a.theme.Accent != (color.RGBA{}) {
			th.Accent = a.theme.Accent
			th.Accent2 = a.theme.Accent2
		}
	case 2: // Current Theme
		th = a.theme
	case 3: // Cyberpunk (Cyan & Pink)
		th.Accent = color.RGBA{0x00, 0xf0, 0xff, 0xff}
		th.Accent2 = color.RGBA{0xff, 0x00, 0x7f, 0xff}
	case 4: // Matrix / Green
		th.Accent = color.RGBA{0x00, 0xff, 0x66, 0xff}
		th.Accent2 = color.RGBA{0x88, 0xff, 0x00, 0xff}
	case 5: // Retro Amber
		th.Accent = color.RGBA{0xff, 0xb0, 0x00, 0xff}
		th.Accent2 = color.RGBA{0xff, 0x66, 0x00, 0xff}
	case 6: // Silver Chrome
		th.Accent = color.RGBA{0xe0, 0xe0, 0xe0, 0xff}
		th.Accent2 = color.RGBA{0xa0, 0xa0, 0xa0, 0xff}
	}
	return th
}

// loadSplashImage intenta cargar una imagen de splash externa si existe.
func (a *App) loadSplashImage() {
	p := a.cfg.UI.SplashImage
	if p == "" {
		candidates := []string{
			filepath.Join(a.paths.Data, "splash.png"),
			filepath.Join(a.paths.Base, "splash.png"),
			"splash.png",
			filepath.Join(a.paths.Data, "splash.jpg"),
			filepath.Join(a.paths.Base, "splash.jpg"),
		}
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				p = cand
				break
			}
		}
	}
	if p == "" {
		a.splashCustomImg = nil
		return
	}
	f, err := os.Open(p)
	if err != nil {
		a.splashCustomImg = nil
		return
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		a.splashCustomImg = nil
		return
	}
	a.splashCustomImg = ebiten.NewImageFromImage(img)
}

// updateSplash avanza el splash; devuelve false cuando terminó.
func (a *App) updateSplash() bool {
	if a.splashT == 0 {
		a.playSound(sfx.Splash) // el jingle, a tiempo con el logo
	}
	a.splashT += 1.0 / 60
	if a.splashT > splashSkip && a.input.any && a.splashT < splashLen-splashOut {
		a.splashT = splashLen - splashOut // se salta: solo queda el fundido
		a.stopSplashSound()               // y el jingle se corta
	}
	if a.splashT >= splashLen {
		a.splashing = false
	}
	return a.splashing
}

// easeOutBounce es la caída con rebote (de 0 a 1).
func easeOutBounce(x float64) float64 {
	const n, d = 7.5625, 2.75
	switch {
	case x < 1/d:
		return n * x * x
	case x < 2/d:
		x -= 1.5 / d
		return n*x*x + 0.75
	case x < 2.5/d:
		x -= 2.25 / d
		return n*x*x + 0.9375
	default:
		x -= 2.625 / d
		return n*x*x + 0.984375
	}
}

// steps redondea t (0..1) a cuatro escalones: los fundidos del splash son
// por escalones, como el resto de la interfaz.
func steps(t float64) float32 {
	return float32(math.Floor(math.Max(0, math.Min(1, t))*4) / 4)
}

// drawSplash pinta el splash en el instante a.splashT.
func (a *App) drawSplash(dst *ebiten.Image) { a.drawSplashLayers(dst, true, true) }

// drawSplashLayers pinta el fondo (bg), lo de encima (fg: logo, ecualizador y
// firma, sobre transparente) o los dos. Por separado sirven a la web, que
// estira el fondo a cualquier pantalla y centra lo de encima sin recortarlo.
func (a *App) drawSplashLayers(dst *ebiten.Image, bg, fg bool) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	t := a.splashT
	th := a.splashTheme()
	if bg && !a.ExternalBackdrop {
		drawBackground(dst, th, t)
	}
	if !fg {
		return
	}

	// Si hay una imagen externa configurada para el splash
	if a.splashCustomImg != nil {
		iw, ih := a.splashCustomImg.Bounds().Dx(), a.splashCustomImg.Bounds().Dy()
		if iw > 0 && ih > 0 {
			op := &ebiten.DrawImageOptions{}
			sx := float64(w) / float64(iw)
			sy := float64(h) / float64(ih)
			sc := math.Min(sx, sy)
			dw := float64(iw) * sc
			dh := float64(ih) * sc
			op.GeoM.Scale(sc, sc)
			op.GeoM.Translate((float64(w)-dw)/2, (float64(h)-dh)/2)

			alpha := steps((t - 0.1) / 0.3)
			op.ColorScale.ScaleAlpha(alpha)
			dst.DrawImage(a.splashCustomImg, op)

			if out := (t - (splashLen - splashOut)) / splashOut; out > 0 && bg {
				bgCol := th.Bg
				k := steps(out)
				bgCol.R, bgCol.G, bgCol.B, bgCol.A = uint8(float32(bgCol.R)*k), uint8(float32(bgCol.G)*k), uint8(float32(bgCol.B)*k), uint8(255*k)
				rect(dst, 0, 0, w, h, bgCol)
			}
			return
		}
	}

	word := a.splashWord()
	lw, lh := splashLogoSizeFor(word)
	scale := splashScale
	if lw*scale > w-8 { // en vertical no cabe al doble
		scale = 1
	}
	lx := (w - lw*scale) / 2
	ly := h/2 - 62
	if scale == 1 {
		ly = h/2 - 40
	}
	drawLogoWordThemeAt(dst, word, lx, ly, t, scale, th)
	drawLEDsTheme(dst, w/2, ly+lh*scale+2, t, math.Min(1, math.Max(0, (t-0.7)/0.4)), th)

	// La firma: si es personalizada no lleva icono de GitHub; si es por defecto sí.
	by := a.splashBy()
	if alpha := steps((t - 0.9) / 0.4); alpha > 0 {
		if a.cfg.UI.SplashBy != "" {
			// Firma propia: centrada sin logo de GitHub
			tw := textWidth(by, SmallFace)
			c := th.Dim
			c.R, c.G, c.B, c.A = uint8(float32(c.R)*alpha), uint8(float32(c.G)*alpha), uint8(float32(c.B)*alpha), uint8(255*alpha)
			drawText(dst, by, SmallFace, (w-tw)/2, h-36+5, c)
		} else {
			// Firma original: con logo de GitHub
			drawSignatureWith(dst, by, (w-signatureWidthFor(by))/2, h-36, alpha, th)
		}
	}

	// Salida: el fondo tapa todo por escalones.
	if out := (t - (splashLen - splashOut)) / splashOut; out > 0 && bg {
		bgCol := th.Bg
		k := steps(out)
		bgCol.R, bgCol.G, bgCol.B, bgCol.A = uint8(float32(bgCol.R)*k), uint8(float32(bgCol.G)*k), uint8(float32(bgCol.B)*k), uint8(255*k)
		rect(dst, 0, 0, w, h, bgCol)
	}
}

// drawLEDs pinta el ecualizador de LEDs centrado en cx, con el tema por defecto.
func drawLEDs(dst *ebiten.Image, cx, y int, t, grow float64) {
	drawLEDsTheme(dst, cx, y, t, grow, DefaultTheme)
}

// drawLEDsTheme pinta el ecualizador de LEDs centrado en cx, con el borde de
// arriba en y; grow (0..1) es cuánto ha crecido, usando el tema th.
func drawLEDsTheme(dst *ebiten.Image, cx, y int, t, grow float64, th Theme) {
	const bars, bw, gap, blocks = 17, 4, 2, 6
	ex := cx - (bars*(bw+gap)-gap)/2
	for i := 0; i < bars; i++ {
		f := float64(i) / bars
		v := 0.5 + 0.5*math.Sin(t*5.3+f*7)
		v *= 0.55 + 0.45*(0.5+0.5*math.Sin(t*2.9+f*17))
		lit := int(v*blocks*grow + 0.5)
		for b := 0; b < blocks; b++ {
			c := mix(th.Accent, th.Accent2, float64(b)/blocks)
			if b >= lit {
				c = th.Bg2
			}
			rect(dst, ex+i*(bw+gap), y+(blocks-1-b)*3, bw, 2, c)
		}
	}
}

// drawSignatureWith pinta el logo y el texto de firma.
func drawSignatureWith(dst *ebiten.Image, by string, x, y int, alpha float32, th Theme) {
	iop := &ebiten.DrawImageOptions{}
	iop.GeoM.Translate(float64(x), float64(y))
	iop.ColorScale.ScaleWithColor(th.Text)
	iop.ColorScale.ScaleAlpha(alpha)
	dst.DrawImage(icon("github"), iop)
	c := th.Dim
	c.R, c.G, c.B, c.A = uint8(float32(c.R)*alpha), uint8(float32(c.G)*alpha), uint8(float32(c.B)*alpha), uint8(255*alpha)
	drawText(dst, by, SmallFace, x+20, y+5, c)
}

// signatureWidthFor es el ancho de drawSignatureWith.
func signatureWidthFor(by string) int {
	return 20 + textWidth(by, SmallFace)
}

// drawSignature pinta el logo de GitHub y juanvillacortac a partir de (x, y),
// con opacidad alpha.
func drawSignature(dst *ebiten.Image, x, y int, alpha float32) {
	drawSignatureWith(dst, splashBy, x, y, alpha, DefaultTheme)
}

// signatureWidth es el ancho de drawSignature.
func signatureWidth() int { return signatureWidthFor(splashBy) }
