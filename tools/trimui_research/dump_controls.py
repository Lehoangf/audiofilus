with open('F:/partitions/boot.img', 'rb') as f:
    kernel = f.read()

import struct

pos = 8236024 - 16
for i in range(pos - 64 * 5, pos + 64 * 25, 64):
    name_ptr = struct.unpack('<Q', kernel[i+16:i+24])[0]
    if 0xffffff8008080000 <= name_ptr < 0xffffff8008080000 + len(kernel):
        fo = 2048 + (name_ptr - 0xffffff8008080000)
        end = kernel.find(b'\x00', fo)
        name = kernel[fo:end].decode('latin1')
        priv = struct.unpack('<Q', kernel[i+56:i+64])[0]
        print(f'Control at {i}: \"{name}\" (priv: {hex(priv)})')