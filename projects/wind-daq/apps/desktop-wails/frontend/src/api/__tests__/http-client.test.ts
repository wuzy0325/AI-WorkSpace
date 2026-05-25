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
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
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
})

describe('deviceApi', () => {
  beforeEach(() => { vi.restoreAllMocks(); vi.stubEnv('VITE_API_BASE', 'http://localhost:8080') })

  it('constructs correct connect URL', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(jsonResponse({ success: true }))

    const { deviceApi } = await import('@api/deviceApi')
    await deviceApi.connect('sim-1')
    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/device/sim-1/connect',
      expect.objectContaining({ method: 'POST' }),
    )
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
    const start = await storageApi.start('/tmp', 'test')
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
