# ADR-001: Multi-Project Workspace Layout

## Status

Accepted (2026-04-23)

## Context

Need a workspace that hosts multiple Wails desktop app projects sharing Go backend libraries, device SDK, and Vue 3 frontend components. Hardware integration (DAQ devices, probes, actuators) must stay decoupled from business logic.

## Decision

Adopt a layered monorepo layout:

- **projects/<name>/** — per-project Wails app + Go service, using hexagonal architecture (core → usecase → ports → adapters)
- **shared/** — cross-project reusable code (algorithms, device-sdk, frontend components, contracts)
- **programs/** — standalone CLI tools that depend only on shared/*
- **device-lab/** — hardware lab artifacts (rigs, captures, firmware, driver docs) kept separate from source code
- **docs/decisions/** — architecture decision records
- **tools/** — development environment helpers (Docker, devcontainer)

Enforced via `workspace.structure.json` + `scripts/validate-structure.ps1`.

## Consequences

- Business logic in `core` stays testable without hardware
- New projects clone the wind-daq skeleton via `scripts/new-project.ps1`
- Structural changes require updating structure.json and documenting in docs/decisions/
- Programs in programs/ must not depend on project internal packages
