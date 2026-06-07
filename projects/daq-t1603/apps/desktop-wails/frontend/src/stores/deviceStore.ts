import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as bridge from '@bridge/deviceBridge'
import type { TemperatureProfile, TemperatureSnapshot, T1603Config, ChannelConfig, ScanResult } from '@bridge/deviceBridge'

const MAX_HISTORY = 200
const ACQUISITION_ACTION_TIMEOUT_MS = 8000
const APPLY_CONFIG_TIMEOUT_MS = 15000
const DISPLAY_REFRESH_RATE_FALLBACK_HZ = 10

const CHANNEL_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#84cc16', '#14b8a6', '#d946ef', '#0ea5e9',
  '#eab308', '#22c55e', '#ef4444', '#8b5cf6',
]

function defaultChannels() {
  return Array.from({ length: 16 }, (_, i) => ({
    index: i,
    name: `通道 ${i + 1}`,
    enabled: true,
    unit: '°C',
    color: CHANNEL_COLORS[i % CHANNEL_COLORS.length],
    precision: 2,
    rangeMin: 0,
    rangeMax: 200,
    thermocoupleType: 'K',
  }))
}

function defaultT1603Config(): T1603Config {
  return {
    thermocoupleTypes: 'KKKKKKKKKKKKKKKK',
    channelMask: 'FFFF',
    samplingRate: 10,
    averageCount: 4,
    showTimestamp: false,
    showSequence: false,
    autoConnect: false,
  }
}

function t1603Defaults(cfg: Partial<T1603Config>): T1603Config {
  return {
    thermocoupleTypes: cfg.thermocoupleTypes ?? 'KKKKKKKKKKKKKKKK',
    channelMask: cfg.channelMask ?? 'FFFF',
    samplingRate: cfg.samplingRate ?? 10,
    averageCount: cfg.averageCount ?? 4,
    showTimestamp: cfg.showTimestamp ?? false,
    showSequence: cfg.showSequence ?? false,
    autoConnect: cfg.autoConnect ?? false,
  }
}

export const useDeviceStore = defineStore('device', () => {
  const profiles = ref<TemperatureProfile[]>([])
  const selectedId = ref<string | null>(null)
  const statusMap = ref<Record<string, string>>({})
  const historyMap = ref<Record<string, TemperatureSnapshot[]>>({})
  const snapshotMap = ref<Record<string, TemperatureSnapshot>>({})
  const pendingSnapshotMap = ref<Record<string, TemperatureSnapshot>>({})
  const chartSelections = ref<Record<string, Set<number>>>({})
  const scanResults = ref<ScanResult[]>([])
  const isScanning = ref(false)
  const displayFlushTimer = ref<ReturnType<typeof setInterval> | null>(null)

  const selectedProfile = computed(() =>
    profiles.value.find((p) => p.id === selectedId.value) ?? null
  )
  const selectedSnapshot = computed(() =>
    selectedId.value ? snapshotMap.value[selectedId.value] ?? null : null
  )

  function syncSelectedDevice(): void {
    if (profiles.value.length === 0) {
      selectedId.value = null
      return
    }

    if (selectedId.value && profiles.value.some((p) => p.id === selectedId.value)) {
      return
    }

    selectedId.value = profiles.value[0]!.id
  }

  function selectDevice(id: string): void {
    selectedId.value = id
  }

  function statusFor(id: string): string {
    return statusMap.value[id] ?? 'Disconnected'
  }

  function acquiringFor(id: string): boolean {
    return statusMap.value[id] === 'Acquiring'
  }

  async function withTimeout<T>(promise: Promise<T>, timeoutMs: number, message: string): Promise<T> {
    let timer: ReturnType<typeof setTimeout> | null = null
    try {
      return await Promise.race([
        promise,
        new Promise<T>((_, reject) => {
          timer = setTimeout(() => reject(new Error(message)), timeoutMs)
        }),
      ])
    } finally {
      if (timer !== null) clearTimeout(timer)
    }
  }

  function historyFor(id: string): TemperatureSnapshot[] {
    return historyMap.value[id] ?? []
  }

  function isChartSelected(id: string, channelIndex: number): boolean {
    return chartSelections.value[id]?.has(channelIndex) ?? false
  }

  function toggleChartSelection(id: string, channelIndex: number): void {
    if (!chartSelections.value[id]) {
      chartSelections.value[id] = new Set()
    }
    const set = chartSelections.value[id]!
    if (set.has(channelIndex)) {
      set.delete(channelIndex)
    } else {
      set.add(channelIndex)
    }
  }

  function pushSnapshot(snapshot: TemperatureSnapshot): void {
    snapshotMap.value[snapshot.deviceId] = snapshot
    pendingSnapshotMap.value[snapshot.deviceId] = snapshot
  }

  function flushPendingSnapshots(): void {
    const pendingEntries = Object.entries(pendingSnapshotMap.value)
    if (pendingEntries.length === 0) return

    for (const [deviceId, snapshot] of pendingEntries) {
      if (!historyMap.value[deviceId]) {
        historyMap.value[deviceId] = []
      }
      const history = historyMap.value[deviceId]!
      history.push(snapshot)
      if (history.length > MAX_HISTORY) {
        history.splice(0, history.length - MAX_HISTORY)
      }
    }
    pendingSnapshotMap.value = {}
  }

  function setDisplayRefreshRateHz(rateHz: number): void {
    if (!Number.isFinite(rateHz) || rateHz <= 0) return
    if (displayFlushTimer.value !== null) {
      clearInterval(displayFlushTimer.value)
      displayFlushTimer.value = null
    }
    const intervalMs = Math.max(16, Math.round(1000 / rateHz))
    displayFlushTimer.value = setInterval(flushPendingSnapshots, intervalMs)
  }

  function stopDisplayFlush(): void {
    if (displayFlushTimer.value !== null) {
      clearInterval(displayFlushTimer.value)
      displayFlushTimer.value = null
    }
    flushPendingSnapshots()
  }

  setDisplayRefreshRateHz(DISPLAY_REFRESH_RATE_FALLBACK_HZ)

  /** 初始化指定设备的通道选择为全选 */
  function initChartSelections(id: string, channels: ChannelConfig[]): void {
    const set = new Set<number>()
    for (const ch of channels) {
      if (ch.enabled) set.add(ch.index)
    }
    chartSelections.value[id] = set
  }

  async function loadProfiles(): Promise<void> {
    const list = await bridge.getProfiles()
    if (Array.isArray(list)) {
      profiles.value = list
      // 加载配置后默认全选所有已启用通道
      for (const p of list) {
        if (!chartSelections.value[p.id] || chartSelections.value[p.id]!.size === 0) {
          initChartSelections(p.id, p.channels)
        }
      }
      syncSelectedDevice()
    }
  }

  /** 自动连接所有开启了自动连接的设备 */
  async function autoConnectAll(): Promise<void> {
    const targets = profiles.value.filter(
      (p) => p.t1603Config?.autoConnect && statusFor(p.id) === 'Disconnected'
    )
    const results = await Promise.allSettled(targets.map(async (profile) => {
      await connect(profile.id)
      return profile
    }))

    const failures = results.flatMap((result, index) => {
      if (result.status === 'fulfilled') return []
      const profile = targets[index]
      const reason = result.reason instanceof Error ? result.reason.message : String(result.reason)
      return [{ profile, reason }]
    })

    if (failures.length > 0) {
      throw new Error(
        failures.map(({ profile, reason }) => `${profile.name || profile.id}: ${reason}`).join('; ')
      )
    }
  }

  async function transitionStatus(
    id: string,
    action: () => Promise<void>,
    targetStatus: string,
    fallbackStatus: string,
    preStatus?: string,
  ): Promise<void> {
    const prev = statusMap.value[id]
    if (preStatus) statusMap.value[id] = preStatus
    try {
      await action()
      statusMap.value[id] = targetStatus
    } catch (err) {
      statusMap.value[id] = prev ?? fallbackStatus
      throw err
    }
  }

  async function connect(id: string): Promise<void> {
    await transitionStatus(id, () => bridge.connect(id), 'Connected', 'Disconnected', 'Connecting')
  }

  async function disconnect(id: string): Promise<void> {
    await transitionStatus(id, () => bridge.disconnect(id), 'Disconnected', 'Disconnected')
  }

  async function startAcquisition(id: string): Promise<void> {
    try {
      await transitionStatus(
        id,
        () => withTimeout(bridge.startAcquisition(id), ACQUISITION_ACTION_TIMEOUT_MS, 'Start acquisition timed out'),
        'Acquiring',
        'Connected',
        'Starting',
      )
    } catch (err) {
      const status = await bridge.getStatus(id).catch(() => false)
      if (status && typeof status === 'object' && 'status' in status) {
        const numericStatus = Number(status.status)
        statusMap.value[id] = numericStatus === 1
          ? 'Connected'
          : numericStatus === 2
            ? 'Acquiring'
            : 'Disconnected'
      }
      throw err
    }
  }

  async function stopAcquisition(id: string): Promise<void> {
    try {
      await transitionStatus(
        id,
        () => withTimeout(bridge.stopAcquisition(id), ACQUISITION_ACTION_TIMEOUT_MS, 'Stop acquisition timed out'),
        'Connected',
        'Connected',
        'Stopping',
      )
    } catch (err) {
      const status = await bridge.getStatus(id).catch(() => false)
      if (status && typeof status === 'object' && 'status' in status) {
        const numericStatus = Number(status.status)
        statusMap.value[id] = numericStatus === 1
          ? 'Connected'
          : numericStatus === 2
            ? 'Acquiring'
            : 'Disconnected'
      }
      throw err
    }
  }

  async function applyConfig(id: string, cfg: Partial<T1603Config>): Promise<void> {
    await withTimeout(
      bridge.applyConfig(id, t1603Defaults(cfg)),
      APPLY_CONFIG_TIMEOUT_MS,
      '应用配置超时，设备可能无响应',
    )
  }

  async function updateT1603Config(id: string, cfg: Partial<T1603Config>): Promise<void> {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return
    const merged = { ...defaultT1603Config(), ...profile.t1603Config, ...cfg }
    // 先更新本地状态，再持久化到后端
    profile.t1603Config = merged
    try {
      await bridge.upsertProfile(profile)
    } catch {
      // 持久化失败时重新加载配置，恢复本地状态一致性
      await loadProfiles()
      throw new Error('保存配置失败')
    }
    // 仅在已连接且非采集状态时应用硬件配置，采集时硬件会拒绝
    const status = statusMap.value[id]
    if (status === 'Connected') {
      await withTimeout(
        bridge.applyConfig(id, merged),
        APPLY_CONFIG_TIMEOUT_MS,
        '应用配置超时，设备可能无响应',
      )
    }
  }

  async function updateChannel(id: string, index: number, patch: Partial<ChannelConfig>): Promise<void> {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile || index < 0 || index >= profile.channels.length) return
    // 先更新本地状态，再持久化到后端
    const ch = profile.channels[index]!
    Object.assign(ch, patch)
    try {
      await bridge.upsertProfile(profile)
    } catch {
      // 持久化失败时重新加载配置，恢复本地状态一致性
      await loadProfiles()
      throw new Error('保存配置失败')
    }
  }

  async function saveProfile(profile: TemperatureProfile): Promise<void> {
    // 先更新本地状态，再持久化到后端（利用 await 的微任务边界让 Vue 处理 watcher）
    const index = profiles.value.findIndex((p) => p.id === profile.id)
    if (index >= 0) {
      profiles.value[index] = profile
    } else {
      profiles.value.push(profile)
    }
    try {
      await bridge.upsertProfile(profile)
    } catch {
      // 持久化失败时重新加载配置，恢复本地状态一致性
      await loadProfiles()
      throw new Error('保存配置失败')
    }
  }

  function clearScanResults(): void {
    scanResults.value = []
  }

  async function scanDevices(): Promise<void> {
    isScanning.value = true
    try {
      scanResults.value = await bridge.scanDevices()
    } finally {
      isScanning.value = false
    }
  }

  async function addProfile(name: string, address: string, port: number): Promise<void> {
    const id = `t1603_${Date.now()}`
    const profile: TemperatureProfile = {
      id,
      name,
      address,
      port,
      samplingRate: 5,
      channels: defaultChannels(),
      t1603Config: defaultT1603Config(),
      createdAt: Date.now(),
    }
    // 先更新本地状态，再持久化到后端
    profiles.value.push(profile)
    // 新增设备时默认全选所有通道
    initChartSelections(id, profile.channels)
    syncSelectedDevice()
    try {
      await bridge.upsertProfile(profile)
    } catch {
      // 持久化失败时重新加载配置，恢复本地状态一致性
      await loadProfiles()
      throw new Error('保存配置失败')
    }
  }

  async function removeProfile(id: string): Promise<void> {
    await bridge.deleteProfile(id)
    profiles.value = profiles.value.filter((p) => p.id !== id)
    syncSelectedDevice()
  }

  return {
    profiles, selectedId, statusMap, historyMap, snapshotMap, chartSelections,
    scanResults, isScanning,
    selectedProfile, selectedSnapshot,
    selectDevice, statusFor, acquiringFor, historyFor, isChartSelected, toggleChartSelection,
    pushSnapshot, loadProfiles, autoConnectAll, connect, disconnect,
    startAcquisition, stopAcquisition, applyConfig, updateT1603Config, updateChannel, saveProfile,
    clearScanResults, scanDevices, addProfile, removeProfile,
    setDisplayRefreshRateHz, stopDisplayFlush,
  }
})
