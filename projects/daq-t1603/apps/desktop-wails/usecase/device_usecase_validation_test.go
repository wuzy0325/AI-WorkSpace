package usecase

import (
	"strings"
	"testing"

	"daq-t1603/core"
)

// TestValidateProfileName_UTF8CharCount 验证 UTF-8 字符计数（中文 1 字 = 1，非字节数）。
//
// 测试前置：
//   - 构造 32 字符的中文名称（32 字 = 96 字节，但字符数 = 32）
//   - 构造 33 字符的中文名称（33 字 = 99 字节，字符数 = 33 > 32 上限）
//
// 测试步骤：
//   - 调用 validateProfileName 校验 32 字名称（应通过）
//   - 调用 validateProfileName 校验 33 字名称（应报错）
//
// 期待结果：
//   - 32 字名称返回 nil
//   - 33 字名称返回 error 且消息含字符数 33 和上限 32
func TestValidateProfileName_UTF8CharCount(t *testing.T) {
	// 32 字符中文（恰好达上限，应通过）
	name32 := strings.Repeat("测", 32)
	if err := validateProfileName(name32); err != nil {
		t.Errorf("32-char name should pass, got error: %v", err)
	}

	// 33 字符中文（超上限，应报错）
	name33 := strings.Repeat("测", 33)
	err := validateProfileName(name33)
	if err == nil {
		t.Fatalf("33-char name should fail validation")
	}
	if !strings.Contains(err.Error(), "33") || !strings.Contains(err.Error(), "32") {
		t.Errorf("error message should contain char count 33 and limit 32, got: %v", err)
	}
}

// TestValidateProfileName_EmptyAndShort 验证空名称和短名称均通过。
//
// 测试前置：构造空字符串和 1 字符名称
// 测试步骤：调用 validateProfileName
// 期待结果：均返回 nil（仅校验上限，不校验下限）
func TestValidateProfileName_EmptyAndShort(t *testing.T) {
	if err := validateProfileName(""); err != nil {
		t.Errorf("empty name should pass, got: %v", err)
	}
	if err := validateProfileName("A"); err != nil {
		t.Errorf("1-char name should pass, got: %v", err)
	}
}

// TestValidateChannelNames_Boundary 验证通道名称长度边界（恰好 32 通过、33 报错）。
//
// 测试前置：
//   - 构造 16 通道，前 15 通道名称合法，第 16 通道（索引 15）名称超长
//   - 构造 16 通道全部恰好 32 字符名称
//
// 测试步骤：调用 validateChannelNames
// 期待结果：
//   - 全部 32 字符：通过
//   - 任一通道 33 字符：报错且消息含通道号 16（i+1）
func TestValidateChannelNames_Boundary(t *testing.T) {
	// 全部恰好 32 字符 → 通过
	channels32 := make([]core.ChannelConfig, 16)
	for i := range channels32 {
		channels32[i] = core.ChannelConfig{
			Index:   i,
			Name:    strings.Repeat("CH", 16), // 32 字符
			Enabled: true,
		}
	}
	if err := validateChannelNames(channels32); err != nil {
		t.Errorf("all 32-char channel names should pass, got: %v", err)
	}

	// 第 16 通道（索引 15）超长 → 报错
	channelsOver := make([]core.ChannelConfig, 16)
	for i := range channelsOver {
		channelsOver[i] = core.ChannelConfig{
			Index:   i,
			Name:    "CH",
			Enabled: true,
		}
	}
	channelsOver[15].Name = strings.Repeat("测", 33) // 33 字符
	err := validateChannelNames(channelsOver)
	if err == nil {
		t.Fatalf("channel 16 with 33-char name should fail")
	}
	// 错误消息应含通道号 16（i+1=16）
	if !strings.Contains(err.Error(), "16") || !strings.Contains(err.Error(), "33") {
		t.Errorf("error should contain channel index 16 and char count 33, got: %v", err)
	}
}

// TestValidateChannelNames_EmptyChannels 验证空通道切片通过。
func TestValidateChannelNames_EmptyChannels(t *testing.T) {
	if err := validateChannelNames(nil); err != nil {
		t.Errorf("nil channels should pass, got: %v", err)
	}
	if err := validateChannelNames([]core.ChannelConfig{}); err != nil {
		t.Errorf("empty channels should pass, got: %v", err)
	}
}

// TestUpsertProfile_EnforcesNameValidation 集成验证 UpsertProfile 调用链路触发校验。
//
// 测试前置：构造超长名称的 profile
// 测试步骤：调用 uc.UpsertProfile
// 期待结果：返回 error，且不调用 config.SaveProfile（通过 fakeConfigPort.profiles 为空验证）
func TestUpsertProfile_EnforcesNameValidation(t *testing.T) {
	fakeCfg := newFakeConfigPort()
	uc := NewDeviceUsecase(newFakeDevicePort(), fakeCfg)

	overlongName := strings.Repeat("A", 33)
	profile := core.TemperatureProfile{
		ID:       "test1",
		Name:     overlongName,
		Channels: make([]core.ChannelConfig, 16),
	}
	err := uc.UpsertProfile(profile)
	if err == nil {
		t.Fatalf("UpsertProfile should reject 33-char name")
	}
	// 验证校验失败时未落盘：fakeConfigPort.profiles 应为空
	if saved, _ := fakeCfg.LoadProfiles(); len(saved) != 0 {
		t.Errorf("SaveProfile should not be called on validation failure, got %d profiles", len(saved))
	}
}

// TestUpsertProfile_EnforcesChannelNameValidation 集成验证通道名称超长时 UpsertProfile 拒绝。
//
// 测试前置：
//   - profile.Name 合法（32 字符以内）
//   - 第 16 通道（索引 15）名称超长（33 字符）
//
// 测试步骤：调用 uc.UpsertProfile
// 期待结果：返回 error，且不调用 config.SaveProfile
//
// 这保护"前端 maxlength 失效（如旧配置文件导入）"的兜底场景。
func TestUpsertProfile_EnforcesChannelNameValidation(t *testing.T) {
	fakeCfg := newFakeConfigPort()
	uc := NewDeviceUsecase(newFakeDevicePort(), fakeCfg)

	channels := make([]core.ChannelConfig, 16)
	for i := range channels {
		channels[i] = core.ChannelConfig{
			Index:   i,
			Name:    "CH",
			Enabled: true,
		}
	}
	// 第 16 通道（索引 15）名称超长
	channels[15].Name = strings.Repeat("X", 33)

	profile := core.TemperatureProfile{
		ID:       "test2",
		Name:     "合法名称",
		Channels: channels,
	}
	err := uc.UpsertProfile(profile)
	if err == nil {
		t.Fatalf("UpsertProfile should reject 33-char channel name")
	}
	if saved, _ := fakeCfg.LoadProfiles(); len(saved) != 0 {
		t.Errorf("SaveProfile should not be called on channel name validation failure, got %d profiles", len(saved))
	}
}
