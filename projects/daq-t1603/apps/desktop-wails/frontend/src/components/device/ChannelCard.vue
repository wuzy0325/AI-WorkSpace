<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Eye, EyeOff, Activity } from '@lucide/vue'

const props = defineProps<{
  index: number
  value: number
  unit: string
  color: string
  name: string
  precision?: number
  chartSelected: boolean
  active: boolean
}>()

const emit = defineEmits<{
  (e: 'toggleChart'): void
}>()

const displayValue = ref('---')
const isFlashing = ref(false)

function formatValue(v: number): string {
  if (v === undefined || v === null || isNaN(v)) return '---'
  const p = typeof props.precision === 'number' ? Math.max(0, Math.min(props.precision, 6)) : 2
  return v.toFixed(p)
}

// 监听数值变化，触发闪烁效果
watch(() => props.value, (newVal, oldVal) => {
  displayValue.value = formatValue(newVal)
  if (typeof newVal === 'number' && !isNaN(newVal) &&
      typeof oldVal === 'number' && !isNaN(oldVal) &&
      newVal !== oldVal) {
    isFlashing.value = true
    setTimeout(() => { isFlashing.value = false }, 400)
  }
}, { immediate: true })

const statusText = computed(() => {
  if (props.active) return '已选择'
  if (props.chartSelected) return '波形中'
  return '空闲'
})

const statusColor = computed(() => {
  if (props.active) return 'var(--accent)'
  if (props.chartSelected) return props.color
  return 'var(--text-muted)'
});
</script>

<template>
  <article
    class="card"
    :class="{
      'card--selected': chartSelected,
      'card--active': active,
    }"
    :style="{ '--ch-color': color }"
  >
    <!-- 顶部彩色条 -->
    <div class="card__topbar" :style="{ background: color }"></div>

    <!-- 头部：通道标识 + 名称 + 切换按钮 -->
    <div class="card__head">
      <span class="card__tag mono">CH{{ String(index + 1).padStart(2, '0') }}</span>
      <span class="card__name">{{ name }}</span>
      <button
        class="card__toggle"
        :class="{ 'card__toggle--on': chartSelected }"
        :title="chartSelected ? '从波形图移除' : '添加到波形图'"
        @click="emit('toggleChart')"
      >
        <Eye v-if="chartSelected" class="card__toggle-icon" />
        <EyeOff v-else class="card__toggle-icon" />
      </button>
    </div>

    <!-- 数值显示区 -->
    <div class="card__value-row">
      <span
        class="card__value mono"
        :class="{ 'card__value--flash': isFlashing }"
      >{{ displayValue }}</span>
      <span class="card__unit">{{ unit }}</span>
    </div>

    <!-- 底部状态栏 -->
    <div class="card__footer">
      <Activity
        class="card__footer-icon"
        :style="{ color: statusColor }"
      />
      <span
        class="card__footer-text"
        :style="{ color: statusColor }"
      >
        {{ statusText }}
      </span>
    </div>
  </article>
</template>

<style scoped>
.card {
  position: relative;
  background: var(--card-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  padding: 0.7rem 0.85rem 0.55rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  transition: all var(--motion-fast) var(--easing-standard);
  overflow: hidden;
}

/* 悬停光效背景 */
.card::before {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, var(--ch-color, var(--accent)) 0%, transparent 40%);
  opacity: 0;
  transition: opacity var(--motion-fast) var(--easing-standard);
  pointer-events: none;
  border-radius: inherit;
}

.card:hover {
  background: var(--card-bg-hover);
  border-color: var(--border-hover);
  transform: translateY(-1px);
  box-shadow: var(--shadow-sm);
}

.card:hover::before {
  opacity: 0.04;
}

/* 选中状态 */
.card--selected {
  border-color: var(--ch-color, var(--accent));
  box-shadow:
    0 0 0 1px var(--ch-color, var(--accent)),
    0 6px 18px color-mix(in srgb, var(--ch-color, var(--accent)) 20%, transparent);
}

.card--selected::before {
  opacity: 0.06;
}

.card--selected:hover {
  box-shadow:
    0 0 0 1px var(--ch-color, var(--accent)),
    0 8px 24px color-mix(in srgb, var(--ch-color, var(--accent)) 30%, transparent);
}

/* 顶部彩色条 */
.card__topbar {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: var(--ch-color, var(--accent));
  opacity: 0.85;
  transition: height var(--motion-fast) var(--easing-standard);
}

.card:hover .card__topbar {
  height: 4px;
}

/* 头部布局 */
.card__head {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.card__tag {
  font-size: 0.6rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.08em;
  background: var(--btn-bg);
  padding: 0.15rem 0.4rem;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}

.card__name {
  font-size: var(--font-size-xs);
  color: var(--text-secondary);
  font-weight: 600;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

/* 波形图切换按钮 */
.card__toggle {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--btn-bg);
  color: var(--text-muted);
  border: 1px solid var(--border-default);
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.card__toggle:hover {
  color: var(--accent);
  background: var(--accent-soft);
  border-color: var(--accent-border);
  transform: scale(1.1);
}

.card__toggle--on {
  background: var(--ch-color, var(--accent));
  color: #ffffff;
  border-color: var(--ch-color, var(--accent));
  box-shadow: 0 0 10px color-mix(in srgb, var(--ch-color, var(--accent)) 40%, transparent);
}

.card__toggle--on:hover {
  background: var(--ch-color, var(--accent));
  color: #ffffff;
  box-shadow: 0 0 14px color-mix(in srgb, var(--ch-color, var(--accent)) 60%, transparent);
}

.card__toggle-icon {
  width: 12px;
  height: 12px;
}

/* 数值显示 */
.card__value-row {
  display: flex;
  align-items: baseline;
  gap: 0.35rem;
  position: relative;
  z-index: 1;
  min-height: 2rem;
}

.card__value {
  font-size: 1.5rem;
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.02em;
  line-height: 1;
  font-variant-numeric: tabular-nums;
  transition: color var(--motion-fast) var(--easing-standard);
}

/* 数值变化闪烁效果 */
.card__value--flash {
  animation: value-flash 0.4s var(--easing-standard);
}

.card--selected .card__value {
  color: var(--ch-color, var(--accent));
}

.card__unit {
  font-size: 0.65rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.04em;
}

/* 底部状态栏 */
.card__footer {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding-top: 0.3rem;
  border-top: 1px solid var(--divider-color);
  margin-top: auto;
}

.card__footer-icon {
  width: 11px;
  height: 11px;
  transition: color var(--motion-fast) var(--easing-standard);
}

.card__footer-text {
  font-size: 0.6rem;
  font-weight: 600;
  transition: color var(--motion-fast) var(--easing-standard);
}

/* 减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .card,
  .card::before,
  .card__topbar,
  .card__toggle,
  .card__value,
  .card__footer-icon,
  .card__footer-text {
    transition: none;
  }

  .card__value--flash {
    animation: none;
  }

  .card:hover {
    transform: none;
  }
}
</style>
