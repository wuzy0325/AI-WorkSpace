// SSE 事件类型常量，与 backend internal/events/event_types.go 保持同步。
// 按发布模块分组，格式为 <模块>.<动作>。

// Session 会话生命周期事件
export const EVENT_SESSION_DEVICE_BOUND = 'session.device_bound'
export const EVENT_SESSION_STATE_CHANGED = 'session.state.changed'

// Device 设备状态事件
export const EVENT_DEVICE_STATUS_CHANGED = 'device.status.changed'
export const EVENT_DEVICE_CONNECT_PROGRESS = 'device.connect_progress'

// Measurement 计量工作流事件
export const EVENT_MEASUREMENT_STATE_CHANGED = 'measurement.state_changed'
export const EVENT_MEASUREMENT_DATA_UPDATED = 'measurement.data_updated'
export const EVENT_MEASUREMENT_DATA_COLLECTED = 'measurement.data.collected'
export const EVENT_MEASUREMENT_POINT_STATUS = 'measurement.point.status'
export const EVENT_MEASUREMENT_ALARM_TRIGGERED = 'measurement.alarm.triggered'
export const EVENT_MEASUREMENT_ALARM_RESOLVED = 'measurement.alarm.resolved'
export const EVENT_MEASUREMENT_STABILITY_UPDATE = 'measurement.stability.update'
export const EVENT_MEASUREMENT_STABILITY_TIMEOUT = 'measurement.stability.timeout'

// Calibration 标定工作流事件
export const EVENT_CALIBRATION_POINT_STATUS = 'calibration.point_status'
export const EVENT_CALIBRATION_ALARM_RESOLVED = 'calibration.alarm.resolved'
export const EVENT_CALIBRATION_STABILITY_UPDATE = 'calibration.stability.update'
export const EVENT_CALIBRATION_STABILITY_CHANGED = 'calibration.stability.changed'
export const EVENT_CALIBRATION_STABILITY_LOST = 'calibration.stability.lost'
export const EVENT_CALIBRATION_STABILITY_PROGRESS = 'calibration.stability.progress'
export const EVENT_CALIBRATION_STABILITY_ACHIEVED = 'calibration.stability.achieved'
export const EVENT_CALIBRATION_STABILITY_PREFIX = 'calibration.stability.'

// AutoCollection 自动采集事件
export const EVENT_AUTO_COLLECTION_STARTED = 'autoCollection.started'
export const EVENT_AUTO_COLLECTION_STOPPED = 'autoCollection.stopped'
export const EVENT_AUTO_COLLECTION_COMPLETED = 'autoCollection.completed'
export const EVENT_AUTO_COLLECTION_ERROR = 'autoCollection.error'

// Point 标定点事件
export const EVENT_POINT_STARTED = 'point.started'
export const EVENT_POINT_COMPLETED = 'point.completed'
export const EVENT_POINT_RECOLLECT = 'point.recollect'
export const EVENT_POINT_SKIPPED = 'point.skipped'
export const EVENT_POINT_STOPPED = 'point.stopped'
export const EVENT_POINT_RETRY = 'point.retry'
export const EVENT_POINT_ERROR = 'point.error'

// CalibrationData 标定数据事件
export const EVENT_DATA_COLLECTED = 'data.collected'
export const EVENT_FITTING_COMPLETED = 'fitting.completed'
export const EVENT_PRESSURE_APPLIED = 'pressure.applied'

// Multi-press 多点打压事件
export const EVENT_MULTIPRESS_PRESSURE_UPDATE = 'multipress.pressure.update'

// Alarm 报警事件
export const EVENT_ALARM_TRIGGERED = 'alarm.triggered'

// Hardware 硬件通讯日志事件
export const EVENT_HARDWARE_COMMAND = 'hardware.command'
export const EVENT_HARDWARE_RESPONSE = 'hardware.response'

// System 系统级事件
export const EVENT_SYSTEM_ERROR = 'system.error'
