<template>
  <PageLayout>
    <!-- ═══ 仪表盘头部 ═══ -->
    <!-- P2-5/P2-11：role=banner 标识页面横幅；header-telemetry 内按语义拆成
         "压力读数" 与 "稳定性状态" 两组，role=group + aria-label 让屏幕阅读器
         能识别分组结构。 -->

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
          计量工作台
        </h1>
        <span
          class="state-chip"
          :class="stateClass"
          role="status"
          aria-live="polite"
          :aria-label="`会话状态：${stateLabel}`"
        >{{ stateLabel }}</span>
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
            <span class="telem-unit">{{ measurementStore.measureUnit || 'MPa' }}</span>
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
              :class="measurementStore.isStable ? 'on' : 'off'"
              :aria-label="measurementStore.isStable ? '压力已稳定' : '压力稳定中'"
            >
              <span
                class="telem-dot"
                aria-hidden="true"
              />
              {{ measurementStore.isStable ? '已稳定' : '稳定中' }}
            </span>
          </div>
          <span
            class="telem-divider telem-divider--inner"
            aria-hidden="true"
          />
          <div class="telem-cell">
            <span class="telem-label">稳定计时</span>
            <!-- 稳定等待时数字脉冲（P2-1） -->
            <span
              class="telem-value mono"
              :class="{ 'pulsing': !measurementStore.isStable }"
              :aria-label="`稳定计时 ${stableSeconds} 秒`"
            >{{ stableSeconds }}<small>s</small></span>
          </div>
        </div>
      </div>
    </header>

    <!-- ═══ 工作台主体 ═══ -->
    <div
      class="workbench"
      :class="{ 'sidebar-collapsed': sidebarCollapsed }"
    >
      <MeasurementSidebar
        ref="sidebarRef"
        :collapsed="sidebarCollapsed"
        @toggle="sidebarCollapsed = !sidebarCollapsed"
      />

      <main class="workbench-main">
        <!-- 报警横幅：alarmPending 时顶部红色提示条，与标定模块视觉一致。
             非确认模式下 ElMessage 会短暂提示，但横幅提供持续可见的报警状态。 -->
        <div
          v-if="measurementStore.alarmPending && measurementStore.alarmData"
          class="alarm-banner"
          role="alert"
        >
          <span class="alarm-dot" />
          <span>
            通道 {{ measurementStore.alarmData.overLimitChannels.join(', ') }} 超限报警，
            最大偏差 {{ (measurementStore.alarmData.maxDeviation * 100).toFixed(2) }}%
          </span>
        </div>

        <!-- 采集通道选择弹窗：由 MeasurementControl 内的"通道"按钮触发。
             通道选择是工作台级配置，但入口已合并到控制卡片首行，避免独立工具条占行。 -->
        <ChannelSelectDialog
          v-model:visible="channelDialogVisible"
          :selected-channels="measurementStore.channels"
          @confirm="handleChannelConfirm"
        />

        <!-- 分批模式：顶部统一工具条 + 阶段内容。
             所有阶段共用 BatchToolbar 提供"分批模式"标识、阶段指示器、通道入口、退出按钮，
             避免某些阶段不渲染 MeasurementControl 导致用户卡死。 -->
        <div
          v-if="batchMode"
          class="scroll-container batch-mode-container"
        >
          <BatchToolbar
            :steps="batchSteps"
            :current-key="batchCurrentKey"
            :done-keys="batchDoneKeys"
            :channel-count="measurementStore.channels.length"
            @exit-batch="toggleBatchMode"
            @open-channel-dialog="channelDialogVisible = true"
          />

          <BatchProgressBar
            v-if="batch.phase !== 'range-input' && batch.phase !== 'group-view'"
            :batches="batchStore.batches"
            :current-batch-index="batchStore.currentBatchIndex"
            @select="batch.handleBatchSelect"
            @reset="batch.handleBatchReset"
          />

          <!-- 量程录入阶段 -->
          <BatchRangeInput
            v-if="batch.phase === 'range-input'"
            @confirm="batch.handleRangeConfirm"
          />

          <!-- 分组确认阶段 -->
          <BatchGroupView
            v-else-if="batch.phase === 'group-view'"
            :channel-ranges="batchStore.channelRanges"
            @confirm="batch.handleGroupConfirm"
          />

          <!-- 批次执行阶段：复用现有加压控制与数据视图。
               分批开关与通道入口已上移到 BatchToolbar，
               此处 MeasurementControl 只负责单批采集控制，不再渲染分批开关。
               注意：MeasurementParamsPanel 必须保留，分批模式下每个批次仍需配置
               min/max/点数/采样次数/生成压力表，否则操作员无法启动批次采集。 -->
          <template v-else-if="batch.phase === 'batch-running'">
            <div class="card-block">
              <MeasurementControl
                :can-start="canStart"
                :is-stable="measurementStore.isStable"
                :stable-seconds="measurementStore.stabilityState.stableDurationMs / 1000"
                :has-pressure-device="hasPressureDevice"
                :exporting="isExporting"
                @start="handleStart"
                @pause="handlePause"
                @resume="handleResume"
                @stop="handleStop"
                @retry="handleRetry"
                @restart="handleRestart"
                @reset="handleReset"
                @export="handleExport"
                @view-error="handleViewError"
                @manual-start="handleManualStart"
                @manual-pressurize="handleManualPressurize"
              />
            </div>
            <div class="card-block">
              <MeasurementParamsPanel />
            </div>
            <div class="card-block card-block--data">
              <MeasurementDataView
                :control-mode="measurementStore.measurementParams.controlMode"
                @collect-point="handleCollectPoint"
              />
              <!-- 导出入口统一到数据区底部（P1-5） -->
              <div
                v-if="measurementStore.hasCompletedPoints"
                class="template-bar"
              >
                <el-icon><DocumentChecked /></el-icon>
                <span>已采集 {{ measurementStore.points.filter(p => p.status === 'completed').length }} 个测点，可导出报告</span>
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
            </div>
          </template>

          <!-- 全部完成阶段 -->
          <div
            v-else-if="batch.phase === 'completed'"
            class="batch-completed-panel"
          >
            <h3>所有批次已完成</h3>
            <p>共完成 {{ batchStore.completedCount }} / {{ batchStore.batches.length }} 批</p>
            <div class="action-row">
              <button
                class="primary-btn"
                @click="handleGenerateBatchReport"
              >
                生成合并报告
              </button>
              <button @click="batch.handleReset">
                重新开始
              </button>
            </div>
          </div>

          <!-- 报告预览阶段 -->
          <BatchReportView
            v-else-if="batch.phase === 'report' && batch.reportData"
            :report-data="batch.reportData"
            @reset="batch.handleReset"
          />

          <!-- 核对码弹窗。
            仅在 batch-running 阶段且 pendingVerificationBatch 非 null 时渲染，
            避免 pendingVerificationBatch 为 null 时因 props.batch 必填抛错。
          -->
          <BatchVerificationDialog
            v-if="batch.verificationDialogVisible && batch.pendingVerificationBatch"
            ref="batchDialogRef"
            :visible="batch.verificationDialogVisible"
            :batch="batch.pendingVerificationBatch"
            @verified="batch.handleVerification"
            @cancel="batch.handleVerificationCancel"
          />
        </div>

        <!-- 常规模式（原有逻辑）。
             顶部 mode-toolbar 已合并到 MeasurementControl 首行；
             AlarmConfigPanel 已合并到 MeasurementParamsPanel 末尾。 -->
        <div
          v-else
          class="scroll-container"
        >
          <MeasurementControl
            :can-start="canStart"
            :is-stable="measurementStore.isStable"
            :stable-seconds="measurementStore.stabilityState.stableDurationMs / 1000"
            :has-pressure-device="hasPressureDevice"
            :exporting="isExporting"
            :batch-mode="batchMode"
            :channel-count="measurementStore.channels.length"
            @toggle-batch="toggleBatchMode"
            @open-channel-dialog="channelDialogVisible = true"
            @start="handleStart"
            @pause="handlePause"
            @resume="handleResume"
            @stop="handleStop"
            @retry="handleRetry"
            @restart="handleRestart"
            @reset="handleReset"
            @export="handleExport"
            @view-error="handleViewError"
            @manual-start="handleManualStart"
            @manual-pressurize="handleManualPressurize"
          />
          <div class="section-gap" />
          <div class="card-block">
            <MeasurementParamsPanel />
          </div>
          <div class="card-block card-block--data">
            <MeasurementDataView
              :control-mode="measurementStore.measurementParams.controlMode"
              @collect-point="handleCollectPoint"
            />
            <!-- 导出入口统一到数据区底部（P1-5）：与标定模块 template-bar 同风格，
                 避免导出按钮散落在控制条里。 -->
            <div
              v-if="measurementStore.hasCompletedPoints"
              class="template-bar"
            >
              <el-icon><DocumentChecked /></el-icon>
              <span>已采集 {{ measurementStore.points.filter(p => p.status === 'completed').length }} 个测点，可导出报告</span>
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
          </div>
        </div>
      </main>
    </div>

    <AlarmConfirmDialog
      :visible="showAlarmDialog"
      :point="alarmPoint"
      :alarm="measurementStore.alarmData"
      @decision="handleAlarmDecision"
    />
    <SkipDeviceDialog
      v-model="showSkipDeviceDialog"
      :device-name="skipDeviceName"
      @confirm="handleSkipDeviceConfirm"
    />
  </PageLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, DocumentChecked, Download } from '@element-plus/icons-vue'
import { useMeasurementStore } from '@/stores/measurement'
import type { MeasurementState } from '@/stores/measurement/types'
import { useMeasurementDeviceStore } from '@/stores/measurement/deviceStore'
import { useMeasurementUI } from '@/composables/useMeasurementUI'
import { useMeasurementSync } from '@/composables/useMeasurementSync'
import { useWorkbenchShortcuts } from '@/composables/useWorkbenchShortcuts'
import { saveMeasurementAlarmConfig, exportMeasurementReport, resolveStabilityTimeout } from '@/api/measurement'
import { showSaveDialog } from '@/composables/useFileSaveDialog'
import { useBatchMeasurementStore } from '@/stores/batchMeasurement'
import { useBatchMeasurement, type BatchPhase } from '@/composables/useBatchMeasurement'
import PageLayout from '@/components/common/PageLayout.vue'
import ChannelSelectDialog from '@/components/common/ChannelSelectDialog.vue'
import MeasurementSidebar from '@/components/measurement/MeasurementSidebar.vue'
import MeasurementControl from '@/components/measurement/MeasurementControl.vue'
import MeasurementParamsPanel from '@/components/measurement/MeasurementParamsPanel.vue'
import MeasurementDataView from '@/components/measurement/MeasurementDataView.vue'
import AlarmConfirmDialog from '@/components/measurement/AlarmConfirmDialog.vue'
import SkipDeviceDialog from '@/components/common/SkipDeviceDialog.vue'
import BatchToolbar from '@/components/measurement/BatchToolbar.vue'
import BatchRangeInput from '@/components/measurement/BatchRangeInput.vue'
import BatchGroupView from '@/components/measurement/BatchGroupView.vue'
import BatchVerificationDialog from '@/components/measurement/BatchVerificationDialog.vue'
import BatchProgressBar from '@/components/measurement/BatchProgressBar.vue'
import BatchReportView from '@/components/measurement/BatchReportView.vue'

const router = useRouter()
const measurementStore = useMeasurementStore()
const deviceStore = useMeasurementDeviceStore()
const measurementUI = useMeasurementUI()

const sidebarCollapsed = ref(false)
const sidebarRef = ref()
const isExporting = ref(false)
// 采集通道选择弹窗：从 MeasurementControl 上移到工作台级工具条（P1-6 同行布局）
const channelDialogVisible = ref(false)

// 通道确认回调：写回 measurementStore.channels
function handleChannelConfirm(channels: number[]) {
  measurementStore.channels = channels
}

// 分批模式：开启后进入分批计量流程
const batchMode = ref<boolean>(false)
const batchStore = useBatchMeasurementStore()
const batch = useBatchMeasurement()

// 核对码弹窗组件引用：通过 watch 同步给 composable，
// 校验失败时由 composable 调用 dialog.setError() 在弹窗内联显示错误。
const batchDialogRef = ref<InstanceType<typeof BatchVerificationDialog> | null>(null)
watch(batchDialogRef, (inst) => {
  batch.setDialogRef(inst)
}, { immediate: true })

// 分批模式下监听 measurementStore.state：
// 当加压序列完成（state='completed'）时触发批次完成流程，
// 由 composable 决定是进入下一批还是切换到"全部完成"阶段。
//
// in-flight 标记防止重复触发：
// handleBatchComplete 是异步的，await 期间 measurementStore.state 可能再次变为 'completed'
// （如重置后重跑完成），导致同一批次被重复 completeBatch。
// 标记在批次开始处理时即记录，处理完成（无论成败）后释放。
const completingBatchIds = new Set<string>()
watch(
  () => measurementStore.state,
  (newState) => {
    if (!batchMode.value) return
    if (newState !== 'completed') return
    const current = batchStore.currentBatch
    if (!current) return
    // 防御：仅当当前批次仍为 running 时才触发完成
    if (current.status !== 'running') return
    // 防御：同一批次正在处理中则跳过，避免重复 completeBatch
    if (completingBatchIds.has(current.batchId)) return
    completingBatchIds.add(current.batchId)
    void batch.handleBatchComplete(current.batchId).finally(() => {
      completingBatchIds.delete(current.batchId)
    })
  }
)

// 生成合并报告：从 measurementStore.measurementParams 读取点数与压力模式，
// 避免硬编码魔法值。pressureMode 已是 'single' / 'roundTrip' 字符串字面量，
// 与后端 report.SelectTemplate 接受的 mode 参数一致。
const handleGenerateBatchReport = (): void => {
  const params = measurementStore.measurementParams
  const mode = params.pressureMode ?? 'single'
  const points = params.pointCount ?? 6
  void batch.handleGenerateReport(points, mode)
}

// 分批模式阶段定义：4 个有序步骤的静态描述。
// 阶段状态（active/done）由 batch.phase 和 batchStore.allBatchesCompleted 实时计算，
// 不再混入 step 定义，让数据形状与 StepIndicator 的 props 直接对齐。
// batch-running 与 completed 共享"批次执行"步骤，避免指示器来回跳。
interface BatchStep {
  key: string
  label: string
  index: number
}

const batchSteps: BatchStep[] = [
  { key: 'range-input', label: '量程录入', index: 1 },
  { key: 'group-view', label: '分组确认', index: 2 },
  { key: 'batch-running', label: '批次执行', index: 3 },
  { key: 'report', label: '合并报告', index: 4 }
]

// 当前阶段 key：直接用 batch.phase。
// 注意 phase='completed' 时 steps 里没有对应项，StepIndicator 不会渲染 active 步骤，
// 与原 batch-stepper 行为一致（completed 状态下只显示 done，不高亮任何步骤）。
const batchCurrentKey = computed(() => batch.phase)

// 分批模式开关切换：合并自原 mode-toolbar 的逻辑。
// 关闭时若已进入分批流程（phase ≠ range-input），需要先清理后端会话与本地 phase，
// 否则下次再开会直接跳进旧阶段，且后端 session 悬挂。
// 已采集的数据用确认框保护，避免误触丢失。
async function toggleBatchMode() {
  if (!batchMode.value) {
    // 关 → 开：直接进入分批模式
    batchMode.value = true
    return
  }
  // 开 → 关：若 phase 仍是初始 range-input（没录入量程/没创建会话），直接关
  if (batch.phase === 'range-input') {
    batchMode.value = false
    return
  }
  // 已进入流程：弹确认框避免误丢失
  try {
    await ElMessageBox.confirm(
      '退出分批模式将清空当前批次的进度与已采集数据，确定继续吗？',
      '退出分批模式',
      {
        confirmButtonText: '退出并清空',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch {
    // 用户取消：保持开关为开
    return
  }
  await batch.handleReset()
  batchMode.value = false
}

// 已完成步骤 key 列表：根据当前 phase 推导各步骤是否完成。
// 逻辑与原 batchSteps 中 done 字段保持一致，仅拆分到独立 computed 以适配 StepIndicator props。
const batchDoneKeys = computed<string[]>(() => {
  const phase = batch.phase as BatchPhase
  const batchesAllDone = batchStore.allBatchesCompleted
  const done: string[] = []
  if (phase !== 'range-input') done.push('range-input')
  if (['batch-running', 'completed', 'report'].includes(phase)) done.push('group-view')
  if (batchesAllDone) done.push('batch-running')
  if (phase === 'report') done.push('report')
  return done
})

// canStart：设备连接 + 计量绑定 + 点位已生成 + 阀门=校准模式（启动必要条件）。
// valveReady / enforceValveCalibrationGate 由 measurementStore 提供，与后端门禁同源。
const canStart = computed(() =>
  deviceStore.measureDevices.some(d => d.status === 'connected') &&
  deviceStore.pressureDevices.some(d => d.status === 'connected') &&
  measurementStore.deviceBound &&
  measurementStore.points.length > 0 &&
  measurementStore.canStart
)

// startBlockedReason：返回阻断"开始采集"的具体原因文案，
// 给 UI 在按钮 tooltip / 弹框中提示用户该如何修复。
// 所有 canStart 分支都在这里穷举，调用方无需再写 `|| '兜底'` fallback。
const startBlockedReason = computed<string>(() => {
  if (!deviceStore.measureDevices.some(d => d.status === 'connected')) return '请先连接计量设备'
  if (!deviceStore.pressureDevices.some(d => d.status === 'connected')) return '请先连接压力源设备'
  if (!measurementStore.deviceBound) return '请先绑定计量设备'
  if (measurementStore.points.length === 0) return '请先生成压力表'
  if (!measurementStore.unitConsistent) return '设备压力单位不一致，请先统一设备单位'
  if (!measurementStore.canStart) return '请先将阀门切换到校准模式'
  // 安全网：理论上 canStart=true 时该 computed 不会被读到；
  // 兜底文案让逻辑回归时仍有一条可读提示。
  return '设备状态异常，请检查后重试'
})

const hasPressureDevice = computed(() =>
  deviceStore.pressureDevices.some(d => d.status === 'connected')
)

onMounted(async () => {
  await deviceStore.loadDevices()
})

useMeasurementSync()

/* ── 状态 ── */
const STATE_LABELS: Record<MeasurementState, string> = {
  idle: '空闲',
  ready: '就绪',
  pressurizing: '打压中',
  stabilizing: '稳定中',
  collecting: '采集中',
  completed: '已完成',
  error: '错误',
  paused: '已暂停',
  stopped: '已停止'
}
const STATE_CLASSES: Record<MeasurementState, string> = {
  idle: 'chip-idle',
  ready: 'chip-running',
  pressurizing: 'chip-running',
  stabilizing: 'chip-running',
  collecting: 'chip-running',
  completed: 'chip-completed',
  error: 'chip-error',
  paused: 'chip-paused',
  stopped: 'chip-idle'
}

const stateLabel = computed(() => STATE_LABELS[measurementStore.state] || measurementStore.state)

const stateClass = computed(() => STATE_CLASSES[measurementStore.state] || '')

/* ── 遥测数据 ── */
const displayPressure = computed(() => {
  const v = measurementStore.currentPressure
  if (v === null || v === undefined) return '—'
  return Number(v).toFixed(3)
})

const stableSeconds = computed(() => {
  const ms = measurementStore.stabilityState.stableDurationMs
  return (ms / 1000).toFixed(1)
})

function goBack() { router.push('/') }

/* ── 采集控制 ── */
async function handleStart() {
  // 开始计量前刷新设备单位一致性，确保门禁基于最新单位状态。
  await measurementStore.refreshUnitConsistency()
  if (!canStart.value) { ElMessage.warning(startBlockedReason.value); return }
  await measurementUI.start(measurementStore.channels)
}

async function handlePause() { await measurementUI.pause() }
async function handleResume() { await measurementUI.start(measurementStore.channels) }
async function handleStop() { await measurementUI.stop() }

async function handleRetry() {
  if (!canStart.value) { ElMessage.warning(startBlockedReason.value); return }
  await measurementUI.start(measurementStore.channels)
}

async function handleRestart() {
  if (!canStart.value) { ElMessage.warning(startBlockedReason.value); return }
  measurementStore.resetCollection()
  await measurementUI.start(measurementStore.channels)
}

function handleViewError() {
  ElMessage.info('请检查设备连接和报警信息')
}

function handleReset() {
  measurementStore.resetCollection()
  ElMessage.info('采集数据已重置')
}

async function handleManualStart() {
  const hasMeasure = deviceStore.measureDevices.some(d => d.status === 'connected')
  if (!hasMeasure) { ElMessage.warning('请先连接计量设备'); return }
  if (measurementStore.points.length === 0) { ElMessage.warning('请先生成压力表'); return }
  if (!measurementStore.canStart) { ElMessage.warning('请先将阀门切换到校准模式'); return }
  await measurementUI.manualStart(measurementStore.channels)
}

async function handleManualPressurize() {
  const idx = measurementStore.currentPointIndex + 1
  await measurementUI.manualPressurize(idx)
}

async function handleCollectPoint(pointIndex: number) {
  await measurementUI.manualCollect(pointIndex)
}

/* ── 报警配置自动保存 ── */
let alarmSaveTimer: ReturnType<typeof setTimeout> | null = null
watch(
  () => [
    measurementStore.alarmConfig.enabled,
    measurementStore.alarmConfig.soundEnabled,
    measurementStore.alarmConfig.confirmOnAlarm,
    measurementStore.channels
  ],
  () => {
    if (alarmSaveTimer) clearTimeout(alarmSaveTimer)
    alarmSaveTimer = setTimeout(() => {
      const cfg = { ...measurementStore.alarmConfig }
      cfg.enabledChannels = [...measurementStore.channels]
      saveMeasurementAlarmConfig(cfg)
    }, 250)
  },
  { deep: true }
)

onUnmounted(() => {
  if (alarmSaveTimer) {
    clearTimeout(alarmSaveTimer)
    alarmSaveTimer = null
  }
})

/* ── 报警弹窗 ── */
const showAlarmDialog = computed(() =>
  measurementStore.alarmPending && measurementStore.alarmConfig.confirmOnAlarm
)

const alarmPoint = computed(() => {
  const d = measurementStore.alarmData
  if (!d) return undefined
  return measurementStore.points.find(p => p.id === d.pointId)
})

// 非确认模式：通知后自动继续采集，避免后端阻塞等待。
// 必须同时监听 confirmOnAlarm：报警挂起期间用户关闭"报警确认"开关时，
// 弹窗会消失但 alarmPending 不变，后端仍阻塞等待决策——若只监听
// alarmPending，watcher 不触发，自动流程卡死（只能重新开开关弹窗才能继续）。
watch(
  () => [measurementStore.alarmPending, measurementStore.alarmConfig.confirmOnAlarm] as const,
  async ([pending, confirmOn]) => {
    if (pending && !confirmOn && measurementStore.alarmData) {
      const a = measurementStore.alarmData
      ElMessage.warning(`报警：${a.overLimitChannels.length} 个通道精度超限，最大偏差 ${(a.maxDeviation * 100).toFixed(2)}%`)
      await measurementStore.resolveAlarm('continue')
    }
  }
)

// 稳定超时弹窗：让用户选择继续等待或跳过当前点。
// 关闭路径（Esc/X/遮罩）一律兜底按"继续等待"放行：后端收到 continue 会重置
// 超时倒计时，再次超时会重新发布事件、弹窗再次弹出；若不发送任何决策，
// 后端将永久阻塞在等待通道上，且 stabilityTimeoutPending 已复位导致弹窗
// 无法再次触发，自动流程卡死（与报警确认取消后卡死同构）。
watch(() => measurementStore.stabilityTimeoutPending, async (pending) => {
  if (!pending) return
  measurementStore.stabilityTimeoutPending = false
  try {
    await ElMessageBox.confirm(
      '当前压力点稳定等待超时，请选择处理方式：',
      '稳定超时',
      {
        confirmButtonText: '继续等待',
        cancelButtonText: '跳过此点',
        distinguishCancelAndClose: true,
        closeOnClickModal: false,
        closeOnPressEscape: false,
        showClose: false,
        type: 'warning'
      }
    )
    await resolveStabilityTimeout('continue')
  } catch (action: unknown) {
    if (typeof action === 'string' && action === 'cancel') {
      await resolveStabilityTimeout('skip')
    } else {
      // 异常关闭兜底（如路由切换强制关闭弹窗）：继续等待，可恢复
      await resolveStabilityTimeout('continue')
    }
  }
})

async function handleAlarmDecision(decision: 'continue' | 'recollect' | 'skip-device') {
  if (decision === 'skip-device') {
    // 设备级报警：打开跳过设备弹窗，由用户选择原因后永久跳过该设备。
    const deviceId = measurementStore.alarmData?.deviceId
    if (!deviceId) return
    skipDeviceId.value = deviceId
    showSkipDeviceDialog.value = true
    return
  }
  await measurementStore.resolveAlarm(decision)
}

// ── 跳过设备（设备级报警入口） ──
const showSkipDeviceDialog = ref(false)
const skipDeviceId = ref('')
const skipDeviceName = computed(() => {
  const dev = deviceStore.measureDevices.find(d => d.id === skipDeviceId.value)
  return dev?.name || dev?.model || skipDeviceId.value
})

async function handleSkipDeviceConfirm(reason: string) {
  const deviceId = skipDeviceId.value
  if (!deviceId) return
  try {
    await measurementStore.skipDevice(deviceId, reason)
    ElMessage.success(`已跳过设备 ${skipDeviceName.value}`)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : '跳过设备失败')
  }
}

/* ── 导出 ── */
async function handleExport() {
  const path = await showSaveDialog('measurement-report.xlsx', 'Excel 文件', '*.xlsx')
  if (!path) return
  isExporting.value = true
  try {
    const paths = await exportMeasurementReport(path)
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

// 快捷键（P1-8）：Space 开始/暂停、Esc 停止、Ctrl+E 导出、Ctrl+S 阻止默认保存。
// 分批模式下 Space/Esc 不触发（避免干扰批次流程），仅 Ctrl+E 导出可用。
useWorkbenchShortcuts({
  onSpace: () => {
    if (batchMode.value) return
    const state = measurementStore.state
    if (['idle', 'stopped', 'completed', 'paused'].includes(state)) {
      void handleStart()
    } else if (['pressurizing', 'stabilizing', 'collecting'].includes(state)) {
      void handlePause()
    }
  },
  onEscape: () => {
    if (batchMode.value) return
    void handleStop()
  },
  onExport: () => {
    if (measurementStore.hasCompletedPoints) {
      void handleExport()
    }
  }
})
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
$slate-900: #111827;
$green: #22c55e;
$red: #ef4444;
$amber: #f59e0b;

/* ═══ 仪表盘头部 ═══ */
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

.header-nav { display: flex; align-items: center; }

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

/* 工作区顶部工具条已合并到 MeasurementControl 首行，
   原独立 mode-toolbar / batch-mode-switch / channel-select-btn 样式已移除。
   分批模式顶部工具条由 BatchToolbar 组件承担，样式封装在组件内。 */

.batch-mode-container {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0;
}

.batch-completed-panel {
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  padding: 24px;
  text-align: center;
}

.batch-completed-panel h3 {
  margin: 0 0 8px;
  font-size: 18px;
}

.batch-completed-panel p {
  margin: 0 0 16px;
  color: $slate-500;
}

/* 操作按钮行：统一项目风格 — primary 用 mint 渐变，次按钮用 slate 半透明 */
.action-row {
  display: flex;
  gap: 8px;
  justify-content: center;
}

.action-row button {
  padding: 8px 16px;
  border: 1px solid $slate-200;
  border-radius: 8px;
  background: rgba(55, 65, 81, 0.08);
  color: $slate-700;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  transition: all 0.15s ease;

  &:hover {
    background: rgba(55, 65, 81, 0.14);
    border-color: $slate-300;
  }

  &:active {
    transform: translateY(1px);
  }
}

.action-row button.primary-btn {
  min-width: 120px;
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  border-color: $mint;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);

  &:hover {
    background: linear-gradient(135deg, $mint-light, $mint);
    box-shadow: 0 4px 12px rgba(16, 185, 129, 0.4);
    transform: translateY(-1px);
  }
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

/* P2-5：telem-group 把语义相关的 telem-cell 聚成一束，组内 gap 收紧到 8px */
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

/* ═══ 工作台 ═══ */
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

.workbench-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
  z-index: 1;
}

/* ── 报警横幅：与标定模块同风格 ── */
.alarm-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
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

.scroll-container {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0;

  &::-webkit-scrollbar { width: 4px; }
  &::-webkit-scrollbar-thumb { background: $slate-300; border-radius: 2px; }
  &::-webkit-scrollbar-track { background: transparent; }
}

.section-gap {
  flex-shrink: 0;
  height: 4px;
}

.card-block {
  background: #ffffff;
  border-radius: 10px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06), 0 1px 2px rgba(0, 0, 0, 0.04);
  position: relative;
  overflow: hidden;
  animation: card-enter 0.35s ease both;
}

.card-block--data {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 导出报告工具条：与标定模块 template-bar 同风格（P1-5） */
.template-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px 14px;
  font-size: 13px;
  font-weight: 500;
  color: $slate-500;
  font-family: $font-sans;
  border-top: 1px solid $slate-100;
  flex-shrink: 0;

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

  &:hover:not(:disabled) {
    background: rgba(16, 185, 129, 0.06);
    color: $mint-dark;
    border-color: $mint;
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .el-icon { color: currentColor; font-size: 14px; }
}

@keyframes card-enter {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 响应式 */
@media (max-width: 1024px) {
  .header-telemetry { gap: 10px; }
  .telem-label { display: none; }
}

@media (max-width: 768px) {
  .instrument-header { height: auto; flex-wrap: wrap; padding: 10px 16px; gap: 8px; }
  .header-telemetry { flex-wrap: wrap; margin-left: 0; width: 100%; }
  .telem-divider { display: none; }
  .workbench { flex-direction: column; }
}
</style>
