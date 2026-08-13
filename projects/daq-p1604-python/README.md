# DAQ-P-1604 Python 客户端库

DAQ-P-1604 压力采集设备（16 压力通道 + 大气压 + 大气温度 = 18 通道）的 Python 客户端库，供客户集成使用。

## 特性

- TCP 连接 / 断开，自动启用长度前缀模式（`w1601`）
- 数据流采集启停（`c 00` → `c 05` → `c 01` → `c 02`）
- 18 通道数据回调（CH1~CH16 压力 + CH17 大气压 + CH18 大气温）
- 压力单位读写（psi / Pa / kPa / MPa / kgf/cm²）
- 轮询备用模式（`rFFFF0`）
- UDP 设备发现（`psi9000` 广播）

零第三方依赖，纯标准库，Python 3.8+ 即可运行。

## 安装

交付包目录结构：

```
daq-p1604-python/
├── daqp1604/          # 客户端库（导入用）
├── demo.py            # 交互式控制台 Demo
├── tests/             # 模拟设备 + 端到端测试
└── pyproject.toml     # 打包配置
```

安装库（任选其一）：

```powershell
pip install .          # 在交付包目录下执行，安装到当前 Python 环境
```

或把 `daqp1604/` 目录直接拷贝到你的项目里（无需安装）。

## 快速开始

```python
from daqp1604 import DAQP1604Device

dev = DAQP1604Device("192.168.3.101", 9000)

def on_data(payload):
    # payload = {
    #   deviceId, timestamp, seq,
    #   channels: [CH1..CH16 压力, CH17 大气压, CH18 大气温],
    #   channelIndices: [0..17],
    # }
    print(payload["channels"])

dev.on_data = on_data
dev.connect()                     # 自动 w1601 + 读取硬件单位
dev.start_acquisition(period_ms=100)
# ... 回调持续被调用 ...
dev.stop_acquisition()
dev.disconnect()
```

## API 摘要

| 方法 | 说明 |
|------|------|
| `connect()` | TCP 连接 + `w1601` 启用长度前缀 + 单位同步 |
| `disconnect()` | 停止采集并关闭连接 |
| `start_acquisition(period_ms=100)` | 启动数据流（命令链任一步失败即回滚停止） |
| `stop_acquisition()` | 停止数据流（`c 02`） |
| `send_command(cmd)` | 发送纯 ASCII 命令并等待响应 |
| `read_unit()` / `set_unit(unit)` | 读取 / 设置硬件压力单位 |
| `poll_all_channels()` | 轮询一次 16 通道压力（调试用） |
| `scan_devices(timeout=3)` | UDP 广播发现局域网设备 |

属性：`device.connected`、`device.acquiring`、`device.unit`。

数据回调 `on_data(payload)` 在后台读线程中被调用，不要在回调里做耗时操作。

## Demo

交互式控制台 Demo，覆盖连接 / 采集 / 停止 / 断开 / 退出：

```powershell
python demo.py [host] [port]
# 默认 host=192.168.3.101 port=9000
```

```
===== DAQ-P-1604 Demo @ 192.168.3.101:9000 [状态: 未连接] =====
  1. 连接
  2. 开始采集
  3. 停止采集
  4. 断开
  5. 扫描设备
  0. 退出
请选择:
```

选择 `2 开始采集` 后实时滚动显示 18 通道数据，按任意键停止采集返回菜单。

## 协议要点

- 所有命令以**纯 ASCII** 发送，**不带换行符**（否则设备返回 N05）
- 连接后必须首先发 `w1601` 启用 2 字节大端长度前缀模式
- 帧长度前缀包含自身 2 字节
- 二进制帧前 16 路按 CH16→CH1 逆序，库已自动反转
- 单位通过 `u01101`/`v01101` 读写 EU 系数，库已内置系数映射

## 测试

在交付包目录下执行：

```powershell
python tests\test_mock_server.py
```

用模拟设备（TCP 服务器）验证连接 / 采集 / 停止 / 单位 / 帧解析的完整链路，无需真实硬件。
