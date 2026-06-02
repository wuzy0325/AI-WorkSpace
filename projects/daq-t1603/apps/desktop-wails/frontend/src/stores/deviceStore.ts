import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as bridge from '@bridge/deviceBridge'
import type { TemperatureProfile, TemperatureSnapshot, T1603Config, ChannelConfig } from '@bridge/deviceBridge'

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
  }))
}

function defaultT1603Config(): T1603Config {
  return { thermocoupleType: 'K', coldJunction: 'internal', filterHz: 50 }
}

function t1603Defaults(cfg: Partial<T1603Config>): T1603Config {
  return {
    thermocoupleType: cfg.thermocoupleType ?? 'K',
    coldJunction: cfg.coldJunction ?? 'internal',
    filterHz: cfg.filterHz ?? 50,
  }
}

export const useDeviceStore = defineStore('device', () => {
  const profiles = ref<TemperatureProfile[]>([])
  const selectedId = ref<string | null>(null)
  const statusMap = ref<Record<string, string>>({})
  const historyMap = ref<Record<string, TemperatureSnapshot[]>>({})
  const snapshotMap = ref<Record<string, TemperatureSnapshot>>({})
  const chartSelections = ref<Record<string, Set<number>>>({})

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

  async function connect(id: string): Promise<void> {
    const prev = statusMap.value[id]
    statusMap.value[id] = 'Connecting'
    try {
      await bridge.connect(id)
      statusMap.value[id] = 'Connected'
    } catch (err) {
      statusMap.value[id] = prev ?? 'Disconnected'
      throw err
    }
  }

  async function disconnect(id: string): Promise<void> {
    const prev = statusMap.value[id]
    try {
      await bridge.disconnect(id)
      statusMap.value[id] = 'Disconnected'
    } catch (err) {
      statusMap.value[id] = prev ?? 'Disconnected'
      throw err
    }
  }

  async function startAcquisition(id: string): Promise<void> {
    const prev = statusMap.value[id]
    try {
      await bridge.startAcquisition(id)
      statusMap.value[id] = 'Acquiring'
    } catch (err) {
      statusMap.value[id] = prev ?? 'Connected'
      throw err
    }
  }

  async function stopAcquisition(id: string): Promise<void> {
    const prev = statusMap.value[id]
    try {
      await bridge.stopAcquisition(id)
      statusMap.value[id] = 'Connected'
    } catch (err) {
      statusMap.value[id] = prev ?? 'Connected'
      throw err
    }
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
    selectedProfile, selectedSnapshot,
    selectDevice, statusFor, acquiringFor, historyFor, isChartSelected, toggleChartSelection,
    pushSnapshot, loadProfiles, connect, disconnect,
    startAcquisition, stopAcquisition, applyConfig, updateT1603Config, updateChannel,
    addProfile, removeProfile,
  }
})
