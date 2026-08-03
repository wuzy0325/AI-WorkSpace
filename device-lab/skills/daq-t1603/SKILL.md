---
name: daq-t1603
description: DAQ-T-1603 热电偶采集设备驱动开发指南。涵盖 ASCII 文本协议（@e3/@f0/@f1/@f3/@fd/@fe 命令体系）、TCP/串口数据帧解析、配置同步和生命周期管理。Use when writing or modifying DAQ-T-1603 driver code, debugging thermocouple data streams, adding DAQ-T-1603 features, or when user mentions DAQ-T-1603, T1603, 热电偶, thermocouple, temp model firmware, binaryFormat, BIN=1, or 16通道温度采集.
---

# DAQ-T-1603 热电偶采集设备驱动开发

## 设备概述

DAQ-T-1603 是一款 16 通道热电偶温度采集设备，支持 TCP/IP 和串口两种通信方式。

| 属性 | 值 |
|------|------|
| 通道数 | 16（CH01 ~ CH16） |
| 默认 TCP 地址 | `192.168.1.7:9000` |
| 发现端口 | UDP 7000 |
| 协议 | ASCII 文本命令 + TCP 数据流 |
| 数据格式 | 二进制 float32 LE（推荐）或 ASCII 文本 |
| 热电偶类型 | K, B, E, J, T, S, N, R, C, WRE325, WRE526, WRE520 |

固件变体：
- **标准型号（T1603）**：支持 BIN=0（ASCII）和 BIN=1（二进制）两种数据格式
- **temp 型号（FW v1.01）**：仅 ASCII 文本格式（`@fd BIN` 始终返回 `0`）

---

## 1. 快速开始

### 1.1 推荐工作流（BIN=1，二进制模式）

**连接后自动设 BIN=1**，以 64 字节 float32 LE 接收数据，解析效率最高。

```pseudocode
driver = new DAQT1603Driver(config, channels)

driver.onData((payload) => {
    // payload.values: float64[16], CH0→CH15 顺序
})
driver.onConfigSynced((hwConfig) => { })

await driver.connect()              // TCP 连接 + 自动配置同步
await driver.startAcquisition()     // 归一化 → @fe BIN 1 → @f0 → 收数据
// ... 采集进行中 ...
await driver.stopAcquisition()
await driver.disconnect()
```

### 1.2 数据格式速查

| 格式 | BIN | TIME/HEAD | 帧大小 | 编码 | 解析方式 | 推荐 |
|------|-----|-----------|--------|------|---------|------|
| 二进制 | `1` | 0 | **64 字节** | float32 LE | `readFloatLE` × 16 + `.reverse()` | **★ 推荐** |
| 二进制带时间戳 | `1` | 1 | **72 字节** | `[uint32 sec LE][uint32 ns LE]` + 16×float32 LE | 读 8B 时间戳 + 16×`readFloatLE` + `.reverse()` | 需硬件时间戳时 |
| ASCII | `0` | 0 | **192 字节** | 12 字符/字段定宽 | `trim().split(/\s+/).map(Number).reverse()` | 调试用 |
| ASCII 变长 | `0` | 1 | **不定长** | 空格分隔连续流 | 字段计数法（17 或 18 token） | 需元数据时 |
| 串口 | — | — | 46 字节 | int16 BE × 0.1℃/LSB | `readInt16BE` × 16 | 串口专用 |

> **帧路由由两个开关决定**：`binaryMode`（BIN）与 `metadataMode`（TIME 或 HEAD）。四个组合对应四种 TCP 帧格式，详见 §2.4。

### 1.3 状态机

```
Disconnected ──connect()──→ Connected ──startAcquisition()──→ Acquiring
     ↑                            ↑                                 │
     │                            └── 完整尾帧 + A_stop ─ Stopping ─┘
     └── disconnect()：发送 @f1、关闭连接、等待 readLoop 退出

Stopping ──ACK超时/边界异常──→ Error（Close连接，要求重连）
```

---

## 2. 数据格式与解析

### 2.1 二进制帧（BIN=1，推荐）

这是**生产环境推荐格式**。设备推送连续 64 字节 TCP 流，无需字符串转换，直接内存读取。

| 属性 | 值 |
|------|------|
| 帧大小 | **64 字节**（固定，无分隔符，无换行） |
| 编码 | float32 小端序（IEEE 754） |
| 通道数 | 16 |
| 通道顺序 | **CH15 → CH0**（递减），解析后需 `.reverse()` |
| 帧间距 | 由 SPS 参数决定（如 SPS=100 → 100ms/帧） |

**解析伪代码：**
```pseudocode
func parseBinaryFrame(raw: byte[64]) -> float64[16]:
    // 直接内存读取 16 × float32 LE
    for i = 0; i < 16; i++:
        values[i] = float64( readFloat32LE(raw, i * 4) )
    // ★ 设备发送 CH15→CH0，反转成 CH0→CH15
    return reverse(values)
```

### 2.1.1 二进制带时间戳帧（BIN=1 + TIME=1 或 HEAD=1，72 字节）

当 `binaryMode=true` 且 `metadataMode=true` 时，设备在 16 个 float32 之前附加 8 字节时间戳头。

| 属性 | 值 |
|------|------|
| 帧大小 | **72 字节**（8B 头 + 64B 数据，固定，无分隔符） |
| 头部 | `[uint32 秒 LE][uint32 纳秒 LE]` |
| 数据区 | 16 × float32 LE |
| 通道顺序 | **CH15 → CH0**，解析后需 `.reverse()` |
| 硬件时间戳 | `float64(sec) + float64(ns) / 1e9`（秒，纳秒精度） |

**解析伪代码：**
```pseudocode
func parseBinaryFrameWithTimestamp(raw: byte[72]) -> (ts: float64, values: float64[16]):
    sec  = uint32LE(raw[0:4])
    ns   = uint32LE(raw[4:8])
    ts   = float64(sec) + float64(ns) / 1e9
    for i = 0; i < 16; i++:
        values[i] = float64( readFloat32LE(raw, 8 + i * 4) )
    return (ts, reverse(values))
```

> **注意**：HEAD=1（仅序号）而 BIN=1 时，设备实际并不发独立序号字段——二进制模式下序号被并入时间戳头，按 72 字节定长读取即可。若需要序列号字段，应在 ASCII 变长模式下使用 HEAD=1。

**合法性校验：**
```pseudocode
func isValidFrame(values: float64[16]) -> bool:
    // 至少 50% 通道值在物理范围内
    validCount = count(values where v >= -100 and v <= 300)
    return validCount >= 8
```

### 2.2 ASCII 文本帧（BIN=0）

当 `@fd BIN` 返回 `0`，或设备为 temp 型号固件时使用。

| 帧模式 | 帧大小 | 终止符 | 触发条件 |
|--------|--------|--------|----------|
| 定长 ASCII | **192 字节**（固定） | 无 | ShowSequence=0 且 ShowTimestamp=0 |
| 变长 ASCII | **不定长**（约 210~230 字节） | 无 | ShowSequence=1 **或** ShowTimestamp=1 |

**定长模式（192 字节）：**
| 属性 | 值 |
|------|------|
| 字段宽度 | 每通道 12 字符，空格左对齐填充 |
| 分隔符 | 空格 |
| 通道顺序 | **CH15 → CH0**，解析后需 `.reverse()` |

**示例帧（CH15=39.95℃，其余为 0）：**
```
   0.000000    0.000000    0.000000    0.000000
   0.000000    0.000000    0.000000   39.952503
```

**变长模式（ShowSequence=1 或 ShowTimestamp=1）：**
| 属性 | 值 |
|------|------|
| 帧格式 | 空格分隔的连续流，**无换行** |
| 读取方式 | 字段计数法，FrameReader 启用 `metadataMode=true` |
| 字段数 | 17（仅 TIME 或仅 HEAD）或 18（两者都开） |
| 通道顺序 | **CH15 → CH0**，解析后需 `.reverse()` |

**变长帧在线形态：**
```
A0 1781600803.751855    0.000000    ...    25.500000 1 1781600803.762758    ...
↑ACK               16×12字符定宽值=192B       ↑下一帧序列号
seq=0 ts=1781600803.75                         seq=1 ts=1781600803.76
```

**变长帧完整示例（HEAD=1 + TIME=1，18 个 token）：**
```
0 1781600803.751855    0.000000    0.000000    0.000000    0.000000    0.000000    0.000000    0.000000   39.952503    0.000000    0.000000    0.000000    0.000000    0.000000    0.000000    0.000000    0.000000    0.000000
│ │                  └──────────────────────── 16 个温度值（CH15→CH0）─────────────────────────────────────────────────────────────────────────────────────┘
│ └─ 时间戳（Unix 秒，浮点）
└─ 帧序号（整数）
```
解析结果：`seq=0, ts=1781600803.751855, values[15]=39.952503, 其余=0.0`（reverse 后 `values[7]=39.952503`）。

**解析伪代码——自动检测 TIME/HEAD 组合：**
```pseudocode
func parseMetadataFrame(raw: byte[]) -> (seq?: int, ts?: float64, values: float64[16]):
    tokens = trim(string(raw)).split(/\s+/)

    // 检测第一个 token 是帧序号(整数)还是时间戳(浮点)
    if parseableAsInt(tokens[0]):
        seq = int(tokens[0])
        offset = 1
        if tokens.length > 17:
            ts = float(tokens[1])  // 第二个 token 是时间戳
            offset = 2
    else:
        ts = float(tokens[0])      // 仅 TIME=1，无序列号
        offset = 1

    values = tokens[offset..offset+15].map(parseFloat)
    return (seq, ts, reverse(values))
```

**字段数速查：**

| TIME | HEAD | Token 数 | token[0] | token[1] |
|------|------|----------|----------|----------|
| 0 | 0 | 16（定长192B） | 无 | 无 |
| 0 | 1 | 17 | seq（整数） | — |
| 1 | 0 | 17 | ts（浮点） | 无 |
| 1 | 1 | 18 | seq（整数） | ts（浮点） |

### 2.3 串口帧（46 字节）

| 偏移 | 长度 | 内容 |
|------|------|------|
| 0-1 | 2B | 帧头 `55 AA` |
| 2 | 1B | 帧长度 |
| 3 | 1B | 帧计数 |
| 4 | 1B | 帧计数反码 |
| 5 | 1B | 状态 |
| 6-7 | 2B | 版本号 |
| **8-39** | **32B** | **温度数据（16通道 × 2B）** |
| 40-43 | 4B | 备用 |
| 44 | 1B | 校验和 |

**温度解析：** 从偏移 8 开始，每通道 2 字节 **int16 大端序**，值 × 0.1 = ℃。

**校验和：** 偏移 44 的 1 字节为前 44 字节（偏移 0~43）的算术和取低 8 位，即 `checksum = sum(raw[0..43]) & 0xFF`。

```pseudocode
func parseSerialFrame(raw: byte[46]) -> float64[16]:
    assert raw.length == 46
    // 校验和验证
    expectedChecksum = sum(raw[0..43]) & 0xFF
    assert raw[44] == expectedChecksum, "串口帧校验和不匹配"
    for i = 0; i < 16; i++:
        offset = 8 + i * 2
        values[i] = float64(readInt16BE(raw, offset)) * 0.1
    return values   // 串口帧已经是 CH0→CH15 顺序，无需 reverse
```

**串口命令**（固定 6 字节，不发 `\n`）：

| 命令 | 十六进制 | 说明 |
|------|----------|------|
| 开始采集 | `55 AA 03 F0 00 00` | |
| 停止采集 | `55 AA 03 F1 00 00` | |

串口模式下不支持配置命令（`@e3` / `@fd` / `@fe`），配置需在 TCP 模式下完成。

### 2.4 帧读取器（FrameReader）

```pseudocode
class FrameReader:
    buffer: byte[]           // 内部累积缓冲区
    binaryMode: bool         // BIN 标志
    metadataMode: bool       // TIME 或 HEAD 标志

    func setBinaryMode(isBinary: bool):
        this.binaryMode = isBinary

    func setMetadataMode(enabled: bool):
        // 当 TIME=1 或 HEAD=1 时启用元数据模式
        this.metadataMode = enabled

    func frameSize() -> int:
        // ★ 四种组合对应四种帧大小
        if binaryMode and metadataMode:  return 72   // 二进制 + 时间戳头
        if binaryMode:                   return 64   // 纯二进制
        // ASCII + metadataMode 走变长分支，不返回定长
        return 192                                  // 纯 ASCII 定长

    func readFrame() -> byte[]:
        // ★ 路由：仅 ASCII + metadataMode 走变长，其余三种走定长
        if metadataMode and not binaryMode:
            return this.readFrameVariable()
        else:
            return this.readFrameFixed(frameSize())

    // —— 定长模式（64 / 72 / 192 字节）——
    func readFrameFixed(frameSize: int) -> byte[]:
        // 检查 buffer 是否有完整帧
        if buffer.length >= frameSize:
            return buffer.extract(frameSize)

        // 从连接读取并累积
        tmp = conn.read(frameSize)
        buffer.append(tmp)
        if buffer.length >= frameSize:
            return buffer.extract(frameSize)
        return null  // 仍需等待更多数据

    // —— 变长模式（TIME=1 或 HEAD=1，空格分隔连续流）——
    func readFrameVariable() -> byte[]:
        // 尝试用 18 或 17 个字段提取一帧，并用解析器验证
        for need in [18, 17]:
            end = findFieldEnd(buffer, need)
            if end >= 0 and parseMetadataFrame(buffer[0..end]) succeeds:
                frame = buffer[0..end]
                buffer = buffer[end..]
                return frame

        // buffer 中字段不足，从连接读取更多数据
        tmp = conn.read(256)
        buffer.append(tmp)
        // 再次尝试提取...
        return retry

    // 辅助：扫描 buffer 前 N 个空白分隔字段的结束位置
    func findFieldEnd(buf: byte[], n: int) -> int:
        count = 0
        inField = false
        for i, b in buf:
            if b 是空白字符:
                if inField:
                    count++
                    if count == n: return i
                inField = false
            else:
                inField = true
        if inField:
            count++
            if count == n: return buf.length
        return -1

    func reset():
        buffer.clear()
        // ★ metadataMode 不清零，保留当前模式配置
```

---

## 3. 在线字节参考

### 3.1 命令在线字节

**命令以裸 ASCII 字符串发送，不追加任何换行或终止符。** 驱动直接 `conn.Write([]byte(cmd))`，cmd 即命令本身。响应边界按命令分别定义，不能假设所有响应都有 `\n` 或 `\r\n`。

| 命令 | ASCII | 十六进制 |
|------|-------|---------|
| `@e3` | `@e3` | `40 65 33` |
| `@f0 FFFF 2` | `@f0 FFFF 2` | `40 66 30 20 46 46 46 46 20 32` |
| `@f1` | `@f1` | `40 66 31` |
| `@f3 0KKKKKKKKKKKKKKKK0` | `@f3 0KKKKKKKKKKKKKKKK0` | `40 66 33 20 30 4B×16 30`（20 字节） |
| `@fd BIN` | `@fd BIN` | `40 66 64 20 42 49 4E` |
| `@fd MCH` | `@fd MCH` | `40 66 64 20 4D 43 48` |
| `@fd SPS` | `@fd SPS` | `40 66 64 20 53 50 53` |
| `@fe BIN 1` | `@fe BIN 1` | `40 66 65 20 42 49 4E 20 31` |
| `@fe BIN 0` | `@fe BIN 0` | `40 66 65 20 42 49 4E 20 30` |
| `@fe SPS 100` | `@fe SPS 100` | `40 66 65 20 53 50 53 20 31 30 30` |
| `@fe TIME 0` | `@fe TIME 0` | `40 66 65 20 54 49 4D 45 20 30` |

### 3.2 实机响应证据（2026-07-29，`192.168.1.10:9000`）

所有命令均裸发、无终止符。以下结果在设备空闲且本机独占连接时测得；只读查询曾分别独立连接和同一连接顺序复测。

| 命令 | 实际响应 | 在线字节 | 证据状态 |
|------|----------|----------|----------|
| `@e3` | `KTTTTTTTTTTTTTTT` + LF | 16 字节类型 + `0A`，共 17 字节 | 实机已验证 |
| `@fd MCH` | `FFFF` | `46 46 46 46` | 实机已验证 |
| `@fd SPS` | `2` | `32` | 实机已验证，值随配置变化 |
| `@fd BIN` | `1` | `31` | 实机已验证 |
| `@fd TIME` | `0` | `30` | 实机已验证 |
| `@fd HEAD` | `0` | `30` | 实机已验证 |
| `@fd AVG` | `4` | `34` | 实机已验证，值随配置变化 |
| `@fd TYPE` | `0` | `30` | 实机已验证 |
| `@fd TRIG` | `0` | `30` | 实机已验证 |
| `@fd TNUM` | `1` | `31` | 实机已验证，值随配置变化 |
| `@fe BIN 1` | `A` | `41` | 实机已验证；发送值与设备原状态一致 |
| `@fe TIME 0` | `A` | `41` | 实机已验证；发送值与设备原状态一致 |
| `@fe HEAD 0` | `A` | `41` | 实机已验证；发送值与设备原状态一致 |

本轮**未发送** `@f3`、`@fd CHECK`、`@fe MCH`、`@fe AVG`、`@fe TYPE`、`@fe TRIG`、`@fe TNUM`，也未发送 `@fe BIN 0`、`@fe TIME 1`、`@fe HEAD 1` 等相反值。因此不得把这些命令/参数组合的响应终止符或 ACK 行为写成“实机已验证”。`@f0`、`@f1` 和 `@fe SPS` 的后续实机证据见 §3.3。

**错误响应 `E` 的触发条件：** 命令格式错误、参数越界、不支持的操作（如向 temp 型号发送不支持的命令）。驱动收到 `E` 后应终止当前操作并上报错误，不应重试同一命令。

> **响应读取规则**：当前生产配置同步只查询确定边界：`@e3` 精确读 17 字节并移除末尾单个 LF；`MCH/BIN/TIME/HEAD/TYPE/TRIG` 按实机固定长度精确读。`SendCommandExact(n)` 读满后立即返回，不再探测尾部。`SPS/AVG/TNUM` 虽已实机观察到无终止符响应，但值长度可变，连接同步阶段不查询，沿用已保存配置。

### 3.3 采集控制实机证据（2026-07-30，`192.168.1.10:9000`）

受控测试条件：`BIN=1`、`TIME=0`、`HEAD=0`、SPS=`2ms`，命令均裸发；同一TCP连接执行5轮物理`@f0 FFFF 2`/`@f1`。每轮Start前先Stop，持续读取到1秒静默，再额外确认500ms零字节。Start后固定抓取50ms，再Stop并读取到1秒静默。测试结束后设备保持SPS=`2ms`且已停止。

#### 3.3.1 Start ACK 在清空旧流后先于数据

`@f0` 的控制响应为单字节 `A`，与采集数据共用同一条无帧头 TCP 字节流。隔离复测先发送`@f1`，持续读取到连续1秒静默，再额外观察500ms零字节，然后发送`@f0`。5轮均首先收到单字节`A`，去掉该字节后分别得到23/27/27/26/26个严格对齐的64字节记录，CH02均约30.3°C。生产驱动必须先物理Stop并排空，再把事务起点的单字节`A`作为Start ACK消费。

> 数据帧内部经常包含字节 `0x41`：约 30°C 的 float32 LE 最高字节本身就是 `41`。在无帧头流中搜索字节 `A` 不能识别 ACK，必须结合当前控制事务和已知帧边界。

#### 3.3.2 Stop ACK 后仍需排空主机接收缓冲

隔离复测中，在已停止状态发送`@f1`只返回单字节`A`并持续静默；采集后发送`@f1`时，先收到0或1个已经在途的完整64字节记录，再收到单字节`A`。该`A`可作为Stop ACK，但不能单独证明主机TCP接收缓冲已经排空。生产路径必须在ACK后继续丢弃迟到旧流字节，达到连续1秒静默后再确认500ms零字节。受控实验未观察到截断帧。

#### 3.3.3 驱动约束

TCP保证字节顺序。隔离实验已经确认旧数据残留是早期误判的原因，主机驱动遵守以下规则：

- Start、Stop、配置和 Disconnect 由同一连接 owner 串行执行，整个连接只有一个 reader。
- `Acquiring` / `Stopping` 状态禁止发送频率、类型、BIN/TIME/HEAD 等配置命令。
- Start 端**容忍 ACK 缺失/迟到**（§3.3.4）：首字节 `0x41` 消费为 ACK；首字节非 `0x41` 用偏移0/偏移1帧合法性对齐，不判错。
- 正常采集路径允许**丢弃 1 个前导字节**做单字节自愈（迟到的 ACK / 残杂字节），但 Stop 事务路径保持严格 `N×64+ACK` 校验，不走自愈。
- Stop后由唯一reader继续消费完整尾帧和事务边界上的单字节`A`，静默窗口确认 `N×64+ACK` 后清空应用层缓冲；边界无法确认时Close连接、进入Error并要求重连。
- **禁止裸搜 `0x41` 判定 ACK/帧边界**（约 25.9°C 的 float32 LE 高字节本身就是 `0x41`），必须用帧合法性校验。
- **快速点击安全**：StopAcquisition 必须在持锁期间把 `stopChs`/`channels` 从 map 移除后再锁外关闭，避免与 `OnReadLoopExit` 回调对同一 channel 二次 close（panic: close of closed channel）。

#### 3.3.4 Start ACK 偶发迟到（2026-07-31 实机探针实测，`192.168.1.10:9000`）

> **结论先行**：`@f0` 的 Start ACK 单字节 `A` 是**每次都会发送**的，但**不是固定排在第一字节**。在约 7%~15% 的启动中，第一个64字节数据帧会**先于** ACK 到达（1000Hz 探针 10/60，5Hz 探针 3/20，1秒探针 2/30）。此现象已在多个独立工具上复现：生产驱动、原始 socket 探针、连续读取1秒探针。设备固件在 `@f0` 后并行发送 ACK 与数据流，发送顺序不保证（固件时序竞争），与采样率无关。

**探测方法与数据（30轮，SPS=1ms，每轮 `@f0` → 读100ms → `@f1` → 连续读1秒）：**

```text
正常轮次（28/30）:
  cycle=01 first=ACK  firstIn=6.0ms  frames100ms=100
          stop1s total=129  last=0x41  windows(100ms)=129,0,0,0,0,0,0,0   ← N×64+ACK 且之后900ms零字节

ACK迟到轮次（2/30）:
  cycle=03 first=0x00  firstIn=1.1ms  frames100ms=103
          stop1s total=130  last=0x41  windows(100ms)=130,0,0,0,0,0,0,0   ← 多出1个前置字节
  cycle=08 first=0x00  firstIn=0.0ms  frames100ms=101
          stop1s total=66   last=0x41  windows(100ms)=66,0,0,0,0,0,0,0
```

**关键事实：**

1. **Stop 后缓存是干净清空的**：所有轮次 Stop 残余在第一个100ms窗口内（实际前~9ms）全部到达，随后连续900ms零字节。即使Stop后连续读满1秒再Start，下一轮仍会偶发首字节`0x00`（cycle 03→04、08→09 均如此）。因此 `first=0x00` **不是** Stop 残留。
2. **Start ACK 迟到产生 1 字节边界偏移**：凡 `first=0x00` 的轮次，该轮 Stop 残余都比 `N×64+ACK` 多出**1个前置字节**。原因：迟到的 `A(0x41)` 被当作数据字节消费，导致后续 64 字节定长边界整体偏移 1 字节，该偏移持续到 Stop，表现为残余多 1 字节。
3. **与旧日志的对应关系**：历史上“Frame misalignment / binary frame values out of expected range”“expected Start ACK got 0x00”都源于此，而不是设备异常温度或 Stop 清空缺陷。审计报告第10.3节中被“剔除”的 `@f0` 首字节非 `A`（`00`）轮次即此现象。

**驱动实现要求（Start 端必须容忍 ACK 缺失/迟到，正常采集需单字节自愈）：**

- `@f0` 后**不能**把“首字节必须为 `0x41`”当作硬性失败前提。
- 首字节为 `0x41` → 正常消费为 Start ACK。
- 首字节非 `0x41` → 不立即报错；用**偏移0/偏移1的帧合法性**确定真实边界：
  - 偏移0帧合法 → 数据优先且无前导残杂字节，ACK 视为未发送，直接消费首帧；
  - 偏移1帧合法 → 存在1个前导残杂字节（上一事务尾字节），丢弃后对齐消费首帧；
  - 两者都非法 → 才判为协议错位并失效连接。
- **正常采集路径单字节自愈**：采集期间若 64 字节边界帧非法，尝试丢弃1个前导字节重试一次（该字节通常是迟到的 ACK 或残杂字节）。只允许丢1字节，且 Stop 事务路径保持严格 `N×64+ACK` 校验，不走自愈。
- 禁止裸搜 `0x41`：约 25.9°C 的 float32 LE 高字节本身就是 `0x41`，数据里常出现，必须用帧合法性校验而非字节值判断。

> **Stop 清空验证结论**：Stop 响应严格为 `N×64 + 尾部ACK`（`N>=0`），全部在 `@f1` 后约 1~9ms 内到达，之后设备保持静默。生产 Stop 采用“150ms 静默窗口确认 + 整段 `N×64+ACK` 校验 + 全部消费后 Reset”即可保证缓存清空，无需秒级等待。

> **迟到 Start ACK 落在 Stop 窗口（2026-07-31 快速点击复现）**：短采集（如 20ms）时，迟到的 Start ACK `A` 可能拖到 Stop 收集窗口才到达，与 Stop ACK 拼成 `raw=41 41`，不满足 `N×64+ACK`。`finalizeStopResponseLocked` 必须容忍：原缓冲非法时，丢弃**1个前导 `A`**（即迟到 Start ACK）再验证。仅原缓冲非法时才丢，合法数据帧首字节为 `0x41` 时不会误删。已用 60 轮 × 3 次短采集压力测试（Start→20ms→Stop）验证 0 警告 0 错误。

### 3.4 数据帧十六进制展开

#### 二进制帧在线形态（BIN=1，64 字节）

实测 TCP 抓包（CH15=39.95℃，CH14~CH0=0.0℃）：

```
偏移 0x00:  C3 F5 21 42  00 00 00 00  00 00 00 00  00 00 00 00
            └─CH15=39.95  └─CH14=0.0    └─CH13=0.0    └─CH12=0.0

偏移 0x10:  00 00 00 00  00 00 00 00  00 00 00 00  00 00 00 00
            └─CH11=0.0    └─CH10=0.0    └─CH9=0.0     └─CH8=0.0

偏移 0x20:  00 00 00 00  00 00 00 00  00 00 00 00  00 00 00 00
            └─CH7=0.0     └─CH6=0.0     └─CH5=0.0     └─CH4=0.0

偏移 0x30:  00 00 00 00  00 00 00 00  00 00 00 00  00 00 00 00
            └─CH3=0.0     └─CH2=0.0     └─CH1=0.0     └─CH0=0.0
```

常见温度值的 float32 LE 字节对照：

| 温度 | 十六进制 | 字节（小端） |
|------|---------|-------------|
| 0.0 ℃ | `0x00000000` | `00 00 00 00` |
| 25.0 ℃ | `0x41C80000` | `00 00 C8 41` |
| 39.95 ℃ | `0x4221F5C3` | `C3 F5 21 42` |
| 100.0 ℃ | `0x42C80000` | `00 00 C8 42` |
| -10.5 ℃ | `0xC1280000` | `00 00 28 C1` |

#### ASCII 帧在线形态（BIN=0，192 字节）

```
偏移 0x00:  20 20 20 20 30 2E 30 30  30 30 30 30  20 20 20 20  "    0.000000    "
偏移 0x10:  30 2E 30 30 30 30 30 30  20 20 20 20  30 2E 30 30  "0.000000    0.00"
... 中间为 12 个 "    0.000000" 重复 ...
偏移 0x80:  35 32 35 30 33 20 20 20  20 20 20 20  20 20 20 20  "52503           "
         CH15=39.952503 结束 ──────→ 第 192 字节
```

关键特征：每字段 12 字符定宽、空格左对齐、**无 `0A` 或 `0D 0A` 结尾**。

#### 串口帧在线形态（46 字节）

CH0~CH3 温度为 0.0~3.0℃ 时的十六进制：

```
55 AA 2E 12 ED 00 01 00  00 00 00 0A 00 14 00 1E ...
│  │  │  │  │  │  │  │   │  │  │  │  │  │  │  │
│  │  │  │  │  │  │  │   CH0  CH1  CH2  CH3
│  │  │  │  │  │  │  │   0.0  1.0  2.0  3.0℃
│  │  │  │  │  │  └──版本号
│  │  │  │  │  └──状态
│  │  │  │  └──帧计数反码
│  │  │  └──帧计数
│  │  └──帧长度(46=0x2E)
│  └──帧头 AA
└──帧头 55
```

---

## 4. 连接生命周期

### 4.1 Connect

```pseudocode
func connect():
    tcpSocket.connect(config.host, config.port, timeout=5s)
    tcpSocket.setKeepAlive(true, interval=10s)
    connected = true
    // 延迟 300ms 后自动执行配置同步
    startAsync(syncHardwareConfig, delay=300ms)
```

### 4.2 配置同步（syncHardwareConfig）

连接后 300ms 自动执行，先逐条查询设备当前配置，**再强制归一化为最稳的二进制模式**（BIN=1 / TIME=0 / HEAD=0）。归一化三命令必须全部成功，否则保留设备实际状态不动。

```pseudocode
func syncHardwareConfig():
    sleep(300ms)   // 连接建立后等设备就绪

    // —— 1. 查询设备当前参数 ——
    config.thermocoupleTypes = trimSuffix(sendCommandExact("@e3", 17), "\n")
    config.channelMask       = sendCommandExact("@fd MCH", 4)
    // SPS / AVG / TNUM 无终止符且长度可变，连接同步阶段沿用已保存配置
    config.binaryFormat      = sendCommandExact("@fd BIN", 1) == "1"
    config.showTimestamp     = sendCommandExact("@fd TIME", 1) == "1"
    config.showSequence      = sendCommandExact("@fd HEAD", 1) == "1"
    config.triggerMode       = parseInt(sendCommandExact("@fd TYPE", 1))
    config.triggerEdge       = parseInt(sendCommandExact("@fd TRIG", 1))
    // 注意：不查询 @fd CHECK，openCircuitCheck 字段保持空，需按需单独调用

    // —— 2. 强制归一化为 64 字节纯二进制帧 ——
    // 三条命令必须全部成功，否则 FrameReader 与设备模式不一致 → 解析乱码
    modeSetOK = false
    if sendCommandExact("@fe BIN 1", 1)  == "A" and
       sendCommandExact("@fe TIME 0", 1) == "A" and
       sendCommandExact("@fe HEAD 0", 1) == "A":
        modeSetOK = true

    // —— 3. 仅在三命令全成功时覆盖本地配置 ——
    if modeSetOK:
        config.binaryFormat  = true
        config.showTimestamp = false
        config.showSequence  = false
    // 任一失败：保留第 1 步读回的实际值，由上层决定如何处理

    // —— 4. 根据最终配置设置帧读取器模式 ——
    frameReader.setBinaryMode(config.binaryFormat)
    frameReader.setMetadataMode(config.showTimestamp || config.showSequence)

    onConfigSynced(config)           // 通知外部配置已就绪
```

> **temp 型号固件说明（重要）**：temp 型号固件不支持二进制模式。向其发送 `@fe BIN 1` 时设备**仍回 `A`（ACK）但实际不生效**，`@fd BIN` 仍返回 `0`。当前同步流程**不重新查询 `@fd BIN` 验证生效**，因此 `modeSetOK=true` 会错误地把 `binaryFormat` 置为 `true`，导致 FrameReader 按 64 字节二进制解析设备实际发送的 ASCII 帧 → 帧错位。
>
> **已知限制**：驱动未实现 temp 型号自动检测与 ASCII 回退。使用 temp 型号时，必须由上层在 `ApplyConfig` 中显式设 `binaryFormat=false`，并避免依赖 sync 的强制归一化结果。

### 4.3 启动前准备

正常Stop已消费Stop ACK并清空FrameReader，因此连接处于干净命令边界。首次Start重置FrameReader并登记前导Start ACK；Stop后的Start重新发送`@f0`，不复用旧物理流。

```pseudocode
func startAcquisitionPreamble():
    writeCommandOnly("@f1")                     // 停止任何残留采集流
    readCompleteTailFramesUntilACK("A")         // ACK前只接受完整在途帧
    waitForQuietWindow()                         // 确认旧流结束
    frameReader.reset()                          // 边界确认后清空内部buffer
    // ★ 根据当前配置设置帧读取器模式
    frameReader.setBinaryMode(currentConfig.binaryFormat)
    frameReader.setMetadataMode(currentConfig.showTimestamp || currentConfig.showSequence)
    // ★ 注意：不自动发 @fe TIME/HEAD，保持用户配置不变
```

### 4.4 StartAcquisition

```pseudocode
func startAcquisition():
    // 等待配置同步完成
    wait(configSyncDone)

    // 配置归一化
    applyNormalizedConfig()

    // ★ 启动前准备（物理停止 + 排空旧流 + 重置帧读取器）
    startAcquisitionPreamble()

    // 发送开始采集
    writeCommandOnly("@f0 FFFF 2")

    // 已建立干净事务边界。实机 ACK 通常为前导单字节 A，
    // 但约 7% 的启动中第一个数据帧先到、ACK 随后迟到（见 §3.3.4）。
    // Start 端必须容忍 ACK 迟到：用“跳过单个字节后连续帧全合法”定位并跳过唯一的迟到 A，
    // 禁止裸搜 0x41 或把“首字节非 A”直接判为协议错位。
    expectStartACKAllowDelayed()

    // 启动读取循环
    startAsync(readLoop)
    acquiring = true
```

### 4.5 readLoop

```pseudocode
func readLoop():
    capturedConn = currentConn   // ★ 入口捕获连接引用，防并发替换
    lastDataAt = now()
    consecutiveFrameErrors = 0
    unexpectedErr = null

    // —— defer：仅在异常退出时发 @f1，正常停止不发 ——
    defer:
        if unexpectedErr != null:
            capturedConn.write("@f1")     // 异常退出才确保停止
            acquiring = false
            onReadLoopExit(unexpectedErr)
        signal(readLoopDone)              // 通知外部 readLoop 已退出

    while stop not signaled:
        rawBytes = capturedConn.read(timeout=200ms)

        if rawBytes == null:     // 超时
            if now() - lastDataAt > 10s:   // 无数据超时
                unexpectedErr = "no data timeout"
                return
            continue

        frameReader.feed(rawBytes)

        while frameReader.hasCompleteFrame():
            frame = frameReader.readFrame()
            // ★ 按当前配置分流解析
            if currentConfig.binaryFormat and (currentConfig.showTimestamp or currentConfig.showSequence):
                (hwTs, values) = parseBinaryFrameWithTimestamp(frame)
            else if currentConfig.binaryFormat:
                values = parseBinaryFrame(frame)
            else if currentConfig.showTimestamp || currentConfig.showSequence:
                (_, _, values) = parseMetadataFrame(frame)
            else:
                values = parseASCIIFrame(frame)
            // 反转通道顺序（串口帧除外，但 TCP 模式下始终需要）
            values = reverse(values)
            // 合法性校验；边界上的非法帧直接使连接失效，禁止逐字节猜边界
            if not isValidFrame(values):
                unexpectedErr = "invalid frame at established boundary"
                return
            consecutiveFrameErrors = 0
            lastDataAt = now()
            // 用户逻辑采集开启时才上送；暂停期间继续读流以维护固定边界
            if acquiring:
                onData({ deviceId, timestamp, hardwareTimestamp: hwTs, values, unit: "°C" })

    // 正常停止：defer 中 unexpectedErr==null，不发 @f1
```

> **固定边界原则**：纯二进制帧无帧头。多数通道为 0 时，偏移 4 字节的候选帧仍可能全部落在合法温度范围，软件无法从内容判断错位。因此生产路径禁止逐字节滑动重同步；连接期间保持唯一 reader 连续消费固定帧，边界异常时废弃连接。

### 4.6 StopAcquisition

```pseudocode
func stopAcquisition():
    acquiring = false
    stopping = true
    frameReader.expectControlACKAfterFrames()
    writeCommandOnly("@f1")
    wait(readLoopDone, timeout=1s)
    // readLoop消费完整尾帧和Stop ACK后退出
    frameReader.reset()
    streaming = false
    stopping = false
    status = Connected
```

> Stop owner的3秒总超时独立于socket deadline。若ACK缺失、ACK后排空未完成、Read忽略deadline或边界异常，owner直接`conn.Close()`解除阻塞，连接进入Error并要求重连。不能启动第二个reader执行drain。

### 4.7 Disconnect

```pseudocode
func disconnect():
    acquiring = false
    capturedConn.write("@f1", timeout=500ms)
    conn.close()
    wait(readLoopDone, timeout=3s)
    connected = false
```

---

## 5. 命令系统

### 5.1 命令发送策略

发送模式必须由命令的已验证响应边界决定：

```pseudocode
// writeCommandOnly —— 只写不读，用于 @f0/@f1
// ★ 命令裸发，不追加 \n（设备协议不要求换行结尾）
func writeCommandOnly(cmd: string):
    conn.write(cmd, timeout=5s)
    // 无响应等待

// SendCommand —— 仅用于确实以 LF 结束的响应
func sendCommand(cmd: string) -> string:
    conn.write(cmd)
    return conn.readUntil('\n', timeout=5s).trimEnd('\r\n')

// SendCommandExact —— 精确读 N 字节，用于固定长度响应
// ★ 命令裸发；用 io.ReadFull 精确读 n 字节，读满立即返回
func sendCommandExact(cmd: string, expectBytes: int) -> string:
    conn.write(cmd)
    resp = conn.readExact(expectBytes, timeout=5s)
    return resp

// SendCommandIdle —— 读至 30ms 静默，用于可变长度响应
// ★ 命令裸发；响应无固定长度，靠静默窗口判定结束
func sendCommandIdle(cmd: string) -> string:
    conn.write(cmd)
    return conn.readUntilIdle(idleTime=30ms, timeout=5s)
```

**生产驱动约束**：T1603 命令必须互斥，采集期间禁止查询；不要假设存在 `pendingResponses` FIFO，当前 Go 驱动使用同步命令所有权。

### 5.2 配置查询（@fd）

| 命令 | 功能 | 响应长度 | 匹配策略 |
|------|------|----------|---------|
| `@fd MCH` | 通道掩码 | 4 字符 | SendCommandExact(4)，实机已验证 |
| `@fd SPS` | 采样间隔 (ms) | 可变、无终止符 | 实机已验证；同步阶段不查询 |
| `@fd BIN` | 二进制标志 | 1 字符 | SendCommandExact(1) |
| `@fd TIME` | 时间戳标志 | 1 字符 | SendCommandExact(1) |
| `@fd HEAD` | 序号标志 | 1 字符 | SendCommandExact(1) |
| `@fd AVG` | 平均次数 | 可变、无终止符 | 实机已验证；同步阶段不查询 |
| `@fd TYPE` | 触发方式 | 1 字符 | SendCommandExact(1) |
| `@fd TRIG` | 触发沿 | 1 字符 | SendCommandExact(1) |
| `@fd TNUM` | 触发次数 | 可变、无终止符 | 实机已验证；同步阶段不查询 |
| `@fd CHECK` | 开路检测 | 4 字符（协议/历史资料） | 本轮未实机验证 |

**`@fd CHECK` 响应格式：** 4 位十六进制掩码，每 bit 对应一个通道（bit0=CH0，bit15=CH15），`1` 表示该通道开路。例如 `0003` 表示 CH0 和 CH1 开路，`FFFF` 表示全部通道开路。

> **注意**：`@fd CHECK` **不在配置同步流程中查询**，`openCircuitCheck` 字段同步后保持空。需要开路检测时由上层单独调用 `@fd CHECK`。

### 5.3 配置设置（@fe）

仅 `@fe BIN 1`、`@fe TIME 0`、`@fe HEAD 0` 已实机验证返回单字节 `A`、无终止符。其他 `@fe` 命令当前按同一单字节 ACK 协议实现并有自动化测试，但本轮未做实机写入验证。

| 命令 | 参数 | 说明 |
|------|------|------|
| `@fe BIN <0\|1>` | `0`=ASCII, `1`=二进制 | **驱动默认强制 `1`** |
| `@fe MCH <mask>` | 4 位十六进制掩码 | 如 `FFFF`=全通道, `0001`=仅 CH0 |
| `@fe SPS <n>` | 采样间隔（毫秒） | n = 正整数 |
| `@fe TIME <0\|1>` | 时间戳显示 | 0=不显示, 1=显示 |
| `@fe HEAD <0\|1>` | 序号显示 | 0=不显示, 1=显示 |
| `@fe AVG <n>` | 平均次数 | n = 1~100 |
| `@fe TYPE <0\|2>` | 触发方式 | 0=软件, 2=硬件 |
| `@fe TRIG <0\|1\|2>` | 触发沿 | 0=上升, 1=下降, 2=变化 |
| `@fe TNUM <n>` | 触发次数 | n = 正整数 |

### 5.4 热电偶类型（@e3 / @f3）

| 命令 | 功能 | 响应 | 说明 |
|------|------|------|------|
| `@e3` | 读 | 16 字符 + 单个 LF | 实机共 17 字节 |
| `@f3 0<16ch>0` | 写 | 单字节 ACK（实现/自动化测试） | 本轮未实机验证 |

**类型映射：**

| 类型 | 设备码 | 类型 | 设备码 |
|------|--------|------|--------|
| K | `K` | N | `N` |
| B | `B` | R | `R` |
| E | `E` | C | `C` |
| J | `J` | WRE325 | `1` |
| T | `T` | WRE526 | `2` |
| S | `S` | WRE520 | `3` |

### 5.5 采集控制（@f0 / @f1）

| 命令 | 功能 | 说明 |
|------|------|------|
| `@f0 <mask> 2` | 开始连续采集 | 完成Stop排空后，实机先返回单字节`A`，随后发送64字节记录，见 §3.3 |
| `@f1` | 停止采集 | 实机返回单字节`A`；先消费完整在途记录，事务边界`A`后旧流静默，见 §3.3 |

参数说明：
- `<mask>`：16 位十六进制通道掩码，bit0=CH0，bit15=CH15。如 `FFFF`（全通道）、`0001`（仅 CH0）、`0003`（CH0+CH1）
- `2`：触发方式参数，对应 `@fd TYPE` 的值（`0`=软件触发, `2`=硬件触发）。驱动默认使用 `2`（硬件触发）以获得连续采集

---

## 6. 配置系统

### 6.1 配置结构

```pseudocode
struct DaqT1603HardwareConfig:
    thermocoupleTypes: string[16]   // 如 "KKKKKKKKKKKKKKKK"
    channelMask:       string       // 十六进制 "0000"~"FFFF"（仅作 @f0 启动参数，不下发 @fe MCH）
    samplingRate:      int          // ★ 字段名虽叫 SamplingRate，但存的是 SPS 原值（毫秒），非 Hz
    binaryFormat:      bool         // ★ 驱动默认 true（BIN=1）
    averageCount:      int          // 1~100，同步回退默认 1，profile 默认 4
    triggerMode:       int          // 0=软件, 2=硬件
    triggerEdge:       int          // 0=上升, 1=下降, 2=变化
    triggerCount:      int          // 触发次数
    showTimestamp:     bool         // 时间戳标志
    showSequence:      bool         // 序号标志
    openCircuitCheck:  string       // 开路检测掩码（同步不查询，保持空）
```

### 6.2 Hz ↔ SPS 转换

```pseudocode
// 应用层常用 Hz，设备层用 SPS（毫秒）
func hzToSpsMs(hz: int) -> int:  return 1000 / hz
func spsMsToHz(spsMs: int) -> int:  return 1000 / spsMs
// 10 Hz → SPS=100（即 100ms/帧）
// SPS=10 → 100 Hz（即 10ms/帧）
```

**精度限制（整数除法）：** `1000 / hz` 为整数除法，高频段分辨率低：

| 请求 Hz | 实际 SPS(ms) | 实际 Hz | 偏差 |
|---------|-------------|---------|------|
| 1 | 1000 | 1.0 | 0% |
| 10 | 100 | 10.0 | 0% |
| 100 | 10 | 100.0 | 0% |
| 200 | 5 | 200.0 | 0% |
| 333 | 3 | 333.3 | 0.1% |
| 500 | 2 | 500.0 | 0% |
| 501~1000 | **1** | **1000.0** | **≤50%** |

**上限（>500 Hz 均映射到 SPS=1 即 1000 Hz），且设备硬件实际最大约 800 Hz。**

应用层输入范围为 **1~1000 Hz**，驱动负责 `hzToSpsMs` 转换。低于 500 Hz 时精度可接受，高于 500 Hz 数据仅供参考。

### 6.3 配置下发（ApplyConfig）

```pseudocode
func applyConfig(config: DaqT1603HardwareConfig):
    assert status == Connected   // 采集中不允许下发

    // ★ 热电偶类型走 @f3（若提供，必须 16 字符）
    if config.thermocoupleTypes != "":
        assert len(config.thermocoupleTypes) == 16
        expectSingleByteACK("@f3 0" + config.thermocoupleTypes + "0")

    // ★ 注意：不下发 @fe MCH —— 通道掩码通过 @f0 启动参数控制
    expectSingleByteACK("@fe BIN " + (config.binaryFormat ? "1" : "0"))
    expectSingleByteACK("@fe TIME " + (config.showTimestamp ? "1" : "0"))
    expectSingleByteACK("@fe HEAD " + (config.showSequence ? "1" : "0"))
    if config.samplingRate > 0:
        expectSingleByteACK("@fe SPS " + config.samplingRate)
    if config.averageCount > 0:
        expectSingleByteACK("@fe AVG " + config.averageCount)
    expectSingleByteACK("@fe TYPE " + config.triggerMode)
    expectSingleByteACK("@fe TRIG " + config.triggerEdge)
    if config.triggerCount > 0:
        expectSingleByteACK("@fe TNUM " + config.triggerCount)

    localConfig = config
    frameReader.setBinaryMode(config.binaryFormat)
    frameReader.setMetadataMode(config.showTimestamp || config.showSequence)
```

### 6.4 默认配置

```json
{
  "thermocoupleTypes": "KKKKKKKKKKKKKKKK",
  "channelMask": "FFFF",
  "samplingRate": 10,
  "binaryFormat": true,
  "averageCount": 4,
  "showTimestamp": false,
  "showSequence": false,
  "autoConnect": false
}
```

---

## 7. 发现协议（UDP 广播）

设备发现通过 UDP 广播实现：向端口 **7000** 发送字符串 **`"T1603"`**，设备在同一端口回复设备信息。

```pseudocode
func scanDevices(timeoutMs: int = 3000) -> DiscoveredDevice[]:
    // ★ bind 临时端口 :0，向各广播地址的 7000 端口发送 "T1603"
    socket = new UdpSocket(bindPort=0)
    socket.setDeadline(timeoutMs)
    for addr in ["255.255.255.255"] + getSubnetBroadcastAddrs():
        socket.sendTo(addr, 7000, "T1603")

    results = [], seen = Set()
    while true:
        (data, remote) = socket.receiveFrom(timeout=剩余时间)
        if (data, remote) == timeout: break
        // 用 UDP 包来源 IP 作为设备地址回退
        device = parseDiscoveryResponse(data, remoteHost=remote.ip)
        if device and device.id not in seen:
            seen.add(device.id)
            results.append(device)
    return results
```

### 响应解析（3 种格式）

```pseudocode
func parseDiscoveryResponse(raw: string) -> DiscoveredDevice | null:
    // 格式 1: JSON
    if raw.startsWith("{"):
        obj = jsonParse(raw)
        return {
            ip: obj.ip, mac: obj.mac,
            serialNumber: obj.serialNumber,
            model: obj.model,              // 可能为 "temp" 而非 "T1603"
            firmwareVersion: obj.firmwareVersion,
            port: obj.port ?? 9000
        }

    // 格式 2: CSV（8 字段）
    if raw.contains(","):
        parts = raw.split(",")
        if parts.length >= 8:
            return {
                ip: parts[0].trim(), mac: parts[1].trim(),
                serialNumber: parts[2].trim(),
                model: "T1603",
                firmwareVersion: parts[4].trim(),
                port: parseInt(parts[7].trim()) ?? 9000
            }

    // 格式 3: 短响应（以 "DAQT1603" 开头）
    if raw == "DAQT1603" or raw.startsWith("DAQT1603"):
        return {
            id: "scan-daq-t-1603-" + remoteHost,
            address: remoteHost, port: 9000,
            model: "T1603"
        }

    return null
```

---

## 8. 错误处理

### 8.1 连接错误检测

```pseudocode
func isConnectionFault(err: Error) -> bool:
    keywords = [
        "i/o timeout", "broken pipe",
        "connection reset by peer", "device disconnected", "EOF",
    ]
    return any(keywords where err.message.contains(keyword))

func isClosedConnError(err: Error) -> bool:
    return err.message.contains("use of closed network connection")
```

### 8.2 超时策略一览

| 场景 | 超时 | 说明 |
|------|------|------|
| TCP Connect | 5s | 建立连接 |
| 命令响应 | 按 operation owner | 固定边界读；timeout/短读后连接毒化，不复用 |
| @fd 静默窗口 | 不用于生产同步 | SPS/AVG/TNUM 沿用保存值 |
| readLoop 读超时 | 200ms | 每次 socket 读取 |
| 无数据超时 | 10s | 超过即退出 readLoop |
| @f0 ACK | operation owner读取 | 完成Stop排空后精确读取前导单字节`A` |
| 缓冲区排空 | Stop事务内执行 | 唯一reader消费完整尾帧和Stop ACK，再丢弃ACK后迟到旧流字节至静默确认完成 |
| StartAcquisition 等旧 readLoop 退出 | 3s | 超时 Close、Error、要求重连，禁止继续 |
| @f1 停止命令写入 | operation watchdog | Write 失败或 timeout 后连接毒化 |
| 配置同步起始延迟 | 300ms | connect 后等设备就绪 |
| 启动前 @f1 后等待 | 50ms | 让设备处理停止命令 |

### 8.3 断线重连策略

驱动**不自动重连**。readLoop 检测到连接故障后退出，上层通过 `onReadLoopExit` 回调获知断线事件，由用户决定是否重连。重连流程：

```pseudocode
// 上层处理断线
onReadLoopExit():
    acquiring = false
    connected = false
    notifyConnectionLost(deviceId, lastError)

// 用户主动重连
await driver.disconnect()       // 清理旧连接状态
await driver.connect()          // 重新建立 TCP + 配置同步
await driver.startAcquisition() // 恢复采集
```

> **注意**：断线后必须先 `disconnect()` 清理旧状态，再 `connect()` 建立新连接，不可直接 `connect()` 覆盖。

### 8.4 采集启停风险点

**残留数据处理：** Stop由唯一reader消费完整尾帧和Stop ACK。收到ACK后先清空FrameReader，再继续丢弃socket中迟到的旧流字节；达到连续1秒静默并额外确认500ms零字节后，才允许配置或再次Start。若排空Read在Windows忽略deadline，独立Stop owner在3秒总预算后直接Close连接并要求重连。

**快速点击防护：** 在Stop完成前，共享驱动层拒绝并发Start（`stopping=true`时返回`stop in progress`）。适配器层记录设备级转换事务标记`Starting/Stopping`，防止相反操作重叠进入驱动。前端在`Starting/Stopping`持续禁用采集按钮，不依赖防抖或固定冷却时间。

**死锁预防：**
```pseudocode
// 1. readLoop 入口捕获连接引用
func readLoop():
    capturedConn = currentConn   // ★ 不直接用 d.conn

// 2. 硬件写入通过互斥锁序列化
writeLock.lock()
conn.write(data)
writeLock.unlock()

// 3. 避免持有锁再调硬件操作（可能循环等待）
```

**readLoop 退出安全：**
```pseudocode
defer:
    // ★ 仅异常退出（unexpectedErr != nil）才发 @f1，正常停止不发
    if unexpectedErr != null:
        capturedConn.write("@f1")    // 用捕获的连接
        acquiring = false
        onReadLoopExit(unexpectedErr)
    close(readLoopDone)

// 用户逻辑停止不关闭 stopSignal，readLoop 继续维护固定边界但不上送数据。
// Disconnect 才关闭 stopSignal、发送 @f1、Close conn 并等待 readLoopDone。
```

### 8.5 绝对禁止的操作

| 禁止 | 后果 |
|------|------|
| 发送 @f1 后未等Stop ACK就退出reader或Reset | 旧帧残留污染后续配置或采集，可能产生不可检测的通道错位 |
| readLoop 中用全局连接引用而非捕获的局部变量 | Connect 替换连接后操作错误连接 |
| 只sleep或只reset应用层buffer，不持续读取到Stop ACK | 设备或TCP队列中的旧数据会污染下一事务 |
| 采集期间发送查询命令 | 数据流与响应混杂 |
| 在未完成Stop排空时直接读取Start ACK | 可能把旧流数据误认为本次Start响应 |

---

## 9. 注意事项速查

| # | 注意点 |
|---|--------|
| 1 | **BIN 推荐值：1**。配置同步后强制 `@fe BIN 1` / `@fe TIME 0` / `@fe HEAD 0`，以 64 字节 float32 LE 接收，效率最高 |
| 2 | 完成Stop排空后，`@f0`实机先返回单字节`A`，随后发送64字节记录；不得在任意数据流中搜索裸`0x41` |
| 3 | 设备发送 CH15→CH0，解析后必须 `.reverse()` |
| 4 | ASCII 定长帧 192 字节**无换行终止**，按长度截取；ASCII 变长帧（BIN=0 + TIME/HEAD=1）同样无换行，按字段计数截取 |
| 5 | `@f0`/`@f1` 走 writeCommandOnly，不进入 pending 队列 |
| 6 | 采集期间**禁止**查询命令，需先停止 |
| 7 | Start前必须位于干净命令边界；正常Stop收到Stop ACK后才能reset FrameReader |
| 8 | StopAcquisition发送`@f1`并等待唯一reader消费Stop ACK；返回后可配置或重新Start |
| 9 | readLoop 内捕获连接引用，不直接使用全局连接变量 |
| 10 | SPS 单位是**毫秒**，应用层可能用 Hz：`Hz = 1000 / SPS`；配置字段 `samplingRate` 存的是 SPS 原值（ms），非 Hz |
| 11 | **temp 型号固件不支持 BIN=1**，`@fd BIN` 始终返回 `0`，但发 `@fe BIN 1` 仍回 ACK。驱动**未实现自动回退**，需上层显式设 `binaryFormat=false` |
| 12 | 串口模式下不支持 `@e3`/`@fd`/`@fe`，配置需在 TCP 下完成 |
| 13 | **四种 TCP 帧格式由 (binaryMode, metadataMode) 两开关决定**：64B / 72B / 192B / 变长 ASCII |
| 14 | BIN=1 + TIME/HEAD=1 → **72 字节定长二进制带时间戳帧**（8B 头 + 64B 数据），走定长读取，不是变长 |
| 15 | BIN=0 + TIME/HEAD=1 → 空格分隔不定长 ASCII 流（无换行），FrameReader 走 `readFrameVariable` 字段计数法 |
| 16 | 变长帧读取：累积 buffer → 尝试提取 18/17 字段 → 解析校验 → 成功则提取，失败则继续读 |
| 17 | 变长帧中检测 HEAD vs TIME：首 token 可解析为整数则视为 seq（HEAD），否则视为 ts（TIME） |
| 18 | 配置下发（ApplyConfig）后必须同步调用 `frameReader.setMetadataMode(config.showTimestamp \|\| config.showSequence)` |
| 19 | ApplyConfig **不下发 `@fe MCH`**，通道掩码通过 `@f0 <mask>` 启动参数控制 |
| 20 | `@fd CHECK` **不在配置同步中查询**，`openCircuitCheck` 同步后为空，需按需单独调用 |
| 21 | 固定边界解析失败时废弃连接；禁止逐字节`resync()`猜测无帧头二进制边界 |
| 22 | `HardwareTimestamp`：72B 二进制帧为 `sec+ns`（纳秒精度）；ASCII 变长帧为 Unix 秒浮点（微秒精度）；无时间戳时为 0，应用本机接收时间 |
| 23 | readLoop 在逻辑停止期间继续消费帧但不上送；仅 Disconnect/异常退出才发 `@f1` 并关闭连接 |
| 24 | 应用层采样率输入范围为 **1~1000 Hz**，驱动通过 `hzToSpsMs` 转换为设备 SPS 参数 |
