<script setup lang="ts">
/**
 * 运动坐标模式选择（共享组件）
 *
 * 在五孔 / 三孔 / 总压 / 七孔校准配置界面复用，切换测点坐标的执行语义：
 *   - absolute（绝对坐标，默认）：点位坐标值作为运动目标绝对位置，直接走到该坐标。
 *   - relative（相对坐标）：点位坐标值作为相对当前位置的位移量，连续点位依次累积。
 *
 * 与后端 core/calibration.CoordinateMode 对齐：
 * 后端 moveToPoint / MoveToPointWithOrder 在相对模式下把目标换算为"当前坐标 + 测点坐标"。
 */
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18nStore } from '@stores/i18nStore'
import type { CalibrationCoordinateMode } from '@shared/types/calibration'
import UiSelect from '@components/ui/UiSelect.vue'

const props = defineProps<{
  /** 当前坐标模式，缺省视为 'absolute' */
  modelValue?: CalibrationCoordinateMode
  /** 是否禁用（如无运动轴绑定时） */
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: CalibrationCoordinateMode): void
}>()

const { t } = storeToRefs(useI18nStore())

const options = computed(() => [
  { value: 'absolute', label: t.value.coordinateModeAbsolute ?? '绝对坐标' },
  { value: 'relative', label: t.value.coordinateModeRelative ?? '相对坐标' },
])

function onChange(v: string) {
  emit('update:modelValue', v === 'relative' ? 'relative' : 'absolute')
}
</script>

<template>
  <div class="field">
    <span class="field-label">{{ t.coordinateMode ?? '坐标模式' }}</span>
    <UiSelect
      :model-value="modelValue === 'relative' ? 'relative' : 'absolute'"
      :options="options"
      :disabled="disabled"
      @update:model-value="onChange"
    />
    <span class="hint-text">{{ t.coordinateModeHint }}</span>
  </div>
</template>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.field-label {
  font-size: var(--text-xs);
  color: var(--text-tertiary);
}
</style>
