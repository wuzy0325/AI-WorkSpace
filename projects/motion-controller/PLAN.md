# Phase 2: Implementation Plan

## Overview

将 wind-daq 中的运动控制器模块提取为独立桌面应用，同时建立共享设备控制层。

## 实施顺序

### Phase 2.1: 创建项目骨架与共享层

#### Step 1: 创建 motion-controller 项目骨架
```powershell
powershell -File .\scripts\new-project.ps1 -Name motion-controller
```

#### Step 2: 在共享 SDK 中创建运动控制器模块

`motion/` 作为 `shared/device-sdk/go` 的子包（**不创建独立的 go.mod**），以便适配器代码可以直接引用同级的 `ffi/`、`serialport/` 包。

```
shared/device-sdk/go/motion/
├── core/
│   └── types.go                # 从 wind-daq 复制并升级
├── ports/
│   └── motion.go               # 从 wind-daq 复制并升级（增加 context.Context）
├── adapters/
│   ├── hardware/
│   │   ├── simulated_motion.go      # 从 wind-daq 复制，调整 import 路径
│   │   ├── b140_motion.go           # ⚠️ 新建（MotionController 接口封装，底层协议待确认）
│   │   ├── wtnmc4a_motion.go        # ⚠️ 新建（基于 shared/device-sdk/go/ffi 封装）
│   │   ├── factory.go               # ⚠️ 新建（默认控制器工厂，按 ControllerType 选择适配器）
│   │   ├── simulated_motion_test.go       # 从 wind-daq 复制
│   │   ├── simulated_motion_debug_test.go  # 从 wind-daq 复制
│   │   └── wtnmc4a_motion_test.go          # ⚠️ 新建（基于 stub 的单元测试）
│   └── config/
│       ├── motion_profile_store.go       # 从 wind-daq 复制
│       └── motion_profile_store_test.go  # 从 wind-daq 复制
└── ffi/
    └── wtnmc4a_stub_test.go              # ⚠️ 新建（FFI 层 test double）
```

**关键说明：**

| 文件 | 来源 | 说明 |
|------|------|------|
| `simulated_motion.go` | 从 wind-daq 复制 | 调整 import 路径指向 shared 模块 |
| `b140_motion.go` | **新建** | 实现 `MotionController` 接口，底层通信协议待确认（串口/TCP？） |
| `wtnmc4a_motion.go` | **新建** | **基于现有 `shared/device-sdk/go/ffi` 封装**，不是从零写协议通信 |
| `factory.go` | **新建** | 默认 `MotionControllerFactory`，按 `ControllerType` 分配适配器，项目可通过注入自定义工厂覆盖 |
| `ports/motion.go` | 从 wind-daq 复制并升级 | 所有接口方法增加 `ctx context.Context` 参数 |

**port 接口升级（迁移到共享层时同步修正）：**

```go
// 旧版（wind-daq 现有）— 无 context
type MotionController interface {
    Connect() error
    Status() motion.ControllerStatus
    MoveTo(axis motion.AxisName, position float64) error
}

// 新版（shared 共享层）— 增加 context
type MotionController interface {
    Connect(ctx context.Context) error
    Status(ctx context.Context) motion.ControllerStatus
    MoveTo(ctx context.Context, axis AxisName, position float64) error
}
```

> **为什么升级？** 共享层是修正此设计缺陷的最佳时机。`context.Context` 支持超时控制、取消传播和链路追踪，对硬件 I/O 操作至关重要。迁移后 wind-daq 代码也同步受益。

**WTNMC4A 实现策略：** `shared/device-sdk/go/ffi/wtnmc4a.go` 已包含完整的 DLL FFI 绑定（`Init`、`CreateDevice`、`SetP`、`ReadLP`、`DecStop`、`StartAutoHomeSearch`、`GetRR1Status` 等）。`wtnmc4a_motion.go` 的任务是在此 FFI 层上实现 `MotionController` 接口，包含连接管理、状态轮询、运动指令封装。单元测试使用 `ffi/wtnmc4a_stub_test.go`（test double）模拟 DLL 调用。

**B140 实现策略：** 需先确认通信方式（串口 RS-232/RS-485？TCP/IP？）。如果协议规格不明确，先创建接口定义和类型占位，标记为 deferred。

**测试文件迁移清单：**

| wind-daq 源文件 | 目标路径 | 迁移方式 |
|-----------------|---------|---------|
| `internal/adapters/hardware/simulated_motion_test.go` | `motion/adapters/hardware/simulated_motion_test.go` | 复制并调整 import |
| `internal/adapters/hardware/simulated_motion_debug_test.go` | `motion/adapters/hardware/simulated_motion_debug_test.go` | 复制并调整 import |
| `internal/adapters/config/motion_profile_store_test.go` | `motion/adapters/config/motion_profile_store_test.go` | 复制并调整 import |
| `internal/usecase/motion_test.go` | **项目私有，不迁移** | 每个项目各自维护 usecase 层测试 |

#### Step 3: 确认模块归属（无需独立 go.mod）

`motion/` 作为 `shared/device-sdk/go` 的子包，**不需要创建独立的 `go.mod`**。现有模块路径如下：

```
shared/device-sdk/go      → module shared/device-sdk/go  (go 1.21)
shared/device-sdk/go/ffi  → 同 module，import "shared/device-sdk/go/ffi"
shared/device-sdk/go/motion/core  → 同 module，import "shared/device-sdk/go/motion/core"
shared/device-sdk/go/motion/ports → 同 module，import "shared/device-sdk/go/motion/ports"
...
```

> **Go 版本说明：** 工作区根 `go.work` 使用 `go 1.25.0`，`shared/device-sdk/go` 的 `go.mod` 标注 `go 1.21`。Go 工具链支持跨版本编译：workspace level 的 toolchain 版本只需 >= 各 module 声明的版本。因此 motion 子包与现有 shared SDK 保持 `go 1.21` 即可，wind-daq 项目标注 `go 1.25.0`。

`motion/` 子包可直接引用同级包：
- `motion/adapters/hardware/wtnmc4a_motion.go` → `import "shared/device-sdk/go/ffi"`
- `motion/adapters/hardware/b140_motion.go`（如需串口）→ `import "shared/device-sdk/go/serialport"`

#### Step 4: 更新 go.work

在 workspace 根目录的 `go.work` 中添加新项目路径（保留所有现有条目）：

```go
go 1.25.0

use (
    projects/five-hole-interpolator/apps/desktop-wails
    projects/wind-daq/apps/desktop-wails
    projects/wind-daq/services/api-go
    shared/algorithms/go/fivehole
    shared/device-sdk/go

    // 新增
    projects/motion-controller/apps/desktop-wails
    projects/motion-controller/services/api-go
)
```

> **注意：** `shared/device-sdk/go/motion` 是子包，不需要独立的 `use` 条目。

#### Step 5: 验证共享层编译与测试

```powershell
cd shared/device-sdk/go
go build ./...
go test ./motion/... -v -cover
```

验证目标：
- `go build ./...` 编译通过
- `go test ./motion/...` 全部测试通过
- `go test -cover ./motion/...` 覆盖率达标（目标 ≥80%）

---

### Phase 2.2: 修改 wind-daq 引用共享层

#### Step 6: 更新 wind-daq Go 依赖

修改 `projects/wind-daq/services/api-go/go.mod`：

```go
module wind-daq/services/api-go

go 1.25.0

require (
    shared/device-sdk/go v0.0.0
)

replace shared/device-sdk/go v0.0.0 => ../../../../shared/device-sdk/go
```

> 从 `projects/wind-daq/services/api-go/` 到根目录需要 4 层 `..`

#### Step 7: 更新 wind-daq 代码引用（按顺序执行）

**7.1: 先确认 shared 模块可用**
```powershell
cd projects/wind-daq/services/api-go
go mod tidy
```

**7.2: 更新 import 路径**

| 原文件 | 旧 import | 新 import |
|--------|----------|----------|
| `internal/usecase/motion.go` | `"wind-daq/services/api-go/internal/core/motion"` → `"shared/device-sdk/go/motion/core"` | 并将包名引用改为 `core.AxisName` 等 |
| `internal/usecase/motion.go` | `"wind-daq/services/api-go/internal/ports"` → `"shared/device-sdk/go/motion/ports"` | |
| `internal/usecase/motion_test.go` | 同上 | 同上 |
| `internal/adapters/config/motion_profile_store.go` | 同上 | 同上 |
| `internal/adapters/config/motion_profile_store_test.go` | 同上 | 同上 |
| `pkg/appcontext/context.go`（如引用） | 同上 | 同上 |

**7.3: wind-daq 删除旧文件（确认编译通过后）**
- `internal/core/motion/types.go`
- `internal/ports/motion.go`
- `internal/adapters/hardware/simulated_motion.go`
- `internal/adapters/config/motion_profile_store.go`

#### Step 8: 验证 wind-daq 编译

```powershell
cd projects/wind-daq/services/api-go
go build ./...
go test ./... -v -cover
```

---

### Phase 2.3: 构建 motion-controller 应用

#### Step 9: 创建 motion-controller 后端

在 `projects/motion-controller/services/api-go/` 下：

```
services/api-go/
├── go.mod                    # 依赖 shared/device-sdk/go
├── cmd/server/
│   └── main.go              # 独立服务入口
├── internal/
│   └── usecase/
│       ├── motion.go         # 项目私有 MotionManager（复用 wind-daq 逻辑，调整 import）
│       └── motion_test.go    # 项目私有 usecase 测试
└── pkg/
    └── appcontext/
        └── context.go       # 应用上下文
```

go.mod 内容：
```go
module motion-controller/services/api-go

go 1.25.0

require shared/device-sdk/go v0.0.0

replace shared/device-sdk/go v0.0.0 => ../../../../shared/device-sdk/go
```

#### Step 10: 创建 Wails 应用

配置 `apps/desktop-wails/`：
- `main.go` — Wails 入口
- `backend/app.go` — 薄绑定层（参数转换 + usecase 调用，零业务逻辑）
- `go.mod` — 引用 `shared/device-sdk/go`（通过 replace）

#### Step 11: 复制前端代码

从 wind-daq 复制（包括测试文件）：

**源文件：**
- `src/components/motion/` → 所有运动控制组件
- `src/stores/motionStore.ts` → Pinia store
- `src/api/motionApi.ts` → API 层
- `src/shared/types/motion.ts` → TypeScript 类型

**测试文件：**
- `src/components/motion/*.spec.ts` 或 `*.test.ts`
- `src/stores/motionStore.spec.ts`（如果存在）
- `src/api/motionApi.spec.ts`（如果存在）

**修改：**
- 移除与 DAQ/校准的耦合（检查清单：store 中 wind-daq 特有的交叉引用、API 层对 DAQ 模块的依赖、类型定义中 DAQ 相关字段）
- 更新 import 路径

#### Step 12: 验证 motion-controller 编译

```powershell
cd projects/motion-controller/services/api-go
go build ./...
go test ./... -v -cover

cd ../apps/desktop-wails
wails build
```

---

### Phase 2.4: 最终验证

#### Step 13: 双项目验证

```powershell
# 验证 wind-daq
cd projects/wind-daq/services/api-go
go test ./... -cover
cd ../apps/desktop-wails
wails build

# 验证 motion-controller
cd projects/motion-controller/services/api-go
go test ./... -cover
cd ../apps/desktop-wails
wails build
```

#### Step 14: 更新文档
- 更新 SPEC.md 中的 Open Questions 答案
- 更新 README.md
- 添加 ADR（如需要）

---

## 并发模型设计

每个设备适配器遵循独立的并发隔离策略，确保一台设备的 I/O 阻塞不会影响其他设备。

```
┌──────────────────────────────────────────────────┐
│                  MotionManager (usecase)          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│  │ Controller│  │ Controller│  │ Controller│      │
│  │ A         │  │ B         │  │ C         │      │
│  │ (goroutine)│  │ (goroutine)│  │ (goroutine)│    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘       │
│       │             │             │              │
│       ▼             ▼             ▼              │
│  ┌─────────────────────────────────────┐         │
│  │     每个 adapter 独立 goroutine      │         │
│  │     + channel 通信（非共享锁）        │         │
│  └─────────────────────────────────────┘         │
└──────────────────────────────────────────────────┘
```

### 设计原则

| 层级 | 策略 |
|------|------|
| **SimulatedMotionController** | 现有 `sync.RWMutex` + goroutine 模型（纯内存计算，无 I/O 阻塞） |
| **B140MotionController** | 独立 goroutine 持有串口/TCP 连接，通过 channel 收发指令；`context.Context` 超时控制 |
| **WTNMC4AMotionController** | 独立 goroutine 持有网络连接 + FFI 调用，channel 序列化命令；`context.Context` 超时控制 |
| **MotionManager** | 仅管理控制器实例生命周期，不直接参与设备 I/O |

### 关键约束

- 每个 `MotionController` 实例的 `Connect()` 创建独立 goroutine/协程
- 所有设备 I/O 调用需有超时（通过 `context.WithTimeout`）
- Wails 事件循环与设备 I/O 通过 channel 解耦，不共享锁
- `EmergencyStop()` 应能跨 goroutine 取消所有正在进行的 I/O 操作

---

## B140/WTNMC4A 适配器实现策略

### 选项 A: 完整实现（推荐）

如果已有协议规格文档：
- Phase 2.1 Step 2 中实现两个适配器
- WTNMC4A：基于现有 `shared/device-sdk/go/ffi` 层封装（已验证 DLL 绑定已就绪）
- B140：需先明确通信协议（串口 RS-232/RS-485 或 TCP/IP）
- 需要额外 2-3 天工作量
- 完全满足 SPEC 验收条件

### 选项 B: Deferred 实现

如果协议规格不明确：
- Phase 2.1 Step 2 中只复制 simulated_motion.go 和相关测试
- WTNMC4A：在 shared/device-sdk/go/ffi 层之上创建接口定义 + 存根实现
- B140：创建接口定义和类型占位
- 在 Phase 2.3 之前或之后单独实现
- SPEC 成功标准调整为"Simulated 完整实现 + B140/WTNMC4A 接口定义"

**决策点：** 在 Phase 2.1 Step 2 开始前确认选择哪个选项。

---

## 回滚策略

| 步骤 | 回滚条件 | 回滚操作 |
|------|---------|---------|
| Step 5 (shared 编译失败) | 共享层编译/测试不通过 | 回退到"复制前"状态，确认文件完整性后重试 |
| Step 8 (wind-daq 编译失败) | 修改 import 后 wind-daq 编译不通过 | 恢复 go.mod 和 import 路径，排查共享层 API 兼容性 |
| Step 7.3 (删除旧文件后失败) | 删除旧文件后编译失败 | 从 Git 恢复已删除文件 |
| Step 12 (motion-controller 编译失败) | 新项目编译不通过 | 排查 go.mod replace 路径和 Wails 配置 |

**建议：** 在 Step 5、7.2、7.3、8 前创建 Git 分支或标签，确保可快速回滚。

---

## 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| 共享层引入循环依赖 | 严格遵循六边形架构，core 层零依赖；motion 子包只引用同级 `ffi/`、`serialport/` |
| wind-daq 现有测试失败 | 逐步迁移，每步验证；先更新 import 再删除旧文件 |
| Wails 绑定冲突 | 使用独立的 module name |
| 前端组件耦合 | 仔细移除 DAQ 相关引用；建立耦合检查清单 |
| B140/WTNMC4A 协议不明确 | 先实现 Simulated，其他 deferred |
| **设备 I/O 阻塞 Wails 事件循环** | **每个 adapter 独立 goroutine + channel 通信，context 超时控制** |
| **共享 SDK module 中的包名冲突** | 统一使用 module `shared/device-sdk/go`，motion 子包按功能分区命名 |

---

## 验证检查点

- [ ] Step 5: shared/device-sdk/go/motion 编译通过
- [ ] Step 5: shared/device-sdk/go/motion 测试通过（含覆盖率 ≥80%）
- [ ] Step 8: wind-daq 编译通过
- [ ] Step 8: wind-daq 测试通过（含覆盖率检查）
- [ ] Step 12: motion-controller 编译通过
- [ ] Step 12: motion-controller 测试通过（含覆盖率 ≥80%）
- [ ] Step 13: 两个项目都能独立运行
- [ ] Step 13: 修改共享代码后两个项目均能编译通过（代码共享验证）
