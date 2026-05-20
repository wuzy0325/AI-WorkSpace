# Wind-DAQ Agent Rules

Single source of truth: `../../AGENTS.md`.

## Project Commands

```powershell
# Backend, after go.mod exists
cd projects\wind-daq\services\api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...
```

## Project Addendum

- Wails backend code in `apps/desktop-wails/backend` must stay thin: parameter conversion and usecase calls only.
- Vue frontend code in `apps/desktop-wails/frontend` must not access hardware or contain calibration/traversal algorithms.
- New migrated behavior should be added test-first unless it is documentation-only or generated code.
