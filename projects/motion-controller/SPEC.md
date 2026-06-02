# Spec: Motion Controller Desktop Application

## Objective

从 wind-daq 项目中提取运动控制器模块，形成一个独立的桌面应用程序。**关键设计原则：分层共享** — 运动控制器的底层设备控制层位于 `shared/device-sdk/go/motion/`，可复用的应用级运动控制编排位于 `shared/motion-control/go/`，wind-daq 和 motion-controller 都通过 Go module 引用，确保修复错误时两个项目都能受益。

**共享边界:**
- **共享（shared/device-sdk/go/motion/）**: 控制器通用类型、接口定义、协议实现、模拟器、可复用硬件 adapter
- **共享（shared/motion-control/go/）**: MotionManager、profile 持久化、HTTP route glue、状态轮询 helper
- **项目私有**: Wails 生命周期、产品级配置选择、页面布局、项目特定 wiring

**用户场景:**
- 工程师需要独立调试和测试运动控制器
- 需要快速验证控制器配置和运动参数
- 不需要完整的 wind-daq 数据采集功能

**成功标准:**
- 可独立运行的 Wails 桌面应用
- **完整实现** Simulated、B140、WTNMC4A 三种控制器类型
- 底层设备控制代码在 `shared/device-sdk/go/motion/` 中，应用级 motion 行为在 `shared/motion-control/go/` 中，两个项目共享引用
- 完整的运动控制 UI（监控、点动、移动、配置）
- 实时状态推送（100ms 刷新）
- 可独立保存/加载控制器配置

## Tech Stack

| Component | Technology | Version |
|-----------|-----------|---------|
| Desktop Shell | Wails | v2.12.0 |
| Backend | Go | Follow `go.work` / module `go.mod` |
| Frontend | Vue | 3.5.14 |
| State Management | Pinia | 3.0.4 |
| UI Framework | Tailwind CSS | 4.3.0 |
| Charts | ECharts | 6.1.0 |
| Build Tool | Vite | 5.4.20 |
| Testing (Go) | testing + testify | - |
| Testing (TS) | Vitest | 4.1.7 |

## Commands

```powershell
# Development
cd projects/motion-controller/apps/desktop-wails
wails dev

# Build
wails build

# Run Backend Only (for API testing)
cd projects/motion-controller/services/api-go
go run cmd/server/main.go

# Test Backend
cd projects/motion-controller/services/api-go
go test ./...

# Test Frontend
cd projects/motion-controller/apps/desktop-wails/frontend
npm run test

# Lint Frontend
cd projects/motion-controller/apps/desktop-wails/frontend
npm run lint
```

## Project Structure

### 共享设备控制代码（关键）

```
shared/device-sdk/go/motion/         # 运动控制器设备控制层（共享）
├── core/                             # 领域类型（零依赖）
│   └── types.go                      # AxisName, AxisConfig, ControllerType, etc.
├── ports/                            # 接口定义（零实现）
│   └── motion.go                     # MotionController, MotionProfileStore
└── adapters/                         # 硬件适配器实现
    ├── hardware/
    │   ├── simulated_motion.go       # Simulated 控制器（完整实现）
    │   ├── b140_motion.go            # B140 控制器（完整实现）
    │   └── wtnmc4a_motion.go         # WTNMC4A 控制器（完整实现）
    └── config/
        └── motion_profile_store.go   # 配置存储
```

### 共享应用级运动控制代码（关键）

```
shared/motion-control/go/          # 运动控制应用层共享模块
├── manager/                       # MotionManager 编排
├── profile/                       # ProfileStore 与文件持久化
├── httpapi/                       # /api/motion/* route glue
└── events/                        # 状态轮询 helper（不直接依赖 Wails）
```

### Motion Controller 项目

```
projects/motion-controller/
├── apps/
│   └── desktop-wails/              # Wails 桌面应用
│       ├── backend/                # Wails Go 宿主（薄绑定层）
│       │   └── app.go             # App struct + MotionXxx 方法
│       ├── frontend/              # Vue 3 前端
│       │   ├── src/
│       │   │   ├── api/           # API 层（Wails + HTTP fallback）
│       │   │   ├── components/
│       │   │   │   └── motion/    # 运动控制 UI 组件
│       │   │   ├── stores/        # Pinia 状态管理
│       │   │   ├── views/         # 页面视图
│       │   │   └── shared/        # TypeScript 类型定义
│       │   └── wailsjs/           # 自动生成的 Wails 绑定
│       ├── config/                # 配置文件
│       ├── main.go                # Wails 入口
│       ├── go.mod                 # 依赖 shared/device-sdk/go/motion
│       └── wails.json
├── services/
│   └── api-go/                    # Go 后端
│       ├── cmd/server/            # 独立服务入口
│       ├── api/                   # HTTP handlers
│       ├── internal/
│       │   └── usecase/           # 项目私有业务逻辑（MotionManager）
│       └── pkg/appcontext/        # 应用上下文
├── SPEC.md                        # 本规范
└── README.md
```

### 依赖关系

```
┌─────────────────────┐      ┌─────────────────────┐
│     wind-daq        │      │  motion-controller  │
│                     │      │                     │
│  services/api-go    │      │  services/api-go    │
│       │             │      │       │             │
│       ▼             │      │       ▼             │
│  ┌─────────────────────────────────────────┐    │
│  │   shared/motion-control/go              │    │
│  │   shared/device-sdk/go/motion           │    │
│  └─────────────────────────────────────────┘    │
└─────────────────────┘      └─────────────────────┘
```

## Code Style

### Go (Backend)

```go
// 接口定义清晰，职责单一
type MotionController interface {
    Connect(ctx context.Context) error
    Disconnect(ctx context.Context) error
    Status(ctx context.Context) (ControllerStatus, error)
    MoveTo(ctx context.Context, axis AxisName, position float64) error
    // ...
}

// 使用有意义的错误类型
type MotionError struct {
    Code    string
    Message string
    Axis    AxisName
}

func (e *MotionError) Error() string {
    return fmt.Sprintf("motion error on axis %s: %s - %s", e.Axis, e.Code, e.Message)
}
```

### TypeScript (Frontend)

```typescript
// 组件使用 Composition API + <script setup>
<script setup lang="ts">
import { ref, computed } from 'vue'
import { useMotionStore } from '@/stores/motionStore'

const motionStore = useMotionStore()
const selectedAxis = ref<AxisName>('X')

const currentStatus = computed(() => 
  motionStore.statusList.find(s => s.axis === selectedAxis.value)
)
</script>

// API 层封装 Wails/HTTP 双通道
export const motionApi = {
  async connect(profileId: string): Promise<void> {
    if (window.go?.backend?.App) {
      return window.go.backend.App.MotionConnect(profileId)
    }
    return httpClient.post('/api/motion/connect', { profileId })
  }
}
```

## Testing Strategy

### Backend (Go)
- **单元测试**: adapters 层（SimulatedMotionController, FileMotionProfileStore）
- **集成测试**: usecase 层（MotionManager 完整流程）
- **覆盖率目标**: 80%+
- **运行**: `go test ./...`

### Frontend (TypeScript)
- **单元测试**: motionStore, API 层, 工具函数
- **组件测试**: 关键组件（MotionControlPanel, MotionControllerConfig）
- **覆盖率目标**: 70%+
- **运行**: `npm run test`

## Boundaries

### Always (必须做)
- 提交前运行测试（`go test ./...` + `npm run test`）
- 遵循命名约定（Go: camelCase, TS: camelCase, 组件: PascalCase）
- 验证所有输入（控制器配置、运动参数）
- 保持类型定义同步（Go core ↔ TypeScript shared）
- 实时状态推送使用事件机制
- **修改共享代码时，同时验证 wind-daq 和 motion-controller 两个项目**

### Ask First (先询问)
- 修改控制器接口（MotionController port）
- 添加新的控制器类型
- 修改配置文件格式
- 添加新的 Wails 绑定方法
- 修改 shared/device-sdk/go/motion 或 shared/motion-control/go 的公共 API

### Never (绝不做)
- 提交密钥或敏感信息
- 直接修改 vendor 目录
- 删除未通过的测试
- 在 UI 组件中直接调用硬件（必须通过 usecase 层）
- 在 core 层引入外部依赖
- **在项目内部复制设备控制代码（必须引用 shared）**
- **在 shared/device-sdk/go/motion 中引入项目特定依赖**

## Success Criteria

### 功能完整性
- [ ] **完整实现** Simulated 控制器
- [ ] **完整实现** B140 控制器
- [ ] **完整实现** WTNMC4A 控制器
- [ ] 完整的运动控制操作（MoveTo, MoveBy, Jog, Home, Stop, EStop）
- [ ] 多轴支持（X, Y, Z, U）
- [ ] 实时状态监控（位置、速度、限位、回零状态）
- [ ] 控制器配置管理（创建、编辑、删除、保存、加载）

### UI 完整性
- [ ] 控制器状态面板
- [ ] 轴控制卡片（监控、点动、移动）
- [ ] 控制器配置对话框
- [ ] 连接管理
- [ ] 键盘快捷键（Esc = EStop）
- [ ] 位置历史梯度条

### 架构完整性
- [ ] **设备控制代码共享** — shared/device-sdk/go/motion 被两个项目引用
- [ ] **应用级 motion 共享** — shared/motion-control/go 被两个项目引用
- [ ] 六边形架构（usecase → core + ports, adapters → ports, core 零依赖）
- [ ] Wails 绑定 + HTTP API 双通道
- [ ] 100ms 状态推送
- [ ] 独立配置文件存储

### 可独立运行
- [ ] 无需 wind-daq 即可编译和运行（通过 shared module 引用）
- [ ] 独立的 Go module（引用 shared/device-sdk/go/motion 和 shared/motion-control/go）
- [ ] 独立的 package.json

### 代码共享验证
- [ ] 修改 shared/device-sdk/go/motion 或 shared/motion-control/go 后，两个项目都能编译通过
- [ ] 两个项目的测试都能通过
- [ ] 共享代码的修复自动同步到两个项目

## Open Questions

1. **配置文件路径**: 是否沿用 wind-daq 的用户配置目录，还是使用新的路径？
2. **默认配置**: 是否需要预设一些常用的控制器配置？
3. **日志级别**: 是否需要可配置的日志级别？
4. **自动更新**: 是否需要 Wails 的自动更新功能？

## Implementation Notes

### 项目骨架创建

使用工作区脚本创建项目骨架：

```powershell
powershell -File .\scripts\new-project.ps1 -Name motion-controller
```

这将创建符合工作区规则的标准项目结构。

### 结构校验

创建项目后，运行结构校验确保合规：

```powershell
powershell -File .\scripts\validate-structure.ps1
```
