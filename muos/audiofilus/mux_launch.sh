#!/bin/sh
# HELP: Audiofilus: a pixel-art music player that keeps playing in the background, with radio, podcasts, servers and games.
# ICON: audiofilus
# GRID: audiofilus

APP_DIR="${1:-$(dirname "$0")}"
cd "$APP_DIR" || exit 1

# Capture all launcher logs from the very start
exec > "$APP_DIR/log.txt" 2>&1
echo "=== Audiofilus Launch ==="
date 2>/dev/null || true
echo "APP_DIR: $APP_DIR"
echo "ARCH: $(uname -m 2>/dev/null)"
echo "Kernel: $(uname -r 2>/dev/null)"

for f in /opt/muos/script/var/func.sh /opt/muos/script/var/stage.sh /opt/muos/script/var/retro.sh; do
	[ -f "$f" ] && . "$f"
done

command -v SETUP_STAGE_OVERLAY >/dev/null 2>&1 && SETUP_STAGE_OVERLAY
if command -v SETUP_APP >/dev/null 2>&1; then
	SETUP_APP "audiofilus" ""
else
	echo app >/tmp/act_go
	command -v SETUP_SDL_ENVIRONMENT >/dev/null 2>&1 && SETUP_SDL_ENVIRONMENT
fi

# The settings and the library cache live in this folder
export AUDIOFILUS_HOME="$APP_DIR"
export AUDIOFILUS_HANDHELD=1
export EBITENGINE_UI_BACKEND=sdl2
export XDG_RUNTIME_DIR="/run"
export AUDIOFILUS_SOCKET="/tmp/audiofilus.sock"
export LD_LIBRARY_PATH="$APP_DIR/libs.aarch64:$LD_LIBRARY_PATH"

if [ -f /sys/class/led_anim/max_scale ] || grep -q "Allwinner" /proc/cpuinfo 2>/dev/null; then
	export AUDIOFILUS_DEVICE="TrimUI Brick Pro"
fi

# Prepare scratchpad directory on SD card
mkdir -p "$APP_DIR/data/tmp" 2>/dev/null
rm -rf "$APP_DIR/data/tmp/_MEI"* 2>/dev/null || true

chmod -R +x ./audiofilus.aarch64 ./yt-dlp* ./libs.aarch64 2>/dev/null || true

# Purge any partial or corrupted download files
find "$APP_DIR/data/cache" -type f \( -name "*.part*" -o -name "*.tmp" -o -name "*.ytdl" \) -delete 2>/dev/null || true

# Check daemon health: if socket exists but daemon is unresponsive, clean it up
if [ -S "$AUDIOFILUS_SOCKET" ]; then
	if ! ./audiofilus.aarch64 status >/dev/null 2>&1; then
		echo "Found stale socket, cleaning up..."
		killall -9 yt-dlp yt-dlp_linux_aarch64 2>/dev/null || true
		killall -9 audiofilus.aarch64 2>/dev/null || true
		rm -f /tmp/audiofilus*.sock "$AUDIOFILUS_SOCKET" 2>/dev/null || true
	else
		echo "Audio daemon is alive and responsive."
	fi
fi

echo "Starting audiofilus.aarch64 UI..."
./audiofilus.aarch64 &
UI=$!
if command -v SET_VAR >/dev/null 2>&1; then
	SET_VAR "system" "foreground_process" "$UI"
fi
wait "$UI"
RET=$?
echo "audiofilus.aarch64 finished with code: $RET"
