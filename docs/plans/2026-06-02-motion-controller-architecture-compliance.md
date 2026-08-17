# Motion-Controller 架构合规重构实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 motion-controller 项目重构为符合六边形架构规范的结构，消除 backend 层的业务逻辑，补全 services/api-go 缺失的层级，清理重复的 shared 代码。

**Architecture:** 采用六边形（ports & adapters）架构。业务逻辑集中在 `services/api-go/internal/` 的 core/usecase/ports/adapters 层；Wails backend 仅做参数转换和 usecase 调用；共享代码统一到工作空间级 `shared/` 目录。

**Tech Stack:** Go 1.25+, Wails v2, Vue 3, Pinia

---

## 当前问题清单

| # | 违规 | 严重度 | 位置 |
|---|---|---|---|
| V1 | `backend/app.go` 包含完整业务逻辑（~330行），应仅做参数转换 | P0 | `apps/desktop-wails/backend/app.go` |
| V2 | `services/api-go/internal/` 缺少 `core/`、`ports/`、`adapters/` 层 | P1 | `services/api-go/internal/` |
| V3 | `services/api-go/cmd/server/main.go` API 层内联业务逻辑 | P1 | `services/api-go/cmd/server/main.go` |
| V4 | `projects/motion-controller/shared/device-sdk/` 与工作空间级 `shared/device-sdk/` 重复 | P2 | `projects/motion-controller/shared/device-sdk/` |
| V5 | `projects/motion-controller/shared/frontend/` 应在工作空间级 `shared/frontend/` | P2 | `projects/motion-controller/shared/frontend/` |

---

## 重构策略

**核心思路：** `services/api-go/internal/usecase/motion.go` 已包含完整的 `MotionManager` 业务逻辑。`backend/app.go` 中的 `motionManagerWrapper` 是它的一个副本。重构的关键是让 `backend/app.go` 通过依赖注入使用 `usecase.MotionManager`，而不是自己重新实现一遍。

**依赖关系：**
- `shared/device-sdk/go/motion/` — 已有完整的 core/ports/adapters（✅ 无需修改）
- `services/api-go/internal/usecase/motion.go` — 已有 MotionManager（✅ 无需修改）
- `services/api-go/pkg/appcontext/context.go` — 已有初始化逻辑（✅ 无需修改）
- `apps/desktop-wails/backend/app.go` — 需要大幅瘦身（🔧 主要修改点）
- `apps/desktop-wails/main.go` — 需要更新依赖注入（🔧 配套修改）

---

## Task 1: 瘦身 backend/app.go — 消除业务逻辑

**目标：** 将 `backend/app.go` 从 ~330 行业务逻辑瘦身为 ~80 行纯参数转换层。

**Files:**
- Modify: `projects/motion-controller/apps/desktop-wails/backend/app.go`
- Modify: `projects/motion-controller/apps/desktop-wails/main.go`
- Modify: `projects/motion-controller/apps/desktop-wails/go.mod`

**Step 1: 更新 go.mod 添加 services/api-go 依赖**

在 `go.mod` 中确认已有 replace 指令指向 `services/api-go`：

```
replace (
    motion-controller/services/api-go => ../../services/api-go
    shared/device-sdk/go => ../../../../shared/device-sdk/go
)
```

并在 `require` 中添加：
```
require (
    motion-controller/services/api-go v0.0.0
    ...
)
```

**Step 2: 重写 backend/app.go**

将 `motionManagerWrapper` 及其所有业务方法替换为对 `usecase.MotionManager` 的委托调用。新结构：

```go
package backend

import (
    "context"
    "time"

    "github.com/wailsapp/wails/v2/pkg/runtime"
    "shared/device-sdk/go/motion/core"

    "motion-controller/services/api-go/internal/usecase"
)

type App struct {
    ctx         context.Context
    cancel      context.CancelFunc
    motionMgr   *usecase.MotionManager
    statusCancel context.CancelFunc
}

func NewApp(motionMgr *usecase.MotionManager) *App {
    return &App{motionMgr: motionMgr}
}

func (a *App) Startup(ctx context.Context) {
    a.ctx, a.cancel = context.WithCancel(ctx)
    statusCtx, cancel := context.WithCancel(ctx)
    a.statusCancel = cancel
    go a.emitStatusLoop(statusCtx)
}

func (a *App) Shutdown(ctx context.Context) {
    if a.statusCancel != nil {
        a.statusCancel()
    }
}

func (a *App) emitStatusLoop(ctx context.Context) {
    ticker := time.NewTicker(200 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            statuses := a.motionMgr.StatusAll(a.ctx)
            if len(statuses) > 0 {
                runtime.EventsEmit(a.ctx, "motion:status", statuses)
            }
        }
    }
}

// 以下为 Wails 绑定方法 — 仅做参数转换，委托给 usecase

func (a *App) MotionGetProfiles() []core.MotionControllerProfile {
    return a.motionMgr.GetProfiles()
}

func (a *App) MotionUpsertProfile(profile core.MotionControllerProfile) error {
    return a.motionMgr.UpsertProfile(profile)
}

func (a *App) MotionDeleteProfile(id string) error {
    return a.motionMgr.DeleteProfile(id)
}

func (a *App) MotionConnect(id string) error {
    return a.motionMgr.Connect(a.ctx, id)
}

func (a *App) MotionDisconnect(id string) error {
    return a.motionMgr.Disconnect(a.ctx, id)
}

func (a *App) MotionGetStatus() []core.ControllerStatus {
    return a.motionMgr.StatusAll(a.ctx)
}

func (a *App) MotionMoveTo(id string, axis string, position float64) error {
    return a.motionMgr.MoveTo(a.ctx, id, core.AxisName(axis), position)
}

func (a *App) MotionMoveBy(id string, axis string, delta float64) error {
    return a.motionMgr.MoveBy(a.ctx, id, core.AxisName(axis), delta)
}

func (a *App) MotionJog(id string, axis string, velocity float64) error {
    return a.motionMgr.Jog(a.ctx, id, core.AxisName(axis), velocity)
}

func (a *App) MotionHome(id string, axis string) error {
    return a.motionMgr.Home(a.ctx, id, core.AxisName(axis))
}

func (a *App) MotionStop(id string, axis string) error {
    return a.motionMgr.Stop(a.ctx, id, core.AxisName(axis))
}

func (a *App) MotionEmergencyStop(id string) error {
    return a.motionMgr.EmergencyStop(a.ctx, id)
}

func (a *App) MotionResetEmergencyStop(id string) error {
    return a.motionMgr.ResetEmergencyStop(a.ctx, id)
}

func (a *App) MotionDefinePosition(id string, axis string, position float64) error {
    return a.motionMgr.DefinePosition(a.ctx, id, core.AxisName(axis), position)
}
```

**关键变化：**
- 删除 `motionManagerWrapper` 结构体及其所有方法（~200行业务逻辑）
- 删除 `motionControllerConfigChanged` 函数（已在 usecase 中存在）
- 删除 `getOrCreateController` 函数（已在 usecase 中存在）
- 删除 `NewApp()` 中的配置文件读写逻辑（已由 appcontext 管理）
- `NewApp` 改为接收 `*usecase.MotionManager` 参数（依赖注入）
- 每个方法仅做 `string → core.AxisName` 的参数转换 + 委托调用

**Step 3: 更新 main.go 依赖注入**

```go
package main

import (
    "embed"

    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"

    "motion-controller/apps/desktop-wails/backend"
    "motion-controller/services/api-go/pkg/appcontext"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    appCtx, err := appcontext.NewAppContext("")
    if err != nil {
        println("Error initializing:", err.Error())
        return
    }

    app := backend.NewApp(appCtx.MotionManager)

    err = wails.Run(&options.App{
        Title:  "Motion Controller",
        Width:  1280,
        Height: 800,
        AssetServer: &assetserver.Options{
            Assets: assets,
        },
        OnStartup:  app.Startup,
        OnShutdown: app.Shutdown,
        Bind: []interface{}{
            app,
        },
    })
    if err != nil {
        println("Error:", err.Error())
    }
}
```

**Step 4: 运行 go build 验证编译**

Run: `cd projects/motion-controller/apps/desktop-wails && go build ./...`
Expected: 编译通过，无错误

**Step 5: 提交**

```bash
git add projects/motion-controller/apps/desktop-wails/backend/app.go
git add projects/motion-controller/apps/desktop-wails/main.go
git add projects/motion-controller/apps/desktop-wails/go.mod
git add projects/motion-controller/apps/desktop-wails/go.sum
git commit -m "refactor(motion-controller): remove business logic from Wails backend, delegate to usecase.MotionManager"
```

---

## Task 2: 补全 services/api-go 六边形架构层级

**目标：** 在 `services/api-go/internal/` 下补全 `core/`、`ports/`、`adapters/` 目录结构，即使目前为空也建立占位文件，为未来扩展提供正确的架构骨架。

**Files:**
- Create: `projects/motion-controller/services/api-go/internal/core/motion/types.go`
- Create: `projects/motion-controller/services/api-go/internal/ports/motion.go`
- Create: `projects/motion-controller/services/api-go/internal/adapters/config/.gitkeep`
- Create: `projects/motion-controller/services/api-go/internal/adapters/hardware/.gitkeep`

**Step 1: 创建 core/motion/types.go**

当前 motion 的核心类型已在 `shared/device-sdk/go/motion/core/` 中定义。项目级 `core/` 仅存放项目特有的领域类型（如有）。创建占位文件：

```go
package motion

// 项目级运动控制领域类型
// 核心共享类型定义在 shared/device-sdk/go/motion/core/
// 此包用于存放 motion-controller 项目特有的领域扩展
```

**Step 2: 创建 ports/motion.go**

当前 motion 的端口接口已在 `shared/device-sdk/go/motion/ports/` 中定义。项目级 `ports/` 仅存放项目特有的接口（如有）。创建占位文件：

```go
package ports

// 项目级运动控制端口接口
// 共享端口接口定义在 shared/device-sdk/go/motion/ports/
// 此包用于存放 motion-controller 项目特有的接口扩展
```

**Step 3: 创建 adapters 占位目录**

```
internal/adapters/config/.gitkeep
internal/adapters/hardware/.gitkeep
```

**Step 4: 提交**

```bash
git add projects/motion-controller/services/api-go/internal/
git commit -m "chore(motion-controller): scaffold hexagonal architecture layers in services/api-go"
```

---

## Task 3: 重构 API 层 — 消除内联业务逻辑

**目标：** 将 `services/api-go/cmd/server/main.go` 中的内联 HTTP handler 重构为使用 `api/` 包的标准路由模式，与 windlabx4 项目的 API 层风格一致。

**Files:**
- Create: `projects/motion-controller/services/api-go/api/server.go`
- Modify: `projects/motion-controller/services/api-go/cmd/server/main.go`

**Step 1: 创建 api/server.go**

```go
package api

import (
    "context"
    "encoding/json"
    "net/http"

    "shared/device-sdk/go/motion/core"

    "motion-controller/services/api-go/internal/usecase"
)

type Deps struct {
    MotionManager *usecase.MotionManager
}

func NewRouter(deps Deps) http.Handler {
    mux := http.NewServeMux()

    mux.HandleFunc("/api/motion/profiles", func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodGet:
            profiles := deps.MotionManager.GetProfiles()
            writeJSON(w, http.StatusOK, profiles)
        case http.MethodPut:
            var profile core.MotionControllerProfile
            if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
                writeError(w, http.StatusBadRequest, err.Error())
                return
            }
            if err := deps.MotionManager.UpsertProfile(profile); err != nil {
                writeError(w, http.StatusBadRequest, err.Error())
                return
            }
            writeJSON(w, http.StatusOK, map[string]bool{"success": true})
        default:
            w.WriteHeader(http.StatusMethodNotAllowed)
        }
    })

    mux.HandleFunc("/api/motion/status", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        writeJSON(w, http.StatusOK, deps.MotionManager.StatusAll(context.Background()))
    })

    return mux
}

func writeJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, map[string]any{"success": false, "error": message})
}
```

**Step 2: 重构 cmd/server/main.go**

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "motion-controller/services/api-go/api"
    "motion-controller/services/api-go/pkg/appcontext"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()

    appCtx, err := appcontext.NewAppContext("")
    if err != nil {
        log.Fatalf("Failed to create app context: %v", err)
    }

    if _, err := appCtx.MotionManager.LoadProfiles(); err != nil {
        log.Printf("Warning: failed to load motion profiles: %v", err)
    }

    handler := api.NewRouter(api.Deps{
        MotionManager: appCtx.MotionManager,
    })

    addr := ":8900"
    if envAddr := os.Getenv("MOTION_SERVER_ADDR"); envAddr != "" {
        addr = envAddr
    }

    srv := &http.Server{Addr: addr, Handler: handler}
    go func() {
        log.Printf("Motion Controller server starting on %s", addr)
        if err := srv.ListenAndServe(); err != http.ErrServerClosed {
            log.Printf("Server error: %v", err)
        }
    }()

    <-ctx.Done()
    log.Println("Shutting down server...")
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer shutdownCancel()
    _ = srv.Shutdown(shutdownCtx)
}
```

**Step 3: 运行 go build 验证**

Run: `cd projects/motion-controller/services/api-go && go build ./...`
Expected: 编译通过

**Step 4: 提交**

```bash
git add projects/motion-controller/services/api-go/api/
git add projects/motion-controller/services/api-go/cmd/
git commit -m "refactor(motion-controller): extract API handlers from cmd/main.go to api/server.go"
```

---

## Task 4: 清理项目级重复 shared 代码

**目标：** 删除 `projects/motion-controller/shared/device-sdk/` 中与工作空间级 `shared/device-sdk/` 重复的 DAQ 代码。motion 相关代码已在工作空间级 `shared/device-sdk/go/motion/` 中存在。

**Files:**
- Delete: `projects/motion-controller/shared/device-sdk/go/daq/` (整个目录)
- Delete: `projects/motion-controller/shared/device-sdk/go/go.mod`
- Keep: `projects/motion-controller/shared/frontend/motion/` (前端共享模块暂保留在项目级，见说明)

**Step 1: 确认无代码引用项目级 shared/device-sdk**

搜索 `motion-controller/shared/device-sdk` 的导入引用。预期：无任何 Go 文件导入此路径（所有导入都指向工作空间级 `shared/device-sdk/go`）。

Run: `cd projects/motion-controller && grep -r "motion-controller/shared/device-sdk" --include="*.go" .`
Expected: 无匹配结果

**Step 2: 删除重复的 DAQ 文件**

删除 `projects/motion-controller/shared/device-sdk/` 整个目录。这些 DAQ 驱动文件与工作空间级 `shared/device-sdk/go/daq/` 完全重复，且 motion-controller 项目不使用 DAQ 设备。

**Step 3: 提交**

```bash
git add -u projects/motion-controller/shared/device-sdk/
git commit -m "chore(motion-controller): remove duplicate DAQ device-sdk, use workspace-level shared/device-sdk"
```

**关于 `shared/frontend/motion/` 的说明：**

前端共享模块 `projects/motion-controller/shared/frontend/motion/` 暂时保留在项目级。原因：
1. 工作空间级 `shared/frontend/` 目录尚未建立标准结构
2. 前端共享模块的迁移需要更新 npm workspace 配置
3. 此迁移影响 windlabx4 项目的前端引用，需要协调处理
4. 建议作为独立任务在所有项目前端统一重构时处理

---

## Task 5: 验证与清理

**目标：** 确保所有修改后项目编译通过，运行结构验证脚本。

**Step 1: 运行 Go 编译检查**

Run: `cd projects/motion-controller/apps/desktop-wails && go build ./...`
Expected: 编译通过

Run: `cd projects/motion-controller/services/api-go && go build ./...`
Expected: 编译通过

**Step 2: 运行工作空间结构验证**

Run: `powershell -File .\scripts\validate-structure.ps1`
Expected: 通过（或仅报告已知的非 motion-controller 相关问题）

**Step 3: 最终提交（如有遗漏修复）**

```bash
git commit -m "chore(motion-controller): final cleanup after architecture compliance refactoring"
```

---

## 重构前后对比

### backend/app.go

| 指标 | 重构前 | 重构后 |
|---|---|---|
| 行数 | ~330 | ~80 |
| 业务逻辑 | Profile CRUD、控制器生命周期、配置变更检测、状态轮询 | 无 |
| 导入 adapters | 是（直接导入 config/hardware） | 否（仅导入 usecase） |
| 依赖注入 | 无（内部创建所有依赖） | 通过构造函数注入 MotionManager |

### services/api-go/internal/

| 目录 | 重构前 | 重构后 |
|---|---|---|
| core/ | 不存在 | 存在（占位） |
| ports/ | 不存在 | 存在（占位） |
| usecase/ | 存在 | 存在（不变） |
| adapters/ | 不存在 | 存在（占位） |

### shared/ 重复

| 路径 | 重构前 | 重构后 |
|---|---|---|
| `projects/motion-controller/shared/device-sdk/go/daq/` | 存在（重复） | 已删除 |
| `projects/motion-controller/shared/frontend/motion/` | 存在 | 保留（待统一迁移） |
