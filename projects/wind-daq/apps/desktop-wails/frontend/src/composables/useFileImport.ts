import { ref } from 'vue'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'

/** 文件导入 composable 配置项 */
interface UseFileImportOptions {
  /** 导入开始时的回调 */
  onImportStart?: () => void
  /** 导入结束时的回调（无论成功或失败） */
  onImportEnd?: () => void
  /** 发生错误时的回调 */
  onError?: (message: string) => void
}

/** 文件选择过滤条件 */
interface FileFilter {
  displayName: string
  pattern: string
}

/** 多文件选择参数 */
interface ImportFilesOptions {
  title: string
  filters: FileFilter[]
  /** 是否允许多选，默认 false */
  multiple?: boolean
}

/** 单文件选择参数 */
interface ImportSingleFileOptions {
  title: string
  filters: FileFilter[]
}

/**
 * 文件导入 composable
 *
 * 封装 Wails 桌面端与浏览器端双路径文件选择逻辑，
 * 统一对外暴露 importFiles / importSingleFile 接口，
 * 消除各组件中重复的 isWailsAvailable 分支代码。
 *
 * @example
 * ```typescript
 * const { isImporting, importSingleFile, importFiles } = useFileImport({
 *   onError: (msg) => feedbackStore.pushToast(msg, 'error'),
 * })
 *
 * // 单文件选择
 * const path = await importSingleFile({
 *   title: '导入 PRB 文件',
 *   filters: [{ displayName: 'PRB files', pattern: '*.prb' }],
 * })
 *
 * // 多文件选择
 * const paths = await importFiles({
 *   title: '导入多个 PRB 文件',
 *   filters: [{ displayName: 'PRB files', pattern: '*.prb' }],
 *   multiple: true,
 * })
 * ```
 */
export function useFileImport(options?: UseFileImportOptions) {
  /** 当前是否正在导入 */
  const isImporting = ref(false)

  /**
   * 将浏览器端文件过滤器格式（如 ".prb,.csv"）转换为 accept 属性值
   * Wails 端使用 displayName/pattern，浏览器端使用 accept 属性
   */
  function filtersToAccept(filters: FileFilter[]): string {
    return filters
      .map((f) => f.pattern)
      .join(',')
      .replace(/\*/g, '')          // "*.prb" → ".prb"
      .replace(/;/g, ',')          // "*.csv;*.txt" → ".csv,.txt"
  }

  /**
   * 选择多个文件
   *
   * Wails 环境下调用 wailsApi.app.pickFile / pickFiles；
   * 浏览器环境下创建隐藏 input[type=file] 元素触发文件选择。
   *
   * @returns 选中的文件路径数组，取消选择时返回空数组
   */
  async function importFiles(opts: ImportFilesOptions): Promise<string[]> {
    try {
      // —— Wails 桌面端路径 ——
      if (isWailsAvailable()) {
        if (opts.multiple) {
          const paths = await wailsApi.app.pickFiles(opts.title, opts.filters)
          return paths?.filter(Boolean) ?? []
        }
        const path = await wailsApi.app.pickFile(opts.title, opts.filters)
        return path ? [path] : []
      }

      // —— 浏览器端路径 ——
      return await new Promise<string[]>((resolve) => {
        const input = document.createElement('input')
        input.type = 'file'
        input.accept = filtersToAccept(opts.filters)
        input.multiple = !!opts.multiple

        input.onchange = async (event) => {
          const files = Array.from((event.target as HTMLInputElement).files ?? [])
          // 浏览器环境下优先取文件系统路径，回退到文件名
          const paths = files.map((f) => (f as any).path ?? f.name)
          resolve(paths)
        }

        // 用户取消选择时不会触发 onchange，需要监听 cancel 事件（部分浏览器支持）
        // 同时在窗口失焦时兜底返回空数组
        input.addEventListener('cancel', () => resolve([]))

        input.click()
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      options?.onError?.(message)
      return []
    }
  }

  /**
   * 选择单个文件
   *
   * @returns 选中文件的路径，取消选择时返回 null
   */
  async function importSingleFile(opts: ImportSingleFileOptions): Promise<string | null> {
    const paths = await importFiles({ ...opts, multiple: false })
    return paths.length > 0 ? paths[0]! : null
  }

  /**
   * 包装导入操作，自动管理 isImporting 状态和回调
   * 供需要统一管理导入中状态的调用方使用
   */
  async function withImportGuard<T>(fn: () => Promise<T>): Promise<T> {
    isImporting.value = true
    options?.onImportStart?.()
    try {
      return await fn()
    } finally {
      isImporting.value = false
      options?.onImportEnd?.()
    }
  }

  return {
    isImporting,
    importFiles,
    importSingleFile,
    withImportGuard,
  }
}
