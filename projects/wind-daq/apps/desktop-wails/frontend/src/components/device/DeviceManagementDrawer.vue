<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { deviceApi } from '@api/deviceApi'
import type { DeviceProfile, DeviceType, ScanResult, ChannelConfig, ChannelSensorType } from '@api/types'
import UiSelect from '@components/ui/UiSelect.vue'
import DaqT1603Config from '@components/device/DaqT1603Config.vue'
import DaqT1602Config from '@components/device/DaqT1602Config.vue'
import DaqP1603Config from '@components/device/DaqP1603Config.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiButton from '@components/ui/UiButton.vue'
import UiToggle from '@components/ui/UiToggle.vue'
// 图标统一使用 lucide-vue，与 GlobalSettingsModal 风格基线对齐：
// - 闭_close 按钮统一 X，新 device/编辑器头部用 Plus/Pencil/Lock 表达模式
// - 扫描按钮用 RotateCcw，可借助 UiButton :loading 实现原生 spinner
// - 折叠箭头用 ChevronDown 配合 CSS 旋转
import { AlertCircle, ChevronDown, Lock, Pencil, Plus, RotateCcw, X } from '@lucide/vue'
import DeviceCard from '@components/device/DeviceCard.vue'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()
const i18n = useI18nStore()

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
  'DAQ-P-1603': 'P',
  'DAQ-P-1604Pre': 'S',
}

const WTN_PXI_CHANNEL_NAMES = [
  '球罐压力', '球罐总压', '球罐静压',
  '球罐稳定时间', '球罐温度1', '球罐温度2', '球罐温度3', '球罐温度4',
] as const

const deviceTypeOptions = computed(() => [
  { value: 'SIMULATED', label: 'SIMULATED' },
  { value: 'DAQ-P-1604', label: 'DAQ-P-1604' },
  { value: 'DAQ-P-1603', label: 'DAQ-P-1603' },
  { value: 'DAQ-P-1604Pre', label: 'DAQ-P-1604Pre' },
  { value: 'DAQ-T-1603', label: 'DAQ-T-1603' },
  { value: 'DAQ-T-1602', label: 'DAQ-T-1602' },
  { value: 'WTN_PXI', label: 'WTN_PXI' },
  { value: 'DSA3217', label: 'DSA3217' },
])

const transportOptions = computed(() => [
  { value: 'tcp', label: 'TCP/IP' },
  { value: 'serial', label: i18n.t.dev_serialRs232 },
])

const pressureUnitOptions = computed(() =>
  PRESSURE_UNIT_OPTIONS.map((u) => ({ value: u, label: u }))
)

function isTcpType(t: DeviceType): boolean {
  // DAQ-P-1603 走 DLL FFI 路径，DLL 内部封装 TCP，对外仍属 TCP 类型
  return t === 'DAQ-P-1604' || t === 'DAQ-P-1603' || t === 'DAQ-P-1604Pre' || t === 'DAQ-T-1603' || t === 'DAQ-T-1602' || t === 'WTN_PXI' || t === 'DSA3217'
}

function supportsTransportSwitch(t: DeviceType): boolean {
  // DAQ-P-1603 由 DLL 管理通信，不支持切串口；DAQ-T-1602 仅支持 Modbus TCP
  return t !== 'WTN_PXI' && t !== 'DAQ-P-1604' && t !== 'DAQ-P-1603' && t !== 'DAQ-P-1604Pre' && t !== 'DAQ-T-1602' && t !== 'DSA3217'
}

function isPortRequired(t: DeviceType): boolean {
  // DAQ-P-1603 通过 WTNDAQ16H_64.dll 内部封装 TCP 通信，
  // profile.Port 字段不使用（DLL 自管端口），无需 UI 输入与校验。
  return t !== 'DAQ-P-1603'
}

function isTempUnitFixed(t: DeviceType): boolean {
  return t === 'DAQ-T-1603' || t === 'DAQ-T-1602'
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
  const has18 = (type === 'DAQ-P-1604' || type === 'DAQ-P-1604Pre') && channels.length >= 18
  if (!has18) {
    // SIMULATED 设备大气通道使用 Pa / degC，与后端默认值一致
    if (type === 'SIMULATED' && channels.length >= 18) {
      if (channels[16]) channels[16].unit = 'Pa'
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
    case 'DAQ-T-1602':
      // 热电偶类型由 daqT1602Config.typeCodes 承载（type code 0~7），通道表不存 thermocoupleType
      return Array.from({ length: 16 }, (_, i) => ({
        index: i, name: `TC${i + 1}`, enabled: true, unit: 'degC', precision: 2,
      }))
    case 'SIMULATED':
      return [
        ...Array.from({ length: 16 }, (_, i) => ({
          index: i, name: `CH${i + 1}`, enabled: true, unit: 'V', precision: 3, rangeMin: -10, rangeMax: 10,
        })),
        { index: 16, name: '大气压', enabled: true, unit: 'Pa', precision: 2, rangeMin: 99000, rangeMax: 106000 },
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
    case 'DAQ-P-1603':
      // DAQ-P-1603：16 通道通用 AI，每通道可接入压力或温度传感器。
      // 默认全部为压力通道（sensorType='pressure'），用户可在通道配置中切为温度。
      // 不含大气通道（用户决策无大气数据）。
      // 设备特殊默认：CH01/CH02（index 0/1）默认不应用校零（calibrationEnabled=false），
      // 前两路通常接入不参与校零的传感器（如总压/静压参考通道），与 DAQ-P-1604 区分。
      // 与后端 default_profiles.go NewDefaultProfile 的默认规则保持一致。
      return Array.from({ length: 16 }, (_, i) => ({
        index: i, name: `CH${i + 1}`, enabled: true, unit: 'Pa', precision: 3, rangeMin: -5000, rangeMax: 5000, sensorType: 'pressure' as ChannelSensorType, calibrationEnabled: i >= 2,
      }))
    case 'DAQ-P-1604Pre':
    case 'DSA3217':
      return Array.from({ length: 16 }, (_, i) => ({
        index: i, name: `CH${i + 1}`, enabled: true, unit: 'Pa', precision: 2, rangeMin: -5000, rangeMax: 5000,
      }))
    case 'WTN_PXI': {
      // 通道 3 为球罐稳定时间（秒），其余 4 路为球罐温度，与后端 defaultWTNPXIChannels 对齐。
      const names = ['球罐压力', '球罐总压', '球罐静压', '球罐稳定时间', '球罐温度1', '球罐温度2', '球罐温度3', '球罐温度4']
      const units = ['Pa', 'Pa', 'Pa', 's', 'degC', 'degC', 'degC', 'degC']
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
  let samplingRate = 20
  if (type === 'DAQ-P-1604' || type === 'DAQ-T-1603' || type === 'WTN_PXI') {
    address = '192.168.3.101'
    port = 9000
  }
  if (type === 'DAQ-P-1604Pre') {
    address = '192.168.1.100'
    port = 5000
  }
  if (type === 'DAQ-T-1602') {
    // Modbus TCP：默认 IP/端口与真机实测一致（spec §Protocol）
    address = '192.168.3.201'
    port = 502
  }
  if (type === 'DSA3217') {
    address = '192.168.1.254'
    port = 5000
  }
  if (type === 'DAQ-P-1603') {
    // DAQ-P-1603：DLL 自管 TCP 端口，profile.Port 不使用；
    // IP 留空让用户在"基本信息"面板手动输入（参考代码无扫描 API）。
    // 默认采样率 100Hz（用户采样率=每秒数据条目数）。
    // 底层硬件采样率固定 1000Hz，100Hz 意味着每 10 个原始点取平均输出 1 条。
    address = ''
    port = 0
    samplingRate = 100
  }
  return {
    id, name: '', type, transport: 'tcp', address, localAddress: '', port,
    serialPort: '', baudRate: 115200, samplingRate,
    autoConnect: true, channels,
    daqT1603Config: type === 'DAQ-T-1603'
      ? { thermocoupleTypes: 'KKKKKKKKKKKKKKKK', channelMask: 'FFFF', samplingRate: 10, binaryFormat: false, averageCount: 4, triggerMode: 0, triggerEdge: 0, triggerCount: 0, showTimestamp: false, showSequence: false, openCircuitCheck: '0000' }
      : undefined,
    // DAQ-T-1602 默认 16 通道全 T 型（type code 2），采样频率 5Hz（设备全速上限）
    daqT1602Config: type === 'DAQ-T-1602'
      ? { typeCodes: Array(16).fill(2), sampleRateHz: 5 }
      : undefined,
    // DAQ-P-1604 默认开启硬件时间戳（更精确），与 daq-p1604 项目对齐；用户可在"基本信息"中关闭回退到系统时间。
    daqP1604UseDeviceTimestamp: type === 'DAQ-P-1604' ? true : undefined,
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

// IP 地址字段的网格列宽：根据是否显示"传输方式"与"端口"字段动态计算，
// 让 IP 输入框填满剩余空间。
// 修复点：原先 DAQ-P-1603（无传输方式、无端口）使用未定义的 col-8 类，
// grid item 默认只占 1 列导致 IP 输入框极小。
// 12 列网格分配：传输方式 4 列 + 端口 3 列 + IP 占剩余列数。
const ipFieldColClass = computed(() => {
  const type = draft.value.type
  const showTransport = isTcpType(type) && supportsTransportSwitch(type)
  const showPort = isPortRequired(type)
  const ipCols = 12 - (showTransport ? 4 : 0) - (showPort ? 3 : 0)
  return `col-${ipCols}`
})

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
  if (!p.name.trim()) errors.name = i18n.t.dev_deviceNameEmpty
  else {
    const hasDup = deviceStore.profiles.some((e) => e.id !== p.id && e.name.trim() === p.name.trim())
    if (hasDup) errors.name = i18n.t.dev_deviceNameExists
  }
  if (isTcpType(p.type)) {
    if (p.transport === 'serial') {
      if (!p.serialPort?.trim()) errors.serialPort = i18n.t.dev_serialPortEmpty
      if (!Number.isFinite(p.baudRate ?? 0) || (p.baudRate ?? 0) <= 0) errors.baudRate = i18n.t.dev_baudRateInvalid
    } else {
      if (!p.address?.trim()) errors.address = i18n.t.dev_ipAddressEmpty
      // DAQ-P-1603 由 DLL 自管端口，profile.Port 不使用，跳过端口校验
      if (isPortRequired(p.type) && (!Number.isFinite(p.port ?? 0) || (p.port ?? 0) <= 0)) errors.port = i18n.t.dev_portInvalid
    }
  }
  // DAQ-P-1603 采样率范围 [1, 500] Hz（用户采样率=每秒数据条目数）。
  // 底层硬件采样率固定 1000Hz，低频时通过多点平均实现。
  if (p.type === 'DAQ-P-1603') {
    if (!Number.isFinite(p.samplingRate) || p.samplingRate < 1 || p.samplingRate > 500) {
      errors.samplingRate = i18n.t.dev_samplingRateInvalid
    }
  } else if (!Number.isFinite(p.samplingRate) || p.samplingRate <= 0) {
    errors.samplingRate = i18n.t.dev_samplingRateInvalid
  }
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
  // 通道列表默认全量展示，不再做关键词或启用状态过滤
  return draft.value.channels.map((channel, originalIndex) => ({ channel, originalIndex }))
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
  // DAQ-T-1602：兼容旧 profile，补齐缺省配置字段（16 通道全 T 型 + 5Hz）
  if (draft.value.type === 'DAQ-T-1602') {
    const config = draft.value.daqT1602Config ?? { typeCodes: Array(16).fill(2), sampleRateHz: 5 }
    draft.value.daqT1602Config = {
      typeCodes: config.typeCodes?.length ? config.typeCodes : Array(16).fill(2),
      sampleRateHz: config.sampleRateHz ?? 5,
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
  // DAQ-P-1603 已连接时回读硬件实际配置（采样率、通道传感器类型等），
  // 让用户看到硬件当前生效的值而非持久化的旧值。回读失败不阻塞编辑。
  if (draft.value.type === 'DAQ-P-1603' && statusForDraft.value === 'Connected') {
    void loadDaqP1603Config()
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
  } else if (next === 'DAQ-T-1602') {
    draft.value.daqT1602Config = { typeCodes: Array(16).fill(2), sampleRateHz: 5 }
    draft.value.address = '192.168.3.201'
    draft.value.port = 502
  } else if (next === 'DAQ-P-1604' || next === 'WTN_PXI') {
    draft.value.address = '192.168.3.101'
    draft.value.port = 9000
  } else if (next === 'DAQ-P-1604Pre') {
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
  const pressureUnit = 'Pa'
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
    // DAQ-P-1603：已连接时同步配置到硬件（ReleaseTask → VerifyParam →
    // InitTask 重新初始化任务），并回读实际生效的 profile。同步失败时
    // 向用户暴露错误（与 DSA3217 的 console.warn 不同：1603 的配置变更
    // 是用户主要操作意图，失败必须明确告知）。
    if (normalized.type === 'DAQ-P-1603' && statusForDraft.value === 'Connected') {
      const verify = await deviceStore.applyDaqP1603Config(normalized.id, normalized)
      if (verify) {
        // 用硬件实际生效值更新本地 draft（避免下次保存时再次下发未生效的值）
        draft.value = { ...draft.value, ...verify }
      }
    }
    if (normalized.autoConnect) {
      // 走 store.connect 以触发乐观更新，UI 可立即显示"连接中"
      try { await deviceStore.connect(normalized.id) } catch { /* 连接失败不阻塞保存 */ }
    }
    feedback.pushToast(i18n.t.dev_deviceSaved.replace('{name}', normalized.name), 'success')
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
  const ok = await feedback.confirm(i18n.t.dev_unsavedChangesCloseConfirm)
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
    // refreshProfiles 失败时 deviceStore 内部已 console.warn 并保留旧 profiles，
    // 这里显式 catch 避免未处理的 rejection 警告（drawer 打开是用户主动行为，失败不需要 toast）。
    void deviceStore.refreshProfiles().catch((e) => {
      console.warn('[DeviceManagementDrawer] refreshProfiles on open failed:', e)
    })
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

// DAQ-P-1603 回读硬件实际配置：已连接设备从 driver 获取最新 profile
// （含采样率、通道传感器类型、单位、量程等），用其覆盖 draft 以反映硬件
// 当前真实状态。回读失败不阻塞编辑（用户仍可基于持久化值修改）。
async function loadDaqP1603Config(): Promise<void> {
  try {
    const hardware = await deviceStore.getDaqP1603Config(draft.value.id)
    if (hardware) {
      // 保留 draft.id（hardware.id 应与 draft.id 一致，但避免意外覆盖）
      // 保留 draft.name（用户可见的设备名，不应被硬件值覆盖）
      // 其余字段（samplingRate、channels 等）采用硬件实际值
      draft.value = {
        ...draft.value,
        ...hardware,
        id: draft.value.id,
        name: draft.value.name,
      }
      // 同步批量同步输入框的初始值（取首个通道的量程/精度）
      const range = getDeviceRangeFromChannels(draft.value.channels)
      deviceRangeMin.value = range?.min ?? null
      deviceRangeMax.value = range?.max ?? null
      devicePrecision.value = getDevicePrecisionFromChannels(draft.value.channels)
      // 重置 dirty 基准：回读的值就是硬件当前生效值，不算未保存变更
      initialDraftSnapshot.value = snapshotDraft(draft.value)
    }
  } catch (e) {
    console.warn('读取 DAQ-P-1603 配置失败:', e)
  }
}

// DSA3217 / DAQ-P-1603 连接成功后自动读取硬件配置
watch(
  statusForDraft,
  (status) => {
    if (status !== 'Connected' || !editorOpen.value) return
    if (draft.value.type === 'DSA3217') {
      void loadDsa3217Config()
    } else if (draft.value.type === 'DAQ-P-1603') {
      void loadDaqP1603Config()
    }
  }
)

async function runScan() {
  scanning.value = true
  scanError.value = null
  try {
    const results = await deviceApi.scanDevices()
    discovered.value = results
    if (results.length) feedback.pushToast(i18n.t.dev_devicesDiscovered.replace('{count}', String(results.length)), 'info')
    else feedback.pushToast(i18n.t.dev_noNewDevices, 'info')
  } catch (err) {
    discovered.value = []
    scanError.value = err instanceof Error ? err.message : String(err)
    feedback.pushToast(i18n.t.dev_scanFailedMsg.replace('{msg}', scanError.value), 'error')
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

// 预计算发现设备与已注册 profile 的匹配映射，避免模板中 O(n×m) 重复查找。
// discovered 列表变化时自动重算；单个发现设备绑定后展示已匹配名称与操作按钮均复用此映射。
const discoveredProfileMap = computed<Map<ScanResult, DeviceProfile | null>>(() => {
  const map = new Map<ScanResult, DeviceProfile | null>()
  for (const d of discovered.value) {
    const matched = deviceStore.profiles.find((p) => p.type === d.type && p.address === d.address && p.port === d.port)
    map.set(d, matched ?? null)
  }
  return map
})

function matchedProfileForDiscovered(d: ScanResult): DeviceProfile | null {
  // 回退到预计算映射（调用方未传 computed 结果的场景亦可直接查 map）
  return discoveredProfileMap.value.get(d) ?? null
}

function discoveryActionLabel(d: ScanResult): string {
  return matchedProfileForDiscovered(d) ? i18n.t.dev_edit : i18n.t.dev_add
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
  if (addingAllDiscovered.value) return
  bulkError.value = null
  const addable = discovered.value.filter((d) => !matchedProfileForDiscovered(d))
  if (!addable.length) { feedback.pushToast(i18n.t.dev_noDevicesToAdd, 'info'); return }
  const existingNames = new Set(deviceStore.profiles.map((p) => p.name.trim()).filter((n) => n))
  addingAllDiscovered.value = true
  try {
    const addedProfiles: DeviceProfile[] = []
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
        addedProfiles.push(profile)
      } catch { /* 跳过失败的 */ }
    }
    // 批量添加后自动连接
    if (autoConnectAfterBulkAdd.value) {
      for (const profile of addedProfiles) {
        try { await deviceStore.connect(profile.id) } catch { /* 跳过连接失败的 */ }
      }
    }
    await deviceStore.refreshProfiles()
    const toastKey = autoConnectAfterBulkAdd.value ? 'dev_devicesAddedAndConnected' : 'dev_devicesAdded'
    feedback.pushToast(i18n.t[toastKey].replace('{count}', String(addedProfiles.length)), 'success')
    clearDiscovered()
  } catch (e) {
    bulkError.value = e instanceof Error ? e.message : String(e)
  } finally {
    addingAllDiscovered.value = false
  }
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
    if (acquiring) await deviceStore.stopAcquisition(p.id)
    else await deviceStore.startAcquisition(p.id)
    await deviceStore.refreshStatusFor(p.id)
  } catch (e) { feedback.pushToast(String(e), 'error') }
}

async function removeProfile(p: DeviceProfile) {
  const ok = await feedback.confirm(i18n.t.dev_confirmDeleteDevice)
  if (!ok) return
  try {
    await deviceApi.disconnect(p.id).catch(() => {})
    await deviceApi.stopAcquisition(p.id).catch(() => {})
    await deviceApi.deleteProfile(p.id)
    await deviceStore.refreshProfiles()
    feedback.pushToast(i18n.t.dev_deviceDeleted, 'info')
  } catch (e) { feedback.pushToast(String(e), 'error') }
}

async function bulkConnect() {
  for (const id of selectedIds.value) {
    // 走 store.connect 以触发乐观更新，让卡片立刻显示"连接中"
    try { await deviceStore.connect(id) } catch { /* 跳过 */ }
  }
  clearSelection()
  feedback.pushToast(i18n.t.dev_bulkConnectDone, 'info')
}

async function bulkDisconnect() {
  // 批量断开必须经过 store.disconnect 才能：
  //   1) 退订该设备的数据流订阅；
  //   2) 显式把本地连接状态置为 Disconnected，让卡片立刻刷新。
  // 直接调用 deviceApi.disconnect 会绕过这两步，导致 UI 看似"点了没反应"。
  for (const id of selectedIds.value) {
    try { await deviceStore.disconnect(id) } catch { /* 跳过 */ }
  }
  clearSelection()
  feedback.pushToast(i18n.t.dev_bulkDisconnectDone, 'info')
}

async function bulkDelete() {
  const ok = await feedback.confirm(i18n.t.dev_confirmBulkDelete.replace('{count}', String(selectedIds.value.length)))
  if (!ok) return
  for (const id of selectedIds.value) {
    try {
      await deviceApi.disconnect(id).catch(() => {})
      await deviceApi.stopAcquisition(id).catch(() => {})
      await deviceApi.deleteProfile(id)
    } catch { /* 跳过 */ }
  }
  // refreshProfiles 失败时仍提示删除完成（设备已从后端删除，仅 UI 列表刷新失败），
  // 显式 catch 避免 unhandled rejection。
  try {
    await deviceStore.refreshProfiles()
  } catch (e) {
    console.warn('[DeviceManagementDrawer] refreshProfiles after bulk delete failed:', e)
  }
  clearSelection()
  feedback.pushToast(i18n.t.dev_bulkDeleteDone, 'info')
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
  if (deviceStore.acquiringFor(p.id)) return i18n.t.acquiring
  const s = deviceStore.statusFor(p.id)
  if (s === 'Connected') return i18n.t.connectedState
  if (s === 'Connecting') return i18n.t.connectingState
  if (s === 'Error') return i18n.t.error
  return i18n.t.disconnectedState
}

function connectLabel(p: DeviceProfile) {
  const acquiring = deviceStore.acquiringFor(p.id)
  const st = deviceStore.statusFor(p.id)
  // 连接中：明确显示"连接中..."并配合按钮 disabled 防止重复点击
  if (st === 'Connecting') return i18n.t.dev_connectingDots
  if (acquiring || st === 'Connected') return i18n.t.disconnectBtn
  return i18n.t.connectBtn
}

function channelLabel(c: ChannelConfig): string {
  return `${c.index + 1}`
}

// ---- 设备按连接状态分组 ----
// 单一真源：把每个 profile 映射到唯一的连接分组键（已连接 / 连接中 / 等待连接）
// connecting 与 error 都不属于"已连接"，但 connecting 是过渡态、error 是终态错误。
// 模板渲染各分组时统一使用同一份 deviceCardClass 等工具函数。
type ConnectionGroup = 'connected' | 'connecting' | 'pending'

function connectionGroupOf(profile: DeviceProfile): ConnectionGroup {
  if (deviceStore.acquiringFor(profile.id) || deviceStore.statusFor(profile.id) === 'Connected') {
    return 'connected'
  }
  if (deviceStore.statusFor(profile.id) === 'Connecting') {
    return 'connecting'
  }
  // Idle / Disconnected / Error 等均归入 "等待连接" 组
  return 'pending'
}

const connectedProfiles = computed(() =>
  deviceStore.profiles.filter((p) => connectionGroupOf(p) === 'connected')
)

const connectingProfiles = computed(() =>
  deviceStore.profiles.filter((p) => connectionGroupOf(p) === 'connecting')
)

const pendingProfiles = computed(() =>
  deviceStore.profiles.filter((p) => connectionGroupOf(p) === 'pending')
)

// ---- 扫描结果元数据 ----
function discoveryMetadataEntries(d: ScanResult): Array<{ label: string; value: string }> {
  return [
    { label: 'MAC', value: d.macAddress ?? '' },
    { label: 'SN', value: d.serialNumber ?? '' },
    { label: 'FW', value: d.firmwareVersion ?? '' },
    { label: 'Model', value: d.model ?? '' },
  ].filter((entry) => entry.value)
}

// ---- 批量添加自动连接 ----
const addingAllDiscovered = ref(false)
const autoConnectAfterBulkAdd = ref(false)
const bulkError = ref<string | null>(null)
const scanError = ref<string | null>(null)
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="drawer-mask" @click.self="close">
      <div class="drawer-shell">
        <header class="drawer-header">
          <div>
            <h2 class="drawer-title">{{ i18n.t.dev_deviceManagement }}</h2>
            <p class="drawer-subtitle">{{ i18n.t.dev_deviceManagementSubtitle }}</p>
          </div>
          <UiButton quaternary size="md" @click="close">
            <template #icon><X :size="14" /></template>
          </UiButton>
        </header>

        <div class="drawer-toolbar">
          <UiButton variant="primary" size="md" @click="openCreate()">
            <template #icon><Plus :size="14" /></template>
            {{ i18n.t.dev_newDevice }}
          </UiButton>
          <UiButton secondary size="md" :loading="scanning" :disabled="scanning" @click="runScan">
            <template #icon><RotateCcw :size="14" /></template>
            {{ scanning ? i18n.t.dev_scanning : i18n.t.dev_scan }}
          </UiButton>
          <div class="drawer-total">
            {{ i18n.t.totalDevices }}: {{ deviceStore.profiles.length }}
          </div>
        </div>

        <!-- 扫描错误提示 -->
        <div v-if="scanError" class="drawer-banner drawer-banner--error">
          <AlertCircle :size="14" />
          {{ i18n.t.dev_scanFailed }}: {{ scanError }}
        </div>

        <!-- 发现的设备区域：可折叠，减少认知负荷 -->
        <div v-if="discovered.length" class="drawer-discovered">
          <div class="drawer-discovered-head" @click="showDiscovered = !showDiscovered" style="cursor: pointer;">
            <div style="display: flex; align-items: center; gap: var(--space-2);">
              <span class="drawer-discovered-dot" aria-hidden="true"></span>
              <span class="drawer-discovered-label">{{ i18n.t.dev_discoveredDevices }}</span>
              <span class="discovered-count">{{ discovered.length }}</span>
            </div>
            <div class="drawer-discovered-actions">
              <UiButton variant="primary" size="sm" :disabled="addingAllDiscovered" @click.stop="addAllDiscoveredDevices">
                <span v-if="addingAllDiscovered" class="inline-spinner" aria-hidden="true"></span>
                {{ addingAllDiscovered ? i18n.t.dev_adding : i18n.t.dev_addAll }}
              </UiButton>
              <UiButton quaternary size="sm" @click.stop="clearDiscovered">
                <template #icon><X :size="14" /></template>
              </UiButton>
              <ChevronDown :size="14" class="discovered-toggle" :class="{ 'discovered-toggle--open': showDiscovered }" />
            </div>
          </div>
          <!-- 批量添加自动连接复选框 -->
          <div class="drawer-discovered-extra">
            <UiCheckbox v-model:checked="autoConnectAfterBulkAdd">{{ i18n.t.dev_autoConnectAfterAdd }}</UiCheckbox>
          </div>
          <!-- 批量添加错误提示 -->
          <div v-if="bulkError" class="drawer-discovered-error">
            <AlertCircle :size="12" />
            {{ bulkError }}
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
                  <!-- 元数据标签：使用 discoveryMetadataEntries 函数 -->
                  <div v-if="discoveryMetadataEntries(d).length" class="discovered-card-meta">
                    <span v-for="entry in discoveryMetadataEntries(d)" :key="entry.label" class="discovered-meta-badge">{{ entry.label }}: {{ entry.value }}</span>
                  </div>
                  <div v-if="discoveredProfileMap.get(d)" class="discovered-matched">
                    {{ i18n.t.dev_matched }}: {{ discoveredProfileMap.get(d)?.name }}
                  </div>
                </div>
                <UiButton size="sm" @click="handleDiscoveredDeviceAction(d)">
                  {{ discoveryActionLabel(d) }}
                </UiButton>
              </div>
            </div>
          </Transition>
        </div>

        <!-- 设备列表：按连接状态分组（已连接 / 连接中 / 等待连接 + 错误） -->
        <main class="drawer-list">
          <div v-if="!deviceStore.profiles.length" class="drawer-empty">
            {{ i18n.t.dev_noDeviceConfigHint }}
          </div>

          <!-- 已连接组 -->
          <template v-if="connectedProfiles.length">
            <div class="device-group-label">{{ i18n.t.connectedState }} · {{ connectedProfiles.length }}</div>
            <DeviceCard
              v-for="p in connectedProfiles"
              :key="p.id"
              :profile="p"
              group="connected"
              :status="deviceStore.statusFor(p.id)"
              :acquiring="deviceStore.acquiringFor(p.id)"
              :selected="isSelected(p.id)"
              :status-class="statusClass(p)"
              :connect-label="connectLabel(p)"
              @toggle-selected="toggleSelected(p.id)"
              @edit="openEdit(p)"
              @connect-toggle="connectToggle(p)"
              @toggle-acquisition="toggleAcquisition(p)"
              @remove="removeProfile(p)"
            />
          </template>

          <!-- 连接中组 -->
          <template v-if="connectingProfiles.length">
            <div class="device-group-label" :class="connectedProfiles.length ? 'device-group-label--spaced' : ''">
              {{ i18n.t.connectingState }} · {{ connectingProfiles.length }}
            </div>
            <DeviceCard
              v-for="p in connectingProfiles"
              :key="p.id"
              :profile="p"
              group="connecting"
              :status="deviceStore.statusFor(p.id)"
              :acquiring="deviceStore.acquiringFor(p.id)"
              :selected="isSelected(p.id)"
              :status-class="statusClass(p)"
              :connect-label="connectLabel(p)"
              @toggle-selected="toggleSelected(p.id)"
              @edit="openEdit(p)"
              @connect-toggle="connectToggle(p)"
              @toggle-acquisition="toggleAcquisition(p)"
              @remove="removeProfile(p)"
            />
          </template>

          <!-- 等待连接组（含 Error 状态） -->
          <template v-if="pendingProfiles.length">
            <div class="device-group-label"
              :class="(connectedProfiles.length || connectingProfiles.length) ? 'device-group-label--spaced' : ''">
              {{ i18n.t.dev_pendingConnection }} · {{ pendingProfiles.length }}
            </div>
            <DeviceCard
              v-for="p in pendingProfiles"
              :key="p.id"
              :profile="p"
              group="pending"
              :status="deviceStore.statusFor(p.id)"
              :acquiring="deviceStore.acquiringFor(p.id)"
              :selected="isSelected(p.id)"
              :status-class="statusClass(p)"
              :connect-label="connectLabel(p)"
              @toggle-selected="toggleSelected(p.id)"
              @edit="openEdit(p)"
              @connect-toggle="connectToggle(p)"
              @toggle-acquisition="toggleAcquisition(p)"
              @remove="removeProfile(p)"
            />
          </template>
        </main>

        <!-- 批量操作栏 -->
        <div v-if="selectedIds.length" class="drawer-bulk">
          <span>{{ i18n.t.selectedCount }} <strong>{{ selectedCount }}</strong></span>
          <div class="drawer-bulk-actions">
            <UiButton variant="primary" size="sm" :disabled="!selectedCount" @click="bulkConnect">{{ i18n.t.dev_bulkConnect }}</UiButton>
            <UiButton secondary size="sm" :disabled="!selectedCount" @click="bulkDisconnect">{{ i18n.t.dev_bulkDisconnect }}</UiButton>
            <UiButton variant="danger" size="sm" :disabled="!selectedCount" @click="bulkDelete">{{ i18n.t.dev_bulkDelete }}</UiButton>
            <UiButton quaternary size="sm" @click="clearSelection">{{ i18n.t.dev_clear }}</UiButton>
          </div>
        </div>
      </div>

      <!-- 编辑器模态框 -->
      <div v-if="editorOpen" class="editor-mask" @click.self="tryCloseEditor">
        <div class="editor-modal">
          <header class="editor-header">
            <div class="editor-header-left">
              <div class="editor-header-icon">
                <!-- 编辑器头部图标随模式切换：只读(锁) / 新建(+) / 编辑(笔)，统一用 lucide 图标避免 emoji 渲染差异 -->
                <Lock v-if="isReadOnly" :size="16" />
                <Plus v-else-if="editorMode === 'create'" :size="16" />
                <Pencil v-else :size="16" />
              </div>
              <div>
                <h3 class="editor-title">{{ editorMode === 'create' ? i18n.t.dev_newDevice : isReadOnly ? i18n.t.dev_viewDeviceReadOnly : i18n.t.dev_editDevice }}</h3>
                <div class="editor-status-row">
                  <!-- statusClass/statusLabel 只用到 id 字段，直接传 draft（DeviceProfile）即可，
                       无需构造假 profile；draft 是真实编辑对象，语义更清晰 -->
                  <span class="editor-status-dot" :class="statusClass(draft)" />
                  <span class="editor-status-text">{{ statusLabel(draft) }}</span>
                </div>
              </div>
            </div>
            <UiButton quaternary size="md" @click="tryCloseEditor">
              <template #icon><X :size="14" /></template>
            </UiButton>
          </header>

          <!-- 标签页切换：复用全局 .settings-tab 类（与 GlobalSettingsModal 同款分段控制器） -->
          <div class="editor-tabs">
            <div class="settings-tabs">
              <button
                class="settings-tab"
                :class="{ 'settings-tab--active': editorTab === 'basic' }"
                @click="editorTab = 'basic'"
              >
                {{ i18n.t.dev_basicInfo }}
              </button>
              <button
                class="settings-tab"
                :class="{ 'settings-tab--active': editorTab === 'channels' }"
                @click="editorTab = 'channels'"
              >
                {{ i18n.t.dev_channelConfig }}
              </button>
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
                <span>{{ i18n.t.dev_saveFailed }}: {{ saveError }}</span>
              </div>
            </Transition>

            <!-- 只读模式提示横幅 -->
            <Transition name="banner">
              <div v-if="isReadOnly" class="editor-readonly-banner">
                <svg class="banner-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                  <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
                </svg>
                <span>{{ i18n.t.dev_deviceAcquiringReadOnly }}</span>
              </div>
            </Transition>

            <!-- 基本信息 -->
            <div v-if="editorTab === 'basic'" class="editor-sections">
              <section class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">{{ i18n.t.dev_deviceIdentity }}</h4>
                  <p class="editor-section-desc">{{ i18n.t.dev_deviceIdentityDesc }}</p>
                </div>
                <div class="editor-grid">
                  <div class="editor-field col-6">
                    <label class="editor-label">{{ i18n.t.dev_deviceName }} *</label>
                    <UiInput v-model="draft.name" :disabled="isReadOnly" :placeholder="i18n.t.dev_enterDeviceName" />
                    <div v-if="fieldErrors.name" class="editor-field-error">● {{ fieldErrors.name }}</div>
                  </div>
                  <div class="editor-field col-4">
                    <label class="editor-label">{{ i18n.t.dev_deviceModel }}</label>
                    <UiSelect
                      :model-value="draft.type"
                      :options="deviceTypeOptions"
                      :disabled="editorMode === 'edit' || isReadOnly"
                      @update:model-value="onTypeChanged($event as DeviceType)"
                    />
                  </div>
                  <div v-if="draft.type !== 'DSA3217'" class="editor-field col-2">
                    <label class="editor-label">{{ i18n.t.dev_samplingRateHz }}</label>
                    <UiInputNumber v-model="draft.samplingRate" class="w-full" :disabled="isReadOnly" />
                    <div v-if="fieldErrors.samplingRate" class="editor-field-error">● {{ fieldErrors.samplingRate }}</div>
                  </div>
                  <div class="editor-field col-12">
                    <label class="editor-label">{{ i18n.t.dev_deviceUnitGlobal }}</label>
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
                          {{ isWtnPxiType(draft.type) ? i18n.t.dev_wtnPxiFixedConfig : i18n.t.dev_daqT1603FixedUnit }}
                        </div>
                      </div>
                      <p class="editor-unit-hint">{{ i18n.t.dev_deviceUnitHint }}</p>
                    </div>
                  </div>
                </div>
              </section>

              <!-- 大气数据开关（SIMULATED / DAQ-P-1604） -->
              <section v-if="draft.type === 'DAQ-P-1604' || draft.type === 'SIMULATED'" class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">{{ i18n.t.dev_atmosphericData }}</h4>
                  <p class="editor-section-desc">{{ i18n.t.dev_atmosphericDataDesc }}</p>
                </div>
                <div class="editor-atmo-row">
                  <!-- 大气数据开关：用 UiToggle 替代自实现 track/thumb，
                       toggleAtmosphericData 在 update:modelValue 时同步通道 16/17 的 enabled 与 unit -->
                  <div class="editor-atmo-toggle">
                    <UiToggle
                      :model-value="enableAtmospheric"
                      :disabled="isReadOnly"
                      @update:model-value="toggleAtmosphericData"
                    />
                    <span class="editor-atmo-label">{{ enableAtmospheric ? i18n.t.dev_atmosphericEnabled : i18n.t.dev_atmosphericDisabled }}</span>
                  </div>
                </div>
              </section>

              <!-- DAQ-P-1604 硬件时间戳开关 -->
              <section v-if="draft.type === 'DAQ-P-1604'" class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">{{ i18n.t.dev_hardwareTimestamp }}</h4>
                  <p class="editor-section-desc">{{ i18n.t.dev_hardwareTimestampDesc }}</p>
                </div>
                <div class="editor-atmo-row">
                  <div class="editor-atmo-toggle">
                    <UiToggle
                      :model-value="!!draft.daqP1604UseDeviceTimestamp"
                      :disabled="isReadOnly"
                      @update:model-value="draft.daqP1604UseDeviceTimestamp = $event"
                    />
                    <span class="editor-atmo-label">
                      {{ draft.daqP1604UseDeviceTimestamp ? i18n.t.dev_useDeviceTimestamp : i18n.t.dev_useHostTimestamp }}
                    </span>
                  </div>
                </div>
              </section>

              <!-- DSA3217 扫描参数（在已连接设备的基本信息中显示） -->
              <section v-if="draft.type === 'DSA3217' && statusForDraft === 'Connected'" class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">{{ i18n.t.dev_dsa3217ScanParams }}</h4>
                  <p class="editor-section-desc">{{ i18n.t.dev_dsa3217ScanParamsDesc }}</p>
                </div>
                <div class="editor-grid">
                  <div class="editor-field col-4">
                    <label class="editor-label">{{ i18n.t.dev_dsa3217Avg }}</label>
                    <UiInputNumber
                      v-model="dsa3217Avg"
                      :min="1" :max="240"
                      :disabled="isReadOnly"
                      class="w-full"
                    />
                  </div>
                  <div class="editor-field col-4">
                    <label class="editor-label">{{ i18n.t.dev_dsa3217Period }}</label>
                    <UiInputNumber
                      v-model="dsa3217Period"
                      :min="73" :max="65535"
                      :disabled="isReadOnly"
                      class="w-full"
                    />
                  </div>
                  <div class="editor-field col-4">
                    <label class="editor-label">{{ i18n.t.dev_dsa3217Fps }}</label>
                    <div class="editor-input editor-input-readonly">
                      {{ dsa3217Fps }}
                    </div>
                  </div>
                </div>
              </section>

              <!-- 通信协议 -->
              <section class="editor-section">
                <div class="editor-section-head">
                  <h4 class="editor-section-title">{{ i18n.t.dev_transportProtocol }}</h4>
                  <p class="editor-section-desc">{{ i18n.t.dev_transportProtocolDesc }}</p>
                </div>
                <div class="editor-grid">
                  <div v-if="isTcpType(draft.type) && supportsTransportSwitch(draft.type)" class="editor-field col-4">
                    <label class="editor-label">{{ i18n.t.dev_transportMode }}</label>
                    <UiSelect
                      :model-value="draft.transport ?? 'tcp'"
                      :options="transportOptions"
                      :disabled="isReadOnly"
                      @update:model-value="draft.transport = $event as 'tcp' | 'serial'"
                    />
                  </div>

                  <template v-if="isTcpType(draft.type) && draft.transport === 'tcp'">
                    <div :class="['editor-field', ipFieldColClass]">
                      <label class="editor-label">{{ i18n.t.dev_ipAddress }} *</label>
                      <UiInput v-model="draft.address" :disabled="isReadOnly" placeholder="192.168.1.100" />
                      <div v-if="fieldErrors.address" class="editor-field-error">● {{ fieldErrors.address }}</div>
                    </div>
                    <div v-if="isPortRequired(draft.type)" class="editor-field col-3">
                      <label class="editor-label">{{ i18n.t.dev_port }} *</label>
                      <UiInputNumber v-model="draft.port" class="w-full" :disabled="isReadOnly" />
                      <div v-if="fieldErrors.port" class="editor-field-error">● {{ fieldErrors.port }}</div>
                    </div>
                    <div v-if="draft.type === 'DAQ-P-1604'" class="editor-field col-12">
                      <label class="editor-label">{{ i18n.t.dev_localAddress }}</label>
                      <UiInput v-model="draft.localAddress" :disabled="isReadOnly" placeholder="192.168.3.10" />
                      <div class="editor-field-hint">{{ i18n.t.dev_localAddressHint }}</div>
                    </div>
                  </template>

                  <template v-if="isTcpType(draft.type) && draft.transport === 'serial'">
                    <div class="editor-field col-7">
                      <label class="editor-label">{{ i18n.t.dev_serialPort }} *</label>
                      <UiInput v-model="draft.serialPort" :disabled="isReadOnly" placeholder="COM1" />
                      <div v-if="fieldErrors.serialPort" class="editor-field-error">● {{ fieldErrors.serialPort }}</div>
                    </div>
                    <div class="editor-field col-5">
                      <label class="editor-label">{{ i18n.t.dev_baudRate }} *</label>
                      <UiInputNumber v-model="draft.baudRate" class="w-full" :disabled="isReadOnly" />
                      <div v-if="fieldErrors.baudRate" class="editor-field-error">● {{ fieldErrors.baudRate }}</div>
                    </div>
                  </template>

                  <div class="editor-field col-12">
                    <div class="editor-autoconnect-row">
                      <UiCheckbox v-model:checked="draft.autoConnect" :disabled="isReadOnly">
                        {{ i18n.t.dev_autoConnectHint }}
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
                        <th class="w-12">#</th>
                        <th>{{ i18n.t.dev_channelName }}</th>
                        <th class="w-24">{{ i18n.t.dev_thermocoupleType }}</th>
                        <th class="w-16 text-right">{{ i18n.t.unit }}</th>
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

              <!-- DAQ-T-1602 专用配置：仅 16 通道热电偶类型，展示逻辑在面板组件内 -->
              <div v-else-if="draft.type === 'DAQ-T-1602' && draft.daqT1602Config" class="editor-channels-special">
                <DaqT1602Config
                  v-model:type-codes="draft.daqT1602Config.typeCodes"
                  v-model:sample-rate="draft.daqT1602Config.sampleRateHz"
                  :disabled="isReadOnly"
                />
              </div>

              <!-- DAQ-P-1603 专用配置：16 通道传感器类型/单位/量程独立配置 -->
              <div v-else-if="draft.type === 'DAQ-P-1603'" class="editor-channels-special">
                <DaqP1603Config
                  v-model:channels="draft.channels"
                  v-model:sampling-rate="draft.samplingRate"
                  v-model:range-min="deviceRangeMin"
                  v-model:range-max="deviceRangeMax"
                  v-model:precision="devicePrecision"
                  :disabled="isReadOnly"
                />
              </div>

              <!-- WTN_PXI 固定通道 -->
              <div v-else-if="draft.type === 'WTN_PXI'" class="editor-channels-special">
                <p class="editor-channels-hint">{{ i18n.t.dev_wtnPxiChannelsFixed }}</p>
                <div class="editor-channels-table-wrap">
                  <table class="editor-channels-table">
                    <thead>
                      <tr>
                        <th class="w-12">#</th>
                        <th>{{ i18n.t.dev_channelName }}</th>
                        <th class="w-16 text-right">{{ i18n.t.unit }}</th>
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
                    <UiButton secondary size="sm" :disabled="isReadOnly" @click="setAllChannels(true)">{{ i18n.t.dev_enableAll }}</UiButton>
                    <UiButton secondary size="sm" :disabled="isReadOnly" @click="setAllChannels(false)">{{ i18n.t.dev_disableAll }}</UiButton>
                    <UiButton secondary size="sm" :disabled="isReadOnly" @click="resetChannelsToDefault">{{ i18n.t.dev_reset }}</UiButton>
                  </div>
                </div>

                <!-- 批量同步：紧凑单行 -->
                <div class="editor-ch-batch">
                  <span class="editor-ch-batch-label">{{ i18n.t.dev_batchApplyTo }}</span>
                  <div class="editor-ch-batch-field">
                    <span class="editor-ch-batch-field-label">{{ i18n.t.dev_range }}</span>
                    <UiInputNumber
                      v-model="deviceRangeMin"
                      class="editor-ch-batch-num"
                      :disabled="isReadOnly"
                      :placeholder="i18n.t.dev_minPlaceholder"
                    />
                    <span class="editor-ch-batch-sep">~</span>
                    <UiInputNumber
                      v-model="deviceRangeMax"
                      class="editor-ch-batch-num"
                      :disabled="isReadOnly"
                      :placeholder="i18n.t.dev_maxPlaceholder"
                    />
                  </div>
                  <div class="editor-ch-batch-field">
                    <span class="editor-ch-batch-field-label">{{ i18n.t.channelPrecision }}</span>
                    <UiInputNumber
                      v-model="devicePrecision"
                      class="editor-ch-batch-num editor-ch-batch-num--narrow"
                      :min="0"
                      :disabled="isReadOnly"
                      placeholder="0"
                    />
                    <span class="editor-ch-batch-field-suffix">{{ i18n.t.dev_decimalPlaces }}</span>
                  </div>
                </div>

                <!-- 通道列表 -->
                <div class="editor-channels-table-wrap">
                  <table class="editor-channels-table">
                    <thead>
                      <tr>
                        <th class="w-12">{{ i18n.t.channelEnabled }}</th>
                        <th class="w-12">#</th>
                        <th>{{ i18n.t.dev_channelName }}</th>
                        <th class="w-56 text-center">{{ i18n.t.dev_engineeringRange }}</th>
                        <th class="w-18 text-right">{{ i18n.t.channelPrecision }}</th>
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
                {{ i18n.t.dev_readOnlyMode }}
              </div>
              <!-- 校验错误状态指示 -->
              <div v-else-if="validationErrorCount > 0" class="editor-footer-errors">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <circle cx="12" cy="12" r="10"/>
                  <line x1="12" y1="8" x2="12" y2="12"/>
                  <line x1="12" y1="16" x2="12.01" y2="16"/>
                </svg>
                {{ i18n.t.dev_validationFailed }}: {{ validationErrorCount }} {{ i18n.t.dev_errorsCount }}
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
                {{ isDirty ? i18n.t.dev_unsavedChanges : i18n.t.dev_configSynced }}
              </div>
            </div>
            <div class="editor-footer-right">
              <UiButton secondary @click="tryCloseEditor">{{ isReadOnly ? i18n.t.close : i18n.t.cancel }}</UiButton>
              <UiButton
                v-if="!isReadOnly"
                variant="primary"
                size="md"
                :disabled="saving || validationErrorCount > 0"
                @click="saveDraft"
              >
                <span v-if="saving" class="btn-spinner" />
                {{ saving ? i18n.t.saving : i18n.t.save }}
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
  /* 遮罩用 bg-app + 透明度，避免硬编码 rgba 让深/浅主题自适应 */
  background: color-mix(in srgb, var(--bg-app) 60%, transparent);
  backdrop-filter: blur(4px);
  display: flex; justify-content: flex-end;
}

.drawer-shell {
  width: 760px; max-width: 95vw; height: 100vh;
  display: flex; flex-direction: column;
  background: var(--bg-panel);
  border-left: 1px solid var(--border-default);
  box-shadow: var(--shadow-overlay-md);
}

.drawer-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-5) var(--space-6);
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  flex-shrink: 0;
}

.drawer-title { margin: 0; font-size: var(--font-size-lg); font-weight: var(--font-weight-black); color: var(--text-primary); letter-spacing: -0.02em; }
.drawer-subtitle { margin: var(--space-1) 0 0; font-size: var(--font-size-xs); color: var(--text-muted); font-weight: var(--font-weight-semibold); }

.drawer-toolbar {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-4) var(--space-6);
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

/* 总数徽章：与 GlobalSettingsModal 的容量预览徽章同款 pill 样式 */
.drawer-total {
  margin-left: auto;
  padding: var(--space-1-5) var(--space-3); border-radius: var(--radius-pill);
  /* slate-500 ≈ text-muted，用 token + 透明度避免硬编码 rgba */
  background: color-mix(in srgb, var(--text-muted) 10%, transparent);
  font-size: var(--font-size-micro); font-weight: var(--font-weight-black); letter-spacing: 0.1em;
  color: var(--text-muted);
}

.drawer-discovered {
  border-bottom: 1px solid var(--border-default);
  padding: var(--space-4) var(--space-6);
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
  flex-shrink: 0;
}
.drawer-discovered-head { display: flex; align-items: center; justify-content: space-between; padding: var(--space-2) 0; transition: opacity 0.2s; }
.drawer-discovered-head:hover { opacity: 0.8; }
/* 中文标签不 uppercase，与全局设置风格对齐 */
.drawer-discovered-label { font-size: var(--font-size-micro); font-weight: var(--font-weight-black); letter-spacing: 0.15em; color: var(--text-muted); }
.drawer-discovered-actions { display: flex; align-items: center; gap: var(--space-2); }
.discovered-count { padding: var(--space-0-5) var(--space-2); border-radius: var(--radius-pill); background: color-mix(in srgb, var(--accent-primary) 15%, transparent); color: var(--accent-primary); font-size: var(--font-size-micro); font-weight: var(--font-weight-black); }
/* ChevronDown 图标组件继承 currentColor，旋转由 --open 修饰类控制 */
.discovered-toggle { color: var(--text-muted); transition: transform 0.2s; display: inline-block; }
.discovered-toggle--open { transform: rotate(180deg); }
.discovered-expand-enter-active, .discovered-expand-leave-active { transition: all 0.25s ease; }
.discovered-expand-enter-from, .discovered-expand-leave-to { opacity: 0; max-height: 0; overflow: hidden; }
.discovered-expand-enter-to, .discovered-expand-leave-from { opacity: 1; max-height: 500px; }
.drawer-discovered-list { display: flex; flex-direction: column; gap: var(--space-2); max-height: 30vh; overflow-y: auto; }
.discovered-card {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-3); border-radius: var(--radius-xl);
  background: var(--bg-panel); border: 1px solid var(--border-default);
}
.discovered-card-icon {
  width: var(--space-8); height: var(--space-8); display: flex; align-items: center; justify-content: center;
  border-radius: 50%;
  /* 蓝色背景统一用 accent-primary + color-mix，深/浅主题一致 */
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent); color: var(--accent-primary);
  font-size: var(--font-size-xs); font-weight: var(--font-weight-black); flex-shrink: 0;
}
.discovered-card-name { font-size: var(--font-size-sm); font-weight: var(--font-weight-bold); color: var(--text-primary); }
.discovered-card-type { font-size: var(--font-size-2xs); font-weight: var(--font-weight-semibold); color: var(--text-muted); margin-top: var(--space-0-5); }
.discovered-card-addr { color: var(--text-muted); opacity: 0.7; }
.discovered-card-meta { display: flex; flex-wrap: wrap; gap: var(--space-1); margin-top: var(--space-1); }
.discovered-meta-badge {
  display: inline-flex; align-items: center;
  padding: var(--space-0-5) var(--space-1-5); border-radius: var(--radius-md);
  font-size: var(--font-size-micro); font-weight: var(--font-weight-semibold);
  /* --bg-secondary 是未定义 token，回落到 --bg-panel-strong 保证可读 */
  background: var(--bg-panel-strong); color: var(--text-tertiary);
  border: 1px solid var(--border-default);
}
.discovered-matched {
  margin-top: var(--space-1); display: inline-flex; align-items: center;
  padding: var(--space-0-5) var(--space-2); border-radius: var(--radius-pill);
  /* 绿色徽章用 accent-success + color-mix，与 status-online 同源 */
  background: color-mix(in srgb, var(--accent-success) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-success) 30%, transparent);
  font-size: var(--font-size-micro); font-weight: var(--font-weight-bold); color: var(--accent-success);
}

.drawer-list { flex: 1; overflow-y: auto; padding: var(--space-4) var(--space-6); display: flex; flex-direction: column; gap: var(--space-3); }
.drawer-empty { padding: var(--space-8) var(--space-4); text-align: center; color: var(--text-muted); font-size: var(--font-size-sm); }

.drawer-bulk {
  flex-shrink: 0; display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-3) var(--space-6); border-top: 1px solid var(--border-default);
  background: var(--bg-panel-strong); font-size: var(--font-size-xs); color: var(--text-secondary);
}
.drawer-bulk-actions {
  display: flex; align-items: center; gap: var(--space-2); margin-left: auto;
}

/* 通用横幅：作为扫描错误、加载错误等顶部提示条 */
.drawer-banner {
  display: flex; align-items: center; gap: var(--space-2);
  padding: var(--space-2) var(--space-6);
  border-bottom: 1px solid var(--border-default);
  font-size: var(--font-size-xs); font-weight: var(--font-weight-bold);
}
.drawer-banner--error {
  background: color-mix(in srgb, var(--accent-danger) 10%, transparent);
  color: var(--accent-danger);
}

/* 设备分组标签：中文不需要 uppercase，letter-spacing 收紧避免中文断字 */
.device-group-label {
  font-size: var(--font-size-micro);
  font-weight: var(--font-weight-black);
  letter-spacing: 0.2em;
  color: var(--text-muted);
  padding: 0 var(--space-1);
  margin-bottom: var(--space-2);
}
.device-group-label--spaced { margin-top: var(--space-4); }

/* 发现的设备：蓝色心跳点，告知用户当前有未处理扫描结果 */
.drawer-discovered-dot {
  display: inline-block;
  width: 6px; height: 6px;
  border-radius: 50%;
  background: var(--accent-primary);
  animation: drawer-discovered-ping 1.6s ease-in-out infinite;
}
@keyframes drawer-discovered-ping {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.4; transform: scale(1.4); }
}
.drawer-discovered-extra {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-1-5) var(--space-1);
}
.drawer-discovered-error {
  display: flex; align-items: center; gap: var(--space-1-5);
  padding: var(--space-1-5) var(--space-1);
  font-size: var(--font-size-xs); font-weight: var(--font-weight-bold);
  color: var(--accent-danger);
}

/* 通用 inline spinner：用 currentColor 描边，自动跟随按钮文字色 */
.inline-spinner {
  display: inline-block;
  width: 12px; height: 12px;
  margin-right: var(--space-1);
  border-radius: 50%;
  border: 2px solid currentColor;
  border-top-color: transparent;
  animation: inline-spinner-spin 0.8s linear infinite;
  vertical-align: -2px;
  flex-shrink: 0;
}
/* Editor Modal */
.editor-mask {
  position: fixed; inset: 0; z-index: 110;
  /* 编辑器遮罩透明度比 drawer 更高（70%），同样用 bg-app 自适应主题 */
  background: color-mix(in srgb, var(--bg-app) 70%, transparent);
  display: flex; align-items: center; justify-content: center;
  padding: var(--space-4);
}

.editor-modal {
  width: 860px; max-width: 98vw; max-height: 92vh;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  /* token 化：raw 1rem → var(--radius-xl)（0.5rem），
     与 discovered-card / drawer-card 等卡片层级半径统一 */
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-overlay-lg);
  display: flex; flex-direction: column;
  overflow: hidden;
}

.editor-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-5) var(--space-6);
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  flex-shrink: 0;
}
.editor-header-left { display: flex; align-items: center; gap: var(--space-3); }
.editor-header-icon {
  width: var(--space-10); height: var(--space-10); display: flex; align-items: center; justify-content: center;
  /* token 化：raw 0.75rem → var(--radius-lg)（0.375rem），
     图标容器半径收紧，与 discovered-card-icon 同源（accent-primary 10%） */
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent); color: var(--accent-primary);
  font-size: var(--font-size-xl); font-weight: var(--font-weight-black);
}
.editor-title { margin: 0; font-size: var(--font-size-lg); font-weight: var(--font-weight-black); color: var(--text-primary); }
.editor-status-row { display: flex; align-items: center; gap: var(--space-2); margin-top: var(--space-1); }
.editor-status-dot {
  width: var(--space-1); height: var(--space-1); border-radius: 50%; background: var(--text-muted);
}
/* 状态点光晕用 accent-success + color-mix 替代硬编码 rgba；
   pulse 改为全局已定义的 pulse-opacity（原 pulse 未定义会导致动画失效） */
.editor-status-dot.status-online { background: var(--accent-success); box-shadow: 0 0 var(--space-1) color-mix(in srgb, var(--accent-success) 50%, transparent); }
.editor-status-dot.status-acq { background: var(--accent-success); box-shadow: 0 0 var(--space-1) color-mix(in srgb, var(--accent-success) 60%, transparent); animation: pulse-opacity 1.5s infinite; }
.editor-status-dot.status-connecting { background: var(--accent-warning); animation: pulse-opacity 0.8s infinite; }
.editor-status-text { font-size: var(--font-size-xs); font-weight: var(--font-weight-semibold); color: var(--text-muted); }

.editor-tabs {
  flex-shrink: 0; padding: var(--space-4) var(--space-6);
  border-bottom: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
}
/* 标签容器：横向分段控制器，与 GlobalSettingsModal 的 .settings-tabs 同款语义。
   注：GlobalSettingsModal 的 .settings-tabs 是垂直布局（侧栏导航），
   此处覆盖为横向 inline-flex 适配编辑器顶部 tab 切换场景。 */
.settings-tabs {
  /* 顶部编辑 tab 为横向分段控制器，视觉规范来自 settings-form.css */
  display: inline-flex;
  flex-direction: row;
  gap: var(--space-0-5);
  padding: var(--space-0-5);
  border-radius: var(--radius-lg);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
}

.editor-body {
  /* 紧凑密度：编辑器正文内边距收紧到 12px 16px */
  padding: var(--space-3) var(--space-4);
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

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
  display: flex; align-items: center; gap: var(--space-2);
  /* 紧凑密度：错误横幅 padding 收紧 */
  padding: var(--space-2) var(--space-3); border-radius: var(--radius-md);
  /* token 化：raw rgba 改用 color-mix + accent-danger，深浅两套主题自动适配 */
  background: color-mix(in srgb, var(--accent-danger) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 20%, transparent);
  font-size: var(--font-size-xs); font-weight: var(--font-weight-bold); color: var(--accent-danger);
  margin-bottom: var(--density-section-gap);
}
.editor-readonly-banner {
  display: flex; align-items: center; gap: var(--space-2);
  padding: var(--space-2) var(--space-3); border-radius: var(--radius-md);
  /* token 化：raw rgba 改用 color-mix + accent-warning，深浅两套主题自动适配 */
  background: color-mix(in srgb, var(--accent-warning) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-warning) 20%, transparent);
  font-size: var(--font-size-xs); font-weight: var(--font-weight-bold); color: var(--accent-warning);
  margin-bottom: var(--density-section-gap);
}
.banner-icon {
  flex-shrink: 0;
  opacity: 0.9;
}

/* 紧凑密度：区块间 12px（原 32px），让 860px modal 多塞字段 */
.editor-sections { display: flex; flex-direction: column; gap: var(--density-section-gap); }
.editor-section { }
/* 紧凑密度：section 标题与正文 4px */
.editor-section-head { margin-bottom: var(--density-group-title-gap); }
.editor-section-title {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-bold);
  letter-spacing: 0.02em;
  color: var(--text-primary);
  /* 中文标题不 uppercase */
  text-transform: none;
  margin: 0;
}
.editor-section-desc { font-size: var(--font-size-2xs); font-weight: var(--font-weight-semibold); color: var(--text-muted); margin-top: var(--space-0-5); }

/* 紧凑密度：字段间 8px（原 16px） */
.editor-grid { display: grid; grid-template-columns: repeat(12, 1fr); gap: var(--density-field-gap); }
.editor-field { margin-bottom: 0; }
.col-2 { grid-column: span 2; }
.col-3 { grid-column: span 3; }
.col-4 { grid-column: span 4; }
.col-5 { grid-column: span 5; }
.col-6 { grid-column: span 6; }
.col-7 { grid-column: span 7; }
.col-9 { grid-column: span 9; }
.col-12 { grid-column: span 12; }

/* 紧凑密度：label 与控件 2px，中文标签不 uppercase */
.editor-label {
  display: block;
  margin-bottom: var(--density-field-inline);
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-muted);
  letter-spacing: 0.02em;
}
.editor-input {
  width: 100%;
  /* 紧凑控件：高度 28px，横向内边距 8px */
  height: var(--density-control-height);
  padding: 0 var(--density-control-pad-x);
  border-radius: var(--radius-sm); border: 1px solid var(--border-default);
  /* token 化：raw rgba(0,0,0,0.2) 改用 --bg-panel-strong，深浅主题由 token 自动适配 */
  background: var(--bg-panel-strong); color: var(--text-primary);
  font-family: var(--font-family-sans); font-size: var(--font-size-sm); font-weight: var(--font-weight-bold);
  outline: none; transition: all 0.2s;
}
.editor-input:focus { border-color: var(--accent-primary); background: var(--bg-panel-strong); }
.editor-input:disabled { opacity: 0.6; cursor: not-allowed; }
.editor-input-readonly {
  display: flex; align-items: center;
  height: var(--density-control-height); font-weight: var(--font-weight-bold); color: var(--text-muted);
}
/* 浅色主题：输入框背景反白，token 化 raw rgba(255,255,255,0.9) → --surface-1 */
:root[data-theme='light'] .editor-input { background: var(--bg-panel); border-color: var(--border-strong); color: var(--text-primary); }
:root[data-theme='light'] .editor-input:focus { background: var(--bg-panel-strong); border-color: var(--accent-primary); }
.editor-field-error { margin-top: var(--density-field-inline); font-size: var(--font-size-2xs); font-weight: var(--font-weight-bold); color: var(--accent-danger); }

.editor-unit-row { display: flex; align-items: center; gap: var(--space-3); }
.editor-unit-select { flex: 1; }
.editor-unit-hint { font-size: var(--font-size-2xs); font-weight: var(--font-weight-bold); color: var(--text-muted); max-width: 200px; line-height: var(--line-height-base); }

/* 紧凑密度：大气数据行 padding 收紧 */
.editor-atmo-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-2) var(--space-3); border-radius: var(--radius-sm);
  border: 1px solid var(--border-default); background: var(--bg-panel);
}
/* atmo-toggle 仅作容器：UiToggle 已封装 NSwitch，无需自实现 track/thumb */
.editor-atmo-toggle { display: flex; align-items: center; gap: var(--space-2); cursor: pointer; }
.editor-atmo-label { font-size: var(--font-size-xs); font-weight: var(--font-weight-bold); color: var(--text-primary); }

/* 紧凑密度：自动连接行 padding 收紧 */
.editor-autoconnect-row {
  display: flex; align-items: center; gap: var(--space-2);
  padding: var(--space-2) var(--space-3); border-radius: var(--radius-sm);
  /* token 化：raw rgba(59,130,246,0.05) 改用 color-mix + accent-primary，深浅主题自动适配 */
  background: color-mix(in srgb, var(--accent-primary) 5%, transparent);
}

/* Channels — 紧凑密度：分组间 10px */
.editor-channels-special { display: flex; flex-direction: column; gap: var(--density-group-gap); }
.editor-channels-hint { font-size: var(--font-size-xs); color: var(--text-muted); margin: 0; }
.editor-channels-full { display: flex; flex-direction: column; gap: var(--density-field-gap); }

.editor-channels-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--density-field-gap); }
.editor-channels-toolbar-left { display: flex; gap: var(--space-1-5); }

/* 批量同步 - 紧凑单行：padding 收紧到 4px 8px */
.editor-ch-batch {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--bg-panel-strong) 30%, transparent);
  border: 1px solid var(--border-default);
  flex-wrap: wrap;
}
.editor-ch-batch-label {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-bold);
  color: var(--text-muted);
  white-space: nowrap;
}
.editor-ch-batch-field {
  display: flex; align-items: center; gap: var(--space-1-5);
}
.editor-ch-batch-field-label {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-secondary);
  white-space: nowrap;
}
.editor-ch-batch-field-suffix {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  white-space: nowrap;
}
.editor-ch-batch-num { width: 96px; }
.editor-ch-batch-num--narrow { width: 64px; }
.editor-ch-batch-sep {
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-bold);
  color: var(--text-muted);
  flex-shrink: 0;
}

/* Channel table */
.editor-channels-table-wrap {
  border-radius: var(--radius-xl);
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
  /* 紧凑密度：表头 padding 收紧 */
  padding: var(--space-1) var(--space-2);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-bold);
  letter-spacing: 0;
  text-transform: none;
  color: var(--text-secondary);
  border-bottom: 1px solid var(--border-default);
  white-space: nowrap;
}
.editor-channels-table td {
  /* 紧凑密度：单元格 padding 收紧到 2px 6px */
  padding: var(--space-0-5) var(--space-1-5);
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
  padding-left: var(--space-2);
  padding-right: var(--space-2);
}

/* 量程单元格容器：两个 UiInputNumber + "~" 分隔符水平排列
   — 旧的 .editor-ch-input / .editor-ch-range-input / .editor-ch-tc /
   .editor-ch-precision-input / .editor-ch-check 已删除（通道表格全部改用
   UiInput / UiInputNumber / UiSelect / UiCheckbox 封装，原生 raw CSS 样式作废） */
.editor-ch-range {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-1-5);
}

.font-mono { font-family: ui-monospace, monospace; }
.text-center { text-align: center; }
.text-right { text-align: right; }
.text-muted { color: var(--text-muted); }

.editor-footer {
  display: flex; align-items: center; justify-content: space-between;
  /* token 化：raw 1rem 1.5rem → space-4 space-6 */
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  flex-shrink: 0;
}
.editor-footer-left {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
}
.editor-footer-right { display: flex; gap: var(--space-3); }
/* footer 文案统一不 uppercase：原 uppercase 是英文 small-caps 风格遗留，
   中文标签不需要字距收紧 + 大写转换，删除以保持与全局风格基线一致 */
.editor-footer-readonly {
  display: flex; align-items: center; gap: var(--space-1-5);
  font-size: var(--font-size-2xs); font-weight: var(--font-weight-black); color: var(--accent-warning);
  letter-spacing: 0.05em;
}
.editor-footer-errors {
  display: flex; align-items: center; gap: var(--space-1-5);
  font-size: var(--font-size-2xs); font-weight: var(--font-weight-black); color: var(--accent-danger);
  letter-spacing: 0.05em;
}
.editor-footer-status {
  display: flex; align-items: center; gap: var(--space-1-5);
  font-size: var(--font-size-2xs); font-weight: var(--font-weight-black); color: var(--text-muted);
  letter-spacing: 0.1em;
  transition: color 0.2s ease;
}
.editor-footer-status.dirty { color: var(--accent-warning); }

/* 保存按钮加载动画：btn-saving 类已废弃（UiButton :loading 内部已渲染 spinner），
   保留 .btn-spinner 给模板 line 1576 <span class="btn-spinner" /> 使用 */
.btn-spinner {
  position: absolute;
  left: var(--space-2-5);
  top: 50%;
  transform: translateY(-50%);
  width: var(--space-3);
  height: var(--space-3);
  /* token 化：raw rgba(255,255,255,0.3) 改用 color-mix 让浅色主题也可读 */
  border: 2px solid color-mix(in srgb, var(--text-primary) 30%, transparent);
  border-top-color: var(--text-primary);
  border-radius: 50%;
  animation: btn-spin 0.8s linear infinite;
}
.w-full { width: 100%; }
/* 通道表格列宽定义
   - w-12/w-16/w-24/w-28 是 Tailwind 标准类，JIT 会自动生成，
     这里显式定义是为了在 scoped 作用域内提供确定性，避免依赖 Tailwind 配置。
   - w-18/w-56 不是 Tailwind 标准刻度，必须显式定义才能生效。
   设计依据：见 §29 设备管理编辑画面列宽规范 */
.w-12 { width: 48px; }   /* 启用复选框、# 序号列 */
.w-16 { width: 64px; }   /* 单位列（℃ 等单字符单位） */
.w-18 { width: 72px; }   /* 精度列（3 位数字） */
.w-24 { width: 96px; }   /* 热电偶类型下拉列 */
.w-28 { width: 112px; }  /* 设备型号/扩展列（容纳 "DAQ-P-1604Pre" 等长型号） */
.w-56 { width: 224px; }  /* 工程量程列（两个 w-full 输入框 + "~" 分隔符 + 单元格 padding） */
.editor-ch-select-min-width { min-width: 80px; }
@keyframes btn-spin {
  to { transform: translateY(-50%) rotate(360deg); }
}
</style>
