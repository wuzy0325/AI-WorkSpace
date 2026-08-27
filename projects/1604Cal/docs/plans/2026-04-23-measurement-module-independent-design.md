# 计量模块独立业务链设计

> 日期：2026-04-23  
> 适用项目：`Cal1604`  
> 参考基线：`C:\Users\wuzhy\Documents\D\SVN\SoftWare\AI Engineering\Measurement\1604 Measurement\1605MeassureApp`

---

## 1. 设计背景

当前工程中，`measurement` 与 `calibration` 的业务边界曾被混淆：

- 参考模块 `1605MeassureApp` 的“计量模块”本身就是一条完整采集业务链，而不是单纯实时采样面板。
- 当前工程的 `measurement` 仅落地了通道级连续采样，参数、测点、稳定等待、报警、导出等业务闭环没有真正形成。
- 项目已明确要求：`measurement` 与 `calibration` 必须是两个**业务逻辑完全隔离**的模块。

因此，本设计的目标不是让 `measurement` 复用 `calibration` 的业务编排，而是让 `measurement` 自己长成一条完整、独立、可交付的计量采集链。

---

## 2. 强制边界

### 2.1 模块边界

- `measurement` 是独立的“计量采集模块”。
- `calibration` 是独立的“标定流程模块”。
- 两者允许共享底层设备驱动、连接管理、通用会话绑定、稳定性监视器等**基础设施能力**。
- 两者**禁止共享**业务状态机、业务编排、页面语义、事件命名、流程入口和导出口径。

### 2.2 禁止事项

- 禁止 `measurement` 调用 `calibrationService` 作为自己的业务引擎。
- 禁止 `measurement` 复用 `calibration` 的 `session/start`、`session/pause`、`session/resume` 业务入口。
- 禁止 `measurement` 使用 `calibration.*` 事件作为自己的页面驱动来源。
- 禁止把“旧系统计量业务链”直接等价映射为“当前工程里的标定链”。

---

## 3. 计量模块目标能力

`measurement` 第一阶段应独立承载以下能力：

1. 设备绑定与启动前门禁
2. 参数配置
3. 测点生成
4. 自动采集链路
5. 手动采集链路
6. 稳定等待与稳定进度反馈
7. 报警配置、报警判定与人工决策
8. 测点结果展示与原始采样展示
9. 导出闭环

这意味着 `measurement` 不是“简简单单实时采样页”，而是参考模块意义上的完整计量采集模块。

---

## 4. 业务流程

### 4.1 启动前门禁

启动计量流程前必须同时满足：

- 已绑定打压设备
- 已绑定计量设备
- 已选择采集通道
- 单位一致性检查通过
- 计量参数有效
- 压力点已生成
- 当前流程不在运行中

### 4.2 自动模式主链

自动模式流程为：

1. 读取当前计量配置
2. 获取压力点计划
3. 进入当前压力点
4. 设置目标压力并启动打压
5. 监控稳定性，累计稳定时间
6. 达到稳定阈值后执行按点采集
7. 计算平均值并保存测点结果
8. 执行报警判定
9. 根据报警决策继续、重采或停止
10. 全部测点完成后进入完成态

### 4.3 手动模式主链

手动模式流程为：

1. 页面选择当前目标压力点
2. 操作者触发手动打压
3. 系统持续监控稳定状态
4. 达到稳定后允许人工点击“手动采集”
5. 采集完成后写入当前测点数据
6. 进入下一点或结束

### 4.4 报警闭环

每个测点采集完成后，需要根据 `measurement` 自己的报警配置执行判定：

- 是否启用报警
- 哪些通道参与报警
- 报警阈值是多少
- 是否要求人工确认
- 是否触发声音提示

当触发报警时，`measurement` 进入自己的 `await_alarm_resolution` 状态，并等待用户作出：

- 继续
- 重采当前点
- 停止流程

---

## 5. 后端设计

### 5.1 独立领域对象

建议在 `internal/application/measurement/` 下引入以下对象：

- `MeasurementConfig`
  - `channels`
  - `minPressure`
  - `maxPressure`
  - `pointCount`
  - `precision`
  - `averageCount`
  - `precisionLevel`
  - `stableWaitMs`
  - `pressureMode`
  - `controlMode`
- `MeasurementAlarmConfig`
  - `enabled`
  - `enabledChannels`
  - `confirmOnAlarm`
  - `soundEnabled`
  - `threshold`
- `MeasurementPoint`
  - `id`
  - `index`
  - `targetPressure`
  - `direction`
  - `status`
  - `actualPressure`
  - `collectedData`
  - `collectTime`
  - `errorMessage`
- `MeasurementSession`
  - `id`
  - `startTime`
  - `endTime`
  - `config`
  - `points`
  - `pressureDeviceId`
  - `measureDeviceId`
  - `status`

### 5.2 独立状态机

`measurement` 自己维护独立状态，不依赖 `calibration` 状态机。建议状态包括：

- `idle`
- `ready`
- `pressuring`
- `stabilizing`
- `await_manual_collect`
- `collecting`
- `await_alarm_resolution`
- `paused`
- `completed`
- `error`
- `stopped`

### 5.3 API 设计

建议新增或收敛为以下计量专属接口：

- `GET /api/v1/config/measurement`
- `POST /api/v1/config/measurement`
- `GET /api/v1/config/measurement-alarm`
- `POST /api/v1/config/measurement-alarm`
- `POST /api/v1/measurement/points/generate`
- `GET /api/v1/measurement/points`
- `GET /api/v1/measurement/session`
- `POST /api/v1/measurement/start`
- `POST /api/v1/measurement/pause`
- `POST /api/v1/measurement/resume`
- `POST /api/v1/measurement/stop`
- `POST /api/v1/measurement/manual-pressurize`
- `POST /api/v1/measurement/manual-collect`
- `POST /api/v1/measurement/alarm/resolve`
- `POST /api/v1/measurement/report/export`

### 5.4 事件命名

计量模块前后端同步只使用 `measurement.*` 事件：

- `measurement.state.changed`
- `measurement.progress.updated`
- `measurement.point.status`
- `measurement.stability.updated`
- `measurement.data.collected`
- `measurement.alarm.triggered`
- `measurement.alarm.resolved`

---

## 6. 前端设计

### 6.1 页面结构

`MeasurementView` 恢复为完整计量工作台，建议分区如下：

1. 左侧设备与门禁区
2. 顶部参数配置区
3. 控制与进度区
4. 报警配置与报警处理区
5. 测点表与数据区
6. 导出区

### 6.2 页面语义

页面文案与流程必须保持“计量模块”口径：

- 不显示 `calibration` 会话概念
- 不借用标定模块的页面术语
- 不通过本地假数据伪造测点进度
- 一律以后端 `measurement` 数据和事件为准

### 6.3 Store 职责

`web/src/stores/measurement/index.ts` 需要升级为完整业务 store，至少维护：

- `config`
- `alarmConfig`
- `points`
- `progress`
- `session`
- `rawRows`
- `manualState`

---

## 7. 迁移策略（方案 C）

采用“先在 `measurement` 内补完整业务链，再视重复情况抽公共底层”的策略：

### 第一阶段

- 不动 `calibration` 业务逻辑
- 不抽共享业务层
- 先在 `measurement` 内补齐独立配置、点位、自动/手动采集、报警、导出

### 第二阶段

- 识别 `measurement` 与 `calibration` 真正重复的**纯底层能力**
- 只抽取：稳定监视器、导出模板工具、纯算法、设备访问适配器等基础设施
- 不抽取任何业务编排对象

---

## 8. 第一阶段验收标准

第一阶段完成后，至少满足以下验收条件：

1. 计量页面可配置参数，且后端真实消费这些参数。
2. 计量页面可生成压力点，并以后端点位状态驱动 UI。
3. 自动模式下，后端可独立完成打压、稳定等待、采集、报警判定。
4. 手动模式下，后端支持手动打压和手动采集闭环。
5. 计量报警配置真正参与判定，不再停留在本地 UI 状态。
6. 计量导出形成自己的用户闭环。
7. `measurement` 的实现不调用 `calibrationService`，不消费 `calibration.*` 事件。

---

## 9. 风险与约束

- 当前仓库中 `measurement` 已被部分收敛为“实时采样页”，这部分需要按本设计回调为完整计量工作台。
- 当前前端测试基座仍存在历史 warning，实施时需只针对受影响区域做定向验证，避免顺手扩散到无关问题。
- 由于 `measurement` 与 `calibration` 都涉及打压、稳定、采集，实施时必须持续审查是否越过了业务边界。

---

## 10. 本设计结论

本次确认的最终口径是：

- `measurement` 不是简单实时采样页，而是参考成熟模块的**独立完整计量采集模块**。
- `calibration` 不是 `measurement` 的替身，也不是可被复用的计量业务引擎。
- 当前阶段优先在 `measurement` 内独立补齐业务链，待业务边界稳定后，再考虑抽取纯底层公共能力。
