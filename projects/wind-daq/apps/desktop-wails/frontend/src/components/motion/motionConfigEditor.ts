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

import {
  DEFAULT_MAX_SPEED,
  HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED,
  defaultMaxSpeedForType,
  createDefaultAxis as createDefaultAxisBase,
} from '@shared-frontend/motion-utils'
import type { AxisName, AxisConfig } from '@shared/types/motion'

export { DEFAULT_MAX_SPEED, HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED, defaultMaxSpeedForType }

/**
 * WTNMC4A 控制器新建时的默认细分数。
 *
 * MC4A 驱动器出厂常用 4 细分（与 B140 的 40 细分不同），新建设备时按硬件实际
 * 默认值填充，避免用户首次连接时因细分不匹配导致位移/速度与预期不符。
 */
const WTNMC4A_DEFAULT_MICRO_STEPS = 4

/**
 * 创建默认轴配置（项目级包装）
 *
 * 速度默认值：wind-daq 项目所有控制器类型（含模拟控制器）新建时统一默认 4
 * （HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED），避免模拟控制器沿用共享模块的 100
 * 导致首次操作速度过快。B140 / WTNMC4A 由 defaultMaxSpeedForType 返回 4，
 * 模拟控制器通过 fallback 参数显式传入 4。
 *
 * 细分数默认值：WTNMC4A 覆盖为 4（驱动器出厂常用细分），B140 / 模拟控制器沿用共享
 * 默认值 40（DEFAULT_MICRO_STEPS）。用户明确要求 B140 保持原默认值不变。
 */
export function createDefaultAxis(name: AxisName, type?: string): AxisConfig {
  const axis = createDefaultAxisBase(name, defaultMaxSpeedForType(type, HARDWARE_CONTROLLER_DEFAULT_MAX_SPEED))
  // MC4A 细分数差异：驱动器出厂 4 细分，与 B140 的 40 细分不同
  if (type === 'WTNMC4A-MC') {
    axis.microSteps = WTNMC4A_DEFAULT_MICRO_STEPS
  }
  return axis
}
