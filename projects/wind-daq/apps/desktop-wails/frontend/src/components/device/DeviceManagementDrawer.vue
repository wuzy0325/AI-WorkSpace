<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { deviceApi } from '@api/deviceApi'
import type { DeviceProfile, DeviceType, ScanResult, ChannelConfig } from '@api/types'
import UiSelect from '@components/ui/UiSelect.vue'
import DaqT1603Config from '@components/device/DaqT1603Config.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiButton from '@components/ui/UiButton.vue'
import { Plug, Zap, LayoutGrid } from '@lucide/vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()

const scanning = ref(false)
const discovered = ref<ScanResult[]>([])
const showDiscovered = ref(true)
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
// DSA3217 扫描配置
const dsa3217Avg = ref<number>(32)
const dsa3217Period = ref<number>(500)
const dsa3217Fps = computed<string>(() => {
  const avg = dsa3217Avg.value
  const period = dsa3217Period.value
  if (avg >= 1 && period >= 73) {
    return (1 / (period * 1e-6 * 16 * avg)).toFixed(3)
  }
  return '--'
})

// DSA3217 AVG 输入校验：自动修正 NaN/越界值，防止兄弟控件清空
watch(dsa3217Avg, (val) => {
  if (typeof val !== 'number' || isNaN(val) || val < 1) {
    dsa3217Avg.value = 32
  } else if (val > 240) {
    dsa3217Avg.value = 240
  } else if (val !== Math.round(val)) {
    dsa3217Avg.value = Math.round(val)
  }
}, { flush: 'sync' })

// DSA3217 PERIOD 输入校验：自动修正 NaN/越界值，防止兄弟控件清空
watch(dsa3217Period, (val) => {
  if (typeof val !== 'number' || isNaN(val) || val < 73) {
    dsa3217Period.value = 500
  } else if (val > 65535) {
    dsa3217Period.value = 65535
  } else if (val !== Math.round(val)) {
    dsa3217Period.value = Math.round(val)
  }
}, { flush: 'sync' })

const dsa3217Loading = ref(false)
const enableAtmospheric = ref(false)

const PRESSURE_UNIT_OPTIONS = ['Pa', 'kPa', 'MPa', 'psi', 'kgf/cm2'] as const

const DISCOVERED_TYPE_ICON: Record<string, string> = {
  'DAQ-T-1603': 'T',
  'DAQ-P-1604': 'P',
  'DAQ-P-1064Pre': 'S',
}

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
  const has18 = (type === 'DAQ-P-1604' || type === 'DAQ-P-1064Pre') && channels.length >= 18
  if (!has18) {
    // SIMULATED 设备大气通道使用 kPa / degC，与后端默认值一致
    if (type === 'SIMULATED' && channels.length >= 18) {
      if (channels[16]) channels[16].unit = 'kPa'
      if (channels[17]) channels[17].unit = 'degC'
    }
    return
  }
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
        index: i, name: `TC${i + 1}`, enabled: true, unit: 'degC', precision: 2, thermocoupleType: 'K',
      }))
    case 'SIMULATED':
      return [
        ...Array.from({ length: 16 }, (_, i) => ({
          index: i, name: `CH${i + 1}`, enabled: true, unit: 'V', precision: 3, rangeMin: -10, rangeMax: 10,
        })),
        { index: 16, name: '大气压', enabled: true, unit: 'kPa', precision: 3, rangeMin: 99, rangeMax: 106 },
        { index: 17, name: '大气温度', enabled: true, unit: 'degC', precision: 2, rangeMin: 20, rangeMax: 25 },
      ]
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
      ? { thermocoupleTypes: 'KKKKKKKKKKKKKKKK', channelMask: 'FFFF', samplingRate: 10, binaryFormat: false, averageCount: 4, triggerMode: 0, triggerEdge: 0, triggerCount: 0, showTimestamp: false, showSequence: false, openCircuitCheck: '0000' }
      : undefined,
  }
}

const TC_TYPES = ['K', 'J', 'T', 'E', 'N', 'R', 'S', 'B']

function cloneProfile(p: DeviceProfile): DeviceProfile {
  try { return structuredClone(p) } catch { return JSON.parse(JSON.stringify(p)) }
}

function snapshotDraft(p: DeviceProfile): string {
  return JSON.stringify(p)
}

const isDirty = computed(() => snapshotDraft(draft.value) !== initialDraftSnapshot.value)
const isReadOnly = computed(() => editorMode.value === 'edit' && deviceStore.acquiringFor(draft.value.id))
const statusForDraft = computed(() => deviceStore.statusFor(draft.value.id))

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
  return draft.value.channels
    .map((channel, originalIndex) => ({ channel, originalIndex }))
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
  // DSA3217：初始化扫描参数
  if (draft.value.type === 'DSA3217') {
    dsa3217Avg.value = 32
    dsa3217Period.value = 500
  }
  enableAtmospheric.value = draft.value.type === 'DAQ-P-1604' ? false : (draft.value.channels[16]?.enabled !== false)
  if (draft.value.type === 'DAQ-P-1604' && draft.value.channels.length > 16) {
    draft.value.channels[16] = { ...draft.value.channels[16], enabled: false }
    draft.value.channels[17] = { ...draft.value.channels[17], enabled: false }
  }
  initialDraftSnapshot.value = snapshotDraft(draft.value)
  saveError.value = null
  editorOpen.value = true
  // DSA3217 已连接时自动读取扫描配置
  if (draft.value.type === 'DSA3217' && statusForDraft.value === 'Connected') {
    void loadDsa3217Config()
  }
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
  // DAQ-T-1603：将存储的 16 字符热电偶类型字符串分发到各通道
  if (draft.value.type === 'DAQ-T-1603' && draft.value.daqT1603Config) {
    const stored = draft.value.daqT1603Config.thermocoupleTypes
    for (let i = 0; i < Math.min(16, draft.value.channels.length); i++) {
      draft.value.channels[i].thermocoupleType = stored[i] || 'K'
    }
  }
  // DSA3217：恢复上次保存的 AVG/PERIOD（未连接时初始化为默认）
  dsa3217Avg.value = 32
  dsa3217Period.value = 500
  enableAtmospheric.value = draft.value.channels[16]?.enabled !== false
  initialDraftSnapshot.value = snapshotDraft(draft.value)
  saveError.value = null
  editorOpen.value = true
  // DSA3217 已连接时自动读取扫描配置
  if (draft.value.type === 'DSA3217' && statusForDraft.value === 'Connected') {
    void loadDsa3217Config()
  }
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
    draft.value.daqT1603Config = { thermocoupleTypes: 'KKKKKKKKKKKKKKKK', channelMask: 'FFFF', samplingRate: 10, binaryFormat: false, averageCount: 4, triggerMode: 0, triggerEdge: 0, triggerCount: 0, showTimestamp: false, showSequence: false, openCircuitCheck: '0000' }
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
  // SIMULATED 设备大气通道使用 kPa / degC，DAQ-P-1604 使用 Pa / ℃
  const pressureUnit = draft.value.type === 'SIMULATED' ? 'kPa' : 'Pa'
  const tempUnit = draft.value.type === 'SIMULATED' ? 'degC' : '℃'
  if (count > 16) draft.value.channels[16] = { ...draft.value.channels[16], enabled: on, unit: on ? pressureUnit : '' }
  if (count > 17) draft.value.channels[17] = { ...draft.value.channels[17], enabled: on, unit: on ? tempUnit : '' }
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
    if (draft.value.type === 'DAQ-T-1603' && draft.value.daqT1603Config) {
      const tcStr = draft.value.channels.slice(0, 16).map((c) => c.thermocoupleType || 'K').join('')
      draft.value.daqT1603Config.thermocoupleTypes = tcStr
    }
    const normalized: DeviceProfile = { ...draft.value, name: draft.value.name.trim() }
    await deviceApi.upsertProfile(normalized)
    await deviceStore.refreshProfiles()
    // DSA3217：已连接时自动写入 AVG/PERIOD 并保存到设备
    if (normalized.type === 'DSA3217' && statusForDraft.value === 'Connected') {
      try {
        const verify = await deviceStore.applyDsa3217ScanConfig(normalized.id, {
          avg: dsa3217Avg.value,
          period: dsa3217Period.value
        })
        // 写入成功后用回读数据更新 UI，确认实际生效值
        if (verify) {
          dsa3217Avg.value = verify.avg
          dsa3217Period.value = verify.period
        }
      } catch (e) {
        console.warn('同步 DSA3217 扫描参数失败:', e)
      }
    }
    if (normalized.autoConnect) {
      // 走 store.connect 以触发乐观更新，UI 可立即显示"连接中"
      try { await deviceStore.connect(normalized.id) } catch { /* 连接失败不阻塞保存 */ }
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

async function loadDsa3217Config(): Promise<void> {
  dsa3217Loading.value = true
  try {
    const config = await deviceStore.getDsa3217ScanConfig(draft.value.id)
    if (config) {
      dsa3217Avg.value = config.avg
      dsa3217Period.value = config.period
    }
  } catch (e) {
    console.warn('读取 DSA3217 配置失败:', e)
  } finally {
    dsa3217Loading.value = false
  }
}

// DSA3217 连接成功后自动读取扫描参数
watch(
  statusForDraft,
  (status) => {
    if (status === 'Connected' && draft.value.type === 'DSA3217' && editorOpen.value) {
      void loadDsa3217Config()
    }
  }
)

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
    try { await deviceStore.disconnect(p.id) } catch (e) { feedback.pushToast(String(e), 'error') }
  } else {
    // 走 store.connect 以触发乐观更新，按钮可立刻显示"连接中..."
    try { await deviceStore.connect(p.id) } catch (e) { feedback.pushToast(String(e), 'error') }
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
    await deviceApi.deleteProfile(p.id)
    await deviceStore.refreshProfiles()
    feedback.pushToast('设备配置已删除', 'info')
  } catch (e) { feedback.pushToast(String(e), 'error') }
}

async function bulkConnect() {
  for (const id of selectedIds.value) {
    // 走 store.connect 以触发乐观更新，让卡片立刻显示"连接中"
    try { await deviceStore.connect(id) } catch { /* 跳过 */ }
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
      await deviceApi.deleteProfile(id)
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
  // 连接中：明确显示"连接中..."并配合按钮 disabled 防止重复点击
  if (st === 'Connecting') return '连接中...'
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
          <UiButton quaternary size="md" @click="close">✕</UiButton>
        </header>

        <div class="drawer-toolbar">
          <UiButton variant="primary" size="md" @click="openCreate()">
            <span class="btn-icon">+</span> 新建设备
          </UiButton>
          <UiButton secondary size="md" :disabled="scanning" @click="runScan">
            <span class="btn-icon" :class="{ spin: scanning }">⟳</span>
            {{ scanning ? '扫描中...' : '扫描' }}
          </UiButton>
          <div class="drawer-total">
            设备: {{ deviceStore.profiles.length }}
          </div>
        </div>

        <!-- 发现的设备区域：可折叠，减少认知负荷 -->
        <div v-if="discovered.length" class="drawer-discovered">
          <div class="drawer-discovered-head" @click="showDiscovered = !showDiscovered" style="cursor: pointer;">
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span class="drawer-discovered-label">发现的设备</span>
              <span class="discovered-count">{{ discovered.length }}</span>
            </div>
            <div class="drawer-discovered-actions">
              <UiButton variant="primary" size="sm" @click.stop="addAllDiscoveredDevices">全部添加</UiButton>
              <UiButton quaternary size="sm" @click.stop="clearDiscovered">✕</UiButton>
              <span class="discovered-toggle" :class="{ 'discovered-toggle--open': showDiscovered }">▼</span>
            </div>
          </div>
          <Transition name="discovered-expand">
            <div v-show="showDiscovered" class="drawer-discovered-list">
              <div v-for="d in discovered" :key="d.id" class="discovered-card">
                <div class="discovered-card-icon">{{ DISCOVERED_TYPE_ICON[d.type] ?? 'D' }}</div>
                <div class="discovered-card-info">
                  <div class="discovered-card-name">{{ d.name }}</div>
                  <div class="discovered-card-type">
                    {{ d.type }}
                    <span v-if="d.address" class="discovered-card-addr"> · {{ d.address }}<template v-if="d.port">:{{ d.port }}</template></span>
                  </div>
                  <!-- 精简元数据：仅显示关键信息，hover 时显示完整信息 -->
                  <div class="discovered-card-meta">
                    <span v-if="d.model" class="discovered-meta-badge">{{ d.model }}</span>
                    <span v-if="d.firmwareVersion" class="discovered-meta-badge">FW: {{ d.firmwareVersion }}</span>
                  </div>
                  <div v-if="matchedProfileForDiscovered(d)" class="discovered-matched">
                    已匹配: {{ matchedProfileForDiscovered(d)?.name }}
                  </div>
                </div>
                <UiButton size="sm" @click="handleDiscoveredDeviceAction(d)">
                  {{ discoveryActionLabel(d) }}
                </UiButton>
              </div>
            </div>
          </Transition>
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
                  <UiCheckbox
                    :checked="isSelected(p.id)"
                    @update:checked="toggleSelected(p.id)" />
                  <h3 class="device-card-name">{{ p.name }}</h3>
                  <span class="device-card-type-badge">{{ p.type }}</span>
                </div>
                <div class="device-card-meta">
                  <span><Plug class="meta-icon" /> {{ p.transport === 'serial' ? (p.serialPort || 'COM?') : `${p.address || '-'}:${p.port || '-'}` }}</span>
                  <span><Zap class="meta-icon" /> {{ p.samplingRate ?? 20 }}Hz</span>
                  <span><LayoutGrid class="meta-icon" /> {{ p.channels?.length ?? 0 }} 通道</span>
                </div>
              </div>

              <div class="device-card-right">
                <UiButton secondary size="md" @click="openEdit(p)">编辑</UiButton>
                <UiButton size="md" :disabled="deviceStore.statusFor(p.id) === 'Connecting'" @click="connectToggle(p)">
                  {{ connectLabel(p) }}
                </UiButton>
                <UiButton v-if="deviceStore.statusFor(p.id) === 'Connected'" size="md" @click="toggleAcquisition(p)">
                  {{ deviceStore.acquiringFor(p.id) ? '停止' : '采集' }}
                </UiButton>
                <UiButton variant="danger" size="md" secondary @click="removeProfile(p)">删除</UiButton>
              </div>
            </div>

            <div v-if="deviceStore.statusFor(p.id) === 'Error'" class="device-card-error">
              <svg class="error-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              设备通信错误
            </div>
          </div>
        </main>

        <div v-if="selectedIds.length" class="drawer-bulk">
          <span>已选 <strong>{{ selectedCount }}</strong></span>
          <UiButton variant="primary" size="sm" :disabled="!selectedCount" @click="bulkConnect">批量连接</UiButton>
          <UiButton secondary size="sm" :disabled="!selectedCount" @click="bulkDisconnect">批量断开</UiButton>
          <UiButton variant="danger" size="sm" :disabled="!selectedCount" @click="bulkDelete">批量删除</UiButton>
          <UiButton quaternary size="sm" @click="clearSelection">清除</UiButton>
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
            <UiButton quaternary size="md" @click="tryCloseEditor">✕</UiButton>
          </header>

          <!-- 标签页切换 -->
          <div class="editor-tabs">
            <div class="editor-tabs-inner">
              <UiButton
                quaternary
                size="md"
                class="editor-tab"
                :class="{ active: editorTab === 'basic' }"
                @click="editorTab = 'basic'"
              >
                基本信息
              </UiButton>
              <UiButton
                quaternary
                size="md"
                class="editor-tab"
                :class="{ active: editorTab === 'channels' }"
                @click="editorTab = 'channels'"
              >
                通道配置
              </UiButton>
            </div>
          </div>

          <div class="editor-body">
            <!-- 保存错误提示横幅 -->
            <Transition name="banner">
              <div v-if="saveError" class="editor-error-banner">
                <svg class="banner-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <circle cx="12" cy="12" r="10"/>
                  <line x1="12" y1="8" x2="12" y2="12"/>
                  <line x1="12" y1="16" x2="12.01" y2="16"/>
                </svg>
                <span>保存失败: {{ saveError }}</span>
              </div>
            </Transition>

            <!-- 只读模式提示横幅 -->
            <Transition name="banner">
              <div v-if="isReadOnly" class="editor-readonly-banner">
                <svg class="banner-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                  <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
                </svg>
                <span>设备正在采集中，无法修改配置</span>
              </div>
            </Transition>

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
                    <UiInput v-model="draft.name" :disabled="isReadOnly" placeholder="输入设备名称" />
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
                    <UiInputNumber v-model="draft.samplingRate" class="w-full" :disabled="isReadOnly" />
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

              <!-- 大气数据开关（SIMULATED / DAQ-P-1604） -->
              <section v-if="draft.type === 'DAQ-P-1604' || draft.type === 'SIMULATED'" class="editor-section">
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

              <!-- DSA3217 扫描参数（在已连接设备的基本信息中显示） -->
              <section v-if="draft.type === 'DSA3217' && statusForDraft === 'Connected'" class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">DSA3217 扫描参数</h4>
                  <p class="editor-section-desc">平均值、周期与数据帧率（保存设备配置时自动写入）</p>
                </div>
                <div class="editor-grid">
                  <div class="editor-field col-4">
                    <label class="editor-label">AVG（平均值 1~240）</label>
                    <UiInputNumber
                      v-model="dsa3217Avg"
                      :min="1" :max="240"
                      :disabled="isReadOnly"
                      class="w-full"
                    />
                  </div>
                  <div class="editor-field col-4">
                    <label class="editor-label">PERIOD（周期 73~65535 μs）</label>
                    <UiInputNumber
                      v-model="dsa3217Period"
                      :min="73" :max="65535"
                      :disabled="isReadOnly"
                      class="w-full"
                    />
                  </div>
                  <div class="editor-field col-4">
                    <label class="editor-label">FPS（数据帧率 Hz）</label>
                    <div class="editor-input editor-input-readonly">
                      {{ dsa3217Fps }}
                    </div>
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
                      <UiInput v-model="draft.address" :disabled="isReadOnly" placeholder="192.168.1.100" />
                      <div v-if="fieldErrors.address" class="editor-field-error">● {{ fieldErrors.address }}</div>
                    </div>
                    <div class="editor-field col-3">
                      <label class="editor-label">端口 *</label>
                      <UiInputNumber v-model="draft.port" class="w-full" :disabled="isReadOnly" />
                      <div v-if="fieldErrors.port" class="editor-field-error">● {{ fieldErrors.port }}</div>
                    </div>
                  </template>

                  <template v-if="isTcpType(draft.type) && draft.transport === 'serial'">
                    <div class="editor-field col-7">
                      <label class="editor-label">串口号 *</label>
                      <UiInput v-model="draft.serialPort" :disabled="isReadOnly" placeholder="COM1" />
                      <div v-if="fieldErrors.serialPort" class="editor-field-error">● {{ fieldErrors.serialPort }}</div>
                    </div>
                    <div class="editor-field col-5">
                      <label class="editor-label">波特率 *</label>
                      <UiInputNumber v-model="draft.baudRate" class="w-full" :disabled="isReadOnly" />
                      <div v-if="fieldErrors.baudRate" class="editor-field-error">● {{ fieldErrors.baudRate }}</div>
                    </div>
                  </template>

                  <div class="editor-field col-12">
                    <div class="editor-autoconnect-row">
                      <UiCheckbox v-model:checked="draft.autoConnect" :disabled="isReadOnly">
                        自动连接（应用启动时及保存后自动连接）
                      </UiCheckbox>
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
                  v-model:channel-mask="draft.daqT1603Config!.channelMask"
                  v-model:sampling-rate="draft.daqT1603Config!.samplingRate"
                  v-model:binary-format="draft.daqT1603Config!.binaryFormat"
                  v-model:trigger-mode="draft.daqT1603Config!.triggerMode"
                  v-model:trigger-edge="draft.daqT1603Config!.triggerEdge"
                  v-model:trigger-count="draft.daqT1603Config!.triggerCount"
                  v-model:show-timestamp="draft.daqT1603Config!.showTimestamp"
                  v-model:open-circuit-check="draft.daqT1603Config!.openCircuitCheck"
                />
                <div class="editor-channels-table-wrap">
                  <table class="editor-channels-table">
                    <thead>
                      <tr>
                        <th class="w-14">#</th>
                        <th>通道名称</th>
                        <th>热电偶类型</th>
                        <th class="w-20 text-right">单位</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="c in draft.channels" :key="c.index">
                        <td class="font-mono">{{ channelLabel(c).padStart(2, '0') }}</td>
                        <td>
                          <UiInput v-model="c.name" :disabled="isReadOnly" />
                        </td>
                        <td>
                          <UiSelect v-model="c.thermocoupleType" :options="[{label:'K',value:'K'},{label:'T',value:'T'},{label:'E',value:'E'},{label:'J',value:'J'},{label:'N',value:'N'},{label:'S',value:'S'},{label:'R',value:'R'},{label:'B',value:'B'}]" class="editor-ch-select-min-width" :disabled="isReadOnly" />
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
                    <UiButton secondary size="sm" :disabled="isReadOnly" @click="setAllChannels(true)">全部启用</UiButton>
                    <UiButton secondary size="sm" :disabled="isReadOnly" @click="setAllChannels(false)">全部禁用</UiButton>
                    <UiButton secondary size="sm" :disabled="isReadOnly" @click="resetChannelsToDefault">重置</UiButton>
                  </div>
                </div>

                <!-- 批量同步：紧凑单行 -->
                <div class="editor-ch-batch">
                  <span class="editor-ch-batch-label">批量应用到 1~16CH:</span>
                  <div class="editor-ch-batch-field">
                    <span class="editor-ch-batch-field-label">量程</span>
                    <UiInputNumber
                      v-model="deviceRangeMin"
                      class="editor-ch-batch-num"
                      :disabled="isReadOnly"
                      placeholder="最小"
                    />
                    <span class="editor-ch-batch-sep">~</span>
                    <UiInputNumber
                      v-model="deviceRangeMax"
                      class="editor-ch-batch-num"
                      :disabled="isReadOnly"
                      placeholder="最大"
                    />
                  </div>
                  <div class="editor-ch-batch-field">
                    <span class="editor-ch-batch-field-label">精度</span>
                    <UiInputNumber
                      v-model="devicePrecision"
                      class="editor-ch-batch-num editor-ch-batch-num--narrow"
                      :min="0"
                      :disabled="isReadOnly"
                      placeholder="0"
                    />
                    <span class="editor-ch-batch-field-suffix">位小数</span>
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
                          <UiCheckbox v-model:checked="row.channel.enabled" :disabled="isReadOnly" />
                        </td>
                        <td class="font-mono">{{ channelLabel(row.channel).padStart(2, '0') }}</td>
                        <td>
                          <UiInput v-model="row.channel.name" :disabled="isReadOnly" />
                        </td>
                        <td>
                          <div class="editor-ch-range">
                            <UiInputNumber v-model="row.channel.rangeMin" class="w-full" :disabled="isReadOnly || row.originalIndex >= 16" />
                            <span>~</span>
                            <UiInputNumber v-model="row.channel.rangeMax" class="w-full" :disabled="isReadOnly || row.originalIndex >= 16" />
                          </div>
                        </td>
                        <td>
                          <UiInputNumber v-model="row.channel.precision" class="w-full" :min="0" :disabled="isReadOnly || row.originalIndex >= 16" />
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
              <!-- 只读模式状态指示 -->
              <div v-if="isReadOnly" class="editor-footer-readonly">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                  <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
                </svg>
                只读模式
              </div>
              <!-- 校验错误状态指示 -->
              <div v-else-if="validationErrorCount > 0" class="editor-footer-errors">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <circle cx="12" cy="12" r="10"/>
                  <line x1="12" y1="8" x2="12" y2="12"/>
                  <line x1="12" y1="16" x2="12.01" y2="16"/>
                </svg>
                校验失败: {{ validationErrorCount }} 项错误
              </div>
              <!-- 正常状态指示 -->
              <div v-else class="editor-footer-status" :class="{ dirty: isDirty }">
                <svg v-if="isDirty" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <circle cx="12" cy="12" r="10"/>
                  <line x1="12" y1="6" x2="12" y2="12"/>
                  <polyline points="12 16 12 16"/>
                </svg>
                <svg v-else width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <polyline points="20 6 9 17 4 12"/>
                </svg>
                {{ isDirty ? '检测到未保存的变更' : '配置已同步' }}
              </div>
            </div>
            <div class="editor-footer-right">
              <UiButton secondary @click="tryCloseEditor">{{ isReadOnly ? '关闭' : '取消' }}</UiButton>
              <UiButton
                v-if="!isReadOnly"
                variant="primary"
                size="md"
                :disabled="saving || validationErrorCount > 0"
                @click="saveDraft"
              >
                <span v-if="saving" class="btn-spinner" />
                {{ saving ? '保存中...' : '保存' }}
              </UiButton>
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
  width: 760px; max-width: 95vw; height: 100vh;
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

.drawer-title { margin: 0; font-size: var(--font-size-lg); font-weight: 800; color: var(--text-primary); letter-spacing: -0.02em; }
.drawer-subtitle { margin: 0.25rem 0 0; font-size: var(--font-size-xs); color: var(--text-muted); font-weight: 600; }

.drawer-close {
  width: var(--space-8); height: var(--space-8); display: flex; align-items: center; justify-content: center;
  border-radius: 0.5rem; color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05); border: 1px solid rgba(255, 255, 255, 0.08);
  font-size: var(--font-size-sm); transition: all 0.2s;
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
  font-size: var(--font-size-micro); font-weight: 800; letter-spacing: 0.1em;
  color: var(--text-muted); text-transform: uppercase;
}

.btn {
  display: inline-flex; align-items: center; gap: 0.375rem;
  padding: 0.5rem 1rem; border-radius: 0.5rem;
  font-size: var(--font-size-xs); font-weight: 700;
  transition: all 0.2s; cursor: pointer; border: 1px solid transparent;
}
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: var(--color-success); color: white; box-shadow: 0 var(--space-1) var(--space-3) rgba(16,185,129,0.3); }
.btn-primary:hover { background: var(--color-success); }
.btn-second { background: rgba(59,130,246,0.1); color: var(--color-accent); border-color: rgba(59,130,246,0.2); }
.btn-second:hover { background: rgba(59,130,246,0.2); color: var(--color-accent); border-color: rgba(59,130,246,0.4); }
.btn-green { background: var(--color-success); color: white; }
.btn-green:hover { background: var(--color-success); }
.btn-danger { background: rgba(244,63,94,0.1); color: var(--color-danger); border-color: rgba(244,63,94,0.2); }
.btn-danger:hover { background: rgba(244,63,94,0.2); }
.btn-warn { background: rgba(245,158,11,0.1); color: var(--color-warning); border-color: rgba(245,158,11,0.2); }
.btn-sm { padding: 0.375rem 0.75rem; font-size: var(--font-size-xs); white-space: nowrap; }
.btn-xs { padding: 0.25rem 0.5rem; font-size: var(--font-size-micro); }
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
.drawer-discovered-head { display: flex; align-items: center; justify-content: space-between; padding: 0.5rem 0; transition: opacity 0.2s; }
.drawer-discovered-head:hover { opacity: 0.8; }
.drawer-discovered-label { font-size: var(--font-size-micro); font-weight: 800; letter-spacing: 0.15em; text-transform: uppercase; color: var(--text-muted); }
.drawer-discovered-actions { display: flex; align-items: center; gap: 0.5rem; }
.discovered-count { padding: 0.125rem 0.5rem; border-radius: 999px; background: color-mix(in srgb, var(--accent-primary) 15%, transparent); color: var(--accent-primary); font-size: var(--font-size-micro); font-weight: 800; }
.discovered-toggle { font-size: var(--font-size-xs); color: var(--text-muted); transition: transform 0.2s; display: inline-block; }
.discovered-toggle--open { transform: rotate(180deg); }
.discovered-expand-enter-active, .discovered-expand-leave-active { transition: all 0.25s ease; }
.discovered-expand-enter-from, .discovered-expand-leave-to { opacity: 0; max-height: 0; overflow: hidden; }
.discovered-expand-enter-to, .discovered-expand-leave-from { opacity: 1; max-height: 500px; }
.drawer-discovered-list { display: flex; flex-direction: column; gap: 0.5rem; max-height: 30vh; overflow-y: auto; }
.discovered-card {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem; border-radius: 0.75rem;
  background: var(--bg-panel); border: 1px solid var(--border-default);
}
.discovered-card-icon {
  width: var(--space-8); height: var(--space-8); display: flex; align-items: center; justify-content: center;
  border-radius: 50%; background: rgba(59,130,246,0.1); color: var(--color-accent);
  font-size: var(--font-size-xs); font-weight: 800; flex-shrink: 0;
}
.discovered-card-name { font-size: var(--font-size-sm); font-weight: 700; color: var(--text-primary); }
.discovered-card-type { font-size: var(--font-size-2xs); font-weight: 600; color: var(--text-muted); margin-top: 0.125rem; }
.discovered-card-addr { color: var(--text-muted); opacity: 0.7; }
.discovered-card-meta { display: flex; flex-wrap: wrap; gap: 0.25rem; margin-top: 0.25rem; }
.discovered-meta-badge {
  display: inline-flex; align-items: center;
  padding: 0.0625rem 0.375rem; border-radius: 4px;
  font-size: var(--font-size-micro); font-weight: 600;
  background: var(--bg-secondary); color: var(--text-tertiary);
  border: 1px solid var(--border-default);
}
.discovered-matched {
  margin-top: 0.25rem; display: inline-flex; align-items: center;
  padding: 0.125rem 0.5rem; border-radius: 999px;
  background: rgba(16,185,129,0.1); border: 1px solid rgba(16,185,129,0.3);
  font-size: var(--font-size-micro); font-weight: 700; color: var(--color-success);
}
.discovered-card .btn-green, .discovered-card .btn-second { margin-left: auto; flex-shrink: 0; }

.drawer-list { flex: 1; overflow-y: auto; padding: 1rem 1.5rem; display: flex; flex-direction: column; gap: 0.75rem; }
.drawer-empty { padding: 2rem 1rem; text-align: center; color: var(--text-muted); font-size: var(--font-size-sm); }

.device-card {
  position: relative; overflow: hidden;
  border-radius: 0.75rem; border: 1px solid var(--border-default);
  background: var(--bg-panel); transition: all 0.2s;
}
.device-card:hover { border-color: var(--accent-success); }
.device-card-stripe { position: absolute; left: 0; top: 0; bottom: 0; width: var(--space-1); background: var(--text-muted); transition: all 0.3s; }
.device-card-stripe.status-online { background: var(--color-success); box-shadow: 0 0 var(--space-2) rgba(16,185,129,0.5); }
.device-card-stripe.status-acq { background: var(--color-success); box-shadow: 0 0 var(--space-3) rgba(16,185,129,0.6); animation: pulse 1.5s infinite; }
.device-card-stripe.status-connecting { background: var(--color-warning); animation: pulse 0.8s infinite; }
.device-card-body { padding: 1rem; display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.device-card-left { min-width: 0; flex: 1; }
.device-card-row { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.5rem; }
.device-checkbox { width: var(--space-3); height: var(--space-3); border-radius: var(--radius-sm); flex-shrink: 0; accent-color: var(--color-accent); }
.device-card-name { margin: 0; font-size: var(--font-size-base); font-weight: 700; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.device-card-type-badge {
  flex-shrink: 0; padding: 0.125rem 0.5rem; border-radius: 0.25rem;
  background: var(--bg-panel-strong); font-size: var(--font-size-micro); font-weight: 800;
  letter-spacing: 0.05em; color: var(--text-muted); text-transform: uppercase;
}
.device-card-meta { display: flex; flex-wrap: wrap; gap: 1rem; font-size: var(--font-size-2xs); font-weight: 600; color: var(--text-muted); }
.device-card-meta span { display: inline-flex; align-items: center; gap: 0.25rem; }
.device-card-meta .meta-icon { width: 12px; height: 12px; opacity: 0.7; }
.device-card-error { display: flex; align-items: center; gap: 0.375rem; }
.device-card-error .error-icon { flex-shrink: 0; }
.device-card-right { display: flex; flex-direction: column; gap: 0.375rem; flex-shrink: 0; }
.device-card-error {
  margin: 0 1rem 0.75rem; padding: 0.5rem 0.75rem; border-radius: var(--radius-lg);
  background: rgba(244,63,94,0.1); border: 1px solid rgba(244,63,94,0.2);
  font-size: var(--font-size-2xs); font-weight: 600; color: var(--color-danger);
}
.device-card-right .btn.ghost { background: transparent; color: var(--text-muted); border: 1px dashed var(--border-default); }
.device-card-right .btn.ghost:hover { color: var(--color-danger); border-color: var(--color-danger); }

.drawer-bulk {
  flex-shrink: 0; display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem 1.5rem; border-top: 1px solid var(--border-default);
  background: var(--bg-panel-strong); font-size: var(--font-size-xs); color: var(--text-secondary);
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
  box-shadow: 0 var(--space-8) 64px -12px rgba(0, 0, 0, 0.5);
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
  width: var(--space-10); height: var(--space-10); display: flex; align-items: center; justify-content: center;
  border-radius: 0.75rem; background: rgba(59,130,246,0.1); color: var(--color-accent);
  font-size: 1.25rem; font-weight: 800;
}
.editor-title { margin: 0; font-size: 1rem; font-weight: 800; color: var(--text-primary); }
.editor-status-row { display: flex; align-items: center; gap: 0.5rem; margin-top: 0.25rem; }
.editor-status-dot {
  width: var(--space-1); height: var(--space-1); border-radius: 50%; background: var(--text-muted);
}
.editor-status-dot.status-online { background: var(--color-success); box-shadow: 0 0 var(--space-1) rgba(16,185,129,0.5); }
.editor-status-dot.status-acq { background: var(--color-success); box-shadow: 0 0 var(--space-1) rgba(16,185,129,0.6); animation: pulse 1.5s infinite; }
.editor-status-dot.status-connecting { background: var(--color-warning); animation: pulse 0.8s infinite; }
.editor-status-text { font-size: var(--font-size-xs); font-weight: 600; color: var(--text-muted); }

.editor-tabs {
  flex-shrink: 0; padding: 1rem 1.5rem;
  border-bottom: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
}
.editor-tabs-inner {
  display: inline-flex; border-radius: 0.75rem;
  background: var(--bg-app, rgba(0,0,0,0.2)); padding: 0.25rem;
}
:deep(.editor-tab) {
  padding: 0.5rem 1.5rem;
  font-size: var(--font-size-xs); font-weight: 800;
  cursor: pointer;
}
:deep(.editor-tab):hover { color: var(--text-primary); }
:deep(.editor-tab.active) { background: var(--bg-panel); color: var(--color-accent); }

.editor-body { padding: 1.5rem; overflow-y: auto; flex: 1; min-height: 0; }

/* 横幅过渡动画 */
.banner-enter-active,
.banner-leave-active {
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}
.banner-enter-from,
.banner-leave-to {
  opacity: 0;
  transform: translateY(-8px);
  max-height: 0;
  margin-bottom: 0;
  padding-top: 0;
  padding-bottom: 0;
}

.editor-error-banner {
  display: flex; align-items: center; gap: 0.625rem;
  padding: 0.875rem 1rem; border-radius: 0.625rem;
  background: rgba(244,63,94,0.08); border: 1px solid rgba(244,63,94,0.2);
  font-size: var(--font-size-xs); font-weight: 700; color: var(--color-danger);
  margin-bottom: 1.25rem;
}
.editor-readonly-banner {
  display: flex; align-items: center; gap: 0.625rem;
  padding: 0.875rem 1rem; border-radius: 0.625rem;
  background: rgba(245,158,11,0.08); border: 1px solid rgba(245,158,11,0.2);
  font-size: var(--font-size-xs); font-weight: 700; color: var(--color-warning);
  margin-bottom: 1.25rem;
}
.banner-icon {
  flex-shrink: 0;
  opacity: 0.9;
}

.editor-sections { display: flex; flex-direction: column; gap: 2rem; }
.editor-section { }
.editor-section-head { margin-bottom: 1rem; }
.editor-section-title { font-size: var(--font-size-xs); font-weight: 800; letter-spacing: 0.1em; color: var(--text-primary); text-transform: uppercase; margin: 0; }
.editor-section-desc { font-size: var(--font-size-2xs); font-weight: 700; color: var(--text-muted); margin-top: 0.25rem; }

.editor-grid { display: grid; grid-template-columns: repeat(12, 1fr); gap: 1rem; }
.editor-field { margin-bottom: 0; }
.col-2 { grid-column: span 2; }
.col-3 { grid-column: span 3; }
.col-4 { grid-column: span 4; }
.col-5 { grid-column: span 5; }
.col-6 { grid-column: span 6; }
.col-7 { grid-column: span 7; }
.col-12 { grid-column: span 12; }

.editor-label { display: block; margin-bottom: 0.375rem; font-size: var(--font-size-2xs); font-weight: 800; color: var(--text-muted); letter-spacing: 0.08em; text-transform: uppercase; }
.editor-input {
  width: 100%; padding: 0.625rem 0.75rem;
  border-radius: 0.5rem; border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2); color: var(--text-primary);
  font-family: var(--font-family-sans); font-size: var(--font-size-sm); font-weight: 700;
  outline: none; transition: all 0.2s;
}
.editor-input:focus { border-color: var(--color-accent); background: var(--bg-panel-strong); }
.editor-input:disabled { opacity: 0.6; cursor: not-allowed; }
.editor-input-readonly {
  display: flex; align-items: center;
  height: var(--space-10); font-weight: 700; color: var(--text-muted);
}
:root[data-theme='light'] .editor-input { background: rgba(255, 255, 255, 0.9); border-color: var(--border-strong); color: var(--text-primary); }
:root[data-theme='light'] .editor-input:focus { background: #ffffff; border-color: var(--accent-primary); }
.editor-field-error { margin-top: 0.375rem; font-size: var(--font-size-2xs); font-weight: 700; color: var(--color-danger); }

.editor-unit-row { display: flex; align-items: center; gap: 0.75rem; }
.editor-unit-select { flex: 1; }
.editor-unit-hint { font-size: var(--font-size-2xs); font-weight: 700; color: var(--text-muted); max-width: 200px; line-height: 1.4; }

.editor-atmo-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0.75rem 1rem; border-radius: 0.5rem;
  border: 1px solid var(--border-default); background: var(--bg-panel);
}
.editor-atmo-toggle { display: flex; align-items: center; gap: 0.75rem; cursor: pointer; }
.editor-toggle-track {
  width: 44px; height: var(--space-6); border-radius: 999px;
  background: var(--border-default); position: relative; transition: all 0.2s;
}
.editor-toggle-track.on { background: var(--color-accent); }
.editor-toggle-thumb {
  position: absolute; top: 2px; left: 3px;
  width: var(--space-5); height: var(--space-5); border-radius: 50%;
  background: white; transition: all 0.2s;
}
.editor-toggle-thumb.on { transform: translateX(var(--space-5)); }
.editor-atmo-label { font-size: var(--font-size-xs); font-weight: 700; color: var(--text-primary); }

.editor-autoconnect-row {
  display: flex; align-items: center; gap: 0.75rem;
  padding: 0.75rem 1rem; border-radius: 0.5rem;
  background: rgba(59,130,246,0.05);
}
.editor-autoconnect-label {
  display: flex; align-items: center; gap: 0.5rem;
  font-size: var(--font-size-xs); font-weight: 800; color: var(--color-accent); cursor: pointer;
}
.editor-autoconnect-check { width: var(--space-4); height: var(--space-4); accent-color: var(--color-accent); }

/* Channels */
.editor-channels-special { display: flex; flex-direction: column; gap: 1rem; }
.editor-channels-hint { font-size: var(--font-size-xs); color: var(--text-muted); margin: 0; }
.editor-channels-full { display: flex; flex-direction: column; gap: 0.625rem; }

.editor-channels-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; }
.editor-channels-toolbar-left { display: flex; gap: 0.375rem; }

/* 批量同步 - 紧凑单行 */
.editor-ch-batch {
  display: flex; align-items: center; gap: 1rem;
  padding: 0.375rem 0.75rem;
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--bg-panel-strong) 30%, transparent);
  border: 1px solid var(--border-default);
  flex-wrap: wrap;
}
.editor-ch-batch-label {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
  white-space: nowrap;
}
.editor-ch-batch-field {
  display: flex; align-items: center; gap: 0.375rem;
}
.editor-ch-batch-field-label {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--text-secondary);
  white-space: nowrap;
}
.editor-ch-batch-field-suffix {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  white-space: nowrap;
}
.editor-ch-batch-num { width: 88px; }
.editor-ch-batch-num--narrow { width: 56px; }
.editor-ch-batch-sep {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
  flex-shrink: 0;
}

/* Channel table */
.editor-channels-table-wrap {
  border-radius: 0.5rem;
  border: 1px solid var(--border-default);
  overflow: hidden;
  background: var(--bg-panel);
}
.editor-channels-table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  text-align: left;
}
.editor-channels-table thead tr {
  background: var(--bg-panel-strong);
}
.editor-channels-table th {
  padding: 0.375rem 0.625rem;
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0;
  text-transform: none;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-default);
  white-space: nowrap;
}
.editor-channels-table td {
  padding: 0.25rem 0.5rem;
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 40%, transparent);
  font-size: var(--font-size-xs);
  color: var(--text-primary);
  transition: background 120ms ease;
  vertical-align: middle;
}
.editor-channels-table tbody tr:last-child td {
  border-bottom: none;
}
.editor-channels-table tbody tr:hover td {
  background: color-mix(in srgb, var(--bg-panel-strong) 35%, transparent);
}

/* 紧凑 input：作用于表格内 UiInput/UiInputNumber 的内部元素 */
.editor-channels-table :deep(.n-input),
.editor-channels-table :deep(.n-input-number),
.editor-channels-table :deep(.n-base-selection) {
  --n-height: 28px;
  min-height: 28px;
}
.editor-channels-table :deep(.n-input__input-el),
.editor-channels-table :deep(.n-input-number .n-input__input-el) {
  height: 28px;
  line-height: 28px;
  font-size: var(--font-size-xs);
}
.editor-channels-table :deep(.n-input .n-input-wrapper) {
  padding-left: 0.5rem;
  padding-right: 0.5rem;
}
.editor-ch-check {
  width: var(--space-4);
  height: var(--space-4);
  accent-color: var(--color-accent);
  cursor: pointer;
}
.editor-ch-check:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
.editor-ch-input {
  width: 100%;
  padding: 0.375rem 0.625rem;
  border-radius: 0.375rem;
  border: 1px solid transparent;
  background: transparent;
  font-family: var(--font-family-sans);
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
  outline: none;
  transition: all 0.2s ease;
}
.editor-ch-input:hover {
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
}
.editor-ch-input:focus {
  background: var(--bg-panel);
  border-color: var(--color-accent);
}
.editor-ch-input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
:root[data-theme='light'] .editor-ch-input { color: var(--text-primary); }
:root[data-theme='light'] .editor-ch-input:focus { background: #ffffff; }
.editor-ch-range {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
}
.editor-ch-range-input {
  width: 80px;
  padding: 0.375rem 0.5rem;
  border-radius: 0.375rem;
  border: 1px solid transparent;
  background: color-mix(in srgb, var(--bg-panel-strong) 40%, transparent);
  font-family: var(--font-family-mono);
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
  text-align: right;
  outline: none;
  transition: all 0.2s ease;
}
.editor-ch-range-input:focus {
  border-color: var(--color-accent);
  background: var(--bg-panel);
}
.editor-ch-range-input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
:root[data-theme='light'] .editor-ch-range-input { color: var(--text-primary); }
:root[data-theme='light'] .editor-ch-range-input:focus { background: #ffffff; }
.editor-ch-tc {
  padding: 0.375rem 0.5rem;
  border-radius: 0.375rem;
  border: 1px solid transparent;
  background: transparent;
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-primary);
  outline: none;
  cursor: pointer;
  transition: all 0.2s ease;
}
.editor-ch-tc:hover {
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
  border-color: var(--border-default);
}
.editor-ch-tc:focus {
  background: var(--bg-panel);
  border-color: var(--color-accent);
}
.editor-ch-tc:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.editor-ch-precision-input {
  width: 100%;
  padding: 0.375rem 0.5rem;
  border-radius: 0.375rem;
  border: 1px solid transparent;
  background: color-mix(in srgb, var(--bg-panel-strong) 40%, transparent);
  font-family: var(--font-family-mono);
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
  text-align: right;
  outline: none;
  transition: all 0.2s ease;
}
.editor-ch-precision-input:focus {
  border-color: var(--color-accent);
  background: var(--bg-panel);
}
.editor-ch-precision-input:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
:root[data-theme='light'] .editor-ch-precision-input { color: var(--text-primary); }
:root[data-theme='light'] .editor-ch-precision-input:focus { background: #ffffff; }

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
.editor-footer-left {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}
.editor-footer-right { display: flex; gap: 0.75rem; }
.editor-footer-readonly {
  display: flex; align-items: center; gap: 0.375rem;
  font-size: var(--font-size-2xs); font-weight: 800; color: var(--color-warning);
  text-transform: uppercase; letter-spacing: 0.05em;
}
.editor-footer-errors {
  display: flex; align-items: center; gap: 0.375rem;
  font-size: var(--font-size-2xs); font-weight: 800; color: var(--color-danger);
  text-transform: uppercase; letter-spacing: 0.05em;
}
.editor-footer-status {
  display: flex; align-items: center; gap: 0.375rem;
  font-size: var(--font-size-2xs); font-weight: 800; color: var(--text-muted);
  text-transform: uppercase; letter-spacing: 0.1em;
  transition: color 0.2s ease;
}
.editor-footer-status.dirty { color: var(--color-warning); }

/* 保存按钮加载动画 */
.btn-saving {
  position: relative;
  padding-left: 2rem;
}
.btn-spinner {
  position: absolute;
  left: 0.625rem;
  top: 50%;
  transform: translateY(-50%);
  width: var(--space-3);
  height: var(--space-3);
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: white;
  border-radius: 50%;
  animation: btn-spin 0.8s linear infinite;
}
.w-full { width: 100%; }
.editor-ch-select-min-width { min-width: 80px; }
@keyframes btn-spin {
  to { transform: translateY(-50%) rotate(360deg); }
}
</style>
