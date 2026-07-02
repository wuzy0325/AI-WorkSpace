---
name: b140
description: >
  B140 (Galil DMC-B140-M) 运动控制器完整命令参考，从生产代码提取。
  涵盖实际发送到硬件的每条命令。用于编写、调试、修改 B140 驱动代码。
---

# B140 运动控制器 — 命令参考

## 1 概述

B140 是基于 Galil DMC-B140-M 的 4 轴步进/伺服运动控制器，通过 TCP 通信。
所有命令为 ASCII 字符串，以 `\r`（`0x0D`）结尾。响应以 `:` 表示成功，`?` 表示错误。
命令严格串行执行，不可并发。

| 参数 | 值 |
|------|-----|
| 协议 | TCP/IP |
| 默认端口 | 5000 |
| 命令超时 | 5000 ms |
| 轴映射（逻辑→物理） | X→A, Y→B, Z→C, U→D |

## 2 连接流程

连接后，驱动按顺序发送：

```
SH                  ← 使能所有轴伺服
MTA=<val>           ← X轴电机方向
MTB=<val>           ← Y轴电机方向
MTC=<val>           ← Z轴电机方向
MTD=<val>           ← U轴电机方向
CEA=<val>           ← X轴编码器方向
CEB=<val>           ← Y轴编码器方向
CEC=<val>           ← Z轴编码器方向
CED=<val>           ← U轴编码器方向
```

方向命令通过签名缓存（如 `X:0|Y:1|Z:0|U:0`），仅在签名变化时重发。
触发时机：连接、`updateProfile()`、断开连接（重置缓存）、以及每次
`moveTo` / `moveBy` / `jog` / `home` / `getAxisStatus` / `getAllAxisStatus`
/ `definePosition` 调用前缓存过期时。

### 2.1 方向命令取值规则

| 条件 | MT 命令 | CE 命令 |
|------|---------|---------|
| `inverted = false` | `MT{b}=2` | `CE{b}=0` |
| `inverted = true` | `MT{b}=-2` | `CE{b}=2` |

## 3 命令速查表

> **关键**：每条命令发出后**必须等待响应**才能发下一条（串行锁）。
> 写命令成功返回 `:`，出错返回 `<错误信息>?`。
> 查询命令成功返回 `<数据>:`。
> 不读取响应就无法知道命令是否生效，也无法发下一条命令。

### 3.1 运动命令

| 命令 | 响应 | 说明 |
|------|------|------|
| `SH` | `:` | 使能所有轴伺服 |
| `PA{b}=<pulse>` | `:` | 设置绝对目标位置（脉冲） |
| `PR{b}=<pulse>` | `:` | 设置相对移动量（脉冲，可为负） |
| `BG{b}` | `:` | 启动轴运动 |
| `SP{b}=<pulseSpeed>` | `:` | 设置速度（脉冲/秒，持久生效） |
| `HM{b}` | `:` | 执行回零序列 |
| `ST` | `:` | 减速停止所有轴 |
| `ST{b}` | `:` | 减速停止指定轴 |
| `AB` | `:` | 急停所有轴（不减速） |

`{b}` = A | B | C | D（物理轴号）

### 3.2 配置命令

| 命令 | 响应 | 说明 |
|------|------|------|
| `MT{b}=2` | `:` | 电机极性正向 |
| `MT{b}=-2` | `:` | 电机极性反向 |
| `CE{b}=0` | `:` | 编码器方向正向 |
| `CE{b}=2` | `:` | 编码器方向反向 |
| `DP{b}=<pulse>` | `:` | 设定寄存器位置值 |
| `DE{b}=<count>` | `:` | 设定编码器计数值 |

### 3.3 查询命令

| 命令 | 响应 | 用途 |
|------|------|------|
| `TD` | `posA,posB,posC,posD:` | 所有轴寄存器位置 |
| `TP{b}` | `value:` | 单轴编码器位置 |
| `TS` | `byteA,byteB,byteC,byteD:` | 所有轴运行状态字节 |
| `MG _LF{b}` | `float:` | 正向限位开关 |
| `MG _LR{b}` | `float:` | 反向限位开关 |

### 3.4 MG 限位响应解码

```
0.xxxx  → 限位触发
1.xxxx  → 限位未触发
```

## 4 通信协议

```
TX: <命令>\r
RX: <数据>:      ← 成功
    <错误>?      ← 出错
```

TCP 接收端逐字节扫描，遇到 `:`（`0x3A`，成功）或 `?`（`0x3F`，错误）
即作为一个完整响应的边界。分隔符之前的数据为响应载荷。
同一时刻只允许一个命令在等待响应（串行锁 `runExclusive`）。

### 4.1 响应解析示例

**TD — 多轴位置**

```
RX: "1000,2000,3000,4000:"
→ { A: 1000, B: 2000, C: 3000, D: 4000 }
```

按逗号拆分，顺序为 A、B、C、D。

**TS — 运行状态字节**

每个值是一个状态字节，bit 7（`0x80`）= 轴正在运动：

```
RX: "128,0,0,0:"
→ A: moving=true, B/C/D: moving=false
```

**TP{b} — 单轴编码器位置**

```
RX: "200:" → 200
```

去掉尾部 `:` 后解析为数值。如编码器不可用，TP 命令可能报错。

## 5 工程单位 ↔ 脉冲换算

> ⚠️ **命名陷阱**：`AxisConfig.stepsPerRev` 字段存储的是**步距角（°/步）**，
> 而非"每转步数"。实际每转步数 = `360 / stepsPerRev`。默认值 `1.8`（度），
> 意味着 `360 / 1.8 = 200` 步/转。

### 5.1 AxisConfig 字段对照

| 字段 | `kind: "LINEAR"` 含义 | `kind: "ROTARY"` 含义 | 默认值 |
|------|----------------------|----------------------|--------|
| `stepsPerRev` | 步距角（°/步） | 步距角（°/步） | 1.8 |
| `microSteps` | 细分数 | 细分数 | 1 |
| `lead` | 丝杆导程（mm/转） | **未使用**（回退 1） | 1 |
| `gearRatio` | **未使用**（回退 1） | 减速比（如 10 = 10:1） | 1 |
| `maxSpeed` | 最大速度 mm/s | 最大速度 °/s | 10 |
| `minLimit` | 最小位置 mm | 最小位置 ° | — |
| `maxLimit` | 最大位置 mm | 最大位置 ° | — |
| `inverted` | 反转电机+编码器方向 | 反转电机+编码器方向 | false |
| `positionSource` | `"register"` 或 `"encoder"` | 同左 | `"register"` |
| `encoderScale` | 工程单位/编码器计数 | 同左 | 0.005 |
| `encoderCompensation` | 补偿配置（见第 7 节） | 同左 | — |

### 5.2 平移轴（LINEAR）推导

平移轴通过丝杆传动，电机转一圈，滑台移动 `lead` mm：

```
每转步数   = 360 / 步距角          // 例: 360 / 1.8 = 200
每转脉冲数 = 每转步数 × 细分数     // 例: 200 × 4 = 800
每mm脉冲数 = 每转脉冲数 / 导程     // 例: 800 / 4 = 200
```

**公式**：

```
pulsesPerMm = ((360 / stepAngleDeg) × microSteps) / lead
```

**算例**：`stepAngleDeg=1.8, microSteps=4, lead=4`

```
每转步数   = 360 / 1.8 = 200
每转脉冲数 = 200 × 4 = 800
每mm脉冲数 = 800 / 4 = 200脉冲/mm
→ 1mm = 200脉冲, 0.5mm = 100脉冲
```

### 5.3 旋转轴（ROTARY）推导

旋转轴有减速机。`gearRatio` 表示减速比：`gearRatio=10` 意味着电机转
10 圈 = 输出转 1 圈（360°）。直驱旋转轴 `gearRatio=1`。

```
每转步数     = 360 / 步距角
每转脉冲数   = 每转步数 × 细分数
每度脉冲数   = 每转脉冲数 × 减速比 / 360
```

**公式**：

```
pulsesPerDeg = ((360 / stepAngleDeg) × microSteps × gearRatio) / 360
```

**算例**：`stepAngleDeg=1.8, microSteps=4, gearRatio=10`

```
每转步数     = 360 / 1.8 = 200
每转脉冲数   = 200 × 4 = 800
每度脉冲数   = 800 × 10 / 360 ≈ 22.222脉冲/°
→ 1° ≈ 22脉冲, 360° = 8000脉冲
```

### 5.4 编码器换算

编码器是独立反馈器件，通过线性比例因子换算：

```
工程值 = 编码器计数 × encoderScale
编码器计数 = round(工程值 / encoderScale)
```

- `encoderScale` = 每个编码器计数对应的工程单位（默认 `0.005`）
- 平移轴 `0.005 mm/count`：500 计数 = 2.5 mm
- 旋转轴 `0.005 °/count`：500 计数 = 2.5°

### 5.5 换算完整伪代码

```python
def computePulsesPerUnit(axis):
    stepAngleDeg = axis.stepsPerRev ?? 1.8   # ⚠️ 字段名是步距角，不是每转步数
    microSteps   = axis.microSteps ?? 1
    stepsPerRev  = 360 / stepAngleDeg

    if axis.kind == "LINEAR":
        lead = axis.lead ?? 1               # 导程 mm/转
        return (stepsPerRev * microSteps) / lead

    else:  # ROTARY
        gearRatio = axis.gearRatio ?? 1     # 减速比
        return (stepsPerRev * microSteps * gearRatio) / 360

def engineeringToPulse(axis, value):
    return round(value * computePulsesPerUnit(axis))

def pulseToEngineering(axis, pulse):
    return pulse / computePulsesPerUnit(axis)

def encoderCountToEngineering(axis, count):
    return count * (axis.encoderScale ?? 0.005)

def engineeringToEncoderCount(axis, value):
    return round(value / (axis.encoderScale ?? 0.005))
```

## 6 操作流程伪代码

### 6.1 连接

```python
connect():
    socket.connect(host, port)
    send("SH")                              # 使能所有轴
    for axis in [A, B, C, D]:               # 每轴方向配置
        send(f"MT{axis}={motorDir[axis]}")
        send(f"CE{axis}={encoderDir[axis]}")
```

### 6.2 绝对定位 moveTo

```python
moveTo(axis, engineeringPosition):
    pulse = engineeringToPulse(axis, engineeringPosition)
    取消该轴已有的补偿任务
    send(f"PA{axis}={pulse}")
    send(f"BG{axis}")
    if positionSource == "encoder" and encoderCompensation.enabled:
        注册挂起补偿请求 { targetPulse: pulse, targetEngineering: engineeringPosition }
```

### 6.3 相对移动 moveBy

```python
moveBy(axis, engineeringDelta):
    deltaPulse = engineeringToPulse(axis, engineeringDelta)
    if deltaPulse == 0: return          # 零移动直接返回
    取消该轴已有的补偿任务
    if 补偿已启用:
        # 必须先读当前寄存器位置以计算补偿目标
        registerPositions = send("TD")
        currentPulse = registerPositions[axis]
        currentEngineering = pulseToEngineering(axis, currentPulse)
        targetEngineering = currentEngineering + engineeringDelta
        targetPulse = currentPulse + deltaPulse
    send(f"PR{axis}={deltaPulse}")
    send(f"BG{axis}")
    if 补偿已启用:
        注册挂起补偿请求 { targetPulse, targetEngineering }
```

### 6.4 点动 jog

```python
jog(axis, direction, speed):
    maxSpeed = axis.maxSpeed ?? 1
    clampedSpeed = min(speed, maxSpeed)     # 速度截断
    pulsePerUnit = abs(engineeringToPulse(axis, 1))
    pulseSpeed = round(clampedSpeed * pulsePerUnit)
    send(f"SP{axis}={pulseSpeed}")
    stepEngineering = 1 if direction == "forward" else -1   # 移动 1 个工程单位
    stepPulse = engineeringToPulse(axis, stepEngineering)   # ⚠️ 不是 ±1 脉冲！
    send(f"PR{axis}={stepPulse}")
    send(f"BG{axis}")
```

### 6.5 回零 home

```python
home(axis):
    取消该轴补偿任务
    send(f"HM{axis}")       # Galil 回零序列
    send(f"BG{axis}")       # ⚠️ 必须手动启动运动
```

### 6.6 停止与急停

```python
stop(axis):               # 有参数停单轴
    if axis:
        取消该轴补偿
        send(f"ST{axis}")
    else:                   # 无参数停所有轴
        取消所有轴补偿
        send("ST")

emergencyStop():
    取消所有轴补偿
    send("AB")
```

### 6.7 位置定义 definePosition

```python
definePosition(axis, engineeringPosition):
    取消该轴补偿任务
    pulse = engineeringToPulse(axis, engineeringPosition)
    encoderCount = engineeringToEncoderCount(axis, engineeringPosition)
    send(f"DP{axis}={pulse}")           # 寄存器置位
    send(f"DE{axis}={encoderCount}")     # 编码器置位
```

### 6.8 读取状态 getAxisStatus / getAllAxisStatus

```python
getAxisStatus(axis):
    # 总是读取所有轴（TD/TS 是多轴响应）
    registerPositions = send("TD")          # "100,200,300,400:"
    statusBytes       = send("TS")          # "0,128,0,0:"
    # 读取所有轴的限位（4 轴 × 2 方向 = 8 条 MG 命令）
    for bAxis in [A, B, C, D]:
        forwardLimit[bAxis] = parseMgLimit(send(f"MG _LF{bAxis}"))
        reverseLimit[bAxis] = parseMgLimit(send(f"MG _LR{bAxis}"))
    # 编码器轴逐轴读取（TP 不支持多轴查询）
    if positionSource == "encoder":
        for bAxis in [A, B, C, D]:
            try: encoderPositions[bAxis] = send(f"TP{bAxis}")

    # 位置读取
    useEncoder = positionSource == "encoder"
    if useEncoder:
        rawPulse = encoderPositions.get(bAxis) ?? registerPositions[bAxis] ?? 0
        position = encoderCountToEngineering(axis, rawPulse)
    else:
        rawPulse = registerPositions[bAxis] ?? 0
        position = pulseToEngineering(axis, rawPulse)

    # 可能触发挂起补偿→活跃任务 或 检查活跃任务状态
    moving = 硬件运动(TS bit7) OR 补偿进行中
    homed = |position| < 0.001 (或 minLimit ≤ position ≤ maxLimit 且 |position| < 0.001)

    return { name, position, moving, homed, posLimit, negLimit,
             compensating, compensationError, positionError }
```

## 7 编码器补偿流程

启用条件：`positionSource = "encoder"` 且 `encoderCompensation.enabled = true`。
寄存器源轴不补偿（无独立反馈，补偿无意义）。

### 7.1 两阶段架构

补偿不是 `moveTo` 时同步完成，而是分两阶段异步进行：

| 阶段 | 时机 | 内容 |
|------|------|------|
| **挂起请求** (Pending) | `moveTo`/`moveBy` 下发 `BG` 后立即注册 | 记录 `targetPulse`、`targetEngineering`，等运动停止 |
| **活跃任务** (Job) | 下次 `Status()` 轮询发现运动已停止 | 升级为 job，启动独立 goroutine 跑状态机 |

为什么分两阶段：`moveTo` 发完 `PA+BG` 立即返回，硬件还在动；补偿必须等硬件停下来才能读编码器判断误差。`Status()` 是 100ms 周期的轮询入口，在那里检测"运动已停"才安全激活。补偿内部轮询间隔 10ms（`waitForAxisStop`）。

### 7.2 状态机

```
waitingStop → settling → checking ↔ compensating → succeeded
                                     │
                                     ├──→ failed（限位/超时/超 maxCycles/编码器错误）
                                     │
任意状态 ──→ cancelled（代次过期：同轴新 move/jog/home/stop/eStop/disconnect）
```

| 状态 | 行为 |
|------|------|
| `waitingStop` | 轮询 TS（10ms 间隔）直到 bit7 清除。**单次** bit7=0 即返回（非连续 2 次）。同时读限位，限位触发也立即返回让上层判失败 |
| `settling` | `sleep(settleMs)`（默认 100ms）让机械震荡衰减，再读编码器 |
| `checking` | 读 TP 取编码器位置，算误差。`|误差| ≤ tolerance` 或 `≤ minStep` → 收敛；否则查 `maxCycles`，未超则转 `compensating` |
| `compensating` | 重读编码器算修正量，发 `PR`+`BG`，`attempts++`，回 `waitingStop` |
| `succeeded`/`failed`/`cancelled` | 终态，goroutine 退出 |

### 7.3 各状态运算

**checking — 误差计算**

```
encoderCount      = TP{axis}                              # 读编码器计数
encoderEngineering = encoderCount × encoderScale          # → 工程值（如 0.005 mm/count）
errorEngineering   = targetEngineering - encoderEngineering
absError           = |errorEngineering|

status.PositionError = absError                           # 实时同步到 UI

# 收敛判定（任一即可）
if absError <= tolerance  OR  absError <= minStep:
    收敛 → 见 7.4
elif attempts >= maxCycles:
    失败（"exceeded max cycles"）
else:
    转 compensating
```

**compensating — 修正移动**

```
# 重新读一次编码器，避免 settling→checking 间的陈旧数据
currentCount        = TP{axis}
correctionEngineering = targetEngineering - (currentCount × encoderScale)

# 最小步长保护：误差小于 minStep 时强制用 minStep 方向补偿，避免无穷小步进
if |correctionEngineering| < minStep:
    correctionEngineering = sign(correctionEngineering) × minStep

correctionPulse = round(correctionEngineering × pulsesPerUnit)

if correctionPulse == 0:
    # 工程变化太小转不出脉冲，认到位
    收敛 → 见 7.4
else:
    PR{axis}=correctionPulse      # 相对移动（可负）
    BG{axis}
    attempts += 1
    state = waitingStop           # 回循环
```

### 7.4 收敛处理

```
registerPulse = round(encoderEngineering × pulsesPerUnit)   # 编码器实际位置 → 脉冲
DP{axis}=registerPulse                                       # 寄存器同步到编码器实际位置
标记 succeeded，PositionError = 0
```

`DP` 把寄存器位置（电机理论位置）同步到编码器实际位置，消除机械间隙/丢步造成的静态偏差。
注意残留量化误差：`DP` 写入值按脉冲分辨率取整，编码器位置来自独立的 `encoderScale`，
两者分辨率不同时会差半个脉冲（步进系统分辨率足够高时通常可忽略）。

### 7.5 挂起 → 活跃的激活规则

`Status()` 轮询时调 `maybeActivatePendingCompensation`，防静止轴误触发：

```
if TS.bit7 == 1:                      # 硬件还在运动
    pending.observedMotion = true     # 标记"看到过运动"
    return                            # 等下次轮询

# 硬件已停
if not observedMotion:
    if 距 issuedAt <= 启动宽限期(100ms):
        return                        # 硬件可能还没开始动，再等等
    else:
        丢弃 pending                  # 判定静止轴误触发（如零移动）
        return

# 已观察到运动 且 硬件已停 → 升级为 job
job = { generation, targetPulse, targetEngineering, cfg, state: waitingStop }
go runAxisCompensation(job)           # 异步跑状态机
```

宽限期 100ms 避免：`moveTo` 后第一次 `Status` 时硬件还没启动（bit7 仍 0），被误判"没动过"而丢弃。

### 7.6 取消机制

以下操作取消同轴的 pending 和 job（递增 generation，运行中 goroutine 循环顶部检查代次过期即退出）：
`moveTo`、`moveBy`、`jog`、`home`、`stop(轴)`、`stop()`、`emergencyStop()`、`disconnect()`

**已知局限**：代次检查只在循环顶部和状态切换处，补偿中已发出的 `PR+BG` 会发完才退出，
不能立即中断命令序列（取消最多滞后一次修正移动）。

### 7.7 参数物理含义与约束

| 参数 | 含义 | 运算角色 |
|------|------|---------|
| `encoderScale` | 光栅尺分辨率（mm/count 或 °/count） | `工程值 = count × scale` |
| `tolerance` | 收敛容差 | `absError ≤ tolerance` → 成功 |
| `minStep` | 最小修正步长 | ① 误差≤此值也认收敛 ② 修正量小于此值时强制放大到此值 |
| `maxCycles` | 最大修正次数 | `attempts ≥ maxCycles` → 失败 |
| `settleMs` | 停后稳定等待 | sleep 后再读编码器 |
| `timeoutMs` | 整个补偿超时 | 超时 → 失败 |

三层精度约束链（校验在 `ValidateCompensationConfig`）：

```
脉冲当量 Δp = 1 / pulsesPerUnit                  [电机最小步进]
    ≥ encoderScale                                [光栅尺可分辨]  warning
          ≥ tolerance                            [容差可达]      error（阻断保存）
                > minStep                        [不振荡]        warning
minStep ≥ Δp                                      [修正有效]      warning
```

### 7.8 完整运算示例

```
配置: 直线轴, 步距角 1.8°, 细分 4, 导程 4mm, encoderScale=0.005mm/count
      tolerance=0.01mm, minStep=0.001mm, maxCycles=3, settleMs=100ms
      → ppu = (360/1.8)×4/4 = 200 脉冲/mm，脉冲当量 = 0.005 mm/脉冲

MoveTo(1.0mm):
  pulse = round(1.0 × 200) = 200
  PA=200, BG
  pending{ targetEng: 1.0, targetPulse: 200 }

[Status 轮询: bit7 1→0, observedMotion=true] → 升级 job

循环第 1 次:
  waitingStop: 轮询 TS 直到 bit7=0
  settling:    sleep(100ms)
  checking:    TP → count=198
               encoderEng = 198 × 0.005 = 0.99mm
               error = 1.0 - 0.99 = 0.01mm
               absError(0.01) ≤ tolerance(0.01)? 是 → 收敛
               DP=round(0.99 × 200)=198
               succeeded ✓

（若误差更大需多次修正时的循环）
  compensating: 重读 TP → 198
                correction = 1.0 - 0.99 = 0.01mm
                correctionPulse = round(0.01 × 200) = 2
                PR=2, BG, attempts=1, → waitingStop
  [下一轮 checking 再判收敛]
```

### 7.9 PositionError 生命周期

```
checking 阶段:  PositionError = absError        # 实时误差
succeeded:      PositionError = 0                # 清零
failed:         PositionError 保留上次值         # ⚠️ 不清零
disconnect:     PositionError = 0                # 清零
```

补偿进行中 `AxisStatus.Moving` 被强制设为 true（`syncCompensationStatusLocked`），
防止上层 `waitForMotionComplete` 误判运动完成。

## 8 关键注意事项

| 条目 | 说明 |
|------|------|
| 命令结尾 | 传输层自动添加 `\r`，**不要**在命令字符串中手动添加 |
| 串行执行 | 一次只能有一个命令在等待响应，禁止流水线 |
| 伺服使能 | 只使用 `SH`（全轴使能），不使用单轴 `SHA` 或关伺服 `MO` |
| TD vs TP | `TD` 读寄存器位置（始终可用）；`TP` 读编码器（无编码器时可能出错） |
| TS 用法 | 仅使用 bit7（`0x80`）判断运动状态。低位（限位标志）**不使用**——限位通过 `MG _LF`/`MG _LR` 读取 |
| 速度持久 | `SP` 设置后持久生效。`moveTo`/`moveBy` 不设速度，使用最近一次 SP 设置的值 |
| 加减速 | B140 代码中**不使用** `AC`/`DC` 命令，使用控制器默认加减速值 |
| 补偿中的 moving | 补偿活跃时，`moving` 报告 true，直到补偿收敛、失败或被取消 |
| 停止与取消 | `stop(轴)` 取消该轴补偿；`stop()` / `emergencyStop()` / `disconnect()` 取消所有轴补偿。取消最多滞后一次修正移动（见 7.6） |
| 编码器读失败 | `Status()` 中 TP 读取/解析失败不静默回退到寄存器位置，记入 `LastError` 使故障可见；补偿中重读 TP 失败直接判 job 失败，避免误差退化为整段目标造成猛冲 |
| HM 需配合 BG | `HM{axis}` 只是设置回零模式，**必须**紧接着 `BG{axis}` 才会开始运动 |
| jog 单位 | jog 移动 **1 个工程单位**（如 1mm），**不是 1 个脉冲**。脉冲数通过 `engineeringToPulse` 计算 |
| jog 速度截断 | jog 速度上限为 `axis.maxSpeed`，超出时截断。若未传 speed 参数，默认使用 maxSpeed |
| 方向配置刷新 | 每次公开方法调用前都检查方向缓存是否过期（`ensureAxisDirectionConfigured`） |
| homed 含义 | `AxisStatus.homed` **不是**硬件回零标志。当 `|position| < 0.001` 时返回 true（若配置了 minLimit/maxLimit，还需在范围内） |
| TP 逐轴读取 | Galil TP 只支持单轴查询，不能批量读取。无编码器时 TP 可能错误 |
| stepsPerRev 命名 | `AxisConfig.stepsPerRev` 存的是步距角（°/步），默认 1.8，不是"每转步数" |