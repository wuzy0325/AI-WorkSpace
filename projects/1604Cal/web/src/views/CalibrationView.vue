<template>
  <PageLayout>
    <!-- ═══ 仪表盘头部 ═══ -->
    <!-- P2-5/P2-11：role=banner 标识页面横幅；header-telemetry 内按语义拆成
         "压力读数" 与 "稳定性状态" 两组，role=group + aria-label 让屏幕阅读器
         能识别分组结构，避免连续读出 4 个无关联的 telem-cell。 -->
    <header
      class="instrument-header"
      role="banner"
    >
      <div class="header-nav">
        <button
          class="back-btn"
          type="button"
          aria-label="返回首页"
          @click="goBack"
        >
          <el-icon><ArrowLeft /></el-icon>
        </button>
      </div>

      <div class="header-identity">
        <h1 class="header-title">
          标定工作台
        </h1>
        <!-- 状态芯片：role=status + aria-live=polite，状态变化时由屏幕阅读器播报 -->
        <span
          class="state-chip"
          :class="stateClass"
          role="status"
          aria-live="polite"
          :aria-label="`会话状态：${stateLabel}`"
        >
          <span
            class="chip-dot"
            aria-hidden="true"
          />
          {{ stateLabel }}
        </span>
      </div>

      <div class="header-telemetry">
        <div
          class="telem-group"
          role="group"
          aria-label="压力读数"
        >
          <div class="telem-cell">
            <span class="telem-label">当前压力</span>
            <span class="telem-value mono">{{ displayPressure }}</span>
            <span class="telem-unit">{{ unitLabel }}</span>
          </div>
        </div>
        <span
          class="telem-divider"
          aria-hidden="true"
        />
        <div
          class="telem-group"
          role="group"
          aria-label="稳定性状态"
        >
          <div class="telem-cell">
            <span class="telem-label">稳定性</span>
            <span
              class="telem-indicator"
              :class="stabilityStatus?.isStable ? 'on' : 'off'"
              :aria-label="stabilityStatus?.isStable ? '压力已稳定' : '压力稳定中'"
            >
              <span
                class="telem-dot"
                aria-hidden="true"
              />
              {{ stabilityStatus?.isStable ? '已稳定' : '稳定中' }}
            </span>
          </div>
          <span
            class="telem-divider telem-divider--inner"
            aria-hidden="true"
          />
          <div class="telem-cell">
            <span class="telem-label">稳定计时</span>
            <!-- 稳定等待时数字脉冲（P2-1），让用户感知系统正在等待压力稳定 -->
            <span
              class="telem-value mono"
              :class="{ 'pulsing': !stabilityStatus?.isStable }"
              :aria-label="`稳定计时 ${stableSeconds} 秒`"
            >{{ stableSeconds }}<small>s</small></span>
          </div>
          <span
            class="telem-divider telem-divider--inner"
            aria-hidden="true"
          />
          <div class="telem-cell">
            <span class="telem-label">偏差</span>
            <span
              class="telem-value mono"
              :class="deviationClass"
            >{{ deviationDisplay }}</span>
          </div>
        </div>
      </div>
    </header>

    <!-- ═══ 工作台 ═══ -->
    <div
      class="workbench"
      :class="{ 'sidebar-collapsed': sidebarCollapsed }"
    >
      <CalibrationSidebar
        :collapsed="sidebarCollapsed"
        @toggle="sidebarCollapsed = !sidebarCollapsed"
      />

      <main class="workbench-main">
        <div
          v-if="alarmEvent"
          class="alarm-banner"
        >
          <span class="alarm-dot" />
          <span>通道 {{ alarmEvent.overLimitChannels?.join(', ') }} 超限报警</span>
        </div>

        <div class="scroll-container">
          <ProgressIndicator :current-step="calibrationStore.currentStep" />

          <!-- 拆解卡片嵌套：Params 与 Control 各自承载卡片视觉，外层不再包裹 card-block。
               scroll-container 自带 gap: 8px，无需额外的 section-gap 分隔元素 -->
          <CalibrationParams />

          <CalibrationControl />

          <section class="card-block card-block-data">
            <div class="card-accent" />
            <CalibrationDataView />
            <div class="template-bar">
              <el-icon v-if="dialogsRef?.templateFilename">
                <DocumentChecked />
              </el-icon>
              <span v-if="dialogsRef?.templateFilename">当前报告模板：{{ dialogsRef.templateFilename }}</span>
              <button
                type="button"
                class="export-btn"
                :disabled="isExporting"
                @click="handleExport"
              >
                <el-icon><Download /></el-icon>
                {{ isExporting ? '导出中...' : '导出报告' }}
              </button>
            </div>
          </section>
        </div>
      </main>
    </div>

    <CalibrationDialogs ref="dialogsRef" />
  </PageLayout>
</template>

<script setup lang="ts">
import { ref, computed, provide } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, DocumentChecked, Download } from '@element-plus/icons-vue'
import type { SessionState } from '@/types/calibration'
import { ElMessage } from 'element-plus'
import { useCalibrationStore } from '@/stores/calibration'
import { useCalibrationSync, stabilityStatusKey, alarmEventKey } from '@/composables/useCalibrationSync'
import { useConfigPersistence } from '@/composables/useConfigPersistence'
import { useWorkbenchShortcuts } from '@/composables/useWorkbenchShortcuts'
import { useCalibrationUI } from '@/composables/useCalibrationUI'
import { showSaveDialog } from '@/composables/useFileSaveDialog'
import { exportCalibrationReport } from '@/api/calibration'
import PageLayout from '@/components/common/PageLayout.vue'
import CalibrationSidebar from '@/components/calibration/CalibrationSidebar.vue'
import CalibrationParams from '@/components/calibration/CalibrationParams.vue'
import CalibrationControl from '@/components/calibration/CalibrationControl.vue'
import CalibrationDataView from '@/components/calibration/CalibrationDataView.vue'
import CalibrationDialogs from '@/components/calibration/CalibrationDialogs.vue'
import ProgressIndicator from '@/components/calibration/ProgressIndicator.vue'

const router = useRouter()
const calibrationStore = useCalibrationStore()
const calibrationUI = useCalibrationUI()
const sidebarCollapsed = ref(false)
const dialogsRef = ref<InstanceType<typeof CalibrationDialogs>>()
const isExporting = ref(false)

const { stabilityStatus, alarmEvent } = useCalibrationSync()
provide(stabilityStatusKey, stabilityStatus)
provide(alarmEventKey, alarmEvent)
useConfigPersistence()

// 快捷键（P1-8）：Space 开始/暂停、Esc 停止、Ctrl+E 导出、Ctrl+S 阻止默认保存
useWorkbenchShortcuts({
  onSpace: () => {
    const state = calibrationStore.sessionState
    if (['idle', 'ready', 'stopped', 'paused'].includes(state)) {
      void calibrationUI.startCalibration()
    } else if (['pressurizing', 'stabilizing', 'collecting', 'point_done'].includes(state)) {
      void calibrationUI.pauseCalibration()
    }
  },
  onEscape: () => {
    void calibrationUI.stopCalibration()
  },
  onExport: () => {
    void handleExport()
  }
})

function goBack(): void {
  router.push('/')
}

/* ── 会话状态 ── */
const STATE_LABELS: Record<SessionState, string> = {
  idle: '空闲',
  ready: '就绪',
  pressurizing: '打压中',
  stabilizing: '稳定中',
  collecting: '采集中',
  point_done: '点完成',
  fitting: '拟合中',
  completed: '已完成',
  paused: '已暂停',
  stopped: '已停止',
  await_manual_collect: '等待手动采集',
  await_alarm_resolution: '等待报警处理',
  recovering: '恢复中',
  error: '错误'
}
const STATE_CLASSES: Record<SessionState, string> = {
  idle: 'chip-idle',
  ready: 'chip-running',
  pressurizing: 'chip-running',
  stabilizing: 'chip-running',
  collecting: 'chip-running',
  point_done: 'chip-running',
  fitting: 'chip-running',
  completed: 'chip-completed',
  paused: 'chip-paused',
  stopped: 'chip-idle',
  await_manual_collect: 'chip-running',
  await_alarm_resolution: 'chip-running',
  recovering: 'chip-running',
  error: 'chip-error'
}

const stateLabel = computed(() => STATE_LABELS[calibrationStore.sessionState] || calibrationStore.sessionState)

const stateClass = computed(() => STATE_CLASSES[calibrationStore.sessionState] || '')

/* ── 压力数据 ── */
const displayPressure = computed(() => {
  const v = calibrationStore.currentPressure
  if (v === null || v === undefined) return '—'
  const n = typeof v === 'number' ? v : Number(v)
  return isNaN(n) ? String(v) : n.toFixed(3)
})

const unitLabel = computed(() => calibrationStore.measureUnit || 'MPa')

/* ── 稳定性 ── */
const stableSeconds = computed(() => {
  if (!stabilityStatus.value) return '0.0'
  return (stabilityStatus.value.stableDurationMs / 1000).toFixed(1)
})

const deviationDisplay = computed(() => {
  if (!stabilityStatus.value) return '—'
  return stabilityStatus.value.deviation.toFixed(4)
})

const deviationClass = computed(() => {
  if (!stabilityStatus.value) return ''
  const d = Math.abs(stabilityStatus.value.deviation)
  if (d < 0.01) return 'dev-good'
  if (d < 0.1) return 'dev-warn'
  return 'dev-bad'
})

/* ── 导出报告 ── */
async function handleExport() {
  const path = await showSaveDialog('calibration_report.xlsx', 'Excel 文件', '*.xlsx')
  if (!path) return
  isExporting.value = true
  try {
    const paths = await exportCalibrationReport(path)
    if (paths.length > 1) {
      ElMessage.success(`报告导出成功，共 ${paths.length} 个文件（每台设备一个）`)
    } else {
      ElMessage.success('报告导出成功')
    }
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '报告导出失败')
  } finally {
    isExporting.value = false
  }
}
</script>

<style scoped lang="scss">
/* ── 设计令牌 ── */
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
$slate-900: #111827;
$green: #22c55e;
$red: #ef4444;
$amber: #f59e0b;

/* ════════════════════════════════════════
   仪表盘头部
   ════════════════════════════════════════ */
.instrument-header {
  display: flex;
  align-items: center;
  gap: 20px;
  flex-shrink: 0;
  height: 56px;
  padding: 0 24px;
  background: $slate-50;
  border-bottom: 1px solid $slate-200;
  font-family: $font-sans;
}

.header-nav {
  display: flex;
  align-items: center;
}

.back-btn {
  width: 32px; height: 32px;
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  color: $slate-500;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease;

  &:hover {
    background: #fff;
    color: $mint;
    border-color: $mint;
  }
}

.header-identity {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.header-title {
  font-size: 20px;
  font-weight: 600;
  color: $slate-800;
  margin: 0;
  font-family: $font-sans;
}

.header-telemetry {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-left: auto;
}

/* P2-5：telem-group 把语义相关的 telem-cell 聚成一束，组内 gap 收紧到 8px
   让"压力读数""稳定性状态"在视觉上明显分成两块，便于用户快速扫描。 */
.telem-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.telem-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.telem-label {
  font-size: 12px;
  font-weight: 500;
  color: $slate-400;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  white-space: nowrap;
}

.telem-value {
  font-size: 14px;
  font-weight: 600;
  color: $slate-800;

  &.mono { font-family: $font-mono; }
  small { font-size: 10px; color: $slate-400; margin-left: 1px; }
}

.telem-unit {
  font-size: 12px;
  color: $slate-400;
  font-weight: 500;
}

.telem-divider {
  width: 1px;
  height: 24px;
  background: $slate-200;
}

/* P2-5：组内分隔线更弱，避免与组间分隔线视觉同级 */
.telem-divider--inner {
  height: 16px;
  background: $slate-100;
}

.telem-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 4px;

  &.on { color: $mint-dark; background: rgba(16, 185, 129, 0.08); }
  &.off { color: $amber; background: rgba(245, 158, 11, 0.08); }
}

.telem-dot {
  width: 6px; height: 6px; border-radius: 50%;

  .on & { background: $mint; box-shadow: 0 0 4px rgba(16, 185, 129, 0.4); animation: pulse-dot 2s ease-in-out infinite; }
  .off & { background: $amber; box-shadow: 0 0 4px rgba(245, 158, 11, 0.4); animation: pulse-dot 1.2s ease-in-out infinite; }
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.85); }
}

/* 稳定等待时计时数字脉冲（P2-1） */
@keyframes pulse-value {
  0%, 100% { color: $slate-800; }
  50% { color: $amber; }
}

.pulsing {
  animation: pulse-value 1.2s ease-in-out infinite;
}

/* ── 状态芯片 ── */
.state-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.05em;

  .chip-dot { width: 5px; height: 5px; border-radius: 50%; }

  &.chip-idle { background: rgba(156,163,175,0.1); color: $slate-500; .chip-dot { background: $slate-400; } }
  &.chip-preparing, &.chip-running { background: rgba(16,185,129,0.08); color: $mint-dark; .chip-dot { background: $mint; box-shadow: 0 0 4px rgba(16,185,129,0.3); } }
  &.chip-paused { background: rgba(245,158,11,0.08); color: $amber; .chip-dot { background: $amber; } }
  &.chip-completed { background: rgba(16,185,129,0.08); color: $mint-dark; .chip-dot { background: $mint; } }
  &.chip-error { background: rgba(239,68,68,0.08); color: $red; .chip-dot { background: $red; } }
}

/* ════════════════════════════════════════
   工作台主体
   ════════════════════════════════════════ */
.workbench {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 280px 1fr;
  gap: 16px;
  overflow: hidden;
  position: relative;
  padding: 4px 24px 24px;
  transition: grid-template-columns 250ms cubic-bezier(0.4, 0, 0.2, 1);

  &.sidebar-collapsed {
    grid-template-columns: 32px 1fr;
  }

  // 坐标纸背景
  &::before {
    content: '';
    position: absolute;
    inset: 0;
    background-image:
      linear-gradient(rgba(16, 185, 129, 0.04) 1px, transparent 1px),
      linear-gradient(90deg, rgba(16, 185, 129, 0.04) 1px, transparent 1px);
    background-size: 24px 24px;
    pointer-events: none;
    z-index: 0;
  }
}

/* ── 中央工作区 ── */
.workbench-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
  z-index: 1;
}

.scroll-container {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 4px 4px 4px 0;

  &::-webkit-scrollbar { width: 4px; }
  &::-webkit-scrollbar-thumb {
    background: $slate-300;
    border-radius: 2px;
  }
  &::-webkit-scrollbar-track { background: transparent; }
}

/* ── 卡片区块 ── */
.card-block {
  background: #ffffff;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  position: relative;
  overflow: hidden;
  animation: card-enter 0.35s ease both;
}

.card-block:nth-child(1) { animation-delay: 0ms; }
.card-block:nth-child(2) { animation-delay: 60ms; }

.card-accent {
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 1px;
  background: $mint;
}

@keyframes card-enter {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

.template-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px 14px;
  font-size: 13px;
  font-weight: 500;
  color: $slate-500;
  font-family: $font-sans;

  .el-icon { color: $mint; font-size: 16px; }
}

.export-btn {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border-radius: 8px;
  border: 1px solid $slate-200;
  background: #fff;
  color: $slate-700;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  cursor: pointer;
  transition: all 0.15s ease;

  &:hover {
    background: rgba(16, 185, 129, 0.06);
    color: $mint-dark;
    border-color: $mint;
  }

  .el-icon { color: currentColor; font-size: 14px; }
}

/* ── 偏差颜色 ── */
.dev-good { color: $mint-light !important; }
.dev-warn { color: $amber !important; }
.dev-bad { color: $red !important; }

/* ── 报警横幅 ── */
.alarm-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: rgba(239, 68, 68, 0.06);
  border-bottom: 1px solid rgba(239, 68, 68, 0.15);
  font-size: 13px;
  font-weight: 600;
  color: $red;
  flex-shrink: 0;
  animation: card-enter 0.3s ease both;
}

.alarm-dot {
  width: 7px; height: 7px;
  border-radius: 50%;
  background: $red;
  animation: pulse-dot 1s ease-in-out infinite;
}

/* ════════════════════════════════════════
   响应式
   ════════════════════════════════════════ */
@media (max-width: 1024px) {
  .instrument-header {
    gap: 12px;
    padding: 0 16px;
  }
  .header-telemetry { gap: 10px; }
  .telem-label { display: none; }
}

@media (max-width: 768px) {
  .instrument-header {
    height: auto;
    flex-wrap: wrap;
    padding: 10px 16px;
    gap: 8px;
  }
  .header-telemetry { flex-wrap: wrap; margin-left: 0; width: 100%; }
  .telem-divider { display: none; }
  .workbench { flex-direction: column; }
  .workbench-main { min-height: 400px; }
}
</style>
