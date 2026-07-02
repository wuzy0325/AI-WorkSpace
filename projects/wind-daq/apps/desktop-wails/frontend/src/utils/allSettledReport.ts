import type { ToastLevel } from '@stores/feedbackStore'

/**
 * 把 Promise.allSettled 的失败结果统一转换为 toast，便于在多个初始化流程复用。
 *
 * 共享 store 的失败语义由 store 自身维护（保留旧状态 + 暴露 error 字段），
 * 此工具只负责 UI 层的可见反馈，不修改 store 状态。
 *
 * @param results  Promise.allSettled 返回的结果数组
 * @param labels   与 results 一一对应的资源标签（用于在 toast 中指明哪一项失败）
 * @param pushToast feedbackStore.pushToast 的引用
 * @param message  失败时的统一文案，默认"加载失败，已保留可用的本地状态"
 */
export function reportAllSettledFailures<T>(
  results: PromiseSettledResult<T>[],
  labels: string[],
  pushToast: (message: string, level?: ToastLevel, durationMs?: number) => void,
  message = '加载失败，已保留可用的本地状态',
): void {
  results.forEach((result, index) => {
    if (result.status === 'fulfilled') return
    const label = labels[index] ?? `任务 ${index + 1}`
    pushToast(`${label}${message}`, 'warning')
  })
}
