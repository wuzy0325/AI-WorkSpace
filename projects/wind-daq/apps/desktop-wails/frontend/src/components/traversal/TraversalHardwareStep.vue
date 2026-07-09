<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ProbeChannelConfig, TraversalMotionAxisConfig, DataValidationConfig, StabilizationConfig } from '@shared/types/traversal'
import { isTraversalRequiredProbeChannel } from '@shared/types/traversal'
import { useDeviceStore } from '@stores/deviceStore'
import { useMotionStore } from '@stores/motionStore'
import UiPanel from '@components/ui/UiPanel.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import UiButton from '@components/ui/UiButton.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'

const probeChannels = defineModel<ProbeChannelConfig[]>('probeChannels', { required: true })
const motionAxes = defineModel<TraversalMotionAxisConfig[]>('motionAxes', { required: true })
const validationEnabled = defineModel<boolean>('validationEnabled', { required: true })
const validationConfig = defineModel<DataValidationConfig>('validationConfig', { required: true })
const stabilizationMode = defineModel<'fixed' | 'adaptive'>('stabilizationMode', { required: true })
const stabilizationConfig = defineModel<StabilizationConfig>('stabilizationConfig', { required: true })

const props = defineProps<{
  t: Record<string, string>
  isLoading: boolean
}>()

const deviceStore = useDeviceStore()
const motionStore = useMotionStore()

function isRequired(c: ProbeChannelConfig) { return isTraversalRequiredProbeChannel(c.role, c.name) }

// 通道索引枚举选项：UI 显示 CH1~CH18（1-based），内部 value 仍为数组索引 0~17
// 通道序号从 1 开始更符合操作员直觉，对应底层数组的 0-based 索引
const channelIndexOptions = Array.from({ length: 18 }, (_, i) => ({ label: `CH${i + 1}`, value: i }))
// 轴选项标签需随语言切换刷新，故用 computed 派生
const axisOptions = computed(() => ['X', 'Y', 'Z', 'U'].map(a => ({ label: props.t.travAxisSuffix.replace('{axis}', a), value: a })))
const mappingOptions = [
  { label: props.t.mappingAlpha, value: 'alpha' },
  { label: props.t.mappingBeta, value: 'beta' },
]

// 数据验证错误策略选项
const errorStrategyOptions = [
  { label: props.t.travStrategyContinue, value: 'continue' },
  { label: props.t.travStrategyRetry, value: 'retry' },
  { label: props.t.travStrategySkip, value: 'skip' },
]

// 稳定化模式选项
const stabilizationOptions = [
  { label: props.t.travFixedTime, value: 'fixed' },
  { label: props.t.travAdaptive, value: 'adaptive' },
]

// ---- 设备连接状态映射 ----
function getDeviceStatus(deviceId: string): 'idle' | 'connected' | 'acquiring' | 'error' | 'warning' {
  const status = deviceStore.statusFor(deviceId)
  if (status === 'Connected') {
    return deviceStore.acquiringFor(deviceId) ? 'acquiring' : 'connected'
  }
  if (status === 'Connecting') return 'warning'
  return 'idle'
}

// ---- 批量操作：统一选择设备 ----
const deviceOptions = computed(() => deviceStore.profiles.map(d => ({ label: d.name, value: d.id })))
const batchDeviceId = ref('')

/** 将选中的设备应用到所有已启用的通道 */
function applyDeviceToAll(): void {
  if (!batchDeviceId.value) return
  probeChannels.value.forEach((ch) => {
    if (ch.enabled) {
      ch.channel = { ...ch.channel, deviceId: batchDeviceId.value }
    }
  })
}

// ---- 批量操作：通道号自动递增 ----
const autoFillStartIndex = ref<number | null>(null)

/** 从指定起始通道号开始，自动递增填充所有已启用通道 */
function autoFillChannelIndices(): void {
  if (autoFillStartIndex.value === null) return
  let idx = autoFillStartIndex.value
  probeChannels.value.forEach((ch) => {
    if (ch.enabled) {
      ch.channel = { ...ch.channel, channelIndex: idx }
      idx++
    }
  })
}
</script>

<template>
  <div class="step-content">
    <!-- 批量操作工具栏 -->
    <div class="batch-toolbar">
      <div class="batch-toolbar-row">
        <!-- 统一选择设备 -->
        <div class="batch-cell">
          <span class="batch-label">{{ t.unifiedDevice }}</span>
          <UiSelect v-model="batchDeviceId" :options="deviceOptions" :placeholder="t.selectDevice" class="batch-select" :disabled="isLoading" />
        </div>
        <div class="batch-cell">
          <UiButton size="sm" variant="primary" :disabled="!batchDeviceId || isLoading" @click="applyDeviceToAll">{{ t.applyToAllChannels }}</UiButton>
        </div>
      </div>
      <div class="batch-toolbar-row">
        <!-- 通道号自动递增 -->
        <div class="batch-cell">
          <span class="batch-label">{{ t.startChannel }}</span>
          <UiSelect
            :model-value="autoFillStartIndex !== null ? String(autoFillStartIndex) : ''"
            @update:model-value="autoFillStartIndex = $event !== '' ? Number($event) : null"
            :options="channelIndexOptions.map(o => ({ label: o.label, value: String(o.value) }))"
            :placeholder="t.selectStartChannel"
            class="batch-select"
            :disabled="isLoading"
          />
        </div>
        <div class="batch-cell">
          <UiButton size="sm" variant="primary" :disabled="autoFillStartIndex === null || isLoading" @click="autoFillChannelIndices">{{ t.autoIncrementFill }}</UiButton>
        </div>
      </div>
    </div>

    <!-- 探头通道配置 -->
    <UiPanel class="section-card">
      <div class="hw-head"><span class="hdr-enabled">{{ t.channelEnabled }}</span><span class="hdr-name">{{ t.channelProbeName }}</span><span class="hdr-device">{{ t.channelDataSource }}</span><span class="hdr-w80">{{ t.channelIndexLabel }}</span><span class="hdr-w80">{{ t.channelPrecision }}</span></div>
      <div v-for="ch in probeChannels" :key="ch.name" class="hw-row">
        <div class="row-check"><UiCheckbox v-model:checked="ch.enabled" :disabled="isRequired(ch)" /></div>
        <div class="row-content">
          <div class="flex items-center gap-2">
            <span class="chan-name">{{ ch.name }}</span>
            <UiStatusBadge v-if="isRequired(ch)" status="connected">Required</UiStatusBadge>
          </div>
        </div>
        <div class="device-select-wrap">
          <UiSelect v-model="ch.channel.deviceId" :options="deviceOptions" :placeholder="t.selectDevice" class="sel-w150" :disabled="!ch.enabled || isLoading" />
          <span
            v-if="ch.channel.deviceId"
            class="device-status-dot"
            :class="`device-status-dot--${getDeviceStatus(ch.channel.deviceId)}`"
            :title="deviceStore.statusFor(ch.channel.deviceId)"
          />
        </div>
        <UiSelect :model-value="ch.channel.channelIndex != null ? String(ch.channel.channelIndex) : ''" @update:model-value="ch.channel.channelIndex = Number($event)" :options="channelIndexOptions.map(o => ({ label: o.label, value: String(o.value) }))" placeholder="Unassigned" class="sel-w80" :disabled="!ch.enabled" />
        <UiInputNumber v-model="ch.precision" :min="0" :max="8" class="sel-w80" :disabled="!ch.enabled" />
      </div>
    </UiPanel>

    <!-- 运动轴配置 -->
    <UiPanel class="section-card">
      <div class="hw-head"><span class="hdr-w50">{{ t.coordinateAxis }}</span><span class="hdr-name">{{ t.motionControllerLabel }}</span><span class="hdr-w80">{{ t.physicalAxis }}</span><span class="hdr-w90">{{ t.mappingLabel }}</span></div>
      <div v-for="ax in motionAxes" :key="ax.name" class="hw-row">
        <span class="axis-name">{{ ax.name }}</span>
        <UiSelect v-model="ax.controllerId" :options="motionStore.profiles.map(c => ({ label: c.name, value: c.id }))" :placeholder="t.selectController" class="sel-flex" :disabled="isLoading" />
        <UiSelect v-model="ax.axis" :options="axisOptions" class="sel-w80" />
        <UiSelect v-model="ax.angleMapping!.type" :options="mappingOptions" class="sel-w90" />
      </div>
    </UiPanel>

    <!-- 数据验证与稳定化配置：合并为紧凑的辅助配置区块 -->
    <UiPanel class="section-card compact-panel" :padded="false">
      <template #header><span class="batch-title">{{ t.travAdvancedConfig }}</span></template>

      <div class="compact-panel-inner">
        <!-- 数据验证：可选，用于校验压力范围和异常尖峰 -->
        <div class="compact-config-row">
          <label class="compact-option-label">
            <UiCheckbox v-model:checked="validationEnabled" size="small" />
            <span class="compact-label-text">{{ t.travEnableValidation }}</span>
          </label>
          <div v-if="validationEnabled" class="sub-config-inline">
            <span class="sub-config-label">{{ t.travErrorStrategy }}</span>
            <div class="radio-group compact-radio-group">
              <label v-for="opt in errorStrategyOptions" :key="opt.value" class="radio-label" :class="{ active: validationConfig.onInvalid === opt.value }">
                <input v-model="validationConfig.onInvalid" type="radio" :value="opt.value" />
                <span>{{ opt.label }}</span>
              </label>
            </div>
          </div>
        </div>

        <!-- 分割线 -->
        <div class="compact-divider"></div>

        <!-- 稳定化模式：fixed 使用固定等待时间，adaptive 持续监测压力变化 -->
        <div class="compact-config-row">
          <span class="compact-label-text">{{ t.travStableMode }}</span>
          <div class="radio-group compact-radio-group">
            <label v-for="opt in stabilizationOptions" :key="opt.value" class="radio-label" :class="{ active: stabilizationMode === opt.value }">
              <input v-model="stabilizationMode" type="radio" :value="opt.value" />
              <span>{{ opt.label }}</span>
            </label>
          </div>
          <div v-if="stabilizationMode === 'fixed'" class="sub-config-inline">
            <span class="sub-config-label">{{ t.travWaitTime }}</span>
            <UiInputNumber v-model="stabilizationConfig.fixedTimeMs" :min="100" :max="60000" class="compact-input" />
          </div>
          <div v-else class="sub-config-hint compact-hint">{{ t.travAdaptiveHint }}</div>
        </div>
      </div>
    </UiPanel>
  </div>
</template>

<style scoped>
.step-content { display:flex; flex-direction:column; gap:var(--space-2) }
.section-card { font-size:var(--text-sm) }

/* 批量操作栏：扁平化工具条，与右侧统计卡片风格协调 */
.batch-toolbar { padding: 10px 12px; border-radius: var(--radius-md); border: 1px solid var(--border-default); background: var(--bg-panel); display:flex; flex-direction:column; gap:8px }
.batch-toolbar-row { display:grid; grid-template-columns:160px 1fr; align-items:end; gap:10px }
.batch-cell { display:flex; flex-direction:column; gap:4px }
.batch-label { font-size:var(--text-xs); font-weight:500; color:var(--text-secondary); white-space:nowrap }
.batch-select { width:100% }

.hw-head { display:flex; align-items:center; gap:var(--space-2); padding-bottom:6px; border-bottom:1px solid var(--border-default) }
.hw-row { display:flex; align-items:center; gap:var(--space-2); padding:4px 0 }
.hw-row:hover { background:var(--bg-panel-strong); border-radius:var(--radius-md) }
.hdr-enabled { font-size:var(--text-xs);flex:0 0 32px;color:var(--text-muted) }
.hdr-name { font-size:var(--text-xs);flex:1;color:var(--text-muted) }
.hdr-device { font-size:var(--text-xs);width:150px;color:var(--text-muted) }
.hdr-w80 { font-size:var(--text-xs);width:80px;color:var(--text-muted) }
.hdr-w50 { font-size:var(--text-xs);width:50px;color:var(--text-muted) }
.hdr-w90 { font-size:var(--text-xs);width:90px;color:var(--text-muted) }
.row-check { flex:0 0 32px }
.row-content { flex:1;min-width:0 }
.chan-name { font-size:var(--text-sm);color:var(--text-primary) }
.axis-name { font-size:var(--text-sm);font-weight:600;width:50px;color:var(--text-primary) }
.sel-w150 { width:150px }
.sel-w120 { width:120px }
.sel-w80 { width:80px }
.sel-w90 { width:90px }
.sel-flex { flex:1 }
.option-label { display:flex; align-items:center; gap:8px; font-size:var(--text-sm); color:var(--text-primary); cursor:pointer; min-height:36px }
.sub-config-block { margin-top:var(--space-2); padding-left:var(--space-4); display:flex; flex-direction:column; gap:4px }
.sub-config-label { font-size:var(--text-xs); color:var(--text-tertiary) }
.sub-config-hint { margin-top:var(--space-2); font-size:var(--text-xs); color:var(--text-tertiary); line-height:1.5 }
.radio-group { display:flex; flex-wrap:wrap; gap:var(--space-2); margin-top:4px }
.radio-label { display:flex; align-items:center; gap:8px; padding:8px 10px; font-size:var(--text-sm); color:var(--text-primary); cursor:pointer; border-radius:var(--radius-md); border:1px solid var(--border-default); min-height:36px }
.radio-label input[type="radio"] { margin:0 }
.radio-label:hover { background:var(--bg-panel-strong) }
.radio-label.active { border-color:var(--color-primary); background:var(--color-primary-light, rgba(59,130,246,0.1)); color:var(--color-primary) }
.w-full { width:100% }

/* 紧凑布局：合并数据验证和稳定模式为单个面板，使用自定义内边距消除多余空白 */
.compact-panel :deep(.n-card__content) { padding: 0 }
.compact-panel .compact-panel-inner { padding: var(--space-2) var(--space-3) }
.compact-panel .compact-config-row { padding: 2px 0 }
.compact-panel .compact-option-label { display:flex; align-items:center; gap:8px; font-size:var(--text-sm); color:var(--text-primary); cursor:pointer; min-height:24px }
.compact-panel .compact-label-text { font-size:var(--text-sm); font-weight:500; color:var(--text-primary) }
.compact-panel .compact-divider { height:1px; background:var(--border-default); margin:4px 0 }
.compact-panel .sub-config-inline { margin-top:2px; display:flex; flex-direction:column; gap:2px }
.compact-panel .compact-radio-group { margin-top:2px; gap:6px }
.compact-panel .compact-radio-group .radio-label { padding:4px 8px; min-height:28px; font-size:var(--text-xs) }
.compact-panel .compact-input { width:140px }
.compact-panel .compact-hint { margin-top:2px; font-size:var(--text-xs) }

/* 设备选择器与状态指示 */
.device-select-wrap { display:flex; align-items:center; gap:6px; width:150px }
.device-status-dot { display:inline-block; width:8px; height:8px; border-radius:50%; flex-shrink:0 }
.device-status-dot--idle { background:var(--text-muted); }
.device-status-dot--connected { background:var(--color-success, #22c55e); }
.device-status-dot--acquiring { background:var(--color-success, #22c55e); box-shadow:0 0 0 2px rgba(34,197,94,0.3); animation:pulse-dot 1.5s ease-in-out infinite; }
.device-status-dot--warning { background:var(--color-warning, #f59e0b); }
.device-status-dot--error { background:var(--color-error, #ef4444); }

@keyframes pulse-dot {
  0%, 100% { opacity:1; transform:scale(1); }
  50% { opacity:0.5; transform:scale(0.8); }
}
</style>
