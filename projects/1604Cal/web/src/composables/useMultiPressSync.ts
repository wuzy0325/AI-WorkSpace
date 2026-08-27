import { onMounted, onUnmounted } from 'vue'
import { createEventStream } from '@/api/client'
import type { StreamEventPayload } from '@/types/api'
import { useMultiPressStore } from '@/stores/multipress'

/**
 * Composable 管理 multipress 页面的 SSE 事件流和 HTTP 轮询。
 * SSE 负责实时推送压力更新，HTTP 轮询作为兜底。
 */
export function useMultiPressSync() {
  const store = useMultiPressStore()

  let eventSource: EventSource | null = null

  function setupSSE() {
    if (eventSource) return
    eventSource = createEventStream({
      onEvent: (payload: StreamEventPayload) => {
        if (payload.type.startsWith('multipress.')) {
          store.handleSSEEvent(payload)
        }
      },
      onError: (error) => {
        console.warn('[useMultiPressSync] SSE 连接断开:', error)
      }
    })
  }

  function teardownSSE() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
  }

  onMounted(() => {
    store.loadPressureDevices()
    store.startPolling()
    setupSSE()
  })

  onUnmounted(() => {
    teardownSSE()
    store.stopPolling()
  })
}
