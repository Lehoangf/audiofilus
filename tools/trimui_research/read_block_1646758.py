import zstandard as zstd

dctx = zstd.ZstdDecompressor()

with open('F:/boot/knulli', 'rb') as f:
    f.seek(1646758)
    buf = f.read(131072)
    decomp = dctx.decompress(buf, max_output_size=262144)
    print(decomp.decode('utf-8', errors='ignore'))
