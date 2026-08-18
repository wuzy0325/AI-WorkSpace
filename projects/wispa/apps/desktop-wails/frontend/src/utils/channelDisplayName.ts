// channelDisplayName.ts 为通道展示名提供"用户自定义名称优先，回退到 i18n 默认名"的统一逻辑。
// p1604 共 18 通道：0~15 为压力通道（默认名按 1-based 序号渲染），16/17 为大气压/大气温度。
//
// Translate 类型用 i18n key 联合字面量显式列举本工具依赖的 key：
//   - config.defaultChannelName：默认通道名模板，参数 {n} 为 1-based 通道序号
//   - config.atmosphericPressure：大气压通道默认名（index=16）
//   - config.atmosphericTemperature：大气温度通道默认名（index=17）
//
// 维护提示：若 i18n 字典重命名上述 key，需同步更新本文件的 Translate 类型联合字面量
// 与 channelDisplayName 函数体中的 t(...) 调用，否则 TypeScript 编译期检查会立即报错
// （这正是用联合字面量而非 string 的好处）。改 key 时记得用 grep 全局搜索旧 key 防遗漏。
interface Translate {
  (key: 'config.defaultChannelName', params: { n: number }): string
  (key: 'config.atmosphericPressure'): string
  (key: 'config.atmosphericTemperature'): string
}

export function channelDisplayName(index: number, name: string, t: Translate): string {
  if (name) return name
  if (index === 16) return t('config.atmosphericPressure')
  if (index === 17) return t('config.atmosphericTemperature')
  return t('config.defaultChannelName', { n: index + 1 })
}
