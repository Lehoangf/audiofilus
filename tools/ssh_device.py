import os
import sys
import paramiko

IP = '192.168.80.99'
USER = 'root'
PASS = 'root'

def setup_ssh():
    pub_path = os.path.expanduser('~/.ssh/id_ed25519.pub')
    if os.path.exists(pub_path):
        with open(pub_path, 'r', encoding='utf-8') as f:
            pub_key = f.read().strip()
    else:
        pub_key = None

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(IP, username=USER, password=PASS, timeout=10)

    if pub_key:
        cmd = f'mkdir -p ~/.ssh && echo "{pub_key}" >> ~/.ssh/authorized_keys && chmod 700 ~/.ssh && chmod 600 ~/.ssh/authorized_keys'
        stdin, stdout, stderr = client.exec_command(cmd)
        stdout.channel.recv_exit_status()
        print("[+] Added PC SSH public key to ~/.ssh/authorized_keys")

    stdin, stdout, stderr = client.exec_command('find /mnt -maxdepth 5 -type d \\( -name "audiofilus-spotify" -o -name "audiofilus" \\) 2>/dev/null')
    dirs = stdout.read().decode('utf-8', errors='ignore').strip().splitlines()
    print("[+] Found Audiofilus directories on device:")
    for d in dirs:
        print("   ", d)

    client.close()

if __name__ == '__main__':
    setup_ssh()
