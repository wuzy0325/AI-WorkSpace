<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useLogStore } from '@stores/logStore'
import { useThemeStore } from '@stores/themeStore'
import type { LogLevel } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import UiInput from '@components/ui/UiInput.vue'

defineProps<{
  embedded?: boolean
}>()

const logStore = useLogStore()
const themeStore = useThemeStore()
const { filteredEntries } = storeToRefs(logStore)
const containerRef = ref<HTMLElement | null>(null)
const autoScroll = ref(true)
const isLight = computed(() => themeStore.theme === 'light')

const LEVELS: Array<{ value: LogLevel | null; label: string; icon: string }> = [
  { value: null, label: 'All', icon: '◇' },
  { value: 'debug', label: 'DEBUG', icon: '·' },
  { value: 'info', label: 'INFO', icon: '●' },
  { value: 'warn', label: 'WARN', icon: '▲' },
  { value: 'error', label: 'ERROR', icon: '✕' },
]

const levelColors = computed<Record<string, string>>(() => {
  if (isLight.value) {
    return {
      debug: 'text-slate-400',
      info: 'text-cyan-600',
      warn: 'text-amber-600',
      error: 'text-rose-600',
    }
  }
  return {
    debug: 'text-slate-500',
    info: 'text-cyan-400',
    warn: 'text-amber-400',
    error: 'text-rose-400',
  }
})

const levelBadgeColors = computed<Record<string, string>>(() => {
  if (isLight.value) {
    return {
      debug: 'bg-slate-500/10 text-slate-500 ring-1 ring-slate-500/15',
      info: 'bg-cyan-500/10 text-cyan-600 ring-1 ring-cyan-500/15',
      warn: 'bg-amber-500/10 text-amber-600 ring-1 ring-amber-500/15',
      error: 'bg-rose-500/10 text-rose-600 ring-1 ring-rose-500/15',
    }
  }
  return {
    debug: 'bg-slate-500/15 text-slate-400 ring-1 ring-slate-500/20',
    info: 'bg-cyan-500/15 text-cyan-400 ring-1 ring-cyan-500/20',
    warn: 'bg-amber-500/15 text-amber-400 ring-1 ring-amber-500/20',
    error: 'bg-rose-500/15 text-rose-400 ring-1 ring-rose-500/20',
  }
})

const levelDotColors: Record<string, string> = {
  debug: 'bg-slate-500',
  info: 'bg-cyan-500',
  warn: 'bg-amber-500',
  error: 'bg-rose-500',
}

function onScroll(): void {
  if (!containerRef.value) return
  const el = containerRef.value
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50
  autoScroll.value = atBottom
}

watch(
  () => filteredEntries.value.length,
  () => {
    if (autoScroll.value) {
      nextTick(() => {
        if (containerRef.value) {
          containerRef.value.scrollTop = containerRef.value.scrollHeight
        }
      })
    }
  }
)

function formatTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
}

async function copyLogs(): Promise<void> {
  try {
    const text = filteredEntries.value
      .map((e) => `[${formatTime(e.timestamp)}] [${e.level.toUpperCase()}] [${e.source}] ${e.message}${e.details ? '\n' + e.details : ''}`)
      .join('\n')
    await navigator.clipboard.writeText(text)
  } catch {
    /* clipboard API may fail in restricted contexts */
  }
}

onMounted(() => {
  logStore.init()
})

onBeforeUnmount(() => {
  logStore.destroy()
})
</script>

<template>
  <div class="log-viewer" :class="{ 'embedded-mode': embedded }">
    <!-- Header -->
    <div class="log-header">
      <div class="log-header-left">
        <div class="log-header-icon">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            <polyline points="14 2 14 8 20 8"/>
            <line x1="16" y1="13" x2="8" y2="13"/>
            <line x1="16" y1="17" x2="8" y2="17"/>
            <polyline points="10 9 9 9 8 9"/>
          </svg>
        </div>
        <div class="log-header-text">
          <span class="log-title">System Log</span>
          <span class="log-count">{{ filteredEntries.length }}</span>
        </div>
      </div>
      <div class="log-header-actions">
        <UiButton
          size="sm"
          class="log-action-btn"
          :class="{ 'pause-active': logStore.isPaused }"
          @click="logStore.togglePause()"
          :title="logStore.isPaused ? 'Resume' : 'Pause'"
        >
          <template #icon>
            <svg v-if="logStore.isPaused" width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>
            <svg v-else width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
          </template>
          {{ logStore.isPaused ? 'Resume' : 'Pause' }}
        </UiButton>
        <UiButton size="sm" class="log-action-btn" @click="logStore.clear()" title="Clear">
          <template #icon>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="3 6 5 6 21 6"/>
              <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
          </template>
          Clear
        </UiButton>
        <UiButton size="sm" class="log-action-btn" @click="copyLogs" title="Copy all">
          <template #icon>
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
            </svg>
          </template>
          Copy
        </UiButton>
      </div>
    </div>

    <!-- Filters -->
    <div class="log-filters">
      <div class="log-level-filters">
        <UiButton
          v-for="lvl in LEVELS"
          :key="lvl.label"
          size="sm"
          class="log-level-chip"
          :class="[`chip-${lvl.label.toLowerCase()}`, { active: logStore.filterLevel === lvl.value }]"
          @click="logStore.setFilterLevel(lvl.value)"
        >
          <span class="chip-dot" v-if="lvl.value" :class="levelDotColors[lvl.value]"></span>
          {{ lvl.label }}
        </UiButton>
      </div>
      <div class="log-search">
        <UiInput
          v-model="logStore.filterSearch"
          class="log-search-input"
          placeholder="Filter logs..."
        >
          <template #prefix>
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"/>
              <line x1="21" y1="21" x2="16.65" y2="16.65"/>
            </svg>
          </template>
        </UiInput>
      </div>
    </div>

    <!-- Log entries -->
    <div ref="containerRef" class="log-entries" @scroll="onScroll">
      <div
        v-for="(entry, idx) in filteredEntries"
        :key="idx"
        class="log-entry"
        :class="`log-level-${entry.level}`"
      >
        <span class="log-time">{{ formatTime(entry.timestamp) }}</span>
        <span class="log-badge" :class="levelBadgeColors[entry.level] || ''">
          {{ entry.level.toUpperCase() }}
        </span>
        <span class="log-source">{{ entry.source }}</span>
        <span class="log-msg" :class="levelColors[entry.level] || ''">{{ entry.message }}</span>
        <span v-if="entry.details" class="log-details">{{ entry.details }}</span>
      </div>
      <div v-if="filteredEntries.length === 0" class="log-empty">
        <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="empty-icon">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
          <polyline points="14 2 14 8 20 8"/>
        </svg>
        <span>No log entries</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.log-viewer {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: linear-gradient(180deg, color-mix(in srgb, var(--color-bg-app) 70%, transparent) 0%, rgba(10, 15, 30, 0.9) 100%);
  border: 1px solid rgba(255, 255, 255, 0.04);
  border-radius: 0.75rem;
  overflow: hidden;
  position: relative;
}

.log-viewer::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 1px;
  background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--color-accent) 15%, transparent), transparent);
  pointer-events: none;
}

.log-viewer.embedded-mode {
  border-radius: 0;
}

.log-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-2) var(--space-4);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  background: color-mix(in srgb, var(--color-bg-app) 60%, transparent);
  backdrop-filter: blur(8px);
  flex-shrink: 0;
}

.log-header-left {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.log-header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.625rem;
  height: 1.625rem;
  border-radius: 0.375rem;
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  color: color-mix(in srgb, var(--color-accent) 70%, transparent);
}

.log-header-text {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
}

.log-title {
  font-size: var(--text-base);
  font-weight: 600;
  color: var(--color-text-primary);
  letter-spacing: 0.02em;
}

.log-count {
  font-size: var(--text-xs);
  font-weight: 500;
  color: var(--color-text-muted);
  background: rgba(255, 255, 255, 0.04);
  padding: 0.0625rem 0.375rem;
  border-radius: var(--radius-md);
  font-variant-numeric: tabular-nums;
}

.log-header-actions {
  display: flex;
  gap: var(--space-1);
}

.log-action-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2);
  font-size: var(--text-xs);
  font-weight: 500;
  border-radius: 0.375rem;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(255, 255, 255, 0.02);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  white-space: nowrap;
  line-height: 1.4;
}

.log-action-btn:hover {
  background: rgba(255, 255, 255, 0.07);
  border-color: rgba(255, 255, 255, 0.1);
  color: var(--color-text-primary);
}

.log-action-btn.pause-active {
  background: color-mix(in srgb, var(--color-warning) 10%, transparent);
  border-color: color-mix(in srgb, var(--color-warning) 25%, transparent);
  color: var(--color-warning);
}

.log-filters {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-4);
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  background: color-mix(in srgb, var(--color-bg-app) 35%, transparent);
  flex-shrink: 0;
}

.log-level-filters {
  display: flex;
  gap: var(--space-1);
  flex-shrink: 0;
}

.log-level-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2);
  font-size: 0.625rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  border-radius: 0.375rem;
  border: 1px solid transparent;
  background: transparent;
  color: var(--color-text-muted);
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  text-transform: uppercase;
}

.chip-dot {
  width: 0.375rem;
  height: 0.375rem;
  border-radius: 50%;
  flex-shrink: 0;
}

.log-level-chip:hover {
  color: var(--color-text-secondary);
  background: rgba(255, 255, 255, 0.04);
}

.log-level-chip.active {
  color: var(--color-text-primary);
  border-color: rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.06);
}

.chip-all.active {
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-accent) 20%, transparent);
  color: color-mix(in srgb, var(--color-accent) 90%, transparent);
}

.chip-debug.active {
  background: color-mix(in srgb, var(--color-text-muted) 12%, transparent);
  border-color: color-mix(in srgb, var(--color-text-muted) 20%, transparent);
  color: var(--color-text-secondary);
}

.chip-info.active {
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-accent) 20%, transparent);
  color: color-mix(in srgb, var(--color-accent) 90%, transparent);
}

.chip-warn.active {
  background: color-mix(in srgb, var(--color-warning) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-warning) 20%, transparent);
  color: color-mix(in srgb, var(--color-warning) 90%, transparent);
}

.chip-error.active {
  background: color-mix(in srgb, var(--color-danger) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-danger) 20%, transparent);
  color: color-mix(in srgb, var(--color-danger) 90%, transparent);
}

.log-search {
  flex: 1;
  min-width: 0;
}

.log-search-input {
  width: 100%;
}

.log-entries {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-1) 0;
  font-family: 'JetBrains Mono', 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
  font-size: 0.7rem;
  line-height: 1.6;
}

.log-entries::-webkit-scrollbar {
  width: var(--space-1);
}

.log-entries::-webkit-scrollbar-track {
  background: transparent;
}

.log-entries::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.06);
  border-radius: var(--radius-sm);
}

.log-entries::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.12);
}

.log-entry {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-4);
  transition: background 0.15s ease;
  position: relative;
}

.log-entry:hover {
  background: rgba(255, 255, 255, 0.025);
}

.log-level-error {
  background: color-mix(in srgb, var(--color-danger) 3%, transparent);
  border-left: 2px solid color-mix(in srgb, var(--color-danger) 30%, transparent);
  padding-left: calc(var(--space-4) - 2px);
}

.log-level-error:hover {
  background: color-mix(in srgb, var(--color-danger) 6%, transparent);
}

.log-level-warn {
  border-left: 2px solid transparent;
  padding-left: calc(var(--space-4) - 2px);
}

.log-time {
  color: var(--color-text-muted);
  flex-shrink: 0;
  font-size: 0.625rem;
  min-width: 5.75rem;
  font-variant-numeric: tabular-nums;
  opacity: 0.85;
}

.log-badge {
  flex-shrink: 0;
  font-size: 0.5625rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  padding: 0.0625rem 0.375rem;
  border-radius: var(--radius-md);
  min-width: 2.75rem;
  text-align: center;
}

.log-source {
  color: var(--color-text-muted);
  flex-shrink: 0;
  min-width: 5rem;
  max-width: 7rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.625rem;
  opacity: 0.7;
}

.log-msg {
  flex: 1;
  min-width: 0;
  word-break: break-all;
}

.log-details {
  display: block;
  color: var(--color-text-muted);
  font-size: 0.625rem;
  padding-left: var(--space-3);
  margin-top: 0.125rem;
  white-space: pre-wrap;
  opacity: 0.7;
}

.log-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-3);
  height: 10rem;
  color: var(--color-text-muted);
  font-size: var(--text-sm);
}

.empty-icon {
  opacity: 0.3;
}
</style>

<style scoped>
:root[data-theme='light'] .log-viewer {
  background: linear-gradient(180deg, color-mix(in srgb, var(--color-bg-app) 95%, transparent) 0%, rgba(241, 245, 249, 0.98) 100%);
  border-color: rgba(0, 0, 0, 0.06);
}

:root[data-theme='light'] .log-viewer::before {
  background: linear-gradient(90deg, transparent, rgba(37, 99, 235, 0.08), transparent);
}

:root[data-theme='light'] .log-header {
  border-bottom-color: rgba(0, 0, 0, 0.06);
  background: rgba(255, 255, 255, 0.8);
}

:root[data-theme='light'] .log-header-icon {
  background: rgba(37, 99, 235, 0.08);
  color: rgba(37, 99, 235, 0.7);
}

:root[data-theme='light'] .log-title {
  color: var(--color-text-primary);
}

:root[data-theme='light'] .log-count {
  color: var(--color-text-muted);
  background: rgba(0, 0, 0, 0.04);
}

:root[data-theme='light'] .log-action-btn {
  border-color: rgba(0, 0, 0, 0.08);
  background: rgba(0, 0, 0, 0.02);
  color: var(--color-text-secondary);
}

:root[data-theme='light'] .log-action-btn:hover {
  background: rgba(0, 0, 0, 0.05);
  border-color: rgba(0, 0, 0, 0.12);
  color: var(--color-text-primary);
}

:root[data-theme='light'] .log-action-btn.pause-active {
  background: rgba(217, 119, 6, 0.08);
  border-color: rgba(217, 119, 6, 0.2);
  color: var(--color-warning);
}

:root[data-theme='light'] .log-filters {
  border-bottom-color: rgba(0, 0, 0, 0.05);
  background: color-mix(in srgb, var(--color-bg-app) 50%, transparent);
}

:root[data-theme='light'] .log-level-chip {
  color: var(--color-text-muted);
}

:root[data-theme='light'] .log-level-chip:hover {
  color: var(--color-text-secondary);
  background: rgba(0, 0, 0, 0.03);
}

:root[data-theme='light'] .log-level-chip.active {
  color: var(--color-text-primary);
  border-color: rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.05);
}

:root[data-theme='light'] .chip-all.active {
  background: rgba(37, 99, 235, 0.08);
  border-color: rgba(37, 99, 235, 0.2);
  color: rgba(37, 99, 235, 0.9);
}

:root[data-theme='light'] .chip-debug.active {
  background: color-mix(in srgb, var(--color-text-muted) 10%, transparent);
  border-color: color-mix(in srgb, var(--color-text-muted) 20%, transparent);
  color: var(--color-text-muted);
}

:root[data-theme='light'] .chip-info.active {
  background: color-mix(in srgb, var(--color-accent) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-accent) 20%, transparent);
  color: color-mix(in srgb, var(--color-accent) 90%, transparent);
}

:root[data-theme='light'] .chip-warn.active {
  background: color-mix(in srgb, var(--color-warning) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-warning) 20%, transparent);
  color: color-mix(in srgb, var(--color-warning) 90%, transparent);
}

:root[data-theme='light'] .chip-error.active {
  background: color-mix(in srgb, var(--color-danger) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-danger) 20%, transparent);
  color: color-mix(in srgb, var(--color-danger) 90%, transparent);
}

:root[data-theme='light'] .log-entries::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.08);
}

:root[data-theme='light'] .log-entries::-webkit-scrollbar-thumb:hover {
  background: rgba(0, 0, 0, 0.15);
}

:root[data-theme='light'] .log-entry:hover {
  background: rgba(0, 0, 0, 0.02);
}

:root[data-theme='light'] .log-level-error {
  background: rgba(220, 38, 38, 0.03);
  border-left-color: rgba(220, 38, 38, 0.25);
}

:root[data-theme='light'] .log-level-error:hover {
  background: rgba(220, 38, 38, 0.06);
}

:root[data-theme='light'] .log-time {
  color: var(--color-text-muted);
}

:root[data-theme='light'] .log-source {
  color: var(--color-text-muted);
}

:root[data-theme='light'] .log-details {
  color: var(--color-text-muted);
}

:root[data-theme='light'] .log-empty {
  color: var(--color-text-muted);
}
</style>
