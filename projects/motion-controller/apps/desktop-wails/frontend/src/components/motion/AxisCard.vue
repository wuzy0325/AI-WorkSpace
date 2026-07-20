<script setup lang="ts">
// AxisCard — 单轴控制卡片
// 从 MotionControlPanel 抽出。集中轴卡片的视图派生（computed）与事件转发，
// 避免父组件模板内调用方法（违反 state#§12）。

import { computed } from 'vue'
import type { AxisName, MotionControllerProfile, AxisStatus } from '@shared/types/motion'
import { getAxisThemeClass } from './motionConfigEditor'
import {
  getAxisUnit,
  axisReadout,
  axisMinLabel,
  axisMaxLabel,
  historyBarStyle,
  axisLimitWarningHint,
  getLimitWarningClass,
} from '@composables/useAxisCard'
import { useI18nStore } from '@stores/i18nStore'

const props = defineProps<{
  /** 控制器 ID */
  controllerId: string
  /** 轴状态 */
  axis: AxisStatus
  /** 控制器配置（用于查限位/类型） */
  profile?: MotionControllerProfile
  /** 是否已连接（影响显示与禁用状态） */
  connected: boolean
  /** 当前零点偏移 */
  zeroOffset: number
  /** 当前目标位置（双向绑定） */
  targetPosition: number
  /** 当前步长（双向绑定） */
  step: number
  /** 历史位置数组（用于历史条带渐变） */
  history: number[]
}>()

const emit = defineEmits<{
  (e: 'update:targetPosition', value: number): void
  (e: 'update:step', value: number): void
  (e: 'move', axis: AxisName): void
  (e: 'jog', axis: AxisName, direction: 'forward' | 'reverse'): void
  (e: 'stop', axis: AxisName): void
  (e: 'set-zero', axis: AxisName): void
}>()

const i18n = useI18nStore()

// 派生视图（一次性 computed，模板不再调用方法）
const axisName = computed(() => props.axis.name as AxisName)
const unit = computed(() => getAxisUnit(axisName.value, props.profile))
const readout = computed(() =>
  props.connected ? axisReadout(axisName.value, props.axis.position, props.zeroOffset) : '--'
)
const minLabel = computed(() => axisMinLabel(axisName.value, props.profile))
const maxLabel = computed(() => axisMaxLabel(axisName.value, props.profile))
const historyStyle = computed(() => historyBarStyle(props.history))
const absoluteTarget = computed(() => props.zeroOffset + props.targetPosition)
const limitHint = computed(() => axisLimitWarningHint(axisName.value, absoluteTarget.value, props.profile))
const limitClass = computed(() => getLimitWarningClass(axisName.value, absoluteTarget.value, props.profile))

// v-model 双向绑定转发
const targetModel = computed({
  get: () => props.targetPosition,
  set: (v: number) => emit('update:targetPosition', v),
})

const stepModel = computed({
  get: () => props.step,
  set: (v: number) => emit('update:step', v),
})

const disabled = computed(() => !props.connected)
const moving = computed(() => props.axis.moving)
const jogDisabled = computed(() => moving.value || disabled.value)
const moveDisabled = computed(() => moving.value || disabled.value)
const zeroDisabled = computed(() => moving.value || disabled.value)
const stopDisabled = computed(() => disabled.value)

const themeClass = computed(() => getAxisThemeClass(axisName.value))
</script>

<template>
  <div
    class="axis-card"
    :class="[themeClass, { 'axis-card--disabled': disabled }]"
  >
    <!-- 卡片头部 -->
    <div class="axis-card__head">
      <div class="axis-card__badge">{{ axis.name }}</div>
      <div class="axis-card__head-info">
        <span class="axis-card__head-label">{{ i18n.t.axisNode }}</span>
        <span class="axis-card__head-value">{{ i18n.t.axisMotionControl }}</span>
      </div>
      <div class="axis-card__head-status">
        <span
          class="axis-card__status-dot"
          :class="moving ? 'moving' : 'idle'"
          aria-hidden="true"
        ></span>
        <span class="axis-card__status-text">{{ moving ? i18n.t.moving : i18n.t.idle }}</span>
      </div>
      <div class="axis-card__unit">{{ unit }}</div>
    </div>

    <!-- 位置读数（核心数据） -->
    <div class="axis-card__readout">
      <span class="axis-card__readout-val">{{ readout }}</span>
      <span class="axis-card__readout-u">{{ unit }}</span>
    </div>

    <!-- 限位条 -->
    <div class="axis-card__limits">
      <div class="axis-card__lim axis-card__lim--left">
        <span
          class="axis-card__lim-end"
          :class="{ 'axis-card__lim-end--active': axis.negLimit }"
          aria-hidden="true"
        ></span>
        <span class="axis-card__lim-lab">{{ i18n.t.negLimit }}</span>
        <span class="axis-card__lim-val">{{ minLabel }}</span>
      </div>
      <div class="axis-card__lim axis-card__lim--right">
        <span class="axis-card__lim-val">{{ maxLabel }}</span>
        <span class="axis-card__lim-lab">{{ i18n.t.posLimit }}</span>
        <span
          class="axis-card__lim-end"
          :class="{ 'axis-card__lim-end--active': axis.posLimit }"
          aria-hidden="true"
        ></span>
      </div>
    </div>

    <!-- 历史条带 -->
    <div class="axis-card__history" :style="historyStyle"></div>

    <!-- 步进 -->
    <div class="axis-card__section-label">{{ i18n.t.jog }}</div>
    <div class="axis-card__jog">
      <button
        class="axis-card__jog-btn"
        @click="emit('jog', axisName, 'reverse')"
        :disabled="jogDisabled"
        :aria-label="i18n.t.jogReverse"
        title="−"
      >−</button>
      <input
        v-model.number="stepModel"
        type="number"
        class="axis-card__jog-in"
        :disabled="jogDisabled"
        min="0"
        step="0.1"
        :aria-label="i18n.t.jogStep"
      />
      <button
        class="axis-card__jog-btn"
        @click="emit('jog', axisName, 'forward')"
        :disabled="jogDisabled"
        :aria-label="i18n.t.jogForward"
        title="+"
      >+</button>
    </div>

    <!-- 定位 -->
    <div class="axis-card__section-label">{{ i18n.t.move }}</div>
    <div class="axis-card__move">
      <input
        v-model.number="targetModel"
        type="number"
        class="axis-card__move-in"
        :class="limitClass"
        :disabled="moveDisabled"
        placeholder="0.00"
        :aria-label="i18n.t.currentPosition"
      />
      <button
        class="axis-card__go"
        @click="emit('move', axisName)"
        :disabled="moveDisabled"
      >GO</button>
    </div>

    <!-- 限位提示 -->
    <div
      v-if="limitHint.text"
      class="axis-card__hint"
      :class="'axis-card__hint--' + limitHint.cls"
      role="status"
    >
      {{ limitHint.text }}
    </div>
    <div v-else class="axis-card__hint axis-card__hint--placeholder" aria-hidden="true"></div>

    <!-- 卡片底部：置零 / 停止 -->
    <div class="axis-card__foot">
      <button
        class="axis-card__foot-btn"
        @click="emit('set-zero', axisName)"
        :disabled="zeroDisabled"
      >{{ i18n.t.setZero }}</button>
      <button
        class="axis-card__foot-btn axis-card__foot-btn--stop"
        @click="emit('stop', axisName)"
        :disabled="stopDisabled"
        :aria-label="i18n.t.stopAxis"
      >{{ i18n.t.stop }}</button>
    </div>
  </div>
</template>

<style scoped>
/* ============================================================
   轴主题色变量
   ============================================================ */
.axis-card.axis-x-theme { --ax: var(--axis-x); --ax-soft: var(--axis-x-soft); }
.axis-card.axis-y-theme { --ax: var(--axis-y); --ax-soft: var(--axis-y-soft); }
.axis-card.axis-z-theme { --ax: var(--axis-z); --ax-soft: var(--axis-z-soft); }
.axis-card.axis-u-theme { --ax: var(--axis-u); --ax-soft: var(--axis-u-soft); }

/* ============================================================
   轴卡片容器
   ============================================================ */
.axis-card {
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 6px;
  /* 增大内边距，让大屏下卡片内部更舒展 */
  padding: 18px;
  position: relative;
  transition: border-color 0.15s ease, box-shadow 0.2s ease;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

/* 左侧轴色竖条 */
.axis-card::before {
  content: '';
  position: absolute;
  left: 0;
  top: 18px;
  bottom: 18px;
  width: 3px;
  border-radius: 2px;
  background: var(--ax, var(--accent-primary));
  opacity: 0.9;
}

.axis-card:hover {
  border-color: var(--ax, var(--accent-primary));
  box-shadow: 0 0 0 1px var(--ax, var(--accent-primary)),
              0 8px 24px -12px var(--ax, var(--accent-primary));
}

.axis-card--disabled {
  opacity: 0.55;
  filter: grayscale(0.3);
}

/* ============================================================
   卡片头部
   ============================================================ */
.axis-card__head {
  display: flex;
  align-items: center;
  gap: 9px;
  margin-bottom: 14px;
  padding-left: 8px;
}

.axis-card__badge {
  width: 30px;
  height: 30px;
  border-radius: 4px;
  display: grid;
  place-items: center;
  font-weight: 800;
  font-size: 15px;
  color: var(--color-on-axis-badge);
  background: var(--ax, var(--accent-primary));
  flex-shrink: 0;
}

.axis-card__head-info {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  flex: 1;
}

.axis-card__head-label {
  font-size: 9px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}

.axis-card__head-value {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.axis-card__head-status {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
}

.axis-card__status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.axis-card__status-dot.idle {
  background: var(--text-muted);
  opacity: 0.5;
}

.axis-card__status-dot.moving {
  background: var(--accent-success);
  box-shadow: 0 0 6px var(--accent-success);
  animation: axis-card-pulse 1.5s ease-in-out infinite;
}

@keyframes axis-card-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.axis-card__status-text {
  font-size: 10px;
  font-weight: 700;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.axis-card__unit {
  margin-left: auto;
  font-size: 11px;
  color: var(--text-muted);
  border: 1px solid var(--border-default);
  border-radius: 3px;
  padding: 2px 7px;
  flex-shrink: 0;
}

/* ============================================================
   位置读数
   ============================================================ */
.axis-card__readout {
  font-family: 'SF Mono', ui-monospace, Menlo, Consolas, 'Courier New', monospace;
  font-variant-numeric: tabular-nums;
  font-size: 36px;
  font-weight: 600;
  line-height: 1;
  text-align: center;
  /* 增大读数区上下呼吸空间 */
  padding: 14px 0 16px;
  color: var(--text-primary);
  letter-spacing: -1px;
}

.axis-card__readout-u {
  font-size: 14px;
  color: var(--text-muted);
  font-weight: 500;
  margin-left: 4px;
  letter-spacing: 0;
}

@media (max-width: 1280px) {
  .axis-card__readout {
    font-size: 30px;
  }
}

/* ============================================================
   限位条
   ============================================================ */
.axis-card__limits {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 0 4px 10px;
  font-size: 11px;
}

.axis-card__lim {
  display: flex;
  align-items: center;
  gap: 5px;
}

.axis-card__lim-lab {
  font-size: 9px;
  letter-spacing: 0.5px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: 700;
}

.axis-card__lim-val {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  color: var(--text-secondary);
  font-weight: 600;
  font-size: 10.5px;
}

.axis-card__lim-end {
  width: 6px;
  height: 6px;
  border-radius: 1px;
  background: color-mix(in srgb, var(--accent-danger) 35%, transparent);
  transition: all 0.15s ease;
}

.axis-card__lim-end--active {
  background: var(--accent-danger);
  box-shadow: 0 0 6px var(--accent-danger);
}

/* ============================================================
   历史条带
   ============================================================ */
.axis-card__history {
  height: 3px;
  border-radius: 2px;
  margin: 0 4px 16px;
  opacity: 0.7;
  background: var(--bg-panel-strong);
}

/* ============================================================
   小节标签
   ============================================================ */
.axis-card__section-label {
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--text-muted);
  font-weight: 700;
  margin-bottom: 6px;
  padding-left: 4px;
}

/* ============================================================
   步进控制
   ============================================================ */
.axis-card__jog {
  display: flex;
  gap: 6px;
  margin-bottom: 16px;
}

.axis-card__jog-btn {
  flex: 1 1 44px;
  min-width: 40px;
  height: 38px;
  border-radius: 4px;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  color: var(--text-primary);
  font-size: 22px;
  font-weight: 800;
  cursor: pointer;
  transition: all 0.12s ease;
  display: grid;
  place-items: center;
  line-height: 1;
}

.axis-card__jog-btn:hover:not(:disabled) {
  border-color: var(--ax, var(--accent-primary));
  color: var(--ax, var(--accent-primary));
}

.axis-card__jog-btn:active:not(:disabled) {
  transform: scale(0.92);
}

.axis-card__jog-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.axis-card__jog-in {
  flex: 2 1 56px;
  min-width: 0;
  height: 38px;
  border-radius: 4px;
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-size: 14px;
  text-align: center;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  outline: none;
  transition: border-color 0.12s ease;
  -moz-appearance: textfield;
  padding: 0 6px;
}

.axis-card__jog-in::-webkit-outer-spin-button,
.axis-card__jog-in::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

.axis-card__jog-in:focus {
  border-color: var(--ax, var(--accent-primary));
}

.axis-card__jog-in:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* ============================================================
   定位控制
   ============================================================ */
.axis-card__move {
  display: flex;
  gap: 6px;
}

.axis-card__move-in {
  flex: 1;
  height: 38px;
  border-radius: 4px;
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-size: 14px;
  text-align: center;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  outline: none;
  transition: all 0.12s ease;
  padding: 0 6px;
  min-width: 0;
}

.axis-card__move-in:focus {
  border-color: var(--ax, var(--accent-primary));
}

.axis-card__move-in.limit-near {
  border-color: var(--accent-warning);
  background: color-mix(in srgb, var(--accent-warning) 8%, var(--bg-canvas));
}

.axis-card__move-in.limit-exceeded {
  border-color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 10%, var(--bg-canvas));
}

.axis-card__move-in:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.axis-card__go {
  width: 54px;
  height: 38px;
  border-radius: 4px;
  border: 1px solid var(--ax, var(--accent-primary));
  background: var(--ax, var(--accent-primary));
  color: var(--color-on-axis-badge);
  font-weight: 800;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.12s ease;
  flex-shrink: 0;
}

.axis-card__go:hover:not(:disabled) {
  filter: brightness(1.1);
}

.axis-card__go:active:not(:disabled) {
  transform: scale(0.95);
}

.axis-card__go:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* ============================================================
   提示
   ============================================================ */
.axis-card__hint {
  font-size: 10.5px;
  margin-top: 7px;
  padding-left: 4px;
  min-height: 14px;
  font-weight: 600;
  line-height: 1.4;
}

.axis-card__hint--placeholder {
  visibility: hidden;
}

.axis-card__hint--warn {
  color: var(--accent-warning);
}

.axis-card__hint--err {
  color: var(--accent-danger);
}

/* ============================================================
   卡片底部按钮
   ============================================================ */
.axis-card__foot {
  display: flex;
  gap: 6px;
  margin-top: 14px;
}

.axis-card__foot-btn {
  flex: 1;
  height: 32px;
  border-radius: 3px;
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  color: var(--text-muted);
  font-size: 11.5px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.12s ease;
}

.axis-card__foot-btn:hover:not(:disabled) {
  color: var(--text-primary);
  border-color: var(--ax, var(--accent-primary));
}

.axis-card__foot-btn:active:not(:disabled) {
  transform: scale(0.96);
}

.axis-card__foot-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.axis-card__foot-btn--stop:hover:not(:disabled) {
  color: var(--accent-danger);
  border-color: var(--accent-danger);
}

/* ============================================================
   prefers-reduced-motion：禁用轴状态脉冲与所有按压位移
   ============================================================ */
@media (prefers-reduced-motion: reduce) {
  .axis-card__status-dot.moving {
    animation: none;
  }
  .axis-card__jog-btn:active:not(:disabled),
  .axis-card__go:active:not(:disabled),
  .axis-card__foot-btn:active:not(:disabled) {
    transform: none;
  }
  .axis-card,
  .axis-card__jog-btn,
  .axis-card__foot-btn,
  .axis-card__move-in,
  .axis-card__jog-in,
  .axis-card__go {
    transition: none;
  }
}
</style>
