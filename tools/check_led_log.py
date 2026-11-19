import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.1.20', username='root', password='linux', timeout=5)

cmd = '''
echo "=== CURRENT DAEMON LOG ==="
cat /tmp/fn-daemon.log

echo "=== CURRENT LED_ANIM VALUES ==="
for f in /sys/class/led_anim/*; do
    if [ -f "$f" ]; then
        echo "$(basename $f): $(cat $f 2>/dev/null)"
    fi
done

echo "=== SEARCHING EXISTING LED SCRIPTS IN KNULLI ==="
find /usr /etc /opt -name "*led*" 2>/dev/null
'''

stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
print(stderr.read().decode())
c.close()
