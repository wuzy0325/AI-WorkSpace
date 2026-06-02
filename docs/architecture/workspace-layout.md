# Workspace Layout

## Goal

This workspace layout separates Wails desktop app code (Vue 3 + Go), business logic, hardware integration, shared algorithms, and standalone programs.

## Layering Rules

1. `projects/*/services/api-go/internal/core` contains pure domain logic for split-service projects.
2. `projects/*/services/api-go/internal/ports` defines interfaces for external dependencies.
3. `projects/*/services/api-go/internal/adapters/hardware` implements hardware-specific ports.
4. `projects/*/apps/desktop-wails` hosts Wails desktop shell (Vue 3 frontend + Go host/bindings).
5. Approved single-module Wails apps may place `core`, `ports`, `usecase`, and `adapters` directly under `apps/desktop-wails`.
6. `shared/device-sdk` hosts reusable low-level device communication primitives, protocols, hardware adapters, serial transport, and FFI wrappers.
7. `shared/motion-control` hosts reusable application-level motion orchestration above the device SDK.
8. `shared/algorithms` hosts reusable algorithm code that is not tied to hardware drivers.

## Why This Matters

- Business rules stay testable without devices.
- Hardware protocol changes stay in adapter layers.
- Desktop shell changes stay in `apps/desktop-wails` and do not leak into `core`.
- Algorithm libraries stay reusable across projects and standalone tools.
- New programs in `programs/` can reuse `shared/*` safely.
- Shared motion behavior is centralized without making one product import another product's `internal/*` package.

## Approved Variants

See `project-variants.md` for the current variants and examples.
