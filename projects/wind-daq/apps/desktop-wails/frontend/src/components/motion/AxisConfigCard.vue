<script setup lang="ts">
import { inject } from 'vue'
import type { AxisConfig } from '@shared/types/motion'
import { getAxisThemeClass, getAxisInfoLabel, computePulsesPerUnit } from './motionConfigEditor'
import { NCheckbox, NInputNumber, NSelect } from 'naive-ui'

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
        <NCheckbox :checked="axis.enabled" @update:checked="onAxisUpdate(index, 'enabled', $event)" size="small" />
        <span>{{ axis.enabled ? '启用' : '禁用' }}</span>
      </label>
    </div>

    <div class="axis-card__body">
      <div class="axis-card__group-title">机械参数</div>
      <div class="axis-card__group">
        <label class="axis-card__label">
          轴类型
          <span class="field-hint" @mouseenter="tooltip('LINEAR=直线运动(单位mm), ROTARY=旋转运动(单位°)', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <NSelect :value="axis.kind" @update:value="onAxisUpdate(index, 'kind', $event)" size="tiny" style="width:96px" :disabled="!axis.enabled" :options="[{value:'LINEAR',label:'直线轴'},{value:'ROTARY',label:'旋转轴'}]" />
      </div>

      <div class="axis-card__group">
        <label class="axis-card__label">
          丝杆导程
          <span class="field-hint" @mouseenter="tooltip('丝杠旋转一周，螺母移动的直线距离(mm)。仅直线轴有效', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <NInputNumber :value="axis.lead" @update:value="onAxisUpdate(index, 'lead', $event ?? 0)" size="tiny" style="width:80px" :disabled="!axis.enabled || axis.kind === 'ROTARY'" :min="0.1" :step="0.1" />
      </div>

      <div class="axis-card__group">
        <label class="axis-card__label">
          传动比
          <span class="field-hint" @mouseenter="tooltip('电机转速与负载转速的比值。减速比>1时填写传动比。仅旋转轴有效', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <NInputNumber :value="axis.gearRatio" @update:value="onAxisUpdate(index, 'gearRatio', $event ?? 0)" size="tiny" style="width:80px" :disabled="!axis.enabled || axis.kind === 'LINEAR'" :min="0.1" :step="0.1" />
      </div>

      <div class="axis-card__group-title">电气参数</div>
      <div class="axis-card__group">
        <label class="axis-card__label">
          步距角 (°/step)
          <span class="field-hint" @mouseenter="tooltip('电机每步转过的角度。常见值: 1.8°, 0.9°, 7.5°', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <NInputNumber :value="axis.stepsPerRev" @update:value="onAxisUpdate(index, 'stepsPerRev', $event ?? 0)" size="tiny" style="width:80px" :disabled="!axis.enabled" :step="0.1" :min="0.1" />
      </div>

      <div class="axis-card__group">
        <label class="axis-card__label">
          细分数
          <span class="field-hint" @mouseenter="tooltip('驱动器的细分数。细分数越高运动越平滑但扭矩越小', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <NInputNumber :value="axis.microSteps" @update:value="onAxisUpdate(index, 'microSteps', $event ?? 0)" size="tiny" style="width:80px" :disabled="!axis.enabled" :min="1" :step="1" />
      </div>

      <div class="axis-card__group-title">运动参数</div>
      <div class="axis-card__group">
        <label class="axis-card__label">
          最大速度
          <span class="field-hint" @mouseenter="tooltip('轴的最大运动速度，单位取决于轴类型(mm/s 或 °/s)', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <NInputNumber :value="axis.maxSpeed" @update:value="onAxisUpdate(index, 'maxSpeed', $event ?? 0)" size="tiny" style="width:80px" :disabled="!axis.enabled" :min="1" />
      </div>

      <div class="axis-card__group">
        <label class="axis-card__label">
          位置源
          <span class="field-hint" @mouseenter="tooltip('register=使用驱动器内部寄存器, encoder=使用外部编码器反馈', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <NSelect :value="axis.positionSource" @update:value="onAxisUpdate(index, 'positionSource', $event)" size="tiny" style="width:96px" :disabled="!axis.enabled" :options="[{value:'register',label:'寄存器'},{value:'encoder',label:'编码器'}]" />
      </div>

      <div class="axis-card__group">
        <label class="axis-card__label">
          方向反转
          <span class="field-hint" @mouseenter="tooltip('勾选后轴的运动方向与默认方向相反', $event)" @mouseleave="hideTooltip">?</span>
        </label>
        <label class="mini-toggle">
          <NCheckbox :checked="axis.inverted" @update:checked="onAxisUpdate(index, 'inverted', $event)" size="small" :disabled="!axis.enabled" />
          <span class="mini-toggle__track">
            <span class="mini-toggle__thumb"></span>
          </span>
        </label>
      </div>

      <div class="axis-card__info">
        <span class="axis-card__info-label">{{ getAxisInfoLabel(axis) }}</span>
        <span class="axis-card__info-value">{{ computePulsesPerUnit(axis).toFixed(2) }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.axis-card {
  border-radius: 0.5rem;
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
  padding: 0.75rem;
  background: var(--axis-hue-soft);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.axis-card__badge {
  width: 2rem;
  height: 2rem;
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
  font-size: 0.7rem;
  font-weight: 600;
  color: #64748b;
  cursor: pointer;
}
.axis-card__body {
  padding: 0.75rem;
}
.axis-card__group {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}
.axis-card__group-title {
  font-size: 0.65rem;
  font-weight: 700;
  color: #64748b;
  letter-spacing: 0.03em;
  text-transform: uppercase;
  margin-bottom: 0.5rem;
  margin-top: 0.5rem;
  padding-bottom: 0.25rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
}
.axis-card__label {
  font-size: 0.675rem;
  font-weight: 600;
  color: #64748b;
}
.axis-card__input {
  width: 6rem;
  height: 1.75rem;
  padding: 0 0.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.2);
  color: #e2e8f0;
  font-size: 0.75rem;
  text-align: right;
  outline: none;
  transition: border-color 0.2s ease;
}
:root[data-theme='light'] .axis-card__input {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
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
  padding: 0 0.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.2);
  color: #e2e8f0;
  font-size: 0.7rem;
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s ease;
}
:root[data-theme='light'] .axis-card__select {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
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
  gap: 0.5rem;
  margin-top: 0.75rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}
.axis-card__info-label {
  font-size: 0.65rem;
  color: #64748b;
}
.axis-card__info-value {
  font-size: 0.75rem;
  font-weight: 600;
  font-family: monospace;
  color: var(--axis-hue);
}
</style>