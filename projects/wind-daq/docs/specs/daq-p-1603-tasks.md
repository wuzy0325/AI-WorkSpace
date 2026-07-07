# Implementation Plan: DAQ-P-1603 垂直切片实现

> Spec 参考：[daq-p-1603-spec.md](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/docs/specs/daq-p-1603-spec.md)
> Plan 参考：[daq-p-1603-plan.md](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/docs/specs/daq-p-1603-plan.md)
> 决策表 D-1~D-7 已确认（见 spec §Open Questions 与 plan §7）

## Overview

在 wind-daq 中新增 DAQ-P-1603 16 通道通用 AI 采集设备。设备特点：每通道可配置为压力或温度传感器；采样率 ≤500Hz；手动输入 IP 连接（不扫描）；CSV 表头按通道类型动态生成。采用 CGo + DLL FFI 调用 WTNDAQ16H_64.dll。

## Architecture Decisions

- **代码分层**：FFI 在 shared/device-sdk/go/ffi，完整驱动在 shared/device-sdk/go/daq/hardware，thin wrapper 在 wind-daq，与现有 DAQ-T-1603 模式对齐
- **垂直切片顺序**：Connect → Configure → Acquire → Record → Resilience → HIL，每个切片端到端可验证
- **FFI 一次性完整封装**：避免多次修改同一文件，FFI 在 Foundation 阶段全部封装完毕
- **通道传感器类型贯穿全链**：`ChannelConfig.SensorType` 字段从 core → adapter → wrapper → UI → CSV recorder 全链路传递

## Task List

### Phase 1: Foundation（基础设施）

- [ ] **Task 1**: 新增 WTNDAQ16H FFI 完整封装
  - Description: 封装 WTNDAQ16H_64.dll 的 12 个核心 API（DEV_Create/Release、AI_InitTask/StartTask/SendSoftTrig/GetStatus/ReadBinary/ReadAnalog/StopTask/ReleaseTask/VerifyParam/ScaleBinToVolt），提供 Go 友好类型签名，配套非 Windows stub。
  - Acceptance:
    - [ ] `shared/device-sdk/go/ffi/wtn_daq16h.go`（`//go:build windows`）封装全部 12 个 API
    - [ ] `shared/device-sdk/go/ffi/wtn_daq16h_stub.go`（`//go:build !windows`）提供 stub 返回 `ErrPlatformNotSupported`
    - [ ] `InitWTNDAQ16H(dllPath string) error` 用 `sync.Once` 幂等加载
    - [ ] `WTNDAQ16H_AI_PARAM` 结构体 Go 端等价定义，字段对齐头文件
    - [ ] 所有 BOOL 返回值转为 Go `error`
  - Verification:
    - [ ] `cd shared/device-sdk/go && go build ./ffi/... && go vet ./ffi/...`
    - [ ] `GOOS=linux go build ./ffi/...`（验证 stub 编译）
    - [ ] `go test ./ffi/... -run TestInitWTNDAQ16H`（DLL 缺失场景）
  - Dependencies: None
  - Files likely touched:
    - `shared/device-sdk/go/ffi/wtn_daq16h.go`（新增）
    - `shared/device-sdk/go/ffi/wtn_daq16h_stub.go`（新增）
    - `shared/device-sdk/go/ffi/wtn_daq16h_test.go`（新增）
  - Estimated scope: Medium（3 文件）

- [ ] **Task 2**: core 类型常量 + ChannelConfig.SensorType 字段
  - Description: 在 shared SDK 与 wind-daq 两处 core 类型中新增 DAQ-P-1603 设备常量，并为 ChannelConfig 添加 SensorType 字段（默认 pressure 保证向后兼容）。
  - Acceptance:
    - [ ] `shared/device-sdk/go/daq/core/types.go` 新增 `DeviceDAQP1603 Type = "DAQ-P-1603"`
    - [ ] `ChannelConfig` 新增 `SensorType string` 字段，JSON 标签 `sensorType,omitempty`，默认 `"pressure"`
    - [ ] 旧 profile（无 SensorType）反序列化时默认填充 `"pressure"`（通过自定义 UnmarshalJSON 或构造器默认值）
    - [ ] `projects/wind-daq/services/api-go/internal/core/device/types.go` 同步新增 `DeviceDAQP1603`
    - [ ] 不破坏现有 DAQ-P-1604 / DAQ-T-1603 测试
  - Verification:
    - [ ] `cd shared/device-sdk/go && go build ./... && go test ./...`
    - [ ] `cd projects/wind-daq/services/api-go && go build ./... && go test ./...`
    - [ ] `cd projects/daq-p1604 && go build ./...`（向后兼容验证）
  - Dependencies: None
  - Files likely touched:
    - `shared/device-sdk/go/daq/core/types.go`（修改）
    - `projects/wind-daq/services/api-go/internal/core/device/types.go`（修改）
  - Estimated scope: Small（2 文件）

### Checkpoint: Foundation
- [ ] shared SDK 全包编译通过
- [ ] wind-daq 后端编译通过
- [ ] daq-p1604 项目向后兼容验证通过
- [ ] 与用户确认后再进入 Phase 2

### Phase 2: Connect 切片（端到端：IP 输入 → Connected 状态）

- [ ] **Task 3**: shared SDK 适配器 Connect/Disconnect/Status
  - Description: 实现 DAQ-P-1603 适配器的连接管理部分：调用 FFI 建立 TCP 连接、同步基础参数、返回 Connected 状态。本任务不涉及采集逻辑。
  - Acceptance:
    - [ ] `shared/device-sdk/go/daq/hardware/daq_p1603.go` 实现 `core.Device` 的 `ID/Connect/Disconnect/Status/SetDataSink` 方法（StartAcquisition/StopAcquisition 留待 Task 9）
    - [ ] 编译期断言：`var _ core.Device = (*DAQP1603)(nil)`
    - [ ] `Connect()` 调用 `ffi.WTNDAQ16HDevCreate` + `AI_VerifyParam` + `AI_InitTask`（仅初始化）
    - [ ] `Disconnect()` 调用 `WTNDAQ16HDevRelease`，handle 归零
    - [ ] `Status()` 返回 `core.Connection` 状态
    - [ ] 状态机：Disconnected → Connected → Disconnected，错误态 Error
    - [ ] `sync.Mutex` 保护状态迁移
    - [ ] 重复 Connect / 重复 Disconnect 安全
  - Verification:
    - [ ] `cd shared/device-sdk/go && go build ./... && go vet ./... && go test ./daq/...`
    - [ ] 单元测试：状态机 Connect/Disconnect 全路径、重复调用安全
  - Dependencies: Task 1, Task 2
  - Files likely touched:
    - `shared/device-sdk/go/daq/hardware/daq_p1603.go`（新增）
    - `shared/device-sdk/go/daq/hardware/daq_p1603_test.go`（新增）
  - Estimated scope: Medium（2 文件）

- [ ] **Task 4**: wind-daq thin wrapper（Connect/Disconnect 部分）
  - Description: 在 wind-daq 内创建 thin wrapper，桥接 shared SDK DAQP1603 与 wind-daq ports.Device 接口。本任务仅实现连接相关方法。
  - Acceptance:
    - [ ] `projects/wind-daq/services/api-go/internal/adapters/hardware/daq_p1603.go` 实现 wind-daq `ports.Device` 的连接方法
    - [ ] 编译期断言：`var _ ports.Device = (*DAQP1603Adapter)(nil)`
    - [ ] 内部持有 `*hardwaredaq.DAQP1603`，方法委托
    - [ ] 实现 `ports.ErrorNotifiable`
  - Verification:
    - [ ] `cd projects/wind-daq/services/api-go && go build ./... && go vet ./...`
  - Dependencies: Task 3
  - Files likely touched:
    - `projects/wind-daq/services/api-go/internal/adapters/hardware/daq_p1603.go`（新增）
  - Estimated scope: Small（1 文件）

- [ ] **Task 5**: bootstrap 注册 + profile + 手动 IP 输入 UI
  - Description: 在装配根注册 DAQ-P-1603 工厂分支，新增默认 profile，并在设备管理抽屉为该类型提供"手动输入 IP"UI（不显示扫描按钮）。
  - Acceptance:
    - [ ] `bootstrap.go` 的 `deviceFactory.Create` 新增 `case device.DeviceDAQP1603:` 分支
    - [ ] `device-profiles.json` 新增 DAQ-P-1603 默认 profile（16 通道默认 SensorType="pressure"、Unit="Pa"、SamplingRate=100）
    - [ ] `DeviceManagementDrawer.vue` 对 DAQ-P-1603 类型显示 IP 输入框 + "添加设备"按钮，不显示"扫描"按钮
    - [ ] 添加设备后能进入配置面板占位（配置面板在 Task 7 实现）
  - Verification:
    - [ ] `cd projects/wind-daq/services/api-go && go build ./... && go vet ./... && go test ./...`
    - [ ] 启动应用，设备管理抽屉能看到 DAQ-P-1603 profile 条目
    - [ ] 手工：IP 输入框可输入，添加设备后出现在设备列表
  - Dependencies: Task 4
  - Files likely touched:
    - `projects/wind-daq/services/api-go/internal/bootstrap/bootstrap.go`（修改）
    - `projects/wind-daq/apps/desktop-wails/config/device-profiles.json`（修改）
    - `projects/wind-daq/apps/desktop-wails/frontend/src/components/device/DeviceManagementDrawer.vue`（修改）
  - Estimated scope: Medium（3 文件）

### Checkpoint: Connect 切片完成
- [ ] 用户可手动输入 IP 添加 DAQ-P-1603 设备
- [ ] 点击连接后 5 秒内 UI 显示 Connected 状态
- [ ] 后端编译 + 前端 typecheck 全绿
- [ ] 与用户确认后再进入 Phase 3

### Phase 3: Configure 切片（端到端：16 通道传感器类型配置 → ApplyConfig）

- [ ] **Task 6**: shared SDK 适配器 ApplyConfig
  - Description: 实现适配器的运行时配置变更：采样率（≤500Hz 校验）、量程、通道传感器类型、单位、精度。变更后同步到硬件（若已连接）。
  - Acceptance:
    - [ ] `daq_p1603.go` 新增 `ApplyConfig(cfg)` 方法
    - [ ] 采样率 ≤500Hz 校验，越界返回错误（D-2）
    - [ ] 通道 SensorType 字段读取并存储到适配器内部状态
    - [ ] 已连接设备调用 AI_VerifyParam + 重新 AI_InitTask 同步硬件
    - [ ] 未连接设备仅更新内部配置，待 Connect 时同步
  - Verification:
    - [ ] `go test ./daq/... -run TestApplyConfig`
    - [ ] 测试用例：采样率 500Hz 通过、501Hz 拒绝、通道类型切换后内部状态正确
  - Dependencies: Task 3
  - Files likely touched:
    - `shared/device-sdk/go/daq/hardware/daq_p1603.go`（修改）
    - `shared/device-sdk/go/daq/hardware/daq_p1603_test.go`（修改）
  - Estimated scope: Small（2 文件）

- [ ] **Task 7**: 前端 DaqP1603Config.vue 通道传感器类型表
  - Description: 实现 DAQ-P-1603 专属配置面板，含采样率输入（≤500Hz 校验）、量程、16 行通道传感器类型表（每行：通道号 + 类型下拉 + 单位下拉 + 启用开关）、精度、硬件时间戳开关。
  - Acceptance:
    - [ ] `DaqP1603Config.vue` 实现 v-model 双向绑定
    - [ ] 采样率输入 >500Hz 时红色提示 + 禁用提交按钮
    - [ ] 16 行通道表，传感器类型下拉切换时单位下拉选项自动切换（压力→Pa/kPa/MPa/mmH2O，温度→℃/℉）
    - [ ] 通道类型切换时单位重置为该类型默认值
    - [ ] TS 严格模式，无 any
    - [ ] 单文件 ≤ 500 行
    - [ ] 所有文本走 i18n
  - Verification:
    - [ ] `cd frontend && npm run typecheck && npm run build`
    - [ ] `.\scripts\validate-frontend-structure.ps1 -CheckFileSize`
    - [ ] 手工：切换通道类型时单位下拉正确切换；501Hz 时按钮禁用
  - Dependencies: Task 2
  - Files likely touched:
    - `frontend/src/components/device/DaqP1603Config.vue`（新增）
    - `frontend/src/shared/types/devices.ts`（修改：新增 DaqP1603Config、ChannelSensorType 类型）
    - `frontend/src/locales/zh-CN.json`（修改）
    - `frontend/src/locales/en-US.json`（修改）
  - Estimated scope: Medium（4 文件）

- [ ] **Task 8**: DeviceManagementDrawer 集成 DaqP1603Config
  - Description: 在设备管理抽屉中，当 profile.Type === "DAQ-P-1603" 时渲染 DaqP1603Config 组件，并通过 deviceStore.applyConfig 提交到后端。
  - Acceptance:
    - [ ] `DeviceManagementDrawer.vue` 根据 profile.Type 动态渲染配置组件
    - [ ] applyConfig action 调用后端 ApplyConfig 方法
    - [ ] 配置变更后 UI 反馈成功/失败状态
  - Verification:
    - [ ] `npm run typecheck && npm run build`
    - [ ] 手工：连接设备后打开配置面板，修改采样率与通道类型，点击应用，UI 显示成功
  - Dependencies: Task 5, Task 6, Task 7
  - Files likely touched:
    - `frontend/src/components/device/DeviceManagementDrawer.vue`（修改）
    - `frontend/src/stores/deviceStore.ts`（修改：applyConfig action）
  - Estimated scope: Small（2 文件）

### Checkpoint: Configure 切片完成
- [ ] 用户可为 16 通道分别选择压力/温度传感器类型与单位
- [ ] ApplyConfig 后实机参数同步（HIL 验证）
- [ ] 前端 typecheck + build 全绿
- [ ] 与用户确认后再进入 Phase 4

### Phase 4: Acquire 切片（端到端：开始采集 → 实时数据 → 停止）

- [ ] **Task 9**: shared SDK 适配器 StartAcquisition/readLoop/StopAcquisition + 通道类型换算
  - Description: 实现采集核心逻辑：启动 AI_StartTask + AI_SendSoftTrig，后台 readLoop 循环 AI_ReadBinary，按通道 SensorType 分支换算（压力/温度）后投递 sink。StopAcquisition 调用 AI_StopTask + AI_ReleaseTask，1 秒内退出 readLoop。
  - Acceptance:
    - [ ] `StartAcquisition()` 调用 AI_StartTask + AI_SendSoftTrig，启动 readLoop goroutine
    - [ ] readLoop 循环 AI_ReadBinary，每帧转 `core.DataPayload` 投递 sink
    - [ ] 压力通道：AI_ScaleBinToVolt → 按系数转 Pa/kPa/MPa/mmH2O
    - [ ] 温度通道：AI_ScaleBinToVolt → 按系数（PT100/热电偶）转 ℃/℉
    - [ ] `StopAcquisition()` 调用 AI_StopTask + AI_ReleaseTask，readLoop 1 秒内退出
    - [ ] 复用 `protocol.StopReasonTracker` 区分主动停止 vs 异常停止
    - [ ] `sync.Mutex` 保护 Start/Stop 并发
  - Verification:
    - [ ] `go test ./daq/... -run TestAcquisition`
    - [ ] 测试用例：Start→Stop 优雅退出、并发 Start/Stop 不 panic、通道类型换算分支正确
    - [ ] 覆盖率 ≥ 70%
  - Dependencies: Task 6
  - Files likely touched:
    - `shared/device-sdk/go/daq/hardware/daq_p1603.go`（修改）
    - `shared/device-sdk/go/daq/hardware/daq_p1603_test.go`（修改）
  - Estimated scope: Medium（2 文件）

- [ ] **Task 10**: wind-daq thin wrapper 扩展采集方法
  - Description: 在 thin wrapper 中补全 StartAcquisition/StopAcquisition/SetDataSink 方法，委托给 shared SDK 适配器。
  - Acceptance:
    - [ ] thin wrapper 实现完整 `ports.Device` 接口
    - [ ] StartAcquisition/StopAcquisition/SetDataSink 委托内部 DAQP1603 实例
    - [ ] 数据 sink 桥接到 wind-daq 的数据流
  - Verification:
    - [ ] `cd projects/wind-daq/services/api-go && go build ./... && go vet ./...`
    - [ ] 编译期断言通过
  - Dependencies: Task 9
  - Files likely touched:
    - `projects/wind-daq/services/api-go/internal/adapters/hardware/daq_p1603.go`（修改）
  - Estimated scope: Small（1 文件）

- [ ] **Task 11**: 前端采集控制 + 实时曲线
  - Description: 在 DeviceDetailPanel 或新建组件中为 DAQ-P-1603 提供采集控制（开始/停止按钮）与实时曲线显示（16 通道，按通道类型着色）。
  - Acceptance:
    - [ ] 采集控制按钮调用后端 StartAcquisition/StopAcquisition
    - [ ] 实时曲线通过 GetLatestSnapshots 轮询（500ms）刷新
    - [ ] 16 通道按 SensorType 着色（压力蓝色系，温度橙色系）
    - [ ] 通道可单独隐藏/显示
  - Verification:
    - [ ] `npm run typecheck && npm run build`
    - [ ] 手工：点击开始采集后曲线刷新 ≥25Hz，点击停止后曲线停止
  - Dependencies: Task 10
  - Files likely touched:
    - `frontend/src/components/device/RealtimeChart.vue`（修改或新增变体）
    - `frontend/src/components/main/DeviceDetailPanel.vue`（修改）
    - `frontend/src/stores/deviceStore.ts`（修改：采集状态管理）
  - Estimated scope: Medium（3 文件）

### Checkpoint: Acquire 切片完成
- [ ] 用户可开始采集、查看 16 通道实时曲线、停止采集
- [ ] 通道类型换算正确（压力/温度单位与配置一致）
- [ ] 与用户确认后再进入 Phase 5

### Phase 5: Record 切片（端到端：录制 CSV，按通道类型动态表头）

- [ ] **Task 12**: CSV recorder 扩展支持 DAQ-P-1603 动态表头
  - Description: 扩展 wind-daq 的 CSV recorder，为 DAQ-P-1603 生成 17 列动态表头（Timestamp + 16 通道），每通道列名按 SensorType 生成（如 CH01_Pa、CH02_℃）。
  - Acceptance:
    - [ ] recorder 识别 DAQ-P-1603 数据源
    - [ ] 表头格式：`Timestamp, CH01_<unit>, CH02_<unit>, ..., CH16_<unit>`
    - [ ] unit 由 ChannelConfig.Unit 决定（Pa/kPa/MPa/mmH2O/℃/℉）
    - [ ] 数据行与表头列数一致（17 列）
    - [ ] 文件滚动 / 自动停止逻辑与现有 recorder 一致
  - Verification:
    - [ ] `go test ./... -run TestCSVRecorder`
    - [ ] 测试用例：16 通道全压力 → 表头全 Pa；混合压力+温度 → 表头混合单位；数据行 17 列
  - Dependencies: Task 9
  - Files likely touched:
    - `projects/wind-daq/services/api-go/internal/adapters/storage/csv_sink.go`（修改，路径待确认）
    - 对应测试文件（修改）
  - Estimated scope: Medium（2 文件）

- [ ] **Task 13**: 前端录制控制集成
  - Description: 在录制控制组件中支持 DAQ-P-1603，用户可选择录制文件路径、文件滚动条件、开始/停止录制。
  - Acceptance:
    - [ ] RecordingControl.vue 对 DAQ-P-1603 类型可用
    - [ ] 录制状态通过 `daq:recording-status` 事件推送
    - [ ] 录制警告通过 `daq:recording-warning` 事件推送
  - Verification:
    - [ ] `npm run typecheck && npm run build`
    - [ ] 手工：开始采集后点击录制，CSV 文件生成且表头正确
  - Dependencies: Task 11, Task 12
  - Files likely touched:
    - `frontend/src/components/device/RecordingControl.vue`（修改）
    - `frontend/src/stores/recordingStore.ts`（修改）
  - Estimated scope: Small（2 文件）

### Checkpoint: Record 切片完成
- [ ] 用户可录制 CSV 文件，表头按通道类型动态生成
- [ ] CSV 文件可被 Excel/Numbers 正确打开
- [ ] 与用户确认后再进入 Phase 6

### Phase 6: Resilience 切片（端到端：断连 → 重连）

- [ ] **Task 14**: 断连检测 + handleConnectionLost + 自动重连
  - Description: 在 readLoop 中检测 DLL 返回 FALSE 或超时，触发 handleConnectionLost，状态置 Disconnected，并通过 OnStateChange 回调通知 UI。提供自动重连选项。
  - Acceptance:
    - [ ] readLoop 检测 AI_ReadBinary 返回 FALSE → 触发 handleConnectionLost
    - [ ] handleConnectionLost 调用 AI_StopTask + AI_ReleaseTask + DEV_Release 清理资源
    - [ ] 状态置 Disconnected，通过 OnStateChange 回调通知
    - [ ] UI 提供"自动重连"开关（默认开启），断连后 5 秒尝试重连
    - [ ] 重连成功后状态恢复 Connected，需手动重新开始采集
  - Verification:
    - [ ] `go test ./daq/... -run TestConnectionLost`
    - [ ] 测试用例：模拟 DLL 返回 FALSE → 状态变更 Disconnected；模拟重连成功 → 状态恢复
  - Dependencies: Task 9
  - Files likely touched:
    - `shared/device-sdk/go/daq/hardware/daq_p1603.go`（修改）
    - `shared/device-sdk/go/daq/hardware/daq_p1603_test.go`（修改）
  - Estimated scope: Medium（2 文件）

- [ ] **Task 15**: 多设备并发安全验证
  - Description: 验证 DAQ-P-1603 与 DAQ-P-1604 可同时连接、并行采集，互不阻塞。必要时在适配器层增加设备级隔离。
  - Acceptance:
    - [ ] DAQ-P-1603 与 DAQ-P-1604 同时 Connect 成功
    - [ ] 两设备并行 StartAcquisition，数据不串台
    - [ ] 两设备并行 StopAcquisition，1 秒内均退出
    - [ ] WTNDAQ16H SDK 内部基于 HANDLE 隔离（HIL 验证）
  - Verification:
    - [ ] `go test ./... -run TestMultiDevice`
    - [ ] HIL：两设备同时采集 60 秒，数据无串台
  - Dependencies: Task 14
  - Files likely touched:
    - `shared/device-sdk/go/daq/hardware/daq_p1603_test.go`（修改）
  - Estimated scope: Small（1 文件）

### Checkpoint: Resilience 切片完成
- [ ] 拔网线 → 5 秒内 UI 显示 Disconnected
- [ ] 重新插回 → 自动重连到 Connected
- [ ] DAQ-P-1603 与 DAQ-P-1604 并行采集互不阻塞
- [ ] 与用户确认后再进入 Phase 7

### Phase 7: HIL 验证

- [ ] **Task 16**: HIL 实机回归测试 + Wails 绑定同步
  - Description: 执行完整 HIL 测试，验证 spec §Success Criteria 表第 5-13 项；若后端方法签名变更则同步 Wails 绑定；执行结构验证脚本。
  - Acceptance:
    - [ ] 手工输入 IP 连接 DAQ-P-1603（不支持扫描）
    - [ ] 连接、配置、采集、停止、断连恢复全链路通过
    - [ ] DAQ-P-1603 与 DAQ-P-1604 并行采集互不阻塞
    - [ ] CSV 文件表头按通道传感器类型动态生成
    - [ ] 采样率 >500Hz 时前端拒绝提交、后端返回错误
    - [ ] 通道传感器类型切换后 CSV 表头与数据单位正确变化
    - [ ] 验证 spec §Open Questions "待 HIL 验证" 5 项
    - [ ] 若 backend `app.go` 公开方法签名变更：`wails3 generate bindings -silent` 后无 diff
    - [ ] `.\scripts\validate-structure.ps1` 全绿
    - [ ] `.\scripts\validate-frontend-structure.ps1 -CheckFileSize` 全绿
  - Verification:
    - [ ] 按 `projects/wind-daq/docs/runbooks/hil-validation-plan.md` 执行
    - [ ] 测试用例三段式记录（前置/步骤/期待结果）
  - Dependencies: Task 13, Task 14, Task 15
  - Files likely touched: 视实际情况
  - Estimated scope: Small（人工执行 + 可能的绑定文件更新）

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| WTNDAQ16H SDK 头文件中文乱码（GBK） | Med | 以结构体字段类型与示例代码为主，注释作辅证 |
| DLL 调用约定（stdcall vs cdecl） | High | 头文件 `FAR PASCAL` = stdcall，syscall.Syscall 默认 stdcall |
| 500Hz 上限与 SDK 头文件 500kSPS 不一致 | High | 前端硬校验 + 后端二次校验；HIL 实测 >500Hz SDK 行为并更新文档 |
| 通道传感器类型动态化影响 DataPayload/CSV | Med | SensorType 字段贯穿全链；CSV 表头按类型生成；单位换算在 adapter 分支 |
| 温度通道换算公式未知 | Med | HIL 确认厂商 API；若无，UI 暴露换算系数让用户手动标定 |
| 多设备并发 DLL 全局状态 | High | HIL 实测验证；适配器层 sync.Mutex 保护单设备 |
| 断网检测延迟 | Med | ReadBinary 超时 10s + handleConnectionLost 触发状态变更 |

## Open Questions（待 HIL 验证）

1. DLL 部署路径：build 时复制到可执行文件同目录，不 go:embed（D-6 默认，HIL 确认）
2. 单位换算 API：厂商是否提供 ScaleBinToVolt 等同 API？温度通道换算公式？
3. 触发模式：默认软件触发，UI 是否需要暴露触发配置（D-5 待 HIL 确认）
4. 500Hz 上限来源：固件限制 vs SDK 降采样 vs 场景需求；>500Hz 时 SDK 行为
5. 前端组件复用：DaqP1603Config 与 DaqP1604Config 是否抽公共子组件

## 任务依赖图

```
Phase 1:  T1 ──┐
          T2 ──┤
                ▼
Phase 2:  T3 ──► T4 ──► T5 ──────────────────┐
                │                              │
                ▼                              │
Phase 3:  T6 ──► T7 ──► T8 ◄─────────────────┘
                │
                ▼
Phase 4:  T9 ──► T10 ──► T11 ──┐
                │               │
                ▼               │
Phase 5:  T12 ──► T13 ◄────────┘
                │
                ▼
Phase 6:  T14 ──► T15
                │
                ▼
Phase 7:  T16
```

## Parallelization Opportunities

- **Phase 1 内**：T1（FFI）与 T2（core 常量）可并行（独立文件）
- **Phase 3 内**：T6（adapter ApplyConfig）与 T7（前端组件）可并行（前后端独立）
- **Phase 5 内**：T12（CSV recorder）与 T11（前端采集控制）可并行
- **必须串行**：T3→T4→T5（连接链路）、T9→T10→T11（采集链路）、T14→T15（断连→并发）

## Verification Checklist

- [x] 每个任务有验收标准
- [x] 每个任务有验证步骤
- [x] 任务依赖已识别并按序
- [x] 没有任务触及超过 5 个文件
- [x] 阶段间存在检查点（6 个）
- [x] 高风险任务前置（FFI 在 Phase 1，500Hz 校验在 Task 6）
