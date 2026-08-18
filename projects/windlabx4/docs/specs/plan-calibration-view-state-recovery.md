# Implementation Plan: 探针校准画面切换状态恢复

> 关联 spec：[spec-calibration-view-state-recovery.md](./spec-calibration-view-state-recovery.md)
> 状态：待批准（已按三文档联合 review 修复）
> 日期：2026-07-15

## Overview

将校准画面的「切走即丢状态」改为「store 跨画面持久化 + 后端兜底恢复 + 落点智能决策」。改造集中在 `calibrationStore` + `useCalibrationWorkflow` + `CalibrationWindow` + 四个 `*Main.vue`，不触及后端。完成后用户从五孔校准切到看板再切回，直接进五孔 Main 显示运行中状态，进度 / 点数 / 已用时连续无跳变。

## Architecture Decisions

| # | 决策 | 理由 |
|---|---|---|
| A1 | 不引入 keep-alive | spec Out of Scope 明确排除；P0+P1 方案已覆盖需求 |
| A2 | store 新增 `recoveryFromBackend` / `acquireView` / `releaseView`，不新增 store | spec Confirmed Decision #12 |
| A3 | `stop()` 改为终态保留（status='stopped'、不清 dataPoints、`stopElapsedTick()` 定格已用时、保留短时 1Hz 心跳确认后端落地），由 `startCalibration` 在后端 start **成功**时清旧会话（失败不清，避免抹掉上趟结果） | spec I7 + Decision #3/#4 |
| A4 | `CalibrationWindow.onMounted` 先 recovery 再决定 `currentView`，recovery 期间显示 loading | spec I10 + Decision #13，避免落点闪烁 |
| A5 | 四个 Main 统一走 `useCalibrationWorkflow` 的 acquire/release + recovery，五孔删除 `onBeforeUnmount` 的 `reset()` | spec I9 |
| A6 | 跨类型切换拦截**仅**放在 `CalibrationWindow.handleSelectCalibration`（Home 点另一种探针卡片，running/paused 且类型不同弹 `UiDialog` 确认）；`handleBack` 返回 Home **不拦截、不 stop**，任务继续后台 1Hz 心跳 | spec I5 + Decision #5/#6 |
| A7 | polling 降频实现：store 内 `activeViewCount` 驱动 `startStatusPolling` 重启 timer 切换间隔（1Hz vs uiRefreshHz） | spec I4 |
| A8 | `CalibrationWindow` 负责模块级 recovery + 落点；Main/composable 仅 `acquireView`，`lastRecoveryAt` 2s 内跳过二次 recovery，避免重复 loading | spec Decision #14 |
| A9 | `stopped` 态需四 Main 的 `statusText`/状态色同步映射，仅改 store `mapCalibrationState` 不够 | spec Decision #15 |

## 依赖图（实现顺序自底向上）

```
Task 1: store 基础设施（activeViewCount / acquireView / releaseView）
    │
    ├── Task 2: store recoveryFromBackend（依赖 Task 1 的 polling 控制）
    │
    ├── Task 3: store stop() 终态保留改造（依赖 Task 2 的 recovery 语义）
    │
    ├── Task 4: useCalibrationWorkflow 接入 acquire/release + recovery（依赖 Task 1/2）
    │
    ├── Task 5: 四个 Main 统一 unmount 行为（依赖 Task 4）
    │       └── 五孔删除 reset()、三孔/总压/总温补 releaseView
    │
    ├── Task 6: CalibrationWindow 落点决策 + recovery（依赖 Task 2）
    │
    ├── Task 7: CalibrationWindow 跨类型切换拦截（依赖 Task 3 的 stop 行为；与 Task 6 改同一文件，建议串行）
    │
    ├── Task 8: CalibrationHome 后台任务标识（依赖 Task 2 的 store 状态）
    │
    └── Task 9: 单元测试（依赖 Task 1-3 的 store 方法稳定）
```

## Task List

### Phase 1: Store 基础设施（Task 1-3）

- [ ] Task 1: store 新增 `activeViewCount` / `acquireView` / `releaseView` + polling 降频
- [ ] Task 2: store 新增 `recoveryFromBackend()` + `isRecovering` / `recoveryError`
- [ ] Task 3: store `stop()` 改终态保留，`startCalibration` 清旧会话

### Checkpoint 1: Store 基础设施

- [ ] `npm run typecheck` 通过
- [ ] 不影响现有 `startCalibration` / `pause` / `resume` 流程
- [ ] （Store 单测 Task 9 在 Phase 4 执行；此处可先为 Task 1-3 写最小单测，但非阻塞）

### Phase 2: 组件接入（Task 4-5）

- [ ] Task 4: `useCalibrationWorkflow` 接入 acquire/release + onMounted 调 recovery
- [ ] Task 5: 四个 Main 统一 unmount 行为（删五孔 reset、四组件补 releaseView）

### Checkpoint 2: 组件接入

- [ ] `npm run typecheck` + `npm run build` 通过
- [ ] U1 手测：五孔 running 切走切回，状态连续
- [ ] U7 手测：四探针行为一致

### Phase 3: 落点与拦截（Task 6-8）

- [ ] Task 6: `CalibrationWindow` onMounted 落点决策 + recovery loading
- [ ] Task 7: `CalibrationWindow` 跨类型切换拦截确认框
- [ ] Task 8: `CalibrationHome` 后台任务进行中标识

### Checkpoint 3: 落点与拦截

- [ ] U4 手测：跨类型切换弹确认框
- [ ] U14 手测：切回落点正确（running→Main / idle→Home）
- [ ] U11 手测：Home 后台任务标识可见

### Phase 4: 测试与验证（Task 9-10）

- [ ] Task 9: store 单元测试（recovery / acquire-release / stop 保留 / start 清空 / 恢复失败）
- [ ] Task 10: 全量验证 + U1-U14 手测清单执行

### Checkpoint 4: Complete

- [ ] SC1-SC13 全部满足
- [ ] `npm run typecheck` + `build` + `test` 全绿
- [ ] store 新增方法单测覆盖率 ≥90%
- [ ] Ready for review

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| `stop()` 改终态保留后，旧的 `status='idle'` 调用方（如 UI 判断 `!status` 显示空闲）行为变化 | 中 | grep 全部 `status.value?.status === 'idle'` / `!status.value` 调用点，改为同时识别 `'stopped'` / `'completed'` / `'error'` |
| `recoveryFromBackend` 与现有 `statusPolling` 并发写 `status.value` 竞态 | 中 | recovery 前先 `stopStatusPolling()`，recovery 完成后由 `acquireView` 决定是否重启高频 polling |
| `CalibrationWindow` 落点决策期间显示 loading，若 recovery 慢用户感知卡顿 | 低 | 300ms 阈值后才显示 loading 文案（spec Recovery UX），通常 recovery < 200ms |
| 四个 Main 改造遗漏总温（`TotalTemperatureMain.vue`） | 中 | Task 5 验收清单明确列出四组件，PR review 检查 |
| `mapCalibrationState` 不识别后端 `stopped` 字符串 | 中 | Task 3 同步改 mapCalibrationState，新增 `case 'stopped': return 'stopped'` |
| 跨类型切换拦截误伤「无任务时切换」或同类型重复点击 | 低 | Task 7 拦截条件明确：仅 `isRunning || isPaused` 且 `type !== store.status?.type` 时弹框，idle/completed/error/stopped 直接切，同类型不弹 |
| `CalibrationWindow` 模块级 recovery 与 Main `onMounted` 二次 recovery 叠加，导致 `isRecovering` 闪两次 / 重复 loading | 低 | store 记录 `lastRecoveryAt`，Main 2s 内跳过二次 recovery（spec Decision #14） |
| 跨类型拦截用 `activeCalibrationType` 判断（不持久化）导致同类型误弹或判断错误 | 中 | 拦截条件统一用 `store.status?.type`（spec Decision #6） |
| `stop()` 终态保留后 `statusText` 仍显示「空闲」，导致 U5/U12 验收失败 | 中 | Task 5 同步四 Main 的 `statusText`/状态色映射 `stopped`（spec Decision #15） |

## Open Questions

无阻塞问题。spec Open Questions 已全部收敛。

## Parallelization Opportunities

- Task 7（跨类型拦截）与 Task 6（落点决策）可并行，但都改 `CalibrationWindow.vue`，建议串行避免冲突
- Task 9（单元测试）可在 Task 1-3 完成后立即开始，与 Task 4-8 并行
- Task 8（Home 标识）独立于 Task 4-7，可并行
