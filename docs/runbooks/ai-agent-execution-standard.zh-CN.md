# AI Agent 执行标准（中文）

本标准用于约束 OpenCode CLI、Claude Code CLI 等 AI Agent 在本工作区的执行行为，确保目录稳定、分层清晰、可验证交付。

> 本文仅承载 AI 执行 SOP（流程模板）。
> 硬约束、六边形边界、命令清单等权威定义见 [AGENTS.md](../../AGENTS.md) 与 [CLAUDE.md](../../CLAUDE.md)。
> 本文不重复上述文档，仅在涉及流程节点时引用其章节号。

## 1. 适用范围

- 适用于 `AI-Workspace` 下所有项目与共享模块。
- 适用于任何会读写本仓库文件的自动化代理与人工协作开发者。

## 2. 强制底线（不可跳过）

强制底线由 [AGENTS.md §Hard Constraints](../../AGENTS.md) 与 [CLAUDE.md §Hard Constraints](../../CLAUDE.md) 定义，本文不重复。AI 执行流程中必须遵守的关键底线：

1. 开始任务前先读取 `AGENTS.md`。
2. 未经明确要求，不得新增/删除/重命名/移动顶层目录。
3. 新建项目必须使用 `scripts/new-project.ps1`。
4. 完成非 trivial 任务前，必须执行 `scripts/validate-structure.ps1`。
5. **修改任何被 Wails binding 暴露给前端的方法签名后，必须立即在 `apps/desktop-wails` 目录运行 `wails3 generate bindings -silent` 重新生成 `frontend/bindings/`。** typecheck/build/test 全绿不等于 binding 已同步——三套检查都触不到 Go↔JS 运行时桥。详见 [frontend-ai-rules-deploy.zh-CN.md §32.1](frontend-ai-rules-deploy.zh-CN.md#321-wails-绑定同步强制零容忍)。
6. **不得把 typecheck/build/test 全绿当成"运行时安全"的证明。** 涉及 Wails binding、IPC、序列化、协议帧等运行时桥层时，必须在真实环境（桌面应用或真机）触发一次实际调用验证，并在汇报中明确写出验证步骤。

分层与职责（Wails + Vue3 + Go 六边形边界）见 [CLAUDE.md §Architecture](../../CLAUDE.md)。

## 3. 分层与职责（引用 CLAUDE.md）

分层与职责由 [CLAUDE.md §Architecture](../../CLAUDE.md) 定义，本文不重复。简而言之：

- 前端只做显示与交互
- Wails backend 是薄桥接层
- Go 后端采用六边形架构（core → usecase → ports → adapters）

复用优先策略见 [development-rules.md §5](development-rules.md)，本文不重复。

## 4. 执行流程（SOP）

### 4.1 开始前

- 读取并确认规则来源：`AGENTS.md`、`docs/runbooks/workspace-directory-rules.zh-CN.md`。
- 识别任务边界：是否涉及目录结构、共享模块、硬件依赖边界。
- 选择最小改动路径：优先改文件，后改目录。

### 4.2 实施中

- 每次修改都能追溯到任务目标，避免“顺手重构”。
- 新增复用代码时，优先放 `shared/*`。
- 若发现必须改结构，先输出变更清单与影响范围，再实施。

### 4.3 完成前

- 执行结构校验：

```powershell
powershell -File .\scripts\validate-structure.ps1
```

- 执行受影响测试（按层）：core/usecase 单测、adapter 集成测试、必要时 HIL。
- 汇报必须包含实际命令与结果，不得用“应该通过”类表述。

## 5. 结构变更申请模板

当任务必须变更目录结构时，先给出以下内容：

```text
变更清单：
1) 新增目录：...
2) 重命名目录：...
3) 删除目录：...

影响范围：
- 代码路径：...
- 脚本/CI：...
- 文档：...

同步动作：
- 更新 workspace.structure.json: 是/否
- 更新 docs/decisions/: 是/否（文件名）
```

## 6. 完成汇报模板

```text
本次改动：
- ...

验证命令：
- powershell -File .\scripts\validate-structure.ps1
- <受影响测试命令>

验证结果：
- <命令输出关键行>

Wails binding 同步（若本次改了被 binding 暴露的方法/结构体）：
- 已运行：wails3 generate bindings -silent（在 apps/desktop-wails 目录）
- 重新生成文件：frontend/bindings/.../app.js（及受影响 models.js）
- 桌面应用实际调用验证：<手动触发对应功能，确认无 "expects N arguments, got M" 等运行时错误>

未完成项/风险：
- ...
```

## 7. 常见反模式（禁止）

1. 未经确认直接改顶层目录。
2. 在 `core` 直接 import 硬件 SDK 或数据库实现。
3. 复制同一逻辑到多个项目而不抽取共享层。
4. 声称“已完成”但没有任何命令验证结果。
5. 将真机测试与普通集成测试混放。
6. 改 Wails binding 暴露的方法签名后不重新生成 `frontend/bindings/`，仅靠 typecheck/build/test 全绿就交付，导致用户在桌面应用运行时撞到 `expects N arguments, got M` 错误。
7. 把"typecheck/build/test 全绿"当成运行时桥层（Wails binding、IPC、序列化、协议帧）已安全的证明，不在真实环境触发一次实际调用验证。
