import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { TraversalProgressEvent, TraversalTestStatus } from '@shared/types/traversal'

// Mock wails-adapter:测试走 fetch 分支,不依赖 Wails 运行时
vi.mock('@api/wails-adapter', () => ({
  isWailsAvailable: () => false,
  wailsApi: {},
}))

// Mock traversalApi:等待字段透传（spec-traversal-acquisition-stop）——
// applyProgressEvent 每次重建 status 时必须显式保留等待字段，否则横幅消失。
const progressCallbacks: Array<(event: TraversalProgressEvent) => void> = []
vi.mock('@api/traversalApi', () => ({
  traversalApi: {
    getStatus: vi.fn(),
    getConfig: vi.fn(async () => ({ success: true, data: null })),
    onProgress: vi.fn((cb: (event: TraversalProgressEvent) => void) => {
      progressCallbacks.push(cb)
      return () => {}
    }),
    onComplete: vi.fn(() => () => {}),
    onError: vi.fn(() => () => {}),
  },
}))

// Mock i18nStore:store 初始化时读取 t.travErrNetwork,Proxy 兜底空字符串
vi.mock('@stores/i18nStore', () => ({
  useI18nStore: () => ({
    t: new Proxy({}, { get: () => '' }),
    locale: 'zh',
  }),
}))

vi.mock('@stores/deviceStore', () => ({
  useDeviceStore: () => ({ profiles: [] }),
}))

vi.mock('@stores/storageStore', () => ({
  useStorageStore: () => ({ settings: { refreshRateHz: 5 } }),
}))

import { useTraversalStore } from '@stores/traversalStore'
import { traversalApi } from '@api/traversalApi'

const mockGetStatus = traversalApi.getStatus as ReturnType<typeof vi.fn>

function runningWithWaiting(): TraversalTestStatus {
  return {
    taskId: 't1',
    state: 'running',
    status: 'running',
    totalPoints: 10,
    completedPoints: 0,
    progress: 0,
    waitingForAcquisition: true,
    waitingDevices: [{ name: 'dev-1', state: 'stopped' }],
    waitingForAcquisitionSinceMs: 1000,
  }
}

function progressEvent(patch: Partial<TraversalProgressEvent>): TraversalProgressEvent {
  return {
    taskId: 't1',
    completedPoints: 0,
    totalPoints: 10,
    currentPoint: { alpha: 0, beta: 0 },
    ...patch,
  } as TraversalProgressEvent
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  progressCallbacks.length = 0
})

describe('traversalStore 等待字段透传（spec-traversal-acquisition-stop）', () => {
  it('progress 事件重建状态后等待字段仍存在', async () => {
    const store = useTraversalStore()
    mockGetStatus.mockResolvedValueOnce({ success: true, data: runningWithWaiting() })
    await store.refreshStatus()

    expect(store.status?.waitingForAcquisition).toBe(true)
    expect(store.status?.waitingDevices).toEqual([{ name: 'dev-1', state: 'stopped' }])
    expect(store.status?.waitingForAcquisitionSinceMs).toBe(1000)
    expect(progressCallbacks.length).toBe(1)

    // progress 事件重建 status（不含等待字段时用 previousStatus 透传）
    progressCallbacks[0](progressEvent({ completedPoints: 2 }))
    expect(store.status?.waitingForAcquisition).toBe(true)
    expect(store.status?.waitingDevices).toEqual([{ name: 'dev-1', state: 'stopped' }])
    expect(store.status?.waitingForAcquisitionSinceMs).toBe(1000)
  })

  it('progress 事件携带新等待字段时用事件值覆盖', async () => {
    const store = useTraversalStore()
    mockGetStatus.mockResolvedValueOnce({ success: true, data: runningWithWaiting() })
    await store.refreshStatus()

    progressCallbacks[0](progressEvent({
      waitingForAcquisition: true,
      waitingDevices: [{ name: 'dev-1', state: 'reconnect_required' }],
      waitingForAcquisitionSinceMs: 2000,
    }))
    expect(store.status?.waitingDevices).toEqual([{ name: 'dev-1', state: 'reconnect_required' }])
    expect(store.status?.waitingForAcquisitionSinceMs).toBe(2000)
  })

  it('后端清空等待字段（waitingForAcquisition=false）后横幅数据被清空', async () => {
    const store = useTraversalStore()
    mockGetStatus.mockResolvedValueOnce({ success: true, data: runningWithWaiting() })
    await store.refreshStatus()

    progressCallbacks[0](progressEvent({
      waitingForAcquisition: false,
      waitingDevices: [],
      waitingForAcquisitionSinceMs: 0,
    }))
    expect(store.status?.waitingForAcquisition).toBe(false)
    expect(store.status?.waitingDevices).toEqual([])
    expect(store.status?.waitingForAcquisitionSinceMs).toBe(0)
  })

  it('paused 状态下 progress 事件不把 status 回退为 running 且等待字段保留', async () => {
    const store = useTraversalStore()
    const paused = runningWithWaiting()
    paused.status = 'paused'
    paused.state = 'paused'
    mockGetStatus.mockResolvedValueOnce({ success: true, data: paused })
    await store.refreshStatus()

    progressCallbacks[0](progressEvent({ completedPoints: 3 }))
    expect(store.status?.status).toBe('paused')
    expect(store.status?.waitingForAcquisition).toBe(true)
  })
})
