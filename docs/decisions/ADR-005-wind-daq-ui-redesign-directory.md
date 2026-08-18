# ADR-005: Register windlabx4-ui-redesign Top-Level Directory

## Status

Accepted (2026-07-01)

## Context

A `windlabx4-ui-redesign/` directory was added at the workspace root to hold an
in-progress UI redesign exploration: static HTML page mockups
(`pages/dashboard.html`, `pages/calibration.html`, `pages/traversal.html`),
a partial project shell, a `colors_and_type.css` design tokens file, an
orchestration summary, and a `.design` artifact.

`scripts/validate-structure.ps1` enforces `allowUnknownTopLevelEntries: false`
in `workspace.structure.json`, so the unregistered directory caused the
structure validation to fail — blocking the "run validation before completing
non-trivial work" gate from `CLAUDE.md`.

The directory is deliberately **not** under `projects/windlabx4/`: it is a
throwaway design exploration (static HTML/CSS mockups), not a buildable
project, and keeping it out of the project tree avoids polluting the Wails/Vue
build and the hexagonal Go module layout.

## Decision

Register `windlabx4-ui-redesign` as an allowed top-level entry in
`workspace.structure.json`, treating it as a design-exploration workspace
alongside `device-lab/` (raw artifacts that are reference material, not source
code).

The directory holds only static, non-built artifacts — no `go.mod`, no
`package.json`, no build tooling. It is not wired into any Go workspace
(`go.work`) or frontend build.

## Consequences

- `validate-structure.ps1` passes again.
- The exploration lives outside the project tree, so it cannot accidentally
  leak into production builds or import graphs.
- If the redesign is adopted, the relevant tokens/components should be ported
  into `projects/windlabx4/apps/desktop-wails/frontend/` and
  `shared/frontend/`, after which this directory can be deleted and the entry
  removed from `workspace.structure.json`.
- It is **not** a required directory — only an allowed one — so removing it
  later requires no `requiredDirectories` cleanup.
