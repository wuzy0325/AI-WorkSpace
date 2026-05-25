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
