# Workspace Win7 LTS Worktree Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task. All product code changes occur in `..\AI-Workspace-win7`; do not modify mainline product code unless the user separately requests a mainline bug fix.

**Goal:** 在不改造当前 `AI-Workspace` 主工作空间的前提下，在长期分支 worktree `AI-Workspace-win7` 中为 6 个维护产品构建 Windows 7 SP1 x64 版本，并通过 AI 定期选择性同步主线修复。

**Architecture:** `master` 保持 Go 1.25+/Wails/WebView2；`lts/win7` 固定 Go 1.20.14/Electron 22.3.27/HTTP。两个产品线共享 Git 历史但不整体 merge，AI 根据同步台账审查主线提交，使用 `cherry-pick -x` 或人工兼容移植同步业务修复。

**Tech Stack:** Git worktree、Go 1.20.14、Electron 22.3.27、Chromium 108、Vue 3/Vite、HTTP JSON、SSE/WebSocket/快照轮询、NSIS、PowerShell。

---

## 1. Scope

### Included

| Order | Product | Migration approach |
|---|---|---|
| 1 | `daq-t1603` | 保留并固化已通过 Win7 真机验证的实现 |
| 2 | `probe-interpolator` | 纯算法/文件项目，优先验证通用 Electron 壳和 dialog |
| 3 | `daq-p1604` | 复用 T1603 的 HTTP/Electron 模式和现有快照缓存 |
| 4 | `1604Cal` | 复用现有 HTTP/SSE；先解决嵌套 Git 仓库所有权 |
| 5 | `motion-controller` | 复用现有 HTTP API；重点验证 DLL、急停和限位 |
| 6 | `wind-daq` | 复用完整 HTTP/SSE API；最后迁移并做硬件矩阵验收 |

### Excluded

- 已被 `probe-interpolator` 取代的 `three-hole-interpolator` 和 `five-hole-interpolator`。
- 仅作诊断/应急的 `daq-t1603-win7-python`。
- 默认不属于 GUI 产品线的 `programs/*`。

## 2. Mandatory Boundaries

1. `AI-Workspace/` 主线产品代码零侵入；只允许保存 ADR、计划和导航文档。
2. `AI-Workspace-win7/` 必须保持为同一 Git 仓库的 worktree，不复制为独立仓库。
3. Win7 分支固定 Go 1.20.14、Electron 22.3.27 和 Windows 7 SP1 x64。
4. 禁止整体 merge `master`，禁止目录级文件覆盖。
5. 通用 bug 先在主线修复；Win7 只做回移或兼容实现。
6. 每次同步必须更新 `WIN7-SYNC-STATE.md`。
7. 每次发布必须重新打包、安装后 smoke test；设备/DLL 改动必须真机验证。

## Phase 0: Freeze the Verified Baseline

### Task 0: Audit the dirty worktrees and commit governance documents

**Workspaces:** `AI-Workspace` and `..\AI-Workspace-win7`

**Baseline facts recorded when this plan was written:**

- Win7 worktree HEAD 为 `b041033`，它只是历史起点，不包含已验证的 Win7 实现。
- Win7 working tree 当前约有 36 个 Git 状态项，主要位于 DAQ-T1603 和 `shared/device-sdk`；其中包括源码、lockfile、文档和未跟踪的 Electron 壳。
- 已验证 Electron 源位于 `projects/daq-t1603/apps/desktop-electron/`，生成的 `node_modules/`、`backend/` 和 `dist/` 已由该目录 `.gitignore` 排除。

**Actions:**

1. 确认主工作空间的 ADR-007、ADR-008、本计划、文档索引和 README 导航已经进入 Git 历史；若尚未提交，先单独提交治理文档。
2. 在 Win7 worktree 用 `git status --short`、`git diff --stat`、`git diff` 和未跟踪文件清单逐项分类。
3. 分类为：DAQ-T1603 Win7 源码、Go 1.20 兼容依赖、生成产物、无关用户改动、待确认文件。
4. 不自动 stash、迁移、还原或删除无法确认所有权的改动；发现真正无关改动时先请求用户决定。
5. 把分类结果写入 `WIN7-BASELINE-INVENTORY.md`，记录文件组、用途、是否纳入基线和理由。

**Acceptance criteria:**

- 主工作空间治理文档可由具体 commit SHA 定位，执行时不再依赖未跟踪文档。
- 每个 Win7 状态项都有分类，不能只凭路径或数量判断是否无关。
- Electron 壳、HTTP/WebSocket、前端 transport 和 Go 1.20 shim 的具体来源路径已记录。
- 未经用户确认，没有 stash、回滚或移动既有改动。

### Task 1: Commit the DAQ-T1603 verified implementation

**Workspace:** `..\AI-Workspace-win7`

**Actions:**
1. 以 Task 0 的 `WIN7-BASELINE-INVENTORY.md` 为准，仅选择已确认属于 DAQ-T1603 Win7 基线的文件。
2. 执行 Go 1.20、前端、Electron 和 NSIS 验证。
3. 只提交源码、lockfile、构建脚本和文档；排除 `node_modules/`、`dist/` 和 backend 构建产物。
4. 创建已验证基线 tag，例如 `win7-baseline-daq-t1603-0.3.3`。

**Acceptance criteria:**
- worktree 干净。
- tag 指向能重建已验证安装包的提交。
- 安装包 SHA256 和 Win7 验证结果已记录。

### Task 2: Promote the branch to workspace Win7 LTS

**Actions:**
1. 确认没有其他 worktree 使用目标分支名。
2. 将 `feature/daq-t1603-win7` 重命名为 `lts/win7`。
3. 更新 `WIN7-PROGRESS.md`、README 和同步台账中的分支名。

**Verification:**
- `git worktree list`
- `git branch --show-current` 返回 `lts/win7`
- `git status --short` 无输出

### Checkpoint 0

- DAQ-T1603 基线可从干净 clone/worktree 重建。
- Windows 7 SP1 x64 安装与启动仍通过。
- 后续同步有明确起点。

## Phase 1: Establish AI Synchronization

### Task 3: Create and validate the synchronization ledger

**Files in Win7 worktree:**
- Maintain: `WIN7-SYNC-STATE.md`
- Create: `docs/runbooks/win7-sync-runbook.md`

**Ledger fields:**
- `Last reviewed master`
- `Current Win7 head`
- `Last review date`
- 已直接 cherry-pick 的提交
- 已人工移植的提交
- 排除及原因
- 待确认及优先级
- 每次同步的验证结果

### Task 4: Add an AI-assisted review script

**Files in Win7 worktree:**
- Create: `scripts/review-win7-sync.ps1`

**Behavior:**
- 读取台账中的主线审查点。
- 输出 `<last-reviewed>..master` 的提交和改动文件，不自动修改代码。
- 按项目路径分组。
- 标记 Wails、Go 版本、依赖和文档-only 提交。
- 生成候选报告供 AI 审查，不自动 cherry-pick。

**Acceptance criteria:**
- 脚本只读且可重复运行。
- 错误的或不存在的基线 SHA 会立即失败。
- 不把“无冲突”当作“兼容 Win7”。

### Task 5: Define synchronization commands

**Direct port:**

```powershell
cd ..\AI-Workspace-win7
git status --short
git cherry-pick -x <master-sha>
```

**Manual port commit message:**

```text
fix(<scope>): port <fix> to Win7

Ported from master commit <sha>.
Excluded platform-only changes: <reason>.
```

**Rollback:**

```powershell
git cherry-pick --abort
```

已完成且已提交的错误回移统一使用 `git revert <win7-commit-sha>`，无论是否推送都保留审计历史。不得使用 `git reset --hard`、覆盖主线文件或重写共享历史来解决同步问题。

### Checkpoint 1

- AI 能从台账和 Git 历史识别未审查提交。
- 至少演练一次直接回移和一次“排除平台升级”的记录流程。
- 同步审查本身不修改主工作空间。

## Phase 2: Reusable Win7 Platform Inside the LTS Branch

### Task 6: Extract the verified Electron shell only in Win7 branch

**Workspace:** `..\AI-Workspace-win7`

**Target files:**
- `shared/desktop/electron-win7/`
- `shared/desktop/go/localserver/`
- `shared/desktop/frontend/transport/`

**Actions:**
1. 从 `projects/daq-t1603/apps/desktop-electron/` 的已验证实现提取 Electron 进程、preload、health、dialog 和 electron-builder NSIS 配置。
2. 共享层只包含平台能力，不包含 DAQ、运动、探针或校准领域类型。
3. 产品通过 manifest 配置名称、版本、图标、backend、窗口和安装包名称。
4. 参考 `projects/three-hole-interpolator/apps/desktop-win7/` 的 `net/http`、嵌入 SPA 和 handler 组织经验；该项目虽已废弃，但保留为实现参考，不恢复其独立产品线。

**Acceptance criteria:**
- DAQ-T1603 使用共享壳后行为不变。
- backend 启动失败、超时、异常退出和应用退出均能清理进程。
- Electron 强制 `contextIsolation: true`、`sandbox: true`、`nodeIntegration: false`、`webSecurity: true`，且不能被产品 manifest 关闭。

### Task 7: Add Win7 build and artifact scripts

**Files in Win7 worktree:**
- Create: `config/win7-products.json`
- Create: `scripts/build-win7-products.ps1`
- Create: `scripts/verify-win7-artifacts.ps1`

**Behavior:**
- 支持 `-Project <name>`，后续支持 `-All`。
- 固定 `GOROOT`、`GOWORK=off`、`GOTOOLCHAIN=local`。
- 输出到 `builds/win7/<product>/<version>/`。
- 生成包含 Git SHA、版本、Go、Electron、文件大小和 SHA256 的 manifest。
- 每个产品提供 electron-builder 产品配置（内置 NSIS target 或必要时的自定义 NSIS include）；不强制创建独立 `.nsi` 文件。

**Acceptance criteria:**

- `verify-win7-artifacts.ps1` 校验 SHA256、非零文件大小、PE x64 架构、安装包命名、版本和签名状态。
- 没有代码签名证书时，manifest 必须明确记录 `unsigned`，不能把“未签名”误报为校验通过的签名。
- 每个迁移任务完成时都能生成对应产品的 NSIS 安装包并执行静默安装 smoke test。

### Checkpoint 2

- 共享平台改动只存在于 Win7 分支。
- DAQ-T1603 可以使用统一脚本重建和安装。
- 主线目录没有 Electron/Go 1.20 兼容代码变化。

## Phase 3: Product Migration

### Task 8: Migrate probe-interpolator

**Acceptance criteria:**
- 三孔、五孔、七孔 PRB、单点、批处理和 CSV 均通过。
- 中文路径、文件选择和帮助文档通过。
- 不重新迁移两个废弃独立产品。

### Task 9: Migrate DAQ-P1604

**Acceptance criteria:**
- 扫描、配置、连接、采集、单位、通道映射、录制和日志通过。
- 高频数据继续使用快照缓存/轮询，不经 Electron IPC。
- Win7 下 UDP 7000/7001 和 TCP 9000 通过。

### Task 10: Migrate 1604Cal

**Prerequisite:** `projects/1604Cal/.git/` 当前确实存在，且使用自己的 `feature/multi-range-calibration` 分支。修改前必须决定继续独立提交/发布，还是在未来转为 submodule；不得只提交外层仓库而漏掉内层改动。

**Acceptance criteria:**
- 复用已有 HTTP/SSE，不复制校准 workflow/domain。
- 现有 Wails 壳只负责启动内嵌 HTTP server、暴露 API 端口和保存对话框；Win7 改造替换这些壳能力，不重写现有 HTTP router 和 SSE event stream。
- 扫描阀、压力控制器、暂停恢复、报告和状态流通过。

### Task 11: Migrate motion-controller

**Acceptance criteria:**
- B140 TCP 与 WTNMC4A DLL 均验证。
- 急停、限位、回零、状态轮询和错误恢复通过。
- DLL 缺失或不兼容时可诊断，不崩溃。

### Task 12: Migrate wind-daq

**Acceptance criteria:**
- 复用已有 HTTP/SSE API。
- 当前交付范围内 DAQ、运动、校准、遍历、存储和报告通过。
- WTNDAQ16H、WTNMC4A 等 DLL 记录版本并真机验证。

### Product checkpoint

每完成一个产品必须：

1. 运行 Go 1.20 tests/vet/build。
2. 运行前端 typecheck/test/build。
3. 生成并安装 NSIS。
4. 验证 health、页面资源、退出和 backend 清理。
5. 涉及设备时完成 Win7 真机核心闭环。
6. 更新 `WIN7-SYNC-STATE.md` 和产品验证记录。

## Phase 4: Ongoing Maintenance

### Task 13: Run scheduled AI sync reviews

触发条件：

- 主线目标产品发布。
- 主线出现算法、协议、采集、录制或数据正确性修复。
- Win7 客户请求主线已有功能。
- 每月常规巡检。
- Win7 发版前。

**Owner:** 工作空间维护者发起并确认 AI 审查。可增加只读 GitHub Action/计划任务生成候选报告，但自动化不得直接 cherry-pick、提交或发布。

### Task 14: Keep mainline commits portable

主线维护建议，不作为硬性 Win7 改造：

- bug 修复保持单一职责。
- 不在同一提交混入格式化、依赖升级和目录重构。
- 提交信息包含产品和问题范围。
- 协议/算法修复附回归测试，便于 AI 回移。

## Verification Template

```powershell
# 根 go.work 需要新 Go，且 daq-t1603 未加入 use；Win7 构建必须关闭 workspace。
$env:GOROOT="C:\go-versions\go1.20.14"
$env:PATH="$env:GOROOT\bin;$env:PATH"
$env:GOWORK="off"
$env:GOTOOLCHAIN="local"

go test ./...
go vet ./...
go build -buildvcs=false ./...
```

```powershell
npm run typecheck
npm run test
npm run build
```

`go.mod` 的 `go 1.20` 不是充分证明；必须由 Go 1.20.14 实际编译。静态扫描 `log/slog`、`math/rand/v2`、`maps`、`slices` 等仅作为快速提示。

## Definition of Done

- `AI-Workspace` 主线产品代码没有因 Win7 改造发生结构或依赖变化。
- `AI-Workspace-win7` 使用统一 `lts/win7` 分支覆盖 6 个维护产品。
- 6 个产品均能生成 Windows 7 SP1 x64 NSIS，并完成对应真机验收。
- 同步台账可追踪所有已回移、人工移植、排除和待处理主线提交。
- 每个安装包都有版本、Git SHA、工具链、SHA256 和验证记录。
- Win7 分支可由 AI 定期维护，但任何同步都需要测试证据，不因 cherry-pick 无冲突而自动判定成功。
