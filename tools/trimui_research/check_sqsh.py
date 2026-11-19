import struct

with open('F:/boot/knulli', 'rb') as f:
    hdr = f.read(96)

magic, inodes, mkfs_time, block_size, fragments, comp, block_log, flags, no_ids, s_major, s_minor = struct.unpack('<IIIIIHHHHHH', hdr[:32])
print(f"Magic: {hex(magic)}")
print(f"Compression ID: {comp}")
comp_names = {1: 'gzip', 2: 'lzma', 3: 'lzo', 4: 'xz', 5: 'lz4', 6: 'zstd'}
print(f"Compression: {comp_names.get(comp, 'unknown')}")
