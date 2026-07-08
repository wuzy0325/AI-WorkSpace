import type {
  CalibrationAnyDataPoint,
  CalibrationDataPoint,
  FiveHoleCoefficients,
  ThreeHoleDataPoint,
  TotalPressureDataPoint,
  TotalTemperatureCalibrationPoint
} from '@shared/types/calibration'

export function isFiveHoleDataPoint(point: CalibrationAnyDataPoint): point is CalibrationDataPoint {
  return 'coefficients' in point && 'Kalpha' in (point as CalibrationDataPoint).coefficients
}

export function isThreeHoleDataPoint(point: CalibrationAnyDataPoint): point is ThreeHoleDataPoint {
  return 'coefficients' in point && 'Kb' in (point as ThreeHoleDataPoint).coefficients
}

export function isTotalPressureDataPoint(point: CalibrationAnyDataPoint): point is TotalPressureDataPoint {
  return 'coefficients' in point && 'CPT' in (point as TotalPressureDataPoint).coefficients
}

export function isTotalTemperatureDataPoint(point: CalibrationAnyDataPoint): point is TotalTemperatureCalibrationPoint {
  return 'targetMachNumber' in point && 'recoveryCoefficient' in point
}

export function isValidFiveHoleCoefficients(coeffs: unknown): coeffs is FiveHoleCoefficients {
  if (!coeffs || typeof coeffs !== 'object') return false
  const c = coeffs as Record<string, unknown>
  return typeof c.Kalpha === 'number' && typeof c.Kbeta === 'number' && typeof c.CPT === 'number' && typeof c.CPS === 'number'
}
