package appcontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"wind-daq/services/api-go/internal/adapters/calstore"
	runtimeadapter "wind-daq/services/api-go/internal/adapters/runtime"
	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/resourcelock"
	windaqports "wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

// 双探针 traversal registry 的统一装配（Task 14）。
//
// 三个生产装配流（NewAppContext / bootstrap.BuildAPIServer / apiserver.Start）
// 都经 NewTraversalRegistry 构造同一语义的 factory/registry（spec FR3）：
//   - factory 为每 probe 新建 TraversalCsvWriter / TraversalResultLog /
//     dual checkpoint port factory（v3 codec）与结果 store；
//   - 共享依赖（AcquisitionHub / MotionAccess / DeviceManager 查询端口 /
//     appConfigStore / checkpointStore / dual recovery index / TaskIDGenerator）
//     经 TraversalRegistryDeps 闭包传入；
//   - dual manager 不注入 legacy traversal-active-index.json。

// TraversalRegistryDeps 装配 registry 所需的共享依赖。
//
// Hub/Motion/DeviceManager/InterpLoader 可为 nil（降级路径与 legacy 一致）；
// ConfigStore / CheckpointStore / DataDir 必填。
type TraversalRegistryDeps struct {
	Hub             *usecase.AcquisitionHub // LatestDataReader（共享只读采集总线）
	Motion          windaqports.MotionAccess
	DeviceManager   *usecase.DeviceManager // ChannelUnitProvider + AcquisitionController 查询端口
	ConfigStore     windaqports.AppConfigStore
	CheckpointStore windaqports.CheckpointStore
	// DataDir dual recovery index 与恢复状态文件所在目录。
	DataDir string
	// InterpLoader 插值器加载端口（nil 时跳过启动恢复）。
	InterpLoader windaqports.InterpolatorLoader
}

// TraversalRegistryBundle registry 及其恢复索引（装配产物，供诊断/测试）。
type TraversalRegistryBundle struct {
	Registry      *usecase.ManagerRegistry
	RecoveryIndex windaqports.DualTraversalRecoveryIndex
}

// traversalManagerFactory 实现 usecase.TraversalManagerFactory：
// 每 probe 新建有状态端口，共享依赖闭包传入。
type traversalManagerFactory struct {
	deps TraversalRegistryDeps
}

// NewManager 创建指定 probe 的完整装配 managed manager。
func (f *traversalManagerFactory) NewManager(probeID usecase.ProbeID) (usecase.ManagedTraversalManager, error) {
	if !probeID.Valid() {
		return nil, fmt.Errorf("%w: %q", usecase.ErrInvalidProbeID, probeID)
	}
	// 每 manager 新建有状态端口（spec FR3：禁止跨 probe 共享实例）。
	csvWriter := storage.NewTraversalCsvWriter()
	resultLog := storage.NewTraversalResultLog()
	resultStore := calstore.NewTraversalResultStore()
	// dual checkpoint port factory：v3 codec（managed 写 v3，legacy factory 不注入）。
	checkpointPortFactory := storage.NewDualCheckpointPortFactory(f.deps.CheckpointStore)

	mgr := usecase.NewTraversalManager(f.deps.Hub, f.deps.Motion, csvWriter, resultStore, f.deps.CheckpointStore, f.deps.ConfigStore)
	mgr.SetCsvPort(csvWriter)
	mgr.SetResultLogPort(resultLog)
	mgr.SetCheckpointPortFactory(checkpointPortFactory)
	// probe-scoped 配置键（traversal.probe1/probe2）；dual 不注入 legacy activeIndex。
	mgr.SetConfigKey("traversal." + string(probeID))
	if f.deps.InterpLoader != nil {
		mgr.SetInterpolatorLoader(f.deps.InterpLoader)
		mgr.RestoreInterpolatorFromPersistedConfigSync()
	}
	if f.deps.DeviceManager != nil {
		mgr.SetUnitProvider(f.deps.DeviceManager)
		mgr.SetAcquisitionController(f.deps.DeviceManager)
	}
	return mgr, nil
}

// NewTraversalRegistry 统一装配双探针 ManagerRegistry（Task 14 单一入口）。
func NewTraversalRegistry(deps TraversalRegistryDeps) (*TraversalRegistryBundle, error) {
	if deps.ConfigStore == nil || deps.CheckpointStore == nil || deps.DataDir == "" {
		return nil, errors.New("traversal registry deps 不完整（ConfigStore/CheckpointStore/DataDir 必填）")
	}
	// 与 legacy single 共享同一进程级锁服务：workflow:traversal 资源互斥语义不变。
	lockSvc := resourcelock.Default()
	recoveryIndex := storage.NewDualTraversalRecoveryIndex(
		filepath.Join(deps.DataDir, storage.DualTraversalRecoveryIndexFileName))
	registry, err := usecase.NewManagerRegistry(
		usecase.ManagerRegistryDeps{
			Factory:         &traversalManagerFactory{deps: deps},
			TaskIDGenerator: runtimeadapter.NewTaskIDGenerator(),
			WorkflowLease:   runtimeadapter.NewWorkflowLease(lockSvc),
			ControllerLease: runtimeadapter.NewControllerLease(lockSvc),
			RecoveryIndex:   recoveryIndex,
			ConfigStore:     deps.ConfigStore,
			MotionAccess:    deps.Motion,
			CheckpointStore: deps.CheckpointStore,
		},
		shutdownTimeoutOptions(deps.ConfigStore)...,
	)
	if err != nil {
		return nil, fmt.Errorf("装配 traversal registry: %w", err)
	}
	return &TraversalRegistryBundle{Registry: registry, RecoveryIndex: recoveryIndex}, nil
}

// shutdownTimeoutsConfig shutdown 双 deadline 的配置文件结构（app_config_store
// 键 "traversalShutdown"；缺省/非法时回退默认 5s/10s）。
type shutdownTimeoutsConfig struct {
	GracefulSeconds float64 `json:"gracefulSeconds"`
	HardSeconds     float64 `json:"hardSeconds"`
}

// shutdownTimeoutOptions 从配置存储读取 shutdown 双 deadline 覆盖项。
// 仅当两个值均为有限正值且 hard > graceful 时生效（WithShutdownTimeouts 校验兜底）。
func shutdownTimeoutOptions(store windaqports.AppConfigStore) []usecase.ManagerRegistryOption {
	data, err := store.LoadConfig("traversalShutdown")
	if err != nil || len(data) == 0 {
		return nil
	}
	var cfg shutdownTimeoutsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("解析 traversalShutdown 配置失败，使用默认 deadline", "error", err)
		return nil
	}
	graceful := time.Duration(cfg.GracefulSeconds * float64(time.Second))
	hard := time.Duration(cfg.HardSeconds * float64(time.Second))
	if graceful <= 0 || hard <= 0 || hard <= graceful {
		slog.Warn("traversalShutdown 配置非法（须为有限正值且 hard > graceful），使用默认 deadline",
			"graceful", graceful, "hard", hard)
		return nil
	}
	return []usecase.ManagerRegistryOption{usecase.WithShutdownTimeouts(graceful, hard)}
}
