import io

target_p1 = 2147.133
target_p2 = 3913.267
target_p3 = 5.050
target_p4 = -3127.817
target_p5 = -3355.217
target_p6 = -2708.183
target_p7 = 1173.833

for fname in [
    'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(小角度区).csv',
    'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(大角度1区).csv',
    'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(大角度2区).csv',
    'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(大角度3区).csv',
    'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(大角度4区).csv',
    'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(大角度5区).csv',
    'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(大角度6区).csv',
]:
    with io.open(fname, encoding='gbk') as f:
        rows = f.read().splitlines()
    for i, row in enumerate(rows[1:], start=1):
        parts = row.split(',')
        p1, p2, p3, p4, p5, p6, p7 = [float(parts[k]) for k in [5, 6, 7, 8, 9, 10, 11]]
        if (abs(p1-target_p1)<0.01 and abs(p2-target_p2)<0.01 and abs(p7-target_p7)<0.01):
            print(fname, 'row', i, ': col0=', parts[0], 'col1=', parts[1],
                  'ka=', parts[12], 'kb=', parts[13])
            print('  P:', p1, p2, p3, p4, p5, p6, p7)
