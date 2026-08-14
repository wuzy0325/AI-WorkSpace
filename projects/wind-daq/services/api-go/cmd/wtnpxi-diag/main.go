// WTN-PXI 球罐数据采集设备通讯协议调试工具。
//
// 协议定义（依据现场抓包 temp/log.txt 校准，可能偏离 wind-daq wtn_pxi.go 旧假设）：
//   - TCP 连接，设备主动推送数据流
//   - 每帧 = 4 字节大端长度前缀（payload 字节数）+ payload
//   - 数据帧 payload = 2 字节协议前缀 + 4 字节大端数组长度 + N × double（大端）
//   - 连接建立后首个 20B 帧为设备信息帧（TLV：记录数 + 若干(长度+数据)，含 "crio" 设备名），
//     不是通道数据，不应解析为通道值
//   - 兜底：仍支持 N × float32（小端）旧格式（模拟服务器 / 其他设备）
//
// 用法：
//
//	wtnpxi-diag.exe [-host 192.168.1.100] [-port 9000] [-detail] [-raw] [-log out.bin]
//
// 交互命令（stdin 回车触发）：
//
//	<hex>         发送十六进制字节，如 00 01 02 或 000102（只含 0-9a-f 时按 hex 发送）
//	ascii <文本>  发送 ASCII 文本（自动追加 \r\n）
//	d             切换逐帧详情模式
//	p / r         暂停 / 恢复打印（接收不受影响）
//	q             退出
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"
)

const (
	defaultHost = "127.0.0.1"
	defaultPort = 9000
	// maxPayload 与 WTN_PXI_MAX_PAYLOAD_BYTES 对齐。
	maxPayload = 64 * 1024
	// requiredValues 与 WTN_PXI_REQUIRED_CHANNELS 对齐。
	requiredValues = 8
	readBufSize    = 16 * 1024
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

type toolState struct {
	printMu    sync.Mutex
	paused     atomic.Bool
	detail     atomic.Bool
	frames     atomic.Uint64
	bytes      atomic.Uint64
	resyncs    atomic.Uint64
	lastValues atomic.Value // []float64
}

func newState() *toolState {
	st := &toolState{}
	st.lastValues.Store([]float64{})
	return st
}

func main() {
	host := flag.String("host", defaultHost, "设备 IP 地址")
	port := flag.Int("port", defaultPort, "设备 TCP 端口")
	timeout := flag.Duration("timeout", 5*time.Second, "连接超时")
	raw := flag.Bool("raw", false, "原始 hexdump 模式（不做帧解析）")
	detail := flag.Bool("detail", false, "启动即进入逐帧详情模式")
	logPath := flag.String("log", "", "把接收到的原始字节写入此文件")
	flag.Parse()

	addr := net.JoinHostPort(*host, strconv.Itoa(*port))
	setupConsole()
	printBanner(addr, *raw)

	conn, err := net.DialTimeout("tcp", addr, *timeout)
	if err != nil {
		fmt.Printf("[连接失败] %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Printf("[已连接] %s -> %s\n", conn.LocalAddr(), conn.RemoteAddr())

	var logFile *os.File
	if *logPath != "" {
		logFile, err = os.Create(*logPath)
		if err != nil {
			fmt.Printf("[警告] 无法创建日志文件 %s: %v\n", *logPath, err)
		} else {
			defer logFile.Close()
			fmt.Printf("[日志] 原始字节将写入 %s\n", *logPath)
		}
	}

	st := newState()
	st.detail.Store(*detail)

	done := make(chan struct{})
	if *raw {
		go receiveRaw(conn, logFile, st, done)
	} else {
		go receiveParsed(conn, logFile, st, done)
	}
	go summaryTicker(st, done)

	interactive(conn, st, done)
}

func setupConsole() {
	if runtime.GOOS != "windows" {
		return
	}
	// 交互终端默认代码页（GBK/cp936）无法正确渲染 UTF-8 中文输出，
	// 切换到 UTF-8 代码页避免中文乱码。仅在连接真实控制台时生效。
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)
}

func printBanner(addr string, raw bool) {
	mode := "帧解析（长度前缀 + float32 通道）"
	if raw {
		mode = "原始 hexdump"
	}
	fmt.Println("==============================================")
	fmt.Println(" WTN-PXI 球罐数据采集设备通讯协议调试工具")
	fmt.Println("==============================================")
	fmt.Printf(" 目标: %s\n", addr)
	fmt.Printf(" 模式: %s\n", mode)
	fmt.Println(" 协议: 4 字节大端长度前缀 + N×float32(小端)")
	fmt.Println("----------------------------------------------")
	printHelp()
	fmt.Println("----------------------------------------------")
}

func printHelp() {
	fmt.Println(" 交互命令:")
	fmt.Println("   <hex>         发送十六进制字节，如: 00 01 02 或 000102")
	fmt.Println("   ascii <文本>  发送 ASCII 文本（追加 \\r\\n）")
	fmt.Println("   d             切换逐帧详情模式")
	fmt.Println("   p / r         暂停 / 恢复打印")
	fmt.Println("   q             退出")
}

func interactive(conn net.Conn, st *toolState, done <-chan struct{}) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case lower == "q" || lower == "quit" || lower == "exit":
			fmt.Println(">>> 退出")
			return
		case lower == "p" || lower == "pause":
			st.paused.Store(true)
			fmt.Println(">>> 已暂停打印（仍在接收）")
		case lower == "r" || lower == "resume":
			st.paused.Store(false)
			fmt.Println(">>> 已恢复打印")
		case lower == "d" || lower == "detail":
			on := !st.detail.Load()
			st.detail.Store(on)
			fmt.Printf(">>> 详情模式: %v\n", on)
		case lower == "h" || lower == "help":
			printHelp()
		case strings.HasPrefix(lower, "ascii "):
			text := line[len("ascii "):]
			sendBytes(conn, append([]byte(text), '\r', '\n'))
		default:
			if b, ok := parseHex(line); ok {
				sendBytes(conn, b)
			} else {
				sendBytes(conn, []byte(line))
			}
		}
	}
	// stdin 已关闭（无交互输入，如被脚本重定向）：
	// 保持运行直到连接结束，避免一启动就退出，错过接收数据。
	<-done
}

func sendBytes(conn net.Conn, data []byte) {
	n, err := conn.Write(data)
	if err != nil {
		fmt.Printf(">>> 发送失败: %v\n", err)
		return
	}
	fmt.Printf(">>> 已发送 %d 字节: %s\n", n, hexify(data))
}

func receiveParsed(conn net.Conn, logFile *os.File, st *toolState, done chan struct{}) {
	defer close(done)
	buf := make([]byte, readBufSize)
	parseBuf := make([]byte, 0, 64*1024)
	detailCount := 0

	for {
		n, err := conn.Read(buf)
		if n > 0 {
			if logFile != nil {
				logFile.Write(buf[:n])
			}
			st.bytes.Add(uint64(n))
			parseBuf = append(parseBuf, buf[:n]...)
			for {
				payload, consumed, perr := parseFrame(parseBuf)
				if perr == io.ErrShortBuffer {
					break
				}
				parseBuf = parseBuf[consumed:]
				if perr != nil {
					reportResync(st)
					continue
				}
				frameNo := st.frames.Add(1)
				decodeAndStore(payload, st)
				if st.paused.Load() {
					continue
				}
				if st.detail.Load() || detailCount < 3 {
					detailCount++
					printDetailFrame(frameNo, payload, st)
				}
			}
		}
		if err != nil {
			fmt.Printf("[连接关闭] %v\n", err)
			return
		}
	}
}

func receiveRaw(conn net.Conn, logFile *os.File, st *toolState, done chan struct{}) {
	defer close(done)
	buf := make([]byte, readBufSize)
	var total uint64

	for {
		n, err := conn.Read(buf)
		if n > 0 {
			if logFile != nil {
				logFile.Write(buf[:n])
			}
			st.bytes.Add(uint64(n))
			if st.paused.Load() {
				total += uint64(n)
				continue
			}
			off := int(total)
			for i := 0; i < n; i += 16 {
				end := i + 16
				if end > n {
					end = n
				}
				fmt.Println(hexdumpLine(off+i, buf[i:end]))
			}
			total += uint64(n)
		}
		if err != nil {
			fmt.Printf("[连接关闭] %v\n", err)
			return
		}
	}
}

func summaryTicker(st *toolState, done <-chan struct{}) {
	prevFrames, prevBytes := uint64(0), uint64(0)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if st.paused.Load() {
				continue
			}
			f := st.frames.Load()
			b := st.bytes.Load()
			printSummary(f-prevFrames, b-prevBytes, f, b, st)
			prevFrames, prevBytes = f, b
		}
	}
}

// parseFrame 从 buffer 解析一帧 WTN-PXI 数据（对齐 wind-daq processData 的组帧/重同步逻辑）。
//
// 返回：
//   - payload 非 nil：完整帧，consumed = 4 + len(payload)
//   - err == errResync：长度前缀无效（0 或超长），consumed = 1（丢弃 1 字节重同步）
//   - err == io.ErrShortBuffer：帧未完整，consumed = 0（等待更多数据）
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

func decodeAndStore(payload []byte, st *toolState) {
	vals := decodePayload(payload)
	if len(vals) > 0 {
		st.lastValues.Store(vals)
	}
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

func reportResync(st *toolState) {
	n := st.resyncs.Add(1)
	if n <= 5 || n%1000 == 0 {
		fmt.Printf("[重同步] 无效长度前缀，丢弃 1 字节（累计 %d 次）\n", n)
	}
}

func printDetailFrame(frameNo uint64, payload []byte, st *toolState) {
	vals := decodePayload(payload)
	info := parseInfoFrame(payload)
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, uint32(len(payload)))

	st.printMu.Lock()
	defer st.printMu.Unlock()
	fmt.Printf("=== 帧 #%d  %s  payload=%dB (%d 值) ===\n",
		frameNo, time.Now().Format("15:04:05.000"), len(payload), len(vals))
	fmt.Printf("  长度前缀: %s\n", hexify(prefix))
	for _, line := range hexDumpLines(payload) {
		fmt.Printf("  hex: %s\n", line)
	}
	if info != "" {
		fmt.Printf("  [设备信息] %s\n", info)
		fmt.Println()
		return
	}
	if len(vals) < requiredValues {
		fmt.Printf("  [警告] 值数量 %d < %d（wind-daq 要求至少 %d 路）\n",
			len(vals), requiredValues, requiredValues)
	}
	for i, v := range vals {
		ch := channelName(i)
		fmt.Printf("  [%d] %-8s %12.2f %s\n", i, ch.name, v, ch.unit)
	}
	fmt.Println()
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

func printSummary(deltaFrames, deltaBytes, totalFrames, totalBytes uint64, st *toolState) {
	st.printMu.Lock()
	defer st.printMu.Unlock()
	line := fmt.Sprintf("[%s] 帧=%d (+%d) 字节=%s (+%s) 速率=%d 帧/s 重同步=%d",
		time.Now().Format("15:04:05"),
		totalFrames, deltaFrames,
		humanBytes(totalBytes), humanBytes(deltaBytes),
		deltaFrames, st.resyncs.Load())
	if vals, ok := st.lastValues.Load().([]float64); ok && len(vals) > 0 {
		line += " | " + formatValues(vals)
	}
	fmt.Println(line)
}

func formatValues(vals []float64) string {
	parts := make([]string, 0, len(vals))
	for i, v := range vals {
		parts = append(parts, fmt.Sprintf("CH%d=%.2f", i, v))
	}
	return strings.Join(parts, " ")
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

func hexdumpLine(offset int, b []byte) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%08X  ", offset)
	for i := 0; i < 16; i++ {
		if i < len(b) {
			fmt.Fprintf(&sb, "%02X ", b[i])
		} else {
			sb.WriteString("   ")
		}
		if i == 7 {
			sb.WriteString(" ")
		}
	}
	sb.WriteString(" |")
	for i := 0; i < 16; i++ {
		if i < len(b) {
			c := b[i]
			if c < 0x20 || c > 0x7E {
				c = '.'
			}
			sb.WriteByte(c)
		} else {
			sb.WriteByte(' ')
		}
	}
	sb.WriteString("|")
	return sb.String()
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
