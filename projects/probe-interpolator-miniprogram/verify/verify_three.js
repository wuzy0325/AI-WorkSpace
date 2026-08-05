// verify_three.js —— Node 端数值校验：用三孔 JS 移植版跑 reference_three.json 的输入，逐字段对比 Go 原版。
// 运行：node verify_three.js
// 依赖：reference_three.json（由 genref_three.go 生成）

const fs = require('fs');
const path = require('path');
const { ThreeHoleInterpolator } = require('../utils/algorithms/threehole/three-hole.js');

// 与 genref_three.go 的 threeSynthLines 逐字节一致
function threeSynthLines(cma) {
  const lines = [cma.toFixed(6), '13'];
  for (let alpha = -30; alpha <= 30; alpha += 5) {
    const kb = (4 + 2 * cma) * Math.sin((alpha * Math.PI) / 180);
    const k0 = 0.5 + 0.01 * alpha + 0.1 * cma;
    const kv = 2.0 + 0.02 * alpha + 0.05 * cma;
    lines.push(kb.toFixed(10) + ' ' + k0.toFixed(10) + ' ' + kv.toFixed(10) + ' ' + alpha.toFixed(0));
  }
  return lines;
}

// velocity 与 machNumber/P0/Ps 同为 double 数值字段，统一纳入跨实现数值比对
const FIELDS = ['alpha', 'machNumber', 'velocity', 'P0', 'Ps'];
const TOL = 1e-9; // 纯 double 数学，Go 与 JS 应逐位一致

function buildRealInterp() {
  const prbPath = path.join(
    __dirname, '..', '..', '..',
    'shared', 'algorithms', 'go', 'threehole', 'interpolation', 'testdata', '0.8.prb'
  );
  const text = fs.readFileSync(prbPath, 'utf-8');
  const lines = text.split('\n');
  const interp = new ThreeHoleInterpolator();
  const res = interp.loadPrbData([{ filePath: '0.8.prb', lines }]);
  if (!res.ok) throw new Error('加载真实 0.8.prb 失败: ' + res.error);
  return interp;
}

function getMultiMaInterp(cmaList) {
  const key = cmaList.join(',');
  if (!getMultiMaInterp._cache) getMultiMaInterp._cache = {};
  if (getMultiMaInterp._cache[key]) return getMultiMaInterp._cache[key];
  const fileData = cmaList.map((cma) => ({
    filePath: cma.toFixed(1) + 'Ma.prb',
    lines: threeSynthLines(cma),
  }));
  const interp = new ThreeHoleInterpolator();
  const res = interp.loadPrbData(fileData);
  if (!res.ok) throw new Error('加载合成多马赫 .prb 失败: ' + res.error);
  getMultiMaInterp._cache[key] = interp;
  return interp;
}

function main() {
  const refPath = path.join(__dirname, 'reference_three.json');
  if (!fs.existsSync(refPath)) {
    console.error('未找到 reference_three.json，请先运行: GOWORK=off go run genref_three.go <golden_dir> reference_three.json <0.8.prb>');
    process.exit(2);
  }
  const refs = JSON.parse(fs.readFileSync(refPath, 'utf-8'));

  const realInterp = buildRealInterp();
  let pass = 0, fail = 0;
  const failures = [];

  for (const c of refs) {
    const input = c.input;
    const go = c.go;
    const interp = c.cmaList ? getMultiMaInterp(c.cmaList) : realInterp;
    const { result, error } = interp.calculate(input);

    if (error) {
      fail++;
      failures.push(`[${c.name}] JS 计算报错: ${error}`);
      continue;
    }
    // calculated 必须一致（区分"未计算"与"已计算-参考"）
    if (!!result.calculated !== !!go.calculated) {
      fail++;
      failures.push(`[${c.name}] calculated 不一致: JS=${result.calculated} Go=${go.calculated} (warning_JS=${result.warning})`);
      continue;
    }
    // isValid 必须一致
    if (result.isValid !== go.isValid) {
      fail++;
      failures.push(`[${c.name}] isValid 不一致: JS=${result.isValid} Go=${go.isValid} (warning_JS=${result.warning})`);
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
        diffs.push(`${f}: JS=${a} Go=${b} Δ=${Math.abs(a - b)}`);
      }
    }
    // warning 字符串完全一致
    if (String(result.warning) !== String(go.warning)) {
      caseOk = false;
      diffs.push(`warning: JS=${JSON.stringify(result.warning)} Go=${JSON.stringify(go.warning)}`);
    }
    if (caseOk) {
      pass++;
    } else {
      fail++;
      failures.push(`[${c.name}] 数值超差:\n    ` + diffs.join('\n    '));
    }
  }

  console.log(`\n===== 三孔 PRB 移植数值校验 =====`);
  console.log(`用例数: ${refs.length}  PASS: ${pass}  FAIL: ${fail}`);
  if (failures.length) {
    console.log('\n失败明细:');
    for (const f of failures) console.log(' - ' + f);
    process.exit(1);
  } else {
    console.log('全部通过 ✅ JS 端口与 Go 原版数值一致（容差 ' + TOL + '）。');
  }
}

main();
