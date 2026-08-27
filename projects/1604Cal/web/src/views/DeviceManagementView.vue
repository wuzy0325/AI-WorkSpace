<template>
  <PageLayout>
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
          设备管理
        </h1>
        <span class="header-sub">配置和管理打压设备与计量设备</span>
      </div>
    </header>

    <div class="content-wrap">
      <DeviceManagementPanel class="content-panel" />
    </div>
  </PageLayout>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import PageLayout from '@/components/common/PageLayout.vue'
import DeviceManagementPanel from '@/components/device/DeviceManagementPanel.vue'

const router = useRouter()

function goBack(): void {
  router.push('/')
}
</script>

<style scoped lang="scss">
$font-sans: 'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
$mint: #10b981;
$slate-50: #f9fafb;
$slate-200: #e5e7eb;
$slate-400: #9ca3af;
$slate-500: #6b7280;
$slate-800: #1f2937;
$slate-900: #111827;

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

/* ═══ 内容区域 ═══ */
.content-wrap {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  padding: 20px 24px;
}

.content-panel {
  width: 100%;
  height: 100%;
}
</style>
