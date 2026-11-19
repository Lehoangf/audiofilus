with open('F:/partitions/boot.img', 'rb') as f:
    kernel = f.read()

import struct

# String 'spk status mute, skip SPK PA' is at file offset 9505886
# String 'hp status in, skip SPK PA' is at file offset 9505858
# Let's search in the entire kernel for occurrences of 9505886
# In 64-bit kernel, string pointers are 8 bytes:
str_va = 0xffffff8008080000 + (9505886 - 2048)
print('str_va:', hex(str_va))
ptr_bytes = struct.pack('<Q', str_va)
pos = kernel.find(ptr_bytes)
print('Pointer to spk status mute:', pos)

# Also let's check 'Headphone exist, skip speaker.' at 9507871
str2_va = 0xffffff8008080000 + (9507871 - 2048)
print('str2_va:', hex(str2_va))
pos2 = kernel.find(struct.pack('<Q', str2_va))
print('Pointer to Headphone exist:', pos2)