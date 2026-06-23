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
  <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
    <div data-test="traversal-workspace-primary" class="min-h-0 flex-1">
      <section
        class="h-full flex flex-col rounded-xl border shadow-sm"
        :style="{ borderColor: 'var(--border-default)', background: 'var(--bg-panel)' }"
      >
        <!-- 工作区头部 -->
        <div
          class="flex flex-wrap items-center justify-between gap-3 border-b"
          :style="{
            borderColor: 'var(--border-default)',
            background: 'var(--bg-panel-strong)',
            padding: 'var(--space-2-5) var(--space-4)',
          }"
        >
          <div class="flex items-center gap-2">
            <div
              class="h-5 w-1 rounded-full"
              :style="{ background: 'var(--accent-info)' }"
            ></div>
            <div>
              <h3 class="text-sm font-semibold leading-tight text-[var(--text-primary)]">{{ tabMeta.title }}</h3>
              <p class="text-[10px] leading-tight text-[var(--text-muted)]">{{ tabMeta.subtitle }}</p>
            </div>
          </div>

          <!-- Tab segmented control -->
          <nav class="traversal-tabs">
            <button
              v-for="tab in tabs"
              :key="tab.value"
              type="button"
              class="traversal-tabs__btn"
              :class="{ 'traversal-tabs__btn--active': activeTab === tab.value }"
              :title="tab.label"
              @click="$emit('update:activeTab', tab.value)"
            >
              <component :is="tabIcon[tab.value]" class="traversal-tabs__icon" />
              <span class="traversal-tabs__label">{{ tab.label }}</span>
            </button>
          </nav>
        </div>

        <!-- 内容区 -->
        <div class="flex-1 overflow-hidden relative">
          <!-- 点位预览图例 -->
          <div
            v-if="activeTab === 'preview'"
            class="absolute right-4 top-3 z-10 flex items-center gap-3 rounded-full border px-3 py-1.5 text-[10px] shadow-sm backdrop-blur"
            :style="{ borderColor: 'var(--border-default)', background: 'color-mix(in srgb, var(--bg-panel) 90%, transparent)' }"
          >
            <div class="flex items-center gap-1">
              <span class="h-2 w-2 rounded-full" style="background: var(--accent-info)"></span>
              <span class="text-[var(--text-muted)]">{{ labels.moving }}</span>
            </div>
            <div class="flex items-center gap-1">
              <span class="h-2 w-2 rounded-full" style="background: var(--state-warning)"></span>
              <span class="text-[var(--text-muted)]">{{ labels.stabilizing }}</span>
            </div>
            <div class="flex items-center gap-1">
              <span class="h-2 w-2 rounded-full" style="background: var(--state-success)"></span>
              <span class="text-[var(--text-muted)]">{{ labels.acquiring }}</span>
            </div>
            <div class="flex items-center gap-1">
              <span
                class="h-2 w-2 rounded-full"
                style="background: linear-gradient(90deg, var(--accent-primary), var(--state-error))"
              ></span>
              <span class="text-[var(--text-muted)]">{{ labels.completed }}</span>
            </div>
            <div class="flex items-center gap-1">
              <span class="h-2 w-2 rounded-full" :style="{ background: 'var(--text-muted)' }"></span>
              <span class="text-[var(--text-muted)]">{{ labels.untested }}</span>
            </div>
          </div>

          <template v-if="activeTab === 'preview'">
            <PointsPreview
              v-if="currentConfig?.layout"
              :layout="currentConfig.layout"
              :current-point="currentPoint"
              :completed-points="completedPoints"
              :current-point-phase="currentPointPhase"
              :visible="activeTab === 'preview'"
            />
            <!-- 空状态 -->
            <div v-else class="flex h-full w-full flex-col items-center justify-center gap-4 text-center">
              <div
                class="flex h-16 w-16 items-center justify-center rounded-2xl"
                :style="{ background: 'var(--bg-panel-strong)' }"
              >
                <ClipboardList class="h-8 w-8 text-[var(--text-muted)]" />
              </div>
              <div>
                <div class="text-sm font-medium text-[var(--text-secondary)]">{{ labels.noLayoutConfigured }}</div>
                <div class="mt-1 text-xs text-[var(--text-muted)]">{{ labels.pleaseConfigureLayout }}</div>
              </div>
              <UiButton
                variant="primary" size="sm"
                @click="$emit('open-settings')"
              >
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
      </section>
    </div>
  </div>
</template>

<style scoped>
/* Segmented control，与 MainTopBar 视图切换风格一致 */
.traversal-tabs {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 3px;
  border-radius: 10px;
  background: var(--bg-canvas);
  border: 1px solid var(--border-default);
  box-shadow: inset 0 1px 2px color-mix(in srgb, var(--text-primary) 4%, transparent);
  flex-shrink: 0;
}

:root[data-theme='light'] .traversal-tabs {
  background: #eef2f7;
  border-color: #d8dee8;
  box-shadow: inset 0 1px 2px rgba(15, 23, 42, 0.05);
}

.traversal-tabs__btn {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.375rem 0.75rem;
  border-radius: 7px;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 500;
  line-height: 1;
  cursor: pointer;
  transition: color 160ms ease, background 160ms ease, box-shadow 160ms ease;
  white-space: nowrap;
}

.traversal-tabs__icon {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  stroke-width: 2;
}

.traversal-tabs__label {
  line-height: 1;
}

.traversal-tabs__btn:hover {
  color: var(--text-primary);
  background: color-mix(in srgb, var(--text-primary) 6%, transparent);
}

:root[data-theme='light'] .traversal-tabs__btn:hover {
  background: rgba(15, 23, 42, 0.04);
}

.traversal-tabs__btn--active,
.traversal-tabs__btn--active:hover {
  background: var(--bg-panel);
  color: var(--accent-info);
  font-weight: 600;
  box-shadow:
    0 1px 2px rgba(15, 23, 42, 0.08),
    0 0 0 1px color-mix(in srgb, var(--accent-info) 22%, transparent);
}

:root[data-theme='light'] .traversal-tabs__btn--active,
:root[data-theme='light'] .traversal-tabs__btn--active:hover {
  background: #ffffff;
  color: #0369a1;
  box-shadow:
    0 1px 2px rgba(15, 23, 42, 0.1),
    0 0 0 1px rgba(2, 132, 199, 0.25);
}

.traversal-tabs__btn--active .traversal-tabs__icon {
  stroke-width: 2.25;
}
</style>