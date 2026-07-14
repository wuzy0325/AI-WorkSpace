//go:build windows

package hardware

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

func wtnmc4aTestProfile() core.MotionControllerProfile {
	return core.MotionControllerProfile{
		ID:   "test",
		Name: "test",
		Type: core.ControllerTypeWTNMC4A,
		Axes: []core.AxisConfig{{
			Name:           core.AxisX,
			Enabled:        true,
			Kind:           core.AxisKindLinear,
			StepsPerRev:    core.PtrFloat64(1.8),
			MicroSteps:     core.PtrInt(4),
			Lead:           core.PtrFloat64(4),
			MaxSpeed:       core.PtrFloat64(10),
			PositionSource: core.PositionSourceRegister,
		}},
	}
}

func wtnmc4aTwoAxisTestProfile() core.MotionControllerProfile {
	profile := wtnmc4aTestProfile()
	yAxis := profile.Axes[0]
	yAxis.Name = core.AxisY
	profile.Axes = append(profile.Axes, yAxis)
	return profile
}

func TestWTNMC4APositionSampleRejectsImpossibleJump(t *testing.T) {
	axis := wtnmc4aTestProfile().Axes[0]
	previous := trustedPositionSample{pulse: 1000, at: time.Unix(10, 0)}

	err := validateWTNMC4APositionSample(axis, math.MaxInt32, previous, time.Unix(10, int64(100*time.Millisecond)))
	if err == nil {
		t.Fatal("expected impossible position jump to be rejected")
	}
}

func TestWTNMC4APositionSampleAllowsFullLogicalRegisterRange(t *testing.T) {
	axis := wtnmc4aTestProfile().Axes[0]
	for _, pulse := range []int32{-wtnmc4aMaxMovePulse, wtnmc4aMaxMovePulse, 300_000_000} {
		if err := validateWTNMC4APositionSample(axis, pulse, trustedPositionSample{}, time.Now()); err != nil {
			t.Fatalf("valid signed LONG position %d was rejected: %v", pulse, err)
		}
	}
}

func TestWTNMC4AStatusKeepsLastTrustedPositionAfterRepeatedBadReads(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true

	var reads atomic.Int32
	ctrl.readLP = func(uintptr, int) int32 {
		if reads.Add(1) <= 2 {
			return 1000
		}
		return math.MaxInt32
	}
	ctrl.readRR1 = func(uintptr, int) (rr1Status, error) { return rr1Status{}, nil }

	first, err := ctrl.Status(context.Background())
	if err != nil {
		t.Fatalf("first status failed: %v", err)
	}
	want := first.Axes[0].Position

	got, err := ctrl.Status(context.Background())
	if err == nil {
		t.Fatal("expected repeated bad reads to return an error")
	}
	if got.Axes[0].Position != want {
		t.Fatalf("bad sample replaced trusted position: got %v want %v", got.Axes[0].Position, want)
	}
}

func TestWTNMC4AInitialPositionRequiresConfirmation(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	values := []int32{268435455, 1000, 1000}
	var index atomic.Int32
	ctrl.readLP = func(uintptr, int) int32 {
		return values[index.Add(1)-1]
	}

	got, err := ctrl.readTrustedPosition(1, 0, ctrl.profile.Axes[0])
	if err != nil {
		t.Fatalf("confirmed initial position failed: %v", err)
	}
	want := wtnmc4aPulseToEngineering(ctrl.profile.Axes[0], 1000)
	if got != want {
		t.Fatalf("initial spike was accepted: got %v want %v", got, want)
	}
}

func TestWTNMC4AMaxMovePulseCanBecomeTrustedPosition(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.readLP = func(uintptr, int) int32 { return wtnmc4aMaxMovePulse }

	got, err := ctrl.readTrustedPosition(1, 0, ctrl.profile.Axes[0])
	if err != nil {
		t.Fatalf("valid repeated position was rejected: %v", err)
	}
	want := wtnmc4aPulseToEngineering(ctrl.profile.Axes[0], wtnmc4aMaxMovePulse)
	if got != want {
		t.Fatalf("trusted position = %v, want %v", got, want)
	}
}

func TestWTNMC4AMoveToDoesNotStartAfterBadPositionRead(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.speedParams[0] = &axisSpeedParams{DriveSpeed: 100, StartSpeed: 10, Acceleration: 500, Deceleration: 500, AccIncRate: 1000, DecIncRate: 1000, Multiple: 1}
	ctrl.trustedPositions[0] = trustedPositionSample{pulse: 1000, at: time.Now()}
	ctrl.readLP = func(uintptr, int) int32 { return math.MaxInt32 }

	var starts atomic.Int32
	ctrl.startMove = func(int, int32) error {
		starts.Add(1)
		return nil
	}

	err := ctrl.MoveTo(context.Background(), core.AxisX, 5)
	if err == nil {
		t.Fatal("expected MoveTo to reject an untrusted current position")
	}
	if starts.Load() != 0 {
		t.Fatalf("movement started %d times after bad position read", starts.Load())
	}
}

func TestWTNMC4AMoveToRejectsTargetOutsideSoftLimits(t *testing.T) {
	profile := wtnmc4aTestProfile()
	profile.Axes[0].MinLimit = core.PtrFloat64(-1)
	profile.Axes[0].MaxLimit = core.PtrFloat64(1)
	ctrl := NewWTNMC4AMotionController(profile)
	ctrl.handle = 1
	ctrl.status.Connected = true

	var reads atomic.Int32
	ctrl.readLP = func(uintptr, int) int32 {
		reads.Add(1)
		return 0
	}

	err := ctrl.MoveTo(context.Background(), core.AxisX, 2)
	if err == nil {
		t.Fatal("expected target outside soft limits to be rejected")
	}
	if reads.Load() != 0 {
		t.Fatalf("read current position %d times for an invalid target", reads.Load())
	}
}

func TestWTNMC4AMoveToRejectsNonFiniteTarget(t *testing.T) {
	axis := wtnmc4aTestProfile().Axes[0]
	for _, target := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := validateWTNMC4ATarget(axis, target); err == nil {
			t.Fatalf("expected target %v to be rejected", target)
		}
	}
}

func TestWTNMC4AMoveByRejectsResultOutsideSoftLimits(t *testing.T) {
	profile := wtnmc4aTestProfile()
	profile.Axes[0].MinLimit = core.PtrFloat64(-1)
	profile.Axes[0].MaxLimit = core.PtrFloat64(1)
	ctrl := NewWTNMC4AMotionController(profile)
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.trustedPositions[0] = trustedPositionSample{pulse: 0, at: time.Now()}
	ctrl.readLP = func(uintptr, int) int32 { return 0 }

	err := ctrl.MoveBy(context.Background(), core.AxisX, 2)
	if err == nil {
		t.Fatal("expected relative target outside soft limits to be rejected")
	}
}

func TestWTNMC4AMoveByUsesInjectedStartMove(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.trustedPositions[0] = trustedPositionSample{pulse: 0, at: time.Now()}
	ctrl.readLP = func(uintptr, int) int32 { return 0 }
	ctrl.startMove = func(int, int32) error { return fmt.Errorf("injected move failure") }

	err := ctrl.MoveBy(context.Background(), core.AxisX, 1)
	if err == nil {
		t.Fatal("expected injected move failure")
	}
	if ctrl.status.Axes[0].Moving {
		t.Fatal("failed relative move marked the axis as moving")
	}
}

func TestWTNMC4AMoveAxisInitRejectsPulseAboveHardwareLimit(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())

	if err := ctrl.moveAxisInit(0, wtnmc4aMaxMovePulse+1); err == nil {
		t.Fatal("expected pulse above hardware limit to be rejected")
	}
}

func TestWTNMC4AStatusSerializesDLLReads(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.trustedPositions[0] = trustedPositionSample{pulse: 0, at: time.Now()}

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	ctrl.readLP = func(uintptr, int) int32 {
		current := active.Add(1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return 0
	}
	ctrl.readRR1 = func(uintptr, int) (rr1Status, error) { return rr1Status{}, nil }

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ctrl.Status(context.Background())
			errs <- err
		}()
	}

	<-entered
	select {
	case <-entered:
		t.Fatal("second Status entered DLL while first call was blocked")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Status failed: %v", err)
		}
	}
	if maxActive.Load() != 1 {
		t.Fatalf("maximum concurrent DLL reads = %d, want 1", maxActive.Load())
	}
}

func TestWTNMC4AStatusPreservesCachedFlagsWhenRR1Fails(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.status.Axes[0].Moving = true
	ctrl.status.Axes[0].PosLimit = true
	ctrl.readLP = func(uintptr, int) int32 { return 0 }
	ctrl.readRR1 = func(uintptr, int) (rr1Status, error) {
		return rr1Status{}, fmt.Errorf("injected RR1 failure")
	}

	got, err := ctrl.Status(context.Background())
	if err == nil {
		t.Fatal("expected RR1 failure to be returned")
	}
	if !got.Axes[0].Moving || !got.Axes[0].PosLimit {
		t.Fatalf("RR1 failure replaced cached flags: %+v", got.Axes[0])
	}
	if !ctrl.lastFullStatusAt.IsZero() {
		t.Fatalf("failed RR1 read advanced full-status timestamp to %v", ctrl.lastFullStatusAt)
	}
}

func TestWTNMC4ADisconnectWaitsForInFlightIO(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.status.Connected = true

	ctrl.ioMu.Lock()
	done := make(chan struct{})
	go func() {
		_ = ctrl.Disconnect(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Disconnect completed while device I/O was still in flight")
	case <-time.After(50 * time.Millisecond):
	}
	ctrl.ioMu.Unlock()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Disconnect did not complete after device I/O finished")
	}
}

func TestWTNMC4AStopWaitsForInFlightIO(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.stopAxis = func(uintptr, int) error { return nil }

	ctrl.ioMu.Lock()
	done := make(chan error, 1)
	go func() {
		done <- ctrl.Stop(context.Background(), core.AxisX)
	}()
	select {
	case <-done:
		t.Fatal("Stop entered the DLL while another device call was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	ctrl.ioMu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not complete after device I/O finished")
	}
}

func TestWTNMC4AStopFailureDoesNotClaimAxisStopped(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.status.Axes[0].Moving = true
	ctrl.stopAxis = func(uintptr, int) error { return fmt.Errorf("injected stop failure") }

	err := ctrl.Stop(context.Background(), core.AxisX)
	if err == nil {
		t.Fatal("expected stop failure to be returned")
	}
	if !ctrl.status.Axes[0].Moving {
		t.Fatal("failed stop incorrectly marked the axis as stopped")
	}
}

func TestWTNMC4AStopAllUpdatesOnlySuccessfullyStoppedAxes(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTwoAxisTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.status.Axes[0].Moving = true
	ctrl.status.Axes[1].Moving = true
	ctrl.stopAxis = func(_ uintptr, axis int) error {
		if axis == 1 {
			return fmt.Errorf("injected stop failure")
		}
		return nil
	}

	err := ctrl.Stop(context.Background(), "")
	if err == nil {
		t.Fatal("expected partial stop failure")
	}
	if ctrl.status.Axes[0].Moving {
		t.Fatal("successfully stopped X axis remained marked moving")
	}
	if !ctrl.status.Axes[1].Moving {
		t.Fatal("failed Y-axis stop was incorrectly marked stopped")
	}
}

func TestWTNMC4AStopRejectsUnknownAxis(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	var calls atomic.Int32
	ctrl.stopAxis = func(uintptr, int) error {
		calls.Add(1)
		return nil
	}

	err := ctrl.Stop(context.Background(), core.AxisName("invalid"))
	if err == nil {
		t.Fatal("expected unknown axis to be rejected")
	}
	if calls.Load() != 0 {
		t.Fatalf("stop DLL called %d times for unknown axis", calls.Load())
	}
}

func TestWTNMC4AQueuedStatusDoesNotOverwriteStopState(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.status.Axes[0].Moving = true
	ctrl.trustedPositions[0] = trustedPositionSample{pulse: 0, at: time.Now()}

	readEntered := make(chan struct{})
	releaseRead := make(chan struct{})
	ctrl.readLP = func(uintptr, int) int32 {
		close(readEntered)
		<-releaseRead
		return 0
	}
	ctrl.readRR1 = func(uintptr, int) (rr1Status, error) {
		return rr1Status{CNST: true}, nil
	}
	ctrl.stopAxis = func(uintptr, int) error { return nil }

	statusDone := make(chan error, 1)
	go func() {
		_, err := ctrl.Status(context.Background())
		statusDone <- err
	}()
	<-readEntered
	stopDone := make(chan error, 1)
	go func() { stopDone <- ctrl.Stop(context.Background(), core.AxisX) }()
	close(releaseRead)
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if err := <-statusDone; err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if ctrl.status.Axes[0].Moving {
		t.Fatal("queued Status overwrote the newer stopped state")
	}
}

func TestWTNMC4AEmergencyStopFailureLatchesControllerAndTracksAxes(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTwoAxisTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.status.Axes[0].Moving = true
	ctrl.status.Axes[1].Moving = true
	ctrl.stopAxis = func(_ uintptr, axis int) error {
		if axis == 1 {
			return fmt.Errorf("injected stop failure")
		}
		return nil
	}

	err := ctrl.EmergencyStop(context.Background())
	if err == nil {
		t.Fatal("expected emergency-stop failure to be returned")
	}
	if !ctrl.status.EmergencyStopped {
		t.Fatal("partial emergency-stop failure did not latch the controller")
	}
	if ctrl.status.Axes[0].Moving {
		t.Fatal("successfully stopped X axis remained marked moving")
	}
	if !ctrl.status.Axes[1].Moving {
		t.Fatal("failed Y-axis emergency stop was incorrectly marked stopped")
	}
	if err := ctrl.MoveBy(context.Background(), core.AxisX, 1); err == nil {
		t.Fatal("latched emergency stop allowed a subsequent move")
	}
}
