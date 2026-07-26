//go:build windows

package hardware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
	"shared.local/device-sdk/go/pkg/slog"
)

const (
	defaultPort = 5000
	speedMin    = 1
	speedMax    = 8000

	defaultSendTO       = 200
	defaultRecvTO       = 200
	defaultStartSpeed   = 500
	defaultAcceleration = 500
	defaultAccIncRate   = 1000
	defaultDecIncRate   = 1000
	wtnmc4aMaxMovePulse = 268435455

	lvdvPulse = 0
	lvdvCont  = 1
	autoDec   = 0
	// 脉冲输出方式：1 = CP/DIR 方式（WTNMC4A_CPDIR），一根脉冲线 + 一根方向线。
	// 注意 SDK 头文件定义：WTNMC4A_CWCCW=0x0（CW/CCW 方式，两根脉冲线），
	//                       WTNMC4A_CPDIR=0x1（CP/DIR 方式，脉冲+方向）。
	// 历史问题：早期常量名 cpDir=0 是命名误导——值 0 实际对应 CW/CCW，
	// 在该模式下负方向脉冲输出异常，位移台不动；把 PLSLogLever 改为 direction
	// 反而会让正向脉冲极性反转，正向变反向。
	// 实测对齐 Cursor DAQ（WTNMC4A_CP_DIR=1）+ PLSLogLever=direction + DIRLogLever=0
	// 后正反方向均正常。
	pulseModeCPDir       = 1
	line                 = 0
	pDirection     int32 = 1
	mDirection     int32 = 0

	// fullStatusInterval 完整状态查询间隔：快速轮询时每隔 500ms 做一次含 getRR1Status 的完整状态，
	// 刷新限位/报警/归零等慢变信号，中间轮次仅 readLP 读位置。
	fullStatusInterval   = 500 * time.Millisecond
	positionReadAttempts = 3
)

type rr1Status struct {
	CMPP, CMPM, ASND, CNST, DSND bool
	IN0, IN1, IN2, IN3           bool
	LMTP, LMTM, ALARM, EMG       bool
}

// rr0Status 对应 SDK WTNMC4A 状态寄存器 RR0 的驱动状态位。
// XDRV=1 表示 X 轴正在驱动（控制器正在输出运动脉冲），
// 0 表示 X 轴已停止。Y/Z/U 同理。
// RR0 是 SDK 明确定义的轴驱动/停止状态来源，不带阶段位残留问题。
type rr0Status struct {
	XDRV, YDRV, ZDRV, UDRV bool
}

// wtnmc4aRR0Struct 对应 C 结构体 WTNMC4A_PARA_RR0。
// XDRV/YDRV/ZDRV/UDRV 是 SDK 明确定义的各轴驱动状态，不能用 RR1 的
// 加速/定速/减速阶段位替代，否则阶段位残留会让停止轴持续显示 Moving=true。
type wtnmc4aRR0Struct struct {
	XDRV, YDRV, ZDRV, UDRV         uint32
	XERROR, YERROR, ZERROR, UERROR uint32
	IDRV, CNEXT                    uint32
	ZONE0, ZONE1, ZONE2            uint32
	BPSC0, BPSC1                   uint32
}

// wtnmc4aRR1Struct 对应 C 结构体 WTNMC4A_PARA_RR1（WTNMC4A.H 第 358-379 行）。
// SDK 定义为 16 个 UINT 字段（每个 4 字节，总 64 字节），每个字段值为 0 或 1。
// 必须用此结构体缓冲区传指针给 DLL，不能再用 [4]byte 当位掩码解析——
// 早期实现这样做导致：
//  1. DLL 写 64 字节到 4 字节缓冲区，越界破坏栈内存
//  2. 位掩码解析使 ASND/CNST/DSND 永远读到 0，axis.moving 永远为 false，
//     运动中停止按钮始终 disabled（B140 走另一条路径不受影响）
type wtnmc4aRR1Struct struct {
	CMPP  uint32
	CMPM  uint32
	ASND  uint32
	CNST  uint32
	DSND  uint32
	AASND uint32 // S曲线加速度增加
	ACNST uint32 // S曲线加速度不变
	ADSND uint32 // S曲线加速度减少
	IN0   uint32
	IN1   uint32
	IN2   uint32
	IN3   uint32
	LMTP  uint32
	LMTM  uint32
	ALARM uint32
	EMG   uint32
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
	getRR0     *syscall.Proc
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

type trustedPositionSample struct {
	pulse int32
	at    time.Time
}

type statusQueryFlight struct {
	done   chan struct{}
	status core.ControllerStatus
	err    error
}

type WTNMC4AMotionController struct {
	mu               sync.RWMutex
	ioMu             sync.Mutex
	statusQueryMu    sync.Mutex
	statusQuery      *statusQueryFlight
	profile          core.MotionControllerProfile
	status           core.ControllerStatus
	handle           uintptr
	dll              *syscall.DLL
	procs            dllProcs
	speedParams      map[int]*axisSpeedParams
	trustedPositions map[int]trustedPositionSample
	stateVersion     uint64

	// Test seams for deterministic DLL failure and concurrency tests.
	readLP    func(handle uintptr, axis int) int32
	readRR0   func(handle uintptr) (rr0Status, error)
	readRR1   func(handle uintptr, axis int) (rr1Status, error)
	startMove func(axis int, pulse int32) error
	stopAxis  func(handle uintptr, axis int) error

	// 每 500ms 做一次完整状态（含 getRR1Status），刷新限位/报警/归零等慢变信号。
	lastFullStatusAt time.Time // 上次完整状态时间

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
		profile:          profile,
		status:           status,
		speedParams:      make(map[int]*axisSpeedParams),
		trustedPositions: make(map[int]trustedPositionSample),
	}
}

func (c *WTNMC4AMotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	c.mu.Lock()
	c.profile = profile
	c.trustedPositions = make(map[int]trustedPositionSample)
	c.stateVersion++
	needReconfig := c.handle != 0 && c.status.Connected
	if needReconfig {
		err := c.cacheAxisSpeedsLocked()
		if err != nil {
			c.mu.Unlock()
			return err
		}
	}
	c.mu.Unlock()
	return nil
}

func (c *WTNMC4AMotionController) Connect(ctx context.Context) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.status.Connected && c.handle != 0 {
		return nil
	}

	// 确保重连时不会残留上一轮的DLL句柄或设备句柄
	c.cleanupConnectionLocked()
	c.lastFullStatusAt = time.Time{}

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
		getRR0:     dll.MustFindProc("WTNMC4A_GetRR0Status"),
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
	if ret == 0 || ret == ^uintptr(0) {
		c.cleanupConnectionLocked()
		c.status.LastError = fmt.Sprintf("创建设备句柄失败: %s:%d", c.profile.Address, c.profile.Port)
		return fmt.Errorf("WTNMC4A 连接 %s:%d 失败: DEV_CreateA 返回失败句柄 0x%x", c.profile.Address, c.profile.Port, ret)
	}
	c.handle = ret
	slog.Info("WTNMC4A DEV_CreateA returned handle", "address", c.profile.Address, "handle", fmt.Sprintf("0x%x", ret))

	if err := c.verifyConnectionLocked(); err != nil {
		c.cleanupConnectionLocked()
		c.status.LastError = err.Error()
		return err
	}

	c.speedParams = make(map[int]*axisSpeedParams)
	c.trustedPositions = make(map[int]trustedPositionSample)
	if err := c.cacheAxisSpeedsLocked(); err != nil {
		c.cleanupConnectionLocked()
		c.status.LastError = err.Error()
		return err
	}

	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	c.stateVersion++
	return nil
}

func (c *WTNMC4AMotionController) Disconnect(ctx context.Context) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
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

	c.lastFullStatusAt = time.Time{}
	c.trustedPositions = make(map[int]trustedPositionSample)

	c.status.Connected = false
	c.status.EmergencyStopped = false
	c.stateVersion++
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
		c.status.Axes[i].Compensating = false
	}
	return nil
}

// Status 返回控制器和所有轴的状态。
//
// 快速路径优化：仅当距离上次完整状态 >= 500ms 时才调用 getRR0Status + getRR1Status（1+4 次 DLL 调用），
// 中间轮次仅调用 readLP（每轴 1 次 DLL 调用），DLL 调用量减少，限位/报警/归零等慢变信号在完整状态时刷新。
//
// 运动判定来源为 SDK RR0 寄存器的 XDRV/YDRV/ZDRV/UDRV 驱动状态位（每控制器 1 次 DLL 调用），
// 而非 RR1 的 ASND/CNST/DSND 加速/定速/减速阶段位（每轴 1 次）。RR0 是 SDK 明确指定用于判断
// 轴是否正在驱动的寄存器，不因阶段位残留而误判为运动中。
// RR1 继续只用于限位（LMTP/LMTM）、报警（ALARM/EMG）等轴级状态。
//
// Single-flight 合并：同一时刻每台控制器最多运行一轮 RR0/RR1/LP 查询，后续调用者共享
// 第一轮的结果（成功或失败）。这是 spec Decision 2 "一个控制器一个采集 flight" 在 WTNMC4A
// adapter 层的最小落地，避免多消费者在 ioMu 上排队后重复访问 DLL。
//
// 注意：等待者 ctx 取消时返回的 status 是当前缓存快照（c.copyStatusLocked），
// 不是发起者最终结果——两者可能不一致（发起者可能稍后成功更新缓存）。
// 调用方拿到 (status, ctx.Err()) 时只能把 status 当作"最近一次已知状态"
// 用于诊断/兜底，不能当作本轮采集结果。如需本轮结果必须用非取消的 ctx
// 重新调用 Status。
func (c *WTNMC4AMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.statusQueryMu.Lock()
	if active := c.statusQuery; active != nil {
		c.statusQueryMu.Unlock()
		select {
		case <-active.done:
			return active.status, active.err
		case <-ctx.Done():
			c.mu.RLock()
			status := c.copyStatusLocked()
			c.mu.RUnlock()
			return status, ctx.Err()
		}
	}

	flight := &statusQueryFlight{done: make(chan struct{})}
	c.statusQuery = flight
	c.statusQueryMu.Unlock()

	status, err := c.queryStatus(ctx)

	c.statusQueryMu.Lock()
	flight.status = status
	flight.err = err
	if c.statusQuery == flight {
		c.statusQuery = nil
	}
	close(flight.done)
	c.statusQueryMu.Unlock()
	return status, err
}

// queryStatus 执行一轮真实硬件查询。Status 保证同一控制器同一时刻仅运行一轮，
// 其他并发调用等待并复用结果，避免多个轮询源在 ioMu 上排队后重复访问 DLL。
func (c *WTNMC4AMotionController) queryStatus(ctx context.Context) (core.ControllerStatus, error) {
	startTime := time.Now()

	c.mu.RLock()
	if err := c.checkConnectedLocked(); err != nil {
		status := c.copyStatusLocked()
		c.mu.RUnlock()
		return status, err
	}

	handle := c.handle
	queryVersion := c.stateVersion
	needFullStatus := time.Since(c.lastFullStatusAt) >= fullStatusInterval

	type axisQuery struct {
		axisNum int
		axisIdx int
		axisCfg core.AxisConfig
	}
	queries := make([]axisQuery, 0, len(c.status.Axes))

	// 快速路径所需的缓存值，必须在 RLock 内读取避免数据竞争
	cachedHomed := make([]bool, len(c.status.Axes))
	cachedMoving := make([]bool, len(c.status.Axes))
	cachedPosLimit := make([]bool, len(c.status.Axes))
	cachedNegLimit := make([]bool, len(c.status.Axes))
	for i := range c.status.Axes {
		axisCfg, ok := c.axisConfigLocked(c.status.Axes[i].Name)
		if !ok {
			continue
		}
		queries = append(queries, axisQuery{
			axisNum: wtnmc4aAxisNum(c.status.Axes[i].Name),
			axisIdx: i,
			axisCfg: axisCfg,
		})
		cachedHomed[i] = c.status.Axes[i].Homed
		cachedMoving[i] = c.status.Axes[i].Moving
		cachedPosLimit[i] = c.status.Axes[i].PosLimit
		cachedNegLimit[i] = c.status.Axes[i].NegLimit
	}
	c.mu.RUnlock()

	type axisResult struct {
		axisIdx       int
		position      float64
		positionValid bool
		moving        bool
		homed         bool
		posLimit      bool
		negLimit      bool
		statusValid   bool
	}
	results := make([]axisResult, 0, len(queries))
	var statusErrors []error

	dllStart := time.Now()
	// 一次性读取 RR0 寄存器（所有轴驱动状态），用于后续逐轴的 moving 判定。
	// 在完整状态窗口中提前查询，避免在每个轴的 ioMu 锁内重复读 RR0。
	var rr0 rr0Status
	var rr0Err error
	if needFullStatus {
		c.ioMu.Lock()
		if !c.connectionMatches(handle) {
			rr0Err = fmt.Errorf("控制器连接已变更")
		} else {
			rr0, rr0Err = c.getRR0Status(handle)
		}
		c.ioMu.Unlock()
		if rr0Err != nil {
			statusErrors = append(statusErrors, fmt.Errorf("WTNMC4A 驱动状态读取失败: %w", rr0Err))
		}
	}
	for _, q := range queries {
		// Serialize one axis query at a time so Stop/EmergencyStop can run between
		// axes instead of waiting for an entire four-axis status batch.
		c.ioMu.Lock()
		if !c.connectionMatches(handle) {
			c.ioMu.Unlock()
			c.mu.RLock()
			status := c.copyStatusLocked()
			c.mu.RUnlock()
			return status, fmt.Errorf("控制器连接已变更")
		}
		retryYield := func() error {
			c.ioMu.Unlock()
			c.ioMu.Lock()
			if !c.connectionMatches(handle) {
				return fmt.Errorf("控制器连接已变更")
			}
			return nil
		}
		position, positionErr := c.readTrustedPosition(handle, q.axisNum, q.axisCfg, retryYield)
		c.ioMu.Unlock()
		if positionErr != nil {
			statusErrors = append(statusErrors, fmt.Errorf("WTNMC4A 轴 %s 位置读取失败: %w", q.axisCfg.Name, positionErr))
		}

		var moving, homed, posLimit, negLimit bool
		statusValid := false
		if needFullStatus {
			c.ioMu.Lock()
			var rr1 rr1Status
			var rrErr error
			if !c.connectionMatches(handle) {
				rrErr = fmt.Errorf("控制器连接已变更")
			} else {
				rr1, rrErr = c.getRR1Status(handle, q.axisNum)
			}
			c.ioMu.Unlock()
			if rrErr != nil {
				statusErrors = append(statusErrors, fmt.Errorf("WTNMC4A 轴 %s 状态读取失败: %w", q.axisCfg.Name, rrErr))
			} else if rr0Err == nil {
				// 使用 RR0 驱动状态位判定运动/停止，RR1 只取限位信号。
				// 不能用 RR1 的 ASND/CNST/DSND 替代：控制器停止后阶段位可能残留，
				// 而上层看门狗依赖准确的 Moving=false 来做到位判定。
				moving = rr0.axisDriving(q.axisNum)
				posLimit = rr1.LMTP
				negLimit = rr1.LMTM
				statusValid = true
				// ReadLP precedes GetRR1Status, so an axis can stop between the two
				// calls. Pair the stopped state with a fresh final position instead
				// of exposing the earlier in-motion sample as a stopped position.
				if cachedMoving[q.axisIdx] && !moving {
					c.ioMu.Lock()
					if !c.connectionMatches(handle) {
						positionErr = fmt.Errorf("控制器连接已变更")
					} else {
						position, positionErr = c.readTrustedPosition(handle, q.axisNum, q.axisCfg)
					}
					c.ioMu.Unlock()
					if positionErr != nil {
						statusErrors = append(statusErrors, fmt.Errorf("WTNMC4A 轴 %s 停止后位置读取失败: %w", q.axisCfg.Name, positionErr))
					}
				}
			}
			if positionErr == nil {
				homed = core.IsHomed(position, q.axisCfg)
			}
		} else {
			// 快速路径：运动状态从缓存读取，避免低速/刚启动时位置未变误判停止
			moving = cachedMoving[q.axisIdx]
			// 限位/报警/归零从 RLock 内读取的缓存值获取（完整状态时刷新）
			homed = cachedHomed[q.axisIdx]
			posLimit = cachedPosLimit[q.axisIdx]
			negLimit = cachedNegLimit[q.axisIdx]
		}

		results = append(results, axisResult{
			axisIdx:       q.axisIdx,
			position:      position,
			positionValid: positionErr == nil,
			moving:        moving,
			homed:         homed,
			posLimit:      posLimit,
			negLimit:      negLimit,
			statusValid:   statusValid,
		})
	}
	dllDuration := time.Since(dllStart)

	// 写锁更新状态（极短时间）
	c.mu.Lock()
	if c.stateVersion != queryVersion {
		statusCopy := c.copyStatusLocked()
		c.mu.Unlock()
		return statusCopy, nil
	}
	for _, r := range results {
		axisStatus := &c.status.Axes[r.axisIdx]
		if r.positionValid {
			axisStatus.Position = r.position
			if needFullStatus {
				axisStatus.Homed = r.homed
			}
		}
		if !needFullStatus || r.statusValid {
			axisStatus.Moving = r.moving
			axisStatus.PosLimit = r.posLimit
			axisStatus.NegLimit = r.negLimit
		}
		axisStatus.Compensating = false
		axisStatus.CompensationError = ""
		axisStatus.PositionError = 0
	}
	statusErr := errors.Join(statusErrors...)
	if statusErr != nil {
		c.status.LastError = statusErr.Error()
	} else {
		c.status.LastError = ""
	}
	if needFullStatus {
		fullStatusSucceeded := true
		for _, r := range results {
			if !r.statusValid {
				fullStatusSucceeded = false
				break
			}
		}
		if fullStatusSucceeded {
			c.lastFullStatusAt = time.Now()
		}
	}
	statusCopy := c.copyStatusLocked()
	c.mu.Unlock()

	totalDuration := time.Since(startTime)
	slog.Debug("WTNMC4A Status",
		"dll_ms", dllDuration.Milliseconds(),
		"total_ms", totalDuration.Milliseconds(),
		"full", needFullStatus,
		"axes", len(queries),
	)
	return statusCopy, statusErr
}

// axisDriving 根据轴序号返回 RR0 中对应轴的驱动状态。
// axisNum 映射：0→XDRV, 1→YDRV, 2→ZDRV, 3→UDRV。
// 未知轴号返回 false（保守处理，不误判为运动中）。
func (s rr0Status) axisDriving(axisNum int) bool {
	switch axisNum {
	case 0:
		return s.XDRV
	case 1:
		return s.YDRV
	case 2:
		return s.ZDRV
	case 3:
		return s.UDRV
	default:
		return false
	}
}

// InitLVDV + StartLVDV 启动轴的运动（参照标准SDK示例用法）。
// InitLVDV 一次性设置所有运动参数（速度、加速度、方向、脉冲数等），
// 避免因硬件清除寄存器导致的运动失败。
func (c *WTNMC4AMotionController) moveAxisInit(an int, targetPulse int32) error {
	if targetPulse > wtnmc4aMaxMovePulse || targetPulse < -wtnmc4aMaxMovePulse {
		return fmt.Errorf("WTNMC4A 轴 %d 目标脉冲 %d 超出硬件上限 %d", an, targetPulse, wtnmc4aMaxMovePulse)
	}
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

	// 脉冲模式必须用 CP/DIR（pulseModeCPDir=1），不能用 CW/CCW（0）。
	// PLSLogLever 必须与 Direction 同步，DIRLogLever 固定 0。
	// 此组合对齐 Cursor DAQ 实测可用的写法（见 WTNMC4AMotionControllerFFI.ts:263-273）。
	// 历史问题：早期用 cpDir=0（CW/CCW 方式）+ PLSLogLever=0，
	// 导致负方向时位移台不移动；仅改 PLSLogLever=direction 不改 PulseMode
	// 会让正向脉冲极性反转，正向变反向。
	lcData := paraLCData{
		AxisNum:     int32(an),
		LVDV:        lvdvPulse,
		DecMode:     autoDec,
		PulseMode:   pulseModeCPDir,
		PLSLogLever: direction,
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
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkReadyLocked(); err != nil {
		return err
	}
	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	if err := validateWTNMC4ATarget(axisCfg, position); err != nil {
		return fmt.Errorf("WTNMC4A 轴 %s 目标位置无效: %w", axis, err)
	}
	an := wtnmc4aAxisNum(axis)

	currentPos, err := c.readTrustedPosition(c.handle, an, axisCfg)
	if err != nil {
		c.status.LastError = fmt.Sprintf("轴 %s 当前位置不可信: %v", axis, err)
		return fmt.Errorf("WTNMC4A 轴 %s 当前位置不可信，拒绝运动: %w", axis, err)
	}
	deltaPulse := wtnmc4aEngineeringToPulse(axisCfg, position) - wtnmc4aEngineeringToPulse(axisCfg, currentPos)
	if deltaPulse == 0 {
		return nil
	}
	if deltaPulse > wtnmc4aMaxMovePulse || deltaPulse < -wtnmc4aMaxMovePulse {
		return fmt.Errorf("WTNMC4A 轴 %s 运动距离超出硬件范围: %d", axis, deltaPulse)
	}

	startMove := c.resolveStartMove()
	if err := startMove(an, int32(deltaPulse)); err != nil {
		c.status.LastError = fmt.Sprintf("轴 %s 运动失败: %v", axis, err)
		return fmt.Errorf("WTNMC4A 轴 %s 运动失败: %w", axis, err)
	}
	c.status.LastError = ""
	c.setAxisMovingLocked(axis, true)
	c.stateVersion++
	return nil
}

// MoveBy 相对定位：使用 InitLVDV + StartLVDV 实现。delta 为正则正方向，为负则反方向。
func (c *WTNMC4AMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
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
	currentPos, err := c.readTrustedPosition(c.handle, an, axisCfg)
	if err != nil {
		return fmt.Errorf("WTNMC4A 轴 %s 当前位置不可信，拒绝运动: %w", axis, err)
	}
	if err := validateWTNMC4ATarget(axisCfg, currentPos+delta); err != nil {
		return fmt.Errorf("WTNMC4A 轴 %s 相对运动目标无效: %w", axis, err)
	}

	deltaPulse := wtnmc4aEngineeringToPulse(axisCfg, delta)
	if deltaPulse == 0 {
		return nil
	}
	if deltaPulse > wtnmc4aMaxMovePulse || deltaPulse < -wtnmc4aMaxMovePulse {
		return fmt.Errorf("WTNMC4A 轴 %s 运动距离超出硬件范围: %d", axis, deltaPulse)
	}

	startMove := c.resolveStartMove()
	if err := startMove(an, int32(deltaPulse)); err != nil {
		c.status.LastError = fmt.Sprintf("轴 %s 运动失败: %v", axis, err)
		return fmt.Errorf("WTNMC4A 轴 %s 运动失败: %w", axis, err)
	}
	c.status.LastError = ""
	c.setAxisMovingLocked(axis, true)
	c.stateVersion++
	return nil
}

// resolveStartMove 解析运动启动函数的测试 seam：c.startMove 为 nil 时回退到生产实现 moveAxisInit。
//
// 抽取自 MoveTo/MoveBy 两处相同的兜底模板，避免新增字段时漏改某一处。
// Jog 走 InitLVDV+StartLVDV 连续模式路径，不经过此 seam。
func (c *WTNMC4AMotionController) resolveStartMove() func(axis int, pulse int32) error {
	if c.startMove != nil {
		return c.startMove
	}
	return c.moveAxisInit
}

// resolveStopAxis 解析轴停止函数的测试 seam：c.stopAxis 为 nil 时回退到生产实现 callInstStop。
//
// 抽取自 Stop 单轴分支与 stopAllAxesLocked 内相同的兜底模板，与 resolveStartMove 对称。
func (c *WTNMC4AMotionController) resolveStopAxis() func(handle uintptr, axis int) error {
	if c.stopAxis != nil {
		return c.stopAxis
	}
	return c.callInstStop
}

// Jog 连续运动：参照标准SDK用法，使用 InitLVDV（连续模式 LVDV=1）+ StartLVDV
func (c *WTNMC4AMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
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

	// 脉冲模式 + PLSLogLever + DIRLogLever 配置与 moveAxisInit 一致（详见其注释）。
	lcData := paraLCData{
		AxisNum:     int32(an),
		LVDV:        lvdvCont,
		DecMode:     autoDec,
		PulseMode:   pulseModeCPDir,
		PLSLogLever: direction,
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
	c.setAxisMovingLocked(axis, true)
	c.stateVersion++
	return nil
}

func (c *WTNMC4AMotionController) Home(ctx context.Context, axis core.AxisName) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
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
	c.setAxisMovingLocked(axis, true)
	c.stateVersion++
	return nil
}

// Stop serializes the immediate-stop command with other calls on the same DLL
// handle. Status releases the I/O lock between axes to bound normal contention.
func (c *WTNMC4AMotionController) Stop(ctx context.Context, axis core.AxisName) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	c.mu.RLock()
	handle := c.handle
	connected := c.status.Connected
	c.mu.RUnlock()
	if handle == 0 || !connected {
		return fmt.Errorf("控制器未连接")
	}

	if axis == "" {
		return c.stopAllAxesLocked(handle, "停止")
	}
	c.mu.RLock()
	_, axisExists := c.axisConfigLocked(axis)
	c.mu.RUnlock()
	if !axisExists {
		return fmt.Errorf("未知或未启用的运动轴: %s", axis)
	}
	an := wtnmc4aAxisNum(axis)
	stopAxis := c.resolveStopAxis()
	if err := stopAxis(handle, an); err != nil {
		return fmt.Errorf("WTNMC4A 轴 %s 停止失败: %w", axis, err)
	}
	c.mu.Lock()
	c.setAxisMovingLocked(axis, false)
	c.stateVersion++
	c.mu.Unlock()
	return nil
}

// EmergencyStop serializes immediate-stop commands with the vendor DLL handle.
//
// 标志位置位时序（安全优先）：
//   - 先置位 EmergencyStopped，再调用 stopAllAxesLocked。即使后续停止调用失败，
//     标志已锁存，可阻止并发的 MoveTo/MoveBy/Jog 等运动命令发起 DLL 调用。
//   - 若先停止再置位，存在时间窗：停止执行期间其他 goroutine 可能发起新运动。
//   - 急停失败也保留标志位（不回滚），由调用方通过返回的 error 诊断现场，
//     需显式调用 ResetEmergencyStop 才能清除。
func (c *WTNMC4AMotionController) EmergencyStop(ctx context.Context) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	c.mu.RLock()
	handle := c.handle
	connected := c.status.Connected
	c.mu.RUnlock()
	if handle == 0 || !connected {
		return fmt.Errorf("控制器未连接")
	}

	c.mu.Lock()
	c.status.EmergencyStopped = true
	c.mu.Unlock()
	return c.stopAllAxesLocked(handle, "急停")
}

// stopAllAxesLocked 在已校验 handle/connected 后，依次停止 0~3 号轴并更新 status。
//
// 抽取自 Stop(axis="") 与 EmergencyStop 的共用逻辑：
//   - 解析 stopAxis 注入点（测试 seam，nil 时回退到 callInstStop）
//   - 依次对 4 个轴调用 stopAxis，记录成功轴索引到 stoppedAxes
//   - 拿 mu 锁，仅将成功停止的轴的 Moving 置 false（失败轴保留原状态，
//     由调用方通过 LastError 诊断）；同步刷新 LastError 与 stateVersion
//
// 不设置 EmergencyStopped 标志：该标志仅 EmergencyStop 应置位，调用方按语义处理。
// errLabel 用于错误消息文案区分（"停止" / "急停"），保留与原实现一致的中文消息。
//
// 调用方约束：必须已持有 ioMu（保证与同控制器的其他命令串行）；
// 调用本方法期间不得持有 mu（方法内部加锁更新 status 后释放）。
func (c *WTNMC4AMotionController) stopAllAxesLocked(handle uintptr, errLabel string) error {
	stopAxis := c.resolveStopAxis()
	var stopErrors []error
	stoppedAxes := make(map[int]bool, 4)
	for _, an := range []int{0, 1, 2, 3} {
		if err := stopAxis(handle, an); err != nil {
			stopErrors = append(stopErrors, fmt.Errorf("轴%d%s失败: %w", an, errLabel, err))
			continue
		}
		stoppedAxes[an] = true
	}
	stopErr := errors.Join(stopErrors...)
	c.mu.Lock()
	for i := range c.status.Axes {
		if stoppedAxes[wtnmc4aAxisNum(c.status.Axes[i].Name)] {
			c.status.Axes[i].Moving = false
		}
	}
	if stopErr != nil {
		c.status.LastError = stopErr.Error()
	} else {
		c.status.LastError = ""
	}
	c.stateVersion++
	c.mu.Unlock()
	return stopErr
}

func (c *WTNMC4AMotionController) ResetEmergencyStop(ctx context.Context) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
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
	c.trustedPositions = make(map[int]trustedPositionSample)
	c.stateVersion++
	return nil
}

func (c *WTNMC4AMotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	c.ioMu.Lock()
	defer c.ioMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}
	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	if err := validateWTNMC4ATarget(axisCfg, position); err != nil {
		return fmt.Errorf("WTNMC4A 轴 %s 定义位置无效: %w", axis, err)
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
	c.trustedPositions[an] = trustedPositionSample{pulse: int32(pulse), at: time.Now()}
	c.status.LastError = ""
	c.stateVersion++
	return nil
}

func (c *WTNMC4AMotionController) checkConnectedLocked() error {
	if c.handle == 0 || !c.status.Connected {
		return fmt.Errorf("控制器未连接")
	}
	return nil
}

func (c *WTNMC4AMotionController) connectionMatches(handle uintptr) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.handle == handle && c.status.Connected
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

func (c *WTNMC4AMotionController) verifyConnectionLocked() error {
	axisNum := 0
	if len(c.profile.Axes) > 0 {
		axisNum = wtnmc4aAxisNum(c.profile.Axes[0].Name)
	}
	// SDK 写入 64 字节结构体，必须用匹配的缓冲区，避免越界破坏栈内存
	var raw wtnmc4aRR1Struct
	ret, _, _ := c.procs.getRR1.Call(c.handle, uintptr(axisNum), uintptr(unsafe.Pointer(&raw)))
	if ret == 0 {
		return fmt.Errorf("WTNMC4A 连接 %s 验证失败: GetRR1Status 无响应", c.profile.Address)
	}
	return nil
}

func (c *WTNMC4AMotionController) cleanupConnectionLocked() {
	if c.handle != 0 && c.procs.devRelease != nil {
		c.procs.devRelease.Call(c.handle)
		c.handle = 0
	}
	if c.dll != nil {
		c.dll.Release()
		c.dll = nil
	}
	c.status.Connected = false
}

func (c *WTNMC4AMotionController) axisConfigLocked(axis core.AxisName) (core.AxisConfig, bool) {
	for _, axisCfg := range c.profile.Axes {
		if axisCfg.Enabled && axisCfg.Name == axis {
			return axisCfg, true
		}
	}
	return core.AxisConfig{}, false
}

func (c *WTNMC4AMotionController) setAxisMovingLocked(axis core.AxisName, moving bool) {
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == axis {
			c.status.Axes[i].Moving = moving
			return
		}
	}
}

func (c *WTNMC4AMotionController) copyStatusLocked() core.ControllerStatus {
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	return status
}

// cacheAxisSpeedsLocked 计算并缓存各轴的速度参数，同时写入硬件寄存器。
// 使用 int32 以匹配 C LONG（4字节）的内存布局。
func (c *WTNMC4AMotionController) cacheAxisSpeedsLocked() error {
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

		if ret, _, _ := c.procs.setSV.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].StartSpeed))); ret == 0 {
			return fmt.Errorf("WTNMC4A 设置轴 %s 起始速度失败", axisCfg.Name)
		}
		if ret, _, _ := c.procs.setV.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].DriveSpeed))); ret == 0 {
			return fmt.Errorf("WTNMC4A 设置轴 %s 驱动速度失败", axisCfg.Name)
		}
		if ret, _, _ := c.procs.setA.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].Acceleration))); ret == 0 {
			return fmt.Errorf("WTNMC4A 设置轴 %s 加速度失败", axisCfg.Name)
		}
		if ret, _, _ := c.procs.setDec.Call(c.handle, uintptr(an), uintptr(int64(c.speedParams[an].Deceleration))); ret == 0 {
			return fmt.Errorf("WTNMC4A 设置轴 %s 减速度失败", axisCfg.Name)
		}
	}
	return nil
}

// getRR1Status 查询指定轴的状态寄存器（接受 handle 参数，不依赖锁）。
//
// SDK 的 WTNMC4A_GetRR1Status 把 64 字节的 WTNMC4A_PARA_RR1 结构体写入调用方提供的缓冲区，
// 每个字段是 0/1 的 UINT。这里用 wtnmc4aRR1Struct 结构体匹配内存布局，逐字段读取。
func (c *WTNMC4AMotionController) getRR1Status(handle uintptr, axisNum int) (rr1Status, error) {
	if c.readRR1 != nil {
		return c.readRR1(handle, axisNum)
	}
	var raw wtnmc4aRR1Struct
	ret, _, _ := c.procs.getRR1.Call(handle, uintptr(axisNum), uintptr(unsafe.Pointer(&raw)))
	if ret == 0 {
		return rr1Status{}, fmt.Errorf("GetRR1Status 返回失败")
	}
	return rr1Status{
		CMPP:  raw.CMPP != 0,
		CMPM:  raw.CMPM != 0,
		ASND:  raw.ASND != 0,
		CNST:  raw.CNST != 0,
		DSND:  raw.DSND != 0,
		IN0:   raw.IN0 != 0,
		IN1:   raw.IN1 != 0,
		IN2:   raw.IN2 != 0,
		IN3:   raw.IN3 != 0,
		LMTP:  raw.LMTP != 0,
		LMTM:  raw.LMTM != 0,
		ALARM: raw.ALARM != 0,
		EMG:   raw.EMG != 0,
	}, nil
}

// getRR0Status 查询 RR0 状态寄存器（所有轴统一查询，不依赖锁）。
//
// SDK 的 WTNMC4A_GetRR0Status 把 12 个 UINT 字段结构体写入调用方提供的缓冲区，
// 每个字段是 0/1 的值。这里用 wtnmc4aRR0Struct 匹配内存布局，仅提取 4 个轴驱动位。
// RR0 不分轴号：一次调用返回 X/Y/Z/U 全部轴的驱动状态。
//
// 与 getRR1Status 的区别：RR1 查询的是单轴状态且需逐个调用，RR0 一次查询所有轴。
func (c *WTNMC4AMotionController) getRR0Status(handle uintptr) (rr0Status, error) {
	if c.readRR0 != nil {
		return c.readRR0(handle)
	}
	var raw wtnmc4aRR0Struct
	ret, _, _ := c.procs.getRR0.Call(handle, uintptr(unsafe.Pointer(&raw)))
	if ret == 0 {
		return rr0Status{}, fmt.Errorf("GetRR0Status 返回失败")
	}
	return rr0Status{
		XDRV: raw.XDRV != 0,
		YDRV: raw.YDRV != 0,
		ZDRV: raw.ZDRV != 0,
		UDRV: raw.UDRV != 0,
	}, nil
}

// getRR1StatusLocked 保留原有方法名（内部调用 handle 版本），
// 确保其他可能在锁内调用的代码兼容。
func (c *WTNMC4AMotionController) getRR1StatusLocked(axisNum int) rr1Status {
	status, _ := c.getRR1Status(c.handle, axisNum)
	return status
}

func (c *WTNMC4AMotionController) readLogicalPosition(handle uintptr, axisNum int) int32 {
	if c.readLP != nil {
		return c.readLP(handle, axisNum)
	}
	ret, _, _ := c.procs.readLP.Call(handle, uintptr(axisNum))
	return int32(ret)
}

func (c *WTNMC4AMotionController) callInstStop(handle uintptr, axisNum int) error {
	ret, _, _ := c.procs.instStop.Call(handle, uintptr(axisNum))
	if ret == 0 {
		return fmt.Errorf("InstStop 返回失败")
	}
	return nil
}

func (c *WTNMC4AMotionController) readTrustedPosition(handle uintptr, axisNum int, axisCfg core.AxisConfig, retryYield ...func() error) (float64, error) {
	var lastErr error
	var initialCandidate *trustedPositionSample
	yield := func() error { return nil }
	if len(retryYield) > 0 && retryYield[0] != nil {
		yield = retryYield[0]
	}
	for attempt := 1; attempt <= positionReadAttempts; attempt++ {
		now := time.Now()
		pulse := c.readLogicalPosition(handle, axisNum)
		previous, hasPrevious := c.trustedPositions[axisNum]
		if !hasPrevious {
			if initialCandidate == nil {
				if err := validateWTNMC4APositionSample(axisCfg, pulse, trustedPositionSample{}, now); err != nil {
					lastErr = err
					slog.Warn("WTNMC4A rejected initial position sample",
						"axis", axisCfg.Name, "raw_pulse", pulse, "attempt", attempt, "error", err)
					if err := yield(); err != nil {
						return 0, err
					}
					continue
				}
				candidate := trustedPositionSample{pulse: pulse, at: now}
				initialCandidate = &candidate
				if err := yield(); err != nil {
					return 0, err
				}
				continue
			}
			if err := validateWTNMC4APositionSample(axisCfg, pulse, *initialCandidate, now); err != nil {
				lastErr = err
				slog.Warn("WTNMC4A initial position samples disagree",
					"axis", axisCfg.Name, "candidate_pulse", initialCandidate.pulse,
					"raw_pulse", pulse, "attempt", attempt, "error", err)
				if standaloneErr := validateWTNMC4APositionSample(axisCfg, pulse, trustedPositionSample{}, now); standaloneErr == nil {
					candidate := trustedPositionSample{pulse: pulse, at: now}
					initialCandidate = &candidate
				}
				if err := yield(); err != nil {
					return 0, err
				}
				continue
			}
			previous = *initialCandidate
		}
		if err := validateWTNMC4APositionSample(axisCfg, pulse, previous, now); err != nil {
			lastErr = err
			slog.Warn("WTNMC4A rejected position sample",
				"axis", axisCfg.Name, "raw_pulse", pulse, "attempt", attempt,
				"has_previous", hasPrevious, "error", err)
			if err := yield(); err != nil {
				return 0, err
			}
			continue
		}
		c.trustedPositions[axisNum] = trustedPositionSample{pulse: pulse, at: now}
		return wtnmc4aPulseToEngineering(axisCfg, float64(pulse)), nil
	}
	return 0, fmt.Errorf("连续 %d 次位置样本无效: %w", positionReadAttempts, lastErr)
}

func validateWTNMC4APositionSample(axisCfg core.AxisConfig, pulse int32, previous trustedPositionSample, now time.Time) error {
	// The LP register is a signed 32-bit counter. Exact extrema commonly indicate
	// a failed native call and are not useful physical positions.
	if pulse == math.MinInt32 || pulse == math.MaxInt32 {
		return fmt.Errorf("脉冲值 %d 为无效边界值", pulse)
	}
	position := wtnmc4aPulseToEngineering(axisCfg, float64(pulse))
	if axisCfg.MinLimit != nil && position < *axisCfg.MinLimit {
		return fmt.Errorf("位置 %.6f 小于软件下限 %.6f", position, *axisCfg.MinLimit)
	}
	if axisCfg.MaxLimit != nil && position > *axisCfg.MaxLimit {
		return fmt.Errorf("位置 %.6f 大于软件上限 %.6f", position, *axisCfg.MaxLimit)
	}
	if previous.at.IsZero() {
		return nil
	}
	elapsed := now.Sub(previous.at)
	if elapsed < 0 {
		elapsed = 0
	}
	ppu := math.Abs(core.PulsesPerUnit(axisCfg))
	maxSpeed := math.Abs(core.ValueOrFloat(axisCfg.MaxSpeed, core.DefaultMaxSpeed))
	// Allow 500ms of acceleration/polling jitter while still rejecting register-sized spikes.
	maxJump := int64(math.Ceil(maxSpeed*ppu*(elapsed.Seconds()+0.5))) + 10
	jump := int64(pulse) - int64(previous.pulse)
	if jump < 0 {
		jump = -jump
	}
	if jump > maxJump {
		return fmt.Errorf("脉冲跳变 %d 超过物理允许值 %d（间隔 %s）", jump, maxJump, elapsed)
	}
	return nil
}

func validateWTNMC4ATarget(axisCfg core.AxisConfig, position float64) error {
	if math.IsNaN(position) || math.IsInf(position, 0) {
		return fmt.Errorf("位置必须是有限数值")
	}
	if axisCfg.MinLimit != nil && axisCfg.MaxLimit != nil && *axisCfg.MinLimit > *axisCfg.MaxLimit {
		return fmt.Errorf("软件下限 %.6f 大于上限 %.6f", *axisCfg.MinLimit, *axisCfg.MaxLimit)
	}
	if axisCfg.MinLimit != nil && position < *axisCfg.MinLimit {
		return fmt.Errorf("位置 %.6f 小于软件下限 %.6f", position, *axisCfg.MinLimit)
	}
	if axisCfg.MaxLimit != nil && position > *axisCfg.MaxLimit {
		return fmt.Errorf("位置 %.6f 大于软件上限 %.6f", position, *axisCfg.MaxLimit)
	}
	pulse := wtnmc4aEngineeringToPulse(axisCfg, position)
	if pulse < math.MinInt32 || pulse > math.MaxInt32 {
		return fmt.Errorf("目标脉冲 %d 超出逻辑位置寄存器范围", pulse)
	}
	return nil
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

// wtnmc4aFindDLL 查找 WTNMC4A_64.dll 的路径。
// 优先从可执行文件所在目录查找，确保安装包部署后能正确定位 DLL。
func wtnmc4aFindDLL() string {
	dllName := "WTNMC4A_64.dll"
	// 获取可执行文件所在目录
	exePath, err := os.Executable()
	if err != nil {
		slog.Warn("获取可执行文件路径失败，使用默认DLL名", "error", err)
		return dllName
	}
	dllPath := filepath.Join(filepath.Dir(exePath), dllName)
	if _, err := os.Stat(dllPath); err == nil {
		return dllPath
	}
	// 回退到系统搜索路径（PATH、exe同目录等）
	slog.Debug("exe目录下未找到DLL，回退到系统搜索路径", "dll", dllName, "checked", dllPath)
	return dllName
}

var _ ports.MotionController = (*WTNMC4AMotionController)(nil)
