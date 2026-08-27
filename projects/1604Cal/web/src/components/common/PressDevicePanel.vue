<template>
  <div class="device-panel">
    <div class="panel-header">
      <div class="device-info">
        <el-icon class="device-icon">
          <FirstAidKit />
        </el-icon>
        <div>
          <div class="device-name">
            打压设备
          </div>
          <div class="device-type">
            压力控制器
          </div>
        </div>
      </div>
      <DeviceStatusBadge :status="deviceStatus" />
    </div>

    <div class="selection-control">
      <el-select
        v-model="selectedDeviceId"
        placeholder="选择打压设备"
        :disabled="isConnected"
        size="small"
        class="device-select"
      >
        <el-option
          v-for="dev in pressureDevices"
          :key="dev.id"
          :label="dev.name || dev.model || '未命名设备'"
          :value="dev.id"
        >
          <div class="device-option">
            <span class="device-option-name">{{ dev.name || dev.model || '未命名设备' }}</span>
            <span :class="['device-option-status', `status-${dev.status}`]">
              {{ statusLabel(dev.status) }}
            </span>
          </div>
        </el-option>
        <template #empty>
          <div class="empty-hint">
            暂无设备，请先在设备管理中添加
          </div>
        </template>
      </el-select>
      <el-button
        :type="isConnected ? 'danger' : 'primary'"
        :loading="isConnecting"
        :disabled="!selectedDeviceId && !isConnected"
        size="small"
        @click="toggleConnection"
      >
        {{ isConnected ? '断开' : '连接' }}
      </el-button>
    </div>

    <div
      v-if="isConnected"
      class="pressure-control"
    >
      <div class="current-pressure">
        <span class="label">当前压力:</span>
        <span class="value">{{ currentPressure?.toFixed(2) || '--' }}
          <span class="unit">{{ selectedUnit }}</span>
        </span>
      </div>
      <div class="unit-row">
        <span class="label">单位:</span>
        <el-select
          v-model="selectedUnit"
          size="small"
          class="unit-select"
          @change="onUnitChange"
        >
          <el-option
            v-for="u in availableUnitOptions"
            :key="u.value"
            :label="u.label"
            :value="u.value"
          />
        </el-select>
      </div>
      <div class="pressure-actions">
        <div class="pressure-row">
          <el-input
            v-model.number="targetPressure"
            type="number"
            :step="1"
            class="target-input"
          />
          <el-button
            type="primary"
            @click="setPressure"
          >
            设定压力
          </el-button>
        </div>
        <el-button
          type="danger"
          class="exhaust-btn"
          @click="exhaustPressure"
        >
          排空
        </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { FirstAidKit } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import DeviceStatusBadge from '@/components/common/DeviceStatusBadge.vue'
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import { upsertDevice } from '@/api/device'
import { isConst820Model } from '@/utils/deviceModels'
import {
  multipressSetUnit,
  multipressSetPressure,
  multipressExhaust,
  multipressListDevices
} from "@/api/multipress"

const emit = defineEmits<{
  connect: [deviceId: string]
  disconnect: [deviceId: string]
  'set-pressure': [deviceId: string, pressure: number]
  exhaust: [deviceId: string]
  'unit-change': [payload: { deviceId: string; unit: string }]
}>()

const deviceStore = useDeviceInventoryStore()
const { pressureDevices } = storeToRefs(deviceStore)

const selectedDeviceId = ref('')
const selectedUnit = ref('kPa')
const targetPressure = ref(10)

const unitOptions = [
  { value: 'kPa', label: 'kPa' },
  { value: 'MPa', label: 'MPa' },
  { value: 'Pa', label: 'Pa' },
  { value: 'bar', label: 'bar' },
  { value: 'mbar', label: 'mbar' },
  { value: 'psi', label: 'psi' },
  { value: 'kgf/cm2', label: 'kgf/cm²' }
]

const unitOptions820 = unitOptions.filter(unit =>
  ['Pa', 'kPa', 'MPa', 'psi', 'kgf/cm2'].includes(unit.value)
)

// 获取选中的打压设备
const device = computed(() =>
  deviceStore.pressureDevices.find(d => d.id === selectedDeviceId.value)
)

const availableUnitOptions = computed(() =>
  isConst820Model(device.value?.model) ? unitOptions820 : unitOptions
)

// 计算状态
const isConnected = computed(() => device.value?.status === 'connected')
const isConnecting = computed(() => device.value?.status === 'connecting')
const deviceStatus = computed(() => {
  if (!device.value) return 'disconnected'
  if (device.value.status === 'connected') return 'connected'
  if (device.value.status === 'connecting') return 'connecting'
  if (device.value.status === 'error') return 'error'
  return 'disconnected'
})
const currentPressure = computed(() => device.value?.currentPressure)

// 自动选中第一个可用设备
watch(
  pressureDevices,
  (devices) => {
    if (!selectedDeviceId.value && devices.length > 0) {
      selectedDeviceId.value = devices[0].id
    }
    // 如果选中设备已不存在，清空选择
    if (selectedDeviceId.value && !devices.find(d => d.id === selectedDeviceId.value)) {
      selectedDeviceId.value = devices.length > 0 ? devices[0].id : ''
    }
  },
  { immediate: true }
)

// 设备连接后同步单位（watch device.unit 而非 device 对象，避免对象引用不变时 watch 不触发）
watch(() => device.value?.unit, (unit) => {
  if (unit) {
    selectedUnit.value = unit
  }
}, { immediate: true })

// 切换单位时同步到后端并更新本地 store
async function onUnitChange(unit: string) {
  if (!device.value) return
  const previousUnit = device.value.unit
  device.value.unit = unit
  try {
    await multipressSetUnit(device.value.id, unit)
    // 同步单位到设备配置，确保 CheckUnitConsistency 比较的是实际单位
    try {
      await upsertDevice({
        id: device.value.id,
        name: device.value.name,
        type: 'pressure',
        model: device.value.model,
        host: device.value.ip,
        port: device.value.port,
        unit: device.value.unit,
        status: device.value.status
      })
    } catch (syncErr) {
      console.warn('同步打压设备单位到配置失败:', syncErr)
    }
    // 切换单位后从后端拉取最新状态（含已按新单位换算的压力值）
    const states = await multipressListDevices()
    const s = states.find(d => d.deviceId === device.value!.id)
    if (s && device.value) {
      device.value.currentPressure = s.currentPressure
      device.value.unit = s.unit
    }
    ElMessage.success(`打压单位已切换为 ${unit}`)
    emit('unit-change', { deviceId: device.value.id, unit })
  } catch {
    device.value.unit = previousUnit
    ElMessage.error('设置打压单位失败')
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'connected': return '已连接'
    case 'connecting': return '连接中'
    case 'error': return '异常'
    default: return '未连接'
  }
}

const toggleConnection = async () => {
  if (!selectedDeviceId.value) return

  if (isConnected.value) {
    emit('disconnect', selectedDeviceId.value)
  } else {
    emit('connect', selectedDeviceId.value)
  }
}

const setPressure = async () => {
  if (!device.value) return
  try {
    // 设定压力使用设备当前单位；单位切换是独立操作，失败不能阻断压力设定。
    await multipressSetPressure(device.value.id, targetPressure.value)
    emit('set-pressure', device.value.id, targetPressure.value)
  } catch (err) {
    console.warn('[PressDevicePanel] 设置压力失败:', err)
    // 将后端错误（如超出量程）反馈给用户，避免设备滴滴响后界面无提示。
    const msg = err instanceof Error ? err.message : String(err)
    ElMessage.error(`设定压力失败：${msg}`)
  }
}

const exhaustPressure = async () => {
  if (!device.value) return
  try {
    await multipressExhaust(device.value.id)
    emit('exhaust', device.value.id)
  } catch (err) {
    console.warn('[PressDevicePanel] 排气失败:', err)
  }
}
</script>

<style scoped lang="scss">
.device-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  font-family: $font-sans;

  /* 头部：设备信息 + 状态徽章 */
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;

    .device-info {
      display: flex;
      align-items: center;
      gap: 10px;

      .device-icon {
        font-size: 20px;
        color: $blue;
        background: rgba(59, 130, 246, 0.1);
        width: 36px;
        height: 36px;
        border-radius: 8px;
        display: flex;
        align-items: center;
        justify-content: center;
      }

      .device-name {
        color: $slate-800;
        font-weight: 600;
        font-size: 14px;
        line-height: 1.3;
      }

      .device-type {
        color: $slate-500;
        font-size: 12px;
        font-weight: 400;
        line-height: 1.3;
      }
    }
  }

  /* 连接控制区 */
  .selection-control {
    display: flex;
    gap: 8px;
    align-items: center;

    .device-select {
      flex: 1;
    }
  }

  /* 下拉选项 */
  .device-option {
    display: flex;
    justify-content: space-between;
    align-items: center;

    .device-option-name {
      color: $slate-700;
      font-size: 13px;
    }

    .device-option-status {
      font-size: 10px;
      font-weight: 600;
      padding: 2px 6px;
      border-radius: 4px;
      letter-spacing: 0.02em;
    }

    .status-connected {
      background: rgba(34, 197, 94, 0.12);
      border: 1px solid rgba(34, 197, 94, 0.25);
      color: #16a34a;
    }

    .status-connecting {
      background: rgba(245, 158, 11, 0.12);
      border: 1px solid rgba(245, 158, 11, 0.25);
      color: #d97706;
    }

    .status-disconnected {
      background: rgba(107, 114, 128, 0.08);
      border: 1px solid rgba(107, 114, 128, 0.18);
      color: $slate-500;
    }

    .status-error {
      background: rgba(239, 68, 68, 0.1);
      border: 1px solid rgba(239, 68, 68, 0.2);
      color: #dc2626;
    }
  }

  .empty-hint {
    padding: 12px;
    color: $slate-400;
    font-size: 12px;
    text-align: center;
    font-family: $font-sans;
  }

  /* 压力控制区（连接后显示） */
  .pressure-control {
    background: $slate-50;
    border-radius: 8px;
    border: 1px solid $slate-100;
    padding: 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;

    .current-pressure {
      display: flex;
      justify-content: space-between;
      align-items: baseline;

      .label {
        color: $slate-500;
        font-size: 12px;
        font-weight: 500;
        letter-spacing: 0.02em;
      }

      .value {
        color: $mint;
        font-size: 24px;
        font-weight: 700;
        font-variant-numeric: tabular-nums;
        font-family: $font-mono;
        line-height: 1;

        .unit {
          font-size: 12px;
          font-weight: 500;
          color: $slate-500;
          margin-left: 4px;
          font-family: $font-sans;
        }
      }
    }

    .unit-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 10px;

      .label {
        color: $slate-500;
        font-size: 12px;
        font-weight: 500;
        letter-spacing: 0.02em;
        flex-shrink: 0;
      }

      .unit-select {
        width: 130px;
      }
    }

    .pressure-actions {
      display: flex;
      flex-direction: column;
      gap: 8px;
      padding-top: 4px;
      border-top: 1px solid $slate-200;

      .pressure-row {
        display: flex;
        gap: 8px;
        align-items: center;

        .target-input {
          flex: 1;
          min-width: 0;
        }

        .target-input :deep(.el-input__wrapper) {
          height: 36px;
        }

        .target-input :deep(.el-input__inner) {
          font-size: 14px;
        }

        .target-input :deep(input::-webkit-outer-spin-button),
        .target-input :deep(input::-webkit-inner-spin-button) {
          -webkit-appearance: none;
          margin: 0;
        }

        .target-input :deep(input[type="number"]) {
          -moz-appearance: textfield;
        }

        .el-button {
          height: 28px;
          font-size: 12px;
          font-weight: 600;
          border-radius: 6px;
          padding: 0 6px;
          flex-shrink: 0;
        }
      }

      .exhaust-btn {
        width: 100%;
        height: 28px;
        font-size: 12px;
        font-weight: 600;
        border-radius: 6px;
      }
    }
  }
}
</style>
