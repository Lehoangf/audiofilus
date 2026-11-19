# TrimUI Brick Pro

The **TrimUI Brick Pro** is a 4:3 handheld powered by the Allwinner A133P SoC, running **muOS** (MustardOS) or stock Linux. This document covers its hardware architecture, physical ports, data transfer protocols, complete button layout, multi-zone RGB LED subsystem, power management, and how Audiofilus integrates with it.

For the corresponding Claude Code developer skill, see [`.claude/skills/trimui-brick-pro/SKILL.md`](../.claude/skills/trimui-brick-pro/SKILL.md).

## Hardware Overview & Physical Ports

| Specification | Details |
|---|---|
| **SoC** | Allwinner A133P (4x ARM Cortex-A53 @ 1.8 GHz) |
| **GPU** | Imagination PowerVR GE8300 |
| **Architecture** | 64-bit ARM (`aarch64` / `arm64`), Linux kernel 5.4/4.9 |
| **Display** | 1024×768 (4:3), 60 Hz IPS LCD |
| **Audio** | Stereo speakers, 3.5mm stereo jack, PipeWire/ALSA |
| **Battery** | Li-Po (~4000–5000 mAh), AXP PMIC sysfs reporting |
| **Storage** | Dual microSD card slots (TF1/TF2), supports up to 256GB+ |

### Port Details & Constraints

- **Top USB Type-C Port (USB Host / OTG)**:
  - Supports high-speed USB 2.0 data communication.
  - Supports connecting USB peripherals via an OTG adapter: external gamepads (for 2-player multiplayer), USB card readers, USB flash drives, wired/wireless keyboards, mice, and external USB DACs.
  - Can connect directly to a PC for file management (MTP / USB Mass Storage Gadget).
- **Bottom USB Type-C Port (Dedicated Charging)**:
  - Dedicated DC 5V/2A power input for charging.
  - **No data transfer** is wired to this port.
  - **No TV-Out / Video Output**: The TrimUI Brick Pro **does not support DisplayPort Alt Mode or TV-Out over Type-C** on either port. Video cannot be output to an external monitor or TV via USB-C.
- **MicroSD Card Slots (TF Slots)**:
  - Holds the OS, applications, and ROM/music libraries.
  - Supports cards from 64GB up to 256GB.
  - Ejecting the microSD card and using a PC card reader is the fastest and most reliable method for transferring large music libraries or game collections.
- **3.5mm Analog Audio Jack**:
  - Standard analog stereo audio output for headphones, IEMs, or auxiliary cables.

## Data Transfer & File Management on muOS

muOS on the TrimUI Brick Pro provides multiple ways to transfer files:

1. **Direct MicroSD Card Reader (Fastest & Recommended for Bulk Data)**:
   - Insert card into PC.
   - Applications go into `MUOS/application/` (e.g. `MUOS/application/audiofilus-spotify/`).
   - Music tracks go into `music/` or `MUOS/music/`.
2. **Top USB Type-C Port (PC Direct Connection & USB OTG)**:
   - Connect the top port to a PC with a data cable. In muOS settings, enable USB Mass Storage / MTP mode to mount partitions on Windows/macOS.
   - Plug in USB flash drives via an OTG adapter to transfer files from `/mnt/usb/`.
3. **Wi-Fi SFTP / SSH**:
   - muOS includes an integrated SSH/SFTP server on port 22.
   - Connect via FileZilla, WinSCP, Cyberduck, or `scp`:
     - Host: `<console-ip>` (or `muos.local`), Port: `22`
     - Username: `root`, Password: `root` (or configured password).
4. **Samba (SMB) Network File Share**:
   - Access the console over LAN via Windows File Explorer: `\\<console-ip>\` or `\\muos\`.
   - Drag and drop files directly without third-party software.
5. **Syncthing / rsync**:
   - Background synchronization of playlists, audio cache, and settings over the local network.

## Controls & Button Mapping

### Front Face
- **D-Pad**: Digital navigation (Up, Down, Left, Right).
- **Face Buttons**: A, B, X, Y in standard diamond layout.
- **Dual Hall Effect Joysticks**: Non-contact magnetic sensors (Hall effect) eliminating potentiometer drift. Includes stick clicks (L3/R3).
- **System Buttons**: Select, Start, and Menu.
- **Function Keys**: F1 and F2 auxiliary buttons with dedicated underneath sub-key illumination.

### Top Edge
- **Bumpers**: L1 and R1 shoulder buttons.
- **Triggers**: L2 and R2 trigger buttons with transparent housings.

### Sides & Bottom
- **Power Button**: Right edge (`KEY_POWER`). Short press toggles sleep/pocket mode. Integrated charging indicator LED.
- **Volume**: Left edge volume rocker (+ / -).
- **Home**: Home navigation key.
- **FN Switch**: 2-position hardware slider switch (`/sys/class/switch/fn/state`). Can be monitored as a hardware pocket mode switch or mute toggle.
- **Reset**: Hardware reset pinhole on bottom edge.

## LED Lighting Subsystem

The console includes multi-zone RGB lighting controlled via the Allwinner animation driver `/sys/class/led_anim/` and `/sys/class/leds/`:

1. **Joystick Rings**: RGB halos around Left and Right analog sticks.
2. **F1 & F2 Sub-Key LEDs**: Dedicated backlights beneath the F1 and F2 buttons.
3. **Top Edge RGB Strip**: Horizontal multi-LED light bar running across the top bezel.
4. **Shoulder LEDs**: Internal illumination through translucent L1/L2/R1/R2 triggers.
5. **Power Button Status LED**:
   - **Solid Red**: Charging.
   - **Solid Green / Cyan**: Battery Full.

### Sysfs Control

```sh
# Turn off all decorative RGB LEDs (for Pocket Mode / battery savings):
echo 0 > /sys/class/led_anim/max_scale

# Restore full LED brightness:
echo 255 > /sys/class/led_anim/max_scale

# Set RGB color (Red Green Blue, 0-255):
echo "0 180 255" > /sys/class/led_anim/color

# Select animation profile:
echo "rainbow" > /sys/class/led_anim/effect
```

## muOS Integration Details

- **Libc Compatibility**: muOS uses `glibc` (`/lib/ld-linux-aarch64.so.1`). Python PyInstaller bundles (`yt-dlp`) must use the standard glibc aarch64 variant (`yt-dlp_linux_aarch64`).
- **Execution Directory**: `/tmp` is a RAM `tmpfs`. Audiofilus places its control socket in `/tmp` because SD card exFAT does not support UNIX domain sockets.
- **Pocket Mode**:
  - Inhibit suspend: `/run/muos/caffeine`.
  - Inhibit idle dim/mute: Pulse `/run/muos/input_activity` every 3 seconds.
  - Blackout: Screen backlight to 0 via `/opt/muos/script/device/bright.sh 0`, LEDs to 0 via `/sys/class/led_anim/max_scale`.
  - Governor: Set CPU to `powersave` (480 MHz).
