<template>
  <div
    class="pressure-card"
    :class="state.status"
  >
    <!-- 头部：设备名 + 状态 + 注销 -->
    <div class="card-head">
      <div class="head-info">
        <span class="device-name">{{ metadata?.name || state.deviceId }}</span>
        <span class="device-model">{{ metadata?.model || '' }}</span>
      </div>
      <div class="head-actions">
        <span
          class="status-dot"
          :class="state.status"
        />
        <span class="status-label">{{ statusLabel }}</span>
        <el-button
          size="small"
          type="danger"
          plain
          @click="$emit('unregister', state.deviceId)"
        >
          注销
        </el-button>
      </div>
    </div>

    <!-- 压力显示 -->
    <div class="pressure-display">
      <span class="pressure-value">{{ pressureDisplay }}</span>
      <span class="pressure-unit">{{ state.unit || 'kPa' }}</span>
      <span
        v-if="state.status === 'pressurizing'"
        class="stable-indicator"
        :class="{ stable: state.stable }"
      >
        {{ state.stable ? '已稳定' : '稳定中...' }}
      </span>
    </div>

    <!-- 目标压力输入 -->
    <div class="pressure-input-row">
      <el-input-number
        v-model="targetInput"
        :min="0"
        :step="1"
        :precision="2"
        size="small"
        controls-position="right"
        class="target-input"
      />
      <el-select
        v-model="unitSelect"
        size="small"
        class="unit-select"
        @change="handleUnitChange"
      >
        <el-option
          label="kPa (千帕)"
          value="kPa"
        />
        <el-option
          label="MPa (兆帕)"
          value="MPa"
        />
        <el-option
          label="Pa (帕)"
          value="Pa"
        />
        <el-option
          label="bar (巴)"
          value="bar"
        />
        <el-option
          label="mbar (毫巴)"
          value="mbar"
        />
        <el-option
          label="psi (磅/平方英寸)"
          value="psi"
        />
        <el-option
          label="kgf/cm²"
          value="kgf/cm2"
        />
        <el-option
          label="mmHg (毫米汞柱)"
          value="mmHg"
        />
        <el-option
          label="atm (标准大气压)"
          value="atm"
        />
      </el-select>
    </div>

    <!-- 操作按钮 -->
    <div class="action-row">
      <el-button
        size="small"
        type="primary"
        :disabled="state.status === 'error'"
        @click="handleSetPressure"
      >
        开始打压
      </el-button>
      <el-button
        size="small"
        type="danger"
        plain
        @click="$emit('stop', state.deviceId)"
      >
        停止
      </el-button>
      <el-button
        size="small"
        type="warning"
        plain
        @click="$emit('exhaust', state.deviceId)"
      >
        排空
      </el-button>
    </div>

    <!-- 状态信息 -->
    <div
      v-if="state.errorMessage"
      class="error-bar"
    >
      {{ state.errorMessage }}
    </div>
    <div
      v-if="state.status === 'exhausting'"
      class="info-bar"
    >
      正在排空压力...
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { MultiPressDeviceState } from '@/types/multipress'
import type { DeviceMeta } from '@/stores/multipress'

const props = defineProps<{
  state: MultiPressDeviceState
  metadata?: DeviceMeta
}>()

const emit = defineEmits<{
  'set-pressure': [deviceId: string, target: number]
  stop: [deviceId: string]
  exhaust: [deviceId: string]
  unregister: [deviceId: string]
  'set-unit': [deviceId: string, unit: string]
}>()

const targetInput = ref(props.state.targetPressure || 0)
const unitSelect = ref(props.state.unit || 'kPa')

watch(
  () => props.state.targetPressure,
  (v) => {
    targetInput.value = v || 0
  }
)

watch(
  () => props.state.unit,
  (v) => {
    if (v) unitSelect.value = v
  }
)

const pressureDisplay = computed(() => {
  const p = props.state.currentPressure
  return typeof p === 'number' ? p.toFixed(3) : '--'
})

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    idle: '空闲',
    pressurizing: '打压中',
    exhausting: '排空中',
    error: '异常'
  }
  return map[props.state.status] || props.state.status
})

function handleSetPressure() {
  emit('set-pressure', props.state.deviceId, targetInput.value)
}

function handleUnitChange(unit: string) {
  emit('set-unit', props.state.deviceId, unit)
}
</script>

<style scoped lang="scss">
.pressure-card {
  background: #ffffff;
  border: 1px solid $slate-200;
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: all 0.2s ease;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  font-family: $font-sans;

  &:hover {
    border-color: $slate-300;
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05);
  }

  &.pressurizing {
    border-top: 1px solid $mint;
  }

  &.error {
    border-top: 1px solid $red;
  }

  &.exhausting {
    border-top: 1px solid $amber;
  }
}

.card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.head-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.device-name {
  color: $slate-800;
  font-size: 14px;
  font-weight: 600;
}

.device-model {
  color: $slate-400;
  font-size: 11px;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: $slate-300;

  &.idle {
    background: $slate-400;
  }

  &.pressurizing {
    background: $mint;
    box-shadow: 0 0 6px rgba(16, 185, 129, 0.4);
  }

  &.exhausting {
    background: $amber;
  }

  &.error {
    background: $red;
  }
}

.status-label {
  color: $slate-500;
  font-size: 11px;
  font-weight: 500;
}

.pressure-display {
  display: flex;
  align-items: baseline;
  gap: 6px;
  padding: 8px 0;
}

.pressure-value {
  color: $mint;
  font-size: 28px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  font-family: $font-mono;
}

.pressure-unit {
  color: $slate-500;
  font-size: 13px;
  font-weight: 500;
}

.stable-indicator {
  font-size: 11px;
  color: $slate-400;
  margin-left: auto;
  font-weight: 500;

  &.stable {
    color: $green;
  }
}

.pressure-input-row {
  display: flex;
  gap: 8px;

  .target-input {
    flex: 1;
  }

  .unit-select {
    width: 90px;
  }
}

.action-row {
  display: flex;
  gap: 6px;
}

.error-bar {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 6px;
  padding: 6px 10px;
  color: #dc2626;
  font-size: 11px;
  font-weight: 500;
}

.info-bar {
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.2);
  border-radius: 6px;
  padding: 6px 10px;
  color: #d97706;
  font-size: 11px;
  font-weight: 500;
}
</style>
