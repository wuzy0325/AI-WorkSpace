<script setup lang="ts">
/**
 * 遍历测试左侧栏：统一卡片风格，紧凑无滚动。
 *
 * 结构（从顶到底）：
 *   - 目标点 + 实际位置（合并为单行大字卡片）
 *   - 实时插值计算（α/β/Ma/V/P0/Ps）
 *   - 实时压力数据（P1-P5/Patm/Tatm）
 *   - 底部信息条：CSV路径 + 验证警告 + 硬件状态
 *
 * 所有数据项统一使用 bg-panel-strong 圆角卡片，
 * 标签左/数值右，数值字号 20px+，整体填满侧边栏不溢出。
 */
import { Activity, AlertTriangle, Cpu, FileText, Wind } from '@lucide/vue'
import type { PressureItem } from '@composables/useTraversalRealtimeData'
import type { InterpolationResult } from '@shared/types/traversal'

interface ConnectionDisplay {
  label: string
  dotColor: string
  dotGlow: string
  textColor: string
}

interface ActualPosition {
  label: string
  position: number | undefined
  moving: boolean
}

defineProps<{
  targetPoint: { alpha: number; beta: number } | undefined
  actualPositions: ActualPosition[]
  machNumber: number | undefined
  velocity: number | undefined
  csvSavePath: string
  lastError: string
  validationWarnings: readonly string[] | undefined

  acquisitionConnection: ConnectionDisplay
  positionerConnection: ConnectionDisplay
  pressureItems: PressureItem[]
  realtimeResult: InterpolationResult | null

  labels: {
    target: string
    actual: string
    mach: string
    velocity: string
    realtimeCalculation: string
    realtimePressureData: string
    alpha: string
    beta: string
    csvPath: string
    validationWarnings: string
    hardwareStatus: string
    acquisitionDevice: string
    positionerDevice: string
    moving: string
  }
}>()
</script>

<template>
  <div class="flex h-full w-96 flex-col flex-shrink-0 border-r border-[var(--border-default)] bg-[var(--bg-panel)] overflow-hidden">
    <!-- 目标点 + 实际位置：同一张卡片内上下两段，结构完全对称（两列 + 大字号 + 标签一致） -->
    <section class="flex-shrink-0 border-b border-[var(--border-default)] p-2.5">
      <div class="rounded-xl bg-[var(--bg-panel-strong)] p-3 space-y-2">
        <!-- 目标位置：α / β 两列，与实际位置结构完全一致 -->
        <div class="grid grid-cols-2 gap-2">
          <div class="flex flex-col">
            <span class="mb-0.5 text-[10px] text-[var(--text-muted)]">{{ labels.target }} α</span>
            <span class="font-mono text-xl font-bold tabular-nums text-[var(--accent-info)]">
              {{ targetPoint ? targetPoint.alpha.toFixed(1) + '°' : '--' }}
            </span>
          </div>
          <div class="flex flex-col">
            <span class="mb-0.5 text-[10px] text-[var(--text-muted)]">{{ labels.target }} β</span>
            <span class="font-mono text-xl font-bold tabular-nums text-[var(--accent-info)]">
              {{ targetPoint ? targetPoint.beta.toFixed(1) + '°' : '--' }}
            </span>
          </div>
        </div>
        <!-- 实际位置：两列，标签/字号/颜色与目标位置对齐，运动中高亮 -->
        <div class="grid grid-cols-2 gap-2 border-t border-[var(--border-default)] pt-2">
          <template v-if="actualPositions.length">
            <div
              v-for="axis in actualPositions"
              :key="axis.label"
              class="flex flex-col"
            >
              <div class="mb-0.5 flex items-center gap-1.5">
                <span class="text-[10px] text-[var(--text-muted)]">{{ labels.actual }} {{ axis.label }}</span>
                <span
                  v-if="axis.moving"
                  class="ml-auto flex items-center gap-1 text-[10px]"
                  :style="{ color: `var(--accent-success)` }"
                >
                  <span class="h-1.5 w-1.5 animate-pulse rounded-full" :style="{ backgroundColor: `var(--accent-success)` }"></span>
                  {{ labels.moving }}
                </span>
              </div>
              <span
                class="font-mono text-xl font-bold tabular-nums"
                :style="{ color: axis.moving ? `var(--accent-success)` : `var(--text-primary)` }"
              >
                {{ typeof axis.position === 'number' ? axis.position.toFixed(1) : '--' }}
              </span>
            </div>
          </template>
          <template v-else>
            <div
              v-for="placeholder in ['X', 'Y']"
              :key="placeholder"
              class="flex flex-col"
            >
              <span class="mb-0.5 text-[10px] text-[var(--text-muted)]">{{ labels.actual }} {{ placeholder }}</span>
              <span class="font-mono text-xl font-bold tabular-nums text-[var(--text-primary)]">--</span>
            </div>
          </template>
        </div>
      </div>
    </section>

    <!-- 实时插值计算：统一卡片，紧凑 py-1.5 -->
    <section class="flex-shrink-0 border-b border-[var(--border-default)] p-2.5">
      <div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--text-primary)]">
        <Activity class="h-3.5 w-3.5 text-[var(--accent-info)]" />
        {{ labels.realtimeCalculation }}
      </div>
      <div class="grid grid-cols-2 gap-1.5">
        <div
          v-for="metric in [
            { label: labels.alpha, value: realtimeResult?.alpha?.toFixed(2), unit: '°', accent: true },
            { label: labels.beta, value: realtimeResult?.beta?.toFixed(2), unit: '°', accent: true },
            { label: labels.mach, value: realtimeResult?.machNumber?.toFixed(3), unit: '', accent: true },
            { label: labels.velocity, value: realtimeResult?.velocity?.toFixed(1), unit: ' m/s', accent: true },
            { label: 'P0', value: realtimeResult?.P0?.toFixed(2), unit: '', accent: false },
            { label: 'Ps', value: realtimeResult?.Ps?.toFixed(2), unit: '', accent: false },
          ]"
          :key="metric.label"
          class="flex items-baseline justify-between rounded-lg bg-[var(--bg-panel-strong)] px-3 py-1.5 min-w-0"
        >
          <span class="text-[11px] text-[var(--text-muted)]">{{ metric.label }}</span>
          <span class="font-mono text-lg font-bold tabular-nums truncate" :style="{ color: metric.accent ? 'var(--accent-info)' : 'var(--text-primary)' }">
            {{ (metric.value ?? '--') + metric.unit }}
          </span>
        </div>
      </div>
    </section>

    <!-- 实时压力数据：统一卡片，7 项分两列 -->
    <section class="flex-1 min-h-0 overflow-hidden flex flex-col p-2.5">
      <div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--text-primary)]">
        <Wind class="h-3.5 w-3.5 text-[var(--accent-info)]" />
        {{ labels.realtimePressureData }}
      </div>
      <div class="grid grid-cols-2 gap-1.5 content-start">
        <div
          v-for="item in pressureItems"
          :key="item.key"
          class="flex items-baseline justify-between rounded-lg px-3 py-1.5 min-w-0"
          :style="{
            background: item.disabled
              ? 'color-mix(in srgb, var(--bg-panel-strong) 40%, transparent)'
              : 'var(--bg-panel-strong)',
          }"
        >
          <span class="text-[11px] font-medium" :style="{ color: item.disabled ? 'var(--text-muted)' : 'var(--text-secondary)' }">{{ item.label }}</span>
          <span class="font-mono text-lg font-bold tabular-nums truncate" :style="{ color: item.disabled ? 'var(--text-muted)' : 'var(--text-primary)' }">{{ item.value }}</span>
        </div>
      </div>
    </section>

    <!-- 底部信息条：CSV路径 + 验证警告 + 错误 + 硬件状态（固定） -->
    <div class="flex-shrink-0 border-t border-[var(--border-default)] p-2.5 space-y-1.5">
      <!-- CSV 路径 -->
      <div
        v-if="csvSavePath"
        class="flex items-center gap-1.5 rounded-md bg-[var(--bg-panel-strong)] px-2.5 py-1"
        :title="csvSavePath"
      >
        <FileText class="h-3 w-3 text-[var(--text-muted)] flex-shrink-0" />
        <span class="text-[10px] text-[var(--text-muted)] flex-shrink-0">{{ labels.csvPath }}</span>
        <span class="text-[10px] text-[var(--text-secondary)] truncate">{{ csvSavePath }}</span>
      </div>

      <!-- 验证警告 -->
      <div
        v-if="validationWarnings?.length"
        class="flex items-center gap-1.5 rounded-md px-2.5 py-1"
        :style="{
          background: 'color-mix(in srgb, var(--state-warning) 10%, transparent)',
        }"
        :title="validationWarnings.join('\n')"
      >
        <AlertTriangle class="h-3 w-3 text-[var(--state-warning)] flex-shrink-0" />
        <span class="text-[10px] text-[var(--state-warning)]">
          {{ labels.validationWarnings.replace('{count}', String(validationWarnings.length)) }}
        </span>
      </div>

      <!-- 错误信息 -->
      <div
        v-if="lastError"
        class="flex items-center gap-1.5 rounded-md px-2.5 py-1"
        :style="{
          background: 'color-mix(in srgb, var(--accent-danger) 10%, transparent)',
        }"
        :title="lastError"
      >
        <span class="text-[10px] font-medium" :style="{ color: `var(--accent-danger)` }">⚠ {{ lastError.length > 36 ? lastError.slice(0, 36) + '...' : lastError }}</span>
      </div>

      <!-- 硬件状态：明确标注“采集设备/位移机构”，避免两个灯看不出含义 -->
      <div class="rounded-md bg-[var(--bg-panel-strong)] px-2.5 py-1.5">
        <div class="mb-1 flex items-center gap-1.5">
          <Cpu class="h-3 w-3 text-[var(--text-muted)]" />
          <span class="text-[10px] text-[var(--text-muted)]">{{ labels.hardwareStatus }}</span>
        </div>
        <div class="flex items-center justify-between gap-2 text-[10px]">
          <div class="flex items-center gap-1.5">
            <span class="text-[var(--text-secondary)]">{{ labels.acquisitionDevice }}</span>
            <span class="h-1.5 w-1.5 rounded-full" :style="{ background: acquisitionConnection.dotColor, boxShadow: acquisitionConnection.dotGlow }"></span>
            <span :style="{ color: acquisitionConnection.textColor }">{{ acquisitionConnection.label }}</span>
          </div>
          <div class="flex items-center gap-1.5">
            <span class="text-[var(--text-secondary)]">{{ labels.positionerDevice }}</span>
            <span class="h-1.5 w-1.5 rounded-full" :style="{ background: positionerConnection.dotColor, boxShadow: positionerConnection.dotGlow }"></span>
            <span :style="{ color: positionerConnection.textColor }">{{ positionerConnection.label }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
