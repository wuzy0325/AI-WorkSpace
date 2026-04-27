package handler

import (
	"net/http"

	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

// ==================== DAQ采集 HTTP Handler ====================
// 处理数据采集相关的REST API请求

// DAQHandler DAQ采集请求处理器
type DAQHandler struct {
	hub           *usecase.AcquisitionHub // 采集Hub
	deviceManager *usecase.DeviceManager  // 设备管理器
}

// NewDAQHandler 构建DAQ处理器
// 参数: hub 采集Hub, deviceManager 设备管理器
// 返回: *DAQHandler 处理器实例
func NewDAQHandler(hub *usecase.AcquisitionHub, deviceManager *usecase.DeviceManager) *DAQHandler {
	return &DAQHandler{hub: hub, deviceManager: deviceManager}
}

// StartAcquisition 开始所有设备采集
// 启动所有已连接设备的采集
func (h *DAQHandler) StartAcquisition(c *gin.Context) {
	started := h.deviceManager.StartAcquisitionAll()
	if started == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// StopAcquisition 停止所有设备采集
func (h *DAQHandler) StopAcquisition(c *gin.Context) {
	h.deviceManager.StopAcquisitionAll()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// SetPublishRate 设置推送频率
// 请求体: { "hz": 20.0 }
// 有效范围: 1-100 Hz
func (h *DAQHandler) SetPublishRate(c *gin.Context) {
	var body struct {
		Hz float64 `json:"hz"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.hub.UpdatePublishRate(body.Hz); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetPublishRate 获取当前推送频率
func (h *DAQHandler) GetPublishRate(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"hz": h.hub.GetPublishRate()})
}
