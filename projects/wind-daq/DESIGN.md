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

- `docs/design/monitor-workspace-spec.md` — **监控工作区规范（2026-07）**：壳层/工作区分隔、全局 vs 本设备操作作用域、默认 6 路曲线、图外读数条、通道命名与分组、token 映射与验收清单。改仪表盘 / 混合视图 / 设备详情时必读。参考原型：`docs/ui-redesign-prototype.html`。
- `docs/design/chart-spec.md` — Chart & waveform visual spec, channel color extension, threshold visualization, anti-patterns.
- `docs/design/light-theme-palette.md` — Light theme audit, target palette, contrast compliance, action plan for filling gaps.（部分填坑项已落地；执行以 remediation 为准）
- `docs/design/iconography.md` — Icon library choice, semantic icon map, custom icon rules, sizing.
- `docs/design/design-system-remediation.md` — **设计系统欠账整改计划（2026-07）**：浅色收尾、App.vue/token brand 对齐、图标清理、硬编码色迁移（保留 channelColors API）、主题基线翻转。执行顺序与 DoD 以该文档为准。

Additional specs to be added: state vocabulary, copy guidelines, motion guidelines, keyboard & a11y.

## Monitor Workspace (summary)

设备监控混合视图的硬规则摘要（细则见 `monitor-workspace-spec.md`）：

1. **分隔**：`rail / sidebar / canvas / panel` 四级 surface；面板内只用细线三级分隔，禁止多层重卡片。
2. **作用域**：顶栏 = 全局运行控制；设备标题行右侧 = 本设备（校零 / 停止 / 断开）。停止按钮禁止使用成功绿。
3. **曲线默认**：默认显示 6 路；图例单击显隐、双击独显；**不提供**「聚焦 / 全部 / 异常」三态切换。
4. **读数**：多通道光标值放在图表外 readout bar，不遮挡波形。
5. **命名**：统一 `CH01`…；压力与环境量分组；数值 mono + tabular-nums。
6. **实现**：颜色/间距/圆角只走 `styles/tokens/*` 与 `--chart-*`；禁止 feature 内 hex 与平行 `CHANNEL_COLORS` 数组。

## Migration Notes

The old `Cursor DAQ` Electron project is no longer the visual target. It remains in the repository (under `docs/migration/`) only as a feature inventory — what features must exist, what operator workflows must be preserved. Visual layout, color choices, and component composition are no longer constrained by that project.

See `docs/migration/README.md` for the current role of migration docs and `docs/ui-design-audit.md` for the per-screen cleanup backlog.

## Density Spec

配置类画面（设备配置、运动控制器配置、校准设置、遍历设置、全局设置）采用 **VSCode 风格紧凑密度**，目标是在 1600×900 默认窗口内尽量多承载字段，同时保持中文标签 + 数字输入的可读性。密度 token 定义在 `styles/tokens/spacing.css`，前缀统一为 `--density-*`。

### 间距分级

| 层级 | Token | 值 | 用途 |
|---|---|---|---|
| 字段内 | `--density-field-inline` | 2px | label ↔ control 纵向间距 |
| 字段间 | `--density-field-gap` | 8px | 同一分组内相邻字段间距 |
| 分组内边距 | `--density-group-padding` | 8px 12px | 配置卡片 / boxed section 的 padding |
| 分组间 | `--density-group-gap` | 10px | 相邻配置卡片之间 |
| 区块间 | `--density-section-gap` | 12px | 顶层 section 之间 |
| 分组标题间距 | `--density-group-title-gap` | 4px | 分组标题与正文之间 |
| 控件高度 | `--density-control-height` | 28px | input / select 统一高度 |
| 控件横向内边距 | `--density-control-pad-x` | 8px | input / select 横向 padding |

### 应用规则

1. **字段内布局**：label 与控件用 `flex-direction: column; gap: var(--density-field-inline)`。
2. **字段网格**：同分组字段用 `display: grid; gap: var(--density-field-gap)`，列数根据面板宽度自适应（通常 2–3 列）。
3. **分组卡片**：`padding: var(--density-group-padding)`，标题与正文之间用 `margin-bottom: var(--density-group-title-gap)`。
4. **区块容器**：顶层 `display: flex; flex-direction: column; gap: var(--density-section-gap)`。
5. **控件高度**：所有 input/select/number 输入统一 `height: var(--density-control-height)`，避免画面内高度参差。
6. **底线**：字段内 label 最小行高 1.25（约 14px line-height），input 最小高度 28px，确保中文与数字可读。

### 禁止事项

- 配置画面内禁止使用 `--space-4`（16px）或更大的字段间距，除非是区块顶层 `--density-section-gap` 之上的留白。
- 禁止在同一画面混用紧凑档（`--density-*`）与标准档（`--space-3/4`）的间距值，必须统一到本规范。
- 禁止字段标签使用 `text-transform: uppercase`（仅英文标签可用），中文标签应保持原形并配 `font-weight: 600`。

### 适用范围与例外

- **适用**：DaqT1603Config、MotionControllerConfig、AxisConfigCard、GlobalSettingsModal、FiveHole/ThreeHole/TotalPressure/TotalTemperature Settings、TraversalSettings。
- **例外**：数据面板（DeviceOverviewPanel、RealtimeChart）、Dashboard 卡片仍使用标准 `--space-*` 档，保持仪表读数的呼吸感；对话框底部操作区（保存/取消按钮）保留 `--space-3` 以上间距，避免误点。
