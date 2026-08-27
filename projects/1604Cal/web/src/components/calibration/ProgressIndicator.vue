<template>
  <div class="progress-indicator">
    <h4 class="title">
      标定流程
    </h4>
    <!-- currentStep === 0 时进入 idle 态：仅渲染第 1 步 active + "等待开始"提示，
         避免用户误以为流程已完成。该 idle 逻辑下沉到 StepIndicator 的 showIdleHint prop，
         此处仅负责把数字步骤映射为 StepIndicator 所需的 key/doneKeys。 -->
    <StepIndicator
      :steps="steps"
      :current-key="currentKey"
      :done-keys="doneKeys"
      :show-idle-hint="isIdle"
      idle-hint-text="等待开始"
      aria-label="标定流程步骤"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import StepIndicator from '@/components/common/StepIndicator.vue'

/** 单个步骤定义：与 StepIndicator 的 Step 接口保持一致 */
interface Step {
  key: string
  label: string
  index?: number
}

const props = defineProps<{
  /** 当前步骤序号（与 CalibrationStep 枚举一一对应，0 表示 idle） */
  currentStep: number
}>()

// 标定流程的 6 个固定步骤：key 用作 StepIndicator 的唯一标识，
// 顺序与 CalibrationStep 枚举值（0-5）一一对应，保证 currentStep 可直接作为数组索引。
const steps: Step[] = [
  { key: 'device-connect', label: '设备连接', index: 1 },
  { key: 'channel-select', label: '通道选择', index: 2 },
  { key: 'start-calibration', label: '开始标定', index: 3 },
  { key: 'data-collection', label: '数据采集', index: 4 },
  { key: 'data-fitting', label: '数据拟合', index: 5 },
  { key: 'completed', label: '完成', index: 6 }
]

// idle 态判定：currentStep <= 0 视为流程尚未开始，
// 与原实现 v-if="currentStep === 0" 行为一致。
const isIdle = computed(() => props.currentStep <= 0)

// 把数字步骤映射为 StepIndicator 的 currentKey：
// idle 时返回第 1 步的 key（StepIndicator 走 idle 分支不会用到，但保持非空避免边界问题）；
// 越界时夹到最后一项，避免 steps[currentStep] 取到 undefined。
const currentKey = computed(() => {
  if (isIdle.value) return steps[0].key
  const idx = Math.min(props.currentStep, steps.length - 1)
  return steps[idx].key
})

// 已完成步骤：currentStep 之前的所有步骤都标记为 done。
// 与原实现 `currentStep > index` 等价（index 是 0-based）。
// idle 态下返回空数组。
const doneKeys = computed(() => {
  if (isIdle.value) return []
  return steps.slice(0, props.currentStep).map(s => s.key)
})
</script>

<style scoped lang="scss">
.progress-indicator {
  padding: 0;
  font-family: $font-sans;

  .title {
    color: $slate-500;
    margin: 0 0 6px 0;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
  }
}
</style>
