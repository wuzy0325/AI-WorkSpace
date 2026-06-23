# Wind-DAQ Iconography

> Companion to `../../DESIGN.md`. Defines the icon library, sizing, semantic map, and how custom icons coexist with the library.

## Status (2026-06)

The project currently uses **two parallel icon systems**:

1. **Lucide** (`@lucide/vue`) — used in 15+ feature files. Used for buttons, status, indicators (`Play`, `Square`, `Activity`, `Plug`, `Zap`, `CheckCircle2`, `AlertTriangle`, etc.).
2. **Hand-drawn project icons** (`components/icons/Icon*.vue`) — 9 custom SVG components, used by the left rail and a few module entry points (`IconDashboard`, `IconMotion`, `IconCalibrationFiveHole`, `IconCalibrationThreeHole`, `IconCalibrationTotalPressure`, `IconCalibrationTotalTemperature`, `IconTraversal`, `IconLog`, `IconStorage`).

The left rail itself also has **two-letter abbreviation strings** (`IO / AX / CP / TR / LG`) as a fallback path in `AppRailNav.vue:43–48`. These look like placeholder text and should be removed once the icon path is fully covered.

### Findings

- **Inconsistent rail semantics.** `MainDashboardView.vue:59–67` registers rail items with `icon: 'IO' | 'CP' | 'TR' | 'LG' | 'AX'` (strings), and `AppRailNav` does a switch on those strings to resolve `Icon*` components. This is a brittle indirection — adding a new rail item requires editing both files.
- **Library mixed with custom for the same domain.** Calibration entry uses `IconCalibrationFiveHole` (custom) in the rail but the calibration screen header uses Lucide's `FlaskConical`. Same concept, two visuals.
- **Custom icons are inline SVG, not part of a sprite or library.** Each is ~10 lines of Vue + raw `<path>` data. They were created when Lucide didn't have a good match for "five-hole probe", "traversal grid", etc.

## Library Choice

**Lucide is the primary icon library.** Reasons:

- Already a dependency.
- Tree-shakeable named imports — only used icons ship.
- Outline style fits "industrial + modern" character.
- Active maintenance, comprehensive coverage of generic UI concepts.
- Consistent 24×24 grid, 2px stroke, currentColor-tinted.

**No second icon library** without a written decision in `docs/decisions/`. Specifically, do not add Heroicons, Tabler, Phosphor, or Material Icons.

## Custom Icons — When and How

Custom project icons (`components/icons/Icon*.vue`) exist only when **none of the following Lucide options fits the concept**:

- A direct match (e.g. `Play`, `Square` for transport controls).
- A reasonable metaphor (e.g. `Activity` for live signal, `Wifi` for connectivity).
- A composition of 2 Lucide icons in a small badge (rare; usually a single icon plus a state dot).

Domain-specific concepts that justify custom icons:

| Concept | Reason no library icon fits |
|---|---|
| Five-hole probe | Geometry-specific; no library has this. |
| Three-hole probe | Same. |
| Total pressure probe | Same. |
| Total temperature probe | Same. |
| Traversal grid / cross-section | Library options are too generic ("Grid"). |

Concepts that should **not** have custom icons (use Lucide instead):

| Was custom | Use this Lucide icon |
|---|---|
| `IconDashboard` | `LayoutDashboard` |
| `IconMotion` | `Move3d` or `Joystick` (pick one) |
| `IconLog` | `ScrollText` |
| `IconStorage` | `Database` or `HardDrive` |

Action: deprecate the four icons above (keep files for one release, redirect imports to Lucide). The four probe icons + traversal icon stay custom.

## Custom Icon Rules

When a new custom icon is needed:

- 24×24 viewBox, even if rendered smaller. Matches Lucide.
- `fill="none"`, `stroke="currentColor"`, `stroke-width="2"`, `stroke-linecap="round"`, `stroke-linejoin="round"`. Matches Lucide's default style.
- Inherit `currentColor` for stroke; do not embed colored fills.
- Single Vue file under `components/icons/`, named `IconXxx.vue`, default size prop `20`.
- No animation in the SVG itself. Wrap the component for animation if needed.

Template:

```vue
<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 20 })
</script>

<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24"
       fill="none" stroke="currentColor" stroke-width="2"
       stroke-linecap="round" stroke-linejoin="round">
    <!-- paths here -->
  </svg>
</template>
```

## Size Tiers

| Tier | Size | Usage |
|---|---|---|
| `xs` | 14px | Inside dense table cells, sparkline badges, micro-tags |
| `sm` | 16px | Inside buttons (next to text label), inline meta info |
| `md` | 20px | Default. Rail icons, header actions, status indicators |
| `lg` | 24px | Large empty-state illustrations, modal headers |
| `xl` | 32px | Hero / onboarding only — rare |

Use multiples of 2 (Lucide grid). Avoid 18, 22, 26 sizes.

## Semantic Map (Authoritative)

This table is the source of truth. **When code needs an icon for the listed concept, use the listed icon.** Drift here causes "same thing, different glyph" bugs.

### Navigation (left rail)

| Concept | Icon | Source |
|---|---|---|
| Dashboard / home | `LayoutDashboard` | Lucide |
| Calibration | `FlaskConical` | Lucide |
| Traversal | `IconTraversal` | Custom |
| Logs | `ScrollText` | Lucide |
| Motion controller (external window) | `Move3d` | Lucide |
| Settings (global) | `Settings` | Lucide |

### Transport controls (acquisition, recording, traversal runs)

| Concept | Icon |
|---|---|
| Start / play | `Play` |
| Stop | `Square` |
| Pause | `Pause` |
| Resume | `Play` (same as start) |
| Record (idle) | `Circle` |
| Recording (active) | `Circle` with `--accent-danger` fill |
| Step forward | `StepForward` |
| Reset | `RotateCcw` |

### Status / state

| Concept | Icon | Color |
|---|---|---|
| Connected / OK | `CheckCircle2` | `--accent-success` |
| Connecting | `Loader2` (spinning) | `--accent-info` |
| Disconnected / offline | `WifiOff` | `--text-muted` |
| Warning | `AlertTriangle` | `--accent-warning` |
| Error | `XCircle` | `--accent-danger` |
| Info / hint | `Info` | `--accent-info` |
| Live activity | `Activity` | `--accent-primary` |
| Network / connectivity | `Wifi` | varies |

### Devices and channels

| Concept | Icon |
|---|---|
| Single device / port | `Plug` |
| Power / energized | `Zap` |
| Channel group / grid | `LayoutGrid` |
| Single channel | (no icon — use channel color dot) |
| Pressure probe (generic) | `Gauge` |
| Five-hole probe | `IconCalibrationFiveHole` (custom) |
| Three-hole probe | `IconCalibrationThreeHole` (custom) |
| Total pressure probe | `IconCalibrationTotalPressure` (custom) |
| Total temperature probe | `IconCalibrationTotalTemperature` (custom) |
| Chart / waveform | `LineChart` |
| Heatmap / matrix view | `Grid3x3` |
| Sparkline | `Activity` |

### Actions

| Concept | Icon |
|---|---|
| Edit | `Pencil` |
| Delete | `Trash2` |
| Add | `Plus` |
| Remove | `Minus` |
| Search | `Search` |
| Filter | `Filter` |
| Sort | `ArrowUpDown` |
| Refresh | `RefreshCw` |
| Save | `Save` |
| Export / download | `Download` |
| Import / upload | `Upload` |
| Copy | `Copy` |
| Settings (panel-local) | `Settings2` |
| Open external | `ExternalLink` |
| Close | `X` |
| Confirm | `Check` |
| Cancel | `X` |
| Expand | `ChevronDown` / `ChevronRight` |
| Show | `Eye` |
| Hide | `EyeOff` |

### Time / measurement

| Concept | Icon |
|---|---|
| Duration | `Timer` |
| Timestamp / clock | `Clock` |
| Calendar / date | `Calendar` |
| Sample rate | `Activity` |
| Threshold | `Gauge` |

## Color Rules

- Default: `currentColor` (inherits from text color of the parent).
- Status icons use semantic accent tokens (`--accent-success` etc.) — declare in component CSS, do not pass color as a prop.
- Icons on colored fill backgrounds (e.g. a Primary button) inherit the foreground text color of that background — `--color-brand-foreground` for brand buttons, `--text-primary` elsewhere.
- Icons inside disabled controls get `opacity: 0.5`, not a different color.
- Never apply gradients or filters to icons. Crisp single-color icons match the industrial character.

## Stroke and Fill Style

- **Outline only** (Lucide default). Do not introduce filled glyph styles in the same area as outline icons — mixing reads as inconsistency.
- Two-tone icons are forbidden. A status indicator that needs two colors should be **icon + colored dot**, not a two-color icon.
- Filled circles in `Circle`-style icons for "recording / live" are allowed because they are functional, not decorative.

## Icon + Text

When an icon precedes text in a button or label:

- 4–6px gap between icon and text (use `--space-1` or `--space-1` + `--space-0_5`).
- Vertical alignment: baseline-aligned, not center-aligned, for inline body copy. Center-aligned for buttons.
- Icon size matches the text's cap height roughly: 16px icon for 14px body text, 14px icon for 12px small text.
- Never use icon + text where the icon alone would be ambiguous and the text already says everything. Pick one.

## Tooltip Rules for Icon-Only Buttons

Every icon-only button **must** have:

- An accessible label via `aria-label` or `title`.
- A tooltip (using `UiTooltip` or Naive's `n-tooltip`) on hover/focus with the same text.
- Keyboard reachability — `tabindex="0"` or be a native `<button>`.

Failure to provide a label is the most common a11y bug in industrial UIs; do not skip.

## Action Plan

1. **Remove the abbreviation fallback in the rail.** Replace `icon: 'IO' | 'CP' | ...` strings in `MainDashboardView.vue` with direct component references (lucide or custom). Delete the `getIconComponent` switch in `AppRailNav.vue`.
2. **Deprecate the four redundant custom icons** (`IconDashboard`, `IconMotion`, `IconLog`, `IconStorage`) — redirect imports to Lucide, then delete files after one release cycle.
3. **Keep the five probe / traversal custom icons.** No library equivalent.
4. **Audit existing call sites against the semantic map.** Where code uses, e.g. `LineChart` for "live signal", switch to `Activity` per the map.
5. **Add a doc comment** to `components/icons/index.ts` (create if missing) re-exporting custom icons for discoverability.

## Anti-Patterns

- Do not invent a new icon when Lucide has a close match.
- Do not use the same icon for two different concepts within one screen.
- Do not animate icons except for `Loader2` (intentional spin) and recording-state pulse (if explicitly approved).
- Do not embed icons as `background-image` — always use the Vue component for currentColor inheritance.
- Do not use emoji as icons.
- Do not mix outline and filled glyph families.

## Open Decisions

- [ ] Final choice between `Move3d` and `Joystick` for motion controller rail icon.
- [ ] Whether to keep the existing 4 redundant custom icons in repo for one release as deprecated, or delete immediately.
- [ ] Whether to add a Pencil-Storybook-equivalent for icon previews, or stay with code search.
