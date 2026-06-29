# Changelog

## [0.1.4] - 2026-06-29

### Added
- 新增 Wails 桌面 App backend 实现：设备管理、日志、录制服务整合到统一桌面应用。

### Internal
- AGENTS.md 和 README.md 区分开发与交付命令，新增 Release Commands 段。
- AGENTS.md 增加 ADR-004 索引。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go run github.com/wailsapp/wails/v3/cmd/wails3 build`: passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

## [0.1.3] - 2026-06-23

### Fixed
- 修复适配器死锁：`StopAcquisition`/`Disconnect` 将硬件 I/O 移到锁外执行，避免与 `OnReadLoopExit`/`OnConfigSynced` 回调相互死锁。
- 修复 readLoop 异常退出后驱动未断开的问题：异常退出时清理 drivers 表并调用 `driver.Disconnect()`，防止下次 StartAcquisition 在坏连接上重试。
- 修复 Status() 状态卡在 Acquiring 的问题：移除有缺陷的 `st.Status != StatusAcquiring` 守卫，改为信任驱动层状态（true source of truth）。
- 修复 CSV 录制数据丢失：relayStream 改为每条 snapshot 即时写入（此前每秒只写一次最新值），并结合 `IsActive()` 无锁热路径避免锁竞争。
- 修复前端扫描列表重复添加问题：ScanResultList 显示"已添加"状态标记，store 层增加重复添加防御。
- 修复设置命令后概率采集数据解析乱码。

### Changed
- CSV flush 行阈值从 100 → 2000，让 1s 时间间隔主导 flush 频率，减少多设备高频场景下的磁盘同步次数。
- CSV `FormatFloat` 替代 `fmt.Sprintf`，消除每秒 16000 次格式串解析开销。
- CSV `sync.Mutex` 替代 `sync.RWMutex`（Status 调用频率低，读写锁额外开销不值得）。
- 采集 channel 缓冲区从 8192 → 65536，在 1000Hz 下提供约 65 秒缓冲，防止 CSV flush 阻塞反压到硬件 readLoop。
- 前端硬件时间戳文案："显示/隐藏" → "启用/禁用"。
- E2E 测试 fixture 默认启用 `showTimestamp: true`。
- 文档文件 `MANUAL_TEST.md`、`TEST_PLAN.md`、`test-cases.html` 移至 `docs/` 目录。

### Removed
- 移除模拟模式：删除 `SimulatedAdapter`、`SimulatedScanner` 及其测试（`simulated_adapter_test.go`、`app_test.go`、`simulated_flow_test.go`）。
- 移除 `DAQ_T1603_MODE` 环境变量分支（main.go 硬编码使用 T1603Adapter）。
- 清理临时文件、测试产物和废弃的 threehole 代码。

### Internal
- `RecordingPort` 接口新增 `IsActive() bool`，`CSVRecorder` 和 `RecordingUsecase` 分别实现无锁热路径。
- `frameprobe` 调试工具重写：支持二进制帧解析和归一化配置查询。
- `deviceStore` 新增 `isScanResultAdded` 方法，基于 IP:Port 去重。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 暂无。

## [0.1.2] - 2026-06-18

### Fixed
- 修复硬件命令协议问题：移除 `SendCommand` 系列函数中追加的 `\n`，设备直接接收原始命令字符串。
- 修复前端状态闪烁：MonitorView 和 DeviceSidebar 补充 "Starting" 状态的处理，连接/断开按钮在过渡状态显示加载动画。
- 修复频率显示错误：MonitorView 改为从 `t1603Config.samplingRate` 读取采样频率（而非顶层 `samplingRate` 字段）。
- 增强帧解析健壮性：`parseSpaceSeparatedFrame` 支持 17 token 元数据格式（TIME 或 HEAD 单独启用），新增 `Resync()` 方法用于帧偏移后重同步。

### Changed
- 采样率输入从下拉选择框改为自由数字输入框（1-1000Hz），支持任意整数值。
- 新增 `metadataMode` 支持：`T1603FrameReader` 可根据 TIME/HEAD 配置切换固定帧/变长帧模式。
- 测试用例文档更新：端口从 5000→9000，移除模拟模式引用，改用卡片式布局。

### Internal
- 新增 E2E 测试辅助：mock-bridge 支持多设备并发，AppPage Page Object Model 和选择器 data-testid 属性。
- 清理 DaqT1603Config.vue 中未使用的 `samplingRateOptions` 和 `samplingRateSelectOptions`。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 暂无。

## [0.1.1] - 2026-06-10

### Fixed
- 移除前端 TC_RANGES 死代码和未使用的 updateT1603Config 函数。
- 将"采集中不下发硬件配置"的判断逻辑从前端移到后端 usecase，前端不再包含硬件行为知识。
- 修复前端解析设备状态时对后端数值枚举的依赖，改为使用后端直接返回的 statusText 字符串。
- 修复配置保存时硬件应用错误被空 catch 吞没的问题。

### Changed
- DeviceState 新增 StatusText 字段和 SetStatus() 辅助方法，所有适配器通过该方法统一设置状态，避免 Status/StatusText 不一致。
- 配置保存成功消息根据硬件下发结果动态显示。

### Internal
- 重构模拟适配器和 T1603 适配器中全部状态赋值操作，统一使用 SetStatus()。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed (pre-existing freqprobe IPv6 warning unrelated)
- `go build -buildvcs=false ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails build`: passed

### Known Issues
- 暂无。
