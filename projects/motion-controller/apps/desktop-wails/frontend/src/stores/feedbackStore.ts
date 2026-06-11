import { defineStore } from 'pinia'
import { reactive, ref } from 'vue'

export type ToastLevel = 'info' | 'success' | 'warning' | 'error'

export interface ToastMessage {
  id: number
  level: ToastLevel
  message: string
  durationMs: number
}

export const useFeedbackStore = defineStore('feedback', () => {
  const toasts = ref<ToastMessage[]>([])
  const nextToastId = ref(1)

  const confirmState = reactive({
    open: false,
    message: '',
    title: '确认操作',
    confirmText: '确认',
    cancelText: '取消',
    variant: 'danger' as 'danger' | 'primary',
  })

  let confirmResolver: ((value: boolean) => void) | null = null

  function pushToast(message: string, level: ToastLevel = 'info', durationMs = 2800): number {
    const id = nextToastId.value++
    toasts.value.push({ id, level, message, durationMs })
    if (durationMs > 0) {
      setTimeout(() => removeToast(id), durationMs)
    }
    return id
  }

  function removeToast(id: number) {
    const idx = toasts.value.findIndex((t) => t.id === id)
    if (idx >= 0) {
      toasts.value.splice(idx, 1)
    }
  }

  function confirm(
    message: string,
    options?: Partial<Pick<typeof confirmState, 'title' | 'confirmText' | 'cancelText' | 'variant'>>,
  ): Promise<boolean> {
    if (confirmResolver) {
      confirmResolver(false)
      confirmResolver = null
    }
    // 重置所有字段为默认值，避免上一次调用的文案残留
    confirmState.open = true
    confirmState.message = message
    confirmState.title = '确认操作'
    confirmState.confirmText = '确认'
    confirmState.cancelText = '取消'
    confirmState.variant = 'danger'
    if (options) {
      if (options.title) confirmState.title = options.title
      if (options.confirmText) confirmState.confirmText = options.confirmText
      if (options.cancelText) confirmState.cancelText = options.cancelText
      if (options.variant) confirmState.variant = options.variant
    }
    return new Promise<boolean>((resolve) => {
      confirmResolver = resolve
    })
  }

  function resolveConfirm(accepted: boolean) {
    confirmState.open = false
    const resolver = confirmResolver
    confirmResolver = null
    if (resolver) resolver(accepted)
  }

  return {
    toasts,
    confirmState,
    pushToast,
    removeToast,
    confirm,
    resolveConfirm,
  }
})
