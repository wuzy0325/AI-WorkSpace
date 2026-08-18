# Spec: 探针校准画面切换状态恢复

> 来源：交互设计审查 → spec-driven-development Phase 1 → 规格审查修订
> 日期：2026-07-15
> 状态：待批准（已按三文档联合 review 修复，v3）
> 修订：v3（v2→v3：返回 Home 不 stop、stop 定格/UI 映射、API 名、acquire/recovery 顺序、双击恢复、依赖图一致、同类型再进入、paused 角标）
> 关联：
> - [spec-traversal-reliability-and-recovery.md](./spec-traversal-reliability-and-recovery.md)（恢复体验语言对齐：loading / 失败可见 / 后端权威）
> - [spec-calibration-motion-safety.md](./spec-calibration-motion-safety.md)（失败终态与故障快照需在切回后正确展示）

## Objective

**目标**：修复探针校准过程中切换到其他画面再返回后，画面状态与实际任务状态不一致的问题，使一次校准任务在画面切换、跨探针类型切换、再次进入校准模块等场景下，前端 UI 始终能正确反映后端真实任务状态。

**用户**：
- 风洞探针校准操作人员——期望离开校准页查看日志 / 监控 / 设备后再回来时，任务进度不丢、按钮可用、完成或失败可一眼看出
- 校准流程开发人员——期望四探针 Main 行为一致，恢复路径集中在 store，而不是各自特殊逻辑

### User Outcome（操作员可感知结果）

校准进行中，操作员可随时离开校准页查看日志、设备状态或主监控，再返回时必须满足：

1. **任务还在不在**：正确显示运行中 / 已暂停 / 已完成 / 失败 / 已停止
2. **进度有没有丢**：已采点数与进度条连续，不会无故归零
3. **时间是否可信**：已用时连续无跳变；暂停期间时间定格
4. **操作是否匹配**：暂停 / 继续 / 停止 / 导出按钮与当前状态一致
5. **终态可处理**：期间完成可导出；期间失败可见原因；期间停止保留已采数据

一句话产品门禁：

> 离开再回来，任务不会“假死”或“假空闲”；完成后回来一定能导出；失败后回来一定看得到原因。

### 范围

覆盖代码审查确认的核心缺陷，以及审查增补的产品缺口：

1. 四类 Main 组件（五孔 / 三孔 / 总压 / **总温**）unmount 行为不一致——五孔调 `calibrationStore.reset()` 清空 store，其它类型不调，导致切走再回来状态错乱。
2. `statusPolling` 启动条件错位——仅在 `startCalibration` 内启动，组件 remount 后即使后端任务仍在运行，前端也不会自动恢复 polling。
3. 零 keep-alive + 硬切换——切走即 unmount，局部订阅 / 定时器销毁，回来是全新实例，无恢复路径。
4. 跨探针类型切换缺少保护——运行中切类型可能静默覆盖当前任务。
5. 恢复过程缺少用户可见反馈——切回瞬间可能空白 / 假 idle，缺少 loading 与失败提示。

### 不在范围内

- 不重做校准画面视觉设计。
- 不改变后端校准任务的状态机或 `status()` API 契约。
- 不引入 keep-alive、不新增 Pinia store、不新增第三方依赖。
- 不改变校准算法、CSV 格式、设备通信协议。
- 不触及后端 Go 代码，仅改造前端。
- 不支持多类型校准任务并行。
- 不做完整“应用崩溃后断点续跑校准”产品能力；仅保证再次进入校准时 UI 能同步后端当前存活 / 终态。

## Confirmed Decisions

1. **覆盖全部 4 种探针**  
   `five-hole` / `three-hole` / `total-pressure` / `total-temperature` 统一改造，不允许只修部分类型。

2. **后端 status 为唯一事实源**  
   进入任一校准 Main、或校准窗口重新挂载时，必须调用 `recoveryFromBackend()`。本地 store 只做切走期间心跳保活与 UI 缓存，不作为恢复权威。

3. **unmount 不清空任务会话**  
   Main 组件销毁只清理局部订阅 / 定时器，不调用 `reset()`。  
   清空会话数据只发生在：
   - 用户**开始新任务**（`startCalibration` 成功启动前清旧会话）
   - 应用进程重启后 store 自然为空
   - 明确的“清除结果 / 重新开始”入口（若当前无此入口，本次不新增）

4. **stop 保留已采数据**  
   用户主动 `stop()` 后：
   - 状态展示为 **已停止**
   - 已采 `dataPoints` 保留
   - 允许导出 / 保存已有数据
   - **禁止** stop 后立即 `reset()` 清空结果  
   “停止 ≠ 清空结果”。
   补充：stop 后 `elapsedTick` 立即定格（`stopElapsedTick()`），已用时冻结；保留短时 1Hz 心跳确认后端 `stopped` 落地，不无限高频空转。

5. **跨类型切换仅两选项**  
   任务为 running / paused 时切换探针类型，弹确认框：
   - **取消**：留在当前画面
   - **停止当前任务并切换**：先 `stop()`，再切换  
   不提供“后台保留任务并切换”，不做多任务并行。  
   文案必须写清：会停止当前校准，不是临时挂起。

6. **拦截入口：返回 Home 不拦截，仅跨类型选卡拦截**  
   - **Main → 返回 Home（仍在校准模块内）**：不弹框、不 `stop()`，任务继续在后台以 1Hz 心跳运行，Home 显示进行中标识（见 Decision #10 / U11）。
   - **Home → 点另一种探针卡片**：仅当 `running || paused` 且目标类型与 `store.status?.type` 不同时弹确认框（取消 / 停止并切换），见 Decision #5。
   - **切到看板 / 日志 / 监控再切回校准**：由 I10 落点决策直接进对应 Main，不弹框。
   - 明确禁止把「返回 Home」做成「停止任务」——这与 1Hz 心跳、后台可感知、U11 全部矛盾。

7. **polling 频率策略固定**  
   - 至少一个校准 Main 可见：`statusPolling = uiRefreshHz`（默认 5Hz），`elapsedTick = 1Hz`
   - 全部切走：`statusPolling = 1Hz` 心跳，`elapsedTick` 暂停
   - 切回 / 进入统一恢复协议（见 I4 / Recovery UX）：`acquireView()`（引用计数+1，重启 elapsedTick） → `stopStatusPolling()`（防并发写） → `recoveryFromBackend()`（拉一次 status） → 依后端状态设频（`running`/`paused` 升 5Hz，否则 1Hz 心跳）
   - 禁止“先 acquire 开 5Hz，再 recovery 里 stop，再开”的抖动

8. **恢复 UX 对齐遍历测试语言**  
   - 恢复中：显示轻量 loading（如“正在恢复校准状态...”）
   - 恢复失败：可见错误提示 + 可重试；若有本地缓存可只读展示，不伪装成“空闲可新开”
   - 恢复完成前：禁用“开始”，避免双开任务；暂停 / 继续 / 停止在后端状态落地后再按真实状态启用

9. **进入校准模块也要 recovery**  
   不只“从其它 Tab 切回 Main”，以下时机都要兜底：
   - 任一校准 Main `onMounted`
   - 应用启动后首次进入校准窗口 / Home 能感知到后台任务时  
   目标：若后端任务仍存活，UI 能接上；若后端已终态，UI 显示正确终态。

10. **后台任务可感知（轻量）**  
    任务在后台 running / paused 时，校准 Home 对应卡片或窗口级状态区至少显示一种进行中标识（角标 / 文案），避免用户离开后以为任务已消失。  
    不做复杂全局通知中心。

11. **终态语义明确，不允许含糊二选一**  

| 后端/事件 | UI 展示 | 数据 |
|---|---|---|
| running | 运行中 | 持续刷新点数 / 进度 |
| paused | 已暂停 | 时间定格，可继续 |
| completed | 已完成 | 完整数据，可导出 |
| error / 运动安全失败 | 失败 + 原因 / 故障快照 | 保留已采点，可复盘 / 视情况导出 |
| 用户 stop / 外部 stop | **已停止** | **保留已采点，可导出** |
| 无任务 idle | 空闲 | 无活跃任务数据 |

12. **扩展现有 store，不另起状态体系**  
    在 `calibrationStore` 内新增 `recoveryFromBackend` / `acquireView` / `releaseView` / `activeViewCount` / `isRecovering` 等字段与方法；不新增第二个 Pinia store。

13. **切回落点：任务态智能 + 空闲回 Home**  
    `CalibrationWindow` 重新挂载（用户从看板 / 日志 / 监控切回校准模块）时，落点由后端任务状态决定：

    | 后端 status | 落点 | 说明 |
    |---|---|---|
    | `running` / `paused` | 直接进 `status.type` 对应的 Main | 任务在跑，直接接上，不强迫用户重新点卡片 |
    | `completed` / `error` / `stopped` | 直接进 `status.type` 对应的 Main | 终态也要让用户看到结果 / 导出，不回 Home 丢失上下文 |
    | `idle`（无任务） | 进 `CalibrationHome` | 空闲态不记忆上次类型，保持简洁 |

    - `activeCalibrationType` **不持久化到 store**——空闲态每次回 Home 重新选
    - 落点决策依赖 `recoveryFromBackend()` 的返回值，因此 `CalibrationWindow.onMounted` 必须先 recovery 再决定 `currentView`
    - recovery 期间显示 loading，避免落点闪烁（先 Home 再跳 Main）

14. **恢复协议归属：Window 模块级 + Main 复用，避免双重 loading**  
    - `CalibrationWindow.onMounted` 负责「模块级 recovery + 落点决策」（一次 `recoveryFromBackend`）。
    - Main/composable 的 `onMounted` 负责 `acquireView`（引用计数）；若 store `lastRecoveryAt` 在最近 2s 内（Window 刚恢复），**跳过二次 recovery**，直接复用，避免 `isRecovering` 闪两次 / 重复 loading。
    - 若 `lastRecoveryAt` 缺失或过期（如直接进入某 Main、绕过 Window 路径），则 Main 自行 `recoveryFromBackend` 兜底。
    - 无论如何，UI loading 不重复打断用户。

15. **「已停止」UI 映射必须显式**  
    store `mapCalibrationState` 识别 `stopped` 之外，四个 Main 的 `statusText` / 状态色也要映射：
    - `status.status==='stopped'` 或 `completeEvent` 且非 success → 显示「已停止」，颜色与 idle 区分（非绿/非灰）。
    - 禁止 stop 后因无 `completeEvent` 退化为显示「空闲」，否则 U5/U12 验收失败。

## Tech Stack

- 前端：Vue 3 + TypeScript + Vite + Naive UI（已锁定版本，本次不改）
- 状态管理：Pinia（单例 `calibrationStore`）
- 桌面壳：Wails v3 alpha.95
- 后端通信：`wailsApi.calibration.status()` 轮询 + `deviceApi.onSnapshot` 引用计数订阅

## Commands

```powershell
# 前端类型检查
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck

# 前端构建
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run build

# 前端测试
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run test

# Wails 开发模式（端到端手测）
cd projects\WindLabX4\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

## Project Structure

本次改造触及的文件（仅前端）：

```
apps/desktop-wails/frontend/src/
  stores/
    calibrationStore.ts
      → 新增 recoveryFromBackend / acquireView / releaseView
      → 新增 activeViewCount / isRecovering / recoveryError
      → stop() 保留 dataPoints，不再隐含“停了就清空”
  composables/
    useCalibrationWorkflow.ts
      → onMounted：acquireView + recoveryFromBackend
      → onUnmounted/onBeforeUnmount：releaseView（清理局部资源）
  components/calibration/
    CalibrationWindow.vue
      → onMounted：模块级 recoveryFromBackend 再决定 currentView（I10 落点决策，Decision #14）
      → 跨类型选卡拦截（handleSelectCalibration）：running/paused 且类型不同弹确认框；返回 Home（handleBack）不拦截不 stop（Decision #6）
      → 确认框文案：停止当前任务并切换
      → recovery 期间 loading，避免落点闪烁
    CalibrationHome.vue
      → 后台任务进行中标识（角标 / 文案）
    five-hole/FiveHoleMain.vue
      → 删除 onBeforeUnmount 中的 reset()
      → 统一 releaseView + 局部资源清理
    three-hole/ThreeHoleMain.vue
      → 统一 acquire/release + recovery
    total-pressure/TotalPressureMain.vue
      → 同上
    total-temperature/TotalTemperatureMain.vue
      → 同上（必须纳入）
```

不新增目录。如恢复 loading / 失败提示可复用现有 `UiLoadingState`、toast、告警卡片，不强制新文件；若抽取小组件，仅限 calibration 目录内共享，不引入新依赖。

## Code Style

遵循项目既有约定：

- TypeScript strict 模式，所有新增 API 必须有显式类型注解
- 中文注释解释「为什么」而非「做了什么」
- Pinia store 用 setup 风格（`defineStore('name', () => {...})`），不写 options 风格
- 定时器句柄用 store 内 `let`，不进 `ref`（避免响应式开销）
- 不用 `any`、不用 `@ts-ignore`、不用 `eslint-disable`

示例片段（store 新增方法的预期风格）：

```ts
/**
 * 从后端拉取一次完整 status 兜底，用于画面切回 / 再次进入时恢复状态。
 *
 * 为什么：组件 remount 后本地 store 可能为空或过期，不能信任本地时效性；
 *   必须以后端为准。若后端任务仍在 running/paused，自动恢复 polling + tick；
 *   若已终态，写入 completeEvent / 失败信息并停止空转轮询。
 *
 * @returns 映射后的前端任务状态；后端无任务时返回 null
 */
async function recoveryFromBackend(): Promise<CalibrationTaskStatus | null> {
  // ...
}
```

## Data / State Semantics

### 会话数据（跨画面保留）

- `status`
- `dataPoints`
- `completeEvent`
- `timeInfo`（及 pause 累计相关内部字段）
- `isRunning` / `isPaused`
- 最近一次 `lastErrorCode` / `motionSafetyFailure`（若 status 中有）

### 视图局部资源（随 Main unmount 销毁）

- `deviceApi.onSnapshot` 订阅
- `deviceStore.attachStatusListener`
- `motionStore.attachStatusListener`
- `motionStatusPollTimer`
- 五孔 `chartTimer` / canvas 局部状态
- 其它仅当前页面使用的 UI 订阅

### 清空规则

| 动作 | 是否清空会话数据 | 说明 |
|---|---|---|
| 切到其它 Tab / 日志 / 监控 | 否 | 只降频心跳 |
| 返回校准 Main | 否 | recovery 覆盖过期字段 |
| 暂停 / 继续 | 否 | |
| 停止 | 否 | 保留已采点，状态=已停止 |
| 任务完成 / 失败 | 否 | 保留结果供导出 / 复盘 |
| 开始新任务 | 是 | 新会话覆盖旧会话 |
| 应用重启 | 是（内存态） | 进入后靠 recovery 接后端 |

## Recovery UX

### 正常恢复（统一协议）

1. `isRecovering = true`
2. `acquireView()`：引用计数 +1，按当前 `isRunning`/`isPaused` 重启 `elapsedTick`（不动 status 轮询频率）
3. `stopStatusPolling()`：暂停 status 轮询，避免与下一步并发写 `status.value`
4. `await recoveryFromBackend()`：调后端 `status()` 一次，写 store（含 dataPoints / 错误快照）
5. 依后端状态设频：`running`/`paused` → `startStatusPolling(uiRefreshHz)`；否则 → `startStatusPolling(1Hz)`
6. 按后端状态渲染按钮与状态栏
7. `isRecovering = false`

超过约 300ms 仍未返回时，展示 loading 文案：“正在恢复校准状态...”。

### 恢复失败

- Toast 或页内错误条：恢复校准状态失败，请重试
- 提供「重试」动作，再次调用 `recoveryFromBackend`
- 有本地缓存时：可只读展示缓存，并明确标注“可能不是最新”
- 无可靠状态时：不显示“可立即开始”的假空闲；至少阻断盲目 start 或二次确认

### 与遍历体验对齐

与遍历测试一致：

- 有恢复中态
- 有失败可见
- 以后端权威状态为准
- 不静默丢进度

## Testing Strategy

### 测试框架

- 单元测试：Vitest（`npm run test`）
- 手测：Wails dev 模式 + 真实硬件 / 模拟器

### 单元测试（store 新增方法）

- `recoveryFromBackend()`：mock status 返回 running / paused / completed / error / stopped / idle，断言 store 同步
- `acquireView()` / `releaseView()`：断言 `activeViewCount` 与 polling 频率切换
- `stop()`：断言会话数据保留，状态为已停止，非 reset
- `startCalibration()`：断言会清旧会话后再进入 running
- 恢复失败路径：status 抛错时 `recoveryError` / 可重试

### 手测用例

| ID | 前置 | 步骤 | 期待 |
|---|---|---|---|
| U1 切走切回 running | 任选一探针 running，已采 ≥5 点 | 切到 log/dashboard ≥30s 再切回 | 运行中；点数 ≥5 且不回 0；进度连续；已用时连续无跳变；按钮=可暂停/停止 |
| U2 切走切回 paused | 校准已暂停 | 切走 10s 再切回 | 已暂停；时间定格；继续可用；点数保留 |
| U3 切走期间完成 | running 且即将完成 | 切走，等完成后切回 | 已完成；数据完整；导出 / 保存可用 |
| U4 跨类型切换拦截 | 五孔 running | 返回 Home → 点三孔卡片 | 弹确认框；取消留在五孔；确认则停止五孔并切换三孔 |
| U5 切走期间被停止 | running | 切走后从可触达入口 stop，再切回 | **已停止**（不是模糊 idle）；已采点仍在；可导出 |
| U6 polling 不空转 | 任意探针 running | 切到 dashboard 观察 Network | `calibration.status` ≤1Hz |
| U7 四探针一致 | 四类探针各自 running | 每类重复 U1 | 行为一致，无“只有五孔被 reset”差异 |
| U8 切走期间失败 | running，触发失败 / 运动安全失败 | 切走后失败，再切回 | 失败态 + 原因 / 故障快照；非 running；已采点保留 |
| U9 恢复 loading / 失败 | 可模拟 status 延迟或失败 | 切回校准页 | 显示恢复中；失败有提示与重试；不出现假空闲可开新任务 |
| U10 进入模块 recovery | 后端任务仍 running，前端 store 空 | 重新进入校准窗口 / Main | 自动接上运行态，无需用户手动重开 |
| U11 后台可感知 | running | 返回校准 Home | 对应卡片 / 状态区显示进行中标识 |
| U12 stop 保留数据 | 已采多点后 stop | 停止后不离开页面 | 已停止；表格数据仍在；可保存 / 导出 |
| U13 开始新任务清空 | 上一任务已停止且有数据 | 点开始新校准 | 旧会话清空，进入新 running |
| U14 切回落点决策 | (a) 五孔 running；(b) 五孔 completed；(c) 无任务 idle | 各自切到看板再切回校准模块 | (a) 直接进五孔 Main 显示运行中；(b) 直接进五孔 Main 显示完成可导出；(c) 进 Home 卡片选择页；均无落点闪烁 |
| U15 同类型再进入 | 五孔 running | 返回 Home → 再点五孔卡片 | 不弹框、不 stop，直接进五孔 Main 接上运行态（与 U4 跨类型区分） |
| U16 配置未加载即 running | 后端 running，本地配置未 load | 切回 Main | recovery（status）与 `loadSavedConfig` 并行；running 时强制再 load 对应 `status.type` 配置，通道面板/按钮可用，不空白 |
| U17 paused 角标文案 | 校准已暂停 | 返回 Home | 对应卡片显示「已暂停」角标（与「运行中」区分），不误导为仍在运动 |

### 覆盖率期望

- `calibrationStore.ts` 新增 / 变更方法行覆盖率 ≥90%
- 不强制整体覆盖率提升

## Boundaries

### Always

- 改 store / 组件前先读现有代码，理解既有引用计数（`deviceStore.snapshotAttachCount`、`motionStore.attachStatusListener`）
- 改 `calibrationStore` 对外方法前评估调用面（建议 `gitnexus_impact` 或等价搜索）
- 提交前 `npm run typecheck` + `npm run build` + `npm run test` 全绿
- 注释用中文，解释「为什么」
- 四探针同步修改，禁止只改五孔

### Ask First

- 是否要把 Home 进行中标识升级为全局状态栏 / 桌面通知
- 是否新增独立“清除结果”按钮（当前无则不做）
- 是否调整 1Hz 心跳以外的功耗策略

### Never

- 不引入 keep-alive（P2 备选，本 spec 明确排除）
- 不在后端新增 API（现有 `status()` 足够）
- 不把 `reset()` 绑到 unmount 或 stop
- 不修改 calibration CSV 格式或校准算法
- 不为本次改造新增第三方依赖
- 不实现多类型任务并行 / “后台保留并切换”

## Core Invariants

### I1. Store 跨画面持久化

`calibrationStore` 是校准任务的「前端运行时镜像」。组件 unmount 不清空 store。  
清空仅允许在「开始新任务」或进程级重启等 Confirmed Decisions 定义场景。

### I2. 后端为事实源

前端切回 / 再次进入校准画面，必须先 `recoveryFromBackend()`。  
**不信任本地 store 的时效性。** 本地 store 仅用于切走期间心跳保活与 UI 渲染缓存。

### I3. 局部资源随组件销毁

组件 unmount 必须清理视图局部订阅与定时器。  
**不清理** store 内 status / dataPoints / completeEvent / 会话级 timeInfo。

### I4. 画面活跃性驱动 polling 频率

- `activeViewCount > 0`：高频 statusPolling + elapsedTick
- `activeViewCount == 0`：1Hz 心跳，elapsedTick 暂停
- 切回 / 进入统一恢复协议：`acquireView()` → `stopStatusPolling()` → `recoveryFromBackend()` → 依后端状态设频（见 Recovery UX），禁止并发写竞态

### I5. 跨探针类型切换保护

running / paused 下切换探针类型必须确认。  
取消则留下；确认则先 stop 再切换。  
**不允许静默覆盖运行中任务。**

### I6. 切走期间终态同步

切走期间完成后台任务完成 / 失败 / 停止时：

1. 1Hz 心跳捕获并写入 store
2. 切回时 recovery 二次确认
3. UI 显示对应终态

**禁止** 后台已结束，切回仍显示 running。

### I7. 停止保留结果

`stop()` 只结束任务，不销毁已采数据。  
导出 / 复盘能力在已停止态仍可用。

### I8. 恢复过程对用户可见

恢复中与恢复失败不可静默。  
恢复完成前不得诱导用户重复 start。

### I9. 四探针行为同构

五孔 / 三孔 / 总压 / 总温在离开-返回、暂停、停止、完成、失败、跨类型拦截上行为一致。

### I10. 切回落点由后端任务态决定

`CalibrationWindow` 重新挂载时，落点以后端 `recoveryFromBackend()` 返回的 `status` 为准：

- 有任务（running / paused / completed / error / stopped）→ 进 `status.type` 对应 Main
- 无任务（idle）→ 进 Home

**禁止**：先渲染 Home 再跳 Main（落点闪烁）；**禁止**：空闲态记忆上次类型自动跳进 Main。

## Success Criteria

### 产品门禁（对人）

| 编号 | 条件 | 验证 |
|---|---|---|
| PO1 | 离开再回来，任务不会假死或假空闲 | U1 / U2 / U10 |
| PO2 | 进度与点数不会无故归零 | U1 / U7 |
| PO3 | 完成后回来一定能看到完成并可导出 | U3 |
| PO4 | 失败后回来一定能看到原因，不会伪装运行中 | U8 |
| PO5 | 换探针类型前一定二次确认，不会默默杀掉任务 | U4 |
| PO6 | 停止后仍能查看 / 导出已采数据 | U12 |
| PO7 | 任务态切回直达对应 Main，空闲回 Home，无落点闪烁 | U14 |

### 工程门禁（对实现）

| 编号 | 条件 | 验证方式 |
|---|---|---|
| SC1 | running 切走切回：运行中 + 进度 / 点数 / 已用时连续 | U1 |
| SC2 | paused 切走切回：暂停 + 时间定格 + 可继续 | U2 |
| SC3 | 切走期间完成：完成态 + 数据完整 + 可导出 | U3 |
| SC4 | 切走期间被停止：已停止 + 数据保留 | U5 |
| SC5 | 跨类型切换确认框生效 | U4 |
| SC6 | 切走期间 `calibration.status` ≤1Hz | U6 |
| SC7 | 四探针行为一致 | U7 |
| SC8 | 恢复 loading / 失败提示可用 | U9 |
| SC9 | 进入模块即可接上后端存活任务 | U10 |
| SC10 | Home 可见后台任务进行中标识 | U11 |
| SC11 | `npm run typecheck` + `build` + `test` 全绿 | CI |
| SC12 | store 新增 / 变更方法单测覆盖率 ≥90% | Vitest |
| SC13 | 任务态切回直接进对应 Main；空闲切回进 Home；无落点闪烁 | U14 |

## Open Questions

无阻塞问题。以下为已收敛结论，保留供追溯：

| # | 原问题 | 结论 |
|---|---|---|
| 1 | 心跳频率 | **1Hz** |
| 2 | 跨类型是否支持后台保留 | **否**，仅取消 / 停止并切换 |
| 3 | 冷启动 / 再进入是否 recovery | **是**，进入校准 Main / 模块即 recovery |
| 4 | stop 后是否 reset | **否**，stop 保留结果；开始新任务再清 |
| 5 | 总温是否纳入 | **是**，四探针全部覆盖 |

非阻塞可选项（不挡开发）：

1. Home 进行中标识是否升级为全局状态栏
2. 是否补一个显式“清除结果”按钮

## Out of Scope (Explicit)

- keep-alive 引入
- 后端 API 新增或修改
- 校准算法 / CSV 格式 / 设备协议改动
- 前端视觉重设计
- 多类型校准任务并行
- 完整崩溃断点续跑产品（checkpoint 体系）；本次只做 UI 状态与后端存活 / 终态同步
