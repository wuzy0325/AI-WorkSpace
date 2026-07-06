# WTNMC4A 运动控制器 — FFI/DLL 参考

## 0 官方资料位置（权威源）

调试 SDK 字段语义、命令格式时**优先查阅这些官方资料**，不要凭记忆推测。

| 资料 | 路径 | 用途 |
|------|------|------|
| **官方头文件** | [device-lab/drivers/位移机构/WTNMC4A/SDK/WTNMC4A.H](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/device-lab/drivers/位移机构/WTNMC4A/SDK/WTNMC4A.H) | 所有 `#define` 常量、结构体字段定义、函数签名的权威来源 |
| 官方资源头文件 | [device-lab/drivers/位移机构/WTNMC4A/SDK/WTNMC4ARSV.h](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/device-lab/drivers/位移机构/WTNMC4A/SDK/WTNMC4ARSV.h) | 资源 ID 定义 |
| 静态链接库（32/64 位） | `device-lab/drivers/位移机构/WTNMC4A/SDK/WTNMC4A.lib` / `WTNMC4A_64.lib` | C/C++ 链接用；Go/TS 通过 FFI 调用 DLL 不需要 |
| **官方 VC 示例** | [device-lab/drivers/位移机构/WTNMC4A/Samples/](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/device-lab/drivers/位移机构/WTNMC4A/Samples) | 18 个完整 VC 示例，验证字段组合的最佳参考 |
| 设备手册 | [device-lab/drivers/位移机构/ACTS1200S_WTNMC4A.pdf](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/device-lab/drivers/位移机构/ACTS1200S_WTNMC4A.pdf) | ACTS1200S 机构 + WTNMC4A 控制器综合手册 |

### 最常引用的示例

| 示例 | 适用场景 |
|------|----------|
| `Samples/正反方向软件限位/` | 验证 `Direction` / `PLSLogLever` 字段组合 |
| `Samples/单轴直线S曲线驱动/` | 验证 `InitLVDV` + `StartLVDV` 调用顺序 |
| `Samples/外部信号启动电机定长或连续驱动/` | 验证定长（`LV_DV=0`）vs 连续（`LV_DV=1`）模式差异 |
| `Samples/自动原点搜寻/` | 验证 `StartAutoHomeSearch` 用法 |

## 1 概述

WTNMC4A 是 ART 北京阿尔泰公司的 4 轴步进/伺服运动控制卡。上位机通过 **koffi (FFI)** 调用官方 `WTNMC4A.dll` / `WTNMC4A_64.dll` 来操作硬件。

| 参数 | 值 |
|------|-----|
| 调用方式 | koffi FFI（Node.js 原生 DLL 绑定） |
| 连接函数 | `WTNMC4A_DEV_CreateA(ip, sendTimeout, recvTimeout)` |
| 返回类型 | `void *`（设备句柄 Buffer） |
| 轴映射 | X→0, Y→1, Z→2, U→3 |
| 速度范围 | 1–8000 脉冲/s |
| 加速度范围 | 125–1000000 |
| DLL 搜索路径 | `C:\ART\WTNMC4A\Driver\INF\Win32&Win64\amd64\WTNMC4A_64.dll` |
| | `C:\Windows\System32\WTNMC4A_64.dll` |
| | `{cwd}\dll\WTNMC4A_64.dll` |

> **关键**：DLL 内部封装了完整的 TCP 通信协议，对外只暴露 C 风格 API。上层代码无需关心 TCP 连接、粘包、串行锁等问题。

## 2 DLL 加载与函数注册

### 2.1 加载流程

```python
function loadDLL(dllPath):
    lib = koffi.load(dllPath)          # 加载原生 DLL
    registerFunctions(lib)             # 注册所有导出函数
    return lib
```

### 2.2 类型定义

DLL 使用 3 个 C 结构体传参，需通过 koffi 映射：

**WTNMC4A_PARA_DataList** — 驱动参数：
```
struct {
    long Multiple;        # 倍率 (1~500)
    long StartSpeed;      # 初始速度 (1~8000)
    long DriveSpeed;      # 驱动速度 (1~8000)
    long Acceleration;    # 加速度 (125~1000000)
    long Deceleration;    # 减速度 (125~1000000)
    long AccIncRate;      # 加速度变化率 (954~62500000)
    long DecIncRate;      # 减速度变化率 (954~62500000)
}
```

**WTNMC4A_PARA_LCData** — 运动模式参数：
```
struct {
    long AxisNum;         # 轴号 0~3
    long LV_DV;           # 0=定长驱动, 1=连续驱动
    long DecMode;         # 0=自动减速, 1=手动减速
    long PulseMode;       # 0=CW/CCW, 1=CP/DIR
    long PLSLogLever;     # 脉冲信号逻辑电平（=方向）
    long DIRLogLever;     # 方向信号逻辑电平
    long Line_Curve;      # 0=直线, 1=S曲线
    long Direction;       # 0=反方向, 1=正方向
    long nPulseNum;       # 定量输出脉冲数 (0~268435455)
}
```

**WTNMC4A_PARA_RR1** — RR1 状态寄存器（完整结构）：
```
struct {
    uint CMPP;            # 逻辑/实位 ≥ COMP+
    uint CMPM;            # 逻辑/实位 < COMP-
    uint ASND;            # 加速中
    uint CNST;            # 定速中
    uint DSND;            # 减速中
    uint AASND;           # S曲线加速度增加
    uint ACNST;           # S曲线加速度不变
    uint ADSND;           # S曲线加速度减少
    uint IN0;             # 外部停止信号 IN0
    uint IN1;             # 外部停止信号 IN1
    uint IN2;             # 外部停止信号 IN2
    uint IN3;             # 外部停止信号 IN3
    uint LMTP;            # 正方向限位
    uint LMTM;            # 反方向限位
    uint ALARM;           # 伺服报警
    uint EMG;             # 紧急停止
}
```

### 2.3 函数签名（koffi 映射）

```
WTNMC4A_DEV_CreateA      (str ip, long sendTimeout, long recvTimeout) → void *
WTNMC4A_DEV_Release      (void *hDevice) → bool
WTNMC4A_Reset            (void *hDevice) → bool

WTNMC4A_SetSV            (void *hDevice, long axis, long speed) → bool
WTNMC4A_SetV             (void *hDevice, long axis, long speed) → bool
WTNMC4A_SetA             (void *hDevice, long axis, long accel) → bool
WTNMC4A_SetDec           (void *hDevice, long axis, long decel) → bool
WTNMC4A_SetP             (void *hDevice, long axis, long pulse) → bool
WTNMC4A_SetLP            (void *hDevice, long axis, long value) → bool
WTNMC4A_SetEP            (void *hDevice, long axis, long value) → bool

WTNMC4A_InitLVDV         (void *hDevice, PDataList, PLCData) → bool
WTNMC4A_StartLVDV        (void *hDevice, long axis) → bool
WTNMC4A_DecStop          (void *hDevice, long axis) → bool
WTNMC4A_InstStop         (void *hDevice, long axis) → bool

WTNMC4A_ReadLP           (void *hDevice, long axis) → long
WTNMC4A_ReadEP           (void *hDevice, long axis) → long
WTNMC4A_ReadCV           (void *hDevice, long axis) → long
WTNMC4A_ReadCA           (void *hDevice, long axis) → long
WTNMC4A_GetRR1Status     (void *hDevice, long axis, PRR1) → bool

WTNMC4A_StartAutoHomeSearch (void *hDevice, long axis) → bool

WTNMC4A_ClearSoftwareLimit    (void *hDevice, long axis) → bool
WTNMC4A_SetPDirSoftwareLimit  (void *hDevice, long axis, long value) → bool
WTNMC4A_SetMDirSoftwareLimit  (void *hDevice, long axis, long value) → bool
```

所有函数返回 `true`（成功）或 `false`（失败），调用后应检查返回值。

## 3 常量定义

```
# 轴号
WTNMC4A_XAXIS = 0
WTNMC4A_YAXIS = 1
WTNMC4A_ZAXIS = 2
WTNMC4A_UAXIS = 3

# 驱动方式
WTNMC4A_DV = 0   # 定长驱动
WTNMC4A_LV = 1   # 连续驱动

# 减速方式
WTNMC4A_AUTO_DEC   = 0  # 自动减速
WTNMC4A_MANUAL_DEC = 1  # 手动减速

# 脉冲方式
WTNMC4A_CW_CCW = 0  # CW/CCW
WTNMC4A_CP_DIR = 1  # CP/DIR

# 运动方式
WTNMC4A_LINE  = 0  # 直线
WTNMC4A_CURVE = 1  # S曲线

# 方向
WTNMC4A_MDIRECTION = 0  # 反方向
WTNMC4A_PDIRECTION = 1  # 正方向
```

## 4 RR1 状态寄存器

DLL 版 RR1 是一个完整结构体（不是 TCP 版的位掩码整数），通过 `GetRR1Status` 填充后读取：

```python
class RR1Status:
    CMPP: 0 or 1   # 逻辑/实位 ≥ COMP+
    CMPM: 0 or 1   # 逻辑/实位 < COMP-
    ASND: 0 or 1   # 加速中
    CNST: 0 or 1   # 定速中
    DSND: 0 or 1   # 减速中
    AASND: 0 or 1  # S曲线加速度增加
    ACNST: 0 or 1  # S曲线加速度不变
    ADSND: 0 or 1  # S曲线加速度减少
    IN0: 0 or 1    # 外部停止信号 IN0
    IN1: 0 or 1    # 外部停止信号 IN1
    IN2: 0 or 1    # 外部停止信号 IN2
    IN3: 0 or 1    # 外部停止信号 IN3
    LMTP: 0 or 1   # 正方向限位
    LMTM: 0 or 1   # 反方向限位
    ALARM: 0 or 1  # 伺服报警
    EMG: 0 or 1    # 紧急停止
```

### 运动判断

DLL 版与 TCP 版不同，运动判定使用 **ASND / CNST / DSND 三字段组合**：

```python
function isMoving(rr1):
    return rr1.ASND == 1 or rr1.CNST == 1 or rr1.DSND == 1
```

限位状态直接读取 `rr1.LMTP` / `rr1.LMTM`。

## 5 工程单位 ↔ 脉冲换算

与 B140 共用同一套换算公式，详见 B140 skill 第 5 节。

### 5.1 关键公式

```python
function computePulsesPerUnit(axis):
    stepAngleDeg = axis.stepsPerRev ?? 1.8    # ⚠️ 字段名 stepsPerRev，实际存步距角
    microSteps   = axis.microSteps ?? 1
    stepsPerRev  = 360 / stepAngleDeg

    if axis.kind == "LINEAR":
        return (stepsPerRev * microSteps) / (axis.lead ?? 1)   # 脉冲/mm
    else:  # ROTARY
        return (stepsPerRev * microSteps * (axis.gearRatio ?? 1)) / 360  # 脉冲/度

function engineeringToPulse(axis, value):
    pulse = round(value * computePulsesPerUnit(axis))
    return -pulse if axis.inverted else pulse

function pulseToEngineering(axis, pulse):
    signedPulse = -pulse if axis.inverted else pulse
    return signedPulse / computePulsesPerUnit(axis)
```

### 5.2 速度换算关键

DLL 的 `SetV` 接收的是**脉冲速度**（1~8000），需要先通过 PPU 换算：

```python
function speedEngineeringToPulse(axis, speed):
    pulsePerUnit = abs(engineeringToPulse(axis, 1))
    pulseSpeed = round(speed * pulsePerUnit)
    return clamp(pulseSpeed, 1, 8000)     # DLL 硬件限制 1~8000
```

## 6 操作流程伪代码

> **注意**：所有 DLL 函数都要求先 `DEV_Create` 拿到有效句柄 `hDevice`。

### 6.1 连接

```python
class WTNMC4AController:
    lib: WTNMC4ALib         # DLL 加载器
    hDevice: Buffer | None   # 设备句柄
    connected: bool = false
    running: bool = false

    function connect(profile):
        lib = WTNMC4ALib(dllPath)   # 搜索并加载 DLL
        lib.load()                  # koffi.load → 注册函数

        ip = profile.address
        # DEV_CreateA: (IP, 发送超时ms, 接收超时ms)
        hDevice = lib.DEV_Create(ip, 200, 200)
        if hDevice is None:
            throw Error("Failed to create device handle")

        connected = true
        running = true

        # 各轴设置默认速度
        for each axis in profile.axes where axis.enabled:
            if axis.maxSpeed > 0:
                setSpeed(axis)
        return true
```

### 6.2 设置速度

```python
function setSpeed(axis):
    axisNum = axisNameToNumber(axis.name)    # X→0, Y→1, Z→2, U→3
    pulseSpeed = speedEngineeringToPulse(axis, speed)
    ok = lib.SetV(hDevice, axisNum, pulseSpeed)
    if not ok: throw Error("SetV failed")
```

### 6.3 设置完整速度参数

```python
function setVelocity(axis, params):
    axisNum = axisNameToNumber(axis)
    if not lib.SetSV(hDevice, axisNum, params.startSpeed):   throw ...
    if not lib.SetV(hDevice, axisNum, params.driveSpeed):    throw ...
    if not lib.SetA(hDevice, axisNum, params.acceleration):  throw ...
    if not lib.SetDec(hDevice, axisNum, params.deceleration): throw ...
```

### 6.4 绝对定位 moveTo

DLL 版使用 `InitLVDV` + `StartLVDV` 组合，与 TCP 版（SETP + START）完全不同：

```python
function moveTo(axis, position):
    axisNum = axisNameToNumber(axis)
    currentPulse = lib.ReadLP(hDevice, axisNum)
    targetPulse = engineeringToPulse(axis, position)
    deltaPulse = targetPulse - currentPulse

    if deltaPulse == 0: return    # 已在目标位

    direction = WTNMC4A_PDIRECTION if deltaPulse > 0 else WTNMC4A_MDIRECTION

    # 填充 DataList — 驱动参数
    dataList = {
        Multiple: 1,
        StartSpeed: 100,
        DriveSpeed: 2000,          # 1~8000 脉冲/s
        Acceleration: 1000,        # 125~1000000
        Deceleration: 1000,        # 125~1000000
        AccIncRate: 10000,
        DecIncRate: 10000
    }

    # 填充 LCData — 运动模式
    lcData = {
        AxisNum: axisNum,
        LV_DV: WTNMC4A_DV,        # 定长驱动
        DecMode: WTNMC4A_AUTO_DEC,
        PulseMode: WTNMC4A_CP_DIR,
        PLSLogLever: direction,    # 脉冲方向
        DIRLogLever: 0,
        Line_Curve: WTNMC4A_LINE,  # 直线
        Direction: direction,
        nPulseNum: abs(deltaPulse)
    }

    # 初始化驱动参数（写入硬件寄存器）
    if not lib.InitLVDV(hDevice, dataList, lcData):
        throw Error("InitLVDV failed")

    # 启动驱动（开始运动）
    if not lib.StartLVDV(hDevice, axisNum):
        throw Error("StartLVDV failed")
```

### 6.5 相对移动 moveBy

```python
function moveBy(axis, delta):
    status = getAxisStatus(axis)
    moveTo(axis, status.position + delta)
```

### 6.6 点动 jog

```python
function jog(axis, direction, speed):
    axisNum = axisNameToNumber(axis)

    # 速度换算 + 截断
    pulseSpeed = speedEngineeringToPulse(axis, speed)

    dir = WTNMC4A_PDIRECTION if direction == "forward" else WTNMC4A_MDIRECTION

    dataList = {
        Multiple: 1,
        StartSpeed: 100,
        DriveSpeed: pulseSpeed,
        Acceleration: 1000,
        Deceleration: 1000,
        AccIncRate: 10000,
        DecIncRate: 10000
    }

    lcData = {
        AxisNum: axisNum,
        LV_DV: WTNMC4A_LV,         # 连续驱动 ⚠️ 与 moveTo 不同
        DecMode: WTNMC4A_AUTO_DEC,
        PulseMode: WTNMC4A_CP_DIR,
        PLSLogLever: dir,
        DIRLogLever: 0,
        Line_Curve: WTNMC4A_LINE,
        Direction: dir,
        nPulseNum: 0               # 连续驱动填 0
    }

    if not lib.InitLVDV(hDevice, dataList, lcData): throw ...
    if not lib.StartLVDV(hDevice, axisNum): throw ...
```

### 6.7 回零 home

```python
function home(axis):
    axisNum = axisNameToNumber(axis)
    if not lib.StartAutoHomeSearch(hDevice, axisNum):
        throw Error("Home failed")
```

### 6.8 停止与急停

```python
function stop(axis=None):
    if axis:
        lib.DecStop(hDevice, axisNameToNumber(axis))   # 减速停止
    else:
        for each axisNum in [0, 1, 2, 3]:
            lib.DecStop(hDevice, axisNum)              # 全部减速停止

function emergencyStop():
    for each axisNum in [0, 1, 2, 3]:
        lib.InstStop(hDevice, axisNum)                 # 立即停止（不减速）
```

### 6.9 读取轴状态

DLL 版可读取完整 RR1 结构体，限位和报警信息直接可用：

```python
function getAxisStatus(axis):
    axisNum = axisNameToNumber(axis)
    logicalPos = lib.ReadLP(hDevice, axisNum)
    rr1 = lib.GetRR1Status(hDevice, axisNum)   # 返回完整的 RR1 结构体

    engineeringPos = pulseToEngineering(axis, logicalPos)
    homed = abs(engineeringPos) < 0.001
    moving = rr1.ASND == 1 or rr1.CNST == 1 or rr1.DSND == 1

    return {
        name: axis,
        position: engineeringPos,
        moving: moving,
        homed: homed,
        posLimit: rr1.LMTP == 1,
        negLimit: rr1.LMTM == 1,
        alarm: rr1.ALARM == 1,
        emergency: rr1.EMG == 1
    }
```

### 6.10 读取计数器

```python
function readLogicalPosition(axis):
    return lib.ReadLP(hDevice, axisNameToNumber(axis))

function readEncoderPosition(axis):
    return lib.ReadEP(hDevice, axisNameToNumber(axis))

function readCurrentVelocity(axis):
    return lib.ReadCV(hDevice, axisNameToNumber(axis))

function readCurrentAcceleration(axis):
    return lib.ReadCA(hDevice, axisNameToNumber(axis))

function setLogicalPosition(axis, value):
    if not lib.SetLP(hDevice, axisNameToNumber(axis), value): throw ...

function setEncoderPosition(axis, value):
    if not lib.SetEP(hDevice, axisNameToNumber(axis), value): throw ...
```

### 6.11 软件限位

```python
function setSoftwareLimit(axis, positive, negative):
    axisNum = axisNameToNumber(axis)
    lib.SetPDirSoftwareLimit(hDevice, axisNum, positive)
    lib.SetMDirSoftwareLimit(hDevice, axisNum, negative)

function clearSoftwareLimit(axis):
    lib.ClearSoftwareLimit(hDevice, axisNameToNumber(axis))
```

### 6.12 复位与断开

```python
function disconnect():
    if hDevice:
        lib.Reset(hDevice)            # 复位硬件
        lib.DEV_Release(hDevice)       # 释放设备句柄
        hDevice = None
    connected = false
    running = false
```

## 7 连接生命周期

```
IDLE → LOAD_DLL → CONNECTING → CONFIGURING → READY → DISCONNECTING → IDLE
```

| 状态 | 动作 |
|------|------|
| LOAD_DLL | koffi.load 查找并加载 `WTNMC4A.dll`，注册所有函数签名 |
| CONNECTING | `DEV_CreateA(ip, 200, 200)` 创建设备句柄，DLL 内部发起 TCP 连接 |
| CONFIGURING | 遍历 enabled 轴，调用 `SetV` 设置默认速度 |
| READY | 可接受 moveTo/jog/home/stop/getAxisStatus 等操作 |
| DISCONNECTING | `Reset` → `DEV_Release` → 句柄置 null |

## 8 关键注意事项

| 条目 | 说明 |
|------|------|
| **DLL 路径** | DLL 默认搜索路径：`C:\ART\WTNMC4A\Driver\INF\Win32&Win64\amd64\`、`C:\Windows\System32`、`{cwd}\dll`。找不到时抛异常 |
| **DEV_Create 无端口参数** | 函数签名为 `(ip, sendTimeout, recvTimeout)`，无 port 参数。DLL 内部管理 TCP 端口，外部无法直接 telnet 连接 |
| **返回值检查** | 所有 DLL 函数返回 bool，**必须**检查返回值。失败时记录 axis/操作信息后再抛异常 |
| **hDevice 生命周期** | 句柄由 `DEV_Create` 创建，`DEV_Release` 释放。释放后不可再用 |
| **InitLVDV + StartLVDV** | 运动控制分为两步：`InitLVDV` 写入参数到硬件，`StartLVDV` 启动运动。缺一不可 |
| **定长 vs 连续** | `LV_DV=0` 定长（moveTo），`nPulseNum` 指定脉冲数；`LV_DV=1` 连续（jog），`nPulseNum=0` |
| **InitLVDV 互斥** | 两次运动之间需重新调用 `InitLVDV` 更新参数（速度、位置等），不能复用上次的 lcData |
| **速度范围** | `SetV` 的脉冲速度必须在 1~8000 之间，加速度在 125~1000000 之间。超出时 DLL 可能静默失败 |
| **moveTo 读当前位置** | moveTo 内部需先 `ReadLP` 读当前脉冲位置，计算差值后填入 `nPulseNum`（相对脉冲数） |
| **Direction 一致性** | `PLSLogLever` 和 `Direction` 保持相同的方向值（`WTNMC4A_PDIRECTION=1` 正方向 / `WTNMC4A_MDIRECTION=0` 反方向） |
| **PulseMode 必须用 CP/DIR（=1）** | 官方示例虽多用 `WTNMC4A_CWCCW=0`（CW/CCW 方式），但 wind-daq 实测在该模式下负方向位移台不移动。必须用 `WTNMC4A_CPDIR=1`（CP/DIR 方式）+ `PLSLogLever=direction` + `DIRLogLever=0` 的组合，对齐 Cursor DAQ 实测可用写法。详见 [wtnmc4a_motion.go:34-48](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go#L34-L48)。**反面案例**：早期常量名 `cpDir=0` 是命名误导——值 0 实际是 CW/CCW；只改 `PLSLogLever=direction` 不改 `PulseMode` 会让正向脉冲极性反转，正向变反向 |
| **DecStop vs InstStop** | `DecStop` 减速停止（按设定的减速度），`InstStop` 立即停止（急停，不减速） |
| **限位直接读取** | DLL 版 RR1 直接包含 `LMTP`/`LMTM` 限位状态，无需额外 MG 命令 |
| **homed 推断** | `homed` 不是硬件回零标志，由 `abs(engineeringPos) < 0.001` 推断 |
| **koffi 结构体传参** | 结构体用 `koffi.struct({...})` 定义，传参用 `koffi.pointer(struct)`，对应 DLL 的 `PDataList`/`PLCData` 指针类型 |
| **GetRR1Status 预初始化** | 调用前需先初始化结构体全部字段为 0，避免悬空指针 |
