package handler

import (
	"net/http"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/usecase"

	"github.com/gin-gonic/gin"
)

// ==================== 运动控制器 HTTP Handler ====================
// 处理运动控制相关的REST API请求

// MotionHandler 运动控制请求处理器
type MotionHandler struct {
	manager *usecase.MotionManager // 运动控制器管理器
}

// NewMotionHandler 构建运动控制器处理器
// 参数: manager 运动控制器管理器
// 返回: *MotionHandler 处理器实例
func NewMotionHandler(manager *usecase.MotionManager) *MotionHandler {
	return &MotionHandler{manager: manager}
}

// GetProfiles 获取所有控制器配置
func (h *MotionHandler) GetProfiles(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetProfiles())
}

// GetStatusAll 获取所有控制器状态
func (h *MotionHandler) GetStatusAll(c *gin.Context) {
	c.JSON(http.StatusOK, h.manager.GetStatusAll())
}

// UpsertProfile 添加或更新控制器配置
// 请求体: MotionControllerProfile JSON
func (h *MotionHandler) UpsertProfile(c *gin.Context) {
	var profile motion.MotionControllerProfile
	if err := c.ShouldBindJSON(&profile); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.manager.UpsertProfile(profile)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteProfile 删除控制器配置
// URL参数: :id 控制器ID
func (h *MotionHandler) DeleteProfile(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.DeleteProfile(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Connect 连接控制器
// URL参数: :id 控制器ID
func (h *MotionHandler) Connect(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.Connect(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Disconnect 断开控制器
// URL参数: :id 控制器ID
func (h *MotionHandler) Disconnect(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.Disconnect(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// MoveTo 绝对位置运动
// URL参数: :id 控制器ID
// 请求体: { "axis": "X", "position": 100.0 }
func (h *MotionHandler) MoveTo(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Axis     string  `json:"axis"`
		Position float64 `json:"position"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.MoveTo(id, motion.AxisName(body.Axis), body.Position); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// MoveBy 相对位置运动(增量)
// URL参数: :id 控制器ID
// 请求体: { "axis": "X", "delta": 10.0 }
func (h *MotionHandler) MoveBy(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Axis  string  `json:"axis"`
		Delta float64 `json:"delta"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.MoveBy(id, motion.AxisName(body.Axis), body.Delta); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Jog 寸动(连续运动)
// URL参数: :id 控制器ID
// 请求体: { "axis": "X", "direction": "+", "speed": 100.0 }
func (h *MotionHandler) Jog(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Axis      string  `json:"axis"`
		Direction string  `json:"direction"`
		Speed     float64 `json:"speed,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.Jog(id, motion.AxisName(body.Axis), body.Direction, body.Speed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Home 回零(寻找原点)
// URL参数: :id 控制器ID
// 请求体: { "axis": "X" }
func (h *MotionHandler) Home(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Axis string `json:"axis"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.Home(id, motion.AxisName(body.Axis)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Stop 停止指定轴或所有轴
// URL参数: :id 控制器ID
// 请求体: { "axis": "X" } (空则停止所有轴)
func (h *MotionHandler) Stop(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Axis string `json:"axis,omitempty"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Axis != "" {
		if err := h.manager.Stop(id, motion.AxisName(body.Axis)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		// 停止所有轴
		for _, axis := range []motion.AxisName{motion.AxisX, motion.AxisY, motion.AxisZ, motion.AxisU} {
			h.manager.Stop(id, axis)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// EmergencyStop 急停(所有轴)
// URL参数: :id 控制器ID
func (h *MotionHandler) EmergencyStop(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.EmergencyStop(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DefinePosition 定义当前位置(用于重新初始化)
// URL参数: :id 控制器ID
// 请求体: { "axis": "X", "position": 0.0 }
func (h *MotionHandler) DefinePosition(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Axis     string  `json:"axis"`
		Position float64 `json:"position"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.DefinePosition(id, motion.AxisName(body.Axis), body.Position); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
