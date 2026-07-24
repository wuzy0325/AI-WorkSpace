<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { CATEGORY_LABELS, LOG_GROUP_LABELS, useLogStore } from '@stores/logStore'
import type { LogGroup } from '@stores/logStore'
import type { LogCategory, LogEntry } from '@api/types'
import { useI18nStore } from '@stores/i18nStore'
import UiButton from '@components/ui/UiButton.vue'
import UiInput from '@components/ui/UiInput.vue'
import LogEntryRow from '@components/log/LogEntryRow.vue'
import { formatTime, categoryLabel } from '@utils/logEntryFormat'
import { fetchRecentLogs, startLogSubscription, stopLogSubscription, fetchCategoryStates, setCategoryEnabled } from '@api/logSseClient'

defineProps<{
  embedded?: boolean
}>()

const logStore = useLogStore()
const i18n = useI18nStore()
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

// 三个独立可见性开关：硬件通信 / 信息 / 调试。
// 替代原 minLevel 单选 chip，原因：
//   - 用户需要"显示硬件收发但隐藏普通 info"的组合，minLevel 无法表达
//   - 开关语义更直观，符合"加开关，默认关闭"的产品描述
const VISIBILITY_TOGGLES = computed<Array<{ key: 'hardware' | 'info' | 'debug'; label: string; hint: string; active: boolean; onToggle: () => void }>>(() => [
  {
    key: 'hardware',
    label: i18n.t.log_showHardware,
    hint: i18n.t.log_showHardwareHint,
    active: logStore.showHardware,
    onToggle: () => logStore.toggleHardware(),
  },
  {
    key: 'info',
    label: i18n.t.log_showInfo,
    hint: i18n.t.log_showInfoHint,
    active: logStore.showInfo,
    onToggle: () => logStore.toggleInfo(),
  },
  {
    key: 'debug',
    label: i18n.t.log_showDebug,
    hint: i18n.t.log_showDebugHint,
    active: logStore.showDebug,
    onToggle: () => logStore.toggleDebug(),
  },
])

// 分组过滤选项：'all' 表示全部。用 computed 依赖 i18n，跟随语言切换刷新。
const GROUPS = computed<Array<{ value: LogGroup | 'all'; label: string }>>(() => [
  { value: 'all', label: i18n.t.log_all },
  { value: 'system', label: LOG_GROUP_LABELS.system },
  { value: 'communication', label: LOG_GROUP_LABELS.communication },
  { value: 'acquisition', label: LOG_GROUP_LABELS.acquisition },
  { value: 'business', label: LOG_GROUP_LABELS.business },
])

// hasActiveFilter：搜索关键字或分组非 all 时认为有激活的筛选。
// 三个可见性开关不计入"激活筛选"，因为它们是常态偏好而非临时筛选。
const hasActiveFilter = computed(
  () =>
    logStore.filterGroup !== 'all' ||
    logStore.filterSearch.trim().length > 0,
)

// 实时通道状态简短文案（标题栏显示）
const streamStatusText = computed(() => {
  switch (logStore.streamStatus) {
    case 'connected':
      return i18n.t.log_streamConnected
    case 'connecting':
      return i18n.t.log_streamConnecting
    case 'reconnecting':
      return i18n.t.log_streamReconnecting
    case 'error':
      return i18n.t.log_streamError
    default:
      return i18n.t.log_streamIdle
  }
})

// 类型开关 aria-label：根据当前状态选择「开启/关闭」+ 分类名
function categoryToggleAria(cat: { value: LogCategory; label: string }): string {
  const enabled = isCategoryEnabled(cat.value)
  return (enabled ? i18n.t.log_turnOffCategoryAria : i18n.t.log_turnOnCategoryAria).replace('{category}', cat.label)
}

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

function setGroup(group: LogGroup | 'all'): void {
  logStore.setFilterGroup(group)
}

function clearFilters(): void {
  // 仅清除临时筛选（分组 + 搜索），可见性开关是用户偏好不重置
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
    <!-- 紧凑标题栏：标题 + 状态 + 计数 + 操作按钮 -->
    <header class="log-header">
      <div class="log-header__left">
        <h1 class="log-header__title">{{ i18n.t.log_title }}</h1>
        <span class="log-header__status" :class="`stream-${logStore.streamStatus}`">
          {{ streamStatusText }}
        </span>
        <span class="log-header__count">{{ filteredEntries.length }} / {{ logStore.entries.length }} {{ i18n.t.entries }}</span>
      </div>
      <div class="log-header__actions">
        <!-- 中英文切换：复用全局 .locale-btn 样式（settings-form.css），与设置面板保持视觉一致 -->
        <div class="locale-switch" role="group" :aria-label="i18n.t.set_interfaceLanguage">
          <button
            class="locale-btn"
            :class="{ 'locale-btn--active': i18n.locale === 'zh' }"
            :aria-label="i18n.t.set_switchToChinese"
            :aria-pressed="i18n.locale === 'zh'"
            @click="i18n.setLocale('zh')"
          >中</button>
          <button
            class="locale-btn"
            :class="{ 'locale-btn--active': i18n.locale === 'en' }"
            :aria-label="i18n.t.set_switchToEnglish"
            :aria-pressed="i18n.locale === 'en'"
            @click="i18n.setLocale('en')"
          >EN</button>
        </div>
        <UiButton
          size="sm"
          variant="secondary"
          :class="{ 'is-paused': logStore.isPaused }"
          :aria-label="logStore.isPaused ? i18n.t.log_resumeScrollAria : i18n.t.log_pauseScrollAria"
          @click="logStore.togglePause()"
        >
          {{ logStore.isPaused ? `${i18n.t.log_resume} (${logStore.bufferCount})` : i18n.t.log_pause }}
        </UiButton>
        <UiButton size="sm" variant="secondary" :aria-label="i18n.t.log_copyAria" @click="copyLogs">
          {{ i18n.t.log_copy }}
        </UiButton>
        <UiButton size="sm" variant="danger" :aria-label="i18n.t.log_clearAria" @click="logStore.clear()">
          {{ i18n.t.log_clear }}
        </UiButton>
      </div>
    </header>

    <!-- 工具栏：可见性开关 + 分组 chip + 搜索框，单行紧凑布局 -->
    <section class="log-toolbar" :aria-label="i18n.t.log_filterAria">
      <div class="log-toolbar__row">
        <!-- 可见性开关 chip：点击切换开关状态，激活时高亮 -->
        <div class="log-toolbar__group">
          <button
            v-for="toggle in VISIBILITY_TOGGLES"
            :key="toggle.key"
            type="button"
            class="log-toggle"
            :class="{ 'log-toggle--active': toggle.active, [`log-toggle--${toggle.key}`]: true }"
            :title="toggle.hint"
            :aria-pressed="toggle.active"
            :aria-label="`${toggle.label}: ${toggle.active ? i18n.t.log_toggleOnAria : i18n.t.log_toggleOffAria}`"
            @click="toggle.onToggle()"
          >
            <span class="log-toggle__dot" :class="{ 'is-on': toggle.active }"></span>
            <span>{{ toggle.label }}</span>
          </button>
        </div>

        <span class="log-toolbar__divider" aria-hidden="true"></span>

        <!-- 分组过滤 chip -->
        <div class="log-toolbar__group">
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

        <UiInput
          v-model="logStore.filterSearch"
          class="log-toolbar__search"
          :placeholder="i18n.t.log_searchPlaceholder"
          :aria-label="i18n.t.log_searchAria"
        >
          <template #prefix>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8" />
              <line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
          </template>
        </UiInput>
        <UiButton v-if="hasActiveFilter" size="sm" variant="ghost" @click="clearFilters">
          {{ i18n.t.log_clearFilter }}
        </UiButton>
        <UiButton
          size="sm"
          variant="ghost"
          :class="{ 'is-active': showCategoryPanel }"
          :aria-expanded="showCategoryPanel"
          :aria-label="i18n.t.log_categoryToggleAria"
          @click="showCategoryPanel = !showCategoryPanel"
        >
          {{ i18n.t.log_categoryToggle }}
        </UiButton>
      </div>

      <!-- 日志类型开关面板：默认收起，点击「类型开关」展开 -->
      <div v-if="showCategoryPanel" class="category-panel">
        <div class="category-panel__header">
          <span>{{ i18n.t.log_categoryPanelTitle }}</span>
          <small>{{ i18n.t.log_categoryPanelHint }}</small>
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
              :aria-label="categoryToggleAria(cat)"
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

    <!-- 日志列表：4 列紧凑网格，时间/级别/分类/消息(含 source 小字) -->
    <section class="log-table" :aria-label="i18n.t.log_listAria">
      <div class="log-table__head" aria-hidden="true">
        <span>{{ i18n.t.log_time }}</span>
        <span>L</span>
        <span>{{ i18n.t.log_category }}</span>
        <span>{{ i18n.t.log_message }}</span>
      </div>
      <div ref="containerRef" class="log-entries" @scroll="onScroll">
        <LogEntryRow
          v-for="entry in filteredEntries"
          :key="entry.id"
          :entry="entry"
        />

        <div v-if="filteredEntries.length === 0" class="log-empty">
          <strong>{{ logStore.entries.length === 0 ? i18n.t.log_noLogsYet : i18n.t.log_noMatchingLogs }}</strong>
          <span>{{ logStore.entries.length === 0 ? i18n.t.log_noLogsHint : i18n.t.log_noMatchingHint }}</span>
          <UiButton v-if="hasActiveFilter" size="sm" variant="secondary" @click="clearFilters">{{ i18n.t.log_showAll }}</UiButton>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* ============================================
   日志视图：标题栏 + 工具栏 + 日志列表
   紧凑布局：4 列网格，单字母级别徽章，消息内联 source 小字
   单屏可见行数比旧版提升约 60%
   ============================================ */
.log-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: var(--space-2);
  gap: var(--space-1-5);
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
  padding: var(--space-1) var(--space-2);
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
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-panel);
  box-shadow: var(--shadow-panel);
  flex-shrink: 0;
}

.log-toolbar__row {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  flex-wrap: wrap;
  min-width: 0;
}

.log-toolbar__group {
  display: flex;
  align-items: center;
  gap: var(--space-0-5);
  flex-wrap: wrap;
  min-width: 0;
}

.log-toolbar__divider {
  width: 1px;
  height: 1rem;
  background: var(--border-default);
  flex-shrink: 0;
}

.log-toolbar__search {
  flex: 1;
  min-width: 10rem;
  max-width: 20rem;
}

/* ---- 可见性开关 chip（带状态圆点） ---- */
.log-toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-0-5);
  min-height: 1.5rem;
  padding: 0 var(--space-1-5);
  border: 1px solid var(--border-default);
  /* 圆角矩形（radius-md），与全局 .locale-btn / UiButton 风格对齐；
   * 原用 radius-pill 胶囊形与其他画面按钮不一致。*/
  border-radius: var(--radius-md);
  background: var(--bg-panel);
  color: var(--text-muted);
  font: inherit;
  font-size: var(--font-size-xs);
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.log-toggle:hover {
  color: var(--text-primary);
  border-color: var(--border-hover, var(--accent-primary));
}

.log-toggle--active {
  color: var(--text-on-accent, #fff);
  background: var(--accent-primary);
  border-color: var(--accent-primary);
}

/* 硬件通信开关激活时使用青色（区别于普通级别筛选） */
.log-toggle--hardware.log-toggle--active {
  background: var(--accent-info);
  border-color: var(--accent-info);
}

/* 调试开关激活时使用 warning 色（橙/琥珀）。
 * 原先用 var(--text-secondary) 作为背景，深色主题下文本色与背景对比度仅 2:1，
 * 不满足 WCAG AA 4.5:1 标准。改用 accent token 保证可读性。*/
.log-toggle--debug.log-toggle--active {
  background: var(--accent-warning);
  border-color: var(--accent-warning);
}

.log-toggle__dot {
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 50%;
  background: var(--text-muted);
  transition: background 0.15s ease;
}

.log-toggle__dot.is-on {
  background: #fff;
}

/* ---- 分组筛选 chip ---- */
.log-chip {
  min-height: 1.5rem;
  padding: 0 var(--space-1);
  border: 1px solid var(--border-default);
  /* 圆角矩形（radius-md），与 .log-toggle / .locale-btn / UiButton 风格对齐 */
  border-radius: var(--radius-md);
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

:deep(.is-active) {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
}

/* ---- 日志类型开关面板（默认收起） ---- */
.category-panel {
  padding: var(--space-1) var(--space-2);
  border-top: 1px solid var(--border-default);
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
  border-radius: var(--radius-md);
}

.category-panel__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  margin-bottom: var(--space-1);
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
  gap: var(--space-1);
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

/* ---- 日志表格（4 列紧凑网格） ---- */
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

/* 列宽说明：
   - 时间 7.5rem：HH:MM:SS.mmm 等宽显示
   - 级别 1.5rem：单字母徽章
   - 分类 3rem：2-3 字中文标签
   - 消息 1fr：消息主体 + source 小字内联
   .log-entry 的 grid 布局在 LogEntryRow.vue 子组件中定义，保持一致。*/
.log-table__head {
  display: grid;
  grid-template-columns: 7.5rem 1.5rem 3rem minmax(0, 1fr);
  gap: var(--space-1-5);
  align-items: baseline;
  padding: var(--space-0-5) var(--space-2);
  border-bottom: 1px solid var(--border-default);
  color: var(--text-muted);
  font-size: 0.625rem;
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
  font-size: 0.6875rem;
  line-height: 1.4;
}

/* 日志条目样式（.log-entry / .log-time / .log-badge / .log-category / .log-message 等）
 * 已迁移到 LogEntryRow.vue 子组件，避免重复定义。*/

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

  .log-toolbar__search {
    max-width: none;
  }
}
</style>
