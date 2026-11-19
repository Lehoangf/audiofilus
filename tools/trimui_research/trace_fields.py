with open('F:/partitions/boot.img', 'rb') as f:
    kernel = f.read()

import struct

start_pc = 0xffffff80085c4000
end_pc = 0xffffff80085c7000

for pc in range(start_pc, end_pc, 4):
    fo = 2048 + (pc - 0xffffff8008080000)
    instr = struct.unpack('<I', kernel[fo:fo+4])[0]
    # Check if STRB or LDRB with imm12 == 136 (0x88)
    # STRB/LDRB unsigned: (instr >> 10) & 0xfff == 0x88
    # opcode is 0x39000000 or 0x39400000
    if (instr & 0xbfc00000) == 0x39000000 and ((instr >> 10) & 0xfff) in (136, 137, 138):
        is_load = (instr >> 22) & 1
        imm = (instr >> 10) & 0xfff
        rn = (instr >> 5) & 0x1f
        rt = instr & 0x1f
        op = 'LDRB' if is_load else 'STRB'
        print(f'{hex(pc)}: {op} w{rt}, [x{rn}, #{imm}]')