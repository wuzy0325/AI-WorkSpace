<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import UiButton from '@components/ui/UiButton.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import { calibrationApi, type CalibrationStatus } from '@api/deviceApi'

const i18n = useI18nStore()
const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()

type CalType = 'five-hole' | 'three-hole' | 'total-pressure' | 'total-temperature'

const activeTab = ref<CalType>('five-hole')
const busy = ref(false)
const error = ref('')
const state = ref('idle')
const currentPoint = ref(0)
const totalPoints = ref(0)
const lastTaskId = ref('')
const lastResult = ref<CalibrationStatus | null>(null)

const calTypes = computed(() => [
  { id: 'five-hole' as const, label: 'Five-Hole', icon: '◎' },
  { id: 'three-hole' as const, label: 'Three-Hole', icon: '◈' },
  { id: 'total-pressure' as const, label: 'Total Pressure', icon: '⬆' },
  { id: 'total-temperature' as const, label: 'Total Temperature', icon: '🌡' },
])

const channelsText = ref('0,1,2,3')
const pressureText = ref('0,50,100,150,200')

const settingsForm = ref({
  deviceId: 'sim-1',
  averageSamples: 10,
})

const running = computed(() => state.value === 'running')
const paused = computed(() => state.value === 'paused')
const completed = computed(() => state.value === 'idle' && currentPoint.value > 0 && currentPoint.value >= totalPoints.value)

function parseChannels(v: string): number[] {
  return v.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n))
}

function parsePressurePoints(v: string): number[] {
  return v.split(',').map(s => parseFloat(s.trim())).filter(n => !isNaN(n))
}

async function run(action: () => Promise<void>) {
  busy.value = true; error.value = ''
  try { await action() } catch (err) { error.value = String(err); feedback.pushToast(String(err), 'error') }
  finally { busy.value = false }
}

async function refreshStatus() {
  try {
    const s = await calibrationApi.status()
    state.value = s.state
    currentPoint.value = s.currentPoint
    totalPoints.value = s.totalPoints
    lastResult.value = null
  } catch { /* offline */ }
}

async function startCal() {
  await run(async () => {
    const taskId = `cal-${Date.now()}`
    lastTaskId.value = taskId
    const channels = parseChannels(channelsText.value)
    const pressurePoints = parsePressurePoints(pressureText.value)
    await calibrationApi.start({ taskId, deviceId: settingsForm.value.deviceId, type: activeTab.value, channels, pressurePoints, averageSamples: settingsForm.value.averageSamples })
    await refreshStatus()
    feedback.pushToast('校准已开始', 'success')
  })
}

async function pauseCal() { await run(async () => { await calibrationApi.pause(); await refreshStatus(); feedback.pushToast('已暂停', 'info') }) }
async function resumeCal() { await run(async () => { await calibrationApi.resume(); await refreshStatus(); feedback.pushToast('已恢复', 'success') }) }
async function stopCal() { await run(async () => { await calibrationApi.stop(); await refreshStatus(); lastResult.value = null; feedback.pushToast('已停止', 'info') }) }
async function collectPoint() {
  await run(async () => {
    await calibrationApi.collect()
    await refreshStatus()
    if (completed.value && lastTaskId.value) {
      lastResult.value = await calibrationApi.getResult(lastTaskId.value)
      feedback.pushToast('校准完成！', 'success')
    }
  })
}

onMounted(refreshStatus)
</script>

<template>
  <div class="calibration-view">
    <div class="calibration-view__head">
      <p class="eyebrow">{{ i18n.t.probeCalibration }}</p>
      <h2>{{ i18n.t.probeCalibration }}</h2>
    </div>

    <section class="state-panel" :class="{ complete: completed, error: !!error && !running }">
      <div class="state-panel__indicator" />
      <div>
        <h3>{{ completed ? '校准完成' : (running ? '校准进行中' : (paused ? '校准已暂停' : '校准流程')) }}</h3>
        <p v-if="completed">所有 {{ totalPoints }} 个压力点已采集完毕</p>
        <p v-else-if="running">采集点 {{ currentPoint + 1 }} / {{ totalPoints }} — 按「采集当前点」记录数据</p>
        <p v-else-if="paused">点击恢复继续校准</p>
        <p v-else>选择探针类型和参数后开始校准</p>
      </div>
    </section>

    <p v-if="error" class="error-text">{{ error }}</p>

    <div class="calibration-tabs">
      <button
        v-for="t in calTypes"
        :key="t.id"
        class="calibration-tab"
        :class="{ active: activeTab === t.id }"
        @click="activeTab = t.id"
      >
        {{ t.icon }} {{ t.label }}
      </button>
    </div>

    <div class="calibration-content">
      <div class="calibration-content__settings">
        <h3>校准设置</h3>
        <div class="cal-field">
          <label>设备</label>
          <UiSelect v-model="settingsForm.deviceId" :options="deviceStore.profiles.map(d => ({ value: d.id, label: d.name }))" />
        </div>
        <div class="cal-field">
          <label>通道索引 (逗号分隔, 如 0,1,2,3)</label>
          <input v-model="channelsText" type="text" />
        </div>
        <div class="cal-field">
          <label>压力点 (逗号分隔, 如 0,50,100)</label>
          <input v-model="pressureText" type="text" />
        </div>
        <div class="cal-field">
          <label>平均采样数</label>
          <input v-model.number="settingsForm.averageSamples" type="number" min="1" />
        </div>
      </div>

      <div class="calibration-content__results">
        <h3>采集进度</h3>
        <div v-if="!running && !paused" class="cal-placeholder">
          配置参数后开始校准
        </div>
        <div v-else class="cal-progress">
          <div
            v-for="i in totalPoints"
            :key="i"
            class="cal-progress__dot"
            :class="{
              done: i <= currentPoint,
              active: i === currentPoint + 1,
            }"
          >
            {{ i }}
          </div>
        </div>
        <div v-if="currentPoint > 0" class="cal-results">
          <p>已采集 {{ currentPoint }} / {{ totalPoints }} 个压力点</p>
        </div>
      </div>
    </div>

    <div class="calibration-controls">
      <UiButton v-if="!running && !paused" variant="primary" :disabled="busy" @click="startCal">
        开始校准
      </UiButton>
      <UiButton v-if="running" variant="secondary" :disabled="busy" @click="pauseCal">
        暂停
      </UiButton>
      <UiButton v-if="paused" variant="primary" :disabled="busy" @click="resumeCal">
        恢复
      </UiButton>
      <UiButton v-if="running || paused" variant="danger" :disabled="busy" @click="stopCal">
        停止
      </UiButton>
      <UiButton v-if="running" variant="ghost" size="sm" :disabled="busy" @click="collectPoint">
        采集当前点
      </UiButton>
    </div>

    <div v-if="lastResult" class="calibration-results">
      <h3>校准结果 — {{ lastResult.taskId }}</h3>
      <table class="results-table">
        <thead>
          <tr><th>点</th><th>目标压力</th><th>时间戳</th><th>通道值</th></tr>
        </thead>
        <tbody>
          <tr v-for="r in (lastResult.results ?? [])" :key="r.pointIndex">
            <td>{{ r.pointIndex }}</td>
            <td>{{ r.targetPressure }}</td>
            <td>{{ r.timestamp }}</td>
            <td>{{ JSON.stringify(r.values) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="calibration-chart">
      <h3>实时数据</h3>
      <div class="cal-chart-placeholder">
        <p>校准曲线将在后续集成 ECharts 后展示</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.calibration-view {
  padding: var(--space-4);
  min-height: 0;
  overflow-y: auto;
}

.calibration-view__head {
  margin-bottom: var(--space-4);
}

.calibration-view__head h2 {
  margin: 0;
  font-size: 1.35rem;
}

.state-panel {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
  padding: var(--space-3);
  border-radius: 0.5rem;
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.15);
  color: #f59e0b;
  font-size: 0.75rem;
}

.state-panel__indicator {
  width: 0.6rem;
  height: 0.6rem;
  margin-top: 0.25rem;
  border-radius: 999px;
  background: currentColor;
  box-shadow: 0 0 12px currentColor;
  flex-shrink: 0;
}

.state-panel h3, .state-panel p { margin: 0; }
.state-panel p { margin-top: 0.25rem; color: var(--text-muted); line-height: 1.5; }

.error-text {
  margin-bottom: var(--space-3);
  color: var(--accent-danger);
  font: 700 0.75rem/1.4 var(--font-family-mono, monospace);
}

.calibration-tabs {
  display: flex;
  gap: 0.25rem;
  margin-bottom: var(--space-4);
  padding: 0.25rem;
  border-radius: 0.5rem;
  background: rgba(0, 0, 0, 0.2);
}

.calibration-tab {
  padding: 0.4rem 1rem;
  border-radius: 0.375rem;
  background: transparent;
  color: var(--text-muted);
  font-size: 0.8rem;
  font-weight: 700;
  flex: 1;
}

.calibration-tab.active {
  color: #f8fbff;
  background: rgba(16, 185, 129, 0.16);
}

.calibration-content {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--space-4);
  margin-bottom: var(--space-4);
}

.calibration-content__settings,
.calibration-content__results {
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
}

.calibration-content h3 {
  margin: 0 0 var(--space-3);
  font-size: 0.85rem;
  font-weight: 800;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.cal-field {
  margin-bottom: var(--space-3);
}

.cal-field label {
  display: block;
  margin-bottom: 0.25rem;
  font-size: 0.7rem;
  font-weight: 700;
  color: var(--text-muted);
}

.cal-field input,
.cal-field select {
  width: 100%;
  padding: 0.4rem 0.6rem;
  border-radius: 0.35rem;
  border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font-size: 0.85rem;
}

.cal-placeholder {
  min-height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 0.8rem;
}

.cal-progress {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.cal-progress__dot {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  font: 700 0.7rem/1 var(--font-family-mono, monospace);
  background: rgba(148, 163, 184, 0.1);
  color: var(--text-muted);
}

.cal-progress__dot.done {
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
}

.cal-progress__dot.active {
  border: 2px solid #10b981;
  box-shadow: 0 0 12px rgba(16, 185, 129, 0.3);
}

.cal-results {
  margin-top: var(--space-3);
  font-size: 0.8rem;
  color: var(--text-secondary);
}

.calibration-controls {
  display: flex;
  gap: 0.5rem;
  margin-bottom: var(--space-4);
}

.calibration-results {
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
  margin-bottom: var(--space-4);
}

.calibration-results h3 {
  margin: 0 0 var(--space-3);
  font-size: 0.85rem;
  font-weight: 800;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.results-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.75rem;
  font-family: var(--font-family-mono, monospace);
}

.results-table th, .results-table td {
  padding: 0.4rem 0.6rem;
  text-align: left;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.results-table th {
  color: var(--text-muted);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-size: 0.65rem;
}

.results-table td {
  color: var(--text-primary);
}

.calibration-chart {
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
}

.calibration-chart h3 {
  margin: 0 0 var(--space-3);
  font-size: 0.85rem;
  font-weight: 800;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.cal-chart-placeholder {
  min-height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 0.8rem;
}

</style>
