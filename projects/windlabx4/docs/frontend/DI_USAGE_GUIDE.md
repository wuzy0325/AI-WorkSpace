# WINDLABX4 依赖注入系统使用指南

## 概述

WINDLABX4 引入了一个统一的依赖注入（DI）容器来管理应用中的所有服务依赖。这个系统提供了：

- **类型安全**：完整的 TypeScript 类型支持
- **统一访问**：通过 Composables 访问服务
- **返回值适配**：自动处理 `Promise<boolean>` 到 `Promise<void>` 的转换
- **向后兼容**：支持逐步迁移，不破坏现有代码

## 快速开始

### 1. 访问运动控制服务

```typescript
import { useMotionService } from '@/composables/useServices';

const motion = useMotionService();

// 方式 1: 使用适配器（推荐）
// 所有方法返回 Promise<void>
await motion.adapter.connect('controller-1');
await motion.adapter.moveTo('controller-1', 'X', 100);
await motion.adapter.home('controller-1', 'Y');

// 方式 2: 使用 Store
// 所有方法返回 Promise<void>
await motion.store.connect('controller-1');
await motion.store.moveTo('controller-1', 'X', 100);

// 方式 3: 使用原始 API
// 需要检查返回值 Promise<boolean>
const success = await motion.api.connect('controller-1');
if (!success) {
  console.error('连接失败');
}
```

### 2. 访问反馈服务

```typescript
import { useFeedbackService } from '@/composables/useServices';

const feedback = useFeedbackService();

// Toast 消息
feedback.toast.info('这是一条信息提示');
feedback.toast.success('操作成功！');
feedback.toast.warning('这是一个警告', 5000);
feedback.toast.error('发生错误，请重试', 6000);

// 确认对话框
const confirmed = await feedback.confirm('确定要删除这个项目吗？', {
  title: '删除确认',
  confirmText: '删除',
  cancelText: '取消'
});

if (confirmed) {
  // 用户点击了确认
  console.log('用户确认删除');
} else {
  // 用户点击了取消
  console.log('用户取消操作');
}
```

### 3. 使用带错误处理的 Composable

```typescript
import { useMotionWithFeedback } from '@/composables/useServices';

const motion = useMotionWithFeedback({
  showError: true,
  errorPrefix: '移动失败'
});

// 安全执行运动操作，自动处理错误和用户反馈
const moved = await motion.safeMoveTo('controller-1', 'X', 100);
if (moved) {
  console.log('移动成功');
}

// 连接控制
const connected = await motion.safeConnect('controller-1');
if (connected) {
  console.log('连接成功');
}

// 回零操作
const homed = await motion.safeHome('controller-1', 'Y');
if (homed) {
  console.log('回零成功');
}
```

## 架构说明

### 依赖注入容器

容器采用**单例模式**和**延迟初始化**策略：

```
┌─────────────────────────────────────────────┐
│              AppContainer                    │
│  ┌─────────────────────────────────────┐    │
│  │         MotionService                │    │
│  │  ├── api: IMotionApi               │    │
│  │  ├── store: IMotionStore           │    │
│  │  └── adapter: IMotionAdapter       │    │
│  └─────────────────────────────────────┘    │
│  ┌─────────────────────────────────────┐    │
│  │         FeedbackService             │    │
│  │  ├── store: IFeedbackStore         │    │
│  │  └── toast: IToastService          │    │
│  └─────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

### 返回值适配

不同层次的 API 返回不同的类型：

| 层次 | 类型 | 说明 |
|------|------|------|
| **API** | `Promise<boolean>` | 原始 API，需要检查返回值 |
| **Store** | `Promise<void>` | Pinia Store，自动处理 |
| **Adapter** | `Promise<void>` | 类型适配器，简化使用 |

### 使用场景推荐

| 场景 | 推荐使用 | 说明 |
|------|---------|------|
| 简单操作 | `adapter` | 无需检查返回值，代码简洁 |
| 需要状态同步 | `store` | 自动更新 Store 状态 |
| 需要检查结果 | `api` | 需要判断操作是否成功 |
| 组件中使用 | `useMotionService` | 与 Vue 生命周期集成 |
| 服务层中使用 | `container.motion` | 直接访问容器 |

## 迁移指南

### 从旧代码迁移

**旧代码：**
```typescript
import { motionApi } from '@/api/motionApi';
import { useMotionStore } from '@/stores/motionStore';
import { useFeedbackStore } from '@/stores/feedbackStore';

// 运动控制
const motionStore = useMotionStore();
await motionStore.connect('controller-1');

// 反馈
const feedbackStore = useFeedbackStore();
feedbackStore.pushToast('成功', 'success');
```

**新代码：**
```typescript
import { useMotionService, useFeedbackService } from '@/composables/useServices';

// 运动控制
const motion = useMotionService();
await motion.adapter.connect('controller-1');

// 反馈
const feedback = useFeedbackService();
feedback.toast.success('成功');
```

### 渐进式迁移

1. **阶段 1**：在新代码中使用 Composables
2. **阶段 2**：逐步替换旧代码
3. **阶段 3**：移除旧的导入路径

## API 参考

### useMotionService

返回 `IMotionService`，包含：

- `api`: 原始运动控制 API
- `store`: Pinia 运动控制 Store
- `adapter`: 类型适配器

### useFeedbackService

返回 `IFeedbackService`，包含：

- `store`: Pinia 反馈 Store
- `toast`: Toast 消息服务
- `confirm()`: 确认对话框方法

### useMotionWithFeedback

返回增强版的运动控制服务，额外包含：

- `safeMoveTo()`: 带错误处理的移动方法
- `safeHome()`: 带错误处理的回零方法
- `safeConnect()`: 带错误处理的连接方法

## 调试支持

在开发环境下，可以访问全局容器对象：

```typescript
// 在浏览器控制台中
window.__WINDLABX4_CONTAINER__.motion
window.__WINDLABX4_CONTAINER__.feedback
```

## 最佳实践

1. **优先使用 Composables**：在 Vue 组件中始终使用 Composables
2. **使用适配器**：除非需要检查返回值，否则使用 adapter
3. **错误处理**：使用 `useMotionWithFeedback` 进行统一的错误处理
4. **类型安全**：充分利用 TypeScript 类型检查
5. **单一职责**：每个组件只导入需要的服务

## 常见问题

### Q: 为什么不直接使用 motionStore？

A: motionStore 仍然可以使用，但建议通过 `useMotionService().store` 访问。这样可以：
- 保持统一的访问模式
- 更容易切换实现
- 支持依赖注入测试

### Q: 如何处理 Pinia store id 冲突？

A: 容器内部已经处理了这个问题。如果需要显式指定 store ID，可以使用 `storeIds.ts` 中定义的新 ID（如 `WINDLABX4-motion`）。

### Q: 如何测试使用了 DI 的组件？

A: 可以 mock 容器或使用依赖注入框架（如 Vue Test Utils 的 `global.plugins`）。

## 下一步

- 查看 [迁移指南](./MIGRATION_GUIDE.md) 了解详细的迁移步骤
- 查看 [类型定义](../core/types.ts) 了解完整的 API
- 查看 [容器实现](../core/container.ts) 了解架构细节
