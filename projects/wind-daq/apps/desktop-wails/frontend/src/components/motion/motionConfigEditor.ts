import type { AxisConfig, AxisName } from '@shared/types/motion'

export const DEFAULT_AXIS_NAMES: AxisName[] = ['X', 'Y', 'Z', 'U']

export function createDefaultAxis(name: AxisName) {
  return {
    name, enabled: true, kind: name === 'U' ? 'ROTARY' as const : 'LINEAR' as const,
    maxSpeed: 10,
    minLimit: undefined, maxLimit: undefined,
    inverted: false,
  }
}

export function getAxisThemeClass(axisName: AxisName): string {
  const themeMap: Record<AxisName, string> = {
    X: 'axis-x-theme', Y: 'axis-y-theme', Z: 'axis-z-theme', U: 'axis-u-theme',
  }
  return themeMap[axisName] || ''
}

export function getAxisUnit(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? 'deg' : 'mm'
}

export function getAxisInfoLabel(axis: AxisConfig): string {
  return axis.kind === 'ROTARY' ? '步/度:' : '步/mm:'
}