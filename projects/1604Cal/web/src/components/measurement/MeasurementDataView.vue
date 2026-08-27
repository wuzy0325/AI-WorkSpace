<template>
  <div class="data-table-wrapper">
    <!-- 压力点表 -->
    <div
      v-if="points.length > 0"
      class="points-section"
    >
      <div class="table-toolbar">
        <div class="toolbar-title">
          <el-icon class="toolbar-icon">
            <Aim />
          </el-icon>
          <h3>目标压力表数据清单</h3>
          <span class="record-badge">{{ points.length }} 个测点</span>
        </div>
        <div class="toolbar-legend">
          <span class="legend-item">
            <span class="legend-dot pending" />
            待采集
          </span>
          <span class="legend-item">
            <span class="legend-dot completed" />
            已采集
          </span>
        </div>
      </div>
      <!-- 多设备 tab：绑定多台计量设备时按设备切换查看各设备通道数据；
           单设备场景不显示，行为与改造前一致 -->
      <div
        v-if="deviceTabs.length > 1"
        class="device-tabs"
      >
        <button
          v-for="tab in deviceTabs"
          :key="tab.deviceId"
          type="button"
          class="device-tab"
          :class="{ active: tab.deviceId === activeDeviceId }"
          @click="measurementStore.setActiveDevice(tab.deviceId)"
        >
          {{ tab.name }}
          <span
            v-if="tab.skipped"
            class="skipped-badge"
          >
            已跳过
          </span>
        </button>
      </div>
      <div class="table-body">
        <!-- 改用 el-table 与标定模块统一表格实现（共用 calibration-table mixin）。
             超限高亮策略保持计量模块原有红色背景方案，与标定的文字颜色方案有意区分。 -->
        <el-table
          :data="points"
          border
          stripe
          :row-class-name="pointRowClassName"
          class="data-el-table"
        >
          <el-table-column
            label="序号"
            width="80"
          >
            <template #default="{ row }">
              <div class="index-cell-wrap">
                <span>{{ row.index }}</span>
                <span
                  v-if="isRoundTrip"
                  :class="['trip-badge', row.direction === 'forward' ? 'forward' : 'backward']"
                >
                  {{ row.direction === 'forward' ? '正' : '回' }}
                </span>
              </div>
            </template>
          </el-table-column>
          <el-table-column
            label="状态"
            width="110"
          >
            <template #default="{ row }">
              <button
                v-if="controlMode === ControlMode.Manual"
                type="button"
                class="row-collect-btn"
                @click="$emit('collect-point', row.index)"
              >
                采集
              </button>
              <span
                v-else
                :class="['status-tag', getStatusType(row.status)]"
              >
                <span :class="['status-dot', getStatusType(row.status)]" />
                {{ getStatusText(row.status) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column
            label="目标值"
            width="90"
            class-name="col-target"
          >
            <template #default="{ row }">
              <!-- 手动模式：逐点采集时需要调整目标，保持可编辑 input；
                   自动模式：目标由程序生成，显示为纯文本以减少边框视觉干扰 -->
              <input
                v-if="controlMode === ControlMode.Manual"
                :value="row.targetPressure"
                type="number"
                class="target-input"
                :step="precisionStep"
                @change="onTargetChange(row.id, ($event.target as HTMLInputElement).valueAsNumber)"
              >
              <span
                v-else
                class="target-text"
              >{{ row.targetPressure.toFixed(precisionForDisplay) }}</span>
            </template>
          </el-table-column>
          <el-table-column
            v-for="ch in visibleChannels"
            :key="ch"
            :label="`${ch}`"
            width="60"
            class-name="col-channel"
          >
            <template #default="{ row }">
              <div
                v-if="channelValue(row, ch) !== undefined"
                :class="['channel-value', { 'channel-over-limit': isChannelOverLimit(row, ch) }]"
              >
                {{ (channelValue(row, ch) as number).toFixed(precisionForDisplay) }}
              </div>
              <div
                v-else
                class="channel-value empty"
              >
                --
              </div>
            </template>
          </el-table-column>
          <el-table-column
            label="采集时间"
            width="100"
            class-name="col-time"
          >
            <template #default="{ row }">
              <span
                v-if="row.collectTime"
                class="time-display"
              >{{ formatTime(row.collectTime) }}</span>
              <span
                v-else
                class="time-display empty"
              >--:--:--</span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <div
      v-else
      class="empty-table-state"
    >
      <el-empty
        description="请配置参数并生成压力表"
        :image-size="80"
      >
        <p class="empty-hint">
          设置最小值、最大值和点数后点击"生成压力表"
        </p>
      </el-empty>
    </div>

    <!-- 实时采样数据 -->
    <div
      v-if="tableRows.length > 0"
      class="sample-section"
    >
      <div class="table-toolbar">
        <div class="toolbar-title">
          <el-icon class="toolbar-icon">
            <DataLine />
          </el-icon>
          <h3>实时采样数据</h3>
          <span class="record-badge">{{ rows.length }} 条采样</span>
        </div>
        <div class="toolbar-actions" />
      </div>
      <div class="table-body">
        <el-table
          :data="tableRows"
          border
          stripe
          class="data-el-table"
        >
          <el-table-column
            prop="index"
            label="序号"
            width="70"
          />
          <el-table-column
            v-if="deviceTabs.length > 1"
            label="设备"
            width="90"
            class-name="col-device"
          >
            <template #default="{ row }">
              {{ deviceNameById(row.deviceId) }}
            </template>
          </el-table-column>
          <el-table-column
            label="平均压力"
            width="100"
            class-name="col-pressure"
          >
            <template #default="{ row }">
              {{ row.actualPressure }}
            </template>
          </el-table-column>
          <el-table-column
            v-for="ch in visibleChannels"
            :key="ch"
            :label="`CH${ch}`"
            width="65"
            class-name="col-channel"
          >
            <template #default="{ row }">
              <div :class="['channel-value', { 'channel-over-limit': isSampleOverLimit(row, ch) }]">
                {{ row.channelValues[ch] ?? '--' }}
              </div>
            </template>
          </el-table-column>
          <el-table-column
            label="时间"
            width="100"
            class-name="col-time"
          >
            <template #default="{ row }">
              {{ row.collectTime }}
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { DataLine, Aim } from '@element-plus/icons-vue'
import { useMeasurementStore } from '@/stores/measurement'
import { useMeasurementDeviceStore } from '@/stores/measurement/deviceStore'
import type { CollectedRow } from '@/stores/measurement'
import type { MeasurementPoint } from '@/api/measurement'
import { ControlMode, PressureMode } from '@/types/calibration'

const props = defineProps<{
  rows?: CollectedRow[]
  channels?: number[]
  controlMode?: string
}>()

defineEmits<{
  'collect-point': [pointIndex: number]
}>()

const measurementStore = useMeasurementStore()
const deviceStore = useMeasurementDeviceStore()

const rows = computed(() => props.rows ?? measurementStore.rows)
const channels = computed(() => props.channels ?? measurementStore.channels)
const points = computed<MeasurementPoint[]>(() => measurementStore.points)

// ── 多设备 tab ──
// 当前查看的设备（默认首个绑定设备）；单设备场景恒为首个设备，不影响旧行为。
const activeDeviceId = computed(() => measurementStore.activeDeviceId)

// 设备 tab 列表：绑定多台计量设备时展示；含已跳过标记与原因。
const deviceTabs = computed(() => {
  const ids = measurementStore.measureDeviceIds
  if (ids.length <= 1) return []
  return ids.map(deviceId => {
    const dev = deviceStore.measureDevices.find(d => d.id === deviceId)
    const reason = measurementStore.skippedDevices[deviceId]
    return {
      deviceId,
      name: dev?.name || dev?.model || deviceId,
      skipped: Boolean(reason),
      skipReason: reason || ''
    }
  })
})

// 当前设备在某压力点的采集数据：多设备取 collectedByDevice[activeDeviceId]，
// 单设备回退 collectedData（兼容旧字段）。
function displayCollectedData(point: MeasurementPoint): number[] | undefined {
  if (activeDeviceId.value && point.collectedByDevice?.[activeDeviceId.value]) {
    return point.collectedByDevice[activeDeviceId.value].collected
  }
  return measurementStore.measureDeviceIds.length <= 1 ? point.collectedData : undefined
}

// 设备 ID → 显示名（实时采样表设备列）。
function deviceNameById(deviceId?: string): string {
  if (!deviceId) return '--'
  const dev = deviceStore.measureDevices.find(d => d.id === deviceId)
  return dev?.name || dev?.model || deviceId
}

// 当前设备在某压力点指定通道的采集值（undefined 表示未采集）。
function channelValue(point: MeasurementPoint, ch: number): number | undefined {
  const data = displayCollectedData(point)
  if (!data) return undefined
  return data[ch - 1]
}

const isRoundTrip = computed(() => measurementStore.measurementParams.pressureMode === PressureMode.RoundTrip)
const precisionForDisplay = computed(() => measurementStore.measurementParams.precision)
const currentPointIndex = computed(() => measurementStore.currentPointIndex)

const precisionStep = computed(() => Math.pow(10, -(measurementStore.measurementParams.precision || 2)))

const alarmEnabled = computed(() => measurementStore.alarmConfig.enabled)

// 量程引用误差容差：与后端 CheckAlarm、报告模板公式一致。
function calculateAllowance(target: number): number {
  const p = measurementStore.measurementParams
  const span = Math.abs(p.maxPressure - p.minPressure)
  const allowance = span * p.precisionLevel
  if (allowance < 1e-10) {
    return Math.abs(target) * p.precisionLevel
  }
  return allowance
}

function getPointOverLimitChannels(pt: MeasurementPoint): number[] {
  if (!alarmEnabled.value) return []
  const data = displayCollectedData(pt)
  if (!data || data.length === 0) return []

  const allowance = calculateAllowance(pt.targetPressure)
  const overLimit: number[] = []
  // 仅对已选通道做超限判定，未选通道不显示也不参与报警（与后端 EnabledChannels 一致）。
  for (const ch of visibleChannels.value) {
    const collectedVal = data[ch - 1]
    if (collectedVal === undefined || collectedVal === null) continue
    if (Math.abs(collectedVal - pt.targetPressure) > allowance) {
      overLimit.push(ch)
    }
  }
  return overLimit
}

function isChannelOverLimit(pt: MeasurementPoint, ch: number): boolean {
  const overLimit = getPointOverLimitChannels(pt)
  return overLimit.includes(ch)
}

function isSampleOverLimit(row: DisplayRow, ch: number): boolean {
  if (!alarmEnabled.value) return false
  const cv = row.channelValues[ch]
  if (!cv || cv === '--') return false
  const raw = parseFloat(cv)
  if (isNaN(raw)) return false
  const target = currentTargetPressure.value
  if (target === undefined) return false

  return Math.abs(raw - target) > calculateAllowance(target)
}

const currentTargetPressure = computed(() => {
  const idx = measurementStore.currentPointIndex
  if (idx <= 0 || idx > measurementStore.points.length) return undefined
  return measurementStore.points[idx - 1]?.targetPressure
})

// el-table 行 class：completed 行轻微高亮，current 行更强高亮 + 左边框
function pointRowClassName(row: MeasurementPoint): string {
  const classes: string[] = []
  if (row.status === 'completed') classes.push('row-completed')
  if (currentPointIndex.value === row.index) classes.push('row-current')
  return classes.join(' ')
}

function getStatusType(status: string): string {
  const map: Record<string, string> = {
    pending: 'pending',
    pressuring: 'processing',
    pressurizing: 'processing',
    stabilizing: 'processing',
    collecting: 'processing',
    completed: 'completed',
    error: 'error'
  }
  return map[status] || 'pending'
}

function getStatusText(status: string): string {
  const map: Record<string, string> = {
    pending: '待采集',
    pressuring: '打压中',
    pressurizing: '打压中',
    stabilizing: '稳定中',
    collecting: '采集中',
    completed: '已完成',
    error: '出错'
  }
  return map[status] || status
}

function onTargetChange(pointId: string, value: number | null) {
  if (value === null || isNaN(value)) return
  measurementStore.updatePointTarget(pointId, value)
}

function formatTime(timestamp: string): string {
  try {
    return new Date(timestamp).toLocaleTimeString('zh-CN', { hour12: false })
  } catch {
    return '--:--:--'
  }
}

function estimateActualPressure(row: CollectedRow | undefined): string {
  if (!row) return '--'
  const sourceChannels = channels.value.length > 0 ? channels.value : visibleChannels.value
  const values = sourceChannels
    .map(ch => row.channels[String(ch)])
    .filter((value): value is number => typeof value === 'number' && Number.isFinite(value))
  if (values.length === 0) return '--'
  const average = values.reduce((sum, val) => sum + val, 0) / values.length
  return average.toFixed(1)
}

const visibleChannels = computed(() => {
  if (channels.value.length > 0) return channels.value
  const channelSet = new Set<number>()
  for (const row of rows.value) {
    for (const key of Object.keys(row.channels)) {
      const ch = Number(key)
      if (Number.isInteger(ch)) channelSet.add(ch)
    }
  }
  return Array.from(channelSet).sort((a, b) => a - b)
})

interface DisplayRow {
  index: number
  deviceId?: string
  actualPressure: string
  channelValues: Record<number, string>
  collectTime: string
}

// 采样数据在 store 中保留最近 200 条，表格只渲染最近 100 条，
// 避免高频采样时大量 DOM 节点和格式化对象造成浏览器内存峰值。
const MAX_VISIBLE_SAMPLE_ROWS = 100

const tableRows = computed<DisplayRow[]>(() => {
  const startIndex = Math.max(0, rows.value.length - MAX_VISIBLE_SAMPLE_ROWS)
  return rows.value.slice(startIndex).map((row, offset) => {
    const channelValues: Record<number, string> = {}
    for (const ch of visibleChannels.value) {
      const raw = row.channels[String(ch)]
      channelValues[ch] = (typeof raw === 'number' && !isNaN(raw)) ? raw.toFixed(3) : '--'
    }
    return {
      index: startIndex + offset + 1,
      deviceId: row.deviceId,
      actualPressure: estimateActualPressure(row),
      channelValues,
      collectTime: formatTime(row.timestamp)
    }
  })
})
</script>

<style scoped lang="scss">
@use "@/styles/calibration-table" as *;

$font-sans: 'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
$font-mono: 'JetBrains Mono', 'Fira Code', 'SF Mono', Consolas, monospace;
$mint: #10b981;
$mint-light: #34d399;
$mint-dark: #059669;
$slate-50: #f9fafb;
$slate-100: #f3f4f6;
$slate-200: #e5e7eb;
$slate-300: #d1d5db;
$slate-400: #9ca3af;
$slate-500: #6b7280;
$slate-600: #4b5563;
$slate-700: #374151;
$slate-800: #1f2937;
$red: #ef4444;
$blue: #3b82f6;
$amber: #f59e0b;

.data-table-wrapper {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  gap: 12px;
  font-family: $font-sans;
}

.points-section,
.sample-section {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  background: #ffffff;
  border-radius: 12px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  border: 1px solid $slate-200;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;

  &:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
    border-color: rgba(16, 185, 129, 0.3);
  }
}

.points-section,
.sample-section {
  flex: 1;
  min-height: 0;
}

.table-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  border-bottom: 1px solid $slate-100;
  background: rgba(249, 250, 251, 0.5);
  flex-shrink: 0;
}

.toolbar-title {
  display: flex;
  align-items: center;
  gap: 8px;

  h3 {
    font-size: 14px;
    font-weight: 600;
    color: $slate-700;
    margin: 0;
    font-family: $font-sans;
  }
}

.toolbar-icon {
  font-size: 16px;
  color: $mint;
}

/* 记录徽章：按 Tags 规范 */
.record-badge {
  padding: 2px 8px;
  background: rgba(107, 114, 128, 0.12);
  border: 1px solid rgba(107, 114, 128, 0.25);
  color: $slate-500;
  font-size: 11px;
  font-weight: 500;
  border-radius: 4px;
  margin-left: 4px;
}

.toolbar-legend {
  display: flex;
  align-items: center;
  gap: 16px;
}

.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: $slate-500;
  font-family: $font-sans;
}

.legend-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;

  &.pending { background: $slate-300; }
  &.completed { background: $mint; }
}

.toolbar-actions {
  display: flex;
  gap: 8px;
}

/* 多设备 tab：按设备切换查看各设备通道数据 */
.device-tabs {
  display: flex;
  gap: 6px;
  padding: 8px 20px;
  border-bottom: 1px solid $slate-100;
  background: rgba(249, 250, 251, 0.5);
  flex-shrink: 0;
  flex-wrap: wrap;
}

.device-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border: 1px solid $slate-200;
  border-radius: 6px;
  background: #fff;
  color: $slate-600;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  font-family: $font-sans;

  &:hover {
    border-color: $mint;
    color: $mint-dark;
  }

  &.active {
    background: rgba(16, 185, 129, 0.1);
    border-color: $mint;
    color: $mint-dark;
    font-weight: 600;
  }
}

.skipped-badge {
  font-size: 12px;
  font-weight: 500;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
  color: #dc2626;
}

/* 表格主体：让 el-table 撑满剩余空间 */
.table-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* 共享 el-table 深度样式（与标定模块共用 _calibration-table.scss mixin） */
.data-el-table {
  width: 100%;
  height: 100%;
  @include calibration-table-deep-styles;
}

/* 行高亮：completed 轻微高亮，current 更强高亮 + 左边框 */
:deep(.el-table__row.row-completed td.el-table__cell) {
  background: rgba(16, 185, 129, 0.04) !important;
}

:deep(.el-table__row.row-current td.el-table__cell) {
  background: rgba(16, 185, 129, 0.06) !important;
  border-left: 2px solid $mint;
}

/* 通道列：等宽 mono 字体，便于数值对齐 */
:deep(.col-channel .cell) {
  font-family: $font-mono;
  font-size: 11px;
  text-align: center;
  padding: 0 4px;
}

/* 时间列：右对齐 mono 字体 */
:deep(.col-time .cell) {
  font-family: $font-mono;
  font-size: 11px;
  color: $slate-400;
  text-align: right;
}

/* 平均压力列：mono 字体 */
:deep(.col-pressure .cell) {
  font-family: $font-mono;
  font-variant-numeric: tabular-nums;
}

.index-cell-wrap {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-family: $font-mono;
  color: $slate-400;
  font-weight: 600;
}

/* 正/回标签 */
.trip-badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  font-weight: 600;

  &.forward {
    background: rgba(59, 130, 246, 0.12);
    border: 1px solid rgba(59, 130, 246, 0.25);
    color: $blue;
  }

  &.backward {
    background: rgba(245, 158, 11, 0.12);
    border: 1px solid rgba(245, 158, 11, 0.25);
    color: #d97706;
  }
}

/* 状态标签：按 Tags 规范 */
.status-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  white-space: nowrap;
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 500;
  font-family: $font-sans;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

/* pending: gray */
.status-tag.pending {
  background: rgba(107, 114, 128, 0.1);
  border: 1px solid rgba(107, 114, 128, 0.2);
  color: $slate-500;
}
.status-dot.pending { background: $slate-300; }

/* processing: blue */
.status-tag.processing {
  background: rgba(59, 130, 246, 0.1);
  border: 1px solid rgba(59, 130, 246, 0.2);
  color: $blue;
}
.status-dot.processing { background: $blue; }

/* completed: green */
.status-tag.completed {
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  color: $mint-dark;
}
.status-dot.completed { background: $mint; }

/* error: red */
.status-tag.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.2);
  color: $red;
}
.status-dot.error { background: $red; }

/* 行内采集按钮 */
.row-collect-btn {
  padding: 3px 12px;
  border: 1px solid $mint;
  border-radius: 6px;
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  font-family: $font-sans;
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover:not(:disabled) {
    background: linear-gradient(135deg, $mint-light, $mint);
    transform: translateY(-1px);
    box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);
  }

  &:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
}

.target-input {
  width: 60px;
  text-align: center;
  border: 1px solid $slate-200;
  border-radius: 8px;
  background: #fff;
  color: $slate-700;
  padding: 4px 6px;
  font-size: 13px;
  font-weight: 600;
  outline: none;
  font-variant-numeric: tabular-nums;
  font-family: $font-mono;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &::-webkit-inner-spin-button,
  &::-webkit-outer-spin-button {
    -webkit-appearance: none;
    margin: 0;
  }

  -moz-appearance: textfield;

  &:focus {
    border-color: $mint;
    box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.15);
  }
}

/* 自动模式目标值：纯文本展示，减少 input 边框的视觉干扰 */
.target-text {
  font-family: $font-mono;
  font-size: 13px;
  color: $slate-700;
}

/* 通道值：mono 字体，超限时红色背景高亮（计量模块策略，与标定文字颜色方案有意区分） */
.channel-value {
  font-variant-numeric: tabular-nums;
  font-family: $font-mono;
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid transparent;
  display: inline-block;
  min-width: 36px;
  text-align: center;

  &.empty {
    color: $slate-300;
  }
}

.channel-over-limit {
  background: rgba(239, 68, 68, 0.12) !important;
  border-color: rgba(239, 68, 68, 0.25) !important;
  color: $red;
  font-weight: 700;
}

.time-display {
  font-size: 11px;
  color: $slate-400;
  font-family: $font-mono;

  &.empty {
    color: $slate-300;
  }
}

.empty-table-state {
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  border: 1px solid $slate-200;
  padding: 40px 0;
}

.empty-hint {
  font-size: 12px;
  color: $slate-400;
  margin-top: 8px;
  font-family: $font-sans;
}

@media (max-width: 768px) {
  .table-toolbar {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }

  .toolbar-legend {
    width: 100%;
  }
}
</style>
