import { defineStore } from 'pinia'
import { ref } from 'vue'
import { isWailsAvailable, wailsApi } from '@api/wails-adapter'

interface StorageSettings {
  baseDirectory: string
}

const STORAGE_KEY = 'wind-daq.storage-settings'

export const useStorageStore = defineStore('storage', () => {
  const settings = ref<StorageSettings | null>(null)

  async function loadSettings(): Promise<void> {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      settings.value = raw ? JSON.parse(raw) as StorageSettings : { baseDirectory: '' }
    } catch {
      settings.value = { baseDirectory: '' }
    }
  }

  async function pickDirectory(): Promise<string> {
    if (isWailsAvailable()) {
      return await wailsApi.app.pickDirectory()
    }
    return settings.value?.baseDirectory ?? ''
  }

  return { settings, loadSettings, pickDirectory }
})
