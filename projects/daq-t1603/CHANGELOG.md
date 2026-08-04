# Changelog

## [0.6.7] - 2026-08-04

### Added
- 设备配置面板"硬件参数" section 顶部新增"设备名称"输入框，用户可随时改名（采集期间禁用，与采样频率等硬件参数一致）。

### Fixed
- 修复配置面板缺少设备名修改入口的问题：`DaqT1603Config.vue` 此前头部只读显示 `profile.name`，整个面板没有设备名 input，`saveConfig` 也不写 `name` 字段。新增 `deviceName` ref + watcher + `saveConfig` 写入 `nextProfile.name`，改名不触发 `applyConfig` 只走 `saveProfile` 持久化。
- 修复改名缺少唯一性校验的问题：扫描路径 addProfile 仅按 IP:port 去重不校验名字，配置面板显式改名应阻断重名。`saveConfig` 开头加唯一性校验（排除当前 profile 自身），冲突时阻断保存并提示。

### Internal
- 同步 6 个版本号文件到 0.6.7：`VERSION` / `wails.json` / `frontend/package.json` / `frontend/package-lock.json`（含 `packages[""]`）/ `build/windows/installer/project.nsi` / `build/config.yml`。
- 与 daq-p1604 v0.7.4 配置面板改名约束对齐，两条路径行为互补。

### Verification
- `npm run typecheck`: passed.
- `npm run test`: 2 passed.
- `npm run build`: passed.
- `$env:GOWORK="off"; go vet ./...`: passed.
- `$env:GOWORK="off"; go test ./... -count=1 -timeout 120s`: passed.
- `task release`: passed.
- `makensis -DARG_WAILS_AMD64_BINARY=... build/windows/installer/project.nsi`: passed.

### Known Issues
- 暂无。

## [0.6.4] - 2026-07-31

### Fixed
- 修复 FrameReader Start 端要求首字节必须为 `'A'` 导致约 10~15% 启动失败的问题。设备固件在 `@f0` 后并行发送 ACK 与数据流,发送顺序不保证(约 85% ACK 首字节;约 10~15% 数据帧先到或带 1 个前导残杂字节;迟到的 Start ACK 还可能落在 Stop 收集窗口,实机 `raw=41 41`)。Start 端改为用偏移 0/偏移 1 帧合法性对齐真实边界;正常采集路径支持丢弃 1 个前导字节自愈;Stop 采用 150ms 静默窗口确认 `N×64+ACK`,`finalize` 容忍 1 个前导残杂 `'A'`。
- 修复适配器 `StopAcquisition` 持锁期间 close channel 可能导致 `OnReadLoopExit` 回调对同一 channel 二次 close 触发 `panic: close of closed channel` 的问题。改为持锁期间原子移除 `stopChs`/`channels` 再锁外关闭。
- 修复 Windows 故障机器 UDP discovery `Send`/`Receive` 在 kernel IOCP deadline 失效时永久阻塞的问题(ADR-009 R0-8/R0-9)。`Send` 加 `SetWriteDeadline` + watchdog 双兜底(原 `WriteTo` 永久阻塞);`Receive` 加 `SetReadDeadline` + watchdog 双兜底(原 `ReadFrom` 同根问题,调用方 `defer Close` 与阻塞调用在同一 goroutine 无法兜底)。触发后 socket 废弃,调用方不得复用。
- 修复 Windows discovery socket watchdog 的 callback 时序问题(ADR-009 finding 5)。`time.AfterFunc.Stop` 返回 false 仅表示已 fire,不保证 callback 已完成。通过 `sync.WaitGroup` 确保 callback 完全退出后才返回,避免 callback 在 `Send`/`Receive` 返回后才执行 `Closesocket` 误关已复用的 socket 数值。新增 `closeHandleLocked`(原子取走 handle + `Closesocket`,多次调用安全)。

### Internal
- 新增 `stopstartprobe` CLI 调试工具,用于排查快速启停采集时 TCP socket 残留帧问题。
- 适配器适配 `shared/device-sdk` 新的 `sendCommand` A/E 严格校验,适配新的 `invalidateConnection` / `resyncHardwareConfigMode` 接口。
- scanner 适配新的 `discoverySocket` 接口,新增 watchdog 触发 / 多目标扫描测试。
- 新增 `discovery_socket_test.go` / `discovery_socket_windows_test.go` / `t1603_adapter_lifecycle_test.go` / `t1603_scanner_test.go` 等回归测试,覆盖 adapter Connect / Disconnect / 采集启停生命周期与 watchdog 触发后连接毒化与重建。
- 新增 `soSNDTIMEO=0x1005` 常量(`golang.org/x/sys/windows` v0.43/0.47 未导出 `SO_SNDTIMEO`,Winsock2.h 定义稳定)。
- `frontend/package-lock.json` 依赖树重新 resolve(npm install 后 lockfile 同步),删除已不在 package.json 中的多余条目。
- `frontend/src/components/layout/AppShell.vue` 布局微调。
- 同步 6 个版本号文件到 0.6.4:`VERSION` / `wails.json` / `frontend/package.json` / `frontend/package-lock.json`(含 `packages[""]`)/ `build/windows/installer/project.nsi` / `build/config.yml`。

### Verification
- `$env:GOWORK="off"; go test ./... -count=1 -timeout 120s`
- `$env:GOWORK="off"; go vet ./...`
- `npm install --no-audit --no-fund`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis -DARG_WAILS_AMD64_BINARY=... project.nsi`
- `task archive-release`

### Known Issues
- 暂无。

## [0.6.3] - 2026-07-29

### Fixed
- 修复 TCP `Dial` 在 ADR-009 故障 Windows 机器上 deadline 失效导致永久卡死、UI 一直显示"连接中"无法翻转的问题。`protocol.DialTCP` 改为子 goroutine 跑 `Dial` + 主 goroutine `time.After` 软超时兜底，超时后立即返回 `os.ErrDeadlineExceeded` 让上层 fail-fast，并在 Dial 较晚返回时通过 select-default 关闭 conn 防 FD 泄漏。
- 修复 `syncHardwareConfigLocked` 阶段 `readAllConfig` 吞掉 watchdog 错误继续走完 10 条 `@fd` 查询命令 + 3 条 `@fe` 强制命令的问题。检测到 `protocol.ErrWatchdogTriggered` 时立即返回错误，暴露真正的"连接被 watchdog 强制关闭"根因，避免操作员误判为"配置命令失败"。
- 修复前端 `connect` 缺少超时兜底导致 UI 卡在 'Connecting' 状态的问题。新增 `CONNECT_TIMEOUT_MS = 10000`（覆盖后端最坏耗时 DialTCP 5s + syncHardwareConfigLocked 4s = 9s + 1s 余量），与 `startAcquisition` / `applyConfig` 一致采用 `withTimeout` 包装，超时后翻转 UI 为 'Disconnected'。

### Internal
- 新增回归测试 `TestDAQT1603SyncHardwareConfig_FailsFastOnWatchdogTriggered`：用 `t1603DeadlineIgnoringConn` 模拟 deadline 失效场景，验证 watchdog 触发后 `syncHardwareConfigLocked` 在 5s 预算内返回且错误链包含 `protocol.ErrWatchdogTriggered`。
- 同步 6 个版本号文件到 0.6.3：`VERSION` / `wails.json` / `frontend/package.json` / `frontend/package-lock.json`（含 `packages[""]`）/ `build/windows/installer/project.nsi` / `build/config.yml`。

### Verification
- `$env:GOWORK="off"; go test -race -count=1 -run "TestDAQT1603" ./shared/device-sdk/go/daq/hardware/...`
- `$env:GOWORK="off"; go test -race -count=1 ./shared/device-sdk/go/protocol/...`
- `$env:GOWORK="off"; go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui" -o build/bin/daq-t1603.exe .`
- `$env:GOWORK="off"; go vet ./...`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 暂无。

## [0.6.2] - 2026-07-29

### Fixed
- Windows UDP 设备扫描的 Winsock 路径补回 `Closesocket` watchdog 兜底,避免 `SO_RCVTIMEO` 在特定环境失效时扫描永久卡死(ADR-009 第 5 条)。
- `sockaddrString` 出现非 IPv4 sockaddr 时显式返回错误,不再静默丢弃设备响应。

### Internal
- 新增 `TestDiscoverySocketReceiveUnblocksOnClose` 回归测试,验证 `Closesocket` 能解除阻塞的 `Recvfrom`(ADR-009 第 6 条要求的"忽略 deadline、只在 Close 后返回"连接 double 的 Windows 等价物)。
- `tsconfig.json` 的 `allowJs` 加注释说明依据(Wails 自动生成的 `bindings/*.js` 无对应 `.d.ts`)。
- 同步 6 个版本号文件到 0.6.2。

### Verification
- `$env:GOWORK="off"; go test -race -count=1 ./...`
- `$env:GOWORK="off"; go vet ./...`
- `npm run typecheck`
- `npm run test`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 暂无。

## [0.6.1] - 2026-07-28

### Fixed
- Windows UDP 设备扫描改用同步 Winsock 和固定总截止时间，避免接收循环在特定系统环境中永久阻塞。
- 点击窗口关闭按钮时显示退出确认，避免误操作直接关闭应用。

### Internal
- 新增真实同步 UDP socket 超时回归测试，并保留非 Windows 平台的 `net.PacketConn` 实现。
- 同步 6 个版本号文件到 0.6.1。

### Verification
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go vet ./...`
- `npm run test`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 暂无。

## [0.6.0] - 2026-07-27

### Added
- 独立应用新增中英文界面切换，并保持语言偏好。

### Fixed
- 设备连接失败写入明确错误日志，便于现场定位地址、网络和协议问题。
- 复用共享 SDK 的连续静默窗口排空逻辑，降低快速启停后的残留帧污染风险。

### Verification
- `$env:GOWORK="off"; go test ./...`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 暂无。

## [0.5.0] - 2026-07-25

### Added
- 新增后端 UI 刷新率动态可调（SetUIRefreshRateHz）：前端 MainTopBar 的刷新率下拉项实时同步到后端 `uiPayloadRefreshInterval`，App.onMounted 启动时也同步 localStorage 偏好；后端 relayStream 用 `Ticker.Reset` 动态跟随，不再硬编码 100ms。

### Changed
- drainConnection 改为「连续 2 个静默窗口才退出」语义：原单次静默即返回会在快速启停后残留帧尾被下一次命令读取当作响应乱码；现要求连续两次 SetReadDeadline 超时才结束 drain，避免误判残留帧尾。
- 移除 MonitorView 连接按钮未使用的 Loader2 图标与 .spin 动画，仅保留 Connecting 态 loading，减少视觉噪音。

### Fixed
- 修复 UI 刷新率设置不生效：后端 `uiPayloadRefreshInterval` 硬编码 100ms 导致 10Hz→30Hz 无变化、10Hz→2Hz 仅图表变慢而数值卡仍 10Hz 更新。改为原子变量 + Ticker.Reset 动态跟随。

### Internal
- 新增 `drainConnection` 连续静默窗口回归测试 `TestDAQT1603DrainConnectionWaitsForDelayedFrameTail`，验证 150ms 延迟到达的帧尾不会被误读为命令响应。
- 新增 Taskfile.yml，与 daq-p1604 对齐：定义 `clean / build-frontend / build-go / release / archive-release / check-bindings / generate-icon` 任务，便于后续 release 流程统一。
- 同步 6 个版本号文件到 0.5.0：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- 生产 Go 构建 `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`：通过，产出 `build/bin/daq-t1603.exe`。
- `go vet ./...`（GOWORK=off）：passed。
- `go test ./...`（GOWORK=off）：passed（adapters/config、adapters/hardware、adapters/recording、usecase 均 ok）。
- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-t1603.exe project.nsi`：产出 `daq-t1603-0.5.0-amd64-installer.exe`，归档至 `releases/bin/`。
- 已知限制：exe 自身 Windows 版本资源固定为 `0.0.0.0`（wails v3 alpha `generate syso` 限制，与历史 0.3.x/0.4.x 一致）；安装包 VIProductVersion 已正确标注 0.5.0。GUI 冒烟测试建议在目标机手动验证。

### Known Issues
- 暂无。

## [0.4.0] - 2026-07-24

### Added
- 新增异步设备状态事件推送（ACQ-010/STB-003）：OnReadLoopExit → hub → service 异步推送，UI 状态更新更及时，避免阻塞采集热路径。
- 新增禁用通道空 CSV 列输出（REC-006）：禁用通道在 CSV 中输出空列，保持列顺序与表头一致。
- 新增日志面板搜索与日志文件轮转（LOG-010/015）：前端日志可关键字搜索，后端日志按大小轮转。
- 适配器扩展支持新 SDK 接口（配合 shared/device-sdk/go/daq/hardware/daq_t1603.go 的接口扩展），为后续能力扩展铺路。

### Changed
- 配置脏状态校正（CFG-017）：硬件配置与 profile 不一致时脏标记更准确。
- 前端 ChannelCard 同步移除数值变化闪烁动画（视觉噪音）。
- CSV 表头列由 20 列（DeviceID,Timestamp,Millisecond,Unit,CH01..CH16）改为 18 列（Timestamp,Unit,CH01..CH16）。
  DeviceID 列移除（文件名已含设备 ID）；Timestamp 列仅保留秒级精度（'YYYY-MM-DD HH:MM:SS），
  与 0.3.2 决策一致——1000Hz 采集时同一秒内的样本共享同一时间戳，不再区分毫秒。

### Fixed
- 修复应用退出阶段 readLoop 收尾时 EmitDeviceState 在已关闭 app 上 panic：device_service.ServiceShutdown 清空 s.app，EmitDeviceState 加 recover 保护。
- 修复 CSV 录制 Stop→Start 会话间禁用通道掩码泄漏：csv_recorder.Stop 清理 deviceProfiles，避免上次会话的禁用通道掩码污染新会话。
- 修复错误信息匹配误判（connection pool exhausted / permission_token 等非目标场景被误判为连接错误）：recordingStore / DaqT1603Config 改用 `\b` 单词边界正则。

### Internal
- 新增 csv_recorder_rec006_test.go、device_usecase_validation_test.go。
- 同步 bindings（EmitDeviceState / SetDeviceProfile 导出）到 daq-t1603 .ts bindings（wails3 运行时会重新生成为 .js 并丢弃提交的 .ts）。
- 同步 6 个版本号文件到 0.4.0：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- 生产 Go 构建 `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`：通过，产出 `build/bin/daq-t1603.exe`。
- `go vet ./...`（GOWORK=off）：passed。
- `go test ./...`（GOWORK=off）：passed（adapters/config、adapters/hardware、adapters/recording、usecase 均 ok）。
- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-t1603.exe project.nsi`：产出 `daq-t1603-0.4.0-amd64-installer.exe`，归档至 `releases/bin/`。
- SHA-256：`22e82689af05ca0ca06fb577a6cd0cb709481a98c9320db4ade86f86ea8a0803`。
- 已知限制：exe 自身 Windows 版本资源固定为 `0.0.0.0`（wails v3 alpha `generate syso` 限制，与历史 0.3.x 一致）；安装包 VIProductVersion 已正确标注 0.4.0。GUI 冒烟测试建议在目标机手动验证。

### Known Issues
- 暂无。

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
