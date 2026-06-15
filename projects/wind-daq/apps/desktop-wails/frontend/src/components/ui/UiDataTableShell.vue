<script setup lang="ts">
import { NDataTable } from 'naive-ui'
import UiLoadingState from '@components/ui/UiLoadingState.vue'
import UiEmptyState from '@components/ui/UiEmptyState.vue'
import UiErrorState from '@components/ui/UiErrorState.vue'
import UiToolbar from '@components/ui/UiToolbar.vue'

withDefaults(
defineProps<{
  columns: any[]
  data: any[]
  loading?: boolean
  error?: string
  emptyTitle?: string
  emptyDescription?: string
}>(),
{ loading: false, error: '', emptyTitle: '暂无数据', emptyDescription: '' },
)
</script>

<template>
  <div class="ui-datatable-shell">
    <UiToolbar v-if="$slots.toolbar" class="ui-datatable-shell__toolbar">
      <slot name="toolbar" />
    </UiToolbar>

    <UiLoadingState v-if="loading && !data.length" :loading="true" text="正在加载数据..." />

    <UiErrorState v-else-if="error" title="加载失败" :message="error">
      <template v-if="$slots['error-action']" #action>
        <slot name="error-action" />
      </template>
    </UiErrorState>

    <UiEmptyState
      v-else-if="!loading && data.length === 0"
      :title="emptyTitle"
      :description="emptyDescription"
    >
      <template v-if="$slots['empty-action']" #action>
        <slot name="empty-action" />
      </template>
    </UiEmptyState>

    <NDataTable
      v-else
      size="small"
      :columns="columns"
      :data="data"
      :loading="loading && data.length > 0"
      :bordered="false"
      :single-line="true"
    />
  </div>
</template>

<style scoped>
.ui-datatable-shell {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}
.ui-datatable-shell__toolbar {
  margin-bottom: var(--space-1);
}
</style>
