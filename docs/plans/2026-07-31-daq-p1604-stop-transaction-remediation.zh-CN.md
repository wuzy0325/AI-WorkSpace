# DAQ-P-1604 Stop 事务整改方案

## 1. 状态与范围

- 日期：2026-07-31
- 状态：待实现
- 适用协议：DAQ-P-1604 TCP，连接已成功执行 `w1601`
- 涉及实现：
  - 独立应用：`projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`
  - Wind-DAQ：`projects/wind-daq/services/api-go/internal/adapters/hardware/daq_p1604.go`
  - 共享协议：`shared/device-sdk/go/protocol/daq_p1604_frame.go`、`shared/device-sdk/go/protocol/conn_helpers.go`

本文只定义 Stop 事务的整改方案，不表示代码已经完成整改。实施后必须更新本文状态，并附上验证结果。

## 2. 结论

DAQ-P-1604 在 `w1601` 模式下使用 2 字节大端长度前缀，压力数据帧与命令应答均为完整协议帧。Stop 的正常在线结构为：

```text
client -> c 02 1
device -> 0..N 个完整压力数据帧
device -> [00 03 41]                 // payload="A" 的成功 ACK 帧
```

因此 Stop 不需要静默 drain，也不需要新建第二套字节累计解析器。正确策略是复用同一个 `FrameReader`：

1. 保留 reader 已缓存的长度前缀和部分 payload；
2. 补齐并跳过 Stop ACK 前的完整合法压力帧；
3. 读取完整 ACK 帧 `[00 03 41]`；
4. ACK 成功后立即完成 Stop；
5. 任一异常都废弃连接，不在未知边界上继续复用。

Stop 前禁止调用 `FrameReader.Reset()`。Reset 可能丢掉一个半帧的前半部分，而 socket 中仍保留后半部分，使后续字节被误当成新的长度前缀。

## 3. 协议不变量

### 3.1 长度前缀

`FrameReader` 使用 2 字节大端 `frameLen`，长度包含前缀自身：

```text
[frameLen uint16 BE][payload frameLen-2 bytes]
```

成功 ACK 固定为：

```text
00 03 41
```

设备拒绝为带长度前缀的 ASCII `Nxx` payload，例如：

```text
00 05 4E 30 35                     // payload="N05"
```

压力流 payload 以 `0x01` 开头，并由 `c 05` 内容掩码决定精确长度。当前生产配置包含 16 路压力和 2 路大气数据，可选设备时间戳。

### 3.2 Stop 成功条件

Stop 只有在以下条件全部成立时成功：

1. `c 02 1` 写入成功；
2. ACK 前遇到的每个非 ASCII 帧均是完整且合法的当前压力流帧；
3. 最终读取到 payload 精确为 `A` 的 ACK 帧；
4. operation gate 等待、写命令、跳尾帧和读 ACK 的全过程未超过同一个绝对 deadline；
5. 未触发 watchdog，连接仍是发起 Stop 时捕获的同一连接。

低频时 `N` 可以为 0；高频或主机存在积压时 `N` 可以较大。`N` 不是固定值，不能用固定 20 帧作为正常协议上限。

### 3.3 Stop 失败条件

以下任一情况均表示 Stop 事务失败，连接不可继续复用：

- 写 `c 02 1` 失败；
- ACK 超时或 watchdog 触发；
- 收到 `Nxx`；
- 收到非 `A`、非 `Nxx` 的 ASCII 响应；
- 长度前缀非法、帧截断或读取失败；
- ACK 前的二进制帧不符合当前流配置；
- 总尾帧字节预算超限；
- readLoop 无法在 join 超时内退出；
- Stop 与其他命令事务的连接身份发生变化。

交互式 `StopAcquisition` 失败后必须关闭捕获的连接、清空本地连接引用、置 Error，并返回包含 `reconnect required` 的错误。不得把状态提前稳定为 Connected。

`Disconnect` 内部的 best-effort Stop 是例外：无论停流命令是否成功，最终状态都应是 Disconnected 且连接关闭；停流错误仍应返回或记录，但不得把用户主动断开后的状态改回 Error。

## 4. 当前问题

### 4.1 独立应用：Stop 前 Reset 可破坏半帧

当前 `P1604Adapter.StopAcquisition` 在 readLoop 退出后执行：

```go
driver.frameReader.Reset()
driver.sendCommandACK("c 02 1")
```

readLoop 的 `ReadFrame()` 可能在 deadline 到期前已经将长度前缀和部分 payload 放入 `FrameReader.buf`。如果此时 Reset：

```text
FrameReader.buf: [len][payload 前半段]    // 被 Reset 丢弃
socket:          [payload 后半段][后续帧][ACK]
```

下一次 `ReadFrame()` 会把 payload 后半段的前两个字节当成长度前缀，产生错误长度、错误吞帧或 ACK 污染。长度前缀协议的优势只有在保留 parser 状态时才能发挥。

同一问题也存在于独立应用的 Disconnect 停流路径。

### 4.2 两套实现：20 帧硬上限与高频能力不匹配

当前共享 helper 和独立实现均最多跳过 20 个非 ASCII 帧。该上限来自“100ms 周期、正常残留不超过 5 帧”的历史假设，但生产配置支持最小 1ms 周期。高频、调度停顿或 socket 积压下，ACK 前超过 20 个合法尾帧是正常可能事件。

固定帧数上限会把合法 Stop 错判成协议混乱并关闭健康连接。

### 4.3 Wind-DAQ：生命周期操作缺少事务级互斥

Wind-DAQ 的 Start 和 Stop 只在局部状态读写时持有 `d.mu`，完整命令事务未串行化。Stop 在 join readLoop 后发送 `c 02 1` 的窗口内，并发 Start 可以继续发送 `c 00`、`c 05`、`c 01`，造成命令交叉或 ACK 串线。

必须以 operation owner 串行化至少以下操作：

- Connect / Disconnect
- StartAcquisition / StopAcquisition
- ApplyConfig / SetUnit
- 零点校准和其他命令请求

operation gate 必须支持有界等待，不能仅使用无法取消的裸 `sync.Mutex.Lock()`。调用方在等待 gate 前捕获连接和连接代次，并启动覆盖整个操作绝对 deadline 的唯一 owner。等待或执行超时后，owner 必须能直接 Close 捕获的旧连接，符合 ADR-009。

多个等待者并发时必须遵守：

- 每个等待者只绑定进入等待时捕获的连接及连接代次；
- 获得 gate 后立即确认连接身份和代次仍匹配，否则放弃操作；
- owner timer 覆盖 gate 等待和后续 I/O，操作结束后立即取消；
- 等待者超时可以关闭仍匹配的旧连接，以解除当前 owner 可能阻塞的 I/O；
- 已重连后的新连接不得被旧等待者 timer 关闭；
- 连接失效和状态通知必须幂等，同一旧连接只完成一次毒化。

### 4.4 Wind-DAQ：设备拒绝 Stop 后仍可能保留连接

Wind-DAQ 在发送 `c 02 1` 前已将本地状态改为非采集。如果设备返回 `Nxx`，当前路径只返回错误，可能保留正在推流的连接和 Connected 状态，造成硬件与本地状态分裂。

Stop 未收到成功 ACK 时必须废弃连接。设备拒绝不是可继续复用的 Stop 结果。

## 5. 目标流程

### 5.1 推荐状态机

```text
Acquiring
    |
    | acquire operation gate with one absolute deadline
    | StopAcquisition becomes operation owner
    v
Stopping
    |
    | signal readLoop to release sole-reader ownership
    | join readLoop
    v
Stopping / command owner owns FrameReader
    |
    | write c 02 1
    | complete and validate 0..N residual stream frames
    | read framed ACK "A"
    +------------------------------+
    | success                      | any failure
    v                              v
Connected                      Error / Disconnected
                                 close conn
                                 reconnect required
```

在 ACK 成功之前，不应向外发布稳定的 Connected 状态。可以先将 `Acquiring=false`，但外部状态应保持 Stopping 或等价的不可启动/不可配置状态。

### 5.2 唯一 reader 所有权

Stop 不启动第二个 reader。所有连接读取权按以下顺序转移：

1. 采集期间由 readLoop 独占 `FrameReader`；
2. Stop 先通知 readLoop 退出并 join；
3. join 成功后由 Stop command owner 独占同一个 `FrameReader`；
4. Stop owner 保留已有 `FrameReader.buf`，补齐可能存在的半帧；
5. Stop 成功后再启动 idleReadLoop 或允许下一命令。

join 超时时直接 Close 并废弃连接，不发送 `c 02 1`，不启动第二个 reader。gate 等待、join、命令 Write 和 ACK Read 共用 Stop 入口创建的绝对 deadline，不允许各阶段重新获得一份完整超时预算。

### 5.3 ACK 前尾帧处理

`P1604ReadCommandACK` 应持续读取完整帧：

```text
payload == "A"       -> Stop success
payload matches N[0-9]{2} -> device rejected, invalidate
binary stream frame  -> validate against active c 05 mask, discard, continue
other payload         -> protocol error, invalidate
```

不得仅用 `IsASCIIFrame` 把任意非 ASCII payload 都视为可丢弃尾帧。必须校验：

- payload 首字节为 `0x01`；
- payload 长度与当前 `c 05` 内容掩码一致；
- 必要时校验序号或其他稳定帧头字段；
- 数据字段至少满足无 NaN/Inf 等现有 `ParseStreamFrameEx` 规则。

## 6. 具体整改

### 6.1 共享协议层

目标文件：

- `shared/device-sdk/go/protocol/daq_p1604_frame.go`
- `shared/device-sdk/go/protocol/conn_helpers.go`

改动：

1. 保留 `FrameReader` 的跨调用缓冲语义，明确 `Reset()` 只能在确认位于协议边界或即将关闭连接时调用。
2. 将 `P1604ReadCommandACK` 的固定 20 帧上限改为：
   - 单一总时间预算；
   - 单一总尾帧字节预算；
   - 每帧严格校验。
3. 建议接口接收尾帧校验函数，避免共享协议层绑定某个业务 profile：

```go
type P1604ResidualFrameValidator func(payload []byte) error

func P1604ReadCommandACK(
    reader *FrameReader,
    conn net.Conn,
    deadline time.Time,
    maxResidualBytes int,
    validateResidual P1604ResidualFrameValidator,
) error
```

4. 为减少一次性改动，可先增加新 helper，再迁移调用点，最后删除旧签名；不得长期维护语义不同的两套 ACK parser。
5. `Nxx`、unexpected ASCII、非法长度、非法尾帧、字节预算超限和超时均返回可被调用方识别的协议/连接错误。
6. helper 只使用 `time.Until(deadline)` 设置 soft deadline，不创建新的完整超时预算。覆盖整个操作的 owner timer 由上层创建，deadline 到期时直接 Close 捕获的连接。
7. timeout 或协议边界不可信时由 owner 关闭连接；helper 不应在调用方不知情的情况下留下可复用状态。

尾帧字节预算统一按 wire bytes 计数：每个被跳过帧计 `2 + len(payload)`，包含长度前缀；`FrameReader` 在 Stop 前已缓存、但在 Stop ACK 等待阶段被补齐和消费的帧同样计入。`00 00` 无数据标记如继续被协议允许，也按 2 wire bytes 计入并必须可观测，不能由 `FrameReader` 静默吞掉后绕过预算。测试必须覆盖恰好达到预算、超出 1 字节和 Stop 前已缓存数据三种边界。

尾帧字节预算必须基于工程上界而非平均帧数。1 MiB 可作为待实机校准的初始候选值，但不是 TCP 接收窗口的通用上界；最终值需要以真实 1kHz 长时间积压测试确定，并在代码中记录依据。

### 6.2 独立 DAQ-P1604

目标文件：`projects/daq-p1604/apps/desktop-wails/adapters/hardware/p1604_adapter.go`

改动：

1. 删除 Stop 路径发送 `c 02 1` 前的 `driver.frameReader.Reset()`。
2. 删除 Disconnect 停流路径发送 `c 02 1` 前的 Reset。
3. 将 `p1604Driver.sendCommandACK` 迁移到共享 ACK helper，避免独立实现和 Wind-DAQ 继续漂移。
4. Stop ACK 前的尾帧使用当前 profile 的时间戳和大气数据配置调用 `ParseStreamFrameEx` 校验。
5. 任意 Stop 错误继续复用现有 `handleConnectionLost`，并统一返回 `reconnect required`。
6. 只有 ACK 成功后才设置 Connected 并启动 idleReadLoop。
7. 将已有裸 `operationMu` 升级为支持有界获取的 operation gate，覆盖完整 Stop 事务。不能依赖 Close 连接间接保证 `sync.Mutex.Lock()` 返回，因为前序 owner 可能阻塞在非 I/O 路径。
8. `Disconnect` 不得在持有 operation owner 时调用会再次获取 owner 的公开 `StopAcquisition`。应提取 `stopAcquisitionOwned` 等私有 helper，由已持有 owner 的 Stop 和 Disconnect 复用，或让 Disconnect 自行完成 best-effort Stop。
9. Disconnect 中停流失败最终仍保持 Disconnected；交互式 Stop 失败才进入 Error 并要求重连。
10. 与 Wind-DAQ 使用相同的连接身份/代次规则和单一绝对 deadline，确保 gate 等待本身也能在预算内取消。

### 6.3 Wind-DAQ

目标文件：`projects/wind-daq/services/api-go/internal/adapters/hardware/daq_p1604.go`

改动：

1. 为 `DAQP1604` 增加支持有界获取的 operation gate，串行化完整生命周期和命令事务；禁止直接使用不可取消的裸 mutex 等待作为唯一机制。
2. Stop 入口创建一个绝对 deadline 和一个绑定连接代次的 owner timer，覆盖 gate 等待、readLoop join、Write、尾帧和 ACK Read；不得在各阶段重置完整预算。
3. Stop 保持 Stopping，直到 readLoop join 和 `c 02 1` ACK 成功。
4. Stop ACK 前不调用 `frameReader.Reset()`；当前 Wind-DAQ Stop 已未调用 Reset，应保持此行为并补测试锁定。
5. `c 02 1` 返回任何错误，包括 `Nxx`，均调用 `invalidateConnection(expectedConn, ...)` 并返回 `reconnect required`。
6. ACK 成功后清理 `readLoopDone`、stop channel 等生命周期字段，再转 Connected。
7. Start 必须等待前一 Stop operation 完成，不能仅等待 readLoopDone。
8. `Disconnect` 不得在已持有 gate 时递归调用公开 `StopAcquisition`；应调用不重复获取 gate 的私有 owned helper。
9. 多等待者超时或旧连接 owner 触发时，必须通过连接身份/代次比较避免关闭重连后的新连接。

### 6.4 非目标

本轮不做以下扩展：

- 不新增静默 drain；
- 不扫描原始 TCP 字节搜索字符 `A`；
- 不实现逐字节猜测长度前缀的 resync；
- 不用固定帧数推断 Stop 完成；
- 不改变 P1604 数据帧内容、通道顺序或时间戳格式；
- 不为未知旧固件增加无证据的兼容分支。

## 7. 测试计划

### 7.1 共享 FrameReader / ACK helper

必须新增：

1. **半帧保留**：预先让 `FrameReader` 缓存 `[len]+部分 payload` 并返回 timeout；随后补齐尾部和 ACK，确认 helper 完成半帧、跳过它并读到 ACK。
2. **零尾帧**：直接收到 `[00 03 A]`，立即成功。
3. **大量合法尾帧**：至少 1000 个完整压力帧后收到 ACK，不能因超过 20 帧失败。
4. **总字节预算**：持续合法帧超过预算，连接失效。
5. **非法尾帧**：合法长度但错误 header、错误 payload 长度或 NaN/Inf，连接失效。
6. **设备拒绝**：尾帧后收到精确格式 `N` + 两位十进制错误码（如 `N05`），返回拒绝错误；`N`、`NO`、`N05junk` 等格式属于协议错误。
7. **ACK 缺失**：持续尾帧直至总超时，watchdog Close，返回 sentinel。
8. **deadline ignored**：连接 double 忽略 read/write deadline，只在 Close 后返回；owner 必须在预算内收敛。

### 7.2 独立应用

必须新增：

1. Stop 时 FrameReader 已缓存半帧，确认未调用 Reset，Stop 成功且连接保留。
2. Stop ACK 前 1000 个合法帧，Stop 成功。
3. Stop ACK 前非法帧，调用 `handleConnectionLost`，driver 被删除，状态为 Error。
4. `c 02 1` 返回 `N05`，连接失效并提示重连。
5. Stop 成功后立即 Start，命令顺序严格为 `c 02` ACK 完成后才出现 `c 00/c 05/c 01`。
6. Disconnect 时半帧存在，停流请求不会因 Reset 人为错位；无论停流结果如何最终连接关闭。
7. Disconnect 在采集中调用不会因 owned Stop helper 重复获取 operation gate 而自锁。
8. Stop Write 在忽略 write deadline 的连接上阻塞时，绝对 deadline owner Close 连接并在预算内返回。
9. 至少 3 个生命周期操作排队时，旧等待者超时不得关闭重连后的新连接，也不得由迟到 timer 关闭已完成重验的连接。

### 7.3 Wind-DAQ

必须新增：

1. 并发 Stop 与 Start，断言不会交叉命令，Start 只能在 Stop operation 完成后执行。
2. 并发 Stop 与 ApplyConfig/SetUnit，断言命令事务串行。
3. Stop ACK 前大量合法尾帧，成功保留连接。
4. Stop 返回 `N05`，`d.conn=nil`、`frameReader=nil`、状态 Error、错误包含 `reconnect required`。
5. readLoop join 超时后不发送 `c 02 1`，关闭且只关闭捕获的旧连接。
6. deadline-ignoring connection 下 operation gate 等待和 ACK Read 均由同一个绝对 deadline owner 有界收敛。
7. `c 02 1` Write 在忽略 write deadline 的连接上阻塞时，owner Close 并毒化连接。
8. Disconnect 在采集中调用不发生嵌套 Stop 自锁，最终为 Disconnected。
9. 至少 3 个 Start/Stop/Disconnect 排队并穿插重连，旧代次 timer 不得误杀新连接。

## 8. 验证命令

```powershell
$repoRoot = (Resolve-Path ".").Path

# 共享协议
Push-Location (Join-Path $repoRoot "shared/device-sdk/go")
go test -race ./protocol -count=1
go vet ./...
Pop-Location

# 独立 DAQ-P1604（独立模块）
Push-Location (Join-Path $repoRoot "projects/daq-p1604/apps/desktop-wails")
$env:GOWORK="off"
go test -race ./adapters/hardware -count=1
go test ./... -count=1
Pop-Location

# Wind-DAQ
Push-Location (Join-Path $repoRoot "projects/wind-daq/services/api-go")
go test -race ./internal/adapters/hardware -count=1
go test ./internal/... ./api/... -count=1
go vet ./...
Pop-Location
```

实机验证至少包括：

1. 1ms、2ms、5ms、10ms、100ms 周期各执行不少于 20 次 Start/Stop；
2. 每轮记录 `c 02` 后 ACK 前跳过的帧数、尾帧总字节数、Stop 总耗时；
3. 高频采集至少持续 10 秒后 Stop；
4. Stop 成功后同连接立即重新 Start；
5. Stop 成功后同连接读取/写入单位配置；
6. 问题 Windows 电脑执行 deadline 失效场景，验证超时后 Close 并要求重连。

## 9. 验收标准

- Stop 前不再 Reset 可能包含半帧的 `FrameReader`。
- Stop 正确处理 `0..N` 个完整合法尾帧和一个带长度前缀的 ACK。
- 高频下 ACK 前超过 20 帧不会误断线。
- Stop 失败后不存在“本地 Connected、设备仍推流”的状态分裂。
- 两套实现均不存在并发 reader，也不存在交叉命令/ACK 串线。
- Windows deadline 失效时，所有 Stop 路径均由独立 owner 在有界时间内 Close。
- 快速 Stop/Start 不出现 `invalid frame length`、unexpected binary response 或复用脏连接。
- 全部定向测试、race 测试和 vet 通过。

## 10. 实施顺序

1. 先补共享协议层半帧和大量尾帧失败测试，证明当前 Reset 与 20 帧上限问题。
2. 改造共享 ACK helper 为总时间 + 总字节预算，并加入尾帧严格校验。
3. 迁移独立 DAQ-P1604，删除 Stop/Disconnect 前 Reset。
4. 为 Wind-DAQ 增加 operation owner，统一 Stop 失败毒化。
5. 执行全量 race/vet 和实机矩阵。
6. 使用 GitNexus `detect_changes` 检查受影响流程，确认只触及预期的 P1604 生命周期和共享协议。
7. 验证完成后将本文状态改为“已完成”，附测试输出摘要和实机记录路径。

## 11. 回退策略

如果整改后出现未预期的旧固件兼容问题：

- 回退到上一版本并要求 Stop 后断开重连，不恢复 Stop 前 Reset；
- 不允许以增加逐字节 resync 或放宽非法帧校验作为临时修复；
- 保存原始 TCP 抓包、固件版本、`c 05` 内容掩码和错误帧十六进制，再决定兼容策略。
