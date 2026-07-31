package hardware

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"shared.local/device-sdk/go/motion/core"
	sharedproto "shared.local/device-sdk/go/protocol"
)

const (
	b140DefaultPort    = 23
	b140CommandTimeout = 5 * time.Second

	// b140CompensationPollInterval 等待运动停止时的轮询间隔。
	// 太短会刷爆 B140 命令队列，太长会拖慢补偿响应。
	b140CompensationPollInterval = 20 * time.Millisecond

	// b140CompensationStartupGrace 启动宽限期：moveTo 后这段时间内即使
	// 硬件未观察到运动也不立即报错，避免硬件响应延迟导致误判。
	b140CompensationStartupGrace = 200 * time.Millisecond
)

// b140CompensationJobState 补偿任务状态机取值。
type b140CompensationJobState int

const (
	compensationStateWaitingStop  b140CompensationJobState = iota // 等待硬件运动停止
	compensationStateSettling                                     // 等待机械震荡衰减
	compensationStateChecking                                     // 检查编码器误差
	compensationStateCompensating                                 // 下发 PR 微调
	compensationStateSucceeded                                    // 补偿成功
	compensationStateFailed                                       // 补偿失败
	compensationStateCancelled                                    // 被新命令取消
)

// b140PendingCompensationRequest moveTo 下发后入队的待激活补偿请求。
// 等状态轮询发现运动已发生且停止后，升级为 b140CompensationJob。
type b140PendingCompensationRequest struct {
	targetPulse       int64                              // 目标寄存器脉冲
	targetEngineering float64                            // 目标工程位置
	cfg               core.AxisEncoderCompensationConfig // 解析后的补偿参数
	issuedAt          time.Time                          // moveTo 下发时间，用于启动宽限
	observedMotion    bool                               // 是否已观察到运动（用于静止轴误触发保护）
}

// b140CompensationJob 已激活的补偿任务。
// 一旦激活就在独立 goroutine 中运行 runAxisCompensation 状态机。
type b140CompensationJob struct {
	mu                sync.Mutex                         // 保护 state/lastError 等跨 goroutine 状态
	generation        int64                              // 代际号：新命令来时让旧任务自废
	axis              core.AxisName                      // 轴名
	physical          string                             // 物理轴（A/B/C/D）
	axisCfg           core.AxisConfig                    // 配置快照（避免运行中 profile 被改）
	cfg               core.AxisEncoderCompensationConfig // 补偿参数
	targetPulse       int64                              // 目标脉冲
	targetEngineering float64                            // 目标工程位置
	state             b140CompensationJobState           // 当前状态
	attempts          int                                // 已尝试的补偿循环次数
	startedAt         time.Time                          // 任务激活时间，用于超时判定
	lastError         string                             // 失败原因（空表示未失败）
}

func (j *b140CompensationJob) currentState() b140CompensationJobState {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state
}

func (j *b140CompensationJob) setState(state b140CompensationJobState) {
	j.mu.Lock()
	j.state = state
	j.mu.Unlock()
}

func (j *b140CompensationJob) statusSnapshot() (b140CompensationJobState, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state, j.lastError
}

// B140MotionController implements Galil DMC-B140-M TCP ASCII motion control.
type B140MotionController struct {
	mu                 sync.Mutex
	connMu             sync.Mutex
	profile            core.MotionControllerProfile
	status             core.ControllerStatus
	conn               net.Conn
	reader             *bufio.Reader
	directionSignature string
	connecting         bool

	// statusQueryMu + statusQuery 实现 Status() 入口的 single-flight 合并：
	// 同一时刻最多一轮 queryStatus 在运行，后续调用者共享第一轮结果。
	// 不与 c.mu 或 connMu 嵌套，等待者仅阻塞在 flight.done 上。
	statusQueryMu sync.Mutex
	statusQuery   *b140StatusFlight

	// compMu 保护以下三个补偿任务 map。与 c.mu 解耦：补偿 goroutine 持有
	// compMu 跑状态机，运动命令持有 c.mu 修改配置/状态，两者通过代际号通信。
	compMu                        sync.Mutex
	pendingRequests               map[core.AxisName]*b140PendingCompensationRequest
	jobs                          map[core.AxisName]*b140CompensationJob
	compensationGenerationCounter int64
}

// b140StatusFlight 表示一轮进行中的 Status 查询。
// 第一个调用者创建并执行查询，后续调用者通过 done channel 复用结果。
//
// 字段访问契约（happens-before）：
//   - 发起者在 close(done) 之前写入 status/err
//   - 等待者通过 <-done 接收后再读取 status/err
//   - close/receive 建立 happens-before，无需额外锁保护字段读写
//   - 等待者 ctx 取消时直接返回 c.copyStatusLocked()，不读 flight.status/err
type b140StatusFlight struct {
	done   chan struct{}
	status core.ControllerStatus
	err    error
}

// NewB140MotionController creates a B140 motion controller adapter.
func NewB140MotionController(profile core.MotionControllerProfile) *B140MotionController {
	if profile.Port == 0 {
		profile.Port = b140DefaultPort
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

	return &B140MotionController{
		profile:         profile,
		status:          status,
		pendingRequests: make(map[core.AxisName]*b140PendingCompensationRequest),
		jobs:            make(map[core.AxisName]*b140CompensationJob),
	}
}

// ApplyConfig applies a new controller profile without disconnecting.
// Updates the cached profile and invalidates the direction signature so
// ensureDirectionConfiguredLocked re-applies inversion settings on the next command.
func (c *B140MotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profile = profile
	c.rebuildStatusAxesLocked()
	c.directionSignature = ""
	return nil
}

// GetProfile returns the controller profile.
func (c *B140MotionController) GetProfile() core.MotionControllerProfile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.profile
}

// Connect opens TCP connection, enables all axes, and configures direction.
func (c *B140MotionController) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.status.Connected {
		c.mu.Unlock()
		return nil
	}
	if c.connecting {
		c.mu.Unlock()
		return fmt.Errorf("controller connection already in progress")
	}
	address := fmt.Sprintf("%s:%d", c.profile.Address, c.profile.Port)
	c.connecting = true
	c.mu.Unlock()

	// ADR-009 R0-7：原实现 DialContext(ctx, ...) 在 ctx 无 deadline 时 Dial 永久阻塞
	// （Windows 故障机器 net 库 deadline 不可靠）。
	// context.WithTimeout 内部用 time.AfterFunc，time.AfterFunc 触发时 close ctx.Done()
	// channel——这是 Go runtime 保证的，不依赖 net 库 deadline。DialContext 监听
	// ctx.Done()，timeout 后立即返回 ctx.Err()。DialContext 内部异步等待 dial 完成后
	// Close conn，保证晚到 conn 不泄漏（与 sharedproto.DialTCP 的 abandoned 信号等价）。
	// 若 ctx 已有 deadline，WithTimeout 自动取较短者。
	dialCtx, cancel := context.WithTimeout(ctx, b140CommandTimeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
	if err != nil {
		c.mu.Lock()
		c.connecting = false
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	if c.status.Connected {
		c.connecting = false
		c.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	c.connMu.Lock()
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.connMu.Unlock()
	c.directionSignature = ""

	dirCmds := c.buildDirectionCommandsLocked()
	dirSignature := c.directionConfigSignatureLocked()
	c.mu.Unlock()

	if _, err := c.sendCommand(ctx, "SH"); err != nil {
		return c.connectCleanup(conn, err)
	}
	for _, cmd := range dirCmds {
		if _, err := c.sendCommand(ctx, cmd); err != nil {
			return c.connectCleanup(conn, err)
		}
	}

	c.mu.Lock()
	c.directionSignature = dirSignature
	c.status.Connected = true
	c.connecting = false
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	c.mu.Unlock()
	return nil
}

func (c *B140MotionController) buildDirectionCommandsLocked() []string {
	sig := c.directionConfigSignatureLocked()
	if c.directionSignature == sig {
		return nil
	}
	var cmds []string
	for _, ac := range c.profile.Axes {
		if !ac.Enabled {
			continue
		}
		physical, _, err := b140PhysicalAxis(ac.Name)
		if err != nil {
			continue
		}
		mt := 2
		ce := 0
		if ac.Inverted {
			mt = -2
			ce = 2
		}
		cmds = append(cmds,
			fmt.Sprintf("MT%s=%d", physical, mt),
			fmt.Sprintf("CE%s=%d", physical, ce),
		)
	}
	return cmds
}

func (c *B140MotionController) connectCleanup(conn net.Conn, err error) error {
	c.connMu.Lock()
	_ = conn.Close()
	c.conn = nil
	c.reader = nil
	c.connMu.Unlock()
	c.mu.Lock()
	c.connecting = false
	c.directionSignature = ""
	c.status.LastError = err.Error()
	c.mu.Unlock()
	return err
}

// Disconnect closes TCP connection and clears cached hardware state.
//
// ADR-009 修复：不再先发 ST 命令再 Close（原实现 sendCommandLocked("ST") 在
// sendCommand 卡死时会死锁）。TCP FIN 足以让 B140 停止运动。
// 改为：锁内取 conn 引用 + 置 nil + 置 c.status.Connected=false，锁外 conn.Close()。
// 若 sendCommand 正在阻塞，Disconnect 等 connMu 的时长由 sendCommand 的 watchdog 兜底
// （最长 b140CommandTimeout），不会无限等待。
func (c *B140MotionController) Disconnect(ctx context.Context) error {
	// 先取消所有补偿任务，避免 disconnect 后 goroutine 还在跑命令。
	c.cancelAllCompensation("controller disconnecting")

	// 1. 先失效 status（under c.mu），让并发 Status 查询快速返回 not connected。
	c.mu.Lock()
	c.status.Connected = false
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
		c.status.Axes[i].Compensating = false
		c.status.Axes[i].CompensationError = ""
		c.status.Axes[i].PositionError = 0
	}
	c.mu.Unlock()

	// 2. 锁内取 conn 引用 + 置 nil（under connMu）。
	// 若 sendCommand 正在阻塞持 connMu，此处等待由 sendCommand 的 watchdog 兜底，
	// 最长 b140CommandTimeout；watchdog 触发后 sendCommand 失效连接并释放 connMu。
	c.connMu.Lock()
	conn := c.conn
	c.conn = nil
	c.reader = nil
	c.connMu.Unlock()

	// 3. 锁外 Close：避免与 sendCommand 的 connMu 竞争。
	// TCP FIN 足以让 B140 停止运动，不需要先发 ST 命令（原实现的死锁链根因）。
	var err error
	if conn != nil {
		err = conn.Close()
	}

	// 4. 清理 directionSignature 与 LastError（under c.mu）。
	c.mu.Lock()
	c.directionSignature = ""
	if err != nil {
		c.status.LastError = err.Error()
	}
	c.mu.Unlock()
	return err
}

func (c *B140MotionController) rebuildStatusAxesLocked() {
	existing := make(map[core.AxisName]core.AxisStatus, len(c.status.Axes))
	for _, axisStatus := range c.status.Axes {
		existing[axisStatus.Name] = axisStatus
	}
	axes := make([]core.AxisStatus, 0, len(c.profile.Axes))
	for _, axisCfg := range c.profile.Axes {
		if !axisCfg.Enabled {
			continue
		}
		axisStatus := existing[axisCfg.Name]
		axisStatus.Name = axisCfg.Name
		axes = append(axes, axisStatus)
	}
	c.status.Axes = axes
}

func (c *B140MotionController) statusError(err error) (core.ControllerStatus, error) {
	c.mu.Lock()
	c.status.LastError = err.Error()
	status := c.copyStatusLocked()
	c.mu.Unlock()
	return status, err
}

// Status 是单轮状态查询入口，使用 single-flight 合并多个并发调用者：
// 同一时刻每台控制器最多运行一轮 TD/TS/MG/TP 命令序列，后续调用者共享
// 第一轮的结果（成功或失败）。这是 spec Decision 2 "一个控制器一个采集 flight"
// 在 B140 adapter 层的最小落地。
//
// 等待者通过 <-flight.done 复用发起者结果，不重新进入 sendCommand，避免多
// 消费者放大 B140 TCP 命令数（B140 单轮 Status 包含 10 至 14 条串行命令）。
// 等待者不持 connMu 或 c.mu，因此不影响 MoveTo/Jog/Home/Stop 等命令的并发性。
//
// 两种 ctx 取消语义：
//   - 发起者 ctx 取消：queryStatus 通过 sendCommand 收到 ctx.Err()，
//     返回 (失败前缓存状态, ctx.Err())；所有等待者通过 flight.done 收到同一结果。
//   - 等待者 ctx 取消：立即返回 (c.copyStatusLocked(), ctx.Err())，
//     不影响发起者和其他等待者。返回的 status 是当前缓存快照，不是本轮采集结果，
//     调用方拿到 ctx.Err() 时只能把 status 当作"最近一次已知状态"用于诊断/兜底，
//     不能当作本轮采集结果。如需本轮结果必须用非取消的 ctx 重新调用 Status。
func (c *B140MotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.statusQueryMu.Lock()
	if active := c.statusQuery; active != nil {
		c.statusQueryMu.Unlock()
		select {
		case <-active.done:
			return active.status, active.err
		case <-ctx.Done():
			c.mu.Lock()
			status := c.copyStatusLocked()
			c.mu.Unlock()
			return status, ctx.Err()
		}
	}

	flight := &b140StatusFlight{done: make(chan struct{})}
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

// queryStatus 执行一轮真实硬件查询：发送 TD/TS/MG/TP 命令并解析结果。
// 由 Status 通过 single-flight 串行调用，禁止直接调用。
// 同时承担补偿任务激活入口：发现运动停止时把 pending 升级为 job。
func (c *B140MotionController) queryStatus(ctx context.Context) (core.ControllerStatus, error) {
	c.mu.Lock()
	if err := c.checkConnectedLocked(); err != nil {
		status := c.copyStatusLocked()
		c.mu.Unlock()
		return status, err
	}
	if err := c.ensureDirectionConfiguredLocked(ctx); err != nil {
		c.status.LastError = err.Error()
		status := c.copyStatusLocked()
		c.mu.Unlock()
		return status, err
	}
	axisConfigs := make(map[core.AxisName]core.AxisConfig, len(c.status.Axes))
	for _, a := range c.status.Axes {
		if cfg, ok := c.axisConfigLocked(a.Name); ok {
			axisConfigs[a.Name] = cfg
		}
	}
	axesSnapshot := make([]core.AxisStatus, len(c.status.Axes))
	copy(axesSnapshot, c.status.Axes)
	c.mu.Unlock()

	tdPayload, err := c.sendCommand(ctx, "TD")
	if err != nil {
		return c.statusError(err)
	}
	registerPositions, parseErr := parseB140Numbers(tdPayload)
	if parseErr != nil {
		return c.statusError(parseErr)
	}

	tsPayload, err := c.sendCommand(ctx, "TS")
	if err != nil {
		return c.statusError(err)
	}
	statusBytes, parseErr := parseB140Numbers(tsPayload)
	if parseErr != nil {
		return c.statusError(parseErr)
	}

	type axisResult struct {
		position float64
		moving   bool
		homed    bool
		posLimit bool
		negLimit bool
	}
	results := make(map[core.AxisName]axisResult)

	// encoderFault 收集编码器源轴的 TP 读取/解析故障。多轴时最后一轴的故障胜出。
	// 留空则在末尾清空 LastError，保留 Status 成功路径的原有语义。
	//
	// 已知局限：故障只进全局 ControllerStatus.LastError，AxisStatus 无 per-axis
	// error 字段，操作员需翻全局错误才能发现某轴位置是降级值。多轴时若多轴同时
	// 故障，只暴露最后一个。待引入 AxisStatus.EncoderFault 字段后可逐轴精确暴露
	//（当前不动 core types 以避免波及 bindings/前端）。
	var encoderFault string

	for _, axisSnapshot := range axesSnapshot {
		axisCfg, ok := axisConfigs[axisSnapshot.Name]
		if !ok {
			continue
		}
		physical, axisIndex, err := b140PhysicalAxis(axisSnapshot.Name)
		if err != nil {
			continue
		}

		registerPulse := numberAt(registerPositions, axisIndex)
		position := core.PulseToEngineering(axisCfg, registerPulse)
		positionSourceReadValid := true

		if axisCfg.PositionSource == core.PositionSourceEncoder {
			payload, encErr := c.sendCommand(ctx, "TP"+physical)
			if encErr != nil {
				positionSourceReadValid = false
				// 编码器读取失败不静默回退到寄存器位置：寄存器与编码器在补偿
				// 启用时本就允许有偏差，静默替换会向操作员呈现一个看似合理
				// 但失真的位置。收集错误使故障可见，位置降级为寄存器换算值。
				encoderFault = fmt.Sprintf("axis %s encoder read (TP%s): %v", axisSnapshot.Name, physical, encErr)
			} else if encoderCount, parseErr2 := strconv.ParseFloat(strings.TrimSpace(payload), 64); parseErr2 == nil {
				position = core.EncoderCountToEngineering(axisCfg, encoderCount)
			} else {
				positionSourceReadValid = false
				encoderFault = fmt.Sprintf("axis %s encoder parse (TP%s payload %q): %v", axisSnapshot.Name, physical, payload, parseErr2)
			}
		}

		forwardPayload, fwErr := c.sendCommand(ctx, "MG _LF"+physical)
		if fwErr != nil {
			continue
		}

		reversePayload, rvErr := c.sendCommand(ctx, "MG _LR"+physical)
		if rvErr != nil {
			continue
		}

		moving := int(numberAt(statusBytes, axisIndex))&0x80 != 0
		if axisSnapshot.Moving && !moving && positionSourceReadValid {
			// TD/TP is read before TS, so the axis can stop between the position
			// and motion-state queries. Refresh the configured position source to
			// keep a stopped snapshot from carrying an earlier in-motion position.
			if axisCfg.PositionSource == core.PositionSourceEncoder {
				payload, refreshErr := c.sendCommand(ctx, "TP"+physical)
				if refreshErr != nil {
					return c.statusError(fmt.Errorf("axis %s final encoder read (TP%s): %w", axisSnapshot.Name, physical, refreshErr))
				}
				encoderCount, refreshErr := strconv.ParseFloat(strings.TrimSpace(payload), 64)
				if refreshErr != nil {
					return c.statusError(fmt.Errorf("axis %s final encoder parse (TP%s payload %q): %w", axisSnapshot.Name, physical, payload, refreshErr))
				}
				position = core.EncoderCountToEngineering(axisCfg, encoderCount)
			} else {
				payload, refreshErr := c.sendCommand(ctx, "TD")
				if refreshErr != nil {
					return c.statusError(fmt.Errorf("axis %s final register read: %w", axisSnapshot.Name, refreshErr))
				}
				positions, refreshErr := parseB140Numbers(payload)
				if refreshErr != nil {
					return c.statusError(fmt.Errorf("axis %s final register parse: %w", axisSnapshot.Name, refreshErr))
				}
				position = core.PulseToEngineering(axisCfg, numberAt(positions, axisIndex))
			}
		}

		results[axisSnapshot.Name] = axisResult{
			position: position,
			moving:   moving,
			homed:    core.IsHomed(position, axisCfg),
			posLimit: parseB140Limit(forwardPayload),
			negLimit: parseB140Limit(reversePayload),
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.status.Axes {
		if r, ok := results[c.status.Axes[i].Name]; ok {
			c.status.Axes[i].Position = r.position
			c.status.Axes[i].Moving = r.moving
			c.status.Axes[i].Homed = r.homed
			c.status.Axes[i].PosLimit = r.posLimit
			c.status.Axes[i].NegLimit = r.negLimit

			// 总是调 maybeActivate：内部根据 runtimeMoving 决定是更新 observedMotion
			// 还是激活/丢弃 pending。早期版本仅在 !moving 时调用，导致 observedMotion
			// 永远不会被设置，pending 永远激活不了。
			physical, _, _ := b140PhysicalAxis(c.status.Axes[i].Name)
			c.maybeActivatePendingCompensationLocked(c.status.Axes[i].Name, physical, r.moving)
		}
	}

	// 同步补偿任务状态到 axis status，并按补偿中状态覆盖 moving 字段。
	c.syncCompensationStatusLocked()
	// 编码器故障优先于"成功清空"语义：有故障则暴露，无故障才清空历史错误。
	c.status.LastError = encoderFault
	return c.copyStatusLocked(), nil
}

// MoveTo moves an axis to an absolute engineering position.
func (c *B140MotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	// 先取消该轴旧补偿任务，避免与新命令竞争。
	c.cancelAxisCompensation(axis, "replaced by new MoveTo command")

	c.mu.Lock()
	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if err := validateB140Target(axisCfg, position); err != nil {
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return err
	}
	pulse := core.EngineeringToPulse(axisCfg, position)
	c.mu.Unlock()

	if err := c.applyAxisSpeed(ctx, axisCfg, physical); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, fmt.Sprintf("PA%s=%d", physical, pulse)); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, "BG"+physical); err != nil {
		c.recordLastError(err)
		return err
	}

	// 入队 pending 补偿请求，由 Status 轮询激活。
	c.enqueuePendingCompensation(axis, physical, axisCfg, pulse, position)
	c.recordLastError(nil)
	return nil
}

// MoveBy moves an axis by a relative engineering delta.
func (c *B140MotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	c.cancelAxisCompensation(axis, "replaced by new MoveBy command")

	c.mu.Lock()
	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	current, err := c.readAxisPosition(ctx, axisCfg, physical)
	if err != nil {
		c.recordLastError(err)
		return err
	}

	c.mu.Lock()
	if err := c.validateB140RelativeMoveLocked(axis, delta, current+delta); err != nil {
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	deltaPulse := core.EngineeringToPulse(axisCfg, delta)
	if deltaPulse == 0 {
		return nil
	}
	if err := c.applyAxisSpeed(ctx, axisCfg, physical); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, fmt.Sprintf("PR%s=%d", physical, deltaPulse)); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, "BG"+physical); err != nil {
		c.recordLastError(err)
		return err
	}

	// 相对运动的目标 = 当前位置 + delta；用工程位置反算目标脉冲避免累积误差。
	targetPulse := core.EngineeringToPulse(axisCfg, current+delta)
	c.enqueuePendingCompensation(axis, physical, axisCfg, targetPulse, current+delta)
	c.recordLastError(nil)
	return nil
}

// Jog moves one engineering unit in the velocity direction.
// Jog 不入队补偿：单步 jog 后用户通常会再发 jog 或 stop，补偿会与新命令竞争。
func (c *B140MotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	c.cancelAxisCompensation(axis, "replaced by new Jog command")

	c.mu.Lock()
	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	maxSpeed := core.ValueOrFloat(axisCfg.MaxSpeed, 10)
	jogSpeed := math.Abs(velocity)
	if jogSpeed > maxSpeed {
		jogSpeed = maxSpeed
	}
	if jogSpeed == 0 {
		jogSpeed = maxSpeed
	}
	pulseSpeed := core.EngineeringToPulse(axisCfg, jogSpeed)
	step := 1.0
	if velocity < 0 {
		step = -1
	}
	current, err := c.readAxisPosition(ctx, axisCfg, physical)
	if err != nil {
		c.recordLastError(err)
		return err
	}

	c.mu.Lock()
	if err := c.validateB140RelativeMoveLocked(axis, step, current+step); err != nil {
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	stepPulse := core.EngineeringToPulse(axisCfg, step)

	if _, err := c.sendCommand(ctx, fmt.Sprintf("SP%s=%d", physical, pulseSpeed)); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, fmt.Sprintf("PR%s=%d", physical, stepPulse)); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, "BG"+physical); err != nil {
		c.recordLastError(err)
		return err
	}
	c.recordLastError(nil)
	return nil
}

// Home starts the Galil home mode on one axis.
// Home 后坐标系会被重定义，pending 补偿的目标位置失效，必须取消。
func (c *B140MotionController) Home(ctx context.Context, axis core.AxisName) error {
	c.cancelAxisCompensation(axis, "replaced by new Home command")

	c.mu.Lock()
	_, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	if _, err := c.sendCommand(ctx, "HM"+physical); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, "BG"+physical); err != nil {
		c.recordLastError(err)
		return err
	}
	c.recordLastError(nil)
	return nil
}

// Stop decelerates either one axis or all axes when axis is empty.
func (c *B140MotionController) Stop(ctx context.Context, axis core.AxisName) error {
	if axis == "" {
		c.cancelAllCompensation("Stop all axes")
	} else {
		c.cancelAxisCompensation(axis, "Stop axis")
	}

	c.mu.Lock()
	if err := c.checkConnectedLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	cmd := "ST"
	if axis != "" {
		physical, _, err := b140PhysicalAxis(axis)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		if _, ok := c.axisConfigLocked(axis); !ok {
			c.mu.Unlock()
			return fmt.Errorf("unknown motion axis: %s", axis)
		}
		cmd += physical
	}
	c.mu.Unlock()

	if _, err := c.sendCommand(ctx, cmd); err != nil {
		c.recordLastError(err)
		return err
	}
	c.recordLastError(nil)
	return nil
}

// EmergencyStop aborts all motion immediately.
func (c *B140MotionController) EmergencyStop(ctx context.Context) error {
	c.cancelAllCompensation("EmergencyStop")

	c.mu.Lock()
	if err := c.checkConnectedLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	if _, err := c.sendCommand(ctx, "AB"); err != nil {
		c.recordLastError(err)
		return err
	}

	c.mu.Lock()
	c.status.EmergencyStopped = true
	c.status.LastError = ""
	c.mu.Unlock()
	return nil
}

// ResetEmergencyStop re-enables servo output and refreshes direction config.
func (c *B140MotionController) ResetEmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	if err := c.checkConnectedLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	if _, err := c.sendCommand(ctx, "SH"); err != nil {
		c.recordLastError(err)
		return err
	}

	c.mu.Lock()
	c.directionSignature = ""
	c.mu.Unlock()

	// 重新下方向配置（不持 mu）
	c.mu.Lock()
	for _, axisCfg := range c.profile.Axes {
		if !axisCfg.Enabled {
			continue
		}
		physical, _, err := b140PhysicalAxis(axisCfg.Name)
		if err != nil {
			continue
		}
		motorDirection := 2
		encoderDirection := 0
		if axisCfg.Inverted {
			motorDirection = -2
			encoderDirection = 2
		}
		c.mu.Unlock()
		if _, err := c.sendCommand(ctx, fmt.Sprintf("MT%s=%d", physical, motorDirection)); err != nil {
			c.recordLastError(err)
			return err
		}
		if _, err := c.sendCommand(ctx, fmt.Sprintf("CE%s=%d", physical, encoderDirection)); err != nil {
			c.recordLastError(err)
			return err
		}
		c.mu.Lock()
	}
	c.mu.Unlock()

	c.mu.Lock()
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	c.mu.Unlock()
	return nil
}

// DefinePosition sets both register and encoder position counters.
// DefinePosition 后坐标系重定义，pending 补偿的目标失效，必须取消。
func (c *B140MotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	c.cancelAxisCompensation(axis, "replaced by DefinePosition")

	c.mu.Lock()
	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	pulse := core.EngineeringToPulse(axisCfg, position)
	encoderCount := core.EngineeringToEncoderCount(axisCfg, position)
	if _, err := c.sendCommand(ctx, fmt.Sprintf("DP%s=%d", physical, pulse)); err != nil {
		c.recordLastError(err)
		return err
	}
	if _, err := c.sendCommand(ctx, fmt.Sprintf("DE%s=%d", physical, encoderCount)); err != nil {
		c.recordLastError(err)
		return err
	}
	c.recordLastError(nil)
	return nil
}

// ---------- 补偿任务管理 ----------

// enqueuePendingCompensation 入队 pending 补偿请求。
// 调用时机：MoveTo/MoveBy 下发 BG 之后立即调用，由 Status 轮询发现运动停止后激活。
// 若该轴未启用补偿则直接返回，不污染 map。
func (c *B140MotionController) enqueuePendingCompensation(axis core.AxisName, physical string, axisCfg core.AxisConfig, targetPulse int64, targetEngineering float64) {
	resolved := core.ResolveEncoderCompensation(axisCfg.EncoderCompensation)
	if !resolved.Enabled {
		return
	}
	// 编码器位置源才需要补偿：寄存器位置源下没有独立反馈，补偿无意义。
	if axisCfg.PositionSource != core.PositionSourceEncoder {
		return
	}

	c.compMu.Lock()
	defer c.compMu.Unlock()
	c.pendingRequests[axis] = &b140PendingCompensationRequest{
		targetPulse:       targetPulse,
		targetEngineering: targetEngineering,
		cfg:               resolved,
		issuedAt:          time.Now(),
		observedMotion:    false,
	}
}

// maybeActivatePendingCompensationLocked 在 Status() 中调用，发现硬件运动停止时
// 把 pending 升级为 job 并启动补偿 goroutine。
// 调用方必须持有 c.mu（用于读 c.status.Axes）但不持有 c.compMu——本方法内部加 compMu。
//
// 激活规则（防静止轴误触发）：
//   - 硬件正在运动：标记 observedMotion=true，等下一次轮询
//   - 硬件已停 + 已观察到运动：立即激活
//   - 硬件已停 + 未观察到运动 + 仍在启动宽限期：保留 pending，等下一次轮询（给硬件反应时间）
//   - 硬件已停 + 未观察到运动 + 超过启动宽限期：判定为静止轴误触发，丢弃 pending
func (c *B140MotionController) maybeActivatePendingCompensationLocked(axis core.AxisName, physical string, runtimeMoving bool) {
	c.compMu.Lock()
	defer c.compMu.Unlock()

	pending, ok := c.pendingRequests[axis]
	if !ok {
		return
	}

	if runtimeMoving {
		// 硬件还在运动：记录已观察到运动，等真正停下来再激活。
		pending.observedMotion = true
		return
	}

	// 硬件已停。判定是否应激活或丢弃。
	elapsed := time.Since(pending.issuedAt)
	if !pending.observedMotion {
		if elapsed <= b140CompensationStartupGrace {
			// 仍在启动宽限期内：硬件可能只是还没开始动，保留 pending 等下次轮询
			return
		}
		// 超过宽限期仍未观察到运动：判定为静止轴误触发，丢弃
		delete(c.pendingRequests, axis)
		return
	}

	// 已观察到运动且硬件已停：升级为 job
	c.compensationGenerationCounter++
	job := &b140CompensationJob{
		generation:        c.compensationGenerationCounter,
		axis:              axis,
		physical:          physical,
		axisCfg:           c.snapshotAxisCfgLocked(axis),
		cfg:               pending.cfg,
		targetPulse:       pending.targetPulse,
		targetEngineering: pending.targetEngineering,
		state:             compensationStateWaitingStop,
		startedAt:         time.Now(),
	}
	delete(c.pendingRequests, axis)
	c.jobs[axis] = job

	// 异步启动补偿状态机。注意：不能持 c.mu 跑循环，否则会死锁。
	go c.runAxisCompensation(job)
}

// snapshotAxisCfgLocked 取轴配置快照。调用方持 c.mu。
func (c *B140MotionController) snapshotAxisCfgLocked(axis core.AxisName) core.AxisConfig {
	for _, a := range c.profile.Axes {
		if a.Enabled && a.Name == axis {
			return a
		}
	}
	return core.AxisConfig{}
}

// runAxisCompensation 补偿状态机主循环。在独立 goroutine 中运行。
// 通过 generation 与新命令通信：若 generation 已被取消则自废退出。
func (c *B140MotionController) runAxisCompensation(job *b140CompensationJob) {
	timeoutAt := job.startedAt.Add(time.Duration(job.cfg.TimeoutMs) * time.Millisecond)
	ctx := context.Background()

	for c.isCompensationJobCurrent(job.axis, job.generation) {
		switch job.currentState() {
		case compensationStateWaitingStop:
			snap, err := c.waitForAxisStop(ctx, job)
			if err != nil {
				c.failCompensationJob(job, err)
				return
			}
			if snap.limit.forward || snap.limit.reverse {
				c.failCompensationJob(job, fmt.Errorf("axis %s limit triggered during compensation: forward=%v reverse=%v", job.axis, snap.limit.forward, snap.limit.reverse))
				return
			}
			if !c.isCompensationJobCurrent(job.axis, job.generation) {
				return
			}
			job.setState(compensationStateSettling)

		case compensationStateSettling:
			if job.cfg.SettleMs > 0 {
				select {
				case <-time.After(time.Duration(job.cfg.SettleMs) * time.Millisecond):
				case <-ctx.Done():
					c.failCompensationJob(job, ctx.Err())
					return
				}
			}
			if time.Now().After(timeoutAt) {
				c.failCompensationJob(job, fmt.Errorf("compensation timed out after %dms", job.cfg.TimeoutMs))
				return
			}
			job.setState(compensationStateChecking)

		case compensationStateChecking:
			encoderCount, err := c.readAxisEncoderPosition(ctx, job.physical)
			if err != nil {
				c.failCompensationJob(job, fmt.Errorf("read encoder position: %w", err))
				return
			}
			encoderEngineering := core.EncoderCountToEngineering(job.axisCfg, encoderCount)
			errorEngineering := job.targetEngineering - encoderEngineering
			absError := math.Abs(errorEngineering)

			// 更新 PositionError（在锁内同步到 status）
			c.updateCompensationFields(job, func(s *core.AxisStatus) {
				s.PositionError = absError
			})

			if absError <= job.cfg.Tolerance || absError <= job.cfg.MinStep {
				// 补偿到位：用编码器位置重定义寄存器位置，消除静态偏差。
				if _, err := c.sendCommand(ctx, fmt.Sprintf("DP%s=%d", job.physical, core.EngineeringToPulse(job.axisCfg, encoderEngineering))); err != nil {
					c.failCompensationJob(job, fmt.Errorf("sync register position: %w", err))
					return
				}
				c.succeedCompensationJob(job)
				return
			}

			if job.attempts >= job.cfg.MaxCycles {
				c.failCompensationJob(job, fmt.Errorf("compensation exceeded max cycles %d (last error=%.6f)", job.cfg.MaxCycles, absError))
				return
			}

			job.setState(compensationStateCompensating)

		case compensationStateCompensating:
			job.attempts++
			// 重新读一次编码器位置以避免使用陈旧数据。
			// 读取失败必须终止补偿：若静默回退为 0，correctionEngineering 会
			// 等于整段目标位置，PR+BG 会把轴推向绝对目标，造成机械猛冲。
			currentCount, err := c.readAxisEncoderPosition(ctx, job.physical)
			if err != nil {
				c.failCompensationJob(job, fmt.Errorf("re-read encoder position: %w", err))
				return
			}
			correctionEngineering := job.targetEngineering - core.EncoderCountToEngineering(job.axisCfg, currentCount)
			absCorrection := math.Abs(correctionEngineering)
			if absCorrection < job.cfg.MinStep {
				// 误差小于最小步长，强制用 minStep 方向补偿，避免无穷小步进。
				if correctionEngineering >= 0 {
					correctionEngineering = job.cfg.MinStep
				} else {
					correctionEngineering = -job.cfg.MinStep
				}
			}
			correctionPulse := core.EngineeringToPulse(job.axisCfg, correctionEngineering)
			if correctionPulse == 0 {
				// 工程位置变化太小无法转换为脉冲，认为已到位。
				c.succeedCompensationJob(job)
				return
			}

			if _, err := c.sendCommand(ctx, fmt.Sprintf("PR%s=%d", job.physical, correctionPulse)); err != nil {
				c.failCompensationJob(job, fmt.Errorf("send correction PR: %w", err))
				return
			}
			if _, err := c.sendCommand(ctx, "BG"+job.physical); err != nil {
				c.failCompensationJob(job, fmt.Errorf("begin correction: %w", err))
				return
			}
			job.setState(compensationStateWaitingStop)

		default:
			// succeeded / failed / cancelled：直接退出
			return
		}
	}
}

// axisStopSnapshot 单次轮询读出的运动/限位状态。
type axisStopSnapshot struct {
	moving bool
	limit  struct {
		forward bool
		reverse bool
	}
}

// waitForAxisStop 轮询 TS 命令直到运动停止或超时。
// 返回最后一次读到的快照。
func (c *B140MotionController) waitForAxisStop(ctx context.Context, job *b140CompensationJob) (axisStopSnapshot, error) {
	var last axisStopSnapshot
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(job.cfg.TimeoutMs)*time.Millisecond)
	defer cancel()

	ticker := time.NewTicker(b140CompensationPollInterval)
	defer ticker.Stop()

	for {
		tsPayload, err := c.sendCommand(timeoutCtx, "TS")
		if err != nil {
			return last, fmt.Errorf("poll TS: %w", err)
		}
		statusBytes, parseErr := parseB140Numbers(tsPayload)
		if parseErr != nil {
			return last, fmt.Errorf("parse TS: %w", parseErr)
		}
		_, axisIndex, err := b140PhysicalAxis(job.axis)
		if err != nil {
			return last, err
		}
		last.moving = int(numberAt(statusBytes, axisIndex))&0x80 != 0

		// 同步限位
		forwardPayload, fwErr := c.sendCommand(timeoutCtx, "MG _LF"+job.physical)
		if fwErr == nil {
			last.limit.forward = parseB140Limit(forwardPayload)
		}
		reversePayload, rvErr := c.sendCommand(timeoutCtx, "MG _LR"+job.physical)
		if rvErr == nil {
			last.limit.reverse = parseB140Limit(reversePayload)
		}

		if !last.moving {
			return last, nil
		}
		if last.limit.forward || last.limit.reverse {
			return last, nil
		}

		select {
		case <-ticker.C:
		case <-timeoutCtx.Done():
			return last, fmt.Errorf("wait for axis stop: %w", timeoutCtx.Err())
		}
	}
}

// readAxisEncoderPosition 读取单轴编码器位置（TP 命令）。
func (c *B140MotionController) readAxisEncoderPosition(ctx context.Context, physical string) (float64, error) {
	payload, err := c.sendCommand(ctx, "TP"+physical)
	if err != nil {
		return 0, err
	}
	count, err := strconv.ParseFloat(strings.TrimSpace(payload), 64)
	if err != nil {
		return 0, fmt.Errorf("parse encoder count %q: %w", payload, err)
	}
	return count, nil
}

// isCompensationJobCurrent 判断 job 是否仍是当前代际。
// 调用方不持 c.mu；本方法只持 c.compMu。
func (c *B140MotionController) isCompensationJobCurrent(axis core.AxisName, generation int64) bool {
	c.compMu.Lock()
	defer c.compMu.Unlock()
	current, ok := c.jobs[axis]
	if !ok {
		return false
	}
	return current.generation == generation
}

// cancelAxisCompensation 取消指定轴的补偿任务。
//
// 实现说明：
//   - pending 直接从 map 删除
//   - job 设置 cancelled 状态后从 map 删除；运行中的 goroutine 通过
//     isCompensationJobCurrent 检测到 jobs[axis] 不存在自动退出
//   - compensationGenerationCounter++ 并非 invalidation 必需——delete 已让旧
//     job 失效——但保留递增作为：(1) 单调失效令牌，保证任何持有旧 generation
//     的 goroutine 即便遇到 map entry 被新 job 重写也能识别；(2) 测试可观测
//     信号（TestB140CompensationNewMoveToCancelsOld 依赖此计数器验证 cancel 被触发）
//
// reason 仅用于日志可观测性，不参与控制流——便于排查"为何补偿被取消"。
func (c *B140MotionController) cancelAxisCompensation(axis core.AxisName, reason string) {
	c.compMu.Lock()
	defer c.compMu.Unlock()
	delete(c.pendingRequests, axis)
	if job, ok := c.jobs[axis]; ok {
		job.setState(compensationStateCancelled)
		c.compensationGenerationCounter++
		delete(c.jobs, axis)
		slog.Debug("b140: cancel axis compensation",
			"axis", axis,
			"reason", reason,
			"generation", job.generation)
	}
}

// cancelAllCompensation 取消所有轴的补偿任务。
//
// 实现说明：
//   - 直接清空 pending/jobs 两个 map
//   - 运行中的 goroutine 通过 isCompensationJobCurrent 检测到 jobs[axis] 不存在自动退出
//   - 不再遍历设置 cancelled 状态：旧实现中 setState 后立刻整体清空 map，
//     goroutine 永远读不到 cancelled 状态，for 循环是死代码
//   - compensationGenerationCounter 不递增：cancelAllCompensation 通常发生在
//     Disconnect/EStop/Stop all 等全局场景，不会有"新 job 立刻重写 map entry"
//     的竞态，无需失效令牌
//
// reason 仅用于日志可观测性，不参与控制流。
func (c *B140MotionController) cancelAllCompensation(reason string) {
	c.compMu.Lock()
	defer c.compMu.Unlock()
	cancellingAxes := make([]core.AxisName, 0, len(c.jobs))
	for axis := range c.jobs {
		cancellingAxes = append(cancellingAxes, axis)
	}
	c.jobs = make(map[core.AxisName]*b140CompensationJob)
	c.pendingRequests = make(map[core.AxisName]*b140PendingCompensationRequest)
	slog.Debug("b140: cancel all compensation",
		"axes", cancellingAxes,
		"reason", reason)
}

// succeedCompensationJob 标记补偿成功并清理 job。
func (c *B140MotionController) succeedCompensationJob(job *b140CompensationJob) {
	c.compMu.Lock()
	if current, ok := c.jobs[job.axis]; ok && current.generation == job.generation {
		job.setState(compensationStateSucceeded)
		delete(c.jobs, job.axis)
	}
	c.compMu.Unlock()

	// 同步 status
	c.mu.Lock()
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == job.axis {
			c.status.Axes[i].Compensating = false
			c.status.Axes[i].CompensationError = ""
			c.status.Axes[i].PositionError = 0
		}
	}
	c.mu.Unlock()
}

// failCompensationJob 标记补偿失败并清理 job，把错误透传到 status。
func (c *B140MotionController) failCompensationJob(job *b140CompensationJob, err error) {
	c.compMu.Lock()
	if current, ok := c.jobs[job.axis]; ok && current.generation == job.generation {
		job.mu.Lock()
		job.state = compensationStateFailed
		job.lastError = err.Error()
		job.mu.Unlock()
		delete(c.jobs, job.axis)
	}
	c.compMu.Unlock()

	c.mu.Lock()
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == job.axis {
			c.status.Axes[i].Compensating = false
			c.status.Axes[i].CompensationError = err.Error()
		}
	}
	c.mu.Unlock()
}

// updateCompensationFields 在锁内更新 axis status 上的补偿字段。
func (c *B140MotionController) updateCompensationFields(job *b140CompensationJob, mutate func(*core.AxisStatus)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == job.axis {
			mutate(&c.status.Axes[i])
			return
		}
	}
}

// syncCompensationStatusLocked 把 compMu 下的 job 状态同步到 c.status.Axes。
// 调用方持 c.mu；本方法内部加 c.compMu。
// 关键副作用：补偿中的轴 Moving 强制为 true，避免上层误判运动完成。
func (c *B140MotionController) syncCompensationStatusLocked() {
	c.compMu.Lock()
	defer c.compMu.Unlock()
	for axis, job := range c.jobs {
		state, lastError := job.statusSnapshot()
		for i := range c.status.Axes {
			if c.status.Axes[i].Name != axis {
				continue
			}
			c.status.Axes[i].Compensating = state == compensationStateWaitingStop ||
				state == compensationStateSettling ||
				state == compensationStateChecking ||
				state == compensationStateCompensating
			c.status.Axes[i].CompensationError = lastError
			if c.status.Axes[i].Compensating {
				// 补偿中：Moving 强制 true，防止上层 waitForMotionComplete 误判
				c.status.Axes[i].Moving = true
			}
		}
	}
}

// ---------- 命令发送 ----------

// b140SendResult 是 sendCommand I/O goroutine 的返回结果。
// soft=true 表示设备返回 "?" 拒绝命令（协议级软错误），连接仍可用，不应失效。
// soft=false 表示 I/O 级硬错误（Write/Read 失败、watchdog 触发），连接不可靠，应失效。
type b140SendResult struct {
	payload string
	err     error
	soft    bool
}

// invalidateConnectionLocked 失效当前连接：置 c.conn=nil + c.reader=nil（under connMu）
// 并标记 c.status.Connected=false（under c.mu）。
//
// 调用方必须持有 c.mu；本方法内部获取 c.connMu。
// 锁顺序：c.mu（caller）→ c.connMu（本方法）—— 与全局锁顺序一致，不会死锁。
//
// 设计说明：不直接在 sendCommand 持 connMu 时调用本方法，因为 sendCommand 持 connMu
// 再获取 c.mu 会形成 c.connMu → c.mu 的反向锁顺序，与 checkConnectedLocked /
// Connect 等持 c.mu 后获取 c.connMu 的路径成环。sendCommand 在调用本方法前先释放
// connMu，再由调用方持 c.mu 调用本方法，保持正向锁顺序。
//
// ADR-009 finding 2 修复：expectedConn 比较避免误杀新连接。
// 调用方必须在触发故障前捕获 c.conn，并传入此参数。仅当 c.conn 仍是 expectedConn
// 时才清空 c.conn/c.reader 并置 Connected=false；若 c.conn 已被 Disconnect -> Connect
// 替换为新连接，仅关闭旧 expectedConn，不修改状态，避免旧命令的 invalidation 误杀新连接。
func (c *B140MotionController) invalidateConnectionLocked(expectedConn net.Conn, message string) {
	c.connMu.Lock()
	currentConn := c.conn
	if currentConn != expectedConn {
		// c.conn 已被替换为新连接（Disconnect -> Connect）或已置 nil，
		// 旧命令的 invalidation 不应误杀新连接。仅关闭旧 expectedConn，不修改状态。
		c.connMu.Unlock()
		if expectedConn != nil {
			_ = expectedConn.Close()
		}
		return
	}
	c.conn = nil
	c.reader = nil
	c.connMu.Unlock()
	c.status.Connected = false
	if message != "" {
		c.status.LastError = message
	}
	if expectedConn != nil {
		_ = expectedConn.Close()
	}
}

// sendCommand 发送命令并读取响应。不要求调用方持 c.mu，仅持 c.connMu。
// 补偿 goroutine 与运动命令共用此入口。
//
// ADR-009 修复：除 SetDeadline 软超时外，独立 watchdog 在 timeout 后强制 Close conn，
// 避免故障 Windows 电脑上 deadline 失效导致 Read 永久阻塞，进而引发 Disconnect 死锁。
// watchdog 触发或 I/O 硬错误后失效连接（c.conn=nil + c.status.Connected=false），
// 让后续命令立即返回 "not connected"，避免向已关闭的 conn 写入。
//
// ctx 取消分支不再无界等 <-done：watchdog 在 watchdogTimeout 后 Close conn，
// <-done 此时为有界等待（最长 watchdogTimeout）。返回 I/O 错误（含 watchdog 上下文）
// 或 ctx.Err()。
func (c *B140MotionController) sendCommand(ctx context.Context, cmd string) (string, error) {
	c.connMu.Lock()

	if c.conn == nil || c.reader == nil {
		c.connMu.Unlock()
		return "", fmt.Errorf("controller not connected")
	}

	// 计算 watchdog 超时：取 b140CommandTimeout 与 ctx 剩余 deadline 的较小值。
	// ctx 无 deadline 时用 b140CommandTimeout。
	deadline := time.Now().Add(b140CommandTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	watchdogTimeout := time.Until(deadline)
	if watchdogTimeout <= 0 {
		// ctx 已过期，直接返回，不启动 watchdog
		c.connMu.Unlock()
		return "", ctx.Err()
	}

	if err := c.conn.SetDeadline(deadline); err != nil {
		message := fmt.Sprintf("SetDeadline failed: %v", err)
		// ADR-009 finding 2：捕获 expectedConn，仅当 c.conn 仍是此 conn 时才清状态。
		expectedConn := c.conn
		c.connMu.Unlock()
		c.mu.Lock()
		c.invalidateConnectionLocked(expectedConn, message)
		c.mu.Unlock()
		return "", err
	}

	conn := c.conn
	reader := c.reader

	// 启动独立 watchdog：超时后强制 Close conn，解除 Read 阻塞。
	// 即使 SetDeadline 在故障 Windows 上失效，watchdog 也能保证 connMu 在有界时间内释放。
	// 注意：watchdog 内部 Close conn 不需要抢 connMu（net.Conn.Close 并发安全）。
	//
	// I-1 说明：与 dsa3217 / daq_p1064pre 标杆模式（watchdog 在 ioMu.Lock 之前启动）不同，
	// b140_motion 在 connMu.Lock 之后启动 watchdog。两者差异源于锁的争用场景：
	//   - dsa3217 / daq_p1064pre：readLoop 持 ioMu 阻塞 ReadString，sendCommand 卡在
	//     ioMu.Lock 上，watchdog 必须先启动才能 Close conn 解除 readLoop 阻塞、释放 ioMu
	//   - b140_motion：无 readLoop，connMu 的所有持有者（Connect/Disconnect/sendCommand）
	//     均不在锁内做阻塞 I/O（除 sendCommand 自身的 Write/Read，但那是 watchdog 覆盖区间）
	//     因此 sendCommand 获取 connMu 的等待时间有界，watchdog 在 Lock 之后启动不会
	//     出现"等锁期间无 Close owner"的死锁窗口。
	// Disconnect 等 connMu 的最长等待时间 = sendCommand 的 watchdogTimeout（b140CommandTimeout），
	// 与标杆模式行为一致。
	wdStop := sharedproto.WatchdogClose(conn, watchdogTimeout)

	done := make(chan b140SendResult, 1)

	go func() {
		if _, wErr := conn.Write([]byte(cmd + "\r")); wErr != nil {
			done <- b140SendResult{err: wErr}
			return
		}

		var payload strings.Builder
		for {
			b, rErr := reader.ReadByte()
			if rErr != nil {
				done <- b140SendResult{err: rErr}
				return
			}
			switch b {
			case ':':
				done <- b140SendResult{payload: strings.TrimSpace(payload.String())}
				return
			case '?':
				// 设备拒绝命令（软错误）：连接仍可用，不应失效
				done <- b140SendResult{
					err:  fmt.Errorf("B140 command %q failed: %s", cmd, strings.TrimSpace(payload.String())),
					soft: true,
				}
				return
			default:
				payload.WriteByte(b)
			}
		}
	}()

	select {
	case <-ctx.Done():
		// ctx 取消：不再无界等 <-done。watchdog 在 watchdogTimeout 后 Close conn，
		// <-done 此时为有界等待（最长 watchdogTimeout）。
		r := <-done
		ok := wdStop()
		c.connMu.Unlock()
		c.applyInvalidate(r, ok, conn)
		// 优先返回 I/O 错误（含 watchdog 上下文），其次返回 ctx.Err()。
		// ctx 取消时若 I/O 已因 watchdog 触发失败，I/O 错误比 ctx.Err() 更能反映根因。
		if r.err != nil && !r.soft {
			return "", wrapB140WatchdogError(r.err, !ok)
		}
		return "", ctx.Err()
	case r := <-done:
		ok := wdStop()
		c.connMu.Unlock()
		c.applyInvalidate(r, ok, conn)
		if r.err == nil {
			// 成功路径：watchdog 未触发，conn 仍可用，清除 SetDeadline 设置的读写 deadline，
			// 避免过期的绝对时间影响下次 sendCommand（虽然下次会重新 SetDeadline 覆盖，
			// 但显式清除符合 ADR-009 决策 3"deadline 残留禁止"原则）。
			// 不需要再次抢 connMu：net.Conn.SetDeadline 并发安全，且 applyInvalidate 在
			// 成功路径不会失效 conn（conn 仍非 nil）。
			_ = conn.SetDeadline(time.Time{})
			return r.payload, nil
		}
		if r.soft {
			// 软错误（? 响应）：连接仍可用，清除 deadline 后返回错误。
			_ = conn.SetDeadline(time.Time{})
			return "", r.err
		}
		return "", wrapB140WatchdogError(r.err, !ok)
	}
}

// applyInvalidate 根据 I/O 结果和 watchdog 状态决定是否失效连接，并在持 c.mu 时调用
// invalidateConnectionLocked。调用方必须已释放 c.connMu。
//
// 失效判定：
//   - watchdog 触发（ok=false）：conn 已被 Close，必须失效
//   - 硬 I/O 错误（r.err != nil && !r.soft）：连接不可靠，失效
//   - 软错误（? 响应）或成功且 watchdog 未触发：连接仍可用，不失效
//
// ADR-009 finding 2：expectedConn 由调用方从 sendCommand 入口捕获，传给
// invalidateConnectionLocked 进行比较，避免旧命令误杀 Disconnect -> Connect 后的新连接。
func (c *B140MotionController) applyInvalidate(r b140SendResult, ok bool, expectedConn net.Conn) {
	var message string
	if !ok {
		// watchdog 已触发（conn 已 Close）
		if r.err != nil {
			message = fmt.Sprintf("%v (watchdog triggered, conn closed)", r.err)
		} else {
			message = "watchdog triggered, conn closed"
		}
	} else if r.err != nil && !r.soft {
		// 硬 I/O 错误
		message = r.err.Error()
	} else {
		// 软错误或成功且 watchdog 未触发：不失效
		return
	}
	c.mu.Lock()
	c.invalidateConnectionLocked(expectedConn, message)
	c.mu.Unlock()
}

// wrapB140WatchdogError 在 watchdog 触发时附加 "(watchdog triggered, conn closed)" 上下文，
// 便于调用方从错误信息识别 ADR-009 兜底路径。watchdogTriggered=false 时原样返回。
func wrapB140WatchdogError(err error, watchdogTriggered bool) error {
	if err == nil || !watchdogTriggered {
		return err
	}
	return fmt.Errorf("%w; %w", err, sharedproto.ErrWatchdogTriggered)
}

// sendCommandLocked 旧入口：保留给 Connect/Disconnect 等已持 c.mu 的调用点。
// 新代码应直接用 sendCommand。
func (c *B140MotionController) sendCommandLocked(ctx context.Context, cmd string) (string, error) {
	return c.sendCommand(ctx, cmd)
}

// recordLastError 把错误写到 status.LastError。nil 时清空。
func (c *B140MotionController) recordLastError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.status.LastError = ""
		return
	}
	c.status.LastError = err.Error()
}

// applyAxisSpeed 不持 c.mu 的速度下发版本。
func (c *B140MotionController) applyAxisSpeed(ctx context.Context, axisCfg core.AxisConfig, physical string) error {
	maxSpeed := core.ValueOrFloat(axisCfg.MaxSpeed, core.DefaultMaxSpeed)
	if maxSpeed <= 0 || math.IsNaN(maxSpeed) || math.IsInf(maxSpeed, 0) {
		maxSpeed = core.DefaultMaxSpeed
	}
	pulseSpeed := core.EngineeringToPulse(axisCfg, maxSpeed)
	if pulseSpeed <= 0 {
		pulseSpeed = 1
	}
	_, err := c.sendCommand(ctx, fmt.Sprintf("SP%s=%d", physical, pulseSpeed))
	return err
}

// readAxisPosition 不持 c.mu 的位置读取版本。
func (c *B140MotionController) readAxisPosition(ctx context.Context, axisCfg core.AxisConfig, physical string) (float64, error) {
	_, axisIndex, err := b140PhysicalAxis(axisCfg.Name)
	if err != nil {
		return 0, err
	}

	var position float64
	if axisCfg.PositionSource == core.PositionSourceEncoder {
		payload, err := c.sendCommand(ctx, "TP"+physical)
		if err != nil {
			return 0, err
		}
		encoderCount, err := strconv.ParseFloat(strings.TrimSpace(payload), 64)
		if err != nil {
			return 0, err
		}
		position = core.EncoderCountToEngineering(axisCfg, encoderCount)
	} else {
		payload, err := c.sendCommand(ctx, "TD")
		if err != nil {
			return 0, err
		}
		registerPositions, err := parseB140Numbers(payload)
		if err != nil {
			return 0, err
		}
		position = core.PulseToEngineering(axisCfg, numberAt(registerPositions, axisIndex))
	}

	c.mu.Lock()
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == axisCfg.Name {
			c.status.Axes[i].Position = position
			break
		}
	}
	c.mu.Unlock()
	return position, nil
}

func (c *B140MotionController) prepareAxisCommandLocked(ctx context.Context, axis core.AxisName) (core.AxisConfig, string, error) {
	if err := c.checkReadyLocked(); err != nil {
		return core.AxisConfig{}, "", err
	}
	if err := c.ensureDirectionConfiguredLocked(ctx); err != nil {
		c.status.LastError = err.Error()
		return core.AxisConfig{}, "", err
	}
	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return core.AxisConfig{}, "", fmt.Errorf("unknown motion axis: %s", axis)
	}
	physical, _, err := b140PhysicalAxis(axis)
	if err != nil {
		return core.AxisConfig{}, "", err
	}
	return axisCfg, physical, nil
}

func (c *B140MotionController) checkConnectedLocked() error {
	c.connMu.Lock()
	connOk := c.conn != nil && c.reader != nil
	c.connMu.Unlock()
	if !connOk || !c.status.Connected {
		return fmt.Errorf("controller not connected")
	}
	return nil
}

func (c *B140MotionController) checkReadyLocked() error {
	if err := c.checkConnectedLocked(); err != nil {
		return err
	}
	if c.status.EmergencyStopped {
		return fmt.Errorf("controller is in emergency stop state")
	}
	return nil
}

func (c *B140MotionController) ensureDirectionConfiguredLocked(ctx context.Context) error {
	signature := c.directionConfigSignatureLocked()
	if c.directionSignature == signature {
		return nil
	}
	// 与原实现一致：signature 失效时下发方向命令。命令在锁内执行，
	// 与 sendCommand 共用 connMu，不会与运动命令竞争。Status 路径上
	// 调用频率低（只在 signature 变化时），可接受短暂持锁。
	for _, axisCfg := range c.profile.Axes {
		if !axisCfg.Enabled {
			continue
		}
		physical, _, err := b140PhysicalAxis(axisCfg.Name)
		if err != nil {
			return err
		}
		motorDirection := 2
		encoderDirection := 0
		if axisCfg.Inverted {
			motorDirection = -2
			encoderDirection = 2
		}
		if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("MT%s=%d", physical, motorDirection)); err != nil {
			return err
		}
		if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("CE%s=%d", physical, encoderDirection)); err != nil {
			return err
		}
	}
	c.directionSignature = signature
	return nil
}

func validateB140Target(axisCfg core.AxisConfig, target float64) error {
	if math.IsNaN(target) || math.IsInf(target, 0) {
		return fmt.Errorf("invalid target position for axis %s", axisCfg.Name)
	}
	if axisCfg.MinLimit != nil && target < *axisCfg.MinLimit {
		return fmt.Errorf("target %.4f is below min limit %.4f for axis %s", target, *axisCfg.MinLimit, axisCfg.Name)
	}
	if axisCfg.MaxLimit != nil && target > *axisCfg.MaxLimit {
		return fmt.Errorf("target %.4f is above max limit %.4f for axis %s", target, *axisCfg.MaxLimit, axisCfg.Name)
	}
	return nil
}

func (c *B140MotionController) directionConfigSignatureLocked() string {
	parts := make([]string, 0, len(c.profile.Axes))
	for _, axisCfg := range c.profile.Axes {
		if axisCfg.Enabled {
			parts = append(parts, fmt.Sprintf("%s:%t", axisCfg.Name, axisCfg.Inverted))
		}
	}
	return strings.Join(parts, "|")
}

func (c *B140MotionController) axisConfigLocked(axis core.AxisName) (core.AxisConfig, bool) {
	for _, axisCfg := range c.profile.Axes {
		if axisCfg.Enabled && axisCfg.Name == axis {
			return axisCfg, true
		}
	}
	return core.AxisConfig{}, false
}

func (c *B140MotionController) validateB140RelativeMoveLocked(axis core.AxisName, delta float64, target float64) error {
	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return fmt.Errorf("unknown motion axis: %s", axis)
	}
	if err := validateB140Target(axisCfg, target); err != nil {
		return err
	}
	for _, axisStatus := range c.status.Axes {
		if axisStatus.Name != axis {
			continue
		}
		if delta > 0 && axisStatus.PosLimit {
			return fmt.Errorf("positive limit is active for axis %s", axis)
		}
		if delta < 0 && axisStatus.NegLimit {
			return fmt.Errorf("negative limit is active for axis %s", axis)
		}
		return nil
	}
	return fmt.Errorf("unknown motion axis: %s", axis)
}

func (c *B140MotionController) copyStatusLocked() core.ControllerStatus {
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	return status
}

func b140PhysicalAxis(axis core.AxisName) (string, int, error) {
	switch axis {
	case core.AxisX:
		return "A", 0, nil
	case core.AxisY:
		return "B", 1, nil
	case core.AxisZ:
		return "C", 2, nil
	case core.AxisU:
		return "D", 3, nil
	default:
		return "", 0, fmt.Errorf("unknown motion axis: %s", axis)
	}
}

func parseB140Numbers(payload string) ([]float64, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	parts := strings.Split(payload, ",")
	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func numberAt(values []float64, index int) float64 {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func parseB140Limit(payload string) bool {
	value, err := strconv.ParseFloat(strings.TrimSpace(payload), 64)
	if err != nil {
		return false
	}
	return value < 1
}
