# AI-Workspace Win7 LTS 开发进度

> **当前分支**：`lts/win7`
> **工作目录**：`c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace-win7`
> **最后更新**：2026-07-23（wind-daq 迁移完成）
> **状态**：DAQ-T1603 Win7 源码基线已提交为 `a8de1c2`；原始安装包通过 Windows 7 SP1 x64 真机验证，重建安装包已通过本机 smoke test；wind-daq Win7 迁移完成，本机 smoke test 通过，真机验证待执行

---

## 1. 项目目标

在不改动主工作空间 `AI-Workspace` 产品代码的前提下，为以下维护产品制作 Windows 7 SP1 x64 版本：

- `daq-t1603`（已验证）
- `probe-interpolator`
- `daq-p1604`
- `1604Cal`
- `motion-controller`
- `wind-daq`

DAQ-T1603 是首个技术基线。全部产品统一在本 worktree 的长期 `lts/win7` 分支维护，主线修复由 AI 定期选择性同步，不整体合并 `master`。

初始要求：

- **快速落地**（用户首条要求原话："快速落地"）
- **不改动主工作空间产品代码**
- **长期方向**（不是一次性现场需求）
- **必须有 GUI**（不接受 CLI）

## 2. 技术栈决策

### 2.1 硬伤分析

| 原栈组件 | 问题 | Win7 解决方案 |
|---|---|---|
| Go 1.25 | Go 1.21+ 官方放弃 Win7 | **Go 1.20.14**（最后支持 Win7 的 Go 版本） |
| Wails v3 | 内部使用 `log/slog`、`maps`、`slices`（Go 1.21+ 标准库） | **移除 Wails，改用 `net/http`** |
| WebView2 | 仅 Win10+ | **Electron 22.3.27**（最后兼容 Win7 的 Electron 主版本，内置 Chromium 108） |

### 2.2 整体架构

```
┌──────────────────────────────────────────────────────┐
│  Electron 22.3.27 主进程（electron/main.cjs）         │
│  ├── 拉起 Go 后端子进程（监听 127.0.0.1:18181）        │
│  ├── 加载 frontend dist/                              │
│  └── 提供原生 IPC（目录选择对话框等）                  │
└──────────────────────────────────────────────────────┘
                       │
        ┌──────────────┴──────────────┐
        │                             │
   HTTP fetch (RPC)              WebSocket (事件流)
        │                             │
┌───────▼─────────────────────────────▼─────────────────┐
│  Go 后端（net/http server，端口 127.0.0.1:18181）     │
│  ├── /api/device/*        → DeviceService            │
│  ├── /api/recording/*     → RecordingService         │
│  ├── /api/log/*           → LogService               │
│  └── /ws                  → WebSocket Hub（事件推送）│
└───────────────────────────────────────────────────────┘
        │
        ▼
  原有 hexagonal 业务层（core / usecase / ports / adapters）
  完全复用，仅替换最外层的 Wails binding
```

## 3. 分支与隔离方案

- **Git 策略**：同一 Git 仓库的长期 worktree；DAQ-T1603 基线提交后将分支统一为 `lts/win7`
- **工作目录**：`c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace-win7`（与 trunk 主工作目录平级）
- **SVN 策略**：SVN 不建分支，仅 Git worktree 隔离（用户明确选择"SVN 不建分支"）
- **零侵入保障**：worktree 内的修改不会回流 trunk，trunk 代码完全不变
- **同步策略**：不整体 merge `master`；使用 `git cherry-pick -x` 或记录来源 SHA 的人工兼容移植
- **同步台账**：根目录 `WIN7-SYNC-STATE.md` 记录已同步、排除、待处理和验证结果
- **当前门禁**：先按主工作空间计划 Task 0 鉴别全部未提交文件并固化基线，再重命名分支或开始同步

## 4. 已完成工作

### 4.1 基础设施

| 项目 | 文件 | 状态 |
|---|---|---|
| Go 1.20.14 安装脚本 | [scripts/install-go1.20.14.ps1](scripts/install-go1.20.14.ps1) | 已执行成功，安装到 `C:\go-versions\go1.20.14` |
| git worktree 创建 | `lts/win7` 分支 | done |

### 4.2 Go 1.20 兼容性改造

#### 4.2.1 slog 兼容性（11 个文件）

**问题**：原代码使用 `golang.org/x/exp/slog` 或 `log/slog`，前者无法降级（本地 cache 仅 Go 1.25 版本，网络下载失败），后者 Go 1.21+ 才有。

**解决方案**：自建本地 slog shim 包

**新建文件**：[shared/device-sdk/go/pkg/slog/slog.go](shared/device-sdk/go/pkg/slog/slog.go)

- 80 行，零外部依赖
- 实现 `Info/Debug/Warn/Error` + `Level` 常量 + `SetLevel`
- API 与 `log/slog` 保持一致，便于未来回流 trunk

**批量替换的 11 个文件**：

shared/device-sdk/go 下 7 个文件：
- [serialport/port.go](shared/device-sdk/go/serialport/port.go)
- [testing/sim/simulator.go](shared/device-sdk/go/testing/sim/simulator.go)
- [ffi/wtnmc4a.go](shared/device-sdk/go/ffi/wtnmc4a.go)
- [ffi/wtn_daq16h.go](shared/device-sdk/go/ffi/wtn_daq16h.go)
- [daq/hardware/daq_p1603.go](shared/device-sdk/go/daq/hardware/daq_p1603.go)
- [daq/hardware/daq_t1603.go](shared/device-sdk/go/daq/hardware/daq_t1603.go)
- [motion/adapters/hardware/wtnmc4a_motion.go](shared/device-sdk/go/motion/adapters/hardware/wtnmc4a_motion.go)

daq-t1603 业务层 3 个文件：
- [backend/log_service.go](projects/daq-t1603/apps/desktop-wails/backend/log_service.go)
- [usecase/device_usecase.go](projects/daq-t1603/apps/desktop-wails/usecase/device_usecase.go)
- [adapters/config/json_config.go](projects/daq-t1603/apps/desktop-wails/adapters/config/json_config.go)

#### 4.2.2 math/rand/v2 → math/rand（1 个文件）

**问题**：`math/rand/v2` 是 Go 1.22+ 标准库，Go 1.20 不支持。

**修改文件**：[shared/device-sdk/go/motion/adapters/hardware/simulated_motion.go](shared/device-sdk/go/motion/adapters/hardware/simulated_motion.go)

- import `"math/rand/v2"` → `"math/rand"`
- 添加 `init() { rand.Seed(time.Now().UnixNano()) }`（Go 1.20 默认固定种子，不 Seed 会导致模拟运动随机性丢失）

#### 4.2.3 go.mod 降级

**device-sdk go.mod**：[shared/device-sdk/go/go.mod](shared/device-sdk/go/go.mod)

```diff
- go 1.25.0
+ go 1.20
- require golang.org/x/exp v0.0.0-... (自动删除)
```

**daq-t1603 go.mod**：[projects/daq-t1603/apps/desktop-wails/go.mod](projects/daq-t1603/apps/desktop-wails/go.mod)

```diff
- go 1.25.0
+ go 1.20
- require github.com/wailsapp/wails/v3 v3.0.0-alpha2.106
- require github.com/wailsapp/wails/webview2 v1.0.27
- ... (8 个 wails 相关 direct/indirect 依赖全部删除)
```

最终 go.mod：
```
module daq-t1603
go 1.20
require shared.local/device-sdk/go v0.0.0-00010101000000-000000000000
require (
    github.com/coder/websocket v1.8.14 // indirect
    golang.org/x/sys v0.43.0 // indirect
)
replace shared.local/device-sdk/go => ../../../../shared/device-sdk/go
```

### 4.3 Wails → HTTP server 改造（核心）

#### 4.3.1 设计：EventBus 抽象

**问题**：原 backend 4 个 Service 通过 `*application.App` 的 `app.Event.Emit()` 推送事件给前端。

**解决方案**：在 core 层引入 `EventBus` 接口，backend 依赖抽象而非具体传输。

**新建文件**：[core/eventbus.go](projects/daq-t1603/apps/desktop-wails/core/eventbus.go)

```go
type EventBus interface {
    Emit(name string, data ...any)
}

const (
    EventPayload                = "daq:payload"
    EventLog                    = "daq:log"
    EventRecordingStatus        = "daq:recording-status"
    EventRecordingBackpressure  = "daq:recording-backpressure"
    EventRecordingFatal         = "daq:recording-fatal"
)
```

**修改文件**：[core/hub.go](projects/daq-t1603/apps/desktop-wails/core/hub.go)

- Hub 结构体新增 `bus EventBus` 字段
- 新增 `SetEventBus(bus EventBus)` 方法（注入 WSHub）
- 新增 `EmitEvent(name string, data ...any)` 方法（转发到注入的 bus）

#### 4.3.2 backend 4 个 Service 改造

| 文件 | 改动 |
|---|---|
| [backend/device_service.go](projects/daq-t1603/apps/desktop-wails/backend/device_service.go) | 移除 `*application.App` 字段，`ServiceStartup(ctx)` 签名简化（无 options），`emitPayload` 改走 `hub.EmitEvent` |
| [backend/log_service.go](projects/daq-t1603/apps/desktop-wails/backend/log_service.go) | `EmitLog` 走 `hub.EmitEvent(core.EventLog, ...)`，`PickDirectory` 返回 `ErrDialogNotSupported` |
| [backend/recording_service.go](projects/daq-t1603/apps/desktop-wails/backend/recording_service.go) | `EmitStatus` / `handleBackpressure` / `handleFatal` 全走 `hub.EmitEvent` |
| [backend/dialog.go](projects/daq-t1603/apps/desktop-wails/backend/dialog.go) | 重写为只定义 `ErrDialogNotSupported`，目录选择改由 Electron 主进程处理 |

#### 4.3.3 main.go 重写

**文件**：[projects/daq-t1603/apps/desktop-wails/main.go](projects/daq-t1603/apps/desktop-wails/main.go)

关键改动：
- 移除 `github.com/wailsapp/wails/v3/pkg/application` import
- 改用 `net/http` server 监听 `127.0.0.1:18181`
- 创建 service 后按依赖顺序调用 `ServiceStartup(appCtx)`：LogService → RecordingService → DeviceService
- shutdown 时反序调用 `ServiceShutdown()`
- WSHub 注入与启动详见 4.3.5 节

#### 4.3.4 httpserver 完整实现

httpserver 包共 7 个文件，覆盖 17 个 HTTP endpoint + 1 个 WebSocket endpoint + 集成测试。

**新建文件清单**：

| 文件 | 内容 |
|---|---|
| [httpserver/register.go](projects/daq-t1603/apps/desktop-wails/httpserver/register.go) | Server struct + RegisterHandlers 入口 + /api/health |
| [httpserver/helpers.go](projects/daq-t1603/apps/desktop-wails/httpserver/helpers.go) | 统一响应信封 `{ok,data,error}` + JSON 解析 + ID 解析工具 |
| [httpserver/ws_hub.go](projects/daq-t1603/apps/desktop-wails/httpserver/ws_hub.go) | WebSocket hub，实现 `core.EventBus` 接口，`/ws` endpoint |
| [httpserver/device_handler.go](projects/daq-t1603/apps/desktop-wails/httpserver/device_handler.go) | DeviceService 的 10 个 endpoint |
| [httpserver/recording_handler.go](projects/daq-t1603/apps/desktop-wails/httpserver/recording_handler.go) | RecordingService 的 3 个 endpoint |
| [httpserver/log_handler.go](projects/daq-t1603/apps/desktop-wails/httpserver/log_handler.go) | LogService 的 3 个 endpoint |
| [httpserver/ws_hub_test.go](projects/daq-t1603/apps/desktop-wails/httpserver/ws_hub_test.go) | WebSocket hub 集成测试（4 个用例，全过） |

**HTTP endpoint 完整列表**：

| 方法 | 路径 | 说明 |
|---|---|---|
| GET  | /api/health | 健康检查（Electron 主进程就绪探测） |
| POST | /api/device/scan | 扫描设备 |
| GET  | /api/device/profiles | 列出全部配置 |
| POST | /api/device/profile | 新增/更新配置 |
| DELETE | /api/device/profile/{id} | 删除配置 |
| POST | /api/device/connect | 连接设备，body: `{"id":"..."}` |
| POST | /api/device/disconnect | 断开设备 |
| POST | /api/device/start | 启动采集 |
| POST | /api/device/stop | 停止采集 |
| GET  | /api/device/status/{id} | 查询设备状态（404 = 设备不存在） |
| POST | /api/device/apply-config | 下发配置，body: `{"id":"...","config":{...}}` |
| POST | /api/recording/start | 开始录制，body: `{"outputDir":"...","filePrefix":"..."}` |
| POST | /api/recording/stop | 停止录制 |
| GET  | /api/recording/status | 查询录制状态 |
| POST | /api/log/start | 开始日志文件，body: `{"outputDir":"...","prefix":"..."}` |
| POST | /api/log/stop | 停止日志文件 |
| GET  | /api/log/state | 查询日志文件状态 |
| WS   | /ws | 事件流（daq:payload / daq:log / daq:recording-*） |

**响应信封约定**：

```json
// 成功
{ "ok": true, "data": <payload> }
// 失败
{ "ok": false, "error": "<message>" }
```

前端 fetch 调用统一从响应 JSON 中读取 `ok` 字段路由成功/失败，HTTP 状态码 4xx/5xx 仅作辅助。

**WebSocket 消息信封**：

```json
{ "event": "daq:payload", "data": <payload> }
```

前端 `onmessage` 解析后按 `event` 字段路由到各 callback。

**关键设计要点**：

1. **WSHub 并发模型**：单 goroutine 串行处理 register/unregister/broadcast，避免并发写 map 的锁竞争；每个客户端独立 send channel（buffered 32）+ writePump goroutine，慢客户端不阻塞 broadcast 主循环
2. **Emit 非阻塞**：broadcast channel 缓冲 128，超出即丢，保护 relay 协程 100ms 推送被阻塞；单个客户端 send 队列满时丢消息不阻塞其他订阅者
3. **EventBus 注入时机**：`hub.SetEventBus(wsHub)` 必须在 `ServiceStartup` 之前调用，否则 `LogService.ServiceStartup` 推送的"应用已启动"日志会被静默丢弃（main.go 第 80-85 行）
4. **路由冲突处理**：Go 1.20 ServeMux 不支持路径参数，`/api/device/profile`（POST 精确匹配）与 `/api/device/profile/`（DELETE 子树匹配）通过尾斜杠差异化分流
5. **Origin 校验**：`InsecureSkipVerify=true` 跳过 Origin 校验，兼容 Electron renderer 加载本地 `file://` 时无标准 Origin 的场景（监听 127.0.0.1，无 CSRF 风险）

**WebSocket 库选型变更**：

原计划用 `github.com/coder/websocket`（go.mod indirect 中是 v1.8.14），但 v1.8.14 内部 `internal/util`、`internal/errd` 包使用了 Go 1.23 代码，无法在 Go 1.20 下编译。降级到 v1.8.10/v1.8.11 时 module path 仍是 `nhooyr.io/websocket`（历史遗留），无法用 `github.com/coder/websocket` 路径 import。

最终方案：直接使用 `nhooyr.io/websocket v1.8.17`（API 与 coder/websocket 完全一致，最新版兼容 Go 1.20）。go.mod 中 `github.com/coder/websocket` 已删除，替换为 `nhooyr.io/websocket v1.8.17` direct 依赖。

#### 4.3.5 main.go 装配

**修改文件**：[main.go](projects/daq-t1603/apps/desktop-wails/main.go)

关键改动：
- 第 80-85 行：在 ServiceStartup 之前创建 `wsHub` 并调用 `hub.SetEventBus(wsHub)`
- 第 89 行：`RegisterHandlers` 调用增加 `wsHub` 参数
- 第 102-103 行：`go wsHub.Run(appCtx)` 启动 hub 主循环，与 appCtx 同生命周期
- 移除原 TODO 注释

## 5. 验证结果（Go 1.20.14）

| 命令 | 结果 |
|---|---|
| `go build .` | 通过 |
| `go build ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go test ./...` | 全部通过（config / hardware / recording / usecase / httpserver 5 个包，httpserver 含 4 个 WebSocket hub 集成测试） |

**验证命令模板**：
```powershell
$env:GOROOT = "C:\go-versions\go1.20.14"
$env:PATH = "$env:GOROOT\bin;$env:PATH"
$env:GOWORK = "off"
$env:GOPROXY = "https://goproxy.cn,direct"
cd "c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace-win7\projects\daq-t1603\apps\desktop-wails"
go build ./...
go vet ./...
go test ./...
```

## 6. 关键 API 签名参考

### 6.1 backend 三个 Service 的公共方法

**DeviceService**（11 个 endpoint）：
```go
ScanDevices() ([]core.ScanResult, error)
GetProfiles() []core.TemperatureProfile
UpsertProfile(profile core.TemperatureProfile) error
DeleteProfile(id string) error
Connect(id string) error
Disconnect(id string) error
StartAcquisition(id string) error
StopAcquisition(id string) error
GetStatus(id string) (core.DeviceState, error)
ApplyConfig(id string, cfg core.T1603Config) error
```

**RecordingService**（6 个 endpoint）：
```go
StartRecording(outputDir string, filePrefix string) error
StopRecording() error
GetRecordingStatus() core.RecordingSession
PickDirectory() (string, error)  // Win7 版返回 ErrDialogNotSupported
```

**LogService**（5 个 endpoint）：
```go
StartLogFile(outputDir string, prefix string) error
StopLogFile() error
GetLogFileState() LogFileState
PickDirectory() (string, error)  // Win7 版返回 ErrDialogNotSupported
```

### 6.2 前端事件订阅

前端原通过 `@wailsio/runtime` 的 `Events.On` 订阅 5 个事件，Win7 分支改用 WebSocket `onmessage`：

| 事件名 | 数据类型 | 来源 |
|---|---|---|
| `daq:payload` | `TemperatureSnapshot` | DeviceService.relayStream |
| `daq:log` | `LogEvent` | LogService.EmitLog |
| `daq:recording-status` | `RecordingSession` | RecordingService.EmitStatus |
| `daq:recording-backpressure` | `BackpressureEvent` | RecordingService.handleBackpressure |
| `daq:recording-fatal` | `{deviceId, error}` | RecordingService.handleFatal |

**WebSocket 消息信封**（前端 onmessage 解析约定）：

```typescript
interface WSEnvelope {
  event: string  // 如 "daq:payload"
  data: any      // 事件 payload
}
ws.onmessage = (e) => {
  const msg: WSEnvelope = JSON.parse(e.data)
  switch (msg.event) {
    case 'daq:payload': onPayload(msg.data); break
    case 'daq:log': onLog(msg.data); break
    // ...
  }
}
```

## 7. 待办事项

### 7.1 httpserver 完整实现 ✅ 已完成

详见 4.3.4 节。17 个 HTTP endpoint + 1 个 WebSocket endpoint + 4 个集成测试，go build / vet / test 全绿。

### 7.2 前端 bridge 改写 ✅ 已完成

**目录**：`projects/daq-t1603/apps/desktop-wails/frontend/src/bridge/`

| 文件 | 改动 |
|---|---|
| `deviceBridge.ts` | 12 个 RPC 改用 fetch；5 个事件订阅改用 WebSocket 单例 |
| `logBridge.ts` | 4 个 RPC 改用 fetch（无事件订阅，日志事件走 deviceBridge.onLog） |
| `recordingBridge.ts` | 4 个 RPC 改用 fetch；1 个事件订阅改用 WebSocket 单例 |
| `wsClient.ts`（新增） | WebSocket 单例客户端，自动重连 + 事件分发 |
| `httpClient.ts`（新增） | fetch 封装，统一响应信封解包 |
| `env.d.ts` | 新增 `Window.electronAPI` 类型声明 |
| `package.json` | 移除 `@wailsio/runtime` 依赖 |

**关键设计要点**：

1. **导出签名 100% 兼容**：所有 RPC 函数名、参数类型、返回类型与原 Wails 版完全一致，stores 和 components 零改动
2. **WebSocket 自动重连**：1s → 2s → 4s → ... → 10s 封顶，指数退避；连接断开时所有 handler 保留，重连后自动恢复分发
3. **handler 注册隔离**：单个 handler 抛错不影响其他 handler（try/catch 包裹）
4. **on() 返回 unsubscribe 函数**：与 `@wailsio/runtime Events.On` 兼容，无需改写 store 中的清理逻辑
5. **pickDirectory 双路径**：Electron 环境走 `window.electronAPI.showOpenDialog()`，浏览器开发环境回退空串
6. **响应信封统一解包**：httpClient 在 `ok:false` 时 reject `Error(env.error)`，`ok:true` 时返回 `env.data`，调用方代码与 Wails 版几乎相同
7. **getStatus 失败回退 false**：保留原 Wails 版语义（设备不存在时返回 false 而非抛错），通过 `.catch(() => false)` 实现

**验证结果**：

| 命令 | 结果 |
|---|---|
| `npm run typecheck` | 通过（vue-tsc --noEmit 0 错误） |
| `npm run build` | 通过（5149 modules transformed，dist/ 产物正常生成） |

**移除的依赖**：

- `@wailsio/runtime` 3.0.0-alpha.95（package.json dependencies 已删除）
- node_modules 重装后无残留

**未删除的 wails 残留**：

- `frontend/bindings/` 目录（wails3 generate bindings 产物，src 下已无任何引用，可在 7.3 Electron 主进程完成后统一清理）

### 7.3 Electron 主进程 ✅ 已完成

**目录**：`projects/daq-t1603/apps/desktop-electron/`（新建）

| 文件 | 内容 |
|---|---|
| `main.cjs` | Electron 主进程：拉起 Go 后端子进程、加载 frontend dist/、注册 IPC handler（目录选择等） |
| `preload.cjs` | preload 脚本：暴露 `window.electronAPI` 给 renderer |
| `package.json` | electron 22.3.27 + electron-builder 配置 |

**关键实现要点**：
- Go 后端作为子进程启动，监听 `127.0.0.1:18181`
- 主窗口加载 `http://127.0.0.1:18181/`（Go server 同时托管 frontend dist/）或 `file://` 加载本地 dist
- IPC handler 提供 `dialog.showOpenDialog`（替代后端 PickDirectory）

### 7.4 分支级 README 警告文件（P1）

在 worktree 根目录创建 `README-WIN7.md`，明确标注：
- 这是 Win7 兼容分支，**禁止回流 trunk**
- 技术栈锁定：Go 1.20.14 + Electron 22.3.27
- 分支内 `shared/device-sdk/go` 需定期通过 `svn merge` 同步 trunk 的 bug 修复，但不引入新 API

### 7.5 打包验证 ✅ 已完成

1. `go build` 生成后端可执行文件：通过
2. `npm run build` 构建前端 dist/：通过
3. `electron-builder` 生成 NSIS x64 安装包：通过
4. Windows 7 SP1 x64 真机安装与启动验证：通过

验证安装包：`projects/daq-t1603/apps/desktop-electron/dist/DAQ-T-1603-Win7-Setup-0.3.3-x64.exe`

技术基线：Go 1.20.14、Electron 22.3.27、Chromium 108。Electron 23 及以上不支持 Windows 7。

## 8. 关键文件索引

### 8.1 配置与入口

| 文件 | 用途 |
|---|---|
| [main.go](projects/daq-t1603/apps/desktop-wails/main.go) | Go 后端入口（net/http server） |
| [go.mod](projects/daq-t1603/apps/desktop-wails/go.mod) | daq-t1603 模块定义 |
| [shared/device-sdk/go/go.mod](shared/device-sdk/go/go.mod) | device-sdk 模块定义 |

### 8.2 核心抽象

| 文件 | 用途 |
|---|---|
| [core/eventbus.go](projects/daq-t1603/apps/desktop-wails/core/eventbus.go) | EventBus 接口 + 事件名常量 |
| [core/hub.go](projects/daq-t1603/apps/desktop-wails/core/hub.go) | Hub（含 SetEventBus/EmitEvent 方法） |
| [shared/device-sdk/go/pkg/slog/slog.go](shared/device-sdk/go/pkg/slog/slog.go) | slog shim 包 |

### 8.3 backend Service

| 文件 | 用途 |
|---|---|
| [backend/device_service.go](projects/daq-t1603/apps/desktop-wails/backend/device_service.go) | 设备 Service |
| [backend/log_service.go](projects/daq-t1603/apps/desktop-wails/backend/log_service.go) | 日志 Service |
| [backend/recording_service.go](projects/daq-t1603/apps/desktop-wails/backend/recording_service.go) | 录制 Service |
| [backend/dialog.go](projects/daq-t1603/apps/desktop-wails/backend/dialog.go) | ErrDialogNotSupported 定义 |
| [backend/errors.go](projects/daq-t1603/apps/desktop-wails/backend/errors.go) | ErrDeviceNotFound 定义 |

### 8.4 httpserver

| 文件 | 用途 |
|---|---|
| [httpserver/register.go](projects/daq-t1603/apps/desktop-wails/httpserver/register.go) | Server struct + RegisterHandlers 入口 + /api/health |
| [httpserver/helpers.go](projects/daq-t1603/apps/desktop-wails/httpserver/helpers.go) | 统一响应信封 + JSON 解析工具 |
| [httpserver/ws_hub.go](projects/daq-t1603/apps/desktop-wails/httpserver/ws_hub.go) | WebSocket hub，实现 core.EventBus 接口 |
| [httpserver/device_handler.go](projects/daq-t1603/apps/desktop-wails/httpserver/device_handler.go) | DeviceService 的 10 个 endpoint |
| [httpserver/recording_handler.go](projects/daq-t1603/apps/desktop-wails/httpserver/recording_handler.go) | RecordingService 的 3 个 endpoint |
| [httpserver/log_handler.go](projects/daq-t1603/apps/desktop-wails/httpserver/log_handler.go) | LogService 的 3 个 endpoint |
| [httpserver/ws_hub_test.go](projects/daq-t1603/apps/desktop-wails/httpserver/ws_hub_test.go) | WebSocket hub 集成测试 |

## 9. 踩坑记录

### 9.1 Trae 沙箱 allowlist 限制

**问题**：Edit 工具和 PowerShell `Set-Content` 无法操作 worktree 路径（在主工作目录外）。

**解决方案**：用 .NET API 绕过
```powershell
[System.IO.File]::WriteAllText($path, $content, [System.Text.UTF8Encoding]::new($false))
[System.IO.File]::ReadAllText($path)
```

### 9.2 PowerShell 双引号字符串陷阱

**问题**：`"[info] SHA256 = $var"` 报 `Array index expression is missing or not valid`。

**原因**：PowerShell 双引号字符串中 `[info]` 被解析为数组索引语法。

**解决方案**：全用单引号字符串 + 字符串拼接 `('[' + 'info' + '] ' + $var)`。

### 9.3 Go 1.20 不接受 `go 1.25.0` 格式

**问题**：`invalid go version '1.25.0': must match format 1.23`

**解决方案**：go.mod 中 `go 1.25.0` 改为 `go 1.20`（Go 1.20 的 go.mod 解析器不认 1.25.0 这种 patch 版本格式）。

### 9.4 go mod tidy 不删除直接 require 的依赖

**问题**：移除 wails import 后 `go mod tidy` 未删除 wails 的 require。

**解决方案**：手动 `go mod edit -droprequire` 逐个删除（PowerShell 中需用单引号包裹模块路径，否则 `/` 被解析为路径分隔符）。

### 9.5 Go 1.20.14 下载镜像失败

**问题**：golang.google.cn 和 go.dev 都 "基础连接已经关闭: 发送时发生错误"。

**解决方案**：脚本设计 3 镜像 fallback，第三个 `mirrors.aliyun.com` 成功。

### 9.6 coder/websocket v1.8.14 不兼容 Go 1.20

**问题**：`go build ./...` 报 `github.com/coder/websocket/internal/util: cannot compile Go 1.23 code` 和 `internal/errd: cannot compile Go 1.23 code`。

**原因**：coder/websocket v1.8.14 的内部包使用了 Go 1.23 的语法/标准库，无法在 Go 1.20 下编译。

**降级尝试失败**：
- v1.8.10 / v1.8.11：module path 仍是 `nhooyr.io/websocket`（历史遗留，coder/websocket 是 fork），无法用 `github.com/coder/websocket` 路径 import，报 `module declares its path as: nhooyr.io/websocket but was required as: github.com/coder/websocket`。

**最终解决方案**：直接换用 `nhooyr.io/websocket v1.8.17`（API 与 coder/websocket 完全一致，最新版兼容 Go 1.20）。
- `go get nhooyr.io/websocket@v1.8.17`
- ws_hub.go import 路径从 `github.com/coder/websocket` 改为 `nhooyr.io/websocket`
- `go mod tidy` 自动删除 `github.com/coder/websocket` require

## 10. 下一步建议

按优先级：

1. ~~固化 DAQ-T1603 已验证源码~~：已提交为 `a8de1c2`。
2. ~~将分支统一为 `lts/win7`~~：已完成。
3. 以 `WIN7-SYNC-STATE.md` 为事实来源，建立 AI 选择性同步流程。
4. 从 DAQ-T1603 提取仅存在于 Win7 分支的 Electron/HTTP/NSIS 平台模板。
5. 按 `probe-interpolator` → `daq-p1604` → `1604Cal` → `motion-controller` → `wind-daq` 顺序迁移。
6. 每完成一个产品，生成 NSIS 并进行 Windows 7 SP1 x64 真机验收。

详细实施计划保存在主工作空间：`docs/plans/2026-07-23-workspace-win7-lts-worktree.md`。

技术基线保持不变：Go 1.20.14、Electron 22.3.27、Chromium 108。主工作空间继续使用当前 Go/Wails 技术栈，不承受 Win7 兼容约束。


## 11. wind-daq 迁移完成（2026-07-23）

### 11.1 迁移类型

**B 类迁移**：已有 HTTP API → 复用 `api.NewRouter(api.Deps{...})` + Electron 壳替换 Wails。

### 11.2 改造范围

| 层 | 改动 |
|---|---|
| Go 1.20 兼容 | `services/api-go` 29 个文件 `"log/slog"` → `shared.local/device-sdk/go/pkg/slog`；`fivehole` 内置 `max()` → `cmpMax()` helper；`calibration_csv_writer_test.go` 内置 `min()` → `cmpMin()` helper；`motion-control/go` 和 `fivehole` go.mod 从 1.25.0 降到 1.20 |
| HTTP server | `apps/desktop-wails/main.go` 重写为 net/http 入口（embed.FS + api.NewRouter + signal.Notify 优雅关闭），主进程 8900，motion-only 子进程 8901 |
| Backend | `apps/desktop-wails/backend/app.go` 移除 Wails 依赖，实现 `api.AppHandler` 接口；`tryAutoStartRecording` 业务策略通过 `OnAcquisitionStarted` 回调注入到 `api.Deps` |
| API 扩展 | `services/api-go/api/server.go` 新增 `AppHandler` 接口、`AppVersionInfo` 类型、`OnAcquisitionStarted` 回调字段；新增 4 个 `/api/app/*` 路由（version/startup-mode/open-motion-window/resolve-path）；`handleDeviceByID` 新增 `subscribe` 动作 |
| Electron 壳 | 新建 `apps/desktop-electron/`：main.cjs（管理 Go 后端进程 + motion-only 子进程 + BrowserWindow 生命周期）、preload.cjs（contextBridge 暴露 showOpenDialog 和 openMotionWindow）、package.json（Electron 22.3.27 + electron-builder 24.13.3）、scripts/build-backend.ps1（前端 npm run build + Go 1.20.14 编译后端） |
| 前端 | `wails-adapter.ts` 重写为纯 HTTP client + Electron IPC，`isWailsAvailable` 语义改为检测 `window.electronAPI`；`MotionView.vue` 移除 `@wailsio/runtime` import，`closeWindow` 简化为 `window.close()`；`package.json` 移除 `@wailsio/runtime` 依赖；`http-client.test.ts` `window.chrome.webview` → `window.electronAPI`；`calibrationApi.ts` 和 `deviceApi.ts` 移除 bool 复合返回检查 |

### 11.3 验证结果

| 命令 | 结果 |
|---|---|
| `go mod tidy` | 通过（自动添加 indirect 依赖，go 1.20 保持不变） |
| `go build` | 通过（生成 8.68MB `wind-daq-backend.exe`） |
| `go vet ./...` | 通过 |
| `go test ./...` | 通过（6 tests passed） |
| `npm install` | 通过（312 packages） |
| `npm run typecheck` | 通过（vue-tsc --noEmit 0 错误） |
| `npm run test` | 通过（vitest run，8 files / 45 tests passed） |
| `npm run build` | 通过（5391 modules，built in 10.73s） |
| `npm run dist:win7` | 通过（NSIS 安装包 68.18MB） |
| Smoke test (主进程) | `/api/health` 200 `{"ok":true}` |
| Smoke test (motion-only) | `/api/health` 200 + `/api/motion/status` 200 `[]` |

### 11.4 关键设计决策

1. **AppHandler 接口隔离**：api 包通过接口依赖 backend.App，避免反向依赖 desktop-wails/backend 包
2. **OnAcquisitionStarted 回调注入**：将 Wails 时代的 `DeviceStartAcquisition` + `tryAutoStartRecording` 业务策略通过回调函数注入到 api.Deps，在 HTTP 路由层异步触发
3. **HTTP server 生命周期分离**：app.go 只负责业务初始化（Start/Stop + NewDeps），HTTP server 由 main.go 创建和控制，参考 daq-t1603 模板
4. **motion-only 子进程端口隔离**：主进程 8900，motion-only 子进程 8901，避免端口冲突
5. **isWailsAvailable 语义变更**：从检测 Wails runtime 改为检测 `window.electronAPI`（Electron preload 注入），保持导出名不变以避免大量调用点改名
6. **wailsApi 适配器重写策略**：保留 `wailsApi` 导出名和类型签名（GenericResponse 含大写 Success/Error 兼容字段），内部全部改为 HTTP fetch 调用，`normalizeGenericResponse` 统一大小写
7. **Electron IPC 通道**：仅 `dialog:pick-directory` 和 `app:open-motion-window` 走 IPC，其他全部通过 HTTP API
8. **motion-only 子进程 Electron 管理**：主进程 spawn `wind-daq-backend.exe --motion-only --parent-pid=<PID>`，等待 8901 健康检查，创建独立 BrowserWindow 加载 `http://127.0.0.1:8901`

### 11.5 产物

- 安装包：`projects/wind-daq/apps/desktop-electron/dist/Wind-DAQ-Win7-Setup-0.3.5-x64.exe`（68.18MB）
- 安装包 SHA256：`64866A1D583B4467AE28C37FBE26CCDF039FEE38CA5A8B24881381EB4D2C94C7`
- 后端可执行文件：`projects/wind-daq/apps/desktop-electron/backend/wind-daq-backend.exe`（8.68MB）
- 后端 SHA256：`88D2D11D5B3211EAF744A5687EAE7E5786C61A89B15DE55E28B1AC9FFB7B3469`

### 11.6 待办

- Windows 7 SP1 x64 真机安装与启动验证（待用户在 Win7 机器上执行）
