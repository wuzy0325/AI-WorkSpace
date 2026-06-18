# Changelog

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
