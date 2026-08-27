# 计量设备选择与阀门/单位统一方案（Drawer 重构）

> 状态：已实施（2026-08-25）
> 创建：2026-08-25
> 上游背景：多设备计量场景下，设备选择与阀门/单位设置混在同一张固定卡片中，存在以下问题：
> 1. 设备选择、连接、阀门/单位/校零全部塞进 `MeasurementDevicePanel` 一张卡片，上下空间占用大，挤占侧栏其它区块（打压设备、启动条件）。
> 2. 卡片标题与侧栏标题写死 `1604`，未按设备类型命名，新增型号需改 UI。
> 3. 详情区只取 `selectedDeviceIds[0]`，多设备时其余设备的阀门/单位无法查看，隐含"只设置了首台"的缺陷。

---

## 1. 目标与范围

- 将"设备选择"与"设备详情"从侧栏固定卡片中拆出，改为**右侧滑出 Drawer**，侧栏仅保留一行入口按钮。
- 消除对具体型号（1604）的硬编码，改为按设备类型（"计量采集设备"）命名。
- 明确**阀门/单位为统一语义（一改全改）**，并正确处理"多台设备连接后阀门/单位可能不一致"的场景：逐台回读、差异高亮、一键统一。

### 非目标

- 不改动计量采集后端核心流程（采集、稳定判定、报警、报告）。
- 不改动打压设备区块（已有"列表 + 连接"结构，维持现状）。
- 不改动校零的逐通道语义（仍按单台设备 + 启用通道执行，多设备统一校零留待后续迭代）。

---

## 2. 核心语义定调（关键决策）

| 项 | 语义 | 说明 |
|---|---|---|
| 阀门写 | **统一，一改全改** | 复用后端已实现的 `SetValveStatusAllDevices`，逐台下发，任一失败即报错并携带 deviceId |
| 单位写 | **统一，一改全改** | 新增后端 `SetMeasureUnitAllDevices`，逐台写当前会话绑定的计量设备，写后回读确认 |
| 阀门读 | **逐台回读 + 差异检测** | 新增批量读端点，返回 `{ deviceId: status }`，前端按设备展示差异 |
| 单位读 | **逐台回读 + 差异检测** | 新增批量读端点，返回 `{ deviceId: unit }`，前端按设备展示差异 |
| 单位一致性 | **硬门禁** | 仅检查当前会话绑定的计量设备及打压设备；不一致时拦截启动，Drawer 统一计量设备后仍需由打压设备区对齐打压单位 |
| 阀门一致性 | **硬门禁（校准态）** | 由 `SetValveStatusAllDevices` 保证整批一致；启动门禁要求阀门处于校准模式 |

---

## 3. 现状勘察结论

### 3.1 后端已具备逐台读/写能力（无需大改）

`internal/application/session/service.go` 已实现：

- `ReadValveStatusForDevice(ctx, token, deviceID)`
- `SetValveStatusForDevice(ctx, token, deviceID, status)`
- `SetValveStatusAllDevices(ctx, token, status)`（按 `measureDevIDs` 逐台下发）
- `ReadMeasureUnitForDevice(ctx, token, deviceID)`（读后同步设备配置存储）
- `SetMeasureUnitForDevice(ctx, token, deviceID, unit)`

现有 `ReadValveStatus` / `SetValveStatus` / `ReadMeasureUnit` / `SetMeasureUnit` 均为"首个设备"的兼容入口（`deviceID=""`）。

### 3.2 缺口：HTTP 层未暴露"批量读"

`internal/api/http/device_session_handler.go` 目前仅暴露单设备端点：

- `sessionGetValveHandler` → `ReadValveStatus`（首台）
- `sessionSetValveHandler`（首台）

**已补齐**：批量读"所有绑定计量设备的阀门/单位"端点；单台读取失败保留在该设备结果中，不影响其它设备结果。

### 3.3 前端现状

- `MeasurementDevicePanel.vue`：设备多选（`el-checkbox-group`）+ 连接按钮 + 详情区（阀门/单位/校零），详情区全部取 `selectedDeviceIds[0]`。
- `MeasurementSidebar.vue`：标题写死 `1604 计量设备`（含折叠态 aria-label），计量设备 section 直接内嵌 `MeasurementDevicePanel`。
- `stores/measurement/index.ts`：`valveStatus` / `measureUnit` 为**单一值**，`apiReadValveStatus()` / `apiSetValveStatus(status)` / `apiReadMeasureUnit()` / `apiSetMeasureUnit(unit)` 均不带 deviceId。
- `MeasurementDataView.vue`：已有多设备 tab（`deviceTabs` / `activeDeviceId`）切看各设备通道数据，保留。

---

## 4. 布局方案（Drawer）

```
┌─ 侧栏（变瘦）───────────────────┐
│ [📟 计量采集设备]  2 台已选 ▸   │  ← 一行入口按钮，点击打开 Drawer
│ [💉 打压设备]       已连  ▸     │
│ [✓ 启动条件]                    │  ← 不再被挤下去
└──────────────────────────────┘
        │ 点击"计量采集设备"
        ▼
┌─ Drawer：计量采集设备（右侧滑出，宽 ~420px）───────────────┐
│ 顶部条：已选 N 台 · 单位 ● 一致 / ⚠ 不一致                    │
│        [连接选中的 N 台]  [断开]                              │
│                                                              │
│ ┌─ 阀门（统一）───────────────────────────────────────┐      │
│ │  状态：● 一致(校准)  /  ⚠ 不一致                      │      │
│ │  设备A: 校准   设备B: 测量   设备C: 校准   ← 差异标红 │      │
│ │  [校准模式] [测量模式] [复位]      ← 一改全改          │      │
│ └──────────────────────────────────────────────────────┘      │
│ ┌─ 单位（统一）───────────────────────────────────────┐      │
│ │  ● 一致(kPa)  /  ⚠ 不一致                            │      │
│ │  设备A: kPa   设备B: MPa   ← 差异标红                 │      │
│ │  统一为 [kPa ▾]   [统一并应用]                        │      │
│ └──────────────────────────────────────────────────────┘      │
│ ┌─ 设备选择（多选）───────────────────────────────────┐      │
│ │  ☐ 设备A [已连] ☐ 设备B [已连] ☐ 设备C [未连·置灰]    │      │
│ └──────────────────────────────────────────────────────┘      │
└──────────────────────────────────────────────────────────────┘
```

### 区块说明

1. **顶部条**：已选数量 + 单位一致性状态圆点 + 连接/断开按钮。
2. **阀门区块（统一）**：显示整体一致性，逐台列出各设备实际阀门状态（差异标红），三个按钮（校准/测量/复位）为全局统一写。
3. **单位区块（统一）**：显示整体一致性，逐台列出各设备实际单位（差异标红），"统一并应用"按钮批量对齐。
4. **当前设备区块**：聚焦 P1603 时提供逐设备校零，聚焦有阀设备时提供逐设备复位。
5. **设备选择区块**：多选列表；连接后锁定选择，断开后可重新配置。

---

## 5. 不一致的处理策略

### 5.1 连接后自动逐台回读

连接完成（`handleMeasureDeviceConnect` 编排结束）后调用新增的批量读端点，得到 `{ deviceId: value }` map。

### 5.2 差异呈现（不阻塞、不静默）

- 顶部状态圆点：`● 一致` / `⚠ 不一致`。
- 差异设备在阀门/单位区块内**标红高亮**，明确"哪台不同、值是多少"。

### 5.3 统一动作

- **阀门**：点「校准模式/测量模式」→ `SetValveStatusAllDevices`，改完回读确认全部一致。
- **单位**：选目标单位点「统一并应用」→ 后端逐台 `SetMeasureUnitForDevice`，改完回读确认。
- 批量写不是事务：中途失败时已成功设备保持新状态，错误必须携带失败 `deviceId`，UI 随后回读并展示实际结果。

### 5.4 启动门禁

- 单位不一致 → `GET /api/v1/session/unit-consistency` 只检查当前会话绑定集合，`canStart` 为 false，`startBlockedReason` 返回"设备压力单位不一致，请先统一设备单位"。
- 阀门不一致/非校准 → 复用 `enforceValveCalibrationGate` 门禁，提示"请先将阀门切换到校准模式"。

---

## 6. 改动清单

### 6.1 后端（小改）

1. `internal/api/http/device_session_handler.go`：
   - 新增 `GET /api/v1/session/valve/all` → 逐台读取，返回 `{ devices: { deviceId: { value?, error? } } }`。
   - 新增 `GET /api/v1/session/measure-unit/all` → 逐台读取，返回相同的逐设备结果结构。
   - 新增 `POST /api/v1/session/measure-unit/all` → 统一写当前会话绑定的全部计量设备。
   - 新增 `GET /api/v1/session/unit-consistency` → 检查当前会话绑定设备，不包含无关已连接设备。
   - 新增 `POST /api/v1/session/measure-device/unbind` → 断开全部计量设备时释放会话模块所有权。
   - `calibrate-zero` / `reset` 请求新增可选 `deviceId`，空值保持首台兼容语义。
   - `router.go` 注册新路由。
2. 保留原单设备端点作为首台兼容入口，不改变已有调用方。

### 6.2 前端

1. `api/session.ts`：新增 `readValveStatusAll()` / `readMeasureUnitAll()`；新增/确认 `setValveStatusAll()` / `setMeasureUnitAll()`（或逐台循环封装）。
2. `stores/measurement/index.ts`：
   - 新增 `valveStatusByDevice`、`measureUnitByDevice` 及对应错误 map。
   - 新增共享 `activeDeviceId`，Drawer 与数据 tab 使用同一聚焦设备。
   - 新增 `refreshValveStatusAll()` / `refreshMeasureUnitAll()` / `setValveAll(status)` / `setUnitAll(unit)`。
   - `setValveAll` / `setUnitAll` 内部逐台写 + 回读校验。
3. 新建 `components/measurement/MeasurementDeviceDrawer.vue`：承载上述四个区块，接收 `visible` / 连接事件，向上抛出 `connect` / `disconnect` / `unit-change`。
4. `components/measurement/MeasurementSidebar.vue`：
   - 计量设备 section 缩成一行入口按钮（显示已选台数），点击打开 Drawer。
   - 标题 `1604 计量设备` → `计量采集设备`；折叠态 aria-label 同步修改。
5. `components/measurement/MeasurementDataView.vue`：使用 store 的 `activeDeviceId`，绑定集合变化时自动回退首台。
6. `MeasurementDevicePanel.vue` 暂时保留供历史引用；计量侧栏已不再渲染。

---

## 7. 影响与风险

| 项 | 风险 | 应对 |
|---|---|---|
| 后端批量读端点新增 | 需与前端 store 同步，否则首屏差异检测缺失 | 实现时前后端同一迭代提交 |
| `valveStatus` / `measureUnit` 单值 → map | 现有多处依赖单值字段（遥测、门禁） | 保留首台兼容视图；启动阀门门禁优先校验全部绑定设备 map |
| 批量读部分失败 | 空 map 容易被误判为一致 | 每台返回 `{ value?, error? }`，有错误即显示未知/失败并禁止判为一致 |
| 批量写部分成功 | 硬件操作无法事务回滚 | 错误携带 `deviceId`，保留已成功状态并立即批量回读 |
| Drawer 引入后 `MeasurementDevicePanel` 弃用 | 其它页面可能引用该组件 | 实施前先 `gitnexus_impact` 检查引用面，再决定删除或保留桥接 |
| 单位不一致硬门禁 | 用户可能被"卡住"不知如何统一 | Drawer 内提供醒目"统一并应用"入口 + 明确差异提示 |

---

## 8. 验收标准

- [ ] 侧栏计量设备 section 缩为一行入口，点击打开 Drawer，标题不再出现"1604"。
- [ ] Drawer 内多选列表可多台勾选、连接/断开。
- [ ] 连接后逐台回读阀门/单位，不一致时差异设备标红高亮。
- [ ] 阀门「校准/测量」一改全改，改后回读一致。
- [ ] 单位「统一并应用」批量对齐，改后回读一致。
- [ ] 单位不一致时"开始计量"被拦截，并提示统一单位。
- [ ] `MeasurementDataView` tab 跟随 Drawer 聚焦设备联动。
- [ ] P1603 校零和有阀设备复位仅作用于 Drawer 当前聚焦设备。
- [ ] 未绑定但已连接的其它设备单位不影响当前会话启动门禁。
- [ ] 批量读取单台失败时其它设备仍展示实际值，失败设备明确显示错误。
- [ ] `go build ./...` / `go test ./...` / `go vet ./...` 全绿；`npm run typecheck` / `lint` / `build` 全绿。
