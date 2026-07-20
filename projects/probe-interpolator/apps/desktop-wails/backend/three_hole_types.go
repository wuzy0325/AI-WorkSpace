package backend

// three_hole_types.go 定义 3 孔探针插值的输入输出契约。
// 字段命名与 JSON tag 与旧 three-hole-interpolator 程序完全一致，
// 老用户的 .prb 文件和 CSV 数据可直接加载，保证向后兼容。
//
// 命名说明：所有类型加 ThreeHole 前缀，避免与 5 孔（FiveHole 前缀的输入输出 +
// 共享的 PrbFileInfo/PrbValidRange 等）以及后续 7 孔的类型在 Wails binding 生成时冲突。

// ThreeHolePrbFileInfo 是单个 3 孔 .prb 校准文件的元信息，前端用于展示已加载文件列表。
type ThreeHolePrbFileInfo struct {
	FilePath   string                 `json:"filePath"`
	FileName   string                 `json:"fileName"`
	MachNumber float64                `json:"machNumber"`
	ValidRange ThreeHolePrbValidRange `json:"validRange"`
}

// ThreeHolePrbValidRange 描述 3 孔 .prb 校准网格的角度与马赫数覆盖范围。
// 3 孔只有一维角度（Alpha），无 Beta 字段（与 5 孔的 PrbValidRange 区别在此）。
type ThreeHolePrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

// ThreeHoleInterpolationInput 是 3 孔单点计算的输入。
// PressureMode 取值 "gauge"（表压，默认）或 "absolute"（绝压），
// 绝压模式下后端会自动减去 Patm 转为表压再传给算法包。
type ThreeHoleInterpolationInput struct {
	P1           float64 `json:"P1"`
	P2           float64 `json:"P2"`
	P3           float64 `json:"P3"`
	Patm         float64 `json:"Patm"`
	Tatm         float64 `json:"Tatm"`
	PressureMode string  `json:"pressureMode"`
}

// ThreeHoleInterpolationResult 是 3 孔单点计算的输出。
// 3 孔只测一维角度（Alpha=迎角），无 Beta/速度分量字段（与 5 孔结果区别在此）。
type ThreeHoleInterpolationResult struct {
	Alpha          float64 `json:"alpha"`
	MachNumber     float64 `json:"machNumber"`
	TotalPressure  float64 `json:"P0"`
	StaticPressure float64 `json:"Ps"`
	IterationCount int     `json:"iterationCount"`
	IsValid        bool    `json:"isValid"`
	Warning        string  `json:"warning,omitempty"`
}

// 以下 Response 类型是 3 孔 Wails 后端方法的统一返回包装。
// 与 5 孔对应类型结构一致，但 Data 字段引用 3 孔专属类型，故独立定义避免 Go 类型冲突。

type ThreeHoleLoadPrbResponse struct {
	Success bool                    `json:"success"`
	Error   string                  `json:"error,omitempty"`
	Data    *ThreeHoleLoadPrbResult `json:"data,omitempty"`
}

type ThreeHoleLoadPrbResult struct {
	Files     []ThreeHolePrbFileInfo `json:"files"`
	MachRange []float64              `json:"machRange"`
	Warnings  []string               `json:"warnings"`
}

type ThreeHoleMachRangeResponse struct {
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
	Data    []float64 `json:"data,omitempty"`
}

type ThreeHoleCalculateResponse struct {
	Success bool                          `json:"success"`
	Error   string                        `json:"error,omitempty"`
	Data    *ThreeHoleInterpolationResult `json:"data,omitempty"`
}

type ThreeHoleBatchCalculateResponse struct {
	Success bool                            `json:"success"`
	Error   string                          `json:"error,omitempty"`
	Data    []*ThreeHoleInterpolationResult `json:"data,omitempty"`
}

type ThreeHoleImportCsvDataResponse struct {
	Success bool                          `json:"success"`
	Error   string                        `json:"error,omitempty"`
	Data    []ThreeHoleInterpolationInput `json:"data,omitempty"`
}
