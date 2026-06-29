# Changelog

## [0.2.1] - 2026-06-29

### Internal
- 规范化生产构建标签：Taskfile.yml `build-go` 增加 `-tags production -trimpath` 与 `-w -s`。

### Verification
- `go test ./...`: passed（无测试文件）
- `npm run typecheck`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed
- `makensis` 构建安装包: passed

### Known Issues
- 暂无。

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
