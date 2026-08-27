## Context

当前后端已具备计量、标定、会话、驱动与报告等基础模块，但核心流程仍存在“接口已定义、行为未闭环”的断点，尤其体现在状态流转、设备控制、报警决策与事件推送。前端依赖 SSE 与 HTTP API 驱动实时界面，若后端状态与事件语义不完整，现场操作会出现“可见但不可控”或“可控但不可观测”的问题。

本设计以 `docs/开发任务清单.md` 的 P0/P1 任务为基线，聚焦“先恢复主流程可靠性，再补齐关键能力接口”，并保持以下约束：
- 不破坏现有接口路径与基础协议。
- 兼容当前 WTN1604 驱动与会话装配方式。
- 遵循现有领域分层（application/workflow/infrastructure/api）。

## Goals / Non-Goals

**Goals:**
- 将计量采集状态扩展为完整生命周期状态机，并确保状态迁移可校验。
- 建立压力点状态追踪与暂停恢复能力，保证采集可中断可继续。
- 打通阀门控制从 API 到 driver 的调用链，确保读写行为真实可执行。
- 为报警处理建立“判定 + 决策执行”闭环，支撑自动采集流程分支。
- 确保稳定性监控事件可靠发布到 SSE，前端可稳定展示进度与达稳。
- 提供报告模板列表与自动匹配规则，减少人工选型错误。
- 实现调零与满量程校准接口，补齐会话与驱动能力矩阵。

**Non-Goals:**
- 不在本次变更实现 P2/P3 项（自定义压力点、N 次平均、历史会话、拟合优化）。
- 不重构整体架构或引入新的消息中间件。
- 不改造前端 UI，仅保证前端所需后端契约可用。

## Decisions

1. **采集状态采用显式状态机 + 迁移表校验**
   - Decision: 在 `measurement/service.go` 增加 `pressuring/stabilizing/completed/error` 等状态，并通过统一迁移表控制合法转换。
   - Rationale: 显式建模比隐式布尔标记更可测试、更易定位异常状态。
   - Alternative considered: 使用多个布尔字段表示阶段（如 `isStable/isCollecting`），被拒绝，因组合状态易失真。

2. **压力点生命周期状态写入领域对象并实时发布事件**
   - Decision: 在采集流程关键节点更新 `PressurePoint.Status`，并统一发布 `calibration.point_status`。
   - Rationale: 前后端共享同一状态源，暂停恢复与审计都可复用该字段。
   - Alternative considered: 仅通过瞬时事件通知前端，不落对象状态；被拒绝，因恢复流程缺少可靠依据。

3. **暂停恢复使用“当前点索引 + 点状态”双重定位**
   - Decision: 暂停时记录 `currentPoint`，恢复时优先从未完成或中断点继续。
   - Rationale: 仅用索引恢复在异常中断场景下不够安全，状态可辅助纠偏。
   - Alternative considered: 恢复后从头重跑，被拒绝，因现场耗时高且影响效率。

4. **阀门控制沿用 session 服务统一入口**
   - Decision: 在 `session/service.go` 增加/完善 `ReadValveStatus` 与 `SetValveStatus`，由 handler 统一调用。
   - Rationale: 避免 HTTP 层直接触达 driver，保持分层边界。
   - Alternative considered: handler 直接调用驱动实例，被拒绝，因破坏应用层职责。

5. **报警决策通过校验 + channel 分发进入采集循环**
   - Decision: `alarm_service` 负责合法性校验，`calibration/service` 负责将决策写入 `alarmCh` 并在主循环执行。
   - Rationale: 解耦“决策是否合法”和“决策如何执行”，便于扩展动作。
   - Alternative considered: 在 API handler 直接修改采集状态，被拒绝，因并发一致性风险高。

6. **稳定性监控采用 publisher 注入而非全局依赖**
   - Decision: 构建 `StabilityMonitor` 时显式传入 `s.publish`。
   - Rationale: 保持监控组件可测试与可替换，避免隐藏依赖。
   - Alternative considered: 监控模块内部依赖全局事件总线，被拒绝，因测试复杂度高。

7. **报告模板匹配采用文件命名约定优先**
   - Decision: 使用 `{pointCount}{s|m}.xlsx` 规则，`single -> s`，`roundTrip -> m`。
   - Rationale: 规则简单、可被运维与测试直接验证。
   - Alternative considered: 额外维护模板元数据配置文件，被拒绝，因维护成本高于当前收益。

8. **调零/满量程能力上提到驱动接口契约**
   - Decision: 在 `MeasureDriver` 增加 `CalibrateZero` 和 `CalibrateFullScale`，由 session 服务对外暴露 API。
   - Rationale: 明确设备能力边界，避免不同型号驱动行为漂移。
   - Alternative considered: 仅在具体驱动上临时扩展方法，被拒绝，因上层无法统一调用。

## Risks / Trade-offs

- [Risk] 状态机扩展后可能与旧前端状态枚举不兼容 -> Mitigation: 保留已有状态语义并补充前端可识别映射，联调时覆盖状态枚举。
- [Risk] 报警决策引入并发通道后可能出现阻塞 -> Mitigation: 明确 `alarmPending` 状态保护，确保发送接收点成对出现并加超时控制。
- [Risk] 报告模板命名不规范导致匹配失败 -> Mitigation: 在模板列表接口中返回可用模板并提供明确错误信息。
- [Risk] 新增驱动校准命令与设备固件版本不兼容 -> Mitigation: 在驱动层增加能力检查与错误包装，避免静默失败。
- [Trade-off] 采用命名约定简化实现，但弱化了复杂模板元数据表达能力 -> Mitigation: 预留后续元数据扩展点。

## Migration Plan

1. 先落地 P0（状态机、压力点状态、阀门控制），保证主链路可运行。
2. 再落地 P1（报警闭环、稳定性 SSE、模板系统、校准 API），完善可观测与可操作能力。
3. 联调前端事件与接口，补齐回归测试（状态迁移、报警动作、阀门控制、模板匹配、校准命令）。
4. 部署采用灰度验证：先测试环境全流程跑通，再推广到现场环境。
5. 回滚策略：按功能点独立回滚对应提交；接口新增保持后向兼容，不影响旧调用路径。

## Open Questions

- `skip` 压力点是否需要在最终报告中显式标注（状态/备注）？
- 调零与满量程 API 的请求体是否允许多通道部分失败并返回逐通道结果？
- 稳定性事件的前端节流策略是否需要与后端 200ms 推送频率协同配置？
