import io, sys, json
sys.path.insert(0, r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\device-lab\skills\seven-hole-probe')
import seven_hole

calib_path = r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\algorithms\go\sevenhole\interpolation\testdata\prb'
big_cal = seven_hole.big_read_file(calib_path)

# sector 6 polygon
lines6 = seven_hole.big_create_line(big_cal, '6')
# 4 edges x 13 points or 4 points: print all
print('sector 6 polygon edges:')
for edge_idx, edge in enumerate(lines6):
    print(f'  edge {edge_idx} ({len(edge)} pts):')
    for i, p in enumerate(edge):
        print(f'    [{i}] a={p.get("a")} b={p.get("b")} ka={p.get("ka")} kb={p.get("kb")}')

# 测试 (ka=0.51384, kb=1.93960)
test_ka, test_kb = 0.5138354582787864, 1.9396019868201346
sign = seven_hole.point_in_polygon({'ka': test_ka, 'kb': test_kb}, lines6)
print(f'\ntest (ka={test_ka}, kb={test_kb}) sign: {sign}')

# 看一下网格点 (theta=30, phi=270) 应该在哪个 polygon 顶点
# sector 6 ab_dict: ('270', '330') -> b 边界 270, 330; a 边界 30, 45
print('\nsector 6 grid:')
for cal in big_cal.get('6', []):
    print(f'  a(θ)={cal["a"]} b(φ)={cal["b"]} ka={cal["ka"]} kb={cal["kb"]}')
