with open('F:/boot/knulli', 'rb') as f:
    # search in chunks
    chunk_size = 50 * 1024 * 1024
    overlap = 1024 * 1024
    offset = 0
    pattern = b'trimui-volume-attenuate'
    pattern2 = b'setSystemVolume'
    
    while True:
        f.seek(offset)
        buf = f.read(chunk_size)
        if not buf:
            break
        
        pos = buf.find(b'#!/bin/sh')
        while pos != -1:
            end = buf.find(b'\n\x00', pos)
            if end != -1 and (end - pos) < 4000:
                script = buf[pos:end]
                if b'trimui' in script or b'volume-attenuate' in script or b'knulli-audio' in script or b'attenuate' in script:
                    print(f"--- Found script at offset {offset + pos} ---")
                    try:
                        print(script.decode('utf-8'))
                    except Exception:
                        print(repr(script))
            pos = buf.find(b'#!/bin/sh', pos + 1)
            
        offset += chunk_size - overlap
