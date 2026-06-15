# Spec: Wind-DAQ P0 Architecture Violation Refactoring

## Objective

Remove hard-constraint violations in 4 frontend files that leak business logic, hardware access, and backend orchestration into the Vue/TypeScript layer. After refactoring, the frontend will be a pure UI orchestration layer — display, interact, call API — with zero calibration algorithms, zero device data processing, and zero backend business rule duplication.

**Success criteria:**
1. `useSphereTankGate.ts` no longer subscribes to raw device data streams or processes channel values
2. `useCalibrationWorkflow.ts` no longer uses regex channel-name matching or duplicates precondition checks
3. `deviceStore.ts` no longer performs tare-offset subtraction or zero-point calibration
4. `traversalStore.ts` no longer makes orchestration decisions about interpolation timing
5. All 4 files pass `npm run typecheck && npm run build`
6. Existing operator workflow is preserved (no regression in calibration/traversal startup flow)

## Tech Stack

- **Frontend:** Vue 3 + TypeScript + Pinia + Naive UI
- **Backend:** Go 1.22+ with hexagonal architecture (core → ports → usecase → api)
- **Communication:** HTTP REST (127.0.0.1:8900) + SSE for real-time data
- **Reference:** Cursor DAQ project at `C:\Users\wuzhy\...\Cursor DAQ\src\renderer\src`

## Commands

```powershell
# Frontend verification
cd projects\wind-daq\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test

# Backend verification (if backend changes are needed)
cd projects\wind-daq\services\api-go
go build -buildvcs=false ./...
go vet ./...
go test ./internal/... ./api/...
```

## Project Structure (affected files only)

```
frontend/src/
├── api/
│   ├── calibrationApi.ts       ← may need new endpoint wrappers
│   ├── deviceApi.ts            ← may need new endpoint wrappers
│   └── traversalApi.ts         ← may need new endpoint wrappers
├── composables/
│   ├── useCalibrationWorkflow.ts  ← REFACTOR (remove channel regex + precondition duplication)
│   └── useSphereTankGate.ts       ← REFACTOR (remove raw data subscription)
├── stores/
│   ├── calibrationStore.ts     ← may need minor updates
│   ├── deviceStore.ts          ← REFACTOR (remove tare logic from frontend)
│   └── traversalStore.ts       ← REFACTOR (simplify interpolation coordination)
└── shared/
    ├── calibrationPrecision.ts ← minor cleanup
    └── calibrationDataGuards.ts ← no change
```

## Code Style

TypeScript conventions:

```typescript
// Stores: thin API passthroughs, no data transformation
async function startCalibration(config: CalibrationConfig): Promise<void> {
  this.status = 'running'
  try {
    await calibrationApi.start(config)
  } catch (e) {
    this.status = 'error'
    throw e
  }
}

// Composables: orchestration only, no business logic derivation
const canStart = computed(() =>
  deviceStore.isAnyDeviceConnected && motionStore.isConnected
)
```

## Testing Strategy

- **Unit tests:** Existing store tests must continue passing
- **No new tests required** for this refactor (behavior-preserving)
- **Verification:** `npm run typecheck && npm run build && npm run test`

## Boundaries

- **Always:**
  - Run `npm run typecheck && npm run build` before marking any file complete
  - Preserve the exact same API call signatures and return types
  - Keep the same UI behavior — the user should not notice any change
- **Ask first:**
  - Adding new backend endpoints (prefer using existing ones)
  - Changing TypeScript interfaces/types that cross API boundaries
  - Restructuring the composable/store public API
- **Never:**
  - Remove or rename Pinia store actions/properties that components consume
  - Change the calibration or traversal workflow steps
  - Introduce new business rules into the frontend

## Success Criteria

| File | Before | After |
|------|--------|-------|
| `useSphereTankGate.ts` | Subscribes to `deviceApi.onSnapshot`, processes raw channel values, calls `updateSphereTankGate` stub | Uses backend SSE `/api/daq/stream/{id}` or periodic `/api/daq/latest/{id}` for display-only stability timing; save is a pure config write via the existing config API |
| `useCalibrationWorkflow.ts` | Regex channel name matching (`/总压/`, `/静压/`, `/风洞.*温/`), hardcoded `hasRequiredWindTunnelChannels`, full `canStartCalibration` precondition check | Channel type identification delegated to backend (via a returned field in config); preconditions simplified to reactive checks on `deviceStore`/`motionStore` connected flags |
| `deviceStore.ts` | `getDisplayValue(rawValue - offset)` performs tare subtraction, `tareAllEnabled` reads raw values to set tare | Display value fetched from backend (which already returns tared values or the offset is applied display-side via a display transform function), tare initiated via existing `/api/device/{id}/tare` endpoint |
| `traversalStore.ts` | `syncRealtimeInterpolation` manages request timing, deduplication, PRB presence checks | Store fires `calculateRealtime` on demand (when user action requires it); backend handles timing/caching; frontend only manages UI refresh throttling (already in `useUiRefreshThrottle`) |

## Plan

### Implementation Order

```
Phase 2.1: deviceStore.ts + FiveHoleMain.vue
  └── Remove tare logic from frontend data pipeline
      ├── getDisplayValue: keep for display formatting, rename to clarify
      ├── FiveHoleMain: send raw values to calibration backend
      └── Panels: keep using display value for threshold checks

Phase 2.2: useCalibrationWorkflow.ts  (parallel with 2.3, 2.4)
  └── Switch channel ID to ProbeChannel.Role
      ├── hasConfiguredProbeChannel → use role enum, not regex
      ├── canStartCalibration → simplify to deviceStore + motionStore connected flags
      └── Fall back to name regex only if Role is unset

Phase 2.3: useSphereTankGate.ts  (parallel with 2.2, 2.4)
  └── Remove architecture violations
      ├── Keep SSE subscription (correct pattern)
      ├── Remove updateSphereTankGate() call (no-op stub)
      ├── Keep stability timing display logic (UI concern)
      └── Config save uses existing config API path

Phase 2.4: traversalStore.ts  (parallel with 2.2, 2.3)
  └── Simplify interpolation coordination
      ├── Remove syncRealtimeInterpolation auto-orchestration
      ├── Components use useUiRefreshThrottle for debounce
      └── Store only provides fire-and-forget calculateRealtime
```

### Dependency Graph

```
deviceStore fix
  └── FiveHoleMain update (sequential: the consumer depends on the API change)
  
useCalibrationWorkflow  ← independent, parallel
useSphereTankGate       ← independent, parallel  
traversalStore          ← independent, parallel
```

### Verification Checkpoints

1. After Phase 2.1: `npm run typecheck && npm run build`
2. After Phase 2.2+2.3+2.4: `npm run typecheck && npm run build && npm run test`
3. Final: manual smoke test of calibration start flow + traversal start flow

### Risks

| Risk | Mitigation |
|------|-----------|
| FiveHoleMain sends raw values but backend expects tared | Backend stores whatever it receives — no expected breakage. Tare will need future backend alignment. |
| Some component depends on getDisplayValue being a raw subtraction | grep shows only 4 consumers, all accounted for in this plan |
| Role field in ProbeChannel is not populated in saved configs | Keep regex fallback when Role is unset/unknown (backward compatible) |
| traversalStore consumers depend on syncRealtimeInterpolation return value | Change signature to simple void, update consumers to handle their own throttling |

## Reference Alignment

Reference project (Cursor DAQ) handles these concerns as follows:
- **Sphere tank gate:** Also frontend-side, but uses IPC event stream (`daq:realtime`) from the main process — NOT raw device driver data. Our fix: use the same SSE-based approach from the backend.
- **Channel identification:** Reference project stores channel type in device profile/channel config — no regex matching needed. Our fix: ensure the backend `/api/calibration/precisionDefaults` or config endpoint returns channel type metadata.
- **Tare:** Reference project also applies tare offset in the renderer store. However, the backend already has `/api/device/{id}/tare` — so we can push this to backend.
- **Interpolation coordination:** Reference project's traversal store is also complex (700+ lines) with similar throttling. However, the backend already handles timing — we can simplify.

## Tasks

### Task A: deviceStore — clarify display tare and decouple from calibration data pipeline

- **Task A1: Rename `getDisplayValue` → `applyDisplayTare` in deviceStore**
  - Acceptance: Same function, renamed to signal display-only purpose
  - Verify: `npm run typecheck`
  - Files: `stores/deviceStore.ts`

- **Task A2: Update DeviceDetailPanel + DeviceOverviewPanel to use new name**
  - Acceptance: Panel threshold checks compile and work
  - Verify: `npm run typecheck && npm run build`
  - Files: `components/main/DeviceDetailPanel.vue`, `components/main/DeviceOverviewPanel.vue`

- **Task A3: Update FiveHoleMain to send raw (un-tared) values**
  - Acceptance: FiveHole calibration collects raw channel values, not tare-adjusted
  - Verify: `npm run typecheck && npm run build`
  - Files: `components/calibration/five-hole/FiveHoleMain.vue`

### Task B: useCalibrationWorkflow — switch to ProbeChannel.Role field

- **Task B1: Replace regex channel matching with role-based lookup**
  - Acceptance: `hasConfiguredProbeChannel` checks `ProbeChannel.Role` instead of regex on `Name`
  - Verify: `npm run typecheck && npm run build`
  - Files: `composables/useCalibrationWorkflow.ts`

- **Task B2: Simplify `canStartCalibration` precondition**
  - Acceptance: Precondition reduces to `deviceStore.isAnyDeviceConnected && motionStore.isConnected`
  - Verify: `npm run typecheck && npm run build`
  - Files: `composables/useCalibrationWorkflow.ts`

### Task C: useSphereTankGate — remove violations, keep display logic

- **Task C1: Remove `updateSphereTankGate()` call**
  - Acceptance: No frontend code calls the no-op stub
  - Verify: `npm run typecheck`
  - Files: `composables/useSphereTankGate.ts`

- **Task C2: Verify SSE pattern and config save path**
  - Acceptance: Subscription uses `deviceApi.onSnapshot` (SSE), config save uses `calibrationApi.saveConfig()`
  - Verify: `npm run typecheck && npm run build`
  - Files: `composables/useSphereTankGate.ts`

### Task D: traversalStore — simplify interpolation coordination

- **Task D1: Strip `syncRealtimeInterpolation` orchestration**
  - Acceptance: Method becomes simple passthrough; throttling uses `useUiRefreshThrottle`
  - Verify: `npm run typecheck && npm run build && npm run test`
  - Files: `stores/traversalStore.ts`

## Decisions (verified from code)

1. **`useSphereTankGate.ts`**: SSE stream (`/api/daq/stream/{id}`) already includes `DataPayload.channels[]` + `channelIndices[]` — same data the composable currently receives via `deviceApi.onSnapshot`. The subscription pattern is correct (SSE, not raw hardware). **Fix**: Remove the no-op `updateSphereTankGate()` call; keep SSE subscription for display-only stability timing; save config via existing config API.

2. **`deviceStore.ts`**: `getDisplayValue` has 4 consumers — `formatValue()` (display), `DeviceDetailPanel` (threshold check), `DeviceOverviewPanel` (threshold check), and **`FiveHoleMain.vue`** (calibration input data). The last one is problematic: tared values are used as calibration data sent to backend, but backend has no record of the tare. **Fix**: Keep `formatValue` for display; change FiveHoleMain to send raw values to backend; the threshold checks in panels work fine with or without tare — keep using display value for visual consistency. No backend change needed for this phase.

3. **`useCalibrationWorkflow.ts`**: Backend `ProbeChannel` already has a `Role` field (part of calibration config). The regex matching was written before Role was consistently populated. **Fix**: Switch `hasConfiguredProbeChannel` to use `ProbeChannel.Role` instead of regex name matching; only fall back to name matching if Role is unset/unknown.
