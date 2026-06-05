# Spec: DAQ-T-1603 网络扫描发现

## Objective

当前 daq-t1603 应用需要用户手动输入 IP:端口来添加设备（默认 `192.168.3.101:9000`）。在网络环境中，用户往往不知道设备 IP，需要在局域网中自动发现设备。

增加 UDP 网络扫描功能：发送广播发现包，解析设备响应，在 UI 中展示发现的设备，允许一键添加到设备列表。

**用户故事：**
- 作为操作员，我点击"扫描"按钮，等待 3 秒，看到局域网中所有 DAQ-T-1603 设备列表
- 我可以在扫描结果中为每个设备点击"添加"，自动填入 IP 和端口
- 扫描中显示加载状态，扫描完成后显示设备数量

## Tech Stack

| 层 | 技术 |
|---|---|
| 后端语言 | Go 1.26.1 (以 go.work 为准) |
| 前端框架 | Vue 3 + TypeScript + Pinia |
| 图标库 | @lucide/vue (已引入) |
| 桌面框架 | Wails v2 |
| 网络 | Go `net` 标准库 (无新依赖) |

## Commands

```powershell
cd projects/daq-t1603/apps/desktop-wails
go test ./...
go vet ./...
go build -buildvcs=false ./...
wails generate module
cd frontend
npm run typecheck
npm run build
cd ..\..\..\..\..
```

> 新增或修改 Wails 绑定方法后必须运行 `wails generate module`，并提交生成的 `frontend/wailsjs/go/backend/App.d.ts`、`frontend/wailsjs/go/backend/App.js`、`frontend/wailsjs/go/models.ts` 相关变化。

## Project Structure (改动部分)

只新增/修改以下文件，不引入新目录：

```
apps/desktop-wails/
├── core/
│   └── types.go                          # + ScanResult 类型
├── ports/
│   └── scan.go                           # + DeviceScanPort 接口 (新文件)
├── usecase/
│   └── device_usecase.go                 # + ScanDevices() 方法 + 测试
├── adapters/hardware/
│   ├── t1603_scanner.go                  # + T1603Scanner UDP 扫描实现 (新文件)
│   ├── t1603_scanner_test.go             # + 单元测试 (新文件)
│   └── simulated_adapter.go              # + 模拟扫描器
├── backend/
│   └── app.go                            # + ScanDevices() Wails 绑定
├── main.go                               # + 注入 scanner
└── frontend/src/
    ├── bridge/deviceBridge.ts            # + scanDevices() + ScanResult 接口
    ├── stores/deviceStore.ts             # + scanDevices action + scanResults/scanning 状态
    ├── components/layout/
    │   ├── AppShell.vue                  # + 扫描按钮 + 扫描结果对话框
    │   └── DeviceSidebar.vue             # + 扫描按钮入口
    └── components/device/
        └── ScanResultList.vue            # + 扫描结果展示组件 (新文件)
```

## Code Style

匹配现有项目风格——无注释、直接委托、简洁错误处理。

**后端风格示例（匹配现有 backend/app.go）：**

```go
func (a *App) ScanDevices() ([]core.ScanResult, error) {
    return a.deviceUC.ScanDevices()
}
```

**端口接口风格（匹配现有 ports/device.go）：**

```go
type DeviceScanPort interface {
    Scan() ([]core.ScanResult, error)
}
```

**适配器风格（匹配现有 t1603_adapter.go）：**

```go
type T1603Scanner struct {}

func NewT1603Scanner() *T1603Scanner {
    return &T1603Scanner{}
}

func (s *T1603Scanner) Scan() ([]core.ScanResult, error) {
    // UDP broadcast "T1603" on port 7000, parse responses
}
```

**前端 Bridge 风格（匹配现有 deviceBridge.ts）：**

```typescript
export interface ScanResult {
  id: string
  name: string
  type: string
  available: boolean
  address: string
  port: number
  macAddress?: string
  serialNumber?: string
  firmwareVersion?: string
}

export function scanDevices(): Promise<ScanResult[]> {
  return ScanDevices() as any
}
```

**ScanResult 字段约束：**
- `address` 必须是纯 IP/host（例如 `192.168.1.7`），不能包含端口或 UDP 响应地址中的 `:7000`
- `port` 是 TCP 连接端口，默认 `9000`，优先使用设备响应中的端口字段
- 当 JSON 缺少 `ip`、CSV 地址为空、或短响应只返回 `DAQT1603` 时，必须从 UDP `remoteAddr` 中用 `net.SplitHostPort` 提取 host 作为 `address`
- `id` 稳定规则：优先 `macAddress`，其次 `address:port`，最后 `remoteHost:port`

**前端 Store 风格（匹配现有 deviceStore.ts）：**

```typescript
const scanResults = ref<ScanResult[]>([])
const isScanning = ref(false)

async function scanDevices(): Promise<void> {
  isScanning.value = true
  try {
    scanResults.value = await bridge.scanDevices()
  } finally {
    isScanning.value = false
  }
}
```

## Testing Strategy

| 级别 | 覆盖范围 | 框架 |
|---|---|---|
| 单元测试 | `T1603Scanner` 响应解析逻辑（JSON/CSV/短响应，含 remoteAddr host 提取） | Go `testing` |
| 单元测试 | 广播目标计算、发现命令、端口、超时返回已发现设备、按稳定 id 去重 | Go `testing`（可注入 packet transport 或 helper 测试） |
| 单元测试 | `DeviceUsecase.ScanDevices()` 委托 | Go `testing`（同现有伪造适配器模式） |
| 集成测试 | 后端 `ScanDevices` 绑定（模拟适配器） | Go `testing`（同 `backend/app_test.go`） |
| 前端 | 当前项目无前端测试框架，通过 typecheck/build 和人工验收覆盖交互 | `npm run typecheck` / `npm run build` |

**测试要求：** 所有后端新代码应有测试覆盖。不要求依赖真实局域网或真实设备；UDP 发送/读取可通过可注入 transport、短超时本地 UDP 测试或 helper 级测试覆盖。

**前端人工验收：**
- 点击扫描按钮后显示 spinner，扫描期间不能重复触发扫描
- 扫描完成后显示发现数量和结果列表
- 关闭扫描对话框时清空 `scanResults`
- 点击结果中的“添加”后关闭扫描对话框，打开添加设备对话框，并预填 `address`、`port`、建议名称

## Boundaries

- **Always:**
  - 扫描后对结果按 `id` 去重
  - 扫描超时 3 秒（与 wind-daq 一致）
  - 向所有子网广播地址 + `255.255.255.255` 发送发现包
  - 解析 T1603 的 JSON 和 CSV 两种响应格式
  - 模拟模式下返回预设的模拟扫描结果
  - 前端扫描期间显示加载状态（spinner）
  - 前端在关闭对话框时清空扫描结果
  - 扫描入口由 `DeviceSidebar` 发起，扫描对话框与添加设备预填逻辑由 `AppShell` 统一管理

- **Ask first:**
  - 修改扫描协议（命令字符串、端口、超时）
  - 增加对其他设备类型的扫描支持
  - 修改扫描 UI 交互流程

- **Never:**
  - 不在扫描逻辑中加入业务状态管理（扫描仅发现，不连接）
  - 不自动连接扫描到的设备
  - 不将扫描结果持久化到配置文件中
  - 不添加新依赖包

## Success Criteria

1. 后端 `ScanDevices()` 返回 `[]ScanResult`（无错误表示至少完成一次扫描）
2. UDP 扫描发送 `"T1603"` 到端口 7000，解析 JSON/CSV 响应
3. 扫描超时 3 秒，超时后返回已发现的设备（可能为空列表）
4. 模拟模式下返回至少 2 个模拟设备结果
5. 前端点击"扫描"按钮显示 spinner，完成后展示设备列表
6. 前端点击设备旁的"添加"按钮，自动填入 IP 和端口，关闭扫描对话框并打开添加对话框（预填信息）
7. Wails 绑定已重新生成，`deviceBridge.ts` 可以从生成的 `App` 模块导入 `ScanDevices`
8. `go test ./...` 通过
9. `go vet ./...` 通过
10. `npm run typecheck` 通过
11. `npm run build` 通过

## Open Questions

1. 模拟扫描应返回几个设备？建议 2 个（固定为 `192.168.1.7:9000` 和 `192.168.1.8:9000`，用于测试和人工验收）
2. 扫描按钮放在哪里？建议放在 DeviceSidebar 顶部的"设备列表"旁，加一个扫描图标按钮，并通过事件通知 AppShell 打开扫描对话框
3. 扫描对话框是否复用现有添加设备对话框的样式？建议复用，在扫描结果中每行加"添加"按钮；点击后由 AppShell 预填现有添加设备表单
