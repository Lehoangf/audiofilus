---
name: trimui-brick-pro
description: Comprehensive hardware specifications, physical ports & OTG, data transfer methods, button & LED subsystem control, power/pocket mode management, and muOS software integration guide for the TrimUI Brick Pro handheld.
---

# TrimUI Brick Pro (muOS) Hardware & Software Guide

This skill documents the hardware specifications, physical I/O ports, data transfer protocols, complete button and input mapping, multi-zone LED lighting subsystem, audio routing, power management, and software integration on the **TrimUI Brick Pro** running **muOS** (MustardOS).

For the Audiofilus-specific workflows, use the companion skills:
[`brick-pro`](../brick-pro/SKILL.md) (hub: build, install, display),
[`test-on-brick-pro`](../test-on-brick-pro/SKILL.md) (SSH test loop),
[`debug-muos-brick-pro`](../debug-muos-brick-pro/SKILL.md) (platform,
sleep, audio, controller),
[`measure-power-brick-pro`](../measure-power-brick-pro/SKILL.md) (CPU and
battery), [`spotify-brick-pro`](../spotify-brick-pro/SKILL.md) (login, cache,
rate limits) and [`pack-release-brick-pro`](../pack-release-brick-pro/SKILL.md)
(muxapp and PortMaster packaging).

---

## 1. Hardware Specifications & Physical Ports

| Component | Specification |
|---|---|
| **SoC** | Allwinner A133P (Sunxi architecture) |
| **CPU** | Quad-core ARM Cortex-A53 up to 1.8 GHz |
| **GPU** | Imagination PowerVR GE8300 |
| **Architecture** | ARM64 (`aarch64`), `GOARCH=arm64` |
| **Display** | 1024×768 (4:3 aspect ratio), 60 Hz IPS LCD |
| **Display Interface** | MIPI-DSI, `/dev/fb0`, Allwinner debugfs `/sys/kernel/debug/dispdbg` |
| **Audio** | Stereo bottom-firing speakers, 3.5mm stereo headphone jack with plug detection |
| **Battery** | 5000 mAh Li-Po, AXP PMIC sysfs interface (usable capacity is lower) |
| **Storage** | Dual microSD card slots (TF1/OS, TF2/Games & Media), supports up to 256GB+ |
| **Connectivity** | 2.4 GHz Wi-Fi 802.11 b/g/n, Bluetooth 4.2 / 5.0 |

### 1.1 Physical Ports & I/O Capabilities
- **Top USB Type-C Port (USB Host / OTG & Data)**:
  - **Data Transfer**: Fully wired for high-speed USB 2.0 data communication.
  - **USB Host (OTG)**: Supports connecting external USB peripherals via an OTG adapter:
    - External gamepads / controllers (for 2-player multiplayer).
    - External USB card readers and USB flash drives.
    - Wired/wireless keyboard and mouse.
    - External USB DACs and audio interfaces.
  - **PC Direct Connection**: When connected to a computer, allows direct file management depending on the active firmware/USB mode (MTP / USB Mass Storage Gadget).
- **Bottom USB Type-C Port (Dedicated Charging Port)**:
  - **Power Input**: Dedicated DC 5V/2A charging port connected to the AXP power management IC (PMIC).
  - **No Data Transfer**: This port does **not** support USB data lines.
  - **No TV-Out / Video Output**: The TrimUI Brick Pro **does not support DisplayPort Alt Mode (DP-Alt) or video output over Type-C** on either port. External HDMI/TV-Out is not available over these USB-C connectors.
- **MicroSD Card Slots (TF Slots)**:
  - Holds the bootloader, muOS operating system, emulators, apps, and media libraries.
  - Supports cards from 64GB up to 256GB (formatted as FAT32 or exFAT).
  - **Best Practice for Bulk Data**: For large transfers (gigabytes of music libraries or game ROMs), ejecting the microSD card and reading it directly through a high-speed PC card reader is the fastest, most reliable, and least error-prone method.
- **3.5mm Analog Audio Jack**:
  - Standard 3.5mm stereo TRS output for wired headphones, earphones, or external auxiliary speakers.
  - Hardware plug insertion is reported to the OS (muOS creates `/run/muos/headphones`).

---

## 2. Complete Button & Input Mapping

### 2.1 Front Face Controls
- **D-Pad (Directional Pad)**:
  - Up, Down, Left, Right directional switches.
  - Evdev codes: `KEY_UP`, `KEY_DOWN`, `KEY_LEFT`, `KEY_RIGHT` or analog Hat0X / Hat0Y axes depending on SDL mapping.
- **Action Buttons (4 Buttons)**:
  - **A**: Confirm / Select (`BTN_EAST` / `BTN_A`)
  - **B**: Back / Cancel (`BTN_SOUTH` / `BTN_B`)
  - **X**: Search / Secondary action (`BTN_NORTH` / `BTN_X`)
  - **Y**: Context menu / Quick action (`BTN_WEST` / `BTN_Y`)
- **Dual Hall Effect Joysticks**:
  - Non-contact magnetic Hall effect sensors with zero potentiometer wear and drift-free center.
  - **Left Joystick**: `ABS_X` (horizontal), `ABS_Y` (vertical).
  - **Right Joystick**: `ABS_Z` (horizontal), `ABS_RZ` (vertical).
  - L3 / R3 stick click buttons (`BTN_THUMBL`, `BTN_THUMBR`).
- **Center Control Buttons**:
  - **Select**: Menu back / auxiliary (`BTN_SELECT`).
  - **Start**: Play/Pause / options confirmation (`BTN_START`).
  - **Menu**: OS dashboard or emulator overlay (`KEY_MENU`).
- **Auxiliary Front Buttons**:
  - **F1**: Front macro / special feature button 1 (`KEY_F1`). Integrated sub-key LED.
  - **F2**: Front macro / special feature button 2 (`KEY_F2`). Integrated sub-key LED.

### 2.2 Top Edge (Shoulder & Trigger Buttons)
- **Bumpers**:
  - **L1**: Left shoulder bumper (`BTN_TL` / `leftshoulder`).
  - **R1**: Right shoulder bumper (`BTN_TR` / `rightshoulder`).
- **Triggers**:
  - **L2**: Left trigger (`BTN_TL2` / `lefttrigger`).
  - **R2**: Right trigger (`BTN_TR2` / `righttrigger`).
- Integrated LED housing inside transparent triggers.

### 2.3 Side & Bottom Controls
- **Power Button**: Right side. Sends `KEY_POWER` (scancode 116).
  - Short press: App sleep / wake toggle.
  - Long press (>3s): OS power menu / suspend / shutdown.
  - Integrated charging/power status LED.
- **Volume Buttons (+ / -)**: Left side.
  - `KEY_VOLUMEUP`, `KEY_VOLUMEDOWN`. Handled by `muhotkey` or `/opt/muos/script/device/audio.sh`.
- **Home Button**: Front/side home navigation (`KEY_HOMEPAGE`).
- **FN Slider Switch**: 2-position hardware sliding toggle switch on the side.
  - Sysfs node: `/sys/class/switch/fn/state`
  - Returns `0` (normal/down) or `1` (active/up).
  - Ideal for hardware **Pocket Mode** switch or Mute/Unmute toggle.
- **Reset Pinhole**: Located on the bottom chassis edge. Directly resets the PMIC hardware lines (no OS involvement).

---

## 3. LED Lighting Subsystem

The TrimUI Brick Pro features a multi-zone RGB lighting subsystem driven by the kernel animation driver `/sys/class/led_anim/` and standard `/sys/class/leds/`.

### 3.1 Hardware LED Zones
1. **Joystick Rings (Front Face)**: Circular RGB halos beneath both Left and Right Hall effect joysticks.
2. **F1 & F2 Sub-Key LEDs (Front Face)**: Individual backlighting LEDs positioned directly under the F1 and F2 keycaps.
3. **Top Edge RGB Light Bar (Top Edge)**: Continuous horizontal multi-LED light strip across the upper bezel.
4. **Shoulder Key LEDs (Top Edge)**: Internal illumination visible through the translucent housings of L1, L2, R1, and R2.
5. **Power / Charging Indicator (Side)**: Dual-color LED embedded in the Power button:
   - **Solid Red**: Charging in progress.
   - **Solid Green / Cyan**: Battery fully charged.
   - **Off / Dim Green**: Discharging / Normal operating battery level.

### 3.2 Sysfs Control Nodes
- **Global LED Brightness & Master Switch**:
  ```sh
  # Turn off all decorative RGB LEDs (Pocket Mode / battery saver):
  echo 0 > /sys/class/led_anim/max_scale

  # Restore full brightness (value 1 to 255):
  echo 255 > /sys/class/led_anim/max_scale
  ```
- **Animation Effects**:
  ```sh
  # Available effects: static, breath, rainbow, pulse, reactive
  cat /sys/class/led_anim/effect
  echo "rainbow" > /sys/class/led_anim/effect
  ```
- **Static RGB Color Selection**:
  ```sh
  # Format: <Red 0-255> <Green 0-255> <Blue 0-255>
  echo "0 180 255" > /sys/class/led_anim/color   # Cyan/Teal
  echo "255 0 128" > /sys/class/led_anim/color   # Magenta
  ```

---

## 4. Data Transfer & File Management on muOS

muOS on the TrimUI Brick Pro provides multiple local and network methods for transferring music, apps, and games:

### 4.1 Method 1: Direct MicroSD Card Reader (Recommended for Bulk Data)
- Eject the microSD card and insert it into a PC card reader.
- Applications live in `MUOS/application/` (e.g. `MUOS/application/audiofilus-spotify/`).
- Music files can be placed in `music/` or `MUOS/music/`.
- Fastest transfer speed (~80–100 MB/s depending on card class) with zero network overhead.

### 4.2 Method 2: Top USB Type-C Port (PC Direct Connection & USB OTG)
- **Direct PC Connection**: Connect the **Top USB Type-C port** to a PC using a USB-C data cable. In muOS settings, enable USB Mass Storage / MTP mode; the console partitions will mount as removable drives on Windows / macOS / Linux.
- **USB OTG Flash Drive Transfer**: Plug a USB flash drive or card reader into the top port via an OTG adapter. muOS mounts external storage under `/mnt/usb/` or `/run/media/`.

### 4.3 Method 3: Wireless SFTP / SSH (Over Wi-Fi)
- muOS includes an integrated SSH/SFTP server. Connect the console to your local Wi-Fi.
- Connect using any SFTP client (FileZilla, WinSCP, Cyberduck, or CLI `scp` / `rsync`):
  - **Host**: `<console-ip>` (e.g. `192.168.1.x` or `muos.local`)
  - **Port**: `22`
  - **Username**: `root`
  - **Password**: `root` (or configured muOS password)
- Upload `.muxapp` archives directly to `/mnt/sdcard/ARCHIVE/` or unpack into `/opt/muos/application/`.

### 4.4 Method 4: Samba (SMB) Network File Share
- muOS supports Windows Network Sharing (Samba).
- When enabled in muOS Network Settings, open Windows File Explorer and navigate to:
  ```text
  \\<console-ip>\
  ```
  or
  ```text
  \\muos\
  ```
- Drag and drop music tracks, configurations, and application packages directly over the LAN without needing third-party software.

### 4.5 Method 5: Syncthing / rsync Background Sync
- For automated synchronization, `rsync` over SSH or the Syncthing daemon can run in the background on muOS to keep playlists, favorites, and music cache synchronized across devices.

---

## 5. Operating System (muOS) Integration

muOS on the TrimUI Brick Pro is built on Buildroot using the standard **glibc** runtime (`/lib/ld-linux-aarch64.so.1`).

### 5.1 Preventing Involuntary Suspend & Mute
- **Caffeine (Keep-awake)**:
  - While `/run/muos/caffeine` exists, muOS hotkey scripts (`script/mux/hotkey.sh`) inhibit system suspend triggered by lid or inactivity.
  - Audiofilus manages this automatically via `platform.SetAwake(true)`.
- **Inactivity Activity Pulse**:
  - The muOS idle watcher (`script/mux/idle.sh`) checks `/run/muos/input_activity` every 5 seconds. If the timestamp does not advance, it dims the screen and engages `idle_mute`, silencing audio in background playback.
  - Audiofilus updates `/run/muos/input_activity` every 3 seconds while active to preserve background audio.

### 5.2 Display & Backlight Control (Pocket Mode)
- **Blanking the screen**:
  ```sh
  # Turn off backlight without losing saved user brightness:
  /opt/muos/script/device/bright.sh 0
  ```
- **Direct LCD controller blanking (Allwinner Sunxi dispdbg)**:
  ```sh
  echo name lcd0 > /sys/kernel/debug/dispdbg
  echo command setbl > /sys/kernel/debug/dispdbg
  echo param 0 > /sys/kernel/debug/dispdbg
  echo start 1 > /sys/kernel/debug/dispdbg
  ```
- **Unblanking the screen**:
  muOS `bright.sh` ignores updates when the target matches the current value. A two-step nudge is required:
  ```sh
  SAVED=$(cat /opt/muos/config/settings/general/brightness 2>/dev/null || echo 90)
  /opt/muos/script/device/bright.sh $((SAVED - 1))
  /opt/muos/script/device/bright.sh $SAVED
  ```

### 5.3 Audio Architecture
- muOS utilizes **PipeWire** without the PulseAudio emulation daemon.
- Audiofilus routes output via **ALSA** (`default` PCM in `/etc/asound.conf`), which forwards directly into PipeWire.
- Requires `XDG_RUNTIME_DIR=/run` so clients can connect to PipeWire's control socket.

### 5.4 CPU Governors & Power Saving
- **Active Playback**: `ondemand` or `schedutil` (~1008 MHz).
- **Pocket Mode / Sleep Playback**: `powersave` (locks CPU to 480 MHz). Pure Go decoding of MP3/AAC consumes ~2% of one core.
- Set via:
  ```sh
  echo powersave > /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
  ```

---

## 6. Binary Compatibility & Packaging Rules

### 6.1 C Library (Glibc)
- muOS uses **glibc** (`/lib/ld-linux-aarch64.so.1`).
- Standalone binaries compiled against musl will fail with `ENOENT: no such file or directory`.
- Go binaries are compiled with **`CGO_ENABLED=0 GOOS=linux GOARCH=arm64`**.
- Helper utilities (such as `yt-dlp`) must use the glibc build (`yt-dlp_linux_aarch64` or unpacked in `yt-dlp/`).

### 6.2 TMPDIR & Sockets
- Standalone PyInstaller bundles extract into `$TMPDIR/_MEIxxxxxx`.
- Sockets cannot reside on FAT/exFAT SD cards; they must reside in `/tmp` (`AUDIOFILUS_SOCKET="/tmp/audiofilus.sock"` in `muos/audiofilus/mux_launch.sh`). Without that variable the path is `/tmp/audiofilus-<hash>.sock`, derived from the binary folder (`internal/config/config.go`), so always set it explicitly on the console.
- Providing yt-dlp unpacked in `yt-dlp/` starts up in ~3s instead of ~11s and does not extract into RAM.

### 6.3 Application Package Structure (.muxapp)
muOS installs applications from `.muxapp` archives (ZIP format) into `MUOS/application/`:

```
audiofilus-spotify/
├── audiofilus.aarch64      # Go binary (CGO_ENABLED=0, ARM64)
├── mux_launch.sh           # Executable POSIX shell launcher
├── yt-dlp/                 # Unpacked glibc yt-dlp directory
│   ├── yt-dlp              # yt-dlp executable (glibc aarch64)
│   └── _internal/          # Python runtime libraries
├── data/
│   ├── spotify_client.json # Spotify OAuth client configuration
│   └── tmp/                # PyInstaller execution scratchpad
├── glyph/
│   ├── audiofilus.png         # app icon named by `# ICON:` in mux_launch.sh
│   └── audiofilus-spotify.png # alternate icon kept in the fork's staging
└── licenses/               # License declarations
```

---

## 7. Testing & Diagnostic Procedures

### 7.1 Inspecting Hardware State via Shell
```sh
# Verify CPU architecture and Allwinner A133P SoC:
grep -E "Hardware|Processor|Allwinner" /proc/cpuinfo

# Test input events (buttons, joysticks, triggers):
cat /proc/bus/input/devices
evtest /dev/input/event0

# Read hardware FN slider switch state:
cat /sys/class/switch/fn/state

# Read battery voltage, capacity, and charging status:
cat /sys/class/power_supply/*/capacity
cat /sys/class/power_supply/*/status

# Test LED animation controller:
echo 0 > /sys/class/led_anim/max_scale    # LEDs off
echo 255 > /sys/class/led_anim/max_scale  # LEDs on

# Verify audio output devices:
cat /proc/asound/cards
cat /proc/asound/pcm
```

### 7.2 Verifying Application Execution
```sh
# Verify process PID (comm is truncated to "audiofilus.aarch", and the
# service's argv[0] is "audiofilusd": match by comm/args, never pkill -f):
ps -eo pid,comm,args | grep '[a]udiofilus'

# Check background daemon logs:
tail -f data/audiofilus.log

# Verify socket connection:
ls -la /tmp/audiofilus-*.sock
```
