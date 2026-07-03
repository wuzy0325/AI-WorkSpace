# Changelog

## [0.3.3] - 2026-07-03

### Fixed

- 修复 DAQ-P-1604 通道选择器文本被截断的问题；排除大气压通道默认不显示在图表中。
- DAQ-P-1604 v0.2.4：应用层增加连续超时断连检测，优化 keepalive 参数，提升长时间采集稳定性。

### Internal

- motion 重构：移除轴启用开关，强制所有轴始终启用（简化状态管理）。

### Verification

- `go test ./internal/... ./api/...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 构建安装包: passed
- 冒烟测试: passed（GUI 启动正常）

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.2] - 2026-07-03

### Fixed

- 修复 T1603 设备停止采集后立即配置参数时命令响应乱码或失败的问题。停止采集后 TCP 缓冲区残留采集数据帧，ApplyDaqT1603Config 的 sendCommand 把残留当作命令响应读出。通过 shared/device-sdk/go/daq/hardware/daq_t1603.go 的修复，在 stopAcquisitionLocked 停止命令后增加 drainConnection 排空残留数据；在 ApplyDaqT1603Config 调用 applyHardwareConfig 前增加 drainConnection。

### Internal

- 修复点位于 shared/device-sdk 共享代码，wind-daq 的 T1603 设备适配器自动受益。
- motion-controller Taskfile.yml 中 rsrc.syso 清理逻辑改用 Test-Path 显式检查，避免 -LiteralPath -ErrorAction SilentlyContinue 在部分环境下失效。
- 同步 wails_windows_amd64.syso 上次 release build 产物。

### Verification

- `go test ./internal/... ./api/...`
- `go build -buildvcs=false ./...`
- `task release`

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.1] - 2026-07-03

### Fixed
- 修复 CSV Timestamp 列时间戳精度问题：采集 CSV 与测点遍历 CSV 统一截断到秒级（`'YYYY-MM-DD HH:MM:SS`），避免展示错误的时间细分。原因详见 daq-p1604 v0.2.2 release note（DAQ-P-1604 设备硬件时间戳固件 bug）。

### Internal
- Taskfile `generate-icon` 任务改用 `build/windows/info.json` 和 `build/windows/wails.exe.manifest` 模板文件，`wails3 generate syso` 内部用 `wails.json` 的 info 字段渲染模板。删除冗余的具体值版本 `build/info.json` 和 `build/windows.manifest`，版本号源从 7 个收敛到 6 个。
- 重新生成 `wails_windows_amd64.syso` 资源段。
- 同步更新 `build/config.yml` 的 `info.version` 字段，对齐项目版本号。

### Verification
- `go build ./...`: passed
- `go vet ./internal/adapters/storage/...`: passed
- `go test ./internal/adapters/storage/...`: passed

### Known Issues
- 暂无。

## [0.3.0] - 2026-07-02

### Added
- CSV sink 按设备切文件：DAQ-P-1604 采用 18 通道宽格式，其余设备长格式；文件名 `prefix-deviceId-YYYYMMDD-HHMMSS-NNN.csv`。
- DAQ-P-1604 硬件时间戳开关（`DaqP1604UseDeviceTimestamp` 改为三态 *bool，nil 视为开启）。
- B140 编码器补偿编辑器 UI（预设/自定义参数、实时 warning 校验、光栅尺精度约束提示）。
- DataPayload 新增 DeviceType 字段，填充 6 个 hardware adapter。

### Changed
- rotation 路径持 statsMu 写锁，修复与 Status() 的 data race。
- T1603 驱动同步化 Connect 时序：OnConfigSynced 回调在 Connect 内同步触发。

### Fixed
- 修复 wind-daq T1603Adapter.Connect() 自死锁：适配器持 a.mu 调 dev.Connect()，而同步 OnConfigSynced 回调重入 a.mu 导致永久死锁。
- 修复 T1603 硬件时间戳开关不生效：驱动层原本无条件下发 `@fe TIME 0` 并将 ShowTimestamp 置 false，导致 UI 开关一直无效；改为按 `d.config.ShowTimestamp` 下发。

### Internal
- shared/device-sdk T1603 驱动重构：Connect/OnConfigSynced 时序重排，支持同步配置同步。
- 编码器补偿校验链路：ValidateCompensationConfig / 三层精度约束链（脉冲当量 ≥ encoderScale ≥ tolerance > minStep）。

### Verification
- `go test ./internal/... ./api/...`: passed
- `go vet ./internal/... ./api/...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- 冒烟测试: passed（GUI 启动正常，无"correct build tags"错误）

### Known Issues
- 暂无。

## [0.2.0] - 2026-06-30

### Added
- 运行时遥测（runtime telemetry）系统：实时采集性能监控与状态上报。
- 设备协议模拟器框架（Simulator）：支持故障注入、DAQ-P-1604 协议处理器，用于硬件不可用时的开发与测试。
- 新增二进制存储引擎（binary_sink）、存储工厂（sink_factory）与存储装配（storage_assembly），支持多格式存储扩展。
- 数据采集用例层（acquisition usecase）新增完整的数据分发与存储链路编排。
- 前端遥测 UI：DeviceDetailPanel 增加实时数据流可视化，DeviceManagementDrawer 增加遥测面板。
- 全局设置弹窗（GlobalSettingsModal）、Log SSE 客户端（logSseClient）增强。
- spacing.css 设计令牌扩充。

### Changed
- DAQ-P-1604 适配器大幅重构，提升协议解析效率与稳定性。
- CSV 存储引擎重构，支持新存储装配体系。
- 前端设备卡（DeviceCard）与管理抽屉布局优化。
- HTTP 客户端与设备 API 层增强以支持遥测数据拉取。
- Bootstrap 服务初始化流程集成新模块。
- appcontext 重构以支持更干净的依赖注入。

### Fixed
- DataStreamRelay 测试稳定性提升：增加 defer Stop 和 sleep margin。
- 模拟器中 acceptLoop 区分正常退出与异常退出。

### Internal
- 五孔探针与三孔探针算法新增 golden baseline、边界值测试。
- 设备存储层新增 CSV sink 基准测试与 binary sink 单元测试。
- acquisition 新增基准测试。
- 工作区全面测试计划文档。

### Verification
- `go test ./internal/... ./api/...`: passed
- `go vet ./internal/... ./api/...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails3 build` (含前端 + Go 生产二进制): passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

### Internal
- 规范化生产构建标签：Taskfile.yml `build-go` 增加 `-tags production -trimpath` 与 `-w -s`。
- 新增 `clean` 和 `release` 任务。
- AGENTS.md 和 README.md 区分开发与交付命令。
- 创建 VERSION、CHANGELOG.md 和发布基础设施。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。
