import paramiko

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect('192.168.1.7', username='root', password='linux', timeout=5)

cmd = '''
grep -A 25 "goroutine 1 " /tmp/fn-daemon.log
'''

stdin, stdout, stderr = c.exec_command(cmd)
print(stdout.read().decode())
c.close()
