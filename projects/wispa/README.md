# WISPA

Standalone Wails desktop application (WISPA — WindTuner Intelligent-Pressure Scanning Application) for DAQ-P-1604 pressure data acquisition, monitoring, and CSV recording.

## Architecture

This project intentionally uses a single Go module under `apps/desktop-wails` instead of a separate `services/api-go` module. It still follows the workspace hexagonal boundaries.

```text
apps/desktop-wails/
  core/               # pure domain types
  ports/              # interfaces only
  usecase/            # orchestration
  adapters/           # hardware, config, recording, logging implementations
  backend/            # Wails binding layer only
  frontend/           # Vue 3 / Vite / Pinia / ECharts UI
```

See `CLAUDE.md` for the project-specific constraints.

## Modes

Real hardware mode only. Configure a DAQ-P-1604 profile in the app with the device IP and port (default 9000).

## Development Commands

```powershell
# Go 开发
cd projects/wispa/apps/desktop-wails
$env:GOWORK="off"
go test ./...
go vet ./...
go build -buildvcs=false ./...
```

```powershell
# 前端开发
cd projects/wispa/apps/desktop-wails/frontend
npm install --no-audit --no-fund
npm run typecheck
npm run build
npm run test
```

```powershell
# 桌面应用开发（热更新 / bindings 生成）
cd projects/wispa/apps/desktop-wails
$env:GOWORK="off"
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings
go run github.com/wailsapp/wails/v3/cmd/wails3 dev
```

## Release Commands（对外交付打包）

```powershell
cd projects/wispa/apps/desktop-wails
$env:GOWORK="off"
go run github.com/wailsapp/wails/v3/cmd/wails3 build   # wails3 build 内部自动使用 -tags production
```

**必须设置 `GOWORK=off`** 以隔离工作空间中其他模块的 wails/v2 间接引用，
否则构建的 exe 可能运行时报 "correct build tags" 错误。
详见 `docs/decisions/ADR-004-wails-v3-production-build.md`。

## Shared Code

- `shared/device-sdk/go/protocol` 包含 DAQ-P-1604 协议帧解析、单位转换、连接 helper 等跨项目复用代码。
- `programs/p1604-ts-diag` 是设备时间戳诊断工具，用于实机验证协议行为。
- 项目特定的录制器、profile 选择、Wails UI 装配留在本项目内。

## Development Rules

- `core/` 不得引入硬件、文件 I/O、串口/网络、框架依赖。
- `ports/` 只含接口定义。
- `usecase/` 通过 ports 接口调用设备，不直接访问硬件。
- `backend/` 只做参数转换 + usecase 调用，不含业务逻辑。
- `frontend/src/bridge/` 是唯一允许 import 生成 `wailsjs/` 绑定的前端层。
