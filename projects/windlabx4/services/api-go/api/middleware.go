package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// statusRecorder 包裹 http.ResponseWriter，记录最终写入的状态码与字节数，
// 供 metricsMiddleware 统计 request 结果。如未显式调用 WriteHeader，
// 默认 200（与 net/http 行为一致）。
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Flush 支持 SSE（/api/daq/stream/*）等需要 http.Flusher 的 handler，
// 否则 statusRecorder 会遮蔽底层 ResponseWriter 的 Flusher 接口。
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// metricsMiddleware 记录每条 HTTP 请求的方法、路径、状态码、耗时与响应字节数。
// 输出到 slog，调用方可通过 slog handler 决定落盘 / 控制台。
//
// 级别策略（避免 LOG 画面被高频请求刷屏）：
//   - 常规请求 → Debug：HTTP 请求是常态事件，对用户无业务价值，
//     默认 LOG 画面 minLevel=warn 即可隐藏；调试网络问题时手动打开 Debug 开关。
//   - 慢请求（>500ms）→ Warn：仍属于异常情况，需要醒目提示。
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}

		// 内部观测类路径（前端心跳、日志服务自身端点、高频遥测拉取）完全不打日志；
		// 其他业务请求打 Debug 级别，便于按需排查但不污染默认日志视图。
		if !isInternalObservationPath(r.URL.Path) {
			slog.Debug("http.request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", duration.Milliseconds(),
				"bytes", rec.bytes,
			)
		}

		const slowThreshold = 500 * time.Millisecond
		// 慢请求仍要告警，但日志服务自身的拉取/流式端点不纳入慢请求统计。
		if duration > slowThreshold && !isLogServicePath(r.URL.Path) {
			slog.Warn("http.slow_request",
				"method", r.Method,
				"path", r.URL.Path,
				"duration_ms", duration.Milliseconds(),
				"threshold_ms", slowThreshold.Milliseconds(),
			)
		}
	})
}

// isInternalObservationPath 判断是否为系统内部观测/自查询路径。
// 这类请求不携带业务语义，记录到用户日志只会形成噪音，应过滤 http.request 输出。
func isInternalObservationPath(path string) bool {
	return strings.HasPrefix(path, "/api/daq/latest/") ||
		path == "/api/health" ||
		isLogServicePath(path)
}

// isLogServicePath 判断是否为日志服务自身端点（拉取、分类、SSE 流）。
// 这些端点由 LOG 画面自身产生，对用户不是业务事件。
func isLogServicePath(path string) bool {
	return strings.HasPrefix(path, "/api/log/")
}

// recoverMiddleware 拦截 handler 中的 panic，记录堆栈并返回 500。
// 没有这层时，net/http 默认会关闭连接但日志只是 "http: panic serving ..."，
// 信息不够。这里把 stack trace 与 path 一起结构化输出。
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := string(debug.Stack())
				slog.Error("http.panic",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", rec,
					"stack", stack,
				)
				// 注意：若 handler 已经 WriteHeader 过，下面这次会触发
				// "superfluous response.WriteHeader" warning，属预期 — 至少不会泄漏 panic 内容。
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
