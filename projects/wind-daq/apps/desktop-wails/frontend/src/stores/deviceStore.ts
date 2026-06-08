import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { DeviceProfile, DeviceStatus, DataPayload, DSA3217ScanConfig } from '@api/types'
import { deviceApi, defaultSimulatedProfile } from '@api/deviceApi'

const MAX_HISTORY_POINTS = 256
const MAX_CHART_CHANNELS = 16

export const useDeviceStore = defineStore('devices', () => {
  const profiles = ref<DeviceProfile[]>([])
  const latestSnapshots = ref<DataPayload[]>([])
  const historyBuffers = ref<Map<string, DataPayload[]>>(new Map())
  const selectedDeviceId = ref<string | null>(null)
  const tareOffsets = ref<Map<string, Record<number, number>>>(new Map())
  const chartSelectedIndices = ref<Map<string, Set<number>>>(new Map())
  const deviceStatuses = ref<Map<string, DeviceStatus>>(new Map())

  let unsubscribeSnapshot: (() => void) | null = null
  let snapshotAttachCount = 0
  const subscribedDeviceIds = new Set<string>()

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
    if (id) initializeDefaultChartSelection(id)
  }

  async function refreshProfiles() {
    try {
      profiles.value = (await deviceApi.getProfiles()).map((profile) => ({
        ...profile,
        channels: Array.isArray(profile.channels) ? profile.channels : [],
      }))
      if (!profiles.value.length) {
        profiles.value = [defaultSimulatedProfile()]
      }
    } catch {
      profiles.value = [defaultSimulatedProfile()]
    }
    syncSnapshotSubscriptions()
    if (selectedDeviceId.value) initializeDefaultChartSelection(selectedDeviceId.value)
  }

  async function refreshInstances() {
    await refreshProfiles()
  }

  async function refreshStatusFor(id: string) {
    try {
      const status = await deviceApi.getStatus(id)
      deviceStatuses.value.set(id, status)
    } catch {
      // Keep the last known status; transient device status reads should not blank the UI.
    }
  }

  function updateStatus(id: string, status: DeviceStatus) {
    deviceStatuses.value.set(id, status)
  }

  function pushSnapshot(payload: DataPayload) {
    if (!payload || !payload.deviceId) return
    const normalized: DataPayload = {
      deviceId: payload.deviceId,
      timestamp: payload.timestamp ?? 0,
      channels: Array.isArray(payload.channels) ? payload.channels : [],
      channelIndices: Array.isArray(payload.channelIndices) ? payload.channelIndices : [],
    }
    const idx = latestSnapshots.value.findIndex((s) => s.deviceId === normalized.deviceId)
    if (idx >= 0) {
      latestSnapshots.value[idx] = normalized
    } else {
      latestSnapshots.value.push(normalized)
    }

    let buffer = historyBuffers.value.get(normalized.deviceId)
    if (!buffer) {
      buffer = []
      historyBuffers.value.set(normalized.deviceId, buffer)
    }
    buffer.push(normalized)
    if (buffer.length > MAX_HISTORY_POINTS) buffer.shift()
  }

  function syncSnapshotSubscriptions() {
    if (!unsubscribeSnapshot) return

    const activeIds = new Set(profiles.value.map((p) => p.id))
    profiles.value.forEach((profile) => {
      if (subscribedDeviceIds.has(profile.id)) return
      deviceApi.subscribeToDevice(profile.id)
      subscribedDeviceIds.add(profile.id)
    })

    Array.from(subscribedDeviceIds).forEach((id) => {
      if (activeIds.has(id)) return
      deviceApi.unsubscribeFromDevice(id)
      subscribedDeviceIds.delete(id)
    })
  }

  function cleanupSnapshotSubscriptions() {
    subscribedDeviceIds.forEach((id) => {
      deviceApi.unsubscribeFromDevice(id)
    })
    subscribedDeviceIds.clear()
  }

  function attachStatusListener(): () => void {
    snapshotAttachCount += 1

    if (!unsubscribeSnapshot) {
      unsubscribeSnapshot = deviceApi.onSnapshot((payload) => {
        pushSnapshot(payload)
      })
    }
    syncSnapshotSubscriptions()

    return () => {
      snapshotAttachCount = Math.max(0, snapshotAttachCount - 1)
      if (snapshotAttachCount === 0 && unsubscribeSnapshot) {
        unsubscribeSnapshot()
        unsubscribeSnapshot = null
        cleanupSnapshotSubscriptions()
      }
    }
  }

  const formatValue = (id: string, channelIndex: number, rawValue: number): string => {
    const value = getDisplayValue(id, channelIndex, rawValue)
    if (!Number.isFinite(value)) return ''
    return value.toFixed(getChannelPrecision(id, channelIndex))
  }

  const getDisplayValue = (id: string, channelIndex: number, rawValue: number): number => {
    const offset = tareOffsets.value.get(id)?.[channelIndex] ?? 0
    return rawValue - offset
  }

  function setTare(id: string, channelIndex: number, value: number) {
    let deviceTares = tareOffsets.value.get(id)
    if (!deviceTares) {
      deviceTares = {}
      tareOffsets.value.set(id, deviceTares)
    }
    deviceTares[channelIndex] = value
  }

  function tareAllEnabled(id: string) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return
    const snapshot = latestSnapshots.value.find((s) => s.deviceId === id)
    if (!snapshot) return
    const indices = Array.isArray(snapshot.channelIndices) ? snapshot.channelIndices : []
    const channels = Array.isArray(snapshot.channels) ? snapshot.channels : []

    const profileChannels = Array.isArray(profile.channels) ? profile.channels : []
    profileChannels.forEach((channel) => {
      if (!channel.enabled) return
      const pos = indices.indexOf(channel.index)
      if (pos >= 0) {
        setTare(id, channel.index, channels[pos])
      }
    })
  }

  function getOffset(id: string, channelIndex: number): number {
    return tareOffsets.value.get(id)?.[channelIndex] ?? 0
  }

  function findChannel(id: string, channelIndex: number) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!Array.isArray(profile?.channels)) return undefined
    return profile.channels.find((c) => c.index === channelIndex)
  }

  function getChannelRange(id: string, channelIndex: number): { min: number; max: number } {
    const channel = findChannel(id, channelIndex)
    const min = channel?.rangeMin ?? -10
    const max = channel?.rangeMax ?? 10
    return min === max ? { min: min - 1, max: max + 1 } : { min, max }
  }

  function getChannelPrecision(id: string, channelIndex: number): number {
    return findChannel(id, channelIndex)?.precision ?? 3
  }

  function toggleChartSelection(id: string, channelIndex: number) {
    let selection = chartSelectedIndices.value.get(id)
    if (!selection) {
      selection = new Set()
      chartSelectedIndices.value.set(id, selection)
    }

    if (selection.has(channelIndex)) {
      selection.delete(channelIndex)
    } else if (selection.size < MAX_CHART_CHANNELS) {
      selection.add(channelIndex)
    }
  }

  function setAllChartSelection(id: string, enabled: boolean) {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return

    if (enabled) {
      const channels = Array.isArray(profile.channels) ? profile.channels : []
      const enabledIndices = channels.filter((ch) => ch.enabled).map((ch) => ch.index)
      chartSelectedIndices.value.set(id, new Set(enabledIndices.slice(0, MAX_CHART_CHANNELS)))
      return
    }

    chartSelectedIndices.value.set(id, new Set())
  }

  function isChartSelected(id: string, channelIndex: number): boolean {
    return chartSelectedIndices.value.get(id)?.has(channelIndex) ?? false
  }

  function initializeDefaultChartSelection(id: string): void {
    const current = chartSelectedIndices.value.get(id)
    if (current && current.size > 0) return

    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return

    const channels = Array.isArray(profile.channels) ? profile.channels : []
    const defaults = channels
      .filter((channel) => channel.enabled)
      .slice(0, 4)
      .map((channel) => channel.index)

    if (defaults.length > 0) {
      chartSelectedIndices.value.set(id, new Set(defaults))
    }
  }

  async function connect(id: string) {
    await deviceApi.connect(id)
    await refreshStatusFor(id)
  }

  async function disconnect(id: string) {
    await deviceApi.disconnect(id)
    deviceApi.unsubscribeFromDevice(id)
    subscribedDeviceIds.delete(id)
    await refreshStatusFor(id)
  }

  async function startAcquisition(id: string): Promise<boolean> {
    const result = await deviceApi.startAcquisition(id)
    if (result.success) {
      deviceApi.subscribeToDevice(id)
      subscribedDeviceIds.add(id)
      await refreshStatusFor(id)
    }
    return result.success
  }

  async function stopAcquisition(id: string): Promise<void> {
    await deviceApi.stopAcquisition(id)
    deviceApi.unsubscribeFromDevice(id)
    subscribedDeviceIds.delete(id)
    await refreshStatusFor(id)
  }

  async function getDsa3217ScanConfig(id: string): Promise<DSA3217ScanConfig | null> {
    try {
      const result = await deviceApi.getDsa3217ScanConfig(id)
      return result?.data ?? null
    } catch {
      return null
    }
  }

  async function applyDsa3217ScanConfig(
    id: string,
    config: { avg: number; period: number },
  ): Promise<DSA3217ScanConfig | null> {
    const result = await deviceApi.applyDsa3217ScanConfig(id, config)
    if (!result.success) {
      throw new Error(result.error || 'Failed to sync DSA3217 scan config')
    }
    return result.data ?? null
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
    refreshInstances,
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
    initializeDefaultChartSelection,
    connect,
    disconnect,
    startAcquisition,
    stopAcquisition,
    getDsa3217ScanConfig,
    applyDsa3217ScanConfig,
  }
})
