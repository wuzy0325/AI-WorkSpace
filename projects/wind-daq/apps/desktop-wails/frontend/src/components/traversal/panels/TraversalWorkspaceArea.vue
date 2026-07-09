<script setup lang="ts">
/**
 * Right-side workspace area: tab switcher + content (preview / visualization / reference).
 * Phase C: segmented-control tab style aligned with MainTopBar; tighter header.
 */
import { Settings, ClipboardList, LayoutGrid, BarChart3, BookOpen } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import PointsPreview from '../PointsPreview.vue'
import ProbeReferenceCard from '../ProbeReferenceCard.vue'
import TraversalVisualization from '../visualization/TraversalVisualization.vue'
import type { TraversalTestConfig, TraversalPointPhase } from '@shared/types/traversal'

export type WorkspaceTab = 'preview' | 'visualization' | 'reference'

interface TabOption {
  value: WorkspaceTab
  label: string
}

defineProps<{
  activeTab: WorkspaceTab
  tabs: TabOption[]
  tabMeta: { title: string; subtitle: string }
  currentConfig: TraversalTestConfig | null
  currentPoint: { alpha: number; beta: number } | undefined
  completedPoints: number | undefined
  currentPointPhase: TraversalPointPhase | undefined
  labels: {
    moving: string
    stabilizing: string
    acquiring: string
    completed: string
    untested: string
    noLayoutConfigured: string
    pleaseConfigureLayout: string
    configureLayout: string
  }
}>()

defineEmits<{
  'update:activeTab': [tab: WorkspaceTab]
  'open-settings': []
}>()

const tabIcon = {
  preview: LayoutGrid,
  visualization: BarChart3,
  reference: BookOpen,
} as const
</script>

<template>
  <div data-test="traversal-workspace-primary" class="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--bg-panel)]">
    <!-- 工作区头部：扁平 Tab 导航 -->
    <div
      class="flex flex-wrap items-center justify-between gap-3 border-b flex-shrink-0"
      :style="{
        borderColor: 'var(--border-default)',
        background: 'var(--bg-panel)',
        padding: 'var(--space-2) var(--space-4)',
      }"
    >
      <div class="flex items-center gap-2">
        <div class="h-4 w-1 rounded-full" :style="{ background: 'var(--accent-info)' }"></div>
        <div>
          <h3 class="text-sm font-semibold leading-tight text-[var(--text-primary)]">{{ tabMeta.title }}</h3>
          <p class="text-[10px] leading-tight text-[var(--text-muted)]">{{ tabMeta.subtitle }}</p>
        </div>
      </div>

      <!-- 三个 Tab 采用三孔校准同款下划线导航 -->
      <nav class="flex items-center">
        <button
          v-for="tab in tabs"
          :key="tab.value"
          type="button"
          class="traversal-tab-underline"
          :class="{ 'traversal-tab-underline--active': activeTab === tab.value }"
          :title="tab.label"
          @click="$emit('update:activeTab', tab.value)"
        >
          <component :is="tabIcon[tab.value]" class="h-4 w-4 flex-shrink-0" />
          <span>{{ tab.label }}</span>
        </button>
      </nav>
    </div>

    <!-- 内容区 -->
    <div class="flex-1 overflow-hidden relative">
      <!-- 点位预览图例：颜色须与 PointsPreview.vue Canvas 绘制保持同步 -->
      <div
        v-if="activeTab === 'preview'"
        class="absolute right-4 top-3 z-10 flex items-center gap-3 rounded-full border px-3 py-1 text-[10px]"
        :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel)' }"
      >
        <div class="flex items-center gap-1">
          <span class="h-2 w-2 rounded-full" style="background: #3b82f6"></span>
          <span class="text-[var(--text-muted)]">{{ labels.moving }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-2 w-2 rounded-full" style="background: #fbbf24"></span>
          <span class="text-[var(--text-muted)]">{{ labels.stabilizing }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-2 w-2 rounded-full" style="background: #10b981"></span>
          <span class="text-[var(--text-muted)]">{{ labels.acquiring }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-2 w-2 rounded-full" style="background: #8b5cf6"></span>
          <span class="text-[var(--text-muted)]">{{ labels.completed }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-2 w-2 rounded-full" style="background: rgba(148, 163, 184, 0.3)"></span>
          <span class="text-[var(--text-muted)]">{{ labels.untested }}</span>
        </div>
      </div>

      <template v-if="activeTab === 'preview'">
        <!-- 布点画布用卡片容器居中展示，四周留 margin，避免直接平铺到画面边缘 -->
        <div
          v-if="currentConfig?.layout"
          class="absolute inset-4 rounded-xl border border-[var(--border-default)] bg-[var(--bg-panel)] p-3 shadow-[var(--shadow-panel)] overflow-hidden"
        >
          <PointsPreview
            :layout="currentConfig.layout"
            :current-point="currentPoint"
            :completed-points="completedPoints"
            :current-point-phase="currentPointPhase"
            :visible="activeTab === 'preview'"
          />
        </div>
        <div v-else class="flex h-full w-full flex-col items-center justify-center gap-4 text-center">
          <div class="flex h-16 w-16 items-center justify-center rounded-2xl" :style="{ background: 'var(--bg-panel-strong)' }">
            <ClipboardList class="h-8 w-8 text-[var(--text-muted)]" />
          </div>
          <div>
            <div class="text-sm font-medium text-[var(--text-secondary)]">{{ labels.noLayoutConfigured }}</div>
            <div class="mt-1 text-xs text-[var(--text-muted)]">{{ labels.pleaseConfigureLayout }}</div>
          </div>
          <UiButton variant="primary" size="sm" @click="$emit('open-settings')">
            <template #icon>
              <Settings class="h-3.5 w-3.5" />
            </template>
            {{ labels.configureLayout }}
          </UiButton>
        </div>
      </template>
      <div v-else-if="activeTab === 'visualization'" class="h-full p-4">
        <TraversalVisualization />
      </div>
      <div v-else class="h-full overflow-auto p-4">
        <ProbeReferenceCard />
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 三孔校准同款下划线 Tab */
.traversal-tab-underline {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.625rem 1rem;
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-size: var(--font-size-xs);
  font-weight: 500;
  line-height: 1;
  cursor: pointer;
  transition: color 160ms ease;
  white-space: nowrap;
}

.traversal-tab-underline:hover {
  color: var(--text-primary);
}

.traversal-tab-underline--active,
.traversal-tab-underline--active:hover {
  color: var(--accent-primary);
}

.traversal-tab-underline--active::after {
  content: '';
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 2px;
  background: var(--accent-primary);
  border-radius: 2px 2px 0 0;
}
</style>