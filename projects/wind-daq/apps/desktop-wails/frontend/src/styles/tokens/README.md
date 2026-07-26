# Wind-DAQ Design Token Rules

> `styles/tokens/` is the visual source of truth for Wind-DAQ. UI work must use these tokens before adding local values.

## Token Files

- `color.css` owns semantic color, status color, surfaces, borders, focus, selection, channel colors, axis colors, and chart chrome (`--chart-*`).
- `spacing.css` owns spacing scale values such as `--space-1`, `--space-2`, `--space-3`, and `--space-4`, plus the `--density-*` semantic tokens for configuration surfaces (see DESIGN.md «Density Spec»).
- `typography.css` owns font families, font sizes, weights, line heights, and dashboard-specific type aliases.
- `radius.css` owns radius values.
- `motion.css` owns transition and animation timing values.
- `layout.css` owns app shell dimensions, panel gaps, and content padding.

Theme overrides live in `styles/themes/`. The application-level Naive UI mapping lives in `App.vue`.

### Chart tokens (`--chart-*`)

Used by live waveforms, calibration plots, and history charts. **Do not** reuse `--axis-*` for chart axes — `--axis-*` is reserved for motion/traversal spatial axes.

| Token | Use |
|---|---|
| `--chart-bg` | Plot background |
| `--chart-grid-line` / `--chart-grid-line-faint` | Grid |
| `--chart-axis-text` / `--chart-axis-line` | Tick labels and axis baseline |
| `--chart-crosshair` | Hover crosshair |
| `--chart-cursor` | User-placed marker |
| `--chart-selection-fill` / `--chart-selection-stroke` | Brush/zoom rect |
| `--chart-band-warning` / `--chart-band-danger` | Threshold bands |
| `--chart-out-of-range` | Out-of-range segment color |
| `--chart-readout-bg` | Readout bar under the plot (outside the series area) |

Specs: `docs/design/chart-spec.md`, `docs/design/monitor-workspace-spec.md`.

## Naming Rules

Use semantic names for tokens:

- `--bg-panel`
- `--text-primary`
- `--border-default`
- `--accent-primary`
- `--state-warning`
- `--layout-sidebar-width`
- `--type-dashboard-data`

Do not add tokens like:

- `--blue1`
- `--gray2`
- `--gap13`
- `--card-special-color`

## Usage Rules

Use tokens for:

- Backgrounds and surfaces.
- Text and muted text.
- Borders and dividers.
- Status colors.
- Focus rings.
- Spacing and gaps.
- Radius.
- Font size, weight, and mono numeric display.
- Header, rail, sidebar, footer, panel, and content dimensions.

Avoid new raw values in components:

- Hex colors.
- `rgba()` colors that duplicate status or surface colors.
- Repeated `px` or `rem` values that match the spacing scale.
- Ad hoc `z-index` values.
- Ad hoc transition durations.

One-off dynamic geometry is allowed when it represents data or measured layout, such as chart coordinates, cursor tooltip position, or canvas/SVG calculations.

## Industrial UI Defaults

Wind-DAQ is a desktop-first industrial DAQ UI. Token usage should preserve:

- Dark theme first.
- Dense but readable panels.
- Strong device, acquisition, and recording status visibility.
- Tabular numeric display for measured values.
- Low decoration noise.
- Restrained motion.

Do not introduce marketing-style gradients, oversized hero typography, or low-density landing-page spacing unless the user explicitly requests a non-tooling surface.

## Adding Tokens

Add a token only when:

- The value appears in multiple places.
- The value expresses a reusable semantic role.
- A component needs a stable contract for theme support.

Do not add a token for a single component-specific tweak. Prefer a component class using existing tokens.

## Migration Checklist

When touching an existing component:

- Replace repeated raw colors with `color.css` tokens.
- Replace repeated spacing values with `spacing.css` tokens.
- Replace repeated font sizes and weights with `typography.css` tokens.
- Replace hard-coded app dimensions with `layout.css` tokens.
- Keep parity-critical dimensions if `DESIGN.md` or migration docs require them.
