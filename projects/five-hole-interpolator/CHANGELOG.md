# Changelog

## [0.2.0] - 2026-06-26

### Added
- Wails v3 迁移：重新生成 bindings，移除旧 wailsjs 桥接
- 首次正式打包发布

### Changed
- 从 wind-daq 分离为独立项目
- go.mod/go.sum 依赖更新适配 Wails v3

### Verification
- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `wails build`

### Known Issues
- 暂无。
