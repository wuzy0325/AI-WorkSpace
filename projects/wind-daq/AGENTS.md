# Wind-DAQ Agent Rules

Single source of truth starts at `../../AGENTS.md`, but this file is the project-level startup entrypoint for Wind-DAQ.

## Progressive Loading

Load project context in this order unless the task is trivial:

1. `projects/wind-daq/AGENTS.md` — project entry, commands, local boundaries
2. `../../AGENTS.md` — workspace startup rules and loading protocol
3. `../../CLAUDE.md` — workspace architecture and hard constraints when implementation or architecture is involved
4. `README.md` — project overview, commands, folder notes, migration entry points
5. `CLAUDE.md` — Wind-DAQ project addendum and migration constraints
6. `DESIGN.md` — only for frontend UI / layout / visual parity work
7. `docs/migration/*` — only for migration or parity work
8. touched source files and tests — always read before editing

Do not load the entire `docs/` tree by default.

## Project Commands

```powershell
# Backend, after go.mod exists
cd projects\wind-daq\services\api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...
```

```powershell
# Backend tests
cd projects\wind-daq\services\api-go
go test ./internal/... ./api/...
```

```powershell
# Frontend
cd projects\wind-daq\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test
```

```powershell
# Wails shell (development / bindings)
cd projects\wind-daq\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
# Release build (production mode, -tags production)
cd projects\wind-daq\apps\desktop-wails
task release
```

See `docs/decisions/ADR-004-wails-v3-production-build.md` for production build tag rules.

## Project Scope

Wind-DAQ is the main wind tunnel DAQ desktop application in this workspace.

It owns:

- device management and acquisition workflows
- calibration and traversal workflows
- storage and reports flows
- integrated motion-control behavior at the product layer
- Wails desktop shell + Vue 3 operator UI + Go backend

It must not import project-local code from sibling product projects such as `projects/motion-controller/*`.

## Project Addendum

- Wails backend code in `apps/desktop-wails/backend` must stay thin: parameter conversion and usecase calls only. No domain business logic.
- Vue frontend code in `apps/desktop-wails/frontend` must not access hardware or contain calibration/traversal algorithms.
- New migrated behavior should be added test-first unless it is documentation-only or generated code.

## Task Routing

Use these docs by task type:

- architecture / code placement: `../../CLAUDE.md`, `../../docs/architecture/workspace-engineering-rules.zh-CN.md`
- coding/detail conventions: `../../docs/runbooks/code-standards.zh-CN.md`
- development/verification rules: `../../docs/runbooks/development-rules.md`
- project overview and commands: `README.md`
- project-specific architecture addendum: `CLAUDE.md`
- frontend UI / component / style / store / API-client rules: `../../docs/runbooks/frontend-ai-rules.zh-CN.md`
- frontend directory structure: `../../docs/runbooks/frontend-directory-rules.zh-CN.md`
- frontend UI / parity / layout: `DESIGN.md`
- frontend UI consistency migration: `docs/ui-design-audit.md`
- migration implementation: `docs/migration/README.md`, `docs/migration/ui-parity-plan.md`, `docs/migration/ts-reference-feature-map.md`
- release / production build: `../../docs/decisions/ADR-004-wails-v3-production-build.md`

## Wind-DAQ-Specific Boundaries

- Reusable motion behavior belongs in `shared/motion-control/go`.
- Low-level device and motion protocol code belongs in `shared/device-sdk/go` or project adapters, not in frontend or Wails glue.
- Frontend parity work should preserve the recognizable operator workflow of the reference UI while keeping implementation architecture cleaner.
