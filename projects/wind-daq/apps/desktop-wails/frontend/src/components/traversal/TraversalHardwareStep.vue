<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ProbeChannelConfig } from '@shared/types/traversal'
import { findDuplicateChannelBindings, channelBindingKey, isTraversalRequiredProbeChannel } from '@shared/types/traversal'
import { useDeviceStore } from '@stores/deviceStore'
import UiPanel from '@components/ui/UiPanel.vue'
import UiCheckbox from '@components/ui/UiCheckbox.vue'
import UiSelect from '@components/ui/UiSelect.vue'
import UiStatusBadge from '@components/ui/UiStatusBadge.vue'
import UiButton from '@components/ui/UiButton.vue'
import UiInputNumber from '@components/ui/UiInputNumber.vue'

const probeChannels = defineModel<ProbeChannelConfig[]>('probeChannels', { required: true })

defineProps<{
  t: Record<string, string>
  isLoading: boolean
}>()

const deviceStore = useDeviceStore()

function isRequired(c: ProbeChannelConfig) { return isTraversalRequiredProbeChannel(c.role, c.name) }

// 通道索引枚举选项：UI 显示 CH1~CH18（1-based），内部 value 仍为数组索引 0~17
// 通道序号从 1 开始更符合操作员直觉，对应底层数组的 0-based 索引
const channelIndexOptions = Array.from({ length: 18 }, (_, i) => ({ label: `CH${i + 1}`, value: i }))

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

// 通道绑定重复检测：多个 enabled 通道绑定同一「设备+通道号」时给出视觉错误提示。
// 不同设备的通道号允许重复（各设备独立编号），跨设备绑定不算冲突。
// 后端 ParseConfig 按设备+通道号检测重复并拒绝启动（见 traversal_config.go channels 收集），
// 因此前端必须阻断保存——视觉错误样式在此处，实际阻断在 TraversalSettings.vue 的 isStepValid。
// 仅检测 enabled 通道；未启用通道不参与采样，重复无影响。
// 重复检测算法提取到 shared/types/traversal.ts 的 findDuplicateChannelBindings，
// 与 TraversalSettings.vue 的 isStepValid 共享同一真相源，避免双实现漂移。
const duplicateChannelBindings = computed<Set<string>>(() => findDuplicateChannelBindings(probeChannels.value))

function isChannelDuplicate(ch: ProbeChannelConfig): boolean {
  if (!ch.enabled || ch.channel.channelIndex == null || ch.channel.channelIndex < 0) return false
  return duplicateChannelBindings.value.has(channelBindingKey(ch))
}
</script>

<template>
  <div class="step-content">
    <!-- 批量操作工具栏：每个批量组内部"标签在上、控件在下"垂直堆叠，两组并排排列，极窄视口才换行 -->
    <div class="batch-toolbar">
      <!-- 统一选择设备：标签在上、控件在下垂直堆叠，组内紧凑、组间并排，避免对话框常见宽度下整组换行 -->
      <div class="batch-group">
        <span class="batch-label">{{ t.unifiedDevice }}</span>
        <div class="batch-row">
          <UiSelect v-model="batchDeviceId" :options="deviceOptions" :placeholder="t.selectDevice" class="batch-select" :disabled="isLoading" />
          <UiButton size="sm" variant="primary" :disabled="!batchDeviceId || isLoading" @click="applyDeviceToAll">{{ t.applyToAllChannels }}</UiButton>
        </div>
      </div>
      <!-- 通道号自动递增 -->
      <div class="batch-group">
        <span class="batch-label">{{ t.startChannel }}</span>
        <div class="batch-row">
          <UiSelect
            :model-value="autoFillStartIndex !== null ? String(autoFillStartIndex) : ''"
            @update:model-value="autoFillStartIndex = $event !== '' ? Number($event) : null"
            :options="channelIndexOptions.map(o => ({ label: o.label, value: String(o.value) }))"
            :placeholder="t.selectStartChannel"
            class="batch-select"
            :disabled="isLoading"
          />
          <UiButton size="sm" variant="primary" :disabled="autoFillStartIndex === null || isLoading" @click="autoFillChannelIndices">{{ t.autoIncrementFill }}</UiButton>
        </div>
      </div>
    </div>

    <!-- 探头通道配置 -->
    <UiPanel class="section-card">
      <div class="hw-head">
        <span class="hdr-enabled">{{ t.channelEnabled }}</span>
        <span class="hdr-name">{{ t.channelProbeName }}</span>
        <span class="hdr-device">{{ t.channelDataSource }}</span>
        <span class="hdr-w80">{{ t.channelIndexLabel }}</span>
        <span class="hdr-w80">{{ t.channelPrecision }}</span>
      </div>
      <div v-for="ch in probeChannels" :key="ch.name" class="hw-row">
        <div class="row-check"><UiCheckbox v-model:checked="ch.enabled" :disabled="isRequired(ch)" /></div>
        <div class="row-content">
          <div class="chan-name-wrap">
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
        <UiSelect
          :model-value="ch.channel.channelIndex != null ? String(ch.channel.channelIndex) : ''"
          @update:model-value="ch.channel.channelIndex = Number($event)"
          :options="channelIndexOptions.map(o => ({ label: o.label, value: String(o.value) }))"
          placeholder="Unassigned"
          class="sel-w80"
          :class="{ 'sel-channel-error': isChannelDuplicate(ch) }"
          :disabled="!ch.enabled"
          :title="isChannelDuplicate(ch) ? t.channelDuplicateHint : undefined"
        />
        <UiInputNumber v-model="ch.precision" :min="0" :max="8" class="sel-w80" :disabled="!ch.enabled" />
      </div>
    </UiPanel>
  </div>
</template>

<style scoped>
/* 步骤内容：紧凑垂直间距，与 Layout/Review 步骤视觉一致 */
.step-content { display: flex; flex-direction: column; gap: 6px }
.section-card { font-size: var(--text-sm) }

/* 紧凑化 UiPanel 内边距：默认 var(--space-3) var(--space-4) 偏大，覆盖为更紧凑的 4px 8px；
   仅作用于本组件内的 NCard content，避免污染全局 UiPanel 视觉 */
.section-card :deep(.n-card__content) {
  padding: 4px 8px
}
/* 带 header 的面板（如运动轴配置无 header，但保留兜底）收紧 header padding */
.section-card :deep(.n-card-header) {
  padding: 4px 8px
}

/* 批量操作栏：扁平化工具条，每个批量组内部采用“标签在上、控件在下”的紧凑垂直堆叠，
   让两个组在常见对话框宽度下稳定并排；组间用分隔线区分，极窄视口才允许换行。 */
.batch-toolbar {
  padding: 6px 8px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 12px
}
.batch-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0
}
.batch-group + .batch-group {
  border-left: 1px solid var(--border-default);
  padding-left: 12px
}
.batch-row {
  display: flex;
  align-items: center;
  gap: 6px
}
.batch-label {
  font-size: var(--text-xs);
  font-weight: 500;
  color: var(--text-secondary);
  white-space: nowrap
}
.batch-select { flex: 1; min-width: 60px; max-width: none }

@media (max-width: 640px) {
  .batch-toolbar { grid-template-columns: minmax(0, 1fr) }
  .batch-group + .batch-group { border-left: 0; padding-left: 0 }
}

/* 通道/运动轴表头与行：紧凑高度，列宽固定对齐 */
.hw-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding-bottom: 3px;
  border-bottom: 1px solid var(--border-default)
}
.hw-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 2px 0
}
.hw-row:hover { background: var(--bg-panel-strong); border-radius: var(--radius-md) }
.hdr-enabled { font-size: var(--text-xs); flex: 0 0 32px; color: var(--text-muted) }
.hdr-name { font-size: var(--text-xs); flex: 1; color: var(--text-muted) }
.hdr-device { font-size: var(--text-xs); width: 150px; color: var(--text-muted) }
.hdr-w80 { font-size: var(--text-xs); width: 80px; color: var(--text-muted) }
.row-check { flex: 0 0 32px }
.row-content { flex: 1; min-width: 0 }
.chan-name-wrap { display: flex; align-items: center; gap: 6px }
.chan-name { font-size: var(--text-sm); color: var(--text-primary) }
.sel-w150 { width: 150px }
.sel-w80 { width: 80px }

/* 通道号重复错误：选择器边框高亮错误色，hover title 显示原因。
   多个探针通道绑定了同一通道号，后端 ParseConfig 会直接报错不启动测试；
   仅做视觉提示，实际阻断在 TraversalSettings.vue 的 isStepValid 中实施。
   扇形轴类型不匹配（.sel-axis-error）已随运动轴配置迁至 TraversalLayoutStep.vue。 */
.sel-channel-error :deep(.n-base-selection) {
  border-color: var(--state-error) !important;
  box-shadow: 0 0 0 2px var(--chart-band-danger);
}

/* 设备选择器与状态指示 */
.device-select-wrap { display: flex; align-items: center; gap: 6px; width: 150px; min-width: 0 }
.device-select-wrap .sel-w150 { width: auto; min-width: 0; flex: 1 }
.device-status-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0 }
.device-status-dot--idle { background: var(--text-muted); }
.device-status-dot--connected { background: var(--color-success, #22c55e); }
.device-status-dot--acquiring { background: var(--color-success, #22c55e); box-shadow: 0 0 0 2px rgba(34, 197, 94, 0.3); animation: pulse-dot 1.5s ease-in-out infinite; }
.device-status-dot--warning { background: var(--color-warning, #f59e0b); }
.device-status-dot--error { background: var(--color-error, #ef4444); }

@keyframes pulse-dot {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.8); }
}
</style>
