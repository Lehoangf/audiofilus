import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.1.7', username='root', password='linux', timeout=5)

cmd = '''
echo "=== 1. UNMUTE EVERYTHING ==="
amixer -c 0 sset Headphone 100% unmute
amixer -c 0 sset HpSpeaker on
amixer -c 0 sset "digital volume" 100%
pactl set-sink-mute @DEFAULT_SINK@ 0
pactl set-sink-volume @DEFAULT_SINK@ 80%

echo "=== 2. RESUME AUDIOFILUS ==="
/userdata/roms/ports/audiofilus/audiofilus.aarch64 resume
/userdata/roms/ports/audiofilus/audiofilus.aarch64 status

echo "=== 3. GENERATE A 1-SECOND BEEP WITH SPEAKER-TEST ==="
speaker-test -t sine -f 880 -l 1 >/dev/null 2>&1
echo "Beep completed with exit code: $?"
'''

stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
print(stderr.read().decode())
c.close()
