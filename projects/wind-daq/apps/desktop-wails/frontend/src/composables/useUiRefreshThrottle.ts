import { ref, computed } from 'vue'

export function useUiRefreshThrottle(storageKey: string, defaultHz = 2) {
  const clampHz = (hz: number): number => Math.min(20, Math.max(1, Math.round(hz)))

  const uiRefreshHz = ref<number>(defaultHz)
  try {
    const raw = localStorage.getItem(storageKey)
    const parsed = raw ? Number(raw) : defaultHz
    uiRefreshHz.value = clampHz(Number.isFinite(parsed) ? parsed : defaultHz)
  } catch {
    // ignore
  }

  const uiRefreshIntervalMs = computed(() => Math.round(1000 / uiRefreshHz.value))

  function setUiRefreshHz(hz: number): void {
    const next = clampHz(hz)
    uiRefreshHz.value = next
    try {
      localStorage.setItem(storageKey, String(next))
    } catch {
      // ignore
    }
  }

  return { uiRefreshHz, uiRefreshIntervalMs, setUiRefreshHz }
}
