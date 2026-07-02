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
  /** 实时波形图缓冲区点数 */
  waveformBufferSize: number
  /** 实时数据刷新频率（Hz），启动时下发后端 AcquisitionHub 并同步前端轮询间隔 */
  refreshRateHz: number
}

const CONFIG_KEY = 'storage-settings'

/** 波形图缓冲区点数最小值 */
export const WAVEFORM_BUFFER_MIN = 50
/** 波形图缓冲区点数最大值 */
export const WAVEFORM_BUFFER_MAX = 2000
/** 波形图缓冲区点数步长 */
export const WAVEFORM_BUFFER_STEP = 50

/** 刷新率默认值（Hz），与后端 AcquisitionHub 默认值对齐 */
export const DEFAULT_REFRESH_RATE_HZ = 20
/** 刷新率范围（Hz），与 DisplaySettingsSection 校验及后端 minPublishHz 对齐 */
export const REFRESH_RATE_MIN = 1
export const REFRESH_RATE_MAX = 20

/** 将刷新率限制到合法范围并取整。加载/保存/校验统一调用此函数，避免多处 clamp 逻辑漂移。 */
export function clampRefreshHz(hz: number): number {
  return Math.max(REFRESH_RATE_MIN, Math.min(REFRESH_RATE_MAX, Math.round(hz)))
}

export const DEFAULT_SETTINGS: StorageSettings = {
  baseDirectory: 'data/recordings',
  filePrefix: 'run',
  autoStartOnAcquisition: false,
  stopConditions: {},
  fileRotation: { enabled: false, maxFileSizeBytes: 104857600, maxDurationMs: 1800000 },
  waveformBufferSize: 100,
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
        const parsed = data as Partial<StorageSettings>
        // 加载时进行边界限制，避免配置文件中存在非法值导致图表异常
        const rawBufferSize = parsed.waveformBufferSize ?? DEFAULT_SETTINGS.waveformBufferSize
        const clampedBufferSize = Math.max(WAVEFORM_BUFFER_MIN, Math.min(WAVEFORM_BUFFER_MAX, rawBufferSize))
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
          waveformBufferSize: clampedBufferSize,
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
    // 保存前再次清理 stopConditions 并限制刷新率范围，避免 UI 通过隐藏输入绕过校验
    const cleaned: StorageSettings = {
      ...next,
      stopConditions: sanitizeStopConditions(next.stopConditions),
      refreshRateHz: clampRefreshHz(next.refreshRateHz),
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

  return { settings, loadError, loadSettings, saveSettings, pickDirectory, pickSaveFile }
})
