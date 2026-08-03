// MultiPrbInterpolator —— 端口自 shared/algorithms/go/fivehole/interpolation/multi_prb_interpolator.go
// 支持加载多个不同马赫数的 PRB 文件，按实时 Ma 自动选择/线性插值。
// 与桌面端 probe-interpolator 的 5 孔入口保持一致：
//   NewMultiPrbInterpolator() -> loadPrbData(files, machNumbers) -> calculate(input)

const { PrbInterpolator } = require('./prb-interpolator.js');

const MODE_NEAREST = 'nearest';
const MODE_LINEAR = 'linear';

function lerp(a, b, t) { return a + (b - a) * t; }

// 角度线性插值，取最短路径
function angleLerp(a, b, t) {
  let diff = b - a;
  if (diff > 180) diff -= 360;
  else if (diff < -180) diff += 360;
  let result = a + diff * t;
  while (result <= -180) result += 360;
  while (result > 180) result -= 360;
  return result;
}

function containsFloat(slice, val) {
  return slice.some((v) => Math.abs(v - val) < 1e-9);
}

function mergeWarnings() {
  const seen = {};
  const parts = [];
  for (let i = 0; i < arguments.length; i++) {
    const warning = arguments[i];
    if (!warning) continue;
    for (const part of warning.split(';')) {
      const p = part.trim();
      if (p === '' || seen[p]) continue;
      seen[p] = true;
      parts.push(p);
    }
  }
  return parts.join('; ');
}

// 从文件名解析马赫数：支持 "0.5Ma.prb" / "Ma0.5.prb"
function parseMachFromFileName(filePath) {
  const parts = filePath.split(/[\\/]/);
  const fileName = parts[parts.length - 1];
  const patterns = [/([0-9]+(?:\.[0-9]+)?)Ma/i, /Ma([0-9]+(?:\.[0-9]+)?)/i];
  for (const re of patterns) {
    const m = fileName.match(re);
    if (m) {
      const ma = Number(m[1]);
      if (isFinite(ma)) return ma;
    }
  }
  return 0;
}

class MultiPrbInterpolator {
  constructor() {
    this.prbFiles = [];
    this.sortedMachNumbers = [];
    this.loaded = false;
    this.mode = MODE_LINEAR;
  }

  clearState() {
    this.prbFiles = [];
    this.sortedMachNumbers = [];
    this.loaded = false;
  }

  // fileData: [{ filePath, lines: string[] }]
  // machNumbers: 可选，显式马赫数数组；为空则从文件名解析
  loadPrbData(fileData, machNumbers) {
    this.clearState();
    const files = [];
    const loadedMachNumbers = [];
    const warnings = [];

    for (let i = 0; i < fileData.length; i++) {
      const data = fileData[i];
      const filePath = data.filePath;
      const fileName = filePath.split(/[\\/]/).pop();

      const interpolator = new PrbInterpolator();
      try {
        interpolator.loadPrbLines(data.lines, filePath);
      } catch (e) {
        warnings.push('加载PRB文件失败: ' + fileName + ' - ' + e.message);
        continue;
      }

      let machNumber;
      if (machNumbers && i < machNumbers.length && isFiniteNum(machNumbers[i])) {
        machNumber = machNumbers[i];
      } else {
        const parsed = parseMachFromFileName(filePath);
        if (parsed === 0) {
          warnings.push('无法从文件名解析马赫数: ' + fileName + ', 已跳过');
          continue;
        }
        machNumber = parsed;
      }

      if (containsFloat(loadedMachNumbers, machNumber)) {
        warnings.push('马赫数 ' + machNumber.toFixed(3) + ' 重复, 已跳过文件: ' + fileName);
        continue;
      }

      const validRange = interpolator.getValidRange();
      validRange.MachMin = machNumber;
      validRange.MachMax = machNumber;

      this.prbFiles.push({ FileInfo: { filePath, fileName, validRange }, Interpolator: interpolator, MachNumber: machNumber });
      files.push({ filePath, fileName, validRange });
      loadedMachNumbers.push(machNumber);
    }

    this.prbFiles.sort((a, b) => a.MachNumber - b.MachNumber);
    this.sortedMachNumbers = this.prbFiles.map((f) => f.MachNumber);

    if (this.prbFiles.length === 0) {
      return { ok: false, error: '没有成功加载任何PRB文件', files, machNumbers: loadedMachNumbers, warnings };
    }
    if (this.prbFiles.length === 1) {
      warnings.push('只加载了一个PRB文件，多马赫数插值功能将退化为单文件模式');
    }
    this.loaded = true;
    return { ok: true, error: null, files, machNumbers: loadedMachNumbers, warnings };
  }

  isLoaded() { return this.loaded; }

  getValidRange() {
    if (this.prbFiles.length === 0) {
      return { AlphaMin: -30, AlphaMax: 30, BetaMin: -30, BetaMax: 30, MachMin: 0, MachMax: 0 };
    }
    let vr = this.prbFiles[0].FileInfo.validRange;
    let alphaMin = vr.AlphaMin, alphaMax = vr.AlphaMax, betaMin = vr.BetaMin, betaMax = vr.BetaMax;
    let machMin = vr.MachMin, machMax = vr.MachMax;
    for (let i = 1; i < this.prbFiles.length; i++) {
      const r = this.prbFiles[i].FileInfo.validRange;
      alphaMin = Math.min(alphaMin, r.AlphaMin); alphaMax = Math.max(alphaMax, r.AlphaMax);
      betaMin = Math.min(betaMin, r.BetaMin); betaMax = Math.max(betaMax, r.BetaMax);
      machMin = Math.min(machMin, r.MachMin); machMax = Math.max(machMax, r.MachMax);
    }
    return { AlphaMin: alphaMin, AlphaMax: alphaMax, BetaMin: betaMin, BetaMax: betaMax, MachMin: machMin, MachMax: machMax };
  }

  calculate(input) {
    if (!this.loaded || this.prbFiles.length === 0) {
      return { result: null, error: 'PRB文件未加载' };
    }
    // 单文件模式
    if (this.prbFiles.length === 1) {
      return this.prbFiles[0].Interpolator.calculate(input);
    }
    // 先以中间马赫文件算初始 Ma
    const middleIndex = Math.floor(this.prbFiles.length / 2);
    const initial = this.prbFiles[middleIndex].Interpolator.calculate(input);
    if (initial.error) return { result: null, error: initial.error };
    const initialResult = initial.result;
    if (!initialResult.isValid || !isFiniteNum(initialResult.machNumber)) {
      return { result: initialResult, error: null };
    }
    const targetMa = initialResult.machNumber;
    if (this.mode === MODE_NEAREST) return this.calculateWithNearest(targetMa, input);
    return this.calculateWithLinear(targetMa, input);
  }

  calculateWithNearest(targetMa, input) {
    const nearestIdx = this.findNearestMachIndex(targetMa);
    const r = this.prbFiles[nearestIdx].Interpolator.calculate(input);
    if (r.error) return { result: null, error: r.error };
    const result = r.result;
    const selectedMa = this.prbFiles[nearestIdx].MachNumber;
    const maDiff = Math.abs(targetMa - selectedMa);
    if (maDiff > 0.01) {
      const warning = '目标 Ma=' + targetMa.toFixed(3) + ' 使用 Ma=' + selectedMa.toFixed(3) + ' 的PRB文件插值 (偏差=' + maDiff.toFixed(3) + ')';
      result.warning = result.warning ? result.warning + '; ' + warning : warning;
    }
    return { result, error: null };
  }

  calculateWithLinear(targetMa, input) {
    const [lower, upper] = this.findMachInterval(targetMa);
    if (lower === upper) return this.calculateWithNearest(targetMa, input);

    const lr = this.prbFiles[lower].Interpolator.calculate(input);
    const ur = this.prbFiles[upper].Interpolator.calculate(input);
    if (lr.error) return { result: null, error: lr.error };
    if (ur.error) return { result: null, error: ur.error };
    const lowerResult = lr.result, upperResult = ur.result;
    if (!lowerResult.isValid || !upperResult.isValid) return this.calculateWithNearest(targetMa, input);

    const lowerMa = this.prbFiles[lower].MachNumber;
    const upperMa = this.prbFiles[upper].MachNumber;
    const weight = (targetMa - lowerMa) / (upperMa - lowerMa);

    const result = {
      alpha: angleLerp(lowerResult.alpha, upperResult.alpha, weight),
      beta: angleLerp(lowerResult.beta, upperResult.beta, weight),
      machNumber: lerp(lowerResult.machNumber, upperResult.machNumber, weight),
      v: lerp(lowerResult.v, upperResult.v, weight),
      vx: lerp(lowerResult.vx, upperResult.vx, weight),
      vy: lerp(lowerResult.vy, upperResult.vy, weight),
      vz: lerp(lowerResult.vz, upperResult.vz, weight),
      velocity: lerp(lowerResult.velocity, upperResult.velocity, weight),
      cas: lerp(lowerResult.cas, upperResult.cas, weight),
      sat: lerp(lowerResult.sat, upperResult.sat, weight),
      dynamicPressure: lerp(lowerResult.dynamicPressure, upperResult.dynamicPressure, weight),
      density: lerp(lowerResult.density, upperResult.density, weight),
      P0: lerp(lowerResult.P0, upperResult.P0, weight),
      Ps: lerp(lowerResult.Ps, upperResult.Ps, weight),
      isValid: true,
      warning: 'Ma=' + targetMa.toFixed(3) + ' 在 Ma=' + lowerMa.toFixed(3) + ' 和 Ma=' + upperMa.toFixed(3) + ' 之间线性插值 (权重=' + weight.toFixed(3) + ')',
    };
    result.warning = mergeWarnings(result.warning, lowerResult.warning, upperResult.warning);
    return { result, error: null };
  }

  setInterpolationMode(mode) { this.mode = mode; }
  getMachRange() {
    if (this.sortedMachNumbers.length === 0) return [0, 0];
    return [this.sortedMachNumbers[0], this.sortedMachNumbers[this.sortedMachNumbers.length - 1]];
  }
  getMachNumbers() { return this.sortedMachNumbers.slice(); }

  findNearestMachIndex(targetMa) {
    let nearestIdx = 0;
    let minDiff = Math.abs(targetMa - this.sortedMachNumbers[0]);
    for (let i = 1; i < this.sortedMachNumbers.length; i++) {
      const diff = Math.abs(targetMa - this.sortedMachNumbers[i]);
      if (diff < minDiff) { minDiff = diff; nearestIdx = i; }
    }
    return nearestIdx;
  }

  findMachInterval(targetMa) {
    const n = this.sortedMachNumbers.length;
    if (targetMa <= this.sortedMachNumbers[0]) return [0, 0];
    if (targetMa >= this.sortedMachNumbers[n - 1]) return [n - 1, n - 1];
    for (let i = 0; i < n - 1; i++) {
      if (targetMa >= this.sortedMachNumbers[i] && targetMa <= this.sortedMachNumbers[i + 1]) return [i, i + 1];
    }
    return [0, 0];
  }
}

function isFiniteNum(f) { return typeof f === 'number' && isFinite(f); }

module.exports = { MultiPrbInterpolator, MODE_NEAREST, MODE_LINEAR, parseMachFromFileName };
