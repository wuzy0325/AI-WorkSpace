# WindLabX4 独立设备扫描工具

本目录存放 WindLabX4 的独立设备扫描工具（免安装单文件 exe，不依赖任何运行时，
可直接拷贝到任意 Windows 10/11 x64 机器运行）。

| 文件 | 说明 |
|---|---|
| `device-scan-gui.exe` | 图形化工具（推荐）：双击启动，选扫描范围后点「开始扫描」，表格展示局域网 DAQ 设备 |
| `device-scan.exe` | 命令行工具：适合脚本 / ssh / 自动化调用 |

两个 exe 功能等价、各自独立，共用同一套扫描逻辑，探测局域网内的 DAQ 设备
（DAQ-P-1604 / DAQ-T-1603 / DAQ-P-1604Pre，含 9116 等不响应 UDP 广播的 P1604 变体）。

扫描分两层：

- **UDP 广播发现**：向各网段广播地址 7000 端口发送发现命令，设备回 CSV
  （含 IP / MAC / 固件 / 子网掩码 / 网关）。
- **TCP 探测兜底**：部分 P1604 变体（如型号 9116）只响应广播、不响应单播 UDP
  发现。对候选地址并发探测 TCP 9000，经 `w1601` 握手后 `q00` 读取型号识别，
  因此 `-iface` / `-subnet` 限定范围内也能发现这类设备。

表格列：类型 / IP / 端口 / MAC / 固件 / 型号 / 子网掩码 / 网关。
（TCP 探测发现的设备仅含型号，子网掩码 / 网关留空。）

## 用法

### GUI 版（推荐）

双击 `device-scan-gui.exe` 启动，然后：
1. 在「范围」下拉框选择：全网卡广播（默认）/ 指定网卡 / 指定子网（CIDR）。
2. 点「开始扫描」。完成后表格显示类型 / IP / 端口 / MAC / 固件 / 型号 / 子网掩码 / 网关。

### CLI 版

```bat
:: 一次性扫描并打印表格（默认全网卡广播）
device-scan.exe

:: 限定扫描范围（二选一）
device-scan.exe -iface Ethernet0
device-scan.exe -subnet 192.168.1.0/24

:: JSON 输出 / 导出到文件
device-scan.exe -json
device-scan.exe -out scan-result.json

:: 自定义超时
device-scan.exe -timeout 5s
```

CLI 退出码：`0` 正常（无论是否发现设备）；`1` 扫描出错或参数无效。

## 源码与重新构建

源码位于 `services/api-go/cmd/device-scan` 与 `cmd/device-scan-gui`，重新构建命令见
`services/api-go/cmd/device-scan/README.md` 与 `cmd/device-scan-gui/README.md`。
