<script setup lang="ts">
import { ref, nextTick, watch, computed } from 'vue'
import { X, Trash2, ChevronDown, ChevronUp } from '@lucide/vue'
import { useLogStore } from '@stores/logStore'
import type { LogLevel } from '@stores/logStore'
import type { LogCategory } from '@bridge/deviceBridge'

const logStore = useLogStore()

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
    <!-- 折叠状态下的标题栏 -->
    <div class="log-panel__header" @click="togglePanel">
      <div class="log-panel__header-left">
        <span class="log-panel__title">日志</span>
        <span v-if="errorCount > 0" class="log-panel__badge log-panel__badge--error">{{ errorCount }}</span>
        <span v-else-if="warnCount > 0" class="log-panel__badge log-panel__badge--warn">{{ warnCount }}</span>
      </div>
      <div class="log-panel__header-right">
        <span class="log-panel__count">{{ logStore.filteredEntries.length }}</span>
        <component :is="expanded ? ChevronDown : ChevronUp" class="log-panel__toggle-icon" />
      </div>
    </div>

    <!-- 展开后的日志内容 -->
    <Transition name="log-slide">
      <div v-if="expanded" class="log-panel__body">
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
          <button class="log-panel__tool-btn" title="清空日志" @click.stop="clearLogs">
            <Trash2 class="log-panel__tool-icon" />
            <span>清空</span>
          </button>
          <button class="log-panel__tool-btn" title="关闭" @click.stop="expanded = false">
            <X class="log-panel__tool-icon" />
          </button>
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
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.log-panel {
  position: relative;
  border-top: 1px solid var(--border-default);
  background: var(--bg-panel);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.log-panel--expanded {
  height: 220px;
}

/* 标题栏 */
.log-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.3rem 0.85rem;
  cursor: pointer;
  user-select: none;
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
  transition: background 0.15s ease;
}

.log-panel__header:hover {
  background: var(--btn-bg-hover);
}

.log-panel__header-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.log-panel__title {
  font-size: 0.7rem;
  font-weight: 700;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.log-panel__badge {
  font-size: 0.6rem;
  font-weight: 700;
  padding: 0.1rem 0.4rem;
  border-radius: var(--radius-pill);
  min-width: 1.2rem;
  text-align: center;
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

.log-panel__header-right {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.log-panel__count {
  font-size: 0.6rem;
  font-weight: 600;
  color: var(--text-muted);
  font-family: var(--font-family-mono);
}

.log-panel__toggle-icon {
  width: 14px;
  height: 14px;
  color: var(--text-muted);
}

/* 展开内容 */
.log-panel__body {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* 工具栏 */
.log-panel__toolbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.25rem 0.65rem;
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--divider-color);
}

.log-panel__filters {
  display: flex;
  flex-wrap: wrap;
  gap: 0.65rem;
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

.log-panel__tool-btn {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.2rem 0.5rem;
  font-size: 0.6rem;
  font-weight: 600;
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
  width: 11px;
  height: 11px;
}

/* 日志列表 */
.log-panel__entries {
  flex: 1;
  overflow-y: auto;
  padding: 0.3rem 0;
  font-family: var(--font-family-mono);
  font-size: 0.68rem;
  line-height: 1.6;
}

.log-entry {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  padding: 0.1rem 0.75rem;
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

.log-entry__tag {
  flex-shrink: 0;
  color: var(--accent);
  font-weight: 600;
  max-width: 8rem;
  overflow: hidden;
  text-overflow: ellipsis;
}

.log-entry__msg {
  color: var(--text-primary);
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
  padding: 1.5rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.7rem;
}

/* 展开/收起动画 */
.log-slide-enter-active,
.log-slide-leave-active {
  transition: all 0.2s ease;
}

.log-slide-enter-from,
.log-slide-leave-to {
  opacity: 0;
  max-height: 0;
}
</style>
