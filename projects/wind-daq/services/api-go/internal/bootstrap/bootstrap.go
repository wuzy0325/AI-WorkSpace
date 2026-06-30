package bootstrap

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"

	"shared.local/device-sdk/go/motion/adapters/hardware"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
	motionmanager "shared.local/motion-control/go/manager"
	motionprofile "shared.local/motion-control/go/profile"

	"wind-daq/services/api-go/api"
	calstore "wind-daq/services/api-go/internal/adapters/calstore"
	windaqconfig "wind-daq/services/api-go/internal/adapters/config"
	windaqhardware "wind-daq/services/api-go/internal/adapters/hardware"
	interpadapter "wind-daq/services/api-go/internal/adapters/interpolation"
	reportadapter "wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	storageadapter "wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/device"
	windaqports 	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
	"wind-daq/services/api-go/pkg/appcontext"
	"wind-daq/services/api-go/pkg/wiring"
)

const (
	DefaultAddress          = ":8080"
	DefaultProfileStorePath = "config/device-profiles.json"
)

// 默认采集参数。在配置文件缺失或字段为零值时使用。
const (
	defaultPublishHz       = 20.0
	defaultHistoryCapacity = 256
)

// Config 装配根配置
type Config struct {
	Address          string
	ProfileStorePath string
}

type APIServer struct {
	Address string
	Handler http.Handler
}

func BuildAPIServer(cfg Config) (APIServer, error) {
	if cfg.Address == "" {
		cfg.Address = DefaultAddress
	}
	if cfg.ProfileStorePath == "" {
		cfg.ProfileStorePath = DefaultProfileStorePath
	}

	store := windaqconfig.NewFileProfileStore(cfg.ProfileStorePath)
	appConfigStore := windaqconfig.NewFileAppConfigStore(filepath.Join(filepath.Dir(cfg.ProfileStorePath), "app"))
	configMgr := usecase.NewConfigManager(appConfigStore)
	if err := ensureDefaultProfiles(store); err != nil {
		return APIServer{}, err
	}

	// 读取采集配置（文件缺失时使用默认值）
	acqCfg := loadAcquisitionConfig(appConfigStore)

	hub := usecase.NewAcquisitionHubWithHistoryCapacity(noopPublisher{}, acqCfg.PublishHz, acqCfg.HistoryCapacity)

	recorder, err := appcontext.NewStorageRecorderFromConfigStore(appConfigStore)
	if err != nil {
		return APIServer{}, err
	}

	reportMgr := usecase.NewReportManager(reportadapter.NewCSVReportWriter())
	profileStore := motionprofile.NewMemoryMotionProfileStore()
	factory := hardware.NewDefaultMotionControllerFactory()
	rawMotionMgr := motionmanager.NewMotionManager(profileStore, func(profile core.MotionControllerProfile) (ports.MotionController, error) {
		return factory.Create(profile)
	})
	motionMgr := wiring.WrapMotionManager(rawMotionMgr)
	calMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, calstore.NewMemoryResultStore())
	// 注入遍历 CSV 写入 sink，承担测试结果落盘
	travSink := storageadapter.NewTraversalCsvWriter()
	travMgr := usecase.NewTraversalManager(hub, motionMgr, travSink, calstore.NewTraversalResultStore(), storageadapter.NewFileCheckpointStore(), appConfigStore)
	// 注入插值器加载端口并异步恢复（通过 ports.InterpolatorLoader 解耦适配器依赖）
	travMgr.SetInterpolatorLoader(interpadapter.NewLoader())
	travMgr.RestoreInterpolatorFromPersistedConfig()

	dataSink := usecase.NewDataSink(hub, recorder)
	manager, err := usecase.NewDeviceManagerWithNormalizer(store, deviceFactory{}, dataSink, windaqconfig.NewProfileNormalizer())
	if err != nil {
		return APIServer{}, err
	}
	manager.SetScanner(scan.NewNetworkScanner())

	router := api.NewRouter(api.Deps{
		DeviceManager:      manager,
		AcquisitionHub:     hub,
		ReportManager:      reportMgr,
		MotionManager:      motionMgr,
		MotionService:      rawMotionMgr,
		CalibrationManager: calMgr,
		TraversalManager:   travMgr,
		StorageRecorder:    recorder,
		ConfigManager:      configMgr,
		LogRing:            nil,
	})
	return APIServer{Address: cfg.Address, Handler: router}, nil
}

// acquisitionAppConfig 采集 hub 的应用配置（JSON 文件 acquisition.json）。
// 文件缺失或字段为零值时使用默认值。
type acquisitionAppConfig struct {
	PublishHz       float64 `json:"publishHz"`
	HistoryCapacity int     `json:"historyCapacity"`
}

// loadAcquisitionConfig 从 app_config_store 读取 acquisition.json。
// 文件不存在、解析失败或字段为零值时使用默认值。
func loadAcquisitionConfig(store windaqports.AppConfigStore) acquisitionAppConfig {
	cfg := acquisitionAppConfig{
		PublishHz:       defaultPublishHz,
		HistoryCapacity: defaultHistoryCapacity,
	}
	data, err := store.LoadConfig("acquisition")
	if err != nil {
		slog.Warn("bootstrap: 读取采集配置失败，使用默认值",
			"component", "bootstrap",
			"error", err,
			"publishHz", cfg.PublishHz,
			"historyCapacity", cfg.HistoryCapacity,
		)
		return cfg
	}
	if len(data) == 0 {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("bootstrap: 解析采集配置失败，使用默认值",
			"component", "bootstrap",
			"error", err,
			"publishHz", defaultPublishHz,
			"historyCapacity", defaultHistoryCapacity,
		)
		return acquisitionAppConfig{
			PublishHz:       defaultPublishHz,
			HistoryCapacity: defaultHistoryCapacity,
		}
	}
	if cfg.PublishHz <= 0 {
		cfg.PublishHz = defaultPublishHz
	}
	if cfg.HistoryCapacity <= 0 {
		cfg.HistoryCapacity = defaultHistoryCapacity
	}
	return cfg
}

// 以下保留兼容旧引用的 noopPublisher 与 deviceFactory
type noopPublisher struct{}

func (noopPublisher) Publish(string, any) {}

type deviceFactory struct{}

func (deviceFactory) Create(profile device.Profile) (windaqports.Device, error) {
	switch profile.Type {
	case device.DeviceDAQP1604:
		return windaqhardware.NewDAQP1604(profile), nil
	case device.DeviceDAQP1064Pre:
		return windaqhardware.NewDAQP1064Pre(profile), nil
	case device.DeviceDaqT1603:
		return windaqhardware.NewT1603Adapter(profile), nil
	case device.DeviceWTNPXI:
		return windaqhardware.NewWTNPXI(profile), nil
	case device.DeviceDSA3217:
		return windaqhardware.NewDSA3217(profile), nil
	default:
		return windaqhardware.NewSimulatedDevice(profile), nil
	}
}

func ensureDefaultProfiles(store windaqports.ProfileStore) error {
	profiles, err := store.LoadProfiles()
	if err != nil {
		return err
	}
	if len(profiles) > 0 {
		return nil
	}
	return store.SaveProfiles([]device.Profile{
		windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated),
	})
}
