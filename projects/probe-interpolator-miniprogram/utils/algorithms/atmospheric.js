// AtmosphericDataCalculator —— 端口自 shared/algorithms/go/fivehole/interpolation/atmospheric_data.go
// 飞行大气数据速度计算：基于总压/静压/总温求马赫数、CAS、SAT、TAS 等。
// 注意：常量与 Go 版保持一致（atmR=287.05 用于 TASByDensity；prb 模块另用 287.06 算密度）。

const ATM_R = 287.05;      // 气体常数 J/(kg·K)
const ATM_P0 = 101325;    // 标准海平面气压 Pa
const ATM_T0 = 288.15;    // 标准海平面温度 K
const ATM_RHO0 = 1.225;   // 标准海平面空气密度 kg/m³
const ATM_GAMMA = 1.4;    // 空气绝热指数
const ATM_C_COEFF = 20.047; // 声速计算系数 C=20.047
const ATM_RECOVERY = 0.9;   // 温度传感器恢复系数（默认）

function isFiniteNum(f) {
  return typeof f === 'number' && isFinite(f);
}

class AtmosphericDataCalculator {
  // 马赫数（亚音速公式）：Ma = sqrt( (2/(γ-1)) * ((Pt/Ps)^((γ-1)/γ) - 1) )
  calculateMach(Pt, Ps) {
    if (Ps <= 0) throw new Error('静压Ps必须大于0');
    if (Pt <= Ps) throw new Error('总压Pt必须大于静压Ps');
    const ratio = Pt / Ps;
    const ma = Math.sqrt((2 / (ATM_GAMMA - 1)) * (Math.pow(ratio, (ATM_GAMMA - 1) / ATM_GAMMA) - 1));
    return ma;
  }

  // 静温 SAT = TAT / (1 + 0.2 * r * Ma^2)
  calculateSAT(TAT, Ma, r) {
    const recovery = (r === undefined || r === null) ? ATM_RECOVERY : r;
    const denominator = 1 + ((ATM_GAMMA - 1) / 2) * recovery * Math.pow(Ma, 2);
    return TAT / denominator;
  }

  // 动压 Qc = Pt - Ps
  calculateQc(Pt, Ps) {
    return Pt - Ps;
  }

  // 校正空速 CAS = a0 * sqrt( (2/(γ-1)) * ((1+Qc/P0)^((γ-1)/γ) - 1) )
  calculateCAS(Qc) {
    const a0 = Math.sqrt(ATM_GAMMA * ATM_P0 / ATM_RHO0);
    const innerTerm = Math.pow(1 + Qc / ATM_P0, (ATM_GAMMA - 1) / ATM_GAMMA) - 1;
    return a0 * Math.sqrt((2 / (ATM_GAMMA - 1)) * innerTerm);
  }

  // 真空速（气压静温法）：TAS = CAS * sqrt(ρ0 / ρs), ρs = Ps / (R * SAT)
  calculateTASByDensity(Ps, Qc, SAT) {
    const CAS = this.calculateCAS(Qc);
    const rhoS = Ps / (ATM_R * SAT);
    return CAS * Math.sqrt(ATM_RHO0 / rhoS);
  }

  // 真空速（声速马赫数法）：TAS = Ma * C, C = 20.047 * sqrt(SAT)
  calculateTASByMach(Ma, SAT) {
    const C = ATM_C_COEFF * Math.sqrt(SAT);
    return Ma * C;
  }

  // 完整大气数据
  calculateAll(Pt, Ps, TAT, r) {
    const ma = this.calculateMach(Pt, Ps);
    const sat = this.calculateSAT(TAT, ma, r);
    const qc = this.calculateQc(Pt, Ps);
    const cas = this.calculateCAS(qc);
    const tasDensity = this.calculateTASByDensity(Ps, qc, sat);
    const tasMach = this.calculateTASByMach(ma, sat);
    return {
      machNumber: ma,
      SAT: sat,
      qc: qc,
      cas: cas,
      tasDensity: tasDensity,
      tasMach: tasMach,
    };
  }
}

module.exports = {
  AtmosphericDataCalculator,
  ATM_R, ATM_P0, ATM_T0, ATM_RHO0, ATM_GAMMA, ATM_C_COEFF, ATM_RECOVERY,
  isFiniteNum,
};
