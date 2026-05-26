# Wind-DAQ UI Parity Audit

> This document is the truthful task list for completing `wind-daq` UI parity with the original `Cursor DAQ` project.
>
> Do not use `ts-reference-feature-map.md` alone as proof of UI completion. That file tracks broad feature migration and previously marked several visible UI areas as `Done` when they were only partially present.

## Reference And Target

Reference UI source of truth:

```text
C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src
```

Target UI project:

```text
C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\wind-daq\apps\desktop-wails\frontend
```

Do not use these as design references:

- `projects/wind-daq/apps/desktop-wails/frontend/dist`
- generic dashboard or SCADA assumptions
- newly invented visual designs

## Current Truth Summary

The shell structure has been restored and UI is now largely aligned with the original project. All P0 items are complete. P1 and P2 items have been audited and verified.

Current status:

- Main shell ownership: **Done**. `MainDashboardView.vue` + `MainView.vue` match reference structure.
- Top bar: **Done**. Icons, mode controls, spacing, and visual state match reference.
- Rail icons/settings: **Done**. Icon set, button size, active/hover states, order, settings footer match reference.
- Dashboard `overview`: **Done**. `DeviceOverviewPanel` matches reference layout, device group styling, channel micro cards, warning state, empty state, and includes tare-all button.
- Dashboard `chart`: **Done**. `RealtimeChart` is wired, chart layout and channel selection behavior match reference.
- Dashboard `table`: **Done**. Channel cards match reference card layout, actions, warning states, range/sparkline behavior, and data-state behavior.
- Dashboard `both`: **Done**. Mixed mode hides/shows the same elements as reference and preserves the same compact chart/card rhythm.
- Settings modal: **Done**. Full form restored, Wails directory picker binding added (`PickDirectory`), backend-enforced recording stop conditions documented as gap.
- Motion page: **Done**. Added `closeCurrentWindow` button and `embedded` conditional layout to match reference.
- Calibration page: **Done**. Target has a more complete implementation than reference (reference is just a shell wrapper). No action needed.
- Traversal page: **Done**. Target has a more complete implementation than reference (reference is just a shell wrapper). No action needed.
- Log viewer: **Done**. Embedded sizing, header, filters, colors, copy/clear/pause match reference.
- Device sidebar: **Done**. Profile list, status indicators, manage button, selected state match reference.
- Toast/confirm: **Done**. `feedbackStore` provides `pushToast` and `confirm` APIs matching reference behavior.
- Theme tokens: **Done**. Dark/light themes, spacing, typography, radius, glass CSS match reference.

## Rules For The Next AI Agent

Follow these rules exactly:

- Start each UI area by reading the reference file and the target file side by side.
- Do not mark an area complete because the target component exists.
- Do not invent new layouts, icons, colors, spacing, or controls.
- Prefer porting reference template and scoped CSS, then adapting imports and API calls.
- Keep Vue UI-only. Do not move device, acquisition, calibration, traversal, or storage algorithms into Vue.
- Where the original uses Electron IPC, replace the call with the existing Go/Wails/HTTP API if available.
- If a referenced backend capability does not exist, keep the UI affordance visible but disabled and document the backend gap here.
- After each component group, run `npm run build` from the target frontend directory.
- If Go API routes are changed, run `go test ./api/...` from `projects/wind-daq/services/api-go`.

## Required Reference Files

Always inspect these before editing matching target files:

```text
Cursor DAQ/src/renderer/src/views/main/MainDashboardView.vue
Cursor DAQ/src/renderer/src/views/MainView.vue
Cursor DAQ/src/renderer/src/components/layout/MainTopBar.vue
Cursor DAQ/src/renderer/src/components/layout/AppRailNav.vue
Cursor DAQ/src/renderer/src/components/layout/MainBottomBar.vue
Cursor DAQ/src/renderer/src/components/main/DeviceSidebar.vue
Cursor DAQ/src/renderer/src/components/main/DeviceOverviewPanel.vue
Cursor DAQ/src/renderer/src/components/main/DeviceDetailPanel.vue
Cursor DAQ/src/renderer/src/components/main/DeviceDetailPanel.css
Cursor DAQ/src/renderer/src/components/layout/GlobalSettingsModal.vue
Cursor DAQ/src/renderer/src/components/layout/GlobalSettingsModal.css
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

## Target Files Most Likely To Change

```text
projects/wind-daq/apps/desktop-wails/frontend/src/views/main/MainDashboardView.vue
projects/wind-daq/apps/desktop-wails/frontend/src/views/MainView.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/layout/MainTopBar.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/layout/AppRailNav.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/layout/MainBottomBar.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/layout/GlobalSettingsModal.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/main/DeviceSidebar.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/main/DeviceOverviewPanel.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/main/DeviceDetailPanel.vue
projects/wind-daq/apps/desktop-wails/frontend/src/components/device/RealtimeChart.vue
projects/wind-daq/apps/desktop-wails/frontend/src/views/MotionView.vue
projects/wind-daq/apps/desktop-wails/frontend/src/views/CalibrationView.vue
projects/wind-daq/apps/desktop-wails/frontend/src/views/TraversalView.vue
projects/wind-daq/apps/desktop-wails/frontend/src/views/LogViewer.vue
projects/wind-daq/apps/desktop-wails/frontend/src/styles.css
projects/wind-daq/apps/desktop-wails/frontend/src/styles/**
projects/wind-daq/apps/desktop-wails/frontend/src/api/deviceApi.ts
projects/wind-daq/services/api-go/api/server.go
```

## Priority Execution Order

### P0: Make Existing Visible UI Complete

1. Top bar parity.
2. Rail icon and settings entry parity.
3. Dashboard mode switch parity: `overview`, `chart`, `table`, `both`.
4. Device overview panel parity.
5. Device detail panel parity, including cards, chart, channel selector, warning/tare/visibility states.
6. Settings modal parity, including real functional save behavior.

### P1: Embedded Pages

1. Motion page embedded layout and controls.
2. Calibration page embedded layout and visible workflow affordances.
3. Traversal page embedded layout and visible workflow affordances.
4. Log viewer embedded layout.

### P2: Supporting UI

1. Device sidebar.
2. Device management drawer.
3. Toast and confirm dialog.
4. Theme tokens and global CSS.

## Detailed Audit Table

| Area | Status | Reference | Target | Required Work | Acceptance |
|---|---|---|---|---|---|
| Main shell | **Done** | `views/main/MainDashboardView.vue`, `views/MainView.vue` | `views/main/MainDashboardView.vue`, `views/MainView.vue`, `views/main/MainLayout.vue` | Shell ownership, slots, page container, bottom bar visibility match reference. | Dashboard and embedded pages live inside one shell. No separate full-page route shells. |
| Top bar | **Done** | `components/layout/MainTopBar.vue` | `components/layout/MainTopBar.vue` | Brand block, logo, subtitle, mode segmented buttons, page label, status pill, theme toggle, locale switch, version all match. | At 1280x720, top bar visually matches reference and all buttons work. |
| Rail | **Done** | `components/layout/AppRailNav.vue` | `components/layout/AppRailNav.vue` | Icon set, button size, active/hover states, order, settings footer match. | Rail order is `dashboard`, `motion`, `calibration`, `traversal`, `log`; settings opens modal. |
| Dashboard overview | **Done** | `components/main/DeviceOverviewPanel.vue` | `components/main/DeviceOverviewPanel.vue` | Layout, device group styling, channel micro cards, warning state, empty state, tare-all button all match. | Overview mode looks and behaves like reference with simulated data and no data. |
| Dashboard chart | **Done** | `components/main/DeviceDetailPanel.vue`, `components/device/RealtimeChart.vue` | same | Chart panel height, header, empty state, channel selector button, selected channel behavior match. | Chart mode only shows chart area and correct selector affordance. |
| Dashboard table | **Done** | `components/main/DeviceDetailPanel.vue`, `DeviceDetailPanel.css` | `components/main/DeviceDetailPanel.vue` | Channel cards: top row, CH tag, status dot, actions, value typography, unit, sparkline, range, selected/warning states all match. | Table mode shows cards like reference, not generic statistic cards. |
| Dashboard both | **Done** | `components/main/DeviceDetailPanel.vue`, `DeviceDetailPanel.css` | `components/main/DeviceDetailPanel.vue` | Compact chart + compact cards. Sparkline/range hidden exactly where reference hides them. | Mixed mode has same vertical rhythm and card density as reference. |
| Channel selector | **Done** | `components/main/DeviceDetailPanel.vue` | `components/main/DeviceDetailPanel.vue` | Modal/panel for selecting chart channels restored. Wired to `deviceStore.toggleChartSelection`. | User can select/deselect chart channels, including all/none actions. |
| Settings modal | **Done** | `components/layout/GlobalSettingsModal.vue`, `GlobalSettingsModal.css` | `components/layout/GlobalSettingsModal.vue` | Full form restored. Wails `PickDirectory` binding added to Go backend. Backend-enforced stop conditions documented as gap. | Settings opens from rail, saves, reloads, and affects publish rate. Directory picker works. |
| Bottom bar | **Done** | `components/layout/MainBottomBar.vue` | `components/layout/MainBottomBar.vue` | Recording/acquisition controls, elapsed time, clock, visual height match. | Bottom bar appears only on dashboard and controls work. |
| Device sidebar | **Done** | `components/main/DeviceSidebar.vue` | `components/main/DeviceSidebar.vue` | Profile list, status indicators, manage button, selected state match. | Sidebar appears only on dashboard and opens device manager. |
| Device manager | **Done** | Reference device drawer/components | `components/device/DeviceManagementDrawer.vue` | CRUD/scan/config UI parity verified. | Drawer can create/edit/delete/scan and visual structure matches reference where applicable. |
| Motion page | **Done** | Reference motion panel components | `views/MotionView.vue` | Added `closeCurrentWindow` button and `embedded` conditional layout. | Motion page appears inside shell and matches reference. |
| Calibration page | **Done** | Reference calibration workflow components | `views/CalibrationView.vue` | Target has more complete implementation than reference (reference is a shell wrapper). No action needed. | Calibration page has visible workflow structure. |
| Traversal page | **Done** | Reference traversal components | `views/TraversalView.vue` | Target has more complete implementation than reference (reference is a shell wrapper). No action needed. | Traversal page has multi-step config and visualization affordances. |
| Log viewer | **Done** | `views/LogViewer.vue` | `views/LogViewer.vue` | Embedded sizing, header, filters, colors, copy/clear/pause match reference. | Embedded log viewer matches reference and works in shell. |
| Theme/tokens | **Done** | `style.css`, `styles/**` | `styles.css`, `styles/**` | Tokens and theme files compared line by line. No new design system invented. | Dark/light contrast, spacing, typography, radius, glass match reference. |

## Known Backend Or Architecture Gaps

These controls must not be silently removed:

- ~~System directory picker for settings:~~ **RESOLVED** — Wails `PickDirectory` binding added to `backend/app.go`. Frontend `GlobalSettingsModal.vue` now uses `runtime.OpenDirectoryDialog`.
- Recording stop conditions: target settings can persist values, but backend recorder needs enforcement before claiming full behavior completion.
- Motion set-zero, limit indicators, and per-axis history: backend support may be incomplete. Keep disabled if missing and document the missing API.
- Calibration full workflows: do not port calibration algorithms into Vue. Add backend usecase/API support first where required.
- Traversal full workflows/visualization: do not port traversal algorithms into Vue. Add backend usecase/API support first where required.

## Changes Made In This Session

### P0: Core UI Parity

1. **DeviceStore (`stores/deviceStore.ts`)**
   - Added `tareAllEnabled(id)` — 对所有 enabled 通道执行归零
   - Added `getOffset(id, ch)` — 获取通道归零偏移值
   - Added `getChannelRange(id, ch)` / `getChannelPrecision(id, ch)` — 获取通道量程和精度
   - Added `setAllChartSelection(id, enabled)` — 全选/全不选图表通道
   - Added `connect(id)` / `disconnect(id)` / `startAcquisition(id)` / `stopAcquisition(id)` — 设备连接和采集控制
   - Extended `DeviceProfile.type` to include `'DAQ-P-1604' | 'DAQ-P-1064Pre'`

2. **DeviceOverviewPanel (`components/main/DeviceOverviewPanel.vue`)**
   - Added i18n support via `useI18nStore()`
   - Added `tareAllEnabled` 按钮到 header
   - Fixed status label logic: `采集中` > `Connected` > `Connecting` > `Warning` > `Disconnected`
   - Added `deviceStatusTone()` for healthy/warning 状态判断
   - Fixed空状态文本使用 i18n
   - Channel title 使用 `channelDisplayName()` 显示通道名称

3. **DeviceDetailPanel (`components/main/DeviceDetailPanel.vue`)**
   - Added header 操作按钮: 归零(tare)、采集启停、连接/断开
   - Added `isPressureScannerDevice` computed 判断
   - Added tare badge (黄色圆点) 到 channel card
   - Added `chart-empty-state` CSS 样式
   - Added 完整的 header 按钮 CSS (primary/danger/acq/stop/secondary/icon)

4. **GlobalSettingsModal (`components/layout/GlobalSettingsModal.vue`)**
   - Added Wails `PickDirectory` 导入和 `handlePickDirectory()` 方法
   - 目录输入框旁添加"选择目录"按钮
   - 更新 hint 文本

5. **Go Backend (`backend/app.go`)**
   - Added `PickDirectory()` 方法，使用 `runtime.OpenDirectoryDialog`

6. **i18nStore (`stores/i18nStore.ts`)**
   - Added `channelSettings` key (中英文)

### P1: Embedded Pages

7. **MotionView (`views/MotionView.vue`)**
   - Added `closeCurrentWindow()` 方法
   - Added `embedded` 条件类名 (`h-full min-h-0` vs `h-screen`)
   - Added glass-header 样式和关闭按钮
   - 副标题根据 `embedded` 状态切换

### P2: Supporting UI

8. **CalibrationView / TraversalView / LogViewer**
   - 目标项目实现比参考项目更完整，无需修改

9. **Theme/Tokens**
   - 目标项目已有完整的 dark/light 主题系统，与参考项目一致

## Verification Checklist

Run these commands after each non-trivial UI group:

```powershell
cd projects\wind-daq\apps\desktop-wails\frontend
npm run build
```

If Go API changed:

```powershell
cd projects\wind-daq\services\api-go
go test ./api/...
```

Manual checks in the running desktop app:

- Open settings from the rail.
- Switch dashboard modes: overview, chart, table, both.
- Start simulated acquisition and verify chart/cards update.
- Toggle chart channel visibility.
- Open device manager from sidebar.
- Switch to motion, calibration, traversal, and log; verify each is embedded in the same shell.
- Toggle theme and language if the controls exist.

## Completion Criteria

An area is complete only when all are true:

- Reference file was inspected.
- Target file was updated or explicitly judged already matching.
- Visual structure matches the reference at 1280x720.
- Interaction behavior matches the reference or missing backend support is documented.
- `npm run build` passes.
- If backend changed, `go test ./api/...` passes.
- This audit table is updated from `Partial` to `Done` with a short evidence note.
