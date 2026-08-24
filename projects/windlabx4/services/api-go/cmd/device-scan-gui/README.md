# device-scan-gui 设备扫描图形工具

WindLabX4 设备扫描工具的**图形化窗口版**：双击 exe 直接启动，无需命令行参数。
点一下「开始扫描」，就能在表格里看到局域网内的 DAQ 设备
（DAQ-P-1604 / DAQ-T-1603 / DAQ-P-1604Pre），信息包括类型 / IP / 端口 / MAC / 固件 / 型号。

与 CLI 版 `device-scan` 复用同一套扫描逻辑（`internal/adapters/scan.NetworkScanner`）。

## 使用

双击 `device-scan-gui.exe` 启动，然后：

1. 在「范围」下拉框选择扫描方式：
   - **全网卡广播**（默认）：遍历全部启用网卡 + 受限广播，覆盖所有设备。
   - **指定网卡**：右侧下拉选择网卡名，只扫该网卡。
   - **指定子网**：右侧输入 CIDR（如 `192.168.1.0/24`），只扫该子网。
2. 点「开始扫描」。扫描期间按钮禁用，完成后表格显示结果、状态栏显示设备数。
3. 底部日志区记录扫描开始 / 完成 / 错误信息。

无设备时表格为空、状态显示「共发现 0 台设备」；范围参数非法会在日志区提示。

## 构建

```powershell
cd projects\WindLabX4\services\api-go

# 生成 manifest 资源（首次或修改 app.manifest 后）
go run github.com/akavel/rsrc@latest -arch amd64 -manifest cmd/device-scan-gui/app.manifest -o cmd/device-scan-gui/rsrc.syso

# 构建（无控制台窗口）
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o device-scan-gui.exe ./cmd/device-scan-gui
```

`rsrc.syso` 已提交在 `cmd/device-scan-gui/` 内，通常无需重新生成。
