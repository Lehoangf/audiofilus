with open('F:/boot/knulli', 'rb') as f:
    chunk_size = 50 * 1024 * 1024
    overlap = 1024 * 1024
    offset = 0
    
    while True:
        f.seek(offset)
        buf = f.read(chunk_size)
        if not buf:
            break
        
        pos = buf.find(b'LINEOUT volume')
        while pos != -1:
            start = max(0, pos - 200)
            end = min(len(buf), pos + 200)
            snippet = buf[start:end]
            print(f"--- Found at {offset + pos} ---")
            print(repr(snippet))
            pos = buf.find(b'LINEOUT volume', pos + 1)
            
        offset += chunk_size - overlap
