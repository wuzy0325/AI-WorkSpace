package manager

import (
	"strings"
	"testing"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// memStore 测试用内存 profile store，避免 manager 包（usecase 层）依赖 adapters。
type memStore struct {
	profiles []core.MotionControllerProfile
}

func (s *memStore) LoadProfiles() ([]core.MotionControllerProfile, error) {
	return append([]core.MotionControllerProfile(nil), s.profiles...), nil
}

func (s *memStore) SaveProfiles(profiles []core.MotionControllerProfile) error {
	s.profiles = append([]core.MotionControllerProfile(nil), profiles...)
	return nil
}

var _ ports.MotionProfileStore = (*memStore)(nil)

// validEncoderAxis 构造一个合法的编码器源轴：tolerance >= encoderScale，可通过校验。
func validEncoderAxis(name core.AxisName) core.AxisConfig {
	scale := 0.01
	return core.AxisConfig{
		Name:           name,
		Enabled:        true,
		Kind:           core.AxisKindLinear,
		PositionSource: core.PositionSourceEncoder,
		EncoderScale:   &scale,
		EncoderCompensation: &core.AxisEncoderCompensationConfig{
			Enabled:   true,
			Tolerance: 0.02, // > scale(0.01)，可收敛
			MaxCycles: 3,
			SettleMs:  100,
			MinStep:   0.001,
			TimeoutMs: 5000,
		},
	}
}

// unreachableToleranceAxis 构造 error 级非法轴：tolerance < encoderScale，永不收敛。
func unreachableToleranceAxis(name core.AxisName) core.AxisConfig {
	axis := validEncoderAxis(name)
	axis.EncoderCompensation.Tolerance = 0.005 // < scale(0.01)
	return axis
}

func TestValidateProfileCompensationRejectsUnreachableTolerance(t *testing.T) {
	profile := core.MotionControllerProfile{
		ID:    "p1",
		Axes:  []core.AxisConfig{unreachableToleranceAxis(core.AxisX)},
	}
	err := validateProfileCompensation(profile)
	if err == nil {
		t.Fatal("expected error when tolerance < encoderScale, got nil")
	}
	if !strings.Contains(err.Error(), "X") || !strings.Contains(err.Error(), "容差") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateProfileCompensationAcceptsValidConfig(t *testing.T) {
	profile := core.MotionControllerProfile{
		ID:    "p1",
		Axes:  []core.AxisConfig{validEncoderAxis(core.AxisX)},
	}
	if err := validateProfileCompensation(profile); err != nil {
		t.Fatalf("expected nil for valid config, got: %v", err)
	}
}

// 寄存器源轴即使配了非法补偿也不校验：补偿在寄存器源下不生效，校验会误导。
func TestValidateProfileCompensationSkipsRegisterSource(t *testing.T) {
	axis := unreachableToleranceAxis(core.AxisX)
	axis.PositionSource = core.PositionSourceRegister
	profile := core.MotionControllerProfile{
		ID:    "p1",
		Axes:  []core.AxisConfig{axis},
	}
	if err := validateProfileCompensation(profile); err != nil {
		t.Fatalf("expected nil for register-source axis, got: %v", err)
	}
}

// 补偿未启用时不校验。
func TestValidateProfileCompensationSkipsDisabledCompensation(t *testing.T) {
	axis := unreachableToleranceAxis(core.AxisX)
	axis.EncoderCompensation.Enabled = false
	profile := core.MotionControllerProfile{
		ID:    "p1",
		Axes:  []core.AxisConfig{axis},
	}
	if err := validateProfileCompensation(profile); err != nil {
		t.Fatalf("expected nil for disabled compensation, got: %v", err)
	}
}

// UpsertProfile 必须拒绝非法补偿配置，且不写入 store。
func TestUpsertProfileRejectsInvalidCompensation(t *testing.T) {
	store := &memStore{}
	mgr := NewMotionManager(store, nil)

	profile := core.MotionControllerProfile{
		ID:   "bad",
		Name: "Bad",
		Axes: []core.AxisConfig{unreachableToleranceAxis(core.AxisX)},
	}

	if err := mgr.UpsertProfile(profile); err == nil {
		t.Fatal("expected UpsertProfile to reject invalid compensation, got nil")
	}

	// 非法 profile 不应被持久化
	for _, p := range mgr.GetProfiles() {
		if p.ID == "bad" {
			t.Fatal("invalid profile must not be persisted")
		}
	}
}

// 确保 valid profile 正常写入（回归保护）。
func TestUpsertProfileAcceptsValidCompensation(t *testing.T) {
	store := &memStore{}
	mgr := NewMotionManager(store, nil)

	profile := core.MotionControllerProfile{
		ID:   "good",
		Name: "Good",
		Axes: []core.AxisConfig{validEncoderAxis(core.AxisX)},
	}

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("expected UpsertProfile to accept valid config, got: %v", err)
	}

	found := false
	for _, p := range mgr.GetProfiles() {
		if p.ID == "good" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("valid profile should be persisted")
	}
}
