package ui

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/juanvillacortac/audiofilus/internal/ui/assets"
)

// Las dos fuentes, como en Trancazo: Jersey 10 para títulos y Tiny5 para el
// resto. Las dos son pixel art y se usan a su tamaño nativo, para que cada
// píxel del diseño de la letra sea uno del lienzo. No tienen japonés ni chino:
// lo que les falta sale de Fusion Pixel 8px (también pixel art, OFL), a 8 px
// en el texto y al doble en los títulos (a 12 px queda fina al lado de
// Jersey). Hay dos variantes, porque el japonés y el chino dibujan distinto
// los mismos caracteres: va primero la del idioma de la interfaz (la china en
// chino, la japonesa en los demás) y la otra detrás. Así los kanji de un
// título salen en cualquier idioma.
var (
	// BigFace es Jersey 10: rejilla de 75 unidades sobre 1400, así que a
	// 1400/75 px cada píxel de la letra cae en uno del lienzo. Mayúsculas de
	// 10 px, empiezan 5 px bajo la parte de arriba de la línea, y la línea mide 20.
	BigFace text.Face
	// SmallFace es Tiny5 a 8 px: mayúsculas de 5 px, empiezan 2 px bajo la parte
	// de arriba, y la línea mide 9.
	SmallFace text.Face

	// Las fuentes sueltas, para armar BigFace y SmallFace según el idioma
	// (setFontOrder): las de base y las de Fusion Pixel ([0] japonesa, [1]
	// china), si cargaron.
	bigBase, smallBase text.Face
	cjkBig, cjkSmall   [2]text.Face

	currentFont = "default"
	bigCapTop   = 5
	bigCap      = 10
	smallCapTop = 2
	smallCap    = 5
)

// thaiFont es Silver solo con el tailandés (10 KB: se carga con las de base).
const thaiFont = "fonts/Silver-thai.ttf"

// thaiPx es el tamaño de Silver: su rejilla es de 1900/100 = 19 px.
const thaiPx = 19

// multiFace junta faces (la primera que tenga cada letra); si no se puede,
// se queda con la primera.
func multiFace(faces []text.Face) text.Face {
	if len(faces) == 1 {
		return faces[0]
	}
	m, err := text.NewMultiFace(faces...)
	if err != nil {
		return faces[0]
	}
	return m
}

var (
	cjkFonts    = [2]string{"fonts/FusionPixel8-ja.ttf.gz", "fonts/FusionPixel8-zh_hans.ttf.gz"}
	lastBaseDir string
)

// CurrentFont devuelve el identificador de la fuente actualmente seleccionada.
func CurrentFont() string {
	return currentFont
}

// AvailableFonts devuelve los nombres de fuentes disponibles (default, coderscrux, retron2000, kvn97, dearpix y los archivos en fonts/).
func AvailableFonts(baseDir string) []string {
	if baseDir != "" {
		lastBaseDir = baseDir
	} else if lastBaseDir != "" {
		baseDir = lastBaseDir
	}
	fonts := []string{"default", "coderscrux", "retron2000", "kvn97", "dearpix"}
	seen := map[string]bool{
		"default":             true,
		"coderscrux":          true,
		"coders_crux":         true,
		"svn-coders-crux":     true,
		"svn-coders-crux.ttf": true,
		"retron2000":          true,
		"retron_2000":         true,
		"svn-retron-2000":     true,
		"svn-retron-2000.ttf": true,
		"kvn97":               true,
		"kvn-97":              true,
		"kvn-97.ttf":          true,
		"dearpix":             true,
		"dearpix-2-01":        true,
		"dearpix-2-01.otf":    true,
	}
	if baseDir == "" {
		return fonts
	}
	fontDir := filepath.Join(baseDir, "fonts")
	entries, err := os.ReadDir(fontDir)
	if err != nil {
		return fonts
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".ttf" || ext == ".otf" {
			base := strings.ToLower(e.Name())
			stem := strings.TrimSuffix(base, ext)
			if !seen[base] && !seen[stem] {
				fonts = append(fonts, e.Name())
				seen[base] = true
				seen[stem] = true
			}
		}
	}
	return fonts
}

func withVietnameseFallback(main text.Face, px float64, like font.Face) text.Face {
	if crux, err := loadFace("fonts/SVN-Coders-Crux.ttf", px, like); err == nil {
		if m, err := text.NewMultiFace(main, text.NewGoXFace(crux)); err == nil {
			return m
		}
	}
	return main
}

// defaultFontNames devuelve las rutas de las fuentes para títulos (grande) y texto/letras
// (pequeña), usando las versiones con soporte nativo de vietnamita (-VN) si están presentes.
func defaultFontNames() (string, string) {
	bigName := "fonts/Jersey10-Regular-VN.ttf"
	if _, err := assets.FS.ReadFile(bigName); err != nil {
		bigName = "fonts/Jersey10-Regular.ttf"
	}
	smallName := "fonts/Tiny5-Regular-VN.ttf"
	if _, err := assets.FS.ReadFile(smallName); err != nil {
		smallName = "fonts/Tiny5-Regular.ttf"
	}
	return bigName, smallName
}

// SetFont cambia la fuente de la interfaz a la indicada ("default", "coderscrux", "retron2000", "kvn97", "dearpix" o un archivo en fonts/).
func SetFont(name, baseDir string) error {
	norm := strings.ToLower(name)
	switch {
	case norm == "" || norm == "default":
		bigFont, smallFont := defaultFontNames()
		big, err := loadFace(bigFont, 1400.0/75, nil)
		if err != nil {
			return err
		}
		small, err := loadFace(smallFont, 8, nil)
		if err != nil {
			return err
		}
		tinyBig, err := loadFace(smallFont, 16, big)
		if err != nil {
			return err
		}
		// Las letras con marcas que les falten (el vietnamita), armadas con sus
		// partes (compose.go).
		bigFaces := []text.Face{text.NewGoXFace(newComposeFace(big, 2)), text.NewGoXFace(newComposeFace(tinyBig, 2))}
		smallFaces := []text.Face{text.NewGoXFace(newComposeFace(small, 1))}
		// El tailandés, con las medidas de cada una (ver newFace): si no carga,
		// sigue sin él.
		if f, err := parseFont(thaiFont); err == nil {
			tb, err1 := newFace(f, thaiFont, thaiPx, big)
			ts, err2 := newFace(f, thaiFont, thaiPx, small)
			if err1 == nil && err2 == nil {
				bigFaces = append(bigFaces, text.NewGoXFace(tb))
				smallFaces = append(smallFaces, text.NewGoXFace(ts))
			}
		}
		bigMain := multiFace(bigFaces)
		bigBase = withVietnameseFallback(bigMain, 20, big)
		smallBase = withVietnameseFallback(multiFace(smallFaces), 12, small)
		bigCapTop, bigCap = 5, 10
		smallCapTop, smallCap = 2, 5
		currentFont = "default"
		setFontOrder()
		return nil

	case norm == "coderscrux" || norm == "coders_crux" || norm == "svn-coders-crux.ttf" || norm == "svn-coders-crux":
		f, err := parseFont("fonts/SVN-Coders-Crux.ttf")
		if err != nil {
			return err
		}
		big, err := newFace(f, "coderscrux", 20, nil)
		if err != nil {
			return err
		}
		small, err := newFace(f, "coderscrux", 14, nil)
		if err != nil {
			return err
		}
		bigBase, smallBase = text.NewGoXFace(big), text.NewGoXFace(small)
		bigCapTop, bigCap = 0, 12
		smallCapTop, smallCap = 0, 8
		currentFont = "coderscrux"
		setFontOrder()
		return nil

	case norm == "retron2000" || norm == "retron_2000" || norm == "svn-retron-2000.ttf" || norm == "svn-retron-2000":
		f, err := parseFont("fonts/SVN-Retron-2000.ttf")
		if err != nil {
			return err
		}
		big, err := newFace(f, "retron2000", 14, nil)
		if err != nil {
			return err
		}
		small, err := newFace(f, "retron2000", 10, nil)
		if err != nil {
			return err
		}
		bigBase, smallBase = text.NewGoXFace(big), text.NewGoXFace(small)
		bigCapTop, bigCap = 9, 11
		smallCapTop, smallCap = 6, 8
		currentFont = "retron2000"
		setFontOrder()
		return nil

	case norm == "kvn97" || norm == "kvn-97" || norm == "kvn-97.ttf":
		f, err := parseFont("fonts/kvn-97.ttf")
		if err != nil {
			return err
		}
		big, err := newFace(f, "kvn97", 14, nil)
		if err != nil {
			return err
		}
		small, err := newFace(f, "kvn97", 10, nil)
		if err != nil {
			return err
		}
		bigBase = withVietnameseFallback(text.NewGoXFace(big), 20, big)
		smallBase = withVietnameseFallback(text.NewGoXFace(small), 12, small)
		bigCapTop, bigCap = 1, 13
		smallCapTop, smallCap = 1, 9
		currentFont = "kvn97"
		setFontOrder()
		return nil

	case norm == "dearpix" || norm == "dearpix-2-01" || norm == "dearpix-2-01.otf":
		f, err := parseFont("fonts/dearpix-2-01.otf")
		if err != nil {
			return err
		}
		big, err := newFace(f, "dearpix", 16, nil)
		if err != nil {
			return err
		}
		small, err := newFace(f, "dearpix", 10, nil)
		if err != nil {
			return err
		}
		bigBase = withVietnameseFallback(text.NewGoXFace(big), 20, big)
		smallBase = withVietnameseFallback(text.NewGoXFace(small), 12, small)
		bigCapTop, bigCap = 4, 10
		smallCapTop, smallCap = 2, 6
		currentFont = "dearpix"
		setFontOrder()
		return nil

	default:
		path := name
		if !filepath.IsAbs(path) && baseDir != "" {
			path = filepath.Join(baseDir, "fonts", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := opentype.Parse(data)
		if err != nil {
			return err
		}
		big, err := newFace(f, name, 16, nil)
		if err != nil {
			return err
		}
		small, err := newFace(f, name, 10, nil)
		if err != nil {
			return err
		}
		bigBase = withVietnameseFallback(text.NewGoXFace(big), 20, big)
		smallBase = withVietnameseFallback(text.NewGoXFace(small), 12, small)
		bigCapTop, bigCap = 4, 10
		smallCapTop, smallCap = 2, 6
		currentFont = name
		setFontOrder()
		return nil
	}
}

// cjkLoaded trae las fuentes de Fusion Pixel cuando terminan de cargar en
// segundo plano (descomprimir y leer 7 MB cuesta en las portátiles): mientras
// tanto, durante el splash, se usan las de base solas.
var cjkLoaded chan [2][2]text.Face

// LoadFonts carga las fuentes de base y empieza a cargar las de Fusion Pixel
// en segundo plano (llegan con pollFonts). Si Fusion Pixel no carga, sigue sin
// ella (el japonés y el chino no se ven).
func LoadFonts() error {
	if err := SetFont("default", ""); err != nil {
		return err
	}
	bigFont, smallFont := defaultFontNames()
	big, _ := loadFace(bigFont, 1400.0/75, nil)
	small, _ := loadFace(smallFont, 8, nil)
	cjkBig, cjkSmall = [2]text.Face{}, [2]text.Face{}
	cjkLoaded = make(chan [2][2]text.Face, 1)
	go func(ch chan [2][2]text.Face) {
		var out [2][2]text.Face // [variante][grande, pequeña]
		for i, name := range cjkFonts {
			f, err := parseFont(name)
			if err != nil {
				continue
			}
			b, err1 := newFace(f, name, 16, big)
			s, err2 := newFace(f, name, 8, small)
			if err1 == nil && err2 == nil {
				out[i] = [2]text.Face{text.NewGoXFace(b), text.NewGoXFace(s)}
			}
		}
		ch <- out
		ebiten.ScheduleFrame()
	}(cjkLoaded)
	return nil
}

// pollFonts pone las de Fusion Pixel en cuanto llegan (desde el hilo de la
// interfaz, que es el que dibuja con ellas). Con wait, las espera.
func pollFonts(wait bool) {
	if cjkLoaded == nil {
		return
	}
	var out [2][2]text.Face
	if wait {
		out = <-cjkLoaded
	} else {
		select {
		case out = <-cjkLoaded:
		default:
			return
		}
	}
	cjkLoaded = nil
	for i := range out {
		cjkBig[i], cjkSmall[i] = out[i][0], out[i][1]
	}
	setFontOrder()
}

// setFontOrder arma BigFace y SmallFace para el idioma en uso: la base y
// detrás las de Fusion Pixel, primero la del idioma.
func setFontOrder() {
	if smallBase == nil {
		return // aún no se cargaron (SetLanguage va antes que LoadFonts)
	}
	order := [2]int{0, 1}
	if lang == "zh" || lang == "zh-TW" {
		order = [2]int{1, 0}
	}
	build := func(base text.Face, cjk [2]text.Face) text.Face {
		faces := []text.Face{base}
		for _, i := range order {
			if cjk[i] != nil {
				faces = append(faces, cjk[i])
			}
		}
		if len(faces) == 1 {
			return base
		}
		m, err := text.NewMultiFace(faces...)
		if err != nil {
			return base
		}
		return m
	}
	BigFace, SmallFace = build(bigBase, cjkBig), build(smallBase, cjkSmall)
}

// parseFont lee una fuente de los assets (descomprimiéndola si es .gz).
func parseFont(name string) (*opentype.Font, error) {
	data, err := assets.FS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if data, err = io.ReadAll(zr); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return f, nil
}

// loadFace carga una fuente a px píxeles.
func loadFace(name string, px float64, like font.Face) (font.Face, error) {
	f, err := parseFont(name)
	if err != nil {
		return nil, err
	}
	return newFace(f, name, px, like)
}

// newFace es f a px píxeles, nítida. Con like, dice tener sus medidas: las
// de Fusion Pixel dicen tener las de Tiny5 o Jersey, porque la mezcla
// (MultiFace) toma la más alta de todas y el texto latino bajaría.
func newFace(f *opentype.Font, name string, px float64, like font.Face) (font.Face, error) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if like != nil {
		return crispFace{metricsFace{face, like.Metrics()}}, nil
	}
	return crispFace{face}, nil
}

// metricsFace es una fuente con las medidas de otra.
type metricsFace struct {
	font.Face
	m font.Metrics
}

func (f metricsFace) Metrics() font.Metrics { return f.m }

// crispFace deja cada píxel de cada letra encendido o apagado: el rasterizador
// suaviza los bordes aunque la rejilla caiga justa, y a 240 px de alto eso se
// ve borroso. Es la misma idea que en Trancazo.
type crispFace struct{ font.Face }

func (c crispFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	dot.X, dot.Y = fixed.I(dot.X.Round()), fixed.I(dot.Y.Round())
	dr, mask, mp, adv, ok := c.Face.Glyph(dot, r)
	if !ok {
		return dr, mask, mp, adv, ok
	}
	out := image.NewAlpha(image.Rect(0, 0, dr.Dx(), dr.Dy()))
	for y := range dr.Dy() {
		for x := range dr.Dx() {
			if _, _, _, a := mask.At(mp.X+x, mp.Y+y).RGBA(); a >= 0x8000 {
				out.Pix[y*out.Stride+x] = 0xff
			}
		}
	}
	return dr, out, image.Point{}, adv, true
}

// drawText escribe s con la parte de arriba de sus mayúsculas en y.
func drawText(dst *ebiten.Image, s string, face text.Face, x, y int, c color.Color) {
	top := smallCapTop
	if face == BigFace {
		top = bigCapTop
	}
	if needsShaping(s) { // hindi o árabe (complex.go)
		l := shapeLine(s, face)
		img := l.image(face)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x), float64(y-top-l.pad))
		op.ColorScale.ScaleWithColor(c)
		dst.DrawImage(img, op)
		return
	}
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y-top))
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, face, op)
}

// drawTextShadow escribe s con una sombra de un píxel abajo a la derecha.
func drawTextShadow(dst *ebiten.Image, s string, face text.Face, x, y int, c, shadow color.Color) {
	drawText(dst, s, face, x+1, y+1, shadow)
	drawText(dst, s, face, x, y, c)
}

// textWidth es el ancho de s en píxeles.
func textWidth(s string, face text.Face) int {
	if needsShaping(s) {
		return shapeLine(s, face).w
	}
	return int(text.Advance(s, face) + 0.5)
}

// fadeW es el ancho del fundido de los textos que no caben (ocho escalones
// de 3 píxeles).
const fadeW = 24

// scratch es un lienzo de paso para pintar un texto y luego desvanecer sus
// bordes. Se reutiliza en cada texto: Ebitengine ejecuta los dibujos en orden.
var scratch *ebiten.Image

// drawTextW escribe s en un ancho w. Si no cabe, en vez de cortarlo con "..."
// se desvanece hacia la derecha con trama.
func drawTextW(dst *ebiten.Image, s string, face text.Face, x, y, w int, c color.Color) {
	if w <= 0 {
		return
	}
	if textWidth(s, face) <= w {
		drawText(dst, s, face, x, y, c)
		return
	}
	drawFaded(dst, s, face, x, y, w, 0, false, true, c, nil)
}

// drawFaded escribe s desplazado off píxeles a la izquierda dentro de un hueco
// de w, con los bordes que se pidan desvanecidos y, si shadow no es nil, con
// sombra.
func drawFaded(dst *ebiten.Image, s string, face text.Face, x, y, w, off int, left, right bool, c, shadow color.Color) {
	if w <= 0 {
		return
	}
	h := 24
	if scratch == nil {
		scratch = newTarget(640, h)
	}
	w = min(w, scratch.Bounds().Dx())
	top := smallCapTop
	if face == BigFace {
		top = bigCapTop
	}
	region := scratch.SubImage(image.Rect(0, 0, w, h)).(*ebiten.Image)
	region.Clear()
	// El texto se pinta con su parte de arriba en y=top+1 del lienzo de paso.
	if shadow != nil {
		drawText(region, s, face, -off+1, top+2, shadow)
	}
	drawText(region, s, face, -off, top+1, c)
	op := &ebiten.DrawRectShaderOptions{}
	op.GeoM.Translate(float64(x), float64(y-top-1))
	op.Images[0] = region
	fl, fr := float32(0), float32(0)
	if left {
		fl = fadeW
	}
	if right {
		fr = fadeW
	}
	op.Uniforms = map[string]any{"Width": float32(w), "FadeL": fl, "FadeR": fr}
	dst.DrawRectShader(w, h, fadeShader, op)
}

// fitKey identifica un recorte ya calculado.
type fitKey struct {
	s    string
	face text.Face
	w    int
}

// fitCache guarda los recortes: medir letra a letra cada título en cada frame
// daba tirones al desplazarse por las listas.
var fitCache = map[fitKey]string{}

// fit recorta s con "..." para que quepa en w píxeles.
func fit(s string, face text.Face, w int) string {
	k := fitKey{s, face, w}
	if r, ok := fitCache[k]; ok {
		return r
	}
	out := s
	if textWidth(s, face) > w {
		r := []rune(s)
		// Búsqueda binaria del largo que cabe.
		lo, hi := 0, len(r)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if textWidth(string(r[:mid])+"...", face) <= w {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		out = string(r[:lo]) + "..."
	}
	if len(fitCache) > 4096 {
		clear(fitCache)
	}
	fitCache[k] = out
	return out
}
