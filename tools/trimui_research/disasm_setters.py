import struct
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

dis(0xffffff80085c5140, 25)
dis(0xffffff80085c52c0, 20)
dis(0xffffff80085c57c0, 20)
dis(0xffffff80085c5910, 25)
