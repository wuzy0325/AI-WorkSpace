# shared.local/device-sdk/go

Reusable low-level device SDK module.

This module owns protocol, transport, hardware adapter, simulator, and FFI code that can be reused by product projects and standalone tools.

## Package Layout

| Package | Responsibility |
|---|---|
| `daq` | DAQ device domain types, ports, and hardware adapters such as DAQ-T-1603. |
| `motion` | Motion device domain types, ports, and low-level controller adapters such as simulated and B140 controllers. |
| `protocol` | Binary/text frame parsing and encoding helpers. |
| `serialport` | Serial-port abstraction and implementation helpers. |
| `ffi` | Native-library wrappers and stubs, including WTNMC4A integration points. |

## Boundaries

Allowed:

- Low-level device communication and protocol translation.
- Hardware adapter implementations.
- Device simulators that implement SDK ports.

Forbidden:

- Product workflows such as calibration, traversal, recording, or UI state.
- HTTP route registration or Wails bindings.
- Imports from `projects/*/internal/*`.
- Application-level motion manager behavior; that belongs in `shared/motion-control/go`.

## Verification

```powershell
cd shared/device-sdk/go
go test ./...
go build ./...
```
