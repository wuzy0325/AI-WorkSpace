// motion-controller 项目的运动控制配置工具
//
// 大部分工具函数已提取到 workspace 级共享模块 @shared-frontend/motion-utils，
// 本文件仅 re-export 共享内容，并保留项目级差异（maxSpeed 默认值）。
//
// 项目级差异说明：
//   motion-controller 是独立的运动控制器测试工具，maxSpeed 默认 10（低速），
//   而共享模块的默认值是 100（wind-daq 主项目的生产速度）。这里通过本地
//   createDefaultAxis 包装覆盖默认值，保持项目行为不变。

import type { AxisName, AxisConfig } from '@shared/types/motion'
export type { AxisConfig, AxisEncoderCompensationConfig, AxisName, PositionSource } from '@shared/types/motion'
export {
  DEFAULT_AXIS_NAMES,
  DEFAULT_STEPS_PER_REV,
  DEFAULT_MICRO_STEPS,
  DEFAULT_LEAD,
  DEFAULT_GEAR_RATIO,
  DEFAULT_ROTARY_GEAR_RATIO,
  defaultGearRatioForKind,
  applyAxisKindDefaults,
  DEFAULT_ENCODER_SCALE,
  DEFAULT_JOG_STEP,
  DEFAULT_ENCODER_COMPENSATION_TOLERANCE,
  DEFAULT_ENCODER_COMPENSATION_MAX_CYCLES,
  DEFAULT_ENCODER_COMPENSATION_SETTLE_MS,
  DEFAULT_ENCODER_COMPENSATION_MIN_STEP,
  DEFAULT_ENCODER_COMPENSATION_TIMEOUT_MS,
  createDefaultEncoderCompensation,
  defaultEncComp,
  normalizeAxisForEditing,
  getAxisThemeClass,
  getAxisUnit,
  getAxisInfoLabel,
  validateEncoderCompensation,
  computePulsesPerUnit,
  parseNumericInput,
  normalizePositive,
  type CompensationWarning,
} from '@shared-frontend/motion-utils'

import { createDefaultAxis as createDefaultAxisBase, defaultMaxSpeedForType, normalizeAxisForEditing } from '@shared-frontend/motion-utils'

// 显式 re-export，供项目内组件直接引用
export { defaultMaxSpeedForType }

/** motion-controller 项目级默认最大速度：测试工具用低速，避免调试时意外碰撞。 */
export const MOTION_CONTROLLER_DEFAULT_MAX_SPEED = 10

/**
 * 创建默认轴配置（项目级包装）
 *
 * 覆盖共享模块的 maxSpeed 默认值（100）为 motion-controller 的低速默认值（10），
 * 其余字段完全复用共享实现。B140 / WTNMC4A 硬件控制器则使用更低速的 4（见 defaultMaxSpeedForType）。
 */
export function createDefaultAxis(name: AxisName, type?: string): AxisConfig {
  return createDefaultAxisBase(name, defaultMaxSpeedForType(type, MOTION_CONTROLLER_DEFAULT_MAX_SPEED))
}

/** 补齐旧配置，同时保留 motion-controller 项目的低速默认策略。 */
export function normalizeAxisForMotionController(axis: AxisConfig, type?: string): AxisConfig {
  return normalizeAxisForEditing({
    ...axis,
    maxSpeed: axis.maxSpeed ?? defaultMaxSpeedForType(type, MOTION_CONTROLLER_DEFAULT_MAX_SPEED),
  })
}
