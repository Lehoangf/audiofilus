---
name: spotify-brick-pro
description: Develop and test the Spotify subsystem of this Audiofilus fork on the TrimUI Brick Pro (muOS): QR/login portal, rate limits and snapshot sync, Premium decrypt/decode, Free yt-dlp fallback, cache layout and socket commands.
---

# Spotify on the Brick Pro

The subsystem lives in `internal/spotify/` and is wired into the service in
`internal/daemon/spotify.go`; the screen is `internal/ui/spotify.go`. Tracks
travel as `spotify://track/<id>` through the normal queue and engine.

| File | Responsibility |
|---|---|
| `auth.go` | PKCE login portal, token storage/refresh, `data/spotify.json` |
| `transport.go` | Token bucket (3 req/s, burst 5), `Retry-After` backoff, singleflight |
| `catalog.go` | Playlists/liked/albums/search, delta sync by `snapshot_id`, `catalog.json` |
| `stream.go` | Stream resolver: Premium CDN or yt-dlp fallback, decrypt, cache, prefetch |
| `manager.go` | Glue for the daemon: metadata, `Source`, last error |
| `daemon/spotify.go` | Socket commands (`spotify …`) |
| `ui/spotify.go` | Tabs, QR screen, track actions |

## Configuration and state

- `audiofilus.json` → `spotify.client_id` / `spotify.client_secret`
  (optional; the portal can save them).
- `data/spotify_client.json` — client ID/secret saved by the portal.
- `data/spotify.json` — access/refresh token, expiry and `product`
  (`premium` / `free`). Never commit or log this file.
- `data/cache/spotify/catalog.json` — playlists, liked songs and snapshot IDs.
- `data/cache/spotify/songs/` — decrypted/streamed audio, covers, and
  `spotify_yt_map.json` (Spotify track → YouTube ID). Eviction: 500 MB or
  50 tracks, whichever comes first.
- `yt-dlp/` next to the binary (unpacked), a single `yt-dlp` file, or `PATH`,
  in that order (`internal/ytm/ytdlp.go`). It must be the glibc aarch64 build.

## Login (QR portal)

`audiofilus spotify login` starts an HTTP portal and returns its URL:

```sh
./audiofilus.aarch64 spotify login
# {"login_url":"http://192.168.x.x:8888/"}
```

The UI shows that URL as a QR code. On the phone (same Wi-Fi), open it, enter
the Spotify **Client ID** (and secret if the app has one), press authorize, log
in, then paste the final `http://127.0.0.1:.../callback?code=...` URL into the
manual field. The console binds `0.0.0.0:8888` (random port if 8888 is busy).

In the Spotify Developer Dashboard, the app's **Redirect URIs** must include:

```text
http://127.0.0.1:8888/callback
http://127.0.0.1/callback
```

Spotify rejects LAN IP redirects; that is why the phone's `127.0.0.1` page is
pasted back instead of reaching the console directly. If 8888 is occupied, the
random port will not match the dashboard entry: free port 8888 first.

## Socket commands

```sh
./audiofilus.aarch64 spotify status
./audiofilus.aarch64 spotify playlists
./audiofilus.aarch64 spotify playlist_tracks <playlist-id>
./audiofilus.aarch64 spotify liked
./audiofilus.aarch64 spotify albums
./audiofilus.aarch64 spotify recent
./audiofilus.aarch64 spotify search "query"
./audiofilus.aarch64 spotify track <track-id>
./audiofilus.aarch64 spotify cached <track-id>
./audiofilus.aarch64 spotify download <track-id>
./audiofilus.aarch64 play 'spotify://track/<track-id>'
./audiofilus.aarch64 status -json      # spotify_err = last resolution error
```

Each `spotify` subcommand is handled by `handleSpotify`; a new one must be
added there (the CLI forwards unknown commands unchanged).

## Premium and Free paths

- **Premium:** fetch the CDN stream, throw away the proprietary 167-byte header,
  decrypt with AES-128-CTR (zero IV) and decode the Ogg Vorbis in pure Go
  (`DecryptSpotifyOgg`). Cache the result so replays are local.
- **Free (or Premium fallback):** resolve ISRC/title/artist to a YouTube Music
  track with yt-dlp and cache the `.m4a`. Downloads are serialized
  (`downloadSem`) and prefetch is cancelled when the user asks for a foreground
  track.
- A `403` from YouTube often means IPv6 or missing cookies. `cookies.txt`
  (Netscape format) next to `yt-dlp` or one folder up is picked up by
  `cookiesFile`.
- If `/tmp` is small, set `TMPDIR=<app>/data/tmp` when using a one-file
  yt-dlp; the muOS launcher creates that folder but does not export it.

## Rate limits: do not "fix" them by removing the limiter

`transport.go` caps requests at 3/s with burst 5, coalesces identical GETs with
singleflight, and honors `Retry-After` with jitter. A 429 means something is
bypassing these paths (a new direct `http.Get`, a loop over playlists without
snapshot checks) or prefetch is too aggressive. Check `data/audiofilus.log` for
`spotify:` lines and `spotify_err` in `status -json`, then fix the caller.

## Tests

```sh
go test ./internal/spotify ./internal/daemon
go test -v -run 'TestRateLimiterAndBackoff|TestSingleflightCoalescing' ./internal/spotify
go test -v -run 'TestDeltaSyncPlaylist|TestLikedSongs' ./internal/spotify
go test -v -run 'TestDecryptSpotifyOgg|TestOfflineCacheRetrieval|TestYTMapCachePersistence' ./internal/spotify
```

They use `httptest` and synthetic data: no Spotify account, no network and no
real music are needed. Add a test with the fix instead of relying on the
console.

## Checklist on the console

1. `spotify status` says `logged_in: true` and the right `product`.
2. Playlists and liked songs load; the second load is served from
   `catalog.json` (check the file's mtime does not change and the log shows no
   API storm).
3. A Premium track plays from the CDN; replaying it plays from
   `data/cache/spotify/songs/` with Wi-Fi off.
4. A Free track resolves through yt-dlp; the second play uses
   `spotify_yt_map.json` and does not search again.
5. `sleep on` keeps the stream playing; `sleep off` resumes the screen.
6. `status -json` has an empty `spotify_err`.

Do not test with the author's real tokens in the repo; delete `data/spotify.json`
before sharing a build, and never commit `cookies.txt`.
