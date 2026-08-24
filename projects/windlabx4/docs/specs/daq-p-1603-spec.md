# Spec: WindLabX4 新增 DAQ-P-1603 压力采集设备

## Objective

在 `projects/WindLabX4` 多设备 DAQ 应用中新增 **DAQ-P-1603** 压力采集设备支持，使其与现有 DAQ-P-1604 / DAQ-T-1603 / DSA3217 / WTN_PXI 并列，可在统一 UI 中连接、配置、采集、录制。

**用户故事**：
- 作为风洞测试工程师，我希望在 WindLabX4 中连接 DAQ-P-1603 设备，配置 16 通道压力采集参数，实时查看压力曲线，并将数据录制为 CSV。
- 作为应用维护者，我希望 DAQ-P-1603 适配器代码复用 shared/device-sdk 抽象层，与其他 DAQ 设备保持一致的接口契约。

**成功画面**：
- 在设备管理抽屉中手动输入 DAQ-P-1603 的 IP 地址（不支持自动扫描，与 WTNDAQ16H SDK 一致）
- 选中设备后点击"连接"，UI 显示 Connected 状态、设备序列号、固件版本
- 配置面板可调整采样率（≤500Hz）、量程（±10V/±5V/±1V/0-20mA），并为每个通道选择传感器类型（压力/温度）与单位
- 点击"开始采集"，16 通道实时曲线刷新（≥25Hz），数据落盘到 CSV
- 点击"停止采集"后采集线程优雅退出，CSV 文件完整关闭
- 拔网线 → 5 秒内 UI 显示 Disconnected，重新插回 → 自动重连

**设备特点（关键）**：
- 16 通道通用 AI，每通道物理接口可接入压力传感器或温度传感器（由用户在 UI 配置）
- 通道传感器类型决定单位与换算逻辑：压力通道用 Pa/kPa/MPa/mmH2O，温度通道用 ℃/℉
- CSV 表头按通道类型动态生成（如 `Timestamp, CH01_Pa, CH02_Pa, CH03_℃`）
- 采样率上限 500Hz（与 WTNDAQ16H SDK 头文件声明的 500kSPS 不同，DAQ-P-1603 为低速定制版本，需 HIL 实测验证）

## Tech Stack

| 层 | 技术 | 版本/路径 |
|---|---|---|
| 后端语言 | Go | 1.25（go.work 主干） |
| 桌面壳 | Wails v3 | alpha.95 |
| 前端 | Vue 3 + TypeScript + Vite + Naive UI + Tailwind | 与 WindLabX4 现有一致 |
| FFI | syscall + DLL | `WTNDAQ16H_64.dll`（仅 Windows） |
| 协议参考 | ART WTNDAQ16H SDK | `C:\ART\WTNDAQ16H\Samples\VC\WTNDAQ16H.H` |
| 共享 SDK | `shared.local/device-sdk/go` | 现有模块，go.work + replace 双机制 |
| 日志 | 标准库 `log/slog` | 工作区硬约束（禁第三方日志库） |
| 测试 | Go 标准 `testing` + 项目内 `testing/sim` 模拟器 | — |

## Commands

```powershell
# 工作区根目录执行
$env:GOWORK="off"   # 仅 daq-t1603 排除场景需要；WindLabX4 默认走 go.work

# 1. shared SDK 编译验证（添加 FFI 后）
cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\device-sdk\go
go build ./...
go vet ./...
go test ./...

# 2. WindLabX4 后端编译验证
cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\WindLabX4\services\api-go
go build ./...
go vet ./...
go test ./...

# 3. WindLabX4 Wails 应用构建
cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\WindLabX4\apps\desktop-wails
go build ./...
go vet ./...

# 4. 前端类型检查 + 构建
cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\projects\WindLabX4\apps\desktop-wails\frontend
npm run typecheck
npm run build

# 5. Wails 绑定同步（后端方法签名变更时强制）
wails3 generate bindings -silent

# 6. 结构验证（工作区根）
cd c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace
.\scripts\validate-structure.ps1
.\scripts\validate-frontend-structure.ps1 -CheckFileSize

# 7. 生产构建
$env:GOWORK="off"
wails3 build -tags production
```

## Project Structure

新增/修改文件清单（按层次从下到上）：

```
shared/device-sdk/go/
├── ffi/
│   ├── wtn_daq16h.go          【新增】WTNDAQ16H_64.dll FFI 封装（//go:build windows）
│   └── wtn_daq16h_stub.go     【新增】非 Windows 平台 stub（编译占位）
├── daq/
│   ├── core/
│   │   └── types.go           【修改】新增 DeviceDAQP1603 = "DAQ-P-1603" 常量
│   └── hardware/
│       └── daq_p1603.go       【新增】完整 DAQ-P-1603 适配器（参考 daq_t1603.go 模式）

projects/windlabx4/services/api-go/internal/
├── core/device/
│   └── types.go               【修改】新增 DeviceDAQP1603 Type 常量
├── adapters/hardware/
│   └── daq_p1603.go           【新增】thin wrapper，桥接 shared SDK 与 WindLabX4 ports
└── bootstrap/
    └── bootstrap.go           【修改】deviceFactory.Create switch 新增 case

projects/windlabx4/apps/desktop-wails/
├── config/device-profiles.json【修改】新增 DAQ-P-1603 默认 profile 条目
└── frontend/src/
    └── components/device/
        └── DaqP1603Config.vue 【新增】DAQ-P-1603 专属配置面板（参考 DaqT1603Config.vue）

projects/windlabx4/docs/specs/
└── daq-p-1603-spec.md         【本文件】
└── daq-p-1603-plan.md         【后续】Phase 2 plan
└── daq-p-1603-tasks.md        【后续】Phase 3 tasks
```

**目录职责定义**（沿用六边形架构约束）：

| 目录 | 职责 | 硬约束 |
|---|---|---|
| `shared/device-sdk/go/ffi/` | Windows DLL 调用封装 | 仅 syscall + unsafe，零业务逻辑 |
| `shared/device-sdk/go/daq/core/` | 领域类型 | 零外部依赖、零硬件导入 |
| `shared/device-sdk/go/daq/hardware/` | 完整设备驱动 | 调用 ffi 包，实现 ports.Device |
| `projects/windlabx4/.../adapters/hardware/` | thin wrapper | 仅做接口桥接，零硬件直调 |
| `projects/windlabx4/.../bootstrap/` | 装配根 | 工厂 switch，零业务逻辑 |
| `projects/windlabx4/.../frontend/` | UI | 零硬件访问，零采集算法 |

## Code Style

### Go 适配器骨架（参考 [daq_t1603.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/daq/hardware/daq_t1603.go) 模式）

```go
//go:build windows

package hardware

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"

    "shared.local/device-sdk/go/daq/core"
    "shared.local/device-sdk/go/ffi"
    "shared.local/device-sdk/go/protocol"
)

const (
    // DAQ-P-1603 协议常量（基于 WTNDAQ16H SDK 头文件 + 用户决策 D-1/D-2）
    daqP1603MaxChannels       = 16        // 16 路通用 AI 通道（每通道可配置为压力或温度传感器）
    daqP1603MaxSampleRate     = 500.0     // 采样率上限 500Hz（用户决策 D-2；与 WTNDAQ16H 头文件 500kSPS 不同，DAQ-P-1603 为低速定制版本）
    daqP1603DefaultSampleRate = 100.0     // 默认 100Hz（保守值，HIL 时可调整）
    daqP1603ReadTimeoutSec    = 10.0      // ReadBinary 单次超时（参考示例 Sys.cpp L14）
    daqP1603ConnectTimeout    = 5 * time.Second
    daqP1603JoinLoopTimeout   = 1 * time.Second  // 与 shared SDK ReadLoopJoinTimeout 对齐
)

// ChannelSensorType 通道传感器类型枚举（DAQ-P-1603 关键特性：每通道可接压力或温度传感器）
type ChannelSensorType string

const (
    SensorPressure ChannelSensorType = "pressure"  // 压力传感器
    SensorTemperature ChannelSensorType = "temperature"  // 温度传感器
)

// DAQP1603 实现 shared SDK 的 ports.Device 接口。
// 完整驱动封装 WTNDAQ16H_64.dll 调用，对外暴露 Connect/StartAcquisition/StopAcquisition 等方法。
type DAQP1603 struct {
    profile core.Profile
    mu      sync.Mutex
    state   core.Connection
    handle  uintptr  // WTNDAQ16H_DEV_Create 返回的 HANDLE
    sink    core.DataSink

    stopReason *protocol.StopReasonTracker  // 复用 shared SDK 协议原语
    cancelRead context.CancelFunc
    readDone   chan struct{}
    log        *slog.Logger
}

// NewDAQP1603 构造 DAQ-P-1603 适配器实例。
func NewDAQP1603(profile core.Profile) *DAQP1603 {
    return &DAQP1603{
        profile:     profile,
        state:       core.StatusDisconnected,
        stopReason:  protocol.NewStopReasonTracker(),
        log:         slog.With("device", "DAQ-P-1603", "id", profile.ID),
    }
}

// Connect 实现 ports.Device.Connect。
// 调用 WTNDAQ16H_DEV_Create 建立 TCP 连接，并同步硬件参数。
func (d *DAQP1603) Connect() error {
    d.mu.Lock()
    defer d.mu.Unlock()

    if d.state != core.StatusDisconnected {
        return fmt.Errorf("device %s not in disconnected state: %s", d.profile.ID, d.state)
    }
    // 1. 确保 DLL 已加载（ffi.Init 内部 sync.Once 保证幂等）
    if err := ffi.InitWTNDAQ16H("WTNDAQ16H_64.dll"); err != nil {
        return fmt.Errorf("load WTNDAQ16H DLL: %w", err)
    }
    // 2. 调用 WTNDAQ16H_DEV_CreateA(IP, sendTO, recvTO)
    handle, err := ffi.WTNDAQ16HDevCreate(d.profile.Address, 500, 500)
    if err != nil {
        return fmt.Errorf("DEV_Create %s: %w", d.profile.Address, err)
    }
    d.handle = handle
    // 3. 同步硬件参数（采样率、量程、触发配置）
    if err := d.syncHardwareParams(); err != nil {
        _ = ffi.WTNDAQ16HDevRelease(handle)
        d.handle = 0
        return fmt.Errorf("sync params: %w", err)
    }
    d.state = core.StatusConnected
    d.log.Info("DAQ-P-1603 connected", "address", d.profile.Address)
    return nil
}

// StartAcquisition 实现 ports.Device.StartAcquisition。
// 调用 AI_InitTask → AI_StartTask → AI_SendSoftTrig，启动后台 readLoop。
func (d *DAQP1603) StartAcquisition() error {
    /* 详细实现见 tasks 阶段 */
    return nil
}

// StopAcquisition 实现 ports.Device.StopAcquisition。
// 调用 AI_StopTask → AI_ReleaseTask，等待 readLoop 优雅退出。
func (d *DAQP1603) StopAcquisition() error {
    /* 详细实现见 tasks 阶段 */
    return nil
}

// Disconnect 实现 ports.Device.Disconnect，调用 DEV_Release。
func (d *DAQP1603) Disconnect() error {
    /* 详细实现见 tasks 阶段 */
    return nil
}

// compile-time interface check
var _ core.Device = (*DAQP1603)(nil)
```

### TypeScript 组件骨架（参考 DaqT1603Config.vue）

```vue
<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { DaqP1603Config } from '@/shared/types/devices'

const props = defineProps<{
  modelValue: DaqP1603Config
  disabled?: boolean
}>()
const emit = defineEmits<{
  'update:modelValue': [value: DaqP1603Config]
}>()

const config = reactive({ ...props.modelValue })
watch(config, (val) => emit('update:modelValue', { ...val }), { deep: true })
</script>

<template>
  <div class="daq-p1603-config space-y-4">
    <!-- 采样率（≤500Hz） -->
    <!-- 量程选择（±10V/±5V/±1V/0-20mA） -->
    <!-- 通道传感器类型表（每行：通道号 + 传感器类型下拉 + 单位下拉 + 启用开关） -->
    <!--   - 传感器类型：压力(Pa/kPa/MPa/mmH2O) / 温度(℃/℉) -->
    <!--   - 单位选项随传感器类型动态切换 -->
    <!-- 通道精度（0~6 位小数） -->
    <!-- 硬件时间戳开关 -->
  </div>
</template>
```

### 命名约定

- Go 类型/函数：`PascalCase`（如 `NewDAQP1603`、`WTNDAQ16HDevCreate`）
- Go 常量：`PascalCase`（如 `daqP1603MaxChannels`，私有前缀小写）
- Vue 组件：`PascalCase.vue`（如 `DaqP1603Config.vue`）
- TS 类型：`PascalCase`，接口不加 `I` 前缀
- 事件名：`kebab-case` 且带项目前缀（如 `daq:device-state`）
- 中文注释必须解释"为什么"，不解释"是什么"

## Testing Strategy

### 测试金字塔

| 层 | 框架 | 位置 | 覆盖目标 |
|---|---|---|---|
| FFI 单元测试 | Go `testing` | `shared/device-sdk/go/ffi/wtn_daq16h_test.go` | DLL 加载失败处理、参数边界（非 Windows 跳过） |
| 适配器单元测试 | Go `testing` | `shared/device-sdk/go/daq/hardware/daq_p1603_test.go` | 状态机迁移、错误传播、并发安全 |
| 集成测试（模拟器） | Go `testing` + `testing/sim` | 同上 | 使用 mock DLL（接口注入），覆盖 Connect→Start→Read→Stop→Disconnect 全链路 |
| 前端组件测试 | （WindLabX4 现有项目无单测框架，省略） | — | — |
| 手工 HIL 测试 | 人工执行 | `projects/windlabx4/docs/runbooks/hil-validation-plan.md` | 真机连接、采集、断网恢复 |

### 测试用例三段式格式（project_memory §20 约束）

每条用例必须包含三段：
- **测试前置**：环境、设备状态、初始配置
- **测试步骤**：可观察、可操作的动作
- **期待结果**：可验证的状态或输出

### 关键测试覆盖点

1. **FFI 层**：DLL 缺失时 `Init` 返回清晰错误；非 Windows 平台 stub 编译通过
2. **状态机**：Disconnected → Connected → Acquiring → Connected → Disconnected 全路径
3. **并发安全**：StartAcquisition 与 StopAcquisition 并发调用不 panic
4. **断连检测**：readLoop 检测到 DLL 返回 FALSE 时正确触发 `handleConnectionLost`
5. **资源清理**：Disconnect 后 handle 归零，重复 Disconnect 安全
6. **采集数据格式**：每帧 `DataPayload.Channels` 长度 = 16，时间戳单调递增
7. **通道传感器类型**：profile 中每通道的 SensorType 字段正确驱动 CSV 表头生成与单位换算
8. **CSV 动态表头**：16 通道混合压力/温度时，表头按通道类型生成（如 `Timestamp, CH01_Pa, CH02_℃, ...`），数据行单位与表头一致
9. **采样率上限**：用户配置 >500Hz 时前端校验拦截，后端 ApplyConfig 返回错误

### 覆盖率要求

- shared SDK 新增代码：≥ 70%（与 daq_t1603.go 现有水平对齐）
- WindLabX4 thin wrapper：≥ 60%（thin wrapper 本身逻辑少）

## Boundaries

### Always do

- 提交前运行 `go build && go vet && go test ./...` 与 `npm run typecheck && npm run build`
- FFI 调用必须包裹 `//go:build windows`，配套 stub 文件保证跨平台编译
- DLL 句柄（HANDLE）封装为 `uintptr`，不暴露给上层
- 所有公开接口加中文注释解释"为什么"
- 状态变更通过 `core.Connection` 枚举，不使用字符串字面量
- 实现编译期接口断言：`var _ ports.Device = (*DAQP1603)(nil)`

### Ask first

- 修改 `shared/device-sdk/go/daq/core/types.go` 的现有类型（新增字段需评估对 daq-p1604 项目的影响）
- 修改 `WTNDAQ16H.H` 头文件对应常量值（可能影响实机行为）
- 修改 `bootstrap.go` 已有 case 分支顺序
- 新增第三方 Go 依赖
- 修改前端路由表或 Pinia store 结构

### Never do

- 在 `core/` 目录 import `ffi/` 或任何硬件相关包
- 在 `frontend/` 调用任何硬件 DLL 或采集算法
- 在 `programs/` CLI 工具 import 任何项目 `internal/*` 包
- 提交 `WTNDAQ16H_64.dll` 二进制到版本控制（应通过 release 包分发）
- 移除现有失败测试以让 CI 通过
- 在 `//go:build !windows` 的 stub 文件中调用任何 syscall

## Success Criteria

| # | 条件 | 验证方式 |
|---|---|---|
| 1 | WindLabX4 后端 `go build ./... && go vet ./... && go test ./...` 全绿 | 命令行执行 |
| 2 | shared SDK `go build ./... && go vet ./... && go test ./...` 全绿 | 命令行执行 |
| 3 | 前端 `npm run typecheck && npm run build` 全绿 | 命令行执行 |
| 4 | `validate-structure.ps1` 与 `validate-frontend-structure.ps1 -CheckFileSize` 全绿 | 命令行执行 |
| 5 | 设备管理抽屉手动输入 IP 后能连接 DAQ-P-1603 设备（profile.Type = "DAQ-P-1603"，不支持自动扫描） | 手工 HIL |
| 6 | 点击连接后 5 秒内 UI 显示 Connected 状态，并展示设备序列号、固件版本 | 手工 HIL |
| 7 | 配置面板可调整采样率（≤500Hz）、量程、每通道传感器类型与单位，ApplyConfig 后实机参数同步 | 手工 HIL |
| 8 | 开始采集后 16 通道实时曲线刷新 ≥ 25Hz，CSV 文件表头按通道类型动态生成（如 `Timestamp, CH01_Pa, CH02_℃, ...`） | 手工 HIL + 文件检查 |
| 9 | 停止采集后 readLoop 在 1 秒内退出，CSV 文件完整关闭（无截断） | 手工 HIL + 文件检查 |
| 10 | 拔网线 → 5 秒内 UI 显示 Disconnected；重新插回 → 自动重连到 Connected | 手工 HIL |
| 11 | DAQ-P-1603 与 DAQ-P-1604 可同时连接、并行采集，互不阻塞 | 手工 HIL |
| 12 | Wails 绑定同步：后端方法签名变更后执行 `wails3 generate bindings -silent`，前端类型完整 | 命令行执行 |
| 13 | 采样率 >500Hz 时前端校验拦截，后端 ApplyConfig 返回错误（不发送到硬件） | 手工 HIL |

## Open Questions

### 已决策（用户 2026-07-06 回复）

| # | 问题 | 决策 |
|---|---|---|
| Q2 | 设备发现方式 | **手动输入 IP，不支持扫描**（与 WTNDAQ16H SDK 一致，参考代码无扫描 API） |
| Q5 | 通道数 | **16 通道** |
| Q6 | 采样率上限 | **500Hz**（注意：与 WTNDAQ16H 头文件 500kSPS 不同，DAQ-P-1603 为低速定制版本） |
| Q7 | CSV 表头 | **Timestamp + 16 通道 = 17 列**（不含大气压/大气温度），表头按通道传感器类型动态生成 |

### 新增关键特性（用户 2026-07-06 反馈）

- **通道传感器类型动态化**：每通道可配置为压力或温度传感器，由 UI 选择。影响范围：
  - `core.ChannelConfig` 新增 `SensorType` 字段（`"pressure"` / `"temperature"`）
  - CSV 表头按通道类型生成（如 `CH01_Pa, CH02_℃`）
  - 单位选项随传感器类型切换（压力：Pa/kPa/MPa/mmH2O；温度：℃/℉）
  - 单位换算逻辑按通道类型分支

### 待 HIL 验证

1. **DLL 部署路径**：`WTNDAQ16H_64.dll` 放在 `projects/windlabx4/apps/desktop-wails/` 根目录还是 `third_party/` 子目录？是否需要 go:embed？（建议：build 时复制到可执行文件同目录，不 embed）
2. **单位换算（已解决 2026-08-24，详见 [postmortem](../postmortems/2026-08-24-daq-p1603-current-scaling.md)）**：DAQ-P-1603 输出原始 ADC 码值（U16）。0-20mA 量程实为**双极性 ±20mA**，零点（0mA）对应码值 32768。换算公式：
   ```
   current = (u16Code - 32768) * CodeWidth - OffsetVolt   // CodeWidth/OffsetVolt 来自 GetVoltRangeInfo
   ```
   厂商 `ScaleBinToVolt` 为电压模式设计，电流模式下会崩溃，不能调用；应通过 `GetVoltRangeInfo` 拿权威参数后在 Go 端自算。温度通道与压力通道换算公式一致（区别在量程 RangeMin/Max），无需独立校准系数表。
3. **触发模式**：示例代码同时启用了数字触发（DIO1 下降沿）和模拟触发。WindLabX4 默认场景是否需要触发？还是默认软件触发、UI 暴露触发配置？
4. **500Hz 上限来源**：用户决策 D-2 为 500Hz，但 WTNDAQ16H SDK 头文件声明 500kSPS。需 HIL 确认：
   - 是 DAQ-P-1603 固件限制？
   - 还是 SDK 内部降采样？
   - 还是用户场景需求（500Hz 已足够）？
   - 实测采样率 >500Hz 时 SDK 行为（拒绝/降速/无效）
5. **前端配置组件复用**：DaqP1603Config.vue 与 DaqP1604Config.vue 是否需要抽公共子组件（如 SampRateInput、RangeSelector、UnitSelector）？还是各自独立实现？
