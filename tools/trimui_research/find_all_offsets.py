import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

start_pc = 0xffffff80085b0000
end_pc = 0xffffff80085d0000

for pc in range(start_pc, end_pc, 4):
    fo = 2048 + (pc - 0xffffff8008080000)
    instr = struct.unpack('<I', k[fo:fo+4])[0]
    # Check LDRB / STRB with imm12 == 0x8a (138) or (offset 0xa with base + 0x80)
    # LDRB/STRB unsigned immediate: bits [31:22] = 001110010x (STRB) or 0011100101 (LDRB)
    # rn = (instr >> 5) & 0x1f
    # rt = instr & 0x1f
    # imm = (instr >> 10) & 0xfff
    if (instr & 0xbfc00000) == 0x39000000:
        imm = (instr >> 10) & 0xfff
        rn = (instr >> 5) & 0x1f
        rt = instr & 0x1f
        is_load = (instr >> 22) & 1
        op = 'LDRB' if is_load else 'STRB'
        if imm in (0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e, 0x8f, 0x08, 0x09, 0x0a):
            print(f"{hex(pc)}: {op} w{rt}, [x{rn}, #{hex(imm)}]")
