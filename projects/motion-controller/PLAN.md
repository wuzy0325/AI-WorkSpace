# 运动控制模块共享架构计划

## 1. 目标

实现 `motion-controller` 和 `wind-daq` 两个项目的：
- ✅ **底层运动控制设备代码统一共享**（shared/device-sdk/go/motion）
- ✅ **应用级运动控制逻辑统一共享**（shared/motion-control/go）
- 📋 **前端 UI 组件逐步共享**（当前仍在 projects/motion-controller/shared/frontend/motion，后续应迁移到工作空间级 shared/frontend/motion）
- ✅ **国际化翻译统一**
- ✅ **设备连接和状态管理一致**

---

## 2. 当前架构现状

### 2.1 后端现状

#### shared/device-sdk/go/motion/
```
motion/
├── core/
│   ├── errors.go          ✅ 错误定义
│   └── types.go           ✅ 核心类型（Axis, Profile, Status 等）
├── ports/
│   └── motion.go          ✅ MotionController 接口
└── adapters/
    ├── config/
    │   └── file_config.go ✅ 文件配置存储
    └── hardware/
        ├── factory.go     ✅ 工厂类
        └── simulated_motion.go ✅ 模拟控制器实现
```

#### projects/motion-controller/
```
services/api-go/
└── internal/
    └── usecase/
        └── motion.go      📋 运动控制器用例

apps/desktop-wails/
├── backend/
│   └── motion_*.go        📋 Motion 控制器服务
└── frontend/src/
    ├── components/
    │   └── MotionControlPanel.vue  📋 运动控制面板
    ├── stores/
    │   └── motion.ts      📋 motionStore
    └── i18n/
        └── modules/
            └── motion.ts  📋 运动控制翻译
```

#### projects/wind-daq/
```
services/api-go/
└── internal/
    └── core/
        └── motion/        📋 运动控制用例

apps/desktop-wails/
└── frontend/src/          ❌ 暂无运动控制 UI
```

---

## 3. 目标架构

```
shared/
├── device-sdk/go/
│   └── motion/            ✅ 运动控制后端（已存在）
│       ├── core/
│       ├── ports/
│       └── adapters/
│
├── motion-control/go/     ✅ 应用级 motion 共享模块
└── frontend/motion/       📋 建议目标：工作空间级前端共享模块
    ├── package.json
    ├── tsconfig.json
    └── src/
        ├── components/
        │   ├── MotionControlPanel.vue
        │   ├── AxisConfigCard.vue
        │   ├── AxisControlGroup.vue
        │   ├── MotionConnectionPanel.vue
        │   └── MotionConfigDialog.vue
        ├── stores/
        │   └── motionStore.ts
        ├── types/
        │   └── motion.ts
        ├── i18n/
        │   └── motion.ts
        └── index.ts       📋 入口导出

projects/
├── motion-controller/
│   └── apps/desktop-wails/frontend/
│       └── package.json   📋 更新：引用 @shared/motion
│
└── wind-daq/
    └── apps/desktop-wails/frontend/
        └── package.json   📋 更新：引用 @shared/motion
```

---

## 4. 实施步骤

### 步骤 1：创建前端共享模块基础
- [ ] 创建目录 `projects/motion-controller/shared/frontend/motion/`
- [ ] 初始化 `package.json`
- [ ] 配置 `tsconfig.json`
- [ ] 配置 `vite.config.ts`（可选，用于构建）

### 步骤 2：提取类型定义
- [ ] 从 `motion-controller/apps/desktop-wails/frontend/src/types/motion.ts` 提取到 `shared/frontend/motion/src/types/motion.ts`

### 步骤 3：提取 Pinia Store
- [ ] 从 `motion-controller/apps/desktop-wails/frontend/src/stores/motion.ts` 提取到 `shared/frontend/motion/src/stores/motionStore.ts`
- [ ] 调整 store 依赖（去掉项目特定逻辑）

### 步骤 4：提取国际化翻译
- [ ] 从 `motion-controller/apps/desktop-wails/frontend/src/i18n/modules/motion.ts` 提取到 `shared/frontend/motion/src/i18n/motion.ts`

### 步骤 5：提取 Vue 组件
- [ ] `MotionControlPanel.vue` → `shared/frontend/motion/src/components/`
- [ ] `AxisControlGroup.vue` → `shared/frontend/motion/src/components/`
- [ ] `AxisConfigCard.vue` → `shared/frontend/motion/src/components/`
- [ ] `MotionConnectionPanel.vue` → `shared/frontend/motion/src/components/`
- [ ] `MotionConfigDialog.vue` → `shared/frontend/motion/src/components/`
- [ ] `MotionView.vue` → `shared/frontend/motion/src/components/`（可选）

### 步骤 6：创建导出入口
- [ ] 创建 `shared/frontend/motion/src/index.ts`，统一导出所有内容

### 步骤 7：更新 motion-controller 项目
- [ ] 更新 `package.json`，添加 workspace 依赖
- [ ] 替换原文件引用为 `@shared/motion`

### 步骤 8：更新 wind-daq 项目
- [ ] 更新 `package.json`，添加 workspace 依赖
- [ ] 在 wind-daq 中集成运动控制模块

---

## 5. 文件清单

### 5.1 后端（已完成）

| 文件 | 路径 | 状态 |
|------|------|------|
| 运动控制核心类型 | [shared/device-sdk/go/motion/core/types.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/core/types.go) | ✅ |
| 运动控制接口 | [shared/device-sdk/go/motion/ports/motion.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/ports/motion.go) | ✅ |
| 模拟控制器 | [shared/device-sdk/go/motion/adapters/hardware/simulated_motion.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/simulated_motion.go) | ✅ |
| 配置存储 | [shared/device-sdk/go/motion/adapters/config/file_config.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/config/file_config.go) | ✅ |
| 工厂类 | [shared/device-sdk/go/motion/adapters/hardware/factory.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/factory.go) | ✅ |

### 5.2 前端（项目级临时共享，待迁移）

当前前端共享模块位于 `projects/motion-controller/shared/frontend/motion/`。如果 `wind-daq` 需要长期复用，应作为后续任务迁移到工作空间级 `shared/frontend/motion/`，避免一个产品项目被另一个产品项目导入。

| 文件 | 目标路径 | 源文件 | 状态 |
|------|---------|--------|------|
| 类型定义 | `projects/motion-controller/shared/frontend/motion/src/types/motion.ts` | [motion-controller/apps/desktop-wails/frontend/src/types/motion.ts](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/types/motion.ts) | 📋 |
| Pinia Store | `projects/motion-controller/shared/frontend/motion/src/stores/motionStore.ts` | [motion-controller/apps/desktop-wails/frontend/src/stores/motion.ts](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/stores/motion.ts) | 📋 |
| i18n 翻译 | `projects/motion-controller/shared/frontend/motion/src/i18n/motion.ts` | [motion-controller/apps/desktop-wails/frontend/src/i18n/modules/motion.ts](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/i18n/modules/motion.ts) | 📋 |
| MotionControlPanel | `projects/motion-controller/shared/frontend/motion/src/components/MotionControlPanel.vue` | [motion-controller/apps/desktop-wails/frontend/src/components/MotionControlPanel.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/components/MotionControlPanel.vue) | 📋 |
| AxisControlGroup | `projects/motion-controller/shared/frontend/motion/src/components/AxisControlGroup.vue` | [motion-controller/apps/desktop-wails/frontend/src/components/AxisControlGroup.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/components/AxisControlGroup.vue) | 📋 |
| AxisConfigCard | `projects/motion-controller/shared/frontend/motion/src/components/AxisConfigCard.vue` | [motion-controller/apps/desktop-wails/frontend/src/components/AxisConfigCard.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/components/AxisConfigCard.vue) | 📋 |
| MotionConnectionPanel | `projects/motion-controller/shared/frontend/motion/src/components/MotionConnectionPanel.vue` | [motion-controller/apps/desktop-wails/frontend/src/components/MotionConnectionPanel.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/components/MotionConnectionPanel.vue) | 📋 |
| MotionConfigDialog | `projects/motion-controller/shared/frontend/motion/src/components/MotionConfigDialog.vue` | [motion-controller/apps/desktop-wails/frontend/src/components/MotionConfigDialog.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/motion-controller/apps/desktop-wails/frontend/src/components/MotionConfigDialog.vue) | 📋 |
| 导出入口 | `projects/motion-controller/shared/frontend/motion/src/index.ts` | - | 📋 |

---

## 6. 使用示例

### 6.1 后端使用

```go
// motion-controller 或 wind-daq 的 usecase
import (
	"shared/device-sdk/go/motion/ports"
	"shared/device-sdk/go/motion/adapters/hardware"
	"shared/device-sdk/go/motion/core"
)

func createMotionController(profile core.Profile) ports.MotionController {
	factory := hardware.NewFactory()
	return factory.Create(profile)
}
```

### 6.2 前端使用

```vue
<script setup lang="ts">
import { useMotionStore, MotionControlPanel, type MotionProfile } from '@shared/motion'

const motionStore = useMotionStore()
</script>

<template>
  <MotionControlPanel />
</template>
```

---

## 7. 注意事项

1. **后端用例层独立**：`motion-controller` 和 `wind-daq` 的 `usecase` 层保留项目特定逻辑，只共享底层设备驱动
2. **UI 布局灵活**：共享组件提供插槽，允许不同项目自定义布局
3. **依赖隔离**：共享模块只依赖通用库（Vue 3, Pinia, i18n），不依赖项目特定代码
4. **版本兼容**：共享模块需保持向后兼容
