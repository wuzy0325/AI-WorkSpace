<script setup lang="ts">
import { useI18nStore } from '@stores/i18nStore'
import UiToggle from '@components/ui/UiToggle.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiFormField from '@components/ui/UiFormField.vue'
import { Cpu } from '@lucide/vue'

// ============================================================
// DAQ-T-1603 专属配置面板
// ------------------------------------------------------------
// 与全局设置画面共享 form-card / UiFormField / form-fields 样式，
// 采用 2 列网格布局容纳字段（标签在上、控件在下，垂直堆叠）。
// 所有字段标签、placeholder 文案均走 i18nStore，随全局语言切换。
// ============================================================

const props = withDefaults(
  defineProps<{
    channelMask?: string
    samplingRate?: number
    showTimestamp?: boolean
  }>(),
  {
    channelMask: 'FFFF',
    samplingRate: 10,
    showTimestamp: false,
  },
)

const emit = defineEmits<{
  (e: 'update:channelMask', v: string): void
  (e: 'update:samplingRate', v: number): void
  (e: 'update:showTimestamp', v: boolean): void
}>()

const i18n = useI18nStore()
</script>

<template>
  <UiPanel :segmented="false" class="form-card t1603-card">
    <template #header>
      <div class="card-head">
        <Cpu :size="15" />
        <span class="card-head__title">{{ i18n.t.dev_t1603_sectionTitle }}</span>
      </div>
    </template>
    <div class="form-fields t1603-grid">
      <UiFormField :label="i18n.t.dev_t1603_channelMask">
        <UiInput
          :model-value="props.channelMask"
          :placeholder="i18n.t.dev_t1603_channelMaskPlaceholder"
          @update:model-value="emit('update:channelMask', $event as string)"
        />
      </UiFormField>

      <UiFormField :label="i18n.t.dev_t1603_samplingRate">
        <UiInputNumber
          :model-value="props.samplingRate"
          :min="1"
          :max="1000"
          @update:model-value="(v) => v !== null && emit('update:samplingRate', Math.max(1, Math.min(1000, v)))"
        />
      </UiFormField>

      <UiFormField :label="i18n.t.dev_t1603_showTimestamp">
        <UiToggle
          :model-value="props.showTimestamp"
          @update:model-value="emit('update:showTimestamp', $event)"
        />
      </UiFormField>
    </div>
  </UiPanel>
</template>

<style scoped>
/* 2 列网格：覆盖 .form-fields 默认的 flex column 布局
 * 字段内：标签在上、控件在下（垂直堆叠）。 */
.t1603-card :deep(.form-fields.t1603-grid) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--density-field-gap);
}

/* 字段内：label ↔ control 纵向 2px，标签左对齐 */
.t1603-card :deep(.ui-form-field.ui-form-field) {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--density-field-inline);
  margin-bottom: 0;
}

.t1603-card :deep(.ui-form-field__label) {
  padding-top: 0;
  text-align: left;
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-muted);
  letter-spacing: 0.02em;
}

.t1603-card :deep(.ui-form-field__error),
.t1603-card :deep(.ui-form-field__hint) {
  grid-column: auto;
  margin-top: var(--density-field-inline);
  font-size: var(--font-size-micro);
  line-height: var(--line-height-base);
}

/* 响应式：窄屏退化为 1 列，避免控件被压缩 */
@media (max-width: 640px) {
  .t1603-card :deep(.form-fields.t1603-grid) {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
