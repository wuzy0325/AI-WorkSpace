// PrbInterpolator —— 端口自 shared/algorithms/go/fivehole/interpolation/prb_interpolator.go
// 五孔探针 PRB 9 区域插值器（纯数学，无系统依赖）。
// 输入：5 个孔压 + 大气压力/温度；输出：攻角α、侧滑角β、马赫数、速度、总/静压等。
// 与 Go 版保持数值一致：网格 -30~30 步长5，区域9 凸四边形双线性反解，网格外不外推。

const { AtmosphericDataCalculator } = require('../atmospheric.js');

// ==================== 常量 ====================
const GRID_MIN_ANGLE = -30;
const GRID_MAX_ANGLE = 30;
const GRID_STEP = 5;
const GRID_AXIS_SIZE = 13;            // -30..30 步长5
const EXPECTED_GRID_COUNT = GRID_AXIS_SIZE * GRID_AXIS_SIZE; // 169
const NUMERIC_COL_COUNT = 6;
const DEFAULT_EPSILON = 1e-3;
const MIN_PRESSURE_DELTA = 1e-4;
const GAS_CONSTANT_AIR = 287.06;      // 注意：prb 模块用 287.06（与 atmospheric 的 287.05 不同，须复刻）
const GAMMA = 1.4;

// ==================== 工具 ====================
function isFiniteNum(f) {
  return typeof f === 'number' && isFinite(f);
}

function parseFloatStrict(s) {
  const f = Number(s);
  if (!isFiniteNum(f)) throw new Error('无效数字: ' + s);
  return f;
}

function clampPressureDelta(delta) {
  if (Math.abs(delta) >= MIN_PRESSURE_DELTA) return delta;
  if (delta === 0) return MIN_PRESSURE_DELTA;
  return Math.sign(delta) * MIN_PRESSURE_DELTA;
}

function interpolateFactor(value, start, end) {
  if (start === end) return 0;
  return (value - start) / (end - start);
}

function interpolateValue(start, end, factor) {
  return start + (end - start) * factor;
}

function ternary(cond, ifTrue, ifFalse) {
  return cond ? ifTrue : ifFalse;
}

function appendUnique(warnings, msg) {
  for (const w of warnings) {
    if (w === msg) return warnings;
  }
  warnings.push(msg);
  return warnings;
}

function isWithinRange(value, min, max) {
  return value >= min - DEFAULT_EPSILON && value <= max + DEFAULT_EPSILON;
}

// ==================== PRB 文件解析 ====================
function parseProbeTableRow(line, index) {
  const columns = line.trim().split(/\s+/);
  if (columns.length !== NUMERIC_COL_COUNT) {
    throw new Error('PRB第' + (index + 1) + '行必须包含' + NUMERIC_COL_COUNT + '列数据');
  }
  const values = columns.map(parseFloatStrict);
  return {
    Ka: values[0], Kb: values[1], CPT: values[2], CPS: values[3],
    Alpha: values[4], Beta: values[5],
  };
}

function parsePrbLines(lines) {
  const nonEmpty = lines.map((l) => l.trim()).filter((l) => l !== '');
  if (nonEmpty.length === 0) throw new Error('PRB文件为空');

  const headerParts = nonEmpty[0].split(/\s+/);
  if (headerParts.length !== 2) throw new Error('PRB文件表头必须包含两个网格维度');

  const width = parseInt(headerParts[0], 10);
  const height = parseInt(headerParts[1], 10);
  if (width !== GRID_AXIS_SIZE || height !== GRID_AXIS_SIZE) {
    throw new Error('PRB文件表头必须为 ' + GRID_AXIS_SIZE + ' ' + GRID_AXIS_SIZE);
  }

  const rowLines = nonEmpty.slice(1);
  if (rowLines.length !== EXPECTED_GRID_COUNT) {
    throw new Error('PRB文件必须包含 ' + EXPECTED_GRID_COUNT + ' 行数据');
  }

  return rowLines.map((line, i) => parseProbeTableRow(line, i));
}

// ==================== 校验与索引 ====================
function createGridAngles() {
  const angles = [];
  for (let a = GRID_MIN_ANGLE; a <= GRID_MAX_ANGLE; a += GRID_STEP) angles.push(a);
  return angles;
}

function gridPointKey(alpha, beta) {
  // JS 中 String(-0) === "0"，无需像 Go 那样规整 -0
  return alpha + ',' + beta;
}

function isFiniteRow(row) {
  return isFiniteNum(row.Ka) && isFiniteNum(row.Kb) && isFiniteNum(row.CPT) &&
    isFiniteNum(row.CPS) && isFiniteNum(row.Alpha) && isFiniteNum(row.Beta);
}

function validateAndIndexTable(table) {
  if (table.length !== EXPECTED_GRID_COUNT) {
    throw new Error('校准表必须包含 ' + EXPECTED_GRID_COUNT + ' 行');
  }

  const expectedKeys = {};
  const gridAngles = createGridAngles();
  for (const alpha of gridAngles) {
    for (const beta of gridAngles) {
      expectedKeys[gridPointKey(alpha, beta)] = true;
    }
  }

  const byGridPoint = {};
  const rows = [];

  for (let i = 0; i < table.length; i++) {
    const row = table[i];
    if (!isFiniteRow(row)) throw new Error('校准第' + (i + 1) + '行包含非有限值');

    const key = gridPointKey(row.Alpha, row.Beta);
    if (!expectedKeys[key]) {
      throw new Error('校准第' + (i + 1) + '行网格点(' + row.Alpha + ',' + row.Beta + ')不在预期范围内');
    }
    if (byGridPoint.hasOwnProperty(key)) {
      throw new Error('校准表存在重复网格点(' + row.Alpha + ',' + row.Beta + ')');
    }
    byGridPoint[key] = row;
    rows.push(row);
  }

  if (Object.keys(byGridPoint).length !== Object.keys(expectedKeys).length) {
    throw new Error('校准表缺少一个或多个网格点');
  }

  return {
    rows,
    getExactGridPoint(alpha, beta) {
      const k = gridPointKey(alpha, beta);
      return byGridPoint.hasOwnProperty(k) ? byGridPoint[k] : null;
    },
    getExactGridPointOrThrow(alpha, beta) {
      const k = gridPointKey(alpha, beta);
      if (!byGridPoint.hasOwnProperty(k)) {
        throw new Error('校准表不包含网格点(' + alpha + ',' + beta + ')');
      }
      return byGridPoint[k];
    },
  };
}

function createValidRange(rows) {
  let alphaMin = rows[0].Alpha, alphaMax = rows[0].Alpha;
  let betaMin = rows[0].Beta, betaMax = rows[0].Beta;
  for (let i = 1; i < rows.length; i++) {
    alphaMin = Math.min(alphaMin, rows[i].Alpha);
    alphaMax = Math.max(alphaMax, rows[i].Alpha);
    betaMin = Math.min(betaMin, rows[i].Beta);
    betaMax = Math.max(betaMax, rows[i].Beta);
  }
  return { AlphaMin: alphaMin, AlphaMax: alphaMax, BetaMin: betaMin, BetaMax: betaMax };
}

// ==================== 几何 ====================
function createLineThroughPoints(start, end) {
  const A = start.Y - end.Y;
  const B = end.X - start.X;
  const C = start.X * end.Y - end.X * start.Y;
  const normalLen = Math.hypot(A, B);
  if (normalLen === 0) return null;
  return { A, B, C, NormalLen: normalLen };
}

function signedDistanceToLine(point, line) {
  return (line.A * point.X + line.B * point.Y + line.C) / line.NormalLen;
}

function resolveReferenceDistance(line, vertices, edgeIndex) {
  const primaryRef = signedDistanceToLine(vertices[(edgeIndex + 2) % 4], line);
  if (Math.abs(primaryRef) > DEFAULT_EPSILON) return primaryRef;
  const secondaryRef = signedDistanceToLine(vertices[(edgeIndex + 3) % 4], line);
  if (Math.abs(secondaryRef) > DEFAULT_EPSILON) return secondaryRef;
  return null;
}

function isPointInsideConvexQuad(point, vertices) {
  for (let i = 0; i < vertices.length; i++) {
    const start = vertices[i];
    const end = vertices[(i + 1) % 4];
    const line = createLineThroughPoints(start, end);
    if (line === null) return false;
    const refDist = resolveReferenceDistance(line, vertices, i);
    if (refDist === null) return false;
    const pointDist = signedDistanceToLine(point, line);
    if (refDist > DEFAULT_EPSILON && pointDist < -DEFAULT_EPSILON) return false;
    if (refDist < -DEFAULT_EPSILON && pointDist > DEFAULT_EPSILON) return false;
  }
  return true;
}

function createRegion9Cells(getExact) {
  const cells = [];
  const angles = createGridAngles();
  for (let i = 0; i < angles.length - 1; i++) {
    for (let j = 0; j < angles.length - 1; j++) {
      let x1, x2, x3, x4;
      try {
        x1 = getExact(angles[i], angles[j]);
        x2 = getExact(angles[i + 1], angles[j]);
        x3 = getExact(angles[i + 1], angles[j + 1]);
        x4 = getExact(angles[i], angles[j + 1]);
      } catch (e) { continue; }
      if (!x1 || !x2 || !x3 || !x4) continue;
      cells.push({
        X1: x1, X2: x2, X3: x3, X4: x4,
        Vertices: [
          { X: x1.Ka, Y: x1.Kb },
          { X: x2.Ka, Y: x2.Kb },
          { X: x3.Ka, Y: x3.Kb },
          { X: x4.Ka, Y: x4.Kb },
        ],
      });
    }
  }
  return cells;
}

// PRB 双线性反解（对应 solvePrbBilinearInverse）。
// 顶点顺序：vertices[0]=X1 左下, [1]=X2 右下, [2]=X3 右上, [3]=X4 左上
function solvePrbBilinearInverse(point, vertices) {
  const x1 = vertices[0].X, y1 = vertices[0].Y;
  const x2 = vertices[1].X, y2 = vertices[1].Y;
  const x3 = vertices[3].X, y3 = vertices[3].Y; // 公式约定与存储顺序互换（同 Go 注释）
  const x4 = vertices[2].X, y4 = vertices[2].Y;

  const a1 = x2 - x1, b1 = x3 - x1, c1 = x1 - x2 - x3 + x4;
  const a2 = y2 - y1, b2 = y3 - y1, c2 = y1 - y2 - y3 + y4;
  const dx = point.X - x1, dy = point.Y - y1;

  const a = b2 * c1 - b1 * c2;
  const b = dx * c2 - dy * c1 + a1 * b2 - a2 * b1;
  const c = dx * a2 - dy * a1;

  const epsilon = 1e-12;
  let betaCandidates = [];
  if (Math.abs(a) < epsilon) {
    if (Math.abs(b) < epsilon) return { alpha: 0, beta: 0, ok: false };
    betaCandidates = [-c / b];
  } else {
    let disc = b * b - 4 * a * c;
    if (disc < -epsilon) return { alpha: 0, beta: 0, ok: false };
    disc = Math.max(0, disc);
    const root = Math.sqrt(disc);
    betaCandidates = [(-b + root) / (2 * a), (-b - root) / (2 * a)];
  }

  let bestAlpha = 0, bestBeta = 0, bestError = Infinity, found = false;
  for (const tBetaRaw of betaCandidates) {
    if (!isFiniteNum(tBetaRaw) || tBetaRaw < -DEFAULT_EPSILON || tBetaRaw > 1 + DEFAULT_EPSILON) continue;
    const denominator = a1 + c1 * tBetaRaw;
    let tAlpha;
    if (Math.abs(denominator) > epsilon) {
      tAlpha = (dx - b1 * tBetaRaw) / denominator;
    } else {
      const denom2 = a2 + c2 * tBetaRaw;
      if (Math.abs(denom2) < epsilon) continue;
      tAlpha = (dy - b2 * tBetaRaw) / denom2;
    }
    if (!isFiniteNum(tAlpha) || tAlpha < -DEFAULT_EPSILON || tAlpha > 1 + DEFAULT_EPSILON) continue;
    const tcAlpha = Math.max(0, Math.min(1, tAlpha));
    const tcBeta = Math.max(0, Math.min(1, tBetaRaw));
    const gx = x1 + a1 * tcAlpha + b1 * tcBeta + c1 * tcAlpha * tcBeta;
    const gy = y1 + a2 * tcAlpha + b2 * tcBeta + c2 * tcAlpha * tcBeta;
    const errSq = (gx - point.X) * (gx - point.X) + (gy - point.Y) * (gy - point.Y);
    if (errSq < bestError) {
      bestAlpha = tcAlpha; bestBeta = tcBeta; bestError = errSq; found = true;
    }
  }
  return { alpha: bestAlpha, beta: bestBeta, ok: found };
}

function resolveRegion9(point, cells) {
  const testPoint = { X: point.Ka, Y: point.Kb };
  for (const cell of cells) {
    if (!isPointInsideConvexQuad(testPoint, cell.Vertices)) continue;
    const res = solvePrbBilinearInverse(testPoint, cell.Vertices);
    if (!res.ok) continue;
    const alpha = interpolateValue(cell.X1.Alpha, cell.X2.Alpha, res.alpha);
    const beta = interpolateValue(cell.X1.Beta, cell.X4.Beta, res.beta);
    return { Ka: point.Ka, Kb: point.Kb, Alpha: alpha, Beta: beta };
  }
  return point;
}

function resolveOutputCellAxis(angle) {
  const truncatedIndex = Math.trunc(angle / GRID_STEP);
  if ((angle >= 0 && angle < GRID_MAX_ANGLE) || angle <= GRID_MIN_ANGLE) {
    return [GRID_STEP * truncatedIndex, GRID_STEP * (truncatedIndex + 1)];
  }
  return [GRID_STEP * (truncatedIndex - 1), GRID_STEP * truncatedIndex];
}

function interpolateOutputValue(point, table, selector) {
  if (point.Alpha == null || point.Beta == null) return 0;
  const [alpha1, alpha2] = resolveOutputCellAxis(point.Alpha);
  const [beta1, beta2] = resolveOutputCellAxis(point.Beta);
  const x1 = table.getExactGridPointOrThrow(alpha1, beta1);
  const x2 = table.getExactGridPointOrThrow(alpha2, beta1);
  const x3 = table.getExactGridPointOrThrow(alpha2, beta2);
  const x4 = table.getExactGridPointOrThrow(alpha1, beta2);
  const lowerEdge = interpolateValue(selector(x1), selector(x2), interpolateFactor(point.Alpha, x1.Alpha, x2.Alpha));
  const upperEdge = interpolateValue(selector(x4), selector(x3), interpolateFactor(point.Alpha, x4.Alpha, x3.Alpha));
  return interpolateValue(lowerEdge, upperEdge, interpolateFactor(point.Beta, x1.Beta, x4.Beta));
}

function calculateVelocityComponents(v, alphaDeg, betaDeg) {
  if (!isFiniteNum(v) || !isFiniteNum(alphaDeg) || !isFiniteNum(betaDeg)) return { vx: 0, vy: 0, vz: 0 };
  const alpha = alphaDeg * Math.PI / 180;
  const beta = betaDeg * Math.PI / 180;
  return {
    vx: v * Math.cos(beta) * Math.sin(alpha),
    vy: v * Math.sin(beta),
    vz: v * Math.cos(beta) * Math.cos(alpha),
  };
}

// ==================== PrbInterpolator ====================
class PrbInterpolator {
  constructor() {
    this.loaded = false;
    this.validRange = { AlphaMin: GRID_MIN_ANGLE, AlphaMax: GRID_MAX_ANGLE, BetaMin: GRID_MIN_ANGLE, BetaMax: GRID_MAX_ANGLE };
    this.context = null;
    this.atmCalc = new AtmosphericDataCalculator();
  }

  clearState() {
    this.context = null;
    this.validRange = { AlphaMin: GRID_MIN_ANGLE, AlphaMax: GRID_MAX_ANGLE, BetaMin: GRID_MIN_ANGLE, BetaMax: GRID_MAX_ANGLE };
    this.loaded = false;
  }

  // lines: PRB 文本行数组；source: 文件名（用于推测马赫数，本端口暂忽略）
  loadPrbLines(lines, source) {
    this.clearState();
    const rows = parsePrbLines(lines);
    const indexedTable = validateAndIndexTable(rows);
    const validRange = createValidRange(rows);
    const region9Cells = createRegion9Cells((a, b) => indexedTable.getExactGridPointOrThrow(a, b));

    const self = this;
    const getInterResult = (input) => {
      const point = calculatePressureCoefficients(input);
      const resolved = resolveRegion9(point, region9Cells);
      if (resolved.Alpha == null || resolved.Beta == null) {
        return { Alpha: 0, Beta: 0, Pt: 0, Ps: 0, V: 0, Ma: 0, Valid: false };
      }
      const alpha = resolved.Alpha;
      const beta = resolved.Beta;
      const cpt = interpolateOutputValue(resolved, indexedTable, (r) => r.CPT);
      const cps = interpolateOutputValue(resolved, indexedTable, (r) => r.CPS);
      const delta = clampPressureDelta(input.FiveHoleData[1] - (input.FiveHoleData[0] + input.FiveHoleData[2] + input.FiveHoleData[3] + input.FiveHoleData[4]) * 0.25);
      const pt = input.FiveHoleData[1] - cpt * delta;
      const avg = (input.FiveHoleData[0] + input.FiveHoleData[2] + input.FiveHoleData[3] + input.FiveHoleData[4]) * 0.25;
      const ps = avg - cps * delta;
      const v = calculateVelocity(self.atmCalc, input, pt, ps);
      const ma = calculateMachFromPressures(self.atmCalc, input, pt, ps);
      return { Alpha: alpha, Beta: beta, Pt: pt, Ps: ps, V: v, Ma: ma, Valid: true };
    };

    this.context = { ValidRange: validRange, GetInterResult: getInterResult };
    this.validRange = validRange;
    this.loaded = true;
    return true;
  }

  isLoaded() { return this.loaded; }
  getValidRange() { return this.validRange; }

  calculate(input) {
    if (!this.loaded || !this.context) {
      return { result: emptyResult(), error: 'PRB文件未加载' };
    }
    const rtInput = toRuntimeInput(input);
    const warnings = collectInputWarnings(rtInput);
    const interResult = this.context.GetInterResult(rtInput);
    const result = toInterpolationResult(interResult, rtInput, this.context.ValidRange, warnings, this.atmCalc);
    return { result, error: null };
  }
}

// ==================== 计算辅助 ====================
function calculatePressureCoefficients(input) {
  const p1 = input.FiveHoleData[0], p2 = input.FiveHoleData[1], p3 = input.FiveHoleData[2];
  const p4 = input.FiveHoleData[3], p5 = input.FiveHoleData[4];
  const avg = (p1 + p3 + p4 + p5) * 0.25;
  const delta = clampPressureDelta(p2 - avg);
  return { Ka: (p4 - p5) / delta, Kb: (p3 - p1) / delta, Alpha: null, Beta: null };
}

function calculateVelocity(calc, input, pt, ps) {
  const absPt = pt + input.AtmP;
  const absPs = ps + input.AtmP;
  const tempK = input.AtmT + 273.15;
  if (absPs <= 0 || tempK <= 0 || absPt <= absPs) return 0;
  let ma;
  try { ma = calc.calculateMach(absPt, absPs); } catch (e) { return 0; }
  const sat = calc.calculateSAT(tempK, ma);
  const qc = calc.calculateQc(absPt, absPs);
  return calc.calculateTASByDensity(absPs, qc, sat);
}

function calculateMachFromPressures(calc, input, pt, ps) {
  const absPt = pt + input.AtmP;
  const absPs = ps + input.AtmP;
  if (absPs <= 0 || absPt <= absPs) return 0;
  try { return calc.calculateMach(absPt, absPs); } catch (e) { return 0; }
}

function toRuntimeInput(input) {
  return {
    AtmP: input.PAtm,
    AtmT: input.TAtm,
    FiveHoleData: [input.P1, input.P2, input.P3, input.P4, input.P5],
  };
}

function collectInputWarnings(input) {
  const p1 = input.FiveHoleData[0], p2 = input.FiveHoleData[1], p3 = input.FiveHoleData[2];
  const p4 = input.FiveHoleData[3], p5 = input.FiveHoleData[4];
  const avg = (p1 + p3 + p4 + p5) * 0.25;
  const delta = p2 - avg;
  const warnings = [];
  if (Math.abs(delta) < MIN_PRESSURE_DELTA) {
    warnings.push('参考压力差接近零，插值使用了最小压力差钳位');
  }
  return warnings;
}

function emptyResult() {
  return {
    alpha: 0, beta: 0, machNumber: 0, v: 0, vx: 0, vy: 0, vz: 0, velocity: 0,
    cas: 0, sat: 0, dynamicPressure: 0, density: 0, P0: 0, Ps: 0,
    isValid: false, warning: '',
  };
}

function toInterpolationResult(result, input, validRange, warnings, atmCalc) {
  if (!result.Valid) {
    warnings = appendUnique(warnings, '压力系数超出PRB校准网格，旧算法不支持外推');
    return Object.assign(emptyResult(), { isValid: false, warning: warnings.join('; ') });
  }

  const dynamicPressure = result.Pt - result.Ps;
  const tempK = input.AtmT + 273.15;
  const absPt = input.AtmP + result.Pt;
  const absPs = input.AtmP + result.Ps;
  const comps = calculateVelocityComponents(result.V, result.Alpha, result.Beta);

  let density = 0;
  if (isFiniteNum(absPs) && absPs > 0 && tempK > 0) {
    density = absPs / (GAS_CONSTANT_AIR * tempK);
  }

  let cas = 0, sat = 0;
  if (absPs > 0 && absPt > absPs && tempK > 0) {
    try {
      const ma = atmCalc.calculateMach(absPt, absPs);
      sat = atmCalc.calculateSAT(tempK, ma);
      const qc = atmCalc.calculateQc(absPt, absPs);
      cas = atmCalc.calculateCAS(qc);
    } catch (e) { /* 保持 0 */ }
  }

  let isValid = true;
  if (!isFiniteNum(result.Alpha) || !isFiniteNum(result.Beta) || !isFiniteNum(result.V) || !isFiniteNum(result.Ma)) {
    warnings = appendUnique(warnings, '插值返回非有限输出');
    isValid = false;
  }
  if (!isFiniteNum(dynamicPressure)) {
    warnings = appendUnique(warnings, '解析动压不是有限值');
    isValid = false;
  }
  if (isFiniteNum(dynamicPressure) && dynamicPressure <= 0) {
    warnings = appendUnique(warnings, '总压低于静压 (pt < ps)');
    isValid = false;
  }
  if (!isWithinRange(result.Alpha, validRange.AlphaMin, validRange.AlphaMax)) {
    warnings = appendUnique(warnings, '解析攻角超出PRB表范围');
    isValid = false;
  }
  if (!isWithinRange(result.Beta, validRange.BetaMin, validRange.BetaMax)) {
    warnings = appendUnique(warnings, '解析侧滑角超出PRB表范围');
    isValid = false;
  }

  const warningStr = warnings.length > 0 ? warnings.join('; ') : '';

  return {
    alpha: result.Alpha,
    beta: result.Beta,
    machNumber: result.Ma,
    v: result.V,
    vx: comps.vx,
    vy: comps.vy,
    vz: comps.vz,
    velocity: result.V,
    cas: ternary(isFiniteNum(cas), cas, 0),
    sat: ternary(isFiniteNum(sat), sat, 0),
    dynamicPressure: ternary(isFiniteNum(dynamicPressure), dynamicPressure, 0),
    density: ternary(isFiniteNum(density) && density > 0, density, 0),
    P0: ternary(isFiniteNum(result.Pt), result.Pt, 0),
    Ps: ternary(isFiniteNum(result.Ps), result.Ps, 0),
    isValid: isValid,
    warning: warningStr,
  };
}

module.exports = { PrbInterpolator, parsePrbLines, createGridAngles, GRID_AXIS_SIZE, EXPECTED_GRID_COUNT };
