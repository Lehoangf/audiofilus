package ui

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/juanvillacortac/audiofilus/internal/engine"
	"github.com/juanvillacortac/audiofilus/internal/lyrics"
)

// TestRadioSong comprueba cómo se parte el título de una radio para buscar
// su letra: "Artista - Canción", con guiones en la canción, y lo que no es
// una canción.
func TestRadioSong(t *testing.T) {
	for _, c := range []struct{ in, artist, title string }{
		{"Radiohead - A Wolf at the Door", "Radiohead", "A Wolf at the Door"},
		{"AC/DC - Back In Black - Live", "AC/DC", "Back In Black - Live"},
		{"Classic FM UK", "", ""},
		{" - Untitled", "", ""},
	} {
		artist, title, ok := radioSong(c.in)
		if ok != (c.artist != "") || (ok && (artist != c.artist || title != c.title)) {
			t.Errorf("radioSong(%q) = %q, %q, %v", c.in, artist, title, ok)
		}
	}
}

// TestTrackerFeedsMeter comprueba que lo que toca el tracker llegue al
// medidor (meterTap) y que, mientras toca, sea lo que miran los
// visualizadores y los juegos (meterPts, hearing).
func TestTrackerFeedsMeter(t *testing.T) {
	an := engine.NewAnalyzer(44100, 0)
	src := make([]byte, 4*2*4096) // 4096 cuadros estéreo de un tono
	for i := 0; i < 2*4096; i++ {
		v := float32(0.5 * math.Sin(float64(i/2)*2*math.Pi*110/44100))
		binary.LittleEndian.PutUint32(src[i*4:], math.Float32bits(v))
	}
	tap := &meterTap{r: bytes.NewReader(src), an: an}
	if _, err := io.ReadAll(tap); err != nil {
		t.Fatal(err)
	}
	pts := an.Points()
	if len(pts) < 3 || pts[0].Level == 0 || pts[0].Bass == 0 {
		t.Fatalf("meter points = %+v", pts)
	}
	a := &App{tracker: &trackerState{playing: true, an: an}}
	if !a.hearing() || len(a.meterPts()) != len(pts) {
		t.Error("while the tracker plays, the meter should be the tracker's")
	}
	a.tracker.playing = false
	if a.hearing() || len(a.meterPts()) != 0 {
		t.Error("with the tracker stopped and no music, nothing should be heard")
	}
}

// TestMiniLyricsMenuItems checks that both the Lyrics menu and Now Playing menu
// provide the toggle for showing mini lyrics inside Now Playing.
func TestMiniLyricsMenuItems(t *testing.T) {
	a := &App{offline: true, playingSong: Song{Path: "/music/test.mp3", Title: "Test"}}

	a.openLyricsMenu()
	if a.menu == nil {
		t.Fatal("expected lyrics menu to open")
	}
	var lyricsItem *menuItem
	for i := range a.menu.items {
		if a.menu.items[i].label == T("lyr_now_playing") {
			lyricsItem = &a.menu.items[i]
			break
		}
	}
	if lyricsItem == nil {
		t.Fatalf("missing %q option in openLyricsMenu", T("lyr_now_playing"))
	}
	if a.cfg.UI.NowPlayingLyrics {
		t.Fatal("expected NowPlayingLyrics default false")
	}
	lyricsItem.do()
	if !a.cfg.UI.NowPlayingLyrics {
		t.Fatal("expected NowPlayingLyrics true after toggling in lyrics menu")
	}

	a.menu = nil
	a.openNowMenu()
	if a.menu == nil {
		t.Fatal("expected now playing menu to open")
	}
	var nowItem *menuItem
	for i := range a.menu.items {
		if a.menu.items[i].label == T("lyr_now_playing") {
			nowItem = &a.menu.items[i]
			break
		}
	}
	if nowItem == nil {
		t.Fatalf("missing %q option in openNowMenu", T("lyr_now_playing"))
	}
	nowItem.do()
	if a.cfg.UI.NowPlayingLyrics {
		t.Fatal("expected NowPlayingLyrics false after toggling in now menu")
	}
}

// TestNowPlayingYKeyQuickJump verifies Y (ActPause) toggles Full Lyrics on NowPlaying
// instead of sending toggle, while keeping toggle behavior on other screens.
func TestNowPlayingYKeyQuickJump(t *testing.T) {
	var sent []string
	sendHook = func(cmd string, args []string) { sent = append(sent, cmd) }
	defer func() { sendHook = nil }()

	a := &App{
		offline:     true,
		w:           320,
		h:           240,
		screen:      NowPlaying,
		playingSong: Song{Path: "/music/song.mp3", Title: "Song"},
	}
	a.input.inject(ActPause)
	a.updateMain()
	if !a.lyricsOn {
		t.Fatal("expected Y (ActPause) on NowPlaying to open Full Lyrics")
	}
	if len(sent) != 0 {
		t.Fatalf("expected no toggle command sent when Y opens Full Lyrics, got %v", sent)
	}

	a.input = Input{}
	a.input.inject(ActPause)
	a.updateMain()
	if a.lyricsOn {
		t.Fatal("expected second Y (ActPause) on NowPlaying to close Full Lyrics")
	}
	if len(sent) != 0 {
		t.Fatalf("expected no toggle command sent when Y closes Full Lyrics, got %v", sent)
	}

	// Outside NowPlaying, ActPause still toggles playback.
	a.screen = LibraryScreen
	a.input = Input{}
	a.input.inject(ActPause)
	a.updateMain()
	if len(sent) != 1 || sent[0] != "toggle" {
		t.Fatalf("expected ActPause on LibraryScreen to send toggle, got %v", sent)
	}
}

// TestMiniLyricsFetchCondition verifies background lyrics fetching runs on NowPlaying
// when NowPlayingLyrics is enabled even if Full Lyrics view is closed.
func TestMiniLyricsFetchCondition(t *testing.T) {
	a := &App{screen: NowPlaying, playingSong: Song{Path: "/music/song.mp3"}}
	if a.wantLyrics() {
		t.Fatal("expected wantLyrics() == false when both lyricsOn and NowPlayingLyrics are false")
	}
	a.cfg.UI.NowPlayingLyrics = true
	if !a.wantLyrics() {
		t.Fatal("expected wantLyrics() == true on NowPlaying when NowPlayingLyrics is true")
	}
	a.screen = LibraryScreen
	if a.wantLyrics() {
		t.Fatal("expected wantLyrics() == false outside NowPlaying when lyricsOn is false")
	}
	a.lyricsOn = true
	if !a.wantLyrics() {
		t.Fatal("expected wantLyrics() == true whenever lyricsOn is true")
	}
}

// TestDrawNowPlayingBottomMiniLyrics exercises rendering of mini lyrics inside the fixed
// Now Playing visualizer area across synced, unsynced, empty, and instrumental states.
func TestDrawNowPlayingBottomMiniLyrics(t *testing.T) {
	if err := SetFont("", ""); err != nil {
		t.Fatal(err)
	}
	SetLanguage("en")
	dst := ebiten.NewImage(320, 240)
	a := &App{
		w:           320,
		h:           240,
		screen:      NowPlaying,
		isPlaying:   true,
		duration:    180,
		shownPos:    15,
		playingSong: Song{Path: "/music/song.mp3", Title: "Song"},
	}
	a.cfg.UI.NowPlayingLyrics = true

	// Synced lyrics with Viz off, led, and wave, including a long line that must wrap.
	a.lyr = lyricsView{
		path: "/music/song.mp3",
		opt: lyrics.Option{
			ID: "synced",
			Lyrics: lyrics.Lyrics{
				Synced: true,
				Lines: []lyrics.Line{
					{At: 5 * time.Second, Text: "Previous line"},
					{At: 10 * time.Second, Text: "Current singing line that is intentionally long enough to wrap onto multiple lines inside the mini lyrics box"},
					{At: 20 * time.Second, Text: "Next upcoming line"},
				},
			},
		},
	}
	for _, viz := range []string{"off", "led", "wave"} {
		a.cfg.UI.NowPlayingViz = viz
		a.drawNowPlayingBottom(dst, 10, 149, 300, 54)
	}
	if len(a.lyr.miniRows) < 4 || len(a.lyr.miniRows[2]) < 2 {
		t.Fatalf("expected long synced line (index 2 after leading break) to wrap into >= 2 rows, got %q", a.lyr.miniRows)
	}

	// Verify spring scroll advances toward the active line across frames.
	a.shownPos = 21
	a.t = 1.0 / 60.0
	prevScroll := a.lyr.miniScroll
	a.drawNowPlayingBottom(dst, 10, 149, 300, 54)
	if a.lyr.miniScroll == prevScroll {
		t.Fatalf("expected miniScroll to move smoothly toward next line, stayed at %v", a.lyr.miniScroll)
	}

	// Unsynced lyrics.
	a.lyr.opt = lyrics.Option{
		ID: "plain",
		Lyrics: lyrics.Lyrics{
			Synced: false,
			Lines: []lyrics.Line{
				{Text: "Line one"},
				{Text: "Line two"},
				{Text: "Line three"},
			},
		},
	}
	a.cfg.UI.NowPlayingViz = "off"
	a.drawNowPlayingBottom(dst, 10, 149, 300, 54)

	// Instrumental and empty lyrics with Viz off and Viz on.
	for _, ly := range []lyrics.Lyrics{{Instrumental: true}, {}} {
		a.lyr.opt = lyrics.Option{ID: "none", Lyrics: ly}
		a.cfg.UI.NowPlayingViz = "off"
		a.drawNowPlayingBottom(dst, 10, 149, 300, 54)
		a.cfg.UI.NowPlayingViz = "led"
		a.drawNowPlayingBottom(dst, 10, 149, 300, 54)
	}
}

// TestMiniLyricsNoTopClip verifies that inside the fixed 54px mini lyrics box
// all three visible lines (previous, current, next) have enough headroom so
// uppercase glyphs and Vietnamese diacritics on the top line are not clipped.
func TestMiniLyricsNoTopClip(t *testing.T) {
	if err := SetFont("", ""); err != nil {
		t.Fatal(err)
	}
	dst := ebiten.NewImage(320, 240)
	a := &App{
		w:           320,
		h:           240,
		screen:      NowPlaying,
		isPlaying:   true,
		duration:    180,
		shownPos:    11,
		playingSong: Song{Path: "/music/vi.mp3", Title: "Vi"},
	}
	a.cfg.UI.NowPlayingLyrics = true
	a.cfg.UI.NowPlayingViz = "off"
	a.lyr = lyricsView{
		path: "/music/vi.mp3",
		opt: lyrics.Option{
			ID: "vi-synced",
			Lyrics: lyrics.Lyrics{
				Synced: true,
				Lines: []lyrics.Line{
					{At: 5 * time.Second, Text: "Ấn tượng đêm nay"},
					{At: 10 * time.Second, Text: "Đêm nay trăng sáng quá"},
					{At: 20 * time.Second, Text: "Ngày mai sẽ khác"},
				},
			},
		},
	}
	const boxH = 54
	a.drawNowPlayingBottom(dst, 10, 149, 300, boxH)

	rowH, gap, ryOff := lyricsMetrics(true)
	step := rowH + gap
	scroll := int(math.Round(a.lyr.miniScroll))
	prevRy := 1*step - scroll + ryOff
	curRy := 2*step - scroll + ryOff
	nextRy := 3*step - scroll + ryOff

	if prevRy < 5 {
		t.Fatalf("top mini lyrics row ry=%d is too close to y0 (needs >=5px headroom for uppercase/diacritics)", prevRy)
	}
	if curRy <= prevRy || nextRy <= curRy {
		t.Fatalf("unexpected row order: prevRy=%d curRy=%d nextRy=%d", prevRy, curRy, nextRy)
	}
	if nextRy+bigCap+3 > boxH {
		t.Fatalf("bottom mini lyrics row exceeds boxH=%d: nextRy=%d bigCap=%d", boxH, nextRy, bigCap)
	}

	fullRowH, fullGap, fullOff := lyricsMetrics(false)
	if fullRowH != lyrRowH || fullGap != lyrGap || fullOff != -2 {
		t.Fatalf("full lyrics metrics changed unexpectedly: (%d, %d, %d)", fullRowH, fullGap, fullOff)
	}
}

// TestKaraokeLyricsMenu checks that openLyricsMenu provides the Karaoke toggle
// and that activating it cycles through off -> light -> full -> off.
func TestKaraokeLyricsMenu(t *testing.T) {
	var sentCmds []string
	var sentArgs [][]string
	sendHook = func(cmd string, args []string) {
		sentCmds = append(sentCmds, cmd)
		sentArgs = append(sentArgs, args)
	}
	defer func() { sendHook = nil }()

	a := &App{offline: true, playingSong: Song{Path: "/music/test.mp3", Title: "Test"}}

	a.openLyricsMenu()
	if a.menu == nil {
		t.Fatal("expected lyrics menu to open")
	}
	var karaokeItem *menuItem
	for i := range a.menu.items {
		if a.menu.items[i].label == T("set_karaoke") {
			karaokeItem = &a.menu.items[i]
			break
		}
	}
	if karaokeItem == nil {
		t.Fatalf("missing %q option in openLyricsMenu", T("set_karaoke"))
	}

	// Initially Karaoke is "" ("off")
	if karaokeItem.active() {
		t.Errorf("expected karaoke inactive initially")
	}
	if v := karaokeItem.value(); v != T("off") {
		t.Errorf("expected value %q, got %q", T("off"), v)
	}

	// First cycle: off -> light
	karaokeItem.do()
	if !karaokeItem.active() {
		t.Errorf("expected karaoke active after toggle to light")
	}
	if a.status.DSP.Karaoke != "light" {
		t.Errorf("expected DSP.Karaoke=light, got %q", a.status.DSP.Karaoke)
	}
	if v := karaokeItem.value(); v != T("karaoke_light") {
		t.Errorf("expected value %q, got %q", T("karaoke_light"), v)
	}

	// Second cycle: light -> full
	karaokeItem.do()
	if !karaokeItem.active() {
		t.Errorf("expected karaoke active after toggle to full")
	}
	if a.status.DSP.Karaoke != "full" {
		t.Errorf("expected DSP.Karaoke=full, got %q", a.status.DSP.Karaoke)
	}
	if v := karaokeItem.value(); v != T("karaoke_full") {
		t.Errorf("expected value %q, got %q", T("karaoke_full"), v)
	}

	// Third cycle: full -> off
	karaokeItem.do()
	if karaokeItem.active() {
		t.Errorf("expected karaoke inactive after cycling back to off")
	}
	if a.status.DSP.Karaoke != "" {
		t.Errorf("expected DSP.Karaoke='', got %q", a.status.DSP.Karaoke)
	}

	if len(sentCmds) != 3 || sentCmds[0] != "karaoke" || sentArgs[0][0] != "light" || sentArgs[1][0] != "full" || sentArgs[2][0] != "off" {
		t.Fatalf("unexpected commands sent: %v, args: %v", sentCmds, sentArgs)
	}
}
