# AI 渐进式上下文加载协议

> 目标：让 `OpenCode`、`Claude Code` 等 AI Agent 在启动时只加载最小必要规则，在任务明确后按需逐步加载更深层文档，降低上下文噪音，减少误判、过载和风格漂移。

## 1. 适用范围

适用于本工作空间中的以下 AI 工作方式：

- `OpenCode`
- `Claude Code`
- 其他会自动读取工作区 `AGENTS.md` / `CLAUDE.md` / 文档索引的 coding agent

本文不是业务规则文档，而是**AI 如何加载规则文档的协议**。

## 2. 设计目标

上下文加载遵循以下原则：

- 启动轻量：第一次进入工作空间时只读取最小规则集
- 按需扩展：根据任务类型再读取更深层规则
- 任务对齐：不同任务加载不同文档，而不是一股脑全读
- 分层清晰：持久规则、架构规则、项目规则、任务文件分级管理
- 减少重复：同一信息尽量只在一个权威文档中定义

一句话原则：

**先知道边界，再知道细节；先知道去哪找，再按任务去读。**

## 3. 上下文分层

建议把 AI 上下文理解成 5 层：

### Level 1：启动常驻规则

这是 AI 每次进入工作空间时都应优先加载的内容。

目标：

- 明确工作空间是什么
- 明确最硬的边界
- 明确去哪里找详细规则
- 不把启动上下文撑得过大

推荐文件：

- `AGENTS.md`

内容应保持精简，只保留：

- 工作空间概况
- 最重要的架构边界
- 最关键的禁止事项
- 文档导航入口
- 按任务加载协议链接

### Level 2：工作空间总纲规则

当任务涉及架构、边界、代码放置、前后端分离时，再加载这一层。

推荐文件：

- `CLAUDE.md`
- `docs/architecture/workspace-engineering-rules.zh-CN.md`

用途：

- 理解总体架构
- 理解六边形边界
- 理解前后端职责
- 理解工程总体规则

### Level 3：专题规则

当任务进入特定方向时，按主题加载对应文档。

推荐映射：

- 编码规范：`docs/runbooks/code-standards.zh-CN.md`
- 开发/验证规则：`docs/runbooks/development-rules.md`
- 工作空间目录规则：`docs/runbooks/workspace-directory-rules.zh-CN.md`
- 模块设计：`docs/architecture/module-design.md`
- 项目结构变体：`docs/architecture/project-variants.md`
- AI 加载协议：`docs/architecture/ai-context-loading.zh-CN.md`

### Level 4：项目级规则

当任务已经锁定到某个项目时，再加载该项目规则。

推荐文件：

- `projects/<name>/AGENTS.md`
- `projects/<name>/README.md`
- `projects/<name>/CLAUDE.md`（若存在）
- `projects/<name>/DESIGN.md`（若涉及 UI）
- `projects/<name>/docs/*`

用途：

- 理解项目特定约束
- 理解项目构建命令
- 理解项目 UI 或迁移目标

### Level 5：任务级上下文

这是执行具体任务时再加载的内容。

包括：

- 将修改的源码文件
- 相关测试文件
- 相关错误输出
- 相似实现示例
- 该任务涉及的 spec / plan / ADR

## 4. 启动加载协议

### 4.1 启动时只做什么

AI 进入工作空间后，默认只需要完成这些动作：

1. 读取 `AGENTS.md`
2. 识别工作空间类型：Go + Vue 3 + Wails + hexagonal
3. 识别最关键硬边界
4. 识别文档入口与任务映射

启动时**不要求默认完整加载**：

- 所有 runbook
- 所有 architecture 文档
- 所有项目文档
- 所有设计文档

### 4.2 启动时不应做什么

启动时不应：

- 默认完整阅读 `docs/` 全部内容
- 默认完整阅读所有项目的 README
- 默认完整阅读所有项目的设计文档
- 把项目级细节塞进常驻上下文

这会导致：

- 上下文过重
- 不相关规则干扰当前任务
- 更容易遗漏真正相关的边界

## 5. 按任务类型加载协议

### 5.1 纯代码修改任务

最小加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. 目标项目的 `AGENTS.md` / `README.md`
4. 将修改的源码文件
5. 相关测试文件

必要时追加：

- `docs/runbooks/code-standards.zh-CN.md`
- `docs/runbooks/development-rules.md`

### 5.2 架构/放置决策任务

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. `docs/architecture/module-design.md`
5. `docs/architecture/project-variants.md`

### 5.3 前端 UI / 设计任务

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. 项目级 `DESIGN.md`
5. 当前前端源码文件与相似组件

必要时追加：

- `docs/runbooks/code-standards.zh-CN.md`

### 5.4 后端 / 硬件 / 六边形边界任务

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. `docs/runbooks/development-rules.md`
5. 项目级 `CLAUDE.md` / `README.md`
6. 目标 `core/usecase/ports/adapters` 文件

### 5.5 文档 / 规则维护任务

加载顺序：

1. `AGENTS.md`
2. `docs/index.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. 被修改的目标文档

## 6. 给 OpenCode 和 Claude Code 的结构建议

### 6.1 对 `AGENTS.md` 的要求

`AGENTS.md` 应作为**轻量入口文件**，而不是总规则全集。

它应该做：

- 说明工作空间类型
- 列出零容忍边界
- 列出关键命令
- 指向总规则和专题规则
- 指明“按需加载，而不是全量加载”

它不应该做：

- 重复全部编码规范
- 重复全部前端设计细节
- 重复全部项目级特殊规则

### 6.2 对 `CLAUDE.md` 的要求

`CLAUDE.md` 应作为**工作空间总纲**，比 `AGENTS.md` 更详细，但仍应保持结构化。

它适合承载：

- 架构图景
- 边界与决策树
- 设计原则
- 统一约束

不适合承载：

- 每个项目的长篇例外规则
- 过细的 UI token 明细
- 任务执行时才需要的详细 runbook

### 6.3 对项目级文档的要求

项目文档只在进入该项目时加载。

项目级文档应包含：

- 项目目标
- 项目命令
- 项目级边界
- 项目级设计规则
- 项目例外说明

## 7. 推荐文档职责分配

建议工作空间长期保持如下职责边界：

- `AGENTS.md`
  - 启动入口
  - 轻量规则
  - 文档导航

- `CLAUDE.md`
  - 工作空间架构总纲
  - 关键边界
  - 决策树

- `docs/architecture/workspace-engineering-rules.zh-CN.md`
  - 整合版工程规则总览

- `docs/architecture/ai-context-loading.zh-CN.md`
  - AI 渐进式上下文加载协议

- `docs/runbooks/code-standards.zh-CN.md`
  - 编码细则

- `docs/runbooks/development-rules.md`
  - 开发与验证规则

- `projects/<name>/AGENTS.md`
  - 项目级最小入口

- `projects/<name>/DESIGN.md`
  - 项目级 UI / 视觉规则

## 8. 反模式

以下做法会显著降低 AI 输出质量：

### 8.1 启动时全量加载

表现：

- 一进入工作区就要求完整读取 `CLAUDE.md` + 所有 docs + 所有项目 README

问题：

- 上下文过载
- 任务无关信息太多
- 容易忽略真正相关约束

### 8.2 把所有规则塞进一个文件

表现：

- `AGENTS.md`、`CLAUDE.md` 变成巨大规则仓库

问题：

- 难以维护
- 难以更新
- AI 每次启动成本太高

### 8.3 规则重复定义

表现：

- 同一条规则同时写在 `AGENTS.md`、`CLAUDE.md`、项目 README、runbook 中

问题：

- 容易不一致
- AI 无法判断哪一份是权威

## 9. 建议的最小执行协议

如果要把这套协议真正用于日常 AI 工作，推荐默认遵循以下最小流程：

1. 启动时只读 `AGENTS.md`
2. 若任务涉及实现、重构、架构判断，再读 `CLAUDE.md`
3. 若任务属于某专题，再读该专题文档
4. 若任务锁定某项目，再读该项目文档
5. 最后再读实际要改的代码和测试

一句话：

**入口轻，专题深，项目准，代码晚一点读但一定要读。**
