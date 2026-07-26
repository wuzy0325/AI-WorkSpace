# Code Review — WTNMC4A 运动控制器 FFI 与 motion 适配器

> **审查日期**：2026-07-14
> **审查范围**：未提交的改动（`git diff` 工作区）
> **关联规格**：[spec-traversal-motion-safety.md](../specs/spec-traversal-motion-safety.md)
> **状态**：审查完成，待修复

## 变更概览

| 文件 | 类型 | 说明 |
|---|---|---|
| `shared/device-sdk/go/ffi/wtnmc4a.go` | 修改 | `RR1Status` 解析从 4 字节位掩码改为 64 字节结构体；新增 `wtnmc4aRR1Raw`；检查 DLL 返回值 |
| `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go` | 修改 | 引入软限位校验、可信位置采样、`ioMu` 串行化、`stateVersion` 乐观并发控制 |
| `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_bench_test.go` | 修改 | 新增只读稳定性测试 |
| `shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion_test.go` | 新增 | 单测覆盖位置校验、软限位、并发串行化、停机失败处理 |
| `shared/device-sdk/go/ffi/wtnmc4a_test.go` | 新增 | 验证 `wtnmc4aRR1Raw` 内存布局大小为 64 字节 |
| `projects/wind-daq/docs/specs/spec-traversal-motion-safety.md` | 新增 | 遍历运动安全防护规格文档 |

**核心改动主题**：

- 软限位校验（`validateWTNMC4ATarget`）：在 `MoveTo`/`MoveBy`/`DefinePosition` 下发 SDK 命令前校验 `MinLimit`/`MaxLimit` 与有限值
- 可信位置采样（`readTrustedPosition` + `validateWTNMC4APositionSample`）：拒绝 LP 寄存器极值、超物理速度跳变，要求首次位置需双次确认
- DLL 调用串行化（`ioMu`）：`Stop`/`EmergencyStop` 与运动命令互斥，避免 DLL 句柄竞争
- `stateVersion` 乐观并发控制：`Status()` 长时间读 DLL 时若状态被其他写者修改，放弃写入
- RR1 结构体修正：从 `[4]byte` 位掩码改为 64 字节 `wtnmc4aRR1Struct`，避免 DLL 越界写栈

## 构建验证

| 命令 | 结果 |
|---|---|
| `go vet ./...`（在 `shared/device-sdk/go`） | ✓ 通过 |
| `go build ./...`（在 `shared/device-sdk/go`） | ✓ 通过 |

---

## Critical（confidence 75-100）

### 1. `EmergencyStop` 部分失败时不锁存 `EmergencyStopped` 状态 — 安全漏洞

- **位置**：[wtnmc4a_motion.go:847-880](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L862-870)
- **问题**：4 轴中任一 `stopAxis` 失败时，函数直接 `return err`，**未执行** `c.status.EmergencyStopped = true`。这意味着：
  - 部分轴已急停、部分轴未急停时，系统不进入"急停"状态
  - [checkReadyLocked:950-958](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L950-958) 不会拒绝后续 `MoveTo`/`MoveBy`/`Jog` 调用
  - 用户/上层可在部分轴已急停的情况下继续下发运动命令 — 撞机风险
- **spec 要求**（[spec-traversal-motion-safety.md:318](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/docs/specs/spec-traversal-motion-safety.md#L318)）："急停逐台尝试；任一失败仍继续停止其它控制器并执行轴级 Stop 兜底" — 即"失败也锁存"
- **建议**：先逐轴尝试并聚合错误，**无条件锁存** `EmergencyStopped=true` 与 `Moving=false`，再返回聚合错误：

```go
var stopErrors []error
for _, an := range []int{0, 1, 2, 3} {
    if err := stopAxis(handle, an); err != nil {
        stopErrors = append(stopErrors, fmt.Errorf("轴%d急停失败: %w", an, err))
    }
}
c.mu.Lock()
c.status.EmergencyStopped = true   // 无条件锁存，避免部分失败时绕过 checkReadyLocked
c.status.LastError = ""
for i := range c.status.Axes {
    c.status.Axes[i].Moving = false
}
c.stateVersion++
c.mu.Unlock()
return errors.Join(stopErrors...)
```

### 2. `Stop("")` 部分失败时不清除已成功停止轴的 `Moving` 标志

- **位置**：[wtnmc4a_motion.go:811-828](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L811-820)
- **问题**：与 #1 同样的模式 — 任一轴失败则整批不更新 `Moving`。成功停止的轴仍显示 `Moving=true`，UI 状态与硬件不一致。单轴 `Stop(axis)` 路径正确（失败时不清除，符合 `TestWTNMC4AStopFailureDoesNotClaimAxisStopped`），但全轴路径语义不同。
- **建议**：按轴单独更新，最后聚合错误：

```go
if axis == "" {
    var stopErrors []error
    c.mu.Lock()
    for _, an := range []int{0, 1, 2, 3} {
        if err := stopAxis(handle, an); err != nil {
            stopErrors = append(stopErrors, fmt.Errorf("轴%d停止失败: %w", an, err))
            continue
        }
        // 成功的轴才清除 Moving
        for i := range c.status.Axes {
            if wtnmc4aAxisNum(c.status.Axes[i].Name) == an {
                c.status.Axes[i].Moving = false
            }
        }
    }
    if len(stopErrors) == 0 {
        c.status.LastError = ""
    }
    c.stateVersion++
    c.mu.Unlock()
    return errors.Join(stopErrors...)
}
```

### 3. `validateWTNMC4APositionSample` 把 `±wtnmc4aMaxMovePulse` 当作"无效边界值" — 拒绝合法位置

- **位置**：[wtnmc4a_motion.go:1170](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L1170)
- **问题**：

```go
if pulse == math.MinInt32 || pulse == math.MaxInt32 || pulse == -wtnmc4aMaxMovePulse || pulse == wtnmc4aMaxMovePulse {
    return fmt.Errorf("脉冲值 %d 为无效边界值", pulse)
}
```

  - `wtnmc4aMaxMovePulse = 268435455`（[L33](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L33)）是**单次移动**的硬件上限，不是 LP 寄存器的极值
  - LP 寄存器是 int32（范围 ±2147483647），`268435455` 完全在合法范围内
  - 注释说"Exact extrema commonly indicate a failed native call" — extrema 应仅指 `math.MinInt32`/`math.MaxInt32`
  - `TestWTNMC4APositionSampleAllowsFullLogicalRegisterRange`（[L45-50](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion_test.go#L45-50)）用 `300_000_000` 通过校验，但 `268435455` 会被拒 — 实际设备完全可能停在该位置（多次累加移动后）
- **建议**：仅保留真正的寄存器极值检查，删除 `±wtnmc4aMaxMovePulse` 分支：

```go
if pulse == math.MinInt32 || pulse == math.MaxInt32 {
    return fmt.Errorf("脉冲值 %d 为 LP 寄存器极值，疑似 DLL 调用失败", pulse)
}
```

如果团队确实观察到 DLL 在失败时返回 `268435455`，应单独定义为 `lpFailureSentinel` 并在注释中给出厂商文档依据，不要复用 `wtnmc4aMaxMovePulse` 常量。

---

## Important（confidence 50-74）

### 4. `MoveBy` 未使用 `c.startMove` 测试缝，与 `MoveTo` 不一致

- **位置**：[wtnmc4a_motion.go:676](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L676)
- **问题**：`MoveTo`（[L631-635](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L631-635)）通过 `startMove` 缝支持测试注入；`MoveBy` 直接调用 `c.moveAxisInit`，无法在单测中模拟 `moveAxisInit` 失败（如测试 `MoveBy` 在运动命令下发失败时是否正确回滚 `Moving` 标志）。Spec 测试矩阵（[L366](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/docs/specs/spec-traversal-motion-safety.md#L366)）要求覆盖 `MoveTo_SoftLimitValidation`，但 `MoveBy` 同样需要等价覆盖。
- **建议**：抽取共用启动函数，`MoveBy` 也走 `startMove` 缝：

```go
startMove := c.startMove
if startMove == nil {
    startMove = c.moveAxisInit
}
if err := startMove(an, int32(deltaPulse)); err != nil { ... }
```

### 5. `moveAxisInit` 仍静默 clip 超标脉冲 — 与 spec 决策矛盾

- **位置**：[wtnmc4a_motion.go:540-544](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L540-544)
- **问题**：

```go
if absPulse > 268435455 {
    slog.Warn("WTNMC4A moveAxisInit pulse clipped to hardware max", ...)
    absPulse = 268435455
}
```

  Spec 明确决策（[L462](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/docs/specs/spec-traversal-motion-safety.md#L462)）："WTNMC4A 软限位失败返回错误且不发送命令，**不自动 clamp**"。虽然 `MoveTo`/`MoveBy` 上游已加 `deltaPulse > wtnmc4aMaxMovePulse` 校验（[L627](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L627)、[L672](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L672)），生产路径下 clip 不可达，但 `moveAxisInit` 是公开方法（且通过 `startMove` 缝暴露给测试），保留 silent clip 行为会让未来直接调用者意外触发。
- **建议**：将 clip 改为返回错误，由 `MoveTo`/`MoveBy` 已有的 `deltaPulse` 范围检查兜底：

```go
if absPulse > wtnmc4aMaxMovePulse {
    return fmt.Errorf("WTNMC4A 轴 %d 目标脉冲 %d 超出硬件上限 %d", an, absPulse, wtnmc4aMaxMovePulse)
}
```

### 6. `wtnmc4aRR1Struct` 与 `wtnmc4aRR1Raw` 是同一 C 结构体的两份定义

- **位置**：[wtnmc4a_motion.go:70-87](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L70-87) 与 [ffi/wtnmc4a.go:205-211](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/ffi/wtnmc4a.go#L205-211)
- **问题**：两份独立定义，字段顺序、类型完全相同。若未来厂商 SDK 更新结构体（如增加字段），需同步两处，易漏。
- **建议**：motion adapter 复用 ffi 包的定义，或抽取到 ffi 包的 `wtnmc4a_types.go` 中供两处引用。

### 7. `Status()` 在 RR1 读取失败后仍更新 `lastFullStatusAt`

- **位置**：[wtnmc4a_motion.go:507-509](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L507-509)
- **问题**：

```go
if needFullStatus {
    c.lastFullStatusAt = time.Now()  // 即使 RR1 失败也更新
}
```

  RR1 失败后，后续 500ms 内的 `Status` 调用走快速路径（不读 RR1），缓存中的 `PosLimit`/`NegLimit`/`Moving` 等慢变信号保持陈旧。若 RR1 失败是因为控制器掉线前兆，限位/报警状态将延迟 500ms+ 才被刷新 — 与 spec 的"快速失败"目标相悖。
- **建议**：仅在 RR1 全部成功时刷新时间戳：

```go
if needFullStatus {
    rr1Failed := false
    for _, r := range results {
        if !r.statusValid {
            rr1Failed = true
            break
        }
    }
    if !rr1Failed {
        c.lastFullStatusAt = time.Now()
    }
}
```

---

## Notes（confidence 25-49）

### 8. `WTNMC4AGetRR1Status`（ffi 包）吞掉 DLL 失败

- **位置**：[ffi/wtnmc4a.go:213-218](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/ffi/wtnmc4a.go#L213-218)
- **观察**：`ret == 0` 时返回零值 `RR1Status{}`，调用方无法区分"全部 false"与"调用失败"。当前 production 路径不调用此函数（motion adapter 自己调用 `procs.getRR1` 并检查返回值），但函数作为公开 API 保留误导性。建议改为返回 `(RR1Status, error)` 或删除。

### 9. `readTrustedPosition` 的 `initialCandidate` 替换逻辑可接受运动中读数作为初始可信位置

- **位置**：[wtnmc4a_motion.go:1135-1149](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L1135-1149)
- **观察**：首次确认时，若第二次读数与第一次"独立合法但互不一致"，代码用第二次读数替换 `initialCandidate` 并继续等第三次。如果轴正在运动（即使本不应运动），每个读数都"独立合法"，最终会用最后一次读数作为可信位置。`previous.at.IsZero()` 跳过跳变检查的设计在初始化阶段是合理的，但建议在 `initialCandidate != nil` 但被替换时打 Warn 日志，便于事后排查。

### 10. `DefinePosition` 中 `setLP` 成功但 `setEP` 失败时，LP/EP 状态不一致（pre-existing）

- **位置**：[wtnmc4a_motion.go:920-931](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L920-931)
- **观察**：本次 diff 在成功路径上增加了 `c.trustedPositions[an]` 更新，但未处理 `setEP` 失败时 `setLP` 已生效的回滚问题。属 pre-existing 问题，spec 也未要求修复，仅作记录。

---

## Compliance 维度

| 维度 | 结论 | 说明 |
|---|---|---|
| 六边形约束 | ✓ | `core/` 无硬件导入，`adapters/hardware/` 仅做协议翻译 + I/O |
| 设备抽象层 | ✓ | 通过 `ports.MotionController` 接口暴露，适配器内部细节封装 |
| 每设备独立线程 | ✓ | `ioMu` 提供 DLL 调用串行化，`Status` 在轴间释放锁避免长阻塞 |
| 中文注释 | ✓ | 充分且解释"为什么"而非"做了什么" |
| 测试三段式格式 | ⚠ | spec 文档已规范，但 `wtnmc4a_motion_test.go` 中的测试未带三段式注释 — 与 memory §20 约束有出入 |
| 状态栏错误传播 | ✓ | `Status()` 通过 `errors.Join` 聚合并写入 `c.status.LastError` |
| spec 一致性 | ⚠ | Critical #1/#2/#3 与 spec 的 "快速失败"和"不自动 clamp"决策相悖 |

---

## 修复优先级建议

| 优先级 | 问题 | 修复成本 |
|---|---|---|
| P0 | #1 EmergencyStop 部分失败不锁存 | 5 行修改 + 1 个测试用例 |
| P0 | #2 Stop("") 部分失败不清除 Moving | 10 行重构 + 1 个测试用例 |
| P1 | #3 `±wtnmc4aMaxMovePulse` 误判为 sentinel | 1 行删除 + 调整测试 |
| P1 | #5 `moveAxisInit` silent clip | 3 行修改 |
| P2 | #4 MoveBy 缺 startMove 缝 | 5 行抽取 |
| P2 | #7 `lastFullStatusAt` 失败时仍刷新 | 8 行修改 |
| P3 | #6 RR1Struct 重复定义 | 跨包重构 |
| P3 | #8-10 | 记录或后续处理 |

---

## 后续动作

1. 修复 P0/P1 后重新跑 `go vet ./...` + `go test ./...`（在 `shared/device-sdk/go`）
2. 若 motion adapter 公开方法签名未变，无需重新生成 Wails binding
3. 修复完成后，本审查文档归档到 `docs/reviews/archive/` 或保留作为发布门禁记录
