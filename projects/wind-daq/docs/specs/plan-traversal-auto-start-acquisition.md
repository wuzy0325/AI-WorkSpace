# 实现计划：遍历测试主动启动关联设备采集

## 概述

修复"遍历测试前置检查假绿 + 启动后无数据"的设计缺陷：让前置检查真实反映"目标设备已连接且正在采集"，并在用户点击开始测试时主动调用 `StartAcquisition`（如未在采集），让遍历测试真正自包含。

## 问题背景

### 当前实现的缺陷

**前置检查的"DAQ acquisition hub is available"是假绿**——只查 hub 注入，不查目标设备是否在采集。

[traversal.go:303-348](../../services/api-go/internal/usecase/traversal.go#L303-L348) 中的 `CheckPreconditions`：

```go
hasReader := m.reader != nil   // ← 仅检查 hub 是否注入
// ...
{"name": "DAQ", "passed": hasReader, "message": "DAQ acquisition hub is available"},
```

而遍历实际取数路径强依赖设备**正在持续采集**：[traversal_acquisition.go:192-201](../../services/api-go/internal/usecase/traversal_acquisition.go#L192-L201)

```go
payload, ok := m.reader.GetLatestData(config.DeviceID)
if !ok {
    return m.failWithCode("no data available for device %s", traversal.ErrAcquisitionFailed, config.DeviceID)
}
```

**结果**：用户看到"全部通过 ✅"，点开始测试后立刻报 `no data available for device xxx`。需要用户自己反推"哦没开始采集"。

### 不合理的三个维度

| 维度 | 问题 |
|---|---|
| **前置检查失真** | "DAQ acquisition hub is available" 字面是"采集可用"，语义上诱导用户认为采集已就绪 |
| **依赖强假设不显式保证** | 遍历依赖 `GetLatestData` 持续返回新帧，但既不验证也不主动建立这个前提 |
| **错误信息不可读** | `no data available for device xxx` 不告诉用户根因是"没开始采集" |

## 架构决策

| 决策 | 理由 |
|---|---|
| 新增窄端口 `ports.AcquisitionController` 而非直接注入 `*DeviceManager` | 保持六边形边界：`usecase` 不依赖具体 manager 类型，便于测试 mock，与 `ChannelUnitProvider` 同模式 |
| 端口仅暴露 `IsConnected(id) bool` / `IsAcquiring(id) bool` / `StartAcquisition(id) error` 三个方法 | YAGNI：当前仅需这三种语义，未来如需 `StopAcquisition` 再加 |
| 主动启动采集放在 `Start(config)` 而非 `RunTraversalLoop` | `Start` 是同步入口，启动失败可直接返回 error 阻止进入 Running 态；Loop 是 goroutine，失败只能 setErrorLocked |
| 启动失败处理：直接返回 error，不进 Running 态 | 让用户立即看到根因，避免"已开始测试但啥也没采" |
| 测试结束后**不**自动停止采集 | 用户可能想继续看实时数据；保持当前行为契约 |
| 前置检查仍保留 `DAQ acquisition hub is available` 项 | hub 注入是基础设施级检查，与"设备在采集"是两个维度，分开更清晰 |

## 任务列表

### 阶段 1：后端端口与注入

#### 任务 1：新增 `ports.AcquisitionController` 端口 + DeviceManager 实现 + TraversalManager setter

**描述：** 在 `ports/device.go` 旁新增窄端口定义采集控制能力；DeviceManager 实现该端口（复用现有 `GetStatus` / `StartAcquisition`）；TraversalManager 新增 `acquisitionController` 字段和 `SetAcquisitionController` setter。

**验收标准：**
- [ ] `ports.AcquisitionController` 接口定义 `IsConnected(id string) bool` / `IsAcquiring(id string) bool` / `StartAcquisition(id string) error` 三个方法
- [ ] `DeviceManager` 实现该接口（无需新方法，复用 `GetStatus` / `StartAcquisition`）
- [ ] `TraversalManager` 新增 `acquisitionController ports.AcquisitionController` 字段（默认 nil）
- [ ] `SetAcquisitionController(c ports.AcquisitionController)` setter 方法实现，与 `SetUnitProvider` 同模式（mu.Lock + 注释说明调用时机）
- [ ] 端口为 nil 时所有调用方走降级路径（保持向后兼容）

**验证：**
- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过

**依赖：** 无

**涉及文件：**
- `services/api-go/internal/ports/device.go`（新增接口，~5 行）
- `services/api-go/internal/usecase/device_manager.go`（确认已实现，可能加注释声明实现接口）
- `services/api-go/internal/usecase/traversal.go`（新增字段 + setter，~15 行）

**预估范围：** 小（2-3 个文件）

---

#### 任务 2：装配根注入 `AcquisitionController`

**描述：** 在三个装配根（`bootstrap.go` / `context.go` / `apiserver.go`）中，紧邻 `SetUnitProvider` 调用处新增 `SetAcquisitionController(deviceMgr)`，确保运行期 TraversalManager 持有端口。

**验收标准：**
- [ ] `bootstrap.go:125` 附近新增 `travMgr.SetAcquisitionController(manager)`
- [ ] `context.go:126` 附近新增 `traversalMgr.SetAcquisitionController(deviceMgr)`
- [ ] `apiserver.go:98` 附近新增 `travMgr.SetAcquisitionController(manager)`
- [ ] 三处注入点位置一致（紧邻 SetUnitProvider），便于后续维护

**验证：**
- [ ] `go build ./...` 通过
- [ ] `go test ./internal/bootstrap/... ./pkg/appcontext/... ./pkg/apiserver/...` 通过

**依赖：** 任务 1

**涉及文件：**
- `services/api-go/internal/bootstrap/bootstrap.go`（1 行）
- `services/api-go/pkg/appcontext/context.go`（1 行）
- `services/api-go/pkg/apiserver/apiserver.go`（1 行）

**预估范围：** 小（3 个文件，每文件 1 行）

---

### 检查点：基础设施就绪
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./...` 现有用例不回归
- [ ] 三处装配根均能注入端口

---

### 阶段 2：后端核心逻辑

#### 任务 3：`CheckPreconditions` 增加"设备已连接"+"设备正在采集"两项检查

**描述：** 在 `traversal.go` 的 `CheckPreconditions` 中，基于注入的 `acquisitionController` 新增两项检查：`DeviceConnected` 和 `DeviceAcquiring`。当端口为 nil 时（旧装配）保持原有 4 项检查不变（向后兼容）。

**验收标准：**
- [ ] 端口非 nil 时新增两项检查：`{"name": "DeviceConnected", "passed": isConnected, "message": "..."}`、`{"name": "DeviceAcquiring", "passed": isAcquiring, "message": "..."}`
- [ ] `allPassed` 计算包含新两项（端口非 nil 时）
- [ ] 端口为 nil 时检查项数量保持 4 项，`allPassed` 不变（向后兼容）
- [ ] 使用 `config.DeviceID` 查询（与 `cfg = *config` / `m.config` 同模式，支持启动确认对话框场景）
- [ ] message 文案为英文硬编码（与现有项一致，前端 i18n 映射处理）

**验证：**
- [ ] `go test ./internal/usecase/... -run CheckPreconditions` 通过
- [ ] 新增单元测试覆盖：端口 nil / 设备未连接 / 设备已连接未采集 / 设备已采集 四种场景

**依赖：** 任务 1

**涉及文件：**
- `services/api-go/internal/usecase/traversal.go`（修改 `CheckPreconditions`，~30 行）
- `services/api-go/internal/usecase/traversal_view_test.go`（新增 4 个测试用例）

**预估范围：** 中（2 个文件）

---

#### 任务 4：`Start` 在启动 loop 之前主动调用 `StartAcquisition`

**描述：** 在 `traversal_config.go` 的 `ParseAndStartTraversal` 中，`m.Start(config)` 成功后、`go m.RunTraversalLoop()` 之前，若 `acquisitionController` 非 nil 且设备未在采集，调用 `StartAcquisition(config.DeviceID)`；失败直接返回 error 并清理状态。

**验收标准：**
- [ ] 端口非 nil 时检查 `IsAcquiring(config.DeviceID)`，已采集则跳过
- [ ] 未采集时调用 `StartAcquisition(config.DeviceID)`，失败返回 error（不启动 loop）
- [ ] 启动失败时调用 `m.Stop` 清理状态（避免遗留 Running 态）
- [ ] 端口为 nil 时保持原行为（向后兼容）
- [ ] 启动成功后 slog.Info 记录"traversal auto-started device acquisition"
- [ ] 错误信息可读：`auto-start acquisition failed for device %s: %v`

**验证：**
- [ ] `go test ./internal/usecase/... -run Start` 通过
- [ ] 新增单元测试覆盖：端口 nil / 已采集（跳过）/ 未采集启动成功 / 未采集启动失败 四种场景

**依赖：** 任务 1

**涉及文件：**
- `services/api-go/internal/usecase/traversal_config.go`（修改 `ParseAndStartTraversal`，~20 行）
- `services/api-go/internal/usecase/traversal_lifecycle_test.go` 或新测试文件（4 个测试用例）

**预估范围：** 中（2 个文件）

---

### 检查点：后端核心逻辑就绪
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go test ./...` 全绿
- [ ] 手动验证：未启动采集时点开始测试，确认对话框显示"设备未在采集"为红色；点确认后自动启动采集

---

### 阶段 3：前端适配

#### 任务 5：i18n 新增 key + 确认对话框文案映射

**描述：** 在 `i18nStore.ts` 中英文区新增 4 个 key：`checkDeviceConnected` / `checkDeviceNotConnected` / `checkDeviceAcquiring` / `checkDeviceNotAcquiring`；在 `TraversalStartConfirm.vue` 的 `MESSAGE_I18N` 映射表新增对应条目。

**验收标准：**
- [ ] i18nStore.ts 中文区新增 4 个 key，文案分别为："目标设备已连接" / "目标设备未连接，请先连接设备" / "目标设备正在采集" / "目标设备未在采集（点击开始后将自动启动）"
- [ ] i18nStore.ts 英文区新增 4 个 key，对应英文文案
- [ ] `TraversalStartConfirm.vue` 的 `MESSAGE_I18N` 映射表新增 4 条：后端硬编码英文 message → i18n key
- [ ] 启动确认对话框在中文/英文下均能正确显示新检查项文案

**验证：**
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过

**依赖：** 任务 3（后端 message 文案确定后才能映射）

**涉及文件：**
- `apps/desktop-wails/frontend/src/stores/i18nStore.ts`（4 个 key × 2 语言区，~8 行）
- `apps/desktop-wails/frontend/src/components/traversal/dialogs/TraversalStartConfirm.vue`（`MESSAGE_I18N` 新增 4 条，~4 行）

**预估范围：** 小（2 个文件）

---

#### 任务 6：确认对话框摘要区加"将自动启动设备采集"提示

**描述：** 在 `TraversalStartConfirm.vue` 摘要区（`currentConfig` 区块）加一行提示，仅当设备未在采集时显示。前端通过 `deviceStore` 查询 `config.deviceId` 的 `acquiring` 状态。

**验收标准：**
- [ ] 摘要区新增一行提示："ℹ 点击开始后将自动启动设备采集"（仅当设备未在采集时显示）
- [ ] 设备已在采集时不显示该提示（避免冗余）
- [ ] 设备未连接时不显示该提示（前置检查会拦截）
- [ ] 文案随 i18n 切换中英文

**验证：**
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] 手动验证：设备未采集时显示提示；设备已采集时不显示

**依赖：** 任务 5

**涉及文件：**
- `apps/desktop-wails/frontend/src/components/traversal/dialogs/TraversalStartConfirm.vue`（新增 ~15 行 template + computed）
- `apps/desktop-wails/frontend/src/stores/i18nStore.ts`（1 个新 key）

**预估范围：** 小（2 个文件）

---

### 检查点：前端就绪
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] 手动验证完整流程：连接设备但未采集 → 点开始测试 → 看到红色"设备未在采集"项 + 蓝色"将自动启动"提示 → 点确认 → 设备自动开始采集 → 测试正常进行

---

### 阶段 4：端到端验证

#### 任务 7：端到端验证与回归

**描述：** 全量验证：后端编译/测试/vet + 前端 typecheck/build/test + Wails binding 同步检查。

**验收标准：**
- [ ] `cd projects\wind-daq\services\api-go && go build -buildvcs=false ./...` 通过
- [ ] `cd projects\wind-daq\services\api-go && go test ./internal/... ./api/...` 全绿
- [ ] `cd projects\wind-daq\services\api-go && go vet ./...` 无 warning
- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend && npm run typecheck` 通过
- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend && npm run build` 通过
- [ ] `cd projects\wind-daq\apps\desktop-wails\frontend && npm run test` 通过（如有相关测试）
- [ ] Wails binding 签名未变（无新暴露方法），无需 `wails3 generate bindings`

**验证：**
- [ ] 上述 6 项命令全部通过
- [ ] 手动验证三种场景：① 设备未连接 → 前置检查拦截；② 设备已连接未采集 → 自动启动；③ 设备已采集 → 跳过自动启动

**依赖：** 任务 1-6 全部完成

**涉及文件：** 无（纯验证任务）

**预估范围：** 小（0 个文件）

---

## 风险与规避

| 风险 | 影响 | 规避 |
|---|---|---|
| 端口 nil 时旧装配路径行为变化 | 中 | 所有新增逻辑都用 `if m.acquisitionController != nil` 守卫，nil 时保持原行为 |
| `StartAcquisition` 是耗时调用（部分设备同步触发硬件） | 中 | 在 `ParseAndStartTraversal` 同步路径调用，失败立即可见；不在 Loop goroutine 中调用避免 setErrorLocked 竞态 |
| 设备已断开连接的窗口期（Connect/Disconnect 并发） | 低 | `DeviceManager.StartAcquisition` 已用 `connMu` 保护，无需额外处理 |
| 多设备场景（未来扩展） | 低 | 当前 `config.DeviceID` 是单设备，端口设计天然支持多设备，无需现在处理 |
| 前端 `deviceStore` 状态轮询延迟导致提示不准 | 低 | 提示文案措辞为"将自动启动"而非"已停止"，即使状态有 1-2 秒延迟也不误导 |

## 待解决问题

无。所有设计决策已确定，端口设计向后兼容，失败路径明确。
