<template>
  <div class="channel-matrix">
    <div class="matrix-header">
      <h4>通道选择</h4>
      <div class="actions">
        <span class="count">已选: {{ selectedCount }}/16</span>
        <el-button
          type="primary"
          link
          size="small"
          @click="selectAll"
        >
          全选
        </el-button>
        <el-button
          type="danger"
          link
          size="small"
          @click="clearAll"
        >
          清空
        </el-button>
      </div>
    </div>

    <div class="matrix-grid">
      <div
        v-for="ch in allChannels"
        :key="ch"
        class="channel-item"
        :class="{ selected: isSelected(ch) }"
        role="checkbox"
        :aria-checked="isSelected(ch)"
        tabindex="0"
        @click="toggleChannel(ch)"
        @keydown.enter="toggleChannel(ch)"
        @keydown.space.prevent="toggleChannel(ch)"
      >
        <span
          class="check-box"
          aria-hidden="true"
        >
          <span
            v-if="isSelected(ch)"
            class="check-mark"
          >✓</span>
        </span>
        <span class="channel-label">CH{{ ch }}</span>
      </div>
    </div>

    <div
      v-if="selectedCount === 0"
      class="warning"
      role="alert"
    >
      <el-icon><Warning /></el-icon>
      <span>请至少选择一个通道</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Warning } from '@element-plus/icons-vue'

const props = defineProps<{
  selectedChannels?: number[]
}>()

const emit = defineEmits<{
  'update:selectedChannels': [channels: number[]]
}>()

const allChannels = Array.from({ length: 16 }, (_, i) => i + 1)

const selected = computed<number[]>(() => props.selectedChannels ?? [])

const selectedCount = computed(() => selected.value.length)

function isSelected(ch: number): boolean {
  return selected.value.includes(ch)
}

function toggleChannel(ch: number) {
  const channels = isSelected(ch)
    ? selected.value.filter(c => c !== ch)
    : [...selected.value, ch]
  emit('update:selectedChannels', channels)
}

function selectAll() {
  emit('update:selectedChannels', [...allChannels])
}

function clearAll() {
  emit('update:selectedChannels', [])
}
</script>

<style scoped lang="scss">
.channel-matrix {
  display: flex;
  flex-direction: column;
  gap: 12px;
  color: #1f2937;

  .matrix-header {
    display: flex;
    justify-content: space-between;
    align-items: center;

    h4 {
      color: #1f2937;
      margin: 0;
      font-size: 12px;
      font-weight: 500;
    }

    .actions {
      display: flex;
      align-items: center;
      gap: 8px;

      .count {
        color: #6b7280;
        font-size: 11px;
      }
    }
  }

  .matrix-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 8px;

    .channel-item {
      display: flex;
      align-items: center;
      gap: 6px;
      min-height: 34px;
      background: #f9fafb;
      border: 1px solid #e5e7eb;
      border-radius: 8px;
      padding: 7px 8px;
      cursor: pointer;
      transition: all 0.15s;
      user-select: none;

      &:hover {
        border-color: #10b981;
        background: #f0fdf4;
      }

      &.selected {
        background: #ecfdf5;
        border-color: #10b981;
      }

      .check-box {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 14px;
        height: 14px;
        flex: 0 0 14px;
        border: 1px solid #d1d5db;
        border-radius: 4px;
        background: #fff;
      }

      &.selected .check-box {
        border-color: #10b981;
        background: #10b981;
      }

      .check-mark {
        color: #fff;
        font-size: 10px;
        line-height: 1;
      }

      .channel-label {
        color: #1f2937;
        font-size: 12px;
        font-weight: 500;
        line-height: 1;
      }
    }
  }

  .warning {
    display: flex;
    align-items: center;
    gap: 6px;
    color: #d97706;
    font-size: 11px;
    padding: 6px 8px;
    background: #fffbeb;
    border-radius: 6px;
  }
}
</style>
