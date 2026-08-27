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
            :class="['device-state-row', { active: device.id === measurementStore.activeDeviceId }]"
            :aria-pressed="device.id === measurementStore.activeDeviceId"
            @click="measurementStore.setActiveDevice(device.id)"
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
            :disabled="deviceStore.measureDevices.length === 0"
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
            v-for="device in deviceStore.measureDevices"
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
          v-if="deviceStore.measureDevices.length === 0"
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
import { fetchDevices } from '@/api/device'
import { useDeviceStore } from '@/stores/deviceStore'
import { useMeasurementStore } from '@/stores/measurement'
import { useMeasurementDeviceStore } from '@/stores/measurement/deviceStore'
import { normalizeValveStatus, valveStatusLabel, type ValveState } from '@/types/valve'
import { enabledChannelIndexes, isValvelessModel } from '@/utils/deviceModels'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{
  'update:modelValue': [visible: boolean]
  connect: [deviceIds: string[]]
  disconnect: [deviceIds: string[]]
  'unit-change': []
}>()

const measurementStore = useMeasurementStore()
const deviceStore = useMeasurementDeviceStore()
const moduleDeviceStore = useDeviceStore()
const selectedDeviceIds = ref<string[]>([])
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
const connectedDeviceIds = computed(() => selectedDeviceIds.value.filter(id =>
  deviceStore.measureDevices.some(device => device.id === id && device.status === 'connected')
))
// 本次可发起连接的设备：勾选中且尚未处于已连接状态的设备。
// 连接后仍允许调整勾选，新增设备只会补连未连接的，已连接设备不会被重复连接。
const pendingConnectIds = computed(() => selectedDeviceIds.value.filter(id =>
  deviceStore.measureDevices.some(device => device.id === id && device.status !== 'connected')
))
const connecting = computed(() => selectedDeviceIds.value.some(id =>
  deviceStore.measureDevices.some(device => device.id === id && device.status === 'connecting')
))
const boundDevices = computed(() => measurementStore.measureDeviceIds
  .map(id => deviceStore.measureDevices.find(device => device.id === id))
  .filter((device): device is NonNullable<typeof device> => Boolean(device)))
const focusedDevice = computed(() => boundDevices.value.find(device => device.id === measurementStore.activeDeviceId))
const isFocusedValveless = computed(() => isValvelessModel(focusedDevice.value?.model))
const unitValues = computed(() => boundDevices.value
  .map(device => measurementStore.measureUnitByDevice[device.id])
  .filter(Boolean))
const unitConsistent = computed(() =>
  boundDevices.value.length > 0 && unitValues.value.length === boundDevices.value.length &&
  Object.keys(measurementStore.measureUnitErrorByDevice).length === 0 &&
  new Set(unitValues.value.map(unit => unit.toLowerCase())).size <= 1
)
const valveValues = computed(() => boundDevices.value.map(device =>
  normalizeValveStatus(measurementStore.valveStatusByDevice[device.id])
))
const valveConsistent = computed(() =>
  boundDevices.value.length > 0 && valveValues.value.every(value => value !== '' && value !== 'unknown') &&
  Object.keys(measurementStore.valveErrorByDevice).length === 0 && new Set(valveValues.value).size <= 1
)
const valveSummary = computed(() => valveConsistent.value ? '全部设备状态一致' : '设备状态不一致或读取失败')
const unitSummary = computed(() => unitConsistent.value ? `全部设备 ${unitValues.value[0] || '--'}` : '设备单位不一致或读取失败')

watch(
  () => deviceStore.measureDevices,
  devices => {
    const valid = selectedDeviceIds.value.filter(id => devices.some(device => device.id === id))
    if (valid.length > 0 || devices.length === 0) {
      selectedDeviceIds.value = valid
      return
    }
    const saved = moduleDeviceStore.selectionByModule('measurement').measureDeviceIds
    selectedDeviceIds.value = saved.filter(id => devices.some(device => device.id === id))
    if (selectedDeviceIds.value.length === 0) selectedDeviceIds.value = [devices[0].id]
  },
  { immediate: true, deep: true }
)

watch(() => measurementStore.measureDeviceIds, ids => {
  if (ids.length > 0) selectedDeviceIds.value = [...ids]
}, { immediate: true })

async function handleOpen() {
  if (measurementStore.deviceBound) await refreshSettings()
}

async function refreshSettings() {
  refreshing.value = true
  try {
    await measurementStore.refreshDeviceSettingsAll()
    targetUnit.value = measurementStore.measureUnit || targetUnit.value
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '读取设备状态失败')
  } finally {
    refreshing.value = false
  }
}

async function setValveAll(status: ValveState) {
  if (status !== 'calibration' && status !== 'measurement') return
  valvePending.value = true
  try {
    const result = await measurementStore.setValveStatus(status)
    if (!result.ok) throw new Error(result.detail || '阀门切换失败')
    ElMessage.success(status === 'calibration' ? '全部阀门已切换到校准模式' : '全部阀门已切换到测量模式')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '阀门切换失败')
  } finally {
    try { await measurementStore.refreshValveStatusAll() } catch { /* 保留写错误反馈 */ }
    valvePending.value = false
  }
}

async function setUnitAll() {
  unitPending.value = true
  try {
    const result = await measurementStore.setMeasureUnitAll(targetUnit.value)
    if (!result.ok) throw new Error(result.detail || '统一设备单位失败')
    emit('unit-change')
    ElMessage.success(`全部计量设备已统一为 ${targetUnit.value}`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '统一设备单位失败')
  } finally {
    try {
      await measurementStore.refreshMeasureUnitAll()
      await measurementStore.refreshUnitConsistency()
    } catch { /* 保留写错误反馈 */ }
    unitPending.value = false
  }
}

async function calibrateFocusedDevice() {
  const deviceId = focusedDevice.value?.id
  if (!deviceId) return
  zeroPending.value = true
  try {
    const channels = enabledChannelIndexes(await fetchDevices(), deviceId)
    if (channels.length === 0) throw new Error('设备未配置启用通道，无法校零')
    await measurementStore.calibrateZero(deviceId, channels)
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
    await measurementStore.resetDevice(deviceId)
    ElMessage.success('设备复位指令已发送')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '设备复位失败')
  } finally {
    resetPending.value = false
  }
}

function valveError(deviceId: string): string { return measurementStore.valveErrorByDevice[deviceId] || '' }
function unitError(deviceId: string): string { return measurementStore.measureUnitErrorByDevice[deviceId] || '' }
function valveDisplay(deviceId: string): string {
  const error = valveError(deviceId)
  if (error) return `读取失败：${error}`
  const raw = measurementStore.valveStatusByDevice[deviceId] || ''
  return raw ? valveStatusLabel(normalizeValveStatus(raw), raw) : '--'
}
function unitDisplay(deviceId: string): string {
  const error = unitError(deviceId)
  return error ? `读取失败：${error}` : measurementStore.measureUnitByDevice[deviceId] || '--'
}
function isUnitDifferent(deviceId: string): boolean {
  const baseline = unitValues.value[0]?.toLowerCase()
  const current = measurementStore.measureUnitByDevice[deviceId]?.toLowerCase()
  return Boolean(baseline && current && baseline !== current)
}

// isValveDifferent 与 isUnitDifferent 对齐：设备间"校准 vs 测量"值差异也要逐台标红
// （验收项：连接后逐台回读阀门/单位，不一致时差异设备标红高亮）。
// 读失败或未知状态的设备不参与差异判定——它已经以"读取失败"标红，不叠加误导。
function isValveDifferent(deviceId: string): boolean {
  if (valveError(deviceId)) return false
  const baseline = valveValues.value[0]
  if (!baseline || baseline === 'unknown') return false
  const current = normalizeValveStatus(measurementStore.valveStatusByDevice[deviceId])
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
  selectedDeviceIds.value = deviceStore.measureDevices.map(device => device.id)
}
function clearSelection() {
  selectedDeviceIds.value = []
}

// 单台断开：从勾选中移除并触发断开，仅断开该设备，不影响其他已连接设备。
function disconnectOne(deviceId: string) {
  selectedDeviceIds.value = selectedDeviceIds.value.filter(id => id !== deviceId)
  emit('disconnect', [deviceId])
}
</script>

<style scoped lang="scss">
/* Drawer 是连续操作面，不使用嵌套卡片；区块仅用分隔线组织高密度设备状态。 */
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
