# Changelog

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
