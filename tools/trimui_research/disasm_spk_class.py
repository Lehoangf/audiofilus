import capstone

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

md = capstone.Cs(capstone.CS_ARCH_ARM64, capstone.CS_MODE_ARM)

# search where "speaker" class is created
start_pc = 0xffffff80082f8300
end_pc = 0xffffff80082f8460
fo = 2048 + (start_pc - 0xffffff8008080000)
code = k[fo:fo + (end_pc - start_pc)]

for ins in md.disasm(code, start_pc):
    print(f"0x{ins.address:x}: {ins.mnemonic} {ins.op_str}")
