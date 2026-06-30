<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useLogStore } from '@stores/logStore'
import type { LogEntry, LogLevel } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import UiInput from '@components/ui/UiInput.vue'
import { fetchRecentLogs } from '@api/logSseClient'

defineProps<{
  embedded?: boolean
}>()

const logStore = useLogStore()
const { filteredEntries } = storeToRefs(logStore)
const containerRef = ref<HTMLElement | null>(null)
const autoScroll = ref(true)

const LEVELS: Array<{ value: LogLevel | null; label: string }> = [
  { value: null, label: 'All' },
  { value: 'debug', label: 'Debug' },
  { value: 'info', label: 'Info' },
  { value: 'warn', label: 'Warn' },
  { value: 'error', label: 'Error' },
]

const levelOrder: LogLevel[] = ['error', 'warn', 'info', 'debug']

const levelCounts = computed<Record<LogLevel, number>>(() => {
  return logStore.entries.reduce(
    (counts, entry) => {
      counts[entry.level] += 1
      return counts
    },
    { debug: 0, info: 0, warn: 0, error: 0 } as Record<LogLevel, number>,
  )
})

const visibleLevelCounts = computed<Record<LogLevel, number>>(() => {
  return filteredEntries.value.reduce(
    (counts, entry) => {
      counts[entry.level] += 1
      return counts
    },
    { debug: 0, info: 0, warn: 0, error: 0 } as Record<LogLevel, number>,
  )
})

const latestEntry = computed(() => logStore.entries.at(-1) ?? null)
const latestError = computed(() => findLatestByLevel('error'))
const latestWarning = computed(() => findLatestByLevel('warn'))
const activeFilterLabel = computed(() => LEVELS.find((level) => level.value === logStore.filterLevel)?.label ?? 'All')
const hasActiveFilter = computed(() => logStore.filterLevel !== null || logStore.filterSearch.trim().length > 0)

const streamStatusText = computed(() => {
  if (logStore.streamStatus === 'connected') return '实时日志已连接'
  if (logStore.streamStatus === 'connecting') return '正在连接实时日志'
  if (logStore.streamStatus === 'reconnecting') return '实时日志重连中'
  if (logStore.streamStatus === 'error') return '实时日志异常'
  return '实时日志未启动'
})

const sourceStats = computed(() => {
  const counts = new Map<string, number>()
  for (const entry of logStore.entries) {
    counts.set(entry.source, (counts.get(entry.source) ?? 0) + 1)
  }
  return Array.from(counts.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, 6)
    .map(([source, count]) => ({ source, count }))
})

function findLatestByLevel(level: LogLevel): LogEntry | null {
  for (let i = logStore.entries.length - 1; i >= 0; i -= 1) {
    if (logStore.entries[i].level === level) return logStore.entries[i]
  }
  return null
}

function onScroll(): void {
  if (!containerRef.value) return
  const el = containerRef.value
  autoScroll.value = el.scrollHeight - el.scrollTop - el.clientHeight < 50
}

watch(
  () => filteredEntries.value.length,
  () => {
    if (!autoScroll.value) return
    nextTick(() => {
      if (containerRef.value) containerRef.value.scrollTop = containerRef.value.scrollHeight
    })
  },
)

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '--:--:--.---'
  return d.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}

function formatDateTime(iso: string | null): string {
  if (!iso) return '尚未收到'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '时间无效'
  return `${d.toLocaleDateString('zh-CN')} ${formatTime(iso)}`
}

function setLevel(level: LogLevel | null): void {
  logStore.setFilterLevel(level)
}

function clearFilters(): void {
  logStore.setFilterLevel(null)
  logStore.setFilterSearch('')
}

async function copyLogs(): Promise<void> {
  try {
    const text = filteredEntries.value
      .map((entry) => `[${formatTime(entry.timestamp)}] [${entry.level.toUpperCase()}] [${entry.source}] ${entry.message}${entry.details ? '\n' + entry.details : ''}`)
      .join('\n')
    await navigator.clipboard.writeText(text)
  } catch {
    /* clipboard API may fail in restricted contexts */
  }
}

onMounted(() => {
  logStore.init()
  void fetchRecentLogs(500)
})

onBeforeUnmount(() => {
  logStore.destroy()
})
</script>

<template>
  <div class="log-viewer" :class="{ 'embedded-mode': embedded }">
    <header class="log-hero">
      <div class="log-hero__title">
        <div class="log-hero__icon" aria-hidden="true">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
            <polyline points="14 2 14 8 20 8" />
            <line x1="16" y1="13" x2="8" y2="13" />
            <line x1="16" y1="17" x2="8" y2="17" />
          </svg>
        </div>
        <div>
          <p class="log-hero__eyebrow">Diagnostics</p>
          <h1>运行日志</h1>
          <p class="log-hero__subtext">按时间、级别、来源和关键字追踪后端事件，优先暴露断线、告警和错误。</p>
        </div>
      </div>

      <div class="log-hero__actions">
        <UiButton
          size="sm"
          variant="secondary"
          :class="{ 'is-paused': logStore.isPaused }"
          :aria-label="logStore.isPaused ? '恢复日志滚动' : '暂停日志滚动'"
          @click="logStore.togglePause()"
        >
          {{ logStore.isPaused ? `恢复 (${logStore.bufferCount})` : '暂停滚动' }}
        </UiButton>
        <UiButton size="sm" variant="secondary" aria-label="复制当前筛选日志" @click="copyLogs">
          复制当前视图
        </UiButton>
        <UiButton size="sm" variant="danger" aria-label="清空前端日志缓冲" @click="logStore.clear()">
          清空缓冲
        </UiButton>
      </div>
    </header>

    <section class="log-status-grid" aria-label="日志状态总览">
      <article class="status-card status-card--stream" :class="`stream-${logStore.streamStatus}`">
        <span class="status-card__label">实时通道</span>
        <strong>{{ streamStatusText }}</strong>
        <span>{{ logStore.streamMessage || '等待日志事件' }}</span>
      </article>
      <article class="status-card">
        <span class="status-card__label">历史加载</span>
        <strong>{{ logStore.recentLoadStatus === 'loaded' ? '已同步' : logStore.recentLoadStatus === 'loading' ? '读取中' : logStore.recentLoadStatus === 'error' ? '读取失败' : '待加载' }}</strong>
        <span>{{ logStore.recentLoadMessage || '进入页面后读取最近 500 条' }}</span>
      </article>
      <article class="status-card">
        <span class="status-card__label">总计 / 当前视图</span>
        <strong>{{ logStore.entries.length }} / {{ filteredEntries.length }}</strong>
        <span>当前筛选: {{ activeFilterLabel }}{{ logStore.filterSearch ? `, ${logStore.filterSearch}` : '' }}</span>
      </article>
      <article class="status-card">
        <span class="status-card__label">最近事件</span>
        <strong>{{ latestEntry ? formatTime(latestEntry.timestamp) : '无事件' }}</strong>
        <span>{{ latestEntry ? `${latestEntry.source}: ${latestEntry.message}` : '等待后端日志进入' }}</span>
      </article>
    </section>

    <div class="log-workbench">
      <main class="log-main-panel">
        <section class="log-toolbar" aria-label="日志筛选">
          <div class="log-levels">
            <button
              v-for="level in LEVELS"
              :key="level.label"
              type="button"
              class="level-filter"
              :class="[{ active: logStore.filterLevel === level.value }, level.value ? `level-${level.value}` : 'level-all']"
              @click="setLevel(level.value)"
            >
              <span>{{ level.label }}</span>
              <strong>{{ level.value ? levelCounts[level.value] : logStore.entries.length }}</strong>
            </button>
          </div>
          <div class="log-search">
            <UiInput
              v-model="logStore.filterSearch"
              class="log-search__input"
              placeholder="搜索消息、来源或详情"
              aria-label="搜索日志"
            >
              <template #prefix>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <circle cx="11" cy="11" r="8" />
                  <line x1="21" y1="21" x2="16.65" y2="16.65" />
                </svg>
              </template>
            </UiInput>
            <UiButton v-if="hasActiveFilter" size="sm" variant="ghost" aria-label="清除日志筛选" @click="clearFilters">
              清除筛选
            </UiButton>
          </div>
        </section>

        <section class="log-table" aria-label="日志列表">
          <div class="log-table__head" aria-hidden="true">
            <span>时间</span>
            <span>级别</span>
            <span>来源</span>
            <span>消息</span>
          </div>
          <div ref="containerRef" class="log-entries" @scroll="onScroll">
            <article
              v-for="entry in filteredEntries"
              :key="entry.id"
              class="log-entry"
              :class="`log-entry--${entry.level}`"
            >
              <time class="log-time" :datetime="entry.timestamp">{{ formatTime(entry.timestamp) }}</time>
              <span class="log-badge" :class="`log-badge--${entry.level}`">{{ entry.level.toUpperCase() }}</span>
              <span class="log-source" :title="entry.source">{{ entry.source }}</span>
              <span class="log-message">
                <span>{{ entry.message }}</span>
                <code v-if="entry.details" class="log-details">{{ entry.details }}</code>
              </span>
            </article>

            <div v-if="filteredEntries.length === 0" class="log-empty">
              <strong>{{ logStore.entries.length === 0 ? '还没有收到日志' : '没有匹配当前筛选的日志' }}</strong>
              <span>{{ logStore.entries.length === 0 ? '检查后端服务是否启动，或查看实时通道状态。' : '放宽级别或关键字筛选后再查看。' }}</span>
              <UiButton v-if="hasActiveFilter" size="sm" variant="secondary" @click="clearFilters">显示全部日志</UiButton>
            </div>
          </div>
        </section>
      </main>

      <aside class="log-side-panel" aria-label="日志诊断摘要">
        <section class="side-section">
          <div class="side-section__header">
            <h2>级别分布</h2>
            <span>{{ filteredEntries.length }} visible</span>
          </div>
          <div class="level-bars">
            <div v-for="level in levelOrder" :key="level" class="level-bar" :class="`level-bar--${level}`">
              <span>{{ level.toUpperCase() }}</span>
              <div class="level-bar__track">
                <i :style="{ width: `${filteredEntries.length ? Math.max(4, (visibleLevelCounts[level] / filteredEntries.length) * 100) : 0}%` }"></i>
              </div>
              <strong>{{ visibleLevelCounts[level] }}</strong>
            </div>
          </div>
        </section>

        <section class="side-section">
          <div class="side-section__header">
            <h2>关键事件</h2>
          </div>
          <div class="event-stack">
            <article class="event-card event-card--error">
              <span>最近错误</span>
              <strong>{{ latestError ? latestError.message : '暂无错误' }}</strong>
              <small>{{ latestError ? `${formatDateTime(latestError.timestamp)} · ${latestError.source}` : 'error 级日志会显示在这里' }}</small>
            </article>
            <article class="event-card event-card--warn">
              <span>最近告警</span>
              <strong>{{ latestWarning ? latestWarning.message : '暂无告警' }}</strong>
              <small>{{ latestWarning ? `${formatDateTime(latestWarning.timestamp)} · ${latestWarning.source}` : 'warn 级日志会显示在这里' }}</small>
            </article>
          </div>
        </section>

        <section class="side-section">
          <div class="side-section__header">
            <h2>来源 Top</h2>
            <span>{{ sourceStats.length }} sources</span>
          </div>
          <div v-if="sourceStats.length" class="source-list">
            <div v-for="item in sourceStats" :key="item.source" class="source-row">
              <span :title="item.source">{{ item.source }}</span>
              <strong>{{ item.count }}</strong>
            </div>
          </div>
          <div v-else class="side-empty">暂无来源统计</div>
        </section>

        <section class="side-section side-section--meta">
          <span>最后接收</span>
          <strong>{{ formatDateTime(logStore.lastReceivedAt) }}</strong>
          <small>前端最多保留 2000 条日志；暂停时新日志进入缓冲区。</small>
        </section>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.log-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: var(--space-4);
  gap: var(--space-4);
  background: var(--bg-app);
  color: var(--text-primary);
  overflow: hidden;
}

.log-viewer.embedded-mode {
  padding: var(--space-3);
}

.log-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  flex-shrink: 0;
}

.log-hero__title {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}

.log-hero__icon {
  display: grid;
  place-items: center;
  width: 2.5rem;
  height: 2.5rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-panel);
  color: var(--accent-primary);
  box-shadow: var(--shadow-panel);
  flex-shrink: 0;
}

.log-hero__eyebrow,
.status-card__label,
.side-section__header span,
.event-card span,
.side-section--meta span {
  margin: 0;
  color: var(--text-muted);
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.log-hero h1 {
  margin: 0;
  font-size: var(--font-size-2xl);
  line-height: 1.15;
  font-weight: 700;
}

.log-hero__subtext {
  margin: var(--space-1) 0 0;
  max-width: 42rem;
  color: var(--text-secondary);
  font-size: var(--font-size-sm);
}

.log-hero__actions,
.log-search {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-shrink: 0;
}

:deep(.is-paused) {
  color: var(--accent-warning);
}

.log-status-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--space-3);
  flex-shrink: 0;
}

.status-card {
  min-width: 0;
  padding: var(--space-3);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-panel);
  box-shadow: var(--shadow-panel);
}

.status-card strong,
.side-section--meta strong {
  display: block;
  margin-top: var(--space-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--font-size-lg);
  font-weight: 700;
}

.status-card span:last-child,
.side-section--meta small {
  display: block;
  margin-top: var(--space-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
}

.status-card--stream.stream-connected strong {
  color: var(--accent-success);
}

.status-card--stream.stream-connecting strong,
.status-card--stream.stream-reconnecting strong {
  color: var(--accent-warning);
}

.status-card--stream.stream-error strong {
  color: var(--accent-danger);
}

.log-workbench {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 20rem;
  gap: var(--space-4);
  flex: 1;
  min-height: 0;
}

.log-main-panel,
.log-side-panel,
.log-table,
.side-section {
  min-width: 0;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-panel);
  box-shadow: var(--shadow-panel);
}

.log-main-panel {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.log-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-3);
  border-bottom: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel-strong) 76%, var(--bg-panel));
  flex-shrink: 0;
}

.log-levels {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.level-filter {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  min-height: 2rem;
  padding: var(--space-1) var(--space-2);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel);
  color: var(--text-secondary);
  font: inherit;
  font-size: var(--font-size-xs);
  cursor: pointer;
}

.level-filter strong {
  font-family: 'JetBrains Mono', 'Cascadia Code', 'Consolas', monospace;
  font-variant-numeric: tabular-nums;
  color: var(--text-primary);
}

.level-filter:hover,
.level-filter.active {
  border-color: var(--accent-primary);
  color: var(--text-primary);
}

.level-filter.level-error.active {
  border-color: var(--accent-danger);
}

.level-filter.level-warn.active {
  border-color: var(--accent-warning);
}

.log-search {
  min-width: 18rem;
  max-width: 28rem;
  flex: 1;
  justify-content: flex-end;
}

.log-search__input {
  width: 100%;
}

.log-table {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  border-width: 0;
  border-radius: 0;
  box-shadow: none;
}

.log-table__head,
.log-entry {
  display: grid;
  grid-template-columns: 7.25rem 4.25rem 8rem minmax(0, 1fr);
  gap: var(--space-3);
  align-items: start;
}

.log-table__head {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border-default);
  color: var(--text-muted);
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  flex-shrink: 0;
}

.log-entries {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  font-family: 'JetBrains Mono', 'Cascadia Code', 'Consolas', monospace;
  font-size: var(--font-size-xs);
  line-height: 1.55;
}

.log-entry {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 55%, transparent);
}

.log-entry:hover {
  background: color-mix(in srgb, var(--bg-panel-strong) 65%, transparent);
}

.log-entry--error {
  background: color-mix(in srgb, var(--accent-danger) 7%, transparent);
}

.log-entry--warn {
  background: color-mix(in srgb, var(--accent-warning) 5%, transparent);
}

.log-time,
.log-source {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.log-badge {
  width: fit-content;
  min-width: 3.25rem;
  padding: var(--space-0-5) var(--space-1);
  border-radius: var(--radius-sm);
  text-align: center;
  font-size: 0.625rem;
  font-weight: 800;
  letter-spacing: 0.05em;
}

.log-badge--debug {
  background: color-mix(in srgb, var(--text-muted) 12%, transparent);
  color: var(--text-secondary);
}

.log-badge--info {
  background: color-mix(in srgb, var(--accent-info) 14%, transparent);
  color: var(--accent-info);
}

.log-badge--warn {
  background: color-mix(in srgb, var(--accent-warning) 15%, transparent);
  color: var(--accent-warning);
}

.log-badge--error {
  background: color-mix(in srgb, var(--accent-danger) 15%, transparent);
  color: var(--accent-danger);
}

.log-message {
  min-width: 0;
  color: var(--text-primary);
  word-break: break-word;
  user-select: text;
}

.log-details {
  display: block;
  margin-top: var(--space-1);
  padding: var(--space-2);
  border: 1px solid color-mix(in srgb, var(--border-default) 65%, transparent);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--bg-app) 62%, transparent);
  color: var(--text-secondary);
  white-space: pre-wrap;
}

.log-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  min-height: 16rem;
  padding: var(--space-6);
  color: var(--text-secondary);
  text-align: center;
}

.log-empty strong {
  color: var(--text-primary);
  font-size: var(--font-size-base);
}

.log-side-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-3);
  overflow-y: auto;
}

.side-section {
  padding: var(--space-3);
  box-shadow: none;
}

.side-section__header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
}

.side-section__header h2 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: 700;
}

.level-bars,
.event-stack,
.source-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.level-bar,
.source-row {
  display: grid;
  grid-template-columns: 3.5rem minmax(0, 1fr) 2.5rem;
  gap: var(--space-2);
  align-items: center;
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
}

.level-bar strong,
.source-row strong {
  text-align: right;
  color: var(--text-primary);
  font-variant-numeric: tabular-nums;
}

.level-bar__track {
  height: 0.5rem;
  overflow: hidden;
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--border-default) 45%, transparent);
}

.level-bar__track i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--text-muted);
}

.level-bar--error .level-bar__track i {
  background: var(--accent-danger);
}

.level-bar--warn .level-bar__track i {
  background: var(--accent-warning);
}

.level-bar--info .level-bar__track i {
  background: var(--accent-info);
}

.event-card {
  padding: var(--space-2);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--bg-panel-strong) 54%, transparent);
}

.event-card strong {
  display: block;
  margin-top: var(--space-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-primary);
  font-size: var(--font-size-sm);
}

.event-card small {
  display: block;
  margin-top: var(--space-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
}

.event-card--error strong {
  color: var(--accent-danger);
}

.event-card--warn strong {
  color: var(--accent-warning);
}

.source-row {
  grid-template-columns: minmax(0, 1fr) 2.5rem;
  padding-bottom: var(--space-2);
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
}

.source-row span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
}

.side-empty {
  color: var(--text-muted);
  font-size: var(--font-size-sm);
}

.side-section--meta {
  margin-top: auto;
}

@media (max-width: 1280px) {
  .log-status-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .log-workbench {
    grid-template-columns: 1fr;
  }

  .log-side-panel {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
