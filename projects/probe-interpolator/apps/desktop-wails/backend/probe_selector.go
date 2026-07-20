package backend

import (
	"fmt"
	"sync"
)

// ProbeKind 标识探针类型，作为 SetActiveProbe / GetActiveProbe 的参数与返回值。
// 取值固定为 three / five / seven 三选一，前端按此字符串路由工作区。
type ProbeKind string

const (
	// ProbeKindThree 三孔探针
	ProbeKindThree ProbeKind = "three"
	// ProbeKindFive 五孔探针
	ProbeKindFive ProbeKind = "five"
	// ProbeKindSeven 七孔探针
	ProbeKindSeven ProbeKind = "seven"
)

// ProbeInfo 是启动选择页每个探针按钮的元信息，前端按此渲染卡片。
type ProbeInfo struct {
	Kind        ProbeKind `json:"kind"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	// Holes 是探针测压孔数量，仅用于卡片角标显示
	Holes int `json:"holes"`
}

// availableProbes 是启动选择页展示的探针列表。
// 顺序固定为 3 → 5 → 7，对应 SPEC § UI Design 的卡片排列。
// 注意：此切片不可变，GetAvailableProbes 每次返回副本，避免前端误改。
var availableProbes = []ProbeInfo{
	{
		Kind:        ProbeKindThree,
		Name:        "三孔探针",
		Description: "测量总压、静压、马赫数、迎角",
		Holes:       3,
	},
	{
		Kind:        ProbeKindFive,
		Name:        "五孔探针",
		Description: "测量总压、静压、马赫数、迎角、侧滑角及三维速度",
		Holes:       5,
	},
	{
		Kind:        ProbeKindSeven,
		Name:        "七孔探针",
		Description: "大角度流场测量，自动判定小角度/大角度模式",
		Holes:       7,
	},
}

// probeSelector 封装"当前激活探针类型"的并发安全状态机。
// 不直接放在 App struct 上是为了让 probe_selector.go 自带锁，
// 避免与后续 five_hole_service / three_hole_service / seven_hole_service 的锁混用。
//
// v0.1.1 起改为可切换语义：
//   - SetActiveProbe 允许覆盖式更新（用户可从工作区返回欢迎页再选其他探针）
//   - ClearActiveProbe 显式清空激活状态，配合前端"返回"按钮
//   - 各探针 service 的 .prb / 输入状态独立保留，切换探针不丢失，再次切回可恢复
type probeSelector struct {
	mu          sync.RWMutex
	activeProbe ProbeKind // 空字符串表示尚未选择（仍在欢迎页）
}

// GetAvailableProbes 返回启动选择页要展示的探针列表（3/5/7 孔）。
// 返回切片的副本，前端修改不影响后端状态。
func (a *App) GetAvailableProbes() []ProbeInfo {
	out := make([]ProbeInfo, len(availableProbes))
	copy(out, availableProbes)
	return out
}

// SetActiveProbe 设置当前会话的探针类型。
// 允许覆盖式更新：用户从工作区返回欢迎页后可再次选择其他探针类型。
// 各探针 service 的 .prb / 输入状态由各 service 自行保留，切换不会丢失。
func (a *App) SetActiveProbe(kind ProbeKind) error {
	if !isValidProbeKind(kind) {
		return fmt.Errorf("未知的探针类型: %q（有效值: three / five / seven）", kind)
	}

	a.selector.mu.Lock()
	defer a.selector.mu.Unlock()

	a.selector.activeProbe = kind
	return nil
}

// ClearActiveProbe 清空当前会话的探针类型选择。
// 配合前端"返回欢迎页"按钮：用户从工作区回到选择页后，GetActiveProbe 返回空字符串。
// 注意：本方法不清理各探针 service 的 .prb / 输入状态，用户再次进入同一探针时可恢复。
func (a *App) ClearActiveProbe() {
	a.selector.mu.Lock()
	defer a.selector.mu.Unlock()

	a.selector.activeProbe = ""
}

// GetActiveProbe 返回当前会话已选择的探针类型。
// 若用户尚未选择（仍在欢迎页），返回空字符串 + nil error，前端按空字符串判定留在选择页。
func (a *App) GetActiveProbe() (ProbeKind, error) {
	a.selector.mu.RLock()
	defer a.selector.mu.RUnlock()

	return a.selector.activeProbe, nil
}

// isValidProbeKind 检查字符串是否为合法的 ProbeKind 常量。
func isValidProbeKind(kind ProbeKind) bool {
	for _, p := range availableProbes {
		if p.Kind == kind {
			return true
		}
	}
	return false
}
