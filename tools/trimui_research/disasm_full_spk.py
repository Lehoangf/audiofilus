import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

# We can use capstone if available, or disassemble basic aarch64
try:
    import capstone
    md = capstone.Cs(capstone.CS_ARCH_ARM64, capstone.CS_MODE_ARM)
    has_capstone = True
except ImportError:
    has_capstone = False

start_pc = 0xffffff80085c4bc0
end_pc = 0xffffff80085c4d60
fo_start = 2048 + (start_pc - 0xffffff8008080000)
fo_end = 2048 + (end_pc - 0xffffff8008080000)

code = k[fo_start:fo_end]

if has_capstone:
    for ins in md.disasm(code, start_pc):
        print(f"0x{ins.address:x}: {ins.mnemonic} {ins.op_str}")
else:
    print("No capstone, dumping hex:")
    for i in range(0, len(code), 4):
        pc = start_pc + i
        raw = struct.unpack('<I', code[i:i+4])[0]
        print(f"0x{pc:x}: {raw:08x}")
