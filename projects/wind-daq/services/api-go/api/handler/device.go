package handler

import (
	"net/http"
	"strings"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

type DeviceHandler struct {
	manager     *usecase.DeviceManager
	scanService *usecase.ScanService
}

func NewDeviceHandler(manager *usecase.DeviceManager, scanService *usecase.ScanService) *DeviceHandler {
	return &DeviceHandler{
		manager:     manager,
		scanService: scanService,
	}
}

func (h *DeviceHandler) GetProfiles(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetProfiles())
}

func (h *DeviceHandler) GetInstances(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetInstances())
}

func (h *DeviceHandler) GetStatusAll(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetStatusAll())
}

func (h *DeviceHandler) UpsertProfile(c *gin.Context) {
	var profile device.DeviceProfile
	if err := c.ShouldBindJSON(&profile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	profile.ID = strings.TrimSpace(profile.ID)
	profile.Name = strings.TrimSpace(profile.Name)
	if profile.ID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	if profile.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if profile.Transport == "" {
		profile.Transport = device.TransportTCP
	}
	if profile.SamplingRate <= 0 {
		profile.SamplingRate = 10
	}

	if err := h.manager.UpsertProfile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *DeviceHandler) DeleteProfile(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.DeleteProfile(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *DeviceHandler) Connect(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.Connect(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *DeviceHandler) Disconnect(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.Disconnect(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *DeviceHandler) StartAcquisition(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.StartAcquisition(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *DeviceHandler) StopAcquisition(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.StopAcquisition(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *DeviceHandler) Scan(c *gin.Context) {
	ctx := c.Request.Context()
	timeout := 3 * time.Second

	devices, err := h.scanService.ScanAll(ctx, timeout)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, devices)
}

func (h *DeviceHandler) SetUnit(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"success": false, "error": "SetUnit not implemented"})
}

func (h *DeviceHandler) GetCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"deviceTypes": []string{"SIMULATED", "DAQ-P-1604", "DAQ-T-1603", "WTN_PXI"},
		"transports":  []string{"tcp", "serial"},
	})
}

func (h *DeviceHandler) GetDaqT1603Config(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"success": false, "error": "GetDaqT1603Config not implemented"})
}

func (h *DeviceHandler) ApplyDaqT1603Config(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"success": false, "error": "ApplyDaqT1603Config not implemented"})
}
