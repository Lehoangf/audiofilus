import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

# Let's inspect bytes around 8242120
# A struct snd_soc_dapm_widget in Linux kernel:
# struct snd_soc_dapm_widget {
#   snd_soc_dapm_type id; (enum, 4 bytes)
#   const char *name; (8 bytes ptr)
#   const char *sname; (8 bytes ptr)
#   ...
#   int (*event)(struct snd_soc_dapm_widget*, struct snd_kcontrol *, int); (8 bytes ptr)
#   unsigned short event_flags;
# }

for base in range(8242120 - 128, 8242120 + 64, 8):
    val = struct.unpack('<Q', k[base:base+8])[0]
    if 0xffffff8008080000 <= val < 0xffffff8008080000 + len(k):
        fo = 2048 + (val - 0xffffff8008080000)
        end = k.find(b'\x00', fo)
        s = k[fo:end].decode('latin1', errors='ignore')
        print(f"Offset {base} (rel {base - 8242120}): {hex(val)} -> \"{s}\"")
    elif val == 0xffffff80085c4bc0:
        print(f"Offset {base} (rel {base - 8242120}): {hex(val)} -> spk_event")
