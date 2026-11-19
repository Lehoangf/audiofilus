import paramiko
import os
import time
import socket

IP = '192.168.1.20'
USER = 'root'
PASS = 'linux'

print(f"Waiting for {IP} to be available on Wi-Fi...")
while True:
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(1)
    res = s.connect_ex((IP, 22))
    s.close()
    if res == 0:
        print(f"[+] Connected to port 22 on {IP}!")
        break
    time.sleep(1)

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(IP, username=USER, password=PASS, timeout=10)

print("[1] Killing any running fn-daemon...")
client.exec_command('killall fn-daemon 2>/dev/null')
time.sleep(0.5)

print("[2] Uploading new binary and config...")
sftp = client.open_sftp()
sftp.put(r'E:\audiofilus\fn-switch\fn-daemon', '/userdata/system/fn_switch/fn-daemon.new')
sftp.put(r'E:\audiofilus\fn-switch\config.json', '/userdata/system/fn_switch/config.json')
sftp.close()

print("[3] Moving binary into place and setting permissions...")
cmd_perm = 'cd /userdata/system/fn_switch && mv -f fn-daemon.new fn-daemon && chmod +x fn-daemon'
stdin, stdout, stderr = client.exec_command(cmd_perm)
stdout.channel.recv_exit_status()

print("[4] Launching updated fn-daemon in background...")
cmd_launch = '/userdata/system/fn_switch/fn-daemon > /tmp/fn-daemon.log 2>&1 &'
client.exec_command(cmd_launch)
time.sleep(1)

print("[5] Checking daemon log...")
stdin, stdout, stderr = client.exec_command('cat /tmp/fn-daemon.log')
print(stdout.read().decode())

client.close()
print(">>> DEPLOY COMPLETE! ALL FIXES APPLIED!")
