# DAQ-P-1604 压力采集桌面应用

## 概述

DAQ-P-1604 是基于 Wails 的桌面应用，用于 DAQ-P-1604 压力采集设备的数据采集、实时显示和录制。

## 架构

采用单模块 Wails 布局，遵循六边形架构：

```
apps/desktop-wails/
├── core/          # 领域类型（零外部依赖）
├── ports/         # 接口定义（零实现）
├── usecase/       # 业务逻辑（通过 ports 调用）
├── adapters/      # 适配器实现
│   ├── hardware/  # P1604 硬件驱动 + 模拟器
│   ├── config/    # JSON 配置持久化
│   ├── recording/ # CSV 录制
│   └── logging/   # 日志文件写入
├── backend/       # Wails 绑定层（参数转换 + usecase 调用）
└── frontend/      # Vue 3 前端
```

## 设备特性

- 18 通道：16 压力 + CH17 大气压力 + CH18 大气温度
- TCP 二进制协议（2 字节大端长度前缀）
- 连接后需发送 `w1601` 启用长度前缀模式
- 采集命令：`c 00`（配置）→ `c 05`（内容）→ `c 01`（启动）→ `c 02`（停止）
- 设备发现：UDP 广播 `psi9000` 到端口 7000，监听端口 7001
- 压力单位：psi / Pa / kPa / MPa / kgf/cm²

## 开发

```powershell
# 模拟模式
$env:DAQ_P1604_MODE="simulated"
go run github.com/wailsapp/wails/v3/cmd/wails3 dev

# 真实设备模式
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

## 对外交付打包

```powershell
cd projects/daq-p1604/apps/desktop-wails
go run github.com/wailsapp/wails/v3/cmd/wails3 build   # wails3 build 内部自动使用 -tags production
```

生产构建规则详见 `../../docs/decisions/ADR-004-wails-v3-production-build.md`。

## 硬件约束

| 位置 | 约束 |
|------|------|
| `core/` | 零硬件导入、零文件 I/O、零框架导入 |
| `ports/` | 零实现 — 仅接口定义 |
| `usecase/` | 零直接硬件调用 — 通过 ports 接口 |
| `backend/` | 零业务逻辑 — 参数转换 + usecase 调用 |
| `frontend/` | 零直接硬件访问、零采集算法 |
