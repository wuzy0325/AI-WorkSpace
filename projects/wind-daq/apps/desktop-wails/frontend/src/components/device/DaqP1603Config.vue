<script setup lang="ts">
import { computed } from 'vue'
import type { ChannelConfig, ChannelSensorType } from '@api/types'
import UiButton from '@components/ui/UiButton.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import { useI18nStore } from '@stores/i18nStore'

// ============================================================
// DAQ-P-1603 专属配置面板
// ------------------------------------------------------------
// 与 DAQ-T-1603 不同：DAQ-P-1603 每通道可独立配置为压力或温度传感器，
// 单位下拉选项随传感器类型切换（压力→Pa/kPa/MPa/kgf/cm2/psi，温度→℃/℉）。
// 压力单位切换时按换算系数同步转换该通道工程量程（rangeMin/rangeMax），
// 保证物理量一致（例：Pa 下 -5000~5000 切到 kPa → -5~5）。
// 采样率范围 1~500Hz（用户采样率=每秒数据条目数，底层硬件固定 1000Hz 通过多点平均实现），
// 越界时红色提示并禁用提交（由父组件通过 samplingRateInvalid slot prop 或本组件的 invalid 状态判断）。
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

const i18n = useI18nStore()

// 采样率有效范围常量（与后端 hardware.DAQP1603MinSampleRate/DAQP1603MaxSampleRate 保持同步）
// 用户采样率 = 每秒输出数据条目数，底层硬件采样率固定 1000Hz，
// 低频时通过多点平均实现（如 20Hz → 每 50 个原始点取平均输出 1 条）。
const MIN_SAMPLE_RATE = 1
const MAX_SAMPLE_RATE = 500

// 压力单位选项（与 DeviceManagementDrawer 全局压力单位保持一致）
const PRESSURE_UNIT_OPTIONS = [
  { value: 'Pa', label: 'Pa' },
  { value: 'kPa', label: 'kPa' },
  { value: 'MPa', label: 'MPa' },
  { value: 'kgf/cm2', label: 'kgf/cm2' },
  { value: 'psi', label: 'psi' },
]

// 压力单位到 Pa 的换算系数（1 单位 = factor Pa）
// 用于单位切换时按比例换算工程量程，保证物理量一致
const PRESSURE_UNIT_TO_PA_FACTOR: Record<string, number> = {
  Pa: 1,
  kPa: 1000,
  MPa: 1_000_000,
  'kgf/cm2': 98066.5,
  psi: 6894.757293168,
}

// 温度单位选项
const TEMPERATURE_UNIT_OPTIONS = [
  { value: '℃', label: '℃' },
  { value: '℉', label: '℉' },
]

// 传感器类型选项（响应式，随全局语言切换）
const SENSOR_TYPE_OPTIONS = computed(() => [
  { value: 'pressure', label: i18n.t.dev_p1603_sensorTypePressure },
  { value: 'temperature', label: i18n.t.dev_p1603_sensorTypeTemperature },
])

// 各传感器类型的默认单位与量程（类型切换时重置）
const DEFAULTS_BY_SENSOR_TYPE: Record<
  ChannelSensorType,
  { unit: string; rangeMin: number; rangeMax: number }
> = {
  pressure: { unit: 'Pa', rangeMin: -5000, rangeMax: 5000 },
  temperature: { unit: '℃', rangeMin: -50, rangeMax: 150 },
}

// 采样率是否越界（用于红色提示与禁用提交）
const samplingRateInvalid = computed(
  () => props.samplingRate < MIN_SAMPLE_RATE || props.samplingRate > MAX_SAMPLE_RATE,
)

// 采样率范围提示文案（响应式，注入 {min}/{max} 占位符）
const samplingRateRangeHint = computed(() =>
  i18n.t.dev_p1603_samplingRateRangeHint
    .replace('{min}', String(MIN_SAMPLE_RATE))
    .replace('{max}', String(MAX_SAMPLE_RATE)),
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
  const channel = props.channels[index]
  patchChannel(index, {
    sensorType: typed,
    unit: defaults.unit,
    rangeMin: defaults.rangeMin,
    rangeMax: defaults.rangeMax,
    calibrationEnabled: typed === 'pressure',
    calibrationOffset: typed === 'pressure' ? channel?.calibrationOffset : 0,
    calibrationUnit: typed === 'pressure' ? channel?.calibrationUnit : '',
    calibrationAt: typed === 'pressure' ? channel?.calibrationAt : 0,
  })
}

// 单位切换：按换算系数同步转换该通道的工程量程上下限
// 例：Pa 下 -5000~5000 切到 kPa → -5~5；切到 MPa → -0.005~0.005
// 仅在旧/新单位均在系数表中时换算；否则只更新单位，保留原量程数值
function onUnitChange(index: number, unit: string): void {
  const channel = props.channels[index]
  if (!channel) {
    patchChannel(index, { unit })
    return
  }
  const oldUnit = channel.unit
  const factorOld = oldUnit ? PRESSURE_UNIT_TO_PA_FACTOR[oldUnit] : undefined
  const factorNew = PRESSURE_UNIT_TO_PA_FACTOR[unit]
  // 单位不在换算表（如温度单位误入）或系数为 0 时，只更新单位字段
  if (factorOld == null || factorNew == null || factorNew === 0) {
    patchChannel(index, { unit })
    return
  }
  const ratio = factorOld / factorNew
  const convert = (v: number | undefined): number | undefined => {
    if (typeof v !== 'number' || !Number.isFinite(v)) return v
    return v * ratio
  }
  patchChannel(index, {
    unit,
    rangeMin: convert(channel.rangeMin),
    rangeMax: convert(channel.rangeMax),
  })
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
    calibrationEnabled: true,
  }))
  emit('update:channels', next)
}
</script>

<template>
  <div class="p1603-config">
    <!-- 采样率输入区 -->
    <div class="p1603-config__sampling-rate">
      <label class="p1603-config__label">{{ i18n.t.dev_p1603_samplingRate }}</label>
      <UiInputNumber
        :model-value="samplingRate"
        :min="MIN_SAMPLE_RATE"
        :max="MAX_SAMPLE_RATE"
        :disabled="disabled"
        class="p1603-config__sampling-rate-input"
        @update:model-value="onSamplingRateChange"
      />
      <span
        class="p1603-config__hint"
        :class="{ 'p1603-config__hint--error': samplingRateInvalid }"
      >
        {{ samplingRateRangeHint }}
      </span>
    </div>

    <!-- 工具栏 -->
    <div class="p1603-config__toolbar">
      <UiButton secondary size="sm" :disabled="disabled" @click="setAllChannelsEnabled(true)">
        {{ i18n.t.dev_enableAll }}
      </UiButton>
      <UiButton secondary size="sm" :disabled="disabled" @click="setAllChannelsEnabled(false)">
        {{ i18n.t.dev_disableAll }}
      </UiButton>
      <UiButton secondary size="sm" :disabled="disabled" @click="resetChannelsToDefault">
        {{ i18n.t.dev_reset }}
      </UiButton>
    </div>

    <!-- 批量同步 -->
    <div class="p1603-config__batch">
      <span class="p1603-config__batch-label">{{ i18n.t.dev_batchApplyTo }}</span>
      <div class="p1603-config__batch-field">
        <span class="p1603-config__batch-field-label">{{ i18n.t.dev_range }}</span>
        <UiInputNumber
          :model-value="rangeMin ?? undefined"
          class="p1603-config__batch-num"
          :disabled="disabled"
          :placeholder="i18n.t.dev_minPlaceholder"
          @update:model-value="onBatchRangeMinChange"
        />
        <span class="p1603-config__batch-sep">~</span>
        <UiInputNumber
          :model-value="rangeMax ?? undefined"
          class="p1603-config__batch-num"
          :disabled="disabled"
          :placeholder="i18n.t.dev_maxPlaceholder"
          @update:model-value="onBatchRangeMaxChange"
        />
      </div>
      <div class="p1603-config__batch-field">
        <span class="p1603-config__batch-field-label">{{ i18n.t.channelPrecision }}</span>
        <UiInputNumber
          :model-value="precision ?? undefined"
          class="p1603-config__batch-num p1603-config__batch-num--narrow"
          :min="0"
          :disabled="disabled"
          placeholder="0"
          @update:model-value="onBatchPrecisionChange"
        />
        <span class="p1603-config__batch-field-suffix">{{ i18n.t.dev_decimalPlaces }}</span>
      </div>
    </div>

    <!-- 16 通道表 -->
    <div class="p1603-config__table-wrap">
      <table class="p1603-config__table">
        <thead>
          <tr>
            <th class="w-12">{{ i18n.t.channelEnabled }}</th>
            <th class="w-16">{{ i18n.t.tareApplyColumn }}</th>
            <th class="w-12">#</th>
            <th>{{ i18n.t.dev_channelName }}</th>
            <th class="w-28">{{ i18n.t.dev_p1603_sensorType }}</th>
            <th class="w-24">{{ i18n.t.unit }}</th>
            <th class="w-56 text-center">{{ i18n.t.dev_engineeringRange }}</th>
            <th class="w-18 text-right">{{ i18n.t.channelPrecision }}</th>
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
            <td class="text-center">
              <UiCheckbox
                :checked="c.calibrationEnabled ?? true"
                :disabled="disabled || c.sensorType === 'temperature'"
                :title="c.sensorType === 'temperature' ? i18n.t.temperatureChannelNotSupported : undefined"
                @update:checked="(v) => patchChannel(i, { calibrationEnabled: v })"
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
/* 卡片容器：与全局 form-card 风格一致，但保留 p1603 自有的纵向堆叠布局 */
.p1603-config {
  display: flex;
  flex-direction: column;
  gap: var(--density-field-gap);
  padding: var(--density-group-padding);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--bg-panel-strong) 40%, transparent);
  border: 1px solid var(--border-default);
}

/* 采样率行：标签 + 输入 + 范围提示水平排列 */
.p1603-config__sampling-rate {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.p1603-config__sampling-rate-input {
  width: 7.5rem;
}

.p1603-config__label {
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-muted);
  letter-spacing: 0.02em;
}

.p1603-config__hint {
  font-size: var(--font-size-micro);
  font-weight: var(--font-weight-bold);
  color: var(--text-muted);
}

.p1603-config__hint--error {
  color: var(--accent-danger);
}

.p1603-config__toolbar {
  display: flex;
  gap: var(--space-2);
}

.p1603-config__batch {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.p1603-config__batch-label {
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-bold);
  color: var(--text-muted);
}

.p1603-config__batch-field {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.p1603-config__batch-field-label {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
}

/* 批量输入框尺寸：§29 规范——量程 96px、精度 64px */
.p1603-config__batch-num {
  width: 6rem;
}

.p1603-config__batch-num--narrow {
  width: 4rem;
}

.p1603-config__batch-sep {
  color: var(--text-muted);
}

.p1603-config__batch-field-suffix {
  font-size: var(--font-size-2xs);
  color: var(--text-muted);
}

.p1603-config__table-wrap {
  overflow-x: auto;
}

.p1603-config__table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-sm);
}

.p1603-config__table th,
.p1603-config__table td {
  padding: var(--space-1-5) var(--space-2);
  border-bottom: 1px solid var(--border-default);
  text-align: left;
  vertical-align: middle;
}

.p1603-config__table th {
  font-size: var(--font-size-2xs);
  font-weight: var(--font-weight-semibold);
  color: var(--text-muted);
}

.p1603-config__range {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

/* ===== §29 通道表格列宽规范 =====
 *   - w-12/w-16/w-24/w-28 是 Tailwind 标准类，JIT 会自动生成，显式定义仅为确定性
 *   - w-18/w-56 不是 Tailwind 标准刻度，必须显式定义
 *   - w-28 用于 DAQ-P-1603 传感器类型列（含中文"温度/压力"）
 *   - w-16 用于校零应用列（与 DAQ-T-1603/WTN_PXI 单位列保持一致） */
.w-12 { width: 48px; }   /* 启用复选框、# 序号列 */
.w-16 { width: 64px; }   /* 校零应用列、单位列 */
.w-18 { width: 72px; }   /* 精度列 */
.w-24 { width: 96px; }   /* 单位列 */
.w-28 { width: 112px; }  /* 传感器类型列 */
.w-56 { width: 224px; }  /* 工程量程列（两个 w-full 输入框 + "~" 分隔符 + 单元格 padding） */
.w-full { width: 100%; }
.text-center { text-align: center; }
.text-right { text-align: right; }
.font-mono { font-family: var(--font-family-mono); }
</style>
