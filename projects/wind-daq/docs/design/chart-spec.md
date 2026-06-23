# Wind-DAQ Chart & Waveform Spec

> Companion to `../../DESIGN.md`. Defines how all charts (live waveforms, calibration plots, traversal heatmaps/cross-sections, history) look and behave. Token-only — no hardcoded hex in feature code.

## Status (2026-06)

Charts in `wind-daq` use **ECharts 6** (`echarts`, `vue-echarts`) plus custom canvas where ECharts is too heavy. This is the chosen library; do not introduce a second charting toolkit without a written decision in `docs/decisions/`.

## Findings — what to fix as part of adopting this spec

These are real inconsistencies discovered in the current codebase. Each fix happens when the relevant screen is touched, not in a single rewrite.

1. **Channel color drift.** `tokens/color.css` defines 8 channel colors (`--color-channel-1..8`), but `RealtimeChart.vue:22` and `DeviceDetailPanel.vue:20` each declare their own `CHANNEL_COLORS = ['#3b82f6', '#10b981', ...]` — a different palette from the tokens, duplicated. Action: delete the local arrays and read from tokens.
2. **Hardcoded chart chrome colors.** `RealtimeChart.vue:65/69/70` hardcodes axis label color `#64748b` and grid line `rgba(255,255,255,0.06)`. The grid line is invisible in light theme. Action: route through chart tokens (defined below).
3. **No light-theme palette for channel colors.** `--color-channel-4` (`#d8b84c`, dark yellow) and `--color-channel-5` (`#f59e0b`, orange) are unreadable on white in the current `tokens/color.css`. Action: define light-theme overrides in `themes/light.css`.
4. **Axis tokens exist but are spatial axes (X/Y/Z/U for traversal), not chart axes.** `--axis-x` etc. refer to the 4-axis motion / traversal coordinate system. Chart axes (time, value, depth) currently have no tokens. Action: introduce a distinct `--chart-*` token family (below).

## Token Plan

Add a new token group `chart-*` to `tokens/color.css` (and overrides in `themes/light.css`). Keep `--axis-*` reserved for spatial axes (motion + traversal coordinates) — do **not** repurpose them.

```css
/* Charts — both themes define these */
--chart-grid-line       /* primary grid */
--chart-grid-line-faint /* secondary/half grid */
--chart-axis-text       /* tick labels */
--chart-axis-line       /* axis baseline */
--chart-crosshair       /* tooltip crosshair */
--chart-cursor          /* user-placed cursor / marker line */
--chart-selection-fill  /* zoom/brush selection rectangle */
--chart-selection-stroke
--chart-bg              /* chart plot background, equals --bg-panel by default */

/* Threshold and warning bands */
--chart-band-warning    /* alpha-filled background band for warning range */
--chart-band-danger     /* alpha-filled background band for danger range */
--chart-out-of-range    /* point/segment color when value exceeds spec */
```

Recommended values (TBD with designer; these are AI-proposed baselines that meet WCAG AA against the relevant background):

| Token | Light (`#ffffff` panel) | Dark (`#172338` panel) |
|---|---|---|
| `--chart-grid-line` | `#e2e8f0` | `rgba(255,255,255,0.08)` |
| `--chart-grid-line-faint` | `#f1f5f9` | `rgba(255,255,255,0.04)` |
| `--chart-axis-text` | `#475569` | `#94a3b8` |
| `--chart-axis-line` | `#cbd5e1` | `#475569` |
| `--chart-crosshair` | `color-mix(in srgb, #0f172a 40%, transparent)` | `color-mix(in srgb, #e2e8f0 35%, transparent)` |
| `--chart-cursor` | `var(--accent-info)` | `var(--accent-info)` |
| `--chart-band-warning` | `color-mix(in srgb, var(--accent-warning) 12%, transparent)` | `color-mix(in srgb, var(--accent-warning) 18%, transparent)` |
| `--chart-band-danger` | `color-mix(in srgb, var(--accent-danger) 12%, transparent)` | `color-mix(in srgb, var(--accent-danger) 18%, transparent)` |
| `--chart-out-of-range` | `var(--accent-danger)` | `var(--accent-danger)` |

## Channel Color System

Channel colors identify **a channel index**, not a physical quantity. Two waveforms on the same chart with the same channel index must use the same color regardless of which device, screen, or session displays them.

### Base palette: 8 colors

Defined in `tokens/color.css` `:root` (light) and `themes/dark.css` (dark). Current dark values are kept; light values must be re-tuned (Finding #3).

Selection criteria for the 8-color palette:

- WCAG AA contrast against the data-panel background in each theme
- Distinct in hue, brightness, **and** for the most common color-vision deficiencies (deuteranopia, protanopia)
- No two adjacent indices appear similar at 1px line width
- Reproducible in print / screenshot export

Current dark palette (kept):
```
ch-1 #2f88ff (blue)        ch-5 #f59e0b (orange)
ch-2 #29b6b0 (teal)        ch-6 #ef7d32 (deep orange)
ch-3 #4c7bd9 (indigo)      ch-7 #c96dd8 (purple)
ch-4 #d8b84c (gold)        ch-8 #37c995 (green)
```

Proposed light palette (TBD — must verify against white, then sign off):
```
ch-1 #1d4ed8 (blue)        ch-5 #c2410c (orange)
ch-2 #0d9488 (teal)        ch-6 #b45309 (deep amber)
ch-3 #3730a3 (indigo)      ch-7 #9333ea (purple)
ch-4 #a16207 (gold)        ch-8 #15803d (green)
```

### Beyond 8 channels

Hardware reports up to 64 channels per device. The 8-color palette must extend without inventing arbitrary new hues.

Strategy: **8 hues × 3 brightness tiers = 24 distinguishable**, then **8 hues × 3 tiers × dashed line = 48**, then **8 hues × 3 tiers × dotted line = 72**. Use line style, not new colors, to extend.

Implementation rule:

```ts
function channelStyle(channelIndex: number) {
  const hue = channelIndex % 8                    // 0..7
  const tier = Math.floor(channelIndex / 8) % 3   // 0..2 → normal / +12% lighter / -12% darker
  const dash = Math.floor(channelIndex / 24) % 3  // 0..2 → solid / dashed / dotted
  return { hue, tier, dash }
}
```

When a screen only displays a handful of channels (typical), just take colors in order — operators see clean primary hues.

### Channel groups

When channels are grouped (e.g. "pressure probes 1–8" + "thermocouples 1–8"), prefer **one color family per group** rather than 16 distinct colors. The group context is established by the chart title and legend; users do not need to globally distinguish T-1 from P-1 across screens.

## Chart Anatomy

A Wind-DAQ chart has at most five layered regions, drawn back-to-front:

```
┌──────────────────────────────────────────┐
│ Title (optional, h6 weight, --text-secondary)
│ ┌──────────────────────────────────────┐ │
│ │ Legend (compact, top-right, --text-secondary)
│ ├──────────────────────────────────────┤ │
│ │ ░░░░░ warning band ░░░░░             │ │  ← --chart-band-warning
│ │  ────────── series ──────────        │ │  ← channel color
│ │  ─ ─ ─ ─ ─ series 2 ─ ─ ─ ─          │ │
│ │ │ grid                                │ │  ← --chart-grid-line
│ ├──┴────────────────────────────────────┤ │
│  axis ticks                              │  ← --chart-axis-text
└──────────────────────────────────────────┘
```

- **Plot background**: `var(--chart-bg)` (defaults to `--bg-panel`). Solid. Never glass.
- **Grid**: 1px lines in `--chart-grid-line`. Optional half-grid in `--chart-grid-line-faint`. Subdivide on Y when value range > 100; subdivide on X when time span > 60s.
- **Series**: 1.5px solid line for primary channel(s); 1px for secondary; dashed/dotted per extension rule. Area fills only when there is a single series and the fill conveys meaning (e.g. integrated quantity). Avoid stacked gradient fills on multi-channel plots.
- **Axis**: 11–12px label, mono font for numbers (`--font-family-mono`), tabular nums. No axis line shadow.
- **Legend**: compact, top-right, color square + label. Hide when there is only one series.
- **Title**: optional, only when the chart sits alone (e.g. in a dialog). Inline charts inside a panel inherit the panel header.

## Live Waveform Specifics

Live waveform = continuously updating real-time chart. Used on the dashboard.

- **Rolling window**: prefer fixed time window (e.g. last 30s) over fixed point count. Time-based windows survive sample-rate changes.
- **No animation on data update.** New samples appear in place. Animating data points adds visual noise and obscures real motion in the signal.
- **No tooltip animation.** Cursor snaps; no easing.
- **Decimation**: when point count exceeds 2× pixel width, decimate to LTTB (largest-triangle-three-buckets). ECharts has `sampling: 'lttb'` — use it.
- **Pause indicator**: when stream is paused, overlay a subtle muted text "PAUSED" in the plot center and dim the most recent series segment. Do not freeze the time axis silently.
- **Reconnect state**: when SSE/event stream is reconnecting, show a small inline reconnect badge (top-right of chart, not a global toast).

## Out-of-Range / Threshold Rules

When a channel value crosses a configured warning or danger threshold:

1. The data panel showing the **numeric** readout gets the corresponding state color on the value and an accent border (this is already partially implemented in `DeviceOverviewPanel`).
2. The **chart** segment for that channel switches to `--chart-out-of-range` color for the out-of-range samples only. The rest of the trace keeps the channel color.
3. Optionally, a horizontal threshold band is drawn using `--chart-band-warning` / `--chart-band-danger` if the user has enabled "show thresholds".

Do not flash, blink, or animate the out-of-range visual.

## Cursors, Selection, Tooltips

- **Hover crosshair**: horizontal + vertical 1px line at cursor, `--chart-crosshair` color.
- **Tooltip**: panel with `--bg-panel-strong`, 1px `--border-default`, 8px radius, panel shadow. Mono font for numbers, regular for labels. Shows: timestamp + each visible channel's value + unit. Maximum 8 rows; scroll if more.
- **User cursor / marker**: a placed reference line uses `--chart-cursor` (info accent) and persists until removed. Show label "T = 1.234s" near the line.
- **Selection rectangle** (zoom or brush): `--chart-selection-fill` (alpha 12%) with `--chart-selection-stroke` (alpha 40%) border.
- **Pan/zoom hints**: appear bottom-right of chart, secondary text, only when the chart is hovered.

## Units, Numbers, Formatting

- All numeric tick labels and tooltip values use mono font + `tabular-nums`.
- Unit labels appear adjacent to the value (`1.234 kPa`) with a thin space separator. The unit is `--text-muted`.
- Decimal precision is per-channel metadata, defaulting to **3 decimals for pressure**, **2 for temperature**, **0 for counts**, **3 for voltage**. Surface this from device metadata, do not hardcode in chart code.
- Engineering notation (`1.23e3`) only when |value| ≥ 10⁴ or |value| ≤ 10⁻³.
- Always show the **sign** for signed channels (force `+` for positives). For unsigned, never show the `+`.

## Calibration / Traversal Plots

These are **non-realtime** charts and have different rules:

- Animation on initial render is allowed (one easing-out, 240ms, value bars rising / curves drawing). Subsequent updates are not animated.
- Symbols on data points are allowed and recommended for calibration scatter plots (operator needs to identify individual measurements).
- Reference curves (theoretical / target) use dashed lines in `--text-muted` color.
- Residual / error panels use a divergent palette — positive in `--accent-info`, negative in `--accent-warning`. **Not the standard channel palette.**

## Heatmaps (Traversal Cross-Section)

- Use a single hue ramp, not a rainbow. Rainbow palettes are perceptually misleading.
- Default ramp: **viridis** (perceptually uniform, color-blind safe). Token name: `--chart-ramp-default` (TBD: ship 3–4 named ramps in tokens — viridis, magma, cividis, diverging).
- Out-of-range cells use `--chart-out-of-range` overlay, not a ramp endpoint.
- Color bar must be vertical, right-aligned, with min/max tick labels in mono.

## Screenshot / Export Rules

Charts must export to PNG/SVG identically to what is on screen, including theme. Export code path:

- Snapshot at **2× device pixel ratio** for raster export.
- Use **the same theme** as on screen — do not auto-switch to white for export. Operators occasionally need dark-theme screenshots for reports too.
- Include channel name + unit in the exported legend even if the on-screen legend is hidden.

## Anti-Patterns

- Do not animate live data updates.
- Do not use rainbow color ramps for heatmaps.
- Do not use the spatial axis tokens (`--axis-x..u`) for chart axes.
- Do not write hex colors in chart component code — read from tokens.
- Do not use `area`-fill on multi-channel waveforms; it makes everything muddy.
- Do not show a glow/shadow on series lines. Crisp 1.5px lines beat soft 3px glow.
- Do not put the legend inside the plot area when the chart is small (< 320px wide); move it above or hide it and rely on tooltip.

## Open Decisions

These need a person, not an AI, to decide. Documented here so they are not lost:

- [ ] Final light-theme channel palette (8 hex values). The values above are AI-proposed defaults.
- [ ] Default ramp set for heatmaps (viridis only, or viridis + cividis + diverging).
- [ ] Whether `RealtimeChart` should use ECharts at all, or a smaller canvas implementation for the dashboard's many tiny per-channel sparklines. ECharts has overhead per instance.
