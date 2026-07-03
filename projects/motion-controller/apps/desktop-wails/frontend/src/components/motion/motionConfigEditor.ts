import type { AxisConfig, AxisEncoderCompensationConfig, AxisName } from '@shared/types/motion'

export const DEFAULT_AXIS_NAMES: AxisName[] = ['X', 'Y', 'Z', 'U']

/** 默认编码器分辨率（工程单位/计数），与 Go core DefaultScale 对齐。 */
export const DEFAULT_ENCODER_SCALE = 0.005

/** 与 shared/device-sdk/go/motion/core DefaultEncoderCompensation* 常量对齐。 */
export function defaultEncComp(): AxisEncoderCompensationConfig {
  return { enabled: false, tolerance: 0.01, maxCycles: 3, settleMs: 100, minStep: 0.001, timeoutMs: 5000 }
}

// ---------- 编码器补偿参数校验 ----------

export interface CompensationWarning {
  field: string
  message: string
  severity: 'error' | 'warning'
}

/** 与 Go 后端 ValidateCompensationConfig 保持逻辑一致。 */
export function validateEncoderCompensation(
  cfg: AxisEncoderCompensationConfig,
  axis: AxisConfig
): CompensationWarning[] {
  if (!cfg.enabled) return []

  const warns: CompensationWarning[] = []
  const scale = axis.encoderScale ?? 0.005
  const ppu = computePulsesPerUnit(axis)

  // tolerance < 编码器分辨率 => 永远无法收敛
  if (cfg.tolerance! < scale) {
    warns.push({
      field: 'tolerance',
      severity: 'error',
      message: `容差(${cfg.tolerance})小于编码器分辨率(${scale})，误差永远无法降到容差以下`,
    })
  }

  // tolerance < 脉冲当量 => 电机走一步即越过容差，补偿会在目标两侧反复振荡。
  // 注意：编码器分辨率粗于脉冲当量是正常情况（电机比编码器精细），
  // 补偿精度受限于两者中较粗者，只要 tolerance ≥ 较粗者即可正常工作。
  if (ppu > 0) {
    const pulseQuantum = 1 / ppu
    if (cfg.tolerance! < pulseQuantum) {
      warns.push({
        field: 'tolerance',
        severity: 'warning',
        message: `容差(${cfg.tolerance})小于脉冲当量(${pulseQuantum.toFixed(6)})，电机走一步即越过容差，补偿可能在目标两侧反复振荡`,
      })
    }
  }

  // minStep → 0 脉冲 => 修正无效果
  if (cfg.minStep! > 0) {
    const minPulse = Math.round(cfg.minStep! * ppu)
    if (minPulse === 0) {
      warns.push({
        field: 'minStep',
        severity: 'warning',
        message: `最小步长(${cfg.minStep})对应脉冲数为 0（脉冲当量 ${ppu.toFixed(2)} 脉冲/单位），一次修正不会产生有效移动`,
      })
    }
  }

  // minStep >= tolerance => 修正过头
  if (cfg.minStep! >= cfg.tolerance!) {
    warns.push({
      field: 'minStep',
      severity: 'warning',
      message: `最小步长(${cfg.minStep}) >= 容差(${cfg.tolerance})，修正可能过头导致在目标两侧反复`,
    })
  }

  // timeout 不够跑完 maxCycles
  if (cfg.maxCycles! > 0 && cfg.settleMs! > 0 && cfg.timeoutMs! > 0) {
    const cycleEstimate = cfg.settleMs! + 50
    const needed = cfg.maxCycles! * cycleEstimate
    if (cfg.timeoutMs! < needed) {
      warns.push({
        field: 'timeoutMs',
        severity: 'warning',
        message: `超时(${cfg.timeoutMs}ms)可能不够跑完 ${cfg.maxCycles} 次循环(估计需 ${needed}ms = ${cfg.maxCycles}×(settle ${cfg.settleMs}ms + 50ms))`,
      })
    }
  }

  return warns
}

export function createDefaultAxis(name: AxisName): AxisConfig {
  return {
    name, enabled: true, kind: name === 'U' ? 'ROTARY' as const : 'LINEAR' as const,
    maxSpeed: 10, stepsPerRev: 1.8,
    microSteps: 4, lead: 4, gearRatio: 1,
    positionSource: 'register' as const, encoderScale: 0.005,
    encoderCompensation: defaultEncComp(),
    minLimit: undefined, maxLimit: undefined,
    inverted: false, encoderInverted: false,
  }
}

export function getAxisThemeClass(axisName: AxisName): string {
  const themeMap: Record<AxisName, string> = {
    X: 'axis-x-theme', Y: 'axis-y-theme', Z: 'axis-z-theme', U: 'axis-u-theme',
  }
  return themeMap[axisName] || ''
}

export function getAxisUnit(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? '°' : 'mm'
}

export function computePulsesPerUnit(axis: AxisConfig): number {
  const stepAngleDeg = (typeof axis.stepsPerRev === 'number' && axis.stepsPerRev > 0) ? axis.stepsPerRev : 1.8
  const microSteps = (typeof axis.microSteps === 'number' && axis.microSteps > 0) ? axis.microSteps : 1
  const stepsPerRev = 360 / stepAngleDeg
  if (axis.kind === 'ROTARY') {
    const gearRatio = (typeof axis.gearRatio === 'number' && axis.gearRatio > 0) ? axis.gearRatio : 1
    return (stepsPerRev * microSteps * gearRatio) / 360
  }
  const lead = (typeof axis.lead === 'number' && axis.lead !== 0) ? axis.lead : 1
  return (stepsPerRev * microSteps) / lead
}

export function getAxisInfoLabel(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? '步/度:' : '步/mm:'
}

/**
 * 归一化正数参数：非有限值或非正值回退到 fallback。
 * 运动控制参数默认不允许零值，避免危险的硬件行为。
 */
export function normalizePositive(value: number | undefined, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : fallback
}
