# 标定 / 计量模块 UI 评审报告

> 评审日期：2026-07-09
> 评审范围：`web/src/views/CalibrationView.vue`、`web/src/views/MeasurementView.vue` 及其子组件
> 评审基线：[DESIGN.md](../../DESIGN.md)「清爽薄荷绿白风」设计规范（2026-04-27）
> 评审视角：使用者角度 + UI 设计师角度

---

## 一、整体评价

两个模块共享同一套「清爽薄荷绿白风」设计令牌（mint `#10b981` / slate 灰阶 / DM Sans + JetBrains Mono），布局骨架同构（PageLayout 180px 全局侧栏 → 56px instrument-header → 280px 模块侧栏 + 主工作区），**视觉语言一致性高**。

但站在使用者与 UI 设计师两个角度看，仍有不少可优化点，主要集中在以下四个方面：

1. **信息密度过载**——头部遥测 + 侧栏多层嵌套 + 控制条信息重复
2. **交互焦点不明确**——按钮静态全展示、主操作被参数稀释、报警态替换按钮组
3. **两个模块细节不对齐**——数据表实现、进度指示器、报警入口、空状态不统一
4. **设计规范执行有偏差**——卡片嵌套、mint 面积、字号层级、focus ring、样式系统混合

---

## 二、使用者角度问题清单

### 2.1 标定模块

| 编号 | 问题 | 文件 / 行号 | 影响 |
|------|------|------------|------|
| M1 | **控制按钮静态 6 个全展示**（开始/暂停/继续/停止/拟合/结束），用户需要自己判断当前哪个可用 | [CalibrationControl.vue#L55-L111](../../web/src/components/calibration/CalibrationControl.vue#L55-L111) | 认知负担大；计量模块已做 `primaryAction + secondaryActions` 动态状态机，标定模块没跟上 |
| M2 | **"开始"按钮 disabled 时无解释**，启动条件列表在侧栏底部 | [CalibrationSidebar.vue#L49-L78](../../web/src/components/calibration/CalibrationSidebar.vue#L49-L78) | 用户需视线左右扫描才知道为什么不能点 |
| M3 | **自动模式下"操作"列空了一列**（手动模式才有打压/采集按钮） | [CalibrationDataView.vue#L64-L100](../../web/src/components/calibration/CalibrationDataView.vue#L64-L100) | 表格列浪费空间 |
| M4 | **缺少"采集时间"列**，事后追溯某点何时采集需翻日志 | [CalibrationDataView.vue#L20-L26](../../web/src/components/calibration/CalibrationDataView.vue#L20-L26) | 计量模块有 `cell-time`，标定应同步 |
| M5 | **报告模板选择入口隐藏**：导出按钮直接弹 dialog 选模板，模板状态只在导出条显示一行小字 | [CalibrationView.vue#L94-L108](../../web/src/views/CalibrationView.vue#L94-L108) | 应允许"先选模板→再导出"两步分离，或显式展示当前模板并支持切换 |
| M6 | **ProgressIndicator 6 步在 idle 态也全显示** | [ProgressIndicator.vue](../../web/src/components/calibration/ProgressIndicator.vue) | 用户没启动就看到"完成"步骤，会误以为已完成 |
| M7 | **报警态替换整个按钮组**为 4 个 amber/blue 按钮（确认继续/跳过/重新采集/停止） | [CalibrationControl.vue#L21-L54](../../web/src/components/calibration/CalibrationControl.vue#L21-L54) | 原本熟悉的"暂停/继续"位置消失，破坏肌肉记忆 |

> **编号说明**：原 M7「标定通道偏差仅用文字颜色」经评审确认**保持现状**后撤销，原 M8（报警态替换按钮组）顺位补入 M7。M8 编号保留空缺不重排，避免与历史讨论记录脱钩。详见 [四、修订记录](#四修订记录)。

### 2.2 计量模块

| 编号 | 问题 | 文件 / 行号 | 影响 |
|------|------|------------|------|
| M9 | **"分批模式" toggle 藏在头部 state-chip 旁边** | [MeasurementView.vue#L23-L35](../../web/src/views/MeasurementView.vue#L23-L35) | 第一次使用者难以发现这个核心功能 |
| M10 | **"生成压力表"主按钮与参数输入挤在同一行**末尾 | [MeasurementParamsPanel.vue#L101-L110](../../web/src/components/measurement/MeasurementParamsPanel.vue#L101-L110) | 主操作焦点被参数项稀释 |
| M11 | **报警设置（启用/声音/报警确认）3 个 checkbox 挤在控制条第二排底部**，与"采集通道"并列 | [MeasurementControl.vue#L172-L195](../../web/src/components/measurement/MeasurementControl.vue#L172-L195) | 缺乏层级感；报警阈值/单位没有独立配置入口 |
| M12 | **常规模式与分批模式参数面板位置不同**：常规模式有 `MeasurementParamsPanel`，分批模式 batch-running 阶段没有 | [MeasurementView.vue#L123-L150](../../web/src/views/MeasurementView.vue#L123-L150) 对比 [MeasurementView.vue#L193-L225](../../web/src/views/MeasurementView.vue#L193-L225) | 分批模式的批次执行阶段无法实时调整参数 |
| M13 | **自动模式动态主+副按钮**：状态切换时按钮数量和文案跳变 | [MeasurementControl.vue#L67-L91](../../web/src/components/measurement/MeasurementControl.vue#L67-L91) | 用户视线焦点不稳；主按钮位置应固定，副按钮才动态出现 |
| M14 | **数据表用原生 `<table>` + 固定列宽**，16 通道在 1280px 以下分辨率会横向溢出，且无虚拟滚动 | [MeasurementDataView.vue#L28-L50](../../web/src/components/measurement/MeasurementDataView.vue#L28-L50) | 标定模块用了 el-table，两模块应统一 |
| M15 | **"目标值"列在自动模式下也是可编辑 input** | [MeasurementDataView.vue#L87-L95](../../web/src/components/measurement/MeasurementDataView.vue#L87-L95) | 用户大部分情况不会编辑，input 边框视觉干扰大 |
| M16 | **缺少 `alarm-banner` 顶部横幅**（标定模块有） | [MeasurementView.vue#L74-L227](../../web/src/views/MeasurementView.vue#L74-L227) | 报警只在弹窗里看到，用户关闭弹窗后失去持续可见性 |
| M17 | **BatchProgressBar 的回退确认用全屏 Overlay** 而非 el-dialog | [BatchProgressBar.vue#L39-L61](../../web/src/components/measurement/BatchProgressBar.vue#L39-L61) | 与其他对话框风格不一致 |

### 2.3 两模块共性问题

| 编号 | 问题 | 影响 |
|------|------|------|
| C1 | **头部信息密度过载**：返回按钮 + 标题 + state-chip + 当前压力 + 稳定性 + 稳定计时 + [偏差/分批toggle]，单行扫描距离过长 | 现场作业时视力疲劳 |
| C2 | **侧边栏三层嵌套**：PageLayout 180px 全局侧栏 + 模块 280px 侧栏 + sidebar-section 白卡片 + 内嵌 Device1604Panel 又是白卡片 | DESIGN.md 明确禁止"卡片嵌套"；1280px 屏可用主区不足 800px |
| C3 | **侧栏折叠后无图标提示**：折叠后整列消失，只留小箭头 | 新用户难以找到展开入口；折叠态应保留 icon-only 导航 |
| C4 | **导出报告入口位置不一致**：标定在数据区底部 `template-bar`，计量在控制按钮组里 | 用户跨模块切换时找不到入口 |
| C5 | **空状态文案与图标不一致**：标定用 SetUp 图标，计量用 el-empty | 应统一空状态组件 |
| C6 | **快捷键缺失**：开始/暂停/停止/采集 等高频操作无键盘快捷键 | 现场操作效率低 |
| C7 | **错误反馈只走 ElMessage 浮窗**，错误位置不直观 | 如某点采集失败，用户需在表格里自己找 |
| C8 | **稳定倒计时仅文字+进度条**，缺少骨架屏或脉冲动画反馈"系统正在工作" | 长时间稳定等待时用户怀疑系统卡死 |
| C9 | **focus ring 缺失**：`<button type="button">` 普遍没有可见的 focus 视觉 | 键盘可达性差，不符合无障碍标准 |
| C10 | **响应式断点缺失**：两个模块在 1280px 以下分辨率下主工作区被压缩，16 通道表格溢出 | 工业 PC 常见 1366×768，需明确断点策略 |
| C11 | **i18n 缺失**：两模块大量硬编码中文文案（按钮、标签、状态、报警提示），未走 i18n 通道 | 与正在推进的五孔/三孔校准配置界面国际化工作不一致，跨语言环境无法复用 |
| C12 | **a11y 语义补强不足**：报警态按钮无 `aria-label`、状态切换无 `aria-live` 公告、报警 dot 仅靠颜色区分 | 屏幕阅读器用户无法感知报警与状态变化，不符合无障碍标准 |

---

## 三、UI 设计师角度问题清单

### 3.1 设计规范执行偏差

| 编号 | 偏差点 | DESIGN.md 规定 | 实际情况 | 文件 |
|------|--------|---------------|----------|------|
| D1 | **卡片嵌套** | 禁止卡片嵌套 | `card-block` 包裹 `CalibrationParams` + `CalibrationControl`，而两者本身又是带背景的卡片 | [CalibrationView.vue#L82-L87](../../web/src/views/CalibrationView.vue#L82-L87) |
| D2 | **mint 使用面积 ≤10%** | One Voice Rule | "开始"按钮 + chip-dot + 进度条填充 + 报警 dot + 导出按钮 hover + 6 步骤 active marker，mint 色块累计可能超标 | 全局 |
| D3 | **字号层级 1.25× 跳变** | Flat Scale Rule | compact-input 输入值、telem-value、按钮文案都是 14px，缺少字重/字号层级 | 全局 |
| D4 | **状态标签规范** | 4px 圆角 + 15% 透明背景 + 30% 透明边框 + 400 级语义色文字 | state-chip 用了实心 chip-dot + 不透明背景，与规范有偏差 | [CalibrationView.vue#L18-L24](../../web/src/views/CalibrationView.vue#L18-L24) |
| D5 | **阴影默认扁平** | 阴影仅在交互态出现 | card-block 顶部 1px mint accent 线 + 默认阴影同时存在，弱化了 hover 反馈 | 全局 |
| D6 | **动效时长未定义** | 150–350ms ease，禁止 bounce/elastic | 进度条 `scaleX` 250ms ease 符合；但 chip-dot 脉冲动画时长未定义 | 全局 |

### 3.2 视觉一致性问题

| 编号 | 问题 | 详情 | 文件 |
|------|------|------|------|
| D7 | **两模块进度指示器风格差异大** | 标定用 6 步圆形 marker + 连接线，计量分批用 4 阶段横向 ol + index 数字 + label | [ProgressIndicator.vue](../../web/src/components/calibration/ProgressIndicator.vue) 对比 [MeasurementView.vue#L81-L99](../../web/src/views/MeasurementView.vue#L81-L99) |
| D8 | **数据表实现不一致** | 标定用 el-table + border + stripe，计量用原生 `<table>` + 自定义样式，两套样式系统并存 | [CalibrationDataView.vue](../../web/src/components/calibration/CalibrationDataView.vue) 对比 [MeasurementDataView.vue](../../web/src/components/measurement/MeasurementDataView.vue) |
| D9 | **样式系统混合** | 大部分组件用 SCSS 变量（`$mint`、`$slate-200`），但 DevicePanel / MeasureDeviceCard / PressureDeviceCard / AlarmChannelSelectDialog / MeasurementPointFlow 用 CSS 自定义属性（`var(--bg-secondary)`） | 多文件 |
| D10 | **Dialog 与 Overlay 混用** | 报警确认、通道选择、模板选择用 el-dialog，但 BatchProgressBar 回退确认用全屏 Overlay | [BatchProgressBar.vue#L39-L61](../../web/src/components/measurement/BatchProgressBar.vue#L39-L61) |
| D11 | **疑似未使用的遗留组件** | 标定的 `AlarmConfigPanel.vue` / `ManualControlPanel.vue`，计量的 `MeasurementPointFlow.vue` / `DevicePanel.vue` / `MeasureDeviceCard.vue` / `PressureDeviceCard.vue` 在各自 View 中未见引用 | 见 [六、待确认遗留组件](#六待确认遗留组件) |

> **注**：原 D8「两模块通道超限高亮策略不一致」已撤销——这是**有意为之的业务差异**，非设计缺陷。标定用文字颜色（关注偏差→拟合），计量用红色背景（合格性判定，强视觉警示）。详见 [四、修订记录](#四修订记录)。

### 3.3 信息架构问题

| 编号 | 问题 | 建议 |
|------|------|------|
| D12 | **头部遥测与控制条进度信息重复** | 头部已有"稳定计时"，控制条又有"稳定标签"，应合并或分工明确（头部=全局状态，控制条=任务进度） |
| D13 | **启动条件列表与控制按钮空间分离** | 应在"开始"按钮 disabled 时通过 tooltip 或 inline hint 直接说明原因，减少视线跳跃 |
| D14 | **报警配置散落** | 报警启用/声音/确认在控制条，报警阈值/单位只在弹窗里看到，应整合为独立"报警配置"区块 |

---

## 四、修订记录

### 4.1 撤销项

| 日期 | 修订内容 | 原因 |
|------|---------|------|
| 2026-07-09 | 撤销原 M7（标定通道偏差仅用文字颜色） | 经评审确认，标定关注"偏差→拟合"，文字颜色辅助即可，背景高亮反而干扰拟合数据视觉连续性。**保持现状**。原 M8（报警态替换按钮组）顺位补入 M7，M8 编号保留空缺。 |
| 2026-07-09 | 撤销原 D8（两模块通道超限高亮策略不一致） | 这是**有意为之的业务差异**，非设计缺陷。标定=文字颜色（偏差辅助），计量=红色背景（合格性强警示）。 |

### 4.2 细化项

| 日期 | 修订内容 | 原因 |
|------|---------|------|
| 2026-07-09 | P0-1 调整为「仅统一数据表实现方式，不统一超限策略」 | 配合原 D8 撤销——超限高亮策略保持两模块各自独立，只统一 el-table 实现层。 |
| 2026-07-09 | 新增 C11（i18n 缺失）、C12（a11y 语义补强不足） | 补齐国际化与无障碍维度，与正在推进的五孔/三孔校准配置界面国际化工作对齐。 |

---

## 五、优化建议（按优先级）

### P0 — 立刻该修（影响可用性 / 设计规范违规）

| # | 建议 | 关联问题 | 预期收益 |
|---|------|---------|---------|
| P0-1 | **统一数据表实现**：计量模块的 `MeasurementDataView.vue` 改用 el-table + 共用 `@/styles/calibration-table` mixin（**仅统一实现，不统一超限策略**）；16 通道 × N 点场景下同步启用 `el-table-v2` 虚拟滚动，避免长列表性能瓶颈 | M14、D8 | 消除两模块最大视觉不一致，并解决 1280px 以下横向溢出 |
| P0-2 | **拆解卡片嵌套**：`CalibrationView.vue#L82-L87` 的 `card-block` 包裹 Params + Control 改为两个独立 card-block | D1、C2 | 符合 DESIGN.md 规范，释放主工作区空间 |
| P0-3 | **标定控制按钮采用动态状态机**：参考计量模块 `primaryAction + secondaryActions` 模式（实现见 [MeasurementControl.vue#L67-L91](../../web/src/components/measurement/MeasurementControl.vue#L67-L91)），主按钮固定位置，副按钮动态出现 | M1、M13 | 提升标定模块可用性，与计量对齐 |
| P0-4 | **报警配置独立成卡片**：计量模块的报警 checkbox + 阈值/单位整合为独立 `AlarmConfigPanel`，从控制条移出 | M11、D14 | 报警配置层级清晰 |
| P0-5 | **删除/归档遗留组件**：经 Grep 确认 `AlarmConfigPanel.vue` / `ManualControlPanel.vue` / `MeasurementPointFlow.vue` / `DevicePanel.vue` / `MeasureDeviceCard.vue` / `PressureDeviceCard.vue` 均无 import 引用（仅自身定义文件命中），属真实死代码。其中 `AlarmConfigPanel.vue` 由 P0-4 接管改造为计量模块报警配置卡片，**不删除**；其余 5 个直接删除 | D11、D9 | 避免样式系统混合，降低维护成本 |

### P1 — 应该修（影响效率 / 一致性）

| # | 建议 | 关联问题 |
|---|------|---------|
| P1-1 | **统一进度指示器风格**：标定的 6 步 ProgressIndicator 与计量的 4 阶段 batch-stepper 抽象为公共 `StepIndicator` 组件，支持圆形 marker 和横向 index 两种 variant | D7 |
| P1-2 | **"开始"按钮 disabled 时显示 tooltip**：列出未满足的启动条件 | M2、D13 |
| P1-3 | **标定数据表补"采集时间"列**，自动模式隐藏"操作"列 | M3、M4 |
| P1-4 | **计量模块补 `alarm-banner` 顶部横幅**，与标定一致 | M16 |
| P1-5 | **导出报告入口统一**：两模块都在数据区底部 `template-bar` 提供导出，控制条不再放导出按钮 | C4 |
| P1-6 | **"分批模式" toggle 移到更显眼位置**：建议作为参数面板上方的模式切换 segment-control，与"控制模式 自动/手动"并列 | M9 |
| P1-7 | **侧栏折叠态保留 icon-only 导航**：折叠后显示设备/参数/报警的图标，点击展开 | C3 |
| P1-8 | **快捷键支持**：Space=开始/暂停、Esc=停止、Ctrl+E=导出、Ctrl+S=保存配置 | C6 |
| P1-9 | **i18n 接入**：两模块按钮、标签、状态、报警提示文案统一走 i18n 通道，与五孔/三孔校准配置界面国际化工作对齐 | C11 |

### P2 — 可以修（提升体验）

| # | 建议 | 关联问题 |
|---|------|---------|
| P2-1 | **稳定等待时增加骨架屏/脉冲动画** | C8 |
| P2-2 | **空状态统一**：两模块都用 `el-empty` + 一致的图标和文案 | C5 |
| P2-3 | **focus ring 全局补齐**：所有 `<button>` 添加可见 focus 视觉 | C9 |
| P2-4 | **响应式断点**：定义 1280px / 1440px / 1920px 三档，1280px 时侧栏自动折叠 | C10 |
| P2-5 | **头部信息分组**：返回+标题+状态为一组，遥测数据为一组，中间用更大间距分隔 | C1 |
| P2-6 | **清理样式系统**：将遗留组件的 CSS 自定义属性统一改为 SCSS 变量 | D9 |
| P2-7 | **批次进度回退确认改用 el-dialog**，与 BatchVerificationDialog 风格一致 | M17、D10 |
| P2-8 | **ProgressIndicator idle 态仅显示第 1 步 active**，其余灰显 | M6 |
| P2-9 | **"目标值"列自动模式下显示为纯文本**，hover 时才出现编辑入口 | M15 |
| P2-10 | **错误反馈就地高亮**：采集失败的行/通道直接标红，不只走 ElMessage | C7 |
| P2-11 | **a11y 语义补强**：报警态按钮补 `aria-label`、状态切换区补 `aria-live="polite"`、报警 dot 增加非颜色冗余（如文字标记） | C12 |

---

## 六、已确认遗留组件

以下组件经 Grep 全局搜索确认**无 import 引用**（仅自身定义文件命中），属真实死代码：

| 模块 | 组件 | 路径 | 处置 |
|------|------|------|------|
| 标定 | `AlarmConfigPanel.vue` | [web/src/components/calibration/AlarmConfigPanel.vue](../../web/src/components/calibration/AlarmConfigPanel.vue) | 由 P0-4 改造复用，不删除 |
| 标定 | `ManualControlPanel.vue` | [web/src/components/calibration/ManualControlPanel.vue](../../web/src/components/calibration/ManualControlPanel.vue) | 删除 |
| 计量 | `MeasurementPointFlow.vue` | [web/src/components/measurement/MeasurementPointFlow.vue](../../web/src/components/measurement/MeasurementPointFlow.vue) | 删除 |
| 计量 | `DevicePanel.vue` | [web/src/components/measurement/DevicePanel.vue](../../web/src/components/measurement/DevicePanel.vue) | 删除 |
| 计量 | `MeasureDeviceCard.vue` | [web/src/components/measurement/MeasureDeviceCard.vue](../../web/src/components/measurement/MeasureDeviceCard.vue) | 删除 |
| 计量 | `PressureDeviceCard.vue` | [web/src/components/measurement/PressureDeviceCard.vue](../../web/src/components/measurement/PressureDeviceCard.vue) | 删除 |

**确认方法**：在 `web/src/` 全局搜索组件名，若仅自身定义文件命中、无 import 引用，则判定为遗留组件。

---

## 七、最小启用集建议

如果只能挑 3 件事先做，建议：

1. **P0-1 统一数据表实现**（仅 el-table 化，不统一超限策略）
2. **P0-3 标定控制按钮采用动态状态机**
3. **P0-5 删除遗留组件 + P2-6 清理样式系统**

这三件事改完后，两个模块的视觉一致性与代码可维护性会有显著提升，再迭代 P1/P2 时阻力更小。

---

## 八、备注

- 本评审基于代码静态分析，建议补充**真实运行截图**对照（`docs/screenshots/e2e/` 目录下有 e2e 截图可参考）。
- 部分问题（如 mint 面积超标、字号层级）需要实际渲染才能确认，建议用浏览器 DevTools 进行视觉回归。
- DESIGN.md 的"清爽工作台"北极星定位很好，但执行层面需要补一份**组件级 Do/Don't 清单**，避免后续开发再次出现卡片嵌套、样式系统混合等问题。
- 后续修复每个问题时，建议遵循 AGENTS.md 第 10 节「AI 协作流程」——关键设计决策落地到 `docs/plans/`，上下文接近上限时主动提示开新 session 并附"续接摘要"。

---

## 九、问题统计

| 类别 | 数量 |
|------|------|
| 使用者角度 - 标定模块 | 7 项（M1-M7，M8 编号空缺） |
| 使用者角度 - 计量模块 | 9 项（M9-M17） |
| 使用者角度 - 共性 | 12 项（C1-C12） |
| 设计师角度 - 规范偏差 | 6 项（D1-D6） |
| 设计师角度 - 视觉一致性 | 5 项（D7-D11） |
| 设计师角度 - 信息架构 | 3 项（D12-D14） |
| **合计** | **42 项**（撤销 2 项后由新内容替换，实际有效问题 42 项） |
| 优化建议 P0 | 5 项 |
| 优化建议 P1 | 9 项 |
| 优化建议 P2 | 11 项 |
| **建议合计** | **25 项** |
