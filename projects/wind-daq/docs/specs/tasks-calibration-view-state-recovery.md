# Tasks: 探针校准画面切换状态恢复

> 关联：[spec](./spec-calibration-view-state-recovery.md) | [plan](./plan-calibration-view-state-recovery.md)
> 状态：待批准（已按三文档联合 review 修复）
> 日期：2026-07-15

## Phase 1: Store 基础设施

### Task 1: store 新增 activeViewCount / acquireView / releaseView + polling 降频

**Description:** 在 `calibrationStore` 内新增画面活跃性计数与 polling 频率切换机制。`acquireView()` 计数+1 并按需把 polling 升到 `uiRefreshHz`、重启 elapsedTick；`releaseView()` 计数-1，归零时 polling 降到 1Hz 心跳、暂停 elapsedTick。

**Acceptance criteria:**
- [ ] 新增 `activeViewCount: Ref<number>`（初始 0）
- [ ] 新增 `acquireView(): void`——计数+1；若从 0→1，按 `uiRefreshHz` 重启 `statusPolling` + 重启 `elapsedTick`（仅当 `isRunning || isPaused`）
- [ ] 新增 `releaseView(): void`——计数-1（下限 0）；若 1→0，polling 重启为 1Hz 心跳、`stopElapsedTick()`
- [ ] polling 间隔由当前 `activeViewCount > 0 ? uiRefreshIntervalMs : 1000` 决定
- [ ] `acquireView` / `releaseView` 不直接清理 `status` / `dataPoints` / `completeEvent`
- [ ] 现有 `startStatusPolling` 重构为接受 interval 参数，或内部读取 `activeViewCount`

**Verification:**
- [ ] `npm run typecheck` 通过
- [ ] 手动调用 `acquireView` → `releaseView` → `acquireView`，确认 polling 间隔切换
- [ ] Task 9 单测：`acquireView` / `releaseView` 计数与频率切换断言通过

**Dependencies:** None

**Files likely touched:**
- `apps/desktop-wails/frontend/src/stores/calibrationStore.ts`

**Estimated scope:** S（1 文件，新增 ~50 行）

---

### Task 2: store 新增 recoveryFromBackend() + isRecovering / recoveryError

**Description:** 新增从后端拉取一次完整 status 的 recovery 方法，用于画面切回 / 进入模块时兜底。recovery 期间置 `isRecovering=true`，失败置 `recoveryError`。若后端任务 running/paused，自动 `acquireView` 已由调用方完成，recovery 仅同步状态字段。

**Acceptance criteria:**
- [ ] 新增 `isRecovering: Ref<boolean>`（初始 false）
- [ ] 新增 `recoveryError: Ref<string | null>`（初始 null）
- [ ] 新增 `async recoveryFromBackend(): Promise<CalibrationTaskStatus | null>`
- [ ] recovery 流程：`isRecovering=true` → `recoveryError=null` → 调 `wailsApi.calibration.status()` → 复用 `updateStatusFromBackend` 同步 → `isRecovering=false` → 返回 status
- [ ] recovery 失败：`recoveryError=err.message`、`isRecovering=false`、保留旧 store 状态（不 reset）
- [ ] recovery 前先 `stopStatusPolling()` 避免并发写竞态；recovery 完成后由调用方依后端状态设频（`running`/`paused` 升 5Hz，否则 1Hz）
- [ ] http 模式（非 wails）调 `calibrationApi.status()` 兜底（注意：API 名为 `status()`，不是 `getStatus()`）

**Verification:**
- [ ] `npm run typecheck` 通过
- [ ] Task 9 单测：mock status 返回 running / paused / completed / error / stopped / idle，断言 store 同步
- [ ] Task 9 单测：mock status 抛错，断言 `recoveryError` 非空、旧状态保留

**Dependencies:** Task 1

**Files likely touched:**
- `apps/desktop-wails/frontend/src/stores/calibrationStore.ts`

**Estimated scope:** S（1 文件，新增 ~40 行）

---

### Task 3: store stop() 改终态保留，startCalibration 清旧会话

**Description:** `stop()` 不再清空 dataPoints、不再置 idle；改为 `status='stopped'`、保留 dataPoints、`stopElapsedTick()` 定格已用时、保留短时 1Hz 心跳确认后端 stopped 落地（不无限高频空转）。`mapCalibrationState` 新增 `stopped` 映射。`startCalibration` 在后端 start **成功**后显式清旧会话（调内部 `resetSession`，不暴露）；start **失败**不清旧会话，避免抹掉上一趟结果。四 Main 的 `statusText`/状态色同步映射「已停止」（UI 文案/色变更见 Task 5）。

**Acceptance criteria:**
- [ ] `mapCalibrationState` 新增 `case 'stopped': return 'stopped'`
- [ ] `stop()` 改为：`isRunning=false`、`isPaused=false`、结算 `pausedAccumulatedMs`、`status.status='stopped'`、**不**清 `dataPoints`、`stopElapsedTick()` 定格已用时（不调 `stopStatusPolling`，保留 1Hz 心跳确认后端落地）
- [ ] `startCalibration` 在 `wailsApi.calibration.start` **成功**后、设置新 status 前，调内部 `resetSession()` 清旧会话（status/dataPoints/completeEvent/realtimePressures/timeInfo/pausedAccumulatedMs）
- [ ] `startCalibration` 后端 start **失败**时，**不**调 `resetSession()`，保留上一趟 `stopped` 结果，避免“start 失败把上趟数据抹了”
- [ ] `reset()` 保留不变（仍用于进程级清理，但 unmount 不再调）
- [ ] grep 全部 `status.value?.status === 'idle'` / `!status.value` 调用点，确认对 `stopped` / `completed` / `error` 的处理正确（idle 判断改为「无任务」，即 `!status || status.status==='idle'`）

**Verification:**
- [ ] `npm run typecheck` + `npm run build` 通过
- [ ] Task 9 单测：`stop()` 后 `status.status==='stopped'`、`dataPoints.length>0`
- [ ] Task 9 单测：`startCalibration` 后旧 dataPoints 清空
- [ ] 手测 U12：stop 后表格数据仍在、可导出

**Dependencies:** Task 2

**Files likely touched:**
- `apps/desktop-wails/frontend/src/stores/calibrationStore.ts`
- 可能触及 UI 组件中 `!status` / `=== 'idle'` 判断点（grep 后确认）

**Estimated scope:** M（1-3 文件，改 store + 可能改 UI 判断点）

---

## Phase 2: 组件接入

### Task 4: useCalibrationWorkflow 接入 acquire/release + onMounted 调 recovery

**Description:** 在 composable 的 `onMounted` 按统一恢复协议：`acquireView()`（引用计数+1）→ `stopStatusPolling()` → `await recoveryFromBackend()` → 依后端状态设频；`onBeforeUnmount` 加 `releaseView()`。若 store `lastRecoveryAt` 在最近 2s 内（Window 已模块级恢复），跳过二次 recovery 直接复用，避免重复 loading。让四个 Main 共享统一的画面活跃性管理与恢复入口。

**Acceptance criteria:**
- [ ] `onMounted` 流程：`acquireView()`（计数+1）→ `stopStatusPolling()`（防并发）→ 与现有 `Promise.allSettled`（设备/运动/配置）并行执行 `recoveryFromBackend()`（仅当 `lastRecoveryAt` 缺失或 >2s，否则跳过复用 Window 的恢复结果）→ 依后端状态设频（`startStatusPolling(uiRefreshHz)` 或 1Hz）→ `isLoading=false`
- [ ] `onBeforeUnmount`（或 `onUnmounted`）新增 `releaseView()`
- [ ] recovery 失败时通过 `feedbackStore.pushToast` 提示，不阻塞页面渲染（用 `recoveryError` 控制 UI 错误条）
- [ ] composable 暴露 `isRecovering` / `recoveryError` 给 Main 组件渲染 loading / 错误条
- [ ] 不破坏现有 `loadSavedConfig` / `startCalibration` / `pause` / `resume` / `stop` 的返回值
- [ ] running 态下 `recoveryFromBackend` 与 `loadSavedConfig` 并行后，强制再 load 对应 `status.type` 配置，避免通道面板/按钮因 `hasConfig=false` 空白（U16）

**Verification:**
- [ ] `npm run typecheck` + `npm run build` 通过
- [ ] 手测 U10：后端 running、前端 store 空，进入 Main 自动接上运行态
- [ ] 手测 U9：模拟 status 延迟/失败，显示恢复中/错误提示

**Dependencies:** Task 1, Task 2

**Files likely touched:**
- `apps/desktop-wails/frontend/src/composables/useCalibrationWorkflow.ts`

**Estimated scope:** S（1 文件，改 ~30 行）

---

### Task 5: 四个 Main 统一 unmount 行为

**Description:** 五孔删除 `onBeforeUnmount` 的 `calibrationStore.reset()`；四个 Main 的 unmount 统一为「只 `cleanupSubscriptions()`」，`releaseView()` 由 composable 的 `onBeforeUnmount` 处理（Task 4 已加）。确保四组件行为同构。

**Acceptance criteria:**
- [ ] `FiveHoleMain.vue` `onBeforeUnmount` 删除 `calibrationStore.reset()` 调用
- [ ] `FiveHoleMain.vue` 保留 `chartTimer` 清理 + `cleanupSubscriptions()`
- [ ] `ThreeHoleMain.vue` / `TotalPressureMain.vue` / `TotalTemperatureMain.vue` 的 `onUnmounted` 保持 `cleanupSubscriptions()`，`releaseView` 由 composable 统一处理
- [ ] 四组件 unmount 后 `calibrationStore.status` / `dataPoints` / `completeEvent` 仍保留
- [ ] 四组件均通过 `useCalibrationWorkflow` 共享 acquire/release（不各自实现）
- [ ] 四 Main 的 `statusText` / 状态色：新增 `status.status==='stopped'` 或 `completeEvent` 且非 success → 显示「已停止」且与 idle 区分（禁止 stop 后退化显示「空闲」，否则 U5/U12 验收失败；与 Task 3 store `mapCalibrationState` 映射一致）

**Verification:**
- [ ] `npm run typecheck` + `npm run build` 通过
- [ ] 手测 U1：五孔 running 切走切回，status/dataPoints 连续
- [ ] 手测 U7：三孔/总压/总温同样行为
- [ ] grep 确认四个 Main 中无 `calibrationStore.reset()` 调用（除非显式「开始新任务」入口，本次不新增）

**Dependencies:** Task 4

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/calibration/five-hole/FiveHoleMain.vue`
- `apps/desktop-wails/frontend/src/components/calibration/three-hole/ThreeHoleMain.vue`
- `apps/desktop-wails/frontend/src/components/calibration/total-pressure/TotalPressureMain.vue`
- `apps/desktop-wails/frontend/src/components/calibration/total-temperature/TotalTemperatureMain.vue`

**Estimated scope:** M（4 文件，每文件改 1-3 行）

---

## Phase 3: 落点与拦截

### Task 6: CalibrationWindow onMounted 落点决策 + recovery loading

**Description:** `CalibrationWindow` 挂载时先显示 loading，调 `recoveryFromBackend()`，根据返回的 `status.type` + `status.status` 决定 `currentView`：有任务（running/paused/completed/error/stopped）→ 进对应 Main；idle → 进 Home。避免落点闪烁。

**Acceptance criteria:**
- [ ] `CalibrationWindow.onMounted` 先 `currentView=loadingPlaceholder`，再 `await recoveryFromBackend()`
- [ ] 落点决策：`status && status.status !== 'idle'` → `currentView = mainComponents[status.type]`、`activeCalibrationType = status.type`
- [ ] 落点决策：`!status || status.status === 'idle'` → `currentView = CalibrationHome`
- [ ] recovery 期间（`isRecovering=true`）显示 loading 占位，不渲染 Home / Main（避免闪烁）
- [ ] `activeCalibrationType` 不持久化到 store（本地 ref，每次挂载重新决策）
- [ ] recovery 失败时仍进 Home（不阻塞用户选探针），但显示错误条

**Verification:**
- [ ] `npm run typecheck` + `npm run build` 通过
- [ ] 手测 U14(a)：五孔 running 切到看板再切回，直接进五孔 Main 显示运行中，无 Home 闪烁
- [ ] 手测 U14(b)：五孔 completed 切回，直接进五孔 Main 显示完成
- [ ] 手测 U14(c)：无任务切回，进 Home

**Dependencies:** Task 2

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/calibration/CalibrationWindow.vue`

**Estimated scope:** M（1 文件，改 ~40 行 + template）

---

### Task 7: CalibrationWindow 跨类型切换拦截确认框

**Description:** **仅** `handleSelectCalibration`（Home 点另一种探针卡片）在 `isRunning || isPaused` 且目标类型与 `store.status?.type` 不同时弹 `UiDialog` 确认框；`handleBack`（Main 返回 Home）**不拦截、不 stop**，直接返回 Home，任务继续后台 1Hz 心跳。取消则留下，确认则 `await stop()` 后再切换。

**Acceptance criteria:**
- [ ] `handleSelectCalibration(type)`：若 `calibrationStore.isRunning || calibrationStore.isPaused` 且 `type !== calibrationStore.status?.type`，弹确认框（同类型卡片重复点击不弹框）
- [ ] `handleBack()`：**不弹框、不 stop**，直接返回 Home（任务继续后台 1Hz 心跳，Home 显示进行中标识）
- [ ] 确认框文案：「当前有校准任务进行中，停止并切换？」+ 「取消 / 停止并切换」两按钮（仅用于「换探针类型」，不要写成「返回也会停」）
- [ ] 取消：不切换，留在当前画面
- [ ] 确认：`await calibrationStore.stop()` → 再执行切换
- [ ] idle / completed / error / stopped 态直接切换，不弹框
- [ ] 确认框用项目现有 `UiDialog` 组件，不引入新依赖

**Verification:**
- [ ] `npm run typecheck` + `npm run build` 通过
- [ ] 手测 U4：五孔 running → Home → 点三孔，弹框；取消留五孔；确认则停止五孔并切三孔
- [ ] 手测 U15：五孔 running → Home → 再点五孔，不弹框不 stop，直接进五孔 Main
- [ ] 手测：五孔 running → 点 Main 内「返回」→ 直接回 Home，任务继续（U11 仍可看到进行中标识），不弹框不 stop

**Dependencies:** Task 3（依赖 stop 行为正确）

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/calibration/CalibrationWindow.vue`

**Estimated scope:** M（1 文件，改 ~50 行 + template）

---

### Task 8: CalibrationHome 后台任务进行中标识

**Description:** `CalibrationHome` 卡片上显示后台任务进行中标识（角标 / 文案），让用户离开校准页后回来能感知到任务仍在跑。读 `calibrationStore.isRunning || calibrationStore.isPaused` + `calibrationStore.status?.type`。

**Acceptance criteria:**
- [ ] `CalibrationHome` 顶部或对应探针卡片显示「校准进行中：五孔」文案 / 角标
- [ ] 标识来源：`calibrationStore.status?.type` + `isRunning || isPaused`
- [ ] idle / completed（可显示「已完成」角标，可选）/ error / stopped 态不显示「进行中」标识；stopped 后标识消失
- [ ] 运行 vs 暂停使用不同文案 / 角标（「校准进行中」vs「已暂停」），避免用户误以为仍在运动（U17）
- [ ] 标识样式复用现有 `UiAlert` / badge 样式，不新增组件
- [ ] 标识实时响应 store 状态变化（computed 派生）

**Verification:**
- [ ] `npm run typecheck` + `npm run build` 通过
- [ ] 手测 U11：running 时返回 Home，对应卡片显示进行中标识
- [ ] 手测：stop 后标识消失

**Dependencies:** Task 2

**Files likely touched:**
- `apps/desktop-wails/frontend/src/components/calibration/CalibrationHome.vue`

**Estimated scope:** S（1 文件，改 ~20 行 + template）

---

## Phase 4: 测试与验证

### Task 9: store 单元测试

**Description:** 为 `calibrationStore` 新增 / 变更方法写 Vitest 单元测试，覆盖 recovery / acquire-release / stop 保留 / start 清空 / 恢复失败路径。

**Acceptance criteria:**
- [ ] `recoveryFromBackend` 测试：mock status 返回 running / paused / completed / error / stopped / idle，断言 store 同步
- [ ] `recoveryFromBackend` 失败测试：mock 抛错，断言 `recoveryError` 非空、旧状态保留
- [ ] `acquireView` / `releaseView` 测试：断言 `activeViewCount` 计数、polling 间隔切换（1Hz vs uiRefreshHz）
- [ ] `stop` 测试：断言 `status.status==='stopped'`、`dataPoints` 保留
- [ ] `startCalibration` 测试：断言旧会话清空（dataPoints 归零、completeEvent=null）
- [ ] 覆盖率：新增 / 变更方法行覆盖率 ≥90%
- [ ] 测试文件放在现有测试目录（确认 `npm run test` 能发现）

**Verification:**
- [ ] `npm run test` 全绿
- [ ] Vitest 覆盖率报告确认 ≥90%

**Dependencies:** Task 1, Task 2, Task 3

**Files likely touched:**
- `apps/desktop-wails/frontend/src/stores/__tests__/calibrationStore.test.ts`（或项目既有测试目录）

**Estimated scope:** M（1 新文件，~200 行测试）

---

### Task 10: 全量验证 + U1-U14 手测清单执行

**Description:** 执行 spec 中 U1-U14 全部手测用例，确认 SC1-SC13 全部满足。修复发现的问题。

**Acceptance criteria:**
- [ ] U1-U14 全部手测通过，结果记录
- [ ] `npm run typecheck` + `npm run build` + `npm run test` 全绿
- [ ] SC1-SC13 全部满足
- [ ] store 新增方法单测覆盖率 ≥90%
- [ ] gitnexus `detect_changes` 确认变更范围符合预期

**Verification:**
- [ ] 手测清单全部 ✅
- [ ] CI 命令全绿
- [ ] 覆盖率报告 ≥90%

**Dependencies:** Task 1-9 全部完成

**Files likely touched:**
- 视手测发现的问题而定

**Estimated scope:** M（验证 + 修复）

---

## 验收总结

| 阶段 | Task | 检查点 |
|---|---|---|
| Phase 1 | 1-3 | Checkpoint 1: store 基础设施 |
| Phase 2 | 4-5 | Checkpoint 2: 组件接入 |
| Phase 3 | 6-8 | Checkpoint 3: 落点与拦截 |
| Phase 4 | 9-10 | Checkpoint 4: Complete |

**总计：10 个 Task，4 个 Checkpoint，预计触及 7-8 个文件。**
