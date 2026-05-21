import type { DataPayload } from '@api/types'

export type SseEventType = 'payload' | 'connected' | 'error'
export type SseEventHandler = (type: SseEventType, data: unknown) => void

export interface SseSubscription {
  unsubscribe: () => void
}

const apiBase = import.meta.env.VITE_API_BASE ?? ''

export function subscribeDaqStream(
  deviceId: string,
  onPayload: (payload: DataPayload) => void,
  onError?: (err: string) => void,
): SseSubscription {
  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null
  let aborted = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let backoff = 500

  async function connect() {
    if (aborted) return
    try {
      const response = await fetch(`${apiBase}/api/daq/stream/${deviceId}`)
      if (!response.ok) {
        onError?.(`SSE HTTP ${response.status}`)
        scheduleReconnect()
        return
      }
      const contentType = response.headers.get('Content-Type') ?? ''
      if (!contentType.includes('text/event-stream')) {
        onError?.('SSE: not an event-stream response')
        scheduleReconnect()
        return
      }
      backoff = 500
      onError?.('connected')

      const body = response.body
      if (!body) {
        onError?.('SSE: no response body')
        scheduleReconnect()
        return
      }

      reader = body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (!aborted) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''

        let currentEvent = ''
        let currentData = ''

        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7).trim()
          } else if (line.startsWith('data: ')) {
            currentData = line.slice(6).trim()
          } else if (line === '' && currentData) {
            if (currentEvent === 'payload') {
              try {
                const payload = JSON.parse(currentData) as DataPayload
                onPayload(payload)
              } catch {
                onError?.('SSE: parse error')
              }
            }
            currentEvent = ''
            currentData = ''
          }
        }
      }
    } catch {
      onError?.('SSE: connection lost')
    }

    if (!aborted) {
      reader = null
      scheduleReconnect()
    }
  }

  function scheduleReconnect() {
    if (aborted) return
    backoff = Math.min(backoff * 1.5, 10000)
    reconnectTimer = setTimeout(() => { void connect() }, backoff)
  }

  void connect()

  return {
    unsubscribe: () => {
      aborted = true
      if (reconnectTimer !== null) clearTimeout(reconnectTimer)
      if (reader !== null) { void reader.cancel().catch(() => {}) }
    },
  }
}
