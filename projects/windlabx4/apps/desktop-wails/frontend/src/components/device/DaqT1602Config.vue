<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18nStore } from '@stores/i18nStore'
import UiSelect from '@components/ui/UiSelect.vue'
import UiPanel from '@components/ui/UiPanel.vue'
import UiFormField from '@components/ui/UiFormField.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import { Cpu } from '@lucide/vue'
import { T1602_TC_TYPES, T1602_DEFAULT_TYPE_CODE, type T1602TcType } from '@utils/t1602Range'

// ============================================================
// DAQ-T-1602 专属配置面板（Modbus TCP 温度扫描阀）
// ------------------------------------------------------------
// 固件采集周期固定 ~100ms，无采样率/通道掩码/触发概念；但"读取/保存"
// 频率由软件轮询间隔控制，故提供采样频率设置（1~5Hz，独立于全局刷新频率）。
// 面板包含：
//   1. 采样频率：采集/保存频率（1~5Hz）
//   2. 全通道类型：一次选择应用到全部 16 通道（一次性操作，应用后复位）
//   3. 16 通道独立下拉（hover 提示该类型量程）
//   4. 热电偶量程图例：来自设备固件量程表（spec-daq-t1602 §Type Code 枚举）
// ============================================================

// T1602 采样频率范围（Hz）：上限 5 贴设备单帧 ~4.9Hz 上限，独立于全局刷新频率
const SAMPLE_RATE_MIN = 1
const SAMPLE_RATE_MAX = 5

const props = withDefaults(
  defineProps<{
    typeCodes?: number[]
    sampleRate?: number
    disabled?: boolean
  }>(),
  {
    typeCodes: () => Array(16).fill(T1602_DEFAULT_TYPE_CODE),
    sampleRate: 5,
    disabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:typeCodes', v: number[]): void
  (e: 'update:sampleRate', v: number): void
}>()

const i18n = useI18nStore()

// 通道数固定 16（卡1 CH0~7 + 卡2 CH0~7）
const CHANNEL_COUNT = 16

// 热电偶类型：code 与设备 Type Code 寄存器值一致（0=J 1=K 2=T 3=E 4=R 5=S 6=B 7=N），
// 量程来自共享表 @utils/t1602Range（spec-daq-t1602 §Type Code 枚举，真机交叉验证）。
type TcType = T1602TcType

/** 完整选项文案：如 "K（0~1200 ℃）"，用于全通道下拉与通道 hover 提示 */
function rangeLabel(t: TcType): string {
  return `${t.label}（${t.min}~${t.max} ℃）`
}

// 16 通道下拉：短标签（J/K/...），title 带量程提示（hover 可见）
const channelOptions = T1602_TC_TYPES.map(t => ({
  value: String(t.code),
  label: t.label,
  title: rangeLabel(t),
}))

// 全通道下拉：完整标签（含量程），选择后立即应用到全部通道
const applyAllOptions = T1602_TC_TYPES.map(t => ({
  value: String(t.code),
  label: rangeLabel(t),
}))

// 全通道下拉当前值：应用后立即复位，避免与后续单通道修改产生陈旧显示
const applyAllValue = ref('')

/** 量程图例文本：J: -50~50 ℃ · K: 0~1200 ℃ · ... */
const rangeLegend = computed(() =>
  T1602_TC_TYPES.map(t => `${t.label}: ${t.min}~${t.max} ℃`).join('  ·  '),
)

const channels = computed(() => Array.from({ length: CHANNEL_COUNT }, (_, i) => i))

function codeAt(index: number): string {
  return String(props.typeCodes[index] ?? T1602_DEFAULT_TYPE_CODE)
}

function updateCode(index: number, value: string) {
  const next = props.typeCodes.slice(0, CHANNEL_COUNT)
  while (next.length < CHANNEL_COUNT) next.push(T1602_DEFAULT_TYPE_CODE)
  next[index] = Number(value)
  emit('update:typeCodes', next)
}

/** 全通道应用：把选中的类型写入全部 16 通道，并复位下拉（一次性操作） */
function applyAll(value: string) {
  if (!value) return
  emit('update:typeCodes', Array(CHANNEL_COUNT).fill(Number(value)))
  applyAllValue.value = ''
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

    <div class="form-fields">
      <!-- 采样频率：采集/保存频率（独立于全局刷新频率） -->
      <UiFormField
        :label="i18n.t.dev_t1602_sampleRate"
        :hint="i18n.t.dev_t1602_sampleRateHint"
      >
        <div class="t1602-sample-rate">
          <UiInputNumber
            :model-value="props.sampleRate"
            :min="SAMPLE_RATE_MIN"
            :max="SAMPLE_RATE_MAX"
            :step="1"
            size="small"
            :disabled="props.disabled"
            @update:model-value="emit('update:sampleRate', Number($event))"
          />
          <span class="input-unit">Hz</span>
        </div>
      </UiFormField>

      <!-- 全通道类型：一次应用到全部 16 通道 -->
      <UiFormField
        :label="i18n.t.dev_t1602_applyAllLabel"
        :hint="i18n.t.dev_t1602_applyAllHint"
      >
        <UiSelect
          :model-value="applyAllValue"
          :options="applyAllOptions"
          :placeholder="i18n.t.dev_t1602_applyAllPlaceholder"
          :disabled="props.disabled"
          @update:model-value="applyAll($event)"
        />
      </UiFormField>

      <div class="form-fields t1602-grid">
        <UiFormField v-for="i in channels" :key="i" :label="`CH${String(i + 1).padStart(2, '0')}`">
          <UiSelect
            :model-value="codeAt(i)"
            :options="channelOptions"
            :disabled="props.disabled"
            @update:model-value="updateCode(i, $event as string)"
          />
        </UiFormField>
      </div>

      <!-- 热电偶量程图例：设备固件量程表（℃） -->
      <p class="t1602-range-legend">
        <span class="t1602-range-legend__title">{{ i18n.t.dev_t1602_rangeTitle }}</span>
        <span class="t1602-range-legend__body">{{ rangeLegend }}</span>
      </p>
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

/* 采样频率行：数字输入 + 单位，靠左紧凑排列 */
.t1602-sample-rate {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

/* 全通道类型下拉：限制最大宽度，避免单控件撑满整行显得突兀 */
.t1602-card :deep(.ui-form-field__control) {
  max-width: 320px;
}

/* 量程图例：小号弱化文字，可换行，与上方网格用虚线分隔 */
.t1602-range-legend {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 10px;
  margin: 0;
  padding-top: var(--space-1-5);
  border-top: 1px dashed var(--border-default);
  font-size: var(--font-size-micro);
  line-height: var(--line-height-base);
  color: var(--text-tertiary);
}

.t1602-range-legend__title {
  font-weight: var(--font-weight-semibold);
  color: var(--text-muted);
  white-space: nowrap;
}

/* 响应式：窄屏退化为 2 列，避免控件被压缩 */
@media (max-width: 640px) {
  .t1602-card :deep(.form-fields.t1602-grid) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
