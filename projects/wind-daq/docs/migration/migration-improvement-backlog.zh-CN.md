# Wind-DAQ 迁移待完善清单

> 目的：基于当前 `docs/plans`、`projects/wind-daq/docs/migration/ts-reference-feature-map.md`、当前源码结构和验证结果，整理后续需要补齐、修正和验收的事项。本文用于后续收敛迁移，不替代原始迁移计划。

## 1. 当前总体判断

当前 `wind-daq` 已完成可运行 MVP、主要 Vue UI 迁移、Go hexagonal backend 基础重建和 Wails 桌面打包闭环，但仍未达到真实硬件生产级完整迁移。

| 评估口径 | 当前完成度 | 说明 |
|---|---:|---|
| 迁移计划看板口径 | 85% - 90% | `docs/plans/2026-05-20-wind-daq-ts-migration.md` 中主要后端、前端、Wails、文档任务已勾选，当前构建和测试基本通过。 |
| 真实工程可交付口径 | 65% - 75% | 模拟设备、UI、API、桌面包可用；真实网络扫描、运动控制硬件、完整校准/遍历流程、HIL 验证和结构验收仍需补齐。 |

## 2. 已确认通过的验证

以下验证已在当前工作区执行并通过：

| 范围 | 命令 | 结果 |
|---|---|---|
| Go backend tests | `go test ./...` in `projects/wind-daq/services/api-go` | 通过 |
| Go backend build | `go build -buildvcs=false ./...` in `projects/wind-daq/services/api-go` | 通过 |
| Go backend vet | `go vet ./...` in `projects/wind-daq/services/api-go` | 通过 |
| Frontend typecheck | `npm run typecheck` in `projects/wind-daq/apps/desktop-wails/frontend` | 通过 |
| Frontend unit tests | `npm run test` in `projects/wind-daq/apps/desktop-wails/frontend` | 通过，5 files / 26 tests |
| Frontend production build | `npm run build` in `projects/wind-daq/apps/desktop-wails/frontend` | 通过，有 ECharts chunk size warning |
| Wails Go tests | `go test ./...` in `projects/wind-daq/apps/desktop-wails` | 通过 |
| Wails package build | `wails build -skipbindings` in `projects/wind-daq/apps/desktop-wails` | 通过，生成 `build/bin/wind-daq.exe` |

当前未通过项：

| 范围 | 命令 | 结果 | 原因 |
|---|---|---|---|
| Workspace structure | `powershell -File .\scripts\validate-structure.ps1` | 失败 | 根目录存在未登记的 `npm-cache/`、`services/`。 |

## 3. 当前已完成能力

### 3.1 后端架构

- Go backend 已按 hexagonal architecture 拆分为 `core`、`ports`、`usecase`、`adapters`、`api`、`bootstrap`。
- `core` 已包含 device、motion、calibration、traversal、storage、report 等领域类型或纯逻辑。
- `ports` 已承载设备、运动、校准、遍历、存储、报告等接口定义。
- `usecase` 已承载设备管理、采集、运动、校准、遍历、存储、报告编排逻辑。
- `adapters` 已包含 simulated device、simulated scanner、DAQ-P-1604、DAQ-T-1603、simulated motion、CSV storage、CSV report 等适配器。
- `api.NewRouter` 已统一暴露设备、DAQ stream、motion、calibration、traversal、storage、report HTTP API。
- `internal/bootstrap.BuildAPIServer` 已统一组装依赖，`cmd/server/main.go` 可保持薄启动器方向。

### 3.2 设备与采集

- Profile CRUD 已实现。
- Connect / disconnect 已实现。
- Start / stop acquisition 已实现。
- Latest data 查询已实现。
- SSE realtime stream 已实现。
- History buffer 已实现。
- Simulated DAQ 默认闭环已实现。
- DAQ-P-1604 和 DAQ-T-1603 adapter 源码已存在，并已接入 profile type factory。
- DAQ-T-1603 config API 和 Unit config API 已有后端支持。

### 3.3 前端 UI

- Vue 3 + Vite + TypeScript 前端已建成。
- AppShell、MainTopBar、AppRailNav、MainBottomBar、DeviceSidebar、DeviceDetailPanel、DeviceOverviewPanel、DeviceManagementDrawer、RealtimeChart、GlobalSettingsModal 已落地。
- Dashboard、Motion、Calibration、Traversal、Storage、Logs 页面入口已落地。
- Pinia store 已覆盖 device、theme、i18n、feedback。
- HTTP client、device API、SSE client 已存在。
- ECharts / vue-echarts 已接入实时图表。
- 前端测试基础已建立。

### 3.4 桌面壳与文档

- Wails v2 桌面壳已建立。
- Wails frontend assets 可嵌入并打包。
- `GetVersion()` 级别 Wails binding 可用于验证桌面桥接通道。
- README、STRUCTURE、runbook、HIL plan、integrated smoke checklist 等文档已存在。

## 4. 待完善事项总览

| 优先级 | 类别 | 事项 | 当前状态 | 完成标准 |
|---|---|---|---|---|
| P0 | 结构验收 | 修复 workspace structure validation | 未通过 | `powershell -File .\scripts\validate-structure.ps1` 通过 |
| P0 | 文档一致性 | 同步 `ts-reference-feature-map.md` 与当前源码 | 滞后 | Feature map 中 Done/Partial/Missing 与源码一致 |
| P0 | 联调验证 | 执行 integrated smoke checklist | 未完整记录 | API + Vite/Wails + simulated acquisition 全流程通过并记录 |
| P1 | 真实硬件 | DAQ-P-1604 HIL 验证 | 未记录 | 真实设备 connect/start/stream/stop/disconnect 通过 |
| P1 | 真实硬件 | DAQ-T-1603 HIL 验证 | 未记录 | 真实设备 connect/config/start/stream/stop/disconnect 通过 |
| P1 | 设备发现 | 实现真实网络扫描 | 缺失或未接入 | `GET /api/device/scan` 可发现真实 DAQ 设备 |
| P1 | 运动控制 | B140 adapter 和 HIL 验证 | 缺失 | B140 connect/status/move/home/stop 通过 |
| P1 | 运动控制 | WTNMC4A adapter 和 HIL 验证 | 缺失 | WTNMC4A FFI/串口链路通过 |
| P1 | 校准 | 1604 校准完整流程 | 基础框架已存在 | 五孔/三孔/总压/总温流程可跑通并保存结果 |
| P1 | 遍历 | Traversal workflow 真实流程 | 基础框架已存在 | 路径生成、运动、采集、存储、进度状态闭环 |
| P1 | 存储报告 | 前端存储页面真实接 API 状态复核 | 文案疑似过期 | UI 与 `/api/storage/*`、`/api/report/*` 实际能力一致 |
| P2 | 前端性能 | ECharts chunk 体积优化 | 有 warning | build 无 chunk size warning 或明确接受该 warning |
| P2 | 桌面运行模式 | 生产环境 API sidecar 策略 | 文档有说明，需验证 | 打包后启动策略、端口冲突、退出清理可验证 |
| P2 | 测试覆盖 | 前端 API/SSE 与页面 smoke 测试增强 | 部分覆盖 | 核心页面和 API error state 有自动化覆盖 |

## 5. P0 必须先处理

### 5.1 修复 workspace structure validation

当前失败信息：

```text
Workspace structure check failed.
 - Unexpected top-level entry: npm-cache
 - Unexpected top-level entry: services
```

处理建议：

1. 判断根目录 `npm-cache/` 是否为误生成目录。
2. 如果是误生成目录，删除或移动到允许的临时目录。
3. 判断根目录 `services/` 是否为误生成目录。
4. 如果是误生成目录，删除或移动到正确 project 下。
5. 如果二者确实是新架构需要的顶层目录，更新 `workspace.structure.json` 并补充 `docs/decisions/` 说明。

验收命令：

```powershell
powershell -File .\scripts\validate-structure.ps1
```

通过标准：输出 `Workspace structure check passed.`。

### 5.2 同步 feature map

`projects/wind-daq/docs/migration/ts-reference-feature-map.md` 当前存在滞后迹象，例如：

- `DAQ-P-1604 driver` 标为 Missing，但当前存在 `internal/adapters/hardware/daq_p1604.go`。
- `DAQ-T-1603 driver` 标为 Missing，但当前存在 `internal/adapters/hardware/daq_t1603.go`。
- `HTTP motion API` 标为 Missing，但当前 `api/server.go` 已暴露 `/api/motion/*`。
- `SSE realtime stream` 前端列仍为空，但当前存在 `src/api/sse-client.ts` 和 `RealtimeChart.vue`。
- `Channel chart` 标为 Partial，但当前已有 ECharts `RealtimeChart`。

同步要求：

- 每一项只按当前源码和验证证据标记。
- 没有 HIL 记录的真实硬件项不应直接标为 Done，可标为 `Partial` 并注明“adapter exists, HIL pending”。
- UI 已接入但业务流未真实闭环的项标为 `Partial`。
- 旧 Electron IPC、preload、generated Wails JS 继续保持 `Do not migrate`。

验收标准：

- Feature map 与当前源码一致。
- Feature map 中每个 `Partial` 都写明剩余工作。
- Feature map 中每个 `Done` 都能关联测试、构建、截图、smoke 或 HIL 证据。

### 5.3 执行 integrated smoke checklist

当前已有文档：`projects/wind-daq/docs/runbooks/integrated-smoke-checklist.md`。

建议至少覆盖：

- API server 启动。
- Frontend dev server 启动。
- Dashboard 默认加载 simulated profile。
- Device scan 显示 simulated scanner 结果。
- Connect + start acquisition。
- Realtime chart 收到数据。
- Stop acquisition。
- Disconnect。
- Storage start/stop 若前端已接入，则验证录制文件生成。
- Wails build 后应用能启动并加载 UI。

通过标准：

- checklist 中所有 P0 项被勾选。
- 无前端 console error。
- 后端无 panic。
- 截图或日志路径写回 checklist。

## 6. P1 真实硬件与业务流程

### 6.1 DAQ-P-1604

当前状态：

- Adapter 源码已存在。
- 已接入 `bootstrap.deviceFactory`。
- 未见真实硬件 HIL 通过记录。

待完善：

- 补充串口参数、协议命令、单位设置、错误处理和超时记录。
- 使用真实 DAQ-P-1604 执行 connect/start/read/stop/disconnect。
- 验证 ASCII frame 解析与实际设备输出一致。
- 验证断线、超时、异常帧不会导致 goroutine 泄漏或 panic。

验收：

```powershell
go test ./internal/adapters/hardware -run DAQP1604 -v
```

人工 HIL：

- 连接真实设备。
- 创建 DAQ-P-1604 profile。
- Dashboard start 后通道值持续更新。
- Stop 后数据停止刷新。
- 日志无 panic。

### 6.2 DAQ-T-1603

当前状态：

- Adapter 源码已存在。
- DAQ-T-1603 config API 已有。
- 前端 `DaqT1603Config.vue` 已存在。
- 未见真实硬件 HIL 通过记录。

待完善：

- 验证 Thermocouple Type / Cold Junction / Filter Hz 写入真实设备。
- 验证温度通道单位、异常值、断偶、冷端补偿行为。
- 验证 config 持久化后重启仍能恢复。

验收：

- 配置读取与写入均成功。
- Dashboard 显示温度数据。
- 断线和异常响应有明确错误提示。

### 6.3 真实设备扫描

当前状态：

- Simulated scanner 已实现。
- 真实网络扫描未闭环。

待完善：

- 增加 UDP broadcast / network scanner adapter。
- 支持 DAQ-P-1604 / DAQ-T-1603 扫描结果转换为 `device.ScanResult`。
- 扫描失败、超时、无设备时返回可展示错误或空结果。
- 前端 Drawer 显示真实扫描结果并能创建 profile。

验收：

- `GET /api/device/scan` 在真实网络环境返回设备。
- 无设备时不阻塞 UI。
- 模拟设备扫描仍可用。

### 6.4 Motion 真实控制器

当前状态：

- Motion core、ports、usecase、simulated motion、HTTP API 已存在。
- B140 / WTNMC4A 真实 adapter 未确认完成。

待完善：

- B140 adapter：connect/status/moveTo/moveBy/jog/home/stop/emergencyStop。
- WTNMC4A adapter：FFI 或串口链路、Windows build tags、资源释放。
- MotionView 接真实 API 状态反馈。
- 运动安全边界：限位、急停、错误状态、超时。

验收：

- simulated motion 自动化测试继续通过。
- 至少一台真实运动控制器完成 HIL。
- 急停优先级高于普通运动命令。
- 退出应用后控制器连接释放。

### 6.5 Calibration 完整流程

当前状态：

- Calibration core/usecase/API/UI shell 已存在。
- 完整五孔、三孔、总压、总温校准业务闭环仍需补齐和验证。

待完善：

- 明确 1604 校准 workflow 状态机。
- 从采集 hub 读取稳定数据点。
- 支持 average samples、pressure points、通道选择。
- 支持 pause/resume/stop。
- 支持结果保存和报告导出。
- 前端显示每一步状态、错误和完成结果。

验收：

- 无真实硬件时可用 fake data 跑完整状态机测试。
- 有真实硬件时完成至少一种校准类型 HIL。
- 校准算法只存在 Go backend/core/usecase，不回流到 Vue。

### 6.6 Traversal 完整流程

当前状态：

- Traversal core types、linear path interpolation、usecase/API/UI shell 已存在。
- 真实 workflow step、运动联动、采集联动、可视化仍需完善。

待完善：

- 路径规划 UI 与后端 config 对齐。
- 每个 traversal point 执行 motion move。
- 到位后读取 DAQ latest data。
- 写入 storage/report sink。
- 支持 pause/resume/stop 和异常恢复。
- 增加 heatmap/vector/cross-section visualization。

验收：

- fake motion + fake acquisition 自动化测试覆盖完整路径。
- 真实硬件 HIL 至少跑一个短路径。
- 停止或失败时运动停止、资源释放、状态可解释。

## 7. P2 工程质量与体验完善

### 7.1 前端 bundle 优化

当前 `npm run build` 有 warning：ECharts chunk 超过 500 KB。

可选处理：

- 对页面做动态 import，确认路由级 code splitting 已生效。
- 对 ECharts 做手动 chunk。
- 若该体积可接受，在文档中记录为已知 warning。

验收：

- build 无 warning，或 warning 被明确记录为可接受。

### 7.2 Wails 生产运行模式

当前状态：

- Wails build 可通过。
- 文档中说明 Go API 作为 sidecar 或单独运行。

待完善：

- 明确生产包中 API server 启动方式。
- 处理端口冲突。
- 处理应用退出时 API 进程清理。
- 处理 API 未启动时前端错误提示。
- 明确日志目录和配置目录。

验收：

- 双击 `wind-daq.exe` 后用户不需要手动启动 API，或文档明确要求手动启动。
- 退出桌面应用后无残留 API 进程。
- 端口占用时有清晰错误。

### 7.3 测试覆盖增强

待补测试：

- API client error mapping。
- SSE reconnect / parse error。
- DeviceManagementDrawer create/edit/delete/scan。
- StorageView start/stop/report generate。
- MotionView API error state。
- Calibration/Traversal status and error state。

验收：

```powershell
cd projects\wind-daq\apps\desktop-wails\frontend
npm run test
npm run typecheck
npm run build
```

全部通过。

## 8. 文档需要同步的位置

后续每完成一项，至少同步以下文档：

| 文档 | 需要更新内容 |
|---|---|
| `docs/plans/2026-05-20-wind-daq-ts-migration.md` | 迁移看板、验证记录、剩余任务状态 |
| `projects/wind-daq/docs/migration/ts-reference-feature-map.md` | 参考功能 Done/Partial/Missing 状态 |
| `projects/wind-daq/docs/runbooks/integrated-smoke-checklist.md` | 联调执行记录、截图、日志路径 |
| `projects/wind-daq/docs/runbooks/hil-validation-plan.md` | 真实硬件 HIL 结果 |
| `projects/wind-daq/README.md` | 用户运行步骤、端口、配置、桌面启动方式 |
| `projects/wind-daq/docs/STRUCTURE.md` | 新增目录和模块结构 |
| `projects/wind-daq/contracts/openapi/openapi.yaml` | API 变化 |

## 9. 建议执行顺序

1. 修复 workspace structure validation。
2. 更新 `ts-reference-feature-map.md`，让文档与源码一致。
3. 执行 simulated integrated smoke test 并记录。
4. 复核 Storage/Report 前端是否已真实接 API，修正文案或补接 API。
5. 真实 DAQ-P-1604 HIL。
6. 真实 DAQ-T-1603 HIL。
7. 实现真实网络扫描。
8. 补 B140 / WTNMC4A motion adapter 和 HIL。
9. 补完整 Calibration workflow。
10. 补完整 Traversal workflow 和 visualization。
11. 明确 Wails 生产 API sidecar 策略。
12. 做最终全量验证和文档收敛。

## 10. 最终完成标准

只有满足以下条件，才能把迁移状态从“基本完成”提升为“完整完成”：

- `powershell -File .\scripts\validate-structure.ps1` 通过。
- `go test ./...`、`go vet ./...`、`go build -buildvcs=false ./...` 在 backend 通过。
- `npm run test`、`npm run typecheck`、`npm run build` 在 frontend 通过。
- `go test ./...` 和 `wails build` 在 desktop-wails 通过。
- integrated smoke checklist 中 simulated 主流程通过。
- DAQ-P-1604 和 DAQ-T-1603 至少完成真实硬件 HIL。
- 至少一条真实 motion 控制器链路完成 HIL，或明确标为后续版本范围。
- Calibration 至少一种真实业务流程完成端到端验证。
- Traversal 至少一个短路径完成 motion + DAQ + storage 端到端验证。
- `ts-reference-feature-map.md` 与当前源码、测试和 HIL 结果一致。
- README 中用户运行方式与实际产物一致。
