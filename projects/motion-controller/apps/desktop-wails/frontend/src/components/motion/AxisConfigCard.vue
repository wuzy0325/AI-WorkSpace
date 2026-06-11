<script setup lang="ts">
import { inject } from 'vue'
import type { AxisConfig } from '@shared/types/motion'
import { getAxisThemeClass, getAxisInfoLabel, computePulsesPerUnit } from './motionConfigEditor'
import UiToggle from '@components/ui/UiToggle.vue'

const props = defineProps<{
  axis: AxisConfig
  index: number
}>()

const emit = defineEmits<{
  update: [index: number, axis: AxisConfig]
}>()

const tooltip = inject<(text: string, event: MouseEvent) => void>('showTooltip', () => {})
const hideTooltip = inject<() => void>('hideTooltip', () => {})

const POSITIVE_NUMBER_KEYS = new Set(['lead', 'gearRatio', 'stepsPerRev', 'maxSpeed'])
const POSITIVE_INTEGER_KEYS = new Set(['microSteps'])

function sanitizeNumericValue(key: string, raw: unknown): unknown {
  if (typeof raw !== 'number' || !Number.isFinite(raw)) return undefined
  if (POSITIVE_NUMBER_KEYS.has(key) && raw <= 0) return undefined
  if (POSITIVE_INTEGER_KEYS.has(key) && (!Number.isInteger(raw) || raw <= 0)) return undefined
  return raw
}

function onAxisUpdate(index: number, key: string, value: unknown) {
  if (typeof value === 'number') {
    value = sanitizeNumericValue(key, value)
  }
  emit('update', index, { ...props.axis, [key]: value })
}
</script>

<template>
  <div
    class="axis-card"
    :class="getAxisThemeClass(axis.name)"
  >
    <!-- 卡片头部 -->
    <div class="axis-card__header">
      <div class="axis-card__header-left">
        <div class="axis-card__badge">{{ axis.name }}</div>
        <span class="axis-card__kind-label">{{ axis.kind === 'ROTARY' ? '旋转轴' : '直线轴' }}</span>
      </div>
    </div>

    <!-- 卡片主体 -->
    <div class="axis-card__body">
      <!-- 第1行：轴类型 | 丝杆导程 | 传动比 | 反转 -->
      <div class="axis-card__row">
        <label class="axis-card__cell">
          <span class="axis-card__cell-label">类型</span>
          <select :value="axis.kind" @change="onAxisUpdate(index, 'kind', ($event.target as HTMLSelectElement).value)" class="axis-card__select config-select">
            <option value="LINEAR">直线</option>
            <option value="ROTARY">旋转</option>
          </select>
        </label>
        <label class="axis-card__cell">
          <span class="axis-card__cell-label">导程</span>
          <input :value="axis.lead" @input="onAxisUpdate(index, 'lead', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input config-input" :disabled="axis.kind === 'ROTARY'" min="0.1" step="0.1" />
        </label>
        <label class="axis-card__cell">
          <span class="axis-card__cell-label">传动比</span>
          <input :value="axis.gearRatio" @input="onAxisUpdate(index, 'gearRatio', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input config-input" :disabled="axis.kind === 'LINEAR'" min="0.1" step="0.1" />
        </label>
        <label class="axis-card__cell axis-card__cell--toggle">
          <span class="axis-card__cell-label">反转</span>
          <UiToggle
            size="sm"
            :model-value="axis.inverted"
            style="--toggle-color: var(--axis-hue)"
            @update:model-value="onAxisUpdate(index, 'inverted', $event)"
          />
        </label>
      </div>

      <!-- 第2行：步距角 | 细分数 | 最大速度 | 位置源 -->
      <div class="axis-card__row">
        <label class="axis-card__cell">
          <span class="axis-card__cell-label">步距角</span>
          <input :value="axis.stepsPerRev" @input="onAxisUpdate(index, 'stepsPerRev', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input config-input" step="0.1" min="0.1" />
        </label>
        <label class="axis-card__cell">
          <span class="axis-card__cell-label">细分</span>
          <input :value="axis.microSteps" @input="onAxisUpdate(index, 'microSteps', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input config-input" min="1" step="1" />
        </label>
        <label class="axis-card__cell">
          <span class="axis-card__cell-label">最大速度</span>
          <input :value="axis.maxSpeed" @input="onAxisUpdate(index, 'maxSpeed', Number(($event.target as HTMLInputElement).value))" type="number" class="axis-card__input config-input" min="1" />
        </label>
        <label class="axis-card__cell">
          <span class="axis-card__cell-label">位置源</span>
          <select :value="axis.positionSource" @change="onAxisUpdate(index, 'positionSource', ($event.target as HTMLSelectElement).value)" class="axis-card__select config-select">
            <option value="register">寄存器</option>
            <option value="encoder">编码器</option>
          </select>
        </label>
      </div>

      <!-- 底部信息栏 -->
      <div class="axis-card__info">
        <span class="axis-card__info-label">{{ getAxisInfoLabel(axis) }}</span>
        <span class="axis-card__info-value">{{ computePulsesPerUnit(axis).toFixed(2) }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ============================================================
   轴配置卡片
   ============================================================ */
.axis-card {
  display: flex;
  flex-direction: column;
  border-radius: var(--radius-lg);
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  overflow: hidden;
  transition: border-color var(--motion-base) var(--easing-standard),
              box-shadow var(--motion-base) var(--easing-standard);
}

.axis-card:hover {
  border-color: var(--axis-hue, var(--border-strong));
  box-shadow: 0 4px 12px color-mix(in srgb, var(--axis-hue, #000) 8%, transparent);
}

/* 主题色 */
.axis-x-theme { --axis-hue: var(--axis-x); --axis-hue-soft: var(--axis-x-soft); }
.axis-y-theme { --axis-hue: var(--axis-y); --axis-hue-soft: var(--axis-y-soft); }
.axis-z-theme { --axis-hue: var(--axis-z); --axis-hue-soft: var(--axis-z-soft); }
.axis-u-theme { --axis-hue: var(--axis-u); --axis-hue-soft: var(--axis-u-soft); }

/* ============================================================
   卡片头部
   ============================================================ */
.axis-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-2) var(--space-3);
  background: var(--axis-hue-soft);
  border-bottom: 1px solid var(--border-default);
}

.axis-card__header-left {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.axis-card__badge {
  width: 1.5rem;
  height: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: var(--axis-hue);
  color: white;
  font-size: 0.75rem;
  font-weight: 800;
  flex-shrink: 0;
}

.axis-card__kind-label {
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--text-muted);
}

/* ============================================================
   卡片主体
   ============================================================ */
.axis-card__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-3);
}

/* ============================================================
   参数行
   ============================================================ */
.axis-card__row {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-2) var(--space-3);
}

@media (min-width: 480px) {
  .axis-card__row {
    grid-template-columns: repeat(4, 1fr);
  }
}

/* ============================================================
   参数单元格
   ============================================================ */
.axis-card__cell {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  border: none;
  background: none;
  cursor: default;
}

.axis-card__cell--toggle {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}

.axis-card__cell-label {
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--text-muted);
  white-space: nowrap;
}

/* ============================================================
   输入框和下拉框 — 使用全局 .config-input / .config-select
   ============================================================ */
.axis-card__input,
.axis-card__select {
  /* 仅补充轴主题色 focus 状态，基础样式继承全局工具类 */
}

.axis-card__input:focus,
.axis-card__select:focus {
  border-color: var(--axis-hue);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--axis-hue) 20%, transparent);
}

/* ============================================================
   底部信息栏
   ============================================================ */
.axis-card__info {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
  padding-top: var(--space-2);
  border-top: 1px solid var(--border-default);
}

.axis-card__info-label {
  font-size: 0.625rem;
  color: var(--text-muted);
}

.axis-card__info-value {
  font-size: 0.75rem;
  font-weight: 600;
  font-family: var(--font-family-mono, monospace);
  font-variant-numeric: tabular-nums;
  color: var(--axis-hue);
}
</style>
