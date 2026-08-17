# HIL (Hardware-in-Loop) Validation Plan

> Updated: 2026-05-21 — Added precise SCPI commands, adapter error counters, and per-device verification scripts.

## 目标
使用真实 DAQ 硬件验证采集、校准和运动控制流程。

## 设备清单

| 设备型号 | 接口 | 协议 | 用途 |
|---------|------|------|------|
| DAQ-P-1604 | RS-232/RS-485 | SCPI + Binary Stream | 压力扫描阀 |
| DAQ-T-1603 | RS-232/RS-485 | SCPI + Binary Frame | 热电偶采集 |
| B140 | RS-232 | Custom ASCII | 4 轴运动控制 |
| WTN1605 | RS-232 | SCPI | 压力校准仪 |

## 准备检查

- [ ] 串口转 USB 驱动已安装
- [ ] 设备已上电（LED 状态灯正常）
- [ ] 串口号已确认（Windows: `设备管理器 → 端口 (COM 和 LPT)`）
- [ ] 后端已启动：`go run .\cmd\server\main.go`
- [ ] 调试日志已启用：`$env:WINDLABX4_DEBUG = "1"`

---

## Phase 1: DAQ-P-1604 HIL

### 1.1 手动串口验证（跳过软件，直接验证设备响应）

```powershell
# 使用串口调试工具（如 PuTTY 或串口助手）
# 参数: 9600-8-N-1, 无流控

# 发送 SCPI 查询
*IDN?
# 预期: DAG_DAQ_P1604,<serial>,<fw_version>

# 查询通道数
SYSTem:CHANnels?
# 预期: 16

# 查询扫描速率
SENSe:SCAN:RATE?
# 预期: 1000 (Hz)

# 设置单位
UNIT:PRESSure kPa
# 预期: OK
```

### 1.2 通过软件验证

**前置条件:**
- 在 UI 创建设备 Profile，Address 填入正确串口名（如 `COM3`）
- 确保 `8601` 设备类型选择 `DAQ-P-1604`

**验证步骤:**

| 步骤 | 操作 | 预期结果 | 实际 |
|------|------|---------|------|
| 1 | 点击「连接」 | 状态 `Connected` | [ ] |
| 2 | 点击「开始采集」 | 状态 `Acquiring` | [ ] |
| 3 | 等待 10s | DeviceDetailPanel 通道值持续刷新 | [ ] |
| 4 | 查看后端日志 | 无 `read error` / `frame parse error` 日志 | [ ] |
| 5 | 点击「停止」 | 状态 `Connected` | [ ] |
| 6 | 点击「断开」 | 状态 `Disconnected` | [ ] |
| 7 | 断开串口线后点连接 | 显示 "open serial: ... 找不到指定文件" | [ ] |
| 8 | 重新连接后连续采集 30 分钟 | 无数据断流 | [ ] |

### 1.3 Adapter 错误计数器

当发生串口读取失败或帧解析错误时，后端会记录：
```json
{"level":"DEBUG","msg":"DAQ-P-1604 read error","device":"daq-1604-1","error":"timeout"}
{"level":"DEBUG","msg":"DAQ-P-1604 frame parse error","device":"daq-1604-1","n":12,"error":"frame too short"}
```

这些日志位于 `slog` 输出中。正常采集时不应出现 `read error` 或 `frame parse error`。

---

## Phase 2: DAQ-T-1603 HIL

### 2.1 手动串口验证

```powershell
# 参数: 115200-8-N-1, 无流控

# 查询身份
*IDN?
# 预期: DAG_DAQ_T1603,<serial>,<fw_version>

# 设置热电偶类型
SENSe:TEMPerature:TC:TYPE K
# 预期: OK

# 查询冷端补偿
SENSe:TEMPerature:CJUNction:STATe?
# 预期: ON 或 OFF

# 设置滤波
SENSe:VOLTage:LPASs:FREQuency 10
# 预期: OK

# 读取通道值
READ:CHANnel1?
# 预期: <温度值>
```

### 2.2 通过软件验证

| 步骤 | 操作 | 预期结果 | 实际 |
|------|------|---------|------|
| 1 | 创建 DAQ-T-1603 Profile, Address=COMx | Profile 保存成功 | [ ] |
| 2 | 打开 DaqT1603Config，设置 Type=K, CJ=ON, Filter=10Hz | "保存"返回成功 | [ ] |
| 3 | 点击「连接」 | 状态 `Connected` | [ ] |
| 4 | 点击「开始采集」 | 状态 `Acquiring` | [ ] |
| 5 | 等待 10s | 温度通道持续刷新 | [ ] |
| 6 | 用热电偶接触热源 | 对应通道值上升 | [ ] |
| 7 | 断开热电偶（模拟断偶） | UI 显示错误/NaN 提示 | [ ] |
| 8 | 点击「停止」→「断开」 | 正常关闭 | [ ] |
| 9 | 重启后端，查看已保存的 DAQ-T-1603 配置是否恢复 | 配置与上次一致 | [ ] |

---

## Phase 3: 设备扫描 HIL

### 3.1 UDP 网络扫描（支持设备）

```powershell
# 设置环境变量启用网络扫描
$env:WINDLABX4_NETWORK_SCAN = "true"
go run .\cmd\server\main.go

# 测试扫描 API
curl http://localhost:8080/api/device/scan
```

**预期:** 返回局域网内响应的 DAQ 设备列表。

**特殊说明:**
- DAQ-P-1604 需回复 `DAQP1604` 到 UDP 30303 端口
- DAQ-T-1603 需回复 `DAQT1603` 到 UDP 30303 端口
- 扫描 3s 超时，无设备返回空数组

### 3.2 手动串口扫描

UI 创建 Profile 时手动输入 `Address`（串口名），不需要网络扫描也可连接。

---

## Phase 4: 串口参数参考

| 参数 | DAQ-P-1604 | DAQ-T-1603 | B140 |
|------|-----------|-----------|------|
| Baud Rate | 9600 | 115200 | 9600 |
| Data Bits | 8 | 8 | 8 |
| Parity | None | None | None |
| Stop Bits | 1 | 1 | 1 |
| Flow Control | None | None | None |
| Read Timeout | 3s | 3s | 3s (软件设) |

---

## Phase 5: 失败处理验证

| 场景 | 预期行为 | 验证方法 |
|------|---------|---------|
| 设备未上电时连接 | 返回 "open serial: 系统找不到指定的文件" | 断开设备电源，点连接 |
| 采集中断开串口线 | 连续 read error 日志，不 panic | 采集中拔掉串口线 |
| 恢复串口线 | readFrame 自动恢复 | 插回串口线 |
| 扫描无设备 | 返回空数组 `[]` | 关闭所有 DAQ 设备 |
| 配置非法值 | API 返回 400 + error message | 发送空 pressurePoints |

---

## 日志收集

```powershell
# 后端日志
go run .\cmd\server\main.go 2>&1 | Tee-Object -FilePath "hil-$(Get-Date -Format yyyyMMdd-HHmmss).log"

# 查看 DAQ 相关日志
# Windows PowerShell
Get-Content .\hil-*.log | Select-String -Pattern "DAQ|read error|frame parse|serial"
```

## 通过标准

1. Phase 1-2 所有步骤标记为 `[x]`
2. 连续采集 30 分钟无 `read error` / `frame parse error` 日志
3. 断线/超时/断偶场景 UI 显示非空白错误提示（非 NaN 或 0）
4. 串口参数与上表一致
