# Wind-DAQ UI Design Notes

Wind-DAQ uses an industrial SCADA-style desktop UI.

## Layout

- Header: 48px.
- Footer/status bar: 32px.
- Left rail: 56px.
- Context sidebar: 220px.
- Minimum target window: 1280x720.
- Desktop-first fixed layout; do not introduce mobile breakpoints unless explicitly requested.

## Visual Rules

- Dark theme first.
- Use CSS custom properties for colors, spacing, typography, radius, and motion.
- Header/footer may use glass effects.
- Data panels use solid backgrounds for readability.
- Numeric values use mono font and tabular numbers.
- Keep animation restrained and functional.

## Migration Source

Use the reference frontend under:

`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src`

Do not use deprecated business UI from `wails-backend/frontend/src`.
