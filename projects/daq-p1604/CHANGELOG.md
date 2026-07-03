# Changelog

## [0.2.2] - 2026-07-03

### Fixed
- 修复 CSV Timestamp 列时间戳错误：DAQ-P-1604 设备硬件时间戳存在固件 bug（fractional 字段以 ~4348Hz 速率递增，每累积约 232ms 跳跃校正），导致 1000Hz 采集下 1 毫秒内出现多帧时间戳；系统毫秒时间戳在 1000Hz 下精度也不足。统一截断到秒级，避免展示错误的时间细分。
- 修复 `CSVRecorder.Stop()` 缺少 `return nil` 导致的编译错误（预存问题，阻塞验证）。
- 修复 `csv_recorder.go` 表头注释错误：从「微秒精度」更正为「秒级精度」，与实际格式串一致。

### Internal
- Taskfile `generate-icon` 任务改用 `build/windows/info.json` 和 `build/windows/wails.exe.manifest` 模板文件，`wails3 generate syso` 内部用 `wails.json` 的 info 字段渲染模板。删除冗余的具体值版本 `build/info.json` 和 `build/windows.manifest`，版本号源从 7 个收敛到 6 个。
- 重新生成 `wails_windows_amd64.syso` 资源段。
- 新增项目 `README.md` 和 `CLAUDE.md` 文档，对齐 daq-t1603 / wind-daq 项目。

### Verification
- `go build ./...`: passed
- `go vet ./adapters/recording/...`: passed
- `go test ./adapters/recording/...`: passed (no test files)

### Known Issues
- 设备硬件时间戳固件 bug 未修复，需联系硬件工程师修复固件后才能恢复毫秒精度时间戳。

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
