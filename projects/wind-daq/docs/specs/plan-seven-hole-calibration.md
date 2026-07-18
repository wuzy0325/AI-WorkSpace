# Implementation Plan: 七孔探针校准模块

> 关联规格：[spec-seven-hole-calibration.md](./spec-seven-hole-calibration.md)（v1.1）
> 状态：Phase 2 计划，待人工批准后进入 BUILD
> 日期：2026-07-16
> 实现范围：**MVP 先行**（内区 3 类图表 + 核心流程，外区 3 类图表第二阶段）
> 采样参数：5 次子采样 + 2000ms 驻留（推荐配置）

## Overview

在 `services/api-go/internal/core/calibration/` 新建七孔算法主体（`seven_hole.go` / `seven_hole_formulas.go` / `seven_hole_uncertainty.go`），复用 `AutomaticCalibration` 模板方法，仅扩展 `moveToPoint` 分支与 `CalibrationType` 常量。`CalPoint` 结构体新增 `MotionCoordinates` 与 `Region`/`Sector` 字段（向后兼容，五孔等已有模块不受影响）。后端新增 `/api/calibration/sevenhole-preview` 与 `/api/calibration/sevenhole-start` 路由（单段命名，当前路由器不支持二级路径），前端通过预览 API 获取点位（**禁止前端本地实现点位生成算法**）。CSV schema 新增内区/外区两套 26 列表头（含采样元数据、不确定度、边界标记），事件契约新增必需事件 `OnRegionChanged`。前端 MVP 阶段实现配置向导、主操作画面、内区 3 类特性曲线图；外区 3 类图表、准备阶段预热倒计时、完整模式 673 点验证留待第二阶段。

## Architecture Decisions

| 决策 | 理由 |
|---|---|
| `CalPoint` 新增 `MotionCoordinates`/`Region`/`Sector` 字段，向后兼容 | 五孔等已有模块不填新字段时走默认 `Coordinates` 路径，零回归；七孔显式填充双坐标 |
| 七孔 `moveToPoint` 复用五孔的硬编码分支模式（非多态） | 与现有 `automatic_calibration.go:276` 一致，不引入新抽象；后续若有第三种类型再重构 |
| 点位生成集中在后端 Go Core `GenerateSevenHolePoints` | 依据 AGENTS.md "前端零校准算法"硬约束；五孔历史遗留 `motionCalibrationUtils.ts` 不作为七孔参考 |
| 前端通过 `/api/calibration/sevenhole/preview` 获取点位 | 配置向导实时显示总点数/预计耗时；离线场景提示"请先连接后端"而非 fallback |
| `OnRegionChanged` 作为必需事件，payload 含 `prevRegion`/`boundaryFlag` | 解决评审 Medium 3："可选事件在验收中成必需"矛盾；UI 顶部状态栏据此显示当前区域 |
| 不确定度合成采用 `u_c = √(\|Σ cᵢu_B,i\|² + Σ cᵢ²u(A,i)²)` | B 类完全正相关取和的绝对值，A 类独立取平方和开方；5.5 节算例可复算验证 |
| 双校准模式：完整模式（673 点，产品默认）+ 数据集模式（481 点，验证基准） | 解决评审 High 1："默认点位/数据集/验收三个产品不一致"；两种模式各自有唯一测试基线 |
| 并列最大压力 tie-break：P7 优先 + 编号小优先 + 滞回（仅相邻扇区）+ `boundary_flag` | 解决评审 Medium 1："并列最大时无确定性分区规则"；TIE_BREAK_TOLERANCE=5 Pa 可配置 |
| MVP 阶段只实现内区 3 类图表，外区 3 类图表第二阶段 | 优先验证算法正确性（内区公式 + 分区判定 + 马赫数），再完善可视化；符合用户"最小启用集"偏好 |
| 沿用项目约定 `plan-*.md` / `tasks-*.md` 与 spec 同目录 | 匹配既有 8 份 plan / 3 份 tasks 文件命名 |

## Assumptions（Phase 3 开始前需确认）

1. **Q1 CSV 方案**：采用 A（内区/外区分两个文件），与数据集格式一致。文件命名 `<配置名>_<工况>_<区域>.csv`。
2. **Q4 采样次数**：5 次（推荐配置），平衡精度与耗时。
3. **Q7 球罐门控超时**：300s，与五孔模块一致。
4. **Q8 驻留时间**：2000ms，等待气流稳定的保守值。
5. **Q9 图表实现范围**：MVP 阶段实现内区 3 类（Kα-Kβ 特性曲线、α-K0 总压系数曲线、α-Ks 静压系数曲线），外区 3 类第二阶段。
6. **Q10 证书导出**：CSV 格式，与原始数据一致。
7. **`CalPoint` 扩展向后兼容性**：五孔/三孔/总压/总温模块不填 `MotionCoordinates` 时，`moveToPoint` 默认走 `Coordinates`（已有行为）；只有 `TypeSevenHole` 走双坐标路径。
8. **前端 `motionCalibrationUtils.ts` 处置**：五孔的历史遗留文件本期不清理，仅在七孔模块严格禁止新增类似文件。
9. **数据集 CSV 编码问题**：数据集文件名含中文且存在 GBK 乱码（见 `W532.202608.P.7H.1-01/*.csv` 首行），回归测试时需用 UTF-8 转码后的副本或直接用 `_headers_utf8.txt` 对照表头。

## Dependency Graph

```
Phase 1: Core 层算法基础（独立于 UI）
  Task 1 (CalPoint 扩展 + TypeSevenHole 常量) ──┐
  Task 2 (七孔公式 formulas) ──────────────────┤
  Task 3 (分区判定 + tie-break) ────────────────┤
  Task 4 (不确定度计算) ────────────────────────┤
                                                 │
Phase 2: Core 层算法主体                         ▼
  Task 5 (SevenHoleAlgorithm + AcquireDataWithChannels) ──┬──> Task 6 (GenerateSevenHolePoints 双坐标)
                                                            │
                                                            ▼
Phase 3: 后端集成                             ┌──> Task 7 (CSV schema 内/外区两套表头)
                                              │
                                              ├──> Task 8 (CalibrationConfigDTO + ValidateConfig 七孔角色)
                                              │
                                              ├──> Task 9 (createAlgorithm 工厂分支)
                                              │
                                              ├──> Task 10 (moveToPoint 七孔分支 + RealtimeEvent 扩展)
                                              │
                                              ├──> Task 11 (EventPublisher OnRegionChanged 必需事件)
                                              │
                                              ├──> Task 12 (server.go 路由 /sevenhole/preview + /sevenhole/start)
                                              │
                                              └──> Task 13 (Wails backend CalibrationPreviewSevenHole binding)
                                                            │
                                                            ▼
Phase 4: 数据集回归测试（算法正确性闸门）
  Task 14 (481 点数据集模式回归测试) ──┬──> Task 15 (5 个黄金用例角度换算测试)
                                       ├──> Task 16 (5 个 tie-break 构造用例测试)
                                       └──> Task 17 (中心点不确定度 7 步算例测试)
                                                            │
                                                            ▼
Phase 5: 前端 MVP
  Task 18 (calibrationApi.ts 新增 previewSevenHole) ──┐
  Task 19 (calibrationStore.ts 七孔状态字段) ──────────┤
  Task 20 (SevenHoleSettings.vue 3 步配置向导) ────────┤
  Task 21 (SevenHoleMain.vue 主画面 + 顶部状态栏) ─────┤
  Task 22 (SevenHoleCharts.vue 内区 3 类图表) ─────────┤
  Task 23 (OnRegionChanged 前端订阅 + 区域显示) ───────┤
  Task 24 (状态恢复协议 recoveryFromBackend) ──────────┘
                                                            │
                                                            ▼
Phase 6（第二阶段，本期不实现）: 外区 3 类图表 + 完整模式 673 点验证
```

## Risks & Mitigations

| 风险 | 等级 | 缓解策略 |
|---|---|---|
| `CalPoint` 扩展导致五孔等已有模块回归 | 高 | Task 1 必须先跑全量 `go test ./internal/core/calibration/...` 确保绿；新字段为可选，JSON omitempty |
| 七孔 α 符号公式负号被误删（评审 Critical 1） | 高 | Task 15 黄金用例 G1~G5 作为单元测试硬门槛，CI 必须通过 |
| 压力基准混用导致大气压重复叠加（评审 Critical 2） | 高 | Task 2 公式实现中明确注释"A→B 不转换、A→C 仅 Ma 入口转换"；Task 14 数据集回归验证 Ma=0.241 |
| 前端误引入点位生成算法（评审 High 4） | 中 | Task 20 配置向导强制调 `/preview` API；Code review 检查 `seven-hole/` 目录无 `.ts` 算法文件 |
| `OnRegionChanged` 事件遗漏首点推送 | 中 | Task 11 单元测试覆盖首点（prevRegion=null）与切换场景；Task 23 前端订阅测试 |
| 不确定度算例无法复算（评审 High 2） | 中 | Task 17 中心点 7 步算例作为单元测试基准，数值精确匹配 |
| 数据集 CSV 中文乱码导致回归测试失败 | 低 | Task 14 使用 UTF-8 转码副本或 `_headers_utf8.txt` 对照；不修改原始数据集文件 |
| MVP 阶段外区点位无图表验证 | 低 | Task 22 内区图表完成后，外区数据仍写入 CSV 可手动验证；Phase 6 补外区图表 |
| 并列最大压力 tie-break 滞回逻辑跨大尺度误触发 | 中 | Task 16 构造用例覆盖"跨大跨度不滞回"场景；3.2 节明确仅相邻扇区生效 |

## Verification Checkpoints

| 检查点 | 验证内容 | 通过标准 |
|---|---|---|
| CP1（Phase 1 完成后） | Core 层算法独立可测 | `go test ./internal/core/calibration/...` 全绿，含五孔既有测试无回归 |
| CP2（Phase 3 完成后） | 后端 API 可独立验证 | `curl POST /api/calibration/sevenhole/preview` 返回 673 点（完整模式）/ 481 点（数据集模式） |
| CP3（Phase 4 完成后） | 算法正确性闸门 | 481 点数据集回归测试全绿；5 个黄金用例 + 5 个 tie-break 用例 + 不确定度算例全部通过 |
| CP4（Phase 5 完成后） | MVP 端到端可用 | 前端配置向导 → 启动校准 → 内区 169 点走完 → CSV 正确 → 状态恢复正常 |
| CP5（最终） | 全量验收 | spec §11.1 算法正确性 + §11.2 流程完整性（MVP 子集）+ §11.3 性能验收全部通过 |

## Out of Scope（本期不实现）

- 外区 3 类特性曲线图（Kθ-Kφ、φ-K0[n]、φ-Ks[n]）—— Phase 6 第二阶段
- 完整模式 673 点端到端流程验收 —— Phase 6 第二阶段（数据集模式 481 点已覆盖算法正确性）
- 五孔历史遗留 `motionCalibrationUtils.ts` 清理 —— 单独议题
- 校准证书 PDF/Excel 导出 —— Q10 选 CSV，其他格式后续按需
- 七孔实测反算应用阶段（spec §4.3 提及的"边界不确定区双解输出"）—— 属于应用层，非校准模块范围
