<script setup lang="ts">
import { Activity, Play } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'
import type { TraversalCheckpoint } from '@shared/types/traversal'

defineProps<{
  checkpoint: TraversalCheckpoint
  /** All visible strings come from i18n; parent passes the slice it needs. */
  labels: {
    detected: string
    completed: string
    config: string
    continueTest: string
    abandon: string
    // I-28 修复：原模板硬编码 'Unknown'，破坏 i18n 一致性。
    // 由父组件传入 i18n.t.travUnknownConfig，让中英文环境下都有正确翻译。
    unknown: string
  }
}>()

defineEmits<{
  resume: []
  discard: []
}>()
</script>

<template>
  <div
    class="flex items-center justify-between border-b px-4 py-2"
    :style="{
      borderColor: 'color-mix(in srgb, var(--state-warning) 30%, transparent)',
      background: 'color-mix(in srgb, var(--state-warning) 10%, transparent)',
    }"
  >
    <div class="flex items-center gap-2.5">
      <div
        class="flex h-7 w-7 items-center justify-center rounded-full text-[var(--state-warning)]"
        :style="{ background: 'color-mix(in srgb, var(--state-warning) 18%, transparent)' }"
      >
        <Activity class="h-3.5 w-3.5" />
      </div>
      <div>
        <div class="text-xs font-medium text-[var(--text-primary)]">{{ labels.detected }}</div>
        <div class="text-[10px] text-[var(--text-muted)]">
          {{ labels.completed }} {{ checkpoint.completedPoints }} / {{ checkpoint.totalPoints }} ·
          {{ labels.config }} {{ checkpoint.config?.name || labels.unknown }}
        </div>
      </div>
    </div>
    <div class="flex items-center gap-1.5">
      <UiButton variant="warning" size="sm" @click="$emit('resume')">
        <template #icon>
          <Play class="h-3 w-3 fill-current" />
        </template>
        {{ labels.continueTest }}
      </UiButton>
      <UiButton quaternary size="sm" @click="$emit('discard')">
        {{ labels.abandon }}
      </UiButton>
    </div>
  </div>
</template>
