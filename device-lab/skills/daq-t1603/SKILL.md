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

| 格式 | BIN | 帧大小 | 编码 | 解析方式 | 推荐 |
|------|-----|--------|------|---------|------|
| 二进制 | `1` | **64 字节** | float32 LE | `readFloatLE` × 16 + `.reverse()` | **★ 推荐** |
| ASCII | `0` | **192 字节** | 12 字符/字段定宽 | `trim().split(/\s+/).map(Number).reverse()` | 调试用 |
| 串口 | — | 46 字节 | int16 BE × 0.1℃/LSB | `readInt16BE` × 16 | 串口专用 |

### 1.3 状态机

```
Disconnected ──connect()──→ Connected ──startAcquisition()──→ Acquiring
     ↑                            ↑                                 │
     │                            │                                 │
     └──disconnect()──────────────┴──stopAcquisition()──────────────┘
                                       │
                                       └──→ Error（异常断连）
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
    frameSize: int           // 64（BIN=1）或 192（BIN=0）
    metadataMode: bool       // true=变长字段计数法, false=定长读取

    func setBinaryMode(isBinary: bool):
        this.frameSize = isBinary ? 64 : 192

    func setMetadataMode(enabled: bool):
        // 当 TIME=1 或 HEAD=1 时启用变长模式
        this.metadataMode = enabled

    func readFrame() -> byte[]:
        if metadataMode:
            return this.readFrameVariable()
        else:
            return this.readFrameFixed()

    // —— 定长模式（BIN=0 标准 ASCII 或 BIN=1 二进制）——
    func readFrameFixed() -> byte[]:
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

所有 ASCII 命令以 `\n`（`0x0A`）结尾，发送到 TCP 线路上如下：

| 命令 | ASCII | 十六进制 |
|------|-------|---------|
| `@e3` | `@e3\n` | `40 65 33 0A` |
| `@f0 FFFF 2` | `@f0 FFFF 2\n` | `40 66 30 20 46 46 46 46 20 32 0A` |
| `@f1` | `@f1\n` | `40 66 31 0A` |
| `@f3 0KKKKKKKKKKKKKKKK0` | `@f3 0KKKKKKKKKKKKKKKK0\n` | `40 66 33 20 30 4B×16 30 0A`（21 字节） |
| `@fd BIN` | `@fd BIN\n` | `40 66 64 20 42 49 4E 0A` |
| `@fd MCH` | `@fd MCH\n` | `40 66 64 20 4D 43 48 0A` |
| `@fd SPS` | `@fd SPS\n` | `40 66 64 20 53 50 53 0A` |
| `@fe BIN 1` | `@fe BIN 1\n` | `40 66 65 20 42 49 4E 20 31 0A` |
| `@fe BIN 0` | `@fe BIN 0\n` | `40 66 65 20 42 49 4E 20 30 0A` |
| `@fe SPS 100` | `@fe SPS 100\n` | `40 66 65 20 53 50 53 20 31 30 30 0A` |
| `@fe TIME 0` | `@fe TIME 0\n` | `40 66 65 20 54 49 4D 45 20 30 0A` |

常见设备响应：

| 响应 | ASCII | 十六进制 |
|------|-------|---------|
| ACK（带换行） | `A\n` | `41 0A` |
| ACK（单字节） | `A` | `41` |
| 错误 | `E` | `45` |

**错误响应 `E` 的触发条件：** 命令格式错误、参数越界、不支持的操作（如向 temp 型号发送不支持的命令）。驱动收到 `E` 后应终止当前操作并上报错误，不应重试同一命令。

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

连接后 300ms 自动执行，逐条查询设备当前配置。**读取后不做修改**，保持设备已有参数不变。

```pseudocode
func syncHardwareConfig():
    // —— 查询设备当前参数 ——
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

    // —— 根据读回配置设置帧读取器模式 ——
    frameReader.setBinaryMode(config.binaryFormat)
    // ★ 关键：TIME=1 或 HEAD=1 时启用 metadataMode
    frameReader.setMetadataMode(config.showTimestamp || config.showSequence)

    onConfigSynced(config)           // 通知外部配置已就绪
```

> **BIN=1 固件兼容说明**：某些固件（如 temp 型号）不支持二进制模式，`@fd BIN` 始终返回 `0`，自动回退为 ASCII 帧读取。向 temp 型号发送 `@fe BIN 1` 时，设备返回 `A`（ACK）但实际不生效，`@fd BIN` 仍返回 `0`。驱动应通过配置同步检测此情况，将 `binaryFormat` 强制设为 `false`。

### 4.3 启动前准备（StartAcquisition preamble）

采集启动前清理残留数据并配置帧读取器。**此步骤是 `startAcquisition` 的内部子流程**，在 `@f0` 发送之前执行，不应独立调用。

```pseudocode
func startAcquisitionPreamble():
    writeCommandOnly("@f1")                     // 停止任何残留采集流
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
    consumeOptionalACK(timeout=200ms)

    // 启动读取循环
    startAsync(readLoop)
    acquiring = true
```

### 4.5 readLoop

```pseudocode
func readLoop():
    capturedConn = currentConn   // ★ 入口捕获连接引用，防并发替换

    while acquiring:
        rawBytes = capturedConn.read(timeout=200ms)

        if rawBytes == null:     // 超时
            if noDataTimeout > 10s:
                break
            continue

        frameReader.feed(rawBytes)

        while frameReader.hasCompleteFrame():
            frame = frameReader.readFrame()
            // ★ 按当前配置分流解析
            if currentConfig.binaryFormat:
                values = parseBinaryFrame(frame)
            else if currentConfig.showTimestamp || currentConfig.showSequence:
                (_, _, values) = parseMetadataFrame(frame)
            else:
                values = parseASCIIFrame(frame)
            // 反转通道顺序（串口帧除外，但 TCP 模式下始终需要）
            values = reverse(values)
            // 合法性校验
            if not isValidFrame(values):
                continue
            // 发出数据
            onData({ deviceId, timestamp, values, unit: "°C" })

    // —— defer 异常退出清理 ——
    capturedConn.write("@f1")      // 确保停止
    acquiring = false
    onReadLoopExit()
    signal(readLoopDone)           // 通知外部 readLoop 已退出
```

### 4.6 StopAcquisition

```pseudocode
func stopAcquisition():
    signal(stop)                   // 通知 readLoop 正常退出
    wait(readLoopDone)             // 等待 readLoop 完全结束
    writeCommandOnly("@f1")        // 发送停止命令
    frameReader.reset()            // 清空缓冲区
```

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
func writeCommandOnly(cmd: string):
    conn.write(cmd + "\n", timeout=5s)
    // 无响应等待

// SendCommand —— 读至换行，用于 @fe 等带 ACK 的命令
func sendCommand(cmd: string) -> string:
    conn.write(cmd + "\n")
    return conn.readUntil('\n', timeout=5s).trimEnd('\r\n')

// SendCommandExact —— 精确读 N 字节，用于固定长度响应
func sendCommandExact(cmd: string, expectBytes: int) -> string:
    conn.write(cmd + "\n")
    resp = conn.readExact(expectBytes, timeout=5s)
    conn.readAvailable()  // 消费尾部 \r\n
    return resp

// SendCommandIdle —— 读至 30ms 静默，用于可变长度响应
func sendCommandIdle(cmd: string) -> string:
    conn.write(cmd + "\n")
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
    channelMask:       string       // 十六进制 "0000"~"FFFF"
    samplingRate:      int          // SPS 值（毫秒），非 Hz
    binaryFormat:      bool         // ★ 驱动默认 true（BIN=1）
    averageCount:      int          // 1~100
    triggerMode:       int          // 0=软件, 2=硬件
    triggerEdge:       int          // 0=上升, 1=下降, 2=变化
    triggerCount:      int          // 触发次数
    showTimestamp:     bool         // 时间戳标志
    showSequence:      bool         // 序号标志
    openCircuitCheck:  string       // 开路检测掩码
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

    sendCommand("@f3 0" + config.thermocoupleTypes + "0")
    sendCommand("@fe BIN " + (config.binaryFormat ? "1" : "0"))
    sendCommand("@fe TIME " + (config.showTimestamp ? "1" : "0"))
    sendCommand("@fe HEAD " + (config.showSequence ? "1" : "0"))
    sendCommand("@fe SPS " + config.samplingRate)
    sendCommand("@fe AVG " + config.averageCount)
    sendCommand("@fe TYPE " + config.triggerMode)
    sendCommand("@fe TRIG " + config.triggerEdge)
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
    socket = new UdpSocket(bindPort=7000)
    for addr in ["255.255.255.255"] + getSubnetBroadcastAddrs():
        socket.sendTo(addr, 7000, "T1603")

    results = [], seen = Set()
    while elapsed < timeoutMs:
        response = socket.receiveFrom(timeout=剩余时间)
        if response == null: break
        device = parseDiscoveryResponse(response.data)
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

    // 格式 3: 短响应
    if raw.contains("DAQT1603") or raw.contains("T1603"):
        return { model: "T1603" }  // 需从 UDP 包来源 IP 补充地址

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
| 缓冲区排空 | 100ms | drainConnection |

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
    capturedConn.write("@f1")    // 用捕获的连接
    acquiring = false
    onReadLoopExit()
    close(readLoopDone)

// 正常停止
close(stopSignal)
wait(readLoopDone)
```

### 8.5 绝对禁止的操作

| 禁止 | 后果 |
|------|------|
| StopAcquisition 后 Disconnect + 重连 | defer 在新连接上发 @f1，设备混乱 |
| defer 中无条件发 @f1 | 正常停止时已发，重复发送导致异常 |
| readLoop 中用全局连接引用而非捕获的局部变量 | Connect 替换连接后操作错误连接 |
| 跳过 drainConnection | 残留数据导致帧错位 |
| 采集期间发送查询命令 | 数据流与响应混杂 |
| 未消费前导 ACK（BIN=0 时） | 192 字节帧错位 1 字节 |

---

## 9. 模拟模式

### 环境变量激活

```
# Windows
set DAQ_T1603_MODE=simulated

# Linux/Mac
DAQ_T1603_MODE=simulated
```

### 模拟行为

```pseudocode
// 设备发现：固定返回 2 个设备
func scanDevices():
    return [
        { ip: "192.168.1.7", port: 9000, serialNumber: "SIM-001" },
        { ip: "192.168.1.8", port: 9000, serialNumber: "SIM-002" }
    ]

// 温度数据：周期性正弦波
func generateSimulatedData(ch: int, elapsedMs: int) -> float64:
    base = 25.0 + ch * 0.5
    wave1 = 5.0 * sin(elapsedMs/1000 + ch * 0.3)
    wave2 = 0.5 * sin(elapsedMs/1000 * 3 + ch * 0.7)
    return base + wave1 + wave2
```

---

## 10. 注意事项速查

| # | 注意点 |
|---|--------|
| 1 | **BIN 推荐值：1**。驱动连接后强制 `@fe BIN 1`，以 64 字节 float32 LE 接收，效率最高 |
| 2 | 二进制帧前**可能无 ACK**。部分固件不发 `A`/`A\n`，consumeOptionalACK 超时 200ms 后正常读取即可 |
| 3 | 设备发送 CH15→CH0，解析后必须 `.reverse()` |
| 4 | ASCII 定长帧 192 字节**无换行终止**，按长度截取；变长帧（TIME=1 或 HEAD=1）同样无换行，按字段计数截取 |
| 5 | `@f0`/`@f1` 走 writeCommandOnly，不进入 pending 队列 |
| 6 | 采集期间**禁止**查询命令，需先停止 |
| 7 | 启动采集前必须 drainConnection + reset FrameReader |
| 8 | 停止→启动是高危操作，必须等 readLoopDone |
| 9 | readLoop 内捕获连接引用，不直接使用全局连接变量 |
| 10 | SPS 单位是**毫秒**，应用层可能用 Hz：`Hz = 1000 / SPS` |
| 11 | temp 型号固件不支持 BIN=1，`@fd BIN` 始终返回 `0`，需回退 ASCII 解析 |
| 12 | 串口模式下不支持 `@e3`/`@fd`/`@fe`，配置需在 TCP 下完成 |
| 13 | 模拟模式：`DAQ_T1603_MODE=simulated` |
| 14 | **TIME=1 或 HEAD=1 时帧格式从 192 字节定长变为空格分隔不定长连续流（无换行）**，FrameReader 必须启用 `metadataMode=true` |
| 15 | 变长帧的读取方式为**字段计数法**：累积 buffer → 尝试提取 18/17 字段 → 解析校验 → 成功则提取，失败则继续读 |
| 16 | 变长帧中检测 HEAD vs TIME 的逻辑：首 token 可解析为整数则视为 seq（HEAD），否则视为 ts（TIME） |
| 17 | 配置下发（ApplyConfig）后必须同步调用 `frameReader.setMetadataMode(config.showTimestamp || config.showSequence)` |
| 18 | 解析帧时应提取 `HardwareTimestamp`（变长帧中的 Unix 秒浮点值），传递给上层用于数据记录 |
| 19 | `HardwareTimestamp` 为 Unix 秒（float64，微秒精度），无时间戳时为 0，此时应使用本机接收时间 |
| 20 | 应用层采样率输入范围为 **1~1000 Hz**，驱动通过 `hzToSpsMs` 转换为设备 SPS 参数 |
