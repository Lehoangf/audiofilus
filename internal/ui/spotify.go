package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/juanvillacortac/audiofilus/internal/control"
)

// Constantes de las pestañas de Spotify.
const (
	spLiked = iota
	spPlaylists
	spAlbums
	spSearch
	spCount
)

var spotifyTabKeys = [spCount]string{
	"spotify_liked",
	"spotify_playlists",
	"spotify_albums",
	"spotify_search",
}

// spotifyTrack representa una pista de Spotify en la interfaz.
type spotifyTrack struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	URI      string `json:"uri"`
	ISRC     string `json:"isrc"`
	CoverURL string `json:"cover_url,omitempty"`
}

// spotifyPlaylist representa una lista de reproducción de Spotify en la interfaz.
type spotifyPlaylist struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	SnapshotID string         `json:"snapshot_id"`
	Tracks     []spotifyTrack `json:"tracks"`
}

// spotifyState almacena el estado de la pantalla de Spotify.
type spotifyState struct {
	tab       int
	tabFocus  bool
	sel       [spCount]int
	scr       [spCount]scroller
	liked     []spotifyTrack
	playlists []spotifyPlaylist
	albums    []spotifyPlaylist
	searchRes []spotifyTrack
	openPl    *spotifyPlaylist
	openPlSel int
	openPlScr scroller
	query     string
	loggedIn  bool
	product   string
	loginURL  string
	loading   bool
	lastCheck time.Time
	syncing   bool
}

// initSpotify inicializa el estado de Spotify.
func (a *App) initSpotify() {
	a.spot.tab = spLiked
	a.checkSpotifyStatus()
}

// checkSpotifyStatus consulta al daemon si el usuario ya ha iniciado sesión en Spotify.
func (a *App) checkSpotifyStatus() {
	if a.offline || a.paths.Socket == "" {
		return
	}
	resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"status"}})
	if err != nil {
		return
	}
	var st struct {
		LoggedIn bool   `json:"logged_in"`
		Product  string `json:"product"`
	}
	if err := json.Unmarshal(resp.Data, &st); err == nil {
		a.spot.loggedIn = st.LoggedIn
		a.spot.product = st.Product
		if a.spot.loggedIn && len(a.spot.liked) == 0 && !a.spot.syncing {
			a.syncSpotifyCatalog()
		}
	}
}

// syncSpotifyCatalog sincroniza canciones favoritas y listas de Spotify desde el daemon.
func (a *App) syncSpotifyCatalog() {
	if a.offline || a.paths.Socket == "" || a.spot.syncing {
		return
	}
	a.spot.syncing = true
	a.spot.loading = true
	go func() {
		defer func() { a.spot.syncing = false; a.spot.loading = false }()

		// Cargar canciones marcadas como favoritas
		if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"liked"}}); err == nil {
			var liked []spotifyTrack
			if err := json.Unmarshal(resp.Data, &liked); err == nil {
				a.spot.liked = liked
			}
		}

		// Cargar listas de reproducción
		if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"playlists"}}); err == nil {
			var pls []spotifyPlaylist
			if err := json.Unmarshal(resp.Data, &pls); err == nil {
				a.spot.playlists = pls
			}
		}
	}()
}

// startSpotifyLogin inicia el flujo de autenticación QR en el daemon.
func (a *App) startSpotifyLogin() {
	if a.offline || a.paths.Socket == "" {
		a.flash(T("spotify_not_logged_in"))
		return
	}
	a.spot.loading = true
	go func() {
		defer func() { a.spot.loading = false }()
		resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"login"}})
		if err != nil {
			a.flash(err.Error())
			return
		}
		var data struct {
			LoginURL string `json:"login_url"`
		}
		if err := json.Unmarshal(resp.Data, &data); err == nil && data.LoginURL != "" {
			a.spot.loginURL = data.LoginURL
		}
	}()
}

// updateSpotify maneja la entrada de usuario en la pantalla de Spotify.
func (a *App) updateSpotify() {
	in := &a.input
	s := &a.spot

	// Comprobación periódica de estado si hay un flujo de inicio de sesión activo
	if s.loginURL != "" && time.Since(s.lastCheck) > 2*time.Second {
		s.lastCheck = time.Now()
		a.checkSpotifyStatus()
		if s.loggedIn {
			s.loginURL = ""
			a.flash(fmt.Sprintf(T("spotify_logged_in"), s.product))
		}
	}

	// Si no ha iniciado sesión
	if !s.loggedIn {
		if in.Pressed(ActConfirm) {
			if s.loginURL == "" {
				a.startSpotifyLogin()
			}
		}
		if in.Pressed(ActBack) {
			if s.loginURL != "" {
				s.loginURL = ""
			} else {
				a.goBack()
			}
		}
		return
	}

	// Si hay una lista de reproducción abierta
	if s.openPl != nil {
		n := len(s.openPl.Tracks)
		switch {
		case in.Repeat(ActUp):
			s.openPlSel = max(0, s.openPlSel-1)
		case in.Repeat(ActDown):
			s.openPlSel = min(max(0, n-1), s.openPlSel+1)
		case in.Repeat(ActLeft):
			s.openPlSel = max(0, s.openPlSel-10)
		case in.Repeat(ActRight):
			s.openPlSel = min(max(0, n-1), s.openPlSel+10)
		case in.Pressed(ActBack):
			s.openPl = nil
		case in.Pressed(ActConfirm) && n > 0:
			tr := s.openPl.Tracks[s.openPlSel]
			a.send("load", "0", "spotify://track/"+tr.ID)
			a.screen = NowPlaying
		case in.Pressed(ActMenu) && n > 0:
			tr := s.openPl.Tracks[s.openPlSel]
			a.spotifyTrackMenu(tr)
		}
		return
	}

	// Pestañas principales
	n := 0
	switch s.tab {
	case spLiked:
		n = len(s.liked)
	case spPlaylists:
		n = len(s.playlists)
	case spAlbums:
		n = len(s.albums)
	case spSearch:
		n = len(s.searchRes)
	}

	sel := s.sel[s.tab]
	switch {
	case in.Pressed(ActBack):
		if s.tabFocus {
			a.goBack()
		} else {
			s.tabFocus = true
		}
	case s.tabFocus && in.Repeat(ActUp):
		s.tab = (s.tab + spCount - 1) % spCount
	case s.tabFocus && in.Repeat(ActDown):
		s.tab = (s.tab + 1) % spCount
	case s.tabFocus && (in.Pressed(ActConfirm) || in.Pressed(ActRight)):
		s.tabFocus = false
	case !s.tabFocus && in.Pressed(ActLeft) && sel%10 == 0:
		s.tabFocus = true
	case !s.tabFocus && in.Repeat(ActUp):
		s.sel[s.tab] = max(0, sel-1)
	case !s.tabFocus && in.Repeat(ActDown):
		s.sel[s.tab] = min(max(0, n-1), sel+1)
	case !s.tabFocus && in.Repeat(ActLeft):
		s.sel[s.tab] = max(0, sel-10)
	case !s.tabFocus && in.Repeat(ActRight):
		s.sel[s.tab] = min(max(0, n-1), sel+10)
	case in.Pressed(ActShuffle): // Botón X: buscar
		a.spotifyAskSearch()
	case !s.tabFocus && in.Pressed(ActConfirm):
		switch s.tab {
		case spLiked:
			if n > 0 {
				tr := s.liked[sel]
				a.send("load", "0", "spotify://track/"+tr.ID)
				a.screen = NowPlaying
			}
		case spPlaylists:
			if n > 0 {
				s.openPl = &s.playlists[sel]
				s.openPlSel = 0
			}
		case spAlbums:
			if n > 0 {
				s.openPl = &s.albums[sel]
				s.openPlSel = 0
			}
		case spSearch:
			if n > 0 {
				tr := s.searchRes[sel]
				a.send("load", "0", "spotify://track/"+tr.ID)
				a.screen = NowPlaying
			} else {
				a.spotifyAskSearch()
			}
		}
	case !s.tabFocus && in.Pressed(ActMenu):
		switch s.tab {
		case spLiked:
			if n > 0 {
				a.spotifyTrackMenu(s.liked[sel])
			}
		case spSearch:
			if n > 0 {
				a.spotifyTrackMenu(s.searchRes[sel])
			}
		case spPlaylists:
			if n > 0 {
				pl := s.playlists[sel]
				a.spotifyPlaylistMenu(pl)
			}
		}
	}
}

// spotifyAskSearch abre el teclado virtual para buscar música en Spotify.
func (a *App) spotifyAskSearch() {
	a.openRawPrompt(T("spotify_search"), a.spot.query, false, func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			return
		}
		a.spot.query = q
		a.spot.tab = spSearch
		a.spot.tabFocus = false
		a.spotifySearch(q)
	})
}

// spotifySearch envía la búsqueda al daemon.
func (a *App) spotifySearch(query string) {
	if a.offline || a.paths.Socket == "" {
		return
	}
	a.spot.loading = true
	go func() {
		defer func() { a.spot.loading = false }()
		resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"search", query}})
		if err != nil {
			return
		}
		var tracks []spotifyTrack
		if err := json.Unmarshal(resp.Data, &tracks); err == nil {
			a.spot.searchRes = tracks
			a.spot.sel[spSearch] = 0
		}
	}()
}

// spotifyTrackMenu despliega el menú contextual de una canción de Spotify.
func (a *App) spotifyTrackMenu(tr spotifyTrack) {
	m := &menu{title: tr.Title, subtitle: tr.Artist}
	m.items = append(m.items, menuItem{
		label: T("spotify_play"),
		icon:  "play",
		do: func() {
			a.send("load", "0", "spotify://track/"+tr.ID)
			a.screen = NowPlaying
		},
	})
	m.items = append(m.items, menuItem{
		label: T("spotify_play_next"),
		icon:  "next",
		do: func() {
			a.send("enqueue", "next", "spotify://track/"+tr.ID)
			a.flash(T("queued_next"))
		},
	})
	m.items = append(m.items, menuItem{
		label: T("spotify_download"),
		icon:  "download",
		do: func() {
			a.send("spotify", "download", tr.ID)
			a.flash(T("spotify_downloaded"))
		},
	})
	a.menu = m
}

// spotifyPlaylistMenu despliega el menú contextual de una lista de reproducción.
func (a *App) spotifyPlaylistMenu(pl spotifyPlaylist) {
	m := &menu{title: pl.Name, subtitle: fmt.Sprintf("%d canciones", len(pl.Tracks))}
	m.items = append(m.items, menuItem{
		label: T("spotify_play"),
		icon:  "play",
		do: func() {
			if len(pl.Tracks) > 0 {
				var uris []string
				uris = append(uris, "0")
				for _, tr := range pl.Tracks {
					uris = append(uris, "spotify://track/"+tr.ID)
				}
				a.send("load", uris...)
				a.screen = NowPlaying
			}
		},
	})
	a.menu = m
}

// drawSpotify renderiza la pantalla de Spotify en la interfaz.
func (a *App) drawSpotify(dst *ebiten.Image) {
	th := a.theme
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	s := &a.spot
	x0, top := a.contentX(), topH+8

	// Si no se ha iniciado sesión, mostrar login o código QR
	if !s.loggedIn {
		a.drawTopBar(dst, T("scr_spotify"))
		if s.loginURL != "" {
			a.drawSpotifyQR(dst, s.loginURL)
		} else {
			a.drawSpotifyLoginPrompt(dst)
		}
		hints := []hint{{"A", T("spotify_login_btn")}, {"B", T("h_back")}}
		a.drawHints(dst, hints)
		return
	}

	// Título de la barra superior
	title := T("scr_spotify") + " · " + T(spotifyTabKeys[s.tab])
	if s.openPl != nil {
		title = T("scr_spotify") + " · " + s.openPl.Name
	}

	// Pestañas laterales si la pantalla es amplia
	if w >= 320 && s.openPl == nil {
		for i := 0; i < spCount; i++ {
			y := topH + 8 + i*16
			tabName := T(spotifyTabKeys[i])
			idx := i
			a.tapZone(margin-3, y-4, 72, 13, func() {
				s.tab = idx
				s.tabFocus = false
			})
			switch {
			case i == s.tab && s.tabFocus:
				panel(dst, margin-3, y-4, 72, 13, th.Accent)
				drawText(dst, tabName, SmallFace, margin+2, y, th.Bg)
			case i == s.tab:
				panel(dst, margin-3, y-4, 72, 13, mix(th.Bg2, th.Accent, 0.35))
				drawText(dst, tabName, SmallFace, margin+2, y, th.Text)
			default:
				drawText(dst, tabName, SmallFace, margin+2, y, th.Dim)
			}
		}
		rect(dst, margin+75, topH+5, 1, h-topH-bottomH-44, withAlpha(th.Dim, 0x40))
	}

	cw := w - x0 - margin
	bottom := h - bottomH - 36
	const rowH = 22

	var hints []hint
	if s.openPl != nil {
		pl := s.openPl
		a.tapZone(x0-3, top-4, cw+6, 13, func() { s.openPl = nil })
		drawTextW(dst, "< "+pl.Name, SmallFace, x0, top, cw, th.Accent2)
		top += 12

		rows := max(1, (bottom-top)/rowH)
		n := len(pl.Tracks)
		first, off, clip := a.touchList(dst, &s.openPlScr, &s.openPlSel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			a.input.inject(ActConfirm)
		})
		whole := dst
		dst = clip
		for i := 0; i <= rows && first+i < n; i++ {
			tr := pl.Tracks[first+i]
			y := top + 3 + i*rowH - off
			if first+i == s.openPlSel {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}
			isCurrent := strings.HasSuffix(a.playingSong.Path, tr.ID)
			icCol := th.Dim
			if isCurrent {
				icCol = th.Accent
			}
			drawIcon(dst, "note", x0, y, icCol)
			drawTextW(dst, tr.Title, SmallFace, x0+12, y, cw-12, th.Text)
			sub := tr.Artist
			if tr.Duration > 0 {
				sub += fmt.Sprintf(" · %s", fmtDur(tr.Duration))
			}
			drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)
		if n == 0 {
			drawText(dst, T("spotify_empty"), SmallFace, x0+(cw-textWidth(T("spotify_empty"), SmallFace))/2, top+30, th.Dim)
		}
		hints = []hint{{"A", T("h_play")}, {"Y", T("h_more")}, {"B", T("h_back")}}
	} else {
		// Mostrar lista de la pestaña activa
		var tracks []spotifyTrack
		var playlists []spotifyPlaylist
		isTracks := true

		switch s.tab {
		case spLiked:
			tracks = s.liked
		case spPlaylists:
			playlists = s.playlists
			isTracks = false
		case spAlbums:
			playlists = s.albums
			isTracks = false
		case spSearch:
			tracks = s.searchRes
		}

		rows := max(1, (bottom-top)/rowH)
		n := len(tracks)
		if !isTracks {
			n = len(playlists)
		}

		first, off, clip := a.touchList(dst, &s.scr[s.tab], &s.sel[s.tab], n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			a.input.inject(ActConfirm)
		})
		whole := dst
		dst = clip

		for i := 0; i <= rows && first+i < n; i++ {
			y := top + 3 + i*rowH - off
			if first+i == s.sel[s.tab] && !s.tabFocus {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}

			if isTracks {
				tr := tracks[first+i]
				isCurrent := strings.HasSuffix(a.playingSong.Path, tr.ID)
				icCol := th.Dim
				if isCurrent {
					icCol = th.Accent
				}
				drawIcon(dst, "note", x0, y, icCol)
				drawTextW(dst, tr.Title, SmallFace, x0+12, y, cw-12, th.Text)
				sub := tr.Artist
				if tr.Duration > 0 {
					sub += fmt.Sprintf(" · %s", fmtDur(tr.Duration))
				}
				drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
			} else {
				pl := playlists[first+i]
				drawIcon(dst, "list", x0, y, th.Accent2)
				drawTextW(dst, pl.Name, SmallFace, x0+12, y, cw-12, th.Text)
				drawTextW(dst, fmt.Sprintf("%d canciones", len(pl.Tracks)), SmallFace, x0+12, y+9, cw-12, th.Dim)
			}
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

		if n == 0 {
			msg := T("spotify_empty")
			if s.tab == spSearch && s.query == "" {
				msg = T("spotify_search") + " (X)"
			}
			drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
		}

		hints = []hint{{"A", T("h_play")}, {"X", T("spotify_search")}, {"Y", T("h_more")}, {"B", T("h_back")}}
	}

	a.drawMiniPlayer(dst)
	a.drawTopBar(dst, title)
	a.drawHints(dst, hints)
}

// drawSpotifyLoginPrompt pinta el mensaje de inicio de sesión cuando aún no se ha conectado.
func (a *App) drawSpotifyLoginPrompt(dst *ebiten.Image) {
	th := a.theme
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx := w / 2
	cy := h / 2

	drawIcon(dst, "note", cx-8, cy-48, th.Accent)
	title := "Spotify"
	drawText(dst, title, BigFace, cx-textWidth(title, BigFace)/2, cy-32, th.Text)

	desc := T("spotify_login_prompt")
	drawTextW(dst, desc, SmallFace, cx-120, cy-10, 240, th.Dim)

	// Botón de inicio de sesión
	btnW, btnH := 160, 24
	btnX := cx - btnW/2
	btnY := cy + 18
	panel(dst, btnX, btnY, btnW, btnH, mix(th.Bg2, th.Accent, 0.4))
	rect(dst, btnX, btnY, 2, btnH, th.Accent)
	btnText := T("spotify_login_btn")
	drawText(dst, btnText, SmallFace, cx-textWidth(btnText, SmallFace)/2, btnY+7, th.Text)
}

// drawSpotifyQR pinta el código QR para el inicio de sesión OAuth2 PKCE.
func (a *App) drawSpotifyQR(dst *ebiten.Image, urlStr string) {
	th := a.theme
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	rect(dst, 0, 0, w, h, withAlpha(th.Bg, 0xd0))

	qr := qrFor(urlStr)
	if qr == nil {
		return
	}

	n := qr.Bounds().Dx()
	room := h - topH - bottomH - 40
	s := max(1, min(room/n, 8))
	if room >= 2*n {
		s = max(2, min(room/n, 8))
	}
	size := n * s

	displayURL := urlStr
	if len(displayURL) > 40 {
		displayURL = displayURL[:37] + "..."
	}

	pw := max(size, textWidth(displayURL, SmallFace)) + 24
	ph := size + 44
	px, py := (w-pw)/2, topH+(h-topH-bottomH-ph)/2

	panel(dst, px, py, pw, ph, th.Bg2)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(s), float64(s))
	op.GeoM.Translate(float64((w-size)/2), float64(py+8))
	dst.DrawImage(qr, op)

	scanText := T("spotify_login_scan")
	drawText(dst, scanText, SmallFace, (w-textWidth(scanText, SmallFace))/2, py+size+14, th.Accent)
	drawText(dst, displayURL, SmallFace, (w-textWidth(displayURL, SmallFace))/2, py+size+26, th.Dim)
}
