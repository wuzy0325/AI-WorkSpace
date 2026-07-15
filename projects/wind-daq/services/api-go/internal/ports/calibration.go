package ports

import (
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/traversal"
)

type LatestDataReader interface {
	GetLatestData(deviceID string) (device.DataPayload, bool)
	GetLatestTimestamp(deviceID string) (int64, bool)
}

type CalibrationPointSink interface {
	WriteCalibrationPoint(point calibration.PointResult) error
}

type CalibrationResultStore interface {
	Save(taskID string, status calibration.Status) error
	Get(taskID string) (calibration.Status, bool)
}

// CalibrationEventPublisher 校准事件发布端口
type CalibrationEventPublisher interface {
	PublishProgress(event calibration.ProgressEvent)
	PublishComplete(event calibration.CompleteEvent)
	PublishRealtime(event calibration.RealtimeEvent)
}

// CalibrationRuntime 校准运行时端口，提供通道读取和运动控制能力
type CalibrationRuntime interface {
	GetChannelValue(deviceID string, channelIndex int) (float64, bool)
	GetLatestTimestamp(deviceID string) (int64, bool)
	MoveToPosition(axis calibration.MotionAxisConfig, position float64) error
	WaitForMotionComplete() error
	StopMotion() error
}

// EmergencyStopProvider 急停能力提供者（可选扩展接口）。
// 实现 CalibrationRuntime 的类型可同时实现此接口，表示具备控制器级急停能力。
// 调用方通过类型断言判断能力是否存在：
//
//	if es, ok := rt.(ports.EmergencyStopProvider); ok {
//	    es.EmergencyStopMotion()
//	}
//
// 不存在时 fallback 到 StopMotion（普通停止），不阻断故障处理流程。
//
// 与 StopMotion 的差异：
//
//	| 维度     | StopMotion                              | EmergencyStopMotion              |
//	|----------|-----------------------------------------|----------------------------------|
//	| 作用域   | 单轴逐台（对 Moving=true 的轴调 Stop）  | 控制器级（所有轴瞬时停止）        |
//	| 停机方式 | 减速停止                                | 瞬时停止 + 锁存 EmergencyStopped |
//	| 恢复方式 | 自动                                    | 需人工复位                        |
//	| 触发条件 | Deviation / NoProgress / Overshoot      | CriticalDeviation / LimitTriggered / StatusUnavailable |
type EmergencyStopProvider interface {
	// EmergencyStopMotion 急停所有参与校准的运动控制器。
	EmergencyStopMotion() error
}

// MotionSafetyAwareRuntime 运动安全感知能力提供者（可选扩展接口）。
// 实现 CalibrationRuntime 的类型可同时实现此接口，表示能返回运动安全故障的三元组语义。
// runtimeAdapter 通过类型断言判断能力是否存在：
//   - 支持时透传三元组 (completed, reason, failure)
//   - 不支持时 fallback 到旧 WaitForMotionComplete() error，映射为 (false, MotionInterruptTimeout, nil)
//
// 设计动机：ports.CalibrationRuntime 既有 WaitForMotionComplete() error 签名不能破坏
// （外部实现如 Wails binding 适配器依赖）；运动安全感知能力通过可选扩展接口按需获取。
// fallbackRuntime 始终实现此接口（内部完整运动安全循环）；外部 runtime 可选实现。
type MotionSafetyAwareRuntime interface {
	// WaitForMotionCompleteWithSafety 返回运动等待结果的三元组：
	//   - completed=true, reason=none, failure=nil：所有参与运动的轴已到位
	//   - completed=false, failure!=nil：检测到运动安全故障（调用方应调用 handleCalibrationMotionSafetyFailure）
	//   - completed=false, failure=nil, reason≠none：因暂停/停止/取消/超时中断
	WaitForMotionCompleteWithSafety() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure)
}

// DeviceStatusProvider 设备状态查询端口
type DeviceStatusProvider interface {
	GetDeviceStatus(deviceID string) (connected bool, acquiring bool)
}

// CalibrationCsvWriter 校准 CSV 写入端口
// 抽象 CSV 字节 I/O，使 usecase 不依赖 adapters/storage。
// 实现见 adapters/storage.CalibrationCsvWriter。
type CalibrationCsvWriter interface {
	Initialize(config calibration.Config) error
	AppendPoint(dataPoint calibration.DataPoint) error
	Flush() error
	Path() string
}
