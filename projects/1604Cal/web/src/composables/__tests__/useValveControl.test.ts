import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useValveControl, type ValveStore } from '@/composables/useValveControl'

// 测试桩：可控的 ValveStore，可被任意改写 valveStatus 字段以模拟硬件回读结果。
function makeStore(initial: string, behavior?: {
  setShouldFail?: boolean
  refreshSequence?: string[]
}): ValveStore & {
  setCalls: string[]
  refreshCalls: number
} {
  let valveStatus = initial
  const refreshSequence = behavior?.refreshSequence ?? []
  const store = {
    setCalls: [] as string[],
    refreshCalls: 0,
    get valveStatus() { return valveStatus },
    set valveStatus(v: string) { valveStatus = v },
    setValveStatus: vi.fn(async (status: string) => {
      store.setCalls.push(status)
      if (behavior?.setShouldFail) {
        return { ok: false, error: 'VALVE_REJECTED', detail: 'device rejected valve command w0C01: N09' }
      }
      return { ok: true }
    }),
    refreshValveStatus: vi.fn(async () => {
      const next = refreshSequence[store.refreshCalls] ?? valveStatus
      valveStatus = next
      store.refreshCalls++
    }),
  } as ValveStore & { setCalls: string[]; refreshCalls: number }
  return store
}

// 把 ElMessage 三个方法替换成 spy。
vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
  },
}))

import { ElMessage } from 'element-plus'

describe('useValveControl', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('在第一轮退避就读到目标态时立即弹 success 并提前结束', async () => {
    const store = makeStore('measurement', { refreshSequence: ['calibration'] })
    const { setValve, valvePending } = useValveControl(store, {
      scenario: 'calibration',
      readbackBackoffMs: [1, 1, 1],
    })

    await setValve('calibration')

    expect(store.setCalls).toEqual(['calibration'])
    // 第一轮就读到了目标态，应该只调用一次 refresh。
    expect(store.refreshCalls).toBe(1)
    expect(ElMessage.success).toHaveBeenCalledWith('阀门已切换到校准模式')
    expect(valvePending.value).toBe(false)
  })

  it('前两轮没读到目标态时继续退避，直到第三轮', async () => {
    const store = makeStore('measurement', {
      refreshSequence: ['measurement', 'measurement', 'calibration'],
    })
    const { setValve } = useValveControl(store, {
      scenario: 'calibration',
      readbackBackoffMs: [1, 1, 1],
    })

    await setValve('calibration')

    expect(store.refreshCalls).toBe(3)
    expect(ElMessage.success).toHaveBeenCalledWith('阀门已切换到校准模式')
  })

  it('三轮回读都未达到目标态时弹 warning，提示设备状态不一致', async () => {
    const store = makeStore('measurement', {
      refreshSequence: ['measurement', 'measurement', 'measurement'],
    })
    const { setValve } = useValveControl(store, {
      scenario: 'calibration',
      readbackBackoffMs: [1, 1, 1],
    })

    await setValve('calibration')

    expect(store.refreshCalls).toBe(3)
    expect(ElMessage.warning).toHaveBeenCalledWith('阀门当前仍为测量模式，请检查设备')
  })

  it('回读为空 / unknown 时弹"状态未知"warning，便于用户去现场确认设备', async () => {
    const store = makeStore('measurement', {
      refreshSequence: ['unknown', 'unknown', 'unknown'],
    })
    const { setValve } = useValveControl(store, {
      scenario: 'calibration',
      readbackBackoffMs: [1, 1, 1],
    })

    await setValve('calibration')

    expect(ElMessage.warning).toHaveBeenCalledWith('已下发指令，但阀门状态未知，请确认设备')
  })

  it('store.setValveStatus 返回 ok=false 时直接弹 error 并保留设备拒绝原文（含 N09）', async () => {
    const store = makeStore('measurement', { setShouldFail: true })
    const { setValve } = useValveControl(store, {
      scenario: 'calibration',
      readbackBackoffMs: [1, 1, 1],
    })

    await setValve('calibration')

    // 失败时不应执行回读。
    expect(store.refreshCalls).toBe(0)
    expect(ElMessage.error).toHaveBeenCalledWith(
      'device rejected valve command w0C01: N09'
    )
  })

  it('valvePending 防抖：并发点击只生效一次', async () => {
    const store = makeStore('measurement', { refreshSequence: ['calibration'] })
    const { setValve } = useValveControl(store, {
      scenario: 'calibration',
      readbackBackoffMs: [5],
    })

    await Promise.all([setValve('calibration'), setValve('calibration')])

    expect(store.setCalls).toEqual(['calibration'])
  })

  it('非法 target 直接 no-op，避免误把 unknown / 空串当作合法状态下发', async () => {
    const store = makeStore('calibration')
    const { setValve } = useValveControl(store, {
      scenario: 'calibration',
      readbackBackoffMs: [1],
    })

    // 'unknown' 已在 ValveState union 中（用于回读语义），但不是合法的写入目标，
    // composable 应在入口处过滤掉。
    await setValve('unknown')

    expect(store.setCalls).toEqual([])
    expect(ElMessage.success).not.toHaveBeenCalled()
  })
})
