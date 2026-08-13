# Spec: DAQ-T1602 温度扫描阀设备集成到 wind-daq

> **协议规范来源**：[`../DAQ-T1602-LabVIEW逻辑解析.md`](../DAQ-T1602-LabVIEW逻辑解析.md)（LabVIEW 源码逆向解析）+ **真机实测**（2026-08-12，设备 192.168.3.201，见 §Protocol 实测校准）
>
> **量程表状态**：✅ 已定案（2026-08-13）。用户提供完整 10 行量程表并经真机交叉验证（K 型 raw 1513 → 27.7℃ 与室温吻合，T 型配置 27.2℃ 一致）；公式经用户确认；raw=0 = 通道未接入（UI 显示 "--"）。见 §Type Code 枚举。

---

## Objective

将 DAQ-T1602 温度扫描阀集成到 wind-daq，作为新的 DAQ 设备类型。T1602 是 **Modbus TCP** 温度采集设备：机内含 2 张独立采集卡（不同网卡、不同 Slave ID），每卡 8 通道，合并对外为 16 通道单设备。

### 与现有 DAQ-T-1603 的身份隔离（Critical）

wind-daq 已存在 **DAQ-T-1603**（`Type = "DAQ-T-1603"`，裸 TCP ASCII 协议 + BIN 二进制）。T1602 与 T1603 协议、传输方式、数据语义完全不同，必须严格隔离：

| 维度 | DAQ-T-1603（现有） | DAQ-T-1602（新增） |
|---|---|---|
| `device.Type` 字面量 | `"DAQ-T-1603"` | `"DAQ-T-1602"` |
| 传输 | 裸 TCP ASCII 文本（@e3/@f0/...） | **Modbus TCP**（端口 502） |
| 物理结构 | 单卡 16 通道 | **2 张独立卡**，Slave ID 1/2，每卡 8 通道 |
| 配置接口 | `DaqT1603Configurable` | `DaqT1602Configurable`（**独立接口，禁止复用**） |
| 适配器文件 | `t1603_adapter.go` | `t1602_adapter.go`（**独立文件**） |
| shared SDK 驱动 | `daq/hardware/daq_t1603.go` | `daq/hardware/daq_t1602.go`（**独立文件**） |
| Modbus 协议栈 | 无 | `protocol/modbus/`（**全 workspace 首个 Modbus 实现**） |
| 前端配置组件 | `DaqT1603Config.vue` | `DaqT1602Config.vue`（**独立组件，仿 T1603 风格简化**） |

**命名红线**：禁止在 T1602 代码中使用 `T1603`、`DAQT1603`、`daq_t1603` 等字样（驱动/适配器/配置结构），反之亦然。

### 用户故事

作为风洞实验人员，我希望在 wind-daq 中：
1. 添加 DAQ-T-1602 设备，配置 IP（默认 192.168.3.201）和端口（默认 502）
2. 连接后自动读取 16 通道热电偶类型（两卡寄存器 200~207）并在 UI 展示
3. 支持修改 16 通道热电偶类型并写入设备（FC6 写单个寄存器）
4. 启动采集后实时接收 16 通道温度值，显示在波形图
5. 断连后状态正确切换，数据帧以实际采集速率（~4.9 Hz）推送

### 成功标准

1. DAQ-T-1602 可作为设备类型添加到 wind-daq，前后端 `Type = "DAQ-T-1602"` 一致，与 T1603 完全隔离
2. Connect → 建立**一条** Modbus TCP 连接（同一 IP:502，Unit ID 1/2 复用，见 §Protocol 连接模型）→ 读回 16 通道类型
3. ApplyConfig → 逐通道写类型寄存器（FC6）→ 读回校验
4. StartAcquisition → 周期轮询双卡输入寄存器（FC4 读 0~7）→ 线性换算 → 16 通道 DataPayload
5. 完整采集速率 ~4.9 Hz（固件串行节流，单请求 ~103ms，单次最多读 8 寄存器）
6. 量程表与换算公式 ✅ 已定案（用户提供设备固件量程表，2026-08-13 真机交叉验证，见 §Type Code 枚举）
7. 驱动含 deadline 失效连接 double 测试（ADR-009）

---

## Tech Stack

| 层 | 技术栈 | 版本 |
|---|---|---|
| Modbus 协议栈 | Go `net` 标准库（纯手写 MBAP + PDU） | Go 1.25 |
| shared SDK 驱动 | Go（`shared.local/device-sdk/go/daq/hardware/daq_t1602.go`） | Go 1.25 |
| wind-daq 适配器 | Go（thin wrapper，类型翻译） | Go 1.25 |
| 日志 | Go 标准库 `log/slog`（禁止第三方） | — |
| 前端 | Vue 3 + TypeScript + Naive UI | Vue 3.x / TS 5.x |
| 桌面壳 | Wails v3 | 现有 |
| 测试 | Go `testing` + fake conn double | 现有 |

---

## Commands

```powershell
# shared SDK 模块（Modbus 包 + T1602 驱动）
cd shared/device-sdk/go
go build ./...
go vet ./...
go test ./daq/hardware/... ./protocol/...

# wind-daq 后端
cd projects/wind-daq/services/api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...
go test ./internal/... ./api/...

# 前端
cd projects/wind-daq/apps/desktop-wails/frontend
npm run typecheck
npm run build

# Wails binding（若后端类型签名变化）
cd projects/wind-daq/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
```

---

## Protocol（实测校准）

> 本节全部来自真机实测（2026-08-12），取代文档中的待核实项。

### 寄存器映射

| 用途 | Modbus 功能码 | 寄存器类型 | 地址 | 每卡数量 | Slave ID | 卡 |
|---|---|---|---|---|---|---|
| 热电偶类型 | FC3 (0x03) | Holding | 200~207 | 8 | 1 | 卡1 CH0~7 |
| 热电偶类型 | FC3 | Holding | 200~207 | 8 | 2 | 卡2 CH0~7 |
| 采集数据 | FC4 (0x04) | Input | 0~7 | 8 | 1 | 卡1 CH0~7 |
| 采集数据 | FC4 | Input | 0~7 | 8 | 2 | 卡2 CH0~7 |

### 关键实测参数

| 项 | 实测值 |
|---|---|
| 端口 | **502**（文档原写 5000，实测 502） |
| 单次读上限 | **8 寄存器**（读 9+ 返回 Modbus 异常码 2，`0x018402`） |
| 单请求 RTT | 固定 **~103ms**（设备 100ms 响应周期，固件串行处理） |
| 双卡完整采集 | **~4.9 Hz**（2 请求/帧，理论下限 ~206ms/帧） |
| 请求并行 | 无效（多连接/流水线均串行排队，实测 s2 恒比 s1 晚 100ms） |
| 类型寄存器现值 | 16 通道全部 `0x0002`（T 型） |
| 数据通道现值 | 卡1 CH1 = ~1395（仅此通道接热电偶），其余 0 |

### 连接模型（设计决策）

- **单 TCP 连接 + Unit ID 复用**：Modbus TCP 的 Unit ID 字段本身就是为单连接寻址多从站设计的；实测已证明固件串行排队、多连接并行无效（s2 恒比 s1 晚 100ms），双连接不会更快，反而需要两套 ADR-009 watchdog/owner。因此驱动只建立**一条**到 `IP:502` 的连接，卡1/卡2 通过 Unit ID 1/2 区分。若未来需要按卡故障隔离再评估双连接。
- **Transaction ID**：请求侧自增（uint16 回绕），响应必须回显校验；ID 不匹配的响应丢弃并按超时处理（防止串帧）。
- **响应超时**：读/写请求超时固定 **1s**（远大于实测 103ms RTT，留足余量）；超时即判定连接损坏，走 ADR-009 owner `Close()` 路径，连接不可复用。
- **串行化**：同一连接上所有请求严格串行（单 in-flight），禁止流水线——与固件行为一致，也简化帧边界切割。

### Type Code 枚举

量程表为用户提供（2026-08-13），共 10 行，行号 = Type Code，8/9 行未使用：

| Type Code | 热电偶 | 量程 [min,max] ℃ |
|---|---|---|
| 0 | J | (-50, 50) |
| 1 | K | (0, 1200) |
| 2 | T | (0, 1300) |
| 3 | E | (-200, 400) |
| 4 | R | (0, 1000) |
| 5 | S | (0, 1700) |
| 6 | B | (0, 1768) |
| 7 | N | (0, 1800) |
| 8 | （未使用） | (-200, 800) |
| 9 | （未使用） | (-2000, 1000) |

> 注：行序与教科书标准量程不对应（如 code 3=E 配 (-200,400)），以用户提供的
> 设备固件量程表为准——真机交叉验证通过（2026-08-13：K 型 raw 1513 → 27.7℃，
> T 型配置 raw 1369 → 27.2℃，同一热电偶两种配置解出一致室温）。

### 换算公式

```
物理温度(℃) = raw / 65535 × (RangeMax - RangeMin) + RangeMin
```

raw 为 16 位无符号输入寄存器值。

> ✅ 公式已经用户确认并真机验证（2026-08-13），见 §Type Code 枚举 附注。

> **raw = 0 的语义（2026-08-13 用户确认）**：表示该通道**未接入热电偶**（开路输出 0），
> 不是"量程下限温度"。驱动输出 NaN → SSE/JSON 序列化为 `null` → UI 通道卡片显示
> `--`、波形图留空点、tooltip 显示 `-`。已知边界：T 型量程下限恰为 0℃，真实 0℃
> 测量与未接入不可区分（用户确认可接受）。

### Modbus 帧格式

- MBAP：事务 ID(2B) + 协议 ID(2B=0) + 长度(2B) + 单元 ID(1B)
- FC3 请求 PDU：`FC(1B) + 起始地址(2B) + 数量(2B)`
- FC4 请求 PDU：同上
- FC6 写单寄存器 PDU：`FC(1B=0x06) + 地址(2B) + 值(2B)`
- 响应：`FC + 字节数(1B) + 数据(N×2B)`；异常：`FC|0x80 + 异常码(1B)`
- **Max ADU**：读响应最长 8×2+9 = 25 字节；帧边界按长度字段 + 字节数精确切割

---

## Project Structure

```
shared/device-sdk/go/
├── protocol/modbus/            → Modbus TCP 协议栈（纯协议，零域逻辑）
│   ├── modbus.go               → Conn（帧编解码、FC3/4/6 读写、WatchdogClose）
│   ├── modbus_test.go          → fake conn 帧测试
│   └── MODBUS.md               → 协议说明（寄存器映射、实测参数）
├── daq/core/types.go           → +DaqT1602HardwareConfig 镜像 + DeviceDaqT1602
├── daq/ports/device.go         → +DaqT1602Configurable 接口
└── daq/hardware/
    ├── daq_t1602.go            → DAQT1602 驱动（单连接双 Unit ID、轮询采集）
    ├── daq_t1602_test.go       → 单元测试（fake conn + deadline double）
    └── daq_t1602_real_test.go  → 真机测试（`DAQ_T1602_REAL=1` 环境变量门控 + `t.Skip`，对齐 T1603 约定）

projects/wind-daq/services/api-go/
├── internal/core/device/types.go         → +DeviceDaqT1602 +DaqT1602HardwareConfig
├── internal/ports/device.go              → +DaqT1602Configurable
├── internal/adapters/hardware/t1602_adapter.go → T1602Adapter
├── internal/adapters/hardware/t1602_adapter_test.go
├── internal/adapters/config/default_profiles.go → T1602 默认 profile
├── internal/adapters/config/default_profiles_test.go
├── internal/usecase/device_manager.go    → +Get/ApplyDaqT1602Config +validate
├── internal/usecase/device_manager_test.go
├── pkg/appcontext/context.go             → 工厂 case
├── pkg/apiserver/apiserver.go            → 工厂 case
├── internal/bootstrap/bootstrap.go       → 工厂 case
└── api/server.go                         → /daqT1602Config 路由

projects/wind-daq/apps/desktop-wails/frontend/src/
├── api/types.ts                  → +'DAQ-T-1602' +DaqT1602HardwareConfig
├── components/device/DaqT1602Config.vue → 简化版 T1602 配置面板
├── components/device/DeviceManagementDrawer.vue → 类型注册 + 默认值 + 条件渲染
├── stores/deviceStore.ts         → 设备类型联合/分支同步 +'DAQ-T-1602'
└── stores/i18nStore.ts           → +dev_t1602_* 文案
```

> 注意：`DeviceManagementDrawer.vue` 已 2147 行，新增 T1602 注册时须跑
> `validate-frontend-structure.ps1 -CheckFileSize`，若触发文件大小门禁，
> 优先把 T1602 相关逻辑下沉到 `DaqT1602Config.vue` 而非继续堆积在 Drawer 中。

### 触点枚举（以 `DAQ-T-1603` 全量 grep 为准，逐一决策）

实现前/验收时对照下表，防止漏改导致类型联合不一致：

| 触点 | 决策 | 说明 |
|---|---|---|
| `internal/adapters/scan/parsers.go` + `network_scanner.go` | **不改** | T1603 发现走私有广播协议；T1602 是标准 Modbus:502，不会被误识别。**T1602 仅支持手动添加，不做自动发现**（如未来需要，单独立项做 Modbus 探测） |
| `pkg/types/types.go` | **extend** | 设备类型注册同步 `DAQ-T-1602` |
| `internal/adapters/config/file_profile_store.go` | **extend** | 新类型 profile 持久化/读取路径确认（含旧 profile 文件向后兼容） |
| `internal/adapters/hardware/sim/` | **不做** | 本期不为 T1602 提供仿真 producer；`SIMULATED` 设备维持现状 |
| `internal/adapters/hardware/simulated_scanner.go` | **不改** | 同上 |
| `frontend/utils/deviceCalibration.ts` | **不改** | 校零按 `CALIBRATABLE_DEVICE_TYPES` 白名单，T1602 默认不在其中即自动排除（与 T1603 一致）；仅需更新注释中的排除清单 |
| `tests/integration/server_test.go` | **extend** | 新增 `/daqT1602Config` PUT/GET 集成用例 |

### 前端集成约定

- **配置保存路径与 T1603 对齐**：`daqT1602Config` 内嵌在设备 profile draft 中，随 profile 一起保存；前端**不直接调用** `/daqT1602Config` REST 端点（该端点为 API 对称性保留，与 T1603 现状一致）。
- **Connect 回读同步**：连接成功后驱动读回 16 通道类型，经 `OnConfigSynced` 等价回调同步进 profile（仿 `t1603_adapter.go` 的 `OnConfigSynced` 链路）。
- ~~待校准显示~~：Q1 已定案（2026-08-13），"待校准"提示条与对应 i18n 键已移除。

---

## Code Style

适配器结构严格仿 `t1603_adapter.go`（编译期接口断言 + thin wrapper + 类型翻译函数）：

```go
// t1602_adapter.go
type T1602Adapter struct {
	mu      sync.RWMutex
	driver  *sharedhw.DAQT1602
	profile device.Profile
	config  device.DaqT1602HardwareConfig
	sink    device.DataSink
	onError func(err error)
}

var _ ports.Device = (*T1602Adapter)(nil)
var _ ports.DaqT1602Configurable = (*T1602Adapter)(nil)
var _ ports.ErrorNotifiable = (*T1602Adapter)(nil)

func NewT1602Adapter(profile device.Profile) *T1602Adapter {
	return &T1602Adapter{
		profile: profile,
		config:  profile.DaqT1602Config,
	}
}
```

驱动采样循环（单连接、双 Unit ID 串行轮询，与实测节流匹配）：

```go
// daq_t1602.go 采集循环核心
for {
	select {
	case <-d.stop:
		return nil
	default:
	}
	card1, err := d.mb.ReadInputRegisters(unitIDCard1, 0, 8) // ~103ms
	if err != nil { return err }
	card2, err := d.mb.ReadInputRegisters(unitIDCard2, 0, 8) // ~103ms
	if err != nil { return err }
	raw := append(card1, card2...)
	d.emit(convertToTemp(raw, d.ranges))
}
```

---

## Testing Strategy

| 层 | 框架 | 覆盖 |
|---|---|---|
| Modbus 包 | Go testing + fake net.Conn | MBAP 编解码、帧边界切割、FC3/4/6、异常码、Transaction ID 不匹配丢弃、响应超时、deadline 失效 double |
| T1602 驱动 | Go testing + fake conn | Connect（Unit ID 1/2 类型回读）、Start/Stop、类型读写、数据换算、ADR-009 watchdog、deadline 失效 double |
| T1602 驱动真机 | 环境变量门控 `DAQ_T1602_REAL=1` + `t.Skip`（对齐 T1603 约定） | 连真实设备 192.168.3.201，读类型 + 轮询数据 |
| T1602Adapter | Go testing | 编译期接口断言、配置翻译、未连接路径 |
| Usecase | Go testing + fake store | 配置持久化、校验、错误路径 |
| 集成 | `tests/integration/server_test.go` | PUT/GET `/api/device/{id}/daqT1602Config` |
| 前端 | `npm run typecheck` | 类型联合 + 面板组件类型安全 |

**ADR-009 硬性要求**：驱动所有命令路径必须 owner 可直接 `conn.Close()` 取消；测试必须含"忽略 deadline、只在 Close 后返回"的连接 double。

---

## Boundaries

- **Always**:
  - `core/` 零硬件 import；`ports/` 零实现；`usecase/` 走 ports
  - 每个命令路径带独立 owner 的 `conn.Close()` 取消机制（Windows 网络约束）
  - 文件 ≤500 行、函数 ≤50 行（`validate-structure.ps1`）
  - 驱动层只输出工程值 ℃，不做单位换算
- **Ask first**:
  - 在 T1603 已有类型/结构中复用字段（禁止，必须隔离）
  - 引入 Modbus 第三方库（本项目决定手写，不引入）
  - 前端复用 T1603 组件而非新建 `DaqT1602Config.vue`
  - 修改 `project.nsi` 或其他共享配置
- **Never**:
  - 提交 secrets；删除失败测试；编辑 vendor 目录
  - 在适配器中写域逻辑；在 `core/` 中 import 硬件
  - 在 T1602 代码中出现 `T1603`/`P1603` 命名泄漏

---

## Success Criteria

1. `go build ./...` + `go vet ./...` + `go test ./daq/hardware/... ./protocol/...` 全部通过（shared SDK）
2. `go build -buildvcs=false ./...` + `gofmt -l .` 无输出 + `go vet ./...` + `go test ./internal/... ./api/...` 全部通过（wind-daq）
3. `npm run typecheck` + `npm run build` 通过（前端）
4. 真机测试：连 192.168.3.201:502，读回 16 通道类型（当前全 T 型），轮询 16 通道数据
5. 数据换算：✅ 已验证（K 型 raw 1513 → 27.7℃ 与室温吻合，2026-08-13）
6. §触点枚举 表中所有 extend 项全部落地，不改项已验证无影响；三处工厂 + API 路由 + 前端类型联合全部同步 `DAQ-T-1602`
7. ADR-009 deadline 失效 double 测试存在且通过

---

## Open Questions

- ~~Q1（换算公式/量程映射矛盾）~~ **已解决（2026-08-13）**：公式经用户确认（`raw/65535×(max-min)+min`），量程表用户提供（设备固件 10 行表，见 §Type Code 枚举）。真机验证：K 型 raw 1513 → 27.7℃、T 型配置 raw 1369 → 27.2℃（同一热电偶交叉一致）。旧矛盾根因：先前内置的是教科书标准量程，与设备固件实际量程不符。另确认 **raw=0 表示通道未接入热电偶**（开路输出），驱动输出 NaN、UI 显示 "--"，见 §换算公式。
- ~~Q2（N 型量程）~~ **已解决（2026-08-13）**：用户量程表第 7 行 N 型 = (0, 1800)。
- **Q3（类型写回）**：✅ **已验证（2026-08-13）**：FC6 运行时改写 Holding 200~207 成功，回显一致、读回校验通过、raw 随类型变化（T→1369 / K→1513），确认类型码参与设备内部换算。测试后已恢复原值。
- **Q4（单位）**：温度单位固定 ℃，是否需要 UI 切换（℉）？先固定 ℃。
- **Q5（通道掩码/采样率）**：T1602 文档无采样率/通道掩码/触发概念（固件固定 ~100ms 周期），确认不需要 T1603 那些配置项。
