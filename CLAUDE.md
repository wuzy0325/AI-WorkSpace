# CLAUDE.md

This file provides guidance to AI agents (Claude Code, OpenCode, or others) when working with code in this workspace.

## Project Context

Industrial DAQ (Data Acquisition) desktop platform — Tauri desktop app (Vue 3 + Rust) for wind tunnel and lab measurement.

- Multiple hardware devices: DAQ cards, stepping motors, pressure probes, position actuators
- Calibration workflows with guided procedures
- Real-time waveform display and high-frequency data acquisition
- Built for small engineering teams in testing labs

## Architecture

**Rust backend + Tauri desktop app with Vue 3 frontend, with hexagonal architecture per project.**

```
projects/<project>/
├── apps/desktop-tauri/
│   ├── frontend/          # Vue 3 UI (display + interaction only)
│   └── src-tauri/         # Tauri shell and command bridge (zero business logic)
├── services/api-rs/
│   ├── Cargo.toml         # Rust backend crate
│   └── src/
│       ├── bin/           # Server entry points (wiring only)
│       ├── core/          # Pure domain logic (zero hardware, zero I/O)
│       ├── usecase/       # Orchestration (coordinates core + ports)
│       ├── ports/         # Trait definitions (zero implementations)
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

- `shared/algorithms/` — Reusable computation, zero device dependencies
- `shared/device-sdk/` — Reusable device protocol/transport primitives
- `shared/frontend/` — Reusable Vue 3 components/composables
- `shared/contracts/` — Cross-project API contracts

Standalone tools:

- `programs/` — CLI utilities (calibrator, serial monitor, firmware upgrader). Depend on `shared/*` only, never on project `internal/*`.

Hardware lab artifacts:

- `device-lab/` — Raw driver docs, firmware, captures, rig diagrams. Reference material, not source code.

### Key Architectural Decisions

**Hexagonal (ports & adapters):** Business logic in `core` is completely isolated from hardware, databases, and UI. External dependencies are defined as Rust traits in `ports`, implemented in `adapters`. This means `core` is testable without any device connected.

**Dependency direction:** `usecase → core + ports`. `core` never imports `ports` or `adapters`. Adapters implement port interfaces. The dependency graph is strictly one-way.

**Tauri as thin shell:** The Tauri shell (`apps/desktop-tauri/src-tauri/`) owns the desktop window, asset hosting, native dialogs, and thin command/process bridge to the Rust backend. No business logic, no hardware calls. If logic needs to survive a UI framework change, it belongs in `services/api-rs/src/`.

**Frontend owns display only:** Vue 3 components handle UI state, visualization, and user interaction. All business rules (calibration algorithms, measurement processing, device control) live in the Rust backend.

### Module Design

See `docs/architecture/module-design.md` for detailed Rust crate and Vue 3 module structure.

## Hard Constraints

These are zero-tolerance rules. Violating them breaks the architecture:

| Location | Constraint |
|---|---|
| `services/api-rs/src/core/` | zero hardware imports, zero file I/O, zero serial/network, zero framework imports |
| `services/api-rs/src/ports/` | zero implementations — trait definitions only |
| `services/api-rs/src/usecase/` | zero direct hardware calls — go through `ports` traits |
| `services/api-rs/src/adapters/hardware/` | zero business logic — pure protocol translation and I/O |
| `apps/desktop-tauri/src-tauri/` | zero business logic — Tauri startup, asset hosting, native shell, command bridge only |
| `apps/desktop-tauri/frontend/` | zero direct hardware access, zero calibration algorithms |
| `programs/` | zero project private service imports — depend on `shared/*` only |

## Decision Tree: Where Does This Code Go?

```
Is it a business rule (calibration, measurement, acquisition logic)?
  → YES: services/api-rs/src/core/<domain>/

Does it orchestrate multiple domains or external dependencies?
  → YES: services/api-rs/src/usecase/

Is it an interface definition for an external dependency?
  → YES: services/api-rs/src/ports/

Is it a concrete implementation of a port (device driver, DB access)?
  → YES: services/api-rs/src/adapters/<type>/

Is it UI display or user interaction?
  → YES: apps/desktop-tauri/frontend/src/modules/<domain>/

Is it desktop shell startup, asset hosting, or Rust backend process/HTTP bridge?
  → YES: apps/desktop-tauri/src-tauri/

Can 2+ projects reuse this logic?
  → YES: shared/ (algorithms, device-sdk, or frontend)

Is it a standalone CLI tool?
  → YES: programs/<tool-name>/

Is it raw hardware documentation (PDF, datasheet, capture logs)?
  → YES: device-lab/
```

## Design Principles

Reference: `docs/runbooks/development-rules.md` (sections 8–12).

1. **Frontend-backend separation** — Frontend displays, backend decides. If swapping Vue for a web UI would still need this logic, put it in the Rust backend.
2. **Program to interfaces** — All external deps via `ports`. Strategy pattern for multi-device. Observer pattern for real-time data. New device = new adapter, zero core changes.
3. **Readability first** — One function, one job. Business-domain names, no abbreviations. Max 3 nesting levels. Comments explain why, not what.
4. **Boundary defense** — Validate at edges (user input, device responses). Trust internal callers. Timeout + retry on all hardware I/O. No silent error swallowing.
5. **Long-term stability** — Explicit resource cleanup with Rust ownership, RAII, `Drop`, and cancellation. Pre-allocate buffers on hot paths. Log state changes at info, communication at debug. Externalize all config.

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

# Rust backend check
cargo check --workspace
```

Per-project commands will be added as projects get build tooling.

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
