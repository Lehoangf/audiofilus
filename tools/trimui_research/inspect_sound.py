with open('dtb.dtb', 'rb') as f:
    data = f.read()

import struct

p = 0x58
off_strings = 0x20a1c
strings_raw = data[off_strings:len(data)]

def get_str(offset):
    end = strings_raw.find(b'\x00', offset)
    return strings_raw[offset:end].decode('latin1')

node_stack = []
nodes = {}

while p < off_strings:
    tag = struct.unpack('>I', data[p:p+4])[0]
    p += 4
    if tag == 1:
        end = data.find(b'\x00', p)
        name = data[p:end].decode('latin1')
        p = (end + 4) & ~3
        node_stack.append(name)
        curr = '/'.join(node_stack)
        nodes[curr] = {}
    elif tag == 2:
        if node_stack: node_stack.pop()
    elif tag == 3:
        val_len, name_off = struct.unpack('>II', data[p:p+8])
        p += 8
        val = data[p:p+val_len]
        p = (p + val_len + 3) & ~3
        prop = get_str(name_off)
        curr = '/'.join(node_stack)
        if curr in nodes: nodes[curr][prop] = val
    elif tag == 9: break

for k, v in nodes.items():
    if 'codec' in k or 'sound' in k:
        print('=== NODE:', k, '===')
        for pk, pv in v.items():
            print(f'  {pk}: {repr(pv)}')