# Development Rules

For Chinese execution SOP, see `docs/runbooks/ai-agent-execution-standard.zh-CN.md`.

## 1) Directory Structure Discipline

- Do not change top-level directories without explicit user request.
- Do not add, remove, rename, or move base directories under `shared/`, `device-lab/`, `docs/`, or `tools/` unless requested.
- Use `scripts/new-project.ps1` to create a new project structure.
- Run `scripts/validate-structure.ps1` before completing non-trivial changes.
- For desktop projects, keep the Tauri app shell under `projects/*/apps/desktop-tauri`.

## 2) Dependency Boundaries

- Core domain logic must not import hardware SDKs directly.
- Hardware dependencies live in `adapters/hardware` or `shared/device-sdk`.
- Shared algorithms live in `shared/algorithms` and are hardware-agnostic.
- Standalone tools in `programs/` should use `shared/*` but not project `internal/*` packages.
- Tauri UI/shell bridge code should stay in `apps/desktop-tauri`; keep domain logic in `services/api-rs/src/*`.

## 3) Testing Strategy

- Unit tests focus on `core` and `usecase` layers.
- Integration tests are in `tests/integration`.
- Hardware-in-loop tests are in `tests/hil` only.
- Keep simulation data and protocol captures in `device-lab/` or project fixtures.

## 4) Structural Changes Process

1. Propose the exact directory change first.
2. Update `workspace.structure.json`.
3. Record rationale in `docs/decisions/`.
4. Re-run `scripts/validate-structure.ps1`.

## 5) Reuse-First Rule (No Duplication)

- If the same logic appears in 2 or more projects, extract it to `shared/*`.
- Use `shared/device-sdk` for reusable hardware protocol/transport logic.
- Use `shared/algorithms` for reusable algorithm logic.
- Use `shared/frontend` for reusable Vue components/composables.
- Avoid copying the same implementation into multiple project folders.

## 6) Desktop Tauri Layering

- `projects/*/apps/desktop-tauri/frontend`: desktop UI only (Vue 3); no direct hardware/DB adapter dependencies.
- `projects/*/apps/desktop-tauri/src-tauri`: Tauri shell, startup wiring, native capabilities, and command bridge only.
- `projects/*/services/api-rs/src/core`: core business rules, hardware-agnostic.
- `projects/*/services/api-rs/src/usecase` + `ports`: orchestrate domain and external traits.
- `projects/*/services/api-rs/src/adapters/*`: concrete infra/hardware implementations.

## 7) Verification Loop Before Completion

- For every non-trivial task, run structure validation and impacted tests.
- If verification fails, fix issues and rerun until passing.
- Report command outputs when summarizing completion.

Suggested baseline:

```powershell
powershell -File .\scripts\validate-structure.ps1
```

## 8) Frontend-Backend Separation

- Frontend (Vue 3) handles **display and interaction only**: UI state management, user input validation, data visualization.
- Backend (Rust) owns **business logic and data control**: device communication, data processing, calibration algorithms, state machines.
- The Tauri command layer is a thin bridge with no business logic. Frontend calls Rust backend APIs or thin Tauri commands, never touches hardware or the file system directly.
- Rule of thumb: if a piece of logic would still work behind a web UI, it belongs in the backend.

## 9) Program to Interfaces, Depend on Abstractions

- All external dependencies (devices, databases, message queues) are defined as traits in the `ports` layer; upper layers depend only on traits.
- Use the **Strategy pattern** for multi-device adaptation: devices of the same type implement a common interface (e.g. `PressureTransducer`); the business layer is unaware of specific models.
- Use the **Observer pattern** for real-time data streams: the acquisition service publishes data, the UI layer subscribes, decoupling acquisition rate from render rate.
- Adding a new device means adding an adapter implementation only — no changes to `core` or `usecase`.

## 10) Readability First

- Functions/methods do one thing; names reveal intent. If over 20 lines, consider splitting.
- Use business-domain names, not abbreviations. `sampleRate` over `sr`, `calibrationCoeff` over `cc`.
- Avoid nesting deeper than 3 levels. Use early returns and guard clauses to reduce indentation.
- Comments explain **why**, never **what** — the code itself should explain what.

## 11) Boundary Defense and Robustness

- Validate at system boundaries (user input, device responses, external files). Trust internal callers.
- Device communication must have timeouts, retries, and error recovery. Never assume hardware always responds normally.
- All operations that can fail (file I/O, network, serial port) must handle errors explicitly. Silent error swallowing is prohibited.
- Long-running acquisition tasks: implement graceful shutdown with cancellation tokens, stop signals, or async task coordination; never rely on force-killing.

## 12) Long-Term Stability

- Resources must be explicitly released (serial connections, file handles, async tasks). Use Rust ownership, RAII, `Drop`, cancellation, and scoped handles to guarantee cleanup.
- Avoid frequent memory allocation on hot paths (high-frequency acquisition). Pre-allocate and reuse buffers.
- Log state changes for post-mortem analysis. Key operations (calibration, device connect/disconnect) at info level; communication details at debug level.
- Externalize configuration: serial port names, sample rates, device addresses — never hardcode. Read from config files or database.
