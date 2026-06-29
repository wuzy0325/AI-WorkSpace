# Changelog

## [0.1.1] - 2026-06-29

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
