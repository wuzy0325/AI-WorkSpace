# Changelog

## [0.4.0-win7.1] - 2026-07-25

### Added

- **UI 刷新率动态生效**（cherry-pick cc78450）：新增 `SetUIRefreshRateHz` 后端方法 + `/api/device/set-ui-refresh-rate` HTTP 路由，前端 MainTopBar 切换刷新率档位时即时同步后端 `relayStream` 的 `uiTicker`，真正改变数值卡与图表的更新节奏。原 0.3.3-win7 后端硬编码 10Hz，前端切换 2/5/15/20/30Hz 无效果。
- **IsConnResetByPeer 单元测试**（cherry-pick acd21c0）：补齐 `shared/device-sdk/go/protocol/conn_helpers_test.go` 的 `TestIsConnResetByPeer` 13 条用例，覆盖 io.EOF / connection reset / broken pipe / WSAECONNABORTED 硬证据与 i/o timeout 软错误的边界。

### Changed

- **CSV 表头简化为 18 列**（cherry-pick 4879ba6）：原 20 列 `DeviceID,Timestamp,Millisecond,Unit,CH01..CH16` 改为 18 列 `Timestamp,Unit,CH01..CH16`。
  - 移除 `DeviceID` 列：每设备独立文件，文件名已含 deviceSlug，列内重复冗余。
  - 移除 `Millisecond` 列：时间戳回到秒级 `'YYYY-MM-DD HH:MM:SS'`，与 0.3.2 决策一致；1000Hz 同秒样本共享同一时间戳，靠文件名毫秒后缀区分文件。
  - 同步更新 `csv_recorder_test.go` / `csv_recorder_rec006_test.go` 列索引与 `docs/test-cases.html` 用例文档。
- **MonitorView 连接按钮移除 Loading spinner**（cherry-pick bdbbd1a）：仅保留 Connecting 态 loading，简化视觉噪音。
- 版本号同步至 `0.4.0-win7.1`：与 master 0.4.0 主版本号对齐，保留 `-win7.1` 后缀以标识 Win7 LTS 兼容版本。

### Internal

- HTTP 路由表更新：`device_handler.go` 头部注释新增 `POST /api/device/set-ui-refresh-rate`，`register.go` 注册路由。
- `deviceBridge.ts` 新增 `setUIRefreshRateHz(hz)` 包装，调用 `POST /api/device/set-ui-refresh-rate`。
- `App.vue` `onMounted` 启动后同步 localStorage 保存的刷新率偏好到后端（不阻塞 onPayload 订阅）。
- `MainTopBar.vue` `selectRefreshRate` 切换档位时立即同步后端，失败不阻塞 UI（displayStore 已本地持久化）。

### Verification

- `go build ./...`（GOWORK=off，Go 1.20.14）：passed
- `go vet ./...`：passed
- `go test ./...`：passed（含 `TestIsConnResetByPeer` 13 用例、`csv_recorder_test` / `csv_recorder_rec006_test` 18 列回归）
- `npm run typecheck`：passed
- `npm run build`：passed
- `npm run build:backend`：passed
- `npm run dist:win7`：passed（产物 NSIS x64 安装包）
- 安装包 SHA-256 见 `releases/0.4.0-win7.1.md`

### Known Issues

- 与 0.3.3-win7 一致：Electron 22.3.27 不支持 `color-mix()` CSS 函数（已用 rgba fallback 规避）；360 主动防御可能锁定 `app.asar`（建议添加信任区或改用 `--config.directories.output=dist2` 绕过）。
- `frontend/bindings/` 目录仍保留 master 上的 Wails v3 .ts binding 文件，但 `frontend/src/` 已无任何引用（lts/win7 用 fetch + WebSocket 替代）。这些文件作为历史遗留保留，不影响构建。

## [0.3.3] - 2026-07-03

### Fixed

- 修复停止采集后立即配置参数时命令响应乱码或失败的问题。停止采集后 TCP 缓冲区残留采集数据帧，ApplyDaqT1603Config 的 sendCommand 把残留当作命令响应读出。在 stopAcquisitionLocked 停止命令后增加 drainConnection 排空残留数据；在 ApplyDaqT1603Config 调用 applyHardwareConfig 前增加 drainConnection。

### Internal

- 修复点位于 shared/device-sdk/go/daq/hardware/daq_t1603.go，wind-daq 的 T1603 设备同样受益。

### Verification

- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `$env:GOWORK="off"; go vet ./...`
- `task release`（手动 fallback：`go build -tags production` + `makensis`，daq-t1603 无 Taskfile）

### Known Issues

- 暂无。

## [0.3.2] - 2026-07-03

### Fixed
- 修复 CSV Timestamp 列时间戳精度问题：跨项目对齐时间戳格式变更，统一截断到秒级（`'YYYY-MM-DD HH:MM:SS`），避免展示错误的时间细分。原因详见 daq-p1604 v0.2.2 release note（DAQ-P-1604 设备硬件时间戳固件 bug）。

### Verification
- `$env:GOWORK="off"; go build ./...`: passed（daq-t1603 工作空间隔离，见 ADR-006）
- `$env:GOWORK="off"; go vet ./adapters/recording/...`: passed
- `$env:GOWORK="off"; go test ./adapters/recording/...`: passed

### Known Issues
- 暂无。

## [0.3.1] - 2026-07-02

### Internal
- 修复构建配置文件版本滞后：build/config.yml、build/info.json、project.nsi 同步到 0.3.1。
- v0.3.0 的 NSIS 安装包未正确生成，本次补全。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed
- `makensis`: passed
- 冒烟测试: passed（GUI 启动正常，无"correct build tags"错误）

### Known Issues
- 暂无。

## [0.3.0] - 2026-07-02

### Added
- 新增硬件通信日志：驱动层 hardware-send/hardware-recv 的 debug 日志提升为 info，前端通信分组可见完整命令交互流程（TCP 连接/断开、@fd/@fe/@f0 等命令的发送与响应）。采集期间的二进制数据帧不打印，避免高频刷屏。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- 冒烟测试: passed（GUI 启动正常，无"correct build tags"错误）

### Known Issues
- 暂无。

## [0.2.0] - 2026-07-01

### Added
- 新增录制背压处理系统：BackpressureEvent 含队列长度/容量/累计丢帧数。
- 新增 SetBackpressureHandler / SetFatalErrorHandler 回调，录制队列饱和或 I/O 错误时非阻塞通知。
- RecordingSession 新增 DroppedCount 字段，前端可监控数据完整性。
- 新增 LogFileState 类型，前端可查询日志文件写入状态。
- RecordingService 新增背压/fatal 事件限频（每类 1Hz），避免事件刷屏。

### Changed
- CSV 录制器重写：支持多设备独立写入、非阻塞异步队列、文件滚动。
- 后端重构：删除 monolithic app.go，拆分 relayStream 到 DeviceService。
- RecordingService 注入背压和致命错误回调，Hub EmitLog 异步广播。
- 前端 App.vue/stores 适配新的录制状态和背压事件。

### Internal
- freqprobe 调试工具小幅调整。
- AGENTS.md 和 README.md 补充 Release Commands 段。
- go.mod 更新依赖。

### Verification
- `go test ./...`: passed
- `go vet ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production`: passed (wails3 因 go.sum 缺失不可用，改用 go build 直出)
- `makensis` 构建安装包: passed
- 冒烟测试: passed (GUI 启动正常，无"correct build tags"错误)

### Known Issues
- 暂无。

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
