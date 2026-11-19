with open('F:/partitions/boot.img', 'rb') as f:
    kernel = f.read()

import struct

# strb w..., [x..., #136] -> strb with imm12 = 136 (0x88)
# STRB (immediate, unsigned offset):
# size=00, 111 001 00, imm12, Rn, Rt
# 0x39000000 | (imm12 << 10)
# imm12 = 136 (0x88) -> (136 << 10) = 0x22000
# opcode: 0x39022000
mask = 0xffc00000
target = 0x39022000

for i in range(2048, 8000000, 4):
    instr = struct.unpack('<I', kernel[i:i+4])[0]
    if (instr & mask) == target:
        rt = instr & 0x1f
        rn = (instr >> 5) & 0x1f
        pc = 0xffffff8008080000 + (i - 2048)
        print(f'STRB at {hex(pc)}: [x{rn}, #136] = w{rt}')
        for j in range(i-20, i+20, 4):
            ins = struct.unpack('<I', kernel[j:j+4])[0]
            print(f'  {hex(0xffffff8008080000 + j - 2048)}: {hex(ins)}')