export const CHART_COLORS = ['#ef4444', '#f97316', '#eab308', '#22c55e', '#14b8a6', '#06b6d4', '#3b82f6', '#8b5cf6', '#ec4899', '#f43f5e']

export function setupCanvas(canvas: HTMLCanvasElement): CanvasRenderingContext2D | null {
  const rect = canvas.getBoundingClientRect()
  canvas.width = rect.width * window.devicePixelRatio
  canvas.height = rect.height * window.devicePixelRatio
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.scale(window.devicePixelRatio, window.devicePixelRatio)
  return ctx
}

export function drawFiveHoleChartScaffold(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  padding: number,
  xLabel: string,
  yLabel: string,
  showInteriorGrid = true,
  fillBackground = true
): void {
  ctx.save()

  if (fillBackground) {
    ctx.fillStyle = 'rgba(128,128,128,0.03)'
    ctx.fillRect(0, 0, width, height)
  }

  // 坐标轴 — 使用主题变量，确保在深浅主题下都有良好对比度
  ctx.strokeStyle = 'var(--border-default, rgba(100,116,139,0.3))'
  ctx.lineWidth = 1

  // X轴
  ctx.beginPath()
  ctx.moveTo(padding, height - padding)
  ctx.lineTo(width - padding, height - padding)
  ctx.stroke()

  // Y轴
  ctx.beginPath()
  ctx.moveTo(padding, padding)
  ctx.lineTo(padding, height - padding)
  ctx.stroke()

  // 内部网格 — 更细腻的虚线网格，提升可读性
  if (showInteriorGrid) {
    ctx.strokeStyle = 'var(--border-default, rgba(100,116,139,0.08))'
    ctx.lineWidth = 0.5
    ctx.setLineDash([3, 3])
    const gridCount = 4
    for (let i = 1; i < gridCount; i++) {
      const x = padding + ((width - 2 * padding) * i) / gridCount
      ctx.beginPath()
      ctx.moveTo(x, padding)
      ctx.lineTo(x, height - padding)
      ctx.stroke()

      const y = padding + ((height - 2 * padding) * i) / gridCount
      ctx.beginPath()
      ctx.moveTo(padding, y)
      ctx.lineTo(width - padding, y)
      ctx.stroke()
    }
    ctx.setLineDash([])
  }

  // 轴标签 — 使用主题变量
  ctx.fillStyle = 'var(--text-muted, #64748b)'
  ctx.font = '11px sans-serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'top'
  ctx.fillText(xLabel, width / 2, height - padding + 20)

  ctx.save()
  ctx.translate(14, height / 2)
  ctx.rotate(-Math.PI / 2)
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(yLabel, 0, 0)
  ctx.restore()

  ctx.restore()
}

export function resolveKAlphaKbetaBounds(
  data: { x: number; y: number }[]
): { xMin: number; xMax: number; yMin: number; yMax: number; tickCount: number } {
  if (data.length === 0) {
    return { xMin: -1, xMax: 1, yMin: -1, yMax: 1, tickCount: 4 }
  }

  const xs = data.map((d) => d.x)
  const ys = data.map((d) => d.y)
  let xMin = Math.min(...xs)
  let xMax = Math.max(...xs)
  let yMin = Math.min(...ys)
  let yMax = Math.max(...ys)

  // 添加边距
  const xMargin = (xMax - xMin) * 0.1 || 0.1
  const yMargin = (yMax - yMin) * 0.1 || 0.1
  xMin -= xMargin
  xMax += xMargin
  yMin -= yMargin
  yMax += yMargin

  // 确保包含原点
  xMin = Math.min(xMin, 0)
  xMax = Math.max(xMax, 0)
  yMin = Math.min(yMin, 0)
  yMax = Math.max(yMax, 0)

  // 对称化
  const xAbsMax = Math.max(Math.abs(xMin), Math.abs(xMax))
  const yAbsMax = Math.max(Math.abs(yMin), Math.abs(yMax))
  xMin = -xAbsMax
  xMax = xAbsMax
  yMin = -yAbsMax
  yMax = yAbsMax

  return { xMin, xMax, yMin, yMax, tickCount: 4 }
}

export function drawNoDataHint(ctx: CanvasRenderingContext2D, width: number, height: number): void {
  ctx.fillStyle = 'var(--text-muted, #64748b)'
  ctx.font = '13px sans-serif'
  ctx.textAlign = 'center'
  ctx.fillText('等待采集数据（可先点击开始校准）', width / 2, height / 2)
}

export function drawAxisTicks(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  padding: number,
  xMin: number,
  xMax: number,
  yMin: number,
  yMax: number,
  tickCount = 3
): void {
  ctx.fillStyle = 'var(--text-muted, #64748b)'
  ctx.font = '10px sans-serif'
  ctx.textBaseline = 'top'
  const xTickY = height - padding + 6

  for (let i = 0; i <= tickCount; i++) {
    const tx = xMin + ((xMax - xMin) * i) / tickCount
    const px = padding + ((width - 2 * padding) * i) / tickCount
    const safePx = Math.min(width - padding - 10, Math.max(padding + 10, px))
    ctx.textAlign = 'center'
    ctx.textBaseline = 'top'
    ctx.fillText(tx.toFixed(1), safePx, xTickY)

    const ty = yMax - ((yMax - yMin) * i) / tickCount
    const py = padding + ((height - 2 * padding) * i) / tickCount
    ctx.textAlign = 'right'
    ctx.textBaseline = 'middle'
    ctx.fillText(ty.toFixed(1), padding - 4, py + 3)
  }

  ctx.textBaseline = 'alphabetic'
}
