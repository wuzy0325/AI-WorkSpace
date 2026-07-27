// 物理量单位换算（纯 JS，不依赖 wx，可在 Node 校验端复用）。
//
// 设计原则：
//  - 算法内部一律使用国际单位基准：压力 Pa、速度 m/s、角度 deg、温度 K（静温）。
//  - 本模块只做「基准值 → 显示单位」的换算与格式化，不改变任何计算。
//  - 线性量（压力/速度/角度）用 factor；温度非线性，用 fromK/toK 双向转换。
//
// 可用单位：
//  pressure : Pa | kPa | MPa
//  velocity : m/s | km/h
//  angle    : deg | rad
//  temp     : °C  | K   | °F   （基准为 K）

const PRESSURE = {
  Pa:  { factor: 1,        label: 'Pa',  short: 'Pa' },
  kPa: { factor: 1e-3,     label: 'kPa', short: 'kPa' },
  MPa: { factor: 1e-6,     label: 'MPa', short: 'MPa' },
};

const VELOCITY = {
  'm/s': { factor: 1,      label: 'm/s', short: 'mps' },
  'km/h': { factor: 3.6,   label: 'km/h', short: 'kmh' },
};

const ANGLE = {
  deg: { factor: 1,             label: '°',   short: 'deg' },
  rad: { factor: Math.PI / 180, label: 'rad', short: 'rad' },
};

const TEMP = {
  '°C': { fromK: (k) => k - 273.15,         toK: (c) => c + 273.15,         label: '°C', short: 'degC' },
  'K':  { fromK: (k) => k,                  toK: (k) => k,                  label: 'K',  short: 'K' },
  '°F': { fromK: (k) => (k - 273.15) * 9 / 5 + 32, toK: (f) => (f - 32) * 5 / 9 + 273.15, label: '°F', short: 'degF' },
};

const TABLES = {
  pressure: PRESSURE,
  velocity: VELOCITY,
  angle: ANGLE,
  temp: TEMP,
};

// 各类型可用单位列表（用于 picker 选项）
const OPTIONS = {
  pressure: ['Pa', 'kPa', 'MPa'],
  velocity: ['m/s', 'km/h'],
  angle: ['deg', 'rad'],
  temp: ['°C', 'K', '°F'],
};

// 各类型默认小数位（formatValue 用）
const DEFAULT_DECIMALS = {
  pressure: { Pa: 2, kPa: 3, MPa: 4 },
  velocity: { 'm/s': 2, 'km/h': 1 },
  angle: { deg: 3, rad: 6 },
  temp: { '°C': 2, K: 2, '°F': 2 },
};

function tableOf(type) {
  return TABLES[type] || null;
}

// 把数值去掉末尾多余 0（toFixed 会带尾零），并容错 NaN/Infinity。
function trimNumber(value, decimals) {
  if (value === null || value === undefined) return '';
  if (typeof value !== 'number' || !isFinite(value)) return String(value);
  const s = value.toFixed(decimals);
  // 去尾零与小数点
  return s.indexOf('.') >= 0 ? s.replace(/\.?0+$/, '') : s;
}

// 通用线性换算：基准值 × factor
function convertLinear(type, unit, raw) {
  const t = tableOf(type);
  if (!t || !t[unit]) return raw; // 未知类型/单位：原样返回
  return raw * t[unit].factor;
}

// 温度换算：基准为 K，经 fromK 转目标单位
function convertTemp(unit, rawK) {
  const t = TEMP[unit];
  if (!t) return rawK;
  return t.fromK(rawK);
}

// 主入口：把基准值 raw 转为显示单位的数值
function convert(type, unit, raw) {
  if (raw === null || raw === undefined || raw === '') return raw;
  if (type === 'temp') return convertTemp(unit, Number(raw));
  if (type === 'none' || !type) return raw;
  return convertLinear(type, unit, Number(raw));
}

// 格式化：换算 + 去尾零 + 拼接单位标签（如 "101.3 kPa"）。
// decimals 可选；缺省用该类型该单位的默认小数位。
function formatValue(type, unit, raw, decimals) {
  if (raw === null || raw === undefined || raw === '') return '';
  let val;
  try {
    val = convert(type, unit, raw);
  } catch (e) {
    return String(raw);
  }
  if (typeof val !== 'number' || !isFinite(val)) return String(raw);
  let d = decimals;
  if (d === undefined || d === null) {
    const tbl = DEFAULT_DECIMALS[type];
    d = (tbl && tbl[unit] !== undefined) ? tbl[unit] : 3;
  }
  const t = tableOf(type);
  const lab = (t && t[unit]) ? t[unit].label : '';
  const num = trimNumber(val, d);
  return lab ? num + ' ' + lab : num;
}

// 仅返回换算后的数值（不含单位后缀），供需要再处理的场景。
function convertNumber(type, unit, raw) {
  if (raw === null || raw === undefined || raw === '') return raw;
  const v = convert(type, unit, raw);
  return (typeof v === 'number' && isFinite(v)) ? v : raw;
}

// 单位短标签（用于 CSV 表头后缀，如 totalPressure_kPa）
function unitLabelShort(type, unit) {
  const t = tableOf(type);
  return (t && t[unit]) ? t[unit].short : (unit || '');
}

// 单位长标签（用于 UI 显示，如 "kPa"）
function unitLabel(type, unit) {
  const t = tableOf(type);
  return (t && t[unit]) ? t[unit].label : (unit || '');
}

module.exports = {
  PRESSURE, VELOCITY, ANGLE, TEMP,
  OPTIONS, DEFAULT_DECIMALS,
  convert, convertNumber,
  convertLinear, convertTemp,
  formatValue, trimNumber,
  unitLabelShort, unitLabel,
};
