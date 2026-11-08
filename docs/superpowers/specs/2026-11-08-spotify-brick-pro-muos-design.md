# Design Specification: Spotify Online Playback & TrimUI Brick Pro (muOS) Integration

**Date:** 2026-11-08  
**Status:** Draft / Pending Review  
**Target Platform:** TrimUI Brick Pro (Allwinner A133P, 1024×768 4:3 display, muOS / MustardOS) and standard Audiofilus platforms (Linux arm64, PC desktop, Windows)

---

## 1. Executive Summary

This specification defines the architectural design for adding online Spotify playback to **Audiofilus** with deep hardware optimizations for the **TrimUI Brick Pro** handheld running **muOS**.

Key capabilities:
1. **Frictionless QR Code Login:** Users log into their Spotify account by scanning an on-screen QR code from their smartphone or pairing via Spotify Connect (Zeroconf), eliminating on-device credential typing.
2. **Anti-Rate-Limit Architecture:** A robust client-side mitigation strategy guarantees zero `429 Too Many Requests` errors using delta synchronization (`snapshot_id`), token-bucket pacing, singleflight coalescing, transparent `Retry-After` backoff with jitter, and SpClient GraphQL endpoints.
3. **Dual-Tier Audio Playback:**
   - **Spotify Premium:** Streams encrypted Ogg Vorbis audio directly from Spotify CDN, decrypts with AES-128-CTR audio keys, strips container headers, and decodes via Audiofilus's pure Go `oggvorbis` decoder (< 3% CPU on Allwinner A133P).
   - **Spotify Free:** Automatically resolves track metadata (ISRC, title, artist, duration) to high-quality audio streams via the existing `internal/ytm` (`yt-dlp`) subsystem, delivered as AAC `.m4a` without transcoding overhead.
4. **Dedicated Spotify Screen (Option A):** A clean browsing interface for Liked Songs, Playlists, Albums, and Search, designed with modular hooks ready for synchronized lyrics (color-lyrics) in subsequent iterations.
5. **TrimUI Brick Pro & muOS Hardware Adaptation:** Complete integration with muOS sleep management, backlight blanking, Brick Pro RGB LED animators, hardware Fn toggle switch for pocket mode, haptic rumble, and ALSA/PipeWire volume synchronization.

---

## 2. Goals & Non-Goals

### Goals
- Support both **Spotify Premium** and **Spotify Free** accounts seamlessly.
- Eliminate HTTP 429 rate limit exceptions when browsing large libraries or playlists.
- Zero CGO (`CGO_ENABLED=0`): compile cleanly for `linux/arm64` and `windows/amd64`.
- Maintain Audiofilus's background playback philosophy: music continues playing during pocket mode or when the UI process is suspended.
- Deep power efficiency: < 5% battery consumption per hour during pocket mode playback on TrimUI Brick Pro.
- Full offline-first fallback: cached tracks and saved metadata remain browseable and playable when Wi-Fi is disconnected.

### Non-Goals
- Full Spotify Connect *remote player control* from other external Spotify apps (only Zeroconf discovery and direct on-device playback are in scope).
- Synchronized color-lyrics implementation in this initial phase (architecture will be prepared with data structures and UI anchors, but rendering color-lyrics will follow in a subsequent phase).
- On-device editing/reordering of remote Spotify playlists (read-only browsing and like/unlike toggling only).

---

## 3. Architecture & Process Model

Audiofilus maintains a strict two-process separation:
- **UI Process (`cmd/audiofilus/ui.go`, `internal/ui/`):** Runs the Ebitengine graphics loop, handles controller input, displays screens, and renders QR codes.
- **Background Daemon (`internal/daemon/`, `internal/engine/`):** Manages playback state, decoders, cache storage, network streaming, and hardware platform hooks. Communicates with UI and CLI clients over a local domain socket.

```
                      +---------------------------------------+
                      |       UI Process (Ebitengine)         |
                      |  - Spotify Screen (Library/Search)    |
                      |  - QR Code Display (Auth/Pairing)     |
                      +-------------------+-------------------+
                                          | Local Socket RPC
                                          v
+-------------------------------------------------------------+
|             Background Daemon (Audio Service)               |
|                                                             |
|  +-----------------------+     +-------------------------+  |
|  |   internal/spotify    |     |   internal/platform     |  |
|  | - Auth & Token Bucket |     | - muOS / TrimUI Brick   |  |
|  | - Delta Sync Cache    |     | - Backlight & RGB LEDs  |  |
|  | - Dual Stream Router  |     | - Fn Switch & Caffeine  |  |
|  +-----------+-----------+     +-------------------------+  |
|              |                                              |
|              +-------------------+                          |
|                                  v                          |
|  +-------------------------------------------------------+  |
|  |               internal/engine (Oto Output)            |  |
|  | - Premium: CDN Ogg Decrypt -> Pure Go Vorbis Dec      |  |
|  | - Free: YTM Stream Resolver -> Pure Go AAC / M4A      |  |
|  | - LRU Disk Cache: data/cache/spotify/songs/           |  |
|  +-------------------------------------------------------+  |
+-------------------------------------------------------------+
```

---

## 4. Spotify Authentication & Anti-Rate-Limit Subsystem (`internal/spotify/`)

### 4.1 Authentication: QR Code & Zeroconf Pairing

1. **Local PKCE Server:**
   - When the user opens the Spotify screen while unauthenticated, the daemon opens an ephemeral local HTTP listener (`http://127.0.0.1:<port>/callback` or LAN IP `http://<ip>:<port>/callback`).
   - The daemon generates a PKCE code verifier, challenge, and an OAuth authorization URL with scopes:
     `user-read-private user-read-email user-library-read user-library-modify playlist-read-private playlist-read-collaborative`.
   - The authorization URL is transmitted via socket to the UI and rendered as a high-contrast QR code via `internal/ui/qr.go`.
   - The user scans the QR code with their mobile phone, logs into Spotify, and grants permission. The mobile browser redirects to the local listener, which captures the auth code, exchanges it for access/refresh tokens, and persists them into `data/spotify.json`.
2. **Spotify Connect Zeroconf Announcement:**
   - Uses `internal/mdns` to broadcast service `_spotify-connect._tcp.local` on the local network.
   - When detected in the official Spotify app (under "Connect to a device"), the device accepts the pairing handshake, extracting user credentials directly without external browser redirects.
3. **Token Management & Storage:**
   - Credentials stored in `data/spotify.json`: `{"access_token": "...", "refresh_token": "...", "expires_at": 1700000000, "product": "premium"|"free"}`.
   - Proactive refresh runs 5 minutes prior to token expiration to avoid mid-stream authentication drops.

### 4.2 Anti-Rate-Limit (HTTP 429) Strategy

The public Spotify Web API (`api.spotify.com/v1`) enforces strict, non-deterministic rate limits. To guarantee immunity against HTTP 429 errors:

1. **Delta Synchronization via `snapshot_id`:**
   - Every Spotify playlist carries a unique `snapshot_id`.
   - The local catalog cache (`data/cache/spotify/catalog.json`) stores:
     ```json
     {
       "playlists": [
         {
           "id": "37i9dQZF1DXcBWIGoYBM5M",
           "name": "Today's Top Hits",
           "snapshot_id": "MTcy...",
           "tracks_cached_at": 1700000000,
           "tracks": [ ... ]
         }
       ]
     }
     ```
   - When syncing, Audiofilus fetches only playlist metadata (name + `snapshot_id`). If `snapshot_id` matches the local cache, **no track list request is made**. This cuts API traffic during startup and browsing by >90%.
2. **Token-Bucket Rate Limiter:**
   - All outgoing requests to Spotify API pass through an internal client-side rate limiter capped at 3 requests/second with a burst capacity of 5.
   - Independent queues isolate user-driven requests (search, track selection) from background sync requests (prefetching playlist items).
3. **Singleflight Request Coalescing:**
   - Duplicate concurrent requests for the same metadata or playlist ID share a single in-flight HTTP request via `golang.org/x/sync/singleflight`.
4. **Transparent Backoff with Jitter:**
   - If an HTTP 429 occurs, the HTTP transport captures `Retry-After` header value $T$, applies exponential jitter $T + \text{rand}(1, 3)$, sleeps the background worker transparently, and retries up to 3 times before returning an error to the UI.
5. **Private GraphQL / SpClient Fallback:**
   - Metadata queries use Spotify's internal GraphQL endpoints (`spclient.wg.spotify.com/open-backend-2/v1/`) with desktop client headers where public API quotas are congested.

---

## 5. Dual-Stream Audio Playback Engine

### 5.1 Stream Resolution Workflow (`spotify://track/<id>`)

When a Spotify track is requested:
1. **Cache Verification:** Check `data/cache/spotify/songs/<id>.audio`. If valid, stream directly from disk.
2. **Account Check:**
   - **Case A: Premium Account (`product == "premium"`):**
     1. Request audio key for track from Spotify Key Exchange service.
     2. Fetch Vorbis stream from Spotify CDN (`audio-ak-spotify-com.akamaized.net` or `audio-fa.scdn.co`).
     3. Decrypt on-the-fly using AES-128 in CTR mode (no MAC, raw ciphertext).
     4. Strip the 167-byte proprietary Spotify header.
     5. Feed standard Ogg Vorbis bistream into `internal/decode` pure Go vorbis decoder.
   - **Case B: Free Account (`product == "free"` or fallback):**
     1. Query track metadata: Title, Artist, Album, Duration, and ISRC.
     2. Pass query to `internal/ytm` (`yt-dlp` backend) matching exact duration ($\pm 3$ seconds) and ISRC/title.
     3. Extract direct audio stream URL (AAC 128-160kbps `.m4a`).
     4. Stream and simultaneously spool into `data/cache/spotify/songs/<id>.audio`.
     5. Pure Go MP4/AAC decoder in `internal/decode` handles output to Oto.

### 5.2 Disk Cache & Pre-Buffering
- Cache files stored in `data/cache/spotify/songs/` with an LRU capacity limit configurable in `audiofilus.json` (default: 2 GB).
- When a track reaches 80% playback progress, the next track in the playlist is pre-buffered in the background to ensure gapless transitions.

---

## 6. User Interface & Experience (`internal/ui/`)

### 6.1 Screen Hierarchy (Option A - Dedicated Spotify Screen)

```
[Main Library] <--- (Select / Tab) ---> [Spotify Screen]
                                               |
         +-------------------------------------+-------------------------------------+
         |                                     |                                     |
  [Liked Songs]                           [Playlists]                           [Search]
  (List of tracks)                        (List of playlists -> Tracks)         (Text input + Results)
```

1. **Top Tabs:** In the Spotify view, tabs along the top header:
   - `Liked Songs` (Bài hát đã thích)
   - `Playlists` (Danh sách phát)
   - `Albums` (Album đã lưu)
   - `Search` (Tìm kiếm)
2. **Track Context Actions (Press 'X' or 'Start'):**
   - `Play` (Phát ngay)
   - `Play Next` (Phát tiếp theo)
   - `Like / Unlike` (Thêm vào / Xóa khỏi yêu thích - syncs with Spotify cloud)
   - `Save to Local Library` (Tải bài hát về thư mục nhạc nội bộ `Music/Spotify`)
3. **Search View:**
   - Uses on-screen virtual keyboard navigable with D-Pad, or physical keyboard on PC.
   - Shows Top Results, Tracks, Albums, and Artists.
4. **Lyrics Architecture Hook:**
   - Track metadata model includes `LyricsID` / `HasSyncedLyrics bool`.
   - Prepares track state for lyrics fetching (LRCLIB or Spotify color-lyrics provider) so the lyrics rendering layer can hook directly into `internal/ui/` in subsequent updates.

---

## 7. TrimUI Brick Pro & muOS Hardware Optimizations

### 7.1 Display, Backlight & Pocket Mode
- **Display Resolution:** 1024×768 (4:3 aspect ratio, 3.2" IPS).
- **QR Code Scaling:** Modify `internal/ui/qr.go` to compute module scaling dynamically:
  $$\text{scale} = \max\left(2, \min\left(\frac{W}{qrSize + 8}, \frac{H}{qrSize + 8}\right)\right)$$
  With 4 modules of quiet zone border, ensuring 100% scan success on compact 3.2" screens.
- **Backlight Blanking:**
  - When entering Pocket Mode / Sleep (`EnterSleep`):
    - Call `/opt/muos/script/device/bright.sh 0`.
    - Check `/sys/kernel/debug/dispdbg` (Allwinner A133P display debug node) to power down the panel driver if present.
  - On `ExitSleep`: Restore previous brightness level saved before blanking.

### 7.2 RGB LED Animator Subsystem
TrimUI Brick Pro features RGB accent LEDs behind shoulder buttons and front controls:
- **Sysfs Paths:**
  - `/sys/class/led_anim/max_scale`: Global brightness scale (0 to 255).
  - `/sys/class/led_anim/effect_enable`: Toggle animator daemon.
  - `/opt/muos/script/device/led.sh`: muOS LED control script.
- **Sleep Behavior:**
  - Upon entering Sleep, Audiofilus reads current `max_scale`, stores it, and writes `0` to `/sys/class/led_anim/max_scale`.
  - Disables LED effects to eliminate unwanted light leak in pockets and conserve battery.
  - Upon wake-up, restores previous `max_scale` and reactivates LED effects.

### 7.3 Hardware Fn Switch (Function Switch)
- TrimUI Brick Pro has an external sliding Fn toggle switch.
- On Linux/muOS, this is exposed as an input event (`SW_LID` or `KEY_FN` on `/dev/input/by-path/*` or `gpio-keys`) or a sysfs switch node (`/sys/class/switch/fn/state`).
- **Functionality:**
  - When Fn switch is engaged: Instantly trigger Pocket Sleep (blank display, turn off LEDs, switch CPU governor to `powersave`, enable double-click shoulder shortcuts `L1`/`R1` for track skipping).
  - When Fn switch is disengaged: Wake system, restore display and LED brightness.

### 7.4 Haptic Vibration & Audio Routing
- **Rumble:**
  - Route through `/opt/muos/device/config/board/rumble`, `/sys/class/timed_output/vibrator/enable`, or evdev `FF_RUMBLE`.
  - Short 40ms buzz confirms pocket actions (play/pause, next track).
- **Volume:**
  - Sync with muOS volume script `/opt/muos/script/device/audio.sh` and fallback to ALSA codec `digital volume` on Allwinner A133P.
- **Packaging:**
  - Update `muos/audiofilus/mux_launch.sh` with `AUDIOFILUS_DEVICE="TrimUI Brick Pro"`.
  - Package binary and required dynamic libraries (`libfdk-aac.so.2`) into `dist/audiofilus.muxapp`.

---

## 8. Error Handling & Offline Resilience

| Failure Scenario | Mitigation |
|---|---|
| Wi-Fi disconnected while playing | Stream falls back to local disk cache (`data/cache/spotify/songs/`). If unbuffered, pause gracefully and display "Offline - playing cached items". |
| Access token expired mid-session | Background worker proactively refreshes token via `refresh_token`. UI operations retry automatically after refresh. |
| Spotify returns HTTP 429 | Request interceptor captures `Retry-After`, enters backoff with randomized jitter, and retries without user error toast. |
| Spotify Free track cannot be matched on YTM | Log matching failure, notify user with subtle status line indicator ("Track unavailable on fallback provider"), and skip to next track in queue. |
| Brick Pro Fn switch toggled repeatedly | Debounce switch state changes with a 300ms refractory period to avoid rapid display on/off cycles. |

---

## 9. Verification & Testing Plan

### 9.1 Automated Tests
- `internal/spotify/ratelimit_test.go`:
  - Mock HTTP transport returning sequential 429 statuses with `Retry-After: 1`. Verify backoff timing and eventual success.
- `internal/spotify/cache_test.go`:
  - Verify Delta Sync: request with identical `snapshot_id` produces zero track fetch requests; modified `snapshot_id` updates track cache.
- `internal/spotify/auth_test.go`:
  - Test PKCE code generation, state verification, and token exchange parsing.
- `internal/platform/muos_test.go`:
  - Mock sysfs files (`max_scale`, `bright.sh`, `dispdbg`, Fn switch) to verify sleep enter/exit state restoration.
- `internal/decode/vorbis_test.go`:
  - Test decryption of AES-128-CTR audio stream fixture and header stripping.

### 9.2 Build Verification
- Compile without CGO for Linux arm64:
  ```sh
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/audiofilus
  ```
- Windows verification:
  ```sh
  GOOS=windows go vet ./...
  ```
- Code formatting and vet:
  ```sh
  gofmt -l .
  go vet ./...
  go test ./...
  ```

### 9.3 Manual & Hardware Testing
- Test QR code scan from iOS/Android mobile camera at various brightness levels on 3.2" screen.
- Verify playback continuity with Wi-Fi toggled on/off.
- Measure battery consumption on TrimUI Brick Pro over a 1-hour pocket mode playback session (target: < 5% drop).
