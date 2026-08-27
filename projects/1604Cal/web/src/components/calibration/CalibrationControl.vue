<template>
  <section class="control-card">
    <div class="control-main">
      <!-- 进度组：仅在已生成压力点时显示 -->
      <div
        v-if="calibrationStore.pressurePoints.length > 0"
        class="progress-group"
      >
        <div class="progress-labels">
          <span class="progress-text">进度 {{ completedCount }}/{{ calibrationStore.pressurePoints.length }}</span>
          <span class="stable-label">{{ calibrationStore.isStable ? '已稳定' : '稳定中' }}</span>
        </div>
        <div class="progress-track">
          <div
            class="progress-fill"
            :style="{ transform: 'scaleX(' + progressPercent / 100 + ')' }"
          />
        </div>
      </div>

      <!-- 动态按钮组：主按钮固定位置，副按钮按状态出现 -->
      <div class="action-group">
        <!-- 主按钮：单一焦点，文案/图标/色阶随会话状态自动切换。
             disabled 时用 el-tooltip 提示阻断原因，避免用户反复点击试探。 -->
        <el-tooltip
          :content="primaryDisabledReason"
          :disabled="!isPrimaryDisabled"
          placement="top"
          show-after="300"
        >
          <span class="primary-btn-wrapper">
            <button
              type="button"
              class="ctrl-btn ctrl-btn-primary"
              :class="`btn-${calibrationStore.primaryAction.variant}`"
              :disabled="isPrimaryDisabled"
              @click="dispatchPrimary"
            >
              <el-icon><component :is="iconMap[calibrationStore.primaryAction.icon]" /></el-icon>
              {{ calibrationStore.primaryAction.label }}
            </button>
          </span>
        </el-tooltip>

        <!-- 副按钮：仅在该状态下确实可用时才出现 -->
        <button
          v-for="action in calibrationStore.secondaryActions"
          :key="action.key"
          type="button"
          class="ctrl-btn"
          :class="`btn-${action.variant}`"
          @click="dispatchSecondary(action)"
        >
          {{ action.label }}
        </button>

        <!-- 设备级报警（多设备采集失败/超限）：提供"跳过该设备"动作 -->
        <button
          v-if="alarmDeviceId && calibrationStore.sessionState === 'await_alarm_resolution'"
          type="button"
          class="ctrl-btn btn-amber"
          @click="handleSkipDevice"
        >
          跳过该设备
        </button>
      </div>
    </div>

    <!-- 跳过设备弹窗：预设原因 + 备注 -->
    <SkipDeviceDialog
      v-model="showSkipDeviceDialog"
      :device-name="skipDeviceName"
      @confirm="handleSkipDeviceConfirm"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import {
  VideoPlay,
  VideoPause,
  RefreshRight,
  CloseBold,
  DataAnalysis,
  CircleClose,
  CircleCheck,
  Right
} from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import { useCalibrationStore } from '@/stores/calibration'
import type { SecondaryAction } from '@/stores/calibration/types'
import { useCalibrationUI } from '@/composables/useCalibrationUI'
import { alarmEventKey, type AlarmEventData } from '@/composables/useCalibrationSync'
import SkipDeviceDialog from '@/components/common/SkipDeviceDialog.vue'
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'

const calibrationStore = useCalibrationStore()
const calibrationUI = useCalibrationUI()
const deviceStore = useDeviceInventoryStore()

// 当前报警事件：携带 deviceId 时表示设备级报警（采集失败/超限），可提供"跳过该设备"动作
const alarmEvent = inject(alarmEventKey, ref<AlarmEventData | null>(null))
const alarmDeviceId = computed(() => alarmEvent?.value?.deviceId || '')

// 跳过设备弹窗状态
const showSkipDeviceDialog = ref(false)
const skipDeviceName = computed(() => {
  const dev = deviceStore.measureDevices.find(d => d.id === alarmDeviceId.value)
  return dev?.name || dev?.model || alarmDeviceId.value
})

// 图标名 → 组件映射，与 primaryAction.icon 字符串对应
const iconMap: Record<string, unknown> = {
  VideoPlay,
  VideoPause,
  RefreshRight,
  CloseBold,
  DataAnalysis,
  CircleClose,
  CircleCheck,
  Right
}

// 主按钮禁用条件：
// - fitting 态：拟合进行中，禁用点击
// - start 态：未满足启动门禁或未生成压力点
const isPrimaryDisabled = computed(() => {
  const key = calibrationStore.primaryAction.key
  if (key === 'fitting') return true
  if (key === 'start') {
    return !calibrationStore.canStartCalibration || calibrationStore.pressurePoints.length === 0
  }
  return false
})

// 主按钮 disabled 时的 tooltip 文案：给用户明确的修复指引，
// 避免反复点击试探。与 isPrimaryDisabled 的判断条件一一对应。
const primaryDisabledReason = computed(() => {
  const key = calibrationStore.primaryAction.key
  if (key === 'fitting') return '数据拟合进行中，请稍候'
  if (key === 'start') {
    if (calibrationStore.pressurePoints.length === 0) return '请先生成压力点表'
    if (!calibrationStore.canStartCalibration) return '请先满足启动条件（设备连接、通道选择、单位一致）'
  }
  return ''
})

const completedCount = computed(() =>
  calibrationStore.pressurePoints.filter(p => p.status === 'completed').length
)
const progressPercent = computed(() => {
  const total = calibrationStore.pressurePoints.length
  if (total === 0) return 0
  return Math.round((completedCount.value / total) * 100)
})

// 主按钮分发：根据当前 key 调用对应动作
async function dispatchPrimary() {
  const key = calibrationStore.primaryAction.key
  switch (key) {
    case 'start':          await calibrationUI.startCalibration(); break
    case 'pause':          await calibrationUI.pauseCalibration(); break
    case 'resume':         await calibrationUI.resumeCalibration(); break
    case 'end':            await calibrationUI.endCalibration(); break
    case 'stop':           await calibrationUI.stopCalibration(); break
    case 'fit':            await calibrationUI.fitData(); break
    case 'reset':          calibrationStore.resetCollection(); break
    case 'alarm-continue': await calibrationUI.resolveAlarm('continue'); break
  }
}

// 副按钮分发：带 confirm 的先弹确认框
async function dispatchSecondary(action: SecondaryAction) {
  if (action.confirm) {
    try {
      await ElMessageBox.confirm(action.confirm, '操作确认', {
        confirmButtonText: '确认',
        cancelButtonText: '取消',
        type: 'warning'
      })
    } catch {
      // 用户取消，直接返回
      return
    }
  }

  switch (action.key) {
    case 'stop':            await calibrationUI.stopCalibration(); break
    case 'reset':           calibrationStore.resetCollection(); break
    case 'fit':             await calibrationUI.fitData(); break
    case 'alarm-skip':      await calibrationUI.resolveAlarm('skip'); break
    case 'alarm-recollect': await calibrationUI.resolveAlarm('recollect'); break
    case 'alarm-stop':      await calibrationUI.resolveAlarm('stop'); break
  }
}

// 设备级报警时跳过指定设备：打开原因选择弹窗，确认后永久从本批次剩余流程移除该设备。
function handleSkipDevice() {
  if (!alarmDeviceId.value) return
  showSkipDeviceDialog.value = true
}

async function handleSkipDeviceConfirm(reason: string) {
  const deviceId = alarmDeviceId.value
  if (!deviceId) return
  await calibrationUI.skipDevice(deviceId, reason)
}
</script>

<style scoped lang="scss">
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
$green: #22c55e;
$red: #ef4444;
$blue: #3b82f6;
$amber: #f59e0b;

/* 自给自足的卡片样式：拆解 card-block 嵌套后，Control 自身承载卡片视觉 */
.control-card {
  position: relative;
  background: #ffffff;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  padding: 16px;
  display: flex;
  flex-direction: column;
  font-family: $font-sans;
  overflow: hidden;
  transition: box-shadow 0.2s ease;

  &:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }
}

/* 顶部 1px mint 强调线，与 Params 卡片保持视觉一致 */
.control-card::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 1px;
  background: $mint;
}

/* ── 进度 + 操作按钮 并列 ── */
.control-main {
  display: flex;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
}

.progress-group {
  flex: 0 0 200px;
  display: flex;
  flex-direction: column;
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

.stable-label {
  font-size: 11px;
  color: $mint;
  font-weight: 600;
}

.progress-track {
  width: 100%;
  height: 6px;
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

.action-group {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: 8px;
  flex: 1;
  min-width: 280px;
}

.ctrl-btn {
  padding: 8px 16px;
  border-radius: 8px;
  border: none;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
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
  min-width: 120px;
  justify-content: center;
}

/* el-tooltip 包裹层：让 disabled 按钮也能触发 tooltip */
.primary-btn-wrapper {
  display: inline-flex;
}

/* Primary：渐变 + 白色文字 + Mint 阴影 */
.btn-mint {
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
.btn-slate {
  background: rgba(55, 65, 81, 0.08);
  color: $slate-700;
  border: 1px solid $slate-200;

  &:hover:not(:disabled) {
    background: rgba(55, 65, 81, 0.14);
    border-color: $slate-300;
  }
}

/* Blue：信息主色 */
.btn-blue {
  background: rgba(59, 130, 246, 0.1);
  color: $blue;
  border: 1px solid rgba(59, 130, 246, 0.2);

  &:hover:not(:disabled) {
    background: rgba(59, 130, 246, 0.16);
    border-color: rgba(59, 130, 246, 0.35);
  }
}

/* Amber：警告色，用于报警确认/重试 */
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
.btn-red {
  background: rgba(239, 68, 68, 0.1);
  color: $red;
  border: 1px solid rgba(239, 68, 68, 0.2);

  &:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.16);
    border-color: rgba(239, 68, 68, 0.35);
  }
}

@media (max-width: 900px) {
  .control-main {
    flex-direction: column;
    align-items: stretch;
  }

  .progress-group {
    width: 100%;
  }

  .action-group {
    width: 100%;
  }
}
</style>
