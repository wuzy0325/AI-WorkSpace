# CLAUDE.md

This file provides guidance to AI agents (Claude Code, OpenCode, or others) when working with code in this workspace.

## Project Context

Industrial DAQ desktop platform — Wails app (Vue 3 + Go) for wind tunnel and lab measurement. Multiple hardware devices (DAQ cards, motors, pressure probes, actuators), calibration workflows, real-time waveform display, high-frequency acquisition.

## Architecture

**Go backend + Vue 3 frontend via Wails, hexagonal boundaries per project.** Larger projects use split `services/api-go/internal/{core,usecase,ports,adapters}` + `apps/desktop-wails/{frontend,backend}`. Small standalone tools may use single-module `apps/desktop-wails/{core,ports,usecase,adapters}` keeping same boundaries.

Shared cross-project code:
- `shared/algorithms/` — Reusable computation (Go + TS), zero device deps
- `shared/device-sdk/` — Device protocol/transport primitives
- `shared/motion-control/` — Motion orchestration, profile persistence, HTTP routes, status polling
- `shared/frontend/` — Vue 3 components/composables
- `shared/contracts/` — Cross-project API contracts

`programs/` = standalone CLI (depend on `shared/*` only, never project `internal/*`). `device-lab/` = raw driver docs/firmware/captures (reference, not source).

### Key Architectural Decisions

- **Hexagonal**: `core` fully isolated from hardware/DB/UI; testable without devices. Deps defined as interfaces in `ports`, implemented in `adapters`.
- **Dependency direction**: `usecase → core + ports` only. `core` never imports `ports`/`adapters`. Strictly one-way.
- **Wails as thin bridge**: `apps/desktop-wails/backend/` only converts params + calls usecase. No business logic, no if/else branching, no hardware calls. If logic must survive a UI framework change, it belongs in `services/api-go/internal/`.
- **Frontend owns display only**: Vue 3 handles UI state/visualization/interaction. All business rules (calibration, measurement, device control) live in Go backend.

Module design: `docs/architecture/module-design.md`. Project layout variants: `docs/architecture/project-variants.md`.

## Hard Constraints

Zero-tolerance table (same as [AGENTS.md](AGENTS.md#hard-constraints-zero-tolerance)). Below are the **sanctioned exceptions and composition boundary** — the bare table is too absolute without these clarifications.

### Constraint Clarifications

1. **`core/` — format descriptions OK, byte I/O not.** `core/` MAY define persistence-format structures (`CsvSchema`, `CalibrationRecord`, column order, units, precision, naming). MUST NOT do byte-level I/O (`os.OpenFile`, `csv.NewWriter`, `json.Marshal`→writer, `net.Dial`). Rule: "what columns/units?" → `core/`; "how bytes reach disk?" → `adapters/`.

2. **Composition root is the only place adapters meet usecase.** `pkg/appcontext/`, `pkg/wiring/`, `pkg/apiserver/`, `internal/bootstrap/`, `cmd/`, or `apps/desktop-wails/backend/app.go` are the SOLE locations allowed to import `ports` + `adapters` + `usecase` simultaneously. `usecase/` MUST NOT import any `adapters/*` — not even inside `New*` factories. Defaults are injected by composition root; usecase only consumes `ports` interface.

3. **`shared/` dependency direction is one-way and layered.**
   - `shared/` MUST NOT import `projects/*/internal/*`
   - `shared/algorithms/` MUST NOT import `shared/device-sdk/` (device-agnostic)
   - `shared/device-sdk/` MAY import `shared/algorithms/`
   - `shared/motion-control/` MAY import `shared/algorithms/` + `shared/device-sdk/`
   - `shared/frontend/` MUST NOT import any project `frontend/src/`

4. **`frontend/` — demo/test exempt, production not.** "Zero calibration algorithms" applies to `frontend/src/{features,shared,modules,stores,views}/`. Demo/mock/test under `frontend/src/__tests__/`, `frontend/src/mocks/`, or `frontend/src/utils/simulate*` MAY contain algorithm copies for offline demo, provided:
   - File header has `// DEMO ONLY — not for production use`
   - No production feature file imports it (enforced by `scripts/validate-frontend-structure.ps1`)

### Automated Enforcement

Structural rules: `scripts/validate-structure.ps1`. Import direction: `golangci-lint` + `depguard`, or `scripts/validate-import-direction.ps1` (works without golangci-lint).

## Decision Tree: Where Does This Code Go?

| Question | If YES → location |
|---|---|
| Business rule (calibration, measurement, acquisition logic)? | `core/<domain>/` |
| Orchestrates multiple domains or external deps? | `usecase/` |
| Interface definition for external dep? | `ports/` |
| Concrete implementation of a port (device driver, DB)? | `adapters/<type>/` |
| UI display or user interaction? | `apps/desktop-wails/frontend/src/modules/<domain>/` |
| Wails method binding (Go → JS bridge)? | `apps/desktop-wails/backend/bindings/` |
| 2+ projects reuse this logic? | `shared/` (algorithms / device-sdk / motion-control / frontend / contracts) |
| Reusable motion-control app logic (manager, profile store, HTTP route, status poller)? | `shared/motion-control/go/` |
| Low-level device protocol / hardware adapter / serial transport / FFI wrapper? | `shared/device-sdk/go/` |
| Standalone CLI tool? | `programs/<tool-name>/` |
| Raw hardware docs (PDF, datasheet, captures)? | `device-lab/` |

## Design Principles

Reference: `docs/runbooks/development-rules.md` (sections 8–12).

1. **Frontend-backend separation** — Frontend displays, backend decides. If swapping Vue for a web UI would still need this logic, put it in Go backend.
2. **Program to interfaces** — All external deps via `ports`. Strategy for multi-device, Observer for real-time data. New device = new adapter, zero core changes.
3. **Readability first** — One function, one job. Business-domain names, no abbreviations. Max 3 nesting levels. Comments explain why, not what.
4. **Boundary defense** — Validate at edges (user input, device responses). Trust internal callers. Timeout + retry on all hardware I/O. On the affected field Windows environment, Go socket deadlines do not reliably unblock an in-flight read; bounded hardware I/O must also have an independent watchdog/cancellation owner that can call `conn.Close()` without waiting for the I/O goroutine or its locks. A watchdog-closed connection is invalid and must be reconnected. See ADR-009. No silent error swallowing.
5. **Long-term stability** — Explicit cleanup (defer, context). Pre-allocate buffers on hot paths. Log state changes at info, communication at debug. Externalize all config.

## Workspace Structure

Validated by `scripts/validate-structure.ps1` (structure) + `scripts/validate-frontend-structure.ps1` (frontend). New projects: `scripts/new-project.ps1 -Name <name>`. Structural changes → update `workspace.structure.json` + document in `docs/decisions/`. Full rules: `docs/runbooks/workspace-directory-rules.zh-CN.md`.

## Commands

See [AGENTS.md §Environment & Commands](AGENTS.md#environment--commands) for the essential command set. Additional workspace-level scripts: `scripts/validate-import-direction.ps1` (hexagonal import check, no golangci-lint required), `scripts/lint-go.ps1` (gofmt + build across all Go projects), `golangci-lint run -c .golangci.yml ./...` (depguard enforces core/ports/usecase rules). Per-project commands live in each `projects/<name>/README.md`.

## Requirements

- **Go** — follow `go.work` and each touched `go.mod`; `go build ./...` / `go test ./...` must pass for touched modules before committing.
- **Node.js** (LTS) — required for Vue 3 frontend builds.
- **Wails CLI v3** — required for desktop app generation/builds.

## Commit Rules

- Atomic commits grouped by logical intent.
- Conventional format: `feat(scope)`, `fix(scope)`, `refactor(scope)`, `docs`, `test(scope)`, `chore(scope)`.

## Code Standards

- `docs/runbooks/code-standards.zh-CN.md` — Full specification (Chinese)
- `docs/runbooks/frontend-ai-rules.zh-CN.md` — AI-executable frontend rules for Vue/Wails UI work
- `docs/runbooks/frontend-directory-rules.zh-CN.md` — Frontend directory structure standard

## Language

Docs: bilingual (EN + ZH) as needed. Code comments: English only. AI execution rules: `docs/runbooks/ai-agent-execution-standard.zh-CN.md`.

## More

- `docs/index.md` — Full documentation index
- `docs/architecture/project-variants.md` — Approved project structure variants

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **AI-WorkSpace** (55790 symbols, 115745 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
| `gitnexus://repo/AI-WorkSpace/context` | Codebase overview, check index freshness |
| `gitnexus://repo/AI-WorkSpace/clusters` | All functional areas |
| `gitnexus://repo/AI-WorkSpace/processes` | All execution flows |
| `gitnexus://repo/AI-WorkSpace/process/{name}` | Step-by-step execution trace |

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
