// ⚠️ 架构边界警告：此文件包含业务规则（精度默认值），按六边形架构应迁移到 Go 后端 core 层。
// 前端只应从后端 API 获取精度配置，不应硬编码业务默认值。
// TODO: 将精度默认值迁移到 Go 后端，前端通过 API 获取。

import type { CalibrationConfig } from '@shared/types/calibration'

export const DEFAULT_CALIBRATION_PROBE_PRECISION = 3
export const DEFAULT_CALIBRATION_MACH_PRECISION = 4
export const DEFAULT_CALIBRATION_VELOCITY_PRECISION = 3

export function applyCalibrationPrecisionDefaults(config: CalibrationConfig): CalibrationConfig {
  const result = { ...config }
  if (!result.derivedValuePrecision) {
    result.derivedValuePrecision = {
      machNumber: DEFAULT_CALIBRATION_MACH_PRECISION,
      velocity: DEFAULT_CALIBRATION_VELOCITY_PRECISION,
    }
  }
  if (result.probeChannels) {
    result.probeChannels = result.probeChannels.map((ch) => ({
      ...ch,
      precision: ch.precision ?? DEFAULT_CALIBRATION_PROBE_PRECISION,
    }))
  }
  return result
}

export function getProbeChannelPrecision(config: CalibrationConfig | null | undefined, role: string): number {
  if (!config?.probeChannels) return DEFAULT_CALIBRATION_PROBE_PRECISION
  const ch = config.probeChannels.find((c) => c.role === role)
  return ch?.precision ?? DEFAULT_CALIBRATION_PROBE_PRECISION
}

export function getDerivedValuePrecision(
  config: CalibrationConfig | null | undefined,
  key: 'machNumber' | 'velocity'
): number {
  if (!config?.derivedValuePrecision) {
    return key === 'machNumber' ? DEFAULT_CALIBRATION_MACH_PRECISION : DEFAULT_CALIBRATION_VELOCITY_PRECISION
  }
  return config.derivedValuePrecision[key] ?? (key === 'machNumber' ? DEFAULT_CALIBRATION_MACH_PRECISION : DEFAULT_CALIBRATION_VELOCITY_PRECISION)
}
