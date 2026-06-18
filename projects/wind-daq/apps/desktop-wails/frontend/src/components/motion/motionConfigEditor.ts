import type { AxisConfig, AxisName, PositionSource } from '@shared/types/motion'

export const DEFAULT_AXIS_NAMES: AxisName[] = ['X', 'Y', 'Z', 'U']

// 轴机械/电气参数默认值
export const DEFAULT_STEPS_PER_REV = 1.8
export const DEFAULT_MICRO_STEPS = 4
export const DEFAULT_LEAD = 4
export const DEFAULT_GEAR_RATIO = 1
export const DEFAULT_MAX_SPEED = 100
export const DEFAULT_ENCODER_SCALE = 1

// 编码器补偿默认值
export const DEFAULT_ENCODER_COMPENSATION_TOLERANCE = 0.01
export const DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES = 10
export const DEFAULT_ENCODER_COMPENSATION_SETTLE_MS = 200
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
    enabled: axis.enabled ?? true,
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
