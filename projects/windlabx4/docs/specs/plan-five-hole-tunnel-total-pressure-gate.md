# Plan: 五孔探针校准 — 风洞总压范围判定

> 对应 Spec：`docs/specs/spec-five-hole-tunnel-total-pressure-gate.md`

## 依赖顺序

```
core 类型 + gate 逻辑 (types.go + tunnel_total_pressure_gate.go)
        │
        ▼
引擎门控 (automatic_calibration.go)  ←— 后端单测覆盖
        │
        ▼
DTO 透传 (pkg/types/calibration.go)  ←— DTO 往返测试
        │
        ▼
前端 shared types (calibration.ts)
        │
        ▼
配置 UI (FiveHoleSettings.vue) + i18n  + 状态条 (FiveHoleMain + useTotalPressureGate)
        │
        ▼
typecheck / build / test
```

## 实施步骤

1. **core 类型与门控纯函数**
   - `types.go`：`Config` 增 `TunnelTotalPressureGate *TunnelTotalPressureGateConfig`；
     新增 `TunnelTotalPressureGateConfig{Enabled, MinTotalPressure, MaxTotalPressure, TimeoutSec}`。
   - `tunnel_total_pressure_gate.go`：`NormalizeTunnelTotalPressureGate(config)`、
     `findFiveHoleTotalPressureChannel(config)`、`IsTotalPressureInRange(gate, value)`、
     `ValidateTunnelTotalPressureGate(config)`。
   - 验证：`tunnel_total_pressure_gate_test.go` 单测。

2. **引擎门控**
   - `automatic_calibration.go`：`processPoint` 在球罐闸门与暂停检查之后、采集之前插入
     `waitForTotalPressureGateIfNeeded(ctx)`；新增该等待函数（镜像
     `waitForSphereTankGateIfNeeded`：轮询 100ms、尊重暂停/取消、超时默认 300s 后 `Stop()` 报错）。
   - 仅 `config.Type == string(TypeFiveHole)` 生效。
   - 验证：`automatic_total_pressure_gate_test.go` 引擎级测试（范围内采集 / 延迟进入范围 /
     超时停止 / 非五孔忽略）。

3. **DTO 透传**
   - `pkg/types/calibration.go`：`CalibrationConfigDTO` 增字段 + `ToCore` 透传。
   - 验证：`pkg/types/calibration_test.go` 往返含 `tunnelTotalPressureGate`。

4. **前端 shared types**
   - `shared/types/calibration.ts`：`TunnelTotalPressureGateConfig` 接口 +
     `CalibrationConfig.tunnelTotalPressureGate?`。
   - 验证：`npm run typecheck`。

5. **前端配置 UI + 状态展示**
   - `FiveHoleSettings.vue`：新增"风洞总压范围判定"配置块（使能开关 + 范围上下限 + 超时），
     step-1 校验（启用时 min < max），`saveConfig`/`loadSavedConfig` 读写。
   - `i18nStore.ts`：新增 `fh_*` 文案（zh + en）。
   - `useTotalPressureGate.ts`：订阅快照读 pTotal，计算 `isInRange/statusText`。
   - `FiveHoleMain.vue`：底部状态条显示当前总压与"等待范围内/范围内"状态。
   - 验证：`npm run typecheck && npm run build`。

## 风险与对策

| 风险 | 对策 |
|---|---|
| `fiveHole.pTotal` 未映射 / 设备未订阅 | `ValidateTunnelTotalPressureGate` 返回明确错误；pTotal 本就属五孔必需通道，`CollectAcquisitionDeviceIds` 已订阅其设备 |
| 等待期间暂停/停止/取消 | 等待循环内复用 `waitWhilePaused` + `sleepContext(ctx)`，与球罐闸门一致 |
| 引擎对 runtime 为 nil 的 nil 指针风险 | gate 等待仅在 runtime 非 nil 时读取通道，nil 时继续轮询（与球罐闸门读取失败路径一致） |
| 超时停止语义 | 与球罐闸门一致：`a.Stop()` + 返回超时错误，调用方以测点失败处理（`StopOnError` 控制） |

## 并行性

- 后端步骤 1-3 串行；前端步骤 4 依赖步骤 3 的字段名，但类型可先定义（并行无碍）。
- 验证命令集中在各步。

## 验收检查点

1. 后端：`go test ./internal/core/calibration/... ./pkg/types/...` 全绿
2. `go vet ./...`
3. 前端：`npm run typecheck && npm run build`
4. 手动冒烟（可选）：配置保存/加载往返