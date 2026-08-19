# Spec: PACE1000 大气压力采集设备集成到 WindLabX4

> **协议来源**：用户提供的 LabVIEW 连接与采集框图（2026-08-19）。
>
> **已确认业务语义**：PACE1000 只输出 1 个数据，该数据固定表示大气压力。
>
> **当前协议置信度**：串口参数、查询命令、命令终止符 CR、`%s%f` 响应扫描语义、500 ms 等待和 `x1000` 换算已由用户确认；响应终止符尚待真机确认，见 Open Questions。

---

## Objective

将 PACE1000 集成到 WindLabX4，作为新的串口 DAQ 设备类型。操作员可在设备管理中选择 PACE1000、选择串口、连接设备并启动采集；系统周期发送查询命令，读取单个大气压力值，并以 Pa 作为 WindLabX4 内部工程单位进入实时显示、数据总线与录制流程。

### 用户故事

作为风洞实验人员，我希望：

1. 在设备管理中添加类型为 PACE1000 的设备。
2. 选择本机串口，其他串口参数自动使用设备默认值。
3. 连接并启动采集后，实时看到一个“大气压力”通道。
4. 大气压力以 Pa 显示并进入现有 CSV 录制和遍历数据链路。
5. 停止采集或断开设备时，不再发送查询命令，串口被可靠释放。

### 成功标准

1. 前后端设备类型字面量统一为 `PACE1000`，UI 显示名为 `PACE1000`。
2. PACE1000 仅支持串口传输，连接参数固定为 `9600-8-N-1`、无流控。
3. 启动采集后，每个采集周期发送 `:sens?\r`，等待 500 ms 后读取可用响应。
4. 从合法响应中得到唯一浮点数，乘以 `1000`，以 Pa 发布到 `Channels[0]`。
5. 默认 profile 只含一个固定通道：`Index=0`、名称“大气压力”、单位 `Pa`。
6. 非法、空或超时响应不得以 0 写入 DataSink；应丢弃该次采样并记录错误。
7. PACE1000 大气压力通道不得参与校零。
8. 后端测试、前端类型检查、前端构建和结构验证通过。

---

## Assumptions

以下假设用于形成首版 spec；未确认项不会在实现阶段静默猜测：

1. PACE1000 是采集设备，不是打压控制设备，不实现设压、泄压或控制模式命令。
2. canonical type 使用 `PACE1000`，不带空格或连字符。
3. 设备只有一个固定大气压力通道，不提供通道增删、通道类型或单位配置。
4. LabVIEW 图中的 `x1000` 是设备输出值到 Pa 的固定换算；即 `pressurePa = parsedValue * 1000`。
5. LabVIEW 图中的 500 ms 是每次写查询命令后读取响应前的必要等待。
6. 本期仅支持用户手动选择串口，不做 PACE1000 自动发现或串口探测握手。
7. 复用现有 `Profile.SerialPort`、`Profile.BaudRate`、`Profile.SamplingRate`，不新增 PACE1000 专属配置结构或 HTTP API。

---

## Tech Stack

| 层 | 技术 | 说明 |
|---|---|---|
| 串口基础设施 | Go + `go.bug.st/serial` | 复用 `shared/device-sdk/go/serialport` |
| shared SDK 驱动 | Go | 串口收发、轮询、响应解析、生命周期 |
| WindLabX4 适配器 | Go | thin wrapper，只做类型翻译 |
| 应用架构 | Go 六边形架构 | core / ports / usecase / adapters |
| 前端 | Vue 3 + TypeScript | 复用设备管理抽屉与通用通道 UI |
| 桌面壳 | Wails v3 | 不新增业务逻辑 |

---

## Commands

```powershell
# shared device SDK
cd shared/device-sdk/go
go build ./...
go test ./...
go vet ./...

# WindLabX4 backend
cd projects/windlabx4/services/api-go
go build -buildvcs=false ./...
gofmt -l .
go vet ./...
go test ./internal/... ./api/...

# WindLabX4 frontend
cd projects/windlabx4/apps/desktop-wails/frontend
npm run typecheck
npm run test
npm run build

# Workspace structure gates
cd ../../..
.\validate-structure.ps1
.\validate-frontend-structure.ps1 -CheckFileSize
```

PACE1000 不新增 Wails binding 方法；若实施时实际修改了 binding 签名，必须额外执行：

```powershell
cd projects/windlabx4/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
```

---

## Device Identity

| 项目 | 值 |
|---|---|
| Go 类型常量 | `DevicePACE1000` |
| 类型字面量 | `PACE1000` |
| UI 显示名 | `PACE1000` |
| 设备类别 | 单通道大气压力采集设备 |
| 传输 | Serial only |
| 自动发现 | 不支持 |
| 通道数 | 1 |
| 通道索引 | 0 |
| 通道名称 | 大气压力 |
| 工程单位 | Pa |
| 校零 | 禁止 |

PACE1000 与 DAQ-P-1604/Pre 的大气辅助通道语义相同，但设备身份、协议和通道布局独立，不复用 DAQ-P-1604 驱动或类型分支。

---

## Serial Contract

### 固定参数

连接时只允许用户选择串口名，其他串口参数采用固定默认值：

| 参数 | 值 | 来源 |
|---|---:|---|
| Baud rate | 9600 | LabVIEW VISA Configure Serial Port |
| Data bits | 8 | LabVIEW VISA Configure Serial Port |
| Parity | None | LabVIEW VISA Configure Serial Port |
| Stop bits | 1 | LabVIEW VISA Configure Serial Port |
| Flow control | None | LabVIEW VISA Configure Serial Port |
| Read timeout | 1 s | 复用 `serialport.DefaultConfig` |

`shared/device-sdk/go/serialport.DefaultConfig(serialPort)` 已提供 `9600-8-N-1`。实现时必须显式确认底层未启用软/硬流控；UI 不展示可编辑的波特率、数据位、校验位、停止位和流控字段。

### 查询周期

单次采集事务：

```text
StartAcquisition
  -> write ":sens?\r"
  -> wait 500 ms
  -> read all currently available response bytes
  -> parse one floating-point value
  -> pressurePa = parsedValue * 1000
  -> emit DataPayload{Channels: [pressurePa], ChannelIndices: [0]}
  -> repeat until StopAcquisition / Disconnect / fatal I/O error
```

约束：

1. 同一串口同一时刻只允许一个查询事务，禁止并发 Write/Read。
2. `StopAcquisition` 必须终止轮询，不关闭仍需保持连接的串口。
3. `Disconnect` 必须先取消轮询，再关闭串口；关闭动作必须能解除阻塞 Read。
4. 设备没有连续主动推流，未发送 `:sens?\r` 时不应产生 DataPayload。
5. 默认 `SamplingRate=2` Hz，与每次至少 500 ms 等待一致；若用户设置更高值，实际频率仍受 500 ms 事务下限约束，不并发补采样。

### 命令与响应

| 方向 | 内容 | 说明 |
|---|---|---|
| TX | `:sens?\r` | 查询命令以 CR（字节 `0x0D`）结尾，不发送 LF |
| RX | `%s%f` | 第一个字符串字段丢弃，第二个浮点字段作为压力原始值 |

解析契约：

1. 去除响应首尾 ASCII 空白和响应终止符。
2. 按 LabVIEW `Scan From String` 的 `%s%f` 语义扫描：第一个非空白字符串字段只作为占位并丢弃，第二个字段必须是浮点数。
3. 必须恰好存在两个字段；空响应、缺少字符串字段、缺少浮点字段和多余字段均为格式错误。
4. 数值必须是有限浮点数，拒绝 `NaN` 和 `Inf`。
5. 合法原始值 `raw` 转换为 `raw * 1000` Pa。
6. 转换结果应通过合理性校验；默认可接受大气压范围建议为 `[30000, 120000] Pa`，最终范围须结合设备量程确认。
7. 单次解析失败时丢弃整次采样，不发布 0 或上一次值。

示例按已确认的 `%s%f` 扫描语义说明：

```text
response = "PACE 101.325"
discarded string = "PACE"
parsed raw = 101.325
pressurePa = 101.325 * 1000 = 101325 Pa
```

### 错误策略

| 错误 | 行为 |
|---|---|
| 串口打开失败 | `Connect` 返回错误，状态为 Error/Disconnected（遵循 DeviceManager 现有约定） |
| Write 失败 | 终止采集循环，关闭损坏串口，触发 `ErrorNotifiable` |
| Read 超时/空响应 | 丢弃本次采样并计数，不发布数据 |
| 响应格式错误 | 丢弃本次采样并计数，不发布数据 |
| 非有限值/越界值 | 丢弃本次采样并计数，不发布数据 |
| 连续 3 次读取或解析失败 | 终止采集并通知 DeviceManager，避免 UI 长时间显示“正在采集”但无有效数据 |
| 手动 Stop | 正常停止，不记为设备错误 |
| 手动 Disconnect | 正常关闭，不自动重连 |

本期不设计自动重连；沿用 WindLabX4 的手动重连路径。后续如需自动重连，应独立补充状态机 spec。

---

## Data Contract

### Default Profile

```go
device.Profile{
	Type:         device.DevicePACE1000,
	Transport:    "serial",
	SerialPort:   "", // 用户选择，例如 COM3
	BaudRate:     9600,
	SamplingRate: 2,
	Channels: []device.ChannelConfig{
		{
			Index:              0,
			Name:               "大气压力",
			Enabled:            true,
			Unit:               "Pa",
			Precision:          1,
			RangeMin:           30000,
			RangeMax:           120000,
			SensorType:         device.SensorPressure,
			CalibrationEnabled: false,
		},
	},
}
```

`RangeMin/RangeMax` 是 UI 与异常检测建议值，不代表 PACE1000 硬件量程；真机量程确认后更新。

### DataPayload

每次合法响应发布一个 payload：

```go
device.DataPayload{
	DeviceID:       profile.ID,
	DeviceType:     device.DevicePACE1000,
	DeviceName:     profile.Name,
	Timestamp:      device.NowMs(),
	Channels:       []float64{pressurePa},
	ChannelIndices: []int{0},
}
```

时间戳使用主机完成解析时的 Unix 毫秒时间；PACE1000 未提供设备时间戳。

### Storage

复用现有单通道长格式录制路径，不新增专属宽格式 CSV。录制数据必须保留设备 ID、设备类型、时间戳、通道索引和 Pa 值。若现有 sink 根据类型选择格式，PACE1000 应明确落入通用长格式分支，并增加回归测试。

---

## Project Structure

### 新增文件

| 路径 | 作用 |
|---|---|
| `shared/device-sdk/go/daq/hardware/pace1000.go` | PACE1000 串口驱动、采集循环和生命周期 |
| `shared/device-sdk/go/daq/hardware/pace1000_test.go` | fake serial 单元测试 |
| `shared/device-sdk/go/protocol/pace1000.go` | 查询命令常量与纯响应解析函数 |
| `shared/device-sdk/go/protocol/pace1000_test.go` | 响应解析与换算测试 |
| `projects/windlabx4/services/api-go/internal/adapters/hardware/pace1000_adapter.go` | WindLabX4 thin adapter |
| `projects/windlabx4/services/api-go/internal/adapters/hardware/pace1000_adapter_test.go` | 类型翻译、sink 和错误回调测试 |

### 修改触点

| 路径 | 改动 |
|---|---|
| `shared/device-sdk/go/daq/core/types.go` | 新增 shared `DevicePACE1000` 类型 |
| `projects/windlabx4/services/api-go/internal/core/device/types.go` | 新增 `DevicePACE1000`；纳入大气通道/禁止校零判断 |
| `projects/windlabx4/services/api-go/internal/adapters/config/default_profiles.go` | PACE1000 默认 serial profile 与单通道 |
| `projects/windlabx4/services/api-go/internal/bootstrap/bootstrap.go` | 工厂注册 PACE1000 adapter |
| `projects/windlabx4/services/api-go/pkg/appcontext/context.go` | 工厂注册同步 |
| `projects/windlabx4/services/api-go/pkg/apiserver/apiserver.go` | 工厂注册同步 |
| `projects/windlabx4/services/api-go/pkg/types/types.go` | 导出设备类型别名 |
| `projects/windlabx4/apps/desktop-wails/frontend/src/api/types.ts` | `DeviceType` 联合新增 `PACE1000` |
| `projects/windlabx4/apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue` | 类型选项、serial-only 表单、单通道默认值 |
| `projects/windlabx4/apps/desktop-wails/frontend/src/utils/deviceCalibration.ts` | 明确 PACE1000 不支持校零 |
| 相关工厂/profile/frontend 测试 | 增加 PACE1000 覆盖 |

不修改 `ports.Device`，PACE1000 直接实现现有通用设备接口；不新增 PACE1000 配置接口、API endpoint 或 Wails binding。

---

## Code Style

协议解析保持纯函数，串口 I/O 留在 hardware 层：

```go
const PACE1000Query = ":sens?\r"

func ParsePACE1000Pressure(response []byte) (float64, error) {
	raw, err := parseConfirmedPACE1000Response(response)
	if err != nil {
		return 0, err
	}
	pressurePa := raw * 1000
	if math.IsNaN(pressurePa) || math.IsInf(pressurePa, 0) {
		return 0, ErrPACE1000InvalidPressure
	}
	return pressurePa, nil
}
```

`parseConfirmedPACE1000Response` 仅在拿到真机响应样例后定案，不用宽松正则猜测格式。

命名约定：

| 类型 | 约定 |
|---|---|
| Go device type | `DevicePACE1000` |
| Go driver | `PACE1000` |
| Go adapter | `PACE1000Adapter` |
| 文件名 | `pace1000.go` / `pace1000_adapter.go` |
| TypeScript literal | `'PACE1000'` |

---

## Testing Strategy

### Protocol Tests

在真机响应格式确认后固定测试样例：

1. 合法整数、小数和科学计数法响应按确认格式解析。
2. `101.325` 转换为 `101325 Pa`。
3. 首尾确认过的空白/终止符可被去除。
4. 空响应、非数字、多值、截断帧、`NaN`、`Inf` 返回错误。
5. 越界压力按最终量程规则返回错误。

### Driver Tests

使用可注入 fake serial transport，覆盖：

1. Connect 使用 `9600-8-N-1` 打开指定串口。
2. StartAcquisition 写入精确命令字节 `:sens?\r`（ASCII 十六进制：`3A 73 65 6E 73 3F 0D`）。
3. 写后等待 500 ms 才读取；测试用 fake clock，禁止真实 sleep 拖慢测试。
4. 合法响应发布单通道 payload，`Channels=[pressurePa]`、`ChannelIndices=[0]`。
5. StopAcquisition 停止后不再写命令，可再次启动。
6. Disconnect 取消阻塞读取并关闭串口。
7. ReadTimeout/空响应/格式错误不进入 DataSink。
8. 连续 3 次失败触发 error callback，状态不再是 Acquiring。
9. 手动 Stop/Disconnect 不触发 error callback。
10. fake serial 必须包含“读取阻塞、只在 Close 后返回”的 double，证明 Disconnect 可有界完成。

### WindLabX4 Integration Tests

1. 默认 profile 是 serial、9600 baud、单通道大气压力 Pa。
2. 三处设备工厂均能创建 PACE1000。
3. adapter 正确翻译 status 与 payload。
4. profile 保存/加载后 `serialPort`、`samplingRate` 与单通道配置不丢失。
5. CSV sink 对 PACE1000 使用通用单通道格式。
6. PACE1000 设备级和通道级校零入口均被拒绝或禁用。

### Frontend Tests

1. PACE1000 出现在设备类型下拉列表。
2. 选中后只展示串口选择，不展示 IP/网络端口或传输切换。
3. 波特率显示为固定 9600 或隐藏，不允许编辑为其他值。
4. 新建设备生成唯一“大气压力”通道，单位固定 Pa。
5. 校零按钮对该设备隐藏或禁用。
6. `npm run typecheck`、`npm run test`、`npm run build` 通过。

### Real Hardware Acceptance

1. 选择真实 COM 口后连接成功。
2. 抓取并保存 `:sens?\r` 的 TX/RX 原始十六进制日志，确认响应格式和响应终止符。
3. 连续采集 10 分钟，无串帧、无持续超时、无 goroutine 泄漏。
4. WindLabX4 显示值与 PACE1000 面板值对比，允许误差由设备显示精度决定。
5. 停止、再次启动、断开、再次连接均成功，串口不会被占用残留。

---

## Boundaries

### Always

1. PACE1000 永远只发布一个大气压力通道，工程单位固定 Pa。
2. 串口参数固定使用 `9600-8-N-1`、无流控。
3. 每次采样严格执行“写 `:sens?\r`（CR=`0x0D`，无 LF）-> 等待 500 ms -> 读响应”。
4. 解析失败的响应整次丢弃，禁止填 0、沿用旧值或发布部分值。
5. 串口事务串行执行；Stop/Disconnect 必须可取消阻塞 I/O。
6. shared SDK 不导入 WindLabX4 `internal/*`；WindLabX4 adapter 只做类型翻译。
7. PACE1000 不参与校零。
8. 非测试 Go 文件不超过 500 行，函数不超过 50 行。

### Ask First

1. 把查询命令改成 `:SENS?`、移除 CR 或追加 LF。
2. 把 `x1000` 换算改为其他倍率或允许用户选择输出单位。
3. 增加 PACE1000 自动发现、自动重连或专属配置 API。
4. 允许 PACE1000 多通道或参与校零。
5. 引入新的串口第三方库而不复用 shared `serialport`。

### Never

1. 在 frontend 或 Wails backend 直接访问串口。
2. 在 core/usecase 层导入串口或硬件实现。
3. 用正则从任意垃圾报文中寻找第一个数字并视为有效压力。
4. 因单次采样失败向 DataSink 发布 0 Pa。
5. Stop 后继续后台轮询，或 Disconnect 后遗留串口句柄。
6. 把 PACE1000 冒充 DAQ-P-1604 的第 17 通道。

---

## Success Criteria

| # | 条件 | 验证方式 |
|---:|---|---|
| 1 | 前后端类型统一为 `PACE1000` | Go build + TS typecheck |
| 2 | 默认 profile 为串口 9600 baud、单通道大气压力 Pa | profile 单元测试 |
| 3 | TX 精确为 `:sens?\r`（`3A 73 65 6E 73 3F 0D`） | fake serial + 真机抓包 |
| 4 | 每次查询等待 500 ms 后读取 | fake clock 测试 |
| 5 | 合法响应乘 1000 后发布单通道 Pa 值 | protocol + driver 测试 |
| 6 | 错误响应不进入 DataSink | driver 测试 |
| 7 | 连续失败后状态退出 Acquiring 并通知 DeviceManager | adapter/usecase 测试 |
| 8 | Stop/Disconnect 可解除阻塞读取并释放 COM 口 | blocking fake + 真机重连 |
| 9 | UI 仅展示串口配置和一个固定通道 | frontend test + 手工验收 |
| 10 | PACE1000 不可校零 | backend + frontend 测试 |
| 11 | 录制文件包含单通道 Pa 数据 | storage 集成测试 |
| 12 | shared SDK、WindLabX4 backend/frontend 全部验证命令通过 | CI/本地验证 |

---

## Open Questions

查询命令终止符已经确认：发送 `:sens?\r`，CR 为 `0x0D`，末尾不跟 LF。

以下一项在真机验收时确认，不阻塞 `%s%f` 解析器实现：

1. **响应终止符和首字段内容**：确认真实响应的行终止符，以及 `%s` 字段的实际文本。字段结构和浮点位置已由 LabVIEW `%s%f` 确定；实现不依赖首字段具体名称。

暂定决策（真机证据可覆盖）：

| 项目 | 暂定值 |
|---|---|
| 原始数值含义 | 由 `%s%f` 中的 `%f` 字段提供，乘 1000 后为 Pa |
| WindLabX4 输出 | `raw * 1000` Pa |
| 默认软件采样率 | 2 Hz |
| 查询后等待 | 500 ms |
| 读取超时 | 1 s |
| 连续失败阈值 | 3 次 |
| 自动重连 | 本期不实现 |
| 自动发现 | 不实现 |
