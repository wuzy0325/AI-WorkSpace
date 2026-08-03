# Windows UDP 设备扫描不返回问题：根因与修复方案

## 0. 代码位置与调用链

UDP 扫描的主要实现位于：

```text
apps/desktop-wails/adapters/hardware/p1604_scanner.go
```

相关代码位置：

- 扫描入口和广播发送：`P1604Scanner.Scan`
- 同步 Winsock 创建与配置：`openP1604DiscoverySocket`
- 响应接收和去重：`readScanResponses`
- 响应 CSV 解析：`parseP1604Response`
- 网卡广播地址枚举：`broadcastTargets`
- 扫描测试：`apps/desktop-wails/adapters/hardware/p1604_scanner_test.go`

从前端到硬件适配器的调用链：

```text
frontend/src/stores/deviceStore.ts
    -> frontend/src/bridge/deviceBridge.ts
    -> backend/app.go: App.ScanDevices
    -> usecase/device_usecase.go: DeviceUsecase.ScanDevices
    -> ports/scan.go: DeviceScanPort.Scan
    -> adapters/hardware/p1604_scanner.go: P1604Scanner.Scan
```

## 1. 问题现象

在当前 Windows 电脑上，DAQ-P-1604 设备地址为 `192.168.1.7:9000`，设备实际在线，但应用通过 UDP 扫描时无法在界面中显示设备。同一程序在其他电脑上可以完成扫描。

表面现象是“扫描不到设备”，实际情况是扫描函数收到设备响应后没有结束，导致结果始终无法返回到前端。

## 2. 网络环境与诊断结果

诊断时的网络信息如下：

- 本机有线网卡：`192.168.1.2/24`
- 设备地址：`192.168.1.7`
- 设备 TCP 服务端口：`9000`
- UDP 发现命令：`psi9000`
- UDP 发送端口：`7000`
- UDP 本地接收端口：`7001`

实际检查结果：

1. `ping 192.168.1.7` 正常，未丢包。
2. TCP `192.168.1.7:9000` 连接成功。
3. 本机 ARP 表能够解析设备 MAC 地址 `00-02-5E-37-4D-87`。
4. Windows 防火墙存在该应用的 Public 入站放行规则。
5. 从 `192.168.1.2:7001` 向 UDP 7000 端口发送 `psi9000` 后，设备立即返回发现响应：

```text
192.168.1.7,00-02-5E-37-4D-87,0,9116,2.48,0,1,9000,255.255.255.0,0,0,0x0,
```

以上结果证明：

- 设备在线且工作正常。
- 本机到设备的有线网络正常。
- UDP 发现请求能够到达设备。
- UDP 发现响应能够返回本机。
- 响应格式满足现有解析器要求。

因此，问题不在设备、网络连通性或 UDP 协议本身，而在应用结束 UDP 接收循环的方式。

## 3. 根因

### 3.1 旧实现

旧代码通过 Windows `setsockopt` 在底层 socket 上设置 `SO_RCVTIMEO`，然后调用 Go 的 `net.PacketConn.ReadFrom` 循环接收响应，期望超时后 `ReadFrom` 自动返回错误：

```go
tv := uint32(timeout.Milliseconds())
_ = windows.Setsockopt(
    handle,
    windows.SOL_SOCKET,
    windows.SO_RCVTIMEO,
    (*byte)(unsafe.Pointer(&tv)),
    4,
)
```

### 3.2 为什么该实现有问题

Windows 下，Go 的 `net` 包通过 Go runtime 的网络轮询器和 IOCP 管理异步网络 I/O。直接修改底层 socket 的 `SO_RCVTIMEO`，不等同于通过 Go `net` API 设置读取期限，也不能作为中断 Go 异步 `ReadFrom` 的可靠机制。

设备响应到达后，代码会成功读取并保存结果，然后再次进入 `ReadFrom`，等待其他设备响应。由于底层 `SO_RCVTIMEO` 没有使这次等待按预期结束，扫描函数一直阻塞：

```text
发送发现请求
    -> 收到并解析 192.168.1.7 的响应
    -> 再次调用 ReadFrom 等待更多响应
    -> 读取没有按预期超时
    -> Scan() 不返回
    -> 前端始终拿不到已经收集到的结果
```

运行时还观察到应用长期占用 `0.0.0.0:7001`，而正常扫描只应占用该端口约 3 秒。这与接收循环未退出的判断一致。

### 3.3 责任归属

已确认的首要问题是旧扫描代码错误混用了同步 Winsock 超时选项和 Go 异步网络 I/O。这是应用实现缺陷，不能归因于设备或简单描述为“Go 语言不稳定”。

同时，本机后续实测发现：改用 Go 标准 `SetReadDeadline`，或从另一 goroutine 调用 `Close` 后，Go `1.26.5` 仍卡在 Windows runtime 的 `cancelIO` 路径。这不是应用通常应观察到的结果，但当前只有一台故障电脑的证据，尚不足以判定为 Go runtime 缺陷。

可能影响该兼容表现的因素包括：

- 特定 Go runtime 版本的 Windows 网络回归。
- Windows 版本及系统补丁组合。
- 网卡驱动或 Winsock 服务提供程序差异。
- VPN、杀毒软件、安全软件或流量过滤驱动介入网络栈。

因此责任边界是：旧代码错误已经确认；本机 IOCP 取消异常的底层归属尚未完全确认。最终实现选择绕开该路径，以保证生产行为不依赖这些环境差异。

## 4. 最终修复方案

### 4.1 使用同步 Winsock UDP socket

第一次修复曾尝试在计时器到期后调用 `net.PacketConn.Close()`，随后又尝试 Go 标准的 `SetReadDeadline`。在当前 Windows 和 Go `1.26.5` 环境的真实 UDP socket 集成测试中，两种方式均卡在 runtime 的 `cancelIO` 路径，不能结束 `ReadFrom`。

最终实现改为直接创建不接入 Go IOCP 网络轮询器的同步 Winsock UDP socket，并在同步 `recvfrom` 上使用 `SO_RCVTIMEO`：

```go
func readScanResponses(socket windows.Handle, timeout time.Duration) []core.ScanResult {
    results := make([]core.ScanResult, 0)
    seen := make(map[string]bool)
    buf := make([]byte, 1024)
    deadline := time.Now().Add(timeout)
    for {
        remaining := time.Until(deadline)
        if remaining <= 0 {
            break
        }
        if err := windows.SetsockoptInt(
            socket,
            windows.SOL_SOCKET,
            windows.SO_RCVTIMEO,
            max(1, int(remaining.Milliseconds())),
        ); err != nil {
            break
        }
        n, remote, err := windows.Recvfrom(socket, buf, 0)
        if err != nil {
            break
        }
        result := parseP1604Response(buf[:n], sockaddrString(remote))
        if result != nil && !seen[result.ID] {
            seen[result.ID] = true
            results = append(results, *result)
        }
    }
    return results
}
```

这种实现中 `SO_RCVTIMEO` 与同步 `recvfrom` 属于同一套 Winsock I/O 模型，不再与 Go 的异步 IOCP 读取混用。接收循环维护绝对截止时间，并在每次 `Recvfrom` 前把剩余时间写入 `SO_RCVTIMEO`；因此收到多个有效或无效报文也不会把总扫描窗口不断延长。超时后 `Recvfrom` 返回错误，接收循环退出并返回已收集结果。

### 4.2 socket 初始化

扫描器通过同步 Winsock API 完成以下操作：

- `windows.Socket` 创建 UDP socket。
- `SO_BROADCAST` 允许发送广播。
- `SO_RCVTIMEO` 设置同步接收超时，并按绝对截止时间更新剩余时长。
- `windows.Bind` 绑定本地 UDP 7001。
- `windows.Sendto` 发送发现请求。
- `windows.Recvfrom` 收集设备响应。
- `windows.Closesocket` 释放 socket。

### 4.3 限制网卡枚举耗时

扫描广播地址前，通过 `broadcastTargetsWithTimeout` 将网卡枚举限制为 500ms。若枚举异常缓慢，则回退到有限广播地址 `255.255.255.255`，避免尚未创建 UDP socket 时扫描就永久卡住。

```go
targets := broadcastTargetsWithTimeout(
    p1604InterfaceTimeout,
    broadcastTargets,
)
```

## 5. 修复后的扫描流程

```text
枚举广播地址，最多等待 500ms
    -> 绑定本地 UDP 7001
    -> 向 UDP 7000 发送 psi9000
    -> 在 3 秒窗口内收集并去重设备响应
    -> 到达固定 3 秒总截止时间后 Recvfrom 超时
    -> 接收循环退出并关闭 socket
    -> 返回全部扫描结果给前端
```

该流程保证无论扫描到零台、一台或多台设备，扫描函数都会在有限时间内返回。

## 6. 测试覆盖

对应单元测试使用真实本地同步 Winsock UDP socket，验证：

- 接收函数能够在超时后返回。
- 未收到响应时返回空结果。

测试位置：

```text
apps/desktop-wails/adapters/hardware/p1604_scanner_test.go
```

关键测试：

```go
func TestReadScanResponsesReturnsAtTimeout(t *testing.T)
```

另外还覆盖了网卡枚举超时后回退到 `255.255.255.255` 的行为，并提供可选真实设备集成测试 `TestP1604ScannerIntegration`。

## 7. 验证结论与限制

协议级实测已确认设备可以正常返回发现响应。最终修复通过了同步 Winsock 超时单元测试；真实设备集成测试在约 `3.08s` 内返回，并识别到 `192.168.1.7:9000`。代码已执行 `gofmt`。

生产构建验证结果：

- 已修复 `vendor/modules.txt` 中本地模块缺少 replace 标记的问题。
- 已使用 `GOWORK=off` 和 `production` 标签完成生产 EXE 构建。
- 已验证生产 EXE 可以启动，主窗口正常响应。
- 最终 EXE SHA-256：`47925B85FEEA16046B2DDD0DDA73A3CB4830E2F6C3ED81507EDDFD8911547EEF`。

前端共享源码目录在当前工作区缺失，因此本次仅后端变更的生产 EXE 嵌入了已有的最后一次成功前端构建产物。UDP 扫描修复位于 Go 后端，不受该限制影响。

## 8. 后续排查原则

再次出现“扫描不到设备”时，应按以下顺序判断：

1. 确认本机和设备处于同一子网。
2. 确认设备 IP 可以 ping 通，TCP 9000 可以连接。
3. 确认 UDP 7001 未被其他进程长期占用。
4. 抓取或直接测试 `psi9000` 请求是否收到 UDP 响应。
5. 若已经收到响应但界面没有结果，检查扫描函数是否按超时退出并返回。
6. 最后再检查响应解析和前端展示逻辑。

“设备没有响应”和“应用收到响应但没有返回结果”是两类不同故障，必须通过协议级收包结果区分。

## 9. 异地现场诊断

无法直接操作故障电脑时，应要求现场人员保留应用日志并执行以下检查。不要仅凭界面显示“扫描超时”判断设备没有响应。

### 9.1 基础网络检查

```powershell
Get-NetIPConfiguration
Get-NetConnectionProfile
Get-NetRoute -AddressFamily IPv4
ping.exe -n 4 192.168.1.7
Test-NetConnection 192.168.1.7 -Port 9000 -InformationLevel Detailed
arp.exe -a 192.168.1.7
Get-NetUDPEndpoint -LocalPort 7001 -ErrorAction SilentlyContinue
```

需要关注：

- 本机是否有与设备同网段的 IPv4 地址。
- TCP 9000 是否可以连接。
- UDP 7001 是否已被其他进程占用。
- 是否同时启用 Wi-Fi、VPN、虚拟网卡等可能改变路由的接口。
- 有线网络是否被标记为 Public，以及防火墙是否放行应用入站 UDP。

### 9.2 Windows 内置抓包

使用管理员 PowerShell：

```powershell
pktmon filter remove
pktmon filter add P1604 -p 17
pktmon start --etw

# 在应用中执行一次设备扫描

pktmon stop
pktmon format PktMon.etl -o p1604-pktmon.txt
```

将以下材料一并提供给开发人员：

- `p1604-pktmon.txt`
- 应用扫描时段日志
- `Get-NetIPConfiguration` 输出
- `Get-NetRoute -AddressFamily IPv4` 输出
- 应用版本、EXE SHA-256、Windows 版本和 Go 构建版本

### 9.3 故障判定矩阵

| 观察结果 | 判断方向 |
|---|---|
| 没有发出目标端口 7000 的 `psi9000` | 检查 socket 创建、UDP 7001 绑定、广播目标和发送错误 |
| 请求已发出但没有设备回包 | 检查设备、交换机、VLAN、防火墙、网卡和广播隔离 |
| 收到 UDP 回包但响应格式无效 | 检查设备固件协议和响应解析器 |
| 收到并解析成功但后端不返回 | 检查接收超时、循环退出和 socket 释放 |
| 后端已返回但界面仍报超时 | 检查 Wails 绑定调用和前端 5 秒超时 |
| UDP 7001 长期被同一进程占用 | 检查扫描调用是否卡住或 socket 未释放 |
| UDP 7001 被其他进程占用 | 查明 PID，关闭冲突程序或调整端口使用方式 |

## 10. 建议的应用诊断日志

为支持无人到场排查，扫描器后续应在应用日志中记录以下结构化信息：

- 应用版本、Go 构建版本、Windows 版本。
- 活动网卡名称、IPv4 地址、掩码和计算出的广播地址。
- UDP 7001 socket 创建、选项设置和绑定结果。
- 每个 `psi9000` 发送目标及 `Sendto` 错误码。
- 每个回包的来源 IP、来源端口、字节数和解析结果。
- 接收循环结束原因，包括正常超时和具体 Winsock 错误码。
- 扫描总耗时、有效响应数、无效响应数和最终设备数。

这些日志必须区分“未收到响应”“收到但解析失败”“已解析但未返回”和“前端等待超时”，否则异地只能看到相同的“扫描失败”文案，无法定位故障层级。
