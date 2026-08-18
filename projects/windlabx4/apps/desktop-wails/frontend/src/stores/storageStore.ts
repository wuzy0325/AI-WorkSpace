import { defineStore } from 'pinia'
import { ref } from 'vue'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
import { request } from '@api/http-client'

export interface FileRotationSettings {
  enabled: boolean
  maxFileSizeBytes: number
  maxDurationMs: number
}

export interface StopConditionsSettings {
  maxDurationMs?: number
  maxFileSizeBytes?: number
  maxRecordCount?: number
}

export interface StorageSettings {
  baseDirectory: string
  filePrefix: string
  autoStartOnAcquisition: boolean
  stopConditions: StopConditionsSettings
  fileRotation: FileRotationSettings
  /**
   * 实时波形图时间窗口（秒）。
   * 与刷新率解耦：无论 refreshRateHz 多少，图表横轴都保留相同的秒数。
   * 容量 = historyWindowSec × refreshRateHz，自动随刷新率变化。
   */
  historyWindowSec: number
  /**
   * 实时数据刷新频率（Hz），启动时下发后端 AcquisitionHub 并同步前端轮询间隔。
   * 同时决定波形图密度——每秒新增多少个点到 historyBuffers。
   */
  refreshRateHz: number
}

const CONFIG_KEY = 'storage-settings'

/** 时间窗口最小值（秒）—— 低于 5 秒波形过于短，看不出趋势 */
export const HISTORY_WINDOW_MIN_SEC = 5
/** 时间窗口最大值（秒）—— 收紧到 60 秒，配合 5Hz 刷新率上限 = 300 点硬上限 */
export const HISTORY_WINDOW_MAX_SEC = 60
/** 时间窗口步长（秒） */
export const HISTORY_WINDOW_STEP_SEC = 5
/** 时间窗口默认值（秒）—— 15 秒在 5Hz 下 = 75 点，CPU 占用最低 */
export const DEFAULT_HISTORY_WINDOW_SEC = 15

/**
 * 刷新率默认值（Hz）。
 *
 * 降到 5Hz 的原因：
 *   - windlabx4 波形图用 smooth + areaStyle 渐变，渲染开销比 daq-p1604 大
 *   - 10Hz 在 300 点 × 4 通道下持续运行 60 秒后仍会卡顿
 *   - 5Hz 渲染每秒 5 帧，视觉上仍连续（人眼对缓慢波形 5Hz 足够）
 *   - CPU 占用从 10Hz 的 ~15% 降到 5Hz 的 ~8%
 *   - 用户可在设置中调到 10Hz，但默认值保证开箱即用不卡
 */
export const DEFAULT_REFRESH_RATE_HZ = 5
/** 刷新率范围（Hz），收紧上限到 10Hz 防止过载 */
export const REFRESH_RATE_MIN = 1
export const REFRESH_RATE_MAX = 10

/**
 * 波形图历史容量硬上限（点数）。
 *
 * 降到 300 的原因：
 *   - windlabx4 的 smooth 平滑曲线 + areaStyle 渐变填充渲染开销大
 *   - 600 点在持续运行 60 秒后仍会卡顿（ECharts Canvas path 绘制是瓶颈）
 *   - 300 点 × 4 通道 = 1200 个 number，5Hz 渲染下 CPU 占用 ~8%
 *   - 对齐 daq-p1604 默认 300 点（10Hz × 30s），实际使用流畅
 *   - 用户设大时间窗口/高刷新率时自动 clamp 到 300，保证 UI 不卡
 */
export const HISTORY_CAPACITY_HARD_CAP = 300

/** 将刷新率限制到合法范围并取整。加载/保存/校验统一调用此函数，避免多处 clamp 逻辑漂移。 */
export function clampRefreshHz(hz: number): number {
  return Math.max(REFRESH_RATE_MIN, Math.min(REFRESH_RATE_MAX, Math.round(hz)))
}

/** 将时间窗口限制到合法范围并取整 */
export function clampHistoryWindowSec(sec: number): number {
  return Math.max(HISTORY_WINDOW_MIN_SEC, Math.min(HISTORY_WINDOW_MAX_SEC, Math.round(sec)))
}

/**
 * 计算波形图历史容量（点数）。
 * 容量 = 时间窗口 × 刷新率，并 clamp 到硬上限。
 * 供 deviceStore.renderTick 计算 ringBuffer 容量使用。
 */
export function computeHistoryCapacity(historyWindowSec: number, refreshRateHz: number): number {
  const cap = Math.round(historyWindowSec * refreshRateHz)
  return Math.min(Math.max(cap, 10), HISTORY_CAPACITY_HARD_CAP)
}

export const DEFAULT_SETTINGS: StorageSettings = {
  baseDirectory: 'data/recordings',
  filePrefix: 'run',
  autoStartOnAcquisition: false,
  stopConditions: {},
  fileRotation: { enabled: false, maxFileSizeBytes: 104857600, maxDurationMs: 1800000 },
  historyWindowSec: DEFAULT_HISTORY_WINDOW_SEC,
  refreshRateHz: DEFAULT_REFRESH_RATE_HZ,
}

/**
 * 清理 stopConditions：移除值为 0/undefined 的键，避免持久化残留旧配置。
 * 例如用户先启用 "按记录数停止=1000000" 再取消勾选后，应删除 maxRecordCount 键
 * 而不是保留 0（后端会按"零值不限制"语义处理，但持久化层应保持干净）。
 */
function sanitizeStopConditions(raw: StopConditionsSettings | undefined): StopConditionsSettings {
  if (!raw) return {}
  const cleaned: StopConditionsSettings = {}
  if (raw.maxDurationMs && raw.maxDurationMs > 0) cleaned.maxDurationMs = raw.maxDurationMs
  if (raw.maxFileSizeBytes && raw.maxFileSizeBytes > 0) cleaned.maxFileSizeBytes = raw.maxFileSizeBytes
  if (raw.maxRecordCount && raw.maxRecordCount > 0) cleaned.maxRecordCount = raw.maxRecordCount
  return cleaned
}

/**
 * 将旧版 storage-settings 配置迁移到新结构。
 *
 * 旧版有 waveformBufferSize（点数），新版改为 historyWindowSec（秒）+ refreshRateHz。
 * 迁移策略：若新字段不存在但旧字段存在，按默认刷新率 20Hz 反推时间窗口：
 *   historyWindowSec = round(waveformBufferSize / 20)
 * 例如旧 waveformBufferSize=100 → 新 historyWindowSec=5（太小，会被 clamp 到 5）
 *         旧 waveformBufferSize=2000 → 新 historyWindowSec=100
 */
function migrateLegacySettings(raw: any): Partial<StorageSettings> {
  const parsed: Partial<StorageSettings> = { ...raw }
  // 删除旧字段避免持久化回写时残留
  if ('waveformBufferSize' in parsed) {
    const legacyBufferSize = typeof parsed.waveformBufferSize === 'number' ? parsed.waveformBufferSize : DEFAULT_HISTORY_WINDOW_SEC * DEFAULT_REFRESH_RATE_HZ
    if (parsed.historyWindowSec === undefined) {
      // 反推：按默认 20Hz 算出秒数
      const inferredSec = Math.round(legacyBufferSize / DEFAULT_REFRESH_RATE_HZ)
      parsed.historyWindowSec = clampHistoryWindowSec(inferredSec)
    }
    delete (parsed as any).waveformBufferSize
  }
  return parsed
}

export const useStorageStore = defineStore('storage', () => {
  const settings = ref<StorageSettings>({ ...DEFAULT_SETTINGS })
  /** 加载失败错误信息（空表示无错误），供 UI 反馈 */
  const loadError = ref<string>('')

  async function loadSettings(): Promise<void> {
    loadError.value = ''
    try {
      let data: any = null
      if (isWailsAvailable()) {
        const res = await wailsApi.config.load(CONFIG_KEY)
        if (res.success) data = res.data
      } else {
        const res = await request<{ success: boolean; data?: any }>(`/api/config/${CONFIG_KEY}`)
        if (res.success && res.data) data = res.data
      }
      if (data) {
        // 先迁移旧版字段（waveformBufferSize → historyWindowSec），再按新结构解析
        const parsed = migrateLegacySettings(data)
        // 加载时进行边界限制，避免配置文件中存在非法值导致图表异常
        const rawWindowSec = parsed.historyWindowSec ?? DEFAULT_SETTINGS.historyWindowSec
        const clampedWindowSec = clampHistoryWindowSec(rawWindowSec)
        // 刷新率同样做边界限制，避免配置文件非法值导致轮询异常
        const rawRefreshHz = parsed.refreshRateHz ?? DEFAULT_SETTINGS.refreshRateHz
        const clampedRefreshHz = clampRefreshHz(rawRefreshHz)
        // 路径解析统一在后端 StorageStartRecording 完成；
        // 前端只持久化用户输入的原始路径（相对或绝对），避免双轨解析。
        settings.value = {
          baseDirectory: parsed.baseDirectory ?? DEFAULT_SETTINGS.baseDirectory,
          filePrefix: parsed.filePrefix ?? DEFAULT_SETTINGS.filePrefix,
          autoStartOnAcquisition: parsed.autoStartOnAcquisition ?? DEFAULT_SETTINGS.autoStartOnAcquisition,
          // 清理 stopConditions 中的零值/残留键，确保配置干净
          stopConditions: sanitizeStopConditions(parsed.stopConditions),
          fileRotation: parsed.fileRotation ?? DEFAULT_SETTINGS.fileRotation,
          historyWindowSec: clampedWindowSec,
          refreshRateHz: clampedRefreshHz,
        }
      } else {
        // 配置文件尚不存在（首次启动）：使用默认值，不算错误
        settings.value = { ...DEFAULT_SETTINGS }
      }
    } catch (err) {
      // 加载失败：保留之前的 settings（若有），并暴露错误给 UI
      loadError.value = err instanceof Error ? err.message : String(err)
      console.warn('[storageStore] loadSettings 失败，保留之前配置:', err)
    }
  }

  async function saveSettings(next: StorageSettings): Promise<void> {
    // 保存前再次清理 stopConditions 并限制刷新率/时间窗口范围，避免 UI 通过隐藏输入绕过校验
    const cleaned: StorageSettings = {
      ...next,
      stopConditions: sanitizeStopConditions(next.stopConditions),
      refreshRateHz: clampRefreshHz(next.refreshRateHz),
      historyWindowSec: clampHistoryWindowSec(next.historyWindowSec),
    }
    settings.value = cleaned
    try {
      if (isWailsAvailable()) {
        await wailsApi.config.save(CONFIG_KEY, cleaned)
      } else {
        await request(`/api/config/${CONFIG_KEY}`, {
          method: 'PUT',
          body: JSON.stringify(cleaned),
        })
      }
    } catch (err) {
      console.error('保存存储设置失败:', err)
      throw err
    }
  }

  async function pickDirectory(): Promise<string> {
    if (isWailsAvailable()) {
      return await wailsApi.app.pickDirectory()
    }
    return settings.value?.baseDirectory ?? ''
  }

  async function pickSaveFile(title: string, defaultFilename: string, filters: Array<{ displayName: string; pattern: string }>): Promise<string> {
    if (isWailsAvailable()) {
      return await wailsApi.app.pickSaveFile(title, defaultFilename, filters)
    }
    return ''
  }

  // fileExists 检查文件是否存在。Wails 不可用时返回 false。
  // 用于校准 Start 前检测 CSV 文件是否已存在，提示用户决定是否覆盖。
  async function fileExists(path: string): Promise<boolean> {
    if (isWailsAvailable()) {
      return await wailsApi.app.fileExists(path)
    }
    return false
  }

  // removeFile 删除文件。Wails 不可用时返回 false。
  // 用于校准 Start 前用户选择"覆盖"时清理旧 CSV 文件。
  async function removeFile(path: string): Promise<boolean> {
    if (isWailsAvailable()) {
      return await wailsApi.app.removeFile(path)
    }
    return false
  }

  return { settings, loadError, loadSettings, saveSettings, pickDirectory, pickSaveFile, fileExists, removeFile }
})
