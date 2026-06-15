# Spec: Wind-DAQ P2 State Pattern Completion

## Objective

Add loading, empty, error, offline, and saving states to all views using existing `UiLoadingState`/`UiEmptyState`/`UiErrorState`/`UiStatusBadge` components. Currently 0 of 8 views use these shared state components.

**Success criteria:**
1. CalibrationView shows loading spinner during async setup
2. MainDashboardView shows loading state during startup and error state on SSE failure
3. StorageView, LogViewer, TraversalView, MotionView handle empty/error states via shared components
4. All views pass `npm run typecheck && npm run build`

## Commands

```powershell
cd projects\wind-daq\apps\desktop-wails\frontend
npm run typecheck && npm run build && npm run test
```

## Tasks

### Task A: CalibrationView + MotionView (minimal shell wrappers)
- Add `UiLoadingState` for async child component loading
- Add `UiErrorState` catch for failed async load
- Files: `views/CalibrationView.vue`, `views/MotionView.vue`

### Task B: MainDashboardView
- Add `UiLoadingState` during initial device profile loading
- Add `UiErrorState` displayed on SSE error (currently only console.error)
- Files: `views/main/MainDashboardView.vue`

### Task C: StorageView + LogViewer
- Add `UiEmptyState` for empty data
- Keep existing `UiErrorState` usage in StorageView
- Files: `views/StorageView.vue`, `views/LogViewer.vue`

### Task D: TraversalView
- Proper loading state during recovery (currently passes bare `isRecovering` boolean)
- Files: `views/TraversalView.vue`
