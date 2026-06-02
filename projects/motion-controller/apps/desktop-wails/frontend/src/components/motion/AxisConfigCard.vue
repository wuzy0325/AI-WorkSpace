<script setup lang="ts">
import { inject } from 'vue'
import type { AxisConfig } from '@shared/types/motion'
import { getAxisThemeClass, getAxisInfoLabel, computePulsesPerUnit } from './motionConfigEditor'

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
    :class="getAxisThemeClass(axis.name)"
  >
    <div class="axis-card__header">
      <div class="axis-card__badge">{{ axis.name }}</div>
      <span class="axis-card__kind-label">{{ axis.kind === 'ROTARY' ? '旋转轴' : '直线轴' }}</span>
    </div>

    <div class="axis-card__body">
      <!-- 机械参数 -->
      <div class="axis-card__group-title">机械参数</div>
      <div class="axis-card__row">
        <div class="axis-card__field">
          <label class="axis-card__label">
            轴类型
            <span class="field-hint" @mouseenter="tooltip('LINEAR=直线运动(单位mm), ROTARY=旋转运动(单位°)', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <select :value="axis.kind" @change="onAxisUpdate(index, 'kind', ($event.target as HTMLSelectElement).value)" class="axis-card__select">
            <option value="LINEAR">直线轴</option>
            <option value="ROTARY">旋转轴</option>
          </select>
        </div>
        <div class="axis-card__field">
          <label class="axis-card__label">
            丝杆导程
            <span class="field-hint" @mouseenter="tooltip('丝杠旋转一周，螺母移动的直线距离(mm)。仅直线轴有效', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <input :value="axis.lead" @input="onAxisUpdate(index, 'lead', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input" :disabled="axis.kind === 'ROTARY'" min="0.1" step="0.1" />
        </div>
      </div>
      <div class="axis-card__row">
        <div class="axis-card__field">
          <label class="axis-card__label">
            传动比
            <span class="field-hint" @mouseenter="tooltip('电机转速与负载转速的比值。减速比>1时填写传动比。仅旋转轴有效', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <input :value="axis.gearRatio" @input="onAxisUpdate(index, 'gearRatio', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input" :disabled="axis.kind === 'LINEAR'" min="0.1" step="0.1" />
        </div>
        <div class="axis-card__field">
          <label class="axis-card__label">
            方向反转
            <span class="field-hint" @mouseenter="tooltip('勾选后轴的运动方向与默认方向相反', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <label class="mini-toggle">
            <input type="checkbox" :checked="axis.inverted" @change="onAxisUpdate(index, 'inverted', !axis.inverted)" />
            <span class="mini-toggle__track">
              <span class="mini-toggle__thumb"></span>
            </span>
          </label>
        </div>
      </div>

      <!-- 电气参数 -->
      <div class="axis-card__group-title">电气参数</div>
      <div class="axis-card__row">
        <div class="axis-card__field">
          <label class="axis-card__label">
            步距角
            <span class="field-hint" @mouseenter="tooltip('电机每步转过的角度。常见值: 1.8°, 0.9°, 7.5°', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <input :value="axis.stepsPerRev" @input="onAxisUpdate(index, 'stepsPerRev', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input" step="0.1" min="0.1" />
        </div>
        <div class="axis-card__field">
          <label class="axis-card__label">
            细分数
            <span class="field-hint" @mouseenter="tooltip('驱动器的细分数。细分数越高运动越平滑但扭矩越小', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <input :value="axis.microSteps" @input="onAxisUpdate(index, 'microSteps', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input" min="1" step="1" />
        </div>
      </div>

      <!-- 运动参数 -->
      <div class="axis-card__group-title">运动参数</div>
      <div class="axis-card__row">
        <div class="axis-card__field">
          <label class="axis-card__label">
            最大速度
            <span class="field-hint" @mouseenter="tooltip('轴的最大运动速度，单位取决于轴类型(mm/s 或 °/s)', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <input :value="axis.maxSpeed" @input="onAxisUpdate(index, 'maxSpeed', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input" min="1" />
        </div>
        <div class="axis-card__field">
          <label class="axis-card__label">
            位置源
            <span class="field-hint" @mouseenter="tooltip('register=使用驱动器内部寄存器, encoder=使用外部编码器反馈', $event)" @mouseleave="hideTooltip">?</span>
          </label>
          <select :value="axis.positionSource" @change="onAxisUpdate(index, 'positionSource', ($event.target as HTMLSelectElement).value)" class="axis-card__select">
            <option value="register">寄存器</option>
            <option value="encoder">编码器</option>
          </select>
        </div>
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
  border-radius: 0.375rem;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  overflow: hidden;
  transition: all 0.2s ease;
}
.axis-x-theme { --axis-hue: var(--axis-x); --axis-hue-soft: var(--axis-x-soft); }
.axis-y-theme { --axis-hue: var(--axis-y); --axis-hue-soft: var(--axis-y-soft); }
.axis-z-theme { --axis-hue: var(--axis-z); --axis-hue-soft: var(--axis-z-soft); }
.axis-u-theme { --axis-hue: var(--axis-u); --axis-hue-soft: var(--axis-u-soft); }
.axis-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.625rem;
  background: var(--axis-hue-soft);
  border-bottom: 1px solid var(--border-default);
}
.axis-card__badge {
  width: 1.625rem;
  height: 1.625rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  background: var(--axis-hue);
  color: white;
  font-size: 0.875rem;
  font-weight: 800;
}
.axis-card__kind-label {
  font-size: 0.65rem;
  font-weight: 600;
  color: var(--text-muted);
}
.axis-card__body {
  padding: 0.5rem 0.625rem;
}
.axis-card__row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.375rem 0.5rem;
  margin-bottom: 0.375rem;
}
.axis-card__group-title {
  font-size: 0.6rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.03em;
  text-transform: uppercase;
  margin-bottom: 0.375rem;
  margin-top: 0.375rem;
  padding-bottom: 0.125rem;
  border-bottom: 1px solid var(--border-default);
}
.axis-card__group-title:first-child {
  margin-top: 0;
}
.axis-card__field {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}
.axis-card__label {
  font-size: 0.625rem;
  font-weight: 600;
  color: var(--text-muted);
}
.axis-card__input {
  width: 100%;
  height: 1.625rem;
  padding: 0 0.375rem;
  border-radius: 0.25rem;
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-size: 0.7rem;
  text-align: right;
  outline: none;
  transition: border-color 0.2s ease;
  box-sizing: border-box;
}
.axis-card__input:focus {
  border-color: var(--axis-hue);
}
.axis-card__input:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.axis-card__select {
  width: 100%;
  height: 1.625rem;
  padding: 0 0.375rem;
  border-radius: 0.25rem;
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-size: 0.7rem;
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s ease;
  box-sizing: border-box;
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
  width: 1.625rem;
  height: 0.875rem;
  border-radius: 9999px;
  background: var(--border-strong);
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
  width: calc(0.875rem - 4px);
  height: calc(0.875rem - 4px);
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
  gap: 0.375rem;
  margin-top: 0.5rem;
  padding-top: 0.375rem;
  border-top: 1px solid var(--border-default);
}
.axis-card__info-label {
  font-size: 0.6rem;
  color: var(--text-muted);
}
.axis-card__info-value {
  font-size: 0.7rem;
  font-weight: 600;
  font-family: monospace;
  color: var(--axis-hue);
}
</style>
