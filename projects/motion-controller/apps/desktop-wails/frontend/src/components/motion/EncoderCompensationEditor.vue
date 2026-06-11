<script setup lang="ts">
import { reactive, watch, computed } from 'vue'
import type { AxisConfig, AxisEncoderCompensationConfig } from '@shared/types/motion'
import { defaultEncComp } from './motionConfigEditor'

interface AxisCompState {
  enabled: boolean
  preset: string
  customParams: AxisEncoderCompensationConfig
}

const props = defineProps<{
  axes: AxisConfig[]
  controllerType: string
}>()

const emit = defineEmits<{
  'update-enc-comp': [index: number, value: AxisEncoderCompensationConfig]
}>()

// 使用 reactive Record 替代 ref(Map)，确保属性赋值触发 Vue 响应式更新
const axisConfigs: Record<string, AxisCompState> = reactive({})

// 初始化轴配置
watch(() => props.axes, (axes) => {
  for (const axis of axes) {
    if (!axisConfigs[axis.name]) {
      const defaultConfig = axis.encoderCompensation || defaultEncComp()
      axisConfigs[axis.name] = {
        enabled: defaultConfig.enabled ?? false,
        preset: 'default',
        customParams: { ...defaultConfig }
      }
    }
  }
}, { immediate: true })

const presetOptions = [
  { label: '默认', value: 'default' },
  { label: '高精度', value: 'high_precision' },
  { label: '高速', value: 'high_speed' },
  { label: '自定义', value: 'custom' }
]

// 预设配置
const COMPENSATION_PRESETS: Record<string, AxisEncoderCompensationConfig> = {
  default: { enabled: true, tolerance: 0.01, maxCycles: 10, settleMs: 100, minStep: 0.001, timeoutMs: 5000 },
  high_precision: { enabled: true, tolerance: 0.001, maxCycles: 20, settleMs: 200, minStep: 0.0001, timeoutMs: 10000 },
  high_speed: { enabled: true, tolerance: 0.05, maxCycles: 5, settleMs: 50, minStep: 0.01, timeoutMs: 3000 },
}

const DEFAULT_STATE: AxisCompState = {
  enabled: false,
  preset: 'default',
  customParams: { enabled: false, tolerance: 0.01, maxCycles: 10, settleMs: 100, minStep: 0.001, timeoutMs: 5000 }
}

function getAxisConfig(axisName: string): AxisCompState {
  return axisConfigs[axisName] || DEFAULT_STATE
}

function getAxisIndex(axisName: string): number {
  return props.axes.findIndex(a => a.name === axisName)
}

function onToggle(axisName: string, enabled: boolean) {
  const config = getAxisConfig(axisName)
  // 直接赋值触发 reactive 响应式更新
  axisConfigs[axisName] = { ...config, enabled }

  const preset = config.preset === 'custom'
    ? config.customParams
    : COMPENSATION_PRESETS[config.preset] || COMPENSATION_PRESETS.default

  emit('update-enc-comp', getAxisIndex(axisName), { ...preset, enabled })
}

function onPresetChange(axisName: string, presetKey: string) {
  const config = getAxisConfig(axisName)
  axisConfigs[axisName] = { ...config, preset: presetKey }

  if (config.enabled) {
    const preset = presetKey === 'custom'
      ? config.customParams
      : COMPENSATION_PRESETS[presetKey] || COMPENSATION_PRESETS.default
    emit('update-enc-comp', getAxisIndex(axisName), { ...preset, enabled: true })
  }
}

function onCustomParamChange(axisName: string, key: keyof AxisEncoderCompensationConfig, value: number | boolean) {
  const config = getAxisConfig(axisName)
  const newCustomParams = { ...config.customParams, [key]: value }
  axisConfigs[axisName] = {
    ...config,
    preset: 'custom',
    customParams: newCustomParams
  }

  if (config.enabled) {
    emit('update-enc-comp', getAxisIndex(axisName), { ...newCustomParams, enabled: true })
  }
}

const enabledAxes = computed(() => {
  return props.axes.filter(axis => axisConfigs[axis.name]?.enabled)
})
</script>

<template>
  <div class="enc-comp-editor">
    <div class="enc-comp-editor__header">
      <h4 class="enc-comp-editor__title">编码器补偿</h4>
      <span class="enc-comp-editor__subtitle">为各轴配置编码器补偿参数</span>
    </div>

    <div class="enc-comp-editor__axes">
      <div
        v-for="axis in axes"
        :key="axis.name"
        class="enc-comp-axis"
      >
        <div class="enc-comp-axis__header">
          <div class="enc-comp-axis__badge">{{ axis.name }}</div>
          <label class="enc-comp-axis__toggle">
            <span class="enc-comp-axis__toggle-label">启用补偿</span>
            <input
              type="checkbox"
              :checked="getAxisConfig(axis.name).enabled"
              @change="onToggle(axis.name, ($event.target as HTMLInputElement).checked)"
              class="enc-comp-axis__toggle-input"
            />
          </label>
        </div>

        <div v-if="getAxisConfig(axis.name).enabled" class="enc-comp-axis__body">
          <div class="enc-comp-axis__field">
            <label class="enc-comp-axis__field-label">预设方案</label>
            <select
              :value="getAxisConfig(axis.name).preset"
              @change="onPresetChange(axis.name, ($event.target as HTMLSelectElement).value)"
              class="enc-comp-axis__select config-select"
            >
              <option v-for="opt in presetOptions" :key="opt.value" :value="opt.value">
                {{ opt.label }}
              </option>
            </select>
          </div>

          <!-- 自定义参数 -->
          <div v-if="getAxisConfig(axis.name).preset === 'custom'" class="enc-comp-axis__custom">
            <div class="enc-comp-axis__custom-row">
              <label class="enc-comp-axis__field">
                <span class="enc-comp-axis__field-label">容差</span>
                <input
                  type="number"
                  :value="getAxisConfig(axis.name).customParams.tolerance"
                  @input="onCustomParamChange(axis.name, 'tolerance', Number(($event.target as HTMLInputElement).value))"
                  step="0.001"
                  min="0"
                  class="enc-comp-axis__input config-input"
                />
              </label>
              <label class="enc-comp-axis__field">
                <span class="enc-comp-axis__field-label">最大循环</span>
                <input
                  type="number"
                  :value="getAxisConfig(axis.name).customParams.maxCycles"
                  @input="onCustomParamChange(axis.name, 'maxCycles', Number(($event.target as HTMLInputElement).value))"
                  step="1"
                  min="1"
                  class="enc-comp-axis__input config-input"
                />
              </label>
            </div>
            <div class="enc-comp-axis__custom-row">
              <label class="enc-comp-axis__field">
                <span class="enc-comp-axis__field-label">稳定时间(ms)</span>
                <input
                  type="number"
                  :value="getAxisConfig(axis.name).customParams.settleMs"
                  @input="onCustomParamChange(axis.name, 'settleMs', Number(($event.target as HTMLInputElement).value))"
                  step="10"
                  min="0"
                  class="enc-comp-axis__input config-input"
                />
              </label>
              <label class="enc-comp-axis__field">
                <span class="enc-comp-axis__field-label">最小步长</span>
                <input
                  type="number"
                  :value="getAxisConfig(axis.name).customParams.minStep"
                  @input="onCustomParamChange(axis.name, 'minStep', Number(($event.target as HTMLInputElement).value))"
                  step="0.0001"
                  min="0"
                  class="enc-comp-axis__input config-input"
                />
              </label>
            </div>
            <div class="enc-comp-axis__custom-row">
              <label class="enc-comp-axis__field">
                <span class="enc-comp-axis__field-label">超时(ms)</span>
                <input
                  type="number"
                  :value="getAxisConfig(axis.name).customParams.timeoutMs"
                  @input="onCustomParamChange(axis.name, 'timeoutMs', Number(($event.target as HTMLInputElement).value))"
                  step="100"
                  min="100"
                  class="enc-comp-axis__input config-input"
                />
              </label>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="enabledAxes.length === 0" class="enc-comp-editor__empty">
      <p>未启用任何轴的编码器补偿</p>
    </div>
  </div>
</template>

<style scoped>
/* ============================================================
   编码器补偿编辑器
   ============================================================ */
.enc-comp-editor {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

/* ============================================================
   编辑器头部
   ============================================================ */
.enc-comp-editor__header {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
}

.enc-comp-editor__title {
  font-size: 0.6875rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  margin: 0;
}

.enc-comp-editor__subtitle {
  font-size: 0.75rem;
  color: var(--text-muted);
}

/* ============================================================
   轴补偿列表
   ============================================================ */
.enc-comp-editor__axes {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: var(--space-3);
}

@media (min-width: 640px) {
  .enc-comp-editor__axes {
    grid-template-columns: repeat(2, 1fr);
  }
}

/* ============================================================
   单轴补偿卡片
   ============================================================ */
.enc-comp-axis {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
}

.enc-comp-axis__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.enc-comp-axis__badge {
  width: 1.5rem;
  height: 1.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: var(--axis-hue, var(--accent-primary));
  color: white;
  font-size: 0.75rem;
  font-weight: 800;
  flex-shrink: 0;
}

.enc-comp-axis__toggle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
}

.enc-comp-axis__toggle-label {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
}

.enc-comp-axis__toggle-input {
  width: 1rem;
  height: 1rem;
  accent-color: var(--accent-primary);
  cursor: pointer;
}

/* ============================================================
   轴补偿主体
   ============================================================ */
.enc-comp-axis__body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.enc-comp-axis__field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.enc-comp-axis__field-label {
  font-size: 0.6875rem;
  font-weight: 600;
  color: var(--text-muted);
}

/* 输入框/选择框 — 基础样式继承全局 .config-input / .config-select */
.enc-comp-axis__select,
.enc-comp-axis__input {
  /* 仅补充组件特有样式 */
}

.enc-comp-axis__select:focus,
.enc-comp-axis__input:focus {
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 2px var(--accent-primary-muted);
}

/* ============================================================
   自定义参数行
   ============================================================ */
.enc-comp-axis__custom {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.enc-comp-axis__custom-row {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--space-2) var(--space-3);
}

/* ============================================================
   空状态
   ============================================================ */
.enc-comp-editor__empty {
  padding: var(--space-6);
  text-align: center;
  color: var(--text-muted);
  font-size: 0.75rem;
  background: var(--bg-panel-strong);
  border: 1px dashed var(--border-default);
  border-radius: var(--radius-md);
}
</style>
