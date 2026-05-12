# Workspace Agent Rules

This file provides guidance to AI agents (Claude Code, OpenCode, or others) when working in this workspace.

> **Single source of truth:** This file is a concise pointer document.
> All authoritative architecture, coding rules, and conventions
> live in **CLAUDE.md** at the workspace root. Read that file first.

## Quick Reference

### Architecture

Rust backend + Tauri desktop app with Vue 3 frontend, hexagonal architecture per project.

- `projects/<name>/apps/desktop-tauri/` — Tauri desktop app (Vue 3 frontend + Rust shell/commands)
- `projects/<name>/services/api-rs/src/` — Rust backend (core → usecase → ports → adapters)
- `shared/` — Cross-project reusable code (algorithms, device-sdk, frontend, contracts)
- `programs/` — Standalone CLI tools (`shared/*` only, never project `internal/*`)
- `device-lab/` — Hardware lab artifacts (raw docs, firmware, captures)
- `docs/` — Architecture, decisions, runbooks

### Hard Constraints (zero-tolerance)

| Location | Constraint |
|---|---|---|
| `services/api-rs/src/core/` | zero hardware imports, zero file I/O, zero serial/network, zero framework imports |
| `services/api-rs/src/ports/` | zero implementations — trait definitions only |
| `services/api-rs/src/usecase/` | zero direct hardware calls — go through `ports` traits |
| `services/api-rs/src/adapters/hardware/` | zero business logic — pure protocol translation and I/O |
| `apps/desktop-tauri/src-tauri/` | zero business logic — Tauri startup, native shell, command bridge only |
| `apps/desktop-tauri/frontend/` | zero direct hardware access, zero calibration algorithms |
| `programs/` | zero project private service imports — depend on `shared/*` only |

### Commands

```powershell
powershell -File .\scripts\validate-structure.ps1   # Structure check
powershell -File .\scripts\new-project.ps1 -Name foo  # New project
```

See CLAUDE.md for complete rules, decision tree, and design principles.
