# DAQ Professional - UI Design Specification

> 本文档描述 DAQ Professional 的完整 UI 设计规范，可用于指导新软件开发时的视觉还原和组件复用。

---

## 1. 设计风格总览

**风格定位：** 工业 SCADA 仪表盘，深色优先，数据密集型

- 深色主题为默认，亮色主题为可选
- 以深蓝色调为主背景，搭配高对比度数据展示
- 数据优先：数值使用特大加粗字体，标签使用小号灰色字体
- Header/Footer 使用毛玻璃营造层次感，主内容区使用实色背景确保可读性和性能
- 无第三方 UI 组件库，全部自建设计系统
- 最小圆角（2~4px），克制动画（120~260ms 仅用于 hover/focus/展开收起）

---

## 2. 整体布局

### 2.1 应用骨架 (AppShell)

```
+------------------------------------------------------------------+
|  Header (48px)  glass-header                                      |
|  Logo(DAQ.PRO) | Nav Tabs | Status Pill | Theme | Locale | Ver   |
+------+--------+--------------------------------------------------+
| Rail | Side   |  Canvas (主内容区，flex: 1)                       |
| 56px | 220px  |                                                   |
|      |        |  [Toolbar] (可选, 40px)                            |
| Dash | Device |  [Main Content]                                   |
| Mot  | List   |    Dashboard / Motion / Calibration / Traversal   |
| Cal  |        |                                                   |
| Trav |        |                                                   |
|      |        |                                                   |
| [⚙]  |        |                                                   |
+------+--------+--------------------------------------------------+
|  StatusBar (32px)  glass-footer                                   |
|  Status | Devices | Elapsed | Clock                               |
+------------------------------------------------------------------+
```

**布局结构：**
- 外层：`flex-direction: column`，`height: 100vh`
- 工作区：`flex-direction: row`，`flex: 1`
- Canvas：`flex: 1`，`min-width: 320px`

**布局规则（桌面固定布局，无响应式断点）：**

| 规则 | 值 |
|---|---|
| 最小窗口 | 1280x720 — 低于此尺寸显示警告，不做重排 |
| 目标窗口 | 1920x1080 — 设计参考基准 |
| 侧边栏 | 可折叠至图标模式（56px），快捷键切换 |

**关键尺寸常量：**

| Token | 值 | 说明 |
|---|---|---|
| `--layout-header-height` | 48px | 顶部导航栏高度 |
| `--layout-rail-width` | 56px | 左侧图标导航宽度 |
| `--layout-sidebar-width` | 220px | 侧边栏宽度 |
| `--layout-toolbar-height` | 40px | 工具栏高度（可选） |
| `--layout-footer-height` | 32px | 底部状态栏高度 |

### 2.2 导航体系

**路由方案：** Vue Router 4（hash mode）

| 路径 | 视图 | 说明 |
|---|---|---|
| `/` | MainDashboardView | 主仪表盘 |
| `/calibration` | CalibrationView | 校准模块（含子路由） |
| `/calibration/five-hole` | FiveHoleCalView | 五孔探针校准 |
| `/calibration/three-hole` | ThreeHoleCalView | 三孔探针校准 |
| `/calibration/total-pressure` | TotalPressureCalView | 总压校准 |
| `/calibration/total-temperature` | TotalTempCalView | 总温校准 |
| `/traversal` | TraversalView | 遍历测试模块 |
| `/motion` | MotionView | 运动控制模块（可独立窗口） |

**Rail 导航（AppRailNav）：** 56px 宽的垂直图标栏，从上到下依次为 Dashboard、Motion、Calibration、Traversal，底部为 Settings 齿轮图标。

### 2.3 Dashboard 网格

```css
.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  grid-template-rows: repeat(4, 1fr);
  gap: 0.75rem;
}
```

---

## 3. 设计令牌 (Design Tokens)

所有设计令牌通过 CSS Custom Properties 定义，位于 `src/renderer/src/styles/tokens/`。

### 3.1 色彩系统

#### 表面层级（暗色主题）

| Token | 值 | 用途 |
|---|---|---|
| `--surface-0` / `--bg-app` | `#0f172a` | 应用背景 |
| `--surface-1` / `--bg-canvas` | `#111c31` | 画布背景 |
| `--surface-2` / `--bg-panel` | `#172338` | 面板背景 |
| `--surface-3` / `--bg-panel-strong` | `#1e293b` | 强调面板 / hover 背景 |

#### 表面层级（亮色主题）

| Token | 值 |
|---|---|
| `--bg-app` | `#f8fafc` |
| `--bg-canvas` | `#f8fafc` |
| `--bg-panel` | `#ffffff` |
| `--bg-panel-strong` | `#f8fafc` |

#### 文字层级

| Token | 暗色 | 亮色 | 用途 |
|---|---|---|---|
| `--text-primary` | `#e2e8f0` | `#0f172a` | 主要文字 |
| `--text-secondary` | `#cbd5e1` | `#334155` | 次要文字 |
| `--text-muted` | `#94a3b8` | `#94a3b8` | 辅助/禁用文字 |

#### 语义色

| Token | 值 | 用途 |
|---|---|---|
| `--accent-primary` | `#56ccf2` (暗) / `#22c55e` (亮) | 主强调色（对比度 ≥ 5:1） |
| `--accent-success` | `#22c55e` | 成功状态 |
| `--accent-warning` | `#f59e0b` | 警告状态 |
| `--accent-danger` | `#ef5b47` | 危险/错误状态 |
| `--accent-info` | `#56ccf2` | 信息提示 |

#### 品牌色

| Token | 值 |
|---|---|
| `--color-brand` | `#2f88ff` |
| `--color-brand-strong` | `#1967d2` |

#### 运动轴颜色

| 轴 | 颜色 Token | 暗色值 | 柔和色（20%透明度） |
|---|---|---|---|
| X | `--axis-x` | `#60a5fa` | `rgba(59,130,246,0.2)` |
| Y | `--axis-y` | `#a78bfa` | `rgba(139,92,246,0.2)` |
| Z | `--axis-z` | `#22d3ee` | `rgba(6,182,212,0.2)` |
| U | `--axis-u` | `#fbbf24` | `rgba(245,158,11,0.2)` |

#### 边框

| Token | 暗色 | 亮色 |
|---|---|---|
| `--border-default` | `#334155` | `#e2e8f0` |
| `--border-strong` | `#475569` | `#cbd5e1` |

#### 通道颜色

核心方案（8 色，覆盖 90% 场景，色盲安全）：

```
#3B82F6  (蓝)   #22C55E  (绿)   #F59E0B  (琥珀)  #A855F7  (紫)
#EC4899  (粉)   #06B6D4  (青)   #F97316  (橙)    #8B5CF6  (堇紫)
```

扩展方案（9-16 通道时启用）：

```
#10B981  #EF4444  #6366F1  #D946EF
#84CC16  #F43F5E  #0EA5E9  #FACC15
```

超过 16 通道时循环使用核心方案 + 虚线样式区分（`stroke-dasharray: 6 3`）。

### 3.2 间距系统

4px 基础单位：

| Token | 值 |
|---|---|
| `--space-1` | 4px |
| `--space-2` | 8px |
| `--space-3` | 12px |
| `--space-4` | 16px |
| `--space-5` | 20px |
| `--space-6` | 24px |
| `--space-8` | 32px |
| `--space-10` | 40px |
| `--space-12` | 48px |
| `--space-16` | 64px |

### 3.3 圆角

| Token | 值 |
|---|---|
| `--radius-xs` | 2px |
| `--radius-sm` | 3px |
| `--radius-md` | 4px |
| `--radius-pill` | 999px |

### 3.4 排版系统

#### 字体

```css
--font-family-sans: 'Microsoft YaHei UI', 'Microsoft YaHei', 'PingFang SC', 'Segoe UI', sans-serif;
--font-family-mono: 'JetBrains Mono', 'Cascadia Code', 'SFMono-Regular', monospace;
```

外部字体：Google Fonts - Inter (400-700)、JetBrains Mono (400-600)。

#### 字号比例（1.125 ratio）

| Token | 值 | 用途 |
|---|---|---|
| `--font-size-micro` | 10px | 最小标注 |
| `--font-size-2xs` | 11px | 辅助说明 |
| `--font-size-xs` | 12px | 标签、表头 |
| `--font-size-sm` | 14px | 小正文 |
| `--font-size-base` | 15px | 默认正文 |
| `--font-size-lg` | 16px | 大正文 |
| `--font-size-xl` | 18px | 小标题 |
| `--font-size-2xl` | 20px | 面板标题 |
| `--font-size-3xl` | 24px | 大数值展示 |

#### 字重

| Token | 值 |
|---|---|
| `--font-weight-regular` | 400 |
| `--font-weight-medium` | 500 |
| `--font-weight-semibold` | 600 |
| `--font-weight-bold` | 700 |
| `--font-weight-black` | 800 |

#### 数据展示语义 Token

| Token | 尺寸 | 字重 | 用途 |
|---|---|---|---|
| `--type-dashboard-data-xl` | 24px | 800 | 通道大数值 |
| `--type-dashboard-data-lg` | 20px | 800 | 次要大数值 |
| `--type-dashboard-data` | 18px | 800 | 普通数值 |
| `--type-dashboard-data-sm` | 16px | 800 | 小数值 |
| `--type-dashboard-label` | 12px | 600 | 字段标签 |
| `--type-dashboard-caption` | 11px | 500 | 辅助说明 |
| `--type-dashboard-title` | 20px | 700 | 面板标题 |

### 3.5 动效

| Token | 值 | 用途 |
|---|---|---|
| `--motion-fast` | 120ms | 快速反馈（hover、active） |
| `--motion-base` | 180ms | 基础过渡（展开、收起） |
| `--motion-slow` | 260ms | 慢速过渡（页面切换） |

缓动曲线：
- `--easing-standard`: `cubic-bezier(0.2, 0, 0, 1)` — 通用标准
- `--easing-emphasis`: `cubic-bezier(0.18, 0.7, 0.2, 1)` — 强调进入
- `--easing-exit`: `cubic-bezier(0.4, 0, 1, 1)` — 退出

---

## 4. 毛玻璃效果 (Glassmorphism)

毛玻璃仅用于低数据密度区域（Header、Footer），主内容面板使用实色背景以确保数值可读性和渲染性能。

系统定义 5 级毛玻璃工具类：

| 类名 | 暗色背景 | blur | 用途 |
|---|---|---|---|
| `.glass` | `rgba(30,41,59,0.7)` | 12px | 通用玻璃面板 |
| `.glass-header` | `rgba(30,41,59,0.75)` | 12px | 顶部导航栏 |
| `.glass-sidebar` | `rgba(15,23,42,0.6)` | 12px | 侧边栏 |
| `.glass-rail` | `rgba(15,23,42,0.5)` | 10px | 图标导航栏 |
| `.glass-footer` | `rgba(30,41,59,0.75)` | 12px | 底部状态栏 |

所有玻璃效果在亮色模式下自动切换为白色半透明背景（`rgba(255,255,255,0.5~0.8)`）。

**边框：** 暗色 `rgba(255,255,255,0.05~0.08)`，亮色 `rgba(0,0,0,0.05)`。

### 特效

```css
/* 状态脉冲动画 */
.status-pulse { animation: status-pulse 2s infinite; }

/* 通道卡片 — 实色背景，不用 blur */
.channel-card {
  background: var(--bg-panel);
  /* hover: translateY(-2px) + 边框高亮 */
}
```

---

## 5. 波形显示规范

### 5.1 图表引擎

**ECharts 6 + vue-echarts**，自定义深色主题。

### 5.2 基础尺寸

- 宽度：自适应容器（flex: 1）
- 默认高度：240px
- 历史数据点：100 点滚动窗口（可配置至 1000）

### 5.3 网格系统

```
主网格 (major): 实线, rgba(255,255,255,0.06), 每 50px
次网格 (minor): 实线, rgba(255,255,255,0.02), 每 10px
轴线: rgba(255,255,255,0.1)
```

### 5.4 触发系统

| 模式 | 行为 |
|---|---|
| Auto | 自动同步，无稳定波形时自由滚动 |
| Normal | 满足触发条件后捕获并冻结，等待下一次触发 |
| Single | 捕获一次后停止 |
| 触发源 | 任意通道 |
| 触发边沿 | 上升沿 / 下降沿 |
| 触发电平 | 用户可拖动设置 |

### 5.5 光标交互

| 光标类型 | 快捷键 | 显示信息 |
|---|---|---|
| 单光标 | `C` | t 值 + 各通道瞬时值 |
| 双光标 (Δ) | `D` | Δt、ΔV、频率、周期 |
| 光标样式 | 1px 虚线 + 悬浮读数框 | 读数框跟随光标 |

### 5.6 缩放与平移

- **缩放**：鼠标滚轮（以光标位置为中心），X 轴独立缩放
- **平移**：鼠标中键拖动 或 `Ctrl` + 左键拖动
- **重置**：双击 或 `R` 键恢复默认视窗
- **框选缩放**：`Shift` + 左键拖动选区

### 5.7 通道显示模式

| 模式 | 说明 |
|---|---|
| 叠加 (Overlay) | 所有通道在同一坐标区，Y 轴自动偏移 |
| 分离 (Split) | 每个通道独立子图，均分垂直空间 |
| 快捷键 | `1`-`8` 切换通道可见性 |

### 5.8 数据线样式

- 线宽：1.5px（活跃），1px（非活跃/冻结）
- 采样点标记：默认隐藏，缩放到逐点时自动显示为 3px 圆点
- 颜色：使用通道颜色核心方案
- 空闲区（无数据）：显示为灰色虚线

---

## 6. UI 组件库

### 6.1 基础组件清单

所有组件位于 `src/renderer/src/components/ui/`。

#### UiButton

| Prop | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `variant` | `'primary' \| 'secondary' \| 'danger' \| 'ghost'` | `'primary'` | 变体 |
| `size` | `'sm' \| 'md'` | `'md'` | 尺寸 |
| `disabled` | `boolean` | `false` | 禁用 |

**视觉规格：**
- `md`: `min-height: 34px`，`padding: 0 16px`
- `sm`: `min-height: 28px`，`padding: 0 12px`
- `border-radius: 3px`
- `font-size: 14px (md) / 12px (sm)`，`font-weight: 600`
- 过渡：所有属性 120ms `easing-standard`

**变体色彩：**

| Variant | 背景 | 文字 | 边框 |
|---|---|---|---|
| primary | `--accent-primary` | `#fff` | 主色 50% 混合 |
| secondary | `--bg-panel` | `--text-secondary` | `--border-default` |
| danger | `--accent-danger` | `#fff` | 危险色 55% 混合 |
| ghost | transparent | `--text-muted` | transparent |

**交互状态：**
- hover：背景加深/加亮
- focus-visible：`box-shadow: 0 0 0 1px focus-ring, 0 0 0 3px focus-ring-soft`
- disabled：`opacity: 0.55`，`cursor: not-allowed`

#### UiInput

文本输入框，支持前缀/后缀插槽。数值输入使用 `--font-mono` + 右对齐。

#### UiSelect

下拉选择框。

#### UiToggle

开关切换组件。

#### UiPanel

面板容器，支持标题/副标题和插槽。

| Prop | 类型 | 默认值 |
|---|---|---|
| `title` | `string` | `''` |
| `subtitle` | `string` | `''` |
| `strong` | `boolean` | `false` |
| `padded` | `boolean` | `true` |

**视觉规格：**
- `background: var(--bg-panel)` （实色，不使用毛玻璃）
- `border-radius: var(--radius-md)` (4px)
- 标题栏：底部边框分隔，`padding: 12px 16px`
- 标题：14px，font-black，`--text-primary`
- 副标题：10px，大写，字间距 0.14em，`--text-muted`

#### UiMetricCard

数据指标卡片，展示标签/值/提示信息，支持 tone 变体（success/warning/danger）。

#### UiSectionHeader

区域分隔标题。

#### UiStatusBadge

状态徽章，支持 tone 变体：neutral、success、warning、danger、info。

### 6.2 布局组件

位于 `src/renderer/src/components/layout/`。

| 组件 | 说明 |
|---|---|
| `AppShell` | 应用骨架，提供 header/rail/sidebar/toolbar/default/statusbar 插槽 |
| `AppRailNav` | 56px 垂直图标导航 |
| `AppContextSidebar` | 通用侧边栏包装器 |
| `AppToolbar` | 可复用工具栏行 |
| `AppStatusBar` | 可复用状态栏 |
| `MainTopBar` | 48px 顶部栏（Logo + 导航 + 状态 + 主题切换） |
| `MainBottomBar` | 32px 底部栏（状态 + 设备 + 计时 + 时钟） |

### 6.3 业务组件

| 组件 | 路径 | 说明 |
|---|---|---|
| `DeviceOverviewPanel` | `components/main/` | 设备概览网格 |
| `DeviceDetailPanel` | `components/main/` | 设备详情（图表/表格/混合视图） |
| `DeviceSidebar` | `components/main/` | 设备列表侧边栏 |
| `RealtimeChart` | `components/` | ECharts 实时波形图 |
| `MotionControlPanel` | `components/` | 运动控制面板 |
| `CalibrationWindow` | `components/calibration/` | 校准窗口路由 |
| `FiveHoleMain` | `components/calibration/five-hole/` | 五孔探针校准主界面 |
| `FiveHoleSettings` | `components/calibration/five-hole/` | 五孔校准参数设置 |
| `ThreeHoleMain` | `components/calibration/three-hole/` | 三孔探针校准主界面 |
| `TotalPressureMain` | `components/calibration/total-pressure/` | 总压校准主界面 |
| `TotalTemperatureMain` | `components/calibration/total-temperature/` | 总温校准主界面 |
| `TraversalMain` | `components/traversal/` | 遍历测试主界面 |
| `DeviceManagementDrawer` | `components/` | 设备管理抽屉（滑出面板） |
| `GlobalSettingsModal` | `components/` | 全局设置弹窗 |

### 6.4 反馈组件

| 组件 | 说明 |
|---|---|
| `UiToastHost` | 全局 toast 通知 |
| `UiConfirmDialog` | 全局确认对话框 |

### 6.5 图标组件

位于 `components/icons/`，全部为 SVG 手绘组件：

- `IconDashboard` — 仪表盘
- `IconMotion` — 运动控制
- `IconCalibrationFiveHole` — 五孔校准
- `IconCalibrationThreeHole` — 三孔校准
- `IconCalibrationTotalPressure` — 总压校准
- `IconCalibrationTotalTemperature` — 总温校准
- `IconTraversal` — 遍历测试

图标库额外使用 **Lucide Vue Next**（Sun、Moon、Play、Square、Timer、Settings、Activity 等）。

---

## 7. 页面结构详解

### 7.1 主仪表盘 (Dashboard)

```
DeviceSidebar          DeviceOverviewPanel / DeviceDetailPanel
+------------------+   +------------------------------------------+
| 设备列表          |   |  [工具栏] 视图切换: 概览 | 图表 | 表格     |
| ▸ DAQ-P-1604 #1  |   +------------------------------------------+
|   18通道 | 已连接  |   |  概览模式: 4x4 网格                       |
| ▸ DAQ-T-1603 #1  |   |    每格显示一个通道的实时数值               |
|   16通道 | 离线    |   |    数值: 24px black                       |
| ▸ 模拟设备        |   |    标签: 12px semibold                    |
|                  |   |    单位: 11px muted                       |
|                  |   +------------------------------------------+
|                  |   |  图表模式: RealtimeChart (ECharts)         |
| [+添加设备]       |   |    自适应宽度，默认 240px 高               |
+------------------+   |    历史 100 点滚动窗口                      |
                       +------------------------------------------+
```

### 7.2 校准模块 (Calibration)

```
CalibrationWindow
+------------------------------------------+
| [返回] 校准类型选择                        |
|  ▸ 五孔探针  ▸ 三孔探针                   |
|  ▸ 总压探针  ▸ 总温探针                   |
+------------------------------------------+
| CalibrationHome 或 具体校准界面            |
|                                          |
| FiveHoleMain:                            |
| +----- Settings -----+--- Main Chart ---+|
| | 校准参数配置         |   校准曲线图      ||
| | α/β角度范围          |   ECharts        ||
| | 采样参数             |                  ||
| | 运动控制             +------------------+
| | [开始校准]           | RealtimeDataPanel |
| +---------------------+  实时压力/系数    |
+------------------------------------------+
```

### 7.3 运动控制 (Motion)

```
MotionControlPanel
+------------------------------------------+
| 轴控制卡片 (每轴一个)                      |
| +--------------------------------------+ |
| | X轴  #60a5fa                         | |
| | 当前位置: 123.456 mm                  | |
| | 目标: [____] mm   [移动] [微调] [回零]| |
| +--------------------------------------+ |
| | Y轴  #a78bfa                         | |
| +--------------------------------------+ |
| | Z轴  #22d3ee                         | |
| +--------------------------------------+ |
| | U轴  #fbbf24                         | |
| +--------------------------------------+ |
| [紧急停止]                                |
+------------------------------------------+
```

### 7.4 遍历测试 (Traversal)

```
TraversalMain + TraversalSettings
+------------------------------------------+
| [设置面板]           [测试主界面]           |
| 遍历路径配置          实时数据展示           |
| PRB文件导入          插值结果图表           |
| 校准CSV导入          进度显示               |
| 插值算法选择                               |
+------------------------------------------+
```

---

## 8. 错误与空态规范

### 8.1 设备状态

| 状态 | 视觉 | 行为 |
|---|---|---|
| 在线 | 绿色圆点 + "Online" | 正常显示数据 |
| 离线 | 灰色圆点 + "Offline" | 数据显示 `--`，操作按钮禁用 |
| 错误 | 红色圆点 + 错误信息 | 显示最后已知值（灰色）+ 错误描述 |
| 连接中 | 黄色脉冲圆点 + "Connecting..." | 数据显示 `...`，3s 超时后标记离线 |

### 8.2 空态

| 场景 | 显示 |
|---|---|
| 无设备 | 插图 + "请添加设备" + 添加按钮 |
| 无数据 | 通道卡片显示 `--` + "等待数据" |
| 无校准记录 | "尚无校准数据" + 开始校准按钮 |

### 8.3 错误反馈

| 级别 | 组件 | 行为 |
|---|---|---|
| 信息 | UiToast (info) | 右下角 3s 自动消失 |
| 警告 | UiToast (warning) | 右下角 5s 自动消失 |
| 错误 | UiToast (error) | 右下角，需手动关闭 |
| 严重 | UiConfirmDialog | 模态弹窗，需确认操作（如设备断连、采集失败） |

---

## 9. 主题切换

**切换方式：** 点击 Header 中的太阳/月亮图标。

**实现机制：**
1. `themeStore` 管理状态，持久化到 `localStorage`
2. 通过 `document.documentElement.dataset.theme` 切换
3. CSS 选择器 `:root[data-theme='light']` 覆盖暗色默认值
4. Tailwind 使用 `class` 策略的 `dark:` 变体

**i18n 切换：** 点击 Header 中的语言标签，支持中文 (`zh`) 和英文 (`en`)。翻译表内联在 `i18nStore` 中，约 200 个 key。

---

## 10. 状态管理架构

**状态管理库：** Pinia

| Store | 职责 |
|---|---|
| `deviceStore` | 设备管理、通道数据、图表选区、去皮 |
| `motionStore` | 运动控制器 CRUD、状态轮询 |
| `calibrationStore` | 校准任务全生命周期 |
| `traversalStore` | 遍历测试全生命周期 |
| `storageStore` | 录制设置和状态 |
| `themeStore` | 主题切换 |
| `i18nStore` | 国际化 |
| `feedbackStore` | Toast 和确认对话框 |

**数据流（高层）：**

```
Go Backend (Wails Bindings / WebSocket)
  → Vue Composable / Store
    → Pinia State Update
      → Vue Reactivity → UI 更新
```

---

## 11. 后端架构

系统使用 **Wails** 框架，Go 后端 + Vue 3 前端：

| 层 | 技术 | 说明 |
|---|---|---|
| 桌面框架 | Wails | Go → JS 绑定，原生窗口 |
| 前端 | Vue 3 + Vite | 纯 SPA，Wails 嵌入 |
| 后端 | Go (hexagonal) | 业务逻辑在 core/usecase，硬件在 adapters |
| 通信 | Wails Bindings | 结构体方法自动暴露为 JS 调用 |
| 实时数据 | WebSocket (可选) | 高频波形数据推送 |

---

## 12. 新项目实施指南

### 12.1 技术栈

| 层级 | 技术 | 版本 |
|---|---|---|
| 框架 | Vue 3 | ^3.4 |
| 桌面 | Wails | ^2.8 |
| 后端 | Go | ^1.22 |
| 构建 | Vite | ^5.0 |
| 状态 | Pinia | ^2.1 |
| 路由 | Vue Router | ^4.3 |
| 样式 | Tailwind CSS | ^3.4 |
| 图表 | ECharts + vue-echarts | ^6.0 / ^8.0 |
| 图标 | Lucide Vue Next | ^0.300 |
| 语言 | TypeScript | ^5.3 |

### 12.2 设计令牌实施步骤

1. 创建 `styles/tokens/` 目录，按类别拆分 CSS 文件：`color.css`、`spacing.css`、`typography.css`、`radius.css`、`motion.css`、`layout.css`
2. 在 `style.css` 中统一 import
3. 所有组件样式引用 CSS 变量而非硬编码值
4. 创建 `styles/themes/dark.css` 和 `styles/themes/light.css` 实现主题切换
5. 创建 `styles/glass.css` 实现 Header/Footer 毛玻璃效果

### 12.3 组件开发顺序

1. **UiButton** → UiInput → UiSelect → UiToggle（基础交互）
2. **UiPanel** → UiSectionHeader（布局容器）
3. **UiMetricCard** → UiStatusBadge（数据展示）
4. **AppShell** → AppRailNav → AppToolbar → AppStatusBar（应用骨架）
5. **MainTopBar** → MainBottomBar（页眉页脚）
6. 业务组件（按模块）

### 12.4 关键设计原则

- **数据优先：** 数值永远是视觉焦点，使用 24px/800 weight，标签 12px/600 weight，辅助 11px/500 weight
- **层次分明：** 4 级表面层级（app → canvas → panel → panel-strong），通过背景色渐变区分
- **毛玻璃克制：** 仅 Header/Footer 使用 backdrop-filter，主内容区实色背景
- **最小圆角：** 全局 2~4px，仅 pill 徽章使用 999px
- **克制动画：** 120~260ms，仅用于 hover/focus/展开收起，不加装饰性动画
- **通道颜色一致：** 8 色核心方案 + 8 色扩展方案，跨图表/卡片/列表保持一致
- **无障碍：** 所有交互元素支持 `focus-visible`，提供 focus-ring 样式
