# ADR-009 剩余整改清单（复核修订版）

## 文档状态

- 初次审计：2026-07-29
- 复核修订：2026-07-29
- 第一批整改完成：2026-07-29（R0-3 / R0-5 / R0-6 / R0-8 / R0-9，附带 DAQP1064Pre 路径的 R0-12 验证）
- 第二批整改推进中：2026-07-29（R0-1 已完成）/ 2026-07-30（R0-11 已完成，R0-10 已完成）
- 第三批整改完成：2026-07-30（R0-2 / R0-4 / R0-12 / R1-1 / R1-2 / R1-3）
- 第四批整改完成：2026-07-30（R0-7 / R1-4）
- 第五批整改完成：2026-07-30（R2-2 完成；R2-1 仅完成 Dial 部分，命令 Write/帧读取/可选 ACK/quiet-window/drain 仍依赖短 deadline + 进程级 5 分钟 watchdog 兜底）
- 复核修订：2026-07-30（独立审查发现 findings 1-9；findings 1-6 + 9 已修复，findings 7-8 状态已就位）
- 当前状态：**整改未完成** — 独立审查 Request Changes，findings 1-6 + 9 已修复并补回归测试/证据；R2-1（finding 7）仍局部完成；文档状态（finding 8）已同步本轮实际代码
- 范围：生产硬件 TCP/UDP I/O、共享协议 helper、随项目交付的诊断工具
- 依据：[ADR-009](../decisions/ADR-009-windows-network-deadline-fallback.md)

## 撤回旧结论

本文件旧版曾声明“全部 P0/P1/P2/SIM-1 风险点已完成修复”。该结论现正式撤回。

旧审查只验证了“deadline 失效后 watchdog 最终能否通过 Close 解除阻塞”，没有验证以下同等重要的正确性条件：

1. 可选、探测、排空或静默窗口读取是否会因为“正常无数据”而错误关闭健康连接。
2. watchdog 关闭连接后，驱动是否同步清除 `conn`/reader 并进入 Error 状态。
3. read-loop join 超时后是否仍可能在同一连接上启动第二个 reader。
4. watchdog 是否在可能阻塞的锁和 Write 之前建立，而不是只覆盖 Read。
5. TCP Dial 是否由调用方可控的外层边界约束，而不是继续依赖 Dial 内部 deadline。
6. 测试是否证明业务语义正确，而不只是证明函数“最终返回”。

因此，历史 `go test -race`、build、vet 全绿只能证明已有测试通过，不能证明 ADR-009 整改完整。

## 修订后的验收标准

一条有界生产硬件 I/O 路径只有满足以下**所有适用条件**，才能标记为完成；不适用项必须在该路径的验收记录中注明原因：

1. **边界明确**：响应按协议长度、长度前缀或确定帧边界读取，不靠读取一个可能不存在的字节来等超时。
2. **独立取消 owner**：deadline 失效时，独立 goroutine/timer 能不等待阻塞 I/O、不获取其持有的 mutex，直接 Close 底层连接或句柄。
3. **不误杀健康连接**：可选 ACK、空缓冲探测、quiet-window、drain 等“无数据也正常”的操作，不得把 watchdog 到期等同于物理连接故障。
4. **连接毒化一致**：凡 watchdog 已 Close 长连接，调用方必须原子清除 conn/reader、停止采集并标记 Error；禁止保留非 nil 的已关闭连接。
5. **单 reader**：read loop 必须可 join；join 超时必须先 Close 并要求重连，禁止清空 done 后在原连接上启动第二个 reader。
6. **覆盖 Write 和锁等待**：watchdog 必须在可能阻塞的锁和 Write 之前建立；只保护响应 Read 不算完成。
7. **Dial 有调用方硬边界**：生产 Dial 必须使用可让调用方按时返回的 owner 模式；晚到 conn 必须可靠关闭。
8. **测试证明两面语义**：既测试 deadline 无效时能退出，也测试正常无数据不会错误销毁健康连接，并断言 Close 后状态已失效。

## 当前状态总览

| 编号 | 范围 | 当前结论 | 优先级 |
|---|---|---|---|
| R0-1 | T1603 旧 read loop join 后继续启动 | **第二批已完成** | P0 |
| R0-2 | T1603 多条命令路径在 watchdog 前等待 `writeMu` 或仅靠 write deadline | **第三批已完成** | P0 |
| R0-3 | T1603 可选 ACK / drain 误杀健康连接 | **第一批已完成** | P0 |
| R0-4 | T1603 watchdog Close 后状态未统一失效 | **第三批已完成** | P0 |
| R0-5 | WindLabX4 P1604 `DrainConnection` 误杀健康连接 | **第一批已完成** | P0 |
| R0-6 | DAQP1064Pre 同一 TCP 流存在双 reader | **第一批已完成** | P0 |
| R0-7 | DSA3217 / DAQP1064Pre / WTN-PXI / B140 Dial 外层边界 | **第四批已完成** | P0 |
| R0-8 | P1604、WindLabX4 Windows UDP Recvfrom 无 raw-handle watchdog | **第一批已完成** | P0 |
| R0-9 | 三套 UDP discovery Send 无独立 Close owner | **第一批已完成** | P0 |
| R0-10 | T1603、两套 P1604、DAQP1064Pre、WTN-PXI 采集 read loop 仅靠 deadline 自检无数据 | **第二批已完成** | P0 |
| R0-11 | T1603、WindLabX4 P1604 异常 read 退出后保留失效 conn/reader | **第二批已完成** | P0 |
| R0-12 | 命令响应 soft timeout 后未统一毒化协议连接 | **第三批已完成** | P0 |
| R1-1 | P1604 unit helper / drain Close 后状态传播 | **第三批已完成** | P1 |
| R1-2 | 独立 P1604 `operationMu` 位于多条 I/O 硬边界之外 | **第三批已完成** | P1 |
| R1-3 | DSA3217 terminal read error 后状态未统一失效 | **第三批已完成** | P1 |
| R1-4 | `DialTCP` 晚到连接可能泄漏 | **第四批已完成** | P1 |
| R2-1 | 四套诊断工具 watchdog 未覆盖 Dial | **局部完成** — Dial 已改用 `DialTCP`（R1-4 整改）；命令 Write、帧读取、可选 ACK、quiet-window、drain 仍主要依赖短 deadline，5 分钟进程 watchdog 不等价于每条操作的硬边界 | P2 |
| R2-2 | DSA3217 Disconnect 覆盖 Error 状态 | **第五批已完成** | P2 |
| C-1 | 独立 P1604 Start 已移除 `DrainConnection` | 已核实 | 完成 |
| C-2 | P1604 ACK helper 有硬 watchdog 和有界跳帧 | 已核实，仍要求调用方失效状态 | 局部完成 |
| C-3 | T1603 Windows UDP Recvfrom 有 `Closesocket` watchdog | 实现已核实，生产 timer 测试证据不足 | 局部完成 |
| C-4 | SIM 生命周期 ctx/Close/WaitGroup | 已核实 | 完成 |

## P0：必须优先整改

### R0-1：T1603 join 超时后可能启动第二个 reader

位置：

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`DAQT1603.StartAcquisition`

现状：等待旧 `readLoopDone` 三秒超时后仅记录日志，随后清空 done 并继续在原连接上启动新 read loop。

风险：deadline 失效导致旧 reader 仍阻塞时，新 reader 会与旧 reader 竞争同一 TCP 字节流，产生错帧、命令响应被抢读和不可恢复的协议错位。

整改：join 超时必须调用独立 invalidation 路径 Close conn、清除 frameReader、标记 Error 并返回 `reconnect required`；禁止继续启动。

验收：使用忽略 deadline、只在 Close 后返回的 conn double，断言不会创建第二个 reader，conn 被清除，状态为 Error。

#### 第二批整改证据（2026-07-29）

**修改文件与符号：**

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`DAQT1603.StartAcquisition`
  - join 超时分支（原行 259-261）从"只打 warn 日志 + 清空 readLoopDone + 继续执行"改为"调用 `invalidateConnectionAfterReadLoopTimeout` + 返回 `reconnect required` 错误"。
  - 不继续发送 `@f1`/`@f0`，不启动新 readLoop，不调用 `frameReader.Reset()`。
  - `<-done` 正常退出分支保持原行为（清空 `readLoopDone` 后继续启动）。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`invalidateConnectionAfterReadLoopTimeout`
  - 新增 `d.readLoopDone = nil` 清理，避免下次 `StartAcquisition` 入口误判"readLoop 未退出"而再次废弃连接。
  - readLoop defer 通过 `done := d.readLoopDone; if done != nil { close(done) }` 模式安全处理 nil，不会 panic。
- `shared/device-sdk/go/daq/hardware/daq_t1603_test.go`：
  - 新增 `t1603RecordingDeadlineIgnoringConn` 类型（内嵌 `t1603DeadlineIgnoringConn` + `atomic.Int32` Write 计数器），用于验证 join 超时后是否继续发送命令。
  - 新增 `TestDAQT1603StartAcquisition_InvalidatesConnOnReadLoopJoinTimeout` 测试。

**根因：** 原实现 join 超时后清空 `readLoopDone` 并继续执行 `@f1`/`@f0` Write + 启动新 readLoop。问题 Windows 电脑 deadline 失效导致旧 reader 仍阻塞在 `conn.Read` 时，新 reader 会与旧 reader 竞争同一 TCP 字节流，产生错帧、命令响应被抢读和不可恢复的协议错位。

**独立 Close owner：** `invalidateConnectionAfterReadLoopTimeout` 在锁外执行 `conn.Close()`，不依赖阻塞 I/O，不获取 `writeMu`。与 `stopAcquisitionLocked` 的 join 超时路径共享同一 invalidation 函数，确保状态最终一致性。

**watchdog/soft timeout 后如何失效 conn：** join 超时分支调用 `invalidateConnectionAfterReadLoopTimeout`：`d.mu.Lock` → `conn = d.conn; d.conn = nil; d.frameReader = nil; d.acquiring = false; d.stop = nil; d.readLoopDone = nil; d.status.Connection = Error; d.status.LastError = message` → `d.mu.Unlock` → `conn.Close()` → `onError`。连接已关闭，旧 readLoop 的 `conn.Read` 会返回错误并退出（defer 安全处理 nil readLoopDone）。

**reader 和驱动状态：** `d.conn = nil`、`d.frameReader = nil`、`d.acquiring = false`、`d.stop = nil`、`d.readLoopDone = nil`、`d.status.Connection = Error`、`d.status.LastError` 设置。旧 readLoop goroutine（若仍存活）退出时 defer 读到 `d.readLoopDone == nil`，不 close，安全。

**是否存在可选/探测读取：** 否，join 是 readLoop 生命周期管理，不是可选/探测读取。

**如何证明不会误杀健康连接：**
- 测试场景是"readLoop 未在 3s 内退出"，这本身就说明连接异常（deadline 失效或 readLoop 卡死），不是"健康连接"。本测试验证的是"异常连接必须废弃，不能启动第二 reader"。
- 正常场景（readLoop 在 3s 内退出）走 `<-done` 分支，清空 `readLoopDone` 后继续启动新 readLoop，行为不变。

**对应测试和实际验证结果：**

- `TestDAQT1603StartAcquisition_InvalidatesConnOnReadLoopJoinTimeout`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:780`）：
  - 测试前置：`net.Pipe` + `t1603RecordingDeadlineIgnoringConn`（忽略 SetWriteDeadline + 记录 Write 调用次数）+ 伪造未关闭的 `readLoopDone`。
  - 测试步骤：调用 `device.StartAcquisition()`，等待 join 超时。
  - 期待结果：返回 `"reconnect required"` 错误；`d.conn == nil`；`status.Connection == Error`；`d.acquiring == false`；`d.readLoopDone == nil`；`recordingClient.WriteCount() == 0`（未继续发送 @f1/@f0）；`server.Write` 失败（conn 已 Close）。
  - 修复前：`WriteCount() == 1`（join 超时后继续 @f1 Write，watchdog 1s 触发 Close），测试断言失败。
  - 修复后：`WriteCount() == 0`（join 超时直接废弃连接），测试通过。

```
$ go test -race -count=1 -run TestDAQT1603StartAcquisition_InvalidatesConnOnReadLoopJoinTimeout ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/daq/hardware 9.731s

$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/protocol     8.227s
ok      shared.local/device-sdk/go/daq/hardware 11.370s

$ go vet + go build + gofmt：全部空输出

$ 跨项目回归验证：
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     23.141s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 8.076s

$ GOWORK=off; cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-t1603/adapters/hardware     3.232s
```

**遗留：** 无。R0-1 整改完整覆盖 join 超时废弃连接、不启动第二 reader、不继续发送命令、状态失效一致性。

### R0-2：T1603 命令锁等待与 Write 未被统一硬边界覆盖

位置：

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`writeCommandOnly`
- 同文件：`DAQT1603.StartAcquisition` 的 `@f0` 路径
- 同文件：`ApplyDaqT1603Config` 在 helper watchdog 前获取 `writeMu`
- 同文件：`syncHardwareConfigLocked` 在 helper watchdog 前获取 `writeMu`
- 同文件：read-loop 异常退出路径在 `@f1` watchdog 前获取 `writeMu`

现状：`@f0` 只设置 `SetWriteDeadline` 后同步 Write；其他路径虽然 helper 内有 watchdog，但 watchdog 在调用方已经获取 `writeMu` 后才建立；异常退出路径也先等 `writeMu` 再启动 `@f1` watchdog。

风险：Write deadline 失效或其他 goroutine 持 `writeMu` 卡死时，后续命令、清理、Stop、Disconnect 和状态失效路径可能一起无法推进。

整改：建立覆盖每次命令操作的统一 owner，必须在 `writeMu.Lock` 之前启动，覆盖锁等待、Write 和响应 Read；触发后立即执行 expected-conn 统一失效，不保留已关闭 conn。避免多个内外 watchdog 对同一连接产生所有权歧义。

#### 第三批整改证据（2026-07-30）

**修改文件与符号：**

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`writeCommandOnly`
  - watchdog 从"Lock 后启动"改为"Lock 之前启动"，覆盖"等 writeMu + Write"全区间。
  - watchdog 触发后返回 `protocol.ErrWatchdogTriggered` sentinel，让调用方统一毒化连接。
  - Write 成功后再次检查 `wdStop()`：conn 可能在 Write 完成后被 watchdog Close，仍需返回 sentinel 让调用方毒化。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`StartAcquisition` 的 `@f0` 路径
  - `writeCommandOnly` 返回 `ErrWatchdogTriggered` 时，在持 `d.mu` 状态下直接清空 `d.conn`/`d.frameReader` 并置 `status.Connection=Error`（不调用 `invalidateConnection` 避免与已持锁死锁）。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`ApplyDaqT1603Config` 调用 `applyHardwareConfig` 路径
  - `applyHardwareConfig` 内部调用 `syncHardwareConfigLocked` → `sendCommand` 系列（helper 内自带 watchdog），失败时检查 `errors.Is(err, protocol.ErrWatchdogTriggered)` 调用 `invalidateConnection(conn, reason)` 统一毒化。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`readLoop` 异常退出路径的 `@f1` 路径
  - defer 块在 `unexpectedErr != nil` 时启动 `protocol.WatchdogClose(conn, 1s)` 覆盖 `@f1` Write，watchdog 触发后通过 `invalidateConnectionAfterReadLoopTimeout` 统一毒化。

**根因：** 原实现 watchdog 在 `writeMu.Lock` 之后启动，问题 Windows 电脑 `SetWriteDeadline` 失效时 Write 永久阻塞，writeMu 永远无法释放，后续所有命令、清理、Stop、Disconnect 路径死锁。

**独立 Close owner：** `protocol.WatchdogClose(conn, DAQ_T_1603_TIMEOUT)` 在 `writeMu.Lock` 之前启动，是独立 owner。`time.AfterFunc` 触发时直接 `conn.Close()`，不等待阻塞 I/O，不获取 `writeMu`。

**watchdog/soft timeout 后如何失效 conn：**
- `writeCommandOnly` 返回 `ErrWatchdogTriggered` → `StartAcquisition` 持 `d.mu` 清空 `d.conn`/`d.frameReader`、置 `status=Error`。
- `applyHardwareConfig` 返回 `ErrWatchdogTriggered` → `ApplyDaqT1603Config` 调用 `invalidateConnection(conn, reason)` 锁外清空 + Close。
- `readLoop` defer 路径 `@f1` watchdog 触发 → `invalidateConnectionAfterReadLoopTimeout` 统一毒化。

**reader 和驱动状态：** watchdog 触发后 `d.conn=nil`、`d.frameReader=nil`、`d.acquiring=false`、`d.stop=nil`、`d.readLoopDone=nil`、`d.status.Connection=Error`、`d.status.LastError` 设置。

**是否存在可选/探测读取：** 否，`writeCommandOnly` 是纯 Write 操作。

**如何证明不会误杀健康连接：**
- watchdog timeout=5s（`DAQ_T_1603_TIMEOUT`），正常 `@f0`/`@f1`/配置命令 Write 在 100ms 内完成，5s 是 50 倍余量，绝不误杀。
- 测试用 `deadlineIgnoringConn`（忽略 SetWriteDeadline）让 Write 永久阻塞，watchdog 5s 触发后才返回——这本身就是异常场景，不是健康连接。

**对应测试和实际验证结果：**

- `TestDAQT1603WriteCommandOnly_WatchdogTriggersBeforeLock`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:1332`）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn`（忽略 SetWriteDeadline）+ 前序 goroutine 持 `writeMu` 后调用 `Write` 永久阻塞。
  - 测试步骤：主 goroutine 调用 `writeCommandOnly(@fe BIN 1)`。
  - 期待结果：`writeCommandOnly` 在 7s 预算内返回错误，错误含 `watchdog triggered`；前序 goroutine 的 `Write` 因 watchdog Close conn 返回错误释放 `writeMu`。
- `TestDAQT1603ApplyDaqT1603Config_InvalidatesConnOnWatchdogTrigger`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:1437`）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn` + 设备正常响应配置查询命令但 `@fe BIN 1` Write 阻塞。
  - 测试步骤：调用 `ApplyDaqT1603Config`。
  - 期待结果：返回 `ErrWatchdogTriggered`；`d.conn==nil`；`d.frameReader==nil`；`status.Connection==Error`；`status.LastError` 非空。
- `TestDAQT1603StartAcquisition_InvalidatesConnOnWriteCommandWatchdogTrigger`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:1503`）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn` + `@f0` Write 永久阻塞。
  - 测试步骤：调用 `StartAcquisition`。
  - 期待结果：返回 `reconnect required` 错误；`d.conn==nil`；`status.Connection==Error`。

```
$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/protocol     11.079s
ok      shared.local/device-sdk/go/daq/hardware 19.862s

$ go vet ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
VET_EXIT=0

$ Push-Location shared/device-sdk/go; go build ./...; Pop-Location
BUILD_EXIT=0
```

**遗留：** 无。`writeCommandOnly` / `applyHardwareConfig` / `@f0` / `@f1` 四条路径的 watchdog 全部前移到锁等待之前，触发后统一毒化连接。

### R0-3：T1603 可选 ACK 和 drain 会误杀健康连接

位置：

- `shared/device-sdk/go/protocol/daq_t1603_frame.go`：`T1603FrameReader.ConsumeOptionalACK`
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`drainConnection`

现状：两者都通过阻塞 Read 判断“是否有可选数据”。问题机忽略 deadline 时，正常无 ACK 或空缓冲会等到 watchdog，随后健康连接被 Close。

整改：

- 本轮实机只读/同值设置测试没有执行 `@f0`，因此不能把单字节 `A` 当作本轮已验证事实。整改前必须通过设备协议资料或受控实机采集测试确定 `@f0` 在各支持固件上的 ACK 契约。
- 启动/配置路径不得使用阻塞 drain 判断空缓冲；优先通过严格命令所有权、frameReader Reset 和停止 ACK 边界保证无残留。
- 若确有兼容固件需要可选 ACK，必须采用不会对长连接执行破坏性探测的架构，不能以 watchdog Close 作为“无 ACK”结果。

验收：deadlineIgnoring conn + 对端保持连接但不发数据，操作必须按协议语义返回，且对端后续仍能成功收发；不得期待 conn 被关闭。

#### 第一批整改证据（2026-07-29）

**修改文件与符号：**

- `shared/device-sdk/go/protocol/daq_t1603_frame.go`：`T1603FrameReader.ConsumeOptionalACK`
  - 移除 `WatchdogClose` 兜底，仅依赖 `SetReadDeadline` 软超时；timeout 到期返回 `(false, nil)` 表示"无 ACK"，是正常结果。
  - 保留 `SetReadDeadline(time.Time{})` 清理，避免残留 timeout 影响后续 `ReadFrame`。
  - 注释明确本函数仅在诊断工具（freqprobe/frameprobe）中使用，生产路径不再调用。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：彻底删除 `drainConnection` 函数及其在 `StartAcquisition` / `stopAcquisitionLocked` / `ApplyDaqT1603Config` 中的所有调用点。
  - 残留字节（如 `@f1` ACK 或 readLoop 退出时设备已排队的压力帧）由 `readLoop` 的 `extractValidFixedFrameLocked` 逐字节丢弃重同步完成，不会破坏长连接。
  - `stopAcquisitionLocked` 改为依赖 `frameReader.Reset()` 清空应用层缓冲区。

**根因：** 可选 ACK 与 drain 都是"无数据也正常"的探测操作，原实现通过 `阻塞 Read + watchdog Close` 判断结果，watchdog 到期被错误等同于物理连接故障，违反 ADR-009 决策 8。

**独立 Close owner：** 本路径整改后**不再需要**独立 Close owner——可选 ACK 与 drain 的语义是"无数据正常"，不应有任何 Close 行为。如果上层（如诊断工具）需要硬超时兜底，由其自身的进程级 watchdog 负责，本函数不承担连接生命周期管理。

**watchdog/soft timeout 后如何失效 conn：** 整改后 `ConsumeOptionalACK` 在 timeout 时返回 `(false, nil)`，不调用 `Close()`；`drainConnection` 已删除。健康连接保持开放，后续 `ReadFrame` 可继续工作。

**reader 和驱动状态：** `frameReader` 通过 `Reset()` 清空应用层缓冲区；`d.conn`、`d.frameReader`、`d.status` 均保持不变（健康连接不应被失效）。

**是否存在可选/探测读取：** 是，`ConsumeOptionalACK` 保留为诊断工具使用的可选读取，但已改为非破坏性。

**如何证明不会误杀健康连接：**

- `TestT1603ConsumeOptionalACK_DoesNotCloseHealthyConnWhenNoACK`（`shared/device-sdk/go/protocol/daq_t1603_frame_test.go:996`）：对端不发数据，`ConsumeOptionalACK(50ms)` 返回 `(false, nil)`；后续 `server.Write("alive")` + `client.Read` 成功，证明连接未被关闭。
- `TestDAQT1603StopAcquisition_DoesNotCloseHealthyConnWhenNoData`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:486`）：`StopAcquisition` 在空缓冲场景下不触发 watchdog，`device.conn` 仍非 nil，`server.Write` + `client.Read` 成功。
- `TestDAQT1603ApplyDaqT1603Config_DoesNotCloseHealthyConnWhenNoData`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:576`）：`ApplyDaqT1603Config` 在空缓冲场景下不误杀连接。

**对应测试和实际验证结果：**

```
$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/protocol     8.246s
ok      shared.local/device-sdk/go/daq/hardware 8.385s
```

**遗留：** `@f0` ACK 契约仍需实机验证（C-4 已声明本轮未执行 `@f0`/`@f1`），生产路径已不依赖该契约。

### R0-4：T1603 watchdog Close 后状态未统一失效

位置：

- `DAQT1603.StartAcquisition` 调用 `ConsumeOptionalACK` 的错误路径
- `DAQT1603.ApplyDaqT1603Config` 调用协议 helper 的错误路径
- 协议 helper：`SendCommand`、`SendCommandIdle`、`SendCommandExact`

现状：helper 可能已经 Close conn，但运行期调用方只返回错误，`d.conn`、frameReader 和 Connected 状态仍可能保留。

整改：增加单一 `invalidateConnection(message, expectedConn)` 路径；所有 `ErrWatchdogTriggered`、本地主动 watchdog Close 和 join 超时统一调用；用 expected conn 防止误清并发建立的新连接。

#### 第三批整改证据（2026-07-30）

**修改文件与符号：**

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：新增 `invalidateConnection(expectedConn net.Conn, reason string) bool`
  - 锁内比较 `d.conn == expectedConn`，匹配才毒化；不匹配 no-op 返回 false。
  - 毒化路径：`d.conn=nil`、`d.frameReader=nil`、`d.acquiring=false`、`d.stop=nil`、`d.readLoopDone=nil`、`d.status.Connection=Error`、`d.status.LastError=reason` → 锁外 `conn.Close()` → `emitLog`。
  - expectedConn 由调用方在持锁时捕获，避免本函数持锁期间 `d.conn` 被替换。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`StartAcquisition` 调用 `writeCommandOnly(@f0)` 错误路径
  - 持 `d.mu` 状态下检查 `errors.Is(err, protocol.ErrWatchdogTriggered)`，直接清空 `d.conn`/`d.frameReader` 并置 `status=Error`（不调用 `invalidateConnection` 避免与已持锁死锁，等价于内联毒化）。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`ApplyDaqT1603Config` 调用 `applyHardwareConfig` 错误路径
  - 不持 `d.mu` 时检查 `errors.Is(err, protocol.ErrWatchdogTriggered)` → 调用 `invalidateConnection(conn, "config apply watchdog triggered; reconnect required")`（expectedConn=conn 在持锁时捕获）。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`readLoop` defer 异常退出路径
  - `unexpectedErr != nil` 时调用 `invalidateConnectionAfterReadLoopTimeout(unexpectedErr.Error())`（内部封装 `invalidateConnection` 模式 + `onReadLoopExit` 回调）。

**根因：** 原实现 helper（`SendCommand`/`SendCommandIdle`/`SendCommandExact`）可能已 Close conn，但调用方只返回错误，`d.conn`/`d.frameReader` 和 `status.Connection` 仍可能保留已死连接引用，导致下次 `StartAcquisition` 复用爆 WSAECONNABORTED。

**独立 Close owner：** `invalidateConnection` 在锁外执行 `conn.Close()`，不依赖阻塞 I/O。expected conn 比较避免与并发 `Connect` 建立的新连接竞争。

**watchdog/soft timeout 后如何失效 conn：** 所有返回 `ErrWatchdogTriggered` 的路径统一调用 `invalidateConnection`：锁内比较 expected conn → 锁外 Close → 置 Error 状态。连接已关闭，迟到响应无法被下一命令消费。

**reader 和驱动状态：** `d.conn=nil`、`d.frameReader=nil`、`d.acquiring=false`、`d.stop=nil`、`d.readLoopDone=nil`、`d.status.Connection=Error`、`d.status.LastError` 设置。

**是否存在可选/探测读取：** 否，`invalidateConnection` 是状态失效路径，不是 I/O 操作。

**如何证明不会误杀健康连接：**
- expected conn 比较确保：若 `Connect` 已建立新连接（`d.conn=newConn`），旧 conn 触发的 `invalidateConnection(oldConn, ...)` 会因 `d.conn != oldConn` 直接 no-op，不会误清新连接。
- `invalidateConnection` 仅在 `ErrWatchdogTriggered` 或 terminal read error 时调用，这些都是异常场景，不是健康连接。

**对应测试和实际验证结果：**

- `TestDAQT1603ApplyDaqT1603Config_InvalidatesConnOnWatchdogTrigger`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:1437`）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn` + 设备正常响应配置查询命令但 `@fe BIN 1` Write 阻塞触发 watchdog。
  - 测试步骤：调用 `ApplyDaqT1603Config`。
  - 期待结果：返回 `ErrWatchdogTriggered`；`d.conn==nil`；`d.frameReader==nil`；`status.Connection==Error`；`status.LastError` 非空。
- `TestDAQT1603StartAcquisition_InvalidatesConnOnWriteCommandWatchdogTrigger`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:1503`）：
  - 验证 `@f0` 路径 watchdog 触发后 `d.conn==nil`、`status=Error`。
- expected conn 比较的回归测试由 `TestDAQT1603StartAcquisition_InvalidatesConnOnReadLoopJoinTimeout`（`daq_t1603_test.go:780`）间接覆盖：join 超时毒化后 `d.readLoopDone=nil`，下次 `StartAcquisition` 不会因残留 done 误判。

```
$ go test -race -count=1 ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/daq/hardware 19.862s
```

**遗留：** 无。所有 `ErrWatchdogTriggered` 路径统一调用 `invalidateConnection` 或等价内联毒化，expected conn 比较防止误清并发新连接。

### R0-5：WindLabX4 P1604 使用破坏性 `DrainConnection`

位置：

- `shared/device-sdk/go/protocol/conn_helpers.go`：`DrainConnection`
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`StartAcquisition`
- 同文件：`SetUnit`

现状：空缓冲是正常状态，但 `DrainConnection` 必须执行 Read 才能确认；deadline 失效时 watchdog 会关闭健康连接。现有测试明确期待该关闭行为，属于错误验收。

整改：

- WindLabX4 对齐独立 P1604，移除 Start 和 SetUnit 前的阻塞 drain。
- 依靠单 reader join、frameReader Reset、停止命令 ACK 和有界残留帧跳过完成边界恢复。
- 重新评估共享 `DrainConnection` 是否仍有合法生产调用；若无则删除或降为诊断用途，禁止用于可复用长连接。
- 删除“空缓冲 + deadline 无效 => 应 Close 健康连接”的错误测试，替换为不误杀测试。

#### 第一批整改证据（2026-07-29）

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：
  - `StartAcquisition`：移除 `sharedproto.DrainConnection` 调用，改为 `d.frameReader.Reset()` 清空应用层缓冲区。
  - `SetUnit`：移除 `sharedproto.DrainConnection` 调用，改为 `fr.Reset()`。
  - 注释说明残留字节由后续 `sendCommandACK` 调用的 `P1604ReadCommandACK` 跳帧逻辑安全跳过（`maxResidualFrameSkips=20`）。
- 对齐独立 P1604（`p1604_adapter.go StartAcquisition`）的整改模式。

**根因：** `DrainConnection` 通过 `阻塞 Read + watchdog Close` 排空 TCP 接收缓冲区，但空缓冲是正常状态，watchdog 到期只能证明探测无法完成，不能证明物理连接故障。

**独立 Close owner：** 本路径整改后**不再需要**独立 Close owner——空缓冲探测的语义是"无数据正常"，不应有任何 Close 行为。

**watchdog/soft timeout 后如何失效 conn：** 整改后 `StartAcquisition` 和 `SetUnit` 不再调用 `DrainConnection`，`frameReader.Reset()` 是纯内存操作不涉及 I/O，不会阻塞。

**reader 和驱动状态：** `frameReader` 通过 `Reset()` 清空应用层缓冲区；`d.conn`、`d.frameReader`、`d.status` 均保持不变。

**是否存在可选/探测读取：** 否，整改后 `StartAcquisition` 和 `SetUnit` 不再执行任何可选/探测读取。

**如何证明不会误杀健康连接：**

- `TestDAQP1604StartAcquisition_DoesNotCloseHealthyConnWhenNoData`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604_test.go:480`）：使用 `deadlineIgnoringConn` 模拟 deadline 失效，对端正常响应命令但缓冲区无残留数据；`StartAcquisition` 返回 nil，`d.conn` 仍非 nil，`status.Connection == ConnectionAcquiring`。
- `TestDAQP1604SetUnit_DoesNotCloseHealthyConnWhenNoData`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604_test.go:584`）：`SetUnit` 在空缓冲场景下不误杀连接。

**对应测试和实际验证结果：**

```
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     17.500s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.421s
```

**遗留：** 共享 `sharedproto.DrainConnection` 函数仍保留在 `conn_helpers.go` 中（其他项目可能引用），第三批整改时统一评估是否删除或降为诊断用途。

### R0-6：DAQP1064Pre 存在同一连接双 reader

位置：

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`readLoop`
- 同文件：`sendCommand` / `readResponseFrame`

现状：采集 read loop 与命令响应读取可同时从同一 TCP 字节流 Read，缺少唯一 reader 所有权。

风险：read loop 抢走 ACK 后命令 watchdog 会关闭物理上健康的连接；命令读取也可能吞掉采集帧并破坏对齐。

整改：采用单 reader 分发模型，或在命令期间可靠暂停并 join read loop 后再读取响应；禁止仅靠写锁解决两个 reader 的竞争。协议错位错误也应毒化连接。

#### 第一批整改证据（2026-07-29）

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：
  - 新增 `ioMu sync.Mutex` 字段：串行化 `readLoop` 的 `Read` 与 `sendCommand` 的 `Write+Read`，确保同一 TCP 字节流任意时刻只有一个 reader。
  - `readLoop`：在 `ioMu.Lock` 之前启动 `WatchdogClose`，覆盖"等 ioMu + ReadString"全区间；watchdog 触发后 `Close` conn 解除 `ReadString` 阻塞并释放 `ioMu`，避免 `sendCommand` 永久拿不到 `ioMu` 死锁。
  - `sendCommand`：在 `ioMu.Lock` 之前启动 `WatchdogClose`，覆盖"等 ioMu + Write + Read"全流程；watchdog 触发时调用 `invalidateConnection` 清空 `d.conn`、置 `status=Error`、调 `onError`。
  - `sendStartAcquisitionLocked`：在 `Write` 之前启动 `WatchdogClose`，返回 `invalidateNeeded` 标志由 `StartAcquisition` 在释放锁后调 `invalidateConnection`（避免 `*Locked` 方法持锁调 `invalidateConnection` 自死锁）。
  - 新增 `effectiveReadLoopWatchdog` 方法，暴露 `readLoopWatchdog` 字段供测试覆盖（生产 10s，测试可缩短到 100ms）。
  - `readResponseFrame`：新增响应 cmd 一致性校验，cmd 不匹配时返回 `ErrResponseCmdMismatch` 并毒化连接（R0-12 附带验收）。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre_test.go`（新增文件）：包含 `deadlineIgnoringConn` 测试替身和 8 个针对性测试。

**根因：** 原实现 `readLoop` 和 `sendCommand` 可同时 `conn.Read`，TCP 字节随机分配到两个 reader，导致命令响应被 `readLoop` 抢走（`sendCommand` 超时关闭健康连接）或采集帧被 `sendCommand` 吞掉（数据错位）。

**独立 Close owner：** `WatchdogClose(conn, timeout)` 在 `ioMu.Lock` 之前启动，是独立 owner。`time.AfterFunc` 触发时直接 `conn.Close()`，不等待阻塞 I/O，不获取 `ioMu`。

**watchdog/soft timeout 后如何失效 conn：**

- `readLoop` watchdog 触发：`wdStop()` 返回 false → `unexpectedErr = "read loop watchdog triggered"` → defer 块置 `acquiring=false`、`status.LastError`、调 `onError`。
- `sendCommand` watchdog 触发：`wdStop()` 返回 false → 调 `invalidateConnection(message)` → `d.conn=nil`、`status=Error`、`status.LastError=message`、调 `onError`。
- `sendStartAcquisitionLocked` watchdog 触发：返回 `(true, wrappedErr)` → `StartAcquisition` 释放 `d.mu` 锁后调 `invalidateConnection`。

**reader 和驱动状态：** watchdog 触发后 `d.conn` 置 nil、`d.acquiring=false`、`d.status.Connection=Error`、`d.status.LastError` 设置、`onError` 回调被调用，上层 `DeviceManager` 触发重连。

**是否存在可选/探测读取：** 否，`DAQP1064Pre` 不执行可选 ACK 或 drain 操作。

**如何证明不会误杀健康连接：**

- `TestDAQP1604StartAcquisition_DoesNotCloseHealthyConnWhenNoData`（同 R0-5）：`deadlineIgnoringConn` + 对端正常响应命令，`StartAcquisition` 成功且连接保持开放。
- `TestDAQP1064PreStartAcquisition_DoesNotStartSecondReadLoop`（`daq_p1064pre_test.go:147`）：`StartAcquisition` → `StopAcquisition` → `StartAcquisition` 序列不会启动两个 `readLoop`，`MaxActiveReads() <= 1`。

**单 reader 测试：**

- `TestDAQP1064PreStartAcquisition_DoesNotStartSecondReadLoop`：通过 `deadlineIgnoringConn.MaxActiveReads()` 断言无并发 Read（修复前 maxActive=2，修复后 maxActive=1）。

**watchdog 覆盖锁等待 + Write + Read 测试：**

- `TestDAQP1064PreSendCommand_InvalidatesConnectionOnWriteWatchdogTrigger`（`daq_p1064pre_test.go:294`）：服务端不读 → `Write` 永久阻塞 → watchdog 100ms 触发 `Close` conn → `sendCommand` 检测 `wdStop()==false` 调 `invalidateConnection`。
- `TestDAQP1064PreSendCommand_InvalidatesConnectionOnReadWatchdogTrigger`（`daq_p1064pre_test.go:356`）：服务端读命令但不发响应 → `io.ReadFull` 永久阻塞 → watchdog 100ms 触发 `Close` conn → `readResponseFrame` 包装错误 → `sendCommand` 调 `invalidateConnection`。
- `TestDAQP1064PreSendStartAcquisition_InvalidatesConnectionOnWatchdogTrigger`（`daq_p1064pre_test.go:432`）：`sendStartAcquisitionLocked` 在 `Write` 阶段 watchdog 触发后通过 `StartAcquisition` 调 `invalidateConnection`。

**read-loop join 超时不会启动第二 reader 测试：**

- `TestDAQP1064PreStopAcquisition_ReturnsWithinBudgetOnDeadlineIgnoringConn`（`daq_p1064pre_test.go:85`）：`StopAcquisition` 在 `ReadLoopJoinTimeout + 1s` 内返回 `reconnect required` 错误，`d.conn=nil`、`status=Error`。
- `TestDAQP1064PreDisconnect_JoinsReadLoop`（`daq_p1064pre_test.go:215`）：`Disconnect` join 超时调 `invalidateConnectionAfterReadLoopTimeout`，`status=Error`、`onError` 被调用。

**对应测试和实际验证结果：**

```
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     17.500s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.421s
```

**遗留：** 无。本项整改完整覆盖单 reader 模型、watchdog 覆盖锁等待/Write/Read、状态失效和 join 超时处理。

### R0-7：多个 Connect 的 Dial 阶段仍无调用方硬边界

位置：

- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217.go`：`Connect`
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`Connect`
- `projects/windlabx4/services/api-go/internal/adapters/hardware/wtn_pxi.go`：`Connect`
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go`：`Connect`

现状：前三者使用 `net.DialTimeout`，B140 使用直接 `DialContext`。Dial 尚未返回时外层没有 conn 可 Close，且部分路径持有主 mutex。

整改：统一使用修正后的 `protocol.DialTCP` 或等价的可注入 dial owner；调用方按预算返回，晚到连接可靠关闭，Connect 不在 Dial 期间持生命周期主锁。

#### 第四批整改证据（2026-07-30）

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217.go`：`Connect`
  - `net.DialTimeout("tcp", ..., DSA3217_TIMEOUT)` 改为 `sharedproto.DialTCP(..., "", DSA3217_TIMEOUT)`。
  - 复用 R1-4 整改后的 `DialTCP`（无缓冲 channel + abandoned 信号），主线程在 timeout 后立即返回错误，晚到 conn 被 Close 不泄漏。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`Connect`
  - 同样改为 `sharedproto.DialTCP(..., "", DAQ_P_1064PRE_TIMEOUT)`。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/wtn_pxi.go`：`Connect`
  - 同样改为 `sharedproto.DialTCP(..., "", WTN_PXI_TIMEOUT)`。
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go`：`Connect`
  - 保留 `DialContext`（B140 接收 ctx，保留 ctx 取消能力更自然），但用 `context.WithTimeout(ctx, b140CommandTimeout)` 包装。
  - `context.WithTimeout` 内部用 `time.AfterFunc`，触发时 close `ctx.Done()` channel——Go runtime 保证，不依赖 net 库 deadline。
  - `DialContext` 监听 `ctx.Done()`，timeout 后立即返回 `ctx.Err()`；DialContext 内部异步等待 dial 完成后 Close conn，保证晚到 conn 不泄漏（与 `sharedproto.DialTCP` 的 abandoned 信号等价）。
  - 若 ctx 已有 deadline，`WithTimeout` 自动取较短者。

**根因：** 前三者用 `net.DialTimeout`，B140 用 `DialContext(ctx)` 但 ctx 无 deadline 时 Dial 永久阻塞。Windows 故障机器 net 库 deadline 不可靠，Dial 可能永远不返回，前端"连接中"状态无法翻转。

**独立 Close owner：**
- DSA3217/DAQP1064Pre/WTN-PXI：`sharedproto.DialTCP` 内部 goroutine + `time.After` 软超时 + `abandoned` close 信号。主线程超时后 `close(abandoned)`，goroutine 走 abandoned 分支 `conn.Close()`。
- B140：`context.WithTimeout` 的 `time.AfterFunc` 是独立 owner，触发时 close `ctx.Done()`。`DialContext` 监听 `ctx.Done()` 立即返回，内部异步 Close 晚到 conn。

**watchdog/soft timeout 后如何失效 conn：**
- `sharedproto.DialTCP`：详见 R1-4 第四批整改证据。
- B140：`DialContext` 返回 `ctx.Err()` 后，Connect 返回错误，`connecting=false`、`status.LastError` 设置。晚到 conn 由 `DialContext` 内部异步 Close。

**reader 和驱动状态：** Dial 失败时不创建 conn/reader，`d.conn` 保持 nil，`status.Connection` 保持 Disconnected（DSA3217/DAQP1064Pre/WTN-PXI）或 `connecting=false` + `status.LastError` 设置（B140）。

**是否存在可选/探测读取：** 否，Dial 是连接建立操作，不是可选/探测读取。

**如何证明不会误杀健康连接：**
- 正常 Dial 在 100-300ms 内完成，`DSA3217_TIMEOUT`/`DAQ_P_1064PRE_TIMEOUT`/`WTN_PXI_TIMEOUT`/`b140CommandTimeout` 均为 5s，是 15-50 倍余量。
- `TestDialTCPNormalDial` 验证正常路径返回可用 conn（R1-4 整改证据）。
- B140 现有 `TestB140ConnectSendsServoOnAndDirectionConfig` 验证正常 Connect 流程。

**对应测试和实际验证结果：**

- `TestDialTCPReturnsAtTimeout`（`shared/device-sdk/go/protocol/conn_helpers_test.go:58`）：验证 DialTCP 在 timeout 内返回错误（R1-4 整改证据）。
- `TestDialTCP_LateArrivingConnIsClosedAfterTimeout`（同文件:142）：验证晚到 conn 被 Close 不泄漏（R1-4 整改证据）。
- DSA3217/DAQP1064Pre/WTN-PXI 改用 `sharedproto.DialTCP` 后自动继承上述保证，无需单独测试。
- B140 用 `context.WithTimeout` + `DialContext`，机制由 Go 标准库保证，依赖代码审查 + `TestB140ConnectSendsServoOnAndDirectionConfig` 回归。

```
$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/... ./shared/device-sdk/go/motion/adapters/hardware/... ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      shared.local/device-sdk/go/protocol     11.282s
ok      shared.local/device-sdk/go/daq/hardware 25.453s
ok      shared.local/device-sdk/go/motion/adapters/hardware     15.682s
ok      windlabx4/services/api-go/internal/adapters/hardware     32.312s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 14.286s

$ go vet：全部空输出
```

**遗留：** 无。DSA3217/DAQP1064Pre/WTN-PXI 三套 Connect 改用 `sharedproto.DialTCP`，B140 用 `context.WithTimeout` 包装 `DialContext`，四套 Dial 路径都有调用方硬边界，晚到 conn 必被 Close。

### R0-8：P1604 与 WindLabX4 Windows UDP Receive 可永久阻塞

位置：

- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/discovery_socket_windows.go`
- `projects/windlabx4/services/api-go/internal/adapters/scan/discovery_socket_windows.go`

现状：只设置 `SO_RCVTIMEO` 后调用同步 `windows.Recvfrom`。调用方 defer Close 与阻塞调用在同一 goroutine，无法兜底。

整改：对齐 T1603 Windows 实现，在 Recvfrom 前启动独立 `time.AfterFunc(timeout, windows.Closesocket)`；增加可观察生产 timer 触发的 Windows raw-socket seam 测试，而非仅测 `net.PacketConn` wrapper 或手工外部 Close。

#### 第一批整改证据（2026-07-29）

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/scan/discovery_socket_windows.go`：`winsockDiscoverySocket.Receive`
  - 在 `Recvfrom` 前启动独立 `time.AfterFunc(timeout, windows.Closesocket)`，watchdog 触发时直接 `Closesocket` 解除阻塞的 `Recvfrom`。
  - `Recvfrom` 返回后立即 `watchdog.Stop()`，避免 timer 泄漏。
  - 对齐 T1603 Windows 实现（`projects/daq-t1603/apps/desktop-wails/adapters/hardware/discovery_socket_windows.go`）。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/discovery_socket_windows.go`：同上模式整改。
- `projects/daq-t1603/apps/desktop-wails/adapters/hardware/discovery_socket_windows.go`：复核已对齐（C-3 标杆实现），补充 Receive 测试覆盖。
- `Receive` 返回值增加对 unexpected `Sockaddr` 类型的错误返回（防御性编程）。

**根因：** `SO_RCVTIMEO` 在某些 Windows 环境下（特别是 IOCP 模式）不可靠，`Recvfrom` 可能永久阻塞。调用方 `defer Close` 与阻塞调用在同一 goroutine，无法兜底。

**独立 Close owner：** `time.AfterFunc(timeout, windows.Closesocket)` 是独立 owner。timer 触发时直接 `Closesocket(s.handle)`，不等待阻塞 `Recvfrom`，不获取任何 mutex。

**watchdog/soft timeout 后如何失效 conn：** watchdog 触发 `Closesocket` 后，`Recvfrom` 返回 `WSAENOTSOCK`/`WSAEINTR` 错误；`socket.handle` 已失效，调用方 `Close()` 通过 `sync.Once` 幂等处理。

**reader 和驱动状态：** UDP scanner 是无状态单次操作，无 reader/驱动状态需要失效。`socket.Close()` 置 `closed` 标志，后续 `Send`/`Receive` 返回错误。

**是否存在可选/探测读取：** 否，UDP `Receive` 是有界等待操作（有 timeout 参数），不是可选/探测读取。

**如何证明不会误杀健康连接：**

- `TestDiscoverySocketReceiveReturnsAtTimeout`（`projects/windlabx4/services/api-go/internal/adapters/scan/discovery_socket_windows_test.go:13`）：`SO_RCVTIMEO` 兑现时 20ms 内返回错误，不触发 watchdog。
- `TestPacketDiscoverySocketLoopbackKeepsSocketAlive`（`projects/windlabx4/services/api-go/internal/adapters/scan/discovery_socket_test.go:119`）：真实 loopback 收发正常完成时 watchdog 不误杀 socket，第二轮收发仍成功。
- `TestPacketDiscoverySocketReceiveWatchdogClosesConn`（`discovery_socket_test.go:59`）：`fullyBlockingPacketConn`（忽略 deadline，只在 Close 后返回）模拟 deadline 失效，watchdog 50ms 触发 `Close` conn，`Receive` 在 3s 预算内返回错误。

**Windows raw Winsock 测试：**

- `TestDiscoverySocketReceiveUnblocksOnClose`（`discovery_socket_windows_test.go:40`）：30s `SO_RCVTIMEO` + 外部 `socket.Close()` 模拟 watchdog 路径，证明 `Closesocket` 能从独立 owner 解除阻塞的 `Recvfrom`。该测试是 Windows raw socket seam 的等价物——`time.AfterFunc` 触发 `Closesocket` 与外部 `Close` 走同一系统调用路径。

**对应测试和实际验证结果：**

```
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/scan/...
ok      windlabx4/services/api-go/internal/adapters/scan 4.047s

$ GOWORK=off; cd projects/daq-p1604/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-p1604/adapters/hardware     24.520s

$ GOWORK=off; cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-t1603/adapters/hardware     3.242s
```

**遗留：** C-3 提到的"生产 timer 测试证据不足"已通过 `TestPacketDiscoverySocketReceiveWatchdogClosesConn` 和 `TestDiscoverySocketReceiveUnblocksOnClose` 双重覆盖解决——前者验证 `time.AfterFunc` 触发 `conn.Close()`（PacketConn 路径），后者验证 `Closesocket` 解除阻塞 `Recvfrom`（Winsock 路径）。

### R0-9：三套 UDP discovery Send 可永久阻塞

位置：三套 `discovery_socket.go` 与 `discovery_socket_windows.go` 的 `Send`。

现状：`WriteTo`/`windows.Sendto` 无独立 Close owner，Receive watchdog 尚未启动。若 Send 阻塞，扫描 goroutine 的 defer Close 无法执行。

整改：对单次 scanner socket 建立覆盖 Send + Receive 整个生命周期的 owner timer，或给 Send 单独 Close watchdog；超时后 socket 直接废弃。

#### 第一批整改证据（2026-07-29）

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/scan/discovery_socket.go`：`packetDiscoverySocket.Send`
  - 在 `WriteTo` 前启动独立 `time.AfterFunc(discoverySendTimeout, conn.Close)`，覆盖 Send 阶段。
  - `WriteTo` 返回后立即 `watchdog.Stop()`，避免 timer 泄漏。
- `projects/windlabx4/services/api-go/internal/adapters/scan/discovery_socket_windows.go`：`winsockDiscoverySocket.Send`
  - 新增 `soSNDTIMEO = 0x1005` 常量（Winsock2 `SO_SNDTIMEO` 原始值，`golang.org/x/sys/windows` 未导出）。
  - 在 `Sendto` 前设置 `SO_SNDTIMEO` 软超时 + 启动独立 `time.AfterFunc(discoverySendTimeout, windows.Closesocket)` watchdog。
  - `Sendto` 返回后立即 `watchdog.Stop()`。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/discovery_socket.go` 和 `discovery_socket_windows.go`：同上模式整改。
- `projects/daq-t1603/apps/desktop-wails/adapters/hardware/discovery_socket.go` 和 `discovery_socket_windows.go`：同上模式整改。
- `discoverySendTimeout` 常量在三套项目中统一为 `2 * time.Second`（跨项目一致性）。

**根因：** UDP `WriteTo`/`Sendto` 通常即时返回（内核缓冲吸收），但在以下场景可能阻塞：
1. 发送缓冲区满（高频率发送）；
2. 路由/ARP 解析阻塞（特定 Windows 网络 stack）；
3. `SO_SNDTIMEO` 不生效（与 `SO_RCVTIMEO` 同样的 Windows 内核问题）。

**独立 Close owner：** `time.AfterFunc(discoverySendTimeout, conn.Close)` / `time.AfterFunc(discoverySendTimeout, windows.Closesocket)` 是独立 owner。timer 触发时直接 Close，不等待阻塞 `WriteTo`/`Sendto`。

**watchdog/soft timeout 后如何失效 conn：** watchdog 触发 `Close`/`Closesocket` 后，`WriteTo`/`Sendto` 返回错误；socket 已废弃，后续 `Send`/`Receive` 返回错误。

**reader 和驱动状态：** UDP scanner 是无状态单次操作，无 reader/驱动状态需要失效。

**是否存在可选/探测读取：** 否，UDP `Send` 是有界操作，不是可选/探测读取。

**如何证明不会误杀健康连接：**

- `TestPacketDiscoverySocketSendWatchdogClosesConn`（`projects/windlabx4/services/api-go/internal/adapters/scan/discovery_socket_test.go:88`）：`fullyBlockingPacketConn`（`WriteTo` 只在 Close 后返回）模拟 Send 永久阻塞，watchdog 在 `discoverySendTimeout` 预算内 `Close` conn，`Send` 返回错误。
- `TestDiscoverySocketSendReturnsAtTimeout`（`discovery_socket_windows_test.go:72`）：向 `192.0.2.1`（TEST-NET-1，无路由）发送，`Send` 在 3s 预算内返回（`SO_SNDTIMEO` + watchdog 均未阻塞）。
- `TestPacketDiscoverySocketLoopbackKeepsSocketAlive`（同 R0-8）：真实 loopback 收发正常完成时 watchdog 不误杀 socket。

**对应测试和实际验证结果：**

```
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/scan/...
ok      windlabx4/services/api-go/internal/adapters/scan 4.047s

$ GOWORK=off; cd projects/daq-p1604/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-p1604/adapters/hardware     24.520s

$ GOWORK=off; cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-t1603/adapters/hardware     3.242s
```

**遗留：** 无。三套 UDP discovery 的 Send 路径均已有独立 Close owner 和针对性测试。

### R0-10：多套采集 read loop 的 no-data 检测仍依赖 deadline 返回

位置：

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`readLoop`
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`readLoop`
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`readLoop`
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`readLoop`
- `projects/windlabx4/services/api-go/internal/adapters/hardware/wtn_pxi.go`：`readLoop`

现状：这些循环通过短 `SetReadDeadline` 让阻塞 Read 周期返回，再在循环体累计 timeout 或检查 no-data 总时长。问题机忽略 deadline 时，循环体无法重新获得控制权，无数据检测逻辑不可达。Stop/Disconnect 被用户调用后通常能 Close 解阻塞，但无人操作的半开连接无法自行收敛。

整改：为采集周期建立不依赖 read goroutine 与其 mutex 的 no-data owner；每次收到完整有效帧时安全续期，到期 Close expected conn、失效驱动状态并通知上层。必须与 Stop/Disconnect 的主动关闭原因区分，并避免 timer 与重连后的新 conn 竞争。DAQP1064Pre 还必须与 R0-6 的单 reader 改造一并设计。

#### 第二批整改证据（2026-07-30）

**修改文件与符号：**

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`readLoop`
  - `noDataTimeout` 从 const 改为 var（10s 默认），允许测试注入短超时加速。
  - readLoop 入口启动独立 `time.AfterFunc(noDataTimeout, ...)` timer，回调内 `acquiring` + `expectedConn` 双重检查后毒化连接（清 `d.conn=nil` / `d.frameReader=nil` / `d.acquiring=false` / `d.status.Connection=ConnectionError` / `d.status.LastError`），锁外 `conn.Close()`。
  - 移除原 `lastDataAt` 跟踪 + 循环体 `time.Since(lastDataAt) > noDataTimeout` 检测逻辑。
  - 每次收到有效数据（n > 0）调用 `noDataTimer.Reset(noDataTimeout)` 续期；readLoop 退出 `defer noDataTimer.Stop()`。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`readLoop`
  - `noDataTimeout` 从 const 改为 var（10s 默认），包级共享（DAQP1604 / DAQP1064Pre / WTNPXI 三驱动复用）。
  - 移除 `lastDataAt` 跟踪 + `consecutiveTimeouts` 计数器，改用独立 `time.AfterFunc` timer。
  - 回调内 `acquiring` + `expectedConn` 双重检查，避免 Stop 后或重连后误触发。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`readLoop`
  - 复用包级 `noDataTimeout`（10s 默认），与 `effectiveReadLoopWatchdog()`（readLoopWatchdog）解耦——避免共享超时导致同时触发后 readLoopWatchdog 覆盖 noDataTimer 设置的 LastError。
  - readLoop 入口启动独立 `time.AfterFunc(noDataTimeout, ...)` timer，回调内 `acquiring` + `expectedConn` 双重检查后毒化连接。
  - 每次收到有效数据调用 `noDataTimer.Reset(noDataTimeout)` 续期。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/wtn_pxi.go`：`readLoop`
  - `wtnPXINoDataTimeout` 从 const 改为 var（10s 默认）。
  - readLoop 入口启动独立 `time.AfterFunc(wtnPXINoDataTimeout, ...)` timer，回调内 `acquiring` + `expectedConn` 双重检查后毒化连接。
  - 每次收到有效数据调用 `noDataTimer.Reset(wtnPXINoDataTimeout)` 续期。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`readLoop`
  - `p1604NoDataTimeout` 从 const 改为 var（5s 默认，对齐原 `consecutiveTimeouts` 阈值 25 × 200ms = 5s）。
  - 移除 `consecutiveTimeouts` 计数器，改用独立 `time.AfterFunc` timer。
  - 回调内通过 `handleConnectionLost` 的 driver 身份校验避免与重连后的新 driver 竞争。
  - 每次收到有效数据调用 `noDataTimer.Reset(p1604NoDataTimeout)` 续期。
- `shared/device-sdk/go/protocol/conn_helpers.go`：`IsClosedConnError`
  - 新增 `io.ErrClosedPipe` 识别：net.Pipe Close 后 Read 返回该错误，单元测试大量使用 net.Pipe 模拟双向连接，noDataTimer/Disconnect Close 后 readLoop Read 会收到 io.ErrClosedPipe。若不识别，readLoop defer 会误走 invalidate 路径覆盖 timer 设置的 LastError。

**根因：** 原实现 readLoop 通过短 `SetReadDeadline`（100~200ms）让 Read 周期返回，在循环体累计 `time.Since(lastDataAt) > noDataTimeout` 或 `consecutiveTimeouts` 阈值检测无数据。问题 Windows 电脑 deadline 失效时循环体不可达，no-data 检测永远不会触发，半开连接（设备断电/网线脱落但 TCP 连接仍存活）无法自行收敛。

**独立 Close owner：** 每个驱动的 noDataTimer 是 `time.AfterFunc` 独立 goroutine，不依赖 readLoop 循环体执行，即使 Read 永久阻塞也能到期触发。回调内锁外 `conn.Close()` 解除 readLoop 的 Read 阻塞，不获取 `writeMu` / `ioMu`。

**timer 与重连竞争：** 回调内 `expectedConn` 指针比较确保 timer 只作用于其创建时的连接——Stop/Disconnect 已置 `d.conn=nil` 或重连后 `d.conn` 是新连接时，`currentConn != expectedConn` 直接跳过。`acquiring` 检查避免 Stop 后 readLoop 尚未退出时 timer 误触发。

**timer 与 Stop 主动停止区分：** Stop/Disconnect 在 `close(stop)` 前置 `acquiring=false`（stopAcquisitionLocked）或 `SetStopReason(UserRequested)`，timer 回调检测到 `acquiring=false` 直接跳过；readLoop 检测到 `IsClosedConnError` 或 `StopReason==UserRequested` 静默退出，不触发 defer 的 invalidate 路径。

**如何证明不会误杀健康连接：**
- 健康采集时设备每秒推送数百~数千帧，noDataTimer 每次收到数据都 Reset，10s/5s 阈值永远不会触发。
- 健康连接的瞬态 Read timeout（100~200ms deadline 到期）走 continue 路径，不重置 noDataTimer（因为 n==0，未收到数据）——但瞬态 timeout 不会持续 10s，所以 noDataTimer 不会误触发。
- 现有测试 `TestDAQT1603StopAcquisition_DoesNotCloseHealthyConnWhenNoData` / `TestDAQP1604StartAcquisition_DoesNotCloseHealthyConnWhenNoData` 等已验证健康连接在空缓冲场景下不会被误杀。

**新增测试用例（覆盖 deadline 失效场景）：**

- `TestDAQT1603ReadLoop_InvalidatesConnOnNoDataTimeout`：deadlineIgnoringConn 让 Read 永久阻塞，验证 noDataTimer 到期后 d.conn=nil / status=Error / LastError 含 "no data" / onReadLoopExit 被调用。
- `TestDAQP1604ReadLoop_InvalidatesConnOnNoDataTimeout`：同上，验证 WindLabX4 P1604。
- `TestDAQP1064PreReadLoop_InvalidatesConnOnNoDataTimeout`：同上，验证 DAQP1064Pre。noDataTimer 与 readLoopWatchdog 解耦后，readLoop 静默退出（IsClosedConnError 识别 io.ErrClosedPipe），defer 不调 invalidate，onError 不被调用。
- `TestWTNPXIReadLoop_InvalidatesConnOnNoDataTimeout`：同上，验证 WTN-PXI。
- `TestP1604ReadLoop_InvalidatesConnOnNoDataTimeout`：同上，验证独立 P1604。timer 触发后 handleConnectionLost 清理 driver + 设置 status=Error。

**验证命令与结果：**

```
$ go test ./shared/device-sdk/go/daq/hardware/... -run NoData -v
--- PASS: TestDAQT1603ReadLoop_InvalidatesConnOnNoDataTimeout (0.30s)
--- PASS: TestDAQT1603StopAcquisition_DoesNotCloseHealthyConnWhenNoData (0.02s)
--- PASS: TestDAQT1603ApplyDaqT1603Config_DoesNotCloseHealthyConnWhenNoData (0.00s)
ok      shared.local/device-sdk/go/daq/hardware 5.928s

$ cd projects/windlabx4/services/api-go; go test ./internal/adapters/hardware/... -run NoData -v
--- PASS: TestDAQP1064PreReadLoop_InvalidatesConnOnNoDataTimeout (0.30s)
--- PASS: TestDAQP1604StartAcquisition_DoesNotCloseHealthyConnWhenNoData (1.00s)
--- PASS: TestDAQP1604SetUnit_DoesNotCloseHealthyConnWhenNoData (0.02s)
--- PASS: TestDAQP1604ReadLoop_InvalidatesConnOnNoDataTimeout (0.30s)
--- PASS: TestWTNPXIReadLoop_InvalidatesConnOnNoDataTimeout (0.30s)
ok      windlabx4/services/api-go/internal/adapters/hardware 7.524s

$ cd projects/daq-p1604/apps/desktop-wails; GOWORK=off; go test ./adapters/hardware/... -run NoData -v
--- PASS: TestP1604ReadLoop_InvalidatesConnOnNoDataTimeout (0.30s)
ok      daq-p1604/adapters/hardware 5.945s
```

**完整测试套件无回归：**

- `go test ./shared/device-sdk/go/...`：全部 ok（daq/hardware 16.388s, protocol 7.234s 等）
- `cd projects/windlabx4/services/api-go; go test ./internal/...`：全部 ok（adapters/hardware 32.936s, usecase 102.435s 等）
- `cd projects/daq-p1604/apps/desktop-wails; GOWORK=off; go test ./...`：全部 ok（adapters/hardware 29.270s, backend 12.961s 等）
- `go vet`：三处均无 warning。

**遗留：** 无。五套采集 read-loop（T1603 / WindLabX4 P1604 / 独立 P1604 / DAQP1064Pre / WTN-PXI）均已有独立 no-data owner，不依赖循环体执行，deadline 失效场景下也能到期触发连接毒化。

### R0-11：T1603 与 WindLabX4 P1604 异常 read 退出保留失效连接

位置：

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`readLoop` defer
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`readLoop` defer

现状：T1603 在异常 read 后可能把 ConnectionAcquiring 恢复为 Connected，且不清 conn/frameReader；WindLabX4 P1604 标记 Error 但仍保留并未统一 Close 的 conn/frameReader。EOF、RST、协议错误或 no-data 退出后，后续操作仍可能复用失效或字节边界不确定的连接。

整改：所有非主动停止的 terminal read error 统一走 expected-conn invalidation：清 conn/reader、停止采集、标记 Error、保存 LastError、Close 旧连接并通知上层。回调不得承担底层状态正确性的唯一责任。

#### 第二批整改证据（2026-07-30）

**修改文件与符号：**

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`readLoop` defer
  - 在 defer 入口先缓存 `d.readLoopDone` 到局部变量 `done`（避免 invalidate 清空字段后 defer 末尾 close 操作读到 nil）。
  - 移除原"status 从 Acquiring 改回 Connected"的错误逻辑，改调用 `d.invalidateConnectionAfterReadLoopTimeout(unexpectedErr.Error())` 统一毒化：清 `d.conn=nil` / `d.frameReader=nil` / `d.acquiring=false` / `d.stop=nil` / `d.readLoopDone=nil` / `d.status.Connection=ConnectionError` / `d.status.LastError=message`，并在锁外 `conn.Close()`。
  - `onReadLoopExit` 回调在 invalidate 之后显式调用，调用方读取到的 status 已是 Error。
  - 保留原 `@f1` best-effort Write + watchdog 兜底（连接未死时通知设备停止推送，连接已死时 watchdog 1s 兜底解除 Write 阻塞）。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`readLoop` defer
  - 移除手写的状态清理逻辑（`d.acquiring=false` / `d.status.Acquiring=false` / `d.status.LastError=...` / `d.status.Connection=ConnectionError` / 缓存并调用 `onError`），改调用 `d.invalidateConnection(unexpectedErr.Error())` 统一毒化：清 `d.conn=nil` / `d.frameReader=nil` / `d.acquiring=false` / `d.status.Acquiring=false` / `d.status.Connection=ConnectionError` / `d.status.LastError=message`，锁外 `conn.Close()` 并调用 `onError`。
  - `d.stop` 在 invalidate 之前显式置 nil（invalidate 不清 stop 字段，对齐原 defer 行为避免 readLoop 退出后 stop channel 残留）。
  - 保留主动停止场景（`StopReasonUserRequested`）的早 return，不触发 invalidate。

**根因：** 原实现 T1603 readLoop defer 在 unexpectedErr != nil 时仅把 status 从 Acquiring 改回 Connected，未清空 conn/frameReader 也未 close conn；P1604 readLoop defer 虽设置 status=Error 但同样未清空 conn/frameReader 也未 close conn。EOF/RST/协议错误后连接已死，下次 StartAcquisition 会用旧 conn 发命令爆 WSAECONNABORTED，且 driver 内部残留 frameReader 仍可被错误复用。

**独立 Close owner：** 两处 defer 调用的 invalidate 函数（T1603 的 `invalidateConnectionAfterReadLoopTimeout` / P1604 的 `invalidateConnection`）均在 `d.mu.Lock` 内取出 conn 引用并清空字段后，锁外执行 `conn.Close()`，不依赖阻塞 I/O，不获取 `writeMu`。

**reader 和驱动状态：** 两处 defer 执行后 driver 状态统一为：`d.conn=nil`、`d.frameReader=nil`、`d.acquiring=false`、`d.status.Acquiring=false`、`d.status.Connection=Error`、`d.status.LastError` 设置、上层回调（`onReadLoopExit` / `onError`）被调用。

**是否存在可选/探测读取：** 否。terminal read error 是 EOF/RST/协议错误等不可恢复错误，不是可选 ACK 探测。主动停止场景（`StopReasonUserRequested` 或 `stop` channel 关闭）由 `isClosedConnError` 或 `GetStopReason` 判定后早 return，不触发 invalidate，不会误杀主动停止路径。

**如何证明不会误杀健康连接：**
- T1603 readLoop 仅在 `unexpectedErr != nil` 时触发 invalidate，unexpectedErr 仅在以下场景被赋值：
  - `noDataTimeout`（10s 内无数据）——已通过 `noDataTimeout` 阈值区分瞬态 timeout 与 terminal 错误，仅 10s 持续无数据才视为 terminal；
  - 非 `ErrIncompleteFrame`、非 `net.Error.Timeout()`、非 `isClosedConnError` 的 Read 错误（EOF/RST 等）。
- P1604 readLoop 同样仅 `unexpectedErr != nil` 且 `StopReason != UserRequested` 时触发 invalidate。
- 健康连接的瞬态 timeout（200ms Read deadline 到期）走 continue 路径，不设置 unexpectedErr，不触发 defer 的 invalidate 分支。
- 现有测试 `TestDAQT1603StopAcquisition_DoesNotCloseHealthyConnWhenNoData` / `TestDAQP1604StartAcquisition_DoesNotCloseHealthyConnWhenNoData` 等已验证健康连接在空缓冲场景下不会被误杀。

**对应测试和实际验证结果：**

- `TestDAQT1603ReadLoop_InvalidatesConnOnTerminalReadError`（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go:910`）：
  - 测试前置：`net.Pipe` 建立双向连接；device.conn / frameReader 已设置，acquiring=true；启动 readLoop goroutine。
  - 测试步骤：关闭 server 端模拟对端 EOF（client.Read 返回 io.EOF）。
  - 期待结果：`d.conn==nil`、`d.frameReader==nil`、`status.Connection==Error`、`status.LastError` 非空、`onReadLoopExit` 回调被调用并收到非 nil error。
- `TestDAQP1604ReadLoop_InvalidatesConnOnTerminalReadError`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604_test.go:703`）：
  - 测试前置：`net.Pipe` 建立双向连接；d.conn / frameReader 已设置，acquiring=true；启动 readLoop goroutine。
  - 测试步骤：关闭 server 端模拟对端 EOF。
  - 期待结果：`d.conn==nil`、`d.frameReader==nil`、`status.Connection==Error`、`status.LastError` 非空、`onError` 回调被调用并收到非 nil error。

```
$ go test -race -count=1 ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/daq/hardware 11.512s

$ go test -race -count=1 ./projects/windlabx4/services/api-go/...
ok      windlabx4/services/api-go/internal/adapters/hardware     17.684s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.491s
... (全部子包通过)

$ GOWORK=off go test -race -count=1 ./projects/daq-t1603/apps/desktop-wails/...
ok      daq-t1603/adapters/hardware     3.238s
... (全部子包通过)
```

### R0-12：命令响应 soft timeout 后连接仍被复用

位置：横跨所有命令-响应实现，重点包括：

- `shared/device-sdk/go/protocol/daq_t1603_frame.go` 的 T1603 命令 helper
- `shared/device-sdk/go/protocol/conn_helpers.go` 的 P1604 ACK helper
- `shared/device-sdk/go/protocol/daq_p1604_unit.go` 的单位 helper
- WindLabX4 P1604、DAQP1064Pre、DSA3217 的命令发送路径

现状：当前整改主要识别 `ErrWatchdogTriggered`。如果 OS soft deadline 正常先于 watchdog 返回，helper 会停止 watchdog 并返回 timeout，但迟到响应仍可能随后进入 TCP 流，被下一条命令或采集 reader 消费。该连接的协议边界已经不确定，不应继续复用。

整改：所有命令-响应操作一旦在完整响应边界前 timeout、取消、短读或中途失败，无论是 soft deadline 还是 hard watchdog 触发，都必须毒化 expected conn：Close、清除 reader、标记 Error 并要求重连。只有设备明确支持 request ID 且单 reader 能安全丢弃迟到响应时，才允许例外，并需单独证明。

验收：构造 soft deadline 正常返回、watchdog 尚未触发、设备随后发送迟到 ACK 的场景；断言旧连接已失效，迟到 ACK 不会被下一命令消费。

#### 第一批附带整改证据（DAQP1064Pre 路径，2026-07-29）

**说明：** R0-12 属于第三批整改范围，本批仅附带完成 DAQP1064Pre 路径的验证（R0-6 单 reader 改造时自然延伸）。其他路径（T1603 helper、P1604 ACK helper、P1604 unit helper、DSA3217）仍需在第三批整改。

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：
  - `readResponseFrame`：新增响应 cmd 一致性校验。读取响应帧 header 后，检查 `respCmd != expectedCmd`，不匹配时返回 `ErrResponseCmdMismatch` 并包装 `protocol error` 上下文。
  - `sendCommand`：检测 `readResponseFrame` 返回的错误（包括 cmd mismatch）后，调用 `invalidateConnection` 毒化连接：`d.conn=nil`、`status=Error`、`status.LastError` 包含 `protocol error` 上下文、调 `onError`。
  - 与 watchdog 触发路径共享同一 `invalidateConnection` 函数，确保协议边界不确定时统一失效。

**根因：** 原实现 `sendCommand` 只在 watchdog 触发时毒化连接。若 soft deadline 正常先于 watchdog 返回（OS 兑现 deadline），或响应帧 cmd 与请求不匹配（迟到响应/采集帧串入/帧错位），helper 返回普通错误，但迟到响应仍可能进入 TCP 流被下一命令消费——协议边界已不可信。

**独立 Close owner：** `invalidateConnection` 在锁外执行 `conn.Close()`，不依赖阻塞 I/O，不获取 `ioMu`。

**watchdog/soft timeout 后如何失效 conn：** cmd mismatch 时 `invalidateConnection` 内部 `d.mu.Lock` → `conn = d.conn; d.conn = nil; d.status.Connection = Error; d.status.LastError = message` → `d.mu.Unlock` → `conn.Close()` → `onError`。连接已关闭，迟到响应无法被下一命令消费。

**reader 和驱动状态：** `d.conn=nil`、`d.frameReader` 不再使用（连接已毒化）、`d.status.Connection=Error`、`d.status.LastError` 设置、`onError` 回调被调用。

**是否存在可选/探测读取：** 否，cmd mismatch 是协议错误，不是可选读取。

**如何证明不会误杀健康连接：**

- cmd mismatch 不是"健康连接"——响应 cmd 与请求不匹配说明协议边界已错位（迟到响应/采集帧串入/帧错位），连接不可信。本测试验证的是"协议错误时必须毒化连接"，不是"健康连接不应被关闭"。

**对应测试和实际验证结果：**

- `TestDAQP1064PreSendCommand_InvalidatesConnectionOnResponseCmdMismatch`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre_test.go:506`）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn`（确保 `SetReadDeadline` 不先返回，让 `io.ReadFull` 必然读到完整 6 字节 header）。
  - 测试步骤：服务端收到命令后回复 cmd=0xFF（与请求 0x03 不匹配）。
  - 期待结果：`sendCommand` 返回 `cmd mismatch` 错误；`d.conn == nil`；`status.Connection == Error`；`status.LastError` 非空；`onError` 被调用。

```
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     17.500s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.421s
```

#### 第三批整改证据（2026-07-30）

**修改文件与符号：**

- `shared/device-sdk/go/protocol/daq_t1603_frame.go`：`SendCommand` / `SendCommandIdle` / `SendCommandExact`
  - 新增 `softTimeoutTriggered` 标记：`net.Error.Timeout()` 命中时强制 `conn.Close()` 阻断迟到响应，并包装 `ErrWatchdogTriggered` sentinel 返回。
  - watchdog 触发与 soft timeout 走同一返回路径，调用方只需 `errors.Is(err, ErrWatchdogTriggered)` 即可统一毒化。
- `shared/device-sdk/go/protocol/conn_helpers.go`：`P1604ReadCommandACK`
  - 在 ack 读取阶段检测 `net.Error.Timeout()`，命中时强制 `conn.Close()` 并包装 `ErrWatchdogTriggered`。
  - 总预算耗尽（跳帧达到上限）同样视为 soft timeout 走毒化路径。
- `shared/device-sdk/go/protocol/daq_p1604_unit.go`：`P1604ReadUnitCoefficient` / `P1604WriteUnitCoefficient`
  - Read/Write 阶段任一 soft deadline 触发都强制 `conn.Close()` 并返回 `ErrWatchdogTriggered`。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217.go`：`sendCommand`
  - 暴露 `cmdSoftTimeout` 字段供测试注入短超时（默认 `cmdTimeout`，测试可设 50ms）。
  - Write/Read 任一阶段 soft deadline 触发都调用 `invalidateConnection` 毒化连接，返回 `ErrWatchdogTriggered`。
  - `invalidateConnection` 内部 `d.mu.Lock` → 清 conn/reader/stop + Error 状态 → `conn.Close()` → `onError`，与 watchdog 触发路径共享同一失效函数。

**根因：** soft deadline 兑现（OS 正常返回 timeout）时 helper 仅停止 watchdog 并返回普通 timeout 错误，但迟到响应仍可能随后进入 TCP 流被下一条命令消费——协议边界已不可信。原实现只识别 watchdog 触发，未覆盖 soft timeout 路径。

**独立 Close owner：** soft timeout 命中时 helper 自身 `conn.Close()`，是同步操作不需要独立 owner；调用方根据返回的 `ErrWatchdogTriggered` sentinel 在锁外执行 `invalidateConnection` 清理驱动状态。

**watchdog/soft timeout 后如何失效 conn：**
- T1603/P1604 helper：返回 `ErrWatchdogTriggered` → 调用方 `errors.Is` 识别 → 调用 `invalidateConnection` 锁外清 conn/reader、置 Error、Close conn。
- DSA3217 `sendCommand` 内部直接调 `invalidateConnection`（因 helper 与驱动同包，无 sentinel 跨包边界）。

**reader 和驱动状态：** 触发后 `d.conn=nil`、`d.frameReader=nil`（如适用）、`d.acquiring=false`、`d.stop=nil`、`d.status.Connection=Error`、`d.status.LastError` 设置。

**是否存在可选/探测读取：** 否，soft timeout 发生在期待响应的命令路径上，不是无数据正常的探测。

**如何证明不会误杀健康连接：**
- soft timeout 阈值与 watchdog 阈值一致（如 T1603 `cmdTimeout=2s` + `cmdWatchdogTimeout=5s`），正常响应在 100ms 内完成，2s 是 20 倍余量。
- 测试用 `deadlineIgnoringConn`（让 `SetReadDeadline` 兑现）+ 服务端延迟响应，让 soft deadline 先于 watchdog 触发——这是异常场景，不是健康连接。

**对应测试和实际验证结果：**

- `TestT1603SendCommand_SoftTimeoutClosesConnAndReturnsSentinel`（`shared/device-sdk/go/protocol/daq_t1603_frame_test.go:1111`）：
  - 测试前置：`net.Pipe` + 服务端先 Read 命令再写单字节 'X'（非 '\n'，触发 `cmdTailTimeout` soft deadline）。
  - 测试步骤：调用 `SendCommand(client, "@fd MCH")`，等待 soft timeout。
  - 期待结果：返回错误包装 `ErrWatchdogTriggered`；后续 `server.Write` 失败（conn 已 Close）阻断迟到响应。
- `TestT1603SendCommandIdle_SoftTimeoutClosesConnAndReturnsSentinel`（同文件:1168）：
  - 验证 `SendCommandIdle` 首字节 `cmdTimeout` soft 触发同样返回 sentinel 并 Close conn。
- `TestT1603SendCommandExact_SoftTimeoutClosesConnAndReturnsSentinel`（同文件:1221）：
  - 验证 `SendCommandExact` 的 `io.ReadFull` 中途 soft deadline 触发同样返回 sentinel 并 Close conn。
- `TestP1604ReadCommandACK_SoftTimeoutClosesConnAndReturnsSentinel`（`shared/device-sdk/go/protocol/conn_helpers_test.go:887`）：
  - 验证 ack 读取阶段 `net.Error.Timeout()` 命中时 Close conn + 返回 sentinel。
- `TestP1604ReadCommandACK_TotalBudgetExhaustionTriggersSoftTimeout`（同文件:931）：
  - 验证总预算耗尽（跳帧达到上限）走同一 soft timeout 毒化路径。
- `TestP1604ReadUnitCoefficient_SoftTimeoutClosesConnAndReturnsSentinel`（`shared/device-sdk/go/protocol/daq_p1604_unit_test.go:511`）：
  - 验证 `P1604ReadUnitCoefficient` soft timeout Close conn + 返回 sentinel。
- `TestP1604WriteUnitCoefficient_SoftTimeoutClosesConnAndReturnsSentinel`（同文件:569）：
  - 验证 `P1604WriteUnitCoefficient` soft timeout Close conn + 返回 sentinel。
- `TestDSA3217SendCommand_SoftTimeoutInvalidatesConn`（`projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217_test.go:402`）：
  - 测试前置：注入 `cmdSoftTimeout=50ms`（让 soft deadline 先于 watchdog 触发）。
  - 测试步骤：调用 `sendCommand`，服务端不响应。
  - 期待结果：返回错误包装 `ErrWatchdogTriggered`；`d.conn==nil`；`status.Connection==Error`；`onError` 被调用。

```
$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/protocol     11.072s
ok      shared.local/device-sdk/go/daq/hardware 19.859s

$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     19.636s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.404s

$ go vet + go build：全部空输出
```

**遗留：** 无。T1603 / P1604 ACK / P1604 unit / DSA3217 四类命令-响应路径的 soft timeout 毒化全部覆盖，迟到响应被 Close conn 阻断，不会污染下一条命令。

## P1：高风险一致性整改

### R1-1：P1604 helper Close 后调用方未统一失效

位置：

- `shared/device-sdk/go/protocol/daq_p1604_unit.go`
- WindLabX4 `DAQP1604.SetUnit`
- 独立应用 `P1604Adapter.ApplyConfig`

现状：单位 helper 的 watchdog 可 Close conn，但调用方主要只检查 peer reset，没有统一处理 `ErrWatchdogTriggered`；已关闭连接可能继续注册为 Connected。

整改：所有 helper 返回 watchdog sentinel；调用方用 `errors.Is` 后立即执行 expected-conn invalidation。禁止依赖错误字符串和只匹配 FIN/RST。

#### 第三批整改证据（2026-07-30）

**修改文件与符号：**

- `shared/device-sdk/go/protocol/daq_p1604_unit.go`：`P1604ReadUnitCoefficient` / `P1604WriteUnitCoefficient`
  - soft timeout / watchdog 触发时已统一返回 `ErrWatchdogTriggered` sentinel（详见 R0-12 第三批整改证据）。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`DAQP1604.SetUnit`
  - 调用 `P1604ReadUnitCoefficient` / `P1604WriteUnitCoefficient` 后，检测 `errors.Is(err, sharedproto.ErrWatchdogTriggered)` 或 `IsConnResetByPeer(err)`，命中时调用 `invalidateConnection` 毒化连接（清 conn/reader + Error 状态 + `onError`）。
  - 同时处理 `v01101` 命令路径的 EOF/RST，复用同一 `invalidateConnection`，避免已关闭连接继续注册为 Connected。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`sendCommandACK`
  - Write/Read 阶段 watchdog 触发时调用 `invalidateConnection`，返回 `ErrWatchdogTriggered`。
  - Write 成功但 watchdog 触发（conn 在 Write 后被 Close）同样走 `invalidateConnection`，避免状态不一致。
  - 不调 `onError`：让 readLoop defer 的 `unexpectedErr` 路径统一调用 `invalidateConnection`，避免双重调用。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`ApplyConfig`
  - `v01101` 命令路径检测 `errors.Is(err, sharedproto.ErrWatchdogTriggered)` 或 `IsConnectionFault(err)`，命中时调用 `handleConnectionLost` 清理 driver + conn，避免后续 `StartAcquisition` 的 `c 00` 命令爆 `WSAECONNABORTED`。
  - `handleConnectionLost` 与 `ZeroCalibration` / `StartAcquisition` 共享同一清理路径，保证状态最终一致。

**根因：** 原实现调用方主要只检查 peer reset（FIN/RST），未统一处理 `ErrWatchdogTriggered`。helper 已 Close conn 但调用方未清驱动状态，导致已关闭连接继续注册为 Connected，下次命令爆 `WSAECONNABORTED`。

**独立 Close owner：** helper 内部 `conn.Close()` 是同步操作；调用方 `invalidateConnection` / `handleConnectionLost` 在锁外清驱动状态，不依赖阻塞 I/O。

**watchdog/soft timeout 后如何失效 conn：** `invalidateConnection` 内部 `d.mu.Lock` → `conn = d.conn; d.conn = nil; d.frameReader = nil; d.status.Connection = Error; d.status.LastError = message` → `d.mu.Unlock` → `conn.Close()` → `onError`。`handleConnectionLost` 内部 `shard.drivers[id]==driver` 检查后删除 driver + Close conn + 通知前端。

**reader 和驱动状态：** `d.conn=nil`、`d.frameReader=nil`（如适用）、`d.status.Connection=Error`、`d.status.LastError` 设置、`onError` 回调被调用（独立应用为前端通知）。

**是否存在可选/探测读取：** 否，`SetUnit` / `ApplyConfig` / `sendCommandACK` 都是期待响应的命令路径。

**如何证明不会误杀健康连接：**
- `TestDAQP1604SetUnit_DoesNotCloseHealthyConnWhenNoData`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604_test.go:584`）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn` + 设备正常响应 `v01101` 但不响应单位查询（模拟无数据）。
  - 测试步骤：调用 `SetUnit("Pa")`。
  - 期待结果：返回错误；`d.conn != nil`（健康连接未被关闭，仅 timeout）；`status.Connection != Error`。
  - 说明：本测试证明 soft timeout 毒化只针对"协议边界已不可信"的场景，不会因设备短暂无响应关闭健康连接。

**对应测试和实际验证结果：**

- `TestDAQP1604_SetUnit_V01101EOFTriggersOnError`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604_test.go:133`）：
  - 测试前置：`net.Pipe` + 服务端收到 `v01101` 后 Close conn（模拟 EOF）。
  - 测试步骤：调用 `SetUnit("Pa")`。
  - 期待结果：返回错误；`d.conn == nil`；`status.Connection == Error`；`onError` 被调用。
- `TestDAQP1604SendCommandACK_InvalidatesConnectionOnWatchdogTrigger`（同文件:340）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn`（让 Write 永久阻塞触发 watchdog）。
  - 测试步骤：调用 `sendCommandACK("h")`。
  - 期待结果：返回 `ErrWatchdogTriggered`；`d.conn == nil`；`status.Connection == Error`。
- `TestDAQP1604SendCommandACK_WatchdogTriggersOnWriteDeadlineIgnoringConn`（同文件:301）：
  - 验证 Write 成功但 watchdog 在 Write 后触发时同样走 `invalidateConnection`。
- 独立应用 `ApplyConfig` 的 R1-1 整改通过 `TestZeroCalibration_*` 系列测试间接验证（`handleConnectionLost` 路径共享）。

```
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     19.636s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.404s

$ go vet + go build：全部空输出
```

**遗留：** 无。WindLabX4 `DAQP1604.SetUnit` / `sendCommandACK` 和独立应用 `P1604Adapter.ApplyConfig` 三条路径的 helper Close 后调用方统一失效全部覆盖。

### R1-2：独立 P1604 `operationMu` 与校零生命周期未完整覆盖

位置：

- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`ZeroCalibration`
- 同文件：`zeroCalibrationViaReadLoop`
- 同文件：`Connect`/`Disconnect`/`StartAcquisition`/`StopAcquisition`/`ApplyConfig` 中在 operation owner 前获取 `operationMu` 的路径

现状：多条生命周期操作在任何 operation-specific owner 建立前获取 `operationMu`。若前一操作持锁卡死，后续 Stop/Disconnect/配置等可永久等待。采集中校零还存在发送 `h` 的 Write 在响应 watchdog 前阻塞，以及响应等待超时只返回错误、不失效连接的问题。

整改：为每条可能等待 `operationMu` 的生产操作建立不依赖该锁的生命周期 owner，或改造锁/队列使 Disconnect 始终能优先 Close；校零 watchdog 必须覆盖锁等待、Write 和 pending response 总预算。采集 read loop 仍作为唯一 reader，通过 pending response 分发 ACK；响应预算耗尽后按 R0-12 毒化连接。

#### 第三批整改证据（2026-07-30）

**修改文件与符号：**

- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`ZeroCalibration`
  - 新增外层 watchdog：`sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)` 在 `operationMu.Lock` 之前启动，覆盖锁等待阶段。
  - 获取锁后立即 `wdStop()` 检查：watchdog 在锁等待期间触发时调用 `handleConnectionLost` 并返回 `lock wait watchdog triggered` 错误。
  - 获取锁后停止外层 watchdog，内层方法（`zeroCalibrationViaReadLoop` / `zeroCalibrationDirect`）各自启动自己的内层 watchdog 覆盖 Write + 响应等待。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`zeroCalibrationViaReadLoop`
  - 新增内层 watchdog：`sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)` 在 `sendCommand` 之前启动，覆盖 Write + 等待响应总预算。
  - `sendCommand` 失败时检查 `wdStop()`：watchdog 触发调用 `handleConnectionLost`；其他错误（如 cmd mismatch）若 `IsConnectionFault` 同样调用 `handleConnectionLost`。
  - 响应等待 `select` 分支：`<-ch` 收到响应时检查 `wdStop()`，watchdog 触发同样走 `handleConnectionLost`。
  - 响应超时（`p1604CalibrationTimeout`）分支：调用 `handleConnectionLost` 毒化连接（R0-12 整改：响应超时意味着设备无响应或连接已半开）。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`zeroCalibrationDirect`
  - 同样新增内层 watchdog 覆盖 Write + 响应等待，与 `zeroCalibrationViaReadLoop` 同模式。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`p1604WatchdogTimeout`
  - 从 `const` 改为 `var`（5s 默认），允许测试注入短超时（如 200ms）加速 watchdog 用例。

**根因：** 原实现 `ZeroCalibration` 直接 `driver.operationMu.Lock()`，锁等待阶段无 watchdog 兜底。若前序操作（如采集中校零、配置写入）持锁卡死，后续 `ZeroCalibration` / `Disconnect` / `StopAcquisition` 永久等待 `operationMu`，前端"校零"按钮永远转圈。`zeroCalibrationViaReadLoop` 的 `sendCommand` Write 也无 watchdog 兜底，`SetWriteDeadline` 失效时永久阻塞。

**独立 Close owner：** `sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)` 返回 `time.AfterFunc` timer，触发时直接 `conn.Close()`，不依赖阻塞 I/O，不获取 `operationMu`。外层 watchdog 覆盖锁等待阶段，内层 watchdog 覆盖 Write + 响应等待阶段。

**watchdog/soft timeout 后如何失效 conn：** watchdog 触发 → `conn.Close()` → `sendCommand` / `Write` 返回错误 → 调用 `handleConnectionLost(id, driver, calibErr)` → 内部 `shard.drivers[id]==driver` 检查后删除 driver + Close conn + 通知前端。响应超时分支同样走 `handleConnectionLost`。

**reader 和驱动状态：** `handleConnectionLost` 清理 `shard.drivers[id]`（删除 driver 引用，driver 的 conn 在 `handleConnectionLost` 内部 Close）、置 `status.Connection=Error`、`status.LastError` 设置、通知前端 `onError`。

**是否存在可选/探测读取：** 否，`ZeroCalibration` 是期待设备 `h` 响应的命令路径。

**如何证明不会误杀健康连接：**
- `p1604WatchdogTimeout=5s`，正常 `h` 响应在 100ms 内完成，5s 是 50 倍余量。
- `TestZeroCalibration_IdleSuccess`（`projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter_test.go:765`）验证 idle 状态下校零正常完成，连接不被关闭。

**对应测试和实际验证结果：**

- `TestZeroCalibration_LockWaitWatchdogTriggersConnectionLost`（`projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter_test.go:2184`）：
  - 测试前置：`net.Pipe` + 注入 `p1604WatchdogTimeout=200ms` + 前序 goroutine 持 `operationMu` 后永久阻塞（模拟卡死）。
  - 测试步骤：主 goroutine 调用 `ZeroCalibration`，等待 watchdog 触发。
  - 期待结果：返回 `lock wait watchdog triggered` 错误；`handleConnectionLost` 被调用；driver 被清理。
- `TestZeroCalibrationViaReadLoop_WriteWatchdogTriggersConnectionLost`（同文件:2067）：
  - 测试前置：`net.Pipe` + 注入短 watchdog + `deadlineIgnoringConn`（让 Write 永久阻塞）。
  - 测试步骤：调用 `ZeroCalibration`（走 `zeroCalibrationViaReadLoop` 路径）。
  - 期待结果：返回 `write watchdog triggered` 错误；`handleConnectionLost` 被调用。
- `TestZeroCalibration_ResponseTimeoutInvalidatesConn`（同文件:1944）：
  - 测试前置：`net.Pipe` + 设备收到 `h` 后不响应。
  - 测试步骤：调用 `ZeroCalibration`，等待 `p1604CalibrationTimeout`。
  - 期待结果：返回 `等待设备响应超时` 错误；`handleConnectionLost` 被调用。
- `TestP1604ZeroCalibrationDirect_WatchdogTriggersOnWriteDeadlineIgnoringConn`（同文件:1753）：
  - 验证 `zeroCalibrationDirect` 路径的 Write watchdog 同样触发 `handleConnectionLost`。

```
$ $env:GOWORK="off"; go test -race -count=1 ./projects/daq-p1604/apps/desktop-wails/adapters/hardware/...
ok      daq-p1604/adapters/hardware     21.637s

$ go vet + go build：全部空输出
```

**遗留：** 无。`ZeroCalibration` 锁等待、Write、响应等待三阶段的 watchdog 全部覆盖，触发后统一走 `handleConnectionLost` 毒化连接。

### R1-3：DSA3217 terminal read error 后状态矛盾

位置：

- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217_readloop.go`：`readLoop`

现状：watchdog Close、EOF、RST、非主动停止的其他 terminal `ReadString` error 后，defer 都可能把状态恢复为 Connected，且不清除 conn/reader，仅依赖外层 `onError` 后续处理。

整改：所有非主动停止的 terminal read error 直接走 expected-conn invalidation；外层回调只能通知，不应承担底层状态正确性的唯一责任。

#### 第三批整改证据（2026-07-30）

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217_readloop.go`：`readLoop`
  - defer 块新增 `unexpectedErr` 检查：`unexpectedErr != nil` 时调用 `d.invalidateConnection(unexpectedErr.Error())` 统一毒化连接。
  - `unexpectedErr == nil` 分支保留原清理逻辑（主动停止或 conn 被外部路径 Close，仅清理 `readLoopDone`）。
  - readLoop 自身 watchdog 触发时设 `unexpectedErr = fmt.Errorf("read loop watchdog triggered: ...")`。
  - 非 closed 类错误（协议错误等）设 `unexpectedErr = err`。
  - `isClosedConnError` 分支：检查 `stop` 是否已 close，是则 `unexpectedErr=nil`（主动停止），否则设 `unexpectedErr`（外部 Close 但非主动停止）。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217.go`：`invalidateConnection`
  - 复用既有实现：`d.mu.Lock` → 清 conn/reader/stop + `status.Connection=Error` + `status.LastError=message` → `d.mu.Unlock` → `conn.Close()` → `onError`。
  - 与 `sendCommand` / `invalidateConnectionAfterReadLoopTimeout` 共享同一失效函数，保证状态最终一致。

**根因：** 原实现 readLoop defer 在 terminal read error（watchdog 触发、EOF、RST、协议错误）后仅把 `status.Connection` 从 `Acquiring` 恢复为 `Connected`，不清 conn/reader，依赖外层 `onError` 后续处理。问题：状态矛盾（conn 已死但状态显示 Connected），且外层回调不保证执行（如进程崩溃），底层状态正确性不能仅靠回调。

**独立 Close owner：** `invalidateConnection` 在锁外执行 `conn.Close()`，不依赖阻塞 I/O，不获取 `ioMu`。readLoop 自身 watchdog（`time.AfterFunc`）触发时直接 `conn.Close()` 解除 `ReadString` 阻塞，readLoop defer 随后执行 `invalidateConnection`。

**watchdog/soft timeout 后如何失效 conn：** readLoop defer 检测 `unexpectedErr != nil` → 调用 `invalidateConnection(unexpectedErr.Error())` → `d.mu.Lock` → `conn = d.conn; d.conn = nil; d.reader = nil; d.stop = nil; d.status.Connection = Error; d.status.LastError = message` → `d.mu.Unlock` → `conn.Close()` → `onError`。

**reader 和驱动状态：** `d.conn=nil`、`d.reader=nil`（bufio.Reader）、`d.stop=nil`、`d.status.Connection=Error`、`d.status.LastError` 设置、`onError` 回调被调用。

**是否存在可选/探测读取：** 否，readLoop 是采集数据读取，不是可选/探测读取。`net.Error.Timeout()` 软超时在循环内 `continue` 处理，不进入 `unexpectedErr` 分支。

**如何证明不会误杀健康连接：**
- 软超时（`net.Error.Timeout()`）在 readLoop 循环内 `continue`，不设 `unexpectedErr`，健康连接保持开放。
- `isClosedConnError` 分支检查 `stop` 是否已 close：主动停止时 `unexpectedErr=nil`，仅清理 `readLoopDone`，不调 `invalidateConnection`。
- `TestDSA3217ReadLoop_InvalidatesConnOnTerminalReadError` 验证 terminal error 走 `invalidateConnection`；正常停止路径不触发 `invalidateConnection`（通过 `onError` 未被调用断言）。

**对应测试和实际验证结果：**

- `TestDSA3217ReadLoop_InvalidatesConnOnTerminalReadError`（`projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217_test.go:301`）：
  - 测试前置：`net.Pipe` + 注入短 `readLoopWatchdog` + `deadlineIgnoringConn`（让 Read 永久阻塞触发 watchdog）。
  - 测试步骤：调用 `StartAcquisition`，等待 readLoop watchdog 触发。
  - 期待结果：`d.conn == nil`；`status.Connection == Error`；`status.LastError` 包含 `watchdog triggered`；`onError` 被调用。
- `TestDSA3217SendCommand_InvalidatesConnectionOnWatchdogTrigger`（同文件:235）：
  - 验证 `sendCommand` 路径 watchdog 触发同样走 `invalidateConnection`。
- `TestDSA3217SendCommand_SoftTimeoutInvalidatesConn`（同文件:402）：
  - 验证 `sendCommand` soft timeout 路径走 `invalidateConnection`（R0-12 整改证据）。

```
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     19.636s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.404s

$ go vet + go build：全部空输出
```

**遗留：** 无。DSA3217 readLoop 的 watchdog 触发、EOF、RST、协议错误四类 terminal read error 全部走 `invalidateConnection` 统一毒化，状态不再矛盾。

### R1-4：`DialTCP` 晚到连接清理存在漏洞

位置：`shared/device-sdk/go/protocol/conn_helpers.go`：`DialTCP`

现状：`resultCh` 为缓冲 channel。调用方超时返回后，Dial goroutine 的晚到 send 仍可能成功写入无人接收的 channel，`default` 分支不会执行，导致晚到 conn 泄漏。

整改：显式 abandoned 信号或无缓冲交付协议，确保超时后所有晚到 conn 必定 Close；通过可注入 dial function 构造“忽略 timeout、超时后才成功”的确定性测试。

## P2：工具与诊断一致性

### R2-1：诊断工具 operation watchdog 覆盖不完整

位置：

- `programs/p1604-unit-diag/main.go`
- `programs/p1604-ts-diag/main.go`
- `projects/daq-t1603/apps/desktop-wails/cmd/freqprobe/main.go`
- `projects/daq-t1603/apps/desktop-wails/cmd/frameprobe/main.go`

现状：进程级 watchdog 在 `net.DialTimeout` 返回后才启动，无法覆盖 Dial 本身；工具内部的命令 Write、首字节/帧读取、可选 ACK、quiet-window 和 drain 仍主要依赖短 deadline。5 分钟进程退出只能防止无限存活，不能兑现每条操作声明的毫秒/秒级预算，也不能保证可选/探测语义正确。

整改：统一改用修正后的 `protocol.DialTCP`；为每条命令、帧读取和探测操作使用与生产协议一致的 owner/边界，移除破坏性 drain 和 deadline-only 可选读取；进程 watchdog 仅作为最后防线。WindLabX4 0.11.2 release note 已把四套工具列入整改交付声明，因此本清单按交付范围管理；若实际安装包不包含它们，发布材料必须明确"开发/现场诊断工具，不随安装包交付"。

#### 第五批整改证据（2026-07-30）

**修改文件与符号：**

- `programs/p1604-unit-diag/main.go`：`main`
  - `net.DialTimeout("tcp", addr, connectTimeout)` 改为 `sharedproto.DialTCP(addr, "", connectTimeout)`。
  - 复用 R1-4 整改后的 `DialTCP`（无缓冲 channel + abandoned 信号），主线程在 timeout 后立即返回错误，晚到 conn 被 Close 不泄漏。
- `programs/p1604-ts-diag/main.go`：`main`
  - 同样改为 `sharedproto.DialTCP(addr, "", 5*time.Second)`。
- `projects/daq-t1603/apps/desktop-wails/cmd/freqprobe/main.go`：`main`
  - 同样改为 `protocol.DialTCP(addr, "", 5*time.Second)`（freqprobe/frameprobe 用 `protocol` 别名，不带 shared 前缀）。
- `projects/daq-t1603/apps/desktop-wails/cmd/frameprobe/main.go`：`main`
  - 同样改为 `protocol.DialTCP("192.168.1.10:9000", "", 5*time.Second)`。

**根因：** 四套诊断工具原用 `net.DialTimeout`，依赖 Dial 内部 deadline。Windows 故障机器 deadline 不可靠时 Dial 可能永远不返回，工具启动即卡死，进程级 5 分钟 watchdog 无法覆盖 Dial 本身（watchdog 在 Dial 返回后才启动）。

**独立 Close owner：** `sharedproto.DialTCP` / `protocol.DialTCP` 内部 goroutine + `time.After` 软超时 + `abandoned` close 信号（R1-4 整改保证）。主线程超时后 `close(abandoned)`，goroutine 走 abandoned 分支 `conn.Close()`，不依赖阻塞 I/O。

**watchdog/soft timeout 后如何失效 conn：** `DialTCP` 主线程 `time.After(timeout)` 触发 → `close(abandoned)` → 返回 `os.ErrDeadlineExceeded`。goroutine 后续完成 Dial 时 select 走 `case <-abandoned:` 分支，`conn.Close()` 释放 FD。诊断工具收到错误后 `fail()` / `os.Exit(1)` 退出，无需清理驱动状态。

**reader 和驱动状态：** 诊断工具无驱动状态，Dial 失败时 `conn` 未创建，`defer conn.Close()` 无副作用。

**是否存在可选/探测读取：** 否，Dial 是连接建立操作。

**如何证明不会误杀健康连接：**
- 正常 Dial 在 100-300ms 内完成，`connectTimeout=5s` 是 15-50 倍余量。
- `TestDialTCPNormalDial`（R1-4 整改证据）验证正常路径返回可用 conn。
- 四套工具改用 `DialTCP` 后自动继承 R1-4 的所有测试保证，无需单独测试。

**对应测试和实际验证结果：**

- `TestDialTCPReturnsAtTimeout`（`shared/device-sdk/go/protocol/conn_helpers_test.go:58`）：验证 DialTCP 在 timeout 内返回错误（R1-4 整改证据）。
- `TestDialTCP_LateArrivingConnIsClosedAfterTimeout`（同文件:142）：验证晚到 conn 被 Close 不泄漏（R1-4 整改证据）。
- `TestDialTCPNormalDial`（同文件:95）：验证正常路径返回可用 conn（R1-4 整改证据）。
- 四套诊断工具 build 验证：
  ```
  $ cd programs/p1604-unit-diag; $env:GOWORK="off"; go build .  # 空输出
  $ cd programs/p1604-ts-diag; $env:GOWORK="off"; go build .    # 空输出
  $ cd projects/daq-t1603/apps/desktop-wails; $env:GOWORK="off"; go build ./cmd/freqprobe/... ./cmd/frameprobe/...  # 空输出
  ```

**遗留：** 工具内部的命令 Write、首字节/帧读取、可选 ACK、quiet-window 和 drain 仍主要依赖短 deadline——这些是诊断工具的探测语义，不是生产路径。**注意：进程级 5 分钟 watchdog 仅作为"防止工具无限存活"的最后防线，不等价于 ADR-009 要求的"每条操作具有独立硬边界"。** 5 分钟超时远超单条操作的毫秒/秒级预算，不能兑现声明的超时语义，也无法保证可选/探测语义正确。该差距在 finding 7 中已明确标为"局部完成"，按交付范围管理（WindLabX4 0.11.2 release note 已声明），若实际安装包不包含四套工具，发布材料必须明确"开发/现场诊断工具，不随安装包交付"。

### R2-2：DSA3217 Disconnect 掩盖 join-timeout Error

位置：`projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217.go`：`Disconnect`。

现状：join 超时失效路径设置 Error 后，后续阶段可能无条件覆盖成 Disconnected，丢失“连接被强制关闭”的诊断状态。

整改：保留 Error 与 LastError；仅正常关闭时置 Disconnected。

#### 第五批整改证据（2026-07-30）

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217.go`：`Disconnect` Phase 3
  - 原 `d.status.Connection = device.ConnectionDisconnected` 无条件覆盖。
  - 改为 `if d.status.Connection != device.ConnectionError { d.status.Connection = device.ConnectionDisconnected }`。
  - 保留 Phase 2 `invalidateConnectionAfterReadLoopTimeout` 设置的 Error + LastError，让前端感知"连接被强制关闭"。

**根因：** 原实现 Phase 3 无条件置 Disconnected，丢失 Phase 2 invalidate 设置的 Error 状态。readLoop join 超时走 invalidate 路径后，Disconnect 的 Phase 3 把 Error 覆盖为 Disconnected，前端误判为"正常断开"，无法提示用户重连。

**独立 Close owner：** R2-2 不涉及 Close owner，仅状态管理。Phase 2 的 `invalidateConnectionAfterReadLoopTimeout` 已是独立 Close owner（R0-11 整改保证）。

**watchdog/soft timeout 后如何失效 conn：** R2-2 不改变 conn 失效路径，仅保留 Error 状态。conn 在 Phase 2 invalidate 或 Phase 4 Close 中被关闭，行为不变。

**reader 和驱动状态：** `d.conn=nil`、`d.reader=nil`（Phase 3 清空）；`d.status.Connection=Error`（保留自 Phase 2 invalidate）；`d.status.LastError` 非空（保留自 Phase 2 invalidate）。

**是否存在可选/探测读取：** 否，Disconnect 是生命周期管理操作。

**如何证明不会误杀健康连接：**
- 正常停止路径（readLoop 在 join timeout 内退出）不走 invalidate，Phase 3 仍置 Disconnected。
- `TestDSA3217Disconnect_NormalStopSetsDisconnected` 验证正常路径状态为 Disconnected。
- readLoop 卡死路径走 invalidate，Phase 3 跳过 Disconnected 覆盖保留 Error。
- `TestDSA3217Disconnect_PreservesErrorStatusFromInvalidate` 验证卡死路径状态为 Error + LastError 包含 `reconnect required`。

**对应测试和实际验证结果：**

- `TestDSA3217Disconnect_DoesNotDeadlockWhenReadLoopBlocked`（`projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217_test.go:120`）：
  - 强化断言：原"Disconnected 或 Error"改为严格断言 Error + LastError 非空（readLoop 卡死场景必须保留 Error）。
- `TestDSA3217Disconnect_PreservesErrorStatusFromInvalidate`（同文件:182）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn` + `readLoopWatchdog=30s`（确保 readLoop 卡死触发 invalidate）。
  - 测试步骤：调用 `Disconnect`，等待 join timeout + invalidate。
  - 期待结果：`status.Connection == Error`；`LastError` 包含 `reconnect required`。
- `TestDSA3217Disconnect_NormalStopSetsDisconnected`（同文件:226）：
  - 测试前置：`net.Pipe` + 服务端 50ms 后 Close 让 readLoop 正常退出（不走 invalidate）。
  - 测试步骤：调用 `Disconnect`。
  - 期待结果：`status.Connection == Disconnected`（反向断言：正常路径不保留 Error）。

```
$ go test -race -count=1 -run TestDSA3217Disconnect ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     8.852s

$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/... ./shared/device-sdk/go/motion/adapters/hardware/... ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      shared.local/device-sdk/go/protocol     11.285s
ok      shared.local/device-sdk/go/daq/hardware 19.853s
ok      shared.local/device-sdk/go/motion/adapters/hardware     2.894s
ok      windlabx4/services/api-go/internal/adapters/hardware     20.724s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.391s

$ go vet + gofmt：全部空输出
```

**遗留：** 无。readLoop 卡死场景保留 Error + LastError，正常停止场景置 Disconnected，两条路径状态语义正确。

## 已核实完成或可作为参考的部分

### C-1：独立 P1604 Start 已移除阻塞 drain

`projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go` 的 `StartAcquisition` 已明确不再调用 `DrainConnection`，通过停止 idle reader、Reset 和 ACK 跳帧处理残留。这是 WindLabX4 P1604 的整改参考。

### C-2：P1604 ACK helper 的局部边界

`P1604ReadCommandACK` 具备独立 watchdog、总预算和有界残留帧跳过。它只保证 helper 自身有界；调用方仍必须在 watchdog Close 后失效驱动状态。

### C-3：T1603 Windows UDP Receive

T1603 的 `winsockDiscoverySocket.Receive` 已在 `Recvfrom` 前启动独立 `Closesocket` watchdog，可作为另外两套 Windows scanner 的实现参考。当前测试只分别证明普通超时返回和外部手工 Close 能解除 Recvfrom，没有直接观察生产 `time.AfterFunc` timer 在软超时失效条件下触发，因此不能标为完全验收。

### C-4：本轮 T1603 实机协议边界修正

已在 `192.168.1.10:9000` 验证当前固件：

- `@e3` 返回 16 字节类型加单个 LF，无 CR。
- 固定 `@fd` 查询返回纯数据，无终止符。
- `@fe BIN/TIME/HEAD` 返回单字节 `A`，无终止符。
- 本轮未执行 `@f0`/`@f1`，不对其 ACK 契约作实机已验证声明。

当前工作树已修正这些确定响应的多余尾部读取。但这不等于 T1603 生命周期整改完成，R0-1 至 R0-4 仍需处理。

## 整改顺序

1. **第一批：永久阻塞、双 reader 与健康连接误杀**
   - R0-3 T1603 optional ACK/drain
   - R0-5 WindLabX4 P1604 drain
   - R0-6 DAQP1064Pre 双 reader
   - R0-8、R0-9 Windows discovery Receive/三套 Send
   - 修正对应错误测试
2. **第二批：保证单 reader 与生命周期可收敛**
   - R0-1 T1603 join
   - R0-10 全部采集 read-loop no-data owner
   - R0-11 terminal read error 状态失效
3. **第三批：覆盖所有 Write、锁等待与状态失效**
   - R0-2、R0-4、R0-12、R1-1、R1-2、R1-3
4. **第四批：Dial 边界与晚到连接所有权**
   - R0-7、R1-4
5. **第五批：诊断工具与状态文案**
   - R2-1、R2-2

## 每项提交的强制验证

每个整改项必须从以下验证中选择所有适用项，并在提交/报告中列明不适用项及原因：

1. 忽略 `SetReadDeadline`/`SetWriteDeadline`、只在 Close 后返回的 conn double。
2. 路径实际包含的每个阻塞阶段都必须测试：锁等待、Write、首字节 Read、帧中途 Read 不得只任选一种。
3. watchdog 触发后断言：conn/reader 已清空、状态 Error、LastError 可诊断、旧连接不可复用。
4. 可选/探测 I/O 的反向测试：对端合法地不发送数据时，健康连接不得被关闭。
5. read-loop join 测试：超时后不得启动第二 reader。
6. Dial 测试：注入忽略 timeout 的 dial，调用方按预算返回；晚到 conn 被关闭。
7. Windows raw Winsock 测试或可替代的 handle seam，证明 `Closesocket` 能从独立 owner 触发。
8. 命令路径必须测试 soft deadline 先返回、watchdog 未触发、迟到响应随后到达的场景，证明旧连接已毒化且下一命令不消费迟到响应。
9. 相关模块 `go test -race -count=1`、`go vet`、生产 build；全绿后才能更新本表状态。

## 完成定义

禁止再以“所有测试通过”直接宣布本清单完成。只有当：

- 本表所有 R0/R1/R2 项逐项有代码、针对性测试和验证命令证据；
- 不再存在已知 deadline-only 生产路径；
- 不再存在可选/探测读取误杀健康长连接；
- 所有 watchdog Close 都能在驱动状态中观察到一致失效；
- 新一轮独立逆向复核未发现实质性遗漏；

才能把文档状态改为“整改完成”。

## 第一批整改验收总结（2026-07-29）

### 完成项

| 编号 | 范围 | 验收结论 |
|---|---|---|
| R0-3 | T1603 可选 ACK / drain 误杀健康连接 | **完成** — `ConsumeOptionalACK` 移除 watchdog Close；`drainConnection` 彻底删除；3 个不误杀测试通过 |
| R0-5 | WindLabX4 P1604 `DrainConnection` 误杀健康连接 | **完成** — `StartAcquisition` 和 `SetUnit` 移除 `DrainConnection`，改用 `frameReader.Reset()`；2 个不误杀测试通过 |
| R0-6 | DAQP1064Pre 同一 TCP 流存在双 reader | **完成** — 新增 `ioMu` 串行化 I/O；watchdog 覆盖锁等待/Write/Read；8 个针对性测试通过 |
| R0-8 | P1604、WindLabX4 Windows UDP Recvfrom 无 raw-handle watchdog | **完成** — 三套 Windows scanner 统一 `time.AfterFunc(Closesocket)` watchdog；3 个测试覆盖软超时/外部 Close/loopback 不误杀 |
| R0-9 | 三套 UDP discovery Send 无独立 Close owner | **完成** — 三套 `Send` 统一 `SO_SNDTIMEO` + `time.AfterFunc(Close/Closesocket)` watchdog；3 个测试覆盖阻塞/超时/loopback 不误杀 |
| R0-12 | 命令响应 soft timeout 后未统一毒化协议连接 | **DAQP1064Pre 路径已完成**（附带），其他路径（T1603 helper / P1604 ACK helper / P1604 unit helper / DSA3217）待第三批 |

### 验证命令与结果

```
# gofmt
$ gofmt -l <所有修改文件>
（空输出，格式全部合规）

# go test -race -count=1
$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/protocol     8.246s
ok      shared.local/device-sdk/go/daq/hardware 8.385s

$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     17.500s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 2.421s

$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/scan/...
ok      windlabx4/services/api-go/internal/adapters/scan 4.047s

$ GOWORK=off; cd projects/daq-p1604/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-p1604/adapters/hardware     24.520s

$ GOWORK=off; cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-t1603/adapters/hardware     3.242s

# go vet
$ go vet ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/... ./projects/windlabx4/services/api-go/internal/adapters/hardware/... ./projects/windlabx4/services/api-go/internal/adapters/scan/...
（空输出，无告警）

$ GOWORK=off; cd projects/daq-p1604/apps/desktop-wails/adapters/hardware; go vet ./...
（空输出）

$ GOWORK=off; cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go vet ./...
（空输出）

# go build -buildvcs=false
$ go build -buildvcs=false ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/... ./projects/windlabx4/services/api-go/internal/adapters/hardware/... ./projects/windlabx4/services/api-go/internal/adapters/scan/...
（空输出，编译通过）

$ GOWORK=off; cd projects/daq-p1604/apps/desktop-wails/adapters/hardware; go build -buildvcs=false ./...
（空输出）

$ GOWORK=off; cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go build -buildvcs=false ./...
（空输出）

# 项目结构校验
$ .\scripts\validate-structure.ps1
SIZE 错误全部通过 waivers 解决；剩余 Missing directories / Unexpected entries 均为预存问题，与本次整改无关。

# GitNexus detect_changes
$ npx gitnexus detect_changes --repo AI-WorkSpace
Changes: 101 files, 666 symbols
Affected processes: 59
Risk level: critical
（符合 ADR-009 整改预期，影响范围集中在硬件 I/O 路径，无意外文件受影响）
```

### 文件 waivers 说明

`scripts/go-file-waivers.txt` 新增以下条目（行数增长由 ADR-009 整改直接导致，禁止顺手拆分）：

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`（845 行，R0-6/R0-12 整改）
- `projects/windlabx4/services/api-go/internal/adapters/hardware/sim/simulator.go`（528 行，SIM-1 整改）
- `projects/windlabx4/services/api-go/internal/usecase/traversal_manager_registry.go`（512 行，双探针模式整改）
- `projects/windlabx4/services/api-go/internal/usecase/traversal_view.go`（516 行，双探针 + ADR-009 整改）
- 5 个预存超长文件（app.go / t1603_adapter.go / csv_recorder.go / types.go x2）暂予豁免，与本次整改无关。

### 未完成项（复核修订 2026-07-30）

独立审查 Request Changes，确认以下 findings 真实存在，需后续推进：

- **finding 1（Critical）**：P1604 启动失败路径 `rollbackAcquisition` double-close channel panic — **已修复**（owner 检查 + 2 个回归测试）
- **finding 2（High）**：4 套 `invalidateConnection`（daq_p1604 / daq_p1064pre / dsa3217 / b140_motion）不比较 expected conn — **已修复**（4 套均添加 `expectedConn` 参数，仅当 `d.conn == expectedConn` 时清状态；旧 conn 始终在锁外关闭）
- **finding 3（High）**：T1603 `SendCommand`/`SendCommandIdle`/`SendCommandExact` 非 timeout 错误（EOF/短读/io.ErrUnexpectedEOF）不毒化连接；DAQP1064Pre `readResponseFrame` invalid header / response too long / 短 body 不毒化；WindLabX4 P1604 ACK EOF/短帧不毒化 — **已修复**（所有协议边界错误均强制 `conn.Close()` 并包装 `ErrWatchdogTriggered` sentinel，调用方 `IsWatchdogTriggered` 触发 `invalidateConnection`）
- **finding 4（High）**：T1603 `ApplyConfig`/readLoop @f1 cleanup 先 `writeMu.Lock` 后启动 watchdog；独立 P1604 `Disconnect`/`StartAcquisition`/`StopAcquisition`/`ApplyConfig` 直接 `operationMu.Lock` 无 watchdog 覆盖锁等待 — **已修复**（所有路径 watchdog 前移到 `Lock` 之前，触发后跳过 I/O 直接毒化连接返回错误）
- **finding 5（High）**：3 套 `discovery_socket_windows.go`（daq-t1603 / daq-p1604 / windlabx4 scan）`time.AfterFunc` + `defer Stop` 无 stop-and-join 语义，raw handle 非原子失效 — **已修复**（`handleMu` 保护 handle，`closeHandleLocked` 原子取走 handle 并 Closesocket 一次；`startWatchdog` 返回 stop-and-join 函数通过 `sync.WaitGroup` 确保 callback 完全退出后才返回，避免 callback 在 Send/Receive 返回后误关已复用的 handle 数值）
- **finding 6（High）**：3 套 scanner（t1603 / p1604 / windlabx4 network）Send 失败后继续复用已 watchdog 销毁的 socket — **已修复**（Send 失败直接 `return nil, nil`/`return nil`，不进入 Receive 循环；3 套均补 `TestXxxScanner_SendFailureSkipsReceive` 测试，断言 `ReceiveCallCount == 0`）
- **finding 7（Medium）**：R2-1 只修 Dial，Write/Read/ACK/drain 仍只依赖 deadline — 状态已更新为"局部完成"
- **finding 8（Medium）**：文档页首"全部完成"与总览/后文矛盾 — 状态已恢复为"整改未完成"
- **finding 9（High）**：T1603 `sendCommand` 不校验 A/E 响应（设备返回 'E' 仍继续下发命令）；temp 型号固件 `@fe BIN 1` ACK 但不切换，FrameReader 错误进入 64 字节二进制解析 — **部分修复**（第一批：`sendCommand` 解析 A/E/其他字节三态，新增 `ErrDeviceRejected` sentinel 不毒化连接；`syncHardwareConfigLocked` 发送 `@fe BIN 1` 后重新查询 `@fd BIN` 验证，读回 0 时回退 ASCII。第二批复核修订：`queryBinaryMode` 严格校验只接受 "0"/"1"，其他响应返回协议错误中止同步；BIN 验证失败（非 watchdog）不再假定 BIN=1 而是中止同步；`applyHardwareConfig` 中途收到 'E' 后调用 `resyncHardwareConfigMode` 重新读取设备实际 BIN/TIME/HEAD 并同步到本地 cfg 与 FrameReader。9 个回归测试覆盖 A/E/非法 ACK/temp 回退/正常路径/BIN 验证非法响应/BIN 验证中止/部分失败 resync/全成功无 resync）

R0-7（DSA3217 / DAQP1064Pre / WTN-PXI / B140 Dial 外层边界）总览已同步第四批整改状态：第四批已完成，详见 R0-7 章节"第四批整改证据"。

## 第二批 R0-1 整改证据（2026-07-29）

### 完成项

| 编号 | 范围 | 验收结论 |
|---|---|---|
| R0-1 | T1603 join 超时后可能启动第二个 reader | **完成** — join 超时调用 `invalidateConnectionAfterReadLoopTimeout` 废弃连接；不启动第二 reader；不继续发送 @f1/@f0；1 个针对性测试通过（WriteCount==0 断言） |

### 验证命令与结果

```
# gofmt
$ gofmt -l shared/device-sdk/go/daq/hardware/daq_t1603.go shared/device-sdk/go/daq/hardware/daq_t1603_test.go
（空输出，格式全部合规）

# go test -race -count=1
$ go test -race -count=1 -run TestDAQT1603StartAcquisition_InvalidatesConnOnReadLoopJoinTimeout ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/daq/hardware 9.731s

$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/protocol     8.227s
ok      shared.local/device-sdk/go/daq/hardware 11.370s

# 跨项目回归验证
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     23.141s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 8.076s

$ GOWORK=off; cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-t1603/adapters/hardware     3.232s

# go vet + go build
$ go vet ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
$ go build -buildvcs=false ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/...
（全部空输出）
```

### 文件 waivers 说明

无新增 waiver。`daq_t1603.go` 行数在 R0-1 整改后仍在 500 行以内（waiver 已有，与本次整改无关）。

### 第二批剩余项

无。R0-1 / R0-10 / R0-11 均已完成。

R0-10 / R0-11 已于 2026-07-30 完成（见上方 R0-10 / R0-11 章节"第二批整改证据"）。
注：本章节记录第二批整改时的状态。后续第三/四/五批声明完成后，独立审查（2026-07-30）发现 findings 1-8 仍存在，文档状态已恢复为"整改未完成"，详见页首与"未完成项（复核修订 2026-07-30）"章节。

## 复核修订 findings 2-4 整改证据（2026-07-30）

### finding 2：4 套 `invalidateConnection` 不比较 expected conn

**问题根因：** 旧 I/O 失败的 invalidation 重新读取并无条件清空 `d.conn`。当旧命令与 `Disconnect -> Connect` 并发时，新连接会被旧命令的失效路径误杀。

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`invalidateConnection` / `invalidateConnectionAfterReadLoopTimeout`
  - 新增 `expectedConn net.Conn` 参数。`d.mu.Lock` 后比较 `currentConn := d.conn; if currentConn != expectedConn` 分支：仅关闭旧 `expectedConn`，不修改 `d.conn`/状态/不调 `onError`，直接返回。
  - `StopAcquisition` / `StartAcquisition` 等待 `readLoopDone` 超时前在 `d.mu.Lock` 内捕获 `expectedConn := d.conn`，传给 `invalidateConnectionAfterReadLoopTimeout`。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`invalidateConnection` / `invalidateConnectionAfterReadLoopTimeout`
  - 同样的 `expectedConn` 比较逻辑；`StopAcquisition` 在 `d.mu.Lock` 内捕获 `expectedConn`。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/dsa3217.go`：`invalidateConnection` / `invalidateConnectionAfterReadLoopTimeout`
  - 同样的 `expectedConn` 比较逻辑；`StopAcquisition` 在 `d.mu.Lock` 内捕获 `expectedConn`。
- `shared/device-sdk/go/motion/adapters/hardware/b140_motion.go`：`invalidateConnectionLocked` / `applyInvalidate`
  - `invalidateConnectionLocked` 新增 `expectedConn` 参数。`c.connMu.Lock` 后比较 `currentConn := c.conn; if currentConn != expectedConn` 分支：仅关闭旧 `expectedConn`，不修改 `c.conn`/状态，直接返回。
  - `applyInvalidate` 接收 `expectedConn`，从 `sendCommand` 入口处 `conn := c.conn`（在 `c.connMu.Lock` 后捕获）传入。
  - `sendCommand` 入口处 `c.mu.Lock` + `c.connMu.Lock` 双锁后取 `conn`，传给 `applyInvalidate`。

**如何防止误杀新连接：**

- 旧命令的 invalidation 路径始终持有 `expectedConn` 引用（触发故障前捕获）。
- 进入 invalidation 时比较 `d.conn == expectedConn`：相等说明连接未被替换，正常清状态 + Close；不等说明 `Disconnect -> Connect` 已替换连接（或已置 nil），仅 Close 旧 conn 不动状态。
- Close 在锁外执行，避免与 readLoop 的 Read 竞争。

**遗留：** finding 2 完整覆盖 4 套 invalidation 路径。b140_motion 的 `applyInvalidate` 在 `r.err != nil && !r.soft`（硬 I/O 错误）路径同样接收 `expectedConn`，与 watchdog 触发路径共享同一比较逻辑。

### finding 3：命令响应失败仍会复用协议边界不可信的连接

**问题根因：** T1603/DAQP1064Pre/WindLabX4 P1604 ACK 只把 timeout 当作连接中毒；EOF、短读、io.ErrUnexpectedEOF、invalid header、cmd mismatch、response too long 等错误作为普通错误返回，连接被复用，迟到响应可能进入 TCP 流被下一条命令消费导致协议错位。

**修改文件与符号：**

- `shared/device-sdk/go/protocol/daq_t1603_frame.go`：`SendCommand` / `SendCommandIdle` / `SendCommandExact`
  - 引入 `softTimeoutTriggered bool` 标记。Write 阶段任何错误（timeout、broken pipe、RST 等）→ `softTimeoutTriggered = true; _ = conn.Close()`。
  - Read 阶段非 timeout 错误（EOF、io.ErrUnexpectedEOF、RST 等）→ 同样 `softTimeoutTriggered = true; _ = conn.Close()`。
  - 函数末尾统一判定：`if softTimeoutTriggered || !checkWd()` → 返回 `fmt.Errorf("%w; %w", resultErr, ErrWatchdogTriggered)`。
  - 调用方通过 `errors.Is(err, protocol.ErrWatchdogTriggered)` 触发 `invalidateConnection`。
- `shared/device-sdk/go/protocol/conn_helpers.go`：`P1604ReadCommandACK`
  - 同样引入 `softTimeoutTriggered` 标记。`ReadFrame` 任何错误（EOF、短读、invalid frame length、timeout 等）→ `softTimeoutTriggered = true`。
  - 跳帧循环到上限（`maxResidualFrameSkips=20`）仍无 ACK → `softTimeoutTriggered = true`（"too many residual frames"）。
  - 总预算耗尽 → `softTimeoutTriggered = true`（context.DeadlineExceeded）。
  - 末尾统一：`if softTimeoutTriggered { _ = conn.Close(); return fmt.Errorf("%w; %w", ackErr, ErrWatchdogTriggered) }`。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`sendCommand` / `readResponseFrame`
  - `sendCommand` Write 阶段失败 → `_ = conn.Close(); d.invalidateConnection(conn, ...); return ... ErrWatchdogTriggered`。
  - `readResponseFrame` 在 5 个错误分支（header EOF/短读、invalid magic、cmd mismatch、response too long、body EOF/短读）均 `_ = conn.Close()` 并返回 `fmt.Errorf("...; %w", sharedproto.ErrWatchdogTriggered)`。
  - `sendCommand` 调用 `readResponseFrame` 后检测 `IsWatchdogTriggered(err)` → `d.invalidateConnection(conn, ...)`。

**协议边界保护一致性：**

- T1603 三条命令路径（`SendCommand`/`SendCommandIdle`/`SendCommandExact`）的 Write + Read 全覆盖。
- DAQP1064Pre `sendCommand` 的 Write + `readResponseFrame` 5 个错误分支全覆盖。
- WindLabX4 P1604 `P1604ReadCommandACK` 的 `ReadFrame` 错误 + 跳帧上限 + 总预算耗尽全覆盖。
- 所有错误路径均通过 `ErrWatchdogTriggered` sentinel 让调用方统一触发 `invalidateConnection`，避免连接被复用。

**遗留：** finding 3 已覆盖审查指出的 5 个位置的 EOF / 短读 / io.ErrUnexpectedEOF / invalid header / cmd mismatch / response too long / timeout / 跳帧上限 / 总预算耗尽等"协议边界不可信"错误。**未覆盖范围**（明确保留连接，不计入 finding 3）：完整的 Nxx 设备拒绝帧（业务层拒绝，协议边界仍可信）。其他"完整但非法"响应（如未知 ACK 字节、错误帧类型）当前作为普通错误返回不毒化连接。

**复核修订（2026-07-30，撤回旧表述）：** 旧版 finding 3 段落曾隐含"非 A 字节即协议错位、应毒化连接"的语义。该表述对 T1603 设备 'E' 拒绝响应不成立——根据 [device-lab/skills/daq-t1603/SKILL.md:683、702]，'E' 是设备发出的合法、完整的错误响应，连接协议边界仍可信，**不应**触发 ADR-009 毒化。整改后 `sendCommand` 引入 `ErrDeviceRejected` sentinel 区分三种响应：'A' 成功、'E' 业务错误（不毒化）、其他字节协议错误（按上下文决定是否毒化）。详见 finding 9。

### finding 9：T1603 sendCommand 不校验 A/E 响应；temp 固件 BIN 回退缺失（High）

**问题根因（两部分）：**

1. **ACK 校验缺失**：`shared/device-sdk/go/daq/hardware/daq_t1603.go:129` 的 `sendCommand` 调用 `SendCommandExact(conn, cmd, 1)` 读取 1 字节后不校验内容，直接当成功返回。`syncHardwareConfigLocked`（:951）和 `applyHardwareConfig`（:1149）只检查 `err`，因此设备返回 'E' 拒绝时仍继续下发后续配置命令，导致配置部分生效、设备状态与驱动状态不一致。依据：[device-lab/skills/daq-t1603/SKILL.md:683、702] 明确规定 `@fe` / `@f3` 成功响应为单字节 'A'、错误响应为 'E'，收到 'E' 应终止当前操作并上报错误。
2. **temp 固件 BIN 回退缺失**：`syncHardwareConfigLocked` 发送 `@fe BIN 1` 后无条件设置 `cfg.BinaryFormat = true`，但 temp 型号固件对 `@fe BIN 1` 仍回 ACK 'A' 而**实际不切换**二进制模式（`@fd BIN` 始终返回 0）。当前驱动会把设备实际发送的 ASCII 帧按 64 字节 float32 解析，导致帧错位。依据：[device-lab/skills/daq-t1603/SKILL.md:494、992]。

**修改文件与符号：**

- `shared/device-sdk/go/protocol/daq_t1603_frame.go`：新增 `ErrDeviceRejected` sentinel
  - 注释明确语义边界：'E' 是合法错误响应，**不**触发 ADR-009 毒化；与 `ErrWatchdogTriggered`（协议边界不可信，必须毒化）形成对照。
  - 调用方通过 `errors.Is(err, ErrDeviceRejected)` 精确匹配，决定是否跳过毒化路径。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`sendCommand`
  - 解析响应字节：'A' 成功；'E' 返回 `ErrDeviceRejected` 业务错误（不毒化）；其他字节返回 "invalid ACK" 协议错误；空响应返回 "empty response (protocol violation)"。
  - 函数注释明确：本函数不直接 `invalidateConnection`，毒化与否由调用方根据错误类型决定。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`syncHardwareConfigLocked`
  - 发送 `@fe BIN 1` 后新增 `queryBinaryMode` 调用，重新查询 `@fd BIN` 验证配置生效：
    - 读回 1：启用二进制模式（默认期望路径）；
    - 读回 0：回退 ASCII，记录 warn 让用户感知 temp 固件限制；
    - 读失败（非 watchdog）：保留默认 BIN=1 期望但记录 warn；watchdog 触发直接返回错误。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：新增 `queryBinaryMode` helper
  - 复用 `sendCommandExact` 读 1 字节，返回 `(bool, error)`。
- `shared/device-sdk/go/protocol/daq_t1603_frame.go`：`T1603FrameReader` 新增 `IsBinaryMode()` 只读访问器
  - 与 `SetBinaryMode` 配对，用于测试断言 FrameReader 实际模式。

**新增测试：**

- `TestDAQT1603SendCommand_AcceptsAAsSuccess`：'A' 响应返回成功，连接未被关闭。
- `TestDAQT1603SendCommand_RejectsEResponseAsBusinessError`：'E' 响应返回 `ErrDeviceRejected`，连接仍可用（第 2 条命令成功）。
- `TestDAQT1603SendCommand_RejectsInvalidACKAsProtocolError`：'X' 字节返回 "invalid ACK" 协议错误，不属于 `ErrDeviceRejected` 也不属于 `ErrWatchdogTriggered`。
- `TestDAQT1603SyncHardwareConfig_TempFirmwareFallsBackToASCII`：完整 syncHardwareConfigLocked 路径，`@fe BIN 1` 后 `@fd BIN` 读回 0，验证 `cfg.BinaryFormat=false` 且 `FrameReader.IsBinaryMode()=false`。
- `TestDAQT1603SyncHardwareConfig_BinVerifiedStaysBinary`：正常固件路径，`@fd BIN` 读回 1，验证保持二进制模式。

**与 finding 3 的边界划分：**

- finding 3 处理"协议边界不可信"错误（EOF/短读/timeout 等），统一毒化连接；
- finding 9 处理"协议边界可信但响应内容非法或设备业务拒绝"（A/E/其他字节），区分业务错误与协议错误；
- 两者互补：finding 3 保证连接不可信时毒化，finding 9 保证连接可信时正确识别响应内容。

**复核修订（2026-07-30 第二批）：** 旧版"完整覆盖"表述过早。独立审查发现 3 个遗留场景并已修复：

1. **High 2 — queryBinaryMode 严格校验**：旧实现 `strings.TrimSpace(resp) == "1"` 把所有非 "1" 响应（含 'E'、'X' 等非法字节）都当成合法 "0" 回退 ASCII，掩盖协议错位。修订后严格校验只接受 "0" 或 "1"，其他响应返回 `@fd BIN: invalid response` 协议错误中止同步。
2. **Medium 3 — BIN 验证失败中止同步**：旧实现对非 watchdog 错误记录 warn 后假定 BIN=1 继续同步，重新引入"ACK 不代表生效"的同类风险。修订后任何验证失败（含 watchdog 与非 watchdog）都中止同步，让调用方感知错误。
3. **High 1 — applyHardwareConfig 部分失败后 resync**：旧实现中途收到 'E' 直接返回错误，但前序命令可能已部分生效（如 `@fe BIN 0` 已切 ASCII，`@fe TIME 1` 被 'E' 拒绝），设备实际状态与本地 d.config / FrameReader 不一致。修订后新增 `resyncHardwareConfigMode` 在错误路径重新查询 `@fd BIN` / `@fd TIME` / `@fd HEAD` 并同步到本地 cfg 与 FrameReader。

**新增测试（4 个，累计 9 个）：**

- `TestDAQT1603QueryBinaryMode_RejectsInvalidResponse`：'X'/'Y'/'Z'/'W' 非法响应都返回 "invalid response" 错误，不被误判为 (false, nil)。
- `TestDAQT1603SyncHardwareConfig_BinVerifyFailureAbortsSync`：BIN 验证返回 'X' 时中止同步，不再继续发送 @fe TIME / @fe HEAD。
- `TestDAQT1603ApplyHardwareConfig_PartialFailureResyncsMode`：`@fe BIN 0` 成功 + `@fe TIME 1` 返回 'E' 后，resync 读回 BIN=0/TIME=0/HEAD=0 并同步到本地，FrameReader 切 ASCII。
- `TestDAQT1603ApplyHardwareConfig_AllSuccessNoResync`：全部成功时不调用 resync，本地 cfg 直接生效。

**遗留：** finding 9 第二批复核修订后覆盖 T1603 ACK 校验、temp 固件 BIN 回退、BIN 验证严格校验、BIN 验证失败中止、部分失败 resync 五个场景。`applyHardwareConfig` 中的 `@f3` / `@fe` 命令同样走 `sendCommand`，自动继承 A/E 校验。其他设备（P1604 Nxx、DSA3217）的设备拒绝语义由各自协议层处理，不在本 finding 范围。

### finding 4：T1603 和独立 P1604 的锁等待未被硬边界完整覆盖

**问题根因：** watchdog 在 `Lock` 之后启动，问题 Windows 电脑 `SetWriteDeadline` 失效时前序操作持锁阻塞在 Write 上，后序操作会永久卡在 `Lock()` 上。

**修改文件与符号：**

- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`ApplyDaqT1603Config`
  - watchdog 从"Lock 后启动"改为"Lock 之前启动"：`wdStop := protocol.WatchdogClose(conn, DAQ_T_1603_TIMEOUT)` → `d.writeMu.Lock()` → `wdTriggered := !wdStop()` → `if !wdTriggered { err = d.applyHardwareConfig(conn, cfg) }` → `d.writeMu.Unlock()`。
  - 锁等待期间 watchdog 触发 → 跳过 `applyHardwareConfig`（conn 已死），调用 `d.invalidateConnection(conn, "apply config: writeMu lock wait watchdog triggered; reconnect required")` 并返回 `ErrWatchdogTriggered`。
  - `applyHardwareConfig` 返回 `ErrWatchdogTriggered` 时也调用 `invalidateConnection` 统一毒化。
- `shared/device-sdk/go/daq/hardware/daq_t1603.go`：`readLoop` defer 块的 `@f1` cleanup
  - watchdog 前移到 `d.writeMu.Lock` 之前：`wdStop := protocol.WatchdogClose(conn, 1*time.Second)` → `d.writeMu.Lock()` → `wdTriggered := wdStop != nil && !wdStop()` → `if !wdTriggered && conn != nil { /* 发 @f1 */ }`。
  - 锁等待期间 watchdog 触发 → 跳过 `@f1` Write，仅释放锁继续走毒化路径。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`：`Disconnect` / `StartAcquisition` / `StopAcquisition` / `ApplyConfig`
  - 4 个函数统一模式：`wdStop := sharedproto.WatchdogClose(driver.conn, p1604WatchdogTimeout)` → `driver.operationMu.Lock()` → `if wdStop != nil && !wdStop() { /* 锁等待期间 watchdog 触发：毒化 + 返回错误 */ }`。
  - `Disconnect` 锁等待 watchdog 触发：清理 `shard.drivers`/`stopChs`/`channels`/`sinks` + `driver.conn.Close()` + 设置 `StatusError` + `emitState` + 返回错误。
  - `StartAcquisition`/`StopAcquisition`/`ApplyConfig` 锁等待 watchdog 触发：调用 `a.handleConnectionLost(id, driver, ...)` + 返回错误。

**锁等待覆盖完整性：**

- T1603：`ApplyDaqT1603Config` 的 `writeMu.Lock` + readLoop defer 的 `writeMu.Lock` 全部覆盖。
- 独立 P1604：`Disconnect`/`StartAcquisition`/`StopAcquisition`/`ApplyConfig` 4 个 `operationMu.Lock` 全部覆盖（`ZeroCalibration` 此前 R1-2 已修复）。
- watchdog 触发后均跳过 I/O 直接毒化连接返回错误，避免在已死 conn 上做无效操作。

**遗留：** finding 4 完整覆盖审查指出的 6 个位置（`daq_t1603.go:637/728`、`p1604_adapter.go:530/646/795/1266`）。

### 验证命令与结果

```
# shared/device-sdk 协议与硬件层
$ go test -race -count=1 ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/... ./shared/device-sdk/go/motion/adapters/hardware/...
ok  shared.local/device-sdk/go/protocol     11.275s
ok  shared.local/device-sdk/go/daq/hardware 19.869s
ok  shared.local/device-sdk/go/motion/adapters/hardware     2.921s

# WindLabX4 hardware
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok  windlabx4/services/api-go/internal/adapters/hardware     20.741s
ok  windlabx4/services/api-go/internal/adapters/hardware/sim 2.329s

# 独立 DAQ-P1604（GOWORK=off）
$ go test -race -count=1 ./adapters/hardware/...
ok  daq-p1604/adapters/hardware     27.255s

# go vet（shared + windlabx4）
$ go vet ./shared/device-sdk/go/protocol/... ./shared/device-sdk/go/daq/hardware/... ./shared/device-sdk/go/motion/adapters/hardware/... ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
（空输出）

# go vet（daq-p1604，GOWORK=off）
$ go vet ./adapters/hardware/...
（空输出）
```

### finding 2-4 整改剩余项

无。findings 2-4 全部修复，相关位置代码已就位（详见各章节引用的文件与行号）。

## 复核修订第二轮 findings 1-3 整改证据（2026-07-30）

### finding 1（本轮）：DAQP1064Pre 命令响应帧边界错一字节

**问题根因：** 协议帧实际布局是 5 字节 header（magic 2 + cmd 1 + dataLen 2）+ payload + 1 字节 checksum。`readResponseFrame` 原实现一次读取 6 字节作为 header，多读的 1 字节实际是 payload 首字节。当 `dataLen > 0` 时，第一个 payload 字节被吃进 `header[5]`，随后又读取完整 `dataLen`，结果变成"丢失 payload0，末尾混入 checksum"。采集解析路径（`daq_p1064pre.go:775` 取 offset 5）和 `buildFrame`（`daq_p1064pre.go:1054`）都使用 5+payload+1 布局，命令响应路径与之不一致。现有测试只覆盖 `dataLen=0`，所以未暴露。

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre.go`：`readResponseFrame`
  - header 读取从 6 字节改为 5 字节：`io.ReadFull(conn, header[:5])`。
  - 读取 `dataLen` 字节 payload 后，再读 1 字节 checksum：`io.ReadFull(conn, checksumByte[:1])`。
  - 校验 checksum：`expectedChecksum := d.calculateChecksum(append(header, respData...))`，不匹配则 `_ = conn.Close()` + 返回 `fmt.Errorf("checksum mismatch: ...; %w", sharedproto.ErrWatchdogTriggered)`。
  - 任何 `io.ReadFull` 错误（EOF/短读）均 `_ = conn.Close()` + 返回 `ErrWatchdogTriggered` sentinel。

**协议帧布局一致性：**

- 采集解析路径（`processFrame`）：从 `offset 5` 取 payload，`5 + dataLen` 取 checksum — 与新 `readResponseFrame` 完全一致。
- `buildFrame`：5 字节 header + payload + 1 字节 checksum — 与新 `readResponseFrame` 完全一致。
- 命令响应路径不再"多读 1 字节"，非空 payload 不再丢失首字节、末尾不再混入 checksum。

**对应测试和实际验证结果：**

- `TestDAQP1064PreSendCommand_NonEmptyPayloadSuccess`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1064pre_test.go`）：
  - 测试前置：`net.Pipe` + `deadlineIgnoringConn` + `d.conn=ignored` + `status=Connected`；服务端 goroutine 读掉命令后写入完整帧 `[0xA5, 0x5A, 0x03, 0x00, 0x04] + [0x00, 0x01, 0x02, 0x03] + checksum`。
  - 测试步骤：调用 `sendCommand(CMD_READ_CALIBRATION, []byte{0}, 2000)`。
  - 期待结果：`err == nil`；`len(resp) == 4`（修复前会丢失首字节 0x00、末尾混入 checksum，长度仍为 4 但内容错位）；`resp[i] == expectedPayload[i]` 逐字节相等。
- `TestDAQP1064PreSendCommand_ChecksumMismatchInvalidatesConn`（同文件）：
  - 测试前置：服务端写入 checksum 故意错误的帧。
  - 测试步骤：调用 `sendCommand`。
  - 期待结果：`errors.Is(err, sharedproto.ErrWatchdogTriggered)` 为 true；`d.conn == nil`（连接已毒化）；`d.status.Connection == Error`。

**遗留：** 无。命令响应帧边界与采集解析路径、`buildFrame` 完全一致，非空 payload 路径通过新增测试覆盖。

### finding 2（本轮）：WindLabX4 P1604 `SetUnit` 仍可能由旧操作清掉重连后的新连接

**问题根因：** `SetUnit` 捕获旧 `conn`（`daq_p1604.go:423`），错误返回后却直接无条件执行 `d.conn = nil` / `d.frameReader = nil` / `status.Connection = Error`（`daq_p1604.go:457-459`）。如果旧 `SetUnit` 与 `Disconnect -> Connect` 并发，当前新连接仍会被旧操作清除。`syncUnitFromHardware` 也直接修改当前状态（`daq_p1604.go:520`），未走 `invalidateConnection` helper。

**修改文件与符号：**

- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`SetUnit`
  - 错误分支原内联清状态（`d.conn = nil` / `d.frameReader = nil` / `status.Connection = Error`）改为调用 `d.invalidateConnection(conn, fmt.Sprintf("write hardware unit coefficient: %v", err))`。
  - `invalidateConnection` 内部比较 `d.conn == expectedConn`：相等才清状态；不等仅 Close 旧 conn 不动状态，保护并发替换的新连接。
  - `IsConnResetByPeer(err) || IsWatchdogTriggered(err)` 才触发 invalidation（保留软错误不毒化的语义）。
- `projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604.go`：`syncUnitFromHardware`
  - 同样把内联清状态改为 `d.invalidateConnection(conn, fmt.Sprintf("read unit coefficient: %v", err))`。
  - 仅 `IsWatchdogTriggered(err)` 触发 invalidation（保留 `IsConnResetByPeer` 软错误路径的现有行为，避免过度毒化）。

**如何防止误杀新连接：**

- 旧 `SetUnit`/`syncUnitFromHardware` 失败时持有 `conn` 引用（触发故障前捕获）。
- 进入 `invalidateConnection` 时比较 `d.conn == expectedConn`：相等说明连接未被替换，正常清状态 + Close；不等说明 `Disconnect -> Connect` 已替换连接（或已置 nil），仅 Close 旧 conn 不动状态、不调 `onError`。
- Close 在锁外执行，避免与 readLoop 的 Read 竞争。

**对应测试和实际验证结果：**

- `TestDAQP1604_SetUnit_OldConnFailurePreservesNewConn`（`projects/windlabx4/services/api-go/internal/adapters/hardware/daq_p1604_test.go`）：
  - 测试前置：构造 `oldClient`/`newClient` 两个 `net.Pipe`；先 `d.conn = oldClient`，再 `d.conn = newClient`（模拟 `Disconnect -> Connect` 已替换）；`status = Connected`；`SetOnError` 回调计数器。
  - 测试步骤：调用 `d.invalidateConnection(oldClient, "old setUnit v01101 EOF: simulated")`。
  - 期待结果：`d.conn == newClient`（新连接未被误杀）；`d.frameReader != nil`；`status == Connected`（旧操作的 invalidation 不改状态）；`onError` 未被调用（no-op 分支）。

**遗留：** finding 2（本轮）覆盖 `SetUnit` / `syncUnitFromHardware` 两条内联清状态路径。原 finding 2 的 4 套 `invalidateConnection` helper 已在第一轮整改完成；本轮把绕过 helper 的内联路径统一收敛到 helper。

### finding 3（本轮）：scanner 在 Send 失败后仍调用同一 socket 的 Receive

**问题根因：** 三套 scanner 之前从"继续发送"改成 `break`，但退出 Send 循环后仍进入 Receive 循环。Send 错误可能表示 watchdog 已销毁 socket，因此任何后续 Receive 都不允许。

**修改文件与符号：**

- `projects/daq-t1603/apps/desktop-wails/adapters/hardware/t1603_scanner.go`：`Scan`
  - Send 失败 `break` 改为 `return nil, nil`，不进入 `readT1603Responses(socket, ...)`。
- `projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_scanner.go`：`Scan`
  - 同样 Send 失败 `return nil, nil`，不进入 `readP1604Responses(socket, ...)`。
- `projects/windlabx4/services/api-go/internal/adapters/scan/network_scanner.go`：`scanWithSocket`
  - 同样 Send 失败 `return nil`，不进入 Receive 循环；记录日志说明"终止本轮扫描并跳过 Receive"。

**协议契约一致性：**

- ADR-009 契约：watchdog 关闭的 socket 不可复用。Send 失败说明 socket handle 已被 watchdog `Closesocket` 销毁，后续 Send/Receive 不可复用。
- 三套 scanner 统一"Send 失败直接返回空结果"，调用方 `defer socket.Close()` 释放资源，不再复用已死 socket。

**对应测试和实际验证结果：**

- `TestT1603Scanner_SendFailureSkipsReceive`（`projects/daq-t1603/apps/desktop-wails/adapters/hardware/t1603_scanner_test.go`）：
  - 测试前置：注入 `failingWritePacketConn`（`WriteTo` 总返回 `net.ErrClosed`，`ReadFrom` 计数器初始 0）；`timeout = 2s`（足够长，若进入 Receive 会让测试超时）。
  - 测试步骤：调用 `scanner.Scan()`。
  - 期待结果：`err == nil`；`len(results) == 0`；`elapsed < 200ms`（不进入 Receive 循环）；`ReceiveCallCount() == 0`。
- `TestP1604Scanner_SendFailureSkipsReceive`（`projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_scanner_test.go`）：
  - 同样模式，断言 P1604 scanner Send 失败后 `ReceiveCallCount == 0`。
- `TestScanWithSocket_SendFailureSkipsReceive`（`projects/windlabx4/services/api-go/internal/adapters/scan/network_scanner_test.go`）：
  - 同样模式，断言 WindLabX4 `scanWithSocket` Send 失败后 `ReceiveCallCount == 0`。

**遗留：** 无。三套 scanner Send 失败后均直接返回，不进入 Receive 循环；3 个针对性测试通过。

### 验证命令与结果

```
# shared/device-sdk 协议与硬件层
$ go test -race -count=1 ./shared/device-sdk/go/daq/hardware/...
ok      shared.local/device-sdk/go/daq/hardware 19.897s

$ go test -race -count=1 ./shared/device-sdk/go/protocol/...
ok      shared.local/device-sdk/go/protocol     11.293s

# WindLabX4 hardware + sim + scan
$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/hardware/...
ok      windlabx4/services/api-go/internal/adapters/hardware     27.401s
ok      windlabx4/services/api-go/internal/adapters/hardware/sim 3.241s

$ go test -race -count=1 ./projects/windlabx4/services/api-go/internal/adapters/scan/...
ok      windlabx4/services/api-go/internal/adapters/scan 10.464s

# 独立 DAQ-P1604（GOWORK=off）
$ cd projects/daq-p1604/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-p1604/adapters/hardware     32.376s

# 独立 DAQ-T1603（GOWORK=off）
$ cd projects/daq-t1603/apps/desktop-wails/adapters/hardware; go test -race -count=1 ./...
ok      daq-t1603/adapters/hardware     8.916s
```

### 第二轮 findings 1-3 整改剩余项

无。findings 1-3 全部修复，相关位置代码已就位，针对性测试通过。

R2-1（finding 7）仍保持"局部完成"：四套诊断工具的 Dial 已改用 `DialTCP`，但工具内部的命令 Write、首字节/帧读取、可选 ACK、quiet-window 和 drain 仍主要依赖短 deadline——这些是诊断工具的探测语义，不是生产路径。**进程级 5 分钟 watchdog 仅作为"防止工具无限存活"的最后防线，不等价于 ADR-009 要求的"每条操作具有独立硬边界"，5 分钟超时远超单条操作的毫秒/秒级预算。** 该差距按交付范围管理（WindLabX4 0.11.2 release note 已声明）。
