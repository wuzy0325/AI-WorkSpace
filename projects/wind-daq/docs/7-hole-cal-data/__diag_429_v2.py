import io, sys, json, tempfile, os
sys.path.insert(0, r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\device-lab\skills\seven-hole-probe')
import seven_hole

# idx 429 输入 (来自大角度 6 区 row 1: theta=30, phi=270, ka=0.514, kb=1.940)
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

# 2. outer - 传字符串
max_keys = seven_hole.big_max_pressure(hd)
print('big first/second:', max_keys)

big_cal = seven_hole.big_read_file(calib_path)
for cand in [max_keys['first'], max_keys['second']]:  # 字符串
    d = seven_hole.big_cal_kakb(hd, cand)
    lines = seven_hole.big_create_line(big_cal, cand)
    s = seven_hole.point_in_polygon(d, lines)
    print(f'sector {cand} ka,kb:', d, 'sign:', s)

# 3. Python cal_ab 完整调用
with tempfile.NamedTemporaryFile('w', suffix='.json', delete=False, encoding='utf-8') as f:
    json.dump(hd, f)
    tmp = f.name
try:
    out = seven_hole.cal_ab(tmp, calib_path)
    if isinstance(out, str) and out != 'no-return':
        result = json.loads(out)
        print('Python cal_ab result:', result)
    else:
        print('Python cal_ab returned:', out)
finally:
    os.unlink(tmp)
