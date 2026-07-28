/**
 * ============================================================================
 * DualProbeCompactMonitor — 单 probe 紧凑监测面板（spec FR7 / Task 20）
 * ============================================================================
 *
 * 【功能定位】
 * 双探针模式下每个 row 内的紧凑监测面板，仅保留 spec FR7 列出的紧凑字段：
 *   - 状态、进度、当前点位
 *   - Alpha/Beta、总压、静压、速度
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
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import UiButton from '@components/ui/UiButton.vue'
import { useDualTraversalStore } from '@stores/dualTraversalStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
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

// 按钮启用状态：未配置时全部禁用，仅允许打开设置
const isUnconfigured = computed(() => !session.value.config)
const isRunning = computed(() => session.value.status?.status === 'running')
const isPaused = computed(() => session.value.status?.status === 'paused')
const isTerminal = computed(() => {
  const s = session.value.status?.status
  return s === 'completed' || s === 'stopped' || s === 'error'
})
const isStarting = computed(() => session.value.isStarting)
const canStart = computed(() => !isUnconfigured.value && !isRunning.value && !isPaused.value && !isStarting.value && session.value.hasLoadedInterpolator)
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

// Warning/Error 摘要
const warningText = computed(() => {
  const s = session.value
  const parts: string[] = []
  if (s.status?.warning) parts.push(s.status.warning)
  if (s.status?.lastError) parts.push(s.status.lastError)
  if (s.error) parts.push(s.error)
  return parts.length > 0 ? parts.join('；') : ''
})
const hasWarning = computed(() => warningText.value.length > 0)
const isErrorStatus = computed(() => session.value.status?.status === 'error')

// 控制按钮 actions
async function onStart(): Promise<void> {
  const ok = await dualStore.start(props.probeId)
  if (!ok) {
    feedbackStore.pushToast(t.value.dualStartFailed + '：' + (session.value.error ?? ''), 'error')
  }
}

async function onPause(): Promise<void> {
  const ok = await dualStore.pause(props.probeId)
  if (!ok) {
    feedbackStore.pushToast(t.value.dualPauseFailed + '：' + (session.value.error ?? ''), 'error')
  }
}

async function onResume(): Promise<void> {
  const ok = await dualStore.resume(props.probeId)
  if (!ok) {
    feedbackStore.pushToast(t.value.dualResumeFailed + '：' + (session.value.error ?? ''), 'error')
  }
}

async function onStop(): Promise<void> {
  const confirmed = await feedbackStore.confirm(t.value.travStopConfirm, {
    title: t.value.travStopTitle,
    confirmText: t.value.travStop,
    cancelText: t.value.cancel,
  })
  if (!confirmed) return
  const ok = await dualStore.stop(props.probeId)
  if (!ok) {
    feedbackStore.pushToast(t.value.dualStopFailed + '：' + (session.value.error ?? ''), 'error')
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

    <!-- 紧凑监测字段 -->
    <div class="dual-compact__fields">
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.status }}</span>
        <span class="dual-compact__field-value">{{ statusText }}</span>
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
        <span class="dual-compact__field-label">{{ alphaLabel }}</span>
        <span class="dual-compact__field-value dual-compact__field-value--mono">{{ alphaText }}</span>
      </div>
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ betaLabel }}</span>
        <span class="dual-compact__field-value dual-compact__field-value--mono">{{ betaText }}</span>
      </div>
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.P0 }}</span>
        <span class="dual-compact__field-value dual-compact__field-value--mono">{{ totalPressureText }}</span>
      </div>
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.Ps }}</span>
        <span class="dual-compact__field-value dual-compact__field-value--mono">{{ staticPressureText }}</span>
      </div>
      <div class="dual-compact__field">
        <span class="dual-compact__field-label">{{ t.velocity }}</span>
        <span class="dual-compact__field-value dual-compact__field-value--mono">{{ velocityText }}</span>
      </div>
    </div>

    <!-- 进度条 -->
    <div class="dual-compact__progress-bar">
      <div class="dual-compact__progress-fill" :style="{ width: progressPercent + '%' }" />
    </div>

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

<style scoped>
.dual-compact {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
  background: var(--bg-surface, #ffffff);
  border: 1px solid var(--border-subtle, #d0d0d0);
  border-radius: 6px;
  min-width: 0;
}

.dual-compact__controls {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.dual-compact__fields {
  display: grid;
  /* 左右并排时容器很窄，120px 会导致字段被截断；改为更紧凑的列宽并允许自由换行 */
  grid-template-columns: repeat(auto-fit, minmax(80px, 1fr));
  gap: 6px 8px;
  font-size: 12px;
}

.dual-compact__field {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.dual-compact__field-label {
  color: var(--text-tertiary, #888);
  font-size: 10px;
}

.dual-compact__field-value {
  color: var(--text-primary, #1f1f1f);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dual-compact__field-value--mono {
  font-family: var(--font-mono, monospace);
  font-weight: 500;
}

.dual-compact__progress-bar {
  height: 4px;
  background: var(--bg-elevated, #e8e8e8);
  border-radius: 2px;
  overflow: hidden;
}

.dual-compact__progress-fill {
  height: 100%;
  background: var(--color-primary, #2080f0);
  transition: width 0.2s ease;
}

.dual-compact__completed {
  padding: 4px 8px;
  background: var(--color-success-bg, #e8f5e9);
  color: var(--color-success, #2e7d32);
  border-radius: 4px;
  font-size: 12px;
}

.dual-compact__completed--error {
  background: var(--color-error-bg, #ffebee);
  color: var(--color-error, #d03030);
}

.dual-compact__warning {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  padding: 4px 8px;
  background: var(--color-warning-bg, #fff8e1);
  color: var(--color-warning, #f0a020);
  border-radius: 4px;
  font-size: 11px;
  overflow: hidden;
}

.dual-compact__warning--error {
  background: var(--color-error-bg, #ffebee);
  color: var(--color-error, #d03030);
}

.dual-compact__warning-icon {
  flex-shrink: 0;
  font-weight: 700;
}

.dual-compact__warning-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
