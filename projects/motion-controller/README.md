# Motion Controller

Standalone Wails desktop application for configuring, testing, and operating motion controllers independently from Wind-DAQ.

## Scope

This project owns the product shell and project-specific wiring for motion-control workflows:

- Wails desktop app and Vue 3 operator UI.
- Project-specific service wiring and app context.
- Thin Wails bindings and optional HTTP API entrypoint.
- Motion profile management and controller commands via shared modules.

Reusable motion behavior lives in shared modules:

- `shared/device-sdk/go/motion` — low-level motion device types, ports, and controller adapters.
- `shared/motion-control/go` — reusable motion manager, profile persistence, HTTP route glue, and status polling helper.

## Structure

```text
projects/motion-controller/
  apps/desktop-wails/
    backend/          # Wails binding layer only
    frontend/         # Vue 3 / Vite / Pinia UI
    main.go           # Wails app wiring
  services/api-go/
    api/              # thin HTTP routes
    cmd/server/       # optional HTTP server entrypoint
    internal/         # project-specific hexagonal layers and wiring
    pkg/appcontext/   # dependency initialization
  shared/frontend/    # temporary project-local frontend sharing area
```

## Development Commands

```powershell
# 桌面应用开发（热更新）
cd projects/motion-controller/apps/desktop-wails
go test ./...
go build -buildvcs=false ./...
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
# 后端服务开发
cd projects/motion-controller/services/api-go
go test ./...
go build -buildvcs=false ./...
go run ./cmd/server
```

```powershell
# 前端开发
cd projects/motion-controller/apps/desktop-wails/frontend
npm install --no-audit --no-fund
npm run typecheck
npm run build
npm run test
```

## Release Commands（对外交付打包）

```powershell
cd projects/motion-controller/apps/desktop-wails
task release               # 清理旧产物 → 构建前端 → 生产模式 Go 二进制
makensis build\windows\installer\project.nsi   # NSIS 安装包
```

生产构建使用 `-tags production -trimpath -ldflags="-w -s -H windowsgui"`，
详见 `docs/decisions/ADR-004-wails-v3-production-build.md`。

## Development Rules

- Follow workspace rules in `../../CLAUDE.md`.
- Keep `apps/desktop-wails/backend` thin: parameter conversion and delegation only.
- Do not copy shared motion device code into this project.
- Do not make other projects import `motion-controller/services/api-go/internal/*`.
- If behavior must be shared with `wind-daq`, move it to `shared/motion-control/go` or `shared/device-sdk/go` depending on its layer.

## Related Docs

- `SPEC.md` — product specification and success criteria.
- `PLAN.md` — motion sharing plan.
- `TASKS.md` — current sharing task status.
- `../../docs/decisions/ADR-003-shared-motion-control-module.md` — shared motion-control module decision.
- `../../docs/decisions/ADR-004-wails-v3-production-build.md` — Wails 生产构建标签规则与发布约束。
