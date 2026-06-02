<script setup lang="ts">
import { inject, reactive, watch } from 'vue'
import type { AxisConfig, AxisEncoderCompensationConfig } from '@shared/types/motion'
import { getAxisThemeClass, defaultEncComp } from './motionConfigEditor'

const props = defineProps<{
  axes: AxisConfig[]
}>()

const emit = defineEmits<{
  updateEncComp: [index: number, value: AxisEncoderCompensationConfig]
}>()

const tooltip = inject<(text: string, event: MouseEvent) => void>('showTooltip', () => {})
const hideTooltip = inject<() => void>('hideTooltip', () => {})

const rawValues = reactive<Record<string, string>>({})

function getEncComp(index: number): AxisEncoderCompensationConfig {
  return props.axes[index]?.encoderCompensation ?? defaultEncComp()
}

function setEncComp(index: number, v: AxisEncoderCompensationConfig): void {
  emit('updateEncComp', index, v)
}

function getRawValue(index: number, field: keyof AxisEncoderCompensationConfig): string {
  const key = `${index}-${field}`
  return rawValues[key] ?? String(getEncComp(index)[field] ?? '')
}

function setRawValue(index: number, field: keyof AxisEncoderCompensationConfig, raw: string): void {
  const key = `${index}-${field}`
  rawValues[key] = raw
  const num = raw === '' ? undefined : Number(raw)
  if (num !== undefined && !isNaN(num)) {
    setEncComp(index, { ...getEncComp(index), [field]: num })
  }
}

function clearRawValues(): void {
  for (const k in rawValues) delete rawValues[k]
}

watch(() => props.axes, clearRawValues, { deep: true })
</script>

<template>
  <div class="config-section">
    <h3 class="config-section__title">
      <svg class="w-3.5 h-3.5 inline-block mr-1 -mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20v-6M6 20V10M18 20V4"/></svg>
      编码器补偿
      <span class="section-subtitle">补偿机械间隙和反向间隙</span>
    </h3>

    <!-- 表头 -->
    <div class="enc-table-header">
      <div class="enc-col-axis">轴</div>
      <div class="enc-col-toggle">启用</div>
      <div class="enc-col-field">
        容差
        <span class="field-hint" @mouseenter="tooltip('编码器位置与指令位置的允许误差范围（单位与轴单位一致）。当位置误差超过此值时触发补偿动作', $event)" @mouseleave="hideTooltip">?</span>
      </div>
      <div class="enc-col-field">
        最大周期
        <span class="field-hint" @mouseenter="tooltip('补偿执行的最大周期数。超过此周期仍未达到目标位置则终止补偿，防止无限循环', $event)" @mouseleave="hideTooltip">?</span>
      </div>
      <div class="enc-col-field">
        稳定时间(ms)
        <span class="field-hint" @mouseenter="tooltip('补偿到位后的稳定等待时间（毫秒）。等待位置稳定后再进行下一步操作', $event)" @mouseleave="hideTooltip">?</span>
      </div>
      <div class="enc-col-field">
        最小步长
        <span class="field-hint" @mouseenter="tooltip('每次补偿的最小移动步长。防止因微小误差导致的频繁微调', $event)" @mouseleave="hideTooltip">?</span>
      </div>
      <div class="enc-col-field">
        超时(ms)
        <span class="field-hint" @mouseenter="tooltip('单次补偿操作的最长等待时间（毫秒）。超时后强制终止本次补偿', $event)" @mouseleave="hideTooltip">?</span>
      </div>
    </div>

    <!-- 数据行 -->
    <div
      v-for="(axis, index) in axes"
      :key="'enc-' + axis.name"
      class="enc-table-row"
      :class="[getAxisThemeClass(axis.name)]"
    >
      <div class="enc-col-axis">
        <span class="enc-axis-badge">{{ axis.name }}</span>
      </div>
      <div class="enc-col-toggle">
        <label class="mini-toggle">
          <input
            type="checkbox"
            :checked="getEncComp(index).enabled"
            @change="setEncComp(index, { ...getEncComp(index), enabled: !getEncComp(index).enabled })"
          />
          <span class="mini-toggle__track">
            <span class="mini-toggle__thumb"></span>
          </span>
        </label>
      </div>
      <div class="enc-col-field">
        <input
          :value="getRawValue(index, 'tolerance')"
          @input="setRawValue(index, 'tolerance', ($event.target as HTMLInputElement).value)"
          type="text"
          inputmode="decimal"
          class="enc-input"
          :disabled="!getEncComp(index).enabled"
        />
      </div>
      <div class="enc-col-field">
        <input
          :value="getEncComp(index).maxCycles"
          @input="setEncComp(index, { ...getEncComp(index), maxCycles: Number(($event.target as HTMLInputElement).value) })"
          type="number"
          class="enc-input"
          min="1"
          :disabled="!getEncComp(index).enabled"
        />
      </div>
      <div class="enc-col-field">
        <input
          :value="getEncComp(index).settleMs"
          @input="setEncComp(index, { ...getEncComp(index), settleMs: Number(($event.target as HTMLInputElement).value) })"
          type="number"
          class="enc-input"
          min="10"
          :disabled="!getEncComp(index).enabled"
        />
      </div>
      <div class="enc-col-field">
        <input
          :value="getRawValue(index, 'minStep')"
          @input="setRawValue(index, 'minStep', ($event.target as HTMLInputElement).value)"
          type="text"
          inputmode="decimal"
          class="enc-input"
          :disabled="!getEncComp(index).enabled"
        />
      </div>
      <div class="enc-col-field">
        <input
          :value="getEncComp(index).timeoutMs"
          @input="setEncComp(index, { ...getEncComp(index), timeoutMs: Number(($event.target as HTMLInputElement).value) })"
          type="number"
          class="enc-input"
          min="100"
          :disabled="!getEncComp(index).enabled"
        />
      </div>
    </div>
  </div>
</template>

<style scoped>
.config-section {
  margin-bottom: 1rem;
}
.config-section__title {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
  margin-bottom: 0.625rem;
}
.section-subtitle {
  display: inline;
  font-size: 0.6rem;
  font-weight: 400;
  color: var(--text-muted);
  text-transform: none;
  letter-spacing: normal;
  margin-left: 0.5rem;
}

/* 表格布局 */
.enc-table-header,
.enc-table-row {
  display: grid;
  grid-template-columns: 2rem 2.5rem repeat(5, 1fr);
  gap: 0.375rem;
  align-items: center;
}
.enc-table-header {
  padding: 0.25rem 0.5rem;
  font-size: 0.6rem;
  font-weight: 600;
  color: var(--text-muted);
  border-bottom: 1px solid var(--border-default);
  margin-bottom: 0.25rem;
}
.enc-table-row {
  padding: 0.375rem 0.5rem;
  border-radius: 0.25rem;
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  margin-bottom: 0.25rem;
}
.enc-col-axis {
  display: flex;
  align-items: center;
  justify-content: center;
}
.enc-axis-badge {
  width: 1.375rem;
  height: 1.375rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  background: var(--axis-hue, var(--text-muted));
  color: white;
  font-size: 0.75rem;
  font-weight: 700;
}
.enc-col-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
}
.enc-col-field {
  display: flex;
  align-items: center;
}
.enc-input {
  width: 100%;
  height: 1.625rem;
  padding: 0 0.375rem;
  border-radius: 0.25rem;
  border: 1px solid var(--border-default);
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-size: 0.7rem;
  text-align: right;
  outline: none;
  transition: border-color 0.2s ease;
  box-sizing: border-box;
}
.enc-input:focus {
  border-color: var(--accent-primary);
}
.enc-input:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* 轴主题色 */
.axis-x-theme { --axis-hue: var(--axis-x); }
.axis-y-theme { --axis-hue: var(--axis-y); }
.axis-z-theme { --axis-hue: var(--axis-z); }
.axis-u-theme { --axis-hue: var(--axis-u); }

/* 迷你开关 */
.mini-toggle {
  display: inline-flex;
  cursor: pointer;
}
.mini-toggle input {
  display: none;
}
.mini-toggle__track {
  width: 1.625rem;
  height: 0.875rem;
  border-radius: 9999px;
  background: var(--border-strong);
  position: relative;
  transition: background 0.2s ease;
  flex-shrink: 0;
}
.mini-toggle input:checked + .mini-toggle__track {
  background: var(--axis-hue, var(--accent-success));
}
.mini-toggle__thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: calc(0.875rem - 4px);
  height: calc(0.875rem - 4px);
  border-radius: 50%;
  background: white;
  transition: transform 0.2s ease;
}
.mini-toggle input:checked + .mini-toggle__track .mini-toggle__thumb {
  transform: translateX(0.75rem);
}
</style>
