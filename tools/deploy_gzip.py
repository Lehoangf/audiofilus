import paramiko
import os
import time

IP = '192.168.1.20'
USER = 'root'
PASS = 'linux'

print(f"Connecting to {IP}...", flush=True)
client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(IP, username=USER, password=PASS, timeout=10)
print("[+] SSH Connected!", flush=True)

print("[1] Stopping old daemon...", flush=True)
client.exec_command('killall -9 fn-daemon 2>/dev/null')
time.sleep(0.5)

print("[2] Uploading fn-daemon.gz (2.4 MB) via SFTP...", flush=True)
sftp = client.open_sftp()
sftp.put(r'E:\audiofilus\fn-switch\fn-daemon.gz', '/userdata/system/fn_switch/fn-daemon.gz')
print("[+] Uploaded binary archive!", flush=True)

print("[3] Uploading config.json...", flush=True)
sftp.put(r'E:\audiofilus\fn-switch\config.json', '/userdata/system/fn_switch/config.json')
print("[+] Uploaded config.json!", flush=True)
sftp.close()

print("[4] Decompressing and setting permissions...", flush=True)
cmd_unpack = 'cd /userdata/system/fn_switch && gzip -df fn-daemon.gz && chmod +x fn-daemon'
stdin, stdout, stderr = client.exec_command(cmd_unpack)
stdout.channel.recv_exit_status()
print("[+] Unpacked fn-daemon successfully!", flush=True)

print("[5] Launching updated daemon...", flush=True)
cmd_launch = '/userdata/system/fn_switch/fn-daemon > /tmp/fn-daemon.log 2>&1 &'
client.exec_command(cmd_launch)
time.sleep(1.5)

print("[6] Checking daemon status and log:", flush=True)
stdin, stdout, stderr = client.exec_command('ps aux | grep fn-daemon | grep -v grep; cat /tmp/fn-daemon.log')
print(stdout.read().decode(), flush=True)

client.close()
print(">>> DEPLOY COMPLETE! <<<", flush=True)
