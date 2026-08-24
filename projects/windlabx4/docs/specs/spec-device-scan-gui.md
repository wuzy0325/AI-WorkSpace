# Spec: WindLabX4 设备扫描 GUI 工具 (`device-scan-gui`)

## Objective

在既有 `device-scan` CLI 基础上，提供一个**图形化小工具窗口**：双击 exe 直接启动，
无需命令行参数；界面提供「开始扫描」按钮、扫描结果列表展示（TableView）、以及
扫描范围（网卡 / 子网）选项。复用 `internal/adapters/scan.NetworkScanner`，与 CLI
共享同一套扫描逻辑。

目标用户：现场工程人员，不愿操作命令行，只想点一下按钮就看到局域网内有哪些 DAQ 设备。

## Tech Stack

- 语言：Go 1.25（与 api-go 模块一致 `windlabx4/services/api-go`）
- GUI 框架：`github.com/lxn/walk` + `github.com/lxn/walk/declarative`（项目已有，
  `wtnpxi-gui` 同款方案）
- 复用：`internal/adapters/scan.NetworkScanner`、`scan.ScopedDiscoveryTargets`
- 无新增第三方依赖
- 构建：`-H windowsgui`（无控制台窗口）

## Commands

```bash
# 构建（在 services/api-go 目录下）
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o device-scan-gui.exe ./cmd/device-scan-gui

# 双击 device-scan-gui.exe 即可启动，点「开始扫描」查看结果
```

## Project Structure

```
services/api-go/
  cmd/device-scan-gui/
    app.manifest          ← 高 DPI / Common-Controls v6（仿 wtnpxi-gui）
    rsrc.syso             ← 由 manifest 生成（如 wtnpxi-gui 所示）
    main.go               ← 窗口构建 + 扫描后台 goroutine + UI 刷新循环
  internal/adapters/scan/        ← 复用，不改动
```

## Code Style

- 与 `cmd/wtnpxi-gui/main.go` 风格一致：`MainWindow` + declarative 构建、`uiRefs`
  持有控件引用、后台 goroutine 执行 I/O、定时器刷新 UI。
- 扫描结果用 `TableView` + 自定义 `walk.ReflectTableModel`（或 ColumnSpec）展示。
- 扫描期间禁用「开始扫描」按钮，完成后恢复，状态标签显示进度/结果。

示例窗口布局：
```
┌──────────────────────────────────────────────┐
│ [范围: (网卡下拉 ▼) ] [子网 CIDR: ____]       │
│ [开始扫描]  [状态: 共发现 N 台设备]            │
├──────────────────────────────────────────────┤
│ 类型       IP            端口  MAC  固件 型号 │
│ DAQ-T-1603 192.168.1.101 9000 ...   v2.3  ... │
├──────────────────────────────────────────────┤
│ [日志区: 扫描开始/结束/错误]                  │
└──────────────────────────────────────────────┘
```

## Testing Strategy

- 复用 `internal/adapters/scan` 既有单测（扫描逻辑不变）。
- `device-scan-gui` 以构建成功 + 人工启动冒烟为准；不引入复杂 GUI 单测（与
  `wtnpxi-gui` 一致，仅少量辅助函数可测）。
- 冒烟：`go build` 通过、双击启动、点「开始扫描」能列出设备 / 空结果显示"未发现"。

## Boundaries

- Always: 复用 `NetworkScanner`，不复制扫描逻辑；UI 层仅做展示与触发。
- Ask first: 若要新增设备类型扫描或改底层扫描目标计算逻辑。
- Never: 不修改 `internal/adapters/scan` 或 CLI 既有逻辑；不引入新的 GUI 框架。

## Success Criteria

- [ ] `go build -H windowsgui ./cmd/device-scan-gui` 通过，产物为无控制台窗口的 exe。
- [ ] 双击启动显示窗口，无黑框；点「开始扫描」后列表展示设备（类型/IP/端口/MAC/固件）。
- [ ] 支持选择网卡 / 输入子网 CIDR 限定扫描范围；扫描期间按钮禁用、状态提示。
- [ ] 无设备时列表空、状态显示「未发现设备」；扫描出错在状态/日志区提示。

## Open Questions

- 是否需要在 GUI 里支持导出结果到文件？（可选，默认仅展示）
