# Spotify Online Playback & TrimUI Brick Pro (muOS) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add online Spotify playback supporting both Premium and Free accounts with QR-code login, immune to HTTP 429 rate limits, and deeply optimize hardware controls (LEDs, display blanking, Fn switch) for TrimUI Brick Pro running muOS.

**Architecture:** Pure Go two-process model (UI Ebitengine <-> Unix socket <-> Daemon). The daemon hosts a local rate-limited Spotify client (`internal/spotify`) with delta catalog sync (`snapshot_id`), PKCE auth server, and a dual-stream audio resolver (Premium AES-128-CTR Ogg Vorbis or Free YTM AAC). Hardware platform hooks in `internal/platform/muos.go` manage Brick Pro display and LED animators during pocket sleep.

**Tech Stack:** Go (1.23+), Ebitengine v2, Oto audio engine, `skip2/go-qrcode`, standard library `crypto/aes`, `crypto/cipher`, `net/http`, `golang.org/x/sync/singleflight`.

**Spec:** `docs/superpowers/specs/2026-11-08-spotify-brick-pro-muos-design.md`

## Global Constraints

- Zero CGO (`CGO_ENABLED=0`): Must compile for `linux/arm64` and `windows/amd64` without cgo.
- Code comments in Spanish in full sentences; error messages, log lines, and tests in English.
- UI strings localized across all 13 language files (`lang.go` and `lang_*.go`).
- Zero real music, passwords, or personal paths in repository or test files.
- Audio and power-saving logic reside in the daemon service, not in the UI process.

## Review Focus

1. **HTTP 429 without or with invalid `Retry-After` header:** Must default to safe backoff (2s + jitter) rather than panicking or immediately retrying.
2. **Expired access token mid-request:** Must automatically trigger proactive token refresh via `refresh_token`, persist to `data/spotify.json`, and retry transparently.
3. **Network loss during track streaming:** If track exists in `data/cache/spotify/songs/`, play from disk; if unbuffered, pause gracefully without crashing the audio thread.
4. **Spotify Free fallback match failure:** When no suitable duration match ($\pm 3\text{s}$) is found via `internal/ytm`, log warning and advance queue cleanly.
5. **Rapid toggling of TrimUI Brick Pro Fn switch:** Debounce hardware switch state changes (300ms window) to avoid display driver thrashing.

---

### Task 1: Rate Limiter & Singleflight Transport

**Files:**
- Create: `internal/spotify/transport.go`
- Test: `internal/spotify/transport_test.go`

**Interfaces:**
- Produces:
  - `type Transport struct`: implements `http.RoundTripper` with token bucket limiter, singleflight coalescing, and transparent 429 `Retry-After` backoff.
  - `NewTransport(rateLimit float64, burst int, base http.RoundTripper) *Transport`

- [ ] **Step 1: Write the failing test for rate limiting and 429 backoff**

```go
package spotify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testRateLimiterAndBackoff(t *testing.T) {
	var attempts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":"rate limit"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer ts.Close()

	client := &http.Client{Transport: NewTransport(10, 2, ts.Client().Transport)}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts.Load())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestRateLimiterAndBackoff ./internal/spotify`  
Expected: FAIL (package or function not defined).

- [ ] **Step 3: Implement `internal/spotify/transport.go`**

Implement `Transport` implementing `http.RoundTripper`:
- Token bucket pacing using `time.Ticker` or elapsed time math.
- Captures `http.StatusTooManyRequests` (429), reads `Retry-After`, adds 1-2s jitter, sleeps, and retries up to 3 times.
- Coalesces duplicate GET requests using `singleflight.Group`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestRateLimiterAndBackoff ./internal/spotify`  
Expected: PASS.

- [ ] **Step 5: Add test for 429 missing `Retry-After` header (Review Focus #1)**

Ensure missing `Retry-After` defaults to safe fallback backoff without panic. Run `go test -v ./internal/spotify`.

---

### Task 2: Spotify Auth (PKCE Local Server & Token Management)

**Files:**
- Create: `internal/spotify/auth.go`
- Test: `internal/spotify/auth_test.go`

**Interfaces:**
- Consumes: `internal/spotify/transport.go`
- Produces:
  - `type Credentials struct { AccessToken string, RefreshToken string, ExpiresAt time.Time, Product string }`
  - `type AuthManager struct`
  - `NewAuthManager(dataPath string, clientID string) *AuthManager`
  - `(a *AuthManager) StartLoginFlow() (loginURL string, err error)`
  - `(a *AuthManager) Credentials() (Credentials, error)`
  - `(a *AuthManager) ProactiveToken() (string, error)`

- [ ] **Step 1: Write the failing test for PKCE generation and token persistence**

```go
package spotify

import (
	"path/filepath"
	"testing"
	"time"
)

func testAuthStorage(t *testing.T) {
	dir := t.TempDir()
	am := NewAuthManager(dir, "test-client-id")
	creds := Credentials{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Product:      "premium",
	}
	if err := am.Save(creds); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := am.Credentials()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AccessToken != creds.AccessToken || got.Product != "premium" {
		t.Errorf("got %+v, want %+v", got, creds)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestAuthStorage ./internal/spotify`  
Expected: FAIL (NewAuthManager undefined).

- [ ] **Step 3: Implement `internal/spotify/auth.go`**

Implement PKCE verifier generation (`crypto/rand`), SHA-256 challenge, local ephemeral HTTP callback listener on port `8888` / random fallback, token exchange with `https://accounts.spotify.com/api/token`, and proactive refresh when `time.Until(ExpiresAt) < 5*time.Minute`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestAuthStorage ./internal/spotify`  
Expected: PASS.

- [ ] **Step 5: Add test for expired access token refresh (Review Focus #2)**

Test that `ProactiveToken()` detects expired tokens, calls mock refresh endpoint, and updates saved credentials.

---

### Task 3: Delta Catalog Cache & Playlist Synchronization

**Files:**
- Create: `internal/spotify/catalog.go`
- Test: `internal/spotify/catalog_test.go`

**Interfaces:**
- Consumes: `internal/spotify/auth.go`, `internal/spotify/transport.go`
- Produces:
  - `type Track struct { ID, Title, Artist, Album, ISRC string; Duration time.Duration; CoverURL string }`
  - `type Playlist struct { ID, Name, SnapshotID string; Tracks []Track }`
  - `type Catalog struct`
  - `NewCatalog(cacheDir string, auth *AuthManager, client *http.Client) *Catalog`
  - `(c *Catalog) SyncPlaylists() ([]Playlist, error)`
  - `(c *Catalog) LikedSongs() ([]Track, error)`

- [ ] **Step 1: Write the failing test for Delta Sync using `snapshot_id`**

```go
package spotify

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func testDeltaSyncPlaylist(t *testing.T) {
	var trackFetches atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me/playlists" {
			w.Write([]byte(`{"items":[{"id":"pl1","name":"Hits","snapshot_id":"snap1"}]}`))
			return
		}
		if r.URL.Path == "/v1/playlists/pl1/tracks" {
			trackFetches.Add(1)
			w.Write([]byte(`{"items":[{"track":{"id":"t1","name":"Song 1","artists":[{"name":"Artist 1"}],"album":{"name":"Album 1"}}}]}`))
			return
		}
	}))
	defer ts.Close()

	cat := NewCatalog(t.TempDir(), nil, ts.Client())
	cat.apiBase = ts.URL
	// First sync: must fetch tracks
	if _, err := cat.SyncPlaylists(); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if trackFetches.Load() != 1 {
		t.Fatalf("expected 1 fetch, got %d", trackFetches.Load())
	}
	// Second sync with identical snapshot_id: must skip track fetch
	if _, err := cat.SyncPlaylists(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if trackFetches.Load() != 1 {
		t.Fatalf("expected tracks to be cached without refetching, got %d fetches", trackFetches.Load())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDeltaSyncPlaylist ./internal/spotify`  
Expected: FAIL.

- [ ] **Step 3: Implement `internal/spotify/catalog.go`**

Implement JSON disk persistence to `data/cache/spotify/catalog.json`. If `playlist.SnapshotID == cached.SnapshotID`, reuse cached `Tracks`. Only request `tracks` endpoint when snapshot ID changes.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestDeltaSyncPlaylist ./internal/spotify`  
Expected: PASS.

---

### Task 4: Dual-Stream Resolver & Audio Cache

**Files:**
- Create: `internal/spotify/stream.go`
- Test: `internal/spotify/stream_test.go`

**Interfaces:**
- Consumes: `internal/spotify/auth.go`, `internal/ytm`
- Produces:
  - `type StreamResolver struct`
  - `NewStreamResolver(cacheDir string, auth *AuthManager, client *http.Client) *StreamResolver`
  - `(s *StreamResolver) Resolve(track Track) (localPath string, ready bool, err error)`
  - `(s *StreamResolver) Prefetch(track Track)`
  - `DecryptSpotifyOgg(in io.Reader, key [16]byte) (io.Reader, error)`

- [ ] **Step 1: Write failing test for AES-128-CTR decryption and header stripping**

```go
package spotify

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"io"
	"testing"
)

func testDecryptSpotifyOgg(t *testing.T) {
	key := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	expectedData := []byte("OggS_valid_vorbis_stream_content")
	// Spotify container: 167 bytes header + AES-128-CTR encrypted payload
	header := make([]byte, 167)
	block, _ := aes.NewCipher(key[:])
	iv := make([]byte, 16) // Counter starts at 0
	stream := cipher.NewCTR(block, iv)
	encryptedPayload := make([]byte, len(expectedData))
	stream.XORKeyStream(encryptedPayload, expectedData)

	fullData := append(header, encryptedPayload...)
	decrypted, err := DecryptSpotifyOgg(bytes.NewReader(fullData), key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	out, _ := io.ReadAll(decrypted)
	if !bytes.Equal(out, expectedData) {
		t.Errorf("got %q, want %q", out, expectedData)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestDecryptSpotifyOgg ./internal/spotify`  
Expected: FAIL (DecryptSpotifyOgg undefined).

- [ ] **Step 3: Implement `internal/spotify/stream.go`**

Implement:
- `DecryptSpotifyOgg`: Discards 167-byte header, creates AES-CTR cipher stream with keystream block counter.
- `Resolve`:
  1. If track is already cached in `cacheDir/songs/<id>.audio`, returns path.
  2. If Premium: retrieves key and streams from Spotify CDN.
  3. If Free: invokes `ytm.Find` matching track title/artist/duration ($\pm 3\text{s}$), downloads to disk cache.
  4. Manages LRU size limit (default 2GB).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestDecryptSpotifyOgg ./internal/spotify`  
Expected: PASS.

- [ ] **Step 5: Add tests for offline cache retrieval & fallback match failure (Review Focus #3 & #4)**

Verify that if network is down, existing cached track is returned. Verify that if YTM search yields no match, returns explicit error without panic.

---

### Task 5: TrimUI Brick Pro Platform & Sleep Optimization

**Files:**
- Modify: `internal/platform/muos.go`
- Test: `internal/platform/muos_test.go`

**Interfaces:**
- Consumes: Platform interface, sysfs filesystem paths
- Produces:
  - Enhanced `EnterSleep(screenOff bool)` / `ExitSleep()` supporting `/sys/class/led_anim/max_scale` and `/sys/kernel/debug/dispdbg`.
  - Fn switch listener hook for pocket mode toggle.

- [ ] **Step 1: Write failing test for Brick Pro LED and display blanking on muOS**

```go
package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func testMuOSTrimUIBrickLED(t *testing.T) {
	temp := t.TempDir()
	ledFile := filepath.Join(temp, "max_scale")
	_ = os.WriteFile(ledFile, []byte("180"), 0o644)

	m := &muos{ledPath: ledFile}
	if err := m.EnterSleep(true); err != nil {
		t.Fatalf("EnterSleep: %v", err)
	}
	val, _ := os.ReadFile(ledFile)
	if string(val) != "0" {
		t.Errorf("expected LED max_scale 0 during sleep, got %s", val)
	}

	if err := m.ExitSleep(); err != nil {
		t.Fatalf("ExitSleep: %v", err)
	}
	val, _ = os.ReadFile(ledFile)
	if string(val) != "180" {
		t.Errorf("expected LED max_scale restored to 180, got %s", val)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestMuOSTrimUIBrickLED ./internal/platform`  
Expected: FAIL.

- [ ] **Step 3: Update `internal/platform/muos.go`**

1. Define paths:
   ```go
   const (
       brickLeds = "/sys/class/led_anim/max_scale"
       brickDisp = "/sys/kernel/debug/dispdbg"
       brickFn   = "/sys/class/switch/fn/state"
   )
   ```
2. In `EnterSleep`: if `brickLeds` exists, save current scale and write `0`. If `brickDisp` exists, blank panel.
3. In `ExitSleep`: restore saved LED scale and restore brightness.
4. Add debounced Fn switch watcher (300ms window, Review Focus #5).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestMuOSTrimUIBrickLED ./internal/platform`  
Expected: PASS.

---

### Task 6: Daemon & Socket Control Integration

**Files:**
- Modify: `internal/daemon/daemon.go`
- Create: `internal/daemon/spotify.go`
- Test: `internal/daemon/spotify_test.go`

**Interfaces:**
- Consumes: `internal/spotify`
- Produces:
  - Remote track handlers for `spotify://track/<id>`.
  - Control socket commands: `spotify login`, `spotify status`, `spotify library`, `spotify search`.

- [ ] **Step 1: Write failing test for Spotify track resolution in daemon**

```go
package daemon

import (
	"testing"
)

func testSpotifyTrackURLRecognition(t *testing.T) {
	uri := "spotify://track/4cOdK2wGLETKBW3PvgPWqT"
	if !isSpotifyURI(uri) {
		t.Errorf("expected %s to be recognized as spotify URI", uri)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSpotifyTrackURLRecognition ./internal/daemon`  
Expected: FAIL.

- [ ] **Step 3: Implement `internal/daemon/spotify.go` and hook into `daemon.go`**

1. Initialize `spotify.NewManager(paths.Data)` in `daemon.Run()`.
2. Hook `eng.OpenRemote`:
   ```go
   if spotify.IsSpotify(path) {
       src, ready, err := spotifyMgr.Source(path)
       return src, ready, err
   }
   ```
3. Hook `library.RemoteTrack`: return title, artist, album for `spotify://` URIs.
4. Hook socket dispatch for `spotify` commands.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestSpotifyTrackURLRecognition ./internal/daemon`  
Expected: PASS.

---

### Task 7: UI - Dedicated Spotify Screen & QR Code Enhancements

**Files:**
- Modify: `internal/ui/qr.go`
- Create: `internal/ui/spotify.go`
- Modify: `internal/ui/app.go`, `internal/ui/lang.go`, and all `internal/ui/lang_*.go`
- Test: `internal/ui/spotify_test.go`

**Interfaces:**
- Consumes: `internal/control` client, `internal/ui/qr.go`
- Produces:
  - `ScreenSpotify`: Tabbed navigation (Liked Songs, Playlists, Albums, Search).
  - Context menu: Play, Play Next, Like/Unlike, Download to Local Library.
  - Dynamically scaled QR display with quiet zone for 1024×768 3.2" screens.

- [ ] **Step 1: Write failing test for Spotify screen tab switching and localization**

```go
package ui

import (
	"testing"
)

func testSpotifyLocalization(t *testing.T) {
	for _, lang := range []string{"es", "en", "vi", "ja", "de"} {
		SetLanguage(lang)
		if T("spotify") == "" || T("spotify_liked") == "" {
			t.Errorf("missing spotify translations for lang %s", lang)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSpotifyLocalization ./internal/ui`  
Expected: FAIL (missing translations).

- [ ] **Step 3: Implement `internal/ui/spotify.go` & update `qr.go` and `lang*.go`**

1. In `internal/ui/qr.go`: compute scale $s = \max(2, \min(room/n, 8))$ ensuring clean rendering on 1024×768 displays.
2. In `internal/ui/spotify.go`: Implement `drawSpotify(dst *ebiten.Image)` with tabs (Liked, Playlists, Albums, Search), virtual keyboard navigation, and track context popup.
3. In `internal/ui/app.go`: Add `ScreenSpotify` to screen rotation (`Tab` / `Select`).
4. Add localization strings in all 13 `lang*.go` files (`spotify`, `spotify_liked`, `spotify_playlists`, `spotify_search`, `spotify_login_scan`, etc.).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestSpotifyLocalization ./internal/ui`  
Expected: PASS.

---

### Task 8: muOS Packaging & Multi-Platform Verification

**Files:**
- Modify: `muos/audiofilus/mux_launch.sh`
- Verify: `build.sh`

- [ ] **Step 1: Update `muos/audiofilus/mux_launch.sh`**

Add check for TrimUI Brick Pro hardware identification:
```sh
if [ -f /sys/class/led_anim/max_scale ] || grep -q "Allwinner" /proc/cpuinfo 2>/dev/null; then
    export AUDIOFILUS_DEVICE="TrimUI Brick Pro"
fi
```

- [ ] **Step 2: Verify zero CGO Linux arm64 build**

Run:
```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/audiofilus
```
Expected: Exit code 0, no compile errors.

- [ ] **Step 3: Verify Windows compatibility**

Run:
```sh
GOOS=windows go vet ./...
```
Expected: PASS, no missing twin files or invalid syscall references.

- [ ] **Step 4: Verify full test suite and code formatting**

Run:
```sh
gofmt -l .
go vet ./...
go test ./...
```
Expected: `gofmt -l .` output is empty; all tests pass.
