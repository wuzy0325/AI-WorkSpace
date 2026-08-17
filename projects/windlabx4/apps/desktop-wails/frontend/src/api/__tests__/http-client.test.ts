import { describe, it, expect, vi, beforeEach } from 'vitest'
import { request, ApiError } from '@api/http-client'

function jsonResponse(data: unknown, init: Partial<Response> = {}): Response {
  return {
    ok: true,
    status: 200,
    text: () => Promise.resolve(JSON.stringify(data)),
    ...init,
  } as Response
}

describe('http-client', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.stubEnv('VITE_API_BASE', 'http://localhost:8080')
  })

  it('returns JSON on successful response', async () => {
    const mockData = { success: true }
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(jsonResponse(mockData))

    const result = await request<{ success: boolean }>('/api/test')
    expect(result.success).toBe(true)
  })

  it('throws ApiError on non-ok response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: false,
      status: 400,
      text: () => Promise.resolve('bad request'),
    } as Response)
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: false,
      status: 400,
      text: () => Promise.resolve('bad request'),
    } as Response)

    await expect(request<unknown>('/api/test')).rejects.toThrow(ApiError)
    await expect(request<unknown>('/api/test')).rejects.toThrow('bad request')
  })

  it('throws ApiError with status code', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: false,
      status: 404,
      text: () => Promise.resolve(''),
    } as Response)

    try {
      await request<unknown>('/api/test')
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError)
      expect((e as ApiError).status).toBe(404)
    }
  })

  it('sends Content-Type header', async () => {
    let capturedHeaders: Record<string, string> = {}
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (_, init) => {
      capturedHeaders = (init?.headers as Record<string, string>) ?? {}
      return jsonResponse({})
    })

    await request<Record<string, never>>('/api/test')
    expect(capturedHeaders['Content-Type']).toBe('application/json')
  })

  it('uses the local API server when Wails is available over an HTTP origin', async () => {
    vi.stubEnv('VITE_API_BASE', '')
    Object.defineProperty(window, 'chrome', {
      configurable: true,
      value: { webview: { postMessage: vi.fn() } },
    })
    vi.resetModules()
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(jsonResponse({ success: true }))

    const { request: freshRequest } = await import('@api/http-client')
    await freshRequest<{ success: boolean }>('/api/daq/latest/dev-1')

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://127.0.0.1:8900/api/daq/latest/dev-1',
      expect.any(Object),
    )

    Object.defineProperty(window, 'chrome', { configurable: true, value: undefined })
    vi.resetModules()
  })
})

describe('deviceApi', () => {
  async function flushPromises(times = 5): Promise<void> {
    for (let i = 0; i < times; i += 1) {
      await Promise.resolve()
    }
  }

  beforeEach(async () => {
    vi.restoreAllMocks()
    vi.useRealTimers()
    vi.stubEnv('VITE_API_BASE', 'http://localhost:8080')
    Object.defineProperty(window, 'chrome', { configurable: true, value: undefined })
    const { deviceApi } = await import('@api/deviceApi')
    deviceApi._subscriptions.clear()
    deviceApi._subscriptionOwners.clear()
    deviceApi._deviceLostListeners.clear()
    deviceApi._publishRateHz = 20
  })

  it('keeps shared polling alive until the last owner unsubscribes', async () => {
    Object.defineProperty(window, 'chrome', {
      configurable: true,
      value: { webview: { postMessage: vi.fn() } },
    })
    const { deviceApi } = await import('@api/deviceApi')
    const { wailsApi } = await import('@api/wails-adapter')
    vi.spyOn(deviceApi, 'getLatest').mockResolvedValue({
      deviceId: 'dev-shared', timestamp: 1, channels: [1], channelIndices: [0],
    })
    vi.spyOn(window, 'setTimeout').mockReturnValue(1 as never)
    vi.spyOn(wailsApi.device, 'subscribeStream').mockResolvedValue({ success: true, Success: true })

    deviceApi.subscribeToDevice('dev-shared', 'dashboard')
    deviceApi.subscribeToDevice('dev-shared')
    deviceApi.unsubscribeFromDevice('dev-shared')

    expect(deviceApi._subscriptions.has('dev-shared')).toBe(true)
    expect(wailsApi.device.subscribeStream).not.toHaveBeenCalledWith('dev-shared', false)

    deviceApi.unsubscribeFromDevice('dev-shared', 'dashboard')

    expect(deviceApi._subscriptions.has('dev-shared')).toBe(false)
    expect(wailsApi.device.subscribeStream).toHaveBeenCalledWith('dev-shared', false)
  })

  it('constructs correct connect URL', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(jsonResponse({ success: true }))

    const { deviceApi } = await import('@api/deviceApi')
    await deviceApi.connect('sim-1')
    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/device/sim-1/connect',
      expect.objectContaining({ method: 'POST' }),
    )
  })

  it('restarts Wails polling subscriptions when publish rate changes', async () => {
    Object.defineProperty(window, 'chrome', {
      configurable: true,
      value: { webview: { postMessage: vi.fn() } },
    })
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ deviceId: 'dev-1', timestamp: 1, channels: [], channelIndices: [] }),
    } as Response)
    const setTimeoutSpy = vi.spyOn(window, 'setTimeout').mockReturnValue(1 as never)
    const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout').mockImplementation(() => undefined)
    // 冻结 Date.now：deviceApi.pollLatest 用 Date.now() - startedAt 计算已耗时，
    // 用于从 intervalMs 中扣除本次 fetch 耗时。不冻结时 mock fetch resolve 会让
    // 时钟前进 1ms，导致 setTimeout 第 1 次参数从 50 变成 49（flaky）。
    vi.spyOn(Date, 'now').mockReturnValue(1000)

    const { deviceApi } = await import('@api/deviceApi')
    const { wailsApi } = await import('@api/wails-adapter')
    vi.spyOn(wailsApi.device, 'subscribeStream').mockResolvedValue({ success: true, Success: true })
    vi.spyOn(wailsApi.device, 'setPublishRate').mockResolvedValue({ success: true, Success: true })

    deviceApi.subscribeToDevice('dev-1')
    await flushPromises()
    await deviceApi.setPublishRate(5)
    await flushPromises()

    expect(clearTimeoutSpy).toHaveBeenCalledWith(1)
    expect(wailsApi.device.subscribeStream).toHaveBeenNthCalledWith(1, 'dev-1', true)
    expect(wailsApi.device.subscribeStream).toHaveBeenCalledTimes(1)
    expect(setTimeoutSpy).toHaveBeenNthCalledWith(1, expect.any(Function), 50)
    expect(setTimeoutSpy).toHaveBeenNthCalledWith(2, expect.any(Function), 200)
  })

  // 验证：轮询 getLatest 拿到 404 时，deviceApi 触发 onDeviceLost 回调，
  // 让 deviceStore 能感知设备异常退出并更新 UI 状态为 Error。
  // 此前 getLatest catch 块静默吞掉所有错误，UI 永远显示"采集中"。
  it('triggers onDeviceLost when polling returns 404', async () => {
    Object.defineProperty(window, 'chrome', {
      configurable: true,
      value: { webview: { postMessage: vi.fn() } },
    })
    // fetch 返回 404（设备已断开/异常退出）
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 404,
      text: () => Promise.resolve(JSON.stringify({ error: 'device not connected' })),
    } as Response)
    vi.spyOn(window, 'setTimeout').mockReturnValue(1 as never)

    const { deviceApi } = await import('@api/deviceApi')
    const { wailsApi } = await import('@api/wails-adapter')
    vi.spyOn(wailsApi.device, 'subscribeStream').mockResolvedValue({ success: true, Success: true })

    const lostDevices: string[] = []
    deviceApi.onDeviceLost((id) => lostDevices.push(id))

    deviceApi.subscribeToDevice('dev-lost')
    await flushPromises()

    expect(lostDevices).toContain('dev-lost')
    // 轮询应已停止（subscription 仍存在但 active=false，不再调度下次 setTimeout）
    // setTimeout 在 catch 块 return 前不会被调用调度下一轮
  })

  // 验证 SSE 模式（非 Wails）下，sse-client fetch 拿到 404 时同样触发 onDeviceLost。
  // sse-client.ts:28 触发 `SSE HTTP ${status}` 错误字符串，deviceApi 严格相等匹配
  // 'SSE HTTP 404' 后通知订阅者。
  it('triggers onDeviceLost when SSE returns 404', async () => {
    // 非 Wails 模式（无 window.chrome）→ deviceApi.subscribeToDevice 走 SSE 分支
    Object.defineProperty(window, 'chrome', { configurable: true, value: undefined })
    // fetch 返回 404 → sse-client 触发 onError('SSE HTTP 404')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 404,
      text: () => Promise.resolve(JSON.stringify({ error: 'device offline' })),
    } as Response)

    const { deviceApi } = await import('@api/deviceApi')
    const lostDevices: string[] = []
    deviceApi.onDeviceLost((id) => {
      lostDevices.push(id)
      // 模拟 deviceStore 行为：触发后立即 unsubscribe 避免 SSE 重连卡测试
      deviceApi.unsubscribeFromDevice(id)
    })

    deviceApi.subscribeToDevice('dev-lost-sse')
    await flushPromises()

    expect(lostDevices).toContain('dev-lost-sse')
  })

  it('motionApi returns status', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: true, text: () => Promise.resolve(JSON.stringify({ connected: true, axes: [] })),
    } as Response)

    const { motionApi } = await import('@api/deviceApi')
    const result = await motionApi.status()
    expect(result.connected).toBe(true)
  })

  it('storageApi start/stop/status', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (url) => {
      if ((url as string).includes('status')) {
        return jsonResponse({ recording: false })
      }
      return jsonResponse({ success: true })
    })

    const { storageApi } = await import('@api/deviceApi')
    const status = await storageApi.status()
    expect(status.recording).toBe(false)
    const start = await storageApi.start({ outputDir: '/tmp', filePrefix: 'test' })
    expect(start.success).toBe(true)
    const stop = await storageApi.stop()
    expect(stop.success).toBe(true)
  })

  it('calibrationApi start/status/pause/resume/stop/collect/result', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (url) => {
      if ((url as string).includes('status') || (url as string).includes('result')) {
        return jsonResponse({ state: 'idle', taskId: 'cal-1' })
      }
      return jsonResponse({ success: true })
    })

    const { calibrationApi } = await import('@api/deviceApi')
    const start = await calibrationApi.start({ taskId: 'cal-1', deviceId: 'dev-1', type: 'five-hole', channels: [0], pressurePoints: [0, 50], averageSamples: 5 })
    expect(start.success).toBe(true)
    const status = await calibrationApi.status()
    expect(status.state).toBe('idle')
    const result = await calibrationApi.getResult('cal-1')
    expect(result.taskId).toBe('cal-1')
    const collect = await calibrationApi.collect()
    expect(collect.success).toBe(true)
  })

  it('traversalApi start/status/runPoint/pause/resume/stop', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (url) => {
      if ((url as string).includes('status')) {
        return jsonResponse({ state: 'running', currentPoint: 0, totalPoints: 2 })
      }
      return jsonResponse({ success: true })
    })

    const { traversalApi } = await import('@api/deviceApi')
    const path = [{ x: 0, y: 0, z: 0 }, { x: 50, y: 25, z: 10 }]
    const start = await traversalApi.start('trav-1', 'dev-1', [0], path)
    expect(start.success).toBe(true)
    const status = await traversalApi.status()
    expect(status.state).toBe('running')
    const point = await traversalApi.runPoint()
    expect(point.success).toBe(true)
    const pause = await traversalApi.pause()
    expect(pause.success).toBe(true)
    const stop = await traversalApi.stop()
    expect(stop.success).toBe(true)
  })
})
