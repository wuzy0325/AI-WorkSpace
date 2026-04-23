# Development Rules

For Chinese execution SOP, see `docs/runbooks/ai-agent-execution-standard.zh-CN.md`.

## 1) Directory Structure Discipline

- Do not change top-level directories without explicit user request.
- Do not add, remove, rename, or move base directories under `shared/`, `device-lab/`, `docs/`, or `tools/` unless requested.
- Use `scripts/new-project.ps1` to create a new project structure.
- Run `scripts/validate-structure.ps1` before completing non-trivial changes.
- For desktop projects, keep Wails app shell under `projects/*/apps/desktop-wails`.

## 2) Dependency Boundaries

- Core domain logic must not import hardware SDKs directly.
- Hardware dependencies live in `adapters/hardware` or `shared/device-sdk`.
- Shared algorithms live in `shared/algorithms` and are hardware-agnostic.
- Standalone tools in `programs/` should use `shared/*` but not project `internal/*` packages.
- Wails UI/backend bridge code should stay in `apps/desktop-wails`; keep domain logic in `services/api-go/internal/*`.

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

## 6) Desktop Wails Layering

- `projects/*/apps/desktop-wails/frontend`: desktop UI only (Vue 3); no direct hardware/DB adapter dependencies.
- `projects/*/apps/desktop-wails/backend`: Wails app shell and bindings only.
- `projects/*/services/api-go/internal/core`: core business rules, hardware-agnostic.
- `projects/*/services/api-go/internal/usecase` + `ports`: orchestrate domain and external interfaces.
- `projects/*/services/api-go/internal/adapters/*`: concrete infra/hardware implementations.

## 7) Verification Loop Before Completion

- For every non-trivial task, run structure validation and impacted tests.
- If verification fails, fix issues and rerun until passing.
- Report command outputs when summarizing completion.

Suggested baseline:

```powershell
powershell -File .\scripts\validate-structure.ps1
```
