package http

import (
	"io/fs"

	"cal1604/internal/application/batch"
	"cal1604/internal/application/calibration"
	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/application/measurement"
	"cal1604/internal/application/multipress"
	"cal1604/internal/application/session"
	"cal1604/internal/config"
	"cal1604/internal/device"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
	"cal1604/internal/events"
	"cal1604/internal/infrastructure/driver"
	"cal1604/internal/report"
	"cal1604/internal/workflow"
)

// Dependencies 包含 apiServer 所需的所有服务依赖。
type Dependencies struct {
	DeviceManager      deviceManager
	WorkflowCoordinator *workflow.WorkflowCoordinator
	DeviceConnector    deviceConnector
	ConnectConfig      deviceconnect.Config
	CalibrationService *calibration.Service
	MultipressService  *multipress.Service
	SessionService     *session.Service
	MeasurementService *measurement.Service
	ReportService      *report.Service
	BatchService       *batch.Service
	ConfigPath         string
	AppConfig          *config.AppConfig
}

// chainActiveDriverProvider 顺序查询多个驱动提供者，返回第一个命中的活跃驱动。
type chainActiveDriverProvider struct {
	providers []device.ActiveDriverProvider
}

func (p chainActiveDriverProvider) GetActiveDriver(id string) device.ConnectionDriver {
	for _, provider := range p.providers {
		if provider == nil {
			continue
		}
		if drv := provider.GetActiveDriver(id); drv != nil {
			return drv
		}
	}
	return nil
}

// newDependencies 创建并组装所有服务依赖。
func newDependencies(
	deviceManager deviceManager,
	connector deviceConnector,
	connectConfig deviceconnect.Config,
	calibrationConfig CalibrationRuntimeConfig,
	appCfg *config.AppConfig,
	configPath string,
	templateDir string,
	templateEmbedFS ...fs.FS,
) *Dependencies {
	if deviceManager == nil {
		deviceManager = manager.NewDeviceManager()
	}

	coordinator := workflow.NewWorkflowCoordinator()
	factory := driver.NewFactory()

	// 通用事件发布适配器，间接调用 package 级 publishEvent。
	publishAdapter := func(eventType string, data any) {
		publishEvent(eventType, data)
	}

	// 设备状态变更事件发布，用于 deviceconnect。
	deviceStatusPublisher := func(dev domain.Device) {
		payload := map[string]any{
			"id":     dev.ID,
			"type":   string(dev.Type),
			"status": string(dev.Status),
		}
		if dev.LastErrorReason != "" {
			payload["errorReason"] = dev.LastErrorReason
		}
		if dev.LastErrorAt != nil {
			payload["lastErrorAt"] = dev.LastErrorAt
		}
		publishEvent(events.EventDeviceStatusChanged, payload)
	}

	// 设备连接服务
	var deviceConnectorSvc deviceConnector
	if connector == nil {
		deviceConnectorSvc = deviceconnect.NewService(
			deviceManager,
			factory,
			connectConfig,
			deviceStatusPublisher,
		)
	} else {
		deviceConnectorSvc = connector
	}

	// 多设备打压控制服务
	multipressSvc := multipress.NewService(
		factory,
		deviceManager,
		publishAdapter,
	)
	multipressSvc.StartPolling()

	// 聚合驱动提供者：优先复用设备连接服务中的驱动，其次复用 multipress 已注册驱动。
	providers := make([]device.ActiveDriverProvider, 0, 2)
	if dp, ok := deviceConnectorSvc.(device.ActiveDriverProvider); ok {
		providers = append(providers, dp)
	}
	providers = append(providers, multipressSvc)

	var driverProvider device.ActiveDriverProvider
	if len(providers) > 0 {
		driverProvider = chainActiveDriverProvider{providers: providers}
	}

	// 共享设备会话服务
	sessionSvc := session.NewService(
		deviceManager,
		factory,
		publishAdapter,
		driverProvider,
	)

	// 计量服务
	measurementSvc := measurement.NewService(
		sessionSvc,
		publishAdapter,
		coordinator,
	)
	if appCfg != nil {
		measurementSvc.SetConfig(measurementConfigFromParams(appCfg.MeasurementParams))
		measurementSvc.SetAlarmConfig(appCfg.Alarm)
	} else {
		measurementSvc.SetConfig(measurementConfigFromParams(config.Default().MeasurementParams))
		measurementSvc.SetAlarmConfig(config.Default().Alarm)
	}
	// 计量启动门禁：阀门=校准模式是必要条件，与标定服务共用同一开关。
	measurementSvc.SetStartPrerequisiteConfig(measurement.StartPrerequisiteConfig{
		EnforceValveCalibration: calibrationConfig.EnforceValveCalibrationGate,
	})

	// 校准服务
	calibrationSvc := calibration.NewService(
		coordinator,
		factory,
		deviceManager,
		publishAdapter,
		driverProvider,
		sessionSvc,
	)
	calibrationSvc.SetStartPrerequisiteConfig(calibration.StartPrerequisiteConfig{
		EnforceValveCalibration: calibrationConfig.EnforceValveCalibrationGate,
	})

	// 报告服务（优先外部目录，其次 embed.FS，最后无模板模式）
	var embedFS fs.FS
	if len(templateEmbedFS) > 0 {
		embedFS = templateEmbedFS[0]
	}
	reportSvc := report.NewService(templateDir, embedFS)
	// 多设备导出：报告文件名后缀优先使用设备名而非内部设备 ID，
	// 设备不存在或名称为空时由报告服务自行回退到设备 ID。
	reportSvc.SetDeviceNameResolver(func(id string) string {
		if dev, ok := deviceManager.Get(id); ok {
			return dev.Name
		}
		return ""
	})

	// 分批计量服务（纯内存状态，不持久化）
	batchSvc := batch.NewService()

	return &Dependencies{
		DeviceManager:      deviceManager,
		WorkflowCoordinator: coordinator,
		DeviceConnector:    deviceConnectorSvc,
		ConnectConfig:      connectConfig,
		CalibrationService: calibrationSvc,
		MultipressService:  multipressSvc,
		SessionService:     sessionSvc,
		MeasurementService: measurementSvc,
		ReportService:      reportSvc,
		BatchService:       batchSvc,
		ConfigPath:         configPath,
		AppConfig:          appCfg,
	}
}
