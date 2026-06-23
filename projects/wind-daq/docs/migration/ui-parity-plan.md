# Cursor DAQ UI Parity AI Implementation Plan

> **DEPRECATED (2026-06):** UI parity with Cursor DAQ is no longer the design target. See `../../DESIGN.md` and `./README.md`. This document is retained for historical reference only; do not treat its visual or layout instructions as current.

## Purpose

This document is the implementation guide for AI agents rebuilding `Cursor DAQ` as `wind-daq`.

The work is not a redesign. `wind-daq` is the refactored successor of the pre-refactor `Cursor DAQ`: the backend moves to Go/Wails and the internal architecture must improve, while the user-facing frontend remains visually and interactively consistent with the old project.

## Product Positioning

`wind-daq` is a refactor of the reference project, not a different product.

- Keep the old UI and user workflow recognizable.
- Upgrade architecture, code boundaries, naming, type safety, and maintainability.
- Replace Electron/Node/backend coupling with Go backend usecases and Wails/API adapters.
- Keep Vue focused on display, interaction, and state presentation.
- Do not copy old frontend business logic when that logic belongs in Go.
- Do not remove old UI controls just because the first refactor pass has not wired them yet. If a backend capability exists, wire it. If it does not exist, keep the visual affordance disabled or document the gap.

## Source And Target

### Reference UI Source

Use this path as the only visual source of truth:

```text
C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src
```

### Target UI Project

Apply changes here:

```text
C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\wind-daq\apps\desktop-wails\frontend
```

### Do Not Use As Visual Reference

Do not use these as the UI design source:

- `projects/wind-daq/apps/desktop-wails/frontend/dist`
- Any new generated design ideas
- Generic dashboard/SCADA UI assumptions

## Definition Of Done

The implementation is done when a user familiar with `Cursor DAQ` sees `wind-daq` and recognizes the old UI structure and styling.

Required outcomes:

- Main shell matches old layout: top bar, left rail, optional dashboard sidebar, central canvas, bottom bar.
- Dashboard page matches old visual hierarchy and spacing.
- Motion, calibration, traversal, and log pages are embedded inside the old rounded translucent page container.
- Rail order matches old project: `dashboard`, `motion`, `calibration`, `traversal`, `log`.
- `storage` is not shown in the main rail unless the user explicitly requests it.
- Dark theme matches old industrial SCADA look.
- Existing Wails data/API wiring still builds and works.
- Old visible controls remain present when the backend capability exists, including acquisition and recording controls.
- No hardware or acquisition algorithms are moved into Vue components.

## Architectural Constraints

Follow workspace rules from `CLAUDE.md` and project `AGENTS.md`.

Hard constraints:

- Frontend displays and coordinates UI only.
- Do not add hardware access to Vue.
- Do not move calibration, traversal, acquisition, or device-control algorithms into Vue.
- Wails backend remains thin.
- Preserve current API adapter boundaries where the old project used Electron IPC.
- Use current Go/Wails/HTTP APIs for behavior parity instead of preserving Electron IPC shapes.
- Do not introduce a new design system.
- Do not add mobile-first breakpoints. Desktop-first behavior is expected.

GitNexus constraint:

- Before editing a Vue component script function, method, composable, or store symbol, run upstream impact analysis for that symbol.
- For pure Markdown edits this is not required.

## Current Root Cause

The earlier frontend-only alignment plan was too narrow. It treated UI parity as small component cleanup, but the real UI mismatch is structural.

Reference behavior:

- `views/main/MainDashboardView.vue` owns the main app shell.
- It uses `views/MainView.vue` as the shell wrapper.
- It controls `activePage` locally.
- It embeds `MotionView`, `CalibrationView`, `TraversalView`, and `LogViewer` inside the same shell.
- It renders `MainBottomBar` only for the dashboard page.

Current `wind-daq` behavior:

- `views/main/MainLayout.vue` owns the shell.
- `router-view` swaps whole pages.
- `MainDashboardView.vue` only renders dashboard content.
- `MainBottomBar` is always rendered by layout.
- `storage` was added to the rail.

This architectural change made the UI feel different even when some tokens/components are similar.

## Current Real Status (2026-05-22)

The earlier plan assumed that Phases 1-9 could be executed sequentially. In practice, `ui-parity-audit.md` revealed that most UI components labelled `Done` in the feature map were only structurally present but visually and behaviorally incomplete.

**Before starting any phase**, read `ui-parity-audit.md` to see the current truthful status. Then read the reference file and the target file side by side. Do not rely on the Phase 0-9 labels below as proof of completion.

## Reference Files To Inspect First

Always inspect these reference files before editing target files:

```text
Cursor DAQ/src/renderer/src/App.vue
Cursor DAQ/src/renderer/src/views/main/MainDashboardView.vue
Cursor DAQ/src/renderer/src/views/MainView.vue
Cursor DAQ/src/renderer/src/components/layout/MainTopBar.vue
Cursor DAQ/src/renderer/src/components/layout/AppRailNav.vue
Cursor DAQ/src/renderer/src/components/layout/MainBottomBar.vue
Cursor DAQ/src/renderer/src/components/main/DeviceSidebar.vue
Cursor DAQ/src/renderer/src/components/main/DeviceOverviewPanel.vue
Cursor DAQ/src/renderer/src/components/main/DeviceDetailPanel.vue
Cursor DAQ/src/renderer/src/components/layout/GlobalSettingsModal.vue
Cursor DAQ/src/renderer/src/components/feedback/UiToastHost.vue
Cursor DAQ/src/renderer/src/components/feedback/UiConfirmDialog.vue
Cursor DAQ/src/renderer/src/style.css
Cursor DAQ/src/renderer/src/styles/tokens/color.css
Cursor DAQ/src/renderer/src/styles/tokens/spacing.css
Cursor DAQ/src/renderer/src/styles/tokens/radius.css
Cursor DAQ/src/renderer/src/styles/tokens/motion.css
Cursor DAQ/src/renderer/src/styles/tokens/typography.css
Cursor DAQ/src/renderer/src/styles/tokens/layout.css
Cursor DAQ/src/renderer/src/styles/themes/dark.css
Cursor DAQ/src/renderer/src/styles/themes/light.css
Cursor DAQ/src/renderer/src/styles/glass.css
```

## Target Files To Change

Expected primary target files:

```text
wind-daq/apps/desktop-wails/frontend/src/views/main/MainLayout.vue
wind-daq/apps/desktop-wails/frontend/src/views/main/MainDashboardView.vue
wind-daq/apps/desktop-wails/frontend/src/views/MainView.vue
wind-daq/apps/desktop-wails/frontend/src/components/layout/MainTopBar.vue
wind-daq/apps/desktop-wails/frontend/src/components/layout/AppRailNav.vue
wind-daq/apps/desktop-wails/frontend/src/components/layout/MainBottomBar.vue
wind-daq/apps/desktop-wails/frontend/src/components/layout/AppShell.vue
wind-daq/apps/desktop-wails/frontend/src/components/layout/GlobalSettingsModal.vue
wind-daq/apps/desktop-wails/frontend/src/components/main/DeviceSidebar.vue
wind-daq/apps/desktop-wails/frontend/src/components/main/DeviceOverviewPanel.vue
wind-daq/apps/desktop-wails/frontend/src/components/main/DeviceDetailPanel.vue
wind-daq/apps/desktop-wails/frontend/src/views/MotionView.vue
wind-daq/apps/desktop-wails/frontend/src/views/CalibrationView.vue
wind-daq/apps/desktop-wails/frontend/src/views/TraversalView.vue
wind-daq/apps/desktop-wails/frontend/src/views/LogViewer.vue
wind-daq/apps/desktop-wails/frontend/src/styles.css
wind-daq/apps/desktop-wails/frontend/src/styles/**
wind-daq/apps/desktop-wails/frontend/src/router/index.ts
```

Do not edit all files blindly. Edit only files required by each phase.

## Migration Strategy

Use the smallest changes that restore old visual behavior while improving architecture.

Preferred approach:

1. Port old templates and scoped styles first.
2. Adapt imports and props to current Wails aliases.
3. Preserve or add current Wails/API calls where required for old visible controls.
4. Move behavior to Go/backend APIs when the old project mixed business logic into Vue.
5. Remove new UI structure only when it conflicts with old parity.
6. Build and typecheck after each phase.

Avoid:

- Rewriting stores unless necessary.
- Replacing backend adapters.
- Adding compatibility layers for unused old Electron APIs.
- Styling current structure to vaguely resemble old UI while leaving shell behavior different.
- Removing old UI affordances without checking whether current backend capability exists.

## Phase 0: Baseline And Safety

Goal: capture current state and confirm reference/target file availability.

Actions:

1. Read current `git status`.
2. Read reference files listed above.
3. Read target files listed above.
4. Confirm current `package.json` scripts.
5. Do not modify code in this phase.

Commands:

```powershell
git status --short
```

Verification:

- You can identify the old shell owner and current shell owner.
- You can list the exact target files for Phase 1.

## Phase 1: Restore Old Shell Ownership

Goal: make `MainDashboardView.vue` own the old app shell again.

Reference behavior:

- `MainDashboardView.vue` imports shell/layout components.
- It keeps `activePage` as local state.
- It builds `railItems` from `activePage`.
- `handleRailSelect()` updates `activePage` without routing.
- Dashboard content and embedded sub-pages render inside one shell.

Target changes:

- Update `src/views/main/MainDashboardView.vue` to follow reference structure.
- Reintroduce `src/views/MainView.vue` if missing or currently deleted.
- Reduce `src/views/main/MainLayout.vue` to a thin wrapper or route only to the main dashboard shell.
- Remove `storage` from the main rail for parity.
- Keep Wails-specific `GetVersion`, stores, and API wiring where needed.

Important adaptation notes:

- Reference imports use `@renderer/...`; target uses aliases like `@components`, `@stores`, `@views`, `@api` depending on `vite.config.ts`.
- Do not copy Electron API imports directly.
- If old code uses `useMainDashboard`, compare with current availability before copying.

Acceptance checks:

- Top bar, rail, dashboard stage, and optional sidebar are all rendered from the same main shell.
- Clicking rail items does not navigate to visibly separate full-page shells.
- Non-dashboard pages appear in the old `page-container` and `page-content` area.
- Bottom bar only appears where the reference shows it.

## Phase 2: Top Bar Parity

Goal: make `MainTopBar.vue` match the reference visual and behavior.

Reference expectations:

- Height: 56px.
- Translucent dark background with blur.
- Brand block: green logo, `WindDAQ`, subtitle `DATA ACQUISITION`.
- Dashboard mode segmented buttons: chart, table, both, overview.
- Non-dashboard active page label shown in the same nav area.
- Status pill: acquiring or idle.
- Theme icon button.
- Locale switch.
- Version text.

Target changes:

- Port reference template and scoped CSS from `Cursor DAQ/components/layout/MainTopBar.vue`.
- Adapt props to current target naming only when necessary.
- If current target passes `labels`, either change caller to pass `t` like reference or add minimal prop compatibility. Prefer matching reference.
- Keep theme toggle behavior working with target `themeStore`.

Acceptance checks:

- The top bar looks visually identical to old Cursor DAQ.
- Dashboard mode buttons appear only on dashboard.
- Non-dashboard page label appears for motion/calibration/traversal/log.
- Theme and locale controls still work.

## Phase 3: Rail And Sidebar Parity

Goal: match old left navigation and dashboard sidebar.

Reference expectations:

- Rail items: dashboard, motion, calibration, traversal, log.
- Rail icons/text match old project style.
- Settings button behavior remains available.
- Device sidebar appears only on dashboard.
- Device sidebar receives status labels and opens device manager.

Target changes:

- Port or align `AppRailNav.vue` with reference.
- Port or align `DeviceSidebar.vue` with reference.
- Ensure `showDeviceDrawer` opens `DeviceManagementDrawer` as before.
- Remove or hide current-only `storage` rail item unless explicitly requested later.

Acceptance checks:

- Rail order and active state match Cursor DAQ.
- Dashboard shows sidebar.
- Embedded pages do not show dashboard sidebar.

## Phase 4: Dashboard Content Parity

Goal: make dashboard stage match old project.

Reference expectations:

- `main-dashboard-view` radial background.
- `main-dashboard-stage` scrollable content with `var(--space-4)` padding.
- Overview mode uses `DeviceOverviewPanel`.
- Detail modes use `DeviceDetailPanel`.
- Empty state matches reference dashed panel, icon, title, description, action button.

Target changes:

- Align `MainDashboardView.vue` dashboard section with reference.
- Port reference scoped CSS for dashboard stage and empty state.
- Align `DeviceOverviewPanel.vue` and `DeviceDetailPanel.vue` templates/styles.
- Keep current Wails acquisition controls only if required, but place them in old visual slots.

Acceptance checks:

- Dashboard background and empty state match old UI.
- Overview/detail modes preserve old spacing and panel structure.
- Start/stop/acquisition actions still work if currently available.

## Phase 5: Embedded Page Parity

Goal: make non-dashboard pages render like old embedded pages.

Reference behavior:

```vue
<div v-else class="page-container">
  <section class="page-content">
    <MotionView v-if="activePage === 'motion'" embedded />
    <CalibrationView v-else-if="activePage === 'calibration'" embedded />
    <TraversalView v-else-if="activePage === 'traversal'" embedded />
    <LogViewer v-else-if="activePage === 'log'" embedded />
  </section>
</div>
```

Target changes:

- Add or preserve `embedded` props in `MotionView.vue`, `CalibrationView.vue`, `TraversalView.vue`, and `LogViewer.vue`.
- Ensure embedded mode avoids creating another full shell.
- Port reference `page-container` and `page-content` CSS.
- Remove current full-page assumptions that visually conflict with embedded mode.

Acceptance checks:

- Motion/calibration/traversal/log all appear inside the same rounded translucent content container.
- No double headers, double rails, or nested app shells.
- Scroll and overflow behavior match old project.

## Phase 6: Bottom Bar Parity

Goal: match old bottom status bar behavior.

Reference expectations:

- `MainBottomBar` appears in dashboard shell statusbar slot.
- It receives acquisition and recording status.
- It exposes start, stop, and recording toggle where available.

Target changes:

- Port or align `MainBottomBar.vue` with reference.
- Wire current target store/API functions into old visual props/events.
- Avoid showing bottom bar on embedded non-dashboard pages if reference does not.

Acceptance checks:

- Bottom bar visual style matches old Cursor DAQ.
- Dashboard controls still work.
- Non-dashboard pages match old bottom-bar behavior.

## Phase 7: Theme, Tokens, And Global CSS Parity

Goal: ensure old colors, spacing, typography, radius, and global surface rules are restored.

Target files:

```text
src/styles.css
src/styles/tokens/color.css
src/styles/tokens/spacing.css
src/styles/tokens/radius.css
src/styles/tokens/motion.css
src/styles/tokens/typography.css
src/styles/tokens/layout.css
src/styles/themes/dark.css
src/styles/themes/light.css
src/styles/glass.css
```

Actions:

- Compare each target file against the reference equivalent.
- Port missing old values where safe.
- Preserve Wails build compatibility.
- Do not invent new token names unless needed by copied reference components.

Acceptance checks:

- Dark theme panel/background contrast matches Cursor DAQ.
- Light theme still works like reference.
- Typography and numeric display match old UI.
- No visual drift from custom new token values.

## Phase 8: Feedback And Modal Parity

Goal: keep old toast, confirm, settings modal, and device drawer behavior working.

Target changes:

- Compare `UiToastHost.vue` and `UiConfirmDialog.vue` against reference.
- Keep them mounted once at app/shell level.
- Align `GlobalSettingsModal.vue` and `GlobalSettingsModal.css` with reference.
- Ensure `DeviceManagementDrawer` still opens from old locations.

Acceptance checks:

- Toasts render above the shell correctly.
- Confirm dialog is usable and styled like old project.
- Settings modal matches reference.
- Device manager can be opened from sidebar/rail settings flow.

## Phase 9: Router Cleanup

Goal: avoid route structure fighting the old shell model.

Actions:

- Inspect `src/router/index.ts`.
- Prefer routing the app to a single `MainDashboardView` shell for the main experience.
- Remove or keep hidden child routes only if needed by tests or deep links.
- Do not expose separate full-screen pages if old UI did not.

Acceptance checks:

- Launching the app lands in the old main shell.
- Rail page switches happen inside the shell.
- No route transition creates a different outer layout.

## Phase 10: Verification

Run from target frontend directory:

```powershell
npm run build
npx vue-tsc --noEmit
```

If tests exist and are relevant:

```powershell
npm test
```

Run from workspace root after non-trivial changes:

```powershell
powershell -File .\scripts\validate-structure.ps1
```

Before commit, run GitNexus change detection:

```text
gitnexus_detect_changes(scope: "all", repo: "AI-WorkSpace")
```

Manual visual checks:

- Start app or dev server.
- Compare against Cursor DAQ reference at 1280x720.
- Check dashboard, motion, calibration, traversal, log.
- Toggle theme.
- Toggle language.
- Open settings.
- Open device manager.
- Start/stop acquisition if backend supports it.
- Trigger toast and confirm flows.

## AI Agent Execution Rules

Follow these rules during implementation:

- Work phase by phase.
- After each phase, build or typecheck if the phase changed Vue/TS code.
- Prefer copying reference template/style and adapting script imports over reimagining layout.
- Keep code changes minimal and traceable to old UI parity.
- Do not refactor unrelated stores, APIs, or backend code.
- Do not delete current Wails API wiring unless it is replaced by equivalent working wiring.
- When a visual mismatch is found, fix the component causing it instead of adding global CSS hacks.
- If reference behavior depends on Electron-only APIs, write a small Wails adapter at the API boundary or keep the existing target API path.
- If uncertain whether a current-only feature should remain visible, default to hiding it for old UI parity and ask the user if it should be retained.

## Known High-Risk Areas

- `MainDashboardView.vue`: high impact because it becomes shell owner again.
- `MainLayout.vue`: current router shell may conflict with restored old shell.
- `MainTopBar.vue`: prop shape differs between reference and target.
- `MainBottomBar.vue`: current acquisition/recording state may differ from reference storage state.
- `DeviceDetailPanel.vue`: Wails data model may differ from old Electron model.
- `CalibrationView.vue` and `TraversalView.vue`: must remain UI-only, no algorithm migration into frontend.
- Theme tokens: small token differences can cause large visual drift.

## Suggested Commit Breakdown

If committing later, use separate commits:

1. `docs(wind-daq): define cursor daq ui parity plan`
2. `refactor(wind-daq-ui): restore cursor daq shell structure`
3. `refactor(wind-daq-ui): align topbar rail and dashboard panels`
4. `refactor(wind-daq-ui): embed secondary pages in main shell`
5. `style(wind-daq-ui): align theme tokens with cursor daq`
6. `test(wind-daq-ui): update parity coverage`

Only commit if the user explicitly asks.

## Completion Report Template

Use this format when reporting implementation progress:

```markdown
## Completed
- [phase/file] What changed and why.

## Parity Status
- Shell: matched / partial / blocked
- Top bar: matched / partial / blocked
- Rail/sidebar: matched / partial / blocked
- Dashboard: matched / partial / blocked
- Embedded pages: matched / partial / blocked
- Theme/tokens: matched / partial / blocked

## Verification
- `npm run build`: pass/fail/not run
- `npx vue-tsc --noEmit`: pass/fail/not run
- `validate-structure.ps1`: pass/fail/not run

## Open Issues
- Any known mismatch, blocker, or user decision needed.
```
