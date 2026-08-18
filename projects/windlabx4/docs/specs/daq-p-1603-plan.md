# Plan: DAQ-P-1603 实现计划

> Spec 参考：[daq-p-1603-spec.md](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/WindLabX4/docs/specs/daq-p-1603-spec.md)

## 1. 主要组件与依赖关系

```
                    ┌──────────────────────────────────────┐
                    │  projects/WindLabX4 前端 (Vue 3)       │
                    │  - DaqP1603Config.vue (新增)          │
                    │  - DeviceManagementDrawer.vue (改)    │
                    │  - deviceStore.ts (改)                │
                    └────────────────┬─────────────────────┘
                                     │ Wails bindings
                    ┌────────────────▼─────────────────────┐
                    │  projects/WindLabX4 backend           │
                    │  - app.go (改：profile 路由)          │
                    │  - bootstrap.go (改：factory case)   │
                    │  - adapters/hardware/daq_p1603.go    │
                    │    (新增：thin wrapper)               │
                    └────────────────┬─────────────────────┘
                                     │ imports
                    ┌────────────────▼─────────────────────┐
                    │  shared/device-sdk/go/daq            │
                    │  - core/types.go (改：常量)           │
                    │  - hardware/daq_p1603.go (新增)       │
                    │  - ports/device.go (复用，不改)       │
                    └────────────────┬─────────────────────┘
                                     │ imports
                    ┌────────────────▼─────────────────────┐
                    │  shared/device-sdk/go/ffi (新增)      │
                    │  - wtn_daq16h.go (//go:build windows)│
                    │  - wtn_daq16h_stub.go (其他平台)      │
                    └────────────────┬─────────────────────┘
                                     │ syscall
                    ┌────────────────▼─────────────────────┐
                    │  WTNDAQ16H_64.dll (厂商提供，部署期)  │
                    └──────────────────────────────────────┘
```

## 2. 实现顺序（依赖图）

```
T1 FFI 封装层  ────► T2 core 类型常量 ────► T3 shared SDK 适配器
                                                  │
                                                  ▼
                            T4 WindLabX4 thin wrapper
                                                  │
                                                  ▼
                            T5 bootstrap 工厂注册
                                                  │
                                                  ▼
                            T6 前端配置组件 ──► T7 集成与 HIL
```

**关键路径**：T1 → T3 → T4 → T5 → T7（必须串行）
**可并行**：T2（独立常量修改）可与 T1 并行；T6 前端可在 T4/T5 完成后启动

## 3. 风险与缓解

| 风险 | 等级 | 影响 | 缓解 |
|---|---|---|---|
| WTNDAQ16H SDK 头文件中文乱码（GBK 编码） | 中 | 误读参数含义 | 以 `WTNDAQ16H_AI_PARAM` 结构体字段类型与示例代码为主，注释作辅证；遇歧义查 Simple/ 示例 |
| DLL 调用约定不明确（stdcall vs cdecl） | 高 | FFI 调用 panic | 头文件 `FAR PASCAL` = stdcall（与 wtnmc4a 一致），syscall.Syscall 系列默认stdcall |
| HANDLE 类型在 Go 中的表达 | 中 | 32/64 位兼容 | 用 `uintptr`（与 wtnmc4a_motion.go 一致），避免 `unsafe.Pointer` 跨平台问题 |
| 采样率上限 500Hz 与 SDK 头文件 500kSPS 不一致 | 高 | 配置错误导致 SDK 行为异常 | 前端硬校验 ≤500Hz；后端 ApplyConfig 二次校验；HIL 实测 >500Hz 时 SDK 行为（拒绝/降速/无效），并更新文档 |
| 通道传感器类型动态化影响 DataPayload/CSV | 中 | 表头/单位/换算错误 | `ChannelConfig.SensorType` 字段贯穿 profile→adapter→recorder→UI；CSV 表头按通道类型生成；单位换算在 adapter 内按类型分支 |
| 温度通道换算公式未知 | 中 | 温度数据显示错误 | HIL 阶段确认厂商是否提供温度换算 API；若无，UI 暴露换算系数（kPa/℃ per Volt）让用户手动标定 |
| 多设备并发采集时 DLL 全局状态 | 高 | 互斥阻塞或数据串台 | WTNDAQ16H SDK 内部应基于 HANDLE 隔离（与 WTNMC4A 同设计），但需 HIL 实测验证；适配器层用 `sync.Mutex` 保护单设备状态 |
| 断网检测延迟 | 中 | UI 卡死 | ReadBinary 超时 10s + 状态轮询；超时后 `handleConnectionLost` 触发 state 变更 |
| 非 Windows 平台编译 | 低 | Linux/macOS 开发者无法编译 | stub 文件提供空实现，返回 `ErrPlatformNotSupported` |

## 4. 并行 vs 串行

| 阶段 | 任务 | 模式 |
|---|---|---|
| 阶段 A | T1 (FFI) + T2 (core 常量) | **并行**（独立文件） |
| 阶段 B | T3 (shared SDK 适配器) | 串行（依赖 T1+T2） |
| 阶段 C | T4 (thin wrapper) + T6 (前端组件) | **并行**（前后端独立） |
| 阶段 D | T5 (bootstrap 注册) | 串行（依赖 T4） |
| 阶段 E | T7 (集成测试 + HIL) | 串行（依赖全部） |

## 5. 验证检查点

| 检查点 | 触发任务 | 验证命令 | 通过标准 |
|---|---|---|---|
| CP-1 | T1 完成 | `cd shared/device-sdk/go && go build ./ffi/... && go vet ./ffi/...` | FFI 包编译通过，stub 在非 Windows 也能编译 |
| CP-2 | T3 完成 | `cd shared/device-sdk/go && go build ./... && go test ./daq/...` | 适配器单元测试 ≥ 70% 覆盖 |
| CP-3 | T5 完成 | `cd projects/WindLabX4/services/api-go && go build ./... && go vet ./...` | WindLabX4 后端编译通过 |
| CP-4 | T6 完成 | `cd projects/WindLabX4/apps/desktop-wails/frontend && npm run typecheck && npm run build` | 前端类型检查与构建通过 |
| CP-5 | T7 完成 | `wails3 generate bindings -silent` 后无 diff；`.\scripts\validate-structure.ps1` 全绿 | Wails 绑定同步、结构验证通过 |
| CP-6 | HIL 完成 | 手工测试 spec §Success Criteria 表 5-11 项 | 全部通过 |

## 6. 实现策略要点

### 6.1 FFI 层（T1）
- 参考 [ffi/wtnmc4a.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/ffi/wtnmc4a.go) 模式：`sync.Once` 加载 DLL、全局 `*syscall.Proc` 缓存
- 导出函数清单（按头文件顺序）：
  - `WTNDAQ16H_DEV_CreateA` / `WTNDAQ16H_DEV_Release`
  - `WTNDAQ16H_AI_InitTask` / `WTNDAQ16H_AI_StartTask` / `WTNDAQ16H_AI_SendSoftTrig`
  - `WTNDAQ16H_AI_GetStatus` / `WTNDAQ16H_AI_ReadBinary` / `WTNDAQ16H_AI_ReadAnalog`
  - `WTNDAQ16H_AI_StopTask` / `WTNDAQ16H_AI_ReleaseTask`
  - `WTNDAQ16H_AI_VerifyParam`
- 参数结构体 `WTNDAQ16H_AI_PARAM` 在 Go 端等价定义为 `[N]byte` 或显式 struct（参考 wtnmc4a 处理 WTNMC4A 结构体的方式）
- 所有 BOOL 返回值转为 Go `error`（FALSE → 调用 `GetLastError` 或返回 generic error）

### 6.2 适配器层（T3）
- 参考 [daq_t1603.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/device-sdk/go/daq/hardware/daq_t1603.go) 模式
- 状态机：`Disconnected → Connected → Acquiring → Connected → Disconnected`，错误态 `Error`
- readLoop：循环调用 `AI_ReadBinary`，每帧转 `core.DataPayload` 投递 `sink`
- 断连处理：DLL 返回 FALSE 或超时 → 触发 `handleConnectionLost`，状态置 `Disconnected`，调用 `OnStateChange` 回调
- 复用 `protocol.StopReasonTracker` 区分主动停止 vs 异常停止
- **通道传感器类型处理（D-7 新增）**：
  - 从 `profile.Channels[i].SensorType` 读取每通道类型（pressure/temperature）
  - 原始 ADC 码值（U16）按通道类型分支换算：
    - 压力：调用 SDK `AI_ScaleBinToVolt` 转 V，再按压力传感器系数转 Pa/kPa/MPa/mmH2O
    - 温度：调用 SDK `AI_ScaleBinToVolt` 转 V，再按温度传感器系数（如 PT100/热电偶查表）转 ℃/℉
  - `DataPayload.Channels[i]` 携带换算后的工程值，`DataPayload.ChannelIndices[i]` 携带通道号
  - 单位信息通过 `ChannelConfig.Unit` 字段传递，CSV recorder 据此生成表头

### 6.3 thin wrapper（T4）
- 参考 [t1603_adapter.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/WindLabX4/services/api-go/internal/adapters/hardware/t1603_adapter.go) 模式
- 仅做接口桥接：WindLabX4 `ports.Device` ↔ shared SDK `daq.ports.Device`
- 实现 `ports.TareConfigurable`（如适用）与 `ports.ErrorNotifiable`

### 6.4 前端组件（T6）
- 参考 [DaqT1603Config.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/WindLabX4/apps/desktop-wails/frontend/src/components/device/DaqT1603Config.vue) 模式
- 配置字段：
  - 采样率（≤500Hz，前端硬校验）
  - 量程（±10V/±5V/±1V/0-20mA）
  - **通道传感器类型表（D-7 新增）**：每行包含 通道号 + 传感器类型下拉（压力/温度） + 单位下拉（随类型切换） + 启用开关
  - 通道精度（0~6 位小数）
  - 硬件时间戳开关
- 通过 `deviceStore` 的 `applyConfig` action 提交，触发后端 `ApplyConfig`
- 遵循 [frontend-ai-rules-deploy.zh-CN.md](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/docs/runbooks/frontend-ai-rules-deploy.zh-CN.md) 量化红线（单 .vue ≤ 500 行）

### 6.5 Wails 绑定同步（T7）
- 若 T4/T5 修改了 `app.go` 公开方法签名，**强制**执行 `wails3 generate bindings -silent`
- 检查 `frontend/bindings/` 目录是否生成对应 TS 类型
- TS 严格模式不允许 any，必须显式类型注解

## 7. 决策点（用户已确认 2026-07-06）

| # | 决策点 | 确认值 | 影响 |
|---|---|---|---|
| D-1 | 通道数 | **16 通道** | 与 WTNDAQ16H_AI_MAX_CHANNELS 对齐；Snapshot 16 个浮点值 |
| D-2 | 采样率上限 | **500Hz** | 与 WTNDAQ16H 头文件 500kSPS 不同（DAQ-P-1603 为低速定制版本，需 HIL 验证来源）；前端校验 ≤500Hz，后端 ApplyConfig 拒绝越界值 |
| D-3 | 设备发现 | **手动输入 IP，不支持扫描** | 与 WTNDAQ16H SDK 一致（参考代码无扫描 API）；UI 提供 IP 输入框，无扫描按钮 |
| D-4 | CSV 表头 | **Timestamp + 16 通道 = 17 列**，按通道传感器类型动态生成 | 不含大气压/大气温度；表头格式 `Timestamp, CH01_<unit>, CH02_<unit>, ...`，unit 由通道 SensorType 决定 |
| D-5 | 触发模式 | **默认软件触发，UI 暴露触发配置**（待 HIL 确认是否需要） | 默认 `bDTriggerEn=FALSE, bATriggerEn=FALSE`，UI 可选启用 |
| D-6 | DLL 部署 | **build 时复制到可执行文件同目录，不 go:embed** | 通过 Taskfile.yml 增加复制步骤 |
| D-7 | 通道传感器类型 | **每通道可配置为压力或温度**（用户新增需求） | `ChannelConfig` 新增 `SensorType` 字段；CSV 表头动态生成；单位选项随类型切换；单位换算按类型分支 |
