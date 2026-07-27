import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { Locale } from './types'

/**
 * 创建 i18n store 的工厂函数。
 *
 * 设计目的：
 * - 消除多项目间 i18n 基础设施（Locale 类型、t 函数、localStorage 读写、timeLocale）的重复
 * - 各项目只需提供自己的 zh/en 字典和 storageKey，调用工厂即可得到 Pinia store
 * - 内置修复：
 *   1. t() 用 split().join() 替代 String.replace + RegExp，避免 $ 模式注入和正则转义问题
 *   2. store 初始化时同步读取 localStorage，避免首屏语言闪烁
 *
 * 类型推导：
 * - K 由 zh 字典推导（keyof typeof zh）
 * - en 字典用 Record<K, string> 约束，编译期保证与 zh 的 key 集合一致
 *   （反向多余 key 由调用方用 satisfies 自行保证，见各项目 i18nStore.ts）
 *
 * 复用方约束（重要）：
 * 本文件位于 shared/frontend，其目录树向上没有 node_modules，导致 TS 编译器和 Rollup
 * 都无法从本文件解析 'pinia' / 'vue'。复用方（各项目的 frontend）必须在自己的
 * tsconfig.json paths 和 vite.config.ts resolve.alias 中显式映射这两个模块到本地
 * node_modules，否则会报 TS2307 和 Rollup "failed to resolve import" 错误：
 *   - tsconfig.json paths: "vue": ["./node_modules/vue"], "pinia": ["./node_modules/pinia"]
 *   - vite.config.ts alias: 'vue': <path>/node_modules/vue, 'pinia': <path>/node_modules/pinia
 */
export function createI18nStore<K extends string>(options: {
  zh: Record<K, string>
  en: Record<K, string>
  /** localStorage 持久化用的 key，每个项目独立避免互相覆盖 */
  storageKey: string
}) {
  const { zh, en, storageKey } = options

  return defineStore('i18n', () => {
    /**
     * 当前语言。
     *
     * store 创建时同步从 localStorage 读取，避免首次渲染用默认 zh、
     * 等 onMounted 再切换到用户保存的语言导致首屏闪烁。
     * localStorage 不可用时（隐私模式等）静默退化为默认 zh。
     */
    const locale = ref<Locale>(readSavedLocale(storageKey))

    /** 当前语言对应的字典。computed 确保 locale 切换时所有依赖 t 的视图自动更新 */
    const dict = computed<Record<K, string>>(() =>
      locale.value === 'zh' ? zh : en,
    )

    /**
     * 时间格式化用的 BCP 47 语言标签。
     *
     * 用于 `Date.prototype.toLocaleTimeString(timeLocale, ...)` 等调用，
     * 让时间戳随语言切换显示习惯（zh→zh-CN，en→en-US），
     * 而不是硬编码 'zh-CN' 导致英文界面下时间仍是中文格式。
     */
    const timeLocale = computed<string>(() => (locale.value === 'zh' ? 'zh-CN' : 'en-US'))

    /**
     * 翻译函数。
     *
     * @param key 字典 key（由调用方传入的 LocaleKey 类型保证存在）
     * @param params 占位符替换，例如 t('monitor.curveCount', { n: 3 })
     * @returns 翻译后的字符串；若 key 不存在（理论上不会发生）原样返回 key
     *
     * 实现说明：
     * 用 split().join() 替代 String.replace(regexp, string)，
     * 避免 replacement string 中的 $ 模式（$&、$`、$'、$$）被解释。
     * 例如后端错误消息含 "$& timeout" 时，replace 会把占位符替换成匹配文本自身，
     * 导致用户看到未替换的 {error} 字面量。split/join 不解释 $ 模式，安全。
     */
    function t(key: K, params?: Record<string, string | number>): string {
      let text = dict.value[key] ?? key
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          text = text.split(`{${k}}`).join(String(v))
        }
      }
      return text
    }

    /**
     * 向后兼容的 no-op。
     *
     * 历史上 App.vue 在 onMounted 中调用 initLocale() 从 localStorage 读取偏好，
     * 但 store 创建时已同步读取，此函数保留为空函数避免破坏调用方。
     * 新代码无需调用。
     */
    function initLocale(): void {
      // 已在 store 创建时同步读取，无需重复
    }

    /** 切换语言并持久化。供 LanguageToggle 组件调用 */
    function setLocale(next: Locale): void {
      locale.value = next
      try {
        localStorage.setItem(storageKey, next)
      } catch {
        // localStorage 不可用时静默忽略，下次启动仍为默认 zh
      }
    }

    /** 在 zh/en 之间切换的便捷方法 */
    function toggleLocale(): void {
      setLocale(locale.value === 'zh' ? 'en' : 'zh')
    }

    return { locale, dict, timeLocale, t, initLocale, setLocale, toggleLocale }
  })
}

/**
 * 同步读取 localStorage 中保存的语言偏好。
 *
 * 用于 store 初始化阶段，避免首次渲染用默认值导致首屏闪烁。
 * localStorage 不可用或值无效时返回默认 'zh'。
 */
function readSavedLocale(storageKey: string): Locale {
  try {
    const saved = localStorage.getItem(storageKey)
    if (saved === 'zh' || saved === 'en') return saved
  } catch {
    // localStorage 不可用（隐私模式、Wails WebView 禁用等），保持默认 zh
  }
  return 'zh'
}
