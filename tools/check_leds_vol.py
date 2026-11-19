import paramiko

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.20', username='root', password='linux', timeout=5)

cmd = '''
echo '=== LED_ANIM FILES ==='
ls -la /sys/class/led_anim/
cat /sys/class/led_anim/effect 2>/dev/null
cat /sys/class/led_anim/state 2>/dev/null

echo '=== LEDS FOLDER ==='
ls -la /sys/class/leds/

echo '=== CHECK INDIVIDUAL LED BRIGHTNESS ==='
for d in /sys/class/leds/*; do
    if [ -f "$d/brightness" ]; then
        echo "$d: $(cat $d/brightness) / $(cat $d/max_brightness 2>/dev/null)"
    fi
done

echo '=== VOLUME CONTROLS ==='
knulli-audio getSystemVolume
amixer -c 0 sget "digital volume"
amixer -c 0 sget "HpSpeaker"
amixer -c 0 sget "Headphone"
'''

stdin, stdout, stderr = client.exec_command(cmd)
print(stdout.read().decode())
client.close()
