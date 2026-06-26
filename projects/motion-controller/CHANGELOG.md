# Changelog

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
