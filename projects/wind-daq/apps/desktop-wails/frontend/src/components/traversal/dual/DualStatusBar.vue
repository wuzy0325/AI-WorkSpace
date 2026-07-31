/**
 * ============================================================================
 * DualStatusBar — 双探针状态栏（顶部双列摘要，spec FR7 / Task 19）
 * ============================================================================
 *
 * 【功能定位】
 * 双探针模式下顶部双列摘要状态栏：每列对应一个 probe，集中展示操作员最
 * 频繁关注的核心信息——状态/进度/当前点位/Alpha·Beta/总压/静压/速度/
 * Warning 摘要。完整通道值、运动状态、点位预览、诊断信息放在该 row 的
 * Tab 详情区（DualProbeRow）。
 *
 * 【设计要点】
 * - 双列等宽布局，1440x900 / 1600x900 下不重叠、不溢出；
 * - 每列字段按探针类型（五孔/七孔）切换 Alpha/Beta 标签语义
 *   （TRAVERSAL_PROBE_PRESENTATION）；
 * - 长错误文本/双 Warning 下控制栏不被遮挡（row 控制栏与 Warning 固定
 *   不滚动；本组件自身不滚动，仅展示摘要）。
 *
 * @module DualStatusBar
 * @see DualProbeRow — 完整 row 容器（紧凑监测 + Tab 详情）
 * ============================================================================
 */
<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useDualTraversalStore } from '@stores/dualTraversalStore'
import { useI18nStore } from '@stores/i18nStore'
import { TRAVERSAL_PROBE_PRESENTATION } from '@shared/types/traversal'
import type { ProbeId, TraversalSessionState, TraversalTestStatusType } from '@shared/types/traversal'

const props = defineProps<{
  probeId: ProbeId
}>()

const dualStore = useDualTraversalStore()
const i18n = useI18nStore()
const { sessions } = storeToRefs(dualStore)
const t = computed(() => i18n.t)

const session = computed<TraversalSessionState>(() => sessions.value[props.probeId])

// 探针标签：probe1 / probe2 → 本地化名称
const probeLabel = computed(() => props.probeId === 'probe1' ? t.value.probe1Label : t.value.probe2Label)

// 探针展示元数据（spec §6.5）：五孔 Alpha=攻角/Beta=侧滑角；
// 七孔 Alpha=侧滑角/Beta=迎角。未配置时按五孔默认。
const probePresentation = computed(() =>
  TRAVERSAL_PROBE_PRESENTATION[session.value.config?.probeType ?? 'five-hole']
)
const alphaLabel = computed(() =>
  (t.value as Record<string, string>)[probePresentation.value.alphaLabelKey] ?? probePresentation.value.alphaLabelKey
)
const betaLabel = computed(() =>
  (t.value as Record<string, string>)[probePresentation.value.betaLabelKey] ?? probePresentation.value.betaLabelKey
)

// 状态文本：未配置/空闲/运行中/暂停/已完成/错误/已停止
const statusText = computed(() => {
  if (!session.value.config) return t.value.dualProbeUnconfigured
  const status = session.value.status?.status as TraversalTestStatusType | undefined
  if (!status) return t.value.dualProbeIdle
  const key = status as string
  return (t.value as Record<string, string>)[key] ?? key
})

// 进度百分比
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

// 当前点位坐标（line/rectangle/sector 模式 z/u 可能 null）
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

// 插值结果关键值
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

// Warning / Error 摘要（来自 status.warning / status.lastError / session.error）
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
</script>

<template>
  <div
    class="dual-status-cell"
    :data-probe-id="probeId"
    :class="{ 'dual-status-cell--error': isErrorStatus, 'dual-status-cell--warning': hasWarning && !isErrorStatus }"
  >
    <div class="dual-status-cell__header">
      <span class="dual-status-cell__probe-label">{{ probeLabel }}</span>
      <span class="dual-status-cell__status">{{ statusText }}</span>
    </div>
    <div class="dual-status-cell__progress">
      <div class="dual-status-cell__progress-bar">
        <div class="dual-status-cell__progress-fill" :style="{ width: progressPercent + '%' }" />
      </div>
      <span class="dual-status-cell__progress-text">{{ progressText }} ({{ progressPercent }}%)</span>
    </div>
    <div class="dual-status-cell__point">
      <span class="dual-status-cell__point-label">{{ t.currentPoint }}:</span>
      <span class="dual-status-cell__point-value">{{ currentPointText }}</span>
    </div>
    <div class="dual-status-cell__metrics">
      <div class="dual-status-cell__metric">
        <span class="dual-status-cell__metric-label">{{ alphaLabel }}</span>
        <span class="dual-status-cell__metric-value">{{ alphaText }}</span>
      </div>
      <div class="dual-status-cell__metric">
        <span class="dual-status-cell__metric-label">{{ betaLabel }}</span>
        <span class="dual-status-cell__metric-value">{{ betaText }}</span>
      </div>
      <div class="dual-status-cell__metric">
        <span class="dual-status-cell__metric-label">{{ t.P0 }}</span>
        <span class="dual-status-cell__metric-value">{{ totalPressureText }}</span>
      </div>
      <div class="dual-status-cell__metric">
        <span class="dual-status-cell__metric-label">{{ t.Ps }}</span>
        <span class="dual-status-cell__metric-value">{{ staticPressureText }}</span>
      </div>
      <div class="dual-status-cell__metric">
        <span class="dual-status-cell__metric-label">{{ t.velocity }}</span>
        <span class="dual-status-cell__metric-value">{{ velocityText }}</span>
      </div>
    </div>
    <div v-if="hasWarning" class="dual-status-cell__warning" :title="warningText">
      <span v-if="isErrorStatus" class="dual-status-cell__warning-icon">⚠</span>
      <span v-else class="dual-status-cell__warning-icon">!</span>
      <span class="dual-status-cell__warning-text">{{ warningText }}</span>
    </div>
  </div>
</template>

<style scoped>
/* C12 修复：原使用 --bg-surface / --bg-elevated / --border-subtle / --text-tertiary /
   --font-mono / --color-warning / --color-error / --color-primary 等项目不存在的 token，
   全部 fallback 成硬编码浅色，导致深色模式下状态栏显示白卡片、文字与背景对比度不足。
   改为对齐 DualProbeRow.vue 与 light.css / dark.css 中真实存在的 token 体系：
   背景 --bg-panel / --bg-panel-strong；边框 --border-default；文本 --text-primary /
   --text-secondary / --text-muted；强调色 --accent-warning / --accent-danger / --accent-info；
   等宽字体 --font-family-mono；阴影 --shadow-panel。
   警告/错误背景统一用 color-mix 透明叠加，随主题切换自动适配。 */
.dual-status-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 14px;
  border: 1px solid var(--border-default);
  border-radius: 6px;
  background: var(--bg-panel);
  min-width: 0;
  flex: 1 1 0;
}

.dual-status-cell--warning {
  border-color: color-mix(in srgb, var(--accent-warning) 45%, transparent);
  background: color-mix(in srgb, var(--accent-warning) 12%, transparent);
}

.dual-status-cell--error {
  border-color: color-mix(in srgb, var(--accent-danger) 45%, transparent);
  background: color-mix(in srgb, var(--accent-danger) 12%, transparent);
}

.dual-status-cell__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 600;
}

.dual-status-cell__probe-label {
  color: var(--text-primary);
}

.dual-status-cell__status {
  color: var(--text-secondary);
  font-weight: 400;
}

.dual-status-cell__progress {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}

.dual-status-cell__progress-bar {
  flex: 1 1 auto;
  height: 6px;
  background: var(--bg-panel-strong);
  border-radius: 3px;
  overflow: hidden;
}

.dual-status-cell__progress-fill {
  height: 100%;
  background: var(--accent-info);
  transition: width 0.2s ease;
}

.dual-status-cell__progress-text {
  color: var(--text-secondary);
  white-space: nowrap;
}

.dual-status-cell__point {
  display: flex;
  gap: 6px;
  font-size: 12px;
  color: var(--text-secondary);
}

.dual-status-cell__point-value {
  color: var(--text-primary);
  font-family: var(--font-family-mono);
}

.dual-status-cell__metrics {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  font-size: 11px;
}

.dual-status-cell__metric {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.dual-status-cell__metric-label {
  color: var(--text-muted);
  font-size: 10px;
}

.dual-status-cell__metric-value {
  color: var(--text-primary);
  font-family: var(--font-family-mono);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dual-status-cell__warning {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  padding: 4px 6px;
  background: var(--bg-panel-strong);
  border-radius: 4px;
  font-size: 11px;
  color: var(--text-secondary);
  overflow: hidden;
}

.dual-status-cell__warning-icon {
  flex-shrink: 0;
  font-weight: 700;
}

.dual-status-cell__warning-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
