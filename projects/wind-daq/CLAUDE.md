# Wind-DAQ Claude Rules

Single source of truth: `../../CLAUDE.md` and `../../AGENTS.md`.

## Project Addendum

### UI Design

All frontend UI work **must** follow the design specification in `DESIGN.md` (same directory). This includes:

- **Layout**: AppShell dimensions (Header 48px, Footer 32px, Rail 56px, Sidebar 220px), fixed desktop layout (no responsive breakpoints)
- **Colors**: Use CSS custom properties from `DESIGN.md` §3 — never hardcode hex values
- **Typography**: Data values → `--font-mono` + tabular-nums, labels → `--font-sans`. Follow the dashboard data type scale
- **Glassmorphism**: Only on Header/Footer. Panels use solid `var(--bg-panel)` backgrounds
- **Waveform**: Follow `DESIGN.md` §5 for grid, triggers, cursors, zoom/pan, channel modes
- **Components**: Match UiButton/UiPanel/UiMetricCard specs exactly — variant colors, sizes, border-radius, transitions
- **Channel colors**: 8-color core scheme, 8-color extended. Never invent new channel colors
- **Error states**: Follow `DESIGN.md` §8 for device status, empty states, feedback levels
- **Tech stack**: Vue 3 + Vite + Pinia + Vue Router + Tailwind CSS + ECharts. No Electron

When generating or modifying any Vue component, CSS, or layout: read `DESIGN.md` first and conform to its tokens and patterns.
