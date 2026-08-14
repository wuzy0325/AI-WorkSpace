// WTN-PXI 球罐数据采集设备通讯协议调试工具（GUI 版）。
//
// 协议定义（依据现场抓包 temp/log.txt 校准，可能偏离 wind-daq wtn_pxi.go 旧假设）：
//   - TCP 连接，设备主动推送数据流
//   - 每帧 = 4 字节大端长度前缀（payload 字节数）+ payload
//   - 数据帧 payload = 2 字节协议前缀 + 4 字节大端数组长度 + N × double（大端）
//   - 连接建立后首个 20B 帧为设备信息帧（TLV：记录数 + 若干(长度+数据)，含 "crio" 设备名），
//     不是通道数据，不应解析为通道值
//   - 兜底：仍支持 N × float32（小端）旧格式（模拟服务器 / 其他设备）
//
// 窗口界面：连接面板、8 路通道实时值、日志区、发送框。
// 构建无控制台窗口版：go build -ldflags "-H windowsgui"
package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const (
	defaultHost = "127.0.0.1"
	defaultPort = 9000
	// maxPayload 与 WTN_PXI_MAX_PAYLOAD_BYTES 对齐。
	maxPayload = 64 * 1024
	// requiredValues 与 WTN_PXI_REQUIRED_CHANNELS 对齐。
	requiredValues = 8
	// dialTimeout 连接超时。
	dialTimeout = 5 * time.Second
	// uiTickInterval 界面刷新周期。
	uiTickInterval = 150 * time.Millisecond
	// logCapChars 日志区超过该长度后清空（帧数据可用「保存原始字节」落盘）。
	logCapChars = 300000
	// displayChannels 界面通道显示数量（真实设备数据帧 12 路）。
	displayChannels = 12
)

var errResync = errors.New("invalid length prefix, resync")

type channelInfo struct {
	name string
	unit string
}

// wtnChannels 与 default_profiles.go 的 defaultWTNPXIChannels 对齐。
var wtnChannels = []channelInfo{
	{"球罐压力", "Pa"},
	{"球罐总压", "Pa"},
	{"球罐静压", "Pa"},
	{"球罐稳定时间", "s"},
	{"球罐温度1", "degC"},
	{"球罐温度2", "degC"},
	{"球罐温度3", "degC"},
	{"球罐温度4", "degC"},
}

func channelName(i int) channelInfo {
	if i >= 0 && i < len(wtnChannels) {
		return wtnChannels[i]
	}
	return channelInfo{fmt.Sprintf("CH%d", i), ""}
}

type appState struct {
	mu             sync.Mutex
	conn           net.Conn
	connected      bool
	connecting     bool
	connectErr     string
	userDisconnect bool

	values      []float64
	frames      uint64
	bytes       uint64
	resyncs     uint64
	prevFrames  uint64
	prevBytes   uint64
	detailShown int

	paused  bool
	detail  bool
	saveLog bool
	logFile *os.File

	logQueue    []string
	lastSummary time.Time
}

func newAppState() *appState {
	return &appState{lastSummary: time.Now()}
}

func (s *appState) pushLog(line string) {
	s.mu.Lock()
	s.logQueue = append(s.logQueue, line)
	if len(s.logQueue) > 1000 {
		s.logQueue = s.logQueue[len(s.logQueue)-1000:]
	}
	s.mu.Unlock()
}

func (s *appState) dial(host string, port int) {
	s.mu.Lock()
	s.connecting = true
	s.connectErr = ""
	s.userDisconnect = false
	s.mu.Unlock()

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), dialTimeout)
	if err != nil {
		s.mu.Lock()
		s.connecting = false
		s.connectErr = err.Error()
		s.mu.Unlock()
		s.pushLog("[连接失败] " + err.Error())
		return
	}

	s.mu.Lock()
	if s.userDisconnect {
		s.mu.Unlock()
		conn.Close()
		return
	}
	s.conn = conn
	s.connected = true
	s.connecting = false
	s.frames, s.bytes, s.resyncs, s.prevFrames, s.prevBytes = 0, 0, 0, 0, 0
	s.detailShown = 0
	s.values = nil
	s.lastSummary = time.Now()
	s.mu.Unlock()

	s.pushLog(fmt.Sprintf("[已连接] %s -> %s", conn.LocalAddr(), conn.RemoteAddr()))
	go s.readLoop(conn)
}

func (s *appState) disconnect() {
	s.mu.Lock()
	s.userDisconnect = true
	conn := s.conn
	s.conn = nil
	s.connected = false
	s.mu.Unlock()

	if conn != nil {
		conn.Close()
	}
	s.pushLog("[已断开]")
}

func (s *appState) readLoop(conn net.Conn) {
	buf := make([]byte, 16*1024)
	parseBuf := make([]byte, 0, 64*1024)

	for {
		n, err := conn.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.bytes += uint64(n)
			if s.saveLog && s.logFile != nil {
				s.logFile.Write(buf[:n])
			}
			s.mu.Unlock()

			parseBuf = append(parseBuf, buf[:n]...)
			for {
				payload, consumed, perr := parseFrame(parseBuf)
				if perr == io.ErrShortBuffer {
					break
				}
				parseBuf = parseBuf[consumed:]
				if perr != nil {
					s.mu.Lock()
					s.resyncs++
					r := s.resyncs
					s.mu.Unlock()
					if r <= 5 || r%1000 == 0 {
						s.pushLog(fmt.Sprintf("[重同步] 无效长度前缀，丢弃 1 字节（累计 %d 次）", r))
					}
					continue
				}
				s.mu.Lock()
				s.frames++
				frameNo := s.frames
				detail := s.detail
				detailCount := s.detailShown
				if detail || detailCount < 3 {
					s.detailShown = detailCount + 1
				}
				vals := decodePayload(payload)
				if len(vals) > 0 {
					s.values = vals
				}
				paused := s.paused
				s.mu.Unlock()

				if !paused && (detail || detailCount < 3) {
					s.pushLog(buildFrameDetail(frameNo, payload))
				}
			}
		}
		if err != nil {
			s.mu.Lock()
			user := s.userDisconnect
			if !user {
				s.connected = false
				s.conn = nil
			}
			s.mu.Unlock()
			if !user {
				s.pushLog(fmt.Sprintf("[连接关闭] %v", err))
			}
			return
		}
	}
}

func (s *appState) enableSaveLog() {
	path := filepath.Join(".", fmt.Sprintf("wtnpxi-raw-%s.bin", time.Now().Format("20060102-150405")))
	f, err := os.Create(path)
	if err != nil {
		s.pushLog("[保存日志失败] " + err.Error())
		return
	}
	s.mu.Lock()
	s.logFile = f
	s.saveLog = true
	s.mu.Unlock()
	s.pushLog("[保存原始字节] -> " + path)
}

func (s *appState) disableSaveLog() {
	s.mu.Lock()
	f := s.logFile
	s.logFile = nil
	s.saveLog = false
	s.mu.Unlock()
	if f != nil {
		f.Close()
	}
	s.pushLog("[停止保存原始字节]")
}

func (s *appState) doSend(ui *uiRefs) {
	text := strings.TrimSpace(ui.send.Text())
	if text == "" {
		return
	}
	var data []byte
	if b, ok := parseHex(text); ok {
		data = b
	} else {
		data = []byte(text)
	}

	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn == nil {
		s.pushLog(">>> 未连接，无法发送")
		return
	}
	n, err := conn.Write(data)
	if err != nil {
		s.pushLog(">>> 发送失败: " + err.Error())
		return
	}
	s.pushLog(fmt.Sprintf(">>> 已发送 %d 字节: %s", n, hexify(data)))
	ui.send.SetText("")
}

type uiRefs struct {
	host, port          *walk.LineEdit
	connBtn, discBtn    *walk.PushButton
	pauseBtn, detailBtn *walk.PushButton
	saveLogCB           *walk.CheckBox
	status              *walk.Label
	values              []*walk.Label
	log                 *walk.TextEdit
	send                *walk.LineEdit
}

func buildWindow(app *appState) (*walk.MainWindow, *uiRefs) {
	ui := &uiRefs{values: make([]*walk.Label, displayChannels)}

	channelChildren := make([]Widget, 0, 3+displayChannels*3)
	channelChildren = append(channelChildren,
		Label{Text: "通道"},
		Label{Text: "数值", TextAlignment: AlignFar},
		Label{Text: "单位"},
	)
	for i := 0; i < displayChannels; i++ {
		ch := channelName(i)
		channelChildren = append(channelChildren,
			Label{Text: ch.name},
			Label{AssignTo: &ui.values[i], Text: "-", TextAlignment: AlignFar},
			Label{Text: ch.unit},
		)
	}

	var mw *walk.MainWindow
	wm := MainWindow{
		AssignTo: &mw,
		Title:    "WTN-PXI 协议调试工具",
		MinSize:  Size{Width: 620, Height: 780},
		Size:     Size{Width: 620, Height: 780},
		Layout:   VBox{Spacing: 6},
		Children: []Widget{
			Composite{
				Layout: HBox{Spacing: 4},
				Children: []Widget{
					Label{Text: "主机:"},
					LineEdit{AssignTo: &ui.host, Text: defaultHost, MinSize: Size{Width: 130}},
					Label{Text: "端口:"},
					LineEdit{AssignTo: &ui.port, Text: strconv.Itoa(defaultPort), MinSize: Size{Width: 60}},
					PushButton{
						AssignTo: &ui.connBtn, Text: "连接",
						OnClicked: func() {
							host := strings.TrimSpace(ui.host.Text())
							if host == "" {
								host = defaultHost
							}
							port, err := strconv.Atoi(strings.TrimSpace(ui.port.Text()))
							if err != nil || port <= 0 {
								port = defaultPort
							}
							app.pushLog(fmt.Sprintf(">>> 正在连接 %s:%d ...", host, port))
							go app.dial(host, port)
						},
					},
					PushButton{
						AssignTo: &ui.discBtn, Text: "断开",
						OnClicked: func() { app.disconnect() },
					},
					Label{AssignTo: &ui.status, Text: "已断开"},
				},
			},
			Composite{
				Layout:   Grid{Columns: 3, Spacing: 4},
				Children: channelChildren,
			},
			Composite{
				Layout: HBox{Spacing: 4},
				Children: []Widget{
					PushButton{
						AssignTo: &ui.pauseBtn, Text: "暂停显示",
						OnClicked: func() {
							app.mu.Lock()
							app.paused = !app.paused
							p := app.paused
							app.mu.Unlock()
							if p {
								ui.pauseBtn.SetText("恢复显示")
							} else {
								ui.pauseBtn.SetText("暂停显示")
							}
							if p {
								app.pushLog("已暂停打印（仍在接收）")
							} else {
								app.pushLog("已恢复打印")
							}
						},
					},
					PushButton{
						AssignTo: &ui.detailBtn, Text: "详情:关",
						OnClicked: func() {
							app.mu.Lock()
							app.detail = !app.detail
							d := app.detail
							app.mu.Unlock()
							if d {
								ui.detailBtn.SetText("详情:开")
							} else {
								ui.detailBtn.SetText("详情:关")
							}
							app.pushLog(fmt.Sprintf("详情模式: %v", d))
						},
					},
					CheckBox{
						AssignTo: &ui.saveLogCB, Text: "保存原始字节",
						OnCheckedChanged: func() {
							if ui.saveLogCB.Checked() {
								app.enableSaveLog()
							} else {
								app.disableSaveLog()
							}
						},
					},
				},
			},
			TextEdit{
				AssignTo: &ui.log,
				ReadOnly: true,
				HScroll:  true,
				VScroll:  true,
				Font:     Font{Family: "Consolas", PointSize: 9},
			},
			Composite{
				Layout: HBox{Spacing: 4},
				Children: []Widget{
					Label{Text: "发送:"},
					LineEdit{
						AssignTo: &ui.send,
						OnKeyDown: func(key walk.Key) {
							if key == walk.KeyReturn {
								app.doSend(ui)
							}
						},
					},
					PushButton{
						Text:      "发送",
						OnClicked: func() { app.doSend(ui) },
					},
					Label{Text: "（hex 或 ASCII）"},
				},
			},
		},
	}

	if err := wm.Create(); err != nil {
		fmt.Fprintln(os.Stderr, "创建窗口失败:", err)
		os.Exit(1)
	}
	return mw, ui
}

func uiLoop(mw *walk.MainWindow, app *appState, ui *uiRefs) {
	ticker := time.NewTicker(uiTickInterval)
	defer ticker.Stop()
	for range ticker.C {
		mw.Synchronize(func() { uiTick(app, ui) })
	}
}

func uiTick(app *appState, ui *uiRefs) {
	var lines []string
	var values []float64
	var statusText string
	var connEnabled, discEnabled bool

	app.mu.Lock()
	connected := app.connected
	connecting := app.connecting
	connectErr := app.connectErr
	values = append([]float64(nil), app.values...)
	lines = app.logQueue
	app.logQueue = nil

	now := time.Now()
	if connected && !app.paused && now.Sub(app.lastSummary) >= time.Second {
		app.lastSummary = now
		df := app.frames - app.prevFrames
		db := app.bytes - app.prevBytes
		app.prevFrames, app.prevBytes = app.frames, app.bytes
		sum := fmt.Sprintf("[%s] 帧=%d (+%d) 字节=%s (+%s) 速率=%d 帧/s 重同步=%d",
			now.Format("15:04:05"), app.frames, df, humanBytes(app.bytes), humanBytes(db), df, app.resyncs)
		if len(app.values) > 0 {
			sum += " | " + formatValues(app.values)
		}
		lines = append(lines, sum)
	}

	switch {
	case connected:
		statusText = "已连接"
	case connecting:
		statusText = "连接中..."
	case connectErr != "":
		statusText = "连接失败: " + connectErr
	default:
		statusText = "已断开"
	}
	connEnabled = !connected && !connecting
	discEnabled = connected
	app.mu.Unlock()

	ui.status.SetText(statusText)
	ui.connBtn.SetEnabled(connEnabled)
	ui.discBtn.SetEnabled(discEnabled)
	for i := 0; i < displayChannels; i++ {
		if i < len(values) {
			ui.values[i].SetText(formatValue(values[i]))
		} else {
			ui.values[i].SetText("-")
		}
	}
	for _, l := range lines {
		ui.log.AppendText(l + "\r\n")
	}
	if ui.log.TextLength() > logCapChars {
		ui.log.SetText("")
		ui.log.AppendText("[日志过长已清空]（帧数据可勾选「保存原始字节」落盘）\r\n")
	}
}

// parseFrame 从 buffer 解析一帧 WTN-PXI 数据（对齐 wind-daq processData 组帧/重同步逻辑）。
func parseFrame(buffer []byte) (payload []byte, consumed int, err error) {
	if len(buffer) < 4 {
		return nil, 0, io.ErrShortBuffer
	}
	payloadLen := int(buffer[0])<<24 | int(buffer[1])<<16 | int(buffer[2])<<8 | int(buffer[3])
	if payloadLen <= 0 || payloadLen > maxPayload {
		return nil, 1, errResync
	}
	if len(buffer) < 4+payloadLen {
		return nil, 0, io.ErrShortBuffer
	}
	return buffer[4 : 4+payloadLen], 4 + payloadLen, nil
}

func decodePayload(payload []byte) []float64 {
	if vals, ok := decodeFloat64Payload(payload); ok {
		return vals
	}
	if vals, ok := decodeInfoFrame(payload); ok {
		return vals
	}
	return decodeFloat32Payload(payload)
}

// decodeFloat64Payload 识别 LabVIEW 数组数据帧：2 字节协议前缀 +
// 4 字节大端数组长度 + N × double（大端）。
// 长度校验：len(payload) == 6 + count*8，能严格区分 float32 小端帧。
func decodeFloat64Payload(payload []byte) ([]float64, bool) {
	if len(payload) < 6 {
		return nil, false
	}
	count := int(binary.BigEndian.Uint32(payload[2:6]))
	if count <= 0 || count > maxPayload/8 {
		return nil, false
	}
	if len(payload) != 6+count*8 {
		return nil, false
	}
	vals := make([]float64, count)
	for i := 0; i < count; i++ {
		vals[i] = math.Float64frombits(binary.BigEndian.Uint64(payload[6+i*8:]))
	}
	return vals, true
}

// decodeInfoFrame 识别设备信息帧（TLV）：[u32 count] + count × (u32 len + data)，含可打印 ASCII 字段。
// 这类帧不是通道数据，返回 nil 以阻止其覆盖通道值。
func decodeInfoFrame(payload []byte) ([]float64, bool) {
	if len(payload) < 4 {
		return nil, false
	}
	count := int(binary.BigEndian.Uint32(payload[:4]))
	if count <= 0 || count > 64 {
		return nil, false
	}
	off := 4
	printable := false
	for i := 0; i < count; i++ {
		if off+4 > len(payload) {
			return nil, false
		}
		l := int(binary.BigEndian.Uint32(payload[off : off+4]))
		off += 4
		if l <= 0 || off+l > len(payload) {
			return nil, false
		}
		if isPrintableASCII(payload[off : off+l]) {
			printable = true
		}
		off += l
	}
	if off != len(payload) || !printable {
		return nil, false
	}
	return nil, true
}

func isPrintableASCII(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return false
		}
	}
	return true
}

func decodeFloat32Payload(payload []byte) []float64 {
	count := len(payload) / 4
	if count <= 0 {
		return nil
	}
	vals := make([]float64, count)
	for i := 0; i < count; i++ {
		vals[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(payload[i*4:])))
	}
	return vals
}

func buildFrameDetail(frameNo uint64, payload []byte) string {
	vals := decodePayload(payload)
	info := parseInfoFrame(payload)
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== 帧 #%d  %s  payload=%dB (%d 值) ===",
		frameNo, time.Now().Format("15:04:05.000"), len(payload), len(vals))
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, uint32(len(payload)))
	sb.WriteString("\r\n  长度前缀: " + hexify(prefix))
	for _, line := range hexDumpLines(payload) {
		sb.WriteString("\r\n  hex: " + line)
	}
	if info != "" {
		fmt.Fprintf(&sb, "\r\n  [设备信息] %s", info)
		return sb.String()
	}
	if len(vals) < requiredValues {
		fmt.Fprintf(&sb, "\r\n  [警告] 值数量 %d < %d", len(vals), requiredValues)
	}
	for i, v := range vals {
		ch := channelName(i)
		fmt.Fprintf(&sb, "\r\n  [%d] %s %12.2f %s", i, ch.name, v, ch.unit)
	}
	return sb.String()
}

// parseInfoFrame 解析设备信息帧（TLV：[u32 count] + count × (u32 len + data)），
// 返回可打印字段拼接串；不是信息帧时返回空串。
func parseInfoFrame(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	count := int(binary.BigEndian.Uint32(payload[:4]))
	if count <= 0 || count > 64 {
		return ""
	}
	off := 4
	parts := make([]string, 0, count)
	printable := false
	for i := 0; i < count; i++ {
		if off+4 > len(payload) {
			return ""
		}
		l := int(binary.BigEndian.Uint32(payload[off : off+4]))
		off += 4
		if l <= 0 || off+l > len(payload) {
			return ""
		}
		field := payload[off : off+l]
		off += l
		if isPrintableASCII(field) {
			printable = true
			parts = append(parts, string(field))
		} else {
			parts = append(parts, hexify(field))
		}
	}
	if off != len(payload) || !printable {
		return ""
	}
	return strings.Join(parts, ", ")
}

func formatValues(vals []float64) string {
	parts := make([]string, 0, len(vals))
	for i, v := range vals {
		parts = append(parts, fmt.Sprintf("CH%d=%s", i, formatValue(v)))
	}
	return strings.Join(parts, " ")
}

func formatValue(v float64) string {
	av := math.Abs(v)
	if (av > 0 && av < 1e-4) || av >= 1e9 {
		return fmt.Sprintf("%.4g", v)
	}
	return fmt.Sprintf("%.2f", v)
}

func hexify(b []byte) string {
	if len(b) > 32 {
		return strings.ToUpper(hex.EncodeToString(b[:32]))
	}
	return strings.ToUpper(hex.EncodeToString(b))
}

func hexDumpLines(b []byte) []string {
	lines := make([]string, 0, (len(b)+15)/16)
	for i := 0; i < len(b); i += 16 {
		end := i + 16
		if end > len(b) {
			end = len(b)
		}
		var sb strings.Builder
		for j := i; j < end; j++ {
			fmt.Fprintf(&sb, "%02X ", b[j])
		}
		lines = append(lines, strings.TrimRight(sb.String(), " "))
	}
	return lines
}

func humanBytes(n uint64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func parseHex(s string) ([]byte, bool) {
	s = strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(s)
	if s == "" || len(s)%2 != 0 {
		return nil, false
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}

func main() {
	hostFlag := flag.String("host", "", "预填主机地址（默认 127.0.0.1）")
	portFlag := flag.Int("port", 0, "预填端口（默认 9000）")
	autoconnect := flag.Bool("autoconnect", false, "启动后自动连接")
	flag.Parse()

	app := newAppState()
	mw, ui := buildWindow(app)

	if *hostFlag != "" {
		ui.host.SetText(*hostFlag)
	}
	if *portFlag > 0 {
		ui.port.SetText(strconv.Itoa(*portFlag))
	}
	if *autoconnect {
		host := strings.TrimSpace(ui.host.Text())
		if host == "" {
			host = defaultHost
		}
		port, err := strconv.Atoi(strings.TrimSpace(ui.port.Text()))
		if err != nil || port <= 0 {
			port = defaultPort
		}
		app.pushLog(fmt.Sprintf(">>> 正在连接 %s:%d ...", host, port))
		go app.dial(host, port)
	}

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		app.disconnect()
		app.disableSaveLog()
	})

	go uiLoop(mw, app, ui)
	mw.Run()
}
