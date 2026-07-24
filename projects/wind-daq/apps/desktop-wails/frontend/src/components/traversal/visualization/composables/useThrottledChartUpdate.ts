import { onBeforeUnmount, watch, type WatchSource } from 'vue'

/**
 * 图表更新 rAF 节流
 *
 * 背景：traversalStore 的 dataPoints 既被整组覆盖（status 推送）又被增量推送
 * （latestData 事件），高频推送下若每次都 setOption 全量重绘会卡顿。
 * 此 composable 用 requestAnimationFrame 合并同一帧内的多次触发，确保
 * 一帧只重绘一次（≈ 60Hz 上限），与 §27 第 5 条"渲染节流"精神一致。
 *
 * 使用方式：
 *   useThrottledChartUpdate([chart, dataPoints, chartTheme], updateChart)
 *
 * 注意：组件 unmount 时取消挂起的 rAF，避免 setOption 操作已 dispose 的实例。
 */
export function useThrottledChartUpdate(
  sources: WatchSource[],
  update: () => void,
  options: { immediate?: boolean } = {}
): void {
  let rafId: number | null = null

  function schedule(): void {
    // 已有挂起的 rAF 则跳过，合并到同一帧
    if (rafId !== null) return
    rafId = requestAnimationFrame(() => {
      rafId = null
      update()
    })
  }

  watch(sources, schedule, { immediate: options.immediate })

  onBeforeUnmount(() => {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
  })
}
