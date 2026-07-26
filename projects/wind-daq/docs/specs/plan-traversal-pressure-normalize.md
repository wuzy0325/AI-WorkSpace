# Implementation Plan: 遍历测试压力通道归一化

> 关联 spec：[spec-traversal-pressure-normalize.md](file:///C:/Users/wuzhy/Documents/D/SVN/SoftWare/trunk/AI-Workspace/projects/wind-daq/docs/specs/spec-traversal-pressure-normalize.md)
> 日期：2026-07-10
> 状态：待审批

## Overview

依据 spec v2，在遍历测试模块建立「压力通道归一化规约」：任何设备配置（表压/绝压、任意单位）的 P1-P5/Patm 通道数据，在喂给插值器前自动归一化到「Pa + 表压/绝压」，消除插值器隐含假设导致的安全隐患。改动覆盖后端 `core/pressure` 新包、`ports.ChannelUnitProvider` 新端口、`TraversalManager` 注入、`BuildRawPressure` 重构、`CheckPreconditions` 通道校验、3 处装配点、以及前端 TraversalSettings 开关。

## Architecture Decisions

- **委托而非重建换算表**：`core/pressure` 不维护独立 `UnitToPaFactor`，委托 `core/device.UnitConverter.ToBaseUnit`，避免双套系数表（spec §Code Style）
- **窄端口而非宽依赖**：新增 `ports.ChannelUnitProvider` 单方法接口，`DeviceManager` 实现之，`TraversalManager` 通过 setter 注入，避免 usecase 兄弟包直接依赖（spec §A1-A3）
- **可变参数 + setter 双轨注入**：构造函数追加 `unitProvider ...ports.ChannelUnitProvider` 保持 10 处现有调用点不破坏，bootstrap 通过 `SetUnitProvider` 在 manager 创建后注入（spec §A3）
- **降级策略**：`unitProvider == nil` 或查询单位失败时跳过换算记 warning，保证旧测试与离线场景不崩（spec §A4）
- **API 响应语义变更**：`rawPressure` 字段从原始通道值改为归一化 Pa+表压值，前端需同步标签（spec §CSV/API 语义说明）
- **kgf/cm2 alias**：在 `unit_converter.pressureFamily()` 同时注册 `kgfcm2` 和 `kgf/cm2` 指向同系数，向后兼容（spec §Code Style 单位字面量统一）

## Task List

### Phase 1: Foundation（基础包与端口）

#### Task 1: unit_converter 新增 kgf/cm2 alias

**Description:** 在 `core/device/unit_converter.go` 的 `pressureFamily()` 中追加 `'kgf/cm2'` key 指向 `98066.5`，与已有 `'kgfcm2'` 共存为 alias。解决前端用 `'kgf/cm2'`、Go 旧代码用 `'kgfcm2'` 的字面量不一致。

**Acceptance criteria:**
- [ ] `pressureFamily()` 同时包含 `kgfcm2: 98066.5` 和 `kgf/cm2: 98066.5` 两个 key
- [ ] 已有 `kgfcm2` 调用方不受影响（向后兼容）
- [ ] 新增单元测试覆盖 `kgf/cm2` alias 换算

**Verification:**
- [ ] `go test ./internal/core/device/...` 通过
- [ ] `go vet ./internal/core/device/...` 无告警

**Dependencies:** None

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/device/unit_converter.go`
- `projects/wind-daq/services/api-go/internal/core/device/unit_converter_test.go`（若不存在则新建）

**Estimated scope:** XS（1-2 文件）

---

#### Task 2: 新建 core/pressure 包 + 单元测试

**Description:** 新建 `core/pressure/normalize.go` 实现 `NormalizePressureToGaugePa` 与 `ConvertToPa`，内部委托 `device.UnitConverter`。同步建 `normalize_test.go` 覆盖 spec 测试用例表中前 9 项（5 种单位 + 绝压/表压 + 未知单位 + converter nil + kgfcm2 alias）。

**Acceptance criteria:**
- [ ] `NormalizePressureToGaugePa(value, unit, pressureType, patmPa, converter)` 签名与 spec §Code Style 一致
- [ ] `ConvertToPa(value, unit, converter)` 签名与 spec §Code Style 一致
- [ ] converter nil 时返回明确 error
- [ ] 未知单位返回 error（包含原单位字符串）
- [ ] `pressureType` 空串与 `"gauge"` 行为一致
- [ ] 测试用例 1-9 全部通过（Pa/kPa/MPa/psi/kgf/cm²/kgfcm2 alias/Patm 换算/未知单位/converter nil）

**Verification:**
- [ ] `go test ./internal/core/pressure/...` 通过
- [ ] `go vet ./internal/core/pressure/...` 无告警
- [ ] 测试覆盖率 ≥ 90%

**Dependencies:** Task 1

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/pressure/normalize.go`（新建）
- `projects/wind-daq/services/api-go/internal/core/pressure/normalize_test.go`（新建）

**Estimated scope:** S（2 文件）

---

#### Task 3: 新增 ports.ChannelUnitProvider 接口

**Description:** 在 `internal/ports/traversal.go`（已有文件）追加 `ChannelUnitProvider` 接口定义，单方法 `ChannelUnit(deviceID, channelIndex) (string, error)`。

**Acceptance criteria:**
- [ ] 接口定义与 spec §A1 完全一致
- [ ] 中文注释解释"为什么需要此端口"（BuildRawPressure 需查通道 Unit 但 LatestDataReader 不暴露）
- [ ] `go vet ./internal/ports/...` 无告警

**Verification:**
- [ ] `go build ./internal/ports/...` 通过

**Dependencies:** None

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/ports/traversal.go`

**Estimated scope:** XS（1 文件）

---

### Checkpoint: Foundation

- [ ] `go build ./...` 通过
- [ ] `go test ./internal/core/pressure/... ./internal/core/device/...` 通过
- [ ] `go vet ./...` 无告警
- [ ] Review：core/pressure 包是否真正委托 UnitConverter，无重复系数表

---

### Phase 2: Domain Types & Usecase Wiring（类型与接线）

#### Task 4: traversal.Config + traversalAPIConfig 新增 PProbePressureType

**Description:** 在 `core/traversal/types.go` 的 `Config` 结构体追加 `PProbePressureType string` 字段（带 `json:"pProbePressureType,omitempty"` tag）；在 `usecase/traversal_config.go` 的 `traversalAPIConfig` 结构体同步追加该字段；在 `ParseAndStartTraversal`（line 514-526）组装 `traversal.Config` 时映射该字段，空串兜底为 `"gauge"`。

**Acceptance criteria:**
- [ ] `traversal.Config.PProbePressureType` 字段存在且 JSON tag 正确
- [ ] `traversalAPIConfig.PProbePressureType` 字段存在且 JSON tag 正确
- [ ] `ParseAndStartTraversal` 映射字段时，空串或缺失自动设为 `"gauge"`
- [ ] 旧 JSON 配置（无该字段）反序列化后 `PProbePressureType == "gauge"`

**Verification:**
- [ ] `go test ./internal/core/traversal/... ./internal/usecase/...` 通过（含旧配置兜底测试）
- [ ] `go vet ./...` 无告警

**Dependencies:** None

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/core/traversal/types.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_config.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_config_test.go`（追加旧配置兜底用例）

**Estimated scope:** S（2-3 文件）

---

#### Task 5: DeviceManager 实现 ChannelUnitProvider

**Description:** 在 `usecase/device_manager.go` 追加 `ChannelUnit(deviceID, channelIndex) (string, error)` 方法，从 `m.profiles` 中查找设备→通道→`Unit`。复用已有 `findProfileLocked` 私有方法。

**Acceptance criteria:**
- [ ] `DeviceManager` 实现 `ports.ChannelUnitProvider` 接口（编译期断言）
- [ ] 设备不存在返回 error 含 deviceID
- [ ] 通道不存在返回 error 含 channelIndex 与 deviceID
- [ ] 正常情况返回 `ChannelConfig.Unit` 字段值
- [ ] 加读锁 `m.mu.RLock`

**Verification:**
- [ ] `go test ./internal/usecase/...` 通过（追加 ChannelUnit 单元测试）
- [ ] `go vet ./...` 无告警
- [ ] 编译期接口断言：`var _ ports.ChannelUnitProvider = (*DeviceManager)(nil)`

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/device_manager.go`
- `projects/wind-daq/services/api-go/internal/usecase/device_manager_test.go`（追加测试）

**Estimated scope:** S（2 文件）

---

#### Task 6: TraversalManager 注入 unitProvider + setter

**Description:** 在 `usecase/traversal.go` 的 `TraversalManager` 结构体追加 `unitProvider ports.ChannelUnitProvider` 字段；构造函数 `NewTraversalManager` 追加可变参数 `unitProvider ...ports.ChannelUnitProvider`；新增 `SetUnitProvider(p ports.ChannelUnitProvider)` setter 方法（与 `SetInterpolatorLoader` 模式一致）。

**Acceptance criteria:**
- [ ] 结构体新增 `unitProvider` 字段
- [ ] 构造函数签名追加可变参数，向后兼容（现有 10 处调用点不需改）
- [ ] `SetUnitProvider` 方法存在且写入字段
- [ ] 可变参数传入时优先取第一个元素
- [ ] 现有所有 `NewTraversalManager` 调用编译通过

**Verification:**
- [ ] `go build ./...` 通过
- [ ] `go test ./internal/usecase/...` 通过
- [ ] `go vet ./...` 无告警

**Dependencies:** Task 3

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/traversal.go`

**Estimated scope:** S（1 文件）

---

### Checkpoint: Domain Types & Usecase Wiring

- [ ] `go build ./...` 通过
- [ ] `go test ./internal/...` 通过
- [ ] `go vet ./...` 无告警
- [ ] Review：可变参数 + setter 双轨注入是否正确，10 处现有调用点未破坏

---

### Phase 3: Core Normalization Logic（核心归一化逻辑）

#### Task 7: BuildRawPressure 新签名 + 归一化逻辑

**Description:** 重构 `usecase/traversal_view.go:145` 的 `BuildRawPressure` 函数，新签名增加 `deviceID string`、`unitProvider ports.ChannelUnitProvider`、`pressureType string` 三个参数。归一化策略：P1-P5 按 Unit 换算到 Pa（绝压类型再减 Patm），Patm 仅换算到 Pa（不减），Tatm 不参与。降级策略：`unitProvider == nil` 或查询单位失败时跳过换算记 warning。

**Acceptance criteria:**
- [ ] 新签名与 spec §A4 完全一致
- [ ] P1-P5 绝压类型时减去已归一化的 Patm（Pa）
- [ ] Patm 仅做单位换算，不减
- [ ] Tatm 不参与归一化（保持原值）
- [ ] `unitProvider == nil` 时跳过换算，记 warning 日志，返回原始值
- [ ] 查询单位失败时跳过该通道换算，记 warning，其他通道正常归一化
- [ ] 返回的 `rawPressure` map 中 P1-P5/Patm 为归一化值，Tatm 为原值
- [ ] `ok` 标志逻辑不变（7 个标签齐全）

**Verification:**
- [ ] `go test ./internal/usecase/...` 通过（追加绝压/表压/降级 3 个集成测试）
- [ ] `go vet ./...` 无告警
- [ ] 测试覆盖 spec 测试用例表中"BuildRawPressure 集成-绝压"与"BuildRawPressure 降级"两项

**Dependencies:** Task 2, Task 3, Task 4

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/traversal_view.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_view_test.go`（若不存在则新建）

**Estimated scope:** M（2 文件，含集成测试）

---

#### Task 8: BuildRawPressure 两处调用方适配

**Description:** 适配 `BuildRawPressure` 的两处调用方：`traversal_view.go:120` 的 `BuildDataPoints` 与 `traversal_acquisition.go:228` 的 `RunTraversalLoop`。从 `m.config.DeviceID`、`m.unitProvider`、`m.config.PProbePressureType` 取参传入。

**Acceptance criteria:**
- [ ] `BuildDataPoints` 调用传入正确参数
- [ ] `RunTraversalLoop` 调用传入正确参数
- [ ] `unitProvider` 为 nil 时调用不 panic（降级路径生效）
- [ ] `PProbePressureType` 空串时按 "gauge" 处理

**Verification:**
- [ ] `go build ./...` 通过
- [ ] `go test ./internal/usecase/...` 通过
- [ ] `go vet ./...` 无告警

**Dependencies:** Task 6, Task 7

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/traversal_view.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_acquisition.go`

**Estimated scope:** S（2 文件）

---

#### Task 9: CheckPreconditions 新增 ChannelMap 校验

**Description:** 在 `usecase/traversal.go:193` 的 `CheckPreconditions` 中新增 `ChannelMap` 校验项，检查 `Config.ChannelLabels` 是否包含 `Patm` 和 `Tatm` 标签。P1-P5 标签存在性由 `BuildRawPressure` 的 `ok` 标志在运行时承担，不重复校验。

**Acceptance criteria:**
- [ ] `checks` 数组新增 `ChannelMap` 项
- [ ] 缺 Patm 时 `passed=false`，message 含 "Patm channel label is required"
- [ ] 缺 Tatm 时 `passed=false`，message 含 "Tatm channel label is required"
- [ ] 两者齐全时 `passed=true`
- [ ] `allPassed` 字段纳入 `ChannelMap` 通过条件

**Verification:**
- [ ] `go test ./internal/usecase/...` 通过（追加 CheckPreconditions 缺 Patm / 完整两个测试）
- [ ] `go vet ./...` 无告警

**Dependencies:** Task 4（Config.ChannelLabels 已存在）

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/usecase/traversal.go`
- `projects/wind-daq/services/api-go/internal/usecase/traversal_test.go`（追加测试）

**Estimated scope:** S（2 文件）

---

### Checkpoint: Core Normalization Logic

- [ ] `go build ./...` 通过
- [ ] `go test ./internal/...` 通过
- [ ] `go vet ./...` 无告警
- [ ] Review：归一化策略是否正确（P1-P5 减 Patm、Patm 不减、Tatm 不参与）
- [ ] Review：降级路径是否覆盖 unitProvider nil 与查询单位失败两种情况

---

### Phase 4: Wiring & Backend Integration（装配与后端集成）

#### Task 10: 三处装配点 SetUnitProvider 注入

**Description:** 在 `bootstrap.go:94`、`apiserver.go:80`、`context.go:94` 三处 `NewTraversalManager` 调用之后，追加 `travMgr.SetUnitProvider(manager)` 注入。注意 `manager`（DeviceManager）的创建顺序在 `travMgr` 之后，需要调整代码顺序或延迟注入。

**Acceptance criteria:**
- [ ] `bootstrap.go` 中 `travMgr.SetUnitProvider(manager)` 在 manager 创建后调用
- [ ] `apiserver.go` 同上
- [ ] `context.go` 同上
- [ ] 三处装配点 `unitProvider` 字段非 nil（运行时归一化生效）

**Verification:**
- [ ] `go build ./...` 通过
- [ ] `go test ./...` 通过（含集成测试 `tests/integration/server_test.go`）
- [ ] `go vet ./...` 无告警
- [ ] 手动验证：启动应用后遍历测试运行时 `BuildRawPressure` 走归一化路径（日志确认）

**Dependencies:** Task 5, Task 6, Task 8

**Files likely touched:**
- `projects/wind-daq/services/api-go/internal/bootstrap/bootstrap.go`
- `projects/wind-daq/services/api-go/pkg/apiserver/apiserver.go`
- `projects/wind-daq/services/api-go/pkg/appcontext/context.go`

**Estimated scope:** S（3 文件）

---

#### Task 11: 后端全量验证

**Description:** 运行后端全量构建、测试、vet，确认所有后端改动通过。修复任何遗留编译错误或测试失败。

**Acceptance criteria:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go vet ./...` 无告警
- [ ] `go test ./internal/... ./api/... ./tests/...` 全部通过
- [ ] 无新增 dead code 或未使用导入

**Verification:**
- [ ] 上述四条命令全部绿色
- [ ] 检查 `gofmt -l .` 无输出（格式规范）

**Dependencies:** Task 10

**Files likely touched:**
- 视情况修复 1-2 个文件

**Estimated scope:** S

---

### Checkpoint: Backend Complete

- [ ] 后端全量验证通过
- [ ] Review：三处装配点注入顺序正确，无循环依赖
- [ ] Review：旧测试（无 unitProvider）走降级路径不失败

---

### Phase 5: Frontend（前端）

#### Task 12: TraversalTestConfig TS 类型新增 pProbePressureType

**Description:** 在 `shared/types/traversal.ts:421-442` 的 `TraversalTestConfig` 接口追加 `pProbePressureType?: 'gauge' | 'absolute'` 字段。

**Acceptance criteria:**
- [ ] 字段类型为 `'gauge' | 'absolute'` 联合类型
- [ ] 可选字段（`?`）
- [ ] 中文注释说明默认 `"gauge"`

**Verification:**
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过

**Dependencies:** None（可与后端并行）

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/shared/types/traversal.ts`

**Estimated scope:** XS（1 文件）

---

#### Task 13: TraversalSettings.vue 新增压力类型开关 + saveConfig/applySavedConfig 同步

**Description:** 在 `TraversalSettings.vue` 新增"五孔压力类型"开关 UI（默认表压），在 `saveConfig`（line 362-376）的 raw 对象中追加 `pProbePressureType` 字段，在 `applySavedConfig`（line 276-335）中加载该字段（缺失时默认 `'gauge'`）。

**Acceptance criteria:**
- [ ] UI 新增开关组件（建议 NSelect 或 NRadioGroup，与现有风格一致）
- [ ] 开关默认值 `'gauge'`
- [ ] `saveConfig` 的 raw 对象包含 `pProbePressureType` 字段
- [ ] `applySavedConfig` 读取 `c.pProbePressureType`，缺失时落 `'gauge'`
- [ ] i18n 文案中英文齐全（"五孔压力类型" / "Five-Hole Pressure Type"）
- [ ] 开关放在合适步骤（建议 Step 1 TraversalHardwareStep 之前或之内，与通道配置同屏）

**Verification:**
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] 手动验证：保存配置后重载，开关状态正确恢复

**Dependencies:** Task 12

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/TraversalSettings.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/i18n.ts`（追加 i18n key）

**Estimated scope:** S（2 文件）

---

#### Task 14: TraversalHardwareStep.vue 通道映射面板显示压力类型提示

**Description:** 在 `TraversalHardwareStep.vue` 的探头通道配置面板（line 128-150）显示压力类型提示，让用户知道 P1-P5 将按何种类型归一化。提示文本从父组件 props 或 store 读取当前 `pProbePressureType`。

**Acceptance criteria:**
- [ ] 通道映射面板顶部或 P1-P5 行附近显示压力类型提示
- [ ] 提示文本随 `pProbePressureType` 变化（表压/绝压）
- [ ] 提示文案 i18n 化
- [ ] 不破坏现有通道配置交互

**Verification:**
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] 手动验证：切换压力类型开关，提示文本同步更新

**Dependencies:** Task 12, Task 13

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/TraversalHardwareStep.vue`
- `projects/wind-daq/apps/desktop-wails/frontend/src/stores/i18n.ts`（追加提示文案）

**Estimated scope:** S（2 文件）

---

#### Task 15: 前端 rawPressure 消费点检查与同步

**Description:** spec §CSV/API 语义说明指出 `rawPressure` 字段从原始通道值改为归一化 Pa+表压值，属 breaking change。需 grep 前端所有 `rawPressure` 消费点，检查是否硬编码单位标签（如 "kPa"），同步改为 "Pa" 或动态读取通道单位。

**Acceptance criteria:**
- [ ] 全前端 grep `rawPressure` 列出所有引用点
- [ ] 每个引用点的单位标签改为 "Pa"（或移除标签让数值自解释）
- [ ] 实时图表纵轴标签同步调整
- [ ] 无硬编码 "kPa"/"MPa" 残留于 rawPressure 展示路径

**Verification:**
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] 手动验证：遍历测试运行时 UI 压力曲线展示 Pa 数值

**Dependencies:** Task 13, Task 14

**Files likely touched:**
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/TraversalMain.vue`（可能）
- `projects/wind-daq/apps/desktop-wails/frontend/src/components/traversal/TraversalLiveMonitor.vue`（可能）
- 其他 grep 命中文件

**Estimated scope:** S（2-3 文件）

---

### Checkpoint: Frontend Complete

- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] 手动验证：配置开关 → 保存 → 重载 → 运行遍历测试 → 压力曲线展示正确
- [ ] Review：i18n 中英文齐全

---

### Phase 6: Final Verification（最终验证）

#### Task 16: 端到端全量验证

**Description:** 运行 spec §Commands 章节列出的全部命令，确认 spec §Success Criteria 全部 18 项达成。

**Acceptance criteria:**
- [ ] `go build -buildvcs=false ./...` 通过
- [ ] `go vet ./...` 无告警
- [ ] `go test ./internal/...` 通过
- [ ] `npm run typecheck` 通过
- [ ] `npm run build` 通过
- [ ] spec §Success Criteria 18 项全部勾选

**Verification:**
- [ ] 上述五条命令全部绿色
- [ ] spec §Success Criteria checklist 逐项核对

**Dependencies:** Task 11, Task 15

**Files likely touched:**
- 视情况修复 1-2 个文件

**Estimated scope:** S

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `BuildRawPressure` 签名变更破坏 `tests/integration/server_test.go` 等测试 | High | Task 7-8 同步更新测试；降级策略保证 unitProvider nil 时旧路径不崩 |
| 三处装配点 DeviceManager 创建顺序在 TraversalManager 之后 | Medium | Task 10 通过 `SetUnitProvider` 延迟注入，避免循环依赖 |
| `rawPressure` 语义变更导致前端图表纵轴标签错误 | Medium | Task 15 专项 grep 检查所有消费点 |
| `unitProvider` nil 时静默跳过归一化，掩盖配置错误 | Medium | CheckPreconditions Task 9 提供 ChannelMap 前置校验；运行时 warning 日志可观测 |
| 旧 JSON 配置无 `pProbePressureType` 字段时反序列化异常 | Low | Task 4 显式兜底为 `"gauge"`，与历史行为一致 |
| kgf/cm2 alias 引入后两个 key 指向同值，UnitConverter 测试覆盖不全 | Low | Task 1 补充 alias 测试用例 |

## Open Questions

无。spec v2 已修复全部 P0/P1/P2 问题，本计划基于 spec 直接分解。

## Parallelization Opportunities

- **可并行**：Task 12（前端 TS 类型）与 Phase 1-4 后端任务无依赖，可并行启动
- **必须串行**：Task 7 → Task 8（BuildRawPressure 签名变更需先于调用方适配）
- **必须串行**：Task 5 + Task 6 → Task 10（装配点注入需 DeviceManager 实现 + setter 就绪）
- **需要协调**：Task 13 与 Task 14 共享 i18n key 文件，建议串行避免合并冲突
