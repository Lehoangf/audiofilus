import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.1.20', username='root', password='linux', timeout=5)

cmd = '''
echo "--- TESTING COMPLETE LED SHUTOFF ---"
echo 0 > /sys/class/led_anim/max_scale
echo 0 > /sys/class/led_anim/max_scale_lr 2>/dev/null
echo 0 > /sys/class/led_anim/max_scale_rear 2>/dev/null
echo 0 > /sys/class/led_anim/effect_enable 2>/dev/null
echo 0 > /sys/class/led_anim/effect_lr 2>/dev/null
echo 0 > /sys/class/led_anim/effect_m 2>/dev/null
echo 0 > /sys/class/led_anim/effect_cycles_lr 2>/dev/null
echo 0 > /sys/class/led_anim/effect_cycles_m 2>/dev/null

for f in /sys/class/leds/sunxi_led*/brightness; do
    echo 0 > "$f" 2>/dev/null
done

echo "Values after shutoff:"
echo "max_scale: $(cat /sys/class/led_anim/max_scale 2>/dev/null)"
echo "max_scale_lr: $(cat /sys/class/led_anim/max_scale_lr 2>/dev/null)"
echo "max_scale_rear: $(cat /sys/class/led_anim/max_scale_rear 2>/dev/null)"
echo "effect_enable: $(cat /sys/class/led_anim/effect_enable 2>/dev/null)"
'''

stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
print(stderr.read().decode())
c.close()
