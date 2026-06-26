# Three-Hole Interpolator

Standalone Wails desktop app for three-hole probe PRB interpolation.

Extracted from `ThreeHoleProbeApp` (C# WPF) and ported to Go + Vue 3.

## Scope

This project owns the user-facing desktop tool: file selection, CSV import,
batch calculation, help documents, and packaging. The reusable interpolation
algorithm lives in `shared/algorithms/go/threehole`.

## Commands

```powershell
cd projects/three-hole-interpolator/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

```powershell
cd projects/three-hole-interpolator/apps/desktop-wails
go test ./...
```

```powershell
cd shared/algorithms/go/threehole
go test ./...
```

```powershell
cd projects/three-hole-interpolator/apps/desktop-wails/frontend
npm run build
```

## Boundaries

- Keep Wails bindings and file dialogs in `apps/desktop-wails/backend`.
- Keep Vue UI in `apps/desktop-wails/frontend`.
- Keep interpolation algorithms in `shared/algorithms/go/threehole`.
- Do not depend on `projects/wind-daq` or `projects/five-hole-interpolator` internal packages.
