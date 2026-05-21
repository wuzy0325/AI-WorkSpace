<script setup lang="ts">
import { computed } from 'vue'
import { Activity, Settings2, Eye, Minus } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import RealtimeChart from '@components/device/RealtimeChart.vue'

const props = withDefaults(
  defineProps<{
    mode?: 'chart' | 'table' | 'both'
  }>(),
  { mode: 'both' },
)

const deviceStore = useDeviceStore()
const i18n = useI18nStore()

const CHANNEL_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#a855f7', '#f43f5e', '#06b6d4', '#f97316', '#6366f1']

const profile = computed(() => deviceStore.selectedProfile)
const snapshot = computed(() => deviceStore.selectedSnapshot)
const showChart = computed(() => props.mode === 'chart' || props.mode === 'both')
const showTable = computed(() => props.mode === 'table' || props.mode === 'both')

function channelColor(index: number): string {
  return CHANNEL_COLORS[index % CHANNEL_COLORS.length]
}

function channelStyle(index: number): Record<string, string> {
  const c = channelColor(index)
  return {
    '--theme-color': c,
    '--theme-color-soft': c + '33',
    '--theme-color-border': c + '55',
  }
}

function formatChannel(ch: number, raw: number): string {
  return deviceStore.formatValue(deviceStore.selectedDeviceId ?? '', ch, raw)
}

function sparkBars(channelIndex: number): number[] {
  const history = deviceStore.historyFor(deviceStore.selectedDeviceId ?? '')
  const values = history
    .map((entry) => {
      const pos = entry.channelIndices.indexOf(channelIndex)
      return pos >= 0 ? entry.channels[pos] : null
    })
    .filter((v): v is number => v !== null)
    .slice(-12)
  if (!values.length) {
    return Array.from({ length: 12 }, () => 15 + ((channelIndex * 37 + 20) % 60))
  }
  const mn = Math.min(...values)
  const mx = Math.max(...values)
  const span = mx - mn || 1
  return values.map((v) => 15 + ((v - mn) / span) * 75)
}
</script>

<template>
  <div class="detail-panel">
    <div class="detail-panel__head">
      <div>
        <p class="eyebrow">
          {{ profile?.type ?? 'SIMULATED' }}
          / {{ profile?.samplingRate ?? 20 }} Hz
        </p>
        <h2>{{ profile?.name ?? 'Wind-DAQ' }}</h2>
        <p class="detail-panel__desc">{{ i18n.t.waitingData }}</p>
      </div>
      <div class="detail-panel__actions">
        <slot name="actions" />
      </div>
    </div>

    <div v-if="showChart" class="detail-panel__chart">
      <div class="detail-panel__chart-head">
        <div class="detail-panel__chart-title">
          <Activity class="w-4 h-4 text-emerald-500" />
          <span>实时趋势</span>
        </div>
        <div class="detail-panel__chart-controls">
          <button class="detail-panel__chart-btn">
            <Settings2 class="w-3.5 h-3.5" />
            <span>通道选择</span>
          </button>
          <span class="detail-panel__chart-buffer">缓冲区 100 点</span>
        </div>
      </div>
      <RealtimeChart :device-id="deviceStore.selectedDeviceId ?? ''" />
    </div>

    <div v-if="showTable" class="channel-grid">
      <article
        v-for="channel in (profile?.channels ?? [])"
        :key="channel.index"
        class="channel-card"
        :style="channelStyle(channel.index)"
      >
        <div class="channel-card__top">
          <div class="channel-card__id">
            <span class="channel-card__dot" :style="{ background: channelColor(channel.index), boxShadow: `0 0 8px ${channelColor(channel.index)}` }" />
            <span class="channel-card__tag">CH_{{ String(channel.index + 1).padStart(2, '0') }}</span>
          </div>
          <div class="channel-card__actions">
            <button class="channel-card__action-btn" title="Show/Hide waveform">
              <Eye class="w-3.5 h-3.5" />
            </button>
            <button class="channel-card__action-btn" title="Tare">
              <Minus class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
        <div class="channel-card__value">
          <strong>{{ formatChannel(channel.index, snapshot?.channels[channel.index] ?? 0) }}</strong>
          <small>{{ channel.unit }}</small>
        </div>
        <div class="channel-card__meta">
          <div class="channel-card__spark">
            <span
              v-for="(h, i) in sparkBars(channel.index)"
              :key="i"
              :style="{ height: `${h}%`, background: channelColor(channel.index) }"
            />
          </div>
          <div class="channel-card__range">
            <span>MIN: {{ channel.rangeMin ?? -10 }}</span>
            <span>MAX: {{ channel.rangeMax ?? 10 }}</span>
          </div>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.detail-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: var(--space-4);
}

.detail-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  flex-shrink: 0;
  padding: 0 var(--space-2);
}

.eyebrow {
  margin: 0 0 0.3rem;
  color: #64748b;
  font-size: 0.65rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.detail-panel__head h2 {
  margin: 0;
  color: #e2e8f0;
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-black);
  letter-spacing: -0.02em;
}

:root[data-theme='light'] .detail-panel__head h2 {
  color: #0f172a;
}

.detail-panel__desc {
  margin: 0.25rem 0 0;
  color: #94a3b8;
  font-size: 0.85rem;
}

.detail-panel__actions {
  display: flex;
  gap: 0.6rem;
}

.channel-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  grid-auto-rows: minmax(160px, 1fr);
  gap: var(--space-3);
  min-height: 0;
  overflow-y: auto;
}

.channel-card {
  min-height: 0;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 0.75rem;
  border-radius: 1rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(30, 41, 59, 0.4);
  backdrop-filter: blur(8px);
  position: relative;
  overflow: hidden;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

:root[data-theme='light'] .channel-card {
  background: rgba(255, 255, 255, 0.85);
  border: 1px solid rgba(0, 0, 0, 0.12);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.channel-card:hover {
  border-color: var(--theme-color, var(--accent-success));
  transform: translateY(-2px);
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.1);
}

.channel-card::before {
  content: '';
  position: absolute;
  inset: 0 0 auto;
  height: 3px;
  background: linear-gradient(90deg, transparent, var(--theme-color, var(--accent-success)), transparent);
  opacity: 0.6;
}

.channel-card__top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.channel-card__id {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.channel-card__tag {
  color: var(--text-secondary, #64748b);
  font: 800 0.72rem/1 var(--font-family-mono, monospace);
}

.channel-card__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
}

.channel-card__actions {
  display: flex;
  gap: 0.3rem;
  opacity: 0;
  transition: opacity 0.2s ease;
}

.channel-card:hover .channel-card__actions {
  opacity: 1;
}

.channel-card__action-btn {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.3rem;
  color: #64748b;
  background: rgba(255, 255, 255, 0.05);
  transition: all 0.2s ease;
}

.channel-card__action-btn:hover {
  color: var(--theme-color, var(--accent-success));
  background: color-mix(in srgb, var(--theme-color, var(--accent-success)) 15%, transparent);
}

.channel-card__value {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
  margin-bottom: 0.75rem;
}

.channel-card__value strong {
  color: var(--text-primary);
  font: 900 2.3rem/1 var(--font-family-mono, monospace);
  text-shadow: none;
}

:root[data-theme='dark'] .channel-card__value strong {
  text-shadow: 0 0 10px currentColor;
}

.channel-card__value small {
  color: #64748b;
  font-weight: 800;
  font-size: 0.85rem;
}

.channel-card__meta {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: auto;
}

.channel-card__spark {
  height: 30px;
  display: flex;
  align-items: end;
  gap: 0.2rem;
}

.channel-card__spark span {
  flex: 1;
  border-radius: 999px 999px 0 0;
  opacity: 0.35;
}

:root[data-theme='light'] .channel-card__spark span {
  opacity: 0.55;
}

.channel-card__spark span:last-child {
  opacity: 1;
}

.channel-card__range {
  display: flex;
  justify-content: space-between;
  color: #64748b;
  font: 700 0.62rem/1 var(--font-family-mono, monospace);
}

.detail-panel__chart {
  flex: 1;
  min-height: 280px;
  margin: 0;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: var(--radius-xl, 1rem);
  background: rgba(30, 41, 59, 0.4);
  backdrop-filter: blur(8px);
  padding: var(--space-4);
  overflow: hidden;
}

:root[data-theme='light'] .detail-panel__chart {
  background: rgba(255, 255, 255, 0.6);
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.detail-panel__chart-head {
  min-height: 32px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-3);
}

.detail-panel__chart-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
  font-weight: 700;
  color: #e2e8f0;
}

:root[data-theme='light'] .detail-panel__chart-title {
  color: #0f172a;
}

.detail-panel__chart-controls {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.detail-panel__chart-btn {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.25rem 0.5rem;
  border-radius: 0.3rem;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.25);
  color: #10b981;
  font-size: 0.7rem;
  font-weight: 700;
  transition: all 0.2s ease;
}

.detail-panel__chart-btn:hover {
  background: rgba(16, 185, 129, 0.2);
  border-color: rgba(16, 185, 129, 0.4);
  transform: translateY(-1px);
}

.detail-panel__chart-buffer {
  color: #64748b;
  font: 700 0.7rem/1 var(--font-family-mono, monospace);
}
</style>
