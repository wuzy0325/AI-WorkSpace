# Workspace Agent Rules

> Startup entrypoint only. Load `CLAUDE.md`, project docs, and runbooks on demand.
> Progressive loading protocol: [docs/architecture/ai-context-loading.zh-CN.md](docs/architecture/ai-context-loading.zh-CN.md) (L1-L5 token budgets + section-level precise loading).

## Quick Reference

### Architecture

Go backend (hexagonal) + Vue 3 + Wails, multi-project. Split layout: `apps/desktop-wails/{frontend,backend}` + `services/api-go/internal/{core,usecase,ports,adapters}`. Single-module: `apps/desktop-wails/{core,ports,usecase,adapters}` (small apps like `wista`). `shared/` = cross-project; `programs/` = CLI (`shared/*` only). Full: [CLAUDE.md](CLAUDE.md).

### Hard Constraints (zero-tolerance)

| Location | Constraint |
|---|---|
| `core/` | zero hardware imports, zero file I/O, zero serial/network, zero framework |
| `ports/` | zero implementations 鈥?interfaces only |
| `usecase/` | zero direct hardware calls 鈥?go through `ports` |
| `api/` | zero domain logic 鈥?thin routing, delegate to `usecase` |
| `adapters/hardware/` | zero domain logic 鈥?pure protocol translation + I/O |
| `apps/desktop-wails/backend/` | zero business logic 鈥?param conversion + usecase calls |
| `apps/desktop-wails/frontend/` | zero hardware access, zero calibration algorithms |
| `programs/` | zero project `internal/*` imports 鈥?`shared/*` only |
| `*.go` (non-test) | 鈮?500 lines per file, 鈮?50 lines per function 鈥?enforced by `validate-structure.ps1` against `scripts/go-file-waivers.txt`; see [code-standards 搂涓€](docs/runbooks/code-standards.zh-CN.md) |

### Environment & Commands

Go + Node.js LTS + Wails v3. `wista` excluded from `go.work` (ADR-006) 鈫?`$env:GOWORK="off"`. Production: `-tags production` + `GOWORK=off` (ADR-004). Scripts: `validate-structure.ps1`, `validate-frontend-structure.ps1 -CheckFileSize`, `check-wails-bindings.ps1`. Full list: [docs/index.md](docs/index.md) 搂浜?

### Windows Network I/O Constraint

- **姘歌繙涓嶈鎶?socket deadline 浣滀负鏈夌晫纭欢 I/O 鐨勫敮涓€鍙栨秷鏈哄埗**銆俆reat socket deadlines as soft timeouts only. 杩欐槸 Go 鍦?Windows 涓婄殑宸茬煡鍐呮牳绾ч棶棰橈紙璇﹁ [go-windows-known-issues](docs/runbooks/go-windows-known-issues.zh-CN.md)锛夈€?- Handshake銆乨iscovery銆乧ommand-response銆乻top銆乨isconnect 璺緞**蹇呴』**鏈夌嫭绔?owner 鍙洿鎺ヨ皟鐢?`conn.Close()`锛屼笉绛夊緟闃诲 goroutine銆傝 watchdog 鍏抽棴鐨勮繛鎺ヤ笉鍙鐢ㄣ€?- 鏈夌晫缃戠粶 I/O 鐨勬祴璇?*蹇呴』**鍖呭惈蹇界暐 deadline銆佸彧鍦?`Close` 鍚庤繑鍥炵殑杩炴帴 double銆傚畬鏁村喅绛栵細[ADR-009](docs/decisions/ADR-009-windows-network-deadline-fallback.md)銆?
### Pre-submit / Release / Loading

- **Pre-submit**: `validate-structure.ps1` + `go test ./...` + `npm run typecheck` + `npm run build`. Wails binding signature changes 鈫?`wails3 generate bindings -silent` (TS-binding projects `wista` / `wispa` regenerate manually). Details: [development-rules.md](docs/runbooks/development-rules.md) + [frontend-ai-rules-deploy 搂32.1](docs/runbooks/frontend-ai-rules-deploy.zh-CN.md#321-wails-缁戝畾鍚屾寮哄埗闆跺蹇?.
- **Release**: [release-versioning.zh-CN.md](docs/runbooks/release-versioning.zh-CN.md) 鈥?version + changelog + note + `task release` + report artifact.
- **Loading**: L1 (this file) 鈫?L2 ([CLAUDE.md](CLAUDE.md) + [workspace-engineering-rules](docs/architecture/workspace-engineering-rules.zh-CN.md)) 鈫?L3 (runbooks) 鈫?L4 (`projects/<name>/`) 鈫?L5 (source + tests). **Never load entire `docs/` by default.** Task map: [ai-task-context-map](docs/architecture/ai-task-context-map.zh-CN.md).

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **AI-WorkSpace** (57135 symbols, 119162 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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
