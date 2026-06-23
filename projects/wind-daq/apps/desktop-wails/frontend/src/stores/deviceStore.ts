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
    // 刷新所有设备的连接和采集状态，确保指示灯能正确显示
    await refreshAllStatuses()
  }

  /** 批量刷新所有已知设备的连接和采集状态 */
  async function refreshAllStatuses() {
    const ids = profiles.value.map((p) => p.id)
    if (ids.length === 0) return
    await Promise.allSettled(ids.map((id) => refreshStatusFor(id)))
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

    // 收到快照说明设备已连接且正在采集，同步更新设备状态
    const existingStatus = deviceStatuses.value.get(normalized.deviceId)
    if (!existingStatus || existingStatus.connection !== 'Connected' || !existingStatus.acquiring) {
      deviceStatuses.value.set(normalized.deviceId, {
        id: normalized.deviceId,
        name: existingStatus?.name ?? normalized.deviceId,
        type: existingStatus?.type ?? 'Unknown',
        connection: 'Connected',
        acquiring: true
      })
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
    const value = applyDisplayTare(id, channelIndex, rawValue)
    if (!Number.isFinite(value)) return ''
    return value.toFixed(getChannelPrecision(id, channelIndex))
  }

  const applyDisplayTare = (id: string, channelIndex: number, rawValue: number): number => {
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
    // 乐观更新：在调用后端连接 API 之前，先将状态切换为 Connecting，
    // 这样 UI 可以立即显示"连接中"提示，避免按下按钮后无任何反馈。
    const prev = deviceStatuses.value.get(id)
    const profile = profiles.value.find((p) => p.id === id)
    deviceStatuses.value.set(id, {
      id,
      name: prev?.name ?? profile?.name ?? id,
      type: prev?.type ?? profile?.type ?? 'Unknown',
      connection: 'Connecting',
      acquiring: prev?.acquiring ?? false,
      lastError: undefined,
    })
    try {
      await deviceApi.connect(id)
      // 连接调用返回后，再以后端真实状态为准
      try {
        await refreshStatusFor(id)
      } catch (refreshErr) {
        // refreshStatusFor 失败不意味着连接失败——设备已连接成功，
        // 仅状态刷新暂时不可用，下一次 StatusPoll 轮询会自行修复。
        console.warn(`[deviceStore] connect succeeded but refreshStatusFor failed for ${id}:`, refreshErr)
      }
    } catch (err) {
      // 连接失败：回滚到 Error 状态，便于上层 UI 复位按钮并提示
      const current = deviceStatuses.value.get(id)
      deviceStatuses.value.set(id, {
        id,
        name: current?.name ?? profile?.name ?? id,
        type: current?.type ?? profile?.type ?? 'Unknown',
        connection: 'Error',
        acquiring: current?.acquiring ?? false,
        lastError: err instanceof Error ? err.message : String(err),
      })
      throw err
    }
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

  // ==================== 全局采集编排 ====================

  const isAnyAcquiring = computed(() =>
    profiles.value.some((p) => acquiringFor(p.id)),
  )

  async function startAllAcquisitions(): Promise<void> {
    const connectedIds = profiles.value
      .filter((p) => statusFor(p.id) === 'Connected')
      .map((p) => p.id)
    const targetIds = connectedIds.length > 0
      ? connectedIds
      : [selectedDeviceId.value ?? 'sim-1']

    // 先把未连接的设备并行连一遍；只对最终处于 Connected 的设备启动采集，
    // 避免对连接失败的设备发出注定失败的 startAcquisition、并误导调用方
    // (例如 MainDashboardView 误判已开始采集而进入录制流程)。
    const needConnect = targetIds.filter((id) => statusFor(id) !== 'Connected')
    if (needConnect.length > 0) {
      const connectResults = await Promise.allSettled(needConnect.map((id) => connect(id)))
      connectResults.forEach((result, idx) => {
        if (result.status === 'rejected') {
          const id = needConnect[idx]
          const profile = profiles.value.find((p) => p.id === id)
          console.warn(`[startAll] 设备 "${profile?.name ?? id}" 连接失败:`, result.reason)
        }
      })
    }

    const startIds = targetIds.filter((id) => statusFor(id) === 'Connected')
    if (startIds.length === 0) {
      console.warn('[startAll] 没有可启动采集的已连接设备，跳过 startAcquisition')
      await refreshInstances()
      return
    }
    const results = await Promise.allSettled(startIds.map((id) => startAcquisition(id)))
    results.forEach((result, idx) => {
      if (result.status === 'rejected') {
        const profile = profiles.value.find((p) => p.id === startIds[idx])
        console.warn(`[startAll] 设备 "${profile?.name ?? startIds[idx]}" 启动采集失败:`, result.reason)
      }
    })
    await refreshInstances()
  }

  async function stopAllAcquisitions(): Promise<void> {
    const acquiringIds = profiles.value
      .filter((p) => acquiringFor(p.id))
      .map((p) => p.id)
    if (acquiringIds.length === 0) return
    const results = await Promise.allSettled(acquiringIds.map((id) => stopAcquisition(id)))
    results.forEach((result, idx) => {
      if (result.status === 'rejected') {
        const profile = profiles.value.find((p) => p.id === acquiringIds[idx])
        console.warn(`[stopAll] 设备 "${profile?.name ?? acquiringIds[idx]}" 停止采集失败:`, result.reason)
      }
    })
    await refreshInstances()
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
    isAnyAcquiring,
    startAllAcquisitions,
    stopAllAcquisitions,
    selectDevice,
    refreshProfiles,
    refreshInstances,
    refreshAllStatuses,
    refreshStatusFor,
    updateStatus,
    pushSnapshot,
    attachStatusListener,
    latestFor,
    historyFor,
    formatValue,
    applyDisplayTare,
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
