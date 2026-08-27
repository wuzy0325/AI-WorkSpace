# session-valve-control Specification

## Purpose
TBD - created by archiving change stabilize-measurement-calibration-core-flow. Update Purpose after archive.
## Requirements
### Requirement: 会话服务 MUST 提供阀门读写能力
会话服务 MUST 提供读取与设置阀门状态的方法，并将调用委托给当前计量设备驱动。若未绑定计量驱动，服务 MUST 返回统一错误 `ErrMeasureDeviceNotSet`。

#### Scenario: 读取阀门状态成功
- **WHEN** 会话已绑定计量驱动且请求读取阀门状态
- **THEN** 服务 MUST 调用驱动读取方法并返回当前状态值

#### Scenario: 设置阀门状态成功
- **WHEN** 会话已绑定计量驱动且请求设置阀门为 `calibration` 或 `measurement`
- **THEN** 服务 MUST 调用驱动设置方法并返回成功结果

#### Scenario: 未绑定驱动时请求阀门操作
- **WHEN** 会话未绑定计量驱动且调用读取或设置阀门方法
- **THEN** 服务 MUST 返回 `ErrMeasureDeviceNotSet`

### Requirement: 阀门 HTTP API MUST 映射服务层行为
`GET /api/v1/session/valve` 与 `PUT /api/v1/session/valve` MUST 调用会话服务阀门方法。`PUT` 输入状态 MUST 限制为 `calibration` 或 `measurement`。

#### Scenario: GET 返回阀门状态
- **WHEN** 客户端调用 `GET /api/v1/session/valve`
- **THEN** 接口 MUST 返回当前阀门状态并与驱动读值一致

#### Scenario: PUT 输入非法状态
- **WHEN** 客户端调用 `PUT /api/v1/session/valve` 且状态值不在允许集合
- **THEN** 接口 MUST 返回参数错误且不得调用驱动写入

