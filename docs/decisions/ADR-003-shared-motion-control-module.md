# ADR-003: Shared Motion Control Module

## Status

Implemented

## Context

`motion-controller` and `wind-daq` both need motion-control capability.

Today they already share low-level hardware protocol code through
`shared/device-sdk/go`, but they still duplicate the application-level motion
control layer:

- motion profile management
- controller connect and disconnect operations
- axis commands such as home, move to, move by, jog, stop, and define position
- emergency stop handling
- `/api/motion/*` HTTP routes
- Wails-facing `Motion*` methods
- periodic `motion:status` event publishing

This duplication is not ideal because fixes to motion command behavior,
validation, profile handling, or route semantics must be repeated in multiple
projects.

At the same time, `wind-daq` must not import `motion-controller` project code.
`motion-controller` is a product, not a library. Cross-project reusable logic
must live under `shared/`.

## Decision

Extract reusable application-level motion control code into a new shared Go
module:

```text
shared/motion-control/go
```

The module currently exists with the Go module path `shared.local/motion-control/go`.

The module owns reusable motion-control orchestration and API glue that is above
raw device protocol code but below project-specific product workflows.

Recommended package layout:

```text
shared/motion-control/go/
  go.mod
  manager/
    motion_manager.go
  httpapi/
    routes.go
  events/
    status_poller.go
  profile/
    store.go
    json_store.go
```

The intended dependency direction is:

```text
projects/motion-controller/*
  -> shared/motion-control/go
  -> shared/device-sdk/go

projects/wind-daq/*
  -> shared/motion-control/go
  -> shared/device-sdk/go
  -> shared/algorithms/go/fivehole
```

`shared/device-sdk/go` remains responsible for low-level device protocol,
transport, hardware adapters, and device-domain types. It must not become a
home for HTTP routes, Wails bindings, or product workflow orchestration.

`shared/motion-control/go` is responsible for reusable application-level motion
capability:

- `MotionManager`
- profile store abstraction and JSON profile persistence
- reusable `/api/motion/*` route registration
- reusable status polling helper with callback-based event emission

Project-specific Wails methods may initially remain in each app as thin
parameter-conversion wrappers. They can call the shared `MotionManager` without
being extracted immediately.

## Workspace Rule Compliance

This plan is compliant only if the implementation includes the required
workspace structure updates.

Relevant workspace rules from `CLAUDE.md`:

- Code reusable by 2+ projects belongs in `shared/`.
- Project `internal/*` packages must not be imported by other projects.
- Wails backend code must remain thin and contain no domain business logic.
- Structural changes require updating `workspace.structure.json` and documenting
  the decision in `docs/decisions/`.

Compliance matrix:

| Rule | Plan compliance |
|---|---|
| Cross-project reusable code goes under `shared/` | Yes. The shared motion layer is placed under `shared/motion-control/go`. |
| Projects must not import another project's `internal/*` | Yes. `wind-daq` will import `shared/motion-control/go`, not `motion-controller/services/api-go/internal/*`. |
| `shared/device-sdk/go` remains low-level device SDK | Yes. Application-level manager and routes are not added to `device-sdk`. |
| Wails backend remains thin | Yes. Wails methods stay as parameter conversion and calls into the shared manager. |
| Structural changes are documented | Yes. This ADR documents the new shared module. |
| Structural validation remains accurate | Must update `workspace.structure.json` before adding the new directory in implementation. |

Required structure updates before implementation:

```text
workspace.structure.json
  requiredDirectories += shared/motion-control
  requiredDirectories += shared/motion-control/go
  requiredFiles += shared/motion-control/go/go.mod
```

The implementation must run:

```powershell
powershell -File .\scripts\validate-structure.ps1
```

## Migration Plan

Current implementation status: the shared module scaffold and core packages exist. Projects should continue replacing duplicated project-local motion behavior with imports from `shared.local/motion-control/go` as follow-up refactors.

1. Create the shared module scaffold.

   Validation:

   ```powershell
   powershell -File .\scripts\validate-structure.ps1
   ```

2. Move reusable motion manager behavior from project-specific services into
   `shared/motion-control/go/manager`.

   Keep only project-specific wiring inside each project's `services/api-go`.

   Validation:

   ```powershell
   go test ./...
   ```

3. Move reusable motion profile persistence into
   `shared/motion-control/go/profile`.

   The shared profile store may use file I/O because it is an adapter-style
   package, not `core/` domain logic.

   Validation:

   ```powershell
   go test ./...
   ```

4. Extract `/api/motion/*` route registration into
   `shared/motion-control/go/httpapi`.

   The HTTP package must remain thin: decode request, validate edge input,
   delegate to `MotionManager`, encode response.

   Validation:

   ```powershell
   go test ./...
   ```

5. Replace `motion-controller` motion implementation with imports from
   `shared/motion-control/go`.

   The project service should only wire dependencies and expose product-specific
   app context.

   Validation:

   ```powershell
   go build -buildvcs=false ./...
   ```

6. Replace `wind-daq` duplicated motion implementation with the same shared
   module.

   Wind-DAQ keeps its DAQ, calibration, traversal, report, storage, and
   five-hole interpolation logic inside its own project modules.

   Validation:

   ```powershell
   go build -buildvcs=false ./...
   ```

7. Optionally extract status polling into `shared/motion-control/go/events`.

   The helper must not import Wails. It should accept an `emit` callback so each
   app decides how to publish events.

   Validation:

   ```powershell
   go test ./...
   ```

## Non-Goals

- Do not make `wind-daq` depend on `motion-controller`.
- Do not move DAQ, calibration, traversal, report, or storage logic into the
  shared motion module.
- Do not move Wails app lifecycle code into the shared module in the first
  migration.
- Do not put HTTP routing or Wails event code into `shared/device-sdk/go`.
- Do not create a generic shared application framework before duplicated behavior
  proves it is worth extracting.

## Consequences

- Motion behavior fixes benefit both products.
- `wind-daq` can keep integrated motion traversal without copying the motion
  stack.
- `motion-controller` remains the dedicated motion-control product, but not the
  owner of reusable shared motion code.
- The workspace gains a new shared module category, so structure validation and
  documentation must be updated together with implementation.
- The first implementation step is slightly heavier because it includes a
  workspace structure change, but the resulting dependency graph is cleaner than
  either project importing the other.
