# DAQ-T-1603 Agent Rules

This file is the project-level startup entrypoint for DAQ-T-1603.

## Progressive Loading

Load project context in this order unless the task is trivial:

1. `projects/wista/AGENTS.md` — project entry, commands, local boundaries
2. `../../AGENTS.md` — workspace startup rules and loading protocol
3. `../../CLAUDE.md` — workspace architecture and hard constraints when implementation or architecture is involved
4. `README.md` — project overview, commands, modes, shared-code notes
5. `CLAUDE.md` — DAQ-T-1603 single-module architecture and project-specific hard constraints
6. touched source files and tests — always read before editing

Do not load the entire `docs/` tree by default.

## Project Scope

DAQ-T-1603 is a standalone Wails desktop application for:

- device profile management
- DAQ-T-1603 acquisition and monitoring
- CSV recording
- real-hardware operation

This project intentionally uses a single Go module under `apps/desktop-wails`, but it must still preserve the workspace hexagonal boundaries inside that module.

## Project Commands

```powershell
# Go module root (development)
cd projects\wista\apps\desktop-wails
go test ./...
go vet ./...
go build -buildvcs=false ./...
```

```powershell
# Frontend
cd projects\wista\apps\desktop-wails\frontend
npm install --no-audit --no-fund
npm run typecheck
npm run build
npm run test
```

```powershell
# Wails development (hot-reload / bindings)
cd projects\wista\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
# Release build (production mode, wails3 build uses -tags production internally)
cd projects\wista\apps\desktop-wails
$env:GOWORK="off"
go run github.com/wailsapp/wails/v3/cmd/wails3 build
```

See `docs/decisions/ADR-004-wails-v3-production-build.md` for production build tag rules and `GOWORK=off` requirement.

## Task Routing

Use these docs by task type:

- architecture / code placement: `../../CLAUDE.md`, `../../docs/architecture/workspace-engineering-rules.zh-CN.md`
- coding/detail conventions: `../../docs/runbooks/code-standards.zh-CN.md`
- development/verification rules: `../../docs/runbooks/development-rules.md`
- project overview and commands: `README.md`
- project-specific architecture and boundaries: `CLAUDE.md`
- release / production build: `../../docs/decisions/ADR-004-wails-v3-production-build.md`

## DAQ-T-1603-Specific Boundaries

- `frontend/src/bridge/` is the only frontend layer allowed to import generated `wailsjs/` bindings.
- `backend/` must stay thin: parameter conversion and usecase calls only.
- `frontend/` must not access hardware directly or host backend business logic.
- Reusable DAQ domain and device SDK logic belongs in `shared/device-sdk/go/daq`; project-specific recording and UI wiring stay local.
- Even though this is a standalone single-module app, do not collapse `core`, `ports`, `usecase`, `adapters`, `backend`, and `frontend` into mixed files.
