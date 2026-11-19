package ui

import (
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/juanvillacortac/audiofilus/internal/radio"
)

// upNextAnimLen es lo que tarda en abrirse o cerrarse la vista previa de
// "Siguiente" (encogiendo la portada actual y metiendo el vinilo).
const upNextAnimLen = 0.35

// upNextEndSec son los últimos segundos de la canción en los que se muestra
// "Siguiente" cuando el modo es "end".
const upNextEndSec = 15.0

// upNextState guarda el estado de la animación de "Siguiente" en Reproduciendo.
type upNextState struct {
	p           float64
	lastT       float64
	nextPath    string
	lastVisible bool
	lastBoxX    int
	lastBoxY    int
	lastBoxSize int

	// promote anima el salto al pasar a la canción que estaba en "Siguiente":
	// su portada crece desde donde estaba hasta ocupar el sitio principal, y
	// el disco vuelve a salir por detrás.
	promote   bool
	promoteAt float64
	fromX     int
	fromY     int
	fromSize  int

	// Caché de portadas de Spotify para la canción siguiente.
	coverReq map[string]bool
	coverAlb map[string]*Album
	coverCh  chan upNextCoverMsg
}

type upNextCoverMsg struct {
	id   string
	path string
}

// upNextInfo describe la canción que sonará a continuación.
type upNextInfo struct {
	song  Song
	album *Album
}

// upNextLayout son las posiciones calculadas de la portada principal, el disco
// y la portada de "Siguiente" para un fotograma concreto.
type upNextLayout struct {
	mainX, mainY, mainSize int
	slide                  int
	nextX, nextY, nextSize int
	alpha                  float64
}

// upNextMode devuelve el modo activo de "Siguiente": "end", "always" u "off".
func (a *App) upNextMode() string {
	switch a.cfg.UI.NowPlayingUpNext {
	case "off", "always", "end":
		return a.cfg.UI.NowPlayingUpNext
	default:
		return "end"
	}
}

// cycleUpNextMode alterna entre "15s finales", "Siempre" y "Apagado".
func (a *App) cycleUpNextMode() {
	switch a.upNextMode() {
	case "end":
		a.cfg.UI.NowPlayingUpNext = "always"
	case "always":
		a.cfg.UI.NowPlayingUpNext = "off"
	default:
		a.cfg.UI.NowPlayingUpNext = "end"
	}
	a.saveConfig()
}

// upNextModeLabel devuelve la etiqueta traducida del modo actual de "Siguiente".
func (a *App) upNextModeLabel() string {
	switch a.upNextMode() {
	case "off":
		return T("off")
	case "always":
		return T("upnext_always")
	default:
		return T("upnext_end")
	}
}

// upNextTrack resuelve cuál es la siguiente canción de la cola, o false si la
// reproducción se detendrá o repetirá la misma canción.
func (a *App) upNextTrack() (upNextInfo, bool) {
	if a.playingSong.Path == "" || radio.IsRadio(a.playingSong.Path) {
		return upNextInfo{}, false
	}
	if a.status.StopAfter || a.status.Repeat == "one" {
		return upNextInfo{}, false
	}
	q := a.queue()
	if len(q) == 0 {
		return upNextInfo{}, false
	}
	cur := a.status.Index
	if a.client == nil {
		cur = a.playingIdx
	}
	nextIdx := -1
	switch {
	case cur >= 0 && cur+1 < len(q):
		nextIdx = cur + 1
	case a.status.Repeat == "all" && len(q) > 1:
		nextIdx = 0
	default:
		return upNextInfo{}, false
	}
	p := q[nextIdx]
	if p == "" {
		return upNextInfo{}, false
	}
	if al, idx := a.albumOf(p); al != nil {
		s := Song{Path: p, Title: strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))}
		if idx >= 0 && idx < len(al.Songs) {
			s = al.Songs[idx]
		}
		return upNextInfo{song: s, album: al}, true
	}
	if a.lib != nil {
		if s, ok := a.lib.Song(p); ok {
			al, _ := a.lib.AlbumOf(p)
			return upNextInfo{song: s, album: al}, true
		}
	}
	if isSpotifyPath(p) {
		spID := extractSpotifyID(p)
		s := Song{Path: p, Title: "Spotify Track"}
		var al *Album
		if tr, ok := a.spotifyTrackMetadata(spID); ok {
			s.Title, s.Artist, s.Album = tr.Title, tr.Artist, tr.Album
			al = a.spotifyNextAlbum(spID, tr)
		}
		return upNextInfo{song: s, album: al}, true
	}
	title := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	return upNextInfo{song: Song{Path: p, Title: title}}, true
}

// spotifyNextAlbum devuelve un álbum temporal con la portada en caché de una
// pista de Spotify, o la descarga en segundo plano si aún no existe en disco.
func (a *App) spotifyNextAlbum(id string, tr spotifyTrack) *Album {
	if id == "" {
		return nil
	}
	u := &a.upNext
	if al := u.coverAlb[id]; al != nil {
		return al
	}
	if a.paths.Data == "" {
		return nil
	}
	coverPath := filepath.Join(a.paths.Data, "cache", "spotify", "songs", id+".jpg")
	if fi, err := os.Stat(coverPath); err == nil && fi.Size() > 0 {
		if u.coverAlb == nil {
			u.coverAlb = map[string]*Album{}
		}
		al := &Album{Title: tr.Album, Artist: tr.Artist, coverPath: coverPath, stray: true, scaled: map[int]*coverImage{}}
		u.coverAlb[id] = al
		return al
	}
	if tr.CoverURL == "" {
		return nil
	}
	if u.coverReq == nil {
		u.coverReq = map[string]bool{}
	}
	if u.coverReq[id] {
		return nil
	}
	u.coverReq[id] = true
	if u.coverCh == nil {
		u.coverCh = make(chan upNextCoverMsg, 8)
	}
	ch := u.coverCh
	url := tr.CoverURL
	go func() {
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil || len(data) == 0 {
			return
		}
		if err := os.MkdirAll(filepath.Dir(coverPath), 0o755); err != nil {
			return
		}
		tmp := coverPath + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			return
		}
		if err := os.Rename(tmp, coverPath); err != nil {
			_ = os.Remove(tmp)
			return
		}
		select {
		case ch <- upNextCoverMsg{id: id, path: coverPath}:
		default:
		}
	}()
	return nil
}

// upNextActive indica si en este fotograma debe mostrarse la vista previa de
// "Siguiente".
func (a *App) upNextActive() bool {
	if a.screen != NowPlaying || a.lyricsOn || a.playingSong.Path == "" {
		return false
	}
	mode := a.upNextMode()
	if mode == "off" {
		return false
	}
	if _, ok := a.upNextTrack(); !ok {
		return false
	}
	switch mode {
	case "always":
		if a.upNext.promote && a.t-a.upNext.promoteAt < juiceTrackLen {
			return false
		}
		if a.juice.trackAt > 0 && a.t-a.juice.trackAt < juiceTrackLen {
			return false
		}
		return true
	case "end":
		if a.duration <= 0 {
			return false
		}
		rem := a.duration - a.shownPos
		return rem <= upNextEndSec && rem > -1
	default:
		return false
	}
}

// updateUpNext avanza la animación de apertura/cierre de "Siguiente" y recoge
// las portadas de Spotify que hayan terminado de descargarse.
func (a *App) updateUpNext() {
	u := &a.upNext
	for u.coverCh != nil {
		select {
		case msg := <-u.coverCh:
			if u.coverAlb == nil {
				u.coverAlb = map[string]*Album{}
			}
			u.coverAlb[msg.id] = &Album{coverPath: msg.path, stray: true, scaled: map[int]*coverImage{}}
		default:
			goto drained
		}
	}
drained:
	dt := a.t - u.lastT
	u.lastT = a.t
	if dt < 0 {
		dt = 0
	} else if dt > 0.1 {
		dt = 0.1
	}

	want := 0.0
	if a.upNextActive() {
		want = 1.0
	}
	step := dt / upNextAnimLen
	if step <= 0 {
		step = 1.0 / (60.0 * upNextAnimLen)
	}
	switch {
	case u.p < want:
		u.p = math.Min(want, u.p+step)
	case u.p > want:
		u.p = math.Max(want, u.p-step)
	}

	if info, ok := a.upNextTrack(); ok {
		u.nextPath = info.song.Path
		if want > 0 || (a.upNextMode() == "end" && a.duration > 0 && a.duration-a.shownPos <= upNextEndSec+5) {
			if info.album != nil && a.covers != nil {
				_, _ = a.cover(info.album, 54)
				_, _ = a.cover(info.album, 108)
			}
		}
	} else {
		u.nextPath = ""
	}
}

// upNextOnTrackChange prepara la transición al cambiar de canción: si estaba
// abierta la vista de "Siguiente" hacia esta nueva pista, su portada crece
// desde el hueco de "Siguiente" hasta el lugar principal.
func (a *App) upNextOnTrackChange(newPath string, dir int) {
	u := &a.upNext
	if dir == 1 && u.lastVisible && u.nextPath == newPath && u.lastBoxSize > 0 {
		u.promote = true
		u.promoteAt = a.t
		u.fromX = u.lastBoxX
		u.fromY = u.lastBoxY
		u.fromSize = u.lastBoxSize
	} else {
		u.promote = false
	}
	u.p = 0
	u.lastVisible = false
}

// computeUpNextLayout calcula las posiciones y tamaños de las portadas y del
// disco según el progreso p (0 cerrado, 1 abierto).
func computeUpNextLayout(cx, cy, size, baseSlide int, p float64, wide bool) upNextLayout {
	p = math.Max(0, math.Min(1, p))
	if p == 0 {
		return upNextLayout{
			mainX:    cx,
			mainY:    cy,
			mainSize: size,
			slide:    baseSlide,
		}
	}
	sp := p * p * (3 - 2*p) // smoothstep
	targetMain := max(48, size*67/100)
	targetNext := max(36, size*50/100)
	mainSize := int(math.Round(float64(size) + float64(targetMain-size)*sp))
	nextSize := int(math.Round(float64(targetNext) * sp))
	slide := int(math.Round(float64(baseSlide) * (1 - sp)))

	mainX := cx
	if !wide {
		pairCenter := cx + size/2
		targetMainX := max(margin, pairCenter-(targetMain+10+targetNext)/2)
		mainX = int(math.Round(float64(cx) + float64(targetMainX-cx)*sp))
	}
	mainY := cy + (size-mainSize)/2
	nextX := mainX + mainSize + 10
	nextY := cy + (size-nextSize)/2 - 4

	return upNextLayout{
		mainX:    mainX,
		mainY:    mainY,
		mainSize: mainSize,
		slide:    slide,
		nextX:    nextX,
		nextY:    nextY,
		nextSize: nextSize,
		alpha:    sp,
	}
}

// upNextPromoteProgress devuelve la geometría de entrada cuando la nueva canción
// asciende desde la portada de "Siguiente" hasta el lugar principal.
func (a *App) upNextPromoteLayout(cx, cy, size, baseSlide int) (mainX, mainY, mainSize, slide int, ok bool) {
	u := &a.upNext
	if !u.promote {
		return 0, 0, 0, 0, false
	}
	q := (a.t - u.promoteAt) / juiceTrackLen
	if q < 0 || q >= 1 {
		u.promote = false
		return 0, 0, 0, 0, false
	}
	ep := 1 - math.Pow(1-q, 3) // easeOutCubic
	mainX = int(math.Round(float64(u.fromX) + float64(cx-u.fromX)*ep))
	mainY = int(math.Round(float64(u.fromY) + float64(cy-u.fromY)*ep))
	mainSize = int(math.Round(float64(u.fromSize) + float64(size-u.fromSize)*ep))
	slide = int(math.Round(float64(baseSlide) * ep))
	return mainX, mainY, mainSize, slide, true
}

// drawCoverScaled dibuja la portada de al escalada suavemente a drawSize usando
// la textura cacheada a fetchSize para no relanzar decodificaciones en cada
// fotograma de una animación.
func (a *App) drawCoverScaled(dst *ebiten.Image, al *Album, x, y, drawSize, fetchSize int, alpha float32) {
	if drawSize <= 0 {
		return
	}
	if drawSize == fetchSize {
		a.drawCover(dst, al, x, y, drawSize, alpha)
		return
	}
	var img *ebiten.Image
	var cAlpha float32 = 1
	if al != nil && a.covers != nil {
		img, cAlpha = a.cover(al, fetchSize)
	}
	if img == nil {
		th := a.theme
		panel(dst, x, y, drawSize, drawSize, withAlpha(th.Bg2, uint8(float32(0xff)*alpha)))
		drawIcon(dst, "note", x+(drawSize-7)/2, y+(drawSize-7)/2, withAlpha(th.Dim, uint8(float32(0xff)*alpha)))
		return
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(drawSize)/float64(b.Dx()), float64(drawSize)/float64(b.Dy()))
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleAlpha(cAlpha * alpha)
	dst.DrawImage(img, op)
}

// drawUpNextArt dibuja la portada de la siguiente canción junto a su etiqueta y
// título resumido cuando la vista previa de "Siguiente" está abierta.
func (a *App) drawUpNextArt(dst *ebiten.Image, l upNextLayout, info upNextInfo, fullSize, cy int) {
	if l.nextSize < 12 || l.alpha <= 0.05 {
		return
	}
	th := a.theme
	al := float32(l.alpha)
	fa := uint8(math.Round(255 * l.alpha))
	rect(dst, l.nextX+2, l.nextY+2, l.nextSize, l.nextSize, withAlpha(th.Shadow, uint8(float64(th.Shadow.A)*l.alpha)))
	a.drawCoverScaled(dst, info.album, l.nextX, l.nextY, l.nextSize, max(36, fullSize*50/100), al)
	frame(dst, l.nextX-1, l.nextY-1, l.nextSize+2, l.nextSize+2, withAlpha(th.Accent, uint8(0xc0*l.alpha)))

	// Pastilla "NEXT" justo encima de la portada siguiente.
	if l.alpha > 0.35 && SmallFace != nil {
		lbl := T("upnext_label")
		bw := textWidth(lbl, SmallFace) + 6
		by := l.nextY - 11
		if by >= topH+2 {
			panel(dst, l.nextX, by, bw, 9, withAlpha(th.Bg2, uint8(0xe0*l.alpha)))
			drawText(dst, lbl, SmallFace, l.nextX+3, by+2, withAlpha(th.Accent, fa))
		}
		// Título y artista resumidos bajo la portada siguiente.
		ty := l.nextY + l.nextSize + 4
		labelW := max(l.nextSize+16, 72)
		lx := max(l.mainX+l.mainSize+4, l.nextX+(l.nextSize-labelW)/2)
		if lx+labelW > dst.Bounds().Dx()-margin {
			labelW = max(l.nextSize, dst.Bounds().Dx()-margin-lx)
		}
		if ty+smallCap <= cy+fullSize {
			title := info.song.Title
			tw := textWidth(title, SmallFace)
			tx := lx + max(0, (labelW-tw)/2)
			drawTextW(dst, title, SmallFace, tx, ty, labelW, withAlpha(th.Text, fa))
			if info.song.Artist != "" && ty+10+smallCap <= cy+fullSize {
				aw := textWidth(info.song.Artist, SmallFace)
				ax := lx + max(0, (labelW-aw)/2)
				drawTextW(dst, info.song.Artist, SmallFace, ax, ty+10, labelW, withAlpha(th.Dim, fa))
			}
		}
	}
}
