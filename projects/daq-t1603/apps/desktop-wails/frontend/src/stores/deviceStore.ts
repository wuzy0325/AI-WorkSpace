import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as bridge from '@bridge/deviceBridge'
import type { TemperatureProfile, TemperatureSnapshot, T1603Config, ChannelConfig, ScanResult } from '@bridge/deviceBridge'

const MAX_HISTORY = 200

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
  }
}

export const useDeviceStore = defineStore('device', () => {
  const profiles = ref<TemperatureProfile[]>([])
  const selectedId = ref<string | null>(null)
  const statusMap = ref<Record<string, string>>({})
  const historyMap = ref<Record<string, TemperatureSnapshot[]>>({})
  const snapshotMap = ref<Record<string, TemperatureSnapshot>>({})
  const chartSelections = ref<Record<string, Set<number>>>({})
  const scanResults = ref<ScanResult[]>([])
  const isScanning = ref(false)

  const selectedProfile = computed(() =>
    profiles.value.find((p) => p.id === selectedId.value) ?? null
  )
  const selectedSnapshot = computed(() =>
    selectedId.value ? snapshotMap.value[selectedId.value] ?? null : null
  )

  function selectDevice(id: string): void {
    selectedId.value = id
  }

  function statusFor(id: string): string {
    return statusMap.value[id] ?? 'Disconnected'
  }

  function acquiringFor(id: string): boolean {
    return statusMap.value[id] === 'Acquiring'
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
    if (!historyMap.value[snapshot.deviceId]) {
      historyMap.value[snapshot.deviceId] = []
    }
    const history = historyMap.value[snapshot.deviceId]!
    history.push(snapshot)
    if (history.length > MAX_HISTORY) {
      history.splice(0, history.length - MAX_HISTORY)
    }
  }

  async function loadProfiles(): Promise<void> {
    const list = await bridge.getProfiles()
    if (Array.isArray(list)) {
      profiles.value = list
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
    await transitionStatus(id, () => bridge.startAcquisition(id), 'Acquiring', 'Connected', 'Starting')
  }

  async function stopAcquisition(id: string): Promise<void> {
    await transitionStatus(id, () => bridge.stopAcquisition(id), 'Connected', 'Connected')
  }

  async function applyConfig(id: string, cfg: Partial<T1603Config>): Promise<void> {
    await bridge.applyConfig(id, t1603Defaults(cfg))
  }

  async function updateT1603Config(id: string, cfg: Partial<T1603Config>): Promise<void> {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return
    const merged = { ...defaultT1603Config(), ...profile.t1603Config, ...cfg }
    profile.t1603Config = merged
    await bridge.upsertProfile(profile)
    const status = statusMap.value[id]
    if (status === 'Connected' || status === 'Acquiring') {
      await bridge.applyConfig(id, merged)
    }
  }

  async function updateChannel(id: string, index: number, patch: Partial<ChannelConfig>): Promise<void> {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile || index < 0 || index >= profile.channels.length) return
    const ch = profile.channels[index]!
    Object.assign(ch, patch)
    await bridge.upsertProfile(profile)
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
    profiles.value.push(profile)
    await bridge.upsertProfile(profile)
  }

  async function removeProfile(id: string): Promise<void> {
    await bridge.deleteProfile(id)
    profiles.value = profiles.value.filter((p) => p.id !== id)
    if (selectedId.value === id) {
      selectedId.value = null
    }
  }

  return {
    profiles, selectedId, statusMap, historyMap, snapshotMap, chartSelections,
    scanResults, isScanning,
    selectedProfile, selectedSnapshot,
    selectDevice, statusFor, acquiringFor, historyFor, isChartSelected, toggleChartSelection,
    pushSnapshot, loadProfiles, connect, disconnect,
    startAcquisition, stopAcquisition, applyConfig, updateT1603Config, updateChannel,
    clearScanResults, scanDevices, addProfile, removeProfile,
  }
})
