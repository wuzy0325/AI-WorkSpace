package backend

// seven_hole_types.go 定义 7 孔探针插值的输入输出契约。
//
// 与 5 孔 / 3 孔的关键差异（参考 shared/algorithms/go/sevenhole/interpolation/types.go）：
//   - 输入为 7 个孔压力 P1..P7（P7=中心孔，P1..P6=外围 60° 等分孔）+ 大气压 Patm + 大气温 Tatm
//   - 不带 PressureMode 字段：7 孔 spec §1.1 强制要求所有孔压力为表压（gauge），绝压需在导入前转换
//   - Alpha=侧滑角（sideslip）、Beta=迎角（angle of attack），与 5 孔语义反转（spec §2.2）
//   - 结果含 Velocity / DynamicPressure，但无 Vx/Vy/Vz 速度分量（5 孔有，7 孔无）
//   - PRB 加载需要 7 个独立文件（1.prb..7.prb），不存在"马赫数范围"概念；ValidRange.MachMin/Max 恒为 0
//
// 命名说明：所有类型加 SevenHole 前缀，避免与 5 孔（FiveHole 前缀 + 共享的 PrbFileInfo 等）
// 以及 3 孔（ThreeHole 前缀）的类型在 Wails binding 生成时冲突。

// SevenHolePrbFileInfo 是单个 7 孔 .prb 校准文件的元信息。
// Sector 字段标识文件角色：0=内区（7.prb），1..6=外区扇区 n（n.prb）。
//
// 注意：曾经存在的 Loaded 字段已移除——后端只在全部 7 个文件成功加载后才写入 sevenHoleState，
// 因此返回列表中的每一项都隐含 Loaded=true。前端若需要"是否已加载"判断应调用 IsSevenHolePrbLoaded。
type SevenHolePrbFileInfo struct {
	FilePath string `json:"filePath"`
	FileName string `json:"fileName"`
	Sector   int    `json:"sector"` // 0=inner (7.prb), 1..6=outer sector n
}

// SevenHolePrbValidRange 描述 7 孔内区网格的角度覆盖范围（±30°）。
// 注意：MachMin/MachMax 恒为 0，仅 Alpha/Beta 范围有意义；
// 算法包明确要求不得用此范围做事后有效性拒绝（spec §2.2）。
type SevenHolePrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	BetaMin  float64 `json:"betaMin"`
	BetaMax  float64 `json:"betaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

// SevenHoleInterpolationInput 是 7 孔单点计算的输入。
// 所有 P1..P7 必须是表压（gauge，Pa），Patm 为绝对压力（Pa），Tatm 为摄氏温度。
// 不提供 PressureMode 字段：7 孔 spec §1.1 强制表压输入，绝压数据需在导入前由调用方转换。
type SevenHoleInterpolationInput struct {
	P1   float64 `json:"P1"`   // 外围孔 1 表压（Pa）
	P2   float64 `json:"P2"`   // 外围孔 2 表压（Pa）
	P3   float64 `json:"P3"`   // 外围孔 3 表压（Pa）
	P4   float64 `json:"P4"`   // 外围孔 4 表压（Pa）
	P5   float64 `json:"P5"`   // 外围孔 5 表压（Pa）
	P6   float64 `json:"P6"`   // 外围孔 6 表压（Pa）
	P7   float64 `json:"P7"`   // 中心孔表压（Pa）
	Patm float64 `json:"Patm"` // 大气绝对压力（Pa）
	Tatm float64 `json:"Tatm"` // 大气温度（℃）
}

// SevenHoleInterpolationResult 是 7 孔单点计算的输出。
//
// 字段语义注意（与 5 孔反转，spec §2.2 / 附录 A）：
//   - Alpha = 侧滑角（sideslip），5 孔里 Alpha 是迎角
//   - Beta  = 迎角（angle of attack），5 孔里 Beta 是侧滑角
//
// JSON tag 与 5 孔结果一致（P0/Ps/alpha/beta/velocity/dynamicPressure），
// 便于前端结果表格组件复用同样的列定义。
type SevenHoleInterpolationResult struct {
	Alpha           float64 `json:"alpha"`           // 侧滑角（deg），7 孔语义
	Beta            float64 `json:"beta"`            // 迎角（deg），7 孔语义
	MachNumber      float64 `json:"machNumber"`      // 马赫数
	Velocity        float64 `json:"velocity"`        // 流速（m/s）
	DynamicPressure float64 `json:"dynamicPressure"` // 动压 Pt-Ps（Pa）
	P0              float64 `json:"P0"`              // 总压（表压，Pa）
	Ps              float64 `json:"Ps"`              // 静压（表压，Pa）
	IsValid         bool    `json:"isValid"`
	Warning         string  `json:"warning,omitempty"`
}

// 以下 Response 类型是 7 孔 Wails 后端方法的统一返回包装。
// 与 5 孔 / 3 孔对应类型结构一致，但 Data 字段引用 7 孔专属类型，故独立定义避免 Go 类型冲突。

type SevenHoleLoadPrbResponse struct {
	Success bool                    `json:"success"`
	Error   string                  `json:"error,omitempty"`
	Data    *SevenHoleLoadPrbResult `json:"data,omitempty"`
}

// SevenHoleLoadPrbResult 是加载 .prb 文件集后返回给前端的结果。
// Files 按内区（7.prb）→ 外区 1..6 顺序排列；ValidRange 来自内区网格角点。
type SevenHoleLoadPrbResult struct {
	Files      []SevenHolePrbFileInfo   `json:"files"`
	ValidRange SevenHolePrbValidRange   `json:"validRange"`
	Warnings   []string                 `json:"warnings"`
}

type SevenHoleValidRangeResponse struct {
	Success bool                  `json:"success"`
	Error   string                `json:"error,omitempty"`
	Data    SevenHolePrbValidRange `json:"data,omitempty"`
}

type SevenHoleCalculateResponse struct {
	Success bool                           `json:"success"`
	Error   string                         `json:"error,omitempty"`
	Data    *SevenHoleInterpolationResult  `json:"data,omitempty"`
}

type SevenHoleBatchCalculateResponse struct {
	Success bool                            `json:"success"`
	Error   string                          `json:"error,omitempty"`
	Data    []*SevenHoleInterpolationResult `json:"data,omitempty"`
}

type SevenHoleImportCsvDataResponse struct {
	Success bool                           `json:"success"`
	Error   string                         `json:"error,omitempty"`
	Data    []SevenHoleInterpolationInput  `json:"data,omitempty"`
}
