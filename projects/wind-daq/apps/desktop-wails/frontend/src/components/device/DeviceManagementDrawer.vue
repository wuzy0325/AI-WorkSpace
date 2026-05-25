<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { deviceApi } from '@api/deviceApi'
import type { DeviceProfile, DeviceType, ScanResult, ChannelConfig } from '@api/types'
import UiSelect from '@components/ui/UiSelect.vue'
import DaqT1603Config from '@components/device/DaqT1603Config.vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()

const scanning = ref(false)
const discovered = ref<ScanResult[]>([])
const selectedIds = ref<string[]>([])

type EditorMode = 'create' | 'edit'
type EditorTab = 'basic' | 'channels'

const editorOpen = ref(false)
const editorMode = ref<EditorMode>('create')
const editorTab = ref<EditorTab>('basic')
const draft = ref<DeviceProfile>(createBlankProfile('SIMULATED'))
const initialDraftSnapshot = ref('')
const saving = ref(false)
const saveError = ref<string | null>(null)

const deviceUnit = ref('')
const devicePrecision = ref<number | null>(null)
const deviceRangeMin = ref<number | null>(null)
const deviceRangeMax = ref<number | null>(null)
const enabledOnlyChannels = ref(false)
const channelKeyword = ref('')
const enableAtmospheric = ref(false)

const PRESSURE_UNIT_OPTIONS = ['Pa', 'kPa', 'MPa', 'psi', 'kgf/cm2'] as const

const WTN_PXI_CHANNEL_NAMES = [
  '球罐压力', '球罐总压', '球罐静压',
  '球罐温度1', '球罐温度2', '球罐温度3', '球罐温度4', '球罐温度5',
] as const

const deviceTypeOptions = computed(() => [
  { value: 'SIMULATED', label: 'SIMULATED' },
  { value: 'DAQ-P-1604', label: 'DAQ-P-1604' },
  { value: 'DAQ-P-1064Pre', label: 'DAQ-P-1064Pre' },
  { value: 'DAQ-T-1603', label: 'DAQ-T-1603' },
  { value: 'WTN_PXI', label: 'WTN_PXI' },
  { value: 'DSA3217', label: 'DSA3217' },
])

const transportOptions = computed(() => [
  { value: 'tcp', label: 'TCP/IP' },
  { value: 'serial', label: '串口 RS232' },
])

const pressureUnitOptions = computed(() =>
  PRESSURE_UNIT_OPTIONS.map((u) => ({ value: u, label: u }))
)

function isTcpType(t: DeviceType): boolean {
  return t === 'DAQ-P-1604' || t === 'DAQ-P-1064Pre' || t === 'DAQ-T-1603' || t === 'WTN_PXI' || t === 'DSA3217'
}

function supportsTransportSwitch(t: DeviceType): boolean {
  return t !== 'WTN_PXI' && t !== 'DAQ-P-1604' && t !== 'DAQ-P-1064Pre' && t !== 'DSA3217'
}

function isTempUnitFixed(t: DeviceType): boolean {
  return t === 'DAQ-T-1603'
}

function isWtnPxiType(t: DeviceType): boolean {
  return t === 'WTN_PXI'
}

function normalizeDeviceUnit(type: DeviceType, unit: string): string {
  if (isTempUnitFixed(type)) return '℃'
  if (isWtnPxiType(type)) return ''
  return unit.trim()
}

function getDeviceUnitFromChannels(channels: ChannelConfig[]): string {
  const count = Math.min(16, channels.length)
  if (count <= 0) return ''
  const first = channels[0]?.unit?.trim() ?? ''
  for (let i = 1; i < count; i++) {
    if ((channels[i]?.unit?.trim() ?? '') !== first) return ''
  }
  return first
}

function getDevicePrecisionFromChannels(channels: ChannelConfig[]): number | null {
  const count = Math.min(16, channels.length)
  if (count <= 0) return null
  let base: number | null = null
  for (let i = 0; i < count; i++) {
    const p = channels[i]?.precision
    if (typeof p !== 'number' || !Number.isFinite(p)) return null
    if (base === null) base = p
    else if (p !== base) return null
  }
  return base
}

function getDeviceRangeFromChannels(channels: ChannelConfig[]): { min: number; max: number } | null {
  const count = Math.min(16, channels.length)
  if (count <= 0) return null
  let baseMin: number | null = null
  let baseMax: number | null = null
  for (let i = 0; i < count; i++) {
    const ch = channels[i]
    if (!Number.isFinite(ch.rangeMin ?? NaN) || !Number.isFinite(ch.rangeMax ?? NaN)) return null
    if (baseMin === null || baseMax === null) {
      baseMin = Number(ch.rangeMin)
      baseMax = Number(ch.rangeMax)
    } else if (ch.rangeMin !== baseMin || ch.rangeMax !== baseMax) return null
  }
  return baseMin !== null && baseMax !== null ? { min: baseMin, max: baseMax } : null
}

function applyFixedUnitsIfNeeded(type: DeviceType, channels: ChannelConfig[]): void {
  if (isWtnPxiType(type) && channels.length >= 8) {
    const units = ['Pa', 'Pa', 'Pa', '℃', '℃', '℃', '℃', '℃']
    for (let i = 0; i < 8; i++) {
      channels[i].name = WTN_PXI_CHANNEL_NAMES[i] ?? `CH${i + 1}`
      channels[i].unit = units[i]
      channels[i].enabled = true
    }
    return
  }
  const has18 = (type === 'SIMULATED' || type === 'DAQ-P-1604' || type === 'DAQ-P-1064Pre') && channels.length >= 18
  if (!has18) return
  if (channels[16]) channels[16].unit = 'Pa'
  if (channels[17]) channels[17].unit = '℃'
}

function applyDeviceUnitToChannels(unit: string): void {
  if (isWtnPxiType(draft.value.type)) {
    applyFixedUnitsIfNeeded(draft.value.type, draft.value.channels)
    return
  }
  const u = normalizeDeviceUnit(draft.value.type, unit)
  const count = Math.min(16, draft.value.channels.length)
  for (let i = 0; i < count; i++) {
    draft.value.channels[i].unit = u
  }
  applyFixedUnitsIfNeeded(draft.value.type, draft.value.channels)
}

function applyDevicePrecisionToChannels(precision: number | null): void {
  if (precision == null || !Number.isFinite(precision)) return
  const value = Math.max(0, Math.floor(precision))
  const count = Math.min(16, draft.value.channels.length)
  for (let i = 0; i < count; i++) {
    draft.value.channels[i].precision = value
  }
}

function applyDeviceRangeToChannels(min: number | null, max: number | null): void {
  if (min == null || max == null || !Number.isFinite(min) || !Number.isFinite(max)) return
  const count = Math.min(16, draft.value.channels.length)
  for (let i = 0; i < count; i++) {
    draft.value.channels[i].rangeMin = Number(min)
    draft.value.channels[i].rangeMax = Number(max)
  }
}

function createDefaultChannels(type: DeviceType): ChannelConfig[] {
  switch (type) {
    case 'DAQ-T-1603':
      return Array.from({ length: 16 }, (_, i) => ({
        index: i, name: `TC${i + 1}`, enabled: true, unit: 'degC', precision: 2,
      }))
    case 'DAQ-P-1604':
      return [
        ...Array.from({ length: 16 }, (_, i) => ({
          index: i, name: `CH${i + 1}`, enabled: true, unit: 'Pa', precision: 2, rangeMin: -5000, rangeMax: 5000,
        })),
        { index: 16, name: '大气压', enabled: false, unit: 'Pa', precision: 2 },
        { index: 17, name: '大气温度', enabled: false, unit: 'degC', precision: 2 },
      ]
    case 'DAQ-P-1064Pre':
    case 'DSA3217':
      return Array.from({ length: 16 }, (_, i) => ({
        index: i, name: `CH${i + 1}`, enabled: true, unit: 'Pa', precision: 2, rangeMin: -5000, rangeMax: 5000,
      }))
    case 'WTN_PXI': {
      const names = ['球罐压力', '球罐总压', '球罐静压', '球罐温度1', '球罐温度2', '球罐温度3', '球罐温度4', '球罐温度5']
      const units = ['Pa', 'Pa', 'Pa', 'degC', 'degC', 'degC', 'degC', 'degC']
      return Array.from({ length: 8 }, (_, i) => ({
        index: i, name: names[i], enabled: true, unit: units[i], precision: 2,
      }))
    }
    default:
      return Array.from({ length: 4 }, (_, i) => ({
        index: i, name: `CH${i + 1}`, enabled: true, unit: 'V', precision: 3, rangeMin: -10, rangeMax: 10,
      }))
  }
}

function createBlankProfile(type: DeviceType): DeviceProfile {
  const id = globalThis.crypto.randomUUID()
  const channels = createDefaultChannels(type)
  let address = '127.0.0.1'
  let port = 0
  if (type === 'DAQ-P-1604' || type === 'DAQ-T-1603' || type === 'WTN_PXI') {
    address = '192.168.3.101'
    port = 9000
  }
  if (type === 'DAQ-P-1064Pre') {
    address = '192.168.1.100'
    port = 5000
  }
  if (type === 'DSA3217') {
    address = '192.168.1.254'
    port = 5000
  }
  return {
    id, name: '', type, transport: 'tcp', address, port,
    serialPort: '', baudRate: 115200, samplingRate: 20,
    autoConnect: true, channels,
    daqT1603Config: type === 'DAQ-T-1603'
      ? { thermocoupleType: 'K', coldJunction: 'internal', filterHz: 50 }
      : undefined,
  }
}

function cloneProfile(p: DeviceProfile): DeviceProfile {
  try { return structuredClone(p) } catch { return JSON.parse(JSON.stringify(p)) }
}

function snapshotDraft(p: DeviceProfile): string {
  return JSON.stringify(p)
}

const isDirty = computed(() => snapshotDraft(draft.value) !== initialDraftSnapshot.value)
const isReadOnly = computed(() => editorMode.value === 'edit' && deviceStore.acquiringFor(draft.value.id))

interface DraftFieldErrors {
  name?: string
  address?: string
  port?: string
  serialPort?: string
  baudRate?: string
  samplingRate?: string
}

const fieldErrors = computed<DraftFieldErrors>(() => {
  const errors: DraftFieldErrors = {}
  const p = draft.value
  if (!p.name.trim()) errors.name = '设备名称不能为空'
  else {
    const hasDup = deviceStore.profiles.some((e) => e.id !== p.id && e.name.trim() === p.name.trim())
    if (hasDup) errors.name = '设备名称已存在'
  }
  if (isTcpType(p.type)) {
    if (p.transport === 'serial') {
      if (!p.serialPort?.trim()) errors.serialPort = '串口号不能为空'
      if (!Number.isFinite(p.baudRate ?? 0) || (p.baudRate ?? 0) <= 0) errors.baudRate = '波特率无效'
    } else {
      if (!p.address?.trim()) errors.address = 'IP 地址不能为空'
      if (!Number.isFinite(p.port ?? 0) || (p.port ?? 0) <= 0) errors.port = '端口号无效'
    }
  }
  if (!Number.isFinite(p.samplingRate) || p.samplingRate <= 0) errors.samplingRate = '采样率无效'
  return errors
})

const validationErrorCount = computed(() =>
  Object.values(fieldErrors.value).filter((v): v is string => Boolean(v)).length
)

function getFirstDraftError(errors: DraftFieldErrors): string | null {
  for (const key of ['name', 'address', 'port', 'serialPort', 'baudRate', 'samplingRate'] as const) {
    if (errors[key]) return errors[key]
  }
  return null
}

const channelRows = computed(() => {
  const kw = channelKeyword.value.trim().toLowerCase()
  return draft.value.channels
    .map((channel, originalIndex) => ({ channel, originalIndex }))
    .filter(({ channel }) => !enabledOnlyChannels.value || channel.enabled)
    .filter(({ channel }) => {
      if (!kw) return true
      return channel.name.toLowerCase().includes(kw) || `${channel.index + 1}`.includes(kw)
    })
})

function openCreate(type: DeviceType = 'SIMULATED') {
  editorMode.value = 'create'
  editorTab.value = 'basic'
  draft.value = createBlankProfile(type)
  deviceUnit.value = normalizeDeviceUnit(draft.value.type, getDeviceUnitFromChannels(draft.value.channels))
  const range = getDeviceRangeFromChannels(draft.value.channels)
  deviceRangeMin.value = range?.min ?? null
  deviceRangeMax.value = range?.max ?? null
  devicePrecision.value = getDevicePrecisionFromChannels(draft.value.channels)
  applyFixedUnitsIfNeeded(draft.value.type, draft.value.channels)
  applyDeviceUnitToChannels(deviceUnit.value)
  enabledOnlyChannels.value = false
  channelKeyword.value = ''
  enableAtmospheric.value = draft.value.type === 'DAQ-P-1604' ? false : (draft.value.channels[16]?.enabled !== false)
  if (draft.value.type === 'DAQ-P-1604' && draft.value.channels.length > 16) {
    draft.value.channels[16] = { ...draft.value.channels[16], enabled: false }
    draft.value.channels[17] = { ...draft.value.channels[17], enabled: false }
  }
  initialDraftSnapshot.value = snapshotDraft(draft.value)
  saveError.value = null
  editorOpen.value = true
}

function openEdit(p: DeviceProfile) {
  editorMode.value = 'edit'
  editorTab.value = 'basic'
  draft.value = cloneProfile(p)
  if (draft.value.type === 'WTN_PXI') draft.value.transport = 'tcp'
  deviceUnit.value = normalizeDeviceUnit(draft.value.type, getDeviceUnitFromChannels(draft.value.channels))
  const range = getDeviceRangeFromChannels(draft.value.channels)
  deviceRangeMin.value = range?.min ?? null
  deviceRangeMax.value = range?.max ?? null
  devicePrecision.value = getDevicePrecisionFromChannels(draft.value.channels)
  applyFixedUnitsIfNeeded(draft.value.type, draft.value.channels)
  applyDeviceUnitToChannels(deviceUnit.value)
  enabledOnlyChannels.value = false
  channelKeyword.value = ''
  enableAtmospheric.value = draft.value.channels[16]?.enabled !== false
  initialDraftSnapshot.value = snapshotDraft(draft.value)
  saveError.value = null
  editorOpen.value = true
}

async function onTypeChanged(next: DeviceType) {
  if (next === draft.value.type) return
  draft.value.type = next
  draft.value.transport = next === 'WTN_PXI' ? 'tcp' : (draft.value.transport ?? 'tcp')
  draft.value.channels = createDefaultChannels(next)
  deviceUnit.value = normalizeDeviceUnit(next, deviceUnit.value)
  const range = getDeviceRangeFromChannels(draft.value.channels)
  deviceRangeMin.value = range?.min ?? null
  deviceRangeMax.value = range?.max ?? null
  devicePrecision.value = getDevicePrecisionFromChannels(draft.value.channels)
  applyDeviceUnitToChannels(deviceUnit.value)
  if (next === 'DAQ-P-1604') {
    enableAtmospheric.value = false
    if (draft.value.channels.length > 16) {
      draft.value.channels[16] = { ...draft.value.channels[16], enabled: false }
      draft.value.channels[17] = { ...draft.value.channels[17], enabled: false }
    }
  } else {
    enableAtmospheric.value = draft.value.channels[16]?.enabled !== false
  }
  if (next === 'DAQ-T-1603') {
    draft.value.daqT1603Config = { thermocoupleType: 'K', coldJunction: 'internal', filterHz: 50 }
    draft.value.address = '192.168.3.101'
    draft.value.port = 9000
  } else if (next === 'DAQ-P-1604' || next === 'WTN_PXI') {
    draft.value.address = '192.168.3.101'
    draft.value.port = 9000
  } else if (next === 'DAQ-P-1064Pre') {
    draft.value.address = '192.168.1.100'
    draft.value.port = 5000
  } else if (next === 'DSA3217') {
    draft.value.address = '192.168.1.254'
    draft.value.port = 5000
  }
}

function toggleAtmosphericData(on: boolean) {
  enableAtmospheric.value = on
  const count = draft.value.channels.length
  if (count > 16) draft.value.channels[16] = { ...draft.value.channels[16], enabled: on, unit: on ? 'Pa' : '' }
  if (count > 17) draft.value.channels[17] = { ...draft.value.channels[17], enabled: on, unit: on ? '℃' : '' }
}

function resetChannelsToDefault() {
  draft.value.channels = createDefaultChannels(draft.value.type)
  const range = getDeviceRangeFromChannels(draft.value.channels)
  deviceRangeMin.value = range?.min ?? null
  deviceRangeMax.value = range?.max ?? null
  devicePrecision.value = getDevicePrecisionFromChannels(draft.value.channels)
  applyDeviceUnitToChannels(deviceUnit.value)
}

function setAllChannels(enabled: boolean) {
  draft.value.channels = draft.value.channels.map((c) => ({ ...c, enabled }))
}

async function saveDraft() {
  saveError.value = null
  const err = getFirstDraftError(fieldErrors.value)
  if (err) {
    saveError.value = err
    editorTab.value = 'basic'
    return
  }
  saving.value = true
  try {
    if (draft.value.type === 'DAQ-P-1604' && draft.value.channels.length > 16) {
      draft.value.channels = draft.value.channels.map((ch, idx) => {
        if (idx === 16) return { ...ch, enabled: enableAtmospheric.value, unit: enableAtmospheric.value ? 'Pa' : ch.unit }
        if (idx === 17) return { ...ch, enabled: enableAtmospheric.value, unit: enableAtmospheric.value ? '℃' : ch.unit }
        return ch
      })
    }
    const normalized: DeviceProfile = { ...draft.value, name: draft.value.name.trim() }
    await deviceApi.upsertProfile(normalized)
    await deviceStore.refreshProfiles()
    if (normalized.autoConnect) {
      try { await deviceApi.connect(normalized.id) } catch { /* 连接失败不阻塞保存 */ }
    }
    feedback.pushToast(`设备 "${normalized.name}" 已保存`, 'success')
    initialDraftSnapshot.value = snapshotDraft(normalized)
    editorOpen.value = false
  } catch (err) {
    saveError.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

async function tryCloseEditor() {
  if (!isDirty.value) { editorOpen.value = false; return }
  const ok = await feedback.confirm('当前有未保存变更，确认关闭吗？')
  if (!ok) return
  editorOpen.value = false
}

function cancelEditor() {
  editorOpen.value = false
}

watch(deviceUnit, (u) => applyDeviceUnitToChannels(u), { immediate: false })
watch(devicePrecision, (p) => applyDevicePrecisionToChannels(p), { immediate: false })
watch([deviceRangeMin, deviceRangeMax], ([min, max]) => applyDeviceRangeToChannels(min, max), { immediate: false })

const selectedCount = computed(() => selectedIds.value.length)

function toggleSelected(id: string) {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}

function isSelected(id: string) {
  return selectedIds.value.includes(id)
}

function clearSelection() {
  selectedIds.value = []
}

watch(() => props.open, (v) => {
  if (v) {
    deviceStore.refreshProfiles()
    clearSelection()
  }
})

async function runScan() {
  scanning.value = true
  try {
    const results = await deviceApi.scanDevices()
    discovered.value = results
    if (results.length) feedback.pushToast(`发现 ${results.length} 个设备`, 'info')
    else feedback.pushToast('未发现新设备', 'info')
  } catch (err) {
    discovered.value = []
    feedback.pushToast(`扫描失败: ${err instanceof Error ? err.message : String(err)}`, 'error')
  } finally {
    scanning.value = false
  }
}

function clearDiscovered() {
  discovered.value = []
}

function addFromDiscovery(d: ScanResult) {
  openCreate((d.type as DeviceType) || 'SIMULATED')
  draft.value.name = d.name
  draft.value.address = d.address ?? draft.value.address
  draft.value.port = d.port ?? draft.value.port
  if (d.macAddress) draft.value.macAddress = d.macAddress
}

function matchedProfileForDiscovered(d: ScanResult): DeviceProfile | null {
  return deviceStore.profiles.find((p) => p.type === d.type && p.address === d.address && p.port === d.port) ?? null
}

function discoveryActionLabel(d: ScanResult): string {
  return matchedProfileForDiscovered(d) ? '编辑' : '添加'
}

function handleDiscoveredDeviceAction(d: ScanResult) {
  const matched = matchedProfileForDiscovered(d)
  if (matched) {
    openEdit(matched)
    draft.value.address = d.address ?? draft.value.address
    draft.value.port = d.port ?? draft.value.port
    if (d.macAddress) draft.value.macAddress = d.macAddress
    return
  }
  addFromDiscovery(d)
}

async function addAllDiscoveredDevices() {
  const addable = discovered.value.filter((d) => !matchedProfileForDiscovered(d))
  if (!addable.length) { feedback.pushToast('没有可添加的设备', 'info'); return }
  const existingNames = new Set(deviceStore.profiles.map((p) => p.name.trim()).filter((n) => n))
  let added = 0
  for (const d of addable) {
    const profile = createBlankProfile((d.type as DeviceType) || 'SIMULATED')
    let baseName = d.name.trim() || `${d.type.replace('DAQ-', '')}-${d.address}`
    let candidate = baseName
    let suffix = 2
    while (existingNames.has(candidate)) { candidate = `${baseName}-${suffix}`; suffix++ }
    existingNames.add(candidate)
    profile.name = candidate
    profile.address = d.address ?? profile.address
    profile.port = d.port ?? profile.port
    if (d.macAddress) profile.macAddress = d.macAddress
    try {
      await deviceApi.upsertProfile(profile)
      added++
    } catch { /* 跳过失败的 */ }
  }
  await deviceStore.refreshProfiles()
  feedback.pushToast(`已添加 ${added} 个设备`, 'success')
  clearDiscovered()
}

async function connectToggle(p: DeviceProfile) {
  const st = deviceStore.statusFor(p.id)
  if (st === 'Connecting') return
  if (deviceStore.acquiringFor(p.id) || st === 'Connected') {
    try { await deviceApi.disconnect(p.id); await deviceStore.refreshStatusFor(p.id) } catch (e) { feedback.pushToast(String(e), 'error') }
  } else {
    try { await deviceApi.connect(p.id); await deviceStore.refreshStatusFor(p.id) } catch (e) { feedback.pushToast(String(e), 'error') }
  }
}

async function toggleAcquisition(p: DeviceProfile) {
  const acquiring = deviceStore.acquiringFor(p.id)
  try {
    if (acquiring) await deviceApi.stopAcquisition(p.id)
    else await deviceApi.startAcquisition(p.id)
    await deviceStore.refreshStatusFor(p.id)
  } catch (e) { feedback.pushToast(String(e), 'error') }
}

async function removeProfile(p: DeviceProfile) {
  const ok = await feedback.confirm('确认删除此设备配置？')
  if (!ok) return
  try {
    await deviceApi.disconnect(p.id).catch(() => {})
    await deviceApi.stopAcquisition(p.id).catch(() => {})
    const empty: DeviceProfile = { id: p.id, name: '', type: 'SIMULATED', samplingRate: 0, channels: [] }
    await deviceApi.upsertProfile(empty)
    await deviceStore.refreshProfiles()
    feedback.pushToast('设备配置已删除', 'info')
  } catch (e) { feedback.pushToast(String(e), 'error') }
}

async function bulkConnect() {
  for (const id of selectedIds.value) {
    try { await deviceApi.connect(id) } catch { /* 跳过 */ }
  }
  clearSelection()
  feedback.pushToast('批量连接完成', 'info')
}

async function bulkDisconnect() {
  for (const id of selectedIds.value) {
    try { await deviceApi.disconnect(id) } catch { /* 跳过 */ }
  }
  clearSelection()
  feedback.pushToast('批量断开完成', 'info')
}

async function bulkDelete() {
  const ok = await feedback.confirm(`确认删除选中的 ${selectedIds.value.length} 个设备？`)
  if (!ok) return
  for (const id of selectedIds.value) {
    try {
      await deviceApi.disconnect(id).catch(() => {})
      await deviceApi.stopAcquisition(id).catch(() => {})
      const p = deviceStore.profiles.find((x) => x.id === id)
      if (p) {
        const empty: DeviceProfile = { id: p.id, name: '', type: 'SIMULATED', samplingRate: 0, channels: [] }
        await deviceApi.upsertProfile(empty)
      }
    } catch { /* 跳过 */ }
  }
  await deviceStore.refreshProfiles()
  clearSelection()
  feedback.pushToast('批量删除完成', 'info')
}

function close() {
  emit('update:open', false)
}

function statusClass(p: DeviceProfile) {
  if (deviceStore.acquiringFor(p.id)) return 'status-acq'
  const s = deviceStore.statusFor(p.id)
  if (s === 'Connected') return 'status-online'
  if (s === 'Connecting') return 'status-connecting'
  return 'status-offline'
}

function statusLabel(p: DeviceProfile) {
  if (deviceStore.acquiringFor(p.id)) return '采集中'
  const s = deviceStore.statusFor(p.id)
  if (s === 'Connected') return '已连接'
  if (s === 'Connecting') return '连接中'
  if (s === 'Error') return '错误'
  return '已断开'
}

function connectLabel(p: DeviceProfile) {
  const acquiring = deviceStore.acquiringFor(p.id)
  const st = deviceStore.statusFor(p.id)
  if (acquiring || st === 'Connected') return '断开'
  return '连接'
}

function channelLabel(c: ChannelConfig): string {
  return `${c.index + 1}`
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="drawer-mask" @click.self="close">
      <div class="drawer-shell">
        <header class="drawer-header">
          <div>
            <h2 class="drawer-title">设备管理</h2>
            <p class="drawer-subtitle">管理设备配置、扫描和连接</p>
          </div>
          <button class="drawer-close" @click="close">✕</button>
        </header>

        <div class="drawer-toolbar">
          <button class="btn btn-primary" @click="openCreate()">
            <span class="btn-icon">+</span> 新建设备
          </button>
          <button class="btn btn-second" :disabled="scanning" @click="runScan">
            <span class="btn-icon" :class="{ spin: scanning }">⟳</span>
            {{ scanning ? '扫描中...' : '扫描' }}
          </button>
          <div class="drawer-total">
            设备: {{ deviceStore.profiles.length }}
          </div>
        </div>

        <div v-if="discovered.length" class="drawer-discovered">
          <div class="drawer-discovered-head">
            <span class="drawer-discovered-label">发现的设备</span>
            <div class="drawer-discovered-actions">
              <button class="btn btn-xs btn-primary" @click="addAllDiscoveredDevices">全部添加</button>
              <span class="discovered-pulse" />
              <button class="btn btn-xs btn-ghost" @click="clearDiscovered">✕</button>
            </div>
          </div>
          <div class="drawer-discovered-list">
            <div v-for="d in discovered" :key="d.id" class="discovered-card">
              <div class="discovered-card-icon">{{ d.type === 'DAQ-T-1603' ? 'T' : d.type === 'DAQ-P-1604' ? 'P' : d.type === 'DAQ-P-1064Pre' ? 'S' : 'D' }}</div>
              <div class="discovered-card-info">
                <div class="discovered-card-name">{{ d.name }}</div>
                <div class="discovered-card-type">
                  {{ d.type }}
                  <span v-if="d.address" class="discovered-card-addr"> · {{ d.address }}<template v-if="d.port">:{{ d.port }}</template></span>
                  <span v-if="d.macAddress" class="discovered-card-addr"> · MAC: {{ d.macAddress }}</span>
                </div>
                <div v-if="matchedProfileForDiscovered(d)" class="discovered-matched">
                  已匹配: {{ matchedProfileForDiscovered(d)?.name }}
                </div>
              </div>
              <button class="btn btn-xs" :class="matchedProfileForDiscovered(d) ? 'btn-second' : 'btn-green'" @click="handleDiscoveredDeviceAction(d)">
                {{ discoveryActionLabel(d) }}
              </button>
            </div>
          </div>
        </div>

        <main class="drawer-list">
          <div v-if="!deviceStore.profiles.length" class="drawer-empty">
            暂无设备配置。点击"新建设备"创建。
          </div>

          <div v-for="p in deviceStore.profiles" :key="p.id" class="device-card" :class="[statusClass(p)]">
            <div class="device-card-stripe" :class="[statusClass(p)]" />

            <div class="device-card-body">
              <div class="device-card-left">
                <div class="device-card-row">
                  <input type="checkbox" class="device-checkbox"
                    :checked="isSelected(p.id)"
                    @change="toggleSelected(p.id)" />
                  <h3 class="device-card-name">{{ p.name }}</h3>
                  <span class="device-card-type-badge">{{ p.type }}</span>
                </div>
                <div class="device-card-meta">
                  <span>🔌 {{ p.transport === 'serial' ? (p.serialPort || 'COM?') : `${p.address || '-'}:${p.port || '-'}` }}</span>
                  <span>⚡ {{ p.samplingRate ?? 20 }}Hz</span>
                  <span>📂 {{ p.channels?.length ?? 0 }} 通道</span>
                </div>
              </div>

              <div class="device-card-right">
                <button class="btn btn-sm btn-second" @click="openEdit(p)">编辑</button>
                <button class="btn btn-sm" :class="connectLabel(p) === '断开' ? 'btn-danger' : 'btn-green'" @click="connectToggle(p)">
                  {{ connectLabel(p) }}
                </button>
                <button v-if="deviceStore.statusFor(p.id) === 'Connected'" class="btn btn-sm" :class="deviceStore.acquiringFor(p.id) ? 'btn-warn' : 'btn-green'" @click="toggleAcquisition(p)">
                  {{ deviceStore.acquiringFor(p.id) ? '停止' : '采集' }}
                </button>
                <button class="btn btn-sm btn-danger ghost" @click="removeProfile(p)">删除</button>
              </div>
            </div>

            <div v-if="deviceStore.statusFor(p.id) === 'Error'" class="device-card-error">
              ⚠ 设备通信错误
            </div>
          </div>
        </main>

        <div v-if="selectedIds.length" class="drawer-bulk">
          <span>已选 <strong>{{ selectedCount }}</strong></span>
          <button class="btn btn-xs btn-green" :disabled="!selectedCount" @click="bulkConnect">批量连接</button>
          <button class="btn btn-xs btn-second" :disabled="!selectedCount" @click="bulkDisconnect">批量断开</button>
          <button class="btn btn-xs btn-danger" :disabled="!selectedCount" @click="bulkDelete">批量删除</button>
          <button class="btn btn-xs btn-ghost" @click="clearSelection" style="margin-left:auto">清除</button>
        </div>
      </div>

      <!-- 编辑器模态框 -->
      <div v-if="editorOpen" class="editor-mask" @click.self="tryCloseEditor">
        <div class="editor-modal">
          <header class="editor-header">
            <div class="editor-header-left">
              <div class="editor-header-icon">
                <span>{{ isReadOnly ? '🔒' : editorMode === 'create' ? '+' : '✎' }}</span>
              </div>
              <div>
                <h3 class="editor-title">{{ editorMode === 'create' ? '新建设备' : isReadOnly ? '查看设备（只读）' : '编辑设备' }}</h3>
                <div class="editor-status-row">
                  <span class="editor-status-dot" :class="statusClass({ id: draft.id, name: '', type: 'SIMULATED', samplingRate: 20, channels: [] })" />
                  <span class="editor-status-text">{{ statusLabel({ id: draft.id, name: '', type: 'SIMULATED', samplingRate: 20, channels: [] }) }}</span>
                </div>
              </div>
            </div>
            <button class="drawer-close" @click="tryCloseEditor">✕</button>
          </header>

          <!-- 标签页切换 -->
          <div class="editor-tabs">
            <div class="editor-tabs-inner">
              <button class="editor-tab" :class="{ active: editorTab === 'basic' }" @click="editorTab = 'basic'">
                基本信息
              </button>
              <button class="editor-tab" :class="{ active: editorTab === 'channels' }" @click="editorTab = 'channels'">
                通道配置
              </button>
            </div>
          </div>

          <div class="editor-body">
            <div v-if="saveError" class="editor-error-banner">
              <span>⚠️</span> 保存失败: {{ saveError }}
            </div>

            <div v-if="isReadOnly" class="editor-readonly-banner">
              <span>🔒</span> 设备正在采集中，无法修改配置
            </div>

            <!-- 基本信息 -->
            <div v-if="editorTab === 'basic'" class="editor-sections">
              <section class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">设备识别 · Identity</h4>
                  <p class="editor-section-desc">基础型号与命名空间</p>
                </div>
                <div class="editor-grid">
                  <div class="editor-field col-6">
                    <label class="editor-label">设备名称 *</label>
                    <input v-model="draft.name" type="text" class="editor-input" :disabled="isReadOnly" placeholder="输入设备名称" />
                    <div v-if="fieldErrors.name" class="editor-field-error">● {{ fieldErrors.name }}</div>
                  </div>
                  <div class="editor-field col-4">
                    <label class="editor-label">设备型号</label>
                    <UiSelect
                      :model-value="draft.type"
                      :options="deviceTypeOptions"
                      :disabled="editorMode === 'edit' || isReadOnly"
                      @update:model-value="onTypeChanged($event as DeviceType)"
                    />
                  </div>
                  <div v-if="draft.type !== 'DSA3217'" class="editor-field col-2">
                    <label class="editor-label">采样率 (Hz)</label>
                    <input v-model.number="draft.samplingRate" type="number" class="editor-input" :disabled="isReadOnly" />
                    <div v-if="fieldErrors.samplingRate" class="editor-field-error">● {{ fieldErrors.samplingRate }}</div>
                  </div>
                  <div class="editor-field col-12">
                    <label class="editor-label">设备单位 (全局)</label>
                    <div class="editor-unit-row">
                      <div class="editor-unit-select">
                        <UiSelect
                          v-if="!isWtnPxiType(draft.type) && !isTempUnitFixed(draft.type)"
                          :model-value="deviceUnit"
                          :options="pressureUnitOptions"
                          :disabled="isReadOnly"
                          @update:model-value="deviceUnit = String($event)"
                        />
                        <div v-else class="editor-input editor-input-readonly">
                          {{ isWtnPxiType(draft.type) ? 'WTN_PXI 固定配置' : 'DAQ-T-1603 固定单位: ℃' }}
                        </div>
                      </div>
                      <p class="editor-unit-hint">设置 CH1~CH16 的全局工程单位</p>
                    </div>
                  </div>
                </div>
              </section>

              <!-- DAQ-P-1604 大气数据开关 -->
              <section v-if="draft.type === 'DAQ-P-1604'" class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">大气数据</h4>
                  <p class="editor-section-desc">控制是否采集大气压 (CH17) 与大气温度 (CH18)</p>
                </div>
                <div class="editor-atmo-row">
                  <div class="editor-atmo-toggle" @click="!isReadOnly && toggleAtmosphericData(!enableAtmospheric)">
                    <div class="editor-toggle-track" :class="{ on: enableAtmospheric }">
                      <span class="editor-toggle-thumb" :class="{ on: enableAtmospheric }" />
                    </div>
                    <span class="editor-atmo-label">{{ enableAtmospheric ? '包含大气压与大气温度数据' : '仅采集 16 路压力数据' }}</span>
                  </div>
                </div>
              </section>

              <!-- 通信协议 -->
              <section class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">通信协议 · Transport</h4>
                  <p class="editor-section-desc">TCP/IP 网络或 RS232 串口链路</p>
                </div>
                <div class="editor-grid">
                  <div v-if="isTcpType(draft.type) && supportsTransportSwitch(draft.type)" class="editor-field col-4">
                    <label class="editor-label">传输方式</label>
                    <UiSelect
                      :model-value="draft.transport ?? 'tcp'"
                      :options="transportOptions"
                      :disabled="isReadOnly"
                      @update:model-value="draft.transport = $event as 'tcp' | 'serial'"
                    />
                  </div>

                  <template v-if="isTcpType(draft.type) && draft.transport === 'tcp'">
                    <div class="editor-field col-5">
                      <label class="editor-label">IP 地址 *</label>
                      <input v-model="draft.address" class="editor-input" :disabled="isReadOnly" placeholder="192.168.1.100" />
                      <div v-if="fieldErrors.address" class="editor-field-error">● {{ fieldErrors.address }}</div>
                    </div>
                    <div class="editor-field col-3">
                      <label class="editor-label">端口 *</label>
                      <input v-model.number="draft.port" type="number" class="editor-input" :disabled="isReadOnly" />
                      <div v-if="fieldErrors.port" class="editor-field-error">● {{ fieldErrors.port }}</div>
                    </div>
                  </template>

                  <template v-if="isTcpType(draft.type) && draft.transport === 'serial'">
                    <div class="editor-field col-7">
                      <label class="editor-label">串口号 *</label>
                      <input v-model="draft.serialPort" class="editor-input" :disabled="isReadOnly" placeholder="COM1" />
                      <div v-if="fieldErrors.serialPort" class="editor-field-error">● {{ fieldErrors.serialPort }}</div>
                    </div>
                    <div class="editor-field col-5">
                      <label class="editor-label">波特率 *</label>
                      <input v-model.number="draft.baudRate" type="number" class="editor-input" :disabled="isReadOnly" />
                      <div v-if="fieldErrors.baudRate" class="editor-field-error">● {{ fieldErrors.baudRate }}</div>
                    </div>
                  </template>

                  <div class="editor-field col-12">
                    <div class="editor-autoconnect-row">
                      <label class="editor-autoconnect-label">
                        <input v-model="draft.autoConnect" type="checkbox" :disabled="isReadOnly" class="editor-autoconnect-check" />
                        保存后自动连接
                      </label>
                    </div>
                  </div>
                </div>
              </section>
            </div>

            <!-- 通道配置 -->
            <div v-else class="editor-sections">
              <!-- DAQ-T-1603 专用配置 -->
              <div v-if="draft.type === 'DAQ-T-1603'" class="editor-channels-special">
                <DaqT1603Config
                  v-model:thermocouple-type="draft.daqT1603Config!.thermocoupleType"
                  v-model:cold-junction="draft.daqT1603Config!.coldJunction"
                  v-model:filter-hz="draft.daqT1603Config!.filterHz"
                />
                <div class="editor-channels-table-wrap">
                  <table class="editor-channels-table">
                    <thead>
                      <tr>
                        <th class="w-14">#</th>
                        <th>通道名称</th>
                        <th class="w-20 text-right">单位</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="c in draft.channels" :key="c.index">
                        <td class="font-mono">{{ channelLabel(c).padStart(2, '0') }}</td>
                        <td>
                          <input v-model="c.name" :disabled="isReadOnly" class="editor-ch-input" />
                        </td>
                        <td class="text-right text-muted">℃</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <!-- WTN_PXI 固定通道 -->
              <div v-else-if="draft.type === 'WTN_PXI'" class="editor-channels-special">
                <p class="editor-channels-hint">WTN_PXI 通道定义固定，不支持编辑。</p>
                <div class="editor-channels-table-wrap">
                  <table class="editor-channels-table">
                    <thead>
                      <tr>
                        <th class="w-14">#</th>
                        <th>通道名称</th>
                        <th class="w-20 text-right">单位</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="c in draft.channels" :key="c.index">
                        <td class="font-mono">{{ channelLabel(c).padStart(2, '0') }}</td>
                        <td>{{ c.name }}</td>
                        <td class="text-right text-muted">{{ c.unit }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>

              <!-- 其他设备通道配置 -->
              <div v-else class="editor-channels-full">
                <div class="editor-channels-toolbar">
                  <div class="editor-channels-toolbar-left">
                    <button class="btn btn-xs btn-second" :disabled="isReadOnly" @click="setAllChannels(true)">全部启用</button>
                    <button class="btn btn-xs btn-second" :disabled="isReadOnly" @click="setAllChannels(false)">全部禁用</button>
                    <button class="btn btn-xs btn-second" :disabled="isReadOnly" @click="resetChannelsToDefault">重置</button>
                  </div>
                  <div class="editor-channels-toolbar-right">
                    <input v-model="channelKeyword" type="text" class="editor-ch-search" placeholder="过滤通道..." />
                    <label class="editor-ch-filter-label">
                      <input v-model="enabledOnlyChannels" type="checkbox" class="editor-ch-filter-check" />
                      仅看启用
                    </label>
                  </div>
                </div>

                <!-- 批量同步 -->
                <div class="editor-ch-batch">
                  <div class="editor-ch-batch-item">
                    <div class="editor-label">批量量程 (1~16CH)</div>
                    <div class="editor-ch-batch-range">
                      <input v-model.number="deviceRangeMin" type="number" :disabled="isReadOnly" class="editor-ch-batch-input" />
                      <span>~</span>
                      <input v-model.number="deviceRangeMax" type="number" :disabled="isReadOnly" class="editor-ch-batch-input" />
                    </div>
                  </div>
                  <div class="editor-ch-batch-divider" />
                  <div class="editor-ch-batch-item">
                    <div class="editor-label">批量精度 (1~16CH)</div>
                    <div class="editor-ch-batch-precision">
                      <input v-model.number="devicePrecision" type="number" min="0" :disabled="isReadOnly" class="editor-ch-batch-input-sm" />
                      <span class="editor-ch-batch-hint">全局小数位</span>
                    </div>
                  </div>
                </div>

                <!-- 通道列表 -->
                <div class="editor-channels-table-wrap">
                  <table class="editor-channels-table">
                    <thead>
                      <tr>
                        <th class="w-14">启用</th>
                        <th class="w-14">#</th>
                        <th>通道名称</th>
                        <th class="w-36 text-center">工程量程</th>
                        <th class="w-20 text-right">精度</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="row in channelRows" :key="row.channel.index">
                        <td class="text-center">
                          <input v-model="row.channel.enabled" type="checkbox" :disabled="isReadOnly" class="editor-ch-check" />
                        </td>
                        <td class="font-mono">{{ channelLabel(row.channel).padStart(2, '0') }}</td>
                        <td>
                          <input v-model="row.channel.name" :disabled="isReadOnly" class="editor-ch-input" />
                        </td>
                        <td>
                          <div class="editor-ch-range">
                            <input v-model.number="row.channel.rangeMin" type="number" :disabled="isReadOnly || row.originalIndex >= 16" class="editor-ch-range-input" />
                            <span>~</span>
                            <input v-model.number="row.channel.rangeMax" type="number" :disabled="isReadOnly || row.originalIndex >= 16" class="editor-ch-range-input" />
                          </div>
                        </td>
                        <td>
                          <input v-model.number="row.channel.precision" type="number" min="0" :disabled="isReadOnly || row.originalIndex >= 16" class="editor-ch-precision-input" />
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </div>

          <footer class="editor-footer">
            <div class="editor-footer-left">
              <div v-if="isReadOnly" class="editor-footer-readonly">🔒 只读模式</div>
              <div v-else-if="validationErrorCount > 0" class="editor-footer-errors">
                ● 校验失败: {{ validationErrorCount }} 项错误
              </div>
              <div v-else class="editor-footer-status" :class="{ dirty: isDirty }">
                {{ isDirty ? '● 检测到未保存的变更' : '✓ 配置已同步' }}
              </div>
            </div>
            <div class="editor-footer-right">
              <button class="btn btn-second" @click="tryCloseEditor">{{ isReadOnly ? '关闭' : '取消' }}</button>
              <button v-if="!isReadOnly" class="btn btn-primary" :disabled="saving || validationErrorCount > 0" @click="saveDraft">
                {{ saving ? '保存中...' : '保存' }}
              </button>
            </div>
          </footer>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.drawer-mask {
  position: fixed; inset: 0; z-index: 100;
  background: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(4px);
  display: flex; justify-content: flex-end;
}

.drawer-shell {
  width: 560px; max-width: 95vw; height: 100vh;
  display: flex; flex-direction: column;
  background: var(--bg-panel);
  border-left: 1px solid var(--border-default);
  box-shadow: 0 25px 50px rgba(0, 0, 0, 0.3);
}

.drawer-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  flex-shrink: 0;
}

.drawer-title { margin: 0; font-size: 1rem; font-weight: 800; color: var(--text-primary); letter-spacing: -0.02em; }
.drawer-subtitle { margin: 0.25rem 0 0; font-size: 0.75rem; color: var(--text-muted); font-weight: 600; }

.drawer-close {
  width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
  border-radius: 0.5rem; color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05); border: 1px solid rgba(255, 255, 255, 0.08);
  font-size: 0.875rem; transition: all 0.2s;
}
.drawer-close:hover { color: var(--accent-danger); border-color: var(--accent-danger); }

.drawer-toolbar {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 1rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

.drawer-total {
  margin-left: auto;
  padding: 0.375rem 0.75rem; border-radius: 999px;
  background: rgba(100, 116, 139, 0.1);
  font-size: 0.625rem; font-weight: 800; letter-spacing: 0.1em;
  color: var(--text-muted); text-transform: uppercase;
}

.btn {
  display: inline-flex; align-items: center; gap: 0.375rem;
  padding: 0.5rem 1rem; border-radius: 0.5rem;
  font-size: 0.75rem; font-weight: 700;
  transition: all 0.2s; cursor: pointer; border: 1px solid transparent;
}
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: #10b981; color: white; box-shadow: 0 4px 12px rgba(16,185,129,0.3); }
.btn-primary:hover { background: #059669; }
.btn-second { background: rgba(59,130,246,0.1); color: #3b82f6; border-color: rgba(59,130,246,0.2); }
.btn-second:hover { background: rgba(59,130,246,0.2); color: #2563eb; border-color: rgba(59,130,246,0.4); }
.btn-green { background: #10b981; color: white; }
.btn-green:hover { background: #059669; }
.btn-danger { background: rgba(244,63,94,0.1); color: #f43f5e; border-color: rgba(244,63,94,0.2); }
.btn-danger:hover { background: rgba(244,63,94,0.2); }
.btn-warn { background: rgba(245,158,11,0.1); color: #f59e0b; border-color: rgba(245,158,11,0.2); }
.btn-sm { padding: 0.375rem 0.75rem; font-size: 0.7rem; white-space: nowrap; }
.btn-xs { padding: 0.25rem 0.5rem; font-size: 0.625rem; }
.btn-icon { font-size: 1rem; line-height: 1; }
.btn-ghost { background: transparent; color: var(--text-muted); border: none; }
.spin { display: inline-block; animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

.drawer-discovered {
  border-bottom: 1px solid var(--border-default);
  padding: 1rem 1.5rem;
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
  flex-shrink: 0;
}
.drawer-discovered-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 0.75rem; }
.drawer-discovered-label { font-size: 0.625rem; font-weight: 800; letter-spacing: 0.15em; text-transform: uppercase; color: var(--text-muted); }
.drawer-discovered-actions { display: flex; align-items: center; gap: 0.5rem; }
.discovered-pulse { width: 6px; height: 6px; border-radius: 50%; background: #3b82f6; animation: pulse 1.5s infinite; }
.drawer-discovered-list { display: flex; flex-direction: column; gap: 0.5rem; max-height: 30vh; overflow-y: auto; }
.discovered-card {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem; border-radius: 0.75rem;
  background: var(--bg-panel); border: 1px solid var(--border-default);
}
.discovered-card-icon {
  width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
  border-radius: 50%; background: rgba(59,130,246,0.1); color: #3b82f6;
  font-size: 0.75rem; font-weight: 800; flex-shrink: 0;
}
.discovered-card-name { font-size: 0.8rem; font-weight: 700; color: var(--text-primary); }
.discovered-card-type { font-size: 0.65rem; font-weight: 600; color: var(--text-muted); margin-top: 0.125rem; }
.discovered-card-addr { color: var(--text-muted); opacity: 0.7; }
.discovered-matched {
  margin-top: 0.25rem; display: inline-flex; align-items: center;
  padding: 0.125rem 0.5rem; border-radius: 999px;
  background: rgba(16,185,129,0.1); border: 1px solid rgba(16,185,129,0.3);
  font-size: 0.6rem; font-weight: 700; color: #10b981;
}
.discovered-card .btn-green, .discovered-card .btn-second { margin-left: auto; flex-shrink: 0; }

.drawer-list { flex: 1; overflow-y: auto; padding: 1rem 1.5rem; display: flex; flex-direction: column; gap: 0.75rem; }
.drawer-empty { padding: 2rem 1rem; text-align: center; color: var(--text-muted); font-size: 0.8rem; }

.device-card {
  position: relative; overflow: hidden;
  border-radius: 0.75rem; border: 1px solid var(--border-default);
  background: var(--bg-panel); transition: all 0.2s;
}
.device-card:hover { border-color: var(--accent-success); }
.device-card-stripe { position: absolute; left: 0; top: 0; bottom: 0; width: 4px; background: #64748b; transition: all 0.3s; }
.device-card-stripe.status-online { background: #10b981; box-shadow: 0 0 8px rgba(16,185,129,0.5); }
.device-card-stripe.status-acq { background: #10b981; box-shadow: 0 0 12px rgba(16,185,129,0.6); animation: pulse 1.5s infinite; }
.device-card-stripe.status-connecting { background: #f59e0b; animation: pulse 0.8s infinite; }
.device-card-body { padding: 1rem; display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.device-card-left { min-width: 0; flex: 1; }
.device-card-row { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.5rem; }
.device-checkbox { width: 14px; height: 14px; border-radius: 3px; flex-shrink: 0; accent-color: #3b82f6; }
.device-card-name { margin: 0; font-size: 0.9rem; font-weight: 700; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.device-card-type-badge {
  flex-shrink: 0; padding: 0.125rem 0.5rem; border-radius: 0.25rem;
  background: var(--bg-panel-strong); font-size: 0.6rem; font-weight: 800;
  letter-spacing: 0.05em; color: var(--text-muted); text-transform: uppercase;
}
.device-card-meta { display: flex; flex-wrap: wrap; gap: 1rem; font-size: 0.65rem; font-weight: 600; color: var(--text-muted); }
.device-card-meta span { display: inline-flex; align-items: center; gap: 0.25rem; }
.device-card-right { display: flex; flex-direction: column; gap: 0.375rem; flex-shrink: 0; }
.device-card-error {
  margin: 0 1rem 0.75rem; padding: 0.5rem 0.75rem; border-radius: 0.375rem;
  background: rgba(244,63,94,0.1); border: 1px solid rgba(244,63,94,0.2);
  font-size: 0.65rem; font-weight: 600; color: #f43f5e;
}
.device-card-right .btn.ghost { background: transparent; color: var(--text-muted); border: 1px dashed var(--border-default); }
.device-card-right .btn.ghost:hover { color: #f43f5e; border-color: #f43f5e; }

.drawer-bulk {
  flex-shrink: 0; display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem 1.5rem; border-top: 1px solid var(--border-default);
  background: var(--bg-panel-strong); font-size: 0.75rem; color: var(--text-secondary);
}

@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }

/* Editor Modal */
.editor-mask {
  position: fixed; inset: 0; z-index: 110;
  background: rgba(0, 0, 0, 0.7);
  display: flex; align-items: center; justify-content: center;
  padding: 1rem;
}

.editor-modal {
  width: 860px; max-width: 98vw; max-height: 92vh;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 1rem;
  box-shadow: 0 32px 64px -12px rgba(0, 0, 0, 0.5);
  display: flex; flex-direction: column;
  overflow: hidden;
}

.editor-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  flex-shrink: 0;
}
.editor-header-left { display: flex; align-items: center; gap: 0.75rem; }
.editor-header-icon {
  width: 40px; height: 40px; display: flex; align-items: center; justify-content: center;
  border-radius: 0.75rem; background: rgba(59,130,246,0.1); color: #3b82f6;
  font-size: 1.25rem; font-weight: 800;
}
.editor-title { margin: 0; font-size: 1rem; font-weight: 800; color: var(--text-primary); }
.editor-status-row { display: flex; align-items: center; gap: 0.5rem; margin-top: 0.25rem; }
.editor-status-dot {
  width: 6px; height: 6px; border-radius: 50%; background: var(--text-muted);
}
.editor-status-dot.status-online { background: #10b981; box-shadow: 0 0 4px rgba(16,185,129,0.5); }
.editor-status-dot.status-acq { background: #10b981; box-shadow: 0 0 6px rgba(16,185,129,0.6); animation: pulse 1.5s infinite; }
.editor-status-dot.status-connecting { background: #f59e0b; animation: pulse 0.8s infinite; }
.editor-status-text { font-size: 0.75rem; font-weight: 600; color: var(--text-muted); }

.editor-tabs {
  flex-shrink: 0; padding: 1rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
}
.editor-tabs-inner {
  display: inline-flex; border-radius: 0.75rem;
  background: var(--bg-app, rgba(0,0,0,0.2)); padding: 0.25rem;
}
.editor-tab {
  padding: 0.5rem 1.5rem; border-radius: 0.5rem;
  font-size: 0.75rem; font-weight: 800;
  color: var(--text-muted); transition: all 0.2s; cursor: pointer;
  border: none; background: transparent;
}
.editor-tab:hover { color: var(--text-primary); }
.editor-tab.active { background: var(--bg-panel); color: #3b82f6; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }

.editor-body { padding: 1.5rem; overflow-y: auto; flex: 1; min-height: 0; }

.editor-error-banner {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 1rem; border-radius: 0.75rem;
  background: rgba(244,63,94,0.1); border: 1px solid rgba(244,63,94,0.3);
  font-size: 0.75rem; font-weight: 700; color: #f43f5e;
  margin-bottom: 1.5rem;
}
.editor-readonly-banner {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 1rem; border-radius: 0.75rem;
  background: rgba(245,158,11,0.1); border: 1px solid rgba(245,158,11,0.3);
  font-size: 0.75rem; font-weight: 700; color: #f59e0b;
  margin-bottom: 1.5rem;
}

.editor-sections { display: flex; flex-direction: column; gap: 2rem; }
.editor-section { }
.editor-section-head { margin-bottom: 1rem; }
.editor-section-title { font-size: 0.75rem; font-weight: 800; letter-spacing: 0.1em; color: var(--text-primary); text-transform: uppercase; margin: 0; }
.editor-section-desc { font-size: 0.65rem; font-weight: 700; color: var(--text-muted); margin-top: 0.25rem; }

.editor-grid { display: grid; grid-template-columns: repeat(12, 1fr); gap: 1rem; }
.editor-field { margin-bottom: 0; }
.col-2 { grid-column: span 2; }
.col-3 { grid-column: span 3; }
.col-4 { grid-column: span 4; }
.col-5 { grid-column: span 5; }
.col-6 { grid-column: span 6; }
.col-7 { grid-column: span 7; }
.col-12 { grid-column: span 12; }

.editor-label { display: block; margin-bottom: 0.375rem; font-size: 0.65rem; font-weight: 800; color: var(--text-muted); letter-spacing: 0.08em; text-transform: uppercase; }
.editor-input {
  width: 100%; padding: 0.625rem 0.75rem;
  border-radius: 0.5rem; border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2); color: var(--text-primary);
  font: inherit; font-size: 0.85rem; font-weight: 700;
  outline: none; transition: all 0.2s;
}
.editor-input:focus { border-color: #3b82f6; background: var(--bg-panel-strong); }
.editor-input:disabled { opacity: 0.6; cursor: not-allowed; }
.editor-input-readonly {
  display: flex; align-items: center;
  height: 40px; font-weight: 700; color: var(--text-muted);
}
:root[data-theme='light'] .editor-input { background: rgba(255, 255, 255, 0.8); }
.editor-field-error { margin-top: 0.375rem; font-size: 0.65rem; font-weight: 700; color: #f43f5e; }

.editor-unit-row { display: flex; align-items: center; gap: 0.75rem; }
.editor-unit-select { flex: 1; }
.editor-unit-hint { font-size: 0.65rem; font-weight: 700; color: var(--text-muted); max-width: 200px; line-height: 1.4; }

.editor-atmo-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0.75rem 1rem; border-radius: 0.5rem;
  border: 1px solid var(--border-default); background: var(--bg-panel);
}
.editor-atmo-toggle { display: flex; align-items: center; gap: 0.75rem; cursor: pointer; }
.editor-toggle-track {
  width: 44px; height: 24px; border-radius: 999px;
  background: var(--border-default); position: relative; transition: all 0.2s;
}
.editor-toggle-track.on { background: #3b82f6; }
.editor-toggle-thumb {
  position: absolute; top: 2px; left: 3px;
  width: 20px; height: 20px; border-radius: 50%;
  background: white; transition: all 0.2s;
}
.editor-toggle-thumb.on { transform: translateX(20px); }
.editor-atmo-label { font-size: 0.75rem; font-weight: 700; color: var(--text-primary); }

.editor-autoconnect-row {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem 1rem; border-radius: 0.5rem;
  background: rgba(59,130,246,0.05);
}
.editor-autoconnect-label {
  display: flex; align-items: center; gap: 0.5rem;
  font-size: 0.75rem; font-weight: 800; color: #3b82f6; cursor: pointer;
}
.editor-autoconnect-check { width: 16px; height: 16px; accent-color: #3b82f6; }

/* Channels */
.editor-channels-special { display: flex; flex-direction: column; gap: 1.5rem; }
.editor-channels-hint { font-size: 0.7rem; color: var(--text-muted); margin: 0; }
.editor-channels-full { display: flex; flex-direction: column; gap: 1rem; }

.editor-channels-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
.editor-channels-toolbar-left { display: flex; gap: 0.5rem; }
.editor-channels-toolbar-right { display: flex; align-items: center; gap: 0.75rem; }
.editor-ch-search {
  padding: 0.375rem 0.75rem; border-radius: 999px;
  border: 1px solid var(--border-default); background: var(--bg-panel);
  font-size: 0.7rem; font-weight: 700; color: var(--text-primary);
  outline: none; width: 160px;
}
.editor-ch-search:focus { border-color: #3b82f6; }
.editor-ch-filter-label {
  display: flex; align-items: center; gap: 0.375rem;
  font-size: 0.7rem; font-weight: 800; color: var(--text-muted); cursor: pointer;
}
.editor-ch-filter-check { width: 14px; height: 14px; accent-color: #3b82f6; }

.editor-ch-batch {
  display: flex; align-items: flex-end; gap: 1.5rem;
  padding: 0.75rem 0;
}
.editor-ch-batch-item { flex: 1; }
.editor-ch-batch-divider { width: 1px; height: 40px; background: var(--border-default); opacity: 0.6; }
.editor-ch-batch-range { display: flex; align-items: center; gap: 0.5rem; }
.editor-ch-batch-input {
  width: 100%; padding: 0.375rem 0.5rem; border-radius: 0.5rem;
  border: 1px solid var(--border-default); background: var(--bg-panel);
  font-size: 0.75rem; font-weight: 700; color: var(--text-primary);
  text-align: center; outline: none;
}
.editor-ch-batch-input:focus { border-color: #3b82f6; }
.editor-ch-batch-input:disabled { opacity: 0.6; cursor: not-allowed; }
.editor-ch-batch-input-sm {
  width: 96px; padding: 0.375rem 0.5rem; border-radius: 0.5rem;
  border: 1px solid var(--border-default); background: var(--bg-panel);
  font-size: 0.75rem; font-weight: 700; color: var(--text-primary);
  text-align: center; outline: none;
}
.editor-ch-batch-input-sm:focus { border-color: #3b82f6; }
.editor-ch-batch-precision { display: flex; align-items: center; gap: 0.75rem; }
.editor-ch-batch-hint { font-size: 0.65rem; font-weight: 700; color: var(--text-muted); }

.editor-channels-table-wrap {
  border-radius: 0.5rem; border: 1px solid var(--border-default);
  overflow: hidden; background: var(--bg-panel);
}
.editor-channels-table { width: 100%; border-collapse: collapse; text-align: left; }
.editor-channels-table thead tr { background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent); }
.editor-channels-table th {
  padding: 0.75rem 0.5rem; font-size: 0.625rem; font-weight: 800;
  letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
}
.editor-channels-table td {
  padding: 0.5rem; border-top: 1px solid color-mix(in srgb, var(--border-default) 60%, transparent);
  font-size: 0.75rem; color: var(--text-primary);
}
.editor-channels-table tr:hover td { background: color-mix(in srgb, var(--bg-panel-strong) 30%, transparent); }
.editor-ch-check { width: 16px; height: 16px; accent-color: #3b82f6; }
.editor-ch-input {
  width: 100%; padding: 0.25rem 0.5rem; border-radius: 0.375rem;
  border: 1px solid transparent; background: transparent;
  font-size: 0.75rem; font-weight: 700; color: var(--text-primary);
  outline: none; transition: all 0.2s;
}
.editor-ch-input:hover { background: var(--bg-panel-strong); }
.editor-ch-input:focus { background: var(--bg-panel); border-color: #3b82f6; }
.editor-ch-input:disabled { opacity: 0.6; cursor: not-allowed; }
.editor-ch-range { display: flex; align-items: center; justify-content: center; gap: 0.25rem; }
.editor-ch-range-input {
  width: 80px; padding: 0.25rem 0.375rem; border-radius: 0.375rem;
  border: 1px solid transparent; background: color-mix(in srgb, var(--bg-panel-strong) 40%, transparent);
  font-size: 0.7rem; font-weight: 700; color: var(--text-primary);
  text-align: right; outline: none;
}
.editor-ch-range-input:focus { border-color: #3b82f6; }
.editor-ch-range-input:disabled { opacity: 0.6; cursor: not-allowed; }
.editor-ch-precision-input {
  width: 100%; padding: 0.25rem 0.375rem; border-radius: 0.375rem;
  border: 1px solid transparent; background: color-mix(in srgb, var(--bg-panel-strong) 40%, transparent);
  font-size: 0.7rem; font-weight: 700; color: var(--text-primary);
  text-align: right; outline: none;
}
.editor-ch-precision-input:focus { border-color: #3b82f6; }
.editor-ch-precision-input:disabled { opacity: 0.6; cursor: not-allowed; }

.font-mono { font-family: ui-monospace, monospace; }
.text-center { text-align: center; }
.text-right { text-align: right; }
.text-muted { color: var(--text-muted); }

.editor-footer {
  display: flex; align-items: center; justify-content: space-between;
  padding: 1rem 1.5rem;
  border-top: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  flex-shrink: 0;
}
.editor-footer-left { display: flex; flex-direction: column; }
.editor-footer-right { display: flex; gap: 0.75rem; }
.editor-footer-readonly { font-size: 0.65rem; font-weight: 800; color: #f59e0b; text-transform: uppercase; letter-spacing: 0.05em; }
.editor-footer-errors { font-size: 0.65rem; font-weight: 800; color: #f43f5e; text-transform: uppercase; letter-spacing: 0.05em; }
.editor-footer-status { font-size: 0.65rem; font-weight: 800; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.1em; }
.editor-footer-status.dirty { color: #f59e0b; }
</style>
