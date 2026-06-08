<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { calibrationApi } from '@api/calibrationApi'
import type { CalibrationConfig, ProbeChannelConfig, MotionAxisConfig, TotalPressurePointLayout, ChannelRef } from '@shared/types/calibration'
import { applyCalibrationPrecisionDefaults, DEFAULT_CALIBRATION_PROBE_PRECISION } from '@shared/calibrationPrecision'
import { NAlert, NButton, NCard, NCheckbox, NInput, NInputNumber, NModal, NSelect, NSpin, NStep, NSteps, NTag, NText } from 'naive-ui'
import UiButton from '@components/ui/UiButton.vue'

const emit = defineEmits<{ close: []; saved: [config: CalibrationConfig] }>()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()

const isLoading = ref(true)
const isSaving = ref(false)
const currentStep = ref(0)
const steps = ['基本设置', '硬件配置', '确认保存']

const pointLayout = ref<TotalPressurePointLayout>({ alphaMin: -30, alphaMax: 30, alphaStep: 5 })
const pointCount = computed(() => Math.floor((pointLayout.value.alphaMax - pointLayout.value.alphaMin) / pointLayout.value.alphaStep) + 1)
const dwellTimeMs = ref(2000)
const samplesPerPoint = ref(10)
const calibrationName = ref(`总压探针校准-${new Date().toLocaleDateString()}`)
const sphereTankGateEnabled = ref(false)
const sphereTankWaitTimeSec = ref(3)
const sphereTankStableChannel = ref<ChannelRef>({ deviceId: '', channelIndex: 0 })

const probeChannels = ref<ProbeChannelConfig[]>([
  { name: '总压', role: 'totalPressure.pTotal', channel: { deviceId: '', channelIndex: 0 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '静压', role: 'totalPressure.pStatic', channel: { deviceId: '', channelIndex: 1 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '大气压', role: 'totalPressure.pAtm', channel: { deviceId: '', channelIndex: 16 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
  { name: '大气温度', role: 'totalPressure.tAtm', channel: { deviceId: '', channelIndex: 17 }, enabled: true, precision: DEFAULT_CALIBRATION_PROBE_PRECISION },
])

const motionAxes = ref<MotionAxisConfig[]>([{ name: 'Alpha', controllerId: '', axis: 'X' }])
const deviceList = computed(() => deviceStore.profiles)
const motionControllerList = computed(() => motionStore.profiles)
const REQUIRED_CHANNEL_ROLES = ['totalPressure.pTotal', 'totalPressure.pStatic', 'totalPressure.pAtm', 'totalPressure.tAtm'] as const

const currentStepErrors = computed<string[]>(() => {
  const errs: string[] = []
  if (currentStep.value === 0) {
    if (!calibrationName.value.trim()) errs.push('请输入配置名称')
    if (pointLayout.value.alphaStep <= 0) errs.push('步长必须大于 0')
    if (pointLayout.value.alphaMax <= pointLayout.value.alphaMin) errs.push('最大值必须大于最小值')
    if (dwellTimeMs.value < 100) errs.push('稳定等待时间不能小于 100 ms')
    if (samplesPerPoint.value < 1) errs.push('每点采样次数不能小于 1')
  }
  if (currentStep.value === 1) {
    if (probeChannels.value.filter(ch => ch.enabled).some(ch => !ch.channel.deviceId || ch.channel.channelIndex < 0)) errs.push('通道映射未完成')
    if (motionAxes.value.some(a => !a.controllerId)) errs.push('运动轴必须绑定控制器')
  }
  return errs
})
const isStepValid = computed(() => currentStep.value === 2 || currentStepErrors.value.length === 0)

function nextStep() { if (currentStep.value < steps.length - 1) currentStep.value++ }
function prevStep() { if (currentStep.value > 0) currentStep.value-- }

function generatePoints() {
  const points = []
  for (let a = pointLayout.value.alphaMin; a <= pointLayout.value.alphaMax; a += pointLayout.value.alphaStep)
    points.push({ id: points.length, coordinates: { Alpha: Math.round(a * 100) / 100 } })
  return points
}

async function saveConfig() {
  isSaving.value = true
  try {
    const config: CalibrationConfig = {
      type: 'total-pressure', name: calibrationName.value, probeChannels: probeChannels.value.filter(ch => ch.enabled),
      motionAxes: motionAxes.value, points: generatePoints(), dwellTimeMs: dwellTimeMs.value, samplesPerPoint: samplesPerPoint.value, savePath: '',
      totalPressureLayout: pointLayout.value,
      sphereTankGate: { enabled: sphereTankGateEnabled.value, waitTimeSec: Math.max(0, sphereTankWaitTimeSec.value), stableTimeChannel: { ...sphereTankStableChannel.value } }
    }
    const res = await calibrationApi.saveConfig('total-pressure', JSON.parse(JSON.stringify(applyCalibrationPrecisionDefaults(config))))
    if (!res.success) throw new Error(res.error || '保存失败')
    emit('saved', applyCalibrationPrecisionDefaults(config)); emit('close')
  } catch (err) { feedbackStore.pushToast('保存失败: ' + (err instanceof Error ? err.message : String(err)), 'error')
  } finally { isSaving.value = false }
}

async function loadSavedConfig() {
  try {
    const res = await calibrationApi.getConfig('total-pressure')
    const config = res.success && res.data ? applyCalibrationPrecisionDefaults(res.data) : null
    if (!config) return
    calibrationName.value = config.name
    if (config.totalPressureLayout) pointLayout.value = { ...config.totalPressureLayout }
    config.probeChannels?.forEach(sc => { const ec = probeChannels.value.find(c => c.role ? c.role === sc.role : c.name === sc.name); if (ec) { ec.channel = { ...sc.channel }; ec.enabled = sc.enabled; ec.role = sc.role; ec.precision = sc.precision } })
    config.motionAxes?.forEach((sa, i) => { if (motionAxes.value[i]) motionAxes.value[i] = { ...sa } })
    dwellTimeMs.value = config.dwellTimeMs; samplesPerPoint.value = config.samplesPerPoint
    if (config.sphereTankGate) { sphereTankGateEnabled.value = config.sphereTankGate.enabled; sphereTankWaitTimeSec.value = config.sphereTankGate.waitTimeSec; sphereTankStableChannel.value = { ...config.sphereTankGate.stableTimeChannel } }
  } catch { /* ok */ }
}

onMounted(async () => { try { await Promise.all([deviceStore.refreshProfiles(), motionStore.refreshProfiles(), loadSavedConfig()]) } finally { isLoading.value = false } })
</script>

<template>
  <NModal :show="true" preset="card" :style="{ maxWidth: '980px', width: '92vw' }" title="总压探针校准配置" closable @close="emit('close')">
    <NSteps :current="currentStep" size="small" style="margin-bottom:16px"><NStep v-for="(s, i) in steps" :key="i" :title="s" :disabled="i > currentStep" /></NSteps>

    <NSpin v-if="isLoading" style="display:flex;justify-content:center;padding:40px 0" />
    <template v-else>
      <NAlert v-if="currentStepErrors.length > 0" type="warning" :bordered="false" style="margin-bottom:12px;font-size:12px"><template #header>请先修正以下问题</template>{{ currentStepErrors[0] }}</NAlert>

      <div v-if="currentStep === 0" class="step-content">
        <NCard size="small" :bordered="true" class="section-card">
          <template #header><NText depth="1" style="font-size:12px;font-weight:600">配置名称</NText></template>
          <NInput v-model:value="calibrationName" placeholder="输入配置名称" size="small" />
        </NCard>
        <NCard size="small" :bordered="true" class="section-card">
          <template #header><NText depth="1" style="font-size:12px;font-weight:600">点位布局</NText></template>
          <NText depth="3" style="font-size:11px;display:block;margin-bottom:8px">攻角 α 范围</NText>
          <div class="mach-grid">
            <div><NText depth="3" style="font-size:11px">最小值 (°)</NText><NInputNumber v-model:value="pointLayout.alphaMin" size="small" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:11px">最大值 (°)</NText><NInputNumber v-model:value="pointLayout.alphaMax" size="small" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:11px">步长 (°)</NText><NInputNumber v-model:value="pointLayout.alphaStep" :min="1" size="small" style="width:100%" /></div>
          </div>
          <div class="point-summary"><NText depth="3" style="font-size:12px">总校准点数</NText><NText depth="1" style="font-size:22px;font-weight:700;color:var(--accent-primary)">{{ pointCount }} 点</NText></div>
        </NCard>
        <NCard size="small" :bordered="true" class="section-card">
          <template #header><NText depth="1" style="font-size:12px;font-weight:600">采集参数</NText></template>
          <div class="param-grid">
            <div><NText depth="3" style="font-size:11px">稳定等待时间 (毫秒)</NText><NInputNumber v-model:value="dwellTimeMs" :min="100" :step="100" size="small" style="width:100%" /><NText depth="3" style="font-size:10px;display:block;margin-top:2px">到达点位后等待稳定的时间</NText></div>
            <div><NText depth="3" style="font-size:11px">每点采样次数</NText><NInputNumber v-model:value="samplesPerPoint" :min="1" :max="1000" size="small" style="width:100%" /><NText depth="3" style="font-size:10px;display:block;margin-top:2px">每个点位采集的样本数量，取平均值</NText></div>
          </div>
        </NCard>
      </div>

      <div v-if="currentStep === 1" class="step-content">
        <NCard size="small" :bordered="true" class="section-card">
          <template #header><NText depth="1" style="font-size:12px;font-weight:600">测点通道映射</NText></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th style="width:48px">启用</th><th>测点名称</th><th>数据源设备</th><th style="width:100px">通道索引</th><th style="width:80px">精度</th></tr></thead>
            <tbody><tr v-for="ch in probeChannels" :key="ch.name"><td style="text-align:center"><NCheckbox v-model:checked="ch.enabled" size="small" /></td><td><NText depth="1" style="font-size:12px">{{ ch.name }}</NText></td><td><NSelect v-model:value="ch.channel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" placeholder="选择设备" size="tiny" style="min-width:140px" :disabled="!ch.enabled" clearable /></td><td><NInputNumber v-model:value="ch.channel.channelIndex" :min="-1" :max="100" size="tiny" style="width:100%" :disabled="!ch.enabled" /></td><td><NInputNumber v-model:value="ch.precision" :min="0" :max="8" size="tiny" style="width:100%" :disabled="!ch.enabled" /></td></tr></tbody></table></div>
        </NCard>
        <NCard size="small" :bordered="true" class="section-card">
          <template #header><NText depth="1" style="font-size:12px;font-weight:600">运动轴配置</NText></template>
          <div class="table-wrap"><table class="ntable"><thead><tr><th>坐标轴</th><th>运动控制器</th><th>物理轴</th></tr></thead>
            <tbody><tr v-for="ax in motionAxes" :key="ax.name"><td><NTag size="tiny" type="primary" :bordered="false">{{ ax.name }}</NTag></td><td><NSelect v-model:value="ax.controllerId" :options="motionControllerList.map(c => ({ label: `${c.name} (${c.type})`, value: c.id }))" placeholder="选择控制器" size="tiny" style="min-width:160px" clearable /></td><td><NSelect v-model:value="ax.axis" :options="[{label:'X 轴',value:'X'},{label:'Y 轴',value:'Y'},{label:'Z 轴',value:'Z'},{label:'U 轴',value:'U'}]" size="tiny" style="width:100px" /></td></tr></tbody></table></div>
        </NCard>
        <NCard size="small" :bordered="true" class="section-card">
          <template #header><NText depth="1" style="font-size:12px;font-weight:600">球罐稳定判定</NText></template>
          <NCheckbox v-model:checked="sphereTankGateEnabled" size="small" style="margin-bottom:8px">启用球罐判定</NCheckbox>
          <div v-if="sphereTankGateEnabled" class="sphere-grid">
            <div><NText depth="3" style="font-size:11px">等待时间 (秒)</NText><NInputNumber v-model:value="sphereTankWaitTimeSec" :min="0" :step="0.1" size="small" style="width:100%" /></div>
            <div><NText depth="3" style="font-size:11px">PXI 设备</NText><NSelect v-model:value="sphereTankStableChannel.deviceId" :options="deviceList.map(d => ({ label: `${d.name} (${d.type})`, value: d.id }))" placeholder="选择设备" size="small" clearable /></div>
            <div><NText depth="3" style="font-size:11px">稳定时间通道</NText><NInputNumber v-model:value="sphereTankStableChannel.channelIndex" :min="0" size="small" style="width:100%" /></div>
          </div>
        </NCard>
      </div>

      <div v-if="currentStep === 2" class="step-content">
        <NCard size="small" :bordered="true" class="section-card">
          <template #header><NText depth="1" style="font-size:12px;font-weight:600">配置摘要</NText></template>
          <div class="summary-grid"><div class="summary-row"><NText depth="3">配置名称</NText><NText depth="1">{{ calibrationName }}</NText></div><div class="summary-row"><NText depth="3">校准类型</NText><NText depth="1">总压探针</NText></div><div class="summary-row"><NText depth="3">点位布局</NText><NText depth="1">α: {{ pointLayout.alphaMin }}° ~ {{ pointLayout.alphaMax }}°（步长 {{ pointLayout.alphaStep }}°）</NText></div><div class="summary-row"><NText depth="3">总点数</NText><NText depth="1" style="color:var(--accent-primary);font-weight:700">{{ pointCount }} 点</NText></div><div class="summary-row"><NText depth="3">启用测点</NText><NText depth="1">{{ probeChannels.filter(ch => ch.enabled).length }} 个</NText></div><div class="summary-row"><NText depth="3">稳定时间</NText><NText depth="1">{{ dwellTimeMs }} ms</NText></div><div class="summary-row"><NText depth="3">每点采样</NText><NText depth="1">{{ samplesPerPoint }} 次</NText></div></div>
        </NCard>
      </div>
    </template>

    <template #footer>
      <div style="display:flex;align-items:center;justify-content:space-between;width:100%"><div><UiButton v-if="currentStep > 0" variant="secondary" size="sm" @click="prevStep">上一步</UiButton></div><div style="display:flex;align-items:center;gap:12px"><NText depth="3" style="font-size:11px">步骤 {{ currentStep + 1 }} / {{ steps.length }}</NText><UiButton v-if="currentStep < steps.length - 1" variant="primary" size="sm" :disabled="!isStepValid" @click="nextStep">下一步</UiButton><NButton v-else size="small" type="primary" :loading="isSaving" :disabled="!isStepValid" @click="saveConfig">保存配置</NButton></div></div>
    </template>
  </NModal>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:12px; }
.section-card { font-size:12px; }
.mach-grid { display:grid; grid-template-columns:repeat(3,1fr); gap:8px; margin-bottom:12px; }
.point-summary { display:flex; align-items:center; gap:8px; padding:10px; border-radius:4px; border:1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent); background:color-mix(in srgb, var(--accent-primary) 5%, transparent); }
.param-grid { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
.table-wrap { overflow-x:auto; }
.ntable { width:100%; border-collapse:collapse; font-size:12px; }
.ntable th { text-align:left; padding:8px 10px; font-weight:600; font-size:11px; color:var(--text-muted); background:var(--bg-panel-strong); border-bottom:1px solid var(--border-default); }
.ntable td { padding:6px 10px; border-bottom:1px solid var(--border-default); }
.sphere-grid { display:grid; grid-template-columns:1fr 1fr; gap:10px; }
.summary-grid { display:flex; flex-direction:column; }
.summary-row { display:flex; justify-content:space-between; align-items:center; padding:8px 0; border-bottom:1px solid var(--border-default); gap:12px; font-size:12px; }
.summary-row:last-child { border-bottom:none; }
</style>