# WISTA

Standalone Wails desktop application for WISTA data acquisition, monitoring, and CSV recording.

## Architecture

This project intentionally uses a single Go module under `apps/desktop-wails` instead of a separate `services/api-go` module. It still follows the workspace hexagonal boundaries.

```text
apps/desktop-wails/
  core/               # pure domain types
  ports/              # interfaces only
  usecase/            # orchestration
  adapters/           # hardware, config, recording implementations
  backend/            # Wails binding layer only
  frontend/           # Vue 3 / Vite / Pinia / ECharts UI
```

See `CLAUDE.md` for the project-specific constraints.

## Modes

Real hardware mode only. Configure a WISTA profile in the app with the device IP and port.

## Windows 7 LTS

Windows 7 SP1 x64 使用独立长期支持 worktree `AI-Workspace-win7`。其未提交 working tree 已于 2026-07-23 完成真机安装与启动验证；当前 HEAD 尚不包含可重建的 Win7 基线，必须先按实施计划 Task 0 鉴别并提交。该版本固定使用 Go 1.20.14、Electron 22.3.27、HTTP/WebSocket bridge 和 NSIS x64 安装包；主线的 Go 1.25 + Wails v3 版本不直接支持 Windows 7。

业务 bug 必须优先在主线修复，再通过 `git cherry-pick -x <commit>` 或兼容移植回移到 Win7 分支；不要将 `master` 整体 merge 到 Win7 分支。wista 技术基线见 `docs/decisions/ADR-007-wista-win7-lts.md`，全工作空间组织与同步规则见 `docs/decisions/ADR-008-workspace-win7-lts-worktree.md`。

## Development Commands

```powershell
# Go 开发
cd projects/wista/apps/desktop-wails
go test ./...
go vet ./...
go build -buildvcs=false ./...
```

```powershell
# 前端开发
cd projects/wista/apps/desktop-wails/frontend
npm install --no-audit --no-fund
npm run typecheck
npm run build
npm run test
```

```powershell
# 桌面应用开发（热更新 / bindings 生成）
cd projects/wista/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

## Release Commands（对外交付打包）

```powershell
cd projects/wista/apps/desktop-wails
$env:GOWORK="off"
go run github.com/wailsapp/wails/v3/cmd/wails3 build   # wails3 build 内部自动使用 -tags production
```

**必须设置 `GOWORK=off`** 以隔离工作空间中其他模块的 wails/v2 间接引用，
否则构建的 exe 可能运行时报"correct build tags"错误。
详见 `docs/decisions/ADR-004-wails-v3-production-build.md`。

Windows 7 安装包使用独立 worktree 构建，命令与约束见 `docs/decisions/ADR-008-workspace-win7-lts-worktree.md`。

## Shared Code

- `shared/device-sdk/go/daq` contains reusable DAQ domain and hardware SDK code.
- Project-specific recording, profile selection, and Wails UI wiring stay inside this project.

## Development Rules

- `core/` must stay free of hardware, file I/O, serial/network, and framework imports.
- `ports/` contains interfaces only.
- `usecase/` calls devices through ports.
- `backend/` is parameter conversion plus usecase calls only.
- `frontend/src/bridge/` is the only frontend layer allowed to import generated `wailsjs/` bindings.
