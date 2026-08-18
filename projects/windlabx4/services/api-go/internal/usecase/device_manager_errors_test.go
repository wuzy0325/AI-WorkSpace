package usecase

import (
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
)

// TestDeviceManagerStartAcquisitionUnconnectedReturnsError 验证未连接的设备
// 调用 StartAcquisition 时返回 "device not connected" 错误。
// 覆盖 TC-UC-07：未连接设备启动采集的错误路径。
func TestDeviceManagerStartAcquisitionUnconnectedReturnsError(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		newTestProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager 返回错误: %v", err)
	}

	// 不调用 Connect，直接 StartAcquisition
	err = manager.StartAcquisition("sim-1")
	if err == nil {
		t.Fatal("期望未连接设备 StartAcquisition 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("期望错误包含 'not connected'，实际 %q", err.Error())
	}
}

// TestDeviceManagerApplyDaqT1603ConfigProfileNotFoundReturnsError 验证
// 对不存在的 profile 调用 ApplyDaqT1603Config 时返回 "device profile not found" 错误。
// 覆盖 TC-UC-09：配置不存在 profile 的错误路径。
func TestDeviceManagerApplyDaqT1603ConfigProfileNotFoundReturnsError(t *testing.T) {
	store := &memoryProfileStore{}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager 返回错误: %v", err)
	}

	// 构造合法的 DAQ-T-1603 硬件配置，确保能通过 validateDaqT1603Config 校验
	config := device.DaqT1603HardwareConfig{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK", // 16 个通道的热电偶类型
		ChannelMask:       "FFFF",
		SamplingRate:      10,
		AverageCount:      4,
	}

	err = manager.ApplyDaqT1603Config("nonexistent", config)
	if err == nil {
		t.Fatal("期望不存在的 profile ApplyDaqT1603Config 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "device profile not found") {
		t.Fatalf("期望错误包含 'device profile not found'，实际 %q", err.Error())
	}
}

// TestDeviceManagerConnectProfileNotFoundReturnsError 验证对不存在的 profile
// 调用 Connect 时返回 "device profile not found" 错误。
func TestDeviceManagerConnectProfileNotFoundReturnsError(t *testing.T) {
	store := &memoryProfileStore{}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager 返回错误: %v", err)
	}

	err = manager.Connect("nonexistent")
	if err == nil {
		t.Fatal("期望不存在的 profile Connect 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "device profile not found") {
		t.Fatalf("期望错误包含 'device profile not found'，实际 %q", err.Error())
	}
}

// TestDeviceManagerDeleteProfileNotFoundReturnsError 验证删除不存在的 profile
// 时返回 "device profile not found" 错误。
func TestDeviceManagerDeleteProfileNotFoundReturnsError(t *testing.T) {
	store := &memoryProfileStore{}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager 返回错误: %v", err)
	}

	err = manager.DeleteProfile("nonexistent")
	if err == nil {
		t.Fatal("期望删除不存在的 profile 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "device profile not found") {
		t.Fatalf("期望错误包含 'device profile not found'，实际 %q", err.Error())
	}
}

// TestDeviceManagerSetUnitProfileNotFoundReturnsError 验证对不存在的 profile
// 调用 SetUnit 时返回 "device profile not found" 错误。
func TestDeviceManagerSetUnitProfileNotFoundReturnsError(t *testing.T) {
	store := &memoryProfileStore{}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager 返回错误: %v", err)
	}

	err = manager.SetUnit("nonexistent", "kPa")
	if err == nil {
		t.Fatal("期望不存在的 profile SetUnit 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "device profile not found") {
		t.Fatalf("期望错误包含 'device profile not found'，实际 %q", err.Error())
	}
}

// TestDeviceManagerSetUnitEmptyStringReturnsError 验证 SetUnit 传入空字符串
// 或纯空白字符串时返回 "unit is required" 错误。
// 注意：SetUnit 的空值校验在 profile 查找之前，因此即使 profile 存在也会先报此错。
func TestDeviceManagerSetUnitEmptyStringReturnsError(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		newTestProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := newTestDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager 返回错误: %v", err)
	}

	// 空字符串
	err = manager.SetUnit("sim-1", "")
	if err == nil {
		t.Fatal("期望空字符串 SetUnit 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "unit is required") {
		t.Fatalf("期望错误包含 'unit is required'，实际 %q", err.Error())
	}

	// 纯空白字符串（TrimSpace 后仍为空）
	err = manager.SetUnit("sim-1", "   ")
	if err == nil {
		t.Fatal("期望纯空白字符串 SetUnit 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "unit is required") {
		t.Fatalf("期望错误包含 'unit is required'，实际 %q", err.Error())
	}
}
