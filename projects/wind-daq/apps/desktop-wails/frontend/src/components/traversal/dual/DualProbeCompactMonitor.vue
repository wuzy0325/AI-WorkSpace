/**
 * ============================================================================
 * DualProbeCompactMonitor — 单 probe 紧凑监测面板（spec FR7 / Task 20）
 * ============================================================================
 *
 * 【功能定位】
 * 双探针模式下每个 row 内的紧凑监测面板，仅保留 spec FR7 列出的紧凑字段：
 *   - 一行运行状态：状态、进度、当前点位、实际位置（轴运动中绿色高亮）
 *   - 实时插值：Alpha/Beta、总压、静压、速度 + 插值状态条（四态，与单探针同构）
 *   - Warning/Error 摘要
 *   - 独立启动、暂停、恢复、停止、设置入口
 *
 * 完整通道值、运动状态、点位预览、诊断信息放在该 row 的 Tab 详情区
 * （DualProbeRow.vue 内）。
 *
 * 【设计要点】
 * - 控制栏与 Warning 固定不滚动（spec FR7），避免长错误文本遮挡关键控制；
 * - 按钮按该 probe 状态启用/禁用：未配置/未加载 PRB 时禁用启动；
 *   运行中禁用启动/恢复，启用暂停/停止；暂停时禁用暂停，启用恢复。
 *
 * @module DualProbeCompactMonitor
 * ============================================================================
 */
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { Activity, AlertTriangle } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import { useDualTraversalStore } from '@stores/dualTraversalStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { useMotionStore } from '@stores/motionStore'
import { useHardwareConnectionStatus } from '@composables/useHardwareConnectionStatus'
import { traversalSessionWarning } from '@api/traversalErrorMapper'
import { TRAVERSAL_PROBE_PRESENTATION } from '@shared/types/traversal'
import type { ProbeId, TraversalSessionState, TraversalTestStatusType } from '@shared/types/traversal'

const props = defineProps<{
  probeId: ProbeId
}>()

const emit = defineEmits<{
  /** 打开该 probe 的配置对话框 */
  openSettings: [probeId: ProbeId]
}>()

const dualStore = useDualTraversalStore()
const i18n = useI18nStore()
const feedbackStore = useFeedbackStore()
const { sessions } = storeToRefs(dualStore)
const t = computed(() => i18n.t)

const session = computed<TraversalSessionState>(() => sessions.value[props.probeId])

const probePresentation = computed(() =>
  TRAVERSAL_PROBE_PRESENTATION[session.value.config?.probeType ?? 'five-hole']
)
const alphaLabel = computed(() =>
  (t.value as Record<string, string>)[probePresentation.value.alphaLabelKey] ?? probePresentation.value.alphaLabelKey
)
const betaLabel = computed(() =>
  (t.value as Record<string, string>)[probePresentation.value.betaLabelKey] ?? probePresentation.value.betaLabelKey
)

// 状态文本
const statusText = computed(() => {
  if (!session.value.config) return t.value.dualProbeUnconfigured
  const status = session.value.status?.status as TraversalTestStatusType | undefined
  if (!status) return t.value.dualProbeIdle
  return (t.value as Record<string, string>)[status as string] ?? (status as string)
})

// 状态点颜色：与单探针 TraversalMain 的 statusColorToken 映射保持一致
const statusDotColor = computed(() => {
  switch (session.value.status?.status) {
    case 'running':
      return 'var(--accent-success)'
    case 'paused':
    case 'stopped':
      return 'var(--accent-warning)'
    case 'completed':
      return 'var(--accent-info)'
    case 'error':
      return 'var(--accent-danger)'
    default:
      return 'var(--text-muted)'
  }
})

// 按钮启用状态：未配置时全部禁用，仅允许打开设置
const isUnconfigured = computed(() => !session.value.config)
const isRunning = computed(() => session.value.status?.status === 'running')
const isPaused = computed(() => session.value.status?.status === 'paused')
const isTerminal = computed(() => {
  const s = session.value.status?.status
  return s === 'completed' || s === 'stopped' || s === 'error'
})
const isStarting = computed(() => session.value.isStarting)
const isCheckpointPending = computed(() => dualStore.checkpointPending[props.probeId])
const canStart = computed(() => !isUnconfigured.value && !isRunning.value && !isPaused.value && !isStarting.value && !session.value.checkpoint && session.value.hasLoadedInterpolator)
const canPause = computed(() => isRunning.value)
const canResume = computed(() => isPaused.value)
const canStop = computed(() => isRunning.value || isPaused.value)

// 进度
const progressPercent = computed(() => {
  const status = session.value.status
  if (!status || status.totalPoints === 0) return 0
  return Math.min(100, Math.round((status.completedPoints / status.totalPoints) * 100))
})
const progressText = computed(() => {
  const status = session.value.status
  if (!status) return '—'
  return `${status.completedPoints} / ${status.totalPoints}`
})

// 当前点位
const currentPointText = computed(() => {
  const point = session.value.status?.currentPoint
  if (!point) return '—'
  const parts: string[] = []
  if (point.alpha !== null && point.alpha !== undefined) parts.push(`X=${point.alpha.toFixed(2)}`)
  if (point.beta !== null && point.beta !== undefined) parts.push(`Y=${point.beta.toFixed(2)}`)
  if (point.z !== null && point.z !== undefined) parts.push(`Z=${point.z.toFixed(2)}`)
  if (point.u !== null && point.u !== undefined) parts.push(`U=${point.u.toFixed(2)}`)
  return parts.length > 0 ? parts.join('  ') : '—'
})

// 实际位置：与单探针 TraversalLiveMonitor 同源，复用 useHardwareConnectionStatus
// （按本 probe 配置的布点模式过滤显示轴）。dual 页面级没有订阅 motion 状态流，
// 由本组件自行挂接/卸载；两个 probe 各挂一个监听器，写入同一 motionStore，互不干扰。
const motionStore = useMotionStore()
const currentConfig = computed(() => session.value.config)
const { axisPositions } = useHardwareConnectionStatus(currentConfig)

let unsubscribeMotionStatus: (() => void) | null = null
onMounted(() => {
  void motionStore.refreshStatus()
  unsubscribeMotionStatus = motionStore.attachStatusListener()
})
onBeforeUnmount(() => {
  if (unsubscribeMotionStatus) {
    unsubscribeMotionStatus()
    unsubscribeMotionStatus = null
  }
})

// 实际位置文本：与「当前点位」同款 `X=12.34  Y=-5.01` 格式，跟随目标点显示在其后
const actualPositionText = computed(() => {
  const axes = axisPositions.value
  if (!axes.length) return '—'
  return axes
    .map((axis) => `${axis.label}=${typeof axis.position === 'number' ? axis.position.toFixed(2) : '--'}`)
    .join('  ')
})
// 任一轴运动中 → 实际位置整行高亮（单探针同款 accent-success 语义）
const anyAxisMoving = computed(() => axisPositions.value.some((axis) => axis.moving))

// 实时插值结果
const realtimeResult = computed(() => session.value.realtimeResult)
const alphaText = computed(() => {
  const r = realtimeResult.value
  if (!r || !r.isValid) return '—'
  return r.alpha.toFixed(2) + '°'
})
const betaText = computed(() => {
  const r = realtimeResult.value
  if (!r || !r.isValid) return '—'
  return r.beta.toFixed(2) + '°'
})
const totalPressureText = computed(() => {
  const r = realtimeResult.value
  if (!r || !r.isValid || r.P0 === undefined) return '—'
  return r.P0.toFixed(2)
})
const staticPressureText = computed(() => {
  const r = realtimeResult.value
  if (!r || !r.isValid || r.Ps === undefined) return '—'
  return r.Ps.toFixed(2)
})
const velocityText = computed(() => {
  const r = realtimeResult.value
  if (!r || !r.isValid) return '—'
  return r.velocity.toFixed(2)
})

/**
 * 实时插值四态判定：与单探针 TraversalLiveMonitor 完全同构。
 *   - isValid=true                          → ok，正常显示数值
 *   - realtimeResult=null + 已加载           → no-data，蓝色"等待采集数据"
 *   - realtimeResult=null + 未加载           → prb-missing，橙色，点击打开配置导入 PRB
 *   - isValid=false + hasLoadedInterpolator  → invalid，红色，tooltip 显示后端 warning
 *   - isValid=false + !hasLoadedInterpolator → prb-missing，橙色，点击打开配置
 * 真相源为 session.hasLoadedInterpolator（后端校验失败时 dualStore 同步置 false）。
 */
type InterpStatus = 'ok' | 'no-data' | 'prb-missing' | 'invalid'
const interpStatus = computed<InterpStatus>(() => {
  const r = realtimeResult.value
  if (r) {
    if (r.isValid) return 'ok'
    return session.value.hasLoadedInterpolator ? 'invalid' : 'prb-missing'
  }
  return session.value.hasLoadedInterpolator ? 'no-data' : 'prb-missing'
})

/**
 * 状态条配色：橙=配置层问题（可点击导入 PRB），蓝=等待数据，红=数据层问题
 *
 * 边框使用 box-shadow inset 而非 border:border 会占据 2px 布局空间,
 * 让状态条总高度(~19.2px)超过标题文字行高(~14.4px),状态条 v-if 显隐时
 * 会撑高标题行 ~4.8px,导致下方 metrics 网格整体抖动。
 * box-shadow inset 不占据布局空间,配合 .dual-compact__section-title 的
 * min-height: 20px,状态条显隐时标题行高度恒定,垂直布局稳定。
 */
const statusBarStyle = computed(() => {
  if (interpStatus.value === 'prb-missing') {
    return {
      background: 'color-mix(in srgb, var(--state-warning) 12%, transparent)',
      color: 'var(--state-warning)',
      boxShadow: 'inset 0 0 0 1px var(--state-warning)',
    }
  }
  if (interpStatus.value === 'no-data') {
    return {
      background: 'color-mix(in srgb, var(--accent-info) 12%, transparent)',
      color: 'var(--accent-info)',
      boxShadow: 'inset 0 0 0 1px var(--accent-info)',
    }
  }
  return {
    background: 'color-mix(in srgb, var(--accent-danger) 12%, transparent)',
    color: 'var(--accent-danger)',
    boxShadow: 'inset 0 0 0 1px var(--accent-danger)',
  }
})

const statusBarText = computed(() => {
  if (interpStatus.value === 'prb-missing') return t.value.interpolationNotLoaded
  if (interpStatus.value === 'no-data') return t.value.interpolationWaitingData
  return t.value.interpolationInvalid
})

/** 插值无效时 tooltip 显示后端 warning 全文（如"压力差值越界"），便于排查 */
const statusBarTooltip = computed(() =>
  interpStatus.value === 'invalid' ? (realtimeResult.value?.warning ?? '') : ''
)

/** PRB 未加载时点击打开该 probe 的配置对话框导入；插值无效时不响应（需排查数据而非配置） */
function onStatusBarClick(): void {
  if (interpStatus.value === 'prb-missing') emit('openSettings', props.probeId)
}

/** I-23 修复：键盘可达性——Enter/Space 触发与点击等价操作。 */
function onStatusBarKeydown(event: KeyboardEvent): void {
  // 仅响应 Enter/Space；其他键（Tab/方向键）交给浏览器默认行为。
  if (event.key !== 'Enter' && event.key !== ' ') return
  event.preventDefault()
  onStatusBarClick()
}

// Warning/Error 摘要
const warningText = computed(() => traversalSessionWarning(session.value, t.value))
const hasWarning = computed(() => warningText.value.length > 0)
const isErrorStatus = computed(() => session.value.status?.status === 'error')

// I-24 修复：所有 IPC handler 包 try-catch，避免 IPC 异常成为 unhandled rejection。
// 失败时仍尝试用 session.error 兜底，若 session.error 也空则用通用错误模板。
async function safeCall(
  action: () => Promise<boolean>,
  failureKey: keyof typeof i18n.t,
): Promise<void> {
  try {
    const ok = await action()
    if (!ok) {
      feedbackStore.pushToast(
        `${t.value[failureKey]}：${session.value.error ?? ''}`,
        'error',
      )
    }
  } catch (err) {
    const detail = err instanceof Error ? err.message : String(err)
    feedbackStore.pushToast(`${t.value[failureKey]}：${detail}`, 'error')
  }
}

// 控制按钮 actions
function onStart(): Promise<void> {
  return safeCall(() => dualStore.start(props.probeId), 'dualStartFailed')
}

async function onResumeCheckpoint(): Promise<void> {
  const checkpoint = session.value.checkpoint
  if (!checkpoint) return
  await safeCall(() => dualStore.resumeFromCheckpoint(props.probeId, checkpoint.taskId), 'failedResume')
}

async function onDiscardCheckpoint(): Promise<void> {
  const checkpoint = session.value.checkpoint
  if (!checkpoint) return
  try {
    const confirmed = await feedbackStore.confirm(t.value.travCheckDetected, {
      title: t.value.travAbandon,
      confirmText: t.value.travAbandon,
      cancelText: t.value.cancel,
    })
    if (!confirmed) return
    await safeCall(() => dualStore.clearCheckpoint(props.probeId, checkpoint.taskId), 'failedDiscardCheckpoint')
  } catch (err) {
    // confirm 弹窗自身异常（如 modal 系统故障）——记录但不抛出。
    const detail = err instanceof Error ? err.message : String(err)
    feedbackStore.pushToast(`${t.value.failedDiscardCheckpoint}：${detail}`, 'error')
  }
}

function onPause(): Promise<void> {
  return safeCall(() => dualStore.pause(props.probeId), 'dualPauseFailed')
}

function onResume(): Promise<void> {
  return safeCall(() => dualStore.resume(props.probeId), 'dualResumeFailed')
}

async function onStop(): Promise<void> {
  try {
    const confirmed = await feedbackStore.confirm(t.value.travStopConfirm, {
      title: t.value.travStopTitle,
      confirmText: t.value.travStop,
      cancelText: t.value.cancel,
    })
    if (!confirmed) return
    await safeCall(() => dualStore.stop(props.probeId), 'dualStopFailed')
  } catch (err) {
    const detail = err instanceof Error ? err.message : String(err)
    feedbackStore.pushToast(`${t.value.dualStopFailed}：${detail}`, 'error')
  }
}

function onOpenSettings(): void {
  emit('openSettings', props.probeId)
}

// 终态显示：完成事件摘要
const completedSummary = computed(() => {
  const ev = session.value.completeEvent
  if (!ev) return ''
  if (ev.status === 'completed') return t.value.dualProbeCompleted.replace('{probeId}', props.probeId === 'probe1' ? t.value.probe1Label : t.value.probe2Label)
  if (ev.status === 'error') return t.value.dualProbeErrorOccurred.replace('{probeId}', props.probeId === 'probe1' ? t.value.probe1Label : t.value.probe2Label)
  return ''
})
</script>

<template>
  <div class="dual-compact" :data-probe-id="probeId">
    <!-- 控制栏：固定不滚动（spec FR7） -->
    <div class="dual-compact__controls">
      <UiButton
        size="sm"
        variant="primary"
        :disabled="!canStart"
        :loading="isStarting"
        @click="onStart"
      >
        {{ t.startRun }}
      </UiButton>
      <UiButton
        size="sm"
        variant="secondary"
        :disabled="!canPause"
        @click="onPause"
      >
        {{ t.travPause }}
      </UiButton>
      <UiButton
        size="sm"
        variant="secondary"
        :disabled="!canResume"
        @click="onResume"
      >
        {{ t.travResume }}
      </UiButton>
      <UiButton
        size="sm"
        variant="danger"
        :disabled="!canStop"
        @click="onStop"
      >
        {{ t.travStop }}
      </UiButton>
      <UiButton
        size="sm"
        variant="ghost"
        @click="onOpenSettings"
      >
        {{ t.dualProbeSetting }}
      </UiButton>
    </div>

    <div v-if="session.checkpoint" class="dual-compact__checkpoint" role="status">
      <span class="dual-compact__checkpoint-text">
        {{ t.travCheckDetected }} · {{ t.travCheckCompleted }}
        {{ session.checkpoint.completedPoints }} / {{ session.checkpoint.totalPoints }}
      </span>
      <UiButton size="sm" variant="warning" :disabled="isCheckpointPending" :loading="isCheckpointPending" @click="onResumeCheckpoint">
        {{ t.travContinueTest }}
      </UiButton>
      <UiButton size="sm" variant="ghost" :disabled="isCheckpointPending" @click="onDiscardCheckpoint">
        {{ t.travAbandon }}
      </UiButton>
    </div>

    <!-- 运行状态：降低视觉权重，为实时数据保留首屏空间 -->
    <div class="dual-compact__status-fields">
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.status }}</span>
        <span class="dual-compact__field-value dual-compact__status-value">
          <span class="dual-compact__status-dot" :style="{ background: statusDotColor }" aria-hidden="true"></span>
          {{ statusText }}
        </span>
      </div>
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.travProgress }}</span>
        <span class="dual-compact__field-value">{{ progressText }} ({{ progressPercent }}%)</span>
      </div>
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.currentPoint }}</span>
        <span class="dual-compact__field-value dual-compact__field-value--mono">{{ currentPointText }}</span>
      </div>
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.travActual }}</span>
        <span
          class="dual-compact__field-value dual-compact__field-value--mono"
          :style="anyAxisMoving ? { color: 'var(--accent-success)' } : undefined"
        >{{ actualPositionText }}</span>
      </div>
    </div>

    <div class="dual-compact__progress-bar">
      <div class="dual-compact__progress-fill" :style="{ width: progressPercent + '%' }" />
    </div>

    <!-- 插值读数：与运行状态分组，避免操作员误认为是点位或原始压力。
         数值配色与单探针一致：角度/速度用 --accent-info 强调，总压/静压用主文本色。 -->
    <section class="dual-compact__interpolation" aria-live="polite">
      <div class="dual-compact__section-title">
        <Activity class="dual-compact__section-icon" aria-hidden="true" />
        <span>{{ t.realtimeCalculation }}</span>
        <!-- 插值状态条：四态视觉区分，与标题同行右侧（与单探针一致）。
             PRB 未加载：橙色可点击打开配置；已加载未采集：蓝色；插值无效：红色 + tooltip -->
        <div
          v-if="interpStatus !== 'ok'"
          class="dual-compact__interp-status"
          :class="{ 'dual-compact__interp-status--clickable': interpStatus === 'prb-missing' }"
          :style="statusBarStyle"
          :title="statusBarTooltip"
          :role="interpStatus === 'prb-missing' ? 'button' : undefined"
          :tabindex="interpStatus === 'prb-missing' ? 0 : undefined"
          @click="onStatusBarClick"
          @keydown="onStatusBarKeydown"
        >
          <AlertTriangle class="dual-compact__interp-status-icon" aria-hidden="true" />
          <span class="dual-compact__interp-status-text">{{ statusBarText }}</span>
        </div>
      </div>
      <div class="dual-compact__metrics">
      <div class="dual-compact__field dual-compact__metric">
        <span class="dual-compact__field-label">{{ alphaLabel }}</span>
        <span class="dual-compact__metric-value dual-compact__metric-value--accent">{{ alphaText }}</span>
      </div>
      <div class="dual-compact__field dual-compact__metric">
        <span class="dual-compact__field-label">{{ betaLabel }}</span>
        <span class="dual-compact__metric-value dual-compact__metric-value--accent">{{ betaText }}</span>
      </div>
      <div class="dual-compact__field dual-compact__metric">
        <span class="dual-compact__field-label">{{ t.P0 }}</span>
        <span class="dual-compact__metric-value">{{ totalPressureText }}</span>
      </div>
      <div class="dual-compact__field dual-compact__metric">
        <span class="dual-compact__field-label">{{ t.Ps }}</span>
        <span class="dual-compact__metric-value">{{ staticPressureText }}</span>
      </div>
      <div class="dual-compact__field dual-compact__metric">
        <span class="dual-compact__field-label">{{ t.velocity }}</span>
        <span class="dual-compact__metric-value dual-compact__metric-value--accent">{{ velocityText }}</span>
      </div>
      </div>
    </section>

    <!-- 完成事件摘要 -->
    <div v-if="completedSummary && isTerminal" class="dual-compact__completed" :class="{ 'dual-compact__completed--error': session.completeEvent?.status === 'error' }">
      {{ completedSummary }}
    </div>

    <!-- Warning/Error 摘要：固定不滚动（spec FR7） -->
    <div v-if="hasWarning" class="dual-compact__warning" :class="{ 'dual-compact__warning--error': isErrorStatus }">
      <span class="dual-compact__warning-icon">{{ isErrorStatus ? '⚠' : '!' }}</span>
      <span class="dual-compact__warning-text">{{ warningText }}</span>
    </div>
  </div>
</template>

<style scoped src="./DualProbeCompactMonitor.css"></style>
