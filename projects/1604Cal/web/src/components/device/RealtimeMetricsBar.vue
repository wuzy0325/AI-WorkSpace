<template>
  <div class="metrics-bar">
    <div class="metric-item pressure-current">
      <span class="metric-label">当前压力</span>
      <span class="metric-value">{{ formatPressure(currentPressure) }}</span>
      <span class="metric-unit">kPa</span>
      <div
        class="mini-bar"
        role="progressbar"
        :aria-valuenow="currentPressure"
        aria-valuemin="0"
        :aria-valuemax="targetPressure ?? undefined"
      >
        <div
          class="mini-bar-fill"
          :style="{ transform: 'scaleX(' + pressurePercent + ')' }"
          :class="pressureBarClass"
        />
      </div>
    </div>

    <div class="metric-separator" />

    <div class="metric-item pressure-target">
      <span class="metric-label">目标压力</span>
      <span class="metric-value">{{ formatPressure(targetPressure) }}</span>
      <span class="metric-unit">kPa</span>
      <span
        class="target-diff"
        :class="targetDiffClass"
      >
        {{ targetDiffText }}
      </span>
    </div>

    <div class="metric-separator" />

    <div class="metric-item stability">
      <span class="metric-label">稳定状态</span>
      <span
        class="status-indicator"
        :class="stabilityClass"
      />
      <span
        class="status-text"
        :class="stabilityClass"
      >{{ stabilityText }}</span>
      <span class="stability-meta">{{ stableDuration }}s / ±{{ stabilityThreshold }}kPa</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  currentPressure: number
  targetPressure: number | null
  isStable: boolean
  stableDuration: number
  stabilityThreshold: number
}>(), {
  currentPressure: 0,
  targetPressure: null,
  isStable: false,
  stableDuration: 0,
  stabilityThreshold: 0.5
})

// ---- 计算属性 ----

const pressurePercent = computed(() => {
  const target = props.targetPressure ?? 0
  if (target <= 0) return 0
  const percent = (props.currentPressure / target) * 100
  return Math.min(Math.max(percent, 0), 100)
})

const pressureBarClass = computed(() => {
  if (props.targetPressure === null) return 'bar-normal'
  if (props.isStable) return 'bar-stable'
  if (Math.abs(props.currentPressure - props.targetPressure) <= props.stabilityThreshold) {
    return 'bar-approaching'
  }
  return 'bar-normal'
})

const stabilityClass = computed(() => ({
  'indicator-stable': props.isStable,
  'indicator-unstable': !props.isStable
}))

const stabilityText = computed(() => props.isStable ? '已稳定' : '未稳定')

const targetDiff = computed<number | null>(() => {
  if (props.targetPressure === null) return null
  return props.currentPressure - props.targetPressure
})

const targetDiffClass = computed(() => {
  if (targetDiff.value === null) return 'diff-unknown'
  const diff = Math.abs(targetDiff.value)
  if (diff <= props.stabilityThreshold) return 'diff-ok'
  if (diff <= props.stabilityThreshold * 3) return 'diff-warning'
  return 'diff-error'
})

const targetDiffText = computed(() => {
  if (targetDiff.value === null) return '--'
  const diff = targetDiff.value
  const sign = diff > 0 ? '+' : ''
  return `${sign}${formatPressure(diff)} kPa`
})

// ---- 工具函数 ----

function formatPressure(value: number | null): string {
  if (value === null || value === undefined) return '--.--'
  return value.toFixed(2)
}
</script>

<style scoped lang="scss">
/* ===== 紧凑指标栏 ===== */
.metrics-bar {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: 3px;
  padding: var(--spacing-xs) var(--spacing-md);
}

.metric-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  white-space: nowrap;
}

.metric-label {
  font-size: 11px;
  font-weight: 500;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.metric-value {
  font-family: Consolas, monospace;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  line-height: 1;
}

.metric-unit {
  font-size: 11px;
  color: var(--text-muted);
  font-weight: 500;
}

.metric-separator {
  width: 1px;
  height: 20px;
  background: var(--border-color-strong);
  flex-shrink: 0;
}

/* 迷你压力条 */
.mini-bar {
  width: 60px;
  height: 4px;
  background: var(--bg-primary);
  border-radius: 2px;
  overflow: hidden;
  margin-left: var(--spacing-xs);
}

.mini-bar-fill {
  height: 100%;
  border-radius: 2px;
  width: 100%;
  transform-origin: left;
  transition: transform 0.3s ease, background-color 0.3s ease;
}

.bar-normal { background: var(--status-info); }
.bar-approaching { background: var(--status-warning); }
.bar-stable { background: var(--status-success); }

/* 目标差值 */
.target-diff {
  font-size: 11px;
  font-weight: 500;
  padding: 1px 5px;
  border-radius: 2px;
  font-family: Consolas, monospace;
}

.diff-ok { background: var(--status-success-bg); color: var(--status-success); }
.diff-warning { background: var(--status-warning-bg); color: var(--status-warning); }
.diff-error { background: var(--status-error-bg); color: var(--status-error); }
.diff-unknown { background: var(--bg-quaternary); color: var(--text-muted); }

/* 稳定状态 */
.status-indicator {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  transition: background-color 0.3s ease;
}

.indicator-stable {
  background: var(--status-success);
  box-shadow: 0 0 0 2px var(--status-success-bg);
}

.indicator-unstable {
  background: var(--status-warning);
}

.status-text {
  font-size: 12px;
  font-weight: 600;
}

.status-text.indicator-stable { color: var(--status-success); }
.status-text.indicator-unstable { color: var(--status-warning); }

.stability-meta {
  font-size: 10px;
  color: var(--text-muted);
  font-family: Consolas, monospace;
}

@media (max-width: 768px) {
  .metrics-bar {
    flex-wrap: wrap;
  }

  .metric-separator {
    display: none;
  }
}
</style>