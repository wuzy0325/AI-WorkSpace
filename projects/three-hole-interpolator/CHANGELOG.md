# Changelog

## [Deprecated] - 2026-07-20

### Deprecation

- 本项目已被 [`projects/probe-interpolator`](../probe-interpolator) 取代。
  新项目将 5 孔 / 3 孔 / 7 孔探针插值整合为单一桌面程序，提供统一 UI 与共享算法包。
- 仓库代码与历史 release 制品保留，但不再发布新版本或修复缺陷。
- 请迁移至 `probe-interpolator` v0.1.0+ 进行后续开发与使用。

### Migration

- 算法包复用：`shared/algorithms/go/threehole/interpolation` 不变，`probe-interpolator` 直接引用。
- PRB 文件格式：完全兼容，可直接迁移。
- CSV 输入/输出格式：完全兼容（含 PressureMode 列）。
- 帮助文档：`probe-interpolator` 内置 `three-hole-用户说明书.html`，与本项目内容一致。

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
