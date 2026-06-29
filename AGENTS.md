# Workspace Agent Rules

This file provides guidance to AI agents (Claude Code, OpenCode, or others) when working in this workspace.

> **Context-loading rule:** This file is the startup entrypoint, not the full rulebook.
> Load it first, then load `CLAUDE.md`, project docs, and runbooks **only when the task needs them**.
> See `docs/architecture/ai-context-loading.zh-CN.md` for the progressive loading protocol.

## Quick Reference

### Architecture

Go backend + Vue 3 frontend via Wails, using hexagonal boundaries. Projects may use either a split service layout or an approved single-module Wails layout.

- `projects/<name>/apps/desktop-wails/` — Wails desktop app (Vue 3 + Go host/bindings)
- `projects/<name>/services/api-go/internal/` — split Go backend for larger projects (core -> usecase -> ports -> adapters)
- `projects/<name>/apps/desktop-wails/{core,ports,usecase,adapters}` — approved single-module layout for small standalone Wails apps such as `daq-t1603`
- `shared/` — cross-project reusable code (algorithms, device-sdk, motion-control, frontend, contracts)
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

- **Go** (workspace target follows `go.work`) — required for backend and Wails builds.
- **Node.js** (LTS) — required for Vue 3 frontend builds.
- **Wails CLI** (v3 for all projects: `wind-daq`, `daq-t1603`, `daq-p1604`, `motion-controller`, `five-hole-interpolator`, `three-hole-interpolator`) — required for desktop app generation/builds. See `docs/decisions/ADR-004-wails-v3-production-build.md` for production build tag rules.

### Pre-submit Checklist

Before committing, run the checks that apply to the touched project:

1. `powershell -File .\scripts\validate-structure.ps1` — must pass
2. `powershell -File .\scripts\validate-frontend-structure.ps1 -ProjectDir "projects/<name>/apps/desktop-wails/frontend/src"` — when modifying frontend files or directories
3. Go project checks: `go test ./...` or the project-specific command in `projects/<name>/README.md` / `CLAUDE.md`
4. Frontend checks: `npm run typecheck`, `npm run build`, and project tests when present
5. `powershell -File .\scripts\check-naive-imports.ps1 -ProjectDir "projects/wind-daq/apps/desktop-wails/frontend/src"` — when modifying wind-daq frontend files (prevents direct naive-ui imports in feature code)

See CLAUDE.md for complete rules, decision tree, and design principles.

### Packaging / Release Rule

Before creating any deliverable package, installer, release build, or user-facing `wails build` output, agents must follow `docs/runbooks/release-versioning.zh-CN.md`: update the target project version, changelog, per-version release note, run applicable verification, ensure production build tags (`-tags production`) and `env: GOWORK: off`, use `task release` when applicable, and report the final artifact path.

See `docs/decisions/ADR-004-wails-v3-production-build.md` for production build tag constraints and the Install / Upgrade release-note requirement.

### Progressive Loading

Load documents in this order unless the task is trivial:

1. `AGENTS.md` — startup boundaries and navigation only
2. `CLAUDE.md` — workspace architecture and hard constraints when architecture or implementation is involved
3. `docs/architecture/workspace-engineering-rules.zh-CN.md` — integrated engineering rules when deciding boundaries, frontend/backend split, UI architecture, or coding approach
4. Project docs under `projects/<name>/` — only after the task is scoped to that project
5. Topic docs in `docs/runbooks/` and `docs/architecture/` — only when the task specifically needs them

For frontend UI, layout, component, style, store, API-client, or Wails frontend tasks, load `docs/runbooks/frontend-ai-rules.zh-CN.md` before editing.
For frontend directory structure decisions, load `docs/runbooks/frontend-directory-rules.zh-CN.md` before creating new files.

For packaging, release, installer, or deliverable build tasks, load `docs/runbooks/release-versioning.zh-CN.md` before building.

For task-specific loading paths, use `docs/architecture/ai-task-context-map.zh-CN.md`.

Do not load the entire `docs/` tree by default.

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **AI-WorkSpace** (27250 symbols, 54998 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
