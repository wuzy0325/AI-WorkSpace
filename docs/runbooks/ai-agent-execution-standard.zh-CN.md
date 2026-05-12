# AI Agent 执行标准（中文）

本标准用于约束 OpenCode CLI、Claude Code CLI 等 AI Agent 在本工作区的执行行为，确保目录稳定、分层清晰、可验证交付。

## 1. 适用范围

- 适用于 `AI-Workspace` 下所有项目与共享模块。
- 适用于任何会读写本仓库文件的自动化代理与人工协作开发者。

## 2. 强制底线（不可跳过）

1. 开始任务前先读取 `AGENTS.md`。
2. 未经明确要求，不得新增/删除/重命名/移动顶层目录。
3. 新建项目必须使用 `scripts/new-project.ps1`。
4. 核心业务层 `projects/*/services/api-rs/src/core` 禁止直接依赖硬件库、文件 I/O、网络、串口、数据库、Web 框架或 Tauri 类型。
5. 硬件依赖只允许在 `adapters/hardware` 或 `shared/device-sdk`。
6. 完成非 trivial 任务前，必须执行 `scripts/validate-structure.ps1`。

## 3. 分层与职责（Tauri + Vue 3 + Rust）

- `projects/*/apps/desktop-tauri/frontend`：桌面前端（Vue 3），只负责展示和交互，不直接依赖硬件驱动、数据库适配或校准算法。
- `projects/*/apps/desktop-tauri/src-tauri`：Tauri 桌面壳与薄命令桥，只做窗口生命周期、系统能力和桥接，不写业务逻辑。
- `projects/*/services/api-rs/src/core`：核心业务规则，纯业务、可单测、硬件无关。
- `projects/*/services/api-rs/src/usecase` + `ports`：编排流程和依赖抽象，依赖 trait 而非具体实现。
- `projects/*/services/api-rs/src/adapters/*`：硬件/数据库/消息等外部依赖实现。

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

未完成项/风险：
- ...
```

## 8. 常见反模式（禁止）

1. 未经确认直接改顶层目录。
2. 在 `core` 直接 import 硬件 SDK 或数据库实现。
3. 复制同一逻辑到多个项目而不抽取共享层。
4. 声称“已完成”但没有任何命令验证结果。
5. 将真机测试与普通集成测试混放。
