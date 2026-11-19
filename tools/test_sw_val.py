import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.1.7', username='root', password='linux', timeout=5)

cmd = '''
python -c "
import struct, fcntl
with open('/dev/input/event3', 'rb') as f:
    buf = bytearray(32)
    fcntl.ioctl(f.fileno(), 0x8020451b, buf)
    print('Current Switch Code 1 state:', (buf[0] >> 1) & 1)
"
'''

stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
print(stderr.read().decode())
c.close()
