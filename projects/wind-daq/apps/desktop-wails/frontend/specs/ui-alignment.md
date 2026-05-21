# Spec: Wind-DAQ UI Alignment

## Objective
Align wind-daq (Wails/Vue 3) frontend UI to match the reference Cursor DAQ (Electron/Vue 3) in visual consistency, component completeness, and code quality.

## Success Criteria
- [ ] Toast notifications render correctly (replacing missing visual feedback)
- [ ] Confirm dialogs render correctly (replacing broken confirmation flow)
- [ ] Layout token values match actual component dimensions
- [ ] All views consistently use UiButton/UiSelect base components
- [ ] Dead calibration code removed (clean build)
- [ ] Dead MainView.vue removed (clean build)

## Commands
```powershell
Build: npm run build
Test: npm test
Typecheck: npx vue-tsc --noEmit
```

## Project Structure
```
src/
  components/
    feedback/     → UiToastHost.vue (NEW), UiConfirmDialog.vue (NEW)
    ui/           → existing UI primitives
    calibration/  → REMOVE (dead code)
  views/
    MainView.vue  → REMOVE (dead code)
    main/MainLayout.vue → ADD Toast + ConfirmDialog
```

## Boundaries
- Always: Port components from reference project with path alias adjustment
- Ask first: Any structural changes beyond this scope
- Never: Change design token values (colors, spacing, radius)
