# WindLabX4 UI 一致性审查报告与整改计划

> **已废止（2026-06）：** 以 Cursor DAQ 为视觉对等目标的策略已废止，详见 `../../DESIGN.md` 与 `./README.md`。本文档保留作为历史审查记录；当前视觉目标改为 DESIGN.md 中定义的"工业仪表 + 现代感"，不再以 Cursor DAQ 为参考标准。

> 审查日期：2026-05-22
> 审查范围：`projects/WindLabX4/apps/desktop-wails/frontend` 全部 UI 组件与页面
> 参考标准：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\Ai Agent\Cursor DAQ\src\renderer\src`

---

## 一、审查概述

本次审查对 WindLabX4 项目的每个 UI 画面进行了逐个、细致的检查，从布局结构、色彩搭配、字体样式、交互效果、组件尺寸等方面与参考项目（Cursor DAQ）进行了全面对比。

### 审查范围

| 序号 | UI 区域 | 参考文件 | 目标文件 | 状态 |
|------|---------|----------|----------|------|
| 1 | 主壳结构 | `views/main/MainDashboardView.vue` | `views/main/MainDashboardView.vue` | 已审查 |
| 2 | 主视图容器 | `views/MainView.vue` | `views/MainView.vue` | 已审查 |
| 3 | 顶部栏 | `components/layout/MainTopBar.vue` | `components/layout/MainTopBar.vue` | 已审查 |
| 4 | 侧边导航栏 | `components/layout/AppRailNav.vue` | `components/layout/AppRailNav.vue` | 已审查 |
| 5 | 底部栏 | `components/layout/MainBottomBar.vue` | `components/layout/MainBottomBar.vue` | 已审查 |
| 6 | 设备侧边栏 | `components/main/DeviceSidebar.vue` | `components/main/DeviceSidebar.vue` | 已审查 |
| 7 | 设备概览面板 | `components/main/DeviceOverviewPanel.vue` | `components/main/DeviceOverviewPanel.vue` | 已审查 |
| 8 | 设备详情面板 | `components/main/DeviceDetailPanel.vue` + `.css` | `components/main/DeviceDetailPanel.vue` | 已审查 |
| 9 | 设置模态框 | `components/layout/GlobalSettingsModal.vue` + `.css` | `components/layout/GlobalSettingsModal.vue` | 已审查 |
| 10 | 运动控制页面 | `views/MotionView.vue` | `views/MotionView.vue` | 已审查 |
| 11 | 校准页面 | `views/CalibrationView.vue` | `views/CalibrationView.vue` | 已审查 |
| 12 | 遍历页面 | `views/TraversalView.vue` | `views/TraversalView.vue` | 已审查 |
| 13 | 日志查看器 | `views/LogViewer.vue` | `views/LogViewer.vue` | 已审查 |
| 14 | 主题系统 | `styles/tokens/*.css` + `styles/themes/*.css` | `styles/tokens/*.css` + `styles/themes/*.css` | 已审查 |
| 15 | 全局样式 | `style.css` | `styles.css` | 已审查 |
| 16 | 反馈组件 | `components/feedback/UiToastHost.vue` | `components/feedback/UiToastHost.vue` | 已审查 |

---

## 二、逐画面审查结果

### 2.1 主壳结构（MainDashboardView + MainView）

**审查结论：一致**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 布局结构 | `MainDashboardView` 拥有主壳，内嵌 `MainView` 作为容器 | 相同结构 | 无 |
| 页面切换 | `activePage` 本地状态控制 | 相同 | 无 |
| 页面嵌入 | Motion/Calibration/Traversal/Log 嵌入同一壳内 | 相同 | 无 |
| 底部栏渲染 | 仅 dashboard 页显示 | 相同 | 无 |

**差异点：无。结构已完全匹配。**

---

### 2.2 顶部栏（MainTopBar）

**审查结论：高度一致，存在微小差异**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 图标库 | `lucide-vue-next` | `@lucide/vue` | 导入路径不同，功能一致 |
| 主题切换标签 | `themeToggleLabel()` 函数 | 相同实现 | 无 |
| 视图模式控制 | `dashboardModes` 数组 + 按钮组 | 相同 | 无 |
| 样式类名 | Tailwind 工具类为主 | 相同 | 无 |
| `t` prop 类型 | `Record<string, string>`（必填） | `Record<string, string>`（可选，默认 `{}`） | **差异：目标项目 t 为可选** |

**差异点：**
1. `t` prop 在目标项目中为可选（`t?: Record<string, string>`），参考项目为必填。这可能导致 i18n 文本回退行为不一致。

---

### 2.3 侧边导航栏（AppRailNav）

**审查结论：一致**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 导航项顺序 | dashboard → motion → calibration → traversal → log | 相同 | 无 |
| 按钮尺寸 | 40x40px 圆形按钮 | 相同 | 无 |
| 激活状态 | 绿色高亮 + 图标变化 | 相同 | 无 |
| 设置按钮 | 底部固定 | 相同 | 无 |

**差异点：无。**

---

### 2.4 底部栏（MainBottomBar）

**审查结论：高度一致，存在微小差异**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 高度 | 64px | 64px | 无 |
| 控制按钮 | 开始/停止 + 录制按钮 | 相同 | 无 |
| 状态显示 | 采集状态 + 设备数 | 相同 | 无 |
| 时间显示 | 运行时间 + 系统时间 | 相同 | 无 |
| 样式 | `main-bottom-bar__status` / `main-bottom-bar__devices` | `main-bottom-bar__status-item` | **差异：类名不同** |
| `t` prop 类型 | 必填 | 可选（默认 `{}`） | **差异：同 TopBar** |
| 图标库 | `lucide-vue-next` | `@lucide/vue` | 导入路径不同 |

**差异点：**
1. 状态区域 CSS 类名不同：参考项目使用 `main-bottom-bar__status` 和 `main-bottom-bar__devices`，目标项目统一使用 `main-bottom-bar__status-item`。
2. `t` prop 为可选而非必填。

---

### 2.5 设备侧边栏（DeviceSidebar）

**审查结论：高度一致，存在微小差异**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 宽度 | `clamp(220px, 24vw, --layout-sidebar-width)` | 相同 | 无 |
| 设备列表 | 卡片式列表项 + 状态指示灯 | 相同 | 无 |
| 状态动画 | `breathe` 关键帧动画 | 相同 | 无 |
| 背景色 | `color-mix(in srgb, var(--bg-panel) 96%, transparent)` | 相同 | 无 |
| 样式位置 | 内联 `<style scoped>` | 内联 `<style scoped>` | 无 |
| props 类型 | `t: Record<string, string>`（必填） | `t?: Record<string, string>`（可选） | **差异：t 为可选** |
| 状态标签回退 | `props.t.connectedState` 等 | 硬编码英文回退（'Connected', 'Warning'） | **差异：回退文本不同** |
| 焦点样式 | `focus-visible:outline-none focus-visible:ring-2` 在按钮上 | 无焦点样式类 | **差异：缺少无障碍焦点样式** |

**差异点：**
1. `t` prop 为可选而非必填。
2. 状态标签回退文本不同：参考项目使用 i18n key，目标项目硬编码英文。
3. 设备列表按钮缺少 `focus-visible` 无障碍焦点样式。

---

### 2.6 设备概览面板（DeviceOverviewPanel）

**审查结论：一致**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 设备分组 | 按设备分组的通道卡片网格 | 相同 | 无 |
| 通道微卡片 | 显示值 + 单位 + 状态色 | 相同 | 无 |
| 警告状态 | 阈值超限变色 | 相同 | 无 |
| 空状态 | 无设备时提示 | 相同 | 无 |
| Tare 按钮 | 全部通道归一化按钮 | 相同 | 无 |

**差异点：无。**

---

### 2.7 设备详情面板（DeviceDetailPanel）

**审查结论：高度一致，存在差异**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 布局结构 | 图表 + 表格混合模式 | 相同 | 无 |
| 通道颜色 | `CHANNEL_COLORS` 数组 | 相同 | 无 |
| 图表选择器 | 弹出式图表可见性控制 | 相同 | 无 |
| Sparkline | 迷你柱状图历史 | 相同 | 无 |
| 样式文件 | 独立 `DeviceDetailPanel.css` | 样式内联在 `.vue` 文件中 | **差异：样式组织方式不同** |
| `getChannelVisualTokens` | 参考项目有独立函数 | 目标项目使用 `channelStyle()` 内联 | **差异：函数命名和组织不同** |

**差异点：**
1. 参考项目将样式提取到独立的 `.css` 文件，目标项目样式内联在 `.vue` 文件中。
2. 通道视觉 token 函数命名不同。

---

### 2.8 设置模态框（GlobalSettingsModal）

**审查结论：功能一致，实现方式有差异**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 模态框结构 | Teleport + Transition | 相同 | 无 |
| 数据保存设置 | 目录选择 + 文件前缀 + 自动启动 | 相同 | 无 |
| 自动停止条件 | 可折叠条件项（时长/大小/数量） | 相同 | 无 |
| 刷新率控制 | 滑块 + 实时预览 | 相同 | 无 |
| 目录选择 | `PickDirectory`（参考项目） | `PickDirectory`（Wails 后端） | 相同 |
| 样式文件 | 独立 `GlobalSettingsModal.css` | 样式内联在 `.vue` 文件中 | **差异：样式组织方式不同** |
| 输入字段 | `UiInput` 组件 | 原生 `<input>` 元素 | **差异：组件使用不同** |
| 验证错误 | `validationErrors` 对象（字段级） | `validationError` 字符串（全局） | **差异：验证粒度不同** |
| 条件项展开 | 完整可折叠交互 | 简化版，仅 `condition-item--enabled` 类控制 | **差异：折叠交互不完整** |

**差异点：**
1. 参考项目使用独立 `.css` 文件，目标项目样式内联。
2. 参考项目使用 `UiInput` 组件，目标项目使用原生 `<input>`。
3. 参考项目有字段级验证错误，目标项目只有全局验证错误。
4. 参考项目的停止条件项有完整的折叠/展开交互，目标项目简化了此交互。

---

### 2.9 运动控制页面（MotionView）

**审查结论：一致**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 页面壳 | `data-test="motion-shell"` | 相同 | 无 |
| 头部栏 | 标题 + 副标题 + 关闭按钮 | 相同 | 无 |
| embedded 模式 | 条件高度样式 | 相同 | 无 |
| 子组件 | `MotionControlPanel` | 相同 | 无 |
| 图标库 | `lucide-vue-next` | `@lucide/vue` | 导入路径不同 |

**差异点：仅图标库导入路径不同，功能一致。**

---

### 2.10 校准页面（CalibrationView）

**审查结论：目标项目实现更完整**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 页面结构 | 简单壳，内嵌 `CalibrationWindow` | 完整实现：设置面板 + 状态面板 + 控制按钮 | **目标项目更完整** |
| 校准类型 | 由子组件处理 | 五种类型标签页 | 目标项目更丰富 |
| 状态面板 | 由子组件处理 | 运行/暂停/完成状态指示 | 目标项目更丰富 |
| 控制按钮 | 由子组件处理 | 开始/暂停/恢复/停止/采集 | 目标项目更丰富 |

**差异点：目标项目的校准页面实现比参考项目更完整，参考项目仅为壳组件。无需整改。**

---

### 2.11 遍历页面（TraversalView）

**审查结论：目标项目实现更完整**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 页面结构 | 简单壳，内嵌 `TraversalMain` + `TraversalSettings` | 完整实现：工具栏 + 侧边栏 + 主内容区 | **目标项目更完整** |
| 网格配置 | 由子组件处理 | X/Y/Z 范围 + 步长输入 | 目标项目更丰富 |
| 状态指示 | 由子组件处理 | 状态点 + 进度摘要 | 目标项目更丰富 |
| 控制按钮 | 由子组件处理 | 启动/暂停/恢复/停止 | 目标项目更丰富 |

**差异点：目标项目的遍历页面实现比参考项目更完整，参考项目仅为壳组件。无需整改。**

---

### 2.12 日志查看器（LogViewer）

**审查结论：一致**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 头部 | 图标 + 标题 + 计数 | 相同 | 无 |
| 过滤 | 级别过滤按钮 + 搜索框 | 相同 | 无 |
| 条目显示 | 时间 + 级别徽章 + 来源 + 消息 | 相同 | 无 |
| 颜色 | 级别颜色映射（深色/浅色） | 相同 | 无 |
| 自动滚动 | 底部检测 + 自动滚动 | 相同 | 无 |
| 操作 | 暂停/清除/复制 | 相同 | 无 |
| embedded 模式 | `embedded-mode` 类 | 相同 | 无 |

**差异点：无。**

---

### 2.13 主题和样式系统

**审查结论：高度一致，存在微小差异**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| 颜色 token | `styles/tokens/color.css` | 相同 | 无 |
| 间距 token | `styles/tokens/spacing.css` | 相同 | 无 |
| 圆角 token | `styles/tokens/radius.css` | 相同 | 无 |
| 动效 token | `styles/tokens/motion.css` | 相同 | 无 |
| 排版 token | `styles/tokens/typography.css` | 相同 | 无 |
| 布局 token | `styles/tokens/layout.css` | 相同 | 无 |
| 深色主题 | `styles/themes/dark.css` | 相同 | 无 |
| 浅色主题 | `styles/themes/light.css` | 相同 | 无 |
| 玻璃效果 | `styles/glass.css` | 相同 | 无 |
| 全局样式 | `style.css` | `styles.css` | **差异：文件名不同** |
| CSS 导入顺序 | color → spacing → radius → motion → typography → layout → dark → light | color → spacing → typography → motion → layout → radius → dark → light → glass | **差异：导入顺序不同** |
| 额外导入 | 无 | `@import './styles/glass.css'` | **差异：目标项目额外导入 glass.css** |
| button 重置 | 无 | `button { border: 0; font: inherit; cursor: pointer; }` | **差异：目标项目有 button 重置样式** |

**差异点：**
1. 全局样式文件名不同（`style.css` vs `styles.css`），不影响功能。
2. CSS token 导入顺序不同，可能导致层叠优先级差异。
3. 目标项目额外导入了 `glass.css` 到全局样式中。
4. 目标项目添加了 button 元素重置样式。

---

### 2.14 反馈组件（UiToastHost）

**审查结论：一致**

| 维度 | 参考项目 | 目标项目 | 差异 |
|------|----------|----------|------|
| Toast 容器 | 固定定位右上角 | 相同 | 无 |
| 级别样式 | success/warning/error/info | 相同 | 无 |
| 动画 | slide-in 从右侧 | 相同 | 无 |
| 自动消失 | 定时器控制 | 相同 | 无 |
| 深色/浅色 | 两套颜色方案 | 相同 | 无 |
| 主题检测 | `html:not(.dark)` | `:root[data-theme='light']` | **差异：主题选择器不同** |

**差异点：**
1. 主题选择器不同：参考项目使用 `html:not(.dark)`，目标项目使用 `:root[data-theme='light']`。功能等价，但选择器不一致。

---

## 三、差异汇总与分类

### 3.1 无差异（完全一致）

| UI 区域 | 状态 |
|---------|------|
| 主壳结构 | 完全一致 |
| 侧边导航栏 | 完全一致 |
| 设备概览面板 | 完全一致 |
| 运动控制页面 | 完全一致 |
| 日志查看器 | 完全一致 |

### 3.2 微小差异（不影响功能，建议统一）

| 编号 | UI 区域 | 差异描述 | 影响等级 |
|------|---------|----------|----------|
| D1 | 多个组件 | `t` prop 为可选而非必填 | 低 |
| D2 | 多个组件 | 图标库导入路径不同（`lucide-vue-next` vs `@lucide/vue`） | 低 |
| D3 | MainBottomBar | CSS 类名不同（`__status` vs `__status-item`） | 低 |
| D4 | 全局样式 | 文件名不同（`style.css` vs `styles.css`） | 低 |
| D5 | 全局样式 | CSS 导入顺序不同 | 低 |
| D6 | UiToastHost | 主题选择器不同（`html:not(.dark)` vs `:root[data-theme='light']`） | 低 |

### 3.3 中等差异（影响代码质量或可维护性）

| 编号 | UI 区域 | 差异描述 | 影响等级 |
|------|---------|----------|----------|
| D7 | DeviceSidebar | 缺少 `focus-visible` 无障碍焦点样式 | 中 |
| D8 | DeviceSidebar | 状态标签回退文本硬编码英文而非 i18n | 中 |
| D9 | DeviceDetailPanel | 样式内联而非独立 CSS 文件 | 中 |
| D10 | GlobalSettingsModal | 样式内联而非独立 CSS 文件 | 中 |
| D11 | GlobalSettingsModal | 使用原生 `<input>` 而非 `UiInput` 组件 | 中 |
| D12 | GlobalSettingsModal | 验证错误为全局而非字段级 | 中 |
| D13 | GlobalSettingsModal | 停止条件折叠交互不完整 | 中 |

### 3.4 目标项目更优（无需整改）

| 编号 | UI 区域 | 描述 |
|------|---------|------|
| B1 | CalibrationView | 目标项目实现比参考项目更完整 |
| B2 | TraversalView | 目标项目实现比参考项目更完整 |
| B3 | GlobalSettingsModal | 目标项目使用 Wails `PickDirectory` 替代 Electron IPC |

---

## 四、UI 整改计划

### 4.1 整改原则

1. **不改变功能行为**：所有整改仅涉及代码组织、样式一致性和可维护性，不改变用户可见的功能行为。
2. **参考项目为视觉标准**：所有样式和布局以参考项目为准。
3. **保持目标项目优势**：对于目标项目实现更优的部分（B1-B3），保持不变。
4. **渐进式整改**：按优先级从高到低依次执行，每次整改后验证构建和功能。

### 4.2 整改任务清单

#### P1 - 高优先级（影响用户体验或代码质量）

| 任务编号 | 任务名称 | 涉及文件 | 具体内容 | 预计工时 | 完成标准 |
|----------|----------|----------|----------|----------|----------|
| T1 | 统一 `t` prop 类型 | `MainTopBar.vue`, `MainBottomBar.vue`, `DeviceSidebar.vue` | 将 `t?: Record<string, string>` 改为 `t: Record<string, string>`，确保所有调用方传入 `t` | 1h | 所有组件 `t` prop 为必填，构建无警告 |
| T2 | 统一图标库导入路径 | 所有使用图标的 `.vue` 文件 | 将 `@lucide/vue` 统一为 `lucide-vue-next`（或反之，取决于项目依赖） | 2h | 所有图标导入路径一致，构建通过 |
| T3 | 补充无障碍焦点样式 | `DeviceSidebar.vue` | 为设备列表按钮添加 `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--focus-ring)] focus-visible:ring-inset` 类 | 0.5h | Tab 键导航时设备项有可见焦点环 |
| T4 | 统一状态标签回退文本 | `DeviceSidebar.vue` | 将硬编码英文回退改为 i18n key 回退（如 `props.t?.connectedState \|\| 'Connected'`） | 0.5h | 所有状态标签使用 i18n key 回退 |

#### P2 - 中优先级（影响代码可维护性）

| 任务编号 | 任务名称 | 涉及文件 | 具体内容 | 预计工时 | 完成标准 |
|----------|----------|----------|----------|----------|----------|
| T5 | 提取 DeviceDetailPanel 样式 | `DeviceDetailPanel.vue` → `DeviceDetailPanel.css` | 将 `<style scoped>` 内容提取到独立 `.css` 文件，与参考项目保持一致 | 1h | 样式文件独立，组件引用正确 |
| T6 | 提取 GlobalSettingsModal 样式 | `GlobalSettingsModal.vue` → `GlobalSettingsModal.css` | 将 `<style scoped>` 内容提取到独立 `.css` 文件，与参考项目保持一致 | 1.5h | 样式文件独立，组件引用正确 |
| T7 | 统一 GlobalSettingsModal 输入组件 | `GlobalSettingsModal.vue` | 将原生 `<input>` 替换为 `UiInput` 组件，与参考项目保持一致 | 1h | 所有输入字段使用 `UiInput` 组件 |
| T8 | 完善 GlobalSettingsModal 验证 | `GlobalSettingsModal.vue` | 将全局 `validationError` 改为字段级 `validationErrors` 对象 | 1.5h | 每个输入字段有独立的错误提示 |
| T9 | 完善停止条件折叠交互 | `GlobalSettingsModal.vue` | 添加条件项的折叠/展开交互，与参考项目一致 | 2h | 条件项可折叠，展开时显示详细配置 |

#### P3 - 低优先级（代码风格统一）

| 任务编号 | 任务名称 | 涉及文件 | 具体内容 | 预计工时 | 完成标准 |
|----------|----------|----------|----------|----------|----------|
| T10 | 统一 CSS 导入顺序 | `styles.css` | 按参考项目顺序排列：color → spacing → radius → motion → typography → layout → dark → light → glass | 0.5h | 导入顺序与参考项目一致 |
| T11 | 统一主题选择器 | `UiToastHost.vue` | 将 `:root[data-theme='light']` 改为 `html:not(.dark)`，与参考项目一致 | 0.5h | Toast 浅色主题样式选择器一致 |
| T12 | 统一 BottomBar CSS 类名 | `MainBottomBar.vue` | 将 `main-bottom-bar__status-item` 改为 `main-bottom-bar__status` 和 `main-bottom-bar__devices` | 0.5h | 类名与参考项目一致 |

### 4.3 整改时间线

```
Phase 1 (P1 任务): 4 小时
├── T1: 统一 t prop 类型 (1h)
├── T2: 统一图标库导入路径 (2h)
├── T3: 补充无障碍焦点样式 (0.5h)
└── T4: 统一状态标签回退文本 (0.5h)

Phase 2 (P2 任务): 7 小时
├── T5: 提取 DeviceDetailPanel 样式 (1h)
├── T6: 提取 GlobalSettingsModal 样式 (1.5h)
├── T7: 统一 GlobalSettingsModal 输入组件 (1h)
├── T8: 完善 GlobalSettingsModal 验证 (1.5h)
└── T9: 完善停止条件折叠交互 (2h)

Phase 3 (P3 任务): 1.5 小时
├── T10: 统一 CSS 导入顺序 (0.5h)
├── T11: 统一主题选择器 (0.5h)
└── T12: 统一 BottomBar CSS 类名 (0.5h)

总计: 12.5 小时
```

### 4.4 验证清单

每个 Phase 完成后执行：

1. `cd projects/WindLabX4/apps/desktop-wails/frontend && npm run build` — 构建通过
2. 视觉对比：逐个页面与参考项目对比截图
3. 功能测试：确保所有交互行为正常
4. 主题切换：深色/浅色主题切换正常
5. 响应式测试：窗口大小变化时布局正常

---

## 五、总结

### 5.1 整体评估

WindLabX4 项目的 UI 实现与参考项目（Cursor DAQ）**高度一致**。主壳结构、导航、设备管理、日志查看等核心 UI 区域已完全匹配参考项目。校准页面和遍历页面的实现甚至**优于**参考项目。

### 5.2 主要发现

1. **结构一致性高**：主壳、导航、页面嵌入等核心结构已完全匹配。
2. **样式组织差异**：部分组件的样式内联在 `.vue` 文件中，而参考项目使用独立 `.css` 文件。
3. **Props 类型差异**：部分组件的 `t` prop 为可选，参考项目为必填。
4. **组件使用差异**：设置模态框中使用原生 `<input>` 而非 `UiInput` 组件。
5. **无障碍差异**：设备侧边栏缺少 `focus-visible` 焦点样式。

### 5.3 整改建议

建议按 P1 → P2 → P3 的优先级顺序执行整改，总预计工时约 **12.5 小时**。整改完成后，WindLabX4 项目的 UI 将与参考项目达到完全一致。

### 5.4 无需整改项

以下区域已完全匹配或目标项目实现更优，无需整改：

- 主壳结构（MainDashboardView + MainView）
- 侧边导航栏（AppRailNav）
- 设备概览面板（DeviceOverviewPanel）
- 运动控制页面（MotionView）
- 日志查看器（LogViewer）
- 校准页面（CalibrationView）— 目标项目更完整
- 遍历页面（TraversalView）— 目标项目更完整

---

*本报告由 AI 自动生成，基于对参考项目和目标项目的逐文件对比分析。*
*审查完成日期：2026-05-22*
