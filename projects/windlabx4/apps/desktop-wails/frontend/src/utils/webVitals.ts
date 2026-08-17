/**
 * Web Vitals 上报 —— 性能可观测性接入点
 *
 * 监控 Core Web Vitals：
 *   - LCP (Largest Contentful Paint): 首屏最大元素绘制 — 目标 ≤ 2.5s
 *   - INP (Interaction to Next Paint):  交互响应延迟      — 目标 ≤ 200ms
 *   - CLS (Cumulative Layout Shift):    累计布局位移      — 目标 ≤ 0.1
 *
 * 补充：
 *   - FCP (First Contentful Paint): 首次内容绘制
 *   - TTFB (Time to First Byte):    首字节时间
 *
 * 当前策略：开发模式 console.log + 内存日志缓存；后续可接入后端 /api/metrics。
 * 不上传任何用户数据，工业 DAQ 通常离线运行。
 */

import { onCLS, onFCP, onINP, onLCP, onTTFB, type Metric } from 'web-vitals'

// 评级阈值（来自 web.dev/vitals 官方推荐）
const THRESHOLDS = {
  LCP: { good: 2500, poor: 4000 },
  INP: { good: 200, poor: 500 },
  CLS: { good: 0.1, poor: 0.25 },
  FCP: { good: 1800, poor: 3000 },
  TTFB: { good: 800, poor: 1800 },
} as const

type Rating = 'good' | 'needs-improvement' | 'poor'

function rate(name: keyof typeof THRESHOLDS, value: number): Rating {
  const t = THRESHOLDS[name]
  if (value <= t.good) return 'good'
  if (value <= t.poor) return 'needs-improvement'
  return 'poor'
}

// 内存缓存最近样本，便于 DevTools 直接读取或后续上报
const sampleBuffer: Array<Metric & { rating: Rating; ts: number }> = []
const MAX_SAMPLES = 100

function report(metric: Metric): void {
  const name = metric.name as keyof typeof THRESHOLDS
  const rating = rate(name, metric.value)
  const sample = { ...metric, rating, ts: Date.now() }

  sampleBuffer.push(sample)
  if (sampleBuffer.length > MAX_SAMPLES) sampleBuffer.shift()

  const formatted = name === 'CLS' ? metric.value.toFixed(3) : `${Math.round(metric.value)}ms`
  const tag = `[web-vitals] ${metric.name}=${formatted} (${rating})`

  // 用 group 让 DevTools 控制台更清爽，可点开看详细 entries
  if (rating === 'poor') {
    console.warn(tag, metric)
  } else if (rating === 'needs-improvement') {
    console.info(tag, metric)
  } else {
    console.debug(tag)
  }
}

/**
 * 启动 Web Vitals 监控。应在 app 启动后尽早调用一次。
 * 多次调用是安全的（web-vitals 内部去重），但推荐只调一次。
 */
export function initWebVitals(): void {
  onLCP(report)
  onINP(report)
  onCLS(report)
  onFCP(report)
  onTTFB(report)

  // 暴露调试入口：DevTools 里 __WEB_VITALS__ 直接看缓存样本
  if (typeof window !== 'undefined') {
    ;(window as unknown as { __WEB_VITALS__?: typeof sampleBuffer }).__WEB_VITALS__ =
      sampleBuffer
  }
}

/** 当前缓存的 vitals 样本（最近 MAX_SAMPLES 条）。用于诊断或上报。 */
export function getSamples(): ReadonlyArray<Metric & { rating: Rating; ts: number }> {
  return sampleBuffer
}
