import { onBeforeUnmount, onMounted, shallowRef, type Ref } from 'vue'
// 按需导入 echarts —— 禁止使用 `import * as echarts from 'echarts'`，那会拉入整包（1.2 MB raw）。
// 仅注册 traversal 可视化（CrossSection / Heatmap / PressureRadar / VectorField）实际用到的模块。
//
// 如果未来新增图表类型/组件，请：
//   1. 在 `use([...])` 列表里追加注册；
//   2. 漏注册会运行时报 "series type ... is not registered"。
import { init, use, type ECharts } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { CustomChart, HeatmapChart, LineChart, RadarChart } from 'echarts/charts'
import {
  GridComponent,
  TitleComponent,
  TooltipComponent,
  VisualMapComponent,
} from 'echarts/components'

// echarts 注册是全局 side-effect（不是 ECharts 实例属性），重复 use() 是 no-op，
// 所以放在模块顶层只执行一次。
use([
  CanvasRenderer,
  // charts
  LineChart,
  HeatmapChart,
  RadarChart,
  CustomChart,
  // components
  GridComponent,
  TooltipComponent,
  TitleComponent,
  VisualMapComponent,
])

export function useECharts(chartRef: Ref<HTMLElement | null>) {
  const chart = shallowRef<ECharts | null>(null)

  function resize(): void {
    chart.value?.resize()
  }

  onMounted(() => {
    if (!chartRef.value) return
    chart.value = init(chartRef.value)
    window.addEventListener('resize', resize)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', resize)
    chart.value?.dispose()
    chart.value = null
  })

  return { chart, resize }
}
