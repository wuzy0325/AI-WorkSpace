package events

// 事件类型常量，集中定义所有 SSE 广播事件名称。
// 按发布模块分组，格式为 <模块>.<动作>。

// Session 会话生命周期事件
const (
	EventSessionDeviceBound   = "session.device_bound"
	EventSessionDeviceUnbound = "session.device_unbound"
	EventSessionStateChanged  = "session.state.changed"
)

// Device 设备状态事件
const (
	EventDeviceStatusChanged   = "device.status.changed"
	EventDeviceConnectProgress = "device.connect_progress" // 连接阶段进度
)

// Measurement 计量工作流事件
const (
	EventMeasurementStateChanged     = "measurement.state_changed"
	EventMeasurementDataUpdated      = "measurement.data_updated"
	EventMeasurementDataCollected    = "measurement.data.collected"
	EventMeasurementPointStatus      = "measurement.point.status"
	EventMeasurementAlarmTriggered   = "measurement.alarm.triggered"
	EventMeasurementAlarmResolved    = "measurement.alarm.resolved"
	EventMeasurementStabilityUpdate  = "measurement.stability.update"
	EventMeasurementStabilityTimeout = "measurement.stability.timeout"
)

// Calibration 标定工作流事件
const (
	EventCalibrationPointStatus       = "calibration.point_status"
	EventCalibrationAlarmResolved     = "calibration.alarm.resolved"
	EventCalibrationStabilityUpdate   = "calibration.stability.update"
	EventCalibrationStabilityChanged  = "calibration.stability.changed"
	EventCalibrationStabilityLost     = "calibration.stability.lost"
	EventCalibrationStabilityProgress = "calibration.stability.progress"
	EventCalibrationStabilityAchieved = "calibration.stability.achieved"
	EventCalibrationStabilityTimeout  = "calibration.stability.timeout"
)

// AutoCollection 自动采集事件
const (
	EventAutoCollectionStarted   = "autoCollection.started"
	EventAutoCollectionStopped   = "autoCollection.stopped"
	EventAutoCollectionCompleted = "autoCollection.completed"
	EventAutoCollectionError     = "autoCollection.error"
)

// Point 标定点事件
const (
	EventPointStarted   = "point.started"
	EventPointCompleted = "point.completed"
	EventPointRecollect = "point.recollect"
	EventPointSkipped   = "point.skipped"
	EventPointStopped   = "point.stopped"
	EventPointRetry     = "point.retry"
	EventPointError     = "point.error"
)

// CalibrationData 标定数据事件
const (
	EventDataCollected    = "data.collected"
	EventFittingCompleted = "fitting.completed"
	EventPressureApplied  = "pressure.applied"
)

// Hardware 硬件通讯日志事件
const (
	EventHardwareCommand  = "hardware.command"  // 发送命令到设备
	EventHardwareResponse = "hardware.response" // 收到设备响应
)

// System 系统级事件
const (
	EventSystemError = "system.error" // 系统错误日志
)
