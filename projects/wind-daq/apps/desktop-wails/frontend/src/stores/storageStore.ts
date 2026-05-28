import { defineStore } from 'pinia'
import { ref } from 'vue'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'

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

const STORAGE_KEY = 'wind-daq.global-settings'

const DEFAULT_SETTINGS: StorageSettings = {
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
      const raw = localStorage.getItem(STORAGE_KEY)
      if (raw) {
        const parsed = JSON.parse(raw) as Partial<StorageSettings>
        settings.value = {
          baseDirectory: parsed.baseDirectory ?? DEFAULT_SETTINGS.baseDirectory,
          filePrefix: parsed.filePrefix ?? DEFAULT_SETTINGS.filePrefix,
          autoStartOnAcquisition: parsed.autoStartOnAcquisition ?? DEFAULT_SETTINGS.autoStartOnAcquisition,
          stopConditions: parsed.stopConditions ?? DEFAULT_SETTINGS.stopConditions,
          fileRotation: parsed.fileRotation ?? DEFAULT_SETTINGS.fileRotation,
        }
      }
    } catch {
      settings.value = { ...DEFAULT_SETTINGS }
    }
  }

  async function saveSettings(next: StorageSettings): Promise<void> {
    settings.value = { ...next }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  }

  async function pickDirectory(): Promise<string> {
    if (isWailsAvailable()) {
      return await wailsApi.app.pickDirectory()
    }
    return settings.value?.baseDirectory ?? ''
  }

  return { settings, loadSettings, saveSettings, pickDirectory }
})
