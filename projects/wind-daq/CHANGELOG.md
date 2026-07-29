# Changelog

## [0.11.2] - 2026-07-29

### Fixed

- 修复双探针遍历测试开始后实时压力值与插值结果冻结的问题。原 C6 修复为避免 DAQ 快照与后端 `latestData` 异步竞态导致抖动，在运行态（running/moving/stabilizing/acquiring）抑制 `onSnapshot` 写入 `realtimePressures`；但 `latestData` 实际来自已保存测点的平均值（历史值，非实时值），导致开始测试后界面冻结在上一个测点或空值，直到下一个测点保存才更新。现撤销该抑制，`onSnapshot` 在所有状态下持续更新 `realtimePressures`，`onProgress.latestData` 仅在测点完成时提供权威插值结果覆盖。新增测试覆盖 moving/acquiring 子状态下 `onSnapshot` 持续更新行为。
- 修复总压校准实时画面概览页"系数卡片"内的马赫数/速度不实时刷新的问题。原实现读 `latestCoefficients.machNumber/velocity`（上一点采集完成时的快照），两点之间不刷新；现改为读 `physics?.machNumber/velocity`（来自后端 `livePhysics` 的 5Hz 推送），与五孔校准和侧边栏"关键数据"卡片的数据源对齐。侧边栏 Ma/V 原本即实时，未改动。数据 Tab 表格中的 Ma/V 列按行渲染每点系数快照，是历史数据，非实时，未改动。
- 修复双探针遍历模式三个问题：1) 运动控制器紧急停止接触后状态未刷新仍显示急停（新增 `refreshStatus()` + `requestFastPolling()` 快速轮询）；2) 不允许配置同一控制器的不同轴（admission 改为 `(controllerID, axis)` 元组粒度，新增 `TestManagerRegistry_Start_SameControllerDifferentAxesAllowed` 等测试覆盖）；3) 误判不同控制器为冲突（前端 `DualTraversalSettings.vue` 改用 ID 比较而非引用比较）。
- 修复双探针遍历实时压力卡片未读取用户在硬件配置步骤逐通道设置的 `precision` 字段的问题。原实现硬编码 `toFixed(3)`，现从 `ch.precision` 提取精度并缺省回退 3，与单探针模式 `useTraversalRealtimeData.ts` 行为对齐。
- 修复遍历视图插值状态栏布局抖动问题（`interpStatus` 切换时高度跳变）。
- 加固 DAQ-T-1603 TCP Dial 路径：原代码缺少 watchdog 兜底，在特定 Windows 环境下 `SetReadDeadline` 失效导致 Dial 永不返回。新增 goroutine + `time.After` 软超时，超时后 `conn.Close()` 解除阻塞。同时修复 T1603 已持 `d.mu` 时再次 `Lock()` 导致自死锁、`drainConnection` watchdog 触发后未清理连接状态等问题。
- 加固 DAQ-P-1064Pre `sendCommand` 在 Write/Read 失败且 watchdog 触发时未调用 `invalidateConnection()` 清空 `d.conn` 的问题，确保上层收到失效通知触发重连。
- 加固 UDP 设备发现：新增 `discovery_socket_windows.go` 分平台实现，Windows 下使用原生 socket + 软超时兜底，防止 `SetReadDeadline` 失效导致扫描永久阻塞；并发扫描保护避免多 goroutine 同时触发发现冲突。
- 加固网络超时取消：所有 sendCommandACK 调用必须循环 ReadFrame 跳过非 ASCII 帧（压力帧），直到读到 ASCII 'A'/'Nxx' 或达到 20 帧上限；StopAcquisition 和 Disconnect 调用 sendCommandACK 前必须 `frameReader.Reset()` 清空残留数据。
- 加固诊断工具（p1604-unit-diag、p1604-ts-diag、freqprobe、frameprobe）：新增 5 分钟进程级硬 watchdog，超时后 `conn.Close()` + `os.Exit(2)`，防止永久卡死。
- 加固 `simulator.go`：`Start(ctx)` 监听 `ctx.Done()` 实现自动 Close；所有 goroutine 纳入 `sync.WaitGroup`，`Close()` 中 `wg.Wait()`；`cmdLoopIdle` 内层循环仅设置一次 idle deadline。

### Changed

- Wails 网络协议层一致性整改：`IsWatchdogTriggered` 改用 sentinel error + `errors.Is` 替代字符串匹配；`WatchdogClose` 入口增加 nil/timeout 防御性检查；`DialTCP` timeout 分支启动 goroutine 关闭 conn 防 FD 泄漏；`dsa3217.readLoop` defer 块检查 conn 失效时跳过 onError 避免双重调用；`daq_t1603.Disconnect` 在 stopAcquisitionLocked 返回 error 时保留 Error 状态不掩盖 invalidate。
- 静态分析集成：`scripts/validate-structure.ps1` pre-submit 流程新增 `staticcheck -checks U1000 ./...`，防止 dead code 漏网；新增 `scripts/staticcheck-u1000-waivers.txt` 豁免清单（仅保留 build tag 分平台存根等误报）；清理 33 处预存 dead code。

### Verification

- `go build ./services/api-go/...` / `go vet ./services/api-go/...`: passed
- `go test -race ./services/api-go/internal/...`: passed
- `shared/device-sdk/go/...`: `go build` / `go vet` / `go test -race -count=1`: passed
- `npm run typecheck`: passed
- `npm run test`: passed (328 用例)
- `npm run build`: passed
- `task release`: passed
- `makensis build/windows/installer/project.nsi`: passed
- `task archive-release`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。

## [0.11.1] - 2026-07-28

### Fixed

- 修复 DAQ-P-1064Pre 启动采集命令 `sendStartAcquisitionLocked` 缺少 watchdog 兜底的问题。原代码仅依赖 `SetWriteDeadline`，在 SetWriteDeadline 失效的 Windows 电脑上 Write 可能永久卡死且无独立 owner 能 Close conn 解除阻塞，违反 ADR-009。现已在 Write 之前启动 `sharedproto.WatchdogClose(conn, DAQ_P_1064PRE_TIMEOUT)`，超时后强制 Close conn 兜底，Write 失败时通过 `WrapWatchdogError` 附加上下文。
- 修复 DAQ-P-1064Pre 响应帧读取 `readResponseFrame` 可能因 `conn.Read` 部分读取导致协议错位的问题。Go 的 `conn.Read` 只保证返回 1-N 字节，单次 Read 可能只读到 3 字节，使 `header[3]<<8 | header[4]` 基于未初始化字节计算 `dataLen`，后续帧对齐错误。现已改用 `io.ReadFull` 保证 6 字节 header 与 dataLen 字节 body 完整读取。

### Verification

- `go build ./services/api-go/...` / `go vet ./services/api-go/...`: passed
- `go test -race ./services/api-go/internal/adapters/hardware/...`: passed (hardware + sim)
- `shared/device-sdk/go/protocol`: `go build` / `go vet` / `go test -race -count=1 ./...`: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。

## [0.11.0] - 2026-07-27

### Added

- DAQ-P-1604 设备配置新增可选本地 IP 绑定，多网卡工位可明确选择通信网卡。
- 七孔探针工作流支持动态 PRB 角度数量与多设备采集配置。

### Fixed

- 修复遍历 `PointResult.StartedAt` / `CompletedAt` 并非真实采样起止时间戳的问题。`traversal_acquisition.go` 原先用 `now - DwellTimeMs - SamplesPerPoint×10ms` 合成（文档 0.8.0 已声明记录"真实起止时间戳"，实现未落地），导致单点间隔恒为稳定等待 + 100ms，与设备采样率无关。现改为在 `collectAveragedSamples` 前后用 `time.Now()` 实测（选项 A：采样窗口语义，不含稳定等待 `DwellTimeMs`）；全 1603/采样 10/20Hz 配置下间隔正确反映为 ~0.5s。同时修正 skip 分支漏设 `StartedAt`（原默认 0 → 空串）的隐患。
- 加固 DAQ-P-1604 连接握手：空帧可恢复、握手有总超时、对端关闭时清理死连接。

### Verification

- `go test ./internal/... ./api/...`
- `npm run build`
- `task release`
- `makensis build/windows/installer/project.nsi`

### Known Issues

- 安装包未进行 Authenticode 数字签名，Windows 可能显示未知发布者提示。

## [0.10.0] - 2026-07-24

### Added

- LogViewer 新增硬件收发命令日志（hardware-send / hardware-recv）可见化能力，新增 `showHardware` 开关。

### Changed

- 模拟控制器直线轴默认速度调整为 4；WTNMC4A 轴创建时默认微步设为 4。
- 抽取 `LogEntryRow.vue` 重构日志视图，日志单行显示、按钮风格统一、标题栏新增中英文切换。

### Fixed

- 修复 NSIS 安装包中文文案乱码（mojibake → UTF-8）。
- 修复探针参考 β 角计算错误。
- 修复 1000Hz 采集下高频硬件日志刷屏问题（新增 `CategorySkipHandler`）。

### Compatibility

- 配置文件格式：兼容；新增字段均为可选字段，旧配置继续使用默认值。
- 数据文件格式：兼容；本次变更不涉及数据文件格式变化。
- 设备协议行为：无不兼容变化。

### Verification

- `go build -buildvcs=false ./...` / `go vet ./...` / `go test ./internal/... ./api/...`: passed
- `npm run test`: passed（19 文件，250 测试）
- `task check-bindings`: passed（57/57）
- `task release` + `makensis` + `task archive-release`: passed
- 生产 EXE 冒烟测试: passed

### Known Issues

- 安装包未进行 Authenticode 数字签名；无头环境无法完整渲染 GUI（WebView2 focus/eval 报错）。

## [0.9.0] - 2026-07-22

### Added

- 新增七孔探针校准完整工作流，覆盖分区采样、实时图表、系数计算、不确定度分析及校准数据持久化。
- 遍历配置新增七孔探针与五孔 PRB 能力，支持对应插值器加载、探针参数配置和实时结果展示。
- 自定义布点支持逐点覆盖运动位置与采集参数，可按测点配置差异化测试工况。

### Changed

- 校准与遍历的运动安全设置统一使用产品默认值，减少现场配置负担。
- 探针校准速度显示与保存精度统一为 3 位小数。

### Fixed

- 修复长距离或低速遍历运动超过 120 秒后，机构仍在正常运行却被误判为超时的问题。
- 修复用户临时停止设备采集后，遍历和校准无法在恢复采集后继续完成当前测点的问题。
- 修复校准等待新帧时把停采时间计入超时预算，导致临近截止时间恢复仍被误判超时的问题。
- 修复桌面端与独立 API 装配路径遗漏校准采集状态控制器的问题。
- 恢复 Wails 前后端官方配对版本，避免 runtime 协议不一致引发调用失败。
- 完成 Wind-DAQ 全量代码审查整改，覆盖校准生命周期、并发访问、数据校验与错误传播等问题。

### Compatibility

- 配置文件格式：兼容；新增字段均为可选字段，旧配置继续使用默认值。
- 数据文件格式：兼容；新增七孔校准数据不改变既有五孔、三孔、总压和总温文件格式。
- 设备协议行为：无不兼容变化。

### Verification

- `go test ./internal/... ./api/... ./pkg/...`
- `go test -race ./internal/core/calibration ./internal/usecase`
- `go vet ./internal/... ./api/... ./pkg/...`
- `npm run typecheck`
- `npm run test`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis /INPUTCHARSET UTF8` 构建安装包

### Known Issues

- 安装包未进行 Authenticode 数字签名。

## [0.8.0] - 2026-07-18

### Added

- 遍历采集支持多设备并行采集，采集结果按设备/通道归并写入。
- 遍历 CSV 计算结果新增 `StartedAt` / `CompletedAt` 两列：记录单点采集真实起止时间戳（秒级，与 `Timestamp` 列格式一致），用户可直接用 `CompletedAt - StartedAt` 算出单点总耗时，不再依赖"点数×10ms"回填公式。0 值写空字符串，兼容旧数据/异常路径。
- 自定义布点步骤内新增运动轴绑定，布点与运动轴配置合并到同一步骤。

### Changed

- 自定义布点交互改造：布点 UX 重排，运动轴绑定迁入布点步骤。
- 实时刷新率统一到全局 `storageStore.settings.refreshRateHz`（默认 5Hz）：遍历实时插值节流间隔改为派生自全局刷新率，压力卡片与插值卡片同频，消除"刷新节奏不一致"的视觉错位。移除独立的 `useUiRefreshThrottle` composable。

### Fixed

- 无有效样本失败时记录设备应答与通道索引，便于现场定位。

### Internal

- 校准 JSON 序列化测试从 core 迁移到 adapters/storage 层，符合分层边界。
- 七孔校准 spec/plan/tasks 文档与 skill 脚本补充。
- `http-client` 测试冻结 `Date.now`，修复 `deviceApi.pollLatest` 中 `setTimeout` 首次参数从 50 变 49 的 flaky。

### Verification

- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `npm run test`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包
- 冒烟测试：启动新构建产物确认 GUI 正常

### Known Issues

- 暂无。

### Compatibility

- 数据文件格式：兼容。CSV 仅新增可选列 `StartedAt` / `CompletedAt`，旧文件与旧解析逻辑仍可用。
- 配置文件格式：兼容。
- 设备协议行为：无变化。

## [0.7.0] - 2026-07-15

### Added

- 运动安全机制：遍历与校准运行过程中按轴实时监控目标-实际偏差、临界偏离、限位触发、卡死无进展等异常，触发即停止运动并快照现场。
- 共享组件 `MotionSafetyAlertCard`（故障现场卡片，红色急停 / 橙色停止双色）与 `MotionSafetyPanel`（运动安全配置面板）。
- 遍历设置面板新增运动安全配置区，支持全局与按轴覆盖（到达容差、临界偏离限、无进展超时、进展阈值）。
- 五种校准类型（五孔 / 三孔 / 总压 / 总温 / 总压探针）设置面板与主界面接入运动安全配置与告警展示。
- 运动安全 spec 文档：`spec-traversal-motion-safety.md` 与 `spec-calibration-motion-safety.md` 共同构成需求契约。

### Changed

- b140 / wtnmc4a 控制器停止 / 急停逻辑收敛：`stopAllAxesLocked` 共享、`resolveStartMove` / `resolveStopAxis` 测试 seam 统一。
- `coalesceFloat64Ptr` / `coalesceIntPtr` 统一 `MotionSafetyConfig.Resolve` / `Merge` 的指针字段合并语义。
- 前端 `getMotionSafetyVerdictLabel` 共享函数消除两处独立 switch 实现。

### Fixed

- 校准运动等待竞态：运动安全机制接入后等待逻辑收敛到统一时序。
- WTNMC4A 急停标志位时序：先置位 `EmergencyStopped` 再调用停止，避免停止执行期间其他 goroutine 发起新运动命令的时间窗。

### Internal

- 后端新增 `traversal_motion_watchdog` 看门狗与 `MotionSafetyConfig` / `MotionSafetyFailure` / `MotionSafetyVerdict` 类型体系。
- 新增 `traversal_motion_safety_test` / `calibration_motion_safety_test` 覆盖 verdict 判定、看门狗触发、急停锁存、按轴停止等关键路径（+2380 行测试）。
- Wails binding 同步：重新生成 traversal / calibration / device 的 JS binding，新增 `internal/core/traversal/` 目录。
- `.gitignore` 追加 `.codebuddy/` / `.trae/` / `.opencode/` 三条 AI 工具私有目录规则。

### Verification

- `go build ./...`
- `go test ./internal/core/... ./internal/usecase/... ./internal/ports/...`
- `go test ./...` (device-sdk/go/motion)
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。
- 运动安全机制目前仅覆盖遍历与校准运行场景，手动 Jog / MoveTo 不接入监控。

## [0.6.0] - 2026-07-14

### Added

- 设备类型扩展：`DeviceTypeDAQP1603`、`DeviceTypeDAQT1603` 等新设备类型支持。
- 默认配置迁移：新增默认配置版本迁移逻辑，支持旧配置文件自动升级。
- 默认配置测试：`default_profiles_test.go` 新增 94 行测试覆盖。

### Changed

- DAQ-P-1603 配置面板（DaqP1603Config）UI 改进。
- 设备管理抽屉（DeviceManagementDrawer）UI 优化。
- `shared/device-sdk` 核心类型扩展：新增设备类型常量。
- i18n 补充：覆盖新增设备类型文本。
- 安装程序语言选择后不再卡顿：WebView2 安装移至正式安装阶段，并保留离线包优先、在线下载回退能力。

### Fixed

- 修复 DAQ-P-1603 已关闭“校零应用”的通道仍可发起校零的问题；全部校零会跳过这些通道，并保持用户的使能设置。
- 修复设备所有压力通道均关闭“校零应用”后，前端设备级校零按钮仍可点击的问题。

### Internal

- `shared/device-sdk/go/daq/core/types.go` 新增设备类型枚举值。
- `internal/core/device/types.go` 同步扩展。
- `internal/adapters/config/migration.go` 新增迁移入口。
- `internal/adapters/config/default_profiles.go` 新增默认配置项。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.4] - 2026-07-13

### Changed

- 遍历采集采样循环日志增强：区分"设备无数据"与"通道不匹配"两类失败，便于排查采集异常。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.3] - 2026-07-13

### Changed

- 遍历活动索引（TraversalActiveIndex）重构：接口简化，逻辑清理。
- 遍历采集（traversal_acquisition）微调。

### Internal

- 新增 `traversal_acquisition_test.go` 测试覆盖。
- `traversal_active_index_test.go` 更新适配重构。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.2] - 2026-07-13

### Added

- 校准授权对话框（`CalibrationLicenseDialog`）：首次进入校准功能时展示授权信息，支持确认/关闭交互。
- 前端 i18n 补充：`i18nStore` 新增 36 条翻译项，覆盖新增授权对话框及面板文本。

### Changed

- 遍历步骤面板（TraversalLayoutStep）UI 改进（对齐新配置步骤顺序 Hardware→Probe→Layout→Review）。
- MotionView 控制器配置 UI 优化。
- MainDashboardView 布局调整。

### Fixed

- 安装程序语言选择对话框中文乱码：`MUI_LANGDLL_INFO` 中文字符修复。
- DAQ-P-1604 校零范围限制：大气压/温度辅助通道禁止校零操作。
- 遍历前置检查纳入运动控制器连接态检测：避免"已装配但全部离线"时仍显示绿色就绪状态。
- 五孔探针 PRB 插值算法重构：精简代码约 160 行，提高可维护性。

### Behavior Changes

- 五孔探针 PRB 插值器：超范围输入（压力系数落在 ±30° 校准网格凸包外）不再钳位到角点 (±30, ±30)；
  改为返回 `IsValid=false` 并附 Warning `"压力系数超出PRB校准网格，旧算法不支持外推"`。
  下游消费者需显式处理 `IsValid=false` 路径。

### Changed

- 遍历配置步骤顺序调整：Hardware → Probe → Layout → Review。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.1] - 2026-07-13

### Fixed

- DAQ-P-1604 校零范围限制：大气压/温度辅助通道禁止校零操作，防止误操作导致设备状态异常。
- 遍历前置检查改进：纳入运动控制器连接态检测，避免"已装配但全部离线"时仍显示绿色就绪状态。

### Changed

- 遍历配置步骤顺序调整：由原顺序改为 Hardware → Probe → Layout → Review，优化用户体验。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.5.0] - 2026-07-13

### Added

- 遍历测点 Point JSON 序列化（`point_json.go`）：NaN↔null 往返契约，支持 line/rectangle/sector 模式未配置轴标记，运动恢复语义完整保留。
- 遍历输出路径集中管理（`output_path.go`）：`ResolveOutputPath` 统一路径派生，消除 CSV/checkpoint/result log 路径碎片。
- 跨平台原子文件操作：`atomic_replace.go` 拆分为 `atomic_replace_common.go` / `_unix.go` / `_windows.go`，支持 Windows 平台安全原子替换。
- 遍历 v2 端口集成测试（`traversal_v2_integration_test.go`，593 行）：覆盖 Resume 截断、ValidateTail 抗损坏、列配置 Open 路径、旧格式兼容性、NaN 往返一致性。
- 遍历 CSV writer 崩溃恢复可靠存储：表头 fsync 落盘、`openCreateUnique` 自动编号防覆盖、双重初始化防御、`applyConfigLocked` 共享列配置逻辑。
- 前端 i18n 国际化补充：FiveHoleMain 模板文案、探针校准入口、设备详情面板。

### Changed

- 遍历采集 `commitPointV2` 三阶段提交增加 NaN 清洗（设备层异常防御），Point 字段 NaN 保持运动恢复语义，Calculated 字段 NaN 清洗为 0。
- 遍历断点重构：`FileCheckpointPort` 增加 `Close` 和 `checkOpen`，`FileCheckpointPortFactory.Create` 接受 `ctx` 参数。
- 遍历活动索引安全加固：`validatePath` 增加 `.` 禁止和 store-based read，防止路径遍历攻击。
- 遍历结果日志：`TraversalResultLog.Open` 支持 `openCreateUnique` 自动编号，与 CSV 行为一致。
- 编译期接口断言哨兵：`TraversalCsvWriter`（2 个：TraversalCSVPort + TraversalPointSink）、`FileCheckpointPort`、`FileCheckpointPortFactory`、`FileCheckpointStore`、`TraversalActiveIndex`、`TraversalResultLog` 共 7 个断言，覆盖 6 个 adapter 文件。
- 前端设备面板优化：DeviceCard、ChartSelector、DeviceDetailPanel UI 调整。

### Fixed

- 修复遍历 Resume 时旧格式 checkpoint（SavePath=目录）被当文件传给 Open 的 bug，通过 `ResolveOutputPath` 重算修复。
- 修复遍历 CSV 表头在 v2 Open 路径缺少通道列的问题（`applyConfigLocked` 共享逻辑）。
- 修复遍历 Resume 时 CSV 文件因已存在直接报错拒绝启动，改为自动编号另存。
- 修复遍历断点文件在 v2 装配下双重初始化导致句柄泄漏 + 文件残留。
- 修复 Point JSON 序列化 NaN 导致 `json.Marshal` 返回 "unsupported value: NaN" 错误。

### Internal

- 遍历核心类型 `types.go` 增加 SaveOptions、CustomFields 等结构字段。
- `ports/traversal.go` 接口扩展（TraversalCSVPort、TraversalCheckpointPort.Close 等）。
- 遍历 usecase 大幅重构：checkpoint 加载/保存、acquisition 状态管理、session 生命周期。
- `traversal_checkpoint_test.go` 适配新接口签名。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.4.1] - 2026-07-10

### Changed

- 遍历测试 UI 样式优化：PointsPreview 布点画布增强（渐变背景、轨迹线、立体点渲染），WorkspaceArea 顶栏极简化（去掉装饰条、分隔线分组），TopBar 状态指示改用圆点+文字替代徽章、进度条颜色对齐主题色。
- 整体视觉更轻盈现代，降低视觉重量。

### Internal

- 3 个 Vue 组件纯前端样式调整，无逻辑或接口变更。

### Verification

- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.4.0] - 2026-07-10

### Added

- 全面国际化 i18n 重构：新增 `i18nStore` 全局语言管理，覆盖设备、校准、遍历、存储、布局、设置等所有 UI 模块的中英文切换。
- 遍历测试 UI 大改：左侧栏与布点画布 UI 全面优化，前端补算预估剩余时间。
- 校准采样进度实时反馈：自动校准流程增加进度上报，用户可实时观察采样状态。
- 三孔系数重命名与马赫数指针化：`Kt→K0`、`Sb→Kv`，马赫数改用指针避免零值歧义。
- 总温 CSV 修复：表头与单位修正，适配中文表头。
- DAQ-T1603 Win7 兼容版温度采集程序（`projects/daq-t1603-win7-python/`）：纯 Python 实现，支持 Win7 无 .NET 环境。

### Changed

- 五孔/三孔/总压/总温校准 UI 全面重构：组件拆分、配置面板重排、表单校验增强。
- 日志查看器（LogViewer）布局重构：更清晰的过滤器与日志流展示。
- DeviceManagementDrawer 重构：设备管理抽屉布局优化。
- `shared/device-sdk` DAQ-P-1603 驱动重构：FFI 超时参数传指针修复 readLoop 立即退出，提升长时间采集稳定性。
- 存储设置（StorageSettings）与全局设置（GlobalSettings）重构：对齐新 i18n 体系。

### Fixed

- 修复遍历测试中绝压假表压与混合单位输入的问题：`traversal_view.go` 单位转换逻辑修正 + 单元测试覆盖。
- 修复 DAQ-P-1603 FFI timeout 参数传指针导致 readLoop 立即超时退出。

### Internal

- `i18nStore` 新增 1078 行中英文字典，覆盖全部 UI 模块。
- 校准 CSV writer 新增中文表头支持（BOM 前缀）。
- 前端 `useCalibrationWorkflow` composable 重构，优化校准状态管理。
- 后端 `calibration_total_temperature_csv_test.go`、`total_pressure_test.go` 等新增测试。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `npm run typecheck`
- `npm run build`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.5] - 2026-07-06

### Added

- 校准采集数据新鲜度检查：各算法在多次采样间通过设备时间戳判断是否新帧，避免重复读取缓存旧数据导致标准差为0。

### Changed

- 三孔/五孔/总压/总温校准算法重构：校验方向统一为"来流为正"，消除各算法间符号约定不一致的问题。
- CSV 模式配置重构：区分用户模式（user）和 CSV 模式（csv），校准选项仅在 CSV 模式下可配置。
- 校准暂停/恢复逻辑优化：暂停时自动保存中间状态，恢复时从断点继续。

### Internal

- 校准核心类型定义重构（types.go），预定义通道转换为运行时初始化。
- CSV 格式定义（csv_schema.go）重构，统一各算法表头生成。
- 前端校准工作流 composable（useCalibrationWorkflow）重构，优化状态管理。
- 新增 TimestampReader 类型与 AcquisitionHub.GetLatestTimestamp，校准引擎注入到 Config.TimestampReader。
- total_pressure.go fallback sleep 恢复 BatchPollIntervalMs 配置感知。

### Verification

- `go test ./...`
- `go build -buildvcs=false ./...`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

## [0.3.4] - 2026-07-06

### Fixed

- 修复 motion 独立窗口可被拉小导致 UI 错乱的问题：禁用缩放，固定最小尺寸为 1280×640，对齐 motion-controller 做法。
- 修复 WTNMC4A 运动控制器负方向点动/定位时位移台不移动的问题：`PLSLogLever` 未与 `Direction` 同步导致 CP 脉冲方向电平不翻转，电机驱动器收不到反向脉冲。

### Verification

- `go test ./internal/... ./api/...`
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包
- 冒烟测试

### Known Issues

- DAQ-P-1604 设备固件时间戳 bug 仍存在，CSV 时间戳已统一截断到秒级规避。

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
