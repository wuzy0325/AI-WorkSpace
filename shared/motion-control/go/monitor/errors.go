package monitor

import "fmt"

// ErrGenerationChanged 表示 WaitNext 期间目标控制器发生 Disconnect/ApplyConfig，
// afterSequence 所属 generation 已被新 generation 替换；消费者必须显式重新决策。
//
// 语义约束：
//   - OldGen: WaitNext 调用时 afterSequence 所属的 generation
//   - NewGen: 当前 controller 的 generation
//   - 旧 generation 的 Sequence 不会被新 generation 复用
//   - ErrGenerationChanged 是单向通知，不能通过"等待更大 sequence"自动恢复
//
// 消费者处理：业务等待循环必须捕获 ErrGenerationChanged 并按安全策略 abort 当前点或重新发起等待。
type ErrGenerationChanged struct {
	ControllerID string
	OldGen       uint64
	NewGen       uint64
}

// Error 实现 error 接口。
func (e *ErrGenerationChanged) Error() string {
	return fmt.Sprintf("monitor: controller %s generation changed from %d to %d during WaitNext; afterSequence belongs to stale generation, caller must re-decide",
		e.ControllerID, e.OldGen, e.NewGen)
}

// Is 支持 errors.Is 识别。同一 ControllerID 的 ErrGenerationChanged 视为同一错误类型。
//
// 去重语义说明：
//   - ControllerID 是身份标识；OldGen/NewGen 可能因调用时机不同而不同
//   - 同一控制器在 WaitNext 期间可能多次发生 generation 变化（如连续 Disconnect），
//     每次都返回 ErrGenerationChanged，errors.Is 把它们视为"同一类错误"
//   - 消费者不能用 OldGen/NewGen 区分"哪一次"变化——如需区分请直接比较字段
//   - 业务侧典型用法：errors.Is(err, &ErrGenerationChanged{ControllerID: id})
//     判定"该控制器是否发生过 generation 变化"，而非细粒度匹配某次变化
func (e *ErrGenerationChanged) Is(target error) bool {
	other, ok := target.(*ErrGenerationChanged)
	if !ok {
		return false
	}
	return e.ControllerID == other.ControllerID
}
