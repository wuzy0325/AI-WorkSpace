# AI Workspace

This workspace hosts multiple product projects that share frontend code, Go backend libraries, and hardware integration utilities.

## Top-Level Layout

- `projects/`: business projects (Wails desktop app + Vue 3 frontend + Go backend)
- `shared/`: reusable algorithms, device SDK, contracts, and frontend shared modules
- `programs/`: standalone utilities and small tools
- `device-lab/`: hardware rigs, captures, firmware, and lab tooling
- `docs/`: architecture notes, decisions, and runbooks
- `scripts/`: workspace automation scripts
- `tools/`: container and environment helpers

## Important Rules

1. Do not change workspace directory structure directly.
2. Use `scripts/new-project.ps1` to add a new project skeleton.
3. Run `scripts/validate-structure.ps1` before and after major development tasks.
4. If structure must change, update `workspace.structure.json` and document the reason in `docs/decisions/`.

## Quick Commands

```powershell
powershell -File .\scripts\validate-structure.ps1
powershell -File .\scripts\new-project.ps1 -Name project-gamma
```

## 中文目录规则

- 目录使用规则与场景说明：`docs/runbooks/workspace-directory-rules.zh-CN.md`
- AI Agent 执行标准：`docs/runbooks/ai-agent-execution-standard.zh-CN.md`
