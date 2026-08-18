# ADR-002: Five-Hole Interpolator as an Independent Project

## Status

Accepted

## Context

The five-hole interpolator was originally located at
`projects/windlabx4/apps/five-hole-interpolator`. That made it possible to build
a separate Wails executable, but its code ownership and Go imports still tied it
to the WindLabX4 project.

The app is intended to be distributed and maintained as a standalone user-facing
tool.

## Decision

Move the app shell to `projects/five-hole-interpolator/apps/desktop-wails` and
extract reusable interpolation code to `shared/algorithms/go/fivehole`.

WindLabX4 and Five-Hole Interpolator both depend on the shared algorithm module.
The standalone app must not import `projects/windlabx4/services/api-go`.

## Consequences

- Algorithm fixes benefit both products.
- The standalone app can be tested and packaged without WindLabX4.
- File dialogs, CSV import, help documents, and Wails bindings remain app-level
  concerns.
- Workspace structure validation must include the new project and shared module.
