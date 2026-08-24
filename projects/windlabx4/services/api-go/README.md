# WindLabX4 配套命令行工具集

本目录（`services/api-go`）下的 `cmd/` 提供若干独立 CLI 工具，可脱离整套 Wails
桌面应用单独使用。

| 工具 | 目录 | 说明 |
|---|---|---|
| **设备扫描（GUI，推荐）** | `cmd/device-scan-gui` | 图形化扫描局域网 DAQ 设备，详见 [cmd/device-scan-gui/README.md](cmd/device-scan-gui/README.md) |
| **设备扫描（CLI）** | `cmd/device-scan` | 一次性探测局域网 DAQ 设备（IP/MAC/固件），详见 [cmd/device-scan/README.md](cmd/device-scan/README.md) |
| WTN-PXI 诊断 CLI | `cmd/wtnpxi-diag` | 球罐采集协议调试，命令行，适合脚本/ssh |
| WTN-PXI 诊断 GUI | `cmd/wtnpxi-gui` | 球罐采集协议调试，原生窗口，实时通道值 |

> 现成的免安装 exe 已发布到 `projects/windlabx4/tools/bin/`（含 GUI 与 CLI 两个版本），
> 可拷贝到任意 Windows x64 机器直接运行，无需 Go 环境。详见该目录下 `README.md`。

## 从源码构建全部工具

```powershell
cd projects\WindLabX4\services\api-go

# 设备扫描 GUI（无控制台窗口）
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o device-scan-gui.exe ./cmd/device-scan-gui

# 设备扫描 CLI
go build -buildvcs=false -trimpath -ldflags "-s -w" -o device-scan.exe ./cmd/device-scan

# WTN-PXI 诊断 CLI / GUI
go build -buildvcs=false -trimpath -ldflags "-s -w" -o wtnpxi-diag.exe ./cmd/wtnpxi-diag
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o wtnpxi-gui.exe ./cmd/wtnpxi-gui
```

测试：

```powershell
go test -buildvcs=false ./cmd/device-scan/... ./cmd/device-scan-gui/... ./cmd/wtnpxi-diag/... ./cmd/wtnpxi-gui/...
```

---

# WTN-PXI 通讯协议调试工具

WTN-PXI 球罐数据采集设备通讯协议调试工具，协议定义对齐
`services/api-go/internal/adapters/hardware/wtn_pxi.go`。

提供两个版本：

| 版本 | 目录 | 说明 |
|---|---|---|
| CLI | `cmd/wtnpxi-diag` | 命令行，适合脚本/ssh |
| **GUI（推荐）** | `cmd/wtnpxi-gui` | 原生窗口，实时通道值 + 日志 + 发送框 |

## 协议概述

- TCP 连接，设备主动推送数据流
- 每帧 = 4 字节大端长度前缀（payload 字节数）+ payload
- payload = N × float32（小端），至少 8 路

8 路通道固定为：球罐压力 / 球罐总压 / 球罐静压 / 球罐稳定时间 / 球罐温度1~4
（与 `internal/adapters/config/default_profiles.go` 一致）。

## 运行（笔记本无需 Go 环境）

Go 编译产物是静态单文件 exe，不依赖任何运行时，直接拷贝到任意 Windows 10/11
x64 笔记本即可运行：

```bat
:: GUI 版：双击 wtnpxi-gui.exe，填 IP/端口 点「连接」
:: 或带参数预填并自动连接
wtnpxi-gui.exe -host 192.168.1.100 -port 9000 -autoconnect
```

```bat
:: CLI 版
wtnpxi-diag.exe -host 192.168.1.100 -port 9000
```

- `-host` 设备 IP（默认 `127.0.0.1`）
- `-port` 设备端口（默认 `9000`）
- GUI 额外支持 `-autoconnect`：启动后自动连接

## GUI 版（wtnpxi-gui.exe）

窗口界面：
- **连接面板**：主机 / 端口 / 连接 / 断开 / 状态
- **通道区**：8 路通道名 + 实时数值 + 单位
- **工具栏**：暂停显示 / 详情开关（逐帧 hex + 通道明细）/ 保存原始字节（落盘 `wtnpxi-raw-*.bin`）
- **日志区**：帧统计、hex、重同步告警（每秒一行）
- **发送框**：输入 hex（如 `00 01 02`）或 ASCII，回车或点「发送」

发送规则与 CLI 一致：输入只含 `0-9a-f` 时按 hex 发送，否则按 ASCII 发送。

## CLI 版（wtnpxi-diag.exe）

```bat
:: 帧解析模式（默认）：实时显示 8 路通道值
wtnpxi-diag.exe -host 192.168.1.100 -port 9000

:: 逐帧详情：hex 原始字节 + 通道名/单位
wtnpxi-diag.exe -detail

:: 原始 hexdump：不做帧解析，看线上真实字节
wtnpxi-diag.exe -raw

:: 原始字节落盘，便于事后分析
wtnpxi-diag.exe -log raw.bin
```

### 交互命令

| 命令 | 说明 |
|---|---|
| `<hex>` | 发送十六进制字节，如 `00 01 02` 或 `000102`（只含 0-9a-f 时按 hex 发送） |
| `ascii <文本>` | 发送 ASCII 文本（自动追加 `\r\n`） |
| `d` | 切换逐帧详情模式 |
| `p` / `r` | 暂停 / 恢复打印（接收不受影响） |
| `q` | 退出 |

每秒输出一行统计：累计帧数、字节数、帧速率、重同步次数、末帧各通道值。
长度前缀无效（0 或超长）时自动丢弃 1 字节重同步，并在统计中累计重同步次数。

## 从源码构建

```powershell
cd projects\WindLabX4\services\api-go

# CLI 版
go build -buildvcs=false -trimpath -ldflags "-s -w" -o wtnpxi-diag.exe ./cmd/wtnpxi-diag

# GUI 版（无控制台窗口，含 Common-Controls v6 manifest / DPI 感知）
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o wtnpxi-gui.exe ./cmd/wtnpxi-gui
```

GUI 版依赖 `cmd/wtnpxi-gui/app.manifest` + `rsrc.syso`（用
`go run github.com/akavel/rsrc -arch amd64 -manifest app.manifest -o rsrc.syso`
重新生成，已提交在目录内）。

测试：

```powershell
go test -buildvcs=false ./cmd/wtnpxi-diag/... ./cmd/wtnpxi-gui/...
```

