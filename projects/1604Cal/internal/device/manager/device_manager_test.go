package manager_test

import (
	"testing"

	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
)

func TestCheckUnitConsistency(t *testing.T) {
	mgr := manager.NewDeviceManager()

	mgr.Upsert(domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "kPa", Status: domain.DeviceStatusConnected})
	mgr.Upsert(domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Unit: "kPa", Status: domain.DeviceStatusConnected})

	ok, conflictIDs := mgr.CheckUnitConsistency()
	if !ok {
		t.Fatalf("expected consistent units, got conflicts: %v", conflictIDs)
	}
}

func TestCheckUnitConsistencyWithConflict(t *testing.T) {
	mgr := manager.NewDeviceManager()

	mgr.Upsert(domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "kPa", Status: domain.DeviceStatusConnected})
	mgr.Upsert(domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Unit: "psi", Status: domain.DeviceStatusConnected})

	ok, conflictIDs := mgr.CheckUnitConsistency()
	if ok {
		t.Fatal("expected unit consistency check to fail")
	}

	if len(conflictIDs) == 0 {
		t.Fatal("expected conflict device ids")
	}
}

func TestCheckUnitConsistencyIgnoresDisconnected(t *testing.T) {
	mgr := manager.NewDeviceManager()

	// 已连接设备单位一致
	mgr.Upsert(domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "kPa", Status: domain.DeviceStatusConnected})
	mgr.Upsert(domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Unit: "kPa", Status: domain.DeviceStatusConnected})
	// 未连接设备单位不同，不应参与判定
	mgr.Upsert(domain.Device{ID: "p2", Type: domain.DeviceTypePressure, Unit: "psi", Status: domain.DeviceStatusDisconnected})

	ok, conflictIDs := mgr.CheckUnitConsistency()
	if !ok {
		t.Fatalf("expected consistent units for connected devices, got conflicts: %v", conflictIDs)
	}
}

func TestCheckUnitConsistencyCaseInsensitive(t *testing.T) {
	mgr := manager.NewDeviceManager()

	// 大小写不同，但语义相同，应视为一致
	mgr.Upsert(domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "MPa", Status: domain.DeviceStatusConnected})
	mgr.Upsert(domain.Device{ID: "p1", Type: domain.DeviceTypePressure, Unit: "mpa", Status: domain.DeviceStatusConnected})

	ok, conflictIDs := mgr.CheckUnitConsistency()
	if !ok {
		t.Fatalf("expected case-insensitive match, got conflicts: %v", conflictIDs)
	}
}

func TestListReturnsStableSortOrder(t *testing.T) {
	mgr := manager.NewDeviceManager()
	ids := []string{"c-device", "a-device", "b-device"}
	for _, id := range ids {
		mgr.Upsert(domain.Device{ID: id, Name: id})
	}

	for i := 0; i < 10; i++ {
		list := mgr.List()
		if len(list) != 3 {
			t.Fatalf("expected 3 devices, got %d", len(list))
		}
		got := []string{list[0].ID, list[1].ID, list[2].ID}
		want := []string{"a-device", "b-device", "c-device"}
		for j := range want {
			if got[j] != want[j] {
				t.Errorf("iteration %d: List()[%d].ID = %q, want %q", i, j, got[j], want[j])
			}
		}
	}
}
