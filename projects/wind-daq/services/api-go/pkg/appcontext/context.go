// Package appcontext provides public access to core application components
package appcontext

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	hardware "shared.local/device-sdk/go/motion/adapters/hardware"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
	motionmanager "shared.local/motion-control/go/manager"
	motionprofile "shared.local/motion-control/go/profile"

	"wind-daq/services/api-go/internal/adapters/calstore"
	windaqconfig "wind-daq/services/api-go/internal/adapters/config"
	windaqhardware "wind-daq/services/api-go/internal/adapters/hardware"
	interpadapter "wind-daq/services/api-go/internal/adapters/interpolation"
	"wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	windaqports "wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
	"wind-daq/services/api-go/pkg/wiring"
)

// AppContext holds all core application services
type AppContext struct {
	DeviceManager    *usecase.DeviceManager
	AcquisitionHub   *usecase.AcquisitionHub
	ReportManager    *usecase.ReportManager
	MotionManager    windaqports.MotionManager
	CalibrationMgr   *usecase.CalibrationManager
	TraversalMgr     *usecase.TraversalManager
	StorageRecorder  *usecase.StorageRecorder
	ConfigManager    *usecase.ConfigManager
	MotionManagerRaw *motionmanager.MotionManager
	DataStreamRelay  *usecase.DataStreamRelay
	// TraversalRegistry 双探针 registry（Task 14）；与 legacy TraversalMgr 并存，
	// legacy single 路径行为不变。Shutdown 必须先于共享服务 Close（spec FR9）。
	TraversalRegistry *usecase.ManagerRegistry
	configDir         string
}

// NewAppContext creates and initializes all core services
func NewAppContext(configDir string) (*AppContext, error) {
	if configDir == "" {
		var err error
		configDir, err = os.UserConfigDir()
		if err != nil {
			configDir = "config"
		} else {
			configDir = filepath.Join(configDir, "wind-daq")
		}
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	deviceProfilePath := filepath.Join(configDir, "device-profiles.json")
	motionProfilePath := filepath.Join(configDir, "motion-profiles.json")

	if err := copyDefaultProfilesIfNeeded(deviceProfilePath, motionProfilePath); err != nil {
		return nil, err
	}

	profileStore := windaqconfig.NewFileProfileStore(deviceProfilePath)
	motionProfileStore := motionprofile.NewFileMotionProfileStore(motionProfilePath)
	appConfigStore := windaqconfig.NewFileAppConfigStore(filepath.Join(configDir, "app"))
	configMgr := usecase.NewConfigManager(appConfigStore)

	hub := usecase.NewAcquisitionHub(noopPublisher{}, 20)
	recorder, err := NewStorageRecorderFromConfigStore(appConfigStore)
	if err != nil {
		return nil, err
	}
	reportMgr := usecase.NewReportManager(report.NewCSVReportWriter())
	rawMotionMgr := motionmanager.NewMotionManager(motionProfileStore, func(profile core.MotionControllerProfile) (ports.MotionController, error) {
		factory := hardware.NewDefaultMotionControllerFactory()
		return factory.Create(profile)
	})
	motionMgr := wiring.WrapMotionManager(rawMotionMgr)
	calStore := calstore.NewMemoryResultStore()
	calibrationMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, calStore)
	// csvWriter 实例同时实现 CalibrationCsvWriter（单 writer 场景）与
	// CalibrationWriterFactory（七孔多 writer 场景）接口，故复用同一实例注入两端口。
	csvWriterInstance := storage.NewCalibrationCsvWriter(calibration.Config{})
	calibrationMgr.SetCsvWriter(csvWriterInstance)
	calibrationMgr.SetCsvWriterFactory(func(config calibration.Config) windaqports.CalibrationCsvWriter {
		return storage.NewCalibrationCsvWriterOverwrite(config)
	})
	calibrationMgr.SetSevenHoleWriterFactory(csvWriterInstance)
	travStore := calstore.NewTraversalResultStore()
	// §40 对齐：桌面生产路径必须与 bootstrap/apiserver 保持一致的 sink 注入，
	// 避免旧 sink 路径在桌面端完全跳过（InitializeTraversal/FinalizeTraversal 副作用丢失）。
	// 此前此处传 nil 导致桌面生产路径下遍历测试静默不输出 CSV（与 bootstrap.go 路径行为分裂），
	// 属于与校零 NewDataSink(nil,nil) 同类的 Critical BUG，现与 bootstrap.go:93 对齐统一注入。
	travSinkV2 := storage.NewTraversalCsvWriter()
	checkpointStore := storage.NewFileCheckpointStore()
	traversalMgr := usecase.NewTraversalManager(hub, motionMgr, travSinkV2, travStore, checkpointStore, appConfigStore)
	// v2 可靠存储端口注入（Task 4-8）：三阶段提交与崩溃恢复
	traversalMgr.SetCsvPort(travSinkV2)
	traversalMgr.SetResultLogPort(storage.NewTraversalResultLog())
	traversalMgr.SetCheckpointPortFactory(storage.NewFileCheckpointPortFactory(checkpointStore))
	traversalMgr.SetActiveIndex(storage.NewTraversalActiveIndex(
		filepath.Join(configDir, "traversal-active-index.json"),
		configDir,
	))
	// 注入插值器加载端口并异步恢复（通过 ports.InterpolatorLoader 解耦适配器依赖）
	traversalMgr.SetInterpolatorLoader(interpadapter.NewLoader())
	traversalMgr.RestoreInterpolatorFromPersistedConfig()

	// 先创建 manager（dataSink 暂传 nil），再统一装配 v2 校零组件 + dataSink。
	// 之前此处 NewDataSink(hub, recorder, nil, nil) 把 calApplier/channels 都置 nil，
	// 导致桌面生产路径校零热路径整段跳过——用户点校零按钮也不生效（Critical BUG #1）。
	// 现统一调用 AssembleDataSinkWithCalibration，与 bootstrap/apiserver 走同一条装配路径。
	deviceMgr, err := usecase.NewDeviceManagerWithNormalizer(profileStore, deviceFactory{}, nil, windaqconfig.NewProfileNormalizer())
	if err != nil {
		return nil, err
	}
	deviceMgr.SetScanner(scan.NewNetworkScanner())
	usecase.AssembleDataSinkWithCalibration(hub, recorder, deviceMgr, 5*time.Second)
	// 注入通道单位提供端口：BuildRawPressure 归一化时按 (deviceID, channelIndex)
	// 查询通道 Unit。deviceMgr 在此处已初始化完成，可安全注入。
	// 与 SetInterpolatorLoader 同模式：装配阶段一次性注入，运行期不切换。
	traversalMgr.SetUnitProvider(deviceMgr)
	// 注入设备采集控制端口：遍历启动前真实校验目标设备已连接/正在采集，
	// 并在 ParseAndStartTraversal 启动 loop 之前主动拉起采集，避免"假绿 → no data"。
	traversalMgr.SetAcquisitionController(deviceMgr)
	calibrationMgr.SetAcquisitionController(deviceMgr)

	// 双探针 registry（Task 14）：与 legacy TraversalMgr 并存，共享
	// AcquisitionHub/MotionAccess/DeviceManager 查询端口/appConfigStore/checkpointStore。
	registryBundle, err := NewTraversalRegistry(TraversalRegistryDeps{
		Hub:             hub,
		Motion:          motionMgr,
		DeviceManager:   deviceMgr,
		ConfigStore:     appConfigStore,
		CheckpointStore: checkpointStore,
		DataDir:         configDir,
		InterpLoader:    interpadapter.NewLoader(),
	})
	if err != nil {
		return nil, err
	}

	return &AppContext{
		DeviceManager:     deviceMgr,
		AcquisitionHub:    hub,
		ReportManager:     reportMgr,
		MotionManager:     motionMgr,
		CalibrationMgr:    calibrationMgr,
		TraversalMgr:      traversalMgr,
		StorageRecorder:   recorder,
		ConfigManager:     configMgr,
		MotionManagerRaw:  rawMotionMgr,
		DataStreamRelay:   usecase.NewDataStreamRelay(hub),
		TraversalRegistry: registryBundle.Registry,
		configDir:         configDir,
	}, nil
}

type noopPublisher struct{}

func (noopPublisher) Publish(string, any) {}

type deviceFactory struct{}

func (deviceFactory) Create(profile device.Profile) (windaqports.Device, error) {
	switch profile.Type {
	case device.DeviceDAQP1604:
		return windaqhardware.NewDAQP1604(profile), nil
	case device.DeviceDAQP1603:
		// DAQ-P-1603：16 通道通用 AI 采集，走 shared SDK + DLL FFI 路径。
		// 必须显式匹配此 case，否则会落入 default 创建 SimulatedDevice，
		// 导致后续 ApplyDAQP1603Config 类型断言失败（SimulatedDevice 未实现
		// ports.DAQP1603Configurable），保存配置时报 "device does not support
		// DAQ-P-1603 configuration"。
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

func copyDefaultProfilesIfNeeded(devicePath, motionPath string) error {
	if err := ensureJSONFile(devicePath, defaultDeviceProfiles()); err != nil {
		return err
	}
	return ensureJSONFile(motionPath, defaultMotionProfiles())
}

func ensureJSONFile(path string, fallback any) error {
	data, err := os.ReadFile(path)
	if err == nil {
		if json.Valid(bytes.TrimSpace(data)) {
			return nil
		}
		backupPath, backupErr := moveInvalidConfigAside(path)
		if backupErr != nil {
			return backupErr
		}
		if err := writeDefaultJSON(path, fallback); err != nil {
			return err
		}
		slog.Warn("replaced invalid config, backup saved",
			"component", "AppContext",
			"path", path,
			"backupPath", backupPath,
		)
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return writeDefaultJSON(path, fallback)
}

func moveInvalidConfigAside(path string) (string, error) {
	backupPath := path + ".invalid"
	if _, err := os.Stat(backupPath); err == nil {
		backupPath = fmt.Sprintf("%s.invalid.%d", path, time.Now().Unix())
	}
	if err := os.Rename(path, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func writeDefaultJSON(path string, value any) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func defaultDeviceProfiles() []device.Profile {
	if productionBuild {
		return []device.Profile{}
	}
	return []device.Profile{
		windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated),
	}
}

// defaultMotionProfiles 返回首次安装时写入的默认运动控制器 profile。
// 默认配置为 B140 控制器（IP 192.168.3.121 / 端口 23 / 细分数 40 / MaxSpeed 4），
// 旋转轴 U 默认传动比 180（产品出厂常用减速比），与产品出厂硬件默认对齐，安装后立即可用。
// 注意：仅在 motion-profiles.json 不存在时写入一次，已存在配置不会被覆盖。
func defaultMotionProfiles() []core.MotionControllerProfile {
	return []core.MotionControllerProfile{
		{
			ID:          "b140-motion-1",
			Name:        "B140 Motion Controller",
			Type:        core.ControllerTypeB140,
			Address:     "192.168.3.121",
			Port:        23,
			AutoConnect: false,
			Axes: []core.AxisConfig{
				{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(4), MicroSteps: core.PtrInt(40)},
				{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(4), MicroSteps: core.PtrInt(40)},
				{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(4), MicroSteps: core.PtrInt(40)},
				// U 轴为旋转轴，传动比默认 180（产品出厂常用减速比）
				{Name: core.AxisU, Enabled: true, Kind: core.AxisKindRotary, MaxSpeed: core.PtrFloat64(4), MicroSteps: core.PtrInt(40), GearRatio: core.PtrFloat64(180)},
			},
		},
	}
}
