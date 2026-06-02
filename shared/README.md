# shared

Shared workspace libraries. Code here may be used by multiple product projects and standalone tools.

## Modules

| Path | Purpose |
|---|---|
| `algorithms/` | Reusable algorithm code. Algorithm packages must not depend on hardware drivers. |
| `device-sdk/` | Low-level hardware communication, protocol parsing, device adapters, serial transport, simulators, and FFI wrappers. |
| `motion-control/` | Application-level motion-control orchestration above `device-sdk`: manager, profile persistence, HTTP routes, and status polling helpers. |
| `contracts/` | Shared API and protocol contracts. |
| `frontend/` | Workspace-level frontend modules and UI assets. |

## Dependency Rules

- Shared modules must not import `projects/*/internal/*` packages.
- Product-specific workflows stay in `projects/<name>/...`.
- Low-level device protocol code belongs in `shared/device-sdk/go`.
- Reusable motion-control application behavior belongs in `shared/motion-control/go`.
- Reusable algorithms belong in `shared/algorithms/*` and should stay hardware-independent.
