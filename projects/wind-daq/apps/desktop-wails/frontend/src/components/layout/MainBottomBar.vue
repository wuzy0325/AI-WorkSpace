<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch } from 'vue'
import { Timer, Clock, HardDrive, FileText, AlertCircle, Folder } from '@lucide/vue'

// RecordingStats 录制运行时统计，由 MainDashboardView 从后端 Status() 同步。
interface RecordingStats {
  outputDir?: string
  currentFile?: string
  fileSize?: number
  fileCount?: number
  recordCount?: number
  durationMs?: number
  droppedCount?: number
  lastError?: string
}

const props = withDefaults(
  defineProps<{
    isAcquiring: boolean
    t?: Record<string, string>
    totalDevices: number
    /** 是否正在录制 */
    isRecording?: boolean
    /** 录制运行时统计（仅 isRecording=true 时展示） */
    recordingStats?: RecordingStats
  }>(),
  {
    t: () => ({}),
    isRecording: false,
    recordingStats: () => ({}),
  }
)

const emit = defineEmits<{
}>()

const currentTime = ref('12:00:00')
const elapsedTime = ref('00:00:00')
const startTimestamp = ref<number | null>(null)
let timeTimer: number | null = null
let elapsedTimer: number | null = null

function updateTime() {
  const now = new Date()
  currentTime.value = now.toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
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

const isRunning = computed(() => props.isAcquiring)

// 格式化字节为人类可读单位（KB/MB/GB）
function formatBytes(bytes: number | undefined): string {
  if (!bytes || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`
}

// 格式化记录条数（千分位）
function formatCount(count: number | undefined): string {
  if (!count || count <= 0) return '0'
  return count.toLocaleString('en-US')
}

// 录制时长（优先使用后端 DurationMs，否则本地估算）
const recordingDuration = computed(() => {
  const ms = props.recordingStats?.durationMs
  if (!ms || ms <= 0) return '00:00:00'
  const hours = Math.floor(ms / 3600000)
  const minutes = Math.floor((ms % 3600000) / 60000)
  const seconds = Math.floor((ms % 60000) / 1000)
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
})

// 保存路径：拼接 outputDir + currentFile 形成完整路径。
// 后端 sink Stop 后仍保留 outputDir/currentFile，因此停止后该值仍可展示，
// 便于用户定位上次保存的文件。两者皆空（未启动过）时返回空串。
const recordingFilePath = computed(() => {
  const dir = props.recordingStats?.outputDir ?? ''
  const file = props.recordingStats?.currentFile ?? ''
  if (!dir && !file) return ''
  if (!dir) return file
  if (!file) return dir
  // 用系统分隔符拼接（Windows 为 \）；这里简单用 \ 即可，
  // 后端 outputDir 已是平台原生路径
  return `${dir}\\${file}`
})

// 是否显示保存路径项：只要存在路径（录制中或停止后保留）就显示
const showFilePath = computed(() => recordingFilePath.value !== '')

onMounted(() => {
  updateTime()
  timeTimer = window.setInterval(updateTime, 1000)
})

onBeforeUnmount(() => {
  if (timeTimer !== null) {
    window.clearInterval(timeTimer)
    timeTimer = null
  }
  if (elapsedTimer !== null) {
    window.clearInterval(elapsedTimer)
    elapsedTimer = null
  }
})

watch(isRunning, (newVal, oldVal) => {
  if (newVal && !oldVal) {
    startTimestamp.value = Date.now()
    updateElapsedTime()
    elapsedTimer = window.setInterval(updateElapsedTime, 1000)
  } else if (!newVal && oldVal) {
    if (elapsedTimer !== null) {
      window.clearInterval(elapsedTimer)
      elapsedTimer = null
    }
    startTimestamp.value = null
    elapsedTime.value = '00:00:00'
  }
}, { immediate: true })
</script>

<template>
  <footer class="main-bottom-bar">
    <!-- Left: Status Info -->
    <div class="main-bottom-bar__left">
      <!-- Status Pill (从顶部栏移入) -->
      <div
        data-test="system-status-pill"
        class="main-bottom-bar__status-pill"
        :class="isAcquiring ? 'main-bottom-bar__status-pill--active' : 'main-bottom-bar__status-pill--idle'"
      >
        <span class="main-bottom-bar__status-dot" :class="{ 'status-pulse': isAcquiring }"></span>
        <span data-test="system-status-label" class="main-bottom-bar__status-text">
          {{ isAcquiring ? (t.acquiring || '采集开启') : (t.idle || '就绪') }}
        </span>
      </div>

      <!-- Device Count -->
      <div class="main-bottom-bar__status-item">
        <span class="main-bottom-bar__status-label">{{ t.totalDevices || '设备' }}</span>
        <span class="main-bottom-bar__status-value">{{ totalDevices }}</span>
      </div>

      <!-- 保存路径：录制中和停止后都显示，便于用户定位文件位置 -->
      <div
        v-if="showFilePath"
        class="main-bottom-bar__status-item main-bottom-bar__filepath"
        :class="{ 'main-bottom-bar__filepath--active': isRecording }"
        data-test="recording-file-path"
      >
        <Folder class="w-3.5 h-3.5" />
        <span class="main-bottom-bar__status-label">{{ t.filePath || '文件路径' }}</span>
        <span
          class="main-bottom-bar__status-value main-bottom-bar__status-value--mono main-bottom-bar__filepath-value"
          :title="recordingFilePath"
        >
          {{ recordingFilePath }}
        </span>
      </div>

      <!-- Recording Stats（仅录制中展示） -->
      <div v-if="isRecording" class="main-bottom-bar__recording-stats" data-test="recording-stats">
        <div class="main-bottom-bar__status-item">
          <span class="main-bottom-bar__status-label">{{ t.fileSize || '大小' }}</span>
          <span class="main-bottom-bar__status-value main-bottom-bar__status-value--mono">
            <HardDrive class="w-3 h-3" />
            {{ formatBytes(recordingStats?.fileSize) }}
          </span>
        </div>
        <div class="main-bottom-bar__status-item">
          <span class="main-bottom-bar__status-label">{{ t.recordCount || '记录数' }}</span>
          <span class="main-bottom-bar__status-value main-bottom-bar__status-value--mono">
            <FileText class="w-3 h-3" />
            {{ formatCount(recordingStats?.recordCount) }}
          </span>
        </div>
        <div class="main-bottom-bar__status-item">
          <span class="main-bottom-bar__status-label">{{ t.recordingDuration || '录制时长' }}</span>
          <span class="main-bottom-bar__status-value main-bottom-bar__status-value--mono">
            <Timer class="w-3 h-4 text-[var(--state-success)]" />
            {{ recordingDuration }}
          </span>
        </div>
        <!-- 丢弃告警：仅在 droppedCount > 0 时显示 -->
        <div v-if="recordingStats?.droppedCount && recordingStats.droppedCount > 0" class="main-bottom-bar__status-item main-bottom-bar__status-item--warn">
          <span class="main-bottom-bar__status-label">{{ t.droppedCount || '丢弃' }}</span>
          <span class="main-bottom-bar__status-value main-bottom-bar__status-value--mono">
            {{ formatCount(recordingStats.droppedCount) }}
          </span>
        </div>
      </div>
      <!-- 错误指示：lastError 独立展示，录制停止后仍可见 -->
      <!-- lastError 在 Start() 时重置，不是 Stop()，因此停止后仍需展示失败原因 -->
      <div v-if="recordingStats?.lastError" class="main-bottom-bar__status-item main-bottom-bar__status-item--error" :title="recordingStats.lastError">
        <AlertCircle class="w-3 h-3" />
        <span class="main-bottom-bar__status-value main-bottom-bar__status-value--mono">{{ recordingStats.lastError }}</span>
      </div>
    </div>

    <!-- Right: Time Stats -->
    <div class="main-bottom-bar__stats">
      <!-- Elapsed Time -->
      <div class="main-bottom-bar__stat">
        <span class="main-bottom-bar__stat-label">{{ t.elapsedTime || '运行时间' }}</span>
        <div class="main-bottom-bar__stat-value">
          <Timer class="w-4 h-4 text-[var(--state-success)]" />
          <span class="mono-font">{{ elapsedTime }}</span>
        </div>
      </div>

      <!-- System Time -->
      <div class="main-bottom-bar__stat">
        <span class="main-bottom-bar__stat-label">{{ t.systemTime || '系统时间' }}</span>
        <div class="main-bottom-bar__stat-value">
          <Clock class="w-4 h-4 text-[var(--text-tertiary)]" />
          <span class="mono-font text-[var(--text-tertiary)]">{{ currentTime }}</span>
        </div>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.main-bottom-bar {
  height: 64px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 2rem;
  background: var(--bg-panel-strong);
  border-top: 1px solid var(--border-default);
}

:root[data-theme='light'] .main-bottom-bar {
  background: var(--bg-panel-strong);
  border-top: 1px solid var(--border-default);
}

.main-bottom-bar__left {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

/* 录制统计区：与设备数等间距分隔，整体左侧留出分隔条 */
.main-bottom-bar__recording-stats {
  display: flex;
  align-items: center;
  gap: 1.25rem;
  padding-left: 1.5rem;
  margin-left: 0.5rem;
  border-left: 1px solid var(--border-default);
}

/* 保存路径项：横向布局（icon + label + value 同行），便于展示长路径 */
.main-bottom-bar__filepath {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  /* 限制最大宽度，避免长路径挤占右侧时间区 */
  max-width: 360px;
  /* 未录制时使用次要色，提示用户这是"上次保存路径" */
  color: var(--text-muted);
  padding-left: 1.5rem;
  margin-left: 0.5rem;
  border-left: 1px solid var(--border-default);
}

/* 录制中：用主色高亮 Folder icon 与路径，与 idle 区分 */
.main-bottom-bar__filepath--active {
  color: var(--accent-primary);
}

.main-bottom-bar__filepath--active .main-bottom-bar__filepath-value {
  color: var(--accent-primary);
}

/* 路径值：等宽 + 省略号，title 提供完整路径 hover 查看 */
.main-bottom-bar__filepath-value {
  color: var(--text-secondary);
  font-size: 0.8125rem;
  /* 覆盖 status-value--mono 的 max-width: 200px，让路径有更多展示空间 */
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 警告/错误状态颜色 */
.main-bottom-bar__status-item--warn .main-bottom-bar__status-value {
  color: var(--accent-warning, #f59e0b);
}

.main-bottom-bar__status-item--error {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  color: var(--accent-danger);
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main-bottom-bar__status-item--error .main-bottom-bar__status-value {
  color: var(--accent-danger);
  font-size: 0.75rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 等宽值（文件名、大小、记录数等）便于扫读 */
.main-bottom-bar__status-value--mono {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
  font-variant-numeric: tabular-nums;
  font-size: 0.8125rem;
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 状态 Pill 样式（从顶部栏移入） */
.main-bottom-bar__status-pill {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.main-bottom-bar__status-pill--active {
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent);
  color: var(--accent-primary);
}

.main-bottom-bar__status-pill--idle {
  background: rgba(148, 163, 184, 0.1);
  border: 1px solid rgba(148, 163, 184, 0.2);
  color: var(--text-muted);
}

.main-bottom-bar__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.status-pulse {
  animation: status-pulse 2s ease-in-out infinite;
}

@keyframes status-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.main-bottom-bar__status-item {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.main-bottom-bar__status-label {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.main-bottom-bar__status-value {
  font-size: 0.875rem;
  font-weight: 700;
}

.main-bottom-bar__stats {
  display: flex;
  align-items: center;
  gap: 3rem;
}

.main-bottom-bar__stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
}

.main-bottom-bar__stat-label {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.main-bottom-bar__stat-value {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 1.25rem;
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

:root[data-theme='light'] .main-bottom-bar__stat-value {
  color: var(--text-primary);
}

.mono-font {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
  font-variant-numeric: tabular-nums;
}
</style>
