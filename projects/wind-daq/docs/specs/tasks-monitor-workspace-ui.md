# Tasks: Wind-DAQ 监控工作区 UI 落地

> 阶段：Phase 3 — Tasks（**待人审阅后 IMPLEMENT**）  
> Spec：`spec-monitor-workspace-ui.md`  
> Plan：`plan-monitor-workspace-ui.md`

约定：一任务一次聚焦；每任务结束跑该任务 Verify；全部完成后跑全量 typecheck/test/build。

---

## Slice A — 操作作用域（P0）

- [ ] **A1. 顶栏全局停止文案与样式**
  - Acceptance:
    - 任一设备采集中时，顶栏主按钮文案为 **「停止全部采集」**（或 i18n 等价键 + 中文 fallback）
    - 未采集时保持「开始采集」类主推进语义（primary）
    - 采集中停止按钮使用 danger / soft-danger，**非** success 绿
    - 可选：操作区有弱文案「全局」
  - Verify: 手测 0/1/多设备采集；`npm run typecheck`
  - Files: `components/layout/MainTopBar.vue`；必要时 `stores/i18nStore.ts` / 文案表

- [ ] **A2. 设备头本设备操作语义**
  - Acceptance:
    - 设备头停止文案保持「停止采集」，与顶栏可区分
    - 停止按钮 danger；连接/断开 ghost 或 secondary 体系与规范一致
    - 断开前 `await feedbackStore.confirm(...)`，取消则不调用 disconnect
    - 校零禁用时有 `title`/tooltip 原因（如采集中不可校零）
    - 记录相关禁用（若本区有）给出可得原因 tip
    - 可选：弱文案「本设备」
  - Verify: 手测断开取消/确认；校零禁用悬停；typecheck
  - Files: `components/main/DeviceDetailPanel.vue`；`stores/feedbackStore.ts`（只读调用）

---

## Slice B — 默认 6 路 + 命名（P1）

- [ ] **B1. 通道显示名工具函数**
  - Acceptance:
    - `formatChannelLabel(index0Based)` → `CH01`…；≥99 时自然增位不截断
    - 单测覆盖 0→CH01、8→CH09、15→CH16
  - Verify: `npx vitest run src/utils/__tests__/channelFormat.test.ts`（路径以实现为准）
  - Files: `utils/channelFormat.ts`（新建）、对应 test

- [ ] **B2. 展示点统一使用 formatChannelLabel**
  - Acceptance:
    - ChannelCard、ChartSelector（及 DeviceDetail 内 compact 通道标签若有）不再手写 `CH${n}` / `CH_`
    - 无 `CH_01` 形式残留于监控主路径
  - Verify: grep 监控组件无 `CH_\${`；相关组件测试或手测；typecheck
  - Files: `ChannelCard.vue`、`ChartSelector.vue`、必要时 `DeviceDetailPanel.vue`

- [ ] **B3. deviceStore 默认 6 路与复位 API**
  - Acceptance:
    - 新增 `DEFAULT_CHART_CHANNELS = 6`；`MAX_CHART_CHANNELS` 仍为 16（硬上限）
    - `initializeDefaultChartSelection`：非大气 + enabled，**slice(0, 6)**
    - `setAllChartSelection(true)`：建议仅非大气 enabled，上限 16（与 Plan 一致；若改行为补测）
    - 新增 `resetChartSelectionToDefault(id)`：强制写回默认 6（忽略「已有 selection 则跳过」）
    - 用户已有非空 selection 不被初始化逻辑清空
  - Verify: deviceStore 单测或新增测例；手测新选设备默认 ≤6
  - Files: `stores/deviceStore.ts`、既有/新建 store 测试

- [ ] **B4. 图表工具：显示全部 / 复位**
  - Acceptance:
    - 混合/图表工具区有「显示全部」「复位窗口」（文案可 i18n）
    - 显示全部 → 非大气全选（≤16）
    - 复位 → `resetChartSelectionToDefault`
    - **无**「聚焦/全部/异常」三态分段控件
  - Verify: 手测两按钮；typecheck
  - Files: `DeviceDetailPanel.vue`（或 RealtimeChart 工具条，二选一，优先与通道选择同区）

---

## Slice C — 图表 token + 读数条（P2a）

- [ ] **C1. RealtimeChart 读取正式 --chart-\* token**
  - Acceptance:
    - `readThemeColors` 使用 `--chart-grid-line`、`--chart-grid-line-faint`、`--chart-axis-text`、`--chart-axis-line`、`--chart-crosshair`、`--chart-bg` 等
    - 删除对 `--chart-grid` / `--chart-axis` 错误名的依赖（或仅作废弃 fallback 且注释标明）
    - fallback hex 仅作 token 缺失兜底
  - Verify: 单测 mock getComputedStyle；浅/深主题手测网格对比度
  - Files: `components/device/RealtimeChart.vue`、`RealtimeChart.test.ts`

- [ ] **C2. 图外多通道读数条**
  - Acceptance:
    - 光标/最新点数值在 plot **下方** readout 区域展示（背景可用 `--chart-readout-bg`）
    - 最多约 8 路，不遮挡主曲线绘制区
    - 多通道时 ECharts tooltip 不再作为唯一读数方式（可弱化内容或仅时间）
  - Verify: 手测 hover/实时；性能主观无额外卡顿；typecheck
  - Files: `RealtimeChart.vue`；必要时父级样式

---

## 收尾

- [ ] **Z1. 全量验证与文档勾选**
  - Acceptance: Spec Success Criteria SC1–SC8 可勾选；本 tasks 文件 A/B/C 全勾
  - Verify:
    ```powershell
    cd projects\wind-daq\apps\desktop-wails\frontend
    npm run typecheck
    npm run test
    npm run build
    ```
    仓库约定结构脚本按需执行
  - Files: 本 tasks 勾选状态；必要时 Spec Change Log 记「已实现」

---

## 实现门禁

在你回复 **「Plan/Tasks 通过，开始实现」**（或等价确认）之前：

- **不**修改业务 Vue/TS 实现（Spec/Plan/Tasks 文档本身除外）
- 实现时按 A1→…→Z1 顺序，或按 Plan 允许的并行组

## Change Log

| 日期 | 说明 |
|---|---|
| 2026-07-12 | 初稿：与 plan-monitor-workspace-ui 对齐的可执行任务列表 |
