package ws

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// ==================== WebSocket Hub ====================
// WebSocket 连接管理 + 消息广播中心
// 负责管理所有前端WebSocket连接,并向订阅者广播消息

// Hub WebSocket Hub
// 管理前端连接,提供频道订阅和广播功能
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{} // 客户端连接集合
}

// NewHub 创建WebSocket Hub
// 返回: *Hub Hub实例
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
	}
}

// Register 注册客户端连接
// 参数: c 客户端连接
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	slog.Info("WS client connected", "total", h.Count())
}

// Unregister 注销(移除)客户端连接
// 参数: c 客户端连接
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	c.Close()
	slog.Info("WS client disconnected", "total", h.Count())
}

// Broadcast 向指定频道的所有订阅者广播消息
// 参数: channel 频道名称, data 要广播的数据
func (h *Hub) Broadcast(channel string, data interface{}) {
	// 构建消息
	msg := Message{Channel: channel, Data: data}
	payload, err := json.Marshal(msg)
	if err != nil {
		slog.Error("WS broadcast marshal error", "err", err)
		return
	}
	// 遍历所有客户端,发送给订阅了该频道的
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.IsSubscribed(channel) {
			c.Send(payload)
		}
	}
}

// Count 获取当前连接数
// 返回: int 连接客户端数量
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Send implements logger.LogSink — pushes log entries to frontend via WebSocket.
func (h *Hub) Send(entry any) {
	h.Broadcast(ChannelLog, entry)
}
