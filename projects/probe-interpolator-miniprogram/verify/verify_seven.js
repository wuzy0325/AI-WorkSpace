'use strict';
// 七孔 JS 端口数值校验：用 Go 原版生成的 reference_seven.json（含 7 个 PRB 行与逐用例结果）
// 加载同款 PRB 并跑同输入，逐字段对比，核对 JS 与 Go 逐位一致。
const fs = require('fs');
const path = require('path');
const { SevenHolePrbInterpolator } = require(path.join(__dirname, '..', 'utils', 'algorithms', 'sevenhole', 'seven-hole.js'));

const ref = JSON.parse(fs.readFileSync(path.join(__dirname, 'reference_seven.json'), 'utf8'));

// 自包含加载 Go 参考中嵌入的 7 个 PRB 文件行。
const interp = new SevenHolePrbInterpolator();
interp.loadInnerPrbLines(ref.prb.innerLines, ref.prb.inner);
for (const o of ref.prb.outer) {
  interp.loadOuterPrbLines(o.sector, o.lines, o.name);
}
if (!interp.isLoaded()) {
  console.error('JS 加载 7 个 PRB 失败');
  process.exit(1);
}

const TOL_A = 1e-7;    // 角度绝对容差
const TOL_REL = 1e-9;  // 相对容差
const TOL_ABS = 1e-6;  // 绝对容差（压力 / 速度）

function close(got, want, tolAbs, tolRel) {
  if (got === want) return true;
  const a = Math.abs(got - want);
  return a <= tolAbs || a <= tolRel * Math.max(1e-12, Math.abs(want));
}

let pass = 0, fail = 0;
const maxErr = { alpha: 0, beta: 0, mach: 0, v: 0, pt: 0, ps: 0, theta: 0, phi: 0 };

for (const c of ref.cases) {
  const go = c.go;
  let res;
  try {
    res = interp.calculate({
      P1: c.input.P1, P2: c.input.P2, P3: c.input.P3, P4: c.input.P4,
      P5: c.input.P5, P6: c.input.P6, P7: c.input.P7,
      PAtm: c.input.PAtm, TAtm: c.input.TAtm,
    });
  } catch (e) {
    fail++;
    if (fail <= 10) console.log('JS-ERROR idx', c.index, c.mode, e.message);
    continue;
  }

  // 有效性必须一致
  if (res.isValid !== go.isValid) {
    fail++;
    if (fail <= 10) console.log('VALID-MISMATCH idx', c.index, c.mode, 'js=', res.isValid, 'go=', go.isValid);
    continue;
  }
  if (!res.isValid) {
    // 超网格（out 模式）：Go 返回 IsValid=false，JS 应一致；warning 含「外推」。
    pass++;
    continue;
  }

  const da = Math.abs(res.alpha - go.alpha);
  const db = Math.abs(res.beta - go.beta);
  const dth = Math.abs(res.theta - go.theta);
  const dph = Math.abs(res.phi - go.phi);
  const dma = Math.abs(res.machNumber - go.machNumber);
  const dv = Math.abs(res.velocity - go.velocity);
  const dpt = Math.abs(res.totalPressure - go.totalPressure);
  const dps = Math.abs(res.staticPressure - go.staticPressure);
  maxErr.alpha = Math.max(maxErr.alpha, da);
  maxErr.beta = Math.max(maxErr.beta, db);
  maxErr.theta = Math.max(maxErr.theta, dth);
  maxErr.phi = Math.max(maxErr.phi, dph);
  maxErr.mach = Math.max(maxErr.mach, dma);
  maxErr.v = Math.max(maxErr.v, dv);
  maxErr.pt = Math.max(maxErr.pt, dpt);
  maxErr.ps = Math.max(maxErr.ps, dps);

  const ok =
    da <= TOL_A && db <= TOL_A && dth <= TOL_A && dph <= TOL_A &&
    close(res.machNumber, go.machNumber, TOL_ABS, TOL_REL) &&
    close(res.velocity, go.velocity, TOL_ABS, TOL_REL) &&
    close(res.totalPressure, go.totalPressure, TOL_ABS, TOL_REL) &&
    close(res.staticPressure, go.staticPressure, TOL_ABS, TOL_REL);
  if (ok) {
    pass++;
  } else {
    fail++;
    if (fail <= 10) console.log('MISMATCH idx', c.index, c.mode,
      'alpha', res.alpha, go.alpha, 'beta', res.beta, go.beta,
      'ma', res.machNumber, go.machNumber, 'pt', res.totalPressure, go.totalPressure);
  }
}

console.log('===== 七孔 PRB 移植数值校验 =====');
console.log('用例数:', ref.cases.length, ' PASS:', pass, ' FAIL:', fail);
console.log('最大误差:',
  'alpha', maxErr.alpha.toExponential(3),
  'beta', maxErr.beta.toExponential(3),
  'theta', maxErr.theta.toExponential(3),
  'phi', maxErr.phi.toExponential(3),
  'ma', maxErr.mach.toExponential(3),
  'v', maxErr.v.toExponential(3),
  'pt', maxErr.pt.toExponential(3),
  'ps', maxErr.ps.toExponential(3));
if (fail === 0) {
  console.log('全部通过 ✅ JS 端口与 Go 原版数值一致。');
} else {
  console.log('存在失败 ❌');
  process.exit(1);
}
