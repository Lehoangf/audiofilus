---
name: debug-muos-brick-pro
description: Diagnose Audiofilus on the TrimUI Brick Pro (muOS): sleep/caffeine, idle mute, backlight, LEDs, governor, PipeWire/ALSA audio, headphones, lid, HDMI, power button, controller and the control socket.
---

# Debugging the muOS integration on the Brick Pro

The code is `internal/platform/muos.go`; the reference for what each muOS
script replaces is `docs/muos.md`. Everything here is read-only unless marked
otherwise.

## First five minutes

```sh
cd <app dir>                       # /mnt/mmc/MUOS/application/audiofilus-spotify, usually
./audiofilus.aarch64 status -json
tail -n 120 data/audiofilus.log
tail -n 60 log.txt                 # launcher output (only if opened from the menu)
ps -ef | grep '[a]udiofilus'
```

`status -json` is the fastest truth: `platform` must be `muos`, `awake` must be
`true` while playing, `lid`, `docked`, `output`, `pad`, `sleep` and
`spotify_err` tell you which subsystem to look at next. In the log, the startup
lines say which platform, controller, power button and output were detected.

If the binary was started by hand, it may be missing the launcher environment.
Re-run it the way `mux_launch.sh` does:

```sh
cd <app dir>
AUDIOFILUS_HOME="$PWD" AUDIOFILUS_HANDHELD=1 EBITENGINE_UI_BACKEND=sdl2 \
XDG_RUNTIME_DIR=/run AUDIOFILUS_SOCKET=/tmp/audiofilus.sock \
LD_LIBRARY_PATH="$PWD/libs.aarch64" ./audiofilus.aarch64 status
```

## muOS mechanisms, one by one

| What | How to check it | Expected |
|---|---|---|
| Platform detected | `test -f /opt/muos/script/device/bright.sh` | exists |
| No suspend | `test -e /run/muos/caffeine` while playing | exists |
| No idle dim/mute | `a=$(cat /run/muos/input_activity); sleep 4; b=$(cat /run/muos/input_activity); echo "$a $b"` | values differ |
| Screen off in sleep | `./audiofilus.aarch64 sleep on` then read the panel and `cat /sys/class/led_anim/max_scale` | screen dark, `max_scale` = `0` |
| Governor in sleep | `cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor` | `powersave` (unless `governor` is `keep`) |
| Wake | `./audiofilus.aarch64 sleep off` | screen lit again, LEDs back, governor restored |
| System volume | `/opt/muos/script/device/audio.sh I` | `75%` (or the current value) |
| Wired headphones | `ls /run/muos/headphones` | exists only with the jack plugged |
| Bluetooth | `wpctl inspect @DEFAULT_AUDIO_SINK@ \| grep bluez` | matches only on a BT sink |
| Lid | `p=$(cat /opt/muos/device/config/board/hall); cat "$p"` | `0` when closed |
| Rumble | `p=$(cat /opt/muos/device/config/board/rumble); echo 1 > "$p"; sleep 0.3; echo 0 > "$p"` | the motor pulses |
| External display | `cat /sys/class/extcon/hdmi/cable.0/state` (or DRM `card*-*/status`) | `1` / `connected` when docked |
| Socket | `ls -l /tmp/audiofilus*.sock` | one live socket; remove it only if no service owns it |

The service creates/removes `/run/muos/caffeine` and writes
`/run/muos/input_activity` (every 3 s) itself; if either is missing while
`status` says `awake: true`, the service is not running the muOS platform (look
at `platform` in the JSON) or crashed after creating it.

## Audio is silent or goes to the wrong place

muOS is PipeWire **without** the PulseAudio protocol: oto tries Pulse and
falls back to ALSA, whose default output (`/etc/asound.conf`) feeds PipeWire.
It needs `XDG_RUNTIME_DIR=/run`; the launcher sets it.

```sh
cat /proc/asound/cards
cat /proc/asound/pcm
XDG_RUNTIME_DIR=/run wpctl status 2>/dev/null | head -40
/opt/muos/script/device/audio.sh I
```

- `output` in `status -json` must match reality (`speaker`, `wired`,
  `bluetooth`, or empty when unknown). An empty value means `wpctl` is missing
  and `/run/muos/headphones` did not decide it.
- If it goes silent after a while with the screen dimmed, the idle watcher is
  muting it because `input_activity` stopped advancing, or `idle_mute` is on
  with no player activity. Check the two files in the table above first.
- `muhotkey` owns the volume buttons; Audiofilus only reads and shows the
  volume. Above 100 % the app raises it itself.

## The screen will not come back

`bright.sh` writes the saved brightness only when the value differs, so waking
uses a two-step nudge (`bright.sh <saved-1>` then `bright.sh <saved>`); the code
does the same as muOS's own resume. If it is still dark:

```sh
cat /opt/muos/config/settings/general/brightness
/opt/muos/script/device/bright.sh 0
/opt/muos/script/device/bright.sh <saved brightness>
```

Check the real panel state in `/sys/class/disp/disp/attr/sys`
(`backlight(  0)` means off) where the kernel exposes it. If the service died
while asleep, restore the LED value and governor by hand (the values saved in
`muos.go` are lost with the process).

## Buttons, controller and lid

- `./audiofilus.aarch64 pad` prints the SDL mapping. On muOS the controller may
  be its virtual **muOS-Keys** device; the service logs which one it found.
- The **power button** is shared with `muhotkey`. If a tap does not enter sleep,
  the hotkey daemon grabbed it; the console still does not suspend because
  `caffeine` exists. Look for `KEY_POWER` in the log.
- The **Fn switch** (`/sys/class/switch/fn/state`) has a debounced watcher in
  `muos.go` (`newFnSwitchWatcher`), but it is **not connected to any action**
  yet. Do not report it as working pocket mode.
- The lid goes through `board/hall`; with an external display connected the
  UI stays on (`docked` in the status).

## The app closes as soon as it opens

muOS closes a foreground application with `pgrep -f <foreground_process>`. The
launcher starts the UI in the background and registers its **PID**
(`SET_VAR system foreground_process "$UI"`); with PortMaster the launcher is
different. If it closes instantly, check `log.txt`, whether a stale
`foreground_process` points at the service, and whether the app was installed
as a muOS app but launched as a port (or the reverse).

## Leaving it clean

`./audiofilus.aarch64 quit` must remove `/run/muos/caffeine`, restore the
governor, the backlight and `max_scale`, resume any stopped UI process and
close the socket. The full verification block is in
[`test-on-brick-pro`](../test-on-brick-pro/SKILL.md#after-stopping-nothing-may-be-left-modified).

## Do not

- Do not use **Settings → Update**: it points at upstream Audiofilus.
- Do not `pkill -f` a string that also appears in your own SSH command.
- Do not leave pocket mode on, LEDs at 0 or the governor at `powersave` after a
  test: warn the author and turn it off.
