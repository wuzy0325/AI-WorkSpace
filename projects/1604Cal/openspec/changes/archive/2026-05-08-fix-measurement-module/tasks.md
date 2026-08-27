## 1. 报警阈值计算修正

- [x] 1.1 修改 `internal/workflow/alarm_service.go:EvaluateMultiChannel`：将阈值公式从 `|maxPressure| × precisionThreshold / 100` 改为 `(maxPressure - minPressure) × precisionThreshold`
- [x] 1.2 在 `EvaluateMultiChannel` 签名中增加 `minPressure` 参数
- [x] 1.3 更新 `checkAlarm` 调用方传入 `config.MinPressure`
- [x] 1.4 更新 `internal/workflow/alarm_service_test.go` 中的现有测试用例，添加量程为 0 时的降级测试用例

## 2. 多次采样平铺存储

- [x] 2.1 在 `internal/domain/pressure_point.go` 中保留 `AverageSamples`（报告层仍需使用），移除 `ManualCollect` 中对 `AverageSamples` 的调用
- [x] 2.2 修改 `internal/application/measurement/collector.go:ManualCollect`：采样后直接平铺拼接 `samples` 为 `CollectedData`，不经过平均值计算
- [x] 2.3 更新 `ManualCollect` 相关单元测试，验证平铺后数组长度为 `通道数 × 重复采样次数`

## 3. 稳定时长默认值修正

- [x] 3.1 修改 `internal/application/measurement/collector.go` 中 `StableWaitMs` 默认值：2000 → 5000

## 4. 稳定性监控增加 SCPI 设备判稳分支

- [x] 4.1 在 `internal/device/interfaces.go` 中定义 `StabilityStatusProvider` 接口（含 `IsStable() bool` 方法）
- [x] 4.2 在 SCPI 设备驱动（ConST811A/ConST820/ConST860）中实现 `StabilityStatusProvider` 接口
- [x] 4.3 修改 `internal/application/measurement/collector.go:waitForMeasurementStability`：使用类型断言检查 `PressureDriver` 是否实现了 `StabilityStatusProvider`，是则走设备判稳路径
- [x] 4.4 更新 `service_test.go` 中的稳定性测试用例

## 5. UI 标签、约束与精度等级修正

- [x] 5.1 修改 `web/src/components/measurement/MeasurementParamsPanel.vue`：`精度:` → `显示精度:`
- [x] 5.2 `平均:` → `重复采样:`，`averageCount` 上限 20 → 10
- [x] 5.3 `精度 Level` → `精度等级`
- [x] 5.4 精度等级输入限制为离散值 `[0.0001, 0.0002, 0.0005, 0.001, 0.002]`，移除自由输入复选框
- [x] 5.5 验证 UI 编译通过（`npm --prefix web run typecheck`）

## 6. 全面验证

- [x] 6.1 运行 `go test ./cmd/... ./internal/...` 确保全部通过
- [x] 6.2 运行 `make check` 通过完整质量门禁
