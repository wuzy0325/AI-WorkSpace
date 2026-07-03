<script setup lang="ts">
import { Eye, EyeOff, Minus } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import { useI18nStore } from '@stores/i18nStore'

/** 单个通道卡片的预计算数据，由 DeviceDetailPanel 的 channelCards computed 产出。 */
export interface ChannelCardData {
  index: number
  rawValue: number
  formattedValue: string
  unit: string
  tone: 'active' | 'warning'
  isChartVisible: boolean
  style: Record<string, string>
  color: string
  showTareBadge: boolean
  disableTare: boolean
  sparkBars: number[]
  range: { min: number; max: number }
}

const props = defineProps<{
  card: ChannelCardData
  compact: boolean
}>()

const emit = defineEmits<{
  (e: 'toggleChart', index: number): void
  (e: 'tare', index: number, rawValue: number): void
}>()

const i18n = useI18nStore()

function onToggleChart(): void {
  emit('toggleChart', props.card.index)
}
function onTare(): void {
  emit('tare', props.card.index, props.card.rawValue)
}
</script>

<template>
  <article
    class="channel-card"
    :class="{
      'channel-card--warning': !compact && card.tone === 'warning',
      'channel-card--selected': card.isChartVisible,
      'channel-card--compact': compact
    }"
    :style="card.style"
    :title="compact ? `CH_${String(card.index + 1).padStart(2, '0')} · ${card.formattedValue} ${card.unit}` : undefined"
  >
    <!-- 混合模式：极简单行卡片，仅保留通道名 + 数值 + 单位，
         最大化把纵向空间让给实时波形图。
         注：混合模式不使用黄色告警配色（用户偏好），仅在卡片模式保留告警高亮
         字体：CH_XX / 数值 均使用 Inter（--font-family-data），对齐 Cursor DAQ 混合画面；
              数值额外叠加 tabular-nums + tracking-tight，让数字紧凑且纵向对齐。 -->
    <template v-if="compact">
      <span class="channel-card__compact-tag">CH_{{ String(card.index + 1).padStart(2, '0') }}</span>
      <span class="channel-card__compact-value">{{ card.formattedValue }}</span>
      <span class="channel-card__compact-unit">{{ card.unit }}</span>
    </template>

    <!-- 卡片模式：完整卡片，含 ID、操作按钮、sparkline、量程 -->
    <template v-else>
      <div class="channel-card__top">
        <div class="channel-card__top-left">
          <div
            v-if="card.showTareBadge"
            class="channel-card__tare-badge"
            :title="i18n.t.tareOffsetApplied || '已应用归零偏移'"
          />
          <span class="channel-card__tag mono-font">CH_{{ String(card.index + 1).padStart(2, '0') }}</span>
        </div>
        <div class="channel-card__id">
          <span class="channel-card__dot" :style="{ background: card.color }" />
          <span class="channel-card__id-text mono-font">CH{{ card.index + 1 }}</span>
        </div>
        <div class="channel-card__actions">
          <UiButton
            variant="ghost"
            size="sm"
            :class="{ 'channel-card__action-btn--active': card.isChartVisible }"
            :aria-label="card.isChartVisible ? '隐藏波形' : '显示波形'"
            @click.stop="onToggleChart"
          >
            <template #icon>
              <Eye v-if="card.isChartVisible" class="channel-card__icon" />
              <EyeOff v-else class="channel-card__icon" />
            </template>
          </UiButton>
          <UiButton
            variant="ghost"
            size="sm"
            :class="{ 'channel-card__action-btn--disabled': card.disableTare }"
            :aria-label="card.disableTare ? '此通道不支持校零' : '归零'"
            :disabled="card.disableTare"
            @click.stop="onTare"
          >
            <template #icon>
              <Minus class="channel-card__icon" />
            </template>
          </UiButton>
        </div>
      </div>
      <div class="channel-card__value-area">
        <div class="channel-card__value-row">
          <span
            class="channel-card__value mono-font"
            :class="{ 'text-amber-500': card.tone === 'warning' }"
          >
            {{ card.formattedValue }}
          </span>
          <span class="channel-card__unit">{{ card.unit }}</span>
        </div>
      </div>
      <div class="channel-card__sparkline">
        <span
          v-for="(h, i) in card.sparkBars"
          :key="i"
          class="channel-card__spark"
          :class="{ 'channel-card__spark--active': i === card.sparkBars.length - 1 }"
          :style="{ height: `${h}%`, background: i === card.sparkBars.length - 1 ? card.color : undefined }"
        />
      </div>
      <div class="channel-card__range mono-font">
        <span>MIN: {{ card.range.min }}</span>
        <span>MAX: {{ card.range.max }}</span>
      </div>
    </template>
  </article>
</template>

<style scoped>
.channel-card {
  background: rgba(30, 41, 59, 0.4);
  backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 1em;
  padding: 0.75em;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  position: relative;
  overflow: hidden;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  min-height: 0;
  font-size: clamp(10px, 2cqw, 20px);
  container-type: inline-size;
  container-name: channel-card;
}

:root[data-theme='light'] .channel-card {
  background: rgba(255, 255, 255, 0.85);
  border: 1px solid rgba(0, 0, 0, 0.12);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

/* 仅在警告状态的通道卡片上显示顶部指示线，保持正常卡片简洁 */
.channel-card--warning::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 0.3rem;
  background: linear-gradient(90deg, transparent, var(--accent-warning), transparent);
  opacity: 0.7;
}

.channel-card:hover {
  transform: translateY(-2px);
  border-color: var(--theme-color, var(--accent-primary));
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.1);
}

.channel-card--selected {
  box-shadow: 0 0 0 1px var(--theme-color, var(--accent-primary)), 0 10px 25px rgba(0, 0, 0, 0.1);
}

.channel-card--warning {
  border-color: rgba(245, 158, 11, 0.3);
}

.channel-card__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5em;
  min-height: 1.5em;
}

.channel-card__top-left {
  display: flex;
  align-items: center;
  gap: 0.375em;
  flex: 0 0 auto;
  min-width: 0;
}

.channel-card__tag,
.channel-card__id-text {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-secondary);
  letter-spacing: 0.1em;
  white-space: nowrap;
}

.channel-card__id {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.375em;
  flex: 1 1 auto;
}

.channel-card__dot {
  width: 0.5em;
  height: 0.5em;
  border-radius: 50%;
  flex-shrink: 0;
}

.channel-card__actions {
  display: flex;
  align-items: center;
  gap: 0.25em;
  flex: 0 0 auto;
}

.channel-card__action-btn {
  width: 1.5em;
  height: 1.5em;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25em;
  color: var(--text-muted);
  background: rgba(0, 0, 0, 0.2);
  transition: all 0.2s ease;
}

:root[data-theme='light'] .channel-card__action-btn {
  background: rgba(0, 0, 0, 0.06);
}

.channel-card__action-btn:hover {
  color: var(--theme-color, var(--accent-primary));
  background: rgba(16, 185, 129, 0.15);
}

.channel-card__action-btn--active {
  color: var(--theme-color, var(--accent-primary));
  background: var(--theme-color-soft, rgba(16, 185, 129, 0.15));
}

.channel-card__action-btn--disabled {
  opacity: 0.3;
  cursor: not-allowed;
  pointer-events: none;
}

.channel-card__icon {
  width: 0.875em;
  height: 0.875em;
  flex-shrink: 0;
}

.channel-card__tare-badge {
  width: 0.5em;
  height: 0.5em;
  border-radius: 50%;
  background: var(--accent-warning);
  flex-shrink: 0;
}

.channel-card__value-area {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-height: 2em;
  max-height: 2.5em;
  padding: 0.125em 0;
}

.channel-card__value-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5em;
}

.channel-card__value {
  font-size: 2.4em;
  font-weight: var(--font-weight-black);
  letter-spacing: -0.02em;
  line-height: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--text-primary, var(--border-default));
}

.channel-card__value.text-amber-500 {
  color: var(--accent-warning);
}

.channel-card__unit {
  font-size: var(--font-size-micro);
  font-weight: var(--font-weight-black);
  color: var(--text-muted);
  font-style: italic;
  letter-spacing: 0.02em;
}

.channel-card__sparkline {
  display: flex;
  align-items: flex-end;
  gap: clamp(1px, 0.4cqw, 3px);
  height: clamp(24px, 6cqw, 40px);
  padding: 0 clamp(2px, 1cqw, 6px);
  flex-shrink: 0;
}

.channel-card__spark {
  flex: 1;
  min-height: 2px;
  background: rgba(255, 255, 255, 0.1);
  border-radius: 1px 1px 0 0;
}

:root[data-theme='light'] .channel-card__spark {
  background: rgba(0, 0, 0, 0.1);
}

.channel-card__spark--active {
  /* 使用纯色高亮代替发光效果 */
  opacity: 1;
}

.channel-card__range {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-secondary);
  margin-top: 0.25em;
}

/* ===== 混合模式极简单行卡片 =====
   仅展示通道名 + 数值 + 单位，单行内联布局，
   行高压到 40px，最大化把纵向空间让给上方波形图。
   字体：混合模式整张卡片统一使用 Inter（--font-family-data），对齐 Cursor DAQ 混合画面。
        离线时按 --font-family-data 回退到 PingFang SC / Microsoft YaHei UI。 */
.channel-card--compact {
  font-family: var(--font-family-data);
  padding: 0 var(--space-2);
  gap: var(--space-2);
  border-radius: var(--radius-sm);
  font-size: 13px;
  container-type: normal;
  flex-direction: row;
  align-items: center;
  justify-content: flex-start;
}

.channel-card--compact:hover {
  transform: none;
  border-color: var(--theme-color, var(--accent-primary));
  box-shadow: none;
}

.channel-card--compact.channel-card--selected {
  box-shadow: 0 0 0 1px var(--theme-color, var(--accent-primary)) inset;
}

/* 混合模式不再使用黄色告警边框（用户偏好），保留占位便于未来复用 */

.channel-card__compact-tag {
  /* 通道名标签使用通道主题色 + 半透明背景徽章，
     与数值形成明显的色相对比（主题色 vs 主文本色），
     避免二者同为灰色导致视觉粘连 */
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--theme-color, var(--accent-primary));
  background: var(--theme-color-soft, rgba(59, 130, 246, 0.14));
  border: 1px solid var(--theme-color-border, rgba(59, 130, 246, 0.28));
  padding: 2px 6px;
  border-radius: var(--radius-xs, 4px);
  letter-spacing: 0.05em;
  white-space: nowrap;
  flex-shrink: 0;
}

.channel-card__compact-value {
  /* 数值使用 Inter（--font-family-data） + tabular-nums + tracking-tight，
     对齐 Cursor DAQ 混合画面数值字体观感：数字紧凑、纵向位对齐、西文 sans 工业感。
     离线或字体未加载时按 --font-family-data 回退到 PingFang SC / Microsoft YaHei UI。 */
  font-family: var(--font-family-data);
  font-size: var(--font-size-base, 14px);
  font-weight: var(--font-weight-black, 800);
  color: var(--text-primary);
  line-height: 1;
  letter-spacing: -0.025em; /* Tailwind tracking-tight 等价值 */
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1 1 auto;
  min-width: 0;
  text-align: right;
}

.channel-card__compact-unit {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--text-muted);
  white-space: nowrap;
  flex-shrink: 0;
}
</style>
