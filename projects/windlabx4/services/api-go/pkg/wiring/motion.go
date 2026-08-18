// Package wiring 提供跨装配根共享的组件装配函数。
//
// 本包是 CLAUDE.md "Constraint Clarifications" 规则 2 定义的装配根延伸：
// 它同时导入 ports、adapters、usecase，把 adapter 实现装配到 usecase。
// 各装配根（appcontext、apiserver、bootstrap）和测试代码均可调用本包，
// 避免 usecase 内部反向依赖 adapters。
package wiring

import (
	sharedcore "shared.local/device-sdk/go/motion/core"
	sharedports "shared.local/device-sdk/go/motion/ports"
	motionmanager "shared.local/motion-control/go/manager"
	motionadapter "windlabx4/services/api-go/internal/adapters/motion"

	"windlabx4/services/api-go/internal/ports"
)

// NewMotionManager 装配运动管理器：构造 shared motion manager 并用 adapter 包装为 ports.MotionManager。
// profileStore 与 factory 由调用方注入（来自 shared device-sdk）。
func NewMotionManager(
	profileStore sharedports.MotionProfileStore,
	factory func(profile sharedcore.MotionControllerProfile) (sharedports.MotionController, error),
) ports.MotionManager {
	return motionadapter.WrapMotionManager(motionmanager.NewMotionManager(profileStore, factory))
}

// WrapMotionManager 将已构造的 shared motion manager 用 adapter 包装为 ports.MotionManager。
// 适用于测试或需要直接访问 raw manager 的场景。
func WrapMotionManager(raw *motionmanager.MotionManager) ports.MotionManager {
	return motionadapter.WrapMotionManager(raw)
}
