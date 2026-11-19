---
name: pack-release-brick-pro
description: Package and release this Audiofilus fork for the TrimUI Brick Pro (muOS): build the arm64 binary, stage dist/muxapp_spotify, pack audiofilus-spotify.muxapp and the PortMaster zip, verify permissions and secrets, and tag only when asked.
---

# Packaging the Spotify fork

The fork's user-facing artifact is **`dist/audiofilus-spotify.muxapp`**, built
from `dist/muxapp_spotify/audiofilus-spotify/` by
`go run ./tools/pack_muxapp`. `build.sh` does **not** create that staging tree
and its own `dist/audiofilus.muxapp` uses the stock `audiofilus/` folder. Do not
confuse the two.

## Preflight

```sh
gofmt -l .                       # must be empty
go vet ./...
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/audiofilus
GOOS=windows go vet ./...
git status --short
```

- `internal/ui/lang_vi.go` and the Vietnamese-capable fonts are part of the
  fork: make sure they are tracked, not left untracked.
- Do not commit `dist/` (ignored), binaries under `muos/audiofilus/`
  (`audiofilus`, `audiofilus.aarch64`), `data/`, `audiofilus.json`,
  `data/spotify.json`, `cookies.txt` or real music.
- The repo is public: scan the diff for tokens, personal paths and the
  author's library names.

## Build the muOS application

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -ldflags="-s -w -X github.com/juanvillacortac/audiofilus/internal/version.Version=${VERSION:-dev}" \
  -o dist/audiofilus-spotify.aarch64 ./cmd/audiofilus

cp dist/audiofilus-spotify.aarch64 dist/muxapp_spotify/audiofilus-spotify/audiofilus.aarch64
cp muos/audiofilus/mux_launch.sh    dist/muxapp_spotify/audiofilus-spotify/mux_launch.sh

# the packer zips the whole folder: remove the stale sibling first
rm -rf dist/muxapp_spotify/audiofilus
go run ./tools/pack_muxapp        # -> dist/audiofilus-spotify.muxapp
```

The staging folder also carries `glyph/`, `licenses/`, `yt-dlp/`,
`libs.aarch64/` and `data/spotify_client.json`. Refresh `libs.aarch64/` from
`build/libgme`, `build/libvgm`, `build/fdk-aac` and `build/ffmpeg` when those
were rebuilt; a missing `libfdk-aac.so.2` means AAC radio and M4A will not
play on muOS.

## Build the PortMaster zip

`tools/pack_port` reads `dist/muxapp_spotify/audiofilus-spotify` and
`portmaster/`, and writes `dist/audiofilus-portmaster.zip`:

```sh
go run ./tools/pack_port
```

Use it only when the release must be installable under **Ports**; the muxapp is
the normal Brick Pro route. `build.sh` still produces the stock
`dist/audiofilus.zip` from `portmaster/` plus `dist/audiofilus-*`; do not mix
the two outputs.

## Verify the archive

```sh
unzip -l dist/audiofilus-spotify.muxapp | head -40
unzip -Z -l dist/audiofilus-spotify.muxapp | grep -E 'mux_launch|audiofilus.aarch64|\.so'
```

- One app folder (`audiofilus-spotify/`), not two variants.
- `mux_launch.sh`, `audiofilus.aarch64`, `yt-dlp` and every `.so` are `-rwxr-xr-x`.
- `glyph/audiofilus.png` exists (the `ICON:` header names it) and the icon
  filename matches.
- No `data/spotify.json`, no `data/cache`, no music, no `cookies.txt`.
- `LICENSE*` files are present for audiofilus, the fonts and each bundled C
  library.

## Version and release

- The version comes from `-ldflags .../internal/version.Version`; `VERSION=vX.Y.Z ./build.sh`
  sets it for the stock artifacts, the command above sets it for the staging
  binary. Without it the app reports `dev` and offers no updates.
- **Self-update still targets upstream**: `internal/update/update.go` has
  `Repo = "juanvillacortac/audiofilus"`. Either change it to
  `Lehoangf/audiofilus` (and publish compatible assets) or leave the fork
  without self-update. Never tell a user to press **Settings → Update** before
  this is fixed.
- `.github/workflows/release.yml` currently copies `dist/audiofilus.muxapp` and
  `dist/audiofilus.zip` into the release. For this fork, update it to
  `dist/audiofilus-spotify.muxapp` and `dist/audiofilus-portmaster.zip`, or
  attach those two files manually.
- Tag and push **only when the author asks** (`release` skill has the exact
  `git tag`/`gh run` flow). After publishing, install the downloaded
  `.muxapp` on the console once (Archive Manager) and smoke-test it; do not
  trust a zip that was never installed.
- CI (`.github/workflows/ci.yml`) already runs gofmt, vet, tests, a 32-bit test
  pass and the arm64 no-cgo build; it does not build the muxapp.
