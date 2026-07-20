# Implementation Plan: Probe Interpolator (3/5/7-Hole Unified Desktop App)

## Overview

新建 `projects/probe-interpolator`，把现有 5 孔、3 孔独立 Wails 程序与新增的 7 孔插值合并到一个安装包。启动选择页 → 进入该探针专属工作区，会话内固定。三种工作区共用 5 孔外观框架，各自维护输入区与结果列。算法包直接复用 `shared/algorithms/go/{fivehole,threehole,sevenhole}/interpolation/`。旧 5/3 孔程序标记 deprecated、保留代码。

参考：[projects/probe-interpolator/SPEC.md](../SPEC.md)

## Architecture Decisions

- **拷贝 5 孔基础改造，不破坏已有**：以 `projects/five-hole-interpolator/apps/desktop-wails` 为模板拷贝，保留 5 孔现有 Taskfile/wails.json/main.go 结构，最小改动。
- **后端每种探针一个 service 文件**：避免一个巨型 App struct 混三种逻辑，便于独立测试与维护。
- **前端先迁移后抽取共用组件**：5 孔 App.vue 是 879 行，迁移时先整段拷到 `FiveHoleWorkspace.vue`，跑通后再抽取 `shared/AppHeader`、`shared/ResultTable`、`shared/FilePicker`。不在一开始就过度设计抽象。
- **7 孔算法包 API 与 5/3 孔不同**：后端 `seven_hole_service.go` 需按文件名 basename 匹配 sector（`7.prb` → inner，`1.prb`~`6.prb` → outer sector 1-6），分别调用 `LoadInnerPrbLines` + `LoadOuterPrbLines`。
- **7 孔 Alpha/Beta 语义反转**：7 孔结果中 Alpha=侧滑、Beta=迎角（与 5 孔反转），UI 文案必须按 7 孔语义显示。
- **垂直切片**：每个探针端到端独立可验证（后端 service + 前端 workspace + 测试），不按"先所有后端再所有前端"横向切。

## Dependency Graph

```
shared/algorithms/go/{fivehole,threehole,sevenhole}/interpolation  (已存在，无需改动)
    │
    ├── Task 1: 拷贝 5 孔目录骨架 + 改名 (go.mod/wails.json/main.go/Taskfile)
    │       │
    │       └── Task 2: 启动选择页后端 + 前端 (probe_selector.go + ProbeSelectPage.vue + App.vue 路由)
    │               │
    │               ├── Task 3: 5 孔端到端 (five_hole_service.go + FiveHoleWorkspace.vue + 测试)
    │               │
    │               ├── Task 4: 3 孔端到端 (three_hole_service.go + ThreeHoleWorkspace.vue + 测试)
    │               │
    │               └── Task 5: 7 孔端到端 (seven_hole_service.go + SevenHoleWorkspace.vue + 测试)
    │                       │
    │                       └── Checkpoint: 三种探针端到端可用
    │                               │
    │                               └── Task 6: 抽取共用组件 (shared/AppHeader + ResultTable + FilePicker)
    │                                       │
    │                                       └── Task 7: 旧 5/3 孔程序 deprecation 标记
    │                                               │
    │                                               └── Task 8: v0.1.0 release note + 最终验证
```

## Task List

### Phase 1: 骨架与启动选择页

- [ ] Task 1: 拷贝 5 孔目录骨架并改名
- [ ] Task 2: 实现启动选择页（后端 + 前端路由）

### Checkpoint: 骨架可启动
- [ ] `wails3 dev` 能起来，看到三按钮选择页（按钮先空跳转）
- [ ] `go build` + `npm typecheck` + `npm build` 全绿

### Phase 2: 三种探针端到端

- [ ] Task 3: 5 孔工作区端到端
- [ ] Task 4: 3 孔工作区端到端
- [ ] Task 5: 7 孔工作区端到端

### Checkpoint: 三种探针端到端可用
- [ ] 5 孔加载多 .prb + 单点 + 批量 CSV，结果与旧 5 孔程序一致
- [ ] 3 孔加载单 .prb + 单点 + 批量 CSV，结果与旧 3 孔程序一致
- [ ] 7 孔加载 7 个 .prb + 单点 + 批量 CSV，大小角度模式自动判定显示
- [ ] 7 孔 UI 文案 Alpha=侧滑、Beta=迎角

### Phase 3: 收尾

- [ ] Task 6: 抽取共用组件（可选，视实际重复度决定）
- [ ] Task 7: 旧 5/3 孔程序 deprecation 标记
- [ ] Task 8: v0.1.0 release note + 最终验证

### Checkpoint: Complete
- [ ] SPEC Success Criteria 全部勾选
- [ ] `go test ./...` + `npm typecheck` + `npm build` 全绿
- [ ] 旧 5/3 孔程序 README 顶部有 deprecation 声明
- [ ] v0.1.0 release note 完成

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| 7 孔算法包 API 与 5/3 孔差异大，后端适配工作量被低估 | Med | Task 5 单独切片，先验证 `LoadInnerPrbLines` + `LoadOuterPrbLines` 调用链跑通，再做 UI |
| 7 孔 Alpha/Beta 语义反转，前端文案混淆 | High | Task 5 acceptance criteria 明确要求 UI 显示"侧滑角 α / 迎角 β"，code review 时重点检查 |
| 5 孔 App.vue 879 行直接迁移可能引入回归 | Med | Task 3 验证用"相同 .prb + 相同输入"对比新旧程序结果一致 |
| 抽取共用组件过早导致过度设计 | Low | Task 6 标记为可选，先跑通三种工作区再评估抽取收益 |
| Wails binding 同步问题（新增 backend 方法后 frontend bindings 不同步） | Med | 每个 service 任务收尾时跑 `check-wails-bindings.ps1` 验证 |
| 旧程序 deprecation 标记影响老用户 | Low | README 标注"代码保留、可继续编译、仅修关键 bug"，不删任何东西 |

## Open Questions

(继承自 SPEC.md Open Questions，实现时按默认值处理，若遇阻再问)

1. 7 孔 .prb 文件名规则：默认按 basename 识别 `7.prb` / `1.prb`~`6.prb`
2. 7 孔 CSV 导入列：默认只要求 `P1`~`P7` + `Patm` + `Tatm`
3. 启动选择页记忆：默认不记忆
4. 帮助文档：默认合并成一份按探针分章节

## Parallelization Opportunities

- **Task 3 / 4 / 5 可并行**：三种探针工作区相互独立，理论上可分三个 agent 并行做。但建议先串行做 Task 3（5 孔）作为参照，再并行 Task 4 + 5，因为 5 孔是迁移成本最低的（直接拷贝），先做能沉淀共用模式。
- **Task 7 可与 Task 6 并行**：旧程序 deprecation 标记与新程序抽取共用组件互不影响。
- **不可并行**：Task 1 → Task 2 必须串行（选择页依赖骨架存在）；Task 6 必须在 Task 3/4/5 之后（要有三个工作区才能评估重复度）。
