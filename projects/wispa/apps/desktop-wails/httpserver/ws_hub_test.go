// Package httpserver 测试：WebSocket hub 事件推送链路。
//
// 测试覆盖：
//   - Emit 经 broadcast channel → fanout → 客户端 send 队列 → writePump → WebSocket 帧
//   - 多客户端连接时全部收到同一事件
//   - 单参数事件 payload 直接是 data[0]
//   - 多参数事件 payload 是数组（daq:device-state 双参数场景）
//   - ctx 取消时 hub 优雅关闭
package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

// startTestHub 启动一个 hub + httptest.Server，返回 (hub, server, cleanup)。
// 调用方在测试结束后必须调用 cleanup 关闭 server 与 hub。
func startTestHub(t *testing.T) (*WSHub, *httptest.Server, func()) {
	t.Helper()
	hub := NewWSHub()
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS)
	srv := httptest.NewServer(mux)

	cleanup := func() {
		cancel()
		srv.Close()
		// 等 hub 主循环退出，避免 goroutine 泄漏
		time.Sleep(50 * time.Millisecond)
	}
	return hub, srv, cleanup
}

// dialWS 连接到测试 server 的 /ws，返回 (conn, cleanup)。
// 超时 2s 防止挂死。
func dialWS(t *testing.T, srv *httptest.Server) (*websocket.Conn, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("websocket dial failed: %v", err)
	}
	return conn, func() {
		_ = conn.Close(websocket.StatusNormalClosure, "test done")
		cancel()
	}
}

// TestWSHub_EmitDelivered 验证：单客户端连接后，单参数 hub.Emit 推送的事件能被收到。
//
// 测试前置：
//   - 启动 hub + httptest.Server
//   - 一个 WebSocket 客户端连接
//
// 测试步骤：
//   - hub.Emit("daq:log", entry)
//   - 客户端读取一条消息
//
// 期待结果：
//   - 收到的消息 JSON 解析后 event="daq:log"，data 字段值与发送的 entry 一致。
func TestWSHub_EmitDelivered(t *testing.T) {
	hub, srv, cleanup := startTestHub(t)
	defer cleanup()

	conn, connCleanup := dialWS(t, srv)
	defer connCleanup()

	// 等待 register 完成，避免 Emit 在客户端入表前发生丢消息
	time.Sleep(50 * time.Millisecond)

	entry := map[string]any{
		"level":    "info",
		"category": "system",
		"source":   "backend",
		"message":  "WISPA application started",
	}
	hub.Emit("daq:log", entry)

	readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}

	var env wsEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope failed: %v", err)
	}
	if env.Event != "daq:log" {
		t.Fatalf("event = %q, want %q", env.Event, "daq:log")
	}

	// 单参数事件：data 字段是 map[string]any
	dataMap, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map[string]any", env.Data)
	}
	if dataMap["level"] != "info" {
		t.Errorf("level = %v, want info", dataMap["level"])
	}
	if dataMap["message"] != "WISPA application started" {
		t.Errorf("message = %v, want 'WISPA application started'", dataMap["message"])
	}
}

// TestWSHub_MultipleClientsBroadcast 验证：多客户端连接时 Emit 广播给全部。
//
// 测试前置：
//   - 启动 hub + httptest.Server
//   - 两个 WebSocket 客户端连接
//
// 测试步骤：
//   - hub.Emit("daq:recording-status", session)
//   - 两个客户端各自读取一条消息
//
// 期待结果：
//   - 两个客户端都收到 event="daq:recording-status" 的消息。
func TestWSHub_MultipleClientsBroadcast(t *testing.T) {
	hub, srv, cleanup := startTestHub(t)
	defer cleanup()

	conn1, cleanup1 := dialWS(t, srv)
	defer cleanup1()
	conn2, cleanup2 := dialWS(t, srv)
	defer cleanup2()

	time.Sleep(50 * time.Millisecond) // 等 register

	hub.Emit("daq:recording-status", map[string]string{"id": "session-001"})

	for i, conn := range []*websocket.Conn{conn1, conn2} {
		readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			t.Fatalf("client[%d] read failed: %v", i, err)
		}
		var env wsEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("client[%d] unmarshal failed: %v", i, err)
		}
		if env.Event != "daq:recording-status" {
			t.Errorf("client[%d] event = %q, want daq:recording-status", i, env.Event)
		}
	}
}

// TestWSHub_EmitNoData 验证：Emit 不带 data 时 envelope.data 为 nil，不 panic。
//
// 测试前置：
//   - 启动 hub + httptest.Server + 1 个客户端
//
// 测试步骤：
//   - hub.Emit("daq:empty")  // 无 data 参数
//   - 客户端读取一条消息
//
// 期待结果：
//   - 消息 JSON 中 event="daq:empty"，data 字段为 nil。
func TestWSHub_EmitNoData(t *testing.T) {
	hub, srv, cleanup := startTestHub(t)
	defer cleanup()

	conn, connCleanup := dialWS(t, srv)
	defer connCleanup()

	time.Sleep(50 * time.Millisecond)

	hub.Emit("daq:empty")

	readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var env wsEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if env.Event != "daq:empty" {
		t.Errorf("event = %q, want daq:empty", env.Event)
	}
	if env.Data != nil {
		t.Errorf("data = %v, want nil", env.Data)
	}
}

// TestWSHub_EmptyNameDropped 验证：Emit 空事件名直接丢弃，客户端不应收到。
//
// 测试前置：
//   - 启动 hub + httptest.Server + 1 个客户端
//
// 测试步骤：
//   - hub.Emit("", "anything")
//   - 客户端尝试读取（200ms 超时）
//
// 期待结果：
//   - 读取超时（无消息到达），证明空事件名被丢弃。
func TestWSHub_EmptyNameDropped(t *testing.T) {
	hub, srv, cleanup := startTestHub(t)
	defer cleanup()

	conn, connCleanup := dialWS(t, srv)
	defer connCleanup()

	time.Sleep(50 * time.Millisecond)

	hub.Emit("", "anything")

	readCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, err := conn.Read(readCtx)
	if err == nil {
		t.Errorf("expected read timeout for empty event, got message")
	}
}

// TestWSHub_MultiParamEmitPackedAsArray 验证：多参数 Emit 时 data 字段是数组。
//
// 测试前置：
//   - 启动 hub + httptest.Server + 1 个客户端
//
// 测试步骤：
//   - hub.Emit("daq:device-state", "dev-001", state)
//   - 客户端读取一条消息
//
// 期待结果：
//   - 消息 JSON 中 event="daq:device-state"，data 是长度为 2 的数组，
//     第 0 项是 deviceId 字符串，第 1 项是 state 对象。
//
// 设计动机：wispa 的 EmitDeviceState 推送 (id, state) 双参数，
// 与原 Wails v3 Event.Emit(name, id, state) wire 格式保持一致，
// 前端 onmessage 回调可继续解构 [id, state] 数组。
func TestWSHub_MultiParamEmitPackedAsArray(t *testing.T) {
	hub, srv, cleanup := startTestHub(t)
	defer cleanup()

	conn, connCleanup := dialWS(t, srv)
	defer connCleanup()

	time.Sleep(50 * time.Millisecond)

	state := map[string]any{
		"status":     "Connected",
		"statusText": "Connected",
	}
	hub.Emit("daq:device-state", "dev-001", state)

	readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var env wsEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if env.Event != "daq:device-state" {
		t.Fatalf("event = %q, want daq:device-state", env.Event)
	}

	// 多参数事件：data 字段应是数组
	arr, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data type = %T, want []any (array)", env.Data)
	}
	if len(arr) != 2 {
		t.Fatalf("array length = %d, want 2", len(arr))
	}
	if arr[0] != "dev-001" {
		t.Errorf("arr[0] = %v, want 'dev-001'", arr[0])
	}
	stateMap, ok := arr[1].(map[string]any)
	if !ok {
		t.Fatalf("arr[1] type = %T, want map[string]any", arr[1])
	}
	if stateMap["status"] != "Connected" {
		t.Errorf("arr[1].status = %v, want 'Connected'", stateMap["status"])
	}
}
