// verify.js —— Node 端数值校验：用 TS 移植版跑 reference.json 中的输入，逐字段对比 Go 原版。
// 运行：node verify.js
// 依赖：reference.json（由 genref.go 生成，GOWORK=off go run genref.go <golden_prb_dir> reference.json）

const fs = require('fs');
const path = require('path');
const { PrbInterpolator } = require('../utils/algorithms/fivehole/prb-interpolator.js');

// 与 Go syntheticPrbLines 完全一致的合成 PRB 生成（13x13 网格）
function syntheticPrbLines(cpt, cps) {
  const lines = ['13 13'];
  for (let alpha = -30; alpha <= 30; alpha += 5) {
    for (let beta = -30; beta <= 30; beta += 5) {
      lines.push(
        (alpha / 100).toFixed(6) + ' ' +
        (beta / 100).toFixed(6) + ' ' +
        cpt.toFixed(6) + ' ' +
        cps.toFixed(6) + ' ' +
        alpha.toFixed(0) + ' ' +
        beta.toFixed(0)
      );
    }
  }
  return lines;
}

const FIELDS = ['alpha', 'beta', 'machNumber', 'v', 'vx', 'vy', 'vz', 'cas', 'sat', 'dynamicPressure', 'density', 'P0', 'Ps'];
const TOL = 1e-9; // 纯 double 数学，Go 与 JS 应逐位一致

function loadOnce() {
  const ip = new PrbInterpolator();
  ip.loadPrbLines(syntheticPrbLines(0.05, 0.01), '0.5Ma.prb');
  return ip;
}

function main() {
  const refPath = path.join(__dirname, 'reference.json');
  if (!fs.existsSync(refPath)) {
    console.error('未找到 reference.json，请先运行: GOWORK=off go run genref.go <golden_prb_dir> reference.json');
    process.exit(2);
  }
  const refs = JSON.parse(fs.readFileSync(refPath, 'utf-8'));

  const ip = loadOnce();
  let pass = 0, fail = 0;
  const failures = [];

  for (const c of refs) {
    const input = c.input;
    const go = c.go;
    const { result, error } = ip.calculate(input);

    if (error) {
      fail++;
      failures.push(`[${c.name}] TS 计算报错: ${error}`);
      continue;
    }
    // isValid 必须一致
    if (result.isValid !== go.isValid) {
      fail++;
      failures.push(`[${c.name}] isValid 不一致: TS=${result.isValid} Go=${go.isValid} (warning_TS=${result.warning})`);
      continue;
    }
    // 数值字段
    let caseOk = true;
    const diffs = [];
    for (const f of FIELDS) {
      const a = result[f], b = go[f];
      if (typeof a !== 'number' || typeof b !== 'number') continue;
      if (!isFinite(a) && !isFinite(b)) continue;
      if (Math.abs(a - b) > TOL) {
        caseOk = false;
        diffs.push(`${f}: TS=${a} Go=${b} Δ=${Math.abs(a - b)}`);
      }
    }
    if (caseOk) {
      pass++;
    } else {
      fail++;
      failures.push(`[${c.name}] 数值超差:\n    ` + diffs.join('\n    '));
    }
  }

  console.log(`\n===== 五孔 PRB 移植数值校验 =====`);
  console.log(`用例数: ${refs.length}  PASS: ${pass}  FAIL: ${fail}`);
  if (failures.length) {
    console.log('\n失败明细:');
    for (const f of failures) console.log(' - ' + f);
    process.exit(1);
  } else {
    console.log('全部通过 ✅ TS 端口与 Go 原版数值一致（容差 ' + TOL + '）。');
  }
}

main();
