# Implementation Plan: 多量程向导式分批计量

> 基于 [Spec](./2026-07-08-multi-range-calibration-spec.md)
> 创建：2026-07-08

## Overview

在现有 `MeasurementView` 上增加分批模式：16 通道量程录入 → 自动分组 → 核对码确认 → 逐批加压 → 合并报告。后端新增 batch handler/service，前端新增 5 个组件 + store + composable + API client。

## Architecture Decisions

| 决策 | 理由 |
|------|------|
| 分批模式作为 MeasurementView 的一个子视图，不新建独立页面 | 复用现有计量工作台布局、设备连接、实时数据面板 |
| 核对码校验在后端执行 | 前端不可信，后端有量程配置的完整上下文 |
| 加压序列完全复用现有 calibration 流程 | 避免重复实现加压/稳定/采集逻辑 |
| 分组逻辑在前端执行，后端仅做验证 | 分组是纯 UI 交互（操作员可调整），后端只做最终校验 |
| 报告合并复用现有报告模板 | 减少开发量，后续再迭代 |

## Task List

### Phase 1: 数据层与后端基础

- [ ] **Task 1: 定义分批计量 TypeScript 类型**
  - 描述：创建 `web/src/types/batch.ts`，定义 `ChannelRange`、`BatchGroup`、`BatchSession`、`VerificationRequest`、`VerificationResult` 等类型
  - 验收：类型通过 `npm run typecheck`，所有字段与 Spec §8.1 一致
  - 验证：`npm run typecheck`
  - 依赖：无
  - 文件：`web/src/types/batch.ts`

- [ ] **Task 2: 后端分批计量 Service**
  - 描述：创建 `internal/application/batch/service.go`，实现分组验证、核对码校验、批次状态管理。复用现有 calibration 流程的加压执行
  - 验收：`go test ./internal/application/batch/...` 通过，覆盖分组验证和核对码校验
  - 验证：`go test ./internal/application/batch/...`
  - 依赖：Task 1（类型定义作为参考）
  - 文件：`internal/application/batch/service.go`、`internal/application/batch/service_test.go`

- [ ] **Task 3: 后端分批计量 Handler + 路由注册**
  - 描述：创建 `internal/api/http/batch_handler.go`，实现 5 个 API 端点（config / verify / start / pause-resume-stop / status / report）。在 `router.go` 注册路由
  - 验收：`go test ./internal/api/http/...` 通过，端点返回正确状态码
  - 验证：`go test ./internal/api/http/...`
  - 依赖：Task 2
  - 文件：`internal/api/http/batch_handler.go`、`internal/api/http/router.go`

### Checkpoint 1: 后端 API 就绪
- [ ] `go build ./...` 通过
- [ ] `go test ./...` 通过
- [ ] API 端点可响应（手动 curl 或集成测试）

### Phase 2: 前端组件

- [ ] **Task 4: BatchRangeInput — 16 通道量程录入面板**
  - 描述：创建 `BatchRangeInput.vue`，16 行输入（通道编号只读 | 量程数值 input | 单位下拉）。支持默认值、快速填充、清空
  - 验收：16 行正确渲染，单位下拉可选 MPa/kPa/bar/psi，输入校验（数值 > 0）
  - 验证：`npm run typecheck` + 手动测试（渲染 16 行，输入各种量程值）
  - 依赖：Task 1（类型）
  - 文件：`web/src/components/measurement/BatchRangeInput.vue`

- [ ] **Task 5: BatchGroupView — 自动分组展示 + 手动调整**
  - 描述：接收 `ChannelRange[]`，自动按 `(rangeValue, rangeUnit)` 分组展示。支持操作员手动增减通道，实时同批量程一致性校验
  - 验收：3 种量程 → 3 个批次卡片；手动移入不同量程通道 → 红色警告 + 冲突通道列表
  - 验证：`npm run typecheck` + 手动测试（分组正确性、校验警告）
  - 依赖：Task 1（类型）
  - 文件：`web/src/components/measurement/BatchGroupView.vue`

- [ ] **Task 6: BatchVerificationDialog — 核对码弹窗**
  - 描述：模态弹窗，大字显示量程值，输入核对码，校验通过后 emit。不可关闭绕过（无 X 按钮、无遮罩关闭）
  - 验收：弹窗正确显示量程值；输入正确 → 通过；输入错误 → 红色提示"核对码不匹配"
  - 验证：`npm run typecheck` + 手动测试（正确/错误输入）
  - 依赖：Task 1（类型）
  - 文件：`web/src/components/measurement/BatchVerificationDialog.vue`

- [ ] **Task 7: BatchProgressBar — 批次进度条**
  - 描述：显示"第 N/共 M 批"，已完成/进行中/待执行 状态，支持点击回退到已完成批次
  - 验收：正确显示进度；点击已完成批次 → 弹出确认 → 回退重跑
  - 验证：`npm run typecheck` + 手动测试（进度显示、回退交互）
  - 依赖：Task 1（类型）
  - 文件：`web/src/components/measurement/BatchProgressBar.vue`

### Checkpoint 2: 组件独立可用
- [ ] `npm run typecheck` 通过
- [ ] `npm run lint` 通过
- [ ] 每个组件可在 Storybook 或独立页面中渲染测试

### Phase 3: 状态管理与集成

- [ ] **Task 8: API Client + Store + Composable**
  - 描述：创建 `web/src/api/batchMeasurement.ts`（API 封装）、`web/src/stores/batchMeasurement.ts`（Pinia store）、`web/src/composables/useBatchMeasurement.ts`（流程编排）
  - 验收：store 正确管理 BatchSession 状态；composable 正确编排量程录入 → 分组 → 核对码 → 加压 → 合并流程
  - 验证：`npm run typecheck` + 单元测试（store 状态转换）
  - 依赖：Task 3（后端 API）、Task 1（类型）
  - 文件：`web/src/api/batchMeasurement.ts`、`web/src/stores/batchMeasurement.ts`、`web/src/composables/useBatchMeasurement.ts`

- [ ] **Task 9: MeasurementView 集成分批模式**
  - 描述：修改 `MeasurementView.vue`，增加"分批模式"开关/标签页。切换后进入分批流程，集成所有组件
  - 验收：切换分批模式 → 量程录入 → 分组展示 → 核对码 → 加压 → 合并报告，完整流程可走通
  - 验证：`npm run typecheck` + `npm run build` + 手动端到端测试
  - 依赖：Task 4-8
  - 文件：`web/src/views/MeasurementView.vue`

### Checkpoint 3: 核心流程端到端
- [ ] 完整分批流程可走通（量程录入 → 分组 → 核对码 → 加压 → 合并）
- [ ] `npm run typecheck` + `npm run lint` + `npm run build` 全绿
- [ ] 回退重跑功能正常

### Phase 4: 报告与收尾

- [ ] **Task 10: BatchReportView — 合并报告预览**
  - 描述：所有批次完成后，展示合并报告预览。按批次分段显示通道列表、量程、采集数据
  - 验收：报告正确显示所有批次数据，按批次分段
  - 验证：`npm run typecheck` + 手动测试（查看合并报告内容）
  - 依赖：Task 8（store 中有完整数据）
  - 文件：`web/src/components/measurement/BatchReportView.vue`

- [ ] **Task 11: 报告合并 API 端点**
  - 描述：实现 `POST /api/calibration/batch/report`，合并所有批次数据，复用现有报告生成逻辑
  - 验收：API 返回合并后的报告模板，包含所有批次数据
  - 验证：`go test` + 手动调用 API 检查返回结构
  - 依赖：Task 3
  - 文件：`internal/api/http/batch_handler.go`（追加）、`internal/application/batch/service.go`（追加）

### Checkpoint 4: 全部完成
- [ ] 所有 11 个任务完成
- [ ] `go test ./...` + `go vet ./...` + `npm run typecheck` + `npm run lint` + `npm run build` 全绿
- [ ] 端到端流程可走通

## Risks and Mitigations

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 复用现有加压序列时 API 不兼容 | 高 | Task 2 先确认现有 calibration service 接口，如果接口不支持分批通道子集，增加适配层 |
| 核对码数值匹配的浮点精度问题 | 低 | 使用 `parseFloat` 数值比对，量程值通常为整数（1/10/100），精度问题概率极低 |
| MeasurementView 集成后页面过于复杂 | 中 | 分批模式作为独立子视图，通过条件渲染切换，不增加 MeasurementView 基础状态复杂度 |
| 报告合并模板变更导致前后端不一致 | 低 | 先复用现有报告模板，格式变更在后续迭代中处理 |

## Open Questions

- 无。所有决策已确认。Execution plan ready.