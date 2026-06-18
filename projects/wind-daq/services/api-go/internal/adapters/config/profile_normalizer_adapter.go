package config

import "wind-daq/services/api-go/internal/core/device"

// ProfileNormalizerAdapter 将 NormalizeProfile 函数适配为 ports.ProfileNormalizer 接口。
// 这样 usecase 可通过 port 接口调用规范化逻辑，无需直接 import adapters/config。
type ProfileNormalizerAdapter struct{}

// NewProfileNormalizer 创建 ProfileNormalizer 适配器
func NewProfileNormalizer() *ProfileNormalizerAdapter {
	return &ProfileNormalizerAdapter{}
}

// Normalize 实现 ports.ProfileNormalizer 接口
func (ProfileNormalizerAdapter) Normalize(profile device.Profile) device.Profile {
	return NormalizeProfile(profile)
}
