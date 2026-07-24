// Package httpserver WebSocket hub：实现 core.EventBus 接口，向前端推送事件流。
//
// 设计要点：
//   - 单 goroutine 串行处理 register/unregister/broadcast，避免并发写 map 的锁竞争；
//   - 每个客户端独立 send channel（buffered 32）+ writePump goroutine，慢客户端
//     不会阻塞 broadcast 主循环；
//   - send 队列满时丢消息（保护其他订阅者），由前端按事件频率容忍；
//   - Emit 非阻塞：broadcast channel 缓冲 128，超出即丢，避免 relay 协程
//     1s 推送录制状态被阻塞；
//   - 监听 127.0.0.1，外部网络无法访问，InsecureSkipVerify 跳过 Origin 校验
//     以兼容 Electron file:// 加载场景。
//
// 与 daq-t1603 WSHub 的差异：
//   - Emit 支持多参数事件：data 长度 > 1 时打包为数组推送，
//     兼容原 Wails v3 Event.Emit(name, id, state) 双参数 wire 格式，
//     前端 daq:device-state 事件回调可继续解构 [id, state] 数组。
package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

// wsEnvelope 是 WebSocket 出站消息的统一信封。
// 前端 onmessage 解析此结构后按 event 字段路由到各 callback。
// 与 core.EventXxx 常量一一对应。
//
// Data 字段类型：
//   - 单参数事件（daq:log / daq:recording-status / daq:recording-warning）：直接是 payload
//   - 多参数事件（daq:device-state）：是 [id, state] 数组
//   - 无参数事件：nil（omitempty 省略）
type wsEnvelope struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// wsClient 表示一个 WebSocket 客户端连接。
//   - send 由 hub 的 broadcast 主循环写入，由 writePump goroutine 消费；
//   - readPump 仅用于检测连接断开，前端不发送业务数据。
type wsClient struct {
	hub  *WSHub
	conn *websocket.Conn
	send chan []byte
}

// WSHub 是 WebSocket 事件总线，实现 core.EventBus 接口。
//
// 字段并发模型：
//   - clients map 由 Run goroutine 独占，无需加锁；
//   - register/unregister/broadcast 三个 channel 是 Run 的输入，外部 goroutine
//     通过它们与 Run 通信。
type WSHub struct {
	clients map[*wsClient]struct{}

	register   chan *wsClient
	unregister chan *wsClient
	broadcast  chan wsEnvelope
}

// NewWSHub 构造一个新的 hub 实例。
// channel 缓冲大小：
//   - register/unregister：8，启动期最多 1 个客户端，缓冲足够；
//   - broadcast：128，吸收 relay 协程 1s 推送 + 多设备并发警告的瞬时峰值。
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*wsClient]struct{}),
		register:   make(chan *wsClient, 8),
		unregister: make(chan *wsClient, 8),
		broadcast:  make(chan wsEnvelope, 128),
	}
}

// Run 启动 hub 主循环，应在单独 goroutine 中调用。
// ctx 取消时关闭所有客户端连接并退出。
func (h *WSHub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.shutdown()
			return
		case c := <-h.register:
			h.clients[c] = struct{}{}
		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
		case msg := <-h.broadcast:
			h.fanout(msg)
		}
	}
}

// fanout 把一条消息序列化后投递给所有客户端的 send 队列。
// 单条 send 队列满时跳过该客户端，避免慢订阅者拖累整体广播。
func (h *WSHub) fanout(msg wsEnvelope) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	for c := range h.clients {
		select {
		case c.send <- payload:
		default:
			// 该客户端 send 队列已满，丢弃本条消息
		}
	}
}

// shutdown 关闭所有客户端连接，由 Run 在 ctx 取消时调用。
// close(send) 让 writePump 退出，conn.Close 让 readPump 的 Read 返回错误。
func (h *WSHub) shutdown() {
	for c := range h.clients {
		close(c.send)
		_ = c.conn.Close(websocket.StatusNormalClosure, "server shutting down")
		delete(h.clients, c)
	}
}

// Emit 实现 core.EventBus 接口。
//
// Wire 格式约定：
//   - name 为空直接返回，避免误发空事件；
//   - data 长度 == 0：payload 为 nil（omitempty 省略）；
//   - data 长度 == 1：payload 为 data[0]（单参数事件，前端直接读 data 字段）；
//   - data 长度 > 1：payload 为 data 数组（多参数事件，前端解构数组）。
//     典型场景：daq:device-state 传 (id, state)，前端 onmessage 收到 [id, state]。
//
// 非阻塞：broadcast channel 缓冲满时丢弃，保护 relay 协程不被前端慢消费拖累。
func (h *WSHub) Emit(name string, data ...any) {
	if name == "" {
		return
	}
	var payload any
	switch len(data) {
	case 0:
		// payload 保持 nil
	case 1:
		payload = data[0]
	default:
		// 多参数事件打包为数组，与 Wails v3 Event.Emit(name, a, b) wire 格式一致
		payload = data
	}
	select {
	case h.broadcast <- wsEnvelope{Event: name, Data: payload}:
	default:
		// 缓冲满，丢弃；前端按事件频率容忍偶发丢帧
	}
}

// ServeWS 处理 /ws 的 WebSocket 升级请求。
//
// Origin 策略说明：
//   - 监听 127.0.0.1，外部网络无法访问，无 CSRF 风险；
//   - InsecureSkipVerify=true 允许任意 Origin，因为 Electron renderer 加载本地
//     file:// 时无标准 Origin，强制校验会拒绝合法前端。
func (h *WSHub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	// 前端仅订阅，不发送业务数据，限制接收消息大小作为兜底防御
	conn.SetReadLimit(1024)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	c := &wsClient{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 32),
	}
	h.register <- c

	go c.writePump(ctx)
	c.readPump(ctx) // 阻塞直到连接关闭
}

// readPump 持续读取客户端消息，仅用于检测连接断开。
// 前端不发送业务数据，所有收到的消息一律丢弃。
//
// 退出时通过 unregister channel 通知 hub 清理，并关闭底层 conn。
// 使用 select+default 防止 Run 已退出时 unregister 阻塞。
func (c *wsClient) readPump(ctx context.Context) {
	defer func() {
		select {
		case c.hub.unregister <- c:
		default:
		}
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		if _, _, err := c.conn.Read(ctx); err != nil {
			return
		}
	}
}

// writePump 从 send 队列读取消息并写入 WebSocket。
//   - send 队列被 hub 关闭时退出（连接清理）；
//   - ctx 取消时退出（应用关闭）；
//   - 写入失败时退出（对端关闭或网络异常），由 readPump 兜底清理。
func (c *wsClient) writePump(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
