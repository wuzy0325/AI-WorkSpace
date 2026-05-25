# Wind-DAQ Migration Documentation

This is the single entry point for the `wind-daq` refactor/migration work.

## Product Positioning

`wind-daq` is the refactored successor of the pre-refactor `Cursor DAQ` project:

- The user-facing UI and operator workflow should stay consistent with `Cursor DAQ`.
- The backend is rebuilt in Go/Wails with clearer architecture and stronger boundaries.
- Vue owns display, interaction, and state presentation only.
- Go backend owns device control, acquisition, calibration, traversal, storage, reports, and business workflows.
- Do not copy old Electron IPC coupling or frontend business logic into the new project.

## Reference Source

Use this project as the visual and feature reference:

```text
C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ
```

Use this frontend subtree for UI parity:

```text
C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src
```

## Current Authoritative Documents

Read these in order before implementing migrated functionality:

1. `projects/wind-daq/README.md`
2. `projects/wind-daq/DESIGN.md`
3. `projects/wind-daq/docs/migration/README.md` (this file)
4. `projects/wind-daq/docs/migration/ui-parity-plan.md`
5. `projects/wind-daq/docs/migration/ui-parity-audit.md`
6. `projects/wind-daq/docs/migration/ts-reference-feature-map.md`
7. Relevant runbook under `projects/wind-daq/docs/runbooks/`

## Document Responsibilities

| Document | Responsibility |
|---|---|
| `ui-parity-plan.md` | AI implementation guide for keeping the frontend visually consistent with Cursor DAQ while preserving the new architecture. |
| `ui-parity-audit.md` | Current truthful UI parity audit. Use this as the task list for UI completion. It records mismatches, target files, and acceptance checks. |
| `ts-reference-feature-map.md` | Feature-level migration status from Cursor DAQ to wind-daq. Keep this aligned with current code and verification evidence. |
| `../runbooks/integrated-smoke-checklist.md` | Manual/automated smoke validation checklist. |
| `../runbooks/hil-validation-plan.md` | Real hardware validation plan and HIL evidence. |
| `../../../../docs/plans/go-backend-migration.md` | Historical completed Go backend migration record. Not a current execution plan. |

## Rules For Future AI Agents

- Treat this file as the migration documentation entry point.
- Do not create new migration plans in `docs/plans/` for `wind-daq` unless the user explicitly asks for a workspace-level plan.
- Do not create duplicate UI parity specs under `apps/desktop-wails/frontend/specs/`.
- If a document becomes stale, update this entry point and either update or delete the stale document in the same change.
- If the UI differs from Cursor DAQ, fix the implementation or record the deliberate exception in `ui-parity-plan.md`.
- If a visible old UI control has a current Go backend capability, wire it to the backend instead of removing it.
- If a visible old UI control has no backend capability yet, keep it disabled or document the gap. Do not silently drop it.
- Do not treat `Done` in older documents as proof of visual parity. Check `ui-parity-audit.md` and the running app.
- A component existing in the target project does not mean the feature is complete. Completion requires visual parity, interaction parity, and verified backend wiring where applicable.

## Deleted Stale Documents

The following stale or duplicate documents were removed during documentation convergence:

- `docs/plans/2026-05-20-wind-daq-ts-migration.md`
- `projects/wind-daq/apps/desktop-wails/frontend/specs/ui-alignment.md`
- `projects/wind-daq/apps/desktop-wails/frontend/specs/cursor-daq-ui-parity-ai-plan.md` (moved to `ui-parity-plan.md`)
- `projects/wind-daq/docs/migration/migration-execution-plan.zh-CN.md`
- `projects/wind-daq/docs/migration/migration-improvement-backlog.zh-CN.md`
