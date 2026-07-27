import io
import sys

sys.path.insert(0, r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\device-lab\skills\seven-hole-probe')
import seven_hole

# 测试输入：大角度 2 区 row 18 (phi=50, theta=35)
hd = {
    "p1": 2147.133, "p2": 3913.267, "p3": 5.050, "p4": -3127.817,
    "p5": -3355.217, "p6": -2708.183, "p7": 1173.833,
    "t": 28.0, "pa": 98891.0,
}

# 1. 计算 inner ka/kb
little_pt = seven_hole.little_cal_kakb(hd)
print('inner ka,kb:', little_pt)

# 2. 加载内区 PRB 看看是否在内区多边形内
calib_path = r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\algorithms\go\sevenhole\interpolation\testdata\prb'
little_lines = seven_hole.little_create_line(seven_hole.little_read_file(calib_path))
sign = seven_hole.point_in_polygon(little_pt, little_lines)
print('inner polygon sign (0/1=inside, -1=outside):', sign)

# 3. 用 big_max_pressure 看看 first/second
max_keys = seven_hole.big_max_pressure(hd)
print('big first/second:', max_keys)

# 4. 用 first 计算大角度 ka/kb
d1 = seven_hole.big_cal_kakb(hd, max_keys['first'])
big_lines1 = seven_hole.big_create_line(seven_hole.big_read_file(calib_path), max_keys['first'])
sign1 = seven_hole.point_in_polygon(d1, big_lines1)
print('first sector', max_keys['first'], 'ka,kb:', d1, 'sign:', sign1)

# 5. 调用 cal_ab 看看 Python 怎么处理
import json, tempfile, os
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
