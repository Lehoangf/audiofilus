import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

base = 8243312 - 16
for i in [8243232-16, 8243312-16, 8243392-16]:
    name_ptr = struct.unpack('<Q', k[i+16:i+24])[0]
    fo = 2048 + (name_ptr - 0xffffff8008080000)
    end = k.find(b'\x00', fo)
    name = k[fo:end].decode('latin1')
    info = struct.unpack('<Q', k[i+40:i+48])[0]
    get = struct.unpack('<Q', k[i+48:i+56])[0]
    put = struct.unpack('<Q', k[i+56:i+64])[0]
    print(f'Offset {i}: name="{name}" info={hex(info)} get={hex(get)} put={hex(put)}')
