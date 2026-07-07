# AI 任务上下文加载速查表

> 目标：让 `OpenCode`、`Claude Code` 等 AI Agent 在接到任务后，快速判断应该加载哪些规则文档、项目文档和源码上下文。
> 本文是 `ai-context-loading.zh-CN.md` 的任务级速查表。

## 1. 使用方式

每次任务开始时，先判断任务类型，再按对应顺序加载上下文。

默认前提：

- 启动时已加载 `AGENTS.md`
- 不默认加载整个 `docs/` 树
- 不默认加载所有项目文档
- 任务锁定项目后，才加载项目级入口文档

## 2. 通用加载原则

所有非 trivial 任务都遵循以下最小流程：

1. 读取工作空间入口：`AGENTS.md`
2. 判断任务类型和项目范围
3. 读取对应项目入口：`projects/<name>/AGENTS.md`（若任务已锁定项目）
4. 读取该任务需要的专题文档
5. 读取将修改的源码文件和测试文件
6. 修改前先确认边界，修改后做针对性验证

## 3. 任务类型速查

### 3.1 问“这个代码怎么工作”

典型请求：

- 解释某个模块
- 追踪调用链
- 看某个功能流程
- 理解架构

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. 项目级 `AGENTS.md` / `README.md`
5. 相关源码文件
6. 相关测试或示例

若使用 GitNexus：

- 优先使用 `gitnexus_query`
- 需要符号上下文时使用 `gitnexus_context`

### 3.2 修 bug / 调试失败

典型请求：

- 报错了
- 测试失败
- 设备连接异常
- UI 行为不符合预期

加载顺序：

1. `AGENTS.md`
2. 项目级 `AGENTS.md`
3. 项目级 `README.md` / `CLAUDE.md`
4. 报错输出或失败测试
5. 相关源码文件
6. 相关测试文件
7. `docs/runbooks/development-rules.md`（需要验证策略时）

若涉及硬件或协议：

- 加载对应设备 skill 或设备协议文档
- 加载 `CLAUDE.md` 的硬边界规则

### 3.3 修改后端业务逻辑

典型请求：

- 修改采集逻辑
- 修改校准流程
- 修改设备状态机
- 增加后端 usecase

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. `docs/runbooks/development-rules.md`
5. 项目级 `AGENTS.md` / `CLAUDE.md`
6. 目标 `core/usecase/ports/adapters` 文件
7. 相关测试文件

必须确认：

- 业务规则是否应在 `core/`
- 编排是否应在 `usecase/`
- 外部依赖是否通过 `ports/`
- 具体实现是否应在 `adapters/`

### 3.4 修改硬件适配器 / 设备协议

典型请求：

- 新增设备型号
- 修改串口/TCP 协议
- 调试硬件响应
- 修改采集帧解析

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. 项目级 `AGENTS.md` / `CLAUDE.md`
5. `shared/device-sdk` 或项目 `adapters/hardware` 相关文件
6. 对应设备协议 skill / docs
7. 相关测试或模拟器

必须确认：

- adapter 只做协议翻译和 I/O
- 领域规则不进入 hardware adapter
- 超时、重试、错误恢复明确
- 可通过模拟器或 fake port 测试

### 3.5 修改 Wails backend 绑定

典型请求：

- 给前端新增 Wails 方法
- 修改 Go -> JS bridge
- 调整参数返回结构

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. 项目级 `AGENTS.md` / `CLAUDE.md`
4. `apps/desktop-wails/backend` 目标文件
5. 相关 `usecase` 文件
6. 前端 bridge / api 调用文件

必须确认：

- Wails backend 只做参数转换和委派
- 不写业务 if/else
- 不直接调用硬件

### 3.6 修改前端 UI / 组件

典型请求：

- 改布局
- 改组件
- 优化弹窗/表单/交互
- 做 UI 一致性

加载顺序：

1. `AGENTS.md`
2. 项目级 `AGENTS.md`
3. `docs/runbooks/frontend-ai-rules.zh-CN.md`
4. `docs/runbooks/frontend-directory-rules.zh-CN.md`
5. `docs/architecture/workspace-engineering-rules.zh-CN.md`
6. 项目级 `DESIGN.md`（若存在）
7. 当前组件文件
8. 相似组件文件
9. `docs/runbooks/code-standards.zh-CN.md`（需要编码细则时）

必须确认：

- 前端不承载校准算法或硬件访问
- 组件放在正确目录（ui / layout / domain）
- token 和样式规则不被随意绕过

### 3.7 修改前端状态 / store / API 调用

典型请求：

- 修改 Pinia store
- 修改前端 API client
- 修改 bridge 调用
- 修复页面状态不同步

加载顺序：

1. `AGENTS.md`
2. 项目级 `AGENTS.md`
3. `docs/runbooks/frontend-ai-rules.zh-CN.md`
4. `docs/runbooks/frontend-directory-rules.zh-CN.md`（新增文件时）
5. 项目级 `README.md` / `CLAUDE.md`
6. 相关 store 文件
7. 相关 api / bridge 文件
8. 后端 contract 或 usecase 文件
9. 相关测试

必须确认：

- 组件本地状态不误放进全局 store
- 通用 UI 组件不依赖业务 store
- 前端 API 只调用后端边界，不直接碰硬件

### 3.8 做迁移 / parity 工作

典型请求：

- 从旧项目迁移功能
- 对齐参考 UI
- 做 Cursor DAQ parity
- 分析迁移差异

加载顺序：

1. `AGENTS.md`
2. 项目级 `AGENTS.md`
3. 项目级 `README.md`
4. 项目级 `CLAUDE.md`
5. 项目级 `DESIGN.md`（UI parity 时）
6. `docs/migration/README.md`
7. 对应 migration plan / audit / feature map
8. 目标源码文件

必须确认：

- 不复制旧架构的错误边界
- 迁移的是行为和体验，不是旧实现耦合方式
- 后端行为进入 Go hexagonal 层
- 前端只保留 UI 与交互

### 3.9 重构 / 移动 / 提取共享代码

典型请求：

- 抽 shared 模块
- 移动目录
- 拆文件
- 合并重复逻辑

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/workspace-engineering-rules.zh-CN.md`
4. `docs/architecture/project-variants.md`
5. `docs/runbooks/development-rules.md`
6. `docs/runbooks/frontend-directory-rules.zh-CN.md`（移动前端目录时）
7. 涉及项目的 `AGENTS.md` / `README.md`
8. 相关源码和测试

必须确认：

- 不能让项目互相导入 `internal/*`
- 2 个以上项目复用的逻辑优先考虑 `shared/*`
- 结构变化需考虑 `workspace.structure.json` 和 ADR
- 前端目录变更后跑 `validate-frontend-structure.ps1`

若使用 GitNexus：

- 改符号前跑 impact
- 重命名用 `gitnexus_rename`
- 提交前跑 `gitnexus_detect_changes`

### 3.10 新增项目 / 调整工作空间结构

典型请求：

- 新建项目
- 改顶层目录
- 改 shared 布局
- 调整项目结构变体

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/architecture/project-variants.md`
4. `docs/runbooks/workspace-directory-rules.zh-CN.md`
5. `docs/architecture/workspace-engineering-rules.zh-CN.md`
6. `workspace.structure.json`

必须确认：

- 是否必须使用 `scripts/new-project.ps1`
- 是否需要更新 `workspace.structure.json`
- 是否需要 ADR
- 是否需要运行 `scripts/validate-structure.ps1`

### 3.11 修改文档 / 规则体系

典型请求：

- 写规则文档
- 调整 AGENTS / CLAUDE
- 优化 AI 上下文加载
- 整理文档索引

加载顺序：

1. `AGENTS.md`
2. `docs/index.md`
3. `docs/architecture/ai-context-loading.zh-CN.md`
4. `docs/architecture/ai-document-responsibility-matrix.zh-CN.md`
5. 被修改的目标文档

必须确认：

- 新文档是否有明确职责
- 是否更新 `docs/index.md`
- 是否避免重复定义已有规则
- 是否保持入口文档轻量

## 4. 项目快速映射

### 4.1 Wind-DAQ

项目入口：

- `projects/wind-daq/AGENTS.md`

常用任务文档：

- `projects/wind-daq/README.md`
- `projects/wind-daq/CLAUDE.md`
- `projects/wind-daq/DESIGN.md`
- `projects/wind-daq/docs/migration/README.md`
- `projects/wind-daq/docs/STRUCTURE.md`

### 4.2 DAQ-T-1603

项目入口：

- `projects/daq-t1603/AGENTS.md`

常用任务文档：

- `projects/daq-t1603/README.md`
- `projects/daq-t1603/CLAUDE.md`

### 4.3 DAQ-P-1604

项目入口：

- `projects/daq-p1604/AGENTS.md`

常用任务文档：

- `projects/daq-p1604/README.md`
- `projects/daq-p1604/CLAUDE.md`

### 4.4 Motion Controller

项目入口：

- `projects/motion-controller/AGENTS.md`

常用任务文档：

- `projects/motion-controller/README.md`
- `projects/motion-controller/SPEC.md`
- `projects/motion-controller/PLAN.md`
- `projects/motion-controller/TASKS.md`
- `docs/decisions/ADR-003-shared-motion-control-module.md`

## 5. 最小口诀

可以用下面这句作为 agent 默认规则：

**先读入口，锁定项目；再读专题，最后读代码；不全量加载，不跳过边界。**
