<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useTheme } from '@composables/useTheme'
import {
  NButton,
  NCard,
  NCheckbox,
  NModal,
  NResult,
  NSpace,
  NTag,
  NText,
} from 'naive-ui'
import {
  Activity,
  Eraser,
  Layers,
  LineChart,
  Network,
  Settings2,
  Wifi,
} from '@lucide/vue'
import ChannelGrid from '@components/device/ChannelGrid.vue'
import RealtimeChart from '@components/device/RealtimeChart.vue'

const deviceStore = useDeviceStore()
const { theme } = useTheme()

const openConfig = inject<() => void>('shell:openConfig', () => {})

const selected = computed(() => deviceStore.selectedProfile)
const status = computed(() => (selected.value ? deviceStore.statusFor(selected.value.id) : ''))
const errorMessage = computed(() => (selected.value ? deviceStore.errorFor(selected.value.id) : ''))
const isAcquiring = computed(() => (selected.value ? deviceStore.acquiringFor(selected.value.id) : false))
const sampleCount = computed(() => (selected.value ? deviceStore.historyFor(selected.value.id).length : 0))
const showChannelSelector = ref(false)

const selectedChannelCount = computed(() => {
  if (!selected.value) return 0
  const id = selected.value.id
  return selected.value.channels.filter((c) => c.enabled && deviceStore.isChartSelected(id, c.index)).length
})

const enabledCount = computed(() => {
  if (!selected.value) return 0
  return selected.value.channels.filter((c) => c.enabled).length
})

const selectableChannels = computed(() => {
  if (!selected.value) return []
  return selected.value.channels.filter((c) => c.enabled)
})

const isChartVisible = computed(() => selectedChannelCount.value > 0)
const isAllChannelsSelected = computed(() => enabledCount.value > 0 && selectedChannelCount.value === enabledCount.value)

function openChannelSelection() {
  showChannelSelector.value = !showChannelSelector.value
}

function toggleChannel(channelIndex: number) {
  if (!selected.value) return
  deviceStore.toggleChartSelection(selected.value.id, channelIndex)
}

function isChannelSelected(channelIndex: number): boolean {
  if (!selected.value) return false
  return deviceStore.isChartSelected(selected.value.id, channelIndex)
}

function selectAllChannels() {
  if (!selected.value) return
  for (const channel of selectableChannels.value) {
    if (!deviceStore.isChartSelected(selected.value.id, channel.index)) {
      deviceStore.toggleChartSelection(selected.value.id, channel.index)
    }
  }
}

function clearAllChannels() {
  if (!selected.value) return
  for (const channel of selectableChannels.value) {
    if (deviceStore.isChartSelected(selected.value.id, channel.index)) {
      deviceStore.toggleChartSelection(selected.value.id, channel.index)
    }
  }
}

/** 清除当前波形（#31）：仅清前端缓冲，不影响后端采集与录制 */
function clearWaveform() {
  if (!selected.value) return
  deviceStore.clearHistory(selected.value.id)
}

async function connectDisconnect() {
  if (!selected.value) return
  if (status.value === 'Starting' || status.value === 'Stopping') return
  if (status.value === 'Connected' || status.value === 'Acquiring') {
    await deviceStore.disconnect(selected.value.id)
  } else if (status.value === 'Disconnected' || status.value === 'Error') {
    await deviceStore.connect(selected.value.id)
  }
}

function statusType(): 'success' | 'warning' | 'error' | 'default' {
  if (isAcquiring.value || status.value === 'Connected') return 'success'
  if (status.value === 'Connecting' || status.value === 'Starting' || status.value === 'Stopping') return 'warning'
  if (status.value === 'Error') return 'error'
  return 'default'
}

function statusLabel(): string {
  if (isAcquiring.value) return '采集中'
  if (status.value === 'Starting') return '启动中'
  if (status.value === 'Stopping') return '停止中'
  if (status.value === 'Connected') return '已连接'
  if (status.value === 'Connecting') return '连接中'
  if (status.value === 'Error') return '错误'
  return '未连接'
}
</script>

<template>
  <div class="detail">
    <div v-if="!selected" class="detail__empty" data-testid="detail-empty">
      <div class="detail__empty-illu">
        <div class="detail__empty-icon">
          <Activity class="detail__empty-icon-svg" />
        </div>
        <div class="detail__empty-rings">
          <div class="detail__empty-ring" />
          <div class="detail__empty-ring" />
          <div class="detail__empty-ring" />
        </div>
      </div>
      <NText tag="h2" depth="1" style="font-size:1.25rem;font-weight:800;letter-spacing:-0.02em">选择一个设备开始监控</NText>
      <NText depth="3" style="font-size:0.95rem;max-width:30rem;text-align:center;line-height:1.6">从左侧设备列表中选择一台 T1603，或者点击顶栏 + 添加新设备</NText>
      <div class="detail__empty-tips">
        <div class="detail__empty-tip">
          <Wifi :size="14" class="detail__empty-tip-icon" />
          <span>实时波形</span>
        </div>
        <div class="detail__empty-tip">
          <Layers :size="14" class="detail__empty-tip-icon" />
          <span>多通道并行</span>
        </div>
        <div class="detail__empty-tip">
          <LineChart :size="14" class="detail__empty-tip-icon" />
          <span>数据导出</span>
        </div>
      </div>
    </div>

    <template v-else>
      <div class="detail__top">
        <NCard size="small" :bordered="false" class="glass-panel" content-style="padding:0">
          <div class="detail__header">
            <div class="detail__header-left">
              <div class="detail__device-icon">
                <Activity class="detail__device-icon-svg" />
              </div>
              <div class="detail__device-info">
                <NText tag="h2" depth="1" style="font-size:1rem;font-weight:800;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">
                  {{ selected.name || '未命名设备' }}
                </NText>
                <NSpace size="small" align="center" style="color:var(--text-muted);font-size:0.7rem;flex-wrap:wrap">
                  <NText depth="3" style="font-size:0.7rem;font-weight:600">{{ selected.address }}:{{ selected.port }}</NText>
                  <span class="detail__meta-dot" />
                  <NText depth="3" style="font-size:0.7rem">{{ (selected.t1603Config?.thermocoupleTypes || 'K')[0] }} 型热电偶</NText>
                  <span class="detail__meta-dot" />
                  <NText depth="3" style="font-size:0.7rem;font-weight:600">{{ selected.t1603Config?.samplingRate ?? selected.samplingRate }} Hz</NText>
                </NSpace>
              </div>
            </div>
            <div class="detail__header-right">
              <NTag :type="statusType()" size="small" :bordered="false" round>
                <template #avatar>
                  <span class="status-dot" :class="`status-dot--${statusType()}`" />
                </template>
                {{ statusLabel() }}
              </NTag>
              <NButton
                size="small"
                :type="status === 'Connected' || status === 'Acquiring' || status === 'Starting' || status === 'Stopping' ? 'error' : 'primary'"
                :loading="status === 'Connecting'"
                @click="connectDisconnect"
              >
                <template #icon>
                  <Network :size="14" />
                </template>
                {{ status === 'Connected' || status === 'Acquiring' || status === 'Starting' || status === 'Stopping' ? '断开' : status === 'Connecting' ? '连接中' : '连接' }}
              </NButton>
              <NButton size="small" secondary @click="openConfig()">
                <template #icon><Settings2 :size="14" /></template>配置
              </NButton>
            </div>
          </div>
          <!-- 设备错误详情条：仅在 errorMessage 非空时显示 -->
          <div v-if="errorMessage" class="detail__error-bar">
            <NText depth="3" style="font-size:0.72rem;color:var(--danger)">{{ errorMessage }}</NText>
          </div>
        </NCard>

        <NCard size="small" :bordered="false" class="glass-panel detail__chart" data-testid="detail-chart" content-style="display:flex;flex-direction:column;flex:1;min-height:0;padding:0">
          <div class="detail__chart-header">
            <div class="detail__chart-title">
              <LineChart :size="15" style="color:var(--accent)" />
              <NText depth="1" style="font-size:0.9rem;font-weight:700">实时波形</NText>
              <NText v-if="isChartVisible" depth="3" style="font-size:0.7rem;margin-left:0.25rem">· {{ selectedChannelCount }} 条曲线</NText>
              <NText v-else-if="sampleCount > 0" depth="3" style="font-size:0.7rem;margin-left:0.25rem;opacity:0.7">· 点击右侧按钮选择通道</NText>
            </div>
            <div class="detail__chart-tools">
              <NButton
                size="tiny"
                :type="showChannelSelector ? 'primary' : 'default'"
                secondary
                @click="openChannelSelection"
              >
                <template #icon><Layers :size="13" /></template>通道选择
              </NButton>
              <!-- 清除当前波形（#31）：清空前端缓冲让波形从头开始绘制 -->
              <NButton
                size="tiny"
                secondary
                :disabled="sampleCount === 0"
                title="清除当前波形"
                @click="clearWaveform"
              >
                <template #icon><Eraser :size="13" /></template>清除波形
              </NButton>
              <div v-if="showChannelSelector" class="detail__channel-popover">
                <div class="detail__channel-popover-head">
                  <div>
                    <NText depth="1" style="font-size:0.72rem;font-weight:700">通道选择</NText>
                    <NText depth="3" style="font-size:0.7rem">{{ selectedChannelCount }}/{{ enabledCount }}</NText>
                  </div>
                  <NSpace size="small">
                    <NButton size="tiny" quaternary :disabled="isAllChannelsSelected" @click="selectAllChannels">全选</NButton>
                    <NButton size="tiny" quaternary :disabled="selectedChannelCount === 0" @click="clearAllChannels">全取消</NButton>
                  </NSpace>
                </div>
                <div class="detail__channel-selector-grid">
                  <label
                    v-for="channel in selectableChannels"
                    :key="channel.index"
                    class="detail__channel-option"
                    :class="{ 'detail__channel-option--active': isChannelSelected(channel.index) }"
                  >
                    <NCheckbox
                      :checked="isChannelSelected(channel.index)"
                      size="small"
                      @update:checked="toggleChannel(channel.index)"
                    />
                    <span class="detail__channel-option-label">CH{{ channel.index + 1 }}</span>
                  </label>
                </div>
              </div>
            </div>
          </div>
          <div class="detail__chart-body">
            <RealtimeChart :device-id="selected.id" :max-points="120" />
          </div>
        </NCard>
      </div>

      <section class="detail__channels">
        <div class="detail__channels-header">
          <NText depth="1" style="font-size:0.95rem;font-weight:800;letter-spacing:-0.01em">通道监控</NText>
          <NText depth="3" style="font-size:0.7rem">通过上方"通道选择"按钮可以控制波形图中显示的通道</NText>
        </div>
        <ChannelGrid :device-id="selected.id" />
      </section>
    </template>
  </div>
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  gap: var(--layout-content-gap);
  height: 100%;
  overflow: hidden;
}

.detail__top {
  display: flex;
  flex-direction: column;
  gap: var(--layout-content-gap);
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
}

/* ---- empty state ---- */
.detail__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 4rem 2rem;
  gap: 1.25rem;
  text-align: center;
  min-height: 60vh;
}

.detail__empty-illu {
  position: relative;
  width: 120px;
  height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 0.5rem;
}

.detail__empty-icon {
  position: relative;
  z-index: 2;
  width: 72px;
  height: 72px;
  border-radius: var(--radius-xl);
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 12px 32px var(--accent-glow);
}

.detail__empty-icon-svg {
  width: 36px;
  height: 36px;
  color: #ffffff;
}

.detail__empty-rings {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}

.detail__empty-ring {
  position: absolute;
  border-radius: 50%;
  border: 1px solid var(--accent-border);
  animation: empty-pulse 3s ease-in-out infinite;
}

.detail__empty-ring:nth-child(1) { width: 88px; height: 88px; animation-delay: 0s; }
.detail__empty-ring:nth-child(2) { width: 108px; height: 108px; animation-delay: 0.5s; }
.detail__empty-ring:nth-child(3) { width: 128px; height: 128px; animation-delay: 1s; }

@keyframes empty-pulse {
  0%, 100% { opacity: 0.4; transform: scale(1); }
  50% { opacity: 0.15; transform: scale(1.05); }
}

.detail__empty-tips {
  display: flex;
  gap: 1.25rem;
  margin-top: 1rem;
  flex-wrap: wrap;
  justify-content: center;
}

.detail__empty-tip {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.85rem;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-pill);
  font-size: 0.75rem;
  color: var(--text-secondary);
  font-weight: 600;
  transition: all var(--motion-fast) var(--easing-standard);
}

.detail__empty-tip:hover {
  background: var(--btn-bg-hover);
  border-color: var(--border-hover);
  transform: translateY(-1px);
}

.detail__empty-tip-icon { color: var(--accent); }

/* ---- header ---- */
.detail__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1.25rem;
  gap: 1.25rem;
}

.detail__header-left {
  display: flex;
  align-items: center;
  gap: 1rem;
  min-width: 0;
  flex: 1;
}

.detail__device-icon {
  width: 40px;
  height: 40px;
  border-radius: var(--radius-lg);
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 6px 18px var(--accent-glow);
  flex-shrink: 0;
}

.detail__device-icon-svg {
  width: 20px;
  height: 20px;
  color: #ffffff;
}

.detail__device-info {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  min-width: 0;
}

.detail__meta-dot {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: var(--text-muted);
  opacity: 0.5;
}

.detail__header-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-shrink: 0;
}

/* 设备错误详情条 */
.detail__error-bar {
  padding: 0.5rem 0.75rem;
  margin-top: 0.5rem;
  background: var(--danger-muted, rgba(244, 63, 94, 0.1));
  border: 1px solid var(--danger-border, rgba(244, 63, 94, 0.25));
  border-radius: var(--radius-md, 6px);
  word-break: break-all;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}

.status-dot--success { animation: status-pulse 1.5s ease-in-out infinite; }
.status-dot--warning { animation: status-pulse 0.8s ease-in-out infinite; }

@keyframes status-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.35; }
}

/* ---- chart panel ---- */
.detail__chart {
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
}

.detail__chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.85rem 1.25rem;
  border-bottom: 1px solid var(--divider-color);
  gap: 1rem;
}

.detail__chart-title {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.detail__chart-tools {
  position: relative;
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.detail__chart-body {
  flex: 1;
  padding: 0.5rem 0.85rem 0.85rem;
  min-height: 0;
}

/* ---- channel popover ---- */
.detail__channel-popover {
  position: absolute;
  top: calc(100% + 0.55rem);
  right: 0;
  z-index: 20;
  width: min(18rem, 78vw);
  padding: 0.85rem;
  border-radius: var(--radius-lg);
  border: 1px solid var(--accent-border);
  background: var(--bg-panel);
  box-shadow: var(--shadow-lg);
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.detail__channel-popover-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
}

.detail__channel-popover-head > div:first-child {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}

.detail__channel-selector-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0.45rem 0.6rem;
}

.detail__channel-option {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  min-height: 2rem;
  padding: 0.35rem 0.45rem;
  border-radius: var(--radius-sm);
  border: 1px solid transparent;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.detail__channel-option:hover {
  background: var(--btn-bg);
}

.detail__channel-option--active {
  background: var(--accent-muted);
  border-color: var(--accent-border);
}

.detail__channel-option-label {
  font-size: 0.72rem;
  font-weight: 600;
  color: var(--text-secondary);
  white-space: nowrap;
}

.detail__channel-option--active .detail__channel-option-label {
  color: var(--accent);
}

/* ---- channel section ---- */
.detail__channels {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  flex: 0 0 auto;
  max-height: 35%;
  overflow-y: auto;
  padding: 0 0.25rem 0.25rem;
}

.detail__channels-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 1rem;
  padding: 0 0.25rem;
}

/* ---- animations ---- */
@media (max-width: 767px) {
  .detail__channel-selector-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}

@media (prefers-reduced-motion: reduce) {
  .detail__empty-ring { animation: none; opacity: 0.3; }
  .status-dot--success, .status-dot--warning { animation: none; }
  .detail__empty-tip:hover { transform: none; }
}
</style>
