<template>
  <section class="params-card">
    <div class="params-grid">
      <div class="param-group">
        <span class="group-label">压力范围</span>
        <div class="group-body">
          <label class="input-label">最小</label>
          <input
            v-model.number="calibrationStore.calibrationParams.minValue"
            type="number"
            step="0.1"
            class="compact-input"
            :disabled="isRunning"
          >
          <label class="input-label">最大</label>
          <input
            v-model.number="calibrationStore.calibrationParams.maxValue"
            type="number"
            step="0.1"
            class="compact-input"
            :disabled="isRunning"
          >
        </div>
      </div>

      <!-- 采集配置 -->
      <div class="param-group">
        <span class="group-label">采集配置</span>
        <div class="group-body">
          <label class="input-label">点数</label>
          <input
            v-model.number="calibrationStore.calibrationParams.points"
            type="number"
            min="2"
            max="5"
            class="compact-input narrow"
            :disabled="isRunning"
          >
          <label class="input-label">精度</label>
          <input
            v-model.number="calibrationStore.calibrationParams.precision"
            type="number"
            min="0"
            max="4"
            class="compact-input narrow"
            :disabled="isRunning"
          >
          <label class="input-label">平均</label>
          <input
            v-model.number="calibrationStore.calibrationParams.averageCount"
            type="number"
            min="1"
            max="100"
            class="compact-input narrow"
            :disabled="isRunning"
          >
        </div>
      </div>

      <div class="param-group">
        <span class="group-label">稳定设置</span>
        <div class="group-body">
          <label class="input-label">时间</label>
          <select
            v-model.number="calibrationStore.calibrationParams.stableTime"
            class="compact-select"
            :disabled="isRunning"
          >
            <option :value="1">
              1s
            </option>
            <option :value="3">
              3s
            </option>
            <option :value="5">
              5s
            </option>
            <option :value="10">
              10s
            </option>
          </select>
        </div>
      </div>

      <!-- 控制 / 通道 并列 -->
      <div class="param-group">
        <span class="group-label">控制模式</span>
        <div class="group-body">
          <div class="segment-control">
            <button
              type="button"
              class="segment-btn"
              :class="{ active: calibrationStore.controlMode === ControlMode.Auto }"
              :disabled="isRunning"
              @click="calibrationStore.controlMode = ControlMode.Auto"
            >
              自动
            </button>
            <button
              type="button"
              class="segment-btn"
              :class="{ active: calibrationStore.controlMode === ControlMode.Manual }"
              :disabled="isRunning"
              @click="calibrationStore.controlMode = ControlMode.Manual"
            >
              手动
            </button>
          </div>
        </div>
      </div>

      <div class="param-group">
        <span class="group-label">采集通道</span>
        <div class="group-body">
          <button
            class="channel-select-btn"
            :disabled="isRunning"
            @click="channelDialogVisible = true"
          >
            <el-icon><Grid /></el-icon>
            <span>{{ calibrationStore.selectedChannels.length }}/16</span>
          </button>
        </div>
      </div>
    </div>

    <ChannelSelectDialog
      v-model:visible="channelDialogVisible"
      :selected-channels="calibrationStore.selectedChannels"
      @confirm="calibrationStore.setSelectedChannels"
    />
  </section>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Grid } from '@element-plus/icons-vue'
import { useCalibrationStore } from '@/stores/calibration'
import ChannelSelectDialog from '@/components/common/ChannelSelectDialog.vue'
import { ControlMode } from '@/types/calibration'

const calibrationStore = useCalibrationStore()
const channelDialogVisible = ref(false)

const isRunning = computed(() => calibrationStore.isRunning)

let debounceTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => calibrationStore.calibrationParams,
  (newVal, oldVal) => {
    if (debounceTimer) clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => {
      const pointsChanged = !oldVal || newVal.points !== oldVal.points
      calibrationStore.generatePressurePoints({ silent: !pointsChanged })
    }, 500)
  },
  { deep: true, immediate: true }
)
</script>

<style scoped lang="scss">
$font-sans: 'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
$font-mono: 'JetBrains Mono', 'Fira Code', 'SF Mono', Consolas, monospace;
$mint: #10b981;
$mint-dark: #059669;
$slate-200: #e5e7eb;
$slate-300: #d1d5db;
$slate-400: #9ca3af;
$slate-500: #6b7280;
$slate-700: #374151;
$slate-800: #1f2937;
$blue: #3b82f6;

/* 自给自足的卡片样式：背景/圆角/阴影/padding 都由本组件承担，
   避免外层再包裹 card-block 造成卡片嵌套（DESIGN.md 禁止） */
.params-card {
  position: relative;
  background: #ffffff;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  padding: 12px 16px;
  font-family: $font-sans;
  overflow: hidden;
}

/* 顶部 1px mint 强调线，与 CalibrationControl 卡片保持视觉一致 */
.params-card::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 1px;
  background: $mint;
}

.params-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 20px 32px;
}

/* ── 参数分组 ── */
.param-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.group-label {
  font-size: 10px;
  font-weight: 600;
  color: $slate-400;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  font-family: $font-sans;
  padding-bottom: 2px;
  border-bottom: 1px solid $slate-200;
}

.group-body {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 8px;
}

.input-label {
  font-size: 12px;
  color: $slate-500;
  font-weight: 500;
  letter-spacing: 0.03em;
  white-space: nowrap;
  font-family: $font-sans;

  // 第一个 label 不需要左边距
  &:first-child {
    margin-left: 0;
  }
}

/* ── 输入控件 ── */
.compact-input {
  height: 30px;
  font-size: 13px;
  border: 1px solid $slate-300;
  border-radius: 8px;
  padding: 0 8px;
  width: 56px;
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

  &.narrow { width: 40px; }
}

.compact-input::-webkit-inner-spin-button,
.compact-input::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.compact-input { -moz-appearance: textfield; }

.compact-select {
  height: 30px;
  font-size: 12px;
  border: 1px solid $slate-300;
  border-radius: 8px;
  padding: 0 22px 0 8px;
  color: $slate-700;
  background: #fff;
  outline: none;
  cursor: pointer;
  min-width: 56px;
  appearance: none;
  font-family: $font-sans;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%239ca3af' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 6px center;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &:focus {
    border-color: $mint;
    box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.15);
  }
}

/* ── 分段控件 ── */

.channel-select-btn {
  height: 28px;
  padding: 0 10px;
  border: 1px solid $slate-300;
  border-radius: 8px;
  background: #fff;
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
    background: rgba(59, 130, 246, 0.06);
    border-color: $blue;
  }
  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .el-icon {
    font-size: 12px;
    color: $slate-400;
  }
}
.segment-control {
  display: flex;
  padding: 2px;
  background: $slate-200;
  border-radius: 8px;
}

.segment-btn {
  padding: 3px 12px;
  font-size: 12px;
  font-weight: 500;
  border: none;
  background: transparent;
  color: $slate-500;
  cursor: pointer;
  border-radius: 4px;
  transition: all 0.15s ease;
  font-family: $font-sans;

  &:hover { color: $slate-700; }
  &:disabled { opacity: 0.5; cursor: not-allowed; }

  &.active {
    background: linear-gradient(135deg, $mint, $mint-dark);
    color: #fff;
    box-shadow: 0 1px 3px rgba(16, 185, 129, 0.25);
  }
}

/* ── 响应式 ── */
@media (max-width: 900px) {
  .params-grid {
    flex-direction: column;
    gap: 14px;
  }
}
</style>
