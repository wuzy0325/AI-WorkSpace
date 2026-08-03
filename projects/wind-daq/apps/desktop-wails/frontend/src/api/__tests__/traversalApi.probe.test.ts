import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Mock wails-adapter：测试走 fetch 分支，不依赖 Wails 运行时
vi.mock('@api/wails-adapter', () => ({
  isWailsAvailable: () => false,
  wailsApi: {},
}))

import { traversalProbeApi } from '@api/traversalApi'
import { probePollingSnapshot, resetProbePolling } from '@api/traversalPolling'
import type { TraversalTestStatus } from '@shared/types/traversal'

// ---------------------------------------------------------------------------
// fake fetch：记录全部请求（URL/method/body），可编程响应
// ---------------------------------------------------------------------------

type FetchCall = { url: string; method: string; body?: string; signal?: AbortSignal | null }
const fetchCalls: FetchCall[] = []
let fetchResponder: (url: string, init?: RequestInit) => Response | Promise<Response>

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function useJsonResponder(body: unknown, status = 200): void {
  fetchResponder = () => jsonResponse(body, status)
}

function lastCall(): FetchCall {
  return fetchCalls[fetchCalls.length - 1]
}

function runningStatus(taskId: string): TraversalTestStatus {
  return {
    taskId,
    status: 'running',
    totalPoints: 3,
    completedPoints: 1,
  } as TraversalTestStatus
}

beforeEach(() => {
  fetchCalls.length = 0
  useJsonResponder({})
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    fetchCalls.push({ url, method: init?.method ?? 'GET', body: init?.body as string | undefined, signal: init?.signal })
    return fetchResponder(url, init)
  }))
  resetProbePolling()
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

// ---------------------------------------------------------------------------
// probe-aware 函数：两段路由与请求体
// ---------------------------------------------------------------------------

describe('traversalProbeApi 路由', () => {
  it('全套函数命中 /api/traversal/{probeId}/{action} 两段路由', async () => {
    useJsonResponder({})
    await traversalProbeApi.getConfig('probe1')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/config')
    expect(lastCall().method).toBe('GET')

    await traversalProbeApi.saveConfig('probe2', { probeType: 'five-hole' } as never)
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe2/config')
    expect(lastCall().method).toBe('POST')

    await traversalProbeApi.importPrb('probe1', 'D:/a.prb')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/importPrb')
    expect(JSON.parse(lastCall().body!)).toEqual({ filePath: 'D:/a.prb' })

    await traversalProbeApi.importCalibrationCsv('probe2', 'D:/a.csv')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe2/importCalibrationCsv')

    await traversalProbeApi.importMultiPrb('probe1', ['a.prb'], [0.3], 'linear')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/importMultiPrb')

    await traversalProbeApi.importSevenHolePrb('probe2', '7.prb', ['1.prb'])
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe2/importSevenHolePrb')

    await traversalProbeApi.importSevenHoleCalibrationCsv('probe1', '7.csv', ['1.csv'])
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/importSevenHoleCalibrationCsv')

    await traversalProbeApi.clearInterpolator('probe2', 'seven-hole')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe2/clearInterpolator')
    expect(JSON.parse(lastCall().body!)).toEqual({ probeType: 'seven-hole' })

    await traversalProbeApi.calculateRealtime('probe1', {} as never, undefined, 'seven-hole')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/calculateRealtime')

    await traversalProbeApi.checkPreconditions('probe2')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe2/checkPreconditions')

    await traversalProbeApi.start('probe1', { taskId: 'client-x' } as never)
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/start')

    for (const action of ['pause', 'resume', 'stop', 'runPoint', 'close'] as const) {
      await traversalProbeApi[action]('probe2')
      expect(lastCall().url).toBe(`http://localhost:8080/api/traversal/probe2/${action}`)
      expect(lastCall().method).toBe('POST')
    }

    await traversalProbeApi.getStatus('probe1')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/status')

    await traversalProbeApi.getResult('probe2', 'task-1')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe2/result?taskId=task-1')

    await traversalProbeApi.loadCheckpoint('probe1')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/loadCheckpoint')

    // resumeFromCheckpoint / clearCheckpoint 请求体只携带 taskId（FR4）
    await traversalProbeApi.resumeFromCheckpoint('probe2', 'probe2-task-9')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe2/resumeFromCheckpoint')
    expect(JSON.parse(lastCall().body!)).toEqual({ taskId: 'probe2-task-9' })

    await traversalProbeApi.clearCheckpoint('probe1', 'probe1-task-8')
    expect(lastCall().url).toBe('http://localhost:8080/api/traversal/probe1/clearCheckpoint')
    expect(JSON.parse(lastCall().body!)).toEqual({ taskId: 'probe1-task-8' })
  })

  it('getStatus 复用 legacy 状态映射（currentPoint 兼容数字/对象）', async () => {
    useJsonResponder({ taskId: 't', status: 'running', currentPoint: 2, currentPointCoordinates: { alpha: 1, beta: 2 } })
    const res = await traversalProbeApi.getStatus('probe1')
    expect(res.success).toBe(true)
    expect(res.data?.currentPoint).toEqual({ alpha: 1, beta: 2 })
  })

  it('HTTP 错误统一收敛为 success=false + error', async () => {
    useJsonResponder({ success: false, error: 'resource_conflict: x' }, 409)
    const res = await traversalProbeApi.start('probe1', {} as never)
    expect(res.success).toBe(false)
    expect(res.error).toContain('resource_conflict')
  })
})

// ---------------------------------------------------------------------------
// keyed polling（spec FR6）
// ---------------------------------------------------------------------------

describe('keyed polling', () => {
  it('首个 subscriber 立即请求一次，之后按 500ms 调度', async () => {
    useJsonResponder(runningStatus('t1'))
    const events: number[] = []
    traversalProbeApi.onProgress('probe1', () => events.push(Date.now()))
    // 首个 subscriber 立即请求（microtask 后到达）
    await vi.advanceTimersByTimeAsync(0)
    expect(fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe1/status')).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(500)
    expect(fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe1/status')).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(500)
    expect(fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe1/status')).toHaveLength(3)
    expect(events.length).toBeGreaterThan(0)
  })

  it('同时最多一个 in-flight status 请求', async () => {
    // fetch 永不 resolve：in-flight 卡住
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      fetchCalls.push({ url, method: 'GET' })
      return new Promise<Response>(() => {})
    }))
    traversalProbeApi.onProgress('probe1', () => {})
    await vi.advanceTimersByTimeAsync(0)
    expect(probePollingSnapshot('probe1').inFlight).toBe(true)
    // 三个 tick 过去：不得叠加请求
    await vi.advanceTimersByTimeAsync(1500)
    expect(fetchCalls).toHaveLength(1)
  })

  it('最后一个 subscriber 注销停止该 probe timer，另一路不受影响', async () => {
    useJsonResponder(runningStatus('t'))
    const unsub1 = traversalProbeApi.onProgress('probe1', () => {})
    const unsub1b = traversalProbeApi.onProgress('probe1', () => {})
    const unsub2 = traversalProbeApi.onProgress('probe2', () => {})
    await vi.advanceTimersByTimeAsync(0)
    expect(probePollingSnapshot('probe1').timerRunning).toBe(true)
    expect(probePollingSnapshot('probe2').timerRunning).toBe(true)

    unsub1()
    expect(probePollingSnapshot('probe1').timerRunning).toBe(true) // 仍有订阅者
    unsub1b()
    expect(probePollingSnapshot('probe1').timerRunning).toBe(false) // 最后一个注销停止
    expect(probePollingSnapshot('probe2').timerRunning).toBe(true) // 另一路不受影响

    const probe2CallsBefore = fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe2/status').length
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe1/status')).toHaveLength(1) // 不再轮询
    expect(fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe2/status').length).toBeGreaterThan(probe2CallsBefore)
    unsub2()
  })

  it('注销后到达的旧响应被丢弃（AbortController + generation）', async () => {
    let resolveFetch!: (r: Response) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((resolve) => { resolveFetch = resolve })))
    const events: unknown[] = []
    const unsub = traversalProbeApi.onProgress('probe1', (e) => events.push(e))
    await vi.advanceTimersByTimeAsync(0)
    // 请求在途时注销：响应到达后不得分发
    unsub()
    resolveFetch(jsonResponse(runningStatus('t1')))
    await vi.advanceTimersByTimeAsync(0)
    expect(events).toHaveLength(0)
    expect(probePollingSnapshot('probe1').timerRunning).toBe(false)
  })

  it('注销最后一个 subscriber 会中止实际 status fetch', async () => {
    let requestSignal: AbortSignal | null | undefined
    vi.stubGlobal('fetch', vi.fn((_url: string, init?: RequestInit) => {
      requestSignal = init?.signal
      return new Promise<Response>((_resolve, reject) => {
        requestSignal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
      })
    }))
    const unsub = traversalProbeApi.onProgress('probe1', () => {})
    await vi.advanceTimersByTimeAsync(0)

    expect(requestSignal?.aborted).toBe(false)
    unsub()
    expect(requestSignal?.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(0)
    expect(probePollingSnapshot('probe1').inFlight).toBe(false)
  })

  it('停止一路 polling 不停止另一路（channel 完全隔离）', async () => {
    useJsonResponder(runningStatus('t'))
    const p1Events: string[] = []
    const p2Events: string[] = []
    const unsub1 = traversalProbeApi.onProgress('probe1', () => p1Events.push('x'))
    traversalProbeApi.onProgress('probe2', () => p2Events.push('y'))
    await vi.advanceTimersByTimeAsync(500)
    unsub1()
    const p1After = fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe1/status').length
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchCalls.filter((c) => c.url === 'http://localhost:8080/api/traversal/probe1/status')).toHaveLength(p1After)
    expect(p2Events.length).toBeGreaterThan(0)
    expect(probePollingSnapshot('probe1').subscribers).toBe(0)
    expect(probePollingSnapshot('probe2').subscribers).toBe(1)
  })

  it('in-flight 请求完成后恢复调度', async () => {
    let resolveFetch!: (r: Response) => void
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      fetchCalls.push({ url, method: 'GET' })
      return new Promise<Response>((resolve) => { resolveFetch = resolve })
    }))
    traversalProbeApi.onProgress('probe1', () => {})
    await vi.advanceTimersByTimeAsync(0)
    expect(probePollingSnapshot('probe1').inFlight).toBe(true)
    resolveFetch(jsonResponse(runningStatus('t1')))
    await vi.advanceTimersByTimeAsync(0)
    expect(probePollingSnapshot('probe1').inFlight).toBe(false)
    // 下一个 tick 正常发起
    resolveFetch = () => {}
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      fetchCalls.push({ url, method: 'GET' })
      return Promise.resolve(jsonResponse(runningStatus('t1')))
    }))
    await vi.advanceTimersByTimeAsync(500)
    expect(fetchCalls).toHaveLength(2)
  })
})
