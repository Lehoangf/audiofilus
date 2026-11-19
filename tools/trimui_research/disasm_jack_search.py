import capstone

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

md = capstone.Cs(capstone.CS_ARCH_ARM64, capstone.CS_MODE_ARM)

start_pc = 0xffffff80085c6020
end_pc = 0xffffff80085c6500
fo = 2048 + (start_pc - 0xffffff8008080000)
code = k[fo:fo + (end_pc - start_pc)]

for ins in md.disasm(code, start_pc):
    if any(k in ins.op_str for k in ['0xc2a', '0xdc', '0xdd', 'dcc', 'dd0']):
        print(f"0x{ins.address:x}: {ins.mnemonic} {ins.op_str}")
