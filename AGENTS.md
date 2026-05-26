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
| `api/` (server handlers) | zero domain logic — thin HTTP routing, delegate to `usecase` |
| `adapters/hardware/` | zero domain logic — pure protocol translation and I/O |
| `apps/desktop-wails/backend/` | zero domain business logic — parameter conversion + usecase calls only |
| `apps/desktop-wails/frontend/` | zero direct hardware access, zero calibration algorithms |
| `programs/` | zero project `internal/*` imports — depend on `shared/*` only |

### Commands

```powershell
powershell -File .\scripts\validate-structure.ps1   # Structure check
powershell -File .\scripts\new-project.ps1 -Name foo  # New project
```

### Environment Requirements

- **Rust toolchain** (stable) — required for building any project in this workspace.
- **Node.js** (LTS) — required for Vue 3 Tauri frontend builds.

### Pre-submit Checklist

Before committing, run:

1. `powershell -File .\scripts\validate-structure.ps1` — must pass
2. `cargo check --workspace` — must pass (requires Rust toolchain)

See CLAUDE.md for complete rules, decision tree, and design principles.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **AI-WorkSpace** (6759 symbols, 16083 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/AI-WorkSpace/context` | Codebase overview, check index freshness |
| `gitnexus://repo/AI-WorkSpace/clusters` | All functional areas |
| `gitnexus://repo/AI-WorkSpace/processes` | All execution flows |
| `gitnexus://repo/AI-WorkSpace/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
