import io
import sys

with io.open(r'W532.202608.P.7H.1-01-85米每秒（0.242Ma）(大角度2区).csv', encoding='gbk') as f:
    rows = f.read().splitlines()
print('total data rows:', len(rows)-1)
for i in [1, 13, 14, 19, 20, 26, 39, 52]:
    parts = rows[i].split(',')
    print('  row', i, ': col0=', parts[0], 'col1=', parts[1],
          'ka=', parts[12], 'kb=', parts[13],
          'P1=', parts[5], 'P2=', parts[6], 'P3=', parts[7], 'P4=', parts[8],
          'P5=', parts[9], 'P6=', parts[10], 'P7=', parts[11])
