// wind-daq 项目的运动控制配置工具
//
// 大部分工具函数已提取到 workspace 级共享模块 @shared-frontend/motion-utils，
// 本文件仅 re-export 共享内容。
//
// wind-daq 的 maxSpeed 默认值（100）与共享模块一致，无需项目级包装。
// 如未来需要项目级差异，可参考 motion-controller 的 motionConfigEditor.ts
// 添加本地 createDefaultAxis 包装。

export type { AxisConfig, AxisEncoderCompensationConfig, AxisName, PositionSource } from '@shared/types/motion'
export {
  DEFAULT_AXIS_NAMES,
  DEFAULT_STEPS_PER_REV,
  DEFAULT_MICRO_STEPS,
  DEFAULT_LEAD,
  DEFAULT_GEAR_RATIO,
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

import {
  DEFAULT_MAX_SPEED,
  HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED,
  defaultMaxSpeedForType,
  createDefaultAxis as createDefaultAxisBase,
} from '@shared-frontend/motion-utils'
import type { AxisName, AxisConfig } from '@shared/types/motion'

export { DEFAULT_MAX_SPEED, HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED, defaultMaxSpeedForType }

/**
 * 创建默认轴配置（项目级包装）
 *
 * wind-daq 模拟控制器沿用共享模块的默认最大速度（100），B140 / WTNMC4A 硬件控制器
 * 使用更低速的 4（见 defaultMaxSpeedForType），使硬件新建设备默认速度更安全。
 */
export function createDefaultAxis(name: AxisName, type?: string): AxisConfig {
  return createDefaultAxisBase(name, defaultMaxSpeedForType(type, DEFAULT_MAX_SPEED))
}
