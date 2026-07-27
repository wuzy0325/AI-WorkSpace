// 单位换算与 CSV 单位回调的 Node 校验（不依赖 wx）。
// 运行：node verify/verify_units.js
const { formatValue, unitLabelShort, convert, OPTIONS } = require('../utils/units.js');
const { runBatch } = require('../utils/csv-batch.js');

let pass = 0, fail = 0;
function ok(cond, name, extra) {
  if (cond) { pass++; }
  else { fail++; console.log('  ✗ ' + name + (extra !== undefined ? '  -> ' + JSON.stringify(extra) : '')); }
}
function approx(a, b, tol) { return Math.abs(a - b) <= tol; }

console.log('== units.js 换算 ==');

// 压力 Pa/kPa/MPa（基准 Pa）
ok(approx(convert('pressure', 'kPa', 100000), 100, 1e-9), '100000 Pa -> 100 kPa', convert('pressure', 'kPa', 100000));
ok(approx(convert('pressure', 'MPa', 100000), 0.1, 1e-12), '100000 Pa -> 0.1 MPa', convert('pressure', 'MPa', 100000));
// 速度 m/s <-> km/h（基准 m/s）
ok(approx(convert('velocity', 'km/h', 100), 360, 1e-9), '100 m/s -> 360 km/h', convert('velocity', 'km/h', 100));
// 角度 deg <-> rad（基准 deg）
ok(approx(convert('angle', 'rad', 180), Math.PI, 1e-12), '180 deg -> π rad', convert('angle', 'rad', 180));
// 温度（基准 K）
ok(approx(convert('temp', '°C', 273.15), 0, 1e-9), '273.15 K -> 0 °C', convert('temp', '°C', 273.15));
ok(approx(convert('temp', '°F', 273.15), 32, 1e-9), '273.15 K -> 32 °F', convert('temp', '°F', 273.15));
ok(approx(convert('temp', '°C', 373.15), 100, 1e-9), '373.15 K -> 100 °C', convert('temp', '°C', 373.15));

console.log('== units.js 格式化 ==');
ok(formatValue('pressure', 'kPa', 120000) === '120 kPa', '120000 Pa -> "120 kPa"', formatValue('pressure', 'kPa', 120000));
ok(formatValue('pressure', 'MPa', 120000) === '0.12 MPa', '120000 Pa -> "0.12 MPa"', formatValue('pressure', 'MPa', 120000));
ok(formatValue('pressure', 'Pa', 120000) === '120000 Pa', '120000 Pa -> "120000 Pa" (去尾零)', formatValue('pressure', 'Pa', 120000));
ok(formatValue('velocity', 'km/h', 100) === '360 km/h', '100 m/s -> "360 km/h"', formatValue('velocity', 'km/h', 100));
ok(formatValue('angle', 'rad', 180) === '3.141593 rad', '180 deg -> "3.141593 rad"', formatValue('angle', 'rad', 180));
ok(formatValue('temp', '°C', 293.15) === '20 °C', '293.15 K -> "20 °C"', formatValue('temp', '°C', 293.15));
ok(formatValue('temp', '°F', 293.15) === '68 °F', '293.15 K -> "68 °F"', formatValue('temp', '°F', 293.15));
ok(formatValue('none', undefined, 0.52345) === '0.523', 'machNumber none -> "0.523"', formatValue('none', undefined, 0.52345));
ok(formatValue('none', undefined, 7) === '7', 'iterationCount none -> "7" (整数去尾零)', formatValue('none', undefined, 7));

console.log('== units.js 单位后缀标签 ==');
ok(unitLabelShort('pressure', 'kPa') === 'kPa', 'pressure kPa short', unitLabelShort('pressure', 'kPa'));
ok(unitLabelShort('temp', '°C') === 'degC', 'temp °C short', unitLabelShort('temp', '°C'));
ok(unitLabelShort('angle', 'rad') === 'rad', 'angle rad short', unitLabelShort('angle', 'rad'));
ok(unitLabelShort('velocity', 'km/h') === 'kmh', 'velocity km/h short', unitLabelShort('velocity', 'km/h'));

console.log('== csv-batch runBatch 单位回调 ==');
const COLUMN_UNIT = { alpha: 'angle', machNumber: 'none', P0: 'pressure', Ps: 'pressure', iterationCount: 'none', sat: 'temp', v: 'velocity' };
const UNITS_KPA = { pressure: 'kPa', velocity: 'm/s', angle: 'deg', temp: '°C' };
const UNITS_MPA = { pressure: 'MPa', velocity: 'm/s', angle: 'deg', temp: '°C' };
const mockResult = { alpha: 5.123456, machNumber: 0.52345, P0: 120000, Ps: 100000, iterationCount: 7, isValid: true };

function runWith(units) {
  const text = 'P1,P2,P3\n1,2,3\n';
  return runBatch(text, {
    holeCount: 3,
    defaults: { PAtm: 101325, TAtm: 20 },
    interp: {},
    calculateRow: () => mockResult,
    resultColumns: ['alpha', 'machNumber', 'P0', 'Ps', 'iterationCount'],
    formatResultValue: (col, raw) => formatValue(COLUMN_UNIT[col] || 'none', units[COLUMN_UNIT[col] || 'none'], raw),
    resultColumnHeader: (col) => {
      const t = COLUMN_UNIT[col] || 'none';
      return t === 'none' ? col : col + '_' + unitLabelShort(t, units[t]);
    },
  });
}
const outK = runWith(UNITS_KPA);
ok(outK.header.indexOf('P0_kPa') >= 0, '表头含 P0_kPa', outK.header);
ok(outK.header.indexOf('Ps_kPa') >= 0, '表头含 Ps_kPa', outK.header);
ok(outK.header.indexOf('machNumber') >= 0, 'none 列保持 machNumber', outK.header);
const p0Idx = outK.header.indexOf('P0_kPa');
ok(outK.rows[0][p0Idx] === '120 kPa', 'P0_kPa 值 = "120 kPa"', outK.rows[0][p0Idx]);

const outM = runWith(UNITS_MPA);
const p0IdxM = outM.header.indexOf('P0_MPa');
ok(outM.rows[0][p0IdxM] === '0.12 MPa', '切换 MPa 后 P0_MPa 值 = "0.12 MPa"', outM.rows[0][p0IdxM]);

console.log('== 各探针结果列定义完整性 ==');
// 与页面内 RESULT_COLUMNS + COLUMN_UNIT 保持一致（此处内联校验覆盖度）
const PROBES = {
  three: {
    cols: ['alpha', 'machNumber', 'P0', 'Ps', 'iterationCount'],
    unit: { alpha: 'angle', machNumber: 'none', P0: 'pressure', Ps: 'pressure', iterationCount: 'none' },
  },
  five: {
    cols: ['alpha', 'beta', 'machNumber', 'v', 'vx', 'vy', 'vz', 'cas', 'sat', 'dynamicPressure', 'density', 'P0', 'Ps'],
    unit: { alpha: 'angle', beta: 'angle', machNumber: 'none', v: 'velocity', vx: 'velocity', vy: 'velocity', vz: 'velocity', cas: 'velocity', sat: 'temp', dynamicPressure: 'pressure', density: 'none', P0: 'pressure', Ps: 'pressure' },
  },
  seven: {
    cols: ['alpha', 'beta', 'theta', 'phi', 'machNumber', 'velocity', 'totalPressure', 'staticPressure', 'dynamicPressure'],
    unit: { alpha: 'angle', beta: 'angle', theta: 'angle', phi: 'angle', machNumber: 'none', velocity: 'velocity', totalPressure: 'pressure', staticPressure: 'pressure', dynamicPressure: 'pressure' },
  },
};
const KNOWN_TYPES = ['pressure', 'velocity', 'angle', 'temp', 'none'];
for (const [name, def] of Object.entries(PROBES)) {
  for (const c of def.cols) {
    const t = def.unit[c];
    ok(KNOWN_TYPES.indexOf(t) >= 0, name + ' 列 ' + c + ' 有已知单位类型', t);
  }
}

console.log('\n== 结果 ==');
console.log('PASS: ' + pass + '  FAIL: ' + fail);
process.exit(fail === 0 ? 0 : 1);
