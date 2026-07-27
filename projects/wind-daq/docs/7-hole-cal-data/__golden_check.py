import json

with open(r'c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\algorithms\go\sevenhole\interpolation\testdata\golden\golden.json', encoding='utf-8') as f:
    entries = json.load(f)

print('total entries:', len(entries))
for idx in [0, 87, 169, 221, 273, 325, 377, 429]:
    for e in entries:
        if e['index'] == idx:
            print(f'idx {idx}: mode={e["mode"]} sector={e["sector"]} fallback={e["fallback"]}')
            print(f'  input: {e["input"]}')
            print(f'  output: a={e["output"]["alpha"]:.4f} b={e["output"]["beta"]:.4f} pt={e["output"]["pt"]:.2f} ps={e["output"]["ps"]:.2f} ma={e["output"]["ma"]:.4f}')
            break
