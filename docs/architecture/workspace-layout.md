# Workspace Layout

## Goal

This workspace layout separates Tauri desktop app code (Vue 3 + Rust shell), Rust backend business logic, hardware integration, shared algorithms, and standalone programs.

## Layering Rules

1. `projects/*/services/api-rs/src/core` contains pure domain logic.
2. `projects/*/services/api-rs/src/ports` defines traits for external dependencies.
3. `projects/*/services/api-rs/src/adapters/hardware` implements hardware-specific ports.
4. `projects/*/apps/desktop-tauri` hosts the Tauri desktop app (Vue 3 frontend + Rust shell/commands).
5. `shared/device-sdk` hosts reusable device communication primitives.
6. `shared/algorithms` hosts reusable algorithm code that is not tied to hardware drivers.

## Why This Matters

- Business rules stay testable without devices.
- Hardware protocol changes stay in adapter layers.
- Desktop shell changes stay in `apps/desktop-tauri` and do not leak into `core`.
- Algorithm libraries stay reusable across projects and standalone tools.
- New programs in `programs/` can reuse `shared/*` safely.
