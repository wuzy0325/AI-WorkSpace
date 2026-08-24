// Command device-scan-gui 是 WindLabX4 设备扫描工具的图形化版本。
//
// 与 CLI `device-scan` 复用同一套扫描逻辑（internal/adapters/scan.NetworkScanner），
// 提供一个 Windows 原生窗口：选择扫描范围（网卡 / 子网）后点「开始扫描」，
// 在表格中列出局域网内的 DAQ 设备（DAQ-P-1604 / DAQ-T-1603 / DAQ-P-1604Pre）。
//
// 构建无控制台窗口版：
//
//	go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o device-scan-gui.exe ./cmd/device-scan-gui
//
// 依赖 cmd/device-scan-gui/app.manifest + rsrc.syso（用 akavel/rsrc 生成）。
package main

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"windlabx4/services/api-go/internal/adapters/scan"
	"windlabx4/services/api-go/internal/core/device"
)

const defaultScanTimeout = 3 * time.Second

// row 是 TableView 中一行的显示数据。
type row struct {
	Type     string
	Address  string
	Port     int
	MAC      string
	Firmware string
	Model    string
}

// rowsModel 适配 walk.ReflectTableModel。它把 Items() 返回的 []*row 用反射映射到
// 表格列（列顺序与声明一致）。数据变更后调用 SetRows 并 PublishRowsReset 通知刷新。
type rowsModel struct {
	walk.ReflectTableModelBase
	items []*row
}

func (m *rowsModel) Items() interface{} { return m.items }

// SetRows 替换全部数据并通知表格刷新。必须在 UI 线程调用。
func (m *rowsModel) SetRows(rows []*row) {
	m.items = rows
	m.PublishRowsReset()
}

type uiRefs struct {
	scopeCombo  *walk.ComboBox
	ifaceCombo  *walk.ComboBox
	subnetEdit  *walk.LineEdit
	scanBtn     *walk.PushButton
	status      *walk.Label
	table       *walk.TableView
	log         *walk.TextEdit
}

// appState 持有 UI 与扫描线程之间的共享状态。
// 说明：walk 要求 UI 控件只能在 UI 线程访问。因此后台扫描 goroutine 只写共享
// 状态（scanning/logLines/pending），UI 刷新循环（uiLoop，运行于 UI 线程）负责
// 把状态应用到控件。
type appState struct {
	mu       sync.Mutex
	scanning bool
	model    *rowsModel
	logLines []string

	// pending 由后台 goroutine 写入、uiLoop 读取：非 nil 表示有新结果待应用。
	pendingResults []*row
	pendingErr     string
}

func newAppState(model *rowsModel) *appState {
	return &appState{model: model}
}

func (s *appState) pushLog(line string) {
	s.mu.Lock()
	s.logLines = append(s.logLines, line)
	if len(s.logLines) > 200 {
		s.logLines = s.logLines[len(s.logLines)-200:]
	}
	s.mu.Unlock()
}

func (s *appState) setScanning(on bool) {
	s.mu.Lock()
	s.scanning = on
	s.mu.Unlock()
}

func (s *appState) isScanning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanning
}

// startScan 由 UI 线程（OnClicked）调用：仅做范围校验并启动后台 goroutine。
func (s *appState) startScan(ui *uiRefs) {
	if s.isScanning() {
		return
	}

	// 读取 UI 控件（UI 线程，安全）。
	scopeIdx := ui.scopeCombo.CurrentIndex()
	ifaceName := ""
	if scopeIdx == 1 { // 按网卡
		ifaceName = ui.ifaceCombo.Text()
	}
	subnet := ""
	if scopeIdx == 2 { // 按子网
		subnet = strings.TrimSpace(ui.subnetEdit.Text())
	}

	targets, err := scan.ScopedDiscoveryTargets(ifaceName, subnet)
	if err != nil {
		s.pushLog("[范围错误] " + err.Error())
		return
	}
	scopeDesc := "全网卡广播"
	if ifaceName != "" {
		scopeDesc = "网卡 " + ifaceName
	} else if subnet != "" {
		scopeDesc = "子网 " + subnet
	}

	s.setScanning(true)
	s.pushLog(fmt.Sprintf("[扫描] 范围=%s, timeout=%v", scopeDesc, defaultScanTimeout))

	go func() {
		opts := []scan.NetworkScannerOption{scan.WithTimeout(defaultScanTimeout)}
		if len(targets) > 0 {
			opts = append(opts, scan.WithTargets(targets...))
		}
		scanner := scan.NewNetworkScanner(opts...)
		results, err := scanner.Scan()

		s.mu.Lock()
		if err != nil {
			s.pendingErr = err.Error()
			s.pushLogLocked("[扫描失败] " + err.Error())
		} else {
			s.pendingResults = resultsToRows(results)
			s.pendingErr = ""
			s.pushLogLocked(fmt.Sprintf("[扫描完成] 共发现 %d 台设备", len(results)))
		}
		s.scanning = false
		s.mu.Unlock()
	}()
}

// pushLogLocked 需持有 s.mu。
func (s *appState) pushLogLocked(line string) {
	s.logLines = append(s.logLines, line)
	if len(s.logLines) > 200 {
		s.logLines = s.logLines[len(s.logLines)-200:]
	}
}

// applyPending 在 UI 线程调用，把后台 goroutine 产出的结果应用到控件。
func (s *appState) applyPending(ui *uiRefs) {
	s.mu.Lock()
	rows := s.pendingResults
	errMsg := s.pendingErr
	s.pendingResults = nil
	s.pendingErr = ""
	s.mu.Unlock()

	if rows != nil {
		s.model.SetRows(rows)
	}
	switch {
	case errMsg != "":
		ui.status.SetText("扫描失败")
	case rows != nil:
		ui.status.SetText(fmt.Sprintf("共发现 %d 台设备", len(rows)))
	default:
		// 无新结果，保持现状
	}
}

// resultsToRows 把 ScanResult 转为表格行指针，并按地址排序便于阅读。
func resultsToRows(results []device.ScanResult) []*row {
	rows := make([]*row, 0, len(results))
	for _, r := range results {
		rows = append(rows, &row{
			Type:     string(r.Type),
			Address:  r.Address,
			Port:     r.Port,
			MAC:      r.MacAddress,
			Firmware: r.FirmwareVersion,
			Model:    r.Model,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Address < rows[j].Address })
	return rows
}

func listUpInterfaces() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var names []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		names = append(names, iface.Name)
	}
	sort.Strings(names)
	return names
}

func buildWindow(app *appState, ui *uiRefs) (*walk.MainWindow, error) {
	var mw *walk.MainWindow
	wm := MainWindow{
		AssignTo: &mw,
		Title:    "WindLabX4 设备扫描",
		MinSize:  Size{Width: 720, Height: 560},
		Size:     Size{Width: 760, Height: 600},
		Layout:   VBox{Spacing: 6, Margins: Margins{Left: 10, Top: 10, Right: 10, Bottom: 10}},
		Children: []Widget{
			Composite{
				Layout: HBox{Spacing: 4},
				Children: []Widget{
					Label{Text: "范围:"},
					ComboBox{
						AssignTo: &ui.scopeCombo,
						Editable: false,
						Model:    []string{"全网卡广播", "指定网卡", "指定子网"},
						CurrentIndex: 0,
						OnCurrentIndexChanged: func() {
							idx := ui.scopeCombo.CurrentIndex()
							ui.ifaceCombo.SetVisible(idx == 1)
							ui.subnetEdit.SetVisible(idx == 2)
						},
					},
					ComboBox{
						AssignTo: &ui.ifaceCombo,
						Editable: true,
						Model:    listUpInterfaces(),
						MinSize:  Size{Width: 160},
						Visible:  false,
					},
					LineEdit{
						AssignTo: &ui.subnetEdit,
						Text:     "",
						MinSize:  Size{Width: 140},
						Visible:  false,
					},
					PushButton{
						AssignTo: &ui.scanBtn,
						Text:     "开始扫描",
						OnClicked: func() {
							if !app.isScanning() {
								app.startScan(ui)
							}
						},
					},
					Label{AssignTo: &ui.status, Text: "就绪"},
				},
			},
			TableView{
				AssignTo: &ui.table,
				Model:    app.model,
				Columns: []TableViewColumn{
					{Title: "类型", DataMember: "Type", Width: 120},
					{Title: "IP", DataMember: "Address", Width: 130},
					{Title: "端口", DataMember: "Port", Width: 60},
					{Title: "MAC", DataMember: "MAC", Width: 160},
					{Title: "固件", DataMember: "Firmware", Width: 80},
					{Title: "型号", DataMember: "Model", Width: 120},
				},
			},
			TextEdit{
				AssignTo: &ui.log,
				ReadOnly: true,
				HScroll:  true,
				VScroll:  true,
				MinSize:  Size{Width: 0, Height: 120},
				Font:     Font{Family: "Consolas", PointSize: 9},
			},
		},
	}
	if err := wm.Create(); err != nil {
		return nil, err
	}
	return mw, nil
}

// uiLoop 周期性刷新日志区、扫描按钮状态与扫描结果（UI 线程）。
func uiLoop(done <-chan struct{}, app *appState, ui *uiRefs) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// 应用后台扫描结果到表格/状态。
			app.applyPending(ui)

			// 刷新日志。
			app.mu.Lock()
			text := strings.Join(app.logLines, "\n")
			app.mu.Unlock()
			if ui.log.Text() != text {
				ui.log.SetText(text)
			}

			// 扫描按钮状态：扫描中禁用，结束恢复。
			if app.isScanning() {
				if ui.scanBtn.Enabled() {
					ui.scanBtn.SetEnabled(false)
					ui.status.SetText("正在扫描...")
				}
			} else if !ui.scanBtn.Enabled() {
				ui.scanBtn.SetEnabled(true)
			}
		}
	}
}

func main() {
	app := newAppState(&rowsModel{})
	ui := &uiRefs{}
	mw, err := buildWindow(app, ui)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建窗口失败:", err)
		os.Exit(1)
	}
	done := make(chan struct{})
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		close(done)
	})
	go uiLoop(done, app, ui)
	mw.Run()
}
