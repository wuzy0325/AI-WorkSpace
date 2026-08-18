import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

const STORAGE_KEY = 'wista:display-refresh-rate-hz'
const DEFAULT_REFRESH_RATE_HZ = 10

function loadRefreshRateHz(): number {
  if (typeof window === 'undefined') return DEFAULT_REFRESH_RATE_HZ
  const raw = window.localStorage.getItem(STORAGE_KEY)
  const parsed = raw ? Number(raw) : NaN
  return Number.isFinite(parsed) && parsed > 0 ? parsed : DEFAULT_REFRESH_RATE_HZ
}

export const useDisplayStore = defineStore('display', () => {
  const refreshRateHz = ref(loadRefreshRateHz())

  watch(refreshRateHz, (value) => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(STORAGE_KEY, String(value))
  })

  function setRefreshRateHz(value: number): void {
    if (!Number.isFinite(value) || value <= 0) return
    refreshRateHz.value = value
  }

  return {
    refreshRateHz,
    setRefreshRateHz,
  }
})
