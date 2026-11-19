#!/usr/bin/env python3
"""
Script deploy toàn bộ hệ thống Brick Pro FN Config và fn-daemon lên TrimUI Brick Pro qua SSH.
"""
import os
import sys
import time
import paramiko

HOST = "192.168.1.7"
USER = "root"
PASS = "linux"

def deploy():
    print(f"[*] Đang kết nối tới {HOST}...")
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    try:
        ssh.connect(HOST, username=USER, password=PASS, timeout=8)
    except Exception as e:
        print(f"[-] Không thể kết nối SSH tới {HOST}: {e}")
        print("[!] Hãy chắc chắn rằng máy TrimUI Brick Pro đã bật nguồn và kết nối cùng mạng Wi-Fi.")
        return False

    print("[+] Đã kết nối SSH thành công!")
    sftp = ssh.open_sftp()

    # 1. Tạo thư mục đích
    ssh.exec_command("mkdir -p /userdata/system/fn_switch /userdata/roms/ports")

    base_dir = os.path.dirname(__file__)

    # 2. Upload fn-daemon.gz
    fn_gz = os.path.join(base_dir, "fn-daemon.gz")
    if os.path.exists(fn_gz):
        print(f"[*] Đang tải lên {fn_gz}...")
        sftp.put(fn_gz, "/userdata/system/fn_switch/fn-daemon.gz")

    # 3. Upload brick-config.gz
    cfg_gz = os.path.join(base_dir, "brick-config.gz")
    if os.path.exists(cfg_gz):
        print(f"[*] Đang tải lên {cfg_gz} ({os.path.getsize(cfg_gz) / 1024 / 1024:.2f} MB)...")
        sftp.put(cfg_gz, "/userdata/system/fn_switch/brick-config.gz")

    # 4. Upload config.json
    cfg_json = os.path.join(base_dir, "config.json")
    if os.path.exists(cfg_json):
        print(f"[*] Đang tải lên config.json...")
        sftp.put(cfg_json, "/userdata/system/fn_switch/config.json")

    # 5. Upload script launcher cho Ports
    sh_local = os.path.join(base_dir, "BrickProConfig.sh")
    sh_remote = "/userdata/roms/ports/Brick Pro FN Config.sh"
    print(f"[*] Đang tạo launcher trong thư mục Ports: {sh_remote}...")
    sftp.put(sh_local, sh_remote)

    # 6. Giải nén, phân quyền và khởi động lại daemon
    print("[*] Đang giải nén và phân quyền trên thiết bị...")
    commands = [
        "cd /userdata/system/fn_switch",
        "gzip -df fn-daemon.gz 2>/dev/null || true",
        "gzip -df brick-config.gz 2>/dev/null || true",
        "chmod +x fn-daemon brick-config",
        f"chmod +x '{sh_remote}'",
        # Khôi phục ngay âm lượng analog phần cứng về 100% (7/7, 0dB) và mở loa
        "amixer -q -c 0 sset Headphone 100% unmute",
        "amixer -q -c 0 sset 'digital volume' 100%",
        "amixer -q -c 0 sset HpSpeaker on",
        "pactl set-sink-mute @DEFAULT_SINK@ 0",
        "knulli-audio setSystemVolume 80",
        # Khởi động lại fn-daemon với bản mới nhất
        "killall fn-daemon 2>/dev/null || true",
        "nohup /userdata/system/fn_switch/fn-daemon > /userdata/system/fn_switch/fn-daemon.log 2>&1 &",
    ]
    stdin, stdout, stderr = ssh.exec_command(" && ".join(commands))
    stdout.channel.recv_exit_status()

    sftp.close()

    # 7. Kiểm tra file trên thiết bị
    stdin, stdout, stderr = ssh.exec_command("ls -la /userdata/system/fn_switch/brick-config /userdata/system/fn_switch/fn-daemon '/userdata/roms/ports/Brick Pro FN Config.sh' && pgrep -l fn-daemon")
    print("[+] Trạng thái trên thiết bị:")
    print(stdout.read().decode())

    ssh.close()
    print("[✓] DEPLOY TOÀN BỘ HỆ THỐNG THÀNH CÔNG!")
    print("\n👉 CÁCH MỞ ỨNG DỤNG:")
    print(" 1. Trên máy cầm tay: Vào mục 'Ports' trên Knulli (EmulationStation) -> chọn 'Brick Pro FN Config'")
    print(" 2. Hoặc qua SSH: /userdata/system/fn_switch/BrickProConfig.sh")
    return True

if __name__ == "__main__":
    deploy()
