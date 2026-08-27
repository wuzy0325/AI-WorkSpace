<template>
  <PageLayout>
    <!-- ═══ 仪表盘头部 ═══ -->
    <header class="instrument-header">
      <div class="header-nav">
        <button
          class="back-btn"
          @click="goBack"
        >
          <el-icon><ArrowLeft /></el-icon>
        </button>
      </div>
      <div class="header-identity">
        <h1 class="header-title">
          多设备打压控制
        </h1>
        <span class="header-sub">并发控制多台打压设备</span>
      </div>
      <div class="header-actions">
        <button
          class="danger-btn"
          :disabled="store.registeredCount === 0"
          @click="handleStopAll"
        >
          全部停止
        </button>
      </div>
    </header>

    <!-- 统计栏 -->
    <div class="stats-bar">
      <StatCard
        label="注册设备"
        :value="store.registeredCount"
      />
      <StatCard
        label="打压中"
        :value="store.pressurizingCount"
        color="#f59e0b"
      />
    </div>

    <!-- ═══ 内容区域 ═══ -->
    <div class="content-scroll">
      <!-- 未注册打压设备 -->
      <section
        v-if="store.availableDevices.length > 0"
        class="content-section"
      >
        <h3 class="section-title">
          可用打压设备
        </h3>
        <div class="available-grid">
          <div
            v-for="dev in store.availableDevices"
            :key="dev.id"
            class="available-card"
          >
            <div class="available-info">
              <span class="available-name">{{ dev.name }}</span>
              <span class="available-detail">{{ dev.host }}:{{ dev.port }}</span>
            </div>
            <button
              class="reg-btn"
              @click="handleRegister(dev.id)"
            >
              注册
            </button>
          </div>
        </div>
      </section>

      <!-- 已注册设备卡片网格 -->
      <section
        v-if="store.registeredDevices.length > 0"
        class="content-section"
      >
        <h3 class="section-title">
          已注册设备
        </h3>
        <div class="registered-grid">
          <PressureControlCard
            v-for="devState in store.registeredDevices"
            :key="devState.deviceId"
            :state="devState"
            :metadata="store.getMeta(devState.deviceId)"
            @set-pressure="handleSetPressure"
            @stop="handleStop"
            @exhaust="handleExhaust"
            @unregister="handleUnregister"
            @set-unit="handleSetUnit"
          />
        </div>
      </section>

      <!-- 空状态 -->
      <section
        v-if="store.registeredDevices.length === 0 && store.availableDevices.length === 0"
        class="empty-state"
      >
        <p class="empty-text">
          暂无打压设备
        </p>
      </section>
    </div>
  </PageLayout>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useMultiPressStore } from '@/stores/multipress'
import { useMultiPressSync } from '@/composables/useMultiPressSync'
import PageLayout from '@/components/common/PageLayout.vue'
import StatCard from '@/components/common/StatCard.vue'
import PressureControlCard from './PressureControlCard.vue'

const router = useRouter()
const store = useMultiPressStore()
useMultiPressSync()

function goBack(): void { router.push('/') }

async function handleRegister(deviceId: string): Promise<void> {
  await store.registerDevice(deviceId)
}

async function handleSetPressure(deviceId: string, pressure: number): Promise<void> {
  await store.setPressure(deviceId, pressure)
}

async function handleStop(deviceId: string): Promise<void> {
  await store.stopDevice(deviceId)
}

async function handleExhaust(deviceId: string): Promise<void> {
  await store.exhaustDevice(deviceId)
}

async function handleUnregister(deviceId: string): Promise<void> {
  await store.unregisterDevice(deviceId)
}

async function handleSetUnit(deviceId: string, unit: string): Promise<void> {
  try {
    await store.setUnit(deviceId, unit)
    ElMessage.success(`打压单位已切换为 ${unit}`)
  } catch {
    ElMessage.error('设置打压单位失败')
  }
}

async function handleStopAll(): Promise<void> {
  await store.stopAll()
}
</script>

<style scoped lang="scss">
$font-sans: 'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
$font-mono: 'JetBrains Mono', 'Fira Code', 'SF Mono', Consolas, monospace;
$mint: #10b981;
$slate-50: #f9fafb;
$slate-100: #f3f4f6;
$slate-200: #e5e7eb;
$slate-300: #d1d5db;
$slate-400: #9ca3af;
$slate-500: #6b7280;
$slate-600: #4b5563;
$slate-700: #374151;
$slate-800: #1f2937;
$slate-900: #111827;
$red: #ef4444;

/* ═══ 仪表盘头部 ═══ */
.instrument-header {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
  height: 56px;
  padding: 0 24px;
  background: $slate-50;
  border-bottom: 1px solid $slate-200;
  font-family: $font-sans;
}

.header-nav { display: flex; align-items: center; }

.back-btn {
  width: 32px; height: 32px;
  background: #fff;
  border: 1px solid $slate-200;
  border-radius: 8px;
  color: $slate-500;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  transition: background 0.15s ease, color 0.15s ease, border-color 0.15s ease;

  &:hover {
    background: #fff;
    color: $mint;
    border-color: $mint;
  }
}

.header-identity { display: flex; align-items: center; gap: 12px; }

.header-title {
  font-size: 20px;
  font-weight: 600;
  color: $slate-800;
  margin: 0;
  font-family: $font-sans;
}

.header-sub {
  font-size: 12px;
  color: $slate-400;
  font-weight: 400;
}

.header-actions {
  margin-left: auto;
  display: flex;
  gap: 10px;
}

.danger-btn {
  height: 30px;
  padding: 0 14px;
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: 6px;
  background: rgba(239, 68, 68, 0.1);
  color: #f87171;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  font-family: $font-sans;
  transition: all 0.15s ease;

  &:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.2);
    border-color: rgba(239, 68, 68, 0.5);
  }

  &:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
}

/* ═══ 统计栏 ═══ */
.stats-bar {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
  padding: 12px 24px 0;
  align-items: center;
}

/* ═══ 内容滚动区 ═══ */
.content-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 16px 24px 24px;
}

.content-section {
  margin-bottom: 24px;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: $slate-700;
  margin: 0 0 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid $slate-200;
  font-family: $font-sans;
  position: relative;
  padding-left: 10px;

  &::before {
    content: '';
    position: absolute;
    left: 0;
    top: 50%;
    transform: translateY(-50%);
    width: 1px;
    height: 14px;
    background: $mint;
    border-radius: 0;
  }
}

/* 可用设备 */
.available-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 10px;
}

.available-card {
  background: #ffffff;
  border: 1px solid $slate-200;
  border-radius: 10px;
  padding: 14px 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  transition: all 0.2s ease;

  &:hover {
    border-color: rgba($mint, 0.3);
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.07);
  }
}

.available-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.available-name {
  font-size: 14px;
  font-weight: 500;
  color: $slate-700;
  font-family: $font-sans;
}

.available-detail {
  font-size: 11px;
  color: $slate-400;
  font-family: $font-mono;
}

.reg-btn {
  height: 28px;
  padding: 0 12px;
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: 6px;
  background: rgba(16, 185, 129, 0.08);
  color: $mint;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  font-family: $font-sans;
  transition: all 0.15s ease;

  &:hover {
    background: rgba(16, 185, 129, 0.15);
    border-color: rgba(16, 185, 129, 0.5);
  }
}

/* 已注册设备 */
.registered-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

/* 空状态 */
.empty-state {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-text {
  font-size: 14px;
  color: $slate-400;
  font-family: $font-sans;
}

/* 响应式 */
@media (max-width: 768px) {
  .instrument-header {
    height: auto; flex-wrap: wrap; padding: 10px 16px;
  }
  .header-sub { display: none; }
  .stats-bar { flex-direction: column; padding: 12px 16px 0; }
  .content-scroll { padding: 12px 16px 16px; }
  .available-grid, .registered-grid { grid-template-columns: 1fr; }
}
</style>
