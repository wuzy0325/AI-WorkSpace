# device-scan 局域网设备扫描工具

WindLabX4 的独立设备扫描命令行工具，把后端 `DeviceManager.ScanDevices()` 的扫描能力
从整套桌面应用中剥离出来，复用 `internal/adapters/scan.NetworkScanner` 一次性探测
局域网内的 DAQ 设备（DAQ-P-1604 / DAQ-T-1603 / DAQ-P-1604Pre）。

适合现场排查：不装整套 Wails 桌面应用，直接跑一个 exe 确认设备是否在线、拿到
IP / MAC / 固件版本。

## 用法

```bat
:: 一次性扫描并打印表格（默认全网卡广播）
device-scan.exe

:: 自定义扫描超时
device-scan.exe -timeout 5s

:: 仅扫描指定网卡（按名称，如 Ethernet0 / WLAN）
device-scan.exe -iface Ethernet0

:: 仅扫描指定子网（CIDR）
device-scan.exe -subnet 192.168.1.0/24

:: 以 JSON 数组输出（便于脚本消费）
device-scan.exe -json

:: 导出到文件（JSON 格式）
device-scan.exe -out scan-result.json
```

### 参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-timeout` | `3s` | 扫描超时时间，例如 `5s` / `3000ms` |
| `-iface` | 空 | 仅扫描指定网卡（按名称）；与 `-subnet` 互斥 |
| `-subnet` | 空 | 仅扫描指定 IPv4 子网（CIDR，如 `192.168.1.0/24`）；与 `-iface` 互斥 |
| `-json` | `false` | 以 JSON 数组形式输出结果 |
| `-out` | 空 | 将结果写入指定文件（与 `-json` 共用格式） |

### 限定扫描范围

默认扫描遍历全部启用网卡、对每个网卡 IP 末位 `.7/.9/.101/.102/.104/.200/.202/.254`
发送单播发现包，并叠加受限广播 `255.255.255.255`。现场设备集中在某张网卡或某子网时，
可用 `-iface` / `-subnet` 缩小发送面、加快扫描：

- `-iface Ethernet0`：仅对该网卡的 IPv4 地址末位按上述 octet 列表生成发现候选。
- `-subnet 192.168.1.0/24`：仅对 `192.168.1.x` 的上述 octet 末位发送发现包。

两种限定方式均**不**再发送受限广播（用户已明确希望缩小范围）。`-iface` 与 `-subnet`
互斥，同时指定会报错退出。

### 输出示例

```
========================================
 WindLabX4 设备扫描 (timeout=3s)
========================================
 类型            IP              端口  MAC                固件    型号
 DAQ-P-1604      192.168.1.7     9000  AC:23:3F:...       2.48    ...
----------------------------------------
 共发现 1 台设备
```

无设备时输出「未发现设备」。`-json` 输出结构对齐 `internal/core/device.ScanResult`
（字段：`Type` / `Address` / `Port` / `MacAddress` / `FirmwareVersion` / `Model`）。

### 退出码

- `0`：正常（无论是否发现设备）
- `1`：扫描出错（如无法打开套接字）

## 从源码构建

```powershell
cd projects\WindLabX4\services\api-go
go build -buildvcs=false -trimpath -ldflags "-s -w" -o device-scan.exe ./cmd/device-scan
```

测试：

```powershell
go test -buildvcs=false ./cmd/device-scan/...
```

> 注：扫描逻辑本身由 `internal/adapters/scan` 既有单测覆盖，本程序仅做 CLI 封装，
> 未新增底层测试。
