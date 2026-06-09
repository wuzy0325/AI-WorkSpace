<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { storageApi, reportApi } from '@api/deviceApi'
import UiButton from '@components/ui/UiButton.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiFormField from '@components/ui/UiFormField.vue'
import UiErrorState from '@components/ui/UiErrorState.vue'

const deviceStore = useDeviceStore()
const feedback = useFeedbackStore()

const recording = ref(false)
const recordingOutputDir = ref('data/recordings')
const recordingFilePrefix = ref('run')
const busy = ref(false)
const error = ref('')

const reportOutputDir = ref('data/reports')
const reportFilePrefix = ref('report')
const reportDeviceId = ref('sim-1')

const lastReportPath = ref('')
const generating = ref(false)

async function refreshStatus() {
  try {
    const s = await storageApi.status()
    recording.value = s.recording
  } catch { /* offline */ }
}

async function toggleRecording() {
  busy.value = true
  try {
    if (recording.value) {
      await storageApi.stop()
      recording.value = false
      feedback.pushToast('录制已停止', 'info')
    } else {
      await storageApi.start(recordingOutputDir.value, recordingFilePrefix.value)
      recording.value = true
      feedback.pushToast('录制已开始', 'success')
    }
  } catch (err) { feedback.pushToast(String(err), 'error') }
  finally { busy.value = false }
}

async function generateReport() {
  busy.value = true; error.value = ''; generating.value = true
  try {
    const result = await reportApi.generate(reportOutputDir.value, reportFilePrefix.value, reportDeviceId.value)
    lastReportPath.value = result.path
    feedback.pushToast('报告已生成: ' + result.path, 'success')
  } catch (err) { error.value = String(err); feedback.pushToast(String(err), 'error') }
  finally { busy.value = false; generating.value = false }
}

onMounted(refreshStatus)
</script>

<template>
  <div class="storage-view">
    <div class="storage-view__head">
      <p class="eyebrow">Storage &amp; Reports</p>
      <h2>数据存储与报告</h2>
    </div>

    <section class="state-panel">
      <div class="state-panel__indicator" />
      <div>
        <h3>存储与报告 API 已接入</h3>
        <p>录制控制与报告生成已通过 HTTP API 接入后端；开始录制后数据写入 CSV，报告生成通过 CSV writer 导出。</p>
      </div>
    </section>

    <UiErrorState
      v-if="error"
      title="操作失败"
      :message="error"
    >
      <template #action>
        <UiButton variant="secondary" size="sm" @click="error = ''">
          关闭
        </UiButton>
      </template>
    </UiErrorState>

    <div class="storage-grid">
      <section class="storage-card">
        <h3>录制控制</h3>
        <UiFormField label="输出目录">
          <UiInput v-model="recordingOutputDir" placeholder="data/recordings" />
        </UiFormField>
        <UiFormField label="文件前缀">
          <UiInput v-model="recordingFilePrefix" placeholder="run" />
        </UiFormField>
        <div class="storage-card__actions">
          <UiButton
            :variant="recording ? 'danger' : 'primary'"
            size="sm"
            :disabled="busy"
            @click="toggleRecording"
          >
            {{ recording ? '停止录制' : '开始录制' }}
          </UiButton>
          <span v-if="recording" class="recording-indicator">REC</span>
        </div>
        <p class="storage-card__hint">
          {{ recording ? '正在录制采集数据到 ' + recordingOutputDir : '录制未启动' }}
        </p>
      </section>

      <section class="storage-card">
        <h3>报告生成</h3>
        <UiFormField label="输出目录">
          <UiInput v-model="reportOutputDir" placeholder="data/reports" />
        </UiFormField>
        <UiFormField label="文件前缀">
          <UiInput v-model="reportFilePrefix" placeholder="report" />
        </UiFormField>
        <UiFormField label="设备 ID">
          <UiSelect v-model="reportDeviceId" :options="deviceStore.profiles.map(p => ({value: p.id, label: p.name}))" />
        </UiFormField>
        <UiButton variant="primary" size="sm" :disabled="busy || generating" @click="generateReport">
          {{ generating ? '生成中...' : '生成报告' }}
        </UiButton>
        <p v-if="lastReportPath" class="storage-card__result">
          上次报告: {{ lastReportPath }}
        </p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.storage-view {
  padding: var(--space-4);
  min-height: 0;
  overflow-y: auto;
}

.storage-view__head {
  margin-bottom: var(--space-4);
}

.storage-view__head h2 {
  margin: 0;
  font-size: var(--font-size-2xl);
}

.state-panel {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
  padding: var(--space-3);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--state-warning) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--state-warning) 15%, transparent);
  color: var(--state-warning);
  font-size: var(--font-size-xs);
}

.state-panel__indicator {
  width: 0.6rem;
  height: 0.6rem;
  margin-top: 0.25rem;
  border-radius: var(--radius-pill);
  background: currentColor;
  box-shadow: 0 0 12px currentColor;
  flex-shrink: 0;
}

.state-panel h3, .state-panel p { margin: 0; }
.state-panel p { margin-top: var(--space-1); color: var(--text-muted); line-height: var(--line-height-base); }

.storage-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--space-4);
}

.storage-card {
  padding: var(--space-4);
  border-radius: var(--radius-xl);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
}

.storage-card h3 {
  margin: 0 0 var(--space-3);
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-black);
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.storage-card__actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.recording-indicator {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.2rem 0.5rem;
  border-radius: var(--radius-pill);
  background: color-mix(in srgb, var(--accent-danger) 12%, transparent);
  color: var(--accent-danger);
  font: var(--font-weight-black) var(--font-size-micro)/1 var(--font-family-mono);
  animation: ui-rec-pulse var(--motion-slow) var(--easing-standard) infinite;
}

@keyframes ui-rec-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.storage-card__hint,
.storage-card__result {
  margin: var(--space-3) 0 0;
  color: var(--text-muted);
  font-size: var(--font-size-2xs);
}
</style>
