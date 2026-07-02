import type { AxisConfig, AxisEncoderCompensationConfig, AxisName, PositionSource } from '@shared/types/motion'

export const DEFAULT_AXIS_NAMES: AxisName[] = ['X', 'Y', 'Z', 'U']

// 轴机械/电气参数默认值
export const DEFAULT_STEPS_PER_REV = 1.8
export const DEFAULT_MICRO_STEPS = 4
export const DEFAULT_LEAD = 4
export const DEFAULT_GEAR_RATIO = 1
export const DEFAULT_MAX_SPEED = 100
export const DEFAULT_ENCODER_SCALE = 0.005

// 编码器补偿默认值
// 与 shared.local/device-sdk/go/motion/core DefaultEncoderCompensation* 常量对齐。
export const DEFAULT_ENCODER_COMPENSATION_TOLERANCE = 0.01
export const DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES = 3
export const DEFAULT_ENCODER_COMPENSATION_SETTLE_MS = 100
export const DEFAULT_ENCODER_COMPENSATION_MIN_STEP = 0.001
export const DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS = 5000

/**
 * 创建编码器补偿默认配置
 */
export function createDefaultEncoderCompensation() {
  return {
    enabled: false,
    tolerance: DEFAULT_ENCODER_COMPENSATION_TOLERANCE,
    maxCycles: DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES,
    settleMs: DEFAULT_ENCODER_COMPENSATION_SETTLE_MS,
    minStep: DEFAULT_ENCODER_COMPENSATION_MIN_STEP,
    timeoutMs: DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS,
  }
}

/**
 * 创建默认轴配置
 *
 * U 轴默认为旋转轴，其余为直线轴。
 */
export function createDefaultAxis(name: AxisName): AxisConfig {
  return {
    name,
    enabled: true,
    kind: name === 'U' ? 'ROTARY' : 'LINEAR',
    maxSpeed: DEFAULT_MAX_SPEED,
    minLimit: undefined,
    maxLimit: undefined,
    inverted: false,
    encoderInverted: false,
    stepsPerRev: DEFAULT_STEPS_PER_REV,
    microSteps: DEFAULT_MICRO_STEPS,
    lead: DEFAULT_LEAD,
    gearRatio: DEFAULT_GEAR_RATIO,
    positionSource: 'register',
    encoderScale: DEFAULT_ENCODER_SCALE,
    encoderCompensation: createDefaultEncoderCompensation(),
  }
}

/**
 * 将已有轴配置补齐为完整配置，兼容旧数据
 */
export function normalizeAxisForEditing(axis: AxisConfig): AxisConfig {
  return {
    ...axis,
    enabled: true,
    kind: axis.kind ?? (axis.name === 'U' ? 'ROTARY' : 'LINEAR'),
    maxSpeed: axis.maxSpeed ?? DEFAULT_MAX_SPEED,
    inverted: axis.inverted ?? false,
    encoderInverted: axis.encoderInverted ?? axis.inverted ?? false,
    stepsPerRev: axis.stepsPerRev ?? DEFAULT_STEPS_PER_REV,
    microSteps: axis.microSteps ?? DEFAULT_MICRO_STEPS,
    lead: axis.lead ?? DEFAULT_LEAD,
    gearRatio: axis.gearRatio ?? DEFAULT_GEAR_RATIO,
    positionSource: (axis.positionSource ?? 'register') as PositionSource,
    encoderScale: axis.encoderScale ?? DEFAULT_ENCODER_SCALE,
    encoderCompensation: {
      ...createDefaultEncoderCompensation(),
      ...(axis.encoderCompensation ?? {}),
    },
  }
}

export function getAxisThemeClass(axisName: AxisName): string {
  const themeMap: Record<AxisName, string> = {
    X: 'axis-x-theme',
    Y: 'axis-y-theme',
    Z: 'axis-z-theme',
    U: 'axis-u-theme',
  }
  return themeMap[axisName] || ''
}

export function getAxisUnit(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? 'deg' : 'mm'
}

export function getAxisInfoLabel(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? '步/度:' : '步/mm:'
}

// ---------- 编码器补偿参数校验 ----------

export interface CompensationWarning {
  field: string
  message: string
  severity: 'error' | 'warning'
}

export function validateEncoderCompensation(
  cfg: AxisEncoderCompensationConfig,
  axis: AxisConfig
): CompensationWarning[] {
  if (!cfg.enabled) return []

  const warns: CompensationWarning[] = []
  const scale = axis.encoderScale ?? DEFAULT_ENCODER_SCALE

  // tolerance < 编码器分辨率 → 永远无法收敛
  if (cfg.tolerance! < scale) {
    warns.push({
      field: 'tolerance',
      severity: 'error',
      message: `容差(${cfg.tolerance})小于编码器分辨率(${scale})，误差无法降到容差以下`,
    })
  }

  // 编码器分辨率粗于脉冲当量 → 编码器无法分辨最小步进，补偿盲区大、精度受限
  const ppu = computePulsesPerUnit(axis)
  if (ppu > 0) {
    const pulseQuantum = 1 / ppu
    if (scale > pulseQuantum) {
      warns.push({
        field: 'encoderScale',
        severity: 'warning',
        message: `编码器分辨率(${scale})粗于脉冲当量(${pulseQuantum.toFixed(6)})，电机最小步进无法被编码器分辨，补偿精度受限`,
      })
    }
  }

  // minStep → 0 脉冲 → 修正无效果
  if (cfg.minStep! > 0) {
    const ppu = computePulsesPerUnit(axis)
    const minPulse = Math.round(cfg.minStep! * ppu)
    if (minPulse === 0) {
      warns.push({
        field: 'minStep',
        severity: 'warning',
        message: `最小步长(${cfg.minStep})对应脉冲数为 0（脉冲当量 ${ppu.toFixed(2)}），修正不会产生有效移动`,
      })
    }
  }

  // minStep >= tolerance → 振荡风险
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
        message: `超时(${cfg.timeoutMs}ms)可能不够 ${cfg.maxCycles} 次循环(约需 ${needed}ms)`,
      })
    }
  }

  return warns
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

/**
 * 安全解析数字输入
 *
 * 过滤 NaN 和非正值，返回 fallback。运动控制参数默认不允许零值，
 * 避免危险的硬件行为。
 */
export function parseNumericInput(value: string, fallback: number, allowZero = false): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return fallback
  if (parsed < 0) return fallback
  if (!allowZero && parsed === 0) return fallback
  return parsed
}

/**
 * 归一化正数参数
 */
export function normalizePositive(value: number | undefined, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : fallback
}
