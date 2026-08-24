# DAQ-P-1603 打压读数只有约一半的问题复盘

## 现象

真机（192.168.3.104，DAQ-P-1603）打压 -1000Pa 后，采集读数只有约 **-480Pa**（期望约 -1000Pa）；
空闲通道校零前显示约 **+483Pa**（期望约 0Pa）。用其它软件确认硬件读数正确（-1000Pa 左右），
问题定位在 WindLabX4 / 1604Cal 共用的 device-sdk 换算逻辑。

## 根因

`shared/device-sdk/go/daq/hardware/daq_p1603.go` 的 `readLoop` 把 U16 原始码值换算电流时，
用了**单极性假设**：

```go
current = u16Code / 65535 * 20
```

但 WTNDAQ16H 的 **0-20mA 量程实为双极性 ±20mA**（`GetVoltRangeInfo` 返回
`MinVolt=-20 / MaxVolt=+20 / NeadCode=-32768`），**零点（0mA）对应码值 32768 而非 0**。

因此正确的换算应为：

```go
current = (u16Code - 32768) * CodeWidth - OffsetVolt
```

其中 `CodeWidth` / `OffsetVolt` 从 `GetVoltRangeInfo` 获取（0-20mA 量程下各通道
CodeWidth≈0.000626 mA/码值）。

错误公式的斜率只有正确值的一半（约 0.49 倍），导致所有压力读数被压缩到约一半：
- 打压 -1000Pa（真实 4mA，码值约 39204）：错误算成 11.96mA → 0Pa，校零后显示 -480Pa
- 空闲 0Pa（真实 12mA，码值约 51980）：错误算成 15.86mA → +483Pa

## 引入回归的提交

| 项 | 值 |
|---|---|
| Commit | `3d5595ea` |
| 作者 | wuzy0325 <wuzynovo@163.com> |
| 时间 | 2026-07-10 |
| 标题 | `fix(daq-p1603): 修复 FFI timeout 参数传指针导致 readLoop 立即超时退出` |

该提交的本意是修复 `ReadBinary` timeout 参数传指针的问题（真实 bug），
**顺带**把原先调用 DLL 官方 `WTNDAQ16H_ScaleBinToVolt`（正确处理双极性零点）的逻辑，
替换成 Go 端自算 `code/65535*20`，理由是"ScaleBinToVolt 在电流模式下会 Access Violation 崩溃"。

替换时误用了"单极性满量程 65535"假设，未查询 `GetVoltRangeInfo` 的权威码值参数，
从而埋下斜率错误约 2 倍的回归。第二次提交 `c1f3aced`（采样率解耦/多点平均）
保留了同一错误公式，未引入新问题也未修复。

> 注：`ScaleBinToVolt` 电流模式崩溃是真实存在的风险，规避方向正确；
> 但正确做法应是用 `GetVoltRangeInfo` 拿权威参数后在 Go 端自算，
> 而不是拍脑袋用单极性假设。本次修复正是补齐这一步。

## 结论

回归由提交 **3d5595ea**（"修复 FFI timeout"）引入：为规避 `ScaleBinToVolt`
电流模式崩溃，改用 Go 端自算电流换算，但误用 `code/65535*20` 单极性假设，
未用 `GetVoltRangeInfo` 的权威码值参数，导致 0-20mA 双极性量程的零点（码值 32768）
被当成 0，斜率错误约 2 倍。

## 修复（已实施 — 2026-08-24）

`shared/device-sdk/go/daq/hardware/daq_p1603.go`：

1. `chanScale` 新增 `codeWidth` / `offsetVolt` 字段，`buildChannelScales` 在采集启动时
   对每个启用通道调用 `GetVoltRangeInfo` 获取 0-20mA 量程的权威参数
   （失败回退理论码宽 `20/32768`，不阻断采集）。
2. `readLoop` 换算改为：
   ```go
   avgCurrent = (avgCode - 32768) * codeWidth - offsetVolt
   ```
   替代原 `avgCode / 65535 * 20`。
3. 同步修正 SetTare/GetTare 的稀疏通道索引（按 `ChannelConfig.Index` 查找而非数组下标，
   避免禁用 CH1/CH2 后通道错位）。

配套提交：`56781a5`（device-sdk，代码 + 注释 + 单测）。

## 验证

真机（192.168.3.104，打压 -1000Pa）：

| 通道 | 修复前 | 修复后 |
|---|---|---|
| 打压 -1000Pa 通道 | ~0Pa → 校零后 -480Pa | **约 -1000Pa** |
| 空闲通道 | +483Pa | **约 0Pa** |

`go build` / `go vet` / `go test ./daq/hardware`（TestDAQP1603 全绿）通过。

## 遗留注意

- **修复后需重新校零**：旧 `tareOffset`（如 483Pa）是基于错误换算记录的，必须清掉重新校零，
  否则会在正确读数上再叠加错误的 offset。
- WindLabX4 与 1604Cal 共用 device-sdk 单源，本次修复两端同时受益；
  重新打包后需在真机复测确认读数正确。
