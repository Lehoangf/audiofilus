with open('F:/partitions/boot.img', 'rb') as f:
    kernel = f.read()

import struct

start_pc = 0xffffff80085c4c30
end_pc = 0xffffff80085c4d30
start_off = 2048 + (start_pc - 0xffffff8008080000)

for pc in range(start_pc, end_pc, 4):
    fo = 2048 + (pc - 0xffffff8008080000)
    instr = struct.unpack('<I', kernel[fo:fo+4])[0]
    print(f'{hex(pc)}: {hex(instr)}')