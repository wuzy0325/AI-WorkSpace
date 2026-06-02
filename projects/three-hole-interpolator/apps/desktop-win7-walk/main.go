package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	three_interp "ai-workspace/shared/algorithms/go/threehole/interpolation"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	msgBox = user32.NewProc("MessageBoxW")
)

func showError(title, message string) {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	msgPtr, _ := syscall.UTF16PtrFromString(message)
	msgBox.Call(0, uintptr(unsafe.Pointer(msgPtr)), uintptr(unsafe.Pointer(titlePtr)), 0x00000010)
}

type AppWindow struct {
	*walk.MainWindow

	btnLoad   *walk.PushButton
	btnImport *walk.PushButton
	btnExport *walk.PushButton
	btnHelp   *walk.PushButton

	lblPrbInfo *walk.Label

	txtP1   *walk.LineEdit
	txtP2   *walk.LineEdit
	txtP3   *walk.LineEdit
	txtPatm *walk.LineEdit
	txtTatm *walk.LineEdit
	cbMode  *walk.ComboBox
	btnAdd  *walk.PushButton

	dataTV     *walk.TableView
	dataModel  *DataTableModel

	resultTV     *walk.TableView
	resultModel  *ResultTableModel

	btnCalc *walk.PushButton
	tab     *walk.TabWidget

	statusBar *walk.StatusBarItem

	loaded       bool
	interpolator *three_interp.ThreeHoleInterpolator
}

// ==================== 输入数据表格模型 ====================

type DataRow struct {
	Index string
	Col1  string
	Col2  string
	Col3  string
	Col4  string
	Col5  string
	Col6  string
}

type DataTableModel struct {
	walk.TableModelBase
	rows []*DataRow
}

func NewDataTableModel() *DataTableModel {
	m := new(DataTableModel)
	return m
}

func (m *DataTableModel) RowCount() int {
	return len(m.rows)
}

func (m *DataTableModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	switch col {
	case 0:
		return r.Index
	case 1:
		return r.Col1
	case 2:
		return r.Col2
	case 3:
		return r.Col3
	case 4:
		return r.Col4
	case 5:
		return r.Col5
	case 6:
		return r.Col6
	}
	return ""
}

func (m *DataTableModel) AddRow(r *DataRow) {
	m.rows = append(m.rows, r)
	m.PublishRowsReset()
}

func (m *DataTableModel) Clear() {
	m.rows = nil
	m.PublishRowsReset()
}

// ==================== 结果表格模型 ====================

type ResultRow struct {
	Index string
	Col1  string
	Col2  string
	Col3  string
	Col4  string
}

type ResultTableModel struct {
	walk.TableModelBase
	rows []*ResultRow
}

func NewResultTableModel() *ResultTableModel {
	m := new(ResultTableModel)
	return m
}

func (m *ResultTableModel) RowCount() int {
	return len(m.rows)
}

func (m *ResultTableModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	switch col {
	case 0:
		return r.Index
	case 1:
		return r.Col1
	case 2:
		return r.Col2
	case 3:
		return r.Col3
	case 4:
		return r.Col4
	}
	return ""
}

func (m *ResultTableModel) SetRows(rows []*ResultRow) {
	m.rows = rows
	m.PublishRowsReset()
}

func (m *ResultTableModel) Clear() {
	m.rows = nil
	m.PublishRowsReset()
}

// ==================== 主逻辑 ====================

func main() {
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("程序崩溃: %v\n\n堆栈:\n%v", r, string(debug.Stack()))
			showError("错误", errMsg)
		}
	}()

	aw := new(AppWindow)

	if _, err := (MainWindow{
		AssignTo: &aw.MainWindow,
		Title:    "三孔探针插值计算",
		MinSize:  Size{1000, 700},
		Size:     Size{1100, 760},
		Layout:   VBox{MarginsZero: true, Spacing: 6},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 4},
				Children: []Widget{
					PushButton{AssignTo: &aw.btnLoad, Text: "加载 PRB 文件", MinSize: Size{120, 30}, OnClicked: aw.onLoadPrb},
					PushButton{AssignTo: &aw.btnImport, Text: "导入 CSV", MinSize: Size{100, 30}, OnClicked: aw.onImportCsv},
					PushButton{AssignTo: &aw.btnExport, Text: "导出结果", MinSize: Size{100, 30}, OnClicked: aw.onExport},
					HSpacer{},
					PushButton{AssignTo: &aw.btnHelp, Text: "帮助", MinSize: Size{80, 30}, OnClicked: aw.onHelp},
				},
			},
			Label{AssignTo: &aw.lblPrbInfo, Text: "就绪 - 请加载 PRB 校准文件", Font: Font{PointSize: 9}},
			TabWidget{
				AssignTo: &aw.tab,
				Pages: []TabPage{
					{Title: "数据输入", Layout: VBox{MarginsZero: true, Spacing: 6}, Children: aw.buildInputPage()},
					{Title: "计算结果", Layout: VBox{MarginsZero: true, Spacing: 6}, Children: aw.buildResultPage()},
				},
			},
		},
		StatusBarItems: []StatusBarItem{
			{AssignTo: &aw.statusBar, Text: "就绪", Width: 400},
		},
	}.Run()); err != nil {
		log.Fatal(err)
	}
}

func (aw *AppWindow) buildInputPage() []Widget {
	aw.dataModel = NewDataTableModel()

	return []Widget{
		GroupBox{
			Title:  "压力参数输入",
			Layout: Grid{Columns: 7, Spacing: 6},
			Children: []Widget{
				Label{Text: "P1 (Pa):"},
				LineEdit{AssignTo: &aw.txtP1, Text: "0"},
				Label{Text: "P2 (Pa):"},
				LineEdit{AssignTo: &aw.txtP2, Text: "0"},
				Label{Text: "P3 (Pa):"},
				LineEdit{AssignTo: &aw.txtP3, Text: "0"},
				Label{Text: ""},

				Label{Text: "大气压 (Pa):"},
				LineEdit{AssignTo: &aw.txtPatm, Text: "101325"},
				Label{Text: "气温 (℃):"},
				LineEdit{AssignTo: &aw.txtTatm, Text: "20"},
				Label{Text: "压力模式:"},
				ComboBox{AssignTo: &aw.cbMode, Editable: false, Model: []string{"表压", "绝压"}, CurrentIndex: 0},
				PushButton{AssignTo: &aw.btnAdd, Text: "添加", OnClicked: aw.onAddRow, MinSize: Size{80, 0}},
			},
		},
		Label{Text: "数据列表:"},
		TableView{
			AssignTo: &aw.dataTV,
			Model:    aw.dataModel,
			Columns: []TableViewColumn{
				{Title: "#", Width: 40},
				{Title: "P1", Width: 120},
				{Title: "P2", Width: 120},
				{Title: "P3", Width: 120},
				{Title: "Patm", Width: 120},
				{Title: "Tatm", Width: 80},
				{Title: "模式", Width: 60},
			},
		},
		PushButton{
			AssignTo:  &aw.btnCalc,
			Text:      "执行插值计算",
			Font:      Font{PointSize: 11, Bold: true},
			MinSize:   Size{0, 38},
			OnClicked: aw.onCalculate,
		},
	}
}

func (aw *AppWindow) buildResultPage() []Widget {
	aw.resultModel = NewResultTableModel()

	return []Widget{
		Label{Text: "计算结果:"},
		TableView{
			AssignTo: &aw.resultTV,
			Columns: []TableViewColumn{
				{Title: "#", Width: 40},
				{Title: "α (°)", Width: 120},
				{Title: "Ma", Width: 120},
				{Title: "Pt (Pa)", Width: 140},
				{Title: "Ps (Pa)", Width: 140},
			},
		},
	}
}

func (aw *AppWindow) setStatus(text string) {
	if aw.statusBar != nil {
		aw.statusBar.SetText(text)
	}
}

func (aw *AppWindow) ensureDataModel() {
	if aw.dataTV.Model() == nil && aw.dataModel != nil {
		aw.dataTV.SetModel(aw.dataModel)
	}
}

func (aw *AppWindow) ensureResultModel() {
	if aw.resultTV.Model() == nil && aw.resultModel != nil {
		aw.resultTV.SetModel(aw.resultModel)
	}
}

func (aw *AppWindow) onLoadPrb() {
	dlg := new(walk.FileDialog)
	dlg.Title = "选择 PRB 校准文件"
	dlg.Filter = "PRB 文件 (*.prb)|*.prb|所有文件 (*.*)|*.*"

	ok, err := dlg.ShowOpen(aw)
	if err != nil || !ok || dlg.FilePath == "" {
		return
	}

	lines, err := readPrbLines(dlg.FilePath)
	if err != nil {
		walk.MsgBox(aw, "错误", fmt.Sprintf("读取失败: %s", err.Error()), walk.MsgBoxIconError)
		return
	}

	fileData := []three_interp.PrbFileData{{FilePath: dlg.FilePath, Lines: lines}}

	interpolator := three_interp.NewThreeHoleInterpolator()
	_, err = interpolator.LoadPrbData(fileData)
	if err != nil {
		walk.MsgBox(aw, "PRB 加载失败", err.Error(), walk.MsgBoxIconError)
		return
	}

	aw.interpolator = interpolator
	aw.loaded = true

	minMa, maxMa := interpolator.GetMachRange()
	info := fmt.Sprintf("已加载: %s | Ma: %.3f ~ %.3f",
		filepath.Base(dlg.FilePath), minMa, maxMa)
	aw.lblPrbInfo.SetText(info)
	aw.setStatus("PRB 文件加载成功")
}

func (aw *AppWindow) onAddRow() {
	p1, err1 := strconv.ParseFloat(aw.txtP1.Text(), 64)
	p2, err2 := strconv.ParseFloat(aw.txtP2.Text(), 64)
	p3, err3 := strconv.ParseFloat(aw.txtP3.Text(), 64)
	patm, err4 := strconv.ParseFloat(aw.txtPatm.Text(), 64)
	tatm, err5 := strconv.ParseFloat(aw.txtTatm.Text(), 64)

	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
		walk.MsgBox(aw, "输入错误", "请确保所有输入字段都是有效数字", walk.MsgBoxIconError)
		return
	}

	if !aw.loaded {
		walk.MsgBox(aw, "提示", "请先加载 PRB 文件", walk.MsgBoxIconWarning)
		return
	}

	mode := "表压"
	if aw.cbMode.CurrentIndex() == 1 {
		mode = "绝压"
	}

	aw.ensureDataModel()

	idx := aw.dataModel.RowCount() + 1
	row := &DataRow{
		Index: fmt.Sprintf("%d", idx),
		Col1:  formatNum(p1),
		Col2:  formatNum(p2),
		Col3:  formatNum(p3),
		Col4:  formatNum(patm),
		Col5:  fmt.Sprintf("%.0f", tatm),
		Col6:  mode,
	}
	aw.dataModel.AddRow(row)
	aw.setStatus(fmt.Sprintf("已添加第 %d 条数据", idx))

	aw.txtP1.SetText("0")
	aw.txtP2.SetText("0")
	aw.txtP3.SetText("0")
	aw.txtP1.SetFocus()
}

func (aw *AppWindow) onCalculate() {
	if !aw.loaded || aw.interpolator == nil {
		walk.MsgBox(aw, "提示", "请先加载 PRB 文件", walk.MsgBoxIconWarning)
		return
	}

	if aw.dataModel.RowCount() == 0 {
		walk.MsgBox(aw, "提示", "请先添加数据行", walk.MsgBoxIconWarning)
		return
	}

	tatm, _ := strconv.ParseFloat(aw.txtTatm.Text(), 64)
	modeIsAbsolute := aw.cbMode.CurrentIndex() == 1

	rows := aw.dataModel.rows
	inputs := make([]three_interp.InterpolationInput, len(rows))
	for i, r := range rows {
		p1, _ := strconv.ParseFloat(strings.ReplaceAll(r.Col1, ",", ""), 64)
		p2, _ := strconv.ParseFloat(strings.ReplaceAll(r.Col2, ",", ""), 64)
		p3, _ := strconv.ParseFloat(strings.ReplaceAll(r.Col3, ",", ""), 64)
		pa, _ := strconv.ParseFloat(strings.ReplaceAll(r.Col4, ",", ""), 64)

		input := three_interp.InterpolationInput{
			P1: p1, P2: p2, P3: p3, PAtm: pa, TAtm: tatm,
		}
		if modeIsAbsolute {
			input.P1 = p1 - pa
			input.P2 = p2 - pa
			input.P3 = p3 - pa
		}
		inputs[i] = input
	}

	aw.setStatus("正在计算中，请稍候...")
	aw.btnCalc.SetEnabled(false)

	results := make([]*ResultRow, len(inputs))
	validCount := 0

	for i, input := range inputs {
		idx := i + 1
		result, err := aw.interpolator.Calculate(input)
		if err != nil {
			results[i] = &ResultRow{
				Index: fmt.Sprintf("%d", idx),
				Col1: "-", Col2: "-", Col3: "-", Col4: "-",
			}
			continue
		}
		results[i] = &ResultRow{
			Index: fmt.Sprintf("%d", idx),
			Col1:  formatVal(result.Alpha),
			Col2:  formatVal(result.MachNumber),
			Col3:  formatVal(result.TotalPressure),
			Col4:  formatVal(result.StaticPressure),
		}
		if result.IsValid {
			validCount++
		}
	}

	aw.ensureResultModel()
	aw.resultModel.SetRows(results)
	aw.tab.SetCurrentIndex(1)
	aw.btnCalc.SetEnabled(true)
	aw.setStatus(fmt.Sprintf("计算完成，共 %d 条，有效 %d 条", len(results), validCount))
}

func (aw *AppWindow) onImportCsv() {
	if !aw.loaded {
		walk.MsgBox(aw, "提示", "请先加载 PRB 文件", walk.MsgBoxIconWarning)
		return
	}

	dlg := new(walk.FileDialog)
	dlg.Title = "选择数据文件"
	dlg.Filter = "数据文件 (*.csv;*.txt;*.dat)|*.csv;*.txt;*.dat|所有文件 (*.*)|*.*"

	ok, err := dlg.ShowOpen(aw)
	if err != nil || !ok || dlg.FilePath == "" {
		return
	}

	records, err := readCsvFile(dlg.FilePath)
	if err != nil {
		walk.MsgBox(aw, "导入失败", err.Error(), walk.MsgBoxIconError)
		return
	}

	colMap, csvErr := parseCsvHeader(records[0])
	if csvErr != "" {
		walk.MsgBox(aw, "CSV 格式错误", csvErr, walk.MsgBoxIconError)
		return
	}

	datas, warnings := parseCsvRows(records[1:], colMap)
	if len(datas) == 0 {
		msg := "没有有效数据"
		if len(warnings) > 0 {
			msg = strings.Join(warnings, "\n")
		}
		walk.MsgBox(aw, "导入失败", msg, walk.MsgBoxIconError)
		return
	}

	mode := "表压"
	if aw.cbMode.CurrentIndex() == 1 {
		mode = "绝压"
	}

	aw.ensureDataModel()

	for _, d := range datas {
		idx := aw.dataModel.RowCount() + 1
		row := &DataRow{
			Index: fmt.Sprintf("%d", idx),
			Col1:  formatNum(d.P1),
			Col2:  formatNum(d.P2),
			Col3:  formatNum(d.P3),
			Col4:  formatNum(d.PAtm),
			Col5:  fmt.Sprintf("%.0f", d.TAtm),
			Col6:  mode,
		}
		aw.dataModel.AddRow(row)
	}

	aw.setStatus(fmt.Sprintf("已导入 %d 条数据", len(datas)))
}

func (aw *AppWindow) onExport() {
	if aw.resultModel.RowCount() == 0 {
		walk.MsgBox(aw, "提示", "没有可导出的结果", walk.MsgBoxIconWarning)
		return
	}

	dlg := new(walk.FileDialog)
	dlg.Title = "导出 CSV 文件"
	dlg.Filter = "CSV 文件 (*.csv)|*.csv"

	ok, err := dlg.ShowSave(aw)
	if err != nil || !ok || dlg.FilePath == "" {
		return
	}

	f, err := os.Create(dlg.FilePath)
	if err != nil {
		walk.MsgBox(aw, "导出失败", err.Error(), walk.MsgBoxIconError)
		return
	}
	defer f.Close()

	f.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(f)
	_ = writer.Write([]string{"序号", "α(°)", "Ma", "Pt(Pa)", "Ps(Pa)"})

	for _, r := range aw.resultModel.rows {
		_ = writer.Write([]string{
			r.Index, r.Col1, r.Col2, r.Col3, r.Col4,
		})
	}
	writer.Flush()

	aw.setStatus(fmt.Sprintf("已导出 %d 条结果", aw.resultModel.RowCount()))
}

func (aw *AppWindow) onHelp() {
	helpPath := findHelpDoc()
	if helpPath == "" {
		walk.MsgBox(aw, "提示", "未找到用户说明书文件", walk.MsgBoxIconWarning)
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", helpPath)
	default:
		cmd = exec.Command("xdg-open", helpPath)
	}
	_ = cmd.Start()
}

func readPrbLines(filePath string) ([]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\r\n")
	if len(lines) == 1 {
		lines = strings.Split(content, "\n")
	}
	var result []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" {
			result = append(result, t)
		}
	}
	return result, nil
}

func readCsvFile(filePath string) ([][]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %s", err.Error())
	}

	content := string(data)
	lines := strings.Split(content, "\r\n")
	if len(lines) == 1 {
		lines = strings.Split(content, "\n")
	}

	var dataLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "\xEF\xBB\xBF\uFEFF")
		dataLines = append(dataLines, trimmed)
	}

	if len(dataLines) < 2 {
		return nil, fmt.Errorf("文件为空或只有表头")
	}

	delimiter := detectDelimiter(dataLines[0])
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

func detectDelimiter(line string) rune {
	tabCount := strings.Count(line, "\t")
	commaCount := strings.Count(line, ",")
	if tabCount > commaCount {
		return '\t'
	}
	return ','
}

func parseCsvHeader(header []string) (map[string]int, string) {
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(col)] = i
	}
	required := []string{"P1", "P2", "P3", "Patm", "Tatm"}
	for _, name := range required {
		if _, ok := colMap[name]; !ok {
			return nil, fmt.Sprintf("缺少必要列: %s", name)
		}
	}
	return colMap, ""
}

func parseCsvRows(rows [][]string, colMap map[string]int) ([]three_interp.InterpolationInput, []string) {
	colCount := len(colMap)
	patmIdx := colMap["Patm"]
	tatmIdx := colMap["Tatm"]

	datas := make([]three_interp.InterpolationInput, 0, len(rows))
	var warnings []string

	for rowIdx := 0; rowIdx < len(rows); rowIdx++ {
		row := rows[rowIdx]
		csvLine := rowIdx + 2

		if len(row) < colCount {
			warnings = append(warnings, fmt.Sprintf("第%d行列数不足", csvLine))
			continue
		}

		p1, err1 := strconv.ParseFloat(row[colMap["P1"]], 64)
		p2, err2 := strconv.ParseFloat(row[colMap["P2"]], 64)
		p3, err3 := strconv.ParseFloat(row[colMap["P3"]], 64)
		if err1 != nil || err2 != nil || err3 != nil {
			warnings = append(warnings, fmt.Sprintf("第%d行压力值解析失败", csvLine))
			continue
		}

		patm, errPatm := strconv.ParseFloat(row[patmIdx], 64)
		if errPatm != nil {
			warnings = append(warnings, fmt.Sprintf("第%d行 Patm 解析失败", csvLine))
			continue
		}

		tatm, errTatm := strconv.ParseFloat(row[tatmIdx], 64)
		if errTatm != nil {
			warnings = append(warnings, fmt.Sprintf("第%d行 Tatm 解析失败", csvLine))
			continue
		}

		datas = append(datas, three_interp.InterpolationInput{
			P1: p1, P2: p2, P3: p3, PAtm: patm, TAtm: tatm,
		})
	}

	return datas, warnings
}

func findHelpDoc() string {
	candidates := []string{
		"docs/用户说明书.html",
		"../docs/用户说明书.html",
		"../../docs/用户说明书.html",
	}
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "docs", "用户说明书.html"),
			filepath.Join(exeDir, "..", "docs", "用户说明书.html"),
		)
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return ""
}

func formatVal(v float64) string {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return "-"
	}
	return fmt.Sprintf("%.4f", v)
}

func formatNum(v float64) string {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return "-"
	}
	intPart := int64(math.Abs(v))
	frac := math.Abs(v) - float64(intPart)
	if frac < 0.01 {
		return formatWithCommas(int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

func formatWithCommas(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var parts []string
	for i := len(s); i > 0; i -= 3 {
		start := i - 3
		if start < 0 {
			start = 0
		}
		parts = append([]string{s[start:i]}, parts...)
	}
	result := strings.Join(parts, ",")
	if neg {
		result = "-" + result
	}
	return result
}
