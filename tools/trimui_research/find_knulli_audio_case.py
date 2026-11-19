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
                if b'setSystemVolume)' in decomp:
                    print(f"=== Found knulli-audio at offset {offset + pos} ===")
                    print(decomp.decode('utf-8', errors='ignore'))
                    found = True
                    break
            except Exception:
                pass
            pos = buf.find(magic, pos + 1)
            
        offset += chunk_size - 65536
