# Changelog

## [0.7.5] - 2026-08-13

### Added
- 新增 `src/utils/channelColors.ts`：18 通道统一默认调色板 + `channelColor()` 颜色解析（自定义颜色优先，否则按通道物理索引回退默认色板，保证同一通道在任何通道组合下颜色恒定）。

### Changed
- "添加设备"入口从顶栏移到侧栏头部（+ 图标），与扫描按钮并列。
- "校零"按钮从顶栏移到监控页设备操作栏，仅设备处于已连接/采集中时可用（`shell:zeroCalibration` / `shell:zeroing` 经 AppShell provide 注入 MonitorView）。
- 18 通道默认波形配色统一：剔除易与告警/异常混淆的琥珀/橙黄色，改用高区分度冷色系；CH17 大气压力 / CH18 大气温度用中性灰。`RealtimeChart` 改走 `channelColor()`，与通道卡片共用同一色板。
- 旧版默认色迁移：`loadProfiles` 将颜色等于某代旧色板对应索引默认色的通道置空（视为未自定义），由渲染器回退到新色板；用户自定义颜色不受影响。
- 移除设备列表侧栏的"搜索设备"过滤框，列表直接展示全部已配置设备（保留"添加设备"与"扫描设备"入口）。

### Internal
- `AppShell` 将 `add-device` 事件从 `MainTopBar` 迁移到 `DeviceSidebar`；`DeviceSidebar` 移除 `filteredSorted` / `searchQuery` 搜索过滤逻辑与 `sidebar__search` 组件及其样式；`MainTopBar` 移除 `add-device` / `zero-calibration` 事件。
- 修复旧版默认色迁移类型错误：`LEGACY_CHANNEL_COLORS` 改为多代色板数组（`string[][]`）后，`loadProfiles` 改用 `.some()` 对各代色板逐一比对对应索引默认色，修复 vue-tsc 报 "types 'string' and 'string[]' have no overlap"。
- i18n 移除 `sidebar.searchDevices` / `sidebar.noSearchMatch` / `sidebar.clearSearch` / `common.clear` 词条；`zeroCalibration` / `zeroing` / `connectBeforeZero` 从 `topbar` 迁到 `monitor`（中英双语）。
- 同步 6 个版本号文件到 0.7.5：`VERSION` / `wails.json` / `frontend/package.json` / `frontend/package-lock.json`（含 `packages[""]`）/ `build/windows/installer/project.nsi` / `build/config.yml`。

### Verification
- `npm run typecheck`: passed.
- `npm run test` (vitest): passed.
- `npm run build`: passed.
- `$env:GOWORK="off"; go test ./... -count=1 -timeout 120s`: passed.
- `task release`: passed.
- `makensis '-DARG_WAILS_AMD64_BINARY=..\..\bin\daq-p1604.exe' project.nsi`（在 `build/windows/installer/` 目录下执行）: passed.

### Known Issues
- 现场故障电脑（SetReadDeadline 失效）的 discovery 与 Stop 路径 watchdog 兜底需实测验证（继承自 0.7.3）。

## [0.7.4] - 2026-08-04

### Fixed
- 修复扫描弹窗内联改名不生效：`ScanResultList` 的 `ScanSelectionItem.name`（UI v-model 字段）与 `ScannedDeviceInput.overrideName`（`planScannedAdditions` 期望字段）字段名不匹配，`AppShell` 把 `scanSelection` 原样传入 `addScannedProfiles` 时 TS 结构子类型不报错，但运行时 `input.overrideName` 永远 `undefined`，永远回退到 `makeDefaultName`。将 `ScannedDeviceInput.overrideName` 改名为 `name`，与 `ScanSelectionItem` 对齐，UI v-model 数据可直接透传，消除字段映射层。
- 修复配置面板缺少设备名修改入口：`DaqP1604Config.vue` 新增 `deviceName` ref，在 `syncFormFromProfile` 回填、`formEqualsProfile` 脏值比较（trim 后比对）、watcher 监听、`saveConfig` 写入 `nextProfile.name`；改名不触发 `applyConfig`（`hasHardwareConfigChanged` 不含 name），只走 `saveProfile` 持久化。
- 修复改名缺少唯一性校验：`saveConfig` 开头加唯一性校验，与扫描路径 `planScannedAdditions` 的 `dedupeName` 约束对齐。批量扫描用无感追加 `(2)/(3)`，配置面板显式改名阻断并提示，两条路径行为互补。

### Internal
- 新增 2 个回归测试：`ScanSelectionItem` 结构（含 `id` + `name`）直接传入 `planScannedAdditions` 时名字生效；`name` 为空字符串时回退到默认名。
- i18n 同步新增 `config.deviceName` / `config.deviceNamePlaceholder` / `config.error.nameExists`（zh/en），`satisfies Record<LocaleKey, string>` 双向校验保证两侧 key 一致。
- 同步 6 个版本号文件到 0.7.4：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- `npm run typecheck` (vue-tsc)
- `npm run test` (vitest): 22 passed
- `npm run build`
- `task release`
- `makensis -DARG_WAILS_AMD64_BINARY=... build/windows/installer/project.nsi`

## [0.7.3] - 2026-07-31

### Fixed
- 修复 discovery（设备发现）阶段在 Windows 网络栈异常时永久阻塞：跨平台 `discovery_socket.go` 的 Send/Receive 加 `SetDeadline` + 独立 watchdog timer 双兜底；Windows `discovery_socket_windows.go` 的 `winsockDiscoverySocket` 加 `handleMu` 保护 handle，`closeHandleLocked` 原子取走 handle，`startWatchdog` 返回 stop-and-join，避免 callback 在操作返回后误关已复用 socket 数值（ADR-009 finding 5）。
- 修复 Stop transaction 在 soft deadline 触发后无法可靠取消：`StopAcquisition` 改用 `context` 控制总预算，soft deadline 触发时强制 `Close(conn)` 并返回 `ErrWatchdogTriggered` sentinel，调用方毒化驱动并重建连接。
- 修复 adapter IO 错误处理不统一：适配 `shared/device-sdk/go/protocol` 层的 `wrapP1604IOError` / `ErrWatchdogTriggered`，soft deadline 触发时强制 Close conn 并返回 sentinel 让调用方毒化驱动，避免后续命令在已失效连接上重试。
- 修复 `expectedConn` 比较误杀新连接：与 daq-t1603 B140 同款 finding 2 修复，Stop transaction remediation 测试中 `expectedConn` 比较避免误杀重建后的新连接。

### Internal
- 删除本地 `watchdogClose` 函数，改用 `shared/device-sdk/go/protocol.WatchdogClose` 统一 ADR-009 watchdog 实现，避免跨项目代码复制。
- `p1604WatchdogTimeout` 从 const 改为 var：允许测试注入短超时（200ms）加速 deadline-ignore 回归测试。
- 删除 `p1604ConsecutiveTimeoutThreshold`（25 次 5s 超时）：原设计依赖 readLoop 活跃做应用层断线检测，ADR-009 后 watchdog + TCP keepalive 已是更可靠的兜底机制。
- 删除 `p1604CommandResponseTimeout`（合并入 `p1604HandshakeTimeout`）。
- 新增 `context` 导入：Stop transaction 改用 ctx 控制总预算。
- 适配 `soSNDTIMEO=0x1005`（golang.org/x/sys/windows 未导出 SO_SNDTIMEO）。
- 新增 Stop transaction remediation 测试覆盖（+870 行）：watchdog 触发后连接毒化与重建、soft timeout / ReadFrame 错误的 conn 失效、expectedConn 比较避免误杀新连接（详见 `docs/plans/2026-07-31-daq-p1604-stop-transaction-remediation.zh-CN.md`）。
- 新增跨平台 discovery watchdog 行为单测：`discovery_socket_test.go`（新）/ `discovery_socket_windows_test.go`（+63）。
- 新增多目标扫描 / watchdog 触发测试：`p1604_scanner_test.go`（+75）。
- 同步 6 个版本号文件到 0.7.3：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./... -count=1 -timeout 120s`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis -DARG_WAILS_AMD64_BINARY=... build/windows/installer/project.nsi`

### Known Issues
- 现场故障电脑（SetReadDeadline 失效）需实测验证 discovery 与 Stop 路径的 watchdog 兜底，本机无法复现该 Windows 网络栈 bug。
- 设备固件时间戳问题仍按既有 CSV 规则规避。

## [0.7.2] - 2026-07-28

### Fixed
- 修复现场 Windows 环境中开始采集卡死：`idleReadLoop` 不再调用 `conn.Read`，彻底放弃依赖 `SetReadDeadline` 保证退出的空闲检测（ADR-009，参考 `docs/acquisition-start-no-response.md` §5.2）。
- 修复 `readLoop` 退出时残留 deadline 导致快速重启采集报 `i/o timeout`：退出时 `defer SetReadDeadline(time.Time{})` 清除残留。
- 修复快速启停残留压力帧污染 ACK 读取导致连接被误判故障：`sendCommandACK` 重写为循环 ReadFrame 跳过非 ASCII 帧，覆盖 StartAcquisition / StopAcquisition / Disconnect / ApplyConfig / ZeroCalibration 全部 5 个调用点。
- 修复 `sendCommandACK` 循环跑满上限时误报成功：超过 20 帧仍无 ACK 时返回 `too many residual frames` 错误。
- 统一 `zeroCalibrationDirect` watchdog 触发时的错误信息。

### Internal
- `sendCommandACK` 引入 5s watchdog 硬兜底（ADR-009），覆盖 `SetReadDeadline` 失效场景。
- `readLoop` 退出时 `defer SetReadDeadline(time.Time{})` 清除残留 deadline。
- 删除 `p1604DrainTimeout` / `p1604IdleCheckInterval` 未使用常量。
- 删除 `StopAcquisition` / `Disconnect` / `ApplyConfig` / `zeroCalibrationDirect` 中的 `DrainConnection` 调用。
- 新增 4 个跳帧回归测试：`TestSendCommandACK_SkipsResidualFramesBeforeACK` / `TestSendCommandACK_TooManyResidualFramesReturnsError` / `TestSendCommandACK_ResidualThenNxxReturnsDeviceError` / `TestSendCommandACK_NoResidualReturnsACKDirectly`。
- 同步 6 个版本号文件到 0.7.2：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go test ./... -count=1 -timeout 180s`
- `task build-go`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 现场故障电脑（SetReadDeadline 失效）需实测验证 watchdog 路径，本机无法复现该 Windows 网络栈 bug。
- 设备固件时间戳问题仍按既有 CSV 规则规避。

## [0.7.1] - 2026-07-28

### Fixed
- Windows UDP 设备扫描改用同步 Winsock 和固定总截止时间，避免收到设备响应后接收循环仍永久阻塞。
- 校零命令兼容设备返回 16 个系数的响应格式。
- 点击窗口关闭按钮时显示退出确认，避免误操作直接关闭应用。

### Internal
- 新增真实同步 UDP socket 超时回归测试，并保留非 Windows 平台的 `net.PacketConn` 实现。
- 同步 6 个版本号文件到 0.7.1。

### Verification
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go vet ./...`
- `npm run test`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 设备固件时间戳问题仍按既有 CSV 规则规避。

## [0.7.0] - 2026-07-27

### Added
- 设备配置新增可选本地 IP 绑定，多网卡 Windows 工位可指定连接 DAQ-P-1604 使用的网卡。

### Fixed
- 连接握手增加强制超时并在超时后关闭 socket，避免 Windows 边缘情况下连接界面永久阻塞。
- 共享帧读取器会消费并跳过 `00 00` 空帧，避免后续命令响应持续错位。
- `Disconnect`、`ApplyConfig`、采集启停和校零统一使用设备操作锁，避免并发关闭 channel 或竞争协议响应。

### Verification
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go test -race ./adapters/hardware`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 设备固件时间戳 bug 仍存在，CSV 时间戳按既有规则规避。

## [0.6.0] - 2026-07-27

### Added
- 新增 DAQ-P-1604 全压力通道校零功能：顶栏右侧以校零按钮替换配置按钮，当前选中设备处于已连接或采集中时均可发送设备原生 `h` 校零命令。
- 新增校零操作锁、连接状态检查、12 秒前端超时提示及中英文界面文案，防止重复触发和未连接误操作。

### Changed
- 顶栏移除配置图标；设备详情区原有配置按钮继续保留，设备配置能力不受影响。

### Internal
- 新增 `DevicePort`、设备用例、Wails 后端和 TypeScript bridge 的校零调用链，并同步生成 Wails TypeScript bindings。
- 模拟设备适配器支持采集中校零调用；新增硬件适配器回归测试，验证采集中发送 `h` 后采集状态保持不变。
- 同步 6 个版本号文件到 0.6.0：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- `$env:GOWORK="off"; go test ./...`
- `$env:GOWORK="off"; go vet ./...`
- `$env:GOWORK="off"; go build -buildvcs=false ./...`
- `npm run test`
- `npm run typecheck`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues
- 校零命令在采集数据流中异步返回新的零位系数；当前版本确认命令已成功写入设备，但界面不解析并展示该系数响应。
- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.0] - 2026-07-25

### Internal
- 与 daq-t1603 版本号同步发布。本版本 daq-p1604 无独立功能改动：最近共享 SDK 改动（`shared/device-sdk/go/daq/hardware/daq_t1603.go` 的 drainConnection 连续静默窗口语义）仅影响 T1603 设备，P1604 使用 w1601 长度前缀协议且 ReadLoop 路径不同，不受影响。
- 同步 6 个版本号文件到 0.5.0：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- `task release`（clean + 前端 build + 生产 Go 构建）：通过。前端 `npm run build`（vue-tsc + vite）成功，`go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"` 产出 `build/bin/daq-p1604.exe`。
- `go vet ./...`（GOWORK=off）：passed。
- `go test ./...`（GOWORK=off）：passed。
- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-p1604.exe project.nsi`：产出 `daq-p1604-0.5.0-amd64-installer.exe`，归档至 `releases/bin/`。
- 已知限制：exe 自身 Windows 版本资源固定为 `0.0.0.0`（wails v3 alpha `generate syso` 限制，与历史 0.3.x/0.4.x 一致）；安装包 VIProductVersion 已正确标注 0.5.0。GUI 冒烟测试建议在目标机手动验证。

### Known Issues
- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.4.0] - 2026-07-24

### Added
- 新增空闲连接 TCP keepalive 失效检测（CONN-008）：connected & idle 状态下 idleReadLoop 探测到对端 keepalive 失败即主动判定断连，作为命令调用之外的补充快速通道。
- 新增禁用通道空 CSV 列输出（REC-006）：禁用通道在 CSV 中输出空列，保持列顺序与表头一致，便于后续按列解析。
- 新增日志面板搜索与日志文件轮转（LOG-010/015）：前端日志可关键字搜索，后端日志按大小轮转，便于现场排查。
- 新增异步设备状态事件推送（ACQ-010/STB-003）：OnReadLoopExit → hub → service 异步推送状态，UI 状态更新更及时，避免阻塞采集热路径。

### Changed
- 配置脏状态校正（CFG-017）：硬件配置与 profile 不一致时脏标记更准确。
- 前端 ChannelCard 改用 computed 派生数值、颜色始终跟随通道色，移除采集期数值变化闪烁动画（视觉噪音）；MonitorView 文案「18 通道并行」→「多通道并行」。

### Fixed
- 修复 ApplyConfig 与 idleReadLoop 竞争同一 conn 读取导致 v01101 响应被污染为乱码：操作 conn 前显式 stopIdleLoop 并 join；空闲读取循环在末尾 defer 以新 stop 通道重启，四重守卫防止双启或失联后重启；StopAcquisition 不再派生重复 idle 循环（guard on driver.idleStopCh == nil）。
- 修复拔网线重连时 u01101/v01101 阶段对端 FIN 被当作软错误吞掉、导致半死连接后续 StartAcquisition 触发 Windows WSAECONNABORTED：新增 sharedproto.IsConnResetByPeer 判定对端 FIN/RST 硬证据（io.EOF / connection reset / broken pipe / WSAECONNABORTED），与 IsConnectionFault（日志降噪用）语义分离；Connect 检测到 unitErr 时关闭 conn 并返回 error 强制重连；ApplyConfig v01101 失败且命中时复用 handleConnectionLost 清理 driver + conn + status。
- 修复连接设备时硬件单位与 profile 不一致导致前端通道卡片显示陈旧单位（psi）而顶部/配置显示新单位（Pa）：新增 syncChannelsUnit helper，规则与前端 getChannelUnit 一致（CH1-CH16 跟随全局压力单位，CH17 大气压力锁定 Pa，CH18 大气温度锁定 ℃）。

### Internal
- 新增 e2e 测试套件（e2e_*.py / cases.json / gen_e2e_report.py）与文档 e2e-testing-guide.md。
- 新增 idle-stop 回归测试（ApplyConfig & StopAcquisition）、IsConnResetByPeer 单元测试（13 条）、syncUnitFromHardware EOF/超时测试、ApplyConfig v01101 EOF/软错误测试、syncChannelsUnit 4 条单元测试。
- 同步 6 个版本号文件到 0.4.0：VERSION、apps/desktop-wails/wails.json、apps/desktop-wails/frontend/package.json、apps/desktop-wails/frontend/package-lock.json、apps/desktop-wails/build/config.yml、apps/desktop-wails/build/windows/installer/project.nsi。

### Verification
- `task release`（clean + 前端 build + 生产 Go 构建）：通过。前端 `npm run build`（vue-tsc + vite）成功，`go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"` 产出 `build/bin/daq-p1604.exe`。
- `go vet ./...`（GOWORK=off）：passed。
- `go test ./...`（GOWORK=off）：passed（adapters/hardware、adapters/recording、backend 均 ok）。
- `makensis /DARG_WAILS_AMD64_BINARY=..\..\bin\daq-p1604.exe project.nsi`：产出 `daq-p1604-0.4.0-amd64-installer.exe`，归档至 `releases/bin/`。
- SHA-256：`24669f125a2e28dd4c55e7e74639c5f3d0f331958dc9a02cb06bcd476d169bda`。
- 已知限制：exe 自身 Windows 版本资源固定为 `0.0.0.0`（wails v3 alpha `generate syso` 限制，与历史 0.3.x 一致）；安装包 VIProductVersion 已正确标注 0.4.0。GUI 冒烟测试建议在目标机手动验证。

### Known Issues
- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

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
