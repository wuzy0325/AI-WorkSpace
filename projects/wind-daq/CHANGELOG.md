# Changelog

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
- `go test ./...`: 
- `go vet ./...`: 
- `npm run typecheck`: 
- `npm run build`: 
- `task release`:
- `makensis` 构建安装包:

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
