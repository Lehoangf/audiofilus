with open('F:/partitions/boot.img', 'rb') as f:
    kernel = f.read()

import struct

target_page = 0xffffff8008990000

for i in range(2048, 8000000, 4):
    instr = struct.unpack('<I', kernel[i:i+4])[0]
    # Check if ADRP: (instr & 0x9f000000) == 0x90000000
    if (instr & 0x9f000000) == 0x90000000:
        immlo = (instr >> 29) & 3
        immhi = (instr >> 5) & 0x7ffff
        imm = (immhi << 2) | immlo
        if imm >= (1 << 20): imm -= (1 << 21)
        pc = 0xffffff8008080000 + (i - 2048)
        pc_page = pc & ~0xfff
        page = pc_page + (imm << 12)
        if page == target_page:
            # Check next instruction: ADD or LDR
            next_ins = struct.unpack('<I', kernel[i+4:i+8])[0]
            # ADD (immediate): (next_ins & 0xff800000) == 0x91000000
            if (next_ins & 0xff800000) == 0x91000000:
                imm12 = (next_ins >> 10) & 0xfff
                target_addr = page + imm12
                # Check target string:
                fo = 2048 + (target_addr - 0xffffff8008080000)
                if 0 <= fo < len(kernel):
                    end = kernel.find(b'\x00', fo)
                    s = kernel[fo:end].decode('latin1', errors='ignore')
                    if any(k in s.lower() for k in ['spk', 'speaker', 'headphone', 'pa_']):
                        print(f'PC {hex(pc)}: {s}')