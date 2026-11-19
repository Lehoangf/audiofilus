import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

def get_str(ptr):
    if 0xffffff8008080000 <= ptr < 0xffffff8008080000 + len(k):
        fo = 2048 + (ptr - 0xffffff8008080000)
        end = k.find(b'\x00', fo)
        return k[fo:end].decode('latin1', errors='ignore')
    return hex(ptr)

# Let's find where "HpSpeaker Switch" (0xffffff8008991265) is referenced:
# Earlier: 0xffffff8008991265 was found at 8243312!
# If 8243312 is the pointer to "HpSpeaker Switch", let's see what is at 8243312 - 8, etc.
ptr_fo = 8243312
for off in range(ptr_fo - 64, ptr_fo + 128, 8):
    val = struct.unpack('<Q', k[off:off+8])[0]
    print(f"offset {off}: {hex(val)} -> '{get_str(val)}'")
