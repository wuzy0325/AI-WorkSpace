package usecase

import (
	"fmt"
	"log/slog"

	"daq-p1604/core"
	"daq-p1604/ports"
)

// DeviceUsecase 设备业务逻辑
type DeviceUsecase struct {
	device  ports.DevicePort
	config  ports.ConfigPort
	scanner ports.DeviceScanPort
}

// NewDeviceUsecase 创建设备 usecase
func NewDeviceUsecase(device ports.DevicePort, config ports.ConfigPort, scanner ...ports.DeviceScanPort) *DeviceUsecase {
	uc := &DeviceUsecase{device: device, config: config}
	if len(scanner) > 0 {
		uc.scanner = scanner[0]
	}
	return uc
}

// GetProfiles 获取所有设备配置
func (uc *DeviceUsecase) GetProfiles() []core.PressureProfile {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		slog.Warn("load profiles", "error", err)
		return nil
	}
	if profiles == nil {
		return []core.PressureProfile{}
	}
	return profiles
}

// UpsertProfile 保存设备配置
func (uc *DeviceUsecase) UpsertProfile(profile core.PressureProfile) error {
	return uc.config.SaveProfile(profile)
}

// DeleteProfile 删除设备配置
func (uc *DeviceUsecase) DeleteProfile(id string) error {
	_ = uc.device.Disconnect(id)
	return uc.config.DeleteProfile(id)
}

// Connect 连接设备
// 连接后若硬件单位与 profile 配置不一致，以硬件为准自动更新 profile 并持久化到磁盘。
func (uc *DeviceUsecase) Connect(id string) error {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	var profile *core.PressureProfile
	for i := range profiles {
		if profiles[i].ID == id {
			profile = &profiles[i]
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("profile %s not found", id)
	}
	prevUnit := profile.P1604Cfg.Unit
	if err := uc.device.Connect(*profile); err != nil {
		return err
	}
	// 连接后检查硬件是否同步了单位：若 adapter 已将单位更新到 status 中的 profile，
	// 则持久化到磁盘，确保下次启动时前端配置面板显示的是硬件实际单位。
	state, ok := uc.device.Status(id)
	if ok && state.Profile.P1604Cfg.Unit != prevUnit {
		slog.Info("unit synced from hardware, persisting profile",
			"device", id, "prev", prevUnit, "new", state.Profile.P1604Cfg.Unit)
		if err := uc.config.SaveProfile(state.Profile); err != nil {
			slog.Warn("persist synced profile failed", "device", id, "error", err)
		}
	}
	return nil
}

// Disconnect 断开设备
func (uc *DeviceUsecase) Disconnect(id string) error {
	return uc.device.Disconnect(id)
}

// StartAcquisition 启动采集
func (uc *DeviceUsecase) StartAcquisition(id string) (<-chan core.PressureSnapshot, error) {
	return uc.device.StartAcquisition(id)
}

// StopAcquisition 停止采集
func (uc *DeviceUsecase) StopAcquisition(id string) error {
	return uc.device.StopAcquisition(id)
}

// ZeroCalibration 对设备的全部压力通道执行零点校准
func (uc *DeviceUsecase) ZeroCalibration(id string) error {
	return uc.device.ZeroCalibration(id)
}

// GetStatus 获取设备状态
func (uc *DeviceUsecase) GetStatus(id string) (core.DeviceState, bool) {
	return uc.device.Status(id)
}

// ScanDevices 扫描设备
func (uc *DeviceUsecase) ScanDevices() ([]core.ScanResult, error) {
	if uc.scanner == nil {
		return []core.ScanResult{}, nil
	}
	return uc.scanner.Scan()
}

// ApplyConfig 应用设备配置
func (uc *DeviceUsecase) ApplyConfig(id string, cfg core.P1604Config) error {
	state, ok := uc.device.Status(id)
	if ok && state.Status == core.StatusAcquiring {
		return fmt.Errorf("cannot apply config while acquiring")
	}
	return uc.device.ApplyConfig(id, cfg)
}
