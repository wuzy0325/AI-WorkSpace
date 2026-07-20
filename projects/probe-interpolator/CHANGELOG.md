# Changelog

## [0.1.0] - 2026-07-20

### Added

- 首次发布：将 5 孔 / 3 孔 / 7 孔探针插值整合为单一桌面程序。
- 启动选择页：3 个卡片按钮，session-locked（重启后切换探针类型）。
- 三个独立工作区组件：`FiveHoleWorkspace.vue` / `ThreeHoleWorkspace.vue` / `SevenHoleWorkspace.vue`，
  通过 `App.vue` 动态 `import()` 懒加载，避免启动时一次性加载全部。
- 共享算法包引用：
  - `shared/algorithms/go/fivehole/interpolation`
  - `shared/algorithms/go/threehole/interpolation`
  - `shared/algorithms/go/sevenhole/interpolation`
- 三套用户说明书（HTML）：`five-hole-用户说明书.html` / `three-hole-用户说明书.html` / `seven-hole-用户说明书.html`。
- 探针选择器（`probe_selector.go`）：基于 `sync.RWMutex` 的并发安全实现，
  每个 probe 类型独立 state（含自己的 `sync.RWMutex`），避免锁混用。
- 7 孔专属 API：`LoadSevenHolePrbFiles` / `CalculateSevenHole` / `BatchCalculateSevenHole` 等 8 个方法，
  所有类型加 `SevenHole` 前缀避免 Wails binding 生成冲突。
- 7 孔后端测试套件：11 个测试函数覆盖内区/外区插值、批量计算、并发安全、PRB 加载等场景，
  使用算法包 golden test data（`boundary.json`）作为权威输入输出对。

### Behavior

- 5 孔 / 3 孔工作区与旧独立程序功能等价：PRB 文件格式、CSV 输入/输出格式、压力模式开关完全兼容。
- 7 孔工作区遵循 spec §1.1：强制表压输入，不提供 PressureMode 切换开关；
  CSV 必含 P1-P7 + Patm + Tatm 共 9 列，全部必需。
- 7 孔结果 α/β 语义与 5 孔反转（spec §2.2）：α=侧滑角、β=迎角，CSV 导出表头明确标注物理含义。
- 7 孔 PRB 加载要求文件名 basename 为 1~7 的纯数字（7.prb=内区，1~6.prb=外区扇区 n）。

### Deprecation

- 旧独立程序 `projects/five-hole-interpolator` 与 `projects/three-hole-interpolator` 标记为 deprecated。
- 旧项目仓库代码与历史 release 制品保留，不再发布新版本或修复缺陷。
- 用户应迁移至本项目（`probe-interpolator`）进行后续开发与使用。

### Verification

- `GOWORK=off; go build ./...`: passed
- `GOWORK=off; go vet ./...`: passed
- `GOWORK=off; go test -race ./backend/...`: passed（含 7 孔 11 个测试 + 5 孔 / 3 孔既有测试）
- `npm --prefix frontend run typecheck`: passed
- `npm --prefix frontend run build`: passed

### Known Issues

- 暂无。
