package backend

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"shared.local/device-sdk/go/motion/core"
	"motion-controller/services/api-go/pkg/appcontext"
)

type App struct {
	ctx          context.Context
	appCtx       *appcontext.AppContext
	statusCancel context.CancelFunc
}

func NewApp(appCtx *appcontext.AppContext) *App {
	return &App{appCtx: appCtx}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	a.appCtx.MotionManager.LoadProfiles()

	statusCtx, cancel := context.WithCancel(ctx)
	a.statusCancel = cancel
	go a.emitStatusLoop(statusCtx)
}

func (a *App) Shutdown(ctx context.Context) {
	if a.statusCancel != nil {
		a.statusCancel()
	}
}

func (a *App) emitStatusLoop(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			statuses := a.MotionGetStatus()
			if len(statuses) > 0 {
				runtime.EventsEmit(a.ctx, "motion:status", statuses)
			}
		}
	}
}

func (a *App) MotionGetProfiles() []core.MotionControllerProfile {
	return a.appCtx.MotionManager.GetProfiles()
}

func (a *App) MotionUpsertProfile(profile core.MotionControllerProfile) error {
	return a.appCtx.MotionManager.UpsertProfile(profile)
}

func (a *App) MotionDeleteProfile(id string) error {
	return a.appCtx.MotionManager.DeleteProfile(id)
}

func (a *App) MotionConnect(id string) error {
	return a.appCtx.MotionManager.Connect(a.ctx, id)
}

func (a *App) MotionDisconnect(id string) error {
	return a.appCtx.MotionManager.Disconnect(a.ctx, id)
}

func (a *App) MotionGetStatus() []core.ControllerStatus {
	return a.appCtx.MotionManager.StatusAll(a.ctx)
}

func (a *App) MotionMoveTo(id string, axis string, position float64) error {
	return a.appCtx.MotionManager.MoveTo(a.ctx, id, core.AxisName(axis), position)
}

func (a *App) MotionMoveBy(id string, axis string, delta float64) error {
	return a.appCtx.MotionManager.MoveBy(a.ctx, id, core.AxisName(axis), delta)
}

func (a *App) MotionJog(id string, axis string, velocity float64) error {
	return a.appCtx.MotionManager.Jog(a.ctx, id, core.AxisName(axis), velocity)
}

func (a *App) MotionHome(id string, axis string) error {
	return a.appCtx.MotionManager.Home(a.ctx, id, core.AxisName(axis))
}

func (a *App) MotionStop(id string, axis string) error {
	return a.appCtx.MotionManager.Stop(a.ctx, id, core.AxisName(axis))
}

func (a *App) MotionEmergencyStop(id string) error {
	return a.appCtx.MotionManager.EmergencyStop(a.ctx, id)
}

func (a *App) MotionResetEmergencyStop(id string) error {
	return a.appCtx.MotionManager.ResetEmergencyStop(a.ctx, id)
}

func (a *App) MotionDefinePosition(id string, axis string, position float64) error {
	return a.appCtx.MotionManager.DefinePosition(a.ctx, id, core.AxisName(axis), position)
}
