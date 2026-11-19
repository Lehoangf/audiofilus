package ui

import (
	"context"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/juanvillacortac/audiofilus/internal/ytm"
)

// Dedicated YouTube Music screen offering Search, Trending, Charts, Genres, and Recent tabs.

type ytmTab int

const (
	ytmTabSearch ytmTab = iota
	ytmTabTrending
	ytmTabCharts
	ytmTabGenres
	ytmTabRecent
	ytmTabCount
)

var ytmTabKeys = []string{
	"ytm_tab_search",
	"ytm_tab_trending",
	"ytm_tab_charts",
	"ytm_tab_genres",
	"ytm_tab_recent",
}

type ytmGenreItem struct {
	Name  string
	Query string
	Icon  string
}

var ytmGenres = []ytmGenreItem{
	{"Lofi & Chill", "lofi chill beats music", "note"},
	{"Focus & Study", "study focus background music", "headphones"},
	{"Gaming & Synthwave", "synthwave gaming retrowave music", "pad"},
	{"Workout & Energy", "workout motivation gym music", "speaker"},
	{"Sleep & Relax", "relaxing ambient sleep music", "moon"},
	{"Rock & Indie", "top rock indie hits music", "note"},
	{"Pop & Electronic", "electronic pop dance hits music", "disc"},
	{"Acoustic & Ballad", "acoustic guitar ballad music", "mic"},
	{"Jazz & Blues", "smooth jazz blues classics music", "note"},
}

type ytScreenState struct {
	tab          ytmTab
	tabFocus     bool
	sel          int
	scr          scroller
	trending     []ytm.Track
	trendLoaded  bool
	charts       []ytm.Track
	chartsLoaded bool
	recent       []ytm.Track
	activeGenre  string
	genreTracks  []ytm.Track
	genreSel     int
	genreScr     scroller
	loading      bool
	loadingMsg   string
	searchQuery  string
	searchRes    []ytm.Track
}

// addRecentYT appends a track to the YouTube Music recent list.
func (a *App) addRecentYT(tr ytm.Track) {
	if tr.ID == "" {
		return
	}
	var updated []ytm.Track
	updated = append(updated, tr)
	for _, t := range a.ytScr.recent {
		if t.ID != tr.ID && len(updated) < 50 {
			updated = append(updated, t)
		}
	}
	a.ytScr.recent = updated
}

func (a *App) loadYTMTrending() {
	if a.ytScr.loading || a.yt.bin == "" {
		return
	}
	a.ytScr.loading = true
	a.ytScr.loadingMsg = T("ytm_loading_trending")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		tracks, err := ytm.Search(ctx, "top hits songs")
		if err == nil && len(tracks) > 0 {
			a.ytScr.trending = tracks
			a.ytScr.trendLoaded = true
		} else {
			tracks2, err2 := ytm.Search(ctx, "popular music")
			if err2 == nil && len(tracks2) > 0 {
				a.ytScr.trending = tracks2
				a.ytScr.trendLoaded = true
			}
		}
		a.ytScr.loading = false
		ebiten.ScheduleFrame()
	}()
}

func (a *App) loadYTMCharts() {
	if a.ytScr.loading || a.yt.bin == "" {
		return
	}
	a.ytScr.loading = true
	a.ytScr.loadingMsg = T("ytm_loading_charts")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		tracks, err := ytm.Search(ctx, "billboard top 50 hits")
		if err == nil && len(tracks) > 0 {
			a.ytScr.charts = tracks
			a.ytScr.chartsLoaded = true
		} else {
			tracks2, err2 := ytm.Search(ctx, "top 50 charts")
			if err2 == nil && len(tracks2) > 0 {
				a.ytScr.charts = tracks2
				a.ytScr.chartsLoaded = true
			}
		}
		a.ytScr.loading = false
		ebiten.ScheduleFrame()
	}()
}

func (a *App) loadYTMGenre(name, query string) {
	if a.ytScr.loading || a.yt.bin == "" {
		return
	}
	a.ytScr.loading = true
	a.ytScr.loadingMsg = name
	a.ytScr.activeGenre = name
	a.ytScr.genreTracks = nil
	a.ytScr.genreSel = 0
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		tracks, err := ytm.Search(ctx, query)
		if err == nil {
			a.ytScr.genreTracks = tracks
		}
		a.ytScr.loading = false
		ebiten.ScheduleFrame()
	}()
}

func (a *App) ytScreenDoSearch(q string) {
	q = strings.TrimSpace(q)
	if q == "" || a.yt.bin == "" {
		return
	}
	a.ytScr.searchQuery = q
	a.ytScr.loading = true
	a.ytScr.loadingMsg = T("ytm_searching")
	a.ytScr.sel = 0
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		tracks, err := ytm.Search(ctx, q)
		if err == nil {
			a.ytScr.searchRes = tracks
		}
		a.ytScr.loading = false
		ebiten.ScheduleFrame()
	}()
}

func (a *App) ytTrackMenu(tr ytm.Track) {
	items := []menuItem{
		{
			label: T("h_play"),
			icon:  "play",
			close: true,
			do: func() {
				a.addRecentYT(tr)
				a.ytPlay(tr)
			},
		},
		{
			label: T("pd_download"),
			icon:  "download",
			close: true,
			do: func() {
				a.ytDownload(tr)
			},
		},
		{
			label: T("h_open") + " (" + tr.Duration + ")",
			icon:  "note",
			close: true,
			do: func() {
				a.flash(tr.Title + " - " + tr.Artist())
			},
		},
	}
	a.menu = &menu{
		title:    tr.Title,
		subtitle: tr.Artist(),
		items:    items,
		cols:     1,
	}
}

func (a *App) updateYTMusicScreen() {
	in := &a.input
	s := &a.ytScr
	w := a.w

	if a.yt.bin == "" {
		if in.Pressed(ActBack) {
			a.goBack()
		}
		return
	}

	if s.tab == ytmTabTrending && !s.trendLoaded && !s.loading && len(s.trending) == 0 {
		a.loadYTMTrending()
	}
	if s.tab == ytmTabCharts && !s.chartsLoaded && !s.loading && len(s.charts) == 0 {
		a.loadYTMCharts()
	}

	// Tab switching via L and R shoulder buttons
	if in.Pressed(ActPrev) {
		s.tab = (s.tab - 1 + ytmTabCount) % ytmTabCount
		s.sel = 0
		s.activeGenre = ""
		return
	}
	if in.Pressed(ActNext) {
		s.tab = (s.tab + 1) % ytmTabCount
		s.sel = 0
		s.activeGenre = ""
		return
	}

	if in.Pressed(ActBack) {
		if s.tab == ytmTabGenres && s.activeGenre != "" {
			s.activeGenre = ""
			return
		}
		a.goBack()
		return
	}

	// Side tab panel navigation on wide displays
	if w >= 320 && s.tabFocus {
		switch {
		case in.Repeat(ActDown):
			s.tab = (s.tab + 1) % ytmTabCount
			s.sel = 0
			s.activeGenre = ""
		case in.Repeat(ActUp):
			s.tab = (s.tab - 1 + ytmTabCount) % ytmTabCount
			s.sel = 0
			s.activeGenre = ""
		case in.Pressed(ActRight), in.Pressed(ActConfirm):
			s.tabFocus = false
		}
		return
	}

	// Focus side tab panel with Left
	if w >= 320 && !s.tabFocus && in.Pressed(ActLeft) && (s.tab != ytmTabGenres || s.activeGenre == "") {
		s.tabFocus = true
		return
	}

	switch s.tab {
	case ytmTabSearch:
		itemsCount := 1 + len(s.searchRes)
		if in.Repeat(ActDown) {
			s.sel = (s.sel + 1) % itemsCount
		} else if in.Repeat(ActUp) {
			s.sel = (s.sel - 1 + itemsCount) % itemsCount
		} else if in.Pressed(ActConfirm) {
			if s.sel == 0 {
				a.openPrompt(T("ytm_search_q"), s.searchQuery, func(q string) {
					a.ytScreenDoSearch(q)
				})
			} else {
				idx := s.sel - 1
				if idx >= 0 && idx < len(s.searchRes) {
					tr := s.searchRes[idx]
					a.addRecentYT(tr)
					a.ytPlay(tr)
				}
			}
		} else if in.Pressed(ActMenu) || in.Pressed(ActPause) {
			idx := s.sel - 1
			if idx >= 0 && idx < len(s.searchRes) {
				a.ytTrackMenu(s.searchRes[idx])
			}
		} else if in.Pressed(ActShuffle) {
			a.openPrompt(T("ytm_search_q"), s.searchQuery, func(q string) {
				a.ytScreenDoSearch(q)
			})
		}

	case ytmTabTrending:
		n := len(s.trending)
		if n == 0 {
			if in.Pressed(ActConfirm) && !s.loading {
				a.loadYTMTrending()
			}
			return
		}
		if in.Repeat(ActDown) {
			s.sel = (s.sel + 1) % n
		} else if in.Repeat(ActUp) {
			s.sel = (s.sel - 1 + n) % n
		} else if in.Pressed(ActConfirm) {
			tr := s.trending[s.sel]
			a.addRecentYT(tr)
			a.ytPlay(tr)
		} else if in.Pressed(ActMenu) || in.Pressed(ActPause) {
			a.ytTrackMenu(s.trending[s.sel])
		}

	case ytmTabCharts:
		n := len(s.charts)
		if n == 0 {
			if in.Pressed(ActConfirm) && !s.loading {
				a.loadYTMCharts()
			}
			return
		}
		if in.Repeat(ActDown) {
			s.sel = (s.sel + 1) % n
		} else if in.Repeat(ActUp) {
			s.sel = (s.sel - 1 + n) % n
		} else if in.Pressed(ActConfirm) {
			tr := s.charts[s.sel]
			a.addRecentYT(tr)
			a.ytPlay(tr)
		} else if in.Pressed(ActMenu) || in.Pressed(ActPause) {
			a.ytTrackMenu(s.charts[s.sel])
		}

	case ytmTabGenres:
		if s.activeGenre != "" {
			n := len(s.genreTracks)
			if n == 0 {
				return
			}
			if in.Repeat(ActDown) {
				s.genreSel = (s.genreSel + 1) % n
			} else if in.Repeat(ActUp) {
				s.genreSel = (s.genreSel - 1 + n) % n
			} else if in.Pressed(ActConfirm) {
				tr := s.genreTracks[s.genreSel]
				a.addRecentYT(tr)
				a.ytPlay(tr)
			} else if in.Pressed(ActMenu) || in.Pressed(ActPause) {
				a.ytTrackMenu(s.genreTracks[s.genreSel])
			}
		} else {
			n := len(ytmGenres)
			if in.Repeat(ActDown) {
				s.sel = (s.sel + 1) % n
			} else if in.Repeat(ActUp) {
				s.sel = (s.sel - 1 + n) % n
			} else if in.Pressed(ActConfirm) {
				g := ytmGenres[s.sel]
				a.loadYTMGenre(g.Name, g.Query)
			}
		}

	case ytmTabRecent:
		n := len(s.recent)
		if n == 0 {
			return
		}
		if in.Repeat(ActDown) {
			s.sel = (s.sel + 1) % n
		} else if in.Repeat(ActUp) {
			s.sel = (s.sel - 1 + n) % n
		} else if in.Pressed(ActConfirm) {
			tr := s.recent[s.sel]
			a.addRecentYT(tr)
			a.ytPlay(tr)
		} else if in.Pressed(ActMenu) || in.Pressed(ActPause) {
			a.ytTrackMenu(s.recent[s.sel])
		}
	}
}

func (a *App) drawYTMusicScreen(dst *ebiten.Image) {
	th := a.theme
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	s := &a.ytScr

	// Display warning banner when yt-dlp binary is missing
	if a.yt.bin == "" {
		a.drawTopBar(dst, T("scr_ytmusic"))
		cx, cy := w/2, h/2-10
		drawIcon(dst, "ytm", cx-3, cy-32, th.Accent)
		title := "YouTube Music"
		drawText(dst, title, BigFace, cx-textWidth(title, BigFace)/2, cy-16, th.Text)

		msg := T("ytm_no_bin")
		tw := textWidth(msg, SmallFace)
		maxW := w - 40
		if tw <= maxW {
			drawText(dst, msg, SmallFace, cx-tw/2, cy+4, th.Dim)
		} else {
			drawTextW(dst, msg, SmallFace, cx-maxW/2, cy+4, maxW, th.Dim)
		}

		a.drawMiniPlayer(dst)
		a.drawHints(dst, []hint{{"B", T("h_back")}})
		return
	}

	x0 := a.contentX()
	cw := w - x0 - margin
	currTrack, isStreaming := a.streamTrack()

	// Loading banner
	if s.loading {
		msg := s.loadingMsg
		if msg == "" {
			msg = T("ytm_searching")
		}
		bw := textWidth(msg, SmallFace) + 24
		bx := x0 + max(0, (cw-bw)/2)
		by := topH + 3
		panel(dst, bx, by, bw, 13, mix(th.Bg2, th.Accent, 0.4))
		rect(dst, bx, by, 2, 13, th.Accent)
		drawIcon(dst, "hourglass1", bx+4, by+3, th.Accent)
		drawText(dst, msg, SmallFace, bx+14, by+3, th.Text)
	}

	// Side tabs
	if w >= 320 {
		for i := 0; i < int(ytmTabCount); i++ {
			y := topH + 8 + i*16
			tabName := T(ytmTabKeys[i])
			idx := ytmTab(i)
			a.tapZone(margin-3, y-4, 72, 13, func() {
				s.tab = idx
				s.tabFocus = false
				s.sel = 0
				s.activeGenre = ""
			})
			switch {
			case idx == s.tab && s.tabFocus:
				panel(dst, margin-3, y-4, 72, 13, th.Accent)
				drawText(dst, tabName, SmallFace, margin+2, y, th.Bg)
			case idx == s.tab:
				panel(dst, margin-3, y-4, 72, 13, mix(th.Bg2, th.Accent, 0.35))
				drawText(dst, tabName, SmallFace, margin+2, y, th.Text)
			default:
				drawText(dst, tabName, SmallFace, margin+2, y, th.Dim)
			}
		}
		rect(dst, margin+75, topH+5, 1, h-topH-bottomH-44, withAlpha(th.Dim, 0x40))
	}

	top := topH + 8
	bottom := h - bottomH - 36
	const rowH = 22

	var hints []hint

	switch s.tab {
	case ytmTabSearch:
		// Library-style search box
		panel(dst, x0-3, top-4, cw+6, 16, withAlpha(th.Bg2, 0xd0))
		if s.sel == 0 && !s.tabFocus {
			panel(dst, x0-3, top-4, cw+6, 16, mix(th.Bg2, th.Accent, 0.35))
			rect(dst, x0-3, top-4, 2, 16, th.Accent)
		}
		drawIcon(dst, "search", x0+1, top-1, th.Dim)
		sq := s.searchQuery
		if sq == "" {
			sq = T("ytm_search_q") + "..."
			drawText(dst, sq, SmallFace, x0+14, top, th.Dim)
		} else {
			drawText(dst, sq, SmallFace, x0+14, top, th.Text)
		}
		a.tapZone(x0-3, top-4, cw+6, 16, func() {
			s.sel = 0
			s.tabFocus = false
			a.openPrompt(T("ytm_search_q"), s.searchQuery, func(q string) {
				a.ytScreenDoSearch(q)
			})
		})
		top += 18

		rows := max(1, (bottom-top)/rowH)
		n := len(s.searchRes)
		itemSel := max(0, s.sel-1)

		first, off, clip := a.touchList(dst, &s.scr, &itemSel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			s.sel = itemSel + 1
			a.input.inject(ActConfirm)
		})
		whole := dst
		dst = clip

		for i := 0; i <= rows && first+i < n; i++ {
			tr := s.searchRes[first+i]
			y := top + 3 + i*rowH - off
			if first+i+1 == s.sel && !s.tabFocus {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}
			isCurrent := isStreaming && currTrack.ID == tr.ID
			icCol := ytRed
			if isCurrent {
				icCol = th.Accent
			}
			drawIcon(dst, "ytm", x0, y, icCol)
			drawTextW(dst, tr.Title, SmallFace, x0+12, y, cw-12, th.Text)
			sub := tr.Artist()
			if tr.Duration != "" {
				sub += " · " + tr.Duration
			}
			drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

		if n == 0 && !s.loading {
			msg := T("ytm_no_results")
			if s.searchQuery == "" {
				msg = T("ytm_search_prompt")
			}
			drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
		}
		hints = []hint{{"A", T("h_play")}, {"X", T("spotify_search")}, {"START", T("h_more")}, {"B", T("h_back")}}

	case ytmTabTrending:
		rows := max(1, (bottom-top)/rowH)
		n := len(s.trending)
		first, off, clip := a.touchList(dst, &s.scr, &s.sel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			a.input.inject(ActConfirm)
		})
		whole := dst
		dst = clip

		for i := 0; i <= rows && first+i < n; i++ {
			tr := s.trending[first+i]
			y := top + 3 + i*rowH - off
			if first+i == s.sel && !s.tabFocus {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}
			isCurrent := isStreaming && currTrack.ID == tr.ID
			icCol := ytRed
			if isCurrent {
				icCol = th.Accent
			}
			drawIcon(dst, "ytm", x0, y, icCol)
			drawTextW(dst, tr.Title, SmallFace, x0+12, y, cw-12, th.Text)
			sub := tr.Artist()
			if tr.Duration != "" {
				sub += " · " + tr.Duration
			}
			drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

		if n == 0 && !s.loading {
			msg := T("ytm_no_trending")
			drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
		}
		hints = []hint{{"A", T("h_play")}, {"START", T("h_more")}, {"B", T("h_back")}}

	case ytmTabCharts:
		rows := max(1, (bottom-top)/rowH)
		n := len(s.charts)
		first, off, clip := a.touchList(dst, &s.scr, &s.sel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			a.input.inject(ActConfirm)
		})
		whole := dst
		dst = clip

		for i := 0; i <= rows && first+i < n; i++ {
			tr := s.charts[first+i]
			y := top + 3 + i*rowH - off
			if first+i == s.sel && !s.tabFocus {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}
			isCurrent := isStreaming && currTrack.ID == tr.ID
			icCol := ytRed
			if isCurrent {
				icCol = th.Accent
			}
			drawIcon(dst, "ytm", x0, y, icCol)
			drawTextW(dst, tr.Title, SmallFace, x0+12, y, cw-12, th.Text)
			sub := tr.Artist()
			if tr.Duration != "" {
				sub += " · " + tr.Duration
			}
			drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

		if n == 0 && !s.loading {
			msg := T("ytm_no_trending")
			drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
		}
		hints = []hint{{"A", T("h_play")}, {"START", T("h_more")}, {"B", T("h_back")}}

	case ytmTabGenres:
		if s.activeGenre != "" {
			a.tapZone(x0-3, top-4, cw+6, 13, func() { s.activeGenre = "" })
			drawTextW(dst, "< "+s.activeGenre, SmallFace, x0, top, cw, th.Accent2)
			top += 12

			rows := max(1, (bottom-top)/rowH)
			n := len(s.genreTracks)
			first, off, clip := a.touchList(dst, &s.genreScr, &s.genreSel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
				a.input.inject(ActConfirm)
			})
			whole := dst
			dst = clip

			for i := 0; i <= rows && first+i < n; i++ {
				tr := s.genreTracks[first+i]
				y := top + 3 + i*rowH - off
				if first+i == s.genreSel {
					panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
					rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
				}
				isCurrent := isStreaming && currTrack.ID == tr.ID
				icCol := ytRed
				if isCurrent {
					icCol = th.Accent
				}
				drawIcon(dst, "ytm", x0, y, icCol)
				drawTextW(dst, tr.Title, SmallFace, x0+12, y, cw-12, th.Text)
				sub := tr.Artist()
				if tr.Duration != "" {
					sub += " · " + tr.Duration
				}
				drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
			}
			dst = whole
			a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

			if n == 0 && !s.loading {
				msg := T("ytm_no_results")
				drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
			}
			hints = []hint{{"A", T("h_play")}, {"START", T("h_more")}, {"B", T("h_back")}}
		} else {
			// Genre list
			rows := max(1, (bottom-top)/rowH)
			n := len(ytmGenres)
			first, off, clip := a.touchList(dst, &s.scr, &s.sel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
				a.input.inject(ActConfirm)
			})
			whole := dst
			dst = clip

			for i := 0; i <= rows && first+i < n; i++ {
				g := ytmGenres[first+i]
				y := top + 3 + i*rowH - off
				if first+i == s.sel && !s.tabFocus {
					panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
					rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
				}
				drawIcon(dst, g.Icon, x0, y, th.Accent)
				drawTextW(dst, g.Name, SmallFace, x0+12, y+3, cw-12, th.Text)
			}
			dst = whole
			a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)
			hints = []hint{{"A", T("h_open")}, {"B", T("h_back")}}
		}

	case ytmTabRecent:
		rows := max(1, (bottom-top)/rowH)
		n := len(s.recent)
		first, off, clip := a.touchList(dst, &s.scr, &s.sel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			a.input.inject(ActConfirm)
		})
		whole := dst
		dst = clip

		for i := 0; i <= rows && first+i < n; i++ {
			tr := s.recent[first+i]
			y := top + 3 + i*rowH - off
			if first+i == s.sel && !s.tabFocus {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}
			isCurrent := isStreaming && currTrack.ID == tr.ID
			icCol := ytRed
			if isCurrent {
				icCol = th.Accent
			}
			drawIcon(dst, "ytm", x0, y, icCol)
			drawTextW(dst, tr.Title, SmallFace, x0+12, y, cw-12, th.Text)
			sub := tr.Artist()
			if tr.Duration != "" {
				sub += " · " + tr.Duration
			}
			drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

		if n == 0 {
			msg := T("ytm_no_recent")
			drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
		}
		hints = []hint{{"A", T("h_play")}, {"START", T("h_more")}, {"B", T("h_back")}}
	}

	a.drawMiniPlayer(dst)
	a.drawTopBar(dst, T("scr_ytmusic")+" · "+T(ytmTabKeys[s.tab]))
	a.drawHints(dst, hints)
}
