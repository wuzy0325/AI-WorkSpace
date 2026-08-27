<template>
  <el-drawer
    v-model="drawerVisible"
    title="计量采集设备"
    direction="rtl"
    :size="drawerSize"
    append-to-body
    :destroy-on-close="false"
    @open="handleOpen"
  >
    <div class="drawer-layout">
      <header class="drawer-summary">
        <div>
          <strong>已选 {{ selectedDeviceIds.length }} 台</strong>
          <span :class="['consistency', unitConsistent ? 'ok' : 'warning']">
            {{ unitConsistent ? '单位一致' : '单位不一致' }}
          </span>
        </div>
        <div class="connection-actions">
          <el-button
            type="primary"
            :loading="connecting"
            :disabled="pendingConnectIds.length === 0"
            @click="emit('connect', [...selectedDeviceIds])"
          >
            连接选中设备
          </el-button>
          <el-button
            type="danger"
            plain
            :disabled="connectedDeviceIds.length === 0"
            @click="emit('disconnect', [...connectedDeviceIds])"
          >
            断开
          </el-button>
        </div>
      </header>

      <!-- 阀门区块：逐台回读状态，行点击切换"当前设备"；
           写命令由后端下发到全部已绑定设备（多设备批次阀门必须整批一致）。 -->
      <section
        class="drawer-section"
        aria-labelledby="valve-section-title"
      >
        <div class="section-heading">
          <div>
            <h3 id="valve-section-title">
              阀门
            </h3>
            <span>{{ valveSummary }}</span>
          </div>
          <el-button
            text
            :loading="refreshing"
            @click="refreshSettings"
          >
            刷新
          </el-button>
        </div>
        <div class="device-state-list">
          <button
            v-for="device in boundDevices"
            :key="device.id"
            type="button"
            :class="['device-state-row', { active: device.id === focusedDeviceId }]"
            :aria-pressed="device.id === focusedDeviceId"
            @click="focusedDeviceId = device.id"
          >
            <span>{{ device.name || device.model || device.id }}</span>
            <span :class="['state-value', { error: valveError(device.id) || isValveDifferent(device.id) }]">
              {{ valveDisplay(device.id) }}{{ isValveDifferent(device.id) ? '（不一致）' : '' }}
            </span>
          </button>
        </div>
        <div class="section-actions">
          <el-button
            type="primary"
            :loading="valvePending"
            :disabled="boundDevices.length === 0"
            @click="setValveAll('calibration')"
          >
            校准模式
          </el-button>
          <el-button
            :loading="valvePending"
            :disabled="boundDevices.length === 0"
            @click="setValveAll('measurement')"
          >
            测量模式
          </el-button>
        </div>
      </section>

      <!-- 压力单位区块：逐台展示回读单位，差异设备标红；
           "统一并应用"走 /session/measure-unit/all 整批写入。 -->
      <section
        class="drawer-section"
        aria-labelledby="unit-section-title"
      >
        <div class="section-heading">
          <div>
            <h3 id="unit-section-title">
              压力单位
            </h3>
            <span>{{ unitSummary }}</span>
          </div>
        </div>
        <div class="device-state-list">
          <div
            v-for="device in boundDevices"
            :key="device.id"
            class="device-state-row static"
          >
            <span>{{ device.name || device.model || device.id }}</span>
            <span :class="['state-value', { error: unitError(device.id) || isUnitDifferent(device.id) }]">
              {{ unitDisplay(device.id) }}{{ isUnitDifferent(device.id) ? '（不一致）' : '' }}
            </span>
          </div>
        </div>
        <div class="unit-actions">
          <el-select
            v-model="targetUnit"
            aria-label="统一压力单位"
          >
            <el-option
              v-for="unit in unitOptions"
              :key="unit"
              :label="unit"
              :value="unit"
            />
          </el-select>
          <el-button
            type="primary"
            :loading="unitPending"
            :disabled="boundDevices.length === 0 || !targetUnit"
            @click="setUnitAll"
          >
            统一并应用
          </el-button>
        </div>
      </section>

      <!-- 当前设备操作：无阀型号（P1603）提供软件校零，其余提供复位。 -->
      <section
        v-if="focusedDevice"
        class="drawer-section"
        aria-labelledby="device-action-title"
      >
        <div class="section-heading">
          <div>
            <h3 id="device-action-title">
              当前设备
            </h3>
            <span>{{ focusedDevice.name || focusedDevice.model || focusedDevice.id }}</span>
          </div>
        </div>
        <div class="section-actions">
          <el-button
            v-if="isFocusedValveless"
            type="primary"
            :loading="zeroPending"
            @click="calibrateFocusedDevice"
          >
            校零
          </el-button>
          <el-button
            v-else
            :loading="resetPending"
            @click="resetFocusedDevice"
          >
            复位
          </el-button>
        </div>
      </section>

      <!-- 设备选择：连接后锁定勾选，断开后可重新配置；与计量模块抽屉交互一致。 -->
      <section
        class="drawer-section selection-section"
        aria-labelledby="selection-section-title"
      >
        <div class="section-heading">
          <div>
            <h3 id="selection-section-title">
              设备选择
            </h3>
            <span>勾选需要连接的设备，可随时调整</span>
          </div>
        </div>
        <div class="selection-toolbar">
          <el-button
            text
            size="small"
            :disabled="inventoryStore.measureDevices.length === 0"
            @click="selectAll"
          >
            全部选择
          </el-button>
          <el-button
            text
            size="small"
            :disabled="selectedDeviceIds.length === 0"
            @click="clearSelection"
          >
            取消选择
          </el-button>
        </div>
        <el-checkbox-group v-model="selectedDeviceIds">
          <el-checkbox
            v-for="device in inventoryStore.measureDevices"
            :key="device.id"
            :value="device.id"
          >
            <span class="device-choice-name">{{ device.name || device.model || device.id }}</span>
            <span :class="['connection-state', device.status]">{{ connectionLabel(device.status) }}</span>
            <el-button
              v-if="device.status === 'connected'"
              text
              size="small"
              class="row-disconnect"
              @click.stop="disconnectOne(device.id)"
            >
              断开
            </el-button>
          </el-checkbox>
        </el-checkbox-group>
        <el-empty
          v-if="inventoryStore.measureDevices.length === 0"
          description="暂无计量采集设备"
          :image-size="56"
        />
      </section>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  calibrateZero,
  readMeasureUnitAll,
  readValveStatusAll,
  resetDevice,
  setMeasureUnitAll,
  type DeviceValueResult
} from '@/api/session'
import { fetchDevices } from '@/api/device'
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import { useCalibrationStore } from '@/stores/calibration'
import { useDeviceStore } from '@/stores/deviceStore'
import { normalizeValveStatus, valveStatusLabel, type ValveState } from '@/types/valve'
import { enabledChannelIndexes, isValvelessModel } from '@/utils/deviceModels'

/**
 * 标定模块计量采集设备抽屉。
 * 与计量模块 MeasurementDeviceDrawer 保持同一交互形态：
 * 摘要（已选台数 + 单位一致性）、逐台阀门/单位状态、当前设备校零/复位、多选设备列表。
 *
 * 与计量模块的差异：标定 store 只保留单值会话状态（valveStatus/measureUnit），
 * 因此逐台状态在组件内用本地 Map 承载，数据源为会话级逐台回读接口——
 * 后端会话接口与模块无关，标定绑定后同样按绑定集合逐台读写。
 */
const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  'update:modelValue': [visible: boolean]
  connect: [deviceIds: string[]]
  disconnect: [deviceIds: string[]]
  'unit-change': []
}>()

const inventoryStore = useDeviceInventoryStore()
const calibrationStore = useCalibrationStore()
const moduleDeviceStore = useDeviceStore()

const selectedDeviceIds = ref<string[]>([])
const valveByDevice = ref<Record<string, DeviceValueResult>>({})
const unitByDevice = ref<Record<string, DeviceValueResult>>({})
const focusedDeviceId = ref('')
const targetUnit = ref('kPa')
const refreshing = ref(false)
const valvePending = ref(false)
const unitPending = ref(false)
const zeroPending = ref(false)
const resetPending = ref(false)
const unitOptions = ['Pa', 'kPa', 'MPa', 'bar', 'mbar', 'psi', 'kgf/cm2', 'mmHg', 'atm']

const drawerVisible = computed({
  get: () => props.modelValue,
  set: visible => emit('update:modelValue', visible)
})
const drawerSize = computed(() => window.innerWidth <= 600 ? '100%' : '440px')

// 勾选中且已处于 connected 状态的设备集合：即本次会话实际绑定的设备。
const connectedDeviceIds = computed(() => selectedDeviceIds.value.filter(id =>
  inventoryStore.measureDevices.some(device => device.id === id && device.status === 'connected')
))
const hasConnectedDevices = computed(() => connectedDeviceIds.value.length > 0)
// 本次可发起连接的设备：勾选中且尚未处于已连接状态的设备。
// 连接后仍允许调整勾选，新增设备只会补连未连接的，已连接设备不会被重复连接。
const pendingConnectIds = computed(() => selectedDeviceIds.value.filter(id =>
  inventoryStore.measureDevices.some(device => device.id === id && device.status !== 'connected')
))
const connecting = computed(() => selectedDeviceIds.value.some(id =>
  inventoryStore.measureDevices.some(device => device.id === id && device.status === 'connecting')
))
const boundDevices = computed(() => connectedDeviceIds.value
  .map(id => inventoryStore.measureDevices.find(device => device.id === id))
  .filter((device): device is NonNullable<typeof device> => Boolean(device)))
const focusedDevice = computed(() =>
  boundDevices.value.find(device => device.id === focusedDeviceId.value)
)
const isFocusedValveless = computed(() => isValvelessModel(focusedDevice.value?.model))

// 单位一致性：所有绑定设备均读到单位且（忽略大小写）完全一致。
const unitValues = computed(() => boundDevices.value
  .map(device => unitByDevice.value[device.id]?.value)
  .filter((unit): unit is string => Boolean(unit)))
const unitConsistent = computed(() =>
  boundDevices.value.length > 0 && unitValues.value.length === boundDevices.value.length &&
  new Set(unitValues.value.map(unit => unit.toLowerCase())).size <= 1
)

// 阀门一致性：全部读到有效状态且取值唯一；读取失败视为不一致。
const valveValues = computed(() => boundDevices.value.map(device =>
  normalizeValveStatus(valveByDevice.value[device.id]?.value || '')
))
const valveConsistent = computed(() =>
  boundDevices.value.length > 0 &&
  valveValues.value.every(value => value !== '' && value !== 'unknown') &&
  new Set(valveValues.value).size <= 1
)
const valveSummary = computed(() => valveConsistent.value ? '全部设备状态一致' : '设备状态不一致或读取失败')
const unitSummary = computed(() => unitConsistent.value ? `全部设备 ${unitValues.value[0] || '--'}` : '设备单位不一致或读取失败')

// 设备列表变化时清理失效勾选；勾选为空时优先恢复上次成功绑定的设备，
// 设备不存在时回退默认勾选第一台（与旧 Device1604Panel 行为一致）。
watch(
  () => inventoryStore.measureDevices,
  devices => {
    const valid = selectedDeviceIds.value.filter(id => devices.some(device => device.id === id))
    if (valid.length > 0 || devices.length === 0) {
      selectedDeviceIds.value = valid
      return
    }
    const saved = moduleDeviceStore.selectionByModule('calibration').measureDeviceIds
    selectedDeviceIds.value = saved.filter(id => devices.some(device => device.id === id))
    if (selectedDeviceIds.value.length === 0) selectedDeviceIds.value = [devices[0].id]
  },
  { immediate: true, deep: true }
)

// 绑定集合变化时维护"当前设备"焦点：清空失效焦点并兜底到第一台。
watch(boundDevices, devices => {
  if (devices.some(device => device.id === focusedDeviceId.value)) return
  focusedDeviceId.value = devices[0]?.id ?? ''
}, { immediate: true })

function handleOpen() {
  if (hasConnectedDevices.value) void refreshSettings()
}

/** 逐台回读阀门与单位状态；单台失败保留在该设备的 error 字段中由 UI 标红。 */
async function refreshSettings() {
  refreshing.value = true
  try {
    const [valves, units] = await Promise.all([
      readValveStatusAll(),
      readMeasureUnitAll()
    ])
    valveByDevice.value = valves
    unitByDevice.value = units
    // 目标单位默认跟随当前一致值，减少统一单位时的重复选择。
    const firstUnit = unitValues.value[0]
    if (firstUnit) targetUnit.value = firstUnit
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '读取设备状态失败')
  } finally {
    refreshing.value = false
  }
}

/** 整批切换阀门：后端把写命令下发到全部绑定设备；store 同步更新单值状态供启动门禁判定。 */
async function setValveAll(status: ValveState) {
  if (status !== 'calibration' && status !== 'measurement') return
  valvePending.value = true
  try {
    const result = await calibrationStore.setValveStatus(status)
    if (!result.ok) throw new Error(result.detail || '阀门切换失败')
    ElMessage.success(status === 'calibration' ? '全部阀门已切换到校准模式' : '全部阀门已切换到测量模式')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '阀门切换失败')
  } finally {
    try { await refreshSettings() } catch { /* 保留写错误反馈 */ }
    valvePending.value = false
  }
}

/** 整批统一单位：走 /session/measure-unit/all；随后同步 store 单值并通知父级重查一致性。 */
async function setUnitAll() {
  unitPending.value = true
  try {
    await setMeasureUnitAll(targetUnit.value)
    emit('unit-change')
    ElMessage.success(`全部计量设备已统一为 ${targetUnit.value}`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '统一设备单位失败')
  } finally {
    try {
      await refreshSettings()
      await calibrationStore.refreshMeasureUnit()
    } catch { /* 保留写错误反馈 */ }
    unitPending.value = false
  }
}

/** 无阀型号（P1603）软件校零：按设备配置的启用通道执行，避免对未接线通道发命令。 */
async function calibrateFocusedDevice() {
  const deviceId = focusedDevice.value?.id
  if (!deviceId) return
  zeroPending.value = true
  try {
    const channels = enabledChannelIndexes(await fetchDevices(), deviceId)
    if (channels.length === 0) throw new Error('设备未配置启用通道，无法校零')
    await calibrateZero(channels, deviceId)
    ElMessage.success('校零完成')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '校零失败')
  } finally {
    zeroPending.value = false
  }
}

async function resetFocusedDevice() {
  const deviceId = focusedDevice.value?.id
  if (!deviceId) return
  resetPending.value = true
  try {
    await resetDevice(deviceId)
    ElMessage.success('设备复位指令已发送')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '设备复位失败')
  } finally {
    resetPending.value = false
  }
}

function valveError(deviceId: string): string { return valveByDevice.value[deviceId]?.error || '' }
function unitError(deviceId: string): string { return unitByDevice.value[deviceId]?.error || '' }

function valveDisplay(deviceId: string): string {
  const error = valveError(deviceId)
  if (error) return `读取失败：${error}`
  const raw = valveByDevice.value[deviceId]?.value || ''
  return raw ? valveStatusLabel(normalizeValveStatus(raw), raw) : '--'
}

function unitDisplay(deviceId: string): string {
  const error = unitError(deviceId)
  return error ? `读取失败：${error}` : unitByDevice.value[deviceId]?.value || '--'
}

function isUnitDifferent(deviceId: string): boolean {
  const baseline = unitValues.value[0]?.toLowerCase()
  const current = unitByDevice.value[deviceId]?.value?.toLowerCase()
  return Boolean(baseline && current && baseline !== current)
}

// 阀门差异判定与单位一致：设备间"校准 vs 测量"取值差异也要逐台标红。
// 读失败或未知状态的设备不参与差异判定——它已经以"读取失败"标红，不叠加误导。
function isValveDifferent(deviceId: string): boolean {
  if (valveError(deviceId)) return false
  const baseline = valveValues.value[0]
  if (!baseline || baseline === 'unknown') return false
  const current = normalizeValveStatus(valveByDevice.value[deviceId]?.value || '')
  return Boolean(current) && current !== 'unknown' && baseline !== current
}

function connectionLabel(status: string): string {
  if (status === 'connected') return '已连接'
  if (status === 'connecting') return '连接中'
  if (status === 'error') return '异常'
  return '未连接'
}

// 全部选择 / 取消选择：作用于设备清单全集，便于多设备场景快速勾选。
function selectAll() {
  selectedDeviceIds.value = inventoryStore.measureDevices.map(device => device.id)
}
function clearSelection() {
  selectedDeviceIds.value = []
}

// 单台断开：从勾选中移除并触发断开，仅断开该设备，不影响其他已连接设备。
function disconnectOne(deviceId: string) {
  selectedDeviceIds.value = selectedDeviceIds.value.filter(id => id !== deviceId)
  emit('disconnect', [deviceId])
}

// 暴露给父级：连接/断开完成后由侧栏主动触发回读，
// 避免"某台设备先 connected 就刷新"导致整批绑定尚未完成时读取报 binding token 500。
defineExpose({ refresh: refreshSettings })
</script>

<style scoped lang="scss">
/* Drawer 是连续操作面，不使用嵌套卡片；区块仅用分隔线组织高密度设备状态。
   样式与计量模块 MeasurementDeviceDrawer 保持一致。 */
.drawer-layout { display: flex; flex-direction: column; gap: 16px; }
.drawer-summary { display: flex; flex-direction: column; gap: 12px; padding-bottom: 16px; border-bottom: 1px solid $slate-200; }
.drawer-summary > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.drawer-summary strong { color: $slate-800; font-size: 15px; }
.consistency { font-size: 12px; font-weight: 600; }
.consistency.ok { color: $green; }
.consistency.warning { color: #d97706; }
.connection-actions { justify-content: flex-start !important; }
.drawer-section { padding: 4px 0 16px; border-bottom: 1px solid $slate-200; }
.section-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.section-heading h3 { margin: 0; color: $slate-800; font-size: 14px; letter-spacing: 0; }
.section-heading span { color: $slate-500; font-size: 12px; }
.device-state-list { display: flex; flex-direction: column; margin-bottom: 12px; }
.device-state-row { width: 100%; min-height: 36px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 6px 8px; border: 0; border-bottom: 1px solid $slate-100; background: transparent; color: $slate-700; text-align: left; cursor: pointer; }
.device-state-row.active { background: rgba(16, 185, 129, 0.08); }
.device-state-row.static { cursor: default; }
.state-value { color: $slate-600; font-family: $font-mono; font-size: 12px; text-align: right; overflow-wrap: anywhere; }
.state-value.error { color: $red; }
.section-actions, .unit-actions { display: flex; align-items: center; gap: 8px; }
.unit-actions :deep(.el-select) { flex: 1; }
.selection-section :deep(.el-checkbox-group) { display: flex; flex-direction: column; gap: 4px; }
.selection-toolbar { display: flex; align-items: center; justify-content: flex-end; gap: 4px; margin-bottom: 4px; }
.selection-section :deep(.el-checkbox) { width: 100%; min-height: 34px; margin-right: 0; }
.selection-section :deep(.el-checkbox__label) { display: flex; align-items: center; width: 100%; min-width: 0; }
.device-choice-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.connection-state { font-size: 11px; color: $slate-400; }
.connection-state.connected { color: $green; }
.connection-state.error { color: $red; }
.row-disconnect { margin-left: 4px; flex-shrink: 0; color: $slate-400; }
.row-disconnect:hover { color: $red; }
@media (max-width: 600px) {
  .connection-actions, .unit-actions { align-items: stretch !important; flex-direction: column; }
  .connection-actions :deep(.el-button), .unit-actions :deep(.el-button) { width: 100%; margin-left: 0; }
}
</style>
