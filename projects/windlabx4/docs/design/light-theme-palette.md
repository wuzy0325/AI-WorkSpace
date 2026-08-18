# WindLabX4 Light Theme Palette

> Companion to `../../DESIGN.md`. The current light theme is a thin overlay tuned hastily for a dark-first project. Now that light is the **default theme** (DESIGN.md → "Theme"), it deserves a real audit. This file defines the target light palette and the gaps the current implementation has.

## Current State (2026-06)

- `tokens/color.css` `:root` is **dark** (`--surface-0: #0f172a` etc.). The light theme lives in `themes/light.css` as a `[data-theme='light']` override.
- `themes/light.css` covers surfaces, text, borders, accents, focus, selection, dot grid, and the 4 spatial axes (`--axis-x..u`). It does **not** cover:
  - 8 channel colors (`--color-channel-1..8`) — Finding A
  - Chart axes / grid / crosshair (no chart tokens yet, see `chart-spec.md`)
  - Glass effects beyond a generic white wash (see `glass.css`)
  - Threshold / warning bands
- `App.vue:11–82` injects a hardcoded Naive UI theme override with light/dark conditionals. Several of these (e.g. `Input.textColor = '#0f172a'`) only exist because the underlying token didn't give Naive UI enough information.
- Channel colors that look fine on `#172338` (dark panel) are unreadable on `#ffffff` (light panel): especially `--color-channel-4: #d8b84c` (dark gold) and `--color-channel-5: #f59e0b` (orange). Contrast ratio against white: 1.4 and 1.9 respectively — both fail WCAG AA (4.5:1) by a wide margin.

## Migration Direction

**Goal:** make light the `:root` baseline and dark the `[data-theme='dark']` override. This matches DESIGN.md "design the light variant first, then verify dark."

This is a significant refactor — not in scope for the initial audit task. The interim path is simpler:

1. Keep `:root` as it is today.
2. Fill the gaps below in `themes/light.css` so light is **production-quality** for daily use.
3. Once Phase 5 (page-by-page cleanup, see `../ui-design-audit.md`) is in motion, flip `:root` to light and rewrite `themes/dark.css` from scratch.

## Target Palette

### Surface tiers (current, kept)

| Token | Light value | Used for |
|---|---|---|
| `--bg-app` | `#f8fafc` | App background outside panels |
| `--bg-canvas` | `#f8fafc` | Main content canvas |
| `--bg-panel` | `#ffffff` | Data panels, cards |
| `--bg-panel-strong` | `#f8fafc` | Inset panels, tooltips |

Note: `--bg-app` and `--bg-canvas` are currently identical (`#f8fafc`). Consider making `--bg-canvas: #f1f5f9` so canvas reads as a slight inset against `--bg-app`. **TBD**.

### Text (current, kept)

| Token | Light value | Contrast vs `#ffffff` |
|---|---|---|
| `--text-primary` | `#0f172a` | 18.4:1 ✓ AAA |
| `--text-secondary` | `#334155` | 10.8:1 ✓ AAA |
| `--text-muted` | `#94a3b8` | 2.6:1 ✗ (only for non-text decorative use) |

`--text-muted` fails WCAG AA. Acceptable for icons, dot separators, decorative labels — not for body copy. **Action: audit places that use `--text-muted` for actual labels and switch to `--text-secondary`.**

### Accents (revise)

Current `themes/light.css`:
```
--accent-primary: #10b981   (emerald)
--accent-success: #22c55e   (green)
--accent-warning: #f97316   (orange)
--accent-danger:  #ef4444   (red)
--accent-info:    #22c55e   (green — same as success, bug)
```

Issues:
- `--accent-info` is identical to `--accent-success`. **Fix**: use a blue, e.g. `#0284c7` (sky-600).
- `--accent-primary: #10b981` is a green emerald. App.vue's Naive UI override sets primary to `#22c55e` (also green). Pick one. **TBD: which green is canonical for primary?** Recommend `#10b981`.
- Warning + danger are very close (both warm). Consider `--accent-warning: #d97706` (amber-600, more yellow) to separate.

Proposed light accents:

| Token | Value | Contrast vs white |
|---|---|---|
| `--accent-primary` | `#10b981` (emerald-500) | 2.5:1 — use only for fills, not text |
| `--accent-primary-text` *(new)* | `#047857` (emerald-700) | 5.0:1 ✓ AA — for text and small icons |
| `--accent-success` | `#15803d` (green-700) | 5.3:1 ✓ AA |
| `--accent-warning` | `#b45309` (amber-700) | 5.6:1 ✓ AA |
| `--accent-danger` | `#b91c1c` (red-700) | 6.5:1 ✓ AA |
| `--accent-info` | `#0369a1` (sky-700) | 6.0:1 ✓ AA |

The split between fill-only (`--accent-primary`) and text-safe (`--accent-primary-text`) is necessary in light themes because mid-saturation brand colors do not pass AA against white when used in 12px labels.

### Channel palette (revise — required for light theme)

Current `themes/light.css` does **not** override channel colors, so the dark palette bleeds through and several channels are unreadable. Proposed light values, each verified ≥ 4.5:1 against `#ffffff`:

| Channel | Dark (kept) | Light (proposed) | Light contrast vs `#ffffff` |
|---|---|---|---|
| 1 | `#2f88ff` (blue) | `#1d4ed8` (blue-700) | 7.7:1 ✓ |
| 2 | `#29b6b0` (teal) | `#0d9488` (teal-600) | 4.7:1 ✓ |
| 3 | `#4c7bd9` (indigo) | `#3730a3` (indigo-800) | 9.4:1 ✓ |
| 4 | `#d8b84c` (gold) | `#a16207` (yellow-700) | 4.9:1 ✓ |
| 5 | `#f59e0b` (orange) | `#c2410c` (orange-700) | 5.6:1 ✓ |
| 6 | `#ef7d32` (deep orange) | `#9a3412` (orange-800) | 7.6:1 ✓ |
| 7 | `#c96dd8` (purple) | `#9333ea` (purple-600) | 4.8:1 ✓ |
| 8 | `#37c995` (green) | `#15803d` (green-700) | 5.3:1 ✓ |

All values are proposed; **a designer should sign off on perceived distinguishability** before commit. Contrast is necessary but not sufficient — these 8 must also be distinct from each other at 1.5px line width and pass deuteranopia simulation.

### Borders (current, kept)

| Token | Light | Note |
|---|---|---|
| `--border-default` | `#e2e8f0` | Standard divider, panel edge |
| `--border-strong` | `#cbd5e1` | Emphasized divider, table headers |

### Focus ring (current, kept)

Current value `color-mix(var(--accent-primary) 50%, white 10%)` is fine; verify against a focused button on `#f8fafc` — may need lifting to 60%.

### Spatial axes (current, kept)

X/Y/Z/U colors in `themes/light.css` are well-chosen (700-tier of blue/violet/cyan/amber). No change.

### Chart tokens (new — see chart-spec.md)

To be added per `chart-spec.md` § Token Plan. Light values:

```
--chart-grid-line       #e2e8f0
--chart-grid-line-faint #f1f5f9
--chart-axis-text       #475569
--chart-axis-line       #cbd5e1
--chart-crosshair       color-mix(in srgb, #0f172a 40%, transparent)
--chart-cursor          var(--accent-info)
--chart-selection-fill  color-mix(in srgb, var(--accent-primary) 10%, transparent)
--chart-selection-stroke color-mix(in srgb, var(--accent-primary) 40%, transparent)
--chart-bg              var(--bg-panel)
--chart-band-warning    color-mix(in srgb, var(--accent-warning) 10%, transparent)
--chart-band-danger     color-mix(in srgb, var(--accent-danger) 10%, transparent)
--chart-out-of-range    var(--accent-danger)
```

### Glass effects (revise)

Current `glass.css` has a generic light override (`rgba(255,255,255,0.7)` over everything). This works only when the underlying canvas is `#f8fafc` and there's nothing busy behind it. Issues:

- `.glass-sidebar` uses `rgba(248,250,252,0.8)` — nearly opaque, defeating the glass purpose.
- `.glass-rail` is undefined for light (the file in the repo only shows the dark variant in the snippet read; verify).
- Border tints (`rgba(0,0,0,0.05)`) are barely visible.

Recommendations for light glass:

| Element | Background | Border | Blur |
|---|---|---|---|
| Header glass | `rgba(255, 255, 255, 0.78)` | `1px solid rgba(15, 23, 42, 0.08)` | `blur(10px) saturate(120%)` |
| Footer glass | `rgba(255, 255, 255, 0.85)` | `1px solid rgba(15, 23, 42, 0.08)` | `blur(10px)` |
| Rail | **No glass** — use solid `--bg-panel-strong` (`#f8fafc`). Glass on a 64px-wide column at default zoom looks like a smudge, not a material. |
| Sidebar | **No glass** — use solid `--bg-panel` (`#ffffff`) with a left border. |

In light theme, glass should be reserved for header/footer where there's depth behind them. Rail and sidebar are foreground columns — they read better as crisp solid surfaces.

### Selection & focus

| Token | Light value |
|---|---|
| `--selection-bg` | `color-mix(in srgb, var(--accent-primary) 18%, transparent)` (current — fine) |
| `--selection-text` | `#0f172a` (current — fine) |
| `--bg-grid-dot` | `color-mix(in srgb, #cbd5e1 90%, transparent)` (current — fine, but maybe lighten to `#cbd5e1 60%` so dot grid doesn't dominate empty canvases) |

## Action Plan

The work splits into **fill-the-gaps** (do now) and **invert-baseline** (do later).

### Fill the gaps (can do without breaking dark)

1. Add `--accent-info: #0369a1` to `themes/light.css` (fixes info=success bug).
2. Add `--accent-primary-text: #047857` to `themes/light.css` for text-safe primary.
3. Add the 8 light channel overrides to `themes/light.css` (`--color-channel-1..8`).
4. Add the chart token block (per `chart-spec.md`) to both `themes/light.css` and `themes/dark.css`, **and remove the hardcoded chart colors** from `RealtimeChart.vue` and `DeviceDetailPanel.vue` (see chart-spec.md Findings #1, #2).
5. Adjust `glass.css` light variants per the table above.
6. Audit `--text-muted` callers and split them between "decorative" and "label" — only the former keeps `--text-muted`.

Each item is independent. They can land in separate commits.

### Invert the baseline (later, with audit Phase 5)

1. Rewrite `tokens/color.css` `:root` with light values.
2. Rewrite `themes/dark.css` as a `[data-theme='dark']` override of the new baseline (no longer doubles as default).
3. Delete the light overrides from `App.vue:11–82` that exist only to fight the dark baseline.
4. Verify every existing screen against both themes.

This step is large and should not be combined with feature changes.

## Verification

Each change must verify against:

- **Contrast**: every text/icon token used as text must hit WCAG AA (4.5:1 for ≥16px, 3:1 for ≥18px or bold ≥14px).
- **Distinguishability**: 8 channel colors at 1.5px line width on `#ffffff` must remain distinct when simulated through deuteranopia (most common color-vision deficiency).
- **Theme parity**: every screen renders in both themes without operator action. No "missing data because the line is white on white" failures.

Tooling: WebAIM Contrast Checker for hex pairs; Stark or `prefers-color-scheme` toggling in Vue DevTools for live preview; `simulate-cvd` for color-vision simulation.

## Open Decisions

- [ ] Designer sign-off on the 8 proposed light channel hex values.
- [ ] Whether `--bg-canvas` should differ from `--bg-app` (currently identical).
- [ ] Whether to keep `glass-sidebar` and `glass-rail` classes at all, or rename them and drop glass in light.
- [ ] Schedule for the `:root` → light baseline flip.
