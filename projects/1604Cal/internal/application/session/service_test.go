package session_test

import (
	"context"
	"errors"
	"testing"

	"cal1604/internal/application/session"
	"cal1604/internal/device"
	"cal1604/internal/domain"
	"cal1604/internal/events"
	"cal1604/internal/infrastructure/driver"
)

// ── fakes ──

type fakeMeasureDriver struct {
	collectData []float64
	collectErr  error
	valveStatus string
	valveErr    error
	unit        string
	unitErr     error
	info        map[string]string
	infoErr     error
	resetErr    error
	setUnit     string
	zeroCalls   int
	resetCalls  int
}

func (f *fakeMeasureDriver) Connect(_ context.Context) error    { return nil }
func (f *fakeMeasureDriver) Disconnect(_ context.Context) error { return nil }
func (f *fakeMeasureDriver) ReadValveStatus(_ context.Context) (string, error) {
	return f.valveStatus, f.valveErr
}
func (f *fakeMeasureDriver) SetValveStatus(_ context.Context, _ string) error { return nil }
func (f *fakeMeasureDriver) ReadUnit(_ context.Context) (string, error)       { return f.unit, f.unitErr }
func (f *fakeMeasureDriver) SetUnit(_ context.Context, unit string) error {
	f.setUnit = unit
	return f.unitErr
}
func (f *fakeMeasureDriver) CollectData(_ context.Context, _ []int) ([]float64, error) {
	return f.collectData, f.collectErr
}
func (f *fakeMeasureDriver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	return f.info, f.infoErr
}
func (f *fakeMeasureDriver) Reset(_ context.Context) error {
	f.resetCalls++
	return f.resetErr
}
func (f *fakeMeasureDriver) CalibrateZero(_ context.Context, _ []int) ([]float64, error) {
	f.zeroCalls++
	return []float64{0}, nil
}
func (f *fakeMeasureDriver) CalibrateFullScale(_ context.Context, _ []int, _ float64) ([]float64, error) {
	return []float64{1}, nil
}

type fakePressureDriver struct {
	pressure  float64
	pressErr  error
	stable    bool
	stableErr error
}

func (f *fakePressureDriver) Connect(_ context.Context) error                      { return nil }
func (f *fakePressureDriver) Disconnect(_ context.Context) error                   { return nil }
func (f *fakePressureDriver) SetTargetPressure(_ context.Context, _ float64) error { return nil }
func (f *fakePressureDriver) Stop(_ context.Context) error                         { return nil }
func (f *fakePressureDriver) Exhaust(_ context.Context) error                      { return nil }
func (f *fakePressureDriver) ReadCurrentPressure(_ context.Context) (float64, error) {
	return f.pressure, f.pressErr
}
func (f *fakePressureDriver) ReadUnit(_ context.Context) (string, error) { return "kPa", nil }
func (f *fakePressureDriver) SetUnit(_ context.Context, _ string) error  { return nil }
func (f *fakePressureDriver) ReadStability(_ context.Context) (bool, error) {
	return f.stable, f.stableErr
}

type fakeStore struct {
	devices map[string]domain.Device
}

func newFakeStore(devs ...domain.Device) *fakeStore {
	s := &fakeStore{devices: make(map[string]domain.Device)}
	for _, d := range devs {
		s.devices[d.ID] = d
	}
	return s
}

func (s *fakeStore) Upsert(dev domain.Device)                      { s.devices[dev.ID] = dev }
func (s *fakeStore) UpdateStatus(string, domain.DeviceStatus) bool { return true }
func (s *fakeStore) UpdateUnit(id, unit string) bool {
	if d, ok := s.devices[id]; ok {
		d.Unit = unit
		s.devices[id] = d
		return true
	}
	return false
}
func (s *fakeStore) Delete(string)                          {}
func (s *fakeStore) Get(id string) (domain.Device, bool)    { d, ok := s.devices[id]; return d, ok }
func (s *fakeStore) List() []domain.Device                  { return nil }
func (s *fakeStore) CheckUnitConsistency() (bool, []string) { return true, nil }

type publisher struct {
	events []string
}

func (p *publisher) Publish(eventType string, _ any) {
	p.events = append(p.events, eventType)
}

type embedMeasure struct{ device.MeasureDriver }
type embedPressure struct{ device.PressureDriver }

type mapProvider struct {
	drivers map[string]device.ConnectionDriver
}

func (p *mapProvider) GetActiveDriver(id string) device.ConnectionDriver {
	return p.drivers[id]
}

func setupService() (*session.Service, *fakeMeasureDriver, *fakePressureDriver, *publisher) {
	mDrv := &fakeMeasureDriver{
		collectData: []float64{1.0, 2.0, 3.0},
		valveStatus: "measurement",
		unit:        "kPa",
		info:        map[string]string{"model": "WTN1604"},
	}
	pDrv := &fakePressureDriver{pressure: 100.5, stable: true}

	store := newFakeStore(
		domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000},
		domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Model: "ConST811A", Host: "127.0.0.1", Port: 9001},
	)

	mp := &mapProvider{drivers: map[string]device.ConnectionDriver{
		"m1": embedMeasure{mDrv},
		"p1": embedPressure{pDrv},
	}}

	pub := &publisher{}
	svc := session.NewService(store, driver.NewFactory(), pub.Publish, mp)
	return svc, mDrv, pDrv, pub
}

// ── tests ──

func TestBindDevicesSuccess(t *testing.T) {
	svc, _, _, pub := setupService()
	token, err := svc.BindDevices("m1", "p1", "test")
	if err != nil {
		t.Fatalf("BindDevices: %v", err)
	}
	if token.BoundBy != "test" || token.MeasureDeviceID != "m1" {
		t.Fatalf("unexpected token: %+v", token)
	}
	if len(pub.events) != 1 || pub.events[0] != events.EventSessionDeviceBound {
		t.Fatalf("expected session.device_bound, got %v", pub.events)
	}
	if svc.MeasureDriver() == nil {
		t.Fatal("measure driver not bound")
	}
	if svc.PressureDriver() == nil {
		t.Fatal("pressure driver not bound")
	}
}

func TestBindMeasureDevicesMulti(t *testing.T) {
	m1 := &fakeMeasureDriver{collectData: []float64{1.0, 2.0}, unit: "kPa"}
	m2 := &fakeMeasureDriver{collectData: []float64{3.0, 4.0}, unit: "kPa"}
	pDrv := &fakePressureDriver{pressure: 100.5, stable: true}

	store := newFakeStore(
		domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000},
		domain.Device{ID: "m2", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9002},
		domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Model: "ConST811A", Host: "127.0.0.1", Port: 9001},
	)

	mp := &mapProvider{drivers: map[string]device.ConnectionDriver{
		"m1": embedMeasure{m1},
		"m2": embedMeasure{m2},
		"p1": embedPressure{pDrv},
	}}

	svc := session.NewService(store, driver.NewFactory(), func(string, any) {}, mp)

	token, err := svc.BindMeasureDevices([]string{"m1", "m2"}, "p1", "test")
	if err != nil {
		t.Fatalf("BindMeasureDevices: %v", err)
	}
	if len(token.MeasureDeviceIDs) != 2 {
		t.Fatalf("expected 2 measure device ids, got %v", token.MeasureDeviceIDs)
	}
	if svc.MeasureDeviceID() != "m1" {
		t.Fatalf("expected first measure device m1, got %s", svc.MeasureDeviceID())
	}
	if len(svc.MeasureDeviceIDs()) != 2 {
		t.Fatalf("expected 2 bound ids, got %v", svc.MeasureDeviceIDs())
	}
	if len(svc.MeasureDrivers()) != 2 {
		t.Fatalf("expected 2 drivers, got %d", len(svc.MeasureDrivers()))
	}
	if svc.MeasureDriver() == nil {
		t.Fatal("measure driver not bound")
	}

	// 校验通过合法 token 读取指定设备数据
	data, err := svc.ReadMeasureDataForDevice(context.Background(), token, "m2")
	if err != nil {
		t.Fatalf("ReadMeasureDataForDevice m2: %v", err)
	}
	if len(data) != 2 || data[0] != 3.0 {
		t.Fatalf("unexpected m2 data: %v", data)
	}
}

func TestAggregateDeviceStateAndTargetedActions(t *testing.T) {
	m1 := &fakeMeasureDriver{valveStatus: "calibration", unit: "kPa"}
	m2 := &fakeMeasureDriver{valveErr: errors.New("timeout"), unit: "MPa"}
	store := newFakeStore(
		domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "kPa"},
		domain.Device{ID: "m2", Type: domain.DeviceTypeMeasure, Unit: "MPa"},
	)
	provider := &mapProvider{drivers: map[string]device.ConnectionDriver{
		"m1": embedMeasure{m1},
		"m2": embedMeasure{m2},
	}}
	svc := session.NewService(store, driver.NewFactory(), nil, provider)
	token, err := svc.BindMeasureDevices([]string{"m1", "m2"}, "", "measurement")
	if err != nil {
		t.Fatalf("BindMeasureDevices: %v", err)
	}

	valves, err := svc.ReadValveStatusAllDevices(context.Background(), token)
	if err != nil {
		t.Fatalf("ReadValveStatusAllDevices: %v", err)
	}
	if valves["m1"].Value != "calibration" || valves["m1"].Error != "" {
		t.Fatalf("unexpected m1 valve result: %+v", valves["m1"])
	}
	if valves["m2"].Error == "" {
		t.Fatalf("expected per-device read error, got %+v", valves["m2"])
	}

	if err := svc.SetMeasureUnitAllDevices(context.Background(), token, "bar"); err != nil {
		t.Fatalf("SetMeasureUnitAllDevices: %v", err)
	}
	if m1.setUnit != "bar" || m2.setUnit != "bar" {
		t.Fatalf("expected both devices updated, got m1=%q m2=%q", m1.setUnit, m2.setUnit)
	}

	if _, err := svc.CalibrateZeroForDevice(context.Background(), token, "m2", []int{1}); err != nil {
		t.Fatalf("CalibrateZeroForDevice: %v", err)
	}
	if err := svc.ResetDeviceForDevice(context.Background(), token, "m2"); err != nil {
		t.Fatalf("ResetDeviceForDevice: %v", err)
	}
	if m1.zeroCalls != 0 || m1.resetCalls != 0 || m2.zeroCalls != 1 || m2.resetCalls != 1 {
		t.Fatalf("actions targeted wrong device: m1 zero/reset=%d/%d m2=%d/%d", m1.zeroCalls, m1.resetCalls, m2.zeroCalls, m2.resetCalls)
	}
}

func TestCheckUnitConsistencyUsesCurrentBinding(t *testing.T) {
	m1 := &fakeMeasureDriver{unit: "kPa"}
	p1 := &fakePressureDriver{}
	store := newFakeStore(
		domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "kPa", Status: domain.DeviceStatusConnected},
		domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Unit: "kPa", Status: domain.DeviceStatusConnected},
		domain.Device{ID: "unrelated", Type: domain.DeviceTypeMeasure, Unit: "MPa", Status: domain.DeviceStatusConnected},
	)
	provider := &mapProvider{drivers: map[string]device.ConnectionDriver{
		"m1": embedMeasure{m1},
		"p1": embedPressure{p1},
	}}
	svc := session.NewService(store, driver.NewFactory(), nil, provider)
	if _, err := svc.BindDevices("m1", "p1", "measurement"); err != nil {
		t.Fatalf("BindDevices: %v", err)
	}

	consistent, conflicts := svc.CheckUnitConsistency()
	if !consistent || len(conflicts) != 0 {
		t.Fatalf("unrelated connected device must not block session: consistent=%v conflicts=%v", consistent, conflicts)
	}

	p := store.devices["p1"]
	p.Unit = "bar"
	store.devices["p1"] = p
	consistent, conflicts = svc.CheckUnitConsistency()
	if consistent || len(conflicts) != 1 || conflicts[0] != "p1" {
		t.Fatalf("bound pressure mismatch must block session: consistent=%v conflicts=%v", consistent, conflicts)
	}
}

func TestUnbindMeasureDevicesReleasesSessionOwnership(t *testing.T) {
	svc, _, _, _ := setupService()
	if _, err := svc.BindDevices("m1", "p1", "measurement"); err != nil {
		t.Fatalf("BindDevices: %v", err)
	}

	svc.UnbindMeasureDevices()

	if len(svc.MeasureDeviceIDs()) != 0 || svc.MeasureDriver() != nil {
		t.Fatalf("expected measure binding cleared, ids=%v driver=%v", svc.MeasureDeviceIDs(), svc.MeasureDriver())
	}
	if _, err := svc.BindDevices("m1", "p1", "calibration"); err != nil {
		t.Fatalf("expected another module to bind after unbind: %v", err)
	}
}

func TestBindMeasureDevicesEmpty(t *testing.T) {
	svc := session.NewService(newFakeStore(), driver.NewFactory(), func(string, any) {}, nil)
	if _, err := svc.BindMeasureDevices(nil, "p1", "test"); err == nil {
		t.Fatal("expected error for empty measure device ids")
	}
}

func TestBindDevicesMeasureNotFound(t *testing.T) {
	svc := session.NewService(newFakeStore(), driver.NewFactory(), func(string, any) {}, nil)
	_, err := svc.BindDevices("nonexistent", "p1", "test")
	if err == nil {
		t.Fatal("expected error for nonexistent device")
	}
}

func TestBindMeasureDeviceSuccess(t *testing.T) {
	svc, _, _, pub := setupService()
	token, err := svc.BindMeasureDevice("m1", "test")
	if err != nil {
		t.Fatalf("BindMeasureDevice: %v", err)
	}
	if token.BoundBy != "test" || token.MeasureDeviceID != "m1" {
		t.Fatalf("unexpected token: %+v", token)
	}
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(pub.events))
	}
	if svc.MeasureDriver() == nil {
		t.Fatal("measure driver not bound")
	}
}

func TestReadPressureWithoutDevice(t *testing.T) {
	svc := session.NewService(newFakeStore(), driver.NewFactory(), func(string, any) {}, nil)
	_, err := svc.ReadPressure(context.Background(), session.BindingToken{})
	if !errors.Is(err, session.ErrBindingExpired) {
		t.Fatalf("expected ErrBindingExpired, got %v", err)
	}
}

func TestReadPressureAfterBind(t *testing.T) {
	svc, _, pDrv, _ := setupService()
	token, _ := svc.BindDevices("m1", "p1", "test")
	pDrv.pressure = 200.5
	val, err := svc.ReadPressure(context.Background(), token)
	if err != nil {
		t.Fatalf("ReadPressure: %v", err)
	}
	if val != 200.5 {
		t.Fatalf("expected 200.5, got %f", val)
	}
}

func TestReadStabilityAfterBind(t *testing.T) {
	svc, _, pDrv, _ := setupService()
	token, _ := svc.BindDevices("m1", "p1", "test")
	pDrv.stable = false
	val, err := svc.ReadStability(context.Background(), token)
	if err != nil {
		t.Fatalf("ReadStability: %v", err)
	}
	if val {
		t.Fatal("expected false")
	}
}

func TestReadMeasureDataWithoutDevice(t *testing.T) {
	svc := session.NewService(newFakeStore(), driver.NewFactory(), func(string, any) {}, nil)
	_, err := svc.ReadMeasureData(context.Background(), session.BindingToken{})
	if !errors.Is(err, session.ErrBindingExpired) {
		t.Fatalf("expected ErrBindingExpired, got %v", err)
	}
}

func TestReadMeasureDataAfterBind(t *testing.T) {
	svc, mDrv, _, _ := setupService()
	token, _ := svc.BindMeasureDevice("m1", "test")
	mDrv.collectData = []float64{10.1, 20.2, 30.3}
	data, err := svc.ReadMeasureData(context.Background(), token)
	if err != nil {
		t.Fatalf("ReadMeasureData: %v", err)
	}
	if len(data) != 3 || data[0] != 10.1 {
		t.Fatalf("unexpected data: %v", data)
	}
}

func TestReadValveStatus(t *testing.T) {
	svc, mDrv, _, _ := setupService()
	token, _ := svc.BindMeasureDevice("m1", "test")
	mDrv.valveStatus = "calibration"
	val, err := svc.ReadValveStatus(context.Background(), token)
	if err != nil {
		t.Fatalf("ReadValveStatus: %v", err)
	}
	if val != "calibration" {
		t.Fatalf("expected calibration, got %s", val)
	}
}

func TestSetMeasureUnitSyncsDeviceStore(t *testing.T) {
	store := newFakeStore(
		domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000},
	)
	mDrv := &fakeMeasureDriver{collectData: []float64{1, 2, 3}, unit: "kPa"}
	mp := &mapProvider{drivers: map[string]device.ConnectionDriver{"m1": embedMeasure{mDrv}}}
	svc := session.NewService(store, driver.NewFactory(), func(string, any) {}, mp)

	token, err := svc.BindMeasureDevice("m1", "test")
	if err != nil {
		t.Fatalf("bind measure device: %v", err)
	}

	// 初始单位为空；设置单位后应同步到设备存储，供单位一致性检查使用。
	if err := svc.SetMeasureUnit(context.Background(), token, "kPa"); err != nil {
		t.Fatalf("set measure unit: %v", err)
	}

	dev, ok := store.Get("m1")
	if !ok {
		t.Fatal("expected m1 device to exist")
	}
	if dev.Unit != "kPa" {
		t.Fatalf("expected device unit synced to kPa, got %q", dev.Unit)
	}
}

func TestReadMeasureUnit(t *testing.T) {
	svc, mDrv, _, _ := setupService()
	token, _ := svc.BindMeasureDevice("m1", "test")
	mDrv.unit = "MPa"
	val, err := svc.ReadMeasureUnit(context.Background(), token)
	if err != nil {
		t.Fatalf("ReadMeasureUnit: %v", err)
	}
	if val != "MPa" {
		t.Fatalf("expected MPa, got %s", val)
	}
}

func TestReadDeviceInfo(t *testing.T) {
	svc, mDrv, _, _ := setupService()
	token, _ := svc.BindMeasureDevice("m1", "test")
	mDrv.info = map[string]string{"model": "WTN1604", "version": "2.0"}
	info, err := svc.ReadDeviceInfo(context.Background(), token)
	if err != nil {
		t.Fatalf("ReadDeviceInfo: %v", err)
	}
	if info["model"] != "WTN1604" {
		t.Fatalf("unexpected info: %v", info)
	}
}

func TestResetDevice(t *testing.T) {
	svc, mDrv, _, _ := setupService()
	token, _ := svc.BindMeasureDevice("m1", "test")
	if err := svc.ResetDevice(context.Background(), token); err != nil {
		t.Fatalf("ResetDevice: %v", err)
	}
	mDrv.resetErr = errors.New("reset failed")
	if err := svc.ResetDevice(context.Background(), token); err == nil {
		t.Fatal("expected reset error")
	}
}

// TestCalibrateZeroPersistsTareOffsets 验证校零后各通道偏移写回设备配置并持久化，
// 未校零的通道不被改动。
func TestCalibrateZeroPersistsTareOffsets(t *testing.T) {
	store := newFakeStore(
		domain.Device{
			ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604",
			Host: "127.0.0.1", Port: 9000,
			Channels: []domain.ChannelConfig{
				{Index: 1, Enabled: true, Unit: "kPa"},
				{Index: 2, Enabled: true, Unit: "kPa"},
			},
		},
	)
	// 专用 driver：校零返回非零偏移（如 1.5），用于验证偏移被正确持久化。
	mDrv := &calibrateZeroFakeDriver{fakeMeasureDriver: &fakeMeasureDriver{unit: "kPa"}, result: []float64{1.5}}
	mp := &mapProvider{drivers: map[string]device.ConnectionDriver{"m1": embedMeasure{mDrv}}}
	svc := session.NewService(store, driver.NewFactory(), func(string, any) {}, mp)

	token, err := svc.BindMeasureDevice("m1", "test")
	if err != nil {
		t.Fatalf("bind measure device: %v", err)
	}

	if _, err := svc.CalibrateZero(context.Background(), token, []int{1}); err != nil {
		t.Fatalf("calibrate zero: %v", err)
	}

	dev, ok := store.Get("m1")
	if !ok {
		t.Fatal("expected m1 device to exist")
	}
	if len(dev.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(dev.Channels))
	}
	// 校零的通道 1 偏移被持久化。
	if dev.Channels[0].TareOffset != 1.5 {
		t.Fatalf("expected channel1 tareOffset 1.5, got %v", dev.Channels[0].TareOffset)
	}
	// 未校零的通道 2 偏移保持为 0。
	if dev.Channels[1].TareOffset != 0 {
		t.Fatalf("expected channel2 tareOffset 0, got %v", dev.Channels[1].TareOffset)
	}
}

// calibrateZeroFakeDriver 覆盖校零返回可配置偏移的驱动。
type calibrateZeroFakeDriver struct {
	*fakeMeasureDriver
	result []float64
}

func (f *calibrateZeroFakeDriver) CalibrateZero(_ context.Context, _ []int) ([]float64, error) {
	return append([]float64(nil), f.result...), nil
}

func TestBindDevicesOnlyMeasure(t *testing.T) {
	svc, _, _, _ := setupService()
	token, err := svc.BindDevices("m1", "", "test")
	if err != nil {
		t.Fatalf("BindDevices with empty pressure: %v", err)
	}
	if token.PressureDeviceID != "" {
		t.Fatalf("expected empty pressure device id, got %q", token.PressureDeviceID)
	}
	if svc.MeasureDriver() == nil {
		t.Fatal("measure driver not bound")
	}
	if svc.PressureDriver() != nil {
		t.Fatal("expected nil pressure driver")
	}
}
