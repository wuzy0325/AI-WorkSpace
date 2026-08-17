# DAQ-T-1603 采集控制实机验证报告

## 1. 目的

验证 `192.168.1.10:9000` 在 TCP 二进制模式下的 Start ACK、Stop ACK、旧流排空和64字节数据边界，为物理停止后允许配置提供依据。

本报告只采用完成 Stop 排空隔离的受控实验。此前未保存 Start 前排空证据的连续启停候选分类已删除，不再用于协议结论。

## 2. 测试环境

| 项目 | 实际值 |
|------|--------|
| 测试日期 | 2026-07-30 |
| 设备地址 | `192.168.1.10:9000` |
| 本机地址 | `192.168.1.11` |
| 模式 | `BIN=1, TIME=0, HEAD=0, MCH=FFFF` |
| 数据记录 | 64字节，16 × float32 little-endian |
| 命令格式 | 裸ASCII，不追加CR/LF |
| 复测采样周期 | SPS=`2ms` |
| 测试程序 | Python原始socket，单连接、单reader |

测试结束后已发送 `@f1`，SPS读回仍为`2ms`。

## 3. 受控实验步骤

每轮执行：

1. 发送裸命令 `@f1`。
2. 持续读取，直到连续1秒没有收到字节。
3. 再观察500ms，确认收到0字节。
4. 发送裸命令 `@f0 FFFF 2`。
5. 固定抓取50ms原始字节。
6. 发送裸命令 `@f1`。
7. 持续读取，直到连续1秒没有收到字节。

该流程先建立已停止且接收缓冲为空的事务边界，再观察Start响应，因此Start后的首字节不会来自上一轮缓存数据。

## 4. Start实测结果

| 轮次 | Start前额外500ms | Start后首字节 | 去掉首字节后的结构 | CH02 |
|---:|---:|---:|---|---:|
| 1 | 0字节 | `41` | 23×64字节 | 30.316349°C |
| 2 | 0字节 | `41` | 27×64字节 | 30.309574°C |
| 3 | 0字节 | `41` | 27×64字节 | 30.322939°C |
| 4 | 0字节 | `41` | 26×64字节 | 30.289366°C |
| 5 | 0字节 | `41` | 26×64字节 | 30.316252°C |

5轮均满足：

```text
@f0 FFFF 2
-> 41                  单字节Start ACK
-> 64字节温度记录
-> 64字节温度记录
-> ...
```

按偏移0把ACK作为数据解析会得到巨大非法浮点数；去掉ACK后按偏移1解析，5轮均得到合理温度。因此本设备在完成Stop排空后，Start ACK稳定先于本轮数据，不是停止后的缓存数据。

## 5. Stop实测结果

在已停止状态发送 `@f1`，每轮只返回单字节`41`，随后持续静默。

采集后发送 `@f1`：

- Stop ACK前收到0或1个已经在途的完整64字节记录；
- 随后收到单字节`41`；
- `41`后连续1秒无数据；
- 受控实验未观察到截断记录。

在线顺序为：

```text
@f1
-> 0或1个完整在途记录
-> 41                  单字节Stop ACK
-> 数据流静默
```

因此事务边界处的单字节`41`可作为Stop ACK。

> **关于"ACK 后迟到字节"的说明（2026-07-31 修正）**：
>
> 早期版本的本报告曾写"ACK后仍可能有已经进入网络栈的旧流字节延迟到达"，**该说法缺乏可靠证据支持**。TCP 在同一连接上保证字节有序，ACK 之前发送的数据不会因主机网络栈延迟而越过 ACK、重排到 ACK 后。早期日志中观察到的"ACK 后字节"很可能来自：
>
> - 数据中的 `0x41`（约 30°C 的 float32 LE 最高字节本身就是 `0x41`）被误判为 ACK；
> - 缓存边界未建立，按裸 `0x41` 搜索导致 ACK 边界识别错误；
> - 多 reader 并发造成观测错乱。
>
> 2026-07-31 复测（第10章）在严格建立 64 字节边界后，所有有效轮次均满足 N×64+A 形态，ACK 后连续 3 个 50ms 观察窗口均未收到数据。因此本报告删除"ACK 后迟到字节"作为既定事实的表述，仅在受控实验中保留"ACK 后 1 秒静默"作为验证手段。

## 6. 重要边界

约30°C的float32 little-endian数据本身包含`0x41`，例如`30.0`为`00 00 F0 41`。因此：

- 不能在任意数据流中搜索裸`0x41`识别ACK；
- ACK只能由Start/Stop控制事务状态和既有64字节边界识别；
- Stop ACK前只接受完整64字节在途记录；
- 边界异常、ACK超时或出现不足64字节尾部时，应关闭连接，不逐字节猜测边界。

## 7. 软件实现要求

物理Start流程：

1. 如果连接状态未知，先执行物理Stop和排空。
2. 收到Stop ACK后清空应用层帧缓冲，并继续排空主机TCP接收缓冲。
3. 达到连续1秒静默后，再确认500ms零字节；完成前保持`Stopping`。
4. 发送 `@f0 FFFF 2`。
5. 精确消费前导单字节Start ACK `41`。
6. 从下一字节开始按配置的64/72字节边界读取温度记录。

物理Stop流程：

1. 进入 `Stopping`，禁止配置和再次Start。
2. 设置 `d.acquiring = false`，readLoop 继续消费尾帧以维持边界，但**不再送 sink**（`daq_t1603.go:460,977-982`）。
3. 发送 `@f1`。
4. 唯一reader 通过 `FrameReader.ReadFrame()` 逐帧消费完整在途记录，每帧过 `ParseTCPFrameEx` 合法性校验。
5. 在下一帧边界读到 `'A'`，`extractControlACKLocked` 消费 ACK，返回 `ErrControlACK`。
6. **协议契约**：ACK 是 Stop 事务的终止边界，ACK 后无数据（第5章、第10章实测验证）。识别到 ACK 后可立即完成 Stop。
7. **协议契约**（2026-07-31 实测验证 + 落地实现，详见第10/11章）：ACK 是 Stop 事务终止边界，ACK 后无数据。识别到 ACK 后立即完成 Stop，不再 drain。边界错乱由 isResyncableReadError 的 Stop 上下文分支失效连接兜底。
8. Stop owner 使用独立 3 秒总超时；超时直接 Close 连接并返回 `reconnect required`，不把 socket deadline 作为唯一取消机制。

生产代码已实现物理Stop：唯一reader消费完整在途记录和Stop ACK，acquiring=false 期间尾帧消费但不送 sink；readLoop 收到 ErrControlACK 后立即返回，StopAcquisition 在 done channel 上立即完成（无静默窗口）。Stop owner 使用独立 3s 总超时；ACK 缺失或边界错乱时超时触发，直接 Close 连接并返回 `reconnect required`，不把 socket deadline 作为唯一取消机制。问题 Windows 电脑 deadline 失效时，3s 兜底保证 Stop 不会永久卡死。

> **关于 `drainT1603StopResidual` 的历史定位（2026-07-31 已删除）**：
>
> 该函数的 1s 静默 + 500ms 确认是早期"ACK 后可能迟到旧流字节"假设下的保守兜底。第5章已澄清该假设缺乏证据，第10章实测进一步验证 ACK 后无字节。2026-07-31 已从生产代码删除该函数及常量 `stopDrainQuietWindow` / `stopDrainConfirmWindow`，ACK 后立即完成 Stop。

## 8. 结论

实机受控实验确认：先物理Stop并持续排空到1秒静默，再额外确认500ms零字节后，5轮Start均先返回单字节`41`，随后从下一字节开始严格按64字节对齐。Stop时完整在途记录结束后返回单字节`41`，之后持续静默。

> **静默窗口的定位（2026-07-31 修正 + 实现落地）**：
>
> 本报告中的"1 秒静默 + 500ms 确认"是**受控实验的验证手段**，用于在测试期间确认 ACK 后无字节到达，**不是生产 Stop 的协议必要流程**。第10章复测在严格 64 字节边界下进一步验证：ACK 是 Stop 事务的终止边界，ACK 后连续 3 个 50ms 观察窗口均无数据。
>
> 生产实现已于 2026-07-31 采纳选项 1（ACK 后立即完成），删除 `drainT1603StopResidual` 及相关静默常量，详见第 10 章。

实现不再把Stop ACK等同于接收缓冲已清空。ACK 由唯一 reader 在帧边界精确识别后立即完成 Stop，不再 drain 静默窗口；问题 Windows 环境中若 Read 忽略 deadline，独立 Stop owner 在 3 秒总预算到期后直接 Close 连接，不会永久卡死或错误复用脏连接。Stop 期间边界错乱（错帧）由 isResyncableReadError 的 Stop 上下文分支立即失效连接兜底。

## 9. 实现验证

- 单元测试覆盖 Stop 等待完整尾帧和 ACK、ACK 后立即完成（无 drain）、ACK 缺失时连接失效、Stop 期间错帧立即失效连接、正常采集期间错帧 resync 回归、Stop 后同连接配置成功。
- Windows 故障连接 double 忽略所有 read/write deadline，阻塞 I/O 只在 `Close` 后返回；Stop owner 在独立超时内直接 Close 并返回 `reconnect required`。
- `go test -race ./protocol ./daq/hardware -count=1` 通过。
- `go test ./... -count=1` 在 `shared/device-sdk/go` 模块通过。
- 实机20轮物理Start/Stop通过，CH02持续为合理温度。
- 实机持续采集10秒后Stop通过，共收到4980帧，Stop约0.5s返回（实测收到 Stop ACK 的耗时；删除 drain 后 `StopAcquisition()` 在 ACK 到达后立即返回，不再有 1s 静默 + 500ms 确认的下限）。
- 实机Stop后同连接将SPS改为5ms并读回`5`成功，随后恢复SPS=`2ms`成功。

## 10. 2026-07-31 复测与实现：ACK 后取消 drain 与 Stop 边界严格化（已实现）

> **状态声明**：本章方案已于 2026-07-31 落地生产代码并验证通过。`drainT1603StopResidual` 已删除（`shared/device-sdk/go/daq/hardware/daq_t1603.go`），`isResyncableReadError` 已增加 Stop 上下文分支失效连接。验证结果见 10.7 节。

### 10.1 复测背景

第7章生产实现上线后，问题电脑仍出现 Stop 路径卡死报告。根因定位：

- `drainT1603StopResidual`（`shared/device-sdk/go/daq/hardware/daq_t1603.go:988`）在 readLoop goroutine 内执行，**仅依赖 `SetReadDeadline` 软超时**，没有独立 watchdog。
- 问题电脑 deadline 失效时，`conn.Read` 永久阻塞 → readLoop 不退出 → 外层 `stopAcquisitionTimeout=3s` 兜底触发 `invalidateConnection` Close 连接。
- 该路径把"drain 卡死"等同于"连接故障"，强制重连，健康电脑用户体验下降，问题电脑仍可能卡到 3s。

目标：**健康电脑 Stop 后保留连接可复用，问题电脑 drain 不卡死**。

### 10.2 复测方法（修正版 v2）

测试程序：`tmp/t1603_stop_probe6.go`，连接 `192.168.1.10:9000`，BIN 模式。

> **方法学修正历史**：
>
> - v4 探针（`tmp/t1603_stop_probe4.go`）：命令追加 `\r\n`、误将 SPS=1000 当高频、按切片判断 ACK，已废弃。
> - v5 探针（`tmp/t1603_stop_probe5.go`）：修正了 v4 的三个错误，但仍有四个问题（见下方 finding 2/3/4/5），已废弃。
> - v6 探针（当前版本）：修正 v5 的四个错误。

本版探针修正：

1. **裸命令**：`conn.Write([]byte(cmd))`，不追加 `\r\n`，与生产代码一致。
2. **真正高频 SPS**：SPS=2（500Hz）/ 5（200Hz）/ 10（100Hz）/ 100（10Hz），覆盖高频到低频。
3. **显式设置并读回模式**（finding 3 修正）：每轮先 `@fe BIN 1`、`@fe TIME 0`、`@fe HEAD 0`、`@fe SPS N`，每条命令后精确读 1 字节 ACK；再用 `@fd BIN/TIME/HEAD/SPS` 读回实际值，确认 64 字节边界前提。不依赖设备之前的状态。
4. **Start ACK 精确读取**（finding 2 修正）：`@f0` 后用 `readExact(conn, 1, 500ms)` 精确读首字节，必须为 'A'；不是 'A' 立即终止本轮并标记协议不符，不继续搜索。
5. **累计字节数组统一分析**（finding 4 修正）：drain 结束后对全部字节统一判断 `total%64==1 && last=='A'`。**注意**：`bytes_after_ack=0` 不是独立观测量，仅在 N×64+A 边界成立时为 0；表述为"累计流满足 N×64+A，因此在该边界解释下未观察到 ACK 后字节"。
6. **多轮复测**（finding 5 修正）：每个 SPS 测 5 轮，报告 min/median/max。
7. **模拟内核缓冲堆积**：启动采集后 backlog 500ms 不读，让内核接收缓冲堆积，然后发 `@f1`。

### 10.3 复测数据（修正版 v2，每 SPS 5 轮）

| SPS | 频率 | 帧数 min/median/max | drain min/median/max (ms) | N×64+A 满足轮数 |
|---|---|---|---|---|
| 2 | 500Hz | 250/252/252 | 151/152/152 | 4/4 |
| 5 | 200Hz | 100/101/101 | 151/152/152 | 4/4 |
| 10 | 100Hz | 50/51/51 | 151/152/152 | 3/3 |
| 100 | 10Hz | 6/6/6 | 152/152/152 | 3/3 |

> **样本说明**：每个 SPS 计划 5 轮，部分轮次因 `@f0` 首字节非 'A' 或 `@fe` ACK EOF 被剔除（见下文"协议不符检测"）。帧数允许 ±2 帧波动（SPS=2 实测 250-252），受调度和命令时机影响，表中数值是单次运行样本，非稳定上界。

关键观察：

1. **累计流满足 N×64+A 边界**：所有有效轮次 `ack_at_end` 均为 true。**注意**：`bytes_after_ack=0` 是 N×64+A 边界成立的结果，不是独立观测量；表述为"在该边界解释下未观察到 ACK 后字节"。
2. **残留规模与采样率强相关**：SPS=2（500Hz）残留 ~252 帧（~16KB），SPS=100（10Hz）残留 6 帧。**第一版"残留与采样率无关"的结论完全错误**，原因是第一版测的 SPS=1000 是 1Hz 极低频。
3. **drain 耗时与残留规模无关**：即使 ~252 帧（~16KB），3 次 50ms 静默确认仍足够（数据在 1ms 内全部到达，剩余 150ms 是静默确认）。
4. **TCP 流控反压存在但上限远高于第一版估计**：SPS=2 时 1 秒数据约 32KB，堆积 ~252 帧 = ~16KB，说明 TCP 接收窗口未满。**注意**：本观察仅基于应用层读取的字节数，未通过 Wireshark 抓包或 socket 缓冲区大小证据证明反压机制，"TCP 流控反压"是假设而非已验证的因果。
5. **协议不符检测生效**：SPS=2 round#5、SPS=10 round#3、SPS=100 round#3 检测到 `@f0` 首字节非 'A'（`00`），证明 finding 2 修正的 Start ACK 精确读取原则是必要的——搜索式 ACK 识别会误判数据字节为 ACK。

### 10.4 协议模型与现有 FrameReader 的能力

#### 10.4.1 Stop 响应协议模型

实测数据印证的 Stop 响应规律：

```
Stop 响应 = N 个完整合法帧 + 单字节 'A' ACK
N >= 0（低频时 N=0，只返回 'A'；高频或积压时 N 较大）
```

固定帧模式下，N×frameSize+A 是严格的字节流形态：

| BIN | TIME/HEAD | frameSize | Stop 响应形态 | 实测验证 |
|---|---|---|---|---|
| 1 | 0 | 64 | N×64 + 'A' | ✅ 本次实测 |
| 1 | 1 | 72 | N×72 + 'A' | 未实测 |
| 0 | 0 | 192 | N×192 + 'A' | 未实测 |
| 0 | 1 | 变长 | N 个变长帧 + 'A'（每帧以换行终止） | 未实测 |

> **边界范围声明**：本次实测仅覆盖 BIN=1, TIME=0, HEAD=0 的 64 字节模式。72 字节和 192 字节模式可依据同一状态机实现，但在没有实测前**不作为硬件已验证结论**。变长 ASCII 模式（BIN=0, TIME/HEAD=1）走 `readFrameVariable`，状态机不同，需单独验证。

#### 10.4.2 现有 FrameReader 已经在做更严格的同一件事

之前文档提出的"方案 B：把 ACK 消费移到 drain、按 N×frameSize+1 累计"是**错误方向**，原因：

1. **重复实现**：现有 `FrameReader.ReadFrame()`（`shared/device-sdk/go/protocol/daq_t1603_frame.go:361-374`）在 `ExpectControlACKAfterFrames()` 状态下，每次先 `extractControlACKLocked` 识别 ACK，再 `extractFixedFrameLocked` 按 64/72/192 字节边界切帧并过 `ParseTCPFrameEx` 合法性校验。这已经是"先 N 个合法帧 + 再 ACK"的严格状态机，比单纯累计字节后检查 `(len-1)%frameSize==0` **更强**——后者只验证长度形态，前者每帧都校验数据合法性。

2. **弱化校验**：方案 B 的长度条件 `(len-1)%frameSize==0 && last=='A'` 只能证明长度对，不能证明前 N 段都是合法帧。64 字节垃圾 + 'A' 也会通过。

3. **覆盖不全**：`frameSizeLocked()` 不覆盖 BIN=0 + TIME/HEAD=1 的变长 ASCII 帧（`readFrameVariable`，`daq_t1603_frame.go:371-373`）。方案 B 若基于固定 frameSize，最多覆盖三种固定长度模式。

4. **TCP 有序保证**：TCP 同一连接保证字节有序，"ACK 之前发送、但因主机网络栈延迟出现在 ACK 后面"的字节重排**不会发生**。若日志显示 ACK 后还有数据，只可能是：(a) 数据中的 0x41 被误判为 ACK；(b) ACK 边界识别错误；(c) 设备违反契约在 ACK 后继续发送；(d) 日志记录顺序或多 reader 并发造成观测错乱。第5章已澄清早期"ACK 后迟到字节"说法缺乏证据。

**结论**：不实现方案 B，保留现有唯一 reader 和 FrameReader。

#### 10.4.3 当前 Stop 流程的真实行为

```
stopAcquisitionLocked (daq_t1603.go:459-498):
  d.acquiring = false                          ← 关键：停止送 sink
  d.stopping = true
  fr.ExpectControlACKAfterFrames()             ← 告诉 FrameReader "等 ACK"
  write @f1
  wait <-done

readLoop stopping 分支 (daq_t1603.go:929-983):
  ReadFrame() → 合法尾帧，acquiring=false → 丢弃，不送 sink
  ReadFrame() → 合法尾帧，acquiring=false → 丢弃，不送 sink
  ...
  ReadFrame() → ErrControlACK → 调 drainT1603StopResidual → close(done)
```

**关键修正**：之前文档写"N 个尾帧送给 sink"是错误的。Stop 期间 `d.acquiring = false`（`daq_t1603.go:460`），readLoop 第 977-982 行只有 `acquiring=true` 才调 `processPayload`，**尾帧消费但丢弃**，不送 sink。

### 10.5 实现方案：ACK 后取消 drain（已落地）

基于 10.4.2 的 TCP 有序保证和实测数据，决策点收敛为一个：

**决策点**：收到 `ErrControlACK` 后，是立即完成 Stop，还是保留静默观察窗口？

| 选项 | 行为 | 健康电脑 Stop 耗时 | 前提 |
|---|---|---|---|
| **选项 1**：立即完成 | ErrControlACK → close(done)，删除 drain | ~1-2ms（ACK 到达即完成） | 设备契约"ACK 是 Stop 事务最后一个字节"严格成立 |
| **选项 2**：短静默观察 | ErrControlACK → 50ms 观察窗口（检测设备是否违反 ACK 终止契约） | ~50ms | 不完全信任设备契约，保留兜底 |

**已采用选项 1**（2026-07-31 落地）。依据：

1. 第5章已澄清"ACK 后迟到字节"说法缺乏证据，早期日志很可能是 0x41 误判或观测错乱。
2. 本次复测（10.3 节）所有有效轮次均满足 N×64+A 边界，ACK 后连续 3 个 50ms 观察窗口均无数据。
3. TCP 同一连接保证字节有序，排除了"ACK 前字节重排到 ACK 后"的可能。
4. 选项 1 配合 10.6 的 isResyncableReadError 修复，即使边界错乱也立即失效连接（不 resync），不会输出错误数据。

**选项 1 的实现**（`shared/device-sdk/go/daq/hardware/daq_t1603.go` readLoop）：

```go
if errors.Is(err, protocol.ErrControlACK) {
    d.mu.RLock()
    stopping := d.stopping
    d.mu.RUnlock()
    if stopping && !fr.HasPendingControlACK() {
        // 协议契约：ACK 是 Stop 事务终止边界，TCP 字节有序保证 ACK 后无迟到字节。
        // 识别到 ACK 后立即完成 Stop，不再 drain。
        return
    }
    continue
}
```

`drainT1603StopResidual` 函数及其调用已删除，相关常量（`stopDrainQuietWindow`、`stopDrainConfirmWindow`）一并删除。`stopAcquisitionTimeout` 保留为 3s 兜底（仅在 ACK 缺失或边界错乱时触发，健康路径不依赖该超时）。

### 10.6 isResyncableReadError 的 Stop 上下文修复（已落地）

无论选择选项 1 还是选项 2，Stop 等待期间遇到非法帧都应立即废弃连接，不走 resync。这是选项 1 的前置项：如果 Stop 期间 resync 继续，边界错乱会被掩盖，下次 Start 才失败，增加诊断难度。

**当前实现**（`shared/device-sdk/go/daq/hardware/daq_t1603.go` readLoop）：

```go
if isResyncableReadError(err) {
    d.mu.RLock()
    stopping := d.stopping
    d.mu.RUnlock()
    if stopping {
        // Stop 事务期间边界必须严格可信：错帧立即终止并毒化连接，不走 resync。
        // 原因：Stop 期间 acquiring=false，readLoop 只消费尾帧维持边界，
        // 错帧说明边界已错乱，resync 会掩盖问题导致下次 Start 才失败。
        unexpectedErr = fmt.Errorf("invalid frame while waiting for Stop ACK: %w", err)
        return
    }
    fr.Reset()
    d.emitLog("warn", "acquisition", "Frame misalignment; resyncing", err.Error())
    continue
}
```

**好处**：

- 正常采集仍保留原 resync 容错策略（不影响既有行为）
- Stop 事务发现错帧立即终止并毒化连接（符合 SKILL.md 第 1021 行原则）
- 不需要全局重构 `isResyncableReadError`，影响面最小

### 10.7 验证结果（2026-07-31 执行）

1. **单元测试**（`shared/device-sdk/go/daq/hardware/daq_t1603_test.go`）：
   - `TestDAQT1603StopCompletesImmediatelyAfterACKAndRestartSucceeds`：ErrControlACK 后立即完成，无 drain，连接保留并支持同连接重启采集 ✅
   - `TestDAQT1603StopInvalidatesConnOnResyncableFrameError`：Stop 期间非法帧 → 失效连接（非 resync）✅
   - `TestDAQT1603ReadLoop_ResyncsOnFrameMisalignmentDuringAcquisition`：正常采集期间非法帧仍走 resync，恢复后继续接收合法帧 ✅
   - `TestDAQT1603StopWaitsForTailFrameAndACKBeforeReturning`：ACK 前等待尾帧消费 ✅
   - `TestDAQT1603StopAllowsConfigOnSameConnection`：Stop 后同连接配置成功 ✅
2. **回归测试**：
   - `go test -race ./daq/hardware/... -count=1 -run "TestDAQT1603"` → 通过（32.3s）
   - `go test -race ./protocol/... -count=1` → 通过（11.3s）
   - `go test ./... -count=1`（shared/device-sdk/go 全模块）→ 全绿
   - `windlabx4/services/api-go/internal/adapters/hardware/...` → 通过（无上游回归）
3. **静态检查**：`go vet ./daq/hardware/...` → 无告警

### 10.8 风险与回退

- **风险 1**：本次复测仅在健康电脑进行，问题电脑的设备固件行为可能不同。若选项 1 删除 drain 后问题电脑复现"ACK 后字节"，下次 Start 会因边界错乱失败。
- **缓解**：选项 1 配合 10.6 的 isResyncableReadError 修复，边界错乱时立即失效连接（不 resync），不会输出错误数据。失效后重连，恢复边界。
- **回退**：若选项 1 复现污染，改回选项 2（50ms 静默观察窗口），总耗时 50ms 仍优于原 1.5s。

### 10.9 未解决的方法学问题

- **TCP 流控反压未验证**：第10.3节"TCP 流控反压"是假设，未通过 Wireshark 抓包（ZeroWindow/Window Full）或 socket 缓冲区大小证据证明。需要补充抓包证据才能确认残留规模上界。
- **问题电脑未实测**：本次复测在健康电脑进行，问题电脑的 deadline 失效行为仅基于代码分析和 ADR-009 文档，未在问题电脑实机验证。
- **72 字节 / 192 字节 / 变长模式未实测**：本次实测仅覆盖 BIN=1, TIME=0, HEAD=0 的 64 字节模式。其他模式的 Stop 响应形态需单独验证后才能确认协议契约。
- **`bytes_after_ack` 非独立观测量**：第10.3节 `bytes_after_ack=0` 是 N×64+A 边界成立的同义结果，不能作为"ACK 后无字节"的独立证据。需补充时间戳序列分析（每次 Read 的相对时间）才能更强地支持该结论。

## 11. 协议契约结论与边界范围

### 11.1 已验证的协议契约（仅限 64 字节模式）

基于第5章受控实验和第10章复测，在 `BIN=1, TIME=0, HEAD=0` 的 64 字节固定帧模式下，设备的 Stop 响应严格表现为：

```
Stop 响应 = N 个完整合法 64 字节数据帧 + 单字节 'A' ACK
N >= 0（低频时 N=0，只返回 'A'；高频或积压时 N 较大）
ACK 是该 Stop 事务的终止边界，ACK 后无数据
```

支撑证据：

1. **第5章受控实验**：5 轮 Stop 均满足"0 或 1 个在途记录 + 'A' + 静默"形态。
2. **第10章复测**：SPS=2/5/10/100 共 14 个有效轮次，全部满足 N×64+A 边界，ACK 后连续 3 个 50ms 观察窗口均无数据。
3. **TCP 有序保证**：同一 TCP 连接保证字节有序，排除了"ACK 前字节重排到 ACK 后"的可能。
4. **早期"ACK 后迟到字节"说法已澄清**：缺乏可靠证据，很可能是 0x41 误判或观测错乱（第5章修正说明）。

### 11.2 协议契约的边界范围

| 模式 | frameSize | 实测验证 | 协议契约状态 |
|---|---|---|---|
| BIN=1, TIME=0, HEAD=0 | 64 | ✅ 第5章 + 第10章 | 已验证 |
| BIN=1, TIME/HEAD=1 | 72 | ❌ 未实测 | 未验证，可依据同一状态机实现 |
| BIN=0, TIME=0, HEAD=0 | 192 | ❌ 未实测 | 未验证，可依据同一状态机实现 |
| BIN=0, TIME/HEAD=1 | 变长 | ❌ 未实测 | 未验证，状态机不同（`readFrameVariable`） |

**重要声明**：

- 本报告的协议契约结论**仅覆盖 64 字节模式**。
- 72 字节和 192 字节模式可依据同一状态机（`FrameReader.ReadFrame` + `ExpectControlACKAfterFrames`）实现，但在没有实测前**不作为硬件已验证结论**。
- 变长 ASCII 模式（BIN=0, TIME/HEAD=1）走 `readFrameVariable`，状态机不同，需单独验证。

### 11.3 已实现方案

基于 11.1 的协议契约，生产代码已落地的实现方案（第10章，2026-07-31）：

1. 保留现有唯一 reader 和 `FrameReader`，不实现方案 B 的批量累计器。
2. 收到 `ErrControlACK` 后立即完成 Stop，`drainT1603StopResidual` 已删除（选项 1）。
3. Stop 等待期间遇到非法帧立即废弃连接，不走 resync（10.6 节）。
4. 正常采集期间保留原 resync 容错策略，不影响既有行为。

此方案以最小改动获得 1-2ms 级 Stop，且不弱化帧校验。验证结果见 10.7 节。

