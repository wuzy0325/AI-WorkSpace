# Spec: 遍历测试"设备停止采集"处理流程设计

状态：**设计稿 v4（统一"设备异常 = 类暂停无限期等待"，待评审）**
范围：`wind-daq services/api-go` + 前端遍历页（legacy + dual）
关联：`plan-traversal-auto-start-acquisition.md`、`spec-traversal-reliability-and-recovery.md`

## 概述

遍历测试运行期间，**任何设备采集异常（主动停采、意外掉线、未连接）都统一为"类暂停"的无限期等待**——不做任何时间预算、不自动判失败。设备是操作员负责的生命周期资源，恢复时机由操作员裁决；遍历只提供等待 + 明确的恢复指引，退出路径只有三种：**操作员恢复设备 / 暂停遍历 / 停止遍历**。

## 现状基线

| 阶段 | 位置 | 当前行为 |
|---|---|---|
| 遍历启动 | `ParseAndStartTraversal` / `StartManaged` | 任一引用设备未采集 → 拒绝启动（**既有行为，非本规格新增**） |
| 点位开始 | `RunCurrentPoint` | 任一设备未采集 → **立即失败**，整个遍历中止 |
| 点位采样中 | `collectAveragedSamples` | 进入等待恢复：连续未采集累计，恢复即清零；连续 ≥60s 判失败 |
| 移动/稳定阶段 | `waitForMotionComplete` / `waitForStabilization` | 不检查采集态，到采样阶段才感知 |
| 遍历暂停 | 暂停分支 | 无限期等待，所有超时绕过 |

超时参数（现状）：`acquisitionNotAcquiringTimeout = 60s`、`acquisitionStallTimeout = 10s`。

## 问题与不一致

1. **点位边界与采样中不对称**：点位之间停采 → 下一个点**立即失败**；采样中停采 → **60s 宽限**。同一动作、不同结局。
2. **主动停采被误判失败**：操作员主动停采是有意行为（类暂停），却被 60s 超时兜底判失败——停多久、何时恢复应由操作员决定，不应有时间上限。
3. **意外掉线也被误判失败**：掉线同样是操作员可恢复的状态（重连 + 重启采集），2s 快速失败会在网络瞬断等场景误杀整场测试（即最初 bug 报告的"概率性"失败根因）。
4. **无法区分根因**：`IsAcquiring` 只返回 bool，无法给出"重启采集"或"重新连接"的针对性指引。
5. **等待无可见性**：等待期间界面停留在"采集中"，没有"等待设备恢复采集"提示。
6. **等待恢复 freshness 基线不一致**：暂停分支已清 `fresh`/`pending`，但 not-acquiring 等待分支未清——恢复后首个样本可能混入停采前旧帧。

## 设计目标

- **设备异常 = 类暂停**：所有"设备未在采集"一律无限期等待，无任何时间预算、不自动判失败。恢复设备、继续测试的决定权完全交给操作员。
- **针对性指引**：按根因给出不同文案（停采 → 重启采集；掉线 → 重新连接并启动采集）。
- **可见**：等待恢复阶段前后端有明确指示（设备、原因、已等待时长）。
- **一致**：点位开始与采样中共享同一套等待语义；所有等待恢复路径统一重新建立 freshness 基线。
- **可实现**：设备分类只用现有 `DeviceManager.GetStatus` 可判定字段，端口以单次一致性快照暴露。
- **保留**：暂停 = 无限期中断的逃生口；`acquisitionStallTimeout` 仍兜底"在采但无新帧"这一**不可见**异常（**已决策：保持 10s 有界**，理由见下）。

## 设备采集状态模型（三态，仅用于文案与诊断，不驱动超时）

遍历视角区分三种可观察状态（不分 ERROR/ABSENT，设备掉线后被 `onError` 从 map 删除，与"未连接"语义一致——都需重连）：

| 状态 | 判别规则（`DeviceManager.GetStatus(id)` 一次调用结果） | 文案 | 遍历处置 |
|---|---|---|---|
| `ACQUIRING` | `Acquiring == true` | — | 正常采样 |
| `STOPPED` | `Acquiring == false` 且 `ok==true` 且 `Connection ∈ {Connected, Acquiring}` | "等待设备 X 恢复采集"（重启采集） | **无限期等待** |
| `RECONNECT_REQUIRED` | 其余全部：`ok==false`，或 `Connection==Error`，或 `Connection==Disconnected` | "设备 X 已断开，请重新连接并启动采集" | **无限期等待** |

判定顺序（单调、所有组合有确定归属，以设备适配器对 `Connection`/`Acquiring` 的契约为准）：

```
if !ok || Connection==Error || Connection==Disconnected  → RECONNECT_REQUIRED
if Acquiring                                             → ACQUIRING
else                                                     → STOPPED
```

注意：

- 先判 `Connection` 再判 `Acquiring`：`Connection==Error && Acquiring==true`（readLoop 退出后时序残留）按 `RECONNECT_REQUIRED`，Error 优先。
- `Connection==Acquiring && Acquiring==false`（异常组合）落入 `STOPPED`（可恢复等待，宁可等不可误杀），属适配器状态不一致诊断范畴，记录日志。
- 设备被 `onError` 删除后 `ok==false`，最近错误信息**不保留**。文案统一"设备 X 已断开，请重新连接并启动采集"；详细 LastError 保留列入后续增强（YAGNI，初版不做）。

## 等待恢复阶段（正式内部阶段）

新增 `PointPhase PhaseWaitingForAcquisition = "waiting_acquisition"`（`core/traversal/types.go`）。

**阶段语义**：

- `State` 保持进入等待前的子状态（`StateMoving/StateStabilizing/StateAcquiring`）不变——等待是横切阶段，**不新增顶层 State**，避免 `RunTraversalLoop` 状态机与前端状态机大改。
- `CurrentPointIndex` 保持当前点（**不推进**）。
- `CurrentPointPhase` 置为 `waiting_acquisition`，前端据此显示等待横幅。
- 恢复后回到**原阶段**：点位开始等待 → 继续正常点位流程（采集校验 → 运动 → 稳定 → 采样）；采样中等待 → 继续采样循环。

**恢复入口的 freshness 基线**：

- 采样中等待恢复：**清空 `fresh`/`pending`，保留 `lastTimestamps`**（与暂停分支语义统一），避免停采前旧帧并入恢复后首个样本。
- 点位开始等待：采样尚未开始，`lastTimestamps` 基线在采样入口建立，无额外处理。
- 恢复后重新对全部设备做一次分类：若仍有设备异常，继续等待（可能切换文案状态）。

**状态转换图**：

```
                 STOPPED / RECONNECT_REQUIRED 检出
  moving/stabilizing/acquiring ─────────────────▶ waiting_acquisition ──▶ 原阶段
        (点位开始或采样中)                           │        ▲
                                                    │        └─ 设备恢复（重启采集 / 重连+启动）
                                                    ├─ Pause ──▶ 语义等同暂停，无限期
                                                    └─ Stop ──▶ 立即退出（isTaskCancelled）
```

**等待循环行为**（与暂停分支同一模式）：

- 每 tick（10ms）重新分类全部设备：任一恢复 `ACQUIRING` → 退出等待，回原阶段。
- 持续响应 `isTaskCancelled`（Stop）与暂停（Pause → 冻结/旁路，Resume 后重新分类）。
- 无任何时间预算；不累计"未采集时长"；不触发超时失败。
- `stallDeadline`（10s 采样停滞）在等待期间重置，等待不计入采样停滞。

## 时间与优先级

**无设备异常时间预算**：`STOPPED` 与 `RECONNECT_REQUIRED` 均无限期等待，不存在 2s / 60s 超时，也就不存在"暂停冻结计时"问题。

**事件优先级**（同一轮询 tick 同时发生时）：

```
Stop > Pause > 设备恢复（ACQUIRING）> 继续等待
```

**暂停交互**：

- 等待中 Pause → 语义等同暂停（无限期），等待字段冻结展示。
- `Resume` 后重新分类全部设备：恢复 `ACQUIRING` → 继续测试；仍异常 → 回到等待。

**超时参数**：

| 超时 | 值 | 适用 | 语义 |
|---|---|---|---|
| `acquisitionNotAcquiringTimeout` | **移除**（原 60s） | — | 设备异常不再有自动失败 |
| `acquisitionDeviceErrorTimeout` | **移除**（原 2s 草案） | — | 同上 |
| `acquisitionStallTimeout` | 10s（现有，**已决策保持**） | `ACQUIRING` 但无新帧 | "在采却无数据"属**不可见**异常（设备显示正常），有界失败能把问题暴露给操作员，故不改为无限期等待 |

## 各阶段行为

| 阶段 | 设计后行为 |
|---|---|
| 遍历启动 | 保持"全部 `ACQUIRING` 才启动"（既有）；`STOPPED` 文案"请先启动采集"，`RECONNECT_REQUIRED` 文案"设备需重新连接" |
| 点位开始 | **统一为等待**（不再立即失败）：任一设备非 `ACQUIRING` → 无限期等待，按根因显示文案 |
| 点位采样中 | 任一设备非 `ACQUIRING` → 无限期等待（类暂停），恢复清 `fresh/pending` |
| 移动/稳定阶段 | 保持不检查；到采样阶段由上述规则接管 |
| 遍历暂停 | 不变：无限期等待，优先级最高 |

**点位开始改为等待的理由**：检查发生在运动下发之前，等待不浪费运动、只是延迟；消除"点位间停采立即终止测试"的不可恢复性，与采样中语义对齐。

## 多设备聚合规则

多设备混采时，每轮询 tick 对全部引用设备分类，**任何一台非 `ACQUIRING` 都进入等待**，按**优先级**决定文案主展示设备：

```
RECONNECT_REQUIRED > STOPPED > ACQUIRING
```

- 有 `RECONNECT_REQUIRED` → 主展示"设备 X 已断开，请重新连接并启动采集"。
- 无 `RECONNECT_REQUIRED` 但有 `STOPPED` → 主展示"等待设备 X 恢复采集"。
- 全部 `ACQUIRING` → 继续。
- 无论哪类，行为一致：无限期等待，直到全部恢复 / Pause / Stop。

**状态字段**：后端 `Status` 维护**设备状态列表**（内部完整信息），前端决定摘要展示：

```go
type AcquisitionDeviceStatus struct {
    Name    string `json:"name"`
    State   string `json:"state"`   // "acquiring" | "stopped" | "reconnect_required"
    SinceMs int64  `json:"sinceMs"` // 进入该状态的时间戳（epoch ms），0 表示未进入
}

// Status 新增字段：
WaitingForAcquisition        bool                      `json:"waitingForAcquisition,omitempty"`
WaitingDevices               []AcquisitionDeviceStatus `json:"waitingDevices,omitempty"`
WaitingForAcquisitionSinceMs int64                     `json:"waitingForAcquisitionSinceMs,omitempty"`
```

- 列表按 `deviceID` 字典序稳定排序（沿用 `traversal_devices.go` 的排序）。
- 前端横幅：显示主展示设备名 + "等 N 台设备"；文案按 `state` 区分（见状态模型表）。
- 已等待时长：前端用 `WaitingForAcquisitionSinceMs`（后端 epoch ms）按现有轮询节奏计算 `now - sinceMs`，单一时钟源（后端），暂停时横幅被暂停 UI 取代、不显示等待累计。

## 前后端数据链路影响清单

后端 `Status` 新增字段后，必须贯通以下全部链路（否则等待信息在进度事件重建时丢失）：

| 层 | 文件 | 改动 |
|---|---|---|
| Go Status | `internal/core/traversal/types.go` | `Status` 新增 `WaitingForAcquisition*` 字段；`PointPhase` 新增 `waiting_acquisition` |
| Go 分类 | `internal/usecase/traversal_devices.go` | 改按快照三态分类，返回设备列表 |
| Go 等待逻辑 | `internal/usecase/traversal_acquisition.go` | 抽 `waitForAcquisitionResume` 公共 helper（无限期 + Stop/Pause 响应 + 多设备聚合 + 清 `fresh/pending`）；点位开始复用；删除 `notAcquiringTotal`/`acquisitionNotAcquiringTimeout` 逻辑 |
| Go 端口 | `internal/ports/device.go` | 新增 `AcquisitionStatus(id)`（既有方法不动） |
| Go 装配/mock | `DeviceManager`（实现）+ 所有 `mockAcquisitionController`/`resumableAcquisitionController`（补新方法） | 见测试要点 |
| legacy status | `apps/.../src/api/traversalApi.ts` `getStatus()` | 后端字段映射进 `TraversalTestStatus` |
| legacy progress | `apps/.../src/api/traversalApi.ts` `onProgress()` | `TraversalProgressEvent` 增加等待字段 |
| legacy store | `apps/.../src/stores/traversalStore.ts` `applyProgressEvent()` / `syncRecoveredStatus()` | 保留/合并等待字段（当前会重建状态，需显式透传） |
| dual status/progress | `apps/.../src/stores/dualTraversalStore.ts` / `dualTraversalRuntime.ts` / `traversalPolling.ts` | 同 legacy 字段贯通 |
| 类型 | `apps/.../src/shared/types/traversal.ts` `TraversalTestStatus` / `TraversalProgressEvent` | 新增等待字段 |
| 展示 | 遍历页采集中组件 + `traversalErrorMapper.ts` + i18n 文案 + 等待横幅组件 | legacy 与 dual 各自组件/测试 |
| 测试 | legacy + dual store/组件/API 单测 | 见验收标准 |

## 实现改动汇总（现有端口调整，非新增接口）

| # | 文件 | 改动 |
|---|---|---|
| 1 | `internal/ports/device.go` | `AcquisitionController` **新增** `AcquisitionStatus(id) AcquisitionStatus`（既有 4 方法不动）；定义 `AcquisitionState`/`AcquisitionStatus` 类型 |
| 2 | `internal/usecase/device_manager.go` | 实现 `AcquisitionStatus`：一次 `GetStatus` → 三态映射 |
| 3 | `internal/core/traversal/types.go` | `PointPhase` 增 `waiting_acquisition`；`Status` 增等待字段 + `AcquisitionDeviceStatus` |
| 4 | `internal/usecase/traversal_devices.go` | 分类改三态、返回设备列表 |
| 5 | `internal/usecase/traversal_acquisition.go` | 抽 `waitForAcquisitionResume`（无限期 + Stop/Pause 响应 + 多设备聚合 + 清 `fresh/pending`）；采样 not-acquiring 分支改为调它；`RunCurrentPoint` 点位开始复用；**删除 `notAcquiringTotal` 累计与 60s 预算** |
| 6 | `internal/usecase/traversal.go` | 删除 `acquisitionNotAcquiringTimeout` 常量；现有 `RunCurrentPoint` 前置检查改调 helper |
| 7 | 前端链路 | 见上表 |

**启动拒绝、点位开始前检查是既有行为**，本规格不新增，仅统一其判定来源与文案。

## 测试要点

- 三态分类映射：`ok=false` / `Connection==Error` / `Disconnected` / `Connected&&!Acquiring` / `Acquiring` / `Connection==Acquiring&&!Acquiring` 各组合 → 期望状态
- `STOPPED` **无限期等待**：长时间不恢复不判失败；重启采集后恢复继续，`fresh/pending` 已清
- `RECONNECT_REQUIRED` **同样无限期等待**：重连 + 重启采集后恢复；不触发任何自动失败
- 点位开始任一设备异常 → 等待恢复后继续，不再立即失败
- 多设备：一台 `RECONNECT_REQUIRED` + 一台 `STOPPED` → 等待，主展示文案为 `RECONNECT_REQUIRED` 设备
- 等待中 Stop → 立即退出；等待中 Pause → 冻结，Resume 后重新分类
- `Status.WaitingForAcquisition*` 字段在等待/恢复/暂停各阶段正确切换
- mock 更新：所有 `AcquisitionController` mock 补 `AcquisitionStatus`

## 验收标准

- [ ] 端口仅新增 `AcquisitionStatus`，既有方法签名不变；`ports` 无实现、`usecase` 无硬件依赖约束保持
- [ ] 三态分类判定单调、覆盖全部 `Connection`/`Acquiring` 组合
- [ ] `STOPPED` 与 `RECONNECT_REQUIRED` 均无限期等待，**无 60s / 2s 自动失败**
- [ ] 点位开始与采样中共享同一等待 helper；所有等待恢复路径统一清 `fresh/pending`
- [ ] `Status.WaitingForAcquisition*` 贯通 legacy + dual 全链路（status + progress + store）
- [ ] 事件优先级 `Stop > Pause > 设备恢复 > 继续等待`；等待中 Stop 即时退出、Pause 冻结
- [ ] 错误/等待文案本地化（`traversalErrorMapper` + i18n），等待横幅在 legacy 与 dual 均显示，文案按 `stopped` / `reconnect_required` 区分
- [ ] 验收命令（全部运行）：
  - `go build ./...` + `go vet ./internal/usecase/`
  - `go test ./internal/... ./api/...`
  - `go test -race ./internal/usecase/ -run "Acquisition|Pause|Traversal"`
  - `npm run typecheck` / `npm run test` / `npm run build`（frontend）
- [ ] 双探针并行：两 probe 各自的等待状态互不影响

## 开放决策（需产品确认）

| # | 决策 | 推荐 | 说明 |
|---|---|---|---|
| 1 | 单点因设备异常等待后失败（仅 Stop）→ 整个遍历中止 | 保持**中止** | 跳过/重试涉及点位表状态、CSV 完整性、运动回退，属更大的产品决策，不在本文范围 |
| 2 | `STOPPED` / `RECONNECT_REQUIRED` 无限期等待无兜底 | 无限期（采纳） | 操作员是恢复时机唯一裁决者，类暂停；如担心操作员遗忘，可另加长兜底（如 30min），默认不加 |
| 3 | 点位开始从"立即失败"改为"等待" | 采用 | 行为变化：点位间停采不再立即终止测试 |
| 4 | 设备移除后的 LastError 保留 | 初版不保留 | 需在 `DeviceManager` 增加"最近断开"内存表，YAGNI 初版不做 |

> 已决策（v4 评审采纳）：**`acquisitionStallTimeout`（在采却无新帧，10s）保持有界**，不改为无限期等待。该异常**不可见**（设备状态显示正常"采集中"，操作员无从感知），有界失败能把问题暴露出来。若未来产品要求彻底统一为等待，需把该场景纳入等待横幅（文案"设备 X 在采集但无数据"），不在本文范围。

## 已知限制

- 设备被 `onError` 移除后，`AcquisitionStatus.LastError` 为空，文案统一"已断开，请重新连接并启动采集"，不含具体原因。
- `ACQUIRING` 但无新帧（驱动/通道配置错误）仍走 `acquisitionStallTimeout`（10s）有界失败，不进入等待横幅。
- 等待横幅的时长按后端 `sinceMs` 计算，展示刷新节奏与现有轮询一致；暂停期间横幅不显示。
