# Five-Hole Interpolator

Standalone Wails desktop app for five-hole probe PRB interpolation.

## Scope

This project owns the user-facing desktop tool: file selection, CSV import,
batch calculation, help documents, and packaging. Reusable interpolation
algorithms live in `shared/algorithms/go/fivehole` and are shared with
`projects/wind-daq`.

## Commands

```powershell
cd projects/five-hole-interpolator/apps/desktop-wails
wails dev
```

```powershell
cd projects/five-hole-interpolator/apps/desktop-wails
go test ./...
```

```powershell
cd projects/five-hole-interpolator/apps/desktop-wails/frontend
npm run build
```

## Boundaries

- Keep Wails bindings and file dialogs in `apps/desktop-wails/backend`.
- Keep Vue UI in `apps/desktop-wails/frontend`.
- Keep interpolation algorithms in `shared/algorithms/go/fivehole`.
- Do not depend on `projects/wind-daq/services/api-go`.
