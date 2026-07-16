import type { InterpolationResult } from '@shared/types/traversal'

export type VisualizationParam =
  | 'machNumber'
  | 'velocity'
  | 'P0'
  | 'Ps'
  | 'dynamicPressure'
  | 'density'
  | 'alpha'
  | 'beta'

export interface VisualizationParamConfig {
  labelKey: 'mach' | 'velocity' | 'totalPressure' | 'staticPressure' | 'dynamicP' | 'density' | 'alpha' | 'beta'
  fallbackLabel: string
  title: string
  unit: string
}

export const VISUALIZATION_PARAM_CONFIG: Record<VisualizationParam, VisualizationParamConfig> = {
  machNumber: { labelKey: 'mach', fallbackLabel: 'Mach', title: 'Mach', unit: '' },
  velocity: { labelKey: 'velocity', fallbackLabel: 'Velocity', title: 'Velocity', unit: 'm/s' },
  P0: { labelKey: 'totalPressure', fallbackLabel: 'P0', title: 'P0', unit: 'Pa' },
  Ps: { labelKey: 'staticPressure', fallbackLabel: 'Ps', title: 'Ps', unit: 'Pa' },
  dynamicPressure: { labelKey: 'dynamicP', fallbackLabel: 'Dynamic Pressure', title: 'Dynamic Pressure', unit: 'Pa' },
  density: { labelKey: 'density', fallbackLabel: 'Density', title: 'Density', unit: 'kg/m^3' },
  alpha: { labelKey: 'alpha', fallbackLabel: 'Alpha', title: 'Alpha', unit: 'deg' },
  beta: { labelKey: 'beta', fallbackLabel: 'Beta', title: 'Beta', unit: 'deg' }
}

export function getParamValue(result: InterpolationResult, param: VisualizationParam): number | null {
  switch (param) {
    case 'machNumber':
      return result.machNumber
    case 'velocity':
      return result.velocity
    case 'P0':
      return result.P0 ?? null
    case 'Ps':
      return result.Ps ?? null
    case 'dynamicPressure':
      return result.dynamicPressure
    case 'density':
      return result.density
    case 'alpha':
      return result.alpha
    case 'beta':
      return result.beta
  }
}

export interface HeatmapCell {
  value: [number, number, number]
  alpha: number
  beta: number
}

export interface ChartTheme {
  textColor: string
  axisColor: string
  gridColor: string
  panelColor: string
  tooltipBackground: string
  tooltipBorder: string
  heatmapColors: string[]
  // 主系列颜色：折线/雷达/箭头等使用，需随主题切换
  seriesPrimary: string
  // 强调色：热力图高亮边框、激活态背景等
  emphasisBorder: string
  // 雷达图填充色（带透明度的主色）
  radarAreaFill: string
  // 雷达图背景分区色（奇偶交替，营造分层感）
  radarSplitArea: [string, string]
}

