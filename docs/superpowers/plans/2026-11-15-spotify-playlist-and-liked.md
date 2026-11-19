# Spotify Playlist Library Integration & Instant Liked Songs Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate Spotify playlist downloads with Audiofilus Library playlists displaying the Spotify icon on the left, and fix Spotify liked songs list so it updates immediately when liking/unliking without requiring an app reload.

**Architecture:** M3U playlists persist custom icon metadata (`#EXT-AUDIOFILUS-ICON:spotify`) and virtual Spotify URIs. Downloading a Spotify playlist enqueues tracks and creates an immediate `.m3u8` playlist in `data/playlists/` with `pl.icon = "spotify"` reflected in `Library -> Playlists`. Spotify Liked Songs tracking normalizes track IDs, sends metadata to daemon, tracks initial load state via `likedLoaded`, and updates the UI slice immediately.

**Tech Stack:** Go (Ebitengine, standard library `bufio`, `net`, `encoding/json`).

**Spec:** Short design approved in session (Spotify Playlist in Library with icon, instant Liked Songs update).

## Global Constraints

- Go 1.22+ conventions; `gofmt -l .` must be clean.
- Code comments in Spanish in full sentences matching project conventions.
- Error messages and logs in English.
- UI strings in all 13 localization files (`lang.go` and `lang_*.go`).
- Zero CGO (`CGO_ENABLED=0`) compatibility on `linux/arm64` and `windows/amd64`.

## Review Focus

1. **Relative path calculation in `savePlaylist` for virtual Spotify paths**: `filepath.Rel(dir, "spotify://track/id")` fails or produces invalid paths on Windows/Linux; must be skipped for remote/Spotify URIs.
2. **Scanner filtering in `readM3U`**: lines starting with `spotify:` or `spotify://` must not be treated as relative local filesystem paths.
3. **ID mismatch when toggling like**: `tr.ID` with `"spotify://track/"` prefix versus clean alphanumeric IDs in `a.spot.liked` causes `==` check to fail unless normalized with `extractSpotifyID`.
4. **First-time navigation to Liked tab after liking a song**: `len(s.liked) == 1` previously blocked fetching the rest of the 50+ liked songs from Spotify API.
5. **Duplicate favorite items in Now Playing Start menu**: When playing Spotify tracks, both `marks.go` favorite and Spotify like were displayed.

---

### Task 1: Support Playlist Icon Metadata and Spotify URIs in M3U

**Files:**
- Modify: `internal/library/scan.go:160-185`
- Modify: `internal/ui/playlists.go:30-55`
- Modify: `internal/ui/library.go:228-245`
- Test: `internal/ui/playlists_test.go`

**Interfaces:**
- Consumes: `Playlist` struct in `internal/ui/library.go`
- Produces: `readPlaylistIcon(path string) string` helper and extended `#EXT-AUDIOFILUS-ICON:` tag support.

- [ ] **Step 1: Write failing test in `internal/ui/playlists_test.go`**

Test that saving a playlist with `pl.icon = "spotify"` and Spotify URIs preserves the icon header and correctly reloads it in `addPlaylists`.

```go
func TestPlaylistIconAndSpotifyURIs(t *testing.T) {
    dir := t.TempDir()
    plPath := filepath.Join(dir, "TestSpot.m3u8")
    pl := &Playlist{
        Name: "TestSpot",
        Path: plPath,
        icon: "spotify",
        Songs: []Song{
            {Path: "spotify://track/123", Title: "Song 1", Artist: "Artist 1"},
        },
    }
    a := &App{}
    if err := a.savePlaylist(pl); err != nil {
        t.Fatalf("savePlaylist failed: %v", err)
    }
    icon := readPlaylistIcon(plPath)
    if icon != "spotify" {
        t.Fatalf("expected icon 'spotify', got %q", icon)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestPlaylistIconAndSpotifyURIs ./internal/ui`
Expected: FAIL with undefined `readPlaylistIcon` or missing icon.

- [ ] **Step 3: Implement icon saving, reading, and URI handling**

1. In `internal/ui/playlists.go`:
   - In `savePlaylist`: If `pl.icon != ""`, write `#EXT-AUDIOFILUS-ICON:%s\n`. If `s.Path` is remote or `strings.HasPrefix(s.Path, "spotify:")` or `strings.HasPrefix(s.Path, "spotify://")`, write `p` directly without `filepath.Rel`.
   - Implement `readPlaylistIcon(path string) string` scanning for `#EXT-AUDIOFILUS-ICON:`.
2. In `internal/library/scan.go`:
   - In `readM3U`: Preserve lines with `strings.Contains(line, "://") || strings.HasPrefix(line, "spotify:")` instead of running `filepath.Join` and `decode.IsAudioFile`.
3. In `internal/ui/library.go`:
   - In `addPlaylists`: Call `readPlaylistIcon(p)` and assign `pl.icon`. For items in `files` starting with `spotify:` or `spotify://` not found in `l.song`, construct `Song{Path: f, Title: filepath.Base(f)}` so songs appear in the playlist view.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestPlaylistIconAndSpotifyURIs ./internal/ui`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/library/scan.go internal/ui/playlists.go internal/ui/library.go internal/ui/playlists_test.go
git commit -m "feat(playlists): support icon metadata and virtual spotify uris in m3u"
```

---

### Task 2: Instant Spotify Liked Songs Updates

**Files:**
- Modify: `internal/ui/spotify.go:1840-1920`
- Modify: `internal/ui/spotify.go:1300-1338`
- Modify: `internal/ui/menu.go:265-270`
- Test: `internal/ui/spotify_test.go`

**Interfaces:**
- Consumes: `control.Call` ("like", "unlike")
- Produces: `a.spot.likedLoaded` state and instant `a.spot.liked` list updates

- [ ] **Step 1: Write failing test in `internal/ui/spotify_test.go`**

Test that `toggleSpotifyLike` properly updates `a.spot.liked` immediately regardless of whether the input `tr.ID` has a `spotify://track/` prefix, and that `likedLoaded` ensures proper loading behavior.

```go
func TestToggleSpotifyLikeNormalizesAndUpdates(t *testing.T) {
    a := &App{}
    a.spot.liked = []spotifyTrack{
        {ID: "xyz", Title: "Existing"},
    }
    // Toggle unlike with prefix
    a.toggleSpotifyLikeLocal(spotifyTrack{ID: "spotify://track/xyz", Title: "Existing"})
    if a.isSpotifyTrackLiked("xyz") {
        t.Fatalf("expected track xyz to be unliked")
    }
    // Toggle like with prefix
    a.toggleSpotifyLikeLocal(spotifyTrack{ID: "spotify:track:abc", Title: "New Track"})
    if !a.isSpotifyTrackLiked("abc") {
        t.Fatalf("expected track abc to be liked")
    }
    if a.spot.liked[0].ID != "abc" {
        t.Fatalf("expected track ID to be normalized to 'abc', got %q", a.spot.liked[0].ID)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestToggleSpotifyLikeNormalizesAndUpdates ./internal/ui`
Expected: FAIL.

- [ ] **Step 3: Implement instant Liked Songs synchronization**

1. In `internal/ui/spotify.go`:
   - In `toggleSpotifyLike`: Normalize `tr.ID = extractSpotifyID(tr.ID)` before sending to socket and before matching in `a.spot.liked`.
   - Normalize elements when comparing: `extractSpotifyID(t.ID) == tr.ID`.
   - Update `a.spot.liked` and `a.spot.likedTotal` immediately on main thread.
   - Add `likedLoaded bool` to `type spotifyState`.
   - In `checkSpotifyTabLoad()`: Replace `if len(s.liked) == 0` with `if !s.likedLoaded`, and set `s.likedLoaded = true` on completion. If `s.liked` already contained optimistically liked tracks, merge them with the fetched results without duplicates.
2. In `internal/ui/menu.go`:
   - In `openNowMenu()`: When `isSpotifyPath(a.playingSong.Path)` is true, suppress local `a.markItems(...)` so only the Spotify Like/Unlike heart option appears.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestToggleSpotifyLikeNormalizesAndUpdates ./internal/ui`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/spotify.go internal/ui/menu.go internal/ui/spotify_test.go
git commit -m "feat(spotify): update liked songs immediately and normalize track ids"
```

---

### Task 3: Spotify Playlist Download Menu & Library Integration

**Files:**
- Modify: `internal/ui/spotify.go:703-725`
- Modify: `internal/ui/spotify.go:1790-1800`
- Modify: `internal/ui/spotify.go:2070-2080`
- Modify: `internal/ui/lang*.go`
- Test: `internal/ui/spotify_test.go`

**Interfaces:**
- Consumes: `a.savePlaylist`, `a.refreshPlaylists`
- Produces: `a.downloadSpotifyPlaylist` that saves to Library with `pl.icon = "spotify"`

- [ ] **Step 1: Write failing test in `internal/ui/spotify_test.go`**

Test that calling `downloadSpotifyPlaylist` creates a playlist in `a.lib.Playlists` with `icon: "spotify"`, saves `.m3u8` to disk, and matches `rowFor` rendering with Spotify icon.

```go
func TestDownloadSpotifyPlaylistCreatesLibraryPlaylist(t *testing.T) {
    a := newTestApp()
    a.paths.Data = t.TempDir()
    pl := spotifyPlaylist{
        ID:   "p1",
        Name: "Top Hits",
        Tracks: []spotifyTrack{
            {ID: "t1", Title: "Hit 1", Artist: "Artist 1"},
        },
    }
    a.createOrUpdateSpotifyLibPlaylist(pl)
    found := false
    for _, p := range a.lib.Playlists {
        if p.Name == "Top Hits" && p.icon == "spotify" {
            found = true
            break
        }
    }
    if !found {
        t.Fatalf("expected 'Top Hits' playlist with icon 'spotify' in library playlists")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDownloadSpotifyPlaylistCreatesLibraryPlaylist ./internal/ui`
Expected: FAIL with missing method or assertion.

- [ ] **Step 3: Implement Spotify playlist creation in Library**

1. In `internal/ui/spotify.go`:
   - Implement `createOrUpdateSpotifyLibPlaylist(pl spotifyPlaylist) *Playlist`:
     - Checks if playlist with `pl.Name` already exists in `a.lib.Playlists`. If not, creates one with `Path: playlistFile(a.playlistsDir(), pl.Name, ".m3u8")`.
     - Sets `icon = "spotify"`.
     - Populates `pl.Songs` (using local path from `isSpotifyTrackDownloaded(tr)` if exists, else `"spotify://track/" + tr.ID`).
     - Calls `a.savePlaylist(newPl)` and appends to `a.lib.Playlists`.
     - Calls `a.refreshPlaylists()`.
   - In `downloadSpotifyPlaylist(pl)`:
     - Call `a.createOrUpdateSpotifyLibPlaylist(pl)`.
     - Enqueue tracks for download via `a.downloadSpotifyTracksTo`.
   - In `spotifyPlaylistMenu` and `spotifyTrackMenu`:
     - Use label `T("spotify_download_playlist")`.
2. In `internal/ui/lang*.go`:
   - Add `"spotify_download_playlist"` key across all 13 languages (e.g. `"Tải playlist"` in `lang_vi.go`, `"Download playlist"` in `lang.go`, `"Descargar playlist"` in `lang.go` Spanish).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestDownloadSpotifyPlaylistCreatesLibraryPlaylist ./internal/ui`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ui/spotify.go internal/ui/lang*.go internal/ui/spotify_test.go
git commit -m "feat(ui): add download playlist menu and integrate spotify playlists into library"
```

---

### Task 4: Complete Validation and Build Checks

**Files:**
- None (verification task)

- [ ] **Step 1: Check formatting**

Run: `gofmt -l .`
Expected: Empty output.

- [ ] **Step 2: Run all Go tests and vet**

Run: `go vet ./...` and `go test ./...`
Expected: PASS with 0 errors.

- [ ] **Step 3: Verify console and Windows builds without CGO**

Run: `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o NUL ./cmd/audiofilus`
Run: `GOOS=windows go vet ./...`
Expected: SUCCESS.
