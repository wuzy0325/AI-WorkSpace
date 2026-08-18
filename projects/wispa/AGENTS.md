# WISPA 压力采集桌面应用

## 概述

WISPA 是基于 Wails 的桌面应用，用于 WISPA 压力采集设备的数据采集、实时显示和录制。

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
$env:WISPA_MODE="simulated"
go run github.com/wailsapp/wails/v3/cmd/wails3 dev

# 真实设备模式
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

## 对外交付打包

```powershell
cd projects/wispa/apps/desktop-wails
$env:GOWORK="off"
go run github.com/wailsapp/wails/v3/cmd/wails3 build   # wails3 build 内部自动使用 -tags production
```

**必须设置 `GOWORK=off`** 以隔离工作空间中其他模块的 wails/v2 间接引用，
否则构建的 exe 可能运行时报"correct build tags"错误。
生产构建规则详见 `../../docs/decisions/ADR-004-wails-v3-production-build.md`。

## Win7 LTS 分支构建（lts/win7）

Win7 兼容版本采用 **Go 1.20.14 + Electron 22.3.27 + net/http** 替代 Wails v3 + WebView2，
业务层（core/ports/usecase/adapters）零改动，仅替换最外层传输壳。详见 `../../docs/runbooks/win7-migration-guide.md`。

```powershell
# 一次性构建 Go 后端 + 前端 dist + 复制 exe 到 desktop-electron/backend/
cd projects/wispa/apps/desktop-electron
npm install
npm run build:backend

# 打包 NSIS 安装包（产物 dist/WISPA-Win7-Setup-0.3.0-win7.1-x64.exe）
npm run dist:win7
```

**关键约束**：
- Go 工具链必须是 `C:\go-versions\go1.20.14`（最后支持 Win7 的 Go 版本），build-backend.ps1 已硬编码路径
- `GOWORK=off` 必须设置（由 build-backend.ps1 自动注入）
- `CGO_ENABLED=0` 纯 Go 静态链接，避免依赖 mingw
- 监听端口 **18182**（与 wista 的 18181 区分，避免同机双开冲突）
- `frontend/dist/` 是 Vite 构建产物，被 Go `//go:embed all:frontend/dist` 嵌入到 exe

**与主线分支的差异**：
- `apps/desktop-wails/main.go` 改为 net/http server（移除 Wails 依赖）
- `apps/desktop-wails/backend/app.go` 改为 hub 模式（移除 `application.App` 依赖）
- 新增 `apps/desktop-wails/httpserver/` 包（HTTP handler + WebSocket hub）
- 新增 `apps/desktop-wails/core/eventbus.go` + `core/hub.go`（EventBus 抽象 + 状态容器）
- 前端 `bridge/` 改为 fetch + WebSocket（移除 `@wailsio/runtime` 依赖）
- 新增 `apps/desktop-electron/` 目录（Electron 主进程 + preload + 打包配置）

## 硬件约束

| 位置 | 约束 |
|------|------|
| `core/` | 零硬件导入、零文件 I/O、零框架导入 |
| `ports/` | 零实现 — 仅接口定义 |
| `usecase/` | 零直接硬件调用 — 通过 ports 接口 |
| `backend/` | 零业务逻辑 — 参数转换 + usecase 调用 |
| `frontend/` | 零直接硬件访问、零采集算法 |
