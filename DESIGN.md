---
name: Wind-DAQ
description: Precision data-acquisition desktop tool suite for wind-tunnel engineers — dark-themed instrument panels, real-time charts, and hardware control widgets.
colors:
  accent-primary: "#22c55e"
  accent-primary-deep: "#16a34a"
  accent-success: "#22c55e"
  accent-warning: "#f59e0b"
  accent-danger: "#ef5b47"
  accent-info: "#38bdf8"
  surface-app: "#0f172a"
  surface-canvas: "#111c31"
  surface-panel: "#172338"
  surface-panel-strong: "#1e293b"
  text-primary: "#e2e8f0"
  text-secondary: "#cbd5e1"
  text-muted: "#94a3b8"
  border-default: "#334155"
  border-strong: "#475569"
  channel-1: "#2f88ff"
  channel-2: "#29b6b0"
  channel-3: "#4c7bd9"
  channel-4: "#d8b84c"
  channel-5: "#f59e0b"
  channel-6: "#ef7d32"
  channel-7: "#c96dd8"
  channel-8: "#37c995"
  axis-x: "#3b82f6"
  axis-y: "#8b5cf6"
  axis-z: "#06b6d4"
  axis-u: "#f59e0b"
typography:
  body:
    fontFamily: "'Microsoft YaHei UI', 'PingFang SC', 'Segoe UI', sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 400
    lineHeight: 1.5
  data:
    fontFamily: "'JetBrains Mono', 'Cascadia Code', 'SFMono-Regular', monospace"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.5
  title:
    fontFamily: "'Microsoft YaHei UI', 'PingFang SC', 'Segoe UI', sans-serif"
    fontSize: "1.125rem"
    fontWeight: 700
    lineHeight: 1.25
  section-label:
    fontFamily: "'Microsoft YaHei UI', 'PingFang SC', 'Segoe UI', sans-serif"
    fontSize: "0.75rem"
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: "0.08em"
  dashboard-data:
    fontFamily: "'JetBrains Mono', 'Cascadia Code', sans-serif"
    fontSize: "1.5rem"
    fontWeight: 800
    lineHeight: 1.25
rounded:
  xs: "2px"
  sm: "3px"
  md: "4px"
  lg: "4px"
  xl: "4px"
  pill: "999px"
spacing:
  "0": "0"
  "1": "4px"
  "2": "8px"
  "3": "12px"
  "4": "16px"
  "5": "20px"
  "6": "24px"
  "8": "32px"
  "10": "40px"
  "12": "48px"
  "16": "64px"
components:
  button-primary:
    backgroundColor: "{colors.accent-success}"
    textColor: "#f8fbff"
    rounded: "{rounded.md}"
    padding: "8px 14px"
    height: "34px"
  button-secondary:
    backgroundColor: "rgba(255, 255, 255, 0.06)"
    textColor: "{colors.text-secondary}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
    height: "34px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.text-muted}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
    height: "34px"
  input-field:
    backgroundColor: "rgba(0, 0, 0, 0.2)"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
  panel:
    backgroundColor: "{colors.surface-panel}"
    rounded: "12px"
    padding: "16px 20px"
---

# Design System: Wind-DAQ

## 1. Overview

**Creative North Star: "The Precision Instrument Panel"**

A dark, focused environment where data is the protagonist. Every pixel earns its place by serving the engineer's workflow — nothing decorative, nothing distracting. The design system borrows from laboratory instrumentation: sharp contrast, unambiguous state signals, predictable layout. Glass-translucent chrome elements (rail, header, footer) frame the workspace without competing with it; the canvas belongs to the data.

The system is explicitly **not** a SaaS dashboard, not a consumer app, not a marketing surface. It rejects gradient text, decorative blur, floating cards, and hero-metric templates. The accent color is a signal, not a decoration — it marks actionable elements and live state. The monospace data font communicates precision; the sans-serif UI font communicates clarity. Together they form a single, coherent voice: this is a tool for engineers who need to trust what they see.

**Key Characteristics:**
- Deep navy base (`#0f172a`) anchors all surfaces; nothing on screen exceeds chroma level of a status signal
- Glass-translucent chrome (rail, header, sidebar, footer) uses backdrop-blur for depth without opacity
- Status colors (success/warning/danger) drive the only moments of saturated color — their presence IS the message
- Monospace data at heavy weight (800) reads like an instrument readout
- Rounded corners at a tight 4px radius keep edges crisp and industrial
- Grid-dot background at 24px spacing provides subtle spatial reference without visual clutter

## 2. Colors

The palette is anchored in function, not fashion. Every color signals something — state, hierarchy, or domain mapping. The primary accent is green (`#22c55e`), consistent across both dark and light modes. Cyan (`#38bdf8`) serves as the info-state signal and focus-ring color, distinct from the actionable green.

### Primary Accent
- **Emerald Signal** (`#22c55e`): Primary interactive elements — buttons, toggle-on, selected states, active indicators. Used at ≤10% surface coverage by the One Voice Rule.
- **Deep Emerald** (`#16a34a`): Stronger pressed/hover variant. Used sparingly on button active states.

### Status Signals
- **Success Green** (`#22c55e`): Connected state, acquisition active, toggle-on, primary CTA button. This is the primary accent — it means "go" and "live."
- **Warning Amber** (`#f59e0b`): Degraded state, caution, confirmation-required. Never used as decoration.
- **Danger Red** (`#ef5b47`): Error state, emergency stop, disconnect warning. Always paired with contextual text; never stands alone.
- **Cyan Info** (`#38bdf8`): Informational indicators, focus rings, secondary badges. Used for states that are informational rather than actionable.

### Surfaces
- **App Base** (`#0f172a`): Deepest background — the void behind the instrument.
- **Canvas** (`#111c31`): Content area behind panels, subtle lift from app base.
- **Panel** (`#172338`): Card, panel, and container surfaces. Distinct from canvas.
- **Panel Strong** (`#1e293b`): Elevated variant for active/hovered panels or nested sections.

### Text
- **Primary** (`#e2e8f0`): Body copy, headings, labels. Must achieve ≥4.5:1 on panel surfaces.
- **Secondary** (`#cbd5e1`): Supporting text, metadata, helper labels.
- **Muted** (`#94a3b8`): Placeholder text, disabled labels, tertiary info.

### Borders & Dividers
- **Default** (`#334155`): Standard panel and input borders. Visible but recessive.
- **Strong** (`#475569`): Active input borders, focus delimiter, emphasized separators.

### Domain Colors
- **Channel 1–8**: Eight-channel DAQ palette, mapped to physical pressure/temperature channels. Functionally stable; never reassigned across instruments.
- **Axis X/Y/Z/U** (`#3b82f6` / `#8b5cf6` / `#06b6d4` / `#f59e0b`): Motion controller axis mapping. Blue→X, Purple→Y, Cyan→Z, Amber→U. These are domain constants, not theme variables.

### Named Rules
**The One Voice Rule.** The primary accent (green) is used on ≤10% of any given screen. It signals focus and state. When everything is highlighted, nothing is.

**The Status-Is-The-Color Rule.** Saturated color on screen always signals device or system state. Never use accent/success/warning/danger colors for decoration or branding alone.

**The Light-Mode Legacy Rule.** Light mode (`data-theme="light"`) swaps the accent from cyan to green and inverts the surface ramp. It is an alternative, not a different design system. All tokens, typography, spacing, and component rules apply identically in both modes.

## 3. Typography

**UI Font:** Microsoft YaHei UI (with PingFang SC → Segoe UI → sans-serif fallback). A practical, CJK-capable sans-serif that ships on every Windows desktop in Chinese-speaking labs. No web font loading; zero layout shift.

**Data Font:** JetBrains Mono (with Cascadia Code → SFMono-Regular → monospace fallback). Tabular figures, clear distinguishability between `0`/`O` and `1`/`l`/`I`. Used exclusively for numeric readouts, channel labels, and configuration values.

**Character:** Direct and functional. The pairing is sans-serif UI + monospace data — no serif, no display face, no decorative weight. Typography conveys information, not atmosphere. Weight contrast (400↔700↔800) does the heavy lifting for hierarchy.

### Hierarchy
- **Dashboard Data** (800, 1.5rem, 1.25): Large instrument-style readout. Single values: pressure, temperature, position. JetBrains Mono.
- **Title** (700, 1.125rem, 1.25): Page-level headings, panel titles, wizard step headers. Microsoft YaHei UI.
- **Body** (400, 0.9375rem, 1.5): All running text, labels, descriptions. Max line length 72ch.
- **Section Label** (600, 0.75rem, 1.25, +0.08em): All-caps section markers. Microsoft YaHei UI. **One per section, not one per paragraph.** See the Section Label Rule.
- **Mono** (600, 0.875rem, 1.5): Inline code, IP addresses, port numbers, configuration values. JetBrains Mono.

### Named Rules
**The Data-Is-Mono Rule.** Any numeric value displayed in the interface — pressure reading, temperature, position coordinate, configuration parameter — uses the data font (JetBrains Mono). Proportional fonts are for prose, labels, and UI chrome only.

**The Section Label Rule.** Upper section labels are architectural markers (spacing blocks, not announcements). One label per logical section — not above every heading. When a section has a subtitle paragraph, that's the body voice; the label is the taxonomy.

## 4. Elevation

The system uses **ambient layering** — a combination of subtle shadow and glass-translucent blur to convey depth without heavy drop shadows. Surfaces are flat at rest; depth is an environmental property, not a per-element decision.

### Shadow Vocabulary
- **Panel Shadow** (`0 10px 30px rgba(2, 6, 23, 0.28)`): The single ambient shadow. Used on `.ui-panel`, modals, and dropdowns. Large, diffuse, low-opacity — reads as atmospheric, not cast.

### Glass Surfaces
- **Header** (`rgba(15, 23, 42, 0.75)` + `backdrop-filter: blur(12px)` + bottom border): Top chrome bar. Always present.
- **Rail** (`rgba(15, 23, 42, 0.5)` + `backdrop-filter: blur(10px)` + right border): Left navigation rail. Translucency shows the app background bleeding through.
- **Sidebar** (`rgba(15, 23, 42, 0.6)` + `backdrop-filter: blur(12px)` + right border): Contextual side panel.
- **Footer** (`rgba(30, 41, 59, 0.75)` + `backdrop-filter: blur(12px)` + top border + `0 -20px 50px rgba(0, 0, 0, 0.1)`): Bottom bar with top-facing ambient shadow.

### Named Rules
**The Flat-By-Default Rule.** Interactive surfaces (buttons, inputs, toggles) are flat at rest. Environmental glass surfaces carry the system's depth. Per-element elevation is prohibited — depth is architecture, not decoration.

**The Blur-Is-Chrome Rule.** Backdrop-filter blur is reserved for the application chrome (rail, header, sidebar, footer). Content panels and data surfaces are opaque. Blur never touches data.

## 5. Components

### Buttons
- **Shape:** Tight 4px radius (`--radius-md`). Industrial, not pill-soft.
- **Primary:** Success-green background (`--accent-success`), white text (`#f8fbff`). The "go" button — used for acquire, confirm, apply. One per visible action group.
- **Secondary:** 6% white overlay on surface, `--text-secondary` color, 10% white border. Default for non-primary actions.
- **Ghost:** Fully transparent, `--text-muted` color. On hover: `--text-primary` + 5% white background. Used in rail, toolbars, inline actions.
- **Danger:** 12% red overlay background, `--accent-danger` text, 25% red border. Soft-danger treatment — reads as caution, not alarm.
- **Warning:** 12% amber overlay background, `#f59e0b` text, 25% amber border.
- **Hover / Focus:** All variants transition `all 0.2s ease`. Focus ring uses `--focus-ring` with offset.
- **Disabled:** 55% opacity, `cursor: not-allowed`.

### Inputs & Selects
- **Shape:** 4px radius, same as buttons. Matching rhythm.
- **Background:** 20% black overlay (`rgba(0, 0, 0, 0.2)`) — darker than the panel surface, signaling "this is where data enters."
- **Border:** `--border-default` at rest. Accepts keyboard as interaction.
- **Placeholder:** `--text-muted` — not grayed further. The muted token is already 4.5:1 on panel surfaces.
- **Font:** Inherits `font: inherit` at 0.85rem. Consistent with body scale.

### Toggle
- **Shape:** 36×20px pill. Thumb: 16px circle with shadow.
- **Off:** 25% muted overlay. Neutral, recessive.
- **On:** `--accent-success`. Thumb slides 16px right. The green means "active."
- **Disabled:** 40% opacity, no interaction.

### Status Badge
- **Shape:** Pill (`999px`). Compact padding (`3.2px × 10.4px`).
- **Type:** All-caps label at 0.65rem, 700 weight, +0.05em tracking. 7px colored dot left of label.
- **States:**
  - **Idle:** Slate-grey dot + text on 10% slate background. Neutral baseline.
  - **Connected:** Emerald dot + text on 10% emerald background. Static.
  - **Acquiring:** Emerald dot + text on 12% emerald background, with `box-shadow` glow on dot. Animated pulse via `status-pulse` keyframe (2s infinite).
  - **Error:** Rose dot + text on 10% rose background.
  - **Warning:** Amber dot + text on 10% amber background.

### Panel
- **Shape:** 12px radius (`0.75rem`). Unique in the system — softer than input/button 4px, but still structural.
- **Background:** `--bg-panel`. Border: `--border-default` at 1px.
- **Shadow:** `0 10px 30px rgba(2, 6, 23, 0.28)`.
- **Padding:** 16px header + body (1rem each). Optional `unpadded` variant removes body padding for full-bleed content like charts.

### Navigation (Rail)
- **Layout:** 64px wide, full height. Glass-translucent background, backdrop-blur at 10px, right border.
- **Items:** Vertical icon stack, 48px touch targets. Active item gets `--accent-primary` icon color + left-border accent indicator. Inactive items at `--text-muted`.
- **Interaction:** Hover brightens icon color to `--text-secondary`. Active state is structural (persistent), not transient (hover-only).

## 6. Do's and Don'ts

### Do:
- **Do** use status color only when the status is live — idle devices get neutral, not green.
- **Do** use JetBrains Mono for every numeric readout, channel label, coordinate, and configuration value displayed on screen.
- **Do** maintain the surface ramp (`--surface-0 → --surface-3`) as the only source of background hierarchy. No custom backgrounds outside these four tokens.
- **Do** use glass surfaces (`.glass-header`, `.glass-rail`, `.glass-sidebar`, `.glass-footer`) only on application chrome. Content panels are opaque.
- **Do** keep the primary accent at ≤10% screen surface. Its rarity is the point.
- **Do** match input/button/select border-radius at 4px. Rhythmic consistency across interactive elements.
- **Do** prefer the data font at weight 800 for dashboard readouts — it reads like an instrument, not a label.

### Don't:
- **Don't** use `border-left` or `border-right` greater than 1px as a colored accent stripe on cards or panels.
- **Don't** apply `background-clip: text` with gradient backgrounds. No gradient text anywhere.
- **Don't** use glassmorphism on content panels, data cards, or any surface that carries information. Blur is chrome-only.
- **Don't** put tiny uppercase tracked eyebrows above every section. Section labels are architectural markers, one per section; subtitle paragraphs use body voice.
- **Don't** nest cards inside cards. Flat hierarchy with surface-ramp differentiation; if you feel the need to nest, you're missing a surface token.
- **Don't** use arbitrary z-index values (999, 9999). The semantic scale is: dropdown → sticky → modal-backdrop → modal → toast → tooltip.
- **Don't** animate CSS layout properties (`width`, `height`, `top`, `left`). Transform and opacity for motion; layout changes are instant.
- **Don't** gate content visibility on CSS-triggered reveal animations. Every element must be visible in its default state; animations enhance, never gate.
