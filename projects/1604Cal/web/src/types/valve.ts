// 阀门状态类型与归一化工具，与后端 internal/domain/valve.go 同源。
// 任何阀门相关字符串字面量都应从这里取，避免组件/store/utils 分裂。

export type ValveState = 'calibration' | 'measurement' | 'unknown' | ''

export const VALVE_STATE_CALIBRATION: ValveState = 'calibration'
export const VALVE_STATE_MEASUREMENT: ValveState = 'measurement'
export const VALVE_STATE_UNKNOWN: ValveState = 'unknown'

/**
 * 把后端 / 设备返回的阀门状态字符串归一化为 ValveState。
 * 兼容历史固件可能返回的 `A1`/`A3` 等带前缀响应、纯数字 `1/2/3`
 * 以及 `open/closed` 等文本同义词；无法识别时返回空串，由 UI 显示为「未知」。
 */
export function normalizeValveStatus(raw: string | null | undefined): ValveState {
  if (!raw) return ''
  const value = String(raw).trim().replace(/^[Aa]/, '').trim().toLowerCase()
  if (value === '') return ''
  if (value === '1' || value === 'calibration' || value === 'calibrate' || value === 'open' || value === 'opened' || value === 'on') {
    return VALVE_STATE_CALIBRATION
  }
  if (value === '2' || value === '3' || value === 'measurement' || value === 'measure' || value === 'close' || value === 'closed' || value === 'off') {
    return VALVE_STATE_MEASUREMENT
  }
  if (value === '0' || value === 'unknown') {
    return VALVE_STATE_UNKNOWN
  }
  return ''
}

/** 给 ElTag 的颜色类型 */
export function valveTagType(status: ValveState): 'success' | 'warning' | 'info' {
  if (status === 'calibration') return 'success'
  if (status === 'measurement') return 'warning'
  return 'info'
}

/** 给 UI 的中文文案 */
export function valveStatusLabel(status: ValveState, raw?: string): string {
  if (status === 'calibration') return '校准模式'
  if (status === 'measurement') return '测量模式'
  if (raw) return `未知(${raw})`
  return '未知'
}
