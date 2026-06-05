# Implementation Plan: VSCode-Style Compact Layout Unification

## Overview

将所有 6 个前端项目统一到 VSCode 风格的紧凑布局体系，解决当前设计系统碎片化问题（5 套配色、token 各自维护）。以 `shared/frontend` 为单一设计源，新增 VSCode 风格布局组件，逐个项目迁移。

## Core Constraint: Style-Logic Separation

**本计划只动样式、组件组合方式，以及少量受控的呈现层状态，不动业务逻辑。**

| 类别 | 允许改动 | 禁止改动 |
|------|----------|----------|
| **样式** | token 值、主题色、字号、间距、圆角、阴影 | — |
| **组件结构** | 替换/包装外层布局组件（如 AppShell → RtWorkbench） | — |
| **呈现层状态** | 包装层 `ref`、受控 `computed`、布局交互事件（tab/panel/split） | store 派生业务状态、领域规则判断、异步业务流程 |
| **业务逻辑** | — | 业务组件内部的 Vue ref/reactive 数据、computed 属性、watchers |
| **控件内容** | — | 业务控件 props 契约、slot 内容语义、文案含义、事件处理意图 |
| **数据流** | — | Pinia store、API 调用、bridge 桥接代码 |
| **路由/导航** | Tab 作为呈现层映射到现有 router | 路由守卫、查询参数处理、懒加载逻辑 |

**判定标准：业务组件的 `<script setup>` 保持不动；允许在新增共享布局组件、App 外层包装层、以及纯呈现层映射文件中加入最小必要的 UI 状态逻辑。**

## Architecture Decisions

| 决策 | 选择 | 理由 |
|------|------|------|
| Accent 色 | 保留 Cyan `#06b6d4` | 改动最小，已有 token 基础 |
| 布局组件位置 | `shared/frontend/components/` | 单一维护点，所有项目直接 import |
| 插值器改造深度 | 仅换色 | 单文件应用，结构改不动 |
| Tab 系统 | 需要 | wind-daq 5 个页面用 Tab 替代 Router 切换（仅呈现层） |
| CSS 方案 | 纯 CSS Custom Properties | 不引入 Tailwind 到 shared 层，保持零依赖 |
| 共享样式发布 | 双入口并行：`index.css` 保持旧风格，新增 `vscode.css` | 避免 Phase 1 全局换肤，支持按项目渐进迁移 |
| 布局模型 | CSS Grid + Flexbox | Workbench 用 Grid 定义区域，内部用 Flex |
| 迁移策略 | 外层替换 + 内层保留 | RtWorkbench 替代 AppShell，内部业务组件原样保留 |

## VSCode 布局元素映射

```
┌─────────────────────────────────────────────────────┐
│ RtTitleBar (30px)                                    │
├────┬──────────┬─────────────────────────────────────┤
│    │          │ RtTabBar                             │
│ Rt │ Rt       │─────────────────────────────────────│
│ Ac │ Side     │                                      │
│ ti │ bar      │  Main Content                        │
│ vi │ (220px)  │  (flexible)                          │
│ ty │          │                                      │
│ Ba │          │                                      │
│ r  │          ├─────────────────────────────────────┤
│48px│          │ RtBottomPanel (collapsible)          │
├────┴──────────┴─────────────────────────────────────┤
│ RtStatusBar (22px)                                   │
└─────────────────────────────────────────────────────┘
```

---

## Phase 1: Token & Theme 重写

### Task 1.1: 重写 layout tokens

**文件:** `shared/frontend/tokens/layout.css`

**变更:**
```css
:root {
  --layout-titlebar-height: 30px;      /* was: --layout-header-height: 56px */
  --layout-statusbar-height: 22px;     /* was: --layout-bottombar-height: 64px */
  --layout-activitybar-width: 48px;    /* new */
  --layout-sidebar-width: 220px;       /* was: 248px */
  --layout-sidebar-min-width: 170px;   /* new */
  --layout-sidebar-max-width: 400px;   /* new */
  --layout-bottompanel-min-height: 120px;  /* new */
  --layout-bottompanel-default-height: 200px; /* new */
  --layout-content-padding: 0.5rem;    /* was: 1.25rem */
  --layout-content-gap: 0.25rem;       /* was: 1rem */
  --layout-panel-radius: 0;            /* was: 0.75rem — VSCode 无圆角 */
  --layout-panel-padding: 0.5rem;      /* was: 1rem */
  --layout-tab-height: 35px;           /* new */
  --layout-section-header-height: 22px; /* new */
}
```

### Task 1.2: 重写 typography tokens

**文件:** `shared/frontend/tokens/typography.css`

**变更:**
```css
:root {
  --font-size-xs: 0.6875rem;   /* 11px, was 0.65rem */
  --font-size-sm: 0.75rem;     /* 12px, keep */
  --font-size-base: 0.8125rem; /* 13px, was 0.875rem — VSCode 标准 */
  --font-size-md: 0.875rem;    /* 14px, was 0.95rem */
  --font-size-lg: 1rem;        /* 16px, was 1.125rem */
  --font-size-xl: 1.125rem;    /* 18px, was 1.25rem */
  --font-size-2xl: 1.25rem;    /* 20px, was 1.5rem */
  --font-size-3xl: 1.5rem;     /* 24px, was 1.875rem */
}
```

### Task 1.3: 新增 VSCode dark theme（并行入口，不覆盖现有 dark）

**文件:** `shared/frontend/themes/vscode-dark.css`

**关键变更:**
- 背景色系对齐 VSCode Dark+ 系列: `#1e1e1e` / `#252526` / `#2d2d2d` / `#333333`
- 去除 glassmorphism (`backdrop-filter`)，改为纯色扁平背景
- 边框改为 VSCode 风格: `#3c3c3c` / `#474747`
- 去除 glow/shadow 效果，改为平面高亮
- Tab 激活指示器用顶部 1px 线条（VSCode 风格）

```css
--bg-app: #1e1e1e;
--bg-canvas: #1e1e1e;
--bg-panel: #252526;
--bg-panel-strong: #2d2d2d;
--bg-elevated: #333333;
--bg-input: #3c3c3c;
--border-default: #3c3c3c;
--border-strong: #474747;
--accent-primary: #06b6d4;       /* 保留 cyan */
--accent-tab-active: #06b6d4;    /* tab 激活指示线 */
--bg-tab-active: #1e1e1e;
--bg-tab-inactive: #2d2d2d;
--bg-activitybar: #333333;
--bg-sidebar: #252526;
--bg-titlebar: #3c3c3c;
--bg-statusbar: #06b6d4;          /* VSCode 风格彩色状态栏 */
--statusbar-text: #ffffff;
```

### Task 1.4: 新增 VSCode light theme

**文件:** `shared/frontend/themes/vscode-light.css`

对齐 VSCode Light+ 色系: `#ffffff` / `#f3f3f3` / `#e8e8e8`

### Task 1.5: 更新 spacing / radius / motion tokens

- `radius`: VSCode 几乎无圆角，`--radius-sm: 2px`, `--radius-md: 3px`, `--radius-lg: 4px`
- `motion`: 保持快速过渡 `--motion-fast: 80ms`, `--motion-base: 120ms`
- `spacing`: 保持现有 scale，布局组件内部用紧凑值

### Task 1.6: 保留 glass.css，新增 vscode-utility.css 与独立入口

- `glass.css` 暂不删除，继续服务未迁移项目
- 新增 VSCode 风格工具类: `.rt-flat`, `.rt-inset`, `.rt-divider`
- 新增 `shared/frontend/vscode.css`，仅导入 VSCode 所需 token/theme/utility
- 已迁移项目改为显式导入 `@shared/frontend/vscode.css`
- 未迁移项目继续使用 `@shared/frontend/index.css`

### Checkpoint: Phase 1
- [ ] `shared/frontend/index.css` 导入链完整
- [ ] 新增 `shared/frontend/vscode.css`，且不影响现有 `index.css`
- [ ] 所有 token 变量名向后兼容；旧变量名在 `compatibility.css` 中保留别名，但不承担旧布局尺寸兼容职责
- [ ] 旧入口视觉不变；新入口在示例页或标杆项目中可独立预览

---

## Phase 2: 共享布局组件

所有组件放在 `shared/frontend/components/` 下，通过 `components/index.ts` 导出。

### Task 2.1: RtActivityBar

**文件:** `shared/frontend/components/RtActivityBar.vue`

VSCode 左侧 Activity Bar（48px 宽图标条）:
- Props: `items: ActivityBarItem[]`（id, icon, label, active, disabled）
- Events: `select(id)`, `open-settings`
- 顶部图标区 + 底部设置图标
- Active 状态: 左侧 2px 竖线指示器 + 图标高亮
- 宽度固定 `var(--layout-activitybar-width)`

### Task 2.2: RtSidebar

**文件:** `shared/frontend/components/RtSidebar.vue`

VSCode 侧边栏面板:
- Props: `title`, `collapsed`, `width`
- Slots: `default`, `header-actions`
- 可折叠的 section headers（VSCode Explorer 风格）
- 宽度可通过 CSS 变量控制
- 折叠时仅显示 section headers

### Task 2.3: RtTabBar

**文件:** `shared/frontend/components/RtTabBar.vue`

VSCode 编辑器标签栏:
- Props: `tabs: TabItem[]`（id, label, icon, closable, dirty）
- Events: `select(id)`, `close(id)`
- Active tab: 底部 1px accent 线 + 背景与编辑区一致
- Inactive tab: 背景略深 + 文字变灰
- 溢出时可横向滚动
- Dirty 标记（未保存圆点）
- Close 按钮（hover 显示）

### Task 2.4: RtBottomPanel

**文件:** `shared/frontend/components/RtBottomPanel.vue`

VSCode 底部面板（Terminal/Problems/Output 区域）:
- Props: `tabs: PanelTab[]`, `activeTab`, `collapsed`, `height`
- Events: `select-tab(id)`, `toggle-collapse`, `resize(height)`
- 可拖拽调整高度（drag handle）
- 折叠/展开切换
- 内部 tab 切换

### Task 2.5: RtStatusBar

**文件:** `shared/frontend/components/RtStatusBar.vue`

VSCode 状态栏（22px）:
- Slots: `left`, `right`
- 背景色使用 `var(--bg-statusbar)`
- 内部 items 用 flex 排列，hover 高亮

### Task 2.6: RtTitleBar

**文件:** `shared/frontend/components/RtTitleBar.vue`

VSCode 标题栏（30px）:
- Props: `title`, `version`
- Slots: `actions`, `breadcrumbs`
- Wails 窗口拖拽区域 (`--wails-draggable: drag`)
- 紧凑布局: 标题 + 操作按钮

### Task 2.7: RtPanelSplit

**文件:** `shared/frontend/components/RtPanelSplit.vue`

可拖拽分割面板:
- Props: `direction: 'horizontal' | 'vertical'`, `initialRatio`, `minSize`
- Slots: `first`, `second`
- 拖拽手柄（2px hover 高亮线条）
- Phase 2 初版仅做内存态拖拽，不做持久化

### Task 2.8: RtWorkbench

**文件:** `shared/frontend/components/RtWorkbench.vue`

完整 VSCode 布局骨架组合组件:
- 组合 ActivityBar + Sidebar + TabBar + Main + BottomPanel + StatusBar + TitleBar
- Props: 控制各区域可见性、sidebar 宽度、bottom panel 高度
- 使用 CSS Grid 定义整体布局
- 提供具名 slots 让各项目填充内容

```
Grid template:
  "titlebar  titlebar  titlebar"  30px
  "activity  sidebar   editor"    1fr
  "activity  sidebar   bottom"    auto
  "statusbar statusbar statusbar" 22px
```

### Task 2.9: 更新 components/index.ts

导出所有新组件，保持现有 Rt* 命名前缀。

### Checkpoint: Phase 2
- [ ] 所有新组件 `npm run typecheck` 通过（至少在 `wind-daq`、`daq-t1603`、`motion-controller` 三类项目中验证导入）
- [ ] 每个组件有基础 props/events 类型定义
- [ ] wind-daq 可以 import 并使用 RtWorkbench 渲染空白布局

---

## Phase 3: wind-daq 迁移（标杆项目，外层替换）

> **非目标（明确禁止改动）:**
> - Pinia store（deviceStore/motionStore/calibrationStore/traversalStore 等 8 个）
> - API 层（deviceApi/motionApi/calibrationApi 等）
> - Bridge 桥接（wails-adapter/http-client/sse-client）
> - 任何业务组件的 `<script setup>` 逻辑
> - calibration/traversal/motion 等子系统的算法和流程

### Task 3.1: 替换 AppShell 外层为 RtWorkbench

- `<template>` 中用 `RtWorkbench` 替代 `AppShell` 的 div 结构
- **保留** MainTopBar/MainBottomBar/DeviceSidebar/LogPanel 等内部组件，**仅替换它们在外层的位置**
- 这些内部组件的 `<script setup>` 逻辑、事件处理、数据绑定全部不动

### Task 3.2: 引入 RtActivityBar 包装 rail nav items

- 把 `AppRailNav` 的 items 列表移到外层（仅 template 结构变化）
- `AppRailNav` 组件本身保留，其 `<script setup>` 中的 getIconComponent、emit 定义不动
- 内部样式按新 token 调整

### Task 3.3: 引入 RtTabBar 作为页面切换呈现层

- 新增一个 `presentation/pageTabs.ts`（纯 UI 状态，非 store），定义 tab 列表
- 保留 vue-router 不删除，将路由名映射为 tab id
- 视图层用 `RtTabBar` 展示，点击 tab 调用 `router.push`
- 允许在 App 外层或布局包装层新增最小 `computed/ref` 用于 tab 激活态同步
- **router 配置、路由守卫、懒加载全部不动**

### Task 3.4: 清理本地 token 覆盖

- 删除 `src/styles/tokens/` 本地 token 文件
- 删除 `src/styles/themes/` 本地主题文件
- 迁移后的布局与主题统一使用 `@shared/frontend/vscode.css` 导入
- 保留项目特有的 channel/axis 颜色变量（如有）

### Task 3.5: 更新 Ui* 组件包装层的 style

- UiButton / UiPanel 等更新为使用新 token
- **不动**这些组件的 props/slots/events 定义

### Checkpoint: Phase 3
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] `npm run test` 通过（现有 4 个 store 测试）
- [ ] diff 检查: 业务组件的 `<script setup>` 部分无变化或仅有纯类型/导入调整
- [ ] 视觉验证: 布局紧凑度接近 VSCode，Tab 切换正常，BottomPanel 可折叠

---

## Phase 4: daq-t1603 迁移（外层替换）

> **非目标（明确禁止改动）:**
> - Pinia store（deviceStore/logStore/recordingStore）
> - Bridge（deviceBridge/recordingBridge）
> - ChannelGrid/ChannelCard/RealtimeChart/DaqT1603Config 等业务组件的 script

### Task 4.1: 替换 AppShell 外层为 RtWorkbench

- `<template>` 中用 `RtWorkbench` 替代 `AppShell` 的 div 结构
- 保留 MainTopBar/MainBottomBar/LogPanel/DeviceSidebar 内部组件
- 它们的 `<script setup>` 逻辑（toggleAcquisition/openAddDevice/openScanDialog 等）全部不动

### Task 4.2: 内部组件样式对齐新 token

- MainTopBar/MainBottomBar/DeviceSidebar/LogPanel 的 `<style>` 部分按新 token 调整
- 业务数据流、事件 emit 保持不变

### Task 4.3: 清理内联 token 重复

- 删除 `styles.css` 中重复的 token 定义（508 行）
- 改为 `@import '@shared/frontend/vscode.css'`
- 保留项目特有样式（ChannelCard 内部 grid 布局等）

### Checkpoint: Phase 4
- [ ] `npm run typecheck` + `npm run build` + `npm run test` 通过
- [ ] diff 检查: 业务组件 script 部分无变化
- [ ] 视觉验证: 与 wind-daq 风格一致

---

## Phase 5: motion-controller 迁移（最小化）

> **非目标:**
> - `@shared/motion` 模块的内部逻辑（MotionControlPanel 的 jog/move/monitoring）
> - motionStore、motionApi、motionConfigEditor

### Task 5.1: 引入 RtWorkbench 外层

- 当前 `App.vue` 直接渲染 MotionView，无外层
- 在 `App.vue` 的 template 中加 `RtWorkbench` 包裹
- MotionView、ToastOverlay 保持原样

### Task 5.2: 清理本地 token

- 与 wind-daq 相同，删除本地 token 覆盖并切到 `@shared/frontend/vscode.css`
- 统一使用 shared

### Checkpoint: Phase 5
- [ ] `npm run typecheck` + `npm run build` + `npm run test` 通过
- [ ] diff 检查: MotionView、motionStore、motionConfigEditor 无变化

---

## Phase 6: 插值器轻量接入（仅换色）

> **非目标:**
> - 三个插值器的单文件结构（不拆为多组件）
> - App.vue 的 `<script setup>` 中所有业务逻辑（PRB 加载、插值计算、文件上传、结果导出等）
> - 不引入 dark theme（保持 light only）
> - 不引入 RtWorkbench（单文件无外层布局需求）

### Task 6.1: five-hole-interpolator 仅换色

- App.vue 的 `<style>` 中：将本地 CSS 变量值改为与 shared 一致
  - `--color-primary`: `#6366f1` → `#06b6d4`（Cyan）
  - 字体 `--font-size-base`: 对齐到 `0.8125rem`
  - 间距、圆角等数值对齐新 token
- 添加 `@import '@shared/frontend/vscode.css'` 到主样式入口（仅引用 token，不引用组件）
- 业务逻辑部分（script 部分、PRB 解析、结果计算）完全不动

### Task 6.2: three-hole-interpolator (Wails) 仅换色

- 同 Task 6.1
- 调整颜色 `#4f46e5` → `#06b6d4`

### Task 6.3: three-hole-interpolator (Win7) 仅换色

- 同 Task 6.1
- 调整颜色 `#4f46e5` → `#06b6d4`
- `http-adapter.ts` 不动

### Checkpoint: Phase 6
- [ ] 三个插值器 `npm run build` 通过
- [ ] diff 检查: script 部分无任何变化
- [ ] 视觉验证: 配色和字号与其他项目一致

---

## Risks and Mitigations

| 风险 | 影响 | 缓解策略 |
|------|------|----------|
| Token 重命名破坏现有组件 | High | 保持 `index.css` 旧入口不变；新增 `vscode.css` 新入口；`compatibility.css` 只解决变量名兼容，不承担旧布局尺寸兼容 |
| wind-daq 改用 Tab 替代 Router 影响导航 | Medium | **保留 vue-router 不删除**，Tab 仅作为呈现层，点击触发 router.push |
| 替换 AppShell 时误改业务组件 script | High | 每个 Phase 的 Checkpoint 增加 diff 检查：业务组件 `<script setup>` 部分无变化 |
| 各项目 Tailwind 依赖与 shared 纯 CSS 冲突 | Low | shared 不引入 Tailwind，项目本地 Tailwind 仅用于 utility class |
| VSCode 扁平风格与现有 glassmorphism 不兼容 | Medium | 保留 `glass.css` 给旧入口；已迁移项目显式切换到 `vscode.css` |
| 插值器单文件结构难以引用 shared | Low | 仅引用 token CSS，不引用组件，避免结构改动 |
| 现有组件 props/slots 不兼容新包装层 | Medium | 新组件（RtWorkbench/RtActivityBar 等）设计为支持 slot 透传，保留子组件 API |

## Verification Strategy

### 每个 Phase 的双重检查

1. **编译/构建检查**
   - `npm run typecheck` 通过
   - `npm run build` 通过
   - `npm run test` 通过

2. **Diff 检查（手动）**
    - `git diff --stat` 查看改动范围
    - 业务组件的 `<script setup>` 部分不应有实质变更
    - 允许变更的脚本范围仅限：`shared/frontend/components/*`、应用级外层包装组件、`presentation/*` 纯 UI 映射文件
    - Store / API / Bridge 文件不应有变更
    - 新增文件允许出现在 `shared/frontend/components/`、`shared/frontend/themes/`、`shared/frontend/tokens/`、应用内 `presentation/`、以及样式相关目录

3. **视觉验证**
   - 启动 dev server，截图对比
   - 关键交互（Tab 切换、面板折叠、按钮点击）行为不变

### 自动化辅助

- 在 `compatibility.css` 中保留所有旧 token 别名 → 旧组件即使引用旧变量名也不会崩
- 新组件初始版本提供必要的可选 props 和默认 slot，最大限度保持向后兼容，但不为旧布局尺寸做隐式兜底

## Open Questions

1. **RtPanelSplit 拖拽实现**: 用原生 pointer events 还是引入轻量拖拽库？建议原生实现。
2. **Tab 状态持久化**: 是否需要记住上次打开的 Tab？建议 Phase 3 先不做，后续按需添加。
3. **@shared/motion 模块**: motion-controller 的共享运动控制模块 UI 是否也需要更新 token？建议跟随 Phase 5 一起处理。
4. **wind-daq 的 vue-router 去留**: 保留（点击 Tab 触发 router.push）还是彻底移除？**当前建议保留**，最小化业务逻辑影响。
5. **RtWorkbench 是否提供"不替换"模式**: 即作为纯布局容器，不强制子组件使用 RtActivityBar/RtSidebar 等。**建议是**——RtWorkbench 只定义 grid 区域，子组件完全可选。
6. **旧入口淘汰时机**: `index.css` 何时切到 VSCode 风格、`glass.css` 何时删除？建议在 6 个项目全部完成迁移并通过视觉回归后，单独做 Phase 7 收口。

## Execution Order

```
Phase 1 (parallel tokens/themes entrypoints)
    │ 新增 `vscode.css` / `vscode-dark.css` / `vscode-light.css`
    │ 不改现有 `index.css` 的视觉表现
    ▼
Phase 2 (layout components)
    │ 新增文件，零对现有代码影响
    ▼
Phase 3 (wind-daq) ──► Phase 4 (daq-t1603) ──► Phase 5 (motion-controller)
    │ 外层替换 + 内部保留
    │ 每个 Phase 都做 diff 检查确保 script 不动
    ▼
Phase 6 (interpolators) ──► 切换到 `vscode.css`，仅换色，最小改动
    │
    ▼
Phase 7 (optional cleanup)
    │ 6 个项目全部迁移完成后，再评估是否统一入口并删除 `glass.css`
    ▼
 Final Checkpoint
```

Phase 3/4/5 理论上可并行，但建议顺序执行以在标杆项目中发现共性问题。
