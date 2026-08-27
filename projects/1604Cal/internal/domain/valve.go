package domain

// ValveState 表示计量设备阀门当前状态。
// 阀门状态是「标定 + 计量」启动的必要条件之一（必须为 ValveStateCalibration）。
type ValveState string

const (
	// ValveStateCalibration 阀门处于校准模式，是启动标定/计量的必要条件。
	ValveStateCalibration ValveState = "calibration"
	// ValveStateMeasurement 阀门处于测量模式。
	ValveStateMeasurement ValveState = "measurement"
	// ValveStateUnknown 阀门状态未知（驱动无法解析硬件返回值时使用）。
	ValveStateUnknown ValveState = "unknown"
)

// String 实现 fmt.Stringer，便于日志/错误链直接拼接。
func (v ValveState) String() string {
	return string(v)
}

// IsCalibration 判定是否处于校准模式（门禁通过条件）。
func (v ValveState) IsCalibration() bool {
	return v == ValveStateCalibration
}
