<script setup lang="ts">
import { inject } from 'vue'
import type { AxisConfig } from '@shared/types/motion'
import { getAxisThemeClass } from './motionConfigEditor'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'

const props = defineProps<{
  axis: AxisConfig
  index: number
}>()

const emit = defineEmits<{
  update: [index: number, axis: AxisConfig]
}>()

const tooltip = inject<(text: string, event: MouseEvent) => void>('showTooltip', () => {})
const hideTooltip = inject<() => void>('hideTooltip', () => {})

function onAxisUpdate(index: number, key: string, value: unknown) {
  emit('update', index, { ...props.axis, [key]: value })
}
</script>

<template>
  <div
    class="axis-card"
    :class="[getAxisThemeClass(axis.name), { 'axis-card--disabled': !axis.enabled }]"
  >
    <div class="axis-card__header">
      <div class="axis-card__badge">{{ axis.name }}</div>
      <label class="axis-card__toggle">
        <UiCheckbox :checked="axis.enabled" @update:checked="onAxisUpdate(index, 'enabled', $event)" />
        <span>{{ axis.enabled ? '启用' : '禁用' }}</span>
      </label>
    </div>

    <div class="axis-card__body">
      <div class="axis-card__group">
        <label class="axis-card__label">
          轴类型
          <span class="field-hint" @mouseenter="tooltip('LINEAR=直线运动(单位mm), ROTARY=旋转运动(单位°)', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <UiSelect :model-value="axis.kind" @update:model-value="onAxisUpdate(index, 'kind', $event)" class="input-width-96" :disabled="!axis.enabled" :options="[{value:'LINEAR',label:'直线轴'},{value:'ROTARY',label:'旋转轴'}]" />
      </div>

      <div class="axis-card__group-title">运动参数</div>
      <div class="axis-card__group">
        <label class="axis-card__label">
          最大速度
          <span class="field-hint" @mouseenter="tooltip('轴的最大运动速度，单位取决于轴类型(mm/s 或 °/s)', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <UiInputNumber :model-value="axis.maxSpeed" @update:model-value="onAxisUpdate(index, 'maxSpeed', $event ?? 0)" class="input-width-80" :disabled="!axis.enabled" :min="1" />
      </div>

      <div class="axis-card__group">
        <label class="axis-card__label">
          方向反转
          <span class="field-hint" @mouseenter="tooltip('勾选后轴的运动方向与默认方向相反', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <label class="mini-toggle">
          <UiCheckbox :checked="axis.inverted" @update:checked="onAxisUpdate(index, 'inverted', $event)" :disabled="!axis.enabled" />
          <span class="mini-toggle__track">
            <span class="mini-toggle__thumb"></span>
          </span>
        </label>
      </div>
    </div>
  </div>
</template>

<style scoped>
.axis-card {
  border-radius: var(--radius-xl);
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.1);
  overflow: hidden;
  transition: all 0.2s ease;
}
:root[data-theme='light'] .axis-card {
  border: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(0, 0, 0, 0.02);
}
.axis-card--disabled {
  opacity: 0.5;
}
.axis-x-theme { --axis-hue: var(--axis-x); --axis-hue-soft: var(--axis-x-soft); }
.axis-y-theme { --axis-hue: var(--axis-y); --axis-hue-soft: var(--axis-y-soft); }
.axis-z-theme { --axis-hue: var(--axis-z); --axis-hue-soft: var(--axis-z-soft); }
.axis-u-theme { --axis-hue: var(--axis-u); --axis-hue-soft: var(--axis-u-soft); }
.axis-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3);
  background: var(--axis-hue-soft);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.axis-card__badge {
  width: var(--space-8);
  height: var(--space-8);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.375rem;
  background: var(--axis-hue);
  color: white;
  font-size: 1rem;
  font-weight: 800;
}
.axis-card__toggle {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--text-muted);
  cursor: pointer;
}
.axis-card__body {
  padding: var(--space-3);
}
.axis-card__group {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-2);
}
.axis-card__group-title {
  font-size: var(--font-size-2xs);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.03em;
  text-transform: uppercase;
  margin-bottom: var(--space-2);
  margin-top: var(--space-2);
  padding-bottom: var(--space-1);
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
}
.axis-card__label {
  font-size: var(--font-size-2xs);
  font-weight: 600;
  color: var(--text-muted);
}
.axis-card__input {
  width: 6rem;
  height: 1.75rem;
  padding: 0 var(--space-2);
  border-radius: var(--radius-md);
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font-size: var(--font-size-sm);
  text-align: right;
  outline: none;
  transition: border-color 0.2s ease;
}
:root[data-theme='light'] .axis-card__input {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: var(--text-primary);
}
.axis-card__input:focus {
  border-color: var(--axis-hue);
}
.axis-card__input:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.axis-card__select {
  width: 6rem;
  height: 1.75rem;
  padding: 0 var(--space-2);
  border-radius: var(--radius-md);
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font-size: var(--font-size-xs);
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s ease;
}
:root[data-theme='light'] .axis-card__select {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: var(--text-primary);
}
.axis-card__select:focus {
  border-color: var(--axis-hue);
}
.axis-card__select:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.mini-toggle {
  display: inline-flex;
  cursor: pointer;
}
.mini-toggle input {
  display: none;
}
.mini-toggle__track {
  width: 1.75rem;
  height: 1rem;
  border-radius: 9999px;
  background: rgba(0, 0, 0, 0.3);
  position: relative;
  transition: background 0.2s ease;
  flex-shrink: 0;
}
.mini-toggle input:checked + .mini-toggle__track {
  background: var(--axis-hue);
}
.mini-toggle__thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: calc(1rem - 4px);
  height: calc(1rem - 4px);
  border-radius: 50%;
  background: white;
  transition: transform 0.2s ease;
}
.mini-toggle input:checked + .mini-toggle__track .mini-toggle__thumb {
  transform: translateX(0.75rem);
}
.axis-card__info {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
  margin-top: var(--space-3);
  padding-top: var(--space-2);
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}
.axis-card__info-label {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
}
.axis-card__info-value {
  font-size: var(--font-size-sm);
  font-weight: 600;
  font-family: monospace;
  color: var(--axis-hue);
}

.input-width-80 {
  width: 80px;
}

.input-width-96 {
  width: 96px;
}
</style>