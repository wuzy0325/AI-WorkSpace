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
      @click="$emit('toggle')"
      @keydown.enter="$emit('toggle')"
      @keydown.space.prevent="$emit('toggle')"
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
      <!-- 计量采集设备：触发按钮 + 抽屉，交互与计量模块侧栏保持一致。
           连接/断开/单位变更仍走标定会话（moduleName='calibration'）。 -->
      <div class="sidebar-section">
        <button
          type="button"
          class="device-drawer-trigger"
          @click="drawerVisible = true"
        >
          <el-icon><Monitor /></el-icon>
          <span class="trigger-label">计量采集设备</span>
          <span class="trigger-status">{{ savedMeasureCount }} 台已选</span>
          <el-icon><ArrowRight /></el-icon>
        </button>
        <CalibrationDeviceDrawer
          ref="drawerRef"
          v-model="drawerVisible"
          @connect="handleMeasureConnect"
          @disconnect="handleMeasureDisconnect"
          @unit-change="checkUnitConsistency"
        />
      </div>

      <!-- 打压设备 -->
      <div class="sidebar-section">
        <h3 class="sidebar-title">
          <el-icon><FirstAidKit /></el-icon>
          打压设备
        </h3>
        <PressDevicePanel
          @connect="calibrationUI.connectPressDevice"
          @disconnect="calibrationUI.disconnectPressDevice"
          @unit-change="handleUnitChange"
        />
      </div>

      <!-- 校准前置条件 -->
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
              <CircleClose />
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
        :class="{ 'is-connected': calibrationStore.device1604Connected }"
        role="button"
        tabindex="0"
        :aria-label="`1604 计量设备：${calibrationStore.device1604Connected ? '已连接' : '未连接'}，点击展开`"
        @click="$emit('toggle')"
        @keydown.enter="$emit('toggle')"
      >
        <Monitor />
      </el-icon>
      <el-icon
        class="collapsed-icon"
        :class="{ 'is-connected': calibrationStore.pressDeviceConnected }"
        role="button"
        tabindex="0"
        :aria-label="`打压设备：${calibrationStore.pressDeviceConnected ? '已连接' : '未连接'}，点击展开`"
        @click="$emit('toggle')"
        @keydown.enter="$emit('toggle')"
      >
        <FirstAidKit />
      </el-icon>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import {
  ArrowLeft,
  ArrowRight,
  Monitor,
  FirstAidKit,
  CircleCheckFilled,
  CircleClose
} from '@element-plus/icons-vue'

import CalibrationDeviceDrawer from '@/components/calibration/CalibrationDeviceDrawer.vue'
import PressDevicePanel from '@/components/common/PressDevicePanel.vue'
import { useCalibrationStore } from '@/stores/calibration'
import { useCalibrationUI } from '@/composables/useCalibrationUI'
import { useDeviceStore } from '@/stores/deviceStore'
import { fetchUnitConsistency } from '@/api/device'
import { ControlMode } from '@/types/calibration'

defineProps<{
  collapsed: boolean
}>()

defineEmits<{
  toggle: []
}>()

const calibrationStore = useCalibrationStore()
const calibrationUI = useCalibrationUI()
const moduleDeviceStore = useDeviceStore()

// 计量设备抽屉显隐与已选台数（读取模块级勾选配置，连接/断开时同步更新）
const drawerVisible = ref(false)
const drawerRef = ref<InstanceType<typeof CalibrationDeviceDrawer> | null>(null)
const savedMeasureCount = computed(() =>
  moduleDeviceStore.selectionByModule('calibration').measureDeviceIds.length
)

// 连接多台计量设备：复用 UI 包装（统一错误提示），成功后持久化勾选，
// 供下次进入标定模块时抽屉恢复上次选择。连接完成后整批绑定已建立，
// 再通知抽屉回读一次阀门/单位，避免绑定完成前触发读取报 binding token 500。
async function handleMeasureConnect(deviceIds: string[]) {
  await calibrationUI.connectDevice1604(deviceIds)
  if (!calibrationStore.device1604Connected) return
  moduleDeviceStore.setModuleSelection('calibration', { measureDeviceIds: [...deviceIds] })
  drawerRef.value?.refresh()
}

// 断开后从恢复配置中移除对应勾选，保持"已选 N 台"与实际一致，并刷新抽屉状态。
async function handleMeasureDisconnect(deviceIds: string[]) {
  await calibrationUI.disconnectDevice1604(deviceIds)
  const saved = moduleDeviceStore.selectionByModule('calibration').measureDeviceIds
  const remaining = saved.filter(id => !deviceIds.includes(id))
  moduleDeviceStore.setModuleSelection('calibration', { measureDeviceIds: remaining })
  drawerRef.value?.refresh()
}

const unitConsistent = ref(true)

// 单位一致性检查：先于 watcher 定义，避免 immediate 回调触发 TDZ 引用错误
// （设备已连接时切入标定页，immediate 回调同步执行会读到未初始化的 const）。
const checkUnitConsistency = async () => {
  try {
    const result = await fetchUnitConsistency()
    unitConsistent.value = result.consistent
  } catch {
    // 静默失败
  }
}

const handleUnitChange = async () => {
  if (calibrationStore.device1604Connected && calibrationStore.pressDeviceConnected) {
    await checkUnitConsistency()
  }
}

// 当计量设备和打压设备都连接时，自动检查单位一致性
watch(
  () => ({
    measureConnected: calibrationStore.device1604Connected,
    pressConnected: calibrationStore.pressDeviceConnected
  }),
  async ({ measureConnected, pressConnected }) => {
    if (measureConnected && pressConnected) {
      await checkUnitConsistency()
    } else {
      unitConsistent.value = true
    }
  },
  { immediate: true }
)

// 计量设备单位切换时重新检查一致性
watch(
  () => calibrationStore.measureUnit,
  async (newUnit, oldUnit) => {
    if (newUnit && oldUnit && newUnit !== oldUnit) {
      if (calibrationStore.device1604Connected && calibrationStore.pressDeviceConnected) {
        await checkUnitConsistency()
      }
    }
  }
)

const prerequisites = computed(() => {
  const items = [
    { label: '1604 设备已连接', satisfied: calibrationStore.device1604Connected },
    { label: '已选择采集通道', satisfied: calibrationStore.channelsSelected }
  ]
  // 手动模式下打压设备为可选条件，不作为启动前置要求
  if (calibrationStore.controlMode === ControlMode.Auto) {
    items.splice(1, 0, { label: '打压设备已连接', satisfied: calibrationStore.pressDeviceConnected })
  }
  // 自动模式下需要采集设备和打压设备单位一致
  if (calibrationStore.controlMode === ControlMode.Auto) {
    items.push({ label: '设备单位一致', satisfied: unitConsistent.value })
  }
  return items
})
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

  &.collapsed {
    width: 48px;
  }
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

  .el-icon {
    color: $slate-400;
    font-size: 10px;
  }

  &:hover {
    background: $slate-50;

    .el-icon {
      color: $mint;
    }
  }
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

/* 计量设备抽屉触发按钮：与计量模块 MeasurementSidebar 同风格 */
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

  .el-icon {
    color: $mint;
    font-size: 14px;
  }
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

    .el-icon {
      font-size: 16px;
      flex-shrink: 0;
    }
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
  .sidebar {
    width: 100% !important;
    border-right: none;
    border-bottom: 1px solid $slate-200;

    .sidebar-toggle {
      display: none;
    }
  }

  .sidebar.collapsed {
    width: 100% !important;
  }

  .sidebar-content {
    padding: 16px;
    gap: 16px;
  }

  .sidebar-collapsed-icons {
    display: none;
  }
}
</style>
