package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	sharedproto "shared.local/device-sdk/go/protocol"
)

// readLoop 持续从 conn 读取扫描数据行并下发给 sink。
//
// ADR-009 关键设计：
//   - 每次 ReadString 前启 watchdog（在 ioMu.Lock 之前），覆盖"等 ioMu + ReadString"
//     全区间。若 deadline 失效导致 ReadString 无限阻塞，watchdog 超时后强制 Close
//     conn 解除阻塞并释放 ioMu，避免 sendCommand 永久拿不到 ioMu 死锁。
//   - 退出时 close(readLoopDone)，供 StopAcquisition/Disconnect join。
//   - 主动停止（close(stop)）后 conn 被 Close 触发的 Read 错误静默退出，
//     不调用 onError，避免误报异常。
//   - I-2 修复：sendCommand watchdog 触发后调 invalidateConnection（含 onError），
//     readLoop 后续 ReadString 会因 conn 已 Close 返回错误。此时若 readLoop 也调
//     onError 会导致同一错误被通知两次（DeviceManager 误判为两次故障）。修复方式：
//     readLoop 检测到 IsClosedConnError 时静默退出，onError 由 invalidate 路径统一负责。
//
// R1-3 整改（2026-07-30）：terminal read error（watchdog 触发、EOF、RST、协议错误等）
// 统一走 invalidateConnection 毒化连接：清 conn/reader/stop + Error 状态 + Close conn +
// 调 onError。原实现仅把 Connection 从 Acquiring 恢复为 Connected 且不清 conn/reader，
// 导致已死连接被后续 sendCommand 复用爆 WSAECONNABORTED，且 frameReader 残留可被错误复用。
func (d *DSA3217) readLoop(stop <-chan struct{}) {
	// readLoop 退出时关闭 readLoopDone，供 StopAcquisition/Disconnect 等待协程退出。
	// 用临时局部变量避免与 StartAcquisition 创建新 readLoopDone 时 race。
	defer func() {
		d.mu.Lock()
		done := d.readLoopDone
		d.mu.Unlock()
		if done != nil {
			close(done)
		}
	}()

	// ADR-009 finding 2：捕获 readLoop 启动时的 expectedConn，
	// defer 中传给 invalidateConnection，避免与并发 Disconnect -> Connect 的新连接误杀。
	d.mu.RLock()
	expectedConn := d.conn
	d.mu.RUnlock()

	if d.reader == nil {
		return
	}

	var unexpectedErr error

	defer func() {
		// unexpectedErr == nil 表示主动停止或 conn 被外部路径 Close（sendCommand
		// invalidate / Disconnect close）触发的预期退出，无需 invalidate。
		// unexpectedErr != nil 表示 watchdog 触发、EOF、RST、协议错误等 terminal
		// read error，连接已不可用——走 invalidateConnection 统一毒化（R1-3）：
		//   - 清 d.conn / d.reader / d.stop / d.acquiring / d.scanning
		//   - 置 status.Connection = Error，status.LastError = unexpectedErr.Error()
		//   - 锁外 Close conn，避免与 readLoop 的 Read 竞争
		//   - 调 onError 通知上层 DeviceManager 触发重连
		// 不再恢复为 Connected：保留失效 conn 会让后续 sendCommand 复用爆错。
		if unexpectedErr == nil {
			return
		}
		slog.Warn("DSA3217 read loop exited unexpectedly", "device", d.profile.ID, "error", unexpectedErr)
		d.invalidateConnection(expectedConn, unexpectedErr.Error())
	}()

	for {
		select {
		case <-stop:
			return
		default:
		}

		d.mu.RLock()
		conn := d.conn
		reader := d.reader
		d.mu.RUnlock()
		if reader == nil || conn == nil {
			return
		}

		// watchdog 兜底（ADR-009）：覆盖 ioMu.Lock + ReadString 整个区间。
		// 若 readLoop 持 ioMu 阻塞在 ReadString 上（200ms deadline 失效），
		// watchdog 超时后强制 Close conn，解除 ReadString 阻塞并释放 ioMu，
		// 避免 sendCommand 永久拿不到 ioMu 导致死锁。
		//
		// 必须在 ioMu.Lock 之前启动：若 readLoop 卡在 ioMu.Lock 等待 sendCommand 释放，
		// watchdog 触发 Close 后 sendCommand 的 Write/Read 会失败并释放 ioMu，
		// readLoop 才能拿到 ioMu 并发现 conn 已失效。
		wdStop := sharedproto.WatchdogClose(conn, d.effectiveReadLoopWatchdog())

		d.ioMu.Lock()
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		line, err := reader.ReadString('\n')
		d.ioMu.Unlock()

		// wdStop 返回 false 表示 watchdog 已触发（conn 已被 Close），readLoop 必须退出。
		// 必须在所有路径调用 wdStop，否则计时器泄漏。
		watchdogTriggered := !wdStop()

		// 主动停止后 conn 被 Close 触发的 Read 错误属于预期，静默退出。
		// 此分支覆盖 StopAcquisition/Disconnect close(stop) 后由 invalidateConnection
		// 强制 Close conn 触发的 ReadString 错误，避免误调 onError。
		select {
		case <-stop:
			return
		default:
		}

		if watchdogTriggered {
			unexpectedErr = fmt.Errorf("read loop watchdog triggered: ReadString blocked and conn closed by watchdog")
			slog.Warn("DSA3217 read loop watchdog triggered", "device", d.profile.ID)
			return
		}

		if err != nil {
			// 读取超时不视为异常，继续等待
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// I-2 修复：conn 被外部路径 Close（sendCommand watchdog / invalidate /
			// Disconnect）触发的 Read 错误属预期，静默退出。onError 已由 invalidate 路径
			// 统一负责，此处再调 onError 会导致同一故障被 DeviceManager 误判为两次。
			// 仅当 readLoop 自身 watchdog 触发或非 closed 类错误时才设 unexpectedErr。
			if sharedproto.IsClosedConnError(err) {
				slog.Debug("DSA3217 read loop exiting silently (conn closed by external path)",
					"device", d.profile.ID, "error", err)
				return
			}
			slog.Debug("DSA3217 read error", "device", d.profile.ID, "error", err)
			unexpectedErr = err
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		d.parseDataLine(line)
	}
}

// parseDataLine 解析扫描数据行并下发到 sink。
// 行格式：空格分隔的浮点数列表，按 channel.Index 取值。
func (d *DSA3217) parseDataLine(line string) {
	d.mu.RLock()
	scanning := d.scanning
	sink := d.sink
	channels := d.profile.Channels
	d.mu.RUnlock()

	if !scanning || sink == nil {
		return
	}

	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		if v, err := strconv.ParseFloat(part, 64); err == nil {
			values = append(values, v)
		}
	}

	if len(values) == 0 {
		return
	}

	indices := make([]int, 0, len(channels))
	channelValues := make([]float64, 0, len(channels))

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if ch.Index >= 0 && ch.Index < len(values) {
			indices = append(indices, ch.Index)
			channelValues = append(channelValues, values[ch.Index])
		}
	}

	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		DeviceType:     d.profile.Type,
		DeviceName:     d.profile.Name,
		Timestamp:      device.NowMs(),
		Channels:       channelValues,
		ChannelIndices: indices,
	})
}
