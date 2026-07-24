package backend

// five_hole_types.go 定义 5 孔探针插值的输入输出契约。
// 字段命名与 JSON tag 与旧 five-hole-interpolator 程序完全一致，
// 老用户的 .prb 文件和 CSV 数据可直接加载，保证向后兼容。

// PrbFileInfo 是单个 .prb 校准文件的元信息，前端用于展示已加载文件列表。
type PrbFileInfo struct {
	FilePath   string        `json:"filePath"`
	FileName   string        `json:"fileName"`
	MachNumber float64       `json:"machNumber"`
	ValidRange PrbValidRange `json:"validRange"`
}

// PrbValidRange 描述 .prb 校准网格的角度与马赫数覆盖范围。
// 前端据此提示用户输入是否在校准区间内。
type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	BetaMin  float64 `json:"betaMin"`
	BetaMax  float64 `json:"betaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

// FiveHoleInterpolationInput 是 5 孔单点计算的输入。
// PressureMode 取值 "gauge"（表压，默认）或 "absolute"（绝压），
// 绝压模式下后端会自动减去 Patm 转为表压再传给算法包。
type FiveHoleInterpolationInput struct {
	P1           float64 `json:"P1"`
	P2           float64 `json:"P2"`
	P3           float64 `json:"P3"`
	P4           float64 `json:"P4"`
	P5           float64 `json:"P5"`
	Patm         float64 `json:"Patm"`
	Tatm         float64 `json:"Tatm"`
	PressureMode string  `json:"pressureMode"`
}

// FiveHoleInterpolationResult 是 5 孔单点计算的输出。
// 字段语义：Alpha=迎角、Beta=侧滑角（注意 7 孔语义反转）。
type FiveHoleInterpolationResult struct {
	Alpha           float64 `json:"alpha"`
	Beta            float64 `json:"beta"`
	MachNumber      float64 `json:"machNumber"`
	V               float64 `json:"V"`
	Vx              float64 `json:"Vx"`
	Vy              float64 `json:"Vy"`
	Vz              float64 `json:"Vz"`
	Velocity        float64 `json:"velocity"`
	CAS             float64 `json:"cas"`
	SAT             float64 `json:"sat"`
	DynamicPressure float64 `json:"dynamicPressure"`
	Density         float64 `json:"density"`
	TotalPressure   float64 `json:"P0"`
	StaticPressure  float64 `json:"Ps"`
	IsValid         bool    `json:"isValid"`
	Warning         string  `json:"warning,omitempty"`
}

// 以下 Response 类型是 Wails 后端方法的统一返回包装。
// Success=false 时 Error 字段携带人类可读的错误信息。

type LoadPrbResponse struct {
	Success bool           `json:"success"`
	Error   string         `json:"error,omitempty"`
	Data    *LoadPrbResult `json:"data,omitempty"`
}

type LoadPrbResult struct {
	Files     []PrbFileInfo `json:"files"`
	MachRange []float64     `json:"machRange"`
	Warnings  []string      `json:"warnings"`
}

type MachRangeResponse struct {
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
	Data    []float64 `json:"data,omitempty"`
}

type CalculateResponse struct {
	Success bool                        `json:"success"`
	Error   string                      `json:"error,omitempty"`
	Data    *FiveHoleInterpolationResult `json:"data,omitempty"`
}

type BatchCalculateResponse struct {
	Success bool                          `json:"success"`
	Error   string                        `json:"error,omitempty"`
	Data    []*FiveHoleInterpolationResult `json:"data,omitempty"`
}

type ImportCsvDataResponse struct {
	Success bool                       `json:"success"`
	Error   string                     `json:"error,omitempty"`
	Data    []FiveHoleInterpolationInput `json:"data,omitempty"`
}
