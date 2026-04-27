# Workspace Agent Rules

This file provides guidance to AI agents (Claude Code, OpenCode, or others) when working in this workspace.

> **Single source of truth:** This file is a concise pointer document.
> All authoritative architecture, coding rules, and conventions
> live in **CLAUDE.md** at the workspace root. Read that file first.

## Quick Reference

### Architecture

Go backend + Vue 3 frontend via Wails, hexagonal architecture per project.

- `projects/<name>/apps/desktop-wails/` — Wails desktop app (Vue 3 + Go bindings)
- `projects/<name>/services/api-go/internal/` — Go backend (core → usecase → ports → adapters)
- `shared/` — Cross-project reusable code (algorithms, device-sdk, frontend, contracts)
- `programs/` — Standalone CLI tools (`shared/*` only, never project `internal/*`)
- `device-lab/` — Hardware lab artifacts (raw docs, firmware, captures)
- `docs/` — Architecture, decisions, runbooks

### Hard Constraints (zero-tolerance)

| Location | Constraint |
|---|---|---|
| `core/` | zero hardware imports, zero file I/O, zero serial/network, zero framework imports |
| `ports/` | zero implementations — interface definitions only |
| `usecase/` | zero direct hardware calls — go through `ports` interfaces |
| `adapters/hardware/` | zero business logic — pure protocol translation and I/O |
| `apps/desktop-wails/backend/` | zero business logic — parameter conversion + usecase calls only |
| `apps/desktop-wails/frontend/` | zero direct hardware access, zero calibration algorithms |
| `programs/` | zero project `internal/*` imports — depend on `shared/*` only |

### Commands

```powershell
powershell -File .\scripts\validate-structure.ps1   # Structure check
powershell -File .\scripts\new-project.ps1 -Name foo  # New project
```

See CLAUDE.md for complete rules, decision tree, and design principles.
