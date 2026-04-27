# Wind-DAQ 开发现状与规划

## 项目概述

风洞数据采集系统，六边形架构（Go 后端 + Vue 3 前端 via Wails），支持多设备数据采集、运动控制、自动校准和曲面扫描。

## 现状（已完成的）

### 后端 (Go) — ✅ 完成

| 层 | 路径 | 状态 |
|---|---|---|
| **core** — 业务核心模型 | `services/api-go/internal/core/` | ✅ 设备、运动、校准、扫描、插值类型已定义 |
| **ports** — 接口抽象 | `services/api-go/internal/ports/` | ✅ Device、MotionController、DataSink、ConfigRepo 等接口 |
| **usecase** — 业务编排 | `services/api-go/internal/usecase/` | ✅ DeviceManager、AcquisitionHub、MotionManager、CalibrationService、TraversalService、StorageService |
| **adapters/hardware** — 硬件驱动 | `services/api-go/internal/adapters/hardware/` | ✅ 模拟设备、P1604、T1603、模拟运动控制器、命令协议 |
| **adapters/config** — 配置存储 | `services/api-go/internal/adapters/config/` | ✅ JSON 配置管理器 |
| **adapters/ws** — WebSocket 推送 | `services/api-go/internal/adapters/ws/` | ✅ Hub、Client、频道定义 |
| **adapters/scan** — 设备扫描 | `services/api-go/internal/adapters/scan/` | ✅ UDP 广播扫描、TCP 扫描 |
| **adapters/storage** — CSV 存储 | `services/api-go/internal/adapters/storage/` | ✅ 数据文件写入 |
| **adapters/report** — 报告生成 | `services/api-go/internal/adapters/report/` | ✅ Markdown/PDF 报告 |
| **api** — HTTP handler | `services/api-go/api/handler/` | ✅ 35 个 REST 端点 + WebSocket |
| **cmd** — 入口 | `services/api-go/cmd/server/` | ✅ 主程序，依赖注入，优雅关闭 |

### 契约 — ✅ 完成

| 文件 | 说明 |
|---|---|
| `contracts/openapi/openapi.yaml` | OpenAPI 3.0 规范（35 端点 + 38 schema + WebSocket 频道） |
| `docs/STRUCTURE.md` | 项目结构文档（已与实际代码同步） |

### API 端点一览（35 个）

| 分组 | 端点数 |
|---|---|
| 应用 | 1 |
| 设备管理 | 12 |
| 设备扫描 | 1 |
| DAQ 采集 | 4 |
| 运动控制 | 12 |
| 校准 | 7 |
| 曲面扫描 | 5 |
| 数据存储 | 6 |
| 报告 | 3 |

### WebSocket 频道（8 个）

`daq:data-snapshot`、`device:status-updated`、`motion:status-updated`、`calibration:progress`、`calibration:complete`、`calibration:realtime`、`traversal:onProgress`、`traversal:onComplete`、`traversal:onError`

## 架构决策

```
┌─────────────────────────────────────────────────────┐
│  Wails Desktop App                                  │
│  ┌──────────────────────────────────────────────┐   │
│  │  Vue 3 Frontend (embedded in Wails WebView)  │   │
│  │  ←→ REST API + WebSocket ←→                  │   │
│  └──────────────────────────────────────────────┘   │
│                      ↕ HTTP/WS (localhost:8080)     │
│  ┌──────────────────────────────────────────────┐   │
│  │  Go Backend (Gin HTTP Server, 独立运行)      │   │
│  │  services/api-go/cmd/server/main.go          │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

- 前后端通过 **REST + WebSocket** 通信（非 Wails Bindings）
- Wails 仅作为 Vue 前端容器，后端独立运行
- 六边形架构确保硬件可替换、业务逻辑可测试

## 待开发（前端）

### 阶段 1：Wails 项目初始化

- [ ] 使用 `wails init` 初始化 `apps/desktop-wails/` 骨架
- [ ] 配置 `apps/desktop-wails/backend/` — Wails Go 绑定入口
- [ ] 配置 `apps/desktop-wails/frontend/` — Vue 3 + TypeScript 项目
- [ ] 配置开发脚本：启动后端 + 前端热重载

### 阶段 2：前端基础设施

- [ ] Vue 3 项目：Vite + TypeScript + Pinia + Vue Router
- [ ] API 客户端层（基于 OpenAPI 生成或手写 axios/fetch 封装）
- [ ] WebSocket 连接管理（自动重连、频道订阅）
- [ ] 深色主题设计系统
- [ ] 中文本地化

### 阶段 3：页面实现

| 页面 | 说明 | 优先级 |
|---|---|---|
| **Live Acquisition** | 实时采集仪表盘 — KPI 卡片 + 波形图 + 通道矩阵 + 事件流 | P0 |
| **Channel Matrix** | 通道详情配置：启用/禁用、量程、单位、精度 | P0 |
| **Trigger and Buffer** | 触发模式配置：边沿触发、预触发、后触发、缓冲区设置 | P1 |
| **Calibration** | 校准工作流：选择探针类型、配置点位、启动/监控/保存 | P0 |
| **Alarm Center** | 报警规则配置 + 报警历史 | P1 |
| **Data Replay** | 历史数据回放 | P2 |
| **Device Manager** | 设备添加/编辑/删除、连接管理 | P0 |
| **Motion Control** | 运动控制器配置、手动操控面板 | P0 |
| **Storage Settings** | 存储路径、文件格式、自动保存策略 | P1 |
| **Report View** | 报告生成与预览 | P1 |
| **Settings** | 系统设置：服务器端口、语言、开机自启 | P2 |

### 阶段 4：集成与发布

- [ ] Wails build 配置（Windows 桌面安装包）
- [ ] 后端嵌入 Wails 的启动逻辑（或独立进程管理）
- [ ] 自动更新机制
- [ ] 安装程序制作

## 开发命令

```powershell
# 后端
cd services/api-go
go run ./cmd/server/main.go               # 启动 API 服务
go build -buildvcs=false ./...            # 编译检查
gofmt -l .                                # 格式化检查

# 结构验证
powershell -File ../../scripts/validate-structure.ps1
```

## 参考文件

| 文件 | 用途 |
|---|---|
| `docs/STRUCTURE.md` | 项目目录结构 & 六边形架构详解 |
| `contracts/openapi/openapi.yaml` | OpenAPI 3.0 前后端契约 |
| `services/api-go/api/server.go` | 路由注册（35 端点） |
| `services/api-go/internal/adapters/ws/channels.go` | WebSocket 频道定义 |
