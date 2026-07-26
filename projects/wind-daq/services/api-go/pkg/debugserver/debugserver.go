// Package debugserver 提供按需开启的 pprof / debug 端点。
//
// 设计原则：
//  1. 独立于主 API 端口，避免误暴露到生产；
//  2. 默认只监听 127.0.0.1，外部不可访问；
//  3. 由环境变量 WINDDAQ_PPROF_ADDR 控制是否启用与监听地址（如 "localhost:6060"）；
//  4. 仅当显式启用时 import 路径副作用注册 pprof handler，避免无谓资源占用。
//
// 用法（开发模式）：
//
//	# PowerShell
//	$env:WINDDAQ_PPROF_ADDR = "localhost:6060"
//	go run ./services/api-go/cmd/server
//
// 然后浏览器或 go tool pprof：
//
//	http://localhost:6060/debug/pprof/
//	go tool pprof http://localhost:6060/debug/pprof/heap
//	go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
package debugserver

import (
	"context"
	"shared.local/device-sdk/go/pkg/slog"
	"net/http"
	_ "net/http/pprof" // 注册 /debug/pprof/* handler 到 http.DefaultServeMux
	"os"
	"strings"
	"time"
)

// EnvAddr 环境变量名：未设置则不启动 debug server。
const EnvAddr = "WINDDAQ_PPROF_ADDR"

// Start 按环境变量启动 debug server。返回 *http.Server 便于上层 Shutdown。
// 若未配置或地址不合法则返回 (nil, nil)，调用方应判空。
//
// 出于安全考虑，地址必须以 "localhost:" 或 "127.0.0.1:" 开头，
// 否则拒绝启动（防止误配置 ":6060" 暴露到所有网卡）。
func Start(ctx context.Context) (*http.Server, error) {
	addr := strings.TrimSpace(os.Getenv(EnvAddr))
	if addr == "" {
		return nil, nil
	}
	if !strings.HasPrefix(addr, "localhost:") && !strings.HasPrefix(addr, "127.0.0.1:") {
		slog.Warn("debugserver: refusing non-localhost address; set WINDDAQ_PPROF_ADDR to localhost:<port>",
			"addr", addr)
		return nil, nil
	}

	// 使用 http.DefaultServeMux —— pprof 通过 init() 把 /debug/pprof/* handler 注册到该 mux。
	// 这里给 server 加合理超时，避免长连接的 profile 抓取阻塞 shutdown。
	srv := &http.Server{
		Addr:              addr,
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 5 * time.Second,
		// pprof profile 抓取本身可能跑 30s+，所以 read/write timeout 留宽
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		slog.Info("debugserver: listening", "addr", addr, "endpoints", "/debug/pprof/")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("debugserver: listen error", "err", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	return srv, nil
}
