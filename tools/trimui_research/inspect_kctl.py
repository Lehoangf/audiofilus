import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

def get_str(ptr):
    if 0xffffff8008080000 <= ptr < 0xffffff8008080000 + len(k):
        fo = 2048 + (ptr - 0xffffff8008080000)
        end = k.find(b'\x00', fo)
        return k[fo:end].decode('latin1', errors='ignore')
    return hex(ptr)

# 8235688 was "digital volume" pointer
ptr_fo = 8235688
for off in range(ptr_fo - 32, ptr_fo + 48, 8):
    p = struct.unpack('<Q', k[off:off+8])[0]
    print(f"rel {off - ptr_fo}: {hex(p)} -> {get_str(p)}")
