<script setup lang="ts">
import { ref, nextTick, watch, computed, onUnmounted } from 'vue'
import { Trash2, PanelRightOpen, PanelRightClose, Copy, Check, FileText, FileX, FolderOpen } from '@lucide/vue'
import { useLogStore } from '@stores/logStore'
import type { LogLevel, LogGroup, LogEntry } from '@stores/logStore'
import { useI18nStore } from '@stores/i18nStore'
import type { LogCategory } from '@bridge/deviceBridge'

const logStore = useLogStore()
const i18n = useI18nStore()

/** 面板展开状态：默认收起 */
const expanded = ref(false)
const scrollContainer = ref<HTMLElement | null>(null)
const autoScroll = ref(true)

/** 拷贝成功反馈：记录 id -> 是否显示勾 */
const copiedId = ref<number | null>(null)
let copiedTimer: ReturnType<typeof setTimeout> | null = null

/** 级别过滤选项 */
const levelOptions = computed<{ value: LogLevel; label: string }[]>(() => [
  { value: 'debug', label: i18n.t('log.levelDebug') },
  { value: 'info', label: i18n.t('log.levelInfo') },
  { value: 'warn', label: i18n.t('log.levelWarn') },
  { value: 'error', label: i18n.t('log.levelError') },
])

/** 分组过滤选项（标签随语言切换，用 computed 让 locale 变化时自动刷新） */
const groupOptions = computed<{ value: LogGroup | 'all'; label: string }[]>(() => [
  { value: 'all', label: i18n.t('log.allGroups') },
  { value: 'system', label: i18n.t('logGroup.system') },
  { value: 'communication', label: i18n.t('logGroup.communication') },
  { value: 'acquisition', label: i18n.t('logGroup.acquisition') },
])

/** 日志条数统计 */
const errorCount = computed(() => logStore.entries.filter((e) => e.level === 'error').length)
const warnCount = computed(() => logStore.entries.filter((e) => e.level === 'warn').length)

/** 级别对应的 CSS 类名 */
function levelClass(level: LogLevel): string {
  return `log-entry--${level}`
}

/** 格式化时间戳：随语言切换显示习惯（zh→zh-CN，en→en-US） */
function formatTime(ts: number): string {
  const d = new Date(ts)
  return d.toLocaleTimeString(i18n.timeLocale, {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}

/** 获取分类的本地化标签 */
function categoryLabel(category: LogCategory): string {
  switch (category) {
    case 'system': return i18n.t('logCategory.system')
    case 'hardware-send': return i18n.t('logCategory.hardwareSend')
    case 'hardware-recv': return i18n.t('logCategory.hardwareRecv')
    case 'acquisition': return i18n.t('logCategory.acquisition')
    default: return category
  }
}

/** 格式化单条日志为纯文本 */
function formatEntry(entry: LogEntry): string {
  const parts = [
    formatTime(entry.timestamp),
    `[${entry.level.toUpperCase()}]`,
    `[${categoryLabel(entry.category)}]`,
  ]
  if (entry.tag) parts.push(entry.tag)
  parts.push(entry.message)
  if (entry.deviceId) parts.push(`device=${entry.deviceId}`)
  if (entry.detail) parts.push(entry.detail)
  return parts.join(' ')
}

/** 拷贝单条日志到剪贴板 */
async function copyEntry(entry: LogEntry): Promise<void> {
  try {
    await navigator.clipboard.writeText(formatEntry(entry))
    copiedId.value = entry.id
    if (copiedTimer !== null) clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => { copiedId.value = null }, 1500)
  } catch {
    // 剪贴板不可用时静默失败
  }
}

/** 拷贝全部已过滤日志到剪贴板 */
async function copyAll(): Promise<void> {
  if (logStore.filteredEntries.length === 0) return
  try {
    const text = logStore.filteredEntries.map(formatEntry).join('\n')
    await navigator.clipboard.writeText(text)
    copiedId.value = -1 // 用 -1 表示"全部拷贝"的反馈
    if (copiedTimer !== null) clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => { copiedId.value = null }, 1500)
  } catch {
    // 剪贴板不可用时静默失败
  }
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

function selectGroup(g: LogGroup | 'all'): void {
  logStore.setGroup(g)
}

/** 切换日志文件保存 */
async function toggleFileSaving(): Promise<void> {
  try {
    if (logStore.fileSaving) {
      await logStore.stopFileSaving()
    } else {
      const dir = await logStore.pickLogDir()
      if (dir) {
        await logStore.startFileSaving(dir)
      }
    }
  } catch {
    // 操作失败时静默忽略，状态由后端决定
    await logStore.refreshFileState()
  }
}

/** 组件卸载时清理定时器 */
onUnmounted(() => {
  if (copiedTimer !== null) {
    clearTimeout(copiedTimer)
    copiedTimer = null
  }
})
</script>

<template>
  <div class="log-panel" :class="{ 'log-panel--expanded': expanded }">
    <!-- 收起状态：窄边栏 + 展开按钮 -->
    <div v-if="!expanded" class="log-panel__collapsed" @click="togglePanel">
      <div class="log-panel__collapsed-inner">
        <PanelRightOpen class="log-panel__collapsed-icon" />
        <span class="log-panel__collapsed-text">{{ i18n.t('log.title') }}</span>
        <span v-if="errorCount > 0" class="log-panel__badge log-panel__badge--error log-panel__badge--collapsed">{{ errorCount }}</span>
        <span v-else-if="warnCount > 0" class="log-panel__badge log-panel__badge--warn log-panel__badge--collapsed">{{ warnCount }}</span>
      </div>
    </div>

    <!-- 展开状态：完整面板 -->
    <template v-else>
      <!-- 面板头部 -->
      <div class="log-panel__header">
        <div class="log-panel__header-left">
          <span class="log-panel__title">{{ i18n.t('log.title') }}</span>
          <span v-if="errorCount > 0" class="log-panel__badge log-panel__badge--error">{{ errorCount }}</span>
          <span v-else-if="warnCount > 0" class="log-panel__badge log-panel__badge--warn">{{ warnCount }}</span>
          <span class="log-panel__count">{{ i18n.t('log.entries', { n: logStore.filteredEntries.length }) }}</span>
        </div>
        <div class="log-panel__header-right">
          <button
            class="log-panel__tool-btn"
            :class="{ 'log-panel__tool-btn--active': logStore.fileSaving }"
            :title="logStore.fileSaving ? i18n.t('log.savingTo', { dir: logStore.fileOutputDir }) : i18n.t('log.saveToFile')"
            @click.stop="toggleFileSaving"
          >
            <FileText v-if="logStore.fileSaving" class="log-panel__tool-icon" />
            <FileX v-else class="log-panel__tool-icon" />
          </button>
          <button
            class="log-panel__tool-btn"
            :class="{ 'log-panel__tool-btn--success': copiedId === -1 }"
            :title="i18n.t('log.copyAll')"
            @click.stop="copyAll"
          >
            <Check v-if="copiedId === -1" class="log-panel__tool-icon" />
            <Copy v-else class="log-panel__tool-icon" />
          </button>
          <button class="log-panel__tool-btn" :title="i18n.t('log.clear')" @click.stop="clearLogs">
            <Trash2 class="log-panel__tool-icon" />
          </button>
          <button class="log-panel__tool-btn" :title="i18n.t('log.collapse')" @click.stop="expanded = false">
            <PanelRightClose class="log-panel__tool-icon" />
          </button>
        </div>
      </div>

      <!-- 工具栏 -->
      <div class="log-panel__toolbar">
        <div class="log-panel__filters">
          <div class="log-panel__filter-group">
            <span class="log-panel__filter-label">{{ i18n.t('log.level') }}</span>
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
            <span class="log-panel__filter-label">{{ i18n.t('log.category') }}</span>
            <button
              v-for="option in groupOptions"
              :key="option.value"
              class="log-panel__chip"
              :class="{ 'log-panel__chip--active': logStore.group === option.value }"
              type="button"
              @click.stop="selectGroup(option.value)"
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
          <span class="log-entry__category" :class="`log-entry__category--${entry.group}`">{{ categoryLabel(entry.category) }}</span>
          <span class="log-entry__tag">{{ entry.tag }}</span>
          <span class="log-entry__msg">{{ entry.message }}</span>
          <span v-if="entry.detail" class="log-entry__detail mono" :title="entry.detail">{{ entry.detail }}</span>
          <button
            class="log-entry__copy"
            :class="{ 'log-entry__copy--done': copiedId === entry.id }"
            :title="i18n.t('log.copyEntry')"
            @click.stop="copyEntry(entry)"
          >
            <Check v-if="copiedId === entry.id" class="log-entry__copy-icon" />
            <Copy v-else class="log-entry__copy-icon" />
          </button>
        </div>
        <div v-if="logStore.filteredEntries.length === 0" class="log-panel__empty">
          {{ i18n.t('log.empty') }}
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

.log-panel__tool-btn--success {
  color: var(--accent);
  border-color: var(--accent-border);
  background: var(--accent-muted);
}

.log-panel__tool-btn--active {
  color: var(--accent);
  border-color: var(--accent-border);
  background: var(--accent-muted);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 20%, transparent);
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
  position: relative;
}

.log-entry:hover {
  background: var(--btn-bg);
}

/* 悬停时显示拷贝按钮 */
.log-entry__copy {
  display: none;
  position: absolute;
  right: 0.35rem;
  top: 50%;
  transform: translateY(-50%);
  align-items: center;
  justify-content: center;
  width: 1.3rem;
  height: 1.3rem;
  padding: 0;
  color: var(--text-muted);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s ease;
  flex-shrink: 0;
}

.log-entry:hover .log-entry__copy {
  display: flex;
}

.log-entry__copy:hover {
  color: var(--accent);
  border-color: var(--accent-border);
  background: var(--accent-muted);
}

.log-entry__copy--done {
  color: var(--accent);
  border-color: var(--accent-border);
  background: var(--accent-muted);
}

.log-entry__copy-icon {
  width: 10px;
  height: 10px;
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

/* 分类标签：按分组着色 */
.log-entry__category {
  flex-shrink: 0;
  font-size: 0.6rem;
  font-weight: 600;
  padding: 0.05rem 0.3rem;
  border-radius: var(--radius-pill);
  border: 1px solid transparent;
}

.log-entry__category--system {
  color: var(--text-muted);
  background: var(--btn-bg);
  border-color: var(--border-default);
}

.log-entry__category--communication {
  color: var(--accent);
  background: var(--accent-muted);
  border-color: var(--accent-border);
}

.log-entry__category--acquisition {
  color: var(--success);
  background: var(--success-muted);
  border-color: var(--success-border, var(--success));
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
  flex: 1;
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
