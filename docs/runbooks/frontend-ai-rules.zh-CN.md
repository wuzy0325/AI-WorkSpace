# AI 前端开发规则

> 本文档已按 token 预算拆分为 4 个主题文件（见 [ai-context-loading.zh-CN.md §3](../architecture/ai-context-loading.zh-CN.md) L3 单文档 ≤ 8,000 tokens 规则）。
> 章节级精读协议见 [ai-context-loading.zh-CN.md §10](../architecture/ai-context-loading.zh-CN.md#10-章节级精确加载规范)。

## 完整目录

### 第一层：架构与边界（→ [foundation](frontend-ai-rules-foundation.zh-CN.md)）

- [§1 适用范围](frontend-ai-rules-foundation.zh-CN.md#1-适用范围)
- [§2 AI 执行流程](frontend-ai-rules-foundation.zh-CN.md#2-ai-执行流程)
- [§3 前端职责边界](frontend-ai-rules-foundation.zh-CN.md#3-前端职责边界)
- [§4 代码放置规则](frontend-ai-rules-foundation.zh-CN.md#4-代码放置规则)
- [§5 UI 设计规则](frontend-ai-rules-foundation.zh-CN.md#5-ui-设计规则)
- [§6 控件使用规则](frontend-ai-rules-foundation.zh-CN.md#6-控件使用规则)
- [§7 布局和响应式规则](frontend-ai-rules-foundation.zh-CN.md#7-布局和响应式规则)
- [§8 可访问性规则](frontend-ai-rules-foundation.zh-CN.md#8-可访问性规则)
  - [§8.1 焦点陷阱与 ARIA live regions](frontend-ai-rules-foundation.zh-CN.md#81-焦点陷阱与-aria-live-regions)
- [§9 模块边界与依赖方向](frontend-ai-rules-foundation.zh-CN.md#9-模块边界与依赖方向)
- [§10 文件与 import 组织规范](frontend-ai-rules-foundation.zh-CN.md#10-文件与-import-组织规范)

### 第二层：状态与数据（→ [state](frontend-ai-rules-state.zh-CN.md)）

- [§11 状态完整性规则](frontend-ai-rules-state.zh-CN.md#11-状态完整性规则)
  - [§11.1 共享 Store 边界规则](frontend-ai-rules-state.zh-CN.md#111-共享-store-边界规则)
- [§12 响应式数据规范](frontend-ai-rules-state.zh-CN.md#12-响应式数据规范)
- [§13 Provide/Inject 与依赖注入](frontend-ai-rules-state.zh-CN.md#13-provideinject-与依赖注入)
- [§14 Pinia store 间依赖](frontend-ai-rules-state.zh-CN.md#14-pinia-store-间依赖)
- [§15 状态机与复杂状态管理](frontend-ai-rules-state.zh-CN.md#15-状态机与复杂状态管理)
- [§16 表单规则](frontend-ai-rules-state.zh-CN.md#16-表单规则)
- [§17 图表和实时数据规则](frontend-ai-rules-state.zh-CN.md#17-图表和实时数据规则)

### 第三层：编码质量（→ [quality](frontend-ai-rules-quality.zh-CN.md)）

- [§18 TypeScript 严格性规范](frontend-ai-rules-quality.zh-CN.md#18-typescript-严格性规范)
- [§19 命名规范](frontend-ai-rules-quality.zh-CN.md#19-命名规范)
- [§20 注释规范与代码自文档化](frontend-ai-rules-quality.zh-CN.md#20-注释规范与代码自文档化)
- [§21 函数与复杂度量化规则](frontend-ai-rules-quality.zh-CN.md#21-函数与复杂度量化规则)
- [§22 错误处理规范](frontend-ai-rules-quality.zh-CN.md#22-错误处理规范)
- [§23 输入校验与防御性编程](frontend-ai-rules-quality.zh-CN.md#23-输入校验与防御性编程)
- [§24 性能规范](frontend-ai-rules-quality.zh-CN.md#24-性能规范)
- [§25 i18n 规范](frontend-ai-rules-quality.zh-CN.md#25-i18n-规范)
- [§26 生命周期与资源清理](frontend-ai-rules-quality.zh-CN.md#26-生命周期与资源清理)
- [§27 并发与竞态处理](frontend-ai-rules-quality.zh-CN.md#27-并发与竞态处理)

### 第四层：样式与集成（→ [deploy](frontend-ai-rules-deploy.zh-CN.md)）

- [§28 样式和 token 规则](frontend-ai-rules-deploy.zh-CN.md#28-样式和-token-规则)
  - [§28.1 文件体量与逻辑分离量化红线](frontend-ai-rules-deploy.zh-CN.md#281-文件体量与逻辑分离量化红线)
  - [§28.2 硬编码颜色与 CSS 字面量禁令](frontend-ai-rules-deploy.zh-CN.md#282-硬编码颜色与-css-字面量禁令)
- [§29 资源管理](frontend-ai-rules-deploy.zh-CN.md#29-资源管理)
- [§30 构建配置](frontend-ai-rules-deploy.zh-CN.md#30-构建配置)
- [§31 面向对象与领域模型](frontend-ai-rules-deploy.zh-CN.md#31-面向对象与领域模型)
- [§32 Wails 前后端桥接规范](frontend-ai-rules-deploy.zh-CN.md#32-wails-前后端桥接规范)
  - [§32.1 Wails 绑定同步（强制，零容忍）](frontend-ai-rules-deploy.zh-CN.md#321-wails-绑定同步强制零容忍)
- [§33 测试规范](frontend-ai-rules-deploy.zh-CN.md#33-测试规范)
- [§34 验证要求](frontend-ai-rules-deploy.zh-CN.md#34-验证要求)
- [§35 AI 友好的代码组织](frontend-ai-rules-deploy.zh-CN.md#35-ai-友好的代码组织)

## 章节级精读

AI 加载本文档体系时遵循 [ai-context-loading.zh-CN.md §10](../architecture/ai-context-loading.zh-CN.md#10-章节级精确加载规范) 章节级精读协议：根据任务关键词命中表（§10.4）只加载对应章节，不整篇加载。
