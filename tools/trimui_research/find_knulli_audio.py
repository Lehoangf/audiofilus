import zstandard as zstd

dctx = zstd.ZstdDecompressor()

with open('F:/boot/knulli', 'rb') as f:
    chunk_size = 32 * 1024 * 1024
    offset = 0
    magic = b'\x28\xb5\x2f\xfd'
    
    found = False
    while offset < 2039803904 and not found:
        f.seek(offset)
        buf = f.read(chunk_size)
        if not buf:
            break
        
        pos = buf.find(magic)
        while pos != -1:
            try:
                decomp = dctx.decompress(buf[pos:pos+131072], max_output_size=262144)
                if b'set speakers' in decomp or b'set headphone' in decomp or b'knulli-audio set' in decomp:
                    print(f"=== Found at offset {offset + pos} ===")
                    for line in decomp.splitlines():
                        if any(w in line for w in [b'speakers', b'headphone', b'LINEOUT', b'HpSpeaker', b'Headphone', b'DAC', b'set()']):
                            print(line.decode('utf-8', errors='ignore'))
            except Exception:
                pass
            pos = buf.find(magic, pos + 1)
            
        offset += chunk_size - 65536
