# stabilize-measurement-calibration-core-flow 手工联调验证记录

> 日期: 2026-04-23  
> 范围: OpenSpec 任务 5.4（阀门读写、校准 API、模板列表、状态与稳定性 SSE、报警决策流程）

## 验证口径

- 本次联调在本地代码环境执行，采用 `httptest + fake driver` 的方式模拟 API 层到应用层链路。
- 目标是验证接口契约与核心事件语义，不依赖现场串口/网口硬件。
- 验证命令均在仓库根目录执行。

## 接口清单与结果

1. 阀门读写
   - 覆盖测试: `TestSessionValvePutUpdatesStatus`
   - 结果: 通过（`PUT /api/v1/session/valve` 后，`GET /api/v1/session/valve` 返回 `calibration`）

2. 校准 API
   - 覆盖测试:
     - `TestSessionCalibrateZeroEndpoint`
     - `TestSessionCalibrateFullScaleEndpoint`
   - 结果: 通过（请求参数正确传递到会话层与驱动层）

3. 模板列表与匹配
   - 覆盖测试:
     - `TestListTemplatesReturnsArray`
     - `TestGetTemplatesParsesTemplateMetadata`
     - `TestMatchTemplateResolvesExpectedFilename`
   - 结果: 通过（模板列表可返回，`single/roundTrip` 匹配规则生效）

4. 状态与稳定性 SSE
   - 覆盖测试:
     - `TestSSEEndpointStreamsEvents`（SSE 通道可正常推送事件）
     - `TestCollectPublishesPointStatusEvents`（`calibration.point_status` 状态事件）
     - `TestStabilityMonitorPublishesProgressAndAchieved`
     - `TestStabilityMonitorPublishesLostEvent`
   - 结果: 通过（状态/稳定性事件类型与进度语义满足预期）

5. 报警决策流程
   - 覆盖测试:
     - `TestResolveAlarmSupportsNewDecisions`
     - `TestCollectPointAlarmDecisionSkip`
     - `TestCollectPointAlarmDecisionStop`
     - `TestCollectPointAlarmDecisionRecollect`
   - 结果: 通过（`continue/skip/recollect/stop` 决策链路闭环）

## 执行命令

```powershell
go test -v ./internal/api/http -run "TestSessionValvePutUpdatesStatus|TestSessionCalibrateZeroEndpoint|TestSessionCalibrateFullScaleEndpoint|TestListTemplatesReturnsArray|TestSSEEndpointStreamsEvents" -count=1

go test -v ./internal/application/calibration -run "TestCollectPublishesPointStatusEvents|TestResolveAlarmSupportsNewDecisions|TestCollectPointAlarmDecisionSkip|TestCollectPointAlarmDecisionStop|TestCollectPointAlarmDecisionRecollect" -count=1

go test -v ./internal/workflow -run "TestStabilityMonitorPublishesProgressAndAchieved|TestStabilityMonitorPublishesLostEvent" -count=1

go test -v ./internal/report -run "TestGetTemplatesParsesTemplateMetadata|TestMatchTemplateResolvesExpectedFilename" -count=1
```

## 结论

- OpenSpec 5.4 所列联调项已完成验证。
- 当前验证结果支持将 `stabilize-measurement-calibration-core-flow` 的任务状态推进为全部完成。
