package ws

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
)

// ==================== WebSocket 客户端连接 ====================
// 单个前端浏览器的WebSocket连接管理

// Client WebSocket客户端连接
// 封装单个前端连接,支持订阅频道和发送消息
type Client struct {
	conn       *websocket.Conn // WebSocket连接
	mu         sync.Mutex      // 发送互斥锁
	subscribed map[string]bool // 已订阅的频道集合
}

// NewClient 创建客户端连接
// 参数: conn WebSocket连接
// 返回: *Client 客户端实例
func NewClient(conn *websocket.Conn) *Client {
	return &Client{
		conn:       conn,
		subscribed: make(map[string]bool),
	}
}

// IsSubscribed 检查是否订阅了指定频道
// 参数: channel 频道名称
// 返回: bool 是否已订阅
func (c *Client) IsSubscribed(channel string) bool {
	return c.subscribed[channel]
}

// Subscribe 订阅频道
// 参数: channels 要订阅的频道列表
func (c *Client) Subscribe(channels []string) {
	for _, ch := range channels {
		c.subscribed[ch] = true
	}
	slog.Debug("WS client subscribed", "channels", channels)
}

// Send 发送消息(线程安全)
// 参数: payload 要发送的JSON数据
func (c *Client) Send(payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		slog.Error("WS send error", "err", err)
	}
}

// Close 关闭连接
func (c *Client) Close() {
	c.conn.Close()
}

// ReadMessage 读取消息(用于处理前端消息)
// 返回: (消息类型, 消息内容, 错误)
func (c *Client) ReadMessage() (int, []byte, error) {
	return c.conn.ReadMessage()
}

// HandleMessages 处理客户端发来的消息(订阅请求等)
// 后台循环处理前端的订阅请求
func (c *Client) HandleMessages() {
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			break
		}
		var req SubscribeRequest
		if err := json.Unmarshal(message, &req); err != nil {
			slog.Warn("WS invalid message", "err", err)
			continue
		}
		// 处理订阅请求
		if req.Type == "subscribe" && len(req.Channels) > 0 {
			c.Subscribe(req.Channels)
		}
	}
}
