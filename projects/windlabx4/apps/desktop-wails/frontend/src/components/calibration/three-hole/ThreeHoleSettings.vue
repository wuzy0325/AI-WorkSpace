<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore } from '@stores/storageStore'
import { buildCalibrationCsvName, joinCalibrationPath, splitCalibrationSavePath } from '@shared/calibrationCsvPath'
import { calibrationApi } from '@api/calibrationApi'
import type { CalibrationConfig, CalibrationCoordinateMode, ProbeChannelConfig, MotionAxisConfig, ThreeHolePointLayout, ChannelRef } from '@shared/types/calibration'
import type { MotionSafetyConfig } from '@shared/types/traversal'
import { applyCalibrationPrecisionDefaults, DEFAULT_CALIBRATION_PROBE_PRECISION } from '@shared/calibrationPrecision'
import { getProbeChannelDisplayName } from '@shared/calibrationChannelI18n'
import MotionSafetyPanel from '@components/shared/MotionSafetyPanel.vue'
import CoordinateModeSelect from '@components/shared/CoordinateModeSelect.vue'
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
const { t } = storeToRefs(useI18nStore())
const storageStore = useStorageStore()

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = computed(() => [t.value.stepBasic, t.value.stepHardware, t.value.stepConfirm])

const pointLayout = ref<ThreeHolePointLayout>({ thetaMin: -30, thetaMax: 30, thetaStep: 5 })
const pointCount = computed(() => Math.floor((pointLayout.value.thetaMax - pointLayout.value.thetaMin) / pointLayout.value.thetaStep) + 1)
const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
// 默认名用 ISO 日期（YYYY-MM-DD），避免 toLocaleDateString 在中文环境返回"2026/7/8"
// 含斜杠——斜杠在文件名里非法，会导致保存按钮默认文件名被原生对话框解析为路径。
const calibrationName = ref(`${t.value.th_threeHoleCalibration}-${new Date().toISOString().slice(0, 10)}`)
// CSV 保存路径：自动校准启动时后端按此路径覆盖初始化 csvWriter，逐点实时写入，
// 崩溃/断电不丢已采集点。空字符串将导致后端跳过实时写入（仅靠校准结束全量导出）。
//
// 拆分为目录 (savePath) + 文件名 (saveFileName) 两个独立字段（与遍历测试一致）：
//   - savePath 仅保存目录，pickSavePath 只选目录不拼接文件名，避免用户改文件名时
//     必须重新点选目录的繁琐交互；
//   - saveFileName 用户可见可手动编辑，watch calibrationName 自动同步默认值，
//     保存前再次清洗（剥离 .csv 后缀 + 非法字符过滤），加载时同样剥离后缀再清洗，
//     防止持久化的 ".csv" 被 buildCalibrationCsvName 当作普通字符再追加一次。
const savePath = ref('')
const saveFileName = ref(buildCalibrationCsvName(calibrationName.value.trim(), 'three-hole'))
const sphereTankGateEnabled = ref(false)
const sphereTankWaitTimeSec = ref(3)
// 球罐判定总超时（秒）：后端 <=0 时使用默认 300 秒，前端默认 300 显式暴露给操作员
const sphereTankTimeoutSec = ref(300)
const sphereTankStableChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })
// 球罐压力通道：仅用于前端实时显示压力值，不参与闸门判定；未配置 deviceId 时 UI 显示"暂无数据"
const sphereTankPressureChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })

const probeChannels = ref<ProbeChannelConfig[]>([
  { name: t.value['threeHoleP1'], role: 'threeHole.p1', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['threeHoleP2'], role: 'threeHole.p2', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['threeHoleP3'], role: 'threeHole.p3', channel: { deviceId: '', channelIndex: 2 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['threeHolePAtm'], role: 'threeHole.pAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['threeHoleTAtm'], role: 'threeHole.tAtm', channel: { deviceId: '', channelIndex: 17 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['threeHolePTotal'], role: 'threeHole.pTotal', channel: { deviceId: '', channelIndex: 3 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: t.value['threeHolePStatic'], role: 'threeHole.pStatic', channel: { deviceId: '', channelIndex: 4 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
])

const motionAxes = ref<MotionAxisConfig[]>([{ name: 'Theta', controllerId: '', axis: 'X' }])

// 运动坐标模式：absolute（点位坐标即绝对目标位置，默认）/ relative（点位坐标作为相对位移量）。
const coordinateMode = ref<CalibrationCoordinateMode>('absolute')

// 运动安全配置：4 个全局阈值 + 按轴覆盖，留空字段等价于"使用后端默认值"。
// 与遍历测试模块共享同一份 MotionSafetyConfig 类型与 MotionSafetyPanel 组件，
// 保证校准与遍历的运动安全语义完全一致。
const motionSafety = ref<MotionSafetyConfig | undefined>(undefined)
const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)
const REQUIRED_CHANNEL_ROLES = ['threeHole.p1', 'threeHole.p2', 'threeHole.p3', 'threeHole.pAtm', 'threeHole.tAtm', 'threeHole.pTotal', 'threeHole.pStatic'] as const

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

// 运动轴物理轴枚举选项：标签随全局语言切换
const axisOptions = computed(() => [
  { label: t.value.th_xAxis, value: 'X' },
  { label: t.value.th_yAxis, value: 'Y' },
  { label: t.value.th_zAxis, value: 'Z' },
  { label: t.value.th_uAxis, value: 'U' },
])

const currentStepErrors = computed<string[]>(() => {
  const errs: string[] = []
  if (currentStep.value === 0) {
    if (!calibrationName.value.trim()) errs.push(t.value.enterConfigName)
    if (pointLayout.value.thetaStep <= 0) errs.push(t.value.th_stepMustGreaterThanZero)
    if (pointLayout.value.thetaMax <= pointLayout.value.thetaMin) errs.push(t.value.maxGreaterThanMin)
    if (dwellTimeMs.value < 100) errs.push(t.value.th_dwellTimeCannotBeLessThan100)
    if (samplesPerPoint.value < 1) errs.push(t.value.th_samplesCannotBeLessThan1)
    if (!savePath.value.trim()) errs.push(t.value.th_pleaseSelectCsvPath)
    if (!saveFileName.value.trim()) errs.push(t.value.th_pleaseInputCsvFileName)
  }
  if (currentStep.value === 1) {
    if (probeChannels.value.filter(ch => ch.enabled).some(ch => !ch.channel.deviceId || ch.channel.channelIndex < 0)) errs.push(t.value.th_channelMappingIncomplete)
    if (motionAxes.value.some(a => !a.controllerId)) errs.push(t.value.th_motionAxisMustBindController)
  }
  return errs
})
const isStepValid = computed(() => currentStep.value === 2 || currentStepErrors.value.length === 0)

function nextStep() { if (currentStep.value < steps.value.length - 1) currentStep.value++ }
function prevStep() { if (currentStep.value > 0) currentStep.value-- }

// generatePoints 使用整数步数循环而非浮点累加，避免 (min + i*step) 浮点误差导致
// 实际生成点数与 pointCount 显示不一致（如 step=0.1 时 0.1+0.2+...≠0.6，可能漏点或重点）。
// 与 TotalPressureSettings.generatePoints 同构，保持四个校准模块点位生成逻辑一致。
function generatePoints() {
  const { thetaMin, thetaMax, thetaStep } = pointLayout.value
  if (thetaStep <= 0 || thetaMax < thetaMin) return []
  const count = Math.floor((thetaMax - thetaMin) / thetaStep) + 1
  const points = []
  for (let i = 0; i < count; i++) {
    const t = thetaMin + i * thetaStep
    // 坐标键名必须用希腊字母 'θ'，与后端 three_hole.go point.Coordinates["θ"] 对齐。
    points.push({ id: i, coordinates: { 'θ': Math.round(t * 100) / 100 } })
  }
  return points
}

// 选择 CSV 保存目录：与遍历测试一致，只选目录赋给 savePath，
// 文件名由独立的 saveFileName 字段管理（用户可见可编辑），避免每次改文件名都要重选目录。
async function pickSavePath() {
  try {
    const picked = await storageStore.pickDirectory()
    if (picked) savePath.value = picked
  } catch (e) {
    feedbackStore.pushToast(t.value.th_failedToSelectSavePath + ': ' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

// calibrationName 变化时同步刷新默认 saveFileName：仅当用户未手动修改（仍为空或等于上一默认值）时覆盖，
// 避免覆盖用户手动输入的文件名。复用共享工具保证清洗规则与遍历测试画面一致。
watch(calibrationName, (next, prev) => {
  const prevDefault = buildCalibrationCsvName(prev.trim(), 'three-hole')
  if (saveFileName.value === '' || saveFileName.value === prevDefault)
    saveFileName.value = buildCalibrationCsvName(next.trim(), 'three-hole')
})

async function saveConfig() {
  isSaving.value = true
  try {
    // 保存前再次清洗 saveFileName：用户可能手动输入了非法字符或未带 .csv 后缀。
    // 剥离 .csv 后缀后交给共享工具，fallback 用 calibrationName 保证空文件名也能落到有意义的默认值。
    const normName = buildCalibrationCsvName(saveFileName.value.replace(/\.csv$/i, ''), calibrationName.value.trim())
    saveFileName.value = normName
    // 后端 csv_writer 约定 SavePath 必须是含 .csv 扩展名的完整文件路径，
    // 故前端在保存时把目录与文件名拼接为完整路径再传给后端，同时持久化 saveFileName
    // 便于下次加载时分离展示。
    const fullSavePath = savePath.value.trim() ? joinCalibrationPath(savePath.value.trim(), normName) : ''
    const config: CalibrationConfig = {
      type: 'three-hole', name: calibrationName.value, probeChannels: probeChannels.value.filter(ch => ch.enabled),
      motionAxes: motionAxes.value,
      // 运动坐标模式：绝对坐标（点位值即目标位置）或相对坐标（点位值为位移量）
      coordinateMode: coordinateMode.value,
      // 运动安全配置透传：未配置字段为 undefined，后端 Resolve() 时合并默认值
      motionSafety: motionSafety.value,
      points: generatePoints(), dwellTimeMs: dwellTimeMs.value, samplesPerPoint: samplesPerPoint.value, savePath: fullSavePath, saveFileName: normName,
      threeHoleLayout: pointLayout.value,
      sphereTankGate: {
        enabled: sphereTankGateEnabled.value,
        waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value),
        timeoutSec: Math.max(0, sphereTankTimeoutSec.value),
        stableTimeChannel: { ...sphereTankStableChannel.value },
        // 仅当配置了压力通道 deviceId 时才落盘，避免写入空 ChannelRef 造成后端误订阅
        ...(sphereTankPressureChannel.value.deviceId ? { pressureChannel: { ...sphereTankPressureChannel.value } } : {}),
      }
    }
    const res = await calibrationApi.saveConfig('three-hole', JSON.parse(JSON.stringify(applyCalibrationPrecisionDefaults(config))))
    if (!res.success) throw new Error(res.error || t.value.th_saveFailed)
    emit('saved', applyCalibrationPrecisionDefaults(config)); emit('close')
  } catch (err) { feedbackStore.pushToast(t.value.th_saveFailed + ': ' + (err instanceof Error ? err.message : String(err)), 'error')
  } finally { isSaving.value = false }
}

async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('three-hole')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (!config) return
    calibrationName.value = config.name
    if (config.threeHoleLayout) pointLayout.value = { ...config.threeHoleLayout }
    config.probeChannels?.forEach(sc => { const ec = probeChannels.value.find(c => c.role ? c.role === sc.role : c.name === sc.name); if (ec) { ec.channel = { ...sc.channel }; ec.enabled = sc.enabled; ec.role = sc.role; ec.precision = sc.precision } })
    config.motionAxes?.forEach((sa, i) => { if (motionAxes.value[i]) motionAxes.value[i] = { ...sa } })
    // 还原运动安全配置：浅拷贝避免修改持久化对象，未配置字段保持 undefined
    if (config.motionSafety) {
      motionSafety.value = { ...config.motionSafety }
    } else {
      motionSafety.value = undefined
    }
    dwellTimeMs.value = config.dwellTimeMs; samplesPerPoint.value = config.samplesPerPoint
    // 还原运动坐标模式：旧配置无此字段时保持默认绝对坐标
    coordinateMode.value = config.coordinateMode === 'relative' ? 'relative' : 'absolute'
    // 还原 savePath 与 saveFileName：
    //   - 优先使用持久化的 saveFileName（新配置），剥离 .csv 后缀再交给共享工具重新清洗，
    //     防止持久化的 ".csv" 被 buildCalibrationCsvName 当作普通字符再追加一次。
    //   - 旧配置无 saveFileName 字段时，用 splitCalibrationSavePath 从完整 savePath
    //     反拆 basename 作为兜底，兼容升级前已落盘的配置；dir 还原为纯目录。
    if (config.saveFileName) {
      saveFileName.value = buildCalibrationCsvName(config.saveFileName.replace(/\.csv$/i, ''), config.name)
    } else if (config.savePath) {
      const { baseName } = splitCalibrationSavePath(config.savePath)
      saveFileName.value = buildCalibrationCsvName(baseName.replace(/\.csv$/i, ''), config.name)
    }
    const { dir: restoredDir } = splitCalibrationSavePath(config.savePath || '')
    savePath.value = restoredDir
    if (config.sphereTankGate) {
      sphereTankGateEnabled.value = config.sphereTankGate.enabled
      sphereTankWaitTimeSec.value = config.sphereTankGate.waitTimeSec
      sphereTankTimeoutSec.value = config.sphereTankGate.timeoutSec ?? 300
      sphereTankStableChannel.value = { ...config.sphereTankGate.stableTimeChannel }
      // 压力通道为可选字段，未配置时回退为空 deviceId（UI 显示"暂无数据"）
      sphereTankPressureChannel.value = config.sphereTankGate.pressureChannel
        ? { ...config.sphereTankGate.pressureChannel }
        : { deviceId: '', channelIndex: 0 }
    }
  } catch (err) {
    // 与 FiveHoleSettings 对齐：首次打开无配置（404）属正常，其他异常需提示。
    // 静默吞错会让用户误以为已成功加载，可能用空表单覆盖已有配置导致数据丢失。
    const msg = err instanceof Error ? err.message : String(err)
    if (!msg.includes('404') && !msg.includes('not found')) {
      feedbackStore.pushToast(t.value.wf_loadConfigFailed + msg, 'warning')
    }
  }
}

onMounted(async () => {
  try {
    const results = await Promise.allSettled([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), loadSavedConfig()])
    reportAllSettledFailures(
      results,
      [t.value.deviceList, t.value.th_motionControllerListLabel, t.value.th_threeHoleCalibrationConfigLabel],
      feedbackStore.pushToast,
    )
    // 加载后若 savePath 仍为空，回退到全局基础目录，与遍历测试体验一致：
    // 用户首次打开对话框不必先点选目录就能完成保存。
    if (!savePath.value.trim()) savePath.value = storageStore.settings?.baseDirectory?.trim() ?? ''
  } finally { isLoading.value = false }
})
</script>

<template>
  <UiDialog :show="true" :title="t.th_threeHoleCalibrationConfig" closable width="min(92vw, 920px)" @update:show="emit('close')">
    <UiSteps :current="currentStep" class="steps-mb"><UiStep v-for="(s, i) in steps" :key="i" :title="s" /></UiSteps>

    <UiSpin v-if="isLoading" :show="true" class="spinner" />
    <template v-else>
      <UiAlert v-if="currentStepErrors.length > 0" type="warning" :title="t.th_pleaseFixFollowingIssues" class="alert-error">{{ currentStepErrors[0] }}</UiAlert>

      <div v-if="currentStep === 0" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.th_configName }}</span></template>
          <UiInput v-model="calibrationName" :placeholder="t.th_inputConfigName" />
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.th_pointLayoutAngleTheta }}</span></template>
          <div class="mach-grid">
            <div class="field"><span class="field-label">{{ t.th_minValueDeg }}</span><UiInputNumber v-model="pointLayout.thetaMin" /></div>
            <div class="field"><span class="field-label">{{ t.th_maxValueDeg }}</span><UiInputNumber v-model="pointLayout.thetaMax" /></div>
            <div class="field"><span class="field-label">{{ t.th_stepDeg }}</span><UiInputNumber v-model="pointLayout.thetaStep" :min="1" /></div>
          </div>
          <div class="point-summary"><span class="summary-label">{{ t.th_totalCalibrationPoints }}</span><span class="point-count">{{ pointCount }} {{ t.th_pointsUnit }}</span></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.th_acquisitionParams }}</span></template>
          <div class="param-grid">
            <div class="field"><span class="field-label">{{ t.th_stableWaitTimeMs }}</span><UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" /></div>
            <div class="field"><span class="field-label">{{ t.th_samplesPerPoint }}</span><UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" /></div>
          </div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.th_csvSavePath }}</span></template>
          <div class="save-path-row">
            <UiInput v-model="savePath" :placeholder="t.th_clickRightBtnToSelectDir" class="flex-1" :title="savePath" />
            <UiInput v-model="saveFileName" :placeholder="t.th_csvFileNamePlaceholder" class="flex-1" />
            <UiButton variant="secondary" size="sm" @click="pickSavePath">{{ t.th_selectDir }}</UiButton>
          </div>
          <span class="hint-text">{{ t.th_realtimeWriteHint }}</span>
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
          <template #header><span class="section-header">{{ t.th_probeChannelMapping }}</span></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th class="col-enabled">{{ t.channelEnabled }}</th><th>{{ t.th_probePointName }}</th><th>{{ t.channelDataSource }}</th><th class="col-channel">{{ t.th_channelIndex }}</th><th class="col-precision">{{ t.channelPrecision }}</th></tr></thead>
            <tbody><tr v-for="ch in probeChannels" :key="ch.name"><td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td><td><span class="cell-name">{{ getProbeChannelDisplayName(ch.role, ch.name, t) }}</span></td><td><UiSelect v-model="ch.channel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" :placeholder="t.selectDevice" :disabled="!ch.enabled" :fallback="false" /></td><td><UiSelect
              :model-value="ch.channel.channelIndex >= 0 ? String(ch.channel.channelIndex) : ''"
              @update:model-value="ch.channel.channelIndex = $event !== '' ? Number($event) : -1"
              :options="channelIndexOptions"
              :placeholder="t.th_unassigned"
              :disabled="!ch.enabled"
            /></td><td><UiInputNumber v-model="ch.precision" :min="0" :max="8" :disabled="!ch.enabled" /></td></tr></tbody></table></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.th_motionAxisConfig }}</span></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th>{{ t.coordinateAxis }}</th><th class="col-controller">{{ t.motionControllerLabel }}</th><th class="col-axis">{{ t.physicalAxis }}</th></tr></thead>
            <tbody><tr v-for="ax in motionAxes" :key="ax.name"><td><UiStatusBadge status="connected">{{ ax.name }}</UiStatusBadge></td><td><UiSelect v-model="ax.controllerId" :options="motionControllerList.map(c => ({ label: `${c.name} (${c.type})`, value: c.id }))" :placeholder="t.selectController" /></td><td><UiSelect v-model="ax.axis" :options="axisOptions" /></td></tr></tbody></table></div>
          <CoordinateModeSelect v-model="coordinateMode" class="coordinate-mode-row" />
        </UiPanel>

        <!-- 球罐判定门控：放在运动安全面板上方，便于操作员优先确认球罐压力条件 -->
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.th_sphereTankStableCheck }}</span></template>
          <UiCheckbox v-model:checked="sphereTankGateEnabled" class="checkbox-mb">{{ t.th_enableSphereTankCheck }}</UiCheckbox>
          <div v-if="sphereTankGateEnabled" class="sphere-grid">
            <div class="field"><span class="field-label">{{ t.th_waitTimeSec }}</span><UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" /></div>
            <div class="field"><span class="field-label">{{ t.th_totalTimeoutSec }}</span><UiInputNumber v-model="sphereTankTimeoutSec" :min="0" :step="30" /></div>
            <div class="field"><span class="field-label">{{ t.th_pxiDevice }}</span><UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" :placeholder="t.selectDevice" :fallback="false" /></div>
            <div class="field"><span class="field-label">{{ t.th_stableChannel }}</span><UiSelect
              :model-value="sphereTankStableChannel.channelIndex >= 0 ? String(sphereTankStableChannel.channelIndex) : ''"
              @update:model-value="sphereTankStableChannel.channelIndex = $event !== '' ? Number($event) : 0"
              :options="channelIndexOptions"
              :placeholder="t.th_selectChannel"
            /></div>
            <!-- 球罐压力通道：仅用于实时显示压力值，不参与闸门判定 -->
            <div class="field"><span class="field-label">{{ t.wf_spherePressureDevice }}</span><UiSelect v-model="sphereTankPressureChannel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" :placeholder="t.selectDevice" :fallback="false" /></div>
            <div class="field"><span class="field-label">{{ t.wf_spherePressureChannel }}</span><UiSelect
              :model-value="sphereTankPressureChannel.channelIndex >= 0 ? String(sphereTankPressureChannel.channelIndex) : ''"
              @update:model-value="sphereTankPressureChannel.channelIndex = $event !== '' ? Number($event) : 0"
              :options="channelIndexOptions"
              :placeholder="t.th_selectChannel"
            /></div>
            <p class="sphere-hint">{{ t.th_sphereTimeoutHint }}</p>
            <p class="sphere-hint muted">{{ t.wf_spherePressureHint }}</p>
          </div>
        </UiPanel>

        <!-- 运动安全配置：紧贴运动轴配置下方，让操作员在绑定轴后立即调整到位容差与异常停机阈值。
             留空字段等价于"使用后端默认值"，避免强制用户填全 4 个字段才能保存。
             与遍历测试模块共享同一份 MotionSafetyPanel 组件，保证语义一致。 -->
        <MotionSafetyPanel
          v-model:motion-safety="motionSafety"
          :motion-axes="motionAxes"
          :t="(t as unknown as Record<string, string>)"
        />
      </div>

      <div v-if="currentStep === 2" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">{{ t.summaryTitle }}</span></template>
          <div class="summary-grid"><div class="summary-row"><span class="summary-label">{{ t.th_configName }}</span><span class="summary-value">{{ calibrationName }}</span></div><div class="summary-row"><span class="summary-label">{{ t.th_calibrationType }}</span><span class="summary-value">{{ t.th_threeHoleProbe }}</span></div><div class="summary-row"><span class="summary-label">{{ t.pointLayout }}</span><span class="summary-value">θ: {{ pointLayout.thetaMin }}° ~ {{ pointLayout.thetaMax }}°（{{ t.step }} {{ pointLayout.thetaStep }}°）</span></div><div class="summary-row"><span class="summary-label">{{ t.totalPoints }}</span><span class="accent-bold">{{ pointCount }} {{ t.th_pointsUnit }}</span></div><div class="summary-row"><span class="summary-label">{{ t.th_enabledPoints }}</span><span class="summary-value">{{ probeChannels.filter(ch => ch.enabled).length }} {{ t.th_countUnit }}</span></div><div class="summary-row"><span class="summary-label">{{ t.coordinateMode }}</span><span class="summary-value">{{ coordinateMode === 'relative' ? t.coordinateModeRelative : t.coordinateModeAbsolute }}</span></div><div class="summary-row"><span class="summary-label">{{ t.th_stableTime }}</span><span class="summary-value">{{ dwellTimeMs }} ms</span></div><div class="summary-row"><span class="summary-label">{{ t.th_samplesPerPointShort }}</span><span class="summary-value">{{ samplesPerPoint }} {{ t.th_timesUnit }}</span></div><div class="summary-row"><span class="summary-label">{{ t.th_csvPath }}</span><span class="summary-value">{{ savePath && saveFileName ? joinCalibrationPath(savePath, saveFileName) : (savePath || t.th_notSelected) }}</span></div></div>
        </UiPanel>
      </div>
    </template>

    <template #footer>
      <div class="footer-bar"><div><UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">{{ t.previous }}</UiButton></div><div class="footer-actions"><span class="step-indicator">{{ t.th_stepIndicator.replace('{current}', String(currentStep + 1)).replace('{total}', String(steps.length)) }}</span><UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">{{ t.next }}</UiButton><UiButton v-else size="sm" variant="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">{{ t.saveConfig }}</UiButton></div></div>
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

/* 字段通用结构 */
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}
.field-label { font-size: var(--text-xs); color: var(--text-muted); }

/* 运动坐标模式选择：与运动轴表格保持间距 */
.coordinate-mode-row {
  margin-top: var(--space-2);
}

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

/* 通道映射表格固定列宽 */
.ntable .col-enabled { width: 48px; }
.ntable .col-channel { width: 96px; }
.ntable .col-precision { width: 80px; }

/* 运动轴表格列宽比例 */
.ntable .col-controller { width: 60%; }
.ntable .col-axis { width: 25%; }

/* 球罐判定：4 列网格（等待时间 | 总超时 | PXI 设备 | 稳定通道） */
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
