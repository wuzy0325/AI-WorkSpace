package handler

import (
	"net/http"

	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

// ==================== 数据存储 HTTP Handler ====================
// 处理数据存储相关的REST API请求

// StorageHandler 存储请求处理器
type StorageHandler struct {
	store   *config.StorageStore    // 存储配置
	service *usecase.StorageService // 存储服务
}

// NewStorageHandler 构建存储处理器
// 参数: store 存储配置, service 存储服务
// 返回: *StorageHandler 处理器实例
func NewStorageHandler(store *config.StorageStore, service *usecase.StorageService) *StorageHandler {
	return &StorageHandler{store: store, service: service}
}

// GetSettings 获取存储设置
func (h *StorageHandler) GetSettings(c *gin.Context) {
	settings, err := h.store.LoadSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}

// UpdateSettings 更新存储设置
// 请求体: StorageSettings JSON
func (h *StorageHandler) UpdateSettings(c *gin.Context) {
	var settings config.StorageSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.SaveSettings(&settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 更新输出目录
	if settings.OutputDir != "" {
		h.service.SetBaseDir(settings.OutputDir)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetStatus 获取存储状态
func (h *StorageHandler) GetStatus(c *gin.Context) {
	recording, fileCount, totalBytes := h.service.GetStatus()
	c.JSON(http.StatusOK, gin.H{
		"isRecording": recording,
		"fileCount":   fileCount,
		"totalBytes":  totalBytes,
	})
}

// StartRecording 开始录制
func (h *StorageHandler) StartRecording(c *gin.Context) {
	if err := h.service.StartRecording(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// StopRecording 停止录制
func (h *StorageHandler) StopRecording(c *gin.Context) {
	if err := h.service.StopRecording(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// PickDirectory 获取数据存储目录
func (h *StorageHandler) PickDirectory(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"path": h.service.GetBaseDir()})
}
