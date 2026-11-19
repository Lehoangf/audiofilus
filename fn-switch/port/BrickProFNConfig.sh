#!/bin/bash
# ==============================================================================
# Knulli port launcher: Brick Pro FN Config (TrimUI Brick Pro)
#
# Drop-in install: copy this .sh, gameinfo.xml and the "brickprofnconfig" folder
# into /userdata/roms/ports/. On the first launch from the Ports menu this
# script installs the daemon + boot service, starts it, then opens the config
# UI. No SSH and no manual setup needed. An existing config.json is preserved.
# ==============================================================================

PORTDIR="$(cd "$(dirname "$0")" 2>/dev/null && pwd)"
ASSETS="$PORTDIR/brickprofnconfig"
SYS="/userdata/system"
APP_DIR="$SYS/fn_switch"
LOGDIR="$SYS/logs"
SERVICES_DIR="$SYS/services"
SERVICE="$SERVICES_DIR/fn_switch"

log() { echo "[brickprofn] $*"; }

if [ ! -d "$ASSETS" ]; then
    echo "ERROR: asset folder not found: $ASSETS" >&2
    echo "Copy the whole port (the .sh, gameinfo.xml and brickprofnconfig/) into /userdata/roms/ports/." >&2
    sleep 4
    exit 1
fi

mkdir -p "$APP_DIR" "$SERVICES_DIR" "$LOGDIR" || exit 1

# 1. Refresh binaries and the audio helper (these are safe to overwrite).
for f in fn-daemon brick-config trimui-volume-attenuate; do
    if [ -f "$ASSETS/$f" ]; then
        cp -f "$ASSETS/$f" "$APP_DIR/$f"
        chmod +x "$APP_DIR/$f"
    fi
done

# 2. Config: seed it on first run only; never overwrite the user's settings.
if [ ! -f "$APP_DIR/config.json" ] && [ -f "$ASSETS/config.json" ]; then
    cp -f "$ASSETS/config.json" "$APP_DIR/config.json"
    log "created default config.json"
else
    log "keeping existing config.json"
fi

# 3. Install the boot service (extensionless name, as Knulli expects).
if [ -f "$ASSETS/service-fn_switch.sh" ]; then
    cp -f "$ASSETS/service-fn_switch.sh" "$SERVICE"
    chmod +x "$SERVICE"
fi

# 4. Enable the service in Knulli settings (idempotent).
SETTINGS_GET=/usr/bin/knulli-settings-get
SETTINGS_SET=/usr/bin/knulli-settings-set
command -v "$SETTINGS_GET" >/dev/null 2>&1 || SETTINGS_GET="$(command -v knulli-settings-get 2>/dev/null)"
command -v "$SETTINGS_SET" >/dev/null 2>&1 || SETTINGS_SET="$(command -v knulli-settings-set 2>/dev/null)"

if [ -n "$SETTINGS_GET" ] && [ -n "$SETTINGS_SET" ]; then
    CUR="$($SETTINGS_GET system.services 2>/dev/null)"
    case " $CUR " in
        *" fn_switch "*)
            log "fn_switch already enabled in system.services"
            ;;
        *)
            if [ -z "$CUR" ]; then
                $SETTINGS_SET system.services fn_switch
            else
                $SETTINGS_SET system.services "$CUR fn_switch"
            fi
            log "enabled fn_switch in system.services"
            ;;
    esac
else
    # Fallback for builds without knulli-settings: keep the deprecated
    # custom.sh path working as well.
    CUSTOM="$SYS/custom.sh"
    touch "$CUSTOM" 2>/dev/null && chmod +x "$CUSTOM" 2>/dev/null
    if [ -f "$CUSTOM" ] && ! grep -q "fn-daemon" "$CUSTOM" 2>/dev/null; then
        {
            echo ""
            echo "# Brick Pro FN switch daemon"
            echo "if [ -x \"$APP_DIR/fn-daemon\" ]; then"
            echo "    $APP_DIR/fn-daemon > \"$LOGDIR/fn-daemon.log\" 2>&1 &"
            echo "fi"
        } >> "$CUSTOM"
        log "added fn-daemon to custom.sh (fallback)"
    fi
fi

# 5. Start it now, so no reboot is needed.
if [ -f "$SERVICE" ]; then
    bash "$SERVICE" start >/dev/null 2>&1
    log "service started"
fi

# 6. Open the configuration UI.
export EBITENGINE_UI_BACKEND=sdl2
export XDG_RUNTIME_DIR=/run
cd "$APP_DIR" || exit 1
exec "$APP_DIR/brick-config" "$@"
