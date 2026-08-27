## Why

当前计量与标定主流程存在多个功能缺口：采集状态机无法表达完整生命周期、压力点状态缺少可追踪性、阀门控制链路不完整、报警决策缺少执行闭环、稳定性与报告模板能力未形成稳定接口、调零/满量程校准尚未落地。这些问题直接影响采集流程可靠性、前后端协同与现场可操作性，需要优先补齐。

## What Changes

- 补齐计量采集状态机，覆盖打压、稳定、采集、完成、异常与暂停恢复路径。
- 为标定压力点建立显式状态流转与暂停恢复机制，保证每个压力点可观测、可继续。
- 打通会话层阀门控制调用链，确保 API 能真实读取/设置阀门状态。
- 完成报警决策执行闭环，支持 continue/skip/recollect/stop 四类动作。
- 将稳定性监控事件稳定接入 SSE，提供进度与达稳事件。
- 建立报告模板枚举与按点数/模式自动匹配能力，并暴露模板查询 API。
- 实现计量设备调零与满量程校准接口、驱动实现与 HTTP API。
- 本次变更聚焦 P0/P1 核心流程；P2/P3 优化项（如自定义压力点、N 次平均、历史会话）留作后续独立 change。

## Capabilities

### New Capabilities
- `measurement-state-machine`: 定义并约束计量采集服务的完整状态集合与合法迁移规则。
- `calibration-point-status-resume`: 定义压力点状态跟踪、状态事件推送与暂停恢复行为。
- `session-valve-control`: 定义会话层阀门状态读取/设置的服务契约与 API 行为。
- `calibration-alarm-decision-flow`: 定义报警决策校验与自动采集流程中的决策执行语义。
- `calibration-stability-sse`: 定义稳定性进度与达稳 SSE 事件的触发频率与载荷要求。
- `report-template-resolution`: 定义报告模板列表、命名解析与模板匹配规则。
- `device-zero-fullscale-calibration`: 定义调零/满量程校准在驱动、服务与 API 层的行为契约。

### Modified Capabilities
- _None_

## Impact

- Affected code:
  - `internal/application/measurement/service.go`
  - `internal/application/calibration/service.go`
  - `internal/application/calibration/collector.go`
  - `internal/application/session/service.go`
  - `internal/workflow/alarm_service.go`
  - `internal/report/report_service.go`
  - `internal/device/interfaces.go`
  - `internal/infrastructure/driver/wtn1604_driver.go`
  - `internal/api/http/handlers/`
- Affected APIs:
  - existing: `GET/PUT /api/v1/session/valve`
  - new: `GET /api/v1/reports/templates`
  - new: `POST /api/v1/session/calibrate-zero`
  - new: `POST /api/v1/session/calibrate-full-scale`
- Affected runtime behavior:
  - 新增/完善 SSE 事件：`measurement.state_changed`、`calibration.point_status`、`calibration.stability.*`、`calibration.alarm.resolved`
- Dependencies:
  - 与现有驱动协议、SSE 发布通道、报告模板目录命名规则保持兼容。
