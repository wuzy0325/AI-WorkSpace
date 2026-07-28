package appcontext

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	seveninterp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

// Task 14：统一 factory/registry 装配语义。
//
// 验证：每 probe 新建独立 manager 与有状态端口、probe-scoped 配置键、
// dual recovery index 装配、registry 满足 api.TraversalRegistry 接口语义、
// shutdown 超时配置覆盖校验。

func newRegistryDeps(t *testing.T) TraversalRegistryDeps {
	t.Helper()
	configDir := t.TempDir()
	return TraversalRegistryDeps{
		ConfigStore:     &fakeAppConfigStore{data: make(map[string][]byte)},
		CheckpointStore: storage.NewFileCheckpointStore(),
		DataDir:         configDir,
	}
}

// fakeAppConfigStore 内存版 AppConfigStore（避免测试依赖文件系统配置目录）。
type fakeAppConfigStore struct {
	data map[string][]byte
}

type blockingInterpolatorLoader struct {
	entered chan struct{}
	release chan struct{}
}

func (l *blockingInterpolatorLoader) LoadPRB(string) (coreinterp.Interpolator, error) {
	close(l.entered)
	<-l.release
	return factoryTestInterpolator{}, nil
}

func (l *blockingInterpolatorLoader) LoadFiveHoleCSV(string) (coreinterp.Interpolator, error) {
	return nil, nil
}

func (l *blockingInterpolatorLoader) LoadMultiPRB([]string, []float64, coreinterp.MultiPrbInterpolationMode) (coreinterp.Interpolator, *ports.MultiPrbLoadMetadata, error) {
	return nil, nil, nil
}

func (l *blockingInterpolatorLoader) LoadSevenHolePRB(string, [6]string) (seveninterp.Interpolator, *ports.SevenHoleLoadMetadata, error) {
	return nil, nil, nil
}

func (l *blockingInterpolatorLoader) LoadSevenHoleCalibrationCSV(string, [6]string) (seveninterp.Interpolator, *ports.SevenHoleLoadMetadata, error) {
	return nil, nil, nil
}

type factoryTestInterpolator struct{}

func (factoryTestInterpolator) IsLoaded() bool { return true }

func (factoryTestInterpolator) GetValidRange() coreinterp.PrbValidRange {
	return coreinterp.PrbValidRange{}
}

func (factoryTestInterpolator) Calculate(coreinterp.InterpolationInput) (coreinterp.InterpolationResult, error) {
	return coreinterp.InterpolationResult{}, nil
}

func (s *fakeAppConfigStore) LoadConfig(key string) ([]byte, error) {
	data, ok := s.data[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (s *fakeAppConfigStore) SaveConfig(key string, data []byte) error {
	s.data[key] = data
	return nil
}

func TestNewTraversalRegistry_PerProbeIsolatedManagers(t *testing.T) {
	deps := newRegistryDeps(t)
	bundle, err := NewTraversalRegistry(deps)
	if err != nil {
		t.Fatalf("NewTraversalRegistry: %v", err)
	}
	if bundle.Registry == nil || bundle.RecoveryIndex == nil {
		t.Fatal("registry 与 recovery index 均应装配")
	}
	ctx := context.Background()

	m1, err := bundle.Registry.GetOrCreate(usecase.Probe1)
	if err != nil {
		t.Fatalf("GetOrCreate probe1: %v", err)
	}
	m2, err := bundle.Registry.GetOrCreate(usecase.Probe2)
	if err != nil {
		t.Fatalf("GetOrCreate probe2: %v", err)
	}
	if m1 == m2 {
		t.Fatal("两 probe 必须获得不同 manager 实例")
	}
	// 同 probe 缓存复用
	again, err := bundle.Registry.GetOrCreate(usecase.Probe1)
	if err != nil || again != m1 {
		t.Fatal("同 probe 应复用缓存 manager")
	}

	// probe-scoped 配置键：各自写入 traversal.probe1/probe2，互不污染，
	// 且不写 legacy "traversal" 键。
	m1.SaveConfigRaw(json.RawMessage(`{"taskId":"p1"}`))
	m2.SaveConfigRaw(json.RawMessage(`{"taskId":"p2"}`))
	store := deps.ConfigStore.(*fakeAppConfigStore)
	if _, ok := store.data["traversal.probe1"]; !ok {
		t.Fatal("probe1 manager 应持久化到 traversal.probe1")
	}
	if _, ok := store.data["traversal.probe2"]; !ok {
		t.Fatal("probe2 manager 应持久化到 traversal.probe2")
	}
	if _, ok := store.data["traversal"]; ok {
		t.Fatal("dual manager 不得写 legacy traversal 键")
	}

	// registry Shutdown（无活动任务）幂等成功
	if err := bundle.Registry.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// closing 后拒绝新任务
	if _, err := bundle.Registry.GetOrCreate(usecase.Probe1); err == nil {
		t.Fatal("Shutdown 后 GetOrCreate 应拒绝")
	}
}

func TestTraversalManagerFactory_NewManagerWaitsForInterpolatorRestore(t *testing.T) {
	deps := newRegistryDeps(t)
	store := deps.ConfigStore.(*fakeAppConfigStore)
	store.data["traversal.probe1"] = []byte(`{"probeType":"five-hole","prbFile":{"filePath":"delayed.prb"}}`)
	loader := &blockingInterpolatorLoader{entered: make(chan struct{}), release: make(chan struct{})}
	deps.InterpLoader = loader
	factory := &traversalManagerFactory{deps: deps}

	type result struct {
		manager usecase.ManagedTraversalManager
		err     error
	}
	done := make(chan result, 1)
	go func() {
		manager, err := factory.NewManager(usecase.Probe1)
		done <- result{manager: manager, err: err}
	}()

	select {
	case <-loader.entered:
	case <-time.After(time.Second):
		t.Fatal("interpolator restore did not start")
	}
	select {
	case got := <-done:
		t.Fatalf("NewManager returned before restore settled: err=%v", got.err)
	default:
	}
	close(loader.release)

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("NewManager: %v", got.err)
		}
		manager := got.manager.(*usecase.TraversalManager)
		if manager.Interpolator() == nil || !manager.Interpolator().IsLoaded() {
			t.Fatal("NewManager must publish settled interpolator state")
		}
	case <-time.After(time.Second):
		t.Fatal("NewManager did not return after restore settled")
	}
}

func TestNewTraversalRegistry_ShutdownTimeoutOverride(t *testing.T) {
	deps := newRegistryDeps(t)
	store := deps.ConfigStore.(*fakeAppConfigStore)

	// 合法覆盖（hard > graceful 的有限正值）→ 生效
	store.data["traversalShutdown"] = []byte(`{"gracefulSeconds":1,"hardSeconds":2}`)
	bundle, err := NewTraversalRegistry(deps)
	if err != nil {
		t.Fatalf("合法覆盖应接受: %v", err)
	}
	if bundle.Registry == nil {
		t.Fatal("registry 应装配")
	}

	// 非法覆盖（hard <= graceful）→ 回退默认且装配成功
	store.data["traversalShutdown"] = []byte(`{"gracefulSeconds":5,"hardSeconds":2}`)
	if _, err := NewTraversalRegistry(deps); err != nil {
		t.Fatalf("非法覆盖应回退默认而非失败: %v", err)
	}
}

func TestNewTraversalRegistry_RejectsIncompleteDeps(t *testing.T) {
	if _, err := NewTraversalRegistry(TraversalRegistryDeps{}); err == nil {
		t.Fatal("缺少必填依赖应返回错误")
	}
	if _, err := NewTraversalRegistry(TraversalRegistryDeps{
		ConfigStore:     &fakeAppConfigStore{data: make(map[string][]byte)},
		CheckpointStore: storage.NewFileCheckpointStore(),
	}); err == nil {
		t.Fatal("缺少 DataDir 应返回错误")
	}
}
