//go:build windows

package hardware

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"syscall"
	"unsafe"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

const (
	defaultPort = 5000
	speedMin    = 1
	speedMax    = 8000

	defaultSendTO      = 200
	defaultRecvTO      = 200
	defaultStartSpeed  = 500
	defaultAcceleration = 500
	defaultAccIncRate  = 1000
	defaultDecIncRate  = 1000

	lvdvPulse = 0
	lvdvCont  = 1
	autoDec   = 0
	cpDir     = 0
	line      = 0
	pDirection int32 = 1
	mDirection int32 = 0
)

type rr1Status struct {
	CMPP, CMPM, ASND, CNST, DSND bool
	IN0, IN1, IN2, IN3           bool
	LMTP, LMTM, ALARM, EMG       bool
}

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

// C struct WTNMC4A_PARA_DataList 映射
// 所有字段必须使用 int32（对应 C LONG 4字节）
type paraDataList struct {
	Multiple     int32
	StartSpeed   int32
	DriveSpeed   int32
	Acceleration int32
	Deceleration int32
	AccIncRate   int32
	DecIncRate   int32
}

// C struct WTNMC4A_PARA_LCData 映射
// 所有字段必须使用 int32（对应 C LONG 4字节）
type paraLCData struct {
	AxisNum     int32
	LVDV        int32
	DecMode     int32
	PulseMode   int32
	PLSLogLever int32
	DIRLogLever int32
	LineCurve   int32
	Direction   int32
	NPulseNum   int32
}

// axisSpeedParams 缓存每个轴的完整 LVDV 参数
type axisSpeedParams struct {
	DriveSpeed   int32
	StartSpeed   int32
	Acceleration int32
	Deceleration int32
	AccIncRate   int32
	DecIncRate   int32
	Multiple     int32
}

type WTNMC4AMotionController struct {
	mu          sync.RWMutex
	profile     core.MotionControllerProfile
	status      core.ControllerStatus
	handle      uintptr
	dll         *syscall.DLL
	procs       dllProcs
	speedParams map[int]*axisSpeedParams
}

func (c *WTNMC4AMotionController) GetProfile() core.MotionControllerProfile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.profile
}

func NewWTNMC4AMotionController(profile core.MotionControllerProfile) *WTNMC4AMotionController {
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
		profile:     profile,
		status:      status,
		speedParams: make(map[int]*axisSpeedParams),
	}
}

func (c *WTNMC4AMotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	c.mu.Lock()
	c.profile = profile
	needReconfig := c.handle != 0 && c.status.Connected
	c.mu.Unlock()
	if needReconfig {
		c.mu.Lock()
		c.cacheAxisSpeedsLocked()
		c.mu.Unlock()
	}
	return nil
}

func (c *WTNMC4AMotionController) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status.Connected && c.handle != 0 {
		return nil
	}

	dllPath := wtnmc4aFindDLL()
	dll, err := syscall.LoadDLL(dllPath)
	if err != nil {
		c.status.LastError = fmt.Sprintf("加载WTNMC4A DLL失败: %v", err)
		return fmt.Errorf("加载WTNMC4A DLL %s 失败: %w", dllPath, err)
	}
	c.dll = dll

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

	c.speedParams = make(map[int]*axisSpeedParams)
	c.cacheAxisSpeedsLocked()

	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	return nil
}

func (c *WTNMC4AMotionController) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handle != 0 {
		c.procs.devRelease.Call(c.handle)
		c.handle = 0
	}
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

func (c *WTNMC4AMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

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

		lpRet, _, _ := c.procs.readLP.Call(c.handle, uintptr(axisNum))
		logicalPos := int64(int32(lpRet))
		position := wtnmc4aPulseToEngineering(axisCfg, float64(logicalPos))

		rr1 := c.getRR1StatusLocked(axisNum)
		moving := rr1.ASND || rr1.CNST || rr1.DSND
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

// InitLVDV + StartLVDV 启动轴的运动（参照标准SDK示例用法）。
// InitLVDV 一次性设置所有运动参数（速度、加速度、方向、脉冲数等），
// 避免因硬件清除寄存器导致的运动失败。
func (c *WTNMC4AMotionController) moveAxisInit(an int, targetPulse int32) error {
	params, ok := c.speedParams[an]
	if !ok || params.DriveSpeed == 0 {
		return fmt.Errorf("轴%d未配置速度参数", an)
	}

	// WTNMC4A_SetP 仅支持正脉冲数(0-268435455)，方向由 Direction 控制
	direction := pDirection
	absPulse := targetPulse
	if targetPulse < 0 {
		direction = mDirection
		absPulse = -targetPulse
	}

	if absPulse > 268435455 {
		slog.Warn("WTNMC4A moveAxisInit pulse clipped to hardware max",
			"axis", an, "original", targetPulse, "clipped", int32(268435455))
		absPulse = 268435455
	}

	slog.Debug("WTNMC4A moveAxisInit",
		"axis", an, "pulse", absPulse, "direction", direction,
		"speed", params.DriveSpeed, "accel", params.Acceleration)

	dataList := paraDataList{
		Multiple:     params.Multiple,
		StartSpeed:   params.StartSpeed,
		DriveSpeed:   params.DriveSpeed,
		Acceleration: params.Acceleration,
		Deceleration: params.Deceleration,
		AccIncRate:   params.AccIncRate,
		DecIncRate:   params.DecIncRate,
	}

	lcData := paraLCData{
		AxisNum:     int32(an),
		LVDV:        lvdvPulse,
		DecMode:     autoDec,
		PulseMode:   cpDir,
		PLSLogLever: 0,
		DIRLogLever: 0,
		LineCurve:   line,
		Direction:   direction,
		NPulseNum:   absPulse,
	}

	ret, _, _ := c.procs.initLVDV.Call(
		c.handle,
		uintptr(unsafe.Pointer(&dataList)),
		uintptr(unsafe.Pointer(&lcData)),
	)
	if ret == 0 {
		slog.Error("WTNMC4A InitLVDV failed", "axis", an)
		return fmt.Errorf("InitLVDV failed")
	}
	slog.Debug("WTNMC4A InitLVDV success", "axis", an)

	ret, _, _ = c.procs.startLVDV.Call(c.handle, uintptr(an))
	if ret == 0 {
		slog.Error("WTNMC4A StartLVDV failed", "axis", an)
		return fmt.Errorf("StartLVDV failed")
	}
	slog.Debug("WTNMC4A StartLVDV success", "axis", an)
	return nil
}

// MoveTo 绝对定位：参照标准SDK 单轴直线S曲线驱动 示例，
// 使用 InitLVDV + StartLVDV 实现。
func (c *WTNMC4AMotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkReadyLocked(); err != nil {
		return err
	}
	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	an := wtnmc4aAxisNum(axis)

	currentLp, _, _ := c.procs.readLP.Call(c.handle, uintptr(an))
	currentPos := wtnmc4aPulseToEngineering(axisCfg, float64(int64(int32(currentLp))))
	deltaPulse := wtnmc4aEngineeringToPulse(axisCfg, position) - wtnmc4aEngineeringToPulse(axisCfg, currentPos)
	if deltaPulse == 0 {
		return nil
	}
	if deltaPulse > 268435455 || deltaPulse < -268435455 {
		return fmt.Errorf("WTNMC4A 轴 %s 运动距离超出硬件范围: %d", axis, deltaPulse)
	}

	if err := c.moveAxisInit(an, int32(deltaPulse)); err != nil {
		c.status.LastError = fmt.Sprintf("轴 %s 运动失败: %v", axis, err)
		return fmt.Errorf("WTNMC4A 轴 %s 运动失败: %w", axis, err)
	}
	c.status.LastError = ""
	return nil
}

// MoveBy 相对定位：使用 InitLVDV + StartLVDV 实现。delta 为正则正方向，为负则反方向。
func (c *WTNMC4AMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkReadyLocked(); err != nil {
		return err
	}
	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	an := wtnmc4aAxisNum(axis)

	deltaPulse := wtnmc4aEngineeringToPulse(axisCfg, delta)
	if deltaPulse == 0 {
		return nil
	}
	if deltaPulse > 268435455 || deltaPulse < -268435455 {
		return fmt.Errorf("WTNMC4A 轴 %s 运动距离超出硬件范围: %d", axis, deltaPulse)
	}

	if err := c.moveAxisInit(an, int32(deltaPulse)); err != nil {
		c.status.LastError = fmt.Sprintf("轴 %s 运动失败: %v", axis, err)
		return fmt.Errorf("WTNMC4A 轴 %s 运动失败: %w", axis, err)
	}
	c.status.LastError = ""
	return nil
}

// Jog 连续运动：参照标准SDK用法，使用 SetV + StartLVDV（连续模式 LVDV=1）
func (c *WTNMC4AMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkReadyLocked(); err != nil {
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
	pulseSpeed := int64(math.Round(jogSpeed * math.Abs(ppu)))
	pulseSpeed = core.ClampInt64(pulseSpeed, speedMin, speedMax)

	// 方向由速度的正负号决定
	direction := pDirection
	if velocity < 0 {
		direction = mDirection
	}

	// 点动使用 InitLVDV 连续模式 (LVDV=lvdvCont=1) + StartLVDV
	params, ok := c.speedParams[an]
	if !ok {
		return fmt.Errorf("轴%d未配置速度参数", an)
	}

	dataList := paraDataList{
		Multiple:     params.Multiple,
		StartSpeed:   params.StartSpeed,
		DriveSpeed:   int32(pulseSpeed),
		Acceleration: params.Acceleration,
		Deceleration: params.Deceleration,
		AccIncRate:   params.AccIncRate,
		DecIncRate:   params.DecIncRate,
	}

	lcData := paraLCData{
		AxisNum:     int32(an),
		LVDV:        lvdvCont,
		DecMode:     autoDec,
		PulseMode:   cpDir,
		PLSLogLever: 0,
		DIRLogLever: 0,
		LineCurve:   line,
		Direction:   direction,
		NPulseNum:   0,
	}

	ret, _, _ := c.procs.initLVDV.Call(
		c.handle,
		uintptr(unsafe.Pointer(&dataList)),
		uintptr(unsafe.Pointer(&lcData)),
	)
	if ret == 0 {
		c.status.LastError = fmt.Sprintf("初始化轴 %s 点动失败", axis)
		return fmt.Errorf("WTNMC4A 初始化轴 %s 点动失败", axis)
	}

	ret, _, _ = c.procs.startLVDV.Call(c.handle, uintptr(an))
	if ret == 0 {
		c.status.LastError = fmt.Sprintf("启动轴 %s 点动失败", axis)
		return fmt.Errorf("WTNMC4A 启动轴 %s 点动失败", axis)
	}
	c.status.LastError = ""
	return nil
}

func (c *WTNMC4AMotionController) Home(ctx context.Context, axis core.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkReadyLocked(); err != nil {
		return err
	}
	if _, ok := c.axisConfigLocked(axis); !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}

	an := wtnmc4aAxisNum(axis)
	ret, _, _ := c.procs.startHome.Call(c.handle, uintptr(an))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 启动轴 %s 回零失败", axis)
	}
	c.status.LastError = ""
	return nil
}

func (c *WTNMC4AMotionController) Stop(ctx context.Context, axis core.AxisName) error {
	// 最优先级：只在 RLock 下读取 handle（纳秒级），释放锁后再调用 DLL。
	// 这样 Stop 永远不会阻塞在 MoveBy/MoveTo 等写锁操作上。
	var handle uintptr
	c.mu.RLock()
	handle = c.handle
	connected := c.status.Connected
	c.mu.RUnlock()

	if handle == 0 || !connected {
		return fmt.Errorf("控制器未连接")
	}

	if axis == "" {
		for _, an := range []int{0, 1, 2, 3} {
			c.procs.decStop.Call(handle, uintptr(an))
		}
		return nil
	}
	an := wtnmc4aAxisNum(axis)
	c.procs.decStop.Call(handle, uintptr(an))
	return nil
}

func (c *WTNMC4AMotionController) EmergencyStop(ctx context.Context) error {
	// 最优先级：只在 RLock 下读取 handle（纳秒级），释放锁后再调用 DLL
	var handle uintptr
	c.mu.RLock()
	handle = c.handle
	connected := c.status.Connected
	c.mu.RUnlock()

	if handle == 0 || !connected {
		return fmt.Errorf("控制器未连接")
	}

	for _, an := range []int{0, 1, 2, 3} {
		c.procs.instStop.Call(handle, uintptr(an))
	}
	c.mu.Lock()
	c.status.EmergencyStopped = true
	c.status.LastError = ""
	c.mu.Unlock()
	return nil
}

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
	ret, _, _ := c.procs.setLP.Call(c.handle, uintptr(an), uintptr(int64(pulse)))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 设置轴 %s 逻辑位置失败", axis)
	}

	encoderCount := core.EngineeringToEncoderCount(axisCfg, position)
	ret, _, _ = c.procs.setEP.Call(c.handle, uintptr(an), uintptr(int64(encoderCount)))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 设置轴 %s 实位失败", axis)
	}
	c.status.LastError = ""
	return nil
}

func (c *WTNMC4AMotionController) checkConnectedLocked() error {
	if c.handle == 0 || !c.status.Connected {
		return fmt.Errorf("控制器未连接")
	}
	return nil
}

func (c *WTNMC4AMotionController) checkReadyLocked() error {
	if err := c.checkConnectedLocked(); err != nil {
		return err
	}
	if c.status.EmergencyStopped {
		return fmt.Errorf("控制器处于急停状态，请先复位急停")
	}
	return nil
}

func (c *WTNMC4AMotionController) axisConfigLocked(axis core.AxisName) (core.AxisConfig, bool) {
	for _, axisCfg := range c.profile.Axes {
		if axisCfg.Enabled && axisCfg.Name == axis {
			return axisCfg, true
		}
	}
	return core.AxisConfig{}, false
}

func (c *WTNMC4AMotionController) copyStatusLocked() core.ControllerStatus {
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	return status
}

// cacheAxisSpeedsLocked 计算并缓存各轴的速度参数，同时写入硬件寄存器。
// 使用 int32 以匹配 C LONG（4字节）的内存布局。
func (c *WTNMC4AMotionController) cacheAxisSpeedsLocked() {
	if c.speedParams == nil {
		c.speedParams = make(map[int]*axisSpeedParams)
	}
	for _, axisCfg := range c.profile.Axes {
		if !axisCfg.Enabled {
			continue
		}
		an := wtnmc4aAxisNum(axisCfg.Name)

		maxSpeed := core.ValueOrFloat(axisCfg.MaxSpeed, 100)
		ppu := core.PulsesPerUnit(axisCfg)
		pulseSpeed := int64(math.Round(maxSpeed * math.Abs(ppu)))
		pulseSpeed = core.ClampInt64(pulseSpeed, speedMin, speedMax)

		c.speedParams[an] = &axisSpeedParams{
			Multiple:     1,
			DriveSpeed:   int32(pulseSpeed),
			StartSpeed:   int32(defaultStartSpeed),
			Acceleration: int32(defaultAcceleration),
			Deceleration: int32(defaultAcceleration),
			AccIncRate:   int32(defaultAccIncRate),
			DecIncRate:   int32(defaultDecIncRate),
		}

		c.procs.setSV.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].StartSpeed)))
		c.procs.setV.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].DriveSpeed)))
		c.procs.setA.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].Acceleration)))
		c.procs.setDec.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].Deceleration)))
	}
}

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

func wtnmc4aEngineeringToPulse(axisCfg core.AxisConfig, value float64) int64 {
	pulse := core.EngineeringToPulse(axisCfg, value)
	if axisCfg.Inverted {
		pulse = -pulse
	}
	return pulse
}

func wtnmc4aPulseToEngineering(axisCfg core.AxisConfig, pulse float64) float64 {
	signedPulse := pulse
	if axisCfg.Inverted {
		signedPulse = -signedPulse
	}
	return core.PulseToEngineering(axisCfg, signedPulse)
}

func wtnmc4aFindDLL() string {
	return "WTNMC4A_64.dll"
}

var _ ports.MotionController = (*WTNMC4AMotionController)(nil)
