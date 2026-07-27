package backend

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	seven_interp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// sevenHoleHelpDocFileName 是 7 孔用户说明书文件名（与 docs/ 目录下文件名一致）。
// 命名带 "seven-hole-" 前缀，避免与 5/3 孔说明书冲突。
const sevenHoleHelpDocFileName = "seven-hole-用户说明书.html"

// sevenHoleState 封装 7 孔探针插值的运行时状态，自带 RWMutex。
// 与 5 孔 / 3 孔的 state 隔离，避免锁混用（SPEC § Boundaries 要求）。
//
// dataSource 字段记录当前已加载的数据源类型（"prb" / "calibration-csv" / ""），
// 供前端在切换数据源时判断是否需要清空槽位并重新分配。
type sevenHoleState struct {
	mu           sync.RWMutex
	interpolator *seven_interp.SevenHolePrbInterpolator
	prbFiles     []SevenHolePrbFileInfo
	dataSource   string // "prb" | "calibration-csv" | ""
}

// LoadSevenHolePrbFiles 加载已分配好的 7 个 .prb 文件路径（1 个内区 + 6 个扇区）。
//
// 与旧实现的差异（对齐 wind-daq 遍历测试 §5.6）：
//   - 旧实现：后端弹多选对话框 + 后端按 basename 路由 sector
//   - 新实现：前端弹对话框 + 前端按 basename 分配 sector + 后端只接收已分配的 inner+outer[6]
//
// 设计要点：
//   - 内区必须最先加载：外区插值在边界场景会回查内区网格（spec §5）
//   - sector 编号 1..6 对应 outerPaths[0..5]，与 7 孔探针外围孔位物理编号一致
//   - 外区路径数必须为 6，否则返回错误（避免静默用零值导致后续解析失败）
//   - 文件路径由前端按 basename 分配（"7.prb"→内区，"1.prb"~"6.prb"→扇区 n），
//     后端不再做 basename 路由，便于前端在 UI 上同步展示每个槽位的文件名
func (a *App) LoadSevenHolePrbFiles(innerPath string, outerPaths []string) SevenHoleLoadPrbResponse {
	if innerPath == "" {
		return SevenHoleLoadPrbResponse{Success: false, Error: "缺少内区文件路径"}
	}
	if len(outerPaths) != 6 {
		return SevenHoleLoadPrbResponse{
			Success: false,
			Error:   fmt.Sprintf("外区扇区文件数必须为 6，实际 %d", len(outerPaths)),
		}
	}

	interpolator := seven_interp.NewSevenHolePrbInterpolator()

	// 内区必须最先加载：外区插值在边界场景会回查内区网格（spec §5）。
	innerLines, err := ReadPrbLines(innerPath)
	if err != nil {
		return SevenHoleLoadPrbResponse{Success: false, Error: fmt.Sprintf("读取内区 PRB 失败: %s", err.Error())}
	}
	if err := interpolator.LoadInnerPrbLines(innerLines, filepath.Base(innerPath)); err != nil {
		return SevenHoleLoadPrbResponse{Success: false, Error: fmt.Sprintf("解析内区 PRB 失败: %s", err.Error())}
	}

	// 外区 1..6 按顺序加载，任一失败立即返回（避免错误被淹没）。
	if err := loadSevenHoleOuterPrbFiles(interpolator, outerPaths); err != nil {
		return SevenHoleLoadPrbResponse{Success: false, Error: err.Error()}
	}

	// 构造前端展示用的文件列表：内区在前（Sector=7），外区 1..6 顺序跟随。
	// PointCount 由算法包运行时读取：内区固定 169，扇区动态 = thetaCount×13。
	prbFiles := buildSevenHoleFileList(innerPath, outerPaths, interpolator)
	validRange := toSevenHoleValidRange(interpolator.GetValidRange())
	innerPointCount := interpolator.GetInnerPointCount()
	outerCounts := buildSevenHoleOuterCounts(interpolator)

	// 写操作加写锁，与读路径（Calculate 等）互斥。
	a.sevenHole.mu.Lock()
	a.sevenHole.interpolator = interpolator
	a.sevenHole.prbFiles = prbFiles
	a.sevenHole.dataSource = "prb"
	a.sevenHole.mu.Unlock()

	return SevenHoleLoadPrbResponse{
		Success: true,
		Data: &SevenHoleLoadPrbResult{
			Files:            prbFiles,
			ValidRange:       validRange,
			InnerPointCount:  innerPointCount,
			OuterPointCounts: outerCounts,
			DataSource:       "prb",
		},
	}
}

// LoadSevenHoleCalibrationCsvFiles 加载已分配好的 7 个校准 CSV 文件路径
// （1 份内区 CSV + 6 份外区 CSV，文件名约定：含"小角度区"→内区，含"大角度N区"→扇区 N）。
//
// 与 LoadSevenHolePrbFiles 同结构，区别仅在内/外区文件解析走 CSV 路径
// （GBK 解码 + 列位置契约 + 退化边抖动，详见 seven_hole_csv.go）。
// 解析失败的错误信息含文件路径，便于前端定位是哪个 CSV 出问题。
func (a *App) LoadSevenHoleCalibrationCsvFiles(innerPath string, outerPaths []string) SevenHoleLoadPrbResponse {
	if innerPath == "" {
		return SevenHoleLoadPrbResponse{Success: false, Error: "缺少内区 CSV 文件路径"}
	}
	if len(outerPaths) != 6 {
		return SevenHoleLoadPrbResponse{
			Success: false,
			Error:   fmt.Sprintf("外区扇区 CSV 数必须为 6，实际 %d", len(outerPaths)),
		}
	}

	interpolator, warnings, err := loadSevenHoleCalibrationCsvFiles(innerPath, outerPaths)
	if err != nil {
		return SevenHoleLoadPrbResponse{Success: false, Error: err.Error()}
	}

	prbFiles := buildSevenHoleFileList(innerPath, outerPaths, interpolator)
	validRange := toSevenHoleValidRange(interpolator.GetValidRange())
	innerPointCount := interpolator.GetInnerPointCount()
	outerCounts := buildSevenHoleOuterCounts(interpolator)

	a.sevenHole.mu.Lock()
	a.sevenHole.interpolator = interpolator
	a.sevenHole.prbFiles = prbFiles
	a.sevenHole.dataSource = "calibration-csv"
	a.sevenHole.mu.Unlock()

	return SevenHoleLoadPrbResponse{
		Success: true,
		Data: &SevenHoleLoadPrbResult{
			Files:            prbFiles,
			ValidRange:       validRange,
			InnerPointCount:  innerPointCount,
			OuterPointCounts: outerCounts,
			DataSource:       "calibration-csv",
			Warnings:         warnings,
		},
	}
}

// PickSevenHoleFiles 弹出多选文件对话框，让用户选择 7 孔 PRB 或校准 CSV 文件。
//
// 仅返回用户选中的文件路径列表，不解析、不分配槽位——分配逻辑由前端按 basename 完成
// （assignSevenHoleFilesByName / assignSevenHoleCsvFilesByName）。
// 这样后端无需关心文件名约定，前端可在 UI 上展示"哪些文件未被识别"。
//
// 取消选择时返回 Success=true + 空 Paths（与 Wails 对话框"OK 但无选择"语义一致）。
//
// 实现说明：直接调用 application.Get() 取全局单例，与 openFileDialog 一致
// （参见 dialog.go 注释解释了为何移除 a.app 双源回退）。
func (a *App) PickSevenHoleFiles() SevenHolePickFilesResponse {
	paths, err := application.Get().Dialog.OpenFile().
		SetTitle("选择 7 个 PRB / CSV 校准文件").
		AddFilter("PRB / CSV Files (*.prb;*.csv)", "*.prb;*.csv").
		AddFilter("PRB Files (*.prb)", "*.prb").
		AddFilter("CSV Files (*.csv)", "*.csv").
		PromptForMultipleSelection()
	if err != nil {
		return SevenHolePickFilesResponse{Success: false, Error: err.Error()}
	}
	// Wails 在用户取消时返回 nil；统一转为空数组便于前端 isEmpty 判断。
	if paths == nil {
		paths = []string{}
	}
	return SevenHolePickFilesResponse{Success: true, Paths: paths}
}

// GetSevenHoleDataSource 返回当前已加载的 7 孔数据源类型。
// 取值："prb"（PRB 文件集）/ "calibration-csv"（校准 CSV）/ ""（未加载）。
// 前端在初始化或切回 7 孔工作区时调用，用于决定槽位过滤器与展示文案。
func (a *App) GetSevenHoleDataSource() SevenHoleDataSourceResponse {
	a.sevenHole.mu.RLock()
	defer a.sevenHole.mu.RUnlock()
	return SevenHoleDataSourceResponse{Success: true, Data: a.sevenHole.dataSource}
}

// IsSevenHolePrbLoaded 返回 7 孔 .prb 文件集是否已全部加载。
func (a *App) IsSevenHolePrbLoaded() bool {
	a.sevenHole.mu.RLock()
	defer a.sevenHole.mu.RUnlock()
	return a.sevenHole.interpolator != nil && a.sevenHole.interpolator.IsLoaded()
}

// GetSevenHolePrbFiles 返回已加载的 7 孔 .prb / CSV 文件列表。
func (a *App) GetSevenHolePrbFiles() []SevenHolePrbFileInfo {
	a.sevenHole.mu.RLock()
	defer a.sevenHole.mu.RUnlock()
	return a.sevenHole.prbFiles
}

// GetSevenHoleValidRange 返回内区网格的角度覆盖范围（±30°）。
// 注意：算法包明确要求此范围仅供 UI 展示，不得用于事后有效性拒绝（spec §2.2）。
func (a *App) GetSevenHoleValidRange() SevenHoleValidRangeResponse {
	a.sevenHole.mu.RLock()
	interpolator := a.sevenHole.interpolator
	a.sevenHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return SevenHoleValidRangeResponse{Success: false, Error: "请先加载 7 孔校准文件（PRB 或 CSV）"}
	}
	vr := interpolator.GetValidRange()
	return SevenHoleValidRangeResponse{Success: true, Data: toSevenHoleValidRange(vr)}
}

// CalculateSevenHole 执行 7 孔单点插值计算。
// 输入的所有 P1..P7 必须为表压（gauge），spec §1.1 强制要求。
func (a *App) CalculateSevenHole(input SevenHoleInterpolationInput) SevenHoleCalculateResponse {
	a.sevenHole.mu.RLock()
	interpolator := a.sevenHole.interpolator
	a.sevenHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return SevenHoleCalculateResponse{Success: false, Error: "请先加载 7 孔校准文件（PRB 或 CSV）"}
	}

	coreInput := toSevenHoleCoreInput(input)
	result, err := interpolator.Calculate(coreInput)
	if err != nil {
		return SevenHoleCalculateResponse{Success: false, Error: err.Error()}
	}

	return SevenHoleCalculateResponse{Success: true, Data: toSevenHoleAppResult(result)}
}

// BatchCalculateSevenHole 批量计算，遇到错误不中断，标记失败行继续。
// 这样 CSV 中个别坏行不会让整个批次作废。
func (a *App) BatchCalculateSevenHole(inputs []SevenHoleInterpolationInput) SevenHoleBatchCalculateResponse {
	a.sevenHole.mu.RLock()
	interpolator := a.sevenHole.interpolator
	a.sevenHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return SevenHoleBatchCalculateResponse{Success: false, Error: "请先加载 7 孔校准文件（PRB 或 CSV）"}
	}

	results := make([]*SevenHoleInterpolationResult, len(inputs))
	var firstError string

	for i, input := range inputs {
		coreInput := toSevenHoleCoreInput(input)
		result, err := interpolator.Calculate(coreInput)
		if err != nil {
			results[i] = &SevenHoleInterpolationResult{
				IsValid: false,
				Warning: fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error()),
			}
			if firstError == "" {
				firstError = fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error())
			}
			continue
		}
		results[i] = toSevenHoleAppResult(result)
	}

	return SevenHoleBatchCalculateResponse{
		Success: firstError == "",
		Error:   firstError,
		Data:    results,
	}
}

// toSevenHoleCoreInput 将应用层输入转换为核心算法输入。
// 7 孔算法包统一使用表压（spec §1.1），应用层不提供 PressureMode，直接透传。
func toSevenHoleCoreInput(input SevenHoleInterpolationInput) seven_interp.InterpolationInput {
	return seven_interp.InterpolationInput{
		P1:   input.P1,
		P2:   input.P2,
		P3:   input.P3,
		P4:   input.P4,
		P5:   input.P5,
		P6:   input.P6,
		P7:   input.P7,
		PAtm: input.Patm,
		TAtm: input.Tatm,
	}
}

// toSevenHoleAppResult 将算法包结果转为应用层结果，字段一一映射。
// 注意 Alpha/Beta 在 7 孔里语义反转（Alpha=侧滑、Beta=迎角），但 JSON tag 与 5 孔一致。
// Theta/Phi 透传算法包的 PRB 网格原始角度坐标，供前端展示。
func toSevenHoleAppResult(r seven_interp.InterpolationResult) *SevenHoleInterpolationResult {
	return &SevenHoleInterpolationResult{
		Alpha:           r.Alpha,
		Beta:            r.Beta,
		Theta:           r.Theta,
		Phi:             r.Phi,
		MachNumber:      r.MachNumber,
		Velocity:        r.Velocity,
		DynamicPressure: r.DynamicPressure,
		P0:              r.TotalPressure,
		Ps:              r.StaticPressure,
		IsValid:         r.IsValid,
		Warning:         r.Warning,
	}
}

// toSevenHoleValidRange 将算法包的 PrbValidRange 转为应用层类型。
func toSevenHoleValidRange(vr seven_interp.PrbValidRange) SevenHolePrbValidRange {
	return SevenHolePrbValidRange{
		AlphaMin: vr.AlphaMin,
		AlphaMax: vr.AlphaMax,
		BetaMin:  vr.BetaMin,
		BetaMax:  vr.BetaMax,
		MachMin:  vr.MachMin,
		MachMax:  vr.MachMax,
	}
}

// buildSevenHoleFileList 构造前端展示用的文件列表：内区在前（Sector=7），外区 1..6 顺序跟随。
// PointCount 由算法包运行时读取：内区固定 169，扇区动态 = thetaCount×13。
// 此函数在 PRB 与 CSV 两条加载路径共用，避免重复构造逻辑。
func buildSevenHoleFileList(innerPath string, outerPaths []string, interp *seven_interp.SevenHolePrbInterpolator) []SevenHolePrbFileInfo {
	files := make([]SevenHolePrbFileInfo, 0, 7)
	files = append(files, SevenHolePrbFileInfo{
		FilePath:   innerPath,
		FileName:   filepath.Base(innerPath),
		Sector:     7,
		PointCount: interp.GetInnerPointCount(),
	})
	for i, p := range outerPaths {
		sector := i + 1
		files = append(files, SevenHolePrbFileInfo{
			FilePath:   p,
			FileName:   filepath.Base(p),
			Sector:     sector,
			PointCount: interp.GetOuterPointCount(sector),
		})
	}
	return files
}

// buildSevenHoleOuterCounts 读取 6 个扇区的实际点数到固定长度数组。
// 供前端展示"各扇区 thetaCount×13"动态值（如 4×13=52、7×13=91）。
func buildSevenHoleOuterCounts(interp *seven_interp.SevenHolePrbInterpolator) [6]int {
	var counts [6]int
	for i := 0; i < 6; i++ {
		counts[i] = interp.GetOuterPointCount(i + 1)
	}
	return counts
}

// csvFieldSetter 描述单个 CSV 列到 SevenHoleInterpolationInput 字段的映射。
// 用 setter 函数指针避免反射，保持与原直写代码相同的运行时性能。
type csvFieldSetter struct {
	name string
	set  func(*SevenHoleInterpolationInput, float64)
}

// sevenHoleInputFields 列出 7 孔数据 CSV 的 9 个必需列及其字段赋值函数。
// 顺序与 SPEC §1.1 一致：P1..P7 + Patm + Tatm。集中定义避免 ImportSevenHoleCsvData
// 中出现 9 行重复的 parse + 赋值代码，新增字段时只需在此追加一项。
var sevenHoleInputFields = []csvFieldSetter{
	{"P1", func(in *SevenHoleInterpolationInput, v float64) { in.P1 = v }},
	{"P2", func(in *SevenHoleInterpolationInput, v float64) { in.P2 = v }},
	{"P3", func(in *SevenHoleInterpolationInput, v float64) { in.P3 = v }},
	{"P4", func(in *SevenHoleInterpolationInput, v float64) { in.P4 = v }},
	{"P5", func(in *SevenHoleInterpolationInput, v float64) { in.P5 = v }},
	{"P6", func(in *SevenHoleInterpolationInput, v float64) { in.P6 = v }},
	{"P7", func(in *SevenHoleInterpolationInput, v float64) { in.P7 = v }},
	{"Patm", func(in *SevenHoleInterpolationInput, v float64) { in.Patm = v }},
	{"Tatm", func(in *SevenHoleInterpolationInput, v float64) { in.Tatm = v }},
}

// ImportSevenHoleCsvData 弹出文件选择对话框让用户选 7 孔数据 CSV，
// 解析 P1-P7 + Patm + Tatm 列（共 9 列，全部必需）。
//
// 与 5 孔 CSV 的区别：
//   - 5 孔：P1-P5 必需，Patm/Tatm/PressureMode 可选
//   - 7 孔：P1-P7 + Patm + Tatm 全部必需（spec §1.1 强制表压，无 PressureMode 列）
//
// 支持 .csv/.txt/.dat 三种扩展名（与 3 孔一致），分隔符自动检测 tab vs 逗号。
// 注意：此方法导入的是"数据 CSV"（待计算的压力数据），与 LoadSevenHoleCalibrationCsvFiles
// 导入的"校准 CSV"（标定网格点系数）完全不同——后者是 7 份 GBK 编码的标定文件。
func (a *App) ImportSevenHoleCsvData() SevenHoleImportCsvDataResponse {
	filePath, err := a.openFileDialog("选择 7 孔数据 CSV 文件").
		AddFilter("CSV/TXT/DAT (*.csv;*.txt;*.dat)", "*.csv;*.txt;*.dat").
		AddFilter("CSV Files (*.csv)", "*.csv").
		AddFilter("All Files (*.*)", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return SevenHoleImportCsvDataResponse{Success: false, Error: err.Error()}
	}
	if filePath == "" {
		return SevenHoleImportCsvDataResponse{Success: false, Error: "已取消选择"}
	}

	records, err := readSevenHoleCsvFile(filePath)
	if err != nil {
		return SevenHoleImportCsvDataResponse{Success: false, Error: err.Error()}
	}

	if len(records) < 2 {
		return SevenHoleImportCsvDataResponse{Success: false, Error: "CSV 文件为空或只有表头"}
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(col)] = i
	}

	// 用必需列的最大索引作为列数校验阈值，
	// 避免用户 CSV 含可选备注列但数据行缺该列时整行被丢弃。
	maxRequiredIdx := 0
	for _, f := range sevenHoleInputFields {
		idx, ok := colMap[f.name]
		if !ok {
			return SevenHoleImportCsvDataResponse{
				Success: false,
				Error:   fmt.Sprintf("缺少必要列: %s（7 孔 CSV 需包含 P1-P7 + Patm + Tatm 共 9 列）", f.name),
			}
		}
		if idx > maxRequiredIdx {
			maxRequiredIdx = idx
		}
	}

	datas := make([]SevenHoleInterpolationInput, 0, len(records)-1)
	var parseWarnings []string

	for rowIdx := 1; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		if len(row) <= maxRequiredIdx {
			parseWarnings = append(parseWarnings, fmt.Sprintf("第%d行列数不足，已跳过", rowIdx+1))
			continue
		}

		// 用 sevenHoleInputFields 驱动解析：每列独立 try-parse，
		// 任一列失败记录到 rowErrs 并继续后续列（让用户在一行内看到所有坏列），
		// 行末若有任何错误则跳过该行，所有错误合并到 parseWarnings。
		var input SevenHoleInterpolationInput
		var rowErrs []string
		for _, f := range sevenHoleInputFields {
			colIdx := colMap[f.name]
			val, err := strconv.ParseFloat(strings.TrimSpace(row[colIdx]), 64)
			if err != nil {
				rowErrs = append(rowErrs, fmt.Sprintf("第%d行第%d列解析失败: %q", rowIdx+1, colIdx+1, row[colIdx]))
				continue
			}
			f.set(&input, val)
		}
		if len(rowErrs) > 0 {
			parseWarnings = append(parseWarnings, rowErrs...)
			continue
		}
		datas = append(datas, input)
	}

	if len(parseWarnings) > 0 && len(datas) > 0 {
		log.Printf("7 孔 CSV 导入警告: %s", strings.Join(parseWarnings, "; "))
	}

	if len(datas) == 0 && len(parseWarnings) > 0 {
		return SevenHoleImportCsvDataResponse{
			Success: false,
			Error:   fmt.Sprintf("所有数据行解析失败: %s", strings.Join(parseWarnings, "; ")),
		}
	}

	return SevenHoleImportCsvDataResponse{Success: true, Data: datas}
}

// readSevenHoleCsvFile 读取 CSV 文件并自动检测分隔符（tab 或逗号）。
// 与 3 孔 readThreeHoleCsvFile 一致：剥离 UTF-8 BOM、跳过空行与 # 注释行。
// 7 孔 CSV 通常用 tab 分隔（与校准数据导出格式一致），但也支持逗号分隔。
func readSevenHoleCsvFile(filePath string) ([][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %s", err.Error())
	}
	defer file.Close()

	// 剥离 UTF-8 BOM：Excel "另存为 CSV UTF-8" 会在首字节加 BOM，
	// 导致首列名变成 "\uFEFFP1"，colMap["P1"] 查找失败。
	br := bufio.NewReader(file)
	StripUTF8BOM(br)

	// 按行读取并跳过空行与 # 注释行（与用户说明书承诺一致）。
	scanner := bufio.NewScanner(br)
	var dataLines []string
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		dataLines = append(dataLines, trimmed)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败: %s", err.Error())
	}

	if len(dataLines) < 2 {
		return nil, fmt.Errorf("文件为空或只有表头")
	}

	// 检测分隔符：统计第一行 tab 与逗号出现次数，多的为主分隔符。
	delimiter := DetectDelimiter(dataLines[0])

	records := make([][]string, 0, len(dataLines))
	for _, line := range dataLines {
		fields := strings.Split(line, string(delimiter))
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		records = append(records, fields)
	}
	return records, nil
}

// OpenSevenHoleHelpDoc 用系统默认程序打开 7 孔用户说明书 HTML 文件。
// 路径解析与跨平台打开逻辑抽到共享工具 GetHelpDocPath / OpenHelpDocByPath。
func (a *App) OpenSevenHoleHelpDoc() error {
	return OpenHelpDocByPath(GetHelpDocPath(sevenHoleHelpDocFileName))
}

// loadSevenHoleOuterPrbFiles 顺序加载 6 个外区 PRB 文件并喂给插值器。
// 任一失败立即返回错误（含扇区号便于定位）；内区已由调用方加载完毕，
// 外区失败时调用方需自行决定是否回滚状态（当前实现不回滚，依赖调用方整体失败语义）。
// 抽出此函数避免 LoadSevenHolePrbFiles 函数体超过 50 行硬约束。
func loadSevenHoleOuterPrbFiles(interpolator *seven_interp.SevenHolePrbInterpolator, outerPaths []string) error {
	for i, outerPath := range outerPaths {
		sector := i + 1
		if outerPath == "" {
			return fmt.Errorf("缺少 %d.prb 外区文件路径", sector)
		}
		outerLines, err := ReadPrbLines(outerPath)
		if err != nil {
			return fmt.Errorf("读取 %d.prb 失败: %s", sector, err.Error())
		}
		if err := interpolator.LoadOuterPrbLines(sector, outerLines, filepath.Base(outerPath)); err != nil {
			return fmt.Errorf("解析 %d.prb 失败: %s", sector, err.Error())
		}
	}
	return nil
}
