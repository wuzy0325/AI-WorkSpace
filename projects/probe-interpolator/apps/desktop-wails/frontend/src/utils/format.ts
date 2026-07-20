// utils/format.ts 抽取 5 / 3 / 7 孔 Vue 组件中完全相同的数值格式化逻辑。
//
// 历史背景：3 个 Workspace.vue 各自重复定义了 formatVal / formatInt，code-review P3 抽出。
// - formatVal：固定 4 位小数（探针插值结果默认精度）
// - formatInt：本地化千分位整数（如马赫数范围、文件数）

/**
 * formatVal 将数值格式化为 4 位小数字符串。
 * 非有限数（NaN/Infinity）返回 "-"，避免 UI 显示 "NaN"。
 */
export function formatVal(v: number): string {
  if (!isFinite(v)) return '-'
  return v.toFixed(4)
}

/**
 * formatInt 将数值格式化为本地化千分位整数（zh-CN）。
 * 非有限数（NaN/Infinity）返回 "-"。
 */
export function formatInt(v: number): string {
  if (!isFinite(v)) return '-'
  return v.toLocaleString('zh-CN')
}
