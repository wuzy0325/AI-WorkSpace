<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import UiButton from '@components/ui/UiButton.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiSectionHeader from '@components/ui/UiSectionHeader.vue'
import { isCalibratableDeviceType, isTemperatureUnit } from '@utils/deviceCalibration'
import { channelUnit } from '@utils/channelUnit'

const deviceStore = useDeviceStore()
const i18n = useI18nStore()
const feedbackStore = useFeedbackStore()
const calibratingAll = ref(false)
/** calibrateAllDevices 启动时记录的正在校零的目标设备 ID 列表，
 *  供 cancelAllCalibrations 取消使用；清空表示当前没有"全部校零"在跑。 */
const calibratingAllIds = ref<Set<string>>(new Set())

const DEVICE_COLORS = [
  { text: 'text-emerald-400', borderLeft: 'border-l-emerald-500' },
  { text: 'text-sky-400', borderLeft: 'border-l-sky-500' },
  { text: 'text-violet-400', borderLeft: 'border-l-violet-500' },
  { text: 'text-amber-400', borderLeft: 'border-l-amber-500' },
]

function getDeviceTheme(index: number) {
  return DEVICE_COLORS[index % DEVICE_COLORS.length]
}

interface OverviewChannelItem {
  key: string
  channelIndex: number
  label: string
  name: string
  formattedValue: string
  unit: string
  tone: 'active' | 'warning'
}

interface OverviewDeviceGroup {
  id: string
  name: string
  type: string
  status: string
  statusLabel: string
  channelCount: number
  warningCount: number
  theme: { text: string; borderLeft: string }
  channels: OverviewChannelItem[]
  /** 该设备是否正在校零（用于在分组头部展示进度） */
  calibrating: boolean
  /** 校零进度文本，如 "3/5s · 120 样本"；非校零中为空字符串 */
  calibrationProgressText: string
}

function channelDisplayName(deviceId: string, channelIndex: number): string {
  const profile = deviceStore.profiles?.find((item) => item.id === deviceId)
  const channels = Array.isArray(profile?.channels) ? profile.channels : []
  const name = channels[channelIndex]?.name?.trim()
  if (!name) return `CH${channelIndex + 1}`
  return name
}

function channelTone(deviceId: string, channelIndex: number, rawValue: number | null): 'active' | 'warning' {
  const status = deviceStore.statusFor(deviceId)
  if (status === 'Error' || status === 'Disconnected') return 'warning'
  // null/NaN（无有效测量值，如 T1602 未接入通道）不参与越限判断，保持中性色
  if (rawValue === null || Number.isNaN(rawValue)) return 'active'

  const range = deviceStore.getChannelRange(deviceId, channelIndex)
  const span = range.max - range.min
  const upper = range.max - span * 0.12
  const lower = range.min + span * 0.12
  return rawValue >= upper || rawValue <= lower ? 'warning' : 'active'
}

/**
 * 判定设备 profile 是否为"支持校零"的设备：
 *   1. 设备类型在白名单内（压力类设备）
 *   2. 至少存在一个非温度通道（温度通道无法校零）
 *
 * 抽出到共享常量（@utils/deviceCalibration）避免多份白名单维护漂移。
 */
function isCalibratableProfile(profile: { type: string; channels: { sensorType?: string; unit?: string }[] }): boolean {
  if (!isCalibratableDeviceType(profile.type)) return false
  return profile.channels.some((channel) => channel.sensorType !== 'temperature' && !isTemperatureUnit(channel.unit ?? ''))
}

async function calibrateAllDevices(): Promise<void> {
  // P1-9：区分"没有支持校零的设备"与"未开始采集"两种空结果场景，
  //       给出不同的提示文案，避免用户误以为按钮坏了。
  const allProfiles = deviceStore.profiles ?? []
  const calibratableProfiles = allProfiles.filter((profile) =>
    isCalibratableProfile({
      type: profile.type,
      channels: Array.isArray(profile.channels) ? profile.channels : [],
    }),
  )
  if (calibratableProfiles.length === 0) {
    feedbackStore.pushToast(i18n.t.noCalibratableDevice || '没有支持校零的设备', 'warning')
    return
  }
  const targets = calibratableProfiles.filter((profile) => deviceStore.acquiringFor(profile.id))
  if (targets.length === 0) {
    feedbackStore.pushToast(i18n.t.pleaseStartAcquisitionFirst || '请先开始采集', 'warning')
    return
  }
  calibratingAll.value = true
  calibratingAllIds.value = new Set(targets.map((p) => p.id))
  try {
    const results = await Promise.allSettled(targets.map((profile) => deviceStore.calibrate(profile.id)))
    const failed = results.filter((result) => result.status === 'rejected').length
    if (failed === 0) {
      feedbackStore.pushToast(i18n.t.tareAllDevicesComplete || '全部设备校零完成', 'success')
    } else {
      // 模板替换：t 是 Record<string, string>，没有内建插值，手动 replace。
      const msg = (i18n.t.tareDeviceFailed || '{success} 台成功，{failed} 台失败')
        .replace('{success}', String(targets.length - failed))
        .replace('{failed}', String(failed))
      feedbackStore.pushToast(msg, 'warning')
    }
  } finally {
    calibratingAll.value = false
    calibratingAllIds.value = new Set()
  }
}

/** 取消"全部校零"：逐个调用 store 的取消接口，触发每台设备的 AbortController.abort()。
 *  Promise.allSettled 不会因取消而立即 resolve，但每台设备的 calibrate 内部
 *  会捕获 AbortError 并标记为 cancelled 状态，UI 通过 calibrationOperations 感知。 */
function cancelAllCalibrations(): void {
  calibratingAllIds.value.forEach((id) => deviceStore.cancelCalibration(id))
}

function deviceStatusTone(profileId: string): 'healthy' | 'warning' {
  const status = deviceStore.statusFor(profileId)
  if (status === 'Error' || status === 'Disconnected') return 'warning'
  return 'healthy'
}

function deviceStatusLabel(profileId: string): string {
  if (deviceStore.acquiringFor(profileId)) return i18n.t.acquiring || '采集中'
  const status = deviceStore.statusFor(profileId)
  if (status === 'Connected') return i18n.t.connectedState || 'Connected'
  if (status === 'Connecting') return i18n.t.connectingState || 'Connecting'
  if (status === 'Error') return i18n.t.warningState || 'Warning'
  return i18n.t.disconnectedState || 'Disconnected'
}

const overviewGroups = computed<OverviewDeviceGroup[]>(() =>
  (deviceStore.profiles ?? []).flatMap((profile, index) => {
    const latest = deviceStore.latestFor(profile.id)
    const indices = Array.isArray(latest?.channelIndices) ? latest.channelIndices : []
    const values = Array.isArray(latest?.channels) ? latest.channels : []
    if (!values.length) return []

    const channels = values.map((rawValue, snapshotIndex) => {
      const channelIndex = indices[snapshotIndex]
      return {
        key: `${profile.id}-${channelIndex}`,
        channelIndex,
        label: `CH_${String(channelIndex + 1).padStart(2, '0')}`,
        name: channelDisplayName(profile.id, channelIndex),
        formattedValue: deviceStore.formatValue(profile.id, channelIndex, rawValue),
        unit: channelUnit(
          profile.type,
          channelIndex,
          (Array.isArray(profile.channels) ? profile.channels[channelIndex]?.unit : undefined) || (i18n.t.unit ?? 'PA'),
        ),
        tone: channelTone(profile.id, channelIndex, rawValue),
      }
    })

    // P1-8：单设备校零进度，从 store 的 calibrationOperations Map 读取
    const op = deviceStore.calibrationOperationFor(profile.id)
    const calibrating = op?.state === 'running'
    const calibrationProgressText = calibrating
      ? `${op!.elapsedSeconds}/${deviceStore.calibrationDurationSec}s · ${op!.sampleCount} ${i18n.t.samples || '样本'}`
      : ''

    return [
      {
        id: profile.id,
        name: profile.name,
        type: profile.type,
        status: deviceStore.statusFor(profile.id),
        statusLabel: deviceStatusLabel(profile.id),
        channelCount: channels.length,
        warningCount: channels.filter((ch) => ch.tone === 'warning').length,
        theme: getDeviceTheme(index),
        channels,
        calibrating,
        calibrationProgressText,
      },
    ]
  }),
)
</script>

<template>
  <UiPanel class="overview-panel h-full" :padded="false">
    <template #header>
      <div class="overview-panel__header-row flex min-w-full items-start justify-between gap-4">
        <UiSectionHeader :title="i18n.t.allDevicesOverview || '设备总览'" />
        <div class="flex items-center gap-2">
          <UiButton
            v-if="calibratingAll"
            variant="danger"
            size="sm"
            class="overview-panel__action-btn"
            @click="cancelAllCalibrations"
          >
            {{ i18n.t.cancelTare || '取消校零' }}
          </UiButton>
          <UiButton
            variant="secondary"
            size="sm"
            class="overview-panel__action-btn"
            :disabled="calibratingAll"
            @click="calibrateAllDevices"
          >
            {{ calibratingAll ? (i18n.t.tareInProgress || '校零中...') : (i18n.t.allDevicesTare || '全部校零') }}
          </UiButton>
        </div>
      </div>
    </template>

    <div class="device-overview">
      <div class="device-overview__stack">
        <section
          v-for="group in overviewGroups"
          :key="group.id"
          class="overview-device-group border-l-4"
          :class="group.theme.borderLeft"
        >
          <header class="overview-device-group__header">
            <div class="min-w-0">
              <div class="overview-device-group__eyebrow">{{ group.type }}</div>
              <div class="overview-device-group__title-row">
                <strong class="overview-device-group__title">{{ group.name }}</strong>
                <span class="overview-device-group__count">{{ group.channelCount }} CH</span>
                <span
                  class="overview-device-group__status"
                  :class="deviceStatusTone(group.id) === 'warning' ? 'overview-device-group__status--warning' : 'overview-device-group__status--healthy'"
                >
                  {{ group.statusLabel }}
                </span>
                <!-- P1-8：单设备校零进度徽章，仅在该设备校零中显示 -->
                <span
                  v-if="group.calibrating"
                  class="overview-device-group__calib"
                  :title="i18n.t.calibrationInProgress || '校零正在进行中'"
                >
                  {{ group.calibrationProgressText }}
                </span>
              </div>
            </div>
            <div class="overview-device-group__summary">
              <span class="overview-device-group__summary-label">{{ i18n.t.warningState || 'Warning' }}</span>
              <span class="overview-device-group__summary-value" :class="group.warningCount > 0 ? 'text-amber-500' : group.theme.text">
                {{ group.warningCount }}
              </span>
            </div>
          </header>

          <div class="overview-device-group__channels">
            <div
              v-for="channel in group.channels"
              :key="channel.key"
              class="overview-channel-micro"
              :class="channel.tone === 'warning' ? 'overview-channel-micro--warning' : ''"
              :title="`${group.name} - ${channel.name}`"
            >
              <div class="overview-channel-micro__main">
                <span
                  class="overview-channel-micro__value"
                  :class="channel.tone === 'warning' ? 'text-amber-500' : group.theme.text"
                >
                  {{ channel.formattedValue }}
                </span>
                <span class="overview-channel-micro__unit">{{ channel.unit }}</span>
              </div>
              <div class="overview-channel-micro__meta">
                <span class="overview-channel-micro__ch">{{ channel.label }}</span>
                <span class="overview-channel-micro__dot" :class="channel.tone === 'warning' ? 'bg-amber-500' : 'bg-emerald-500'" />
              </div>
            </div>
          </div>
        </section>
      </div>

      <div v-if="overviewGroups.length === 0" class="overview-empty">
        <p>{{ i18n.t.noConnectedDevices || '暂无设备数据' }}</p>
      </div>
    </div>
  </UiPanel>
</template>

<style scoped>
.overview-panel__header-row {
  padding-inline: var(--space-3);
}

.device-overview {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding-inline: var(--space-3);
  padding-bottom: var(--space-2);
}

.device-overview__stack {
  display: grid;
  gap: var(--space-4);
}

.overview-device-group {
  border-radius: var(--radius-lg, 0.75rem);
  border: 1px solid color-mix(in srgb, var(--border-default) 40%, transparent);
  background: color-mix(in srgb, var(--bg-panel-strong) 40%, transparent);
  padding: var(--space-2-5) var(--space-3) var(--space-3);
}

:root[data-theme='light'] .overview-device-group {
  background: color-mix(in srgb, var(--bg-panel-strong) 50%, transparent);
  border: 1px solid color-mix(in srgb, var(--border-default) 30%, transparent);
}

.overview-device-group__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-2);
  margin-bottom: var(--space-2);
}

.overview-device-group__eyebrow {
  font-size: var(--font-size-micro);
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-muted);
}

.overview-device-group__title-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-1);
  margin-top: var(--space-1);
}

.overview-device-group__title {
  font-size: var(--font-size-sm);
  font-weight: 700;
  color: var(--text-primary);
}

:root[data-theme='light'] .overview-device-group__title {
  color: var(--text-primary);
}

.overview-device-group__count,
.overview-device-group__status {
  display: inline-flex;
  align-items: center;
  min-height: 1.1rem;
  padding: 0 0.35rem;
  border-radius: 999px;
  font-size: var(--font-size-2xs);
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.overview-device-group__count {
  border: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
  background: var(--bg-canvas);
  color: var(--text-tertiary);
}

:root[data-theme='light'] .overview-device-group__count {
  border: 1px solid color-mix(in srgb, var(--border-default) 40%, transparent);
  background: color-mix(in srgb, var(--bg-panel) 20%, transparent);
  color: var(--text-muted);
}

.overview-device-group__status--healthy {
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
  color: var(--accent-primary);
}

.overview-device-group__status--warning {
  background: color-mix(in srgb, var(--accent-warning) 12%, transparent);
  color: var(--accent-warning);
}

/* P1-8：单设备校零进度徽章，使用主色弱化背景与等宽字体让进度数字对齐 */
.overview-device-group__calib {
  display: inline-flex;
  align-items: center;
  min-height: 1.1rem;
  padding: 0 0.4rem;
  border-radius: 999px;
  font-family: ui-monospace, monospace;
  font-size: var(--font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.02em;
  background: color-mix(in srgb, var(--accent-primary) 14%, transparent);
  color: var(--accent-primary);
  white-space: nowrap;
}

.overview-device-group__summary {
  display: grid;
  justify-items: end;
  gap: 0.05rem;
  min-width: 2.5rem;
}

.overview-device-group__summary-label {
  font-size: var(--font-size-micro);
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-muted);
}

.overview-device-group__summary-value {
  font-family: ui-monospace, monospace;
  font-size: var(--font-size-sm);
  font-weight: 700;
  line-height: 1;
}

/* 通道卡片布局：优先横向填充，保持阅读顺序 */
.overview-device-group__channels {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  gap: var(--space-2);
}

.overview-channel-micro {
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
  border: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
  border-radius: var(--radius-md, 0.6rem);
  padding: var(--space-2) var(--space-2-5);
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--space-1);
  min-height: 0;
  position: relative;
  overflow: hidden;
}

:root[data-theme='light'] .overview-channel-micro {
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
  border: 1px solid color-mix(in srgb, var(--border-default) 40%, transparent);
}

/* 仅在警告状态的卡片上显示顶部指示线，正常状态保持简洁 */
.overview-channel-micro--warning::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 2px;
  background: linear-gradient(90deg, transparent, var(--accent-warning), transparent);
  opacity: 0.6;
}

.overview-channel-micro--warning {
  border-color: color-mix(in srgb, var(--accent-warning) 30%, transparent);
  background: color-mix(in srgb, var(--accent-warning) 8%, transparent);
}

.overview-channel-micro__main {
  display: flex;
  align-items: baseline;
  gap: var(--space-1);
  line-height: 1;
}

.overview-channel-micro__value {
  font-family: ui-monospace, monospace;
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-black);
  letter-spacing: -0.02em;
}

/* 移除发光文字效果，保持清晰可读 */

.overview-channel-micro__unit {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-muted);
  font-style: italic;
  text-transform: uppercase;
}

.overview-channel-micro__meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-1);
}

.overview-channel-micro__ch {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.04em;
}

.overview-channel-micro__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.overview-empty {
  display: flex;
  height: 16rem;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
}
</style>
