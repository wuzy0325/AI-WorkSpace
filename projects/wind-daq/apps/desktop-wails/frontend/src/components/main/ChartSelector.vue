<script setup lang="ts">
import UiButton from '@components/ui/UiButton.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import { useI18nStore } from '@stores/i18nStore'

/** 通道选择列表项：颜色/样式/可见性由父组件预计算，避免子组件持有业务状态。 */
export interface SelectorChannel {
  index: number
  name: string
  color: string
  style: Record<string, string>
  visible: boolean
}

defineProps<{
  profileName: string
  channels: SelectorChannel[]
  selectedCount: number
  totalCount: number
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'toggle', index: number): void
  (e: 'setAll', visible: boolean): void
}>()

const i18n = useI18nStore()
</script>

<template>
  <div class="chart-selector" @click.self="emit('close')">
    <div class="chart-selector__panel" @click.stop>
      <div class="chart-selector__header">
        <div class="chart-selector__title-row">
          <h3 class="chart-selector__title">{{ i18n.t.channelSelection || '通道选择' }}</h3>
          <p class="chart-selector__subtitle">{{ profileName }}</p>
        </div>
        <UiButton variant="ghost" size="sm" aria-label="关闭" @click="emit('close')">
          <template #icon>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </template>
        </UiButton>
      </div>

      <div class="chart-selector__grid">
        <label
          v-for="channel in channels"
          :key="channel.index"
          class="chart-selector__item"
          :class="{ 'chart-selector__item--active': channel.visible }"
          :style="channel.style"
        >
          <span class="chart-selector__item-info">
            <span class="chart-selector__dot" :style="{ background: channel.color }" />
            <span class="chart-selector__name" :title="channel.name">{{ channel.name }}</span>
          </span>
          <span class="chart-selector__channel">CH{{ String(channel.index + 1).padStart(2, '0') }}</span>
          <UiCheckbox
            :checked="channel.visible"
            @update:checked="emit('toggle', channel.index)"
          />
        </label>
      </div>

      <div class="chart-selector__footer">
        <div class="chart-selector__footer-left">
          <span class="chart-selector__count">
            {{ i18n.t.selectedCount || '已选' }} {{ selectedCount }} / {{ totalCount }}
          </span>
          <UiButton variant="ghost" size="sm" @click="emit('setAll', true)">{{ i18n.t.selectAll || '全选' }}</UiButton>
          <UiButton variant="ghost" size="sm" @click="emit('setAll', false)">{{ i18n.t.clearAll || '清空' }}</UiButton>
        </div>
        <div class="chart-selector__footer-right">
          <UiButton variant="primary" size="sm" @click="emit('close')">{{ i18n.t.done || '完成' }}</UiButton>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chart-selector {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

.chart-selector__panel {
  width: 100%;
  max-width: 48rem;
  max-height: 78vh;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 0.75rem;
  box-shadow: 0 24px 56px -16px rgba(0, 0, 0, 0.45);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

:root[data-theme='light'] .chart-selector__panel {
  background: var(--bg-panel);
  border-color: var(--border-strong);
}

.chart-selector__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.625rem 0.875rem 0.625rem 1rem;
  border-bottom: 1px solid var(--border-default);
  flex-shrink: 0;
}

:root[data-theme='light'] .chart-selector__header {
  border-bottom: 1px solid var(--border-default);
}

.chart-selector__title-row {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
}

.chart-selector__title {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1.2;
  margin: 0;
}

:root[data-theme='light'] .chart-selector__title {
  color: var(--text-primary);
}

.chart-selector__subtitle {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--text-muted);
  line-height: 1.2;
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chart-selector__subtitle::before {
  content: '·';
  margin-right: 0.375rem;
  color: var(--border-strong);
}

.chart-selector__grid {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem 0.75rem;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
  gap: 0.25rem;
  align-content: start;
}

.chart-selector__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.25rem 0.5rem 0.25rem 0.5rem;
  border-radius: 0.375rem;
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
  transition: background 120ms ease, border-color 120ms ease;
  min-width: 0;
  gap: 0.5rem;
  min-height: 28px;
}

:root[data-theme='light'] .chart-selector__item {
  background: transparent;
  border: 1px solid transparent;
}

.chart-selector__item:hover {
  background: color-mix(in srgb, var(--text-primary) 5%, transparent);
  border-color: var(--border-default);
}

.chart-selector__item--active {
  border-color: var(--theme-color-border, rgba(16, 185, 129, 0.35));
  background: var(--theme-color-soft, rgba(16, 185, 129, 0.08));
}

.chart-selector__item-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
  flex: 1;
}

.chart-selector__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.chart-selector__name {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.2;
}

:root[data-theme='light'] .chart-selector__name {
  color: var(--text-primary);
}

.chart-selector__channel {
  font-size: var(--font-size-micro);
  font-weight: 600;
  color: var(--text-muted);
  font-family: ui-monospace, monospace;
  flex-shrink: 0;
  letter-spacing: 0.02em;
}

.chart-selector__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.5rem 0.875rem;
  border-top: 1px solid var(--border-default);
  flex-shrink: 0;
}

.chart-selector__footer-left {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.chart-selector__footer-right {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.chart-selector__count {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  margin-right: 0.5rem;
  font-variant-numeric: tabular-nums;
}

:root[data-theme='light'] .chart-selector__footer {
  border-top: 1px solid var(--border-default);
}
</style>
