# ADR-001: Multi-Project Workspace Layout

## Status

Accepted (2026-04-23)

Updated (2026-05-12): desktop/backend stack changed to Tauri + Rust.

## Context

The workspace must host multiple industrial DAQ desktop projects that share device SDK code, algorithms, contracts, and Vue 3 frontend components. Hardware integration, acquisition workflows, calibration logic, and motion control must remain decoupled from UI and framework concerns.

The current desktop stack is Tauri 2 with a Vue 3 frontend and Rust backend crates. The previous desktop/backend stack is no longer part of the active architecture.

## Decision

Adopt a layered monorepo layout:

- **projects/<name>/**: per-project product code, including a Tauri desktop app and Rust backend service.
- **projects/<name>/apps/desktop-tauri/**: desktop shell, Vue 3 frontend, and thin Tauri command bridge.
- **projects/<name>/services/api-rs/**: Rust backend service using hexagonal architecture (`core` -> `usecase` -> `ports` -> `adapters`).
- **shared/**: cross-project reusable code, including algorithms, device SDKs, frontend components, and contracts.
- **programs/**: standalone CLI/tools that depend only on `shared/*`, never on project-private service internals.
- **device-lab/**: hardware lab artifacts such as rigs, captures, firmware, and driver docs.
- **docs/decisions/**: architecture decision records.
- **tools/**: development environment helpers.

The structure is enforced with `workspace.structure.json` and `scripts/validate-structure.ps1`.

## Consequences

- Business logic in `services/api-rs/src/core` stays testable without hardware.
- Tauri remains a thin desktop shell, not a business logic host.
- Hardware protocol changes stay in `services/api-rs/src/adapters/hardware` or reusable SDK code under `shared/device-sdk`.
- New projects should be scaffolded with `scripts/new-project.ps1`.
- Structural changes require updating `workspace.structure.json` and the relevant documentation.
- Programs in `programs/` must not depend on project-private service code.
