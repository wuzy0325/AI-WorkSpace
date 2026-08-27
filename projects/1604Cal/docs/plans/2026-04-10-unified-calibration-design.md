# 1604 统一系统设计文档（Vue3 + Go API）

## 1. 文档信息

- 文档名称：1604 统一系统设计文档
- 创建日期：2026-04-10
- 当前版本：v1.0
- 适用范围：新系统（Vue3 前端 + Go 后端 API）
- 关联旧系统：
  - `C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\标定软件\1604标定软件`
  - `C:\Users\wuzhy\Documents\D\SVN\SoftWare\AI Engineering\Measurement\1604 Measurement\1605MeassureApp`

---

## 2. 背景与目标

本项目目标是在一个新程序内统一两套旧系统能力，形成一套可长期维护的架构：

- 前端：Vue3（工作台化、可扩展 UI）。
- 后端：Go（设备通信、流程编排、报告导出、配置与持久化）。
- 通信：HTTP API（REST）+ 实时事件流（SSE）。

设计必须满足以下要求：

1. 最终覆盖两个旧系统所有功能，无永久缺失项。
2. 代码整洁、结构清晰、可读性高。
3. 中文注释完整，复杂逻辑解释“为什么”。
4. 面向对象适度使用，优先组合与小接口。
5. 使用简单可理解的模式增强健壮性，避免过度设计。

---

## 3. 设计决策摘要（已确认）

### 3.1 总体方案

- 采用 `方案A：Go 原生重构`。
- 不延续 Electron IPC 主链路，改为 Vue3 直接调用 Go API。
- 本机一体化部署优先（Go + Web 前端同机运行）。

### 3.2 第一阶段边界（MVP）

- 计量设备：WTN1604。
- 打压设备：ConST 811A / ConST 820。
- 支持：自动模式 + 手动模式。
- 支持：单位一致性校验门禁。
- 支持：报告导出且与旧模板规则 100% 兼容。

### 3.3 最终目标边界（全量）

- 两套旧系统所有功能全部映射至新系统。
- 功能矩阵中每项最终必须为“已替代”或“等效增强”。

---

## 4. 规范基线与执行口径

统一遵循 `AGENTS.md` 的项目规范，规范来源包含：

- `https://github.com/uber-go/guide`
- `https://google.github.io/styleguide/go/`
- `https://github.com/vuejs/docs/tree/main/src/style-guide`
- `https://github.com/golang-standards/project-layout`

执行口径：

- 可读性优先，简单优先。
- 中文注释强制覆盖公开类型、关键逻辑、协议与状态机。
- 允许模式：Adapter / Factory / Strategy / State / Repository。
- 禁止“炫技式”抽象与复杂模式堆叠。

---

## 5. 统一架构设计

### 5.1 架构总览

```text
┌───────────────────────────────────────────────────────────────┐
│                        Vue3 Frontend                          │
│  设备管理  | 参数配置  | 采集工作台  | 报告中心  | 诊断中心      │
└───────────────────────────────────────────────────────────────┘
                         REST + SSE
┌───────────────────────────────────────────────────────────────┐
│                          Go API                               │
│  API层  应用层  领域层  基础设施层                             │
│                                                               │
│  DeviceManager  Workflow  Report  Config  EventBus            │
│  DriverRegistry (WTN1604 / 811A / 820 / 扩展)                 │
└───────────────────────────────────────────────────────────────┘
                         TCP/协议命令
┌───────────────────────────────────────────────────────────────┐
│                       物理设备层                              │
│                  1604计量设备 + 打压设备                      │
└───────────────────────────────────────────────────────────────┘
```

### 5.2 分层职责

- API 层：入参校验、错误码映射、响应结构统一、SSE 推送。
- 应用层：用例编排（会话控制、设备协调、报告任务管理）。
- 领域层：状态机、规则引擎、单位一致性、稳定判定、报警决策。
- 基础设施层：驱动实现、持久化、模板填充、日志与配置文件。

---

## 6. 后端设计（Go）

### 6.1 推荐目录结构

```text
unified-calibration/
  cmd/server/
  internal/domain/
  internal/workflow/
  internal/device/
  internal/driver/wtn1604/
  internal/driver/const811a/
  internal/driver/const820/
  internal/report/
  internal/config/
  internal/api/http/
  internal/api/dto/
  web/
  docs/
  resources/templates/1604V2/
```

### 6.2 驱动接口设计

#### 打压设备接口（PressureDriver）

- `Connect(ctx)`
- `Disconnect(ctx)`
- `SetTargetPressure(ctx, target)`
- `Stop(ctx)`
- `Exhaust(ctx)`
- `ReadCurrentPressure(ctx)`
- `ReadUnit(ctx)`
- `SetUnit(ctx, unit)`
- `ReadStability(ctx)`

#### 计量设备接口（MeasureDriver）

- `Connect(ctx)`
- `Disconnect(ctx)`
- `ReadValveStatus(ctx)`
- `SetValveStatus(ctx, status)`
- `ReadUnit(ctx)`
- `SetUnit(ctx, unit)`
- `CollectData(ctx, channels)`
- `ReadDeviceInfo(ctx)`
- `Reset(ctx)`

### 6.3 并发模型与资源控制

- 每台设备独立命令队列，串行下发，防止并发写乱序。
- 每次 I/O 必须带 `context + timeout + cancel`。
- 所有 goroutine 必须有退出机制与回收路径。
- 多计量设备采集使用并行聚合，但单设备故障可隔离。

### 6.4 错误处理策略

- 统一错误响应结构：`code/message/detail/requestId`。
- 错误处理一次原则：降级或上抛，不重复处理。
- 协议错误、网络错误、业务门禁错误明确分层。

---

## 7. API 设计（REST + SSE）

### 7.1 设备管理 API

- `GET /api/v1/devices`
- `POST /api/v1/devices`
- `PUT /api/v1/devices/{id}`
- `DELETE /api/v1/devices/{id}`
- `POST /api/v1/devices/{id}/connect`
- `POST /api/v1/devices/{id}/disconnect`
- `GET /api/v1/devices/{id}/status`

### 7.2 设备控制 API

- `POST /api/v1/measure/{id}/valve`
- `GET /api/v1/measure/{id}/unit`
- `POST /api/v1/measure/{id}/unit`
- `POST /api/v1/pressure/{id}/target`
- `POST /api/v1/pressure/{id}/stop`
- `POST /api/v1/pressure/{id}/exhaust`
- `GET /api/v1/pressure/{id}/current`

### 7.3 会话控制 API

- `POST /api/v1/sessions`
- `POST /api/v1/sessions/{id}/start`
- `POST /api/v1/sessions/{id}/pause`
- `POST /api/v1/sessions/{id}/resume`
- `POST /api/v1/sessions/{id}/stop`
- `POST /api/v1/sessions/{id}/retry-point`
- `POST /api/v1/sessions/{id}/manual/pressurize`
- `POST /api/v1/sessions/{id}/manual/collect`
- `GET /api/v1/sessions/{id}`
- `GET /api/v1/sessions/{id}/data`

### 7.4 报告与配置 API

- `GET /api/v1/reports/templates`
- `POST /api/v1/reports/export`
- `GET /api/v1/reports/jobs/{jobId}`
- `GET /api/v1/config/calibration`
- `PUT /api/v1/config/calibration`
- `GET /api/v1/config/alarm`
- `PUT /api/v1/config/alarm`
- `GET /api/v1/checks/unit-consistency`

### 7.5 实时事件（SSE）

- `GET /api/v1/events/stream?sessionId=...`

事件类型最小集合：

- `device.status.changed`
- `pressure.updated`
- `stability.updated`
- `session.state.changed`
- `point.completed`
- `alarm.triggered`
- `alarm.resolved`
- `report.progress`

---

## 8. 流程状态机设计

### 8.1 主状态

```text
idle
  -> ready
  -> pressurizing
  -> stabilizing
  -> collecting
  -> point_done
  -> (loop next point)
  -> fitting
  -> completed
```

### 8.2 异常与分叉状态

- `paused`
- `stopped`
- `await_manual_collect`
- `await_alarm_resolution`
- `recovering`
- `error`

### 8.3 关键规则

- 自动模式：稳定累计时间达到阈值后自动采集。
- 手动模式：稳定达标后允许人工触发采集。
- 报警后仅允许 `continue` 或 `retry` 决策。
- 非法状态迁移必须拒绝并记录日志。

---

## 9. 数据模型与持久化

### 9.1 关键实体

- `Device`
- `DeviceRuntime`
- `CalibrationSession`
- `PressurePoint`
- `PointSample`
- `PointAggregate`
- `AlarmEvent`
- `ReportJob`
- `AppConfig`
- `CommandTrace`

### 9.2 存储建议

- 首选：SQLite（本机一体化部署场景）。
- 兼容：JSON 导入导出（用于旧系统配置迁移）。

### 9.3 数据一致性原则

- 每个压力点保存原始样本与聚合值，支持追溯。
- 报告导出使用聚合值，必要时可重算。
- 所有关键写入带会话 ID 与时间戳。

---

## 10. 前端设计（Vue3）

### 10.1 路由与页面

- `/`：首页（系统摘要）
- `/workbench`：统一工作台（主页面）
- `/reports`：报告中心
- `/settings`：配置中心
- `/diagnostics`：诊断中心

### 10.2 工作台布局

- 左侧设备面板（可折叠）：打压设备区 + 计量设备区。
- 右侧主区：参数配置、压力点编辑、控制按钮、数据表格。
- 弹层：报警决策对话框、关键操作确认对话框。

### 10.3 Store 设计

- `deviceStore`
- `sessionStore`
- `calibrationStore`
- `dataStore`
- `reportStore`

所有 Store 仅通过 API 客户端调用后端，不直接访问硬件。

---

## 11. 报告兼容策略（100%）

### 11.1 模板规则

- 模板目录：`resources/templates/1604V2/`
- 模板命名：`{测点数}{模式}.xlsx`
- 支持：`2-11` 点，`s/m` 模式

### 11.2 兼容验收

- 字段映射一致
- 关键单元格值一致
- 报警与采集记录完整
- 可直接替代旧系统出具结果

---

## 12. 功能覆盖矩阵（核心映射）

| 来源模块 | 功能项 | 新系统模块 | 阶段 | 验收标准 |
|---|---|---|---|---|
| 1604标定软件 | 1604协议命令与状态机 | driver/wtn1604 + workflow | MVP | 命令回放一致，状态迁移可回放 |
| 1604标定软件 | 手动确认/重采/平均采样 | workflow/session | MVP | UI可触发，结果可追溯 |
| 1604标定软件 | 打压设备工厂扩展 | device/registry | MVP | 新型号接入无需改业务层 |
| 1605MeassureApp | 设备台账与多设备管理 | device manager | MVP | 增删改连断全覆盖 |
| 1605MeassureApp | 单位一致性门禁 | domain/rule + api check | MVP | 不一致不可开始 |
| 1605MeassureApp | 自动/手动采集流程 | workflow | MVP | 双模式完整闭环 |
| 1605MeassureApp | 报警决策闭环 | alarm service | MVP | continue/retry 完整记录 |
| 1605MeassureApp | 报告导出 | report service | MVP | 模板与字段 100% 兼容 |
| 两系统融合 | 诊断与追踪能力 | diagnostics + command trace | P2 | 可定位协议/流程问题 |
| 两系统融合 | 扩展型号支持 | strategy/adapter | P3 | 860/SPC4000 等可扩展 |

> 说明：该矩阵为 v1，后续每轮迭代补齐全部功能项细表。

---

## 13. 分阶段计划

### 13.1 MVP

- 设备：WTN1604 + ConST 811A/820。
- 核心：设备管理、单位一致性、自动/手动采集、报警决策、报告导出。

### 13.2 Phase 2

- 补齐高级诊断、异常恢复细节、流程可观测能力。
- 强化多设备并发稳定性与可维护性。

### 13.3 Phase 3

- 扩展更多打压型号与协议插件化能力。
- 完成旧系统功能全替代并进入长期演进。

---

## 14. 风险与缓解

### 14.1 关键风险

- 协议时序差异导致行为不一致。
- 模板映射偏差导致报告不兼容。
- 状态机边界不全导致流程卡死。
- 多设备并发导致局部故障扩大。

### 14.2 缓解措施

- 建立黄金样本对照。
- 驱动契约测试 + 真机联调。
- 状态迁移白名单。
- 设备隔离与超时重试策略。

---

## 15. 测试策略与发布门禁

### 15.1 测试分层

- 单元测试（协议、规则、计算）
- API 集成测试
- 前端组件/Store 测试
- 端到端流程测试
- 真机联调（HIL）
- 新旧系统差异回归测试

### 15.2 发布门禁

- Go：`go test ./...`、`go vet ./...`、`staticcheck ./...`
- Vue：`npm run typecheck`、`npm run lint`
- 功能矩阵全绿
- 报告兼容性全绿
- 8小时长稳验证通过
- 回滚演练通过

---

## 16. 迁移与回滚方案

### 16.1 迁移步骤

1. 冻结旧系统基线并提取黄金样本。
2. 新系统离线等价验证。
3. 实验工位双轨并行。
4. 小范围灰度。
5. 扩面上线。
6. 满足门禁后退役旧系统。

### 16.2 回滚触发与动作

- 触发：P0/P1 严重故障、报告不可用、数据偏差超阈。
- 动作：立即切回旧系统、冻结新系统写入、保留现场日志复盘。

---

## 17. Session Handoff（跨 Session 续接）

### 17.1 已确认决策

- 采用 Go 原生重构方案。
- 本机一体化部署优先。
- MVP 设备范围与全量覆盖目标已确认。
- 规范基线与编码原则已写入 `AGENTS.md`。

### 17.2 未决事项

- 详细 API 字段字典与错误码清单。
- 数据库表结构精细字段（索引、约束）。
- 设备命令时序契约测试样例集合。

### 17.3 下个 Session 入口

1. 依据本设计文档产出“实施计划文档”（任务拆解到迭代级）。
2. 建立 API 契约文档与错误码规范。
3. 初始化项目骨架与最小可运行链路。

### 17.4 上下文容量提示机制

- 当上下文预计剩余约 20%-25% 时，主动提示用户开启新 session。
- 提示内容固定包含：
  - 已完成内容
  - 当前阻塞点
  - 下步执行入口
  - 文档路径

---

## 18. 相关文档路径

- 设计文档：`docs/plans/2026-04-10-unified-calibration-design.md`
- 执行规范：`AGENTS.md`
