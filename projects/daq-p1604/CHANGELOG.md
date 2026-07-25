# Changelog

## [0.5.0-win7.1] - 2026-07-25

### Changed

- 版本号同步至 `0.5.0-win7.1`：与 master 0.5.0 主版本号对齐，保留 `-win7.1` 后缀以标识 Win7 LTS 兼容版本。`apps/desktop-electron/package.json` 与 `frontend/package.json` 同步更新；NSIS 安装包文件名格式 `DAQ-P-1604-Win7-Setup-0.5.0-win7.1-x64.exe`。

### Internal

- 本次未引入新的业务功能改动，仅为版本号同步与 master 0.5.0 对齐。master 0.5.0 上 daq-p1604 也无独立功能改动（仅同步发布），共享 SDK 的 drainConnection 改动仅影响 T1603 设备，P1604 使用 w1601 长度前缀协议且 ReadLoop 路径不同，不受影响。
- master 0.5.0 涉及的 `wails.json` / `wails_windows_amd64.syso` / `frontend/package-lock.json` / `build/config.yml` / `build/windows/installer/project.nsi` 等 master 版本号文件在 lts/win7 上不存在（已由 `apps/desktop-electron/package.json` 替代），无需同步。

### Verification

- `go build ./...`（GOWORK=off，Go 1.20.14）：passed
- `go vet ./...`：passed
- `go test ./...`：passed
- `npm run typecheck`：passed
- `npm run build`：passed
- `npm run build:backend`：passed
- `npm run dist:win7`：passed（产物 NSIS x64 安装包）
- 安装包 SHA-256 见 `releases/0.5.0-win7.1.md`

### Known Issues

- 与 0.4.0-win7.1 一致：Electron 22.3.27 不支持 `color-mix()` CSS 函数（已用 rgba fallback 规避）；360 主动防御可能锁定 `app.asar`（建议添加信任区或改用 `--config.directories.output=dist2` 绕过）。

## [0.4.0-win7.1] - 2026-07-25

### Added

- **IsConnResetByPeer 单元测试**（cherry-pick acd21c0）：补齐 `shared/device-sdk/go/protocol/conn_helpers_test.go` 的 `TestIsConnResetByPeer` 13 条用例。函数本体与 `p1604_adapter.go` 的硬/软错误处理已在 0.3.0-win7.1（d586dec）引入，本次仅同步 master 上的测试覆盖。
  - 覆盖硬证据：io.EOF / 包装 EOF / connection reset by peer / broken pipe / wsasend aborted / wsarecv aborted / connection abort。
  - 软错误不匹配：i/o timeout / timeout net error / device error N05 / parse error。

### Changed

- 版本号同步至 `0.4.0-win7.1`：与 master 0.4.0 主版本号对齐，保留 `-win7.1` 后缀以标识 Win7 LTS 兼容版本。
- `apps/desktop-electron/package.json` 版本号字段同步更新；NSIS 安装包文件名格式 `DAQ-P-1604-Win7-Setup-0.4.0-win7.1-x64.exe`。

### Internal

- 本次未引入新的业务功能改动，仅为版本号同步与 master 0.4.0 对齐 + 测试用例补充。
- master 0.4.0 涉及的 wails.json / wails_windows_amd64.syso / frontend/package.json 等版本号文件在 lts/win7 上不存在（已由 `apps/desktop-electron/package.json` 替代），无需同步。

### Verification

- `go build ./...`（GOWORK=off，Go 1.20.14）：passed
- `go vet ./...`：passed
- `go test ./...`：passed（含 `TestIsConnResetByPeer` 13 用例）
- `npm run typecheck`：passed
- `npm run build`：passed
- `npm run build:backend`：passed
- `npm run dist:win7`：passed（产物 NSIS x64 安装包）
- 安装包 SHA-256 见 `releases/0.4.0-win7.1.md`

### Known Issues

- 与 0.3.0-win7.1 一致：Electron 22.3.27 不支持 `color-mix()` CSS 函数（已用 rgba fallback 规避）；360 主动防御可能锁定 `app.asar`（建议添加信任区或改用 `--config.directories.output=dist2` 绕过）。

## [0.3.0-win7.1] - 2026-07-23

### Changed

- **Win7 LTS 改造**：将桌面壳从 Wails v3 + WebView2 替换为 **Go 1.20.14 + Electron 22.3.27 + net/http**，业务层（core/ports/usecase/adapters）零改动。详见 `docs/runbooks/win7-migration-guide.md`。
- 监听端口改为 `127.0.0.1:18182`（与 daq-t1603 的 18181 区分，避免同机双开冲突）。
- `apps/desktop-wails/main.go` 改为 net/http server + `//go:embed all:frontend/dist` + 优雅关闭，移除 Wails 依赖。
- `apps/desktop-wails/backend/app.go` 改为 hub 模式，移除 `application.App` 依赖，依赖 `core.EventBus` 接口而非具体传输。
- Go 1.20 无 `log/slog` 包，统一替换为 `shared.local/device-sdk/go/pkg/slog` polyfill（7 个文件）。
- 前端 `bridge/` 改为 fetch + WebSocket（移除 `@wailsio/runtime` 依赖）：`httpClient.ts`（统一响应信封解包）、`wsClient.ts`（WebSocket 单例 + 指数退避重连）、`deviceBridge.ts` / `logBridge.ts` / `recordingBridge.ts`（HTTP RPC + WebSocket 事件订阅）。
- 前端 `package.json` 移除 `@wailsio/runtime` 依赖；`tsconfig.json` 移除 `bindings/**` 避免引用 Wails 绑定导致 vue-tsc 报错。
- `go.mod` 改为 `go 1.20` + `nhooyr.io/websocket v1.8.17` + `shared.local/device-sdk/go`，移除 Wails v3 依赖。
- 工作空间 `go.work` 注释掉 `projects/daq-p1604/apps/desktop-wails` 行（Go 1.20 + Wails v3 alpha 与工作空间 Go 1.26 不兼容）。

### Added

- **新增 `apps/desktop-wails/httpserver/` 包**：HTTP handler + WebSocket hub（`register.go` / `device_handler.go` / `recording_handler.go` / `log_handler.go` / `ws_hub.go` / `helpers.go`），实现 `core.EventBus` 接口，单 goroutine 串行处理 register/unregister/broadcast，每客户端独立 send channel（buffered 32）+ writePump goroutine。
- **新增 `apps/desktop-wails/core/eventbus.go`**：`EventBus` 接口 + 4 个事件常量（`daq:log` / `daq:recording-status` / `daq:recording-warning` / `daq:device-state`），解耦事件推送与传输层。
- **新增 `apps/desktop-wails/core/hub.go`**：Hub 状态容器，集中管理 ctx、relay 协程映射、LogEmitter、EventBus，避免 Service 间循环依赖。
- **新增 `apps/desktop-electron/` 目录**：Electron 22.3.27 桌面壳，包含 `main.cjs`（主进程：spawn Go 后端 + 创建 BrowserWindow + IPC 桥）、`preload.cjs`（contextBridge 暴露 `showOpenDialog`）、`package.json`（electron-builder NSIS 打包配置）、`scripts/build-backend.ps1`（Go 1.20.14 路径硬编码 + GOWORK=off + CGO_ENABLED=0）、`scripts/generate-ico.ps1`（从 appicon.png 生成多尺寸 ICO）、`.gitignore`。
- **新增 `frontend/src/bridge/httpClient.ts`**：统一响应信封解包（`{ok:true, data}` / `{ok:false, error}`），导出 `post<T>` / `get<T>` / `del<T>` 便捷封装。
- **新增 `frontend/src/bridge/wsClient.ts`**：WebSocket 单例 + 指数退避重连（1s → 2s → 4s → ... 上限 10s），自动重连后重新订阅事件。
- **重新生成 `appicon.ico`**：采用 `tools/ico/wave_green_512.png`（512x512 32bpp ARGB，波浪绿主题）作为图标源，通过 `scripts/generate-ico.ps1` 生成 6 尺寸（256/128/64/48/32/16）多分辨率 ICO（23074 bytes），满足 electron-builder 至少 256x256 要求。原 ico 仅 192 bytes 单尺寸不达标。`appicon.png` 同步替换为 wave_green_512.png。

### Internal

- **多参数事件 wire 格式**：`daq:device-state` 是双参数事件 `[id, state]`，WSHub.Emit 当 data 长度 > 1 时打包为数组推送，前端 onmessage 解构数组。
- **统一响应信封**：`{ok:true, data}` / `{ok:false, error}` 便于前端 fetch 统一处理，`apiOK` / `apiErr` 辅助函数集中在 `httpserver/helpers.go`。
- 临时复用 daq-t1603 的多尺寸 `appicon.ico` 覆盖 daq-p1604 原 ico（原 ico 仅 192 bytes 单尺寸，不满足 electron-builder 至少 256x256 要求）——**已在本次重新生成专属 ICO 后移除该临时措施**。
- `frontend/dist/` 目录由 `.gitignore` 第 5 行 `dist/` 规则忽略，clone 后不存在；`build-backend.ps1` 通过"先 `npm run build` 再 `go build`"保证 `//go:embed all:frontend/dist` 在编译时目录必然存在（与 daq-t1603 一致，不保留 `.gitkeep` 占位文件）。
- 同步 3 个版本号文件到 `0.3.0-win7.1`：`VERSION`、`apps/desktop-wails/frontend/package.json`、`apps/desktop-electron/package.json`。
- 更新 `AGENTS.md`（新增"Win7 LTS 分支构建"节）、`CLAUDE.md`（新增"Win7 LTS Branch"节，含架构变化图、关键设计、端口与事件清单、与 daq-t1603 Win7 版差异）。

### Verification

- `$env:GOWORK="off"; go build -buildvcs=false ./...`（Go 1.20.14）
- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`（adapters/hardware、backend、httpserver 三包测试全绿）
- `npm run typecheck`（vue-tsc）
- `npm run build`（Vite，5153 modules）
- `npm run build:backend`（生成 `backend/daq-p1604-backend.exe` 6.6MB）
- `npm run dist:win7`（生成 `dist/DAQ-P-1604-Win7-Setup-0.3.0-win7.1-x64.exe` 67.6MB）

### Known Issues

- 无重大已知问题。
- `frontend/dist/` 被 `.gitignore` 忽略，clone 后单独 `go build` 会因 embed 目录缺失失败；开发者须先执行 `npm run build` 或直接使用 `npm run build:backend`（脚本内已串行执行两步）。此行为与 daq-t1603 Win7 版一致。

## [0.3.0] - 2026-07-03

### Changed

- **CSV 录制改为每设备一个文件**：原单文件设计在多设备同时录制时，两台设备的硬件时间戳交替跳跃写入同一 CSV 文件，时间戳列在两个值之间来回跳变，数据列也混杂。现按 deviceId 路由到独立文件，与 wind-daq 设计对齐。
- 文件名格式变更（不兼容旧版本）：
  - 旧：`<prefix>_YYYYMMDD-HHMMSS.csv`
  - 新：`<prefix>-<deviceSlug>-YYYYMMDD-HHMMSS-NNN.csv`
  - `deviceSlug` 优先用设备名（sanitize 后），同名冲突时追加 deviceId 前 6 位
- 文件滚动条件（MaxSize/MaxRecordCount/MaxDuration）改为**按设备独立评估**，停止条件（跨设备汇总）保持不变。
- 录制启动时不再预创建空文件，改为第一个 payload 到达时按 deviceId 懒创建，避免多设备场景下未投递数据的设备产生空 CSV。

### Internal

- `CSVRecorder` 重构为多 writer 架构：`map[deviceId]*perDeviceWriter`，每设备独立持有文件/缓冲/统计，单 writer goroutine 串行消费 channel 消除多设备锁争用。
- `core.RecordingConfig` 新增 `DeviceNames map[string]string` 字段，由 backend 在 StartRecording 时从 profiles 一次性填充 deviceId→name 映射，recorder 用于生成人类可读的文件名 slug。
- `backend/app.go` 的 `StartRecordingWithConfig` 调整为：先取 profiles → 聚合通道精度 → 构建 deviceNames map → 注入 RecordingConfig。
- 清理 dead code：移除未消费的 `autoDone`/`autoDoneOnce`/`signalAutoDone` 信号机制（靠 `started.CompareAndSwap` + writerLoop 串行 I/O 已保证并发安全），以及 `perDeviceWriter` 的 `deviceID`/`headerWritten`/`totalRecords` 三个未读字段。
- 同步 6 个版本号文件到 0.3.0：`VERSION`、`apps/desktop-wails/wails.json`、`apps/desktop-wails/frontend/package.json`、`apps/desktop-wails/frontend/package-lock.json`、`apps/desktop-wails/build/config.yml`、`apps/desktop-wails/build/windows/installer/project.nsi`。

### Verification

- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis /DARG_WAILS_AMD64_BINARY=build/bin/daq-p1604.exe project.nsi`

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。
- 多 writer 核心路由逻辑（`getOrCreateWriter` / `sanitizeFileSegment` / `uniqueFileSlugLocked` / `shouldRotate(deviceID)`）暂无单元测试覆盖，依赖实机多设备验证。

## [0.2.4] - 2026-07-03

### Added

- 新增应用层连续超时断连检测：`readLoop` 维护 `consecutiveTimeouts` 计数器，连续 25 次（5s）ReadFrame 超时即调用 `handleConnectionLost` 主动判定断连，作为 TCP keepalive 之外的快速通道。

### Changed

- `p1604KeepAlivePeriod` 从 `10 * time.Second` 调整为 `3 * time.Second`，Windows ~33s / Linux ~12s 兜底，比原 ~110s 快 3 倍以上。
- 形成双保险检测架构：采集期 readLoop 活跃时由连续超时计数器主检测（5s），非采集期 readLoop 空闲时由 keepalive 兜底（~33s/12s）。

### Fixed

- 修复通道选择器组件文本溢出截断的问题（v0.2.3 遗漏未发布）。
- 修复 CH17/CH18 大气通道默认被勾选进实时图表的问题。

### Internal

- 同步更新 `enableTCPKeepalive` 设计注释、`Connect` keepalive 启用块注释、`p1604ConsecutiveTimeoutThreshold` 双保险说明，修正与新版 keepalive 数值（3s/33s）矛盾的过期描述（原 10s/100s/110s）。
- 同步 6 个版本号文件到 0.2.4：`VERSION`、`apps/desktop-wails/wails.json`、`apps/desktop-wails/frontend/package.json`、`apps/desktop-wails/frontend/package-lock.json`、`apps/desktop-wails/build/config.yml`、`apps/desktop-wails/build/windows/installer/project.nsi`。
- 通过 `npm install --package-lock-only` 同步 package-lock.json 与 package.json 版本号。

### Verification

- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis /DARG_WAILS_AMD64_BINARY=build/bin/daq-p1604.exe project.nsi`

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。
- 非采集期间无周期性 I/O 触发 keepalive 失败上报，断连检测仍依赖下一次命令调用。

## [0.2.3] - 2026-07-03

### Fixed

- 修复停止采集后立即配置单位时返回 "unexpected v01101 response" 错误的问题。停止采集后 TCP 缓冲区残留采集数据帧，v01101 命令的 ReadFrame 把残留当作响应读出。在 ApplyConfig 发送 v01101 前增加 frameReader.Reset + DrainConnection 排空残留数据。

### Internal

- 对齐 wind-daq 已有的 SetUnit 缓冲区排空修复方案。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go vet ./...`
- `task release`

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

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
