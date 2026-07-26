package usecase

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

// testNormalizer 测试用 ProfileNormalizer，补全缺失通道为 18 通道默认布局。
// 不依赖 adapters/config，避免 usecase test 反向依赖 adapter。
type testNormalizer struct{}

func (testNormalizer) Normalize(profile device.Profile) device.Profile {
	if len(profile.Channels) > 0 {
		return profile
	}
	// 与 adapters/config.defaultSimulatedChannels 一致的简化版（18 通道）
	channels := make([]device.ChannelConfig, 18)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{Index: i, Name: "CH" + string(rune('1'+i)), Enabled: true, Unit: "V"}
	}
	channels[16] = device.ChannelConfig{Index: 16, Name: "大气压", Enabled: true, Unit: "kPa"}
	channels[17] = device.ChannelConfig{Index: 17, Name: "大气温度", Enabled: true, Unit: "degC"}
	profile.Channels = channels
	return profile
}

// newTestProfile 构造测试用设备配置（不依赖 adapters/config）
func newTestProfile(id string, deviceType device.Type) device.Profile {
	return device.Profile{
		ID:           id,
		Name:         id,
		Type:         deviceType,
		Transport:    "tcp",
		SamplingRate: 20,
		AutoConnect:  true,
	}
}

// newTestDeviceManager 构造带 normalizer 的 DeviceManager（测试辅助）
func newTestDeviceManager(store ports.ProfileStore, factory ports.DeviceFactory, dataSink device.DataSink) (*DeviceManager, error) {
	return NewDeviceManagerWithNormalizer(store, factory, dataSink, testNormalizer{})
}

type memoryProfileStore struct {
	profiles []device.Profile
}

func (s *memoryProfileStore) LoadProfiles() ([]device.Profile, error) {
	return append([]device.Profile(nil), s.profiles...), nil
}

func (s *memoryProfileStore) SaveProfiles(profiles []device.Profile) error {
	s.profiles = append([]device.Profile(nil), profiles...)
	return nil
}

type simulatedFactory struct{}

func (simulatedFactory) Create(profile device.Profile) (ports.Device, error) {
	return &fakeDevice{id: profile.ID}, nil
}

type captureFactory struct {
	last *fakeDevice
}

func (f *captureFactory) Create(profile device.Profile) (ports.Device, error) {
	dev := &fakeDevice{id: profile.ID}
	f.last = dev
	return dev, nil
}

type fakeDevice struct {
	id string
	mu sync.Mutex // protects conn / dataSink / emitDone — accessed from both
	// the test goroutine and the emit goroutine spawned in StartAcquisition.
	conn     device.Connection
	dataSink device.DataSink
	emitDone chan struct{}
	unit     string
	// customChannelIndices / customChannels 允许测试自定义发送的通道数据
	// （如含 Index=16 大气压通道）。未设置时 StartAcquisition 用默认 4 通道。
	customChannelIndices []int
	customChannels       []float64
}

func (d *fakeDevice) ID() string { return d.id }

func (d *fakeDevice) Status() device.Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Acquiring 语义等同于 conn==ConnectionAcquiring，Calibrate 依赖此字段判断设备是否在采集
	return device.Status{ID: d.id, Connection: d.conn, Acquiring: d.conn == device.ConnectionAcquiring}
}

func (d *fakeDevice) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conn = device.ConnectionConnected
	return nil
}

func (d *fakeDevice) Disconnect() error {
	d.mu.Lock()
	done := d.emitDone
	d.emitDone = nil
	d.conn = device.ConnectionDisconnected
	d.mu.Unlock()
	if done != nil {
		close(done)
	}
	return nil
}

func (d *fakeDevice) StartAcquisition() error {
	d.mu.Lock()
	d.conn = device.ConnectionAcquiring
	done := make(chan struct{})
	d.emitDone = done
	sink := d.dataSink
	// 支持测试自定义通道数据（如含大气压通道 Index=16）；未设置则用默认 4 通道
	indices := d.customChannelIndices
	values := d.customChannels
	if len(indices) == 0 {
		indices = []int{0, 1, 2, 3}
	}
	if len(values) == 0 {
		values = []float64{1, 2, 3, 4}
	}
	d.mu.Unlock()

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if sink != nil {
					sink(device.DataPayload{
						DeviceID:       d.id,
						Timestamp:      time.Now().UnixMilli(),
						Channels:       append([]float64(nil), values...),
						ChannelIndices: append([]int(nil), indices...),
					})
				}
			}
		}
	}()
	return nil
}

func (d *fakeDevice) StopAcquisition() error {
	d.mu.Lock()
	done := d.emitDone
	d.emitDone = nil
	d.conn = device.ConnectionConnected
	d.mu.Unlock()
	if done != nil {
		close(done)
	}
	return nil
}

func (d *fakeDevice) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dataSink = sink
}

func (d *fakeDevice) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unit = unit
	return nil
}

type fakeScanner struct {
	results []device.ScanResult
}

func (s fakeScanner) Scan() ([]device.ScanResult, error) {
	return append([]device.ScanResult(nil), s.results...), nil
}

func TestDeviceManagerLoadsProfilesFromStore(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		newTestProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	profiles := manager.GetProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected one profile, got %d", len(profiles))
	}
	if profiles[0].ID != "sim-1" {
		t.Fatalf("expected profile sim-1, got %q", profiles[0].ID)
	}
}

func TestDeviceManagerNormalizesStoredProfilesWithNoChannels(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		{
			ID:           "legacy-sim",
			Name:         "Legacy Simulator",
			Type:         device.DeviceSimulated,
			SamplingRate: 20,
		},
	}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	profiles := manager.GetProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected one profile, got %d", len(profiles))
	}
	if len(profiles[0].Channels) != 18 {
		t.Fatalf("expected normalized channels, got %d", len(profiles[0].Channels))
	}
}

func TestDeviceManagerUpsertProfileNormalizesNoChannels(t *testing.T) {
	store := &memoryProfileStore{}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	if err := manager.UpsertProfile(device.Profile{
		ID:           "legacy-sim",
		Name:         "Legacy Simulator",
		Type:         device.DeviceSimulated,
		SamplingRate: 20,
	}); err != nil {
		t.Fatalf("UpsertProfile returned error: %v", err)
	}

	if len(store.profiles) != 1 {
		t.Fatalf("expected one saved profile, got %d", len(store.profiles))
	}
	if len(store.profiles[0].Channels) != 18 {
		t.Fatalf("expected normalized channels to persist, got %d", len(store.profiles[0].Channels))
	}
}

func TestDeviceManagerUpsertProfileAppliesUnitToConnectedDevice(t *testing.T) {
	profile := newTestProfile("p1604-1", device.DeviceDAQP1604)
	profile.Channels = []device.ChannelConfig{
		{Index: 0, Name: "P1", Enabled: true, Unit: "psi"},
		{Index: 1, Name: "P2", Enabled: true, Unit: "psi"},
	}
	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	factory := &captureFactory{}
	manager, err := newTestDeviceManager(store, factory, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	if err := manager.Connect("p1604-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	updated := profile
	updated.Channels = []device.ChannelConfig{
		{Index: 0, Name: "P1", Enabled: true, Unit: "kPa"},
		{Index: 1, Name: "P2", Enabled: true, Unit: "kPa"},
	}
	if err := manager.UpsertProfile(updated); err != nil {
		t.Fatalf("UpsertProfile returned error: %v", err)
	}

	factory.last.mu.Lock()
	got := factory.last.unit
	factory.last.mu.Unlock()
	if got != "kPa" {
		t.Fatalf("expected connected device unit kPa, got %q", got)
	}
}

func TestDeviceManagerScansDevices(t *testing.T) {
	manager, err := newTestDeviceManager(&memoryProfileStore{}, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetScanner(fakeScanner{results: []device.ScanResult{
		{ID: "sim-1", Name: "Simulator 1", Type: device.DeviceSimulated, Available: true},
	}})

	results, err := manager.ScanDevices()
	if err != nil {
		t.Fatalf("ScanDevices returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one scan result, got %d", len(results))
	}
	if results[0].ID != "sim-1" || !results[0].Available {
		t.Fatalf("unexpected scan result: %+v", results[0])
	}
}

func TestDeviceManagerUpsertProfilePersistsProfile(t *testing.T) {
	store := &memoryProfileStore{}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	profile := newTestProfile("sim-1", device.DeviceSimulated)
	profile.Name = "Simulator 1"
	if err := manager.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile returned error: %v", err)
	}

	if len(store.profiles) != 1 {
		t.Fatalf("expected one saved profile, got %d", len(store.profiles))
	}
	if store.profiles[0].Name != "Simulator 1" {
		t.Fatalf("expected saved name Simulator 1, got %q", store.profiles[0].Name)
	}
}

func TestDeviceManagerConnectsProfileAndReportsStatus(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		newTestProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	if err := manager.Connect("sim-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	status, ok := manager.GetStatus("sim-1")
	if !ok {
		t.Fatal("expected status for connected device")
	}
	if status.Connection != device.ConnectionConnected {
		t.Fatalf("expected connected status, got %q", status.Connection)
	}
}

func TestDeviceManagerDisconnectsConnectedDevice(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		newTestProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	if err := manager.Connect("sim-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if err := manager.Disconnect("sim-1"); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
	if _, ok := manager.GetStatus("sim-1"); ok {
		t.Fatal("expected disconnected device to be removed from active statuses")
	}
}

func TestDeviceManagerDeleteProfileDisconnectsAndPersistsRemoval(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		newTestProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	if err := manager.Connect("sim-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	if err := manager.DeleteProfile("sim-1"); err != nil {
		t.Fatalf("DeleteProfile returned error: %v", err)
	}
	if len(manager.GetProfiles()) != 0 {
		t.Fatalf("expected no profiles, got %d", len(manager.GetProfiles()))
	}
	if len(store.profiles) != 0 {
		t.Fatalf("expected persisted profile removal, got %d saved profiles", len(store.profiles))
	}
	if _, ok := manager.GetStatus("sim-1"); ok {
		t.Fatal("expected deleted profile to disconnect active device")
	}
}

func TestDeviceManagerSetUnitUpdatesAllChannelsAndPersists(t *testing.T) {
	profile := newTestProfile("sim-1", device.DeviceSimulated)
	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	if err := manager.SetUnit("sim-1", "kPa"); err != nil {
		t.Fatalf("SetUnit returned error: %v", err)
	}

	saved := store.profiles[0]
	for _, channel := range saved.Channels {
		if channel.Unit != "kPa" {
			t.Fatalf("expected channel %d unit kPa, got %q", channel.Index, channel.Unit)
		}
	}
}

func TestDeviceManagerDaqT1603ConfigPersistsProfileConfig(t *testing.T) {
	profile := newTestProfile("temp-1", device.DeviceDaqT1603)
	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	config := device.DaqT1603HardwareConfig{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
		ChannelMask:       "FFFF",
		SamplingRate:      10,
		BinaryFormat:      false,
		AverageCount:      4,
	}
	if err := manager.ApplyDaqT1603Config("temp-1", config); err != nil {
		t.Fatalf("ApplyDaqT1603Config returned error: %v", err)
	}

	got, err := manager.GetDaqT1603Config("temp-1")
	if err != nil {
		t.Fatalf("GetDaqT1603Config returned error: %v", err)
	}
	if got != config {
		t.Fatalf("expected config %+v, got %+v", config, got)
	}
	if store.profiles[0].DaqT1603Config != config {
		t.Fatalf("expected persisted config %+v, got %+v", config, store.profiles[0].DaqT1603Config)
	}
}

func TestDeviceManagerAcquisitionFeedsAcquisitionHub(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		newTestProfile("sim-1", device.DeviceSimulated),
	}}
	hub := NewAcquisitionHub(&capturePublisher{}, 20)
	manager, err := newTestDeviceManager(store, simulatedFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	if err := manager.Connect("sim-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := manager.StartAcquisition("sim-1"); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer manager.StopAcquisition("sim-1")

	deadline := time.After(700 * time.Millisecond)
	for {
		if payload, ok := hub.GetLatestData("sim-1"); ok && len(payload.Channels) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for acquisition hub data")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// TestCalibrationEnabledForProfileRejectsAtmosphericChannel 验证校零使能查询
// 对大气压辅助通道返回 false，与前端 shouldDisableTare 及 Calibrate 过滤逻辑对齐。
//
// 测试前置：DAQ-P-1604Pre 的大气压通道（Index=16, Unit=Pa）在物理上是环境量。
// 测试步骤：对大气压通道、常规压力通道、DAQ-P-1603 通道 16 分别查询。
// 期待结果：仅大气压通道返回 false，其余压力通道返回 true。
func TestCalibrationEnabledForProfileRejectsAtmosphericChannel(t *testing.T) {
	atmCh := device.ChannelConfig{Index: device.P1604PreAtmChannelIndex, Unit: "Pa", CalibrationEnabled: true}
	if calibrationEnabledForProfile(device.DeviceDAQP1604Pre, atmCh) {
		t.Fatal("大气压通道 calibrationEnabledForProfile 应返回 false")
	}
	atmTempCh := device.ChannelConfig{Index: device.P1604PreAtmTempChannelIndex, Unit: "degC"}
	if calibrationEnabledForProfile(device.DeviceDAQP1604Pre, atmTempCh) {
		t.Fatal("大气温度通道 calibrationEnabledForProfile 应返回 false")
	}
	normalCh := device.ChannelConfig{Index: 0, Unit: "Pa", CalibrationEnabled: true}
	if !calibrationEnabledForProfile(device.DeviceDAQP1604Pre, normalCh) {
		t.Fatal("常规压力通道 calibrationEnabledForProfile 应返回 true")
	}
	// DAQ-P-1603 无大气辅助通道，Index 16 是常规采集通道
	p1603Ch := device.ChannelConfig{Index: 16, Unit: "Pa", CalibrationEnabled: true}
	if !calibrationEnabledForProfile(device.DeviceDAQP1603, p1603Ch) {
		t.Fatal("DAQ-P-1603 通道 16 calibrationEnabledForProfile 应返回 true")
	}
}

// TestCalibrateFiltersAtmosphericChannelOnFullCalibration 验证全部校零时
// 大气压辅助通道被过滤，不写入校零偏移。
//
// 测试前置：DAQ-P-1604Pre 18 通道，fakeDevice 发送 CH0=100Pa + CH16=101325Pa（大气压）。
// 测试步骤：调用 Calibrate(targetChannel=nil) 全部校零。
// 期待结果：CH0 被校零（offset=100Pa），CH16 不被校零（offset=0）。
func TestCalibrateFiltersAtmosphericChannelOnFullCalibration(t *testing.T) {
	profile := newTestProfile("p1604pre-1", device.DeviceDAQP1604Pre)
	channels := make([]device.ChannelConfig, 18)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{Index: i, Name: fmt.Sprintf("CH%d", i+1), Enabled: true, Unit: "Pa", Precision: 2}
	}
	channels[16] = device.ChannelConfig{Index: 16, Name: "Atm", Enabled: true, Unit: "Pa", Precision: 1}
	channels[17] = device.ChannelConfig{Index: 17, Name: "AtmTemp", Enabled: true, Unit: "degC", Precision: 2}
	profile.Channels = channels

	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	factory := &captureFactory{}
	stream := NewCalibrationStream()
	uc := device.NewUnitConverter()
	applier := NewCalibrationApplier(uc)
	// 150ms 采样窗口覆盖 3 个 ticker 周期（50ms×3），确保至少收到 2 帧
	sampler := NewCalibrationSampler(stream, uc, 150*time.Millisecond)
	dataSink := func(payload device.DataPayload) { stream.Publish(payload) }
	manager, err := newTestDeviceManager(store, factory, dataSink)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetCalibrationComponents(applier, sampler)

	if err := manager.Connect("p1604pre-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	// CH0=100Pa 常规压力，CH16=101325Pa 大气压（环境量）
	factory.last.customChannelIndices = []int{0, 16}
	factory.last.customChannels = []float64{100.0, 101325.0}
	if err := manager.StartAcquisition("p1604pre-1"); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer manager.StopAcquisition("p1604pre-1")

	results, err := manager.Calibrate("p1604pre-1", context.Background(), nil)
	if err != nil {
		t.Fatalf("Calibrate returned error: %v", err)
	}

	// 大气压通道不应出现在 results 中
	for _, r := range results {
		if r.ChannelIndex == 16 {
			t.Fatalf("大气压通道(16)不应被校零，但 results 包含: %+v", r)
		}
	}
	// CH0 应被校零，offset=100Pa（基单位）
	foundCH0 := false
	for _, r := range results {
		if r.ChannelIndex == 0 {
			foundCH0 = true
			if r.Offset != 100.0 {
				t.Fatalf("CH0 offset = %v, want 100", r.Offset)
			}
		}
	}
	if !foundCH0 {
		t.Fatalf("CH0 应被校零，但 results 中未找到: %+v", results)
	}
	// profile 中大气压通道的 CalibrationOffset 应为 0
	saved := manager.GetProfiles()
	for _, ch := range saved[0].Channels {
		if ch.Index == 16 && ch.CalibrationOffset != 0 {
			t.Fatalf("大气压通道 CalibrationOffset = %v, want 0", ch.CalibrationOffset)
		}
	}
}

// TestCalibrateRejectsAtmosphericChannelTarget 验证单通道校零大气压通道时
// 返回明确错误，而非静默执行。
//
// 测试前置：DAQ-P-1604Pre 设备已连接且正在采集。
// 测试步骤：对 Index=16（大气压通道）发起单通道校零。
// 期待结果：返回错误，拒绝校零。
func TestCalibrateRejectsAtmosphericChannelTarget(t *testing.T) {
	profile := newTestProfile("p1604pre-2", device.DeviceDAQP1604Pre)
	channels := make([]device.ChannelConfig, 18)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{Index: i, Enabled: true, Unit: "Pa"}
	}
	channels[16] = device.ChannelConfig{Index: 16, Enabled: true, Unit: "Pa"}
	channels[17] = device.ChannelConfig{Index: 17, Enabled: true, Unit: "degC"}
	profile.Channels = channels

	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	factory := &captureFactory{}
	stream := NewCalibrationStream()
	uc := device.NewUnitConverter()
	applier := NewCalibrationApplier(uc)
	sampler := NewCalibrationSampler(stream, uc, 100*time.Millisecond)
	dataSink := func(payload device.DataPayload) { stream.Publish(payload) }
	manager, err := newTestDeviceManager(store, factory, dataSink)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetCalibrationComponents(applier, sampler)

	if err := manager.Connect("p1604pre-2"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := manager.StartAcquisition("p1604pre-2"); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer manager.StopAcquisition("p1604pre-2")

	target := 16
	_, err = manager.Calibrate("p1604pre-2", context.Background(), &target)
	if err == nil {
		t.Fatal("校零大气压通道应返回错误，但 err=nil")
	}
}

// TestCalibrateFiltersDisabledChannelOnFullCalibration 验证 CalibrationEnabled=false
// 的通道在"全部通道校零"模式下不参与采样、不写偏移、不被强制改回 true。
//
// 测试前置：DAQ-P-1603 16 通道，CH00/CH01 CalibrationEnabled=false（用户禁用校零应用），
// fakeDevice 发送所有通道 = 100 Pa。
// 测试步骤：调用 Calibrate(targetChannel=nil) 全部校零。
// 期待结果：
//   - CH00/CH01 不出现在 results 中
//   - CH02 出现在 results 中且 offset=100
//   - 落库后 CH00/CH01 的 CalibrationEnabled 仍为 false（未被强制覆盖）
//   - 落库后 CH00/CH01 的 CalibrationOffset 仍为 0（未写偏移）
//   - 落库后 CH02 的 CalibrationEnabled 仍为 true、CalibrationOffset=100
//
// 回归症状：若 Calibrate 全部模式未过滤 CalibrationEnabled=false 通道，
// 或落库阶段仍强制 CalibrationEnabled=true，CH00/CH01 会被采样并写偏移，
// 用户禁用状态被抹掉，下次实时数据被错误减去偏移——"禁用了还给我校零"。
func TestCalibrateFiltersDisabledChannelOnFullCalibration(t *testing.T) {
	profile := newTestProfile("p1603-cal-1", device.DeviceDAQP1603)
	channels := make([]device.ChannelConfig, 16)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{
			Index: i, Name: fmt.Sprintf("CH%d", i+1), Enabled: true,
			Unit: "Pa", Precision: 3,
			CalibrationEnabled: i >= 2, // CH00/CH01 禁用校零应用
		}
	}
	profile.Channels = channels

	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	factory := &captureFactory{}
	stream := NewCalibrationStream()
	uc := device.NewUnitConverter()
	applier := NewCalibrationApplier(uc)
	sampler := NewCalibrationSampler(stream, uc, 150*time.Millisecond)
	dataSink := func(payload device.DataPayload) { stream.Publish(payload) }
	manager, err := newTestDeviceManager(store, factory, dataSink)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetCalibrationComponents(applier, sampler)

	if err := manager.Connect("p1603-cal-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	// 所有通道都发 100 Pa
	factory.last.customChannelIndices = []int{0, 1, 2}
	factory.last.customChannels = []float64{100.0, 100.0, 100.0}
	if err := manager.StartAcquisition("p1603-cal-1"); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer manager.StopAcquisition("p1603-cal-1")

	results, err := manager.Calibrate("p1603-cal-1", context.Background(), nil)
	if err != nil {
		t.Fatalf("Calibrate returned error: %v", err)
	}

	// CH00/CH01 不应出现在 results 中
	for _, r := range results {
		if r.ChannelIndex == 0 || r.ChannelIndex == 1 {
			t.Errorf("禁用校零应用的通道 %d 不应被校零，但 results 包含: %+v", r.ChannelIndex, r)
		}
	}
	// CH02 应被校零，offset=100 Pa
	foundCH02 := false
	for _, r := range results {
		if r.ChannelIndex == 2 {
			foundCH02 = true
			if r.Offset != 100.0 {
				t.Errorf("CH02 offset = %v, want 100", r.Offset)
			}
		}
	}
	if !foundCH02 {
		t.Errorf("CH02 应被校零，但 results 中未找到: %+v", results)
	}

	// 落库后验证：CH00/CH01 禁用状态与偏移都保留，CH02 偏移已写、使能仍 true
	saved := manager.GetProfiles()
	for _, ch := range saved[0].Channels {
		if ch.Index == 0 || ch.Index == 1 {
			if ch.CalibrationEnabled {
				t.Errorf("CH%02d CalibrationEnabled = true，应保持 false（用户禁用不被覆盖）", ch.Index+1)
			}
			if ch.CalibrationOffset != 0 {
				t.Errorf("CH%02d CalibrationOffset = %v，应保持 0（禁用通道不写偏移）", ch.Index+1, ch.CalibrationOffset)
			}
		}
		if ch.Index == 2 {
			if !ch.CalibrationEnabled {
				t.Errorf("CH02 CalibrationEnabled = false，应保持 true")
			}
			if ch.CalibrationOffset != 100.0 {
				t.Errorf("CH02 CalibrationOffset = %v, want 100", ch.CalibrationOffset)
			}
		}
	}
}

// TestCalibrateRejectsDisabledChannelTarget 验证单通道校准对 CalibrationEnabled=false
// 的通道返回明确错误，而非静默执行并强制覆盖使能位。
//
// 测试前置：DAQ-P-1603 设备已连接且正在采集，CH00 CalibrationEnabled=false。
// 测试步骤：对 Index=0 发起单通道校零。
// 期待结果：返回错误，拒绝校零。
func TestCalibrateRejectsDisabledChannelTarget(t *testing.T) {
	profile := newTestProfile("p1603-cal-2", device.DeviceDAQP1603)
	channels := make([]device.ChannelConfig, 16)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{
			Index: i, Enabled: true, Unit: "Pa",
			CalibrationEnabled: i >= 2, // CH00/CH01 禁用
		}
	}
	profile.Channels = channels

	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	factory := &captureFactory{}
	stream := NewCalibrationStream()
	uc := device.NewUnitConverter()
	applier := NewCalibrationApplier(uc)
	sampler := NewCalibrationSampler(stream, uc, 100*time.Millisecond)
	dataSink := func(payload device.DataPayload) { stream.Publish(payload) }
	manager, err := newTestDeviceManager(store, factory, dataSink)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetCalibrationComponents(applier, sampler)

	if err := manager.Connect("p1603-cal-2"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := manager.StartAcquisition("p1603-cal-2"); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer manager.StopAcquisition("p1603-cal-2")

	target := 0
	_, err = manager.Calibrate("p1603-cal-2", context.Background(), &target)
	if err == nil {
		t.Fatal("校零已禁用校零应用的通道应返回错误，但 err=nil")
	}
}
