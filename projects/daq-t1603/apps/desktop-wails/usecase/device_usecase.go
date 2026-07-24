package usecase

import (
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"unicode/utf8"

	"daq-t1603/core"
	"daq-t1603/ports"
)

// 名称长度上限（CONN-001/CFG-014 后端兜底校验）。
// 前端通过 maxlength=32 限制输入，但若通过直接调 backend API 或导入旧配置文件
// 传入超长名称，前端约束可被绕过，因此 usecase 层兜底校验。
// UTF-8 字符计数（不是字节数），中文一个字算 1。
const (
	maxProfileNameLen = 32
	maxChannelNameLen = 32
)

type DeviceUsecase struct {
	device ports.DevicePort
	config ports.ConfigPort
	scanner ports.DeviceScanPort
}

func NewDeviceUsecase(device ports.DevicePort, config ports.ConfigPort, scanner ...ports.DeviceScanPort) *DeviceUsecase {
	uc := &DeviceUsecase{device: device, config: config}
	if len(scanner) > 0 {
		uc.scanner = scanner[0]
	}
	return uc
}

func (uc *DeviceUsecase) GetProfiles() []core.TemperatureProfile {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		slog.Warn("load profiles", "error", err)
		return nil
	}
	if profiles == nil {
		return []core.TemperatureProfile{}
	}
	return profiles
}

// validateProfileName 长度校验（CONN-001/CFG-014 后端兜底）。
// 返回 error 让调用方决定如何反馈给用户。
func validateProfileName(name string) error {
	if n := utf8.RuneCountInString(name); n > maxProfileNameLen {
		return fmt.Errorf("设备名称长度 %d 超过上限 %d", n, maxProfileNameLen)
	}
	return nil
}

// validateChannelNames 校验所有通道名称长度（CFG-014 后端兜底）。
func validateChannelNames(channels []core.ChannelConfig) error {
	for i, ch := range channels {
		if n := utf8.RuneCountInString(ch.Name); n > maxChannelNameLen {
			return fmt.Errorf("通道 %d 名称长度 %d 超过上限 %d", i+1, n, maxChannelNameLen)
		}
	}
	return nil
}

func (uc *DeviceUsecase) UpsertProfile(profile core.TemperatureProfile) error {
	// 后端兜底校验：前端 maxlength 已限制输入，此处防止绕过 UI 的非法数据落盘
	if err := validateProfileName(profile.Name); err != nil {
		return err
	}
	if err := validateChannelNames(profile.Channels); err != nil {
		return err
	}
	return uc.config.SaveProfile(profile)
}

func (uc *DeviceUsecase) DeleteProfile(id string) error {
	_ = uc.device.Disconnect(id)
	return uc.config.DeleteProfile(id)
}

func (uc *DeviceUsecase) Connect(id string) error {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	var profile *core.TemperatureProfile
	for i := range profiles {
		if profiles[i].ID == id {
			profile = &profiles[i]
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("profile %s not found", id)
	}
	return uc.device.Connect(*profile)
}

func (uc *DeviceUsecase) Disconnect(id string) error {
	return uc.device.Disconnect(id)
}

func (uc *DeviceUsecase) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	return uc.device.StartAcquisition(id)
}

func (uc *DeviceUsecase) StopAcquisition(id string) error {
	return uc.device.StopAcquisition(id)
}

func (uc *DeviceUsecase) GetStatus(id string) (core.DeviceState, bool) {
	return uc.device.Status(id)
}

func (uc *DeviceUsecase) ScanDevices() ([]core.ScanResult, error) {
	if uc.scanner == nil {
		return []core.ScanResult{}, nil
	}
	return uc.scanner.Scan()
}

func (uc *DeviceUsecase) ApplyConfig(id string, cfg core.T1603Config) error {
	state, ok := uc.device.Status(id)
	if ok && state.Status == core.StatusAcquiring {
		return fmt.Errorf("cannot apply config while acquiring")
	}
	return uc.device.ApplyConfig(id, cfg)
}
