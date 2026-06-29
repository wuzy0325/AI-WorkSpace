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
	DeviceTypeDAQP1064Pre = device.DeviceDAQP1064Pre
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
