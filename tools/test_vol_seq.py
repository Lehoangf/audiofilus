import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.1.7', username='root', password='linux', timeout=5)

cmd = '''
echo "1. Current volume:"
knulli-audio getSystemVolume
pactl get-sink-volume @DEFAULT_SINK@

echo "2. Setting volume to 90:"
knulli-audio setSystemVolume 90
pactl set-sink-volume @DEFAULT_SINK@ 90%

echo "3. Query after setting 90:"
knulli-audio getSystemVolume
pactl get-sink-volume @DEFAULT_SINK@

echo "4. Setting volume to 0 (mute):"
knulli-audio setSystemVolume 0
pactl set-sink-volume @DEFAULT_SINK@ 0%
pactl set-sink-mute @DEFAULT_SINK@ 1

echo "5. Query after setting 0:"
knulli-audio getSystemVolume
pactl get-sink-volume @DEFAULT_SINK@

echo "6. Setting volume back to 90:"
pactl set-sink-mute @DEFAULT_SINK@ 0
knulli-audio setSystemVolume 90
pactl set-sink-volume @DEFAULT_SINK@ 90%

echo "7. Final query:"
knulli-audio getSystemVolume
pactl get-sink-volume @DEFAULT_SINK@
'''

stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
print(stderr.read().decode())
c.close()
