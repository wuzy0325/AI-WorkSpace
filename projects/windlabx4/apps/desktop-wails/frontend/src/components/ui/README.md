# windlabx4 UI Component Rules

> This directory is the project-level UI foundation for windlabx4. Use these components first when building product UI. Naive UI is the underlying widget library, not the default surface for feature code.

## Role

`components/ui/` owns small, reusable UI primitives that keep windlabx4 visually consistent:

- `UiButton` for standard actions
- `UiInput` for text-like input
- `UiSelect` for single selection
- `UiToggle` for boolean settings
- `UiPanel` for panel/card containers
- `UiSectionHeader` for compact section titles
- `UiStatusBadge` for device and workflow status

These components may wrap Naive UI, but they must not know about devices, calibration, traversal, motion, storage, reports, or any business workflow.

## Usage Rules

Use `Ui*` components when the interaction matches an existing primitive:

- Use `UiButton` before direct `NButton` for normal feature actions.
- Use `UiInput` before direct `NInput` for simple text or numeric text fields.
- Use `UiSelect` before direct `NSelect` for simple single-select fields.
- Use `UiToggle` before direct `NSwitch` for boolean settings.
- Use `UiPanel` before direct `NCard` for product panels.
- Use `UiStatusBadge` before direct `NTag` for connected, acquiring, warning, error, and idle status.

### UiButton 使用边界

`UiButton` 适用于以下场景：
- 工具栏操作按钮（连接、断开、开始、停止等）
- 表单提交按钮
- 弹窗确认/取消按钮
- 简单的图标按钮

`UiButton` **不适用于**以下场景（应使用原生 `<button>` 或专门的列表项组件）：
- 侧边栏设备列表项（需要复杂的内部 flex 布局，如名称+状态+地址的多行结构）
- 控制器列表项（需要状态圆点、名称、连接地址的多列布局）
- 任何需要 `NButton` 内部 `.n-button__content` 作为 flex 容器且需要自定义布局的列表项

原因：`UiButton` 包装了 `NButton`，其内部 `.n-button__content` 使用 `display: inline-flex`，会压缩和干扰复杂的内部布局。强行使用会导致文本截断、布局错乱，需要使用大量 `:deep()` 覆盖，反而增加维护成本。

### 列表项组件规范

对于侧边栏列表、卡片列表等场景，优先使用以下方式：
1. 原生 `<div>` 或 `<button>` 元素 + 自定义样式
2. 如果该列表项模式在多个领域复用，创建专门的 `UiListItem` 或 `UiSidebarItem` 组件
3. 列表项组件可以放在 `components/ui/`（如果通用）或 `components/patterns/`（如果带特定布局模式）

列表项组件应支持：
- 激活状态样式
- 悬停状态样式
- 错误状态样式
- 内部自由布局（不受 `NButton` 限制）

Direct Naive UI usage is allowed when:

- No `Ui*` wrapper exists yet.
- The component is complex enough that wrapping it first would add churn, such as `NDataTable`, `NModal`, `NSteps`, `NInputNumber`, `NCheckbox`, or chart-adjacent controls.
- The file is a spike or migration-only experiment.
- **需要 `NButton` 的 `type` 属性（如 `type="primary"`、`type="error"`）且 `UiButton` 的 `variant` 无法满足时**。

When direct Naive UI usage repeats across two or more feature areas, add or extend a `Ui*` primitive before continuing broad usage.

## API Rules

Keep primitive APIs small and semantic:

- Use product terms like `variant`, `size`, `status`, and `padded`.
- Do not expose every Naive UI prop by default.
- Add props only when at least two real call sites need them.
- Preserve existing prop names unless there is a concrete migration plan.

Do not add:

- Business-domain props such as `deviceId`, `calibrationType`, `axisName`, or `probeMode`.
- Store imports.
- API calls.
- File, hardware, serial, TCP, or Wails calls.

## Visual Rules

Primitives must use project tokens from `styles/tokens/` or Naive theme overrides from `App.vue`.

Avoid in primitive components:

- Inline font sizes.
- Inline spacing values.
- Raw hex colors.
- Component-local visual systems that do not map back to tokens.

Existing inline styles in this directory are migration debt. New changes should move repeated values into scoped classes or token-backed CSS.

## Primitive Inventory

All planned primitives are now implemented:

| Component | Status | Purpose |
|---|---|---|
| `UiButton` | ✅ | Standard action button |
| `UiInput` | ✅ | Text input |
| `UiSelect` | ✅ | Single select |
| `UiToggle` | ✅ | Boolean toggle |
| `UiCheckbox` | ✅ | Checkbox |
| `UiInputNumber` | ✅ | Numeric input |
| `UiPanel` | ✅ | Panel/card container |
| `UiSectionHeader` | ✅ | Section title |
| `UiStatusBadge` | ✅ | Status indicator |
| `UiFormField` | ✅ | Form field layout |
| `UiDialog` | ✅ | Modal dialog |
| `UiEmptyState` | ✅ | Empty data panels |
| `UiLoadingState` | ✅ | Loading spinner |
| `UiErrorState` | ✅ | Recoverable errors |
| `UiToolbar` | ✅ | Dense action rows |
| `UiDataTableShell` | ✅ | Table container with states |
| `UiAlert` | ✅ | Alert banners |
| `UiSteps` / `UiStep` | ✅ | Step indicators |
| `UiSpin` | ✅ | Spinner overlay |
| `UiListItem` | 📝 | Sidebar/card list item with free internal layout |

## Review Checklist

Before merging UI changes, confirm:

- Existing `Ui*` primitives were used when available.
- New primitives are business-agnostic.
- Direct Naive UI usage is justified.
- Loading, empty, error, disabled, and selected states are covered where applicable.
- Long labels and small target windows do not break layout.
- Styles use project tokens rather than raw visual values.
