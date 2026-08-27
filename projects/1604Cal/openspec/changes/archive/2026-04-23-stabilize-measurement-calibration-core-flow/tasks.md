## 1. 计量状态机与压力点流程

- [x] 1.1 在 `internal/application/measurement/service.go` 扩展状态常量、迁移表与 `SetState` 逻辑，并确保非法迁移返回错误
- [x] 1.2 在 `internal/application/calibration/service.go` 的压力点采集流程中实现 `pressurizing/stabilizing/collecting/completed/error` 状态更新与 `calibration.point_status` 事件发布
- [x] 1.3 在标定服务实现暂停记录与恢复续跑（基于 `currentPoint` 与压力点状态）
- [x] 1.4 补充状态机与暂停恢复单元测试，覆盖初始化状态、合法迁移、非法迁移、恢复起点选择

## 2. 会话阀门控制与设备校准 API

- [x] 2.1 在 `internal/application/session/service.go` 实现或完善 `ReadValveStatus`/`SetValveStatus`，并统一处理 `ErrMeasureDeviceNotSet`
- [x] 2.2 确认并修正 `GET/PUT /api/v1/session/valve` handler 调用链，补充状态值校验（`calibration`/`measurement`）
- [x] 2.3 在 `internal/device/interfaces.go` 扩展 `MeasureDriver` 的 `CalibrateZero` 与 `CalibrateFullScale` 契约
- [x] 2.4 在 `internal/infrastructure/driver/wtn1604_driver.go` 实现调零与满量程校准命令及错误返回
- [x] 2.5 在会话服务与 HTTP 层新增 `POST /api/v1/session/calibrate-zero`、`POST /api/v1/session/calibrate-full-scale` 并完成请求参数校验

## 3. 报警决策闭环与稳定性 SSE

- [x] 3.1 在 `internal/workflow/alarm_service.go` 扩展决策常量并更新 `ValidateDecision` 仅允许 `continue/skip/recollect/stop`
- [x] 3.2 在 `internal/application/calibration/service.go` 实现 `ResolveAlarm` 与 `alarmCh` 协同，在自动采集循环执行四类决策分支
- [x] 3.3 实现报警决策结果事件发布，确保 `calibration.alarm.resolved` 包含可追踪上下文
- [x] 3.4 创建 `StabilityMonitor` 时注入 `s.publish`，确保发布 `calibration.stability.progress/achieved/lost` 事件且进度范围为 0-100
- [x] 3.5 补充报警与稳定性相关测试，覆盖非法决策拒绝、skip/recollect/stop 行为与稳定事件载荷

## 4. 报告模板列表与匹配

- [x] 4.1 在 `internal/report/report_service.go` 实现模板目录扫描与 `{pointCount}{s|m}.xlsx` 命名解析
- [x] 4.2 实现 `GetTemplates` 与 `MatchTemplate(pointCount, mode)`，覆盖 `single->s`、`roundTrip->m` 映射与未命中处理
- [x] 4.3 新增 `GET /api/v1/reports/templates` handler 与路由绑定，并返回标准化模板信息
- [x] 4.4 补充模板服务与接口测试，覆盖合法模板、非法文件名、模板缺失场景

## 5. 验证与回归

- [x] 5.1 执行并通过 `go test ./...`，处理新增能力引入的回归问题
- [x] 5.2 执行并通过 `go vet ./...`，修复并发与错误处理告警
- [x] 5.3 若仓库已接入 staticcheck，执行并通过 `GOFLAGS=-buildvcs=false staticcheck ./...`（工作区同时存在 Git/SVN，需关闭 buildvcs）
- [x] 5.4 按接口清单进行手工联调验证：阀门读写、校准 API、模板列表、状态与稳定性 SSE、报警决策流程
