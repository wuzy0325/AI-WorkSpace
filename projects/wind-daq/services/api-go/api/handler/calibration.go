package handler

import (
	"net/http"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

// ==================== 校准 HTTP Handler ====================
// 处理探针校准相关的REST API请求

// CalibrationHandler 校准请求处理器
type CalibrationHandler struct {
	service *usecase.CalibrationService // 校准服务
}

// NewCalibrationHandler 构建校准处理器
// 参数: service 校准服务
// 返回: *CalibrationHandler 处理器实例
func NewCalibrationHandler(service *usecase.CalibrationService) *CalibrationHandler {
	return &CalibrationHandler{service: service}
}

// Start 开始自动校准
// 请求体: CalibrationConfig JSON
// 返回: taskId 任务ID
func (h *CalibrationHandler) Start(c *gin.Context) {
	var config calibration.CalibrationConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	taskID, err := h.service.Start(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "taskId": taskID})
}

// Pause 暂停校准
func (h *CalibrationHandler) Pause(c *gin.Context) {
	if err := h.service.Pause(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Resume 恢复校准
func (h *CalibrationHandler) Resume(c *gin.Context) {
	if err := h.service.Resume(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Stop 停止校准
func (h *CalibrationHandler) Stop(c *gin.Context) {
	h.service.Stop()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetStatus 获取校准状态
func (h *CalibrationHandler) GetStatus(c *gin.Context) {
	status := h.service.GetStatus()
	c.JSON(http.StatusOK, status)
}

// SaveConfig 保存校准配置
// 请求体: CalibrationConfig JSON
func (h *CalibrationHandler) SaveConfig(c *gin.Context) {
	var config calibration.CalibrationConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.SaveConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetConfig 获取校准配置
func (h *CalibrationHandler) GetConfig(c *gin.Context) {
	config := h.service.GetConfig()
	c.JSON(http.StatusOK, config)
}
