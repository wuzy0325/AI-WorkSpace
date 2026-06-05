# wind-daq DAQ-T-1603 适配器重构计划

## 现状

### 代码分布

两个项目中各有独立的 DAQ-T-1603 驱动实现：

```
shared/device-sdk/go/protocol/           ← 共享层（T1603FrameReader, Parse, SendCommand）
shared/device-sdk/go/daq/hardware/
  └── daq_t1603.go                       ← 共享驱动（DAQT1603 结构体）

projects/daq-t1603/apps/desktop-wails/
  └── adapters/hardware/t1603_adapter.go  ← 项目适配层（类型转换）

projects/wind-daq/services/api-go/
  ├── internal/core/device/types.go      ← 独立类型副本（DaqT1603HardwareConfig）
  └── internal/adapters/hardware/
      └── daq_t1603.go                   ← 独立驱动副本（DAQT1603 结构体）
```

### 问题

wind-daq 的 `internal/adapters/hardware/daq_t1603.go` 是与共享驱动 `shared/daq/hardware/daq_t1603.go` **近乎相同的副本**，而非通过共享层接入。

这导致：

| 问题 | 详情 |
|------|------|
| 重复维护 | 两个驱动有相同的结构体、状态机、锁、readLoop、config sync 逻辑。任何协议层变更（如 @f3 支持）需要同步修改两处 |
| bug 放大 | 上次修复的 4 个 P0 bug（FrameReader 协议不匹配、缺少 ASCII 解析、@f0/@f1 缺失、config sync 缺失）同时在两处存在 |
| 类型分裂 | `device.DaqT1603HardwareConfig`（wind-daq）与 `sharedcore.DaqT1603HardwareConfig`（共享）属于不同包的独立类型，即使字段保持一致也需要双向映射 |
| 结构不一致 | DAQ-T-1603 已在 `projects/daq-t1603` 中通过共享驱动接入，但 wind-daq 仍保留独立副本。注意：wind-daq 当前 P1604 也是自有实现，本计划不顺手重构 P1604 |

## 目标

消除 wind-daq 中重复的 T1603 驱动副本，改为通过共享 `sharedhw.DAQT1603` 接入，并与 `projects/daq-t1603` 的共享驱动接入方式保持一致。

## 计划

### 步骤 1：确认替换边界

```
目标删除: projects/wind-daq/.../hardware/daq_t1603.go
```

该文件中的以下能力全部由 `sharedhw.DAQT1603` 提供：

- `Connect()` / `Disconnect()` — TCP 连接 + TCP keep-alive
- `StartAcquisition()` / `StopAcquisition()` — 发送 @f0/@f1
- `readLoop()` — 使用 T1603FrameReader 接收数据
- `processPayload()` — 解析帧并调用 DataSink
- `syncHardwareConfig()` / `readAllConfig()` — 连接后自动轮询

实际删除动作放在步骤 4，避免新增适配层和更新工厂前出现不可编译的中间状态。

### 步骤 2：新建适配层

参考 daq-t1603 项目 `t1603_adapter.go` 的类型转换方式，但 wind-daq 的 `ports.DeviceFactory` 是“每个 profile 创建一个 `ports.Device`”的单设备模型，因此新增适配器必须是单设备 wrapper，而不是 daq-t1603 项目中的多设备 manager：

```
新建: projects/wind-daq/.../hardware/t1603_adapter.go
```

适配器职责：

```go
type T1603Adapter struct {
    mu sync.RWMutex
    driver *sharedhw.DAQT1603
    profile device.Profile
    config device.DaqT1603HardwareConfig
}
```

适配器必须实现 wind-daq 的接口：

- `ports.Device`：`ID/Connect/Disconnect/StartAcquisition/StopAcquisition/SetDataSink/Status`
- `ports.UnitConfigurable`：`SetUnit`
- `ports.TareConfigurable`：`SetTare/GetTare/ClearTare`
- `ports.DaqT1603Configurable`：`GetDaqT1603Config/ApplyDaqT1603Config`

#### 类型转换映射

| wind-daq 类型 | 共享类型 | 转换规则 |
|---------------|----------|----------|
| `device.Profile.ID/Name` | `sharedcore.Profile.ID/Name` | 直接映射 |
| `device.Profile.Type` | `sharedcore.Profile.Type` | 固定为 `sharedcore.DeviceDaqT1603` |
| `device.Profile.Transport` | `sharedcore.Profile.Transport` | 直接映射 |
| `device.Profile.Address` | `sharedcore.Profile.Address` | 直接映射 |
| `device.Profile.Port` | `sharedcore.Profile.Port` | 直接映射 |
| `device.Profile.SamplingRate` | `sharedcore.Profile.SamplingRate` | 直接映射 |
| `device.Profile.Channels[]` | `sharedcore.Profile.Channels[]` | 16通道完整映射，包含 Index/Name/Enabled/Unit/Precision/RangeMin/RangeMax/TareOffset |
| `device.DaqT1603HardwareConfig` | `sharedcore.DaqT1603HardwareConfig` | 字段一对一映射，包含 ThermocoupleTypes/ChannelMask/SamplingRate/BinaryFormat/AverageCount/TriggerMode/TriggerEdge/TriggerCount/ShowTimestamp/ShowSequence/OpenCircuitCheck |

`SetDataSink` 需要把 wind-daq 的 `device.DataSink` 包装成 shared driver 的 `sharedcore.DataSink`，并把 `sharedcore.DataPayload` 转换为 wind-daq 的 `device.DataPayload`。`DeviceID/Timestamp/Channels/ChannelIndices` 保持原样语义；实现上可以不额外保存 sink 字段，但必须立即把转换回调注册到 `sharedhw.DAQT1603.SetDataSink`。

#### OnConfigSynced 回调

连接后共享驱动自动执行 `syncHardwareConfig()`。wind-daq 当前没有 daq-t1603 项目中的 `DeviceState/status map`，因此回调只应更新 adapter 内部运行态 `config/profile.DaqT1603Config`：

```go
dev.OnConfigSynced(func(cfg sharedcore.DaqT1603HardwareConfig) {
    adapter.mu.Lock()
    defer adapter.mu.Unlock()
    adapter.config = mapSharedToDevice(cfg)
    adapter.profile.DaqT1603Config = adapter.config
})
```

当前 `DeviceManager.GetDaqT1603Config` 会优先从已连接设备读取配置，因此运行态同步后可以通过 `ports.DaqT1603Configurable` 暴露给 API。若后续要求把硬件同步结果持久化到 profile JSON，需要另行设计 `DeviceManager` 到 `ProfileStore` 的回写路径；本计划不隐式引入持久化行为。

### 步骤 3：更新设备工厂

`internal/usecase/device_manager.go` 不负责按设备类型实例化硬件；它只接收 `ports.DeviceFactory` 并在 `Connect` 时调用 `factory.Create(profile)`。

需要更新所有 wind-daq 的 `ports.DeviceFactory` 实现，把 `DeviceDaqT1603` 从实例化独立 `hardware.DAQT1603` 改为实例化单设备 `hardware.T1603Adapter`：

- `projects/wind-daq/services/api-go/internal/bootstrap/bootstrap.go`
- `projects/wind-daq/services/api-go/pkg/appcontext/context.go`
- `projects/wind-daq/services/api-go/pkg/apiserver/apiserver.go`

```go
case device.DeviceDaqT1603:
    return hardware.NewT1603Adapter(profile), nil
```

`DeviceManager` 已通过 `DevicePort` 接口调用设备方法，更换适配器无需修改 usecase 层的其他代码。

### 步骤 4：清理

- 删除 `internal/adapters/hardware/daq_t1603.go` — 独立驱动副本
- 删除 `internal/adapters/hardware/` 中仅供独立驱动使用的辅助代码；当前 `daq_t1603.go` 的辅助逻辑主要在文件内，删除前确认没有其他设备依赖
- 保留 wind-daq 自有 `daq_p1604.go`，P1604 共享驱动化不属于本计划范围

单元测试至少覆盖：

- `T1603Adapter` 编译期实现 `ports.Device`、`ports.UnitConfigurable`、`ports.TareConfigurable`、`ports.DaqT1603Configurable`
- profile/config/channel 字段完整映射
- `SetDataSink` payload 转换
- `OnConfigSynced` 后 `GetDaqT1603Config` 返回同步配置
- 三处 factory 对 `DeviceDaqT1603` 返回 `NewT1603Adapter(profile)`，或至少通过编译验证覆盖

最终验证命令：

- 运行 `go test ./...` 确认无回归
- 运行 `go vet ./...` 确认无警告

## 最终结构

重构完成后：

```
shared/device-sdk/go/protocol/           ← 共享层（协议解析 + 命令发送）
shared/device-sdk/go/daq/hardware/
  └── daq_t1603.go                       ← 唯一驱动实现

projects/wind-daq/services/api-go/
  └── internal/adapters/hardware/
      └── t1603_adapter.go               ← 仅类型转换，无业务逻辑
```

重构后的设备驱动状态：

| 设备 | 共享驱动 | 项目适配层 |
|------|----------|------------|
| DAQ-P-1604 | 本计划不变更；当前 wind-daq 为自有 `hardware/daq_p1604.go`，仅复用 shared protocol primitives | 无 |
| DAQ-T-1603 | `shared/.../hardware/daq_t1603.go` | wind-daq `hardware/t1603_adapter.go`（新建） |
| DSA3217 | 无共享驱动 | wind-daq `hardware/dsa3217.go`（自有实现） |

## 例外说明

DSA3217 没有共享驱动，因为其协议（Telnet SET/LIST/SCAN）严重异构，且只在 wind-daq 中使用。但它同样通过 `DevicePort` 接口接入，无需特殊处理。

## 时间估计

| 步骤 | 估计工作量 |
|------|------------|
| 步骤 1-2 | ~2 小时（新建适配器 + 单元测试） |
| 步骤 3 | ~0.5 小时（修改工厂逻辑） |
| 步骤 4 | ~0.5 小时（清理 + 验证） |
| **总计** | **~3 小时** |
