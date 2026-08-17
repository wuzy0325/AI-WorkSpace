# Spec: WindLabX4 监控工作区 UI 落地

> 阶段：Phase 1 — Specify（**待人审阅，未进入编码**）  
> 视觉/交互真源：`docs/design/monitor-workspace-spec.md`  
> 参考原型：`docs/ui-redesign-prototype.html`  
> 总纲：`DESIGN.md` · 图表微观：`docs/design/chart-spec.md`  
> Token：`styles/tokens/*`（含已落地的 `--chart-*`）

---

## ASSUMPTIONS（请先纠偏）

以下假设若不成立，请直接指出；未反对则按此推进后续 Plan / Tasks。

1. **范围 = 前端 UI 落地**，不改 Go 后端采集/记录协议，不改 API 契约。
2. **行为语义基本保持现状**：顶栏开始/停止 = 全局（任一/全部设备的现有 store 语义）；设备头开始/停止/断开/校零 = **当前选中设备**。本次主要澄清文案、颜色、布局与作用域标签，而不是重写采集状态机。
3. **不做**「聚焦 / 全部 / 异常」三态切换（用户已否决）。
4. **默认可见曲线改为 ≤6 路**（非大气压/温度通道中的前 6 路），用户仍可通过通道选择器全选；「显示全部」作为图表工具按钮可选。
5. **通道显示名统一为 `CH01` 格式**（两位零填充；图例、卡片、tooltip、title 一致），废弃混用 `CH1` / `CH_01`。
6. **图外读数条（readout bar）纳入本 Spec 的 P1**，替换/弱化多通道大面积 tooltip 遮挡。
7. **颜色**：继续走 `utils/channelColors.ts` 的稳定映射（含 DAQ-P-1603 压力/温度分色），图表 chrome 改读 `--chart-*`；逐步减少 feature 内 fallback hex，但不强行一夜之间把 16 档压力色板全部改成 8 个 CSS 变量。
8. **浅色主题为默认验收面**，深色做回归；桌面固定布局，无响应式重构。
9. **不改**校准 / 遍历 / 运动配置页的 Density Spec 与业务布局。
10. **原型是风格与信息架构参考，不是像素级强制**；生产实现用现有 Vue 组件与 token，不引入新 UI 库。

→ 有异议请在审阅时标出编号。

---

## Objective（目标）

### 我们在解决什么问题

监控混合视图已经有可用功能，但存在：

- 全局 vs 本设备操作语义不清（顶栏与设备头都有「停止采集」，颜色不一致）
- 默认多通道同图导致曲线难读
- 通道命名不一致（`CH1` / `CH_01` / `CH01`）
- 图表 chrome 部分仍读错 token 名或硬编码，与已发布的 `monitor-workspace-spec` / `--chart-*` 未对齐
- 多通道读数易遮挡波形

设计规范与 token 已就绪；本 Spec 定义**如何把规范落到现有 Vue 代码**。

### 用户是谁

风洞/实验室操作员：长时间盯着实时曲线与通道现值，需要快速判断「停谁、记不记、哪路在显示」。

### 成功长什么样

操作员在混合视图下 **3 秒内**能回答：

1. 当前是全局操作还是本设备操作？  
2. 图上默认在看哪几路？怎么加路/独显？  
3. 光标处各通道数值是多少且不挡曲线？  
4. 通道名是否与卡片一致？

---

## Tech Stack（技术栈）

| 项 | 选型 |
|---|---|
| 壳 | Wails v3 + Vue 3.5 + TypeScript |
| 状态 | Pinia（`deviceStore` / `storageStore` / `i18nStore` / `themeStore`） |
| 图表 | ECharts 6 + vue-echarts（`RealtimeChart.vue`） |
| UI 原语 | `components/ui/*` + Naive UI 兜底 |
| 样式 | Design tokens + scoped CSS（禁止 feature hex 扩散） |
| 测试 | Vitest + @vue/test-utils |

---

## Commands（命令）

```powershell
cd projects\WindLabX4\apps\desktop-wails\frontend

npm run typecheck
npm run test
# 相关单测示例
npx vitest run src/utils/__tests__/channelColors.test.ts
npx vitest run src/components/device/__tests__/RealtimeChart.test.ts

npm run build

# 仓库根目录结构校验（提交前）
powershell -File ..\..\..\..\validate-frontend-structure.ps1 -CheckFileSize
```

提交前强制顺序：`typecheck` → `test` → `build` → `validate-frontend-structure.ps1`。

---

## Project Structure（改动落点）

```
projects/windlabx4/
├── DESIGN.md                                 # 已含 Monitor Workspace 摘要
├── docs/
│   ├── design/monitor-workspace-spec.md      # 交互/视觉真源（已有）
│   ├── ui-redesign-prototype.html            # 视觉参考（已有）
│   └── specs/
│       ├── spec-monitor-workspace-ui.md      # 本文件
│       ├── plan-monitor-workspace-ui.md      # Phase 2（Spec 通过后写）
│       └── tasks-monitor-workspace-ui.md     # Phase 3（Plan 通过后写）
└── apps/desktop-wails/frontend/src/
    ├── components/layout/
    │   ├── MainTopBar.vue                    # 全局作用域文案/按钮语义色
    │   └── MainBottomBar.vue                 # 状态栏补齐（采样/记录等，按需）
    ├── components/main/
    │   ├── DeviceDetailPanel.vue             # 本设备操作区、默认通道、分组
    │   ├── ChannelCard.vue                   # CH01 命名、样式对齐
    │   ├── ChartSelector.vue                 # 选择器命名/默认
    │   └── DeviceSidebar.vue                 # 设备卡元信息（轻量）
    ├── components/device/
    │   └── RealtimeChart.vue                 # --chart-*、readout、冻结/工具条
    ├── utils/
    │   └── channelColors.ts                  # 与 token 对齐策略（保持 API）
    ├── stores/
    │   └── deviceStore.ts                    # isChartSelected 默认策略（若需要）
    └── styles/tokens|themes/                 # --chart-* 已加；本 Spec 原则上不再扩 token
```

**不在范围**：`views/CalibrationView`、`TraversalView`、运动独立窗、后端 usecase。

---

## Code Style（代码风格）

- 中文注释；完整 TypeScript 类型。
- 视觉值只走 `var(--token)`；runtime 读 CSS 变量时使用**已存在**的 token 名（如 `--chart-grid-line`，不要 `--chart-grid`）。
- 通道显示名单一函数，禁止各组件各自 `padStart` 各写一套。

```ts
// 推荐：单一格式化入口（可放 utils/channelFormat.ts 或既有 util）
/** 通道显示名：CH01、CH02…（与 monitor-workspace-spec 一致） */
export function formatChannelLabel(index: number): string {
  // index 为 0-based 设备通道序号
  return `CH${String(index + 1).padStart(2, '0')}`
}
```

```vue
<!-- 设备头：本设备作用域 -->
<div class="detail-panel__actions" data-scope="device">
  <span class="ops-scope-label">{{ t.scopeDevice || '本设备' }}</span>
  <UiButton variant="secondary" size="sm" :disabled="..." :title="tareDisabledReason">
    {{ t.tare || '校零' }}
  </UiButton>
  <UiButton variant="danger" size="sm" @click="stopThisDevice">
    {{ t.stopAcquisition || '停止采集' }}
  </UiButton>
  <UiButton variant="ghost" size="sm" @click="confirmDisconnect">
    {{ t.disconnect || '断开' }}
  </UiButton>
</div>
```

```vue
<!-- 顶栏：全局作用域 -->
<div class="main-topbar__actions" data-scope="global">
  <span class="ops-scope-label">{{ t.scopeGlobal || '全局' }}</span>
  <UiButton ...>{{ acquiring ? (t.stopAllAcquisition || '停止全部采集') : ... }}</UiButton>
  <UiButton ...>{{ recording ? ... : (t.startRecording || '开始记录') }}</UiButton>
</div>
```

---

## Testing Strategy（测试策略）

| 层级 | 内容 |
|---|---|
| 单元 | `formatChannelLabel`；默认可见通道选取（≤6、排除大气通道）；`readThemeColors` 读到正确 `--chart-*` |
| 组件 | `ChannelCard` 展示 `CH01`；`MainTopBar` 采集中文案含「全部」或等价全局语义；`RealtimeChart` 使用 chart token（mock getComputedStyle） |
| 手工 | 混合视图：默认 6 路、图例/选择器联动、读数条不挡线、本设备停止 vs 顶栏停止、禁用校零有 tip、浅/深主题 |
| 回归 | 现有 `channelColors` / `RealtimeChart` / deviceStore 测试全绿 |

覆盖率：新增纯函数 ≥ 现有 utils 水平；不强制全项目 coverage 数字。

---

## Boundaries（边界）

### Always（必须）

- 改前阅读 `docs/design/monitor-workspace-spec.md` 对应章节。
- 视觉值使用 tokens；图表 chrome 使用 `--chart-*`。
- 保持 `buildChannelColorMap` 颜色稳定性契约（同 profile → 同色）。
- 每个垂直切片可手测混合视图主路径。
- 提交前跑 typecheck / test / build / 结构校验。

### Ask first（先问人）

- 改变顶栏 stop 的**后端语义**（例如从「停全部」改为「只停当前」）。
- 默认 6 路的选取规则若要改成「按板卡/按告警/按上次会话」等非「前 6 个非大气通道」。
- 大改 ECharts 为自研 canvas、或换图表库。
- 新增 npm 依赖。
- 修改 i18n 键的大规模英文文案策略。

### Never（禁止）

- 引入「聚焦 / 全部 / 异常」三态模式控件。
- 用成功绿表示「停止」。
- 在 feature 组件新增平行 `CHANNEL_COLORS = ['#...']` 数组。
- 多通道实时图用大面积遮挡 plot 的 tooltip 作为唯一读数方式。
- 把配置页 Density Spec 套到监控数据面板（或反过来）。
- 在未更新本 Spec 的情况下扩大范围到校准/遍历。

---

## Functional Requirements（功能需求）

### FR1 操作作用域

| ID | 需求 | 验收 |
|---|---|---|
| FR1.1 | 顶栏操作标记为全局；停止文案体现「全部」或等价全局语义 | 采集中顶栏按钮文案 ≠ 设备头「停止采集」的无修饰重复，或有「全局」标签 |
| FR1.2 | 设备头操作标记为本设备；停止/连接仅影响 `selectedDeviceId` | 与现 `deviceStore.startAcquisition(id)` 等一致 |
| FR1.3 | 停止类按钮使用 danger / soft-danger，不用 primary/success 绿 | 视觉检查 + class/variant |
| FR1.4 | 断开需确认（confirm 或项目内 UiConfirm） | 误触一次不会直接断开 |
| FR1.5 | 禁用校零/记录时提供原因（title/tooltip） | 悬停可见原因字符串 |

### FR2 曲线默认与通道选择

| ID | 需求 | 验收 |
|---|---|---|
| FR2.1 | 无用户选择时，默认图表通道 = 非大气通道中最多 6 个 | 新建设备/清空选择后可见 ≤6 |
| FR2.2 | 用户显式多选仍可 >6 | 选择器全选后图上全部选中通道 |
| FR2.3 | 不提供聚焦/全部/异常分段控件 | UI 无此控件 |
| FR2.4 | 提供「显示全部非大气通道」与「复位为默认 6 路」入口（图表工具区） | 两键可测 |

### FR3 读数与图表 chrome

| ID | 需求 | 验收 |
|---|---|---|
| FR3.1 | 混合/图表模式：光标多通道值在 plot 外 readout 区域 | 不遮挡主曲线区 |
| FR3.2 | RealtimeChart 网格/轴/十字线读 `--chart-grid-line` 等正式 token | 无 `--chart-grid` 错误名依赖 |
| FR3.3 | 实时数据更新无动画 | ECharts animation 关闭（保持现性能 spec） |

### FR4 命名与卡片

| ID | 需求 | 验收 |
|---|---|---|
| FR4.1 | 卡片/图例/选择器/ compact title 统一 `CHxx` | 无 `CH_01` / 无零填充缺失 |
| FR4.2 | 大气压/温度通道不进默认压力同图（保持现 isAtmospheric 逻辑） | 与现行为一致并有测试或注释 |
| FR4.3 | 卡片模式可按压力/环境分组展示（若现结构允许；否则 P2） | Spec 审阅确认是否本迭代必做 |

### FR5 Token 与规范对齐

| ID | 需求 | 验收 |
|---|---|---|
| FR5.1 | 不新增与 `monitor-workspace-spec` 冲突的硬编码布局魔法数（壳尺寸继续 `layout.css`） | code review |
| FR5.2 | 版本号不与主 CTA 抢视觉（可降权到 muted） | 顶栏检查 |

---

## Success Criteria（总验收）

- [ ] SC1：混合视图下，全局停止与本设备停止在**位置 + 文案 + 颜色**上可区分。  
- [ ] SC2：冷启动默认曲线通道数 ≤ 6（非大气）。  
- [ ] SC3：通道标签全链路 `CH01` 格式一致。  
- [ ] SC4：多通道读数不遮挡主 plot（readout bar 或等价方案）。  
- [ ] SC5：`RealtimeChart` 使用 `--chart-*` 正式 token。  
- [ ] SC6：无「聚焦/全部/异常」模式开关。  
- [ ] SC7：`npm run typecheck` / `npm run test` / `npm run build` 通过。  
- [ ] SC8：浅色 + 深色各手测一遍主路径。

---

## Out of Scope（明确不做）

- 后端采集调度、记录文件格式、SSE 协议变更  
- 校准/遍历/运动页视觉重构  
- 色盲色板第二套切换 UI  
- 键盘快捷键体系  
- 将 `:root` 从 dark baseline 整体翻转为 light（`light-theme-palette.md` 长期项）  
- 像素级复刻 HTML 原型的装饰阴影/字体

---

## Risks（风险）

| 风险 | 缓解 |
|---|---|
| 默认 6 路改变老用户「默认全看」习惯 | 「显示全部」一键；记住用户 `isChartSelected` 选择优先于默认 |
| 顶栏文案改成「停止全部」与 i18n 键不齐 | 先中文 fallback，键名写入 i18nStore |
| readout bar 与 ECharts tooltip 双读数 | 多通道时弱化 tooltip 为单点时间，或 tooltip 仅显示当前系列 |
| `channelColors` 与 CSS `--color-channel-*` 双源 | 本迭代保持 TS 色板权威；文档标明；避免再引入第三份 |

---

## Open Questions（已决议）

| # | 问题 | 决议 | 日期 |
|---|---|---|---|
| Q1 | 顶栏停止文案 | **「停止全部采集」**（与设备头「停止采集」区分） | 2026-07-12 |
| Q2 | 默认 6 路规则 | 按 `profile.channels` 顺序，跳过大气通道，取前 6 个 enabled | 2026-07-12（采用推荐默认） |
| Q3 | 压力/环境卡片分组 | **本迭代不做**；放入后续迭代 P2 | 2026-07-12 |
| Q4 | 断开确认 | 优先项目 `UiConfirmDialog` / 既有 feedback 确认；无现成则再议 | 2026-07-12（采用推荐默认） |
| Q5 | 记录禁用 tip | 本迭代做可得状态的 tip（如未采集）；无路径状态则不强行造数据 | 2026-07-12（采用推荐默认） |

Spec 状态：**已审阅通过 → 进入 Plan / Tasks**。

---

## Phased Delivery（本迭代范围）

| 阶段 | 内容 | 本迭代 |
|---|---|---|
| P0 | 作用域文案/颜色/断开确认/禁用 tip | ✅ |
| P1 | 默认 6 路 + 显示全部/复位 + CH01 统一 | ✅ |
| P2a | readout bar + chart token 修正 | ✅ |
| P2b | 压力/环境卡片分组、侧栏元信息增强 | ❌ 下迭代 |

正式实现文档：

- `docs/specs/plan-monitor-workspace-ui.md`
- `docs/specs/tasks-monitor-workspace-ui.md`

---

## Change Log

| 日期 | 说明 |
|---|---|
| 2026-07-12 | 初稿：自 UI 审查 + 原型 + monitor-workspace-spec 收敛为可落地 Spec |
| 2026-07-12 | 人审通过：停止全部采集；卡片分组下迭代；进入 Plan |
