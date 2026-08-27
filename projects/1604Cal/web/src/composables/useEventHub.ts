import { createEventStream } from '@/api/client'
import type { StreamEventPayload } from '@/types/api'

type EventHandler = (payload: StreamEventPayload) => void
type PollTask = () => Promise<void>

let sharedEventSource: EventSource | null = null
let subscriberCount = 0
const handlers = new Map<string, Set<EventHandler>>()
const globalHandlers = new Set<EventHandler>()
const pollTasks = new Map<string, { task: PollTask; intervalMs: number; timer: ReturnType<typeof setInterval> | null }>()

function ensureConnection() {
  if (sharedEventSource) return
  sharedEventSource = createEventStream({
    onEvent: (payload) => {
      const typeHandlers = handlers.get(payload.type)
      if (typeHandlers) {
        for (const h of typeHandlers) h(payload)
      }
      for (const h of globalHandlers) h(payload)
    },
    onError: (error) => {
      console.warn('[SSE] connection error, will auto-reconnect:', error)
    }
  })
}

function closeConnection() {
  if (sharedEventSource) {
    sharedEventSource.close()
    sharedEventSource = null
  }
}

export function useEventHub() {
  function subscribe(type: string, handler: EventHandler): () => void {
    subscriberCount++
    if (!handlers.has(type)) handlers.set(type, new Set())
    handlers.get(type)!.add(handler)
    ensureConnection()

    return () => {
      const typeHandlers = handlers.get(type)
      if (typeHandlers) {
        typeHandlers.delete(handler)
        if (typeHandlers.size === 0) handlers.delete(type)
      }
      subscriberCount--
      if (subscriberCount <= 0) {
        closeConnection()
        for (const [, entry] of pollTasks) {
          if (entry.timer) clearInterval(entry.timer)
          entry.timer = null
        }
        pollTasks.clear()
        subscriberCount = 0
      }
    }
  }

  function subscribeGlobal(handler: EventHandler): () => void {
    subscriberCount++
    globalHandlers.add(handler)
    ensureConnection()
    return () => {
      globalHandlers.delete(handler)
      subscriberCount--
      if (subscriberCount <= 0) {
        closeConnection()
        subscriberCount = 0
      }
    }
  }

  function registerPoll(id: string, task: PollTask, intervalMs: number): () => void {
    const existing = pollTasks.get(id)
    if (existing) {
      if (existing.timer) clearInterval(existing.timer)
    }
    const timer = setInterval(task, intervalMs)
    pollTasks.set(id, { task, intervalMs, timer })
    return () => {
      const entry = pollTasks.get(id)
      if (entry && entry.timer) {
        clearInterval(entry.timer)
        pollTasks.delete(id)
      }
    }
  }

  return { subscribe, subscribeGlobal, registerPoll }
}
