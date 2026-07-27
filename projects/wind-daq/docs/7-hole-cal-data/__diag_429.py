import io, sys, json, tempfile, os
sys.path.insert(0, r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\device-lab\skills\seven-hole-probe')
import seven_hole

# idx 429 输入
hd = {
    "p1": -591.933, "p2": -2580.233, "p3": -2480.633, "p4": -947.4,
    "p5": 3172.817, "p6": 3231.433, "p7": 2234.083,
    "t": 28.0, "pa": 98925.0,
}

calib_path = r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\algorithms\go\sevenhole\interpolation\testdata\prb'

# 1. inner
little_pt = seven_hole.little_cal_kakb(hd)
little_lines = seven_hole.little_create_line(seven_hole.little_read_file(calib_path))
sign = seven_hole.point_in_polygon(little_pt, little_lines)
print('inner ka,kb:', little_pt, 'sign:', sign)

# 2. outer
max_keys = seven_hole.big_max_pressure(hd)
print('big first/second:', max_keys)

# 3. 用 CSV 数据源测试
csv_dir = r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\wind-daq\docs\7-hole-cal-data'
target = (hd['p1'], hd['p2'], hd['p7'])
for fname in ['(小角度区)', '(大角度1区)', '(大角度2区)', '(大角度3区)', '(大角度4区)', '(大角度5区)', '(大角度6区)']:
    full = os.path.join(csv_dir, f'W532.202608.P.7H.1-01-85米每秒（0.242Ma）{fname}.csv')
    if not os.path.exists(full):
        continue
    with io.open(full, encoding='gbk') as f:
        rows = f.read().splitlines()
    for i, row in enumerate(rows[1:], start=1):
        parts = row.split(',')
        p1, p2, p7 = [float(parts[k]) for k in [5, 6, 11]]
        if abs(p1-target[0])<0.01 and abs(p2-target[1])<0.01 and abs(p7-target[2])<0.01:
            print(f'CSV match: {fname} row {i}: col0(phi)={parts[0]} col1(theta)={parts[1]} ka={parts[12]} kb={parts[13]}')
            print(f'  full row: {row}')

# 4. golden.json idx 429
with open(r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\algorithms\go\sevenhole\interpolation\testdata\golden\golden.json', encoding='utf-8') as f:
    entries = json.load(f)
for e in entries:
    if e['index'] == 429:
        print(f'golden idx 429: mode={e["mode"]} sector={e["sector"]} fallback={e["fallback"]}')
        print(f'  output: {e["output"]}')
        break

# 5. 计算 idx 429 在 dataset 中的位置
# dataset 顺序：inner (169), sector 1 (52), sector 2 (52), sector 3 (52), sector 4 (52), sector 5 (52), sector 6 (52)
# idx 429 = 169 + 52*5 + 0  (sector 6 row 1)
# 或 = 169 + 52*4 + 51 = 428 (sector 5 last)
# 429 - 169 = 260, 260 / 52 = 5, 260 % 52 = 0 -> sector 6 row 1
# 但其实 sector 索引可能不同
# 429 - 169 = 260 -> sector 6 (index 5), row 0 (theta=30, phi=270 or 330)
print('expected dataset position: idx 429 = sector 6 row 1 (theta=30, phi=270)')
