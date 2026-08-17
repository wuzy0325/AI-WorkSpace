/**
 * DAQ-T-1602 热电偶类型量程工具。
 *
 * 热电偶类型 code 与设备 Type Code 寄存器值一致（0=J 1=K 2=T 3=E 4=R 5=S 6=B 7=N）。
 * 量程 [min,max] ℃ 为用户提供的设备固件量程表（spec-daq-t1602 §Type Code 枚举，
 * 2026-08-13 真机交叉验证，含非教科书量程如 code 3=E (-200,400)）。
 *
 * 该表同时服务两处 UI：
 *   - 配置面板（DaqT1602Config.vue）的类型下拉、量程图例
 *   - 仪表盘通道卡片（DeviceDetailPanel）的量程展示
 */
import type { DeviceProfile } from '@api/types'

export interface T1602TcType {
  code: number
  label: string
  min: number
  max: number
}

export const T1602_TC_TYPES: T1602TcType[] = [
  { code: 0, label: 'J', min: -50, max: 50 },
  { code: 1, label: 'K', min: 0, max: 1200 },
  { code: 2, label: 'T', min: 0, max: 1300 },
  { code: 3, label: 'E', min: -200, max: 400 },
  { code: 4, label: 'R', min: 0, max: 1000 },
  { code: 5, label: 'S', min: 0, max: 1700 },
  { code: 6, label: 'B', min: 0, max: 1768 },
  { code: 7, label: 'N', min: 0, max: 1800 },
]

/** 默认热电偶类型：T 型（type code 2），与设备出厂默认一致 */
export const T1602_DEFAULT_TYPE_CODE = 2

/** 兜底类型常量：避免依赖运行时 find，保证 t1602RangeForTypeCode 永不抛错 */
const T1602_DEFAULT_TYPE: T1602TcType = { code: 2, label: 'T', min: 0, max: 1300 }

/**
 * 按类型码取量程。未知/越界 code 回退到默认 T 型量程，
 * 避免对非法类型码返回 NaN 导致 UI 显示异常。
 */
export function t1602RangeForTypeCode(code: number | undefined): { min: number; max: number } {
  const type = T1602_TC_TYPES.find((t) => t.code === code) ?? T1602_DEFAULT_TYPE
  return { min: type.min, max: type.max }
}

/**
 * 按设备 profile 取通道量程：
 *   - DAQ-T-1602：由热电偶类型码决定（通道表不存 rangeMin/rangeMax）
 *   - 其他设备：回退到通道表 rangeMin/rangeMax（缺省 ±10）
 *
 * 供 DeviceDetailPanel（仪表盘卡片）与 deviceStore.getChannelRange（总览告警判定）共用，
 * 避免两处维护不同量程来源。
 */
export function getDeviceChannelRange(
  profile: DeviceProfile | undefined,
  channelIndex: number,
): { min: number; max: number } {
  if (profile?.type === 'DAQ-T-1602') {
    return t1602RangeForTypeCode(profile.daqT1602Config?.typeCodes?.[channelIndex])
  }
  const channel = profile?.channels.find((item) => item.index === channelIndex)
  return {
    min: channel?.rangeMin ?? -10,
    max: channel?.rangeMax ?? 10,
  }
}
