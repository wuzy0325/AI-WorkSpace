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
// 采集流程（Phase 4 Task 9）：
//   1. StartAcquisition: ffi.StartTask + ffi.SendSoftTrig → 启动 readLoop goroutine
//   2. readLoop: 循环 ReadBinary(1 sample/chan) → ScaleBinToVolt → 按通道
//      SensorType 端点插值到工程量 → 组装 DataPayload → 投递 sink
//   3. StopAcquisition: 设置 StopReasonUserRequested → close(stop) → ffi.StopTask
//      → 等待 readLoopDone（1 秒超时）
//
// 状态机：
//   Disconnected ──Connect()──▶ Connected ──StartAcquisition()──▶ Acquiring
//        │                          │                                   │
//        └───────── Error ──────────┴────── StopAcquisition() ──────────┘
// 任意步骤失败进入 Error 态，LastError 记录原因。
// ============================================================

// ErrDAQP1603NotImplemented 标记尚未实现的方法。
// Phase 2 阶段 StartAcquisition/StopAcquisition 返回此错误，
// 防止上层在能力未就绪时进入采集流程。
var ErrDAQP1603NotImplemented = fmt.Errorf("DAQ-P-1603 acquisition not implemented (Task 9)")

// DAQP1603MaxSampleRate 是 DAQ-P-1603 的采样率上限（500Hz）。
// 项目 spec D-2 决策：低速定制版本，上限 500Hz。
// 前端 UI 与后端 ApplyConfig 都做校验，越界拒绝提交并返回错误。
// 双重校验保证即使前端被绕过（如直接调 API），后端仍能拦截非法值。
const DAQP1603MaxSampleRate = 500

// 电压量程上下限（与 buildAIParamLocked 默认 SAMPRANGE_N10_P10V 对齐）。
// 端点线性插值公式：E = rangeMin + (V - vMin) / (vMax - vMin) * (rangeMax - rangeMin)
// 用户在 profile 中配置的 rangeMin/rangeMax 已隐含所选单位（Pa/kPa/℃ 等），
// 因此采集层无需二次单位换算；℃ → ℉ 等显示转换是 UI 层职责。
const (
	daqP1603VoltMin = -10.0
	daqP1603VoltMax = 10.0
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

	// 1. 建立 TCP 连接（DLL 内部封装，超时 0 表示用 DLL 默认 200ms）
	handle, err := ffi.WTNDAQ16HDevCreate(ip, 0, 0)
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
	if err := ffi.WTNDAQ16HStartTask(handle); err != nil {
		d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 start task: %w", err))
		return fmt.Errorf("DAQ-P-1603 start task: %w", err)
	}
	// 连续采集模式下仍需 SendSoftTrig 触发首帧数据流（厂商示例行为）
	if err := ffi.WTNDAQ16HSendSoftTrig(handle); err != nil {
		// 启动失败需回滚 StartTask，避免 DLL 内部状态卡在"已启动但无触发"
		_ = ffi.WTNDAQ16HStopTask(handle)
		d.setStatusErrorLocked(fmt.Errorf("DAQ-P-1603 send soft trig: %w", err))
		return fmt.Errorf("DAQ-P-1603 send soft trig: %w", err)
	}

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
// 提取为包级私有类型便于在 readLoop 与 buildChannelScales 间传递。
type chanScale struct {
	origIndex int     // 通道在 profile.Channels 中的原始索引（用户可见通道号）
	rangeMin  float64 // 工程量下限（隐含单位：Pa / kPa / ℃ 等）
	rangeMax  float64 // 工程量上限
}

// buildChannelScales 根据通道配置构造换算参数切片。
//
// 设计动机：把"扫描 profile.Channels + 过滤启用 + 量程兜底"的纯逻辑
// 从 readLoop 中提取出来，便于单元测试覆盖通道过滤与量程兜底分支。
//
// 兜底策略：当某通道 rangeMin==0 && rangeMax==0 时，用 ±10V 兜底
// （与电压量程一致，相当于"无换算"），避免除零或负斜率。
//
// 上限截断：profile.Channels 长度可能 >16（用户误配），按 WTNDAQ16H_AI_MAX_CHANNELS 截断。
func buildChannelScales(channels []core.ChannelConfig) []chanScale {
	scales := make([]chanScale, 0, len(channels))
	for i, ch := range channels {
		if i >= ffi.WTNDAQ16H_AI_MAX_CHANNELS {
			break
		}
		if !ch.Enabled {
			continue
		}
		rMin, rMax := ch.RangeMin, ch.RangeMax
		if rMin == 0 && rMax == 0 {
			rMin, rMax = daqP1603VoltMin, daqP1603VoltMax
		}
		scales = append(scales, chanScale{origIndex: i, rangeMin: rMin, rangeMax: rMax})
	}
	return scales
}

// scaleVoltToEngineering 将电压值按端点线性插值换算为工程量。
//
// 公式：E = rangeMin + (V - vMin) / (vMax - vMin) * (rangeMax - rangeMin)
//   - vMin/vMax = ±10V（与 buildAIParamLocked 的 SAMPRANGE_N10_P10V 对齐）
//   - rangeMin/rangeMax 由用户在 profile 中配置，隐含所选单位（Pa/kPa/℃ 等）
//   - 压力通道与温度通道走同一公式，区别仅在于 rangeMin/rangeMax 的语义
//
// 提取为独立函数便于单元测试覆盖：
//   - 端点边界（V == vMin / V == vMax）
//   - 中点（V == 0）
//   - 越界（V > vMax 或 V < vMin，理论上不应发生但需可处理）
func scaleVoltToEngineering(volt, rMin, rMax float64) float64 {
	voltSpan := daqP1603VoltMax - daqP1603VoltMin
	return rMin + (volt-daqP1603VoltMin)/voltSpan*(rMax-rMin)
}

// readLoop 是采集主循环 goroutine。
//
// 设计要点：
//   - 启动时持锁快照 handle/sink/profile（含 channels 的 SensorType），
//     之后整个循环不持锁，所有 FFI 调用与 sink 投递并发安全（单 goroutine 串行）
//   - 每帧 ReadBinary 读取 1 sample/chan（即所有启用通道的一个时间点快照）
//   - ScaleBinToVolt 将 U16 码值换算为电压（V）
//   - 按通道 SensorType 端点线性插值到工程量（rangeMin/rangeMax 隐含单位）
//   - select stop 通道实现优雅退出（StopAcquisition close 后立即返回）
//
// 异常退出处理：
//   - ReadBinary/ScaleBinToVolt 错误：日志 + 设置 status.Error + 调用 onReadLoopExit
//   - 主动停止（StopReasonUserRequested）：不上报，静默退出
//
// 通道索引映射：
//   - binArray 布局：[s0_ch0, s0_ch1, ..., s0_chN-1, s1_ch0, ...]（按通道顺序排列）
//   - 取每帧第 0 个 sample：binArray[0..N-1] 对应 N 个启用通道
//   - DataPayload.ChannelIndices = 启用通道的原始 index（用户可见通道号）
//   - DataPayload.Channels = 工程量值（按 ChannelIndices 顺序）
func (d *DAQP1603) readLoop() {
	// 持锁快照：避免循环中反复加锁，且保证 profile.Channels 一致性。
	// 关键：stop channel 必须快照，StopAcquisition 会将 d.stop 置 nil，
	// 循环中若直接读 d.stop 会变成 nil channel（永久阻塞），无法响应停止。
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
		// 无启用通道：直接退出并标记错误（不应发生，Connect 时已兜底全启用）
		d.handleReadLoopExit(fmt.Errorf("DAQ-P-1603 readLoop: no enabled channels"))
		if done != nil {
			close(done)
		}
		return
	}

	// 缓冲区：每帧 1 sample/chan，binArray 长度 = nChans
	binBuf := make([]uint16, nChans)
	voltBuf := make([]float64, nChans)
	const sampsPerChan = uint32(1)
	const readTimeout = 1.0 // 秒，覆盖 500Hz 下 2 帧间隔

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

	for {
		// 优先检查 stop 信号，避免在停止后仍执行一次 ReadBinary（可能阻塞 1 秒）
		select {
		case <-stop:
			// 主动停止：优雅退出，不上报错误
			return
		default:
		}

		sampsRead, _, err := ffi.WTNDAQ16HReadBinary(handle, binBuf, sampsPerChan, readTimeout)
		if err != nil {
			// 主动停止时 stop channel 已 close，但 ReadBinary 可能仍返回 timeout 错误
			// → 检查 stopReason 区分
			if d.stopReason.GetStopReason() == protocol.StopReasonUserRequested {
				return
			}
			d.handleReadLoopExit(fmt.Errorf("DAQ-P-1603 read binary: %w", err))
			return
		}
		if sampsRead == 0 {
			// timeout 但无错误：继续下一轮（可能设备暂未产生数据）
			continue
		}

		// U16 → 电压（V）。ScaleBinToVolt 的 rangeInfo 用 nil 表示使用 InitTask 时的量程，
		// gainInfo 用 nil 表示无增益（与 buildAIParamLocked 默认配置一致）。
		if _, err := ffi.WTNDAQ16HScaleBinToVolt(nil, nil, voltBuf, binBuf, nChans); err != nil {
			if d.stopReason.GetStopReason() == protocol.StopReasonUserRequested {
				return
			}
			d.handleReadLoopExit(fmt.Errorf("DAQ-P-1603 scale to volt: %w", err))
			return
		}

		// 按通道端点线性插值到工程量（压力/温度通道走同一公式，区别在 rangeMin/rangeMax）
		engValues := make([]float64, nChans)
		indices := make([]int, nChans)
		for i := uint32(0); i < nChans; i++ {
			engValues[i] = scaleVoltToEngineering(voltBuf[i], scales[i].rangeMin, scales[i].rangeMax)
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
// 默认配置策略：
//   - 通道扫描模式：连续扫描（CONTINUOUS），覆盖所有启用通道
//   - 采样信号：AI 通道信号（SAMPSIGNAL_AI）
//   - 采样模式：连续采集（CONTINUOUS），适合长时间压力/温度监测
//   - 时钟源：本地时钟（LOCAL）
//   - 量程：±10V（N10_P10V），后续 Task 6 ApplyConfig 时按通道覆盖
//   - 触发：禁用（无 DTriggerEn / ATriggerEn）
//   - SampsPerChan：1024，连续模式下表示 DLL 内部环形缓冲区大小
//
// 通道配置：
//   - 优先使用 profile.Channels 中 Enabled=true 的通道
//   - 若无启用通道，默认启用全部 16 通道
func (d *DAQP1603) buildAIParamLocked() ffi.WTNDAQ16HAIParam {
	var p ffi.WTNDAQ16HAIParam

	enabledCount := uint32(0)
	for i, ch := range d.profile.Channels {
		if i >= ffi.WTNDAQ16H_AI_MAX_CHANNELS {
			break
		}
		if !ch.Enabled {
			continue
		}
		p.CHParam[enabledCount] = ffi.WTNDAQ16HAIChParam{
			Channel:     uint32(i),
			SampleRange: ffi.WTNDAQ16H_AI_SAMPRANGE_N10_P10V,
			RefGround:   ffi.WTNDAQ16H_AI_REFGND_DIFF,
		}
		enabledCount++
	}

	// 无启用通道时默认全部 16 通道，保证 Connect 阶段能完成 InitTask。
	// 实际通道屏蔽由 Task 6 ApplyConfig 在运行时调整。
	if enabledCount == 0 {
		enabledCount = ffi.WTNDAQ16H_AI_MAX_CHANNELS
		for i := uint32(0); i < ffi.WTNDAQ16H_AI_MAX_CHANNELS; i++ {
			p.CHParam[i] = ffi.WTNDAQ16HAIChParam{
				Channel:     i,
				SampleRange: ffi.WTNDAQ16H_AI_SAMPRANGE_N10_P10V,
				RefGround:   ffi.WTNDAQ16H_AI_REFGND_DIFF,
			}
		}
	}

	p.SampChanCount = enabledCount
	p.ChanScanMode = ffi.WTNDAQ16H_AI_CHAN_SCANMODE_CONTINUOUS
	p.GroupLoops = 1
	p.SampleSignal = ffi.WTNDAQ16H_AI_SAMPSIGNAL_AI

	p.SampleMode = ffi.WTNDAQ16H_AI_SAMPMODE_CONTINUOUS
	p.SampsPerChan = 1024

	rate := float64(d.profile.SamplingRate)
	if rate <= 0 {
		// 默认 500Hz：与项目 spec 中 DAQ-P-1603 的采样率上限一致
		rate = 500
	}
	p.SampleRate = rate
	p.ClockSource = ffi.WTNDAQ16H_AI_CLKSRC_LOCAL

	return p
}
