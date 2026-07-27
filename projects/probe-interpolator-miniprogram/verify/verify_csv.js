// CSV 批量逻辑校验（Node 端，纯 JS，不依赖 wx）。
// 用法：node verify_csv.js
// 覆盖：
//   1) parseCsv / toCsv 往返一致（含引号、逗号、BOM）
//   2) runBatch：列匹配、缺列(ERROR)、异常(ERROR)、有效行、resultColumns 输出顺序
//   3) 三个探针的 resultColumns 定义均是各 reference JSON 中 `go` 输出键的子集（防止列名写错导致空白列）

const fs = require('fs');
const path = require('path');
const { parseCsv, toCsv, detectColumns, runBatch } = require('../utils/csv-batch.js');

let pass = 0;
let fail = 0;
function ok(cond, name) {
  if (cond) { pass++; console.log('  PASS ' + name); }
  else { fail++; console.log('  FAIL ' + name); }
}
function approx(a, b, tol) { return Math.abs(a - b) <= tol; }

// ---------- 1) parseCsv / toCsv 往返 ----------
console.log('[1] parseCsv / toCsv 往返');
{
  const src = 'P1,P2,Patm,TAtm\n1.5,2.5,101325,20\n,"comma, field",,30\n3,4,101325,20\r\n';
  const parsed = parseCsv(src);
  ok(parsed.header.length === 4, '表头 4 列');
  ok(parsed.rows.length === 3, '数据 3 行');
  ok(parsed.rows[1][1] === 'comma, field', '引号内逗号保留');
  const back = toCsv(parsed.header, parsed.rows);
  const reparsed = parseCsv(back);
  ok(reparsed.rows.length === 3, '往返后行数一致');
  ok(reparsed.rows[1][1] === 'comma, field', '往返后引号字段一致');
  ok(reparsed.rows[1][3] === '30', '往返后末列一致');
}
{
  // BOM
  const bom = '\uFEFFP1,P2\n1,2\n';
  const p = parseCsv(bom);
  ok(p.header[0] === 'P1', 'BOM 被正确剥离');
}
{
  // 中文表头 + 括号单位
  const t = 'P1 (Pa),P2 (Pa),大气压,气温(℃)\n1,2,101325,20\n';
  const p = parseCsv(t);
  const m = detectColumns(p.header, 2);
  ok(m.pCols[0] === 0 && m.pCols[1] === 1, '中文/单位表头命中 P1/P2');
  ok(m.patmIdx === 2, '大气压 命中 Patm');
  ok(m.tatmIdx === 3, '气温 命中 TAtm');
}

// ---------- 2) runBatch 行为 ----------
console.log('[2] runBatch 行为');
{
  // mock 插值器：result.alpha = P1+P2, isValid 由 P1>0 决定
  const interp = {};
  const calculateRow = (it, input) => ({
    alpha: input.P1 + input.P2,
    machNumber: 0.5,
    isValid: input.P1 > 0,
    warning: input.P1 > 0 ? '' : 'P1<=0',
  });
  const resultColumns = ['alpha', 'machNumber'];
  const csv =
    'P1,P2,Patm,TAtm\n' +
    '10,20,101325,20\n' +     // 有效
    '0,5,101325,20\n' +       // 无效（P1=0）
    ',,101325,20\n' +         // 缺 P1/P2 -> ERROR
    '30,40,200000,25\n';      // 有效，使用行内 Patm/TAtm
  const out = runBatch(csv, {
    holeCount: 2,
    defaults: { PAtm: 101325, TAtm: 20 },
    interp, calculateRow, resultColumns,
  });
  ok(out.header.join(',') === 'P1,P2,Patm,TAtm,alpha,machNumber,isValid,warning', '表头顺序正确 (got: ' + out.header.join(',') + ')');
  ok(out.rows.length === 4, '4 行输出');
  ok(out.summary.total === 4 && out.summary.ok === 2 && out.summary.invalid === 1 && out.summary.errors === 1, '统计 ok=2 invalid=1 errors=1');
  ok(out.rows[0][4] === '30' && out.rows[0][6] === '1', '有效行 alpha=30 isValid=1');
  ok(out.rows[1][6] === '0' && out.rows[1][7] === 'P1<=0', '无效行 isValid=0 带警告');
  ok(out.rows[2][out.header.length - 2] === 'ERROR', '缺列行 isValid=ERROR');
  ok(out.rows[3][2] === '200000' && out.rows[3][3] === '25', '行内 Patm/TAtm 被采用');
  // 缺列检测
  const m2 = detectColumns(parseCsv('P1,Patm\n1,101325').header, 2);
  ok(m2.pCols[1] < 0, 'detectColumns 报告缺 P2');
}

// ---------- 3) resultColumns 与 reference go 键集一致性 ----------
console.log('[3] resultColumns 与 reference 输出键一致');
const PROBE_RC = {
  three: ['alpha', 'machNumber', 'P0', 'Ps', 'iterationCount'],
  five: ['alpha', 'beta', 'machNumber', 'v', 'vx', 'vy', 'vz', 'cas', 'sat', 'dynamicPressure', 'density', 'P0', 'Ps'],
  seven: ['alpha', 'beta', 'theta', 'phi', 'machNumber', 'velocity', 'totalPressure', 'staticPressure', 'dynamicPressure'],
};
function goKeysOf(ref, probe) {
  if (probe === 'seven') {
    const cases = ref.cases || [];
    return cases.length ? Object.keys(cases[0].go) : [];
  }
  return ref.length ? Object.keys(ref[0].go) : [];
}
{
  const three = JSON.parse(fs.readFileSync(path.join(__dirname, 'reference_three.json'), 'utf-8'));
  const five = JSON.parse(fs.readFileSync(path.join(__dirname, 'reference.json'), 'utf-8'));
  const seven = JSON.parse(fs.readFileSync(path.join(__dirname, 'reference_seven.json'), 'utf-8'));
  const datasets = { three, five, seven };
  for (const probe of ['three', 'five', 'seven']) {
    const keys = goKeysOf(datasets[probe], probe);
    const keySet = new Set(keys);
    const missing = PROBE_RC[probe].filter((c) => !keySet.has(c));
    ok(missing.length === 0, probe + ' resultColumns 均为 go 输出键的子集' + (missing.length ? '（缺失: ' + missing.join(',') + '）' : ''));
  }
}

console.log('\n结果: PASS=' + pass + ' FAIL=' + fail);
process.exit(fail === 0 ? 0 : 1);
