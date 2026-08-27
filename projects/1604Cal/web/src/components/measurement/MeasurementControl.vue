<template>
  <section class="control-card">
    <!-- 第一排：模式与进度 -->
    <div class="control-row-top">
      <!-- 工作台级开关：分批模式 + 采集通道入口。
           原独立 mode-toolbar 占整行浪费垂直空间，合并到控制卡片首行左侧。
           分批模式开关放最左，与模式/打压分段控件视觉同级。 -->
      <button
        type="button"
        class="batch-mode-switch"
        :class="{ active: batchMode }"
        :aria-pressed="batchMode"
        @click="$emit('toggle-batch')"
      >
        <span class="switch-track">
          <span class="switch-thumb" />
        </span>
        <span class="switch-label">分批</span>
      </button>

      <button
        type="button"
        class="channel-select-btn"
        @click="$emit('open-channel-dialog')"
      >
        <el-icon><Grid /></el-icon>
        <span>通道 {{ channelCount }}/16</span>
      </button>

      <span
        class="toolbar-divider"
        aria-hidden="true"
      />

      <div class="mode-group">
        <div class="mode-item">
          <span class="mode-label">模式</span>
          <div class="segment-control">
            <button
              type="button"
              class="segment-btn"
              :class="{ active: measurementStore.measurementParams.controlMode === ControlMode.Auto }"
              @click="setControlMode(ControlMode.Auto)"
            >
              自动
            </button>
            <button
              type="button"
              class="segment-btn"
              :class="{ active: measurementStore.measurementParams.controlMode === ControlMode.Manual }"
              @click="setControlMode(ControlMode.Manual)"
            >
              手动
            </button>
          </div>
        </div>

        <div class="mode-item">
          <span class="mode-label">打压</span>
          <div class="segment-control">
            <button
              type="button"
              class="segment-btn"
              :class="{ active: measurementStore.measurementParams.pressureMode === PressureMode.Single }"
              @click="measurementStore.measurementParams.pressureMode = PressureMode.Single"
            >
              单程
            </button>
            <button
              type="button"
              class="segment-btn pressure-active"
              :class="{ active: measurementStore.measurementParams.pressureMode === PressureMode.RoundTrip }"
              @click="measurementStore.measurementParams.pressureMode = PressureMode.RoundTrip"
            >
              回程
            </button>
          </div>
        </div>
      </div>

      <!-- 进度条 -->
      <div class="progress-group">
        <div class="progress-labels">
          <span class="progress-text">任务进度: {{ completedCount }}/{{ totalCount }}</span>
          <span class="progress-percent">{{ progressPercent }}%</span>
        </div>
        <div class="progress-track">
          <div
            class="progress-fill"
            :style="{ transform: 'scaleX(' + progressPercent / 100 + ')' }"
          />
        </div>
      </div>

      <!-- 操作按钮组 -->
      <div class="control-buttons">
        <template v-if="measurementStore.measurementParams.controlMode === ControlMode.Auto">
          <!-- 主按钮：单一焦点，文案/图标/色阶随会话状态自动切换 -->
          <button
            type="button"
            class="ctrl-btn ctrl-btn-primary"
            :class="`btn-${measurementStore.primaryAction.variant}`"
            :disabled="isPrimaryDisabled"
            @click="dispatchPrimary"
          >
            <el-icon><component :is="iconMap[measurementStore.primaryAction.icon]" /></el-icon>
            {{ measurementStore.primaryAction.label }}
          </button>

          <!-- 副按钮：仅在该状态下确实可用时才出现 -->
          <button
            v-for="action in measurementStore.secondaryActions"
            :key="action.key"
            type="button"
            class="ctrl-btn"
            :class="`btn-${action.variant}`"
            @click="dispatchSecondary(action)"
          >
            {{ action.label }}
          </button>
        </template>
        <template v-else>
          <button
            type="button"
            class="ctrl-btn btn-start"
            :disabled="!canManualStart"
            @click="$emit('manual-start')"
          >
            开始
          </button>
          <button
            v-if="hasPressureDevice"
            type="button"
            class="ctrl-btn btn-start"
            :disabled="!canManualPressurize"
            @click="$emit('manual-pressurize')"
          >
            手动打压
          </button>
          <button
            type="button"
            class="ctrl-btn btn-pause"
            :disabled="!measurementStore.isRunning"
            @click="$emit('pause')"
          >
            <el-icon><VideoPause /></el-icon>
            暂停
          </button>
          <button
            type="button"
            class="ctrl-btn btn-resume"
            :disabled="!measurementStore.isPaused"
            @click="$emit('resume')"
          >
            恢复
          </button>
          <button
            type="button"
            class="ctrl-btn btn-stop"
            :disabled="measurementStore.isIdle"
            @click="$emit('stop')"
          >
            <el-icon><CloseBold /></el-icon>
            停止
          </button>
          <button
            v-if="measurementStore.hasCompletedPoints"
            type="button"
            class="ctrl-btn btn-reset"
            @click="$emit('reset')"
          >
            重置
          </button>
          <!-- 导出按钮已移到数据区底部 template-bar（P1-5），与标定模块入口统一 -->
        </template>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, type PropType } from 'vue'
import { VideoPlay, VideoPause, CloseBold, Download, Refresh, Grid } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import { useMeasurementStore } from '@/stores/measurement'
import type { SecondaryAction } from '@/stores/measurement/types'
import { ControlMode, PressureMode } from '@/types/calibration'

const props = defineProps({
  channels: {
    type: Array as PropType<number[]>,
    default: undefined
  },
  canStart: {
    type: Boolean,
    default: undefined
  },
  isStable: {
    type: Boolean,
    default: undefined
  },
  stableSeconds: {
    type: Number,
    default: undefined
  },
  selectedChannelCount: {
    type: Number,
    default: undefined
  },
  hasPressureDevice: {
    type: Boolean,
    default: true
  },
  exporting: {
    type: Boolean,
    default: false
  },
  // 分批模式开关状态：合并自原 mode-toolbar
  batchMode: {
    type: Boolean,
    default: false
  },
  // 采集通道数量：合并自原 mode-toolbar
  channelCount: {
    type: Number,
    default: 0
  }
})

const emit = defineEmits<{
  start: []
  pause: []
  resume: []
  stop: []
  reset: []
  export: []
  retry: []
  restart: []
  'view-error': []
  'manual-start': []
  'manual-pressurize': []
  // 工作台级开关事件：合并自原 mode-toolbar
  'toggle-batch': []
  'open-channel-dialog': []
}>()

const iconMap: Record<string, unknown> = {
  VideoPlay,
  VideoPause,
  Download,
  Refresh
}

const measurementStore = useMeasurementStore()

const completedCount = computed(() =>
  measurementStore.points.filter(p => p.status === 'completed').length
)

const totalCount = computed(() => measurementStore.points.length)

const progressPercent = computed(() => {
  if (totalCount.value === 0) return 0
  return Math.round((completedCount.value / totalCount.value) * 100)
})

function setControlMode(mode: ControlMode) {
  measurementStore.measurementParams.controlMode = mode
}

const canManualPressurize = computed(() =>
  props.hasPressureDevice &&
  measurementStore.points.length > 0 &&
  !measurementStore.isRunning
)

const canManualStart = computed(() =>
  measurementStore.measurementParams.controlMode === ControlMode.Manual &&
  measurementStore.points.length > 0 &&
  ['idle', 'stopped', 'completed'].includes(measurementStore.state)
)

const isPrimaryDisabled = computed(() => {
  if (props.exporting) return true
  if (!measurementStore.deviceBound) return true
  if (measurementStore.primaryAction.key === 'start') {
    return measurementStore.points.length === 0
  }
  return false
})

function dispatchPrimary() {
  const key = measurementStore.primaryAction.key
  switch (key) {
    case 'start':  emit('start'); break
    case 'pause':  emit('pause'); break
    case 'resume': emit('resume'); break
    case 'export': emit('export'); break
    case 'retry':  emit('retry'); break
  }
}

async function dispatchSecondary(action: SecondaryAction) {
  if (action.confirm) {
    try {
      await ElMessageBox.confirm(action.confirm, '操作确认', {
        confirmButtonText: '确认',
        cancelButtonText: '取消',
        type: 'warning'
      })
    } catch {
      return
    }
  }

  switch (action.key) {
    case 'stop':       emit('stop'); break
    case 'reset':      emit('reset'); break
    case 'export':     emit('export'); break
    case 'restart':    emit('restart'); break
    case 'view-error': emit('view-error'); break
  }
}

</script>

<style scoped lang="scss">
.control-card {
  background: #ffffff;
  border-radius: 10px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  border: 1px solid $slate-200;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-family: $font-sans;
  transition: box-shadow 0.2s ease, border-color 0.2s ease;

  &:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }
}

.control-row-top {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
}

/* 工具条内分隔线：弱化，避免与外部分隔线同级 */
.toolbar-divider {
  width: 1px;
  height: 18px;
  background: $slate-200;
}

/* 分批模式开关：紧凑版 toggle，移到控制卡片首行左侧 */
.batch-mode-switch {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px 4px 6px;
  border: 1px solid $slate-200;
  border-radius: 999px;
  background: $slate-50;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  color: $slate-700;
  transition: all 0.15s ease;
  font-family: $font-sans;

  &:hover {
    border-color: $slate-300;
    background: #fff;
  }

  &.active {
    border-color: $mint;
    background: rgba(16, 185, 129, 0.08);
    color: $mint-dark;
  }
}

.batch-mode-switch .switch-track {
  position: relative;
  width: 24px;
  height: 14px;
  border-radius: 999px;
  background: $slate-200;
  transition: background 0.15s ease;
  flex-shrink: 0;
}

.batch-mode-switch .switch-thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
  transition: left 0.15s ease;
}

.batch-mode-switch.active .switch-track {
  background: $mint;
}

.batch-mode-switch.active .switch-thumb {
  left: 12px;
}

/* 采集通道入口：紧凑按钮 */
.channel-select-btn {
  height: 28px;
  padding: 0 10px;
  border: 1px solid $slate-200;
  border-radius: 6px;
  background: $slate-50;
  color: $blue;
  font-size: 12px;
  font-family: $font-mono;
  font-weight: 500;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 0.15s ease;

  &:hover {
    background: $slate-100;
    border-color: $slate-300;
  }

  .el-icon {
    font-size: 12px;
    color: $slate-400;
  }
}

.mode-group {
  display: flex;
  align-items: center;
  gap: 12px;
}

.mode-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

/* Label 规范：500, 12px, 1.5, 0.05em */
.mode-label {
  font-size: 12px;
  color: $slate-500;
  font-weight: 500;
  letter-spacing: 0.05em;
  white-space: nowrap;
  font-family: $font-sans;
}

.segment-control {
  display: flex;
  padding: 2px;
  background: $slate-100;
  border-radius: 6px;
  border: 1px solid $slate-200;
}

.segment-btn {
  padding: 3px 10px;
  font-size: 12px;
  font-weight: 500;
  border: none;
  background: transparent;
  color: $slate-500;
  cursor: pointer;
  border-radius: 4px;
  transition: all 0.15s ease;
  font-family: $font-sans;

  &:hover {
    color: $slate-700;
  }

  &.active {
    background: linear-gradient(135deg, $mint, $mint-dark);
    color: #fff;
    box-shadow: 0 1px 3px rgba(16, 185, 129, 0.25);
  }

  &.pressure-active.active {
    background: linear-gradient(135deg, $blue, #2563eb);
    box-shadow: 0 1px 3px rgba(59, 130, 246, 0.25);
  }
}

.progress-group {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 160px;
  max-width: 400px;
  gap: 6px;
}

.progress-labels {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.progress-text {
  font-size: 12px;
  color: $slate-500;
  font-weight: 500;
  letter-spacing: 0.02em;
  font-family: $font-sans;
}

.progress-percent {
  font-size: 12px;
  font-weight: 700;
  color: $mint-dark;
  font-family: $font-sans;
}

.progress-track {
  width: 100%;
  height: 4px;
  background: $slate-100;
  border-radius: 999px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, $mint, $mint-light);
  width: 100%;
  transform-origin: left;
  transition: transform 0.3s ease;
}

.control-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-left: auto;
}

/* 按钮基础：8px radius，规范过渡 */
.ctrl-btn {
  padding: 6px 12px;
  border-radius: 6px;
  border: none;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 600;
  transition: all 0.15s ease;
  font-family: $font-sans;

  &:active {
    transform: translateY(1px);
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

/* 主按钮：默认较宽，视觉层级最高 */
.ctrl-btn-primary {
  min-width: 96px;
  justify-content: center;
}

/* Primary：渐变 + 白色文字 + Mint 阴影 */
.btn-mint,
.btn-start {
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);

  &:hover:not(:disabled) {
    background: linear-gradient(135deg, $mint-light, $mint);
    box-shadow: 0 4px 12px rgba(16, 185, 129, 0.4);
    transform: translateY(-1px);
  }
}

/* Default：半透明 slate */
.btn-slate,
.btn-pause,
.btn-resume,
.btn-reset {
  background: rgba(55, 65, 81, 0.08);
  color: $slate-700;
  border: 1px solid $slate-200;

  &:hover:not(:disabled) {
    background: rgba(55, 65, 81, 0.14);
    border-color: $slate-300;
  }
}

/* Blue：信息主色 */
.btn-blue,
.btn-export {
  background: rgba(59, 130, 246, 0.1);
  color: $blue;
  border: 1px solid rgba(59, 130, 246, 0.2);

  &:hover:not(:disabled) {
    background: rgba(59, 130, 246, 0.16);
    border-color: rgba(59, 130, 246, 0.35);
  }
}

/* Amber：警告色，用于重试 */
.btn-amber {
  background: rgba(245, 158, 11, 0.1);
  color: $amber;
  border: 1px solid rgba(245, 158, 11, 0.2);

  &:hover:not(:disabled) {
    background: rgba(245, 158, 11, 0.18);
    border-color: rgba(245, 158, 11, 0.35);
    box-shadow: 0 2px 8px rgba(245, 158, 11, 0.2);
  }
}

/* Stop Red */
.btn-red,
.btn-stop {
  background: rgba(239, 68, 68, 0.1);
  color: $red;
  border: 1px solid rgba(239, 68, 68, 0.2);

  &:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.16);
    border-color: rgba(239, 68, 68, 0.35);
  }
}

@media (max-width: 900px) {
  .control-row-top {
    flex-direction: column;
    align-items: flex-start;
  }

  .progress-group {
    width: 100%;
  }

  .control-buttons {
    width: 100%;
    margin-left: 0;
  }
}
</style>
