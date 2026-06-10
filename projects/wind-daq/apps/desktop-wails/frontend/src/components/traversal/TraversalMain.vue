/**
 * ============================================================================
 * 浜斿瓟鎺㈤拡绉讳綅娴嬭瘯涓荤敾闈?(FiveHoleTraversalMain)
 * ============================================================================
 *
 * 銆愬姛鑳藉畾浣嶃€?
 * 鐢ㄤ簬浜斿瓟鎺㈤拡鐨勬寮忛娲炲疄楠岋紝鎸夐璁捐建杩硅嚜鍔ㄩ亶鍘嗗涓偣浣嶏紝
 * 浣跨敤宸叉牎鍑嗙殑 PRB 绯绘暟杩涜瀹炴椂鎻掑€艰绠椼€?
 *
 * 銆愪娇鐢ㄥ満鏅€?
 * - 椋庢礊娴佸満娴嬮噺瀹為獙
 * - 浣跨敤宸叉牎鍑嗘帰閽堣繘琛屾寮忔祴璇?
 * - 瀹炴椂鑾峰彇娴佸満鍙傛暟锛堟敾瑙掋€佷晶婊戣銆侀┈璧暟绛夛級
 *
 * 銆愬叧閿壒寰併€?
 * - 闇€瑕侀鍏堝姞杞?PRB 鏍″噯鏂囦欢
 * - 鏀寔杩愯/鏆傚仠/鍋滄鎺у埗
 * - 鏄剧ず鐐逛綅棰勮鍥惧拰褰撳墠杩涘害
 * - 瀹炴椂鎻掑€艰绠楀苟鏄剧ず缁撴灉锛埼便€佄层€丮ach銆丳0銆丳s锛?
 * - 鏄剧ず鍘熷鍘嬪姏閫氶亾鏁版嵁锛圥1-P5銆丳atm銆乀atm锛?
 * - 鏀寔浠庝笂娆′腑鏂鎭㈠娴嬭瘯
 *
 * 銆愬墠缃潯浠躲€?
 * 蹇呴』鍏堝畬鎴愭帰閽堟爣瀹氾紒鏍″噯绯荤粺鍦細
 * @/components/calibration/five-hole/FiveHoleMain.vue
 *
 * @module FiveHoleTraversalMain
 * @see TraversalSettings.vue - 娴嬭瘯閰嶇疆
 * @see FiveHoleMain.vue - 鎺㈤拡鏍″噯绯荤粺
 * ============================================================================
 */
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import UiButton from '@components/ui/UiButton.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import { deviceApi } from '@api/deviceApi'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useMotionStore } from '@stores/motionStore'
import { useTraversalStore } from '@stores/traversalStore'
import { useI18nStore } from '@stores/i18nStore'
import type {
  TraversalInterpolationInput,
  TraversalTestConfig
} from '@shared/types/traversal'
import type { DataPayload } from '@api/types'
import PointsPreview from './PointsPreview.vue'
import ProbeReferenceCard from './ProbeReferenceCard.vue'
import TraversalVisualization from './visualization/TraversalVisualization.vue'
import { AlertTriangle, Activity, ClipboardList, Pause, Play, Settings, Square } from '@lucide/vue'
import IconTraversal from '@components/icons/IconTraversal.vue'

withDefaults(
  defineProps<{
    recovering?: boolean
  }>(),
  {
    recovering: false
  }
)

const emit = defineEmits<{
  openSettings: []
  back: []
}>()

const traversalStore = useTraversalStore()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()
const hasConfig = computed(() => traversalStore.config !== null)
const currentConfig = computed(() => traversalStore.config)

const i18n = useI18nStore()
const t = computed(() => i18n.t)

type WorkspaceTab = 'preview' | 'visualization' | 'reference'

const activeWorkspaceTab = ref<WorkspaceTab>('preview')

const workspaceTabs = computed<Array<{ value: WorkspaceTab; label: string }>>(() => [
  { value: 'preview', label: t.value.pointsPreview },
  { value: 'visualization', label: t.value.flowVisualization },
  { value: 'reference', label: t.value.probeReference }
])

type LivePressureKey = 'P1' | 'P2' | 'P3' | 'P4' | 'P5' | 'Patm' | 'Tatm'
type LivePressureMap = Partial<Record<LivePressureKey, number>>

const latestSnapshots = ref<DataPayload[]>([])
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null

onMounted(() => {
  void deviceStore.refreshInstances()
  unsubscribeDeviceStatus = deviceStore.attachStatusListener()

  void motionStore.refreshStatus()
  unsubscribeMotionStatus = motionStore.attachStatusListener()

  unsubscribeDaqSnapshot = deviceApi.onSnapshot((snapshot: DataPayload) => {
    const next = latestSnapshots.value.filter((item) => item.deviceId !== snapshot.deviceId)
    latestSnapshots.value = [...next, snapshot]
  })
})

onBeforeUnmount(() => {
  if (unsubscribeDaqSnapshot) {
    unsubscribeDaqSnapshot()
    unsubscribeDaqSnapshot = null
  }
  if (unsubscribeDeviceStatus) {
    unsubscribeDeviceStatus()
    unsubscribeDeviceStatus = null
  }
  if (unsubscribeMotionStatus) {
    unsubscribeMotionStatus()
    unsubscribeMotionStatus = null
  }
  if (realtimeThrottleTimer) {
    clearTimeout(realtimeThrottleTimer)
    realtimeThrottleTimer = null
  }
  traversalStore.reset()
})

function openSettings(): void {
  emit('openSettings')
}

async function startTest(): Promise<void> {
  if (!currentConfig.value) {
    feedbackStore.pushToast('Please configure traversal settings first.', 'warning')
    return
  }

  const preconditions = await traversalStore.checkPreconditions(currentConfig.value)
  if (!preconditions.allPassed) {
    const failed = preconditions.checks.filter((check) => !check.passed)
    feedbackStore.pushToast(
      `${t.value.preconditionsFailed}\n${failed.map((check) => `- ${check.message ?? check.name}`).join('\n')}`,
      'error',
      5000
    )
    return
  }

  try {
    await traversalStore.startTest(currentConfig.value)
  } catch (err) {
    console.error('Failed to start test:', err)
    feedbackStore.pushToast(
      'Failed to start traversal: ' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  }
}

async function pauseTest(): Promise<void> {
  try {
    await traversalStore.pause()
  } catch (err) {
    console.error('Failed to pause:', err)
  }
}

async function resumeTest(): Promise<void> {
  try {
    await traversalStore.resume()
  } catch (err) {
    console.error('Failed to resume:', err)
  }
}

async function stopTest(): Promise<void> {
  try {
    await traversalStore.stop()
  } catch (err) {
    console.error('Failed to stop:', err)
  }
}

const STATUS_CONFIG: Record<string, { text: string; dotClass: string }> = {
  running:   { text: 'RUNNING', dotClass: 'bg-emerald-500 shadow-[0_0_6px_#10b981]' },
  paused:    { text: 'PAUSED',  dotClass: 'bg-amber-500 animate-pulse' },
  completed: { text: 'DONE',    dotClass: 'bg-blue-500 shadow-[0_0_6px_#3b82f6]' },
  error:     { text: 'ERROR',   dotClass: 'bg-rose-500 shadow-[0_0_6px_#f43f5e]' },
  stopped:   { text: 'STOPPED', dotClass: 'bg-amber-500' },
  unknown:   { text: 'UNKNOWN', dotClass: 'bg-rose-500 shadow-[0_0_6px_#f43f5e]' },
  idle:      { text: 'IDLE',    dotClass: 'bg-slate-400' },
}

const statusConfig = computed(() => STATUS_CONFIG[traversalStore.statusType] ?? STATUS_CONFIG.idle)
const statusText = computed(() => statusConfig.value.text)
const statusDotClass = computed(() => statusConfig.value.dotClass)

const workspaceTabMeta = computed(() => {
  const tval = t.value
  const maps: Record<WorkspaceTab, { title: string; subtitle: string }> = {
    preview:       { title: tval.pointsPreview,     subtitle: tval.layoutTopology },
    visualization: { title: tval.flowVisualization, subtitle: tval.realtimeFlow },
    reference:     { title: tval.probeReference,    subtitle: tval.probeReferenceHint },
  }
  return maps[activeWorkspaceTab.value]
})

const progressSummary = computed(() => `${traversalStore.status?.completedPoints || 0} / ${traversalStore.status?.totalPoints || 0}`)

const currentPointSummary = computed(() => ({
  alpha: traversalStore.status?.currentPoint?.alpha?.toFixed(2) || '--',
  beta: traversalStore.status?.currentPoint?.beta?.toFixed(2) || '--'
}))

const prbLabel = computed(() => traversalStore.config?.prbFile?.fileName || 'No PRB')

const hasRealtimeResult = computed(() => traversalStore.realtimeResult !== null)

type PositionerConnectionState = 'connected' | 'disconnected' | 'unconfigured'

function buildConnectionDisplay(state: PositionerConnectionState, unconfiguredLabel: string) {
  const dotClassMap: Record<PositionerConnectionState, string> = {
    connected: 'bg-emerald-500 shadow-[0_0_8px_#10b981]',
    disconnected: 'bg-rose-500 shadow-[0_0_8px_#f43f5e]',
    unconfigured: 'bg-slate-400'
  }

  const textClassMap: Record<PositionerConnectionState, string> = {
    connected: 'text-emerald-600 dark:text-emerald-400',
    disconnected: 'text-rose-600 dark:text-rose-400',
    unconfigured: 'text-slate-500 dark:text-slate-400'
  }

  const labelMap: Record<PositionerConnectionState, string> = {
    connected: t.value.connected,
    disconnected: t.value.disconnected,
    unconfigured: unconfiguredLabel
  }

  return {
    state,
    label: labelMap[state],
    dotClass: dotClassMap[state],
    textClass: textClassMap[state]
  }
}

const positionerConnection = computed(() => {
  const controllerIds = Array.from(
    new Set(
      (currentConfig.value?.channels.motionAxes ?? [])
        .map((axis) => axis.controllerId?.trim())
        .filter((controllerId): controllerId is string => Boolean(controllerId))
    )
  )

  let state: PositionerConnectionState = 'unconfigured'
  if (controllerIds.length > 0) {
    state = controllerIds.every((controllerId) => motionStore.statusById(controllerId)?.connected)
      ? 'connected'
      : 'disconnected'
  }

  return buildConnectionDisplay(state, t.value.unconfigured)
})

const acquisitionConnection = computed(() => {
  const deviceIds = Array.from(
    new Set(
      (currentConfig.value?.channels.probeChannels ?? [])
        .filter((channel) => channel.enabled)
        .map((channel) => channel.channel.deviceId?.trim())
        .filter((deviceId): deviceId is string => Boolean(deviceId))
    )
  )

  let state: PositionerConnectionState = 'unconfigured'
  if (deviceIds.length > 0) {
    state = deviceIds.every((deviceId) => deviceStore.statusFor(deviceId) === 'Connected')
      ? 'connected'
      : 'disconnected'
  }

  return buildConnectionDisplay(state, t.value.unconfigured)
})

function buildRealtimePressuresFromSnapshots(
  config: TraversalTestConfig,
  snapshots: DataPayload[]
): LivePressureMap | null {
  const toValue = (deviceId: string, channelIndex: number): number | undefined => {
    const payload = snapshots.find((entry) => entry.deviceId === deviceId)
    if (!payload) return undefined
    const indices = Array.isArray(payload.channelIndices) ? payload.channelIndices : []
    const channels = Array.isArray(payload.channels) ? payload.channels : []
    const index = indices.indexOf(channelIndex)
    if (index < 0) return undefined
    const value = channels[index]
    return typeof value === 'number' ? value : undefined
  }

  const result: LivePressureMap = {}
  let matchedChannelCount = 0

  for (const channel of config.channels.probeChannels) {
    if (!channel.enabled || !channel.channel.deviceId) continue

    const value = toValue(channel.channel.deviceId, channel.channel.channelIndex)
    if (typeof value !== 'number') continue

    switch (channel.role) {
      case 'fiveHole.p1':
        result.P1 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p2':
        result.P2 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p3':
        result.P3 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p4':
        result.P4 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.p5':
        result.P5 = value
        matchedChannelCount += 1
        break
      case 'fiveHole.pAtm':
        result.Patm = value
        matchedChannelCount += 1
        break
      case 'fiveHole.tAtm':
        result.Tatm = value
        matchedChannelCount += 1
        break
      default:
        break
    }
  }

  return matchedChannelCount > 0 ? result : null
}

const livePressures = computed(() => {
  if (!currentConfig.value || latestSnapshots.value.length === 0) {
    return null
  }
  return buildRealtimePressuresFromSnapshots(currentConfig.value, latestSnapshots.value)
})

function toRealtimeInterpolationInput(pressures: LivePressureMap | null): TraversalInterpolationInput | null {
  if (!pressures) {
    return null
  }

  const { P1, P2, P3, P4, P5, Patm, Tatm } = pressures
  if (
    typeof P1 !== 'number'
    || typeof P2 !== 'number'
    || typeof P3 !== 'number'
    || typeof P4 !== 'number'
    || typeof P5 !== 'number'
    || typeof Patm !== 'number'
    || typeof Tatm !== 'number'
  ) {
    return null
  }

  return { P1, P2, P3, P4, P5, Patm, Tatm }
}

const liveInterpolationInput = computed(() => toRealtimeInterpolationInput(livePressures.value))

let realtimeThrottleTimer: ReturnType<typeof setTimeout> | null = null
let pendingRealtimeInput: TraversalInterpolationInput | null = null
let pendingRealtimeConfig: TraversalTestConfig | undefined
watch(
  [liveInterpolationInput, () => currentConfig.value?.prbFile?.filePath ?? null],
  ([input]) => {
    const hasDataset = Boolean(
      currentConfig.value?.prbFile ||
      (currentConfig.value?.useMultiPrb && currentConfig.value?.multiPrb?.files.length)
    )
    if (!input || !hasDataset) {
      pendingRealtimeInput = null
      pendingRealtimeConfig = undefined
      traversalStore.realtimeResult = null
      return
    }
    pendingRealtimeInput = input
    pendingRealtimeConfig = currentConfig.value ?? undefined
    if (realtimeThrottleTimer) return
    realtimeThrottleTimer = setTimeout(() => {
      realtimeThrottleTimer = null
      const nextInput = pendingRealtimeInput
      const nextConfig = pendingRealtimeConfig
      pendingRealtimeInput = null
      pendingRealtimeConfig = undefined
      if (nextInput) {
        traversalStore.requestRealtimeResult(nextInput, nextConfig)
      }
    }, traversalStore.uiRefreshIntervalMs)
  }
)

const pressureItems = computed(() => {
  const data = livePressures.value ?? traversalStore.realtimePressures
  const formatValue = (value?: number): string => (typeof value === 'number' ? value.toFixed(3) : '--')
  return [
    { key: 'P1', label: 'P1', unit: 'kPa', value: formatValue(data?.P1), disabled: !hasConfig.value },
    { key: 'P2', label: 'P2', unit: 'kPa', value: formatValue(data?.P2), disabled: !hasConfig.value },
    { key: 'P3', label: 'P3', unit: 'kPa', value: formatValue(data?.P3), disabled: !hasConfig.value },
    { key: 'P4', label: 'P4', unit: 'kPa', value: formatValue(data?.P4), disabled: !hasConfig.value },
    { key: 'P5', label: 'P5', unit: 'kPa', value: formatValue(data?.P5), disabled: !hasConfig.value },
    { key: 'Patm', label: 'Patm', unit: 'kPa', value: formatValue(data?.Patm), disabled: !hasConfig.value },
    { key: 'Tatm', label: 'Tatm', unit: 'C', value: formatValue(data?.Tatm), disabled: !hasConfig.value }
  ]
})

watch(
  () => traversalStore.completeEvent,
  (event) => {
    if (!event) return

    if (event.success) {
      const duration = event.duration ? (event.duration / 1000).toFixed(1) : '--'
      feedbackStore.pushToast(
        `${t.value.testCompleted}\n${t.value.filePath}: ${event.filePath}\n${t.value.duration}: ${duration}s\n${t.value.totalPoints}: ${event.totalPoints}`,
        'success',
        8000
      )
    } else {
      feedbackStore.pushToast(
        `${t.value.testFailed}: ${event.error || 'Unknown error'}\n${t.value.filePath}: ${event.filePath || '--'}`,
        'error',
        8000
      )
    }
  },
  { deep: true }
)
</script>

<template>
  <div class="flex h-full flex-col bg-slate-50/50 dark:bg-transparent text-[color:var(--text-primary)]">
    <!-- Top Toolbar -->
    <div data-test="traversal-top-toolbar" class="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4 py-2.5 dark:border-slate-700 dark:bg-slate-900">
      <!-- 左侧：标题区 -->
      <div class="flex items-center gap-3">
        <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-500 text-white shadow-sm">
          <IconTraversal :size="16" />
        </div>
        <div>
          <h1 class="text-sm font-bold text-slate-900 dark:text-slate-100">{{ t.fiveHoleTraversalTest }}</h1>
          <div class="flex items-center gap-2 mt-0.5">
            <span class="flex h-1.5 w-1.5 rounded-full" :class="statusDotClass"></span>
            <p class="text-[11px] text-slate-400">{{ statusText }} / {{ t.automatedRun }}</p>
          </div>
        </div>
      </div>

      <!-- 右侧：控制区 -->
      <div class="flex items-center gap-2">
        <!-- 状态信息组 -->
        <div class="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 dark:border-slate-700 dark:bg-slate-800/50">
          <div class="flex items-center gap-1.5">
            <span class="text-[11px] text-slate-400">{{ t.travProgress }}</span>
            <span class="font-mono text-xs font-semibold text-blue-500">{{ progressSummary }}</span>
          </div>
          <div class="h-3 w-px bg-slate-200 dark:bg-slate-600"></div>
          <div class="flex items-center gap-1.5">
            <span class="text-[11px] text-slate-400">{{ t.prb }}</span>
            <span class="max-w-[80px] truncate font-mono text-[11px] text-slate-600 dark:text-slate-300">{{ prbLabel }}</span>
          </div>
        </div>

        <!-- 操作按钮组 -->
        <UiButton
          @click="openSettings"
          quaternary size="sm"
        >
          <template #icon>
            <Settings class="h-3.5 w-3.5" />
          </template>
          {{ t.configBtn }}
        </UiButton>

        <div class="h-4 w-px bg-slate-200 dark:bg-slate-700"></div>

        <div class="flex items-center gap-1.5">
          <UiButton
            v-if="!recovering && traversalStore.canStart"
            variant="primary" size="sm"
            :disabled="!hasConfig"
            @click="startTest"
          >
            <template #icon>
              <Play class="h-3.5 w-3.5 fill-current" />
            </template>
            {{ t.startRun }}
          </UiButton>
          <template v-else-if="!recovering && traversalStore.canPause">
            <UiButton variant="warning" size="sm" @click="pauseTest">
              <template #icon>
                <Pause class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ t.travPause }}
            </UiButton>
            <UiButton variant="danger" size="sm" @click="stopTest">
              <template #icon>
                <Square class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ t.travStop }}
            </UiButton>
          </template>
          <template v-else-if="!recovering && traversalStore.canResume">
            <UiButton variant="primary" size="sm" @click="resumeTest">
              <template #icon>
                <Play class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ t.travResume }}
            </UiButton>
            <UiButton variant="danger" size="sm" @click="stopTest">
              <template #icon>
                <Square class="h-3.5 w-3.5 fill-current" />
              </template>
              {{ t.travStop }}
            </UiButton>
          </template>
        </div>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="recovering" class="flex flex-1 items-center justify-center p-6">
      <div class="flex flex-col items-center justify-center gap-4 text-center">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-blue-500 border-t-transparent"></div>
        <div class="text-sm font-black uppercase tracking-widest text-slate-500">{{ t.loadingWorkspace }}</div>
      </div>
    </div>

    <!-- Main Workspace -->
    <div class="flex-1 overflow-hidden p-5">
      <div class="grid h-full grid-cols-[300px_1fr] gap-4">
        <!-- Sidebar -->
        <aside class="flex min-h-0 flex-col gap-2 overflow-y-auto pr-1">
          <!-- 实时监测面板 -->
          <section data-test="traversal-sidebar-monitor" class="rounded-xl border border-slate-200 bg-white p-3 shadow-sm dark:border-slate-700 dark:bg-slate-900 flex-shrink-0">
            <!-- 面板头部 -->
            <div class="mb-2 flex items-center justify-between">
              <div class="flex items-center gap-1.5">
                <Activity class="h-3.5 w-3.5 text-blue-500" />
                <div>
                  <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-100">{{ t.travMonitor }}</h2>
                  <p class="text-[10px] text-slate-400">{{ t.realtimeCalculation }}</p>
                </div>
              </div>
              <div class="flex shrink-0 items-center gap-1.5">
                <div data-test="traversal-acquisition-indicator" class="flex items-center gap-1">
                  <span class="h-1.5 w-1.5 rounded-full" :class="acquisitionConnection.dotClass"></span>
                  <span class="text-[10px]" :class="acquisitionConnection.textClass">
                    {{ t.acquiring }}
                  </span>
                </div>
                <div class="h-2.5 w-px bg-slate-200 dark:bg-slate-700"></div>
                <div data-test="traversal-positioner-indicator" class="flex items-center gap-1">
                  <span class="h-1.5 w-1.5 rounded-full" :class="positionerConnection.dotClass"></span>
                  <span class="text-[10px]" :class="positionerConnection.textClass">
                    {{ t.positioner }}
                  </span>
                </div>
              </div>
            </div>

            <!-- 当前点位 -->
            <div class="mb-2 rounded-lg border border-slate-100 bg-slate-50/80 p-2 dark:border-slate-700/50 dark:bg-slate-800/50">
              <div class="mb-1 flex items-center gap-1.5">
                <div class="h-1 w-1 rounded-full bg-blue-400"></div>
                <span class="text-[10px] font-medium uppercase tracking-wider text-slate-400">{{ t.currentPoint }}</span>
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div class="flex flex-col">
                  <span class="text-[10px] text-slate-400">{{ t.alpha }}</span>
                  <span class="font-mono text-sm font-semibold text-slate-700 dark:text-slate-200">{{ currentPointSummary.alpha }}°</span>
                </div>
                <div class="flex flex-col">
                  <span class="text-[10px] text-slate-400">{{ t.beta }}</span>
                  <span class="font-mono text-sm font-semibold text-slate-700 dark:text-slate-200">{{ currentPointSummary.beta }}°</span>
                </div>
              </div>
            </div>

            <!-- 气动参数网格 -->
            <div class="space-y-1">
              <div class="grid grid-cols-2 gap-1">
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-[10px] text-slate-400">{{ t.alpha }}</span>
                  <span class="font-mono text-sm font-semibold text-blue-600">{{ traversalStore.realtimeResult?.alpha?.toFixed(2) ?? '--' }}°</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-[10px] text-slate-400">{{ t.beta }}</span>
                  <span class="font-mono text-sm font-semibold text-slate-700 dark:text-slate-200">{{ traversalStore.realtimeResult?.beta?.toFixed(2) ?? '--' }}°</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-[10px] text-slate-400">{{ t.mach }}</span>
                  <span class="font-mono text-sm font-semibold text-blue-600">{{ traversalStore.realtimeResult?.machNumber?.toFixed(3) ?? '--' }}</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-[10px] text-slate-400">{{ t.velocity }}</span>
                  <span class="font-mono text-sm font-semibold text-slate-700 dark:text-slate-200">{{ traversalStore.realtimeResult?.velocity?.toFixed(1) ?? '--' }}</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-[10px] text-slate-400">P0</span>
                  <span class="font-mono text-sm font-semibold text-blue-600">{{ traversalStore.realtimeResult?.P0?.toFixed(2) ?? '--' }}</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-[10px] text-slate-400">Ps</span>
                  <span class="font-mono text-sm font-semibold text-blue-600">{{ traversalStore.realtimeResult?.Ps?.toFixed(2) ?? '--' }}</span>
                </div>
              </div>
              <!-- 有效性状态 -->
              <div class="flex items-center justify-between rounded-md border px-2.5 py-1.5" :class="hasRealtimeResult && traversalStore.realtimeResult?.isValid ? 'border-emerald-200 bg-emerald-50/60 dark:border-emerald-800/50 dark:bg-emerald-900/15' : 'border-slate-100 bg-slate-50/60 dark:border-slate-700/50 dark:bg-slate-800/60'">
                <div class="flex items-center gap-1.5">
                  <span class="h-1.5 w-1.5 rounded-full" :class="hasRealtimeResult && traversalStore.realtimeResult?.isValid ? 'bg-emerald-500' : hasRealtimeResult ? 'bg-rose-500' : 'bg-slate-300'"></span>
                  <span class="text-xs text-slate-500">{{ t.validity }}</span>
                </div>
                <span class="font-mono text-xs font-semibold" :class="hasRealtimeResult && traversalStore.realtimeResult?.isValid ? 'text-emerald-600 dark:text-emerald-400' : hasRealtimeResult ? 'text-rose-600 dark:text-rose-400' : 'text-slate-400'">
                  {{ hasRealtimeResult ? (traversalStore.realtimeResult?.isValid ? t.valid : t.limit) : '--' }}
                </span>
              </div>
            </div>
          </section>

          <!-- 实时压力数据面板 -->
          <section data-test="traversal-sidebar-pressure" class="rounded-xl border border-slate-200 bg-white p-3 shadow-sm dark:border-slate-700 dark:bg-slate-900 flex-shrink-0">
            <div class="mb-2 flex items-center gap-1.5">
              <ClipboardList class="h-3.5 w-3.5 text-slate-400" />
              <div>
                <h3 class="text-sm font-semibold text-slate-800 dark:text-slate-100">{{ t.realtimePressureData }}</h3>
                <p class="text-[10px] text-slate-400">{{ t.rawChannelFeed }}</p>
              </div>
            </div>

            <div class="grid grid-cols-2 gap-1.5">
              <div
                v-for="item in pressureItems"
                :key="item.key"
                class="flex flex-col rounded-md border px-2 py-1.5 transition-colors"
                :class="item.disabled
                  ? 'border-slate-100 bg-slate-50/40 dark:border-slate-700/50 dark:bg-slate-800/40'
                  : 'border-blue-100 bg-blue-50/20 dark:border-blue-800/30 dark:bg-blue-900/10'"
              >
                <div class="flex items-center justify-between">
                  <span class="text-[10px] font-medium" :class="item.disabled ? 'text-slate-400' : 'text-slate-500 dark:text-slate-400'">{{ item.label }}</span>
                  <span v-if="!item.disabled" class="text-[9px] text-slate-300 dark:text-slate-600">{{ item.unit }}</span>
                </div>
                <div class="font-mono text-sm font-semibold" :class="item.disabled ? 'text-slate-300 dark:text-slate-600' : 'text-slate-700 dark:text-slate-200'">{{ item.value }}</div>
              </div>
            </div>
          </section>
        </aside>

        <!-- Main Content Area -->
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div data-test="traversal-workspace-primary" class="min-h-0 flex-1">
            <section class="h-full flex flex-col rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
              <!-- 工作区头部 -->
              <div class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 px-4 py-3 dark:border-slate-800">
                <div class="flex items-center gap-2">
                  <div class="h-6 w-1 rounded-full bg-blue-500"></div>
                  <div>
                  <h3 class="text-sm font-semibold text-slate-800 dark:text-slate-100">
                      {{ workspaceTabMeta.title }}
                    </h3>
                    <p class="text-[10px] text-slate-400">
                      {{ workspaceTabMeta.subtitle }}
                    </p>
                  </div>
                </div>
                <!-- 标签页切换 -->
                <div class="flex rounded-lg border border-slate-200 bg-slate-50 p-0.5 dark:border-slate-700 dark:bg-slate-800">
                  <UiButton
                    v-for="tab in workspaceTabs"
                    :key="tab.value"
                    quaternary size="sm"
                    :class="activeWorkspaceTab === tab.value ? 'bg-blue-500 text-white shadow-sm' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-200'"
                    @click="activeWorkspaceTab = tab.value"
                  >
                    {{ tab.label }}
                  </UiButton>
                </div>
              </div>

              <!-- 内容区 -->
              <div class="flex-1 overflow-hidden relative">
                <!-- 点位预览图例 -->
                <div v-if="activeWorkspaceTab === 'preview'" class="absolute right-4 top-3 z-10 flex items-center gap-3 rounded-full border border-slate-200 bg-white/90 px-3 py-1.5 text-[10px] shadow-sm backdrop-blur dark:border-slate-700 dark:bg-slate-900/90">
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-blue-500"></span>
                    <span class="text-slate-500">{{ t.moving }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-amber-400"></span>
                    <span class="text-slate-500">{{ t.stabilizing }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-emerald-500"></span>
                    <span class="text-slate-500">{{ t.acquiring }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-gradient-to-r from-purple-500 to-pink-500"></span>
                    <span class="text-slate-500">{{ t.completed }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-slate-300"></span>
                    <span class="text-slate-500">{{ t.untested }}</span>
                  </div>
                </div>

                <template v-if="activeWorkspaceTab === 'preview'">
                  <PointsPreview
                    v-if="currentConfig?.layout"
                    :layout="currentConfig.layout"
                    :current-point="traversalStore.status?.currentPoint"
                    :completed-points="traversalStore.status?.completedPoints"
                    :current-point-phase="traversalStore.status?.currentPointPhase"
                  />
                  <!-- 空状态 -->
                  <div v-else class="flex h-full w-full flex-col items-center justify-center gap-4 text-center">
                    <div class="flex h-16 w-16 items-center justify-center rounded-2xl bg-slate-100 dark:bg-slate-800">
                      <ClipboardList class="h-8 w-8 text-slate-300 dark:text-slate-600" />
                    </div>
                    <div>
                      <div class="text-sm font-medium text-slate-500 dark:text-slate-400">{{ t.noLayoutConfigured }}</div>
                      <div class="mt-1 text-xs text-slate-400">请先配置测试点位布局以开始移位测试</div>
                    </div>
                    <UiButton
                      variant="primary" size="sm"
                      @click="openSettings"
                    >
                      <template #icon>
                        <Settings class="h-3.5 w-3.5" />
                      </template>
                      {{ t.configureLayout }}
                    </UiButton>
                  </div>
                </template>
                <div v-else-if="activeWorkspaceTab === 'visualization'" class="h-full p-4">
                  <TraversalVisualization />
                </div>
                <div v-else class="h-full overflow-auto p-4">
                  <ProbeReferenceCard />
                </div>
              </div>
            </section>
          </div>
        </div>
      </div>
    </div>

    <!-- Error Banner -->
    <div v-if="traversalStore.error" class="shrink-0 border-t border-rose-200 bg-rose-50 px-6 py-3 dark:border-rose-900/30 dark:bg-rose-900/10">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-3 text-sm font-medium text-rose-600 dark:text-rose-400">
          <AlertTriangle class="h-4 w-4" />
          <span>{{ traversalStore.error }}</span>
        </div>
        <UiButton variant="danger" size="sm" @click="traversalStore.clearError">{{ t.dismiss }}</UiButton>
      </div>
    </div>
  </div>
</template>

