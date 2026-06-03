<script setup lang="ts">
import { computed, inject } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useTheme } from '@composables/useTheme'
import { Activity, Wifi, WifiOff, Loader2, Settings2, Eye, EyeOff, LineChart, Layers } from '@lucide/vue'
import ChannelGrid from '@components/device/ChannelGrid.vue'
import RealtimeChart from '@components/device/RealtimeChart.vue'

const deviceStore = useDeviceStore()
const { theme } = useTheme()

const openConfig = inject<() => void>('shell:openConfig', () => {})

const selected = computed(() => deviceStore.selectedProfile)
const status = computed(() => (selected.value ? deviceStore.statusFor(selected.value.id) : ''))
const isAcquiring = computed(() => (selected.value ? deviceStore.acquiringFor(selected.value.id) : false))
const sampleCount = computed(() => (selected.value ? deviceStore.historyFor(selected.value.id).length : 0))

const selectedChannelCount = computed(() => {
  if (!selected.value) return 0
  const id = selected.value.id
  return selected.value.channels.filter((c) => c.enabled && deviceStore.isChartSelected(id, c.index)).length
})

const enabledCount = computed(() => {
  if (!selected.value) return 0
  return selected.value.channels.filter((c) => c.enabled).length
})

const isChartVisible = computed(() => selectedChannelCount.value > 0)

function toggleAllCharts() {
  if (!selected.value) return
  const id = selected.value.id
  if (isChartVisible.value) {
    // 隐藏所有
    for (const ch of selected.value.channels) {
      if (deviceStore.isChartSelected(id, ch.index)) {
        deviceStore.toggleChartSelection(id, ch.index)
      }
    }
  } else {
    // 显示前4个有数据的通道
    let added = 0
    for (const ch of selected.value.channels) {
      if (ch.enabled && !deviceStore.isChartSelected(id, ch.index) && added < 4) {
        deviceStore.toggleChartSelection(id, ch.index)
        added++
      }
    }
  }
}

async function connectDisconnect() {
  if (!selected.value) return
  if (status.value === 'Connected' || status.value === 'Acquiring') {
    await deviceStore.disconnect(selected.value.id)
  } else if (status.value === 'Disconnected' || status.value === 'Error') {
    await deviceStore.connect(selected.value.id)
  }
}

function statusBadgeClass(): string {
  if (isAcquiring.value) return 'detail__status-badge--acquiring'
  if (status.value === 'Connected') return 'detail__status-badge--connected'
  if (status.value === 'Connecting') return 'detail__status-badge--connecting'
  if (status.value === 'Error') return 'detail__status-badge--error'
  return 'detail__status-badge--disconnected'
}

function statusLabel(): string {
  if (isAcquiring.value) return '采集中'
  if (status.value === 'Connected') return '已连接'
  if (status.value === 'Connecting') return '连接中'
  if (status.value === 'Error') return '错误'
  return '未连接'
}
</script>

<template>
  <div class="detail">
    <!-- 空状态 -->
    <div v-if="!selected" class="detail__empty glass-panel">
      <div class="detail__empty-illu">
        <div class="detail__empty-icon">
          <Activity class="detail__empty-icon-svg" />
        </div>
        <div class="detail__empty-rings">
          <div class="detail__empty-ring"></div>
          <div class="detail__empty-ring"></div>
          <div class="detail__empty-ring"></div>
        </div>
      </div>
      <h2 class="detail__empty-title">选择一个设备开始监控</h2>
      <p class="detail__empty-hint">从左侧设备列表中选择一台 T1603，或者点击顶栏 + 添加新设备</p>
      <div class="detail__empty-tips">
        <div class="detail__empty-tip">
          <Wifi class="detail__empty-tip-icon" />
          <span>实时波形</span>
        </div>
        <div class="detail__empty-tip">
          <Layers class="detail__empty-tip-icon" />
          <span>16 通道并行</span>
        </div>
        <div class="detail__empty-tip">
          <LineChart class="detail__empty-tip-icon" />
          <span>数据导出</span>
        </div>
      </div>
    </div>

    <!-- 详情内容 -->
    <template v-else>
      <!-- 详情面板头部 -->
      <header class="detail__header glass-panel">
        <div class="detail__header-left">
          <div class="detail__device-icon">
            <Activity class="detail__device-icon-svg" />
          </div>
          <div class="detail__device-info">
            <h2 class="detail__device-name">{{ selected.name || '未命名设备' }}</h2>
            <div class="detail__device-meta">
              <span class="detail__device-meta-item mono">{{ selected.address }}:{{ selected.port }}</span>
              <span class="detail__device-meta-divider"></span>
              <span class="detail__device-meta-item">
                {{ (selected.t1603Config?.thermocoupleTypes || 'K')[0] }} 型热电偶
              </span>
              <span class="detail__device-meta-divider"></span>
              <span class="detail__device-meta-item mono">{{ selected.samplingRate }} Hz</span>
            </div>
          </div>
        </div>
        <div class="detail__header-right">
          <div class="detail__status-badge" :class="statusBadgeClass()">
            <span class="detail__status-dot"></span>
            {{ statusLabel() }}
          </div>
          <button
            class="detail__action"
            :class="{
              'detail__action--connected': status === 'Connected' || status === 'Acquiring',
            }"
            :title="status === 'Connected' || status === 'Acquiring' ? '断开连接' : '连接设备'"
            @click="connectDisconnect"
          >
            <Wifi v-if="status === 'Connected' || status === 'Acquiring'" class="detail__action-icon" />
            <Loader2 v-else-if="status === 'Connecting'" class="detail__action-icon detail__action-icon--spin" />
            <WifiOff v-else class="detail__action-icon" />
            <span>{{ status === 'Connected' || status === 'Acquiring' ? '断开' : status === 'Connecting' ? '连接中' : '连接' }}</span>
          </button>
          <button
            class="detail__action detail__action--accent"
            title="设备配置"
            @click="openConfig()"
          >
            <Settings2 class="detail__action-icon" />
            <span>配置</span>
          </button>
        </div>
      </header>

      <!-- 图表面板 -->
      <section class="detail__chart glass-panel">
        <div class="detail__chart-header">
          <div class="detail__chart-title">
            <LineChart class="detail__chart-title-icon" />
            <span>实时波形</span>
            <span v-if="isChartVisible" class="detail__chart-meta">
              · {{ selectedChannelCount }} 条曲线
            </span>
            <span v-else-if="sampleCount > 0" class="detail__chart-meta detail__chart-meta--hint">
              · 点击右侧按钮添加波形
            </span>
          </div>
          <button
            class="detail__chart-toggle"
            :class="{ 'detail__chart-toggle--off': !isChartVisible }"
            :title="isChartVisible ? '隐藏全部波形' : '显示波形'"
            @click="toggleAllCharts"
          >
            <Eye v-if="isChartVisible" class="detail__chart-toggle-icon" />
            <EyeOff v-else class="detail__chart-toggle-icon" />
            <span>{{ isChartVisible ? '隐藏波形' : '添加波形' }}</span>
          </button>
        </div>
        <div class="detail__chart-body">
          <RealtimeChart :device-id="selected.id" :max-points="120" />
        </div>
      </section>

      <!-- 通道网格 -->
      <section class="detail__channels">
        <div class="detail__channels-header">
          <h3 class="detail__channels-title">通道监控</h3>
          <p class="detail__channels-hint">点击通道卡片上的眼睛图标即可在波形图中加入或移除</p>
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
  min-height: 100%;
}

/* ----------------------------
   空状态
   ---------------------------- */
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

.detail__empty-ring:nth-child(1) {
  width: 88px;
  height: 88px;
  animation-delay: 0s;
}

.detail__empty-ring:nth-child(2) {
  width: 108px;
  height: 108px;
  animation-delay: 0.5s;
}

.detail__empty-ring:nth-child(3) {
  width: 128px;
  height: 128px;
  animation-delay: 1s;
}

.detail__empty-title {
  font-size: var(--font-size-xl);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

.detail__empty-hint {
  font-size: var(--font-size-base);
  color: var(--text-muted);
  max-width: 30rem;
  line-height: 1.6;
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
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
  font-weight: 600;
  transition: all var(--motion-fast) var(--easing-standard);
}

.detail__empty-tip:hover {
  background: var(--btn-bg-hover);
  border-color: var(--border-hover);
  transform: translateY(-1px);
}

.detail__empty-tip-icon {
  width: 14px;
  height: 14px;
  color: var(--accent);
}

/* ----------------------------
   详情头部
   ---------------------------- */
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

.detail__device-name {
  font-size: var(--font-size-lg);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.02em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.detail__device-meta {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  flex-wrap: wrap;
  color: var(--text-muted);
  font-size: var(--font-size-xs);
}

.detail__device-meta-item {
  font-weight: 600;
  letter-spacing: 0.02em;
}

.detail__device-meta-divider {
  width: 3px;
  height: 3px;
  border-radius: 50%;
  background: var(--text-muted);
  opacity: 0.5;
}

.detail__header-right {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.detail__status-badge {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.85rem;
  border-radius: var(--radius-pill);
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  border: 1px solid var(--border-default);
  background: var(--btn-bg);
}

.detail__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.detail__status-badge--acquiring {
  background: var(--success-muted);
  border-color: var(--accent-border);
  color: var(--accent);
}

.detail__status-badge--acquiring .detail__status-dot {
  animation: status-pulse 1.5s ease-in-out infinite;
}

.detail__status-badge--connected {
  background: var(--success-muted);
  border-color: var(--accent-border);
  color: var(--accent);
}

.detail__status-badge--connecting {
  background: var(--warning-muted);
  border-color: var(--warning);
  color: var(--warning);
}

.detail__status-badge--connecting .detail__status-dot {
  animation: status-pulse 0.8s ease-in-out infinite;
}

.detail__status-badge--error {
  background: var(--danger-muted);
  border-color: var(--danger-border);
  color: var(--danger);
}

.detail__status-badge--disconnected {
  color: var(--text-muted);
}

.detail__action {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.5rem 0.95rem;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 700;
  background: var(--btn-bg);
  color: var(--text-secondary);
  border: 1px solid var(--border-default);
  transition: all var(--motion-fast) var(--easing-standard);
}

.detail__action:hover {
  background: var(--btn-bg-hover);
  color: var(--text-primary);
  border-color: var(--border-hover);
}

.detail__action--connected {
  color: var(--accent);
  background: var(--success-muted);
  border-color: var(--accent-border);
}

.detail__action--connected:hover {
  color: var(--danger);
  background: var(--danger-muted);
  border-color: var(--danger-border);
}

.detail__action--accent {
  background: var(--accent-muted);
  color: var(--accent);
  border-color: var(--accent-border);
}

.detail__action--accent:hover {
  background: var(--accent);
  color: #ffffff;
  border-color: var(--accent);
  box-shadow: 0 4px 14px var(--accent-glow);
}

.detail__action-icon {
  width: 14px;
  height: 14px;
}

.detail__action-icon--spin {
  animation: spin 1s linear infinite;
}

/* ----------------------------
   图表面板
   ---------------------------- */
.detail__chart {
  display: flex;
  flex-direction: column;
  min-height: 280px;
}

.detail__chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--divider-color);
}

.detail__chart-title {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: var(--font-size-base);
  font-weight: 700;
  color: var(--text-primary);
}

.detail__chart-title-icon {
  width: 16px;
  height: 16px;
  color: var(--accent);
}

.detail__chart-meta {
  color: var(--text-muted);
  font-weight: 500;
  font-size: var(--font-size-xs);
  margin-left: 0.25rem;
}

.detail__chart-meta--hint {
  opacity: 0.7;
}

.detail__chart-toggle {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.75rem;
  border-radius: var(--radius-md);
  font-size: var(--font-size-xs);
  font-weight: 700;
  background: var(--accent-muted);
  color: var(--accent);
  border: 1px solid var(--accent-border);
  transition: all var(--motion-fast) var(--easing-standard);
}

.detail__chart-toggle:hover {
  background: var(--accent);
  color: #ffffff;
}

.detail__chart-toggle--off {
  background: var(--btn-bg);
  color: var(--text-secondary);
  border-color: var(--border-default);
}

.detail__chart-toggle--off:hover {
  background: var(--btn-bg-hover);
  color: var(--text-primary);
}

.detail__chart-toggle-icon {
  width: 12px;
  height: 12px;
}

.detail__chart-body {
  flex: 1;
  padding: 0.5rem 0.85rem 0.85rem;
  min-height: 220px;
}

/* ----------------------------
   通道区域
   ---------------------------- */
.detail__channels {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
}

.detail__channels-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 1rem;
  padding: 0 0.25rem;
}

.detail__channels-title {
  font-size: var(--font-size-md);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}

.detail__channels-hint {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

/* 减少动画偏好 */
@media (prefers-reduced-motion: reduce) {
  .detail__empty-ring {
    animation: none;
    opacity: 0.3;
  }

  .detail__status-badge--acquiring .detail__status-dot,
  .detail__status-badge--connecting .detail__status-dot {
    animation: none;
  }

  .detail__action-icon--spin {
    animation: none;
  }

  .detail__empty-tip:hover {
    transform: none;
  }
}
</style>
