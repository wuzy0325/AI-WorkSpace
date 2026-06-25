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
}

const CONFIG_KEY = 'storage-settings'

export const DEFAULT_SETTINGS: StorageSettings = {
  baseDirectory: 'data/recordings',
  filePrefix: 'run',
  autoStartOnAcquisition: false,
  stopConditions: {},
  fileRotation: { enabled: false, maxFileSizeBytes: 104857600, maxDurationMs: 1800000 },
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
        settings.value = {
          baseDirectory: parsed.baseDirectory ?? DEFAULT_SETTINGS.baseDirectory,
          filePrefix: parsed.filePrefix ?? DEFAULT_SETTINGS.filePrefix,
          autoStartOnAcquisition: parsed.autoStartOnAcquisition ?? DEFAULT_SETTINGS.autoStartOnAcquisition,
          stopConditions: parsed.stopConditions ?? DEFAULT_SETTINGS.stopConditions,
          fileRotation: parsed.fileRotation ?? DEFAULT_SETTINGS.fileRotation,
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
