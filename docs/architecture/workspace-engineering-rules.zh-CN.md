# 工作空间工程规则总览

> 本文是工作空间工程规则的**导航中枢与快速决策表**，不重复其他文档的详细规则。
> 详细规则由各权威文档承载（见 §4 文档导航）。
> 本文作用：
> - 在不知道读哪份文档时，给出快速入口
> - 在"代码该放哪"时，给出快速决策表
> - 在规则冲突时，给出优先级

---

## 1. 工作空间总体架构

本工作空间采用 Go 后端（六边形架构）+ Vue 3 前端 + Wails 桌面壳 + 多项目组织方式。

典型结构：`projects/<name>/apps/desktop-wails/{frontend,backend}` 或 `projects/<name>/services/api-go/internal/{core,usecase,ports,adapters}`。小型独立工具（如 `wista`）允许单模块变体，但必须保留 `core/usecase/ports/adapters` 逻辑边界。

一句话概括：**前端负责显示与交互，后端负责业务与控制，六边形架构负责隔离变化。**

详细架构总纲与硬约束见 [CLAUDE.md](../../CLAUDE.md)；项目结构变体见 [project-variants.md](project-variants.md)。

---

## 2. 前后端职责边界（快速判断）

- **前端**：展示、用户交互、页面状态、组件组合、图表与可视化、输入校验界面反馈
- **后端**：业务规则、领域模型、采集流程编排、校准算法、设备控制、配置持久化、报告生成、与硬件/存储/网络交互
- **Wails 边界**：`apps/desktop-wails/backend/` 是薄桥接层，只做参数转换 + 调用 usecase + 返回前端友好结果，**禁止**写业务逻辑或直接访问硬件

判断口诀：**如果把 Vue UI 替换成 Web 页面、命令行或别的前端，这段逻辑仍然成立，那它应属于 Go 后端。**

详细前后端分离规则见 [CLAUDE.md §Architecture](../../CLAUDE.md) 与 [development-rules.md §8](../runbooks/development-rules.md)。

---

## 3. 代码放置快速决策表

### 3.1 后端代码该放哪

| 代码类型 | 放置位置 |
|---|---|
| 纯业务规则或算法 | `core/` |
| 编排多个对象与外部接口 | `usecase/` |
| 外部依赖抽象（接口定义） | `ports/` |
| 具体设备、存储、扫描、文件实现 | `adapters/` |
| API 或 Wails 入参出参转换 | `api/` 或 `backend/` |

### 3.2 前端代码该放哪

| 代码类型 | 放置位置 |
|---|---|
| 通用 UI 控件 | `components/ui/` |
| 复用界面模式 | `components/patterns/` 或 `composables/` |
| 业务领域组件 | `components/<domain>/` |
| 页面拼装 | `pages/` 或 `views/` |
| 主题、颜色、间距、动效 | `styles/tokens/` 与 `styles/themes/` |
| API 调用封装 | `api/` 或 `bridge/` |
| Pinia 状态 | `stores/` |

详细前端目录规则见 [frontend-directory-rules.zh-CN.md](../runbooks/frontend-directory-rules.zh-CN.md)。

### 3.3 共享代码该放哪

`shared/device-sdk`（硬件协议/传输）、`shared/algorithms`（算法）、`shared/frontend`（Vue 组件/composables）、`shared/motion-control`（运动控制）。

---

## 4. 文档导航

| 主题 | 权威文档 |
|---|---|
| 启动入口 | [AGENTS.md](../../AGENTS.md) |
| 架构总纲、硬约束、决策树 | [CLAUDE.md](../../CLAUDE.md) |
| 文档索引（完整列表） | [docs/index.md](../index.md) |
| 通用编码规范 | [code-standards.zh-CN.md](../runbooks/code-standards.zh-CN.md) |
| 开发流程规则 | [development-rules.md](../runbooks/development-rules.md) |
| 前端 AI 规则 | [frontend-ai-rules.zh-CN.md](../runbooks/frontend-ai-rules.zh-CN.md) |
| 前端目录结构 | [frontend-directory-rules.zh-CN.md](../runbooks/frontend-directory-rules.zh-CN.md) |
| 工作空间目录规则 | [workspace-directory-rules.zh-CN.md](../runbooks/workspace-directory-rules.zh-CN.md) |
| AI 执行 SOP | [ai-agent-execution-standard.zh-CN.md](../runbooks/ai-agent-execution-standard.zh-CN.md) |
| 发布版本规范 | [release-versioning.zh-CN.md](../runbooks/release-versioning.zh-CN.md) |
| 模块设计 | [module-design.md](module-design.md) |
| 项目结构变体 | [project-variants.md](project-variants.md) |
| AI 上下文加载协议 | [ai-context-loading.zh-CN.md](ai-context-loading.zh-CN.md) |
| AI 文档职责矩阵 | [ai-document-responsibility-matrix.zh-CN.md](ai-document-responsibility-matrix.zh-CN.md) |
| 任务→文档映射 | [ai-task-context-map.zh-CN.md](ai-task-context-map.zh-CN.md) |

---

## 5. 规则优先级

冲突时按优先级：用户明确要求 > 项目级强约束（`projects/<name>/CLAUDE.md` / `AGENTS.md`）> 工作空间 [CLAUDE.md](../../CLAUDE.md) > 专题规则（`docs/runbooks/*` / `docs/architecture/*`）> 项目辅助文档（README / DESIGN）。

同一规则多处出现时：入口文档只保留摘要与链接，专题文档保留完整定义，项目文档只补充特例。

---

## 6. 良好架构判断标准

| 维度 | 判断标准 |
|---|---|
| 边界清楚 | 能快速判断代码该放哪里；改前端不误伤核心业务；换设备实现不改核心算法 |
| 可替换 | UI 可换但业务不丢；硬件适配器可换但 usecase 不重写；仿真器与真实设备共用接口契约 |
| 可测试 | core 不依赖真实硬件可测；usecase 可通过 fake/mock port 测；前端基础 UI 可单测 |
| 可维护 | 新人能看懂目录；同类控件不反复造轮子；改一个规则不导致全局样式散乱 |

---

## 7. 落地建议

先立边界 → 再定规则 → 再做结构 → 先把新代码写对 → 再逐步整理旧代码。复用出现两次评估抽象，出现三次优先抽离。
