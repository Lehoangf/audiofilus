---
name: crossmix
description: Build, deploy, install and test Audiofilus on TrimUI devices running CrossMix OS (TrimUI Smart Pro / Brick) via SSH (default 192.168.1.3 or TSP) or direct microSD card. Covers dropbear SSH setup (root:tina / authorized_keys), fast gzip-compressed SCP transfer, atomic binary replacement in Apps and PortMaster ports, log inspection, and runtime troubleshooting.
---

# Audiofilus on CrossMix OS (TrimUI Smart Pro / Brick)

CrossMix OS is an enhanced OS for the **TrimUI Smart Pro** (and compatible Allwinner A133P / TG5040 devices) based on Allwinner Tina Linux (`Linux TinaLinux 4.9.191 aarch64`).

This skill details how to build, deploy, update, and debug Audiofilus on CrossMix OS via **SSH/SCP over Wi-Fi** or directly on the **microSD card**.

---

## 1. Network & SSH Access

- **Default IP**: Usually `192.168.1.3` (or the IP displayed in CrossMix under `System Tools -> NETWORK -> Display IP`).
- **SSH Daemon**: Dropbear on port `22`.
- **Default Credentials**:
  - Username: `root`
  - Password: `tina`
- **Authorized Keys**:
  - Dropbear reads keys from `/root/.ssh/authorized_keys` and `/etc/dropbear/authorized_keys`.
  - To authorize your PC's SSH key once:
    ```powershell
    $PUB = Get-Content "$env:USERPROFILE\.ssh\id_ed25519.pub"
    ssh root@192.168.1.3 "mkdir -p /root/.ssh /etc/dropbear && echo '$PUB' >> /root/.ssh/authorized_keys && echo '$PUB' >> /etc/dropbear/authorized_keys && chmod 700 /root/.ssh /etc/dropbear && chmod 600 /root/.ssh/authorized_keys /etc/dropbear/authorized_keys"
    ```
  - After installing the key, `ssh root@192.168.1.3` and `scp` connect passwordlessly.

---

## 2. Audiofilus Layout on CrossMix OS

CrossMix mounts the main FAT32/exFAT microSD card at `/mnt/SDCARD`. Audiofilus can be installed in two locations:

| Type | Path on Device | Notes |
|---|---|---|
| **Main App** | `/mnt/SDCARD/Apps/Audiofilus/` | App launcher: `launch.sh`, binary: `audiofilus.aarch64`. Contains `data/`, `fonts/`, `libs.aarch64/`, `glyph/`, `audiofilus.json`. |
| **PortMaster Port** | `/mnt/SDCARD/Data/ports/audiofilus/` | Launched from `/mnt/SDCARD/Roms/Ports/Audiofilus.sh` or `/mnt/SDCARD/Data/ports/Audiofilus.sh`. |

Both locations use the same Linux ARM64 binary: `audiofilus.aarch64`.

---

## 3. Build & Deploy via SSH

### 3.1 Preflight & Build

Always build clean with `CGO_ENABLED=0`:

```powershell
# 1. Tests & formatting
gofmt -l .
go vet ./...
go test ./...

# 2. Build Linux arm64 binary
$env:CGO_ENABLED = "0"; $env:GOOS = "linux"; $env:GOARCH = "arm64"
go build -trimpath -ldflags="-s -w" -o dist/audiofilus-spotify.aarch64 ./cmd/audiofilus
```

### 3.2 Fast Gzip-Compressed SCP Transfer

Wi-Fi on handhelds can be slow. Compressing the 26MB binary with gzip cuts the transfer size down to ~11MB:

```powershell
# Compress locally
python -c "import gzip, shutil; shutil.copyfileobj(open('dist/audiofilus-spotify.aarch64','rb'), gzip.open('dist/audiofilus-spotify.aarch64.gz','wb',compresslevel=6))"

# Upload to /tmp on the device
scp -o BatchMode=yes -o StrictHostKeyChecking=no dist/audiofilus-spotify.aarch64.gz root@192.168.1.3:/tmp/audiofilus.aarch64.gz

# Decompress and verify MD5 on the device
$LOCAL_MD5 = (Get-FileHash dist/audiofilus-spotify.aarch64 -Algorithm MD5).Hash.ToLower()
ssh root@192.168.1.3 "gzip -dc /tmp/audiofilus.aarch64.gz > /tmp/audiofilus.aarch64 && md5sum /tmp/audiofilus.aarch64"
```

### 3.3 Atomic Installation & Process Replacement

Do not overwrite a binary that might be running in memory. Replace atomically and sync:

```powershell
ssh root@192.168.1.3 "chmod +x /tmp/audiofilus.aarch64 && cp -f /tmp/audiofilus.aarch64 /mnt/SDCARD/Apps/Audiofilus/audiofilus.aarch64 && cp -f /tmp/audiofilus.aarch64 /mnt/SDCARD/Data/ports/audiofilus/audiofilus.aarch64 && sync && rm -f /tmp/audiofilus.aarch64 /tmp/audiofilus.aarch64.gz && echo 'DEPLOY_OK'"
```

Clean up the local temporary `.gz` file:
```powershell
Remove-Item dist/audiofilus-spotify.aarch64.gz -ErrorAction SilentlyContinue
```

---

## 4. Direct MicroSD Installation (Offline PC Reader)

When the SD card is plugged directly into the PC:

1. Copy `dist/audiofilus-spotify.aarch64` to `<Drive>:\Apps\Audiofilus\audiofilus.aarch64`
2. If PortMaster ports exist, also copy to `<Drive>:\Data\ports\audiofilus\audiofilus.aarch64`
3. If staging fresh files (fonts, icons, libs), copy `dist/muxapp_spotify/audiofilus-spotify/*` into `<Drive>:\Apps\Audiofilus\`.

---

## 5. Control & Inspection Commands

```powershell
$H = "root@192.168.1.3"

# Check running audiofilus processes
ssh $H "ps | grep '[a]udiofilus'"

# Stop daemon / UI
ssh $H "pkill -f audiofilus"

# Read UI launch log
ssh $H "tail -n 50 /mnt/SDCARD/Apps/Audiofilus/log.txt"

# Read background service log
ssh $H "tail -n 80 /mnt/SDCARD/Apps/Audiofilus/data/audiofilus.log"

# Check daemon socket
ssh $H "ls -la /tmp/audiofilus.sock*"
```

---

## 6. Troubleshooting

1. **Authentication failed**:
   - Check if password `tina` was changed.
   - Re-check `/root/.ssh/authorized_keys` and `/etc/dropbear/authorized_keys`.
2. **Transfer stalls / hang**:
   - Embedded dropbear SFTP can stall on large continuous unbuffered pipes. Use native `scp` with pre-compressed `.gz` archives instead of raw SFTP stream puts.
3. **Stale socket after crash**:
   - If UI does not connect to daemon, remove stale lock:
     ```sh
     rm -f /tmp/audiofilus*.sock*
     ```
   - `launch.sh` in CrossMix also auto-cleans stale sockets on launch.
