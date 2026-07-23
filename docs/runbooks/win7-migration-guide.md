# Win7 迁移指南 — 将主线项目移植到 lts/win7

> 适用分支：`lts/win7`（worktree `AI-Workspace-win7`）
>
> 前置阅读：`docs/runbooks/win7-lts-development-workflow.md`（日常同步流程）  
> 架构决策：`docs/decisions/ADR-008-workspace-win7-lts-worktree.md`  
> 实施计划：`docs/plans/2026-07-23-workspace-win7-lts-worktree.md`

## 1. 项目分类

| 类型 | 特征 | 示例 | 迁移策略 |
|---|---|---|---|
| A — 单模块 Wails | `core/ports/usecase/adapters/backend` 都在一个 Go module | `daq-t1603`、`daq-p1604` | 拆分业务模块，新增 HTTP adapter + Electron 壳 |
| B — 已有 HTTP API | 已有 `services/api-go` 通过 HTTP 提供业务能力 | `wind-daq`、`motion-controller`、`1604Cal` | 复用 HTTP API，替换 Wails 壳为 Electron |
| C — 纯计算/文件 | 无硬件依赖，仅算法 + 文件 I/O | `probe-interpolator` | 新增 HTTP service + Electron 壳 |
| D — 嵌套 Git | 项目自身是独立 Git 仓库 | `1604Cal` | 先确定提交所有权，再分层改造 |

## 2. 可复用平台组件

以下组件存在于 `lts/win7` 分支、可在各产品迁移中直接复用。

### 2.1 Electron 壳模板

```
projects/daq-t1603/apps/desktop-electron/
├── main.cjs              # Electron 主进程：启动 backend、health 轮询、创建窗口、dialog IPC
├── preload.cjs           # contextBridge 暴露 electronAPI
├── package.json          # electron 22.3.27 + electron-builder NSIS 配置
├── scripts/
│   └── build-backend.ps1 # Go 1.20.14 编译脚本
└── .gitignore            # 排除 node_modules/ backend/ dist/
```

迁移时只需修改：
- `package.json` 中的 `productName`、`appId`、`artifactName`、图标路径
- `main.cjs` 中 backend 子进程路径和命名
- 窗口标题、尺寸

### 2.2 Go 1.20 兼容日志

```
shared/device-sdk/go/pkg/slog/slog.go
```

80 行、零外部依赖。提供 `Info/Debug/Warn/Error` + `SetLevel`。

引入方式：

```go
import "shared.local/device-sdk/go/pkg/slog"
```

### 2.3 HTTP 服务模式

```
projects/daq-t1603/apps/desktop-wails/httpserver/
├── register.go           # Server struct + RegisterHandlers 入口
├── helpers.go            # 统一响应信封 {ok, data, error}
├── ws_hub.go             # WebSocket hub + EventBus 实现
├── device_handler.go     # 设备 HTTP handler 模板
├── recording_handler.go  # 录制 HTTP handler 模板
└── log_handler.go        # 日志 HTTP handler 模板
```

### 2.4 前端 Transport

```
projects/daq-t1603/apps/desktop-wails/frontend/src/bridge/
├── httpClient.ts         # fetch 封装，统一响应信封解包
├── wsClient.ts           # WebSocket 单例，自动重连 + 事件分发
```

## 3. 通用迁移步骤

### 3.1 移除 @wailsio/runtime

主线 `package.json` 依赖：

```json
"dependencies": {
  "@wailsio/runtime": "3.0.0-alpha.95",
  ...
}
```

Win7 版删除此行，替换为 HTTP/WebSocket transport。

### 3.2 替换 Go import

| 主线 | Win7 |
|---|---|
| `log/slog` | `shared.local/device-sdk/go/pkg/slog` |
| `math/rand/v2` | `math/rand` + `init() { rand.Seed(...) }` |
| `maps`、`slices`、`cmp` | std 替代或自写工具 |

### 3.3 go.mod 降级

```
-go 1.25.0
-go 1.25
+go 1.20
```

并删除 Wails 及其间接依赖：

```
-github.com/wailsapp/wails/v3 ...
-github.com/wailsapp/wails/webview2 ...
-github.com/coder/websocket ...
```

### 3.4 替换 Event 系统

主线通过 `app.Emit("daq:payload", data)` 推送事件。Win7 改为 WebSocket hub：

```go
wsHub := httpserver.NewWSHub()
hub.SetEventBus(wsHub)
go wsHub.Run(appCtx)
```

### 3.5 替换 Dialog

主线通过 Wails runtime 调用系统对话框。Win7 通过 Electron preload：

```ts
// preload.cjs
contextBridge.exposeInMainWorld('electronAPI', {
  showOpenDialog: () => ipcRenderer.invoke('dialog:pick-directory'),
})
```

前端 bridge 层统一封装：

```ts
export function pickDirectory(): Promise<string> {
  if (window.electronAPI?.showOpenDialog) {
    return window.electronAPI.showOpenDialog()
  }
  return Promise.resolve('')
}
```

### 3.6 main.go 改写

主线的 Wails 入口：

```go
app := application.New(application.Options{...})
app.Window.NewWithOptions(...)
app.Run()
```

Win7 改为 `net/http` 入口：

```go
mux := http.NewServeMux()
registerHandlers(mux, services...)
frontendFS, _ := fs.Sub(frontendAssets, "frontend/dist")
mux.Handle("/", http.FileServer(http.FS(frontendFS)))
srv := &http.Server{Addr: listenAddr, Handler: mux}
go srv.ListenAndServe()
```

## 4. A 类迁移：单模块 Wails → 拆分 + HTTP

适用：`daq-t1603`（已完成）、`daq-p1604`

### 步骤

1. 将 `core/ports/usecase/adapters` 从 `apps/desktop-wails/` 下拆出到新目录 `services/internal/` 或保持原位但更新 `go.mod`。
2. 创建 `services/api-go/` 或 `httpserver/` 包，将 Wails binding 方法转为 HTTP handler。
3. 在 `main.go` 中移除 Wails application，替换为 `net/http` 初始化 + `RegisterHandlers`。
4. 创建 `apps/desktop-electron/`（从 DAQ-T1603 模板复制并修改）。
5. 前端 bridge 层将 Wails bindings 调用改为 `httpClient.post/get`。
6. 事件订阅改为 `wsClient.on`。

### 验收

- 所有 usecase 测试在 Go 1.20 下通过。
- HTTP endpoint 覆盖全部 Wails binding 功能。
- 高频数据继续使用快照轮询，不改为 Electron IPC。
- NSIS 安装包生成通过。

## 5. B 类迁移：已有 HTTP API → 复用 + Electron 壳

适用：`wind-daq`、`motion-controller`、`1604Cal`

### 步骤

1. 确认 `services/api-go/` 可在 Go 1.20 编译；修复 `log/slog`、`math/rand/v2` 等不兼容 API。
2. `main.go`（Wails 壳）替换为 `net/http` 入口，复用已有 `api.NewRouter` 或 `RegisterRoutes`。
3. 创建 `apps/desktop-electron/`，Electron 启动 Go backend 后直接 `loadURL(backendURL)`。
4. 前端将 Wails adapter 替换为 HTTP client；已有 HTTP client 可直接复用。
5. 仅替换 Wails 专用功能：API 端口发现、文件对话框、退出确认。已有 SSE/WebSocket 保持不变。

### 验收

- 业务 API 和 SSE/WebSocket 行为与主线一致。
- 文件对话框走 Electron preload，不走 Wails。
- `1604Cal` 的嵌套 Git 仓库在修改前已明确提交所有权。

## 6. C 类迁移：纯计算/文件 → 新增 HTTP service

适用：`probe-interpolator`

### 步骤

1. 在项目下创建 `services/api-go/`。
2. 为每个 Wails binding 函数创建 HTTP handler。
3. 文件上传/下载通过 multipart 请求或 Electron dialog 返回绝对路径。
4. Go 1.20 下验证算法计算结果与主线一致。
5. 创建 `apps/desktop-electron/`。

### 验收

- 三孔、五孔、七孔 PRB 加载、计算、批处理和 CSV 导出均通过。
- Alpha/Beta 语义不做错误统一。
- 不迁移已废弃的五孔、三孔独立项目。

## 7. 常见问题

| 问题 | 原因 | 解决 |
|---|---|---|
| `go build` 失败：`log/slog: cannot find package` | 主线使用 Go 1.21+ 标准库 | 改为 `shared/device-sdk/go/pkg/slog` |
| `syscall.LoadDLL` 在 init 中导致崩溃 | DLL 不存在时包初始化失败 | 改为显式构造函数，报错返回 error |
| Electron 无法启动 backend | 子进程路径不对或 health 超时 | 检查 `main.cjs` 中 `backendPath` 和 `waitForBackend(15000)` |
| 前端白屏，JS 加载失败 | Vite 产物使用绝对路径 `/<script>` | 确认 `main.go` 中 `mux.Handle("/", http.FileServer(...))` |
| CORS 跨域错误 | Electron 加载 `file://` | 改为 Go 托管前端 `dist/`，Electron 加载 `http://127.0.0.1:port/` |
| `GOWORK` 错误 | Go 1.20 无法加载根 `go.work` | 设置 `$env:GOWORK="off"` |
| `go 1.20` 编译通过但无法运行 | 第三方依赖声明过高 go 版本 | 检查间接依赖的 `go.mod` |

## 8. 迁移完成后

1. 更新 `WIN7-SYNC-STATE.md` 记录产品状态。
2. 创建对应目录结构并在 Win7 分支提交。
3. 执行 Go 1.20、前端、Electron、NSIS 和 smoke test 验证。
4. 涉及设备时安排 Win7 真机验收。
5. 更新 `WIN7-PROGRESS.md` 中的产品迁移状态。
