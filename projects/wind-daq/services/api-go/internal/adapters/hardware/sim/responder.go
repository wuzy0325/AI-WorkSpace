package sim

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// responder.go 为命令式设备实现 CommandResponder。
//
// 命令式设备（DSA3217/T1603/P1604）的 adapter 会发送 SCPI/文本命令并读取响应，
// 模拟器需要识别这些命令并返回合理响应，同时通过 StartStream/StopStream 控制
// 数据帧发送。流式设备（P1064Pre/WTN_PXI）不需要 responder。

// P1604Responder 响应 DAQ-P-1604 的采集控制与单位系数命令。
//
// 命令以纯 ASCII 发送，不带任何换行符（实测设备 w1601 长度前缀模式下
// 将 \r\n 视为命令字符的一部分，对 u01101 等命令返回 N05）。
// 因此模拟器使用空闲模式读取：读到数据后短暂无新字节即认为命令完整。
//
// 支持的命令：
//   - w1601        启用长度前缀模式，回 "A" 帧
//   - u01101       读取全局 EU 压力转换系数，回 "<coeff> " 帧
//   - v01101 <c>   写入 EU 压力转换系数，回 "A" 帧
//   - c 01 1       开始发帧（StartStream）
//   - c 02 1       停止发帧（StopStream）
//
// 单位系数响应均以 2 字节大端长度前缀帧返回，对齐设备 w1601 模式与
// shared/device-sdk/go/protocol 的 FrameReader.ReadFrame 读取逻辑。
type P1604Responder struct {
	mu    sync.Mutex
	coeff float64 // 当前 EU 压力转换系数，默认 psi=1.0
}

// NewP1604Responder 构造 P1604 命令响应器，初始单位系数为 psi（1.0）。
func NewP1604Responder() *P1604Responder { return &P1604Responder{coeff: 1.0} }

// ReadMode 返回空闲模式（adapter 写裸命令，不带分隔符）。
func (r *P1604Responder) ReadMode() ReadMode { return ReadModeIdle }

// prefixedFrame 构造带 2 字节大端长度前缀的响应帧。
// 长度字段 = 前缀(2) + payload 长度，与设备 w1601 模式一致。
func prefixedFrame(payload string) []byte {
	body := []byte(payload)
	frameLen := uint16(len(body) + 2)
	buf := make([]byte, 2, len(body)+2)
	binary.BigEndian.PutUint16(buf, frameLen)
	return append(buf, body...)
}

// HandleCommand 解析 P1604 命令，识别采集启停与单位系数读写。
// adapter Connect 阶段发送 w1601 + u01101，SetUnit 发送 v01101，
// initStream 发送 c 00/c 05 等配置命令（静默接受），
// c 01 1 触发开始发帧，c 02 1 触发停止发帧。
func (r *P1604Responder) HandleCommand(line []byte) (Response, error) {
	cmd := strings.TrimSpace(string(line))
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return Response{}, nil
	}

	// w1601：启用长度前缀模式，回 ACK 帧
	if fields[0] == "w1601" {
		return Response{Data: prefixedFrame("A")}, nil
	}

	// u01101：读取全局 EU 压力转换系数，回 "<coeff> " 帧
	if fields[0] == "u01101" {
		r.mu.Lock()
		coeff := r.coeff
		r.mu.Unlock()
		return Response{Data: prefixedFrame(fmt.Sprintf("%.6f ", coeff))}, nil
	}

	// v01101 <coeff>：写入 EU 压力转换系数，回 ACK 帧
	if fields[0] == "v01101" {
		if len(fields) >= 2 {
			if v, err := strconv.ParseFloat(fields[1], 64); err == nil && v > 0 {
				r.mu.Lock()
				r.coeff = v
				r.mu.Unlock()
			}
		}
		return Response{Data: prefixedFrame("A")}, nil
	}

	if len(fields) < 3 {
		return Response{}, nil
	}
	// 精确匹配 "c 01 1"（开始）/ "c 02 1"（停止），避免与 "c 00 ..." 等配置命令混淆
	if fields[0] == "c" && fields[1] == "01" && fields[2] == "1" {
		return Response{StartStream: true}, nil
	}
	if fields[0] == "c" && fields[1] == "02" && fields[2] == "1" {
		return Response{StopStream: true}, nil
	}
	return Response{}, nil
}

// DSA3217Responder 响应 DSA3217 的 SCPI 命令并维护扫描配置。
// adapter 的 sendCommand 写 cmd+"\r\n" 并读响应到 '\n'，使用行模式。
type DSA3217Responder struct {
	mu     sync.Mutex
	avg    int
	period int
	unit   string
}

// NewDSA3217Responder 构造 DSA3217 响应器，初始配置为合理默认值。
func NewDSA3217Responder() *DSA3217Responder {
	return &DSA3217Responder{avg: 1, period: 1000, unit: "Pa"}
}

// ReadMode 返回行模式。
func (r *DSA3217Responder) ReadMode() ReadMode { return ReadModeLine }

// HandleCommand 解析 DSA3217 SCPI 命令。
// 命令：SCAN（开始）/STOP（停止）/SET AVG n/SET PERIOD n/SET UNITSCAN x/SAVE/LIST S。
func (r *DSA3217Responder) HandleCommand(line []byte) (Response, error) {
	cmd := strings.TrimSpace(strings.TrimRight(string(line), "\r\n"))
	upper := strings.ToUpper(cmd)

	switch {
	case upper == "SCAN":
		return Response{Data: []byte("OK\r\n"), StartStream: true}, nil
	case upper == "STOP":
		return Response{Data: []byte("OK\r\n"), StopStream: true}, nil
	case strings.HasPrefix(upper, "SET AVG"):
		if v, err := parseIntField(cmd, 2); err == nil {
			r.mu.Lock()
			r.avg = v
			r.mu.Unlock()
		}
		return Response{Data: []byte("OK\r\n")}, nil
	case strings.HasPrefix(upper, "SET PERIOD"):
		if v, err := parseIntField(cmd, 2); err == nil {
			r.mu.Lock()
			r.period = v
			r.mu.Unlock()
		}
		return Response{Data: []byte("OK\r\n")}, nil
	case strings.HasPrefix(upper, "SET UNITSCAN"):
		fields := strings.Fields(cmd)
		if len(fields) >= 3 {
			r.mu.Lock()
			r.unit = fields[2]
			r.mu.Unlock()
		}
		return Response{Data: []byte("OK\r\n")}, nil
	case upper == "SAVE":
		return Response{Data: []byte("OK\r\n")}, nil
	case strings.HasPrefix(upper, "LIST S"):
		// 真实设备返回多行配置；adapter sendCommand 读到首个 '\n' 返回单行。
		// 这里返回完整多行，adapter 按其读取逻辑取首行（adapter 已知限制，非模拟器问题）。
		r.mu.Lock()
		avg, period, unit := r.avg, r.period, r.unit
		r.mu.Unlock()
		resp := fmt.Sprintf("SET AVG %d\r\nSET PERIOD %d\r\nSET UNITSCAN %s\r\n", avg, period, unit)
		return Response{Data: []byte(resp)}, nil
	default:
		// 未知命令返回 OK，保持设备友好
		return Response{Data: []byte("OK\r\n")}, nil
	}
}

// parseIntField 从空格分隔的命令中解析第 idx 个字段为整数（idx 从 0 开始，跳过命令名）。
// 例如 "SET AVG 5" 的 parseIntField(cmd, 2) 返回 5。
func parseIntField(cmd string, idx int) (int, error) {
	fields := strings.Fields(cmd)
	if idx >= len(fields) {
		return 0, fmt.Errorf("field %d out of range", idx)
	}
	return strconv.Atoi(fields[idx])
}

// T1603Responder 响应 DAQ-T-1603 的 SCPI 命令并维护硬件配置。
// adapter 的 writeCommandOnly/SendCommand 写裸命令（不带分隔符），使用空闲模式读取。
// 响应格式对齐 shared/device-sdk/go/protocol 的 SendCommand/SendCommandIdle/SendCommandExact：
//   - @f0 <mask> 2：ACK "A\n" + StartStream
//   - @f1：ACK "A\n" + StopStream
//   - @e3：16 字节热电偶类型字符串（exact 16 字节）
//   - @fd XXX：查询响应（idle/exact 视字段而定）
//   - @fe XXX Y / @f3 ...：配置命令，返回 "OK\n"
type T1603Responder struct {
	mu     sync.Mutex
	config t1603Config
}

// t1603Config 保存 T1603 可查询的硬件配置，与 adapter syncHardwareConfig 读取的字段对齐。
type t1603Config struct {
	thermocoupleTypes string
	channelMask       string
	samplingRate      int
	binaryFormat      bool
	showTimestamp     bool
	showSequence      bool
	averageCount      int
	triggerMode       int
	triggerEdge       int
	triggerCount      int
}

// NewT1603Responder 构造 T1603 响应器，初始配置为 K 型热电偶默认值。
// 这些值与 shared driver syncHardwareConfig 期望的查询结果一致，
// 使 adapter 能完成配置同步并进入 64 字节纯二进制帧模式。
func NewT1603Responder() *T1603Responder {
	return &T1603Responder{
		config: t1603Config{
			thermocoupleTypes: "KKKKKKKKKKKKKKKK",
			channelMask:       "FFFF",
			samplingRate:      10,
			binaryFormat:      true,
			averageCount:      1,
		},
	}
}

// ReadMode 返回空闲模式（T1603 命令不带分隔符）。
func (r *T1603Responder) ReadMode() ReadMode { return ReadModeIdle }

// HandleCommand 解析 T1603 命令并返回响应。
// 命令以裸文本发送（无 \n），line 可能含尾部噪声，统一 trim。
func (r *T1603Responder) HandleCommand(line []byte) (Response, error) {
	cmd := strings.TrimSpace(string(line))

	// 采集控制命令
	if cmd == "@f1" {
		// 停止采集：返回 ACK，停止发帧
		return Response{Data: []byte("A\n"), StopStream: true}, nil
	}
	if strings.HasPrefix(cmd, "@f0") {
		// 开始采集：返回 ACK，开始发帧。adapter ConsumeOptionalACK 读 'A'。
		return Response{Data: []byte("A\n"), StartStream: true}, nil
	}

	// 配置查询命令（@fd XXX / @e3）
	r.mu.Lock()
	cfg := r.config
	r.mu.Unlock()

	switch {
	case cmd == "@e3":
		// 16 字节热电偶类型，exact 模式读 16 字节，不带 \n
		return Response{Data: []byte(cfg.thermocoupleTypes)}, nil
	case strings.HasPrefix(cmd, "@fd MCH"):
		// channel mask，idle 模式读，带 \n 便于 trim
		return Response{Data: []byte(cfg.channelMask + "\n")}, nil
	case strings.HasPrefix(cmd, "@fd SPS"):
		return Response{Data: []byte(fmt.Sprintf("%d\n", cfg.samplingRate))}, nil
	case strings.HasPrefix(cmd, "@fd BIN"):
		// exact 1 字节，不带 \n
		return Response{Data: []byte(boolByte(cfg.binaryFormat))}, nil
	case strings.HasPrefix(cmd, "@fd TIME"):
		return Response{Data: []byte(boolByte(cfg.showTimestamp))}, nil
	case strings.HasPrefix(cmd, "@fd HEAD"):
		return Response{Data: []byte(boolByte(cfg.showSequence))}, nil
	case strings.HasPrefix(cmd, "@fd AVG"):
		return Response{Data: []byte(fmt.Sprintf("%d\n", cfg.averageCount))}, nil
	case strings.HasPrefix(cmd, "@fd TYPE"):
		return Response{Data: []byte(fmt.Sprintf("%d", cfg.triggerMode))}, nil
	case strings.HasPrefix(cmd, "@fd TRIG"):
		return Response{Data: []byte(fmt.Sprintf("%d", cfg.triggerEdge))}, nil
	case strings.HasPrefix(cmd, "@fd TNUM"):
		return Response{Data: []byte(fmt.Sprintf("%d\n", cfg.triggerCount))}, nil
	}

	// 配置写入命令（@fe XXX Y / @f3 ...）：静默接受并更新本地状态
	r.applyConfigCommand(cmd)
	if strings.HasPrefix(cmd, "@fe") || strings.HasPrefix(cmd, "@f3") {
		return Response{Data: []byte("OK\n")}, nil
	}

	// 未知命令：返回 OK 避免 adapter 报错
	return Response{Data: []byte("OK\n")}, nil
}

// applyConfigCommand 解析 @fe/@f3 写入命令并更新本地配置，使后续查询返回一致值。
func (r *T1603Responder) applyConfigCommand(cmd string) {
	fields := strings.Fields(cmd)
	if len(fields) < 2 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch fields[0] {
	case "@fe":
		if len(fields) < 3 {
			return
		}
		val := fields[2]
		switch fields[1] {
		case "BIN":
			r.config.binaryFormat = val == "1"
		case "TIME":
			r.config.showTimestamp = val == "1"
		case "HEAD":
			r.config.showSequence = val == "1"
		case "SPS":
			if v, err := strconv.Atoi(val); err == nil {
				r.config.samplingRate = v
			}
		case "AVG":
			if v, err := strconv.Atoi(val); err == nil {
				r.config.averageCount = v
			}
		case "TYPE":
			if v, err := strconv.Atoi(val); err == nil {
				r.config.triggerMode = v
			}
		case "TRIG":
			if v, err := strconv.Atoi(val); err == nil {
				r.config.triggerEdge = v
			}
		case "TNUM":
			if v, err := strconv.Atoi(val); err == nil {
				r.config.triggerCount = v
			}
		}
	case "@f3":
		// @f3 0<Types>0 写热电偶类型（16 字符）
		if len(fields) >= 2 {
			raw := fields[1]
			// 去掉首尾的 '0' 标记位
			if len(raw) >= 16 {
				r.config.thermocoupleTypes = raw[len(raw)-17 : len(raw)-1]
				if len(r.config.thermocoupleTypes) != 16 {
					r.config.thermocoupleTypes = raw[:16]
				}
			}
		}
	}
}

// boolByte 返回 "1" 或 "0"。
func boolByte(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
