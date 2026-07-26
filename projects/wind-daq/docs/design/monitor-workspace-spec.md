# Wind-DAQ 监控工作区规范

> Companion to `../../DESIGN.md`。  
> 本规范将 **2026-07 混合视图改版原型**（`docs/ui-redesign-prototype.html`）中已确认的 UI 风格、分隔方式与交互语义落地为正式规则。  
> 实现时只允许使用 design tokens 与 `components/ui/*` 原语，禁止在 feature 代码写死 hex / 随意 px。

## 1. 适用范围

| 场景 | 是否适用 |
|---|---|
| 仪表盘设备详情 · 混合 / 图表 / 卡片视图 | ✅ 主适用 |
| 设备列表侧栏、顶栏全局运行控制、底栏状态 | ✅ |
| 校准 / 遍历 / 运动配置类紧凑表单 | ❌ 走 `DESIGN.md` Density Spec |
| 全局设置弹窗 | ❌ 走 Density Spec |

**参考原型**：`docs/ui-redesign-prototype.html`  
**视觉目标**：工业仪表 + 现代桌面工具；冷静、可读、作用域清晰。不追求营销站审美。

---

## 2. 设计原则（从原型提炼）

1. **数据优先，Chrome 退后**  
   曲线与数值是主角；导航、边框、装饰只负责定位，不抢对比度。

2. **作用域必须可见**  
   全局操作与本设备操作分开放置，文案与按钮颜色不能让人猜「停的是谁」。

3. **默认少显示，按需展开**  
   多通道默认只画 6 路；其余通过图例 / 卡片显隐，而不是一上来 16+ 线糊成一团。  
   **不提供**「聚焦 / 全部 / 异常」三态切换控件（已否决）。

4. **分隔靠层级，不靠厚边框堆砌**  
   用 surface 阶梯 + 1px border + 轻阴影分区；同一面板内用细分割线，不套多层重卡片。

5. **读数不挡曲线**  
   多通道光标读数放在图表**外侧**读数条；禁止大面积深色 tooltip 盖住波形主体。

6. **命名与单位全站一致**  
   通道一律 `CH01`…`CHnn`；不同物理量分组展示；大气压等大数量级优先工程单位（kPa）。

---

## 3. 布局骨架与分隔

### 3.1 应用壳（与 `tokens/layout.css` 对齐）

```
┌─────────────────────────────────────────────────────────────┐
│ TopBar  (--layout-header-height: 56px)                      │
├────┬──────────┬─────────────────────────────────────────────┤
│Rail│ Sidebar  │  Workspace (canvas)                         │
│64px│ 244px    │  ┌ device header ─────────────────────────┐ │
│    │          │  │ chart panel                            │ │
│    │          │  │ channel cards                          │ │
│    │          │  └────────────────────────────────────────┘ │
├────┴──────────┴─────────────────────────────────────────────┤
│ StatusBar (--layout-footer-height 目标；实现可略矮)          │
└─────────────────────────────────────────────────────────────┘
```

| 区域 | Token / 尺寸 | 表面 | 分隔规则 |
|---|---|---|---|
| 顶栏 | `--layout-header-height` | `--bg-panel` 或玻璃（主题可调） | 底边 `1px solid var(--border-default)` |
| 左轨 | `--layout-rail-width` | 实心 `--bg-canvas` / sidebar | 右边框 1px；**禁止玻璃** |
| 设备侧栏 | `--layout-sidebar-width` | 实心 `--bg-panel` | 右边框 1px；与 canvas 明确分层 |
| 主工作区 | 剩余宽度 | `--bg-canvas`（略灰于 panel） | 内边距 `--space-4` / `--space-5` |
| 底栏 | footer | `--bg-panel` | 顶边 1px |

### 3.2 工作区内部分隔（原型核心）

工作区采用 **「头 + 主滚动区」**，主滚动区再拆 **图表面板 + 通道卡面板**：

| 区块 | 容器 | 间距 |
|---|---|---|
| 设备标题行 | 透明背景，贴 canvas | 上下 `--space-3` / `--space-2`，左右与主区对齐 |
| 图表面板 | `UiPanel` 语义：`--bg-panel` + `--border-default` + `--radius-lg` + `--shadow-panel` | 与下方卡片间距 `--space-3` |
| 通道卡面板 | 同上 | 内部 group 之间用 `1px` 分割，不用再套第三层重阴影 |
| 图表面板内 | toolbar / plot / legend / readout | 横向 padding `--space-3`～`--space-4`；区块之间 `border-top` 细分隔 |

**分隔原则**：

- **一级分隔**（壳层）：边框 + surface 阶梯（rail / sidebar / canvas / panel）。
- **二级分隔**（工作区卡片）：panel 白底 + 轻阴影，浮在 canvas 上。
- **三级分隔**（卡片内）：仅 `border-default` 细线，不再加阴影。

禁止：

- 卡片套卡片套卡片（三层以上实心框）。
- 用粗色条、渐变条当分区装饰。
- 主数据区使用大面积玻璃模糊。

---

## 4. 操作作用域（必须落地）

### 4.1 全局 vs 本设备

| 作用域 | 放置位置 | 典型动作 | 视觉 |
|---|---|---|---|
| **全局** | 顶栏右侧 | 开始记录、停止全部采集 | 可加极弱文案「全局」；危险操作用 soft-danger / danger，**不要**用成功绿表示「停止」 |
| **本设备** | 设备标题行右侧 | 校零、停止采集、断开 | 可加极弱文案「本设备」；停止= danger 实心；断开= ghost + 二次确认 |

### 4.2 按钮语义

| 动作 | Variant | 说明 |
|---|---|---|
| 开始记录 / 主推进 | `primary`（accent） | 仅一个主 CTA 抢眼 |
| 停止本设备采集 | `danger` | 红色实心，不可与 primary 同色 |
| 停止全部采集 | `soft-danger` 或 `danger` 次级 | 与「本设备停止」文案必须带「全部/本设备」 |
| 断开连接 | `ghost` + confirm | 高风险但非高频；必须确认 |
| 校零 | `soft` / disabled | 采集中禁用，**必须有禁用原因**（title 或 tooltip） |
| 冻结刷新 / 复位 / 显示全部 | `ghost` sm | 图表工具，弱化 |

### 4.3 禁用态

- 禁用按钮 opacity ≈ 0.45，`cursor: not-allowed`。
- **必须**提供原因：`title` 或 hover tip。  
  例：「请先配置记录路径」「采集中不可校零」。
- 禁止灰按钮无解释。

### 4.4 否决项

- ❌ 顶栏与设备区同时出现同文案「停止采集」且颜色不一致。
- ❌ 用绿色表示「停止」。
- ❌ 「聚焦 / 全部 / 异常」分段切换作为默认图表模式（用户已明确不要）。

---

## 5. 实时趋势图（混合视图）

### 5.1 结构

```
┌ chart panel ──────────────────────────────────────────┐
│ 标题「实时趋势」 + 状态 hint          [显示全部][冻结][复位] │
├───────────────────────────────────────────────────────┤
│ Y轴 │ plot（仅压力组，单位 Pa）                         │
│     │                                                  │
├─────┴─────────────────────────────────────────────────┤
│ 图例 chips（CH01… 可点）                                 │
├───────────────────────────────────────────────────────┤
│ 读数条（图外）：时间 · 各可见通道值                        │
└───────────────────────────────────────────────────────┘
```

### 5.2 默认可见通道

- 默认可见：**6** 路压力通道（`CH01`–`CH06`，或用户上次会话记忆）。
- 「显示全部」：打开全部压力通道曲线（环境量仍不进压力图）。
- 「复位窗口」：恢复默认 6 路 + 取消独显 + 光标回实时端。
- 图例：**单击**显隐；**双击**独显；再次双击或复位退出独显。
- 卡片点击与图例状态同步。

### 5.3 曲线与读数

- 线宽：主可见系列 **2–2.2px**；非焦点压暗 opacity ≈ 0.18。
- 网格 / 轴 / 十字线：只用 `--chart-*` tokens（见 §8 与 `chart-spec.md`）。
- 实时更新：**禁止**数据点过渡动画。
- 冻结：停止滚动刷新，按钮文案切为「继续刷新」。
- 光标读数：在 **readout bar**，最多展示 8 路；更多可横向换行，不盖 plot。
- Tooltip 大卡片：仅单通道详情或校准图可用；多通道实时图优先 readout bar。

### 5.4 单位与分组

- 压力组曲线 Y 轴单位：`Pa`（或设备元数据）。
- 环境量（大气压、温度）**不进同一 Y 轴**；卡片区分组展示。
- 大气压展示优先 `kPa`（保留 2 位）；温度 `°C`（2 位）；差压 `Pa`（默认 3 位，以通道元数据为准）。

详细绘图规则继承 `docs/design/chart-spec.md`。

---

## 6. 通道现值卡片

### 6.1 分组

| 分组 | 内容 | 说明 |
|---|---|---|
| 压力通道 | CH01–CHn 差压 | 主网格，可点显隐曲线 |
| 环境量 | 大气压、温度等 | 与压力分区，避免量级混读 |

### 6.2 卡片结构

```
┌──────────────┐
│ CH05   spark │  ← 色标 + 迷你走势
│  -2.239 Pa   │  ← 数值 mono + tabular；单位 muted
│ 差压   显示中 │  ← 类型 + 曲线可见状态
└──────────────┘
```

| 元素 | 规范 |
|---|---|
| 通道标签 | `CH` + 两位数字；色块背景 = 通道色；白字 |
| 数值 | `--font-family-mono` + `tabular-nums`；字号明显大于标签（约 18px 档） |
| 单位 | 紧随数值，`--text-muted`，约 11px |
| 可见态 | 曲线显示中 → card `active`（teal 描边）；隐藏 → `dim` opacity ~0.42 |
| 间距 | 网格 `gap: var(--space-2)`；卡片 padding `--space-2`～`--space-3` |

### 6.3 命名

- 全站统一：`CH01`、`CH02`…（零填充两位；超过 99 则自然增位）。
- 禁止混用：`CH1` / `CH_01` / `ch-1`。
- 图例、卡片、读数条、导出列名同一字符串。

---

## 7. 设备列表与状态

### 7.1 设备卡

```
标题（设备名）
副标题：型号 · 接口 · 采样率
状态徽章：采集中 / 未记录 / …
在线点
```

- 选中：浅绿描边 + 极淡 tint（`--accent-primary-muted`），不要重阴影。
- 未选中：默认 border；hover 略加强 border。
- 信息优先级：名称 > 状态徽章 > 元数据。

### 7.2 状态色

一律走语义 token，不写死：

| 状态 | Token |
|---|---|
| 成功 / 采集中 / 在线 | `--state-success` / `--accent-success` |
| 警告 | `--state-warning` |
| 错误 / 断开失败 | `--state-error` |
| 信息 | `--state-info`（浅色主题禁止与 success 同色） |

### 7.3 底栏状态

至少包含（可折行，不可省略关键项）：

- 运行态（采集中 / 空闲）
- 设备数
- 当前采样率
- 丢包（或等价健康度）
- 记录状态
- 运行时间、系统时间

---

## 8. Token 映射（原型 → 系统）

原型 HTML 中的硬编码值 **不得** 进入生产组件。映射如下：

### 8.1 颜色与表面

| 原型意图 | 生产 Token |
|---|---|
| 页面灰底 | `--bg-canvas` / `--bg-app` |
| 面板白底 | `--bg-panel` |
| 内嵌条（readout / toolbar 弱底） | `--bg-panel-strong` 或 `color-mix` 自 surface |
| 主文字 | `--text-primary` |
| 次文字 | `--text-secondary` |
| 弱文字 | `--text-muted`（勿作正文） |
| 边框 | `--border-default` / `--border-strong` |
| 品牌/主操作 | `--accent-primary` |
| 危险停止 | `--accent-danger` |
| 成功点/采集 | `--state-success` |
| 通道色 | `--color-channel-1..8`（>8 见 chart-spec 扩展规则） |
| 面板阴影 | `--shadow-panel` |

### 8.2 图表专用（`--chart-*`）

定义于 `styles/tokens/color.css`，主题在 `themes/light.css` / `themes/dark.css` 覆盖：

| Token | 用途 |
|---|---|
| `--chart-bg` | 绘图区背景 |
| `--chart-grid-line` | 主网格 |
| `--chart-grid-line-faint` | 次网格 |
| `--chart-axis-text` | 刻度文字 |
| `--chart-axis-line` | 轴线 |
| `--chart-crosshair` | 悬停十字线 |
| `--chart-cursor` | 用户标记线 |
| `--chart-selection-fill` / `--chart-selection-stroke` | 框选 |
| `--chart-band-warning` / `--chart-band-danger` | 阈值带 |
| `--chart-out-of-range` | 超限段 |

### 8.3 间距与圆角

| 用途 | Token |
|---|---|
| 工作区内边距 | `--space-4` / `--space-5` |
| 面板内边距 | `--space-3` / `--space-4` |
| 卡片网格 gap | `--space-2` |
| 工具按钮 gap | `--space-2` |
| 面板圆角 | `--radius-lg`（12–14px 档） |
| 小芯片/徽章 | `--radius-full` / `--radius-md` |
| 控件高度（非配置表） | 按钮 sm 约 28–32px；与 density 控件高度协调 |

### 8.4 字体

| 用途 | Token |
|---|---|
| UI 文案 | `--font-family-sans` |
| 数值 / 时间轴 | `--font-family-mono` + `tabular-nums` |
| 面板标题 | 13px / weight 700 档（`--text-sm` + bold） |
| 设备名 H1 | 约 18px / weight 700 |

### 8.5 动效

| 用途 | Token |
|---|---|
| hover / 按钮 | `--motion-fast` + `--easing-standard` |
| 面板显隐 | `--motion-base` |
| 实时曲线 | **无动画**（`--motion-instant` / 不插值） |

---

## 9. 组件落点（实现指引）

| UI 块 | 优先组件 / 位置 |
|---|---|
| 顶栏全局操作 | `components/layout/MainTopBar` |
| 设备侧栏 | `components/main/DeviceSidebar` |
| 设备标题 + 本设备操作 | `DeviceDetailPanel` 头区 |
| 实时图 | `RealtimeChart`（读 token，删本地 `CHANNEL_COLORS`） |
| 通道卡 | 可沉淀为 `components/patterns/ChannelValueCard`（跨页复用时） |
| 状态徽章 | `UiStatusBadge` |
| 按钮 | `UiButton` variants |
| 面板容器 | `UiPanel` |

新增 pattern 前先查 `components/ui` 与 `components/patterns`，避免平行实现。

---

## 10. 验收清单（改监控 UI 时勾选）

- [ ] 全局 / 本设备操作位置与文案可区分
- [ ] 停止类按钮不使用成功绿
- [ ] 禁用按钮有原因提示
- [ ] 默认曲线 ≤ 6 路；无「聚焦/全部/异常」三态控件
- [ ] 多通道读数不遮挡 plot
- [ ] 通道名统一 `CH01` 格式
- [ ] 压力与环境量分组
- [ ] 无 feature 内 hex 色 / 随意 spacing 魔法数
- [ ] 浅色、深色各验一次对比度
- [ ] 底栏含采样率、记录状态、运行时间

---

## 11. 与其它规范的关系

| 文档 | 关系 |
|---|---|
| `DESIGN.md` | 总纲；本文件是监控工作区细则 |
| `docs/design/chart-spec.md` | 图表微观规则；本文件定义工作区骨架与 readout 策略 |
| `docs/design/light-theme-palette.md` | 浅色对比度审计 |
| `docs/design/iconography.md` | 图标 |
| `styles/tokens/*` | 唯一视觉数值来源 |
| `docs/ui-redesign-prototype.html` | 视觉参考，**不是**生产实现 |

---

## 12. 变更记录

| 日期 | 说明 |
|---|---|
| 2026-07-12 | 初版：自混合视图改版原型提炼；明确作用域分隔、默认 6 路、图外读数条、去掉聚焦/全部/异常三态 |
