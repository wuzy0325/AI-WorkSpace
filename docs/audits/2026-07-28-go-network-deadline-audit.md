# Go 网络 Deadline 工作空间审计

## Scope

- 日期：2026-07-28
- 范围：工作空间内全部非测试 Go 代码的 `SetDeadline`、`SetReadDeadline`、`SetWriteDeadline` 调用；测试代码仅用于判断是否覆盖 deadline 失效模式。
- 现场约束：一台 Windows 电脑可重复出现 deadline 到期后阻塞网络读取仍不返回。
- 判定标准：如果没有独立 goroutine/timer 能绕过阻塞 I/O 及其 mutex 调用 `Close()`，则 deadline 是唯一退出机制，判为风险。

## Summary

审计覆盖 `shared/`、`projects/` 和 `programs/`。风险不是 deadline API 本身的使用语法，而是连接生命周期设计：大量路径设置 deadline 后在同一 goroutine 中直接 `Read`，而 `defer conn.Close()`、错误清理或 `Disconnect()` 只有等读取返回后才能执行。

已确认安全范式：

- DAQ-P1604 TCP Connect：外层 watchdog 可直接关闭握手连接；握手协议读取使用 `timeout=0`，不依赖 per-read deadline。
- DAQ-P1604 UDP scanner：deadline 之外有 `time.AfterFunc(timeout, conn.Close)`。

本次立即修复：

- DAQ-T1603 UDP scanner 增加独立 Close watchdog。
- WindLabX4 `NetworkScanner.scanWithSocket` 增加独立 Close watchdog。
- 两处均增加“忽略 deadline、只响应 Close”的回归测试。
- 两套 DAQ-P1604 `StopAcquisition` 在 reader join 超时后立即关闭并废弃连接，不再发送 `c 02` 或启动第二个 reader；回归测试断言服务端未收到任何停止命令字节。

## Risk Register

### P0: 可阻塞连接、停止或断开

| 组件 | 风险路径 | 原因 | 所需整改 |
|---|---|---|---|
| Shared DAQ-T1603 | `ConsumeOptionalACK`, `SendCommand`, `SendCommandIdle`, `SendCommandExact` | Connect/配置持锁执行阻塞 I/O，Disconnect 无法先 Close | Connect/命令级 watchdog 直接关闭捕获的连接；超时后重连 |
| Shared DAQ-T1603 | `drainConnection`, `readLoop`, `stopAcquisitionLocked` | stop channel 不解除 Read；Disconnect 在 Close 前执行 stop/drain | join 超时立即 Close，禁止在失效连接上继续 drain/stop |
| B140 motion | `sendCommand` | I/O goroutine 阻塞后取消分支仍等待 `done`；Close 需要同一 `connMu` | watchdog 绕过 `connMu` 关闭捕获连接，取消后禁止无界等待 |
| DAQ-P1604 desktop | `zeroCalibrationDirect`, `idleReadLoop` | 校准/idle reader 只靠 deadline；idle join 超时后仍可能继续使用同一连接 | join/校准超时 Close 并标记断线，后续必须重连 |
| WindLabX4 DSA3217 | `readLoop`, `sendCommand`, `Disconnect` | `ioMu` 覆盖阻塞读写，Disconnect 先发 STOP 后 Close | Close 必须可绕过 `ioMu`；无法有序停止时直接废弃连接 |
| WindLabX4 P1064Pre | `sendCommand`, `readLoop`, start write | deadline 是唯一边界，且命令 reader 可能与采集 reader 竞争 | 单一 reader 所有权；命令 watchdog；停止后 join 或 Close |
| WindLabX4 WTN-PXI | `readLoop`, `StopAcquisition` | stop 只关闭 channel，不 join 或 Close | 停止时 Close/reconnect，或统一 connection-owner 取消模型 |

### P1: 通用 helper 依赖调用方，但契约不够强

| 文件/符号 | 风险 | 处置方向 |
|---|---|---|
| `protocol.SendCommandNoNewline` | write deadline 是唯一边界 | 仅在有外层连接 watchdog 的操作中使用，或增加明确 owner API |
| `protocol.DrainConnection` | 单次 Read 可永久阻塞，循环次数无效 | 外层 watchdog；触发后废弃连接 |
| `protocol.P1604ReadCommandACK` | `timeout>0` 仍只依赖 deadline；`timeout=0` 要求调用方 Close | 保持 owner 契约；所有生产调用点必须证明有 Close 路径 |
| `protocol.P1604ReadUnitCoefficient` | 同上，包含 Write + ReadFrame | Connect 使用现有外层 watchdog；运行期调用需补 owner watchdog |
| `protocol.P1604WriteUnitCoefficient` | 读写均只靠 deadline | 单位切换操作超时应关闭并重连，不复用连接 |

### P2: 开发诊断工具可永久挂起

- `programs/p1604-unit-diag`: `probeNewlineVariants`, `writeCmd`, `queryCmd`, `readPressures`, `drainW1601`。
- `programs/p1604-ts-diag`: 数据帧读取循环。
- `projects/daq-t1603/.../cmd/freqprobe`: frame read、write、drain。
- `projects/daq-t1603/.../cmd/frameprobe`: command read/write、exact read、idle read、drain。

这些不影响生产应用，但现场诊断时同样可能永久挂起。应增加进程级或连接级硬 watchdog；触发后退出，不再尝试复用连接。

## Simulator Review

WindLabX4 TCP simulator 的 command/read loops 依赖 deadline，但存在显式 `Close`。当前 `Start(ctx)` 未把 `ctx.Done()` 接到 Close，且连接 goroutine 未统一 join，判为待审而非生产 P0。建议将 context cancellation 与 simulator Close 绑定，并纳入 wait group。

## Required Test Pattern

```go
type deadlineIgnoringConn struct {
    closed chan struct{}
}

func (c *deadlineIgnoringConn) SetReadDeadline(time.Time) error { return nil }

func (c *deadlineIgnoringConn) Read([]byte) (int, error) {
    <-c.closed
    return 0, net.ErrClosed
}

func (c *deadlineIgnoringConn) Close() error {
    close(c.closed)
    return nil
}
```

测试必须断言被测操作在预算内返回，并确认返回由独立 `Close` 触发。只断言普通 deadline 返回 timeout 不足以验收。

## Remediation Order

1. 已完成：DAQ-P1604 Connect、两套 DAQ-P1604 Stop/read-loop join、P1604 scanner、T1603 scanner、WindLabX4 scanner。
2. 下一批：各设备 Stop/Disconnect 和 reader join，因为它们决定应用能否退出或重连。
3. 再下一批：Connect/command-response helper，统一超时后连接失效语义。
4. 最后：诊断工具和 simulator 生命周期。

长连接整改必须逐设备执行 GitNexus impact、deadline-ignore RED 测试、模拟器回归和可用时的真实硬件回归。不得用全局文本替换批量增加 `Close()`。
