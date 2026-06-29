# Changelog

## [0.1.1] - 2026-06-29

### Internal
- README.md 区分开发与交付命令，新增 Release Commands 段。
- 创建 VERSION、CHANGELOG.md 和发布基础设施。

### Verification
- `go test ./...`: passed
- `npm run build`: passed
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`: passed

### Known Issues
- 暂无。
