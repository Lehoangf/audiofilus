package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/juanvillacortac/audiofilus/internal/config"
	"github.com/juanvillacortac/audiofilus/internal/daemon"
)

// TestUpNextModeCycleAndDefault verifies default Up Next mode ("end") and cycling
// through "end" -> "always" -> "off" -> "end".
func TestUpNextModeCycleAndDefault(t *testing.T) {
	SetLanguage("en")
	a := &App{offline: true}
	if got := a.upNextMode(); got != "end" {
		t.Fatalf("expected default upNextMode == %q, got %q", "end", got)
	}
	if a.upNextModeLabel() == "" {
		t.Fatal("expected non-empty label for end mode")
	}

	a.cycleUpNextMode()
	if got := a.upNextMode(); got != "always" {
		t.Fatalf("expected %q after first cycle, got %q", "always", got)
	}

	a.cycleUpNextMode()
	if got := a.upNextMode(); got != "off" {
		t.Fatalf("expected %q after second cycle, got %q", "off", got)
	}

	a.cycleUpNextMode()
	if got := a.upNextMode(); got != "end" {
		t.Fatalf("expected %q after third cycle, got %q", "end", got)
	}
}

// TestUpNextMenuOption checks that Now Playing's More menu exposes the Up Next
// toggle option and updates the config mode.
func TestUpNextMenuOption(t *testing.T) {
	SetLanguage("en")
	a := &App{offline: true, playingSong: Song{Path: "/music/a.mp3", Title: "A"}}
	a.openNowMenu()
	if a.menu == nil {
		t.Fatal("expected now playing menu to open")
	}
	var item *menuItem
	for i := range a.menu.items {
		if a.menu.items[i].label == T("set_up_next") {
			item = &a.menu.items[i]
			break
		}
	}
	if item == nil {
		t.Fatalf("missing %q option in openNowMenu", T("set_up_next"))
	}
	item.do()
	if got := a.upNextMode(); got != "always" {
		t.Fatalf("expected mode %q after clicking menu item, got %q", "always", got)
	}
}

// TestUpNextTrackResolution verifies next-track selection across queue ends,
// repeat modes, stop-after, radio, and Spotify metadata lookup.
func TestUpNextTrackResolution(t *testing.T) {
	al := &Album{
		Title:  "Album",
		Artist: "Artist",
		Songs: []Song{
			{Path: "/music/1.mp3", Title: "One", Artist: "Artist", Album: "Album"},
			{Path: "/music/2.mp3", Title: "Two", Artist: "Artist", Album: "Album"},
		},
	}
	a := &App{
		offline:     true,
		playing:     al,
		playingIdx:  0,
		playingSong: al.Songs[0],
		lib: &Library{
			song:   map[string]Song{al.Songs[0].Path: al.Songs[0], al.Songs[1].Path: al.Songs[1]},
			byPath: map[string]*Album{al.Songs[0].Path: al, al.Songs[1].Path: al},
		},
	}

	// Next track in album queue.
	info, ok := a.upNextTrack()
	if !ok || info.song.Path != "/music/2.mp3" || info.song.Title != "Two" {
		t.Fatalf("expected song Two next, got %+v (ok=%v)", info, ok)
	}

	// End of queue with Repeat off.
	a.playingIdx = 1
	a.playingSong = al.Songs[1]
	a.status = daemon.FullStatus{}
	a.status.Index = 1
	a.status.Repeat = "off"
	if _, ok := a.upNextTrack(); ok {
		t.Fatal("expected no up next at end of queue with Repeat=off")
	}

	// End of queue with Repeat all wraps to index 0.
	a.status.Repeat = "all"
	info, ok = a.upNextTrack()
	if !ok || info.song.Path != "/music/1.mp3" {
		t.Fatalf("expected wrap to song One with Repeat=all, got %+v (ok=%v)", info, ok)
	}

	// Repeat one suppresses Up Next.
	a.status.Repeat = "one"
	if _, ok := a.upNextTrack(); ok {
		t.Fatal("expected no up next with Repeat=one")
	}

	// StopAfter suppresses Up Next.
	a.status.Repeat = "all"
	a.status.StopAfter = true
	if _, ok := a.upNextTrack(); ok {
		t.Fatal("expected no up next with StopAfter=true")
	}

	// Radio suppresses Up Next.
	a.status.StopAfter = false
	a.playingSong = Song{Path: "radio://uuid-1", Title: "FM"}
	if _, ok := a.upNextTrack(); ok {
		t.Fatal("expected no up next during live radio")
	}
}

// TestUpNextActiveConditions tests when Up Next activates for "off", "end",
// and "always" modes, plus screen/lyrics visibility guards.
func TestUpNextActiveConditions(t *testing.T) {
	al := &Album{
		Title: "Album",
		Songs: []Song{
			{Path: "/music/1.mp3", Title: "One"},
			{Path: "/music/2.mp3", Title: "Two"},
		},
	}
	a := &App{
		offline:     true,
		screen:      NowPlaying,
		playing:     al,
		playingIdx:  0,
		playingSong: al.Songs[0],
		duration:    100,
		shownPos:    50,
		t:           10,
	}

	// "end" mode: inactive at 50s left, active at 14s left.
	a.cfg.UI.NowPlayingUpNext = "end"
	if a.upNextActive() {
		t.Fatal("expected upNextActive=false with 50s remaining in end mode")
	}
	a.shownPos = 86
	if !a.upNextActive() {
		t.Fatal("expected upNextActive=true with 14s remaining in end mode")
	}

	// Hidden when Full Lyrics is open or on another screen.
	a.lyricsOn = true
	if a.upNextActive() {
		t.Fatal("expected upNextActive=false when lyricsOn=true")
	}
	a.lyricsOn = false
	a.screen = LibraryScreen
	if a.upNextActive() {
		t.Fatal("expected upNextActive=false outside NowPlaying")
	}
	a.screen = NowPlaying

	// "off" mode: always false.
	a.cfg.UI.NowPlayingUpNext = "off"
	if a.upNextActive() {
		t.Fatal("expected upNextActive=false in off mode")
	}

	// "always" mode: waits for juiceTrack entry animation, then activates.
	a.cfg.UI.NowPlayingUpNext = "always"
	a.shownPos = 5
	a.juice.trackAt = a.t - 0.1
	if a.upNextActive() {
		t.Fatal("expected upNextActive=false while track entry animation is in progress")
	}
	a.juice.trackAt = a.t - juiceTrackLen - 0.05
	if !a.upNextActive() {
		t.Fatal("expected upNextActive=true once track entry animation settled in always mode")
	}
}

// TestUpNextLayoutAndPromote verifies vinyl retraction, cover shrink, Up Next
// cover placement without colliding with right text column, and smooth render.
func TestUpNextLayoutAndPromote(t *testing.T) {
	if err := SetFont("", ""); err != nil {
		t.Fatal(err)
	}
	if err := LoadShaders(); err != nil {
		t.Fatal(err)
	}
	SetLanguage("en")

	baseSlide := 108 * 34 / 100
	closed := computeUpNextLayout(14, 30, 108, baseSlide, 0, true)
	if closed.mainSize != 108 || closed.mainX != 14 || closed.mainY != 30 || closed.slide != baseSlide || closed.nextSize != 0 {
		t.Fatalf("unexpected closed layout: %+v", closed)
	}

	opened := computeUpNextLayout(14, 30, 108, baseSlide, 1, true)
	if opened.slide != 0 {
		t.Fatalf("expected vinyl slide=0 when Up Next fully open, got %d", opened.slide)
	}
	if opened.mainSize >= 108 || opened.mainSize < 64 {
		t.Fatalf("expected shrunken main cover around ~72px, got %d", opened.mainSize)
	}
	if opened.nextSize <= 0 {
		t.Fatalf("expected visible Up Next cover when open, got %+v", opened)
	}
	tx := 14 + 108 + baseSlide + (108/2-3)/2 + 12 // right text column in drawNowPlaying
	if opened.nextX+opened.nextSize >= tx {
		t.Fatalf("Up Next cover (%d..%d) overlaps right text column tx=%d", opened.nextX, opened.nextX+opened.nextSize, tx)
	}

	// Smoke-test rendering NowPlaying with Up Next active and promoting.
	dst := ebiten.NewImage(320, 240)
	al := &Album{
		Title:  "Album",
		Artist: "Artist",
		Songs: []Song{
			{Path: "/music/1.mp3", Title: "Current Song", Artist: "Artist", Album: "Album"},
			{Path: "/music/2.mp3", Title: "Next Song", Artist: "Artist", Album: "Album"},
		},
	}
	a := &App{
		offline:     true,
		w:           320,
		h:           240,
		screen:      NowPlaying,
		isPlaying:   true,
		playing:     al,
		playingIdx:  0,
		playingSong: al.Songs[0],
		duration:    100,
		shownPos:    90,
		t:           10,
	}
	a.cfg.UI.NowPlayingUpNext = "end"
	for i := 0; i < 30; i++ {
		a.t += 1.0 / 60.0
		a.updateUpNext()
	}
	if a.upNext.p < 0.9 {
		t.Fatalf("expected upNext.p to advance toward 1, got %v", a.upNext.p)
	}
	a.drawNowPlaying(dst)
	if !a.upNext.lastVisible {
		t.Fatal("expected drawNowPlaying to record lastVisible=true when Up Next is open")
	}

	// Simulate advancing to next song and promoting from Up Next box.
	a.upNextOnTrackChange(al.Songs[1].Path, 1)
	if !a.upNext.promote {
		t.Fatal("expected promote=true when advancing to the Up Next song")
	}
	a.playingIdx = 1
	a.playingSong = al.Songs[1]
	a.shownPos = 0.1
	a.t += 0.15
	a.drawNowPlaying(dst)
	_ = time.Second
}

// TestUpNextSpotifyAndNarrowLayout verifies Spotify Up Next metadata & cached
// cover resolution and ensures narrow (1:1 / vertical) layout stays inside bounds.
func TestUpNextSpotifyAndNarrowLayout(t *testing.T) {
	tmp := t.TempDir()
	spID := "4uLU6hMCjMI75M1A2tKUQC"
	coverFile := filepath.Join(tmp, "cache", "spotify", "songs", spID+".jpg")
	if err := os.MkdirAll(filepath.Dir(coverFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverFile, []byte("fakejpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &App{
		offline: true,
		paths:   config.Paths{Data: tmp},
		playing: &Album{
			Title: "Now",
			Songs: []Song{
				{Path: "spotify:track:cur", Title: "Cur"},
				{Path: "spotify:track:" + spID, Title: "Spotify Track"},
			},
		},
		playingIdx:  0,
		playingSong: Song{Path: "spotify:track:cur", Title: "Cur"},
	}
	a.spot.metaCache = map[string]spotifyTrack{
		spID: {ID: spID, Title: "Never Gonna Give You Up", Artist: "Rick Astley", Album: "Whenever You Need Somebody"},
	}

	info, ok := a.upNextTrack()
	if !ok {
		t.Fatal("expected Spotify Up Next track to resolve")
	}
	if info.song.Title != "Never Gonna Give You Up" || info.song.Artist != "Rick Astley" {
		t.Fatalf("unexpected Spotify Up Next metadata: %+v", info.song)
	}
	if info.album == nil || info.album.coverPath != coverFile {
		t.Fatalf("expected cached Spotify cover path %q, got %+v", coverFile, info.album)
	}

	// Narrow 240x240 layout (wide=false): main cover + Up Next cover stay within [margin, 240-margin].
	narrow := computeUpNextLayout((240-92)/2-16, topH+8, 92, 92*34/100, 1, false)
	if narrow.mainX < margin || narrow.nextX+narrow.nextSize > 240-margin {
		t.Fatalf("narrow Up Next layout exceeds 240px width: %+v", narrow)
	}
}
