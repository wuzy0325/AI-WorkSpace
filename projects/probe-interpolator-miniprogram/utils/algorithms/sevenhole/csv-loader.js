'use strict';

// 七孔校准 CSV → SevenHolePrbInterpolator（JavaScript 移植）。
// 端口自 shared/algorithms/go/sevenhole/interpolation/csv_loader.go。
//
// 与 Go 版逐位对齐：列位置契约（按位置读，不依赖表头名称）、退化边确定性抖动
// （1e-9 量级，最多 100 轮）、外区 theta 网格派生、PRB 行重建后喂入既有的
// loadInnerPrbLines / loadOuterPrbLines。
//
// 纯 JS（wx 无关），可在 Node 端校验；小程序侧仅负责把文件读成 UTF-8 文本后传入。
//
// 列位置契约（spec seven-hole-calibration §7.2/§7.3，0-indexed）：
//   col 0:  角度1（inner: α / outer: φ）
//   col 1:  角度2（inner: β / outer: θ）
//   col 12: ka 系数（inner: Kα / outer: Kθ[n]）
//   col 13: kb 系数（inner: Kβ / outer: Kφ[n]）
//   col 14: cpt     （inner: K0 / outer: K0[n]）
//   col 15: cps     （inner: Ks / outer: Ks[n]）
// 必需列数 = max 列索引 + 1 = 16（实际数据集为 18 列基础格式）。
// 注意：outer CSV 表头存在历史遗留命名错误（写"侧滑角α/迎角β"，实为 φ/θ），
// 故必须按列位置读取，不能按表头名称匹配。
//
// 编码说明：本模块要求传入已解码的 UTF-8 文本。校准软件导出的 CSV 常见 GB18030/GBK
// 编码，但其数据列（数字 + ASCII 逗号/换行）均落在 ASCII 区间，GB18030 不会把逗号或
// 换行编码进多字节序列，因此以 UTF-8 读取时数字列仍能正确切分与解析；仅中文表头会乱码
// （触发一条非致命 warning，不影响按位置解析）。若你的 CSV 数值列本身因编码异常无法解析，
// 请用 Excel/记事本另存为 UTF-8 后再导入。

const { parseCsv } = require('../../csv-batch.js');
const { SevenHolePrbInterpolator } = require('./seven-hole.js');

// ── 位置契约常量 ─────────────────────────────────────────────────
const CSV_ANGLE1_COL = 0; // inner: α；outer: φ
const CSV_ANGLE2_COL = 1; // inner: β；outer: θ
const CSV_KA_COL = 12;    // Kα / Kθ[n]
const CSV_KB_COL = 13;    // Kβ / Kφ[n]
const CSV_CPT_COL = 14;   // K0 / K0[n]
const CSV_CPS_COL = 15;   // Ks / Ks[n]
const CSV_MIN_COLS = 16;  // 必需列最大索引 + 1

// 6 个外区扇区的 φ 网格线（每扇区 13 点，φ 递减）。
const SECTOR_PHI_LINES = [
  [30, 25, 20, 15, 10, 5, 0, 355, 350, 345, 340, 335, 330],
  [90, 85, 80, 75, 70, 65, 60, 55, 50, 45, 40, 35, 30],
  [150, 145, 140, 135, 130, 125, 120, 115, 110, 105, 100, 95, 90],
  [210, 205, 200, 195, 190, 185, 180, 175, 170, 165, 160, 155, 150],
  [270, 265, 260, 255, 250, 245, 240, 235, 230, 225, 220, 215, 210],
  [330, 325, 320, 315, 310, 305, 300, 295, 290, 285, 280, 275, 270],
];

const GRID_STEP = 5.0;
const OUTER_THETA_MIN = 30.0;
const GRID_EPS = 1e-9;
const DITHER_PASSES = 100;

// 历史别名（仅诊断 warning 用，解析按位置不依赖表头名称）。
const HISTORICAL_ALIASES = ['侧滑角α', '迎角β', 'α角度系数Kα', 'β角度系数Kβ', '总压系数K0', '静压系数Ks'];

function key(a, b) { return a + ',' + b; }

// ── 列解析（按位置，不依赖表头）──────────────────────────────────
function resolveColumns(header, inner) {
  const cols = { ka: CSV_KA_COL, kb: CSV_KB_COL, cpt: CSV_CPT_COL, cps: CSV_CPS_COL };
  if (inner) {
    cols.a = CSV_ANGLE1_COL; // α
    cols.b = CSV_ANGLE2_COL; // β
  } else {
    cols.a = CSV_ANGLE2_COL; // θ
    cols.b = CSV_ANGLE1_COL; // φ
  }
  const warnings = [];
  const missing = HISTORICAL_ALIASES.filter((name) => !header.some((h) => (h == null ? '' : String(h).trim()) === name));
  if (missing.length) {
    warnings.push('csv 表头缺少历史别名 [' + missing.join(', ') + ']（实际表头: [' + header.join(', ') + ']）；按位置契约继续解析，若数据为 26 列证书导出格式请勿使用本入口');
  }
  return { cols, warnings };
}

// 解析 1 份校准 CSV（inner 或某扇区 outer）为按 (a,b) 索引的网格点 Map。
// 返回 { points: Map<"a,b", {ka,kb,cpt,cps}>, warnings: [] }。
function parseRecords(label, text, inner) {
  const parsed = parseCsv(text);
  const header = parsed.header || [];
  const allRows = parsed.rows || [];
  if (allRows.length < 1) throw new Error(label + ': 至少需要一行数据（表头除外）');
  if (header.length < CSV_MIN_COLS) {
    throw new Error(label + ': 表头列数 ' + header.length + ' < ' + CSV_MIN_COLS + '（最小必需列数），实际表头: [' + header.join(', ') + ']');
  }
  const { cols, warnings } = resolveColumns(header, inner);
  const points = new Map();
  const required = Math.max(cols.ka, cols.kb, cols.cpt, cols.cps, cols.a, cols.b) + 1;
  for (let i = 0; i < allRows.length; i++) {
    const rec = allRows[i];
    const lineNo = i + 2; // +1 表头 +1 起始行
    // 跳过完全空白行（真实 CSV 常见尾部空行）。
    if (rec.length === 0 || rec.every((f) => (f == null ? '' : String(f).trim()) === '')) continue;
    if (rec.length < required) {
      throw new Error(label + ' 第' + lineNo + '行: 至少 ' + required + ' 列（必需列最大索引+1），实际 ' + rec.length + ' 列');
    }
    const vals = [cols.ka, cols.kb, cols.cpt, cols.cps, cols.a, cols.b].map((c) => {
      const raw = rec[c] == null ? '' : String(rec[c]).trim();
      const x = Number(raw);
      if (!isFinite(x)) throw new Error(label + ' 第' + lineNo + '行第' + (c + 1) + '列: 不是有效数字 ' + JSON.stringify(rec[c]));
      return x;
    });
    const a = vals[4], b = vals[5];
    const k = key(a, b);
    if (points.has(k)) throw new Error(label + ' 第' + lineNo + '行: 重复网格点 (a=' + a + ', b=' + b + ')');
    points.set(k, { ka: vals[0], kb: vals[1], cpt: vals[2], cps: vals[3] });
  }
  if (points.size === 0) throw new Error(label + ': 未解析到任何网格点');
  return { points, warnings };
}

// 外区 theta 网格：收集唯一 theta 值，校验起点=30、步长=5。
function deriveOuterThetaGrid(points, label) {
  const thetaSet = new Set();
  for (const k of points.keys()) {
    const a = Number(k.split(',')[0]);
    thetaSet.add(a);
  }
  if (thetaSet.size < 2) {
    throw new Error(label + ': 外区 theta 网格点数 ' + thetaSet.size + ' < 2（至少需要 2 点形成插值单元格）');
  }
  const theta = Array.from(thetaSet).sort((x, y) => x - y);
  if (Math.abs(theta[0] - OUTER_THETA_MIN) > GRID_EPS) {
    throw new Error(label + ': 外区 theta 起点 ' + theta[0] + ' 必须 = ' + OUTER_THETA_MIN);
  }
  for (let i = 1; i < theta.length; i++) {
    const step = theta[i] - theta[i - 1];
    if (Math.abs(step - GRID_STEP) > GRID_EPS) {
      throw new Error(label + ': 外区 theta 步长 ' + step + ' 必须 = ' + GRID_STEP + '（theta=' + theta[i - 1] + '→' + theta[i] + '）');
    }
  }
  return theta;
}

// 查找退化边（相邻网格点 ka/kb 相等 → bilinear 单元格不可逆）。
function findDegenerateEdges(points, aValues, bValues) {
  const bad = [];
  for (let bi = 0; bi < bValues.length; bi++) {
    const b = bValues[bi];
    for (let ai = 0; ai < aValues.length; ai++) {
      const a = aValues[ai];
      const point = points.get(key(a, b));
      if (!point) continue;
      // 水平邻居（a 增大方向）：右邻 ka 相同 → 抖动右邻的 ka。
      if (ai + 1 < aValues.length) {
        const next = points.get(key(aValues[ai + 1], b));
        if (next && next.ka === point.ka) bad.push({ field: 0, k: key(aValues[ai + 1], b) });
      }
      // 垂直邻居（b 增大方向）：下邻 ka 或 kb 相同 → 抖动下邻。
      if (bi + 1 < bValues.length) {
        const nextB = bValues[bi + 1];
        const next = points.get(key(a, nextB));
        if (next) {
          if (next.ka === point.ka) bad.push({ field: 0, k: key(a, nextB) });
          if (next.kb === point.kb) bad.push({ field: 1, k: key(a, nextB) });
        }
      }
    }
  }
  return bad;
}

// 对退化边施加确定性抖动（1e-9 * 累计 nudge 数）。100 轮后仍存在则报错（避免后续 NaN/Inf）。
function ditherGrid(points, aValues, bValues) {
  let nudges = 0;
  for (let pass = 0; pass < DITHER_PASSES; pass++) {
    const bad = findDegenerateEdges(points, aValues, bValues);
    if (bad.length === 0) return { nudges, err: null };
    for (const t of bad) {
      nudges++;
      const p = points.get(t.k);
      if (!p) continue;
      if (t.field === 0) p.ka += 1e-9 * nudges;
      else p.kb += 1e-9 * nudges;
    }
  }
  const finalBad = findDegenerateEdges(points, aValues, bValues);
  return { nudges, err: new Error('退化边抖动 ' + DITHER_PASSES + ' 轮后仍存在 ' + finalBad.length + ' 处退化边，校准数据可能存在系统性异常') };
}

// 由网格点 Map 重建 PRB 行（"ka kb cpt cps a b" + 数据行）。
function buildPrbLines(points, aValues, bValues) {
  const lines = ['ka kb cpt cps a b'];
  for (const b of bValues) {
    for (const a of aValues) {
      const p = points.get(key(a, b));
      if (!p) continue;
      lines.push([p.ka, p.kb, p.cpt, p.cps, a, b].map((v) => String(v)).join(' '));
    }
  }
  return lines;
}

// ── 主入口 ─────────────────────────────────────────────────────
// loadCalibrationCSV(innerText, outerTexts)
//   innerText:   内区 CSV 文本（UTF-8）
//   outerTexts:  长度 6 的数组，依次为外区扇区 1..6 的 CSV 文本
// 返回 { interpolator: SevenHolePrbInterpolator, warnings: [] }
// 出错时抛出 Error（含具体原因）。
function loadCalibrationCSV(innerText, outerTexts) {
  if (!Array.isArray(outerTexts) || outerTexts.length !== 6) {
    throw new Error('seven-hole outer calibration csv count must be 6, got ' + (outerTexts ? outerTexts.length : 0));
  }
  const gridLines = [];
  for (let i = 0; i < 13; i++) gridLines.push(-30 + 5 * i);

  const interpolator = new SevenHolePrbInterpolator();
  const warnings = [];

  // 内区
  const innerParsed = parseRecords('内区', innerText, true);
  warnings.push.apply(warnings, innerParsed.warnings);
  const inDither = ditherGrid(innerParsed.points, gridLines, gridLines);
  if (inDither.err) throw inDither.err;
  if (inDither.nudges > 0) {
    warnings.push('内区 CSV 已对 ' + inDither.nudges + ' 处退化边施加确定性抖动（1e-9 量级）');
  }
  const innerLines = buildPrbLines(innerParsed.points, gridLines, gridLines);
  interpolator.loadInnerPrbLines(innerLines, 'inner.csv');

  // 外区 6 扇区
  for (let s = 0; s < 6; s++) {
    const outerParsed = parseRecords('扇区' + (s + 1), outerTexts[s], false);
    warnings.push.apply(warnings, outerParsed.warnings);
    const theta = deriveOuterThetaGrid(outerParsed.points, '扇区' + (s + 1));
    const outDither = ditherGrid(outerParsed.points, theta, SECTOR_PHI_LINES[s]);
    if (outDither.err) throw outDither.err;
    if (outDither.nudges > 0) {
      warnings.push('扇区 ' + (s + 1) + ' CSV 已对 ' + outDither.nudges + ' 处退化边施加确定性抖动（1e-9 量级）');
    }
    const outerLines = buildPrbLines(outerParsed.points, theta, SECTOR_PHI_LINES[s]);
    interpolator.loadOuterPrbLines(s + 1, outerLines, 'outer' + (s + 1) + '.csv');
  }

  return { interpolator, warnings };
}

module.exports = { loadCalibrationCSV, SECTOR_PHI_LINES };
