# WindLabX4 设计系统整改计划

> 状态基线核对日期：2026-07-12（基于 `apps/desktop-wails/frontend/src` 实际代码）
> 关联文档：`DESIGN.md` / `docs/design/light-theme-palette.md` / `docs/design/iconography.md` / `docs/design/chart-spec.md` / `docs/design/monitor-workspace-spec.md` / `docs/ui-design-audit.md`
> 修订：2026-07-12 review 后修正（D3 通道色策略、Phase 0 体量、App.vue brand 分裂、B/Phase 5 关系等）

## 0. 目的与现状基线

WindLabX4 **已经有一套文档 + 代码双轨落地的设计系统**（token 层、主题层、Ui* 原语、图标规范、密度规范均齐备）。本计划只收口**已知欠账**，避免重复造轮子。

**重要修正**：`light-theme-palette.md`（2026-06 快照）里「浅色缺 8 通道色 / 缺图表 token」的填坑项，**代码侧已大部分落地**——`styles/themes/light.css` 现在已含 `--color-channel-1..8` 与 `--chart-*`。因此本计划不再把这些列为待办，仅保留其中**真正残留**的子项（见 A 系列）。

### 0.1 代码侧已知真问题（2026-07-12 实测）

| 问题 | 证据 | 严重度 |
|---|---|---|
| 浅色 `info === success` | `light.css` 两者均为 `#22c55e` | P0 |
| Naive 主色与 token 品牌分裂 | token primary `#10b981`；`App.vue` light primary `#22c55e`、dark primary `#38bdf8` | P0 |
| rail 图标字符串间接层 | `MainDashboardView` 用 `IO/CP/TR/LG/AX`，`AppRailNav.getIconComponent` switch | P1 |
| 通道色逻辑正确但色值硬编码 | `utils/channelColors.ts` 含 `PRESSURE_PALETTE` / `TEMPERATURE_PALETTE` / `CHANNEL_COLORS` 字面 hex | P1 |
| feature 字面 hex 扩散 | 约 25 个 vue/ts 文件；最重 `CalibrationHome.vue`(62)、`ProbeReferenceCard.vue`(44) | P1 |
| `--text-muted` 大量用于标签 | 约 443 处 / 54 文件（体量大，不可当小 commit） | P1 |
| 主题三层真源不清 | `color.css` `:root` 为 dark；`dark.css` 同时挂 `:root` + `[data-theme='dark']`；`light.css` 为覆盖 | P2（大改） |

## 1. 已确认完成（不再列入整改）

- 浅色主题通道色 8 个 token 已写入 `light.css`。
- 浅色主题图表 token 块（`--chart-*`）已写入 `light.css`。
- 浅色主题空间轴 `--axis-x..u` 已写入 `light.css`。
- 浅/深双主题切换机制（`[data-theme='light'|'dark']`）已就绪。
- `glass.css` 已有浅色 header/footer/sidebar/rail 覆盖（问题在配方，不是「完全未定义」）。

## 2. 待整改项总览

| 工作流 | 主题 | 量级 | 风险 |
|---|---|---|---|
| **A** | 浅色主题收尾 + brand 对齐 | 小→中 | 低→中 |
| **B** | 主题基线翻转（独立大改） | 大 | 高 |
| **C** | 图标两套并行清理 | 中 | 低 |
| **D** | 硬编码色值迁移（保留通道色 API） | 大 | 中 |

## 3. 详细整改清单

### A. 浅色主题收尾 + brand 对齐

#### A1 — `--accent-info` 与 `--accent-success` 撞色（P0）

- **现状**：`styles/themes/light.css` 中 `--accent-info: #22c55e` 与 `--accent-success: #22c55e` 相同。
- **目标**：info 改为蓝色（提案 `#0369a1` sky-700，白底 ≥ AA）。
- **动作**：
  1. 改 `light.css` 的 `--accent-info`。
  2. 同步 `App.vue` 的 `infoColor` 浅色分支（当前也是 `#22c55e`）。
- **验收**：两 token 值不同；info 文本/图标场景白底对比度 ≥ 4.5:1。
- **工作量**：约 0.5h。

#### A6 — `App.vue` Naive 主题覆盖与 token brand 对齐（P0，A1 配套）

- **现状**：CSS token 品牌色是 emerald `#10b981`；Naive `primaryColor` 浅色却是 `#22c55e`、深色是 `#38bdf8`。结果是「按钮/输入 focus 一条色，页面 chrome 另一条色」。
- **目标**：Naive 覆盖从 token 语义色派生，不再另立一套 brand。
  - light primary → `#10b981`（或 token 等价）
  - dark primary → 与 `--accent-primary` 一致（当前 token 也是 `#10b981`；若产品坚持 dark 用 info 蓝，须先改 token 再改 App.vue，禁止只改一边）
  - success / warning / danger / info 全部对齐对应 accent token
- **动作**：改写 `App.vue:11–82` 的 `themeOverrides`，尽量读 CSS 变量或集中映射表；禁止继续手写与 token 冲突的 hex。
- **验收**：浅/深主题下 Naive Button primary、Input focus、Select focus 与页面 accent 一致。
- **工作量**：约 1–2h。
- **备注**：B3 是「token 基线翻转后删除覆盖」；A6 是「在翻转前先消除 brand 分裂」。两者不冲突。

#### A2 — 浅色通道色设计师签字（P1）

- **现状**：`light.css` 已有 8 通道色，但与 `light-theme-palette.md` 提案有偏差（例：channel-6 现用 `#b45309`，提案 `#9a3412`）。
- **目标**：8 色在白底互区分、≥4.5:1、通过 deuteranopia 模拟。
- **动作**：设计师对**下表现状值**签字或给出定稿；必要时回写 `light.css`。

| Channel | light.css 现状 | palette 提案 | 决策 |
|---|---|---|---|
| 1 | `#1d4ed8` | `#1d4ed8` | 待签 |
| 2 | `#0d9488` | `#0d9488` | 待签 |
| 3 | `#3730a3` | `#3730a3` | 待签 |
| 4 | `#a16207` | `#a16207` | 待签 |
| 5 | `#c2410c` | `#c2410c` | 待签 |
| 6 | `#b45309` | `#9a3412` | **有偏差，待定** |
| 7 | `#9333ea` | `#9333ea` | 待签 |
| 8 | `#15803d` | `#15803d` | 待签 |

- **验收**：签字记录 + 1.5px 线宽白底区分度 + 色觉缺陷模拟。
- **工作量**：设计 0.5–1d；工程改值约 0.5h。

#### A3 — `--bg-canvas` vs `--bg-app` 决策（P2）

- **现状**：二者均为 `#f8fafc`。
- **目标**：决定 canvas 是否内凹为 `#f1f5f9`。
- **动作**：产品/设计决策后改 `light.css`（及后续翻转后的 `color.css`）。
- **验收**：决策记录 + 双主题目视检查 shell。
- **工作量**：决策 + 改值约 0.5h。

#### A4 — `--text-muted` 误用审计（P1，**非小 commit**）

- **现状**：浅色 `--text-muted: #94a3b8` 白底约 2.6:1，不达标；实际约 **443 处 / 54 文件** 使用，大量用于标签/meta 文案。
- **目标**：仅装饰用途保留 `--text-muted`；标签/正文改 `--text-secondary`。
- **动作**：
  1. 先定规则：装饰（分隔符、占位图标、非必要 hint）可 muted；可读文案必须 secondary。
  2. 按模块分 PR：校准 → 遍历 → 设备监控 → shell。
  3. 禁止一次性全仓替换。
- **验收**：高流量页面（Dashboard、设备详情、校准主屏、遍历主屏）无「muted 当正文」；抽样对比度通过。
- **工作量**：约 1–2d，分 3–4 个 PR。
- **排期**：Phase 1+，**不要**放进 Phase 0。

#### A5 — 浅色玻璃效果重调（P2）

- **现状（以当前 `glass.css` 为准，非 6 月推测）**：
  - light header/footer：`rgba(255,255,255,0.75)` + blur，border 偏弱。
  - light sidebar：`rgba(248,250,252,0.8)` 近乎实色却仍挂 glass。
  - light rail：`rgba(255,255,255,0.5)` 发虚。
  - `.app-bg-gradient` 仍硬编码 hex。
- **目标**（对齐 palette 建议）：
  - header/footer：`rgba(255,255,255,0.78/0.85)` + `blur(10px)`，border 用 `rgba(15,23,42,0.08)`。
  - rail / sidebar：**取消 glass**，改实色 `--bg-panel-strong` / `--bg-panel`。
  - gradient 改 token 或删除装饰渐变。
- **动作**：改 `glass.css` + 核对 layout 组件 class 引用。
- **验收**：浅色下 rail/sidebar 不发雾；header/footer 仍可有轻玻璃；双主题截图。
- **工作量**：约 0.5–1d。

#### A7 — 补 `--accent-primary-text`（P2，可选）

- **背景**：palette 提案在浅色下拆 fill-only primary 与 text-safe primary（`#047857`），因 mid-sat 品牌色当 12px 文本不过 AA。
- **动作**：若审计发现 primary 当文本用的场景，增加 `--accent-primary-text` 并替换调用点。
- **验收**：primary 文本场景 ≥ AA。

### B. 主题基线翻转（独立大改，≠ audit Phase 5）

> **关系澄清**：`docs/ui-design-audit.md` 的 Phase 5 是**逐页清理顺序**（shell → device → storage → motion → traversal → calibration），**本身没有写 invert baseline**。invert 来自 `light-theme-palette.md` 的「Invert the baseline」。  
> **策略**：invert 可与 Phase 5 **同一 release 推进**，但必须是**独立 PR / 独立验收**，不能把「翻基线」和「逐页改 UI」糊成一个 diff。

#### B0 — 先理清三层真源（动手前置）

当前实际结构：

| 文件 | 选择器 | 实际角色 |
|---|---|---|
| `tokens/color.css` | `:root` | dark 默认值（真源之一） |
| `themes/dark.css` | `:root, :root[data-theme='dark']` | 再次声明 dark（与 color.css 重叠） |
| `themes/light.css` | `:root[data-theme='light']` | light 覆盖层 |

#### B1 — `:root` 改为浅色基线

- 将 `color.css` 的 `:root` 改写为浅色生产值（含 A1/A2/A6 定稿后的 accent / channel / chart）。

#### B2 — dark 只保留覆盖层

- `dark.css` **去掉** `:root` 双挂，仅保留 `[data-theme='dark']`（或 `:root[data-theme='dark']`）。
- 删除与 `color.css` 重复且无暗色差异的声明，避免三处维护。

#### B3 — 删除或最小化 `App.vue` 手写覆盖

- 在 A6 对齐且 B1/B2 完成后，删除仅为对抗旧 dark 基线而存在的 Naive 覆盖。
- 若 Naive 仍需要 overrides，应改为「读 token 的薄映射」，禁止平行 hex 表。

#### B4 — 逐屏双主题验证

- 所有屏幕在两种主题下无「白底白线 / 数据消失 / brand 色分裂」。
- 验收命令：`npm run typecheck` + `npm run build`；关键页双主题截图清单（Dashboard、设备详情、校准首页、五孔主屏、遍历主屏、日志）。

- **风险**：大改；禁止与功能变更合并；依赖 A1、A6、A2（至少定稿通道色）、D 系列部分完成（尤其 channelColors token 化）。
- **工作量**：约 2–4d（含验证）。

### C. 图标两套并行清理

#### C1 — 移除左 rail 两字母占位（P1）

- **现状**：`MainDashboardView.vue` 使用 `icon: 'IO'|'CP'|'TR'|'LG'|'AX'`；`AppRailNav.vue` 的 `getIconComponent` switch 解析。
- **目标**：直接传组件引用或统一 icon key 映射表（单点定义，不跨文件 switch）。
- **动作**：改两文件；删除/内联 switch。
- **验收**：rail 无字母 fallback；新增 rail 项无需改 switch。
- **工作量**：约 1–2h。

#### C2 — 弃用 4 个冗余自绘图标（P1，可跨发布周期）

- `IconDashboard` → Lucide `LayoutDashboard`
- `IconMotion` → Lucide `Move3d` 或 `Joystick`（见 Open Decisions）
- `IconLog` → Lucide `ScrollText`
- `IconStorage` → Lucide `Database` 或 `HardDrive`（先确认是否仍有引用；若已死代码可直接删）
- **策略**：先改引用；文件保留一个发布周期后删除（或确认零引用后立即删）。

#### C3 — 保留 5 个领域自绘图标（**维持，非弃用**）

- `IconCalibrationFiveHole` / `IconCalibrationThreeHole` / `IconCalibrationTotalPressure` / `IconCalibrationTotalTemperature` / `IconTraversal`
- 无库等价物；规则见 `iconography.md`。

#### C4 — 语义图标映射审计（P1）

- 按 `iconography.md` 语义表核对调用点（例：实时信号用 `Activity`，不用 `LineChart`）。
- **工作量**：约 0.5–1d 抽样 + 修复。

#### C5 — 创建 `components/icons/index.ts`（P2）

- **现状**：该文件**不存在**。
- **目标**：新建并 re-export 自绘图标 + 简短使用注释（何时用自绘、何时用 Lucide）。
- **验收**：从 `@components/icons` 可发现 5 个领域图标；冗余图标若已弃用则不导出或标 `@deprecated`。

### D. 硬编码色值迁移（保留通道色 API）

> **D3 策略修正（关键）**：`utils/channelColors.ts` **不是该删的平行数组**。它是产品逻辑：
>
> - `PRESSURE_PALETTE`（16 档，DAQ-P-1603 压力）
> - `TEMPERATURE_PALETTE`（4 档，温度暖色）
> - `CHANNEL_COLORS`（8 色，非 1603 设备）
> - `buildChannelColorMap` + 单测保证「通道 index → 颜色」稳定
>
> **正确目标**：保留 API 与映射逻辑；把**字面 hex 迁到 token / 运行时读 CSS 变量**；图表消费方继续走 `buildChannelColorMap`，不要再散落第二套色表。

#### D1 — lint / CI 分三步落地（P1）

1. **warn + allowlist**：新增规则检测 feature 内字面 hex；先 warn。
2. **分模块清债**：按 D2 顺序降 allowlist。
3. **error**：清完后升 error。

**例外白名单（DoD 必须保留）**：

- canvas / WebGL / 截图导出等**无法直接吃 CSS var** 的路径（可集中在 `utils/` 用「读 computed style → hex」辅助函数）。
- 测试 fixture 中的期望色值（优先改为读 token 解析结果，短期可 allowlist）。
- SVG path 数据本身（非 fill/stroke 硬编码色）。

#### D2 — 按债务分布迁移（P1）

优先序按**实际字面 hex 密度**，不是旧 spec 点名的两个文件：

1. **视觉债最大**：`CalibrationHome.vue`、`ProbeReferenceCard.vue`、`PointsPreview.vue`、traversal 可视化主题（`useTraversalChartTheme.ts` 等）——营销色/卡片色改语义 token 或局部 theme map（map 内只引用 token）。
2. **系统债**：`utils/channelColors.ts` 色板 token 化（见 D3）；`RealtimeChart.vue` / `DeviceDetailPanel.vue` 确认只消费 map，无旁路 hex。
3. **反馈/壳层**：`UiToastHost.vue`、`MainBottomBar.vue`、其余长尾文件。

每批 PR 验收：`npm run typecheck` + `npm run build`；双主题 spot check 相关页。

#### D3 — 通道色 token 化（保留 API）（P1）

- 在 `tokens/color.css` + light/dark 中扩展通道/传感器色板 token（或 `--color-channel-*` 扩展到 16 + 温度 4 档，命名需写进 `chart-spec` / 本文件附录）。
- `channelColors.ts`：`PRESSURE_PALETTE` / `TEMPERATURE_PALETTE` / `CHANNEL_COLORS` 改为读取 token（构建期注入或运行时 `getComputedStyle`），**禁止**再维护第二份互不相同的 hex 真相源。
- 消费方继续 `import { buildChannelColorMap, CHANNEL_COLORS } from '@utils/channelColors'`；单测更新为「与 token 解析结果一致」。
- **验收**：
  - API 行为不变（同 profile + 同 channels → 同色）。
  - 色值唯一真源在 CSS token。
  - 无第二套 `CHANNEL_COLORS = ['#...']` 硬编码真相源。

#### D4 — 图表 token 消费对齐（P1）

- 网格/十字线/轴文字/阈值带只走 `--chart-*`。
- 禁止组件内平行 chart 色常量。

## 4. 分阶段路线

| 阶段 | 内容 | 阻塞 / 依赖 | 预估 |
|---|---|---|---|
| **Phase 0（立刻，小 commit）** | A1、A6、C1、C5、D1-step1（warn+allowlist） | 无 | 0.5–1d |
| **Phase 1（浅色生产化 + 清债启动）** | A2 签字、A5、A4 首批模块、D2 第 1 批、D3 启动 | A2 需设计签字 | 3–5d |
| **Phase 2（图标 + 清债继续）** | C2、C4、A4 剩余、D2 第 2–3 批、D3 完成、D1-step2 | C2 可跨发布周期 | 3–5d |
| **Phase 3（基线翻转，独立大改）** | B0–B4；D1-step3 升 error | 依赖 A1/A6/A2/D3；可与 audit Phase 5 同 release，但独立 PR | 2–4d |

## 5. 完成定义（Definition of Done）

- [ ] 浅色主题全屏幕生产可用，关键页无对比度失败（WCAG AA 文本）。
- [ ] `--accent-info` ≠ `--accent-success`；Naive primary 与 CSS `--accent-primary` 双主题一致。
- [ ] 8 通道浅色经设计签字；双主题 ≥AA 且互区分（含色觉缺陷模拟）。
- [ ] `channelColors` API 保留；色值真源在 token；单测通过。
- [ ] feature 字面 hex 清到白名单内；lint 最终为 error。
- [ ] 图标：Lucide 为主 + 5 个领域自绘；rail 无 `IO/CP/...` 字母间接层。
- [ ] `:root` 为浅色基线；dark 仅为覆盖层；`App.vue` 无平行 brand hex 表。
- [ ] 验收命令通过：`npm run typecheck`、`npm run build`；关键页双主题截图归档。

## 6. 待决策 / 需签字（Open Decisions）

- [ ] 设计师对 8 个浅色通道 hex 签字（见 A2 对照表）。
- [ ] `--bg-canvas` 是否区别于 `--bg-app`。
- [ ] dark 主题 primary 是否保持 emerald，还是产品要 sky（若要 sky，先改 token 再改 Naive）。
- [ ] motion 图标用 `Move3d` 还是 `Joystick`。
- [ ] 冗余自绘图标：立即删（零引用）还是保留一发布周期。
- [ ] 是否引入图标预览（Storybook 类）还是保持代码搜索。
- [ ] invert baseline 的 release 排期（与 audit Phase 5 同窗或错开）。
- [ ] 1603 压力 16 档 / 温度 4 档 token 命名方案（D3）。

## 7. 风险与依赖

- **B 系列是大改**：独立于功能变更；与 audit Phase 5 是「可同 release」而非「同一任务」。
- **D 系列范围大**：按模块拆 PR；**禁止**以「删除 `CHANNEL_COLORS`」为目标。
- **A4 体量大**（443 处）：禁止塞进 Phase 0。
- **A2 可能阻塞浅色定稿**：签字前通道色不轻易全量改值。
- **A5 以当前 `glass.css` 为准**：勿基于 2026-06 palette 文字臆测实现。
- **canvas 例外**：通道色 token 化后，图表仍可能需要解析为 hex 才能画 canvas；集中 helper，避免组件内手写 fallback hex 散落。

## 8. 验收命令与截图清单

### 8.1 命令

在 `apps/desktop-wails/frontend`：

```bash
npm run typecheck
npm run build
```

相关单测（通道色）：

```bash
# 按项目现有测试命令执行 channelColors 相关测试
npm test -- channelColors
```

（若仓库实际脚本名不同，以 `package.json` 为准。）

### 8.2 双主题截图清单（B4 / 大 PR 必做）

1. Main shell（header / rail / sidebar / footer）
2. Dashboard 设备总览
3. Device detail + RealtimeChart
4. Calibration home
5. Five-hole main（运行中若可）
6. Traversal main / 可视化
7. Log viewer

每张含 light + dark。

## 9. 附录：与旧文档差异

| 旧说法 | 现结论 |
|---|---|
| 浅色缺 8 通道色 / chart token | **已落地**，不再待办 |
| 只清 `RealtimeChart` / `DeviceDetailPanel` 硬编码 | 债务主要在校准卡片 + `channelColors.ts` 色板 |
| 禁止平行 `CHANNEL_COLORS` 数组 = 删 API | **错误**；应 token 化色值、保留 API |
| A4 可进 Phase 0 | **错误**；443 处，Phase 1+ |
| invert = audit Phase 5 | **不准确**；可同 release，非同一任务 |
| `icons/index.ts` 加注释 | 文件不存在，应**创建** |
| glass 浅色未定义 | 已有覆盖；问题是配方与 rail/sidebar 不该 glass |
