# CLAUDE.md

This file provides guidance to AI agents (Claude Code, OpenCode, or others) when working with code in this workspace.

## Project Context

Industrial DAQ (Data Acquisition) desktop platform — Wails app (Vue 3 + Go) for wind tunnel and lab measurement.

- Multiple hardware devices: DAQ cards, stepping motors, pressure probes, position actuators
- Calibration workflows with guided procedures
- Real-time waveform display and high-frequency data acquisition
- Built for small engineering teams in testing labs

## Architecture

**Go backend + Vue 3 frontend via Wails, with hexagonal boundaries per project.**

Most larger projects use a split `services/api-go` backend plus a Wails app shell. Smaller standalone tools may use a single Go module under `apps/desktop-wails` while keeping the same `core -> usecase -> ports -> adapters` boundaries inside that module.

```
projects/<project>/
├── apps/desktop-wails/
│   ├── frontend/          # Vue 3 UI (display + interaction only)
│   └── backend/           # Wails bindings/app shell glue (zero business logic)
├── services/api-go/
│   ├── cmd/               # Server entry points (wiring only)
│   └── internal/
│       ├── core/          # Pure domain logic (zero hardware, zero I/O)
│       ├── usecase/       # Orchestration (coordinates core + ports)
│       ├── ports/         # Interface definitions (zero implementations)
│       └── adapters/      # Concrete implementations
│           ├── hardware/  # Device drivers (DAQ, motors, probes)
│           ├── db/        # Database persistence
│           └── mq/        # Event/message
├── contracts/             # API contracts (OpenAPI, Proto)
├── tests/
│   ├── integration/       # Integration tests
│   └── hil/               # Hardware-in-loop tests (real devices only)
└── deploy/                # Environment configs (dev/staging/prod)
```

Shared cross-project code:

- `shared/algorithms/` — Reusable computation (Go + TS), zero device dependencies
- `shared/device-sdk/` — Reusable device protocol/transport primitives
- `shared/motion-control/` — Reusable application-level motion orchestration, profile persistence, HTTP routes, and status polling helpers
- `shared/frontend/` — Reusable Vue 3 components/composables
- `shared/contracts/` — Cross-project API contracts

Standalone tools:

- `programs/` — CLI utilities (calibrator, serial monitor, firmware upgrader). Depend on `shared/*` only, never on project `internal/*`.

Hardware lab artifacts:

- `device-lab/` — Raw driver docs, firmware, captures, rig diagrams. Reference material, not source code.

### Key Architectural Decisions

**Hexagonal (ports & adapters):** Business logic in `core` is completely isolated from hardware, databases, and UI. External dependencies are defined as interfaces in `ports`, implemented in `adapters`. This means `core` is testable without any device connected.

**Dependency direction:** `usecase → core + ports`. `core` never imports `ports` or `adapters`. Adapters implement port interfaces. The dependency graph is strictly one-way.

**Wails as thin bridge:** The Wails backend (`apps/desktop-wails/backend/`) only converts parameters and calls usecase methods. No business logic, no if/else branching, no hardware calls. If logic needs to survive a UI framework change, it belongs in `services/api-go/internal/`.

**Frontend owns display only:** Vue 3 components handle UI state, visualization, and user interaction. All business rules (calibration algorithms, measurement processing, device control) live in the Go backend.

### Module Design

See `docs/architecture/module-design.md` for detailed Go package and Vue 3 module structure.
See `docs/architecture/project-variants.md` for approved project layout variants.

## Hard Constraints

These are zero-tolerance rules. Violating them breaks the architecture:

| Location | Constraint |
|---|---|
| `core/` | zero hardware imports, zero file I/O, zero serial/network, zero framework imports |
| `ports/` | zero implementations, zero structs with methods — interface definitions only |
| `usecase/` | zero direct hardware calls — go through `ports` interfaces |
| `api/` (server handlers) | zero domain logic — thin HTTP routing, delegate to `usecase` |
| `adapters/hardware/` | zero domain logic — pure protocol translation and I/O |
| `apps/desktop-wails/backend/` | zero domain business logic — parameter conversion + usecase calls only |
| `apps/desktop-wails/frontend/` | zero direct hardware access, zero calibration algorithms |
| `programs/` | zero project `internal/*` imports — depend on `shared/*` only |

### Constraint Clarifications

The zero-tolerance table above is the baseline. The clarifications below define the only sanctioned exceptions and the composition boundary. They exist because the bare table was too absolute and pushed domain knowledge into the wrong layer.

**1. `core/` — format descriptions are allowed, byte I/O is not.**
`core/` MAY define data structures that describe persistence formats (e.g. `CsvSchema`, `CalibrationRecord`, column order, units, precision, file naming conventions). What `core/` MUST NOT do is perform byte-level I/O: no `os.OpenFile`, no `csv.NewWriter`, no `json.Marshal` writing to a writer, no `net.Dial`. Rule of thumb: if the code answers "what columns and units?" it belongs in `core/`; if it answers "how do bytes reach disk?" it belongs in `adapters/`.

**2. Composition root is the only place adapters meet usecase.**
`pkg/appcontext/`, `pkg/wiring/`, `pkg/apiserver/`, `internal/bootstrap/`, `cmd/`, or `apps/desktop-wails/backend/app.go` are the SOLE locations allowed to import `ports`, `adapters`, and `usecase` simultaneously. `usecase/` MUST NOT import any `adapters/*` package — not even to construct a default implementation inside a `New*` factory. Factory functions that wire adapters into usecase belong in `pkg/wiring/` (shared assembly helpers) or the composition root. If a usecase needs a default, the composition root injects it; the usecase only consumes the `ports` interface.

**3. `shared/` dependency direction is one-way and layered.**
- `shared/` MUST NOT import any `projects/*/internal/*` — shared code is downstream of nothing.
- `shared/algorithms/` MUST NOT import `shared/device-sdk/` — algorithms are device-agnostic.
- `shared/device-sdk/` MAY import `shared/algorithms/` but never project internals.
- `shared/motion-control/` MAY import `shared/algorithms/` and `shared/device-sdk/`.
- `shared/frontend/` MUST NOT import any project `frontend/src/` — it is consumed by them, not the reverse.

**4. `frontend/` — demo and test code are exempt, production code is not.**
The "zero calibration algorithms" rule applies to `frontend/src/features/`, `frontend/src/shared/`, `frontend/src/modules/`, `frontend/src/stores/`, and `frontend/src/views/`. Demo, mock, and test utilities under `frontend/src/__tests__/`, `frontend/src/mocks/`, or files matching `frontend/src/utils/simulate*` MAY contain algorithm copies for offline demonstration, provided:
- The file header contains `// DEMO ONLY — not for production use`.
- No production feature file imports it (enforced by `scripts/validate-frontend-structure.ps1`).

### Automated Enforcement

Structural rules are validated by scripts; import-direction rules are validated by `golangci-lint` + `depguard` and by `scripts/validate-import-direction.ps1` (which works without `golangci-lint` installed). See Commands below.

## Decision Tree: Where Does This Code Go?

```
Is it a business rule (calibration, measurement, acquisition logic)?
  → YES: core/<domain>/

Does it orchestrate multiple domains or external dependencies?
  → YES: usecase/

Is it an interface definition for an external dependency?
  → YES: ports/

Is it a concrete implementation of a port (device driver, DB access)?
  → YES: adapters/<type>/

Is it UI display or user interaction?
  → YES: apps/desktop-wails/frontend/src/modules/<domain>/

Is it Wails method binding (Go → JS bridge)?
  → YES: apps/desktop-wails/backend/bindings/

Can 2+ projects reuse this logic?
  → YES: shared/ (algorithms, device-sdk, motion-control, frontend, or contracts)

Is it reusable motion-control application logic (manager, profile store, HTTP route glue, status poller)?
  → YES: shared/motion-control/go/

Is it low-level device protocol, hardware adapter, serial transport, or FFI wrapper?
  → YES: shared/device-sdk/go/

Is it a standalone CLI tool?
  → YES: programs/<tool-name>/

Is it raw hardware documentation (PDF, datasheet, capture logs)?
  → YES: device-lab/
```

## Design Principles

Reference: `docs/runbooks/development-rules.md` (sections 8–12).

1. **Frontend-backend separation** — Frontend displays, backend decides. If swapping Vue for a web UI would still need this logic, put it in the Go backend.
2. **Program to interfaces** — All external deps via `ports`. Strategy pattern for multi-device. Observer pattern for real-time data. New device = new adapter, zero core changes.
3. **Readability first** — One function, one job. Business-domain names, no abbreviations. Max 3 nesting levels. Comments explain why, not what.
4. **Boundary defense** — Validate at edges (user input, device responses). Trust internal callers. Timeout + retry on all hardware I/O. No silent error swallowing.
5. **Long-term stability** — Explicit resource cleanup (defer, context). Pre-allocate buffers on hot paths. Log state changes at info, communication at debug. Externalize all config.

## Workspace Structure

- Structure is validated by `scripts/validate-structure.ps1`. Run before completing non-trivial work.
- Frontend directory structure is validated by `scripts/validate-frontend-structure.ps1`.
- New projects: `scripts/new-project.ps1 -Name <name>`.
- Structural changes require: update `workspace.structure.json` + document in `docs/decisions/`.
- Full rules: `docs/runbooks/workspace-directory-rules.zh-CN.md`.

## Commands

```powershell
# Structure validation
powershell -File .\scripts\validate-structure.ps1

# Import-direction validation (hexagonal boundaries) — no golangci-lint required
powershell -File .\scripts\validate-import-direction.ps1

# New project scaffold
powershell -File .\scripts\new-project.ps1 -Name project-gamma

# Go lint (gofmt + build) across all Go projects
powershell -File .\scripts\lint-go.ps1

# golangci-lint with depguard (enforces core/ports/usecase import rules)
# Install once: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run -c .golangci.yml ./...
```

Per-project commands will be added as projects get build tooling.

## Requirements

- **Go** — follow the version declared in `go.work` and each touched `go.mod`; `go build ./...` or `go test ./...` must pass for touched Go modules before committing.
- **Node.js** (LTS) — required for Vue 3 frontend builds.
- **Wails CLI v3** — required for desktop app generation/builds.

## Commit Rules

- Atomic commits grouped by logical intent.
- Conventional format: `feat(scope)`, `fix(scope)`, `refactor(scope)`, `docs`, `test(scope)`, `chore(scope)`.

## Code Standards

Detailed code structure, naming, comment, and writing conventions for AI-friendly maintainability:

- `docs/runbooks/code-standards.zh-CN.md` — Full specification (Chinese)
- `docs/runbooks/frontend-ai-rules.zh-CN.md` — AI-executable frontend rules for Vue/Wails UI work
- `docs/runbooks/frontend-directory-rules.zh-CN.md` — Frontend directory structure standard

## Language

- Documentation files: bilingual (English + Chinese) as needed.
- Code comments: English only.
- AI agent execution standard (Chinese): `docs/runbooks/ai-agent-execution-standard.zh-CN.md`.
- AI frontend execution rules (Chinese): `docs/runbooks/frontend-ai-rules.zh-CN.md`.
- Frontend directory structure (Chinese): `docs/runbooks/frontend-directory-rules.zh-CN.md`.

## More

- `docs/index.md` — Full documentation index (coding standards, architecture, runbooks, scripts).
- `docs/architecture/project-variants.md` — Approved project structure variants and examples.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **AI-WorkSpace** (27250 symbols, 54998 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
