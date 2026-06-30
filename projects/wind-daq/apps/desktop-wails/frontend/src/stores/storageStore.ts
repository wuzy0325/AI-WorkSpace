import { defineStore } from 'pinia'
import { ref } from 'vue'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'
import { request } from '@api/http-client'

export interface FileRotationSettings {
  enabled: boolean
  maxFileSizeBytes: number
  maxDurationMs: number
}

export interface StorageSettings {
  baseDirectory: string
  filePrefix: string
  autoStartOnAcquisition: boolean
  stopConditions: {
    maxDurationMs?: number
    maxFileSizeBytes?: number
    maxRecordCount?: number
  }
  fileRotation: FileRotationSettings
  /** 实时波形图缓冲区点数 */
  waveformBufferSize: number
}

const CONFIG_KEY = 'storage-settings'

/** 波形图缓冲区点数最小值 */
export const WAVEFORM_BUFFER_MIN = 50
/** 波形图缓冲区点数最大值 */
export const WAVEFORM_BUFFER_MAX = 2000
/** 波形图缓冲区点数步长 */
export const WAVEFORM_BUFFER_STEP = 50

export const DEFAULT_SETTINGS: StorageSettings = {
  baseDirectory: 'data/recordings',
  filePrefix: 'run',
  autoStartOnAcquisition: false,
  stopConditions: {},
  fileRotation: { enabled: false, maxFileSizeBytes: 104857600, maxDurationMs: 1800000 },
  waveformBufferSize: 100,
}

export const useStorageStore = defineStore('storage', () => {
  const settings = ref<StorageSettings>({ ...DEFAULT_SETTINGS })

  async function loadSettings(): Promise<void> {
    try {
      let data: any = null
      if (isWailsAvailable()) {
        const res = await wailsApi.config.load(CONFIG_KEY)
        if (res.success) data = res.data
        if (data?.baseDirectory) {
          data.baseDirectory = await wailsApi.app.resolvePath(data.baseDirectory)
        }
      } else {
        const res = await request<{ success: boolean; data?: any }>(`/api/config/${CONFIG_KEY}`)
        if (res.success && res.data) data = res.data
      }
      if (data) {
        const parsed = data as Partial<StorageSettings>
        // 加载时进行边界限制，避免配置文件中存在非法值导致图表异常
        const rawBufferSize = parsed.waveformBufferSize ?? DEFAULT_SETTINGS.waveformBufferSize
        const clampedBufferSize = Math.max(WAVEFORM_BUFFER_MIN, Math.min(WAVEFORM_BUFFER_MAX, rawBufferSize))
        settings.value = {
          baseDirectory: parsed.baseDirectory ?? DEFAULT_SETTINGS.baseDirectory,
          filePrefix: parsed.filePrefix ?? DEFAULT_SETTINGS.filePrefix,
          autoStartOnAcquisition: parsed.autoStartOnAcquisition ?? DEFAULT_SETTINGS.autoStartOnAcquisition,
          stopConditions: parsed.stopConditions ?? DEFAULT_SETTINGS.stopConditions,
          fileRotation: parsed.fileRotation ?? DEFAULT_SETTINGS.fileRotation,
          waveformBufferSize: clampedBufferSize,
        }
      }
      if (isWailsAvailable() && settings.value.baseDirectory) {
        settings.value.baseDirectory = await wailsApi.app.resolvePath(settings.value.baseDirectory)
      }
    } catch {
      settings.value = { ...DEFAULT_SETTINGS }
    }
  }

  async function saveSettings(next: StorageSettings): Promise<void> {
    settings.value = { ...next }
    try {
      if (isWailsAvailable()) {
        await wailsApi.config.save(CONFIG_KEY, next)
      } else {
        await request(`/api/config/${CONFIG_KEY}`, {
          method: 'PUT',
          body: JSON.stringify(next),
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

  return { settings, loadSettings, saveSettings, pickDirectory, pickSaveFile }
})
