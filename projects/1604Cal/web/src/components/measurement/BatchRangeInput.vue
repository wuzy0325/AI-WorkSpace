<template>
  <section class="batch-range-input">
    <header class="panel-header">
      <h3>16 通道量程录入</h3>
      <div class="header-actions">
        <button
          class="action-btn"
          @click="handleFillAll"
        >
          全部填充
        </button>
        <button
          class="action-btn"
          @click="handleClearAll"
        >
          清空
        </button>
      </div>
    </header>

    <!--
      量程录入网格：2 列布局
      每行结构：CH 标签 | 最小值 | ~ | 最大值 | 单位 | 复制数量 | 复制按钮
      「复制 N 个↓」：将当前行的 (min, max, unit) 复制到后续 N 个通道
    -->
    <div class="range-grid">
      <div
        v-for="(item, index) in channelRanges"
        :key="item.channelId"
        class="range-row"
      >
        <span class="channel-label">CH{{ item.channelId }}</span>

        <input
          v-model.number="item.rangeMin"
          type="number"
          step="0.1"
          class="range-value-input"
          placeholder="最小"
          :class="{ invalid: !isValid(item) }"
          @blur="emitChange"
        >

        <span class="range-separator">~</span>

        <input
          v-model.number="item.rangeMax"
          type="number"
          step="0.1"
          class="range-value-input"
          placeholder="最大"
          :class="{ invalid: !isValid(item) }"
          @blur="emitChange"
        >

        <select
          v-model="item.rangeUnit"
          class="range-unit-select"
          @change="emitChange"
        >
          <option
            v-for="opt in RANGE_UNIT_OPTIONS"
            :key="opt.value"
            :value="opt.value"
          >
            {{ opt.label }}
          </option>
        </select>

        <!-- 行内复制区：数量输入 + 复制按钮 -->
        <div class="copy-area">
          <input
            v-model.number="copyCounts[index]"
            type="number"
            min="1"
            max="15"
            class="copy-count-input"
            title="向后复制的通道数"
          >
          <button
            class="copy-btn"
            :disabled="!isValid(item) || index >= channelRanges.length - 1"
            title="将当前量程复制到后续 N 个通道"
            @click="handleCopyDown(index)"
          >
            复制↓
          </button>
        </div>
      </div>
    </div>

    <footer class="panel-footer">
      <button
        class="confirm-btn"
        :disabled="!allValid"
        @click="handleConfirm"
      >
        确认并自动分组
      </button>
      <span
        v-if="!allValid"
        class="hint"
      >请填写所有通道的有效量程（最大值必须严格大于最小值）</span>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import {
  type ChannelRange,
  type RangeUnit,
  RANGE_UNIT_OPTIONS,
  DEFAULT_RANGE_UNIT,
  TOTAL_CHANNELS
} from '@/types/batch'

const emit = defineEmits<{
  /** 量程变更时触发（实时） */
  change: [channelRanges: ChannelRange[]]
  /** 确认提交时触发 */
  confirm: [channelRanges: ChannelRange[]]
}>()

// 初始化 16 通道量程配置，默认 min=0 / max=0 / MPa
const channelRanges = reactive<ChannelRange[]>(
  Array.from({ length: TOTAL_CHANNELS }, (_, i) => ({
    channelId: i + 1,
    rangeMin: 0,
    rangeMax: 0,
    rangeUnit: DEFAULT_RANGE_UNIT as RangeUnit,
    skipped: false
  }))
)

// 每行的「复制数量」输入值，默认 3（用户典型场景：输入一个后向后复制 3 个）
// 独立 ref 数组而非塞进 ChannelRange，避免污染数据模型
const copyCounts = ref<number[]>(
  Array.from({ length: TOTAL_CHANNELS }, () => 3)
)

// 单通道量程校验：max 必须严格大于 min（支持负压量程，min 可为负）
const isValid = (item: ChannelRange): boolean => {
  return (
    typeof item.rangeMin === 'number' &&
    typeof item.rangeMax === 'number' &&
    !isNaN(item.rangeMin) &&
    !isNaN(item.rangeMax) &&
    item.rangeMax > item.rangeMin
  )
}

// 所有通道量程均有效
const allValid = computed<boolean>(() => {
  return channelRanges.every(isValid)
})

// 量程变更时实时通知父组件
const emitChange = (): void => {
  emit('change', [...channelRanges])
}

// 全部填充：用第一个有效通道的量程填充所有空通道
const handleFillAll = (): void => {
  const firstValid = channelRanges.find(isValid)
  const fillMin = firstValid?.rangeMin ?? 0
  const fillMax = firstValid?.rangeMax ?? 1
  const fillUnit = firstValid?.rangeUnit ?? DEFAULT_RANGE_UNIT

  channelRanges.forEach((item) => {
    if (!isValid(item)) {
      item.rangeMin = fillMin
      item.rangeMax = fillMax
      item.rangeUnit = fillUnit
    }
  })
  emitChange()
}

// 清空所有量程
const handleClearAll = (): void => {
  channelRanges.forEach((item) => {
    item.rangeMin = 0
    item.rangeMax = 0
    item.rangeUnit = DEFAULT_RANGE_UNIT
  })
  emitChange()
}

// 行内「复制 N 个↓」：将当前行 (min, max, unit) 复制到后续 N 个通道
//
// 设计意图：操作员录入一个通道后，若后续若干通道量程相同，
// 可直接在当前行点击「复制↓」将量程批量应用，避免逐个输入。
//
// 边界处理：
//  - 数量上限不超过剩余通道数（防止越界）
//  - 源通道未通过校验时按钮禁用，避免复制无效值
//  - 最后一行禁用复制按钮（无后续通道可复制）
const handleCopyDown = (sourceIndex: number): void => {
  const source = channelRanges[sourceIndex]
  if (!isValid(source)) return

  const requested = copyCounts.value[sourceIndex] ?? 3
  // 实际可复制数量：不超过剩余通道数，至少 1
  const remaining = channelRanges.length - sourceIndex - 1
  const actualCount = Math.max(1, Math.min(requested, remaining))

  for (let i = 1; i <= actualCount; i++) {
    const target = channelRanges[sourceIndex + i]
    if (!target) break
    target.rangeMin = source.rangeMin
    target.rangeMax = source.rangeMax
    target.rangeUnit = source.rangeUnit
  }
  emitChange()
}

// 确认提交
const handleConfirm = (): void => {
  if (!allValid.value) return
  emit('confirm', [...channelRanges])
}

// 暴露给父组件的方法：获取当前量程配置
defineExpose({
  getChannelRanges: (): ChannelRange[] => [...channelRanges],
  setChannelRanges: (ranges: ChannelRange[]): void => {
    ranges.forEach((r, i) => {
      if (i < channelRanges.length) {
        channelRanges[i] = { ...r }
      }
    })
  }
})
</script>

<style scoped lang="scss">
.batch-range-input {
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.panel-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: $slate-800;
}

.header-actions {
  display: flex;
  gap: 8px;
}

/* 次按钮：slate 半透明，与项目 ctrl-btn 风格一致 */
.action-btn {
  padding: 6px 12px;
  border: 1px solid $slate-200;
  background: rgba(55, 65, 81, 0.08);
  color: $slate-700;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  transition: all 0.15s ease;

  &:hover {
    background: rgba(55, 65, 81, 0.14);
    border-color: $slate-300;
  }

  &:active {
    transform: translateY(1px);
  }
}

/* 2 列网格：每行内容增多（min/max/unit/复制），需要更宽的行 */
.range-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
}

.range-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border: 1px solid $slate-200;
  border-radius: 6px;
  background: #fff;
}

.channel-label {
  font-weight: 600;
  font-size: 13px;
  min-width: 36px;
  color: $slate-700;
}

.range-value-input {
  width: 64px;
  padding: 4px 6px;
  border: 1px solid $slate-200;
  border-radius: 4px;
  font-size: 13px;
  text-align: center;

  &:focus {
    outline: none;
    border-color: $mint;
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
  }
}

.range-value-input.invalid {
  border-color: $danger-500;
  background: $danger-50;
}

.range-separator {
  color: $slate-400;
  font-size: 13px;
  font-weight: 600;
}

.range-unit-select {
  padding: 4px;
  border: 1px solid $slate-200;
  border-radius: 4px;
  font-size: 12px;
  min-width: 56px;
}

/* 行内复制区：数量输入 + 复制按钮 */
.copy-area {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
}

.copy-count-input {
  width: 40px;
  padding: 4px 4px;
  border: 1px solid $slate-200;
  border-radius: 4px;
  font-size: 12px;
  text-align: center;

  &:focus {
    outline: none;
    border-color: $mint;
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
  }
}

/* 复制按钮：slate 次按钮，hover 变 mint，保持与主操作区分 */
.copy-btn {
  padding: 4px 8px;
  border: 1px solid $slate-200;
  background: rgba(55, 65, 81, 0.08);
  color: $slate-700;
  border-radius: 4px;
  cursor: pointer;
  font-size: 11px;
  font-weight: 600;
  font-family: $font-sans;
  white-space: nowrap;
  transition: all 0.15s ease;

  &:hover:not(:disabled) {
    background: rgba(16, 185, 129, 0.15);
    border-color: $mint;
    color: $mint-dark;
  }

  &:active:not(:disabled) {
    transform: translateY(1px);
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

.panel-footer {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* 主按钮：mint 渐变，与项目 btn-mint/btn-start 一致 */
.confirm-btn {
  padding: 8px 20px;
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #fff;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  font-family: $font-sans;
  min-width: 120px;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);
  transition: all 0.15s ease;

  &:disabled {
    background: $slate-300;
    cursor: not-allowed;
    box-shadow: none;
  }

  &:not(:disabled):hover {
    background: linear-gradient(135deg, $mint-light, $mint);
    box-shadow: 0 4px 12px rgba(16, 185, 129, 0.4);
    transform: translateY(-1px);
  }
}

.hint {
  color: $danger-500;
  font-size: 13px;
}
</style>
