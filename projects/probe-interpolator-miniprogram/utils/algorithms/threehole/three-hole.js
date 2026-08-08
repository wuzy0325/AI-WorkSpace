// three-hole.js —— 端口自 shared/algorithms/go/threehole/interpolation/three_hole.go
// ThreeHoleInterpolator：三孔探针迭代插值。与桌面端 probe-interpolator 的 3 孔入口一致：
//   new ThreeHoleInterpolator() -> loadPrbData(files) -> calculate(input)
//
// 关键差异（相对五孔 PRB 链路）：
//   - 文件首行内嵌 CMa（校准马赫数），第二行 Nalpha，随后 Nalpha 行 "Kb K0 Kv Alpha"。
//   - 单个 ThreeHoleInterpolator 即可加载多个 .prb（不同 Ma），calculate 内部按实时 Ma
//     自动挑选/线性插值两张最近马赫表的 Kb/K0/Kv/Alpha。
//   - 输入仅 3 个孔压（P1/P2/P3）+ 大气参数；输出含 MachNumber 及由其推导的 Velocity，
//     不含 Beta/CAS/SAT 等派生量（三孔无 Beta）。
//
// 同步自桌面端 0.2.1（docs/three-hole-interpolation-improvements.md）：
//   - A1：输入有限性校验（NaN/Inf），返回结构化无效结果而非 error，避免批量接口整体失败。
//   - A2：calcMach 对齐七孔 calVelocityMach 全部前置条件，不再用 Abs 掩盖非物理状态。
//   - A3：超范围警告携带诊断数值（恢复Ma、校准范围）。
//   - A4：多文件加载时校验各档 Alpha 网格一致、Kb 严格单调无重复。
//   - B1：单 PRB 文件快速路径，跳过空转迭代与每帧排序，IterationCount=1。
//   - B2：加载时预计算 Kb 排序表，单文件热路径免去每帧排序/分配。
//   - C2：单文件快速路径不再发 maClamped 警告。
//   - E：多文件 interpolateWithWarning 移除运行时排序，改用二分查找
//     （docs/three-hole-algorithm-improvements.md §6；凸组合保持 Kb 严格递增）。
//   - 结果对象新增 Calculated 字段，区分"是否计算过"与"是否有效"。

const MAX_ITERATIONS = 20;
const CONVERGE_TOL = 1e-4;
const DELTA_P_TOL = 1e-6;
// alphaGridEps：多文件加载时 Alpha 网格一致性校验的容差（deg）。
const ALPHA_GRID_EPS = 1e-9;

// 空气比热比温度修正系数（-40°C ~ +60°C 范围内误差 <0.1%）
const GAMMA_REF = 1.4; // 20°C 时的参考比热比（γ）
const TEMP_REF = 20.0; // 参考温度(°C)
const TEMP_COEFF = 0.0002; // 温度修正系数 γ ≈ gammaRef - tempCoeff*(T-tempRef)

// AIR_GAS_CONSTANT 干空气气体常数 R = 287 J/(kg·K)
// 用于由马赫数反算气流速度: V = Ma · sqrt(γ · R · T_K)
const AIR_GAS_CONSTANT = 287.0;

function isFiniteNum(f) {
  return typeof f === 'number' && isFinite(f);
}

function baseName(p) {
  if (!p) return '';
  const parts = String(p).split(/[\\/]/);
  return parts[parts.length - 1];
}

// validateFiniteInput 校验输入字段的有限性（NaN/Inf）。
// 改善项 A1：P1/P2/P3 为 NaN/Inf 时会被 deltaP/Kb 校验拦截，但 Patm=NaN 会一路
// 穿透到 calcMach 产生 NaN 马赫数，而 NaN 与校准范围比较恒为 false，导致
// isValid=true 且 MachNumber=NaN 的静默放行。此处统一在入口拦截。
//
// 错误契约：返回结构化无效结果而非 error——批量接口中 error 会使整批 Success=false，
// 而目标是"个别坏行不使批次失败"。
function validateFiniteInput(input) {
  const fields = [
    { name: 'P1', v: input.P1 },
    { name: 'P2', v: input.P2 },
    { name: 'P3', v: input.P3 },
    { name: 'Patm', v: input.PAtm },
    { name: 'Tatm', v: input.TAtm },
  ];
  for (const f of fields) {
    if (isNaN(f.v) || !isFinite(f.v)) {
      return new Error('输入字段 ' + f.name + ' 为非有限数值: ' + f.v);
    }
  }
  return null;
}

// 计算马赫数：由总压/静压（表压）+ 大气压换算绝对压。
// 改善项 A2：对齐七孔 calVelocityMach 的全部前置条件，任一不满足返回 error
// （由 Calculate 转成 isValid=false + 警告），不再 return 0、不再用 Math.Abs 掩盖非物理状态：
//   1. patm 有限且 > 0；
//   2. tatm+273.15 有限且 > 0（即 tatm > -273.15℃）；
//   3. pt >= ps；
//   4. ps+patm > 0；
//   5. ratio = (pt+patm)/(ps+patm) >= 1；
//   6. 最终 Ma 有限（maSq 非负且有限）。
// 返回 { value: number, error: string|null }。
function calcMach(pt, ps, pa, tatm) {
  if (isNaN(pa) || !isFinite(pa) || pa <= 0) {
    return { value: 0, error: '大气压力非法: pa=' + pa };
  }
  const tempK = tatm + 273.15;
  if (isNaN(tempK) || !isFinite(tempK) || tempK <= 0) {
    return { value: 0, error: '大气温度非法: t=' + tatm + ' degC' };
  }
  if (pt < ps) {
    return { value: 0, error: '总压低于静压 (pt < ps): pt=' + pt + ', ps=' + ps };
  }
  const psAbs = ps + pa;
  if (psAbs <= 0) {
    return { value: 0, error: '绝对静压非正: ps+pa=' + psAbs + ' (ps=' + ps + ', pa=' + pa + ')' };
  }
  const ratio = (pt + pa) / psAbs;
  if (ratio < 1) {
    return { value: 0, error: '压力比 ' + ratio + ' < 1 (pt=' + pt + ', ps=' + ps + ', pa=' + pa + ')' };
  }

  const gamma = calcGamma(tatm);
  if (isNaN(gamma) || !isFinite(gamma) || gamma <= 1) {
    return { value: 0, error: '比热比非法: gamma=' + gamma };
  }
  const exp = (gamma - 1) / gamma;
  const coeff = 2 / (gamma - 1);

  const powered = Math.pow(ratio, exp);
  const maSq = coeff * (powered - 1);
  if (isNaN(maSq) || !isFinite(maSq) || maSq < 0) {
    return { value: 0, error: '马赫数根号内非有限或为负: ' + maSq };
  }
  return { value: Math.sqrt(maSq), error: null };
}

// 空气比热比随温度近似变化 γ ≈ gammaRef - tempCoeff*(T-tempRef)
function calcGamma(tatm) {
  if (isNaN(tatm) || !isFinite(tatm)) return GAMMA_REF;
  return GAMMA_REF - TEMP_COEFF * (tatm - TEMP_REF);
}

// calcVelocity 由马赫数与大气温度计算气流速度（m/s）。
//
// 公式: V = Ma · sqrt(γ · R · T_K)
//   - γ 复用 calcGamma 的温度修正比热比，与 calcMach 内部一致
//   - R = AIR_GAS_CONSTANT (287 J/(kg·K))
//   - T_K = tatm + 273.15
//
// 任一输入非法（Ma 非有限或负、T_K 非正、γ 非法）时返回 0，
// 与 MachNumber 的兜底语义对齐：Ma 有效（含 initMa/currentMa 兜底）则给出对应速度，
// Ma 为 0/NaN（如输入非法、calcMach 失败）时返回 0。
function calcVelocity(ma, tatm) {
  if (isNaN(ma) || !isFinite(ma) || ma < 0) return 0;
  const tempK = tatm + 273.15;
  if (isNaN(tempK) || !isFinite(tempK) || tempK <= 0) return 0;
  const gamma = calcGamma(tatm);
  if (isNaN(gamma) || !isFinite(gamma) || gamma <= 1) return 0;
  return ma * Math.sqrt(gamma * AIR_GAS_CONSTANT * tempK);
}

class ThreeHoleInterpolator {
  constructor() {
    this.loaded = false;
    this.calib = []; // [{ CMa, Nalpha, Items: [{Kb,K0,Kv,Alpha}], kbSorted: [...] }]
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
    let firstCal = null;

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

      // A4：各档 Alpha 网格必须与首档完全一致（含顺序）。
      // 否则 interpolateWithWarning 按下标混合各 Ma 档时会静默错配角度。
      if (firstCal === null) {
        firstCal = cal;
      } else {
        for (let i = 0; i < cal.Items.length; i++) {
          if (Math.abs(cal.Items[i].Alpha - firstCal.Items[i].Alpha) > ALPHA_GRID_EPS) {
            return {
              ok: false,
              error: '文件 ' + baseName(fd.filePath) + ' 的Alpha网格与其他文件不一致(第' + (i + 1) +
                '点: ' + cal.Items[i].Alpha + ' vs ' + firstCal.Items[i].Alpha + ')',
            };
          }
        }
      }

      // A4：每档 Kb 必须按行序严格单调递增且无重复。
      // 否则区间插值 r 的分母可能为零（K0/Kv/Alpha 变 NaN/Inf）或角度错配。
      for (let i = 1; i < cal.Items.length; i++) {
        if (cal.Items[i].Kb <= cal.Items[i - 1].Kb) {
          return {
            ok: false,
            error: '文件 ' + baseName(fd.filePath) + ' 的Kb非严格单调(第' + (i + 1) + '点 ' +
              cal.Items[i].Kb + ' <= 第' + i + '点 ' + cal.Items[i - 1].Kb + ')',
          };
        }
      }

      // B2：预计算按 Kb 排序的插值表，单文件快速路径免去每帧排序/分配。
      cal.kbSorted = cal.Items.slice().sort((a, b) => a.Kb - b.Kb);

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

    // A1：输入有限性校验（契约：结构化无效结果，不返回 error）。
    const validateErr = validateFiniteInput(input);
    if (validateErr) {
      return {
        result: {
          alpha: 0,
          machNumber: 0,
          velocity: 0,
          P0: 0,
          Ps: 0,
          calculated: false,
          isValid: false,
          warning: validateErr.message,
        },
        error: null,
      };
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
          machNumber: 0,
          velocity: 0,
          P0: p2,
          Ps: p2,
          calculated: false,
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
          velocity: calcVelocity(this.initMa, tatm),
          P0: p2,
          Ps: p2,
          calculated: false,
          isValid: false,
          warning: 'Kb值为无穷大或非数值',
        },
        error: null,
      };
    }

    // B1：单文件快速路径。len(calib)==1 时 calib1==calib2、ratio=0，
    // 插值结果与 Ma 无关，迭代循环与每帧排序均为空转；直接单次插值，
    // 且无 maClamped 警告（C2）。
    if (this.calib.length === 1) {
      return { result: this.finalizeSingle(kbTemp, p2, deltaP, pa, tatm), error: null };
    }

    return { result: this.calculateMulti(kbTemp, p2, deltaP, pa, tatm), error: null };
  }

  // finalizeSingle 单 PRB 文件快速路径：按预排序 Kb 表插值一次并组装结果。
  finalizeSingle(kbTemp, p2, deltaP, pa, tatm) {
    const { match, kbExtrapolated } = this.interpolateSingle(kbTemp);
    if (match === null) {
      return {
        alpha: 0,
        machNumber: this.initMa,
        velocity: calcVelocity(this.initMa, tatm),
        P0: p2,
        Ps: p2,
        calculated: false,
        isValid: false,
        warning: '最终插值未能返回有效结果',
      };
    }

    const pt = p2 + match.K0 * deltaP;
    const ps = pt - match.Kv * deltaP;
    const machRes = calcMach(pt, ps, pa, tatm);
    if (machRes.error) {
      return {
        alpha: 0,
        machNumber: 0,
        velocity: 0,
        P0: pt,
        Ps: ps,
        calculated: false,
        isValid: false,
        warning: machRes.error,
      };
    }
    const mach = machRes.value;

    const warnings = [];
    let isValid = true;

    // A3：超范围警告携带实际值与校准范围，便于定位。
    if (mach > this.maxMa + 0.01 || mach < this.minMa - 0.01) {
      warnings.push('计算马赫数超出校准范围: 恢复Ma=' + mach.toFixed(3) +
        '，校准范围[' + this.minMa.toFixed(3) + ', ' + this.maxMa.toFixed(3) + ']');
      isValid = false;
    }
    if (kbExtrapolated) {
      warnings.push('Kb值超出校准数据范围，已使用最近边界点外推');
    }

    return {
      alpha: match.Alpha,
      machNumber: mach,
      velocity: calcVelocity(mach, tatm),
      P0: pt,
      Ps: ps,
      calculated: true,
      isValid,
      warning: warnings.join('; '),
    };
  }

  // calculateMulti 多 PRB 文件路径：保留原迭代收敛行为（B1 只优化单文件场景）。
  calculateMulti(kbTemp, p2, deltaP, pa, tatm) {
    let currentMa = this.initMa;
    let iteration = 0;
    let maClamped = false;

    for (iteration = 0; iteration < MAX_ITERATIONS; iteration++) {
      const { match } = this.interpolateWithWarning(kbTemp, currentMa);
      if (match === null) break;

      const pt = p2 + match.K0 * deltaP;
      const ps = pt - match.Kv * deltaP;
      const newMaRes = calcMach(pt, ps, pa, tatm);
      if (newMaRes.error) {
        return {
          alpha: 0,
          machNumber: 0,
          velocity: 0,
          P0: pt,
          Ps: ps,
          calculated: false,
          isValid: false,
          warning: newMaRes.error,
        };
      }
      const newMa = newMaRes.value;

      if (Math.abs(newMa - currentMa) < CONVERGE_TOL) {
        currentMa = newMa;
        break;
      }

      const clampedMa = Math.max(this.minMa, Math.min(this.maxMa, newMa));
      if (Math.abs(clampedMa - newMa) > 1e-6) maClamped = true;
      currentMa = clampedMa;
    }

    const { match: finalMatch, kbExtrapolated } = this.interpolateWithWarning(kbTemp, currentMa);
    if (finalMatch === null) {
      return {
        alpha: 0,
        machNumber: currentMa,
        velocity: calcVelocity(currentMa, tatm),
        P0: p2,
        Ps: p2,
        calculated: false,
        isValid: false,
        warning: '最终插值未能返回有效结果',
      };
    }

    const pt = p2 + finalMatch.K0 * deltaP;
    const ps = pt - finalMatch.Kv * deltaP;
    const machRes = calcMach(pt, ps, pa, tatm);
    if (machRes.error) {
      return {
        alpha: 0,
        machNumber: 0,
        velocity: 0,
        P0: pt,
        Ps: ps,
        calculated: false,
        isValid: false,
        warning: machRes.error,
      };
    }
    const mach = machRes.value;

    const warnings = [];
    let isValid = true;

    // A3：超范围警告携带实际值与校准范围，便于定位。
    if (mach > this.maxMa + 0.01 || mach < this.minMa - 0.01) {
      warnings.push('计算马赫数超出校准范围: 恢复Ma=' + mach.toFixed(3) +
        '，校准范围[' + this.minMa.toFixed(3) + ', ' + this.maxMa.toFixed(3) + ']');
      isValid = false;
    }
    if (maClamped) {
      warnings.push('迭代过程中马赫数被限制到标定边界，结果精度可能降低');
    }
    if (kbExtrapolated) {
      warnings.push('Kb值超出校准数据范围，已使用最近边界点外推');
    }

    return {
      alpha: finalMatch.Alpha,
      machNumber: mach,
      velocity: calcVelocity(mach, tatm),
      P0: pt,
      Ps: ps,
      calculated: true,
      isValid,
      warning: warnings.join('; '),
    };
  }

  // interpolateSingle 单文件插值（B2）：使用加载时预计算的 Kb 排序表直接查表，
  // 无分配、无每帧排序。行为与 interpolateWithWarning 在 len(calib)==1 时一致。
  // 返回 { match: {Kb,K0,Kv,Alpha}|null, kbExtrapolated: boolean }。
  interpolateSingle(kbMeasured) {
    const entries = this.calib[0].kbSorted;
    if (!entries || entries.length === 0) {
      return { match: null, kbExtrapolated: false };
    }
    let kbExtrapolated = false;

    if (kbMeasured <= entries[0].Kb) {
      if (kbMeasured < entries[0].Kb) kbExtrapolated = true;
      return { match: entries[0], kbExtrapolated };
    }
    if (kbMeasured >= entries[entries.length - 1].Kb) {
      if (kbMeasured > entries[entries.length - 1].Kb) kbExtrapolated = true;
      return { match: entries[entries.length - 1], kbExtrapolated };
    }

    for (let j = 0; j < entries.length - 1; j++) {
      if (kbMeasured >= entries[j].Kb && kbMeasured <= entries[j + 1].Kb) {
        const r = (kbMeasured - entries[j].Kb) / (entries[j + 1].Kb - entries[j].Kb);
        return {
          match: {
            Kb: kbMeasured,
            K0: entries[j].K0 + r * (entries[j + 1].K0 - entries[j].K0),
            Kv: entries[j].Kv + r * (entries[j + 1].Kv - entries[j].Kv),
            Alpha: entries[j].Alpha + r * (entries[j + 1].Alpha - entries[j].Alpha),
          },
          kbExtrapolated,
        };
      }
    }

    return { match: null, kbExtrapolated };
  }

  // interpolateWithWarning 按当前 Ma 在 Kb 表上插值，返回插值项与外推标志。
  // 多文件路径使用：从 calib 中线性扫描最近两个 Ma 档（等价于原 sort 取前二，
  // 且交换 calib1/calib2 时 ratio→1-ratio 得到相同的插值混合，结果不变）。
  // 单文件场景由 calculate 走 interpolateSingle 快速路径，不进入本函数。
  // 返回 { match: {Kb,K0,Kv,Alpha}|null, kbExtrapolated: boolean }。
  interpolateWithWarning(kbMeasured, ma) {
    if (this.calib.length === 0) return { match: null, kbExtrapolated: false };

    let calib1 = null;
    let calib2 = null;
    let d1 = Infinity;
    let d2 = Infinity;
    for (const c of this.calib) {
      const d = Math.abs(c.CMa - ma);
      if (d < d1) {
        calib2 = calib1;
        d2 = d1;
        calib1 = c;
        d1 = d;
      } else if (d < d2) {
        calib2 = c;
        d2 = d;
      }
    }
    if (calib2 === null) calib2 = calib1;

    const entries = [];
    let kbExtrapolated = false;
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

    // 改善项 E：混合后的 Kb 序列是两档严格递增序列的凸组合，在实数数学下仍
    // 严格递增（docs/three-hole-algorithm-improvements.md §6.1）。但 IEEE-754
    // 舍入在相邻值仅差 1 ULP、或两档数量级差异悬殊时，混合结果可能被舍入成
    // 相同值或逆序，使"区间存在且唯一"的前提失效。加载器允许任意有限浮点值，
    // 故此处对实际混合结果做防御性校验：一旦发现非严格递增即返回 match=null
    // （由调用方转为"最终插值未能返回有效结果"的明确失败），不继续二分，
    // 避免在非单调区间上产生未定义行为。Go 与 JS 采用相同策略。
    for (let i = 1; i < entries.length; i++) {
      if (entries[i].Kb <= entries[i - 1].Kb) {
        return { match: null, kbExtrapolated };
      }
    }

    // 二分定位第一个满足 entries[j+1].Kb >= kbMeasured 的左区间，与既有线性
    // 扫描的区间语义一致；内部节点沿用原插值表达式，保持浮点运算路径不变。
    if (kbMeasured <= entries[0].Kb) {
      if (kbMeasured < entries[0].Kb) kbExtrapolated = true;
      return {
        match: { Kb: entries[0].Kb, K0: entries[0].K0, Kv: entries[0].Kv, Alpha: entries[0].Alpha },
        kbExtrapolated,
      };
    }
    if (kbMeasured >= entries[entries.length - 1].Kb) {
      if (kbMeasured > entries[entries.length - 1].Kb) kbExtrapolated = true;
      const last = entries[entries.length - 1];
      return { match: { Kb: last.Kb, K0: last.K0, Kv: last.Kv, Alpha: last.Alpha }, kbExtrapolated };
    }

    // 二分：entries 严格递增，寻找第一个使 entries[j+1].Kb >= kbMeasured 的 j。
    // 由 §6.1 凸组合单调性保证存在且唯一；lo/hi 边界处理与线性扫描语义一致。
    let lo = 0;
    let hi = entries.length - 2;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (entries[mid + 1].Kb < kbMeasured) {
        lo = mid + 1;
      } else {
        hi = mid;
      }
    }
    const j = lo;
    const r = (kbMeasured - entries[j].Kb) / (entries[j + 1].Kb - entries[j].Kb);
    return {
      match: {
        Kb: kbMeasured,
        K0: entries[j].K0 + r * (entries[j + 1].K0 - entries[j].K0),
        Kv: entries[j].Kv + r * (entries[j + 1].Kv - entries[j].Kv),
        Alpha: entries[j].Alpha + r * (entries[j + 1].Alpha - entries[j].Alpha),
      },
      kbExtrapolated,
    };
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

module.exports = { ThreeHoleInterpolator, calcMach, calcGamma, calcVelocity, validateFiniteInput };
