# Spec: WindLabX4 P1 Naive UI → Ui\* Replacement

## Objective

Replace all direct `naive-ui` imports in feature/business code with existing `Ui*` project primitives, and create missing `Ui*` wrappers for commonly used Naive UI components that lack them.

After refactoring, feature code will not import `NButton`, `NCard`, `NInput`, `NSelect`, `NTag`, `NModal`, `NText`, or `NSwitch` directly — these will only appear inside `components/ui/` wrapper files.

**Success criteria:**
1. 35 feature files no longer import Naive UI components that have Ui\* wrappers
2. New Ui* wrappers created: `UiCheckbox`, `UiInputNumber`, `UiAlert`, `UiSteps`, `UiSpin`
3. `npm run typecheck && npm run build` passes
4. Visual behavior is preserved — buttons look the same, inputs behave the same

## Tech Stack

- **Frontend:** Vue 3 + TypeScript + Naive UI (underlying library)
- **Primitives:** `components/ui/Ui*` — WindLabX4 project component layer
- **Tokens:** `styles/tokens/` — color, spacing, typography CSS custom properties

## Commands

```powershell
# After each group
cd projects\windlabx4\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test
```

## Project Structure (affected files)

```
frontend/src/
├── components/
│   ├── ui/                          ← CREATE: UiCheckbox, UiInputNumber, UiAlert, UiSteps, UiSpin
│   ├── calibration/*/               ← REPLACE: NButton→UiButton, NText→UiSectionHeader, NCard→UiPanel
│   ├── traversal/                   ← REPLACE: NButton→UiButton, NSelect→UiSelect, NInput→UiInput
│   ├── motion/                      ← REPLACE: NButton→UiButton, NSelect→UiSelect, NInput→UiInput
│   ├── device/                      ← REPLACE: NButton→UiButton, NInput→UiInput, NSelect→UiSelect
│   ├── main/                        ← REPLACE: NButton→UiButton
│   └── layout/                      ← REPLACE: NButton→UiButton, NModal→UiDialog
├── views/                           ← REPLACE: NButton→UiButton, NInput→UiInput
└── spikes/                          ← no change (spike file)
```

## Code Style

```vue
<!-- GOOD: Feature code uses Ui* wrappers -->
<UiButton variant="primary" @click="start">开始采集</UiButton>
<UiSelect v-model="type" :options="types" />

<!-- BAD: Direct Naive UI in feature code -->
<NButton type="primary" @click="start">开始采集</NButton>
<NSelect v-model:value="type" :options="types" />

<!-- Ui wrapper keeps Naive UI props, exposes simplified API -->
<!-- UiButton.vue -->
<template>
  <NButton v-bind="naiveProps"><slot /></NButton>
</template>
```

When replacing, match the exact Naive UI prop to the corresponding Ui\* prop:

| Naive UI | Ui\* | Notes |
|----------|------|-------|
| `type="primary"` | `variant="primary"` | UiButton maps `primary`→`primary`, `warning`→`warning`, `error`→`danger` |
| `size="small"` | `size="sm"` | Ui* uses 2-letter size codes |
| `:v-model:value="x"` | `v-model="x"` | Ui* uses standard v-model |
| `label` slot | `default` slot | check Ui* component API in README |

## Testing Strategy

- Existing tests must continue passing
- No new tests — this is a pure import replacement (behavior-preserving)
- Verification: `npm run typecheck && npm run build && npm run test`

## Plan

### Phase order (sequential groups, files within each group are parallel)

```
Phase 1: Create missing Ui* wrappers (UiCheckbox, UiInputNumber, UiAlert, UiSteps, UiSpin)
  → enables Phase 2-4 to fully eliminate Naive UI imports

Phase 2: Replace NButton → UiButton (28 files, biggest win)
  → can be done file-by-file in parallel

Phase 3: Replace pairs (NSelect→UiSelect, NInput→UiInput, NCard→UiPanel, NSwitch→UiToggle)
  → 9 + 7 + 7 + 2 files, parallel within group

Phase 4: Replace remaining (NText→UiSectionHeader, NTag→UiStatusBadge, NModal→UiDialog)
  → 9 + 5 + 5 files, parallel within group

Phase 5: Clean up — remove unused naive-ui imports, verify build
```

### Dependency Graph

```
Create missing Ui* wrappers (Phase 1)
  │
  ├── Phase 2: NButton → UiButton (no new wrappers needed, UiButton exists)
  │
  ├── Phase 3: NSelect→UiSelect, NInput→UiInput, NCard→UiPanel, NSwitch→UiToggle
  │   └── All wrappers already exist
  │
  └── Phase 4: NText→UiSectionHeader, NTag→UiStatusBadge, NModal→UiDialog
      └── All wrappers already exist
```

### Risks

| Risk | Mitigation |
|------|-----------|
| UiButton doesn't support a Naive UI prop used in feature code | Check Ui* README for supported props; add prop to wrapper if ≥2 call sites need it |
| Wrong prop mapping changes visual appearance | Test visually after each group; buttons/inputs should look identical |
| Ui* wrapper has different slot names | Check README; use default slot or named slot per wrapper API |

## Tasks

### Phase 1: Create missing Ui\* wrappers

- **1a: Create UiCheckbox**
  - Acceptance: `UiCheckbox` wraps `NCheckbox` with `v-model:checked`, `label`, `disabled` props
  - Verify: `npm run typecheck`
  - Files: `components/ui/UiCheckbox.vue`

- **1b: Create UiInputNumber**
  - Acceptance: `UiInputNumber` wraps `NInputNumber` with `v-model`, `min`, `max`, `step`, `disabled` props
  - Verify: `npm run typecheck`
  - Files: `components/ui/UiInputNumber.vue`

- **1c: Create UiAlert**
  - Acceptance: `UiAlert` wraps `NAlert` with `type` (info/success/warning/error), `title`, `default` slot
  - Verify: `npm run typecheck`
  - Files: `components/ui/UiAlert.vue`

- **1d: Create UiSteps**
  - Acceptance: `UiSteps` wraps `NSteps` + `NStep` with `current`, `status` props; exposes `UiStep` sub-component
  - Verify: `npm run typecheck`
  - Files: `components/ui/UiSteps.vue`

- **1e: Create UiSpin**
  - Acceptance: `UiSpin` wraps `NSpin` with `show`, `size`, `description` props
  - Verify: `npm run typecheck`
  - Files: `components/ui/UiSpin.vue`

### Phase 2: NButton → UiButton (28 files)

Each file: replace `NButton` import with `UiButton`, map props (`type`→`variant`, `size`→`size`), use `@click` instead of `@click`.

Top files by count:
- `components/calibration/*/` — 8 files (all *Main.vue + *Settings.vue)
- `components/traversal/` — 6 files
- `components/motion/` — 4 files
- `components/layout/` — 3 files
- `components/device/` — 3 files
- `components/main/` — 2 files
- `components/feedback/` — 2 files (UiToastHost, UiConfirmDialog)
- `views/` — 2 files
- `components/traversal/visualization/` — 2 files

### Phase 3: Pairs replacement (parallel within group)

- **3a: NSelect → UiSelect** — 9 files
- **3b: NInput → UiInput** — 7 files  
- **3c: NCard → UiPanel** — 7 files
- **3d: NSwitch → UiToggle** — 2 files

### Phase 4: Remaining wrappers (parallel within group)

- **4a: NText → UiSectionHeader** — 9 files
- **4b: NTag → UiStatusBadge** — 5 files
- **4c: NModal → UiDialog** — 5 files

### Phase 5: Final verification

- **5a: Remove unused naive-ui imports** — grep for unused `naive-ui` imports after replacement
- **5b: Run full verification** — `npm run typecheck && npm run build && npm run test`

## Boundaries

- **Always:**
  - Run `npm run typecheck` after each file change
  - Preserve exact visual appearance — mapping must be 1:1
  - Check Ui* component README before using props/slots
- **Ask first:**
  - Adding new props to existing Ui* wrappers (only if ≥2 call sites need it)
  - Changing Ui* wrapper API (existing consumers must not break)
- **Never:**
  - Replace `NDataTable` — no Ui* wrapper exists and it's too complex
  - Replace `NForm`/`NFormItem` — tightly coupled to Naive UI form validation
  - Remove naive-ui from `package.json` dependencies

## Success Criteria

| Phase | Before | After |
|-------|--------|-------|
| Phase 1 | 5 missing wrappers | UiCheckbox, UiInputNumber, UiAlert, UiSteps, UiSpin exist |
| Phase 2 | 28 files import NButton | 28 files import UiButton instead |
| Phase 3 | 9+ files import NSelect/NInput/NCard/NSwitch | Same files import Ui* wrappers |
| Phase 4 | 9+ files import NText/NTag/NModal | Same files import Ui* wrappers |
| Final | naive-ui imported by 35 feature files | naive-ui imported by 0 feature files (only in components/ui/) |
