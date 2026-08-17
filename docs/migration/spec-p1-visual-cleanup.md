# Spec: WindLabX4 P1 Visual Cleanup — Inline Styles & Raw Values → Design Tokens

## Objective

Replace all inline `style=""` attributes, raw hex colors, and hardcoded `px` values in feature components with CSS custom properties from `styles/tokens/`. After cleanup, no feature component should contain raw visual values — all colors, spacing, font sizes, and border radii must reference project design tokens.

**Success criteria:**
1. Zero inline `style=""` attributes in feature component `<template>` sections
2. Zero raw hex colors (e.g. `#334155`, `#38bdf8`) in feature component `<style>` blocks
3. Zero hardcoded `px` values for spacing, radius, font-size in feature `<style>` blocks
4. `npm run typecheck && npm run build` passes
5. Visual appearance is preserved

## Tech Stack

- **Token system:** `styles/tokens/{color,spacing,typography,radius,motion,layout}.css`
- **Themes:** `styles/themes/{dark,light}.css`
- **Framework:** Vue 3 scoped styles + Tailwind utility classes

## Available Tokens (reference)

```css
/* Color tokens (from color.css) */
--color-bg-panel           /* Panel backgrounds */
--color-bg-surface         /* Surface/container backgrounds */
--color-bg-canvas          /* Page background */
--color-text-primary       /* Primary text */
--color-text-secondary     /* Secondary text */
--color-text-muted         /* Muted/disabled text */
--color-border-default     /* Default borders */
--color-accent             /* Primary action color */
--color-success            /* Success states */
--color-warning            /* Warning states */
--color-danger             /* Error/danger states */
--color-info               /* Info states */

/* Spacing tokens (from spacing.css) */
--space-1     /* 4px */
--space-2     /* 8px */
--space-3     /* 12px */
--space-4     /* 16px */
--space-5     /* 20px */
--space-6     /* 24px */
--space-8     /* 32px */
--space-10    /* 40px */
--space-12    /* 48px */

/* Typography tokens (from typography.css) */
--text-xs     /* 11px */
--text-sm     /* 12px */
--text-base   /* 13px */
--text-lg     /* 14px */
--text-xl     /* 16px */
--font-mono   /* Monospace font stack */

/* Radius tokens (from radius.css) */
--radius-sm   /* 2px */
--radius-md   /* 4px */
--radius-lg   /* 6px */
--radius-xl   /* 8px */

/* Layout tokens (from layout.css) */
--header-height   /* 56px */
--rail-width      /* 64px */
--sidebar-width   /* 244px */
--footer-height   /* 72px */
```

## Commands

```powershell
cd projects\windlabx4\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test
```

## Code Style

```vue
<!-- BAD: inline style, raw hex, raw px -->
<div style="margin-bottom:12px;font-size:11px;color:#334155">
  状态正常
</div>

<!-- GOOD: token-backed classes -->
<div class="status-text">
  状态正常
</div>
<style scoped>
.status-text {
  margin-bottom: var(--space-3);
  font-size: var(--text-xs);
  color: var(--color-text-secondary);
}
</style>

<!-- BAD: raw px in style block -->
<style scoped>
.panel { padding: 10px 12px; border-radius: 4px; font-size: 11px; }
</style>

<!-- GOOD: token values -->
<style scoped>
.panel { padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); font-size: var(--text-xs); }
</style>
```

## Testing Strategy

- Existing tests must continue passing
- No new tests — purely visual refactoring (same rendered output)
- Verification: `npm run typecheck && npm run build && npm run test`

## Plan

### Phase order (files grouped by domain, each group visible-verifiable independently)

```
Phase 1: Ui* primitives (UiButton, UiPanel, UiSectionHeader, UiDialog)
  → clean up the wrappers themselves first (currently 5 inline style instances)
  → low risk, foundation for everything else

Phase 2: Calibration Settings (TotalTemperatureSettings, TotalPressureSettings, ThreeHoleSettings, FiveHoleSettings)
  → ~30 inline styles + ~30 raw px each (~120 total). High impact, repetitive patterns.
  → All 4 settings files share nearly identical CSS — migrate in parallel.

Phase 3: Traversal components (TraversalLayoutStep, TraversalPrbStep, TraversalSettings, TraversalHardwareStep)
  → ~90 + ~40 + ~18 + ~15 inline styles (~163 total). The worst offender.
  → LayoutStep alone has ~90 inline styles on NText/NInput/NInputNumber elements.

Phase 4: Motion components (MotionControlPanel, AxisConfigCard, ConnectionConfigEditor, EncoderCompensationEditor, MotionControllerConfig)
  → ~30 + ~30 + ~15 + ~10 + ~18 raw px (~103 total)

Phase 5: Device components (DeviceManagementDrawer, DaqT1603Config, RecordingControl)
  → ~80 + ~2 + ~4 raw px (~86 total)

Phase 6: Visualization + remaining (ProbeReferenceCard, PointsPreview, LogViewer, MainDashboardView)
  → ~35 + ~14 + ~25 + ~4 raw hex colors (~78 total). Color-focused.
```

### Dependency Graph

```
Phase 1: Ui* primitives (independent, foundation)
  │
  ├── Phase 2: Calibration Settings (depends on Ui* being clean — otherwise mixed patterns)
  ├── Phase 3: Traversal (depends on Ui* being clean)
  ├── Phase 4: Motion (depends on Ui* being clean)
  ├── Phase 5: Device (depends on Ui* being clean)
  └── Phase 6: Visualization (independent — SVG colors, chart themes)
```

### Risks

| Risk | Mitigation |
|------|-----------|
| Tailwind arbitrary values `min-h-[360px]` in visualization | Replace with nearest token or keep if no token exists (chart containers need exact pixel heights) |
| SVG fills/strokes in ProbeReferenceCard (35 raw hex colors) | Define SVG-specific token overrides or keep as component-local design choices |
| Glass effect `rgba()` values in glass.css | Glass.css is a utility layer — raw rgba is acceptable here |
| Over-tokenization: replacing px that are actually layout-critical | Only replace spacing, radius, font-size; preserve explicit layout dimensions that match the parity target |

## Tasks

### Phase 1: Ui* primitives (2 files)

- **1a: Clean UiSectionHeader.vue** — replace 2 inline styles with token classes
  - Acceptance: `style="display:block;font-size:0.65rem;font-weight:700;text-transform:uppercase"` becomes class-based
  - Files: `components/ui/UiSectionHeader.vue`

- **1b: Clean UiPanel.vue + UiDialog.vue** — replace `:content-style` with class-based padding
  - Acceptance: Inline padding values use `var(--space-*)`
  - Files: `components/ui/UiPanel.vue`, `components/ui/UiDialog.vue`

### Phase 2: Calibration Settings (4 files, parallel)

- **2a: TotalTemperatureSettings.vue** — migrate ~30 inline styles + ~30 raw px to tokens
- **2b: TotalPressureSettings.vue** — same pattern (nearly identical)
- **2c: ThreeHoleSettings.vue** — same pattern
- **2d: FiveHoleSettings.vue** — same pattern
  - Acceptance: All `font-size:11px` → `var(--text-xs)`, `padding:8px 10px` → `var(--space-2) var(--space-2)`, hex colors → `var(--color-*)`
  - Files: `components/calibration/*/*Settings.vue`

### Phase 3: Traversal (4 files, sequential by size)

- **3a: TraversalLayoutStep.vue** — migrate ~90 inline style attributes to scoped classes
- **3b: TraversalPrbStep.vue** — migrate ~40 inline styles
- **3c: TraversalSettings.vue** — migrate ~18 inline styles
- **3d: TraversalHardwareStep.vue** — migrate ~15 inline styles
  - Acceptance: Zero `style="..."` in templates; repeated patterns extracted to shared class
  - Files: `components/traversal/Traversal*.vue`

### Phase 4: Motion (5 files, parallel)

- **4a: Migrate MotionControlPanel.vue** — ~30 raw px → tokens
- **4b: Migrate AxisConfigCard.vue** — ~30 raw px → tokens
- **4c: Migrate ConnectionConfigEditor.vue** — ~15 raw px → tokens
- **4d: Migrate EncoderCompensationEditor.vue** — ~10 raw px → tokens
- **4e: Migrate MotionControllerConfig.vue** — ~18 raw px → tokens
  - Acceptance: No `width:80px`, `border-radius:3px`, `backdrop-filter:blur(4px)` raw values
  - Files: `components/motion/*.vue`

### Phase 5: Device (1 large + 2 small, sequential)

- **5a: DeviceManagementDrawer.vue** — migrate ~80 raw px values
  - Acceptance: `width:560px` → layout token, `border-radius:3px` → `var(--radius-sm)`, `backdrop-filter:blur(4px)` removed or tokenized
  - Files: `components/device/DeviceManagementDrawer.vue`

- **5b: DaqT1603Config.vue + RecordingControl.vue** — minor px cleanup
  - Files: `components/device/DaqT1603Config.vue`, `components/device/RecordingControl.vue`

### Phase 6: Remaining visual files (4 files, parallel)

- **6a: ProbeReferenceCard.vue** — SVG hex colors → component-level CSS variables
- **6b: PointsPreview.vue** — raw hex → token variables (~14 colors)
- **6c: LogViewer.vue** — raw hex → token variables (~25 colors)
- **6d: MainDashboardView.vue** — raw hex → token variables (~4 colors)
  - Acceptance: Colors reference `var(--color-*)` tokens; SVG marker colors use local CSS vars with token defaults
  - Files: as listed

## Boundaries

- **Always:**
  - Run `npm run typecheck` after each phase
  - Verify visual appearance is preserved (button colors, panel backgrounds, text colors must match)
  - Use existing tokens from `styles/tokens/` — do not add new tokens without approval
  - Preserve exact parity with reference UI dimensions
- **Ask first:**
  - Adding new CSS custom properties to tokens
  - Changing layout dimensions (header height, rail width, sidebar width)
  - Removing glass effects
  - Changing SVG probe diagram colors (they have specific engineering meaning)
- **Never:**
  - Remove Tailwind utility classes that are already token-correct
  - Change component structure (div nesting, slot arrangement) — visual cleanup only
  - Remove `backdrop-filter` glass effects — these are a DESIGN.md requirement
