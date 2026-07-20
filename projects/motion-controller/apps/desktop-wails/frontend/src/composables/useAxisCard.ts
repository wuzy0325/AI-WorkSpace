// composables/useAxisCard.ts
//
// 轴卡片相关的纯函数工具集合。从 MotionControlPanel 抽出，便于复用与单测。
// 这些函数均无 Vue 响应式依赖，仅做纯计算。

import type { AxisName, AxisConfig, MotionControllerProfile } from '@shared/types/motion'

/** 轴单位：旋转轴返回 °，直线轴返回 mm。 */
export function getAxisUnit(axisName: AxisName, profile?: MotionControllerProfile): string {
  return getAxisKind(axisName, profile) === 'ROTARY' ? '°' : 'mm'
}

/** 获取轴类型（LINEAR/ROTARY），从 profile.axes 推断；缺省为 LINEAR。 */
export function getAxisKind(axisName: AxisName, profile?: MotionControllerProfile): 'LINEAR' | 'ROTARY' {
  if (!profile) return 'LINEAR'
  const axisConfig = profile.axes.find((a) => a.name === axisName)
  return axisConfig?.kind ?? 'LINEAR'
}

/** 获取轴限位 {min, max}；profile 缺失或字段为空时返回 undefined。 */
export function getAxisLimits(axisName: AxisName, profile?: MotionControllerProfile): { min: number | undefined; max: number | undefined } {
  if (!profile) return { min: undefined, max: undefined }
  const axisConfig = profile.axes.find((a) => a.name === axisName)
  return {
    min: axisConfig?.minLimit,
    max: axisConfig?.maxLimit,
  }
}

export interface TargetValidation {
  valid: boolean
  /** 接近限位或超限位的提示文案；valid 为 true 但接近限位时也会填充。 */
  warning?: string
}

/** 校验目标位置是否在限位范围内；返回 {valid, warning}。 */
export function validateTargetPosition(axisName: AxisName, targetPosition: number, profile?: MotionControllerProfile): TargetValidation {
  const limits = getAxisLimits(axisName, profile)
  if (limits.min !== undefined && targetPosition < limits.min) {
    return { valid: false, warning: `目标位置 ${targetPosition.toFixed(2)} 超出负限位 ${limits.min.toFixed(2)}` }
  }
  if (limits.max !== undefined && targetPosition > limits.max) {
    return { valid: false, warning: `目标位置 ${targetPosition.toFixed(2)} 超出正限位 ${limits.max.toFixed(2)}` }
  }
  if (limits.min !== undefined && limits.max !== undefined) {
    const range = limits.max - limits.min
    const margin = range * 0.1
    if (targetPosition < limits.min + margin || targetPosition > limits.max - margin) {
      return { valid: true, warning: '接近限位，请谨慎操作' }
    }
  }
  return { valid: true }
}

/** 返回限位警告的 CSS class 名（limit-exceeded / limit-near / 空串）。 */
export function getLimitWarningClass(axisName: AxisName, targetPosition: number, profile?: MotionControllerProfile): string {
  const result = validateTargetPosition(axisName, targetPosition, profile)
  if (!result.valid) return 'limit-exceeded'
  if (result.warning) return 'limit-near'
  return ''
}

/** 格式化限位最小值用于显示。 */
export function axisMinLabel(axisName: AxisName, profile?: MotionControllerProfile): string {
  const lim = getAxisLimits(axisName, profile)
  return lim.min !== undefined ? lim.min.toFixed(2) : '—'
}

/** 格式化限位最大值用于显示。 */
export function axisMaxLabel(axisName: AxisName, profile?: MotionControllerProfile): string {
  const lim = getAxisLimits(axisName, profile)
  return lim.max !== undefined ? lim.max.toFixed(2) : '—'
}

/** 计算轴读数显示值（原始位置减去零点偏移）。 */
export function axisReadout(axisName: AxisName, rawPosition: number, zeroOffset: number): string {
  return (rawPosition - zeroOffset).toFixed(2)
}

/**
 * 计算历史条带的渐变背景样式。
 * NOTE: 动态色相渐变（蓝→紫→红），无法用静态 token 表达，依 §28.2 豁免。
 * @param data 位置历史数组
 * @returns CSS backgroundImage 字符串；空数组返回空对象
 */
export function historyBarStyle(data: number[]): Record<string, string> {
  if (data.length === 0) return {}
  const min = data.reduce((a, b) => Math.min(a, b), Infinity)
  const max = data.reduce((a, b) => Math.max(a, b), -Infinity)
  if (!Number.isFinite(min) || !Number.isFinite(max) || min === max) {
    return {}
  }
  const segments: string[] = []
  const n = data.length
  for (let i = 0; i < n; i += 1) {
    const t = n === 1 ? 0 : (i / (n - 1)) * 100
    const ratio = (data[i] - min) / (max - min)
    // NOTE: 动态 hue 渐变 — 按 §28.2 豁免 token 规则
    const hue = 210 - ratio * 90
    segments.push(`hsl(${hue} 80% 60%) ${t.toFixed(1)}%`)
  }
  return {
    backgroundImage: `linear-gradient(to right, ${segments.join(', ')})`,
  }
}

/** 限位警告提示文案（用于卡片内 hint 行）。 */
export function axisLimitWarningHint(axisName: AxisName, absoluteTarget: number, profile?: MotionControllerProfile): { text: string; cls: string } {
  const result = validateTargetPosition(axisName, absoluteTarget, profile)
  if (!result.valid) return { text: result.warning || '', cls: 'err' }
  if (result.warning) return { text: result.warning, cls: 'warn' }
  return { text: '', cls: '' }
}
