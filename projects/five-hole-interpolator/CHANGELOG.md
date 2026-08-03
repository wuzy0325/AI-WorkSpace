# Changelog

## [Deprecated] - 2026-07-20

### Deprecation

- 本项目已被 [`projects/probe-interpolator`](../probe-interpolator) 取代。
  新项目将 5 孔 / 3 孔 / 7 孔探针插值整合为单一桌面程序，提供统一 UI 与共享算法包。
- 仓库代码与历史 release 制品保留，但不再发布新版本或修复缺陷。
- 请迁移至 `probe-interpolator` v0.1.0+ 进行后续开发与使用。

### Migration

- 算法包复用：`shared/algorithms/go/fivehole/interpolation` 不变，`probe-interpolator` 直接引用。
- PRB 文件格式：完全兼容，可直接迁移。
- CSV 输入/输出格式：完全兼容（含 PressureMode 列）。
- 帮助文档：`probe-interpolator` 内置 `five-hole-用户说明书.html`，与本项目内容一致。

## [0.2.2] - 2026-07-13

### Changed

- 五孔探针 PRB 插值算法重构：精简代码约 160 行，提高可维护性（同步 `shared/algorithms/go/fivehole/interpolation/prb_interpolator.go` 变更）。

### Behavior Changes

- 超范围输入（压力系数落在 ±30° 校准网格凸包外）不再钳位到角点 (±30, ±30)；
  改为返回 `IsValid=false` 并附 Warning `"压力系数超出PRB校准网格，旧算法不支持外推"`。
  下游消费者需显式处理 `IsValid=false` 路径。

### Verification

- `go test ./...`
- `npm run typecheck`
- `npm run build`
- `go build -tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"`
- `makensis` 构建安装包

### Known Issues

- 暂无。

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
