'use strict';

// 七孔探针遍历插值算法（JavaScript 移植）。
// 端口自 shared/algorithms/go/sevenhole/interpolation 全套源文件。
// 与 Go 版逐位对齐：内区（小角度）+ 外区（大角度）双管线、几何四边形反演、
// 大小角度模式自动判定、Alpha/Beta 语义反转（Alpha=侧滑角，Beta=迎角）。
//
// 小程序与 Node 校验端共用（CommonJS require）。
//
// 输入：7 个孔压（P1..P6 外围环，P7 中心，单位 Pa 表压）+ 大气（PAtm 绝对 Pa，TAtm degC）
// 输出：alpha / beta / theta / phi / machNumber / velocity / totalPressure /
//       staticPressure / dynamicPressure / isValid / warning
//
// 注意：PRB 文件无马赫维度——7 个 .prb（7.prb 内区 + 1~6.prb 外区）是同一
// 马赫数下的固定校准集，由调用方一次性加载。

// ── 物理与网格常量（spec-seven-hole-traversal §2.1）──────────────
const INNER_GRID_SIDE = 13;                 // a,b ∈ [-30,30] 步长 5
const INNER_POINT_COUNT = INNER_GRID_SIDE * INNER_GRID_SIDE; // 169
const OUTER_PHI_COUNT = 13;                 // 扇区 phi 网格：center±30 步长 5（13 条线，含 0/360 跨界）
const OUTER_SECTOR_COUNT = 6;
const INNER_GRID_MIN = -30.0;
const GRID_STEP = 5.0;
const OUTER_THETA_MIN = 30.0;
const GRID_EPS = 1e-9;                       // 网格点匹配容差（deg）

const GAS_CONSTANT_R = 287.06;              // 空气气体常数 J/(kg·K)
const GAMMA_AIR = 1.4;                      // 空气比热比
const CELSIUS_TO_KELVIN = 273.15;

// ── 工具函数 ─────────────────────────────────────────────────────
function normalize360(deg) {
  let d = deg % 360;
  if (d < 0) d += 360;
  return d;
}

function angularDiffDeg(a, b) {
  let d = Math.abs(normalize360(a) - normalize360(b));
  if (d > 180) d = 360 - d;
  return d;
}

// 多边形去重（保留首现顺序），与 Python point_in_polygon 的 edge_tuple dedup 一致。
function dedupPolygon(pts) {
  const out = [];
  for (const p of pts) {
    let dup = false;
    for (const q of out) {
      if (p.x === q.x && p.y === q.y) { dup = true; break; }
    }
    if (!dup) out.push(p);
  }
  return out;
}

// 射线法判定点 (x,y) 与多边形关系：0=边界，1=内部，-1=外部。
// 含顶点 1e-9 命中前置检查（修复自提取 PRB 反推时非极值顶点误判为外的 bug）。
function pointInPolygon(x, y, polygon) {
  const n = polygon.length;
  if (n === 0) return -1;
  for (const v of polygon) {
    if (Math.abs(v.x - x) < GRID_EPS && Math.abs(v.y - y) < GRID_EPS) return 0;
  }
  let inside = false;
  let p1 = polygon[0];
  for (let i = 0; i <= n; i++) {
    const p2 = polygon[i % n];
    if (y === p1.y && y === p2.y) {
      if (Math.min(p1.x, p2.x) <= x && x <= Math.max(p1.x, p2.x)) return 0; // on boundary
    }
    if (Math.min(p1.y, p2.y) < y && y <= Math.max(p1.y, p2.y)) {
      if (x <= Math.max(p1.x, p2.x)) {
        if (p1.y !== p2.y) {
          const xInters = (y - p1.y) * (p2.x - p1.x) / (p2.y - p1.y) + p1.x;
          if (p1.x === p2.x || x <= xInters) {
            inside = !inside;
          }
        }
      }
    }
    p1 = p2;
  }
  return inside ? 1 : -1;
}

// 通过两点构造直线 y = k*x + b（Go lineThrough）。
function lineThrough(p, q) {
  const k = (q.y - p.y) / (q.x - p.x);
  return { k, b: p.y - k * p.x };
}

// 定位包含 (ka,kb) 的畸变四边形并反演网格坐标 (a,b)（Go locateInvertAB）。
function locateInvertAB(ka, kb, quads) {
  for (const q of quads) {
    const y1 = q.e[0].k * ka + q.e[0].b;
    const y3 = q.e[2].k * ka + q.e[2].b;
    const x2 = (kb - q.e[1].b) / q.e[1].k;
    const x4 = (kb - q.e[3].b) / q.e[3].k;
    if (y1 <= kb && kb <= y3 && x2 >= ka && ka >= x4) {
      const d1 = Math.abs((-q.e[0].k * ka + kb - q.e[0].b) / Math.sqrt(q.e[0].k * q.e[0].k + 1));
      const d2 = Math.abs((-q.e[1].k * ka + kb - q.e[1].b) / Math.sqrt(q.e[1].k * q.e[1].k + 1));
      const d3 = Math.abs((-q.e[2].k * ka + kb - q.e[2].b) / Math.sqrt(q.e[2].k * q.e[2].k + 1));
      const d4 = Math.abs((-q.e[3].k * ka + kb - q.e[3].b) / Math.sqrt(q.e[3].k * q.e[3].k + 1));
      const b = q.b1 + q.bStep * d1 / (d1 + d3);
      const a = q.a1 + 5 * d4 / (d2 + d4);
      return { a, b, found: true };
    }
  }
  return { a: 0, b: 0, found: false };
}

// 构造畸变四边形（四角 X1..X4 + X1 网格坐标 a1,b1 + b 方向步长 bStep）。
function newDistortedQuad(x1, x2, x3, x4, a1, b1, bStep) {
  return {
    e: [lineThrough(x1, x2), lineThrough(x2, x3), lineThrough(x3, x4), lineThrough(x4, x1)],
    a1, b1, bStep,
  };
}

function innerCoordIndex(v) {
  const idx = Math.round((v - INNER_GRID_MIN) / GRID_STEP);
  if (idx < 0 || idx >= INNER_GRID_SIDE) return { ok: false };
  if (Math.abs(v - (INNER_GRID_MIN + GRID_STEP * idx)) > GRID_EPS) return { ok: false };
  return { idx, ok: true };
}

function outerThetaIndex(theta, thetaCount) {
  const idx = Math.round((theta - OUTER_THETA_MIN) / GRID_STEP);
  if (idx < 0 || idx >= thetaCount) return { ok: false };
  if (Math.abs(theta - (OUTER_THETA_MIN + GRID_STEP * idx)) > GRID_EPS) return { ok: false };
  return { idx, ok: true };
}

function outerPhiIndex(sector, phi) {
  const center = (sector - 1) * 60;
  for (let k = 0; k < OUTER_PHI_COUNT; k++) {
    const line = normalize360(center + 30 - GRID_STEP * k);
    if (angularDiffDeg(phi, line) <= GRID_EPS) return { idx: k, ok: true };
  }
  return { ok: false };
}

// 内区 a/b 轴单元格下界（Go innerCellLo，含 int 向零截断与边界收缩）。
function innerCellLo(v) {
  const k = Math.trunc(v / GRID_STEP);
  if ((0 <= v && v < 30) || v <= -30) return GRID_STEP * k;
  return GRID_STEP * (k - 1);
}

// 内区 cpt/cps 双线性：先 x（a）后 y（b）。
function bilinearXFirst(x, y, xLo, yLo, vX1, vX2, vX3, vX4) {
  const k1 = (vX2 - vX1) / GRID_STEP;
  const cp1 = vX1 + k1 * (x - xLo);
  const k3 = (vX3 - vX4) / GRID_STEP;
  const cp2 = vX3 + k3 * (x - (xLo + GRID_STEP));
  return (cp2 - cp1) * (y - yLo) / GRID_STEP + cp1;
}

// 外区 theta 单元格下界（Go outerThetaCellLo），a==thetaMax 时单元收缩。
function outerThetaCellLo(a, thetaCount) {
  const k = Math.trunc(a / GRID_STEP);
  const kMax = Math.trunc(OUTER_THETA_MIN / GRID_STEP) + thetaCount - 1; // 6 + (thetaCount-1)
  if (k !== kMax) return GRID_STEP * k;
  return GRID_STEP * (kMax - 1);
}

// 外区 phi 单元格（Go outerPhiCell），l==71 特例返回 (355,0)。
function outerPhiCell(b) {
  const l = Math.trunc(b / GRID_STEP);
  if (l === 71) return { lo: 355, hi: 0 };
  return { lo: GRID_STEP * l, hi: GRID_STEP * (l + 1) };
}

function outerCorner(sec, aC, bC) {
  for (let it = 0; it < sec.thetaCount; it++) {
    for (let ip = 0; ip < OUTER_PHI_COUNT; ip++) {
      const gp = sec.points[it][ip];
      if (gp.a === aC && gp.b === bC) return gp;
    }
  }
  return null;
}

// 外区 cpt/cps 双线性：先 b（phi）后 a（theta），方向与前区相反。
function bilinearBFirst(a, b, aLo, bLo, bHi, vX1, vX2, vX3, vX4) {
  const k1 = (vX2 - vX1) / (bHi - bLo);
  const cp1 = vX1 + k1 * (b - bLo);
  const k3 = (vX3 - vX4) / (bHi - bLo);
  const cp2 = vX3 + k3 * (b - bHi);
  return (cp2 - cp1) * (a - aLo) / GRID_STEP + cp1;
}

// 外区网格坐标 (theta,phi) → 输出角度 (alpha,beta)（Go convertThetaPhiToAlphaBeta）。
// alpha 的负号是载荷（phi 自探头尾部看顺时针增长）。
function convertThetaPhiToAlphaBeta(theta, phi) {
  if (Math.abs(theta) >= 89.5) return { alpha: -theta, beta: phi };
  const tTheta = Math.tan(theta * Math.PI / 180);
  const radPhi = phi * Math.PI / 180;
  const alpha = -Math.atan(tTheta * Math.sin(radPhi)) * 180 / Math.PI;
  const beta = Math.atan(tTheta * Math.cos(radPhi)) * 180 / Math.PI;
  return { alpha, beta };
}

function ringPressures(inp) {
  return [inp.P1, inp.P2, inp.P3, inp.P4, inp.P5, inp.P6];
}

// 外围环孔 n（1..6），环绕邻居（n=1 左侧 p6，n=6 右侧 p1）。
function holePressure(inp, n) {
  const p = ringPressures(inp);
  return p[((n - 1) % 6 + 6) % 6];
}

// 最大压力孔（first）与次大压力孔（second），平局取首次出现。
function maxPressureHoles(inp) {
  const p = ringPressures(inp);
  const sorted = p.slice().sort((a, b) => a - b);
  const maxV = sorted[5], secondV = sorted[4];
  let first = 0, second = 0;
  for (let i = 0; i < p.length; i++) { if (p[i] === maxV) { first = i + 1; break; } }
  for (let i = 0; i < p.length; i++) { if (p[i] === secondV) { second = i + 1; break; } }
  return { first, second };
}

// 内区方向系数（小角度，Go innerKaKb）。
function innerKaKb(inp) {
  const pAvg = (inp.P1 + inp.P2 + inp.P3 + inp.P4 + inp.P5 + inp.P6) / 6;
  const denom = inp.P7 - pAvg;
  if (Math.abs(denom) < 1e-12) throw new Error(`小角度模式: |p7-pAverage|=${denom} < 1e-12`);
  const cpa = (inp.P4 - inp.P1) / denom;
  const cpb = (inp.P5 - inp.P2) / denom;
  const cpc = (inp.P6 - inp.P3) / denom;
  const ka = (cpb + cpc) / Math.sqrt(3);
  const kb = -(2 * cpa + cpb - cpc) / 3;
  return { ka, kb };
}

// 外区方向系数（大角度扇区 n，Go outerKaKb）。
function outerKaKb(inp, n) {
  const pc = holePressure(inp, n);
  const pl = holePressure(inp, n - 1);
  const pr = holePressure(inp, n + 1);
  const denom = pc - (pl + pr) / 2;
  if (Math.abs(denom) < 1e-12) throw new Error(`大角度模式孔${n}: |pcenter-(pleft+pright)/2|=${denom} < 1e-12`);
  const ka = (pc - inp.P7) / denom;
  const kb = (pl - pr) / denom;
  return { ka, kb };
}

// 内区总/静压求解（小角度，Go solveInnerPtPs）。
function solveInnerPtPs(inp, cpt, cps) {
  const pAvg = (inp.P1 + inp.P2 + inp.P3 + inp.P4 + inp.P5 + inp.P6) / 6;
  const d = 1 + cpt + cps;
  if (Math.abs(d) < 1e-12) throw new Error(`小角度模式: |1+cpt+cps|=${d} < 1e-12`);
  const pt = (inp.P7 * (1 + cps) + cpt * pAvg) / d;
  const ps = (inp.P7 * cps + pAvg * (1 + cpt)) / d;
  return { pt, ps };
}

// 外区总/静压求解（大角度扇区 n，Go solveOuterPtPs）。
function solveOuterPtPs(inp, n, cpt, cps) {
  const pc = holePressure(inp, n);
  const pMid = (holePressure(inp, n - 1) + holePressure(inp, n + 1)) / 2;
  const d = 1 + cpt + cps;
  if (Math.abs(d) < 1e-12) throw new Error(`大角度模式孔${n}: |1+cpt+cps|=${d} < 1e-12`);
  const pt = (pc * (1 + cps) + cpt * pMid) / d;
  const ps = (pc * cps + pMid * (1 + cpt)) / d;
  return { pt, ps };
}

// 大气计算：速度 V 与马赫数 Ma（Go calVelocityMach）。
function calVelocityMach(pt, ps, patm, tatm) {
  if (isNaN(patm) || !isFinite(patm) || patm <= 0) throw new Error(`大气压力非法: pa=${patm}`);
  const tempK = tatm + CELSIUS_TO_KELVIN;
  if (isNaN(tempK) || !isFinite(tempK) || tempK <= 0) throw new Error(`大气温度非法: t=${tatm} degC`);
  const delta = pt - ps;
  if (delta < 0) throw new Error(`总压低于静压 (pt < ps): pt=${pt}, ps=${ps}`);
  const vSq = 2 * delta * GAS_CONSTANT_R * tempK / patm;
  if (vSq < 0) throw new Error(`速度根号内为负: ${vSq}`);
  const v = Math.sqrt(vSq);
  const psAbs = ps + patm;
  if (psAbs <= 0) throw new Error(`绝对静压非正: ps+pa=${psAbs} (ps=${ps}, pa=${patm})`);
  const ratio = (pt + patm) / psAbs;
  if (ratio < 1) throw new Error(`压力比 ${ratio} < 1 (pt=${pt}, ps=${ps}, pa=${patm})`);
  const maSq = 5 * (Math.pow(ratio, 0.4 / GAMMA_AIR) - 1);
  if (maSq < 0) throw new Error(`马赫数根号内为负: ${maSq}`);
  const ma = Math.sqrt(maSq);
  return { v, ma };
}

function validateFiniteInput(inp) {
  const fields = [
    ['P1', inp.P1], ['P2', inp.P2], ['P3', inp.P3], ['P4', inp.P4],
    ['P5', inp.P5], ['P6', inp.P6], ['P7', inp.P7],
    ['Patm', inp.PAtm], ['Tatm', inp.TAtm],
  ];
  for (const [name, val] of fields) {
    if (isNaN(val) || !isFinite(val)) throw new Error(`输入字段 ${name} 为非有限数值: ${val}`);
  }
}

function assembleResult(input, alpha, beta, theta, phi, pt, ps) {
  const { v, ma } = calVelocityMach(pt, ps, input.PAtm, input.TAtm);
  return {
    alpha, beta, theta, phi,
    machNumber: ma,
    velocity: v,
    totalPressure: pt,
    staticPressure: ps,
    dynamicPressure: pt - ps,
    isValid: true,
  };
}

// ── 主插值器 ─────────────────────────────────────────────────────
class SevenHolePrbInterpolator {
  constructor() {
    this.inner = null;
    this.outer = [null, null, null, null, null, null];
    this.innerPolygon = [];
    this.innerQuads = [];
    this.outerPolygons = [[], [], [], [], [], []];
    this.outerQuads = [[], [], [], [], [], []];
    this.innerSource = '';
    this.outerSources = ['', '', '', '', '', ''];
  }

  isLoaded() {
    if (!this.inner) return false;
    for (const sec of this.outer) if (!sec) return false;
    return true;
  }

  getValidRange() {
    if (!this.inner) return {};
    const g = this.inner;
    return {
      alphaMin: g.points[0][0].a,
      alphaMax: g.points[INNER_GRID_SIDE - 1][0].a,
      betaMin: g.points[0][0].b,
      betaMax: g.points[0][INNER_GRID_SIDE - 1].b,
      machMin: 0,
      machMax: 0,
    };
  }

  // ── 加载 ──
  loadInnerPrbLines(lines, source) {
    const rows = parsePrbDataLines(lines, source);
    if (rows.length !== INNER_POINT_COUNT) {
      throw new Error(`${source}: 内区数据行数必须为 ${INNER_POINT_COUNT}（13×13），实际 ${rows.length} 行`);
    }
    const grid = { points: Array.from({ length: INNER_GRID_SIDE }, () => Array(INNER_GRID_SIDE).fill(null)) };
    const filled = Array.from({ length: INNER_GRID_SIDE }, () => Array(INNER_GRID_SIDE).fill(false));
    for (let i = 0; i < rows.length; i++) {
      const r = rows[i];
      const iaRes = innerCoordIndex(r.a);
      if (!iaRes.ok) throw new Error(`${source} 第${i + 2}行: 内区网格点 a=${r.a} 越界或非网格点 (期望 -30..30 步长5)`);
      const ibRes = innerCoordIndex(r.b);
      if (!ibRes.ok) throw new Error(`${source} 第${i + 2}行: 内区网格点 b=${r.b} 越界或非网格点 (期望 -30..30 步长5)`);
      if (filled[iaRes.idx][ibRes.idx]) throw new Error(`${source} 第${i + 2}行: 重复网格点 (a=${r.a}, b=${r.b})`);
      filled[iaRes.idx][ibRes.idx] = true;
      grid.points[iaRes.idx][ibRes.idx] = r;
    }
    for (let ia = 0; ia < INNER_GRID_SIDE; ia++) {
      for (let ib = 0; ib < INNER_GRID_SIDE; ib++) {
        if (!filled[ia][ib]) throw new Error(`${source}: 内区网格缺失点 (a=${INNER_GRID_MIN + GRID_STEP * ia}, b=${INNER_GRID_MIN + GRID_STEP * ib})`);
      }
    }
    this.inner = grid;
    this.innerSource = source;
    this.buildInnerGeometry();
    return true;
  }

  loadOuterPrbLines(sector, lines, source) {
    if (sector < 1 || sector > OUTER_SECTOR_COUNT) {
      throw new Error(`${source}: 扇区编号 ${sector} 非法，必须在 1..${OUTER_SECTOR_COUNT} 之间`);
    }
    const rows = parsePrbDataLines(lines, source);
    if (rows.length < OUTER_PHI_COUNT * 2 || rows.length % OUTER_PHI_COUNT !== 0) {
      throw new Error(`${source}: 数据行数 ${rows.length} 必须是 ${OUTER_PHI_COUNT} 的整数倍且 ≥${OUTER_PHI_COUNT * 2}（thetaCount×phiCount，phi 固定 ${OUTER_PHI_COUNT} 条）`);
    }
    const thetaCount = rows.length / OUTER_PHI_COUNT;
    const sec = {
      sector,
      centerPhi: (sector - 1) * 60,
      thetaCount,
      points: Array.from({ length: thetaCount }, () => Array(OUTER_PHI_COUNT).fill(null)),
    };
    const filled = Array.from({ length: thetaCount }, () => Array(OUTER_PHI_COUNT).fill(false));
    for (let i = 0; i < rows.length; i++) {
      const r = rows[i];
      const lineNo = i + 2;
      const itRes = outerThetaIndex(r.a, thetaCount);
      if (!itRes.ok) throw new Error(`${source} 第${lineNo}行: 外区网格点 theta=${r.a} 越界或非网格点 (期望 ${OUTER_THETA_MIN}..${OUTER_THETA_MIN + GRID_STEP * (thetaCount - 1)} 步长5)`);
      const ipRes = outerPhiIndex(sector, r.b);
      if (!ipRes.ok) throw new Error(`${source} 第${lineNo}行: 外区扇区${sector} 网格点 phi=${r.b} 越界或非网格点`);
      if (filled[itRes.idx][ipRes.idx]) throw new Error(`${source} 第${lineNo}行: 重复网格点 (theta=${r.a}, phi=${r.b})`);
      filled[itRes.idx][ipRes.idx] = true;
      r.b = normalize360(r.b); // 存储归一化 phi
      sec.points[itRes.idx][ipRes.idx] = r;
    }
    for (let it = 0; it < thetaCount; it++) {
      for (let ip = 0; ip < OUTER_PHI_COUNT; ip++) {
        if (!filled[it][ip]) throw new Error(`${source}: 外区扇区${sector} 网格缺失点 (theta=${OUTER_THETA_MIN + GRID_STEP * it}, phi=${normalize360(sec.centerPhi + 30 - GRID_STEP * ip)})`);
      }
    }
    this.outer[sector - 1] = sec;
    this.outerSources[sector - 1] = source;
    this.buildOuterGeometry(sector);
    return true;
  }

  // ── 几何预计算 ──
  buildInnerGeometry() {
    this.innerPolygon = this.buildInnerPolygon(this.inner);
    this.innerQuads = this.buildInnerQuads(this.inner);
  }

  buildInnerPolygon(g) {
    const pts = [];
    for (let ia = 0; ia < INNER_GRID_SIDE; ia++) pts.push({ x: g.points[ia][0].ka, y: g.points[ia][0].kb });
    for (let ib = 0; ib < INNER_GRID_SIDE; ib++) pts.push({ x: g.points[INNER_GRID_SIDE - 1][ib].ka, y: g.points[INNER_GRID_SIDE - 1][ib].kb });
    for (let ia = INNER_GRID_SIDE - 1; ia >= 0; ia--) pts.push({ x: g.points[ia][INNER_GRID_SIDE - 1].ka, y: g.points[ia][INNER_GRID_SIDE - 1].kb });
    for (let ib = INNER_GRID_SIDE - 1; ib >= 0; ib--) pts.push({ x: g.points[0][ib].ka, y: g.points[0][ib].kb });
    return dedupPolygon(pts);
  }

  buildInnerQuads(g) {
    const quads = [];
    for (let j = 0; j < INNER_GRID_SIDE - 1; j++) {
      for (let i = 0; i < INNER_GRID_SIDE - 1; i++) {
        quads.push(newDistortedQuad(
          { x: g.points[i][j].ka, y: g.points[i][j].kb },
          { x: g.points[i + 1][j].ka, y: g.points[i + 1][j].kb },
          { x: g.points[i + 1][j + 1].ka, y: g.points[i + 1][j + 1].kb },
          { x: g.points[i][j + 1].ka, y: g.points[i][j + 1].kb },
          INNER_GRID_MIN + GRID_STEP * i, INNER_GRID_MIN + GRID_STEP * j, GRID_STEP,
        ));
      }
    }
    return quads;
  }

  buildOuterGeometry(sector) {
    const sec = this.outer[sector - 1];
    this.outerPolygons[sector - 1] = this.buildOuterPolygon(sec);
    this.outerQuads[sector - 1] = this.buildOuterQuads(sec);
  }

  buildOuterPolygon(sec) {
    const thetaCount = sec.thetaCount;
    const pts = [];
    for (let it = 0; it < thetaCount; it++) pts.push({ x: sec.points[it][0].ka, y: sec.points[it][0].kb });
    for (let ip = 0; ip < OUTER_PHI_COUNT; ip++) pts.push({ x: sec.points[thetaCount - 1][ip].ka, y: sec.points[thetaCount - 1][ip].kb });
    for (let it = thetaCount - 1; it >= 0; it--) pts.push({ x: sec.points[it][OUTER_PHI_COUNT - 1].ka, y: sec.points[it][OUTER_PHI_COUNT - 1].kb });
    for (let ip = OUTER_PHI_COUNT - 1; ip >= 0; ip--) pts.push({ x: sec.points[0][ip].ka, y: sec.points[0][ip].kb });
    return dedupPolygon(pts);
  }

  buildOuterQuads(sec) {
    const thetaCount = sec.thetaCount;
    const quads = [];
    for (let j = 0; j < OUTER_PHI_COUNT - 1; j++) {
      for (let i = 0; i < thetaCount - 1; i++) {
        quads.push(newDistortedQuad(
          { x: sec.points[i][j].ka, y: sec.points[i][j].kb },
          { x: sec.points[i + 1][j].ka, y: sec.points[i + 1][j].kb },
          { x: sec.points[i + 1][j + 1].ka, y: sec.points[i + 1][j + 1].kb },
          { x: sec.points[i][j + 1].ka, y: sec.points[i][j + 1].kb },
          OUTER_THETA_MIN + GRID_STEP * i, normalize360(sec.centerPhi + 30 - GRID_STEP * j), -GRID_STEP,
        ));
      }
    }
    return quads;
  }

  // ── 内区管线 ──
  innerZoneInterpolate(ka, kb) {
    const sign = pointInPolygon(ka, kb, this.innerPolygon);
    if (sign < 0) return { coef: null, inZone: false };
    const { a, b, found } = locateInvertAB(ka, kb, this.innerQuads);
    if (!found) throw new Error(`小角度模式: (ka,kb)=(${ka},${kb}) 在边界多边形内但未定位到四边形`);
    if (isNaN(a) || isNaN(b)) throw new Error(`小角度模式: 四边形反演结果非有限 (ka=${ka}, kb=${kb})`);
    const { cpt, cps } = this.innerBilinearCptCps(a, b);
    return { coef: { a, b, cpt, cps }, inZone: true };
  }

  innerBilinearCptCps(a, b) {
    const aLo = innerCellLo(a);
    const bLo = innerCellLo(b);
    const ia = Math.round((aLo - INNER_GRID_MIN) / GRID_STEP);
    const ib = Math.round((bLo - INNER_GRID_MIN) / GRID_STEP);
    const g = this.inner;
    const x1 = g.points[ia][ib];     // (aLo, bLo)
    const x2 = g.points[ia + 1][ib]; // (aLo+5, bLo)
    const x3 = g.points[ia + 1][ib + 1]; // (aLo+5, bLo+5)
    const x4 = g.points[ia][ib + 1]; // (aLo, bLo+5)
    const cpt = bilinearXFirst(a, b, aLo, bLo, x1.cpt, x2.cpt, x3.cpt, x4.cpt);
    const cps = bilinearXFirst(a, b, aLo, bLo, x1.cps, x2.cps, x3.cps, x4.cps);
    return { cpt, cps };
  }

  // ── 外区管线 ──
  outerZoneTrySector(sector, ka, kb) {
    const sign = pointInPolygon(ka, kb, this.outerPolygons[sector - 1]);
    if (sign < 0) return { coef: null, hit: false };
    const { a, b, found } = locateInvertAB(ka, kb, this.outerQuads[sector - 1]);
    if (!found) {
      const gp = this.outerFindGridPointByKaKb(this.outer[sector - 1], ka, kb);
      if (gp) return { coef: { a: gp.a, b: gp.b, cpt: gp.cpt, cps: gp.cps }, hit: true };
      throw new Error(`大角度模式孔${sector}: (ka,kb)=(${ka},${kb}) 在扇区多边形内但未定位到四边形`);
    }
    if (isNaN(a) || isNaN(b)) throw new Error(`大角度模式孔${sector}: 四边形反演结果非有限 (ka=${ka}, kb=${kb})`);
    let cptcps;
    try {
      cptcps = this.outerBilinearCptCps(this.outer[sector - 1], a, b);
    } catch (e) {
      const gp = this.outerFindGridPointByKaKb(this.outer[sector - 1], ka, kb);
      if (gp) return { coef: { a: gp.a, b: gp.b, cpt: gp.cpt, cps: gp.cps }, hit: true };
      throw e;
    }
    return { coef: { a, b, cpt: cptcps.cpt, cps: cptcps.cps }, hit: true };
  }

  outerFindGridPointByKaKb(sec, ka, kb) {
    const findGridPointEps = 1e-6;
    if (!sec) return null;
    for (let it = 0; it < sec.thetaCount; it++) {
      for (let ip = 0; ip < OUTER_PHI_COUNT; ip++) {
        const gp = sec.points[it][ip];
        if (Math.abs(gp.ka - ka) < findGridPointEps && Math.abs(gp.kb - kb) < findGridPointEps) return gp;
      }
    }
    return null;
  }

  outerBilinearCptCps(sec, a, b) {
    const aLo = outerThetaCellLo(a, sec.thetaCount);
    const aHi = aLo + GRID_STEP;
    const { lo: bLo, hi: bHi } = outerPhiCell(b);
    const x1 = outerCorner(sec, aLo, bLo);
    const x2 = outerCorner(sec, aLo, bHi);
    const x3 = outerCorner(sec, aHi, bHi);
    const x4 = outerCorner(sec, aHi, bLo);
    if (!x1 || !x2 || !x3 || !x4) {
      throw new Error(`大角度模式扇区${sec.sector}: cpt/cps 单元格角点缺失 (a=${a}, b=${b}, 单元 a=[${aLo},${aHi}] b=[${bLo},${bHi}])`);
    }
    const cpt = bilinearBFirst(a, b, aLo, bLo, bHi, x1.cpt, x2.cpt, x3.cpt, x4.cpt);
    const cps = bilinearBFirst(a, b, aLo, bLo, bHi, x1.cps, x2.cps, x3.cps, x4.cps);
    return { cpt, cps };
  }

  // ── 主计算 ──
  calculate(input) {
    try {
      validateFiniteInput(input);
      if (!this.isLoaded()) throw new Error('七孔PRB校准数据未加载');
      // 内区优先（小角度）
      const { ka, kb } = innerKaKb(input);
      const innerRes = this.innerZoneInterpolate(ka, kb);
      if (innerRes.inZone) {
        const { pt, ps } = solveInnerPtPs(input, innerRes.coef.cpt, innerRes.coef.cps);
        // 内区网格坐标 (a,b) == (alpha,beta)，theta/phi 与之同值
        return assembleResult(input, innerRes.coef.a, innerRes.coef.b, innerRes.coef.a, innerRes.coef.b, pt, ps);
      }
      // 大角度：先最大压力孔扇区，再次大（first/second 候选）
      const { first, second } = maxPressureHoles(input);
      for (const sector of [first, second]) {
        const { ka: kaO, kb: kbO } = outerKaKb(input, sector);
        const zc = this.outerZoneTrySector(sector, kaO, kbO);
        if (!zc.hit) continue;
        const { pt, ps } = solveOuterPtPs(input, sector, zc.coef.cpt, zc.coef.cps);
        const { alpha, beta } = convertThetaPhiToAlphaBeta(zc.coef.a, zc.coef.b);
        return assembleResult(input, alpha, beta, zc.coef.a, zc.coef.b, pt, ps);
      }
      return {
        alpha: 0, beta: 0, theta: 0, phi: 0,
        machNumber: 0, velocity: 0, totalPressure: 0, staticPressure: 0, dynamicPressure: 0,
        isValid: false,
        warning: '压力系数超出七孔PRB校准网格范围，不支持外推',
      };
    } catch (e) {
      throw new Error('七孔插值计算内部panic: ' + e.message);
    }
  }
}

// ── PRB 行解析 ───────────────────────────────────────────────────
// 跳过首行（表头），每行 6 列 ka kb cpt cps a b（空格分隔的有限数值）。
function parsePrbDataLines(lines, source) {
  if (lines.length === 0) throw new Error(`${source}: 文件为空，缺少表头行`);
  const data = lines.slice(1); // 跳过表头
  if (data.length === 0) throw new Error(`${source}: 缺少数据行（仅表头）`);
  const rows = [];
  for (let i = 0; i < data.length; i++) {
    const lineNo = i + 2;
    const fields = data[i].trim().split(/\s+/);
    if (fields.length !== 6) {
      throw new Error(`${source} 第${lineNo}行: 必须包含 6 列 (ka kb cpt cps a b)，实际 ${fields.length} 列`);
    }
    const v = [];
    for (let j = 0; j < 6; j++) {
      const x = Number(fields[j]);
      if (!isFinite(x)) throw new Error(`${source} 第${lineNo}行第${j + 1}列: 不是有效数字 ${fields[j]}`);
      v.push(x);
    }
    rows.push({ ka: v[0], kb: v[1], cpt: v[2], cps: v[3], a: v[4], b: v[5] });
  }
  return rows;
}

module.exports = { SevenHolePrbInterpolator };
