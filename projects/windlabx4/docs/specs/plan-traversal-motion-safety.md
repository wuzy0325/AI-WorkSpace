# Implementation Plan: 遍历测试运动安全防护

> 关联 Spec: [spec-traversal-motion-safety.md](./spec-traversal-motion-safety.md)
> 状态: 待审阅
> 日期: 2026-07-14

## Overview

按 spec 在遍历测试运动阶段引入 7 类安全判定（到位/正常/超差/越过目标/无进展/严重偏离/撞限位）、可配置阈值、跨样本看门狗、按控制器作用域的 Stop/EmergencyStop、配置强校验、稳定阶段持续复检，以及 WTNMC4A 软限位事前校验。共 16 个任务，5 个检查点。

## Architecture Decisions

1. **纯函数优先**：`EvaluateMotionSafety` 不访问硬件，所有判定逻辑可单测覆盖；故障场景通过 fake `MotionAccess` 注入状态序列
2. **看门狗单一职责**：跨样本的 `NoProgress` 与 `Overshoot` 由同一 `motionWatchdog` 结构维护，共享位置快照，避免双重轮询开销
3. **`MotionSafetyFailure` 携带事故现场**：判定失败时立即快照 `controllerID/axis/verdict/target/actual/pointIndex`，错误处理阶段不再读硬件，避免现场被覆盖
4. **复用已有 `EmergencyStop`**：`motionManagerWrapper` 已实现 `EmergencyStop(ctx, id)`，只需在 `MotionAccess` 端口暴露，不改适配器层
5. **配置随快照恢复**：`MotionSafetyConfig` 进入 `Config.MotionSafety`，自动随 `TraversalRunSnapshot` 进入 checkpoint，无需新增平行字段
6. **WTNMC4A 软限位参照 B140**：在 `MoveTo` 入口加 `validateWTNMC4ATarget`，与 `validateB140Target` 签名一致，确保两个真实硬件适配器行为对齐

## Task List

### Phase 1: 类型与默认值（基础）

#### Task 1: 新增 `MotionSafetyConfig` 类型与默认值

**Description**: 在 `core/traversal/types.go` 新增 `MotionSafetyConfig` 结构、`DefaultMotionSafety` 默认值、`Resolve`/`Merge`/`WithoutAxisOverrides` 辅助方法；在 `Config` 结构中新增 `MotionSafety *MotionSafetyConfig` 字段。

**Acceptance criteria:**
- [ ] `MotionSafetyConfig` 包含 `ArrivalTolerance`、`CriticalDeviationLimit`、`NoProgressTimeoutMs`、`ProgressEpsilon`、`AxisOverrides` 5 个字段，全部用指针支持零值即默认
- [ ] `DefaultMotionSafety` 提供默认值（`ArrivalTolerance=0.2`、`CriticalDeviationLimit=5.0`、`NoProgressTimeoutMs=2000`、`ProgressEpsilon=0.001`）
- [ ] `Resolve(axis)` 返回"默认 > 全局 > 按轴覆盖"合并后的有效配置
- [ ] `Config.MotionSafety` 字段为 `*MotionSafetyConfig`，零值时下游使用 `DefaultMotionSafety`
- [ ] JSON tag 使用 `omitempty`，旧配置无此字段时反序列化不报错

**Verification:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] 手动确认 `Config` JSON 序列化/反序列化往返测试通过

**Dependencies:** 无

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/core/traversal/types.go`

**Estimated scope:** S（1 文件）

---

### Phase 2: 纯函数与配置校验（不依赖硬件）

#### Task 2: 实现 `EvaluateMotionSafety` 纯函数 + 单测

**Description**: 在 `traversal_helpers.go` 新增 `MotionSafetyVerdict` 枚举（7 个值）和 `EvaluateMotionSafety(axisStatus, target, config)` 纯函数；编写覆盖所有分支的单测。

**Acceptance criteria:**
- [ ] `MotionSafetyVerdict` 枚举包含 `OK/Arrived/Deviation/CriticalDeviation/LimitTriggered/NoProgress/Overshoot` 7 个值
- [ ] `EvaluateMotionSafety` 按 spec 优先级判定：限位 > 运动中(OK) > 到位 > 严重偏离 > 超差
- [ ] 单测覆盖：到位、运动中长行程不误判、轴停超差、严重偏离、撞限位、按轴覆盖、未配置字段继承全局
- [ ] 函数签名纯函数，不访问硬件或端口

**Verification:**
- [ ] `go test ./internal/usecase/... -run TestEvaluateMotionSafety -v` 全绿
- [ ] 覆盖率 100% 分支覆盖

**Dependencies:** Task 1

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_helpers.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_safety_test.go`（新建）

**Estimated scope:** S（2 文件）

---

#### Task 3: 实现跨样本 `motionWatchdog`（NoProgress + Overshoot）+ 单测

**Description**: 新建 `traversal_motion_watchdog.go`，实现 `motionWatchdog` 结构，跟踪每轴位置、运动方向、上次进展时间；提供 `Observe(controllerID, axis, target)` 方法，返回 `*MotionSafetyFailure` 或 nil。

**Acceptance criteria:**
- [ ] 首次观察 `Moving=true` 时记录位置、方向 `sign(position-target)`、时间
- [ ] 后续 `Moving=true` 且方向翻转且 `|position-target| > ArrivalTolerance` 时返回 `Overshoot` 故障
- [ ] `Moving=true` 且超过 `NoProgressTimeoutMs` 无 `ProgressEpsilon` 量进展时返回 `NoProgress` 故障
- [ ] 有效进展（位移 ≥ `ProgressEpsilon`）重置进展时间
- [ ] 轴停止时立即交还 `EvaluateMotionSafety`，看门狗不再判定
- [ ] 使用单调时钟（`time.Monotonic` 或 `time.Now()` 的 monotonic 部分）
- [ ] 单测覆盖：NoProgress 触发、Overshoot 触发、正常接近不误报、有效进展重置看门狗

**Verification:**
- [ ] `go test ./internal/usecase/... -run TestMotionWatchdog -v` 全绿

**Dependencies:** Task 1, Task 2

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_watchdog.go`（新建）
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_safety_test.go`

**Estimated scope:** M（2 文件）

---

#### Task 4: `ParseConfig` 校验 `MotionSafetyConfig`

**Description**: 在 `traversal_config.go` 的 `ParseConfig` 中加入 `MotionSafetyConfig` 校验：浮点有限性、阈值关系（`ArrivalTolerance > 0`、`CriticalDeviationLimit > ArrivalTolerance`、`ProgressEpsilon > 0`）、`NoProgressTimeoutMs >= 2*motionCompletePoll`、`AxisOverrides` 仅允许已绑定轴名且不允许递归覆盖。

**Acceptance criteria:**
- [ ] 非法阈值（负数、NaN、Inf、阈值倒置）在 Start 前被拒绝
- [ ] 未知轴名（不在 `MotionAxes` 绑定中）的覆盖被拒绝
- [ ] 覆盖项内嵌 `AxisOverrides` 被拒绝（避免递归）
- [ ] 校验失败时不获取工作流锁、不创建文件、不下发运动命令
- [ ] 旧配置无 `motionSafety` 字段时使用 `DefaultMotionSafety`，不报错

**Verification:**
- [ ] `go test ./internal/usecase/... -run TestParseConfig_MotionSafety -v` 全绿
- [ ] 测试用例覆盖：默认值、按轴覆盖、非法值拒绝、未知轴拒绝、递归覆盖拒绝

**Dependencies:** Task 1

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_config.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_safety_test.go`

**Estimated scope:** S（2 文件）

---

### Checkpoint 1: 基础完成

- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./internal/usecase/... -run 'TestEvaluateMotionSafety|TestMotionWatchdog|TestParseConfig_MotionSafety' -v` 全绿
- [ ] 类型与纯函数不依赖任何硬件或端口
- [ ] **审阅点：与用户确认纯函数语义后再进入集成阶段**

---

### Phase 3: 端口扩展与错误码

#### Task 5: `MotionAccess` 端口新增 `EmergencyStop`

**Description**: 在 `ports/traversal.go` 的 `MotionAccess` 接口新增 `EmergencyStop(ctx context.Context, id string) error` 方法；确认 `motionManagerWrapper` 已实现该方法（wrapper.go:79-81 已存在），无需修改适配器层。

**Acceptance criteria:**
- [ ] `MotionAccess` 接口含 `EmergencyStop(ctx, id) error`
- [ ] `motionManagerWrapper` 实现该方法（已存在，仅需接口暴露）
- [ ] 所有 `MotionAccess` 的 fake/mock 实现同步添加 `EmergencyStop` 方法
- [ ] 编译通过，既有测试不破坏

**Verification:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./internal/usecase/... ` 全绿（既有测试不回归）

**Dependencies:** 无（端口独立）

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/ports/traversal.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_*_test.go`（fake 实现补方法）

**Estimated scope:** S（2-3 文件）

---

#### Task 6: 新增运动安全错误码

**Description**: 在 `core/traversal/types.go` 的 `ErrorCode` 常量区新增 7 个错误码：`ErrPositionDeviation`、`ErrMotionOvershoot`、`ErrMotionNoProgress`、`ErrCriticalPositionDeviation`、`ErrLimitSwitchTriggered`、`ErrMotionStatusUnavailable`、`ErrEmergencyStopFailed`、`ErrMotionTimeout`；新增 `errorCodeFor(verdict)` 映射函数。

**Acceptance criteria:**
- [ ] 8 个新错误码（含 `ErrMotionTimeout`）作为 `ErrorCode` 常量
- [ ] `errorCodeFor(MotionSafetyVerdict)` 返回对应错误码
- [ ] `RequiresEmergencyStop()` 方法在 `MotionSafetyFailure` 上判定是否需要急停（`CriticalDeviation`/`LimitTriggered`/`StatusUnavailable` 返回 true）
- [ ] 既有错误码（`ErrMotionFailed` 等）保持不变

**Verification:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./internal/core/traversal/... ` 全绿

**Dependencies:** Task 2（使用 `MotionSafetyVerdict`）

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/core/traversal/types.go`
- `projects/windlabx4/services/api-go/internal/core/traversal/errors.go`

**Estimated scope:** S（2 文件）

---

### Phase 4: 故障处理与集成

#### Task 7: 实现 `handleMotionSafetyFailure` 与 `emergencyStopMotionControllers`

**Description**: 在 `traversal.go` 新增 `emergencyStopMotionControllers` 方法（从 `motionAxes` 解析唯一 controllerID 集合，逐台调用 `EmergencyStop`，聚合错误，不因首台失败跳过后续）和 `handleMotionSafetyFailure(failure)` 方法（按 `RequiresEmergencyStop` 分发 Stop/EmergencyStop，急停失败时 fallback Stop）。

**Acceptance criteria:**
- [ ] `emergencyStopMotionControllers` 对所有参与控制器逐台调用，聚合所有错误
- [ ] `handleMotionSafetyFailure` 对 `CriticalDeviation/LimitTriggered/StatusUnavailable` 调用急停
- [ ] 急停失败时 fallback 到 `stopMotionAxes`，返回 `ErrEmergencyStopFailed` 聚合错误
- [ ] 普通超差/无进展/越过目标调用 `stopMotionAxes`，返回对应错误码
- [ ] 所有 `EmergencyStop` 调用打 `Warn` 级日志，含 `controllerID/axis/verdict/pointIndex/target/actual`
- [ ] 单测覆盖：Stop 路径、EmergencyStop 路径、部分急停失败 + Stop 兜底

**Verification:**
- [ ] `go test ./internal/usecase/... -run 'TestHandleMotionSafetyFailure|TestEmergencyStop' -v` 全绿
- [ ] `handleMotionSafetyFailure` 100% 路径覆盖

**Dependencies:** Task 5, Task 6

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_safety_test.go`

**Estimated scope:** M（2 文件）

---

#### Task 8: 改造 `waitForMotionComplete` 集成安全判定

**Description**: 修改 `waitForMotionComplete` 签名返回 `(completed bool, failure *MotionSafetyFailure)`；在轮询循环中：(1) `validateMotionStatuses` 检查控制器掉线/急停/目标轴状态缺失（连续 3 个快照）；(2) `motionTargetsReached` 用可配置 `ArrivalTolerance`；(3) 每轴 `EvaluateMotionSafety` + 看门狗 `Observe`；(4) 120s 兜底返回独立 `ErrMotionTimeout`。

**Acceptance criteria:**
- [ ] 签名变更：`(completed bool, failure *MotionSafetyFailure)`
- [ ] `motionTargetsReached` 接受 `MotionSafetyConfig` 参数，使用 `ArrivalTolerance` 替代硬编码 0.01
- [ ] `validateMotionStatuses` 检测：已绑定控制器掉线、已急停、目标轴连续 3 快照缺失
- [ ] 看门狗 `Observe` 与 `EvaluateMotionSafety` 共享同一轮询数据
- [ ] ctx 取消优先于故障判定
- [ ] 单测覆盖：到位、超差快速失败、限位快速失败、无进展、越过目标、掉线、状态缺失去抖、ctx 取消、120s 兜底

**Verification:**
- [ ] `go test ./internal/usecase/... -run TestWaitForMotionComplete -v` 全绿
- [ ] 覆盖率 ≥ 90%

**Dependencies:** Task 2, Task 3, Task 7

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_acquisition.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_helpers.go`（`motionTargetsReached` 签名）
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_safety_test.go`

**Estimated scope:** M（3 文件）

---

### Checkpoint 2: Usecase 集成完成

- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./internal/usecase/... ` 全绿
- [ ] **审阅点：与用户确认故障处理流程后再进入 RunCurrentPoint 改造**

---

### Phase 5: 主流程接入与稳定阶段复检

#### Task 9: `RunCurrentPoint` Moving 阶段接入新 `waitForMotionComplete`

**Description**: 修改 `RunCurrentPoint` 的 Moving 阶段，调用新 `waitForMotionComplete`；返回 failure 时调用 `handleMotionSafetyFailure`，返回错误终止遍历。同时替换原有"调用 `stopMotionAxes` + 返回 `ErrMotionFailed`"的旧超时分支。

**Acceptance criteria:**
- [ ] Moving 阶段使用新签名 `waitForMotionComplete`
- [ ] 失败时调用 `handleMotionSafetyFailure` 而非旧的 `stopMotionAxes + ErrMotionFailed`
- [ ] 暂停场景（`paused=true`）保持原行为，不视为故障
- [ ] 既有 `RunCurrentPoint` 测试全部通过（必要时更新 mock 期望）

**Verification:**
- [ ] `go test ./internal/usecase/... -run TestRunCurrentPoint -v` 全绿
- [ ] 既有 RunCurrentPoint 测试不回归

**Dependencies:** Task 7, Task 8

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_acquisition.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_*_test.go`（更新 mock 期望）

**Estimated scope:** M（2 文件）

---

#### Task 10: 稳定阶段持续安全复检

**Description**: 修改 `waitForStabilization`，在 fixed 模式的 sleep 循环和 adaptive 模式的轮询循环中加入安全复检：调用 `EvaluateMotionSafety` + `motionWatchdog.Observe` + `validateMotionStatuses`；任一失败立即中断稳定等待并返回故障，由 `RunCurrentPoint` 走 `handleMotionSafetyFailure`。

**Acceptance criteria:**
- [ ] fixed 模式：sleep 期间周期性（`motionCompletePoll` 间隔）调用安全复检
- [ ] adaptive 模式：每次压力采样前调用安全复检
- [ ] 复检失败立即返回 `*MotionSafetyFailure`，不再等待稳定
- [ ] 稳定阶段故障与 Moving 阶段共用 `handleMotionSafetyFailure` 处理
- [ ] 单测覆盖：稳定阶段位置漂移触发超差、限位触发、状态掉线

**Verification:**
- [ ] `go test ./internal/usecase/... -run 'TestRunCurrentPoint_Stabilization|TestWaitForStabilization' -v` 全绿

**Dependencies:** Task 9

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_acquisition.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_safety_test.go`

**Estimated scope:** M（2 文件）

---

### Checkpoint 3: 后端运动安全完成

- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./internal/... ./api/...` 全绿
- [ ] `go vet ./...` 无错
- [ ] fake 状态序列验证：注入轴停未到位/运动中无进展/越过目标/撞限位场景，遍历在预期时间内终止并返回正确错误码
- [ ] **审阅点：与用户确认后端完整行为后再进入设备适配器与前端**

---

### Phase 6: 设备适配器与恢复（可与 Phase 7 并行）

#### Task 11: WTNMC4A `MoveTo` 软限位事前校验

**Description**: 在 `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go` 的 `MoveTo` 入口（脉冲换算和 SDK 调用之前）加入 `validateWTNMC4ATarget` 校验：目标为有限值；已配置 `MinLimit <= MaxLimit`；目标位于闭区间 `[MinLimit, MaxLimit]`。失败时返回错误且不调用 `InitLVDV`/`StartLVDV`。

**Acceptance criteria:**
- [ ] `MoveTo` 在脉冲换算前调用 `validateWTNMC4ATarget`
- [ ] 非有限目标（NaN/Inf）返回错误
- [ ] `MinLimit > MaxLimit` 配置错误返回错误
- [ ] 目标超出闭区间返回错误
- [ ] 边界值（恰好等于 `MinLimit` 或 `MaxLimit`）允许执行
- [ ] 单侧限位（仅配 `MinLimit` 或仅配 `MaxLimit`）只校验该侧
- [ ] 校验失败时 SDK 调用数为 0

**Verification:**
- [ ] `go test ./shared/device-sdk/go/motion/adapters/hardware/... -run TestWTNMC4AMoveTo_SoftLimit -v` 全绿

**Dependencies:** 无（独立于 usecase 改造）

**Files likely touched:**
- `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go`
- `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion_test.go`

**Estimated scope:** S（2 文件）

---

#### Task 12: 恢复任务使用 checkpoint 中的 `MotionSafetyConfig`

**Description**: 确认 `MotionSafetyConfig` 作为 `Config.MotionSafety` 字段已随 `TraversalRunSnapshot.Config` 进入 checkpoint；在 `Resume` 路径中校验使用 snapshot 中的 `MotionSafety`，不重新读取前端当前配置。

**Acceptance criteria:**
- [ ] `Config.MotionSafety` 序列化进入 checkpoint
- [ ] `Resume` 时使用 snapshot 中的 `MotionSafety`，不重新解析前端配置
- [ ] snapshot 中 `MotionSafety` 为 nil 时使用 `DefaultMotionSafety`
- [ ] 单测覆盖：snapshot 阈值与前端当前配置不同时使用 snapshot 值

**Verification:**
- [ ] `go test ./internal/usecase/... -run TestResume_UsesSnapshottedMotionSafety -v` 全绿

**Dependencies:** Task 1

**Files likely touched:**
- `projects/windlabx4/services/api-go/internal/usecase/traversal_checkpoint.go`
- `projects/windlabx4/services/api-go/internal/usecase/traversal_motion_safety_test.go`

**Estimated scope:** S（2 文件）

---

### Phase 7: 前端类型与 UI（可与 Phase 6 并行）

#### Task 13: 前端 `MotionSafetyConfig` TS 类型 + Wails binding 同步

**Description**: 在 `apps/desktop-wails/frontend/src/types/traversal.ts` 新增 `MotionSafetyConfig` TS 类型；运行 `wails3 generate bindings` 同步后端类型；更新 `traversalStore.ts` 的 config 类型定义。

**Acceptance criteria:**
- [ ] `MotionSafetyConfig` TS 类型与后端 Go 结构字段一一对应
- [ ] `AxisOverrides` 为 `Record<string, MotionSafetyConfig>` 类型
- [ ] 所有指针字段在 TS 中为可选（`?`）
- [ ] `wails3 generate bindings` 生成结果已提交
- [ ] `traversalStore.ts` 中 `Config` 类型含 `motionSafety?: MotionSafetyConfig`

**Verification:**
- [ ] `npm run typecheck` 无错
- [ ] `npm run build` 无错

**Dependencies:** Task 1

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/types/traversal.ts`
- `projects/windlabx4/apps/desktop-wails/frontend/src/stores/traversalStore.ts`
- `projects/windlabx4/apps/desktop-wails/frontend/wailsjs/`（自动生成）

**Estimated scope:** S（2-3 文件）

---

#### Task 14: 新建 `MotionSafetyPanel.vue` 配置面板

**Description**: 在 `apps/desktop-wails/frontend/src/views/traversal/` 新建 `MotionSafetyPanel.vue`，提供全局阈值输入（到位容差/严重偏离/无进展超时/进展阈值）和按轴覆盖表格；遵循 WindLabX4 全局配置规范（§35：120px 标签列 + 1fr 控件列，紧凑密度）；在遍历配置页运动绑定区域附近挂载。

**Acceptance criteria:**
- [ ] 4 个全局阈值输入控件，单位按轴类型显示（线性 mm / 旋转 °）
- [ ] 按轴覆盖表格：每行一个绑定轴，可独立覆盖 4 个阈值，留空继承全局
- [ ] 输入值实时校验（正数、有限数、阈值关系），非法值红色提示
- [ ] 紧凑密度：标签列 120px、控件高度 1.625rem、字段间距 6px
- [ ] 配置变更通过 `traversalStore` 持久化到 localStorage

**Verification:**
- [ ] `npm run typecheck` 无错
- [ ] `npm run build` 无错
- [ ] 手动验证：修改阈值并刷新页面，值持久化

**Dependencies:** Task 13

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/views/traversal/MotionSafetyPanel.vue`（新建）
- `projects/windlabx4/apps/desktop-wails/frontend/src/views/traversal/TraversalConfigView.vue`（挂载点）

**Estimated scope:** M（2 文件）

---

#### Task 15: 前端安全失败告警显示

**Description**: 在遍历状态视图根据 `lastErrorCode` 显示对应严重级别的告警：超差/无进展/越过目标为"警告"级（橙色），严重偏离/撞限位/状态不可用/急停失败为"危险"级（红色），含轴名、目标、实际位置、错误码本地化文案。

**Acceptance criteria:**
- [ ] 8 个新错误码在 `i18nStore` 中有本地化文案
- [ ] 告警按错误码分级显示（警告/危险）
- [ ] 告警含轴名、目标位置、实际位置、错误码
- [ ] 急停类告警需用户手动确认才能关闭

**Verification:**
- [ ] `npm run typecheck` 无错
- [ ] `npm run build` 无错

**Dependencies:** Task 13

**Files likely touched:**
- `projects/windlabx4/apps/desktop-wails/frontend/src/views/traversal/TraversalStatusView.vue`
- `projects/windlabx4/apps/desktop-wails/frontend/src/stores/i18nStore.ts`

**Estimated scope:** S（2 文件）

---

### Checkpoint 4: 前端完成

- [ ] `npm run typecheck` 无错
- [ ] `npm run build` 无错
- [ ] 手动验证：配置面板可修改阈值并持久化
- [ ] 手动验证：fake 故障场景下前端显示对应告警

---

### Phase 8: 端到端验证

#### Task 16: 全套预提交校验 + 模拟器端到端验证

**Description**: 执行 spec 验证清单的全部命令；在模拟器中执行端到端验证：正常长行程不误停、轴停未到位快速失败、运动中无进展快速失败、越过目标快速失败、撞限位急停。

**Acceptance criteria:**
- [ ] `validate-structure.ps1` 通过
- [ ] `go test ./internal/... ./api/...` 全绿
- [ ] `go vet ./...` 无错
- [ ] `npm run typecheck` 无错
- [ ] `npm run build` 无错
- [ ] 模拟器手动验证：大于 5mm 正常长行程不误触发急停并正常到位
- [ ] 模拟器手动验证：注入撞限位场景立即急停
- [ ] fake 状态序列验证：轴停未到位/无进展/越过目标在预期时间内终止

**Verification:**
- [ ] spec §验证清单 全部勾选
- [ ] spec §Success Criteria 12 条功能性 + 4 条非功能性全部满足

**Dependencies:** Task 10, Task 11, Task 12, Task 14, Task 15

**Files likely touched:** 无（仅验证）

**Estimated scope:** XS

---

### Checkpoint 5: 完整交付

- [ ] spec §Approval Gate 满足
- [ ] 所有 16 个任务验收通过
- [ ] spec 状态由"待审阅"更新为"已批准 → 已实现"
- [ ] **提交前与用户最终审阅**

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| `motionTargetsReached` 签名变更影响既有调用方 | Med | 全局搜索调用点，统一更新；既有测试用默认 `ArrivalTolerance=0.01` 保持行为等价 |
| `waitForMotionComplete` 签名变更影响 `RunCurrentPoint` 既有流程 | High | Task 9 显式接入并更新既有测试 mock 期望；Checkpoint 2 专门审阅 |
| `MotionAccess.EmergencyStop` 接口变更影响所有 fake/mock | Med | Task 5 同步更新所有测试 fake；既有 mock 加空实现保持兼容 |
| 看门狗在暂停期间错误计时 | Med | Task 3 显式处理暂停/恢复时的时钟重置；单测覆盖暂停场景 |
| 默认阈值不适合所有设备 | High | spec §Confirmed Decisions #8 明确：默认值仅作为 HIL 起点，发布门禁必须完成 HIL 调参 |
| WTNMC4A 软限位校验与既有 `validateWTNMC4APositionSample` 重复 | Low | 事前校验只检查目标值合法性，事后校验保留位置样本合理性判定，二者职责不重叠 |
| `MotionSafetyFailure` 携带位置快照但 RunCurrentPoint 后续仍读硬件 | Med | 错误处理路径禁止重新读硬件状态；Task 7 单测验证 |
| 前端配置 UI 单位显示错误（线性/旋转混淆） | Med | Task 14 强制按轴类型显示单位；前端类型校验 |

## Parallelization Opportunities

可并行的任务组合：

- **Task 11（WTNMC4A 软限位）** 与 Phase 4-5 完全独立，可并行
- **Task 12（resume checkpoint）** 仅依赖 Task 1，可与 Phase 3-5 并行
- **Phase 7（前端 Task 13-15）** 仅依赖 Task 1，可与 Phase 4-6 并行

必须串行的依赖链：

- Task 1 → Task 2 → Task 3 → Task 8 → Task 9 → Task 10（核心后端链）
- Task 5 + Task 6 → Task 7 → Task 8（故障处理链）

## Open Questions

无。所有决策已在 spec §Confirmed Decisions 中确认。默认阈值的真机定值属于 HIL 发布门禁，不阻塞实现。
