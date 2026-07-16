# Implementation Plan: MPS-03 集成到 wind-daq

> **配套 spec**：[spec-mps03-integration.md](./spec-mps03-integration.md)
> **方法论**：`planning-and-task-breakdown` skill — 依赖图 + 垂直切片 + 显式 AC + Checkpoint
> **任务清单**：[tasks-mps03-integration.md](./tasks-mps03-integration.md)（Phase 3 产出）

---

## Overview

将 MPS-03 多功能探针设备作为新 DAQ 设备类型集成到 wind-daq。采用 shared SDK + thin wrapper 双层架构：协议层和驱动层放 `shared/device-sdk/go/`，wind-daq 内做类型翻译。集成范围涵盖 TCP 通信、9 项硬件配置同步、16 通道 CSV 数据流解析、断连重连状态机、专属 CSV 17 列表头、配置 UI 面板、HTTP/Wails API。

实现遵循依赖图自底向上：协议层 → 驱动层 → 适配器 → API → 前端 → Wails binding。

---

## Architecture Decisions

| # | 决策 | 理由 | 替代方案（已否决） |
|---|---|---|---|
| AD1 | canonical type = `"MPS03"`（无连字符） | 与 `"DAQ-P-1603"` 视觉区分，避免 grep 误匹配 | `"MPS-03"`（与 P-1603 风格混淆） |
| AD2 | shared SDK + thin wrapper 双层 | 跨项目复用（参照 DAQ-P-1603 模板 C）；shared SDK 禁止依赖 wind-daq internal | 仅 wind-daq adapter（无法复用） |
| AD3 | 命令响应用"超时 + 响应识别"机制 | SET/GET/SAVE 响应不带 `\r\n`，无法用行解析器 | `bufio.ReadLine`（错误，会阻塞） |
| AD4 | 采集期间禁止 #SET/#GET/#SAVE | 数据流与命令响应在同一 TCP 流混杂，无法可靠区分 | 复用 readLoop 偷空发命令（不可靠） |
| AD5 | 重连状态机内嵌 shared SDK 驱动层 | 状态机与设备生命周期紧耦合；DeviceManager 无重连能力 | wind-daq adapter 层重连（需跨层协调） |
| AD6 | 错误帧丢弃 + 计数，禁止填 0 | 风洞测量数据填 0 会掩盖通信损坏 | 填 0 兜底（危险） |
| AD7 | CHA != FFFF 时 DataPayload 仅含启用通道 | 避免与真实零值混淆；ChannelIndices 表达原始索引 | 用 0 填充禁用通道（语义模糊） |
| AD8 | CSV 17 列固定表头（Timestamp + 16 ASCII 列名） | 结构稳定，便于 Excel/pandas 解析；禁用通道填空字符串 | 动态列（CHA 变化导致表头不稳定） |
| AD9 | DELAY 是权威值，samplingRate 只读派生 | 设备 DELAY 是协议实际参数；Hz 是 UI 便利展示 | Hz 双向绑定（非整除导致循环） |
| AD10 | 9 项硬件配置全部可编辑（采集时禁用） | 完整暴露协议能力，UI 用 disabled 约束 | 仅暴露 6 项（隐藏 BIN/HEAD/CNUM 透传） |

---

## Dependency Graph

```
                    ┌─────────────────────────────────────────┐
                    │  Phase 1: shared SDK 协议层（基础）      │
                    │  T1: MPS03CommandClient                  │
                    │  T2: ParseCSVLine + TMODE/TCHO 转换      │
                    └────────────┬────────────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────────────────────┐
                    │  Phase 2: shared SDK 驱动层             │
                    │  T3: shared core/ports 扩展              │
                    │  T4: MPS03Device 主体（依赖 T1/T2/T3）   │
                    │  T5: 重连状态机（依赖 T4）               │
                    │  T6: MPS-03 模拟器（依赖 T1/T2）         │
                    └────────────┬────────────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────────────────────┐
                    │  Phase 3: wind-daq 项目层               │
                    │  T7: wind-daq core/ports 镜像            │
                    │  T8: MPS03Adapter thin wrapper（依赖 T4）│
                    │  T9: default_profiles + bootstrap 注册   │
                    │      （依赖 T7/T8）                       │
                    │  T10: HTTP API 端点（依赖 T8/T9）        │
                    │  T11: CSV sink 17 列表头（依赖 T7）      │
                    │  T12: API 集成测试（依赖 T10/T11）       │
                    └────────────┬────────────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────────────────────┐
                    │  Phase 4: 前端                           │
                    │  T13: types.ts + deviceApi.ts（依赖 T10）│
                    │  T14: Mps03ConfigPanel.vue（依赖 T13）   │
                    │  T15: DeviceManagementDrawer 分支        │
                    │       （依赖 T13/T14）                    │
                    │  T16: deviceStore.ts MPS03 操作          │
                    │       （依赖 T13）                        │
                    └────────────┬────────────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────────────────────┐
                    │  Phase 5: Wails binding + 验证           │
                    │  T17: Wails binding + 同步（依赖 T10）   │
                    │  T18: 端到端验证（依赖全部）              │
                    └─────────────────────────────────────────┘
```

**可并行任务**：
- T1 与 T2 可并行（同属协议层，无相互依赖）
- T6 与 T4/T5 可并行（模拟器只依赖协议层）
- T11 与 T10 可并行（CSV sink 与 HTTP API 独立）
- T14、T15、T16 在 T13 完成后可并行

---

## Task List

### Phase 1: shared SDK 协议层

#### Task 1: MPS03CommandClient 命令收发客户端

**Description**：实现 MPS-03 命令收发客户端，处理 SET/GET/SAVE 响应（不带换行符）的识别、半包/粘包、超时。响应识别按 spec §协议契约 §1 的 7 条规则实现。

**Acceptance criteria**:
- [ ] `Send(cmd string) (string, error)` 方法实现，按 7 条规则识别响应
- [ ] 半包：响应分两次 Read 到达时能正确拼接
- [ ] 粘包：两个响应合并到一个 Read 时能正确分割
- [ ] 命令回显（`#` 开头）被跳过，继续等待真实响应
- [ ] 含逗号的响应识别为采集数据行，返回 `ErrCommandDuringAcquisition`
- [ ] 超时（3s）返回 `ErrCommandTimeout`
- [ ] `ioMu` 串行化命令发送
- [ ] `recvBuffer` 剩余字节保留供下次使用

**Verification**:
- [ ] `cd shared/device-sdk/go && go test ./protocol/... -run MPS03 -v`
- [ ] 测试用例覆盖：A 响应、E/I 响应、数字响应、十六进制响应、回显跳过、半包、粘包、超时、数据行误判

**Dependencies**: None

**Files likely touched**:
- `shared/device-sdk/go/protocol/mps03_frame.go`（新增）
- `shared/device-sdk/go/protocol/mps03_frame_test.go`（新增，T1 部分用例）

**Estimated scope**: S（1-2 文件）

---

#### Task 2: ParseCSVLine + TMODE/TCHO 转换

**Description**：实现 CSV 数据行解析（HEAD=0/1 两种格式）和 TMODE（十进制 ↔ 十六进制）、TCHO（字母 ↔ 数字）的双向转换函数。错误帧返回明确错误，禁止填 0。

**Acceptance criteria**:
- [ ] `ParseCSVLine(line string, headEnabled bool) ([16]float64, error)` 实现
- [ ] HEAD=1 时跳过首字段序号，返回 16 通道值
- [ ] HEAD=0 时直接解析 16 字段
- [ ] 字段数 != 预期 → `ErrInvalidFieldCount`
- [ ] 单字段 parseFloat 失败 → `ErrInvalidFieldValue`
- [ ] `TmodeToHexString(tmode int) string`：17 → "11"
- [ ] `TmodeFromHexString(s string) (int, error)`："11" → 17
- [ ] `TchoToSetString(tcho string) (string, error)`："E" → "1"，"I" → "0"
- [ ] `TchoFromGetResponse(s string) (string, error)`："E" → "E"，"I" → "I"

**Verification**:
- [ ] `cd shared/device-sdk/go && go test ./protocol/... -run "ParseCSVLine|Tmode|Tcho" -v`
- [ ] 测试用例覆盖：HEAD=0/1 合法解析、字段数不足、字段数超长、非数字字段、TMODE 边界值、TCHO 非法值

**Dependencies**: None（与 T1 并行）

**Files likely touched**:
- `shared/device-sdk/go/protocol/mps03_frame.go`（同 T1 文件，函数追加）
- `shared/device-sdk/go/protocol/mps03_frame_test.go`（同 T1 文件，用例追加）

**Estimated scope**: S（1-2 文件）

---

### Checkpoint: Phase 1 协议层完成

- [ ] `cd shared/device-sdk/go && go build ./... && go test ./protocol/... && go vet ./...`
- [ ] 协议层无 wind-daq 依赖（`grep -r "wind-daq" shared/device-sdk/go/protocol/` 无命中）
- [ ] Review with human before proceeding to Phase 2

---

### Phase 2: shared SDK 驱动层

#### Task 3: shared SDK core/ports 扩展

**Description**：在 shared SDK 的 `daq/core/types.go` 新增 `DeviceMPS03` 类型常量、`MPS03HardwareConfig` 结构体、`Profile.MPS03Config` 字段；在 `daq/ports/device.go` 新增 `MPS03Configurable` 接口。

**Acceptance criteria**:
- [ ] `core.Type` 新增 `DeviceMPS03 Type = "MPS03"`
- [ ] `MPS03HardwareConfig` 结构体含 9 字段（Avg/Bin/Delay/Cnum/Head/Cha/Tmode/Ttype/Tcho）+ JSON tag
- [ ] `Profile` 结构体新增 `MPS03Config *MPS03HardwareConfig` 字段
- [ ] `ports.MPS03Configurable` 接口定义：`GetHardwareConfig(forceRefresh bool) (core.MPS03HardwareConfig, error)` + `ApplyHardwareConfig(core.MPS03HardwareConfig) (core.MPS03HardwareConfig, error)`
- [ ] 编译通过：`cd shared/device-sdk/go && go build ./...`

**Verification**:
- [ ] `cd shared/device-sdk/go && go build ./... && go vet ./...`
- [ ] `grep -r "DAQP1603" shared/device-sdk/go/daq/core/ shared/device-sdk/go/daq/ports/` 无新增命中（命名隔离）

**Dependencies**: None（与 T1/T2 并行，但建议在 T4 前完成）

**Files likely touched**:
- `shared/device-sdk/go/daq/core/types.go`（修改）
- `shared/device-sdk/go/daq/ports/device.go`（修改）

**Estimated scope**: S（2 文件）

---

#### Task 4: MPS03Device 驱动主体

**Description**：实现 shared SDK 真实驱动 `MPS03Device`：TCP 连接、9 项 #GET 配置同步、采集循环（readLoop）、#SET BIN 0 + #START / #STOP、DataPayload 组装、错误帧计数、onError 回调。采集期间禁止配置命令。

**Acceptance criteria**:
- [ ] `MPS03Device` 实现 `ports.Device` + `ports.MPS03Configurable` + `ports.ErrorNotifiable`（编译期断言）
- [ ] `Connect()` 建立 TCP + 发送 9 个 #GET + 触发 onConfigSynced，不发送任何 #SET
- [ ] `Disconnect()` 设置 manualDisconnect 标志，关闭 conn，等待 readLoop 退出（1s 超时）
- [ ] `StartAcquisition()` 发送 `#SET BIN 0` + `#START`，启动 readLoop，状态切换为 Acquiring
- [ ] `StopAcquisition()` 发送 `#STOP`（不等响应），停止 readLoop，状态切换为 Connected
- [ ] `#START` 后 2 倍 DELAY 无数据 → `ErrStartNoData`
- [ ] readLoop 解析 CSV 行，错误帧丢弃 + `consecutiveParseErrors++`，不进入 sink
- [ ] 连续 50 帧解析错误 → 触发 onError + Error 态
- [ ] 成功解析一帧 → `consecutiveParseErrors` 重置为 0
- [ ] `GetHardwareConfig(true)` 强制刷新（9 个 #GET），`GetHardwareConfig(false)` 用缓存
- [ ] `ApplyHardwareConfig(config)` 仅在非采集状态执行，完成后自动 #SAVE
- [ ] 采集期间调用 Get/Apply → 返回 `ErrAcquiringNotAllowConfig`
- [ ] DataPayload.Channels 仅含启用通道值，ChannelIndices 含原始索引（CHA 契约）
- [ ] per-id 互斥锁 `ioMu` 串行化命令发送

**Verification**:
- [ ] `cd shared/device-sdk/go && go test ./daq/hardware/... -run MPS03 -v`
- [ ] mock TCP server 测试用例：连接成功、9 个 #GET 完成、采集启停、错误帧丢弃、采集期间配置被拒
- [ ] `go vet ./...` 通过

**Dependencies**: T1, T2, T3

**Files likely touched**:
- `shared/device-sdk/go/daq/hardware/mps03.go`（新增）
- `shared/device-sdk/go/daq/hardware/mps03_test.go`（新增）

**Estimated scope**: M（2 文件，但代码量大）

---

#### Task 5: 重连状态机

**Description**：在 MPS03Device 中实现重连状态机：指数退避（1/2/4/8/16s × 5 次）、恢复逻辑（9 个 #GET + 条件 #START）、manualDisconnect 标志、旧连接清理。

**Acceptance criteria**:
- [ ] TCP Read 错误触发重连状态机（非 manualDisconnect 时）
- [ ] 退避序列：1s, 2s, 4s, 8s, 16s（最多 5 次）
- [ ] 重连期间状态为 `ConnectionError`，LastError 记录最近错误
- [ ] 重连成功 → 重新执行 9 个 #GET
- [ ] 断连前 acquiring == true → 重连后重新 #SET BIN 0 + #START，状态切换为 Acquiring
- [ ] 断连前 acquiring == false → 状态切换为 Connected
- [ ] 重连后 `consecutiveParseErrors` 重置为 0
- [ ] 5 次重连失败 → Error 态，不再自动重连，调用 onError
- [ ] manualDisconnect == true 时禁止自动重连
- [ ] 旧连接清理：conn.Close() + 等待 readDone（1s 超时）+ 超时强制 Close

**Verification**:
- [ ] `cd shared/device-sdk/go && go test ./daq/hardware/... -run MPS03Reconnect -v`
- [ ] 测试用例：TCP 断连触发重连、5 次内重连成功、5 次失败、manualDisconnect 禁止重连、断连前采集→重连后恢复
- [ ] `go vet ./...` 通过

**Dependencies**: T4

**Files likely touched**:
- `shared/device-sdk/go/daq/hardware/mps03.go`（同 T4 文件，追加状态机）
- `shared/device-sdk/go/daq/hardware/mps03_test.go`（同 T4 文件，追加用例）

**Estimated scope**: M（2 文件）

---

#### Task 6: MPS-03 模拟器

**Description**：基于 `shared/device-sdk/go/testing/sim/simulator.go` 框架实现 MPS-03 模拟器，支持 9 个 #GET 响应回放、#SET 更新内部状态、#START 后按 DELAY 推送 CSV 数据、#STOP 停止、#SAVE 返回 A、CHA != FFFF 时按启用通道输出。

**Acceptance criteria**:
- [ ] `MPS03Simulator` 实现 `ProtocolHandler` 接口
- [ ] 9 个 #GET 命令返回当前内部配置
- [ ] #SET 命令更新内部配置，返回 A
- [ ] #SAVE 返回 A
- [ ] #START 后按 DELAY 间隔推送 CSV 行（HEAD=1 时含序号）
- [ ] #STOP 立即停止推送
- [ ] CHA != FFFF 时仅输出启用通道字段（验证 §CHA 契约假设）
- [ ] BIN=1 模式不支持（返回错误或忽略）
- [ ] 模拟器可独立启动监听指定端口

**Verification**:
- [ ] `cd shared/device-sdk/go && go test ./testing/sim/... -run MPS03 -v`
- [ ] 测试用例：9 个 #GET 响应正确、#START 后数据推送、#STOP 停止、CHA 掩码生效
- [ ] 手动启动模拟器 + wind-daq 前端可完成全流程（Phase 5 验证）

**Dependencies**: T1, T2（与 T4/T5 并行）

**Files likely touched**:
- `shared/device-sdk/go/testing/sim/mps03_sim.go`（新增）
- `shared/device-sdk/go/testing/sim/mps03_sim_test.go`（新增）

**Estimated scope**: M（2 文件）

---

### Checkpoint: Phase 2 shared SDK 驱动层完成

- [ ] `cd shared/device-sdk/go && go build ./... && go test ./... && go vet ./...`
- [ ] shared SDK 全部测试通过（含协议层、驱动层、模拟器）
- [ ] `grep -r "wind-daq" shared/device-sdk/go/` 无命中（依赖方向验证）
- [ ] `grep -r "DAQP1603" shared/device-sdk/go/daq/hardware/mps03*.go shared/device-sdk/go/protocol/mps03*.go` 无命中（命名隔离）
- [ ] Review with human before proceeding to Phase 3

---

### Phase 3: wind-daq 项目层

#### Task 7: wind-daq core/ports 镜像

**Description**：在 wind-daq 的 `core/device/types.go` 和 `ports/device.go` 镜像 shared SDK 的 MPS-03 类型与接口定义，作为 adapter 翻译的本地类型。

**Acceptance criteria**:
- [ ] `device.Type` 新增 `DeviceMPS03 Type = "MPS03"`
- [ ] `device.MPS03HardwareConfig` 结构体（9 字段，与 shared SDK 镜像）
- [ ] `device.Profile` 新增 `MPS03Config *MPS03HardwareConfig` 字段
- [ ] `ports.MPS03Configurable` 接口定义（与 shared SDK 镜像，但用 wind-daq 类型）
- [ ] 编译通过：`cd projects/wind-daq/services/api-go && go build ./...`

**Verification**:
- [ ] `cd projects/wind-daq/services/api-go && go build ./... && go vet ./...`
- [ ] `grep -r "DeviceMPS03\|MPS03Configurable\|MPS03HardwareConfig" projects/wind-daq/services/api-go/internal/core/ projects/wind-daq/services/api-go/internal/ports/` 命中位置正确

**Dependencies**: T3（shared SDK 类型定义先行）

**Files likely touched**:
- `projects/wind-daq/services/api-go/internal/core/device/types.go`（修改）
- `projects/wind-daq/services/api-go/internal/ports/device.go`（修改）

**Estimated scope**: S（2 文件）

---

#### Task 8: MPS03Adapter thin wrapper

**Description**：实现 wind-daq 适配器 `MPS03Adapter`，委托给 shared SDK 的 `MPS03Device`，负责 wind-daq Profile ↔ shared SDK Profile、Status、DataPayload 的双向类型翻译。

**Acceptance criteria**:
- [ ] `MPS03Adapter` 实现 `ports.Device` + `ports.MPS03Configurable` + `ports.ErrorNotifiable`（编译期断言）
- [ ] `NewMPS03Adapter(profile device.Profile) *MPS03Adapter` 构造函数
- [ ] `Connect()` 翻译 profile → shared profile，调用 driver.Connect，翻译 status 回来
- [ ] `Disconnect()` 委托 driver.Disconnect
- [ ] `StartAcquisition()` / `StopAcquisition()` 委托 + 状态翻译
- [ ] `SetDataSink(sink)` 包装 wind-daq sink 为 shared sink 并注册到 driver
- [ ] `SetOnError(cb)` 委托 driver.SetOnError
- [ ] `Status()` 翻译 shared status → wind-daq status
- [ ] `GetHardwareConfig` / `ApplyHardwareConfig` 委托 + 类型翻译
- [ ] DataPayload 翻译：shared.DataPayload → wind-daq.DataPayload（含 Channels/ChannelIndices）

**Verification**:
- [ ] `cd projects/wind-daq/services/api-go && go test ./internal/adapters/hardware/... -run MPS03 -v`
- [ ] `go vet ./...` 通过
- [ ] 类型翻译测试：Profile 双向转换、Status 转换、DataPayload 转换

**Dependencies**: T4, T7

**Files likely touched**:
- `projects/wind-daq/services/api-go/internal/adapters/hardware/mps03_adapter.go`（新增）
- `projects/wind-daq/services/api-go/internal/adapters/hardware/mps03_adapter_test.go`（新增，可选）

**Estimated scope**: M（2 文件）

---

#### Task 9: default_profiles + bootstrap 注册

**Description**：在 `default_profiles.go` 新增 MPS03 默认 profile 工厂分支（默认 IP 192.168.1.9 / 端口 9000 / 16 通道定义）和 `NormalizeProfile` MPS03 规范化；在 `bootstrap.go` 的 `deviceFactory.Create` switch 新增 `case device.DeviceMPS03`。

**Acceptance criteria**:
- [ ] `NewDefaultProfile(device.MPS03)` 返回默认 profile（IP/端口/16 通道）
- [ ] 16 通道定义含语义化 Name（Alpha/Beta/MachNumber/...）和 Enabled=true
- [ ] `NormalizeProfile` 对 MPS03 类型执行规范化（CHA 大写、TCHO 合法值校验等）
- [ ] `deviceFactory.Create` switch 新增 `case device.DeviceMPS03: return NewMPS03Adapter(profile), nil`
- [ ] 默认 MPS03HardwareConfig（AVG=8/BIN=0/DELAY=1000/CNUM=0/HEAD=1/CHA="FFFF"/TMODE=17/TTYPE=2/TCHO="E"）
- [ ] 编译通过

**Verification**:
- [ ] `cd projects/wind-daq/services/api-go && go build ./... && go vet ./...`
- [ ] 单元测试：`NewDefaultProfile(MPS03)` 返回正确默认值
- [ ] 单元测试：`NormalizeProfile` 对非法 CHA/TCHO 返回错误

**Dependencies**: T7, T8

**Files likely touched**:
- `projects/wind-daq/services/api-go/internal/adapters/config/default_profiles.go`（修改）
- `projects/wind-daq/services/api-go/internal/bootstrap/bootstrap.go`（修改）

**Estimated scope**: S（2 文件）

---

#### Task 10: HTTP API 端点

**Description**：在 `api/server.go` 新增 MPS03 配置端点：GET/PUT `/api/device/{id}/mps03-config`，实现 409（采集中）/400（校验失败）/502（设备拒绝）/503（未连接）/504（超时）状态码，并在 `/api/storage/start` 收集 MPS03 channels 注入 `RecordingConfig.DeviceChannels`。

**Acceptance criteria**:
- [ ] `GET /api/device/{id}/mps03-config?forceRefresh=true|false` 返回当前配置
- [ ] `PUT /api/device/{id}/mps03-config` 应用配置（自动 #SAVE）
- [ ] 采集期间 GET/PUT 返回 409 Conflict
- [ ] 校验失败返回 400 Bad Request（含具体错误消息）
- [ ] 设备未连接返回 503
- [ ] #SET/#SAVE 失败返回 502
- [ ] 超时返回 504
- [ ] `/api/storage/start` 收集 MPS03 profile 的 channels 填入 `DeviceChannels`
- [ ] 路由注册在 `handleDeviceByID` 或独立 handler

**Verification**:
- [ ] `cd projects/wind-daq/services/api-go && go test ./api/... -run MPS03 -v`
- [ ] 测试用例：GET 成功、PUT 成功、PUT 采集中 409、PUT 校验失败 400、GET 设备未连接 503

**Dependencies**: T8, T9

**Files likely touched**:
- `projects/wind-daq/services/api-go/api/server.go`（修改）

**Estimated scope**: M（1 文件，但改动量大）

---

#### Task 11: CSV sink 17 列表头分支

**Description**：在 `csv_sink.go` 的 `buildDynamicHeader` 新增 MPS03 分支，输出固定 17 列表头（Timestamp + 16 ASCII 列名 + 单位后缀），禁用通道列填空字符串。

**Acceptance criteria**:
- [ ] `buildDynamicHeader` 新增 `case device.DeviceMPS03:` 分支
- [ ] 表头为 17 列：`Timestamp,Alpha_deg,Beta_deg,MachNumber,Velocity_mps,Vx_mps,Vy_mps,Vz_mps,Sensor1,Sensor2,Sensor3,Sensor4,Sensor5,Sensor6,TotalPressure_kPa,ExtTemp_degC,IntTemp_degC`
- [ ] 数据行禁用通道列填空字符串（不是 0）
- [ ] CSV 编码 UTF-8 with BOM
- [ ] 行尾 `\r\n`
- [ ] 不影响其他设备的表头分支

**Verification**:
- [ ] `cd projects/wind-daq/services/api-go && go test ./internal/adapters/storage/... -run MPS03 -v`
- [ ] 测试用例：MPS03 表头 17 列正确、禁用通道填空字符串
- [ ] `go vet ./...` 通过

**Dependencies**: T7（与 T10 并行）

**Files likely touched**:
- `projects/wind-daq/services/api-go/internal/adapters/storage/csv_sink.go`（修改）

**Estimated scope**: S（1 文件）

---

#### Task 12: API 集成测试

**Description**：在 `api/server_test.go` 新增 MPS03 集成测试用例，覆盖 GET/PUT 全流程 + 状态码 + 采集期间禁止配置 + CSV 录制 17 列。

**Acceptance criteria**:
- [ ] 测试用例：连接 → GET 配置 → 修改 → PUT → 重读验证
- [ ] 测试用例：采集期间 PUT 返回 409
- [ ] 测试用例：校验失败返回 400（每项字段一个用例）
- [ ] 测试用例：CSV 录制 17 列表头正确
- [ ] 测试用例：CHA != FFFF 时 DataPayload 仅含启用通道
- [ ] 使用 mock TCP server 或 MPS-03 模拟器

**Verification**:
- [ ] `cd projects/wind-daq/services/api-go && go test ./api/... -run MPS03 -v`
- [ ] 全部测试通过
- [ ] `go vet ./...` 通过

**Dependencies**: T10, T11

**Files likely touched**:
- `projects/wind-daq/services/api-go/api/server_test.go`（修改或新增 MPS03 用例段）

**Estimated scope**: M（1 文件，但用例多）

---

### Checkpoint: Phase 3 wind-daq 项目层完成

- [ ] `cd projects/wind-daq/services/api-go && go build ./... && go test ./... && go vet ./...`
- [ ] wind-daq 全部测试通过
- [ ] `grep -r "DAQP1603" projects/wind-daq/services/api-go/internal/adapters/hardware/mps03*.go` 无命中
- [ ] Review with human before proceeding to Phase 4

---

### Phase 4: 前端

#### Task 13: types.ts + deviceApi.ts

**Description**：在前端 `api/types.ts` 新增 `DeviceType` 联合字面量 `'MPS03'` + `MPS03HardwareConfig` 接口；在 `api/deviceApi.ts` 新增 `getMps03Config` / `applyMps03Config` 函数。

**Acceptance criteria**:
- [ ] `DeviceType` 联合类型新增 `'MPS03'`
- [ ] `MPS03HardwareConfig` 接口含 9 字段（与后端 JSON tag 对齐）
- [ ] `getMps03Config(id: string, forceRefresh?: boolean): Promise<MPS03HardwareConfig>`
- [ ] `applyMps03Config(id: string, config: Partial<MPS03HardwareConfig>): Promise<MPS03HardwareConfig>`
- [ ] 错误处理：409/400/502/503/504 抛出含状态码的 Error
- [ ] `npm run typecheck` 通过

**Verification**:
- [ ] `cd projects/wind-daq/apps/desktop-wails/frontend && npm run typecheck`
- [ ] `npm run build` 通过

**Dependencies**: T10（API 端点先行，提供契约）

**Files likely touched**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/types.ts`（修改）
- `projects/wind-daq/apps/desktop-wails/frontend/src/api/deviceApi.ts`（修改）

**Estimated scope**: S（2 文件）

---

#### Task 14: Mps03ConfigPanel.vue

**Description**：实现 MPS-03 配置面板组件，9 项硬件配置可编辑，采集状态下禁用，DELAY 修改时实时计算 Hz 只读展示。

**Acceptance criteria**:
- [ ] 9 项配置全部有 UI 控件（AVG 下拉、DELAY 输入、CHA 输入、TMODE 输入、TTYPE 下拉、TCHO 单选、BIN/HEAD/CNUM 透传）
- [ ] `acquiring == true` 时所有输入 disabled
- [ ] DELAY 修改时实时显示 `采样率: {round(1000/delay)} Hz`
- [ ] AVG 合法值 4/8/32/64 下拉
- [ ] TTYPE 合法值 0-8 下拉（K/B/E/J/T/S/N/R/C）
- [ ] TCHO 单选 E/I
- [ ] CHA 输入框 4 位十六进制
- [ ] 校验失败时 UI 提示（红色边框 + 错误消息）
- [ ] 保存按钮调用 `applyMps03Config`
- [ ] 使用设计 token（var(--xx)），禁止硬编码颜色

**Verification**:
- [ ] `npm run typecheck && npm run build`
- [ ] 手动 UI 验证：编辑 → 保存 → 重读一致
- [ ] 手动 UI 验证：采集时输入 disabled

**Dependencies**: T13

**Files likely touched**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/Mps03ConfigPanel.vue`（新增）

**Estimated scope**: M（1 文件，但组件复杂）

---

#### Task 15: DeviceManagementDrawer.vue MPS03 分支

**Description**：在设备管理抽屉中新增 MPS03 类型分支：设备类型下拉含 MPS-03 选项、默认值加载、配置面板加载/保存、采样率输入框隐藏、DELAY 编辑入口、Hz 只读展示、校零跳过 T_ext/T_int。

**Acceptance criteria**:
- [ ] 设备类型下拉新增 "MPS-03" 选项
- [ ] 选择 MPS-03 时默认 IP 192.168.1.9 / 端口 9000
- [ ] 通用 samplingRate 输入框对 MPS-03 隐藏
- [ ] MPS03ConfigPanel 嵌入抽屉
- [ ] 连接成功后自动加载配置（getMps03Config）
- [ ] 设备卡片显示派生 Hz（只读）
- [ ] 校零通道列表排除 T_ext/T_int（通道 14/15）

**Verification**:
- [ ] `npm run typecheck && npm run build`
- [ ] 手动 UI 验证：添加 MPS-03 → 连接 → 配置 → 采集 → 录制全流程

**Dependencies**: T13, T14

**Files likely touched**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue`（修改）

**Estimated scope**: M（1 文件，改动点多）

---

#### Task 16: deviceStore.ts MPS03 操作

**Description**：在 Pinia store 中新增 MPS03 配置加载/保存操作，校零跳过 T_ext/T_int 通道逻辑。

**Acceptance criteria**:
- [ ] `loadMps03Config(deviceId)` action 调用 deviceApi.getMps03Config
- [ ] `saveMps03Config(deviceId, config)` action 调用 deviceApi.applyMps03Config
- [ ] 配置加载失败时 UI 显示错误提示
- [ ] 保存成功后更新本地 profile.mps03Config
- [ ] 校零通道列表过滤：MPS03 类型排除通道 14/15（T_ext/T_int）
- [ ] `npm run typecheck` 通过

**Verification**:
- [ ] `npm run typecheck && npm run build`

**Dependencies**: T13（与 T14/T15 并行）

**Files likely touched**:
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/deviceStore.ts`（修改）

**Estimated scope**: S（1 文件）

---

### Checkpoint: Phase 4 前端完成

- [ ] `cd projects/wind-daq/apps/desktop-wails/frontend && npm run typecheck && npm run build`
- [ ] 前端全部构建通过
- [ ] 手动 UI 验证：MPS-03 添加 → 连接 → 配置 → 采集 → 录制全流程（用模拟器）
- [ ] Review with human before proceeding to Phase 5

---

### Phase 5: Wails binding + 验证

#### Task 17: Wails binding 新增 + 同步

**Description**：在 `backend/app.go` 新增 `DeviceGetMps03Config` / `DeviceApplyMps03Config` binding 方法，执行 `wails3 generate bindings -silent` 同步前端 TS 声明。

**Acceptance criteria**:
- [ ] `DeviceGetMps03Config(id string, forceRefresh bool) (MPS03ConfigResult, error)` 实现
- [ ] `DeviceApplyMps03Config(id string, config MPS03HardwareConfig) (MPS03ConfigResult, error)` 实现
- [ ] `MPS03ConfigResult` 结构体定义（Success/Error/Data）
- [ ] 委托给 DeviceManager → MPS03Adapter
- [ ] `wails3 generate bindings -silent` 执行成功
- [ ] 前端 TS 声明文件更新
- [ ] `npm run typecheck` 通过

**Verification**:
- [ ] `cd projects/wind-daq/apps/desktop-wails && wails3 generate bindings -silent`
- [ ] `cd projects/wind-daq/services/api-go && go build ./... && go vet ./...`
- [ ] `cd projects/wind-daq/apps/desktop-wails/frontend && npm run typecheck && npm run build`

**Dependencies**: T10

**Files likely touched**:
- `projects/wind-daq/apps/desktop-wails/backend/app.go`（修改）
- 自动生成的 TS binding 文件

**Estimated scope**: S（1 文件 + 自动生成）

---

#### Task 18: 端到端验证

**Description**：使用 MPS-03 模拟器 + wind-daq 完成端到端验证，覆盖 spec 全部 16 项成功标准，并在真实硬件上验证 CHA != FFFF 契约。

**Acceptance criteria**:
- [ ] 模拟器端到端：添加 → 连接 → 9 项 #GET → UI 显示 → 修改配置 → #SAVE → 重连验证
- [ ] 模拟器端到端：启动采集 → 波形图 16 通道 → 停止采集
- [ ] 模拟器端到端：录制 CSV → 17 列表头正确
- [ ] 模拟器端到端：CHA != FFFF → DataPayload 仅含启用通道 → CSV 禁用列空字符串
- [ ] 模拟器端到端：拔网线 → 重连状态机 → 恢复采集
- [ ] 模拟器端到端：注入错误帧 → 不进 sink → 连续 50 帧触发 Error
- [ ] 真实硬件验证：CHA != FFFF 时 CSV 字段顺序（TODO-HW-VERIFY 项）
- [ ] 真实硬件验证：TMODE 十六进制 SET / 十进制 GET 转换正确
- [ ] 真实硬件验证：TCHO 字母 GET / 数字 SET 转换正确
- [ ] shared SDK + wind-daq 双模块 `go test ./... && go vet ./...` 全绿
- [ ] `npm run typecheck && npm run build` 全绿
- [ ] `validate-structure.ps1` 全绿

**Verification**:
- [ ] `cd shared/device-sdk/go && go test ./... && go vet ./...`
- [ ] `cd projects/wind-daq/services/api-go && go test ./... && go vet ./...`
- [ ] `cd projects/wind-daq/apps/desktop-wails/frontend && npm run typecheck && npm run build`
- [ ] `.\validate-structure.ps1`
- [ ] `.\validate-frontend-structure.ps1 -CheckFileSize`
- [ ] 手动端到端测试报告（含截图/录屏）

**Dependencies**: T1-T17 全部完成

**Files likely touched**: 无（验证任务）

**Estimated scope**: M（无代码改动，但验证工作量大）

---

### Checkpoint: Phase 5 完成即项目完成

- [ ] spec 16 项成功标准全部满足
- [ ] 双模块测试全绿
- [ ] 前端构建全绿
- [ ] 结构验证全绿
- [ ] 真实硬件验证完成（CHA 契约确认）
- [ ] Ready for release

---

## Risks and Mitigations

| # | 风险 | 影响 | 概率 | 缓解策略 |
|---|---|---|---|---|
| R1 | CHA != FFFF 时 CSV 字段顺序假设错误 | High | 中 | 模拟器按假设实现；真实硬件验证（T18）后如不符更新 spec；标注 `TODO-HW-VERIFY` |
| R2 | 命令响应识别规则漏判 | High | 中 | T1 单元测试覆盖 7 种响应类型 + 半包/粘包；mock TCP server 注入异常响应 |
| R3 | 重连状态机死锁/goroutine 泄漏 | High | 低 | T5 专门测试 5 种重连场景；旧连接清理 1s 超时强制 Close |
| R4 | 采集期间误发配置命令导致响应错位 | High | 低 | AD4 强制采集期间禁止配置；驱动层 + HTTP 层双重拦截（409） |
| R5 | shared SDK 误导入 wind-daq internal | High | 低 | T1-T6 checkpoint 用 grep 验证；`validate-structure.ps1` 兜底 |
| R6 | MPS-03 与 DAQ-P-1603 命名混淆 | Medium | 低 | AD1 + 命名红线 + grep 验证；Type 字面量 "MPS03" 无连字符 |
| R7 | Wails binding 签名变更未同步 | Medium | 中 | T17 强制 `wails3 generate bindings -silent`；typecheck 兜底 |
| R8 | CSV 17 列表头与现有 sink 逻辑冲突 | Medium | 低 | T11 独立 case 分支，不动其他设备逻辑；单元测试覆盖 |
| R9 | 模拟器与真实硬件行为不一致 | Medium | 中 | 模拟器严格按 SKILL.md 实现；真实硬件验证（T18）发现差异则更新模拟器 |
| R10 | DELAY 非整除导致 Hz 显示抖动 | Low | 低 | AD9 接受精度损失；UI 显示 round(1000/delay) |
| R11 | 错误帧阈值 50 触发误判 | Medium | 低 | 阈值可通过常量调整；成功帧重置计数 |
| R12 | 前端配置面板在弱网下保存超时 | Low | 中 | applyMps03Config 设置 10s 超时；UI 显示 loading + 错误提示 |

---

## Open Questions

| # | 问题 | 状态 | 处理时机 |
|---|---|---|---|
| Q1 | CHA != FFFF 时 CSV 字段顺序 | 待硬件验证 | T18 真实硬件测试 |
| Q2 | HEAD=1 + CHA != FFFF 时序号字段是否仍存在 | 待硬件验证 | T18 真实硬件测试 |
| Q3 | 错误帧阈值 50 是否合理 | 待实测调整 | T18 模拟器注入错误帧测试 |
| Q4 | 重连退避 5 次是否足够 | 待实测调整 | T18 弱网测试 |

---

## Parallelization Opportunities

| 阶段 | 可并行任务 | 前提 |
|---|---|---|
| Phase 1 | T1 ∥ T2 | 协议层无相互依赖 |
| Phase 2 | T6 ∥ (T4 → T5) | 模拟器只依赖协议层 |
| Phase 3 | T11 ∥ T10 | CSV sink 与 HTTP API 独立 |
| Phase 4 | T14 ∥ T15 ∥ T16 | T13 完成后前端三件套可并行 |
| Phase 5 | T17 ∥ T18 准备 | binding 同步与端到端验证准备可并行 |

**不可并行**：
- T4 → T5（重连状态机依赖驱动主体）
- T7 → T8 → T9 → T10（adapter → 注册 → API 链式依赖）
- T18 必须最后执行（依赖全部）

---

## Verification Checkpoints Summary

| Checkpoint | 位置 | 验证内容 | 通过标准 |
|---|---|---|---|
| C1 | Phase 1 后 | 协议层编译 + 测试 + 依赖方向 | `go test ./protocol/...` 全绿 + grep 无 wind-daq 依赖 |
| C2 | Phase 2 后 | shared SDK 全部测试 + 命名隔离 | `go test ./...` 全绿 + grep 无 DAQP1603 |
| C3 | Phase 3 后 | wind-daq 全部测试 + 命名隔离 | `go test ./...` 全绿 + grep 无 DAQP1603 |
| C4 | Phase 4 后 | 前端构建 + 模拟器端到端 | `npm typecheck && npm build` 全绿 + 模拟器全流程通过 |
| C5 | Phase 5 后 | 双模块 + 前端 + 结构验证 + 真实硬件 | spec 16 项成功标准全部满足 |
