# Changelog

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
