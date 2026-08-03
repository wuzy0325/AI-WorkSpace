import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Mock wails-adapter:测试走 fetch 分支,不依赖 Wails 运行时
vi.mock('@api/wails-adapter', () => ({
  isWailsAvailable: () => false,
  wailsApi: {},
}))

// Mock traversalApi:calculateRealtime/importPrb 由各测试用例覆盖返回值,
// 其他方法返回 success 占位避免 store 初始化报错
vi.mock('@api/traversalApi', () => ({
  traversalApi: {
    calculateRealtime: vi.fn(),
    importPrb: vi.fn(),
    getConfig: vi.fn(async () => ({ success: true, data: null })),
    getStatus: vi.fn(async () => ({ success: true, data: null })),
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
import type { TraversalRealtimeInput, TraversalTestConfig } from '@shared/types/traversal'

const mockCalculateRealtime = traversalApi.calculateRealtime as ReturnType<typeof vi.fn>
const mockImportPrb = traversalApi.importPrb as ReturnType<typeof vi.fn>

// 构造最小可用五孔插值输入(P1-P5 + Patm + Tatm)
const baseInput: TraversalRealtimeInput = {
  P1: 101325, P2: 101320, P3: 101318, P4: 101322, P5: 101319,
  Patm: 101325, Tatm: 25,
} as unknown as TraversalRealtimeInput

// 构造最小可用 config,probeType=five-hole 走五孔路径
const baseConfig = { probeType: 'five-hole' } as unknown as TraversalTestConfig

/**
 * 通过 store.importPrb 把 hasLoadedInterpolator 切换为 true。
 *
 * store.syncRealtimeInterpolation 内部 guard:hasLoadedInterpolator=false 时
 * 直接置 realtimeResult=null 并 return,不会调用 calculateRealtime。
 * 因此要测试 runRealtimeInterpolation 的失败路径,必须先让 store 进入"已加载"状态,
 * 模拟"前端认为已加载,但后端报错"的状态不一致场景(这正是本次修复要兜底的核心场景)。
 */
async function setupLoadedStore() {
  mockImportPrb.mockResolvedValueOnce({
    success: true,
    data: { validRange: { minAlpha: -10, maxAlpha: 10, minBeta: -10, maxBeta: 10 } },
  })
  const store = useTraversalStore()
  await store.importPrbFile('/fake/path.prb')
  expect(store.hasLoadedInterpolator).toBe(true)
  return store
}

// 等待节流定时器(5Hz → 200ms)+ await 链完成,留 500ms 余量
function flushThrottle(): Promise<void> {
  return new Promise((r) => setTimeout(r, 500))
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('traversalStore.runRealtimeInterpolation 失败路径', () => {
  // ===== 场景 1:HTTP 400(PRB 未加载)→ 构造 IsValid=false 占位对象,防止 UI 吞提示 =====
  //
  // 测试前置:
  //   - store.hasLoadedInterpolator = true(模拟"前端认为已加载,后端实际未加载"的不一致)
  //   - mock calculateRealtime 返回 { success: false, error: 'PRB interpolation data is not loaded' }
  //
  // 测试步骤:
  //   - 调用 store.syncRealtimeInterpolation(baseInput, baseConfig)
  //   - 等待节流定时器 + await 链完成
  //
  // 期待结果:
  //   - store.realtimeResult 不为 null(旧实现会置 null,导致 UI 三态吞提示)
  //   - realtimeResult.isValid === false
  //   - realtimeResult.warning === 'PRB interpolation data is not loaded'
  //   - 数值字段全为 0(与后端 PrbMissing/Invalid 零值契约一致)
  it('HTTP 400 (PRB 未加载) 构造 IsValid=false 占位对象,不吞 null', async () => {
    mockCalculateRealtime.mockResolvedValueOnce({
      success: false,
      error: 'PRB interpolation data is not loaded',
    })

    const store = await setupLoadedStore()
    store.syncRealtimeInterpolation(baseInput, baseConfig)
    await flushThrottle()

    // 核心断言:不被吞成 null
    expect(store.realtimeResult).not.toBeNull()
    expect(store.realtimeResult?.isValid).toBe(false)
    expect(store.realtimeResult?.warning).toBe('PRB interpolation data is not loaded')
    // 数值字段为零值,与后端 PrbMissing 契约一致
    expect(store.realtimeResult?.alpha).toBe(0)
    expect(store.realtimeResult?.beta).toBe(0)
    expect(store.realtimeResult?.machNumber).toBe(0)
    expect(store.realtimeResult?.velocity).toBe(0)
    expect(store.realtimeResult?.dynamicPressure).toBe(0)
  })

  // ===== 场景 2:网络失败 → 同样构造占位对象,warning 携带网络错误文案 =====
  //
  // 测试前置:hasLoadedInterpolator=true
  // 测试步骤:mock calculateRealtime 返回网络错误文案
  // 期待结果:realtimeResult.isValid=false + warning=网络错误文案
  it('网络失败 构造 IsValid=false 占位对象,warning 携带网络错误文案', async () => {
    mockCalculateRealtime.mockResolvedValueOnce({
      success: false,
      error: '网络连接失败,请检查后端服务是否已启动',
    })

    const store = await setupLoadedStore()
    store.syncRealtimeInterpolation(baseInput, baseConfig)
    await flushThrottle()

    expect(store.realtimeResult).not.toBeNull()
    expect(store.realtimeResult?.isValid).toBe(false)
    expect(store.realtimeResult?.warning).toBe('网络连接失败,请检查后端服务是否已启动')
  })

  // ===== 场景 3:HTTP 200 + IsValid=true → 透传后端返回的真实结果(成功路径回归保护) =====
  //
  // 测试前置:hasLoadedInterpolator=true
  // 测试步骤:mock calculateRealtime 返回 success + 真实 InterpolationResult
  // 期待结果:realtimeResult 透传,数值字段不为零
  it('HTTP 200 + IsValid=true 透传后端真实结果(成功路径回归保护)', async () => {
    mockCalculateRealtime.mockResolvedValueOnce({
      success: true,
      data: {
        isValid: true,
        alpha: 15.32,
        beta: -2.10,
        machNumber: 0.325,
        velocity: 110.5,
        dynamicPressure: 1234.56,
      },
    })

    const store = await setupLoadedStore()
    store.syncRealtimeInterpolation(baseInput, baseConfig)
    await flushThrottle()

    expect(store.realtimeResult).not.toBeNull()
    expect(store.realtimeResult?.isValid).toBe(true)
    expect(store.realtimeResult?.alpha).toBe(15.32)
    expect(store.realtimeResult?.beta).toBe(-2.10)
    expect(store.realtimeResult?.machNumber).toBe(0.325)
  })

  // ===== 场景 4:HTTP 200 + IsValid=false(数据层失败)→ 透传后端 IsValid=false =====
  //
  // 测试前置:hasLoadedInterpolator=true
  // 测试步骤:mock 返回 success + { isValid: false, warning: '压力越界' }
  // 期待结果:realtimeResult 透传,isValid=false,warning=压力越界
  it('HTTP 200 + IsValid=false 透传后端数据层失败结果', async () => {
    mockCalculateRealtime.mockResolvedValueOnce({
      success: true,
      data: {
        isValid: false,
        warning: '压力差值越界',
        alpha: 0, beta: 0, machNumber: 0, velocity: 0, dynamicPressure: 0,
      },
    })

    const store = await setupLoadedStore()
    store.syncRealtimeInterpolation(baseInput, baseConfig)
    await flushThrottle()

    expect(store.realtimeResult).not.toBeNull()
    expect(store.realtimeResult?.isValid).toBe(false)
    expect(store.realtimeResult?.warning).toBe('压力差值越界')
  })
})
