# Cal1604 校准系统

面向压力计量与标定流程的桌面系统，将遗留的 1605MeassureApp（计量）与 1604 标定软件合并为统一的 Go 后端 + Vue 3 前端 Web/Desktop 应用。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.25 + Wails v2（桌面壳）+ net/http（独立 HTTP 服务） |
| 前端 | Vue 3 + TypeScript + Vite + Element Plus + Pinia |
| 通信 | HTTP API + SSE（事件流） |
| 测试 | Go testing + Vitest + Playwright（E2E） |
| 构建 | Makefile + npm scripts + wails CLI |

## 目录结构

```
1604Cal/
├── main.go / app.go            # Wails 桌面应用入口
├── cmd/server/                 # 独立 HTTP 服务器入口（不依赖 Wails）
├── internal/                   # Go 后端（六边形架构）
│   ├── api/                    # HTTP 路由与 DTO
│   ├── application/            # 业务用例（calibration/measurement/multipress/session/deviceconnect）
│   ├── config/                 # 配置加载与默认值
│   ├── device/                 # 设备管理与驱动
│   ├── domain/                 # 领域模型
│   ├── errors/                 # 错误类型
│   ├── events/                 # 事件定义
│   ├── infrastructure/driver/  # 设备驱动适配
│   ├── report/                 # 报告生成
│   └── workflow/               # 工作流协调
├── web/                        # Vue 3 前端
│   └── src/
│       ├── api/                # HTTP 客户端
│       ├── components/         # UI 组件（按业务域分组）
│       ├── composables/        # 组合式函数
│       ├── router/             # 路由
│       ├── shared/             # 共享常量与事件
│       ├── stores/             # Pinia 状态管理
│       └── views/              # 页面视图
├── configs/                    # 应用配置示例
├── templates/reports/          # xlsx 报告模板（embed 嵌入）
├── build/                      # 图标 / 安装器 / manifest
├── docs/                       # ADR / 计划 / 业务说明 / 截图 / 演示
├── openspec/                   # OpenSpec spec-driven 变更管理
├── scripts/                    # 构建 / 检查脚本
│   └── debug/                  # 设备调试脚本（原 test/*.go）
├── e2e/                        # Playwright E2E 测试
├── Makefile                    # 顶层任务入口
├── wails.json                  # Wails 配置
├── start-dev.ps1 / start-dev.sh # 一键启动后端 + 前端
├── AGENTS.md / CLAUDE.md       # AI 代理执行规则
├── CONTEXT.md                  # 领域上下文（Ubiquitous Language）
├── PRODUCT.md                  # 产品定位与设计原则
├── DESIGN.md / DESIGN.json     # 设计规范（人读 / 机器可读令牌）
└── resourceinfo.json           # generate_icon.py --refresh-syso 依赖
```

## 快速开始

### 环境要求

- Go ≥ 1.25
- Node.js LTS
- Wails v2 CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### 开发模式

**一键启动（推荐）**：

```powershell
.\start-dev.ps1
```

启动后访问 http://localhost:5173（前端），后端 API 在 http://localhost:18080。

**单独启动**：

```powershell
# 后端（独立 HTTP 服务）
$env:GOWORK = "off"
go run ./cmd/server

# 前端
cd web
npm install
npm run dev
```

**Wails 桌面应用开发模式**：

```powershell
wails dev
```

### 构建

```powershell
# 桌面安装包
make desktop-build    # 等价于 wails build -clean

# 前端构建（main.go 通过 embed 嵌入 web/dist）
cd web
npm run build
```

## 质量门禁

```powershell
make check    # 等价于 pwsh ./scripts/check.ps1
```

执行内容：
- `go test ./cmd/... ./internal/...`
- `go vet ./cmd/... ./internal/...`
- `npm run typecheck`（vue-tsc）
- `npm run lint`（eslint）
- `npm run test`（vitest）

### 手动验证

```powershell
# Go
$env:GOWORK = "off"
go build ./internal/... ./cmd/...
go test ./internal/... ./cmd/...
go vet ./internal/... ./cmd/...

# 前端
cd web
npm run typecheck
npm run lint
npm run test
```

## 配置

应用配置示例见 [configs/app.example.json](configs/app.example.json)。复制为 `configs/app.json` 后修改即可生效。

关键配置项：
- `deviceConnect.connectMaxAttempts`：连接重试次数（默认 2，用于短时网络抖动容错）
- `deviceConnect.connectAttemptTimeoutMs`：单次连接超时（毫秒）

默认值定义见 [internal/application/deviceconnect/service.go](internal/application/deviceconnect/service.go) 的 `DefaultConfig()`。

## 关键文档

| 文档 | 说明 |
|---|---|
| [CONTEXT.md](CONTEXT.md) | 领域上下文与统一语言（Ubiquitous Language） |
| [PRODUCT.md](PRODUCT.md) | 产品定位、用户画像与设计原则 |
| [DESIGN.md](DESIGN.md) | UI 设计规范（颜色 / 字体 / 组件 / Dos & Don'ts） |
| [AGENTS.md](AGENTS.md) | AI 代理执行规则 |
| [CLAUDE.md](CLAUDE.md) | Claude Code 项目指南 |
| [docs/](docs/) | ADR、计划与业务说明 |
| [openspec/](openspec/) | OpenSpec spec-driven 变更管理 |

## 业务模块

- **标定工作台**：会话状态机驱动的流程控制，自动/手动动作与过程状态可视化
- **计量工作台**：数据采集与压力检测，适合作业准备、状态巡检与异常定位
- **设备管理**：集中维护计量设备与打压设备，单位一致性检查与连接异常追踪
- **多设备打压**：同时控制多台打压设备，实时压力监控与稳定检测

## 测试

- **单元测试**：`go test` + `npm run test`
- **E2E 测试**：`e2e/` 目录下的 Playwright 脚本

```powershell
cd e2e
npm install
npx playwright test
```
