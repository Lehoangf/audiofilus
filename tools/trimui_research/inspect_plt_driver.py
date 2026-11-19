import struct

with open('F:/partitions/boot.img', 'rb') as f:
    k = f.read()

# struct platform_driver:
# int (*probe)(struct platform_device *);
# int (*remove)(struct platform_device *);
# void (*shutdown)(struct platform_device *);
# int (*suspend)(struct platform_device *, pm_message_t state);
# int (*resume)(struct platform_device *);
# struct device_driver driver; (name is at driver.name)

# 10698456 is driver.name!
# driver struct is inside platform_driver at offset 10698456 - 48 (or similar)
# Let's inspect around 10698456:
base = 10698456
for off in range(base - 64, base + 64, 8):
    p = struct.unpack('<Q', k[off:off+8])[0]
    print(f"rel {off - base}: {hex(p)}")
