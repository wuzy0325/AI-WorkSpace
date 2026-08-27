# Spec: DAQ-P-1603 计量设备接入

> 状态：待审核
> 创建：2026-08-19
> 前置：原 1604Pre spec/实现已按用户指示删除（方向纠正为 P1603）
> 决策来源：用户确认（删除 1604Pre 只做 P1603 / 支持每通道量程配置 / DLL 对齐 WindLabX4 模式）

---

## 1. Objective

**目标**：将 DAQ-P-1603 16 通道通用 AI 采集设备作为**计量设备**接入 1604Cal，支持计量模块（实时采集 / 检定流程）完整使用；标定模块仍仅支持 WTN1604。

**用户**：一线计量操作员——现场用 4-20mA 电流环传感器（压力变送器等）接入 P1603，需要在 1604Cal 计量软件中完成采集与检定记录。

**核心用户故事**：
1. 操作员在设备管理中新增计量设备 → 型号下拉出现 "DAQ-P-1603" → 配置 16 通道各自的量程（RangeMin/RangeMax）与单位（**P1603 必须配量程才能输出正确的工程量**）
2. 连接 P1603 → 阀门门禁自动放行（无阀门设备）→ 计量工作台选通道
3. 加压稳定 → 采集 → 16 通道 4-20mA 信号按各通道量程映射为工程量返回
4. 调零（软件 TareOffset）可用

**范围外（明确不做）**：
- 标定模块对 P1603 的支持（`CalibrationCapable` 流程不实现）
- 满量程校准（CalibrateFullScale 返回明确"不支持"；调零 CalibrateZero **本期实现**，软件 TareOffset）
- 采样率配置 UI（固定 WindLabX4 默认 100Hz，adapter 内常量）

---

## 2. 背景与现状

| 项 | 1604 (WTN1604) | P1603 |
|---|---|---|
| 通信 | ASCII WTN1605，TCP 9000 直连 | **DLL FFI**：`WTNDAQ16H_64.dll` 封装 TCP/IP，Go 端持 handle |
| 通道 | 16 压力 | 16 通道通用 AI（0-20mA 量程，4-20mA 传感器标准） |
| 数据链 | 压力值直出 | U16 码值 → 电流(mA) → 工程量（每通道 engMin~engMax 映射） |
| 阀门命令 | w0C01/w0C00 | **无** |
| 单位命令 | u01101/v01101 | **无**（工程量由通道量程配置决定） |
| 硬件校准 | C 00/C 01/C 02/C 03 | **无**（软件 TareOffset 归零） |
| 采集模型 | 同步命令-响应 | 流式 sink 回调（device-sdk 内 readLoop） |

**参考实现**：WindLabX4 `daq_p1603_adapter.go`（thin wrapper 翻译 shared SDK 类型）+ device-sdk `daq/hardware/daq_p1603.go`（真实 DLL FFI 驱动）。

**工程量换算公式**（device-sdk 已验证，adapter 需对齐）：
```
current = (u16Code - 32768) * CodeWidth - OffsetVolt   // 0-20mA 量程（双极性 ±20mA）
engValue = engMin + (current-4)/16 * (engMax-engMin)   // 4-20mA 线性映射
```

> ⚠️ **关键协议结论（2026-08-24 真机修正）**：WTNDAQ16H 的 0-20mA 量程实为
> **双极性 ±20mA**（`GetVoltRangeInfo` 返回 `MinVolt=-20 / MaxVolt=+20 / NeadCode=-32768`），
> **零点（0mA）对应码值 32768 而非 0**。
>
> 早期 spec 曾误写 `current = u16Code / 65535 * 20`（假设单极性 0=0mA），
> 斜率错误约 2 倍，导致打压 -1000Pa 只显示约 -480Pa。正确换算必须用
> `GetVoltRangeInfo` 返回的权威参数 `CodeWidth`/`OffsetVolt`（NeadCode=-32768 即零点码 32768），
> 不能调用 `ScaleBinToVolt`（该函数为电压模式设计，电流模式下会崩溃），
> 也**不能**硬编码 `65535` 作为满量程。

---

## 3. Tech Stack

| 层 | 技术 | 版本 |
|----|------|------|
| 后端 | Go | 1.25.0（已升级，device-sdk 要求） |
| 设备 SDK | shared.local/device-sdk/go（`daq/hardware` + `ffi`） | replace 指向 ../../shared/device-sdk/go（已引入） |
| 桌面壳 | Wails | v2.12.0（不变） |
| 前端 | Vue 3 + TypeScript | 现有 |
| 构建隔离 | GOWORK=off | 已修复 check.ps1 |

**DLL 依赖**：`WTNDAQ16H_64.dll`。启动时 `ffi.InitWTNDAQ16HFromEnv()`（环境变量 `WTNDAQ16H_DLL_PATH` 或可执行文件同目录，对齐 WindLabX4）。**部署需自带 DLL**（安装包后续处理，本期代码就位）。

---

## 4. Commands

```powershell
cd projects\1604Cal
$env:GOWORK='off'
go build ./... ; go test ./... ; go vet ./...

cd web
npm run typecheck ; npm run lint ; npm run build
```

---

## 5. 关键设计决策

| # | 决策点 | 方案 | 理由 |
|---|---|---|---|
| D1 | 设备分类 | `type=measure` + `model="DAQ-P-1603"`，不新增 DeviceType 枚举 | 1604Cal 两级路由（type/model）；工厂按 model 路由 |
| D2 | 驱动来源 | 直接复用 device-sdk `sharedhw.DAQP1603`（真实 DLL FFI 驱动），1604Cal 侧只写 **thin adapter** 翻译到 `MeasureDriver` 接口 | 避免重写 FFI 协议（1604Cal 与 WindLabX4 共用 device-sdk 单源，符合七孔协议复用教训） |
| D3 | 采集适配 | adapter 内部 `SetDataSink` 缓存**最新帧**；首次 `CollectData` 惰性启动 `StartAcquisition`（启动后等首帧）；后续从缓存同步取值；`Disconnect` 前 `StopAcquisition` | 1604Cal 的 `CollectData` 是同步按点取值契约，device-sdk P1603 是流式 sink——缓存最新帧桥接两种模型（与 1604Pre 的 readLoop 缓存同思路，但采集循环在 device-sdk 内部） |
| D4 | 阀门门禁 | adapter `ReadValveStatus` 恒返回 calibration、`SetValveStatus` 幂等空操作；前端 model=DAQ-P-1603 隐藏阀门控件 | 无阀设备恒可用态，计量业务层零改动 |
| D5 | 每通道量程配置 | **扩展 `domain.Device` 增加 `Channels []ChannelConfig`**（index/name/enabled/unit/rangeMin/rangeMax/precision）；adapter 用各通道 rangeMin/rangeMax 做 4-20mA→工程量映射；前端设备表单对 P1603 展示 16 通道量程/单位编辑表 | **用户明确要求**：P1603 必须配置每通道量程才能输出正确数据。通用扩展，WTN1604 也可受益（本期仅 P1603 使用） |
| D6 | 单位 | 工程量单位由各通道 `unit` 字段决定（WindLabX4 默认 Pa）；adapter `ReadUnit` 返回设备级 `Unit`（若空取首通道单位）、`SetUnit` 更新设备级 Unit（软件层，不写硬件） | P1603 无硬件单位命令；单位一致性检查基于设备级 Unit 照常生效 |
| D7 | 归零 | `CalibrateZero` 实现：读当前各通道值 → `SetTare(通道, 当前值)`（软件 TareOffset）；`CalibrateFullScale` 返回明确"不支持" | 对齐 device-sdk `TareConfigurable` 与 WindLabX4 P1603 归零能力 |
| D8 | DLL 初始化 | `main.go` 启动时 `ffi.InitWTNDAQ16HFromEnv()`（对齐 WindLabX4）；失败仅告警不阻塞启动（未使用 P1603 时不影响） | DLL 加载幂等（sync.Once）；未配置 P1603 时应用应正常启动 |
| D9 | 连接生命周期 | Connect → `DAQP1603.Connect()`（FFI Create+VerifyParam+InitTask）；Disconnect → `StopAcquisition` + `DAQP1603.Disconnect()`（StopTask+ReleaseTask+DevRelease） | 对齐 device-sdk 生命周期；DLL 内部管理 socket，Go 端不接触 net.Conn |
| D10 | 超时 | device-sdk 内部处理 DLL 调用超时；adapter 层：首帧等待复用 WindLabX4 P1603 默认（StartAcquisition 后首帧 <100ms @100Hz，等待 2s 兜底） | P1603 无 socket deadline 概念（FFI 路径），ADR-009 的 deadline-ignore 场景不适用 |
| D11 | 码值换算（2026-08-24 修正） | 0-20mA 量程为双极性 ±20mA，零点码值 32768；Go 端用 `GetVoltRangeInfo` 返回的 `CodeWidth`/`OffsetVolt` 按 `(code-32768)*CodeWidth - OffsetVolt` 换算，**不调用 `ScaleBinToVolt`**（电流模式崩溃），**不硬编码 65535** | 真机（192.168.3.104）打压 -1000Pa 早期版本只显示 -480Pa，根因是误用 `code/65535*20` 单极性假设；见 §2 关键协议结论 |

### D3 补充：采集时序

```
首次 CollectData:
  ensureAcquisition → dev.StartAcquisition()（FFI StartTask + SendSoftTrig，启动 readLoop）
  → SetDataSink 已注册（Connect 时），缓存最新帧
  → 等待首帧到达（2s 超时）
CollectData(ctx, channels):
  从缓存最新帧取 channels 对应通道值（已按各通道量程换算为工程量）
Disconnect:
  dev.StopAcquisition()（close stop → join readLoop 1s）
  dev.Disconnect()（StopTask + ReleaseTask + DevRelease）
```

---

## 6. Project Structure

### 6.1 新增文件

```
projects/1604Cal/
├── internal/infrastructure/driver/
│   ├── p1603_driver.go              # P1603 thin adapter：缓存帧 + CollectData + 归零 + 阀门桩
│   └── p1603_driver_test.go         # 单测：工程量映射/通道量程/归零/阀门桩/首帧等待
└── docs/plans/
    └── 2026-08-19-p1603-measure-device-spec.md   # 本文档
```

### 6.2 修改文件

```
projects/1604Cal/
├── main.go                          # + ffi.InitWTNDAQ16HFromEnv()（DLL 初始化）
├── internal/domain/device.go        # + ChannelConfig 类型 + Device.Channels 字段
├── internal/api/http/device_handler.go  # upsertDeviceRequest + channels 透传
├── internal/infrastructure/driver/
│   └── factory.go                   # CreateMeasureDriver 注册 "DAQ-P-1603" 分支
└── web/src/
    ├── types/device.ts              # DeviceDTO + channels
    └── components/device/
        └── DeviceFormDialog.vue     # model=DAQ-P-1603 选项 + 每通道量程/单位编辑表
```

### 6.3 明确不改的文件

- `internal/application/measurement/*` —— D4 方案下计量业务零改动
- `internal/application/calibration/*` —— 标定模块不接入 P1603
- `internal/device/interfaces.go` —— 不新增接口（MeasureDriver 已覆盖所需能力）

---

## 7. Code Style

遵循 [AGENTS.md](../../AGENTS.md)（中文注释解释"为什么"、错误小写、≤50 行/函数、≤500 行/文件）。adapter 骨架：

```go
// P1603Driver DAQ-P-1603 计量设备 thin adapter。
//
// 职责：在 1604Cal 的 device.MeasureDriver（同步按点采集契约）与
// device-sdk 的 sharedhw.DAQP1603（流式 sink 回调）之间做翻译。
// 真实 DLL FFI 协议逻辑全部在 device-sdk 内（单源复用，对齐 WindLabX4）。
//
// 关键点：
//   - 每通道工程量 = engMin + (current-4)/16*(engMax-engMin)，engMin/engMax
//     来自 domain.Device.Channels 的 rangeMin/rangeMax（必须配置才能正确输出）
//   - 阀门桩恒 calibration（无阀设备恒可用态）
//   - 归零走软件 TareOffset（SetTare），无硬件校准命令
type P1603Driver struct {
    mu     sync.Mutex
    dev    *sharedhw.DAQP1603   // device-sdk 真实驱动
    config domain.Device        // 含每通道量程/单位配置
    // 最新帧缓存（sink 回调写入，CollectData 读取）
    latestFrame []float64
    frameValid  bool
    firstFrame  chan struct{}
    unit        string
}
```

---

## 8. Testing Strategy

| 层级 | 位置 | 覆盖点 |
|---|---|---|
| 驱动单测 | `p1603_driver_test.go` | ① 工程量映射：4mA→engMin / 20mA→engMax / 12mA→中间值（各通道量程独立）② 通道量程缺省回退（未配置 Channels → 默认 ±5000 Pa）③ 归零 SetTare（当前值→offset）④ 阀门桩语义 ⑤ 首帧等待/超时 ⑥ 单位桩 ⑦ 满量程不支持 |
| 工厂单测 | `factory_test.go` 扩展 | model="DAQ-P-1603"（含大小写归一化）→ 返回 P1603 驱动 |
| 回归 | 全量 | `go test ./...` + `go vet` + 前端 typecheck/lint/build 全绿；现有 WTN1604/打压设备测试不受影响 |
| 联调（人工） | 真机 | ① DLL 加载 ② Connect（FFI Create+InitTask）③ 16 通道采集数值与已知传感器比对 ④ 量程配置影响工程量 ⑤ 归零后读数归零 |

---

## 9. Boundaries

- **Always**：device-sdk 单源复用（不复制 FFI 协议）；中文注释；GOWORK=off 构建验证；现有测试全绿
- **Ask first**：安装包 DLL 分发策略（本期仅代码就位，不打包 DLL）；domain.Device 模型变更（Channels 字段）
- **Never**：不复制 device-sdk FFI 代码；不动标定模块；不实现满量程校准；不引入超前抽象

---

## 10. Success Criteria

1. 设备管理中可创建 model="DAQ-P-1603" 的计量设备，前端展示 16 通道量程/单位编辑表
2. `Factory.CreateMeasureDriver` 对 "DAQ-P-1603"（含大小写归一化）返回 P1603 驱动
3. 连接后 `ReadValveStatus` 返回 calibration，计量工作台启动门禁通过
4. `CollectData([1..16])` 返回按各通道量程换算的工程量（mock sink 注入帧，值正确）
5. 通道量程未配置时回退默认（±5000 Pa），配置后按 rangeMin/rangeMax 映射
6. `CalibrateZero` 逐通道 SetTare(当前值) 成功；`CalibrateFullScale` 返回"不支持"
7. `go build/test/vet`（GOWORK=off）+ 前端 typecheck/lint/build 全绿，现有测试零回归
8. 前端 model=DAQ-P-1603 时阀门控制区隐藏，WTN1604 不受影响
9. `main.go` 启动调用 `ffi.InitWTNDAQ16HFromEnv()`（DLL 缺失时仅告警不阻塞）

---

## 11. Open Questions

| # | 问题 | 影响 | 处置 |
|---|---|---|---|
| Q1 | P1603 设备级 Unit 与各通道 Unit 的关系：设备面板的单位下拉切换应作用于哪一层？ | D6 单位语义 | 本期：设备级 Unit 下拉（作用于设备级字段 + 一致性检查）；各通道单位仅在通道配置表编辑，不做下拉联动 |
| Q2 | DLL 安装路径：1604Cal 安装包是否把 WTNDAQ16H_64.dll 打到应用目录？ | 部署 | 本期代码就位（可执行文件同目录可加载）；安装包改动在打包阶段处理 |
