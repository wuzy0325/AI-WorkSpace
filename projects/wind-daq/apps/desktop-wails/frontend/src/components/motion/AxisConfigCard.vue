<script setup lang="ts">
import { computed, watch } from 'vue'
import type { AxisConfig, AxisKind, PositionSource, MotionControllerType } from '@shared/types/motion'
import { getAxisThemeClass, validateEncoderCompensation, type CompensationWarning } from './motionConfigEditor'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import { useI18nStore } from '@stores/i18nStore'

const props = defineProps<{
  axis: AxisConfig
  index: number
  controllerType: MotionControllerType
}>()

const emit = defineEmits<{
  update: [index: number, axis: AxisConfig]
}>()

const i18n = useI18nStore()

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

const compWarnings = computed<CompensationWarning[]>(() => {
  const cfg = localAxis.value.encoderCompensation
  if (!cfg || !cfg.enabled) return []
  return validateEncoderCompensation(cfg, localAxis.value)
})

// 参数帮助文本（随语言切换）
const fieldHelp = computed<Record<string, string>>(() => ({
  encoderScale: i18n.t.motion_help_encoderScale,
  tolerance: i18n.t.motion_help_tolerance,
  maxCycles: i18n.t.motion_help_maxCycles,
  settleMs: i18n.t.motion_help_settleMs,
  minStep: i18n.t.motion_help_minStep,
  timeoutMs: i18n.t.motion_help_timeoutMs,
}))

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
    :class="getAxisThemeClass(axis.name)"
  >
    <!-- 轴头部：名称 + 类型选择 -->
    <div class="axis-card__header">
      <div class="flex items-center gap-2">
        <div class="axis-card__badge">{{ axis.name }}</div>
        <UiSelect
          :model-value="axis.kind"
          @update:model-value="updateField('kind', $event as AxisKind)"
          width-class="w-20"
          :aria-label="`${axis.name} ${i18n.t.motion_axisKind}`"
          :data-test="`motion-axis-${axis.name}-kind`"
          :options="[
            { value: 'LINEAR', label: i18n.t.motion_linearAxis },
            { value: 'ROTARY', label: i18n.t.motion_rotaryAxis },
          ]"
        />
      </div>
    </div>

    <!-- 参数网格 -->
    <div class="axis-card__body">
      <div class="axis-card__grid">
        <div class="axis-card__field">
          <span class="axis-card__field-label">{{ i18n.t.motion_stepAngle }}</span>
          <UiInputNumber
            :model-value="axis.stepsPerRev ?? 1.8"
            @update:model-value="updateField('stepsPerRev', $event ?? 1.8)"
            class="axis-card__field-input w-16"
            :min="0.1"
            :step="0.1"
          />
        </div>
        <div class="axis-card__field">
          <span class="axis-card__field-label">{{ i18n.t.motion_microSteps }}</span>
          <UiInputNumber
            :model-value="axis.microSteps ?? 4"
            @update:model-value="updateField('microSteps', $event ?? 4)"
            class="axis-card__field-input w-16"
            :min="1"
            :step="1"
          />
        </div>
        <div class="axis-card__field">
          <span class="axis-card__field-label">
            {{ axis.kind === 'ROTARY' ? i18n.t.motion_gearRatio : i18n.t.motion_lead }}
          </span>
          <UiInputNumber
            v-if="axis.kind === 'ROTARY'"
            :model-value="axis.gearRatio ?? 1"
            @update:model-value="updateField('gearRatio', $event ?? 1)"
            class="axis-card__field-input w-16"
            :min="0.001"
            :step="0.001"
          />
          <UiInputNumber
            v-else
            :model-value="axis.lead ?? 4"
            @update:model-value="updateField('lead', $event ?? 4)"
            class="axis-card__field-input w-16"
            :min="0.001"
            :step="0.001"
          />
        </div>
        <div class="axis-card__field">
          <span class="axis-card__field-label axis-card__field-label--highlight">
            {{ axis.kind === 'ROTARY' ? i18n.t.motion_maxRpm : i18n.t.motion_maxSpeed }}
          </span>
          <UiInputNumber
            :model-value="axis.maxSpeed ?? 100"
            @update:model-value="updateField('maxSpeed', $event ?? 100)"
            class="axis-card__field-input w-16 axis-card__field-input--highlight"
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
          />
          <span class="axis-card__footer-label">{{ i18n.t.motion_directionInverted }}</span>
        </label>
        <div class="axis-card__footer-item">
          <span class="axis-card__footer-label">{{ i18n.t.motion_positionSource }}</span>
          <UiSelect
            v-model="positionSourceModel"
            class="w-20"
            :aria-label="`${axis.name} ${i18n.t.motion_positionSource}`"
            :data-test="`motion-axis-${axis.name}-position-source`"
            :disabled="!supportsEncoder"
            :options="[
              { value: 'register', label: i18n.t.motion_register },
              { value: 'encoder', label: i18n.t.motion_encoder },
            ]"
          />
        </div>
      </div>

      <!-- 编码器补偿区域（仅 B140-MC 且位置来源为编码器时显示） -->
      <div v-if="supportsEncoder && axis.positionSource === 'encoder'" class="encoder-section">
        <div class="encoder-section__header">
          <span class="encoder-section__title">{{ i18n.t.motion_encoderCompensation }}</span>
          <label class="flex items-center gap-1 cursor-pointer">
            <UiCheckbox
              :checked="encoderComp.enabled"
              @update:checked="updateCompensationField('enabled', $event)"
            />
            <span class="encoder-section__label">{{ i18n.t.motion_enable }}</span>
          </label>
        </div>
        <div class="encoder-section__row">
          <div class="encoder-section__field encoder-section__field--scale">
            <span class="encoder-section__label">
              {{ i18n.t.motion_encoderResolution }}
              <span
                class="enc-help"
                :title="fieldHelp['encoderScale']"
              >?</span>
            </span>
            <UiInputNumber
              :model-value="axis.encoderScale ?? 0.005"
              @update:model-value="updateField('encoderScale', $event ?? 0.005)"
              class="encoder-section__input"
              :min="0.0001"
              :step="0.0001"
            />
          </div>
        </div>
        <div v-if="encoderComp.enabled" class="encoder-section__grid">
          <div class="encoder-section__field">
            <span class="encoder-section__label">
              {{ i18n.t.motion_tolerance }}
              <span
                class="enc-help"
                :title="fieldHelp['tolerance']"
              >?</span>
            </span>
            <UiInputNumber
              :model-value="encoderComp.tolerance"
              @update:model-value="updateCompensationField('tolerance', $event ?? 0.01)"
              class="encoder-section__input"
              :min="0.0001"
              :step="0.001"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">
              {{ i18n.t.motion_maxCycles }}
              <span
                class="enc-help"
                :title="fieldHelp['maxCycles']"
              >?</span>
            </span>
            <UiInputNumber
              :model-value="encoderComp.maxCycles"
              @update:model-value="updateCompensationField('maxCycles', $event ?? 3)"
              class="encoder-section__input"
              :min="1"
              :step="1"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">
              {{ i18n.t.motion_settleMs }}
              <span
                class="enc-help"
                :title="fieldHelp['settleMs']"
              >?</span>
            </span>
            <UiInputNumber
              :model-value="encoderComp.settleMs"
              @update:model-value="updateCompensationField('settleMs', $event ?? 100)"
              class="encoder-section__input"
              :min="10"
              :step="10"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">
              {{ i18n.t.motion_minStep }}
              <span
                class="enc-help"
                :title="fieldHelp['minStep']"
              >?</span>
            </span>
            <UiInputNumber
              :model-value="encoderComp.minStep"
              @update:model-value="updateCompensationField('minStep', $event ?? 0.001)"
              class="encoder-section__input"
              :min="0.0001"
              :step="0.0001"
            />
          </div>
          <div class="encoder-section__field">
            <span class="encoder-section__label">
              {{ i18n.t.motion_timeoutMs }}
              <span
                class="enc-help"
                :title="fieldHelp['timeoutMs']"
              >?</span>
            </span>
            <UiInputNumber
              :model-value="encoderComp.timeoutMs"
              @update:model-value="updateCompensationField('timeoutMs', $event ?? 5000)"
              class="encoder-section__input"
              :min="100"
              :step="100"
            />
          </div>
        </div>

        <!-- 校验警告 -->
        <div v-if="compWarnings.length > 0" class="encoder-warnings">
          <div
            v-for="(w, wi) in compWarnings"
            :key="wi"
            class="encoder-warning"
            :class="'encoder-warning--' + w.severity"
          >
            <span class="encoder-warning__icon">{{ w.severity === 'error' ? '!' : '△' }}</span>
            <span class="encoder-warning__text">{{ w.message }}</span>
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
.axis-card__body {
  padding: var(--space-2) var(--space-3);
}
.axis-card__grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-1-5) var(--space-3);
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

/* 帮助提示图标 ? */
.enc-help {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  font-size: 0.625rem;
  font-weight: 700;
  color: var(--text-muted);
  background: color-mix(in srgb, var(--accent-info) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-info) 20%, transparent);
  cursor: help;
  vertical-align: middle;
  margin-left: 0.25rem;
  transition: all 0.15s ease;
}
.enc-help:hover {
  color: var(--accent-info);
  border-color: var(--accent-info);
  background: color-mix(in srgb, var(--accent-info) 15%, transparent);
}

/* 校验警告 */
.encoder-warnings {
  margin-top: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}
.encoder-warning {
  display: flex;
  align-items: flex-start;
  gap: 0.375rem;
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-2xs);
  line-height: 1.4;
}
.encoder-warning--error {
  color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 20%, transparent);
}
.encoder-warning--warning {
  color: var(--text-muted);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
}
.encoder-warning__icon {
  flex-shrink: 0;
  margin-top: 1px;
  font-weight: 700;
}
.encoder-warning__text {
  flex: 1;
}

@keyframes encoderFadeIn {
  from { opacity: 0; transform: translateY(-3px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
