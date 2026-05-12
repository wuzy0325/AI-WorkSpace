# 工作区目录使用规则与场景说明

本文档用于统一说明当前工作区每个主要目录的用途、适用场景和边界规则，避免 AI Agent 或人工开发过程中随意改目录结构。

## 1. 总体规则（必须遵守）

0. 执行流程规范见 `docs/runbooks/ai-agent-execution-standard.zh-CN.md`。
1. 未经明确需求，不得新增、删除、重命名、移动顶层目录。
2. 新建业务项目必须使用 `scripts/new-project.ps1`，不要手工随意搭目录。
3. 完成中大型改动前后，执行 `scripts/validate-structure.ps1`。
4. 若确实要改结构：先给出目录变更清单，再更新 `workspace.structure.json`，并在 `docs/decisions/` 记录原因。
5. 业务核心层（`core`）禁止直接依赖硬件 SDK；硬件依赖只能在 `adapters/hardware` 或 `shared/device-sdk`。

## 2. 顶层目录与文件

| 路径 | 用途 | 典型场景 | 不要做 |
|---|---|---|---|
| `projects/` | 业务项目集合 | 新产品、新客户项目 | 在顶层散落业务代码 |
| `shared/` | 跨项目复用库 | 共通算法、设备 SDK、前端公共模块 | 放项目私有逻辑 |
| `programs/` | 独立小程序 | 标定工具、串口监视器、升级器 | 依赖项目 `internal/*` |
| `device-lab/` | 硬件实验与现场资料 | 接线、抓包、固件、驱动文档 | 直接放业务核心代码 |
| `docs/` | 架构/决策/操作文档 | 设计说明、ADR、运维手册 | 把实现代码放到文档目录 |
| `scripts/` | 自动化脚本 | 结构校验、项目脚手架 | 存放业务逻辑代码 |
| `tools/` | 开发环境工具配置 | Docker、devcontainer | 存放项目实现源码 |
| `AGENTS.md` | AI Agent 统一规则 | OpenCode/Claude Code 行为约束 | 跳过该文件直接开发 |
| `CLAUDE.md` | Claude Code 规则入口 | Claude 行为约束与提醒 | 与 `AGENTS.md` 冲突 |
| `workspace.structure.json` | 结构白名单与必需项 | 校验目录一致性 | 修改后不运行校验 |
| `README.md` | 工作区总入口 | 新成员快速了解 | 长期不更新导致过期 |

## 3. `projects/` 目录（业务项目）

建议每个项目使用一致结构：

### 3.1 `projects/<project>/apps/desktop-tauri`

- 用途：Tauri 桌面应用壳（Vue 3 前端 + Rust shell/命令桥）。
- 场景：桌面 UI 开发、窗口能力、前后端桥接。
- 规则：桌面壳、启动装配和 Tauri 命令桥放这里；核心业务逻辑仍放 `services/api-rs/src/*`。

### 3.2 `projects/<project>/services/api-rs/src/bin`

- 用途：Rust 服务入口（server/worker 等）。
- 场景：启动 API 服务、注册路由、组装依赖。
- 规则：入口层做装配，不写复杂业务逻辑。

### 3.3 `projects/<project>/services/api-rs/src/core`

- 用途：核心领域逻辑（纯业务规则）。
- 场景：计算、判定、流程编排中的领域规则。
- 规则：必须硬件无关、框架无关、易测试。

### 3.4 `projects/<project>/services/api-rs/src/usecase`

- 用途：应用服务层（协调 core 与外部端口）。
- 场景：实现业务用例、事务边界、调用顺序。
- 规则：依赖 `ports` trait，不直接依赖具体硬件实现。

### 3.5 `projects/<project>/services/api-rs/src/ports`

- 用途：对外依赖抽象接口。
- 场景：设备 trait、仓储 trait、消息 trait 定义。
- 规则：trait 保持稳定、与实现解耦。

### 3.6 `projects/<project>/services/api-rs/src/adapters/hardware`

- 用途：硬件协议与设备适配实现。
- 场景：TCP/串口/CAN/Modbus 收发、协议编解码。
- 规则：硬件依赖只放这里；不要把硬件库带进 `core`。

### 3.7 `projects/<project>/services/api-rs/src/adapters/db`

- 用途：数据库适配实现。
- 场景：持久化、查询、事务实现。
- 规则：实现 `ports`，避免把 SQL 细节扩散到 usecase/core。

### 3.8 `projects/<project>/services/api-rs/src/adapters/mq`

- 用途：消息队列适配实现。
- 场景：事件发布、异步消费。
- 规则：消息协议映射放适配层，业务语义留在 usecase/core。

### 3.9 `projects/<project>/contracts/openapi` 与 `contracts/proto`

- 用途：接口契约定义。
- 场景：前后端联调、跨服务通信、SDK 生成。
- 规则：契约变更要可追踪，避免代码先改契约后补。

### 3.10 `projects/<project>/tests/integration` 与 `tests/hil`

- 用途：集成测试与硬件在环测试。
- 场景：接口联调验证、真机回归。
- 规则：`hil` 仅放真机相关测试，不与普通集成测试混放。

### 3.11 `projects/<project>/deploy/dev|staging|prod`

- 用途：环境部署配置。
- 场景：不同环境参数与部署清单。
- 规则：环境差异显式化，不在代码里硬编码环境值。

## 4. `shared/` 目录（跨项目复用）

### 4.1 `shared/algorithms`

- 用途：共通算法库（Rust/TS）。
- 场景：滤波、标定、数据处理、计算模型。
- 规则：算法库应与具体设备驱动解耦。

### 4.2 `shared/device-sdk`

- 用途：共通设备能力与模拟器。
- 场景：协议封装、连接管理、重试策略、模拟设备。
- 规则：统一设备抽象在此沉淀，项目适配层按需组合。

### 4.3 `shared/device-sdk/docs/commands`

- 用途：标准化命令说明（开发契约版）。
- 场景：后端适配实现、模拟器实现、AI 代码生成依据。
- 规则：放“整理后可实现”的文档，不放原始扫描件。

### 4.4 `shared/contracts`

- 用途：跨项目共享契约。
- 场景：多个项目共用同一 API/消息模型。
- 规则：版本兼容策略要明确，避免破坏性变更无提示。

### 4.5 `shared/frontend`

- 用途：前端共享模块（组件、hooks、utils）。
- 场景：多个 Tauri + Vue 3 桌面项目复用 UI 或逻辑。
- 规则：公共模块保持通用，避免携带某项目私有耦合。

## 5. `programs/` 目录（独立小程序）

| 子目录 | 典型用途 | 规则 |
|---|---|---|
| `programs/calibrator-cli` | 校准命令工具 | 可依赖 `shared/*`，不要依赖项目 `internal/*` |
| `programs/serial-monitor` | 串口监视与抓取 | 输出日志格式尽量标准化 |
| `programs/firmware-upgrader` | 固件升级工具 | 升级前后校验和回滚策略要明确 |
| `programs/data-replay` | 数据回放与复现 | 输入输出格式要可回归测试 |

## 6. `device-lab/` 目录（硬件实验区）

### 6.1 `device-lab/rigs`

- 用途：台架清单、接线图、端口映射。
- 场景：新设备接入、现场复现实验。

### 6.2 `device-lab/captures`

- 用途：抓包、串口日志、现场数据记录。
- 场景：协议逆向、故障定位、回放样本。

### 6.3 `device-lab/firmware`

- 用途：固件文件与版本说明。
- 场景：升级验证、回退验证。

### 6.4 `device-lab/drivers`

- 用途：厂商原始资料归档（PDF、Excel、原始说明书）。
- 场景：追溯原始协议来源、核对厂商参数。
- 规则：这是资料库，不是代码契约主来源。

### 6.5 `device-lab/tools`

- 用途：硬件联调辅助脚本。
- 场景：批量诊断、辅助命令发送、现场排障。

## 7. 文档存放建议（你当前场景）

1. 设备原始手册：优先归档到 `device-lab/drivers/...`。
2. 可直接开发的命令规范：维护在 `shared/device-sdk/docs/commands/...`。
3. 某项目特殊覆盖：写到 `projects/<project>/services/api-rs/src/adapters/hardware/docs/...`。

## 8. AI Agent 执行约束（防止乱改结构）

1. 先读 `AGENTS.md`，再动代码。
2. 未经明确指令，不改目录结构。
3. 需要改结构时，先提交“变更清单 + 影响范围”。
4. 改完结构必须更新 `workspace.structure.json` 并通过校验。
5. 若校验失败，优先修复结构问题，不得忽略并结束任务。

## 9. 常用命令

```powershell
# 校验结构是否被意外改动
powershell -File .\scripts\validate-structure.ps1

# 新建项目骨架
powershell -File .\scripts\new-project.ps1 -Name project-gamma
```

## 10. 工程实践补充（参考优秀开源规范）

1. 单一事实来源：目录边界以 `AGENTS.md` + 本文档为准，出现冲突时先更新文档再改代码。
2. 去重优先：同类逻辑在 2 个及以上项目出现时，优先提取到 `shared/*`，禁止长期复制粘贴。
3. 分层清晰：Tauri 桌面壳负责 UI 与命令桥，核心业务逻辑必须留在 `services/api-rs/src/core`。
4. 状态与数据职责分离：前端展示状态在前端层管理，业务规则与设备协议处理在 Rust 后端层管理。
5. 测试就近：核心规则测 `core/usecase`，外部依赖测 `adapters`，真机验证只放 `tests/hil`。
6. 完成前验证：执行结构校验 + 受影响测试，并在结果中附带命令输出信息。
