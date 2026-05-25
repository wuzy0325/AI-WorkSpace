<script setup lang="ts">
import { computed, ref } from 'vue'
import { Activity, Settings2, Eye, EyeOff, Minus } from '@lucide/vue'
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
const isMixedMode = computed(() => props.mode === 'both')
const isTableMode = computed(() => props.mode === 'table')

function channelColor(index: number): string {
  return CHANNEL_COLORS[index % CHANNEL_COLORS.length]
}

function channelStyle(index: number): Record<string, string> {
  const c = channelColor(index)
  return {
    '--theme-color': c,
    '--theme-color-soft': c + '33',
    '--theme-color-border': c + '55',
    '--detail-channel-color': c,
    '--detail-channel-surface': c + '33',
    '--detail-channel-border': c + '55',
    '--detail-spark-accent': c,
  }
}

function currentStatus(): string {
  return profile.value ? deviceStore.statusFor(profile.value.id) : ''
}

function currentAcquiring(): boolean {
  return profile.value ? deviceStore.acquiringFor(profile.value.id) : false
}

function formatChannel(ch: number, raw: number): string {
  const id = deviceStore.selectedDeviceId ?? ''
  return deviceStore.formatValue(id, ch, raw)
}

function channelUnit(channelIndex: number): string {
  return profile.value?.channels[channelIndex]?.unit || 'PA'
}

function channelRange(channelIndex: number): { min: number; max: number } {
  const channel = profile.value?.channels[channelIndex]
  return {
    min: channel?.rangeMin ?? -10,
    max: channel?.rangeMax ?? 10,
  }
}

function detailChannelTone(channelIndex: number, rawValue: number): 'active' | 'warning' {
  const id = deviceStore.selectedDeviceId ?? ''
  const status = currentStatus()
  if (status === 'Error' || status === 'Disconnected') return 'warning'

  const value = deviceStore.getDisplayValue(id, channelIndex, rawValue)
  const range = channelRange(channelIndex)
  const span = range.max - range.min
  const upperThreshold = range.max - span * 0.12
  const lowerThreshold = range.min + span * 0.12
  return value >= upperThreshold || value <= lowerThreshold ? 'warning' : 'active'
}

function isChartVisible(channelIndex: number): boolean {
  const id = deviceStore.selectedDeviceId
  if (!id) return false
  return deviceStore.isChartSelected(id, channelIndex)
}

function toggleChartVisibility(channelIndex: number): void {
  const id = deviceStore.selectedDeviceId
  if (!id) return
  deviceStore.toggleChartSelection(id, channelIndex)
}

function shouldDisableTare(channelIndex: number): boolean {
  const type = String(profile.value?.type ?? '')
  if (type !== 'DAQ-P-1604' && type !== 'DAQ-P-1064Pre') return true
  return channelIndex === 16 || channelIndex === 17
}

function setTare(channelIndex: number, rawValue: number): void {
  const id = deviceStore.selectedDeviceId
  if (!id || shouldDisableTare(channelIndex)) return
  deviceStore.setTare(id, channelIndex, rawValue)
}

const chartSelectorOpen = ref(false)

function openChartSelector(): void {
  chartSelectorOpen.value = true
}

function closeChartSelector(): void {
  chartSelectorOpen.value = false
}

function setAllChartVisibility(visible: boolean): void {
  const id = deviceStore.selectedDeviceId
  if (!id) return
  const channels = profile.value?.channels ?? []
  channels.forEach((ch) => {
    const currently = deviceStore.isChartSelected(id, ch.index)
    if (visible !== currently) {
      deviceStore.toggleChartSelection(id, ch.index)
    }
  })
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

const isPressureScannerDevice = computed(() => {
  const type = profile.value?.type
  return type === 'DAQ-P-1604' || type === 'DAQ-P-1064Pre'
})
</script>

<template>
  <div class="detail-panel">
    <div class="detail-panel__head">
      <div class="detail-panel__header-info">
        <div class="detail-panel__header-icon">
          <Activity class="w-5 h-5 text-emerald-500" />
        </div>
        <div>
          <h2 class="detail-panel__header-title">{{ profile?.name ?? 'Wind-DAQ' }}</h2>
          <p class="detail-panel__header-desc">
            {{ currentAcquiring() ? (i18n.t.acquiring || '采集中') : currentStatus() === 'Connected' ? (i18n.t.deviceRunning || '设备运行正常') : currentStatus() }}
            · {{ profile?.channels?.length ?? 0 }} {{ i18n.t.channels || '通道' }}
          </p>
        </div>
      </div>
      <div class="detail-panel__actions">
        <button
          class="detail-panel__btn detail-panel__btn--secondary"
          :class="{ 'opacity-40 cursor-not-allowed': !isPressureScannerDevice }"
          :disabled="!isPressureScannerDevice"
          @click="isPressureScannerDevice && profile && deviceStore.tareAllEnabled(profile.id)"
        >
          {{ i18n.t.tare || '归零' }}
        </button>
        <button
          v-if="currentStatus() === 'Connected'"
          class="detail-panel__btn"
          :class="currentAcquiring() ? 'detail-panel__btn--stop' : 'detail-panel__btn--acq'"
          @click="() => { const id = profile!.id; currentAcquiring() ? deviceStore.stopAcquisition(id) : deviceStore.startAcquisition(id) }"
        >
          {{ currentAcquiring() ? (i18n.t.stopAcquisition || '停止采集') : (i18n.t.startAcquisition || '开始采集') }}
        </button>
        <button
          class="detail-panel__btn"
          :class="currentStatus() === 'Connected' || currentAcquiring() ? 'detail-panel__btn--danger' : 'detail-panel__btn--primary'"
          @click="() => { const id = profile!.id; currentStatus() === 'Connected' || currentAcquiring() ? deviceStore.disconnect(id) : deviceStore.connect(id) }"
        >
          {{ currentAcquiring() ? (i18n.t.disconnect || '断开') : currentStatus() === 'Connected' ? (i18n.t.disconnect || '断开') : (i18n.t.connect || '连接') }}
        </button>
        <button
          v-if="props.mode !== 'table'"
          class="detail-panel__btn detail-panel__btn--icon"
          @click="openChartSelector"
          :title="i18n.t.channelSettings || '通道设置'"
        >
          <Settings2 class="w-4 h-4" />
        </button>
      </div>
    </div>

    <div class="detail-panel__content">
      <div v-if="showChart" class="detail-panel__chart" :class="{ 'detail-panel__chart--compact': isMixedMode }">
      <div class="detail-panel__chart-header">
        <div class="detail-panel__chart-title">
          <Activity class="w-4 h-4 text-emerald-500" />
          <span>实时趋势</span>
        </div>
        <div class="detail-panel__chart-controls">
          <button class="detail-panel__chart-btn" @click="openChartSelector">
            <Settings2 class="w-3.5 h-3.5" />
            <span>通道选择</span>
          </button>
          <div class="detail-panel__chart-info">
            <span class="detail-panel__chart-label">缓冲区</span>
            <span class="detail-panel__chart-value mono-font">100 点</span>
          </div>
        </div>
      </div>
      <div class="detail-panel__chart-body">
        <RealtimeChart :device-id="deviceStore.selectedDeviceId ?? ''" />
      </div>
      </div>

      <div
        v-if="snapshot?.channels?.length && showTable"
        class="detail-panel__grid"
        :class="{
          'detail-panel__grid--table': isTableMode,
          'detail-panel__grid--mixed': isMixedMode
        }"
      >
        <article
        v-for="(rawValue, snapshotIndex) in snapshot.channels"
        :key="snapshot.channelIndices[snapshotIndex]"
        class="channel-card"
        :class="{
          'channel-card--warning': detailChannelTone(snapshot.channelIndices[snapshotIndex], rawValue) === 'warning',
          'channel-card--selected': isChartVisible(snapshot.channelIndices[snapshotIndex])
        }"
        :style="channelStyle(snapshot.channelIndices[snapshotIndex])"
      >
        <div class="channel-card__top">
          <div class="channel-card__top-left">
            <div
              v-if="deviceStore.getOffset(deviceStore.selectedDeviceId ?? '', snapshot.channelIndices[snapshotIndex]) !== 0"
              class="channel-card__tare-badge"
              :title="i18n.t.tareOffsetApplied || '已应用归零偏移'"
            />
            <span class="channel-card__tag mono-font">CH_{{ String(snapshot.channelIndices[snapshotIndex] + 1).padStart(2, '0') }}</span>
          </div>
          <div class="channel-card__id">
            <span class="channel-card__dot" :style="{ background: channelColor(snapshot.channelIndices[snapshotIndex]), boxShadow: `0 0 8px ${channelColor(snapshot.channelIndices[snapshotIndex])}` }" />
            <span class="channel-card__id-text mono-font">CH{{ snapshot.channelIndices[snapshotIndex] + 1 }}</span>
          </div>
          <div class="channel-card__actions">
            <button
              class="channel-card__action-btn"
              :class="{ 'channel-card__action-btn--active': isChartVisible(snapshot.channelIndices[snapshotIndex]) }"
              :title="isChartVisible(snapshot.channelIndices[snapshotIndex]) ? '隐藏波形' : '显示波形'"
              @click.stop="toggleChartVisibility(snapshot.channelIndices[snapshotIndex])"
            >
              <Eye v-if="isChartVisible(snapshot.channelIndices[snapshotIndex])" class="channel-card__icon" />
              <EyeOff v-else class="channel-card__icon" />
            </button>
            <button
              class="channel-card__action-btn"
              :class="{ 'channel-card__action-btn--disabled': shouldDisableTare(snapshot.channelIndices[snapshotIndex]) }"
              :title="shouldDisableTare(snapshot.channelIndices[snapshotIndex]) ? '此通道不支持校零' : '归零'"
              :disabled="shouldDisableTare(snapshot.channelIndices[snapshotIndex])"
              @click.stop="setTare(snapshot.channelIndices[snapshotIndex], rawValue)"
            >
              <Minus class="channel-card__icon" />
            </button>
          </div>
        </div>
        <div class="channel-card__value-area">
          <div class="channel-card__value-row">
            <span
              class="channel-card__value mono-font"
              :class="{ 'text-amber-500': detailChannelTone(snapshot.channelIndices[snapshotIndex], rawValue) === 'warning' }"
            >
              {{ formatChannel(snapshot.channelIndices[snapshotIndex], rawValue) }}
            </span>
            <span class="channel-card__unit">{{ channelUnit(snapshot.channelIndices[snapshotIndex]) }}</span>
          </div>
        </div>
        <div v-if="!isMixedMode" class="channel-card__sparkline">
            <span
              v-for="(h, i) in sparkBars(snapshot.channelIndices[snapshotIndex])"
              :key="i"
              class="channel-card__spark"
              :class="{ 'channel-card__spark--active': i === sparkBars(snapshot.channelIndices[snapshotIndex]).length - 1 }"
              :style="{ height: `${h}%`, background: i === sparkBars(snapshot.channelIndices[snapshotIndex]).length - 1 ? channelColor(snapshot.channelIndices[snapshotIndex]) : undefined }"
            />
        </div>
        <div v-if="!isMixedMode" class="channel-card__range mono-font">
          <span>MIN: {{ channelRange(snapshot.channelIndices[snapshotIndex]).min }}</span>
          <span>MAX: {{ channelRange(snapshot.channelIndices[snapshotIndex]).max }}</span>
        </div>
      </article>
      </div>

      <div v-else-if="showTable" class="detail-panel__empty">
        {{ i18n.t.waitingData || '等待数据...' }}
      </div>
    </div>

    <div v-if="chartSelectorOpen" class="chart-selector" @click.self="closeChartSelector">
      <div class="chart-selector__panel" @click.stop>
        <div class="chart-selector__header">
          <div>
            <h3 class="chart-selector__title">通道选择</h3>
            <p class="chart-selector__subtitle">{{ profile?.name }}</p>
          </div>
          <button class="chart-selector__close" @click="closeChartSelector">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M18 6L6 18M6 6l12 12"/>
            </svg>
          </button>
        </div>

        <div class="chart-selector__grid">
          <label
            v-for="channel in (profile?.channels ?? [])"
            :key="channel.index"
            class="chart-selector__item"
            :class="{ 'chart-selector__item--active': isChartVisible(channel.index) }"
            :style="channelStyle(channel.index)"
          >
            <div class="chart-selector__item-info">
              <span class="chart-selector__dot" :style="{ background: channelColor(channel.index) }" />
              <div>
                <div class="chart-selector__name">{{ channel.name }}</div>
                <div class="chart-selector__channel">通道 {{ channel.index + 1 }}</div>
              </div>
            </div>
            <input
              type="checkbox"
              class="chart-selector__checkbox"
              :checked="isChartVisible(channel.index)"
              @change="toggleChartVisibility(channel.index)"
            />
          </label>
        </div>

        <div class="chart-selector__footer">
          <button class="chart-selector__btn chart-selector__btn--secondary" @click="setAllChartVisibility(true)">全选</button>
          <button class="chart-selector__btn chart-selector__btn--secondary" @click="setAllChartVisibility(false)">取消全选</button>
          <button class="chart-selector__btn chart-selector__btn--primary" @click="closeChartSelector">确定</button>
        </div>
      </div>
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
  padding: 0 var(--space-2);
  flex-shrink: 0;
}

.detail-panel__header-info {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.detail-panel__header-icon {
  width: 40px;
  height: 40px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  border-radius: 0.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
}

.detail-panel__header-title {
  margin: 0;
  font-size: var(--font-size-2xl);
  font-weight: var(--font-weight-black);
  color: #e2e8f0;
  letter-spacing: -0.02em;
}

:root[data-theme='light'] .detail-panel__header-title {
  color: #0f172a;
}

.detail-panel__header-desc {
  margin: 0.125rem 0 0;
  font-size: var(--font-size-xs);
  color: #64748b;
}

.detail-panel__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.detail-panel__btn {
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  font-size: var(--font-size-xs);
  font-weight: 700;
  transition: all 0.2s ease;
  cursor: pointer;
}

.detail-panel__btn:active {
  transform: scale(0.96);
}

.detail-panel__btn--primary {
  background: #10b981;
  color: white;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.3);
}

.detail-panel__btn--primary:hover {
  background: #059669;
}

.detail-panel__btn--danger {
  background: rgba(244, 63, 94, 0.1);
  color: #f43f5e;
  border: 1px solid rgba(244, 63, 94, 0.2);
}

.detail-panel__btn--danger:hover {
  background: rgba(244, 63, 94, 0.2);
}

.detail-panel__btn--acq {
  background: #10b981;
  color: white;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.3);
}

.detail-panel__btn--acq:hover {
  background: #059669;
}

.detail-panel__btn--stop {
  background: rgba(245, 158, 11, 0.1);
  color: #f59e0b;
  border: 1px solid rgba(245, 158, 11, 0.2);
}

.detail-panel__btn--stop:hover {
  background: rgba(245, 158, 11, 0.2);
}

.detail-panel__btn--secondary {
  background: rgba(255, 255, 255, 0.05);
  color: #94a3b8;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

:root[data-theme='light'] .detail-panel__btn--secondary {
  background: rgba(0, 0, 0, 0.03);
  color: #64748b;
  border: 1px solid rgba(0, 0, 0, 0.08);
}

.detail-panel__btn--secondary:hover {
  background: rgba(255, 255, 255, 0.1);
  color: #e2e8f0;
}

.detail-panel__btn--icon {
  width: 36px;
  height: 36px;
  padding: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.05);
  color: #64748b;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.detail-panel__btn--icon:hover {
  color: #10b981;
  background: rgba(16, 185, 129, 0.1);
}

.detail-panel__content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  overflow: hidden;
}

.detail-panel__chart {
  flex: 1;
  min-height: 280px;
  background: rgba(30, 41, 59, 0.4);
  backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: var(--radius-xl, 1rem);
  padding: var(--space-4);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

:root[data-theme='light'] .detail-panel__chart {
  background: rgba(255, 255, 255, 0.6);
  border: 1px solid rgba(0, 0, 0, 0.05);
}

.detail-panel__chart--compact {
  flex: 0 0 220px;
  min-height: 220px;
}

.detail-panel__chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-3);
  flex-shrink: 0;
}

.detail-panel__chart-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: #e2e8f0;
}

:root[data-theme='light'] .detail-panel__chart-title {
  color: #0f172a;
}

.detail-panel__chart-controls {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.detail-panel__chart-btn {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.375rem 0.75rem;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.25);
  border-radius: 0.5rem;
  color: #10b981;
  font-size: var(--font-size-xs);
  font-weight: 700;
  transition: all 0.2s ease;
}

.detail-panel__chart-btn:hover {
  background: rgba(16, 185, 129, 0.2);
  border-color: rgba(16, 185, 129, 0.4);
  transform: translateY(-1px);
}

:root[data-theme='light'] .detail-panel__chart-btn {
  background: rgba(16, 185, 129, 0.08);
  border-color: rgba(16, 185, 129, 0.2);
}

.detail-panel__chart-info {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.125rem;
}

.detail-panel__chart-label {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: #64748b;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.detail-panel__chart-value {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: #94a3b8;
}

.detail-panel__chart-body {
  flex: 1;
  min-height: 0;
  position: relative;
}

.detail-panel__chart-body :deep(.realtime-chart) {
  height: 100%;
}

.detail-panel__grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  grid-auto-rows: minmax(160px, 1fr);
  gap: var(--space-3);
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.detail-panel__grid--table,
.detail-panel__grid--mixed {
  grid-template-columns: repeat(4, 1fr);
}

@media (max-width: 1400px) {
  .detail-panel__grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 1100px) {
  .detail-panel__grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

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

.channel-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 0.3rem;
  background: linear-gradient(90deg, transparent, var(--theme-color, #10b981), transparent);
  opacity: 0.6;
}

.channel-card:hover {
  transform: translateY(-2px);
  border-color: var(--theme-color, #10b981);
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.1);
}

.channel-card--selected {
  box-shadow: 0 0 0 1px var(--theme-color, #10b981), 0 10px 25px rgba(0, 0, 0, 0.1);
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
  font-size: 0.625em;
  font-weight: 700;
  color: var(--text-secondary, #64748b);
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
  width: 0.6em;
  height: 0.6em;
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
  color: #64748b;
  background: rgba(0, 0, 0, 0.2);
  transition: all 0.2s ease;
}

:root[data-theme='light'] .channel-card__action-btn {
  background: rgba(0, 0, 0, 0.06);
}

.channel-card__action-btn:hover {
  color: var(--theme-color, #10b981);
  background: rgba(16, 185, 129, 0.15);
}

.channel-card__action-btn--active {
  color: var(--theme-color, #10b981);
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
  width: 0.6em;
  height: 0.6em;
  border-radius: 50%;
  background: #f59e0b;
  box-shadow: 0 0 4px #f59e0b;
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
  color: var(--text-primary, #334155);
}

.channel-card__value.text-amber-500 {
  color: #f59e0b;
}

.channel-card__unit {
  font-size: 0.625em;
  font-weight: var(--font-weight-black);
  color: var(--text-secondary, #64748b);
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
  box-shadow: 0 0 6px currentColor;
}

.channel-card__range {
  display: flex;
  justify-content: space-between;
  font-size: 0.625em;
  font-weight: 700;
  color: var(--text-secondary, #64748b);
  margin-top: 0.25em;
}

.detail-panel__empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-sm);
  color: #64748b;
}

.chart-empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-xs);
  color: #64748b;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 0.75rem;
}

:root[data-theme='light'] .chart-empty-state {
  background: rgba(0, 0, 0, 0.05);
}

.chart-selector {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(4px);
}

.chart-selector__panel {
  width: 100%;
  max-width: 32rem;
  max-height: 80vh;
  background: rgba(30, 41, 59, 0.95);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 1.5rem;
  box-shadow: 0 32px 80px -24px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

:root[data-theme='light'] .chart-selector__panel {
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid rgba(0, 0, 0, 0.1);
}

.chart-selector__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

:root[data-theme='light'] .chart-selector__header {
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.chart-selector__title {
  font-size: var(--font-size-xl);
  font-weight: 700;
  color: #e2e8f0;
}

:root[data-theme='light'] .chart-selector__title {
  color: #0f172a;
}

.chart-selector__subtitle {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: #64748b;
  margin-top: 0.25rem;
}

.chart-selector__close {
  width: 2rem;
  height: 2rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  color: #64748b;
  background: rgba(0, 0, 0, 0.2);
  transition: all 0.2s ease;
}

.chart-selector__close:hover {
  color: #10b981;
  background: rgba(16, 185, 129, 0.15);
}

.chart-selector__grid {
  flex: 1;
  overflow-y: auto;
  padding: 0.75rem 1.5rem;
  display: grid;
  gap: 0.5rem;
}

.chart-selector__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.625rem 0.75rem;
  border-radius: 0.75rem;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background: rgba(0, 0, 0, 0.15);
  cursor: pointer;
  transition: all 0.2s ease;
}

:root[data-theme='light'] .chart-selector__item {
  background: rgba(0, 0, 0, 0.03);
  border: 1px solid rgba(0, 0, 0, 0.06);
}

.chart-selector__item:hover {
  border-color: var(--theme-color, rgba(16, 185, 129, 0.3));
}

.chart-selector__item--active {
  border-color: var(--theme-color-border, rgba(16, 185, 129, 0.3));
  background: var(--theme-color-soft, rgba(16, 185, 129, 0.08));
}

.chart-selector__item-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.chart-selector__dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.chart-selector__name {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: #e2e8f0;
}

:root[data-theme='light'] .chart-selector__name {
  color: #0f172a;
}

.chart-selector__channel {
  font-size: var(--font-size-xs);
  color: #64748b;
}

.chart-selector__checkbox {
  width: 1rem;
  height: 1rem;
  accent-color: #10b981;
  cursor: pointer;
}

.chart-selector__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
  padding: 0.75rem 1.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}

:root[data-theme='light'] .chart-selector__footer {
  border-top: 1px solid rgba(0, 0, 0, 0.05);
}

.chart-selector__btn {
  padding: 0.375rem 0.875rem;
  border-radius: 0.5rem;
  font-size: var(--font-size-xs);
  font-weight: 700;
  transition: all 0.2s ease;
}

.chart-selector__btn--secondary {
  background: rgba(255, 255, 255, 0.05);
  color: #94a3b8;
  border: 1px solid rgba(255, 255, 255, 0.1);
}

:root[data-theme='light'] .chart-selector__btn--secondary {
  background: rgba(0, 0, 0, 0.03);
  color: #64748b;
  border: 1px solid rgba(0, 0, 0, 0.08);
}

.chart-selector__btn--primary {
  background: #10b981;
  color: white;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.3);
}
</style>
