package monitor

// CommandKind 区分运动命令和普通命令，用于 NotifyCommandExecuted 触发不同 refresh 策略。
//
// 设计依据：spec Interface Contract - NotifyCommandExecuted
//   - CmdKindMove:  Move/Jog/Home，触发 2s 快速观察窗口 + 一轮额外采集
//   - CmdKindStop:  Stop/EStop/Reset，仅触发单轮 refresh（不进入快速窗口，避免 Stop 后高频采集放大硬件压力）
//   - CmdKindConfig: ApplyConfig，触发 generation 重置 + 单轮 refresh
type CommandKind int

const (
	// CmdKindMove 是 Move/Jog/Home 命令的类别。
	// 这类命令后硬件状态会快速变化，需要 2s 快速观察窗口确保 UI 及时反映到位。
	CmdKindMove CommandKind = iota
	// CmdKindStop 是 Stop/EStop/Reset 命令的类别。
	// 这类命令后硬件状态变化已完成，单轮 refresh 即可；快速窗口会放大硬件压力无意义。
	CmdKindStop
	// CmdKindConfig 是 ApplyConfig 命令的类别。
	// 配置变更使旧快照失效，必须递增 generation 让旧 generation 的在途结果被丢弃。
	CmdKindConfig
)
