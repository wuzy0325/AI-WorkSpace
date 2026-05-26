<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import { useTraversalStore } from '@stores/traversalStore'
import CrossSectionView from './CrossSectionView.vue'
import HeatmapView from './HeatmapView.vue'
import PressureRadarView from './PressureRadarView.vue'
import VectorFieldView from './VectorFieldView.vue'
import { VISUALIZATION_PARAM_CONFIG, type VisualizationParam } from './types'

type VisualizationTab = 'heatmap' | 'crossSection' | 'vectorField' | 'pressureRadar'

const traversalStore = useTraversalStore()
const i18n = useI18nStore()
const t = computed(() => i18n.t)

const activeTab = ref<VisualizationTab>('heatmap')
const selectedParam = ref<VisualizationParam>('machNumber')

const tabs = computed<Array<{ value: VisualizationTab; label: string }>>(() => [
  { value: 'heatmap', label: t.value.heatmap },
  { value: 'crossSection', label: t.value.crossSection },
  { value: 'vectorField', label: t.value.vectorField },
  { value: 'pressureRadar', label: t.value.pressureRadar }
])

const paramOptions = computed<Array<{ value: VisualizationParam; label: string }>>(() => (
  Object.entries(VISUALIZATION_PARAM_CONFIG).map(([value, config]) => ({
    value: value as VisualizationParam,
    label: t.value[config.labelKey] ?? config.fallbackLabel
  }))
))

const dataPoints = computed(() => traversalStore.dataPoints)
const hasData = computed(() => dataPoints.value.length > 0)
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="mb-3 flex flex-wrap items-center gap-2 border-b border-[color:var(--border-default)] pb-3">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        class="rounded-lg px-3 py-1.5 text-xs font-semibold transition-colors"
        :class="activeTab === tab.value ? 'bg-blue-500 text-white' : 'text-[color:var(--text-secondary)] hover:bg-[color:var(--bg-panel-strong)]'"
        @click="activeTab = tab.value"
      >
        {{ tab.label }}
      </button>

      <select
        v-if="activeTab === 'heatmap' || activeTab === 'crossSection'"
        v-model="selectedParam"
        class="ml-auto rounded-lg border border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-3 py-1.5 text-xs text-[color:var(--text-primary)]"
      >
        <option v-for="option in paramOptions" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
    </div>

    <div class="min-h-0 flex-1 overflow-hidden">
      <div v-if="!hasData" class="flex h-full min-h-[360px] flex-col items-center justify-center gap-2 text-center text-[color:var(--text-muted)]">
        <p class="text-sm font-semibold">{{ t.noVisualizationData }}</p>
        <p class="text-xs">{{ t.startTestForVisualization }}</p>
      </div>

      <template v-else>
        <HeatmapView v-if="activeTab === 'heatmap'" :data-points="dataPoints" :param="selectedParam" />
        <CrossSectionView v-else-if="activeTab === 'crossSection'" :data-points="dataPoints" :param="selectedParam" />
        <VectorFieldView v-else-if="activeTab === 'vectorField'" :data-points="dataPoints" />
        <PressureRadarView v-else :data-points="dataPoints" />
      </template>
    </div>
  </div>
</template>

