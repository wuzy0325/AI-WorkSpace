# Changelog

## [0.2.1] - 2026-07-02

### Added
- 新增扫描弹窗多选 + 内联改名 + 批量添加设备功能，支持首次装机场景一次勾选多台设备一键落库。
- 新增硬件通信 hardware-send/hardware-recv 分类日志，前端通信分组可见完整命令交互流程。

### Changed
- 扫描弹窗放大至 44rem x 80vh，已添加设备置灰不可重加，未添加设备默认预勾选。
- 添加后立即触发新设备并发连接，不再需要重启应用。

### Internal
- deviceStoreHelpers 抽出 6 个纯 TS 工具函数，新增 18 条 vitest 单元测试。
- 补齐 build/config.yml 和 build/info.json 版本号到与项目一致。

### Verification
- `go test ./...`: passed (no test files)
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `npm test`: 18/18 passed
- `go build -tags production`: passed
- `makensis`: passed
- 冒烟测试: passed (GUI 启动正常，无"correct build tags"错误)

### Known Issues
- 暂无。

## [0.2.0] - 2026-07-01

### Added
- 新增 FileRotation 文件滚动配置（按大小/时长/记录数自动切文件）。
- 新增 StopConditions 录制自动停止条件。
- 新增 RecordingStopping 状态和 DroppedCount 丢帧计数，前端可显示数据完整性指标。
- 新增 Taskfile.yml 构建任务定义。

### Changed
- RecordingPort.Start 接收 RecordingConfig 结构体，替代离散参数。
- RecordingSession 新增 Format/DroppedCount/FileCount/CurrentFile/LastError 字段。
- CSV 录制器重构为异步 writer 架构，支持多设备并发写入和文件滚动。
- CSV Timestamp 列改为带毫秒的单列格式，前缀单引号强制 Excel 文本模式。
- 硬件适配器 p1604_adapter 重构，提升连接稳定性。
- 前端绑定和 stores 层适配新的录制配置和状态模型。

### Removed
- 移除 v0.1.x 实验性 Binary 录制格式：无读端、孤儿格式，维护成本高。
  CSV 已能满足 1000Hz 采集需求。原 v0.1.x 录制的 Binary 文件无法在本版本读取。

### Internal
- AGENTS.md 增加 ADR-004 索引。
- 调整 appicon 图标。

### Verification
- `go test ./...`: passed (no test files)
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed (wails3 因 go.sum 缺失不可用，改用 go build 直出)
- `makensis` 构建安装包: passed
- 冒烟测试: passed (GUI 启动正常，无"correct build tags"错误)

### Known Issues
- 暂无。

## [0.1.1] - 2026-06-29

### Internal
- AGENTS.md 新增「对外交付打包」节，使用 wails3 build（内部自动启用 -tags production）。
- 创建 CHANGELOG.md 和发布基础设施。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go run github.com/wailsapp/wails/v3/cmd/wails3 build`: passed

### Known Issues
- 暂无。
