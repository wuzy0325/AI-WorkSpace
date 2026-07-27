// three-hole.js —— 端口自 shared/algorithms/go/threehole/interpolation/three_hole.go
// ThreeHoleInterpolator：三孔探针迭代插值。与桌面端 probe-interpolator 的 3 孔入口一致：
//   new ThreeHoleInterpolator() -> loadPrbData(files) -> calculate(input)
//
// 关键差异（相对五孔 PRB 链路）：
//   - 文件首行内嵌 CMa（校准马赫数），第二行 Nalpha，随后 Nalpha 行 "Kb K0 Kv Alpha"。
//   - 单个 ThreeHoleInterpolator 即可加载多个 .prb（不同 Ma），calculate 内部按实时 Ma
//     自动挑选/线性插值两张最近马赫表的 Kb/K0/Kv/Alpha。
//   - 输入仅 3 个孔压（P1/P2/P3）+ 大气参数；输出不含 Beta/V/CAS/SAT 等派生量。

const MAX_ITERATIONS = 20;
const CONVERGE_TOL = 1e-4;
const DELTA_P_TOL = 1e-6;

// 空气比热比温度修正系数（-40°C ~ +60°C 范围内误差 <0.1%）
const GAMMA_REF = 1.4; // 20°C 时的参考比热比（γ）
const TEMP_REF = 20.0; // 参考温度(°C)
const TEMP_COEFF = 0.0002; // 温度修正系数 γ ≈ gammaRef - tempCoeff*(T-tempRef)

function isFiniteNum(f) {
  return typeof f === 'number' && isFinite(f);
}

function baseName(p) {
  if (!p) return '';
  const parts = String(p).split(/[\\/]/);
  return parts[parts.length - 1];
}

// 计算马赫数：由总压/静压（表压）+ 大气压换算绝对压
function calcMach(pt, ps, pa, tatm) {
  const absPs = ps + pa;
  if (absPs < DELTA_P_TOL) return 0;
  const absPt = pt + pa;
  const ratio = absPt / absPs;
  const gamma = calcGamma(tatm);
  const exp = (gamma - 1) / gamma;
  const coeff = 2 / (gamma - 1);
  const powered = Math.pow(ratio, exp);
  return Math.sqrt(coeff * Math.abs(powered - 1));
}

// 空气比热比随温度近似变化 γ ≈ gammaRef - tempCoeff*(T-tempRef)
function calcGamma(tatm) {
  if (!isFiniteNum(tatm)) return GAMMA_REF;
  return GAMMA_REF - TEMP_COEFF * (tatm - TEMP_REF);
}

class ThreeHoleInterpolator {
  constructor() {
    this.loaded = false;
    this.calib = []; // [{ CMa, Nalpha, Items: [{Kb,K0,Kv,Alpha}] }]
    this.alphaSeq = [];
    this.initMa = 0;
    this.minMa = 0;
    this.maxMa = 0;
  }

  isLoaded() {
    return this.loaded;
  }

  getValidRange() {
    if (!this.loaded) {
      return { AlphaMin: 0, AlphaMax: 0, MachMin: 0, MachMax: 0 };
    }
    return {
      AlphaMin: this.alphaSeq[0],
      AlphaMax: this.alphaSeq[this.alphaSeq.length - 1],
      MachMin: this.minMa,
      MachMax: this.maxMa,
    };
  }

  // fileData: [{ filePath, lines: string[] }]
  loadPrbData(fileData) {
    this.loaded = false;
    this.calib = [];
    this.alphaSeq = [];

    const calibList = [];
    let firstNalpha = null;

    for (const fd of fileData) {
      const cal = this.parsePrbLines(fd.lines);
      if (cal === null) {
        return { ok: false, error: '解析 ' + baseName(fd.filePath) + ' 失败' };
      }
      if (firstNalpha === null) {
        firstNalpha = cal.Nalpha;
      } else if (cal.Nalpha !== firstNalpha) {
        return {
          ok: false,
          error: '文件 ' + baseName(fd.filePath) + ' 的Nalpha(' + cal.Nalpha + ')与其他文件(' + firstNalpha + ')不一致',
        };
      }
      calibList.push(cal);
    }

    if (calibList.length === 0) {
      return { ok: false, error: '未加载任何有效校准数据' };
    }

    let sumMa = 0;
    let minMa = calibList[0].CMa;
    let maxMa = calibList[0].CMa;
    for (const c of calibList) {
      sumMa += c.CMa;
      if (c.CMa < minMa) minMa = c.CMa;
      if (c.CMa > maxMa) maxMa = c.CMa;
    }

    const alphaSeq = [];
    for (let i = 0; i < calibList[0].Items.length; i++) {
      alphaSeq.push(calibList[0].Items[i].Alpha);
    }

    this.calib = calibList;
    this.alphaSeq = alphaSeq;
    this.initMa = sumMa / calibList.length;
    this.minMa = minMa;
    this.maxMa = maxMa;
    this.loaded = true;

    const machNumbers = [];
    const files = [];
    for (let i = 0; i < fileData.length; i++) {
      machNumbers.push(calibList[i].CMa);
      files.push({
        filePath: fileData[i].filePath,
        fileName: baseName(fileData[i].filePath),
        machNumber: calibList[i].CMa,
        validRange: {
          AlphaMin: alphaSeq[0],
          AlphaMax: alphaSeq[alphaSeq.length - 1],
          MachMin: minMa,
          MachMax: maxMa,
        },
      });
    }
    return { ok: true, error: null, files, machNumbers, warnings: [] };
  }

  calculate(input) {
    if (!this.loaded) {
      return { result: null, error: '校准数据未加载' };
    }

    const p1 = input.P1;
    const p2 = input.P2;
    const p3 = input.P3;
    const pa = input.PAtm;
    const tatm = input.TAtm;

    const deltaP = 2 * p2 - p1 - p3;
    if (Math.abs(deltaP) < DELTA_P_TOL) {
      return {
        result: {
          alpha: 0,
          machNumber: this.initMa,
          P0: p2,
          Ps: p2,
          iterationCount: 0,
          isValid: false,
          warning: '压力差分ΔP接近零，无法计算',
        },
        error: null,
      };
    }

    const kbTemp = (p3 - p1) / deltaP;
    if (!isFiniteNum(kbTemp)) {
      return {
        result: {
          alpha: 0,
          machNumber: this.initMa,
          P0: p2,
          Ps: p2,
          iterationCount: 0,
          isValid: false,
          warning: 'Kb值为无穷大或非数值',
        },
        error: null,
      };
    }

    let currentMa = this.initMa;
    let iteration = 0;
    let maClamped = false;

    for (iteration = 0; iteration < MAX_ITERATIONS; iteration++) {
      const match = this.interpolateWithWarning(kbTemp, currentMa);
      if (match === null) break;

      const pt = p2 + match.K0 * deltaP;
      const ps = pt - match.Kv * deltaP;
      const newMa = calcMach(pt, ps, pa, tatm);

      if (Math.abs(newMa - currentMa) < CONVERGE_TOL) {
        currentMa = newMa;
        break;
      }

      const clampedMa = Math.max(this.minMa, Math.min(this.maxMa, newMa));
      if (Math.abs(clampedMa - newMa) > 1e-6) maClamped = true;
      currentMa = clampedMa;
    }

    const finalMatch = this.interpolateWithWarning(kbTemp, currentMa);
    if (finalMatch === null) {
      return {
        result: {
          alpha: 0,
          machNumber: currentMa,
          P0: p2,
          Ps: p2,
          iterationCount: iteration,
          isValid: false,
          warning: '最终插值未能返回有效结果',
        },
        error: null,
      };
    }

    const pt = p2 + finalMatch.K0 * deltaP;
    const ps = pt - finalMatch.Kv * deltaP;
    const mach = calcMach(pt, ps, pa, tatm);

    const warnings = [];
    let isValid = true;

    if (mach > this.maxMa + 0.01 || mach < this.minMa - 0.01) {
      warnings.push('计算马赫数超出校准范围');
      isValid = false;
    }
    if (maClamped) {
      warnings.push('迭代过程中马赫数被限制到标定边界，结果精度可能降低');
    }
    if (finalMatch.kbExtrapolated) {
      warnings.push('Kb值超出校准数据范围，已使用最近边界点外推');
    }

    const warning = warnings.join('; ');

    return {
      result: {
        alpha: finalMatch.Alpha,
        machNumber: mach,
        P0: pt,
        Ps: ps,
        iterationCount: iteration + 1,
        isValid,
        warning,
      },
      error: null,
    };
  }

  // 在两张最近马赫校准表之间线性插值出给定 Ma 下的 Kb/K0/Kv/Alpha 序列，
  // 再按 Kb 升序排列，并对 kbMeasured 做分段线性插值。
  // 返回 { Kb, K0, Kv, Alpha, kbExtrapolated } 或 null。
  interpolateWithWarning(kbMeasured, ma) {
    let kbExtrapolated = false;
    if (this.calib.length === 0) return null;

    const sorted = this.calib.slice().sort((a, b) => Math.abs(a.CMa - ma) - Math.abs(b.CMa - ma));
    const calib1 = sorted[0];
    const calib2 = sorted.length > 1 ? sorted[1] : sorted[0];

    const entries = [];
    let ratio = 0.0;
    if (Math.abs(calib2.CMa - calib1.CMa) > 1e-6) {
      ratio = (ma - calib1.CMa) / (calib2.CMa - calib1.CMa);
      if (ratio < 0 || ratio > 1) kbExtrapolated = true;
      ratio = Math.max(0, Math.min(1, ratio));
    }

    for (let i = 0; i < this.alphaSeq.length; i++) {
      const kb = calib1.Items[i].Kb + ratio * (calib2.Items[i].Kb - calib1.Items[i].Kb);
      const k0 = calib1.Items[i].K0 + ratio * (calib2.Items[i].K0 - calib1.Items[i].K0);
      const kv = calib1.Items[i].Kv + ratio * (calib2.Items[i].Kv - calib1.Items[i].Kv);
      entries.push({ Kb: kb, Alpha: this.alphaSeq[i], K0: k0, Kv: kv });
    }

    entries.sort((a, b) => a.Kb - b.Kb);

    if (kbMeasured <= entries[0].Kb) {
      if (kbMeasured < entries[0].Kb) kbExtrapolated = true;
      return {
        Kb: entries[0].Kb, K0: entries[0].K0, Kv: entries[0].Kv, Alpha: entries[0].Alpha, kbExtrapolated,
      };
    }
    if (kbMeasured >= entries[entries.length - 1].Kb) {
      if (kbMeasured > entries[entries.length - 1].Kb) kbExtrapolated = true;
      const last = entries[entries.length - 1];
      return {
        Kb: last.Kb, K0: last.K0, Kv: last.Kv, Alpha: last.Alpha, kbExtrapolated,
      };
    }

    for (let j = 0; j < entries.length - 1; j++) {
      if (kbMeasured >= entries[j].Kb && kbMeasured <= entries[j + 1].Kb) {
        const r = (kbMeasured - entries[j].Kb) / (entries[j + 1].Kb - entries[j].Kb);
        return {
          Kb: kbMeasured,
          K0: entries[j].K0 + r * (entries[j + 1].K0 - entries[j].K0),
          Kv: entries[j].Kv + r * (entries[j + 1].Kv - entries[j].Kv),
          Alpha: entries[j].Alpha + r * (entries[j + 1].Alpha - entries[j].Alpha),
          kbExtrapolated,
        };
      }
    }

    return null;
  }

  // 解析三孔 .prb 文本行：首行 CMa，次行 Nalpha，随后 Nalpha 行 "Kb K0 Kv Alpha"
  parsePrbLines(lines) {
    const nonEmpty = [];
    for (const l of lines) {
      const trimmed = (l || '').trim();
      if (trimmed !== '') nonEmpty.push(trimmed);
    }

    if (nonEmpty.length < 2) return null;

    const cma = Number(nonEmpty[0]);
    if (!isFiniteNum(cma)) return null;
    const nalpha = parseInt(nonEmpty[1], 10);
    if (!isFiniteNum(nalpha) || nalpha <= 0) return null;

    const dataLines = nonEmpty.slice(2);
    if (dataLines.length < nalpha) return null;

    const cal = { CMa: cma, Nalpha: nalpha, Items: [] };
    for (let i = 0; i < nalpha; i++) {
      const parts = dataLines[i].split(/\s+/);
      if (parts.length !== 4) return null;

      const vals = [];
      for (let j = 0; j < 4; j++) {
        const v = Number(parts[j]);
        if (!isFiniteNum(v)) return null;
        vals.push(v);
      }
      cal.Items.push({ Kb: vals[0], K0: vals[1], Kv: vals[2], Alpha: vals[3] });
    }
    return cal;
  }

  getMachRange() {
    return [this.minMa, this.maxMa];
  }
}

module.exports = { ThreeHoleInterpolator, calcMach, calcGamma };
