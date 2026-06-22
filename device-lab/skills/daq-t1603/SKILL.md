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
     │                            │                                 │
     └──disconnect()──────────────┴──stopAcquisition()──────────────┘
     │                                                              │
     │            disconnect() 内部会先 stopAcquisition()            │
     │                                                              │
     └────────────────── readLoop 异常退出（连接故障/无数据超时）────────┘
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

    // 消费 @f0 的可选 ACK 前导
    func consumeOptionalACK(timeoutMs: int):
        ...（同前）

    func reset():
        buffer.clear()
        // ★ metadataMode 不清零，保留当前模式配置
```

---

## 3. 在线字节参考

### 3.1 命令在线字节

**命令以裸 ASCII 字符串发送，不追加任何换行或终止符。** 驱动直接 `conn.Write([]byte(cmd))`，cmd 即命令本身。换行只出现在**设备响应**中（响应以 `\n` 或 `\r\n` 结尾，由 `SendCommand`/`SendCommandIdle` 的 read-until-`\n` 逻辑消费）。

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

常见设备响应（**响应**带换行，命令不带）：

| 响应 | ASCII | 十六进制 |
|------|-------|---------|
| ACK（带换行） | `A\n` | `41 0A` |
| ACK（单字节） | `A` | `41` |
| 错误 | `E` | `45` |

**错误响应 `E` 的触发条件：** 命令格式错误、参数越界、不支持的操作（如向 temp 型号发送不支持的命令）。驱动收到 `E` 后应终止当前操作并上报错误，不应重试同一命令。

> **响应读取策略差异**：`SendCommand` / `SendCommandIdle` 读到 `\n` 即结束（响应带换行）；`SendCommandExact(n)` 用 `io.ReadFull` 精确读 n 字节，再消费尾部 `\r\n`（固定长度响应如 `@fd BIN` 的单字符 `0`/`1`）。

### 3.2 数据帧十六进制展开

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
    config.thermocoupleTypes = sendCommandExact("@e3", 16)
    config.channelMask       = sendCommandIdle("@fd MCH")
    config.samplingRate      = parseInt(sendCommandIdle("@fd SPS"))
    config.binaryFormat      = sendCommandExact("@fd BIN", 1) == "1"
    config.showTimestamp     = sendCommandExact("@fd TIME", 1) == "1"
    config.showSequence      = sendCommandExact("@fd HEAD", 1) == "1"
    config.averageCount      = parseInt(sendCommandIdle("@fd AVG"))
    config.triggerMode       = parseInt(sendCommandExact("@fd TYPE", 1))
    config.triggerEdge       = parseInt(sendCommandExact("@fd TRIG", 1))
    config.triggerCount      = parseInt(sendCommandIdle("@fd TNUM"))
    // 注意：不查询 @fd CHECK，openCircuitCheck 字段保持空，需按需单独调用

    // —— 2. 强制归一化为 64 字节纯二进制帧 ——
    // 三条命令必须全部成功，否则 FrameReader 与设备模式不一致 → 解析乱码
    modeSetOK = false
    if sendCommand("@fe BIN 1")  succeeds and
       sendCommand("@fe TIME 0") succeeds and
       sendCommand("@fe HEAD 0") succeeds:
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

### 4.3 启动前准备（StartAcquisition preamble）

采集启动前清理残留数据并配置帧读取器。**此步骤是 `startAcquisition` 的内部子流程**，在 `@f0` 发送之前执行，不应独立调用。

```pseudocode
func startAcquisitionPreamble():
    writeCommandOnly("@f1")                     // 停止任何残留采集流
    sleep(50ms)                                 // 等设备处理停止命令
    drainConnection(conn, waitTime=100ms)       // 排空 TCP 缓冲区
    frameReader.reset()                         // 清空内部 buffer
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

    // ★ 启动前准备（停止残留采集 + 排空缓冲区 + 重置帧读取器）
    startAcquisitionPreamble()

    // 发送开始采集
    writeCommandOnly("@f0 FFFF 2")

    // 消费可选的 ACK 前导
    // BIN=1 时部分固件发 'A' / 'A\n'，部分不发，超时 200ms 即过
    // ★ ConsumeOptionalACK 返回 error 时直接终止 StartAcquisition（非 ACK 字节
    //   被回填到 buffer，不视为错误；仅 I/O 错误才返回 error）
    if consumeOptionalACK(timeout=200ms) returns error:
        return error

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
            // 合法性校验
            if not isValidFrame(values):
                consecutiveFrameErrors++
                // ★ 连续 5 次失败 → 自动重同步
                if consecutiveFrameErrors >= 5:
                    frameReader.resync()    // 丢弃缓冲区首字节重新对齐
                    consecutiveFrameErrors = 0
                continue
            consecutiveFrameErrors = 0
            lastDataAt = now()
            // 发出数据
            onData({ deviceId, timestamp, hardwareTimestamp: hwTs, values, unit: "°C" })

    // 正常停止：defer 中 unexpectedErr==null，不发 @f1
```

> **Resync 设计要点**：`resync()` 仅丢弃内部 buffer 首字节，**不持锁阻塞在 conn.Read 上**，避免与 `setBinaryMode` / `setMetadataMode` / `reset` 等持锁操作互相阻塞。buffer 为空时不做任何 I/O，等下次读到新数据再由调用方决定是否再次 resync。

### 4.6 StopAcquisition

```pseudocode
func stopAcquisition():
    if acquiring:
        close(stopSignal)          // 通知 readLoop 正常退出
    acquiring = false
    // ★ 异步发送 @f1（goroutine），不阻塞调用方
    //   连接已被 Disconnect 关闭时写失败属预期，降级为 debug 日志
    if wasAcquiring:
        go func():
            writeLock.lock()
            capturedConn.write("@f1", timeout=500ms)
            writeLock.unlock()
    frameReader.reset()            // 清空缓冲区
```

> **注意**：StopAcquisition **不等待 readLoop 退出**。等待 readLoop 退出的逻辑在 **StartAcquisition** 入口：若上一次的 `readLoopDone` 仍在，先等其结束（最多 3s 超时），再启动新循环。这避免「停止→立即启动」时旧 readLoop 与新 readLoop 并发读同一连接。

### 4.7 Disconnect

```pseudocode
func disconnect():
    if acquiring:
        stopAcquisition()
    close(configSyncDone)
    conn.close()
    connected = false
```

---

## 5. 命令系统

### 5.1 命令发送策略

四种发送模式，适应不同的响应特征：

```pseudocode
// writeCommandOnly —— 只写不读，用于 @f0/@f1
// ★ 命令裸发，不追加 \n（设备协议不要求换行结尾）
func writeCommandOnly(cmd: string):
    conn.write(cmd, timeout=5s)
    // 无响应等待

// SendCommand —— 读至换行，用于 @fe 等带 ACK 的命令
// ★ 命令裸发；响应以 \n 结尾，读到 \n 即止
func sendCommand(cmd: string) -> string:
    conn.write(cmd)
    return conn.readUntil('\n', timeout=5s).trimEnd('\r\n')

// SendCommandExact —— 精确读 N 字节，用于固定长度响应
// ★ 命令裸发；用 io.ReadFull 精确读 n 字节，再消费尾部 \r\n
func sendCommandExact(cmd: string, expectBytes: int) -> string:
    conn.write(cmd)
    resp = conn.readExact(expectBytes, timeout=5s)
    conn.readAvailable()  // 消费尾部 \r\n
    return resp

// SendCommandIdle —— 读至 30ms 静默，用于可变长度响应
// ★ 命令裸发；响应无固定长度，靠静默窗口判定结束
func sendCommandIdle(cmd: string) -> string:
    conn.write(cmd)
    return conn.readUntilIdle(idleTime=30ms, timeout=5s)
```

**响应匹配**：驱动内维护 `pendingResponses` FIFO 队列，命令互斥不会交叉。

### 5.2 配置查询（@fd）

| 命令 | 功能 | 响应长度 | 匹配策略 |
|------|------|----------|---------|
| `@fd MCH` | 通道掩码 | 4 字符 | SendCommandIdle |
| `@fd SPS` | 采样间隔 (ms) | 可变 | SendCommandIdle |
| `@fd BIN` | 二进制标志 | 1 字符 | SendCommandExact(1) |
| `@fd TIME` | 时间戳标志 | 1 字符 | SendCommandExact(1) |
| `@fd HEAD` | 序号标志 | 1 字符 | SendCommandExact(1) |
| `@fd AVG` | 平均次数 | 可变 | SendCommandIdle |
| `@fd TYPE` | 触发方式 | 1 字符 | SendCommandExact(1) |
| `@fd TRIG` | 触发沿 | 1 字符 | SendCommandExact(1) |
| `@fd TNUM` | 触发次数 | 可变 | SendCommandIdle |
| `@fd CHECK` | 开路检测 | 4 字符 | SendCommandExact(4) |

**`@fd CHECK` 响应格式：** 4 位十六进制掩码，每 bit 对应一个通道（bit0=CH0，bit15=CH15），`1` 表示该通道开路。例如 `0003` 表示 CH0 和 CH1 开路，`FFFF` 表示全部通道开路。

> **注意**：`@fd CHECK` **不在配置同步流程中查询**，`openCircuitCheck` 字段同步后保持空。需要开路检测时由上层单独调用 `@fd CHECK`。

### 5.3 配置设置（@fe）

所有 `@fe` 响应为 `A\n`（ACK）。

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
| `@e3` | 读 | 16 字符 | 如 `KKKKKKKKKKKKKKKK` |
| `@f3 0<16ch>0` | 写 | `A\n` | 格式：`0` + 16 类型字符 + `0` |

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
| `@f0 <mask> 2` | 开始连续采集 | writeCommandOnly，可能带 ACK 前导 |
| `@f1` | 停止采集 | writeCommandOnly |

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
        sendCommand("@f3 0" + config.thermocoupleTypes + "0")

    // ★ 注意：不下发 @fe MCH —— 通道掩码通过 @f0 启动参数控制
    sendCommand("@fe BIN " + (config.binaryFormat ? "1" : "0"))
    sendCommand("@fe TIME " + (config.showTimestamp ? "1" : "0"))
    sendCommand("@fe HEAD " + (config.showSequence ? "1" : "0"))
    if config.samplingRate > 0:
        sendCommand("@fe SPS " + config.samplingRate)
    if config.averageCount > 0:
        sendCommand("@fe AVG " + config.averageCount)
    sendCommand("@fe TYPE " + config.triggerMode)
    sendCommand("@fe TRIG " + config.triggerEdge)
    if config.triggerCount > 0:
        sendCommand("@fe TNUM " + config.triggerCount)

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
| 命令响应 | 5s | 等待设备回复 |
| @fd 静默窗口 | 30ms | 可变长度响应结束判定 |
| readLoop 读超时 | 200ms | 每次 socket 读取 |
| 无数据超时 | 10s | 超过即退出 readLoop |
| ACK 消费 | 200ms | 等待 @f0 前导 |
| 缓冲区排空 | 100ms/次 | drainConnection，最多 200 次迭代（~20s 安全上限） |
| StartAcquisition 等旧 readLoop 退出 | 3s | 超时则告警并继续 |
| @f1 停止命令写入 | 500ms | StopAcquisition 异步发送 |
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

**残留数据污染：** `@f1` 停止后 TCP 缓冲区约残留 3000+ 字节，不清空直接 `@f0` 导致帧错位。

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

// 正常停止：close(stopSignal)，readLoop 检测到后 return，defer 不发 @f1
// 等待 readLoop 退出：在 StartAcquisition 入口 wait(readLoopDone)，最多 3s 超时
```

### 8.5 绝对禁止的操作

| 禁止 | 后果 |
|------|------|
| defer 中无条件发 @f1 | 正常停止时 StopAcquisition 已异步发过，重复发送导致异常 |
| readLoop 中用全局连接引用而非捕获的局部变量 | Connect 替换连接后操作错误连接 |
| 跳过 drainConnection | 残留数据导致帧错位 |
| 采集期间发送查询命令 | 数据流与响应混杂 |
| 未消费前导 ACK | 首字节错位，整帧解析失败 |

---

## 9. 注意事项速查

| # | 注意点 |
|---|--------|
| 1 | **BIN 推荐值：1**。配置同步后强制 `@fe BIN 1` / `@fe TIME 0` / `@fe HEAD 0`，以 64 字节 float32 LE 接收，效率最高 |
| 2 | 二进制帧前**可能无 ACK**。部分固件不发 `A`/`A\n`，consumeOptionalACK 超时 200ms 后正常读取即可；返回 I/O error 时中断启动 |
| 3 | 设备发送 CH15→CH0，解析后必须 `.reverse()` |
| 4 | ASCII 定长帧 192 字节**无换行终止**，按长度截取；ASCII 变长帧（BIN=0 + TIME/HEAD=1）同样无换行，按字段计数截取 |
| 5 | `@f0`/`@f1` 走 writeCommandOnly，不进入 pending 队列 |
| 6 | 采集期间**禁止**查询命令，需先停止 |
| 7 | 启动采集前必须 drainConnection + reset FrameReader（含 50ms 等待） |
| 8 | 停止→启动是高危操作：StopAcquisition 异步发 `@f1` 不等待；StartAcquisition 入口等待上一次 readLoopDone（最多 3s） |
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
| 21 | 连续 5 次帧解析失败 → 自动 `resync()`（丢缓冲区首字节重新对齐），计数清零；resync 不持锁阻塞 I/O |
| 22 | `HardwareTimestamp`：72B 二进制帧为 `sec+ns`（纳秒精度）；ASCII 变长帧为 Unix 秒浮点（微秒精度）；无时间戳时为 0，应用本机接收时间 |
| 23 | readLoop defer **仅异常退出**发 `@f1`，正常停止不发（StopAcquisition 已异步发过） |
| 24 | 应用层采样率输入范围为 **1~1000 Hz**，驱动通过 `hzToSpsMs` 转换为设备 SPS 参数 |
