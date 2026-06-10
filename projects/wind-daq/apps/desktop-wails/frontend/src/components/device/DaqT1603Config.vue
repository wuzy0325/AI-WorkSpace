<script setup lang="ts">
import UiToggle from '@components/ui/UiToggle.vue'
import UiInput from '@components/ui/UiInput.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiSelect from '@components/ui/UiSelect.vue'

const props = withDefaults(
  defineProps<{
    channelMask?: string
    samplingRate?: number
    binaryFormat?: boolean
    triggerMode?: number
    triggerEdge?: number
    triggerCount?: number
    showTimestamp?: boolean
    openCircuitCheck?: string
  }>(),
  {
    channelMask: 'FFFF',
    samplingRate: 10,
    binaryFormat: false,
    triggerMode: 0,
    triggerEdge: 0,
    triggerCount: 0,
    showTimestamp: false,
    openCircuitCheck: '0000',
  },
)

const emit = defineEmits<{
  (e: 'update:channelMask', v: string): void
  (e: 'update:samplingRate', v: number): void
  (e: 'update:binaryFormat', v: boolean): void
  (e: 'update:triggerMode', v: number): void
  (e: 'update:triggerEdge', v: number): void
  (e: 'update:triggerCount', v: number): void
  (e: 'update:showTimestamp', v: boolean): void
  (e: 'update:openCircuitCheck', v: string): void
}>()

const samplingRateOptions = [
  { value: '1', label: '1 Hz' },
  { value: '2', label: '2 Hz' },
  { value: '5', label: '5 Hz' },
  { value: '10', label: '10 Hz' },
  { value: '20', label: '20 Hz' },
  { value: '50', label: '50 Hz' },
  { value: '100', label: '100 Hz' },
]

const triggerModeOptions = [
  { value: '0', label: '软件触发' },
  { value: '2', label: '硬件触发' },
]

const triggerEdgeOptions = [
  { value: '0', label: '上升沿' },
  { value: '1', label: '下降沿' },
  { value: '2', label: '跳变' },
]

function onSamplingRateChange(e: Event): void {
  emit('update:samplingRate', Number((e.target as HTMLSelectElement).value))
}

function onTriggerModeChange(e: Event): void {
  emit('update:triggerMode', Number((e.target as HTMLSelectElement).value))
}

function onTriggerEdgeChange(e: Event): void {
  emit('update:triggerEdge', Number((e.target as HTMLSelectElement).value))
}

function onNumberEmit(fn: (v: number) => void, e: Event): void {
  const v = Number((e.target as HTMLInputElement).value)
  if (Number.isFinite(v)) fn(v)
}
</script>

<template>
  <div class="t1603-config">
    <div class="t1603-config__field">
      <label class="t1603-config__label">通道掩码</label>
      <UiInput :model-value="channelMask" @update:model-value="emit('update:channelMask', $event as string)" placeholder="0000-FFFF" />
    </div>

    <div class="t1603-config__field">
      <label class="t1603-config__label">采样率</label>
      <UiSelect :model-value="String(samplingRate)" @update:model-value="emit('update:samplingRate', Number($event))" :options="samplingRateOptions" />
    </div>

    <div class="t1603-config__field">
      <label class="t1603-config__label">二进制格式</label>
      <UiToggle :model-value="binaryFormat" @update:model-value="emit('update:binaryFormat', $event)" />
    </div>

    <div class="t1603-config__field">
      <label class="t1603-config__label">触发模式</label>
      <UiSelect :model-value="String(triggerMode)" @update:model-value="emit('update:triggerMode', Number($event))" :options="triggerModeOptions" />
    </div>

    <div class="t1603-config__field">
      <label class="t1603-config__label">触发边沿</label>
      <UiSelect :model-value="String(triggerEdge)" @update:model-value="emit('update:triggerEdge', Number($event))" :options="triggerEdgeOptions" />
    </div>

    <div class="t1603-config__field">
      <label class="t1603-config__label">触发计数</label>
      <UiInputNumber :model-value="triggerCount" @update:model-value="(v) => v !== null && emit('update:triggerCount', v)" class="t1603-config__input--trigger-count" :min="0" />
    </div>

    <div class="t1603-config__field">
      <label class="t1603-config__label">显示时间戳</label>
      <UiToggle :model-value="showTimestamp" @update:model-value="emit('update:showTimestamp', $event)" />
    </div>

    <div class="t1603-config__field">
      <label class="t1603-config__label">开路检测</label>
      <UiInput :model-value="openCircuitCheck" @update:model-value="emit('update:openCircuitCheck', $event as string)" placeholder="hex mask" />
    </div>
  </div>
</template>

<style scoped>
.t1603-config {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
  padding: 1rem;
  border-radius: 0.75rem;
  background: color-mix(in srgb, var(--bg-panel-strong) 40%, transparent);
  border: 1px solid var(--border-default);
}

.t1603-config__field {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.t1603-config__label {
  display: block;
  font-size: 0.65rem;
  font-weight: 800;
  color: var(--text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.t1603-config__input {
  width: 100%;
  padding: 0.625rem 0.75rem;
  border-radius: 0.5rem;
  border: 1px solid var(--border-default);
  background: rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
  font: inherit;
  font-size: 0.85rem;
  font-weight: 700;
  outline: none;
  transition: all 0.2s ease;
}

.t1603-config__input:focus {
  border-color: var(--color-accent);
  background: var(--bg-panel-strong);
}

:root[data-theme='light'] .t1603-config__input {
  background: rgba(255, 255, 255, 0.8);
}

.t1603-config__input--trigger-count {
  width: 80px;
}

.t1603-config__hint {
  font-size: 0.6rem;
  font-weight: 700;
  color: var(--text-muted);
  margin-top: 0.125rem;
}
</style>
