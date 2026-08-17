# WindLabX4 Agent Rules

Single source of truth starts at `../../AGENTS.md`, but this file is the project-level startup entrypoint for WindLabX4.

## Progressive Loading

Load project context in this order unless the task is trivial:

1. `projects/windlabx4/AGENTS.md` — project entry, commands, local boundaries
2. `../../AGENTS.md` — workspace startup rules and loading protocol
3. `../../CLAUDE.md` — workspace architecture and hard constraints when implementation or architecture is involved
4. `README.md` — project overview, commands, folder notes, migration entry points
5. `CLAUDE.md` — WindLabX4 project addendum and migration constraints
6. `DESIGN.md` — only for frontend UI / layout / visual parity work
7. `docs/design/monitor-workspace-spec.md` — when changing dashboard hybrid/monitor layout, action scope, channel cards, or live chart chrome
8. `docs/migration/*` — only for migration or parity work
9. touched source files and tests — always read before editing

Do not load the entire `docs/` tree by default.

## Project Commands

```powershell
# Backend, after go.mod exists
cd projects\WindLabX4\services\api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...
```

```powershell
# Backend tests
cd projects\WindLabX4\services\api-go
go test ./internal/... ./api/...
```

```powershell
# Frontend
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test
```

```powershell
# Wails shell (development / bindings)
cd projects\WindLabX4\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
# Release build (production mode, -tags production)
cd projects\WindLabX4\apps\desktop-wails
task release
```

See `docs/decisions/ADR-004-wails-v3-production-build.md` for production build tag rules.

## Project Scope

WindLabX4 is the main wind tunnel DAQ desktop application in this workspace.

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

## WindLabX4-Specific Boundaries

- Reusable motion behavior belongs in `shared/motion-control/go`.
- Low-level device and motion protocol code belongs in `shared/device-sdk/go` or project adapters, not in frontend or Wails glue.
- Frontend parity work should preserve the recognizable operator workflow of the reference UI while keeping implementation architecture cleaner.

## NSIS Installer (project.nsi) Hard Rules

`build/windows/installer/project.nsi` 已纳入 git 跟踪（强制添加，覆盖 .gitignore）。

- **永远不要**用 `[System.Text.Encoding]::Default` 或任何 PowerShell 命令转换 .nsi 文件编码。
- **永远不要**删除 `project.nsi`——如需恢复，用 `git checkout` 而非依赖 `wails3 generate build-assets`（该命令生成的 .nsi 中文乱码，不可用）。
- 修改中文文本只能通过 Edit 工具进行（它保留原文件编码）。
- NSIS 构建失败时不假设文件损坏，先检查是否是其他原因（如 DLL 缺失）。
- **技术卡点**：`scripts/check-nsi-encoding.ps1` 已接入 pre-commit 钩子——任何 tracked `project.nsi` 含非 ASCII 字节却无 BOM 时阻止提交（makensis 对无 BOM 文件按系统代码页解析，UTF-8 无 BOM 直接报 "Bad text encoding"）。标准形态：**UTF-8 with BOM**。
