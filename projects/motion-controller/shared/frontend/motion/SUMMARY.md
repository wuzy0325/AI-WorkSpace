# 共享 Motion 模块 - 完成总结

## 已完成的工作

### 1. 创建共享模块结构
在 `motion-controller/shared/frontend/motion/` 目录下创建了完整的共享模块结构：

```
motion-controller/shared/frontend/motion/
├── package.json
├── tsconfig.json
├── WIND_DAQ_CONFIG.md  # Wind-DAQ 配置指南
└── src/
    ├── index.ts         # 模块入口
    ├── init.ts          # 初始化示例
    ├── types/motion.ts  # 类型定义
    ├── stores/
    │   ├── index.ts
    │   ├── motionApi.ts  # API 接口定义
    │   └── motionStore.ts # Pinia Store
    ├── i18n/
    │   ├── index.ts
    │   └── motion.ts     # 国际化翻译
    └── components/
        ├── index.ts
        └── MotionControlPanel.vue # 运动控制组件
```

### 2. 修复 i18nStore 重复属性错误
移除了 `motion-controller/apps/desktop-wails/frontend/src/stores/i18nStore.ts` 中重复的 `moving` 和 `idle` 属性定义。

### 3. 恢复 motion-controller 项目配置
暂时回退了 motion-controller 的集成，以保持项目正常运行。可以在未来逐步迁移到共享模块。

## 需要手动完成的步骤

### 对于 Wind-DAQ 项目

由于权限限制，需要在 Wind-DAQ 项目中手动完成以下配置：

#### 步骤 1: 更新 tsconfig.json
在 `wind-daq/apps/desktop-wails/frontend/tsconfig.json` 中添加路径别名：

```json
{
  "compilerOptions": {
    // ... 其他配置
    "paths": {
      "@/*": ["src/*"],
      // ... 其他现有配置
      "@shared/motion": ["../../../../motion-controller/shared/frontend/motion/src"]
    }
  }
}
```

#### 步骤 2: 更新 vite.config.ts
在 `wind-daq/apps/desktop-wails/frontend/vite.config.ts` 中添加 Vite 别名：

```typescript
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      // ... 其他现有配置
      '@shared/motion': fileURLToPath(new URL('../../../../motion-controller/shared/frontend/motion/src', import.meta.url)),
    },
  },
  // ... 其他配置
})
```

#### 步骤 3: 集成共享模块（可选）
在 `wind-daq/apps/desktop-wails/frontend/src/main.ts` 或 `App.vue` 中初始化共享模块：

```typescript
import { setMotionApi, setToastService } from '@shared/motion'
import { motionApi } from '@api/motionApi'
import { useFeedbackStore } from '@stores/feedbackStore'

// 配置 Toast 服务
setToastService({
  pushToast: (message, type = 'error') => {
    const feedback = useFeedbackStore()
    feedback.pushToast(message, type)
  }
})

// 配置 Motion API
setMotionApi(motionApi)
```

#### 步骤 4: 使用共享组件
在 Wind-DAQ 项目的视图中使用共享组件：

```vue
<script setup lang="ts">
import MotionControlPanel from '@shared/motion/components/MotionControlPanel.vue'
</script>

<template>
  <MotionControlPanel />
</template>
```

## 设计考虑

### 共享模块的优势
1. **代码复用** - Motion 控制器的核心逻辑只需维护一次
2. **一致性** - 两个项目的 Motion 控制界面和行为保持一致
3. **可维护性** - 更新只需修改一个地方

### 依赖注入设计
共享模块使用依赖注入设计模式：
- `setMotionApi()` - 注入项目特定的 Motion API 实现
- `setToastService()` - 注入项目特定的 Toast 通知服务
- 这样可以适应不同项目的技术栈（Wails、HTTP API 等）

### 未来扩展
1. 可以在 motion-controller 中也逐步迁移到共享模块
2. 可以添加更多共享组件（如校准组件、遍历组件等）
3. 如果 Wind-DAQ 长期复用该模块，应迁移到工作空间级 `shared/frontend/motion/`，避免 Wind-DAQ 依赖 motion-controller 产品目录
4. 可以考虑将共享模块打包成独立的 npm 包
