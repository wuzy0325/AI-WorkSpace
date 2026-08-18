# Spec: 探针校准运动安全防护（位置超差快速失败与急停）

> 来源：遍历测试运动安全机制移植 → spec-driven-development Phase 1 → 规格审查修订
> 日期：2026-07-14
> 状态：待审阅
> 修订：v2
> 关联：
> - [spec-traversal-motion-safety.md](./spec-traversal-motion-safety.md)（同源机制，本 spec 复用其 verdict 体系与 MotionSafetyConfig 数据模型）
> - [spec-traversal-reliability-and-recovery.md](./spec-traversal-reliability-and-recovery.md)（不修改其不变量，仅扩展校准运动阶段的安全判定）

## Objective

**目标**：将遍历测试已验证的运动安全防护机制移植到探针校准模块，覆盖五孔/三孔/总压/总温全部 4 种探针类型，在运动阶段引入位置偏差主动检测与快速失败机制，避免因轴失步、卡死、撞限位或编码器故障导致长时间空跑（当前 fallbackRuntime 超时 30s）或撞机事故。

**用户**：
- 风洞探针校准操作人员——期望异常发生时立即停机并收到明确告警，而非"卡在某点位"假象
- 设备维护工程师——期望事故现场可追溯，错误信息能区分"超差"与"撞限位"与"控制器掉线"
- 校准流程开发人员——期望安全判定逻辑可配置、可测试、与遍历测试保持一致语义

**痛点**（来自代码调研）：

| # | 缺口 | 当前行为 | 风险 |
|---|---|---|---|
| 1 | 到位容差硬编码 `0.01` | [calibration.go:892](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/WindLabX4/services/api-go/internal/usecase/calibration.go#L892) `defaultTolerance = 0.01`，虽从 `EncoderCompensation.Tolerance` 兜底但全局不可配、无按轴覆盖 | 不同精度设备/旋转轴无法适配 |
| 2 | 无严重偏离急停 | 轴已停但偏差巨大时仍等 30s 超时 | 严重失步/撞阻后继续空跑，可能撞机 |
| 3 | 无运动中无进展检测 | `WaitForMotionComplete` 只判 `Moving=false`，运动中卡死（Moving=true 但位置不变）无法识别 | 卡死后等满 30s 超时 |
| 4 | 无越过目标检测 | 运动穿越目标位置后继续冲，无主动停机 | 越过目标后撞限位或撞机 |
| 5 | 不消费 `PosLimit`/`NegLimit` | 撞限位后等超时 | 撞机后不能快速失败 |
| 6 | 不检测控制器掉线/急停 | 掉线后 `StatusAll` 返回空，`remaining` 不减，等满 30s | 控制器故障后长时间空等 |
| 7 | `stopMotion` 只调轴级 `Stop` | [calibration.go:921](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/WindLabX4/services/api-go/internal/usecase/calibration.go#L921) `stopAllMotion` 仅 `Stop`，无 `EmergencyStop` | 严重异常不会锁存急停状态，也不会停止同一控制器的其它轴 |
| 8 | 无故障现场快照 | 超时只返回"运动超时未完成（>30s）"字符串，无控制器/轴/目标/实际/偏差信息 | 事故现场不可追溯，操作员需从日志反推 |
| 9 | 暂停恢复竞态 | `WaitForMotionComplete` 与遍历旧实现同构，暂停/恢复/超时都返回同一错误，调用方事后读 `IsRunning` 推断 | 恢复操作发生在两者之间会被误判为超时 |

**不在范围内**：
- 不改变校准主循环结构（`AutomaticCalibration.Run` 各探针类型的点位遍历流程）
- 不改变 4 种探针的算法逻辑（五孔插值、三孔马赫数、总压/总温公式）
- 不改变 CSV schema 与保存逻辑
- 不改变球罐闸门等待逻辑（`waitForSphereTankGateIfNeeded`）
- 不改变采集协调器（`AcquisitionCoordinator`）
- 不引入新的运动控制器适配器
- 不引入新的第三方依赖
- 不改变 `CalibrationRuntime` 既有接口签名（`MoveToPosition`/`WaitForMotionComplete`/`StopMotion` 保持向后兼容，急停能力通过可选扩展接口 `EmergencyStopProvider` 按需获取）

## Tech Stack

- **后端**：Go 1.25 + 标准库 `slog`（日志强制只用标准库）
- **架构**：六边形——`core/calibration`（类型）+ `usecase`（编排）+ `ports`（接口）+ `adapters`（共享 manager 包装）
- **设备 SDK**：`shared/device-sdk/go/motion`（端口与适配器，复用遍历测试已集成的 WTNMC4A/B140/Simulated）
- **前端**：Vue 3 + TypeScript + Naive UI + Wails v3 binding
- **测试**：Go 标准 `testing` + simulated 适配器注入故障

## Commands

```powershell
# 后端构建与校验
cd projects\WindLabX4\services\api-go
$env:GOWORK="off"
go build -buildvcs=false ./...
go vet ./...
go test ./internal/usecase/... ./internal/core/calibration/...

# 前端构建与校验
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run build
```

## Project Structure

新增与修改的文件分布：

```
projects/WindLabX4/
├── services/api-go/internal/
│   ├── core/calibration/
│   │   ├── types.go                  # 修改：Config 新增 MotionSafety 字段，Status 新增 LastErrorCode/MotionSafetyFailure 字段
│   │   └── automatic_calibration.go  # 修改：Run/MoveToPointWithOrder 适配 (completed, reason, failure) 三元组返回值
│   ├── core/traversal/
│   │   └── types.go                  # 修改：提升 motionInterruptReason → 导出类型 MotionInterruptReason（含 5 个常量）
│   ├── ports/
│   │   └── calibration.go            # 修改：新增 EmergencyStopProvider 可选扩展接口；CalibrationRuntime 自身签名不变
│   └── usecase/
│       ├── calibration.go             # 修改：fallbackRuntime.WaitForMotionComplete 重构为运动安全版本
│       │                              #       新增 handleCalibrationMotionSafetyFailure/failWithCode/recordMotionSafetyFailure
│       │                              #       新增 toMotionAxisBindings 转换函数
│       │                              #       runtimeAdapter 支持 EmergencyStopProvider 类型断言
│       ├── traversal_acquisition.go   # 修改：motionInterruptReason → traversal.MotionInterruptReason（类型提升，向后兼容）
│       └── calibration_motion_safety_test.go  # 新增：23 条单测覆盖安全判定/看门狗/急停/状态转换/配置合并
├── apps/desktop-wails/frontend/src/
│   ├── views/calibration/             # 修改：4 种探针配置界面挂载 MotionSafetyPanel（从 components/traversal/ 移动到 components/common/ 或在 calibration 中通过 @ alias 引用）
│   ├── components/common/
│   │   └── MotionSafetyPanel.vue      # 迁移：从 components/traversal/ 移动到共享位置（校准 + 遍历共用）
│   ├── stores/calibrationStore.ts     # 修改：持久化 motionSafety 字段
│   └── types/calibration.ts           # 修改：Config 类型新增 motionSafety 字段，Status 新增 lastErrorCode/motionSafetyFailure
```

> **MotionSafetyPanel.vue 迁移说明**：该组件当前位于 `components/traversal/MotionSafetyPanel.vue`，校准模块复用需要将其迁移到 `components/common/`（共享组件目录），遍历模块的导入路径同步更新。

## Confirmed Decisions

1. **复用遍历测试的 `MotionSafetyConfig` 数据模型**——`core/traversal.MotionSafetyConfig` 已是跨模块稳定类型，校准模块直接引用，避免重复定义与漂移。`core/calibration` 不新建配置类型。
2. **复用 7 类 verdict + 急停语义**——`MotionSafetyVerdict`（OK/Arrived/Deviation/CriticalDeviation/LimitTriggered/NoProgress/Overshoot/StatusUnavailable）及其 `RequiresEmergencyStop()`、`ErrorCodeFor()` 直接复用 `core/traversal` 定义。校准模块不新增 verdict。
3. **`MotionSafetyConfig` 挂到 `calibration.Config`**——新增字段 `MotionSafety *traversal.MotionSafetyConfig`，零值时下游使用 `traversal.DefaultMotionSafety()`。4 种探针共用同一份配置，不按探针类型区分（运动安全语义与探针类型正交）。
4. **`CalibrationRuntime` 接口扩展**——`EmergencyStopMotion` 不直接加入接口（避免破坏所有外部实现），而是定义为独立的可选扩展接口 `ports.EmergencyStopProvider`。`fallbackRuntime` 同时实现该接口（内部委托 `ports.MotionManager.EmergencyStop`）；`runtimeAdapter` 通过类型断言判断底层 runtime 是否支持急停，不支持时 fallback 到 `StopMotion`。
5. **`WaitForMotionComplete` 返回中断原因枚举**——与遍历测试 P1-2 修复一致，返回 `(completed, reason, failure)` 三元组，调用方按 reason 分支处理，不再事后读 `IsRunning` 推断，消除暂停恢复竞态。`reason` 类型使用 `core/traversal.MotionInterruptReason`（导出类型，与 `MotionSafetyVerdict` 同包），确保 `core/calibration` 包可引用。
6. **故障现场快照写入 `calibration.Status`**——新增字段 `MotionSafetyFailure *traversal.MotionSafetyFailure`，前端轮询拿到后展示告警卡片（复用遍历测试的 `TraversalLiveMonitor` 告警组件模式）。
7. **`arrivalTolerance` 默认值 `0.2`**——与遍历测试保持一致（兼顾定位精度与机械抖动/背隙），不再使用硬编码 `0.01`。`EncoderCompensation.Tolerance` 作为按轴兜底优先级高于默认值但低于 `MotionSafetyConfig`。
8. **超时时间从 30s 提升到 120s**——与遍历测试 `motionCompleteTimeout` 对齐，作为兜底超时；正常异常由 verdict 主动检测提前触发，不会真等满 120s。
9. **WTNMC4A 软限位校验被动覆盖**——校准模块的 `MoveToPosition` 调用链 `fallbackRuntime.MoveToPosition → ports.MotionManager.MoveTo → WTNMC4AMotionController.MoveTo` 已内建 `validateWTNMC4ATarget` 校验（遍历测试规格中实现），脉冲换算前拦截超行程目标。校准模块无需新增校验代码，仅需确保 `EncoderCompensation` 的 `MinLimit/MaxLimit` 配置被正确定义。
10. **不改变 4 种探针的运动顺序**——五孔 `MoveToPointWithOrder`（先α后β）与其它三种 `MoveToPoint`（并行）的现有顺序保持不变，安全判定在 `WaitForMotionComplete` 内部统一执行，与运动下发顺序解耦。

## Data Model

### `calibration.Config` 扩展

```go
// Config 校准任务通用配置（新增 MotionSafety 字段）
type Config struct {
    // ... 既有字段不变 ...
    // MotionSafety 运动安全配置。为 nil 时下游使用 traversal.DefaultMotionSafety()。
    // 4 种探针共用同一份配置，不按探针类型区分（运动安全语义与探针类型正交）。
    // 与遍历测试 traversal.Config.MotionSafety 同类型，保证跨模块配置语义一致。
    MotionSafety *traversal.MotionSafetyConfig `json:"motionSafety,omitempty"`
}
```

### `calibration.Status` 扩展

```go
// Status 校准任务状态（新增 LastErrorCode 与 MotionSafetyFailure 字段）
type Status struct {
    // ... 既有字段不变 ...
    // LastError 错误描述（既有字段，向后兼容）
    LastError string `json:"lastError,omitempty"`
    // LastErrorCode 结构化错误码（新增）。运动安全故障时写入对应的 MotionSafetyVerdict 错误码，
    // 前端根据此字段展示对应级别的告警（急停类红色 / 普通停止类橙色 / 超时类黄色）。
    // 非运动安全错误（采集失败/保存失败等）写入对应业务错误码。
    LastErrorCode string `json:"lastErrorCode,omitempty"`
    // MotionSafetyFailure 运动安全故障现场快照。
    // 仅在运动安全故障路径写入，其他错误路径（采集失败/保存失败等）保持 nil。
    // 前端轮询拿到后用于展示故障现场（控制器/轴/verdict/目标/实际/点号），
    // 避免 lastError 字符串正则解析的不稳定。
    MotionSafetyFailure *traversal.MotionSafetyFailure `json:"motionSafetyFailure,omitempty"`
}

// failWithCode 是 CalibrationManager 新增的结构化错误写入方法（对比旧版 fail 仅写字符串）。
// 签名与遍历模块 failMotionSafety 语义一致：
//
//   func (m *CalibrationManager) failWithCode(err error, code string) error
//
// 行为：设置 StateError + LastError(err.Error()) + LastErrorCode(code)。
// 如果 err 是 *MotionSafetyFailure 包装的错误，同时调用 recordMotionSafetyFailure 写入快照。
```

### `calibration.MotionAxisConfig` → `traversal.MotionAxisBinding` 转换规则

校准模块的 `calibration.MotionAxisConfig` 与遍历模块的 `traversal.MotionAxisBinding` 结构不同：

| 字段 | `calibration.MotionAxisConfig` | `traversal.MotionAxisBinding` |
|---|---|---|
| ControllerID | `string` (json: `controllerId`) | `string` (json: `controllerId`, omitempty) |
| Axis | `string` (json: `axis`)，物理轴名（如 X/Y） | `string` (json: `axis`)，物理轴名 |
| Name | `string` (json: `name`)，逻辑轴名（如 α/β） | —（不存在） |

转换规则（单向：校准 → 遍历）：
- `Axis` 字段直接映射
- `ControllerID` 字段直接映射
- `Name` 字段丢弃（遍历模块的安全判定全部按物理轴名 `Axis` 匹配 `MotionAxisBinding.Axis`）

```go
// toMotionAxisBindings 将校准运动轴配置转换为遍历模块的绑定类型（usecase 内部函数）
func toMotionAxisBindings(axes []calibration.MotionAxisConfig) []traversal.MotionAxisBinding {
    bindings := make([]traversal.MotionAxisBinding, len(axes))
    for i, a := range axes {
        bindings[i] = traversal.MotionAxisBinding{
            ControllerID: a.ControllerID,
            Axis:         a.Axis,
        }
    }
    return bindings
}
```

此转换在 `fallbackRuntime.WaitForMotionComplete` 内部执行，从 `f.targets` 的 key（`calibrationMotionAxis{controllerID, axis}`）重建轴列表后调用。

### `ports.CalibrationRuntime` 接口扩展

`EmergencyStopMotion` 不直接加入 `CalibrationRuntime` 接口（那会无条件破坏所有外部实现，包括 Wails binding 注入的适配器），而是定义为独立的可选扩展接口：

```go
// ports/calibration.go

// CalibrationRuntime 校准运行时端口（既有接口，签名不变）
type CalibrationRuntime interface {
    GetChannelValue(deviceID string, channelIndex int) (float64, bool)
    GetLatestTimestamp(deviceID string) (int64, bool)
    MoveToPosition(axis calibration.MotionAxisConfig, position float64) error
    WaitForMotionComplete() error
    StopMotion() error
}

// EmergencyStopProvider 急停能力提供者（可选扩展接口）。
// 实现 CalibrationRuntime 的类型可同时实现此接口，表示具备控制器级急停能力。
// 调用方通过类型断言判断能力是否存在：
//
//   if es, ok := rt.(ports.EmergencyStopProvider); ok {
//       es.EmergencyStopMotion()
//   }
//
// 不存在时 fallback 到 StopMotion（普通停止），不阻断故障处理流程。
type EmergencyStopProvider interface {
    // EmergencyStopMotion 急停所有参与校准的运动控制器。
    // 与 StopMotion 的差异：
    //
    //   | 维度       | StopMotion                              | EmergencyStopMotion              |
    //   |------------|-----------------------------------------|----------------------------------|
    //   | 作用域     | 单轴逐台（对 Moving=true 的轴调 Stop）   | 控制器级（所有轴瞬时停止）         |
    //   | 停机方式   | 减速停止                                 | 瞬时停止 + 锁存 EmergencyStopped 标志 |
    //   | 恢复方式   | 自动                                     | 需人工复位                         |
    //   | 触发条件   | Deviation / NoProgress / Overshoot       | CriticalDeviation / LimitTriggered / StatusUnavailable |
    //
    EmergencyStopMotion() error
}
```

`fallbackRuntime` 同时实现 `calibration.RuntimeAccess` 和 `ports.EmergencyStopProvider`（内部委托给 `ports.MotionManager.EmergencyStop`）。`runtimeAdapter` 在构造时检查底层 `ports.CalibrationRuntime` 是否也实现了 `EmergencyStopProvider`，若是则透传，否则 `EmergencyStopMotion()` 返回 `nil`（no-op 兜底，此时故障处理 fallback 到 `StopMotion`）。

### `WaitForMotionComplete` 返回值变更

```go
// WaitForMotionComplete 返回值从 error 改为三元组，消除暂停恢复竞态。
//
// 返回值：
//   - completed=true, reason=none, failure=nil：所有参与运动的轴已到位
//   - completed=false, failure!=nil：检测到运动安全故障（调用方应调用 handleMotionSafetyFailure）
//   - completed=false, failure=nil, reason≠none：因暂停/停止/取消/超时中断
//
// 调用方按 reason 分支处理，不再事后读 IsRunning() 推断中断类型。
// reason 类型复用 core/traversal.MotionInterruptReason（导出类型，与 MotionSafetyVerdict 同包定义），
// 确保 motionInterruptReason 在 core/calibration 和 usecase 两个包之间共享类型安全。
// 当前定义在 usecase/traversal_acquisition.go 为未导出类型 motionInterruptReason，
// 需提升到 core/traversal/types.go 为导出类型 MotionInterruptReason 并添加对应常量：

// MotionInterruptReason 运动等待中断原因（core/traversal/types.go 新增）
type MotionInterruptReason int

const (
    MotionInterruptNone      MotionInterruptReason = iota
    MotionInterruptPaused
    MotionInterruptStopped
    MotionInterruptCancelled
    MotionInterruptTimeout
)

// WaitForMotionComplete 签名变更（破坏性，但 CalibrationRuntime 是内部接口，仅 fallbackRuntime 实现）
WaitForMotionComplete() (completed bool, reason traversal.MotionInterruptReason, failure *traversal.MotionSafetyFailure)
```

### 配置合并优先级（与遍历测试一致）

```
默认值 (traversal.DefaultMotionSafety)
  ↓ 覆盖
全局 MotionSafetyConfig (calibration.Config.MotionSafety)
  ↓ 覆盖
按轴 AxisOverrides (MotionSafetyConfig.AxisOverrides[axis])
  ↓ 兜底（仅 arrivalTolerance）
EncoderCompensation.Tolerance（profile 级，向后兼容旧配置）
```

**特殊处理**：`arrivalTolerance` 的合并优先级在 `MotionSafetyConfig` 之后增加 `EncoderCompensation.Tolerance` 兜底，保持旧配置（依赖 EncoderCompensation 的项目）行为兼容。其余 3 个字段（criticalDeviationLimit/noProgressTimeoutMs/progressEpsilon）不读 EncoderCompensation。

## Function Spec

### 1. `evaluateCalibrationMotionSafety`（复用遍历纯函数）

直接调用 `usecase.EvaluateMotionSafety(axis, target, resolved)`，不新建判定函数。校准模块的 `WaitForMotionComplete` 内部循环结构与遍历测试 `waitForMotionComplete` 同构：

```
for ticker {
    1. 优先检查到位（motionTargetsReachedWithTolerance 复用）
    2. 暂停/停止检查 → 返回 reason=paused/stopped
    3. validateMotionStatuses → 掉线/急停/缺轴返回 StatusUnavailable
    4. 每轴 EvaluateMotionSafety → 撞限位/到位/超差/严重偏离
    5. 跨样本看门狗 Observe → 无进展/越过目标
    6. 120s 兜底超时 → 返回 reason=timeout
}
```

### 2. `handleCalibrationMotionSafetyFailure`

与遍历测试 `handleMotionSafetyFailure` 同构：

```go
func (m *CalibrationManager) handleCalibrationMotionSafetyFailure(failure *traversal.MotionSafetyFailure) error {
    // 1. 急停类 verdict → EmergencyStopMotion（失败 fallback StopMotion）
    // 2. 普通停止类 verdict → StopMotion
    // 3. failWithCode 设置 StateError + LastError + LastErrorCode
    // 4. recordMotionSafetyFailure 写入 Status.MotionSafetyFailure 快照
    // 5. 返回包装错误
}
```

### 3. `fallbackRuntime.WaitForMotionComplete` 重构

```go
func (f *fallbackRuntime) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
    // 复用遍历测试 waitForMotionComplete 的循环结构
    // 差异：
    //   - motionAxes 从 f.targets 的 key 重建轴列表（calibration.MotionAxisConfig → traversal.MotionAxisBinding 转换，见 §Data Model 转换规则）
    //   - safetyCfg 从回调传入的 MotionSafetyConfig 读取（fallbackRuntime 不持有 config，由 Manager 在调用时注入或通过 resolveMotionSafetyConfig 延迟获取）
    //   - 暂停检查通过 context 取消/暂停信号 channel 传递（fallbackRuntime 不持有引擎引用）
    //   - 看门狗、statusMissCounter、EvaluateMotionSafety 逻辑完全复用
    //   - 五孔探针顺序运动时，每次 WaitForMotionComplete 创建独立看门狗实例（α/β 轴运动无关）
}
```

### 4. `AutomaticCalibration.Run` 调用点适配

4 种探针的 `Run` 方法调用 `WaitForMotionComplete` 的位置（五孔 `MoveToPointWithOrder` / 其它三种 `MoveToPoint`）改为接收三元组返回值，按 reason 分支：

```go
completed, reason, failure := a.runtime.WaitForMotionComplete()
if failure != nil {
    return a.handleMotionSafetyFailure(failure)  // 引擎层委托 manager 处理
}
if !completed {
    switch reason {
    case traversal.MotionInterruptPaused:
        return nil  // 暂停，不算错误
    case traversal.MotionInterruptStopped, traversal.MotionInterruptCancelled:
        return nil  // 停止/取消，由上层退出
    case traversal.MotionInterruptTimeout:
        return fmt.Errorf("运动超时未完成（>120s）")
    }
}
```

### 5. 前端配置面板

4 种探针的校准配置界面复用 `MotionSafetyPanel.vue` 组件（已在遍历测试实现），挂载到各自的"硬件"配置步骤。`calibration.Config` 的前端类型新增 `motionSafety?: MotionSafetyConfig` 字段。

### 6. 前端故障告警

4 种探针的校准主界面复用遍历测试的 `TraversalLiveMonitor` 运动安全告警卡片模式，在状态栏展示 `motionSafetyFailure` 快照。急停类红色高亮，普通停止类橙色提示。

## Test Plan

### 后端单测（三段式：测试前置 / 测试步骤 / 期待结果）

| 用例 | 前置 | 步骤 | 期待 |
|---|---|---|---|
| `TestCalibration_ResolveDefaultsWhenNil` | `Config.MotionSafety=nil` | Resolve("X") | arrivalTolerance=0.2, critical=5.0, noProgress=2000, epsilon=0.001 |
| `TestCalibration_ResolveAxisOverride` | 全局 arrival=0.5，X 轴覆盖 arrival=0.1 | Resolve("X") | arrival=0.1（轴覆盖优先） |
| `TestCalibration_ResolveEncoderCompensationFallback` | MotionSafety 无 arrival，profile 有 EncoderCompensation.Tolerance=0.05 | Resolve("X") | arrival=0.05（EncoderCompensation 兜底） |
| `TestCalibration_ResolveMotionSafetyOverridesEncoder` | MotionSafety arrival=0.2，profile EncoderCompensation.Tolerance=0.05 | Resolve("X") | arrival=0.2（MotionSafety 优先） |
| `TestCalibration_EvaluateSafety_Arrived` | 轴停，偏差 0.1 ≤ 0.2 | Evaluate | Arrived |
| `TestCalibration_EvaluateSafety_Deviation` | 轴停，偏差 1.0（>0.2 且 <5.0） | Evaluate | Deviation（普通停止） |
| `TestCalibration_EvaluateSafety_CriticalDeviation` | 轴停，偏差 6.0 ≥ 5.0 | Evaluate | CriticalDeviation（急停） |
| `TestCalibration_EvaluateSafety_LimitTriggered` | PosLimit=true | Evaluate | LimitTriggered（急停） |
| `TestCalibration_EvaluateSafety_StatusUnavailable_Offline` | 控制器连续 3 帧掉线 | validateMotionStatuses | StatusUnavailable（急停） |
| `TestCalibration_EvaluateSafety_StatusUnavailable_EmergencyStopped` | 控制器 EmergencyStopped=true | validateMotionStatuses | StatusUnavailable（急停） |
| `TestCalibration_Watchdog_NoProgress` | Moving=true，位置 2s 不变 | Observe | NoProgress（普通停止） |
| `TestCalibration_Watchdog_Overshoot` | Moving=true，位置从 29→31 穿越目标 30 | Observe | Overshoot（普通停止） |
| `TestCalibration_WaitForMotionComplete_PauseRaceFixed` | 运动中暂停，等待函数返回后立即恢复 | 检查 reason | reason=paused（不误判超时） |
| `TestCalibration_HandleFailure_EmergencyStopSuccess` | CriticalDeviation 故障 | handleCalibrationMotionSafetyFailure | EmergencyStopMotion 调用，Status.MotionSafetyFailure 写入 |
| `TestCalibration_HandleFailure_EmergencyStopFallback` | CriticalDeviation + EmergencyStop 失败 | handleCalibrationMotionSafetyFailure | fallback StopMotion，错误码 ErrEmergencyStopFailed |
| `TestCalibration_HandleFailure_NormalStop` | Deviation 故障 | handleCalibrationMotionSafetyFailure | StopMotion 调用（非 EmergencyStop） |
| `TestCalibration_Status_SnapshotClearedOnError` | 非运动安全错误（采集失败） | setErrorLocked | Status.MotionSafetyFailure=nil（避免残留） |
| `TestValidateCalibrationMotionSafety_CrossFieldInverted` | 全局 arrival=10 + X 轴覆盖 critical=5 | validateMotionSafetyConfig | 拒绝（合并后倒置） |
| `TestValidateCalibrationMotionSafety_AxisNotBound` | AxisOverrides 含未绑定轴 "Z" | validateMotionSafetyConfig | 拒绝 |
| `TestCalibration_FiveHole_AxisOrderPreserved` | 五孔探针，α=30 β=15 | MoveToPointWithOrder(["α","β"]) | 先动 α 后动 β，安全判定在每次 WaitForMotionComplete 独立执行 |
| `TestCalibration_Status_StateAfterEmergencyStop` | 运行中触发 CriticalDeviation 急停 | handleCalibrationMotionSafetyFailure | State=Error，IsRunning=false，IsPaused=false |
| `TestCalibration_Status_FailureOverwritten` | 先触发 Deviation 故障，恢复后触发 NoProgress 故障 | 两次 handleCalibrationMotionSafetyFailure | MotionSafetyFailure 被覆盖为最新故障，LastErrorCode 更新 |
| `TestCalibration_Status_FailureClearedOnStart` | 上一轮残留 MotionSafetyFailure | Start 新一轮校准 | MotionSafetyFailure=nil，LastErrorCode="" |

### 前端校验

- `npm run typecheck` 通过
- `npm run build` 通过
- 4 种探针配置界面均能加载 `MotionSafetyPanel`，placeholder 显示默认值
- 4 种探针校准主界面在故障时展示告警卡片

## Risks

| 风险 | 影响 | 缓解 |
|---|---|---|
| `CalibrationRuntime` 可选扩展接口 `EmergencyStopProvider` 未被外部 runtime 实现 | Med | `runtimeAdapter` 通过类型断言检测，不支持时 `EmergencyStopMotion` 返回 nil（no-op），此时严重异常 fallback 到 `StopMotion` |
| `WaitForMotionComplete` 签名变更影响既有调用方 | Med | `AutomaticCalibration` 是唯一调用方，同步更新；测试 mock 同步更新 |
| `EncoderCompensation.Tolerance` 与 `MotionSafetyConfig` 优先级混淆 | Low | Resolve 函数明确分层并加注释；单测覆盖 4 种组合（无/仅有 MotionSafety/仅有 EncoderCompensation/两者都有） |
| 4 种探针运动顺序差异导致安全判定遗漏 | Low | 安全判定在 `WaitForMotionComplete` 内部统一执行，与运动下发顺序解耦；五孔分轴顺序下每次 WaitForMotionComplete 独立判定 |
| 未来引入 checkpoint 机制时旧快照反序列化无 `MotionSafety` 字段 | Low | `*traversal.MotionSafetyConfig` 指针 + `omitempty`，零值时下游使用 DefaultMotionSafety，向后兼容（当前校准模块无 checkpoint 机制，此风险属于预埋设计） |
| 30s→120s 超时变更导致卡死场景响应变慢 | Low | 正常异常由 verdict 主动检测提前触发（<2s），120s 仅兜底；NoProgress 超时默认 2s 远小于 120s |

## Success Criteria

### 功能性

1. **快速失败**：轴停止且未进入 `arrivalTolerance` 时，1 个轮询周期内返回超差；运动中连续无进展达到配置时限后返回无进展错误；越过目标后仍在运动时立即返回越过错误
2. **正常长行程不误停**：轴仍在运动时，距最终目标超过 `criticalDeviationLimit` 不得触发严重偏离；持续有效进展会重置看门狗
3. **急停触发**：轴停止后的偏差达到 `criticalDeviationLimit` 或 `PosLimit/NegLimit` 触发时，`EmergencyStopProvider` 可用时控制器级急停，不可用时 fallback 到 `StopMotion`
4. **错误码区分**：普通超差、越过目标、无进展、严重偏离、撞限位、状态不可用、超时和急停失败均返回独立错误码，前端根据 `LastErrorCode` 显示对应严重级别
5. **配置可调**：`MotionSafetyConfig` 通过前端 `MotionSafetyPanel` 配置并持久化，重启后生效；4 种探针共用同一份配置
6. **按轴覆盖**：通过 `AxisOverrides` 独立设置阈值，未覆盖字段继承全局值
7. **配置拒错**：非法阈值和未绑定轴在 Start 前被拒绝，且不产生运动或文件副作用
8. **WTNMC4A 软限位**：`MoveToPosition` 调用链已内建 `validateWTNMC4ATarget` 校验（被动覆盖），无需新增代码
9. **4 种探针全覆盖**：五孔（分轴顺序）/三孔/总压/总温的 `WaitForMotionComplete` 均接入运动安全判定
10. **状态故障快速失败**：参与控制器掉线、已急停或目标轴状态连续缺失时不等待 120s，进入明确错误态
11. **停机失败可见**：急停不可用或失败时 fallback 到 `StopMotion`，错误信息记录在 `LastError` 和 `LastErrorCode`
12. **向后兼容**：旧配置 JSON（不含 `motionSafety`）加载后使用默认值，不报错不迁移；旧 `CalibrationRuntime` 实现无需修改接口

### 非功能性

13. **性能**：安全判定循环逻辑复用遍历测试实现，单次判定 < 1μs（纯函数），不影响 50ms 轮询周期
14. **可观测**：所有安全失败事件有 `Warn` 或 `Error` 级日志，带 `controllerID`/`axis`/`verdict`/`pointIndex`/`target`/`actual` 字段
15. **可测试**：所有安全判定分支通过纯函数或可控 fake 端口覆盖，不依赖真机和固定 sleep
16. **跨模块一致**：校准模块的运动安全语义与遍历测试完全一致（共用同一套 verdict/配置类型/判定函数）

### 验证清单

- [ ] `go test ./internal/usecase/... -run 'TestCalibration.*MotionSafety|TestCalibration.*WaitForMotionComplete|TestCalibration.*HandleFailure' -v` 全绿
- [ ] `go test ./internal/core/calibration/... -v` 全绿
- [ ] `go build -buildvcs=false ./...` 无错
- [ ] `go vet ./...` 无错
- [ ] `npm run typecheck` 无错
- [ ] `npm run build` 无错
- [ ] 模拟器手动验证：执行大于 5mm 的正常长行程，不误触发急停并正常到位（4 种探针各一次）
- [ ] fake 状态序列验证：注入轴停止未到位场景，校准在 < 200ms 内终止并显示"超差"告警
- [ ] fake 状态序列验证：注入运动中无进展场景，在配置期限 + 1 个轮询周期内终止并显示"运动无进展"告警
- [ ] fake 状态序列验证：注入越过目标场景，立即终止并显示"越过目标"告警
- [ ] fake 状态序列验证：注入撞限位场景，立即急停并显示"急停"告警
- [ ] 4 种探针配置界面均能加载 `MotionSafetyPanel`，placeholder 显示默认值
- [ ] 4 种探针校准主界面在故障时展示告警卡片（急停类红色高亮，普通停止类橙色提示）

## Approval Gate

只有在以下条件满足后才能进入 PLAN：

- [ ] 本规格中 P0-P1 级别问题全部解决并通过复审
- [ ] 用户确认 Success Criteria（16 项）和 Project Structure（文件分布）
- [ ] 用户确认 `MotionInterruptReason` 提升到 `core/traversal/` 的类型方案（方案 B）
- [ ] 用户确认 `EmergencyStopProvider` 可选扩展接口方案
- [ ] 本规格状态由"待审阅"更新为"已批准"

## Out of Scope (后续可选)

- 校准模块的运动安全配置默认值与遍历测试联动（用户改一处生效两处）——需独立 spec
- 校准过程的实时位置展示（当前仅展示压力/插值结果）——需独立 spec
- 校准断点恢复（当前校准无 checkpoint 机制，暂停后只能重跑）——需独立 spec
