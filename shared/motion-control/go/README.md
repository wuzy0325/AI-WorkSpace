# shared.local/motion-control/go

Reusable application-level motion-control module.

This module sits above `shared.local/device-sdk/go`. It owns behavior that is common to motion-capable products but is not low-level device protocol code.

## Package Layout

| Package | Responsibility |
|---|---|
| `manager` | Motion manager orchestration: profiles, controller lifecycle, axis commands, emergency stop, status queries. |
| `profile` | Profile store abstraction and file-backed profile persistence. |
| `httpapi` | Thin reusable `/api/motion/*` route registration. |
| `events` | Status polling helper with callback-based event emission. |

## Boundaries

Allowed:

- Depend on `shared.local/device-sdk/go`.
- Provide reusable application-level motion behavior for `wind-daq` and `motion-controller`.
- Decode HTTP requests and delegate to the manager in `httpapi`.

Forbidden:

- Import any `projects/*/internal/*` package.
- Import Wails runtime directly.
- Own DAQ, calibration, traversal, storage, or reporting workflows.
- Implement low-level serial/protocol details that belong in `shared/device-sdk/go`.

## Verification

```powershell
cd shared/motion-control/go
go test ./...
go build ./...
```
