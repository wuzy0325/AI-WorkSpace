# Project Structure Variants

This workspace uses the same dependency boundaries across several project shapes. Do not judge compliance by directory names alone; judge by dependency direction and documented responsibilities.

## 1. Split Service Wails Project

Use for larger products with an HTTP-capable Go backend and a Wails desktop shell.

Examples:

- `projects/wind-daq`
- `projects/motion-controller`

Shape:

```text
projects/<name>/
  apps/desktop-wails/
    backend/          # Wails binding layer only
    frontend/         # Vue 3 frontend
  services/api-go/
    cmd/              # server entrypoints
    api/              # thin HTTP routing
    internal/
      core/           # pure domain logic
      ports/          # interfaces only
      usecase/        # orchestration
      adapters/       # hardware, config, storage, etc.
```

Rules:

- `apps/desktop-wails/backend` must delegate to usecases or shared application services.
- `api/` decodes requests, delegates, and encodes responses. It must not contain domain logic.
- Project `internal/*` packages are private to that project.

## 2. Single-Module Wails Project

Use for small standalone apps where a separate `services/api-go` module would add noise.

Example:

- `projects/daq-t1603`

Shape:

```text
projects/<name>/apps/desktop-wails/
  core/               # pure domain types and rules
  ports/              # interfaces only
  usecase/            # orchestration
  adapters/           # hardware/config/recording implementations
  backend/            # Wails binding layer only
  frontend/           # Vue 3 frontend
```

Rules:

- The same hexagonal constraints still apply.
- `backend/` must remain a thin parameter conversion and usecase delegation layer.
- `frontend/src/bridge` should be the only layer that imports generated `wailsjs/` bindings when the project uses bridge isolation.

## 3. Shared Go Module

Use for reusable code required by two or more projects.

Examples:

- `shared/device-sdk/go`
- `shared/motion-control/go`
- `shared/algorithms/go/fivehole`

Rules:

- Shared modules must not import `projects/*/internal/*` packages.
- `shared/device-sdk/go` owns low-level protocol, transport, hardware adapter, serial, and FFI code.
- `shared/motion-control/go` owns reusable application-level motion manager, profile persistence, HTTP route glue, and status polling helpers.
- `shared/algorithms/*` must stay independent from hardware drivers.

## 4. Standalone Tool Or Delivery Project

Use for focused tools, generated release artifacts, or legacy delivery packages.

Examples:

- `projects/three-hole-interpolator/apps/desktop-win7`
- `programs/*`

Rules:

- Tools may depend on `shared/*`.
- Tools must not import product `internal/*` packages.
- Binary release artifacts and zip packages should be documented by the owning project README or moved to an explicit release directory.
