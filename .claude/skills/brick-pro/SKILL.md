---
name: brick-pro
description: Develop, build and install this Audiofilus fork (Spotify + TrimUI Brick Pro) on muOS. Device facts, build targets, the .muxapp/SSH/microSD/PortMaster install routes, display and UI options, and which companion skill to use.
---

# Audiofilus on the TrimUI Brick Pro (muOS)

This is the fork at `github.com/Lehoangf/audiofilus`: upstream Audiofilus plus
Spotify (`internal/spotify/`, the Spotify screen in `internal/ui/spotify.go`)
and Brick Pro hardware handling. The target device is a **TrimUI Brick Pro
running muOS**; the PC is only for tests and screenshots.

## What the device forces on the code

| Fact | Consequence |
|---|---|
| Allwinner A133P, arm64, 1 GB LPDDR3, glibc | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`; never build musl |
| 1024×768 4:3, 60 Hz IPS | UI is 320×240 scaled up; no resolution setting |
| 5000 mAh battery | Pocket mode must stay cheap; measure, don't guess |
| PipeWire without the PulseAudio protocol | oto falls back to ALSA → PipeWire; `XDG_RUNTIME_DIR=/run` |
| muOS hotkeys (`muhotkey`, `idle.sh`, `bright.sh`) | The service creates `/run/muos/caffeine` and pulses `/run/muos/input_activity` |
| exFAT SD card | The control socket lives in `/tmp`, never next to the binary |
| muOS `pgrep -f` closes apps by foreground process | `mux_launch.sh` starts the UI in the background and registers its PID |

Hardware details (buttons, LEDs, Fn switch, PMIC) are in the
[`trimui-brick-pro`](../trimui-brick-pro/SKILL.md) skill. Platform code is
`internal/platform/muos.go`; what each muOS script replaces is in
`docs/muos.md` and `docs/trimui-brick-pro.md`.

## Where the app lives on the console

Do not assume a path: discover it once and export it in your shell.

```sh
find /mnt -maxdepth 5 -type d -name 'audiofilus*' 2>/dev/null
```

- Installed as a muOS application: a folder under `MUOS/application`,
  usually `/mnt/mmc/MUOS/application/audiofilus-spotify`.
- Installed through PortMaster: usually `/mnt/mmc/ports/audiofilus`.

Both layouts contain `audiofilus.aarch64` (UI + service + CLI), `mux_launch.sh`
when it is the muOS app, `audiofilus.json`, `data/` and optionally `yt-dlp/`
and `libs.aarch64/`. The UI's own log is `log.txt`; the service's is
`data/audiofilus.log`.

The launcher exports `AUDIOFILUS_HOME=<app dir>`, `AUDIOFILUS_HANDHELD=1`,
`AUDIOFILUS_SOCKET=/tmp/audiofilus.sock`, `EBITENGINE_UI_BACKEND=sdl2`,
`XDG_RUNTIME_DIR=/run`, `LD_LIBRARY_PATH=<app>/libs.aarch64`, and on this
hardware `AUDIOFILUS_DEVICE="TrimUI Brick Pro"`. Running the binary by hand
over SSH without them skips the LEDs, the device name and possibly PipeWire.

## Build

```sh
gofmt -l . && go vet ./... && go test ./...                  # always first
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /dev/null ./cmd/audiofilus
GOOS=windows go vet ./...
```

`./build.sh` produces the PC, arm64 and Windows binaries, `dist/audiofilus.zip`
(PortMaster) and `dist/audiofilus.muxapp` (stock name). For the Spotify muxapp
see the packaging section below.

Never use **Settings → Update** on this fork: `internal/update/update.go` still
points at `juanvillacortac/audiofilus`, so it would replace the fork with
upstream and remove Spotify. Change `update.Repo` before shipping self-updates.

## Install routes

### 1. muOS application (.muxapp) — for users

The fork stages the app by hand in `dist/muxapp_spotify/audiofilus-spotify/`
(`build.sh` does not create this tree). Refresh what changed and pack it:

```sh
# arm64 binary + launcher
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" \
  -o dist/audiofilus-spotify.aarch64 ./cmd/audiofilus
cp dist/audiofilus-spotify.aarch64 dist/muxapp_spotify/audiofilus-spotify/audiofilus.aarch64
cp muos/audiofilus/mux_launch.sh dist/muxapp_spotify/audiofilus-spotify/mux_launch.sh

# pack (tools/pack_muxapp zips the whole dist/muxapp_spotify)
rm -rf dist/muxapp_spotify/audiofilus        # stale sibling, if present
go run ./tools/pack_muxapp                   # -> dist/audiofilus-spotify.muxapp
```

Then either copy the `.muxapp` to the SD card's archive folder and install it
with muOS's Archive Manager, or upload it over SSH/SFTP and run the Archive
Manager. The app appears under **Applications**, not Ports. The folder keeps
`data/` and `audiofilus.json`, so reinstalling over it must not wipe them.

### 2. SSH development loop — for development

Build the arm64 binary, upload it, replace atomically and restart. The full
procedure (password/key, atomic `mv`, logs, cleanup) is in
[`test-on-brick-pro`](../test-on-brick-pro/SKILL.md).

### 3. microSD, Samba, SFTP

- **microSD**: fastest for music libraries; eject the card, copy music and the
  app folder from the PC, put it back. Do this only with the console off.
- **Samba**: enable it in muOS network settings and open `\\<console-ip>\` (or
  `\\muos\`) from Windows.
- **SFTP/SCP**: port 22, user `root`, muOS's password (default `root`). Use it
  for single files and `.muxapp` archives, not for a whole library.

### 4. PortMaster port

`go run ./tools/pack_port` builds `dist/audiofilus-portmaster.zip` from
`dist/muxapp_spotify/audiofilus-spotify` and `portmaster/`. Use it only when
the target must show up under Ports; the muOS app is the normal route here.

## Display and UI options

- The UI renders at its 320×240 canvas and scales to the 1024×768 panel; there
  is no in-app resolution or scaling control. Aspect is fixed at 4:3.
- **Settings → screen** (all saved in `audiofilus.json` under `ui`):
  `sleep_mode` (`backlight` turns the panel off, `oled` only paints black),
  `theme_color`, `orientation` (`""`, `left`, `right`), `vertical_controls`
  (`""` or `one`), language (`language`; this fork adds `vi` in
  `internal/ui/lang_vi.go`) and `font`.
- **Sleep/pocket**: double `START` or the power button; `audiofilus sleep on`
  from SSH. Screen, LEDs and CPU governor are handled by `muos.go`. Always
  leave a way out (`sleep off` or `START`) and warn before turning it on.
- **Screenshots/GIFs**: render them on the PC, not on the console:
  `dist/audiofilus -shots /tmp/s -music <dir> -lang en` (add `-frames` for
  animations). Visible UI changes should also refresh the site/port images
  with the `refresh-media` skill.
- **Vietnamese text**: the font must contain the glyphs; this fork ships
  Vietnamese-capable fonts under `internal/ui/assets/fonts/` and the UI font
  can be changed in Settings. Check a long Vietnamese title at 1024×768 before
  calling it done.

## Definition of done

`gofmt -l .` empty, `go vet ./...` and `go test ./...` green, arm64 builds
without cgo, `GOOS=windows go vet ./...` green, and the change was tried on the
PC with real audio before touching the console. On the console, verify launch
from the muOS menu, background playback, sleep/wake, controller, audio through
speakers and headphones, and that `quit` leaves no `/run/muos/caffeine`,
`powersave` governor or stopped UI processes behind.

## Companion skills

| Skill | When |
|---|---|
| [`test-on-brick-pro`](../test-on-brick-pro/SKILL.md) | Deploy a build over SSH, run/stop the service, read logs |
| [`debug-muos-brick-pro`](../debug-muos-brick-pro/SKILL.md) | Audio, sleep, backlight, LEDs, lid, power button, controller |
| [`measure-power-brick-pro`](../measure-power-brick-pro/SKILL.md) | CPU and battery drain, pocket-mode comparisons |
| [`spotify-brick-pro`](../spotify-brick-pro/SKILL.md) | Login, 429s, Premium/Free streaming, cache, yt-dlp |
| [`pack-release-brick-pro`](../pack-release-brick-pro/SKILL.md) | Build the muxapp/PortMaster zip and cut a release |
| [`trimui-brick-pro`](../trimui-brick-pro/SKILL.md) | Hardware reference: ports, buttons, LEDs, PMIC |
