# Wind-DAQ 项目配置指南

本文档说明如何在 wind-daq 项目中配置共享模块。

> 当前路径 `projects/motion-controller/shared/frontend/motion/` 是临时项目级共享位置。若 Wind-DAQ 长期复用该模块，应先迁移到工作空间级 `shared/frontend/motion/`，避免产品项目之间相互依赖。

## 1. 修改 tsconfig.json

在 `wind-daq/apps/desktop-wails/frontend/tsconfig.json` 的 `compilerOptions.paths` 中添加：

```json
{
  "compilerOptions": {
    // ... 其他配置
    "paths": {
      "@/*": ["src/*"],
      "@components/*": ["src/components/*"],
      "@views/*": ["src/views/*"],
      "@stores/*": ["src/stores/*"],
      "@api/*": ["src/api/*"],
      "@styles/*": ["src/styles/*"],
      "@shared/*": ["src/shared/*"],
      "@composables/*": ["src/composables/*"],
      "@shared/motion": ["../../../../motion-controller/shared/frontend/motion/src"]
    }
  }
}
```

## 2. 修改 vite.config.ts

在 `wind-daq/apps/desktop-wails/frontend/vite.config.ts` 的 `resolve.alias` 中添加：

```typescript
alias: {
  '@': fileURLToPath(new URL('./src', import.meta.url)),
  '@components': fileURLToPath(new URL('./src/components', import.meta.url)),
  '@views': fileURLToPath(new URL('./src/views', import.meta.url)),
  '@stores': fileURLToPath(new URL('./src/stores', import.meta.url)),
  '@api': fileURLToPath(new URL('./src/api', import.meta.url)),
  '@styles': fileURLToPath(new URL('./src/styles', import.meta.url)),
  '@shared': fileURLToPath(new URL('./src/shared', import.meta.url)),
  '@composables': fileURLToPath(new URL('./src/composables', import.meta.url)),
  '@shared/motion': fileURLToPath(new URL('../../../../motion-controller/shared/frontend/motion/src', import.meta.url)),
},
```

## 3. 使用共享模块

配置完成后，在 wind-daq 项目中可以这样使用：

```typescript
import { useMotionStore, initMotionModule, setMotionApi, getMotionApi } from '@shared/motion';
import MotionControlPanel from '@shared/motion/components/MotionControlPanel.vue';
```

## 4. 初始化示例

```typescript
// 在 main.ts 或 App.vue 中初始化
import { initMotionModule, setMotionApi, setToastService } from '@shared/motion';
import { useFeedbackStore } from '@stores/feedbackStore';

// 设置 Toast 服务
setToastService({
  pushToast: (message: string, type?: 'info' | 'warning' | 'error' | 'success') => {
    const feedback = useFeedbackStore();
    feedback.pushToast(message, type || 'error');
  }
});

// 设置 Motion API（通过 Wails 或 HTTP）
setMotionApi({
  // 实现 IMotionApi 接口的方法
  // ...
});
```
