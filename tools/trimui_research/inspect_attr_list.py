import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

def get_str(ptr):
    if 0xffffff8008080000 <= ptr < 0xffffff8008080000 + len(k):
        fo = 2048 + (ptr - 0xffffff8008080000)
        end = k.find(b'\x00', fo)
        return k[fo:end].decode('latin1', errors='ignore')
    return hex(ptr)

base = 10698992
for off in range(base - 32, base + 64, 8):
    attr_ptr = struct.unpack('<Q', k[off:off+8])[0]
    name = "NULL"
    if attr_ptr:
        # name is at attr_ptr
        fo = 2048 + (attr_ptr - 0xffffff8008080000)
        if 0 <= fo < len(k):
            name_ptr = struct.unpack('<Q', k[fo:fo+8])[0]
            name = get_str(name_ptr)
    print(f"offset {off}: attr={hex(attr_ptr)} (name='{name}')")
