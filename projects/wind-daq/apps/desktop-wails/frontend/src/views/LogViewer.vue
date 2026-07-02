<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { CATEGORY_LABELS, LOG_GROUP_LABELS, mapCategoryToGroup, useLogStore } from '@stores/logStore'
import type { LogGroup } from '@stores/logStore'
import type { LogCategory, LogEntry, LogLevel } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import UiInput from '@components/ui/UiInput.vue'
import { fetchRecentLogs, startLogSubscription, stopLogSubscription, fetchCategoryStates, setCategoryEnabled } from '@api/logSseClient'

defineProps<{
  embedded?: boolean
}>()

const logStore = useLogStore()
const { filteredEntries } = storeToRefs(logStore)
const containerRef = ref<HTMLElement | null>(null)
const autoScroll = ref(true)

// 日志类型开关面板默认收起，避免占用主要视觉空间。
// 需要时点击「类型开关」按钮展开，对齐 daq-t1603 的简洁布局。
const showCategoryPanel = ref(false)

// 所有已知的日志分类及其中文标签（用于后端分类开关面板）
const ALL_CATEGORIES: Array<{ value: LogCategory; label: string }> = [
  { value: 'system', label: CATEGORY_LABELS.system },
  { value: 'hardware-send', label: CATEGORY_LABELS['hardware-send'] },
  { value: 'hardware-recv', label: CATEGORY_LABELS['hardware-recv'] },
  { value: 'acquisition', label: CATEGORY_LABELS.acquisition },
  { value: 'business', label: CATEGORY_LABELS.business },
]

// 正在切换分类开关的集合，防抖避免快速点击导致请求堆积
const togglingCategories = ref(new Set<string>())

async function handleCategoryToggle(category: LogCategory, enabled: boolean): Promise<void> {
  if (togglingCategories.value.has(category)) return
  togglingCategories.value.add(category)
  try {
    await setCategoryEnabled(category, enabled)
  } finally {
    togglingCategories.value.delete(category)
  }
}

// 判断某个 category 是否启用：未显式设置时默认启用
function isCategoryEnabled(category: LogCategory): boolean {
  const states = logStore.categoryEnabled
  if (!(category in states)) return true
  return states[category]
}

// 级别过滤选项：minLevel 语义，显示该级别及更高严重度。
// 默认 'info' 隐藏 Debug，避免采集期间高频命令收发日志刷屏（对齐 daq-t1603）。
const LEVELS: Array<{ value: LogLevel; label: string }> = [
  { value: 'debug', label: 'Debug' },
  { value: 'info', label: 'Info' },
  { value: 'warn', label: 'Warn' },
  { value: 'error', label: 'Error' },
]

// 分组过滤选项：'all' 表示全部
const GROUPS: Array<{ value: LogGroup | 'all'; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'system', label: LOG_GROUP_LABELS.system },
  { value: 'communication', label: LOG_GROUP_LABELS.communication },
  { value: 'acquisition', label: LOG_GROUP_LABELS.acquisition },
  { value: 'business', label: LOG_GROUP_LABELS.business },
]

const hasActiveFilter = computed(
  () =>
    logStore.minLevel !== 'debug' ||
    logStore.filterGroup !== 'all' ||
    logStore.filterSearch.trim().length > 0,
)

// 实时通道状态简短文案（标题栏显示）
const streamStatusText = computed(() => {
  switch (logStore.streamStatus) {
    case 'connected':
      return '已连接'
    case 'connecting':
      return '连接中'
    case 'reconnecting':
      return '重连中'
    case 'error':
      return '异常'
    default:
      return '未启动'
  }
})

function onScroll(): void {
  if (!containerRef.value) return
  const el = containerRef.value
  autoScroll.value = el.scrollHeight - el.scrollTop - el.clientHeight < 50
}

// 日志条目数变化时，若开启自动滚动则滚到底部
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
  return (
    d.toLocaleTimeString('zh-CN', { hour12: false }) +
    '.' +
    String(d.getMilliseconds()).padStart(3, '0')
  )
}

function setLevel(level: LogLevel): void {
  logStore.setMinLevel(level)
}

function setGroup(group: LogGroup | 'all'): void {
  logStore.setFilterGroup(group)
}

function clearFilters(): void {
  // 重置为 Debug（最低级）= 显示全部级别
  logStore.setMinLevel('debug')
  logStore.setFilterGroup('all')
  logStore.setFilterSearch('')
}

async function copyLogs(): Promise<void> {
  try {
    const text = filteredEntries.value
      .map(
        (entry) =>
          `[${formatTime(entry.timestamp)}] [${entry.level.toUpperCase()}] [${categoryLabel(entry)}] [${entry.source}] ${entry.message}${entry.details ? '\n' + entry.details : ''}`,
      )
      .join('\n')
    await navigator.clipboard.writeText(text)
  } catch {
    /* clipboard API may fail in restricted contexts */
  }
}

function categoryLabel(entry: LogEntry): string {
  if (entry.category) return CATEGORY_LABELS[entry.category] ?? entry.category
  return LOG_GROUP_LABELS[mapCategoryToGroup(entry.category)]
}

onMounted(() => {
  logStore.init()
  // 拉取最近 500 条历史日志，并启动 SSE 实时订阅。
  // SSE 订阅生命周期绑定到 LogViewer：
  // - 离开页面时停止订阅，避免后台持续接收日志造成 IPC/资源浪费；
  // - 再次进入页面时重新拉取最近 500 条并重启订阅，覆盖离开期间的日志。
  void fetchRecentLogs(500)
  void fetchCategoryStates()
  startLogSubscription()
})

onBeforeUnmount(() => {
  // 与 onMounted 配对：停止 SSE 订阅，释放 EventSource 资源，避免泄漏。
  stopLogSubscription()
  logStore.destroy()
})
</script>

<template>
  <div class="log-viewer" :class="{ 'embedded-mode': embedded }">
    <!-- 简化标题栏：标题 + 状态 + 计数 + 操作按钮，对齐 daq-t1603 的简洁头部 -->
    <header class="log-header">
      <div class="log-header__left">
        <h1 class="log-header__title">日志</h1>
        <span class="log-header__status" :class="`stream-${logStore.streamStatus}`">
          {{ streamStatusText }}
        </span>
        <span class="log-header__count">{{ filteredEntries.length }} / {{ logStore.entries.length }} 条</span>
      </div>
      <div class="log-header__actions">
        <UiButton
          size="sm"
          variant="secondary"
          :class="{ 'is-paused': logStore.isPaused }"
          :aria-label="logStore.isPaused ? '恢复日志滚动' : '暂停日志滚动'"
          @click="logStore.togglePause()"
        >
          {{ logStore.isPaused ? `恢复 (${logStore.bufferCount})` : '暂停' }}
        </UiButton>
        <UiButton size="sm" variant="secondary" aria-label="复制当前筛选日志" @click="copyLogs">
          复制
        </UiButton>
        <UiButton size="sm" variant="danger" aria-label="清空前端日志缓冲" @click="logStore.clear()">
          清空
        </UiButton>
      </div>
    </header>

    <!-- 工具栏：级别 chip + 分类 chip + 搜索框，参考 daq-t1603 的两行布局 -->
    <section class="log-toolbar" aria-label="日志筛选">
      <div class="log-toolbar__row">
        <div class="log-toolbar__group">
          <span class="log-toolbar__label">级别</span>
          <button
            v-for="level in LEVELS"
            :key="level.label"
            type="button"
            class="log-chip"
            :class="[
              { active: logStore.minLevel === level.value },
              `log-chip--${level.value}`,
            ]"
            @click="setLevel(level.value)"
          >
            {{ level.label }}
          </button>
        </div>

        <div class="log-toolbar__group">
          <span class="log-toolbar__label">分类</span>
          <button
            v-for="group in GROUPS"
            :key="group.value"
            type="button"
            class="log-chip"
            :class="{ active: logStore.filterGroup === group.value }"
            @click="setGroup(group.value)"
          >
            {{ group.label }}
          </button>
        </div>
      </div>

      <div class="log-toolbar__row log-toolbar__row--search">
        <UiInput
          v-model="logStore.filterSearch"
          class="log-toolbar__search"
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
        <UiButton v-if="hasActiveFilter" size="sm" variant="ghost" @click="clearFilters">
          清除筛选
        </UiButton>
        <UiButton
          size="sm"
          variant="ghost"
          :class="{ 'is-active': showCategoryPanel }"
          :aria-expanded="showCategoryPanel"
          aria-label="展开日志类型开关"
          @click="showCategoryPanel = !showCategoryPanel"
        >
          类型开关
        </UiButton>
      </div>

      <!-- 日志类型开关面板：默认收起，点击「类型开关」展开 -->
      <div v-if="showCategoryPanel" class="category-panel">
        <div class="category-panel__header">
          <span>后端日志分类开关</span>
          <small>关闭后该类日志不写入前端缓冲，文件和 stderr 仍完整输出</small>
        </div>
        <div class="category-panel__list">
          <label
            v-for="cat in ALL_CATEGORIES"
            :key="cat.value"
            class="category-toggle"
            :class="{ 'category-toggle--disabled': !isCategoryEnabled(cat.value) }"
          >
            <span class="category-toggle__label">{{ cat.label }}</span>
            <button
              type="button"
              role="switch"
              :aria-checked="isCategoryEnabled(cat.value)"
              :aria-label="`${isCategoryEnabled(cat.value) ? '关闭' : '开启'} ${cat.label} 日志`"
              class="category-toggle__switch"
              :class="{ 'is-on': isCategoryEnabled(cat.value) }"
              :disabled="togglingCategories.has(cat.value)"
              @click="handleCategoryToggle(cat.value, !isCategoryEnabled(cat.value))"
            >
              <span class="switch-thumb"></span>
            </button>
          </label>
        </div>
      </div>
    </section>

    <!-- 日志列表：5 列网格，时间/级别/分类/来源/消息 -->
    <section class="log-table" aria-label="日志列表">
      <div class="log-table__head" aria-hidden="true">
        <span>时间</span>
        <span>级别</span>
        <span>分类</span>
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
          <span class="log-category" :class="`log-category--${mapCategoryToGroup(entry.category)}`">{{ categoryLabel(entry) }}</span>
          <span class="log-source" :title="entry.source">{{ entry.source }}</span>
          <span class="log-message">
            <span>{{ entry.message }}</span>
            <small v-if="entry.deviceId" class="log-device">device={{ entry.deviceId }}</small>
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
  </div>
</template>

<style scoped>
/* ============================================
   日志视图：标题栏 + 工具栏 + 日志列表
   布局对齐 daq-t1603 的简洁风格，去除诊断摘要卡片和侧边面板
   ============================================ */
.log-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: var(--space-2);
  gap: var(--space-2);
  background: var(--bg-app);
  color: var(--text-primary);
  overflow: hidden;
}

.log-viewer.embedded-mode {
  padding: var(--space-2);
}

/* ---- 标题栏 ---- */
.log-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-1-5) var(--space-2);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-panel);
  box-shadow: var(--shadow-panel);
  flex-shrink: 0;
}

.log-header__left {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  min-width: 0;
}

.log-header__title {
  margin: 0;
  font-size: var(--font-size-base);
  font-weight: 700;
  color: var(--text-primary);
  flex-shrink: 0;
}

.log-header__status {
  padding: 0.1rem 0.45rem;
  border-radius: var(--radius-pill);
  font-size: 0.65rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  color: var(--text-secondary);
  flex-shrink: 0;
}

.log-header__status.stream-connected {
  color: var(--accent-success);
  border-color: color-mix(in srgb, var(--accent-success) 40%, transparent);
  background: color-mix(in srgb, var(--accent-success) 10%, transparent);
}

.log-header__status.stream-connecting,
.log-header__status.stream-reconnecting {
  color: var(--accent-warning);
  border-color: color-mix(in srgb, var(--accent-warning) 40%, transparent);
  background: color-mix(in srgb, var(--accent-warning) 10%, transparent);
}

.log-header__status.stream-error {
  color: var(--accent-danger);
  border-color: color-mix(in srgb, var(--accent-danger) 40%, transparent);
  background: color-mix(in srgb, var(--accent-danger) 10%, transparent);
}

.log-header__count {
  font-family: 'JetBrains Mono', 'Cascadia Code', 'Consolas', monospace;
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
  flex-shrink: 0;
}

.log-header__actions {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  flex-shrink: 0;
}

:deep(.is-paused) {
  color: var(--accent-warning);
  border-color: var(--accent-warning);
}

/* ---- 工具栏 ---- */
.log-toolbar {
  display: flex;
  flex-direction: column;
  gap: var(--space-1-5);
  padding: var(--space-1-5) var(--space-2);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-panel);
  box-shadow: var(--shadow-panel);
  flex-shrink: 0;
}

.log-toolbar__row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  min-width: 0;
}

.log-toolbar__row--search {
  justify-content: flex-end;
}

.log-toolbar__group {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-wrap: wrap;
  min-width: 0;
}

.log-toolbar__label {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.04em;
  text-transform: uppercase;
  flex-shrink: 0;
  margin-right: var(--space-0-5);
}

.log-toolbar__search {
  flex: 1;
  min-width: 12rem;
  max-width: 24rem;
}

/* ---- 筛选 chip ---- */
.log-chip {
  min-height: 1.65rem;
  padding: var(--space-0-5) var(--space-1-5);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-pill);
  background: var(--bg-panel);
  color: var(--text-secondary);
  font: inherit;
  font-size: var(--font-size-xs);
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.log-chip:hover {
  color: var(--text-primary);
  border-color: var(--border-hover, var(--accent-primary));
  background: color-mix(in srgb, var(--accent-primary) 6%, var(--bg-panel));
}

.log-chip.active {
  color: var(--text-on-accent, #fff);
  background: var(--accent-primary);
  border-color: var(--accent-primary);
}

.log-chip--error.active {
  background: var(--accent-danger);
  border-color: var(--accent-danger);
}

.log-chip--warn.active {
  background: var(--accent-warning);
  border-color: var(--accent-warning);
}

:deep(.is-active) {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
}

/* ---- 日志类型开关面板（默认收起） ---- */
.category-panel {
  padding: var(--space-1-5) var(--space-2);
  border-top: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
  border-radius: var(--radius-md);
}

.category-panel__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  margin-bottom: var(--space-1-5);
}

.category-panel__header span {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.category-panel__header small {
  font-size: 0.65rem;
  color: var(--text-muted);
  line-height: 1.4;
}

.category-panel__list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(10rem, 1fr));
  gap: var(--space-1-5);
}

.category-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-1);
  padding: var(--space-0-5) var(--space-1);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel);
  cursor: pointer;
  user-select: none;
}

.category-toggle--disabled {
  opacity: 0.7;
}

.category-toggle--disabled .category-toggle__label {
  color: var(--text-muted);
}

.category-toggle__label {
  font-size: var(--font-size-xs);
  color: var(--text-primary);
}

.category-toggle__switch {
  position: relative;
  width: 2rem;
  height: 1.1rem;
  border: none;
  border-radius: var(--radius-pill);
  background: var(--border-default);
  cursor: pointer;
  transition: background 0.15s ease;
  flex-shrink: 0;
}

.category-toggle__switch.is-on {
  background: var(--accent-primary);
}

.category-toggle__switch:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.category-toggle__switch .switch-thumb {
  position: absolute;
  top: 0.1rem;
  left: 0.1rem;
  width: 0.9rem;
  height: 0.9rem;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 3px rgb(0 0 0 / 20%);
  transition: transform 0.15s ease;
}

.category-toggle__switch.is-on .switch-thumb {
  transform: translateX(0.9rem);
}

/* ---- 日志表格 ---- */
.log-table {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-panel);
  box-shadow: var(--shadow-panel);
  overflow: hidden;
}

.log-table__head,
.log-entry {
  display: grid;
  grid-template-columns: 6.75rem 3.8rem 4rem 7rem minmax(0, 1fr);
  gap: var(--space-2);
  align-items: start;
}

.log-table__head {
  padding: var(--space-1) var(--space-2);
  border-bottom: 1px solid var(--border-default);
  color: var(--text-muted);
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  flex-shrink: 0;
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
}

.log-entries {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  font-family: 'JetBrains Mono', 'Cascadia Code', 'Consolas', monospace;
  font-size: var(--font-size-xs);
  line-height: 1.35;
}

.log-entry {
  padding: var(--space-1) var(--space-2);
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
.log-source,
.log-category {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.log-category {
  font-weight: 700;
}

.log-category--communication {
  color: var(--accent-info);
}

.log-category--acquisition {
  color: var(--accent-success);
}

.log-category--business {
  color: var(--accent-warning);
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
  margin-top: var(--space-0-5);
  padding: var(--space-1);
  border: 1px solid color-mix(in srgb, var(--border-default) 65%, transparent);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--bg-app) 62%, transparent);
  color: var(--text-secondary);
  white-space: pre-wrap;
}

.log-device {
  display: block;
  margin-top: var(--space-0-5);
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.log-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  min-height: 12rem;
  padding: var(--space-4);
  color: var(--text-secondary);
  text-align: center;
}

.log-empty strong {
  color: var(--text-primary);
  font-size: var(--font-size-base);
}

/* ---- 小屏适配：工具栏换行 ---- */
@media (max-width: 1024px) {
  .log-toolbar__row {
    flex-wrap: wrap;
  }

  .log-toolbar__row--search {
    justify-content: flex-start;
  }
}
</style>
