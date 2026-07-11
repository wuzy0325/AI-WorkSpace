import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useRafThrottle } from '@composables/useRafThrottle'

describe('useRafThrottle', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('markDirty 5 次合并 → callback 调用 1 次', () => {
    const cb = vi.fn()
    const { markDirty } = useRafThrottle(cb)

    markDirty()
    markDirty()
    markDirty()
    markDirty()
    markDirty()

    // 此时 callback 尚未被调用
    expect(cb).toHaveBeenCalledTimes(0)

    // 推进所有定时器（setTimeout 降级模式）
    vi.runAllTimers()

    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('flush 同步执行 pending dirty', () => {
    const cb = vi.fn()
    const { markDirty, flush } = useRafThrottle(cb)

    markDirty()
    // 不推进定时器，直接 flush
    flush()

    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('dispose 后 markDirty 不触发 callback', () => {
    const cb = vi.fn()
    const { markDirty, dispose } = useRafThrottle(cb)

    markDirty()
    dispose()
    vi.runAllTimers()

    expect(cb).toHaveBeenCalledTimes(0)
  })

  it('rAF 回调后 dirty 清零，下次 markDirty 重新调度', () => {
    const cb = vi.fn()
    const { markDirty } = useRafThrottle(cb)

    markDirty()
    vi.runAllTimers()
    expect(cb).toHaveBeenCalledTimes(1)

    // dirty 已清零，再次 markDirty 应该重新调度
    markDirty()
    vi.runAllTimers()
    expect(cb).toHaveBeenCalledTimes(2)
  })

  it('同一帧内 markDirty → flush → markDirty 触发两次 callback', () => {
    // flush 清零 dirty，后续 markDirty 正常触发
    const cb = vi.fn()
    const { markDirty, flush } = useRafThrottle(cb)

    markDirty()
    flush()
    expect(cb).toHaveBeenCalledTimes(1)

    markDirty()
    vi.runAllTimers()
    expect(cb).toHaveBeenCalledTimes(2)
  })

  it('dispose 后再次 markDirty 正常触发（重新创建状态）', () => {
    const cb = vi.fn()
    const { markDirty, dispose } = useRafThrottle(cb)

    markDirty()
    dispose()

    // dispose 后 dirty 被重置，但 composable 调用方应创建新实例
    // 测试 dispose 清理干净：markDirty 后不会被上次的 rAF 触发
    markDirty()
    vi.runAllTimers()

    // dispose 取消了已调度的 rAF，但新 markDirty 会调度新的
    expect(cb).toHaveBeenCalledTimes(1)
  })

  it('markDirty 后未推进时间 → callback 不会触发', () => {
    const cb = vi.fn()
    const { markDirty } = useRafThrottle(cb)

    markDirty()
    expect(cb).not.toHaveBeenCalled()
  })
})
