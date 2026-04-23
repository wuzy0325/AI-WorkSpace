# Claude Code Workspace Rules

Claude Code must follow the same rules in `AGENTS.md`.

## Required Behavior

1. Never change workspace directory structure without explicit user approval.
2. Use `scripts/new-project.ps1` for new projects.
3. Run `scripts/validate-structure.ps1` before claiming work is complete.
4. Keep hardware dependencies out of core domain logic.
5. Keep desktop app code under `projects/*/apps/desktop-wails` (Vue 3 + Go + Wails).
6. Reuse shared code instead of duplicating: algorithms in `shared/algorithms`, device protocol code in `shared/device-sdk`, UI/composables in `shared/frontend`.
7. Keep Wails backend as app-shell/bindings glue; put business rules in `projects/*/services/api-go/internal/core`.
8. Run impacted tests and report concrete command outputs before marking tasks done.
9. Follow Chinese execution SOP when needed: `docs/runbooks/ai-agent-execution-standard.zh-CN.md`.

If a requested change conflicts with these rules, stop and ask for confirmation with a concrete structure diff.
