# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cal1604 is a unified industrial pressure calibration system. It merges two legacy applications (1605MeassureApp for measurement, 1604 Calibration Software for calibration) into a Go backend + Vue 3 SPA frontend.

**Two run modes:**
- **Desktop** (Wails v2): `main.go` + `app.go` at project root. Embeds Vue SPA and starts a local HTTP API server on a random port. Build with `wails build`, run with `wails dev`.
- **Web**: `cmd/server/main.go`. Classic HTTP server on `:8080`. Run with `go run ./cmd/server`.

## Build & Development Commands

```bash
# Run full quality gate (Go test + vet, Vue typecheck + lint + test)
make check

# Desktop mode (Wails)
wails dev                                  # Desktop dev with hot-reload
wails build -clean                         # Build standalone .exe → build/bin/

# Web mode
go run ./cmd/server                        # Start web backend (:8080)
./start-dev.sh                            # Start both backend + Vite frontend

# Go backend
go test ./cmd/... ./internal/...          # All Go tests
go test ./internal/workflow/...           # Specific package tests
go vet ./cmd/... ./internal/...           # Static analysis

# Vue frontend (run from web/ or with --prefix web)
npm --prefix web run dev                  # Vite dev server (:5173)
npm --prefix web run build                # Production build
npm --prefix web run typecheck            # vue-tsc --noEmit
npm --prefix web run lint                 # eslint src --ext .ts,.vue
npm --prefix web run test                 # vitest run

# Runtime config (optional)
export CAL1604_CONFIG=path/to/app.json    # Override connection retry params
```

## Architecture

### Backend (Go, standard library only — no external dependencies)

Clean Architecture with layered `internal/` packages:

```
cmd/server/main.go              → Entry point, HTTP server on :8080
internal/
  api/http/                     → HTTP handlers on http.NewServeMux
    router.go                   → All routes registered here, prefix /api/v1/
    response.go                 → Generic Response[T] DTO
    events_handler.go           → SSE endpoint for real-time push
    device_session_handler.go   → /api/v1/session/* handlers (shared device binding & reads)
    measurement_handler.go      → /api/v1/measurement/* handlers (measurement workflow)
  application/
    deviceconnect/service.go    → Connect/disconnect with retry + exponential backoff
    session/service.go          → Shared device session: binding, pressure/stability/data reads
    measurement/service.go      → Measurement workflow: 3-state machine (idle→collecting→paused)
    calibration/service.go      → Calibration workflow orchestration (delegates device ops to session)
  domain/
    device.go                   → Device entity, DeviceType/DeviceStatus enums
    session_state.go            → 14-state session state machine enum
  device/
    interfaces.go               → PressureDriver, MeasureDriver, ConnectionDriver, DeviceStore
  infrastructure/driver/
    factory.go                  → Creates TCP drivers by model (WTN1604, ConST811A, ConST820)
    tcp_connection_driver.go    → TCP protocol implementation
  workflow/
    session_machine.go          → Session lifecycle state machine with allowed transitions
    alarm_service.go            → Alarm detection
    stability_service.go        → Pressure stability detection
  events/bus.go                 → In-process pub/sub with buffered channels
  config/app_config.go          → JSON config loading via CAL1604_CONFIG env var
```

Key patterns: interface-driven device abstraction, state machine for session lifecycle, factory for driver creation, event bus for decoupling handlers from business logic.

### Frontend (Vue 3 + TypeScript + Element Plus + Pinia)

```
web/src/
  router/index.ts               → 5 routes: hub, device-mgmt, measurement, calibration, multi-pressure
  services/apiClient.ts         → Centralized API client
  api/
    session.ts                  → /api/v1/session/* endpoint calls (shared device binding & reads)
    measurement.ts              → /api/v1/measurement/* endpoint calls (measurement workflow)
    calibration.ts              → /api/v1/calibration/* endpoint calls (calibration workflow)
  stores/
    deviceStore.ts              → Device Pinia store
    calibration/index.ts        → Calibration Pinia store
    measurement/index.ts        → Measurement Pinia store (independent from calibration)
    measurement/deviceStore.ts  → Measurement device selection store
  views/                        → Page-level components by business domain
  components/
    calibration/                → Calibration workflow components
    measurement/                → Measurement components
    common/                     → Shared: Sidebar, StatCard, DeviceStatusBadge, Device1604Panel, PressDevicePanel, ChannelMatrix
  styles/
    variables.scss              → CSS custom properties (dark theme)
    button-override.css         → Element Plus button style overrides
```

Layout: 240px fixed sidebar + main content area. Dark industrial theme with gold accent (#c9a45f). 4px spacing grid. Element Plus for all UI components — no non-Element-Plus component libraries.

### UI Design Specifications

详细的设计规范请参考 **DESIGN.md** 文件,包含:

- **色彩系统**: 暗金棕褐配色方案,包含背景色、强调色(暖金主题)、文字色的完整定义
- **按钮规范**: 极简线条风格,透明背景 + 细线边框 + 滑入填充动画效果
- **圆角与间距**: 统一的圆角规范(4px/8px/12px)和间距系统(4px-32px)
- **样式文件位置**:
  - 色彩变量: `web/src/styles/variables.scss`
  - 按钮样式: `web/src/styles/button-override.css`
  - 参考Demo: `web/button-demos.html`

所有前端 UI 开发必须遵循 DESIGN.md 中定义的设计规范,保持视觉一致性。

### API Structure

~40 endpoints under `/api/v1/`:
- **Health/Events**: `GET /health`, `GET /events/stream` (SSE)
- **Config**: `GET /config/device-connect`
- **Devices**: CRUD + connect/disconnect + unit-consistency check
- **Sessions** (lifecycle): state/start/pause/resume/stop
- **Session** (shared device ops): bind devices, read pressure/stability/measure-data, valve control, unit, device-info, reset
- **Measurement**: state/start/pause/stop/data/export
- **Calibration**: set devices/config/channels, generate points, pressurize, collect, fit, read pressure/stability/measure-data (delegates to session)
- **Multi-press**: register/unregister/set-pressure/stop/exhaust/pressure/stability/unit/devices/stop-all
- **Reports**: template selection

## Coding Conventions

### Go
- Chinese comments on all public types, functions, and key domain objects
- Comments explain "why", not just "what" — especially protocol commands, state transitions, fault tolerance, unit conversions
- `gofmt` + `goimports` formatting
- Early return, minimal nesting
- Errors: lowercase start, no trailing period, handle once (wrap or degrade)
- All I/O must carry `context` with timeout/cancellation
- No "fire-and-forget" goroutines — must be controllable

### Vue / TypeScript
- Multi-word PascalCase component names
- Typed `props` — no bare array props
- `v-for` always with stable `key`; never `v-if` + `v-for` on same element
- Pages split by business domain
- Stores handle state + business actions only, no UI concerns
- API calls in service layer (`apiClient.ts`), components never construct API URLs
- Scoped component styles to avoid global pollution

### Design Patterns (allowed)
Adapter (protocol per device model), Factory (driver creation), Strategy (device command variants), State (session state machine), Repository (persistence abstraction). No over-engineering — patterns only when the problem genuinely calls for them.

## Quality Gate

Run `make check` before committing. It runs:
1. `go test ./cmd/... ./internal/...`
2. `go vet ./cmd/... ./internal/...`
3. `npm --prefix web run typecheck`
4. `npm --prefix web run lint`
5. `npm --prefix web run test`

All must pass. Design decisions and architecture records go to `docs/plans/`.

## graphify

This project has a graphify knowledge graph at graphify-out/.

Rules:
- Before answering architecture or codebase questions, read graphify-out/GRAPH_REPORT.md for god nodes and community structure
- If graphify-out/wiki/index.md exists, navigate it instead of reading raw files
- For cross-module "how does X relate to Y" questions, prefer `graphify query "<question>"`, `graphify path "<A>" "<B>"`, or `graphify explain "<concept>"` over grep — these traverse the graph's EXTRACTED + INFERRED edges instead of scanning files
- After modifying code files in this session, run `graphify update .` to keep the graph current (AST-only, no API cost)

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **1604Cal** (6240 symbols, 12891 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/1604Cal/context` | Codebase overview, check index freshness |
| `gitnexus://repo/1604Cal/clusters` | All functional areas |
| `gitnexus://repo/1604Cal/processes` | All execution flows |
| `gitnexus://repo/1604Cal/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
