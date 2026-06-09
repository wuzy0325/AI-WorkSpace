# Wind-DAQ UI Design Notes

Wind-DAQ uses the industrial SCADA-style desktop UI from the pre-refactor Cursor DAQ project.

The UI target is parity with the reference project, not a redesign. Internal implementation should be cleaner than the reference, but user-facing layout, controls, and workflow should remain recognizable.

## Layout

- Layout dimensions are inherited from the active Cursor DAQ parity implementation and CSS tokens.
- Do not override dimensions here if they conflict with `docs/migration/ui-parity-plan.md` or the reference UI.
- Minimum target window: 1280x720.
- Desktop-first fixed layout; do not introduce mobile breakpoints unless explicitly requested.

Current parity token targets:

- Header: 56px.
- Footer/status bar: 72px token target, with component visual height controlled by `MainBottomBar.vue`.
- Left rail: 64px.
- Context sidebar: 244px.

## Visual Rules

- Dark theme first.
- Use CSS custom properties for colors, spacing, typography, radius, and motion.
- Header/footer may use glass effects.
- Data panels use solid backgrounds for readability.
- Numeric values use mono font and tabular numbers.
- Keep animation restrained and functional.

## UI System Rules

- Naive UI is the low-level widget toolkit.
- `apps/desktop-wails/frontend/src/components/ui/` is the Wind-DAQ project primitive layer.
- Feature code should prefer existing `Ui*` primitives before direct Naive UI usage.
- `apps/desktop-wails/frontend/src/styles/tokens/` is the visual token source of truth.
- New UI work must follow `../../docs/runbooks/frontend-ai-rules.zh-CN.md` from the workspace root.

Local execution docs:

- `apps/desktop-wails/frontend/src/components/ui/README.md` — component usage rules.
- `apps/desktop-wails/frontend/src/styles/tokens/README.md` — token usage rules.
- `docs/ui-design-audit.md` — current UI debt and migration order.

## Migration Source

Use the reference frontend under:

`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src`

Do not use deprecated business UI from `wails-backend/frontend/src`.

## Migration Docs

Use `docs/migration/README.md` as the single migration documentation entry point.

Use `docs/migration/ui-parity-plan.md` for AI implementation guidance on frontend UI parity.
