package device

import (
	"context"

	"cal1604/internal/domain"
)

// PressureDriver 抽象打压设备能力。
type PressureDriver interface {
	ConnectionDriver
	SetTargetPressure(ctx context.Context, target float64) error
	Stop(ctx context.Context) error
	Exhaust(ctx context.Context) error
	ReadCurrentPressure(ctx context.Context) (float64, error)
	ReadUnit(ctx context.Context) (string, error)
	SetUnit(ctx context.Context, unit string) error
	ReadStability(ctx context.Context) (bool, error)
}

// PressureControlCapable 可选能力：打压设备支持自动压力控制。
type PressureControlCapable interface {
	StartControl(ctx context.Context) error
}

// MeasureDriver 抽象计量设备能力。
type MeasureDriver interface {
	ConnectionDriver
	ReadValveStatus(ctx context.Context) (string, error)
	SetValveStatus(ctx context.Context, status string) error
	ReadUnit(ctx context.Context) (string, error)
	SetUnit(ctx context.Context, unit string) error
	CollectData(ctx context.Context, channels []int) ([]float64, error)
	// CalibrateZero 执行调零校准，返回各通道调零结果。
	CalibrateZero(ctx context.Context, channels []int) ([]float64, error)
	// CalibrateFullScale 执行满量程校准，返回各通道校准结果。
	CalibrateFullScale(ctx context.Context, channels []int, fullScaleValue float64) ([]float64, error)
	ReadDeviceInfo(ctx context.Context) (map[string]string, error)
	Reset(ctx context.Context) error
}

// CalibrationCapable 可选能力：计量设备支持多点校准流程。
// WTN1604 等设备在校准模式下使用专用的按点采集和拟合命令。
type CalibrationCapable interface {
	StartCalibration(ctx context.Context, channels []int, pressurePoints int, avgPoints int) error
	CollectCalibrationPoint(ctx context.Context, pointIndex int, targetPressure float64) ([]float64, error)
	PerformFitting(ctx context.Context) error
	SaveCoefficients(ctx context.Context) error
	EndCalibration(ctx context.Context) error
}

// DeviceStore 抽象设备配置存储能力。
type DeviceStore interface {
	Upsert(dev domain.Device)
	UpdateStatus(id string, status domain.DeviceStatus) bool
	UpdateUnit(id string, unit string) bool
	Delete(id string)
	Get(id string) (domain.Device, bool)
	List() []domain.Device
	CheckUnitConsistency() (bool, []string)
}

// ConnectionDriver 抽象设备连接链路能力。
// 当前阶段只要求覆盖连接与断开，协议命令能力后续分阶段补齐。
type ConnectionDriver interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
}

// ConnectionDriverFactory 按设备模型创建连接驱动。
type ConnectionDriverFactory interface {
	Create(dev domain.Device) (ConnectionDriver, error)
}

// ActiveDriverProvider 返回已连接的设备驱动实例，供校准/测量等服务复用。
// 避免各服务独立创建驱动导致连接丢失。
type ActiveDriverProvider interface {
	GetActiveDriver(id string) ConnectionDriver
}

// StabilityStatusProvider 可选能力：打压设备支持硬件级判稳状态查询。
// SCPI 设备（ConST 系列）通过专用命令直接返回稳定标志，比软件判稳更准确。
type StabilityStatusProvider interface {
	IsStable(ctx context.Context) (bool, error)
}

// DriverFactory 按设备型号创建对应驱动实例（ports 层接口）。
// application 层通过此接口创建驱动，不直接依赖 infrastructure/driver 包，
// 维持六边形架构"usecase 不得导入 adapters"的依赖方向约束。
// 具体实现由 composition root（cmd/server 或 api/http/assembly）注入。
type DriverFactory interface {
	// Create 返回通用连接驱动（仅含 Connect/Disconnect）。
	Create(dev domain.Device) (ConnectionDriver, error)
	// CreateMeasureDriver 返回计量设备驱动（含阀门控制、数据采集、校准等）。
	CreateMeasureDriver(dev domain.Device) (MeasureDriver, error)
	// CreatePressureDriver 返回打压设备驱动（含压力控制、稳定检测等）。
	CreatePressureDriver(dev domain.Device) (PressureDriver, error)
}

