package motion

import (
	"context"

	motionmanager "shared.local/motion-control/go/manager"
	sharedcore "shared.local/device-sdk/go/motion/core"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

var _ ports.MotionManager = (*motionManagerWrapper)(nil)

type motionManagerWrapper struct {
	inner *motionmanager.MotionManager
}

func WrapMotionManager(raw *motionmanager.MotionManager) ports.MotionManager {
	return &motionManagerWrapper{inner: raw}
}

func (w *motionManagerWrapper) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	profiles, err := w.inner.LoadProfiles()
	if err != nil {
		return nil, err
	}
	return toProjectProfiles(profiles), nil
}

func (w *motionManagerWrapper) SaveProfiles(profiles []motion.MotionControllerProfile) error {
	return w.inner.SaveProfiles(toSharedProfiles(profiles))
}

func (w *motionManagerWrapper) GetProfiles() []motion.MotionControllerProfile {
	return toProjectProfiles(w.inner.GetProfiles())
}

func (w *motionManagerWrapper) UpsertProfile(profile motion.MotionControllerProfile) error {
	return w.inner.UpsertProfile(toSharedProfile(profile))
}

func (w *motionManagerWrapper) DeleteProfile(id string) error {
	return w.inner.DeleteProfile(id)
}

func (w *motionManagerWrapper) Connect(ctx context.Context, id string) error {
	return w.inner.Connect(ctx, id)
}

func (w *motionManagerWrapper) Disconnect(ctx context.Context, id string) error {
	return w.inner.Disconnect(ctx, id)
}

func (w *motionManagerWrapper) StatusAll(ctx context.Context) []motion.ControllerStatus {
	return toProjectStatuses(w.inner.StatusAll(ctx))
}

func (w *motionManagerWrapper) MoveTo(ctx context.Context, id string, axis motion.AxisName, position float64) error {
	return w.inner.MoveTo(ctx, id, sharedcore.AxisName(axis), position)
}

func (w *motionManagerWrapper) MoveBy(ctx context.Context, id string, axis motion.AxisName, delta float64) error {
	return w.inner.MoveBy(ctx, id, sharedcore.AxisName(axis), delta)
}

func (w *motionManagerWrapper) Jog(ctx context.Context, id string, axis motion.AxisName, velocity float64) error {
	return w.inner.Jog(ctx, id, sharedcore.AxisName(axis), velocity)
}

func (w *motionManagerWrapper) Home(ctx context.Context, id string, axis motion.AxisName) error {
	return w.inner.Home(ctx, id, sharedcore.AxisName(axis))
}

func (w *motionManagerWrapper) Stop(ctx context.Context, id string, axis motion.AxisName) error {
	return w.inner.Stop(ctx, id, sharedcore.AxisName(axis))
}

func (w *motionManagerWrapper) EmergencyStop(ctx context.Context, id string) error {
	return w.inner.EmergencyStop(ctx, id)
}

func (w *motionManagerWrapper) ResetEmergencyStop(ctx context.Context, id string) error {
	return w.inner.ResetEmergencyStop(ctx, id)
}

func (w *motionManagerWrapper) DefinePosition(ctx context.Context, id string, axis motion.AxisName, position float64) error {
	return w.inner.DefinePosition(ctx, id, sharedcore.AxisName(axis), position)
}

func toProjectProfile(p sharedcore.MotionControllerProfile) motion.MotionControllerProfile {
	axes := make([]motion.AxisConfig, len(p.Axes))
	for i, a := range p.Axes {
		axes[i] = motion.AxisConfig{
			Name:     motion.AxisName(a.Name),
			Enabled:  a.Enabled,
			Kind:     motion.AxisKind(a.Kind),
			MaxSpeed: a.MaxSpeed,
			MinLimit: a.MinLimit,
			MaxLimit: a.MaxLimit,
			Inverted: a.Inverted,
		}
	}
	return motion.MotionControllerProfile{
		ID:          p.ID,
		Name:        p.Name,
		Type:        motion.ControllerType(p.Type),
		Address:     p.Address,
		Port:        p.Port,
		AutoConnect: p.AutoConnect,
		Axes:        axes,
	}
}

func toProjectProfiles(profiles []sharedcore.MotionControllerProfile) []motion.MotionControllerProfile {
	if profiles == nil {
		return nil
	}
	out := make([]motion.MotionControllerProfile, len(profiles))
	for i, p := range profiles {
		out[i] = toProjectProfile(p)
	}
	return out
}

func toSharedProfile(p motion.MotionControllerProfile) sharedcore.MotionControllerProfile {
	axes := make([]sharedcore.AxisConfig, len(p.Axes))
	for i, a := range p.Axes {
		axes[i] = sharedcore.AxisConfig{
			Name:     sharedcore.AxisName(a.Name),
			Enabled:  a.Enabled,
			Kind:     sharedcore.AxisKind(a.Kind),
			MaxSpeed: a.MaxSpeed,
			MinLimit: a.MinLimit,
			MaxLimit: a.MaxLimit,
			Inverted: a.Inverted,
		}
	}
	return sharedcore.MotionControllerProfile{
		ID:          p.ID,
		Name:        p.Name,
		Type:        sharedcore.ControllerType(p.Type),
		Address:     p.Address,
		Port:        p.Port,
		AutoConnect: p.AutoConnect,
		Axes:        axes,
	}
}

func toSharedProfiles(profiles []motion.MotionControllerProfile) []sharedcore.MotionControllerProfile {
	if profiles == nil {
		return nil
	}
	out := make([]sharedcore.MotionControllerProfile, len(profiles))
	for i, p := range profiles {
		out[i] = toSharedProfile(p)
	}
	return out
}

func toProjectAxisStatus(s sharedcore.AxisStatus) motion.AxisStatus {
	return motion.AxisStatus{
		Name:     motion.AxisName(s.Name),
		Position: s.Position,
		Velocity: s.Velocity,
		Moving:   s.Moving,
		Homed:    s.Homed,
		PosLimit: s.PosLimit,
		NegLimit: s.NegLimit,
	}
}

func toProjectStatus(s sharedcore.ControllerStatus) motion.ControllerStatus {
	axes := make([]motion.AxisStatus, len(s.Axes))
	for i, a := range s.Axes {
		axes[i] = toProjectAxisStatus(a)
	}
	return motion.ControllerStatus{
		ID:               s.ID,
		Name:             s.Name,
		Type:             motion.ControllerType(s.Type),
		Connected:        s.Connected,
		EmergencyStopped: s.EmergencyStopped,
		Axes:             axes,
		LastError:        s.LastError,
	}
}

func toProjectStatuses(statuses []sharedcore.ControllerStatus) []motion.ControllerStatus {
	if statuses == nil {
		return nil
	}
	out := make([]motion.ControllerStatus, len(statuses))
	for i, s := range statuses {
		out[i] = toProjectStatus(s)
	}
	return out
}
