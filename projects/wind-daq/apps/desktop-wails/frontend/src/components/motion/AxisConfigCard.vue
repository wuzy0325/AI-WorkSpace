<script setup lang="ts">
import { computed, watch } from 'vue'
import type { AxisConfig, AxisKind, PositionSource, MotionControllerType } from '@shared/types/motion'
import { getAxisThemeClass } from './motionConfigEditor'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'

const props = defineProps<{
  axis: AxisConfig
  index: number
  controllerType: MotionControllerType
}>()

const emit = defineEmits<{
  update: [index: number, axis: AxisConfig]
}>()

const localAxis = computed<AxisConfig>({
  get: () => props.axis,
  set: (val) => emit('update', props.index, val),
})

const positionSourceModel = computed<PositionSource>({
  get: () => (localAxis.value.positionSource ?? 'register') as PositionSource,
  set: (next) => {
    localAxis.value = { ...localAxis.value, positionSource: next }
  },
})

const supportsEncoder = computed(() => props.controllerType === 'B140-MC')

const encoderComp = computed(() => localAxis.value.encoderCompensation!)

/**
 * 仅 B140-MC 支持编码器位置来源；当切换为其他设备类型时，
 * 强制当前轴回退到寄存器模式，避免遗留非法配置。
 */
watch(
  () => props.controllerType,
  (next) => {
    if (next !== 'B140-MC' && localAxis.value.positionSource !== 'register') {
      localAxis.value = { ...localAxis.value, positionSource: 'register' }
    }
  },
  { immediate: true }
)

function updateField<K extends keyof AxisConfig>(key: K, value: AxisConfig[K]): void {
  localAxis.value = { ...localAxis.value, [key]: value }
}

function updateCompensationField<K extends keyof NonNullable<AxisConfig['encoderCompensation']>>(
  key: K,
  value: NonNullable<AxisConfig['encoderCompensation']>[K]
): void {
  const base = localAxis.value.encoderCompensation!
  localAxis.value = {
    ...localAxis.value,
    encoderCompensation: { ...base, [key]: value },
  }
}
</script>

<template>
  <div
    class="axis-card"
    :class="[getAxisThemeClass(axis.name), { 'axis-card--disabled': !axis.enabled }]"
  >
    <!-- 轴头部：名称 + 类型选择 + 启用开关 -->
    <div class="axis-card__header">
      <div class="flex items-center gap-2">
        <div class="axis-card__badge">{{ axis.name }}</div>
        <UiSelect
          :model-value="axis.kind"
          @update:model-value="updateField('kind', $event as AxisKind)"
          width-class="w-20"
          :disabled="!axis.enabled"
          :options="[
            { value: 'LINEAR', label: '直线轴' },
            { value: 'ROTARY', label: '旋转轴' },
          ]"
        />
      </div>
      <label class="axis-card__toggle">
        <UiCheckbox
          :checked="axis.enabled"
          @update:checked="updateField('enabled', $event)"
        />
        <span>{{ axis.enabled ? '启用' : '禁用' }}</span>
      </label>
    </div>

    <!-- 参数网格 -->
    <div class="axis-card__body">
      <div class="axis-card__grid" :class="{ 'axis-card__grid--disabled': !axis.enabled }">
        <div class="axis-card__field">
          <span class="axis-card__field-label">步距角 °</span>
          <UiInputNumber
            :model-value="axis.stepsPerRev ?? 1.8"
            @update:model-value="updateField('stepsPerRev', $event ?? 1.8)"
            class="axis-card__field-input w-16"
            :disabled="!axis.enabled"
            :min="0.1"
            :step="0.1"
          />
        </div>
        <div class="axis-card__field">
          <span class="axis-card__field-label">细分数</span>
          <UiInputNumber
            :model-value="axis.microSteps ?? 4"
            @update:model-value="updateField('microSteps', $event ?? 4)"
            class="axis-card__field-input w-16"
            :disabled="!axis.enabled"
            :min="1"
            :step="1"
          />
        </div>
        <div class="axis-card__field">
          <span class="axis-card__field-label">
            {{ axis.kind === 'ROTARY' ? '减速比' : '导程 mm' }}
          </span>
          <UiInputNumber
            v-if="axis.kind === 'ROTARY'"
            :model-value="axis.gearRatio ?? 1"
            @update:model-value="updateField('gearRatio', $event ?? 1)"
            class="axis-card__field-input w-16"
            :disabled="!axis.enabled"
            :min="0.001"
            :step="0.001"
          />
          <UiInputNumber
            v-else
            :model-value="axis.lead ?? 4"
            @update:model-value="updateField('lead', $event ?? 4)"
            class="axis-card__field-input w-16"
            :disabled="!axis.enabled"
            :min="0.001"
            :step="0.001"
          />
        </div>
        <div class="axis-card__field">
          <span class="axis-card__field-label axis-card__field-label--highlight">
            {{ axis.kind === 'ROTARY' ? '最大转速' : '最大速度' }}
          </span>
          <UiInputNumber
            :model-value="axis.maxSpeed ?? 100"
            @update:model-value="updateField('maxSpeed', $event ?? 100)"
            class="axis-card__field-input w-16 axis-card__field-input--highlight"
            :disabled="!axis.enabled"
            :min="1"
            :step="1"
          />
        </div>
      </div>

      <!-- 底部选项：方向反转 + 位置来源（所有控制器都显示，仅 B140-MC 可选编码器） -->
      <div class="axis-card__footer">
        <label class="axis-card__footer-item cursor-pointer">
          <UiCheckbox
            :checked="axis.inverted"
            @update:checked="updateField('inverted', $event)"
            :disabled="!axis.enabled"
          />
          <span class="axis-card__footer-label">方向反转</span>
        </label>
        <div class="axis-card__footer-item">
          <span class="axis-card__footer-label">位置来源</span>
          <UiSelect
            v-model="positionSourceModel"
            class="w-20"
            :disabled="!axis.enabled || !supportsEncoder"
            :options="[
              { value: 'register', label: '寄存器' },
              { value: 'encoder', label: '编码器' },
            ]"
          />
        </div>
      </div>

      <!-- 编码器补偿区域（仅 B140-MC 且位置来源为编码器时显示） -->
      <div v-if="supportsEncoder && axis.positionSource === 'encoder'" class="encoder-section">
        <div class="encoder-section__header">
          <span class="encoder-section__title">编码器补偿</span>
          <label class="flex items-center gap-1 cursor-pointer">
            <UiCheckbox
              :checked="encoderComp.enabled"
              @update:checked="updateCompensationField('enabled', $event)"
              :disabled="!axis.enabled"
            />
            <span class="encoder-section__label">启用</span>
          </label>
        </div>
        <div class="encoder-section__row">
          <div class="encoder-section__field encoder-section__field--scale">
            <span class="encoder-section__label">编码器倍率</span>
            <UiInputNumber
              :model-value="axis.encoderScale ?? 1"
              @update:model-value="updateField('encoderScale', $event ?? 1)"
              class="encoder-section__input"
              :disabled="!axis.enabled"
              :min="0.0001"
              :step="0.0001"
            />
          </div>
        </div>
        <div class="encoder-section__grid">
          <div class="encoder-section__field">
            <span class="encoder-section__label">容差</span>
            <UiInputNumber
              :model-value="encoderComp.tolerance"
              @update:model-value="updateCompensationField('tolerance', $event ?? 0.01)"
              class="encoder-section__input"
              :disabled="!axis.enabled"
              :min="0.0001"
              :step="0.001"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">最大循环</span>
            <UiInputNumber
              :model-value="encoderComp.maxCycles"
              @update:model-value="updateCompensationField('maxCycles', $event ?? 10)"
              class="encoder-section__input"
              :disabled="!axis.enabled"
              :min="1"
              :step="1"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">稳定 ms</span>
            <UiInputNumber
              :model-value="encoderComp.settleMs"
              @update:model-value="updateCompensationField('settleMs', $event ?? 200)"
              class="encoder-section__input"
              :disabled="!axis.enabled"
              :min="10"
              :step="10"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">最小白</span>
            <UiInputNumber
              :model-value="encoderComp.minStep"
              @update:model-value="updateCompensationField('minStep', $event ?? 0.001)"
              class="encoder-section__input"
              :disabled="!axis.enabled"
              :min="0.0001"
              :step="0.0001"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">超时 ms</span>
            <UiInputNumber
              :model-value="encoderComp.timeoutMs"
              @update:model-value="updateCompensationField('timeoutMs', $event ?? 5000)"
              class="encoder-section__input"
              :disabled="!axis.enabled"
              :min="100"
              :step="100"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.axis-card {
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  overflow: hidden;
  transition: all 0.2s ease;
}
.axis-card:hover {
  border-color: var(--axis-hue);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--axis-hue) 8%, transparent);
}
.axis-card--disabled {
  opacity: 0.55;
}
.axis-card--disabled .axis-card__body {
  pointer-events: none;
}
.axis-x-theme { --axis-hue: var(--axis-x); --axis-hue-soft: var(--axis-x-soft); }
.axis-y-theme { --axis-hue: var(--axis-y); --axis-hue-soft: var(--axis-y-soft); }
.axis-z-theme { --axis-hue: var(--axis-z); --axis-hue-soft: var(--axis-z-soft); }
.axis-u-theme { --axis-hue: var(--axis-u); --axis-hue-soft: var(--axis-u-soft); }

.axis-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-1-5) var(--space-3);
  background: var(--axis-hue-soft);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.axis-card__badge {
  width: 1.5rem;
  height: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  background: var(--axis-hue-soft);
  color: var(--axis-hue);
  font-size: 0.875rem;
  font-weight: 800;
}
.axis-card__toggle {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  font-size: var(--font-size-2xs);
  font-weight: 600;
  color: var(--text-muted);
  cursor: pointer;
}

.axis-card__body {
  padding: var(--space-2) var(--space-3);
}
.axis-card__grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-1-5) var(--space-3);
}
.axis-card__grid--disabled {
  opacity: 0.55;
}
.axis-card__field {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}
.axis-card__field-label {
  font-size: var(--font-size-2xs);
  font-weight: 600;
  color: var(--text-muted);
  white-space: nowrap;
}
.axis-card__field-label--highlight {
  color: var(--axis-hue);
}
.axis-card__field-input.w-16 {
  width: 4rem;
}
.axis-card__field-input :deep(.n-input-number-input) {
  text-align: right;
  font-size: var(--font-size-xs);
}
.axis-card__field-input--highlight :deep(.n-input) {
  border-color: color-mix(in srgb, var(--axis-hue) 25%, transparent);
  background: color-mix(in srgb, var(--axis-hue-soft) 20%, transparent);
}

.axis-card__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: var(--space-1-5);
  padding-top: var(--space-1-5);
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  gap: var(--space-2);
}
.axis-card__footer-item {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}
.axis-card__footer-label {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
  white-space: nowrap;
}

.encoder-section {
  margin-top: var(--space-2);
  padding: var(--space-2);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--accent-info) 5%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-info) 15%, transparent);
  animation: encoderFadeIn 0.2s ease;
}
.encoder-section__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-2);
}
.encoder-section__title {
  font-size: var(--font-size-xs);
  font-weight: 700;
  color: var(--accent-info);
}
.encoder-section__row {
  margin-bottom: var(--space-2);
}
.encoder-section__grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--space-2);
}
.encoder-section__field {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}
.encoder-section__field--scale {
  max-width: 8rem;
}
.encoder-section__label {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
}
.encoder-section__input :deep(.n-input-number-input) {
  text-align: right;
  font-size: var(--font-size-2xs);
}

@keyframes encoderFadeIn {
  from { opacity: 0; transform: translateY(-3px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
