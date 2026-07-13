// 运动控制配置工具函数共享模块
//
// 被 motion-controller 和 wind-daq 两个项目共用，消除两份 motionConfigEditor.ts 副本的 drift。
// 设计原则：
//   - 纯工具函数，不依赖 Vue 运行时、Pinia store 或项目级 i18n
//   - 类型定义采用可选字段（与 motion-controller 的 @shared/types/motion 一致），
//     wind-daq 的必填字段类型可结构兼容赋值给可选字段
//   - 项目级差异（如 maxSpeed 默认值）通过参数化处理，各项目在本地包装

// ---------- 类型定义 ----------
// 与两个项目的 @shared/types/motion 结构保持一致，字段可选以兼容必填/可选两种类型。

export type AxisName = 'X' | 'Y' | 'Z' | 'U'
export type AxisKind = 'LINEAR' | 'ROTARY'
export type PositionSource = 'register' | 'encoder'

export interface AxisEncoderCompensationConfig {
  enabled?: boolean
  tolerance?: number
  maxCycles?: number
  settleMs?: number
  minStep?: number
  timeoutMs?: number
}

export interface AxisConfig {
  name: AxisName
  enabled: boolean
  kind: AxisKind
  stepsPerRev?: number
  microSteps?: number
  lead?: number
  gearRatio?: number
  maxSpeed?: number
  minLimit?: number
  maxLimit?: number
  inverted?: boolean
  encoderInverted?: boolean
  positionSource?: PositionSource
  encoderScale?: number
  encoderCompensation?: AxisEncoderCompensationConfig
}

// ---------- 常量 ----------

export const DEFAULT_AXIS_NAMES: AxisName[] = ['X', 'Y', 'Z', 'U']

// 轴机械/电气参数默认值
export const DEFAULT_STEPS_PER_REV = 1.8
export const DEFAULT_MICRO_STEPS = 4
export const DEFAULT_LEAD = 4
export const DEFAULT_GEAR_RATIO = 1
/** 默认最大速度，wind-daq 主项目值；motion-controller 可在本地包装中覆盖为 10（测试工具低速）。 */
export const DEFAULT_MAX_SPEED = 100

/** 硬件运动控制器（MC4A、B140）新建设备时的默认最大速度，低速更安全。 */
export const HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED = 4

/**
 * 根据控制器类型返回默认最大速度。
 * B140-MC / WTNMC4A-MC 返回硬件低速默认值（4），其余（模拟控制器）返回项目既定默认值。
 * 使 MC4A / B140 新建设备默认速度为 4，同时不影响模拟控制器既有的默认值。
 */
export function defaultMaxSpeedForType(type: string | undefined, simulatedDefault: number = DEFAULT_MAX_SPEED): number {
  if (type === 'B140-MC' || type === 'WTNMC4A-MC') return HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED
  return simulatedDefault
}

/** 默认编码器分辨率（工程单位/计数），与 Go core DefaultScale 对齐。 */
export const DEFAULT_ENCODER_SCALE = 0.005

/** 默认点动步长（工程单位），MotionControlPanel 中 ensureAxisLocalState 使用。 */
export const DEFAULT_JOG_STEP = 1

// 编码器补偿默认值
// 与 shared/device-sdk/go/motion/core DefaultEncoderCompensation* 常量对齐。
export const DEFAULT_ENCODER_COMPENSATION_TOLERANCE = 0.01
export const DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES = 3
export const DEFAULT_ENCODER_COMPENSATION_SETTLE_MS = 100
export const DEFAULT_ENCODER_COMPENSATION_MIN_STEP = 0.001
export const DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS = 5000

// ---------- 默认配置工厂 ----------

/**
 * 编码器补偿配置的必填版本。
 *
 * 共享模块的 AxisEncoderCompensationConfig 字段可选（兼容 motion-controller 的类型），
 * 但 createDefaultEncoderCompensation / createDefaultAxis / normalizeAxisForEditing
 * 返回的对象所有字段都有值，用 Required 表示必填，确保与 wind-daq 的必填类型兼容。
 */
type RequiredEncoderCompensation = Required<AxisEncoderCompensationConfig>

/**
 * 轴配置的完整版本，encoderCompensation 字段必填。
 *
 * 用于 createDefaultAxis / normalizeAxisForEditing 的返回类型，确保返回值可以
 * 赋值给 wind-daq 的 AxisConfig（encoderCompensation 字段必填）。
 */
type AxisConfigWithCompensation = Omit<AxisConfig, 'encoderCompensation'> & {
  encoderCompensation: RequiredEncoderCompensation
}

/**
 * 创建编码器补偿默认配置
 */
export function createDefaultEncoderCompensation(): RequiredEncoderCompensation {
  return {
    enabled: false,
    tolerance: DEFAULT_ENCODER_COMPENSATION_TOLERANCE,
    maxCycles: DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES,
    settleMs: DEFAULT_ENCODER_COMPENSATION_SETTLE_MS,
    minStep: DEFAULT_ENCODER_COMPENSATION_MIN_STEP,
    timeoutMs: DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS,
  }
}

// 保留旧命名作为别名，兼容 motion-controller 现有引用（defaultEncComp）
export const defaultEncComp = createDefaultEncoderCompensation

/**
 * 创建默认轴配置
 *
 * U 轴默认为旋转轴，其余为直线轴。
 * maxSpeed 默认 100（wind-daq 主项目值），motion-controller 可传入 10 保持低速。
 */
export function createDefaultAxis(name: AxisName, maxSpeed: number = DEFAULT_MAX_SPEED): AxisConfigWithCompensation {
  return {
    name,
    enabled: true,
    kind: name === 'U' ? 'ROTARY' : 'LINEAR',
    maxSpeed,
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
export function normalizeAxisForEditing(axis: AxisConfig): AxisConfigWithCompensation {
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
    positionSource: axis.positionSource ?? 'register',
    encoderScale: axis.encoderScale ?? DEFAULT_ENCODER_SCALE,
    encoderCompensation: {
      ...createDefaultEncoderCompensation(),
      ...(axis.encoderCompensation ?? {}),
    },
  }
}

// ---------- UI 辅助函数 ----------

export function getAxisThemeClass(axisName: AxisName): string {
  const themeMap: Record<AxisName, string> = {
    X: 'axis-x-theme',
    Y: 'axis-y-theme',
    Z: 'axis-z-theme',
    U: 'axis-u-theme',
  }
  return themeMap[axisName] || ''
}

/** 返回轴的工程单位符号（旋转轴为度符号，直线轴为毫米）。 */
export function getAxisUnit(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? '°' : 'mm'
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

/**
 * 与 Go 后端 ValidateCompensationConfig 保持逻辑一致。
 *
 * 校验链路：脉冲当量 ≥ encoderScale ≥ tolerance > minStep，
 * 任何一环违反都会产生告警。
 */
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
      message: `容差(${cfg.tolerance})小于编码器分辨率(${scale})，误差永远无法降到容差以下`,
    })
  }

  // tolerance < 脉冲当量 → 电机走一步即越过容差，补偿会在目标两侧反复振荡。
  // 注意：编码器分辨率粗于脉冲当量是正常情况（电机比编码器精细），
  // 补偿精度受限于两者中较粗者，只要 tolerance ≥ 较粗者即可正常工作。
  const ppu = computePulsesPerUnit(axis)
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

  // minStep → 0 脉冲 → 修正无效果
  // 复用上方已计算的 ppu，避免重复调用 computePulsesPerUnit
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

  // minStep >= tolerance → 修正过头
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

// ---------- 机械参数计算 ----------

/**
 * 计算脉冲当量（脉冲/单位）。
 * 旋转轴：(步/转 × 细分数 × 减速比) / 360
 * 直线轴：(步/转 × 细分数) / 导程
 */
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

// ---------- 输入解析与归一化 ----------

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
 * 归一化正数参数：非有限值或非正值回退到 fallback。
 * 运动控制参数默认不允许零值，避免危险的硬件行为。
 */
export function normalizePositive(value: number | undefined, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : fallback
}
