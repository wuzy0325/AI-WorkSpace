<template>
  <article class="device-card">
    <div class="card-top">
      <div class="title-block">
        <strong>{{ device.name || device.id }}</strong>
        <span :class="['type-badge', device.type]">
          {{ typeLabel }}
        </span>
      </div>
      <span :class="['status-badge', `status-${device.status}`]">
        <el-icon v-if="device.status === 'connected'"><CircleCheck /></el-icon>
        <el-icon v-else-if="device.status === 'error'"><CircleClose /></el-icon>
        <el-icon v-else-if="device.status === 'connecting'"><Loading /></el-icon>
        <el-icon v-else><Remove /></el-icon>
        {{ statusLabel(device.status) }}
      </span>
    </div>

    <div class="card-grid">
      <div class="info-item">
        <span class="info-label">设备ID</span>
        <span class="info-value">{{ device.id }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">型号</span>
        <span class="info-value">{{ device.model || '-' }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">地址</span>
        <span class="info-value">{{ device.host }}:{{ device.port }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">单位</span>
        <span class="info-value">{{ device.unit || '-' }}</span>
      </div>
    </div>

    <div
      v-if="device.lastErrorReason || device.lastErrorAt"
      class="error-section"
    >
      <p
        v-if="device.lastErrorReason"
        class="meta-error"
      >
        <el-icon><Warning /></el-icon>
        错误原因：{{ device.lastErrorReason }}
      </p>
      <p
        v-if="device.lastErrorAt"
        class="meta-error"
      >
        <el-icon><Clock /></el-icon>
        最近错误时间：{{ formatErrorTime(device.lastErrorAt) }}
      </p>
    </div>

    <div class="card-actions">
      <button
        type="button"
        class="btn btn-ghost"
        @click="$emit('edit', device)"
      >
        <el-icon><Edit /></el-icon>
        编辑
      </button>
      <button
        type="button"
        :class="['btn', device.status === 'connected' ? 'btn-danger' : 'btn-success']"
        @click="$emit('toggle-connection', device)"
      >
        <el-icon v-if="device.status === 'connected'">
          <Close />
        </el-icon>
        <el-icon v-else>
          <Link />
        </el-icon>
        {{ device.status === 'connected' ? '断开' : '连接' }}
      </button>
      <button
        type="button"
        class="btn btn-delete"
        @click="$emit('delete', device)"
      >
        <el-icon><Delete /></el-icon>
        删除
      </button>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  CircleCheck,
  CircleClose,
  Loading,
  Remove,
  Warning,
  Clock,
  Edit,
  Close,
  Link,
  Delete
} from '@element-plus/icons-vue'
import type { DeviceDTO } from '@/types/device'

const props = defineProps<{
  device: DeviceDTO
}>()

defineEmits<{
  (e: 'edit', device: DeviceDTO): void
  (e: 'toggle-connection', device: DeviceDTO): void
  (e: 'delete', device: DeviceDTO): void
}>()

// ---- 工具函数 ----

function statusLabel(status: DeviceDTO['status']): string {
  switch (status) {
    case 'connected': return '已连接'
    case 'connecting': return '连接中'
    case 'error': return '异常'
    default: return '未连接'
  }
}

const typeLabel = computed(() => {
  return props.device.type === 'measure' ? '计量' : '打压'
})

function formatErrorTime(value: string): string {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString()
}
</script>

<style scoped lang="scss">
.device-card {
  background: #ffffff;
  border: 1px solid $slate-200;
  border-radius: 10px;
  padding: 14px;
  transition: all 0.2s ease;

  &:hover {
    border-color: $slate-300;
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }
}

.card-top {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
  margin-bottom: 12px;
}

.title-block {
  align-items: center;
  display: flex;
  gap: 8px;
}

.title-block strong {
  color: $slate-800;
  font-size: 14px;
  font-weight: 600;
}

.type-badge {
  background: $slate-50;
  border-radius: 999px;
  font-size: 10px;
  font-weight: 600;
  padding: 2px 8px;
  border: 1px solid $slate-200;

  &.measure {
    color: $blue;
    background: rgba(59, 130, 246, 0.08);
    border-color: rgba(59, 130, 246, 0.15);
  }

  &.pressure {
    color: $amber;
    background: rgba(245, 158, 11, 0.08);
    border-color: rgba(245, 158, 11, 0.15);
  }
}

.card-grid {
  display: grid;
  gap: 10px;
  grid-template-columns: 1fr 1fr;
  margin-bottom: 12px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.info-label {
  color: $slate-400;
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0.05em;
}

.info-value {
  color: $slate-600;
  font-size: 12px;
  font-weight: 500;
}

.error-section {
  background: rgba(239, 68, 68, 0.06);
  border: 1px solid rgba(239, 68, 68, 0.15);
  border-radius: 6px;
  padding: 8px 10px;
  margin-bottom: 12px;
}

.meta-error {
  color: $red;
  font-size: 11px;
  margin: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  font-weight: 500;

  & + .meta-error {
    margin-top: 4px;
  }

  .el-icon {
    font-size: 12px;
  }
}

.status-badge {
  border-radius: 4px;
  font-size: 11px;
  font-weight: 500;
  padding: 3px 8px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  line-height: 1.5;

  .el-icon {
    font-size: 10px;
  }
}

.status-connected {
  background: rgba(16, 185, 129, 0.12);
  border: 1px solid rgba(16, 185, 129, 0.25);
  color: #059669;
}

.status-connecting {
  background: rgba(245, 158, 11, 0.12);
  border: 1px solid rgba(245, 158, 11, 0.25);
  color: #d97706;
}

.status-disconnected {
  background: rgba(107, 114, 128, 0.12);
  border: 1px solid rgba(107, 114, 128, 0.25);
  color: $slate-500;
}

.status-error {
  background: rgba(239, 68, 68, 0.12);
  border: 1px solid rgba(239, 68, 68, 0.25);
  color: #dc2626;
}

.card-actions {
  display: flex;
  justify-content: flex-end;
  gap: 6px;
}

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

.btn-success {
  background: $green;
  color: #ffffff;
  border-color: $green;

  &:hover {
    background: #16a34a;
  }
}

.btn-danger {
  background: $red;
  color: #fff;
  border-color: $red;

  &:hover {
    background: #dc2626;
  }
}

.btn-ghost {
  color: $slate-600;
  background: $slate-50;

  &:hover {
    background: $slate-100;
    color: $slate-800;
    border-color: $slate-300;
  }
}

.btn-delete {
  color: $red;
  border-color: rgba(239, 68, 68, 0.2);

  &:hover {
    background: rgba(239, 68, 68, 0.08);
    border-color: rgba(239, 68, 68, 0.35);
  }
}
</style>