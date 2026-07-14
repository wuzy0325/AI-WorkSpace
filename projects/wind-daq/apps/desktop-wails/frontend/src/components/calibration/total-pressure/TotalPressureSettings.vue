<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore } from '@stores/storageStore'
import { buildCalibrationCsvName, joinCalibrationPath } from '@shared/calibrationCsvPath'
import { calibrationApi } from '@api/calibrationApi'
import type { CalibrationConfig, ProbeChannelConfig, MotionAxisConfig, TotalPressurePointLayout, ChannelRef } from '@shared/types/calibration'
import { applyCalibrationPrecisionDefaults, DEFAULT_CALIBRATION_PROBE_PRECISION } from '@shared/calibrationPrecision'
import UiButton from '@components/ui/UiButton.vue'
import UiAlert from '@components/ui/UiAlert.vue'
import { reportAllSettledFailures } from '@utils/allSettledReport'
import UiPanel from '@components/ui/UiPanel.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiDialog from '@components/ui/UiDialog.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiSpin from '@components/ui/UiSpin.vue'
import UiStep from '@components/ui/UiStep.vue'
import UiSteps from '@components/ui/UiSteps.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import { useChannelBatchOperations } from '@composables/useChannelBatchOperations'

const emit = defineEmits<{ close: []; saved: [config: CalibrationConfig] }>()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()
const i18n = useI18nStore()
const t = computed(() => i18n.t)
const storageStore = useStorageStore()

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = computed(() => [t.value.stepBasic, t.value.stepHardware, t.value.stepConfirm])

const pointLayout = ref<TotalPressurePointLayout>({ alphaMin: -30, alphaMax: 30, alphaStep: 5 })
const pointCount = computed(() => {
  if (pointLayout.value.alphaStep <= 0) return 0
  return Math.floor((pointLayout.value.alphaMax - pointLayout.value.alphaMin) / pointLayout.value.alphaStep) + 1
})
const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const calibrationName = ref(`${t.value.tp_defaultCalibNamePrefix}-${new Date().toISOString().slice(0, 10)}`)
// CSV 保存路径：自动校准启动时后端按此路径覆盖初始化 csvWriter，逐点实时写入，
// 崩溃/断电不丢已采集点。空字符串将导致后端跳过实时写入（仅靠校准结束全量导出）。
const savePath = ref('')
const sphereTankGateEnabled = ref(false)
const sphereTankWaitTimeSec = ref(3)
const sphereTankTimeoutSec = ref(300)
// 默认 channelIndex=-1 表示"未分配"，强制操作员手动选择，
// 避免静默使用 CH0（可能是风洞总压通道）作为球罐稳定时间通道。
const sphereTankStableChannel = ref<ChannelRef>({ deviceId: '', channelIndex: -1 })

// 通道角色必须与后端 total_pressure.go ValidateConfig / read_probe_channels.go roleMap 严格一致。
// 旧版本曾错误使用 totalPressure.pTotal / totalPressure.pStatic，已被弃用，
// 后端会拒绝接收这两个角色。
const probeChannels = ref<ProbeChannelConfig[]>([
  { name: t.value.tp_probeTotal, role: 'totalPressure.pProbeTotal', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.fiveHolePTotal, role: 'totalPressure.pTunnelTotal', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.fiveHolePTunnelStatic, role: 'totalPressure.pTunnelStatic', channel: { deviceId: '', channelIndex: 2 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.Patm, role: 'totalPressure.pAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value.Tatm, role: 'totalPressure.tAtm', channel: { deviceId: '', channelIndex: 17 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
])

const motionAxes = ref<MotionAxisConfig[]>([{ name: 'Alpha', controllerId: '', axis: 'X' }])
const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)
// 与后端 total_pressure.go requiredRoles 保持同步：4 个必需角色 + 1 个可选 tAtm
const REQUIRED_CHANNEL_ROLES = ['totalPressure.pProbeTotal', 'totalPressure.pTunnelTotal', 'totalPressure.pTunnelStatic', 'totalPressure.pAtm'] as const

// 批量操作：统一选择设备 + 通道号自动递增填充（仿照遍历测试模块）
const {
  batchDeviceId,
  autoFillStartIndex,
  applyDeviceToAll,
  autoFillChannelIndices,
} = useChannelBatchOperations(probeChannels)

// 设备下拉选项（批量工具栏与单通道选择共用）
const deviceOptions = computed(() =>
  deviceList.value.map((d) => ({ label: `${d.name} (${d.type})`, value: d.id })),
)

// 通道索引枚举选项：UI 显示 CH1~CH18（1-based），内部 value 仍为数组索引 0~17
// 通道序号从 1 开始更符合操作员直觉，对应底层数组的 0-based 索引
const channelIndexOptions = Array.from({ length: 18 }, (_, i) => ({ label: `CH${i + 1}`, value: String(i) }))

const currentStepErrors = computed<string[]>(() => {
  const errs: string[] = []
  if (currentStep.value === 0) {
    if (!calibrationName.value.trim()) errs.push(t.value.enterConfigName)
    if (pointLayout.value.alphaStep <= 0) errs.push(t.value.tp_stepMustPositive)
    if (pointLayout.value.alphaMax <= pointLayout.value.alphaMin) errs.push(t.value.maxGreaterThanMin)
    if (dwellTimeMs.value < 100) errs.push(t.value.tp_dwellTimeAtLeast100Ms)
    if (samplesPerPoint.value < 1) errs.push(t.value.tp_samplesAtLeast1)
    if (!savePath.value.trim()) errs.push(t.value.tp_pleaseSelectCsvPath)
  }
  if (currentStep.value === 1) {
    // 校验所有必需角色均已映射且通道索引有效
    const enabledRoles = new Set(
      probeChannels.value
        .filter(ch => ch.enabled && ch.channel.deviceId && ch.channel.channelIndex >= 0)
        .map(ch => ch.role),
    )
    for (const role of REQUIRED_CHANNEL_ROLES) {
      if (!enabledRoles.has(role)) {
        errs.push(t.value.tp_channelMapIncompleteMissing.replace('{role}', role))
        break
      }
    }
    if (motionAxes.value.some(a => !a.controllerId)) errs.push(t.value.tp_motionAxisMustBindController)
    if (sphereTankGateEnabled.value) {
      if (!sphereTankStableChannel.value.deviceId) errs.push(t.value.tp_sphereEnabledNoDevice)
      if (sphereTankStableChannel.value.channelIndex < 0) errs.push(t.value.tp_sphereEnabledNoChannel)
    }
  }
  return errs
})
const isStepValid = computed(() => currentStep.value === 2 || currentStepErrors.value.length === 0)

function nextStep() { if (currentStep.value < steps.value.length - 1) currentStep.value++ }
function prevStep() { if (currentStep.value > 0) currentStep.value-- }

// generatePoints 使用整数步数循环而非浮点累加，避免 (min + i*step) 浮点误差导致
// 实际生成点数与 pointCount 显示不一致（曾出现 step=0.1 时多生成 1 个点）。
function generatePoints() {
  const { alphaMin, alphaMax, alphaStep } = pointLayout.value
  if (alphaStep <= 0 || alphaMax < alphaMin) return []
  const count = Math.floor((alphaMax - alphaMin) / alphaStep) + 1
  const points = []
  for (let i = 0; i < count; i++) {
    const a = alphaMin + i * alphaStep
    // 坐标键名必须用希腊字母 'α'，与后端 total_pressure.go:101 point.Coordinates["α"] 对齐。
    // 早期版本误用英文 'Alpha'，导致后端 AcquireDataWithChannels 第一个点就报"测点缺少 α 坐标"，
    // 校准启动后 completedPoints 永远为 0，UI 进度条不更新。
    points.push({ id: i, coordinates: { 'α': Math.round(a * 100) / 100 } })
  }
  return points
}

// 选择 CSV 保存目录：用 buildCalibrationCsvName 生成清洗后的默认文件名，
// joinCalibrationPath 拼接目录与文件名（POSIX 风格，避免 Windows 反斜杠混用）。
async function pickSavePath() {
  try {
    const defaultName = buildCalibrationCsvName(calibrationName.value.trim(), 'total-pressure')
    const picked = await storageStore.pickDirectory()
    if (picked) savePath.value = joinCalibrationPath(picked, defaultName)
  } catch (e) {
    feedbackStore.pushToast(t.value.tp_failedPickSavePath + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

async function saveConfig() {
  isSaving.value = true
  try {
    const config: CalibrationConfig = {
      type: 'total-pressure',
      name: calibrationName.value,
      probeChannels: probeChannels.value.filter(ch => ch.enabled),
      motionAxes: motionAxes.value,
      points: generatePoints(),
      dwellTimeMs: dwellTimeMs.value,
      samplesPerPoint: samplesPerPoint.value,
      savePath: savePath.value.trim(),
      totalPressureLayout: pointLayout.value,
      sphereTankGate: {
        enabled: sphereTankGateEnabled.value,
        waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value),
        timeoutSec: Math.max(0, sphereTankTimeoutSec.value),
        stableTimeChannel: { ...sphereTankStableChannel.value },
      },
    }
    const normalizedConfig = applyCalibrationPrecisionDefaults(config)
    const res = await calibrationApi.saveConfig('total-pressure', JSON.parse(JSON.stringify(normalizedConfig)))
    if (!res.success) throw new Error(res.error || t.value.tp_saveFailed)
    emit('saved', normalizedConfig); emit('close')
  } catch (err) {
    feedbackStore.pushToast(t.value.tp_saveFailedColon + (err instanceof Error ? err.message : String(err)), 'error')
  } finally { isSaving.value = false }
}

async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('total-pressure')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (!config) return
    calibrationName.value = config.name
    if (config.totalPressureLayout) pointLayout.value = { ...config.totalPressureLayout }
    // 兼容旧配置：若旧 role（pTotal/pStatic）已不存在于 probeChannels 默认列表，
    // 则 find 返回 undefined，自动跳过，避免旧配置残留导致角色错位。
    config.probeChannels?.forEach(sc => {
      const ec = probeChannels.value.find(c => c.role === sc.role)
      if (ec) {
        ec.channel = { ...sc.channel }
        ec.enabled = sc.enabled
        ec.role = sc.role
        ec.precision = sc.precision
      }
    })
    config.motionAxes?.forEach((sa, i) => { if (motionAxes.value[i]) motionAxes.value[i] = { ...sa } })
    dwellTimeMs.value = config.dwellTimeMs
    samplesPerPoint.value = config.samplesPerPoint
    savePath.value = config.savePath || ''
    if (config.sphereTankGate) {
      sphereTankGateEnabled.value = config.sphereTankGate.enabled
      sphereTankWaitTimeSec.value = config.sphereTankGate.waitTimeSec
      sphereTankTimeoutSec.value = config.sphereTankGate.timeoutSec ?? 300
      // 旧配置可能 channelIndex=0 且未选 deviceId，保留原值以便用户后续修正
      sphereTankStableChannel.value = { ...config.sphereTankGate.stableTimeChannel }
    }
  } catch { /* ok */ }
}

onMounted(async () => {
  try {
    const results = await Promise.allSettled([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), loadSavedConfig()])
    reportAllSettledFailures(
      results,
      [t.value.deviceList, t.value.tp_motionControllerList, t.value.tp_totalPressureConfig],
      feedbackStore.pushToast,
    )
  } finally { isLoading.value = false }
})
</script>

<template>
  <UiDialog :show="true" :title="t.tp_totalPressureCalibrationConfig" closable width="min(92vw, 920px)" @update:show="emit('close')">
    <UiSteps :current="currentStep" class="steps-mb"><UiStep v-for="(s, i) in steps" :key="i" :title="s" /></UiSteps>

    <UiSpin v-if="isLoading" :show="true" class="spinner" />
    <template v-else>
      <UiAlert v-if="currentStepErrors.length > 0" type="warning" :title="t.tp_pleaseFixIssues" class="alert-error">{{ currentStepErrors[0] }}</UiAlert>

      <div v-if="currentStep === 0" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.tp_configNameLabel }}</span></template>
          <UiInput v-model="calibrationName" :placeholder="t.tp_inputConfigName" />
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.tp_pointLayoutAlpha }}</span></template>
          <div class="mach-grid">
            <div class="field"><span class="field-label">{{ t.tp_minValueDeg }}</span><UiInputNumber v-model="pointLayout.alphaMin" /></div>
            <div class="field"><span class="field-label">{{ t.tp_maxValueDeg }}</span><UiInputNumber v-model="pointLayout.alphaMax" /></div>
            <div class="field"><span class="field-label">{{ t.tp_stepDeg }}</span><UiInputNumber v-model="pointLayout.alphaStep" :min="1" /></div>
          </div>
          <div class="point-summary"><span class="summary-label">{{ t.tp_totalCalibPoints }}</span><span class="point-count">{{ pointCount }} {{ t.point }}</span></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.tp_acquisitionParams }}</span></template>
          <div class="param-grid">
            <div class="field"><span class="field-label">{{ t.tp_dwellTimeMsLabel }}</span><UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" /></div>
            <div class="field"><span class="field-label">{{ t.tp_samplesPerPointLabel }}</span><UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" /></div>
          </div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.tp_csvSavePath }}</span></template>
          <div class="save-path-row">
            <UiInput v-model="savePath" :placeholder="t.tp_clickToPickSaveDir" class="flex-1" />
            <UiButton variant="secondary" size="sm" @click="pickSavePath">{{ t.tp_pickDirectory }}</UiButton>
          </div>
          <span class="hint-text">{{ t.tp_csvRealtimeWriteHint }}</span>
        </UiPanel>
      </div>

      <div v-if="currentStep === 1" class="step-content">
        <!-- 批量操作工具栏：统一选择设备 + 通道号自动递增填充 -->
        <div class="batch-toolbar">
          <div class="batch-toolbar-row">
            <div class="batch-cell">
              <span class="batch-label">{{ t.unifiedDevice }}</span>
              <UiSelect v-model="batchDeviceId" :options="deviceOptions" :placeholder="t.selectDevice" class="batch-select" />
            </div>
            <div class="batch-cell">
              <UiButton size="sm" variant="primary" :disabled="!batchDeviceId" @click="applyDeviceToAll">{{ t.applyToAllChannels }}</UiButton>
            </div>
          </div>
          <div class="batch-toolbar-row">
            <div class="batch-cell">
              <span class="batch-label">{{ t.startChannel }}</span>
              <UiSelect
                :model-value="autoFillStartIndex !== null ? String(autoFillStartIndex) : ''"
                @update:model-value="autoFillStartIndex = $event !== '' ? Number($event) : null"
                :options="channelIndexOptions"
                :placeholder="t.selectStartChannel"
                class="batch-select"
              />
            </div>
            <div class="batch-cell">
              <UiButton size="sm" variant="primary" :disabled="autoFillStartIndex === null" @click="autoFillChannelIndices">{{ t.autoIncrementFill }}</UiButton>
            </div>
          </div>
        </div>

        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.tp_pointChannelMapping }}</span></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th class="col-enabled">{{ t.channelEnabled }}</th><th>{{ t.tp_pointName }}</th><th>{{ t.channelDataSource }}</th><th class="col-channel">{{ t.tp_channelIndex }}</th><th class="col-precision">{{ t.channelPrecision }}</th></tr></thead>
            <tbody><tr v-for="ch in probeChannels" :key="ch.name"><td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td><td><span class="cell-name">{{ ch.name }}</span></td><td><UiSelect v-model="ch.channel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" :placeholder="t.selectDevice" :disabled="!ch.enabled" :fallback="false" /></td><td><UiSelect
              :model-value="ch.channel.channelIndex >= 0 ? String(ch.channel.channelIndex) : ''"
              @update:model-value="ch.channel.channelIndex = $event !== '' ? Number($event) : -1"
              :options="channelIndexOptions"
              :placeholder="t.tp_unassigned"
              :disabled="!ch.enabled"
            /></td><td><UiInputNumber v-model="ch.precision" :min="0" :max="8" :disabled="!ch.enabled" /></td></tr></tbody></table></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.tp_motionAxisConfig }}</span></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th>{{ t.coordinateAxis }}</th><th class="col-controller">{{ t.motionControllerLabel }}</th><th class="col-axis">{{ t.physicalAxis }}</th></tr></thead>
            <tbody><tr v-for="ax in motionAxes" :key="ax.name"><td><UiStatusBadge status="connected">{{ ax.name }}</UiStatusBadge></td><td><UiSelect v-model="ax.controllerId" :options="motionControllerList.map(c => ({ label: `${c.name} (${c.type})`, value: c.id }))" :placeholder="t.selectController" /></td><td><UiSelect v-model="ax.axis" :options="[{label: t.tp_axisX, value: 'X'},{label: t.tp_axisY, value: 'Y'},{label: t.tp_axisZ, value: 'Z'},{label: t.tp_axisU, value: 'U'}]" /></td></tr></tbody></table></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.tp_sphereTankStableCheck }}</span></template>
          <UiCheckbox v-model:checked="sphereTankGateEnabled" class="checkbox-mb">{{ t.tp_enableSphereTankCheck }}</UiCheckbox>
          <div v-if="sphereTankGateEnabled" class="sphere-grid">
            <div class="field"><span class="field-label">{{ t.tp_waitTimeSec }}</span><UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" /></div>
            <div class="field"><span class="field-label">{{ t.tp_timeoutSec }}</span><UiInputNumber v-model="sphereTankTimeoutSec" :min="0" :step="30" /></div>
            <div class="field"><span class="field-label">{{ t.tp_pxiDevice }}</span><UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" :placeholder="t.selectDevice" :fallback="false" /></div>
            <div class="field"><span class="field-label">{{ t.tp_stableChannel }}</span><UiSelect
              :model-value="sphereTankStableChannel.channelIndex >= 0 ? String(sphereTankStableChannel.channelIndex) : ''"
              @update:model-value="sphereTankStableChannel.channelIndex = $event !== '' ? Number($event) : -1"
              :options="channelIndexOptions"
              :placeholder="t.tp_selectChannel"
            /></div>
            <p class="sphere-hint">{{ t.tp_sphereTimeoutHint }}</p>
          </div>
        </UiPanel>
      </div>

      <div v-if="currentStep === 2" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.summaryTitle }}</span></template>
          <div class="summary-grid">
            <div class="summary-row"><span class="summary-label">{{ t.tp_configNameLabel }}</span><span class="summary-value">{{ calibrationName }}</span></div>
            <div class="summary-row"><span class="summary-label">{{ t.tp_calibrationTypeLabel }}</span><span class="summary-value">{{ t.tp_totalPressureProbe }}</span></div>
            <div class="summary-row"><span class="summary-label">{{ t.pointLayout }}</span><span class="summary-value">{{ t.tp_alphaRangeSummary.replace('{min}', String(pointLayout.alphaMin)).replace('{max}', String(pointLayout.alphaMax)).replace('{step}', String(pointLayout.alphaStep)) }}</span></div>
            <div class="summary-row"><span class="summary-label">{{ t.totalPoints }}</span><span class="accent-bold">{{ pointCount }} {{ t.point }}</span></div>
            <div class="summary-row"><span class="summary-label">{{ t.tp_enabledPointsLabel }}</span><span class="summary-value">{{ probeChannels.filter(ch => ch.enabled).length }} {{ t.tp_countUnit }}</span></div>
            <div class="summary-row"><span class="summary-label">{{ t.tp_dwellTimeMsLabel }}</span><span class="summary-value">{{ dwellTimeMs }} {{ t.tp_msUnit }}</span></div>
            <div class="summary-row"><span class="summary-label">{{ t.tp_samplesPerPointLabel }}</span><span class="summary-value">{{ samplesPerPoint }} {{ t.tp_timesUnit }}</span></div>
            <div class="summary-row"><span class="summary-label">{{ t.tp_csvPathLabel }}</span><span class="summary-value" style="word-break:break-all;text-align:right">{{ savePath || '—' }}</span></div>
            <div v-if="sphereTankGateEnabled" class="summary-row"><span class="summary-label">{{ t.tp_sphereTimeoutLabel }}</span><span class="summary-value">{{ sphereTankTimeoutSec > 0 ? sphereTankTimeoutSec : 300 }} {{ t.tp_secondsUnit }}</span></div>
          </div>
        </UiPanel>
      </div>
    </template>

    <template #footer>
      <div class="footer-bar"><div><UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">{{ t.previous }}</UiButton></div><div class="footer-actions"><span class="step-indicator">{{ t.tp_stepIndicator.replace('{current}', String(currentStep + 1)).replace('{total}', String(steps.length)) }}</span><UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">{{ t.next }}</UiButton><UiButton v-else size="sm" variant="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">{{ t.saveConfig }}</UiButton></div></div>
    </template>
  </UiDialog>
</template>

<style scoped>
/* ============================================================
   枚举控件（NSelect）宽度稳定化
   所有 UiSelect 统一撑满父容器，防止选项文字长短导致宽度跳动
   ============================================================ */
.field :deep(.n-select),
.batch-cell :deep(.n-select),
.ntable td :deep(.n-select),
.sphere-grid :deep(.n-select) {
  width: 100% !important;
  min-width: 0;
}

/* UiInputNumber 同样撑满父容器，避免内联 style 残留导致宽度不统一 */
.field :deep(.n-input-number),
.ntable td :deep(.n-input-number) {
  width: 100% !important;
  min-width: 0;
}

/* 步骤内容：使用固定 height（而非 max-height）确保步骤切换时画面尺寸稳定。
   关键：用 height: 60vh 让内容少时主体保持固定高度、内容多时内部滚动，
   避免步骤切换时对话框整体高度跳动破坏视觉锚点（与遍历测试一致）。 */
.step-content {
  display: flex;
  flex-direction: column;
  gap: 6px;
  height: 60vh;
  overflow-y: auto;
  scrollbar-width: thin;
  padding-right: var(--space-1);
}

.section-card { font-size: var(--text-sm); }

/* 紧凑化 UiPanel 内边距：默认 var(--space-3) var(--space-4) 偏大，
   覆盖为 4px 8px 让卡片视觉更紧凑、与遍历测试一致。 */
.section-card :deep(.n-card__content) {
  padding: 4px 8px;
}
.section-card :deep(.n-card-header) {
  padding: 4px 8px;
}

/* 批量操作工具栏：扁平化工具条，与面板风格协调 */
.batch-toolbar {
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  display: flex;
  flex-direction: column;
  gap: var(--space-1-5);
}
.batch-toolbar-row { display: grid; grid-template-columns: 180px 1fr; align-items: end; gap: var(--space-2); }
.batch-cell { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.batch-label { font-size: var(--text-xs); font-weight: 500; color: var(--text-secondary); white-space: nowrap; }
.batch-select { width: 100%; }

/* 字段通用结构：label + 控件垂直堆叠，min-width:0 保证内部控件能收缩 */
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}
.field-label { font-size: var(--text-xs); color: var(--text-muted); }

/* 点位布局：3 列网格 */
.mach-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-2); margin-bottom: var(--space-2); }
.point-summary {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1-5) var(--space-2);
  border-radius: var(--radius-md);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent);
  background: color-mix(in srgb, var(--accent-primary) 5%, transparent);
}

/* 采集参数：2 列网格 */
.param-grid { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-2); }

/* CSV 保存路径 */
.save-path-row { display: flex; gap: var(--space-2); align-items: center; }
.save-path-row :deep(input) { flex: 1; }

/* 表格：固定列宽，防止 select 宽度跟随内容变化 */
.table-wrap { overflow-x: auto; }
.ntable { width: 100%; border-collapse: collapse; font-size: var(--text-sm); table-layout: fixed; }
.ntable th { text-align: left; padding: var(--space-1-5) 8px; font-weight: 600; font-size: var(--text-xs); color: var(--text-muted); background: var(--bg-panel-strong); border-bottom: 1px solid var(--border-default); }
.ntable td { padding: var(--space-1) 8px; border-bottom: 1px solid var(--border-default); overflow: hidden; }
.ntable tbody tr:hover { background: color-mix(in srgb, var(--accent-primary) 3%, transparent); }

/* 通道映射表格固定列宽：启用 | 名称(弹性) | 设备(弹性) | 通道 | 精度 */
.ntable .col-enabled { width: 48px; }
.ntable .col-channel { width: 96px; }
.ntable .col-precision { width: 80px; }

/* 运动轴表格列宽比例 */
.ntable .col-controller { width: 60%; }
.ntable .col-axis { width: 25%; }

/* 球罐判定：4 列网格（等待时间 | 超时 | 设备 | 通道） */
.sphere-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--space-2); }
.sphere-hint { grid-column: 1 / -1; margin: 0; font-size: var(--text-xs); color: var(--text-muted); line-height: 1.5; }

/* 摘要 */
.summary-grid { display: flex; flex-direction: column; }
.summary-row { display: flex; justify-content: space-between; align-items: center; padding: var(--space-1-5) 0; border-bottom: 1px solid var(--border-default); gap: var(--space-3); font-size: var(--text-sm); }
.summary-row:last-child { border-bottom: none; }

.section-header { font-size: var(--text-sm); font-weight: 600; color: var(--text-primary); }
.hint-text { font-size: 10px; display: block; margin-top: 2px; color: var(--text-muted); }
.summary-label { color: var(--text-muted); font-size: var(--text-sm); }
.summary-value { color: var(--text-primary); }
.point-count { font-size: 18px; font-weight: 700; color: var(--accent-primary); font-variant-numeric: tabular-nums; }
.accent-bold { color: var(--accent-primary); font-weight: 700; }
.checkbox-mb { margin-bottom: var(--space-2); }
.cell-center { text-align: center; }
.cell-name { font-size: var(--text-sm); color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.steps-mb { margin-bottom: var(--space-3); }
.spinner { display: flex; justify-content: center; padding: var(--space-10) 0; }
.alert-error { margin-bottom: var(--space-2); font-size: var(--text-sm); }
.footer-bar { display: flex; align-items: center; justify-content: space-between; width: 100%; }
.footer-actions { display: flex; align-items: center; gap: var(--space-2); }
.step-indicator { font-size: var(--text-xs); color: var(--text-muted); }
</style>
