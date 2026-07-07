<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useStorageStore } from '@stores/storageStore'
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
const storageStore = useStorageStore()

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = ['基本设置', '硬件配置', '确认保存']

const pointLayout = ref<TotalPressurePointLayout>({ alphaMin: -30, alphaMax: 30, alphaStep: 5 })
const pointCount = computed(() => {
  if (pointLayout.value.alphaStep <= 0) return 0
  return Math.floor((pointLayout.value.alphaMax - pointLayout.value.alphaMin) / pointLayout.value.alphaStep) + 1
})
const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const calibrationName = ref(`总压探针校准-${new Date().toLocaleDateString()}`)
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
  { name: '探针总压', role: 'totalPressure.pProbeTotal', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '风洞总压', role: 'totalPressure.pTunnelTotal', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '风洞静压', role: 'totalPressure.pTunnelStatic', channel: { deviceId: '', channelIndex: 2 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '大气压', role: 'totalPressure.pAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '大气温度', role: 'totalPressure.tAtm', channel: { deviceId: '', channelIndex: 17 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
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
    if (!calibrationName.value.trim()) errs.push('请输入配置名称')
    if (pointLayout.value.alphaStep <= 0) errs.push('步长必须大于 0')
    if (pointLayout.value.alphaMax <= pointLayout.value.alphaMin) errs.push('最大值必须大于最小值')
    if (dwellTimeMs.value < 100) errs.push('稳定等待时间不能小于 100 ms')
    if (samplesPerPoint.value < 1) errs.push('每点采样次数不能小于 1')
    if (!savePath.value.trim()) errs.push('请选择 CSV 保存路径')
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
        errs.push(`通道映射未完成：缺少 ${role}`)
        break
      }
    }
    if (motionAxes.value.some(a => !a.controllerId)) errs.push('运动轴必须绑定控制器')
    if (sphereTankGateEnabled.value) {
      if (!sphereTankStableChannel.value.deviceId) errs.push('球罐判定已启用，但未选择 PXI 设备')
      if (sphereTankStableChannel.value.channelIndex < 0) errs.push('球罐判定已启用，但未选择稳定通道')
    }
  }
  return errs
})
const isStepValid = computed(() => currentStep.value === 2 || currentStepErrors.value.length === 0)

function nextStep() { if (currentStep.value < steps.length - 1) currentStep.value++ }
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
    points.push({ id: i, coordinates: { Alpha: Math.round(a * 100) / 100 } })
  }
  return points
}

// 选择 CSV 保存目录：拼接默认文件名后回填 savePath，与 FiveHoleSettings 行为一致。
async function pickSavePath() {
  try {
    const defaultName = `${calibrationName.value.trim() || 'total-pressure'}-${new Date().toISOString().slice(0, 10)}.csv`
    const picked = await storageStore.pickDirectory()
    if (picked) savePath.value = joinPath(picked, defaultName)
  } catch (e) {
    feedbackStore.pushToast('选择保存路径失败: ' + (e instanceof Error ? e.message : String(e)), 'error')
  }
}

// joinPath 归一化为 POSIX 风格，避免 Windows 反斜杠与正斜杠混合的拼接错误。
function joinPath(dir: string, fileName: string): string {
  const normalizedDir = dir.replace(/[\\/]+$/, '').replace(/\\/g, '/')
  return `${normalizedDir}/${fileName}`
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
    if (!res.success) throw new Error(res.error || '保存失败')
    emit('saved', normalizedConfig); emit('close')
  } catch (err) {
    feedbackStore.pushToast('保存失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
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
      ['设备列表', '运动控制器列表', '总压校准配置'],
      feedbackStore.pushToast,
    )
  } finally { isLoading.value = false }
})
</script>

<template>
  <UiDialog :show="true" title="总压探针校准配置" closable width="980px" @update:show="emit('close')">
    <UiSteps :current="currentStep" class="steps-mb"><UiStep v-for="(s, i) in steps" :key="i" :title="s" /></UiSteps>

    <UiSpin v-if="isLoading" :show="true" class="spinner" />
    <template v-else>
      <UiAlert v-if="currentStepErrors.length > 0" type="warning" title="请先修正以下问题" class="alert-error">{{ currentStepErrors[0] }}</UiAlert>

      <div v-if="currentStep === 0" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">配置名称</span></template>
          <UiInput v-model="calibrationName" placeholder="输入配置名称" />
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">点位布局</span></template>
          <span class="section-desc">攻角 α 范围</span>
          <div class="mach-grid">
            <div><span class="field-label">最小值 (°)</span><UiInputNumber v-model="pointLayout.alphaMin" style="width:100%" /></div>
            <div><span class="field-label">最大值 (°)</span><UiInputNumber v-model="pointLayout.alphaMax" style="width:100%" /></div>
            <div><span class="field-label">步长 (°)</span><UiInputNumber v-model="pointLayout.alphaStep" :min="1" style="width:100%" /></div>
          </div>
          <div class="point-summary"><span class="field-label" style="font-size:var(--text-sm)">总校准点数</span><span class="point-count">{{ pointCount }} 点</span></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">采集参数</span></template>
          <div class="param-grid">
            <div><span class="field-label">稳定等待时间 (毫秒)</span><UiInputNumber v-model="dwellTimeMs" :min="100" :step="100" style="width:100%" /><span class="hint-text">到达点位后等待稳定的时间</span></div>
            <div><span class="field-label">每点采样次数</span><UiInputNumber v-model="samplesPerPoint" :min="1" :max="1000" style="width:100%" /><span class="hint-text">每个点位采集的样本数量，取平均值</span></div>
          </div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">CSV 保存路径</span></template>
          <div class="save-path-row">
            <UiInput v-model="savePath" placeholder="点击右侧按钮选择保存目录" class="flex-1" />
            <UiButton variant="secondary" size="sm" @click="pickSavePath">选择目录</UiButton>
          </div>
          <span class="hint-text">校准过程中逐点实时写入，崩溃/断电不丢已采集数据。空路径将跳过实时写入。</span>
        </UiPanel>
      </div>

      <div v-if="currentStep === 1" class="step-content">
        <!-- 批量操作工具栏：统一选择设备 + 通道号自动递增填充 -->
        <div class="batch-toolbar">
          <div class="batch-toolbar-row">
            <div class="batch-cell">
              <span class="batch-label">统一设备</span>
              <UiSelect v-model="batchDeviceId" :options="deviceOptions" placeholder="选择设备" class="batch-select" />
            </div>
            <div class="batch-cell">
              <UiButton size="sm" variant="primary" :disabled="!batchDeviceId" @click="applyDeviceToAll">应用到全部通道</UiButton>
            </div>
          </div>
          <div class="batch-toolbar-row">
            <div class="batch-cell">
              <span class="batch-label">起始通道</span>
              <UiSelect
                :model-value="autoFillStartIndex !== null ? String(autoFillStartIndex) : ''"
                @update:model-value="autoFillStartIndex = $event !== '' ? Number($event) : null"
                :options="channelIndexOptions"
                placeholder="选择起始通道号"
                class="batch-select"
              />
            </div>
            <div class="batch-cell">
              <UiButton size="sm" variant="primary" :disabled="autoFillStartIndex === null" @click="autoFillChannelIndices">自动递增填充</UiButton>
            </div>
          </div>
        </div>

        <UiPanel class="section-card">
          <template #header><span class="section-header">测点通道映射</span></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th style="width:48px">启用</th><th>测点名称</th><th>数据源设备</th><th style="width:100px">通道索引</th><th style="width:80px">精度</th></tr></thead>
            <tbody><tr v-for="ch in probeChannels" :key="ch.name"><td class="cell-center"><UiCheckbox v-model:checked="ch.enabled" /></td><td><span class="cell-name">{{ ch.name }}</span></td><td><UiSelect v-model="ch.channel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" placeholder="选择设备" style="min-width:140px" :disabled="!ch.enabled" :fallback="false" /></td><td><UiSelect
              :model-value="ch.channel.channelIndex >= 0 ? String(ch.channel.channelIndex) : ''"
              @update:model-value="ch.channel.channelIndex = $event !== '' ? Number($event) : -1"
              :options="channelIndexOptions"
              placeholder="未分配"
              :disabled="!ch.enabled"
            /></td><td><UiInputNumber v-model="ch.precision" :min="0" :max="8" style="width:100%" :disabled="!ch.enabled" /></td></tr></tbody></table></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">运动轴配置</span></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th>坐标轴</th><th>运动控制器</th><th>物理轴</th></tr></thead>
            <tbody><tr v-for="ax in motionAxes" :key="ax.name"><td><UiStatusBadge status="connected">{{ ax.name }}</UiStatusBadge></td><td><UiSelect v-model="ax.controllerId" :options="motionControllerList.map(c => ({ label: `${c.name} (${c.type})`, value: c.id }))" placeholder="选择控制器" style="min-width:160px" /></td><td><UiSelect v-model="ax.axis" :options="[{label:'X 轴',value:'X'},{label:'Y 轴',value:'Y'},{label:'Z 轴',value:'Z'},{label:'U 轴',value:'U'}]" style="width:100px" /></td></tr></tbody></table></div>
        </UiPanel>
        <UiPanel class="section-card">
          <template #header><span class="section-header">球罐稳定判定</span></template>
          <UiCheckbox v-model:checked="sphereTankGateEnabled" class="checkbox-mb">启用球罐判定</UiCheckbox>
          <div v-if="sphereTankGateEnabled" class="sphere-grid">
            <div><span class="field-label">等待时间 (秒)</span><UiInputNumber v-model="sphereTankWaitTimeSec" :min="0" :step="0.1" style="width:100%" /></div>
            <div><span class="field-label">总超时 (秒)</span><UiInputNumber v-model="sphereTankTimeoutSec" :min="0" :step="30" style="width:100%" /><span class="hint-text">0 表示使用默认 300 秒；超时后停止校准</span></div>
            <div><span class="field-label">PXI 设备</span><UiSelect v-model="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" placeholder="选择设备" style="width:100%" :fallback="false" /></div>
            <div><span class="field-label">稳定通道</span><UiSelect
              :model-value="sphereTankStableChannel.channelIndex >= 0 ? String(sphereTankStableChannel.channelIndex) : ''"
              @update:model-value="sphereTankStableChannel.channelIndex = $event !== '' ? Number($event) : -1"
              :options="channelIndexOptions"
              placeholder="选择通道"
            /></div>
          </div>
        </UiPanel>
      </div>

      <div v-if="currentStep === 2" class="step-content">
        <UiPanel class="section-card">
          <template #header><span class="section-header">配置摘要</span></template>
          <div class="summary-grid"><div class="summary-row"><span class="summary-label">配置名称</span><span class="summary-value">{{ calibrationName }}</span></div><div class="summary-row"><span class="summary-label">校准类型</span><span class="summary-value">总压探针</span></div><div class="summary-row"><span class="summary-label">点位布局</span><span class="summary-value">α: {{ pointLayout.alphaMin }}° ~ {{ pointLayout.alphaMax }}°（步长 {{ pointLayout.alphaStep }}°）</span></div><div class="summary-row"><span class="summary-label">总点数</span><span class="accent-bold">{{ pointCount }} 点</span></div><div class="summary-row"><span class="summary-label">启用测点</span><span class="summary-value">{{ probeChannels.filter(ch => ch.enabled).length }} 个</span></div><div class="summary-row"><span class="summary-label">稳定时间</span><span class="summary-value">{{ dwellTimeMs }} ms</span></div><div class="summary-row"><span class="summary-label">每点采样</span><span class="summary-value">{{ samplesPerPoint }} 次</span></div><div class="summary-row"><span class="summary-label">CSV 路径</span><span class="summary-value" style="word-break:break-all;text-align:right">{{ savePath || '—' }}</span></div><div v-if="sphereTankGateEnabled" class="summary-row"><span class="summary-label">球罐超时</span><span class="summary-value">{{ sphereTankTimeoutSec > 0 ? sphereTankTimeoutSec : 300 }} 秒</span></div></div>
        </UiPanel>
      </div>
    </template>

    <template #footer>
      <div class="footer-bar"><div><UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">上一步</UiButton></div><div class="footer-actions"><span class="step-indicator">步骤 {{ currentStep + 1 }} / {{ steps.length }}</span><UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">下一步</UiButton><UiButton v-else size="sm" variant="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">保存配置</UiButton></div></div>
    </template>
  </UiDialog>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:var(--space-3); }
.section-card { font-size:var(--text-sm); }

/* 批量操作工具栏：扁平化工具条，与面板风格协调 */
.batch-toolbar {
  padding: 10px 12px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.batch-toolbar-row { display:grid; grid-template-columns:160px 1fr; align-items:end; gap:10px }
.batch-cell { display:flex; flex-direction:column; gap:4px }
.batch-label { font-size:var(--text-xs); font-weight:500; color:var(--text-secondary); white-space:nowrap }
.batch-select { width:100% }
.mach-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:var(--space-2); margin-bottom:var(--space-3); }
.point-summary { display:flex; align-items:center; gap:var(--space-2); padding:10px; border-radius:var(--radius-md); border:1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent); background:color-mix(in srgb, var(--accent-primary) 5%, transparent); }
.param-grid { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
.save-path-row { display:flex; gap:var(--space-2); align-items:center; }
.save-path-row :deep(input) { flex:1; }
.table-wrap { overflow-x:auto; }
.ntable { width:100%; border-collapse:collapse; font-size:var(--text-sm); }
.ntable th { text-align:left; padding:var(--space-2) 10px; font-weight:600; font-size:var(--text-xs); color:var(--text-muted); background:var(--bg-panel-strong); border-bottom:1px solid var(--border-default); }
.ntable td { padding:6px 10px; border-bottom:1px solid var(--border-default); }
.sphere-grid { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
.summary-grid { display:flex; flex-direction:column; }
.summary-row { display:flex; justify-content:space-between; align-items:center; padding:var(--space-2) 0; border-bottom:1px solid var(--border-default); gap:var(--space-3); font-size:var(--text-sm); }
.summary-row:last-child { border-bottom:none; }

.section-header { font-size:var(--text-sm); font-weight:600; color:var(--text-primary); }
.section-desc { font-size:var(--text-xs); display:block; margin-bottom:var(--space-2); color:var(--text-muted); }
.field-label { font-size:var(--text-xs); color:var(--text-muted); }
.hint-text { font-size:10px; display:block; margin-top:2px; color:var(--text-muted); }
.summary-label { color:var(--text-muted); }
.summary-value { color:var(--text-primary); }
.point-count { font-size:22px; font-weight:700; color:var(--accent-primary); }
.accent-bold { color:var(--accent-primary); font-weight:700; }
.checkbox-mb { margin-bottom:var(--space-2); }
.cell-center { text-align:center; }
.cell-name { font-size:var(--text-sm); color:var(--text-primary); }
.steps-mb { margin-bottom:var(--space-4); }
.spinner { display:flex; justify-content:center; padding:var(--space-10) 0; }
.alert-error { margin-bottom:var(--space-3); font-size:var(--text-sm); }
.footer-bar { display:flex; align-items:center; justify-content:space-between; width:100%; }
.footer-actions { display:flex; align-items:center; gap:var(--space-3); }
.step-indicator { font-size:var(--text-xs); color:var(--text-muted); }
</style>
