# Spec: WindLabX4 独立设备扫描程序 (`device-scan`)

## Objective

把 WindLabX4 中"局域网设备扫描"能力从后端 `DeviceManager.ScanDevices()` 中剥离，
做一个**独立的命令行程序**，方便现场/运维人员不启动整套桌面应用就能直接探测
局域网内的 DAQ 设备（DAQ-P-1604 / DAQ-T-1603 / DAQ-P-1604Pre）。

用户故事：
- 现场工程师拿到一台新设备，不想装整套 Wails 桌面应用，只想跑一个 exe 确认设备
  是否在线、拿到 IP/MAC/固件版本 → `device-scan.exe` 一次性输出表格。
- CI / 部署脚本需要在装机后验证设备可达 → 以 JSON 退出码形式返回结果（预留）。

## Tech Stack

- 语言：Go 1.25（与 api-go 模块一致 `windlabx4/services/api-go`）
- 复用：`internal/adapters/scan.NetworkScanner`（仅依赖 `internal/core/device`）
- 无新增第三方依赖（沿用 `flag` + 标准库）
- Windows 控制台 UTF-8 输出（仿 `wtnpxi-diag` 的 `setupConsole`）

## Commands

```bash
# 构建（在 services/api-go 目录下）
go build -o bin/device-scan.exe ./cmd/device-scan

# 运行：一次性扫描并表格打印
bin/device-scan.exe

# 自定义超时
bin/device-scan.exe -timeout 5s

# 仅 JSON 输出（脚本消费）
bin/device-scan.exe -json

# 导出到文件
bin/device-scan.exe -out scan-result.json

# 限定扫描范围（二选一，互斥）
bin/device-scan.exe -iface Ethernet0
bin/device-scan.exe -subnet 192.168.1.0/24

# 编译检查
go vet ./cmd/device-scan
go build ./cmd/device-scan
```

## Project Structure

```
services/api-go/
  cmd/device-scan/
    main.go            ← 新增：CLI 入口、flag 解析、调用 NetworkScanner、格式化输出
  internal/adapters/scan/   ← 复用，不改动
    network_scanner.go
    parsers.go
    discovery_socket*.go
  internal/core/device/      ← 复用，不改动
    types.go (ScanResult 等)
```

## Code Style

- 与 `wtnpxi-diag/main.go` 风格一致：文件头注释说明用途与用法、`setupConsole()` 切换
  Windows UTF-8 代码页、`fmt` 打印中文。
- 输出表格用 `text/tabwriter`，字段对齐。
- 复用 `scan.NewNetworkScanner(scan.WithTimeout(...))` 构造扫描器。

示例输出：
```
========================================
 WindLabX4 设备扫描 (timeout=3s)
========================================
 类型            IP              端口  MAC                固件
 DAQ-T-1603      192.168.1.101   9000  A1:B2:C3:00:11:22  v2.3.1
 DAQ-P-1604      192.168.1.102   9000  A1:B2:C3:00:22:33  v1.8.0
----------------------------------------
 共发现 2 台设备
```

## Testing Strategy

- 复用 `internal/adapters/scan` 既有单测（不在此程序内新增，避免重复）。
- 验收通过 `go build` + 手动在有设备/无设备环境下运行确认输出格式与退出码。
- `-json` / `-out` 路径做基本冒烟（输出可被 `json.Unmarshal` 解析）。

## Boundaries

- Always: 复用现有 `scan.NetworkScanner`，不复制扫描逻辑；输出中文走 UTF-8。
- Ask first: 若需新增设备类型扫描（如 DSA3217/PACE1000 走非 UDP 发现协议）。
- Never: 不修改 `internal/adapters/scan` 或 `DeviceManager` 现有逻辑；不引入新依赖。

## Success Criteria

- [ ] `go build ./cmd/device-scan` 通过，无编译错误。
- [ ] 运行后打印表格，列出扫描到的设备（类型/IP/端口/MAC/固件），无设备时输出"未发现设备"。
- [ ] `-timeout` 生效；`-json` 输出合法 JSON 数组；`-out` 写入文件。
- [ ] 退出码：发现设备=0，扫描出错=1，无设备（正常）=0。

## Open Questions

- 无（需求已确认：一次性 CLI 工具 + 表格文本 + 全网卡广播）。
