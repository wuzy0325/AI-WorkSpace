# CLAUDE.md

This file provides guidance to AI agents (Claude Code, OpenCode, or others) when working with code in this workspace.

## Project Context

Industrial DAQ (Data Acquisition) desktop platform — Wails app (Vue 3 + Go) for wind tunnel and lab measurement.

- Multiple hardware devices: DAQ cards, stepping motors, pressure probes, position actuators
- Calibration workflows with guided procedures
- Real-time waveform display and high-frequency data acquisition
- Built for small engineering teams in testing labs

## Architecture

**Go backend + Vue 3 frontend via Wails, with hexagonal architecture per project.**

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
  → YES: shared/ (algorithms, device-sdk, or frontend)

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
- New projects: `scripts/new-project.ps1 -Name <name>`.
- Structural changes require: update `workspace.structure.json` + document in `docs/decisions/`.
- Full rules: `docs/runbooks/workspace-directory-rules.zh-CN.md`.

## Commands

```powershell
# Structure validation
powershell -File .\scripts\validate-structure.ps1

# New project scaffold
powershell -File .\scripts\new-project.ps1 -Name project-gamma

# Go lint (gofmt + build) across all Go projects
powershell -File .\scripts\lint-go.ps1
```

Per-project commands will be added as projects get build tooling.

## Requirements

- **Go** (1.21+) — `go build ./...` must pass before committing.
- **Node.js** (LTS) — required for Vue 3 frontend builds.

## Commit Rules

- Atomic commits grouped by logical intent.
- Conventional format: `feat(scope)`, `fix(scope)`, `refactor(scope)`, `docs`, `test(scope)`, `chore(scope)`.

## Code Standards

Detailed code structure, naming, comment, and writing conventions for AI-friendly maintainability:

- `docs/runbooks/code-standards.zh-CN.md` — Full specification (Chinese)

## Language

- Documentation files: bilingual (English + Chinese) as needed.
- Code comments: English only.
- AI agent execution standard (Chinese): `docs/runbooks/ai-agent-execution-standard.zh-CN.md`.

## More

- `docs/index.md` — Full documentation index (coding standards, architecture, runbooks, scripts).

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **AI-WorkSpace** (6916 symbols, 16542 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
