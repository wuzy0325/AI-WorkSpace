# Spec: MPS-03 多功能探针设备集成到 WindLabX4

> **协议规范来源**：`device-lab/skills/mps03/SKILL.md` + `device-lab/drivers/MPS03/MPS03-协议.docx`
> **参考实现**：`Cursor DAQ` 项目（TypeScript 版本，已完成实测验证）
> **协议实测修正版**：`Cursor DAQ/docs/protocols/MPS03-protocol.md`（端口 9000，非文档标注的 900）
>
> **Review 修订记录**：本 spec 已根据首轮 review 修订，解决命名冲突、依赖方向、命令分帧、CHA mask、错误帧、CSV 列规范、API 契约、重连状态机、验证命令覆盖度等问题。

---

## Objective

将 MPS-03 多功能探针设备集成到 WindLabX4 系统，作为新的 DAQ 设备类型。MPS-03 是基于 TCP/IP 的风洞测量数据采集设备（**非运动控制器**），输出 16 通道异构气动数据：攻角 α、侧滑角 β、马赫数 Ma、速度 V、三向速度 Vx/Vy/Vz、6 路传感器 S1-S6、总压 P_total、内外温度 T_ext/T_int。

### 与现有 DAQ-P-1603 的身份隔离（Critical）

WindLabX4 已存在 **DAQ-P-1603**（`Type = "DAQ-P-1603"`，16 通道通用 AI 采集，DLL FFI 驱动）。MPS-03 与 DAQ-P-1603 协议、传输方式、数据语义完全不同，必须严格隔离：

| 维度 | DAQ-P-1603（现有） | MPS-03（新增） |
|---|---|---|
| `device.Type` 字面量 | `"DAQ-P-1603"` | `"MPS03"`（**无连字符**，避免与 `DAQ-P-1603` 视觉混淆） |
| 显示名 | DAQ-P-1603 | MPS-03 |
| 传输 | DLL FFI（WTNDAQ16H_64.dll） | 裸 TCP + ASCII 文本 |
| 通道语义 | 同构（每通道可接压力或温度，由 `SensorType` 区分） | 异构（每通道含义固定：角度/速度/马赫数/压力/温度/裸传感器） |
| 配置接口 | `DAQP1603Configurable` | `MPS03Configurable`（**独立接口，禁止复用**） |
| 适配器文件 | `daq_p1603_adapter.go` | `mps03_adapter.go`（**独立文件**） |
| shared SDK 驱动 | `daq/hardware/daq_p1603.go` | `daq/hardware/mps03.go`（**独立文件**） |
| 模拟器 | 无独立模拟器 | `testing/sim/mps03_sim.go` |
| 前端配置组件 | 复用通用通道配置 | `Mps03ConfigPanel.vue`（**独立组件**） |
| CSV 表头分支 | `case device.DeviceDAQP1603:` | `case device.DeviceMPS03:`（**独立分支**） |

**命名红线**：禁止在 MPS-03 代码中使用 `DAQP1603`、`P1603`、`daq_p1603` 等字样，反之亦然。

### 用户故事

作为风洞实验人员，我希望在 WindLabX4 中：
1. 添加 MPS-03 设备，配置 IP（默认 192.168.1.9）和端口（默认 9000）
2. 连接后自动读取设备硬件配置（9 项 #GET 参数）并在 UI 只读展示
3. **停止采集状态下**通过 UI 修改核心硬件配置（AVG/DELAY/CHA/TMODE/TTYPE/TCHO）并 #SAVE 持久化
4. 启动采集后实时接收 16 通道 CSV 数据流，显示在波形图
5. 录制 CSV 文件，表头为 17 列（Timestamp + 16 通道 ASCII 列名 + 单位后缀）
6. 断连后按状态机自动重连，重连后恢复采集

### 成功标准

1. MPS-03 可作为设备类型添加到 WindLabX4，前后端 `Type = "MPS03"` 一致，与 DAQ-P-1603 完全隔离
2. 连接 → 自动 #GET 全部 9 项硬件配置 → UI 只读显示当前值
3. 启动采集 → 发送 `#SET BIN 0` + `#START` → 接收 CSV 数据流 → 解析为 16 通道 DataPayload
4. 停止采集 → 发送 `#STOP` → 立即停止数据推送（无响应帧，发送后即切换状态）
5. **采集状态下调用 ApplyMps03Config 返回 409 Conflict**；停止状态下修改配置 → 发送对应 `#SET` → 自动 `#SAVE` 持久化
6. CSV 录制时 MPS-03 走专属表头分支（17 列：Timestamp + 16 通道 ASCII 列名 + 单位后缀）
7. 模拟器可独立运行，前端开发无需真实硬件
8. 断连按状态机自动重连（指数退避，最多 5 次），重连后恢复采集状态
9. 解析错误的 CSV 帧被丢弃并计入错误计数，**禁止填 0 进入 DataSink**
10. 连续解析错误超过阈值（默认 50 帧）触发连接异常，进入 Error 态

---

## Tech Stack

| 层 | 技术栈 | 版本 |
|---|---|---|
| shared SDK 驱动 | Go + `net` 标准库（TCP socket） | Go 1.25 |
| WindLabX4 适配器 | Go（thin wrapper，类型翻译） | Go 1.25 |
| 日志 | Go 标准库 `log/slog`（禁止第三方） | — |
| 前端 | Vue 3 + TypeScript + Naive UI | Vue 3.x / TS 5.x |
| 桌面壳 | Wails v3 | alpha.95 |
| 架构 | 六边形架构（core/ports/usecase/adapters） | — |
| 模拟器 | `shared/device-sdk/go/testing/sim/` 框架 | 现有 |

---

## Commands

```powershell
# shared SDK 模块（独立验证）
cd shared/device-sdk/go
go build ./...                              # 编译
go test ./...                               # 全部测试
go vet ./...                                # 静态检查

# WindLabX4 后端模块
cd projects/windlabx4/services/api-go
go build ./...                              # 编译
go test ./...                               # 全部测试
go test ./internal/adapters/hardware/... -v # 仅硬件适配器测试
go vet ./...                                # 静态检查

# 前端
cd projects/windlabx4/apps/desktop-wails/frontend
npm run typecheck                           # TS 类型检查
npm run build                               # 生产构建

# Wails binding 同步（修改后端 binding 签名后必须执行）
cd projects/windlabx4/apps/desktop-wails
wails3 generate bindings -silent

# 结构验证（提交前）
.\validate-structure.ps1                    # 后端六边形约束
.\validate-frontend-structure.ps1 -CheckFileSize  # 前端约束

# MPS-03 模拟器独立测试
cd shared/device-sdk/go/testing/sim
go test ./... -run MPS03 -v

# 跨模块集成测试（WindLabX4 API + shared SDK 模拟器）
cd projects/windlabx4/services/api-go
go test ./internal/adapters/hardware/... -run MPS03 -v
```

---

## Project Structure

### 依赖方向（Critical）

```
shared/device-sdk/go/                    ← 共享 SDK，禁止导入 projects/*
  ├── daq/core/types.go                  ← shared SDK 自有领域类型
  ├── daq/ports/device.go                ← shared SDK 自有接口
  ├── protocol/mps03_frame.go            ← 协议层（纯协议，无设备状态）
  ├── daq/hardware/mps03.go              ← 真实驱动（依赖 daq/core + daq/ports + protocol）
  └── testing/sim/mps03_sim.go           ← 模拟器

projects/windlabx4/services/api-go/internal/
  ├── core/device/types.go               ← WindLabX4 自有领域类型
  ├── ports/device.go                    ← WindLabX4 自有接口
  ├── adapters/hardware/mps03_adapter.go ← thin wrapper（翻译 shared ↔ WindLabX4 类型）
  └── ...                                ← 其他 WindLabX4 内部代码
```

**硬约束**：
- `shared/device-sdk/go/*` 禁止 `import "windlabx4/services/api-go/internal/*"`
- `shared/device-sdk/go/*` 只能依赖 `shared.local/device-sdk/go/*` 内部包
- WindLabX4 adapter 双向依赖：`WindLabX4 internal/*` + `shared.local/device-sdk/go/*`

### 新增文件

| # | 路径 | 说明 |
|---|---|---|
| 1 | `shared/device-sdk/go/protocol/mps03_frame.go` | MPS-03 协议层：命令超时收发 + CSV 行解析 + TMODE/TCHO 转换 + 错误帧识别 |
| 2 | `shared/device-sdk/go/protocol/mps03_frame_test.go` | 协议层单元测试 |
| 3 | `shared/device-sdk/go/daq/hardware/mps03.go` | MPS-03 真实驱动：TCP 连接 + 采集循环 + 配置同步 + 重连状态机 |
| 4 | `shared/device-sdk/go/daq/hardware/mps03_test.go` | 驱动单元测试（用 mock TCP server） |
| 5 | `shared/device-sdk/go/testing/sim/mps03_sim.go` | MPS-03 模拟器（#GET 响应 + CSV 数据流） |
| 6 | `shared/device-sdk/go/testing/sim/mps03_sim_test.go` | 模拟器测试 |
| 7 | `shared/device-sdk/docs/commands/mps03.md` | 命令规范文档（参照 `COMMAND-SPEC-TEMPLATE.md`） |
| 8 | `projects/windlabx4/services/api-go/internal/adapters/hardware/mps03_adapter.go` | WindLabX4 thin wrapper（类型翻译） |
| 9 | `projects/windlabx4/apps/desktop-wails/frontend/src/components/device/Mps03ConfigPanel.vue` | MPS-03 配置面板组件 |

### 修改文件

| # | 路径 | 改动 |
|---|---|---|
| 10 | `shared/device-sdk/go/daq/core/types.go` | 新增 `DeviceMPS03 Type = "MPS03"` + `MPS03HardwareConfig` 结构体 + `Profile.MPS03Config` 字段 |
| 11 | `shared/device-sdk/go/daq/ports/device.go` | 新增 `MPS03Configurable` 接口 |
| 12 | `projects/windlabx4/services/api-go/internal/core/device/types.go` | 新增 `DeviceMPS03 Type = "MPS03"` + `MPS03HardwareConfig` + `Profile.MPS03Config`（WindLabX4 自有类型，与 shared SDK 镜像） |
| 13 | `projects/windlabx4/services/api-go/internal/ports/device.go` | 新增 `MPS03Configurable` 接口 |
| 14 | `projects/windlabx4/services/api-go/internal/adapters/config/default_profiles.go` | `NewDefaultProfile` switch 新增 MPS03 分支（默认 IP/端口/16 通道）+ `NormalizeProfile` MPS03 规范化 |
| 15 | `projects/windlabx4/services/api-go/internal/bootstrap/bootstrap.go` | `deviceFactory.Create` switch 新增 `case device.DeviceMPS03` |
| 16 | `projects/windlabx4/services/api-go/api/server.go` | `handleDeviceByID` 新增 MPS03 配置端点 + `/api/storage/start` 收集 MPS03 channels |
| 17 | `projects/windlabx4/services/api-go/internal/adapters/storage/csv_sink.go` | `buildDynamicHeader` 新增 MPS03 分支（17 列固定表头） |
| 18 | `projects/windlabx4/apps/desktop-wails/frontend/src/api/types.ts` | `DeviceType` 联合新增 `'MPS03'` + `MPS03HardwareConfig` 接口 |
| 19 | `projects/windlabx4/apps/desktop-wails/frontend/src/api/deviceApi.ts` | 新增 `getMps03Config` / `applyMps03Config` |
| 20 | `projects/windlabx4/apps/desktop-wails/frontend/src/stores/deviceStore.ts` | 新增 MPS03 配置操作 + 校零跳过 T_ext/T_int |
| 21 | `projects/windlabx4/apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue` | MPS03 类型分支（设备类型下拉、默认值、配置加载/保存、采样率只读展示、校零跳过） |
| 22 | `projects/windlabx4/apps/desktop-wails/backend/app.go` | 新增 Wails binding：`DeviceGetMps03Config` / `DeviceApplyMps03Config` |

---

## Code Style

### shared SDK 驱动层风格（参照 `daq_p1603.go`）

```go
// 文件：shared/device-sdk/go/daq/hardware/mps03.go
package hardware

import (
	"log/slog"
	"net"
	"sync"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/daq/ports"
	"shared.local/device-sdk/go/protocol"
)

const (
	MPS03DefaultHost       = "192.168.1.9"
	MPS03DefaultPort       = 9000
	MPS03CmdTimeout        = 3 * time.Second
	MPS03ConnectTimeout    = 5 * time.Second
	MPS03ReconnectMaxRetry = 5                 // 最大重连次数
	MPS03ReconnectBaseDelay = 1 * time.Second  // 指数退避基础延迟
	MPS03ParseErrorThreshold = 50              // 连续解析错误阈值
)

// MPS03Device shared SDK 真实驱动，实现 shared.ports.Device + shared.ports.MPS03Configurable。
//
// 关键设计：
//   - 命令响应采用"超时 + 响应识别"机制，禁止用 bufio.ReadLine（SET/GET/SAVE 响应无换行符）
//   - 采集数据流与命令响应通过 pendingResolve 单槽位严格分离
//   - 重连状态机内嵌（见 §重连状态机），由驱动自身负责
//   - 解析错误帧丢弃并计数，连续超阈值触发 Error 态
type MPS03Device struct {
	mu        sync.RWMutex
	ioMu      sync.Mutex  // 串行化命令发送，避免响应错位
	profile   core.Profile
	status    core.Status
	sink      core.DataSink
	cmdClient *protocol.MPS03CommandClient
	conn      net.Conn
	stop      chan struct{}
	readDone  chan struct{}

	// 重连状态机
	reconnectOwner *MPS03ReconnectState

	// 错误计数
	consecutiveParseErrors int

	onError func(error)
}

// 编译时接口检查：确保 MPS03Device 实现 shared SDK 接口
var _ ports.Device = (*MPS03Device)(nil)
var _ ports.MPS03Configurable = (*MPS03Device)(nil)

// Connect 建立 TCP 连接并自动读取全部硬件配置（9 项 #GET）。
// 不发送任何 #SET 命令，避免修改设备状态。
func (d *MPS03Device) Connect() error {
	// ... 实现见 §重连状态机
}
```

### shared SDK 协议层风格（参照 `daq_p1604_frame.go`）

```go
// 文件：shared/device-sdk/go/protocol/mps03_frame.go
package protocol

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MPS03CommandClient MPS-03 命令收发客户端。
//
// 关键设计：SET/GET/SAVE 响应不带换行符，必须用"发送后等待 + 超时 + 响应识别"机制。
// 仅采集数据行以 \r\n 结尾，可用行分割解析。
type MPS03CommandClient struct {
	conn    net.Conn
	ioMu    sync.Mutex
	timeout time.Duration
}

// Send 发送命令并等待响应。
//
// 响应识别规则（按优先级）：
//  1. 收到 `A` → SET/SAVE 成功响应
//  2. 收到 `E`/`I` → TCHO GET 响应
//  3. 收到全数字或十六进制字符串 → 其他 GET 响应
//  4. 收到以 `#` 开头 → 命令回显，跳过继续等待
//  5. 收到含逗号 → 采集数据行，说明命令发送时机错误，返回 ErrCommandDuringAcquisition
//  6. 超时 → ErrCommandTimeout
//
// 半包/粘包处理：
//   - 每次 Read 追加到内部缓冲区
//   - 检查缓冲区是否包含完整响应（按上述规则识别）
//   - 未识别完整响应则继续 Read，直到超时
func (c *MPS03CommandClient) Send(cmd string) (string, error) {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	// ... 实现略
}

// ParseCSVLine 解析 MPS-03 采集数据行。
//
// HEAD=1: 17 字段，首字段为序号，需 slice(1) 跳过
// HEAD=0: 16 字段，全部为通道数据
//
// 错误策略（Critical）：
//   - 字段数 != 预期 → 返回 ErrInvalidFieldCount，调用方丢弃整帧
//   - 单字段无法 parseFloat → 返回 ErrInvalidFieldValue，调用方丢弃整帧
//   - 禁止用 0 兜底，避免掩盖通信损坏
func ParseCSVLine(line string, headEnabled bool) ([16]float64, error) {
	// ... 实现略
}

// 错误定义
var (
	ErrCommandTimeout          = errors.New("mps03: command timeout")
	ErrCommandDuringAcquisition = errors.New("mps03: command sent during acquisition")
	ErrInvalidFieldCount       = errors.New("mps03: invalid CSV field count")
	ErrInvalidFieldValue       = errors.New("mps03: invalid CSV field value")
)
```

### WindLabX4 thin wrapper 风格（参照 `daq_p1603_adapter.go`）

```go
// 文件：projects/windlabx4/services/api-go/internal/adapters/hardware/mps03_adapter.go
package hardware

import (
	"sync"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/ports"

	mpssdk "shared.local/device-sdk/go/daq/hardware"
	mpscore "shared.local/device-sdk/go/daq/core"
)

// MPS03Adapter WindLabX4 项目层 thin wrapper，委托给 shared SDK 的 MPS03Device。
// 仅做类型翻译：WindLabX4 Profile ↔ shared SDK Profile、Status、DataPayload。
type MPS03Adapter struct {
	mu      sync.RWMutex
	profile device.Profile
	status  device.Status
	sink    device.DataSink
	driver  *mpssdk.MPS03Device
	onError func(err error)
}

// 编译时接口检查
var _ ports.Device = (*MPS03Adapter)(nil)
var _ ports.MPS03Configurable = (*MPS03Adapter)(nil)
var _ ports.ErrorNotifiable = (*MPS03Adapter)(nil)

// Connect 委托给 shared SDK 驱动，完成后翻译状态。
func (a *MPS03Adapter) Connect() error {
	// ... 翻译 profile → mpssdk profile，调用 driver.Connect，翻译 status 回来
}
```

### Vue 组件风格

```vue
<template>
  <UiPanel section="MPS-03 配置">
    <UiFormField label="采样平均次数">
      <UiSelect v-model="draft.avg" :options="avgOptions" :disabled="!editable" />
    </UiFormField>
    <!-- ... -->
  </UiPanel>
</template>

<script setup lang="ts">
import { reactive, computed } from 'vue'
import type { MPS03HardwareConfig } from '@/api/types'

const props = defineProps<{
  modelValue: MPS03HardwareConfig
  acquiring: boolean  // 采集状态下禁用编辑
}>()
const emit = defineEmits<{
  'update:modelValue': [config: MPS03HardwareConfig]
}>()

// editable: 仅在非采集状态下允许编辑（Critical: 采集期间禁止 #SET）
const editable = computed(() => !props.acquiring)

const draft = reactive<MPS03HardwareConfig>({ ...props.modelValue })
</script>
```

### 命名约定

| 类型 | 约定 | 示例 |
|---|---|---|
| Go 类型 | PascalCase | `MPS03Device`、`MPS03HardwareConfig`、`MPS03Adapter` |
| Go 常量 | PascalCase（exported） | `MPS03DefaultPort`、`MPS03CmdTimeout` |
| Go 接口 | PascalCase + able 后缀 | `MPS03Configurable` |
| Go 错误 | Err 前缀 | `ErrCommandTimeout`、`ErrInvalidFieldCount` |
| TS 类型 | PascalCase | `MPS03HardwareConfig` |
| Vue 组件 | PascalCase + Panel 后缀 | `Mps03ConfigPanel` |
| 设备 Type 字面量 | 全大写无连字符 | `'MPS03'`（**禁止 `'MPS-03'`**） |
| 文件名 | 全小写下划线 | `mps03.go`、`mps03_adapter.go`、`mps03_frame.go` |

---

## 协议契约（Critical）

### 1. 命令响应分帧规则

MPS-03 协议的 TCP 流存在两种数据：
- **命令响应**：SET/GET/SAVE 的响应，**不带换行符**
- **采集数据行**：CSV 格式，以 `\r\n` 结尾

**响应识别规则**（按优先级，在协议层 `MPS03CommandClient.Send` 中实现）：

| 响应内容 | 识别为 | 处理 |
|---|---|---|
| `A` | SET/SAVE 成功 | 返回 `"A"` |
| `E` 或 `I` | TCHO GET 响应 | 返回字母 |
| 全数字字符串（含负号、小数点） | 数值 GET 响应（AVG/BIN/DELAY/CNUM/HEAD/TMODE/TTYPE） | 返回字符串 |
| 4 位十六进制（如 `FFFF`） | CHA GET 响应 | 返回字符串 |
| 以 `#` 开头 | 命令回显 | **跳过，继续等待** |
| 含逗号 | 采集数据行 | 返回 `ErrCommandDuringAcquisition`（说明调用时机错误） |
| 其他 | 未知响应 | 返回错误 |
| 超时（3s） | 无响应 | 返回 `ErrCommandTimeout` |

**半包/粘包处理**：
- 每次 `conn.Read` 追加到内部缓冲区 `recvBuffer`
- 每次追加后检查缓冲区是否包含完整响应（按上述规则）
- 未识别完整响应则继续 Read，直到超时
- 已识别响应后，缓冲区剩余字节保留供下次使用（不丢弃）

**`#START` / `#STOP` 无响应处理**：
- 发送 `#START` / `#STOP` 后**不等待响应**，直接返回 nil
- 通过"首条数据行到达"判断 `#START` 生效
- 通过"数据流停止"判断 `#STOP` 生效
- 如果 `#START` 后 2 倍 DELAY 时间内无数据，返回 `ErrStartNoData`

### 2. 采集期间禁止配置命令（Critical）

**约束**：采集期间（`acquiring == true`）禁止发送任何 `#SET` / `#GET` / `#SAVE` 命令。

**原因**：数据流与命令响应在同一 TCP 流中混杂，无法可靠区分。

**实现**：
- `MPS03Device.ApplyHardwareConfig` 在 `acquiring == true` 时返回 `ErrAcquiringNotAllowConfig`
- `MPS03Device.GetHardwareConfig` 在 `acquiring == true` 时返回 `ErrAcquiringNotAllowConfig`
- WindLabX4 HTTP 层将此错误映射为 `409 Conflict`
- 前端 `Mps03ConfigPanel` 在 `acquiring == true` 时禁用所有配置输入

### 3. CHA 通道掩码与 DataPayload 契约

**设备行为假设**（需在模拟器和真实硬件上验证）：
- `CHA` 控制设备是否在 CSV 行中**输出**对应通道字段
- `CHA != FFFF` 时，CSV 行字段数 < 16，**仅包含启用通道**
- 字段顺序按通道索引升序（CH1, CH2, ..., CH16 中启用的那些）

**DataPayload 契约**：
- `Channels []float64`：**仅包含启用通道的值**，长度 = 启用通道数
- `ChannelIndices []int`：**启用通道的原始索引**（0-15），与 `Channels` 一一对应
- 禁止用 0 填充禁用通道（避免与真实零值混淆）

**CSV 录制表头**：
- **始终输出 17 列**（Timestamp + 16 通道），禁用通道列填空字符串
- 原因：保持 CSV 结构稳定，便于后续分析和回放

**Profile.Channels 配置**：
- `Profile.Channels` 始终包含 16 个通道定义（完整元数据）
- `CHA` 掩码由 `Profile.Channels[i].Enabled` 推导：`cha = 0; for i, c := range channels { if c.Enabled { cha |= 1 << i } }`
- 用户在 UI 修改通道 `Enabled` 复选框 → `ApplyMps03Config` 自动计算 `CHA` 并下发

**待验证项**（标注 `TODO-HW-VERIFY`）：
1. 设备在 `CHA = 000F`（仅前 4 通道启用）时 CSV 行的实际字段数和顺序
2. `HEAD=1` + `CHA != FFFF` 时序号字段是否仍存在
3. 模拟器按上述假设实现，真实硬件验证后如有差异需更新本 spec

### 4. 错误帧处理策略（Critical）

**禁止填 0 兜底**。错误策略分层：

| 错误类型 | 处理 | 计数 | 阈值动作 |
|---|---|---|---|
| CSV 字段数 != 预期 | 丢弃整帧 | `consecutiveParseErrors++` | 连续 50 帧 → 触发 Error 态 |
| 单字段 parseFloat 失败 | 丢弃整帧 | `consecutiveParseErrors++` | 同上 |
| 命令超时 | 返回错误，调用方决定 | 不计入解析错误 | 连续 3 次命令超时 → Error 态 |
| TCP Read 错误 | 触发重连状态机 | 不计入 | 见 §重连状态机 |

**成功解析一帧**后 `consecutiveParseErrors` 重置为 0。

**错误帧不进入 DataSink**：
- 协议层 `ParseCSVLine` 返回错误时，驱动层不调用 `sink(payload)`
- 驱动层记录 Debug 日志（含原始行内容）+ 错误计数
- 达到阈值时调用 `onError` 回调，状态切换为 `Error`，触发重连状态机

### 5. 重连状态机（Required）

**Owner**：shared SDK 驱动层 `MPS03Device` 自身负责重连（不在 WindLabX4 adapter 或 DeviceManager 层）。

**状态机**：

```
                   ┌─────────────────────────────────┐
                   │                                 │
                   ▼                                 │
  ┌─────────┐  Connect()   ┌───────────┐  StartAcq  ┌────────────┐
  │ Disconn │ ──────────▶ │ Connected │ ─────────▶ │ Acquiring  │
  └─────────┘             └───────────┘            └────────────┘
       ▲                       │                        │
       │                       │ TCP Read 错误          │ TCP Read 错误
       │                       ▼                        ▼
       │                  ┌───────────┐            ┌────────────┐
       │                  │ Reconnect │ ◀───────── │   Error    │
       │                  │ (backoff) │            │ (recorded) │
       │                  └───────────┘            └────────────┘
       │                       │                        │
       │                       │ 重连成功                │ 手动 Disconnect
       │                       ▼                        │
       │                  恢复原状态                     │
       │                  (Connected 或                 │
       │                   Acquiring)                   │
       │                                               │
       └───────────────────────────────────────────────┘
                        手动 Disconnect
                        (禁止自动重连)
```

**重连退避策略**：
- 基础延迟：1s
- 指数退避：1s, 2s, 4s, 8s, 16s
- 最大重试次数：5 次
- 重试期间状态：`ConnectionError`（不是 `Disconnected`）
- 重试期间 `LastError`：记录最近一次错误

**重连成功后恢复逻辑**：
1. 重新执行 9 个 `#GET` 读取硬件配置
2. 如果断连前 `acquiring == true`：
   - 重新发送 `#SET BIN 0`
   - 重新发送 `#START`
   - 状态切换为 `Acquiring`
3. 如果断连前 `acquiring == false`：
   - 状态切换为 `Connected`
4. `consecutiveParseErrors` 重置为 0

**手动 Disconnect 禁止重连**：
- `Disconnect()` 方法设置 `manualDisconnect = true` 标志
- 重连状态机检查此标志，为 true 时立即退出
- 状态切换为 `Disconnected`

**重连失败处理**：
- 5 次重连均失败 → 状态 `ConnectionError` + `LastError` 记录
- 调用 `onError` 回调通知 WindLabX4 adapter
- WindLabX4 adapter 通知 DeviceManager 更新 UI 状态
- 不再自动重连，等待用户手动 `Connect()`

**旧连接清理**：
- 重连前确保旧 `conn.Close()` 已调用
- 旧 readLoop goroutine 通过 `stop` channel 退出，等待 `readDone` 关闭（1s 超时）
- 超时未退出则强制 `conn.Close()`，readLoop 会因 Read 错误退出

---

## MPS03HardwareConfig 数据契约（Required）

### shared SDK 定义

```go
// 文件：shared/device-sdk/go/daq/core/types.go

// MPS03HardwareConfig MPS-03 设备硬件配置（9 项 #GET/#SET 参数）。
//
// 存储：
//   - tmode 内部用十进制存储（GET 返回十进制，SET 时转十六进制发送）
//   - tcho 内部用字母 "E"/"I" 存储（GET 返回字母，SET 时转数字发送）
//   - 其他字段直接对应协议值
type MPS03HardwareConfig struct {
	// AVG 采样平均次数，合法值 4|8|32|64
	Avg int `json:"avg"`
	// BIN 输出格式，0=字符串（强制），1=十六进制小端（不支持）
	Bin int `json:"bin"`
	// DELAY 输出间隔，单位 ms，范围 [10, 60000]
	Delay int `json:"delay"`
	// CNUM 采集次数，0=无限，>0=按次数采集
	Cnum int `json:"cnum"`
	// HEAD 序号头，0=关闭，1=开启
	Head int `json:"head"`
	// CHA 通道掩码，4 位十六进制字符串（如 "FFFF"），按位使能通道 0-15
	Cha string `json:"cha"`
	// TMODE 温度模式，内部十进制存储，SET 时转十六进制
	// 高 4 位=外部传感器类型（1=热电偶, 0=PT100），低 4 位=内部（1=DS18B20 固定）
	Tmode int `json:"tmode"`
	// TTYPE 热电偶类型，0=K,1=B,2=E,3=J,4=T,5=S,6=N,7=R,8=C
	Ttype int `json:"ttype"`
	// TCHO 参与计算的温度，"E"=外部, "I"=内部
	Tcho string `json:"tcho"`
}
```

### WindLabX4 镜像定义

```go
// 文件：projects/windlabx4/services/api-go/internal/core/device/types.go
// 与 shared SDK 镜像，但属于 WindLabX4 自有类型，由 adapter 翻译

type MPS03HardwareConfig struct {
	Avg   int    `json:"avg"`
	Bin   int    `json:"bin"`
	Delay int    `json:"delay"`
	Cnum  int    `json:"cnum"`
	Head  int    `json:"head"`
	Cha   string `json:"cha"`
	Tmode int    `json:"tmode"`
	Ttype int    `json:"ttype"`
	Tcho  string `json:"tcho"`
}
```

### 校验规则

| 字段 | 合法值 | 非法值处理 |
|---|---|---|
| `Avg` | 4, 8, 32, 64 | 返回 400 Bad Request |
| `Bin` | 0（强制） | 非 0 返回 400 |
| `Delay` | [10, 60000] | 超范围返回 400 |
| `Cnum` | >= 0 | 负数返回 400 |
| `Head` | 0 或 1 | 其他返回 400 |
| `Cha` | 4 位十六进制 | 格式错误返回 400 |
| `Tmode` | [0, 255] | 超范围返回 400 |
| `Ttype` | [0, 8] | 超范围返回 400 |
| `Tcho` | "E" 或 "I" | 其他返回 400 |

---

## API 契约（Required）

### HTTP 端点

| Method | Path | 说明 | 采集状态要求 |
|---|---|---|---|
| GET | `/api/device/{id}/mps03-config` | 读取硬件配置（缓存优先，forceRefresh=true 强制刷新） | 任意（采集时返回 409） |
| PUT | `/api/device/{id}/mps03-config` | 应用硬件配置（自动 #SAVE） | **必须非采集状态** |

### 请求/响应结构

**GET `/api/device/{id}/mps03-config`**

```
Query: forceRefresh=true|false（可选，默认 false）

Response 200:
{
  "success": true,
  "data": {
    "avg": 8,
    "bin": 0,
    "delay": 1000,
    "cnum": 0,
    "head": 1,
    "cha": "FFFF",
    "tmode": 17,
    "ttype": 2,
    "tcho": "E"
  }
}

Response 409 (采集期间):
{
  "success": false,
  "error": "acquiring in progress, stop acquisition first"
}

Response 503 (设备未连接):
{
  "success": false,
  "error": "device not connected"
}
```

**PUT `/api/device/{id}/mps03-config`**

```
Request Body:
{
  "avg": 8,
  "delay": 500,
  "cha": "FFFF",
  "tmode": 17,
  "ttype": 2,
  "tcho": "E"
  // 所有字段可选，仅下发非 nil 字段
}

Response 200:
{
  "success": true,
  "data": { ... }  // 应用后的完整配置（GET 回读）
}

Response 400 (校验失败):
{
  "success": false,
  "error": "invalid avg: must be one of 4,8,32,64"
}

Response 409 (采集期间):
{
  "success": false,
  "error": "acquiring in progress, stop acquisition first"
}

Response 502 (#SET 或 #SAVE 失败):
{
  "success": false,
  "error": "device rejected SET command: <原始错误>"
}
```

### Apply 失败处理

| 失败阶段 | 处理 |
|---|---|
| 校验失败 | 返回 400，不发送任何命令 |
| 设备未连接 | 返回 503 |
| 采集进行中 | 返回 409 |
| 单个 #SET 失败 | 立即停止后续 #SET，返回 502，**不执行 #SAVE** |
| #SAVE 失败 | 返回 502，但已下发的 #SET 实际生效（仅未持久化） |
| 超时 | 视为失败，返回 504 |

### Wails binding

```go
// 文件：projects/windlabx4/apps/desktop-wails/backend/app.go

// DeviceGetMps03Config 读取 MPS-03 硬件配置。
// forceRefresh=true 时强制从设备读取，否则用缓存。
func (a *App) DeviceGetMps03Config(id string, forceRefresh bool) (MPS03ConfigResult, error)

// DeviceApplyMps03Config 应用 MPS-03 硬件配置（自动 #SAVE）。
// 采集进行中返回错误。
func (a *App) DeviceApplyMps03Config(id string, config MPS03HardwareConfig) (MPS03ConfigResult, error)

type MPS03ConfigResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    *MPS03HardwareConfig `json:"data,omitempty"`
}
```

**Wails binding 同步**：修改 binding 签名后必须执行 `wails3 generate bindings -silent`，否则前端类型不匹配。

---

## CSV 录制契约（Required）

### 17 列固定表头

MPS-03 录制 CSV 始终输出 **17 列**（Timestamp + 16 通道），与 DAQ-P-1604 的 18 列宽格式独立。

**列顺序与列名**（ASCII，稳定可编程，**禁止中文列名**）：

| # | 列名 | 单位 | 通道索引 |
|---|---|---|---|
| 1 | `Timestamp` | ms（Unix 毫秒） | — |
| 2 | `Alpha_deg` | ° | 0 |
| 3 | `Beta_deg` | ° | 1 |
| 4 | `MachNumber` | — | 2 |
| 5 | `Velocity_mps` | m/s | 3 |
| 6 | `Vx_mps` | m/s | 4 |
| 7 | `Vy_mps` | m/s | 5 |
| 8 | `Vz_mps` | m/s | 6 |
| 9 | `Sensor1` | — | 7 |
| 10 | `Sensor2` | — | 8 |
| 11 | `Sensor3` | — | 9 |
| 12 | `Sensor4` | — | 10 |
| 13 | `Sensor5` | — | 11 |
| 14 | `Sensor6` | — | 12 |
| 15 | `TotalPressure_kPa` | kPa | 13 |
| 16 | `ExtTemp_degC` | ℃ | 14 |
| 17 | `IntTemp_degC` | ℃ | 15 |

### 禁用通道处理

- `CHA` 关闭的通道列**保留在表头**，数据行对应列填**空字符串**（不是 0）
- 原因：保持 CSV 结构稳定，便于 Excel/pandas 解析

### CSV 编码

- UTF-8 with BOM（与 WindLabX4 现有 CSV 一致，确保 Excel 正确识别中文路径）
- 行尾 `\r\n`（Windows 风格）

### sink 实现

`csv_sink.go` 的 `buildDynamicHeader` 新增分支：

```go
case device.DeviceMPS03:
	// MPS-03 固定 17 列表头：Timestamp + 16 通道 ASCII 列名 + 单位后缀
	header := []string{"Timestamp"}
	header = append(header, []string{
		"Alpha_deg", "Beta_deg", "MachNumber", "Velocity_mps",
		"Vx_mps", "Vy_mps", "Vz_mps",
		"Sensor1", "Sensor2", "Sensor3", "Sensor4", "Sensor5", "Sensor6",
		"TotalPressure_kPa", "ExtTemp_degC", "IntTemp_degC",
	}...)
	return header
```

---

## 采样率 UI 语义（Required）

### 权威值

- **设备 DELAY（ms）是权威值**，profile.SamplingRate 是派生值
- `profile.SamplingRate = round(1000 / hardwareConfig.Delay)`
- 设备实际 DELAY 与 profile.SamplingRate 不一致时，以设备 DELAY 为准

### UI 行为

| UI 位置 | 行为 |
|---|---|
| 设备管理抽屉 - 采样率输入框 | **隐藏**（MPS-03 不显示通用 samplingRate 输入） |
| Mps03ConfigPanel - DELAY 输入框 | **可编辑**（10-60000 ms），显示当前设备 DELAY |
| Mps03ConfigPanel - 采样率只读展示 | `采样率: {round(1000/delay)} Hz`（DELAY 修改时实时计算） |
| 设备卡片 - 采样率显示 | 显示派生的 Hz 值（只读） |

### 非整除处理

- `DELAY = 10` → `SamplingRate = 100` Hz
- `DELAY = 30` → `SamplingRate = round(1000/30) = 33` Hz（显示 33 Hz，实际 DELAY 仍为 30ms）
- `DELAY = 1000` → `SamplingRate = 1` Hz
- 不强制 DELAY 必须整除 1000，接受精度损失

### UI 修改入口

- 用户**只能通过修改 DELAY** 间接改变采样率
- 禁止直接输入 Hz 值（避免 DELAY 与 Hz 双向绑定导致循环）

---

## Testing Strategy

### 测试金字塔

| 层 | 框架 | 位置 | 覆盖目标 |
|---|---|---|---|
| 协议层单元测试 | Go testing | `shared/device-sdk/go/protocol/mps03_frame_test.go` | CSV 解析、TMODE 十六进制转换、TCHO 字母映射、响应识别规则、半包粘包、超时 |
| 驱动单元测试 | Go testing + mock TCP server | `shared/device-sdk/go/daq/hardware/mps03_test.go` | 连接/断开、配置读写、采集启停、数据帧解析、错误帧丢弃、重连状态机、采集期间禁止配置 |
| 模拟器测试 | Go testing | `shared/device-sdk/go/testing/sim/mps03_sim_test.go` | 模拟器响应正确性 |
| WindLabX4 适配器测试 | Go testing | `projects/windlabx4/.../adapters/hardware/mps03_adapter_test.go` | 类型翻译正确性、sink 转发、错误回调 |
| WindLabX4 API 集成测试 | Go testing | `projects/windlabx4/.../api/server_test.go`（新增 MPS03 用例） | HTTP 端点、409/400/502 状态码、采集期间禁止 Apply |
| 前端类型检查 | tsc | `npm run typecheck` | DeviceType 联合类型完整 |
| 前端构建 | vite | `npm run build` | 组件可编译 |
| 端到端验证 | 手动 + 模拟器 | 真实硬件 | 连接 → 配置 → 采集 → 录制全流程 |

### 测试用例格式（三段式）

```go
func TestParseCSVLine_HEAD1_Valid(t *testing.T) {
	// 测试前置：构造 HEAD=1 的 17 字段 CSV 行
	line := "0,14.50,-5.87,0.57,182.99,-45.59,18.71,176.23,-11.64,0.00,-12.38,-0.00,-13.36,72.02,1.40,-13.12,0.00"

	// 测试步骤：以 headEnabled=true 解析
	values, err := protocol.ParseCSVLine(line, true)

	// 期待结果：首字段序号被跳过，16 通道值正确，无错误
	if err != nil { t.Fatalf("解析失败: %v", err) }
	if values[0] != 14.50 { t.Errorf("α 期望 14.50, 实际 %f", values[0]) }
	if values[13] != 1.40 { t.Errorf("P_total 期望 1.40, 实际 %f", values[13]) }
}

func TestParseCSVLine_InvalidFieldCount(t *testing.T) {
	// 测试前置：构造字段数不足的 CSV 行
	line := "14.50,-5.87,0.57"

	// 测试步骤：以 headEnabled=false 解析
	_, err := protocol.ParseCSVLine(line, false)

	// 期待结果：返回 ErrInvalidFieldCount，禁止填 0
	if !errors.Is(err, protocol.ErrInvalidFieldCount) {
		t.Fatalf("期望 ErrInvalidFieldCount, 实际 %v", err)
	}
}
```

### 关键测试场景

1. **协议层**
   - HEAD=0/HEAD=1 两种数据行解析（合法）
   - 字段数不足/超长 → `ErrInvalidFieldCount`（禁止填 0）
   - 单字段非数字 → `ErrInvalidFieldValue`（禁止填 0）
   - TMODE SET 十六进制转换（`17` → `"11"`）
   - TCHO 字母 ↔ 数字映射（`"E"` ↔ `"1"`、`"I"` ↔ `"0"`）
   - 命令响应识别（`A` / `E` / `I` / 数字 / 十六进制 / `#` 回显 / 含逗号数据行）
   - 半包：响应分两次 Read 到达
   - 粘包：两个响应合并到一个 Read
   - 命令超时（3s）

2. **驱动层**
   - 连接成功后自动发送 9 个 #GET 并触发 onConfigSynced
   - #SET BIN 0 + #START 启动采集
   - #START 后 2 倍 DELAY 无数据 → `ErrStartNoData`
   - #STOP 停止采集（不等响应）
   - **采集期间调用 ApplyHardwareConfig → 返回 `ErrAcquiringNotAllowConfig`**
   - **采集期间调用 GetHardwareConfig → 返回 `ErrAcquiringNotAllowConfig`**
   - 解析错误帧不进入 sink
   - 连续 50 帧解析错误 → 触发 onError + Error 态
   - TCP 断连 → 重连状态机启动
   - 重连成功 → 恢复 9 个 #GET + 重新 #START（如果断连前在采集）
   - 重连 5 次失败 → Error 态，不再重连
   - 手动 Disconnect → 禁止自动重连

3. **模拟器**
   - 9 个 #GET 命令的正确响应回放
   - #START 后按 DELAY 间隔推送 CSV 数据
   - #STOP 立即停止推送
   - #SAVE 返回 `A`
   - CHA != FFFF 时按启用通道输出（验证 §CHA 契约假设）

4. **WindLabX4 API 集成**
   - GET /api/device/{id}/mps03-config 返回当前配置
   - PUT /api/device/{id}/mps03-config 采集期间返回 409
   - PUT /api/device/{id}/mps03-config 校验失败返回 400
   - PUT /api/device/{id}/mps03-config 成功后 #SAVE 已执行
   - CSV 录制 17 列表头正确

---

## Boundaries

### Always（必须遵守）

1. **六边形架构约束**：`core/` 零硬件依赖、`ports/` 零实现、`usecase/` 零直接硬件调用、`adapters/hardware/` 零业务逻辑
2. **shared SDK 依赖方向**：`shared/device-sdk/go/*` 禁止 `import "windlabx4/services/api-go/internal/*"`
3. **命名隔离**：MPS-03 与 DAQ-P-1603 命名严格隔离，禁止复用 `DAQP1603*` 名称
4. **per-id 互斥锁**：MPS-03 适配器内 `ioMu` 串行化命令发送；DeviceManager 的 `connMuRegistry` 保证同设备串行、跨设备并行
5. **命令响应分帧**：SET/GET/SAVE 响应按 §协议契约 §1 的识别规则处理，禁止用 `bufio.ReadLine`
6. **采集期间禁止配置**：`acquiring == true` 时禁止 #SET/#GET/#SAVE，返回 `ErrAcquiringNotAllowConfig`
7. **错误帧不填 0**：解析失败的 CSV 帧丢弃整帧，禁止用 0 兜底
8. **配置变更必 #SAVE**：`applyHardwareConfig` 完成所有 #SET 后自动调用 #SAVE，禁止跳过
9. **connect() 不改设备状态**：仅建立 TCP + 9 个 #GET 读取配置，禁止发送任何 #SET
10. **startAcquisition 强制 BIN=0**：每次启动采集前发送 `#SET BIN 0`
11. **CSV 17 列固定表头**：MPS-03 在 `buildDynamicHeader` 走专属 17 列分支，ASCII 列名 + 单位后缀
12. **重连状态机内嵌**：重连由 shared SDK 驱动层负责，不在 WindLabX4 adapter 或 DeviceManager 层
13. **手动 Disconnect 禁止重连**：`manualDisconnect` 标志阻止自动重连
14. **日志级别**：高频采集数据 Debug，命令收发 Info（受前端开关控制），warn/error 强制可见
15. **生产代码中文注释**：解释"为什么"而非"做了什么"
16. **提交前验证**：shared SDK + WindLabX4 两个模块均 `go build` + `go test` + `go vet` 全绿，前端 `npm typecheck` + `npm build` 全绿

### Ask First（先问再做）

1. 修改 `DeviceType` 联合类型（影响前后端全链路）
2. 修改 `device.Profile` 结构（影响配置持久化格式）
3. 修改 `RecordingConfig.DeviceChannels` 注入逻辑（影响 CSV 录制）
4. 新增 shared SDK 公共 API（影响跨项目复用契约）
5. 修改 Wails binding 签名（必须同步 `wails3 generate bindings`）
6. 修改 §CHA 通道掩码契约（需先在真实硬件验证）

### Never（禁止）

1. 在 shared SDK 驱动层导入 `windlabx4/services/api-go/internal/*`
2. 在驱动层直接操作 BrowserWindow 或 Wails runtime
3. 在驱动层使用 `any` 类型（Go 用 `interface{}` 时必须有类型断言保护）
4. 跳过 #SAVE 直接假设配置已生效
5. 用 `bufio.ReadLine` 接收 SET/GET/SAVE 响应
6. 在采集期间发送 #GET/#SET/#SAVE 命令
7. 用 0 兜底解析错误的 CSV 字段
8. 在 `core/` 引入 `net`、`os`、`io` 等硬件/IO 包
9. 在 `ports/` 写实现代码
10. 删除现有设备的测试或表头分支
11. 复用 `DAQP1603*` 命名
12. 提交未通过 `validate-structure.ps1` 的代码
13. 使用中文 CSV 列名（必须 ASCII）

---

## Success Criteria

| # | 条件 | 验证方式 |
|---|---|---|
| 1 | `DeviceType = 'MPS03'` 在前后端均定义且一致，与 DAQ-P-1603 完全隔离 | `npm run typecheck` + `go build` + grep 确认无 `DAQP1603` 复用 |
| 2 | 添加 MPS-03 设备 → 默认 IP 192.168.1.9 / 端口 9000 / 16 通道 | 手动 UI 操作 |
| 3 | 连接成功 → 后端日志显示 9 个 #GET 完成 → 前端只读显示当前硬件配置 | 后端日志 + UI |
| 4 | 采集期间调用 ApplyMps03Config → 返回 409 Conflict | HTTP 请求验证 |
| 5 | 修改 DELAY → 设备 #SAVE 后断电不丢失 → 重连验证 | UI 修改 → 重连 |
| 6 | 启动采集 → 波形图显示启用通道实时数据 → 采样率符合 DELAY 设定 | UI 波形图 + 时间戳间隔 |
| 7 | 录制 CSV → 17 列表头（Timestamp + 16 ASCII 列名 + 单位后缀） | 打开 CSV 文件 |
| 8 | 停止采集 → 数据流立即停止 → 状态变为 Connected | UI 状态徽章 |
| 9 | 拔网线 → 触发重连状态机 → 5 次内重连成功 → 恢复采集 | 手动断网测试 |
| 10 | 重连 5 次失败 → Error 态 → 不再自动重连 → 等待手动 Connect | 手动断网测试 |
| 11 | 解析错误帧不进入 DataSink → 连续 50 帧错误触发 Error 态 | 模拟器注入错误帧 |
| 12 | 模拟器独立运行 → 前端用 MPS03 模拟器可完成全流程 | `go test ./testing/sim/... -run MPS03` |
| 13 | shared SDK 模块 `go test ./...` + `go vet ./...` 全绿 | CI |
| 14 | WindLabX4 模块 `go test ./...` + `go vet ./...` 全绿 | CI |
| 15 | `npm run typecheck && npm run build` 全绿 | CI |
| 16 | `validate-structure.ps1` 全绿 | CI |

---

## Open Questions

全部已确认，无遗留问题。

| 问题 | 决策 | 理由 |
|---|---|---|
| SDK 位置 | shared SDK + thin wrapper | 跨项目复用，参照 DAQ-P-1603 模板 C |
| canonical type 字面量 | `"MPS03"`（无连字符） | 与 `DAQ-P-1603` 视觉区分，避免混淆 |
| 通道类型 | MPS-03 走独立通道系统，不扩展 ChannelSensorType | 通道异构，不干扰其他设备 |
| 配置 UI 范围 | 9 项全部可编辑（采集状态下禁用） | 完整暴露协议能力，UI 通过 disabled 状态约束 |
| 模拟器 | 包含最小模拟器 | 前端开发和无硬件联调必需 |
| 压力单位切换 | 不实现 | 协议中无单位设置命令 |
| 校准命令 #CAL | v1 不实现 | SKILL.md 明确标注 v1 未实现 |
| 设备发现 scan | 不支持 | MPS-03 无 UDP 广播发现机制 |
| BIN=1 十六进制模式 | 不支持 | SKILL.md 规定 startAcquisition 强制 BIN=0 |
| CHA != FFFF 时 CSV 格式 | 假设仅输出启用通道，待 HW 验证 | 标注 `TODO-HW-VERIFY`，模拟器按假设实现 |
| 重连 owner | shared SDK 驱动层 | 状态机内嵌，避免上层协调 |
| 采样率权威值 | 设备 DELAY | Hz 是派生值，只读展示 |
| CSV 列名 | ASCII + 单位后缀 | 稳定可编程，禁止中文 |

---

## 实现顺序（依赖关系概览）

> 详细 plan 和 tasks 在 Phase 2/3 阶段单独产出（`plan-mps03-integration.md` / `tasks-mps03-integration.md`），本 spec 仅给出概览。

```
Phase 1: shared SDK 协议层（无依赖）
  ├── 1.1 protocol/mps03_frame.go — 命令收发 + CSV 解析 + TMODE/TCHO 转换 + 响应识别 + 错误定义
  └── 1.2 protocol/mps03_frame_test.go — 单元测试（含半包粘包、错误帧、超时）

Phase 2: shared SDK 驱动层（依赖 Phase 1）
  ├── 2.1 daq/core/types.go — DeviceMPS03 + MPS03HardwareConfig + Profile.MPS03Config
  ├── 2.2 daq/ports/device.go — MPS03Configurable 接口
  ├── 2.3 daq/hardware/mps03.go — TCP 连接 + 采集循环 + 配置同步 + 重连状态机 + 错误计数
  ├── 2.4 daq/hardware/mps03_test.go — 单元测试（mock TCP server）
  └── 2.5 testing/sim/mps03_sim.go + 测试 — 模拟器

Phase 3: WindLabX4 项目层（依赖 Phase 2）
  ├── 3.1 core/device/types.go — Type 常量 + MPS03HardwareConfig + Profile.MPS03Config（镜像）
  ├── 3.2 ports/device.go — MPS03Configurable 接口
  ├── 3.3 adapters/hardware/mps03_adapter.go — thin wrapper（类型翻译）
  ├── 3.4 adapters/config/default_profiles.go — 默认 profile + 规范化
  ├── 3.5 bootstrap/bootstrap.go — deviceFactory.Create 新增 case
  ├── 3.6 api/server.go — MPS03 配置端点（GET/PUT + 409/400/502）+ CSV DeviceChannels 注入
  ├── 3.7 adapters/storage/csv_sink.go — MPS03 17 列表头分支
  └── 3.8 api/server_test.go — MPS03 API 集成测试

Phase 4: 前端（依赖 Phase 3）
  ├── 4.1 api/types.ts — DeviceType 联合 + MPS03HardwareConfig 接口
  ├── 4.2 api/deviceApi.ts — getMps03Config / applyMps03Config
  ├── 4.3 stores/deviceStore.ts — MPS03 配置操作 + 校零跳过 T_ext/T_int
  ├── 4.4 components/device/Mps03ConfigPanel.vue — 配置面板（采集状态禁用）
  └── 4.5 components/device/DeviceManagementDrawer.vue — MPS03 分支（采样率隐藏、DELAY 编辑、Hz 只读）

Phase 5: Wails binding 同步 + 验证
  ├── 5.1 backend/app.go — DeviceGetMps03Config / DeviceApplyMps03Config
  ├── 5.2 wails3 generate bindings -silent
  ├── 5.3 shared SDK + WindLabX4 双模块 go test / go vet
  ├── 5.4 npm typecheck + npm build
  └── 5.5 端到端验证（模拟器 + 真实硬件，含 CHA != FFFF 验证）
```
