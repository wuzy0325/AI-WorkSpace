<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Activity } from '@lucide/vue'

const props = defineProps<{
  index: number
  value: number
  unit: string
  color: string
  name: string
  precision?: number
  active: boolean
}>()

const emit = defineEmits<{
  (e: 'changeColor', color: string): void
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
  return '空闲'
})

const statusColor = computed(() => {
  if (props.active) return 'var(--accent)'
  return 'var(--text-muted)'
});

/** 打开原生调色盘 */
function openColorPicker() {
  const input = document.getElementById(`ch-color-${props.index}`) as HTMLInputElement | null
  input?.click()
}

/** 调色盘颜色变更回调 */
function onColorChange(target: HTMLInputElement) {
  emit('changeColor', target.value)
}
</script>

<template>
  <article
    class="card"
    :class="{
      'card--active': active,
    }"
    :style="{ '--ch-color': color }"
  >
    <!-- 顶部彩色条 -->
    <div class="card__topbar" :style="{ background: color }"></div>

    <!-- 头部：通道标识 + 操作按钮 -->
    <div class="card__head">
      <span class="card__tag mono">CH{{ String(index + 1).padStart(2, '0') }}</span>
      <div class="card__actions">
        <!-- 颜色选择按钮 -->
        <button
          class="card__color-btn"
          :style="{ background: color }"
          title="选择波形颜色"
          @click="openColorPicker"
        >
          <input
            :id="`ch-color-${index}`"
            type="color"
            class="card__color-input"
            :value="color"
            @input="onColorChange(($event as InputEvent).target as HTMLInputElement)"
          />
        </button>
      </div>
    </div>

    <!-- 数值显示区 -->
    <div class="card__value-row">
      <span
        class="card__value mono"
        :class="{ 'card__value--flash': isFlashing }"
      >{{ displayValue }}</span>
      <span class="card__unit">{{ unit }}</span>
    </div>
  </article>
</template>

<style scoped>
.card {
  position: relative;
  background: var(--card-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0.45rem 0.6rem 0.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
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
.card--active {
  border-color: var(--ch-color, var(--accent));
  box-shadow:
    0 0 0 1px var(--ch-color, var(--accent)),
    0 6px 18px color-mix(in srgb, var(--ch-color, var(--accent)) 20%, transparent);
}

.card--active::before {
  opacity: 0.06;
}

.card--active:hover {
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
  justify-content: space-between;
  gap: 0.35rem;
  min-width: 0;
}

.card__tag {
  font-size: 0.55rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.08em;
  background: var(--btn-bg);
  padding: 0.1rem 0.3rem;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}

/* 操作按钮组 */
.card__actions {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  flex-shrink: 0;
}

/* 颜色选择按钮 */
.card__color-btn {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: 2px solid var(--border-default);
  cursor: pointer;
  position: relative;
  overflow: hidden;
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.card__color-btn:hover {
  border-color: var(--text-primary);
  transform: scale(1.15);
  box-shadow: 0 0 8px rgba(255, 255, 255, 0.15);
}

/* 隐藏原生颜色输入框 */
.card__color-input {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  opacity: 0;
  cursor: pointer;
  border: none;
  padding: 0;
}

/* 数值显示 */
.card__value-row {
  display: flex;
  align-items: baseline;
  gap: 0.3rem;
  position: relative;
  z-index: 1;
  min-height: 1.4rem;
}

.card__value {
  font-size: 1.15rem;
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

.card--active .card__value {
  color: var(--ch-color, var(--accent));
}

.card__unit {
  font-size: 0.6rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.04em;
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
