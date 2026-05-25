package interpolation

// InterpolationResult 插值计算结果
type InterpolationResult struct {
	Alpha           float64 `json:"alpha"`             // 攻角 (度)
	Beta            float64 `json:"beta"`              // 侧滑角 (度)
	MachNumber      float64 `json:"machNumber"`        // 马赫数
	Velocity        float64 `json:"velocity"`          // 速度 (m/s)
	DynamicPressure float64 `json:"dynamicPressure"`   // 动压 (Pa)
	Density         float64 `json:"density"`           // 密度 (kg/m³)
	TotalPressure   float64 `json:"P0"`                // 总压 (表压 Pa)
	StaticPressure  float64 `json:"Ps"`                // 静压 (表压 Pa)
	IsValid         bool    `json:"isValid"`           // 结果是否有效
	Warning         string  `json:"warning,omitempty"` // 警告信息
}

// PrbValidRange PRB文件有效范围
type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"` // 最小攻角
	AlphaMax float64 `json:"alphaMax"` // 最大攻角
	BetaMin  float64 `json:"betaMin"`  // 最小侧滑角
	BetaMax  float64 `json:"betaMax"`  // 最大侧滑角
	MachMin  float64 `json:"machMin"`  // 最小马赫数
	MachMax  float64 `json:"machMax"`  // 最大马赫数
}

// PrbFileInfo PRB文件信息
type PrbFileInfo struct {
	FilePath   string        `json:"filePath"`   // 文件路径
	FileName   string        `json:"fileName"`   // 文件名
	LoadedAt   int64         `json:"loadedAt"`   // 加载时间戳(ms)
	ValidRange PrbValidRange `json:"validRange"` // 有效范围
}

// InterpolationInput 插值输入（五孔探针压力数据）
type InterpolationInput struct {
	P1   float64 `json:"P1"`   // 下孔压力
	P2   float64 `json:"P2"`   // 中心孔压力
	P3   float64 `json:"P3"`   // 上孔压力
	P4   float64 `json:"P4"`   // 左孔压力
	P5   float64 `json:"P5"`   // 右孔压力
	PAtm float64 `json:"Patm"` // 大气压力
	TAtm float64 `json:"Tatm"` // 大气温度
}

// Interpolator 通用插值器接口
type Interpolator interface {
	// IsLoaded 检查是否已加载校准数据
	IsLoaded() bool

	// GetValidRange 获取有效范围
	GetValidRange() PrbValidRange

	// Calculate 执行插值计算
	Calculate(input InterpolationInput) (InterpolationResult, error)
}

// MultiPrbInterpolationMode 多PRB插值模式
type MultiPrbInterpolationMode string

const (
	// ModeNearest 最近邻插值模式
	ModeNearest MultiPrbInterpolationMode = "nearest"
	// ModeLinear 线性插值模式
	ModeLinear MultiPrbInterpolationMode = "linear"
)
