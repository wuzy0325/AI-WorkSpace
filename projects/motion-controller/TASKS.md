# 任务执行计划：运动控制模块共享

## 任务概述

将 `motion-controller` 和 `wind-daq` 两个项目的运动控制逻辑统一共享，并逐步收敛 UI 组件共享边界。

当前状态：Go 应用级共享模块位于 `shared/motion-control/go`；前端共享模块仍位于 `projects/motion-controller/shared/frontend/motion`，属于临时项目级共享位置。

---

## 任务清单

### Task 1：确认架构范围
- [x] 确认 DAQ 设备层不迁移
- [x] 确认只共享运动控制层

### Task 2：创建前端共享模块基础结构
- [x] 创建目录 `projects/motion-controller/shared/frontend/motion/`
- [x] 初始化 `package.json`
- [x] 配置 `tsconfig.json`

### Task 3：提取类型定义
- [x] 从 `motion-controller/apps/desktop-wails/frontend/src/shared/types/motion.ts` 复制
- [x] 保存到 `projects/motion-controller/shared/frontend/motion/src/types/motion.ts`

### Task 4：提取 Pinia Store
- [x] 创建 `projects/motion-controller/shared/frontend/motion/src/stores/motionApi.ts`（API 接口定义）
- [x] 创建 `projects/motion-controller/shared/frontend/motion/src/stores/motionStore.ts`（Store 实现）
- [x] 使用依赖注入解耦项目特定逻辑

### Task 5：提取国际化翻译
- [x] 创建 `projects/motion-controller/shared/frontend/motion/src/i18n/motion.ts`
- [x] 包含中英文翻译

### Task 6：提取 Vue 组件
- [x] `MotionControlPanel.vue` - 运动控制面板组件

### Task 7：创建导出入口
- [x] 创建 `projects/motion-controller/shared/frontend/motion/src/index.ts`
- [x] 创建 `projects/motion-controller/shared/frontend/motion/src/init.ts`

### Task 8：更新 motion-controller 项目
- [x] 创建初始化示例 `init.ts`

### Task 9：更新 wind-daq 项目
- [x] 确认 wind-daq 已有完整的运动控制实现
- [x] 两个项目各自保持独立的前端组件（UI 布局不同）

---

## 已创建的文件清单

```
shared/frontend/motion/
├── package.json
├── tsconfig.json
└── src/
    ├── index.ts              # 模块入口
    ├── init.ts               # 初始化示例
    ├── types/
    │   └── motion.ts        # 类型定义
    ├── stores/
    │   ├── index.ts         # Store 导出
    │   ├── motionApi.ts     # API 接口定义
    │   └── motionStore.ts   # Store 实现
    ├── i18n/
    │   ├── index.ts         # i18n 导出
    │   └── motion.ts        # 翻译文本
    └── components/
        ├── index.ts         # 组件导出
        └── MotionControlPanel.vue  # 运动控制面板
```

---

## 依赖关系

```
Task 1 ──┐
         ├──→ Task 2 ──→ Task 3 ──→ Task 4 ──→ Task 5 ──→ Task 6 ──→ Task 7 ──→ Task 8
         └──→ Task 9
```

---

## 使用方法

### 1. 在项目中使用共享模块

```typescript
// main.ts 或入口文件
import { initMotionModule } from '@shared/motion/init';
initMotionModule();
```

### 2. 使用共享的 Store

```typescript
import { useMotionStore } from '@shared/motion';

const motionStore = useMotionStore();
await motionStore.refreshProfiles();
```

### 3. 使用共享的组件

```vue
<script setup>
import { MotionControlPanel } from '@shared/motion';
</script>

<template>
  <MotionControlPanel />
</template>
```

### 4. 使用共享的 i18n

```typescript
import { motionZh, motionEn } from '@shared/motion';

const t = computed(() => locale.value === 'zh' ? motionZh : motionEn);
```

---

## 验证方法

### 验证 motion-controller
1. `cd projects/motion-controller/apps/desktop-wails && go test ./...`
2. `cd projects/motion-controller/apps/desktop-wails && go build -buildvcs=false ./...`
3. `cd projects/motion-controller/services/api-go && go test ./...`
4. `cd projects/motion-controller/apps/desktop-wails/frontend && npm run typecheck && npm run build`
5. 运行 `wails dev`，确认运动控制面板正常显示并可执行基础操作。

### 验证 wind-daq
1. `cd projects/wind-daq/services/api-go && go test ./...`
2. `cd projects/wind-daq/apps/desktop-wails && go build -buildvcs=false ./...`
3. `cd projects/wind-daq/apps/desktop-wails/frontend && npm run typecheck && npm run build`
4. 确认 wind-daq 使用共享 Go motion 模块时不导入 `projects/motion-controller/*`。

---

## 注意事项

1. **保持向后兼容**：修改时注意不破坏现有功能
2. **依赖最小化**：共享模块只依赖通用库
3. **中文注释**：所有新增文件使用中文注释
4. **两个项目独立**：wind-daq 和 motion-controller 保持各自的 UI 布局
