<template>
  <aside
    class="sidebar"
    :class="{ collapsed: collapsed }"
  >
    <div
      class="sidebar-toggle"
      role="button"
      tabindex="0"
      aria-label="切换侧边栏"
      @click="emit('toggle')"
      @keydown.enter="emit('toggle')"
      @keydown.space.prevent="emit('toggle')"
    >
      <el-icon>
        <ArrowRight v-if="collapsed" />
        <ArrowLeft v-else />
      </el-icon>
    </div>
    <div
      v-show="!collapsed"
      class="sidebar-content"
    >
      <div class="sidebar-section">
        <button
          type="button"
          class="device-drawer-trigger"
          @click="deviceDrawerVisible = true"
        >
          <el-icon><Monitor /></el-icon>
          <span class="trigger-label">计量采集设备</span>
          <span class="trigger-status">{{ measurementStore.measureDeviceIds.length }} 台已选</span>
          <el-icon><ArrowRight /></el-icon>
        </button>
        <MeasurementDeviceDrawer
          v-model="deviceDrawerVisible"
          @connect="handleMeasureDeviceConnect"
          @disconnect="handleMeasureDeviceDisconnect"
          @unit-change="handleMeasureUnitChange"
        />
      </div>
      <div class="sidebar-section">
        <h3 class="sidebar-title">
          <el-icon><FirstAidKit /></el-icon>
          打压设备
        </h3>
        <PressDevicePanel
          @connect="handlePressDeviceConnect"
          @disconnect="handlePressDeviceDisconnect"
          @set-pressure="handleSetPressure"
          @exhaust="handleExhaust"
          @unit-change="handlePressUnitChange"
        />
      </div>
      <div class="sidebar-section">
        <h3 class="sidebar-title">
          <el-icon><CircleCheckFilled /></el-icon>
          启动条件
        </h3>
        <div class="prerequisites-list">
          <div
            v-for="(item, index) in prerequisites"
            :key="index"
            class="prereq-item"
            :class="{ satisfied: item.satisfied, unsatisfied: !item.satisfied }"
          >
            <el-icon
              v-if="item.satisfied"
              class="icon-satisfied"
            >
              <CircleCheckFilled />
            </el-icon>
            <el-icon
              v-else
              class="icon-unsatisfied"
            >
              <CircleCloseFilled />
            </el-icon>
            <span class="prereq-label">{{ item.label }}</span>
            <span class="prereq-status">{{ item.satisfied ? '已满足' : '未满足' }}</span>
          </div>
        </div>
      </div>
    </div>
    <!-- 折叠态：垂直 icon-only 状态指示器列，点击任意 icon 展开侧栏 -->
    <div
      v-if="collapsed"
      class="sidebar-collapsed-icons"
    >
      <el-icon
        class="collapsed-icon"
        :class="{ 'is-connected': hasConnectedMeasureDevice }"
        role="button"
        tabindex="0"
        :aria-label="`计量采集设备：${hasConnectedMeasureDevice ? '已连接' : '未连接'}，点击展开`"
        @click="emit('toggle')"
        @keydown.enter="emit('toggle')"
      >
        <Monitor />
      </el-icon>
      <el-icon
        class="collapsed-icon"
        :class="{ 'is-connected': hasConnectedPressureDevice }"
        role="button"
        tabindex="0"
        :aria-label="`打压设备：${hasConnectedPressureDevice ? '已连接' : '未连接'}，点击展开`"
        @click="emit('toggle')"
        @keydown.enter="emit('toggle')"
      >
        <FirstAidKit />
      </el-icon>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import {
  ArrowLeft, ArrowRight, CircleCheckFilled, CircleCloseFilled,
  Monitor, FirstAidKit
} from '@element-plus/icons-vue'
import MeasurementDeviceDrawer from '@/components/measurement/MeasurementDeviceDrawer.vue'
import PressDevicePanel from '@/components/common/PressDevicePanel.vue'
import { readSessionUnitConsistency } from '@/api/session'
import { useMeasurementStore } from '@/stores/measurement'
import { useMeasurementDeviceStore } from '@/stores/measurement/deviceStore'
import { useDeviceStore } from '@/stores/deviceStore'

interface UnitCheckPayload {
  consistent: boolean
  conflicts: string[]
}

defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{
  toggle: []
  unitCheck: [payload: UnitCheckPayload]
}>()

const measurementStore = useMeasurementStore()
const deviceStore = useMeasurementDeviceStore()
const moduleDeviceStore = useDeviceStore()
const unitConsistent = ref(true)
const unitConflicts = ref<string[]>([])
const deviceDrawerVisible = ref(false)

const hasConnectedPressureDevice = computed(() =>
  deviceStore.pressureDevices.some(d => d.status === 'connected')
)

// 计量设备是否已连接（用于折叠态状态指示，与打压设备判断方式保持对称）
const hasConnectedMeasureDevice = computed(() =>
  deviceStore.measureDevices.some(d => d.status === 'connected')
)

const showUnitCheck = computed(() =>
  measurementStore.deviceBound && hasConnectedPressureDevice.value
)

const unitCheckSatisfied = computed(() =>
  showUnitCheck.value && unitConsistent.value
)

const emitUnitCheck = () => {
  emit('unitCheck', {
    consistent: unitConsistent.value,
    conflicts: [...unitConflicts.value]
  })
}

const checkUnitConsistency = async () => {
  try {
    const result = await readSessionUnitConsistency()
    unitConsistent.value = result.consistent
    unitConflicts.value = result.conflicts ?? []
    // 同步到计量 store，供"开始计量"门禁使用。
    measurementStore.unitConsistent = result.consistent
    emitUnitCheck()
  } catch {
    // 静默失败：保留当前 UI 状态，不打断流程。
  }
}

// 连接多台计量设备：逐台连接（跳过已连接设备，避免重复建链），
// 然后整批绑定到会话。设备集合为勾选全集，保证"已连接基础上追加设备"时
// 不会把旧设备从绑定中挤出。
const handleMeasureDeviceConnect = async (deviceIds: string[]) => {
  for (const deviceId of deviceIds) {
    const device = deviceStore.measureDevices.find(d => d.id === deviceId)
    if (!device || device.status === 'connected') continue
    const result = await deviceStore.connectMeasureDevice(deviceId)
    if (!result.ok) {
      return
    }
  }

  moduleDeviceStore.setModuleSelection('measurement', { measureDeviceIds: deviceIds })

  const connectedPressure = deviceStore.pressureDevices.find(d => d.status === 'connected')
  if (connectedPressure) {
    await measurementStore.bindDevices(deviceIds, connectedPressure.id)
  } else {
    unitConsistent.value = true
    unitConflicts.value = []
    emitUnitCheck()
    await measurementStore.bindMeasureDevice(deviceIds)
  }

  await Promise.all([
    measurementStore.refreshDeviceInfo(),
    measurementStore.refreshDeviceSettingsAll()
  ])

  // 批量回读已由后端逐台同步硬件实际单位，此处直接检查当前会话一致性。
  if (connectedPressure) {
    await checkUnitConsistency()
  }
}

const handleMeasureDeviceDisconnect = async (deviceIds: string[]) => {
  for (const deviceId of deviceIds) {
    const result = await deviceStore.disconnectMeasureDevice(deviceId)
    if (!result.ok) return
  }
  // 断开后从绑定列表移除已断开设备；若全部断开则整体解绑。
  const remaining = measurementStore.measureDeviceIds.filter(id => !deviceIds.includes(id))
  if (remaining.length === 0) {
    await measurementStore.clearMeasureDeviceBinding()
  } else {
    const connectedPressure = deviceStore.pressureDevices.find(d => d.status === 'connected')
    if (connectedPressure) {
      await measurementStore.bindDevices(remaining, connectedPressure.id)
    } else {
      await measurementStore.bindMeasureDevice(remaining)
    }
  }

  moduleDeviceStore.setModuleSelection('measurement', {
    measureDeviceIds: remaining,
    pressureDeviceId: measurementStore.pressureDeviceId
  })

  unitConsistent.value = true
  unitConflicts.value = []
  emitUnitCheck()
}

const handlePressDeviceConnect = async (deviceId: string) => {
  const ok = await deviceStore.connectPressureDevice(deviceId)
  if (!ok) {
    return
  }

  moduleDeviceStore.setModuleSelection('measurement', { pressureDeviceId: deviceId })

  if (measurementStore.measureDeviceIds.length > 0) {
    await measurementStore.bindDevices(measurementStore.measureDeviceIds, deviceId)
  }

  await checkUnitConsistency()
}

const handlePressDeviceDisconnect = async (deviceId: string) => {
  await deviceStore.disconnectPressureDevice(deviceId)
  if (measurementStore.pressureDeviceId === deviceId) {
    measurementStore.unbindPressureDevice()
  }

  moduleDeviceStore.setModuleSelection('measurement', {
    measureDeviceIds: measurementStore.measureDeviceIds,
    pressureDeviceId: ''
  })

  unitConsistent.value = true
  unitConflicts.value = []
  emitUnitCheck()
}

const handlePressUnitChange = async () => {
  await checkUnitConsistency()
}

const handleMeasureUnitChange = async () => {
  await checkUnitConsistency()
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
const handleSetPressure = async (_deviceId: string, _pressure: number) => {}
// eslint-disable-next-line @typescript-eslint/no-unused-vars
const handleExhaust = async (_deviceId: string) => {}

const prerequisites = computed(() => [
  { label: '设备已选择', satisfied: measurementStore.deviceBound },
  { label: '打压设备已连接', satisfied: hasConnectedPressureDevice.value },
  { label: '已选择采集通道', satisfied: measurementStore.channels.length > 0 },
  { label: '单位一致', satisfied: unitCheckSatisfied.value }
])

onMounted(() => {
  emitUnitCheck()
})

// 从标定画面切入时，自动绑定设备存储中已连接的计量设备（多设备整批绑定）
watch(
  () => deviceStore.measureDevices,
  async (devices) => {
    if (measurementStore.measureDeviceIds.length > 0) return
    const connectedDevices = devices.filter(d => d.status === 'connected')
    if (connectedDevices.length === 0) return

    try {
      const deviceIds = connectedDevices.map(d => d.id)
      moduleDeviceStore.setModuleSelection('measurement', { measureDeviceIds: deviceIds })

      const connectedPressure = deviceStore.pressureDevices.find(d => d.status === 'connected')
      if (connectedPressure) {
        await measurementStore.bindDevices(deviceIds, connectedPressure.id)
      } else {
        await measurementStore.bindMeasureDevice(deviceIds)
        unitConsistent.value = true
        unitConflicts.value = []
        emitUnitCheck()
      }

      await Promise.all([
        measurementStore.refreshDeviceInfo(),
        measurementStore.refreshDeviceSettingsAll()
      ])

      if (connectedPressure) {
        await checkUnitConsistency()
      }
    } catch (err) {
      console.warn('自动绑定计量设备失败:', err)
    }
  },
  { immediate: true }
)

defineExpose({ checkUnitConsistency })
</script>

<style scoped lang="scss">
.sidebar {
  width: 280px;
  background: #f6f7f6;
  border-right: 1px solid $slate-200;
  position: relative;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  &.collapsed { width: 48px; }
}
.sidebar-toggle {
  position: absolute;
  right: -12px;
  top: 50%;
  transform: translateY(-50%);
  width: 12px;
  height: 36px;
  background: #fff;
  border: 1px solid $slate-200;
  border-left: none;
  border-radius: 0 6px 6px 0;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  z-index: 10;
  .el-icon { color: $slate-400; font-size: 10px; }
  &:hover { background: $slate-50; .el-icon { color: $mint; } }
}
.sidebar-content {
  padding: 16px;
  overflow-y: auto;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 16px;

  &::-webkit-scrollbar { width: 4px; }
  &::-webkit-scrollbar-thumb { background: $slate-300; border-radius: 4px; }
  &::-webkit-scrollbar-track { background: transparent; }
}

/* 折叠态：垂直 icon-only 状态指示器列 */
.sidebar-collapsed-icons {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 12px 0;

  .collapsed-icon {
    width: 24px;
    height: 24px;
    font-size: 20px;
    color: $slate-300;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    transition: color 0.2s ease;
    border-radius: 6px;

    /* 已连接：图标点亮为绿色 */
    &.is-connected {
      color: $green;
    }

    &:hover {
      color: $mint;
    }

    &:focus-visible {
      outline: 2px solid $mint;
      outline-offset: 2px;
    }

    /* 状态点：右下角小圆点，强化连接状态指示 */
    &::after {
      content: '';
      position: absolute;
      right: -2px;
      bottom: -2px;
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: $slate-300;
      border: 1.5px solid #f6f7f6;
      transition: background 0.2s ease;
    }

    &.is-connected::after {
      background: $green;
    }
  }
}

/* Section 卡片：白色背景 + 圆角 + 阴影 + 边框 */
.sidebar-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: #ffffff;
  border-radius: 12px;
  border: 1px solid $slate-200;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  padding: 16px;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;

  &:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }
}

.device-drawer-trigger {
  width: 100%;
  min-height: 40px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid $slate-200;
  border-radius: 8px;
  background: #fff;
  color: $slate-700;
  cursor: pointer;
  font-family: $font-sans;
  transition: border-color 0.15s ease, background 0.15s ease;

  &:hover,
  &:focus-visible {
    border-color: $mint;
    background: rgba(16, 185, 129, 0.05);
    outline: none;
  }

  > .el-icon:first-child { color: $mint; font-size: 16px; }
  > .el-icon:last-child { color: $slate-400; font-size: 13px; }
  .trigger-label { flex: 1; min-width: 0; text-align: left; font-size: 13px; font-weight: 600; }
  .trigger-status { color: $slate-500; font-size: 11px; white-space: nowrap; }
}

/* Section 标题：左侧 Mint 竖线 + 文字 */
.sidebar-title {
  color: $slate-500;
  font-size: 12px;
  font-weight: 600;
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  font-family: $font-sans;
  position: relative;
  padding-left: 10px;

  &::before {
    content: '';
    position: absolute;
    left: 0;
    top: 50%;
    transform: translateY(-50%);
    width: 1px;
    height: 14px;
    background: $mint;
    border-radius: 0;
  }

  .el-icon { color: $mint; font-size: 14px; }
}

/* 启动条件 */
.prerequisites-list {
  display: flex;
  flex-direction: column;
  background: #ffffff;
  border-radius: 8px;
  overflow: hidden;

  .prereq-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 0;
    font-size: 13px;
    font-family: $font-sans;
    border-bottom: 1px solid $slate-100;

    &:last-child { border-bottom: none; }

    .el-icon { font-size: 16px; flex-shrink: 0; }
    .icon-satisfied { color: $green; }
    .icon-unsatisfied { color: $slate-300; }
    .prereq-label {
      flex: 1;
      color: $slate-600;
      font-weight: 500;
    }
    .prereq-status {
      font-size: 11px;
      padding: 2px 8px;
      border-radius: 4px;
      font-weight: 600;
      letter-spacing: 0.02em;
      flex-shrink: 0;
    }

    &.satisfied {
      .prereq-label { color: $slate-700; }
      .prereq-status {
        background: rgba(34, 197, 94, 0.12);
        border: 1px solid rgba(34, 197, 94, 0.25);
        color: #16a34a;
      }
    }
    &.unsatisfied {
      .prereq-label { color: $slate-400; }
      .prereq-status {
        background: rgba(239, 68, 68, 0.1);
        border: 1px solid rgba(239, 68, 68, 0.2);
        color: #dc2626;
      }
    }
  }
}

@media (max-width: 900px) {
  .sidebar { width: 100% !important; border-right: none; border-bottom: 1px solid $slate-200; .sidebar-toggle { display: none; } }
  .sidebar.collapsed { width: 100% !important; }
  .sidebar-collapsed-icons { display: none; }
}
</style>
