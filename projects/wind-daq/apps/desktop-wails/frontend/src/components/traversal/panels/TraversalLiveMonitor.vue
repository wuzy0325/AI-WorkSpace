<script setup lang="ts">
/**
 * Left-side live monitor panel for the traversal page.
 * Pure display: takes already-derived data from the parent and renders it.
 * Phase B: tokenised colors — no slate-* / blue-* / emerald-* / rose-* literals.
 */
import { Activity, ClipboardList } from '@lucide/vue'
import type { PressureItem } from '@composables/useTraversalRealtimeData'
import type { InterpolationResult } from '@shared/types/traversal'

// Local types that mirror the unexported shapes from useHardwareConnectionStatus.
// Re-declared here (not imported) so this component does not reach into a
// sibling composable's internals; parent passes already-built display models.
interface ConnectionDisplay {
  label: string
  dotColor: string
  dotGlow: string
  textColor: string
}

interface AxisPositionDatum {
  label: string
  position: number | undefined
  moving: boolean
}

defineProps<{
  hasConfig: boolean
  currentPointSummary: { alpha: string; beta: string }
  axisPositions: AxisPositionDatum[]
  acquisitionConnection: ConnectionDisplay
  positionerConnection: ConnectionDisplay
  pressureItems: PressureItem[]
  realtimeResult: InterpolationResult | null
  labels: {
    monitor: string
    currentPoint: string
    positioner: string
    realtimeCalculation: string
    realtimePressureData: string
    alpha: string
    beta: string
    mach: string
    velocity: string
  }
}>()
</script>

<template>
  <aside class="flex min-h-0 h-full flex-col">
    <section
      data-test="traversal-sidebar-monitor"
      class="flex h-full min-h-0 flex-col rounded-lg border shadow-sm overflow-y-auto"
      :style="{
        borderColor: 'var(--border-default)',
        background: 'var(--bg-panel)',
        padding: 'var(--space-3)',
      }"
    >
      <!-- 面板头部 -->
      <header
        class="flex items-center justify-between"
        :style="{ marginBottom: 'var(--space-3)' }"
      >
        <div class="flex items-center gap-2">
          <Activity class="h-4 w-4 text-[var(--accent-info)]" />
          <h2 class="text-sm font-semibold text-[var(--text-primary)]">{{ labels.monitor }}</h2>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <div data-test="traversal-acquisition-indicator" class="flex items-center gap-1">
            <span
              class="h-2 w-2 rounded-full"
              :style="{ background: acquisitionConnection.dotColor, boxShadow: acquisitionConnection.dotGlow }"
            ></span>
            <span class="text-xs" :style="{ color: acquisitionConnection.textColor }">
              {{ acquisitionConnection.label }}
            </span>
          </div>
          <div class="h-3 w-px" :style="{ background: 'var(--border-default)' }"></div>
          <div data-test="traversal-positioner-indicator" class="flex items-center gap-1">
            <span
              class="h-2 w-2 rounded-full"
              :style="{ background: positionerConnection.dotColor, boxShadow: positionerConnection.dotGlow }"
            ></span>
            <span class="text-xs" :style="{ color: positionerConnection.textColor }">
              {{ positionerConnection.label }}
            </span>
          </div>
        </div>
      </header>

      <!-- 当前点位 -->
      <section class="monitor-section">
        <div class="monitor-section__title">{{ labels.currentPoint }}</div>
        <div class="grid grid-cols-2" :style="{ gap: 'var(--space-3)' }">
          <div>
            <div class="text-xs text-[var(--text-muted)]">{{ labels.alpha }}</div>
            <div class="font-mono text-base font-bold tabular-nums text-[var(--text-primary)]">{{ currentPointSummary.alpha }}°</div>
          </div>
          <div>
            <div class="text-xs text-[var(--text-muted)]">{{ labels.beta }}</div>
            <div class="font-mono text-base font-bold tabular-nums text-[var(--text-primary)]">{{ currentPointSummary.beta }}°</div>
          </div>
        </div>
      </section>

      <!-- 轴位置 -->
      <section v-if="axisPositions.length && hasConfig" class="monitor-section">
        <div class="monitor-section__title">{{ labels.positioner }}</div>
        <div class="flex flex-wrap gap-x-4 gap-y-1 font-mono text-sm tabular-nums">
          <div v-for="axis in axisPositions" :key="axis.label" class="flex items-center gap-1">
            <span class="text-xs text-[var(--text-muted)]">{{ axis.label }}:</span>
            <span
              class="text-sm"
              :class="axis.moving ? 'font-semibold' : ''"
              :style="{ color: axis.moving ? 'var(--state-success)' : 'var(--text-primary)' }"
            >
              {{ axis.position !== undefined ? axis.position.toFixed(2) : '--' }}
            </span>
          </div>
        </div>
      </section>

      <!-- 气动参数 -->
      <section class="monitor-section">
        <div class="monitor-section__title">{{ labels.realtimeCalculation }}</div>
        <div class="grid grid-cols-2" :style="{ gap: 'var(--space-1-5)' }">
          <div
            v-for="metric in [
              { label: labels.alpha, value: realtimeResult?.alpha?.toFixed(2), unit: '°', accent: true },
              { label: labels.beta, value: realtimeResult?.beta?.toFixed(2), unit: '°', accent: false },
              { label: labels.mach, value: realtimeResult?.machNumber?.toFixed(3), unit: '', accent: true },
              { label: labels.velocity, value: realtimeResult?.velocity?.toFixed(1), unit: '', accent: false },
              { label: 'P0', value: realtimeResult?.P0?.toFixed(2), unit: '', accent: true },
              { label: 'Ps', value: realtimeResult?.Ps?.toFixed(2), unit: '', accent: true },
            ]"
            :key="metric.label"
            class="monitor-metric"
          >
            <div class="text-xs text-[var(--text-muted)]">{{ metric.label }}</div>
            <div
              class="font-mono text-sm font-bold tabular-nums"
              :style="{ color: metric.accent ? 'var(--accent-info)' : 'var(--text-primary)' }"
            >
              {{ (metric.value ?? '--') + metric.unit }}
            </div>
          </div>
        </div>
      </section>

      <!-- 原始压力数据 -->
      <section class="monitor-section monitor-section--last">
        <div class="monitor-section__title monitor-section__title--with-icon">
          <ClipboardList class="h-3.5 w-3.5 text-[var(--text-muted)]" />
          <span>{{ labels.realtimePressureData }}</span>
        </div>
        <div class="grid grid-cols-2" :style="{ gap: 'var(--space-1)' }">
          <div
            v-for="item in pressureItems"
            :key="item.key"
            class="monitor-metric"
            :style="{
              borderColor: item.disabled
                ? 'color-mix(in srgb, var(--border-default) 50%, transparent)'
                : 'color-mix(in srgb, var(--accent-info) 25%, transparent)',
              background: item.disabled
                ? 'color-mix(in srgb, var(--bg-panel-strong) 40%, transparent)'
                : 'color-mix(in srgb, var(--accent-info) 6%, transparent)',
            }"
          >
            <div class="flex items-center justify-between">
              <span
                class="text-xs font-medium"
                :style="{ color: item.disabled ? 'var(--text-muted)' : 'var(--text-secondary)' }"
              >{{ item.label }}</span>
              <span
                v-if="!item.disabled"
                class="text-xs"
                :style="{ color: 'var(--text-muted)' }"
              >{{ item.unit }}</span>
            </div>
            <div
              class="font-mono text-sm font-semibold tabular-nums"
              :style="{ color: item.disabled ? 'var(--text-muted)' : 'var(--text-primary)' }"
            >{{ item.value }}</div>
          </div>
        </div>
      </section>
    </section>
  </aside>
</template>

<style scoped>
/* 章节统一间距、统一小标题 */
.monitor-section {
  padding-bottom: var(--space-3);
  margin-bottom: var(--space-3);
  border-bottom: 1px solid color-mix(in srgb, var(--border-default) 40%, transparent);
}

.monitor-section--last {
  padding-bottom: 0;
  margin-bottom: 0;
  border-bottom: none;
}

.monitor-section__title {
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--text-muted);
  margin-bottom: var(--space-2);
  letter-spacing: 0;
  text-transform: none;
}

.monitor-section__title--with-icon {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

/* 度量小卡片：统一 padding，边框默认透明（用于压力数据外的卡片） */
.monitor-metric {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  padding: 0.375rem 0.5rem;
  border-radius: 0.375rem;
  border: 1px solid color-mix(in srgb, var(--border-default) 50%, transparent);
  background: color-mix(in srgb, var(--bg-panel-strong) 60%, transparent);
  min-width: 0;
}
</style>
