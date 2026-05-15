# Workspace Layout

## Goal

This workspace layout separates Wails desktop app code (Vue 3 + Go), business logic, hardware integration, shared algorithms, and standalone programs.

## Layering Rules

1. `projects/*/services/api-go/internal/core` contains pure domain logic.
2. `projects/*/services/api-go/internal/ports` defines interfaces for external dependencies.
3. `projects/*/services/api-go/internal/adapters/hardware` implements hardware-specific ports.
4. `projects/*/apps/desktop-wails` hosts Wails desktop shell (Vue 3 frontend + Go host/bindings).
5. `shared/device-sdk` hosts reusable device communication primitives.
6. `shared/algorithms` hosts reusable algorithm code that is not tied to hardware drivers.

## Why This Matters

- Business rules stay testable without devices.
- Hardware protocol changes stay in adapter layers.
- Desktop shell changes stay in `apps/desktop-wails` and do not leak into `core`.
- Algorithm libraries stay reusable across projects and standalone tools.
- New programs in `programs/` can reuse `shared/*` safely.
