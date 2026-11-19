---
name: test-on-brick-pro
description: Install and test this Audiofilus fork on the TrimUI Brick Pro (muOS) over SSH/SFTP at muos.local. Upload a binary atomically, start or stop the service, read its log, test the gamepad, and verify nothing was left behind.
---

# Testing Audiofilus on the TrimUI Brick Pro (muOS)

The console answers at `muos.local` (or its LAN IP), user `root`, with muOS's
SSH password (default `root`, or whatever the author set). Windows has OpenSSH
(`ssh`, `scp`); Git Bash also has `ssh-copy-id`.

**Set up a key once** so no password is typed or stored:

```powershell
type $env:USERPROFILE\.ssh\id_ed25519.pub | ssh root@muos.local "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 700 ~/.ssh && chmod 600 ~/.ssh/authorized_keys"
```

If `muos.local` does not resolve, the console's Wi-Fi is asleep or on another
network: ask the author to wake it. Do not retry in a loop. Never put the
password or a private key in the repo.

## Find the app folder

```powershell
$H = "root@muos.local"
ssh $H 'find /mnt -maxdepth 5 -type d \( -name "audiofilus-spotify" -o -name "audiofilus" \) 2>/dev/null'
```

Set `$D` to the result, for example
`/mnt/mmc/MUOS/application/audiofilus-spotify` or
`/mnt/mmc/ports/audiofilus`. Confirm the launcher's binary name before
uploading:

```powershell
ssh $H "grep -n 'audiofilus' '$D/mux_launch.sh' | head"
```

The launcher runs `./audiofilus.aarch64`; some copies also contain a second
file named `audiofilus`. Replace the one the launcher actually executes.

## Build and upload

```powershell
gofmt -l .; go vet ./...; go test ./...
$env:CGO_ENABLED = "0"; $env:GOOS = "linux"; $env:GOARCH = "arm64"
go build -trimpath -ldflags="-s -w" -o dist/audiofilus-spotify.aarch64 ./cmd/audiofilus
```

Upload to the **same folder** and rename with `mv`, so a running service is
never overwritten in place:

```powershell
scp dist/audiofilus-spotify.aarch64 "${H}:$D/.audiofilus.aarch64.new"
ssh $H "cd '$D' && chmod +x .audiofilus.aarch64.new && mv -f .audiofilus.aarch64.new audiofilus.aarch64"
```

Check the transfer (both sides must match):

```powershell
(Get-FileHash dist/audiofilus-spotify.aarch64 -Algorithm MD5).Hash
ssh $H "md5sum '$D/audiofilus.aarch64'"
```

The folder holds `audiofilus.json` and `data/` (history, cache, Spotify
credentials, yt-dlp downloads). Do not delete or replace the folder to
"reinstall"; update files inside it.

## Start, inspect, stop

```powershell
ssh $H "cd '$D' && ./audiofilus.aarch64 quit"                 # unload the old binary
ssh $H "cd '$D' && ./audiofilus.aarch64 play /path/to/music"  # starts the detached service
ssh $H "cd '$D' && ./audiofilus.aarch64 status"
ssh $H "cd '$D' && ./audiofilus.aarch64 status -json"
ssh $H "tail -n 80 '$D/data/audiofilus.log'"
ssh $H "tail -n 40 '$D/log.txt'"                              # launcher output, if launched from muOS
ssh $H "ps -ef | grep '[a]udiofilus'"                         # not pgrep -f with your own text
```

- `play` starts the service **without** opening the UI, which is what you want
  over SSH. The UI needs the framebuffer: launch it from the muOS **Applications**
  menu, not from an SSH session that would fight the menu.
- The detached service logs to `data/audiofilus.log`; the UI writes `log.txt`.
- `./audiofilus.aarch64 pad` prints the gamepad and its SDL mapping. Run it
  after `quit` (or with the UI closed) so the device is free.

## The checks worth doing after a change

1. Launch from the muOS menu: splash, library, now playing, no black screen.
2. Music keeps playing when the UI is closed or a game is opened.
3. Sleep/pocket: `./audiofilus.aarch64 sleep on`, then `sleep off` (or double
   `START`). Warn the author before leaving it in pocket mode.
4. Controller: D-pad, A/B/X/Y, L1/R1/L2/R2, Start/Select, sticks.
5. Audio: speakers, then wired headphones (unplugging pauses); Bluetooth if
   the change touches output.
6. Spotify: login portal, a Premium track, a Free track through yt-dlp, and
   the same track again from cache (see `spotify-brick-pro`).
7. Power button and lid: sleep and wake; on this device the Fn switch is not
   wired to any action yet.
8. Vietnamese strings and long titles at 1024×768.

Screenshots are rendered on the PC, not captured on the console:

```powershell
dist/audiofilus -shots C:\tmp\shots -music <music-dir> -lang en
```

## After stopping: nothing may be left modified

```powershell
ssh $H "test -e /run/muos/caffeine && echo 'caffeine LEFT' || echo 'caffeine clean'"
ssh $H "cat /sys/class/led_anim/max_scale 2>/dev/null"
ssh $H "cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"
ssh $H "ps -o pid,stat,comm | awk '\$2 ~ /^[Tt]/'"
ssh $H "ls -l /tmp/audiofilus*.sock 2>/dev/null"
```

Expected after a clean `quit`: no `caffeine`, `max_scale` back to its previous
value (not `0`), governor back to the one from before sleep, no stopped
(`T`) UI process, and no stale socket. If the service crashed while asleep,
resume stopped processes (`kill -CONT <pid>`), restore the governor and LEDs,
and remove only the stale socket.

`/var/run/battery-saver` and the bind mounts are **Knulli-only**; do not look
for them on muOS. muOS uses `/run/muos/caffeine` and `/run/muos/input_activity`.

## Cautions

- Do not use **Settings → Update** on this fork: it points at upstream
  `juanvillacortac/audiofilus` and would replace the Spotify build.
- `pkill -f <text>` / `pgrep -f <text>` also match your own SSH command. Match
  the exact process or filter `ps` output.
- Pocket mode turns the panel off and can look like a hung console: always
  know the way out before enabling it.
- Test on the PC with real audio first; only then upload to the console.
- The repo is public: no passwords, personal paths or real music in commits.
