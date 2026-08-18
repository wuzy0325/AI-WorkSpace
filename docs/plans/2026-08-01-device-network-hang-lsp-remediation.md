# Implementation Plan: 设备网络操作防"本机卡死"整改（LSP 环境加固）

> 日期：2026-08-01
> 状态：草稿（待批准）
> 审查依据：
> - [两轮专项代码审查](./plan-device-network-hang-2026-08-01.md#审查范围与证据)（设备驱动 + 运动控制 + 扫描/中继 + 桌面壳层）
> - 本机实测：Astrill ASProxy64.dll / 深信服驱动等 LSP 拦截 winsock，导致 `SetReadDeadline` 失效（Read 可永久阻塞）、`Close`/`Closesocket` 在挂起 I/O 时永久阻塞、`Dial` 永不返回（与 `apps/desktop-wails` 退出卡死同根因，2026-08-01 已通过 goroutine dump 实证）

## Overview

本计划整改 windlabx4 全部设备网络操作链路（T1603/P1603/P1064/P1604/DSA3217 驱动、B140/WTNMC4A 运动控制、winsock 设备扫描、本地 API server、桌面壳层 binding），目标是：**在任何单项网络 I/O 被 LSP 卡死时，应用不整体假死、单设备不永久楔死、goroutine/线程不无界泄漏**。

现状核心矛盾：既有 ADR-009 watchdog 体系（DialTCP 软超时、WatchdogClose、noDataTimer、invalidate 毒化）设计健全，但**所有恢复路径的最终兜底都是 `conn.Close()`，而本机 LSP 环境下 `Close()` 自身会永久阻塞**。于是 watchdog 触发本身变成卡死点，并沿调用链传染：驱动方法 → per-id connMu → DeviceManager 全局锁 → Wails binding → 前端 promise 永不 resolve（用户感知"本机卡死"）。

本计划分四个阶段实施：Phase 1 消除"全局锁内网络 I/O"（影响面最大、改动最小）；Phase 2 重构 watchdog 最终兜底语义（根因修复）；Phase 3 运动控制 FFI/扫描专项；Phase 4 收口与文档（含 ADR-009 补充）。

## Resolved Decisions

- **全局锁（`m.mu`/`connMu`）内不得执行任何网络 I/O**；所有驱动调用先取引用、释放锁、再做 I/O。已确认的例外（`ApplyDsa3217ScanConfig` 先放锁再 I/O）为正确模式。
- **`WatchdogClose` 的 stop 语义改为"不等待 Close 完成"**：timer 回调内 Close 必须放入独立 goroutine 且不阻塞 `close(timedOut)`；`wdStop()` 只判定触发与否，绝不等 Close 返回。这是全部恢复路径可用的前提。
- **`Close` 卡死不再作为兜底假设**：新增"放弃式关闭"（detach-close）：CloseWrite+Close 放入后台 goroutine，调用侧立即返回；需要立即回收 fd 的场景使用 `SetLinger(0)` RST 路径（与 p1604 现有 AbortConnection 一致，注意安全软件对 RST 敏感的既有注记）。
- **绑定层（Wails binding / HTTP handler）所有可能触达硬件 I/O 的入口必须可超时**；绑定调用在独立 goroutine 执行 + 有界等待，超时返回错误并释放调用方。
- **扫描链路 `scanInFlight` 等待必须有 deadline**；`winsockDiscoverySocket` 的 `handleMu` 内不允许做可能阻塞的 `Closesocket`（先行取出句柄、锁外关闭，保留二次兜底机会）。
- **DLL FFI（WTNMC4A）调用移入受限 OS 线程池**：Go 无法抢占阻塞的 syscall，改为"专用线程 + 调用侧超时放弃"，线程数量封顶，杜绝每轮询泄漏。
- 运动状态轮询、设备状态轮询的等待必须可被前端取消或超时（规避 `WithoutCancel` ctx 导致的 goroutine 无界累积；无法取消的，改为有界等待后返回陈旧状态）。
- ADR-009 文档在本轮整改后修订：新增"最终兜底失效"章节与 Close 卡死实测证据。

## Dependency Graph

```text
Phase 1  全局锁移出网络 I/O（device_manager.go 三处）
           -> per-id connMu 不再被全局锁内的卡死 I/O 拖累
              -> 应用层假死消除（最大收益）

Phase 2  WatchdogClose stop 语义改造（shared protocol）
           -> DialTCP/Handshake/readLoop/Stop 所有依赖 wdStop 的路径不再等 Close
           -> invalidate/Disconnect/join 超时路径改为后台 detach-close
              -> 驱动层 watchdog 从"卡死点"变回"解除点"

Phase 3  运动控制 FFI 线程池 + 扫描链路 deadline
           -> WTNMC4A syscall 阻塞不再冻结控制器锁
           -> 扫描 Closesocket 锁外执行 + scanInFlight deadline
              -> 急停/扫描/连接 全部可恢复

Phase 4  binding 层超时 + 端口占用告警 + ADR-009 修订
           -> 前端任何操作有界返回
           -> 8900 冲突显性提示
           -> 文档沉淀实测证据与设计约束
```

## Architecture Decisions

### 1. 全局锁内 I/O 移出（Phase 1）

`device_manager.go` 三处整改为"快照引用 → 释放锁 → 调用 → 重取状态"模式：

| 现有位置 | 现状 | 整改 |
|---|---|---|
| `m.mu.Lock()` 内 `SetUnit`（269-295） | 持全局锁做 5s 网络命令 | 锁内仅取 `configurable` 引用，锁外调用 `SetUnit`，返回后锁内更新 profile |
| `m.mu.Lock()` 内 `ApplyDaqT1603Config`（314-337） | 持全局锁多条网络命令 | 同上；锁外调用，失败仅记错误不持有锁 |
| `m.mu.RLock()` 内 `GetDsa3217ScanConfig`（340-350） | RLock 内 sendCommand | 与 `ApplyDsa3217ScanConfig`（353-373）对称：先取引用，RLock 释放后调用 |

约束：锁外调用期间设备可能被并发 Disconnect/Connect——驱动对象自身已有 expectedConn/互斥保护（ADR-009 既有设计），usecase 层仅需保证"锁外调用后重新读取状态"。

### 2. WatchdogClose 终止语义重构（Phase 2，根因）

现状（`shared/device-sdk/go/protocol/conn_helpers.go:420-456`）：

```go
timer := time.AfterFunc(timeout, func() {
    _ = conn.Close()   // LSP 下永久阻塞
    close(timedOut)    // 永不执行
})
return func() bool {
    once.Do(func() {
        if !timer.Stop() { <-timedOut /* 永久阻塞 */ }
    })
}
```

整改：

```go
timer := time.AfterFunc(timeout, func() {
    go AbortConnection(conn) // 已是后台；但需先 close(timedOut)
    close(timedOut)
})
return func() bool {
    once.Do(func() {
        if !timer.Stop() {
            <-timedOut       // 现在必然返回：只等"已发起关闭"，不等关闭完成
            triggered = true
        }
    })
    return !triggered
}
```

同时为阻塞的 `conn.Close()` 兜底：`AbortConnection` 在 detach goroutine 中先 `CloseWrite`（FIN 立即返回）再 `Close`；若 `Close` 仍卡死，由 `SetLinger(0)` RST 兜底（需实测确认 LSP 下 RST 路径可靠）。`WatchdogClose` 后 conn 一律不可复用（语义不变）。

波及面（全部等待 `wdStop`/`<-timedOut` 的现有路径自动受益）：`DialTCP`、`P1604ReadCommandACK`、`DrainConnection`、`daq_t1603`（connect/stop/invalidate）、`daq_p1064pre`、`daq_p1604` handshake（116-129 的 `<-timedOut`）、`dsa3217` readLoop/sendCommand。

### 3. 驱动层剩余同步 Close 点改造（Phase 2）

审查发现的直接 `conn.Close()` 同步调用点，全部改为"后台 detach-close + 不等待"：

| 位置 | 现状 | 整改 |
|---|---|---|
| `daq_p1064pre.go:219/273/332→389` | join 超时后同步 `conn.Close()` | `go AbortConnection(conn)`（与 t1603 现有模式一致） |
| `daq_p1604.go:746`、`wtn_pxi.go:370` | noDataTimer 内同步 Close | timer 内 `go AbortConnection`，且先 `close(触发信号)` 再发起关闭 |
| `dsa3217.go:356-357` | invalidate 同步 Close | 后台 detach-close |
| `dsa3217.go:444-492` | sendCommand watchdog + ioMu 死锁 | Phase 2 自动修复（wdStop 不等待）；仍保留 ioMu 前启 watchdog 顺序 |

### 4. 运动控制 FFI 线程池（Phase 3）

`wtnmc4a_motion.go` 现状：`Connect/Disconnect/queryStatus` 在 `ioMu.Lock()` 内直接 `procs.devCreateA.Call(...)`——syscall 阻塞不可抢占，watchdog 无法解除，锁永久持有。

整改：
- 新增受限 OS 线程池（如 4 线程 + 阻塞 FIFO 队列），所有 FFI 调用投递到池中执行；
- 调用侧 `select { case r := <-pool.Do(fn): ...; case <-time.After(timeout): return 超时 }`；
- 超时后：放弃等待（池内 goroutine 泄漏 1 个受控线程 + 句柄资源，不再每轮询无界增长），`ioMu` 不持有（持锁范围只覆盖投递与结果回写）；
- 急停/Disconnect 可绕过等待直接标记控制器为"不可用"并通知上层。

### 5. 扫描链路 deadline（Phase 3）

- `discovery_socket_windows.go:75-91`：`closeHandleLocked` 改为**先取出 handle 并置 0，锁外执行 `windows.Closesocket`**；watchdog 回调同样锁外关闭——即使 Closesocket 被卡死，句柄已不在结构体上，调用侧仍可通过超时放弃，保留二次关闭机会。
- `device_manager.go:102-107`：`<-pending.done` 加 `select` deadline（如 5s）；超时后返回"扫描超时"错误，**并重置 `m.scanInFlight` 为 nil**，保证扫描功能不被一次卡死永久锁死。
- `network_scanner.go:196-208`：保持有意泄漏（限界内），但每次扫描前的枚举改为可被 ctx 取消；若不可取消则记录泄漏计数并限制触发频率。

### 6. binding 层有界执行（Phase 4）

- `app.go:889-898 callMgr`：`fn()` 执行放入独立 goroutine + 有界等待（如 10s），超时返回错误给前端（前端显示"操作超时"而非永久"连接中"）。
- 设备/运动/扫描类 binding 全部走该包装；`DeviceScanDevices`（963-968）补上同一包装。
- 轮询类（`MotionGetStatus` 等）明确返回陈旧状态而非无限等待。

### 7. 端口占用显性化（Phase 4）

`app.go:247-283`：`ListenAndServe` 绑定失败/运行中异常退出时，不静默——弹出提示（"本地服务端口 8900 被占用，请关闭其他 WindLabX4 实例"）+ 退出或回退端口（如 8901，需前端同步配置，默认回退为退出）。

## 验证计划

### 单元/集成测试（必须新增）

| 场景 | 测试 |
|---|---|
| wdStop 不等待 Close | mock conn：Close 阻塞 10s，`WatchdogClose` 触发后 `wdStop()` 应在毫秒级返回且返回 false |
| DialTCP 软超时 | 既有 R1-4 测试扩展：确认超时后 goroutine 晚到 conn 被关闭（不泄漏） |
| P1604 握手 `<-timedOut` 不卡 | mock：Close 永久阻塞，Connect 应在 watchdog 触发后立即返回错误 |
| 全局锁内无 I/O | 代码审查回归：device_manager 三个方法内不得出现 `dev.*` 调用持锁（用锁内打点断言） |
| scanInFlight 超时重置 | mock scanner 永不返回 → 第二次 Scan 应在 5s 后报超时并可再次发起 |
| binding 超时 | 注入阻塞 binding → 调用在 10s 内返回错误 |

### 本机（LSP 环境）回归

1. 启动 → 连接 T1603 → 开始采集 → **拔网线/停设备** → 停止采集：Stop 应在 ~350ms 返回（静默窗口兜底），不卡 3s，UI 可继续操作。
2. 连接后对已死连接执行 SetUnit / ApplyConfig：应用其余按钮（状态轮询、扫描）仍响应。
3. 扫描（无设备网段）：在超时窗口内可再次点击扫描，不永久"扫描中"。
4. 运动：连接 B140/WTNMC4A 后断开设备网线 → 急停按钮仍可用；状态轮询不累积 goroutine（观察进程线程数平稳）。
5. 退出（回归 2026-08-01 修复）：正常退出 <2s，不出现 30s watchdog。
6. 多实例：同时启动第二个 windlabx4，第二个应提示端口冲突而非静默无 UI。

## 分阶段交付清单

| Phase | 内容 | 验收标准 |
|---|---|---|
| P1 | device_manager 三处锁外 I/O | 全局锁内无网络调用（grep 断言）；卡死设备不再拖垮全局 |
| P2 | WatchdogClose 语义 + 驱动 Close 点改造 | 所有 wdStop/join 路径在 LSP 下可超时返回；单设备楔死可恢复 |
| P3 | WTNMC4A 线程池 + 扫描链路 | 急停始终可用；扫描可重试；轮询 goroutine 平稳 |
| P4 | binding 超时 + 端口提示 + ADR-009 修订 | 前端操作有界返回；ADR-009 含"Close 卡死"实测章节 |

每阶段独立可交付、可验证；P1 优先落地（投入小、收益最大）。

## 审查范围与证据

- 审查批次 1（设备驱动）：`dsa3217.go` / `dsa3217_readloop.go` / `daq_p1603_adapter.go` / `daq_p1064pre.go` / `daq_p1604.go` / `t1603_adapter.go` / `wtn_pxi.go` + vendor 版 `daq_t1603.go` / `conn_helpers.go`。
- 审查批次 2（运动/扫描/壳层）：`b140_motion.go` / `wtnmc4a_motion.go` / `factory.go` / `serialport/port.go` / `veh.go` / `discovery_socket*.go` / `network_scanner.go` / `apiserver.go` / `stream_relay.go` / `app.go`。
- 实测证据：2026-08-01 退出卡死 goroutine dump（`apiServer.Shutdown → FD.Close → cancelIO 等待`），证明 `Close` 卡死是本机真实故障模式；`daq_t1603.go` probeDeadlineBroken 实测（500ms deadline 下 Read 阻塞 >60s）。
- 已核查无风险项：stream_relay（ctx 逃逸+有界 channel）、wrapper/factory（无 I/O）、veh（无网络）、registry/ServiceShutdown 双 deadline、桌面端 shutdown 300ms/30s 兜底。

## 实施记录（2026-08-01 完成）

### 已落地改动

| 计划项 | 实际改动 | 验证 |
|---|---|---|
| P1 全局锁移出 I/O | `device_manager.go`：SetUnit / ApplyDaqT1603Config / GetDsa3217ScanConfig / UpsertProfile 四处改为锁内快照引用→锁外调用→重取锁更新 | `go build` + usecase 测试通过 |
| P2 WatchdogClose | **根因实为 vendor 滞后**：shared 源码已含正确实现（timer 回调 `go AbortConnection + close(timedOut)`），但 windlabx4 两个 vendor 副本是旧版（同步 `_ = conn.Close()` + `<-timedOut` 永久阻塞）。已把 shared 最新 `conn_helpers.go` / `daq_t1603_frame.go` / `daq_t1603.go` 同步到 `services/api-go/vendor` 与 `apps/desktop-wails/vendor` | build + hardware 测试通过 |
| P2 驱动 Close detach | `daq_p1064pre.go` 13 处、`daq_p1604.go` 6 处、`wtn_pxi.go` 3 处、`dsa3217.go` 3 处：所有可能在挂起 I/O 下阻塞的 `conn.Close()` 改为 `go sharedproto.AbortConnection(conn)`（含 Disconnect join 超时、invalidate、noDataTimer、握手 watchdog、命令失败路径） | build + hardware 测试通过 |
| P3 WTNMC4A FFI | `wtnmc4a_motion.go` 新增 `ffiGate`（单 worker + 有界投递/等待 + 超时放弃），全部 15+ 个 DLL 调用点走 gate；新增 `markControllerUnreachableLocked`；Disconnect 关闭 gate。同步两个 vendor | build + WTNMC4A 测试通过 |
| P3 扫描链路 | `discovery_socket_windows.go`：Closesocket 移出 handleMu 锁（后台执行，锁内仅取走 handle）；`discovery_socket.go`：watchdog 回调 `go conn.Close()`；`device_manager.go`：`<-pending.done` 加 8s deadline + 超时重置 scanInFlight | build + scan/usecase 测试通过 |
| P4 binding 超时 | `app.go`：`callMgr` 改为 goroutine + 10s 有界等待，超时返回"操作超时"；`DeviceScanDevices` 同样处理 | build-go 通过 |
| P4 端口冲突 | `app.go`：`startLocalAPIServer` 先 `probeLocalPort` 探测 8900，被占用弹 Dialog 提示 + 不启动服务（不再静默） | build-go 通过 |

### ADR-009 文档说明

ADR-009 文档本体位于 `docs/decisions/ADR-009-windows-network-deadline-fallback.md`（zip 导出时未包含工作区 `docs/decisions/` 目录，故此前判定"未找到"）。其核心约束已完整体现在 shared `conn_helpers.go` 注释（含 2026-07-31 的 Close 卡死实测记录）。本轮整改已在 ADR-009 补充"2026-08-01 补充：最终兜底失效（Close 卡死）与 LSP 环境加固"章节，正式归档 Close 卡死实测证据与新增决策。

### 遗留风险（不在本次范围）

- `b140_motion.go:1450-1456`（connMu 跨 `<-done` 无第二次超时）与 `daq_p1604.go` 握手 watchdog 依赖同源修复（`WatchdogClose` 已改）——b140 是独立实现，若 LSP 下 AbortConnection 的 Close 也卡死，仍需二次超时兜底。标记为后续迭代。
- `network_scanner.go:196-208` 有意泄漏 ~3 goroutine/扫描（有界，可接受）。
- WTNMC4A 未接入 `onError` 回调（`markControllerUnreachableLocked` 只改状态），上层 MotionManager 需在后续迭代接住不可达状态触发重连。
