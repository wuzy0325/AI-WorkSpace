package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ==================== 应用 HTTP Handler ====================
// 处理应用级REST API请求(如版本查询)

// AppHandler 应用请求处理器
type AppHandler struct{}

// NewAppHandler 构建应用处理器
// 返回: *AppHandler 处理器实例
func NewAppHandler() *AppHandler {
	return &AppHandler{}
}

// GetVersion 获取应用版本
func (h *AppHandler) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": "0.1.0-go"})
}
