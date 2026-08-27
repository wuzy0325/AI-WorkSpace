# 模块UI照搬实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将计量模块（1605MeassureApp）和标定模块（1604标定软件）的UI界面完整照搬至新Vue3项目，采用深色主题。

**Architecture:** 保持两个模块独立，各自维护自己的组件和状态。计量模块包含「计量工作台」和「多设备打压」两个页面，标定模块包含完整的6步标定流程界面。使用Element Plus组件库，Pinia状态管理。

**Tech Stack:** Vue 3 + TypeScript + Element Plus + Pinia + Vue Router + SCSS (深色主题)

---

## 前置检查

### Task 0: 检查当前项目结构

**Files:**
- Read: `web/package.json`
- Read: `web/src/main.ts`
- Read: `web/src/App.vue`

**Step 1: 确认项目已存在**
```bash
cd web && ls -la
```
Expected: 看到src/、package.json等Vue项目文件

**Step 2: 确认依赖已安装**
```bash
cat package.json | grep -A 5 '"dependencies"'
```
Expected: 看到vue、vue-router、element-plus、pinia

**Step 3: 确认能正常启动**
```bash
npm run dev
```
Expected: 开发服务器正常启动，无报错

---

## Phase 1: 基础设置（主题 + 路由 + 布局）

### Task 1: 创建深色主题CSS变量文件

**Files:**
- Create: `web/src/styles/variables.scss`

**Step 1: 创建样式目录**
```bash
mkdir -p web/src/styles
```

**Step 2: 写入CSS变量**
```scss
// web/src/styles/variables.scss
:root {
  /* 背景色 */
  --bg-primary: #1a1a2e;
  --bg-secondary: #16213e;
  --bg-tertiary: #0f3460;
  
  /* 强调色 */
  --accent-primary: #e94560;
  --accent-secondary: #533483;
  
  /* 文字 */
  --text-primary: #ffffff;
  --text-secondary: #a0a0a0;
  --text-muted: #666666;
  
  /* 状态色 */
  --status-success: #10b981;
  --status-warning: #f59e0b;
  --status-error: #ef4444;
  --status-info: #3b82f6;
  
  /* 边框 */
  --border-color: rgba(255, 255, 255, 0.1);
  
  /* 间距 */
  --spacing-xs: 4px;
  --spacing-sm: 8px;
  --spacing-md: 16px;
  --spacing-lg: 24px;
  --spacing-xl: 32px;
  
  /* 圆角 */
  --radius-sm: 4px;
  --radius-md: 8px;
  --radius-lg: 12px;
}

/* Element Plus 深色主题覆盖 */
.dark-theme {
  --el-bg-color: var(--bg-primary);
  --el-bg-color-overlay: var(--bg-secondary);
  --el-text-color-primary: var(--text-primary);
  --el-text-color-regular: var(--text-secondary);
  --el-border-color: var(--border-color);
  --el-fill-color: var(--bg-tertiary);
  --el-fill-color-light: var(--bg-secondary);
}
```

**Step 3: 在main.ts中引入**
```typescript
// web/src/main.ts
import './styles/variables.scss'
```

**Step 4: 验证样式生效**
```bash
npm run dev
```
检查浏览器开发者工具，确认CSS变量已加载

**Step 5: Commit**
```bash
git add web/src/styles/variables.scss web/src/main.ts
git commit -m "feat: add dark theme CSS variables"
```

---

### Task 2: 配置路由

**Files:**
- Modify: `web/src/router/index.ts`

**Step 1: 添加新路由**
```typescript
// web/src/router/index.ts
import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: HomeView
    },
    {
      path: '/measurement',
      name: 'measurement',
      component: () => import('../views/measurement/CalibrationView.vue')
    },
    {
      path: '/multi-pressure',
      name: 'multi-pressure',
      component: () => import('../views/measurement/PressureWorkbenchView.vue')
    },
    {
      path: '/calibration',
      name: 'calibration',
      component: () => import('../views/calibration/MainView.vue')
    }
  ]
})

export default router
```

**Step 2: 创建视图目录**
```bash
mkdir -p web/src/views/measurement
mkdir -p web/src/views/calibration
```

**Step 3: 创建占位视图文件**
```vue
<!-- web/src/views/HomeView.vue -->
<template>
  <div class="home-view">
    <h1>首页 - 模块选择</h1>
  </div>
</template>

<style scoped>
.home-view {
  padding: 20px;
  color: var(--text-primary);
}
</style>
```

```vue
<!-- web/src/views/measurement/CalibrationView.vue -->
<template>
  <div class="calibration-view">
    <h1>计量工作台</h1>
  </div>
</template>

<style scoped>
.calibration-view {
  padding: 20px;
  color: var(--text-primary);
}
</style>
```

```vue
<!-- web/src/views/measurement/PressureWorkbenchView.vue -->
<template>
  <div class="pressure-workbench">
    <h1>多设备打压</h1>
  </div>
</template>

<style scoped>
.pressure-workbench {
  padding: 20px;
  color: var(--text-primary);
}
</style>
```

```vue
<!-- web/src/views/calibration/MainView.vue -->
<template>
  <div class="main-view">
    <h1>标定工作台</h1>
  </div>
</template>

<style scoped>
.main-view {
  padding: 20px;
  color: var(--text-primary);
}
</style>
```

**Step 4: 验证路由**
```bash
npm run dev
```
访问以下URL，确认都能正常显示：
- http://localhost:5173/
- http://localhost:5173/measurement
- http://localhost:5173/multi-pressure
- http://localhost:5173/calibration

**Step 5: Commit**
```bash
git add web/src/router/index.ts web/src/views/
git commit -m "feat: add router configuration and view placeholders"
```

---

### Task 3: 创建基础布局组件

**Files:**
- Create: `web/src/components/common/Sidebar.vue`
- Create: `web/src/components/common/StatCard.vue`
- Create: `web/src/components/common/DeviceStatusBadge.vue`

**Step 1: 创建组件目录**
```bash
mkdir -p web/src/components/common
mkdir -p web/src/components/measurement
mkdir -p web/src/components/calibration
```

**Step 2: 创建Sidebar组件**
```vue
<!-- web/src/components/common/Sidebar.vue -->
<template>
  <aside class="sidebar">
    <div class="logo">
      <h2>1604校准系统</h2>
    </div>
    <nav class="nav-menu">
      <router-link to="/" class="nav-item" :class="{ active: $route.path === '/' }">
        <el-icon><HomeFilled /></el-icon>
        <span>首页</span>
      </router-link>
      <router-link to="/measurement" class="nav-item" :class="{ active: $route.path === '/measurement' }">
        <el-icon><Tools /></el-icon>
        <span>计量工作台</span>
      </router-link>
      <router-link to="/multi-pressure" class="nav-item" :class="{ active: $route.path === '/multi-pressure' }">
        <el-icon><CircleCheck /></el-icon>
        <span>多设备打压</span>
      </router-link>
      <router-link to="/calibration" class="nav-item" :class="{ active: $route.path === '/calibration' }">
        <el-icon><SetUp /></el-icon>
        <span>标定工作台</span>
      </router-link>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { HomeFilled, Tools, CircleCheck, SetUp } from '@element-plus/icons-vue'
</script>

<style scoped lang="scss">
.sidebar {
  width: 240px;
  height: 100vh;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-color);
  display: flex;
  flex-direction: column;
  
  .logo {
    padding: var(--spacing-lg);
    border-bottom: 1px solid var(--border-color);
    
    h2 {
      color: var(--text-primary);
      font-size: 18px;
      margin: 0;
    }
  }
  
  .nav-menu {
    flex: 1;
    padding: var(--spacing-md);
    
    .nav-item {
      display: flex;
      align-items: center;
      gap: var(--spacing-sm);
      padding: var(--spacing-sm) var(--spacing-md);
      color: var(--text-secondary);
      text-decoration: none;
      border-radius: var(--radius-sm);
      margin-bottom: var(--spacing-xs);
      transition: all 0.3s;
      
      &:hover {
        background: var(--bg-tertiary);
        color: var(--text-primary);
      }
      
      &.active {
        background: var(--accent-primary);
        color: var(--text-primary);
      }
      
      .el-icon {
        font-size: 18px;
      }
    }
  }
}
</style>
```

**Step 3: 创建StatCard组件**
```vue
<!-- web/src/components/common/StatCard.vue -->
<template>
  <div class="stat-card">
    <div class="label">{{ label }}</div>
    <div class="value" :style="{ color: color }">{{ value }}</div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: string | number
  color?: string
}>()
</script>

<style scoped lang="scss">
.stat-card {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  text-align: center;
  
  .label {
    color: var(--text-secondary);
    font-size: 12px;
    margin-bottom: var(--spacing-xs);
  }
  
  .value {
    color: var(--text-primary);
    font-size: 24px;
    font-weight: bold;
  }
}
</style>
```

**Step 4: 创建DeviceStatusBadge组件**
```vue
<!-- web/src/components/common/DeviceStatusBadge.vue -->
<template>
  <span class="status-badge" :class="status">
    {{ statusText }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: 'connected' | 'disconnected' | 'error'
}>()

const statusText = computed(() => {
  const map = {
    connected: '已连接',
    disconnected: '未连接',
    error: '错误'
  }
  return map[props.status]
})
</script>

<style scoped lang="scss">
.status-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-weight: 500;
  
  &.connected {
    background: rgba(16, 185, 129, 0.2);
    color: var(--status-success);
  }
  
  &.disconnected {
    background: rgba(160, 160, 160, 0.2);
    color: var(--text-secondary);
  }
  
  &.error {
    background: rgba(239, 68, 68, 0.2);
    color: var(--status-error);
  }
}
</style>
```

**Step 5: 更新App.vue使用布局**
```vue
<!-- web/src/App.vue -->
<template>
  <div class="app dark-theme">
    <Sidebar />
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
import Sidebar from './components/common/Sidebar.vue'
</script>

<style scoped lang="scss">
.app {
  display: flex;
  min-height: 100vh;
  background: var(--bg-primary);
  
  .main-content {
    flex: 1;
    overflow: auto;
  }
}
</style>
```

**Step 6: 验证组件**
```bash
npm run dev
```
确认侧边栏显示正常，导航切换正常

**Step 7: Commit**
```bash
git add web/src/components/common/
git add web/src/App.vue
git commit -m "feat: add sidebar, statcard, and status badge components"
```

---

## Phase 2: 首页实现

### Task 4: 实现HomeView（模块入口）

**Files:**
- Modify: `web/src/views/HomeView.vue`

**Step 1: 完整实现HomeView**
```vue
<!-- web/src/views/HomeView.vue -->
<template>
  <div class="home-view">
    <header class="page-header">
      <h1>欢迎使用1604校准系统</h1>
      <p class="subtitle">请选择要进入的模块</p>
    </header>
    
    <div class="module-cards">
      <router-link to="/measurement" class="module-card">
        <div class="card-icon">
          <el-icon><Tools /></el-icon>
        </div>
        <h3>计量工作台</h3>
        <p>单设备校准和数据采集</p>
        <ul class="features">
          <li>自动/手动控制模式</li>
          <li>16通道实时数据</li>
          <li>压力表自动生成</li>
        </ul>
      </router-link>
      
      <router-link to="/multi-pressure" class="module-card">
        <div class="card-icon">
          <el-icon><CircleCheck /></el-icon>
        </div>
        <h3>多设备打压</h3>
        <p>并行控制多个打压设备</p>
        <ul class="features">
          <li>多设备同时监控</li>
          <li>独立压力设定</li>
          <li>实时状态显示</li>
        </ul>
      </router-link>
      
      <router-link to="/calibration" class="module-card">
        <div class="card-icon">
          <el-icon><SetUp /></el-icon>
        </div>
        <h3>标定工作台</h3>
        <p>完整的1604标定流程</p>
        <ul class="features">
          <li>6步向导式操作</li>
          <li>通道选择矩阵</li>
          <li>数据拟合与导出</li>
        </ul>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Tools, CircleCheck, SetUp } from '@element-plus/icons-vue'
</script>

<style scoped lang="scss">
.home-view {
  padding: var(--spacing-xl);
  
  .page-header {
    margin-bottom: var(--spacing-xl);
    
    h1 {
      color: var(--text-primary);
      font-size: 28px;
      margin: 0 0 var(--spacing-sm) 0;
    }
    
    .subtitle {
      color: var(--text-secondary);
      font-size: 16px;
      margin: 0;
    }
  }
  
  .module-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: var(--spacing-lg);
    
    .module-card {
      background: var(--bg-secondary);
      border: 1px solid var(--border-color);
      border-radius: var(--radius-lg);
      padding: var(--spacing-xl);
      text-decoration: none;
      transition: all 0.3s;
      
      &:hover {
        border-color: var(--accent-primary);
        transform: translateY(-2px);
        box-shadow: 0 4px 12px rgba(233, 69, 96, 0.2);
      }
      
      .card-icon {
        width: 60px;
        height: 60px;
        background: linear-gradient(135deg, var(--accent-primary), var(--accent-secondary));
        border-radius: var(--radius-md);
        display: flex;
        align-items: center;
        justify-content: center;
        margin-bottom: var(--spacing-md);
        
        .el-icon {
          font-size: 28px;
          color: white;
        }
      }
      
      h3 {
        color: var(--text-primary);
        font-size: 20px;
        margin: 0 0 var(--spacing-sm) 0;
      }
      
      p {
        color: var(--text-secondary);
        margin: 0 0 var(--spacing-md) 0;
      }
      
      .features {
        list-style: none;
        padding: 0;
        margin: 0;
        
        li {
          color: var(--text-muted);
          font-size: 14px;
          padding: var(--spacing-xs) 0;
          padding-left: 16px;
          position: relative;
          
          &::before {
            content: '•';
            position: absolute;
            left: 0;
            color: var(--accent-primary);
          }
        }
      }
    }
  }
}
</style>
```

**Step 2: 验证首页**
```bash
npm run dev
```
访问 http://localhost:5173/，确认三个模块卡片显示正常，点击进入各模块正常

**Step 3: Commit**
```bash
git add web/src/views/HomeView.vue
git commit -m "feat: implement home view with module cards"
```

---

## Phase 3: 计量模块实现

### Task 5: 创建设备相关组件

**Files:**
- Create: `web/src/components/measurement/PressureDeviceCard.vue`
- Create: `web/src/components/measurement/MeasureDeviceCard.vue`
- Create: `web/src/components/measurement/DevicePanel.vue`

**Step 1: 创建PressureDeviceCard**
```vue
<!-- web/src/components/measurement/PressureDeviceCard.vue -->
<template>
  <div class="device-card">
    <div class="device-header">
      <span class="device-name">{{ device.name }}</span>
      <DeviceStatusBadge :status="device.status" />
    </div>
    <div class="device-info">
      <div class="info-row">
        <span class="label">IP:</span>
        <span class="value">{{ device.ip }}:{{ device.port }}</span>
      </div>
      <div class="info-row">
        <span class="label">当前压力:</span>
        <span class="value pressure">{{ device.currentPressure?.toFixed(2) || '--' }} {{ device.unit }}</span>
      </div>
    </div>
    <div class="device-actions">
      <el-button 
        :type="device.status === 'connected' ? 'danger' : 'primary'"
        size="small"
        @click="toggleConnection"
      >
        {{ device.status === 'connected' ? '断开' : '连接' }}
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import DeviceStatusBadge from '@/components/common/DeviceStatusBadge.vue'

interface PressureDevice {
  id: string
  name: string
  ip: string
  port: number
  status: 'connected' | 'disconnected'
  currentPressure?: number
  unit: string
}

const props = defineProps<{
  device: PressureDevice
}>()

const emit = defineEmits<{
  connect: [id: string]
  disconnect: [id: string]
}>()

const toggleConnection = () => {
  if (props.device.status === 'connected') {
    emit('disconnect', props.device.id)
  } else {
    emit('connect', props.device.id)
  }
}
</script>

<style scoped lang="scss">
.device-card {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
  
  .device-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-sm);
    
    .device-name {
      color: var(--text-primary);
      font-weight: 500;
    }
  }
  
  .device-info {
    margin-bottom: var(--spacing-sm);
    
    .info-row {
      display: flex;
      justify-content: space-between;
      margin-bottom: var(--spacing-xs);
      
      .label {
        color: var(--text-secondary);
        font-size: 12px;
      }
      
      .value {
        color: var(--text-primary);
        font-size: 12px;
        
        &.pressure {
          color: var(--accent-primary);
          font-weight: bold;
        }
      }
    }
  }
  
  .device-actions {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
```

**Step 2: 创建MeasureDeviceCard**
```vue
<!-- web/src/components/measurement/MeasureDeviceCard.vue -->
<template>
  <div class="device-card">
    <div class="device-header">
      <span class="device-name">{{ device.name }}</span>
      <DeviceStatusBadge :status="device.status" />
    </div>
    <div class="device-info">
      <div class="info-row">
        <span class="label">型号:</span>
        <span class="value">{{ device.model }}</span>
      </div>
      <div class="info-row">
        <span class="label">通道数:</span>
        <span class="value">{{ device.channels }}</span>
      </div>
    </div>
    <div class="device-actions">
      <el-button 
        :type="device.status === 'connected' ? 'danger' : 'primary'"
        size="small"
        @click="toggleConnection"
      >
        {{ device.status === 'connected' ? '断开' : '连接' }}
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import DeviceStatusBadge from '@/components/common/DeviceStatusBadge.vue'

interface MeasureDevice {
  id: string
  name: string
  model: string
  channels: number
  status: 'connected' | 'disconnected'
}

const props = defineProps<{
  device: MeasureDevice
}>()

const emit = defineEmits<{
  connect: [id: string]
  disconnect: [id: string]
}>()

const toggleConnection = () => {
  if (props.device.status === 'connected') {
    emit('disconnect', props.device.id)
  } else {
    emit('connect', props.device.id)
  }
}
</script>

<style scoped lang="scss">
.device-card {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
  
  .device-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-sm);
    
    .device-name {
      color: var(--text-primary);
      font-weight: 500;
    }
  }
  
  .device-info {
    margin-bottom: var(--spacing-sm);
    
    .info-row {
      display: flex;
      justify-content: space-between;
      margin-bottom: var(--spacing-xs);
      
      .label {
        color: var(--text-secondary);
        font-size: 12px;
      }
      
      .value {
        color: var(--text-primary);
        font-size: 12px;
      }
    }
  }
  
  .device-actions {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
```

**Step 3: 创建DevicePanel**
```vue
<!-- web/src/components/measurement/DevicePanel.vue -->
<template>
  <div class="device-panel">
    <div class="panel-header" @click="toggleCollapse">
      <span class="title">{{ title }}</span>
      <el-icon :class="{ 'is-collapsed': isCollapsed }"><ArrowDown /></el-icon>
    </div>
    <div v-show="!isCollapsed" class="panel-content">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ArrowDown } from '@element-plus/icons-vue'

defineProps<{
  title: string
}>()

const isCollapsed = ref(false)

const toggleCollapse = () => {
  isCollapsed.value = !isCollapsed.value
}
</script>

<style scoped lang="scss">
.device-panel {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  margin-bottom: var(--spacing-md);
  
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
    user-select: none;
    
    .title {
      color: var(--text-primary);
      font-weight: 500;
    }
    
    .el-icon {
      color: var(--text-secondary);
      transition: transform 0.3s;
      
      &.is-collapsed {
        transform: rotate(-90deg);
      }
    }
  }
  
  .panel-content {
    padding: 0 var(--spacing-md) var(--spacing-md);
  }
}
</style>
```

**Step 4: 创建计量模块Store**
```typescript
// web/src/stores/measurement/deviceStore.ts
import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface PressureDevice {
  id: string
  name: string
  ip: string
  port: number
  status: 'connected' | 'disconnected'
  currentPressure?: number
  unit: string
}

export interface MeasureDevice {
  id: string
  name: string
  model: string
  channels: number
  status: 'connected' | 'disconnected'
}

export const useMeasurementDeviceStore = defineStore('measurementDevices', () => {
  // State
  const pressureDevices = ref<PressureDevice[]>([
    { id: '1', name: '打压设备-1', ip: '192.168.1.100', port: 502, status: 'disconnected', unit: 'kPa' }
  ])
  
  const measureDevices = ref<MeasureDevice[]>([
    { id: '1', name: '计量设备-1', model: '1604', channels: 16, status: 'disconnected' }
  ])
  
  // Actions
  const connectPressureDevice = (id: string) => {
    const device = pressureDevices.value.find(d => d.id === id)
    if (device) {
      device.status = 'connected'
      device.currentPressure = 0
    }
  }
  
  const disconnectPressureDevice = (id: string) => {
    const device = pressureDevices.value.find(d => d.id === id)
    if (device) {
      device.status = 'disconnected'
      device.currentPressure = undefined
    }
  }
  
  const connectMeasureDevice = (id: string) => {
    const device = measureDevices.value.find(d => d.id === id)
    if (device) {
      device.status = 'connected'
    }
  }
  
  const disconnectMeasureDevice = (id: string) => {
    const device = measureDevices.value.find(d => d.id === id)
    if (device) {
      device.status = 'disconnected'
    }
  }
  
  return {
    pressureDevices,
    measureDevices,
    connectPressureDevice,
    disconnectPressureDevice,
    connectMeasureDevice,
    disconnectMeasureDevice
  }
})
```

**Step 5: Commit**
```bash
git add web/src/components/measurement/
git add web/src/stores/measurement/
git commit -m "feat: add measurement device components and store"
```

---

### Task 6: 实现计量工作台（CalibrationView）

**Files:**
- Modify: `web/src/views/measurement/CalibrationView.vue`

**Step 1: 完整实现CalibrationView**
```vue
<!-- web/src/views/measurement/CalibrationView.vue -->
<template>
  <div class="calibration-view">
    <!-- 可折叠侧边栏 -->
    <aside class="sidebar" :class="{ collapsed: sidebarCollapsed }">
      <div class="sidebar-toggle" @click="toggleSidebar">
        <el-icon><ArrowLeft v-if="!sidebarCollapsed" /><ArrowRight v-else /></el-icon>
      </div>
      <div v-show="!sidebarCollapsed" class="sidebar-content">
        <DevicePanel title="打压设备">
          <PressureDeviceCard
            v-for="device in deviceStore.pressureDevices"
            :key="device.id"
            :device="device"
            @connect="deviceStore.connectPressureDevice"
            @disconnect="deviceStore.disconnectPressureDevice"
          />
          <el-button type="primary" plain size="small" class="add-btn">
            <el-icon><Plus /></el-icon>添加设备
          </el-button>
        </DevicePanel>
        
        <DevicePanel title="计量设备">
          <MeasureDeviceCard
            v-for="device in deviceStore.measureDevices"
            :key="device.id"
            :device="device"
            @connect="deviceStore.connectMeasureDevice"
            @disconnect="deviceStore.disconnectMeasureDevice"
          />
          <el-button type="primary" plain size="small" class="add-btn">
            <el-icon><Plus /></el-icon>添加设备
          </el-button>
        </DevicePanel>
      </div>
    </aside>
    
    <!-- 主工作区 -->
    <main class="workbench">
      <!-- 第一行控制条 -->
      <div class="control-bar">
        <div class="control-group">
          <label>最小值</label>
          <el-input-number v-model="params.minValue" :precision="2" :step="0.1" />
        </div>
        <div class="control-group">
          <label>最大值</label>
          <el-input-number v-model="params.maxValue" :precision="2" :step="0.1" />
        </div>
        <div class="control-group">
          <label>点数</label>
          <el-input-number v-model="params.points" :min="2" :max="50" />
        </div>
        <div class="control-group">
          <label>精度</label>
          <el-input-number v-model="params.precision" :min="0" :max="4" />
        </div>
        <div class="control-group">
          <label>平均数</label>
          <el-input-number v-model="params.averageCount" :min="1" :max="100" />
        </div>
        <div class="control-group">
          <label>稳定时间</label>
          <el-select v-model="params.stableTime">
            <el-option label="1秒" :value="1" />
            <el-option label="3秒" :value="3" />
            <el-option label="5秒" :value="5" />
            <el-option label="10秒" :value="10" />
          </el-select>
        </div>
        <div class="control-group">
          <label>精度Level</label>
          <el-select v-model="params.precisionLevel">
            <el-option label="0.01%" value="0.01" />
            <el-option label="0.05%" value="0.05" />
            <el-option label="0.1%" value="0.1" />
            <el-option label="0.2%" value="0.2" />
          </el-select>
        </div>
        <el-button type="primary" class="generate-btn">
          生成压力表
        </el-button>
      </div>
      
      <!-- 第二行控制条 -->
      <div class="control-bar secondary">
        <div class="mode-switches">
          <div class="switch-group">
            <span>控制模式</span>
            <el-radio-group v-model="controlMode">
              <el-radio-button label="auto">自动</el-radio-button>
              <el-radio-button label="manual">手动</el-radio-button>
            </el-radio-group>
          </div>
          <div class="switch-group">
            <span>打压模式</span>
            <el-radio-group v-model="pressureMode">
              <el-radio-button label="single">单程</el-radio-button>
              <el-radio-button label="round">回程</el-radio-button>
            </el-radio-group>
          </div>
        </div>
        
        <div class="progress-section">
          <div class="progress-info">
            <span>进度: {{ currentPoint }}/{{ totalPoints }}</span>
            <el-progress :percentage="progressPercent" :stroke-width="8" />
          </div>
          <div class="stable-status">
            <span>稳定状态: {{ isStable ? '已稳定' : '稳定中' }}</span>
            <span v-if="!isStable" class="countdown">剩余: {{ stableCountdown }}s</span>
          </div>
        </div>
        
        <div class="action-buttons">
          <el-button type="success" @click="startCollection">开始采集</el-button>
          <el-button @click="pauseCollection">暂停</el-button>
          <el-button type="danger" @click="stopCollection">停止</el-button>
          <el-button @click="resetCollection">重置</el-button>
          <el-button type="primary" plain @click="exportReport">导出报告</el-button>
        </div>
      </div>
      
      <!-- 数据表格 -->
      <div class="data-table-wrapper">
        <el-table :data="tableData" border stripe class="data-table">
          <el-table-column prop="index" label="序号" width="60" />
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="getStatusType(row.status)">{{ getStatusText(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="targetValue" label="目标值" width="120" />
          <el-table-column 
            v-for="ch in 16" 
            :key="ch"
            :label="`CH${ch}`" 
            width="80"
          >
            <template #default="{ row }">
              <span :class="getChannelClass(row, ch - 1)">{{ row.channelValues[ch - 1]?.toFixed(2) || '--' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="timestamp" label="时间" width="160" />
        </el-table>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ArrowLeft, ArrowRight, Plus } from '@element-plus/icons-vue'
import { useMeasurementDeviceStore } from '@/stores/measurement/deviceStore'
import DevicePanel from '@/components/measurement/DevicePanel.vue'
import PressureDeviceCard from '@/components/measurement/PressureDeviceCard.vue'
import MeasureDeviceCard from '@/components/measurement/MeasureDeviceCard.vue'

// 侧边栏状态
const sidebarCollapsed = ref(false)
const toggleSidebar = () => {
  sidebarCollapsed.value = !sidebarCollapsed.value
}

// Store
const deviceStore = useMeasurementDeviceStore()

// 参数
const params = ref({
  minValue: 0,
  maxValue: 100,
  points: 10,
  precision: 2,
  averageCount: 5,
  stableTime: 3,
  precisionLevel: '0.05'
})

// 模式
const controlMode = ref('auto')
const pressureMode = ref('single')

// 进度
const currentPoint = ref(0)
const totalPoints = ref(10)
const progressPercent = computed(() => (currentPoint.value / totalPoints.value) * 100)

// 稳定状态
const isStable = ref(false)
const stableCountdown = ref(0)

// 表格数据
const tableData = ref([
  { index: 1, status: 'completed', targetValue: 10.00, channelValues: [10.01, 10.02, 10.00, 10.01, 9.99, 10.00, 10.01, 10.02, 10.00, 10.01, 9.99, 10.00, 10.01, 10.02, 10.00, 10.01], timestamp: '2024-01-15 10:30:00' },
  { index: 2, status: 'collecting', targetValue: 20.00, channelValues: [20.01, 20.02, 20.00, 20.01, 19.99, 20.00, 20.01, 20.02, null, null, null, null, null, null, null, null], timestamp: '2024-01-15 10:31:00' }
])

// 状态处理
const getStatusType = (status: string) => {
  const map: Record<string, string> = {
    pending: 'info',
    pressurizing: 'warning',
    stabilizing: '',
    collecting: 'primary',
    completed: 'success'
  }
  return map[status] || 'info'
}

const getStatusText = (status: string) => {
  const map: Record<string, string> = {
    pending: '待执行',
    pressurizing: '打压中',
    stabilizing: '稳定中',
    collecting: '采集中',
    completed: '已完成'
  }
  return map[status] || status
}

const getChannelClass = (row: any, index: number) => {
  const value = row.channelValues[index]
  if (!value) return ''
  const diff = Math.abs(value - row.targetValue)
  if (diff < 0.1) return 'channel-good'
  if (diff < 0.5) return 'channel-warning'
  return 'channel-error'
}

// 操作
const startCollection = () => console.log('开始采集')
const pauseCollection = () => console.log('暂停采集')
const stopCollection = () => console.log('停止采集')
const resetCollection = () => console.log('重置')
const exportReport = () => console.log('导出报告')
</script>

<style scoped lang="scss">
.calibration-view {
  display: flex;
  height: calc(100vh - 60px);
  background: var(--bg-primary);
  
  .sidebar {
    width: 300px;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border-color);
    position: relative;
    transition: width 0.3s;
    
    &.collapsed {
      width: 40px;
    }
    
    .sidebar-toggle {
      position: absolute;
      right: -20px;
      top: 50%;
      transform: translateY(-50%);
      width: 20px;
      height: 60px;
      background: var(--bg-tertiary);
      border-radius: 0 4px 4px 0;
      display: flex;
      align-items: center;
      justify-content: center;
      cursor: pointer;
      z-index: 10;
      
      .el-icon {
        color: var(--text-secondary);
        font-size: 12px;
      }
    }
    
    .sidebar-content {
      padding: var(--spacing-md);
      overflow-y: auto;
      height: 100%;
      
      .add-btn {
        width: 100%;
        margin-top: var(--spacing-sm);
      }
    }
  }
  
  .workbench {
    flex: 1;
    padding: var(--spacing-md);
    overflow: auto;
    
    .control-bar {
      background: var(--bg-secondary);
      border: 1px solid var(--border-color);
      border-radius: var(--radius-md);
      padding: var(--spacing-md);
      margin-bottom: var(--spacing-md);
      display: flex;
      align-items: center;
      gap: var(--spacing-md);
      flex-wrap: wrap;
      
      .control-group {
        display: flex;
        flex-direction: column;
        gap: var(--spacing-xs);
        
        label {
          color: var(--text-secondary);
          font-size: 12px;
        }
        
        .el-input-number,
        .el-select {
          width: 100px;
        }
      }
      
      .generate-btn {
        margin-left: auto;
      }
      
      &.secondary {
        justify-content: space-between;
        
        .mode-switches {
          display: flex;
          gap: var(--spacing-lg);
          
          .switch-group {
            display: flex;
            flex-direction: column;
            gap: var(--spacing-xs);
            
            span {
              color: var(--text-secondary);
              font-size: 12px;
            }
          }
        }
        
        .progress-section {
          display: flex;
          flex-direction: column;
          gap: var(--spacing-xs);
          min-width: 200px;
          
          .progress-info {
            display: flex;
            align-items: center;
            gap: var(--spacing-sm);
            color: var(--text-primary);
            font-size: 14px;
            
            .el-progress {
              flex: 1;
            }
          }
          
          .stable-status {
            display: flex;
            gap: var(--spacing-md);
            color: var(--text-secondary);
            font-size: 12px;
            
            .countdown {
              color: var(--accent-primary);
            }
          }
        }
        
        .action-buttons {
          display: flex;
          gap: var(--spacing-sm);
        }
      }
    }
    
    .data-table-wrapper {
      background: var(--bg-secondary);
      border: 1px solid var(--border-color);
      border-radius: var(--radius-md);
      padding: var(--spacing-md);
      
      .data-table {
        width: 100%;
        
        :deep(th) {
          background: var(--bg-tertiary);
          color: var(--text-secondary);
        }
        
        :deep(td) {
          color: var(--text-primary);
        }
        
        .channel-good {
          color: var(--status-success);
        }
        
        .channel-warning {
          color: var(--status-warning);
        }
        
        .channel-error {
          color: var(--status-error);
        }
      }
    }
  }
}
</style>
```

**Step 2: 验证计量工作台**
```bash
npm run dev
```
访问 http://localhost:5173/measurement，确认：
- 侧边栏可折叠/展开
- 设备卡片显示正常
- 控制条参数可调整
- 数据表格显示正常

**Step 3: Commit**
```bash
git add web/src/views/measurement/CalibrationView.vue
git commit -m "feat: implement measurement calibration view"
```

---

### Task 7: 实现多设备打压（PressureWorkbenchView）

**Files:**
- Modify: `web/src/views/measurement/PressureWorkbenchView.vue`

**Step 1: 完整实现PressureWorkbenchView**
```vue
<!-- web/src/views/measurement/PressureWorkbenchView.vue -->
<template>
  <div class="pressure-workbench">
    <!-- 顶部工具栏 -->
    <header class="toolbar">
      <div class="left">
        <el-button @click="$router.push('/')">
          <el-icon><ArrowLeft /></el-icon>返回首页
        </el-button>
        <h2>多设备打压控制</h2>
      </div>
      <div class="center">
        <StatCard label="设备总数" :value="devices.length" />
        <StatCard label="在线设备" :value="onlineCount" color="#10b981" />
      </div>
      <div class="right">
        <el-button @click="refreshStatus">
          <el-icon><Refresh /></el-icon>刷新状态
        </el-button>
        <el-button type="primary" @click="showAddDialog = true">
          <el-icon><Plus /></el-icon>添加设备
        </el-button>
      </div>
    </header>
    
    <!-- 设备卡片网格 -->
    <div class="device-grid">
      <div 
        v-for="device in devices" 
        :key="device.id"
        class="device-card"
        :class="{ 'is-connected': device.status === 'connected' }"
      >
        <div class="card-header">
          <h4>{{ device.name }}</h4>
          <div class="status-indicator" :class="device.status" />
        </div>
        
        <div class="pressure-display">
          <div class="current-pressure">
            <span class="value">{{ device.currentPressure?.toFixed(2) || '--' }}</span>
            <span class="unit">{{ device.unit }}</span>
          </div>
          <div class="label">当前压力</div>
        </div>
        
        <div class="control-section">
          <div class="input-group">
            <label>设定压力</label>
            <el-input-number 
              v-model="device.targetPressure" 
              :precision="2" 
              :step="0.1"
              :min="0"
            />
          </div>
          <div class="input-group">
            <label>单位</label>
            <el-select v-model="device.unit" size="small">
              <el-option label="kPa" value="kPa" />
              <el-option label="MPa" value="MPa" />
              <el-option label="bar" value="bar" />
              <el-option label="psi" value="psi" />
            </el-select>
          </div>
        </div>
        
        <div class="card-actions">
          <el-button 
            :type="device.status === 'connected' ? 'danger' : 'success'"
            @click="toggleConnection(device)"
          >
            {{ device.status === 'connected' ? '断开' : '连接' }}
          </el-button>
          <el-button 
            type="primary" 
            :disabled="device.status !== 'connected'"
            @click="setPressure(device)"
          >
            设定压力
          </el-button>
        </div>
      </div>
    </div>
    
    <!-- 添加设备对话框 -->
    <el-dialog v-model="showAddDialog" title="添加打压设备" width="400px">
      <el-form :model="newDevice" label-width="80px">
        <el-form-item label="设备名称">
          <el-input v-model="newDevice.name" placeholder="请输入设备名称" />
        </el-form-item>
        <el-form-item label="IP地址">
          <el-input v-model="newDevice.ip" placeholder="192.168.1.xxx" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="newDevice.port" :min="1" :max="65535" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="addDevice">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ArrowLeft, Refresh, Plus } from '@element-plus/icons-vue'
import StatCard from '@/components/common/StatCard.vue'

interface PressureDevice {
  id: string
  name: string
  ip: string
  port: number
  status: 'connected' | 'disconnected'
  currentPressure?: number
  targetPressure: number
  unit: string
}

// 设备列表
const devices = ref<PressureDevice[]>([
  { id: '1', name: '打压设备-1', ip: '192.168.1.101', port: 502, status: 'connected', currentPressure: 50.25, targetPressure: 100, unit: 'kPa' },
  { id: '2', name: '打压设备-2', ip: '192.168.1.102', port: 502, status: 'disconnected', targetPressure: 200, unit: 'kPa' },
  { id: '3', name: '打压设备-3', ip: '192.168.1.103', port: 502, status: 'connected', currentPressure: 150.00, targetPressure: 300, unit: 'kPa' }
])

// 统计
const onlineCount = computed(() => devices.value.filter(d => d.status === 'connected').length)

// 添加设备对话框
const showAddDialog = ref(false)
const newDevice = ref({
  name: '',
  ip: '',
  port: 502
})

// 方法
const toggleConnection = (device: PressureDevice) => {
  if (device.status === 'connected') {
    device.status = 'disconnected'
    device.currentPressure = undefined
  } else {
    device.status = 'connected'
    device.currentPressure = 0
  }
}

const setPressure = (device: PressureDevice) => {
  console.log(`设置设备 ${device.name} 压力为 ${device.targetPressure} ${device.unit}`)
  // 模拟压力变化
  if (device.status === 'connected') {
    device.currentPressure = device.targetPressure
  }
}

const refreshStatus = () => {
  console.log('刷新设备状态')
}

const addDevice = () => {
  const id = String(devices.value.length + 1)
  devices.value.push({
    id,
    name: newDevice.value.name || `打压设备-${id}`,
    ip: newDevice.value.ip || '192.168.1.100',
    port: newDevice.value.port,
    status: 'disconnected',
    targetPressure: 0,
    unit: 'kPa'
  })
  showAddDialog.value = false
  newDevice.value = { name: '', ip: '', port: 502 }
}
</script>

<style scoped lang="scss">
.pressure-workbench {
  padding: var(--spacing-lg);
  
  .toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    padding: var(--spacing-md);
    margin-bottom: var(--spacing-lg);
    
    .left {
      display: flex;
      align-items: center;
      gap: var(--spacing-md);
      
      h2 {
        color: var(--text-primary);
        margin: 0;
        font-size: 20px;
      }
    }
    
    .center {
      display: flex;
      gap: var(--spacing-md);
    }
    
    .right {
      display: flex;
      gap: var(--spacing-sm);
    }
  }
  
  .device-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));
    gap: var(--spacing-lg);
    
    .device-card {
      background: var(--bg-secondary);
      border: 1px solid var(--border-color);
      border-radius: var(--radius-lg);
      padding: var(--spacing-lg);
      transition: all 0.3s;
      
      &.is-connected {
        border-color: var(--status-success);
        box-shadow: 0 0 0 1px var(--status-success);
      }
      
      .card-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--spacing-md);
        
        h4 {
          color: var(--text-primary);
          margin: 0;
        }
        
        .status-indicator {
          width: 12px;
          height: 12px;
          border-radius: 50%;
          
          &.connected {
            background: var(--status-success);
            box-shadow: 0 0 8px var(--status-success);
          }
          
          &.disconnected {
            background: var(--text-muted);
          }
        }
      }
      
      .pressure-display {
        text-align: center;
        padding: var(--spacing-lg);
        background: var(--bg-tertiary);
        border-radius: var(--radius-md);
        margin-bottom: var(--spacing-md);
        
        .current-pressure {
          .value {
            font-size: 48px;
            font-weight: bold;
            color: var(--accent-primary);
          }
          
          .unit {
            font-size: 24px;
            color: var(--text-secondary);
            margin-left: var(--spacing-sm);
          }
        }
        
        .label {
          color: var(--text-secondary);
          font-size: 14px;
          margin-top: var(--spacing-xs);
        }
      }
      
      .control-section {
        display: grid;
        grid-template-columns: 1fr 100px;
        gap: var(--spacing-md);
        margin-bottom: var(--spacing-md);
        
        .input-group {
          label {
            display: block;
            color: var(--text-secondary);
            font-size: 12px;
            margin-bottom: var(--spacing-xs);
          }
          
          .el-input-number,
          .el-select {
            width: 100%;
          }
        }
      }
      
      .card-actions {
        display: flex;
        gap: var(--spacing-sm);
        
        .el-button {
          flex: 1;
        }
      }
    }
  }
}
</style>
```

**Step 2: 验证多设备打压**
```bash
npm run dev
```
访问 http://localhost:5173/multi-pressure，确认：
- 顶部工具栏显示正常
- 设备卡片双列布局
- 压力显示、设定功能正常
- 添加设备对话框正常

**Step 3: Commit**
```bash
git add web/src/views/measurement/PressureWorkbenchView.vue
git commit -m "feat: implement multi-pressure workbench view"
```

---

## Phase 4: 标定模块实现

### Task 8: 创建标定模块组件（第1批）

**Files:**
- Create: `web/src/components/calibration/ProgressIndicator.vue`
- Create: `web/src/components/calibration/Device1604Panel.vue`
- Create: `web/src/components/calibration/PressDevicePanel.vue`

**Step 1: 创建ProgressIndicator**
```vue
<!-- web/src/components/calibration/ProgressIndicator.vue -->
<template>
  <div class="progress-indicator">
    <h4 class="title">校准流程</h4>
    <div class="steps">
      <div 
        v-for="(step, index) in steps" 
        :key="index"
        class="step"
        :class="{
          completed: currentStep > index,
          active: currentStep === index,
          pending: currentStep < index
        }"
      >
        <div class="step-marker">
          <el-icon v-if="currentStep > index"><Check /></el-icon>
          <span v-else>{{ index + 1 }}</span>
        </div>
        <div class="step-label">{{ step }}</div>
        <div v-if="index < steps.length - 1" class="step-line" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Check } from '@element-plus/icons-vue'

defineProps<{
  currentStep: number
}>()

const steps = [
  '设备连接',
  '通道选择',
  '开始校准',
  '数据采集',
  '数据拟合',
  '完成'
]
</script>

<style scoped lang="scss">
.progress-indicator {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  
  .title {
    color: var(--text-primary);
    margin: 0 0 var(--spacing-md) 0;
    font-size: 16px;
  }
  
  .steps {
    display: flex;
    position: relative;
    
    .step {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      position: relative;
      
      .step-marker {
        width: 28px;
        height: 28px;
        border-radius: 50%;
        display: flex;
        align-items: center;
        justify-content: center;
        font-size: 12px;
        font-weight: bold;
        z-index: 2;
        
        .el-icon {
          font-size: 16px;
        }
      }
      
      .step-label {
        margin-top: var(--spacing-xs);
        font-size: 11px;
        text-align: center;
        white-space: nowrap;
      }
      
      .step-line {
        position: absolute;
        top: 14px;
        left: 50%;
        width: 100%;
        height: 2px;
        background: var(--border-color);
        z-index: 1;
      }
      
      &.completed {
        .step-marker {
          background: var(--status-success);
          color: white;
        }
        
        .step-label {
          color: var(--status-success);
        }
        
        .step-line {
          background: var(--status-success);
        }
      }
      
      &.active {
        .step-marker {
          background: var(--accent-primary);
          color: white;
          box-shadow: 0 0 0 4px rgba(233, 69, 96, 0.3);
        }
        
        .step-label {
          color: var(--accent-primary);
          font-weight: bold;
        }
      }
      
      &.pending {
        .step-marker {
          background: var(--bg-tertiary);
          color: var(--text-muted);
          border: 2px solid var(--border-color);
        }
        
        .step-label {
          color: var(--text-muted);
        }
      }
    }
  }
}
</style>
```

**Step 2: 创建Device1604Panel**
```vue
<!-- web/src/components/calibration/Device1604Panel.vue -->
<template>
  <div class="device-panel">
    <div class="panel-header">
      <div class="device-info">
        <el-icon class="device-icon"><Cpu /></el-icon>
        <div>
          <div class="device-name">1604设备</div>
          <div class="device-type">计量采集设备</div>
        </div>
      </div>
      <DeviceStatusBadge :status="status" />
    </div>
    
    <div class="connection-control">
      <div class="input-group">
        <span class="prefix">TCP://</span>
        <el-input v-model="ip" placeholder="192.168.1.100" />
      </div>
      <el-button 
        :type="status === 'connected' ? 'danger' : 'primary'"
        @click="toggleConnection"
      >
        {{ status === 'connected' ? '断开' : '连接' }}
      </el-button>
    </div>
    
    <div v-if="status === 'connected'" class="device-status">
      <div class="status-row">
        <span class="label">阀门状态:</span>
        <el-tag :type="valveStatus === 'open' ? 'success' : 'info'" size="small">
          {{ valveStatus === 'open' ? '开启' : '关闭' }}
        </el-tag>
      </div>
      <div class="status-row">
        <span class="label">单位类型:</span>
        <span class="value">kPa</span>
      </div>
      <div v-if="needCalibration" class="warning">
        <el-icon><Warning /></el-icon>
        <span>设备需要校准</span>
      </div>
    </div>
    
    <div v-if="status === 'connected'" class="valve-control">
      <el-button 
        :type="valveStatus === 'open' ? 'primary' : 'default'"
        @click="valveStatus = 'open'"
      >
        打开阀门
      </el-button>
      <el-button 
        :type="valveStatus === 'close' ? 'primary' : 'default'"
        @click="valveStatus = 'close'"
      >
        关闭阀门
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Cpu, Warning } from '@element-plus/icons-vue'
import DeviceStatusBadge from '@/components/common/DeviceStatusBadge.vue'

const ip = ref('192.168.1.100')
const status = ref<'connected' | 'disconnected'>('disconnected')
const valveStatus = ref<'open' | 'close'>('close')
const needCalibration = ref(false)

const toggleConnection = () => {
  status.value = status.value === 'connected' ? 'disconnected' : 'connected'
}
</script>

<style scoped lang="scss">
.device-panel {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-md);
    
    .device-info {
      display: flex;
      align-items: center;
      gap: var(--spacing-sm);
      
      .device-icon {
        font-size: 24px;
        color: var(--accent-primary);
      }
      
      .device-name {
        color: var(--text-primary);
        font-weight: 500;
      }
      
      .device-type {
        color: var(--text-secondary);
        font-size: 12px;
      }
    }
  }
  
  .connection-control {
    display: flex;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-md);
    
    .input-group {
      flex: 1;
      display: flex;
      align-items: center;
      background: var(--bg-tertiary);
      border: 1px solid var(--border-color);
      border-radius: var(--radius-sm);
      padding: 0 var(--spacing-sm);
      
      .prefix {
        color: var(--text-muted);
        font-size: 12px;
        margin-right: var(--spacing-xs);
        white-space: nowrap;
      }
      
      .el-input {
        flex: 1;
        
        :deep(.el-input__wrapper) {
          background: transparent;
          box-shadow: none;
        }
      }
    }
  }
  
  .device-status {
    background: var(--bg-tertiary);
    border-radius: var(--radius-sm);
    padding: var(--spacing-md);
    margin-bottom: var(--spacing-md);
    
    .status-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: var(--spacing-sm);
      
      &:last-child {
        margin-bottom: 0;
      }
      
      .label {
        color: var(--text-secondary);
        font-size: 13px;
      }
      
      .value {
        color: var(--text-primary);
        font-size: 13px;
      }
    }
    
    .warning {
      display: flex;
      align-items: center;
      gap: var(--spacing-xs);
      color: var(--status-warning);
      font-size: 12px;
      margin-top: var(--spacing-sm);
      padding-top: var(--spacing-sm);
      border-top: 1px solid var(--border-color);
    }
  }
  
  .valve-control {
    display: flex;
    gap: var(--spacing-sm);
    
    .el-button {
      flex: 1;
    }
  }
}
</style>
```

**Step 3: 创建PressDevicePanel**
```vue
<!-- web/src/components/calibration/PressDevicePanel.vue -->
<template>
  <div class="device-panel">
    <div class="panel-header">
      <div class="device-info">
        <el-icon class="device-icon"><FirstAidKit /></el-icon>
        <div>
          <div class="device-name">打压设备</div>
          <div class="device-type">压力控制器</div>
        </div>
      </div>
      <DeviceStatusBadge :status="status" />
    </div>
    
    <div class="connection-control">
      <el-input v-model="ip" placeholder="IP地址" />
      <el-input-number v-model="port" :min="1" :max="65535" controls-position="right" />
      <el-button 
        :type="status === 'connected' ? 'danger' : 'primary'"
        @click="toggleConnection"
      >
        {{ status === 'connected' ? '断开' : '连接' }}
      </el-button>
    </div>
    
    <div v-if="status === 'connected'" class="pressure-control">
      <div class="current-pressure">
        <span class="label">当前压力:</span>
        <span class="value">{{ currentPressure.toFixed(2) }} kPa</span>
      </div>
      <div class="pressure-actions">
        <el-button @click="adjustPressure(-1)">
          <el-icon><ArrowDown /></el-icon>降压
        </el-button>
        <el-input-number v-model="targetPressure" :precision="2" :step="1" />
        <el-button @click="adjustPressure(1)">
          <el-icon><ArrowUp /></el-icon>升压
        </el-button>
      </div>
      <el-button type="primary" class="set-btn" @click="setPressure">
        设定压力
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { FirstAidKit, ArrowUp, ArrowDown } from '@element-plus/icons-vue'
import DeviceStatusBadge from '@/components/common/DeviceStatusBadge.vue'

const ip = ref('192.168.1.101')
const port = ref(502)
const status = ref<'connected' | 'disconnected'>('disconnected')
const currentPressure = ref(0)
const targetPressure = ref(100)

const toggleConnection = () => {
  status.value = status.value === 'connected' ? 'disconnected' : 'connected'
  if (status.value === 'connected') {
    currentPressure.value = 0
  }
}

const adjustPressure = (delta: number) => {
  targetPressure.value += delta * 10
}

const setPressure = () => {
  console.log(`设定压力: ${targetPressure.value}`)
  currentPressure.value = targetPressure.value
}
</script>

<style scoped lang="scss">
.device-panel {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-md);
    
    .device-info {
      display: flex;
      align-items: center;
      gap: var(--spacing-sm);
      
      .device-icon {
        font-size: 24px;
        color: var(--status-info);
      }
      
      .device-name {
        color: var(--text-primary);
        font-weight: 500;
      }
      
      .device-type {
        color: var(--text-secondary);
        font-size: 12px;
      }
    }
  }
  
  .connection-control {
    display: flex;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-md);
    
    .el-input {
      flex: 2;
    }
    
    .el-input-number {
      flex: 1;
    }
  }
  
  .pressure-control {
    background: var(--bg-tertiary);
    border-radius: var(--radius-sm);
    padding: var(--spacing-md);
    
    .current-pressure {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: var(--spacing-md);
      
      .label {
        color: var(--text-secondary);
        font-size: 13px;
      }
      
      .value {
        color: var(--accent-primary);
        font-size: 20px;
        font-weight: bold;
      }
    }
    
    .pressure-actions {
      display: flex;
      align-items: center;
      gap: var(--spacing-sm);
      margin-bottom: var(--spacing-md);
      
      .el-input-number {
        flex: 1;
      }
    }
    
    .set-btn {
      width: 100%;
    }
  }
}
</style>
```

**Step 4: Commit**
```bash
git add web/src/components/calibration/
git commit -m "feat: add calibration progress indicator and device panels"
```

---

### Task 9: 创建标定模块组件（第2批）

**Files:**
- Create: `web/src/components/calibration/ChannelMatrix.vue`
- Create: `web/src/components/calibration/CalibrationControlPanel.vue`

**Step 1: 创建ChannelMatrix**
```vue
<!-- web/src/components/calibration/ChannelMatrix.vue -->
<template>
  <div class="channel-matrix">
    <div class="matrix-header">
      <h4>通道选择</h4>
      <div class="actions">
        <span class="count">已选: {{ selectedCount }}/16</span>
        <el-button type="primary" link size="small" @click="selectAll">全选</el-button>
        <el-button type="danger" link size="small" @click="clearAll">清空</el-button>
      </div>
    </div>
    
    <div class="matrix-grid">
      <div 
        v-for="(selected, index) in channels" 
        :key="index"
        class="channel-item"
        :class="{ selected }"
        @click="toggleChannel(index)"
      >
        <el-checkbox v-model="channels[index]" @click.stop>
          CH{{ index + 1 }}
        </el-checkbox>
      </div>
    </div>
    
    <div v-if="selectedCount === 0" class="warning">
      <el-icon><Warning /></el-icon>
      <span>请至少选择一个通道</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Warning } from '@element-plus/icons-vue'

const channels = ref<boolean[]>(new Array(16).fill(false))

const selectedCount = computed(() => channels.value.filter(Boolean).length)

const toggleChannel = (index: number) => {
  channels.value[index] = !channels.value[index]
}

const selectAll = () => {
  channels.value.fill(true)
}

const clearAll = () => {
  channels.value.fill(false)
}
</script>

<style scoped lang="scss">
.channel-matrix {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  
  .matrix-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-md);
    
    h4 {
      color: var(--text-primary);
      margin: 0;
    }
    
    .actions {
      display: flex;
      align-items: center;
      gap: var(--spacing-sm);
      
      .count {
        color: var(--text-secondary);
        font-size: 13px;
      }
    }
  }
  
  .matrix-grid {
    display: grid;
    grid-template-columns: repeat(8, 1fr);
    gap: var(--spacing-sm);
    
    @media (max-width: 1200px) {
      grid-template-columns: repeat(4, 1fr);
    }
    
    .channel-item {
      background: var(--bg-tertiary);
      border: 2px solid transparent;
      border-radius: var(--radius-sm);
      padding: var(--spacing-sm);
      cursor: pointer;
      transition: all 0.2s;
      
      &:hover {
        border-color: var(--accent-primary);
      }
      
      &.selected {
        background: rgba(16, 185, 129, 0.2);
        border-color: var(--status-success);
      }
      
      .el-checkbox {
        color: var(--text-primary);
        
        :deep(.el-checkbox__input.is-checked + .el-checkbox__label) {
          color: var(--status-success);
        }
      }
    }
  }
  
  .warning {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    color: var(--status-warning);
    font-size: 12px;
    margin-top: var(--spacing-md);
    padding: var(--spacing-sm);
    background: rgba(245, 158, 11, 0.1);
    border-radius: var(--radius-sm);
  }
}
</style>
```

**Step 2: 创建CalibrationControlPanel**
```vue
<!-- web/src/components/calibration/CalibrationControlPanel.vue -->
<template>
  <div class="control-panel">
    <h4 class="title">校准控制</h4>
    
    <div class="main-action">
      <el-button 
        type="primary" 
        size="large" 
        class="start-btn"
        :disabled="!canStart"
        @click="startCalibration"
      >
        <el-icon><VideoPlay /></el-icon>
        开始校准
      </el-button>
      
      <div class="prerequisites">
        <div 
          v-for="(item, index) in prerequisites" 
          :key="index"
          class="prereq-item"
          :class="{ satisfied: item.satisfied }"
        >
          <el-icon v-if="item.satisfied"><CircleCheckFilled /></el-icon>
          <el-icon v-else><CircleClose /></el-icon>
          <span>{{ item.label }}</span>
        </div>
      </div>
    </div>
    
    <el-divider />
    
    <div class="secondary-actions">
      <el-button 
        type="success" 
        :disabled="!canFit"
        @click="fitData"
      >
        <el-icon><DataAnalysis /></el-icon>
        数据拟合
      </el-button>
      <el-button 
        type="danger" 
        plain
        @click="endCalibration"
      >
        <el-icon><CircleClose /></el-icon>
        结束校准
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { VideoPlay, CircleCheckFilled, CircleClose, DataAnalysis } from '@element-plus/icons-vue'

const props = defineProps<{
  device1604Connected: boolean
  pressDeviceConnected: boolean
  channelsSelected: boolean
  hasCollectedData: boolean
}>()

const prerequisites = computed(() => [
  { label: '1604设备已连接', satisfied: props.device1604Connected },
  { label: '打压设备已连接', satisfied: props.pressDeviceConnected },
  { label: '已选择通道', satisfied: props.channelsSelected }
])

const canStart = computed(() => 
  props.device1604Connected && 
  props.pressDeviceConnected && 
  props.channelsSelected
)

const canFit = computed(() => props.hasCollectedData)

const emit = defineEmits<{
  start: []
  fit: []
  end: []
}>()

const startCalibration = () => emit('start')
const fitData = () => emit('fit')
const endCalibration = () => emit('end')
</script>

<style scoped lang="scss">
.control-panel {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  
  .title {
    color: var(--text-primary);
    margin: 0 0 var(--spacing-md) 0;
  }
  
  .main-action {
    text-align: center;
    
    .start-btn {
      width: 100%;
      height: 50px;
      font-size: 16px;
      margin-bottom: var(--spacing-md);
      
      .el-icon {
        margin-right: var(--spacing-xs);
      }
    }
    
    .prerequisites {
      text-align: left;
      
      .prereq-item {
        display: flex;
        align-items: center;
        gap: var(--spacing-xs);
        padding: var(--spacing-xs) 0;
        color: var(--text-muted);
        font-size: 13px;
        
        .el-icon {
          font-size: 14px;
        }
        
        &.satisfied {
          color: var(--status-success);
        }
      }
    }
  }
  
  :deep(.el-divider) {
    margin: var(--spacing-md) 0;
    border-color: var(--border-color);
  }
  
  .secondary-actions {
    display: flex;
    gap: var(--spacing-sm);
    
    .el-button {
      flex: 1;
    }
  }
}
</style>
```

**Step 3: Commit**
```bash
git add web/src/components/calibration/
git commit -m "feat: add channel matrix and calibration control panel"
```

---

### Task 10: 创建标定模块组件（第3批）

**Files:**
- Create: `web/src/components/calibration/PressurePointList.vue`
- Create: `web/src/components/calibration/CalibrationDataTable.vue`

**Step 1: 创建PressurePointList**
```vue
<!-- web/src/components/calibration/PressurePointList.vue -->
<template>
  <div class="pressure-point-list">
    <div class="list-header">
      <h4>压力点设置</h4>
      <div class="actions">
        <div class="point-count">
          <label>压力点个数:</label>
          <el-input-number v-model="pointCount" :min="1" :max="50" size="small" />
        </div>
        <div class="progress">
          <label>完成进度:</label>
          <el-progress :percentage="progressPercent" :stroke-width="8" style="width: 120px" />
        </div>
      </div>
    </div>
    
    <el-table :data="points" border stripe class="point-table">
      <el-table-column type="index" label="序号" width="60" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="getStatusType(row.status)" size="small">
            {{ getStatusText(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="目标压力" width="150">
        <template #default="{ row }">
          <el-input-number 
            v-model="row.targetPressure" 
            :precision="2" 
            :step="0.1"
            size="small"
          />
        </template>
      </el-table-column>
      <el-table-column label="打压/确认" width="120">
        <template #default="{ row }">
          <el-button 
            v-if="row.status === 'pending_press'"
            type="primary" 
            size="small"
            @click="pressurize(row)"
          >
            打压
          </el-button>
          <el-button 
            v-else-if="row.status === 'pending_confirm'"
            type="success" 
            size="small"
            @click="confirm(row)"
          >
            确认
          </el-button>
          <span v-else class="done-text">--</span>
        </template>
      </el-table-column>
      <el-table-column label="采集" width="120">
        <template #default="{ row }">
          <el-button 
            v-if="row.status === 'pending_collect'"
            type="primary" 
            size="small"
            @click="collect(row)"
          >
            采集
          </el-button>
          <el-button 
            v-else-if="row.status === 'completed'"
            type="warning" 
            link
            size="small"
            @click="recollect(row)"
          >
            重新采集
          </el-button>
          <span v-else class="wait-text">等待中</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="80">
        <template #default="{ row, $index }">
          <el-button type="danger" link size="small" @click="removePoint($index)">
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

type PointStatus = 'pending_press' | 'pending_confirm' | 'pending_collect' | 'completed'

interface PressurePoint {
  targetPressure: number
  status: PointStatus
}

const pointCount = ref(5)
const points = ref<PressurePoint[]>([
  { targetPressure: 10, status: 'completed' },
  { targetPressure: 20, status: 'completed' },
  { targetPressure: 30, status: 'pending_collect' },
  { targetPressure: 40, status: 'pending_confirm' },
  { targetPressure: 50, status: 'pending_press' }
])

const progressPercent = computed(() => {
  const completed = points.value.filter(p => p.status === 'completed').length
  return Math.round((completed / points.value.length) * 100)
})

const getStatusType = (status: PointStatus) => {
  const map: Record<PointStatus, string> = {
    pending_press: 'info',
    pending_confirm: 'warning',
    pending_collect: 'primary',
    completed: 'success'
  }
  return map[status]
}

const getStatusText = (status: PointStatus) => {
  const map: Record<PointStatus, string> = {
    pending_press: '待打压',
    pending_confirm: '待确认',
    pending_collect: '待采集',
    completed: '完成'
  }
  return map[status]
}

const pressurize = (row: PressurePoint) => {
  console.log('打压:', row.targetPressure)
  row.status = 'pending_confirm'
}

const confirm = (row: PressurePoint) => {
  console.log('确认压力:', row.targetPressure)
  row.status = 'pending_collect'
}

const collect = (row: PressurePoint) => {
  console.log('采集数据:', row.targetPressure)
  row.status = 'completed'
}

const recollect = (row: PressurePoint) => {
  console.log('重新采集:', row.targetPressure)
  row.status = 'pending_collect'
}

const removePoint = (index: number) => {
  points.value.splice(index, 1)
}
</script>

<style scoped lang="scss">
.pressure-point-list {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  
  .list-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-md);
    
    h4 {
      color: var(--text-primary);
      margin: 0;
    }
    
    .actions {
      display: flex;
      align-items: center;
      gap: var(--spacing-lg);
      
      .point-count,
      .progress {
        display: flex;
        align-items: center;
        gap: var(--spacing-sm);
        
        label {
          color: var(--text-secondary);
          font-size: 13px;
        }
      }
    }
  }
  
  .point-table {
    :deep(th) {
      background: var(--bg-tertiary);
      color: var(--text-secondary);
    }
    
    :deep(td) {
      color: var(--text-primary);
    }
    
    .done-text,
    .wait-text {
      color: var(--text-muted);
      font-size: 13px;
    }
  }
}
</style>
```

**Step 2: 创建CalibrationDataTable**
```vue
<!-- web/src/components/calibration/CalibrationDataTable.vue -->
<template>
  <div class="data-table-panel">
    <div class="panel-header">
      <h4>采集数据</h4>
      <div class="actions">
        <span class="record-count">记录数: {{ data.length }}</span>
        <el-button type="primary" @click="exportData">
          <el-icon><Download /></el-icon>
          导出CSV
        </el-button>
      </div>
    </div>
    
    <el-table :data="data" border stripe class="data-table">
      <el-table-column prop="point" label="压力点" width="80" />
      <el-table-column prop="targetPressure" label="目标压力" width="120" />
      <el-table-column 
        v-for="ch in selectedChannels" 
        :key="ch"
        :label="`CH${ch}`" 
        width="100"
      >
        <template #default="{ row }">
          {{ row.channelData[ch - 1]?.toFixed(4) || '--' }}
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'collected' ? 'success' : 'info'" size="small">
            {{ row.status === 'collected' ? '已采集' : '待采集' }}
          </el-tag>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { Download } from '@element-plus/icons-vue'

interface CalibrationData {
  point: number
  targetPressure: number
  channelData: number[]
  status: 'collected' | 'pending'
}

const props = defineProps<{
  data: CalibrationData[]
  selectedChannels: number[]
}>()

const exportData = () => {
  console.log('导出数据')
  // 生成CSV并下载
}
</script>

<style scoped lang="scss">
.data-table-panel {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--spacing-md);
  
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-md);
    
    h4 {
      color: var(--text-primary);
      margin: 0;
    }
    
    .actions {
      display: flex;
      align-items: center;
      gap: var(--spacing-md);
      
      .record-count {
        color: var(--text-secondary);
        font-size: 13px;
      }
    }
  }
  
  .data-table {
    :deep(th) {
      background: var(--bg-tertiary);
      color: var(--text-secondary);
    }
    
    :deep(td) {
      color: var(--text-primary);
    }
  }
}
</style>
```

**Step 3: 创建标定模块Store**
```typescript
// web/src/stores/calibration/index.ts
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export enum CalibrationStep {
  DEVICE_CONNECT = 0,
  CHANNEL_SELECT = 1,
  START_CALIBRATION = 2,
  DATA_COLLECTION = 3,
  DATA_FITTING = 4,
  COMPLETED = 5
}

export interface PressurePoint {
  id: string
  index: number
  targetPressure: number
  status: 'pending_press' | 'pending_confirm' | 'pending_collect' | 'completed'
  collectedData?: number[]
}

export const useCalibrationStore = defineStore('calibration', () => {
  // State
  const currentStep = ref(CalibrationStep.DEVICE_CONNECT)
  const device1604Connected = ref(false)
  const pressDeviceConnected = ref(false)
  const selectedChannels = ref<number[]>([])
  const pressurePoints = ref<PressurePoint[]>([])
  
  // Getters
  const channelsSelected = computed(() => selectedChannels.value.length > 0)
  const hasCollectedData = computed(() => 
    pressurePoints.value.some(p => p.status === 'completed')
  )
  
  // Actions
  const setStep = (step: CalibrationStep) => {
    currentStep.value = step
  }
  
  const toggleDevice1604 = () => {
    device1604Connected.value = !device1604Connected.value
  }
  
  const togglePressDevice = () => {
    pressDeviceConnected.value = !pressDeviceConnected.value
  }
  
  const setSelectedChannels = (channels: number[]) => {
    selectedChannels.value = channels
  }
  
  const addPressurePoint = (point: PressurePoint) => {
    pressurePoints.value.push(point)
  }
  
  return {
    currentStep,
    device1604Connected,
    pressDeviceConnected,
    selectedChannels,
    pressurePoints,
    channelsSelected,
    hasCollectedData,
    setStep,
    toggleDevice1604,
    togglePressDevice,
    setSelectedChannels,
    addPressurePoint
  }
})
```

**Step 4: Commit**
```bash
git add web/src/components/calibration/
git add web/src/stores/calibration/
git commit -m "feat: add pressure point list, data table and calibration store"
```

---

### Task 11: 实现标定工作台（MainView）

**Files:**
- Modify: `web/src/views/calibration/MainView.vue`

**Step 1: 完整实现MainView**
```vue
<!-- web/src/views/calibration/MainView.vue -->
<template>
  <div class="main-view">
    <div class="calibration-container">
      <!-- 第一行：进度指示器 + 设备面板 -->
      <div class="row row-3">
        <div class="col col-progress">
          <ProgressIndicator :current-step="currentStep" />
        </div>
        <div class="col col-device-1604">
          <Device1604Panel />
        </div>
        <div class="col col-device-press">
          <PressDevicePanel />
        </div>
      </div>
      
      <!-- 第二行：通道选择 + 校准控制 -->
      <div class="row row-2">
        <div class="col col-channels">
          <ChannelMatrix />
        </div>
        <div class="col col-control">
          <CalibrationControlPanel
            :device1604-connected="device1604Connected"
            :press-device-connected="pressDeviceConnected"
            :channels-selected="channelsSelected"
            :has-collected-data="hasCollectedData"
            @start="startCalibration"
            @fit="fitData"
            @end="endCalibration"
          />
        </div>
      </div>
      
      <!-- 第三行：压力点列表 -->
      <div class="row row-full">
        <PressurePointList />
      </div>
      
      <!-- 第四行：数据表格 -->
      <div class="row row-full">
        <CalibrationDataTable :data="calibrationData" :selected-channels="[1, 2, 3, 4, 5]" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import ProgressIndicator from '@/components/calibration/ProgressIndicator.vue'
import Device1604Panel from '@/components/calibration/Device1604Panel.vue'
import PressDevicePanel from '@/components/calibration/PressDevicePanel.vue'
import ChannelMatrix from '@/components/calibration/ChannelMatrix.vue'
import CalibrationControlPanel from '@/components/calibration/CalibrationControlPanel.vue'
import PressurePointList from '@/components/calibration/PressurePointList.vue'
import CalibrationDataTable from '@/components/calibration/CalibrationDataTable.vue'

// 当前步骤
const currentStep = ref(0)

// 设备连接状态（应从store获取）
const device1604Connected = ref(false)
const pressDeviceConnected = ref(false)
const channelsSelected = ref(false)
const hasCollectedData = ref(false)

// 模拟数据
const calibrationData = ref([
  { point: 1, targetPressure: 10, channelData: [10.001, 10.002, 10.000, 10.001, 9.999], status: 'collected' },
  { point: 2, targetPressure: 20, channelData: [20.001, 20.002, 20.000, 20.001, 19.999], status: 'collected' },
  { point: 3, targetPressure: 30, channelData: [30.001, 30.002, 30.000, 30.001, 29.999], status: 'collected' }
])

// 操作
const startCalibration = () => {
  console.log('开始校准')
  currentStep.value = 2
}

const fitData = () => {
  console.log('数据拟合')
  currentStep.value = 4
}

const endCalibration = () => {
  console.log('结束校准')
  currentStep.value = 5
}
</script>

<style scoped lang="scss">
.main-view {
  padding: var(--spacing-lg);
  
  .calibration-container {
    max-width: 1600px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: var(--spacing-lg);
    
    .row {
      display: flex;
      gap: var(--spacing-lg);
      
      &.row-3 {
        .col-progress {
          flex: 5;
        }
        .col-device-1604 {
          flex: 3;
        }
        .col-device-press {
          flex: 4;
        }
      }
      
      &.row-2 {
        .col-channels {
          flex: 7;
        }
        .col-control {
          flex: 5;
        }
      }
      
      &.row-full {
        width: 100%;
      }
      
      .col {
        min-width: 0;
      }
    }
  }
}
</style>
```

**Step 2: 验证标定工作台**
```bash
npm run dev
```
访问 http://localhost:5173/calibration，确认：
- 6步进度指示器显示正常
- 两个设备面板正常
- 通道选择矩阵正常
- 校准控制面板正常
- 压力点列表正常
- 数据表格正常

**Step 3: Commit**
```bash
git add web/src/views/calibration/MainView.vue
git commit -m "feat: implement calibration main view with all components"
```

---

## Phase 5: 最终验证与提交

### Task 12: 运行质量检查

**Step 1: 类型检查**
```bash
cd web && npm run typecheck 2>&1 || echo "Type check completed with issues"
```
Expected: 无严重类型错误

**Step 2: Lint检查**
```bash
cd web && npm run lint 2>&1 || echo "Lint check completed with issues"
```
Expected: 无严重Lint错误

**Step 3: 构建检查**
```bash
cd web && npm run build 2>&1
```
Expected: 构建成功

**Step 4: 最终验证**
```bash
cd web && npm run dev
```
手动验证：
- [ ] 首页三个模块卡片显示正常
- [ ] 计量工作台布局正确
- [ ] 多设备打压布局正确
- [ ] 标定工作台布局正确
- [ ] 深色主题样式一致
- [ ] 导航切换正常

**Step 5: 最终提交**
```bash
git add -A
git commit -m "feat: complete module UI migration - measurement and calibration modules

- Add dark theme CSS variables
- Implement HomeView with module selection
- Implement Measurement CalibrationView with sidebar and data table
- Implement Multi-Pressure WorkbenchView with device cards
- Implement Calibration MainView with 6-step workflow
- Add all necessary components (ProgressIndicator, DevicePanels, ChannelMatrix, etc.)
- Add Pinia stores for state management
- Configure routing for all views"
```

---

## 验收标准

- [x] 深色主题CSS变量文件创建
- [x] 路由配置完成（首页、计量、多设备打压、标定）
- [x] 侧边栏导航组件实现
- [x] 首页三个模块入口卡片
- [x] 计量工作台完整界面（侧边栏、控制条、数据表格）
- [x] 多设备打压完整界面（工具栏、设备卡片网格）
- [x] 标定工作台完整界面（6步进度、设备面板、通道矩阵、控制面板、压力点列表、数据表格）
- [x] 所有组件使用深色主题
- [x] Pinia Store创建（计量模块、标定模块）
- [x] 构建通过无错误

---

## 后续优化建议（V2）

1. **提取公共组件**: DeviceStatusBadge、StatCard等已在common中，后续可提取更多公共组件
2. **响应式优化**: 当前布局适配桌面端，移动端需要额外优化
3. **真实数据接入**: 当前使用模拟数据，后续接入后端API
4. **国际化**: 添加i18n支持
5. **性能优化**: 大数据表格虚拟滚动

---

**计划版本**: v1.0  
**预计耗时**: 4-6小时  
**依赖**: Vue3 + Element Plus已安装
