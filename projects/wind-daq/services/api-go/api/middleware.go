package api

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== HTTP 中间件 ====================

// Recovery panic恢复中间件
// 捕获panic并返回500错误,防止服务器崩溃
func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}

// Logger 请求日志中间件
// 记录每个HTTP请求的方法、路径、状态、延迟、客户端IP
func Logger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		slog.Info("HTTP request",
			"method", param.Method,
			"path", param.Path,
			"status", param.StatusCode,
			"latency", param.Latency.Round(time.Millisecond),
			"ip", param.ClientIP,
		)
		return ""
	})
}

// CORS 跨域资源共享中间件
// 允许所有来源的跨域请求
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		// 处理预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
