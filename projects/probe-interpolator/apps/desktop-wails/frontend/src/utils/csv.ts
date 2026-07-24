// utils/csv.ts 抽取 5 / 3 / 7 孔 Vue 组件中完全相同的 CSV 字段转义逻辑。
//
// 历史背景：3 个 Workspace.vue 各自重复定义了 escapeCsvField，code-review P3 抽出。
// CSV 转义规则：含逗号、换行符、双引号时用双引号包裹，内部双引号转义为两个。

/**
 * escapeCsvField 处理 CSV 字段转义。
 * 含逗号、换行符、双引号时用双引号包裹，内部双引号转义为两个。
 */
export function escapeCsvField(value: string | number): string {
  const s = String(value)
  if (s.includes(',') || s.includes('"') || s.includes('\n') || s.includes('\r')) {
    return '"' + s.replace(/"/g, '""') + '"'
  }
  return s
}
