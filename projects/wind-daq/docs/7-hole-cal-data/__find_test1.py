import json

with open(r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\algorithms\go\sevenhole\interpolation\testdata\golden\golden.json', encoding='utf-8') as f:
    entries = json.load(f)

# 测试 1 的输入
target = (2147.133, 3913.267, 1173.833)
for e in entries:
    inp = e['input']
    if abs(inp['p1']-target[0])<0.01 and abs(inp['p2']-target[1])<0.01 and abs(inp['p7']-target[2])<0.01:
        print(f'idx {e["index"]}: mode={e["mode"]} sector={e["sector"]} fallback={e["fallback"]}')
        print(f'  output: a={e["output"]["alpha"]:.4f} b={e["output"]["beta"]:.4f} pt={e["output"]["pt"]:.2f} ps={e["output"]["ps"]:.2f}')
        break

# 统计 mode
from collections import Counter
modes = Counter(e['mode'] for e in entries)
print('\nmode counts:', dict(modes))

# 统计 mode=little 但实际数据来自大角度 CSV 的（"误判"）
# dataset 顺序：inner (idx 0..168), sector 1 (169..220), sector 2 (221..272), ..., sector 6 (429..480)
for e in entries:
    if e['index'] >= 169 and e['mode'] == 'little':
        # 大角度 CSV 的点被误判为内区
        pass
# 输出每个 sector 中各 mode 的数量
import collections
sector_mode = collections.defaultdict(lambda: collections.Counter())
for e in entries:
    if e['index'] < 169:
        sector_mode['inner'][e['mode']] += 1
    else:
        s = (e['index'] - 169) // 52 + 1
        sector_mode[f'outer{s}'][e['mode']] += 1
for k, v in sector_mode.items():
    print(f'  {k}: {dict(v)}')
