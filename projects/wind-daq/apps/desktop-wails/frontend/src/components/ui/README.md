# Wind-DAQ UI Component Rules

> This directory is the project-level UI foundation for Wind-DAQ. Use these components first when building product UI. Naive UI is the underlying widget library, not the default surface for feature code.

## Role

`components/ui/` owns small, reusable UI primitives that keep Wind-DAQ visually consistent:

- `UiButton` for standard actions
- `UiInput` for text-like input
- `UiSelect` for single selection
- `UiToggle` for boolean settings
- `UiPanel` for panel/card containers
- `UiSectionHeader` for compact section titles
- `UiStatusBadge` for device and workflow status

These components may wrap Naive UI, but they must not know about devices, calibration, traversal, motion, storage, reports, or any business workflow.

## Usage Rules

Use `Ui*` components when the interaction matches an existing primitive:

- Use `UiButton` before direct `NButton` for normal feature actions.
- Use `UiInput` before direct `NInput` for simple text or numeric text fields.
- Use `UiSelect` before direct `NSelect` for simple single-select fields.
- Use `UiToggle` before direct `NSwitch` for boolean settings.
- Use `UiPanel` before direct `NCard` for product panels.
- Use `UiStatusBadge` before direct `NTag` for connected, acquiring, warning, error, and idle status.

Direct Naive UI usage is allowed when:

- No `Ui*` wrapper exists yet.
- The component is complex enough that wrapping it first would add churn, such as `NDataTable`, `NModal`, `NSteps`, `NInputNumber`, `NCheckbox`, or chart-adjacent controls.
- The file is a spike or migration-only experiment.

When direct Naive UI usage repeats across two or more feature areas, add or extend a `Ui*` primitive before continuing broad usage.

## API Rules

Keep primitive APIs small and semantic:

- Use product terms like `variant`, `size`, `status`, and `padded`.
- Do not expose every Naive UI prop by default.
- Add props only when at least two real call sites need them.
- Preserve existing prop names unless there is a concrete migration plan.

Do not add:

- Business-domain props such as `deviceId`, `calibrationType`, `axisName`, or `probeMode`.
- Store imports.
- API calls.
- File, hardware, serial, TCP, or Wails calls.

## Visual Rules

Primitives must use project tokens from `styles/tokens/` or Naive theme overrides from `App.vue`.

Avoid in primitive components:

- Inline font sizes.
- Inline spacing values.
- Raw hex colors.
- Component-local visual systems that do not map back to tokens.

Existing inline styles in this directory are migration debt. New changes should move repeated values into scoped classes or token-backed CSS.

## Missing Primitives

Add these next before large UI cleanup work:

- `UiFormField` for compact label, unit, hint, and field error layout.
- `UiDialog` for standard modal shell behavior.
- `UiEmptyState` for empty data panels.
- `UiLoadingState` for loading panels.
- `UiErrorState` for recoverable UI errors.
- `UiToolbar` for dense action rows.
- `UiDataTableShell` for table container, toolbar, empty, and loading states.

## Review Checklist

Before merging UI changes, confirm:

- Existing `Ui*` primitives were used when available.
- New primitives are business-agnostic.
- Direct Naive UI usage is justified.
- Loading, empty, error, disabled, and selected states are covered where applicable.
- Long labels and small target windows do not break layout.
- Styles use project tokens rather than raw visual values.
