package hardware

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/daq/ports"
	"shared.local/device-sdk/go/ffi"
	"shared.local/device-sdk/go/protocol"
)

// ============================================================
// DAQ-P-1603 16 通道通用 AI 采集设备适配器
// ------------------------------------------------------------
// 与 DAQ-T-1603（裸 TCP + 文本协议）不同，DAQ-P-1603 通过厂商
// DLL（WTNDAQ16H_64.dll）封装 TCP/IP 通信，Go 端只持有 DLL
// 返回的 device handle（uintptr），不直接管理 net.Conn。
//
// 硬件配置（固定，所有 16 通道一致）：
//   - 量程：0-20mA（WTNDAQ16H_AI_SAMPRANGE_0_20mA）
//   - 传感器：4-20mA 工业标准电流环
//
// 数据转换链（两步）：
//   Step 1: U16 原始码值 → 电流值(mA)
//     Go 端按公式 current = code / 65535 * 20 换算（0-20mA 量程下满量程对应 20mA）
//     例：U16=39700 → 39700/65535*20 ≈ 12.12 mA
//     不调用 DLL 的 ScaleBinToVolt，该函数为电压模式设计，电流模式下会崩溃。
//   Step 2: 电流值(mA) → 工程量(Pa/kPa/℃ 等)
//     scaleCurrentToEngineering 使用 4-20mA 标准映射：
//     4mA → engMin, 20mA → engMax
//     公式：engValue = engMin + (current-4)/16 * (engMax-engMin)
//
// 采集流程：
//   1. StartAcquisition: ffi.StartTask + ffi.SendSoftTrig → 启动 readLoop goroutine
//   2. readLoop: 循环 ReadBinary → Go 端换算(mA)
//      → scaleCurrentToEngineering(工程量) → 组装 DataPayload → 投递 sink
//   3. StopAcquisition: close(stop) → ffi.StopTask → 等待 readLoopDone（1 秒超时）
//
// 状态机：
//   Disconnected ──Connect()──▶ Connected ──StartAcquisition()──▶ Acquiring
//        │                          │                                   │
//        └───────── Error ──────────┴────── StopAcquisition() ──────────┘
// 任意步骤失败进入 Error 态，LastError 记录原因。
// ============================================================

// DAQP1603MaxSampleRate 是 DAQ-P-1603 的采样率上限。
// 硬件限制 500Hz（WTNDAQ16H DLL 内部定时器精度限制），前后端均有校验拒绝越界值。
const DAQP1603MaxSampleRate = 500

// 4-20mA 电流环传感器的电气量程。
// 所有 DAQ-P-1603 通道固定使用 0-20mA 硬件量程（SAMPRANGE_0_20mA），
// 传感器遵循 4-20mA 工业标准：4mA=量程下限，20mA=量程上限。
// 低于 4mA 表示传感器故障/断线，输出值会低于 engMin（前端可据此告警）。
const (
	daqP1603CurrentMin  = 4.0  // 4mA 活零点，对应工程量下限
	daqP1603CurrentMax  = 20.0 // 20mA 满量程，对应工程量上限
	daqP1603CurrentSpan = daqP1603CurrentMax - daqP1603CurrentMin // 16mA 量程跨度
)

// DAQP1603 实现 core.Device 接口。
// 所有状态迁移由 mu 互斥锁保护，handle==0 视为未连接。
type DAQP1603 struct {
	mu sync.RWMutex

	profile core.Profile
	status  core.Status
	sink    core.DataSink

	// handle 是 WTNDAQ16H_DEV_CreateA 返回的设备句柄。
	// 0 表示未连接，非 0 表示已建立连接并完成 InitTask。
	handle uintptr

	// acquiring 标记是否正在采集。
	// Start/Stop 互斥由 mu 保护，readLoop 退出时也会置 false。
	acquiring bool

	// stop 是 StartAcquisition 创建的停止信号 channel。
	// StopAcquisition 通过 close(stop) 通知 readLoop 主动退出。
	// nil 表示当前无活跃采集。
	stop chan struct{}

	// readLoopDone 由 StartAcquisition 创建，readLoop 退出时关闭。
	// StopAcquisition 通过 select 等待它实现"1 秒内退出"语义。
	// nil 表示当前无活跃 readLoop。
	readLoopDone chan struct{}

	// stopReason 区分主动停止 vs 异常停止。
	// StopAcquisition 先 SetStopReason(UserRequested) 再 close(stop)；
	// readLoop 退出时检查此值决定是否上报异常。
	stopReason protocol.StopReasonTracker

	// onReadLoopExit 在 readLoop 异常退出时被调用（nil 表示无回调）。
	// wind-daq thin wrapper 注册此回调以推送 UI 状态栏错误。
	onReadLoopExit func(error)
}

// OnReadLoopExit 注册 readLoop 异常退出回调。
// thin wrapper 调用此方法注册一个将错误推送 UI 状态栏的回调。
// 可在 Connect 之前或之后调用；仅当下次 readLoop 启动时生效。
func (d *DAQP1603) OnReadLoopExit(fn func(error)) {
	d.mu.Lock()
	d.onReadLoopExit = fn
	d.mu.Unlock()
}

// 编译期断言：DAQP1603 必须实现 ports.Device 全部方法。
// 若接口新增方法而本类型未实现，编译期即可发现。
var _ ports.Device = (*DAQP1603)(nil)

// NewDAQP1603 构造一个 DAQ-P-1603 设备实例。
// 初始状态为 Disconnected，调用 Connect 后才与硬件通信。
func NewDAQP1603(profile core.Profile) *DAQP1603 {
	return &DAQP1603{
		profile: profile,
		status: core.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: core.ConnectionDisconnected,
		},
	}
}

// ID 返回设备唯一标识，用于日志、UI 展示与 sink 路由。
func (d *DAQP1603) ID() string { return d.profile.ID }

// Connect 建立 TCP 连接并初始化采集任务。
// 流程：DLL 句柄检查 → DEV_Create → VerifyParam → InitTask。
// 仅完成任务初始化，不启动数据流（StartTask 留待 StartAcquisition）。
// 重复调用安全：已连接时直接返回 nil。
func (d *DAQP1603) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 幂等：已连接直接返回，避免重复 InitTask 破坏 DLL 内部状态
	if d.handle != 0 {
		return nil
	}

	// 配置错误优先于 DLL 检查报出：IP 缺失属于 profile 配置问题，
	// 操作员应先修正 profile；DLL 未初始化属于程序启动流程问题。
	// 两者错误码不同，先报配置错误便于定位。
	ip := d.profile.Address
	if ip == "" {
		err := fmt.Errorf("DAQ-P-1603 profile missing address (IP)")
		d.setStatusErrorLocked(err)
		return err
	}

	// DLL 必须由程序启动代码先调用 ffi.InitWTNDAQ16H 加载，
	// 否则所有 Proc 调用会 panic。此处前置检查给出明确错误。
	if !ffi.IsWTNDAQ16HInitialized() {
		err := fmt.Errorf("WTNDAQ16H DLL not initialized, call ffi.InitWTNDAQ16H at startup")
		d.setStatusErrorLocked(err)
		return err
	}

	// 1. 建立 TCP 连接（DLL 内部封装）
	// sendTimeout/recvTimeout 对应 C 头文件 WTNDAQ16H_DEV_CreateA 的
	// nSendTimeout/nRecvTimeout 参数（默认 200ms）。C++ 默认参数仅在编译期
	// 由编译器填充，通过 Go FFI 调用时必须显式传值，传 0 会导致 0ms 超时
	// 使 TCP 连接来不及握手即超时断开。
	handle, err := ffi.WTNDAQ16HDevCreate(ip, 200, 200)
	if err != nil {
		d.setStatusErrorLocked(err)
		return fmt.Errorf("DAQ-P-1603 connect %s: %w", ip, err)
	}

	// 2. 构造 AI 参数并校验合法性
	// VerifyParam 在 InitTask 之前调用，避免非法参数导致 DLL 内部状态异常。
	param := d.buildAIParamLocked()
	if err := ffi.WTNDAQ16HVerifyParam(handle, &param); err != nil {
		// 失败时必须释放已建立的连接，否则 DLL 内部会泄漏 socket
		_ = ffi.WTNDAQ16HDevRelease(handle)
		d.setStatusErrorLocked(err)
		return fmt.Errorf("DAQ-P-1603 verify param: %w", err)
	}

	// 3. 初始化采集任务（不启动采集）
	// sampEvent 传 0 表示使用轮询模式，不依赖 Windows Event 通知，
	// 简化跨 goroutine 采集循环的实现（Task 9）。
	if err := ffi.WTNDAQ16HInitTask(handle, &param, 0); err != nil {
		_ = ffi.WTNDAQ16HDevRelease(handle)
		d.setStatusErrorLocked(err)
		return fmt.Errorf("DAQ-P-1603 init task: %w", err)
	}

	d.handle = handle
	d.status.Connection = core.ConnectionConnected
	d.status.LastError = ""
	return nil
}

// Disconnect 释放采集任务与设备连接。
// 顺序：StopTask（防御性）→ ReleaseTask → DevRelease。
// 重复调用安全：未连接时直接返回 nil。
func (d *DAQP1603) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 幂等：未连接直接返回
	if d.handle == 0 {
		return nil
	}

	// 防御性停止：上层应先调用 StopAcquisition，但若遗忘则在此兜底。
	// DLL 的 StopTask 重复调用安全，不会返回错误。
	if d.acquiring {
		_ = ffi.WTNDAQ16HStopTask(d.handle)
		d.acquiring = false
		d.status.Acquiring = false
	}

	// ReleaseTask 必须在 DevRelease 之前调用，释放采集任务资源。
	// 顺序颠倒会导致 DLL 内部资源泄漏。
	_ = ffi.WTNDAQ16HReleaseTask(d.handle)
	_ = ffi.WTNDAQ16HDevRelease(d.handle)

	d.handle = 0
	d.status.Connection = core.ConnectionDisconnected
	return nil
}

// SetDataSink 注册数据回调。采集循环通过 sink 推送 DataPayload。
// 可在 Connect 之前或之后调用；Task 9 的 readLoop 启动时读取此值。
func (d *DAQP1603) SetDataSink(sink core.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

// Status 返回当前设备状态快照。返回值是拷贝，调用方可安全读取。
func (d *DAQP1603) Status() core.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

// StartAcquisition 启动数据采集。
//
// 流程（持锁）：
//   1. 幂等：已采集直接返回 nil
//   2. 检查 handle != 0（必须先 Connect）
//   3. ffi.WTNDAQ16HStartTask + ffi.WTNDAQ16HSendSoftTrig 启动 DLL 数据流
//   4. 设置 acquiring=true, status.Acquiring=true, status.Connection=Acquiring
//   5. 创建 stop / readLoopDone channel，ClearStopReason
//   6. 启动 readLoop goroutine
//
// 失败处理：StartTask/SendSoftTrig 失败时设置 status.Error，不修改 acquiring。
// 重复调用安全：已采集时直接返回 nil，不会重启 readLoop。
func (d *DAQP1603) StartAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.acquiring {
		return nil
	}
	if d.handle == 0 {
		return fmt.Errorf("DAQ-P-1603 start acquisition: device not connected")
	}

	handle := d.handle
	ip := d.profile.Address

	// startAndCheck 封装 StartTask + SendSoftTrig + 分步 GetStatus 诊断。
	// 返回 true 表示任务已成功启动（TaskState==2）；false 表示启动后停滞在 idle 态。
	startAndCheck := func(h uintptr) (bool, error) {
		if err := ffi.WTNDAQ16HStartTask(h); err != nil {
			return false, fmt.Errorf("start task: %w", err)
		}
		// 诊断点 A：StartTask 后立即查 TaskState
		var sA ffi.WTNDAQ16HAIStatus
		_ = ffi.WTNDAQ16HGetStatus(h, &sA)

		if err := ffi.WTNDAQ16HSendSoftTrig(h); err != nil {
			slog.Error("DAQ-P-1603 SendSoftTrig failed, taskState after StartTask",
				"device", d.profile.ID,
				"taskStateAfterStartTask", sA.TaskState,
				"err", err)
			_ = ffi.WTNDAQ16HStopTask(h)
			return false, fmt.Errorf("send soft trig: %w", err)
		}
		// 诊断点 B：SendSoftTrig 后查 TaskState
		// 按厂商头文件 WTNDAQ16H.H 第 154 行：nTaskState =1 表示"正常(运行中)"，
		// 其他值才表示异常。曾经误把 TaskState==2 当成 running，导致 StartTask+SendSoftTrig
		// 后 TaskState=1（正常态）被误判为"未运行"，触发无效 reconnect 重试并最终误报
		// "DLL global state corrupted"。
		var sB ffi.WTNDAQ16HAIStatus
		if err := ffi.WTNDAQ16HGetStatus(h, &sB); err != nil {
			_ = ffi.WTNDAQ16HStopTask(h)
			return false, fmt.Errorf("post-start GetStatus: %w", err)
		}
		if sB.TaskState != 1 {
			slog.Warn("DAQ-P-1603 TaskState abnormal after StartTask+SendSoftTrig",
				"device", d.profile.ID,
				"taskStateAfterStartTask", sA.TaskState,
				"taskStateAfterSendSoftTrig", sB.TaskState,
				"note", "expected=1(running) per WTNDAQ16H.H")
			_ = ffi.WTNDAQ16HStopTask(h)
			return false, nil
		}
		return true, nil
	}

	ok, err := startAndCheck(handle)
	if err != nil {
		d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 start acquisition: %w", err))
		return fmt.Errorf("DAQ-P-1603 start acquisition: %w", err)
	}
	if !ok {
		// 首次启动失败，尝试自愈：Disconnect → Connect 重建 DLL 会话。
		// 触发条件：StartTask+SendSoftTrig 后 TaskState 不是 1（按 WTNDAQ16H.H 第 154 行
		// 1=正常,其它值=异常）。
		// 历史背景：曾因 ScaleBinToVolt 在电流模式下 Access Violation 污染 DLL 进程级状态，
		// 重建会话可清理 per-handle 残留状态。当前已改为 Go 端换算，该风险已消除，
		// 此分支仅作为 DLL 异常状态的兜底自愈，常规情况下不会触发。
		slog.Warn("DAQ-P-1603 first StartTask abnormal, retrying with reconnect",
			"device", d.profile.ID, "ip", ip)

		_ = ffi.WTNDAQ16HStopTask(handle)
		_ = ffi.WTNDAQ16HReleaseTask(handle)
		_ = ffi.WTNDAQ16HDevRelease(handle)
		d.handle = 0

		// 重新 Connect。若重连后 TaskState 仍非 1（即仍异常），说明 DLL 进程级状态已坏，
		// per-handle 重建无法修复，需完全退出 Go 进程再启动。
		newHandle, connErr := ffi.WTNDAQ16HDevCreate(ip, 200, 200)
		if connErr != nil {
			d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 reconnect failed: %w", connErr))
			return fmt.Errorf("DAQ-P-1603 reconnect failed: %w", connErr)
		}
		param := d.buildAIParamLocked()
		if verifyErr := ffi.WTNDAQ16HVerifyParam(newHandle, &param); verifyErr != nil {
			_ = ffi.WTNDAQ16HDevRelease(newHandle)
			d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 reconnect verify param: %w", verifyErr))
			return fmt.Errorf("DAQ-P-1603 reconnect verify param: %w", verifyErr)
		}
		if initErr := ffi.WTNDAQ16HInitTask(newHandle, &param, 0); initErr != nil {
			_ = ffi.WTNDAQ16HDevRelease(newHandle)
			d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 reconnect init task: %w", initErr))
			return fmt.Errorf("DAQ-P-1603 reconnect init task: %w", initErr)
		}
		d.handle = newHandle
		handle = newHandle

		ok, err = startAndCheck(handle)
		if err != nil {
			d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 start acquisition (retry): %w", err))
			return fmt.Errorf("DAQ-P-1603 start acquisition (retry): %w", err)
		}
		if !ok {
			// 两次 StartTask+SendSoftTrig 后 TaskState 均非 1（异常） → DLL 进程级状态已坏，
			// per-handle 重建无法修复，需完全退出 Go 进程再启动
			err := fmt.Errorf("DAQ-P-1603 start acquisition: DLL TaskState still abnormal after reconnect — DLL global state corrupted, exit & restart (ip=%s)", ip)
			d.setStatusErrorLocked(err)
			return err
		}
	}

	// 自检通过后读取最终状态用于日志
	var aiStatus ffi.WTNDAQ16HAIStatus
	_ = ffi.WTNDAQ16HGetStatus(handle, &aiStatus)

	enabledChans := 0
	for _, ch := range d.profile.Channels {
		if ch.Enabled {
			enabledChans++
		}
	}
	slog.Info("DAQ-P-1603 acquisition starting readLoop",
		"device", d.profile.ID,
		"ip", ip,
		"rate", d.profile.SamplingRate,
		"enabledChans", enabledChans,
		"totalChans", len(d.profile.Channels),
		"taskState", aiStatus.TaskState,
		"availSampsPerChan", aiStatus.AvailSampsPerChan)

	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.stopReason.ClearStopReason()

	slog.Info("DAQ-P-1603 acquisition started", "device", d.profile.ID, "rate", d.profile.SamplingRate)
	go d.readLoop()
	return nil
}

// StopAcquisition 停止数据采集。
//
// 流程（持锁设置信号 → 释放锁等待 → 重新持锁清理）：
//   1. 幂等：未采集直接返回 nil
//   2. 设置 StopReasonUserRequested（readLoop 据此跳过异常上报）
//   3. close(stop) 通知 readLoop 主动退出
//   4. acquiring=false, status.Acquiring=false, status.Connection=Connected
//   5. ffi.WTNDAQ16HStopTask 停止 DLL 数据流
//   6. 释放锁等待 readLoopDone（1 秒超时，spec 要求 1 秒内退出）
//   7. 重新持锁清理 readLoopDone
//
// 重复调用安全：未采集时直接返回 nil。
func (d *DAQP1603) StopAcquisition() error {
	d.mu.Lock()
	if !d.acquiring {
		d.mu.Unlock()
		return nil
	}

	// 标记主动停止，readLoop 退出时据此跳过错误上报
	d.stopReason.SetStopReason(protocol.StopReasonUserRequested)

	// close(stop) 通知 readLoop 优雅退出
	if d.stop != nil {
		close(d.stop)
		d.stop = nil
	}
	d.acquiring = false
	d.status.Acquiring = false
	if d.status.Connection == core.ConnectionAcquiring {
		d.status.Connection = core.ConnectionConnected
	}

	handle := d.handle
	done := d.readLoopDone
	d.mu.Unlock()

	// 不持锁调用 FFI 与等待 readLoop 退出，避免长时间阻塞 Status() 等读操作
	if handle != 0 {
		if err := ffi.WTNDAQ16HStopTask(handle); err != nil {
			slog.Warn("DAQ-P-1603 stop task failed", "device", d.profile.ID, "err", err)
		}
	}

	if done != nil {
		select {
		case <-done:
			// readLoop 已优雅退出
		case <-time.After(time.Second):
			slog.Warn("DAQ-P-1603 timeout waiting for readLoop to exit", "device", d.profile.ID)
		}
		d.mu.Lock()
		if d.readLoopDone == done {
			d.readLoopDone = nil
		}
		d.mu.Unlock()
	}

	slog.Info("DAQ-P-1603 acquisition stopped", "device", d.profile.ID)
	return nil
}

// chanScale 是 readLoop 内部使用的单通道换算参数。
type chanScale struct {
	origIndex int     // 通道在 profile.Channels 中的原始索引（用户可见通道号）
	engMin    float64 // 工程量下限（隐含单位：Pa / kPa / ℃ 等，由 profile 配置）
	engMax    float64 // 工程量上限
}

// buildChannelScales 根据通道配置构造换算参数切片。
//
// 兜底策略：当某通道 RangeMin==0 && RangeMax==0 时，用 4-20mA 电流范围兜底，
// 相当于"无换算"——输出值就是电流值(mA)，便于用户在没有配置工程量程时
// 仍能看到原始电流数据。
func buildChannelScales(channels []core.ChannelConfig) []chanScale {
	scales := make([]chanScale, 0, len(channels))
	for i, ch := range channels {
		if i >= ffi.WTNDAQ16H_AI_MAX_CHANNELS {
			break
		}
		if !ch.Enabled {
			continue
		}
		engMin, engMax := ch.RangeMin, ch.RangeMax
		if engMin == 0 && engMax == 0 {
			engMin, engMax = daqP1603CurrentMin, daqP1603CurrentMax
		}
		scales = append(scales, chanScale{origIndex: i, engMin: engMin, engMax: engMax})
	}
	return scales
}

// scaleCurrentToEngineering 将电流值(mA)按 4-20mA 标准映射换算为工程量。
//
// 公式：engValue = engMin + (current - 4) / (20 - 4) * (engMax - engMin)
//   - current：ScaleBinToVolt 输出的电流值(mA)，范围 0~20
//   - 4mA 对应 engMin（量程下限），20mA 对应 engMax（量程上限）
//   - 压力通道与温度通道走同一公式，区别仅在于 engMin/engMax 的语义
//   - current < 4mA 时输出值低于 engMin，表示传感器故障/断线
func scaleCurrentToEngineering(current, engMin, engMax float64) float64 {
	// current < 0 在正常工作中不会发生（DLL 输出 ≥0），但防御性截断为 engMin
	if current < 0 {
		return engMin
	}
	return engMin + (current-daqP1603CurrentMin)/daqP1603CurrentSpan*(engMax-engMin)
}

// readLoop 是采集主循环 goroutine。
//
// 数据流：
//   ReadBinary → binBuf(U16) → Go端directScaleToCurrent → currentBuf(mA) → scaleCurrentToEngineering → engValues
//
// 每帧读取 1 sample/chan，两步换算后组装 DataPayload 投递到 sink。
//
// 异常退出处理：
//   - ReadBinary 错误：日志 + 设置 status.Error + 调用 onReadLoopExit
//   - 主动停止（StopReasonUserRequested）：不上报，静默退出
func (d *DAQP1603) readLoop() {
	d.mu.RLock()
	handle := d.handle
	sink := d.sink
	channels := d.profile.Channels
	deviceID := d.profile.ID
	done := d.readLoopDone
	stop := d.stop
	d.mu.RUnlock()

	scales := buildChannelScales(channels)
	nChans := uint32(len(scales))
	if nChans == 0 {
		d.handleReadLoopExit(fmt.Errorf("DAQ-P-1603 readLoop: no enabled channels"))
		if done != nil {
			close(done)
		}
		return
	}

	binBuf := make([]uint16, ffi.WTNDAQ16H_AI_MAX_CHANNELS) // InitTask 强制 16 通道，ReadBinary buffer 必须匹配
	currentBuf := make([]float64, nChans)
	const sampsPerChan = uint32(1)
	const readTimeout = 10.0
	const maxConsecutiveTimeouts = 6

	select {
	case <-stop:
		if done != nil {
			close(done)
		}
		return
	default:
	}

	// [诊断] readLoop 启动时查询 GetStatus，确认 DLL 侧任务状态
	var aiStatus ffi.WTNDAQ16HAIStatus
	if err := ffi.WTNDAQ16HGetStatus(handle, &aiStatus); err != nil {
		slog.Error("DAQ-P-1603 readLoop GetStatus failed", "device", deviceID, "err", err)
	} else {
		slog.Info("DAQ-P-1603 readLoop pre-loop status",
			"device", deviceID,
			"taskState", aiStatus.TaskState,
			"availSampsPerChan", aiStatus.AvailSampsPerChan,
			"sampsPerChanAcquired", aiStatus.SampsPerChanAcquired,
			"nChans", nChans)
	}

	defer func() {
		// readLoop 退出时确保 acquiring=false（StopAcquisition 路径已设置，
		// 此处兜底异常退出路径），并通知 readLoopDone
		d.mu.Lock()
		d.acquiring = false
		d.status.Acquiring = false
		if d.status.Connection == core.ConnectionAcquiring {
			d.status.Connection = core.ConnectionConnected
		}
		d.mu.Unlock()
		if done != nil {
			close(done)
		}
	}()

	// 预分配：engValues 和 indices 复用以避免每帧 make，scales 元数据不变
	engValues := make([]float64, nChans)
	indices := make([]int, nChans)

	// 连续超时计数器：设备启动稳定期可能首次超时，容忍 maxConsecutiveTimeouts 次后判定故障
	consecutiveTimeouts := 0
	firstReadLogged := false

	for {
		select {
		case <-stop:
			return
		default:
		}

		readStart := time.Now()
		sampsRead, avail, err := ffi.WTNDAQ16HReadBinary(handle, binBuf, sampsPerChan, readTimeout)
		readElapsed := time.Since(readStart).Truncate(time.Millisecond)
		if !firstReadLogged {
			slog.Info("DAQ-P-1603 readLoop first ReadBinary result",
				"device", deviceID,
				"sampsRead", sampsRead,
				"avail", avail,
				"err", err,
				"elapsed", readElapsed.String(),
				"nChans", nChans,
				"timeout", readTimeout)
			firstReadLogged = true
		}
		if err != nil {
			// 仅参数非法（空缓冲区）会走到这里；主动停止时检查 stopReason 跳过上报
			if d.stopReason.GetStopReason() == protocol.StopReasonUserRequested {
				return
			}
			d.handleReadLoopExit(fmt.Errorf("DAQ-P-1603 read binary: %w", err))
			return
		}
		if sampsRead == 0 {
			// 超时未读到数据：设备启动稳定期可能首次超时，容忍若干次后判定故障
			consecutiveTimeouts++
			if consecutiveTimeouts >= maxConsecutiveTimeouts {
				d.handleReadLoopExit(fmt.Errorf("DAQ-P-1603 read binary: %d consecutive timeouts (read=0, avail=%d)", consecutiveTimeouts, avail))
				return
			}
			continue
		}
		consecutiveTimeouts = 0

		// Step 1: U16 码值 → 电流值(mA)
		// 0-20mA 量程下，U16 满量程 65535 对应 20mA。
		// InitTask 强制 16 通道全开，ReadBinary 返回 16 通道数据，
		// 按 scales[].origIndex 从 binBuf 抽取已启用通道。
		const u16FullScale = 65535.0
		const currentFullScale = 20.0
		for i := uint32(0); i < nChans; i++ {
			currentBuf[i] = float64(binBuf[scales[i].origIndex]) / u16FullScale * currentFullScale
		}

		// Step 2: 电流值(mA) → 工程量(Pa/kPa/℃ 等)
		// 使用 4-20mA 标准映射：4mA=engMin, 20mA=engMax
		for i := uint32(0); i < nChans; i++ {
			engValues[i] = scaleCurrentToEngineering(currentBuf[i], scales[i].engMin, scales[i].engMax)
			indices[i] = scales[i].origIndex
		}

		payload := core.DataPayload{
			DeviceID:       deviceID,
			Timestamp:      core.NowMs(),
			Channels:       engValues,
			ChannelIndices: indices,
		}
		if sink != nil {
			sink(payload)
		}
	}
}

// handleReadLoopExit 处理 readLoop 异常退出：
//   - 设置 status.Error + LastError（让 Status() 反映错误状态）
//   - 调用 onReadLoopExit 回调（thin wrapper 转发到 UI 状态栏）
func (d *DAQP1603) handleReadLoopExit(err error) {
	d.mu.Lock()
	d.setStatusErrorLocked(err)
	fn := d.onReadLoopExit
	d.mu.Unlock()

	slog.Warn("DAQ-P-1603 readLoop exited unexpectedly", "device", d.profile.ID, "err", err)
	if fn != nil {
		fn(err)
	}
}

// GetProfile 返回当前设备 profile 的拷贝。
// 调用方可安全读取，不会受后续 ApplyConfig 影响。
// 与 ApplyConfig 配对，构成 Read-Modify-Write 闭环：
// UI 先 GetProfile 拿到当前配置 → 修改字段 → ApplyConfig 提交。
func (d *DAQP1603) GetProfile() core.Profile {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// 拷贝 Channels 切片，避免外部修改影响内部状态
	out := d.profile
	if len(d.profile.Channels) > 0 {
		out.Channels = append([]core.ChannelConfig(nil), d.profile.Channels...)
	}
	return out
}

// ApplyConfig 应用新的设备配置。
//
// 校验规则：
//   - profile.Type 必须为 DeviceDAQP1603
//   - SamplingRate 必须 >0 且 ≤DAQP1603MaxSampleRate（500Hz，spec D-2）
//   - 采集进行中拒绝（acquiring==true），避免运行时改参数导致 DLL 内部状态不一致
//
// 同步硬件策略：
//   - 已连接且未采集：ReleaseTask → VerifyParam → InitTask 重新初始化任务
//     （DLL 不支持热更新参数，必须先 Release 再 Init）
//   - 未连接：仅更新内部 profile，待 Connect 时由 buildAIParamLocked 生效
//
// 失败处理：
//   - 校验失败：直接返回错误，不修改任何内部状态
//   - 已连接设备的硬件同步失败：将状态置 Error，记录 LastError，保留新 profile（避免半同步状态）
func (d *DAQP1603) ApplyConfig(profile core.Profile) error {
	// 1. 参数校验（不持锁，纯只读检查）
	if profile.Type != core.DeviceDAQP1603 {
		return fmt.Errorf("DAQ-P-1603 apply config: type mismatch, got %q", profile.Type)
	}
	if profile.SamplingRate <= 0 {
		return fmt.Errorf("DAQ-P-1603 apply config: sampling rate must be > 0, got %d", profile.SamplingRate)
	}
	if profile.SamplingRate > DAQP1603MaxSampleRate {
		return fmt.Errorf("DAQ-P-1603 apply config: sampling rate %d exceeds max %d (spec D-2)",
			profile.SamplingRate, DAQP1603MaxSampleRate)
	}

	d.mu.Lock()
	// 2. 采集进行中拒绝配置变更
	if d.acquiring {
		d.mu.Unlock()
		return fmt.Errorf("DAQ-P-1603 apply config: cannot apply while acquiring")
	}
	handle := d.handle
	d.mu.Unlock()

	// 3. 已连接设备：重新初始化采集任务同步硬件
	// 释放锁后调用 FFI，避免长时间持锁阻塞 Status() 等读操作。
	// FFI 调用是原子的，且本方法在 acquiring==false 时与 Start/Stop 互斥（Start 会先持锁置 acquiring=true）。
	if handle != 0 {
		// 顺序：ReleaseTask（释放旧任务）→ VerifyParam（校验新参数）→ InitTask（用新参数初始化）
		// 与 Connect 阶段的 VerifyParam→InitTask 一致，多一步 ReleaseTask 是因为 DLL 不支持热更新。
		if err := ffi.WTNDAQ16HReleaseTask(handle); err != nil {
			d.mu.Lock()
			d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 apply config: release task: %w", err))
			d.mu.Unlock()
			return fmt.Errorf("DAQ-P-1603 apply config: release task: %w", err)
		}

		// 用新 profile 构造参数（先持锁更新 profile，再读 buildAIParamLocked）
		d.mu.Lock()
		d.profile = profile
		param := d.buildAIParamLocked()
		d.mu.Unlock()

		if err := ffi.WTNDAQ16HVerifyParam(handle, &param); err != nil {
			d.mu.Lock()
			d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 apply config: verify param: %w", err))
			d.mu.Unlock()
			return fmt.Errorf("DAQ-P-1603 apply config: verify param: %w", err)
		}
		if err := ffi.WTNDAQ16HInitTask(handle, &param, 0); err != nil {
			d.mu.Lock()
			d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 apply config: init task: %w", err))
			d.mu.Unlock()
			return fmt.Errorf("DAQ-P-1603 apply config: init task: %w", err)
		}
		return nil
	}

	// 4. 未连接设备：仅更新内部 profile
	d.mu.Lock()
	d.profile = profile
	d.mu.Unlock()
	return nil
}

// SetTare 设置指定通道的归零偏移（tare offset）。
//
// 归零语义：UI 层展示值 = 原始工程量 - TareOffset。
// 与 DAQ-P-1604 / DSA3217 / DAQ-T-1603 保持一致：偏移持久化到 profile.Channels[i].TareOffset，
// 采集 readLoop 输出的 DataPayload.Channels 仍为原始值，由前端 deviceStore.applyDisplayTare
// 在展示侧减去偏移。这样归零可即时生效且不影响采集数据流完整性。
//
// 通道越界返回错误，避免静默写入导致下游 GetTare 读取错位。
func (d *DAQP1603) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("DAQ-P-1603 set tare: invalid channel index %d (valid 0-%d)",
			channelIndex, len(d.profile.Channels)-1)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

// GetTare 读取指定通道的归零偏移。
// 未连接时同样可读（profile 在 Connect 前已由 ApplyConfig 或构造器填充）。
func (d *DAQP1603) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("DAQ-P-1603 get tare: invalid channel index %d (valid 0-%d)",
			channelIndex, len(d.profile.Channels)-1)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

// ClearTare 清除指定通道的归零偏移（置 0）。
// 等价于 SetTare(channelIndex, 0)，单独提供以匹配 ports.TareConfigurable 接口语义。
func (d *DAQP1603) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

// ---- 内部辅助方法 ----

// setStatusErrorLocked 将状态置为 Error 并记录最近错误信息。
// 调用方必须持有 d.mu 写锁。
func (d *DAQP1603) setStatusErrorLocked(err error) {
	d.status.Connection = core.ConnectionError
	d.status.LastError = err.Error()
}

// buildAIParamLocked 根据当前 profile 构造 FFI 调用所需的 WTNDAQ16HAIParam。
// 调用方必须持有 d.mu（读 d.profile）。
//
// 通道数策略：InitTask 将 16 通道全部初始化（SampChanCount=16）。历史上曾观察到
// SampChanCount<16 时数据异常，遂固定为 16；readLoop 侧再按 profile.Channels[].Enabled
// 过滤，仅处理已启用通道的数据。
// 注：厂商头文件 WTNDAQ16H.H 第 154 行 nTaskState=1 即"正常(运行中)"——早期代码误以为
// "TaskState=2 才算 running"，把 16 通道下的 TaskState=1（正常态）当成 idle，错误归因到
// SampChanCount 上；2026-07 修复 TaskState 判断后该结论需 HIL 重新验证，暂保持 16 通道。
//
// 其他配置策略：
//   - 通道扫描模式：连续扫描（CONTINUOUS）
//   - 采样信号：AI 通道信号（SAMPSIGNAL_AI）
//   - 采样模式：连续采集（CONTINUOUS）
//   - 时钟源：本地时钟（LOCAL）
//   - 量程：0-20mA（SAMPRANGE_0_20mA）
//   - 触发：禁用（无 DTriggerEn / ATriggerEn）
//   - SampsPerChan：1024
func (d *DAQP1603) buildAIParamLocked() ffi.WTNDAQ16HAIParam {
	var p ffi.WTNDAQ16HAIParam

	// 全部 16 通道统一初始化（历史结论，见上方函数注释）
	p.SampChanCount = ffi.WTNDAQ16H_AI_MAX_CHANNELS
	for i := uint32(0); i < ffi.WTNDAQ16H_AI_MAX_CHANNELS; i++ {
		p.CHParam[i] = ffi.WTNDAQ16HAIChParam{
			Channel:     i,
			SampleRange: ffi.WTNDAQ16H_AI_SAMPRANGE_0_20mA,
			RefGround:   ffi.WTNDAQ16H_AI_REFGND_DIFF,
		}
	}

	p.ChanScanMode = ffi.WTNDAQ16H_AI_CHAN_SCANMODE_CONTINUOUS
	p.GroupLoops = 1
	p.GroupInterval = 1 // 厂家示例设为 1，0 会导致采集异常
	p.SampleSignal = ffi.WTNDAQ16H_AI_SAMPSIGNAL_AI

	p.SampleMode = ffi.WTNDAQ16H_AI_SAMPMODE_CONTINUOUS
	p.SampsPerChan = 1024

	rate := float64(d.profile.SamplingRate)
	if rate <= 0 {
		rate = 500
	}
	p.SampleRate = rate
	p.ClockSource = ffi.WTNDAQ16H_AI_CLKSRC_LOCAL

	return p
}
