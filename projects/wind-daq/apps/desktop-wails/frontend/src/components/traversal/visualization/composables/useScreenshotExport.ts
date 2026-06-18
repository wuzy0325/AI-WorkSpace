import type { ECharts } from 'echarts'
import type { ShallowRef } from 'vue'

/**
 * 图表截图导出 composable
 * 基于 ECharts 的 getDataURL 能力，将当前图表导出为 PNG 文件
 * 通过创建临时 <a> 标签触发浏览器下载，无需后端参与
 */
export function useScreenshotExport(chart: ShallowRef<ECharts | null>) {
  /**
   * 导出当前图表为 PNG
   * @param filename 下载文件名（不含扩展名）
   * @param pixelRatio 像素比，默认 2 倍以获得清晰输出
   */
  function exportScreenshot(filename: string, pixelRatio = 2): void {
    const instance = chart.value
    if (!instance) return

    const dataUrl = instance.getDataURL({
      type: 'png',
      pixelRatio,
      backgroundColor: '#fff'
    })

    const link = document.createElement('a')
    link.download = `${filename}.png`
    link.href = dataUrl
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  }

  return { exportScreenshot }
}
