---
name: measure-power-brick-pro
description: Measure Audiofilus CPU and battery drain on the TrimUI Brick Pro (Allwinner A133P, 5000 mAh, muOS), verify pocket mode, and compare before/after an optimization without mixing it with the RG34XX-SP numbers.
---

# Measuring power on the TrimUI Brick Pro

The Brick Pro is **not** the RG34XX-SP: 5000 mAh instead of ~3300, A133P
instead of H700. Keep its numbers in their own `docs/power.md` section and
never compare them with the Knulli table.

Record every measurement with the date, what was on screen, the governor, the
volume, Wi-Fi on/off, and whether the music was local, Spotify cached or
Spotify streaming.

## CPU of the service or the UI

`ps %CPU` is the average since the process started and is useless here.
Measure the tick difference over an interval:

```sh
ps -eo pid,comm,args | grep '[a]udiofilus'     # identify the service (args: audiofilusd daemon) and the UI
P=<pid>
HZ=$(getconf CLK_TCK)
a=$(awk '{print $14+$15}' /proc/$P/stat); sleep 20; b=$(awk '{print $14+$15}' /proc/$P/stat)
echo "$(( (b-a)*100/(20*HZ) ))% of one core"
```

- Measure the **service** for playback cost and the **UI** for drawing cost;
  both share the executable name `audiofilus.aarch`, so pick by `args`.
- The same loop over `/proc/$P/task/*/stat` splits the cost by thread
  (decoder, resampler, output). `audiofilus bench <file>` on the PC separates
  the decoding stages, but it is not an A133P measurement.
- Pocket mode should show the service near idle with a stable buffer; if it
  climbs, check for a busy prefetch, a visualizer still drawing, or the UI not
  actually suspended.

## Battery

Discover the PMIC node first; do not assume the Knulli path:

```sh
for d in /sys/class/power_supply/*; do
  echo "== $d"; for f in capacity status time_to_empty_now voltage_now current_now; do
    [ -r "$d/$f" ] && echo "$f=$(cat "$d/$f")"
  done
done
```

- **Quick (2–3 min):** with `status` = `Discharging`, read `capacity` and
  `time_to_empty_now` every 10 s. With a 5000 mAh cell,
  `mA ≈ capacity/100 * 5000 / (time_to_empty_now/3600)`. It jumps around and
  takes minutes to settle: average at least five readings.
- If `time_to_empty_now` does not exist, use the **reliable** method:
  **30–60 min**, note the time of each `capacity` change (1 % ≈ 50 mAh on a
  5000 mAh cell) and report minutes per 1 % → %/h.
- Unplug the charger and wait a few minutes: the estimate stays frozen after
  charging. USB data (the other port) also disturbs the reading.
- Start a logger with `nohup setsid` so it survives the SSH session.

## Prove the screen is really off

`max_scale` = 0 only proves the decorative LEDs are off. Look for the panel
backlight:

```sh
grep -r . /sys/class/backlight/*/brightness 2>/dev/null
cat /sys/class/disp/disp/attr/sys 2>/dev/null       # backlight(  0) = off
cat /sys/class/led_anim/max_scale 2>/dev/null
cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
```

`./audiofilus.aarch64 sleep on` should leave the panel dark, `max_scale` at 0
and the governor at `powersave` (unless `sleep.governor` is `keep`). Compare
the same music with sleep on and off; that difference is what pocket mode buys.

## Frames

`strace` may not be installed on muOS. Before reaching for it, measure CPU as
above; on the PC, `AUDIOFILUS_CPUPROFILE=<file>` writes a 30 s CPU profile of
the UI after 10 s of startup (`cmd/audiofilus/ui.go`). The panel's 60 Hz is not
proof that the UI draws 60 FPS.

## Reporting

Add a **TrimUI Brick Pro (muOS)** subsection to `docs/power.md`:

```text
| Date | Build | Situation (screen, governor, volume, source) | CPU (1 core) | Drain |
```

State the exact conditions next to every number, and never edit the existing
RG34XX-SP rows to make a comparison look better.
