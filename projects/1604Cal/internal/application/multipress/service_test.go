package multipress

import (
	"context"
	"errors"
	"testing"
	"time"

	"cal1604/internal/device"
	"cal1604/internal/domain"
)

// memoryStore 内存版 DeviceStore，仅用于测试 SetUnit 同步 dev.Unit。
type memoryStore struct {
	devices map[string]domain.Device
}

func (s *memoryStore) Upsert(dev domain.Device) { s.devices[dev.ID] = dev }
func (s *memoryStore) UpdateStatus(id string, st domain.DeviceStatus) bool {
	if d, ok := s.devices[id]; ok {
		d.Status = st
		s.devices[id] = d
		return true
	}
	return false
}
func (s *memoryStore) UpdateUnit(id string, unit string) bool {
	if d, ok := s.devices[id]; ok {
		d.Unit = unit
		s.devices[id] = d
		return true
	}
	return false
}
func (s *memoryStore) Delete(id string) { delete(s.devices, id) }
func (s *memoryStore) Get(id string) (domain.Device, bool) {
	d, ok := s.devices[id]
	return d, ok
}
func (s *memoryStore) List() []domain.Device {
	out := make([]domain.Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	return out
}
func (s *memoryStore) CheckUnitConsistency() (bool, []string) { return true, nil }

type fakeSetUnitDriver struct {
	device.PressureDriver
	unit string
}

func (f *fakeSetUnitDriver) SetUnit(_ context.Context, unit string) error { f.unit = unit; return nil }
func (f *fakeSetUnitDriver) ReadUnit(_ context.Context) (string, error)   { return f.unit, nil }
func (f *fakeSetUnitDriver) ReadCurrentPressure(_ context.Context) (float64, error) {
	return 0, errors.New("not supported")
}

// TestSetUnitSyncsDeviceStore 验证打压设备切换单位后同步到设备存储，供单位一致性检查使用。
func TestSetUnitSyncsDeviceStore(t *testing.T) {
	store := &memoryStore{devices: map[string]domain.Device{
		"p1": {ID: "p1", Type: domain.DeviceTypePressure, Model: "ConST811A"},
	}}
	svc := NewService(nil, store, nil)

	drv := &fakeSetUnitDriver{}
	svc.entries["p1"] = &deviceEntry{
		driver: drv,
		state:  DevicePressureState{DeviceID: "p1"},
	}

	if err := svc.SetUnit(context.Background(), "p1", "kPa"); err != nil {
		t.Fatalf("SetUnit: %v", err)
	}

	if dev, ok := store.Get("p1"); !ok {
		t.Fatal("expected p1 device to exist")
	} else if dev.Unit != "kPa" {
		t.Fatalf("expected device unit synced to kPa, got %q", dev.Unit)
	}
}

func TestStopPollingReturnsAfterStart(t *testing.T) {
	svc := NewService(nil, nil, nil)
	svc.StartPolling()

	done := make(chan struct{})
	go func() {
		svc.StopPolling()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StopPolling did not return after cancellation")
	}
}

func TestStopPollingWithoutStartReturns(t *testing.T) {
	svc := NewService(nil, nil, nil)

	done := make(chan struct{})
	go func() {
		svc.StopPolling()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("StopPolling did not return when polling was not started")
	}
}
