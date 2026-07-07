<script setup lang="ts">
import { computed } from 'vue'
import type { ChannelConfig, ChannelSensorType } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'

// ============================================================
// DAQ-P-1603 专属配置面板
// ------------------------------------------------------------
// 与 DAQ-T-1603 不同：DAQ-P-1603 每通道可独立配置为压力或温度传感器，
// 单位下拉选项随传感器类型切换（压力→Pa/kPa/MPa/mmH2O，温度→℃/℉）。
// 采样率上限 500Hz（spec D-2），越界时红色提示并禁用提交（由父组件
// 通过 samplingRateExceedsMax slot prop 或本组件的 invalid 状态判断）。
//
// 组件为 controlled 模式：所有状态由父组件通过 v-model 传入，
// 组件内部不持有任何状态，保证父组件 draft 的唯一真相源。
// ============================================================

const props = withDefaults(
  defineProps<{
    /** 16 通道配置（v-model:channels） */
    channels: ChannelConfig[]
    /** 采样率 Hz（v-model:samplingRate） */
    samplingRate: number
    /** 量程下限（v-model:rangeMin），用于批量同步 */
    rangeMin?: number | null
    /** 量程上限（v-model:rangeMax），用于批量同步 */
    rangeMax?: number | null
    /** 精度（v-model:precision），用于批量同步 */
    precision?: number | null
    /** 只读模式（采集进行中） */
    disabled?: boolean
  }>(),
  {
    rangeMin: null,
    rangeMax: null,
    precision: null,
    disabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:channels', v: ChannelConfig[]): void
  (e: 'update:samplingRate', v: number): void
  (e: 'update:rangeMin', v: number | null): void
  (e: 'update:rangeMax', v: number | null): void
  (e: 'update:precision', v: number | null): void
}>()

// 采样率上限常量（与后端 sharedhw.DAQP1603MaxSampleRate 对齐）
const MAX_SAMPLE_RATE = 500

// 压力单位选项
const PRESSURE_UNIT_OPTIONS = [
  { value: 'Pa', label: 'Pa' },
  { value: 'kPa', label: 'kPa' },
  { value: 'MPa', label: 'MPa' },
  { value: 'mmH2O', label: 'mmH2O' },
]

// 温度单位选项
const TEMPERATURE_UNIT_OPTIONS = [
  { value: '℃', label: '℃' },
  { value: '℉', label: '℉' },
]

// 传感器类型选项
const SENSOR_TYPE_OPTIONS = [
  { value: 'pressure', label: '压力' },
  { value: 'temperature', label: '温度' },
]

// 各传感器类型的默认单位与量程（类型切换时重置）
const DEFAULTS_BY_SENSOR_TYPE: Record<
  ChannelSensorType,
  { unit: string; rangeMin: number; rangeMax: number }
> = {
  pressure: { unit: 'Pa', rangeMin: -5000, rangeMax: 5000 },
  temperature: { unit: '℃', rangeMin: -50, rangeMax: 150 },
}

// 采样率是否超过上限（用于红色提示与禁用提交）
const samplingRateExceedsMax = computed(
  () => props.samplingRate > MAX_SAMPLE_RATE || props.samplingRate <= 0,
)

// 根据通道传感器类型返回对应的单位选项
function unitOptionsFor(sensorType: ChannelSensorType | undefined) {
  return sensorType === 'temperature' ? TEMPERATURE_UNIT_OPTIONS : PRESSURE_UNIT_OPTIONS
}

// 通道号格式化：1 → "01"
function channelLabel(index: number): string {
  return String(index + 1).padStart(2, '0')
}

// ---- 通道字段变更处理 ----

// 修改某个通道的字段并 emit 完整 channels 数组
function patchChannel(index: number, patch: Partial<ChannelConfig>): void {
  const next: ChannelConfig[] = props.channels.map((c, i) =>
    i === index ? { ...c, ...patch } : c,
  )
  emit('update:channels', next)
}

// 传感器类型切换：重置单位、量程为该类型默认值
function onSensorTypeChange(index: number, nextType: string): void {
  const typed = nextType as ChannelSensorType
  const defaults = DEFAULTS_BY_SENSOR_TYPE[typed]
  patchChannel(index, {
    sensorType: typed,
    unit: defaults.unit,
    rangeMin: defaults.rangeMin,
    rangeMax: defaults.rangeMax,
  })
}

// 单位切换：仅更新单位字段
function onUnitChange(index: number, unit: string): void {
  patchChannel(index, { unit })
}

// 启用切换
function onEnabledChange(index: number, enabled: boolean): void {
  patchChannel(index, { enabled })
}

// 通道名变更
function onNameChange(index: number, name: string): void {
  patchChannel(index, { name })
}

// 量程下限变更
function onRangeMinChange(index: number, v: number | null): void {
  patchChannel(index, { rangeMin: v ?? 0 })
}

// 量程上限变更
function onRangeMaxChange(index: number, v: number | null): void {
  patchChannel(index, { rangeMax: v ?? 0 })
}

// 精度变更
function onPrecisionChange(index: number, v: number | null): void {
  patchChannel(index, { precision: v ?? 0 })
}

// ---- 批量同步（应用到全部 16 通道）----
// 设计：输入新值时立即 emit update:rangeMin 与 update:channels，
// 用新值作为参数直接更新所有通道，避免依赖 props 异步更新时机。

function applyRangeToAll(min: number | null, max: number | null): void {
  if (min == null || max == null) return
  const next: ChannelConfig[] = props.channels.map((c) => ({
    ...c,
    rangeMin: min,
    rangeMax: max,
  }))
  emit('update:channels', next)
}

function applyPrecisionToAll(precision: number | null): void {
  if (precision == null) return
  const next: ChannelConfig[] = props.channels.map((c) => ({
    ...c,
    precision,
  }))
  emit('update:channels', next)
}

// 批量量程输入变更处理：同时 emit update:rangeMin/rangeMax 与 update:channels
function onBatchRangeMinChange(v: number | null): void {
  emit('update:rangeMin', v)
  applyRangeToAll(v, props.rangeMax)
}

function onBatchRangeMaxChange(v: number | null): void {
  emit('update:rangeMax', v)
  applyRangeToAll(props.rangeMin, v)
}

function onBatchPrecisionChange(v: number | null): void {
  emit('update:precision', v)
  applyPrecisionToAll(v)
}

// ---- 采样率变更 ----

function onSamplingRateChange(v: number | null): void {
  emit('update:samplingRate', v ?? 0)
}

// ---- 工具栏操作 ----

function setAllChannelsEnabled(enabled: boolean): void {
  const next: ChannelConfig[] = props.channels.map((c) => ({ ...c, enabled }))
  emit('update:channels', next)
}

function resetChannelsToDefault(): void {
  const next: ChannelConfig[] = Array.from({ length: 16 }, (_, i) => ({
    index: i,
    name: `CH${i + 1}`,
    enabled: true,
    unit: 'Pa',
    precision: 3,
    rangeMin: -5000,
    rangeMax: 5000,
    sensorType: 'pressure' as ChannelSensorType,
  }))
  emit('update:channels', next)
}
</script>

<template>
  <div class="p1603-config">
    <!-- 采样率输入区 -->
    <div class="p1603-config__sampling-rate">
      <label class="p1603-config__label">采样率 (Hz)</label>
      <UiInputNumber
        :model-value="samplingRate"
        :min="1"
        :max="MAX_SAMPLE_RATE"
        :disabled="disabled"
        class="p1603-config__sampling-rate-input"
        @update:model-value="onSamplingRateChange"
      />
      <span
        class="p1603-config__hint"
        :class="{ 'p1603-config__hint--error': samplingRateExceedsMax }"
      >
        上限 {{ MAX_SAMPLE_RATE }} Hz
      </span>
    </div>

    <!-- 工具栏 -->
    <div class="p1603-config__toolbar">
      <UiButton secondary size="sm" :disabled="disabled" @click="setAllChannelsEnabled(true)">
        全部启用
      </UiButton>
      <UiButton secondary size="sm" :disabled="disabled" @click="setAllChannelsEnabled(false)">
        全部禁用
      </UiButton>
      <UiButton secondary size="sm" :disabled="disabled" @click="resetChannelsToDefault">
        重置
      </UiButton>
    </div>

    <!-- 批量同步 -->
    <div class="p1603-config__batch">
      <span class="p1603-config__batch-label">批量应用到 1~16CH:</span>
      <div class="p1603-config__batch-field">
        <span class="p1603-config__batch-field-label">量程</span>
        <UiInputNumber
          :model-value="rangeMin ?? undefined"
          class="p1603-config__batch-num"
          :disabled="disabled"
          placeholder="最小"
          @update:model-value="onBatchRangeMinChange"
        />
        <span class="p1603-config__batch-sep">~</span>
        <UiInputNumber
          :model-value="rangeMax ?? undefined"
          class="p1603-config__batch-num"
          :disabled="disabled"
          placeholder="最大"
          @update:model-value="onBatchRangeMaxChange"
        />
      </div>
      <div class="p1603-config__batch-field">
        <span class="p1603-config__batch-field-label">精度</span>
        <UiInputNumber
          :model-value="precision ?? undefined"
          class="p1603-config__batch-num p1603-config__batch-num--narrow"
          :min="0"
          :disabled="disabled"
          placeholder="0"
          @update:model-value="onBatchPrecisionChange"
        />
        <span class="p1603-config__batch-field-suffix">位小数</span>
      </div>
    </div>

    <!-- 16 通道表 -->
    <div class="p1603-config__table-wrap">
      <table class="p1603-config__table">
        <thead>
          <tr>
            <th class="w-14">启用</th>
            <th class="w-14">#</th>
            <th>通道名称</th>
            <th class="w-28">传感器类型</th>
            <th class="w-24">单位</th>
            <th class="w-36 text-center">工程量程</th>
            <th class="w-20 text-right">精度</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(c, i) in channels" :key="c.index">
            <td class="text-center">
              <UiCheckbox
                :checked="c.enabled"
                :disabled="disabled"
                @update:checked="(v) => onEnabledChange(i, v)"
              />
            </td>
            <td class="font-mono">{{ channelLabel(c.index) }}</td>
            <td>
              <UiInput
                :model-value="c.name"
                :disabled="disabled"
                @update:model-value="(v) => onNameChange(i, v as string)"
              />
            </td>
            <td>
              <UiSelect
                :model-value="c.sensorType ?? 'pressure'"
                :options="SENSOR_TYPE_OPTIONS"
                :disabled="disabled"
                @update:model-value="(v) => onSensorTypeChange(i, v)"
              />
            </td>
            <td>
              <UiSelect
                :model-value="c.unit"
                :options="unitOptionsFor(c.sensorType)"
                :disabled="disabled"
                @update:model-value="(v) => onUnitChange(i, v)"
              />
            </td>
            <td>
              <div class="p1603-config__range">
                <UiInputNumber
                  :model-value="c.rangeMin"
                  class="w-full"
                  :disabled="disabled"
                  @update:model-value="(v) => onRangeMinChange(i, v)"
                />
                <span>~</span>
                <UiInputNumber
                  :model-value="c.rangeMax"
                  class="w-full"
                  :disabled="disabled"
                  @update:model-value="(v) => onRangeMaxChange(i, v)"
                />
              </div>
            </td>
            <td>
              <UiInputNumber
                :model-value="c.precision"
                class="w-full"
                :min="0"
                :disabled="disabled"
                @update:model-value="(v) => onPrecisionChange(i, v)"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.p1603-config {
  display: flex;
  flex-direction: column;
  gap: var(--density-field-gap, 8px);
  padding: var(--density-group-padding, 8px 12px);
  border-radius: var(--radius-md, 6px);
  background: color-mix(in srgb, var(--bg-panel-strong, #1e1e1e) 40%, transparent);
  border: 1px solid var(--border-default, #333);
}

.p1603-config__sampling-rate {
  display: flex;
  align-items: center;
  gap: 8px;
}

.p1603-config__sampling-rate-input {
  width: 120px;
}

.p1603-config__label {
  font-size: var(--font-size-2xs, 12px);
  font-weight: var(--font-weight-semibold, 600);
  color: var(--text-muted, #888);
  letter-spacing: 0.02em;
}

.p1603-config__hint {
  font-size: var(--font-size-micro, 11px);
  font-weight: 700;
  color: var(--text-muted, #888);
}

.p1603-config__hint--error {
  color: var(--color-danger, #e5484d);
}

.p1603-config__toolbar {
  display: flex;
  gap: 8px;
}

.p1603-config__batch {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.p1603-config__batch-label {
  font-size: var(--font-size-2xs, 12px);
  font-weight: 700;
  color: var(--text-muted, #888);
}

.p1603-config__batch-field {
  display: flex;
  align-items: center;
  gap: 4px;
}

.p1603-config__batch-field-label {
  font-size: var(--font-size-2xs, 12px);
  color: var(--text-muted, #888);
}

.p1603-config__batch-num {
  width: 100px;
}

.p1603-config__batch-num--narrow {
  width: 60px;
}

.p1603-config__batch-sep {
  color: var(--text-muted, #888);
}

.p1603-config__batch-field-suffix {
  font-size: var(--font-size-2xs, 12px);
  color: var(--text-muted, #888);
}

.p1603-config__table-wrap {
  overflow-x: auto;
}

.p1603-config__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-sm, 13px);
}

.p1603-config__table th,
.p1603-config__table td {
  padding: 6px 8px;
  border-bottom: 1px solid var(--border-default, #333);
  text-align: left;
  vertical-align: middle;
}

.p1603-config__table th {
  font-size: var(--font-size-2xs, 12px);
  font-weight: var(--font-weight-semibold, 600);
  color: var(--text-muted, #888);
}

.p1603-config__range {
  display: flex;
  align-items: center;
  gap: 4px;
}

.w-14 { width: 56px; }
.w-20 { width: 80px; }
.w-24 { width: 96px; }
.w-28 { width: 112px; }
.w-36 { width: 144px; }
.text-center { text-align: center; }
.text-right { text-align: right; }
.font-mono { font-family: var(--font-mono, monospace); }
</style>
