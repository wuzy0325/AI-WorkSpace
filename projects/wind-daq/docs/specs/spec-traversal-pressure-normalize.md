# Spec: 遍历测试压力通道归一化

> 来源：interview-me → spec-driven-development Phase 1
> 日期：2026-07-10
> 状态：待审批
> 修订：v2（2026-07-10，依据代码 review 修复 P0/P1/P2 共 9 项问题）

## Objective

**目标**：在遍历测试模块建立"压力通道归一化规约"——任何设备配置（表压/绝压、任意单位）的通道数据，在喂给插值器前都自动归一化到「Pa + 表压」，消除当前"插值器隐含假设 Pa+表压"的安全隐患。

**用户**：风洞测试工程师（配置各种压力传感器的实际操作者）+ 应用维护者（避免隐含假设导致未来 bug）。

**背景**：当前 `BuildRawPressure`（[traversal_view.go:145](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal_view.go#L145)）只做 `channelIndex → label` 的简单映射，把 `values[chIdx]` 原封不动填入 `InterpolationInput`。但插值器入口 [five_hole_new_interpolator.go:633](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/shared/algorithms/go/fivehole/interpolation/five_hole_new_interpolator.go#L633) 通过 `P0+input.PAtm` 计算绝压，`InterpolationResult` 注释明确标注"表压 Pa"——即插值器隐含假设 P1-P5 为「Pa+表压」、PAtm 为「Pa+绝压」。一旦用户配置 kPa/MPa/psi 单位或绝压传感器，插值结果将静默错误。

**成功画面**：
1. 遍历测试配置 UI 新增"五孔压力类型"全局开关（表压/绝压），默认表压
2. **约束**：P1-P5 必须为同一压力类型（5 孔传感器物理上同型号），由全局开关统一表达；Patm 始终视为绝压
3. P1-P5 若为绝压传感器，运行时自动 `P_gauge = P_abs - Patm` 转换
4. 通道单位为 kPa/MPa/psi/kgf/cm² 时，运行时自动换算到 Pa
5. Patm 通道只做单位换算，不做类型转换（本身就是绝压值）
6. 插值器入口数据始终是「Pa + 表压」（P1-P5）/「Pa + 绝压」（PAtm）
7. `go build/test/vet` + `npm typecheck/build` 全绿

## Tech Stack

| 层 | 技术 | 版本 |
|---|---|---|
| 后端 | Go | 1.25（go.work 主干） |
| 前端 | Vue 3 + TypeScript + Vite + Naive UI | 与 wind-daq 现有一致 |
| 插值器 | `shared/algorithms/go/fivehole/interpolation` | 已有，不修改 |
| 测试 | Go `testing` + Vitest | 已有 |

## Commands

```powershell
# Backend
cd projects\wind-daq\services\api-go
go build -buildvcs=false ./...
go vet ./...
go test ./internal/...

# Frontend
cd projects\wind-daq\apps\desktop-wails\frontend
npm run typecheck
npm run build
```

## Architecture Changes

> **本章节为 v2 新增**：归一化需要 TraversalManager 能查询每个通道的 `Unit`，但当前 `TraversalManager` 只持有 `ports.LatestDataReader`，该接口（[ports/calibration.go:8](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/ports/calibration.go#L8)）只暴露 `GetLatestData` / `GetLatestTimestamp`，**不暴露 ChannelConfig**。必须新增端口。

### A1. 新增端口 `ChannelUnitProvider`

位置：`internal/ports/traversal.go`（已有文件，追加接口）

```go
// ChannelUnitProvider 提供指定设备通道的单位查询。
// 用于遍历测试压力归一化：BuildRawPressure 需查每通道 Unit 才能换算到 Pa。
// 实现由 DeviceManager 提供（持有 profiles），避免 TraversalManager 直接依赖 usecase 兄弟包。
type ChannelUnitProvider interface {
    // ChannelUnit 返回指定设备通道的工程单位字符串（如 "Pa"/"kPa"/"MPa"）。
    // 通道不存在时返回 "" 与 error；调用方按 error 决定是否中断归一化。
    ChannelUnit(deviceID string, channelIndex int) (string, error)
}
```

**设计理由**：
- 不直接复用 `DeviceManager.GetDAQP1603Config(id)`：返回完整 `device.Profile` 暴露过多内部结构，且仅 DAQ-P-1603 实现
- 不直接注入 `DeviceManager` 引用：违反 usecase 兄弟包解耦原则，且 `DeviceManager` 接口过宽
- 抽象为窄接口（ISP 原则）：未来其他设备类型只需实现该接口即可参与归一化

### A2. `DeviceManager` 实现 `ChannelUnitProvider`

位置：[device_manager.go](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/device_manager.go)（追加方法）

```go
// ChannelUnit 实现 ports.ChannelUnitProvider。
// 从 profiles 中查找设备→通道→Unit；找不到返回 error 让 BuildRawPressure 走兜底。
func (m *DeviceManager) ChannelUnit(deviceID string, channelIndex int) (string, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    profile, ok := m.findProfileLocked(deviceID)
    if !ok {
        return "", fmt.Errorf("device profile not found: %s", deviceID)
    }
    for _, ch := range profile.Channels {
        if ch.Index == channelIndex {
            return ch.Unit, nil
        }
    }
    return "", fmt.Errorf("channel %d not found in device %s", channelIndex, deviceID)
}
```

### A3. `TraversalManager` 注入 `ChannelUnitProvider`

位置：[traversal.go:48](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal.go#L48) 与 [traversal.go:91](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal.go#L91)

- 结构体新增字段 `unitProvider ports.ChannelUnitProvider`（带 `omitempty` JSON 不需要——是 Go 字段非 JSON 字段）
- 构造函数追加可变参数（保持向后兼容，避免破坏现有 10 处调用点）：

```go
func NewTraversalManager(
    reader ports.LatestDataReader,
    motion ports.MotionAccess,
    sink ports.TraversalPointSink,
    store ports.TraversalResultStore,
    checkpointStore ports.CheckpointStore,
    configStore ...ports.AppConfigStore,
    // v2 归一化：单位查询端口，可空（旧测试兼容）
    unitProvider ...ports.ChannelUnitProvider,
) *TraversalManager
```

- 同步增加 setter `SetUnitProvider(p ports.ChannelUnitProvider)`，供 [bootstrap.go:94](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/bootstrap/bootstrap.go#L94) 与 [apiserver.go:80](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/pkg/apiserver/apiserver.go#L80) 与 [context.go:94](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/pkg/appcontext/context.go#L94) 三处装配点注入
- 装配点改为：`travMgr.SetUnitProvider(manager)`（在 `manager` 创建之后调用，与现有 `SetInterpolatorLoader` 模式一致）

### A4. `BuildRawPressure` 签名与调用方改造

位置：[traversal_view.go:145](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal_view.go#L145)

**新签名**：

```go
// BuildRawPressure 从通道值构建原始压力数据和插值输入。
// - unitProvider 用于查每通道 Unit，nil 时跳过单位换算（兼容旧测试）
// - pressureType "gauge"|"absolute"，空串按 "gauge" 兜底
// - deviceID 用于 unitProvider 查询
// 归一化策略：
//   - P1-P5：按 Unit 换算到 Pa；若 pressureType=="absolute" 再减去已归一化的 Patm
//   - Patm：仅按 Unit 换算到 Pa（绝压值，不减）
//   - Tatm：不参与归一化（温度通道）
func BuildRawPressure(
    values map[int]float64,
    labels map[int]string,
    deviceID string,
    unitProvider ports.ChannelUnitProvider,
    pressureType string,
) (rawPressure map[string]float64, input coreinterp.InterpolationInput, ok bool)
```

**两处调用方同步改造**：
- [traversal_view.go:120](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal_view.go#L120) `BuildDataPoints`：从 `m.config.DeviceID` + `m.unitProvider` + `m.config.PProbePressureType` 取参
- [traversal_acquisition.go:228](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal_acquisition.go#L228) `RunTraversalLoop`：同上

**降级策略**：`unitProvider == nil` 或查询单位失败时，记录 warning 日志并跳过换算（保持原始值），避免破坏旧测试与离线场景。

### A5. CheckPreconditions 新增通道校验

位置：[traversal.go:193](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal.go#L193)

当前 `CheckPreconditions` 只校验 PRB/Motion/DAQ 三类资源，**完全没有通道级校验**。本次新增：

```go
// 新增校验项 ChannelMap：检查 ChannelLabels 是否包含 P1-P5 + Patm + Tatm 全部 7 个标签
hasPatm := false
hasTatm := false
for _, label := range m.config.ChannelLabels {
    switch label {
    case "Patm":
        hasPatm = true
    case "Tatm":
        hasTatm = true
    }
}
checks = append(checks, map[string]any{
    "name":    "ChannelMap",
    "passed":  hasPatm && hasTatm,
    "message": func() string {
        if !hasPatm {
            return "Patm channel label is required for pressure normalization"
        }
        if !hasTatm {
            return "Tatm channel label is required for atmospheric calculation"
        }
        return "All required channel labels are mapped"
    }(),
})
```

P1-P5 标签存在性校验由 `BuildRawPressure` 返回的 `ok` 标志在运行时承担（已有逻辑），不重复校验。

## Project Structure

```
projects/wind-daq/services/api-go/internal/
├── core/
│   ├── pressure/                     # 新建：共享压力归一化工具包
│   │   ├── normalize.go              # NormalizePressureToGaugePa（委托 device.UnitConverter 做单位换算）
│   │   └── normalize_test.go         # 绝压转表压 + 未知单位报错 + Patm 不减 + 旧配置兜底
│   └── traversal/
│       └── types.go                  # 修改：Config 新增 PProbePressureType 字段（line 52-68）
├── ports/
│   └── traversal.go                  # 修改：新增 ChannelUnitProvider 接口
├── usecase/
│   ├── traversal.go                  # 修改：TraversalManager 注入 unitProvider + CheckPreconditions 通道校验
│   ├── traversal_view.go             # 修改：BuildRawPressure 新签名 + 归一化逻辑
│   ├── traversal_config.go           # 修改：traversalAPIConfig 新增字段 + ParseAndStartTraversal 映射
│   ├── traversal_acquisition.go      # 修改：BuildRawPressure 调用方适配
│   └── device_manager.go             # 修改：实现 ChannelUnitProvider.ChannelUnit
└── adapters/storage/                  # 不修改：CSV 录制层保持原样（写原始通道值）

projects/wind-daq/apps/desktop-wails/frontend/src/
├── shared/types/
│   └── traversal.ts                  # 修改：TraversalTestConfig 新增 pProbePressureType（line 421-442）
└── components/traversal/
    ├── TraversalSettings.vue         # 修改：新增"五孔压力类型"开关 + saveConfig/applySavedConfig 同步（line 276-335, 351-384）
    └── TraversalHardwareStep.vue     # 修改：通道映射面板显示压力类型提示
```

## Code Style

### Go 归一化函数（新建 `core/pressure/normalize.go`）

> **v2 修订**：原 spec 提议新建独立 `UnitToPaFactor` 表（6 单位），但项目已有 [device/unit_converter.go:31](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/core/device/unit_converter.go#L31) 的 `UnitConverter`（11 单位，含 mmH2O/mmHg/bar 等），功能等价于 `ConvertToPa`。为避免维护两套系数表，归一化函数内部**委托 `device.UnitConverter.ToBaseUnit`** 做单位换算，自身只负责"绝压→表压"减法。

```go
// Package pressure 提供压力通道归一化工具。
// 单位换算委托 core/device.UnitConverter（已有 11 个压力单位注册表），
// 本包只额外负责"绝压→表压"减法逻辑，避免双套系数表。
// 后续校准模块复用同一入口，保证全应用压力数据语义统一。
package pressure

import (
    "fmt"

    "wind-daq/services/api-go/internal/core/device"
)

// PressureType 压力传感器类型
type PressureType string

const (
    PressureTypeGauge    PressureType = "gauge"    // 表压（相对大气压）
    PressureTypeAbsolute PressureType = "absolute" // 绝压（相对真空）
)

// NormalizePressureToGaugePa 将任意单位/类型的压力值归一化为 Pa + 表压。
// - converter 必须非 nil，由调用方注入（DeviceManager 持有的单例）
// - unit 工程单位字符串（如 "kPa"/"MPa"/"psi"），未知单位返回 error
// - pressureType 为空或 "gauge" 时原样换算；"absolute" 时减去 patmPa
// - patmPa 已是 Pa 单位的绝压值（由调用方事先用 ConvertToPa 归一化）
func NormalizePressureToGaugePa(
    value float64,
    unit string,
    pressureType string,
    patmPa float64,
    converter *device.UnitConverter,
) (float64, error) {
    if converter == nil {
        return 0, fmt.Errorf("pressure: UnitConverter is nil")
    }
    paValue, err := converter.ToBaseUnit(value, unit)
    if err != nil {
        return 0, fmt.Errorf("pressure: convert %q to Pa failed: %w", unit, err)
    }
    if PressureType(pressureType) == PressureTypeAbsolute {
        return paValue - patmPa, nil
    }
    return paValue, nil
}

// ConvertToPa 仅做单位换算，不做绝压→表压。
// Patm 通道专用：大气压本身就是绝对值，插值器需要的就是 Patm 绝压值。
func ConvertToPa(value float64, unit string, converter *device.UnitConverter) (float64, error) {
    if converter == nil {
        return 0, fmt.Errorf("pressure: UnitConverter is nil")
    }
    return converter.ToBaseUnit(value, unit)
}
```

### 单位字面量统一

> **v2 修订**：原 spec 与前端 `PRESSURE_UNIT_TO_PA_FACTOR`（[DaqP1603Config.vue:69](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/apps/desktop-wails/frontend/src/components/device/DaqP1603Config.vue#L69)）均使用 `'kgf/cm2'`，但 [device/unit_converter.go:45](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/core/device/unit_converter.go#L45) 注册的是 `'kgfcm2'`（无斜杠）。三处不一致。

**统一方案**：在 `unit_converter.go` 的 `pressureFamily()` 中**同时注册两个 key**（`kgfcm2` 和 `kgf/cm2`）指向同一系数 `98066.5`，作为 alias。这样：
- 前端 `'kgf/cm2'` 直接生效
- 旧代码 `'kgfcm2'` 仍兼容
- 不需要批量改前端

**mmH2O 决策**：原 spec 提议 Go 端 `UnitToPaFactor` 含 `mmH2O`，但前端 `PRESSURE_UNIT_OPTIONS` 只有 5 个单位（Pa/kPa/MPa/kgf/cm2/psi），UI 无法选 mmH2O。本次保持前端不变，Go 端因复用 `UnitConverter`（已含 mmH2O）天然支持，未来前端扩展时无需改后端。

## Testing Strategy

| 层 | 框架 | 位置 | 覆盖要点 |
|---|---|---|---|
| Go 单元测试 | `testing` | `core/pressure/normalize_test.go` | 5 种单位换算、绝压→表压、表压原样、Patm 不参与类型转换、未知单位报错、converter nil 报错 |
| Go 集成测试 | `testing` | `usecase/` 扩展 | `BuildRawPressure` 传入 PProbePressureType=absolute 时输出正确归一化值；unitProvider nil 时降级 |
| Go 集成测试 | `testing` | `usecase/` | `CheckPreconditions` 在 ChannelLabels 缺 Patm/Tatm 时返回 passed=false |
| 前端类型检查 | `npm run typecheck` | — | `TraversalTestConfig.pProbePressureType` 类型正确 |
| 前端构建 | `npm run build` | — | UI 开关渲染正常 |

### 测试用例（三段式）

| 用例 | 测试前置 | 测试步骤 | 期待结果 |
|---|---|---|---|
| Pa+表压原样 | unit="Pa", type="gauge", value=1000, Patm=101325, converter=NewUnitConverter() | NormalizePressureToGaugePa | 1000.0, nil |
| kPa+表压 | unit="kPa", type="gauge", value=5.0, Patm=101325 | NormalizePressureToGaugePa | 5000.0, nil |
| MPa+绝压 | unit="MPa", type="absolute", value=0.2, Patm=101325 | NormalizePressureToGaugePa | 98675.0, nil |
| psi+绝压 | unit="psi", type="absolute", value=14.7, Patm=101325 | NormalizePressureToGaugePa | ≈ -140.4, nil（误差 <0.1） |
| kgf/cm²+表压 | unit="kgf/cm2", type="gauge", value=1, Patm=101325 | NormalizePressureToGaugePa | 98066.5, nil |
| kgfcm2 alias | unit="kgfcm2", type="gauge", value=1 | NormalizePressureToGaugePa | 98066.5, nil（验证 alias） |
| Patm 单位换算 | unit="kPa", value=101.325 | ConvertToPa | 101325.0, nil |
| 未知单位报错 | unit="degC" | NormalizePressureToGaugePa | 0, non-nil error |
| converter nil | converter=nil | NormalizePressureToGaugePa | 0, non-nil error |
| 旧配置兜底 | JSON 无 pProbePressureType | 反序列化 Config | PProbePressureType == "gauge" |
| BuildRawPressure 集成-绝压 | 5 个 P 通道 kPa+absolute + Patm 101.325kPa + unitProvider 返回 "kPa" | BuildRawPressure | P1-P5 已归一化 Pa+表压, Patm 为 Pa 绝压 |
| BuildRawPressure 降级 | unitProvider=nil | BuildRawPressure | 跳过换算，返回原始值 + warning 日志 |
| CheckPreconditions 缺 Patm | ChannelLabels 无 "Patm" | CheckPreconditions | checks 含 ChannelMap 项 passed=false |
| CheckPreconditions 完整 | ChannelLabels 含 P1-P5+Patm+Tatm | CheckPreconditions | ChannelMap 项 passed=true |

## Boundaries

### Always do
- 提交前运行 `go build && go vet && go test ./internal/core/pressure/... ./internal/usecase/...` 与 `npm run typecheck && npm run build`
- 单位换算统一委托 `core/device.UnitConverter`，不在 `core/pressure` 重复维护系数表
- 新字段使用 `omitempty` JSON tag，保证旧配置反序列化兼容
- `PProbePressureType` 反序列化兜底为 `"gauge"`（在 `traversal.Config` 的 `UnmarshalJSON` 或 ParseAndStartTraversal 中处理）
- 所有公开函数加中文注释解释"为什么"

### Ask first
- 修改 `shared/algorithms/go/fivehole/interpolation` 包（不在本次范围）
- 修改 CSV 录制层（不在本次范围，详见下方"CSV 语义说明"）
- 新增第三方 Go 依赖
- 修改 `core/device/types.go` 的 `ChannelConfig`

### Never do
- 在 `core/pressure/` 目录导入任何硬件相关包（`adapters/hardware/*`）
- 在 `core/pressure/` 目录导入 `core/traversal` 或 `usecase`（保持依赖方向正确）
- 在 `core/pressure/` 重复维护单位→Pa 系数表（必须委托 `core/device.UnitConverter`）
- 修改 CSV 录制层（原始数据按通道原始单位记录，不改）
- 将归一化逻辑放入前端（所有归一化在后端 `BuildRawPressure` 中完成）

## CSV / API 语义说明

> **v2 新增**：澄清 CSV 与 API 响应中压力值的语义差异，避免用户困惑。

### CSV 录制层（不改）
[traversal_csv_writer.go:263](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/adapters/storage/traversal_csv_writer.go#L263) 写 CSV `SaveRawPressure` 列时直接从 `p.Values[e.Channel]` 取值，**不经过 BuildRawPressure**。意味着：
- CSV 中 P1-P5 列 = **设备原始单位 + 原始类型**（如 kPa+绝压）
- 插值器消费的 `InterpolationInput` = **Pa + 表压**

**为何不改**：
- CSV 作为原始数据归档，保留设备原生量便于审计与跨工具复用
- 归一化是插值器入口的局部语义，不应污染存档
- `SaveCalculatedResult` 列（Pt/Ps/Mach，[traversal_csv_writer.go:270-284](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/adapters/storage/traversal_csv_writer.go#L270)）已包含归一化后的派生结果，用户可据此反推

**用户提示**：未来若用户反馈 CSV 与 UI 数值不一致导致困惑，可在 CSV 表头追加单位标注（如 `P1(kPa)`），但属独立增强，不在本 spec 范围。

### API 响应 `rawPressure` 字段
[traversal_view.go:130-138](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal_view.go#L130) 的 `BuildDataPoints` 返回 `rawPressure` 字段——本次改造后该字段**改为归一化后的 Pa+表压值**（与 `InterpolationInput` 一致），不再暴露原始通道值。理由：
- 前端实时图表展示压力曲线时，单位混用（kPa 与 Pa 混杂）会导致纵轴无法标注
- 若前端需要原始值，可调用 `device.GetDAQP1603Config` 查询单位后反向换算
- `interpolationResult` 字段已包含 Pt/Ps/Mach，与 `rawPressure` 归一化后语义自洽

**Breaking Change 提示**：前端 [TraversalMain.vue](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/TraversalMain.vue) 等消费 `rawPressure` 的组件若硬编码了"kPa"标签，需同步改为"Pa"或读取通道单位。spec 实施时需 grep 前端 `rawPressure` 引用点并同步调整。

## Success Criteria

- [ ] `core/pressure/normalize.go` 包含 `NormalizePressureToGaugePa` / `ConvertToPa`，内部委托 `device.UnitConverter`
- [ ] `core/pressure/normalize_test.go` 覆盖 5 种单位 + 绝压/表压 + 未知单位 + converter nil + kgfcm2 alias
- [ ] `ports/traversal.go` 新增 `ChannelUnitProvider` 接口
- [ ] `usecase/device_manager.go` 实现 `ChannelUnitProvider.ChannelUnit`
- [ ] `traversal.Config` Go 类型新增 `PProbePressureType string` 字段，JSON 反序列化兜底为 `"gauge"`
- [ ] `traversalAPIConfig`（[traversal_config.go:301](file:///c:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/services/api-go/internal/usecase/traversal_config.go#L301)）新增 `PProbePressureType` 字段，`ParseAndStartTraversal`（line 514-526）映射到 `traversal.Config`
- [ ] `TraversalManager` 结构体新增 `unitProvider` 字段 + `SetUnitProvider` setter
- [ ] `bootstrap.go` / `apiserver.go` / `context.go` 三处装配点调用 `SetUnitProvider(manager)`
- [ ] `BuildRawPressure` 新签名（含 `deviceID` / `unitProvider` / `pressureType`），两处调用方同步适配
- [ ] `CheckPreconditions` 新增 `ChannelMap` 校验项（Patm + Tatm 标签存在）
- [ ] `TraversalTestConfig` TS 类型新增 `pProbePressureType?: 'gauge' | 'absolute'`
- [ ] `TraversalSettings.vue` 新增"五孔压力类型"开关（默认表压），`saveConfig`（line 362-376）与 `applySavedConfig`（line 276-335）同步读写该字段
- [ ] `TraversalHardwareStep.vue` 通道映射面板显示压力类型提示
- [ ] `core/device/unit_converter.go` 的 `pressureFamily()` 新增 `'kgf/cm2'` alias 指向 `98066.5`
- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] `go test ./internal/core/pressure/... ./internal/usecase/...` 通过
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过

## Open Questions

无。interview-me 阶段已全部澄清，v2 review 已修复全部 P0/P1/P2 问题。
