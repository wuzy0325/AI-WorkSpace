---
name: 1604 融合平台
description: 清爽薄荷绿白风工业计量与标定系统
colors:
  primary: "#10b981"
  primary-light: "#34d399"
  primary-dark: "#059669"
  accent: "#14b8a6"
  success: "#22c55e"
  warning: "#f59e0b"
  danger: "#ef4444"
  info: "#3b82f6"
  warm-slate-50: "#f9fafb"
  warm-slate-100: "#f3f4f6"
  warm-slate-200: "#e5e7eb"
  warm-slate-300: "#d1d5db"
  warm-slate-400: "#9ca3af"
  warm-slate-500: "#6b7280"
  warm-slate-600: "#4b5563"
  warm-slate-700: "#374151"
  warm-slate-800: "#1f2937"
  warm-slate-900: "#111827"
  text-primary: "#1f2937"
  text-secondary: "#6b7280"
  text-tertiary: "#9ca3af"
  lab-white: "#ffffff"
  surface: "#f6f7f6"
  border: "#e5e7eb"
typography:
  display:
    fontFamily: "'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "36px"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.02em"
  headline:
    fontFamily: "'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "28px"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.02em"
  title:
    fontFamily: "'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "20px"
    fontWeight: 600
    lineHeight: 1.25
  body:
    fontFamily: "'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "'DM Sans', 'SF Pro Display', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: 1.5
    letterSpacing: "0.05em"
rounded:
  sm: "4px"
  md: "8px"
  lg: "12px"
  xl: "16px"
  full: "9999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "32px"
  2xl: "48px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "#ffffff"
    rounded: "{rounded.md}"
    padding: "8px 16px"
  button-primary-hover:
    backgroundColor: "{colors.primary-light}"
    textColor: "#ffffff"
  button-default:
    backgroundColor: "rgba(55, 65, 81, 0.5)"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
  card:
    backgroundColor: "{colors.lab-white}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: "24px"
  input:
    backgroundColor: "{colors.lab-white}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
---

# Design System: 1604 融合平台

## 1. Overview: The Clean Workbench

**Creative North Star: "The Clean Workbench"**

A design system built for field engineers who spend hours in front of industrial PCs. The interface should feel like a freshly organized workbench: every tool is within reach, surfaces are clean, and the ambient color never tires the eyes. We reject dark themes, neon accents, and decorative gradients. The aesthetic is light-first, mint-accented, and state-driven.

This system serves a fusion of measurement and calibration workflows. Density is mid-to-high because engineers need to see status, parameters, and data at a glance. Yet the atmosphere must remain breathable: white dominates, mint guides attention, and every interactive element responds with tactile feedback.

**Key Characteristics:**
- Light background dominance with Fresh Mint as the sole accent.
- Tactile and responsive components: buttons lift, cards gain depth on hover.
- Information density optimized for mid-to-high resolution monitors.
- Element Plus as the sole component library, visually overridden to match the brand.

## 2. Colors: The Mint & Slate Palette

The palette is restrained: one primary accent (Fresh Mint) against a Warm Slate neutral family, all on a Lab White ground. This combination creates a clinical yet approachable atmosphere suitable for long-duration technical work.

### Primary
- **Fresh Mint** (#10b981): The signature accent. Used for primary buttons, active navigation states, success indicators, and hover glows. Its presence is intentional and sparse.
- **Fresh Mint Light** (#34d399): Hover state for primary actions and subtle glow shadows.
- **Fresh Mint Dark** (#059669): Pressed or emphasized primary states.

### Secondary
- **Teal Stream** (#14b8a6): Secondary actions, alternative emphasis, and complementary highlights where Fresh Mint would be overused.

### Neutral
- **Lab White** (#ffffff): Page background and card surfaces.
- **Surface** (#f6f7f6): Sidebar background, secondary page areas, and elevated surfaces.
- **Warm Slate 200** (#e5e7eb): Borders, dividers, and subtle separators.
- **Warm Slate 300** (#d1d5db): Stronger borders on focus or hover.
- **Warm Slate 400** (#9ca3af): Muted text, placeholders, disabled states.
- **Warm Slate 500** (#6b7280): Secondary text, labels, descriptions.
- **Warm Slate 700** (#374151): Dark tint for default button fills and subtle darkening effects.
- **Warm Slate 800** (#1f2937): Primary text color. Also used as the dark tint for certain legacy overrides.
- **Warm Slate 900** (#111827): Deepest neutral, used for strong contrast or shadow bases.

### Semantic
- **Signal Green** (#22c55e): Success states, completed badges, positive trends.
- **Amber Alert** (#f59e0b): Warnings, paused states, attention-required items.
- **Stop Red** (#ef4444): Errors, danger actions, disconnect states.
- **Info Blue** (#3b82f6): Informational tags, hints, neutral highlights.

### Named Rules
**The One Voice Rule.** The primary accent (Fresh Mint) is used on ≤10% of any given screen. Its rarity is the point. Most surfaces remain Lab White or Surface.

## 3. Typography: The Engineered Sans

**Display Font:** DM Sans, SF Pro Display, -apple-system, BlinkMacSystemFont, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif
**Body Font:** Same stack.
**Label/Mono Font:** JetBrains Mono, Fira Code, SF Mono, Consolas, monospace.

**Character:** A highly legible neo-grotesque sans-serif with generous spacing. Engineered for clarity on industrial monitors, not fashion. The pairing is single-stack: one family handles every role, differentiated solely by weight and size.

### Hierarchy
- **Display** (700, 36px, 1.2, -0.02em): Rarely used. Reserved for hero numbers or singular page titles.
- **Headline** (700, 28px, 1.2, -0.02em): Page titles inside workbenches (e.g., "计量工作台").
- **Title** (600, 20px, 1.25): Section headers, card titles, module names.
- **Body** (400, 14px, 1.5): All running text, descriptions, form labels. Max line length: 65–75ch.
- **Label** (500, 12px, 1.5, 0.05em): Uppercase or small-caps tags, status labels, metadata. Often paired with the mono font for numeric readouts.

### Named Rules
**The Flat Scale Rule.** Hierarchy is built through weight contrast (400 vs 600 vs 700) and size jumps of at least 1.25×. Avoid intermediate sizes that flatten the ladder.

## 4. Elevation: Layered by Response

The system uses a hybrid of tonal layering and soft shadows. At rest, surfaces are flat (Lab White on Surface). Elevation is expressed primarily through shadow on interaction: cards and buttons lift when hovered, gaining a soft diffused shadow. There is no persistent "floating" chrome; depth is a response to state.

### Shadow Vocabulary
- **Ambient Low** (`0 1px 2px rgba(0,0,0,0.05)`): Default resting shadow for cards and inputs. Barely perceptible.
- **Ambient Medium** (`0 4px 6px -1px rgba(0,0,0,0.07), 0 2px 4px -2px rgba(0,0,0,0.05)`): Raised cards, dropdowns, popovers.
- **Ambient High** (`0 10px 15px -3px rgba(0,0,0,0.08), 0 4px 6px -4px rgba(0,0,0,0.05)`): Hover state for feature cards, modals, drawers.
- **Glow Mint** (`0 0 8px rgba(16,185,129,0.25)`): Focus rings and active pulse effects on primary elements.
- **Card Hover** (`0 10px 15px -3px rgba(0,0,0,0.08), 0 4px 6px -4px rgba(0,0,0,0.05), 0 0 20px rgba(16,185,129,0.1)`): The signature card hover — a lifted ambient shadow combined with a subtle mint aura.

### Named Rules
**The Flat-By-Default Rule.** Surfaces are flat at rest. Shadows appear only as a response to state (hover, elevation, focus).

## 5. Components

### Buttons
- **Shape:** Gently rounded corners (8px radius).
- **Primary:** Fresh Mint gradient fill (135deg from #10b981 to #059669), white text, no border, soft mint shadow (`0 2px 8px rgba(16,185,129,0.3)`). Padding: 8px 16px.
- **Hover / Focus:** Gradient lightens (#34d399 to #10b981), shadow deepens (`0 4px 12px rgba(16,185,129,0.4)`), button translates up 1px. Transition: 150ms ease.
- **Default (Secondary):** Semi-transparent Warm Slate 700 background (`rgba(55,65,81,0.5)`), 1px border in Warm Slate 200, Warm Slate 800 text. Hover: background lightens to `rgba(75,85,99,0.6)`, border darkens to Warm Slate 300.

### Cards / Containers
- **Corner Style:** 12px radius.
- **Background:** Lab White (#ffffff).
- **Shadow Strategy:** Ambient Low at rest. Card Hover shadow on mouseover.
- **Border:** 1px solid Warm Slate 200 (#e5e7eb). On hover, border shifts to a translucent Fresh Mint (`rgba(16,185,129,0.3)`).
- **Internal Padding:** 24px (standard), 16px (compact).

### Inputs / Fields
- **Style:** Lab White background (#ffffff), 1px border in Warm Slate 200, 8px radius. (Note: current global overrides in `global.scss` apply a dark-tinted background to Element Plus inputs; this is legacy drift and should be aligned to the canonical Lab White token.)
- **Focus:** Border shifts to Fresh Mint, accompanied by a soft glow ring (`0 0 0 3px rgba(16,185,129,0.15)`).
- **Text:** Warm Slate 800 primary text; Warm Slate 400 placeholder text.

### Navigation
- **Sidebar:** 240px fixed width, Surface background (#f6f7f6), right border in Warm Slate 200.
- **Items:** 8px vertical padding, 12px horizontal padding, 8px radius. Default: Warm Slate 500 text. Hover: Surface darken + Warm Slate 700 text. Active: Fresh Mint translucent background (`rgba(16,185,129,0.12)`), 1px left border in Fresh Mint, Fresh Mint icon, bold text.
- **Mobile:** Horizontal bottom bar, icon-only with 10px labels.

### Tags / Status Badges
- **Style:** Small rounded rectangles (4px radius), medium weight (500). Background is a 15% opacity tint of the semantic color, border is 30% opacity tint, text is the 400-level shade.
- **Variants:** Signal Green (success), Amber Alert (warning), Stop Red (danger), Info Blue (info).

## 6. Do's and Don'ts

### Do:
- **Do** use Lab White (#ffffff) as the dominant background for all work surfaces.
- **Do** use Fresh Mint (#10b981) sparingly: primary buttons, active nav states, and success indicators only.
- **Do** maintain a 4px base spacing grid (4, 8, 12, 16, 20, 24, 32, 48px).
- **Do** use 12px radius for cards and 8px radius for buttons and inputs.
- **Do** ensure body text lines cap at 65–75ch for readability.
- **Do** use Element Plus components as the foundation and override tokens via SCSS/CSS Variables for consistency.

### Don't:
- **Don't** use dark themes. The system is light-first by brand mandate.
- **Don't** use high-saturation gradients or decorative gradient text.
- **Don't** use border-left or border-right greater than 1px as a colored stripe accent on cards, alerts, or list items.
- **Don't** nest cards inside cards. Use spacing and dividers for hierarchy.
- **Don't** use over-animation or bounce/elastic easing. Keep transitions to 150–350ms with ease curves.
- **Don't** introduce non-Element Plus components into the UI; maintain library consistency.
- **Don't** use glassmorphism as a default decorative treatment.
- **Don't** rely on the hero-metric template (big number + small label + gradient) for data display.
