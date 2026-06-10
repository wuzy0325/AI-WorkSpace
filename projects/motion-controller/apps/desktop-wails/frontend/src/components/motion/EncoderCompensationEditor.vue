<script setup lang="ts">
import { inject, reactive, watch, computed } from 'vue'
import type { AxisConfig, AxisEncoderCompensationConfig, MotionControllerType } from '@shared/types/motion'
import { getAxisThemeClass, defaultEncComp } from './motionConfigEditor'
import UiToggle from '@components/ui/UiToggle.vue'

const props = defineProps<{
  axes: AxisConfig[]
  controllerType: MotionControllerType
}>()

const emit = defineEmits<{
  updateEncComp: [index: number, value: AxisEncoderCompensationConfig]
}>()

const tooltip = inject<(text: string, event: MouseEvent) => void>('showTooltip', () => {})
const hideTooltip = inject<() => void>('hideTooltip', () => {})

// 是否为 B140 控制器（只有 B140 才支持编码器补偿）
const isB140 = computed(() => props.controllerType === 'B140-MC')

// 筛选出使用编码器的轴（只有编码器轴才支持补偿）
const encoderAxes = computed(() =>
  props.axes.filter((axis) => axis.positionSource === 'encoder')
)

const rawValues = reactive<Record<string, string>>({})

// 根据轴名查找在 props.axes 中的索引
function findAxisIndex(axisName: string): number {
  return props.axes.findIndex((a) => a.name === axisName)
}

function getEncComp(axisName: string): AxisEncoderCompensationConfig {
  const axis = props.axes.find((a) => a.name === axisName)
  return axis?.encoderCompensation ?? defaultEncComp()
}

function setEncComp(axisName: string, v: AxisEncoderCompensationConfig): void {
  const index = findAxisIndex(axisName)
  if (index >= 0) emit('updateEncComp', index, v)
}

function getRawValue(axisName: string, field: keyof AxisEncoderCompensationConfig): string {
  const key = `${axisName}-${field}`
  return rawValues[key] ?? String(getEncComp(axisName)[field] ?? '')
}

function setRawValue(axisName: string, field: keyof AxisEncoderCompensationConfig, raw: string): void {
  const key = `${axisName}-${field}`
  rawValues[key] = raw
  const num = raw === '' ? undefined : Number(raw)
  if (num !== undefined && !isNaN(num)) {
    setEncComp(axisName, { ...getEncComp(axisName), [field]: num })
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

    <!-- 非 B140 控制器：显示不支持提示 -->
    <div v-if="!isB140" class="enc-unavailable-hint">
      <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12.01" y2="16"/><path d="M12 12v-4"/></svg>
      <span>编码器补偿仅支持 B140 控制器</span>
    </div>

    <!-- B140 控制器但无编码器轴：显示配置提示 -->
    <div v-else-if="encoderAxes.length === 0" class="enc-unavailable-hint">
      <svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12.01" y2="16"/><path d="M12 12v-4"/></svg>
      <span>请将轴的位置源设置为「编码器」后启用补偿功能</span>
    </div>

    <!-- B140 + 编码器轴：显示补偿配置 -->
    <template v-else>
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

      <!-- 数据行：仅显示编码器轴 -->
      <div
        v-for="axis in encoderAxes"
        :key="'enc-' + axis.name"
        class="enc-table-row"
        :class="[getAxisThemeClass(axis.name)]"
      >
        <div class="enc-col-axis">
          <span class="enc-axis-badge">{{ axis.name }}</span>
        </div>
        <div class="enc-col-toggle">
          <UiToggle
            size="sm"
            :model-value="getEncComp(axis.name).enabled"
            style="--toggle-color: var(--axis-hue, var(--accent-success))"
            @update:model-value="setEncComp(axis.name, { ...getEncComp(axis.name), enabled: $event })"
          />
        </div>
        <div class="enc-col-field">
          <input
            :value="getRawValue(axis.name, 'tolerance')"
            @input="setRawValue(axis.name, 'tolerance', ($event.target as HTMLInputElement).value)"
            type="text"
            inputmode="decimal"
            class="enc-input"
            :disabled="!getEncComp(axis.name).enabled"
          />
        </div>
        <div class="enc-col-field">
          <input
            :value="getEncComp(axis.name).maxCycles"
            @input="setEncComp(axis.name, { ...getEncComp(axis.name), maxCycles: Number(($event.target as HTMLInputElement).value) })"
            type="number"
            class="enc-input"
            min="1"
            :disabled="!getEncComp(axis.name).enabled"
          />
        </div>
        <div class="enc-col-field">
          <input
            :value="getEncComp(axis.name).settleMs"
            @input="setEncComp(axis.name, { ...getEncComp(axis.name), settleMs: Number(($event.target as HTMLInputElement).value) })"
            type="number"
            class="enc-input"
            min="10"
            :disabled="!getEncComp(axis.name).enabled"
          />
        </div>
        <div class="enc-col-field">
          <input
            :value="getRawValue(axis.name, 'minStep')"
            @input="setRawValue(axis.name, 'minStep', ($event.target as HTMLInputElement).value)"
            type="text"
            inputmode="decimal"
            class="enc-input"
            :disabled="!getEncComp(axis.name).enabled"
          />
        </div>
        <div class="enc-col-field">
          <input
            :value="getEncComp(axis.name).timeoutMs"
            @input="setEncComp(axis.name, { ...getEncComp(axis.name), timeoutMs: Number(($event.target as HTMLInputElement).value) })"
            type="number"
            class="enc-input"
            min="100"
            :disabled="!getEncComp(axis.name).enabled"
          />
        </div>
      </div>
    </template>
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

/* 不可用提示 */
.enc-unavailable-hint {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.625rem 0.75rem;
  border-radius: 0.375rem;
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  font-size: 0.7rem;
  color: var(--text-muted);
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


</style>
