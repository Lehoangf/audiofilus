import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

def get_str(ptr):
    if 0xffffff8008080000 <= ptr < 0xffffff8008080000 + len(k):
        fo = 2048 + (ptr - 0xffffff8008080000)
        end = k.find(b'\x00', fo)
        return k[fo:end].decode('latin1', errors='ignore')
    return hex(ptr)

start = 10698456 - 64
for off in range(start, start + 500, 8):
    p = struct.unpack('<Q', k[off:off+8])[0]
    s = get_str(p)
    print(f"Offset {off}: {hex(p)} -> {s}")
