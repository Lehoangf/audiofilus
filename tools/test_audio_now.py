import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.1.7', username='root', password='linux', timeout=5)

cmd = '''
echo "=== 1. CHECK SWITCH IN KERNEL ==="
cat /sys/class/switch/fn/state 2>/dev/null

echo "=== 2. EVTEST OR EVIOCGSW ==="
python -c "
import fcntl
with open('/dev/input/event3', 'rb') as f:
    buf = bytearray(32)
    fcntl.ioctl(f.fileno(), 0x8020451b, buf)
    print('Switch state (bit 1):', (buf[0] >> 1) & 1)
" 2>&1

echo "=== 3. AUDIO TEST: UNMUTE SINK & PLAY A BEEP ==="
pactl set-sink-mute @DEFAULT_SINK@ 0
pactl set-sink-volume @DEFAULT_SINK@ 80%
amixer -c 0 sset Headphone 100% unmute
amixer -c 0 sset HpSpeaker on
amixer -c 0 sset 'digital volume' 100%

echo "=== 4. CURRENT SINK STATE ==="
pactl list sinks | grep -E 'State:|Mute:|Volume:'

echo "=== 5. CHECK AUDIOFILUS STATUS ==="
/userdata/roms/ports/audiofilus/audiofilus.aarch64 status 2>&1
'''

stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
print(stderr.read().decode())
c.close()
