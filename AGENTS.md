# Workspace Agent Rules

These rules apply to any AI agent operating in this workspace (OpenCode CLI, Claude Code CLI, or others).

## Structure Safety (Mandatory)

1. Do not create, rename, move, or delete top-level directories unless the user explicitly asks for it.
2. Do not change base shared folders under `shared/`, `device-lab/`, `docs/`, or `tools/` unless the user explicitly asks for it.
3. Do not manually scaffold new business projects. Use `scripts/new-project.ps1`.
4. Before finishing any non-trivial task, run `scripts/validate-structure.ps1`.
5. If a task requires structural change, propose the exact change first, then update `workspace.structure.json` together with docs.

## Dependency Boundaries (Mandatory)

1. Project domain logic in `projects/*/services/api-go/internal/core` must stay hardware-agnostic.
2. Hardware dependencies must remain in adapter layers (`adapters/hardware`) or `shared/device-sdk`.
3. Reusable algorithm code goes to `shared/algorithms`.
4. Standalone tools belong to `programs/*` and should not depend on project `internal/*` packages.

## Working Mode

1. Prefer minimal, local changes.
2. Keep structure stable; change files before changing folders.
3. Record structural decisions in `docs/decisions/`.

## Reuse and Boundary Rules (Mandatory)

1. If the same logic appears in 2 or more projects, extract it to `shared/*` instead of copy-pasting.
2. Put reusable device protocol/transport code in `shared/device-sdk`.
3. Put reusable algorithm code in `shared/algorithms`.
4. Put reusable Vue UI/composables in `shared/frontend`.
5. `projects/*/apps/desktop-wails/frontend` must not directly depend on hardware drivers or DB adapters.
6. `projects/*/apps/desktop-wails/backend` is for Wails bindings/app shell glue, not core domain rules.
7. Core business rules stay in `projects/*/services/api-go/internal/core` and are accessed through `usecase/ports`.

## Testing and Verification (Mandatory)

1. Tests should live close to the layer they verify (core/usecase unit tests, adapters integration tests).
2. Real hardware verification must stay in `projects/*/tests/hil` only.
3. Before claiming completion, run `scripts/validate-structure.ps1` and relevant impacted tests/commands.
4. Report verification result with command output, not assumptions.

## 中文快速规则（给 AI 与开发者）

1. 先读 `AGENTS.md` 再开始开发；目录细则见 `docs/runbooks/workspace-directory-rules.zh-CN.md`。
   执行标准见 `docs/runbooks/ai-agent-execution-standard.zh-CN.md`。
2. 未经明确要求，不新增/删除/重命名/移动顶层目录。
3. 新建项目必须使用 `scripts/new-project.ps1`，禁止手工随意搭目录。
4. 结构改动必须先给“变更清单 + 影响范围”，再更新 `workspace.structure.json`。
5. 完成中大型任务前，必须运行 `scripts/validate-structure.ps1` 并确保通过。
6. 设备原始资料放 `device-lab/drivers/`；可直接开发的命令规范放 `shared/device-sdk/docs/commands/`。
7. `core` 层禁止直接依赖硬件库；硬件依赖只允许在 `adapters/hardware` 或 `shared/device-sdk`。
8. 桌面应用统一放 `projects/*/apps/desktop-wails/`（Vue 3 + Go + Wails）。
