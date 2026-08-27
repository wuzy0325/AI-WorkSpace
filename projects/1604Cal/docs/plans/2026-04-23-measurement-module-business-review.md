# 计量模块业务逻辑复核与后续开发建议

> 日期：2026-04-23  
> 复核对象：`Cal1604` 当前代码库  
> 参考基线：`1605MeassureApp` 既有业务流程、`docs/计量业务逻辑参考说明.md`

---

## 1. 复核范围

本次复核聚焦“计量业务主链”是否真正落地，而不是只看接口或页面是否存在。

纳入复核的范围：

- 设备接入与会话绑定
- 计量页面参数配置与采集流程
- 标定页面中承载的自动/手动采集主链
- 单位一致性检查
- 报警判定与人工决策闭环
- 报告导出链路
- 前后端事件同步与页面可观测性

说明：当前新系统中，旧系统“计量业务主链”被拆散到了 `measurement`、`calibration`、`session`、`multipress` 四块实现里，因此本次结论按业务链路而不是按目录名判断。

---

## 2. 总体结论

当前项目**已经具备设备连接、基础采集、自动采集编排、WTN1604/ConST 协议命令、基础报告导出能力**，但距离“旧系统那套可稳定交付现场业务的计量模块”仍有明显差距。

当前状态更准确的判断是：

- **底层协议与基础服务：已具备骨架，部分能力已落地。**
- **业务闭环：只完成了一部分，且有若干关键断点停留在‘后端做了、前端没接上’或‘页面有了、后端没真正执行’的阶段。**
- **计量页面本身：更接近“实时采样看板”，还不是旧系统意义上的“完整计量作业台”。**

如果按是否可以完整支撑“设备准备 -> 参数配置 -> 自动/手动采集 -> 报警处理 -> 导出报告”来判断，当前结论为：

- **单机接入：基本可用**
- **自动采集主链：后端已成型，但前端同步与可视化不足**
- **手动采集主链：可执行，但状态机与配置持久化不足**
- **报警闭环：后端有框架，但默认配置和前端交互仍有缺口**
- **报告导出：后端已有基础导出，但页面端尚未形成可交付闭环**

---

## 3. 业务覆盖矩阵

| 业务能力 | 当前状态 | 结论 | 关键证据 |
| --- | --- | --- | --- |
| 单设备新增/编辑/连接/断开 | 已实现 | 已落地 | `internal/api/http/router.go:170-175`，`web/src/components/device/DeviceManagementPanel.vue:9-28,203-225` |
| 共享设备会话绑定 | 已实现 | 已落地 | `internal/application/session/service.go:62-112`，`internal/api/http/device_session_handler.go:48-100` |
| 计量实时采样 | 已实现 | 已落地 | `internal/application/measurement/service.go:82-129,266-330` |
| 计量页面参数配置 | 页面有、后端未真正消费 | 部分落地 | `web/src/views/MeasurementView.vue:137-197,360-383`，`internal/api/http/measurement_handler.go:10-48` |
| 计量页面按测点执行业务闭环 | 未实现 | 缺失 | `measurement/start` 仅接收 `channels`，未接收压力点/稳定时间/报警配置：`internal/api/http/measurement_handler.go:10-48` |
| 自动采集编排 | 后端已实现 | 部分落地 | `internal/application/calibration/service.go:330-415,417-504` |
| 自动采集进度同步到前端 | 事件未消费完整 | 部分落地 | 后端发布 `calibration.point_status` / `point.completed`，前端 `useCalibrationSync` 未消费：`internal/application/calibration/service.go:502,714-730`，`web/src/composables/useCalibrationSync.ts:110-129` |
| 手动打压/手动采集 | 已有 API 与组件 | 部分落地 | `internal/api/http/router.go:220-221`，`web/src/components/calibration/ManualControlPanel.vue:54-84` |
| 单位一致性检查 | 仅基于配置快照 | 部分落地 | `internal/device/manager/device_manager.go:76-114`，`internal/application/deviceconnect/service.go:132-170` |
| 报警判定 | 已实现 | 已落地 | `internal/workflow/alarm_service.go:63-99` |
| 报警确认/重采/跳过/停止 | 后端支持，前端只接了一部分 | 部分落地 | `internal/application/calibration/service.go:506-596`，`web/src/components/calibration/CalibrationControl.vue:46-61` |
| 报警默认配置可直接生效 | 未实现 | 缺失 | 默认 `EnabledChannels=[]`，后端按空列表不检查任何通道：`internal/config/app_config.go:113-119`，`internal/workflow/alarm_service.go:81-97` |
| Excel 报告导出 | 后端已有基础能力 | 部分落地 | `internal/report/report_service.go:38-134` |
| 报告导出页面闭环 | 页面未打通 | 缺失 | `web/src/components/calibration/ReportExportDialog.vue:58-81` 存在，但 `CalibrationView.vue` 未挂载该组件：`web/src/views/CalibrationView.vue:20-35` |
| 批量连接/断开 | 未实现 | 缺失 | 路由仅单设备接口：`internal/api/http/router.go:170-175`；设备管理页面无批量按钮：`web/src/components/device/DeviceManagementPanel.vue:9-28` |

---

## 4. 核心问题清单

以下问题按优先级排列，优先处理真正阻断业务闭环的项。

### P0-1 计量页面参数并没有真正驱动后端采集逻辑

**现象**

- 计量页面提供了 `最小值 / 最大值 / 测点数 / 平均次数 / 稳定时间 / 控制模式 / 打压模式` 等参数。
- 但 `measurement/start` 接口只上传 `channels`，后端 `measurement.Service` 也只做周期性 `ReadMeasureData()` 采样。
- 页面上的压力点列表与进度，只是前端通过 `buildPointTargets()` 本地计算出来的展示结果。

**证据**

- `internal/api/http/measurement_handler.go:10-48`
- `internal/application/measurement/service.go:82-129,266-330`
- `web/src/views/MeasurementView.vue:137-197,255-291,360-383`

**影响**

- 计量页面当前不是“按目标压力点执行的业务作业台”，只是“实时采样页”。
- 页面上的测点数、回程模式、稳定时间，对后端采集链路没有约束力。
- 用户会误以为自己在跑完整业务流程，实际只是在持续采样。

**改善建议**

1. 明确计量页面定位：
   - 方案 A：保留为“实时采样页”，删除会误导用户的测点/打压业务参数。
   - 方案 B：升级为真正的计量作业台，复用校准服务的测点、稳定、报警与导出主链。
2. 如果选择方案 B，需新增统一的 `measurement session config` 或直接复用 `calibration` 编排服务，避免两套近似但不一致的状态机并存。

### P0-2 计量页面恢复采集会丢失前端已显示的数据

**现象**

- 后端 `measurement.Service.Start()` 在 `paused` 状态恢复时会保留历史采样行。
- 但前端 `measurementStore.start()` 每次调用都会把 `rows.value = []` 清空。
- `MeasurementView` 的“恢复”按钮实际调用的仍是 `start()`。

**证据**

- 后端保留数据：`internal/application/measurement/service.go:93-99,108-129`
- 前端清空数据：`web/src/stores/measurement/index.ts:137-147`
- 恢复逻辑复用 start：`web/src/views/MeasurementView.vue:315-323`

**影响**

- 暂停后恢复，前端会丢掉之前已采到的样本行，导致进度和数据视图断裂。
- 后端与前端状态不一致，现场排障会非常困难。

**改善建议**

1. 增加独立 `measurement/resume` 接口，前端不要再复用 `start()`。
2. 或至少在 `start()` 时按场景区分“首次启动”和“从 paused 恢复”。
3. 恢复后主动调用一次 `measurement/data` 重新同步历史数据。

### P1-1 单位一致性检查仍然停留在配置快照层，没有读取运行期真实单位

**现象**

- `/checks/unit-consistency` 调用的是 `DeviceManager.CheckUnitConsistency()`。
- 该逻辑只比较 `domain.Device.Unit` 字段。
- 设备连接成功后，连接服务不会把驱动真实读取到的单位回写到 `DeviceManager`。
- `multipress` 注册时虽然读取了单位，但只保存在自己内部状态里，也没有同步到设备存储。

**证据**

- `internal/device/manager/device_manager.go:76-114`
- `internal/application/deviceconnect/service.go:132-170`
- `internal/application/multipress/service.go:114-131`
- `web/src/components/measurement/MeasurementSidebar.vue:150-159`

**影响**

- 页面看到的“单位一致/不一致”可能是旧值，不是设备当前真实值。
- 如果现场手动改过设备单位，系统可能继续放行错误采集。

**改善建议**

1. 抽出“运行期单位检查服务”，优先读取活动驱动的实际单位。
2. 仅当设备未连接时，才回退到配置快照。
3. 在检查结果中区分：`pressureUnit`、`measureUnits`、`baselineSource`，便于 UI 解释问题。

### P1-2 计量页面的报警设置只是本地 UI 状态，没有接入任何后端逻辑

**现象**

- 计量页面有“启用/声音/报警确认/通道选择”UI。
- 但这些值只是 `MeasurementControl.vue` 内部的 `ref`，没有 emit，没有 API，没有写入 store。
- `measurement.Service` 也没有任何报警判定逻辑。

**证据**

- `web/src/components/measurement/MeasurementControl.vue:74-92,205-208`
- `internal/application/measurement/service.go:266-330`

**影响**

- 用户以为已经配置了报警，但实际采集过程中不会生效。
- 这是典型的“UI 存在但业务未落地”的误导性功能。

**改善建议**

1. 如果计量页面不承担完整业务闭环，移除这组控件。
2. 如果要保留，必须接到统一的报警配置与采集判定服务上。

### P1-3 报警默认配置下实际上不会检查任何通道

**现象**

- 默认报警配置 `EnabledChannels` 为空数组。
- 后端 `EvaluateMultiChannel()` 只遍历 `EnabledChannels`，空数组意味着不检查任何通道。
- UI 却把“空值”表达成“全部通道”。

**证据**

- 默认配置：`internal/config/app_config.go:113-119`
- 后端判定：`internal/workflow/alarm_service.go:81-97`
- UI 文案：`web/src/components/calibration/AlarmConfigPanel.vue:19-23`

**影响**

- 默认配置下，报警功能名义上开启，实际永远不触发。
- 现场很难第一时间发现该问题，因为页面看起来是“已启用”。

**改善建议**

1. 统一规则：空数组明确表示“全部通道”。
2. 后端进入 `EvaluateMultiChannel()` 时，如 `EnabledChannels` 为空，自动展开为当前采集通道列表。
3. 在 UI 上显示“未选择任何通道 = 默认全选”或显式填充 1..16。

### P1-4 `ConfirmOnAlarm` / `SoundEnabled` 还没有真正参与报警流程

**现象**

- 报警配置结构里有 `ConfirmOnAlarm` 和 `SoundEnabled`。
- 但 `checkAlarm()` 触发报警后会无条件进入 `await_alarm_resolution` 并阻塞等待用户决策。
- 声音开关也没有任何实际执行路径。

**证据**

- 配置定义：`internal/domain/alarm.go:3-10`
- 报警处理：`internal/application/calibration/service.go:528-596`

**影响**

- 配置项与实际行为不一致。
- 旧系统中的“自动继续/是否确认/声音提示”策略没有完整迁移。

**改善建议**

1. `ConfirmOnAlarm=false` 时，后端应直接按默认动作继续或按配置策略自动处理。
2. 声音提示交给前端也可以，但必须有明确事件字段让前端执行。

### P1-5 自动采集后端已经在跑，但前端没有消费关键事件，进度与测点状态不同步

**现象**

- 后端自动采集会发布 `calibration.point_status`、`point.completed`、`point.skipped`、`data.collected` 等事件。
- 但 `useCalibrationSync()` 只处理了 `session.state.changed`、`device.status.changed`、`calibration.stability.*`、`alarm.triggered`。
- 标定页面的测点表仍主要依赖本地 store 更新，自动模式下不能完整反映后端真实执行结果。

**证据**

- 事件发布：`internal/application/calibration/service.go:419,484,494,502,714-730`
- 前端监听：`web/src/composables/useCalibrationSync.ts:110-129`

**影响**

- 后端自动采集完成了，前端表格可能还停留在旧状态。
- 跳点、重采、自动完成等状态无法稳定展示。

**改善建议**

1. 在 `useCalibrationSync()` 中消费 `calibration.point_status` 与 `data.collected` 事件。
2. 或在收到 `session.state.changed` 后按节流方式重新拉取 `/calibration/points`。
3. 前端测点表应以“后端点位状态”为准，不再主要依赖本地即时改写。

### P1-6 标定参数持久化链路没有真正闭合，`pressureMode` / `controlMode` 易失真

**现象**

- `useConfigPersistence()` 只在 `onMounted` 时加载配置，没有真正监听参数变化后自动保存。
- `toPayload()` 还把 `controlMode` 写死成 `auto`。
- `pressurePointStore.generatePressurePoints()` 调用 `setCalibrationConfig()` 时又遗漏了 `pressureMode`。

**证据**

- `web/src/composables/useConfigPersistence.ts:22-33,86-112`
- `web/src/components/calibration/CalibrationParams.vue:97-108`
- `web/src/stores/calibration/pressurePoints.ts:63-71`

**影响**

- UI 改成手动模式或回程模式后，后端配置不一定同步。
- 页面上看到的是回程，后端生成的可能还是单程点。

**改善建议**

1. 统一“配置变更 -> 持久化 -> 同步后端运行态”的链路。
2. `pressureMode`、`controlMode` 不要再分散在多个地方各自组包。
3. 由一个统一 store/action 负责写配置并生成压力点。

### P1-7 报告导出后端已有基础，但页面端尚未形成可交付闭环

**现象**

- 后端已实现模板匹配、模板列表、导出 Excel。
- 前端已有 `ReportExportDialog.vue`，但当前 `CalibrationView.vue` 没有挂载它。
- 计量页面中“报告模板”按钮还是禁用状态，只支持前端本地 CSV 导出。

**证据**

- 后端导出：`internal/report/report_service.go:38-189`
- 导出组件：`web/src/components/calibration/ReportExportDialog.vue:58-81`
- 页面未挂载：`web/src/views/CalibrationView.vue:20-35`
- 计量页仅 CSV：`web/src/components/measurement/MeasurementDataView.vue:9-21`，`web/src/views/MeasurementView.vue:387-406`

**影响**

- 导出能力停留在“服务端有基础、用户端不可完整使用”的状态。
- 无法替代旧系统标准报表导出流程。

**改善建议**

1. 先在标定页面打通“选择模板 -> 选择路径 -> 导出 -> 结果反馈”的闭环。
2. 计量页面若不支持标准报告，就不要保留“报告模板”入口文案。

### P2-1 批量连接/断开能力仍缺失

**现象**

- 设备管理页只有单设备“连接/断开”按钮。
- 路由也只有单设备 connect/disconnect。

**证据**

- `internal/api/http/router.go:170-175`
- `web/src/components/device/DeviceManagementPanel.vue:203-225`

**影响**

- 多设备现场开工前准备效率低。

**改善建议**

1. 增加批量连接/断开 API。
2. 前端支持“全部连接 / 仅计量 / 仅打压”三种入口。

### P2-2 WTN1604 设备信息仍不完整

**现象**

- 当前只读取 `commTest`、`model`、`version`。
- 缺少通道数、序列号、固件版本等更完整的信息结构。

**证据**

- `internal/infrastructure/driver/wtn1604_driver.go:165-178`

**影响**

- 设备履历、报表头信息、现场核验能力不足。

**改善建议**

1. 统一设备信息 DTO。
2. 补全 WTN1604 协议读取命令并向前端暴露结构化字段。

### P2-3 前端测试环境当前不稳定，影响后续重构验证

**现象**

- `go test ./internal/...` 通过。
- `web` 下 `npm test` 失败，当前失败集中在 `DeviceManagementPanel.test.ts`，同时存在多个 Element Plus 组件解析 warning 与相对 URL fetch warning。

**影响**

- 后续修复业务逻辑时，前端回归验证可信度不足。

**改善建议**

1. 先修复测试基座：Element Plus stubs、fetch base、dialog 渲染断言。
2. 再进入大范围业务联调改造。

---

## 5. 建议的下一步开发顺序

建议按下面顺序推进，而不是并行散改。

### 第一阶段：先把“真假业务入口”理顺

1. 明确计量页面定位：实时采样页 or 完整业务页。
2. 若维持实时采样页：删掉未生效的测点/报警/模板文案。
3. 若升级为完整业务页：直接复用标定主链，不再维护第二套近似流程。

### 第二阶段：补齐关键闭环

1. 修复单位一致性为运行期真实值校验。
2. 修复报警默认通道为空不生效的问题。
3. 让 `ConfirmOnAlarm` 真正控制是否阻塞等待用户。
4. 打通自动采集事件到前端测点视图的同步。

### 第三阶段：补配置一致性与导出

1. 收敛 `controlMode/pressureMode` 的唯一配置入口。
2. 修复回程模式生成点位失真问题。
3. 挂载并打通报告导出对话框。

### 第四阶段：补效率与可维护性

1. 增加批量连接/断开。
2. 完善设备信息 DTO。
3. 修前端测试基座，补自动化回归。

---

## 6. 建议的验收标准

后续每个阶段完成后，至少满足以下验收点：

- 用户在页面上能看到的配置，后端必须真实消费。
- 后端状态变化，前端必须能同步看到，而不是依赖本地猜测。
- 默认配置不能出现“UI 显示已启用，实际完全不生效”的情况。
- 导出能力必须形成完整用户闭环，而不只是后端接口存在。
- 关键链路需要有至少一条自动化验证：
  - Go：`go test ./internal/...`
  - Web：`npm test`

---

## 7. 最终判断

当前 `Cal1604` 的计量业务不是“完全没做”，而是已经完成了**协议层、基础服务层、部分编排层**，但**真正能替代旧系统现场作业的业务闭环还没有全部打通**。

最需要优先解决的，不是继续零散加按钮，而是：

1. 统一计量页面与标定页面的业务定位。
2. 消除“页面看起来有、实际上后端没执行”的假功能。
3. 把自动采集、报警、单位检查、导出这四条关键链路接成闭环。

做到这三点后，后续再补批量连接、完整设备信息、历史会话、测试基座，整体项目才会真正进入可稳定交付阶段。
