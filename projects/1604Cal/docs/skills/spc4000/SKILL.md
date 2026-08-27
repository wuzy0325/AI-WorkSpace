---
name: spc4000
description: SPC4000 (Scanivalve 压力校准器，基于 Mensor CPC6000 平台) 打压设备驱动开发指南。涵盖 Mensor 远程命令集、TCP 通信、控制/测量模式切换、单位系统、量程查询与稳定判定。Use when writing or modifying SPC4000 driver code, debugging pressure control commands, adding SPC4000 features, or when user mentions SPC4000, Scanivalve pressure calibrator, Mensor command set, or pressure control mode.
---

# SPC4000 打压设备驱动开发

## 1 设备概述

SPC4000 是 Scanivalve 公司的压力校准器，底层基于 **Mensor CPC6000** 平台。
默认远程命令集为 **Mensor 指令集**（非标准 SCPI）。命令规范见
`docs/spc4000_v1_web.pdf` 第 4 章「Remote Operations」Table 4.1。

| 参数 | 值 |
|------|-----|
| 协议 | TCP/IP（SCPI 风格，`<cmd>\r\n` 结尾） |
| 命令集 | Mensor（非标准 SCPI，命令为短助记词） |
| 通道 | 最多 2 个独立控制通道（A/B），每通道最多 2 个传感器 |
| 量程前缀 | 可选 `1`~`4` 前缀指定传感器（如 `1RP`、`3GP 53.47`） |

**关键区别**：Mensor 命令集与 ConST 811A/820/860 的标准 SCPI 命令**完全不同**，
不要把 ConST 的 `:STABle?`、`MODE CONTROL` 等命令套用到 SPC4000 上。

## 2 核心命令语义（务必理解）

| 命令 | 说明 |
|------|------|
| `GP <value>` / `GN <value>` | 设定正/负目标压力，**同时隐含切入控制模式** |
| `Measure` | 切入 Measure（陷阱）模式，停止打压并隔离气路 |
| `ZO` | 泄压（Vents any pressure）+ 正/负气路短接，等价本地 [Vent] 键 |
| `RP` | 读当前压力（可带 `1`~`4` 量程前缀） |
| `Mode?` | 返回当前操作模式字符串（CONTROL / MEASURE 等） |
| `Measure?` | 返回 `YES`（Measure 模式）/ `NO`（其他） |
| `Units <code>` | 设置单位（可用数字代码或文本） |
| `Units?` | 返回单位文本字符串 |
| `RangeMin?` / `RangeMax?` | 返回当前传感器量程上下限（单值） |

**两个关键限制**：
- **无独立「仅进控制模式」命令**：控制模式只能通过 `GP`/`GN` 下发目标压力触发。
- **无「是否稳定」查询命令**：稳定判定需软件侧采样判稳（见第 5 节）。

## 3 命令速查表

> 写命令（不含 `?`）设备通常**不回复**；查询命令（含 `?`）返回 `<sp>{value}<cr><lf>`。
> `tcpConnectionDriver.sendSCPICommand` 已对不含 `?` 的命令跳过读取，避免超时阻塞。

### 3.1 压力控制

| 命令 | 功能 |
|------|------|
| `GP <value>` | 切入控制模式并设定正压力（`value ≥ 0`） |
| `GN <value>` | 切入控制模式并设定负压力（`value` 为绝对值） |
| `Measure` | 切入 Measure（陷阱）模式，停止打压 |
| `ZO` | 泄压 + 正/负气路短接（泄压到大气） |

### 3.2 压力读取

| 命令 | 功能 |
|------|------|
| `RP` | 读当前压力（`<n>RP` 指定第 n 个传感器，`n=1~4`） |
| `RP/C` | 连续返回压力（`<esc><cr>` 终止，仅调试用） |

### 3.3 模式查询

| 命令 | 返回 | 功能 |
|------|------|------|
| `Mode?` | `{string}` | 当前操作模式 |
| `Measure?` | `YES` / `NO` | 是否处于 Measure 模式 |

### 3.4 单位

| 命令 | 功能 |
|------|------|
| `Units <code>` | 设置单位（数字代码或文本均可） |
| `Units?` | 返回单位**文本字符串**（如 `psi`） |

### 3.5 量程

| 命令 | 返回 | 功能 |
|------|------|------|
| `RangeMin?` | `{value}` | 当前传感器量程下限（当前单位） |
| `RangeMax?` | `{value}` | 当前传感器量程上限（当前单位） |

### 3.6 稳定相关配置（只写参数，无只读标志）

| 命令 | 功能 |
|------|------|
| `Stabletime {0~65535}` | 设置稳定时间（秒） |
| `Stabledelay {0~65535}` | 设置稳定延迟（秒） |
| `StableWin {%fs}` | 设置稳定窗口（%FS） |

## 4 单位系统（数字代码映射）

手册附录 Table 11.1 定义单位数字代码，`Units` 命令可传代码或文本。
驱动 `SetUnit` 使用数字代码（见 `pressureUnitToCodeSPC4000`）：

| 单位 | 代码 | 单位 | 代码 |
|------|------|------|------|
| psi | 1 | mmHg | 19 |
| atm | 13 | kPa | 22 |
| bar | 14 | Pa | 23 |
| mbar | 15 | kgf/cm² | 26 |
| MPa | 36 | | |

**注意**：`Units?` 返回的是**文本字符串**（如 `psi`），不是数字代码。
解析时应直接按文本规范化（`NormalizePressureUnit`），**不要**用数字码查表。

## 5 稳定判定（软件判稳）

Mensor 命令集**没有**类似 ConST `:STABle?` 的只读稳定标志。
稳定状态由 `Stabletime` / `StableWin` 参数定义，但无标志可查。

驱动 `ReadStability` 采用软件判稳：
1. 尽量用 `RangeMax?` 获取满量程（自适应阈值），失败退回默认满量程。
2. 连续采样 N 次 `RP`（默认 5 次，间隔 200ms）。
3. 若首末采样压力极差 ≤ 阈值（默认 0.05% 满量程），判定稳定。

```go
stable := math.Abs(last-first) <= span*fracThreshold // fracThreshold = 0.0005
```

## 6 控制模式切换（StartControl 语义）

由于无独立「进控制模式」命令，`StartControl` 实现为：
1. `Mode?` 查询当前模式，若已为 `CONTROL` 则直接返回。
2. 否则读当前压力 `RP`，用 `GP`/`GN` **重发当前压力**触发切入控制模式。

> ⚠️ 这意味着 StartControl 不改变目标压力，只是把设备从 Measure 拉回 Control。
> 若业务流程是 StartControl 之后紧跟 SetTargetPressure，此实现是安全且幂等的。

## 7 接口契约（PressureDriver）

驱动实现 `device.PressureDriver` 接口，方法映射：

| 接口方法 | Mensor 命令 | 说明 |
|----------|-------------|------|
| `SetTargetPressure` | `GP`/`GN` | target≥0 用 GP，否则 GN 用绝对值 |
| `Stop` | `Measure` | 停止打压（陷阱模式） |
| `Exhaust` | `ZO` | 泄压 + 气路短接 |
| `ReadCurrentPressure` | `RP` | 解析 `parseSCPIPressure` |
| `ReadUnit` | `Units?` | 文本规范化 |
| `SetUnit` | `Units <code>` | 数字代码 |
| `ReadStability` | （软件判稳） | 采样 RP 判极差 |
| `StartControl` | `Mode?` + `GP`/`GN` | 拉回控制模式 |
| `ReadTargetRange` | `RangeMin?` + `RangeMax?` | 返回 (min, max) |

## 8 关键注意事项

| 条目 | 说明 |
|------|------|
| 无 `Vent` 命令 | 泄压用 `ZO`，不要发 `Vent`（ConST 习惯命令，SPC4000 无此命令） |
| 无 `:STABle?` | 稳定判定需软件采样，勿套 ConST 的 `:STABle?` |
| 无独立控制命令 | 控制模式只能靠 `GP`/`GN` 触发 |
| `Units?` 返回文本 | 用文本规范化，非数字码查表 |
| 量程返回单值 | `RangeMin?`/`RangeMax?` 各返回一个值，非 `min,max` 组合 |
| 写命令无响应 | 不含 `?` 的命令不读响应，避免超时阻塞 |
| 量程前缀 | 命令可带 `1~4` 前缀指定传感器（如 `2GN 10`、`3GP 53.47`） |

## 9 参考资料

- 完整命令表：`REFERENCE.md`（手册 Table 4.1 全部命令，含驱动未接入的命令）
- 手册：`docs/spc4000_v1_web.pdf`（第 4 章 Remote Operations，Table 4.1）
- 驱动：`internal/infrastructure/driver/spc4000_driver.go`
- 单位映射：`internal/infrastructure/driver/helpers.go`（`pressureUnitToCodeSPC4000`）
- 工厂注册：`internal/infrastructure/driver/factory.go`（型号 `SPC4000`）
