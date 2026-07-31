/**
 * ============================================================================
 * DualProbeRow — 单 probe 完整 row 容器（spec FR7 / Task 20）
 * ============================================================================
 *
 * 【功能定位】
 * 双探针模式下每 probe 的完整 row 容器：
 *   - 紧凑监测区（DualProbeCompactMonitor）：固定不滚动，含控制按钮 +
 *     Warning 摘要 + 运行状态 + 实时插值
 *   - 固定数据区：实时压力与实时插值同时可见，下方展示点位预览
 *
 * 【布局约束】
 * - 控制栏与 Warning 固定不滚动（spec FR7），避免长错误文本遮挡关键控制；
 * - 数据区独立滚动，长内容不影响另一 probe 的 row；
 * - 五孔/七孔探针类型组合下展示字段正确（Alpha/Beta 标签按
 *   TRAVERSAL_PROBE_PRESENTATION 切换）。
 *
 * @module DualProbeRow
 * @see DualProbeCompactMonitor — 紧凑监测面板
 * ============================================================================
 */
<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { Crosshair, Wind } from '@lucide/vue'

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

// 通道压力：来自 session.realtimePressures（每 probe 独立）
// TraversalRawPressure 是结构化对象（P1..P7/Patm/Tatm/P0/Ps），按通道名匹配展示。
// 卡片只保留 标签 + 数值 + 单位（与单探针 TraversalLiveMonitor 一致），
// 设备 ID / 通道序号属于配置层信息，不在实时监测画面展示。
interface ChannelRow {
  index: number
  label: string
  value: number | null
  unit: string
  // 通道显示精度：取自 ProbeChannelConfig.precision（用户在硬件配置步骤逐通道调整），
  // 与单探针 useTraversalRealtimeData 的 getChannelPrecision 语义一致。
  // 缺省回退 3，保持与历史行为兼容。
  precision: number
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
      value,
      unit: ch.role?.endsWith('.tAtm') ? '°C' : 'Pa',
      // precision 在 TraversalHardwareStep 中已 clamp 到 [0,8]，
      // 但旧配置或外部导入可能缺失，统一回退到 3 与单探针模式一致
      precision: typeof ch.precision === 'number' ? ch.precision : 3,
    }
  })
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

    <div class="dual-row__live-data">
      <section class="dual-row__section" aria-live="polite">
        <div class="dual-row__section-title">
          <Wind class="dual-row__section-icon" aria-hidden="true" />
          <span>{{ t.realtimePressureData }}</span>
        </div>
        <div v-if="channelRows.length === 0" class="dual-row__empty">{{ t.pleaseConfigureLayout }}</div>
        <div v-else class="dual-row__pressure-grid">
          <div v-for="row in channelRows" :key="row.index" class="dual-row__pressure-item">
            <span class="dual-row__pressure-label">{{ row.label }}</span>
            <div class="dual-row__pressure-reading">
              <span class="dual-row__pressure-value">{{ row.value !== null ? row.value.toFixed(row.precision) : '—' }}</span>
              <span class="dual-row__pressure-unit">{{ row.unit }}</span>
            </div>
          </div>
        </div>
      </section>

      <section class="dual-row__section dual-row__points">
        <div class="dual-row__section-title">
          <Crosshair class="dual-row__section-icon" aria-hidden="true" />
          <span>{{ t.pointsPreview }}</span>
        </div>
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
      </section>
    </div>
  </div>
</template>

<style scoped>
/* 配色对齐单探针 TraversalLiveMonitor：面板 --bg-panel + 卡片 --bg-panel-strong，
   标签 --text-muted，数值 mono bold，图标/强调 --accent-info。
   旧实现使用 --bg-surface/--bg-elevated/--border-subtle 等不存在的 token，
   fallback 成硬编码浅色，导致与整体主题脱节（深色模式下更是白卡片）。 */
.dual-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 14px;
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: 12px;
  box-shadow: var(--shadow-panel);
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
  color: var(--text-primary);
}

.dual-row__probe-title {
  color: var(--text-muted);
  font-size: 12px;
}

.dual-row__live-data {
  display: flex;
  flex-direction: column;
  gap: 10px;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 0;
}

.dual-row__section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.dual-row__section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--text-primary);
  font-size: 12px;
  font-weight: 500;
}

.dual-row__section-icon {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  color: var(--accent-info);
}

.dual-row__points {
  /* 关键：shrink=1 + min-height: 0 + overflow: hidden 三者配合，
     让窗口缩小时 flex item 能跟随 .dual-row__live-data 一起缩小。
     原 flex: 1 0 240px 的 shrink=0 + min-height: auto（默认值），
     导致 PointsPreview 的 canvas 在窗口放大时撑大 containerRef，
     反向撑大本 section 到放大时的尺寸（如 600px），
     窗口缩小时本 section 卡在大尺寸，ResizeObserver 不触发，
     canvas 不缩小，"放大窗口再缩小不回缩"。
     min-height: 0 允许 flex item 缩小到内容以下，
     overflow: hidden 裁剪 canvas 短暂溢出（draw 更新前的过渡帧）。 */
  flex: 1 1 240px;
  min-height: 0;
  overflow: hidden;
  padding-top: 10px;
  border-top: 1px solid var(--border-default);
}

.dual-row__pressure-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 6px;
}

/* 单探针式单行卡片：左标签右数值，基线对齐 */
.dual-row__pressure-item {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
  padding: 6px 12px;
  background: var(--bg-panel-strong);
  border-radius: 8px;
}

.dual-row__pressure-label {
  color: var(--text-muted);
  font-size: 11px;
  font-weight: 500;
  flex-shrink: 0;
}

.dual-row__pressure-reading {
  display: flex;
  align-items: baseline;
  gap: 4px;
  min-width: 0;
}

.dual-row__pressure-unit {
  color: var(--text-muted);
  font-size: 10px;
  font-weight: 500;
  flex-shrink: 0;
}

.dual-row__pressure-value {
  color: var(--text-primary);
  font-family: var(--font-family-mono, monospace);
  font-size: 17px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dual-row__empty {
  padding: 16px;
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
}

</style>
