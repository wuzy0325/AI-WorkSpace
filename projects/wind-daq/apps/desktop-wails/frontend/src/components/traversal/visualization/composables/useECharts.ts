import { onBeforeUnmount, onMounted, shallowRef, type Ref } from 'vue'
import * as echarts from 'echarts'
import type { ECharts } from 'echarts'

export function useECharts(chartRef: Ref<HTMLElement | null>) {
  const chart = shallowRef<ECharts | null>(null)

  function resize(): void {
    chart.value?.resize()
  }

  onMounted(() => {
    if (!chartRef.value) return
    chart.value = echarts.init(chartRef.value)
    window.addEventListener('resize', resize)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', resize)
    chart.value?.dispose()
    chart.value = null
  })

  return { chart, resize }
}

