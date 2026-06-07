<script setup lang="ts">
import { ref, nextTick, watch, computed } from 'vue'
import { X, Trash2, ChevronLeft, ChevronRight, PanelRightOpen, PanelRightClose } from '@lucide/vue'
import { useLogStore } from '@stores/logStore'
import type { LogLevel } from '@stores/logStore'
import type { LogCategory } from '@bridge/deviceBridge'

const logStore = useLogStore()

/** 面板展开状态：默认收起 */
const expanded = ref(false)
const scrollContainer = ref<HTMLElement | null>(null)
const autoScroll = ref(true)

/** 级别过滤选项 */
const levelOptions: { value: LogLevel; label: string }[] = [
  { value: 'debug', label: 'Debug' },
  { value: 'info', label: 'Info' },
  { value: 'warn', label: 'Warn' },
  { value: 'error', label: 'Error' },
]

const categoryOptions: { value: LogCategory | 'all'; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'system', label: '系统' },
  { value: 'hardware-send', label: '下发' },
  { value: 'hardware-recv', label: '返回' },
  { value: 'acquisition', label: '采集' },
]

/** 日志条数统计 */
const errorCount = computed(() => logStore.entries.filter((e) => e.level === 'error').length)
const warnCount = computed(() => logStore.entries.filter((e) => e.level === 'warn').length)

/** 级别对应的 CSS 类名 */
function levelClass(level: LogLevel): string {
  return `log-entry--${level}`
}

/** 格式化时间戳 */
function formatTime(ts: number): string {
  const d = new Date(ts)
  return d.toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}

/** 自动滚动到底部 */
watch(
  () => logStore.filteredEntries.length,
  async () => {
    if (!autoScroll.value || !expanded.value) return
    await nextTick()
    if (scrollContainer.value) {
      scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight
    }
  }
)

/** 切换展开/收起 */
function togglePanel(): void {
  expanded.value = !expanded.value
}

/** 清空日志 */
function clearLogs(): void {
  logStore.clear()
}

function selectLevel(level: LogLevel): void {
  logStore.setMinLevel(level)
}

function selectCategory(category: LogCategory | 'all'): void {
  logStore.setCategory(category)
}
</script>

<template>
  <div class="log-panel" :class="{ 'log-panel--expanded': expanded }">
    <!-- 收起状态：窄边栏 + 展开按钮 -->
    <div v-if="!expanded" class="log-panel__collapsed" @click="togglePanel">
      <div class="log-panel__collapsed-inner">
        <PanelRightOpen class="log-panel__collapsed-icon" />
        <span class="log-panel__collapsed-text">日志</span>
        <span v-if="errorCount > 0" class="log-panel__badge log-panel__badge--error log-panel__badge--collapsed">{{ errorCount }}</span>
        <span v-else-if="warnCount > 0" class="log-panel__badge log-panel__badge--warn log-panel__badge--collapsed">{{ warnCount }}</span>
      </div>
    </div>

    <!-- 展开状态：完整面板 -->
    <template v-else>
      <!-- 面板头部 -->
      <div class="log-panel__header">
        <div class="log-panel__header-left">
          <span class="log-panel__title">日志</span>
          <span v-if="errorCount > 0" class="log-panel__badge log-panel__badge--error">{{ errorCount }}</span>
          <span v-else-if="warnCount > 0" class="log-panel__badge log-panel__badge--warn">{{ warnCount }}</span>
          <span class="log-panel__count">{{ logStore.filteredEntries.length }} 条</span>
        </div>
        <div class="log-panel__header-right">
          <button class="log-panel__tool-btn" title="清空日志" @click.stop="clearLogs">
            <Trash2 class="log-panel__tool-icon" />
          </button>
          <button class="log-panel__tool-btn" title="收起" @click.stop="expanded = false">
            <PanelRightClose class="log-panel__tool-icon" />
          </button>
        </div>
      </div>

      <!-- 工具栏 -->
      <div class="log-panel__toolbar">
        <div class="log-panel__filters">
          <div class="log-panel__filter-group">
            <span class="log-panel__filter-label">级别</span>
            <button
              v-for="option in levelOptions"
              :key="option.value"
              class="log-panel__chip"
              :class="{ 'log-panel__chip--active': logStore.minLevel === option.value }"
              type="button"
              @click.stop="selectLevel(option.value)"
            >
              {{ option.label }}
            </button>
          </div>

          <div class="log-panel__filter-group">
            <span class="log-panel__filter-label">分类</span>
            <button
              v-for="option in categoryOptions"
              :key="option.value"
              class="log-panel__chip"
              :class="{ 'log-panel__chip--active': logStore.category === option.value }"
              type="button"
              @click.stop="selectCategory(option.value)"
            >
              {{ option.label }}
            </button>
          </div>
        </div>
      </div>

      <!-- 日志列表 -->
      <div ref="scrollContainer" class="log-panel__entries">
        <div
          v-for="entry in logStore.filteredEntries"
          :key="entry.id"
          class="log-entry"
          :class="levelClass(entry.level)"
        >
          <span class="log-entry__time mono">{{ formatTime(entry.timestamp) }}</span>
          <span class="log-entry__level">[{{ entry.level.toUpperCase() }}]</span>
          <span class="log-entry__category">[{{ entry.category }}]</span>
          <span class="log-entry__tag">{{ entry.tag }}</span>
          <span class="log-entry__msg">{{ entry.message }}</span>
          <span v-if="entry.deviceId" class="log-entry__device mono">{{ entry.deviceId }}</span>
          <span v-if="entry.detail" class="log-entry__detail mono">{{ entry.detail }}</span>
        </div>
        <div v-if="logStore.filteredEntries.length === 0" class="log-panel__empty">
          暂无日志
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
/* ============================================
   右侧边栏日志面板
   ============================================ */
.log-panel {
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  background: var(--bg-panel);
  border-left: 1px solid var(--border-default);
  overflow: hidden;
  transition: width 0.25s var(--easing-standard);
}

/* 收起状态：窄边栏 */
.log-panel:not(.log-panel--expanded) {
  width: 2.2rem;
  cursor: pointer;
}

.log-panel__collapsed {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 0.6rem 0;
  transition: background 0.15s ease;
}

.log-panel__collapsed:hover {
  background: var(--btn-bg-hover);
}

.log-panel__collapsed-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
}

.log-panel__collapsed-icon {
  width: 16px;
  height: 16px;
  color: var(--text-muted);
}

.log-panel__collapsed-text {
  font-size: 0.65rem;
  font-weight: 700;
  color: var(--text-secondary);
  writing-mode: vertical-rl;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

/* 展开状态 */
.log-panel--expanded {
  width: 28rem;
  max-width: 40vw;
  cursor: default;
}

/* 标题栏 */
.log-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.75rem;
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

.log-panel__header-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.log-panel__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  flex-shrink: 0;
}

.log-panel__count {
  font-size: 0.6rem;
  font-weight: 600;
  color: var(--text-muted);
  font-family: var(--font-family-mono);
  flex-shrink: 0;
}

.log-panel__header-right {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  flex-shrink: 0;
}

/* 徽章 */
.log-panel__badge {
  font-size: 0.6rem;
  font-weight: 700;
  padding: 0.1rem 0.4rem;
  border-radius: var(--radius-pill);
  min-width: 1.2rem;
  text-align: center;
  flex-shrink: 0;
}

.log-panel__badge--error {
  background: var(--danger-muted);
  color: var(--danger);
  border: 1px solid var(--danger-border);
}

.log-panel__badge--warn {
  background: var(--warning-muted);
  color: var(--warning);
  border: 1px solid var(--warning);
}

.log-panel__badge--collapsed {
  min-width: 1rem;
  padding: 0.05rem 0.25rem;
  font-size: 0.55rem;
}

/* 工具按钮 */
.log-panel__tool-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.6rem;
  height: 1.6rem;
  color: var(--text-secondary);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s ease;
}

.log-panel__tool-btn:hover {
  color: var(--text-primary);
  background: var(--btn-bg-hover);
  border-color: var(--border-hover);
}

.log-panel__tool-icon {
  width: 12px;
  height: 12px;
}

/* 工具栏 */
.log-panel__toolbar {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.4rem 0.65rem;
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--divider-color);
  flex-shrink: 0;
}

.log-panel__filters {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-width: 0;
}

.log-panel__filter-group {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.35rem;
}

.log-panel__filter-label {
  font-size: 0.6rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.04em;
  flex-shrink: 0;
  margin-right: 0.2rem;
}

.log-panel__chip {
  padding: 0.18rem 0.45rem;
  font-size: 0.62rem;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-pill);
  cursor: pointer;
  transition: all 0.15s ease;
}

.log-panel__chip:hover {
  color: var(--text-primary);
  border-color: var(--border-hover);
  background: var(--btn-bg-hover);
}

.log-panel__chip--active {
  color: var(--text-on-accent, #fff);
  background: var(--accent);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 36%, transparent);
}

/* 日志列表 */
.log-panel__entries {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0.3rem 0;
  font-family: var(--font-family-mono);
  font-size: 0.68rem;
  line-height: 1.6;
  min-height: 0;
}

.log-entry {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  padding: 0.15rem 0.65rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-entry:hover {
  background: var(--btn-bg);
}

.log-entry__time {
  color: var(--text-muted);
  flex-shrink: 0;
  font-size: 0.62rem;
}

.log-entry__level {
  flex-shrink: 0;
  font-weight: 700;
  font-size: 0.6rem;
  width: 3.2rem;
}

.log-entry__category {
  flex-shrink: 0;
  color: var(--text-muted);
  font-size: 0.6rem;
}

.log-entry__tag {
  flex-shrink: 0;
  color: var(--accent);
  font-weight: 600;
  max-width: 6rem;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-entry__msg {
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

.log-entry__device {
  flex-shrink: 0;
  color: var(--text-muted);
  font-size: 0.6rem;
}

.log-entry__detail {
  flex-shrink: 0;
  color: var(--text-muted);
  font-size: 0.6rem;
  max-width: 10rem;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 级别颜色 */
.log-entry--debug .log-entry__level { color: var(--text-muted); }
.log-entry--info .log-entry__level  { color: var(--accent); }
.log-entry--warn .log-entry__level  { color: var(--warning); }
.log-entry--error .log-entry__level { color: var(--danger); }

.log-entry--warn .log-entry__msg   { color: var(--warning); }
.log-entry--error .log-entry__msg  { color: var(--danger); }

.log-entry--warn  { background: rgba(245, 158, 11, 0.04); }
.log-entry--error { background: rgba(244, 63, 94, 0.06); }

/* 空状态 */
.log-panel__empty {
  padding: 2rem 1rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.7rem;
}

/* 响应式：小屏幕下禁用展开 */
@media (max-width: 1024px) {
  .log-panel--expanded {
    width: 22rem;
  }
}

@media (max-width: 767px) {
  .log-panel {
    display: none;
  }
}
</style>
