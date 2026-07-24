<script setup lang="ts">
/**
 * 右侧工作区：Tab 切换 + 内容（点位预览 / 可视化 / 探针参考）。
 * 视觉风格与 TraversalTopBar 方案C保持一致：
 *   - 极简信息栏，去掉厚装饰条
 *   - 标题与 Tab 同排，用分隔线自然划分
 *   - 图例改为更轻量的浮动圆角条
 */
import { Settings, ClipboardList, LayoutGrid, BarChart3, BookOpen } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import PointsPreview from '../PointsPreview.vue'
import ProbeReferenceCard from '../ProbeReferenceCard.vue'
import TraversalVisualization from '../visualization/TraversalVisualization.vue'
import type { TraversalTestConfig, TraversalPointPhase, TraversalCoordPoint } from '@shared/types/traversal'

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
  // alpha/beta 允许 null：line 模式 Y 轴 NaN 序列化为 null
  currentPoint: TraversalCoordPoint | undefined
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
    <!-- 工作区头部：单行，标题 + 分隔线 + Tab 导航 -->
    <div
      class="flex items-center justify-between border-b px-5 py-2.5 flex-shrink-0"
      :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel)' }"
    >
      <div class="flex items-center gap-3">
        <div>
          <h3 class="text-sm font-semibold leading-tight text-[var(--text-primary)]">{{ tabMeta.title }}</h3>
          <p class="text-[11px] leading-tight text-[var(--text-muted)]">{{ tabMeta.subtitle }}</p>
        </div>
        <div class="h-4 w-px" :style="{ background: 'var(--border-default)' }"></div>
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
            <component :is="tabIcon[tab.value]" class="h-3.5 w-3.5 flex-shrink-0" />
            <span>{{ tab.label }}</span>
          </button>
        </nav>
      </div>
    </div>

    <!-- 内容区 -->
    <div class="flex-1 overflow-hidden relative">
      <!-- 点位预览图例：浮动在右上角，更轻量 -->
      <div
        v-if="activeTab === 'preview'"
        class="absolute right-4 top-3 z-10 flex items-center gap-2 rounded-full border px-2.5 py-1 text-[10px]"
        :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel)' }"
      >
        <div class="flex items-center gap-1">
          <span class="h-1.5 w-1.5 rounded-full" style="background: #3b82f6"></span>
          <span class="text-[var(--text-muted)]">{{ labels.moving }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-1.5 w-1.5 rounded-full" style="background: #fbbf24"></span>
          <span class="text-[var(--text-muted)]">{{ labels.stabilizing }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-1.5 w-1.5 rounded-full" style="background: #10b981"></span>
          <span class="text-[var(--text-muted)]">{{ labels.acquiring }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-1.5 w-1.5 rounded-full" style="background: #8b5cf6"></span>
          <span class="text-[var(--text-muted)]">{{ labels.completed }}</span>
        </div>
        <div class="flex items-center gap-1">
          <span class="h-1.5 w-1.5 rounded-full" style="background: rgba(148, 163, 184, 0.3)"></span>
          <span class="text-[var(--text-muted)]">{{ labels.untested }}</span>
        </div>
      </div>

      <template v-if="activeTab === 'preview'">
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
/* 与 ThreeHole 同款下划线 Tab，调整 padding 更紧凑 */
.traversal-tab-underline {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.375rem 0.75rem;
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
  right: 0.5rem;
  bottom: 0;
  left: 0.5rem;
  height: 2px;
  background: var(--accent-primary);
  border-radius: 2px 2px 0 0;
}
</style>