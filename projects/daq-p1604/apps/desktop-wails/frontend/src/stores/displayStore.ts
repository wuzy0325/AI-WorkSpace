import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

// ---- 存储键 ----
const STORAGE_KEY_REFRESH = 'daq-p1604:display-refresh-rate-hz'
const STORAGE_KEY_WINDOW = 'daq-p1604:display-history-window-sec'

// ---- 默认值 ----
/** UI 渲染刷新率默认 10Hz（每 100ms 触发一次图表/数值卡重绘） */
const DEFAULT_REFRESH_RATE_HZ = 10
/** 图表历史时间窗口默认 30 秒 —— 与刷新率解耦，窗口=秒×刷新率个点 */
const DEFAULT_HISTORY_WINDOW_SEC = 30

// ---- 合法范围 ----
/** 上限 60Hz 保护 WebView2 GUI 线程 */
const MIN_REFRESH_HZ = 1
const MAX_REFRESH_HZ = 60
const MIN_WINDOW_SEC = 5
const MAX_WINDOW_SEC = 600

/** 从 localStorage 读取数值配置并进行范围校验 */
function loadNumber(key: string, fallback: number, min: number, max: number): number {
  if (typeof window === 'undefined') return fallback
  const raw = window.localStorage.getItem(key)
  const parsed = raw ? Number(raw) : NaN
  if (!Number.isFinite(parsed)) return fallback
  if (parsed < min || parsed > max) return fallback
  return parsed
}

/**
 * 显示相关的用户偏好设置。
 *
 * 概念说明：
 * - refreshRateHz：**UI 渲染频率**。控制图表 / 数值卡从最新快照更新到 DOM 的节奏。
 *   与后端数据产生频率无关；后端已经按设备采样率把最新快照缓存在内存中，
 *   前端只是选择"多快地把它取过来画一次"。
 * - historyWindowSec：**图表可视时间窗口（秒）**。控制波形图横轴保留多长的历史。
 *   与刷新率解耦：窗口 30 秒 + 20Hz 刷新率 = 最多保留 600 个点，
 *                    窗口 30 秒 + 2Hz 刷新率  = 最多保留  60 个点。
 *   避免"选高刷新率反而看到的时间变短"的意外行为。
 */
export const useDisplayStore = defineStore('display', () => {
  const refreshRateHz = ref(
    loadNumber(STORAGE_KEY_REFRESH, DEFAULT_REFRESH_RATE_HZ, MIN_REFRESH_HZ, MAX_REFRESH_HZ),
  )
  const historyWindowSec = ref(
    loadNumber(STORAGE_KEY_WINDOW, DEFAULT_HISTORY_WINDOW_SEC, MIN_WINDOW_SEC, MAX_WINDOW_SEC),
  )

  // 持久化到 localStorage
  watch(refreshRateHz, (value) => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(STORAGE_KEY_REFRESH, String(value))
  })
  watch(historyWindowSec, (value) => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(STORAGE_KEY_WINDOW, String(value))
  })

  /** 设置 UI 渲染刷新率（Hz），范围外的值会被忽略 */
  function setRefreshRateHz(value: number): void {
    if (!Number.isFinite(value)) return
    if (value < MIN_REFRESH_HZ || value > MAX_REFRESH_HZ) return
    refreshRateHz.value = value
  }

  /** 设置图表历史时间窗口（秒），范围外的值会被忽略 */
  function setHistoryWindowSec(value: number): void {
    if (!Number.isFinite(value)) return
    if (value < MIN_WINDOW_SEC || value > MAX_WINDOW_SEC) return
    historyWindowSec.value = value
  }

  return {
    refreshRateHz,
    historyWindowSec,
    setRefreshRateHz,
    setHistoryWindowSec,
  }
})