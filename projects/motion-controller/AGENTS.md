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

Motion Controller is a standalone Wails desktop application for configuring, testing, and operating motion controllers independently from Wind-DAQ.

It owns:

- Wails desktop shell and Vue 3 motion operator UI
- project-specific wiring and app context
- thin Wails bindings and optional HTTP API entrypoint
- product-level motion workflows built on shared modules

## Project Commands

```powershell
# Desktop app shell
cd projects\motion-controller\apps\desktop-wails
go test ./...
go build -buildvcs=false ./...
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
# Backend service
cd projects\motion-controller\services\api-go
go test ./...
go build -buildvcs=false ./...
go run ./cmd/server
```

```powershell
# Frontend
cd projects\motion-controller\apps\desktop-wails\frontend
npm install --no-audit --no-fund
npm run typecheck
npm run build
npm run test
```

## Task Routing

Use these docs by task type:

- architecture / code placement: `../../CLAUDE.md`, `../../docs/architecture/workspace-engineering-rules.zh-CN.md`
- coding/detail conventions: `../../docs/runbooks/code-standards.zh-CN.md`
- development/verification rules: `../../docs/runbooks/development-rules.md`
- project overview and commands: `README.md`
- product scope and shared boundaries: `SPEC.md`
- sharing / extraction / migration planning: `PLAN.md`, `TASKS.md`, `../../docs/decisions/ADR-003-shared-motion-control-module.md`

## Motion-Controller-Specific Boundaries

- `apps/desktop-wails/backend` must stay thin: parameter conversion and delegation only.
- Reusable motion device code belongs in `shared/device-sdk/go/motion`.
- Reusable application-level motion orchestration belongs in `shared/motion-control/go`.
- Do not make sibling product projects import `projects/motion-controller/services/api-go/internal/*`.
- If behavior must be shared with `wind-daq`, move it to a proper shared module instead of duplicating or cross-importing project internals.
- `shared/frontend` usage inside this project is transitional; if frontend sharing becomes long-term workspace reuse, move it to workspace-level `shared/frontend` with explicit documentation.
