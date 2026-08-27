# Changelog

## [1.0.1] - 2026-08-27

### Changed
- 通讯日志面板不再记录实时压力轮询（poll）命令与响应，避免高频轮询日志不断刷新刷屏。

### Verification
- `npm run typecheck`
- `npm run lint`
- `go test ./cmd/... ./internal/...`
- `wails build --nsis`

### Known Issues
- 暂无。