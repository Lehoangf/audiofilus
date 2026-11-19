import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

def get_str(ptr):
    if 0xffffff8008080000 <= ptr < 0xffffff8008080000 + len(k):
        fo = 2048 + (ptr - 0xffffff8008080000)
        end = k.find(b'\x00', fo)
        return k[fo:end].decode('latin1', errors='ignore')
    return hex(ptr)

# Search for function pointer 0xffffff80082f84c0
target = 0xffffff80082f84c0
for i in range(0, len(k)-8, 8):
    val = struct.unpack('<Q', k[i:i+8])[0]
    if val == target:
        print(f"Found function pointer at {i} (PC: {hex(0xffffff8008080000 + i - 2048)})")
        # struct device_attribute has:
        # struct attribute attr { const char *name; umode_t mode; }
        # ssize_t (*show)(...);
        # ssize_t (*store)(...);
        # So name is at i - 16 or i - 24
        for off in range(i - 32, i + 16, 8):
            p = struct.unpack('<Q', k[off:off+8])[0]
            print(f"  rel {off - i}: {hex(p)} -> {get_str(p)}")
