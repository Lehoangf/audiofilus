import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

target1 = 0xffffff8008c2adcc
target2 = 0xffffff8008c2add0

start_pc = 0xffffff8008080000
end_pc = 0xffffff8008080000 + len(k)

for pc in range(start_pc, end_pc - 8, 4):
    fo = 2048 + (pc - 0xffffff8008080000)
    instr = struct.unpack('<I', k[fo:fo+4])[0]
    # Check if ADRP
    if (instr & 0x9f000000) == 0x90000000:
        immlo = (instr >> 29) & 3
        immhi = (instr >> 5) & 0x7ffff
        imm = ((immhi << 2) | immlo) << 12
        if imm & (1 << 32): imm -= (1 << 33)
        base = (pc & ~0xfff) + imm
        if base == 0xffffff8008c2a000:
            rd = instr & 0x1f
            next_instr = struct.unpack('<I', k[fo+4:fo+8])[0]
            rn = (next_instr >> 5) & 0x1f
            rt = next_instr & 0x1f
            imm12 = (next_instr >> 10) & 0xfff
            addr = base + (imm12 << 2)
            if rn == rd and addr in (target1, target2):
                is_load = (next_instr >> 22) & 1
                op_str = "LDR" if is_load else "STR"
                which = "hp_status" if addr == target1 else "spk_status"
                print(f"{hex(pc)}: ADRP + {op_str} w{rt}, [{hex(addr)}] ({which})")
