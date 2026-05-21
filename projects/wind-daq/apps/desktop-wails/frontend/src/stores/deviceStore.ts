import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { DeviceProfile, DeviceStatus, DataPayload } from '@api/types'
import { deviceApi, defaultSimulatedProfile } from '@api/deviceApi'

export const useDeviceStore = defineStore('devices', () => {
  const profiles = ref<DeviceProfile[]>([])
  const latestSnapshots = ref<DataPayload[]>([])
  const historyBuffers = ref<Map<string, DataPayload[]>>(new Map())
  const selectedDeviceId = ref<string | null>(null)
  const tareOffsets = ref<Map<string, Record<number, number>>>(new Map())
  const chartSelectedIndices = ref<Map<string, Set<number>>>(new Map())
  const deviceStatuses = ref<Map<string, DeviceStatus>>(new Map())

  const selectedProfile = computed(() =>
    profiles.value.find((p) => p.id === selectedDeviceId.value),
  )
  const selectedSnapshot = computed(() =>
    latestSnapshots.value.find((s) => s.deviceId === selectedDeviceId.value),
  )
  const statusFor = (id: string): string => deviceStatuses.value.get(id)?.connection ?? 'Disconnected'
  const acquiringFor = (id: string): boolean => deviceStatuses.value.get(id)?.acquiring ?? false
  const latestFor = (id: string): DataPayload | undefined =>
    latestSnapshots.value.find((s) => s.deviceId === id)
  const historyFor = (id: string): DataPayload[] => historyBuffers.value.get(id) ?? []

  function selectDevice(id: string | null) {
    selectedDeviceId.value = id
  }

  async function refreshProfiles() {
    try {
      profiles.value = await deviceApi.getProfiles()
      if (!profiles.value.length) {
        profiles.value = [defaultSimulatedProfile()]
      }
    } catch {
      profiles.value = [defaultSimulatedProfile()]
    }
  }

  async function refreshStatusFor(id: string) {
    try {
      const status = await deviceApi.getStatus(id)
      deviceStatuses.value.set(id, status)
    } catch {
      // keep stale status
    }
  }

  function updateStatus(id: string, status: DeviceStatus) {
    deviceStatuses.value.set(id, status)
  }

  function pushSnapshot(payload: DataPayload) {
    const idx = latestSnapshots.value.findIndex((s) => s.deviceId === payload.deviceId)
    if (idx >= 0) {
      latestSnapshots.value[idx] = payload
    } else {
      latestSnapshots.value.push(payload)
    }
    let buf = historyBuffers.value.get(payload.deviceId)
    if (!buf) {
      buf = []
      historyBuffers.value.set(payload.deviceId, buf)
    }
    buf.push(payload)
    if (buf.length > 256) buf.shift()
  }

  const formatValue = (id: string, ch: number, raw: number): string => {
    const offset = tareOffsets.value.get(id)?.[ch] ?? 0
    return (raw - offset).toFixed(3)
  }

  const getDisplayValue = (id: string, ch: number, raw: number): number => {
    const offset = tareOffsets.value.get(id)?.[ch] ?? 0
    return raw - offset
  }

  function setTare(id: string, ch: number, val: number) {
    let deviceTares = tareOffsets.value.get(id)
    if (!deviceTares) {
      deviceTares = {}
      tareOffsets.value.set(id, deviceTares)
    }
    deviceTares[ch] = val
  }

  function toggleChartSelection(id: string, ch: number) {
    let sel = chartSelectedIndices.value.get(id)
    if (!sel) {
      sel = new Set()
      chartSelectedIndices.value.set(id, sel)
    }
    if (sel.has(ch)) sel.delete(ch)
    else if (sel.size < 16) sel.add(ch)
  }

  function isChartSelected(id: string, ch: number): boolean {
    return chartSelectedIndices.value.get(id)?.has(ch) ?? false
  }

  return {
    profiles,
    latestSnapshots,
    selectedDeviceId,
    selectedProfile,
    selectedSnapshot,
    deviceStatuses,
    statusFor,
    acquiringFor,
    selectDevice,
    refreshProfiles,
    refreshStatusFor,
    updateStatus,
    pushSnapshot,
    latestFor,
    historyFor,
    formatValue,
    getDisplayValue,
    setTare,
    toggleChartSelection,
    isChartSelected,
  }
})
