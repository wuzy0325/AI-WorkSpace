<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch } from 'vue'
import { Timer, Clock, FileText, FolderOpen } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useRecordingStore } from '@stores/recordingStore'

const deviceStore = useDeviceStore()
const recordingStore = useRecordingStore()

const currentTime = ref('00:00:00')
const elapsedTime = ref('00:00:00')
const startTimestamp = ref<number | null>(null)
let timeTimer: ReturnType<typeof setInterval> | null = null
let elapsedTimer: ReturnType<typeof setInterval> | null = null

function updateTime() {
  const now = new Date()
  currentTime.value = now.toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function updateElapsedTime() {
  if (!startTimestamp.value) {
    elapsedTime.value = '00:00:00'
    return
  }
  const now = Date.now()
  const elapsed = now - startTimestamp.value
  const hours = Math.floor(elapsed / 3600000)
  const minutes = Math.floor((elapsed % 3600000) / 60000)
  const seconds = Math.floor((elapsed % 60000) / 1000)
  elapsedTime.value = `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}

const isAcquiring = computed(() =>
  deviceStore.profiles.some((p) => deviceStore.acquiringFor(p.id))
)
const totalDevices = computed(() => deviceStore.profiles.length)
const connectedDevices = computed(
  () => deviceStore.profiles.filter((p) => deviceStore.statusFor(p.id) === 'Connected' || deviceStore.statusFor(p.id) === 'Acquiring').length
)
const recordingCount = computed(() => recordingStore.snapshotCount)

// 状态栏显示的保存路径：优先显示当前正在写入的完整文件路径，
// 缺省时退化为保存目录（例如尚未创建文件或仅设置了目录的场景）
const savedPath = computed(() => recordingStore.currentFile || recordingStore.outputDir)
const savedPathLabel = computed(() => (recordingStore.currentFile ? '保存文件' : '保存目录'))
const savedPathIcon = computed(() => (recordingStore.currentFile ? FileText : FolderOpen))

onMounted(() => {
  updateTime()
  timeTimer = setInterval(updateTime, 1000)
  void recordingStore.refreshStatus()
})

onBeforeUnmount(() => {
  if (timeTimer !== null) clearInterval(timeTimer)
  if (elapsedTimer !== null) clearInterval(elapsedTimer)
})

watch(isAcquiring, (newVal, oldVal) => {
  if (newVal && !oldVal) {
    startTimestamp.value = Date.now()
    updateElapsedTime()
    elapsedTimer = setInterval(updateElapsedTime, 1000)
  } else if (!newVal && oldVal) {
    if (elapsedTimer !== null) {
      clearInterval(elapsedTimer)
      elapsedTimer = null
    }
    startTimestamp.value = null
    elapsedTime.value = '00:00:00'
  }
}, { immediate: true })

</script>

<template>
  <footer class="bottombar">
    <div class="bottombar__status">
      <div class="bottombar__status-item" data-testid="status-acquisition">
        <span class="bottombar__status-label">采集状态</span>
        <span class="bottombar__status-value" :class="{ 'bottombar__status-value--active': isAcquiring }">
          {{ isAcquiring ? '运行中' : '已停止' }}
        </span>
      </div>
      <div class="bottombar__status-item" data-testid="status-recording">
        <span class="bottombar__status-label">记录状态</span>
        <span class="bottombar__status-value" :class="{ 'bottombar__status-value--rec': recordingStore.isRecording }">
          {{ recordingStore.isRecording ? '保存中' : '未保存' }}
        </span>
      </div>
      <div class="bottombar__status-item" data-testid="status-devices">
        <span class="bottombar__status-label">设备</span>
        <span class="bottombar__status-value mono">{{ totalDevices }}</span>
      </div>
      <div class="bottombar__status-item" data-testid="status-online">
        <span class="bottombar__status-label">在线</span>
        <span class="bottombar__status-value mono bottombar__status-value--active">{{ connectedDevices }}</span>
      </div>
      <div class="bottombar__status-item" data-testid="status-recorded">
        <span class="bottombar__status-label">已记录</span>
        <span class="bottombar__status-value mono" :class="{ 'bottombar__status-value--rec': recordingStore.isRecording }">{{ recordingCount }}</span>
      </div>
      <div v-if="recordingStore.isRecording" class="bottombar__status-item" data-testid="status-dropped">
        <span class="bottombar__status-label">丢弃</span>
        <span
          class="bottombar__status-value mono"
          :class="{ 'bottombar__status-value--warn': recordingStore.droppedCount > 0 }"
        >{{ recordingStore.droppedCount }}</span>
      </div>
      <div v-if="recordingStore.isRecording && recordingStore.fileCount > 0" class="bottombar__status-item" data-testid="status-filecount">
        <span class="bottombar__status-label">文件数</span>
        <span class="bottombar__status-value mono">{{ recordingStore.fileCount }}</span>
      </div>
      <div v-if="recordingStore.lastError" class="bottombar__status-item" data-testid="status-recerror">
        <span class="bottombar__status-label">录制错误</span>
        <span class="bottombar__status-value bottombar__status-value--danger" :title="recordingStore.lastError">
          {{ recordingStore.lastError }}
        </span>
      </div>
      <div v-if="savedPath" class="bottombar__rec-folder" :class="{ 'bottombar__rec-folder--active': recordingStore.isRecording }" :title="savedPath">
        <component :is="savedPathIcon" class="bottombar__rec-icon" />
        <span class="bottombar__rec-label">{{ savedPathLabel }}</span>
        <span class="bottombar__rec-path">{{ savedPath }}</span>
      </div>
    </div>

    <div class="bottombar__stats">
      <div class="bottombar__stat">
        <span class="bottombar__stat-label">运行时间</span>
        <div class="bottombar__stat-value">
          <Timer class="bottombar__stat-icon bottombar__stat-icon--accent" />
          <span class="mono">{{ elapsedTime }}</span>
        </div>
      </div>
      <div class="bottombar__stat">
        <span class="bottombar__stat-label">系统时间</span>
        <div class="bottombar__stat-value">
          <Clock class="bottombar__stat-icon" />
          <span class="mono bottombar__stat-value--muted">{{ currentTime }}</span>
        </div>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.bottombar {
  height: var(--layout-bottombar-height);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1.5rem;
  padding: 0 1.5rem;
  background: var(--bottombar-bg);
  backdrop-filter: blur(14px) saturate(140%);
  -webkit-backdrop-filter: blur(14px) saturate(140%);
  border-top: 1px solid var(--border-default);
  box-shadow: 0 -20px 40px rgba(0, 0, 0, 0.15);
}

[data-theme='light'] .bottombar {
  box-shadow: 0 -20px 40px rgba(15, 23, 42, 0.05);
}

.bottombar__status {
  display: flex;
  align-items: center;
  gap: 1.25rem;
  min-width: 0;
  flex: 1;
  flex-wrap: wrap;
}

.bottombar__status-item {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.bottombar__status-label {
  font-size: 0.55rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.bottombar__status-value {
  font-size: var(--font-size-base);
  font-weight: 700;
  color: var(--text-primary);
}

.bottombar__status-value--active {
  color: var(--accent);
}

.bottombar__status-value--rec {
  color: var(--danger);
}

.bottombar__status-value--warn {
  color: var(--warning, #f59e0b);
}

.bottombar__status-value--danger {
  color: var(--danger);
  font-weight: 600;
  max-width: 22rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bottombar__rec-folder {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.3rem 0.65rem;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  min-width: 0;
  max-width: 32rem;
}

.bottombar__rec-folder--active {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--accent) 10%, var(--btn-bg));
}

.bottombar__rec-folder--active .bottombar__rec-icon,
.bottombar__rec-folder--active .bottombar__rec-path {
  color: var(--accent);
}

.bottombar__rec-label {
  font-size: 0.7rem;
  color: var(--text-muted);
  flex-shrink: 0;
}

.bottombar__rec-icon {
  width: 12px;
  height: 12px;
  color: var(--text-muted);
  flex-shrink: 0;
}

.bottombar__rec-path {
  font-size: 0.7rem;
  color: var(--text-secondary);
  font-family: var(--font-family-mono);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bottombar__stats {
  display: flex;
  align-items: center;
  gap: 1.75rem;
  flex-shrink: 0;
}

.bottombar__stat {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.2rem;
}

.bottombar__stat-label {
  font-size: 0.55rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.bottombar__stat-value {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: var(--font-size-xl);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

.bottombar__stat-value--muted {
  color: var(--text-secondary);
  font-weight: 700;
}

.bottombar__stat-icon {
  width: 16px;
  height: 16px;
  color: var(--text-muted);
}

.bottombar__stat-icon--accent {
  color: var(--accent);
}

@media (max-width: 1024px) {
  .bottombar__rec-folder,
  .bottombar__stats {
    display: none;
  }
}
</style>
