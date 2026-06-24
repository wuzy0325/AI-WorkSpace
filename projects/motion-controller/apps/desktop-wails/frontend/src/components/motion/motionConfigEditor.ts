import type { AxisConfig, AxisEncoderCompensationConfig, AxisName } from '@shared/types/motion'

export const DEFAULT_AXIS_NAMES: AxisName[] = ['X', 'Y', 'Z', 'U']

export function defaultEncComp(): AxisEncoderCompensationConfig {
  return { enabled: false, tolerance: 0.01, maxCycles: 10, settleMs: 100, minStep: 0.001, timeoutMs: 5000 }
}

export function createDefaultAxis(name: AxisName): AxisConfig {
  return {
    name, enabled: name !== 'U', kind: name === 'U' ? 'ROTARY' as const : 'LINEAR' as const,
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
