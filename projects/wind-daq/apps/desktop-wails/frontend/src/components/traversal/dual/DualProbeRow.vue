/**
 * ============================================================================
 * DualProbeRow — 单 probe 完整 row 容器（spec FR7 / Task 20）
 * ============================================================================
 *
 * 【功能定位】
 * 双探针模式下每 probe 的完整 row 容器：
 *   - 紧凑监测区（DualProbeCompactMonitor）：固定不滚动，含控制按钮 +
 *     Warning 摘要 + 核心字段（状态/进度/当前点位/Alpha·Beta/总压/静压/速度）
 *   - Tab 详情区：可独立滚动；包含完整通道值、运动状态、点位预览、诊断信息
 *
 * 【布局约束】
 * - 控制栏与 Warning 固定不滚动（spec FR7），避免长错误文本遮挡关键控制；
 * - Tab 详情区独立滚动，长内容不影响另一 probe 的 row；
 * - 五孔/七孔探针类型组合下展示字段正确（Alpha/Beta 标签按
 *   TRAVERSAL_PROBE_PRESENTATION 切换）。
 *
 * @module DualProbeRow
 * @see DualProbeCompactMonitor — 紧凑监测面板
 * ============================================================================
 */
<script setup lang="ts">
import { computed, ref } from 'vue'
import { storeToRefs } from 'pinia'

import DualProbeCompactMonitor from './DualProbeCompactMonitor.vue'
import PointsPreview from '@components/traversal/PointsPreview.vue'

import { useDualTraversalStore } from '@stores/dualTraversalStore'
import { useI18nStore } from '@stores/i18nStore'
import { TRAVERSAL_PROBE_PRESENTATION } from '@shared/types/traversal'
import type { ProbeId, TraversalSessionState } from '@shared/types/traversal'

const props = defineProps<{
  probeId: ProbeId
}>()

const emit = defineEmits<{
  openSettings: [probeId: ProbeId]
}>()

const dualStore = useDualTraversalStore()
const i18n = useI18nStore()
const { sessions } = storeToRefs(dualStore)
const t = computed(() => i18n.t)

const session = computed<TraversalSessionState>(() => sessions.value[props.probeId])

// 探针展示元数据：五孔 Alpha=攻角/Beta=侧滑角；七孔 Alpha=侧滑角/Beta=迎角
const probePresentation = computed(() =>
  TRAVERSAL_PROBE_PRESENTATION[session.value.config?.probeType ?? 'five-hole'],
)
const probeTitleText = computed(() => {
  const key = probePresentation.value.titleKey as unknown as keyof typeof t.value
  return (t.value[key] as string | undefined) ?? probePresentation.value.titleKey
})
const probeLabel = computed(() =>
  props.probeId === 'probe1' ? t.value.probe1Label : t.value.probe2Label,
)

// Tab 切换：详情/通道/运动/诊断（仅展示该 probe 配置/状态相关的 Tab）
type DetailTab = 'points' | 'channels' | 'motion' | 'diagnostics'
const activeTab = ref<DetailTab>('points')

const tabs = computed<{ value: DetailTab; label: string }[]>(() => [
  { value: 'points', label: t.value.pointsPreview },
  { value: 'channels', label: t.value.realtimePressureData },
  { value: 'motion', label: t.value.travHardwareStatus },
  { value: 'diagnostics', label: t.value.travValidationWarnings },
])

// 通道压力：来自 session.realtimePressures（每 probe 独立）
// TraversalRawPressure 是结构化对象（P1..P7/Patm/Tatm/P0/Ps），按通道名匹配展示
interface ChannelRow {
  index: number
  label: string
  deviceId: string
  channelIndex: string
  value: number | null
}

// 角色到压力字段的映射（与 ProbeChannelRole 枚举对齐）
const roleToField: Record<string, keyof NonNullable<TraversalSessionState['realtimePressures']>> = {
  'fiveHole.p1': 'P1',
  'fiveHole.p2': 'P2',
  'fiveHole.p3': 'P3',
  'fiveHole.p4': 'P4',
  'fiveHole.p5': 'P5',
  'sevenHole.p1': 'P1',
  'sevenHole.p2': 'P2',
  'sevenHole.p3': 'P3',
  'sevenHole.p4': 'P4',
  'sevenHole.p5': 'P5',
  'sevenHole.p6': 'P6',
  'sevenHole.p7': 'P7',
  'fiveHole.pAtm': 'Patm',
  'fiveHole.tAtm': 'Tatm',
  'sevenHole.pAtm': 'Patm',
  'sevenHole.tAtm': 'Tatm',
  'fiveHole.pTotal': 'P0',
  'fiveHole.pStatic': 'Ps',
  'sevenHole.pTotal': 'P0',
  'sevenHole.pTunnelStatic': 'Ps',
}

const channelRows = computed<ChannelRow[]>(() => {
  const cfg = session.value.config
  const pressures = session.value.realtimePressures
  if (!cfg) return []
  const channels = cfg.channels?.probeChannels ?? []
  return channels.map((ch, idx) => {
    const field = ch.role ? roleToField[ch.role] : undefined
    const value = field && pressures ? (pressures[field] ?? null) : null
    return {
      index: idx,
      label: ch.name || `CH${idx + 1}`,
      deviceId: ch.channel?.deviceId ?? '',
      channelIndex: ch.channel?.channelIndex != null ? String(ch.channel.channelIndex) : '',
      value,
    }
  })
})

// 运动状态：轴绑定 + 当前点位目标（来自 session.status.currentPoint）
// 注意：后端 TraversalTestStatus 不返回轴实际位置（axisPositions 字段不存在），
// 故仅展示绑定信息与目标点位；实际位置由硬件状态面板（不在 dual 紧凑视图中）展示。
interface MotionRow {
  axis: string
  controllerId: string
  physicalAxis: string
  target: number | null
}

const motionRows = computed<MotionRow[]>(() => {
  const cfg = session.value.config
  const status = session.value.status
  if (!cfg) return []
  // TraversalTestConfig.channels.motionAxes 是绑定数组（不是顶层 motionAxes）
  const axes = cfg.channels?.motionAxes ?? []
  const targets = status?.currentPoint ?? null
  const targetMap: Record<string, number | null> = targets
    ? {
        X: targets.alpha ?? null,
        Y: targets.beta ?? null,
        Z: targets.z ?? null,
        U: targets.u ?? null,
      }
    : {}
  return axes.map((axis) => {
    const axisName = axis.name?.toUpperCase() ?? ''
    return {
      axis: axis.name,
      controllerId: axis.controllerId || '—',
      physicalAxis: axis.axis || '—',
      target: targetMap[axisName] ?? null,
    }
  })
})

// 诊断信息：警告/错误/validationWarnings
const diagnostics = computed(() => {
  const s = session.value
  const items: { type: 'warning' | 'error' | 'info'; text: string }[] = []
  if (s.error) items.push({ type: 'error', text: s.error })
  if (s.status?.warning) items.push({ type: 'warning', text: s.status.warning })
  if (s.status?.lastError) items.push({ type: 'error', text: s.status.lastError })
  if (s.status?.motionSafetyFailure) {
    items.push({ type: 'error', text: JSON.stringify(s.status.motionSafetyFailure) })
  }
  if (s.status?.validationWarnings) {
    for (const w of s.status.validationWarnings) {
      items.push({ type: 'warning', text: w })
    }
  }
  if (s.interpolatorRestoreMessage) {
    items.push({ type: 'warning', text: s.interpolatorRestoreMessage })
  }
  if (items.length === 0) {
    items.push({ type: 'info', text: t.value.noLayoutConfigured })
  }
  return items
})

// 点位预览数据：从 config 派生
const layoutForPreview = computed(() => session.value.config?.layout ?? null)

/** 转发紧凑监测区的 openSettings 事件 */
function onOpenSettings(): void {
  emit('openSettings', props.probeId)
}
</script>

<template>
  <div
    class="dual-row"
    :data-probe-id="probeId"
    :data-test="`dual-probe-row-${probeId}`"
  >
    <!-- row 头部：探针标签 + 标题 -->
    <div class="dual-row__header">
      <span class="dual-row__probe-label">{{ probeLabel }}</span>
      <span class="dual-row__probe-title">{{ probeTitleText }}</span>
    </div>

    <!-- 紧凑监测区：固定不滚动（spec FR7） -->
    <DualProbeCompactMonitor :probe-id="probeId" @open-settings="onOpenSettings" />

    <!-- Tab 切换条 -->
    <div class="dual-row__tabs" role="tablist">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        type="button"
        role="tab"
        :aria-selected="activeTab === tab.value"
        :class="['dual-row__tab', { 'dual-row__tab--active': activeTab === tab.value }]"
        @click="activeTab = tab.value"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- Tab 详情区：可独立滚动；控制栏与 Warning 不被滚动隐藏 -->
    <div class="dual-row__detail" role="tabpanel">
      <!-- 点位预览 -->
      <div v-if="activeTab === 'points'" class="dual-row__panel">
        <PointsPreview
          v-if="layoutForPreview"
          :layout="layoutForPreview"
          :current-point="session.status?.currentPoint"
          :completed-points="session.status?.completedPoints ?? 0"
          :current-point-phase="session.status?.currentPointPhase"
          :labels="{
            moving: t.moving,
            stabilizing: t.stabilizing,
            acquiring: t.acquiring,
            completed: t.completed,
            untested: t.untested,
            noLayoutConfigured: t.noLayoutConfigured,
            pleaseConfigureLayout: t.pleaseConfigureLayout,
            configureLayout: t.configureLayout,
          }"
        />
        <div v-else class="dual-row__empty">{{ t.pleaseConfigureLayout }}</div>
      </div>

      <!-- 通道压力 -->
      <div v-else-if="activeTab === 'channels'" class="dual-row__panel">
        <div v-if="channelRows.length === 0" class="dual-row__empty">{{ t.pleaseConfigureLayout }}</div>
        <table v-else class="dual-row__table">
          <thead>
            <tr>
              <th>#</th>
              <th>{{ t.realtimePressureData }}</th>
              <th>Device</th>
              <th>Ch</th>
              <th>Value</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in channelRows" :key="row.index">
              <td>{{ row.index + 1 }}</td>
              <td>{{ row.label }}</td>
              <td class="dual-row__mono">{{ row.deviceId || '—' }}</td>
              <td class="dual-row__mono">{{ row.channelIndex || '—' }}</td>
              <td class="dual-row__mono">{{ row.value !== null ? row.value.toFixed(3) : '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 运动状态 -->
      <div v-else-if="activeTab === 'motion'" class="dual-row__panel">
        <div v-if="motionRows.length === 0" class="dual-row__empty">{{ t.pleaseConfigureLayout }}</div>
        <table v-else class="dual-row__table">
          <thead>
            <tr>
              <th>{{ t.travMotionSafetyAxis }}</th>
              <th>{{ t.travMotionSafetyController }}</th>
              <th>{{ t.travActual }}</th>
              <th>{{ t.travTarget }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in motionRows" :key="row.axis">
              <td>{{ row.axis }}</td>
              <td class="dual-row__mono">{{ row.controllerId }}</td>
              <td class="dual-row__mono">{{ row.physicalAxis }}</td>
              <td class="dual-row__mono">{{ row.target !== null ? row.target.toFixed(3) : '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 诊断 -->
      <div v-else-if="activeTab === 'diagnostics'" class="dual-row__panel">
        <div
          v-for="(item, idx) in diagnostics"
          :key="idx"
          :class="['dual-row__diag', `dual-row__diag--${item.type}`]"
        >
          <span class="dual-row__diag-icon">{{ item.type === 'error' ? '⚠' : item.type === 'warning' ? '!' : 'i' }}</span>
          <span class="dual-row__diag-text">{{ item.text }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dual-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
  background: var(--bg-surface, #ffffff);
  border: 1px solid var(--border-subtle, #d0d0d0);
  border-radius: 6px;
  min-height: 0;
  min-width: 0;
  /* 关键：左右并排时各占 50%，内容溢出必须裁剪而不是撑破 flex 行 */
  flex: 1 1 0;
  overflow: hidden;
}

.dual-row__header {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 13px;
  flex: 0 0 auto;
}

.dual-row__probe-label {
  font-weight: 700;
  color: var(--text-primary, #1f1f1f);
}

.dual-row__probe-title {
  color: var(--text-secondary, #5a5a5a);
  font-size: 12px;
}

.dual-row__tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--border-subtle, #e0e0e0);
  flex: 0 0 auto;
  overflow-x: auto;
}

.dual-row__tab {
  padding: 6px 10px;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text-secondary, #5a5a5a);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
  transition: color 0.15s, border-color 0.15s;
}

.dual-row__tab:hover {
  color: var(--text-primary, #1f1f1f);
}

.dual-row__tab--active {
  color: var(--color-primary, #2080f0);
  border-bottom-color: var(--color-primary, #2080f0);
  font-weight: 600;
}

.dual-row__detail {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 6px 4px;
}

.dual-row__panel {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.dual-row__empty {
  padding: 16px;
  color: var(--text-tertiary, #888);
  font-size: 12px;
  text-align: center;
}

.dual-row__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 11px;
}

.dual-row__table th,
.dual-row__table td {
  padding: 4px 6px;
  border-bottom: 1px solid var(--border-subtle, #e8e8e8);
  text-align: left;
}

.dual-row__table th {
  background: var(--bg-elevated, #f5f5f5);
  color: var(--text-tertiary, #888);
  font-weight: 600;
}

.dual-row__mono {
  font-family: var(--font-mono, monospace);
  font-size: 11px;
}

.dual-row__diag {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 6px 8px;
  border-radius: 4px;
  font-size: 11px;
  margin-bottom: 4px;
}

.dual-row__diag--error {
  background: var(--color-error-bg, #ffebee);
  color: var(--color-error, #d03030);
}

.dual-row__diag--warning {
  background: var(--color-warning-bg, #fff8e1);
  color: var(--color-warning, #f0a020);
}

.dual-row__diag--info {
  background: var(--bg-elevated, #f5f5f5);
  color: var(--text-secondary, #5a5a5a);
}

.dual-row__diag-icon {
  flex-shrink: 0;
  font-weight: 700;
}

.dual-row__diag-text {
  word-break: break-word;
  white-space: pre-wrap;
}
</style>
