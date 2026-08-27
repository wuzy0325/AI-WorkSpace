<template>
  <section class="selector-panel">
    <header class="selector-header">
      <h3>设备选择</h3>
      <button
        type="button"
        class="btn btn-ghost"
        @click="refreshDevices"
      >
        <el-icon><Refresh /></el-icon>
        刷新列表
      </button>
    </header>

    <div class="selector-grid">
      <div class="selector-field">
        <span>计量设备（可多选）</span>
        <div class="checkbox-list">
          <label
            v-for="device in measureDevices"
            :key="device.id"
            class="checkbox-item"
          >
            <input
              v-model="selectedMeasureDeviceIds"
              type="checkbox"
              :value="device.id"
            >
            <span>{{ device.name || device.id }}</span>
            <span :class="['checkbox-status', `status-${device.status}`]">
              {{ statusLabel(device.status) }}
            </span>
          </label>
          <div
            v-if="measureDevices.length === 0"
            class="empty-hint"
          >
            暂无计量设备
          </div>
        </div>
      </div>

      <label>
        <span>打压设备</span>
        <div class="select-wrapper">
          <select v-model="selectedPressureDeviceId">
            <option value="">
              请选择打压设备
            </option>
            <option
              v-for="device in pressureDevices"
              :key="device.id"
              :value="device.id"
            >
              {{ device.name || device.id }} ({{ statusLabel(device.status) }})
            </option>
          </select>
          <el-icon class="select-icon"><ArrowDown /></el-icon>
        </div>
      </label>
    </div>

    <div class="selection-summary">
      <div class="summary-item">
        <el-icon><Tools /></el-icon>
        <div>
          <span class="summary-label">计量设备</span>
          <span class="summary-value">{{ selectedMeasureDeviceNames }}</span>
          <span
            v-if="selectedMeasureDeviceIds.length > 0"
            class="summary-count"
          >
            {{ selectedMeasureDeviceIds.length }} 台
          </span>
        </div>
      </div>
      <div class="summary-divider" />
      <div class="summary-item">
        <el-icon><DArrowRight /></el-icon>
        <div>
          <span class="summary-label">打压设备</span>
          <span class="summary-value">{{ selectedPressureDeviceName }}</span>
          <span :class="['summary-status', `status-${selectedPressureDevice?.status || 'disconnected'}`]">
            {{ statusLabel(selectedPressureDevice?.status || 'disconnected') }}
          </span>
        </div>
      </div>
    </div>

    <div
      v-if="errorMessage"
      class="error-message"
    >
      <el-icon><Warning /></el-icon>
      {{ errorMessage }}
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Refresh, ArrowDown, Tools, DArrowRight, Warning } from '@element-plus/icons-vue'

import { fetchDevices } from "@/api/device"
import type { DeviceDTO } from "@/types/device"
import { useDeviceStore, type ModuleKey } from '@/stores/deviceStore'

interface Props {
  moduleKey: ModuleKey
}

const props = defineProps<Props>()

const deviceStore = useDeviceStore()

const devices = ref<DeviceDTO[]>([])
const errorMessage = ref('')

const measureDevices = computed(() => devices.value.filter((item) => item.type === 'measure'))
const pressureDevices = computed(() => devices.value.filter((item) => item.type === 'pressure'))

const selectedMeasureDeviceIds = computed({
  get: () => deviceStore.selectionByModule(props.moduleKey).measureDeviceIds,
  set: (value: string[]) => {
    deviceStore.setModuleSelection(props.moduleKey, { measureDeviceIds: value })
  }
})

const selectedPressureDeviceId = computed({
  get: () => deviceStore.selectionByModule(props.moduleKey).pressureDeviceId,
  set: (value: string) => {
    deviceStore.setModuleSelection(props.moduleKey, { pressureDeviceId: value })
  }
})

const selectedMeasureDeviceNames = computed(() => {
  const names = selectedMeasureDeviceIds.value
    .map(id => measureDevices.value.find(item => item.id === id)?.name || id)
  return names.length > 0 ? names.join('、') : '未选择'
})

const selectedPressureDeviceName = computed(() => {
  const selected = pressureDevices.value.find((item) => item.id === selectedPressureDeviceId.value)
  return selected?.name || selected?.id || '未选择'
})

const selectedPressureDevice = computed(() => {
  return pressureDevices.value.find((item) => item.id === selectedPressureDeviceId.value)
})

async function refreshDevices() {
  errorMessage.value = ''
  try {
    devices.value = await fetchDevices()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '获取设备列表失败'
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'connected':
      return '已连接'
    case 'connecting':
      return '连接中'
    case 'error':
      return '异常'
    default:
      return '未连接'
  }
}

watch(
  devices,
  (list) => {
    // 清理已失效的计量设备勾选
    const validMeasure = selectedMeasureDeviceIds.value.filter(id =>
      list.some(item => item.type === 'measure' && item.id === id)
    )
    if (validMeasure.length !== selectedMeasureDeviceIds.value.length) {
      selectedMeasureDeviceIds.value = validMeasure
    }

    const pressureExists = list.some((item) => item.type === 'pressure' && item.id === selectedPressureDeviceId.value)
    if (!pressureExists) {
      selectedPressureDeviceId.value = ''
    }
  },
  { deep: false }
)

onMounted(async () => {
  await refreshDevices()
  // 恢复上次成功绑定的设备勾选（多设备按勾选顺序），tasks 10.1 闭环。
  await deviceStore.restoreLastDevices()
})
</script>

<style scoped lang="scss">
.selector-panel {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 4px;
  padding: var(--spacing-sm);
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.selector-header {
  align-items: center;
  display: flex;
  gap: var(--spacing-sm);
  justify-content: space-between;
  margin-bottom: var(--spacing-xs);
}

.selector-header h3 {
  color: var(--text-primary);
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: 4px 10px;
  border: 1px solid var(--border-color-strong);
  border-radius: 3px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  background: transparent;
  color: var(--text-secondary);

  .el-icon {
    font-size: 12px;
  }
}

.btn-ghost {
  &:hover {
    background: var(--accent-primary);
    border-color: var(--accent-primary);
    color: var(--bg-primary);
  }
}

.selector-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-xs);
}

.selector-grid label,
.selector-field {
  color: var(--text-secondary);
  display: flex;
  flex-direction: column;
  font-size: 12px;
  gap: var(--spacing-xs);
}

.checkbox-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 160px;
  overflow-y: auto;
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color-strong);
  border-radius: 3px;
  padding: var(--spacing-xs);
}

.checkbox-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-primary);
  cursor: pointer;

  input[type='checkbox'] {
    accent-color: var(--accent-primary);
    cursor: pointer;
  }
}

.checkbox-status {
  font-size: 12px;
  font-weight: 500;
  padding: 1px 5px;
  border-radius: 2px;
  margin-left: auto;
}

.empty-hint {
  color: var(--text-muted);
  font-size: 12px;
  text-align: center;
  padding: 8px 0;
}

.select-wrapper {
  position: relative;
}

.selector-grid select {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color-strong);
  border-radius: 3px;
  color: var(--text-primary);
  padding: var(--spacing-sm) var(--spacing-lg) var(--spacing-sm) var(--spacing-sm);
  width: 100%;
  appearance: none;
  cursor: pointer;
  font-size: 12px;

  &:focus {
    outline: none;
    border-color: var(--accent-primary);
  }
}

.select-icon {
  position: absolute;
  right: var(--spacing-sm);
  top: 50%;
  transform: translateY(-50%);
  color: var(--text-muted);
  font-size: 11px;
  pointer-events: none;
}

.selection-summary {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: 3px;
  padding: var(--spacing-sm);
  margin-top: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 1px minmax(0, 1fr);
  align-items: center;
  gap: var(--spacing-sm);
}

.summary-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);

  .el-icon {
    font-size: 14px;
    color: var(--accent-primary);
  }

  > div {
    display: flex;
    flex-direction: column;
  }
}

.summary-label {
  color: var(--text-muted);
  font-size: 10px;
}

.summary-value {
  color: var(--text-primary);
  font-size: 13px;
  font-weight: 500;
  max-width: 160px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.summary-status {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 2px;
  margin-left: var(--spacing-xs);
}

.summary-count {
  font-size: 12px;
  font-weight: 500;
  padding: 1px 5px;
  border-radius: 2px;
  margin-left: var(--spacing-xs);
  background: var(--accent-primary-subtle, rgba(16, 185, 129, 0.12));
  color: var(--accent-primary);
}

.status-connected {
  background: var(--status-success-bg);
  color: var(--status-success);
}

.status-connecting {
  background: var(--status-warning-bg);
  color: var(--status-warning);
}

.status-disconnected {
  background: var(--bg-secondary);
  color: var(--text-muted);
}

.status-error {
  background: var(--status-error-bg);
  color: var(--status-error);
}

.summary-divider {
  width: 1px;
  height: 32px;
  background: var(--border-color-strong);
  margin: 0;
}

.error-message {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  color: var(--status-error);
  font-size: 12px;
  margin-top: var(--spacing-md);
  padding: var(--spacing-sm);
  background: var(--status-error-bg-subtle);
  border-radius: 3px;

  .el-icon {
    font-size: 13px;
  }
}

@media (max-width: 900px) {
  .selector-grid {
    grid-template-columns: 1fr;
  }

  .selection-summary {
    grid-template-columns: 1fr;
  }

  .summary-divider {
    width: 100%;
    height: 1px;
  }
}
</style>
