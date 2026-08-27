# device-zero-fullscale-calibration Specification

## Purpose
TBD - created by archiving change stabilize-measurement-calibration-core-flow. Update Purpose after archive.
## Requirements
### Requirement: 计量驱动接口 MUST 提供调零与满量程校准能力
计量驱动接口 MUST 定义 `CalibrateZero(ctx, channels)` 与 `CalibrateFullScale(ctx, channels, fullScaleValue)` 方法。方法 MUST 支持 `context` 取消与超时，返回值 MUST 包含每个通道的校准结果。

#### Scenario: 调零校准调用
- **WHEN** 会话服务请求执行调零并传入通道列表
- **THEN** 驱动 MUST 执行调零命令并返回对应通道结果

#### Scenario: 满量程校准调用
- **WHEN** 会话服务请求执行满量程校准并传入通道列表与满量程值
- **THEN** 驱动 MUST 执行满量程命令并返回对应通道结果

#### Scenario: 上下文取消
- **WHEN** 校准过程中 `context` 被取消或超时
- **THEN** 驱动 MUST 中止操作并返回取消/超时错误

### Requirement: 会话 API MUST 暴露校准端点
HTTP 层 MUST 提供 `POST /api/v1/session/calibrate-zero` 与 `POST /api/v1/session/calibrate-full-scale`，并将请求参数转发到会话服务校准方法。

#### Scenario: 调零接口成功响应
- **WHEN** 客户端调用调零接口且设备执行成功
- **THEN** 接口 MUST 返回校准结果数据并标识成功

#### Scenario: 满量程接口参数校验失败
- **WHEN** 客户端调用满量程接口但未提供有效满量程值
- **THEN** 接口 MUST 返回参数校验错误且不得调用驱动

