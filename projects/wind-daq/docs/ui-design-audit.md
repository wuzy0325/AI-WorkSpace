# Wind-DAQ UI Design Audit

> This audit tracks the path from the current mixed UI implementation to a consistent Wind-DAQ design system. It is intentionally incremental: do not rewrite the whole UI at once.

## Current State

Wind-DAQ already has the foundation for a UI system:

- Naive UI is installed and used as the underlying widget library.
- `shared/frontend/NaiveThemeProvider.vue` bridges Naive UI theme providers into the app.
- `App.vue` maps Wind-DAQ light/dark theme values into Naive UI theme overrides.
- `src/styles/tokens/` contains color, spacing, typography, layout, radius, and motion tokens.
- `src/components/ui/` contains project-level `Ui*` primitives.
- `src/spikes/UiLibrarySpikeView.vue` is a Naive UI theme and density spike gated by `VITE_UI_SPIKE=1`.

The current implementation is not yet a fully unified component library:

- Many feature files still import Naive UI directly.
- Many templates contain inline style attributes.
- Several pages contain raw colors, raw spacing, and page-local visual systems.
- Some `Ui*` primitives still contain inline values that should become token-backed classes.

## Baseline Decision

Use this strategy for future UI work:

- Naive UI is the low-level component toolkit.
- `components/ui/Ui*` is the default project primitive layer.
- Feature components should use `Ui*` first and direct Naive UI only when no project wrapper exists.
- `styles/tokens/` is the visual source of truth.
- `DESIGN.md` remains the project-level product design target and parity constraint.

## Audit Findings

Observed through source search:

- Direct Naive UI imports appear in feature, layout, page, feedback, device, motion, traversal, and calibration components.
- Inline `style="..."` appears heavily in traversal and calibration setup flows.
- Raw colors and raw `px` / `rem` values appear in page and component CSS, especially log, storage, traversal, motion, and dashboard surfaces.
- `UiButton`, `UiPanel`, `UiSectionHeader`, `UiStatusBadge`, `UiSelect`, and `UiToggle` are already used in several real features.
- `UiInput`, `UiSelect`, and `UiPanel` APIs are intentionally minimal and do not yet cover all common form patterns.

These findings are expected migration debt, not immediate defects.

## Migration Order

### Phase 1: Rule and Asset Stabilization

Status: in progress.

- Add AI-executable frontend rules.
- Add `components/ui/README.md`.
- Add `styles/tokens/README.md`.
- Keep `DESIGN.md` as the Wind-DAQ product UI target.
- Record this audit as the UI migration backlog.

### Phase 2: Primitive Completion

Add missing primitives before broad page cleanup:

- `UiFormField`
- `UiDialog`
- `UiEmptyState`
- `UiLoadingState`
- `UiErrorState`
- `UiToolbar`
- `UiDataTableShell`

Priority should go to primitives that reduce repeated inline style and repeated Naive UI composition.

### Phase 3: Low-Risk Replacement

Replace direct Naive UI usage only where the mapping is straightforward:

- `NButton` to `UiButton` for normal actions.
- `NInput` to `UiInput` for simple text fields.
- `NSelect` to `UiSelect` for simple single-select fields.
- `NTag` to `UiStatusBadge` for status labels.
- `NCard` to `UiPanel` for simple product panels.

Do not replace complex controls until a matching project primitive exists.

### Phase 4: State Pattern Unification

Unify repeated state rendering:

- Loading panels.
- Empty panels.
- Error panels.
- Disabled capability gaps.
- Offline and reconnecting states.
- Saving and saved feedback.

Device, acquisition, recording, motion, traversal, and calibration flows must expose visible state rather than relying on button labels alone.

### Phase 5: Page-by-Page Cleanup

Proceed in this order:

1. Main shell, rail, top bar, bottom bar.
2. Device overview and device detail surfaces.
3. Storage and log pages.
4. Motion configuration and control panels.
5. Traversal setup and visualization.
6. Calibration home, settings, and run screens.

Each page batch must preserve current operator workflow and run the frontend verification commands.

## Do Not Do

- Do not introduce a second UI library.
- Do not wrap every Naive UI component preemptively.
- Do not rewrite all pages for visual purity.
- Do not remove parity-critical controls just because they are visually inconsistent.
- Do not move backend business rules into Vue to simplify UI state.

## Acceptance Checklist

A UI cleanup batch is acceptable when:

- It follows `docs/runbooks/frontend-ai-rules.zh-CN.md`.
- It follows `DESIGN.md` and keeps the operator workflow recognizable.
- It uses existing `Ui*` primitives where they fit.
- It uses project tokens for visual values.
- It improves or preserves loading, empty, error, disabled, and offline states.
- It passes `npm run typecheck` and `npm run build` from the frontend directory.
- It documents any skipped visual or state gaps in this audit or a follow-up task.
