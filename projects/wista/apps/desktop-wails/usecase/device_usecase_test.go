package usecase

import (
	"errors"
	"sync"
	"testing"

	"wista/core"
)

type fakeDevicePort struct {
	mu       sync.Mutex
	conns    map[string]core.TemperatureProfile
	acquired map[string]chan struct{}
}

func newFakeDevicePort() *fakeDevicePort {
	return &fakeDevicePort{
		conns:    make(map[string]core.TemperatureProfile),
		acquired: make(map[string]chan struct{}),
	}
}

func (f *fakeDevicePort) Connect(profile core.TemperatureProfile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.conns[profile.ID]; ok {
		return errors.New("already connected")
	}
	f.conns[profile.ID] = profile
	return nil
}

func (f *fakeDevicePort) Disconnect(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.conns, id)
	if ch, ok := f.acquired[id]; ok {
		close(ch)
		delete(f.acquired, id)
	}
	return nil
}

func (f *fakeDevicePort) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.conns[id]; !ok {
		return nil, errors.New("not connected")
	}
	ch := make(chan core.TemperatureSnapshot)
	release := make(chan struct{})
	f.acquired[id] = release
	go func() {
		ch <- core.TemperatureSnapshot{DeviceID: id, Timestamp: 1, Values: make([]float64, 16)}
		<-release
		close(ch)
	}()
	return ch, nil
}

func (f *fakeDevicePort) StopAcquisition(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if release, ok := f.acquired[id]; ok {
		close(release)
		delete(f.acquired, id)
	}
	return nil
}

func (f *fakeDevicePort) Status(id string) (core.DeviceState, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.conns[id]; ok {
		return core.DeviceState{Profile: p, Status: core.StatusConnected, StatusText: core.StatusConnected.String()}, true
	}
	return core.DeviceState{}, false
}

func (f *fakeDevicePort) ApplyConfig(id string, cfg core.T1603Config) error {
	return nil
}

func (f *fakeDevicePort) SetDataSink(id string, sink func(core.TemperatureSnapshot)) {
}

type fakeConfigPort struct {
	mu       sync.Mutex
	profiles map[string]core.TemperatureProfile
}

func newFakeConfigPort() *fakeConfigPort {
	return &fakeConfigPort{profiles: make(map[string]core.TemperatureProfile)}
}

func (f *fakeConfigPort) LoadProfiles() ([]core.TemperatureProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]core.TemperatureProfile, 0, len(f.profiles))
	for _, p := range f.profiles {
		result = append(result, p)
	}
	return result, nil
}

func (f *fakeConfigPort) SaveProfile(profile core.TemperatureProfile) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[profile.ID] = profile
	return nil
}

func (f *fakeConfigPort) DeleteProfile(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.profiles, id)
	return nil
}

type fakeRecordingPort struct {
	mu       sync.Mutex
	status   core.RecordingStatus
	count    int
}

func newFakeRecordingPort() *fakeRecordingPort {
	return &fakeRecordingPort{}
}

func (f *fakeRecordingPort) Start(outputDir string, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = core.RecordingActive
	f.count = 0
	return nil
}

func (f *fakeRecordingPort) Write(snapshot core.TemperatureSnapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status == core.RecordingActive {
		f.count++
	}
	return nil
}

func (f *fakeRecordingPort) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = core.RecordingIdle
	return nil
}

func (f *fakeRecordingPort) IsActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status == core.RecordingActive
}

func (f *fakeRecordingPort) Status() core.RecordingSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return core.RecordingSession{
		Status:        f.status,
		SnapshotCount: f.count,
	}
}

// SetBackpressureHandler 测试 stub，记录是否被调用即可。
func (f *fakeRecordingPort) SetBackpressureHandler(handler func(core.BackpressureEvent)) {
	// 测试环境无需真正注入；保留空实现以满足接口契约
}

// SetFatalErrorHandler 测试 stub，保留空实现以满足接口契约。
func (f *fakeRecordingPort) SetFatalErrorHandler(handler func(deviceID string, err error)) {
	// 测试环境无需真正注入
}

// SetDeviceProfile 测试 stub（REC-006），保留空实现以满足接口契约。
// 测试环境不验证通道掩码透传逻辑，由 adapters/recording 包的独立测试覆盖。
func (f *fakeRecordingPort) SetDeviceProfile(deviceID string, channels []core.ChannelConfig) {
	// 测试环境无需真正注入
}

type fakeScanPort struct {
	results []core.ScanResult
}

func newFakeScanPort(results []core.ScanResult) *fakeScanPort {
	return &fakeScanPort{results: results}
}

func (f *fakeScanPort) Scan() ([]core.ScanResult, error) {
	return f.results, nil
}

func TestDeviceUsecase_GetProfiles_Empty(t *testing.T) {
	dp := newFakeDevicePort()
	cp := newFakeConfigPort()
	uc := NewDeviceUsecase(dp, cp)

	profiles := uc.GetProfiles()
	if len(profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestDeviceUsecase_UpsertAndGet(t *testing.T) {
	uc := NewDeviceUsecase(newFakeDevicePort(), newFakeConfigPort())

	p := core.TemperatureProfile{ID: "dev1", Name: "Test", Address: "x", Port: 1}
	if err := uc.UpsertProfile(p); err != nil {
		t.Fatal(err)
	}

	profiles := uc.GetProfiles()
	if len(profiles) != 1 || profiles[0].ID != "dev1" {
		t.Fatalf("expected 1 profile (dev1), got %v", profiles)
	}
}

func TestDeviceUsecase_ConnectMissingProfile(t *testing.T) {
	uc := NewDeviceUsecase(newFakeDevicePort(), newFakeConfigPort())

	err := uc.Connect("nonexistent")
	if err == nil {
		t.Fatal("expected error connecting missing profile")
	}
}

func TestDeviceUsecase_ConnectExistingProfile(t *testing.T) {
	cp := newFakeConfigPort()
	dp := newFakeDevicePort()
	uc := NewDeviceUsecase(dp, cp)

	_ = cp.SaveProfile(core.TemperatureProfile{ID: "dev1", Name: "t1", Address: "x", Port: 1})
	if err := uc.Connect("dev1"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
}

func TestDeviceUsecase_StartStopAcquisition(t *testing.T) {
	cp := newFakeConfigPort()
	dp := newFakeDevicePort()
	uc := NewDeviceUsecase(dp, cp)

	_ = cp.SaveProfile(core.TemperatureProfile{ID: "dev1"})
	_ = uc.Connect("dev1")

	ch, err := uc.StartAcquisition("dev1")
	if err != nil {
		t.Fatalf("StartAcquisition: %v", err)
	}

	snap, ok := <-ch
	if !ok {
		t.Fatal("expected snapshot on channel")
	}
	if snap.DeviceID != "dev1" {
		t.Fatalf("expected dev1, got %s", snap.DeviceID)
	}

	if err := uc.StopAcquisition("dev1"); err != nil {
		t.Fatalf("StopAcquisition: %v", err)
	}
}

func TestDeviceUsecase_ApplyConfig(t *testing.T) {
	cp := newFakeConfigPort()
	dp := newFakeDevicePort()
	uc := NewDeviceUsecase(dp, cp)

	_ = cp.SaveProfile(core.TemperatureProfile{ID: "dev1"})
	_ = uc.Connect("dev1")

	err := uc.ApplyConfig("dev1", core.T1603Config{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK", ChannelMask: "FFFF", SamplingRate: 10, AverageCount: 4,
	})
	if err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
}

func TestDeviceUsecase_ScanDevices_NilScanner(t *testing.T) {
	uc := NewDeviceUsecase(newFakeDevicePort(), newFakeConfigPort())
	results, err := uc.ScanDevices()
	if err != nil {
		t.Fatalf("ScanDevices: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results with nil scanner, got %d", len(results))
	}
}

func TestDeviceUsecase_ScanDevices_WithScanner(t *testing.T) {
	expected := []core.ScanResult{
		{ID: "dev1", Address: "10.0.0.1", Port: 9000},
		{ID: "dev2", Address: "10.0.0.2", Port: 9000},
	}
	uc := NewDeviceUsecase(newFakeDevicePort(), newFakeConfigPort(), newFakeScanPort(expected))

	results, err := uc.ScanDevices()
	if err != nil {
		t.Fatalf("ScanDevices: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "dev1" {
		t.Fatalf("expected dev1 first, got %s", results[0].ID)
	}
}

func TestRecordingUsecase(t *testing.T) {
	rec := newFakeRecordingPort()
	uc := NewRecordingUsecase(rec)

	if err := uc.Start("/tmp", "test"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := uc.Write(core.TemperatureSnapshot{DeviceID: "d1", Timestamp: 1, Values: make([]float64, 16)}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := uc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	session := uc.Status()
	if session.SnapshotCount != 1 {
		t.Fatalf("expected 1 snapshot count, got %d", session.SnapshotCount)
	}
}
