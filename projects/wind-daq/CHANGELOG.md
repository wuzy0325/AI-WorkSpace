# Changelog

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
