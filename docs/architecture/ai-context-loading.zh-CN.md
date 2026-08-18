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

建议把 AI 上下文理解成 5 层。每层带 token 预算（按中文估算：1 字符 ≈ 0.6 token，英文 1 词 ≈ 1.3 token）。预算是**单次加载上限**，超标必须改用章节级精读（见 §10）或拆分文档。

### Level 1：启动常驻规则

这是 AI 每次进入工作空间时都应优先加载的内容。

- **token 预算：≤ 2,000**（约 3,000 中文字符，主体段；自动注入的工具段如 GitNexus 不计入，按需加载）
- 超标处理：砍历史细节、砍示例代码、改为"详见 §X"链接

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

- **token 预算：≤ 6,000**（CLAUDE.md ≤ 4,000 + workspace-engineering-rules ≤ 2,000）
- 超标处理：CLAUDE.md 砍设计原则长篇、workspace-engineering-rules 改纯导航

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

- **token 预算：单文档 ≤ 8,000；同任务最多加载 2 个专题文档**
- 超标处理：用 §10 章节级精读替代整文档加载；超 8k 的文档必须拆分

推荐映射：

- 编码规范：`docs/runbooks/code-standards.zh-CN.md`
- 前端开发规则：`docs/runbooks/frontend-ai-rules.zh-CN.md`（薄索引，已拆分为 4 个主题文件：foundation / state / quality / deploy，每个 ≤ 8k）
- 前端目录结构：`docs/runbooks/frontend-directory-rules.zh-CN.md`
- 开发/验证规则：`docs/runbooks/development-rules.md`
- 工作空间目录规则：`docs/runbooks/workspace-directory-rules.zh-CN.md`
- Windows Go 已知问题：`docs/runbooks/go-windows-known-issues.zh-CN.md`
- 模块设计：`docs/architecture/module-design.md`
- 项目结构变体：`docs/architecture/project-variants.md`
- AI 加载协议：`docs/architecture/ai-context-loading.zh-CN.md`

### Level 4：项目级规则

当任务已经锁定到某个项目时，再加载该项目规则。

- **token 预算：项目 AGENTS ≤ 3,000；项目 README + CLAUDE 合计 ≤ 6,000**
- 超标处理：项目 AGENTS 删重复 workspace 规则、改"详见 ../../AGENTS.md §X"

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

- **token 预算：源码 + 测试 ≤ 10,000；spec/plan/ADR 按需**
- 超标处理：用 Grep/SearchCodebase 替代全文件 Read；大文件用 offset/limit 分段读

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
3. `docs/runbooks/frontend-ai-rules.zh-CN.md`
4. `docs/runbooks/frontend-directory-rules.zh-CN.md`
5. `docs/architecture/workspace-engineering-rules.zh-CN.md`
6. 项目级 `DESIGN.md`
7. 当前前端源码文件与相似组件

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

### 5.6 前端目录 / 文件结构变更任务

典型请求：

- 新增页面目录
- 新建前端组件目录
- 重构前端文件组织
- 创建新前端项目

加载顺序：

1. `AGENTS.md`
2. `CLAUDE.md`
3. `docs/runbooks/frontend-directory-rules.zh-CN.md`
4. `docs/runbooks/frontend-ai-rules.zh-CN.md`
5. `docs/architecture/workspace-engineering-rules.zh-CN.md`
6. 项目级 `DESIGN.md`（涉及 UI 时）

必须确认：

- 目录位置符合 frontend-directory-rules
- 不破坏已有加载约定（api/ 与 bridge/ 二选一、pages/ 与 views/ 二选一）
- 新增文件放在正确分层（ui / layout / domain）
- 修改后运行 `validate-frontend-structure.ps1`

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

## 10. 章节级精确加载规范

### 10.1 为什么需要章节级加载

L3 专题文档（如拆分前的 `frontend-ai-rules.zh-CN.md` 1541 行 ≈ 9k token）整篇加载会迅速耗尽预算。章节级加载以**单个 §X 章节为最小单元**，只读必要部分，节省 60-80% token。拆分后每个文件 ≤ 8k，整篇加载也可接受，但章节级精读仍更高效。

### 10.2 锚点引用规范

外部文档引用 L3 文档时**必须带锚点**，禁止"详见 frontend-ai-rules.zh-CN.md"这种无章节引用。

锚点格式：`<文件名>#<章节号-章节标题slug>`

- 章节号用数字（含小数点）：`32`、`32.1`
- slug 规则：中文保留、空格转 `-`、`/` 转 `-`、其他特殊字符删除
- 例：`§32.1 Wails 绑定同步` → `#321-wails-绑定同步强制零容忍`

正确示例：

```markdown
详见 [frontend-ai-rules-deploy.zh-CN.md §32.1](frontend-ai-rules-deploy.zh-CN.md#321-wails-绑定同步强制零容忍)。
```

错误示例：

```markdown
详见 frontend-ai-rules.zh-CN.md。              // ❌ 无章节号
详见 frontend-ai-rules.zh-CN.md §32.1。        // ❌ 无锚点，AI 仍需整篇加载
```

### 10.3 AI 章节级加载流程

AI 收到带锚点的引用后，按以下步骤精读：

1. 用 Grep 在目标文档中定位章节标题（如 `^## 32\.1 ` 或 `^### 32\.1 `）
2. Read 该章节起点 + 后续内容，遇到下一个同级或上级 `##` 标题即停
3. 估算实际读取 token，若超 §3 预算的 50% 则停下重新评估
4. 仅在确认需要上下文时才向上回溯（读父章节 §32 再读 §32.1）

### 10.4 关键词 → 章节触发器表

AI 收到任务后先扫描关键词，命中即精读对应章节，跳过未命中文档：

| 任务关键词 | 必读章节（精读） | 选读章节 |
|---|---|---|
| wails binding / wails 绑定 | frontend-ai-rules-deploy §32.1 | code-standards §五 |
| pinia / store / 状态管理 | frontend-ai-rules-state §11-§14 | workspace-engineering-rules §2 |
| 硬件协议 / SCPI / 串口 / TCP | CLAUDE.md Hard Constraints | daq-pressure-devices skill |
| CSV 录制 / recorder / sink | project_memory §6 | wispa CLAUDE.md |
| 设备时间戳 / 硬件时间戳 | project_memory §9.6-§9.10 | frontend-ai-rules-state §17 |
| 六边形 / hexagonal / 边界 | CLAUDE.md Hard Constraints | workspace-engineering-rules §2 |
| 命名规范 / naming | code-standards §二 | frontend-ai-rules-quality §19 |
| 注释规范 / comment | code-standards §三 | frontend-ai-rules-quality §20 |
| TypeScript 严格性 / any | frontend-ai-rules-quality §18 | code-standards §四 |
| 错误处理 / catch / try | frontend-ai-rules-quality §22 | code-standards §四 |
| 性能 / 性能优化 / 卡顿 | frontend-ai-rules-quality §24 | development-rules §6 |
| 生命周期 / onMounted / onBeforeUnmount | frontend-ai-rules-quality §26 | — |
| 并发 / 竞态 / goroutine / channel | frontend-ai-rules-quality §27 | development-rules §6 |
| 状态机 / state machine | frontend-ai-rules-state §15 | — |
| 表单 / form / 校验 | frontend-ai-rules-state §16, frontend-ai-rules-quality §23 | — |
| 图表 / chart / 实时数据 | frontend-ai-rules-state §17 | — |
| i18n / 国际化 / 多语言 | frontend-ai-rules-quality §25 | project_memory §16 |
| 文件体量 / 行数 / 拆分 | frontend-ai-rules-deploy §28.1 | code-standards §一 |
| 硬编码颜色 / token / design token | frontend-ai-rules-deploy §28.2 | — |
| 测试 / test / 单元测试 | frontend-ai-rules-deploy §33 | development-rules §3 |
| 验证 / verify / 提交前检查 | frontend-ai-rules-deploy §34 | AGENTS.md Pre-submit Checklist |
| release / 发布 / 打包 | release-versioning.zh-CN.md | ADR-004 |
| motion / 运动 / 编码器补偿 | project_memory §12 | shared/device-sdk/go/motion |
| WTNMC4A / 脉冲模式 / CP/DIR | project_memory §18 | shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go |
| syso / 图标 / rsrc | project_memory §7 | ADR-004 |

### 10.5 关键词触发器执行规则

1. AI 收到任务后先扫描用户输入和系统 reminder 中的关键词
2. 命中即按"必读章节"列精读对应章节（带锚点）
3. "选读章节"仅在必读章节信息不足时才加载
4. 未命中关键词时按 §5 任务类型加载协议走
5. 同一任务最多精读 5 个章节，超出必须重新评估任务范围

### 10.6 维护规则

新增章节时，必须同步更新：
- 本表（§10.4）添加关键词映射
- `docs/index.md` 快速定位段添加章节引用
- 外部文档引用该章节时必须带锚点

## 11. token 预算自检协议

### 11.1 AI 启动自检流程

AI 每次加载文档后必须自检：

```
1. 累计已加载 token = Σ(各文档 token 估算)
2. 对照 §3 各层预算：
   - L1 启动: ≤ 2k
   - L2 总纲: ≤ 6k
   - L3 专题（单文档）: ≤ 8k
   - L4 项目: ≤ 6k（README + CLAUDE）
   - L5 任务: ≤ 10k（源码 + 测试）
3. 若超标：
   - L1/L2 超标 → 删历史细节、改"详见 §X"链接
   - L3 超标 → 改用 §10 章节级精读
   - L4 超标 → 项目 AGENTS 删重复 workspace 规则
   - L5 超标 → 用 Grep 替代全文件 Read
4. 自检结果计入汇报模板："本次加载 X tokens，预算 Y tokens，超标/达标"
```

### 11.2 估算方法

中文：1 字符 ≈ 0.6 token（含标点）
英文：1 词 ≈ 1.3 token
代码：1 行 ≈ 5-10 token（视复杂度）

简化估算：`token ≈ 字符数 × 0.5`（中英混合平均）

精确估算：跑 `scripts/estimate-token.ps1`（见 §11.4）

### 11.3 超标处理决策树

```
文档超 8k token？
├─ 是 → 该文档是否已被章节级引用？
│       ├─ 是 → 用 §10.3 流程精读对应章节
│       └─ 否 → 文档需拆分（开 issue 跟踪）
└─ 否 → 整文档加载 OK

L1 启动超 2k？
├─ 是 → AGENTS.md 需瘦身（删示例、删历史、改链接）
└─ 否 → OK

L4 项目 AGENTS 超 3k？
├─ 是 → 检查是否重复 workspace 规则
│       ├─ 是 → 删重复段，改"详见 ../../AGENTS.md §X"
│       └─ 否 → 项目确有大量例外，开 issue 评估拆分
└─ 否 → OK
```

### 11.4 估算脚本

`scripts/estimate-token.ps1` 提供精确估算：

```powershell
# 估算单文件
powershell -File .\scripts\estimate-token.ps1 -Path AGENTS.md

# 估算目录下所有 .md
powershell -File .\scripts\estimate-token.ps1 -Path docs/runbooks -Recurse

# 与预算对照
powershell -File .\scripts\estimate-token.ps1 -Path AGENTS.md -Budget 2000
```

脚本输出：文件名、字符数、估算 token、预算、达标/超标、超标量。

### 11.5 自检汇报模板

AI 完成任务汇报时必须包含加载预算段：

```markdown
## 上下文加载汇报

| 文档 | token 估算 | 预算 | 状态 |
|---|---|---|---|
| AGENTS.md | 1,850 | 2,000 | ✅ |
| frontend-ai-rules §32.1 | 480 | 8,000 | ✅ |
| motion-controller/AGENTS.md | 2,100 | 3,000 | ✅ |
| **合计** | **4,430** | — | ✅ |

未加载：frontend-ai-rules 整篇（已用 §10 章节级精读替代）
```

## 12. 一句话版本

这套协议可以压缩成一句话：

**入口轻，专题深，项目准，代码晚一点读但一定要读；超预算就改章节级精读，别整篇加载。**
