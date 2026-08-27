<template>
  <section class="workbench-section data-section">
    <div
      class="table-panel panel-points"
      data-testid="pressure-point-operation-table"
    >
      <div class="table-header">
        <div class="table-title">
          <el-icon><Operation /></el-icon>
          <h3>压力点设置</h3>
          <span
            v-if="calibrationStore.pressurePoints.length > 0"
            class="record-count"
          >
            {{ calibrationStore.pressurePoints.length }} 个测点
          </span>
        </div>
      </div>

      <div class="table-body point-table-body">
        <el-table
          :data="tableData"
          border
          stripe
          class="point-operation-table"
        >
          <el-table-column
            prop="index"
            label="序号"
            width="55"
          />
          <el-table-column
            label="状态"
            width="85"
          >
            <template #default="{ row }">
              <el-tag
                :type="getPointStatusType(row.status)"
                size="small"
              >
                {{ getPointStatusText(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column
            label="目标压力"
            width="130"
          >
            <template #default="{ row }">
              <input
                v-if="row.status === 'pending'"
                :value="row.targetValue"
                type="number"
                :step="0.1"
                class="target-pressure-input"
                @change="(e: Event) => handleTargetPressureChange(row, (e.target as HTMLInputElement).value)"
              >
              <span
                v-else
                class="target-pressure-text"
              >{{ row.targetValue.toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column
            v-if="calibrationStore.controlMode === ControlMode.Manual"
            label="操作"
            min-width="170"
          >
            <template #default="{ row }">
              <div class="row-actions">
                <el-button
                  v-if="row.status === 'pending' && !manualModeWithoutPressDevice"
                  type="primary"
                  size="small"
                  :disabled="!canPressurize(row.status)"
                  :data-testid="`pressurize-btn-point-${row.index}`"
                  @click="calibrationStore.pressurize(row.id)"
                >
                  打压
                </el-button>
                <el-button
                  size="small"
                  type="info"
                  :disabled="!canCollect(row.status)"
                  :data-testid="`collect-btn-point-${row.index}`"
                  @click="calibrationStore.collectData(row.id)"
                >
                  采集
                </el-button>
                <span
                  v-if="row.status === 'collecting'"
                  class="collecting-text"
                >采集中...</span>
                <span
                  v-if="row.status !== 'pending' && row.status !== 'collecting' && !canCollect(row.status)"
                  class="idle-text"
                >--</span>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <!-- 空状态统一用 el-empty（P2-2），与计量模块视觉一致 -->
        <el-empty
          v-if="calibrationStore.pressurePoints.length === 0"
          description="配置标定参数后开始标定流程"
          :image-size="80"
          class="table-empty"
        />
      </div>
    </div>

    <div
      class="table-panel panel-results"
      data-testid="collected-data-table"
    >
      <div class="table-header">
        <div class="table-title">
          <el-icon><DataLine /></el-icon>
          <h3>采集数据</h3>
          <span
            v-if="tableData.length > 0"
            class="record-count"
          >
            {{ collectedCount }}/{{ tableData.length }} 已采集
          </span>
        </div>
        <div
          v-if="deviceOptions.length > 1"
          class="device-tabs"
        >
          <el-radio-group
            v-model="activeDeviceId"
            size="small"
          >
            <el-radio-button
              v-for="opt in deviceOptions"
              :key="opt.value"
              :value="opt.value"
            >
              {{ opt.label }}
            </el-radio-button>
          </el-radio-group>
        </div>
      </div>

      <div class="table-body data-table-body">
        <el-table
          :data="tableData"
          border
          stripe
          class="data-table"
        >
          <el-table-column
            prop="index"
            label="压力点"
            width="70"
          />
          <el-table-column
            label="目标压力"
            width="100"
          >
            <template #default="{ row }">
              {{ row.targetValue.toFixed(2) }}
            </template>
          </el-table-column>
          <el-table-column
            label="实际压力"
            width="100"
          >
            <template #default="{ row }">
              {{ row.actualPressure?.toFixed(2) || '--' }}
            </template>
          </el-table-column>
          <el-table-column
            label="设备状态"
            width="110"
          >
            <template #default="{ row }">
              <el-tag
                v-if="row.deviceStatus === 'skipped'"
                type="warning"
                size="small"
              >
                已跳过
              </el-tag>
              <el-tag
                v-else-if="row.deviceStatus === 'error'"
                type="danger"
                size="small"
              >
                异常
              </el-tag>
              <span v-else>--</span>
            </template>
          </el-table-column>
          <el-table-column
            v-for="ch in calibrationStore.selectedChannels"
            :key="ch"
            :label="`CH${ch}`"
            width="75"
          >
            <template #default="{ row }">
              <span :class="getChannelClass(row, ch - 1)">
                {{ row.channelValues[ch - 1]?.toFixed(precision) || '--' }}
              </span>
            </template>
          </el-table-column>
        </el-table>
        <div class="channel-legend">
          <span class="legend-label">通道偏差：</span>
          <span class="legend-item legend-good">≤ 0.1</span>
          <span class="legend-item legend-warning">≤ 0.5</span>
          <span class="legend-item legend-error">&gt; 0.5</span>
        </div>

        <el-empty
          v-if="calibrationStore.pressurePoints.length === 0"
          description="暂无采集数据"
          :image-size="80"
          class="table-empty"
        />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Operation } from '@element-plus/icons-vue'
import { useCalibrationStore } from '@/stores/calibration'
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import { type SessionState, ControlMode } from '@/types/calibration'
import { type DevicePointData } from '@/stores/calibration/types'

const calibrationStore = useCalibrationStore()
const deviceStore = useDeviceInventoryStore()

const precision = computed(() => calibrationStore.calibrationParams.precision || 2)

// 设备维度：多设备时按设备切换展示
const activeDeviceId = ref('')

// 设备选项：优先取压力点携带的设备维度数据中的设备 ID，缺失时用设备清单
const deviceOptions = computed(() => {
  const ids = new Map<string, string>()
  for (const p of calibrationStore.pressurePoints) {
    if (p.collectedByDevice) {
      for (const devId of Object.keys(p.collectedByDevice)) {
        const dev = deviceStore.measureDevices.find(d => d.id === devId)
        ids.set(devId, dev?.name || dev?.model || devId)
      }
    }
  }
  if (ids.size === 0 && deviceStore.measureDevices.length > 0) {
    for (const dev of deviceStore.measureDevices) {
      ids.set(dev.id, dev.name || dev.model || dev.id)
    }
  }
  return Array.from(ids.entries()).map(([value, label]) => ({ value, label }))
})

// 设备选项异步加载完成后自动选中首个设备（避免初始化时选项为空导致活动设备留空）
watch(
  deviceOptions,
  (options) => {
    if (!activeDeviceId.value && options.length > 0) {
      activeDeviceId.value = options[0].value
    }
    // 若活动设备已不在选项中（如设备被移除），回退到首个
    if (activeDeviceId.value && !options.some(o => o.value === activeDeviceId.value)) {
      activeDeviceId.value = options.length > 0 ? options[0].value : ''
    }
  },
  { immediate: true }
)

// 测点状态
const getPointStatusType = (status: string) => {
  const map: Record<string, string> = {
    pending: 'info',
    pressurizing: 'warning',
    stabilizing: '',
    collecting: 'primary',
    completed: 'success',
    error: 'danger'
  }
  return map[status] || 'info'
}

const getPointStatusText = (status: string) => {
  if (manualModeWithoutPressDevice.value) {
    if (status === 'pending') return '待采集'
    if (status === 'stabilizing') return '待采集'
  }

  const map: Record<string, string> = {
    pending: '待执行',
    pressurizing: '打压中',
    stabilizing: '稳定中',
    collecting: '采集中',
    completed: '已完成',
    error: '错误'
  }
  return map[status] || status
}

// 表格数据
interface TableRow {
  id: string
  index: number
  status: string
  targetValue: number
  channelValues: (number | undefined)[]
  actualPressure?: number
  deviceStatus?: string
}

// 取指定压力点当前活动设备的通道数据
function pointChannelValues(point: import('@/stores/calibration/types').PressurePoint): (number | undefined)[] {
  if (point.collectedByDevice && activeDeviceId.value) {
    const d = point.collectedByDevice[activeDeviceId.value]
    if (d?.collected) return d.collected
    return []
  }
  return point.collectedData || []
}

const tableData = computed<TableRow[]>(() =>
  calibrationStore.pressurePoints.map(point => {
    const devData: DevicePointData | undefined =
      point.collectedByDevice && activeDeviceId.value ? point.collectedByDevice[activeDeviceId.value] : undefined
    return {
      id: point.id,
      index: point.index,
      status: point.status,
      targetValue: point.targetPressure,
      channelValues: pointChannelValues(point),
      actualPressure: point.actualPressure,
      deviceStatus: devData?.status
    }
  })
)

const collectedCount = computed(() =>
  tableData.value.filter(r => r.status === 'completed').length
)

const operableSessionStates: SessionState[] = [
  'ready',
  'pressurizing',
  'stabilizing',
  'collecting',
  'point_done',
  'await_manual_collect',
  'await_alarm_resolution',
  'recovering'
]

const canOperatePointActions = computed(() =>
  operableSessionStates.includes(calibrationStore.sessionState)
)

const manualModeWithoutPressDevice = computed(() =>
  calibrationStore.controlMode === ControlMode.Manual && !calibrationStore.pressDeviceConnected
)

const canPressurize = (status: string) =>
  canOperatePointActions.value && status === 'pending' && !manualModeWithoutPressDevice.value

const canCollect = (status: string) =>
  canOperatePointActions.value && (
    status === 'stabilizing' ||
    status === 'completed' ||
    status === 'error' ||
    (status === 'pending' && manualModeWithoutPressDevice.value)
  )

const getChannelClass = (row: TableRow, index: number) => {
  const value = row.channelValues[index]
  if (value === undefined) return ''
  const diff = Math.abs(value - row.targetValue)
  if (diff < 0.1) return 'channel-good'
  if (diff < 0.5) return 'channel-warning'
  return 'channel-error'
}

// 修改目标压力
const handleTargetPressureChange = async (row: TableRow, val: string) => {
  const numVal = parseFloat(val)
  if (isNaN(numVal) || numVal < 0) {
    ElMessage.warning('目标压力必须为非负数')
    return
  }
  const result = await calibrationStore.updateTargetPressure(row.id, numVal)
  if (!result.ok) {
    ElMessage.error(result.detail || '更新目标压力失败')
  } else {
    ElMessage.success('目标压力已更新')
  }
}
</script>

<style scoped lang="scss">
@use "@/styles/calibration-table" as *;

.data-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  flex: 1;
}

.table-panel {
  background: #ffffff;
  border-radius: 12px;
  border: 1px solid $slate-200;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  padding: 12px 16px 16px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  font-family: $font-sans;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;

  &:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }
}

.panel-points {
  flex-shrink: 0;
}

.panel-results {
  flex: 1;
  min-height: 0;
}

.table-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  flex-shrink: 0;
  padding-bottom: 8px;
  border-bottom: 1px solid $slate-100;
}

.table-title {
  display: flex;
  align-items: center;
  gap: 8px;

  .el-icon {
    color: $mint;
    font-size: 16px;
  }

  h3 {
    color: $slate-700;
    margin: 0;
    font-size: 14px;
    font-weight: 600;
  }
}

/* 记录徽章：按 Tags 规范 */
.record-count {
  padding: 1px 8px;
  background: rgba(107, 114, 128, 0.12);
  border: 1px solid rgba(107, 114, 128, 0.25);
  color: $slate-500;
  font-size: 11px;
  font-weight: 500;
  border-radius: 4px;
  margin-left: 4px;
}

.table-actions {
  display: flex;
  gap: 8px;

  .el-button {
    height: 28px;
    padding: 0 12px;
    font-size: 12px;
    font-weight: 500;
    border-radius: 8px;
  }
}

/* 多设备切换标签 */
.device-tabs {
  flex-shrink: 0;

  :deep(.el-radio-group) {
    .el-radio-button__inner {
      font-size: 12px;
      padding: 6px 12px;
    }
  }
}

.table-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  border-radius: 8px;
}

.point-table-body {
  max-height: 280px;
}

.data-table-body {
  min-height: 220px;
}

.point-operation-table {
  width: 100%;

  @include calibration-table-deep-styles;
}

.data-table {
  width: 100%;
  height: 100%;

  @include calibration-table-deep-styles;

  .channel-good, .channel-warning, .channel-error {
    font-family: $font-mono;
  }
  .channel-good { color: $green; }
  .channel-warning { color: $amber; }
  .channel-error { color: $red; }
}

.row-actions { display: flex; gap: 6px; }

/* 目标压力输入框 */
.target-pressure-input {
  width: 80px;
  height: 28px;
  font-size: 13px;
  border: 1px solid $slate-300;
  border-radius: 6px;
  padding: 0 6px;
  text-align: center;
  color: $slate-800;
  background: #fff;
  outline: none;
  font-variant-numeric: tabular-nums;
  font-family: $font-mono;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &:focus {
    border-color: $mint;
    box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.15);
  }
}

.target-pressure-input::-webkit-inner-spin-button,
.target-pressure-input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

.target-pressure-input {
  -moz-appearance: textfield;
}

.target-pressure-text {
  font-family: $font-mono;
  font-size: 13px;
  color: $slate-700;
}

.channel-legend {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  font-size: 11px;
  font-family: $font-sans;
  color: $slate-500;
  border-top: 1px solid $slate-200;
}
.legend-label { font-weight: 500; }
.legend-item {
  padding: 1px 8px;
  border-radius: 4px;
  font-weight: 600;
  font-family: $font-mono;
}
.legend-good {
  background: rgba(34, 197, 94, 0.1);
  color: $green;
}
.legend-warning {
  background: rgba(245, 158, 11, 0.1);
  color: $amber;
}
.legend-error {
  background: rgba(239, 68, 68, 0.1);
  color: $red;
}

.row-btn {
  min-width: 56px;
}

.collecting-text { color: $slate-400; font-size: 12px; }

.idle-text { color: $slate-400; font-size: 12px; }

/* el-empty 空状态：与计量模块保持视觉一致（P2-2） */
.table-empty {
  padding: 32px 0;

  :deep(.el-empty__description) {
    font-size: 13px;
    color: $slate-400;
    font-family: $font-sans;
  }
}
</style>
