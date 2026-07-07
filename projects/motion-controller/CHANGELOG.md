# Changelog

## [0.3.1] - 2026-07-06

### Changed

- 轴配置卡片布局改进：直线轴显示"导程 mm"、旋转轴显示"传动比"，取代固定双字段，减少混淆。
- 轴配置标签文案优化：步距角加单位"°"、"反转"→"方向反转"、"位置源"→"位置来源"、"最大速度"自适应显示为"最大转速"（旋转轴）。
- 轴配置脉冲当量改为 computed 缓存，避免模板每次渲染重复计算。
- 点动静步默认值从 1 改为 0.1，降低调试时意外碰撞风险。

### Internal

- 将 motion 配置工具函数（createDefaultAxis、computePulsesPerUnit、validateEncoderCompensation 等）提取到 workspace 级 `shared/frontend/motion-utils`，motion-controller 项目通过 re-export 使用，保留项目级 maxSpeed 默认值（低速 10）。

### Verification

- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- 暂无。

## [0.3.0] - 2026-07-02

### Added
- B140 编码器补偿全链路支持：补偿参数编辑器 UI、两阶段状态机（waitingStop→settling→checking↔compensating）、三层精度约束校验链（脉冲当量 ≥ encoderScale ≥ tolerance > minStep）。
- 补偿状态机新增 compensating 分支重读 TP 失败告警，不再静默吞错。
- 光栅尺分辨率可编辑输入入口，支持预设/自定义参数切换。

### Changed
- encoderScale 归一化显示，配置不合理时告警升级为 error 级阻断保存。
- Status() 编码器读取失败时收集到 LastError，使故障对操作员可见。

### Internal
- shared/device-sdk: ValidateCompensationConfig / ResolveEncoderCompensation 物理合理性校验函数。
- shared/device-sdk: B140 补偿状态机实现（b140_motion.go）。
- shared/motion-control: UpsertProfile 边界兜底，阻断物理不可能的补偿配置。
- 测试覆盖：conversions_test (6)、motion_manager_compensation_test (6)、b140_compensation_test（状态机各场景 + 编码器读失败暴露）。

### Verification
- `go test ./...` (with GOWORK=off): passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `task release`: passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

## [0.2.6] - 2026-06-29

### Fixed
- 修复窗口标题栏左上角图标不显示的问题：main.go 缺少 `application.Options.Icon` 设置，导致 Windows 窗口标题栏图标缺失（任务栏图标因 `rsrc.syso` 嵌入 EXE 资源而正常，但窗口图标需要显式提供 PNG 数据）。

### Internal
- 在 main.go 中添加 `//go:embed build/appicon.png` 和 `var appIcon []byte`，并在 `application.Options` 中设置 `Icon: appIcon`，与 wind-daq、daq-t1603、daq-p1604 等其他项目保持一致。

### Verification
- `go build -tags production`: passed
- 冒烟启动测试: passed（窗口左上角图标已正常显示）
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

## [0.2.5] - 2026-06-29

### Fixed
- 修复窗口缩放时运动控制主面板下方出现大片空白的问题：MotionControlPanel 根容器与主面板 section 未建立完整高度继承链，轴卡片网格未占满可用高度。

### Internal
- MotionControlPanel 根容器补充 `h-full min-h-0`，主面板 section 移除 `h-fit` 改为 `min-h-0`。
- `.axis-content` 改为 flex 列布局，`.axis-grid` 增加 `flex: 1; min-height: 0`，`.axis-card` 改为 `height: 100%`，使轴卡片随窗口高度合理填充。

### Verification
- `npm run typecheck`: passed
- `npm run build`: passed
- `validate-frontend-structure.ps1`: passed
- `task release`: passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

## [0.2.4] - 2026-06-29

### Internal
- 规范化生产构建标签：Taskfile.yml `build-go` 增加 `-tags production -trimpath` 与 `-w -s`。

### Verification
- `go test ./...`: passed（无测试文件）
- `npm run typecheck`: passed
- `npm run build`: passed - vite build
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

## [0.2.3] - 2026-06-29

### Changed
- 移除轴启用/禁用开关，所有轴始终启用，简化配置流程。

### Fixed
- 设置 Windows GUI subsystem，正式构建程序不再显示控制台窗口。

### Internal
- 旧配置文件读入时 enabled 字段自动归一化为 true，无需用户手动迁移。

### Verification
- `go test ./...`: passed（无测试文件）
- `npm run typecheck`: passed
- `npm run build`: passed - vite build, 1788 modules
- `go build -buildvcs=false`: passed
- `makensis` 构建安装包: passed - 7.45 MB installer

### Known Issues
- 暂无。

## [0.2.2] - 2026-06-26

### Fixed
- 修复 0.2.1 安装后 Windows 报“应用程序的并行配置不正确”导致无法启动的问题。

### Internal
- Windows 资源生成改为仅嵌入应用图标，避免将未渲染的 Wails manifest 模板嵌入 `.exe`。

### Verification
- `rsrc -ico + go build`: passed
- `go test ./...`: passed
- `npm run typecheck`: passed
- `npm run build`: passed
- `wails3 build`: passed
- `makensis /DARG_WAILS_AMD64_BINARY=..\\..\\bin\\motion-controller.exe project.nsi`: passed
- 本地启动 `build/bin/motion-controller.exe` 后进程保持运行，SideBySide 日志无新增 manifest 错误
- `npm run test`: not applicable，当前无 `src/**/*.test.ts` 测试文件，Vitest 返回 code 1

### Known Issues
- 暂无。

## [0.2.1] - 2026-06-26

### Fixed
- 修复安装包无应用图标的问题：icon.ico 文件损坏导致 .exe 未嵌入图标资源，桌面快捷方式显示空白图标
- 构建流程补充 .syso 资源文件生成步骤，确保图标正确嵌入可执行文件

### Internal
- icon.ico 由 wails3 generate icons 从 512×512 appicon.png 重新生成（6 种分辨率）
- Taskfile.yml 新增 generate-syso 任务，在 build-go 前自动生成并清理 .syso

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `rsrc -ico + go build + NSIS` 构建安装包

### Known Issues
- 暂无。

## [0.2.0] - 2026-06-26

### Added
- Wails v3 迁移：重新生成 bindings，移除旧 wailsjs 桥接
- B140 控制器实时位置读取支持
- 优先级命令通道（priorityCmdCh），Stop/Jog/MoveTo 等动作命令不再被 Status 查询阻塞
- Dll 查找路径增强：优先从可执行文件所在目录查找 WTNMC4A_64.dll

### Changed
- 运动控制器全链路改造：状态推送重构、UI 交互增强
- ApplyConfig 接口逻辑升级：配置保存不再断连
- 运动控制主画面和配置画面布局优化与代码质量提升
- go.mod/go.sum 依赖更新适配 Wails v3

### Fixed
- Stop/EmergencyStop 完全绕过锁调用 DLL（纳秒级持锁）
- 提高停止/急停优先级，不与 Status 轮询争抢写锁
- WTNMC4A 驱动稳定性加固：DLL 崩溃防护、输入校验、锁优化
- Stop 改用 instStop 立即停止 + atomic 无锁读消除锁竞争
- UI spatial rules compliance + token system fixes

### Internal
- Wails v2 → v3 全平台迁移（daq-p1604, five-hole-interpolator, motion-controller, three-hole-interpolator）
- motion-controller 项目脚手架初始化和结构完善
- docs: README、SPEC、PLAN、AGENTS 文档补充

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `wails build`

### Known Issues
- 暂无。
