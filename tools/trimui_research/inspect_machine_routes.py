import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

def get_str(ptr):
    if 0xffffff8008080000 <= ptr < 0xffffff8008080000 + len(k):
        fo = 2048 + (ptr - 0xffffff8008080000)
        end = k.find(b'\x00', fo)
        return k[fo:end].decode('latin1', errors='ignore')
    return hex(ptr)

base = 8243368
for i in range(base - 32 * 5, base + 32 * 5, 32):
    sink_p, ctrl_p, src_p, conn_p = struct.unpack('<QQQQ', k[i:i+32])
    sink = get_str(sink_p)
    ctrl = get_str(ctrl_p) if ctrl_p else "None"
    src = get_str(src_p)
    print(f"Machine Route at {i}: sink='{sink}' <- ctrl='{ctrl}' <- src='{src}'")
