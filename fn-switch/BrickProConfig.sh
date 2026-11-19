#!/bin/bash
# ==========================================================
# Launcher for Brick Pro FN Config on TrimUI Brick Pro (Knulli)
# ==========================================================

export EBITENGINE_UI_BACKEND=sdl2
export XDG_RUNTIME_DIR=/run

APP_DIR="/userdata/system/fn_switch"
BINARY="$APP_DIR/brick-config"

cd "$APP_DIR" || exit 1
chmod +x "$BINARY" 2>/dev/null || true

# Tắt con trỏ chuột console nếu có
if [ -e /sys/class/graphics/fbcon/cursor_blink ]; then
    echo 0 > /sys/class/graphics/fbcon/cursor_blink 2>/dev/null || true
fi

# Chạy ứng dụng cấu hình
"$BINARY" "$@"
