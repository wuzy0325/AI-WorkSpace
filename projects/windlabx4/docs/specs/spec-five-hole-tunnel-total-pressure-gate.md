# Spec: 五孔探针校准 — 风洞总压范围判定

## Objective

五孔探针校准新增一个配置项"风洞总压范围判定"。用户可配置：

- **使能开关**：是否启用总压范围判定
- **总压范围 [min, max]**（单位 Pa，与 `fiveHole.pTotal` 通道原始读数同口径）
- **超时时间**（秒，缺省 300 秒，与球罐闸门一致）

校准运行时（逐测点采集前）：若已使能，则读取当前 `fiveHole.pTotal` 通道值，
判定其是否在 `[min, max]` 范围内；**在范围内 → 立即采集；否则轮询等待，直到进入范围或超时**。

### 已确认的决策（与用户对齐）

| 决策点 | 结论 |
|---|---|
| 总压数据来源 | **复用 `fiveHole.pTotal` 探针通道**（风洞总压），不新增独立通道选择 UI |
| 超时行为 | **停止整个校准**并返回错误（与球罐闸门门控超时一致） |
| 适用范围 | **仅五孔探针校准**（引擎层实现通用 gate，但只在五孔配置 UI 暴露） |

## 现状（对齐的既有模式）

本功能完全镜像既有"球罐闸门（SphereTankGate）"模式：

- `AutomaticCalibration.processPoint`（`internal/core/calibration/automatic_calibration.go:242`）流程：
  `move → dwell → sphereTankGate → acquire → save → progress`，新增的总压范围判定插入
  **球罐闸门之后、采集之前**。
- 门控配置进入 `calibration.Config`（`types.go`），经 `pkg/types.CalibrationConfigDTO` 透传，
  前端 `FiveHoleSettings.vue` 配置并随 `saveConfig` 落盘。
- 引擎门控等待函数：`waitForSphereTankGateIfNeeded`（`automatic_calibration.go:575`）。

## Tech Stack

- Go 后端（六边形架构）：`services/api-go/internal/core/calibration`、`pkg/types`
- Vue 3 + Wails 前端：`apps/desktop-wails/frontend`

## Commands

```powershell
# 后端
cd projects\windlabx4\services\api-go
go build -buildvcs=false ./...
go vet ./...
go test ./internal/core/calibration/... ./pkg/types/... ./internal/...

# 前端
cd projects\windlabx4\apps\desktop-wails\frontend
npm run typecheck
npm run build
npm run test
```

## Project Structure

```
services/api-go/internal/core/calibration/
  types.go                          → Config 新增 TunnelTotalPressureGate 字段 + 新配置类型
  tunnel_total_pressure_gate.go     → 新增：标准化/校验/范围内判定/通道查找
  tunnel_total_pressure_gate_test.go→ 新增：单元测试
  automatic_calibration.go          → processPoint 插入 gate 调用 + waitForTotalPressureGateIfNeeded
  automatic_total_pressure_gate_test.go → 新增：引擎级测试
services/api-go/pkg/types/
  calibration.go                    → DTO 新增字段 + ToCore 透传
  calibration_test.go               → DTO 往返测试
apps/desktop-wails/frontend/src/
  shared/types/calibration.ts       → 新增接口 + CalibrationConfig 字段
  composables/useTotalPressureGate.ts → 新增：实时总压状态 composable
  components/calibration/five-hole/FiveHoleSettings.vue → 配置 UI（使能 + 范围 + 超时）
  components/calibration/five-hole/FiveHoleMain.vue     → 底部状态条（等待/范围内提示）
  stores/i18nStore.ts               → i18n（zh + en）
```

## Code Style

镜像 `sphere_tank_gate.go` 的既有风格（中文注释、归一化 + 校验 + 判定函数拆分）：

```go
// TunnelTotalPressureGateConfig 风洞总压范围判定配置（五孔探针校准专用）
type TunnelTotalPressureGateConfig struct {
	Enabled          bool    `json:"enabled"`               // 是否启用总压范围判定
	MinTotalPressure float64 `json:"minTotalPressure"`      // 风洞总压范围下限（Pa）
	MaxTotalPressure float64 `json:"maxTotalPressure"`      // 风洞总压范围上限（Pa）
	TimeoutSec       int     `json:"timeoutSec,omitempty"`  // 判定总超时（秒），<=0 时使用默认 300 秒
}

// IsTotalPressureInRange 判断风洞总压值是否在配置范围内（闭区间）
func IsTotalPressureInRange(gate *TunnelTotalPressureGateConfig, value float64) bool {
	return value >= gate.MinTotalPressure && value <= gate.MaxTotalPressure
}
```

命名约定：`TunnelTotalPressureGate*` 前缀；JSON key 一律 camelCase；
core 层不新增任何 I/O，仅依赖 `RuntimeAccess.GetChannelValue`。

## Testing Strategy

- **框架**：Go `testing`（后端）+ Vitest（前端，仅既有模式）。
- **覆盖要求**：
  - `tunnel_total_pressure_gate_test.go`：归一化（nil/禁用→nil）、`IsTotalPressureInRange` 闭区间边界、`Validate`（缺 pTotal 通道 / min>max / 禁用→nil）。
  - `automatic_total_pressure_gate_test.go`（引擎级，复用 `fakeCalibrationRuntime`）：
    1. 启用 + 值在范围内 → 正常采集，得到数据点
    2. 启用 + 初始值在范围外 → 等待，测试侧改 `runtime.values` 进入范围后 → 正常采集
    3. 启用 + 永不进范围 + 短超时 → 超时停止校准，返回超时错误，`IsRunning()==false`
    4. 非五孔类型 + 配置了 gate → 忽略 gate 正常执行
  - `pkg/types/calibration_test.go`：DTO 往返含 `tunnelTotalPressureGate`。
- 前端：`npm run typecheck` + `npm run build` 为准；`useTotalPressureGate` 轻量逻辑随 main 页验证。

## Boundaries

- **Always**：运行后端 `go test` 与前端 `typecheck/build`；gate 仅 `TypeFiveHole` 生效；
  `fiveHole.pTotal` 通道缺失或 `min > max` 时必须返回明确校验错误。
- **Ask first**：把该 gate 扩展到其他校准类型（三孔/七孔/总压）；改动 `fiveHole.pTotal`
  通道读取语义（表压/绝压）。
- **Never**：在 `core/` 引入硬件/文件/网络 I/O；在 `apps/desktop-wails/backend` 放业务逻辑；
  修改 `project.nsi`。

## Success Criteria

- [ ] 五孔配置 UI 可编辑"总压范围判定"：使能开关、范围下限/上限、超时秒数，保存后落盘并在下次加载还原。
- [ ] 启用后运行五孔校准：总压在范围内测点正常采集；范围外测点等待直至进入范围。
- [ ] 等待超时（默认 300s / 自定义）后校准停止并返回明确错误。
- [ ] 未启用时行为与现在完全一致（回归：现有五孔测试全绿）。
- [ ] 后端单测 + `go vet` + 前端 `typecheck/build` 通过。

## Code Review 修复（2026-08-27）

| 问题 | 修复 |
|---|---|
| Critical: 门控超时在 `StopOnError=false`（前端默认）下被当作成功完成 | 新增哨兵 `ErrGateConditionFailed`，`runCalibrationLoop` 对门控超时无条件返回错误（不受 `StopOnError` 影响）；球罐门控超时同样修复。回归测试：`TestTotalPressureGateTimeoutStopsCalibrationUnconditionally`、`TestSphereTankGateTimeoutStopsCalibrationUnconditionally` |
| Critical: 门控配置在探针移动/驻留后才校验 | `CalibrationManager.Start` 预启动阶段（五孔类型）调用 `ValidateTunnelTotalPressureGate`，非法配置在发布运行态前拒绝。测试：`TestCalibrationManagerStartRejectsInvalidTotalPressureGate` |
| Critical: 门控接受可能过期的缓存总压 | 进入门控时记录 `fiveHole.pTotal` 设备时间戳，仅接受时间戳推进（当前采集周期新帧）且在范围内的读数；设备无时间戳能力时退化为仅按值判定。测试：`TestTotalPressureGateRejectsStaleCachedValue`、`TestTotalPressureGateAcceptsFreshTimestampValue` |
| Important: 范围边界未验证 NaN/Inf、null 解码为 [0,0] | `ValidateTunnelTotalPressureGate` 拒绝 NaN/Inf 边界与 `[0,0]`（null 解码形态）。测试：`TestValidateTunnelTotalPressureGateRejectsNonFiniteAndZeroRange` |