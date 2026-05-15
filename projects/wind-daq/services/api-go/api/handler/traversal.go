package handler

import (
	"net/http"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

// ==================== 曲面扫描 HTTP Handler ====================
// 处理曲面扫描(遍历)相关的REST API请求

// TraversalHandler 曲面扫描请求处理器
type TraversalHandler struct {
	service *usecase.TraversalService // 曲面扫描服务
}

// NewTraversalHandler 构建曲面扫描处理器
// 参数: service 曲面扫描服务
// 返回: *TraversalHandler 处理器实例
func NewTraversalHandler(service *usecase.TraversalService) *TraversalHandler {
	return &TraversalHandler{service: service}
}

// Start 开始曲面扫描
// 请求体: TraversalConfig JSON
func (h *TraversalHandler) Start(c *gin.Context) {
	var config traversal.TraversalConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.Start(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Pause 暂停扫描
func (h *TraversalHandler) Pause(c *gin.Context) {
	if err := h.service.Pause(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Resume 恢复扫描
func (h *TraversalHandler) Resume(c *gin.Context) {
	if err := h.service.Resume(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Stop 停止扫描
func (h *TraversalHandler) Stop(c *gin.Context) {
	h.service.Stop()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetProgress 获取扫描进度
func (h *TraversalHandler) GetProgress(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.GetProgress())
}
