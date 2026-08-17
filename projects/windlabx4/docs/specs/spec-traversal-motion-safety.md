# Spec: 遍历测试运动安全防护（位置超差快速失败与急停）

> 来源：运动控制安全机制调研 → spec-driven-development Phase 1 → 规格审查修订
> 日期：2026-07-14
> 状态：待审阅
> 修订：v2
> 关联：[spec-traversal-reliability-and-recovery.md](./spec-traversal-reliability-and-recovery.md)（不修改其不变量，仅扩展运动阶段的安全判定）

## Objective

**目标**：在遍历测试的运动阶段引入位置偏差主动检测与快速失败机制，避免因轴失步、卡死、撞限位或编码器故障导致长时间空跑（当前需等满 120s 超时）或撞机事故。

**用户**：
- 风洞遍历测试操作人员——期望异常发生时立即停机并收到明确告警，而非"卡在某阶段"假象
- 设备维护工程师——期望事故现场可追溯，错误信息能区分"超差"与"撞限位"
- 遍历流程开发人员——期望安全判定逻辑可配置、可测试、可扩展到不同设备

**痛点**（来自代码调研）：

| # | 缺口 | 当前行为 | 风险 |
|---|---|---|---|
| 1 | 到位阈值 `0.01` 硬编码 | [traversal_helpers.go:157](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/windlabx4/services/api-go/internal/usecase/traversal_helpers.go#L157) | 旋转轴/不同精度设备无法适配 |
| 2 | 轴已停但未到位、运动中无进展、越过目标后未停均无主动停机 | 等 120s 超时 | 失步/卡死/越过目标后继续冲，可能撞机 |
| 3 | `stopMotionAxes` 只调轴级 `Stop` | 停止语义取决于适配器 | 严重异常不会锁存急停状态，也不会停止同一控制器的其它轴 |
| 4 | WTNMC4A `MoveTo` 缺少命令下发前的软限位校验 | `moveAxisInit` 仅校验脉冲范围 ±268435455（且超标时静默 clip 而非报错），不读取 `MinLimit/MaxLimit`；`validateWTNMC4APositionSample` 的事后读数校验无法阻止命令下发 | 配置的 `MinLimit/MaxLimit` 形同虚设。B140 的 `validateB140Target` 在 `MoveTo`/`MoveBy` 下发命令前即校验，为正确参照实现 |
| 5 | 遍历层不消费 `PosLimit`/`NegLimit` | 撞限位后等超时 | 撞机后不能快速失败 |
| 6 | 稳定阶段不校验位置 | 只看压力稳定性 | 编码器误报到位后无法发现 |

**不在范围内**：
- 不改变遍历测试的主循环结构（`RunTraversalLoop`、`RunCurrentPoint` 四阶段状态机）
- 不改变三阶段提交协议与 checkpoint 恢复机制
- 不改变 `StabilizationConfig`（压力稳定参数不承载运动安全参数；稳定阶段并行执行位置安全监测）
- 不引入新的运动控制器适配器
- 不改变五孔探针插值算法与压力归一化算法
- 不引入新的第三方依赖

## Tech Stack

- **后端**：Go 1.25 + 标准库 `slog`（日志强制只用标准库）
- **架构**：六边形——`core/traversal`（类型）+ `usecase`（编排）+ `ports`（接口）+ `adapters`（共享 manager 包装）
- **设备 SDK**：`shared/device-sdk/go/motion`（端口与适配器）
- **前端**：Vue 3 + TypeScript + Naive UI + Wails v3 binding
- **测试**：Go 标准 `testing` + simulated 适配器注入故障

## Commands

```powershell
# 后端构建与校验
cd projects\WindLabX4\services\api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...

# 后端测试（含新增的运动安全单测）
cd projects\WindLabX4\services\api-go
go test ./internal/usecase/... -run 'Test.*MotionSafety|TestWaitForMotionComplete|TestEmergencyStop' -v
go test ./internal/... ./api/...

# 前端类型检查与构建
cd projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run build

# Wails binding 同步（若后端方法签名变化）
cd projects\WindLabX4\apps\desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings

# 预提交全套校验
cd <repo-root>
.\validate-structure.ps1
```

## Project Structure

新增与修改的文件分布：

```
projects/windlabx4/
├── services/api-go/internal/
│   ├── core/traversal/
│   │   └── types.go                       # 修改：新增 MotionSafetyConfig、MotionSafetyLevel
│   ├── usecase/
│   │   ├── traversal_helpers.go           # 修改：motionTargetsReached 改用可配置阈值；新增纯函数安全判定
│   │   ├── traversal_acquisition.go       # 修改：运动等待加超差/限位/无进展/状态缺失检查；稳定阶段持续复检
│   │   ├── traversal.go                   # 修改：按参与控制器执行 Stop/EmergencyStop，并处理部分下发失败
│   │   ├── traversal_config.go            # 修改：ParseConfig 解析 MotionSafetyConfig，未提供用默认值
│   │   └── traversal_motion_safety_test.go # 新增：单测覆盖阈值、限位、无进展、掉线、按轴覆盖和停机失败
│   └── ports/
│       └── traversal.go                   # 修改：MotionAccess 新增 EmergencyStop(ctx, controllerID)
├── apps/desktop-wails/frontend/src/
│   ├── views/traversal/
│   │   └── MotionSafetyPanel.vue          # 新增：运动安全配置面板
│   ├── stores/
│   │   └── traversalStore.ts              # 修改：持久化 MotionSafetyConfig
│   └── types/traversal.ts                 # 修改：MotionSafetyConfig TS 类型
shared/device-sdk/go/motion/adapters/hardware/
├── wtnmc4a_motion.go                      # 修改：MoveTo 加有限值与 MinLimit/MaxLimit 校验（参照 B140 现有校验模式）
└── wtnmc4a_motion_test.go                 # 修改：覆盖边界值、越界值、NaN/Inf 和无效限位配置
```

## Code Style

### 配置结构（Go，带可配置默认值与按轴覆盖）

```go
// MotionSafetyConfig 遍历运动安全防护配置。
// 零值字段表示使用 DefaultMotionSafety 中的默认值，保证旧配置无需迁移。
type MotionSafetyConfig struct {
    // ArrivalTolerance 到位容差（mm/°）。|实际位置 - 目标位置| ≤ 此值视为到位。
    // 替换原硬编码 0.01。
    ArrivalTolerance *float64 `json:"arrivalTolerance,omitempty"`

    // CriticalDeviationLimit 严重偏离阈值（mm/°）。超过此值或撞限位时，
    // 锁存 EmergencyStop 并终止遍历，需人工复位。
    // 仅在轴已停止时比较当前位置与目标位置；运动中不得用距终点距离判定。
    CriticalDeviationLimit *float64 `json:"criticalDeviationLimit,omitempty"`

    // NoProgressTimeoutMs 运动中无有效位置进展的最长时间。默认 2000ms。
    NoProgressTimeoutMs *int `json:"noProgressTimeoutMs,omitempty"`

    // ProgressEpsilon 构成有效进展的最小位置变化量（mm/°）。默认 0.001。
    ProgressEpsilon *float64 `json:"progressEpsilon,omitempty"`

    // AxisOverrides 按轴覆盖阈值。键为轴名（X/Y/Z/U）。
    // 旋转轴 U 通常需要更大的容差，可在此覆盖。
    AxisOverrides map[string]MotionSafetyConfig `json:"axisOverrides,omitempty"`
}

// DefaultMotionSafety 默认运动安全配置。
// 阈值基于经验值，应在真机回归后调整。
var DefaultMotionSafety = MotionSafetyConfig{
    ArrivalTolerance:       ptr(0.2),   // mm，兼顾定位精度与机械抖动/背隙
    CriticalDeviationLimit: ptr(5.0),   // mm
    NoProgressTimeoutMs:    ptr(2000),
    ProgressEpsilon:        ptr(0.001), // mm
}

// Resolve 返回解析后的有效配置：按轴覆盖 > 全局配置 > 默认值。
func (c MotionSafetyConfig) Resolve(axis string) MotionSafetyConfig {
    global := DefaultMotionSafety.Merge(c.WithoutAxisOverrides())
    if override, ok := c.AxisOverrides[axis]; ok {
        return global.Merge(override.WithoutAxisOverrides())
    }
    return global
}
```

`MotionSafetyConfig` 作为 `traversal.Config.MotionSafety` 的一部分进入 `TraversalRunSnapshot.Config`。因此 checkpoint 格式不新增平行字段，恢复沿用服务端权威快照，避免运行配置出现两份真相。

配置必须在启动前完成校验，校验失败不得获取工作流锁、创建输出文件或下发运动命令：

- 所有浮点阈值必须为有限数，且 `ArrivalTolerance > 0`、`CriticalDeviationLimit > ArrivalTolerance`、`ProgressEpsilon > 0`
- `NoProgressTimeoutMs >= 2 * motionCompletePoll`，避免单次采样抖动触发故障
- `AxisOverrides` 只允许已绑定轴名，覆盖项不得再包含 `AxisOverrides`，避免递归配置
- 每个轴按自己的工程单位解释阈值；线性轴为 mm，旋转轴为 °，前端必须显示对应单位
- 旧配置不存在 `motionSafety` 时使用默认值；配置一旦进入运行快照，恢复时必须使用 checkpoint 中的值，不重新读取前端当前配置
- 限位开关触发急停是不可关闭的安全不变量，不提供全局或按轴绕过配置

WTNMC4A `MoveTo` 必须在脉冲换算和任何 SDK 调用之前校验：目标为有限值；已配置的 `MinLimit <= MaxLimit`；目标位于闭区间 `[MinLimit, MaxLimit]`。目标恰好等于边界允许执行；任一校验失败返回明确错误且不得调用 `InitLVDV`/`StartLVDV`。只配置单侧限位时仅校验该侧。

### 安全判定函数（纯函数，便于单测）

```go
// MotionSafetyVerdict 安全判定结果。
type MotionSafetyVerdict int

const (
    MotionSafetyOK          MotionSafetyVerdict = iota // 正常运动中或已到位
    MotionSafetyArrived                                // 已到位
    MotionSafetyDeviation                              // 超差（轴已停但未到位）
    MotionSafetyCriticalDeviation                      // 严重偏离
    MotionSafetyLimitTriggered                         // 撞限位
    MotionSafetyNoProgress                             // 运动中持续无有效进展
    MotionSafetyOvershoot                              // 越过目标位置仍在运动
)

// EvaluateMotionSafety 评估单轴当前位置相对目标的安全性。
// 纯函数，不访问硬件，便于单测覆盖所有分支。
func EvaluateMotionSafety(
    axisStatus motion.AxisStatus,
    target float64,
    config MotionSafetyConfig,
) MotionSafetyVerdict {
    resolved := config.Resolve(axisStatus.Name)
    deviation := math.Abs(axisStatus.Position - target)

    // 1. 撞限位优先判定（独立于偏差，因为可能偏差小但限位已触发）
    if axisStatus.PosLimit || axisStatus.NegLimit {
        return MotionSafetyLimitTriggered
    }

    // 2. 运动中只消费限位状态。距最终目标较远是正常运动过程，不能据此急停。
    // 无进展判定需要跨轮询样本，由 waitForMotionComplete 的进展看门狗负责。
    if axisStatus.Moving {
        return MotionSafetyOK
    }

    // 3. 轴停止后再按最终位置偏差分级。
    if deviation <= *resolved.ArrivalTolerance {
        return MotionSafetyArrived
    }
    if deviation >= *resolved.CriticalDeviationLimit {
        return MotionSafetyCriticalDeviation
    }
    return MotionSafetyDeviation
}
```

`DeviationLimit` 不进入 v2 配置。轴一旦报告停止但未进入 `ArrivalTolerance` 就已经不能继续可信采样，应立即普通停止并终止；增加第二个普通超差阈值只会产生"未到位但继续等 120s"的模糊区间。

### 越过目标检测（跨样本侧向判定）

`EvaluateMotionSafety` 只判断单次快照，无法检测"越过目标"。`waitForMotionComplete` 的看门狗同时负责越过检测：

1. 首次观察到 `Moving=true` 时记录当前位置和运动方向（`sign(position - target)`）。
2. 后续轮询中，若运动方向翻转（`sign` 改变）且 `Moving=true` 且 `|position - target| > ArrivalTolerance`，立即返回 `MotionSafetyOvershoot`。
3. 越过检测仅当轴在运动中才有意义；轴已停止的偏差由 `EvaluateMotionSafety` 处理。
4. 越过检测与无进展看门狗共享同一位置快照，不增加额外硬件查询。

### 跨样本进展看门狗

`EvaluateMotionSafety` 只判断单次快照。`waitForMotionComplete` 必须为每个目标轴维护上次有效进展位置与时间：

1. 首次观察到 `Moving=true` 时记录当前位置和当前时间。
2. 相对上次记录位置变化达到该轴 `ProgressEpsilon` 时，更新位置和时间。
3. `Moving=true` 且连续 `NoProgressTimeoutMs` 没有有效进展时返回 `MotionSafetyNoProgress`。
4. 轴停止后立即交给 `EvaluateMotionSafety`，不再等待看门狗超时。
5. 看门狗使用单调时钟；暂停、停止或 context 取消优先于故障判定。
6. 不使用 `PositionError` 作为 v2 的安全输入，除非所有适配器先统一其物理语义和单位。

### `waitForMotionComplete` 集成（轮询循环加并行检查）

```go
func (m *TraversalManager) waitForMotionComplete(
    ctx context.Context, point TraversalPoint, taskID string,
) (completed bool, failure *MotionSafetyFailure) {
    ticker := time.NewTicker(motionCompletePoll)     // 100ms
    defer ticker.Stop()
    deadline := time.Now().Add(motionCompleteTimeout) // 120s 兜底

    for {
        select {
        case <-ctx.Done():
            return false, MotionSafetyOK, ""
        case <-ticker.C:
            statuses := m.motion.StatusAll(ctx)

            // 1. 状态完整性：已绑定控制器掉线、已进入急停或目标轴连续
            // 3 个快照缺失时立即返回明确故障，不能退化为 120s 超时。
            if failure := validateMotionStatuses(statuses, m.motionAxes); failure != nil {
                return false, failure
            }

            // 2. 全部到位？
            if motionTargetsReached(statuses, point, m.motionAxes, m.config.MotionSafety) {
                return true, nil
            }

            // 3. 检查单次快照安全状态，并更新跨样本看门狗（无进展 + 越过目标）。
            for _, status := range statuses {
                if !status.Connected {
                    continue
                }
                targets := availableAxisTargets(status, point, m.motionAxes)
                for _, axis := range status.Axes {
                    target, hasTarget := targets[axis.Name]
                    if !hasTarget {
                        continue
                    }
                    v := EvaluateMotionSafety(axis, target, m.config.MotionSafety)
                    if v != MotionSafetyOK && v != MotionSafetyArrived {
                        return false, newMotionSafetyFailure(status.ID, axis, target, v)
                    }
                    if failure := watchdog.Observe(status.ID, axis, target); failure != nil {
                        // 看门狗返回的故障可能是 NoProgress 或 Overshoot
                        return false, failure
                    }
                }
            }

            // 4. 120s 仅作为未知慢速故障的最后兜底，返回独立 Timeout 错误。
            if time.Now().After(deadline) {
                return false, newMotionTimeoutFailure()
            }
        }
    }
}
```

`MotionSafetyFailure` 至少携带 `controllerID`、`axis`、`verdict`、`target`、`actual` 和 `pointIndex`，避免错误处理阶段重新读取硬件状态导致事故现场被覆盖。

### 错误码与停机策略映射

```go
// 停机策略：根据安全判定结果选择 Stop 或 EmergencyStop。
func (m *TraversalManager) handleMotionSafetyFailure(
    failure MotionSafetyFailure,
) error {
    // 严重偏离、限位、掉线或状态不可用：对所有参与遍历的控制器逐台锁存急停。
    if failure.RequiresEmergencyStop() {
        stopErr := m.emergencyStopMotionControllers(context.Background())
        if stopErr != nil {
            // 急停失败时仍尝试所有参与轴的普通 Stop，并把两类错误聚合返回。
            fallbackErr := m.stopMotionAxes()
            return m.failMotionSafety(failure, traversal.ErrEmergencyStopFailed,
                errors.Join(stopErr, fallbackErr))
        }
        return m.failMotionSafety(failure, errorCodeFor(failure.Verdict), nil)
    }

    // 普通超差或无进展：停止所有参与轴；不得忽略 Stop 下发错误。
    stopErr := m.stopMotionAxes()
    return m.failMotionSafety(failure, errorCodeFor(failure.Verdict), stopErr)
}
```

停机作用域与错误码：

| 判定 | 动作 | 错误码 |
|---|---|---|
| `Deviation` | 所有参与遍历轴执行 `Stop` | `ErrPositionDeviation` |
| `Overshoot` | 所有参与遍历轴执行 `Stop` | `ErrMotionOvershoot` |
| `NoProgress` | 所有参与遍历轴执行 `Stop` | `ErrMotionNoProgress` |
| `CriticalDeviation` | 所有参与控制器执行 `EmergencyStop` | `ErrCriticalPositionDeviation` |
| `LimitTriggered` | 所有参与控制器执行 `EmergencyStop` | `ErrLimitSwitchTriggered` |
| 控制器掉线、目标轴状态缺失或状态已急停 | 对仍可访问的参与控制器执行 `EmergencyStop` | `ErrMotionStatusUnavailable` |
| 任一急停下发失败 | 继续尝试其它控制器，再对参与轴执行 `Stop` 兜底 | `ErrEmergencyStopFailed` |
| 120s 兜底超时 | 所有参与遍历轴执行 `Stop` | `ErrMotionTimeout` |

`MotionAccess` 必须新增 `EmergencyStop(ctx context.Context, id string) error`。`emergencyStopMotionControllers` 从本次运行已解析的绑定快照生成唯一控制器 ID 集合，逐台调用且聚合全部错误；不能因第一台失败而跳过后续控制器。`Stop`/`EmergencyStop` 的物理减速度由适配器决定，规格只保证“轴级普通停止”与“控制器级锁存急停”的语义，不承诺所有硬件具有相同制动曲线。

### 关键约定

- **纯函数优先**：`EvaluateMotionSafety` 不访问硬件，所有故障场景通过 simulated 适配器注入状态值复现
- **错误码区分语义**：普通超差、越过目标、无进展、严重偏离、撞限位、状态不可用、兜底超时和急停失败分别使用独立错误码
- **日志必带上下文**：`axis`、`verdict`、`pointIndex`、`target`、`actual`，便于事后追溯
- **配置零值即默认**：旧配置 JSON 不含 `motionSafety` 字段时使用 `DefaultMotionSafety`，无需迁移脚本
- **稳定阶段持续监测**：运动完成后、整个稳定等待期间以及进入采集前均执行位置/限位/状态检查；任一失败立即中断压力等待并走同一停机策略

## Testing Strategy

### 测试框架

- Go 标准 `testing` + `testify/assert`（项目已用）
- usecase 测试使用可控 fake `MotionAccess` 注入状态序列和停机错误；设备适配器测试只验证其自身状态与命令映射

### 测试位置

```
projects/windlabx4/services/api-go/internal/usecase/
└── traversal_motion_safety_test.go   # 新增
```

### 测试覆盖矩阵

| 测试用例 | 注入场景 | 期望判定 | 期望停机策略 | 期望错误码 |
|---|---|---|---|---|
| `TestEvaluateMotionSafety_Arrived` | 轴停、偏差 0.005mm | `Arrived` | — | — |
| `TestEvaluateMotionSafety_LongMoveDoesNotTrip` | 轴运动中、距目标 100mm | `OK` | — | — |
| `TestEvaluateMotionSafety_Deviation_Stopped` | 轴停、偏差 0.5mm（超 0.01 容差） | `Deviation` | `Stop` | `ErrPositionDeviation` |
| `TestEvaluateMotionSafety_CriticalDeviation` | 轴停、偏差 6mm | `CriticalDeviation` | `EmergencyStop` | `ErrCriticalPositionDeviation` |
| `TestEvaluateMotionSafety_LimitTriggered` | `PosLimit=true`、偏差 0.005mm | `LimitTriggered` | `EmergencyStop` | `ErrLimitSwitchTriggered` |
| `TestEvaluateMotionSafety_AxisOverride` | U 轴停止、偏差 0.3°、轴级 `arrivalTolerance=0.5°` | `Arrived` | — | — |
| `TestResolve_AxisOverrideInheritsGlobal` | 全局无进展时限 3000ms，U 仅覆盖到位容差 | U 继承全局 3000ms | — | — |
| `TestWaitForMotionComplete_AllAxesArrived` | 全轴到位 | `completed=true` | — | — |
| `TestWaitForMotionComplete_DeviationFailsFast` | 单轴停且偏差 0.5mm，立即返回 | `completed=false, verdict=Deviation` | `Stop` | `ErrPositionDeviation` |
| `TestWaitForMotionComplete_NoProgress` | `Moving=true`，位置超过 2s 不变 | `verdict=NoProgress` | `Stop` | `ErrMotionNoProgress` |
| `TestWaitForMotionComplete_ProgressResetsWatchdog` | 每次在期限内移动超过 epsilon | 继续等待，不误报 | — | — |
| `TestWaitForMotionComplete_Overshoot` | 目标 30，位置从 29.5 穿越到 31.0，`Moving=true` | `verdict=Overshoot` | `Stop` | `ErrMotionOvershoot` |
| `TestWaitForMotionComplete_OvershootNotTriggeredOnApproach` | 目标 30，位置从 15 到 20，未穿越目标 | 继续等待 | — | — |
| `TestWaitForMotionComplete_LimitFailsFast` | 单轴 `PosLimit=true`，立即返回 | `verdict=LimitTriggered` | `EmergencyStop` | `ErrLimitSwitchTriggered` |
| `TestWaitForMotionComplete_Disconnected` | 已绑定控制器从状态中变为离线 | 立即失败 | `EmergencyStop` | `ErrMotionStatusUnavailable` |
| `TestWaitForMotionComplete_MissingAxisDebounced` | 目标轴连续 3 个快照缺失 | 失败，单次缺失不误报 | `EmergencyStop` | `ErrMotionStatusUnavailable` |
| `TestEmergencyStop_AllControllersAttempted` | 两台参与控制器，第一台急停失败 | 第二台仍收到急停，随后轴级 Stop 兜底 | `EmergencyStop` + `Stop` | `ErrEmergencyStopFailed` |
| `TestRunCurrentPoint_StabilizationContinuousRecheck` | 稳定等待开始后位置漂移 2mm | 中断稳定等待并终止遍历 | `Stop` | `ErrPositionDeviation` |
| `TestParseConfig_MotionSafetyDefault` | 配置 JSON 不含 `motionSafety` | 使用 `DefaultMotionSafety` | — | — |
| `TestParseConfig_MotionSafetyAxisOverride` | 配置含 U 轴覆盖 | `Resolve("U")` 返回覆盖值 | — | — |
| `TestParseConfig_MotionSafetyRejectsInvalidValues` | 负数、NaN、Inf、阈值倒置、未知轴或递归覆盖 | Start 前拒绝且无设备/文件副作用 | — | 配置错误码 |
| `TestResume_UsesSnapshottedMotionSafety` | checkpoint 阈值与当前前端配置不同 | 使用 checkpoint 阈值 | — | — |
| `TestWTNMC4AMoveTo_SoftLimitValidation` | 边界、越界、NaN/Inf、单侧限位、`MinLimit > MaxLimit` | 合法边界下发；其它拒绝且 SDK 调用数为 0 | — | 设备错误 |

### 测试用例格式（遵循 memory §20）

每个测试用例分三段：

```
// 测试前置：构造 MotionSafetyConfig，arrivalTolerance=0.01, criticalDeviationLimit=5.0
// 测试步骤：调用 EvaluateMotionSafety，传入 axis.Moving=false, axis.Position=10.5, target=10.0
// 期待结果：返回 MotionSafetyDeviation（偏差 0.5mm，超 arrivalTolerance 且未达到 criticalDeviationLimit）
```

### 覆盖率目标

- `EvaluateMotionSafety`：100% 分支覆盖
- `waitForMotionComplete`：≥ 90%（ctx 取消、超时、到位、超差、限位、无进展、越过目标、掉线、状态缺失、按轴覆盖路径）
- `handleMotionSafetyFailure`：100%（Stop、EmergencyStop、部分急停失败与 Stop 兜底路径）

### 不在测试范围

- 真机回归——由 HIL 验证清单 `docs/runbooks/hil-validation-plan.md` 单独覆盖
- WTNMC4A 软限位校验的真机验证——开发阶段仅单测，发布前做 HIL

## Boundaries

### Always do

- 改动 `core/traversal/types.go` 后必须更新前端 TS 类型并跑 `npm run typecheck`
- 新增/修改后端方法签名后必须跑 `wails3 generate bindings` 同步 TS binding
- 任何安全判定逻辑变更必须有对应单测，且单测先于实现（TDD）
- `EmergencyStop` 调用必须有 `Warn` 级日志，包含控制器 ID、轴名、判定结果、点位索引、目标和实际位置
- 提交前跑 `validate-structure.ps1` + `go test ./...` + `npm run typecheck` + `npm run build` 全绿
- 阈值常量集中定义在 `core/traversal/types.go` 的 `DefaultMotionSafety`，禁止散落硬编码

### Ask first

- 修改 `RunCurrentPoint` 四阶段状态机的阶段顺序
- 修改三阶段提交协议（CSV → JSONL → Checkpoint）
- 修改 `motionCompleteTimeout`（120s）默认值
- 新增本规格已批准范围以外的 `MotionController` 接口方法（影响所有适配器）
- 修改 simulated 适配器的故障注入接口（影响既有测试）

### Never do

- 在 `core/traversal` 中导入硬件相关包（六边形硬约束）
- 在前端实现位置判定算法（前端零硬件访问，零校准/遍历算法）
- 在 `stopMotionAxes` 中默认调用 `EmergencyStop`（急停仅限严重异常，需人工复位）
- 删除既有测试用例以"修复"失败（memory §32：遍历测试前置条件检查的反面案例）
- 在 `EvaluateMotionSafety` 中访问硬件或调用端口（必须保持纯函数）
- 提交未跑 `go vet` 的代码

## Success Criteria

### 功能性

1. **快速失败**：轴停止且未进入 `arrivalTolerance` 时，1 个轮询周期内返回超差；运动中连续无进展达到配置时限后返回无进展错误；越过目标后仍在运动时立即返回越过错误
2. **正常长行程不误停**：轴仍在运动时，距最终目标超过 `criticalDeviationLimit` 不得触发严重偏离；持续有效进展会重置看门狗
3. **急停触发**：轴停止后的偏差达到 `criticalDeviationLimit` 或 `PosLimit/NegLimit` 触发时，所有参与控制器在 1 个轮询周期内收到 `EmergencyStop`
4. **错误码区分**：普通超差、越过目标、无进展、严重偏离、撞限位、状态不可用、超时和急停失败均返回独立错误码，并在前端显示对应严重级别
5. **配置可调**：`MotionSafetyConfig` 通过前端配置界面修改并持久化，重启后生效；恢复任务使用 checkpoint 快照值
6. **按轴覆盖**：U 旋转轴可通过 `AxisOverrides["U"]` 独立设置阈值，未覆盖字段继承全局值且不影响 X/Y/Z
7. **配置拒错**：非法阈值、未知轴和递归覆盖在 Start 前被拒绝，且不产生运动或文件副作用
8. **WTNMC4A 软限位**：`MoveTo` 拒绝非有限目标、无效限位配置和闭区间外目标，且不调用运动 SDK；边界值和合法单侧限位正常工作
9. **稳定阶段持续复检**：运动完成后、整个稳定等待期间和采集前持续校验位置、限位和状态，异常立即中断
10. **状态故障快速失败**：参与控制器掉线、已急停或目标轴状态连续缺失时不等待 120s，进入明确错误态并执行停机策略
11. **停机失败可见**：急停逐台尝试；任一失败仍继续停止其它控制器并执行轴级 Stop 兜底，最终返回 `ErrEmergencyStopFailed`
12. **向后兼容**：旧配置 JSON（不含 `motionSafety`）加载后使用默认值，不报错不迁移

### 非功能性

13. **性能**：`EvaluateMotionSafety` 单次调用 < 1μs（纯函数，无 I/O），不影响 100ms 轮询周期
14. **可观测**：所有安全失败事件有 `Warn` 或 `Error` 级日志，带 `controllerID`/`axis`/`verdict`/`pointIndex`/`target`/`actual` 字段
15. **可测试**：所有安全判定分支通过纯函数或可控 fake 端口覆盖，不依赖真机和固定 sleep
16. **不回退**：现有 `spec-traversal-reliability-and-recovery.md` 的全部不变量（I1-I8）保持成立，安全失败不得推进 `commitSeq`

### 验证清单

- [ ] `go test ./internal/usecase/... -run 'Test.*MotionSafety|TestWaitForMotionComplete|TestEmergencyStop' -v` 全绿
- [ ] `go test ./internal/usecase/... -run TestWaitForMotionComplete -v` 全绿
- [ ] `go test ./internal/usecase/... -run TestRunCurrentPoint -v` 全绿
- [ ] `go test ./internal/usecase/... -run TestParseConfig -v` 全绿
- [ ] `go build -buildvcs=false ./...` 无错
- [ ] `go vet ./...` 无错
- [ ] `npm run typecheck` 无错
- [ ] `npm run build` 无错
- [ ] 模拟器手动验证：执行大于 5mm 的正常长行程，不误触发急停并正常到位
- [ ] fake 状态序列验证：注入轴停止未到位场景，遍历在 < 200ms 内终止并显示"超差"告警
- [ ] fake 状态序列验证：注入运动中无进展场景，在配置期限 + 1 个轮询周期内终止并显示"运动无进展"告警
- [ ] fake 状态序列验证：注入越过目标场景（目标 30，位置穿越到 31 且 Moving），立即终止并显示"越过目标"告警
- [ ] 模拟器手动验证：注入撞限位场景，遍历立即急停并显示"急停"告警

## Confirmed Decisions

1. 不设置 `DeviationLimit`；轴停止但未进入到位容差即终止，严重程度由 `CriticalDeviationLimit` 区分。
2. 运动中不得用距最终目标的距离判断严重偏离；v2 使用无进展看门狗识别卡死。
3. 线性轴阈值单位为 mm，旋转轴阈值单位为 °；全局默认值按轴工程单位解释，前端明确显示单位并允许按轴覆盖。
4. 不提供“忽略并继续”。运动安全失败进入 `error`，不推进点位提交水位；人工确认安全并复位后再恢复任务。
5. WTNMC4A 软限位失败返回错误且不发送命令，不自动 clamp。
6. 急停后任务沿用 `error` 状态，通过独立错误码区分原因，不新增公开状态。
7. 安全配置放在遍历配置页的运动绑定区域，并随任务配置进入运行快照。
8. 默认阈值是上线前 HIL 调参的起点，不是设备认证值；发布门禁必须完成各轴精度、编码器分辨率、正常最慢速度和制动行为验证。
9. 限位开关触发急停不可由任务配置关闭；需要检修旁路时必须离开遍历工作流并使用设备维护流程。
10. 越过目标检测通过跨样本方向翻转判定；运动中穿越目标位置且偏差 > `ArrivalTolerance` 时立即触发 Stop，防止继续冲出撞机。

## Open Questions

无。默认阈值的真机定值属于 HIL 发布门禁，不阻塞进入 PLAN，但未完成 HIL 不得发布到生产设备。

## Approval Gate

只有在以下条件满足后才能进入 PLAN：

- 用户确认 v2 的安全判定、停机作用域、错误码、配置模型和 Success Criteria。
- 本规格状态由“待审阅”更新为“已批准”。
