# Wind-DAQ UI Design

Wind-DAQ is an industrial DAQ desktop tool for wind tunnel and lab measurement teams. Its UI target is **industrial instrumentation with a modern feel**: dense readable telemetry, calm chrome, restrained motion. Visually it should feel like a credible measurement instrument, not a marketing landing page and not a 1990s SCADA reskin.

This file is the product-level UI target. Token, primitive, and rule documents derive from it.

## Visual Character

The UI should read as:

- **Instrument-grade** — operators trust the numbers. Data panels are solid, dense, and read like a multimeter. Telemetry uses mono font with tabular numbers. Channel readouts get visual priority over chrome.
- **Modern** — clean hierarchy, generous-but-purposeful spacing in chrome, current type and iconography. No skeuomorphic bezels, no neon glow, no fake CRT scanlines.
- **Calm** — animation is restrained and functional (transitions, feedback, state changes). No decorative motion. No parallax. No marketing micro-interactions.
- **Hierarchical** — `surface-0 → surface-3` layers must be visibly separated. Chrome (header, rail, sidebar, footer) reads as a different layer from the data stage.

If a design choice would not fit a piece of lab instrument software a measurement engineer keeps open for 8 hours, reject it.

## Window and Layout

- **Minimum window size: 1440×900.** This is a hard floor for the main application window. Below this the layout is allowed to break.
- **Default window size: 1600×900** (main app). The Wails config sets these.
- **Motion controller standalone window** (`--motion-only`) is exempt: it stays at 1440×860 / minimum 1200×720 because it is a single-purpose narrow window.
- **Desktop-first, fixed layout.** No responsive breakpoints, no mobile collapse. The application targets engineering workstations.

Chrome dimensions (current targets; change here and in `tokens/layout.css` together):

| Region | Size |
|---|---|
| Header (top bar) | 56px |
| Footer / status bar | 72px token target; visual height controlled by `MainBottomBar.vue` |
| Left rail (icon nav) | 64px |
| Context sidebar (device list) | 244px |

These dimensions are the working baseline. Adjust them as the design matures, but adjust them in `tokens/layout.css` and update this table — do not hardcode pixel widths in feature components.

## Theme

- **Light theme is the default.** First-run users see the light theme unless their OS reports a dark preference (`prefers-color-scheme: dark`) or they previously chose dark.
- **Dark theme is a fully supported alternative** — not a contrast-broken afterthought. Every screen and every token must be designed against both themes.
- Theme selection persists in `localStorage` and is exposed via the global settings dialog.
- Token files in `styles/tokens/*.css` define the light theme as the `:root` baseline. Dark overrides live in `styles/themes/dark.css` and are scoped by `[data-theme='dark']`.

Theme implementation notes for AI agents:

- When designing a new visual element, design the **light variant first**, then verify dark.
- Never write color literals in feature components. Use tokens.
- Channel colors (`--color-channel-1..8`), grid lines, crosshairs, and chart axes must be defined in both themes with checked contrast.
- The header/footer glass effect must be re-tuned per theme; a single backdrop-blur recipe does not work for both.

## Data Panel Rules

- Data panels use **solid backgrounds**. No glass, no transparency, no gradient washes. Operators read numbers off these.
- Numeric values use **mono font with `font-variant-numeric: tabular-nums`**.
- Unit labels are visually secondary to the value (smaller, muted color).
- Out-of-range / warning states are color-coded against tokenized thresholds, not hardcoded hex.
- A data panel must look correct in both themes without operator action.

## Chrome Rules

- Header and footer **may** use glass / backdrop-blur, but only when the underlying canvas has enough contrast for the chrome to read clearly. Re-tune per theme.
- The left rail and context sidebar use solid surfaces, not glass.
- Status indicators in the footer use tokenized state colors (`--state-success`, `--state-warning`, `--state-error`, `--state-info`).

## UI System

The UI system is layered. Newer code lives higher in the stack:

```
Feature components (views/, modules/)
        │
        ▼
Ui* primitives  (components/ui/*)        ← prefer these
        │
        ▼
Naive UI                                  ← low-level fallback
        │
        ▼
Design tokens   (styles/tokens/*)         ← single source of truth for color/space/type/radius/motion
```

Rules:

- Feature code prefers `Ui*` primitives. Use Naive UI directly only when no project primitive covers the case, and document why in the component header.
- Visual values (color, spacing, font-size, radius, duration) come from tokens. No hex literals, no raw `px` for spacing, in feature code.
- Inline `style="..."` is allowed only for runtime-computed values (e.g. waveform canvas sizing). Static visual values go in scoped CSS using tokens.
- See `apps/desktop-wails/frontend/src/components/ui/README.md` for the primitive catalog and `apps/desktop-wails/frontend/src/styles/tokens/README.md` for token usage.
- AI agents writing UI code must also follow `../../docs/runbooks/frontend-ai-rules.zh-CN.md` and `../../docs/runbooks/frontend-directory-rules.zh-CN.md` from the workspace root.

## Companion Design Specs

These cover topics too detailed for this file:

- `docs/design/chart-spec.md` — Chart & waveform visual spec, channel color extension, threshold visualization, anti-patterns.
- `docs/design/light-theme-palette.md` — Light theme audit, target palette, contrast compliance, action plan for filling gaps.
- `docs/design/iconography.md` — Icon library choice, semantic icon map, custom icon rules, sizing.

Additional specs to be added: state vocabulary, copy guidelines, motion guidelines, keyboard & a11y.

## Migration Notes

The old `Cursor DAQ` Electron project is no longer the visual target. It remains in the repository (under `docs/migration/`) only as a feature inventory — what features must exist, what operator workflows must be preserved. Visual layout, color choices, and component composition are no longer constrained by that project.

See `docs/migration/README.md` for the current role of migration docs and `docs/ui-design-audit.md` for the per-screen cleanup backlog.
