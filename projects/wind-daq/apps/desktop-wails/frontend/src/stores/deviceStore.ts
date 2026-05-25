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
  let _unsubscribeSnapshot: (() => void) | null = null

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

  function attachStatusListener(): () => void {
    if (_unsubscribeSnapshot) {
      _unsubscribeSnapshot()
    }
    
    // 自动订阅所有设备的快照
    _unsubscribeSnapshot = deviceApi.onSnapshot((payload) => {
      pushSnapshot(payload)
    })

    // 同时自动订阅现有所有设备的数据流
    profiles.value.forEach((p) => {
      deviceApi.subscribeToDevice(p.id)
    })

    return () => {
      if (_unsubscribeSnapshot) {
        _unsubscribeSnapshot()
        _unsubscribeSnapshot = null
      }
      // 取消所有设备订阅
      profiles.value.forEach((p) => {
        deviceApi.unsubscribeFromDevice(p.id)
      })
    }
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

  function tareAllEnabled(id: string) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return
    const snapshot = latestSnapshots.value.find((s) => s.deviceId === id)
    if (!snapshot) return
    profile.channels.forEach((ch) => {
      if (ch.enabled) {
        const pos = snapshot.channelIndices.indexOf(ch.index)
        if (pos >= 0) {
          setTare(id, ch.index, snapshot.channels[pos])
        }
      }
    })
  }

  function getOffset(id: string, ch: number): number {
    return tareOffsets.value.get(id)?.[ch] ?? 0
  }

  function getChannelRange(id: string, channelIndex: number): { min: number; max: number } {
    const profile = profiles.value.find((p) => p.id === id)
    const channel = profile?.channels.find((c) => c.index === channelIndex)
    return {
      min: channel?.rangeMin ?? -10,
      max: channel?.rangeMax ?? 10,
    }
  }

  function getChannelPrecision(id: string, channelIndex: number): number {
    const profile = profiles.value.find((p) => p.id === id)
    const channel = profile?.channels.find((c) => c.index === channelIndex)
    return channel?.precision ?? 3
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

  function setAllChartSelection(id: string, enabled: boolean) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return
    let sel = chartSelectedIndices.value.get(id)
    if (!sel) {
      sel = new Set()
      chartSelectedIndices.value.set(id, sel)
    }
    profile.channels.forEach((ch) => {
      if (enabled) {
        if (sel!.size < 16) sel!.add(ch.index)
      } else {
        sel!.delete(ch.index)
      }
    })
  }

  function isChartSelected(id: string, ch: number): boolean {
    return chartSelectedIndices.value.get(id)?.has(ch) ?? false
  }

  async function connect(id: string) {
    await deviceApi.connect(id)
  }

  async function disconnect(id: string) {
    await deviceApi.disconnect(id)
  }

  async function startAcquisition(id: string): Promise<boolean> {
    const ok = await deviceApi.startAcquisition(id)
    return ok.success
  }

  async function stopAcquisition(id: string): Promise<void> {
    await deviceApi.stopAcquisition(id)
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
    attachStatusListener,
    latestFor,
    historyFor,
    formatValue,
    getDisplayValue,
    setTare,
    tareAllEnabled,
    getOffset,
    getChannelRange,
    getChannelPrecision,
    toggleChartSelection,
    setAllChartSelection,
    isChartSelected,
    connect,
    disconnect,
    startAcquisition,
    stopAcquisition,
  }
})
