# Win7 Baseline Inventory

## Baseline Identity

- Source branch before promotion: `feature/daq-t1603-win7`
- Historical HEAD before Win7 commit: `b04103361e2c28360033886d7b6d4a60e24f1e27`
- Verified product: `daq-t1603 0.3.3`
- Target: Windows 7 SP1 x64
- Toolchain: Go 1.20.14 + Electron 22.3.27 + Chromium 108
- Original Win7-tested artifact SHA256: `9739A41EFFEF26E7187275CEC2219EBB3DC8BA6669E94448938050C1A169A52F`
- Rebuilt baseline artifact SHA256: `3C74C055237D3585942D707A14A0CEA549EB22308D14F0356A64E9A28E8BED7E`

## Included Source Groups

| Group | Paths | Purpose |
|---|---|---|
| Go 1.20 compatibility | `shared/device-sdk/go/go.mod`, `go.sum`, `pkg/slog/`, affected SDK imports | Remove Go 1.21+ standard-library requirements |
| DAQ-T1603 backend | `projects/daq-t1603/apps/desktop-wails/main.go`, `core/`, `backend/`, `httpserver/` | Replace Wails runtime with loopback HTTP and WebSocket events |
| Frontend transport | `frontend/src/bridge/`, `frontend/src/env.d.ts`, frontend package files | Replace Wails bindings/events with HTTP, WebSocket and Electron dialog bridge |
| Electron shell | `projects/daq-t1603/apps/desktop-electron/` excluding ignored artifacts | Start backend, load local UI, expose dialog IPC and build NSIS x64 |
| Reproducible toolchain | `scripts/install-go1.20.14.ps1` | Install the final Go release supporting Windows 7 |
| Governance | `WIN7-PROGRESS.md`, `WIN7-SYNC-STATE.md`, this file | Record verification, synchronization and baseline provenance |

## Excluded Generated Artifacts

The following paths are intentionally excluded by `projects/daq-t1603/apps/desktop-electron/.gitignore`:

- `node_modules/`
- `backend/`
- `dist/`

Installers and backend binaries are rebuilt from committed source. Release records store their SHA256 separately.

## Unrelated Changes

No `wind-daq` or `motion-controller` source changes were present in this worktree baseline. All 36 Git status entries were attributable to the groups above.

## Verification Required Before Tagging

1. Go 1.20.14: `go test ./...`, `go vet ./...`, `go build -buildvcs=false ./...`.
2. Frontend: `npm run typecheck`, `npm run test`, `npm run build`.
3. Electron: `node --check main.cjs`, `node --check preload.cjs`, `npm run dist:win7`.
4. Packaged application: `/api/health`, index and JavaScript asset smoke test.
5. NSIS installation: silent install and installed executable health check.
