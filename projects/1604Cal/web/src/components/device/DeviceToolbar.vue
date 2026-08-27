<template>
  <div class="device-toolbar">
    <section class="status-strip">
      <div class="status-highlight">
        <div
          class="status-item"
          :class="{ 'has-error': errorCount > 0 }"
        >
          <span
            class="status-dot"
            :class="errorCount > 0 ? 'dot-danger' : 'dot-success'"
          />
          <strong
            class="status-value"
            :class="errorCount > 0 ? 'danger' : 'success'"
          >{{ errorCount }}</strong>
          <span class="status-label">异常</span>
        </div>
        <div class="status-item">
          <span class="status-dot dot-success" />
          <strong class="status-value success">{{ connectedCount }}</strong>
          <span class="status-label">在线</span>
        </div>
        <div class="status-item">
          <span
            class="status-dot"
            :class="unitConsistent ? 'dot-success' : 'dot-warning'"
          />
          <strong :class="['status-value', unitConsistent ? 'success' : 'warning']">{{ unitStatusText }}</strong>
          <span class="status-label">单位</span>
        </div>
      </div>
      <div class="status-aux">
        <span>共 <strong>{{ totalCount }}</strong> 台</span>
        <span class="aux-divider" />
        <span>计量 <strong>{{ measureCount }}</strong></span>
        <span>打压 <strong>{{ pressureCount }}</strong></span>
      </div>
    </section>

    <section class="policy-strip">
      <div class="policy-item">
        <el-icon><Link /></el-icon>
        <span data-test="connect-policy">{{ connectPolicyText }}</span>
      </div>
      <div class="policy-item">
        <el-icon><Close /></el-icon>
        <span data-test="disconnect-policy">{{ disconnectPolicyText }}</span>
      </div>
      <div class="policy-item">
        <el-icon><Timer /></el-icon>
        <span>最后刷新：{{ lastRefreshText }}</span>
      </div>
      <label class="auto-refresh">
        <input
          :checked="autoRefresh"
          type="checkbox"
          @change="$emit('update:autoRefresh', ($event.target as HTMLInputElement).checked)"
        >
        <span>自动刷新（3秒）</span>
      </label>
    </section>

    <section class="filter-bar">
      <label>
        设备类型
        <select
          :value="typeFilter"
          @change="$emit('update:typeFilter', ($event.target as HTMLSelectElement).value)"
        >
          <option value="all">全部</option>
          <option value="measure">计量设备</option>
          <option value="pressure">打压设备</option>
        </select>
      </label>

      <label>
        连接状态
        <select
          :value="statusFilter"
          @change="$emit('update:statusFilter', ($event.target as HTMLSelectElement).value)"
        >
          <option value="all">全部</option>
          <option value="connected">已连接</option>
          <option value="connecting">连接中</option>
          <option value="disconnected">未连接</option>
          <option value="error">异常</option>
        </select>
      </label>

      <label class="keyword-field">
        检索
        <input
          :value="keyword"
          type="text"
          placeholder="输入设备ID/名称/型号/IP"
          @input="$emit('update:keyword', ($event.target as HTMLInputElement).value)"
        >
      </label>

      <button
        type="button"
        class="btn btn-ghost"
        @click="$emit('resetFilters')"
      >
        <el-icon><RefreshRight /></el-icon>
        重置筛选
      </button>
    </section>
  </div>
</template>

<script setup lang="ts">
import { Link, Close, Timer, RefreshRight } from '@element-plus/icons-vue'

defineProps<{
  // 状态数据
  errorCount: number
  connectedCount: number
  totalCount: number
  measureCount: number
  pressureCount: number
  unitStatusText: string
  unitConsistent: boolean
  // 策略数据
  connectPolicyText: string
  disconnectPolicyText: string
  lastRefreshText: string
  autoRefresh: boolean
  // 筛选数据
  typeFilter: string
  statusFilter: string
  keyword: string
}>()

defineEmits<{
  (e: 'update:autoRefresh', value: boolean): void
  (e: 'update:typeFilter', value: string): void
  (e: 'update:statusFilter', value: string): void
  (e: 'update:keyword', value: string): void
  (e: 'resetFilters'): void
}>()
</script>

<style scoped lang="scss">
// ---- 状态条 ----

.status-strip {
  background: $slate-50;
  border: 1px solid $slate-200;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 12px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.status-highlight {
  display: flex;
  align-items: center;
  gap: 20px;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 6px;

  &.has-error {
    .status-value { font-size: 18px; }
  }
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;

  &.dot-success { background: $green; }
  &.dot-danger { background: $red; box-shadow: 0 0 4px rgba(239, 68, 68, 0.4); }
  &.dot-warning { background: $amber; }
}

.status-value {
  font-size: 15px;
  font-weight: 600;
  font-family: $font-mono;
  color: $slate-800;

  &.success { color: $green; }
  &.danger { color: $red; }
  &.warning { color: $amber; }
}

.status-label {
  font-size: 12px;
  color: $slate-500;
  font-weight: 500;
}

.status-aux {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: $slate-400;

  strong {
    color: $slate-600;
    font-weight: 600;
    font-family: $font-mono;
  }
}

.aux-divider {
  width: 1px;
  height: 12px;
  background: $slate-200;
}

// ---- 策略条 ----

.policy-strip {
  background: $slate-50;
  border: 1px solid $slate-200;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding: 8px 12px;
  flex-wrap: wrap;
  flex-shrink: 0;
}

.policy-item {
  display: flex;
  align-items: center;
  gap: 6px;
  color: $slate-500;
  font-size: 12px;

  .el-icon {
    color: $mint;
    font-size: 13px;
  }
}

.auto-refresh {
  display: flex;
  align-items: center;
  gap: 6px;
  color: $slate-500;
  font-size: 12px;
  margin-left: auto;
  cursor: pointer;
  font-weight: 500;

  input[type="checkbox"] {
    width: 14px;
    height: 14px;
    accent-color: $mint;
  }
}

// ---- 筛选条 ----

.filter-bar {
  background: $slate-50;
  border: 1px solid $slate-200;
  border-radius: 8px;
  display: flex;
  align-items: flex-end;
  gap: 12px;
  margin-bottom: 12px;
  padding: 10px 12px;
  flex-shrink: 0;
}

.filter-bar label {
  color: $slate-500;
  display: flex;
  flex-direction: column;
  font-size: 12px;
  gap: 4px;
  flex: 1;
  font-weight: 500;
}

.keyword-field {
  flex: 2;
}

.filter-bar select,
.filter-bar input {
  background: #ffffff;
  border: 1px solid $slate-300;
  border-radius: 6px;
  color: $slate-700;
  padding: 6px 8px;
  font-size: 12px;
  font-family: $font-sans;
  outline: none;

  &:focus {
    border-color: $mint;
    box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.12);
  }
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

.btn-ghost {
  color: $slate-600;
  background: $slate-50;

  &:hover {
    background: $slate-100;
    color: $slate-800;
    border-color: $slate-300;
  }
}
</style>