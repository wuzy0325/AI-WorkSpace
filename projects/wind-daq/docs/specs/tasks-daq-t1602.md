# Tasks: DAQ-T1602 温度扫描阀集成到 wind-daq

> 对应 spec：[spec-daq-t1602.md](spec-daq-t1602.md)。所有设计决策以 spec 为准，本文件只做执行拆解与进度跟踪。

## Phase 0 — 前置验证（真机）✅ 2026-08-13

- [x] T0.1 Q1 公式/映射验证：✅ 定案。公式经用户确认；量程表用户提供（设备固件 10 行表）。真机交叉验证：K 型 raw 1513 → 27.7℃、T 型 raw 1369 → 27.2℃ 一致；无信号通道 raw=0 → 0℃。已回填 spec §Type Code 枚举 + 驱动 `t1602ThermocoupleRanges`
- [x] T0.2 Q3 类型写回验证：✅ FC6 运行时改写成功（读原值 → 写 → 读回校验 → 恢复原值），raw 随类型变化确认类型码参与换算

## Phase 1 — shared SDK（`shared/device-sdk/go`）✅ 2026-08-13

- [x] T1.1 `protocol/modbus/modbus.go`（248 行）：MBAP 编解码、FC3/FC4/FC6、Transaction ID 自增+回显校验、1s 响应超时、单 in-flight 串行、异常码 `*ExceptionError`、WatchdogClose（ADR-009）
- [x] T1.2 `protocol/modbus/modbus_test.go`（359 行，11 用例）：帧编解码/边界切割/异常码/ID 不匹配/超时/deadline 失效 double
- [x] T1.3 `protocol/modbus/MODBUS.md`
- [x] T1.4 `daq/core/types.go` +`DaqT1602HardwareConfig{TypeCodes [16]uint8}` +`DeviceDaqT1602`；`daq/ports/device.go` +`DaqT1602Configurable`
- [x] T1.5 `daq/hardware/daq_t1602.go`（483 行）：单连接双 Unit ID、Connect 读 16 通道类型+OnConfigSynced、FC6 逐通道写+整卡读回校验、双卡轮询换算 emit、OnLog/OnReadLoopExit
- [x] T1.6 `daq/hardware/daq_t1602_test.go`（478 行，10 用例，含 ADR-009 watchdog + deadline double，-race 通过）
- [x] T1.7 `daq/hardware/daq_t1602_real_test.go`：`DAQ_T1602_REAL=1` 门控（未运行，见 Phase 0 阻塞）
- [x] T1.8 验证：`build/vet/test` 全绿（2026-08-13 复跑确认）

## Phase 2 — wind-daq 后端（`services/api-go`）✅ 2026-08-13

- [x] T2.1 `internal/core/device/types.go`：+`DeviceDaqT1602` +`DaqT1602HardwareConfig` +Profile 字段
- [x] T2.2 `internal/ports/device.go`：+`DaqT1602Configurable`
- [x] T2.3 `internal/adapters/hardware/t1602_adapter.go`（270 行）+ 测试（146 行，10 用例）
- [x] T2.4 `default_profiles.go`（默认 192.168.3.201:502、16×TC、全 T）+ `file_profile_store.go` 兼容性测试（store 本体无需改）
- [x] T2.5 `usecase/device_manager.go`：+Get/ApplyDaqT1602Config + validate（TypeCode ≤7）+ 测试
- [x] T2.6 `pkg/types/types.go` 注册；三处工厂 + case
- [x] T2.7 `api/server.go` /daqT1602Config GET/PUT；`tests/integration/server_test.go` +TestDaqT1602ConfigHTTPFlow
- [x] T2.8 触点枚举验收：+TestScanNeverDetectsDaqT1602（scan 零改动）；sim/ 不涉及
- [x] T2.9 验证：`build/vet/test ./internal/... ./api/... ./tests/...` 全绿（2026-08-13 复跑确认）

## Phase 3 — 前端（`apps/desktop-wails/frontend`）✅ 2026-08-13

- [x] T3.1 `api/types.ts`：+`'DAQ-T-1602'` +`DaqT1602HardwareConfig{typeCodes: number[]}`
- [x] T3.2 `components/device/DaqT1602Config.vue`（~150 行）：16 通道类型下拉（J/K/T/E/R/S/B/N）+ 待校准提示条
- [x] T3.3 `DeviceManagementDrawer.vue`（净 +40 行）：类型注册 + 默认值 + 条件渲染
- [x] T3.4 `deviceStore.ts`/`i18nStore.ts`：`dev_t1602_sectionTitle`、`dev_t1602_calibrationNotice`（中英文）
- [x] T3.5 待校准显示：配置面板顶部提示条（轻量实现，校准后删提示条+i18n 键即可）
- [x] T3.6 `utils/deviceCalibration.ts`：注释排除清单 +DAQ-T-1602（白名单不动）
- [x] T3.7 验证：`npm run typecheck` + `npm run build` 通过（2026-08-13 typecheck 复跑确认）

## Phase 4 — 端到端验收

- [x] T4.1 `validate-structure.ps1`：仅 2 个**既有**失败项（顶层 `tmp/` 目录、未跟踪的 `cmd/wtnpxi-gui/main.go` 588 行），与本次改动无关
- [x] T4.2 真机验收（2026-08-13）：✅ `DAQ_T1602_REAL=1` 测试通过——16 通道类型读回、10s 轮询 48 帧 = 4.76 Hz（≈4.9 Hz 理论值）、类型写回校验通过
- [x] T4.3 spec §Success Criteria 逐条过：自动化项 + 真机项均已通过；"待校准"提示条与 i18n 键已移除（量程已定案）

## Phase 5 — 配置面板 UI 增强（2026-08-19）

- [x] T5.2 全通道类型设置：`DaqT1602Config.vue` 面板顶部新增"全通道类型"下拉（选项含量程），选择后一次性应用到全部 16 通道并复位（避免陈旧显示）
- [x] T5.3 热电偶范围显示：量程表（设备固件 10 行表，§Type Code 枚举）在图例行展示（J: -50~50 ℃ · K: 0~1200 ℃ · …）；通道下拉 hover 提示量程（`UiSelect` 选项支持可选 `title`，向后兼容）
- [x] T5.4 验证：`npm run typecheck` + `npm run build` + `vitest run` 全绿；`validate-frontend-structure.ps1 -CheckFileSize` 无新增问题（命中项均为既有）
- [x] T5.5 **T1602 独立采样频率 1~5 Hz**（2026-08-19 定案）：T1602 配置面板新增"采样频率"（`DaqT1602HardwareConfig.SampleRateHz`，默认 5，独立于全局刷新频率，**全局刷新频率保持 1~10 不动**）——`shared/device-sdk/go/daq/hardware/daq_t1602_poll.go`（`SetPollIntervalFn` + 帧节拍器：间隔动态变化即时生效、读耗时超间隔自动追赶不积压）；适配器从 `config.SampleRateHz` 动态喂驱动（ApplyConfig 即时生效）；core/shared 类型 + 默认 profile + usecase 校验同步。设 1Hz 每秒读存 1 帧，设 5Hz 全速 ~4.9Hz；其他设备不受影响。测试：驱动节流/全速默认/动态变更 3 用例 + 适配器 SampleRateHz 往返用例
- [x] T5.6 验证：`go build/vet/test`（shared SDK `-race` + wind-daq `./internal/... ./api/...`）+ 前端 `typecheck/build/test` 全绿
