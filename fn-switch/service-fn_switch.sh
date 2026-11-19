#!/bin/sh
# Knulli boot service for the FN switch daemon (TrimUI Brick Pro).
#
# Everything that can leave the internal speaker silent lives in fn-daemon's
# audio repair (`fn-daemon -audio`): the PA chip, the Headphone gain, the default
# sink, the software mute and the volume. This script only wires that up at boot,
# so there is a single source of truth instead of two scripts fighting.

case "$1" in
    start)
        LOGFILE="/userdata/system/logs/fn-service.log"
        mkdir -p /userdata/system/logs
        exec >> "$LOGFILE" 2>&1
        echo "=== [$(date)] fn_switch service START ==="

        APP_DIR="/userdata/system/fn_switch"
        DAEMON="$APP_DIR/fn-daemon"
        ATTENUATE="$APP_DIR/trimui-volume-attenuate"

        # Knulli's own scripts use /var/run as XDG_RUNTIME_DIR.
        export XDG_RUNTIME_DIR="/var/run"
        export PULSE_SERVER="unix:/var/run/pulse/native"

        # 1. Patch /usr/bin/trimui-volume-attenuate: keep Knulli's own gains
        #    (0 when unplugged, 3 when plugged) and force the switch on, so a
        #    manual mute cannot survive a headphone plug/unplug.
        if [ -f "$ATTENUATE" ]; then
            chmod +x "$ATTENUATE" 2>/dev/null
            if ! grep -q "/usr/bin/trimui-volume-attenuate" /proc/mounts 2>/dev/null; then
                mount --bind "$ATTENUATE" /usr/bin/trimui-volume-attenuate \
                    && echo "Patched /usr/bin/trimui-volume-attenuate (bind mount)."
            fi
        fi

        # 2. Check and repair the audio path, dumping what it found.
        if [ -f "$DAEMON" ]; then
            chmod +x "$DAEMON" 2>/dev/null
            "$DAEMON" -audio
        else
            echo "ERROR: $DAEMON not found."
        fi

        # 3. (Bo qua) Am thanh khoi dong da duoc tat theo yeu cau nguoi dung.
        #    Neu can kiem tra nhanh PipeWire/loa, dung: fn-daemon -audio -tone
        # if [ -f "/userdata/system/startup_sound.wav" ]; then
        #     paplay /userdata/system/startup_sound.wav >/dev/null 2>&1 &
        # fi

        # 4. Start the daemon (single instance).
        if [ -f "$DAEMON" ]; then
            if ! pgrep -x fn-daemon >/dev/null 2>&1; then
                "$DAEMON" >> /userdata/system/logs/fn-daemon.log 2>&1 &
                echo "fn-daemon started (PID $!)."
            else
                echo "fn-daemon is already running."
            fi
        fi
        ;;

    stop)
        echo "=== [$(date)] fn_switch service STOP ===" >> /userdata/system/logs/fn-service.log 2>&1
        killall fn-daemon 2>/dev/null || true
        umount /usr/bin/trimui-volume-attenuate 2>/dev/null || true
        ;;
esac

exit 0
