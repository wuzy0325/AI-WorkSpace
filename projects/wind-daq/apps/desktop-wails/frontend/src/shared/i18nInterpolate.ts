// i18n 文本占位符安全替换工具
//
// 设计目的：
//   String.prototype.replace(pattern, replacement) 中，replacement 字符串里的
//   $ 字符会被特殊解析（$$ → $、$& → 匹配子串、$` → 前缀、$' → 后缀、$n → 捕获组）。
//   当 replacement 来自不可信输入（如后端错误消息、用户输入的设备名）时，
//   会引发注入风险：例如后端错误 "1/$&foo" 会被解析为 "1/<匹配的占位符>foo"，
//   导致 UI 显示错乱或被构造特殊字符绕过预期语义。
//
//   memory 中已明确约束："i18n t() 占位符替换必须使用函数式 replace 或 split/join
//   防止 $ 特殊模式注入风险"。本 helper 统一封装函数式 replace，避免每个调用点
//   重复写 () => value 模板，降低遗漏风险。
//
// 使用方式：
//   import { safeInterpolate } from '@shared/i18nInterpolate'
//   const msg = safeInterpolate(t.value.travErrVerifyInterpolator, '{error}', res.error || '')

/**
 * 安全替换 i18n 文本中的占位符。
 *
 * @param template 含 {placeholder} 占位符的模板字符串
 * @param placeholder 占位符（含花括号，如 '{error}'）
 * @param value 替换值；可为任意字符串，$ 字符不会被特殊解析
 * @returns 替换后的字符串；若 template 为空直接返回原值
 */
export function safeInterpolate(
  template: string,
  placeholder: string,
  value: string,
): string {
  if (!template) return template
  // 函数式 replace：返回值作为字面量替换文本，$ 字符不被特殊解析。
  return template.replace(placeholder, () => value)
}

/**
 * 多占位符安全替换便捷函数：按 (placeholder, value) 对依次替换。
 *
 * @param template 含多个占位符的模板字符串
 * @param pairs [placeholder, value] 数组，按顺序替换
 * @returns 替换后的字符串
 *
 * @example
 *   safeInterpolateMulti(t.value.someKey, ['{min}', '10'], ['{max}', '100'])
 */
export function safeInterpolateMulti(
  template: string,
  ...pairs: Array<[string, string]>
): string {
  let result = template
  for (const [placeholder, value] of pairs) {
    result = safeInterpolate(result, placeholder, value)
  }
  return result
}
