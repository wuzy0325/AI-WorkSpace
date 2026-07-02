# AI Agent 执行标准（中文）

本标准用于约束 OpenCode CLI、Claude Code CLI 等 AI Agent 在本工作区的执行行为，确保目录稳定、分层清晰、可验证交付。

## 1. 适用范围

- 适用于 `AI-Workspace` 下所有项目与共享模块。
- 适用于任何会读写本仓库文件的自动化代理与人工协作开发者。

## 2. 强制底线（不可跳过）

1. 开始任务前先读取 `AGENTS.md`。
2. 未经明确要求，不得新增/删除/重命名/移动顶层目录。
3. 新建项目必须使用 `scripts/new-project.ps1`。
4. 核心业务层 `projects/*/services/api-go/internal/core` 禁止直接依赖硬件库。
5. 硬件依赖只允许在 `adapters/hardware` 或 `shared/device-sdk`。
6. 完成非 trivial 任务前，必须执行 `scripts/validate-structure.ps1`。
7. **修改任何被 Wails binding 暴露给前端的方法签名后，必须立即在 `apps/desktop-wails` 目录运行 `wails3 generate bindings -silent` 重新生成 `frontend/bindings/`。** typecheck/build/test 全绿不等于 binding 已同步——三套检查都触不到 Go↔JS 运行时桥。详见 `docs/runbooks/frontend-ai-rules.zh-CN.md` 第 13.1 节。
8. **不得把 typecheck/build/test 全绿当成"运行时安全"的证明。** 涉及 Wails binding、IPC、序列化、协议帧等运行时桥层时，必须在真实环境（桌面应用或真机）触发一次实际调用验证，并在汇报中明确写出验证步骤。

## 3. 分层与职责（Wails + Vue3 + Go）

- `projects/*/apps/desktop-wails/frontend`：桌面前端（Vue 3），不直接依赖硬件驱动与数据库适配。
- `projects/*/apps/desktop-wails/backend`：Wails 宿主与绑定层，只做 app shell glue。
- `projects/*/services/api-go/internal/core`：核心业务规则，纯业务、可单测、硬件无关。
- `projects/*/services/api-go/internal/usecase` + `ports`：编排流程和依赖抽象。
- `projects/*/services/api-go/internal/adapters/*`：硬件/数据库/消息等外部依赖实现。

## 4. 复用优先策略

1. 同类逻辑在 2 个及以上项目出现，必须优先抽取到 `shared/*`。
2. 共通设备协议/传输代码放 `shared/device-sdk`。
3. 共通算法放 `shared/algorithms`。
4. 共通 Vue 组件与 composables 放 `shared/frontend`。
5. 禁止在多个项目长期复制粘贴同一实现。

## 5. 执行流程（SOP）

### 5.1 开始前

- 读取并确认规则来源：`AGENTS.md`、`docs/runbooks/workspace-directory-rules.zh-CN.md`。
- 识别任务边界：是否涉及目录结构、共享模块、硬件依赖边界。
- 选择最小改动路径：优先改文件，后改目录。

### 5.2 实施中

- 每次修改都能追溯到任务目标，避免“顺手重构”。
- 新增复用代码时，优先放 `shared/*`。
- 若发现必须改结构，先输出变更清单与影响范围，再实施。

### 5.3 完成前

- 执行结构校验：

```powershell
powershell -File .\scripts\validate-structure.ps1
```

- 执行受影响测试（按层）：core/usecase 单测、adapter 集成测试、必要时 HIL。
- 汇报必须包含实际命令与结果，不得用“应该通过”类表述。

## 6. 结构变更申请模板

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

## 7. 完成汇报模板

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

## 8. 常见反模式（禁止）

1. 未经确认直接改顶层目录。
2. 在 `core` 直接 import 硬件 SDK 或数据库实现。
3. 复制同一逻辑到多个项目而不抽取共享层。
4. 声称“已完成”但没有任何命令验证结果。
5. 将真机测试与普通集成测试混放。
6. 改 Wails binding 暴露的方法签名后不重新生成 `frontend/bindings/`，仅靠 typecheck/build/test 全绿就交付，导致用户在桌面应用运行时撞到 `expects N arguments, got M` 错误。
7. 把"typecheck/build/test 全绿"当成运行时桥层（Wails binding、IPC、序列化、协议帧）已安全的证明，不在真实环境触发一次实际调用验证。
