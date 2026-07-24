# Motion Controller Agent Rules

This file is the project-level startup entrypoint for Motion Controller.

## Progressive Loading

Load project context in this order unless the task is trivial:

1. `projects/motion-controller/AGENTS.md` — project entry, commands, local boundaries
2. `../../AGENTS.md` — workspace startup rules and loading protocol
3. `../../CLAUDE.md` — workspace architecture and hard constraints when implementation or architecture is involved
4. `README.md` — project scope, structure, commands, related docs
5. `SPEC.md` — product scope, shared-boundary intent, success criteria
6. `PLAN.md` / `TASKS.md` — only for sharing, extraction, or migration work
7. touched source files and tests — always read before editing

Do not load the entire `docs/` tree by default.

## Project Scope

Motion Controller is a standalone desktop application for configuring, testing, and operating motion controllers independently from Wind-DAQ.

It owns:

- Desktop shell (Electron 22.3.27 for Win7 / Wails v3 for trunk) and Vue 3 motion operator UI
- project-specific wiring and app context
- thin HTTP API entrypoint (`apps/desktop-wails/main.go` listens on `127.0.0.1:16888`)
- product-level motion workflows built on shared modules

### Win7 兼容版架构（当前主路径）

- **Go 后端** (`apps/desktop-wails/`): `net/http` server + `embed` 前端静态资源，监听 `127.0.0.1:16888`，motion 路由由 `shared.local/motion-control/go/httpapi.RegisterMotionRoutes` 注册。Go 1.20.14，移除 Wails v3 依赖。
- **前端** (`apps/desktop-wails/frontend/`): Vue 3 + Vite，通过 `fetch(MOTION_HTTP_BASE)` 调用后端，200ms 轮询 `/api/motion/status` 获取实时状态。
- **Electron 壳** (`apps/desktop-electron/`): Electron 22.3.27（最后兼容 Win7 的 Electron 主版本），spawn Go 后端 exe + 加载 `http://127.0.0.1:16888`。
- **端口分配**: 16888（与 wind-daq 8900/8901、daq-t1603 18181、daq-p1604 18182、probe-interpolator 18183 区分）。

## Project Commands

```powershell
# Win7 版桌面应用（主路径）
cd projects\motion-controller\apps\desktop-electron
npm install --no-audit --no-fund
npm run build:backend   # 编译 Go 后端 + 前端 dist，输出到 desktop-electron/backend/
npm run dist:win7       # electron-builder 打包 NSIS 安装包
```

```powershell
# Go 后端单独验证（GOWORK=off + Go 1.20.14）
$env:GOWORK = 'off'
$env:GOROOT = 'C:\go-versions\go1.20.14'
$env:PATH = "$env:GOROOT\bin;$env:PATH"
cd projects\motion-controller\apps\desktop-wails
go vet ./...
go test ./...
go build -buildvcs=false -trimpath -ldflags '-s -w -H=windowsgui' .
```

```powershell
# 前端单独验证
cd projects\motion-controller\apps\desktop-wails\frontend
npm install --no-audit --no-fund
npm run typecheck
npm run build
npm run test
```

```powershell
# Backend service（独立 CLI server，非 Win7 版必需）
cd projects\motion-controller\services\api-go
go test ./...
go build -buildvcs=false ./...
go run ./cmd/server
```

## Task Routing

Use these docs by task type:

- architecture / code placement: `../../CLAUDE.md`, `../../docs/architecture/workspace-engineering-rules.zh-CN.md`
- coding/detail conventions: `../../docs/runbooks/code-standards.zh-CN.md`
- development/verification rules: `../../docs/runbooks/development-rules.md`
- project overview and commands: `README.md`
- product scope and shared boundaries: `SPEC.md`
- sharing / extraction / migration planning: `PLAN.md`, `TASKS.md`, `../../docs/decisions/ADR-003-shared-motion-control-module.md`
- release / production build: `../../docs/decisions/ADR-004-wails-v3-production-build.md`

## Motion-Controller-Specific Boundaries

- `apps/desktop-wails/backend` must stay thin: parameter conversion and delegation only.
- Reusable motion device code belongs in `shared/device-sdk/go/motion`.
- Reusable application-level motion orchestration belongs in `shared/motion-control/go`.
- Do not make sibling product projects import `projects/motion-controller/services/api-go/internal/*`.
- If behavior must be shared with `wind-daq`, move it to a proper shared module instead of duplicating or cross-importing project internals.
- `shared/frontend` usage inside this project is transitional; if frontend sharing becomes long-term workspace reuse, move it to workspace-level `shared/frontend` with explicit documentation.
