# Live Acquisition — 实时采集仪表盘 UI 设计规格

> 本文档为 Wind-DAQ 系统「实时采集仪表盘」页面的精确视觉还原规格。所有尺寸基于 8px 网格系统，目标分辨率 1920×1080（桌面全屏）。
> 基础设计令牌继承自项目根 `DESIGN.md`，本文档在此基础上定义页面级扩展和精确布局参数。

---

## 1. 页面概述

| 属性 | 值 |
|---|---|
| 页面名称 | Live Acquisition |
| 路由路径 | `/acquisition/live` |
| 视图组件 | `LiveAcquisitionView.vue` |
| 主题模式 | 深色优先（Dark Mode），暂不支持亮色 |
| 目标分辨率 | 1920 × 1080（设计基准） |
| 最小窗口 | 1280 × 720 — 低于此显示警告覆盖层，不做重排 |

**设计原则：**
- **数据优先**：当前值、状态指示使用最大字号，确保 3 米外投影清晰可见
- **零装饰**：无渐变、无阴影装饰、无圆角滥用，纯扁平工业风格
- **密度优先**：信息密度最大化，模块间 1px 实线分隔，无多余留白
- **状态即反馈**：运行/停止/报警状态通过颜色 + 动画即时传达

---

## 2. 画布空间计算

基于 AppShell 框架（来自 `DESIGN.md`）：

```
┌─────────────────────────────────────────────────────────────┐
│  MainTopBar (48px)                                          │
├──────────┬──────────────────────────────────────────────────┤
│          │  Canvas Area                                     │
│  Rail    │  Width:  1644px  (1920 - 56 - 220)               │
│  (56px)  │  Height: 1000px  (1080 - 48 - 32)                │
│          │                                                  │
│  SideBar │  ┌────────────────────────────────────────────┐  │
│  (220px) │  │  Live Acquisition Page Content             │  │
│          │  │  [共 1000px 高]                              │  │
│          │  └────────────────────────────────────────────┘  │
│          │                                                  │
├──────────┴──────────────────────────────────────────────────┤
│  MainBottomBar (32px)                                       │
└─────────────────────────────────────────────────────────────┘
```

**页面内容区（1000px 高）垂直分配：**

| 模块 | 高度 | 说明 |
|---|---|---|
| 页面顶部状态栏 | 48px | 固定 |
| 分隔线 | 1px | `--border-default` |
| KPI 指标卡片区 | 96px | 固定，8px × 12 |
| 分隔线 | 1px | `--border-default` |
| 主波形图区 | 596px | 自适应主区域（约 60%） |
| 分隔线 | 1px | `--border-default` |
| 底部信息面板 | 200px | 固定 |
| 分隔线 | 1px | `--border-default` |
| 底部操作栏 | 56px | 固定 |
| **总计** | **1000px** | 精确匹配 Canvas 高度 |

---

## 3. 模块详细规格

### 3.1 页面顶部状态栏（Page Header）

**尺寸：** 宽 1644px × 高 48px
**背景：** `--bg-panel` (`#172338`)，实色，无 blur
**边框：** 底部 1px `--border-default`
**内边距：** `padding: 0 16px`

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [Logo]  Live Acquisition                              ● 运行中   14:32:07  │
│  32×32                    14px/600                        12px/500    14px  │
└─────────────────────────────────────────────────────────────────────────────┘
```

**左侧区域（`flex: 1`，左对齐）：**
- Logo 占位：32px × 32px，左侧距边缘 16px，垂直居中
  - 使用 `IconActivity`（Lucide）或项目 Logo，颜色 `--accent-primary`
  - 背景：透明
- 页面标题："Live Acquisition"
  - 字体：`--font-family-sans`
  - 字号：`--font-size-xl` (18px)
  - 字重：`--font-weight-semibold` (600)
  - 颜色：`--text-primary`
  - 左距 Logo：12px

**右侧区域（`display: flex; gap: 16px; align-items: center`）：**

1. **系统状态指示**
   - 布局：`flex` 行，gap: 6px，垂直居中
   - 状态圆点：8px × 8px，`border-radius: 50%`
     - 运行中：`--accent-success` (`#22c55e`) + `.status-pulse` 呼吸动画
     - 已停止：`--text-muted` (`#94a3b8`)，无动画
     - 报警中：`--accent-danger` (`#ef5b47`) + `.status-pulse` 呼吸动画（1s 周期，快速闪烁）
   - 状态文本：
     - 字号：`--font-size-xs` (12px)
     - 字重：`--font-weight-medium` (500)
     - 颜色：与圆点同色
   - 状态切换时：文本 + 圆点颜色过渡 180ms `easing-standard`

2. **当前时间**
   - 字号：`--font-size-sm` (14px)
   - 字重：`--font-weight-regular` (400)
   - 字体：`--font-family-mono`（等宽，确保数字不跳动）
   - 颜色：`--text-secondary`
   - 格式：`HH:mm:ss`，每秒更新

3. **网络状态**
   - 使用 `IconWifi` / `IconWifiOff`（Lucide）
   - 图标尺寸：16px × 16px
   - 在线：`--accent-success`
   - 离线：`--accent-danger`
   - 弱网：`--accent-warning`
   - Hover 提示：Tooltip 显示延迟（ms）和连接状态文本

---

### 3.2 KPI 指标卡片区（KPI Cards）

**尺寸：** 宽 1644px × 高 96px
**背景：** `--bg-canvas` (`#111c31`)
**内边距：** `padding: 16px 16px`
**布局：** `display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px;`

每个卡片尺寸：约 399px × 64px（(1644 - 16×2 - 16×3) / 4 = 399px）

**单卡结构：**

```
┌─────────────────────────────────────────┐
│▎ 采样率                                 │
│▎ 1024 Hz                                │
│▎ ─────────────────────────────────────  │
│▎ 当前采样频率                            │
└─────────────────────────────────────────┘
```

**卡片容器样式：**
- 背景：`--bg-panel` (`#172338`)
- 圆角：`--radius-md` (4px) — 用户要求 8px，但为与现有系统保持一致，使用 4px；若需严格 8px，设为 `--radius-lg: 8px`
- 左边框：4px 实线（彩色区分）
- 内边距：`padding: 12px 16px`
- Hover 态：背景变为 `--bg-panel-strong` (`#1e293b`)，过渡 120ms

**卡片内容（垂直居中排列，gap: 4px）：**

| 层级 | 内容示例 | 字号 | 字重 | 颜色 | 字体 |
|---|---|---|---|---|---|
| 标签（上） | "采样率" | 12px | 600 | `--text-muted` | Sans |
| 数值（中） | "1024 Hz" | 24px | 800 | `--text-primary` | Mono |
| 描述（下） | "当前采样频率" | 11px | 500 | `--text-muted` | Sans |

**4 张卡片配色方案：**

| 卡片 | 标签 | 数值示例 | 左边框颜色 | Token |
|---|---|---|---|---|
| 1 | 采样率 | 1024 Hz | `#0ea5e9` (主蓝) | `--accent-primary` 映射 `#0ea5e9` |
| 2 | 活动通道 | 16/32 | `#22c55e` (成功绿) | `--accent-success` |
| 3 | 数据量 | 1.2 GB | `#eab308` (警告黄) | `--accent-warning` 映射 `#eab308` |
| 4 | 运行时长 | 00:15:30 | `#a855f7` (紫色) | 扩展色 `--accent-purple: #a855f7` |

> **注：** 数值使用 `--font-family-mono`（JetBrains Mono），确保等宽显示，时间数字不跳动。

---

### 3.3 主波形图区（Main Waveform Chart）

**尺寸：** 宽 1644px × 高 596px
**背景：** `--bg-canvas` (`#111c31`)
**布局：** 上下分栏 — 图表区 + 通道选择栏

#### 3.3.1 图表显示区（Chart Area）

**尺寸：** 宽 1644px × 高 548px（596 - 48）
**内边距：** `padding: 8px 16px 0 16px`

**ECharts 配置规范：**

```javascript
{
  grid: {
    top: 24,
    right: 16,
    bottom: 32,
    left: 56,
    containLabel: false
  },
  backgroundColor: 'transparent',
  animation: false, // 实时数据禁用动画，提升性能
}
```

**坐标轴规范：**

| 元素 | 样式 |
|---|---|
| X 轴（时间） | 轴线 `#334155` (`--border-default`)，刻度标签 11px/500/mono，`--text-muted`，时间格式 `mm:ss.ms` |
| Y 轴（电压） | 轴线 `#334155`，刻度标签 11px/500/mono，`--text-muted`，范围 -10V ~ +10V，步进 5V |
| 主网格线 | 实线，`rgba(255,255,255,0.06)`，每 50px |
| 次网格线 | 实线，`rgba(255,255,255,0.02)`，每 10px |

**数据线规范：**
- 线宽：1.5px（选中通道），1px（未选中/冻结）
- 采样点：默认隐藏，缩放至逐点时显示 3px 圆点
- 颜色：使用通道颜色方案（见 DESIGN.md 3.1 通道颜色）
- 数据窗口：默认 1000 点滚动，可配置至 5000 点
- 空闲区：灰色虚线 `rgba(148,163,184,0.3)`，`stroke-dasharray: 6 3`

**图表交互：**
- 鼠标滚轮：X 轴缩放（以光标位置为中心）
- 鼠标中键拖动 / `Ctrl` + 左键：平移
- `Shift` + 左键拖动：框选缩放
- 双击 / `R` 键：重置视图
- 单光标（`C` 键）：1px 虚线垂线 + 悬浮读数框
- 双光标（`D` 键）：显示 Δt、ΔV、频率

#### 3.3.2 通道选择栏（Channel Selector Bar）

**尺寸：** 宽 1644px × 高 48px
**背景：** `--bg-panel` (`#172338`)
**边框：** 顶部 1px `--border-default`
**内边距：** `padding: 0 16px`
**布局：** `display: flex; align-items: center; gap: 8px; overflow-x: auto;`

**单个通道复选框（Channel Checkbox）：**

```
┌──────────────────────┐
│ □  ● CH01  压力传感器 │
└──────────────────────┘
```

- 容器：高度 28px，`padding: 0 10px`，`border-radius: 3px`
- 背景：默认透明，Hover 时 `--bg-panel-strong`
- 选中态：背景 `rgba(87, 179, 242, 0.12)`，文字 `--accent-primary`
- 复选框：14px × 14px，边框 `--border-strong`，圆角 2px
  - 未选中：透明背景
  - 选中：填充 `--accent-primary`，内部白色对勾（Lucide `IconCheck` 10px）
- 颜色指示点：8px × 8px 圆点，对应通道颜色，左距复选框 6px
- 通道号："CH01"，`--font-family-mono`，12px/600，`--text-secondary`
- 通道名称："压力传感器"，12px/400，`--text-muted`，最大宽度 80px，溢出省略号

**通道颜色分配（前 8 通道）：**

| 通道 | 颜色 | Hex |
|---|---|---|
| CH01 | 蓝 | `#3B82F6` |
| CH02 | 绿 | `#22C55E` |
| CH03 | 琥珀 | `#F59E0B` |
| CH04 | 紫 | `#A855F7` |
| CH05 | 粉 | `#EC4899` |
| CH06 | 青 | `#06B6D4` |
| CH07 | 橙 | `#F97316` |
| CH08 | 堇紫 | `#8B5CF6` |

**全选/清空按钮：**
- 位于通道选择栏最左侧（或最右侧）
- 使用 `UiButton` `ghost` `sm` 变体
- 文本："全选" / "清空"

---

### 3.4 底部信息面板（Info Panel）

**尺寸：** 宽 1644px × 高 200px
**背景：** `--bg-canvas` (`#111c31`)
**布局：** 左右分栏（40% / 60%），中间 1px `--border-default` 分隔

#### 3.4.1 左侧：通道状态矩阵（Channel Status Matrix）

**尺寸：** 宽 657px（1644 × 0.4，向下取整到 8px 倍数：656px，微调至 657px 或保持 656px）× 高 200px
**实际分配：** 宽 656px（`1644 × 2/5` 约等于 657.6，取 656px 为 8 的倍数，右侧补 1px 边框）
**背景：** `--bg-panel` (`#172338`)
**内边距：** `padding: 12px 0 0 0`（表格头部自带 padding）

**表头（Header Row）：**
- 高度：32px
- 背景：`--bg-panel-strong` (`#1e293b`)
- 字体：12px/600，`--text-secondary`
- 列：`padding: 0 12px`

| 列名 | 宽度 | 对齐 |
|---|---|---|
| 通道号 | 72px | 左对齐 |
| 名称 | 160px | 左对齐 |
| 当前值 | 120px | 右对齐 |
| 状态 | 80px | 居中对齐 |

**表格体：**
- 行高：36px（`(200 - 32) / 4.6` ≈ 36px，显示约 4 行完整 + 1 行部分）
- 奇偶行背景：奇数 `--bg-panel`，偶数 `rgba(255,255,255,0.02)`
- Hover 行：背景 `rgba(255,255,255,0.04)`

**单元格样式：**

| 列 | 样式 |
|---|---|
| 通道号 | Mono 12px/600，`--text-secondary` |
| 名称 | Sans 12px/400，`--text-primary` |
| 当前值 | Mono 16px/700，`--text-primary`；报警时变 `--accent-danger` |
| 状态 | 图标 + 文本，12px/500 |

**状态指示器样式：**

| 状态 | 图标 | 颜色 | 背景/动画 |
|---|---|---|---|
| 正常 | ● 实心圆（8px） | `#22c55e` (`--accent-success`) | 无 |
| 警告 | ▲ 三角形（10px） | `#eab308` (`--accent-warning`) | `.status-pulse` 2s 呼吸 |
| 错误 | ■ 方块（8px） | `#ef4444` (`--accent-danger`) | `.status-pulse` 1s 快速闪烁 |

> **3米可读性规则：** 当前值列使用 16px/700 mono 字体，状态图标 ≥ 8px，确保远距离清晰可辨。

**行内报警闪烁效果：**
- 当通道状态从"正常"变为"错误"时：
  - 整行背景闪烁红色：`animation: row-alarm-flash 1s ease-in-out 3`
  - 当前值文字变 `--accent-danger`
  - 状态图标变为 ■ + 快速脉冲

```css
@keyframes row-alarm-flash {
  0%, 100% { background-color: transparent; }
  50% { background-color: rgba(239, 68, 68, 0.15); }
}
```

#### 3.4.2 右侧：事件流日志（Event Log Stream）

**尺寸：** 宽 988px（1644 - 656）× 高 200px
**背景：** `--bg-panel` (`#172338`)
**布局：** 上下分栏 — 筛选栏（32px）+ 日志列表（168px）

**筛选栏（Filter Bar）：**
- 高度：32px
- 背景：`--bg-panel-strong`
- 内边距：`padding: 0 12px`
- 布局：`flex` 行，`gap: 4px`，垂直居中
- 左侧标签："事件筛选"，10px/600，`--text-muted`，大写，字间距 0.1em
- 筛选按钮组：
  - 使用 `UiButton` `ghost` `sm` 变体
  - 选项：全部 / 信息 / 警告 / 错误
  - 选中态：背景 `rgba(87, 179, 242, 0.15)`，文字 `--accent-primary`
  - 未选中态：文字 `--text-muted`
  - 各选项带对应颜色小圆点（6px）：`--accent-info` / `--accent-warning` / `--accent-danger`

**日志列表（Log List）：**
- 高度：168px
- 溢出：`overflow-y: auto`，自定义滚动条（4px 宽，`#334155` track，`#475569` thumb）
- 行高：28px（显示 6 行完整）

**单条日志结构：**

```
┌──────────────────────────────────────────────────────────────────┐
│ [i]  14:32:07.245    通道 CH03 电压超出阈值 (+12.5V > +10V)      │
└──────────────────────────────────────────────────────────────────┘
```

- 内边距：`padding: 0 12px`
- 布局：`grid: 24px 120px 1fr / 1fr`，`align-items: center`

| 元素 | 样式 |
|---|---|
| 类型图标 | 16px × 16px，Lucide 图标：信息 `IconInfo`，警告 `IconAlertTriangle`，错误 `IconXCircle` |
| 时间戳 | Mono 11px/500，`--text-muted`，格式 `HH:mm:ss.ms` |
| 描述文本 | Sans 12px/400，`--text-primary`；警告文字 `--accent-warning`；错误文字 `--accent-danger` |

**日志行颜色编码：**

| 级别 | 左侧边框 | 图标颜色 | 背景 Hover |
|---|---|---|---|
| 信息 (Info) | 2px `#0ea5e9` | `#0ea5e9` | `rgba(87,179,242,0.04)` |
| 警告 (Warning) | 2px `#eab308` | `#eab308` | `rgba(234,179,8,0.04)` |
| 错误 (Error) | 2px `#ef4444` | `#ef4444` | `rgba(239,68,68,0.04)` |

**新日志进入动画：**
- 新行从顶部滑入：`transform: translateY(-100%)` → `translateY(0)`，opacity 0 → 1
- 时长：`--motion-base` (180ms)
- 缓动：`easing-emphasis`
- 错误级别日志额外添加左侧边框闪烁（1s，3 次）

---

### 3.5 底部操作栏（Action Bar）

**尺寸：** 宽 1644px × 高 56px
**背景：** `--bg-panel-strong` (`#1e293b`)
**边框：** 顶部 1px `--border-default`
**内边距：** `padding: 0 16px`
**布局：** `display: flex; justify-content: space-between; align-items: center;`

#### 3.5.1 左侧：采集控制按钮组

**布局：** `display: flex; gap: 12px; align-items: center;`

**按钮规格（大操作按钮）：**

| 按钮 | 文本 | 变体 | 尺寸 | 背景色 | 文字色 | 图标 |
|---|---|---|---|---|---|---|
| 开始 | "开始采集" | primary | md | `#22c55e` (`--accent-success`) | `#ffffff` | `IconPlay` (16px) |
| 暂停 | "暂停" | warning | md | `#eab308` (`--accent-warning`) | `#0f172a` | `IconPause` (16px) |
| 停止 | "停止" | danger | md | `#ef4444` (`--accent-danger`) | `#ffffff` | `IconSquare` (16px) |
| 保存数据 | "保存数据" | primary | md | `#0ea5e9` (`--accent-primary`) | `#ffffff` | `IconSave` (16px) |

**按钮基础样式：**
- 高度：36px（比标准 `UiButton md` 略大，强调操作性）
- 内边距：`padding: 0 20px`
- 圆角：`--radius-md` (4px)
- 字重：`--font-weight-bold` (700)
- 字号：`--font-size-sm` (14px)
- 图标右距文字：8px

**按钮交互状态：**

| 状态 | 效果 | 时长 |
|---|---|---|
| Hover | 背景亮度 +15%（`filter: brightness(1.15)`），`translateY(-1px)` | 120ms |
| Pressed (Active) | 背景亮度 -10%，`translateY(0)`，`scale(0.98)` | 60ms |
| Focus-visible | `box-shadow: 0 0 0 2px currentColor, 0 0 0 4px rgba(255,255,255,0.1)` | - |
| Disabled | `opacity: 0.4`，`cursor: not-allowed`，Hover 无变化 | - |
| Loading | 文字替换为旋转 Spinner（14px），背景不变 | - |

**特殊状态：**
- "开始"按钮在运行中自动变为 Disabled，"暂停"和"停止"变为 Enabled
- "保存数据"在未有数据时 Disabled

#### 3.5.2 右侧：辅助操作按钮组

**布局：** `display: flex; gap: 8px; align-items: center;`

| 按钮 | 图标 | 样式 | 行为 |
|---|---|---|---|
| 快速设置 | `IconSettings` (16px) | `UiButton` `ghost` `sm` | 打开设置抽屉（右侧滑入，400px 宽） |
| 全屏切换 | `IconMaximize2` / `IconMinimize2` (16px) | `UiButton` `ghost` `sm` | 切换应用全屏模式（F11） |

**Ghost 按钮 Hover：** 背景 `rgba(255,255,255,0.06)`，文字 `--text-primary`，过渡 120ms。

---

## 4. 设计令牌映射

### 4.1 颜色系统

#### 页面级颜色扩展

以下令牌为 Live Acquisition 页面在全局令牌基础上做的扩展或精确映射：

| Token | 值 | 用途 | 备注 |
|---|---|---|---|
| `--la-bg-app` | `#121212` | 应用背景（如独立窗口模式） | 用户指定，略深于 `--bg-app` |
| `--la-bg-card` | `#1e1e1e` | 卡片背景 | 用户指定，映射至 `--bg-panel-strong` 近似值 |
| `--la-bg-hover` | `#2d2d2d` | 悬浮背景 | 用户指定 |
| `--la-accent-blue` | `#0ea5e9` | 主强调蓝 | 用户指定，替代 `--accent-primary` (#56ccf2) |
| `--la-accent-green` | `#22c55e` | 成功/运行 | 用户指定，与全局一致 |
| `--la-accent-yellow` | `#eab308` | 警告/暂停 | 用户指定，略深于全局 `--accent-warning` |
| `--la-accent-red` | `#ef4444` | 错误/停止 | 用户指定，替代 `--accent-danger` (#ef5b47) |
| `--la-text-primary` | `#e5e5e5` | 主要文字 | 用户指定，略暖于 `--text-primary` |
| `--la-text-secondary` | `#a3a3a3` | 次要文字 | 用户指定 |
| `--la-text-disabled` | `#737373` | 禁用文字 | 用户指定 |
| `--la-accent-purple` | `#a855f7` | KPI 卡片 4 边框 | 新增扩展色 |

**与全局令牌兼容策略：**
- 若全局令牌已定义且值接近（如 `--accent-success` = `#22c55e`），直接使用全局令牌
- 若用户指定值与全局差异较大（如主蓝 `#0ea5e9` vs `#56ccf2`），在页面级 CSS 中覆盖：`:root { --accent-primary: #0ea5e9; }`
- 所有 `--la-*` 令牌仅在 Live Acquisition 视图及其子组件中使用

#### 背景层级

| 层级 | Token | 值 | 使用位置 |
|---|---|---|---|
| 第 0 层（最深） | `--bg-app` | `#0f172a` | AppShell 底层 |
| 第 1 层 | `--bg-canvas` | `#111c31` | Canvas 区域、波形图背景 |
| 第 2 层 | `--bg-panel` | `#172338` | 面板、卡片、操作栏 |
| 第 3 层 | `--bg-panel-strong` | `#1e293b` | 表头、Hover 背景、强面板 |

---

### 4.2 字体系统

| 用途 | 字体族 | 回退栈 |
|---|---|---|
| 中文界面 / 标签 / 正文 | 系统无衬线 | `'Microsoft YaHei UI', 'Microsoft YaHei', 'PingFang SC', 'Segoe UI', sans-serif` |
| 数值 / 时间 / 通道号 | 等宽 | `'JetBrains Mono', 'Cascadia Code', 'SFMono-Regular', 'Consolas', monospace` |

**字号层级（页面专用）：**

| Token | 尺寸 | 字重 | 行高 | 用途 |
|---|---|---|---|---|
| `--la-text-xs` | 11px | 500 | 1.4 | 辅助说明、日志时间戳 |
| `--la-text-sm` | 12px | 400/500/600 | 1.5 | 标签、表头、通道名、日志描述 |
| `--la-text-base` | 14px | 400/600 | 1.5 | 按钮文字、时间显示 |
| `--la-text-lg` | 16px | 700 | 1.3 | 表格当前值（远距离可读） |
| `--la-text-xl` | 18px | 600 | 1.3 | 页面标题 |
| `--la-text-2xl` | 20px | 700 | 1.2 | 面板标题 |
| `--la-text-kpi` | 24px | 800 | 1.1 | KPI 数值（远距离可读） |

---

### 4.3 间距系统（8px 网格）

所有尺寸、padding、margin、gap 必须为 8px 的整数倍（或 4px 用于细线/紧凑场景）。

| Token | 值 | 用途 |
|---|---|---|
| `--space-1` | 4px | 细线间距、图标间隙 |
| `--space-2` | 8px | 紧凑 padding、小 gap |
| `--space-3` | 12px | 卡片内边距、表格行 padding |
| `--space-4` | 16px | 标准 padding、模块间隙 |
| `--space-5` | 20px | 按钮水平 padding |
| `--space-6` | 24px | 大 padding |
| `--space-8` | 32px | 段落间距 |

**网格检查清单：**
- [ ] 所有高度值：48, 96, 596, 200, 56, 32, 28, 36 均为 4 的倍数，且主模块为 8 的倍数
- [ ] 所有 padding/margin：4, 8, 12, 16, 20, 24
- [ ] 所有 gap：4, 8, 12, 16

---

## 5. 交互状态详表

### 5.1 按钮状态机

```
[Normal] --hover--> [Hover] --mousedown--> [Pressed] --mouseup--> [Normal]
   |                      |                       |
   |                      |--mouseleave--> [Normal]
   |--disabled----------------------------------> [Disabled]
   |--loading-----------------------------------> [Loading]
```

| 状态 | 背景 | 文字 | 边框 | Transform | Shadow | 过渡 |
|---|---|---|---|---|---|---|
| Normal | 令牌定义色 | `#fff` / 深色 | 无 | none | none | - |
| Hover | `brightness(1.15)` | 同 Normal | 无 | `translateY(-1px)` | `0 4px 12px rgba(0,0,0,0.2)` | 120ms ease |
| Pressed | `brightness(0.9)` | 同 Normal | 无 | `scale(0.98)` | none | 60ms ease |
| Disabled | 同 Normal | 同 Normal | 无 | none | none | `opacity: 0.4` |
| Loading | 同 Normal | 隐藏 | 无 | none | none | Spinner 旋转 1s linear infinite |

### 5.2 通道选中状态机

| 状态 | 背景 | 文字 | 边框 | 说明 |
|---|---|---|---|---|
| 未选中 | transparent | `--text-secondary` | 无 | 波形图对应数据线 30% 透明度 |
| Hover | `rgba(255,255,255,0.03)` | `--text-primary` | 无 | 波形图不变 |
| 选中 | `rgba(87,179,242,0.12)` | `--accent-primary` | 无 | 波形图 100% 透明度，线宽 1.5px |
| 报警 | `rgba(239,68,68,0.12)` | `--accent-danger` | 无 | 波形图红色，快速闪烁 |

### 5.3 报警闪烁效果

**状态脉冲（呼吸灯）：**
```css
@keyframes status-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
/* 周期：2s（正常）, 1s（紧急） */
```

**行报警闪烁：**
```css
@keyframes row-alarm-flash {
  0%, 100% { background-color: transparent; }
  25% { background-color: rgba(239, 68, 68, 0.2); }
  75% { background-color: rgba(239, 68, 68, 0.1); }
}
/* 周期：1s，播放 3 次后停止 */
```

**KPI 卡片报警：**
- 当系统出现错误时，顶部状态栏右侧系统状态圆点变为红色 + 1s 快速脉冲
- 若报警与特定 KPI 相关（如采样率异常），该 KPI 卡片左边框变为红色，卡片背景 `rgba(239,68,68,0.08)`

---

## 6. 3 米可读性规范（投影演示）

为确保风洞实验现场投影演示时关键信息清晰可见，执行以下最小字号和对比度规则：

| 信息类型 | 最小字号 | 最小字重 | 最小对比度 | 特殊处理 |
|---|---|---|---|---|
| KPI 数值 | 24px | 800 | 7:1 | 等宽字体，卡片左侧 4px 彩色边框增强识别 |
| 当前电压/通道值 | 16px | 700 | 7:1 | 表格中右对齐，Mono 字体 |
| 状态图标（●▲■） | 8px（图标高度） | - | 4.5:1 | 报警状态加脉冲动画吸引注意 |
| 按钮文字 | 14px | 700 | 4.5:1 | 大按钮（36px 高），高饱和度背景色 |
| 时间戳 | 14px | 400 | 4.5:1 | Mono 字体，确保格式固定 |
| 波形图 Y 轴刻度 | 11px | 500 | 4.5:1 | 主刻度线每 5V 一条，加粗至 2px |
| 波形图数据线 | 1.5px（线宽） | - | - | 选中通道 2px，颜色饱和度高 |

**对比度验证（基于深色背景 #121212）：**
- `#e5e5e5` on `#121212` = **14.8:1** ✓
- `#22c55e` on `#121212` = **7.2:1** ✓
- `#0ea5e9` on `#121212` = **6.8:1** ✓
- `#eab308` on `#121212` = **7.5:1** ✓
- `#ef4444` on `#121212` = **5.8:1** ✓
- `#a3a3a3` on `#121212` = **7.4:1** ✓

---

## 7. 组件清单与文件映射

### 7.1 需新建的业务组件

| 组件名 | 路径 | 说明 |
|---|---|---|
| `LiveAcquisitionView` | `views/LiveAcquisitionView.vue` | 页面根组件，负责整体布局 |
| `AcquisitionHeader` | `components/acquisition/AcquisitionHeader.vue` | 页面顶部状态栏 |
| `AcquisitionKpiBar` | `components/acquisition/AcquisitionKpiBar.vue` | KPI 卡片区容器 |
| `KpiCard` | `components/acquisition/KpiCard.vue` | 单个 KPI 卡片 |
| `WaveformChart` | `components/acquisition/WaveformChart.vue` | ECharts 波形图封装 |
| `ChannelSelector` | `components/acquisition/ChannelSelector.vue` | 通道选择栏 |
| `ChannelStatusMatrix` | `components/acquisition/ChannelStatusMatrix.vue` | 通道状态表格 |
| `EventLogPanel` | `components/acquisition/EventLogPanel.vue` | 事件日志面板 |
| `EventLogFilter` | `components/acquisition/EventLogFilter.vue` | 日志筛选栏 |
| `EventLogItem` | `components/acquisition/EventLogItem.vue` | 单条日志行 |
| `AcquisitionControlBar` | `components/acquisition/AcquisitionControlBar.vue` | 底部操作栏 |
| `AcquisitionSettingsDrawer` | `components/acquisition/AcquisitionSettingsDrawer.vue` | 快速设置抽屉 |

### 7.2 复用现有 UI 组件

| 组件 | 来源 | 用途 |
|---|---|---|
| `UiButton` | `components/ui/UiButton.vue` | 所有按钮 |
| `UiPanel` | `components/ui/UiPanel.vue` | 信息面板容器（可选） |
| `UiMetricCard` | `components/ui/UiMetricCard.vue` | KPI 卡片（可继承扩展） |
| `UiStatusBadge` | `components/ui/UiStatusBadge.vue` | 状态徽章 |

---

## 8. 动画与动效规格

| 动画 | 触发条件 | 属性 | 时长 | 缓动 |
|---|---|---|---|---|
| 按钮 Hover | mouseenter | background, transform, box-shadow | 120ms | `ease` |
| 按钮 Press | mousedown | transform | 60ms | `ease` |
| 状态脉冲 | 状态变化 | opacity | 2s/1s | `ease-in-out` infinite |
| 日志滑入 | 新日志到达 | transform, opacity | 180ms | `cubic-bezier(0.18, 0.7, 0.2, 1)` |
| 行报警闪 | 通道报警 | background-color | 1s × 3 | `ease-in-out` |
| KPI 数值更新 | 数据刷新 | opacity（闪一下） | 80ms | `ease` |
| 通道选中 | click | background, color | 120ms | `ease` |
| 抽屉滑入 | 点击设置 | transform (translateX) | 260ms | `cubic-bezier(0.18, 0.7, 0.2, 1)` |
| 波形刷新 | 数据帧到达 | 无（直接 setOption） | 0ms | - |

> **性能规则：** 波形图数据更新禁用 ECharts 动画（`animation: false`），使用 `setOption` 直接刷新，确保 1024Hz 数据流不丢帧。

---

## 9. 键盘快捷键

| 快捷键 | 功能 | 作用域 |
|---|---|---|
| `Space` | 开始/暂停采集 | 全局 |
| `Esc` | 停止采集 | 全局 |
| `S` | 保存数据 | 全局 |
| `F` | 全屏切换 | 全局 |
| `C` | 切换单光标模式 | 波形图区 |
| `D` | 切换双光标模式 | 波形图区 |
| `R` | 重置波形视图 | 波形图区 |
| `1` ~ `8` | 切换通道 1~8 可见性 | 波形图区 |
| `Ctrl` + `=` | 放大时间轴 | 波形图区 |
| `Ctrl` + `-` | 缩小时间轴 | 波形图区 |

---

## 10. 空态与错误态

### 10.1 无设备连接

- 波形图区显示占位：大号 `IconActivity`（48px，`--text-muted`）+ "未检测到采集设备"
- 按钮状态："开始"Disabled，"保存数据"Disabled
- KPI 数值显示 `--`

### 10.2 设备离线

- 顶部状态栏系统状态：灰色圆点 + "已停止"
- 通道状态矩阵：所有通道显示 "离线"，灰色圆点
- 波形图：冻结最后一帧，数据线变为 1px 灰色虚线

### 10.3 采集进行中掉线

- 顶部状态栏：红色脉冲圆点 + "连接中断"
- 弹出 `UiConfirmDialog`："采集设备连接中断，是否保存已采集数据？"
- 波形图：冻结，显示掉线时刻时间戳标注

---

## 附录：快速参考图

```
Live Acquisition View (1920×1080, Canvas 1644×1000)
┌────────────────────────────────────────────────────────────────────────────┐
│ AcquisitionHeader                                    48px                  │
├────────────────────────────────────────────────────────────────────────────┤
│ [KPI 1] [KPI 2] [KPI 3] [KPI 4]                      96px                  │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  WaveformChart (ECharts, 596px)                                            │
│  ~ Y: -10V ~ +10V, X: Time, Grid: dashed, 1000-point rolling window      │
│                                                                            │
│  [□ CH01 ■] [□ CH02 ■] [□ CH03 ■] ...        ChannelSelector (48px)      │
├────────────────────────────────────────────────────────────────────────────┤
│  ChannelStatusMatrix (656px)  │  EventLogPanel (988px)       200px       │
│  ┌────┬────────┬──────┬────┐  │  [全部|信息|警告|错误]                      │
│  │CH01│压力    │+2.35 │ ●  │  │  [i] 14:32:07  通道 CH03 电压超限        │
│  │CH02│温度    │+12.5 │ ■  │  │  [▲] 14:32:05  采样率下降至 512Hz        │
│  └────┴────────┴──────┴────┘  │  [i] 14:32:00  采集开始                  │
├────────────────────────────────────────────────────────────────────────────┤
│ [▶ 开始] [⏸ 暂停] [⏹ 停止] [💾 保存]        [⚙] [⛶]    56px            │
└────────────────────────────────────────────────────────────────────────────┘
```

---

> 本文档版本：v1.0
> 最后更新：2026-04-29
> 关联文档：`DESIGN.md`（全局设计系统）、`STRUCTURE.md`（项目结构）
