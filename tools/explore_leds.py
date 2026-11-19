import paramiko
import time

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

for attempt in range(5):
    try:
        client.connect('192.168.1.20', username='root', password='linux', timeout=5)
        break
    except Exception as e:
        time.sleep(1)

cmd = '''
echo "=== LED_ANIM ==="
ls -la /sys/class/led_anim/ 2>/dev/null
for f in /sys/class/led_anim/*; do
    if [ -f "$f" ]; then
        echo "led_anim/$(basename $f): $(cat $f 2>/dev/null)"
    fi
done

echo "=== LEDS ==="
ls -la /sys/class/leds/ 2>/dev/null
for d in /sys/class/leds/*; do
    if [ -f "$d/brightness" ]; then
        echo "leds/$(basename $d): $(cat $d/brightness 2>/dev/null) / $(cat $d/max_brightness 2>/dev/null)"
    fi
done
'''

stdin, stdout, stderr = client.exec_command(cmd)
print(stdout.read().decode())
client.close()
