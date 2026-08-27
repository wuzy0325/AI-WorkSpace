# Spec: 多量程向导式分批计量

> 状态：已审核
> 创建：2026-07-08
> 输入基线：[意图文档](./2026-07-08-multi-range-calibration-intent.md) + [idea-refine one-pager](../ideas/multi-range-guided-batch.md)

---

## 1. Objective

**目标**：在现有 `MeasurementView` 上增加分批模式，让操作员录入 16 通道量程后，系统自动分组、逐批引导（含核对码确认）、最终自动合并报告。

**用户**：一线计量操作员——怕流程散、怕报告散、怕换错标准器。

**核心用户故事**：
1. 操作员进入计量工作台 → 切换到"分批模式" → 录入 16 通道量程
2. 系统自动分组，显示"本次共 3 批" → 操作员可调整每批通道
3. 开始第 1 批 → 弹窗提示"请切换到 X MPa 标准器" → **输入核对码（量程值）** → 解锁加压
4. 逐批执行，每批独立加压序列 → 可暂停/中止/回退重跑
5. 全部批次完成 → 自动合并为一份报告，按批次分段

---

## 2. Tech Stack

| 层 | 技术 | 版本 |
|----|------|------|
| 后端 | Go | 1.25 |
| 前端 | Vue 3 + TypeScript + Vite | 最新 |
| UI 库 | Naive UI / Element Plus Icons | 现有 |
| 桌面壳 | Wails v3 | alpha.95 |
| 设备通信 | SCPI / TCP | 现有 |
| 构建 | Taskfile.yml + npm scripts | 现有 |

---

## 3. Commands

```bash
# 后端
go build ./...         # 编译
go test ./...          # 测试
go vet ./...           # 静态检查

# 前端
cd web
npm run typecheck      # 类型检查
npm run lint           # 代码规范
npm run build          # 构建

# 桌面应用
wails3 build           # 打包
```

---

## 4. Project Structure

### 4.1 新增文件

```
web/src/
├── types/
│   └── batch.ts                          # 分批计量相关类型定义
├── stores/
│   └── batchMeasurement.ts              # 分批计量状态管理 Pinia store
├── composables/
│   └── useBatchMeasurement.ts           # 分批计量流程编排 composable
├── components/
│   └── measurement/
│       ├── BatchRangeInput.vue           # 16 通道量程录入面板
│       ├── BatchGroupView.vue            # 自动分组展示 + 手动调整
│       ├── BatchVerificationDialog.vue   # 核对码弹窗（物理切换确认）
│       ├── BatchProgressBar.vue          # 批次进度条（第 N/共 M 批）
│       └── BatchReportView.vue           # 合并报告预览
├── api/
│   └── batchMeasurement.ts              # 分批计量 API 封装

internal/
├── api/http/
│   └── batch_handler.go                 # 分批计量 HTTP handler
├── application/
│   └── batch/
│       └── service.go                   # 分批计量业务逻辑
```

### 4.2 修改文件

```
web/src/views/
└── MeasurementView.vue                   # 增加"分批模式"入口与流程编排

internal/api/http/
└── router.go                             # 注册分批计量路由
```

---

## 5. Code Style

遵循 [AGENTS.md](../../AGENTS.md) 规范。关键示例：

```vue
<script setup lang="ts">
// ---- 批次量程数据模型 ----
interface ChannelRange {
  channelId: number      // 通道编号 1-16
  rangeValue: number     // 量程数值
  rangeUnit: string      // 量程单位：MPa / kPa / bar / psi
  skipped: boolean       // 是否跳过（一期不做，预留字段）
}

// ---- 批次定义 ----
interface BatchGroup {
  batchId: string        // 批次唯一标识，如 batch-1
  rangeValue: number     // 该批次量程数值
  rangeUnit: string      // 该批次量程单位
  channels: ChannelRange[] // 该批次包含的通道
  status: 'pending' | 'running' | 'completed' | 'retrying'
}
</script>
```

关键约定：
- 中文注释解释"为什么"，不是"做什么"
- 公开类型/函数必须中文注释
- 组件名多单词 PascalCase
- Props 必须类型定义
- 早返回减少嵌套

---

## 6. Testing Strategy

| 层级 | 框架 | 位置 | 覆盖要求 |
|------|------|------|----------|
| Go 单元测试 | `testing` | `internal/application/batch/*_test.go` | 分组逻辑、核对码校验 |
| Go 集成测试 | `testing` | `internal/api/http/*_test.go` | API 端点 |
| 前端类型检查 | `vue-tsc` | 全量 | 零错误 |
| 前端组件测试 | `vitest` | `web/src/components/__tests__/` | 量程录入、分组展示 |
| 端到端测试 | 后续接入 | `e2e/` | 完整分批流程 |

测试用例格式（三段式）：
```
测试前置：录入 16 通道量程，包含 3 种不同量程
测试步骤：触发自动分组
期待结果：生成 3 个批次，每个批次内通道量程一致
```

---

## 7. Boundaries

### Always
- 中文注释，公开类型/函数必须注释
- 设备抽象层 + 每设备独立线程（AGENTS.md 第 3 节）
- 核对码必须与批次量程值精确匹配
- 同批量程一致性校验（实时警告 + 二次确认后允许继续）
- 提交前 `go test ./...` + `npm run typecheck` + `npm run lint`

### Ask First
- 数据库 schema 变更（如需引入持久化批次结果）
- 添加新依赖
- 修改现有加压序列 API 签名
- 报告模板格式变更

### Never
- 跨设备共享线程
- 忽略错误不处理
- 核对码比对使用模糊匹配
- 批量操作跳过设备状态检查

---

## 8. 数据模型

### 8.1 前端类型（`types/batch.ts`）

```typescript
/** 单通道量程录入 */
export interface ChannelRange {
  channelId: number          // 1-16
  rangeValue: number         // 量程数值
  rangeUnit: 'MPa' | 'kPa' | 'bar' | 'psi'
  skipped: boolean           // 一期固定 false
}

/** 批次分组 */
export interface BatchGroup {
  batchId: string            // 如 "batch-1"
  batchIndex: number         // 批次序号 1-based
  rangeValue: number
  rangeUnit: string
  channels: ChannelRange[]   // 该批次包含的通道
  status: 'pending' | 'running' | 'completed' | 'retrying'
  collectedData?: Record<number, number[]>  // channelId -> 采集数据
  pressurePoints?: number[]  // 已完成的加压点列表
}

/** 分批计量会话 */
export interface BatchSession {
  channelRanges: ChannelRange[]   // 16 通道量程配置
  batches: BatchGroup[]           // 自动分组结果
  currentBatchIndex: number       // 当前正在执行的批次索引（-1 表示未开始）
}

/** 核对码校验请求 */
export interface VerificationRequest {
  batchId: string
  verificationCode: string        // 操作员输入的核对码
}

/** 核对码校验结果 */
export interface VerificationResult {
  valid: boolean
  message: string                  // 失败时的提示信息
}
```

### 8.2 后端类型（Go）

```go
// BatchConfig 批次配置（由前端传入）。
type BatchConfig struct {
    ChannelRanges []ChannelRange `json:"channelRanges"`
    Batches       []BatchGroup   `json:"batches"`
}

// ChannelRange 单通道量程配置。
type ChannelRange struct {
    ChannelID  int     `json:"channelId"`
    RangeValue float64 `json:"rangeValue"`
    RangeUnit  string  `json:"rangeUnit"`
    Skipped    bool    `json:"skipped"`
}

// BatchGroup 批次分组信息。
type BatchGroup struct {
    BatchID     string         `json:"batchId"`
    BatchIndex  int            `json:"batchIndex"`
    RangeValue  float64        `json:"rangeValue"`
    RangeUnit   string         `json:"rangeUnit"`
    Channels    []ChannelRange `json:"channels"`
    Status      string         `json:"status"`
}

// VerifyBatchRequest 核对码校验请求。
type VerifyBatchRequest struct {
    BatchID          string `json:"batchId"`
    VerificationCode string `json:"verificationCode"`
}
```

---

## 9. API 设计

### 9.1 分批配置

```
POST /api/calibration/batch/config
  Request:  BatchConfig（通道量程 + 分组结果）
  Response: { batchId: string }
  说明: 前端完成分组后提交配置，后端验证并初始化批次会话
```

### 9.2 核对码校验

```
POST /api/calibration/batch/{batchId}/verify
  Request:  { verificationCode: string }
  Response: { valid: bool, message: string }
  说明: 校验操作员输入的核对码是否与批次量程值匹配
  校验逻辑: parseFloat(verificationCode) == rangeValue（数值匹配，10 == 10.0）
```

### 9.3 批次执行

```
POST /api/calibration/batch/{batchId}/start
  Response: { sessionState: string }
  说明: 开始指定批次的加压序列，复用现有加压流程

POST /api/calibration/batch/{batchId}/pause
POST /api/calibration/batch/{batchId}/resume
POST /api/calibration/batch/{batchId}/stop
  Response: { sessionState: string }
  说明: 复用现有校准会话控制 API

GET /api/calibration/batch/{batchId}/status
  Response: { state, pressurePoints[], collectedData }
  说明: 查询当前批次状态
```

### 9.4 报告合并

```
POST /api/calibration/batch/report
  Request: { batches: BatchGroup[] }
  Response: { reportTemplate: ReportTemplateDTO }
  说明: 合并所有批次数据生成最终报告，按批次分段
```

---

## 10. UI 组件设计

### 10.1 组件树

```
MeasurementView（修改）
├── BatchRangeInput          ← 新增：16 通道量程录入面板
│   ├── 16 行输入：通道编号（只读）| 量程数值（input）| 单位（下拉）
│   └── 录入完成后 → emit('confirm', channelRanges[])
├── BatchGroupView           ← 新增：自动分组展示
│   ├── 显示 N 个批次卡片，每卡片：量程 | 通道列表 | 手动调整
│   └── 同批量程一致性校验（实时警告）
├── BatchVerificationDialog  ← 新增：核对码弹窗
│   ├── 标题："请确保已切换到 X MPa 标准器"
│   ├── 输入框：核对码（placeholder: "请输入量程值以确认"）
│   ├── 按钮：确认（校验通过后关闭+解锁加压）| 取消（留在弹窗）
│   └── 错误提示："核对码不匹配，请确认标准器量程标识"
├── BatchProgressBar         ← 新增：批次进度条
│   └── 显示 "第 N/共 M 批" + 已完成/进行中/待执行 状态
└── BatchReportView          ← 新增：合并报告预览
    └── 按批次分段展示采集数据 + 拟合结果
```

### 10.2 核对码弹窗交互

```
┌─────────────────────────────────────┐
│  ⚠ 物理切换确认                     │
│                                     │
│  请确保已将标准器切换为：            │
│  ┌─────────────────────────────┐    │
│  │        10 MPa               │    │
│  └─────────────────────────────┘    │
│                                     │
│  请输入标准器量程值以确认切换：       │
│  ┌─────────────────────────────┐    │
│  │ 10                          │    │
│  └─────────────────────────────┘    │
│                                     │
│  [ 取消 ]        [ 确认切换 ]       │
└─────────────────────────────────────┘
```

- 量程值大字醒目显示，方便操作员与标准器标识比对
- 输入框聚焦，支持回车确认
- 校验失败显示红色提示："核对码不匹配，请确认标准器量程标识"
- 弹窗不可关闭绕过（无 X 按钮、无点击遮罩关闭）

---

## 11. 流程设计

### 11.1 主流程

```
┌─────────────┐    ┌─────────────┐    ┌──────────────────┐    ┌──────────────┐
│ 量程录入     │───→│ 自动分组     │───→│ 逐批执行（循环）   │───→│ 报告合并      │
│ BatchRange  │    │ BatchGroup  │    │ 核对码→加压→完成  │    │ BatchReport  │
│ Input       │    │ View        │    │ Verification     │    │ View         │
└─────────────┘    └─────────────┘    └──────────────────┘    └──────────────┘
```

### 11.2 单批次执行流程

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│ 核对码弹窗 │───→│ 加压序列  │───→│ 采集完成  │───→│ 批次完成  │
│           │    │ （复用现有）│    │ 记录数据  │    │ 下一批/   │
│ 校验通过   │    │           │    │           │    │ 合并报告  │
└──────────┘    └──────────┘    └──────────┘    └──────────┘
       │              │              │
       │ 校验失败      │ 暂停/中止     │ 回退重跑
       ▼              ▼              ▼
   留在弹窗        暂停弹窗       回到该批次
                  可恢复/中止     重新执行
```

### 11.3 自动分组算法

```
输入: channelRanges[] (16 个通道的量程配置)
输出: batches[] (按量程分组的结果)

算法:
1. 过滤掉 skipped == true 的通道
2. 按 (rangeValue, rangeUnit) 分组
3. 每组生成一个 BatchGroup，默认所有通道选中
4. 按 rangeValue 升序排列批次（小量程优先）
5. 操作员可手动调整每批通道（增减）
6. 调整后实时校验：同批内所有通道量程必须一致
```

---

## 12. Success Criteria

| # | 条件 | 验证方式 |
|---|------|----------|
| 1 | 16 通道量程录入后，系统自动按量程分组，分组正确率 100% | 单元测试：3 种量程 → 3 个批次 |
| 2 | 核对码校验：输入与量程值不匹配 → 拒绝；匹配 → 通过 | 单元测试：`"10"` vs `10.0` → 通过 |
| 3 | 同批次内通道量程不一致 → 红色警告 + 列出冲突通道 | 手动测试：手动将不同量程通道加入同批 |
| 4 | 每批加压序列可独立执行，复用现有加压流程 | 集成测试：配置 2 个批次 → 逐批执行 |
| 5 | 全部批次完成后，自动合并为一份报告，按批次分段 | 集成测试：验证报告内容包含所有批次数据 |
| 6 | 批次回退重跑：已完成批次可重新执行，覆盖原数据 | 手动测试：回退到第 1 批 → 重跑 → 数据更新 |
| 7 | typecheck + lint 零错误 | `npm run typecheck` + `npm run lint` |
| 8 | 后端编译通过 | `go build ./...` |

---

## 13. 已确认决策

| 决策点 | 结论 |
|--------|------|
| 加压点配置 | 所有批次共用同一套加压点，复用现有 `CalibrationConfigDTO` |
| 核对码精度 | 数值匹配：`10` == `10.0`，使用 `parseFloat` 数值比对 |
| 报告格式 | 先复用现有标定报告模板，后续根据实际使用反馈调整