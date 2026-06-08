<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { storageApi, reportApi } from '@api/deviceApi'
import { NButton, NInput, NSelect } from 'naive-ui'

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

    <p v-if="error" class="error-text">{{ error }}</p>

    <div class="storage-grid">
      <section class="storage-card">
        <h3>录制控制</h3>
        <div class="storage-card__field">
          <label>输出目录</label>
          <NInput v-model:value="recordingOutputDir" size="small" />
        </div>
        <div class="storage-card__field">
          <label>文件前缀</label>
          <NInput v-model:value="recordingFilePrefix" size="small" />
        </div>
        <div class="storage-card__actions">
          <NButton
            type="primary"
            size="small"
            :class="{ active: recording }"
            :disabled="busy"
            @click="toggleRecording"
          >
            {{ recording ? '停止录制' : '开始录制' }}
          </NButton>
          <span v-if="recording" class="recording-indicator">REC</span>
        </div>
        <p class="storage-card__hint">
          {{ recording ? '正在录制采集数据到 ' + recordingOutputDir : '录制未启动' }}
        </p>
      </section>

      <section class="storage-card">
        <h3>报告生成</h3>
        <div class="storage-card__field">
          <label>输出目录</label>
          <NInput v-model:value="reportOutputDir" size="small" />
        </div>
        <div class="storage-card__field">
          <label>文件前缀</label>
          <NInput v-model:value="reportFilePrefix" size="small" />
        </div>
        <div class="storage-card__field">
          <label>设备 ID</label>
          <NSelect v-model:value="reportDeviceId" :options="deviceStore.profiles.map(p => ({value: p.id, label: p.name}))" size="tiny" />
        </div>
        <NButton type="primary" size="small" :disabled="busy || generating" @click="generateReport">
          {{ generating ? '生成中...' : '生成报告' }}
        </NButton>
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
  font-size: 1.35rem;
}

.error-text {
  margin-bottom: var(--space-3);
  color: var(--accent-danger);
  font: 700 0.75rem/1.4 var(--font-family-mono, monospace);
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

.storage-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--space-4);
}

.storage-card {
  padding: var(--space-4);
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
}

.storage-card h3 {
  margin: 0 0 var(--space-3);
  font-size: 0.85rem;
  font-weight: 800;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.storage-card__field {
  margin-bottom: var(--space-3);
}

.storage-card__field label {
  display: block;
  margin-bottom: 0.25rem;
  font-size: 0.7rem;
  font-weight: 700;
  color: var(--text-muted);
}

.storage-card__field input,
.storage-card__field select {
  width: 100%;
  padding: 0.4rem 0.6rem;
  border-radius: 0.35rem;
  border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font-size: 0.85rem;
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
  border-radius: 999px;
  background: rgba(239, 68, 68, 0.12);
  color: var(--accent-danger);
  font: 800 0.65rem/1 var(--font-family-mono, monospace);
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.storage-card__hint,
.storage-card__result {
  margin: var(--space-3) 0 0;
  color: var(--text-muted);
  font-size: 0.7rem;
}


</style>
