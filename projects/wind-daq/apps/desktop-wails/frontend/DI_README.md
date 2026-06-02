# WindDAQ 依赖注入系统

## 快速开始

### 基本使用

```typescript
import { useMotionService, useFeedbackService } from '@/composables/useServices';

// 运动控制
const motion = useMotionService();
await motion.adapter.moveTo('controller-1', 'X', 100);

// 反馈
const feedback = useFeedbackService();
feedback.toast.success('操作成功');
```

### 查看文档

- [使用指南](./docs/DI_USAGE_GUIDE.md) - 详细的使用说明
- [迁移指南](./docs/MIGRATION_GUIDE.md) - 从旧代码迁移的步骤
- [实现总结](./docs/DI_IMPLEMENTATION_SUMMARY.md) - 技术实现细节

## 核心功能

### 1. 运动控制服务

```typescript
// 基础使用
const motion = useMotionService();

// 使用适配器（推荐）
await motion.adapter.connect('id');
await motion.adapter.moveTo('id', 'X', 100);

// 带错误处理
import { useMotionWithFeedback } from '@/composables/useServices';
const safe = useMotionWithFeedback();
await safe.safeMoveTo('id', 'X', 100);
```

### 2. 反馈服务

```typescript
const feedback = useFeedbackService();

// Toast 消息
feedback.toast.info('信息');
feedback.toast.success('成功');
feedback.toast.warning('警告');
feedback.toast.error('错误');

// 确认对话框
const confirmed = await feedback.confirm('确定吗？');
```

## 架构

```
┌─────────────────────────────────┐
│      Vue Composables            │
│  (useMotionService, ...)       │
└──────────────┬──────────────────┘
               │
               ▼
┌─────────────────────────────────┐
│      AppContainer               │
│  (依赖注入容器，单例模式)          │
└──────────────┬──────────────────┘
               │
       ┌───────┴───────┐
       ▼               ▼
┌─────────────┐  ┌─────────────┐
│  Motion     │  │  Feedback   │
│  Service    │  │  Service    │
└─────────────┘  └─────────────┘
```

## 文件结构

```
src/
├── core/                 # 核心模块
│   ├── container.ts      # 依赖注入容器
│   ├── types.ts          # 类型定义
│   ├── storeIds.ts       # Store ID 管理
│   └── index.ts          # 导出
├── composables/          # Composables
│   └── useServices.ts    # 服务 Hooks
└── docs/                 # 文档
    ├── DI_USAGE_GUIDE.md
    ├── MIGRATION_GUIDE.md
    └── DI_IMPLEMENTATION_SUMMARY.md
```

## 开发

### 类型检查

```bash
npm run typecheck
```

### 构建

```bash
npm run build
```

### 调试

在浏览器控制台访问：

```javascript
window.__WINDDAQ_CONTAINER__.motion
window.__WINDDAQ_CONTAINER__.feedback
```

## 迁移状态

✅ **已完成**：
- 依赖注入容器实现
- 类型适配器
- Composables 层
- 文档

🚧 **进行中**：
- 逐步迁移现有组件
- 添加单元测试

## 贡献指南

1. 在 Vue 组件中使用 Composables
2. 优先使用 `adapter` 而不是直接使用 `api`
3. 使用 `useMotionWithFeedback` 进行错误处理
4. 保持向后兼容

## 许可证

与 WindDAQ 项目相同
