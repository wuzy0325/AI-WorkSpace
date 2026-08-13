<script setup lang="ts">
import { computed } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import UiSelect from '@components/ui/UiSelect.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiFormField from '@components/ui/UiFormField.vue'
import { Cpu } from '@lucide/vue'

// ============================================================
// DAQ-T-1602 专属配置面板（Modbus TCP 温度扫描阀）
// ------------------------------------------------------------
// 配置项仅 16 通道热电偶类型（2 卡 × 8 通道），无采样率/触发等配置
// （固件固定 ~100ms 采集周期）。类型码 0~7 ↔ J/K/T/E/R/S/B/N，
// 与后端 shared SDK 的 TypeCodes [16]uint8 一一对应。
// 布局与全局设置画面共享 form-card / UiFormField / form-fields 样式，
// 采用 4 列网格容纳 16 个通道下拉框。
// ============================================================

const props = withDefaults(
  defineProps<{
    typeCodes?: number[]
    disabled?: boolean
  }>(),
  {
    typeCodes: () => Array(16).fill(2),
    disabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:typeCodes', v: number[]): void
}>()

const i18n = useI18nStore()

// 通道数固定 16（卡1 CH0~7 + 卡2 CH0~7）
const CHANNEL_COUNT = 16

// 热电偶类型选项：value 与设备 Type Code 寄存器值一致（0=J 1=K 2=T 3=E 4=R 5=S 6=B 7=N）
const TC_TYPE_OPTIONS = [
  { value: '0', label: 'J' },
  { value: '1', label: 'K' },
  { value: '2', label: 'T' },
  { value: '3', label: 'E' },
  { value: '4', label: 'R' },
  { value: '5', label: 'S' },
  { value: '6', label: 'B' },
  { value: '7', label: 'N' },
]

const channels = computed(() => Array.from({ length: CHANNEL_COUNT }, (_, i) => i))

function codeAt(index: number): string {
  return String(props.typeCodes[index] ?? 2)
}

function updateCode(index: number, value: string) {
  const next = props.typeCodes.slice(0, CHANNEL_COUNT)
  while (next.length < CHANNEL_COUNT) next.push(2)
  next[index] = Number(value)
  emit('update:typeCodes', next)
}
</script>

<template>
  <UiPanel :segmented="false" class="form-card t1602-card">
    <template #header>
      <div class="card-head">
        <Cpu :size="15" />
        <span class="card-head__title">{{ i18n.t.dev_t1602_sectionTitle }}</span>
      </div>
    </template>

    <div class="form-fields t1602-grid">
      <UiFormField v-for="i in channels" :key="i" :label="`CH${String(i + 1).padStart(2, '0')}`">
        <UiSelect
          :model-value="codeAt(i)"
          :options="TC_TYPE_OPTIONS"
          :disabled="props.disabled"
          @update:model-value="updateCode(i, $event as string)"
        />
      </UiFormField>
    </div>
  </UiPanel>
</template>

<style scoped>
/* 4 列网格：16 个通道下拉框，字段内标签在上、控件在下 */
.t1602-card :deep(.form-fields.t1602-grid) {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--density-field-gap);
}

.t1602-card :deep(.ui-form-field.ui-form-field) {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--density-field-inline);
  margin-bottom: 0;
}

.t1602-card :deep(.ui-form-field__label) {
  padding-top: 0;
  text-align: left;
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-muted);
  letter-spacing: 0.02em;
}

/* 响应式：窄屏退化为 2 列，避免控件被压缩 */
@media (max-width: 640px) {
  .t1602-card :deep(.form-fields.t1602-grid) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
