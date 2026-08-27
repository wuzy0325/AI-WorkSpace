<template>
  <section class="device-panel">
    <header class="panel-header">
      <div>
        <h2>设备统一管理面板</h2>
        <p>统一维护计量设备与打压设备，实时观察状态、错误与连接策略。</p>
      </div>

      <div class="header-actions">
        <button
          data-test="refresh-devices"
          type="button"
          class="btn btn-ghost"
          @click="refreshAll"
        >
          <el-icon><Refresh /></el-icon>
          立即刷新
        </button>
        <button
          data-test="add-device"
          type="button"
          class="btn btn-primary"
          @click="openCreateDialog"
        >
          <el-icon><Plus /></el-icon>
          新增设备
        </button>
      </div>
    </header>

    <DeviceToolbar
      :error-count="errorCount"
      :connected-count="connectedCount"
      :total-count="devices.length"
      :measure-count="measureCount"
      :pressure-count="pressureCount"
      :unit-status-text="unitStatusText"
      :unit-consistent="unitConsistent"
      :connect-policy-text="connectPolicyText"
      :disconnect-policy-text="disconnectPolicyText"
      :last-refresh-text="lastRefreshText"
      :auto-refresh="autoRefresh"
      :type-filter="typeFilter"
      :status-filter="statusFilter"
      :keyword="keyword"
      @update:auto-refresh="autoRefresh = $event"
      @update:type-filter="typeFilter = $event as DeviceFilterType"
      @update:status-filter="statusFilter = $event as DeviceStatusFilter"
      @update:keyword="keyword = $event"
      @reset-filters="resetFilters"
    />

    <section class="device-board">
      <p
        v-if="visibleDevices.length === 0"
        class="empty"
      >
        <el-icon><InfoFilled /></el-icon>
        暂无符合筛选条件的设备
      </p>

      <DeviceCard
        v-for="device in visibleDevices"
        :key="device.id"
        :device="device"
        @edit="openEditDialog"
        @toggle-connection="toggleConnection"
        @delete="handleDeleteDevice"
      />
    </section>

    <!-- 连接中进度对话框 -->
    <el-dialog
      v-model="showConnectDialog"
      title="设备连接中"
      width="400px"
      :close-on-click-modal="false"
      :show-close="false"
      :close-on-press-escape="false"
    >
      <div style="text-align:center;padding:20px 0">
        <el-icon
          style="font-size:36px;margin-bottom:12px"
          class="is-loading"
        >
          <Loading />
        </el-icon>
        <p style="margin:8px 0 0;color:#666;font-size:14px">
          {{ connectProgressMessage }}
        </p>
      </div>
    </el-dialog>

    <!-- 设备创建/编辑对话框 -->
    <DeviceFormDialog
      v-model:visible="dialogVisible"
      :mode="dialogMode"
      :existing-ids="existingIds"
      :initial-device="editingDevice"
      @save="handleFormSave"
      @cancel="dialogVisible = false"
    />

    <p
      v-if="errorMessage"
      class="error-banner"
    >
      <el-icon><Warning /></el-icon>
      {{ errorMessage }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  Refresh,
  Plus,
  InfoFilled,
  Loading,
  Warning
} from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'

import {
  connectDevice,
  deleteDevice,
  disconnectDevice,
  upsertDevice
} from "@/api/device"
import {
  multipressRegister,
  multipressUnregister
} from "@/api/multipress"
import type {
  DeviceConnectConfigDTO,
  DeviceDTO
} from "@/types/device"

import DeviceFormDialog from './DeviceFormDialog.vue'
import DeviceCard from './DeviceCard.vue'
import DeviceToolbar from './DeviceToolbar.vue'
import { useDevicePolling } from '@/composables/useDevicePolling'

type DeviceFilterType = 'all' | DeviceDTO['type']
type DeviceStatusFilter = 'all' | DeviceDTO['status']

// ---- 响应式状态 ----

const devices = ref<DeviceDTO[]>([])
const connectConfig = ref<DeviceConnectConfigDTO | null>(null)
const typeFilter = ref<DeviceFilterType>('all')
const statusFilter = ref<DeviceStatusFilter>('all')
const keyword = ref('')

const unitStatusText = ref('未检查')
const unitConsistent = ref(false)
const errorMessage = ref('')
const dialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')

const showConnectDialog = ref(false)
const connectingDeviceId = ref<string | null>(null)
const connectProgressMessage = ref('')
let connectTimeoutTimer: ReturnType<typeof setTimeout> | null = null

// 当前编辑的设备（用于 edit 模式传入 DeviceFormDialog）
const editingDevice = ref<DeviceDTO | null>(null)

// ---- 轮询与 SSE 事件 ----

const {
  autoRefresh,
  lastRefreshAt,
  refreshAll
} = useDevicePolling({
  onRefreshed: (data) => {
    devices.value = data.devices
    connectConfig.value = data.config
    unitConsistent.value = data.unitConsistent
    unitStatusText.value = data.unitStatusText
  },
  onDeviceStatusChanged: (data) => {
    const index = devices.value.findIndex((item) => item.id === data.id)
    if (index < 0) {
      void refreshAll()
      return
    }
    const current = devices.value[index]
    const nextStatus = data.status ?? current.status
    let nextReason = current.lastErrorReason
    let nextTime = current.lastErrorAt
    if (nextStatus === 'connected' || nextStatus === 'disconnected') {
      nextReason = ''
      nextTime = undefined
    }
    if (typeof data.errorReason === 'string') nextReason = data.errorReason
    if (typeof data.lastErrorAt === 'string') nextTime = data.lastErrorAt
    devices.value.splice(index, 1, {
      ...current,
      status: nextStatus,
      lastErrorReason: nextReason,
      lastErrorAt: nextTime
    })
  },
  onConnectProgress: (_deviceId, msg) => {
    connectProgressMessage.value = msg
  },
  onError: (err) => {
    errorMessage.value = err.message
  }
})

// ---- 计算属性 ----

const existingIds = computed(() => devices.value.map(d => d.id))

const lastRefreshText = computed(() => {
  if (!lastRefreshAt.value) return '--'
  return lastRefreshAt.value.toLocaleTimeString()
})

const measureCount = computed(() => devices.value.filter((item) => item.type === 'measure').length)
const pressureCount = computed(() => devices.value.filter((item) => item.type === 'pressure').length)
const connectedCount = computed(() => devices.value.filter((item) => item.status === 'connected').length)
const errorCount = computed(() => devices.value.filter((item) => item.status === 'error').length)

const normalizedKeyword = computed(() => keyword.value.trim().toLowerCase())

const visibleDevices = computed(() => {
  return devices.value.filter((item) => {
    if (typeFilter.value !== 'all' && item.type !== typeFilter.value) return false
    if (statusFilter.value !== 'all' && item.status !== statusFilter.value) return false
    if (!normalizedKeyword.value) return true
    const fields = [item.id, item.name, item.model, item.host]
      .map((field) => field.toLowerCase())
      .join(' ')
    return fields.includes(normalizedKeyword.value)
  })
})

const connectPolicyText = computed(() => {
  if (!connectConfig.value) return '--'
  const cfg = connectConfig.value
  return `连接超时 ${cfg.connectAttemptTimeoutMs}ms，重试 ${cfg.connectMaxAttempts} 次`
})

const disconnectPolicyText = computed(() => {
  if (!connectConfig.value) return '--'
  const cfg = connectConfig.value
  return `断开超时 ${cfg.disconnectAttemptTimeoutMs}ms，重试 ${cfg.disconnectMaxAttempts} 次`
})

// ---- 方法 ----

// ---- 设备表单操作 ----

function openCreateDialog() {
  dialogMode.value = 'create'
  editingDevice.value = null
  dialogVisible.value = true
}

function openEditDialog(device: DeviceDTO) {
  dialogMode.value = 'edit'
  editingDevice.value = device
  dialogVisible.value = true
}

async function handleFormSave(device: DeviceDTO) {
  try {
    await upsertDevice({
      id: device.id,
      name: device.name,
      type: device.type,
      model: device.model,
      host: device.host,
      port: device.port,
      localAddr: device.localAddr || undefined,
      status: device.status,
      channels: device.channels
    })
    dialogVisible.value = false
    await refreshAll()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '保存设备失败'
  }
}

// ---- 设备连接操作 ----

async function toggleConnection(device: DeviceDTO) {
  errorMessage.value = ''

  if (device.status === 'connecting') {
    errorMessage.value = '设备正在连接中，请稍候'
    return
  }

  try {
    if (device.status === 'connected') {
      if (device.type === 'pressure') {
        await multipressUnregister(device.id)
      } else {
        await disconnectDevice(device.id)
      }
    } else {
      device.status = 'connecting'
      connectingDeviceId.value = device.id
      connectProgressMessage.value = '准备连接...'
      showConnectDialog.value = true

      // 前端超时兜底
      connectTimeoutTimer = setTimeout(() => {
        if (showConnectDialog.value) {
          showConnectDialog.value = false
          connectingDeviceId.value = null
          errorMessage.value = '连接超时，请检查设备网络和地址'
        }
      }, 12000)

      if (device.type === 'pressure') {
        await multipressRegister(device.id)
      } else {
        const result = await connectDevice(device.id)
        if (result.status === 'error') {
          errorMessage.value = result.lastErrorReason || '连接失败，请检查设备地址和网络'
        }
      }
    }
    await refreshAll()
  } catch (error) {
    device.status = 'disconnected'
    errorMessage.value = error instanceof Error ? error.message : '切换连接状态失败'
  } finally {
    showConnectDialog.value = false
    connectingDeviceId.value = null
    if (connectTimeoutTimer) {
      clearTimeout(connectTimeoutTimer)
      connectTimeoutTimer = null
    }
  }
}

async function handleDeleteDevice(device: DeviceDTO) {
  errorMessage.value = ''
  try {
    await ElMessageBox.confirm(
      `确定要删除设备「${device.name || device.id}」吗？此操作不可撤销。`,
      '确认删除',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' }
    )
    await deleteDevice(device.id)
    await refreshAll()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      errorMessage.value = error instanceof Error ? error.message : '删除设备失败'
    }
  }
}

function resetFilters() {
  typeFilter.value = 'all'
  statusFilter.value = 'all'
  keyword.value = ''
}
</script>

<style scoped lang="scss">
// ---- 面板容器 ----

.device-panel {
  background: #ffffff;
  border: 1px solid $slate-200;
  border-radius: 12px;
  padding: 16px;
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  font-family: $font-sans;
}

.panel-header {
  align-items: flex-start;
  display: flex;
  gap: 12px;
  justify-content: space-between;
  margin-bottom: 16px;
  flex-shrink: 0;
}

.panel-header h2 {
  color: $slate-800;
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.panel-header p {
  color: $slate-500;
  margin: 4px 0 0;
  font-size: 12px;
}

.header-actions {
  display: flex;
  gap: 8px;
}

// ---- 按钮 ----

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border: 1px solid $slate-200;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  background: transparent;
  font-family: $font-sans;

  .el-icon {
    font-size: 14px;
  }
}

.btn-primary {
  background: linear-gradient(135deg, $mint, $mint-dark);
  color: #ffffff;
  border-color: transparent;

  &:hover {
    background: linear-gradient(135deg, #34d399, $mint);
    box-shadow: 0 4px 12px rgba(16, 185, 129, 0.25);
  }
}

// ---- 设备卡片列表 ----

.device-board {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(2, 1fr);
  flex: 1;
  min-height: 0;
  overflow: auto;
  align-content: start;
  padding-right: 4px;

  &::-webkit-scrollbar { width: 4px; }
  &::-webkit-scrollbar-thumb { background: $slate-300; border-radius: 4px; }
  &::-webkit-scrollbar-track { background: transparent; }
}

.empty {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: $slate-400;
  padding: 40px 0;

  .el-icon {
    font-size: 18px;
  }
}

// ---- 错误横幅 ----

.error-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #dc2626;
  background: rgba(239, 68, 68, 0.06);
  border: 1px solid rgba(239, 68, 68, 0.15);
  border-radius: 8px;
  padding: 10px 14px;
  margin-top: 12px;
  flex-shrink: 0;
  font-weight: 500;
  font-size: 13px;

  .el-icon {
    font-size: 18px;
  }
}

// ---- 响应式 ----

@media (max-width: 900px) {
  .panel-header {
    flex-direction: column;
  }

  .policy-strip {
    flex-wrap: wrap;
    gap: 8px;
  }

  .filter-bar {
    flex-wrap: wrap;
  }

  .keyword-field {
    flex: 1 1 100%;
  }

  .device-board {
    grid-template-columns: 1fr;
  }
}
</style>