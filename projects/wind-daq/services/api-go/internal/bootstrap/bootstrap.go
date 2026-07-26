package bootstrap

import (
	"encoding/json"
	"shared.local/device-sdk/go/pkg/slog"
	"net/http"
	"path/filepath"
	"time"

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
	windaqports "wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
	"wind-daq/services/api-go/pkg/appcontext"
	"wind-daq/services/api-go/pkg/logging"
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
	// LogRing 可选：若传入，则注册 /api/log/stream 和 /api/log/recent 端点。
	// 独立服务器模式由 main.go 初始化日志系统后传入；Wails 桌面模式由 app.go 传入。
	LogRing *logging.RingBuffer
	// LogManager 可选：若传入，则注册 /api/log/categories 端点用于日志分类开关。
	LogManager *logging.Manager
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
	checkpointStore := storageadapter.NewFileCheckpointStore()
	travMgr := usecase.NewTraversalManager(hub, motionMgr, travSink, calstore.NewTraversalResultStore(), checkpointStore, appConfigStore)
	// v2 可靠存储端口注入（Task 4-8）：三阶段提交与崩溃恢复
	// csvPort 复用 travSink（TraversalCsvWriter 同时实现旧 sink 与新 TraversalCSVPort）
	travMgr.SetCsvPort(travSink)
	travMgr.SetResultLogPort(storageadapter.NewTraversalResultLog())
	// checkpointPort 按 SavePath 动态创建（工厂模式），支持多任务隔离
	travMgr.SetCheckpointPortFactory(storageadapter.NewFileCheckpointPortFactory(checkpointStore))
	dataDir := filepath.Dir(cfg.ProfileStorePath)
	travMgr.SetActiveIndex(storageadapter.NewTraversalActiveIndex(
		filepath.Join(dataDir, "traversal-active-index.json"),
		dataDir,
	))
	// 注入插值器加载端口并异步恢复（通过 ports.InterpolatorLoader 解耦适配器依赖）
	travMgr.SetInterpolatorLoader(interpadapter.NewLoader())
	travMgr.RestoreInterpolatorFromPersistedConfig()

	// v2 校零组件 + dataSink 统一装配：
	// 之前在本文件内联实现，appcontext/apiserver 各自复制一份时遗漏了 calApplier/channels
	// 闭包，导致桌面生产路径与独立 API 服务器路径校零整段失效。
	// 现统一调用 usecase.AssembleDataSinkWithCalibration，任何装配根都无法绕过正确顺序。
	// 装配函数内部会调用 manager.SetCalibrationComponents 与 manager.UpdateDataSink。
	manager, err := usecase.NewDeviceManagerWithNormalizer(store, deviceFactory{}, nil, windaqconfig.NewProfileNormalizer())
	if err != nil {
		return APIServer{}, err
	}
	manager.SetScanner(scan.NewNetworkScanner())
	usecase.AssembleDataSinkWithCalibration(hub, recorder, manager, 5*time.Second)
	// 注入通道单位提供端口：BuildRawPressure 归一化时按 (deviceID, channelIndex)
	// 查询通道 Unit。manager 在此处已初始化完成，可安全注入。
	// 与 SetInterpolatorLoader 同模式：装配阶段一次性注入，运行期不切换。
	travMgr.SetUnitProvider(manager)
	// 注入设备采集控制端口：遍历启动前真实校验目标设备已连接/正在采集，
	// 并在 ParseAndStartTraversal 启动 loop 之前主动拉起采集，避免"假绿 → no data"。
	travMgr.SetAcquisitionController(manager)
	// 同源注入给校准管理器：校准采样过程中用户停采集后，算法在 waitForFreshData 超时后
	// 查询 IsAcquiring 区分"用户停采集"（可恢复，继续等待）与"设备在采集但帧不更新"（真异常），
	// 与 traversal 的"等待恢复"语义对齐，避免"误停一次采集，整个校准就报废"。
	calMgr.SetAcquisitionController(manager)

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
		LogRing:            cfg.LogRing,
		LogManager:         cfg.LogManager,
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
	case device.DeviceDAQP1603:
		// DAQ-P-1603：16 通道通用 AI 采集，走 shared SDK + DLL FFI 路径。
		// 与 DAQ-P-1604（裸 TCP）不同，1603 通过 WTNDAQ16H_64.dll 封装通信。
		return windaqhardware.NewDAQP1603Adapter(profile), nil
	case device.DeviceDAQP1604Pre:
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
