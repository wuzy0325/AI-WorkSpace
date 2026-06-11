//go:build windows

package hardware

import (
	"context"
	"fmt"
	"math"
	"sync"
	"syscall"
	"unsafe"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

const (
	// WTNMC4A default port
	defaultPort = 5000
	// WTNMC4A speed range (pulse/s)
	speedMin = 1
	speedMax = 8000
	// WTNMC4A default timeout (ms)
	defaultSendTO = 200
	defaultRecvTO = 200
	// Default motion parameters
	defaultStartSpeed  = 100
	defaultAcceleration = 1000
	defaultAccIncRate   = 10000
)

// WTNMC4A drive mode constants
const (
	dv = 0 // fixed-length drive
	lv = 1 // continuous drive
)

// WTNMC4A deceleration mode constants
const (
	autoDec = 0 // auto deceleration
)

// WTNMC4A pulse mode constants
const (
	cpDir = 1 // CP/DIR mode
)

// WTNMC4A motion mode constants
const (
	line = 0 // linear
)

// WTNMC4A motion direction constants
const (
	mDirection = 0 // negative direction
	pDirection = 1 // positive direction
)

// paraDataList maps to C struct WTNMC4A_PARA_DataList.
// Common parameters used by InitLVDV.
type paraDataList struct {
	Multiple     int64 // multiplier ratio (1~500)
	StartSpeed   int64 // start speed (1~8000)
	DriveSpeed   int64 // drive speed (1~8000)
	Acceleration int64 // acceleration (125~1000000)
	Deceleration int64 // deceleration (125~1000000)
	AccIncRate   int64 // accel change rate (954~62500000)
	DecIncRate   int64 // decel change rate (954~62500000)
}

// paraLCData maps to C struct WTNMC4A_PARA_LCData.
// Linear/S-curve parameters used by InitLVDV.
type paraLCData struct {
	AxisNum     int64 // axis number
	LVDV        int64 // drive mode (0=fixed-length, 1=continuous)
	DecMode     int64 // deceleration mode (0=auto, 1=manual)
	PulseMode   int64 // pulse mode (0=CW/CCW, 1=CP/DIR)
	PLSLogLever int64 // pulse direction (0=negative, 1=positive)
	DIRLogLever int64 // direction signal logic level
	LineCurve   int64 // motion mode (0=linear, 1=S-curve)
	Direction   int64 // motion direction (0=negative, 1=positive)
	NPulseNum   int64 // fixed output pulse count (0~268435455)
}

// dllProcs caches DLL function pointers to avoid repeated lookups.
type dllProcs struct {
	devCreateA *syscall.Proc
	devRelease *syscall.Proc
	reset      *syscall.Proc
	setSV      *syscall.Proc
	setV       *syscall.Proc
	setA       *syscall.Proc
	setDec     *syscall.Proc
	setP       *syscall.Proc
	setLP      *syscall.Proc
	setEP      *syscall.Proc
	initLVDV   *syscall.Proc
	startLVDV  *syscall.Proc
	decStop    *syscall.Proc
	instStop   *syscall.Proc
	readLP     *syscall.Proc
	readEP     *syscall.Proc
	readCV     *syscall.Proc
	readCA     *syscall.Proc
	getRR1     *syscall.Proc
	startHome  *syscall.Proc
	clearLimit *syscall.Proc
	setPDirLim *syscall.Proc
	setMDirLim *syscall.Proc
}

// rr1Status RR1 status register parse result
type rr1Status struct {
	CMPP, CMPM, ASND, CNST, DSND bool
	IN0, IN1, IN2, IN3           bool
	LMTP, LMTM, ALARM, EMG       bool
}

// WTNMC4AMotionController implements a 4-axis motion controller via the WTNMC4A official DLL.
// It uses syscall to call WTNMC4A.dll, ensuring protocol correctness.
// Each controller instance has an independent device handle and is concurrency-safe.
type WTNMC4AMotionController struct {
	mu      sync.Mutex
	profile core.MotionControllerProfile
	status  core.ControllerStatus
	handle  uintptr      // device handle
	dll     *syscall.DLL // DLL instance
	procs   dllProcs     // DLL function pointers
}

// NewWTNMC4AMotionController creates a WTNMC4A motion controller adapter.
func NewWTNMC4AMotionController(profile core.MotionControllerProfile) *WTNMC4AMotionController {
	// WTNMC4A defaults to port 5000
	if profile.Port == 0 {
		profile.Port = defaultPort
	}

	status := core.ControllerStatus{
		ID:               profile.ID,
		Name:             profile.Name,
		Type:             profile.Type,
		Connected:        false,
		EmergencyStopped: false,
		Axes:             make([]core.AxisStatus, 0),
	}
	for _, axisCfg := range profile.Axes {
		if axisCfg.Enabled {
			status.Axes = append(status.Axes, core.AxisStatus{Name: axisCfg.Name})
		}
	}

	return &WTNMC4AMotionController{
		profile: profile,
		status:  status,
	}
}

// ApplyConfig applies a new controller profile without disconnecting.
// Updates the profile and re-applies axis speed parameters to the hardware.
func (c *WTNMC4AMotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profile = profile
	if c.status.Connected && c.handle != 0 {
		c.applyAxisSpeedsLocked()
	}
	return nil
}

// GetProfile returns the controller profile.
func (c *WTNMC4AMotionController) GetProfile() core.MotionControllerProfile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.profile
}

// Connect loads the DLL, creates a device handle, and applies axis speed parameters.
func (c *WTNMC4AMotionController) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status.Connected && c.handle != 0 {
		return nil
	}

	// load DLL
	dllPath := wtnmc4aFindDLL()
	dll, err := syscall.LoadDLL(dllPath)
	if err != nil {
		c.status.LastError = fmt.Sprintf("加载WTNMC4A DLL失败: %v", err)
		return fmt.Errorf("加载WTNMC4A DLL %s 失败: %w", dllPath, err)
	}
	c.dll = dll

	// lookup all function pointers
	c.procs = dllProcs{
		devCreateA: dll.MustFindProc("WTNMC4A_DEV_CreateA"),
		devRelease: dll.MustFindProc("WTNMC4A_DEV_Release"),
		reset:      dll.MustFindProc("WTNMC4A_Reset"),
		setSV:      dll.MustFindProc("WTNMC4A_SetSV"),
		setV:       dll.MustFindProc("WTNMC4A_SetV"),
		setA:       dll.MustFindProc("WTNMC4A_SetA"),
		setDec:     dll.MustFindProc("WTNMC4A_SetDec"),
		setP:       dll.MustFindProc("WTNMC4A_SetP"),
		setLP:      dll.MustFindProc("WTNMC4A_SetLP"),
		setEP:      dll.MustFindProc("WTNMC4A_SetEP"),
		initLVDV:   dll.MustFindProc("WTNMC4A_InitLVDV"),
		startLVDV:  dll.MustFindProc("WTNMC4A_StartLVDV"),
		decStop:    dll.MustFindProc("WTNMC4A_DecStop"),
		instStop:   dll.MustFindProc("WTNMC4A_InstStop"),
		readLP:     dll.MustFindProc("WTNMC4A_ReadLP"),
		readEP:     dll.MustFindProc("WTNMC4A_ReadEP"),
		readCV:     dll.MustFindProc("WTNMC4A_ReadCV"),
		readCA:     dll.MustFindProc("WTNMC4A_ReadCA"),
		getRR1:     dll.MustFindProc("WTNMC4A_GetRR1Status"),
		startHome:  dll.MustFindProc("WTNMC4A_StartAutoHomeSearch"),
		clearLimit: dll.MustFindProc("WTNMC4A_ClearSoftwareLimit"),
		setPDirLim: dll.MustFindProc("WTNMC4A_SetPDirSoftwareLimit"),
		setMDirLim: dll.MustFindProc("WTNMC4A_SetMDirSoftwareLimit"),
	}

	// create device handle: WTNMC4A_DEV_CreateA(IP, sendTimeout, recvTimeout)
	// [FIX] check BytePtrFromString error to avoid passing nil pointer to DLL
	ipPtr, err := syscall.BytePtrFromString(c.profile.Address)
	if err != nil {
		c.dll.Release()
		c.dll = nil
		c.status.LastError = fmt.Sprintf("无效的IP地址: %v", err)
		return fmt.Errorf("WTNMC4A IP地址无效 %q: %w", c.profile.Address, err)
	}
	ret, _, _ := c.procs.devCreateA.Call(
		uintptr(unsafe.Pointer(ipPtr)),
		uintptr(defaultSendTO),
		uintptr(defaultRecvTO),
	)
	if ret == 0 {
		c.dll.Release()
		c.dll = nil
		c.status.LastError = fmt.Sprintf("创建设备句柄失败: %s:%d", c.profile.Address, c.profile.Port)
		return fmt.Errorf("WTNMC4A 连接 %s:%d 失败: DEV_CreateA 返回空句柄", c.profile.Address, c.profile.Port)
	}
	c.handle = ret

	// apply axis speed parameters
	c.applyAxisSpeedsLocked()

	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	return nil
}

// Disconnect releases the device handle and DLL instance, closes connection.
func (c *WTNMC4AMotionController) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handle != 0 {
		c.procs.devRelease.Call(c.handle)
		c.handle = 0
	}
	// [FIX] release DLL instance to prevent handle leak
	if c.dll != nil {
		c.dll.Release()
		c.dll = nil
	}

	c.status.Connected = false
	c.status.EmergencyStopped = false
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
		c.status.Axes[i].Compensating = false
	}
	return nil
}

// Status reads logical position and RR1 status register, updates all axes.
func (c *WTNMC4AMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return c.copyStatusLocked(), err
	}

	for i := range c.status.Axes {
		axisStatus := &c.status.Axes[i]
		axisCfg, ok := c.axisConfigLocked(axisStatus.Name)
		if !ok {
			continue
		}
		axisNum := wtnmc4aAxisNum(axisStatus.Name)

		// read logical position
		lpRet, _, _ := c.procs.readLP.Call(c.handle, uintptr(axisNum))
		logicalPos := int64(lpRet)
		position := wtnmc4aPulseToEngineering(axisCfg, float64(logicalPos))

		// read RR1 status register
		rr1 := c.getRR1StatusLocked(axisNum)

		// determine motion state: accelerating/constant-speed/decelerating
		moving := rr1.ASND || rr1.CNST || rr1.DSND

		// check if near home position
		homed := core.IsHomed(position, axisCfg)

		axisStatus.Position = position
		axisStatus.Moving = moving
		axisStatus.Homed = homed
		axisStatus.PosLimit = rr1.LMTP
		axisStatus.NegLimit = rr1.LMTM
		axisStatus.Compensating = false
		axisStatus.CompensationError = ""
		axisStatus.PositionError = 0
	}
	c.status.LastError = ""
	return c.copyStatusLocked(), nil
}

// MoveTo does absolute positioning to the given position (engineering units).
func (c *WTNMC4AMotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	an := wtnmc4aAxisNum(axis)

	// read current position (pulses)
	lpRet, _, _ := c.procs.readLP.Call(c.handle, uintptr(an))
	currentPulse := int64(lpRet)

	// calculate target position (pulses)
	targetPulse := wtnmc4aEngineeringToPulse(axisCfg, position)
	deltaPulse := targetPulse - currentPulse
	if deltaPulse == 0 {
		return nil
	}

	// initialize and start fixed-length drive
	if err := c.initAndStartDriveLocked(axisCfg, an, deltaPulse, dv); err != nil {
		c.status.LastError = err.Error()
		return err
	}

	c.status.LastError = ""
	return nil
}

// MoveBy does relative movement by the given delta (engineering units).
func (c *WTNMC4AMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	an := wtnmc4aAxisNum(axis)

	// read current position
	lpRet, _, _ := c.procs.readLP.Call(c.handle, uintptr(an))
	currentPulse := int64(lpRet)
	currentPos := wtnmc4aPulseToEngineering(axisCfg, float64(currentPulse))

	// calculate target position
	targetPulse := wtnmc4aEngineeringToPulse(axisCfg, currentPos+delta)
	deltaPulse := targetPulse - currentPulse
	if deltaPulse == 0 {
		return nil
	}

	if err := c.initAndStartDriveLocked(axisCfg, an, deltaPulse, dv); err != nil {
		c.status.LastError = err.Error()
		return err
	}

	c.status.LastError = ""
	return nil
}

// Jog performs continuous motion (jog).
// velocity > 0 for positive direction, velocity < 0 for negative direction.
func (c *WTNMC4AMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	an := wtnmc4aAxisNum(axis)

	maxSpeed := core.ValueOrFloat(axisCfg.MaxSpeed, 100)
	jogSpeed := math.Abs(velocity)
	if jogSpeed > maxSpeed {
		jogSpeed = maxSpeed
	}
	if jogSpeed == 0 {
		jogSpeed = maxSpeed
	}

	ppu := core.PulsesPerUnit(axisCfg)
	driveSpeedPulse := int64(math.Round(jogSpeed * math.Abs(ppu)))
	driveSpeedPulse = core.ClampInt64(driveSpeedPulse, speedMin, speedMax)

	dataList := paraDataList{
		Multiple:     1,
		StartSpeed:   defaultStartSpeed,
		DriveSpeed:   driveSpeedPulse,
		Acceleration: defaultAcceleration,
		Deceleration: defaultAcceleration,
		AccIncRate:   defaultAccIncRate,
		DecIncRate:   defaultAccIncRate,
	}

	// direction determined by velocity sign
	direction := int64(pDirection)
	if velocity < 0 {
		direction = mDirection
	}

	lcData := paraLCData{
		AxisNum:     int64(an),
		LVDV:        lv, // continuous drive
		DecMode:     autoDec,
		PulseMode:   cpDir,
		PLSLogLever: direction,
		DIRLogLever: 0,
		LineCurve:   line,
		Direction:   direction,
		NPulseNum:   0, // continuous drive: pulse count is 0
	}

	ret, _, _ := c.procs.initLVDV.Call(
		c.handle,
		uintptr(unsafe.Pointer(&dataList)),
		uintptr(unsafe.Pointer(&lcData)),
	)
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 初始化轴 %s 点动失败", axis)
	}

	ret, _, _ = c.procs.startLVDV.Call(c.handle, uintptr(an))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 启动轴 %s 点动失败", axis)
	}

	c.status.LastError = ""
	return nil
}

// Home starts automatic home search.
func (c *WTNMC4AMotionController) Home(ctx context.Context, axis core.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	an := wtnmc4aAxisNum(axis)
	ret, _, _ := c.procs.startHome.Call(c.handle, uintptr(an))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 启动轴 %s 回零失败", axis)
	}

	c.status.LastError = ""
	return nil
}

// Stop decelerates and stops the specified axis (empty string stops all axes).
func (c *WTNMC4AMotionController) Stop(ctx context.Context, axis core.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	if axis == "" {
		for _, an := range []int{0, 1, 2, 3} {
			c.procs.decStop.Call(c.handle, uintptr(an))
		}
		c.status.LastError = ""
		return nil
	}

	if _, ok := c.axisConfigLocked(axis); !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}

	an := wtnmc4aAxisNum(axis)
	ret, _, _ := c.procs.decStop.Call(c.handle, uintptr(an))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 停止轴 %s 失败", axis)
	}

	c.status.LastError = ""
	return nil
}

// EmergencyStop immediately stops all axes.
func (c *WTNMC4AMotionController) EmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	for _, an := range []int{0, 1, 2, 3} {
		c.procs.instStop.Call(c.handle, uintptr(an))
	}

	c.status.EmergencyStopped = true
	c.status.LastError = ""
	return nil
}

// ResetEmergencyStop resets the emergency stop state.
func (c *WTNMC4AMotionController) ResetEmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	ret, _, _ := c.procs.reset.Call(c.handle)
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 复位设备失败")
	}

	c.status.EmergencyStopped = false
	c.status.LastError = ""
	return nil
}

// DefinePosition sets the logical position and encoder counter.
func (c *WTNMC4AMotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}

	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	an := wtnmc4aAxisNum(axis)

	pulse := wtnmc4aEngineeringToPulse(axisCfg, position)

	// [FIX] pass signed integer via unsafe.Pointer to avoid negative-to-uintptr overflow
	ret, _, _ := c.procs.setLP.Call(c.handle, uintptr(an), uintptr(unsafe.Pointer(&pulse)))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 设置轴 %s 逻辑位置失败", axis)
	}

	// set encoder counter
	encoderCount := core.EngineeringToEncoderCount(axisCfg, position)
	ret, _, _ = c.procs.setEP.Call(c.handle, uintptr(an), uintptr(unsafe.Pointer(&encoderCount)))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 设置轴 %s 实位失败", axis)
	}

	c.status.LastError = ""
	return nil
}

// ---- internal helper methods ----

// checkConnectedLocked checks if the controller is connected (must hold lock).
func (c *WTNMC4AMotionController) checkConnectedLocked() error {
	if c.handle == 0 || !c.status.Connected {
		return fmt.Errorf("控制器未连接")
	}
	return nil
}

// axisConfigLocked looks up axis config by name (must hold lock).
func (c *WTNMC4AMotionController) axisConfigLocked(axis core.AxisName) (core.AxisConfig, bool) {
	for _, axisCfg := range c.profile.Axes {
		if axisCfg.Enabled && axisCfg.Name == axis {
			return axisCfg, true
		}
	}
	return core.AxisConfig{}, false
}

// copyStatusLocked copies the current status (must hold lock).
func (c *WTNMC4AMotionController) copyStatusLocked() core.ControllerStatus {
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	return status
}

// applyAxisSpeedsLocked applies axis speed parameters after connecting (must hold lock).
func (c *WTNMC4AMotionController) applyAxisSpeedsLocked() {
	for _, axisCfg := range c.profile.Axes {
		if !axisCfg.Enabled {
			continue
		}
		an := wtnmc4aAxisNum(axisCfg.Name)
		if axisCfg.MaxSpeed != nil && *axisCfg.MaxSpeed > 0 {
			ppu := core.PulsesPerUnit(axisCfg)
			pulseSpeed := int64(math.Round(*axisCfg.MaxSpeed * math.Abs(ppu)))
			pulseSpeed = core.ClampInt64(pulseSpeed, speedMin, speedMax)
			c.procs.setV.Call(c.handle, uintptr(an), uintptr(pulseSpeed))
		}
	}
}

// getRR1StatusLocked reads the RR1 status register (must hold lock).
func (c *WTNMC4AMotionController) getRR1StatusLocked(axisNum int) rr1Status {
	var buf [4]byte
	c.procs.getRR1.Call(c.handle, uintptr(axisNum), uintptr(unsafe.Pointer(&buf[0])))
	status := uint16(buf[0]) | uint16(buf[1])<<8
	return rr1Status{
		CMPP: (status&(1<<0)) != 0, CMPM: (status&(1<<1)) != 0,
		ASND: (status&(1<<2)) != 0, CNST: (status&(1<<3)) != 0,
		DSND: (status&(1<<4)) != 0, IN0: (status&(1<<5)) != 0,
		IN1: (status&(1<<6)) != 0, IN2: (status&(1<<7)) != 0,
		IN3: (status&(1<<8)) != 0, LMTP: (status&(1<<9)) != 0,
		LMTM: (status&(1<<10)) != 0, ALARM: (status&(1<<11)) != 0,
		EMG: (status&(1<<12)) != 0,
	}
}

// initAndStartDriveLocked initializes and starts the drive (must hold lock).
func (c *WTNMC4AMotionController) initAndStartDriveLocked(axisCfg core.AxisConfig, an int, deltaPulse int64, driveMode int64) error {
	maxSpeed := core.ValueOrFloat(axisCfg.MaxSpeed, 100)
	ppu := core.PulsesPerUnit(axisCfg)
	driveSpeedPulse := int64(math.Round(maxSpeed * math.Abs(ppu)))
	driveSpeedPulse = core.ClampInt64(driveSpeedPulse, speedMin, speedMax)

	dataList := paraDataList{
		Multiple:     1,
		StartSpeed:   defaultStartSpeed,
		DriveSpeed:   driveSpeedPulse,
		Acceleration: defaultAcceleration,
		Deceleration: defaultAcceleration,
		AccIncRate:   defaultAccIncRate,
		DecIncRate:   defaultAccIncRate,
	}

	direction := int64(pDirection)
	if deltaPulse < 0 {
		direction = mDirection
	}

	lcData := paraLCData{
		AxisNum:     int64(an),
		LVDV:        driveMode,
		DecMode:     autoDec,
		PulseMode:   cpDir,
		PLSLogLever: direction,
		DIRLogLever: 0,
		LineCurve:   line,
		Direction:   direction,
		NPulseNum:   int64(math.Abs(float64(deltaPulse))),
	}

	ret, _, _ := c.procs.initLVDV.Call(
		c.handle,
		uintptr(unsafe.Pointer(&dataList)),
		uintptr(unsafe.Pointer(&lcData)),
	)
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 初始化轴驱动失败")
	}

	ret, _, _ = c.procs.startLVDV.Call(c.handle, uintptr(an))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 启动轴驱动失败")
	}

	return nil
}

// ---- conversion functions ----

// wtnmc4aAxisNum converts axis name to WTNMC4A axis number.
func wtnmc4aAxisNum(axis core.AxisName) int {
	switch axis {
	case core.AxisX:
		return 0
	case core.AxisY:
		return 1
	case core.AxisZ:
		return 2
	case core.AxisU:
		return 3
	default:
		return 0
	}
}

// wtnmc4aEngineeringToPulse converts engineering units to pulses.
// Inverted handling is applied here (B140 handles inversion via MT/CE commands).
func wtnmc4aEngineeringToPulse(axisCfg core.AxisConfig, value float64) int64 {
	pulse := core.EngineeringToPulse(axisCfg, value)
	if axisCfg.Inverted {
		pulse = -pulse
	}
	return pulse
}

// wtnmc4aPulseToEngineering converts pulses to engineering units.
func wtnmc4aPulseToEngineering(axisCfg core.AxisConfig, pulse float64) float64 {
	signedPulse := pulse
	if axisCfg.Inverted {
		signedPulse = -signedPulse
	}
	return core.PulseToEngineering(axisCfg, signedPulse)
}

// wtnmc4aFindDLL finds the WTNMC4A DLL path.
func wtnmc4aFindDLL() string {
	return "WTNMC4A_64.dll"
}

// Ensure interface compliance.
var _ ports.MotionController = (*WTNMC4AMotionController)(nil)