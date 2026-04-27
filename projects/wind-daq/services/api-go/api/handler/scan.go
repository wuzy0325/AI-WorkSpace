package handler

import (
	"net/http"
	"time"

	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type ScanHandler struct {
	scanService *usecase.ScanService
}

func NewScanHandler(scanService *usecase.ScanService) *ScanHandler {
	return &ScanHandler{scanService: scanService}
}

func (h *ScanHandler) ScanDevices(c *gin.Context) {
	var body struct {
		Type    string `json:"type"`
		Timeout int    `json:"timeout"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Type == "" {
		body.Type = "all"
	}
	timeout := 3 * time.Second
	if body.Timeout > 0 {
		timeout = time.Duration(body.Timeout) * time.Millisecond
	}

	ctx := c.Request.Context()

	devices, err := h.scanService.ScanByType(ctx, body.Type, timeout)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": devices})
}
