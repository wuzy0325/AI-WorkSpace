# Plan: Wind-DAQ 监控工作区 UI 落地

> 阶段：Phase 2 — Plan（**待人审阅，未编码**）  
> Spec：`docs/specs/spec-monitor-workspace-ui.md`（已审阅）  
> 设计真源：`docs/design/monitor-workspace-spec.md`

## 1. 决议摘要

| 项 | 决策 |
|---|---|
| 顶栏停止文案 | **停止全部采集** |
| 默认曲线 | 非大气 enabled 通道前 **6** 路 |
| 卡片压力/环境分组 | **本迭代不做**（下迭代） |
| 断开确认 | `feedbackStore.confirm(...)` |
| 本迭代交付 | P0 作用域 + P1 默认 6 路/命名 + P2a readout & chart token |

## 2. 现状关键点（实现约束）

| 点 | 现状 | 计划动作 |
|---|---|---|
| `MAX_CHART_CHANNELS` | `deviceStore.ts` = **16**；默认初始化 `slice(0, 16)` 全选非大气 | 拆成 `DEFAULT_CHART_CHANNELS = 6` + 保留 `MAX_CHART_CHANNELS = 16`（硬上限）；默认初始化用 6；「显示全部」仍受 16 上限 |
| `initializeDefaultChartSelection` | 已排除大气通道；仅在 selection 为空时写入 | 改 slice 上限为 6；新增 `resetChartSelectionToDefault(id)` 供「复位」 |
| `setAllChartSelection(true)` | 取全部 enabled 再 slice 到 MAX | 「显示全部」可继续走此路径（可选再过滤大气；推荐过滤大气以保护 Y 轴） |
| 顶栏停止 | 文案 `t.stopAcquisition`，variant 在 stop/start 间切换 | 采集中改为全局文案 + soft-danger/danger；弱标签「全局」可选 |
| 设备头 | 本设备 start/stop/disconnect/tare 已存在 | 加「本设备」弱标签；stop=danger；disconnect 走 `feedback.confirm`；tare 禁用 title |
| 通道名 | `ChannelCard` 等为 `CH${n+1}` / 可能混用 | 统一 `formatChannelLabel` |
| `RealtimeChart.readThemeColors` | 读 `--chart-grid` 等**错误名**，fallback hex | 改为 `--chart-grid-line` 等正式 token |
| 多通道 tooltip | ECharts 内 tooltip 易挡线 | 增加图外 readout；多系列时缩小 tooltip 信息量 |

## 3. 组件与依赖顺序

```
[T1] utils/channelFormat.ts          无依赖
        │
        ├─► [T2] ChannelCard / ChartSelector / 其它展示点用 formatChannelLabel
        │
[T3] deviceStore 默认 6 路 + reset API   可与 T1 并行
        │
        ├─► [T4] DeviceDetailPanel 工具：显示全部 / 复位
        │
[T5] MainTopBar 全局语义               可与 T1–T4 并行
[T6] DeviceDetailPanel 本设备语义      可与 T5 并行（同文件则串行合并）
        │
        └─► [T7] RealtimeChart token + readout bar
                 （依赖 T3 通道集合稳定更佳，但可并行）
```

**顺序原则**：先纯函数与 store 契约 → 再壳层按钮语义 → 再图表 chrome。  
**同文件合并**：`DeviceDetailPanel` 的 T4+T6 在实现时可合并为一个 task session，避免双开冲突。

## 4. 垂直切片（可演示）

### Slice A — 作用域清晰（P0）

- 改：`MainTopBar.vue`、`DeviceDetailPanel.vue`（动作区）、i18n 键（或中文 fallback）
- 断开支路：`feedbackStore.confirm`
- 验收：顶栏「停止全部采集」；设备头「停止采集」；颜色 danger；断开有确认；禁用有 tip

### Slice B — 默认可读曲线 + 命名（P1）

- 改：`deviceStore.ts`（DEFAULT=6、reset）、`channelFormat`、卡片/选择器标签
- DeviceDetail 增加「显示全部」「复位窗口」（调用 store）
- 验收：冷启动 ≤6 路；复位回 6；显示全部 ≤16 且优先非大气；标签 `CH01`

### Slice C — 图表 chrome + 读数（P2a）

- 改：`RealtimeChart.vue`（token、readout、tooltip 策略）
- 验收：无错误 CSS 变量名；读数在 plot 外；浅/深主题网格可读

## 5. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 用户已有内存中的 16 路选择 | 默认只在 **selection 为空** 时写入；不强制清空运行中选择 |
| 「显示全部」含大气压压扁 Y 轴 | `setAllChartSelection` 或专用 `selectAllNonAtmospheric` 排除大气；大气仍可在选择器手动勾选 |
| i18n 键缺失 | 使用 `t.xxx \|\| '中文默认'` 模式（与现 MainTopBar 一致） |
| RealtimeChart 文件大 | 只改 theme 读取 + 底部 readout 区域；不重写缓冲/性能路径 |
| 与 waveform-performance Spec 冲突 | 保持 animation off、不增每帧 DOM 重排；readout 用轻量文本更新 |

## 6. 并行 vs 串行

| 可并行 | 必须串行 |
|---|---|
| T1 format 与 T3 store | T4 依赖 T3 API |
| T5 顶栏 与 T3 | T7 不依赖 UI 文案，但建议 A/B 合入后再测 C |
| 单测与实现同切片内 | 全量 typecheck/build 在全部切片后 |

## 7. 验证检查点

| 检查点 | 时机 | 命令 / 动作 |
|---|---|---|
| CP1 | Slice A 后 | 手测顶栏/设备头；`npm run typecheck` |
| CP2 | Slice B 后 | 单测 store 默认 6；手测显示全部/复位；`npx vitest run` 相关 |
| CP3 | Slice C 后 | 手测 readout + 深浅主题；`npm run test`；`npm run build` |
| CP4 | 合并前 | `validate-frontend-structure.ps1 -CheckFileSize` |

## 8. 明确不在本 Plan

- 压力/环境**卡片分组**布局  
- 侧栏设备卡大改、底栏大改  
- 后端、校准/遍历页  
- 将 `channelColors.ts` 完全改写为只读 CSS 变量（保持现 API）

## 9. 文件触点预算

| 切片 | 预计文件（≤5/task 目标） |
|---|---|
| A | MainTopBar.vue, DeviceDetailPanel.vue, i18nStore（若有集中字典） |
| B | deviceStore.ts, channelFormat.ts(+test), ChannelCard.vue, ChartSelector.vue, DeviceDetailPanel.vue |
| C | RealtimeChart.vue(+test), 必要时 DeviceDetailPanel 包一层 readout 容器 |

## Change Log

| 日期 | 说明 |
|---|---|
| 2026-07-12 | 初稿：Spec 审阅决议后的实现计划 |
