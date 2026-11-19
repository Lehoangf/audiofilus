import zstandard as zstd

dctx = zstd.ZstdDecompressor()

with open('F:/boot/knulli', 'rb') as f:
    f.seek(354478147)
    buf = f.read(131072)
    decomp = dctx.decompress(buf, max_output_size=262144)
    text = decomp.decode('utf-8', errors='ignore')
    pos = text.find('OutputL Mixer DACL')
    if pos != -1:
        print(text[max(0, pos-500):min(len(text), pos+1500)])
