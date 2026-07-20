// Package types exposes the core types used by Wind-DAQ applications
package types

import (
	configadapter "wind-daq/services/api-go/internal/adapters/config"
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
	SevenHoleConfigDTO      = calibration.SevenHoleConfig
	SevenHolePreviewResult  = calibration.SevenHolePreviewResult
	SevenHoleRawData        = calibration.SevenHoleRawData
	SevenHoleCoefficients   = calibration.SevenHoleCoefficients
	SevenHoleDataPoint      = calibration.SevenHoleDataPoint

	// 以下 DTO 别名把 adapters/config 层的传输对象对外暴露，供 desktop-wails backend
	// 等独立模块通过公共包访问。backend 是独立 Go module，无法直接 import
	// internal/adapters/config（Go internal 规则），因此沿用 pkg/types 的 facade 模式。
	CalibrationConfigDTO = configadapter.CalibrationConfigDTO
	ProbeChannelDTO      = configadapter.ProbeChannelDTO
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
	DSA3217ScanConfig      = device.DSA3217ScanConfig
)

// Device type constants
const (
	DeviceTypeSimulated   = device.DeviceSimulated
	DeviceTypeDAQP1604    = device.DeviceDAQP1604
	DeviceTypeDAQP1604Pre = device.DeviceDAQP1604Pre
	DeviceTypeDaqT1603    = device.DeviceDaqT1603
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
