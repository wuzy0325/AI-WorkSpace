# AI Workspace

This workspace hosts industrial DAQ, motion-control, and probe-interpolation desktop applications built with Go, Vue 3, and Wails. Product projects reuse shared algorithms, hardware SDKs, and application-level motion-control modules.

## Top-Level Layout

- `projects/`: product projects and standalone desktop apps.
- `shared/`: reusable algorithms, device SDKs, motion-control orchestration, contracts, and frontend modules.
- `programs/`: standalone utilities and small tools. Programs may depend on `shared/*`, never project `internal/*` packages.
- `device-lab/`: hardware rigs, captures, firmware, raw vendor docs, and lab tooling.
- `docs/`: architecture notes, decisions, plans, and runbooks.
- `scripts/`: workspace automation scripts.
- `tools/`: container and environment helpers.

## Active Projects

| Project | Purpose | Main docs |
|---|---|---|
| `projects/windlabx4` | Wind tunnel DAQ desktop application with device management, calibration, traversal, storage, reports, and integrated motion control. | `projects/windlabx4/README.md`, `projects/windlabx4/docs/STRUCTURE.md` |
| `projects/wista` | Standalone DAQ-T-1603 desktop app using a single Wails Go module. | `projects/wista/README.md`, `projects/wista/CLAUDE.md` |
| `projects/motion-controller` | Standalone motion-controller desktop app. | `projects/motion-controller/README.md`, `projects/motion-controller/SPEC.md` |
| `projects/five-hole-interpolator` | Standalone five-hole probe interpolation tool. | `projects/five-hole-interpolator/README.md`, `projects/five-hole-interpolator/SPEC.md` |
| `projects/three-hole-interpolator` | Standalone three-hole probe interpolation and Win7 delivery tooling. | `projects/three-hole-interpolator/README.md`, `projects/three-hole-interpolator/SPEC.md` |

## Shared Modules

| Module | Purpose |
|---|---|
| `shared/algorithms/go/fivehole` | Canonical five-hole interpolation algorithms. |
| `shared/device-sdk/go` | Low-level DAQ, motion, protocol, serial, and FFI device SDK code. |
| `shared/motion-control/go` | Reusable application-level motion manager, profile persistence, HTTP routes, and status polling helpers. |
| `shared/frontend` | Workspace-level frontend sharing area. Project-local frontend sharing under `projects/*/shared/frontend` is temporary unless documented. |

## Important Rules

1. Do not change workspace directory structure directly.
2. Use `scripts/new-project.ps1` for standard new project skeletons.
3. Run `scripts/validate-structure.ps1` before and after major development tasks.
4. If structure must change, update `workspace.structure.json` and document the reason in `docs/decisions/`.
5. Reusable code used by two or more projects belongs under `shared/`, not under a product project.

## Quick Commands

```powershell
powershell -File .\scripts\validate-structure.ps1
powershell -File .\scripts\new-project.ps1 -Name project-gamma
powershell -File .\scripts\lint-go.ps1
```

## Documentation Entry Points

- Workspace architecture and hard constraints: `CLAUDE.md`
- Full documentation index: `docs/index.md`
- Project structure variants: `docs/architecture/project-variants.md`
- Directory rules: `docs/runbooks/workspace-directory-rules.zh-CN.md`
- AI agent execution standard: `docs/runbooks/ai-agent-execution-standard.zh-CN.md`
