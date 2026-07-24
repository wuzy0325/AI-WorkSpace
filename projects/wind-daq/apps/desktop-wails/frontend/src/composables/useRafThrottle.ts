// rAF 合并层：同一帧内多次 markDirty 合并为一次 callback 调用。
// 用于实时波形图 setOption 节流，防止后端突发补帧压垮 ECharts 渲染。
export interface RafThrottle {
  /** 标记脏状态，调度下一帧执行 callback */
  markDirty(): void
  /** 同步执行 pending callback（跳过 rAF 调度） */
  flush(): void
  /** 取消未执行的 rAF 回调并清理状态 */
  dispose(): void
}

/** 当前环境是否支持 requestAnimationFrame */
function hasRaf(): boolean {
  return typeof requestAnimationFrame === 'function' && typeof cancelAnimationFrame === 'function'
}

export function useRafThrottle(callback: () => void): RafThrottle {
  let dirty = false
  let rafId: number | null = null

  function flush(): void {
    if (rafId !== null) {
      if (hasRaf()) {
        cancelAnimationFrame(rafId)
      } else {
        clearTimeout(rafId)
      }
      rafId = null
    }
    if (!dirty) return
    dirty = false
    callback()
  }

  function scheduleFlush(): void {
    if (hasRaf()) {
      rafId = requestAnimationFrame(() => {
        rafId = null
        if (!dirty) return
        dirty = false
        callback()
      })
    } else {
      // jsdom / 无 rAF 环境降级为 setTimeout
      rafId = window.setTimeout(() => {
        rafId = null
        if (!dirty) return
        dirty = false
        callback()
      }, 0) as unknown as number
    }
  }

  function markDirty(): void {
    dirty = true
    if (rafId === null) {
      scheduleFlush()
    }
  }

  function dispose(): void {
    if (rafId !== null) {
      if (hasRaf()) {
        cancelAnimationFrame(rafId)
      } else {
        clearTimeout(rafId)
      }
      rafId = null
    }
    dirty = false
  }

  return { markDirty, flush, dispose }
}
