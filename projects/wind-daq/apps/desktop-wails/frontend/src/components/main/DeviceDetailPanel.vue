<script setup lang="ts">
import { computed, defineAsyncComponent, ref } from 'vue'
import type { DataPayload } from '@api/types'
import { Activity, Settings2 } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore } from '@stores/storageStore'
import ChannelCard, { type ChannelCardData } from './ChannelCard.vue'
import ChartSelector, { type SelectorChannel } from './ChartSelector.vue'
// RealtimeChart 异步加载：echarts 是重量依赖（gzip ~250 KB），仅当用户进入设备面板时才下载，
// 避免拖慢首屏 LCP。参见 docs/runbooks/perf-frontend-bundle-baseline.md。
const RealtimeChart = defineAsyncComponent(() => import('@components/device/RealtimeChart.vue'))

const props = withDefaults(
  defineProps<{
    mode?: 'chart' | 'table' | 'both'
  }>(),
  { mode: 'both' },
)

const deviceStore = useDeviceStore()
const i18n = useI18nStore()
const storageStore = useStorageStore()

const CHANNEL_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#a855f7', '#f43f5e', '#06b6d4', '#f97316', '#6366f1']

const profile = computed(() => deviceStore.selectedProfile)
const snapshot = computed(() => deviceStore.selectedSnapshot)
const showChart = computed(() => props.mode === 'chart' || props.mode === 'both')
const showTable = computed(() => props.mode === 'table' || props.mode === 'both')
const isMixedMode = computed(() => props.mode === 'both')
const isTableMode = computed(() => props.mode === 'table')
const chartChannelIndices = computed(() => {
  const id = deviceStore.selectedDeviceId
  const channels = profile.value?.channels ?? []
  // fallback：默认选中全部 enabled 通道，排除大气压力 / 大气温度通道
  // （量程差异过大，同图绘制会压扁常规通道波形）。
  const fallback = channels
    .filter((channel) => channel.enabled && !deviceStore.isAtmosphericChannel(channel))
    .map((channel) => channel.index)
  if (!id) return fallback

  const selected = channels
    .filter((channel) => channel.enabled && deviceStore.isChartSelected(id, channel.index))
    .map((channel) => channel.index)
  return selected.length > 0 ? selected : fallback
})

/** 从全局配置读取波形图缓冲区点数，供实时趋势图使用 */
const waveformBufferSize = computed(() => storageStore.settings.waveformBufferSize)

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

  const value = deviceStore.applyDisplayTare(id, channelIndex, rawValue)
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

// 计算单个通道的 sparkline 高度数组（0-100 之间）。
// 采用倒序遍历 + 预分配数组，避免 .map().filter() 产生大量临时数组。
function computeSparkBars(history: DataPayload[], channelIndex: number): number[] {
  const values: number[] = []
  const target = 12
  for (let i = history.length - 1; i >= 0 && values.length < target; i--) {
    const entry = history[i]
    const indices = Array.isArray(entry?.channelIndices) ? entry.channelIndices : []
    const channels = Array.isArray(entry?.channels) ? entry.channels : []
    const pos = indices.indexOf(channelIndex)
    if (pos >= 0) {
      const v = channels[pos]
      if (typeof v === 'number') values.push(v)
    }
  }
  if (values.length === 0) {
    return Array.from({ length: target }, (_, i) => 15 + ((channelIndex * 37 + i * 17 + 20) % 60))
  }
  let mn = values[0]
  let mx = values[0]
  for (let i = 1; i < values.length; i++) {
    const v = values[i]
    if (v < mn) mn = v
    if (v > mx) mx = v
  }
  // 归一化到 15%-90% 区间，保留顶部呼吸空间
  const span = mx - mn || 1
  return values.reverse().map((v) => 15 + ((v - mn) / span) * 75)
}

// 按通道索引缓存 sparkline 数据，只有对应设备历史数据变化时才重新计算。
const sparkBarsMap = computed(() => {
  const id = deviceStore.selectedDeviceId ?? ''
  const history = deviceStore.historyFor(id)
  const snap = snapshot.value
  const map = new Map<number, number[]>()
  if (!snap?.channelIndices) return map
  for (const idx of snap.channelIndices) {
    map.set(idx, computeSparkBars(history, idx))
  }
  return map
})

const isPressureScannerDevice = computed(() => {
  const type = profile.value?.type
  return type === 'DAQ-P-1604' || type === 'DAQ-P-1064Pre'
})

// 预计算表格/卡片视图所需的全部通道数据，避免模板渲染时反复调用函数。
// 将每帧 O(通道数 × 函数调用次数) 的开销压缩为一次 computed 计算。
const channelCards = computed<ChannelCardData[]>(() => {
  const id = deviceStore.selectedDeviceId ?? ''
  const snap = snapshot.value
  if (!snap?.channels?.length) return []

  const indices = snap.channelIndices
  const channels = snap.channels
  const sparks = sparkBarsMap.value
  const cards: ChannelCardData[] = []
  for (let i = 0; i < channels.length; i++) {
    const index = indices[i]
    const rawValue = channels[i]
    const tone = detailChannelTone(index, rawValue)
    cards.push({
      index,
      rawValue,
      formattedValue: formatChannel(index, rawValue),
      unit: channelUnit(index),
      tone,
      isChartVisible: isChartVisible(index),
      style: channelStyle(index),
      color: channelColor(index),
      showTareBadge: deviceStore.getOffset(id, index) !== 0,
      disableTare: shouldDisableTare(index),
      sparkBars: sparks.get(index) ?? [],
      range: channelRange(index),
    })
  }
  return cards
})

// 通道选择弹窗的列表项：颜色/样式/可见性预计算，避免子组件持有业务状态。
const selectorChannels = computed<SelectorChannel[]>(() => {
  const channels = profile.value?.channels ?? []
  return channels.map((ch) => ({
    index: ch.index,
    name: ch.name,
    color: channelColor(ch.index),
    style: channelStyle(ch.index),
    visible: isChartVisible(ch.index),
  }))
})

const statusText = computed(() => {
  if (currentAcquiring()) {
    return i18n.t.acquiring || '采集中'
  }
  const status = currentStatus()
  if (status === 'Connected') {
    return i18n.t.deviceRunning || '设备运行正常'
  }
  if (status === 'Connecting') {
    return i18n.t.connectingState || '连接中'
  }
  if (status === 'Error') {
    return i18n.t.warningState || '警告'
  }
  return i18n.t.disconnectedState || '已断开'
})

const acquisitionButtonLabel = computed(() => {
  return currentAcquiring()
    ? (i18n.t.stopAcquisition || '停止采集')
    : (i18n.t.startAcquisition || '开始采集')
})

const connectionButtonLabel = computed(() => {
  // 连接中：明确显示"连接中..."并由按钮 disabled 防止用户重复点击
  if (currentStatus() === 'Connecting') {
    return (i18n.t.connectingState || '连接中') + '...'
  }
  return currentAcquiring() || currentStatus() === 'Connected'
    ? (i18n.t.disconnect || '断开')
    : (i18n.t.connect || '连接')
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
            {{ statusText }}
            · {{ profile?.channels?.length ?? 0 }} {{ i18n.t.channels || '通道' }}
          </p>
        </div>
      </div>
      <div class="detail-panel__actions">
        <UiButton
          variant="secondary"
          size="sm"
          :disabled="!isPressureScannerDevice"
          @click="isPressureScannerDevice && profile && deviceStore.tareAllEnabled(profile.id)"
        >
          {{ i18n.t.tare || '归零' }}
        </UiButton>
        <UiButton
          v-if="currentStatus() === 'Connected'"
          :variant="currentAcquiring() ? 'danger' : 'primary'"
          size="sm"
          @click="() => { const id = profile!.id; currentAcquiring() ? deviceStore.stopAcquisition(id) : deviceStore.startAcquisition(id) }"
        >
          {{ acquisitionButtonLabel }}
        </UiButton>
        <UiButton
          :variant="currentStatus() === 'Connected' || currentAcquiring() ? 'danger' : 'primary'"
          size="sm"
          :disabled="currentStatus() === 'Connecting'"
          @click="() => {
            if (!profile) return
            const id = profile.id
            if (currentStatus() === 'Connecting') return
            currentStatus() === 'Connected' || currentAcquiring()
              ? deviceStore.disconnect(id)
              : deviceStore.connect(id)
          }"
        >
          {{ connectionButtonLabel }}
        </UiButton>
        <UiButton
          v-if="props.mode !== 'table'"
          variant="ghost"
          size="sm"
          @click="openChartSelector"
          :aria-label="i18n.t.channelSettings || '通道设置'"
        >
          <template #icon>
            <Settings2 class="w-4 h-4" />
          </template>
        </UiButton>
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
            <UiButton variant="secondary" size="sm" @click="openChartSelector">
              <template #icon>
                <Settings2 class="w-4 h-4" />
              </template>
              通道选择
            </UiButton>
            <div class="detail-panel__chart-info">
              <span class="detail-panel__chart-label">{{ i18n.t.bufferWindowLabel }}</span>
              <span class="detail-panel__chart-value mono-font">{{ waveformBufferSize }} {{ i18n.t.pts }}</span>
            </div>
          </div>
        </div>
        <div class="detail-panel__chart-body">
          <RealtimeChart
            :device-id="deviceStore.selectedDeviceId ?? ''"
            :channel-indices="chartChannelIndices"
            :max-points="waveformBufferSize"
          />
        </div>
      </div>

      <div
        v-if="channelCards.length && showTable"
        class="detail-panel__grid"
        :class="{
          'detail-panel__grid--table': isTableMode,
          'detail-panel__grid--mixed': isMixedMode
        }"
      >
        <ChannelCard
          v-for="card in channelCards"
          :key="card.index"
          :card="card"
          :compact="isMixedMode"
          @toggle-chart="toggleChartVisibility"
          @tare="setTare"
        />
      </div>

      <div v-else-if="showTable" class="detail-panel__empty">
        {{ i18n.t.waitingData || '等待数据...' }}
      </div>
    </div>

    <ChartSelector
      v-if="chartSelectorOpen"
      :profile-name="profile?.name ?? ''"
      :channels="selectorChannels"
      :selected-count="chartChannelIndices.length"
      :total-count="profile?.channels?.length ?? 0"
      @close="closeChartSelector"
      @toggle="toggleChartVisibility"
      @set-all="setAllChartVisibility"
    />
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
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

:root[data-theme='light'] .detail-panel__header-title {
  color: var(--text-primary);
}

.detail-panel__header-desc {
  margin: 0.125rem 0 0;
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

.detail-panel__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
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

/* 混合模式：波形图应占据主体可用空间，而非固定 220px 导致底部留白 */
.detail-panel__chart--compact {
  flex: 1;
  min-height: 280px;
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
  color: var(--text-primary);
}

:root[data-theme='light'] .detail-panel__chart-title {
  color: var(--text-primary);
}

.detail-panel__chart-controls {
  display: flex;
  align-items: center;
  gap: var(--space-3);
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
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.detail-panel__chart-value {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--text-tertiary);
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

.detail-panel__grid--table {
  grid-template-columns: repeat(4, 1fr);
}

/* 混合模式：极简单行卡片网格，行高仅 40px，
   配合单行卡片布局，最大化把纵向空间让给波形图。 */
.detail-panel__grid--mixed {
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  grid-auto-rows: 40px;
  gap: var(--space-1);
  flex: 0 0 auto;
  overflow-y: auto;
  padding-bottom: 2px;
}

@media (max-width: 1400px) {
  .detail-panel__grid:not(.detail-panel__grid--mixed) {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (max-width: 1100px) {
  .detail-panel__grid:not(.detail-panel__grid--mixed) {
    grid-template-columns: repeat(2, 1fr);
  }
}

.detail-panel__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--text-muted);
  font-size: var(--font-size-sm);
}
</style>
