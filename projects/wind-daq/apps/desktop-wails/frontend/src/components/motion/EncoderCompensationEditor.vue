<script setup lang="ts">
import { inject } from 'vue'
import type { AxisConfig, AxisEncoderCompensationConfig } from '@shared/types/motion'
import { getAxisThemeClass, defaultEncComp } from './motionConfigEditor'
import { NCheckbox, NInputNumber } from 'naive-ui'

const props = defineProps<{
  axes: AxisConfig[]
}>()

const emit = defineEmits<{
  updateEncComp: [index: number, value: AxisEncoderCompensationConfig]
}>()

const tooltip = inject<(text: string, event: MouseEvent) => void>('showTooltip', () => {})
const hideTooltip = inject<() => void>('hideTooltip', () => {})

function getEncComp(index: number): AxisEncoderCompensationConfig {
  return props.axes[index]?.encoderCompensation ?? defaultEncComp()
}

function setEncComp(index: number, v: AxisEncoderCompensationConfig): void {
  emit('updateEncComp', index, v)
}
</script>

<template>
  <div class="config-section">
    <h3 class="config-section__title">
      <svg class="w-4 h-4 inline-block mr-1.5 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20v-6M6 20V10M18 20V4"/></svg>
      编码器补偿
      <span class="section-subtitle">补偿机械间隙和反向间隙</span>
    </h3>
    <div v-for="(axis, index) in axes" :key="'enc-' + axis.name" class="encoder-compensation" :class="[getAxisThemeClass(axis.name)]" style="margin-bottom: 0.5rem;">
      <div class="encoder-compensation__header">
        <label class="encoder-compensation__toggle">
          <NCheckbox :checked="getEncComp(index).enabled" @update:checked="setEncComp(index, { ...getEncComp(index), enabled: $event })" size="small" />
          <span>{{ axis.name }} 轴编码器补偿</span>
        </label>
      </div>
      <div class="encoder-compensation__fields" v-if="getEncComp(index).enabled">
        <div class="encoder-compensation__row">
          <div class="encoder-compensation__field">
            <label>
              容差
              <span class="field-hint" @mouseenter="tooltip('补偿停止的允许位置误差', $event)" @mouseleave="hideTooltip">?</span>
            </label>
            <NInputNumber :value="getEncComp(index).tolerance" @update:value="setEncComp(index, { ...getEncComp(index), tolerance: $event ?? 0 })" size="tiny" style="width:80px" :step="0.001" :min="0" />
          </div>
          <div class="encoder-compensation__field">
            <label>
              最大周期
              <span class="field-hint" @mouseenter="tooltip('补偿尝试的最大循环次数', $event)" @mouseleave="hideTooltip">?</span>
            </label>
            <NInputNumber :value="getEncComp(index).maxCycles" @update:value="setEncComp(index, { ...getEncComp(index), maxCycles: $event ?? 0 })" size="tiny" style="width:80px" :min="1" />
          </div>
          <div class="encoder-compensation__field">
            <label>
              稳定时间 (ms)
              <span class="field-hint" @mouseenter="tooltip('每次补偿移动后的等待稳定时间', $event)" @mouseleave="hideTooltip">?</span>
            </label>
            <NInputNumber :value="getEncComp(index).settleMs" @update:value="setEncComp(index, { ...getEncComp(index), settleMs: $event ?? 0 })" size="tiny" style="width:80px" :min="10" />
          </div>
          <div class="encoder-compensation__field">
            <label>
              最小步长
              <span class="field-hint" @mouseenter="tooltip('补偿移动的最小步进值', $event)" @mouseleave="hideTooltip">?</span>
            </label>
            <NInputNumber :value="getEncComp(index).minStep" @update:value="setEncComp(index, { ...getEncComp(index), minStep: $event ?? 0 })" size="tiny" style="width:80px" :step="0.0001" :min="0" />
          </div>
          <div class="encoder-compensation__field">
            <label>
              超时时间 (ms)
              <span class="field-hint" @mouseenter="tooltip('补偿过程的最大允许时间', $event)" @mouseleave="hideTooltip">?</span>
            </label>
            <NInputNumber :value="getEncComp(index).timeoutMs" @update:value="setEncComp(index, { ...getEncComp(index), timeoutMs: $event ?? 0 })" size="tiny" style="width:80px" :min="100" />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.config-section {
  margin-bottom: 1.5rem;
}
.config-section__title {
  font-size: 0.8rem;
  font-weight: 700;
  color: #94a3b8;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  margin-bottom: 0.875rem;
}
.section-subtitle {
  display: inline;
  font-size: 0.65rem;
  font-weight: 400;
  color: #64748b;
  text-transform: none;
  letter-spacing: normal;
  margin-left: 0.5rem;
}
.encoder-compensation {
  border-radius: 0.5rem;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.1);
  padding: 0.875rem;
}
:root[data-theme='light'] .encoder-compensation {
  border: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(0, 0, 0, 0.02);
}
.encoder-compensation__header {
  margin-bottom: 0.875rem;
}
.encoder-compensation__toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: #94a3b8;
  cursor: pointer;
}
.encoder-compensation__fields {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.encoder-compensation__row {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 0.75rem;
}
.encoder-compensation__field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.encoder-compensation__field label {
  font-size: 0.675rem;
  font-weight: 600;
  color: #64748b;
}
.encoder-compensation__input {
  height: 1.875rem;
  padding: 0 0.5rem;
  border-radius: 0.25rem;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.15);
  color: #e2e8f0;
  font-size: 0.75rem;
  outline: none;
  text-align: right;
  transition: border-color 0.2s ease;
}
:root[data-theme='light'] .encoder-compensation__input {
  border: 1px solid rgba(0, 0, 0, 0.1);
  background: rgba(0, 0, 0, 0.04);
  color: #0f172a;
}
.encoder-compensation__input:focus {
  border-color: #8b5cf6;
}
</style>