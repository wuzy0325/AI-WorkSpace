// Package types exposes the core types used by Wind-DAQ applications
package types

import (
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/report"
	"wind-daq/services/api-go/internal/core/storage"
)

// ==================== Calibration Types ====================
type (
	CalibrationState          = calibration.State
	CalibrationType           = calibration.CalibrationType
	CalibrationConfig         = calibration.Config
	CalibrationStatus         = calibration.Status
	CalPoint                  = calibration.CalPoint
	ProbeChannel              = calibration.ProbeChannel
	FiveHoleRawData           = calibration.FiveHoleRawData
	FiveHoleCoefficients      = calibration.FiveHoleCoefficients
	FiveHoleDataPoint         = calibration.FiveHoleDataPoint
	ThreeHoleRawData          = calibration.ThreeHoleRawData
	ThreeHoleCoefficients     = calibration.ThreeHoleCoefficients
	ThreeHoleDataPoint        = calibration.ThreeHoleDataPoint
	TotalPressureRawData      = calibration.TotalPressureRawData
	TotalPressureCoefficients = calibration.TotalPressureCoefficients
	TotalPressureDataPoint    = calibration.TotalPressureDataPoint
	TotalTemperatureDataPoint = calibration.TotalTemperatureDataPoint

	// 七孔探针校准相关类型（spec Task 13）
	// SevenHoleConfigDTO：前端"配置向导"提交的七孔点位生成参数（α/β/θ/φ 范围与步长），
	//   经 backend binding 透传到 usecase.PreviewSevenHolePoints，纯计算不涉及 I/O。
	// SevenHolePreviewResult：点位预览结果，含完整点位列表 + 内/外区聚合统计，
	//   供前端在启动校准前确认点位规模（如 673 点 = 169 内区 + 504 外区）。
	// SevenHoleRawData / SevenHoleCoefficients / SevenHoleDataPoint：实时数据与结果数据点类型，
	//   Wails binding 通过 GenericResponse.Data 暴露给前端。
	SevenHoleConfigDTO     = calibration.SevenHoleConfig
	SevenHolePreviewResult = calibration.SevenHolePreviewResult
	SevenHoleRawData       = calibration.SevenHoleRawData
	SevenHoleCoefficients  = calibration.SevenHoleCoefficients
	SevenHoleDataPoint     = calibration.SevenHoleDataPoint

	// 五孔探针校准相关类型（spec Task 11）
	// FiveHolePointLayoutDTO：前端"配置向导"提交的五孔点位生成参数（α/β 范围与步长 + serpentine 开关），
	//   经 backend binding 透传到 usecase.PreviewFiveHolePoints，纯计算不涉及 I/O。
	//   与 SevenHoleConfigDTO 对称——直接是 calibration.FiveHolePointLayout 别名，
	//   让 Wails binding 入参类型有显式名字，便于生成 TS 类型与文档。
	FiveHolePointLayoutDTO = calibration.FiveHolePointLayout
	FiveHoleSnakePoint     = calibration.FiveHoleSnakePoint

	// CalibrationConfigDTO / ProbeChannelDTO 在本包 calibration.go 中以一等类型定义
	// （Task 05 从 internal/adapters/config 迁移），不再使用 alias。
)

// Calibration type constants
const (
	CalibrationTypeFiveHole         = calibration.TypeFiveHole
	CalibrationTypeThreeHole        = calibration.TypeThreeHole
	CalibrationTypeTotalPressure    = calibration.TypeTotalPressure
	CalibrationTypeTotalTemperature = calibration.TypeTotalTemperature
	CalibrationTypeSevenHole        = calibration.TypeSevenHole
)

// ==================== Device Types ====================
type (
	DeviceType             = device.Type
	DeviceConnection       = device.Connection
	DeviceProfile          = device.Profile
	DeviceStatus           = device.Status
	DeviceDataPayload      = device.DataPayload
	DeviceScanResult       = device.ScanResult
	DeviceChannelConfig    = device.ChannelConfig
	DaqT1603HardwareConfig = device.DaqT1603HardwareConfig
	DaqT1602HardwareConfig = device.DaqT1602HardwareConfig
	DSA3217ScanConfig      = device.DSA3217ScanConfig
)

// Device type constants
const (
	DeviceTypeSimulated   = device.DeviceSimulated
	DeviceTypeDAQP1604    = device.DeviceDAQP1604
	DeviceTypeDAQP1604Pre = device.DeviceDAQP1604Pre
	DeviceTypeDaqT1603    = device.DeviceDaqT1603
	DeviceTypeDaqT1602    = device.DeviceDaqT1602
	DeviceTypeWTNPXI      = device.DeviceWTNPXI
	DeviceTypeDSA3217     = device.DeviceDSA3217
)

// ==================== Motion Types ====================
type (
	MotionControllerProfile = motion.MotionControllerProfile
	MotionControllerStatus  = motion.ControllerStatus
	MotionAxisName          = motion.AxisName
)

// ==================== Storage Types ====================
type (
	StorageRecordingConfig = storage.RecordingConfig
	StorageRecordingStatus = storage.RecordingStatus
)

// ==================== Report Types ====================
type (
	ReportConfig = report.ReportConfig
	ReportResult = report.ReportResult
	ReportStatus = report.ReportStatus
)
