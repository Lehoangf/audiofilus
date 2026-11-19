import capstone

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

md = capstone.Cs(capstone.CS_ARCH_ARM64, capstone.CS_MODE_ARM)

def dis(start_pc, count):
    fo = 2048 + (start_pc - 0xffffff8008080000)
    code = k[fo:fo + count*4]
    print(f"=== Disasm from {hex(start_pc)} ===")
    for ins in md.disasm(code, start_pc):
        print(f"0x{ins.address:x}: {ins.mnemonic} {ins.op_str}")

dis(0xffffff80085c5fb0, 30)
dis(0xffffff80085c65a0, 30)
