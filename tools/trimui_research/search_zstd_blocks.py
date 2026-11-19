import zstandard as zstd

dctx = zstd.ZstdDecompressor()

with open('F:/boot/knulli', 'rb') as f:
    chunk_size = 32 * 1024 * 1024
    offset = 0
    pattern = b'#!/bin/'
    magic = b'\x28\xb5\x2f\xfd'
    
    found_count = 0
    while offset < 2039803904:
        f.seek(offset)
        buf = f.read(chunk_size)
        if not buf:
            break
        
        pos = buf.find(magic)
        while pos != -1:
            try:
                decomp = dctx.decompress(buf[pos:pos+131072], max_output_size=262144)
                if b'trimui' in decomp or b'knulli-audio' in decomp or b'attenuate' in decomp:
                    print(f"=== Found match at file offset {offset + pos} ===")
                    for line in decomp.splitlines():
                        if any(w in line for w in [b'amixer', b'sound', b'volume', b'attenuate', b'Headphone', b'HpSpeaker', b'LINEOUT', b'knulli-audio', b'brick']):
                            print(line.decode('utf-8', errors='ignore'))
                    found_count += 1
                    if found_count > 10:
                        break
            except Exception:
                pass
            pos = buf.find(magic, pos + 1)
            
        if found_count > 10:
            break
        offset += chunk_size - 65536
