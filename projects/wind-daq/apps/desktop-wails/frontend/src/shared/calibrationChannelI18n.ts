// 探针校准通道角色 → i18n key 的统一映射
//
// 设计目的：
//   校准配置的 name 字段会持久化到磁盘，但 UI 显示需要随全局语言切换。
//   方案是优先按 role 从 i18n 取当前语言的默认名称，role 缺失或翻译缺失时
//   回退到持久化的 name 字段。这样切换语言无需重写配置文件即可立即生效。
//
// 使用方式：
//   import { getProbeChannelDisplayName } from '@shared/calibrationChannelI18n'
//   const displayName = getProbeChannelDisplayName(ch.role, ch.name, t.value)

// 五孔探针通道角色 → i18n key
const FIVE_HOLE_ROLE_I18N_KEY: Record<string, string> = {
  'fiveHole.p1': 'fiveHoleP1',
  'fiveHole.p2': 'fiveHoleP2',
  'fiveHole.p3': 'fiveHoleP3',
  'fiveHole.p4': 'fiveHoleP4',
  'fiveHole.p5': 'fiveHoleP5',
  'fiveHole.pAtm': 'fiveHolePAtm',
  'fiveHole.tAtm': 'fiveHoleTAtm',
  'fiveHole.pTotal': 'fiveHolePTotal',
  'fiveHole.pTunnelStatic': 'fiveHolePTunnelStatic',
  'fiveHole.tTunnel': 'fiveHoleTTunnel',
}

// 三孔探针通道角色 → i18n key
const THREE_HOLE_ROLE_I18N_KEY: Record<string, string> = {
  'threeHole.p1': 'threeHoleP1',
  'threeHole.p2': 'threeHoleP2',
  'threeHole.p3': 'threeHoleP3',
  'threeHole.pAtm': 'threeHolePAtm',
  'threeHole.tAtm': 'threeHoleTAtm',
  'threeHole.pTotal': 'threeHolePTotal',
  'threeHole.pStatic': 'threeHolePStatic',
}

// 合并后的统一映射表，供所有探针类型共用
export const PROBE_CHANNEL_ROLE_I18N_KEY: Record<string, string> = {
  ...FIVE_HOLE_ROLE_I18N_KEY,
  ...THREE_HOLE_ROLE_I18N_KEY,
}

/**
 * 根据通道角色获取当前语言的默认显示名称
 *
 * @param role         通道角色（如 'fiveHole.p1'），决定取哪个 i18n key
 * @param fallbackName 回退名称（通常是持久化的 name 字段）
 * @param translations 当前语言的翻译表（即 i18nStore 的 t.value）
 * @returns 当前语言的通道显示名称；role 未映射或翻译缺失时返回 fallbackName
 */
export function getProbeChannelDisplayName(
  role: string | undefined,
  fallbackName: string,
  translations: Record<string, string>,
): string {
  if (!role) return fallbackName
  const key = PROBE_CHANNEL_ROLE_I18N_KEY[role]
  return key && translations[key] ? translations[key] : fallbackName
}
