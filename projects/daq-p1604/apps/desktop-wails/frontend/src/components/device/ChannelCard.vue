<script setup lang="ts">
import { computed } from 'vue'
import { Palette } from '@lucide/vue'
import { useI18nStore } from '@stores/i18nStore'

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

const i18n = useI18nStore()

function formatValue(v: number): string {
  if (v === undefined || v === null || isNaN(v)) return '---'
  const p = typeof props.precision === 'number' ? Math.max(0, Math.min(props.precision, 6)) : 2
  return v.toFixed(p)
}

// 数值直接派生显示文本：采集期间值持续变化，不再触发闪烁动画，避免视觉噪音
const displayValue = computed(() => formatValue(props.value))

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
    <!-- 头部：彩色标识点 + 通道标识 + 通道名 + 颜色选择按钮 -->
    <div class="card__head">
      <div class="card__head-left">
        <!-- 彩色小圆点：标识通道对应波形颜色，替代粗边框降低视觉噪音 -->
        <span class="card__dot" :style="{ backgroundColor: color }" />
        <span class="card__tag mono">CH{{ String(index + 1).padStart(2, '0') }}</span>
        <!-- 通道名：用户自定义名称，未设置时隐藏 -->
        <span v-if="name" class="card__name" :title="name">{{ name }}</span>
      </div>
      <div class="card__actions">
        <!-- 颜色选择按钮：使用图标替代彩色圆点，降低视觉噪音 -->
        <button
          class="card__color-btn"
          :title="i18n.t('channel.pickColor')"
          @click="openColorPicker"
        >
          <Palette class="card__color-icon" :style="{ color: color }" />
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
      <span class="card__value mono">{{ displayValue }}</span>
      <span class="card__unit">{{ unit }}</span>
    </div>
  </article>
</template>

<style scoped>
.card {
  position: relative;
  /* 使用极浅背景色替代边框来区分卡片，视觉上更干净柔和 */
  background: var(--card-bg-elevated, var(--card-bg));
  border: none;
  border-radius: var(--radius-md);
  padding: 0.45rem 0.6rem 0.4rem;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  transition: all var(--motion-fast) var(--easing-standard);
  overflow: hidden;
}

.card:hover {
  /* 悬浮时轻微提亮背景并添加柔和阴影，替代边框变化 */
  background: var(--card-bg-hover);
  transform: translateY(-1px);
  box-shadow: var(--shadow-sm);
}

/* 选中状态：使用柔和阴影替代边框，保持无边框设计 */
.card--active {
  background: var(--card-bg-hover);
  box-shadow: 0 0 0 1.5px rgba(var(--accent-rgb, 16, 185, 129), 0.35), var(--shadow-sm);
}

.card--active:hover {
  box-shadow: 0 0 0 1.5px rgba(var(--accent-rgb, 16, 185, 129), 0.45), var(--shadow-sm);
}

/* 头部布局 */
.card__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.35rem;
  min-width: 0;
}

.card__head-left {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  min-width: 0;
}

/* 彩色小圆点：标识通道对应波形颜色 */
.card__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.card__tag {
  font-size: 0.55rem;
  font-weight: 700;
  /* 标签文字跟随通道颜色，让卡片与波形颜色快速对应 */
  color: var(--ch-color);
  letter-spacing: 0.08em;
  /* 背景使用通道颜色的低透明度混合，保持可读性同时强化识别 */
  background: color-mix(in srgb, var(--ch-color) 12%, transparent);
  padding: 0.1rem 0.3rem;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}

/* 通道名：用户自定义名称 */
.card__name {
  font-size: 0.62rem;
  font-weight: 600;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
  max-width: 5rem;
}

/* 操作按钮组 */
.card__actions {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  flex-shrink: 0;
}

/* 颜色选择按钮：使用图标样式替代彩色圆点 */
.card__color-btn {
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  border: none;
  background: transparent;
  cursor: pointer;
  position: relative;
  overflow: hidden;
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.card__color-btn:hover {
  background: var(--btn-bg);
}

.card__color-icon {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
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
  /* 数值始终跟随通道色：与圆点/标签/波形保持一致的识别线索，避免闪烁干扰 */
  color: var(--ch-color);
  letter-spacing: -0.02em;
  line-height: 1;
  font-variant-numeric: tabular-nums;
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
  .card__value {
    transition: none;
  }

  .card:hover {
    transform: none;
  }
}
</style>
