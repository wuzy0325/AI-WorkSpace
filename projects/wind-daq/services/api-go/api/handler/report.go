package handler

import (
	"net/http"

	"wind-daq/services/api-go/internal/adapters/report"

	"github.com/gin-gonic/gin"
)

// ==================== 报告生成 HTTP Handler ====================
// 处理报告生成相关的REST API请求

// ReportHandler 报告请求处理器
type ReportHandler struct {
	service *report.Service // 报告服务
}

// NewReportHandler 构建报告处理器
// 参数: service 报告服务
// 返回: *ReportHandler 处理器实例
func NewReportHandler(service *report.Service) *ReportHandler {
	return &ReportHandler{service: service}
}

// GenerateCalibrationReport 生成校准报告
// 请求体: { "calibType": "five-hole", "config": {...}, "results": {...} }
func (h *ReportHandler) GenerateCalibrationReport(c *gin.Context) {
	var body struct {
		CalibType string                 `json:"calibType"` // 校准类型
		Config    map[string]interface{} `json:"config"`    // 校准配置
		Results   map[string]interface{} `json:"results"`   // 校准结果
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path, err := h.service.GenerateCalibrationReport(body.CalibType, body.Config, body.Results)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"filepath": path}})
}

// GenerateTraversalReport 生成曲面扫描报告
// 请求体: { "config": {...}, "results": {...} }
func (h *ReportHandler) GenerateTraversalReport(c *gin.Context) {
	var body struct {
		Config  map[string]interface{} `json:"config"`  // 扫描配置
		Results map[string]interface{} `json:"results"` // 扫描结果
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path, err := h.service.GenerateTraversalReport(body.Config, body.Results)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"filepath": path}})
}

// GenerateReport 生成通用报告
// 请求体: ReportData JSON
func (h *ReportHandler) GenerateReport(c *gin.Context) {
	var data report.ReportData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path, err := h.service.GenerateReport(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"filepath": path}})
}
