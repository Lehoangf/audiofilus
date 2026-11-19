package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanvillacortac/audiofilus/internal/config"
	"github.com/juanvillacortac/audiofilus/internal/remote"
)

func TestSpotifyLocalization(t *testing.T) {
	for _, lang := range []string{"es", "en", "pt", "fr", "it", "de", "pl", "tr", "ru", "id", "ja", "zh", "ko"} {
		SetLanguage(lang)
		if T("spotify") == "" || T("spotify_liked") == "" {
			t.Errorf("missing spotify translations for lang %s", lang)
		}
	}
}

func TestSpotifyScreenEnum(t *testing.T) {
	if Spotify.String() != "spotify" {
		t.Errorf("expected Spotify.String() == 'spotify', got %q", Spotify.String())
	}
}

func TestSpotifyStateTabSwitching(t *testing.T) {
	s := &spotifyState{}
	if s.tab != spSearch {
		t.Errorf("expected initial tab spSearch (0), got %d", s.tab)
	}
	s.tab = spRecent
	if s.tab != spRecent {
		t.Errorf("expected spRecent, got %d", s.tab)
	}
	s.tab = spLiked
	if s.tab != spLiked {
		t.Errorf("expected spLiked, got %d", s.tab)
	}
	s.tab = spPlaylists
	if s.tab != spPlaylists {
		t.Errorf("expected spPlaylists, got %d", s.tab)
	}
	s.tab = spAlbums
	if s.tab != spAlbums {
		t.Errorf("expected spAlbums, got %d", s.tab)
	}
}

func TestIsSpotifyTrackDownloadedMatchingAndCacheInvalidation(t *testing.T) {
	musicDir := t.TempDir()
	localFile := filepath.Join(musicDir, "Vũ", "Một Vạn Năm", "Những Lời Hứa Bỏ Quên.flac")
	if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localFile, []byte("flac"), 0o644); err != nil {
		t.Fatal(err)
	}

	remotePath := remote.SongPath("navidrome", "rem123")
	lib := buildLibrary([]entry{
		{Path: remotePath, Title: "Remote Only", Artist: "Remote Artist"},
	}, nil)

	a := &App{
		cfg: config.Config{MusicDirs: []string{musicDir}},
	}
	a.setLocal(lib)

	// Remote songs must never count as locally downloaded.
	if _, ok := a.isSpotifyTrackDownloaded(spotifyTrack{ID: "spRem", Title: "Remote Only", Artist: "Remote Artist"}); ok {
		t.Fatal("remote server song should not be treated as locally downloaded")
	}

	// Local FLAC file with Vietnamese diacritics and feat. mismatch should be found.
	tr := spotifyTrack{ID: "spLocal1", Title: "Những Lời Hứa Bỏ Quên (feat. Dear Jane)", Artist: "Vũ., Dear Jane", Album: "Bảo Tàng Của Nuối Tiếc"}
	got, ok := a.isSpotifyTrackDownloaded(tr)
	if !ok || got != localFile {
		t.Fatalf("isSpotifyTrackDownloaded = (%q, %v), want %q", got, ok, localFile)
	}

	// A Remix must NOT reuse the original version file.
	if _, ok := a.isSpotifyTrackDownloaded(spotifyTrack{ID: "spRemix1", Title: "Những Lời Hứa Bỏ Quên (Remix)", Artist: "Vũ."}); ok {
		t.Fatal("remix track should not match studio track on disk")
	}

	// After setLocal updates the library index with a newly tagged track, cache invalidates immediately.
	taggedPath := filepath.Join(musicDir, "Misc", "abc.mp3")
	_ = os.MkdirAll(filepath.Dir(taggedPath), 0o755)
	_ = os.WriteFile(taggedPath, []byte("mp3"), 0o644)

	trNew := spotifyTrack{ID: "spNew1", Title: "Tagged Track", Artist: "Tag Artist"}
	if _, ok := a.isSpotifyTrackDownloaded(trNew); ok {
		t.Fatal("expected not downloaded before library scan refresh")
	}
	lib2 := buildLibrary([]entry{
		{Path: taggedPath, Title: "Tagged Track", Artist: "Tag Artist"},
	}, nil)
	a.setLocal(lib2)
	if p, ok := a.isSpotifyTrackDownloaded(trNew); !ok || p != taggedPath {
		t.Fatalf("after setLocal invalidation got (%q, %v), want %q", p, ok, taggedPath)
	}

	// .library.json idPaths match without matching title/artist tags.
	idFile := filepath.Join(musicDir, "Custom", "mapped.m4a")
	_ = os.MkdirAll(filepath.Dir(idFile), 0o755)
	_ = os.WriteFile(idFile, []byte("m4a"), 0o644)
	rel, _ := filepath.Rel(musicDir, idFile)
	dotLib, _ := json.Marshal(map[string]string{"spotify:spIDOnly": rel})
	_ = os.WriteFile(filepath.Join(musicDir, ".library.json"), dotLib, 0o644)
	a.loadHaveIDs()
	if p, ok := a.isSpotifyTrackDownloaded(spotifyTrack{ID: "spIDOnly", Title: "Other", Artist: "Other"}); !ok || p != idFile {
		t.Fatalf("idPaths match got (%q, %v), want %q", p, ok, idFile)
	}
}

func TestDownloadSpotifyTracksDeduplicatesAndSkipsLocal(t *testing.T) {
	musicDir := t.TempDir()
	existing := filepath.Join(musicDir, "Artist One", "Singles", "Already Here.m4a")
	_ = os.MkdirAll(filepath.Dir(existing), 0o755)
	_ = os.WriteFile(existing, []byte("m4a"), 0o644)

	a := &App{
		cfg: config.Config{MusicDirs: []string{musicDir}},
		yt:  ytState{queue: make(chan *ytJob, 32)},
	}

	tracks := []spotifyTrack{
		{ID: "idExisting", Title: "Already Here", Artist: "Artist One"},
		{ID: "idMiss1", Title: "Missing Song", Artist: "Artist Two"},
		{ID: "idMiss1", Title: "Missing Song", Artist: "Artist Two"},                        // exact ID duplicate
		{ID: "idMiss1Album", Title: "Missing Song (Remastered 2024)", Artist: "Artist Two"}, // same song & artist
	}
	a.downloadSpotifyTracks("Test Playlist", tracks)

	jobs := a.ytJobsSnapshot()
	if len(jobs) != 1 {
		t.Fatalf("queued jobs = %d, want 1", len(jobs))
	}
	if jobs[0].track.ID != "spotify:idMiss1" {
		t.Errorf("queued job ID = %q, want spotify:idMiss1", jobs[0].track.ID)
	}
	if !a.isSpotifyTrackInFlight(spotifyTrack{ID: "idMiss1Album", Title: "Missing Song", Artist: "Artist Two"}) {
		t.Error("expected album edition of queued song to report in-flight")
	}
}

// Verify all Spotify playback entry points queue the full surrounding track list at the chosen index
// rather than loading a single track (which breaks next/prev skipping).
func TestSpotifyPlaybackQueuesFullTrackList(t *testing.T) {
	tracks := []spotifyTrack{
		{ID: "t1", Title: "One", Artist: "A"},
		{ID: "t2", Title: "Two", Artist: "B"},
		{ID: "t3", Title: "Three", Artist: "C"},
	}

	var gotCmd string
	var gotArgs []string
	sendHook = func(cmd string, args []string) {
		gotCmd = cmd
		gotArgs = append([]string(nil), args...)
	}
	defer func() { sendHook = nil }()

	assertLoad := func(t *testing.T, wantStart string, wantIDs ...string) {
		t.Helper()
		if gotCmd == "spotify" {
			if len(gotArgs) < 4 || gotArgs[0] != "play_collection" {
				t.Fatalf("expected play_collection, got %v", gotArgs)
			}
			if gotArgs[3] != wantStart {
				t.Errorf("play_collection start = %q, want %q (args=%v)", gotArgs[3], wantStart, gotArgs)
			}
			trackArgs := gotArgs[4:]
			if len(trackArgs) != len(wantIDs) {
				t.Fatalf("play_collection track args len = %d (%v), want %d", len(trackArgs), trackArgs, len(wantIDs))
			}
			for i, id := range wantIDs {
				if trackArgs[i] != id {
					t.Errorf("play_collection track[%d] = %q, want %q", i, trackArgs[i], id)
				}
			}
			return
		}
		if gotCmd != "load" {
			t.Fatalf("expected load command, got %q (%v)", gotCmd, gotArgs)
		}
		if len(gotArgs) != 1+len(wantIDs) {
			t.Fatalf("load args len = %d (%v), want %d", len(gotArgs), gotArgs, 1+len(wantIDs))
		}
		if gotArgs[0] != wantStart {
			t.Errorf("load start = %q, want %q (args=%v)", gotArgs[0], wantStart, gotArgs)
		}
		for i, id := range wantIDs {
			want := "spotify://track/" + id
			if gotArgs[i+1] != want {
				t.Errorf("load track[%d] = %q, want %q", i, gotArgs[i+1], want)
			}
		}
	}

	t.Run("openPl", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		pl := spotifyPlaylist{ID: "pl1", Name: "Mix", Tracks: tracks}
		a := &App{offline: true, screen: Spotify}
		a.spot.loggedIn = true
		a.spot.openPl = &pl
		a.spot.openPlSel = 1
		a.input.inject(ActConfirm)
		a.updateSpotify()
		assertLoad(t, "1", "t1", "t2", "t3")
		if a.screen != NowPlaying {
			t.Errorf("screen = %v, want NowPlaying", a.screen)
		}
	})

	t.Run("spSearch", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		a := &App{offline: true, screen: Spotify}
		a.spot.loggedIn = true
		a.spot.tab = spSearch
		a.spot.searchRes = tracks
		a.spot.sel[spSearch] = 2 // index 0 is the search box
		a.input.inject(ActConfirm)
		a.updateSpotify()
		assertLoad(t, "1", "t1", "t2", "t3")
	})

	t.Run("spRecent_preserves_order_before_reordering", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		a := &App{offline: true, screen: Spotify}
		a.spot.loggedIn = true
		a.spot.tab = spRecent
		a.spot.recent = append([]spotifyTrack(nil), tracks...)
		a.spot.sel[spRecent] = 1
		a.input.inject(ActConfirm)
		a.updateSpotify()
		assertLoad(t, "1", "t1", "t2", "t3")
		if len(a.spot.recent) != 3 || a.spot.recent[0].ID != "t2" {
			t.Errorf("expected t2 moved to top of recent after playback, got %+v", a.spot.recent)
		}
	})

	t.Run("spLiked", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		a := &App{offline: true, screen: Spotify}
		a.spot.loggedIn = true
		a.spot.tab = spLiked
		a.spot.liked = tracks
		a.spot.sel[spLiked] = 2
		a.input.inject(ActConfirm)
		a.updateSpotify()
		assertLoad(t, "2", "t1", "t2", "t3")
	})

	t.Run("spotifyTrackMenu_play", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		a := &App{offline: true, screen: Spotify}
		a.spot.loggedIn = true
		a.spot.tab = spLiked
		a.spot.liked = tracks
		a.spot.sel[spLiked] = 1
		a.input.inject(ActMenu)
		a.updateSpotify()
		if a.menu == nil || len(a.menu.items) == 0 {
			t.Fatal("expected context menu for Spotify track")
		}
		a.menu.items[0].do()
		assertLoad(t, "1", "t1", "t2", "t3")
	})

	t.Run("universal_search_resSpotify", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		a := &App{offline: true, screen: LibraryScreen}
		a.search.results = []searchResult{
			{kind: resSong, song: Song{Path: "/music/local.flac", Title: "Local"}},
			{kind: resSpotify, spot: tracks[0]},
			{kind: resSpotify, spot: tracks[1]},
			{kind: resSpotify, spot: tracks[2]},
		}
		a.openResult(a.search.results[2])
		assertLoad(t, "1", "t1", "t2", "t3")
	})

	t.Run("spAlbums_openPl", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		al := spotifyPlaylist{ID: "al1", Name: "Album One", Tracks: tracks}
		a := &App{offline: true, screen: Spotify}
		a.spot.loggedIn = true
		a.spot.tab = spAlbums
		a.spot.albums = []spotifyPlaylist{al}
		a.spot.sel[spAlbums] = 0
		a.input.inject(ActConfirm)
		a.updateSpotify()
		if a.spot.openPl == nil || a.spot.openPl.ID != "al1" {
			t.Fatalf("expected album al1 to open, got %+v", a.spot.openPl)
		}
		a.spot.openPlSel = 2
		a.input = Input{}
		a.input.inject(ActConfirm)
		a.updateSpotify()
		assertLoad(t, "2", "t1", "t2", "t3")
	})

	t.Run("spotifyPlaylistMenu_play", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		pl := spotifyPlaylist{ID: "plm", Name: "Menu Mix", Tracks: tracks}
		a := &App{offline: true, screen: Spotify}
		a.spot.loggedIn = true
		a.spot.tab = spPlaylists
		a.spot.playlists = []spotifyPlaylist{pl}
		a.spot.sel[spPlaylists] = 0
		a.input.inject(ActMenu)
		a.updateSpotify()
		if a.menu == nil || len(a.menu.items) == 0 {
			t.Fatal("expected context menu for Spotify playlist")
		}
		a.menu.items[0].do()
		assertLoad(t, "0", "t1", "t2", "t3")
	})

	t.Run("skips_empty_id_and_adjusts_start", func(t *testing.T) {
		gotCmd, gotArgs = "", nil
		withEmpty := []spotifyTrack{
			{ID: "", Title: "Local/Unavailable"},
			tracks[0],
			tracks[1],
		}
		a := &App{offline: true, screen: Spotify}
		a.playSpotifyTracks(withEmpty, 2)
		assertLoad(t, "1", "t1", "t2")
	})
}

// Verify global playback shortcuts (prev/next/pause) work while viewing the Spotify screen.
func TestSpotifyGlobalPrevNextKeysWork(t *testing.T) {
	var sent []string
	sendHook = func(cmd string, args []string) { sent = append(sent, cmd) }
	defer func() { sendHook = nil }()

	a := &App{offline: true, w: 320, h: 240, screen: Spotify}
	a.spot.loggedIn = true
	for _, act := range []Action{ActPrev, ActNext, ActPause} {
		a.input = Input{}
		a.input.inject(act)
		a.updateMain()
	}
	if len(sent) != 3 || sent[0] != "prev" || sent[1] != "next" || sent[2] != "toggle" {
		t.Fatalf("global music keys on Spotify screen sent %v, want [prev next toggle]", sent)
	}
}

// Verify playlist download summary and UI track lookup avoid synchronous disk scans on Draw and
// update accurately in the background.
func TestSpotifyPlaylistDownloadCountNonBlockingAndCached(t *testing.T) {
	musicDir := t.TempDir()
	f1 := filepath.Join(musicDir, "Artist One", "Album", "Song One.flac")
	if err := os.MkdirAll(filepath.Dir(f1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f1, []byte("flac"), 0o644); err != nil {
		t.Fatal(err)
	}

	var trs []spotifyTrack
	trs = append(trs, spotifyTrack{ID: "dl1", Title: "Song One", Artist: "Artist One", Album: "Album"})
	for i := 0; i < 20; i++ {
		trs = append(trs, spotifyTrack{
			ID:     "miss_" + string(rune('a'+i)),
			Title:  "Missing Track " + string(rune('A'+i)),
			Artist: "Missing Artist",
			Album:  "Missing Album",
		})
	}

	lib1 := buildLibrary([]entry{{Path: f1, Title: "Song One", Artist: "Artist One"}}, nil)
	a := &App{
		cfg: config.Config{MusicDirs: []string{musicDir}},
	}
	a.setLocal(lib1)

	pl := spotifyPlaylist{ID: "pl_perf", Name: "Perf Mix", SnapshotID: "snap1", Tracks: trs}

	// Cold cache: UI helper must return immediately without blocking on disk scans.
	if p, ok := a.cachedSpotifyTrackDownloaded(trs[0]); ok || p != "" {
		t.Fatalf("cold cachedSpotifyTrackDownloaded = (%q, %v), want empty/false before background check", p, ok)
	}
	down, complete := a.playlistDownloadCount(pl)
	if complete {
		t.Fatalf("expected cold playlistDownloadCount to report incomplete while background check runs (down=%d)", down)
	}

	// After background worker finishes, both track and playlist summary report the downloaded track.
	a.waitSpotifyDlChecks()
	if p, ok := a.cachedSpotifyTrackDownloaded(trs[0]); !ok || p != f1 {
		t.Fatalf("warm cachedSpotifyTrackDownloaded = (%q, %v), want (%q, true)", p, ok, f1)
	}
	down, complete = a.playlistDownloadCount(pl)
	if !complete || down != 1 {
		t.Fatalf("warm playlistDownloadCount = (%d, %v), want (1, true)", down, complete)
	}

	// Calling again with unchanged gen/rev hits memoized plDown entry directly.
	a.spot.dlMu.Lock()
	a.spot.dlCache["miss_a"] = dlCacheEntry{
		path:  "/fake/override.flac",
		ok:    true,
		gen:   a.spot.dlGen.Load(),
		expAt: a.spot.dlCache["miss_a"].expAt,
	}
	a.spot.dlMu.Unlock()
	downMemo, completeMemo := a.playlistDownloadCount(pl)
	if !completeMemo || downMemo != 1 {
		t.Fatalf("expected memoized playlistDownloadCount to return cached count 1 without re-scanning tracks, got (%d, %v)", downMemo, completeMemo)
	}

	// Invalidating via setLocal triggers recalculation for the new generation.
	f2 := filepath.Join(musicDir, "Missing Artist", "Missing Album", "Missing Track A.flac")
	_ = os.MkdirAll(filepath.Dir(f2), 0o755)
	_ = os.WriteFile(f2, []byte("flac"), 0o644)
	lib2 := buildLibrary([]entry{
		{Path: f1, Title: "Song One", Artist: "Artist One"},
		{Path: f2, Title: "Missing Track A", Artist: "Missing Artist"},
	}, nil)
	a.setLocal(lib2)

	_, completeAfter := a.playlistDownloadCount(pl)
	if completeAfter {
		t.Fatal("expected playlistDownloadCount to refresh after setLocal invalidation")
	}
	a.waitSpotifyDlChecks()
	downAfter, completeAfter := a.playlistDownloadCount(pl)
	if !completeAfter || downAfter != 2 {
		t.Fatalf("after setLocal playlistDownloadCount = (%d, %v), want (2, true)", downAfter, completeAfter)
	}
}

func TestPlaylistAuthoritativeTrackCountBadges(t *testing.T) {
	musicDir := t.TempDir()
	var trs []spotifyTrack
	for i := 0; i < 10; i++ {
		id := "t_" + string(rune('a'+i))
		f := filepath.Join(musicDir, "Artist", "Album", id+".flac")
		_ = os.MkdirAll(filepath.Dir(f), 0o755)
		_ = os.WriteFile(f, []byte("flac"), 0o644)
		trs = append(trs, spotifyTrack{ID: id, Title: id, Artist: "Artist"})
	}
	var entries []entry
	for _, tr := range trs {
		entries = append(entries, entry{Path: filepath.Join(musicDir, "Artist", "Album", tr.ID+".flac"), Title: tr.Title, Artist: tr.Artist})
	}
	lib := buildLibrary(entries, nil)
	a := &App{cfg: config.Config{MusicDirs: []string{musicDir}}}
	a.setLocal(lib)

	// 10 tracks cached locally and downloaded, but authoritative TrackCount on Spotify is 423.
	pl := spotifyPlaylist{ID: "pl_edm", Name: "EDM", TrackCount: 423, Tracks: trs}
	a.playlistDownloadCount(pl)
	a.waitSpotifyDlChecks()
	down, complete := a.playlistDownloadCount(pl)
	if complete {
		t.Fatalf("expected complete=false because only 10 out of 423 are downloaded, got complete=%v", complete)
	}
	if down != 10 {
		t.Fatalf("expected down=10, got %d", down)
	}
}

func TestSpotifyAlbumTracksUsesAlbumSubcommandNotPlaylistTracks(t *testing.T) {
	al := spotifyPlaylist{ID: "al_123", Name: "Album Test", IsAlbum: true}
	subCmd := spotifySubcommandForPlaylist(al)
	if subCmd != "album_tracks" {
		t.Fatalf("expected album subcommand 'album_tracks', got %q", subCmd)
	}
	pl := spotifyPlaylist{ID: "pl_123", Name: "Playlist Test", IsAlbum: false}
	if sub := spotifySubcommandForPlaylist(pl); sub != "playlist_tracks" {
		t.Fatalf("expected playlist subcommand 'playlist_tracks', got %q", sub)
	}
}

func TestSpotifyPlayCollectionSentForPlaylistLikedAndAlbum(t *testing.T) {
	var sentCmd string
	var sentArgs []string
	sendHook = func(cmd string, args []string) {
		sentCmd = cmd
		sentArgs = append([]string(nil), args...)
	}
	defer func() { sendHook = nil }()

	a := &App{offline: true, screen: Spotify}
	a.spot.loggedIn = true

	tracks := []spotifyTrack{
		{ID: "t0", Title: "Song 0"},
		{ID: "t1", Title: "Song 1"},
	}

	// 1. Playlist
	pl := spotifyPlaylist{ID: "pl_mix", Name: "Mix", Tracks: tracks}
	a.spot.openPl = &pl
	a.spot.openPlSel = 1
	a.input.inject(ActConfirm)
	a.updateSpotify()

	if sentCmd != "spotify" || len(sentArgs) < 5 || sentArgs[0] != "play_collection" || sentArgs[1] != "playlist" || sentArgs[2] != "pl_mix" || sentArgs[3] != "1" {
		t.Fatalf("play playlist got cmd=%q args=%v, want spotify play_collection playlist pl_mix 1 t0 t1", sentCmd, sentArgs)
	}

	// 2. Liked
	a.spot.openPl = nil
	a.spot.tab = spLiked
	a.spot.liked = tracks
	a.spot.sel[spLiked] = 0
	a.input.inject(ActConfirm)
	a.updateSpotify()

	if sentCmd != "spotify" || len(sentArgs) < 5 || sentArgs[0] != "play_collection" || sentArgs[1] != "liked" || sentArgs[2] != "liked" || sentArgs[3] != "0" {
		t.Fatalf("play liked got cmd=%q args=%v, want spotify play_collection liked liked 0 t0 t1", sentCmd, sentArgs)
	}

	// 3. Album
	album := spotifyPlaylist{ID: "al_rock", Name: "Rock Album", IsAlbum: true, Tracks: tracks}
	a.spot.openPl = &album
	a.spot.openPlSel = 0
	a.input.inject(ActConfirm)
	a.updateSpotify()

	if sentCmd != "spotify" || len(sentArgs) < 5 || sentArgs[0] != "play_collection" || sentArgs[1] != "album" || sentArgs[2] != "al_rock" || sentArgs[3] != "0" {
		t.Fatalf("play album got cmd=%q args=%v, want spotify play_collection album al_rock 0 t0 t1", sentCmd, sentArgs)
	}
}

func TestQueueScreenResolvesSpotifyTrackMetadata(t *testing.T) {
	a := &App{}
	a.spot.metaCache = map[string]spotifyTrack{
		"id123": {ID: "id123", Title: "Resolved Song", Artist: "Resolved Artist"},
	}
	tr, ok := a.spotifyTrackMetadata("id123")
	if !ok || tr.Title != "Resolved Song" || tr.Artist != "Resolved Artist" {
		t.Fatalf("expected resolved metadata, got %+v (ok=%v)", tr, ok)
	}
	_, okMissing := a.spotifyTrackMetadata("missing")
	if okMissing {
		t.Fatalf("expected missing track to return ok=false")
	}
}

func TestSpotifyPaginationTriggerLogic(t *testing.T) {
	a := &App{}
	// Not needed when all tracks are loaded
	plFull := spotifyPlaylist{ID: "pl1", TrackCount: 5, Tracks: make([]spotifyTrack, 5)}
	if a.shouldFetchNextSpotifyPage(&plFull, 4) {
		t.Errorf("expected shouldFetchNextSpotifyPage = false when all tracks loaded")
	}
	// Needed when close to end and more tracks exist
	plPartial := spotifyPlaylist{ID: "pl2", TrackCount: 50, Tracks: make([]spotifyTrack, 10)}
	if !a.shouldFetchNextSpotifyPage(&plPartial, 8) {
		t.Errorf("expected shouldFetchNextSpotifyPage = true when cursor is at 8/10 and total is 50")
	}
	if a.shouldFetchNextSpotifyPage(&plPartial, 2) {
		t.Errorf("expected shouldFetchNextSpotifyPage = false when cursor is far from bottom (2/10)")
	}
}

func TestSpotifyTrackMenuLikeAndAddToPlaylist(t *testing.T) {
	a := &App{}
	tr := spotifyTrack{ID: "track_xyz", Title: "Song XYZ", Artist: "Artist XYZ"}

	// 1. Initial state: not liked
	if a.isSpotifyTrackLiked(tr.ID) {
		t.Fatalf("expected track_xyz to not be liked initially")
	}

	// Menu before like has "spotify_like" label and "heart" icon
	a.spotifyTrackMenu([]spotifyTrack{tr}, 0)
	if a.menu == nil {
		t.Fatalf("expected menu to be open")
	}
	var likeItem *menuItem
	var addListItem *menuItem
	for i := range a.menu.items {
		if a.menu.items[i].icon == "heart" {
			likeItem = &a.menu.items[i]
		}
		if a.menu.items[i].icon == "list" && a.menu.items[i].label == T("t_addlist") {
			addListItem = &a.menu.items[i]
		}
	}
	if likeItem == nil || likeItem.label != T("spotify_like") {
		t.Fatalf("expected heart menu item with label %q, got %+v", T("spotify_like"), likeItem)
	}
	if addListItem == nil {
		t.Fatalf("expected add to playlist menu item")
	}

	// 2. Toggle like
	a.toggleSpotifyLike(tr)
	if !a.isSpotifyTrackLiked(tr.ID) {
		t.Fatalf("expected track_xyz to be liked after toggle")
	}

	// Menu after like has "spotify_unlike" label
	a.spotifyTrackMenu([]spotifyTrack{tr}, 0)
	likeItem = nil
	for i := range a.menu.items {
		if a.menu.items[i].icon == "heart" {
			likeItem = &a.menu.items[i]
		}
	}
	if likeItem == nil || likeItem.label != T("spotify_unlike") {
		t.Fatalf("expected heart menu item with label %q, got %+v", T("spotify_unlike"), likeItem)
	}

	// 3. Toggle unlike
	a.toggleSpotifyLike(tr)
	if a.isSpotifyTrackLiked(tr.ID) {
		t.Fatalf("expected track_xyz to not be liked after second toggle")
	}

	// 4. Test openAddToSpotifyPlaylist and addSpotifyTrackToPlaylist
	pl1 := spotifyPlaylist{ID: "pl1", Name: "Chill Mix", TrackCount: 2}
	al1 := spotifyPlaylist{ID: "al1", Name: "Rock Album", IsAlbum: true, TrackCount: 5}
	a.spot.playlists = []spotifyPlaylist{pl1, al1}

	a.openAddToSpotifyPlaylist(tr)
	if a.menu == nil {
		t.Fatalf("expected openAddToSpotifyPlaylist menu")
	}
	if len(a.menu.items) != 1 || a.menu.items[0].label != "Chill Mix" {
		t.Fatalf("expected 1 playlist item ('Chill Mix'), got %d items", len(a.menu.items))
	}

	// Add track to playlist
	a.menu.items[0].do()
	if a.spot.playlists[0].TrackCount != 3 {
		t.Fatalf("expected TrackCount to be 3, got %d", a.spot.playlists[0].TrackCount)
	}
	if len(a.spot.playlists[0].Tracks) == 0 || a.spot.playlists[0].Tracks[0].ID != tr.ID {
		t.Fatalf("expected track_xyz to be added to playlist tracks")
	}

	// 5. Test openNowMenu for a playing Spotify track
	a.playingSong = Song{Path: "spotify://track/" + tr.ID, Title: tr.Title, Artist: tr.Artist}
	a.openNowMenu()
	if a.menu == nil {
		t.Fatalf("expected openNowMenu to open")
	}
	hasHeart := false
	hasAddList := false
	for _, it := range a.menu.items {
		if it.icon == "heart" {
			hasHeart = true
		}
		if it.icon == "list" && it.label == T("t_addlist") {
			hasAddList = true
		}
	}
	if !hasHeart || !hasAddList {
		t.Fatalf("openNowMenu missing heart or addlist items for spotify track: hasHeart=%v hasAddList=%v", hasHeart, hasAddList)
	}
}

func TestSmartShuffleCycleAndLabels(t *testing.T) {
	var sentCmds [][]string
	sendHook = func(cmd string, args []string) {
		sentCmds = append(sentCmds, append([]string{cmd}, args...))
	}
	defer func() { sendHook = nil }()

	a := &App{offline: true}
	a.spot.loggedIn = true

	// Initial: Off
	if a.status.Shuffle || a.status.SpotifySmartShuffle {
		t.Fatalf("expected shuffle off initially")
	}
	if lbl := a.shuffleModeLabel(); lbl != T("off") {
		t.Fatalf("expected label %q, got %q", T("off"), lbl)
	}

	// 1st cycle: Normal Shuffle On
	a.cycleShuffle()
	if !a.status.Shuffle || a.status.SpotifySmartShuffle {
		t.Fatalf("after 1st cycle, expected Shuffle=true and Smart=false, got Shuffle=%v Smart=%v", a.status.Shuffle, a.status.SpotifySmartShuffle)
	}
	if lbl := a.shuffleModeLabel(); lbl != T("on") {
		t.Fatalf("expected label %q, got %q", T("on"), lbl)
	}

	// 2nd cycle: Smart Shuffle On
	a.cycleShuffle()
	if !a.status.Shuffle || !a.status.SpotifySmartShuffle {
		t.Fatalf("after 2nd cycle, expected Shuffle=true and Smart=true, got Shuffle=%v Smart=%v", a.status.Shuffle, a.status.SpotifySmartShuffle)
	}
	if lbl := a.shuffleModeLabel(); lbl != T("spotify_smart_shuffle") {
		t.Fatalf("expected label %q, got %q", T("spotify_smart_shuffle"), lbl)
	}

	// 3rd cycle: All Off
	a.cycleShuffle()
	if a.status.Shuffle || a.status.SpotifySmartShuffle {
		t.Fatalf("after 3rd cycle, expected Shuffle=false and Smart=false, got Shuffle=%v Smart=%v", a.status.Shuffle, a.status.SpotifySmartShuffle)
	}
	if lbl := a.shuffleModeLabel(); lbl != T("off") {
		t.Fatalf("expected label %q, got %q", T("off"), lbl)
	}
}

func TestQueueSmartTrackDetectionAndIDExtraction(t *testing.T) {
	smartURI := "spotify://track/4cOdK2wGLETKBW3PvgPWqT?smart=1"
	normalURI := "spotify://track/4cOdK2wGLETKBW3PvgPWqT"
	localPath := "/home/music/song.flac"

	if !isSmartTrack(smartURI) {
		t.Errorf("expected isSmartTrack(%q) = true", smartURI)
	}
	if isSmartTrack(normalURI) {
		t.Errorf("expected isSmartTrack(%q) = false", normalURI)
	}
	if isSmartTrack(localPath) {
		t.Errorf("expected isSmartTrack(%q) = false", localPath)
	}

	if id := extractSpotifyID(smartURI); id != "4cOdK2wGLETKBW3PvgPWqT" {
		t.Errorf("extractSpotifyID(%q) = %q, want %q", smartURI, id, "4cOdK2wGLETKBW3PvgPWqT")
	}
	if id := extractSpotifyID(normalURI); id != "4cOdK2wGLETKBW3PvgPWqT" {
		t.Errorf("extractSpotifyID(%q) = %q, want %q", normalURI, id, "4cOdK2wGLETKBW3PvgPWqT")
	}
}

func TestLibraryAndSpotifyDpadLeftSkipsLikeRight(t *testing.T) {
	// 1. Library List view (e.g. songs / playlist songs)
	a := &App{offline: true, screen: LibraryScreen, w: 320, h: 240}
	var songs []Song
	for i := 0; i < 30; i++ {
		songs = append(songs, Song{Path: fmt.Sprintf("/music/song_%02d.flac", i), Title: fmt.Sprintf("Song %d", i)})
	}
	a.stacks[tabSongs] = []*view{{kind: vSongs, songs: songs, sel: 25}}
	a.libTab = tabSongs
	a.tabFocus = false

	// ActLeft in library list skips backward by 10
	a.input = Input{}
	a.input.inject(ActLeft)
	a.updateLibrary()
	if v := a.cur(); v.sel != 15 {
		t.Fatalf("expected library sel to skip from 25 to 15, got %d", v.sel)
	}

	a.input = Input{}
	a.input.inject(ActLeft)
	a.updateLibrary()
	if v := a.cur(); v.sel != 5 {
		t.Fatalf("expected library sel to skip from 15 to 5, got %d", v.sel)
	}

	// ActLeft at 5 caps at 0 and does NOT pop view or enter tabFocus
	a.input = Input{}
	a.input.inject(ActLeft)
	a.updateLibrary()
	if v := a.cur(); v.sel != 0 || a.tabFocus {
		t.Fatalf("expected library sel=0, tabFocus=false, got sel=%d tabFocus=%v", v.sel, a.tabFocus)
	}

	// ActRight skips forward by 10
	a.input = Input{}
	a.input.inject(ActRight)
	a.updateLibrary()
	if v := a.cur(); v.sel != 10 {
		t.Fatalf("expected library sel to skip from 0 to 10, got %d", v.sel)
	}

	// ActBack handles returning to tabs
	a.input = Input{}
	a.input.inject(ActBack)
	a.updateLibrary()
	if !a.tabFocus {
		t.Fatalf("expected ActBack to set tabFocus=true")
	}

	// 2. Spotify Opened Playlist / Album view
	aSpot := &App{offline: true, screen: Spotify, w: 320, h: 240}
	aSpot.spot.loggedIn = true
	var spTracks []spotifyTrack
	for i := 0; i < 30; i++ {
		spTracks = append(spTracks, spotifyTrack{ID: fmt.Sprintf("t%d", i), Title: fmt.Sprintf("T %d", i)})
	}
	pl := &spotifyPlaylist{ID: "pl_test", Name: "Test", Tracks: spTracks}
	aSpot.spot.openPl = pl
	aSpot.spot.openPlSel = 25

	// ActLeft in openPl skips backward by 10 and does NOT close playlist
	aSpot.input = Input{}
	aSpot.input.inject(ActLeft)
	aSpot.updateSpotify()
	if aSpot.spot.openPl == nil || aSpot.spot.openPlSel != 15 {
		t.Fatalf("expected openPlSel to skip from 25 to 15 without closing pl, got sel=%d pl=%v", aSpot.spot.openPlSel, aSpot.spot.openPl)
	}

	aSpot.input = Input{}
	aSpot.input.inject(ActLeft)
	aSpot.updateSpotify()
	if aSpot.spot.openPl == nil || aSpot.spot.openPlSel != 5 {
		t.Fatalf("expected openPlSel to skip from 15 to 5, got sel=%d pl=%v", aSpot.spot.openPlSel, aSpot.spot.openPl)
	}

	// ActLeft at 5 caps at 0 and does NOT close playlist (even though sel reaches 0)
	aSpot.input = Input{}
	aSpot.input.inject(ActLeft)
	aSpot.updateSpotify()
	if aSpot.spot.openPl == nil || aSpot.spot.openPlSel != 0 {
		t.Fatalf("expected openPlSel=0 and openPl open, got sel=%d pl=%v", aSpot.spot.openPlSel, aSpot.spot.openPl)
	}

	// ActBack closes openPl
	aSpot.input = Input{}
	aSpot.input.inject(ActBack)
	aSpot.updateSpotify()
	if aSpot.spot.openPl != nil {
		t.Fatalf("expected ActBack to close openPl")
	}

	// 3. Spotify Main Tab view (e.g. Liked songs)
	aSpot.spot.tab = spLiked
	aSpot.spot.liked = spTracks
	aSpot.spot.sel[spLiked] = 20
	aSpot.spot.tabFocus = false

	// ActLeft in main tab skips backward by 10 without jumping to tabFocus (even though 20%10 == 0)
	aSpot.input = Input{}
	aSpot.input.inject(ActLeft)
	aSpot.updateSpotify()
	if aSpot.spot.tabFocus || aSpot.spot.sel[spLiked] != 10 {
		t.Fatalf("expected sel to skip from 20 to 10 with tabFocus=false, got sel=%d tabFocus=%v", aSpot.spot.sel[spLiked], aSpot.spot.tabFocus)
	}

	aSpot.input = Input{}
	aSpot.input.inject(ActLeft)
	aSpot.updateSpotify()
	if aSpot.spot.tabFocus || aSpot.spot.sel[spLiked] != 0 {
		t.Fatalf("expected sel to skip from 10 to 0 with tabFocus=false, got sel=%d tabFocus=%v", aSpot.spot.sel[spLiked], aSpot.spot.tabFocus)
	}

	// ActBack enters tabFocus
	aSpot.input = Input{}
	aSpot.input.inject(ActBack)
	aSpot.updateSpotify()
	if !aSpot.spot.tabFocus {
		t.Fatalf("expected ActBack to set tabFocus=true in Spotify tab")
	}
}

func TestToggleSpotifyLikeNormalizesAndUpdates(t *testing.T) {
	a := &App{offline: true}
	a.spot.liked = []spotifyTrack{
		{ID: "xyz", Title: "Existing"},
	}
	a.spot.likedTotal = 1

	// Toggle unlike with prefix
	a.toggleSpotifyLike(spotifyTrack{ID: "spotify://track/xyz", Title: "Existing"})
	if a.isSpotifyTrackLiked("xyz") {
		t.Fatalf("expected track xyz to be unliked")
	}
	if len(a.spot.liked) != 0 {
		t.Fatalf("expected 0 liked tracks, got %d", len(a.spot.liked))
	}

	// Toggle like with prefix
	a.toggleSpotifyLike(spotifyTrack{ID: "spotify:track:abc", Title: "New Track"})
	if !a.isSpotifyTrackLiked("abc") {
		t.Fatalf("expected track abc to be liked")
	}
	if len(a.spot.liked) != 1 || a.spot.liked[0].ID != "abc" {
		t.Fatalf("expected track ID to be normalized to 'abc', got %+v", a.spot.liked)
	}
}

func TestDownloadSpotifyPlaylistCreatesLibraryPlaylist(t *testing.T) {
	a := &App{offline: true}
	a.paths.Data = t.TempDir()
	pl := spotifyPlaylist{
		ID:   "p1",
		Name: "Top Hits",
		Tracks: []spotifyTrack{
			{ID: "t1", Title: "Hit 1", Artist: "Artist 1"},
		},
	}
	a.createOrUpdateSpotifyLibPlaylist(pl)
	var found *Playlist
	for _, p := range a.lib.Playlists {
		if p.Name == "Top Hits" {
			found = p
			break
		}
	}
	if found == nil {
		t.Fatalf("expected playlist 'Top Hits' to be in a.lib.Playlists")
	}
	if found.icon != "spotify" {
		t.Fatalf("expected playlist icon 'spotify', got %q", found.icon)
	}
	if len(found.Songs) != 1 || found.Songs[0].Path != "spotify://track/t1" {
		t.Fatalf("expected 1 song with spotify://track/t1, got %+v", found.Songs)
	}
	// Verify rowFor emits icon "spotify"
	v := &view{kind: vPlaylists, playlists: a.lib.Playlists}
	row := a.rowFor(v, 0)
	if row.icon != "spotify" {
		t.Fatalf("expected row.icon 'spotify', got %q", row.icon)
	}

	// Verify menu item in spotifyPlaylistMenu
	a.spotifyPlaylistMenu(pl)
	if a.menu == nil {
		t.Fatalf("expected spotifyPlaylistMenu to open")
	}
	var downItem *menuItem
	for i := range a.menu.items {
		if a.menu.items[i].icon == "download" {
			downItem = &a.menu.items[i]
			break
		}
	}
	if downItem == nil || downItem.label != T("spotify_download_playlist") {
		t.Fatalf("expected download item with label %q, got %+v", T("spotify_download_playlist"), downItem)
	}
	downItem.do()
}
