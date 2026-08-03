<script setup lang="ts">
import { reactive, watch, computed, inject } from 'vue'
import type { AxisConfig, AxisEncoderCompensationConfig } from '@shared/types/motion'
import { defaultEncComp, validateEncoderCompensation, normalizePositive, DEFAULT_ENCODER_SCALE, type CompensationWarning } from './motionConfigEditor'
import UiInput from '@components/ui/UiInput.vue'

interface AxisCompState {
  enabled: boolean
  preset: string
  customParams: AxisEncoderCompensationConfig
  warnings: CompensationWarning[]
}

// 从父级（MotionControllerConfig）注入 tooltip 函数
const showTooltip = inject<(text: string, event: MouseEvent) => void>('showTooltip')!
const hideTooltip = inject<() => void>('hideTooltip')!

const props = defineProps<{
  axes: AxisConfig[]
  controllerType: string
}>()

const emit = defineEmits<{
  'update-enc-comp': [index: number, value: AxisEncoderCompensationConfig]
  'update-encoder-scale': [index: number, value: number]
}>()

// 光栅尺分辨率（encoderScale）输入处理。零/负值非法，回退默认。
function onEncoderScaleInput(axisName: string, raw: string | number): void {
  const value = typeof raw === 'number' ? raw : Number(raw)
  const normalized = normalizePositive(value, DEFAULT_ENCODER_SCALE)
  const index = getAxisIndex(axisName)
  emit('update-encoder-scale', index, normalized)
}

// 使用 reactive Record 替代 ref(Map)，确保属性赋值触发 Vue 响应式更新
const axisConfigs: Record<string, AxisCompState> = reactive({})

function computeWarnings(axis: AxisConfig, cfg: AxisEncoderCompensationConfig): CompensationWarning[] {
  const merged: AxisEncoderCompensationConfig = { ...cfg, enabled: true }
  return validateEncoderCompensation(merged, axis)
}

// 初始化轴配置
watch(() => props.axes, (axes) => {
  for (const axis of axes) {
    if (!axisConfigs[axis.name]) {
      const defaultConfig = axis.encoderCompensation || defaultEncComp()
      const merged = { ...defaultConfig, enabled: defaultConfig.enabled ?? false }
      axisConfigs[axis.name] = {
        enabled: defaultConfig.enabled ?? false,
        preset: 'default',
        customParams: { ...defaultConfig },
        warnings: computeWarnings(axis, merged),
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

// 预设配置。默认值与 shared/device-sdk/go/motion/core DefaultEncoderCompensation* 对齐。
const COMPENSATION_PRESETS: Record<string, AxisEncoderCompensationConfig> = {
  default: { enabled: true, tolerance: 0.01, maxCycles: 3, settleMs: 100, minStep: 0.001, timeoutMs: 5000 },
  high_precision: { enabled: true, tolerance: 0.001, maxCycles: 20, settleMs: 200, minStep: 0.0001, timeoutMs: 10000 },
  high_speed: { enabled: true, tolerance: 0.05, maxCycles: 5, settleMs: 50, minStep: 0.01, timeoutMs: 3000 },
}

const DEFAULT_STATE: AxisCompState = {
  enabled: false,
  preset: 'default',
  customParams: { enabled: false, tolerance: 0.01, maxCycles: 3, settleMs: 100, minStep: 0.001, timeoutMs: 5000 },
  warnings: [],
}

function getAxisConfig(axisName: string): AxisCompState {
  return axisConfigs[axisName] || DEFAULT_STATE
}

function getAxisIndex(axisName: string): number {
  return props.axes.findIndex(a => a.name === axisName)
}

function recomputeWarnings(axisName: string) {
  const axis = props.axes.find(a => a.name === axisName)
  if (!axis) return
  const config = getAxisConfig(axisName)
  const activeCfg = config.preset === 'custom'
    ? { ...config.customParams, enabled: true }
    : { ...COMPENSATION_PRESETS[config.preset], enabled: true }
  axisConfigs[axisName] = {
    ...config,
    warnings: computeWarnings(axis, activeCfg),
  }
}

function onToggle(axisName: string, enabled: boolean) {
  const config = getAxisConfig(axisName)
  // 直接赋值触发 reactive 响应式更新
  axisConfigs[axisName] = { ...config, enabled }

  const preset = config.preset === 'custom'
    ? config.customParams
    : COMPENSATION_PRESETS[config.preset] || COMPENSATION_PRESETS.default

  emit('update-enc-comp', getAxisIndex(axisName), { ...preset, enabled })
  recomputeWarnings(axisName)
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
  recomputeWarnings(axisName)
}

function onCustomParamChange(axisName: string, key: keyof AxisEncoderCompensationConfig, value: number | boolean) {
  if (typeof value === 'number' && !Number.isFinite(value)) return
  if (typeof value === 'number') {
    if (key === 'tolerance' && value <= 0) return
    if (key === 'minStep' && value <= 0) return
    if (key === 'maxCycles' && (!Number.isInteger(value) || value < 1)) return
    if (key === 'settleMs' && value < 0) return
    if (key === 'timeoutMs' && value < 100) return
  }
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
  recomputeWarnings(axisName)
}

// 参数帮助文本
const FIELD_HELP: Record<string, string> = {
  encoderScale: '每个编码器计数对应的工程单位（mm 或 °）。如光栅尺分辨率 0.005mm/计数。设得越小精度越高，但 tolerance 必须 ≥ 此值。',
  tolerance: '补偿到位判定阈值。当前后两次编码器读数差值 ≤ 此值时，认为补偿收敛。不可小于编码器分辨率。',
  maxCycles: '单次 MoveTo/MoveBy 后补偿循环的最大次数。达到上限后仍未收敛则标记失败。',
  settleMs: '机械停止后等待震荡衰减的时间（毫秒）。设太短会在轴未稳时读数，导致误判。',
  minStep: '单次修正的最小工程步长。误差小于此值时不再修正，避免无穷小步进振荡。设太大会修正过头。',
  timeoutMs: '单次补偿任务的总超时（毫秒）。超时后标记失败。',
}

const enabledAxes = computed(() => {
  return props.axes.filter(axis => axisConfigs[axis.name]?.enabled)
})

// 编码器补偿仅 B140 控制器支持：WTNMC4A / 模拟控制器不显示配置项，避免误配置
const isB140Controller = computed(() => props.controllerType === 'B140-MC')

// 仅展示「B140 控制器 + 该轴位置来源选了编码器」的轴。
// positionSource 默认 'register'，所以用户未主动切到 encoder 时配置项不出现，
// 与"只有 B140 在选择编码器时才让配置"的需求一致。
const visibleAxes = computed(() =>
  isB140Controller.value
    ? props.axes.filter(axis => axis.positionSource === 'encoder')
    : []
)
</script>

<template>
  <div class="enc-comp-editor">
    <div class="enc-comp-editor__header">
      <h4 class="enc-comp-editor__title">编码器补偿</h4>
      <span class="enc-comp-editor__subtitle">为各轴配置编码器补偿参数</span>
    </div>

    <div v-if="visibleAxes.length > 0" class="enc-comp-editor__axes">
      <div
        v-for="axis in visibleAxes"
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
          <!-- 编码器分辨率（光栅尺分辨率） -->
          <div class="enc-comp-axis__scale-info">
            <span class="enc-comp-axis__field-label">
              编码器分辨率
              <span
                class="enc-comp-help"
                @mouseenter="showTooltip(FIELD_HELP['encoderScale'], $event)"
                @mousemove="showTooltip(FIELD_HELP['encoderScale'], $event)"
                @mouseleave="hideTooltip()"
              >?</span>
            </span>
            <div class="enc-comp-axis__scale-row">
              <UiInput
                :model-value="axis.encoderScale ?? DEFAULT_ENCODER_SCALE"
                type="number"
                :min="0.0001"
                :step="0.0001"
                compact
                class="enc-comp-axis__scale-input"
                @update:model-value="onEncoderScaleInput(axis.name, $event)"
              />
              <span class="enc-comp-axis__scale-unit">{{ axis.kind === 'ROTARY' ? '°/计数' : 'mm/计数' }}</span>
            </div>
          </div>

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

          <!-- 当前预设值的简表（非 custom 时显示当前值） -->
          <div v-if="getAxisConfig(axis.name).preset !== 'custom'" class="enc-comp-axis__preset-info">
            <span class="enc-comp-axis__preset-item">容差: {{ (COMPENSATION_PRESETS[getAxisConfig(axis.name).preset] || COMPENSATION_PRESETS.default).tolerance }}</span>
            <span class="enc-comp-axis__preset-item">步长: {{ (COMPENSATION_PRESETS[getAxisConfig(axis.name).preset] || COMPENSATION_PRESETS.default).minStep }}</span>
            <span class="enc-comp-axis__preset-item">循环: {{ (COMPENSATION_PRESETS[getAxisConfig(axis.name).preset] || COMPENSATION_PRESETS.default).maxCycles }}</span>
            <span class="enc-comp-axis__preset-item">超时: {{ (COMPENSATION_PRESETS[getAxisConfig(axis.name).preset] || COMPENSATION_PRESETS.default).timeoutMs }}ms</span>
          </div>

          <!-- 自定义参数 -->
          <div v-if="getAxisConfig(axis.name).preset === 'custom'" class="enc-comp-axis__custom">
            <div class="enc-comp-axis__custom-row">
              <label class="enc-comp-axis__field">
                <span class="enc-comp-axis__field-label">
                  容差
                  <span
                    class="enc-comp-help"
                    @mouseenter="showTooltip(FIELD_HELP['tolerance'], $event)"
                    @mousemove="showTooltip(FIELD_HELP['tolerance'], $event)"
                    @mouseleave="hideTooltip()"
                  >?</span>
                </span>
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
                <span class="enc-comp-axis__field-label">
                  最大循环
                  <span
                    class="enc-comp-help"
                    @mouseenter="showTooltip(FIELD_HELP['maxCycles'], $event)"
                    @mousemove="showTooltip(FIELD_HELP['maxCycles'], $event)"
                    @mouseleave="hideTooltip()"
                  >?</span>
                </span>
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
                <span class="enc-comp-axis__field-label">
                  稳定时间(ms)
                  <span
                    class="enc-comp-help"
                    @mouseenter="showTooltip(FIELD_HELP['settleMs'], $event)"
                    @mousemove="showTooltip(FIELD_HELP['settleMs'], $event)"
                    @mouseleave="hideTooltip()"
                  >?</span>
                </span>
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
                <span class="enc-comp-axis__field-label">
                  最小步长
                  <span
                    class="enc-comp-help"
                    @mouseenter="showTooltip(FIELD_HELP['minStep'], $event)"
                    @mousemove="showTooltip(FIELD_HELP['minStep'], $event)"
                    @mouseleave="hideTooltip()"
                  >?</span>
                </span>
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
                <span class="enc-comp-axis__field-label">
                  超时(ms)
                  <span
                    class="enc-comp-help"
                    @mouseenter="showTooltip(FIELD_HELP['timeoutMs'], $event)"
                    @mousemove="showTooltip(FIELD_HELP['timeoutMs'], $event)"
                    @mouseleave="hideTooltip()"
                  >?</span>
                </span>
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

          <!-- 校验警告 -->
          <div v-if="getAxisConfig(axis.name).warnings.length > 0" class="enc-comp-axis__warnings">
            <div
              v-for="(w, wi) in getAxisConfig(axis.name).warnings"
              :key="wi"
              class="enc-comp-axis__warning"
              :class="'enc-comp-axis__warning--' + w.severity"
            >
              <span class="enc-comp-axis__warning-icon">{{ w.severity === 'error' ? '!' : '△' }}</span>
              <span class="enc-comp-axis__warning-text">{{ w.message }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 三类空态：非 B140 / B140 但无编码器轴 / B140 且未启用任何补偿 -->
    <div v-else-if="!isB140Controller" class="enc-comp-editor__empty">
      <p>仅 B140 控制器支持编码器补偿配置</p>
    </div>
    <div v-else-if="visibleAxes.length === 0" class="enc-comp-editor__empty">
      <p>暂无可配置的轴，请先将轴的「位置来源」设为编码器</p>
    </div>
    <div v-else-if="enabledAxes.length === 0" class="enc-comp-editor__empty">
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
  padding: 11px 12px;
  border: 1px solid var(--border-default);
  /* 仪器质感：紧凑圆角 */
  border-radius: 4px;
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
  /* 仪器质感：紧凑圆角 */
  border-radius: 4px;
  background: var(--axis-hue, var(--accent-primary));
  color: #0b0e13;
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
   编码器分辨率信息行
   ============================================================ */
.enc-comp-axis__scale-info {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.enc-comp-axis__scale-row {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.enc-comp-axis__scale-value {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--accent-info);
  font-variant-numeric: tabular-nums;
}

.enc-comp-axis__scale-input {
  flex: 1;
  min-width: 0;
}

.enc-comp-axis__scale-unit {
  font-size: 0.6875rem;
  color: var(--text-muted);
}

/* ============================================================
   预设值摘要
   ============================================================ */
.enc-comp-axis__preset-info {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  padding: var(--space-1-5) var(--space-2);
  border-radius: var(--radius-sm);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
}

.enc-comp-axis__preset-item {
  font-size: 0.6875rem;
  color: var(--text-muted);
  white-space: nowrap;
}

/* ============================================================
   帮助提示图标 ?
   ============================================================ */
.enc-comp-help {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  font-size: 0.625rem;
  font-weight: 700;
  color: var(--text-muted);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  cursor: help;
  transition: all var(--motion-fast) var(--easing-standard);
  vertical-align: middle;
  margin-left: var(--space-1);
}

.enc-comp-help:hover {
  color: var(--accent-primary);
  border-color: var(--accent-primary);
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
}

/* ============================================================
   校验警告
   ============================================================ */
.enc-comp-axis__warnings {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin-top: var(--space-1);
}

.enc-comp-axis__warning {
  display: flex;
  align-items: flex-start;
  gap: var(--space-1-5);
  padding: 7px 9px;
  /* 仪器质感：紧凑圆角 */
  border-radius: 3px;
  font-size: 0.6875rem;
  line-height: 1.4;
}

.enc-comp-axis__warning--error {
  color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 8%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-danger) 20%, transparent);
}

.enc-comp-axis__warning--warning {
  color: var(--text-muted);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
}

.enc-comp-axis__warning-icon {
  flex-shrink: 0;
  margin-top: 1px;
  font-weight: 700;
  font-size: 0.75rem;
}

.enc-comp-axis__warning-text {
  flex: 1;
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
  /* 仪器质感：紧凑圆角 */
  border-radius: 4px;
}
</style>
