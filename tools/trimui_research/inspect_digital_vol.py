import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

# Search for "digital volume" control in kernel
# Earlier dump_controls.py: Control at 8235688: "digital volume" (priv: 0xffffff80085bdde4)
fo = 8235688 - 16
fields = struct.unpack('<IIIIQQQQQ', k[fo:fo+56])
info, get, put, priv = fields[5], fields[6], fields[7], fields[4] # let's check
print("info:", hex(info), "get:", hex(get), "put:", hex(put))

# Let's inspect the put function
import capstone
md = capstone.Cs(capstone.CS_ARCH_ARM64, capstone.CS_MODE_ARM)

# In ALSA, SOC_SINGLE / SOC_DOUBLE stores reg, shift, max, invert in priv
priv_val = struct.unpack('<Q', k[fo+56:fo+64])[0]
print("priv:", hex(priv_val))
priv_fo = 2048 + (priv_val - 0xffffff8008080000)
reg, shift, rshift, max_val, invert = struct.unpack('<IIIIQ', k[priv_fo:priv_fo+24])
print(f"reg={hex(reg)}, shift={shift}, rshift={rshift}, max={max_val}, invert={invert}")
