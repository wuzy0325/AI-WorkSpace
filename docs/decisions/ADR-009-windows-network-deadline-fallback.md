# ADR-009: Windows 网络阻塞 I/O 使用独立 Close 兜底

## Date

2026-07-28

## Status

Accepted

## Context

DAQ-P-1604 现场故障中，一台 Windows 电脑可重复出现以下行为：Go TCP 读取已经设置 `SetReadDeadline`，到期后同一 goroutine 内的阻塞 `Read` 仍不返回。旧握手代码在读取 `w1601` 的唯一 ACK 后又执行一次读取，因此该环境把一处协议边界错误放大为永久卡死。

在 `192.168.1.7:9000` 真实设备上验证后确认：删除多余读取、严格区分每条命令响应，并让外层 watchdog 到期调用 `conn.Close()`，可以可靠解除阻塞。零 deadline 真实设备探针完成 `w1601` 和 `u01101`，统计 `SetReadDeadline` / `SetWriteDeadline` 调用均为 0。

该证据说明的是已确认现场环境约束，不推断所有 Windows 或所有 Go 版本都存在同一问题。工程上必须避免把 deadline 的正常行为当作唯一安全边界。

## Decision

1. `SetDeadline`、`SetReadDeadline` 和 `SetWriteDeadline` 只作为正常路径的软超时，不作为有界网络操作的唯一取消机制。
2. 连接握手、设备发现、命令-响应、启动、停止和断开等必须有独立生命周期所有者。超时或取消时，该所有者必须能够调用目标连接的 `Close()`，且不能等待阻塞 I/O goroutine，也不能先获取被该 goroutine持有的 mutex。
3. watchdog 调用 `Close()` 后，连接进入不可复用状态。调用方必须清理状态并重连，禁止清除 deadline 后继续使用原连接。
4. 采集循环可以保留短 read deadline 做健康检测，但停止/断开路径仍必须能主动关闭连接并等待 reader 退出。仅关闭 stop channel 不足以解除内核网络读取。
5. UDP 扫描保留 deadline，同时使用 `time.AfterFunc(timeout, conn.Close)` 作为硬兜底。
6. 硬件网络测试必须包含一个忽略所有 deadline、仅在 `Close()` 后解除 `Read` / `ReadFrom` 的测试连接。普通 `net.Pipe` 或本机 TCP deadline 测试不能证明现场故障模式安全。
7. watchdog 超时错误必须明确表示连接已失效；不得把它作为可在同一连接上重试的普通命令错误。
8. 可选 ACK、空缓冲探测、quiet-window 和 drain 等“无数据也正常”的操作，不得通过阻塞 Read + watchdog Close 判断结果。对可复用长连接而言，watchdog 到期只能证明该探测无法完成，不能证明物理连接故障；协议应改用确定边界、单 reader 分发或不破坏连接的状态机。
9. 命令-响应在完整响应边界前发生 soft deadline timeout、取消、短读或中途错误时，即使 hard watchdog 尚未触发，连接的协议边界也已不确定。除非协议具备 request ID 且单 reader 能证明可安全丢弃迟到响应，否则必须 Close 并要求重连，禁止让下一条命令消费迟到响应。
10. read loop 的 no-data 检测不能只依赖短 read deadline 让循环重新获得控制权。若产品要求无人操作时自动识别半开连接，必须有独立于阻塞 reader 的 no-data owner；Stop/Disconnect 可 Close 只证明主动生命周期有界，不等于自动健康检测有效。

## Safe Pattern

一次性 socket 的最小模式：

```go
if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
    return err
}
watchdog := time.AfterFunc(timeout, func() { _ = conn.Close() })
defer watchdog.Stop()

// Blocking I/O. If the deadline is ignored, Close still releases it.
n, addr, err := conn.ReadFrom(buf)
```

长连接不能机械套用该片段。必须由连接状态机统一决定关闭、reader join、状态失效和重连顺序，避免两个 goroutine 同时读取同一连接。

一次性 UDP scanner 还必须覆盖 Send 阶段；若 watchdog 在 Send 后、Receive 前才建立，阻塞的 `WriteTo` / `Sendto` 仍无硬边界。Windows raw Winsock 实现必须直接对实际 handle 调用 `Closesocket`，仅测试 `net.PacketConn` wrapper 不足以证明该实现合规。

## Rejected Alternatives

### 只调大 deadline

拒绝。现场问题不是 timeout 太短，而是到期没有解除阻塞。

### timeout 后仅返回错误，不关闭连接

拒绝。调用 goroutine 尚未从阻塞 I/O 返回，无法执行返回逻辑；即使另一层先返回，原 reader 仍可能消费后续响应。

### 停止时只关闭 channel

拒绝。Go channel 不会取消已经进入内核的 `net.Conn.Read`。

### 所有 helper 内部自动关闭调用方连接

拒绝。通用 helper 不一定拥有连接生命周期，且关闭后必须同步更新驱动状态。Close 兜底应位于明确拥有连接和重连策略的边界。

## Consequences

- 超时可能将原本可恢复的命令错误升级为断线重连，但能保证调用不会永久卡死。
- 对命令-响应协议，soft timeout 也可能要求重连，因为迟到响应会污染下一条命令；这不是普通可重试错误。
- 对可选/探测读取，不能用“超时即 Close”换取有界性；必须先消除破坏性探测或重新设计边界。
- 驱动必须明确单一 reader、连接所有权和重连状态转换。
- 测试会多一种 deadline 失效替身，但可直接覆盖现场故障，而不是依赖操作系统复现。
- 既有代码按审计报告分阶段整改；不得在无设备回归时批量改变所有长连接状态机。

## Verification Evidence

- DAQ-P-1604 `192.168.1.7:9000` 完成两轮真实协议闭环。
- 严格验证 `w1601`、`u01101`、`v01101`、`c 00`、`c 05`、`c 01`、数据帧和 `c 02`。
- 零 deadline 探针输出：`deadline calls: read=0 write=0`，并正常读取 ACK 和单位系数。
- DAQ-P-1604 两套适配器均有 watchdog 关闭阻塞连接的自动化测试。
- DAQ-P-1604 两套停止路径均验证 reader join 超时后关闭并废弃连接，且不会在旧连接上发送 `c 02` 或启动第二个 reader。
- 历史测试已证明外部 Close 可以解除部分 UDP 阻塞，但不能据此认定三套生产 scanner 全部合规。2026-07-29 复核确认：T1603 Windows 实现已有 raw `Closesocket` watchdog，但生产 timer 触发证据仍需加强；P1604 与 Wind-DAQ Windows `Recvfrom`、以及三套 scanner Send 阶段仍列入整改。

## References

- `docs/audits/2026-07-28-go-network-deadline-audit.md`
- `docs/audits/2026-07-29-adr009-remaining-remediation.md`
- `shared/device-sdk/go/protocol/conn_helpers.go`
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`
- `projects/wind-daq/services/api-go/internal/adapters/hardware/daq_p1604.go`
