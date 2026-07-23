# ADR-007: DAQ-T1603 Windows 7 长期支持版本

## Date

2026-07-23

## Status

Accepted as verified working-tree baseline（Windows 7 SP1 x64 真机验证通过；源码尚待提交固化）

工作空间级分支组织与同步策略由 `ADR-008-workspace-win7-lts-worktree.md` 接管。本 ADR 继续保留 DAQ-T1603 的已验证技术方案和验收证据。

## Context

DAQ-T1603 需要继续支持 Windows 7。主线使用 Go 1.25、Wails v3 和 WebView2，无法直接运行在 Windows 7：Go 1.21 及以上不再官方支持 Windows 7，Wails v3 依赖较新的 Go 标准库，WebView2 也不是可靠的 Windows 7 运行时。

2026-07-23 已用 `AI-Workspace-win7` 中的未提交实现，在 Windows 7 SP1 x64 真机完成安装与启动验证，确认以下替代方案可行。该结论证明当前 working tree 可行，不代表 HEAD `b041033` 本身包含 Win7 实现；正式开发前必须先按实施计划 Task 0 鉴别并提交基线。Windows 7 版仍需长期接收主线中的业务 bug 修复，但不能让旧运行时约束阻止主线升级。

## Decision

### 技术基线

Windows 7 版采用独立的长期支持分支 `feature/daq-t1603-win7`，并通过 Git worktree 检出到与主工作区平级的 `AI-Workspace-win7`：

- Go 固定为 `1.20.14`，使用 `GOWORK=off` 构建。
- Electron 固定为 `22.3.27`。Electron 22 是支持 Windows 7 的最后一个主版本；Electron 23 及以上只支持 Windows 10 及以上。
- 前端固定使用 Electron 内置的 Chromium 108。
- 移除 Wails 运行时，Go 后端使用 `net/http` 提供 RPC 和静态资源，WebSocket 推送采集事件。
- Electron 主进程负责启动和关闭 Go 后端、等待 `/api/health` 就绪、创建窗口及提供目录选择 IPC。
- NSIS 生成 x64 安装包；Windows 7 的目标基线为 Windows 7 SP1 x64。

已验证但尚未提交的实现位置：

- Electron 壳：`AI-Workspace-win7/projects/daq-t1603/apps/desktop-electron/`
- Electron 入口：`main.cjs`、`preload.cjs`
- 构建与 NSIS 配置：`package.json`、`scripts/build-backend.ps1`
- Go HTTP/WebSocket：`projects/daq-t1603/apps/desktop-wails/httpserver/`
- 前端 transport：`frontend/src/bridge/httpClient.ts`、`wsClient.ts`

这些路径只有在当前 Win7 working tree 中存在；在完成基线提交前，不得把 `feature/daq-t1603-win7` 的 HEAD 当作可重建依据。

### 代码同步模型（由 ADR-008 扩展到全工作空间）

采用“主线优先修复、按提交回移”的长期支持模型，不定期整体合并 `master`：

1. 可跨平台复用的 bug 首先在 `master` 修复并完成测试。
2. 每个修复保持为职责单一、可独立验证的提交，避免混入格式化、重构或平台升级。
3. 在 Win7 worktree 中执行 `git cherry-pick -x <commit>`。`-x` 会在提交信息中记录主线来源，便于审计和判断是否已同步。
4. 解决冲突时保留 Win7 的 Go 1.20、HTTP/WebSocket 和 Electron 外壳，只移植业务修复。
5. 回移后必须用 Go 1.20.14 执行后端测试和构建，执行前端 typecheck/build，并重新生成 NSIS 安装包。
6. 涉及硬件驱动、DLL、串口或采集时，发布前增加 Windows 7 真机回归。

标准操作：

```powershell
# 主线：先提交一个可独立回移的修复
git switch master
git log -1 --oneline

# Win7 worktree：回移该修复并保留来源信息
cd ..\AI-Workspace-win7
git status --short
git cherry-pick -x <master-commit-sha>

# 验证 Go 1.20 兼容性
$env:GOROOT="C:\go-versions\go1.20.14"
$env:PATH="$env:GOROOT\bin;$env:PATH"
$env:GOWORK="off"
cd projects\daq-t1603\apps\desktop-wails
go test ./...
go vet ./...
go build -buildvcs=false ./...

# 验证并生成安装包
cd ..\desktop-electron
npm run dist:win7
```

### 同步边界

通常可以回移：

- `core/`、`ports/`、`usecase/` 中不依赖新 Go API 的业务修复。
- `adapters/hardware/`、`adapters/recording/` 和 `adapters/config/` 的协议与数据修复。
- `shared/device-sdk/go/` 中与 DAQ-T1603 相关、能在 Go 1.20 编译的修复。
- `frontend/src/components/`、`views/`、`stores/`、样式和纯类型定义。

必须人工改写或拒绝回移：

- Wails application、binding、WebView2 和生成绑定变更。
- `frontend/src/bridge/` 中直接依赖 Wails 的变更；Win7 版需要映射到 HTTP/WebSocket bridge。
- 使用 `log/slog`、`math/rand/v2`、`maps`、`slices` 或其他 Go 1.21+ API 的代码。
- 提升 Electron、Vite 或依赖最低操作系统版本的升级。
- 与业务修复无关的大规模重构。

### 定期维护

- 每次主线 DAQ-T1603 发布时，检查自上次 Win7 基线以来的提交，而不是盲目合并全部提交。
- 使用固定格式记录同步结果：主线提交 SHA、Win7 提交 SHA、是否回移、原因、验证命令和真机结果。
- 当回移冲突开始频繁出现时，优先把主线中的纯业务逻辑进一步下沉到不依赖桌面框架的包；不要在两个分支分别维护两套算法。
- 每个 Win7 安装包都记录 Go、Electron、应用版本和 SHA256。Electron 22 已停止安全维护，因此 Win7 版只用于受控工业环境，不开放不受信任的网络访问。
- Electron 必须保持 `contextIsolation: true`、`sandbox: true`、`nodeIntegration: false` 和 `webSecurity: true`，只加载 `127.0.0.1` 本地服务；默认禁止外部网络内容。

## Alternatives Considered

### 定期将 master 整体 merge 到 Win7 分支

拒绝。主线和 Win7 分支的 Go 版本、桌面框架、入口、前端 bridge 与依赖树永久不同，整体 merge 会反复引入不可用的平台升级并产生大量无关冲突。

### 维护一份脱离 Git 历史的 Win7 代码副本

拒绝。文件复制无法追踪修复来源，也无法可靠判断某个主线 bug 是否已同步。

### 在 master 内使用 build tags 同时维护 Wails 和 Electron

暂不采用。它能减少分支漂移，但会把 Go 1.20 和 Electron 22 的长期约束带入所有主线开发与依赖升级。只有在 Win7 需求扩大、回移成本显著高于双目标持续集成成本时再重新评估。

## Consequences

- 主线可以继续升级 Go、Wails 和依赖，不受 Windows 7 限制。
- Win7 修复具备明确来源和审计链，适合低频长期维护。
- 同一个业务修复可能需要一次主线提交和一次 Win7 回移验证。
- 跨平台代码越纯，回移成本越低；平台 bridge 和桌面入口必须保持隔离。
- Windows 7 最终兼容性仍以真机和厂商 DLL/驱动验证为准。

## References

- `projects/daq-t1603/README.md`
- `docs/decisions/ADR-004-wails-v3-production-build.md`
- `docs/decisions/ADR-006-daq-t1603-workspace-isolation.md`
- Electron 官方公告：`https://www.electronjs.org/blog/windows-7-to-8-1-deprecation-notice`
- `docs/decisions/ADR-008-workspace-win7-lts-worktree.md`
